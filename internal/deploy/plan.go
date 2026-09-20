package deploy

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

type PlanInput struct {
	Executable                   string
	ServiceUser                  string
	CurrentUser                  string
	ManagementPasswordConfigured bool
	PrivilegeReady               bool
	PathsConfigured              bool
	NginxConfigOK                bool
	NginxTestOutput              string
	SystemdAvailable             bool
	ManagerInstalled             bool
	ManagerEnabled               bool
	ManagerActive                bool
	CertbotAvailable             bool
	RenewalInstalled             bool
	RenewalEnabled               bool
	RenewalActive                bool
}

type PlanStep struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Required bool     `json:"required"`
	Complete bool     `json:"complete"`
	Detail   string   `json:"detail,omitempty"`
	Commands []string `json:"commands"`
}

type Plan struct {
	Ready         bool       `json:"ready"`
	Completed     int        `json:"completed"`
	Total         int        `json:"total"`
	RequiredReady int        `json:"required_ready"`
	RequiredTotal int        `json:"required_total"`
	Steps         []PlanStep `json:"steps"`
	VerifyCommand string     `json:"verify_command"`
}

func BuildPlan(input PlanInput) (Plan, error) {
	executable := strings.TrimSpace(input.Executable)
	if executable == "" || !path.IsAbs(executable) || strings.ContainsAny(executable, "\x00\r\n") {
		return Plan{}, errors.New("executable path must be absolute")
	}
	serviceUser := strings.TrimSpace(input.ServiceUser)
	if serviceUser == "" {
		serviceUser = "nginx-manager"
	}
	if !serviceUserRE.MatchString(serviceUser) {
		return Plan{}, errors.New("service user must match [a-z_][a-z0-9_-]{0,31}")
	}

	q := shellQuote(executable)
	user := shellQuote(serviceUser)
	currentUser := accountName(input.CurrentUser)
	serviceUserOK := currentUser == serviceUser
	passwordReady := serviceUserOK && input.ManagementPasswordConfigured
	privilegeReady := serviceUserOK && input.PrivilegeReady

	serviceDetail := "推荐使用专用低权限用户和 loopback 监听；远程访问由 HTTPS 反代或 SSH Tunnel 提供。"
	serviceCommands := []string{
		q + " service systemd --service-user " + user + " --binary " + q + " --listen 127.0.0.1:8020 | sudo tee /etc/systemd/system/nginx-manager.service >/dev/null",
		"sudo systemctl daemon-reload",
		"sudo systemctl enable --now nginx-manager.service",
		"sudo systemctl status nginx-manager.service --no-pager",
	}
	renewalDetail := "Certbot 可用后建议启用；每天两次检查并带随机延迟。"
	renewalCommands := []string{
		q + " service renewal-service --binary " + q + " | sudo tee /etc/systemd/system/nginx-manager-renew.service >/dev/null",
		q + " service renewal-timer | sudo tee /etc/systemd/system/nginx-manager-renew.timer >/dev/null",
		"sudo systemctl daemon-reload",
		"sudo systemctl enable --now nginx-manager-renew.timer",
	}
	if !input.SystemdAvailable {
		serviceDetail = "当前主机未检测到 systemd；可跳过此可选步骤并使用现有进程管理器托管 CLI。"
		serviceCommands = nil
		renewalDetail = "当前主机未检测到 systemd；内置 renewal timer 不适用，可使用其他调度器执行证书续期。"
		renewalCommands = nil
	}

	serviceUserDetail := "当前 CLI 未运行在专用服务用户下。"
	if currentUser == "" {
		serviceUserDetail = "无法确认当前 CLI 运行用户；生产服务应使用 " + serviceUser + "。"
	} else if serviceUserOK {
		serviceUserDetail = "当前 CLI 运行用户：" + currentUser
	} else {
		serviceUserDetail = "当前 CLI 运行用户：" + currentUser + "；生产服务应切换到 " + serviceUser + "。"
	}

	passwordDetail := "需要以 " + serviceUser + " 用户重新设置管理密码，确保配置目录与 systemd 服务一致。"
	if passwordReady {
		passwordDetail = "管理密码属于 " + serviceUser + " 服务用户配置。"
	}

	privilegeDetail := "为 " + serviceUser + " 安装仅允许 privileged apply 的最小 sudoers 规则。"
	if privilegeReady {
		privilegeDetail = "受限 root helper 与 nginx -t 已就绪。"
	}

	steps := []PlanStep{
		{
			ID:       "service_user",
			Title:    "使用专用低权限服务用户",
			Required: true,
			Complete: serviceUserOK,
			Detail:   serviceUserDetail,
			Commands: []string{
				"id -u " + serviceUser + " >/dev/null 2>&1 || sudo useradd --system --user-group --home-dir /var/lib/nginx-manager --create-home --shell /usr/sbin/nologin " + serviceUser,
				"sudo install -d -o " + serviceUser + " -g " + serviceUser + " -m 0750 /var/lib/nginx-manager",
			},
		},
		{
			ID:       "management_password",
			Title:    "设置服务用户管理密码",
			Required: true,
			Complete: passwordReady,
			Detail:   passwordDetail,
			Commands: []string{
				"sudo -u " + user + " -H " + q + " auth set-password",
			},
		},
		{
			ID:       "nginx",
			Title:    "Nginx / OpenResty 配置可用",
			Required: true,
			Complete: input.NginxConfigOK,
			Detail:   strings.TrimSpace(input.NginxTestOutput),
			Commands: []string{q + " doctor"},
		},
		{
			ID:       "privilege",
			Title:    "安装受限 root helper 权限",
			Required: true,
			Complete: privilegeReady,
			Detail:   privilegeDetail,
			Commands: []string{
				q + " privileged sudoers --service-user " + user + " --helper " + q + " | sudo tee /tmp/nginx-manager.sudoers >/dev/null",
				"sudo visudo -cf /tmp/nginx-manager.sudoers",
				"sudo install -o root -g root -m 0440 /tmp/nginx-manager.sudoers /etc/sudoers.d/nginx-manager",
				"sudo rm -f /tmp/nginx-manager.sudoers",
			},
		},
		{
			ID:       "trusted_paths",
			Title:    "安装可信路径配置",
			Required: true,
			Complete: input.PathsConfigured,
			Detail:   "固定快照、ACME、证书和站点日志目录，避免由远端请求传入任意路径。",
			Commands: []string{
				"sudo install -d -o root -g root -m 0755 /etc/nginx-manager",
				q + " config paths-template | sudo tee /etc/nginx-manager/paths.json >/dev/null",
				"sudo chown root:root /etc/nginx-manager/paths.json",
				"sudo chmod 0644 /etc/nginx-manager/paths.json",
			},
		},
		{
			ID:       "service",
			Title:    "交给 systemd 托管 API",
			Required: false,
			Complete: input.SystemdAvailable && input.ManagerInstalled && input.ManagerEnabled && input.ManagerActive,
			Detail:   serviceDetail,
			Commands: serviceCommands,
		},
		{
			ID:       "certbot",
			Title:    "准备 Certbot",
			Required: false,
			Complete: input.CertbotAvailable,
			Detail:   "仅 HTTPS / ACME 功能需要；下面命令是 Debian / Ubuntu 示例，其他发行版请使用对应包管理器。",
			Commands: []string{
				"# Debian / Ubuntu",
				"sudo apt-get update",
				"sudo apt-get install -y certbot",
			},
		},
		{
			ID:       "renewal",
			Title:    "启用自动证书续期",
			Required: false,
			Complete: input.CertbotAvailable && input.SystemdAvailable && input.RenewalInstalled && input.RenewalEnabled && input.RenewalActive,
			Detail:   renewalDetail,
			Commands: renewalCommands,
		},
	}

	plan := Plan{Steps: steps}
	for _, step := range steps {
		plan.Total++
		if step.Complete {
			plan.Completed++
		}
		if step.Required {
			plan.RequiredTotal++
			if step.Complete {
				plan.RequiredReady++
			}
		}
	}
	plan.Ready = plan.RequiredReady == plan.RequiredTotal
	plan.VerifyCommand = fmt.Sprintf("sudo -u %s -H %s doctor", user, q)
	return plan, nil
}

func accountName(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.LastIndexAny(value, "/\\"); index >= 0 {
		value = value[index+1:]
	}
	return value
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
