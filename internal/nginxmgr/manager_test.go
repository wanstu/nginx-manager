package nginxmgr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type scriptedRunner struct {
	results []runnerResult
	calls   [][]string
}

type runnerResult struct {
	output string
	err    error
}

func (r *scriptedRunner) Run(_ context.Context, path string, args ...string) (string, error) {
	call := append([]string{path}, args...)
	r.calls = append(r.calls, call)
	if len(r.results) == 0 {
		return "", nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result.output, result.err
}

func newTestManager(t *testing.T, runner Runner) *Manager {
	t.Helper()
	root := t.TempDir()
	confD := filepath.Join(root, "conf.d")
	if err := os.MkdirAll(confD, 0o755); err != nil {
		t.Fatal(err)
	}
	return &Manager{
		Runtime: RuntimeInfo{Kind: "nginx", Path: "/usr/sbin/nginx", Version: "nginx/test"},
		Layout: Layout{
			MainConfig:   filepath.Join(root, "nginx.conf"),
			AvailableDir: confD,
			EnabledDir:   confD,
			Mode:         "conf.d",
		},
		Runner: runner,
	}
}

func TestCreateReverseProxySuccess(t *testing.T) {
	runner := &scriptedRunner{results: []runnerResult{
		{output: "candidate ok"},
		{output: "live ok"},
		{output: "reload ok"},
	}}
	manager := newTestManager(t, runner)

	result, err := manager.CreateReverseProxy(context.Background(), ReverseProxyRequest{
		ServerName: "me.example.com",
		Upstream:   "http://127.0.0.1:8002",
		WebSocket:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Site.Managed || !result.Site.Enabled {
		t.Fatalf("unexpected site: %+v", result.Site)
	}
	data, err := os.ReadFile(result.Site.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{
		managedMarker,
		"server_name me.example.com;",
		"proxy_pass http://127.0.0.1:8002;",
		"proxy_set_header Upgrade $http_upgrade;",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated site missing %q:\n%s", expected, text)
		}
	}

	sites, err := manager.ListSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].ServerName != "me.example.com" || !sites[0].Managed {
		t.Fatalf("ListSites() = %+v", sites)
	}
}

func TestCreateReverseProxyCandidateFailureDoesNotWrite(t *testing.T) {
	runner := &scriptedRunner{results: []runnerResult{
		{output: "candidate invalid", err: errors.New("exit 1")},
	}}
	manager := newTestManager(t, runner)

	_, err := manager.CreateReverseProxy(context.Background(), ReverseProxyRequest{
		ServerName: "bad.example.com",
		Upstream:   "http://127.0.0.1:8002",
	})
	if err == nil || !strings.Contains(err.Error(), "candidate nginx -t failed") {
		t.Fatalf("error = %v", err)
	}
	entries, _ := os.ReadDir(manager.Layout.AvailableDir)
	if len(entries) != 0 {
		t.Fatalf("candidate failure left files: %+v", entries)
	}
}

func TestCreateReverseProxyLiveTestFailureRollsBack(t *testing.T) {
	runner := &scriptedRunner{results: []runnerResult{
		{output: "candidate ok"},
		{output: "duplicate listen", err: errors.New("exit 1")},
	}}
	manager := newTestManager(t, runner)

	_, err := manager.CreateReverseProxy(context.Background(), ReverseProxyRequest{
		ServerName: "rollback.example.com",
		Upstream:   "http://127.0.0.1:8002",
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("error = %v", err)
	}
	entries, _ := os.ReadDir(manager.Layout.AvailableDir)
	if len(entries) != 0 {
		t.Fatalf("live test failure left files: %+v", entries)
	}
}

func TestCreateReverseProxyReloadFailureRollsBack(t *testing.T) {
	runner := &scriptedRunner{results: []runnerResult{
		{output: "candidate ok"},
		{output: "live ok"},
		{output: "reload failed", err: errors.New("exit 1")},
		{output: "old config ok"},
		{output: "old reload ok"},
	}}
	manager := newTestManager(t, runner)

	_, err := manager.CreateReverseProxy(context.Background(), ReverseProxyRequest{
		ServerName: "reload.example.com",
		Upstream:   "http://127.0.0.1:8002",
	})
	if err == nil || !strings.Contains(err.Error(), "reload failed") {
		t.Fatalf("error = %v", err)
	}
	entries, _ := os.ReadDir(manager.Layout.AvailableDir)
	if len(entries) != 0 {
		t.Fatalf("reload failure left files: %+v", entries)
	}
	if len(runner.calls) != 5 {
		t.Fatalf("runner calls = %d, want 5", len(runner.calls))
	}
}

func TestSetSiteEnabledConfDRoundTrip(t *testing.T) {
	snapshotDir := t.TempDir()
	t.Setenv("NGINX_MANAGER_SNAPSHOT_DIR", snapshotDir)
	runner := &scriptedRunner{results: []runnerResult{
		{output: "disable test ok"},
		{output: "disable reload ok"},
		{output: "enable test ok"},
		{output: "enable reload ok"},
	}}
	manager := newTestManager(t, runner)
	path := filepath.Join(manager.Layout.AvailableDir, "nginx-manager-toggle.example.com.conf")
	content := renderReverseProxy("toggle.example.com", "http://127.0.0.1:8002", false)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	disabled, err := manager.SetSiteEnabled(context.Background(), "nginx-manager-toggle.example.com.conf", false)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled {
		t.Fatal("site remained enabled")
	}
	if _, err := os.Stat(path + ".disabled"); err != nil {
		t.Fatalf("disabled site missing: %v", err)
	}
	sites, err := manager.ListSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].ID != "nginx-manager-toggle.example.com.conf" || sites[0].Enabled {
		t.Fatalf("disabled ListSites() = %+v", sites)
	}

	enabled, err := manager.SetSiteEnabled(context.Background(), "nginx-manager-toggle.example.com.conf", true)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.Enabled {
		t.Fatal("site remained disabled")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("enabled site missing: %v", err)
	}

	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(entries))
	}
}

