package nginxmgr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func useTestPathsConfig(t *testing.T, path string) {
	t.Helper()
	previous := trustedPathsConfigPath
	trustedPathsConfigPath = path
	t.Cleanup(func() { trustedPathsConfigPath = previous })
}

func TestLoadPathsConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "paths.json")
	useTestPathsConfig(t, path)

	want := PathsConfig{
		SnapshotDir:       filepath.Join(root, "snapshots"),
		SnapshotRetention: 123,
		ACMEWebroot:       filepath.Join(root, "acme"),
		CertLiveDir:       filepath.Join(root, "certs"),
		SiteLogDir:        filepath.Join(root, "logs"),
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, exists, err := LoadPathsConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !exists || got != want {
		t.Fatalf("config = %+v, exists=%v; want %+v", got, exists, want)
	}
}

func TestListCertificatesRejectsInvalidPathsConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "paths.json")
	useTestPathsConfig(t, path)

	bad, _ := json.Marshal(PathsConfig{CertLiveDir: "relative-certs"})
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ListCertificates(); err == nil {
		t.Fatal("ListCertificates accepted invalid trusted paths config")
	}
}

func TestLoadPathsConfigRejectsWritableAndRelativePaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "paths.json")
	useTestPathsConfig(t, path)

	badRelative, _ := json.Marshal(PathsConfig{SiteLogDir: "relative"})
	if err := os.WriteFile(path, badRelative, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadPathsConfig(); err == nil {
		t.Fatal("relative site_log_dir was accepted")
	}

	good, _ := json.Marshal(PathsConfig{SiteLogDir: filepath.Join(root, "logs")})
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadPathsConfig()
	if runtime.GOOS == "linux" && err == nil {
		t.Fatal("group/world writable config was accepted")
	}
}
