package deploy

import (
	"errors"
	"fmt"
	"net"
	"path"
	"regexp"
	"strings"
)

type SystemdOptions struct {
	ServiceUser string
	BinaryPath  string
	Listen      string
}

var serviceUserRE = regexp.MustCompile("^[a-z_][a-z0-9_-]{0,31}$")

func SystemdUnit(options SystemdOptions) (string, error) {
	user := strings.TrimSpace(options.ServiceUser)
	binary := strings.TrimSpace(options.BinaryPath)
	listen := strings.TrimSpace(options.Listen)

	if !serviceUserRE.MatchString(user) {
		return "", errors.New("service user must match [a-z_][a-z0-9_-]{0,31}")
	}
	if binary == "" || !path.IsAbs(binary) || strings.ContainsAny(binary, " \t\r\n") {
		return "", errors.New("binary path must be an absolute path without whitespace")
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		return "", errors.New("listen must be HOST:PORT")
	}
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return "", errors.New("generated systemd unit only permits loopback listen addresses")
	}

	unit := "[Unit]\n" +
		"Description=Nginx Manager API\n" +
		"After=network-online.target\n" +
		"Wants=network-online.target\n\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"User=%s\n" +
		"Group=%s\n" +
		"StateDirectory=nginx-manager\n" +
		"WorkingDirectory=/var/lib/nginx-manager\n" +
		"Environment=HOME=/var/lib/nginx-manager\n" +
		"Environment=NGINX_MANAGER_PRIVILEGED_HELPER=%s\n" +
		"ExecStart=%s serve --listen %s\n" +
		"Restart=on-failure\n" +
		"RestartSec=3\n" +
		"TimeoutStopSec=15\n" +
		"UMask=0077\n" +
		"PrivateTmp=true\n" +
		"PrivateDevices=true\n" +
		"ProtectHome=true\n" +
		"ProtectKernelTunables=true\n" +
		"ProtectKernelModules=true\n" +
		"ProtectControlGroups=true\n" +
		"LockPersonality=true\n" +
		"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6\n\n" +
		"[Install]\n" +
		"WantedBy=multi-user.target\n"

	return fmt.Sprintf(unit, user, user, binary, binary, listen), nil
}
