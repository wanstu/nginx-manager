package deploy

import (
	"strings"
	"testing"
)

func TestSystemdUnit(t *testing.T) {
	unit, err := SystemdUnit(SystemdOptions{
		ServiceUser: "nginx-manager",
		BinaryPath:  "/usr/local/bin/nginx-manager",
		Listen:      "127.0.0.1:8020",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"User=nginx-manager",
		"ExecStart=/usr/local/bin/nginx-manager serve --listen 127.0.0.1:8020",
		"NGINX_MANAGER_PRIVILEGED_HELPER=/usr/local/bin/nginx-manager",
		"StateDirectory=nginx-manager",
	} {
		if !strings.Contains(unit, expected) {
			t.Fatalf("unit missing %q", expected)
		}
	}
}

func TestSystemdUnitRejectsRemoteListen(t *testing.T) {
	_, err := SystemdUnit(SystemdOptions{
		ServiceUser: "nginx-manager",
		BinaryPath:  "/usr/local/bin/nginx-manager",
		Listen:      "0.0.0.0:8020",
	})
	if err == nil {
		t.Fatal("remote listen was accepted")
	}
}
