package nginxmgr

import (
	"context"
	"os/exec"
	"strings"
)

const renewalTimerUnit = "nginx-manager-renew.timer"

type RenewalTimerStatus struct {
	SystemdAvailable bool   `json:"systemd_available"`
	Installed        bool   `json:"installed"`
	Enabled          bool   `json:"enabled"`
	Active           bool   `json:"active"`
	LoadState        string `json:"load_state,omitempty"`
}

func DetectRenewalTimer(ctx context.Context) RenewalTimerStatus {
	path, err := exec.LookPath("systemctl")
	if err != nil {
		return RenewalTimerStatus{}
	}
	status := RenewalTimerStatus{SystemdAvailable: true}

	loadState, _ := ExecRunner{}.Run(ctx, path, "show", renewalTimerUnit, "--property=LoadState", "--value")
	status.LoadState = strings.TrimSpace(loadState)
	status.Installed = status.LoadState == "loaded"

	enabled, _ := ExecRunner{}.Run(ctx, path, "is-enabled", renewalTimerUnit)
	status.Enabled = strings.TrimSpace(enabled) == "enabled"

	active, _ := ExecRunner{}.Run(ctx, path, "is-active", renewalTimerUnit)
	status.Active = strings.TrimSpace(active) == "active"
	return status
}
