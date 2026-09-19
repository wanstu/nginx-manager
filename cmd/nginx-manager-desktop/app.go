package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/wanstu/wails-desktop-kit/jsonstore"
	"github.com/wanstu/wails-desktop-kit/paths"
	"github.com/wanstu/wails-desktop-kit/secureconfig"
)

const desktopAppID = "nginx-manager-desktop"

type Connection struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	CredentialRef string `json:"-"`
}

type connectionRecord struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	CredentialRef string `json:"credential_ref"`
}

type Settings struct {
	Version     int                `json:"version"`
	SelectedID  string             `json:"selected_id,omitempty"`
	Connections []connectionRecord `json:"connections"`
}

type SaveConnectionRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Password string `json:"password"`
}

type TestResult struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message"`
	Hostname string `json:"hostname,omitempty"`
	Runtime  string `json:"runtime,omitempty"`
	Version  string `json:"version,omitempty"`
}

type RemoteSite struct {
	ID         string `json:"id"`
	ServerName string `json:"server_name"`
	ProxyPass  string `json:"proxy_pass,omitempty"`
	Enabled    bool   `json:"enabled"`
	Managed    bool   `json:"managed"`
	Path       string `json:"path"`
}

type RemoteLayout struct {
	MainConfig   string `json:"main_config"`
	AvailableDir string `json:"available_dir"`
	EnabledDir   string `json:"enabled_dir"`
	Mode         string `json:"mode"`
}

type SiteListResult struct {
	Sites  []RemoteSite `json:"sites"`
	Layout RemoteLayout `json:"layout"`
}

type CreateReverseProxyRequest struct {
	ServerName string `json:"server_name"`
	Upstream   string `json:"upstream"`
	WebSocket  bool   `json:"websocket"`
}

type CreateReverseProxyResult struct {
	Site       RemoteSite `json:"site"`
	TestOutput string     `json:"test_output"`
}

type App struct {
	settings *jsonstore.Store[Settings]
	secure   *secureconfig.Store
	client   *http.Client
}

func NewApp() (*App, error) {
	dir, err := paths.EnsureConfigDir(desktopAppID)
	if err != nil {
		return nil, err
	}
	secure, err := secureconfig.New(desktopAppID)
	if err != nil {
		return nil, err
	}
	store := jsonstore.New(filepath.Join(dir, "settings.json"), jsonstore.Options[Settings]{
		Default: func() Settings { return Settings{Version: 1, Connections: []connectionRecord{}} },
		Normalize: func(s *Settings) {
			if s.Version == 0 {
				s.Version = 1
			}
			if s.Connections == nil {
				s.Connections = []connectionRecord{}
			}
		},
		Validate: func(s Settings) error {
			if s.Version != 1 {
				return fmt.Errorf("unsupported settings version %d", s.Version)
			}
			return nil
		},
	})
	return &App{
		settings: store,
		secure:   secure,
		client:   &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func (a *App) ListConnections() ([]Connection, error) {
	cfg, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(cfg.Connections))
	for _, c := range cfg.Connections {
		out = append(out, Connection{ID: c.ID, Name: c.Name, URL: c.URL})
	}
	return out, nil
}

func (a *App) GetSelectedID() (string, error) {
	cfg, err := a.settings.Load()
	if err != nil {
		return "", err
	}
	return cfg.SelectedID, nil
}

func (a *App) SaveConnection(req SaveConnectionRequest) (Connection, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Connection{}, errors.New("connection name is required")
	}
	endpoint, err := normalizeEndpoint(req.URL)
	if err != nil {
		return Connection{}, err
	}

	cfg, err := a.settings.Load()
	if err != nil {
		return Connection{}, err
	}
	id := strings.TrimSpace(req.ID)
	index := -1
	for i, c := range cfg.Connections {
		if c.ID == id && id != "" {
			index = i
			break
		}
	}
	if id == "" {
		id, err = newID()
		if err != nil {
			return Connection{}, err
		}
	}

	ref := "connection:" + id + ":password"
	if index >= 0 && cfg.Connections[index].CredentialRef != "" {
		ref = cfg.Connections[index].CredentialRef
	}
	password := req.Password
	if password == "" {
		exists, err := a.secure.Exists(ref)
		if err != nil {
			return Connection{}, err
		}
		if !exists {
			return Connection{}, errors.New("management password is required")
		}
	} else {
		if err := a.secure.Put(ref, []byte(password)); err != nil {
			return Connection{}, err
		}
	}

	record := connectionRecord{ID: id, Name: name, URL: endpoint, CredentialRef: ref}
	if index >= 0 {
		cfg.Connections[index] = record
	} else {
		cfg.Connections = append(cfg.Connections, record)
	}
	if cfg.SelectedID == "" {
		cfg.SelectedID = id
	}
	if err := a.settings.Save(cfg); err != nil {
		return Connection{}, err
	}
	return Connection{ID: id, Name: name, URL: endpoint}, nil
}

