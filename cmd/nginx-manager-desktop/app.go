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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wanstu/wails-desktop-kit/jsonstore"
	"github.com/wanstu/wails-desktop-kit/paths"
	"github.com/wanstu/wails-desktop-kit/secureconfig"
)

const desktopAppID = "nginx-manager-desktop"

var accessLogStatusRE = regexp.MustCompile(`"\s+(\d{3})\s+(\d+|-)`)
var accessLogTimeRE = regexp.MustCompile(`\[([^\]]+)\]`)

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

type ConnectionHealth struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	OK             bool   `json:"ok"`
	PrivilegeReady bool   `json:"privilege_ready"`
	Message        string `json:"message,omitempty"`
	Runtime        string `json:"runtime,omitempty"`
}

type TestResult struct {
	OK               bool   `json:"ok"`
	Message          string `json:"message"`
	Hostname         string `json:"hostname,omitempty"`
	Runtime          string `json:"runtime,omitempty"`
	Version          string `json:"version,omitempty"`
	PrivilegeReady   bool   `json:"privilege_ready"`
	PrivilegeMessage string `json:"privilege_message,omitempty"`
}

type RemoteSite struct {
	ID                    string `json:"id"`
	ServerName            string `json:"server_name"`
	ProxyPass             string `json:"proxy_pass,omitempty"`
	Enabled               bool   `json:"enabled"`
	Managed               bool   `json:"managed"`
	WebSocket             bool   `json:"websocket"`
	HTTPS                 bool   `json:"https"`
	RedirectHTTPS         bool   `json:"redirect_https"`
	Certificate           string `json:"certificate,omitempty"`
	MaxBodySizeMB         int    `json:"max_body_size_mb"`
	ConnectTimeoutSeconds int    `json:"connect_timeout_seconds"`
	ReadTimeoutSeconds    int    `json:"read_timeout_seconds"`
	AccessLog             string `json:"access_log,omitempty"`
	ErrorLog              string `json:"error_log,omitempty"`
	Path                  string `json:"path"`
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
	ServerName            string `json:"server_name"`
	Upstream              string `json:"upstream"`
	WebSocket             bool   `json:"websocket"`
	MaxBodySizeMB         int    `json:"max_body_size_mb"`
	ConnectTimeoutSeconds int    `json:"connect_timeout_seconds"`
	ReadTimeoutSeconds    int    `json:"read_timeout_seconds"`
}

type CreateReverseProxyResult struct {
	Site       RemoteSite `json:"site"`
	TestOutput string     `json:"test_output"`
}

type RemoteSnapshot struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	Operation string `json:"operation"`
	SiteID    string `json:"site_id"`
	Enabled   bool   `json:"enabled"`
}

type SnapshotListResult struct {
	Snapshots []RemoteSnapshot `json:"snapshots"`
}

type RemoteCertificate struct {
	Name      string   `json:"name"`
	Domains   []string `json:"domains"`
	NotBefore string   `json:"not_before"`
	NotAfter  string   `json:"not_after"`
	Issuer    string   `json:"issuer"`
	Path      string   `json:"path"`
}

type RemoteCertbotStatus struct {
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

type RemoteRenewalTimerStatus struct {
	SystemdAvailable bool   `json:"systemd_available"`
	Installed        bool   `json:"installed"`
	Enabled          bool   `json:"enabled"`
	Active           bool   `json:"active"`
	LoadState        string `json:"load_state,omitempty"`
}

type RemoteServiceUnitStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	Active    bool   `json:"active"`
	LoadState string `json:"load_state,omitempty"`
}

type RemoteServiceDiagnostics struct {
	SystemdAvailable bool                      `json:"systemd_available"`
	Manager          RemoteServiceUnitStatus   `json:"manager"`
	Runtime          RemoteServiceUnitStatus   `json:"runtime"`
	Candidates       []RemoteServiceUnitStatus `json:"candidates"`
}

