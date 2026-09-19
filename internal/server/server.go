package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wanstu/wails-desktop-kit/jsonstore"
	"github.com/wanstu/wails-desktop-kit/paths"
	"golang.org/x/crypto/bcrypt"
)

const (
	AppID         = "nginx-manager"
	ConfigVersion = 1
)

type Config struct {
	Version      int    `json:"version"`
	PasswordHash string `json:"password_hash"`
}

type RuntimeInfo struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

type Info struct {
	Version  string      `json:"version"`
	Hostname string      `json:"hostname"`
	Runtime  RuntimeInfo `json:"runtime"`
}

type NginxStatus struct {
	Runtime  RuntimeInfo `json:"runtime"`
	ConfigOK bool        `json:"config_ok"`
	Output   string      `json:"output"`
}

func configStore() (*jsonstore.Store[Config], error) {
	dir, err := paths.EnsureConfigDir(AppID)
	if err != nil {
		return nil, err
	}
	store := jsonstore.New(filepath.Join(dir, "server.json"), jsonstore.Options[Config]{
		Default: func() Config { return Config{Version: ConfigVersion} },
		Normalize: func(c *Config) {
			if c.Version == 0 {
				c.Version = ConfigVersion
			}
		},
		Validate: func(c Config) error {
			if c.Version != ConfigVersion {
				return fmt.Errorf("unsupported config version %d", c.Version)
			}
			return nil
		},
	})
	return store, nil
}

func ConfigPath() (string, error) {
	store, err := configStore()
	if err != nil {
		return "", err
	}
	return store.Path(), nil
}

func SetPassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	store, err := configStore()
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	cfg.PasswordHash = string(hash)
	return store.Save(cfg)
}

func HasPassword() (bool, error) {
	store, err := configStore()
	if err != nil {
		return false, err
	}
	cfg, err := store.Load()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(cfg.PasswordHash) != "", nil
}

func Serve(ctx context.Context, listen, version string) error {
	store, err := configStore()
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.PasswordHash) == "" {
		return errors.New("management password is not configured; run 'nginx-manager auth set-password' first")
	}
	if strings.TrimSpace(listen) == "" {
		listen = "127.0.0.1:8020"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": version})
	})
	mux.Handle("GET /api/v1/info", authenticate(cfg.PasswordHash, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()
		writeJSON(w, http.StatusOK, Info{Version: version, Hostname: hostname, Runtime: DetectRuntime(r.Context())})
	})))
	mux.Handle("GET /api/v1/nginx/status", authenticate(cfg.PasswordHash, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runtime := DetectRuntime(r.Context())
		status := NginxStatus{Runtime: runtime}
		if runtime.Path == "" {
			status.Output = "nginx/openresty executable not found"
			writeJSON(w, http.StatusOK, status)
			return
		}
		cmd := exec.CommandContext(r.Context(), runtime.Path, "-t")
		output, testErr := cmd.CombinedOutput()
		status.ConfigOK = testErr == nil
		status.Output = strings.TrimSpace(string(output))
		writeJSON(w, http.StatusOK, status)
	})))

	srv := &http.Server{
		Addr:              listen,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	fmt.Printf("Nginx Manager %s\n", version)
	fmt.Printf("Listen: http://%s\n", listen)
	fmt.Printf("Health: http://%s/healthz\n", listen)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func DetectRuntime(ctx context.Context) RuntimeInfo {
	for _, candidate := range []struct{ kind, name string }{
		{"nginx", "nginx"},
		{"openresty", "openresty"},
	} {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, path, "-v")
		output, _ := cmd.CombinedOutput()
		version := strings.TrimSpace(string(output))
		kind := candidate.kind
		if strings.Contains(strings.ToLower(version), "openresty") {
			kind = "openresty"
		}
		return RuntimeInfo{Kind: kind, Path: path, Version: version}
	}
	return RuntimeInfo{}
}

func authenticate(hash string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "admin" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="nginx-manager"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func IsLoopbackURLHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
