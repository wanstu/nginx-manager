package nginxmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSiteConfig(t *testing.T) {
	manager := newTestManager(t, &scriptedRunner{})
	path := filepath.Join(manager.Layout.AvailableDir, "external.conf")
	content := "server {\n    server_name external.example.com;\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	view, err := manager.ReadSiteConfig("external.conf")
	if err != nil {
		t.Fatal(err)
	}
	if view.Site.Managed {
		t.Fatalf("external config marked managed: %+v", view.Site)
	}
	if view.Content != content || view.SizeBytes != int64(len(content)) {
		t.Fatalf("view = %+v", view)
	}
}

func TestReadSiteConfigRejectsUnknownAndEscapingSymlink(t *testing.T) {
	manager := newTestManager(t, &scriptedRunner{})

	if _, err := manager.ReadSiteConfig("../nginx.conf"); err == nil {
		t.Fatal("path traversal site id was accepted")
	}

	outside := filepath.Join(t.TempDir(), "outside.conf")
	if err := os.WriteFile(outside, []byte("server { server_name outside.example.com; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(manager.Layout.AvailableDir, "escape.conf")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	if _, err := manager.ReadSiteConfig("escape.conf"); err == nil {
		t.Fatal("escaping symlink was readable")
	}
	if sites, err := manager.ListSites(); err != nil {
		t.Fatal(err)
	} else if len(sites) != 0 {
		t.Fatalf("unsafe symlink appeared in ListSites: %+v", sites)
	}
}

func TestReadSiteConfigRejectsOversizedFile(t *testing.T) {
	manager := newTestManager(t, &scriptedRunner{})
	path := filepath.Join(manager.Layout.AvailableDir, "large.conf")
	data := strings.Repeat("#", MaxSiteConfigBytes+1)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.ReadSiteConfig("large.conf"); err == nil {
		t.Fatal("oversized config was readable")
	}
	if sites, err := manager.ListSites(); err != nil {
		t.Fatal(err)
	} else if len(sites) != 0 {
		t.Fatalf("oversized config appeared in ListSites: %+v", sites)
	}
}
