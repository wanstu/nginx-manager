package nginxmgr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type logRunner struct {
	config string
	prefix string
}

func (r logRunner) Run(_ context.Context, _ string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "-T" {
		return r.config, nil
	}
	if len(args) > 0 && args[0] == "-V" {
		return "configure arguments: --prefix=" + r.prefix, nil
	}
	return "", nil
}

func TestListLogsRestrictsPathsAndTailIsBounded(t *testing.T) {
	prefix := t.TempDir()
	logDir := filepath.Join(prefix, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	accessPath := filepath.Join(logDir, "access.log")
	var body strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&body, "line-%03d\n", i)
	}
	if err := os.WriteFile(accessPath, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := &Manager{
		Runtime: RuntimeInfo{Kind: "nginx", Path: "/usr/sbin/nginx"},
		Layout:  Layout{MainConfig: filepath.Join(prefix, "nginx.conf")},
		Runner: logRunner{
			prefix: prefix,
			config: "http {\n    access_log " + accessPath + ";\n    error_log /etc/passwd;\n}\n",
		},
	}

	logs, err := manager.ListLogs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %+v; want exactly one allowed log", logs)
	}
	if logs[0].Kind != "access" || logs[0].Path != filepath.Clean(accessPath) {
		t.Fatalf("unexpected log = %+v", logs[0])
	}

	tail, err := manager.TailLog(context.Background(), logs[0].ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if tail.Lines != 20 || !tail.Truncated {
		t.Fatalf("tail metadata = %+v", tail)
	}
	if !strings.HasPrefix(tail.Content, "line-281\n") || !strings.HasSuffix(tail.Content, "line-300") {
		t.Fatalf("unexpected tail content:\n%s", tail.Content)
	}
}

func TestPathWithinRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "logs")
	inside := filepath.Join(root, "site", "access.log")
	outside := filepath.Join(filepath.Dir(root), "secret.log")

	if !pathWithinRoots(inside, []string{root}) {
		t.Fatal("inside path was rejected")
	}
	if pathWithinRoots(outside, []string{root}) {
		t.Fatal("outside path was accepted")
	}
}

func TestTailLogRejectsTooManyLines(t *testing.T) {
	manager := &Manager{
		Runtime: RuntimeInfo{Kind: "nginx", Path: "/usr/sbin/nginx"},
		Layout:  Layout{MainConfig: filepath.Join(t.TempDir(), "nginx.conf")},
		Runner:  logRunner{prefix: t.TempDir()},
	}
	if _, err := manager.TailLog(context.Background(), "missing", MaxLogLines+1); err == nil {
		t.Fatal("too many lines were accepted")
	}
}