type RemotePathsConfig struct {
	SnapshotDir       string `json:"snapshot_dir,omitempty"`
	SnapshotRetention int    `json:"snapshot_retention,omitempty"`
	ACMEWebroot       string `json:"acme_webroot,omitempty"`
	CertLiveDir       string `json:"cert_live_dir,omitempty"`
	SiteLogDir        string `json:"site_log_dir,omitempty"`
}

type DiagnosticsResult struct {
	Executable      string                   `json:"executable,omitempty"`
	Runtime         RemoteRuntimeInfo        `json:"runtime"`
	Layout          RemoteLayout             `json:"layout"`
	Services        RemoteServiceDiagnostics `json:"services"`
	Certbot         RemoteCertbotStatus      `json:"certbot"`
	RenewalTimer    RemoteRenewalTimerStatus `json:"renewal_timer"`
	PathsConfigPath string                   `json:"paths_config_path"`
	PathsConfigured bool                     `json:"paths_configured"`
	PathsConfig     RemotePathsConfig        `json:"paths_config"`
}

type CertificateListResult struct {
	Certificates []RemoteCertificate      `json:"certificates"`
	Certbot      RemoteCertbotStatus      `json:"certbot"`
	RenewalTimer RemoteRenewalTimerStatus `json:"renewal_timer"`
}

type IssueCertificateRequest struct {
	Email         string `json:"email"`
	RedirectHTTPS bool   `json:"redirect_https"`
}

type UpdateTLSSettingsRequest struct {
	Enabled       bool `json:"enabled"`
	RedirectHTTPS bool `json:"redirect_https"`
}

type RenewCertificatesResult struct {
	Output string `json:"output"`
}

type RemoteLogFile struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type LogListResult struct {
	Logs []RemoteLogFile `json:"logs"`
}

type RemoteLogTail struct {
	File      RemoteLogFile `json:"file"`
	Lines     int           `json:"lines"`
	Truncated bool          `json:"truncated"`
	Content   string        `json:"content"`
}

type RemoteReloadResult struct {
	TestOutput   string `json:"test_output"`
	ReloadOutput string `json:"reload_output"`
}

type RemoteSiteConfigView struct {
	Site      RemoteSite `json:"site"`
	Content   string     `json:"content"`
	SizeBytes int64      `json:"size_bytes"`
}

type RemoteUpstreamHealth struct {
	SiteID     string `json:"site_id"`
	ServerName string `json:"server_name"`
	Upstream   string `json:"upstream"`
	Reachable  bool   `json:"reachable"`
	Healthy    bool   `json:"healthy"`
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

type RemoteRuntimeInfo struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version"`
}

type RemoteNginxStatus struct {
	Runtime  RemoteRuntimeInfo `json:"runtime"`
	Layout   RemoteLayout      `json:"layout"`
	ConfigOK bool              `json:"config_ok"`
	Output   string            `json:"output"`
}

type SiteTrafficSummary struct {
	ServerName    string `json:"server_name"`
	MatchedLines  int    `json:"matched_lines"`
	ParsedLines   int    `json:"parsed_lines"`
	Status2xx     int    `json:"status_2xx"`
	Status3xx     int    `json:"status_3xx"`
	Status4xx     int    `json:"status_4xx"`
	Status5xx     int    `json:"status_5xx"`
	ResponseBytes int64  `json:"response_bytes"`
}

