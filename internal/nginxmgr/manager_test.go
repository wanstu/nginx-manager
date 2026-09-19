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
