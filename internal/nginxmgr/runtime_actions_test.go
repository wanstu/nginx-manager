package nginxmgr

import (
	"context"
	"strings"
	"testing"
)

func TestSafeReload(t *testing.T) {
	runner := &scriptedRunner{results: []runnerResult{
		{output: "syntax is ok"},
		{output: "reload ok"},
	}}
	manager := newTestManager(t, runner)

	result, err := manager.SafeReload(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.TestOutput != "syntax is ok" || result.ReloadOutput != "reload ok" {
		t.Fatalf("result = %+v", result)
	}
	if len(runner.calls) != 2 ||
		strings.Join(runner.calls[0], " ") == strings.Join(runner.calls[1], " ") {
		t.Fatalf("unexpected calls: %+v", runner.calls)
	}
}

func TestSafeReloadStopsWhenConfigInvalid(t *testing.T) {
	runner := &scriptedRunner{results: []runnerResult{
		{output: "syntax error", err: context.Canceled},
	}}
	manager := newTestManager(t, runner)

	_, err := manager.SafeReload(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reload was not attempted") {
		t.Fatalf("error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("reload should not run: %+v", runner.calls)
	}
}