type ServerOverview struct {
	Connected               bool                     `json:"connected"`
	Hostname                string                   `json:"hostname,omitempty"`
	CLIVersion              string                   `json:"cli_version,omitempty"`
	Runtime                 string                   `json:"runtime,omitempty"`
	PrivilegeReady          bool                     `json:"privilege_ready"`
	PrivilegeMessage        string                   `json:"privilege_message,omitempty"`
	ConfigOK                bool                     `json:"config_ok"`
	ConfigOutput            string                   `json:"config_output,omitempty"`
	Layout                  RemoteLayout             `json:"layout"`
	TotalSites              int                      `json:"total_sites"`
	ManagedSites            int                      `json:"managed_sites"`
	EnabledSites            int                      `json:"enabled_sites"`
	HTTPSSites              int                      `json:"https_sites"`
	Certificates            int                      `json:"certificates"`
	CertificatesExpiring    int                      `json:"certificates_expiring"`
	CertificatesExpired     int                      `json:"certificates_expired"`
	HTTPSWithoutCertificate int                      `json:"https_without_certificate"`
	NearestExpiry           string                   `json:"nearest_expiry,omitempty"`
	LogFiles                int                      `json:"log_files"`
	TrafficSource           string                   `json:"traffic_source,omitempty"`
	TrafficSources          []string                 `json:"traffic_sources"`
	TrafficLogsAvailable    int                      `json:"traffic_logs_available"`
	TrafficLogsUsed         int                      `json:"traffic_logs_used"`
	TrafficReadLines        int                      `json:"traffic_read_lines"`
	TrafficSampleLines      int                      `json:"traffic_sample_lines"`
	TrafficTimestampUnknown int                      `json:"traffic_timestamp_unknown"`
	TrafficWindowMinutes    int                      `json:"traffic_window_minutes"`
	TrafficParsedLines      int                      `json:"traffic_parsed_lines"`
	TrafficUnparsedLines    int                      `json:"traffic_unparsed_lines"`
	Status2xx               int                      `json:"status_2xx"`
	Status3xx               int                      `json:"status_3xx"`
	Status4xx               int                      `json:"status_4xx"`
	Status5xx               int                      `json:"status_5xx"`
	ResponseBytes           int64                    `json:"response_bytes"`
	SiteTraffic             []SiteTrafficSummary     `json:"site_traffic"`
	Certbot                 RemoteCertbotStatus      `json:"certbot"`
	RenewalTimer            RemoteRenewalTimerStatus `json:"renewal_timer"`
	Issues                  []string                 `json:"issues"`
	Warnings                []string                 `json:"warnings"`
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

func (a *App) CheckConnections() ([]ConnectionHealth, error) {
	connections, err := a.ListConnections()
	if err != nil {
		return nil, err
	}
	results := make([]ConnectionHealth, len(connections))
	if len(connections) == 0 {
		return results, nil
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for i, connection := range connections {
		i, connection := i, connection
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			health := ConnectionHealth{
				ID:   connection.ID,
				Name: connection.Name,
				URL:  connection.URL,
			}
			result, testErr := a.TestConnection(connection.ID)
			if testErr != nil {
				health.Message = testErr.Error()
				results[i] = health
				return
			}
			health.OK = result.OK
			health.PrivilegeReady = result.PrivilegeReady
			health.Runtime = result.Runtime
			health.Message = result.Message
			if result.OK && !result.PrivilegeReady && result.PrivilegeMessage != "" {
				health.Message = result.PrivilegeMessage
			}
			results[i] = health
		}()
	}
	wg.Wait()
	return results, nil
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

	var privilegeStatus struct {
		Ready      bool   `json:"ready"`
		Error      string `json:"error,omitempty"`
		ConfigOK   bool   `json:"config_ok"`
		TestOutput string `json:"test_output,omitempty"`
	}
	privilegeMessage := ""
	if err := a.requestJSON(id, http.MethodGet, "/api/v1/privilege/status", nil, &privilegeStatus); err != nil {
		privilegeMessage = err.Error()
	} else if !privilegeStatus.Ready {
		privilegeMessage = privilegeStatus.Error
	} else if !privilegeStatus.ConfigOK {
		privilegeMessage = "受限 helper 可用，但 nginx -t 未通过"
	} else {
		privilegeMessage = "受限 helper 正常"
	}

	return TestResult{
		OK:               true,
		Message:          "连接正常",
		Hostname:         payload.Hostname,
		Runtime:          runtime,
		Version:          payload.Version,
		PrivilegeReady:   privilegeStatus.Ready && privilegeStatus.ConfigOK,
		PrivilegeMessage: privilegeMessage,
	}, nil
}

func (a *App) LoadOverview(id string) (ServerOverview, error) {
	return a.loadOverview(id, 0)
}

func (a *App) LoadOverviewWindow(id string, trafficWindowMinutes int) (ServerOverview, error) {
	if trafficWindowMinutes != 15 &&
		trafficWindowMinutes != 60 &&
		trafficWindowMinutes != 360 &&
		trafficWindowMinutes != 1440 {
		return ServerOverview{}, errors.New("traffic window must be 15, 60, 360 or 1440 minutes")
	}
	return a.loadOverview(id, trafficWindowMinutes)
}

func (a *App) loadOverview(id string, trafficWindowMinutes int) (ServerOverview, error) {
	result := ServerOverview{Issues: []string{}, Warnings: []string{}, TrafficWindowMinutes: trafficWindowMinutes}

	var info struct {
		Version  string `json:"version"`
		Hostname string `json:"hostname"`
		Runtime  struct {
			Kind    string `json:"kind"`
			Version string `json:"version"`
		} `json:"runtime"`
	}
	if err := a.requestJSON(id, http.MethodGet, "/api/v1/info", nil, &info); err != nil {
		return ServerOverview{}, err
	}
	result.Connected = true
	result.Hostname = info.Hostname
	result.CLIVersion = info.Version
	result.Runtime = info.Runtime.Kind
	if info.Runtime.Version != "" {
		result.Runtime += " · " + info.Runtime.Version
	}

	var privilegeStatus struct {
		Ready    bool   `json:"ready"`
		Error    string `json:"error,omitempty"`
		ConfigOK bool   `json:"config_ok"`
	}
	if err := a.requestJSON(id, http.MethodGet, "/api/v1/privilege/status", nil, &privilegeStatus); err != nil {
		result.PrivilegeMessage = err.Error()
		result.Issues = append(result.Issues, "管理权限："+err.Error())
	} else if !privilegeStatus.Ready {
		result.PrivilegeMessage = privilegeStatus.Error
		result.Issues = append(result.Issues, "管理权限："+privilegeStatus.Error)
	} else if !privilegeStatus.ConfigOK {
		result.PrivilegeMessage = "受限 helper 可用，但 nginx -t 未通过"
		result.Issues = append(result.Issues, result.PrivilegeMessage)
	} else {
		result.PrivilegeReady = true
		result.PrivilegeMessage = "受限 helper 正常"
	}

	var nginxStatus RemoteNginxStatus
	if err := a.requestJSON(id, http.MethodGet, "/api/v1/nginx/status", nil, &nginxStatus); err != nil {
		result.Issues = append(result.Issues, "Nginx 状态："+err.Error())
	} else {
		result.ConfigOK = nginxStatus.ConfigOK
		result.ConfigOutput = nginxStatus.Output
		result.Layout = nginxStatus.Layout
		if !nginxStatus.ConfigOK {
			result.Issues = append(result.Issues, "nginx -t 未通过")
		}
	}

	var siteList []RemoteSite
	sites, err := a.ListSites(id)
	if err != nil {
		result.Issues = append(result.Issues, "站点："+err.Error())
	} else {
		siteList = sites.Sites
		result.TotalSites = len(sites.Sites)
		if result.Layout.Mode == "" {
			result.Layout = sites.Layout
		}
		for _, site := range sites.Sites {
			if site.Managed {
				result.ManagedSites++
			}
			if site.Enabled {
				result.EnabledSites++
			}
			if site.HTTPS {
				result.HTTPSSites++
			}
		}
	}

	certificates, err := a.ListCertificates(id)
	if err != nil {
		result.Issues = append(result.Issues, "证书："+err.Error())
	} else {
		result.Certificates = len(certificates.Certificates)
		result.Certbot = certificates.Certbot
		result.RenewalTimer = certificates.RenewalTimer
		now := time.Now()
		var nearest time.Time
		for _, certificate := range certificates.Certificates {
			expires, parseErr := time.Parse(time.RFC3339, certificate.NotAfter)
			if parseErr != nil {
				expires, parseErr = time.Parse(time.RFC3339Nano, certificate.NotAfter)
			}
			if parseErr == nil {
				if nearest.IsZero() || expires.Before(nearest) {
					nearest = expires
				}
				if expires.Before(now) {
					result.CertificatesExpired++
				} else if expires.Before(now.Add(30 * 24 * time.Hour)) {
					result.CertificatesExpiring++
				}
			}
		}

		for _, site := range siteList {
			if !site.HTTPS || strings.TrimSpace(site.ServerName) == "" || site.ServerName == "_" {
				continue
			}
			covered := false
			for _, certificate := range certificates.Certificates {
				if certificateCoversHost(certificate, site.ServerName) {
					covered = true
					break
				}
			}
			if !covered {
				result.HTTPSWithoutCertificate++
			}
		}

		if !nearest.IsZero() {
			result.NearestExpiry = nearest.Format(time.RFC3339)
		}
		if result.CertificatesExpired > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%d 张证书已过期", result.CertificatesExpired))
		}
		if result.CertificatesExpiring > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%d 张证书将在 30 天内到期", result.CertificatesExpiring))
		}
		if result.HTTPSWithoutCertificate > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%d 个 HTTPS 站点未找到匹配证书", result.HTTPSWithoutCertificate))
		}
		if result.Certificates > 0 && !certificates.Certbot.Available {
			result.Warnings = append(result.Warnings, "服务器存在证书，但 Certbot 当前不可用")
		}
		if result.Certificates > 0 && (!certificates.RenewalTimer.Enabled || !certificates.RenewalTimer.Active) {
			result.Warnings = append(result.Warnings, "服务器存在证书，但自动续期 Timer 未正常运行")
		}
	}

	logs, err := a.ListLogs(id)
	if err != nil {
		result.Issues = append(result.Issues, "日志："+err.Error())
	} else {
		result.LogFiles = len(logs)
		accessLogs, available := selectOverviewAccessLogs(logs, siteList, 8)
		result.TrafficLogsAvailable = available
		if available > len(accessLogs) {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"访问概况共有 %d 个 access log，本次按优先级聚合前 %d 个",
				available, len(accessLogs),
			))
		}

		var since time.Time
		if trafficWindowMinutes > 0 {
			since = time.Now().Add(-time.Duration(trafficWindowMinutes) * time.Minute)
		}
		failed := 0
		for _, logFile := range accessLogs {
			tail, tailErr := a.TailLog(id, logFile.ID, 1000)
			if tailErr != nil {
				failed++
				result.Warnings = append(result.Warnings, "访问日志读取失败："+logFile.Path+"："+tailErr.Error())
				continue
			}
			result.TrafficLogsUsed++
			lines, readLines, timestampUnknown := trafficLinesForWindow(tail.Content, since)
			applyTrafficSummaryLines(&result, tail.File.Path, lines, readLines, timestampUnknown)
			appendSiteTrafficSummaryLines(&result, lines, siteList, tail.File.Path)
		}
		if len(accessLogs) > 0 && result.TrafficLogsUsed == 0 && failed > 0 {
			result.Issues = append(result.Issues, "所有访问日志都无法读取")
		}
	}

	return result, nil
}

