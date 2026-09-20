package nginxmgr

import (
	"context"
	"os/exec"
	"strings"
)

type ServiceUnitStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	Active    bool   `json:"active"`
	LoadState string `json:"load_state,omitempty"`
}

type ServiceDiagnostics struct {
	SystemdAvailable bool                `json:"systemd_available"`
	Manager          ServiceUnitStatus   `json:"manager"`
	Runtime          ServiceUnitStatus   `json:"runtime"`
	Candidates       []ServiceUnitStatus `json:"candidates"`
}

func DetectServiceDiagnostics(ctx context.Context, runtime RuntimeInfo) ServiceDiagnostics {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return ServiceDiagnostics{}
	}

	return detectServiceDiagnosticsWith(ctx, runtime, systemctl, ExecRunner{})
}

func detectServiceDiagnosticsWith(ctx context.Context, runtime RuntimeInfo, systemctl string, runner Runner) ServiceDiagnostics {
	result := ServiceDiagnostics{SystemdAvailable: true}
	result.Manager = inspectSystemdUnit(ctx, runner, systemctl, "nginx-manager.service")

	names := []string{"nginx.service", "openresty.service"}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		status := inspectSystemdUnit(ctx, runner, systemctl, name)
		result.Candidates = append(result.Candidates, status)
	}

	preferred := "nginx.service"
	if strings.EqualFold(runtime.Kind, "openresty") {
		preferred = "openresty.service"
	}
	for _, status := range result.Candidates {
		if status.Name == preferred && status.Installed {
			result.Runtime = status
			return result
		}
	}
	for _, status := range result.Candidates {
		if status.Installed {
			result.Runtime = status
			return result
		}
	}
	result.Runtime = ServiceUnitStatus{Name: preferred}
	return result
}

func inspectSystemdUnit(ctx context.Context, runner Runner, systemctl, name string) ServiceUnitStatus {
	status := ServiceUnitStatus{Name: name}
	loadState, _ := runner.Run(ctx, systemctl, "show", name, "--property=LoadState", "--value")
	status.LoadState = strings.TrimSpace(loadState)
	status.Installed = status.LoadState == "loaded"

	enabled, _ := runner.Run(ctx, systemctl, "is-enabled", name)
	status.Enabled = strings.TrimSpace(enabled) == "enabled"

	active, _ := runner.Run(ctx, systemctl, "is-active", name)
	status.Active = strings.TrimSpace(active) == "active"
	return status
}
