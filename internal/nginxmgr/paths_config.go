package nginxmgr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const trustedPathsConfigDefault = "/etc/nginx-manager/paths.json"

var trustedPathsConfigPath = trustedPathsConfigDefault

type PathsConfig struct {
	SnapshotDir       string `json:"snapshot_dir,omitempty"`
	SnapshotRetention int    `json:"snapshot_retention,omitempty"`
	ACMEWebroot       string `json:"acme_webroot,omitempty"`
	CertLiveDir       string `json:"cert_live_dir,omitempty"`
	SiteLogDir        string `json:"site_log_dir,omitempty"`
}

func PathsConfigPath() string {
	return trustedPathsConfigPath
}

func LoadPathsConfig() (PathsConfig, bool, error) {
	path := PathsConfigPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return PathsConfig{}, false, nil
	}
	if err != nil {
		return PathsConfig{}, false, fmt.Errorf("inspect trusted paths config: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return PathsConfig{}, true, errors.New("trusted paths config must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return PathsConfig{}, true, errors.New("trusted paths config must be a regular file")
	}
	if err := validateTrustedPathsFile(path, info); err != nil {
		return PathsConfig{}, true, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return PathsConfig{}, true, fmt.Errorf("read trusted paths config: %w", err)
	}
	if len(data) > 64<<10 {
		return PathsConfig{}, true, errors.New("trusted paths config is too large")
	}

	var config PathsConfig
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return PathsConfig{}, true, fmt.Errorf("decode trusted paths config: %w", err)
	}
	if err := validatePathsConfig(config); err != nil {
		return PathsConfig{}, true, err
	}
	return config, true, nil
}

func validatePathsConfig(config PathsConfig) error {
	for name, value := range map[string]string{
		"snapshot_dir":  config.SnapshotDir,
		"acme_webroot":  config.ACMEWebroot,
		"cert_live_dir": config.CertLiveDir,
		"site_log_dir":  config.SiteLogDir,
	} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("%s must be an absolute path", name)
		}
	}
	if config.SnapshotRetention < 0 || config.SnapshotRetention > 10000 {
		return errors.New("snapshot_retention must be between 0 and 10000")
	}
	return nil
}

func PathsConfigTemplate() PathsConfig {
	return PathsConfig{
		SnapshotDir:       "/var/lib/nginx-manager/snapshots",
		SnapshotRetention: 200,
		ACMEWebroot:       "/var/lib/nginx-manager/acme-webroot",
		CertLiveDir:       "/etc/letsencrypt/live",
		SiteLogDir:        "/var/log/nginx/nginx-manager",
	}
}

func configuredPathsOrZero() PathsConfig {
	config, exists, err := LoadPathsConfig()
	if err != nil || !exists {
		return PathsConfig{}
	}
	return config
}
