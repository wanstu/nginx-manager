package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wanstu/nginx-manager/internal/nginxmgr"
	"github.com/wanstu/nginx-manager/internal/privilege"
	"github.com/wanstu/nginx-manager/internal/server"
)

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type Report struct {
	Version     string    `json:"version"`
	GeneratedAt time.Time `json:"generated_at"`
	Healthy     bool      `json:"healthy"`
	Checks      []Check   `json:"checks"`
}

func Run(ctx context.Context, version string) Report {
	report := Report{
		Version:     version,
		GeneratedAt: time.Now().UTC(),
		Healthy:     true,
		Checks:      []Check{},
	}
	add := func(name, status, detail string) {
		report.Checks = append(report.Checks, Check{Name: name, Status: status, Detail: strings.TrimSpace(detail)})
		if status == "error" {
			report.Healthy = false
		}
	}

	configPath, err := server.ConfigPath()
	if err != nil {
		add("server_config", "error", err.Error())
	} else if configured, err := server.HasPassword(); err != nil {
		add("management_password", "error", err.Error())
	} else if !configured {
		add("management_password", "error", "management password is not configured; config: "+configPath)
	} else {
		add("management_password", "ok", configPath)
	}

	pathsConfig, pathsConfigured, err := nginxmgr.LoadPathsConfig()
	if err != nil {
		add("trusted_paths", "error", err.Error())
	} else if !pathsConfigured {
		add("trusted_paths", "warn", nginxmgr.PathsConfigPath()+" is not installed; defaults/fallbacks are in use")
	} else {
		data, _ := json.Marshal(pathsConfig)
		add("trusted_paths", "ok", nginxmgr.PathsConfigPath()+" "+string(data))
	}

	runtime := nginxmgr.DetectRuntime(ctx)
	if runtime.Path == "" {
		add("nginx_runtime", "error", "nginx/openresty executable was not detected")
	} else {
		detail := runtime.Kind
		if runtime.Version != "" {
			detail += " " + runtime.Version
		}
		detail += " · " + runtime.Path
		add("nginx_runtime", "ok", detail)
	}

	manager, err := nginxmgr.New(ctx)
	if err != nil {
		add("nginx_config", "error", err.Error())
	} else {
		ok, output := manager.Status(ctx)
		if ok {
			add("nginx_config", "ok", output)
		} else {
			add("nginx_config", "error", output)
		}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	probe, err := privilege.Apply(probeCtx, privilege.ApplyRequest{Operation: privilege.OperationProbe})
	if err != nil {
		add("privileged_helper", "error", err.Error())
	} else if !probe.ConfigOK {
		add("privileged_helper", "error", "helper is reachable but nginx -t failed: "+probe.TestOutput)
	} else {
		add("privileged_helper", "ok", "sudo -n restricted helper is ready")
	}

	services := nginxmgr.DetectServiceDiagnostics(ctx, runtime)
	if !services.SystemdAvailable {
		add("systemd", "warn", "systemctl is unavailable")
	} else {
		if services.Manager.Installed {
			status := "ok"
			if !services.Manager.Active || !services.Manager.Enabled {
				status = "warn"
			}
			add("manager_service", status, formatUnit(services.Manager))
		} else {
			add("manager_service", "warn", "nginx-manager.service is not installed")
		}
		if services.Runtime.Installed {
			status := "ok"
			if !services.Runtime.Active {
				status = "warn"
			}
			add("runtime_service", status, formatUnit(services.Runtime))
		} else {
			add("runtime_service", "warn", "nginx/openresty systemd unit was not detected")
		}
	}

	certbot := nginxmgr.DetectCertbot(ctx)
	if !certbot.Available {
		add("certbot", "warn", "certbot is not installed or not available in PATH")
	} else {
		add("certbot", "ok", strings.TrimSpace(certbot.Version))
	}

	timer := nginxmgr.DetectRenewalTimer(ctx)
	if !timer.SystemdAvailable {
		add("renewal_timer", "warn", "systemd unavailable")
	} else if !timer.Installed {
		add("renewal_timer", "warn", "nginx-manager-renew.timer is not installed")
	} else {
		status := "ok"
		if !timer.Enabled || !timer.Active {
			status = "warn"
		}
		add("renewal_timer", status, fmt.Sprintf("enabled=%t active=%t", timer.Enabled, timer.Active))
	}

	return report
}

func formatUnit(unit nginxmgr.ServiceUnitStatus) string {
	return fmt.Sprintf("%s · installed=%t enabled=%t active=%t", unit.Name, unit.Installed, unit.Enabled, unit.Active)
}

func FormatText(report Report) string {
	var b strings.Builder
	if report.Healthy {
		b.WriteString("Nginx Manager doctor: OK\n")
	} else {
		b.WriteString("Nginx Manager doctor: NEEDS ATTENTION\n")
	}
	if report.Version != "" {
		b.WriteString("Version: " + report.Version + "\n")
	}
	for _, check := range report.Checks {
		marker := "OK"
		switch check.Status {
		case "warn":
			marker = "WARN"
		case "error":
			marker = "ERROR"
		}
		b.WriteString(fmt.Sprintf("[%s] %s", marker, check.Name))
		if check.Detail != "" {
			b.WriteString(": " + check.Detail)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
