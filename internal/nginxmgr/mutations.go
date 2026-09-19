package nginxmgr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wanstu/wails-desktop-kit/atomicfile"
)

var (
	ErrManagedSiteNotFound   = errors.New("managed site not found")
	ErrExternalConfiguration = errors.New("refusing to mutate external nginx configuration")
)

type Snapshot struct {
	ID        string
	CreatedAt time.Time
	Operation string
	SiteID    string
	Enabled   bool
	Content   string
}

type managedSiteState struct {
	Site       Site
	Content    []byte
	ActualPath string
}

func (m *Manager) SetSiteEnabled(ctx context.Context, siteID string, enabled bool) (Site, error) {
	mutationMu.Lock()
	defer mutationMu.Unlock()

	state, err := m.loadManagedSite(siteID)
	if err != nil {
		return Site{}, err
	}
	if state.Site.Enabled == enabled {
		return state.Site, nil
	}
	if err := m.snapshotSite("set_enabled", state); err != nil {
		return Site{}, err
	}

	rollback, err := m.changeEnabledState(state, enabled)
	if err != nil {
		return Site{}, err
	}
	if err := m.testAndReload(ctx, rollback); err != nil {
		return Site{}, err
	}

	state.Site.Enabled = enabled
	if m.Layout.Mode == "conf.d" {
		if enabled {
			state.Site.Path = filepath.Join(m.Layout.AvailableDir, state.Site.ID)
		} else {
			state.Site.Path = filepath.Join(m.Layout.AvailableDir, state.Site.ID+".disabled")
		}
	}
	return state.Site, nil
}

func (m *Manager) DeleteSite(ctx context.Context, siteID string) (Site, error) {
	mutationMu.Lock()
	defer mutationMu.Unlock()

	state, err := m.loadManagedSite(siteID)
	if err != nil {
		return Site{}, err
	}
	if err := m.snapshotSite("delete", state); err != nil {
		return Site{}, err
	}

	enabledPath := filepath.Join(m.Layout.EnabledDir, state.Site.ID)
	wasEnabled := state.Site.Enabled

	if m.Layout.Mode == "sites-enabled" && wasEnabled {
		if err := os.Remove(enabledPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Site{}, fmt.Errorf("disable site before delete: %w", err)
		}
	}
	if err := os.Remove(state.ActualPath); err != nil {
		if m.Layout.Mode == "sites-enabled" && wasEnabled {
			_ = m.createEnableSymlink(state.Site.ID)
		}
		return Site{}, fmt.Errorf("delete managed site: %w", err)
	}

	rollback := func() {
		_ = atomicfile.Write(state.ActualPath, state.Content, 0o644)
		if m.Layout.Mode == "sites-enabled" && wasEnabled {
			_ = m.createEnableSymlink(state.Site.ID)
		}
	}
	if err := m.testAndReload(ctx, rollback); err != nil {
		return Site{}, err
	}
	return state.Site, nil
}

func (m *Manager) validateManagedSiteID(siteID string) error {
	if filepath.Base(siteID) != siteID ||
		strings.ContainsAny(siteID, "/\\\x00\r\n") ||
		!strings.HasPrefix(siteID, "nginx-manager-") {
		return errors.New("invalid managed site id")
	}
	if m.Layout.Mode == "conf.d" && !strings.HasSuffix(siteID, ".conf") {
		return errors.New("invalid conf.d managed site id")
	}
	return nil
}

