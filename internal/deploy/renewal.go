package deploy

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

func RenewalServiceUnit(binaryPath string) (string, error) {
	binary := strings.TrimSpace(binaryPath)
	if binary == "" || !path.IsAbs(binary) || strings.ContainsAny(binary, " \t\r\n") {
		return "", errors.New("binary path must be an absolute path without whitespace")
	}
	unit := "[Unit]\n" +
		"Description=Nginx Manager certificate renewal\n" +
		"After=network-online.target\n" +
		"Wants=network-online.target\n\n" +
		"[Service]\n" +
		"Type=oneshot\n" +
		"ExecStart=%s privileged renew-certificates\n" +
		"TimeoutStartSec=20min\n" +
		"UMask=0077\n" +
		"Nice=10\n"
	return fmt.Sprintf(unit, binary), nil
}

func RenewalTimerUnit() string {
	return "[Unit]\n" +
		"Description=Nginx Manager certificate renewal timer\n\n" +
		"[Timer]\n" +
		"OnCalendar=*-*-* 03,15:00:00\n" +
		"RandomizedDelaySec=45m\n" +
		"Persistent=true\n" +
		"Unit=nginx-manager-renew.service\n\n" +
		"[Install]\n" +
		"WantedBy=timers.target\n"
}
