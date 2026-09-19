package nginxmgr

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wanstu/wails-desktop-kit/atomicfile"
)

var certbotDetector = DetectCertbot

const (
	acmeWebrootDefault      = "/var/lib/nginx-manager/acme-webroot"
	letsEncryptLiveDefault  = "/etc/letsencrypt/live"
	redirectHTTPSMarker     = "# nginx-manager-redirect-http-to-https"
	certificateMarkerPrefix = "# nginx-manager-certificate: "
)

type TLSConfig struct {
	CertificateName string
	FullchainPath   string
	PrivateKeyPath  string
	RedirectHTTPS   bool
}

type IssueCertificateRequest struct {
	Email         string `json:"email"`
	RedirectHTTPS bool   `json:"redirect_https"`
}

type Certificate struct {
	Name      string    `json:"name"`
	Domains   []string  `json:"domains"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	Issuer    string    `json:"issuer"`
	Path      string    `json:"path"`
}

type CertbotStatus struct {
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

func DetectCertbot(ctx context.Context) CertbotStatus {
	path, err := exec.LookPath("certbot")
	if err != nil {
		return CertbotStatus{}
	}
	output, _ := ExecRunner{}.Run(ctx, path, "--version")
	return CertbotStatus{Available: true, Path: path, Version: strings.TrimSpace(output)}
}

func (m *Manager) IssueCertificate(ctx context.Context, siteID string, req IssueCertificateRequest) (Site, error) {
	mutationMu.Lock()
	defer mutationMu.Unlock()

	current, err := m.loadManagedSite(siteID)
	if err != nil {
		return Site{}, err
	}
	if !current.Site.Enabled {
		return Site{}, errors.New("site must be enabled before requesting a certificate")
	}
	if current.Site.ServerName == "" || current.Site.ServerName == "_" || net.ParseIP(current.Site.ServerName) != nil {
		return Site{}, errors.New("site must use a DNS hostname before requesting a certificate")
	}
	email, err := validateACMEEmail(req.Email)
	if err != nil {
		return Site{}, err
	}
	certbot := certbotDetector(ctx)
	if !certbot.Available {
		return Site{}, errors.New("certbot is not installed or not available in PATH")
	}
	if err := m.snapshotSite("enable_https", current); err != nil {
		return Site{}, err
	}

	webroot := acmeWebroot()
	if err := os.MkdirAll(filepath.Join(webroot, ".well-known", "acme-challenge"), 0o755); err != nil {
		return Site{}, fmt.Errorf("create ACME webroot: %w", err)
	}

	var existingTLS *TLSConfig
	if current.Site.HTTPS {
		existingTLS = tlsConfigForSite(current.Site.ServerName, current.Site.RedirectHTTPS)
	}
	challengeContent := renderReverseProxyManaged(
		current.Site.ServerName,
		current.Site.ProxyPass,
		current.Site.WebSocket,
		existingTLS,
		webroot,
	)
	if err := m.validateCandidate(ctx, siteID, challengeContent); err != nil {
		return Site{}, err
	}
	if err := m.replaceManagedSiteContent(ctx, current, challengeContent); err != nil {
		return Site{}, err
	}

	restoreOriginal := func() {
		_ = m.removeManagedSiteFiles(siteID)
		_ = m.restoreManagedState(current)
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = m.Runner.Run(rollbackCtx, m.Runtime.Path, "-t")
		_, _ = m.Runner.Run(rollbackCtx, m.Runtime.Path, "-s", "reload")
	}

	output, issueErr := m.Runner.Run(ctx, certbot.Path,
		"certonly",
		"--webroot",
		"--webroot-path", webroot,
		"--domain", current.Site.ServerName,
		"--cert-name", current.Site.ServerName,
		"--email", email,
		"--agree-tos",
		"--non-interactive",
		"--keep-until-expiring",
	)
	if issueErr != nil {
		restoreOriginal()
		return Site{}, fmt.Errorf("certbot certificate request failed: %s", output)
	}

	tls := tlsConfigForSite(current.Site.ServerName, req.RedirectHTTPS)
	if err := validateCertificateFiles(tls, current.Site.ServerName); err != nil {
		restoreOriginal()
		return Site{}, err
	}
	finalContent := renderReverseProxyManaged(
		current.Site.ServerName,
		current.Site.ProxyPass,
		current.Site.WebSocket,
		tls,
		webroot,
	)
	if err := m.validateCandidate(ctx, siteID, finalContent); err != nil {
		restoreOriginal()
		return Site{}, fmt.Errorf("HTTPS candidate validation failed: %w", err)
	}

	challengeState := managedSiteState{
		Site:       parseManagedSite(siteID, current.ActualPath, true, challengeContent),
		Content:    challengeContent,
		ActualPath: current.ActualPath,
	}
	if err := m.replaceManagedSiteContent(ctx, challengeState, finalContent); err != nil {
		restoreOriginal()
		return Site{}, err
	}
	return parseManagedSite(siteID, current.ActualPath, true, finalContent), nil
}

func (m *Manager) RenewCertificates(ctx context.Context) (string, error) {
	mutationMu.Lock()
	defer mutationMu.Unlock()

	certbot := certbotDetector(ctx)
	if !certbot.Available {
		return "", errors.New("certbot is not installed or not available in PATH")
	}
	output, err := m.Runner.Run(ctx, certbot.Path, "renew", "--non-interactive")
	if err != nil {
		return "", fmt.Errorf("certbot renew failed: %s", output)
	}
	testOutput, err := m.Runner.Run(ctx, m.Runtime.Path, "-t")
	if err != nil {
		return "", fmt.Errorf("nginx -t after certificate renewal failed: %s", testOutput)
	}
	reloadOutput, err := m.Runner.Run(ctx, m.Runtime.Path, "-s", "reload")
	if err != nil {
		return "", fmt.Errorf("nginx reload after certificate renewal failed: %s", reloadOutput)
	}
	return output, nil
}

func ListCertificates() ([]Certificate, error) {
	root := letsEncryptLiveRoot()
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []Certificate{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read certificate directory: %w", err)
	}

	certificates := make([]Certificate, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		if filepath.Base(name) != name {
			continue
		}
		fullchain := filepath.Join(root, name, "fullchain.pem")
		cert, err := readCertificate(fullchain)
		if err != nil {
			continue
		}
		certificates = append(certificates, Certificate{
			Name:      name,
			Domains:   append([]string(nil), cert.DNSNames...),
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
			Issuer:    cert.Issuer.String(),
			Path:      fullchain,
		})
	}
	sort.Slice(certificates, func(i, j int) bool {
		return certificates[i].Name < certificates[j].Name
	})
	return certificates, nil
}

func validateACMEEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 254 || strings.ContainsAny(value, "\r\n") {
		return "", errors.New("a valid ACME contact email is required")
	}
	address, err := mail.ParseAddress(value)
	if err != nil || !strings.EqualFold(address.Address, value) {
		return "", errors.New("a valid ACME contact email is required")
	}
	return value, nil
}

func tlsConfigForSite(serverName string, redirect bool) *TLSConfig {
	root := letsEncryptLiveRoot()
	return &TLSConfig{
		CertificateName: serverName,
		FullchainPath:   filepath.Join(root, serverName, "fullchain.pem"),
		PrivateKeyPath:  filepath.Join(root, serverName, "privkey.pem"),
		RedirectHTTPS:   redirect,
	}
}

func validateCertificateFiles(tls *TLSConfig, serverName string) error {
	if tls == nil {
		return errors.New("TLS configuration is required")
	}
	cert, err := readCertificate(tls.FullchainPath)
	if err != nil {
		return fmt.Errorf("read issued certificate: %w", err)
	}
	if err := cert.VerifyHostname(serverName); err != nil {
		return fmt.Errorf("issued certificate does not match %s: %w", serverName, err)
	}
	info, err := os.Stat(tls.PrivateKeyPath)
	if err != nil || info.IsDir() {
		return errors.New("issued private key was not found")
	}
	return nil
}

func readCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("certificate PEM is invalid")
	}
	return x509.ParseCertificate(block.Bytes)
}

func acmeWebroot() string {
	if value := strings.TrimSpace(os.Getenv("NGINX_MANAGER_ACME_WEBROOT")); value != "" {
		return value
	}
	return acmeWebrootDefault
}

func letsEncryptLiveRoot() string {
	if value := strings.TrimSpace(os.Getenv("NGINX_MANAGER_CERT_LIVE_DIR")); value != "" {
		return value
	}
	return letsEncryptLiveDefault
}

func (m *Manager) replaceManagedSiteContent(ctx context.Context, current managedSiteState, content []byte) error {
	if !strings.Contains(string(content), managedMarker) {
		return errors.New("refusing to write unmanaged nginx content")
	}
	if err := atomicfile.Write(current.ActualPath, content, 0o644); err != nil {
		return fmt.Errorf("write managed site: %w", err)
	}
	rollback := func() {
		_ = atomicfile.Write(current.ActualPath, current.Content, 0o644)
	}
	if err := m.testAndReload(ctx, rollback); err != nil {
		return err
	}
	return nil
}

func applyTLSFields(site *Site, text string) {
	site.RedirectHTTPS = strings.Contains(text, redirectHTTPSMarker)
	if match := sslCertificateRE.FindStringSubmatch(text); len(match) == 2 {
		site.HTTPS = true
		site.Certificate = strings.TrimSpace(match[1])
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, certificateMarkerPrefix) {
			site.Certificate = strings.TrimSpace(strings.TrimPrefix(line, certificateMarkerPrefix))
			break
		}
	}
}
