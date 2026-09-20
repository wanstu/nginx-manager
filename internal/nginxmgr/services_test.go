package nginxmgr

import (
	"context"
	"strings"
	"testing"
)

type serviceRunner map[string]string

func (r serviceRunner) Run(_ context.Context, path string, args ...string) (string, error) {
	key := path + " " + strings.Join(args, " ")
	return r[key], nil
}

func TestDetectServiceDiagnosticsPrefersRuntimeKind(t *testing.T) {
	runner := serviceRunner{
		"/bin/systemctl show nginx-manager.service --property=LoadState --value": "loaded",
		"/bin/systemctl is-enabled nginx-manager.service":                        "enabled",
		"/bin/systemctl is-active nginx-manager.service":                         "active",

		"/bin/systemctl show nginx.service --property=LoadState --value": "loaded",
		"/bin/systemctl is-enabled nginx.service":                        "enabled",
		"/bin/systemctl is-active nginx.service":                         "active",

		"/bin/systemctl show openresty.service --property=LoadState --value": "loaded",
		"/bin/systemctl is-enabled openresty.service":                        "disabled",
		"/bin/systemctl is-active openresty.service":                         "active",
	}

	result := detectServiceDiagnosticsWith(
		context.Background(),
		RuntimeInfo{Kind: "openresty"},
		"/bin/systemctl",
		runner,
	)
	if !result.SystemdAvailable || !result.Manager.Installed || !result.Manager.Enabled || !result.Manager.Active {
		t.Fatalf("manager status = %+v", result.Manager)
	}
	if result.Runtime.Name != "openresty.service" || !result.Runtime.Installed || result.Runtime.Enabled || !result.Runtime.Active {
		t.Fatalf("runtime status = %+v", result.Runtime)
	}
}

func TestDetectServiceDiagnosticsFallsBackToInstalledUnit(t *testing.T) {
	runner := serviceRunner{
		"/bin/systemctl show nginx.service --property=LoadState --value": "loaded",
		"/bin/systemctl is-enabled nginx.service":                        "enabled",
		"/bin/systemctl is-active nginx.service":                         "active",
	}
	result := detectServiceDiagnosticsWith(
		context.Background(),
		RuntimeInfo{Kind: "openresty"},
		"/bin/systemctl",
		runner,
	)
	if result.Runtime.Name != "nginx.service" || !result.Runtime.Installed {
		t.Fatalf("runtime status = %+v", result.Runtime)
	}
}
