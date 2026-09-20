package nginxmgr

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderReverseProxyManagedTLS(t *testing.T) {
	tls := &TLSConfig{
		CertificateName: "me.example.com",
		FullchainPath:   "/etc/letsencrypt/live/me.example.com/fullchain.pem",
		PrivateKeyPath:  "/etc/letsencrypt/live/me.example.com/privkey.pem",
		RedirectHTTPS:   true,
	}
	text := string(renderReverseProxyManaged(
		"me.example.com",
		"http://127.0.0.1:8002",
		true,
		tls,
		"/var/lib/nginx-manager/acme-webroot",
	))

	for _, expected := range []string{
		managedMarker,
		certificateMarkerPrefix + "me.example.com",
		redirectHTTPSMarker,
		"listen 443 ssl;",
		"ssl_certificate /etc/letsencrypt/live/me.example.com/fullchain.pem;",
		"ssl_certificate_key /etc/letsencrypt/live/me.example.com/privkey.pem;",
		"location ^~ /.well-known/acme-challenge/",
		"return 301 https://$host$request_uri;",
		"proxy_pass http://127.0.0.1:8002;",
		"proxy_set_header Upgrade $http_upgrade;",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("TLS config missing %q:\n%s", expected, text)
		}
	}

	site := Site{}
	applyTLSFields(&site, text)
	if !site.HTTPS || !site.RedirectHTTPS || site.Certificate != "me.example.com" {
		t.Fatalf("parsed TLS fields = %+v", site)
	}
}

func TestValidateACMEEmail(t *testing.T) {
	if got, err := validateACMEEmail("admin@example.com"); err != nil || got != "admin@example.com" {
		t.Fatalf("valid email = %q, %v", got, err)
	}
	for _, value := range []string{"", "not-an-email", "Name <admin@example.com>", "a@example.com\nX: y"} {
		if _, err := validateACMEEmail(value); err == nil {
			t.Fatalf("invalid email accepted: %q", value)
		}
	}
}

func TestListCertificates(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NGINX_MANAGER_CERT_LIVE_DIR", root)
	writeTestCertificate(t, root, "me.example.com")

	certificates, err := ListCertificates()
	if err != nil {
		t.Fatal(err)
	}
	if len(certificates) != 1 {
		t.Fatalf("certificate count = %d", len(certificates))
	}
	cert := certificates[0]
	if cert.Name != "me.example.com" || len(cert.Domains) != 1 || cert.Domains[0] != "me.example.com" {
		t.Fatalf("certificate = %+v", cert)
	}
}

