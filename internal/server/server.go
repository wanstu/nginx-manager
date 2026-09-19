package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wanstu/nginx-manager/internal/nginxmgr"
	"github.com/wanstu/nginx-manager/internal/privilege"
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

type Info struct {
	Version  string               `json:"version"`
	Hostname string               `json:"hostname"`
	Runtime  nginxmgr.RuntimeInfo `json:"runtime"`
}

type NginxStatus struct {
	Runtime  nginxmgr.RuntimeInfo `json:"runtime"`
	Layout   nginxmgr.Layout      `json:"layout"`
	ConfigOK bool                 `json:"config_ok"`
	Output   string               `json:"output"`
}

type PrivilegeStatus struct {
	Ready      bool                  `json:"ready"`
	Error      string                `json:"error,omitempty"`
	Runtime    *nginxmgr.RuntimeInfo `json:"runtime,omitempty"`
	Layout     *nginxmgr.Layout      `json:"layout,omitempty"`
	ConfigOK   bool                  `json:"config_ok"`
	TestOutput string                `json:"test_output,omitempty"`
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
	if err := privilege.RefuseRootServer(); err != nil {
		return err
	}
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

	protected := func(pattern string, handler http.HandlerFunc) {
		mux.Handle(pattern, authenticate(cfg.PasswordHash, handler))
	}

	protected("GET /api/v1/info", func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()
		writeJSON(w, http.StatusOK, Info{Version: version, Hostname: hostname, Runtime: nginxmgr.DetectRuntime(r.Context())})
	})

	protected("GET /api/v1/nginx/status", func(w http.ResponseWriter, r *http.Request) {
		manager, err := nginxmgr.New(r.Context())
		if err != nil {
			writeJSON(w, http.StatusOK, NginxStatus{Runtime: nginxmgr.DetectRuntime(r.Context()), Output: err.Error()})
			return
		}
		ok, output := manager.Status(r.Context())
		writeJSON(w, http.StatusOK, NginxStatus{Runtime: manager.Runtime, Layout: manager.Layout, ConfigOK: ok, Output: output})
	})

	protected("GET /api/v1/privilege/status", func(w http.ResponseWriter, r *http.Request) {
		response, err := privilege.Apply(r.Context(), privilege.ApplyRequest{Operation: privilege.OperationProbe})
		if err != nil {
			writeJSON(w, http.StatusOK, PrivilegeStatus{Ready: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, PrivilegeStatus{
			Ready:      true,
			Runtime:    response.Runtime,
			Layout:     response.Layout,
			ConfigOK:   response.ConfigOK,
			TestOutput: response.TestOutput,
		})
	})

	protected("GET /api/v1/sites", func(w http.ResponseWriter, r *http.Request) {
		manager, err := nginxmgr.New(r.Context())
		if err != nil {
			writeAPIError(w, http.StatusServiceUnavailable, err)
			return
		}
		sites, err := manager.ListSites()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sites": sites, "layout": manager.Layout})
	})

	protected("POST /api/v1/sites/reverse-proxy", func(w http.ResponseWriter, r *http.Request) {
		var req nginxmgr.ReverseProxyRequest
		if err := decodeJSONRequest(w, r, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		response, err := privilege.Apply(r.Context(), privilege.ApplyRequest{
			Operation: privilege.OperationCreateReverseProxy,
			Create:    &req,
		})
		if err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		if response.Apply == nil {
			writeAPIError(w, http.StatusInternalServerError, errors.New("privileged helper returned no apply result"))
			return
		}
		writeJSON(w, http.StatusCreated, response.Apply)
	})

	protected("PUT /api/v1/sites/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodeJSONRequest(w, r, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		response, err := privilege.Apply(r.Context(), privilege.ApplyRequest{
			Operation: privilege.OperationSetSiteEnabled,
			SiteID:    r.PathValue("id"),
			Enabled:   req.Enabled,
		})
		if err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		if response.Site == nil {
			writeAPIError(w, http.StatusInternalServerError, errors.New("privileged helper returned no site result"))
			return
		}
		writeJSON(w, http.StatusOK, response.Site)
	})

	protected("DELETE /api/v1/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		response, err := privilege.Apply(r.Context(), privilege.ApplyRequest{
			Operation: privilege.OperationDeleteSite,
			SiteID:    r.PathValue("id"),
		})
		if err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		if response.Site == nil {
			writeAPIError(w, http.StatusInternalServerError, errors.New("privileged helper returned no site result"))
			return
		}
		writeJSON(w, http.StatusOK, response.Site)
	})

	protected("GET /api/v1/snapshots", func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 100 {
				writeAPIError(w, http.StatusBadRequest, errors.New("limit must be between 1 and 100"))
				return
			}
			limit = value
		}
		response, err := privilege.Apply(r.Context(), privilege.ApplyRequest{
			Operation: privilege.OperationListSnapshots,
			Limit:     limit,
		})
		if err != nil {
			writeAPIError(w, http.StatusServiceUnavailable, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"snapshots": response.Snapshots})
	})

	protected("POST /api/v1/snapshots/{id}/restore", func(w http.ResponseWriter, r *http.Request) {
		response, err := privilege.Apply(r.Context(), privilege.ApplyRequest{
			Operation:  privilege.OperationRestoreSnapshot,
			SnapshotID: r.PathValue("id"),
		})
		if err != nil {
			writeAPIError(w, http.StatusConflict, err)
			return
		}
		if response.Site == nil {
			writeAPIError(w, http.StatusInternalServerError, errors.New("privileged helper returned no site result"))
			return
		}
		writeJSON(w, http.StatusOK, response.Site)
	})

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

func authenticate(hash string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "admin" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"nginx-manager\"")
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

func decodeJSONRequest(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid request: multiple JSON values")
		}
		return fmt.Errorf("invalid request: %w", err)
	}
	return nil
}

func writeAPIError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
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
