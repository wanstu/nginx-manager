package privilege

import (
	"strings"
	"testing"
)

func TestSudoersRule(t *testing.T) {
	got, err := SudoersRule("nginx-manager", "/usr/local/bin/nginx-manager")
	if err != nil {
		t.Fatal(err)
	}
	want := "nginx-manager ALL=(root) NOPASSWD: /usr/local/bin/nginx-manager privileged apply"
	if !strings.Contains(got, want) {
		t.Fatalf("rule = %q; want line %q", got, want)
	}
}

func TestSudoersRuleRejectsInjection(t *testing.T) {
	cases := [][2]string{
		{"nginx-manager ALL=(ALL)", "/usr/local/bin/nginx-manager"},
		{"nginx-manager", "nginx-manager"},
		{"nginx-manager", "/usr/local/bin/nginx manager"},
		{"nginx-manager", "/usr/local/bin/nginx-manager, /bin/sh"},
	}
	for _, tc := range cases {
		if _, err := SudoersRule(tc[0], tc[1]); err == nil {
			t.Fatalf("SudoersRule(%q, %q) succeeded", tc[0], tc[1])
		}
	}
}
