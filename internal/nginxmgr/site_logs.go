package nginxmgr

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const managedSiteLogRootDefault = "/var/log/nginx/nginx-manager"

func managedSiteLogRoot() (string, error) {
	root := strings.TrimSpace(configuredPathsOrZero().SiteLogDir)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("NGINX_MANAGER_SITE_LOG_DIR"))
	}
	if root == "" {
		root = managedSiteLogRootDefault
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("NGINX_MANAGER_SITE_LOG_DIR must be an absolute path")
	}
	return filepath.Clean(root), nil
}

func managedSiteLogPaths(serverName string) (string, string, error) {
	serverName = strings.TrimSpace(strings.ToLower(serverName))
	if serverName == "" {
		return "", "", errors.New("server_name is required for managed site logs")
	}
	safeName := serverName
	if safeName == "_" {
		safeName = "default"
	}
	if strings.ContainsAny(safeName, "/\\\x00\r\n") {
		return "", "", errors.New("invalid server_name for managed site logs")
	}
	root, err := managedSiteLogRoot()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(root, safeName+".access.log"), filepath.Join(root, safeName+".error.log"), nil
}

func ensureManagedSiteLogDir() error {
	root, err := managedSiteLogRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create managed site log directory: %w", err)
	}
	return nil
}

func withManagedSiteLogs(options ProxyOptions, serverName string) (ProxyOptions, error) {
	accessLog, errorLog, err := managedSiteLogPaths(serverName)
	if err != nil {
		return ProxyOptions{}, err
	}
	options.AccessLog = accessLog
	options.ErrorLog = errorLog
	return options, nil
}