func certificateCoversHost(certificate RemoteCertificate, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}

	names := append([]string(nil), certificate.Domains...)
	if certificate.Name != "" {
		names = append(names, certificate.Name)
	}
	for _, candidate := range names {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == host {
			return true
		}
		if !strings.HasPrefix(candidate, "*.") {
			continue
		}
		suffix := strings.TrimPrefix(candidate, "*")
		if !strings.HasSuffix(host, suffix) {
			continue
		}
		prefix := strings.TrimSuffix(host, suffix)
		if prefix != "" && !strings.Contains(prefix, ".") {
			return true
		}
	}
	return false
}

func parseAccessLogStatusAndBytes(line string) (status int, size int64, ok bool) {
	match := accessLogStatusRE.FindStringSubmatch(line)
	if len(match) != 3 {
		return 0, 0, false
	}
	status, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, false
	}
	if match[2] != "-" {
		if parsed, err := strconv.ParseInt(match[2], 10, 64); err == nil && parsed > 0 {
			size = parsed
		}
	}
	return status, size, true
}

func applyStatusClass(status int, status2xx, status3xx, status4xx, status5xx *int) {
	switch status / 100 {
	case 2:
		*status2xx++
	case 3:
		*status3xx++
	case 4:
		*status4xx++
	case 5:
		*status5xx++
	}
}

