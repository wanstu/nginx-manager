package nginxmgr

import (
	"os"
	"testing"
)

func TestSnapshotRetentionPrunesOldFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NGINX_MANAGER_SNAPSHOT_DIR", root)
	t.Setenv("NGINX_MANAGER_SNAPSHOT_RETENTION", "3")

	manager := newTestManager(t, &scriptedRunner{})
	state := managedSiteState{
		Site: Site{
			ID:      "nginx-manager-retention.example.com.conf",
			Managed: true,
			Enabled: true,
		},
		Content:    []byte(managedMarker + "\nserver { server_name retention.example.com; }\n"),
		ActualPath: "unused",
	}
	for i := 0; i < 5; i++ {
		if err := manager.snapshotSite("retention_test", state); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("snapshot count = %d, want 3", len(entries))
	}
}

func TestSnapshotRetentionFallsBackForInvalidValue(t *testing.T) {
	t.Setenv("NGINX_MANAGER_SNAPSHOT_RETENTION", "invalid")
	if got := snapshotRetention(); got != defaultSnapshotRetention {
		t.Fatalf("snapshotRetention() = %d, want %d", got, defaultSnapshotRetention)
	}
	t.Setenv("NGINX_MANAGER_SNAPSHOT_RETENTION", "0")
	if got := snapshotRetention(); got != defaultSnapshotRetention {
		t.Fatalf("snapshotRetention() = %d, want %d", got, defaultSnapshotRetention)
	}
}
