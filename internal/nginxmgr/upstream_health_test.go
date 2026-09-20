package nginxmgr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckUpstreamUsesSavedManagedSite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	manager := newTestManager(t, &scriptedRunner{})
	siteID := siteFileName("health.example.com", manager.Layout.Mode)
	content := renderReverseProxyManaged(
		"health.example.com",
		server.URL,
		false,
		nil,
		"",
		ProxyOptions{},
	)
	if err := os.WriteFile(filepath.Join(manager.Layout.AvailableDir, siteID), content, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := manager.CheckUpstream(context.Background(), siteID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reachable || !result.Healthy || result.StatusCode != http.StatusNoContent {
		t.Fatalf("health = %+v", result)
	}
	if result.Upstream != server.URL {
		t.Fatalf("upstream = %q", result.Upstream)
	}
}

func TestCheckUpstreamRejectsExternalSite(t *testing.T) {
	manager := newTestManager(t, &scriptedRunner{})
	siteID := siteFileName("external.example.com", manager.Layout.Mode)
	if err := os.WriteFile(
		filepath.Join(manager.Layout.AvailableDir, siteID),
		[]byte("server { server_name external.example.com; proxy_pass http://127.0.0.1:1; }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.CheckUpstream(context.Background(), siteID); err == nil {
		t.Fatal("external site was accepted")
	}
}