func TestUpdateReverseProxyPreservesHTTPS(t *testing.T) {
	t.Setenv("NGINX_MANAGER_SNAPSHOT_DIR", t.TempDir())
	t.Setenv("NGINX_MANAGER_CERT_LIVE_DIR", t.TempDir())
	runner := &scriptedRunner{results: []runnerResult{
		{output: "candidate ok"},
		{output: "live ok"},
		{output: "reload ok"},
	}}
	manager := newTestManager(t, runner)
	tls := tlsConfigForSite("secure.example.com", true)
	path := filepath.Join(manager.Layout.AvailableDir, "nginx-manager-secure.example.com.conf")
	content := renderReverseProxyManaged(
		"secure.example.com",
		"http://127.0.0.1:8002",
		true,
		tls,
		acmeWebroot(),
	)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := manager.UpdateReverseProxy(context.Background(), "nginx-manager-secure.example.com.conf", UpdateReverseProxyRequest{
		ServerName: "secure.example.com",
		Upstream:   "http://127.0.0.1:9000",
		WebSocket:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Site.HTTPS || !result.Site.RedirectHTTPS {
		t.Fatalf("HTTPS state lost: %+v", result.Site)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "listen 443 ssl;") ||
		!strings.Contains(string(updated), "proxy_pass http://127.0.0.1:9000;") {
		t.Fatalf("updated HTTPS config invalid:\n%s", updated)
	}

	_, err = manager.UpdateReverseProxy(context.Background(), "nginx-manager-secure.example.com.conf", UpdateReverseProxyRequest{
		ServerName: "other.example.com",
		Upstream:   "http://127.0.0.1:9000",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot change server_name") {
		t.Fatalf("HTTPS hostname change error = %v", err)
	}
}

func TestIssueCertificateTransaction(t *testing.T) {
	snapshotDir := t.TempDir()
	certRoot := t.TempDir()
	acmeRoot := t.TempDir()
	t.Setenv("NGINX_MANAGER_SNAPSHOT_DIR", snapshotDir)
	t.Setenv("NGINX_MANAGER_CERT_LIVE_DIR", certRoot)
	t.Setenv("NGINX_MANAGER_ACME_WEBROOT", acmeRoot)
	writeTestCertificate(t, certRoot, "me.example.com")

	oldDetector := certbotDetector
	certbotDetector = func(context.Context) CertbotStatus {
		return CertbotStatus{Available: true, Path: "/usr/bin/certbot", Version: "certbot test"}
	}
	defer func() { certbotDetector = oldDetector }()

	runner := &scriptedRunner{results: []runnerResult{
		{output: "challenge candidate ok"},
		{output: "challenge live ok"},
		{output: "challenge reload ok"},
		{output: "certbot ok"},
		{output: "https candidate ok"},
		{output: "https live ok"},
		{output: "https reload ok"},
	}}
	manager := newTestManager(t, runner)
	path := filepath.Join(manager.Layout.AvailableDir, "nginx-manager-me.example.com.conf")
	if err := os.WriteFile(path, renderReverseProxy("me.example.com", "http://127.0.0.1:8002", true), 0o644); err != nil {
		t.Fatal(err)
	}

	site, err := manager.IssueCertificate(context.Background(), "nginx-manager-me.example.com.conf", IssueCertificateRequest{
		Email:         "admin@example.com",
		RedirectHTTPS: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !site.HTTPS || !site.RedirectHTTPS || site.Certificate != "me.example.com" {
		t.Fatalf("issued site = %+v", site)
	}
	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(final), "listen 443 ssl;") {
		t.Fatalf("final config missing HTTPS:\n%s", final)
	}

	foundCertbot := false
	for _, call := range runner.calls {
		if len(call) > 1 && call[0] == "/usr/bin/certbot" {
			joined := strings.Join(call, " ")
			if !strings.Contains(joined, "--cert-name me.example.com") ||
				!strings.Contains(joined, "--domain me.example.com") {
				t.Fatalf("unsafe/unexpected certbot args: %v", call)
			}
			foundCertbot = true
		}
	}
	if !foundCertbot {
		t.Fatal("certbot was not invoked")
	}
}

func TestUpdateSiteTLS(t *testing.T) {
	t.Run("disable keeps certificate files", func(t *testing.T) {
		snapshotDir := t.TempDir()
		certRoot := t.TempDir()
		t.Setenv("NGINX_MANAGER_SNAPSHOT_DIR", snapshotDir)
		t.Setenv("NGINX_MANAGER_CERT_LIVE_DIR", certRoot)
		writeTestCertificate(t, certRoot, "secure.example.com")

		runner := &scriptedRunner{results: []runnerResult{
			{output: "candidate ok"},
			{output: "live ok"},
			{output: "reload ok"},
		}}
		manager := newTestManager(t, runner)
		path := filepath.Join(manager.Layout.AvailableDir, "nginx-manager-secure.example.com.conf")
		tls := tlsConfigForSite("secure.example.com", true)
		options := ProxyOptions{MaxBodySizeMB: 256, ConnectTimeoutSeconds: 12, ReadTimeoutSeconds: 180}
		if err := os.WriteFile(path, renderReverseProxyManaged(
			"secure.example.com",
			"http://127.0.0.1:8002",
			true,
			tls,
			acmeWebroot(),
			options,
		), 0o644); err != nil {
			t.Fatal(err)
		}

		site, err := manager.UpdateSiteTLS(context.Background(), "nginx-manager-secure.example.com.conf", UpdateTLSRequest{
			Enabled: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		if site.HTTPS || site.RedirectHTTPS {
			t.Fatalf("TLS was not disabled: %+v", site)
		}
		if site.MaxBodySizeMB != 256 || site.ConnectTimeoutSeconds != 12 || site.ReadTimeoutSeconds != 180 {
			t.Fatalf("proxy options were lost while disabling TLS: %+v", site)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "listen 443 ssl;") {
			t.Fatalf("TLS directives remain:\n%s", data)
		}
		if !strings.Contains(string(data), "location ^~ /.well-known/acme-challenge/") {
			t.Fatalf("ACME challenge route was removed:\n%s", data)
		}
		if _, err := os.Stat(filepath.Join(certRoot, "secure.example.com", "fullchain.pem")); err != nil {
			t.Fatalf("certificate was removed: %v", err)
		}

		manager.Runner = &scriptedRunner{results: []runnerResult{
			{output: "candidate ok"},
			{output: "live ok"},
			{output: "reload ok"},
		}}
		reenabled, err := manager.UpdateSiteTLS(context.Background(), "nginx-manager-secure.example.com.conf", UpdateTLSRequest{
			Enabled:       true,
			RedirectHTTPS: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !reenabled.HTTPS || reenabled.RedirectHTTPS {
			t.Fatalf("existing certificate was not reused: %+v", reenabled)
		}
		if reenabled.MaxBodySizeMB != 256 || reenabled.ConnectTimeoutSeconds != 12 || reenabled.ReadTimeoutSeconds != 180 {
			t.Fatalf("proxy options were lost while re-enabling TLS: %+v", reenabled)
		}
	})

	t.Run("toggle redirect preserves TLS", func(t *testing.T) {
		snapshotDir := t.TempDir()
		certRoot := t.TempDir()
		t.Setenv("NGINX_MANAGER_SNAPSHOT_DIR", snapshotDir)
		t.Setenv("NGINX_MANAGER_CERT_LIVE_DIR", certRoot)
		writeTestCertificate(t, certRoot, "secure.example.com")

		runner := &scriptedRunner{results: []runnerResult{
			{output: "candidate ok"},
			{output: "live ok"},
			{output: "reload ok"},
		}}
		manager := newTestManager(t, runner)
		path := filepath.Join(manager.Layout.AvailableDir, "nginx-manager-secure.example.com.conf")
		tls := tlsConfigForSite("secure.example.com", false)
		if err := os.WriteFile(path, renderReverseProxyManaged(
			"secure.example.com",
			"http://127.0.0.1:8002",
			true,
			tls,
			acmeWebroot(),
		), 0o644); err != nil {
			t.Fatal(err)
		}

		site, err := manager.UpdateSiteTLS(context.Background(), "nginx-manager-secure.example.com.conf", UpdateTLSRequest{
			Enabled:       true,
			RedirectHTTPS: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !site.HTTPS || !site.RedirectHTTPS {
			t.Fatalf("redirect setting was not applied: %+v", site)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), redirectHTTPSMarker) ||
			!strings.Contains(string(data), "listen 443 ssl;") {
			t.Fatalf("TLS config invalid:\n%s", data)
		}
	})
}

func writeTestCertificate(t *testing.T, root, domain string) {
	t.Helper()
	dir := filepath.Join(root, domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
}
