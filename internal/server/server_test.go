package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordRequiredAndStoredAsHash(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if ok, err := HasPassword(); err != nil || ok {
		t.Fatalf("HasPassword() = %v, %v; want false, nil", ok, err)
	}
	if err := SetPassword("short"); err == nil {
		t.Fatal("SetPassword(short) succeeded")
	}
	const password = "correct horse battery staple"
	if err := SetPassword(password); err != nil {
		t.Fatal(err)
	}
	ok, err := HasPassword()
	if err != nil || !ok {
		t.Fatalf("HasPassword() = %v, %v; want true, nil", ok, err)
	}
	store, err := configStore()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PasswordHash == password || strings.Contains(cfg.PasswordHash, password) {
		t.Fatal("password was stored in plaintext")
	}
}

func TestAuthenticate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SetPassword("password-123"); err != nil {
		t.Fatal(err)
	}
	store, _ := configStore()
	cfg, _ := store.Load()

	handler := authenticate(cfg.PasswordHash, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "password-123")
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, req)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d", authorized.Code)
	}
}

func TestServeRefusesMissingPassword(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	err := Serve(context.Background(), "127.0.0.1:0", "test")
	if err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("Serve() error = %v; want password error", err)
	}
}

func TestCapabilityFeaturesStable(t *testing.T) {
	if APIVersion < 1 {
		t.Fatalf("APIVersion = %d", APIVersion)
	}
	seen := map[string]bool{}
	required := map[string]bool{
		"deployment_plan":     false,
		"sites_read":          false,
		"sites_write":         false,
		"https_acme":          false,
		"logs":                false,
		"diagnostics":         false,
		"safe_reload":         false,
		"traffic_window":      false,
		"trusted_paths":       false,
		"site_config_preview": false,
		"site_logs":           false,
	}
	for i, feature := range capabilityFeatures {
		if feature == "" {
			t.Fatal("empty capability")
		}
		if seen[feature] {
			t.Fatalf("duplicate capability %q", feature)
		}
		seen[feature] = true
		if _, ok := required[feature]; ok {
			required[feature] = true
		}
		if i > 0 && capabilityFeatures[i-1] > feature {
			t.Fatalf("capability list is not sorted: %q before %q", capabilityFeatures[i-1], feature)
		}
	}
	for feature, found := range required {
		if !found {
			t.Fatalf("required capability %q missing", feature)
		}
	}
}