func (m *Manager) loadManagedSite(siteID string) (managedSiteState, error) {
	if err := m.validateManagedSiteID(siteID); err != nil {
		return managedSiteState{}, err
	}

	candidates := []string{filepath.Join(m.Layout.AvailableDir, siteID)}
	if m.Layout.Mode == "conf.d" {
		candidates = append(candidates, filepath.Join(m.Layout.AvailableDir, siteID+".disabled"))
	}

	var actual string
	var data []byte
	for _, candidate := range candidates {
		value, err := os.ReadFile(candidate)
		if err == nil {
			actual = candidate
			data = value
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return managedSiteState{}, fmt.Errorf("read managed site: %w", err)
		}
	}
	if actual == "" {
		if m.Layout.Mode == "sites-enabled" {
			link := filepath.Join(m.Layout.EnabledDir, siteID)
			if _, err := os.Lstat(link); err == nil {
				return managedSiteState{}, ErrExternalConfiguration
			} else if !errors.Is(err, os.ErrNotExist) {
				return managedSiteState{}, fmt.Errorf("inspect enabled site link: %w", err)
			}
		}
		return managedSiteState{}, ErrManagedSiteNotFound
	}
	if !strings.Contains(string(data), managedMarker) {
		return managedSiteState{}, ErrExternalConfiguration
	}

	site := Site{
		ID:      siteID,
		Path:    actual,
		Enabled: m.siteEnabled(filepath.Base(actual)),
		Managed: true,
	}
	text := string(data)
	if match := serverNameRE.FindStringSubmatch(text); len(match) == 2 {
		fields := strings.Fields(match[1])
		if len(fields) > 0 {
			site.ServerName = fields[0]
		}
	}
	if match := proxyPassRE.FindStringSubmatch(text); len(match) == 2 {
		site.ProxyPass = strings.TrimSpace(match[1])
	}

	return managedSiteState{Site: site, Content: data, ActualPath: actual}, nil
}

func (m *Manager) changeEnabledState(state managedSiteState, enabled bool) (func(), error) {
	if m.Layout.Mode == "sites-enabled" {
		link := filepath.Join(m.Layout.EnabledDir, state.Site.ID)
		if enabled {
			if err := m.createEnableSymlink(state.Site.ID); err != nil {
				return nil, err
			}
			return func() { _ = os.Remove(link) }, nil
		}
		if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("disable site: %w", err)
		}
		return func() { _ = m.createEnableSymlink(state.Site.ID) }, nil
	}

	enabledPath := filepath.Join(m.Layout.AvailableDir, state.Site.ID)
	disabledPath := enabledPath + ".disabled"
	if enabled {
		if state.ActualPath == enabledPath {
			return func() {}, nil
		}
		if err := os.Rename(disabledPath, enabledPath); err != nil {
			return nil, fmt.Errorf("enable conf.d site: %w", err)
		}
		return func() { _ = os.Rename(enabledPath, disabledPath) }, nil
	}
	if state.ActualPath == disabledPath {
		return func() {}, nil
	}
	if err := os.Rename(enabledPath, disabledPath); err != nil {
		return nil, fmt.Errorf("disable conf.d site: %w", err)
	}
	return func() { _ = os.Rename(disabledPath, enabledPath) }, nil
}

func (m *Manager) createEnableSymlink(siteID string) error {
	target := filepath.Join(m.Layout.AvailableDir, siteID)
	link := filepath.Join(m.Layout.EnabledDir, siteID)
	rel, err := filepath.Rel(m.Layout.EnabledDir, target)
	if err != nil {
		return fmt.Errorf("resolve enable symlink: %w", err)
	}
	if err := os.Symlink(rel, link); err != nil {
		return fmt.Errorf("enable site: %w", err)
	}
	return nil
}

func (m *Manager) testAndReload(ctx context.Context, rollback func()) error {
	output, err := m.Runner.Run(ctx, m.Runtime.Path, "-t")
	if err != nil {
		rollback()
		return fmt.Errorf("live nginx -t failed; rolled back: %s", output)
	}
	reloadOutput, err := m.Runner.Run(ctx, m.Runtime.Path, "-s", "reload")
	if err == nil {
		return nil
	}

	rollback()
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = m.Runner.Run(rollbackCtx, m.Runtime.Path, "-t")
	_, _ = m.Runner.Run(rollbackCtx, m.Runtime.Path, "-s", "reload")
	return fmt.Errorf("nginx reload failed; rolled back: %s", reloadOutput)
}

func snapshotRoot() string {
	root := strings.TrimSpace(os.Getenv("NGINX_MANAGER_SNAPSHOT_DIR"))
	if root == "" {
		root = "/var/lib/nginx-manager/snapshots"
	}
	return root
}

func (m *Manager) snapshotSite(operation string, state managedSiteState) error {
	root := snapshotRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}

	now := time.Now().UTC()
	snapshot := Snapshot{
		ID:        fmt.Sprintf("%d-%s", now.UnixNano(), state.Site.ID),
		CreatedAt: now,
		Operation: operation,
		SiteID:    state.Site.ID,
		Enabled:   state.Site.Enabled,
		Content:   string(state.Content),
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode site snapshot: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(root, snapshot.ID+".json")
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("write site snapshot: %w", err)
	}
	return nil
}
