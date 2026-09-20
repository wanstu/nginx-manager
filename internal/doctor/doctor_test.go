package doctor

import (
	"strings"
	"testing"
	"time"
)

func TestFormatText(t *testing.T) {
	report := Report{
		Version:     "v1.2.3",
		GeneratedAt: time.Unix(0, 0).UTC(),
		Healthy:     false,
		Checks: []Check{
			{Name: "management_password", Status: "ok", Detail: "/tmp/server.json"},
			{Name: "certbot", Status: "warn", Detail: "not installed"},
			{Name: "privileged_helper", Status: "error", Detail: "sudo failed"},
		},
	}
	text := FormatText(report)
	for _, expected := range []string{
		"Nginx Manager doctor: NEEDS ATTENTION",
		"Version: v1.2.3",
		"[OK] management_password: /tmp/server.json",
		"[WARN] certbot: not installed",
		"[ERROR] privileged_helper: sudo failed",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("doctor output missing %q:\n%s", expected, text)
		}
	}
}

func TestFormatTextHealthy(t *testing.T) {
	text := FormatText(Report{
		Healthy: true,
		Checks:  []Check{{Name: "nginx_config", Status: "ok", Detail: "syntax is ok"}},
	})
	if !strings.Contains(text, "Nginx Manager doctor: OK") {
		t.Fatalf("unexpected doctor output:\n%s", text)
	}
}
