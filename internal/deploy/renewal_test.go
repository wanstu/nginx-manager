package deploy

import (
	"strings"
	"testing"
)

func TestRenewalServiceUnit(t *testing.T) {
	unit, err := RenewalServiceUnit("/usr/local/bin/nginx-manager")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unit, "ExecStart=/usr/local/bin/nginx-manager privileged renew-certificates") {
		t.Fatalf("unexpected renewal service:\n%s", unit)
	}
}

func TestRenewalServiceUnitRejectsUnsafePath(t *testing.T) {
	for _, value := range []string{"nginx-manager", "/usr/local/bin/nginx manager", "/tmp/x\nExecStart=/bin/sh"} {
		if _, err := RenewalServiceUnit(value); err == nil {
			t.Fatalf("unsafe path accepted: %q", value)
		}
	}
}

func TestRenewalTimerUnit(t *testing.T) {
	unit := RenewalTimerUnit()
	for _, expected := range []string{
		"OnCalendar=*-*-* 03,15:00:00",
		"RandomizedDelaySec=45m",
		"Persistent=true",
		"Unit=nginx-manager-renew.service",
	} {
		if !strings.Contains(unit, expected) {
			t.Fatalf("timer missing %q:\n%s", expected, unit)
		}
	}
}