func (a *App) DeleteConnection(id string) error {
	cfg, err := a.settings.Load()
	if err != nil {
		return err
	}
	next := make([]connectionRecord, 0, len(cfg.Connections))
	var ref string
	for _, c := range cfg.Connections {
		if c.ID == id {
			ref = c.CredentialRef
			continue
		}
		next = append(next, c)
	}
	cfg.Connections = next
	if cfg.SelectedID == id {
		cfg.SelectedID = ""
		if len(next) > 0 {
			cfg.SelectedID = next[0].ID
		}
	}
	if err := a.settings.Save(cfg); err != nil {
		return err
	}
	if ref != "" {
		if err := a.secure.Delete(ref); err != nil && !errors.Is(err, secureconfig.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (a *App) SelectConnection(id string) error {
	cfg, err := a.settings.Load()
	if err != nil {
		return err
	}
	for _, c := range cfg.Connections {
		if c.ID == id {
			cfg.SelectedID = id
			return a.settings.Save(cfg)
		}
	}
	return errors.New("connection not found")
}

func (a *App) TestConnection(id string) (TestResult, error) {
	cfg, err := a.settings.Load()
	if err != nil {
		return TestResult{}, err
	}
	var selected *connectionRecord
	for i := range cfg.Connections {
		if cfg.Connections[i].ID == id {
			selected = &cfg.Connections[i]
			break
		}
	}
	if selected == nil {
		return TestResult{}, errors.New("connection not found")
	}
	password, err := a.secure.Get(selected.CredentialRef)
	if err != nil {
		return TestResult{}, fmt.Errorf("load password: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, selected.URL+"/api/v1/info", nil)
	if err != nil {
		return TestResult{}, err
	}
	request.SetBasicAuth("admin", string(password))
	response, err := a.client.Do(request)
	if err != nil {
		return TestResult{Message: err.Error()}, nil
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		return TestResult{Message: "密码错误或服务端认证配置不一致"}, nil
	}
	if response.StatusCode != http.StatusOK {
		return TestResult{Message: "HTTP " + response.Status}, nil
	}
	var payload struct {
		Version  string `json:"version"`
		Hostname string `json:"hostname"`
		Runtime  struct {
			Kind    string `json:"kind"`
			Version string `json:"version"`
		} `json:"runtime"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return TestResult{Message: "响应格式无效: " + err.Error()}, nil
	}
	runtime := payload.Runtime.Kind
	if payload.Runtime.Version != "" {
		runtime += " · " + payload.Runtime.Version
	}
	return TestResult{OK: true, Message: "连接正常", Hostname: payload.Hostname, Runtime: runtime, Version: payload.Version}, nil
}

func (a *App) ListSites(id string) (SiteListResult, error) {
	var result SiteListResult
	if err := a.requestJSON(id, http.MethodGet, "/api/v1/sites", nil, &result); err != nil {
		return SiteListResult{}, err
	}
	if result.Sites == nil {
		result.Sites = []RemoteSite{}
	}
	return result, nil
}

func (a *App) CreateReverseProxy(id string, req CreateReverseProxyRequest) (CreateReverseProxyResult, error) {
	var result CreateReverseProxyResult
	if err := a.requestJSON(id, http.MethodPost, "/api/v1/sites/reverse-proxy", req, &result); err != nil {
		return CreateReverseProxyResult{}, err
	}
	return result, nil
}

func (a *App) requestJSON(id, method, path string, input any, output any) error {
	connection, password, err := a.connectionCredentials(id)
	if err != nil {
		return err
	}
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, connection.URL+path, body)
	if err != nil {
		return err
	}
	request.SetBasicAuth("admin", string(password))
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &payload) == nil && payload.Error != "" {
			return errors.New(payload.Error)
		}
		return fmt.Errorf("HTTP %s", response.Status)
	}
	if output != nil && len(data) > 0 {
		if err := json.Unmarshal(data, output); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (a *App) connectionCredentials(id string) (connectionRecord, []byte, error) {
	cfg, err := a.settings.Load()
	if err != nil {
		return connectionRecord{}, nil, err
	}
	for _, connection := range cfg.Connections {
		if connection.ID != id {
			continue
		}
		password, err := a.secure.Get(connection.CredentialRef)
		if err != nil {
			return connectionRecord{}, nil, fmt.Errorf("load password: %w", err)
		}
		return connection, password, nil
	}
	return connectionRecord{}, nil, errors.New("connection not found")
}

func normalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("CLI URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid CLI URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("CLI URL must use http or https")
	}
	host := parsed.Hostname()
	if host == "" {
		return "", errors.New("CLI URL host is required")
	}
	if parsed.Scheme == "http" {
		if !strings.EqualFold(host, "localhost") {
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				return "", errors.New("remote CLI connections must use HTTPS; plain HTTP is allowed only for localhost/loopback tunnels")
			}
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func newID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
