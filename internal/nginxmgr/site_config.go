package nginxmgr

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const MaxSiteConfigBytes = 256 << 10

type SiteConfigView struct {
	Site      Site   `json:"site"`
	Content   string `json:"content"`
	SizeBytes int64  `json:"size_bytes"`
}

func (m *Manager) ReadSiteConfig(siteID string) (SiteConfigView, error) {
	sites, err := m.ListSites()
	if err != nil {
		return SiteConfigView{}, err
	}

	var selected *Site
	for i := range sites {
		if sites[i].ID == siteID {
			selected = &sites[i]
			break
		}
	}
	if selected == nil {
		return SiteConfigView{}, errors.New("site not found")
	}

	data, err := m.readSiteConfigFile(selected.Path)
	if err != nil {
		return SiteConfigView{}, err
	}
	return SiteConfigView{
		Site:      *selected,
		Content:   string(data),
		SizeBytes: int64(len(data)),
	}, nil
}

func (m *Manager) readSiteConfigFile(path string) ([]byte, error) {
	resolved, info, err := m.resolveSiteConfigPath(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxSiteConfigBytes {
		return nil, fmt.Errorf("site config exceeds %d bytes", MaxSiteConfigBytes)
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open site config: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxSiteConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read site config: %w", err)
	}
	if len(data) > MaxSiteConfigBytes {
		return nil, fmt.Errorf("site config exceeds %d bytes", MaxSiteConfigBytes)
	}
	return data, nil
}

func (m *Manager) resolveSiteConfigPath(path string) (string, os.FileInfo, error) {
	availableRoot, err := filepath.Abs(m.Layout.AvailableDir)
	if err != nil {
		return "", nil, fmt.Errorf("resolve sites directory: %w", err)
	}
	availableRoot, err = filepath.EvalSymlinks(availableRoot)
	if err != nil {
		return "", nil, fmt.Errorf("resolve sites directory symlinks: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, fmt.Errorf("resolve site config path: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("resolve site config absolute path: %w", err)
	}
	if !pathWithinRoots(resolved, []string{availableRoot}) {
		return "", nil, errors.New("site config resolves outside the allowed sites directory")
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("stat site config: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, errors.New("site config is not a regular file")
	}
	return resolved, info, nil
}