func selectOverviewAccessLogs(logs []RemoteLogFile, sites []RemoteSite, limit int) ([]RemoteLogFile, int) {
	managedPaths := map[string]bool{}
	for _, site := range sites {
		if path := strings.TrimSpace(site.AccessLog); path != "" {
			managedPaths[path] = true
		}
	}

	accessLogs := make([]RemoteLogFile, 0)
	for _, logFile := range logs {
		if logFile.Kind == "access" {
			accessLogs = append(accessLogs, logFile)
		}
	}
	sort.SliceStable(accessLogs, func(i, j int) bool {
		leftManaged := managedPaths[accessLogs[i].Path]
		rightManaged := managedPaths[accessLogs[j].Path]
		if leftManaged != rightManaged {
			return leftManaged
		}
		return accessLogs[i].Path < accessLogs[j].Path
	})

	available := len(accessLogs)
	if limit > 0 && len(accessLogs) > limit {
		accessLogs = accessLogs[:limit]
	}
	return accessLogs, available
}

func parseAccessLogTime(line string) (time.Time, bool) {
	match := accessLogTimeRE.FindStringSubmatch(line)
	if len(match) != 2 {
		return time.Time{}, false
	}
	parsed, err := time.Parse("02/Jan/2006:15:04:05 -0700", match[1])
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func trafficLinesForWindow(content string, since time.Time) (lines []string, readLines int, timestampUnknown int) {
	raw := strings.Split(strings.TrimSpace(content), "\n")
	if len(raw) == 1 && raw[0] == "" {
		raw = nil
	}
	readLines = len(raw)
	if since.IsZero() {
		return raw, readLines, 0
	}

	lines = make([]string, 0, len(raw))
	for _, line := range raw {
		loggedAt, ok := parseAccessLogTime(line)
		if !ok {
			timestampUnknown++
			continue
		}
		if !loggedAt.Before(since) {
			lines = append(lines, line)
		}
	}
	return lines, readLines, timestampUnknown
}

func applyTrafficSummary(result *ServerOverview, tail RemoteLogTail) {
	lines, readLines, timestampUnknown := trafficLinesForWindow(tail.Content, time.Time{})
	applyTrafficSummaryLines(result, tail.File.Path, lines, readLines, timestampUnknown)
}

func applyTrafficSummaryLines(result *ServerOverview, source string, lines []string, readLines, timestampUnknown int) {
	if result.TrafficSource == "" {
		result.TrafficSource = source
	}
	if source != "" {
		seen := false
		for _, existing := range result.TrafficSources {
			if existing == source {
				seen = true
				break
			}
		}
		if !seen {
			result.TrafficSources = append(result.TrafficSources, source)
		}
	}
	result.TrafficReadLines += readLines
	result.TrafficSampleLines += len(lines)
	result.TrafficTimestampUnknown += timestampUnknown

	for _, line := range lines {
		status, size, ok := parseAccessLogStatusAndBytes(line)
		if !ok {
			result.TrafficUnparsedLines++
			continue
		}
		result.TrafficParsedLines++
		applyStatusClass(status, &result.Status2xx, &result.Status3xx, &result.Status4xx, &result.Status5xx)
		result.ResponseBytes += size
	}
}

func applySiteTrafficSummaries(result *ServerOverview, tail RemoteLogTail, sites []RemoteSite) {
	lines, _, _ := trafficLinesForWindow(tail.Content, time.Time{})
	appendSiteTrafficSummaryLines(result, lines, sites, tail.File.Path)
}

func appendSiteTrafficSummaryLines(result *ServerOverview, lines []string, sites []RemoteSite, logPath string) {
	existing := map[string]SiteTrafficSummary{}
	for _, summary := range result.SiteTraffic {
		existing[strings.ToLower(summary.ServerName)] = summary
	}

	seen := map[string]bool{}
	for _, site := range sites {
		host := strings.ToLower(strings.TrimSpace(site.ServerName))
		if host == "" || host == "_" || strings.Contains(host, "*") || seen[host] {
			continue
		}
		seen[host] = true

		summary := existing[host]
		if summary.ServerName == "" {
			summary.ServerName = site.ServerName
		}
		directLog := strings.TrimSpace(site.AccessLog)
		for _, line := range lines {
			matched := false
			if directLog != "" {
				matched = filepath.Clean(directLog) == filepath.Clean(logPath)
			} else {
				matched = lineContainsHostname(line, host)
			}
			if !matched {
				continue
			}
			summary.MatchedLines++
			status, size, ok := parseAccessLogStatusAndBytes(line)
			if !ok {
				continue
			}
			summary.ParsedLines++
			applyStatusClass(status, &summary.Status2xx, &summary.Status3xx, &summary.Status4xx, &summary.Status5xx)
			summary.ResponseBytes += size
		}
		if summary.MatchedLines > 0 {
			existing[host] = summary
		}
	}

	summaries := make([]SiteTrafficSummary, 0, len(existing))
	for _, summary := range existing {
		if summary.MatchedLines > 0 {
			summaries = append(summaries, summary)
		}
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].MatchedLines == summaries[j].MatchedLines {
			return summaries[i].ServerName < summaries[j].ServerName
		}
		return summaries[i].MatchedLines > summaries[j].MatchedLines
	})
	result.SiteTraffic = summaries
}