func TestSetSiteEnabledRollbackOnReloadFailure(t *testing.T) {
	t.Setenv("NGINX_MANAGER_SNAPSHOT_DIR", t.TempDir())
	runner := &scriptedRunner{results: []runnerResult{
		{output: "test ok"},
		{output: "reload failed", err: errors.New("exit 1")},
		{output: "rollback test ok"},
		{output: "rollback reload ok"},
	}}
	manager := newTestManager(t, runner)
	path := filepath.Join(manager.Layout.AvailableDir, "nginx-manager-rollback-toggle.example.com.conf")
	if err := os.WriteFile(path, renderReverseProxy("rollback-toggle.example.com", "http://127.0.0.1:8002", false), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := manager.SetSiteEnabled(context.Background(), "nginx-manager-rollback-toggle.example.com.conf", false)
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("enabled file was not restored: %v", err)
	}
	if _, err := os.Stat(path + ".disabled"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled file remained after rollback: %v", err)
	}
}

func TestDeleteManagedSiteRollbackAndExternalProtection(t *testing.T) {
	t.Setenv("NGINX_MANAGER_SNAPSHOT_DIR", t.TempDir())
	runner := &scriptedRunner{results: []runnerResult{
		{output: "test ok"},
		{output: "reload failed", err: errors.New("exit 1")},
		{output: "rollback test ok"},
		{output: "rollback reload ok"},
	}}
	manager := newTestManager(t, runner)
	managedPath := filepath.Join(manager.Layout.AvailableDir, "nginx-manager-delete.example.com.conf")
	managedContent := renderReverseProxy("delete.example.com", "http://127.0.0.1:8002", false)
	if err := os.WriteFile(managedPath, managedContent, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := manager.DeleteSite(context.Background(), "nginx-manager-delete.example.com.conf")
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("delete error = %v", err)
	}
	restored, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("managed site not restored: %v", err)
	}
	if string(restored) != string(managedContent) {
		t.Fatal("managed site content changed after rollback")
	}

	externalPath := filepath.Join(manager.Layout.AvailableDir, "nginx-manager-external.example.com.conf")
	if err := os.WriteFile(externalPath, []byte("server { server_name external.example.com; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.DeleteSite(context.Background(), "nginx-manager-external.example.com.conf"); err == nil ||
		!strings.Contains(err.Error(), "external") {
		t.Fatalf("external delete error = %v", err)
	}
}

func TestValidationRejectsUnsafeInput(t *testing.T) {
	manager := newTestManager(t, &scriptedRunner{})
	cases := []ReverseProxyRequest{
		{ServerName: "bad;include.example", Upstream: "http://127.0.0.1:8002"},
		{ServerName: "ok.example.com", Upstream: "file:///etc/passwd"},
		{ServerName: "ok.example.com", Upstream: "http://127.0.0.1:8002; include /tmp/x"},
		{ServerName: "ok.example.com", Upstream: "http://$backend:8000"},
	}
	for _, req := range cases {
		if _, err := manager.CreateReverseProxy(context.Background(), req); err == nil {
			t.Fatalf("unsafe request succeeded: %+v", req)
		}
	}
}
