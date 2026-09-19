package nginxmgr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wanstu/wails-desktop-kit/atomicfile"
)

type SnapshotMeta struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Operation string    `json:"operation"`
	SiteID    string    `json:"site_id"`
	Enabled   bool      `json:"enabled"`
}

func (m *Manager) ListSnapshots(limit int) ([]SnapshotMeta, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	root := snapshotRoot()
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []SnapshotMeta{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshot directory: %w", err)
	}

	items := make([]SnapshotMeta, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		snapshot, err := loadSnapshot(id)
		if err != nil {
			return nil, err
		}
		items = append(items, SnapshotMeta{
			ID:        snapshot.ID,
			CreatedAt: snapshot.CreatedAt,
			Operation: snapshot.Operation,
			SiteID:    snapshot.SiteID,
			Enabled:   snapshot.Enabled,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (m *Manager) RestoreSnapshot(ctx context.Context, snapshotID string) (Site, error) {
	mutationMu.Lock()
	defer mutationMu.Unlock()

	snapshot, err := loadSnapshot(snapshotID)
	if err != nil {
		return Site{}, err
	}
	if err := m.validateManagedSiteID(snapshot.SiteID); err != nil {
		return Site{}, fmt.Errorf("invalid snapshot site: %w", err)
	}
	if !strings.Contains(snapshot.Content, managedMarker) {
		return Site{}, errors.New("snapshot does not contain a managed nginx site")
	}

	content := []byte(snapshot.Content)
	if err := m.validateCandidate(ctx, snapshot.SiteID, content); err != nil {
		return Site{}, fmt.Errorf("snapshot candidate validation failed: %w", err)
	}

	current, currentErr := m.loadManagedSite(snapshot.SiteID)
	currentExists := currentErr == nil
	if currentErr != nil && !errors.Is(currentErr, ErrManagedSiteNotFound) {
		return Site{}, currentErr
	}
	if currentExists {
		if err := m.snapshotSite("restore_before", current); err != nil {
			return Site{}, err
		}
	}

	if err := m.removeManagedSiteFiles(snapshot.SiteID); err != nil {
		return Site{}, err
	}

	restoreCurrent := func() {
		_ = m.removeManagedSiteFiles(snapshot.SiteID)
		if currentExists {
			_ = m.restoreManagedState(current)
		}
	}

	actualPath := filepath.Join(m.Layout.AvailableDir, snapshot.SiteID)
	if m.Layout.Mode == "conf.d" && !snapshot.Enabled {
		actualPath += ".disabled"
	}
	if err := atomicfile.Write(actualPath, content, 0o644); err != nil {
		restoreCurrent()
		return Site{}, fmt.Errorf("restore snapshot file: %w", err)
	}
	if m.Layout.Mode == "sites-enabled" && snapshot.Enabled {
		if err := m.createEnableSymlink(snapshot.SiteID); err != nil {
			restoreCurrent()
			return Site{}, err
		}
	}

	if err := m.testAndReload(ctx, restoreCurrent); err != nil {
		return Site{}, err
	}

	site := parseManagedSite(snapshot.SiteID, actualPath, snapshot.Enabled, content)
	return site, nil
}

func loadSnapshot(snapshotID string) (Snapshot, error) {
	if filepath.Base(snapshotID) != snapshotID ||
		strings.ContainsAny(snapshotID, "/\\\x00\r\n") ||
		!strings.Contains(snapshotID, "-nginx-manager-") {
		return Snapshot{}, errors.New("invalid snapshot id")
	}

	path := filepath.Join(snapshotRoot(), snapshotID+".json")
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, errors.New("snapshot not found")
		}
		return Snapshot{}, fmt.Errorf("inspect snapshot: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return Snapshot{}, errors.New("invalid snapshot file")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read snapshot: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode snapshot %s: %w", snapshotID, err)
	}
	if snapshot.ID != snapshotID || snapshot.SiteID == "" || snapshot.CreatedAt.IsZero() {
		return Snapshot{}, errors.New("snapshot metadata mismatch")
	}
	return snapshot, nil
}

func (m *Manager) removeManagedSiteFiles(siteID string) error {
	if err := m.validateManagedSiteID(siteID); err != nil {
		return err
	}

	if m.Layout.Mode == "sites-enabled" {
		link := filepath.Join(m.Layout.EnabledDir, siteID)
		if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove enabled site link: %w", err)
		}
		target := filepath.Join(m.Layout.AvailableDir, siteID)
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove managed site file: %w", err)
		}
		return nil
	}

	for _, path := range []string{
		filepath.Join(m.Layout.AvailableDir, siteID),
		filepath.Join(m.Layout.AvailableDir, siteID+".disabled"),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove managed site file: %w", err)
		}
	}
	return nil
}

func (m *Manager) restoreManagedState(state managedSiteState) error {
	if err := atomicfile.Write(state.ActualPath, state.Content, 0o644); err != nil {
		return err
	}
	if m.Layout.Mode == "sites-enabled" && state.Site.Enabled {
		return m.createEnableSymlink(state.Site.ID)
	}
	return nil
}

func parseManagedSite(siteID, actualPath string, enabled bool, content []byte) Site {
	text := string(content)
	site := Site{
		ID:      siteID,
		Path:    actualPath,
		Enabled: enabled,
		Managed: true,
	}
	if match := serverNameRE.FindStringSubmatch(text); len(match) == 2 {
		fields := strings.Fields(match[1])
		if len(fields) > 0 {
			site.ServerName = fields[0]
		}
	}
	if match := proxyPassRE.FindStringSubmatch(text); len(match) == 2 {
		site.ProxyPass = strings.TrimSpace(match[1])
	}
	site.WebSocket = websocketEnabled(text)
	applyTLSFields(&site, text)
	return site
}