func lineContainsHostname(line, host string) bool {
	line = strings.ToLower(line)
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for offset := 0; offset < len(line); {
		index := strings.Index(line[offset:], host)
		if index < 0 {
			return false
		}
		index += offset
		beforeOK := index == 0 || !hostnameByte(line[index-1])
		afterIndex := index + len(host)
		afterOK := afterIndex == len(line) || !hostnameByte(line[afterIndex])
		if beforeOK && afterOK {
			return true
		}
		offset = index + 1
	}
	return false
}

func hostnameByte(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= '0' && value <= '9' ||
		value == '.' || value == '-'
}

func (a *App) LoadDiagnostics(id string) (DiagnosticsResult, error) {
	var result DiagnosticsResult
	if err := a.requestJSON(id, http.MethodGet, "/api/v1/diagnostics", nil, &result); err != nil {
		return DiagnosticsResult{}, err
	}
	return result, nil
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

func (a *App) CheckUpstream(id, siteID string) (RemoteUpstreamHealth, error) {
	var result RemoteUpstreamHealth
	path := "/api/v1/sites/" + url.PathEscape(siteID) + "/upstream-health"
	if err := a.requestJSON(id, http.MethodGet, path, nil, &result); err != nil {
		return RemoteUpstreamHealth{}, err
	}
	return result, nil
}

func (a *App) GetSiteConfig(id, siteID string) (RemoteSiteConfigView, error) {
	var result RemoteSiteConfigView
	path := "/api/v1/sites/" + url.PathEscape(siteID) + "/config"
	if err := a.requestJSON(id, http.MethodGet, path, nil, &result); err != nil {
		return RemoteSiteConfigView{}, err
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

func (a *App) UpdateReverseProxy(id, siteID string, req CreateReverseProxyRequest) (CreateReverseProxyResult, error) {
	var result CreateReverseProxyResult
	path := "/api/v1/sites/" + url.PathEscape(siteID) + "/reverse-proxy"
	if err := a.requestJSON(id, http.MethodPut, path, req, &result); err != nil {
		return CreateReverseProxyResult{}, err
	}
	return result, nil
}

func (a *App) ListSnapshots(id string) ([]RemoteSnapshot, error) {
	var result SnapshotListResult
	if err := a.requestJSON(id, http.MethodGet, "/api/v1/snapshots?limit=50", nil, &result); err != nil {
		return nil, err
	}
	if result.Snapshots == nil {
		result.Snapshots = []RemoteSnapshot{}
	}
	return result.Snapshots, nil
}

func (a *App) RestoreSnapshot(id, snapshotID string) (RemoteSite, error) {
	var site RemoteSite
	path := "/api/v1/snapshots/" + url.PathEscape(snapshotID) + "/restore"
	if err := a.requestJSON(id, http.MethodPost, path, nil, &site); err != nil {
		return RemoteSite{}, err
	}
	return site, nil
}

func (a *App) ListCertificates(id string) (CertificateListResult, error) {
	var result CertificateListResult
	if err := a.requestJSON(id, http.MethodGet, "/api/v1/certificates", nil, &result); err != nil {
		return CertificateListResult{}, err
	}
	if result.Certificates == nil {
		result.Certificates = []RemoteCertificate{}
	}
	return result, nil
}

func (a *App) IssueCertificate(id, siteID string, req IssueCertificateRequest) (RemoteSite, error) {
	var site RemoteSite
	path := "/api/v1/sites/" + url.PathEscape(siteID) + "/certificate"
	if err := a.requestJSON(id, http.MethodPost, path, req, &site); err != nil {
		return RemoteSite{}, err
	}
	return site, nil
}

func (a *App) UpdateSiteTLS(id, siteID string, req UpdateTLSSettingsRequest) (RemoteSite, error) {
	var site RemoteSite
	path := "/api/v1/sites/" + url.PathEscape(siteID) + "/tls"
	if err := a.requestJSON(id, http.MethodPut, path, req, &site); err != nil {
		return RemoteSite{}, err
	}
	return site, nil
}

func (a *App) RenewCertificates(id string) (RenewCertificatesResult, error) {
	var result RenewCertificatesResult
	if err := a.requestJSON(id, http.MethodPost, "/api/v1/certificates/renew", nil, &result); err != nil {
		return RenewCertificatesResult{}, err
	}
	return result, nil
}

func (a *App) SafeReload(id string) (RemoteReloadResult, error) {
	var result RemoteReloadResult
	if err := a.requestJSON(id, http.MethodPost, "/api/v1/nginx/reload", nil, &result); err != nil {
		return RemoteReloadResult{}, err
	}
	return result, nil
}

func (a *App) ListLogs(id string) ([]RemoteLogFile, error) {
	var result LogListResult
	if err := a.requestJSON(id, http.MethodGet, "/api/v1/logs", nil, &result); err != nil {
		return nil, err
	}
	if result.Logs == nil {
		result.Logs = []RemoteLogFile{}
	}
	return result.Logs, nil
}

func (a *App) TailLog(id, logID string, lines int) (RemoteLogTail, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > 1000 {
		return RemoteLogTail{}, errors.New("日志行数最多为 1000")
	}
	var result RemoteLogTail
	path := "/api/v1/logs/" + url.PathEscape(logID) + "?lines=" + fmt.Sprintf("%d", lines)
	if err := a.requestJSON(id, http.MethodGet, path, nil, &result); err != nil {
		return RemoteLogTail{}, err
	}
	return result, nil
}

func (a *App) SetSiteEnabled(id, siteID string, enabled bool) (RemoteSite, error) {
	var site RemoteSite
	path := "/api/v1/sites/" + url.PathEscape(siteID) + "/enabled"
	if err := a.requestJSON(id, http.MethodPut, path, map[string]bool{"enabled": enabled}, &site); err != nil {
		return RemoteSite{}, err
	}
	return site, nil
}

func (a *App) DeleteSite(id, siteID string) (RemoteSite, error) {
	var site RemoteSite
	path := "/api/v1/sites/" + url.PathEscape(siteID)
	if err := a.requestJSON(id, http.MethodDelete, path, nil, &site); err != nil {
		return RemoteSite{}, err
	}
	return site, nil
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
