package nginxmgr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wanstu/wails-desktop-kit/atomicfile"
)

const managedMarker = "# managed-by: nginx-manager"

var mutationMu sync.Mutex

var (
	serverNameRE          = regexp.MustCompile("(?m)^\\s*server_name\\s+([^;]+);")
	proxyPassRE           = regexp.MustCompile("(?m)^\\s*proxy_pass\\s+([^;]+);")
	sslCertificateRE      = regexp.MustCompile("(?m)^\\s*ssl_certificate\\s+([^;]+);")
	clientMaxBodySizeRE   = regexp.MustCompile("(?m)^\\s*client_max_body_size\\s+(\\d+)m;")
	proxyConnectTimeoutRE = regexp.MustCompile("(?m)^\\s*proxy_connect_timeout\\s+(\\d+)s;")
	proxyReadTimeoutRE    = regexp.MustCompile("(?m)^\\s*proxy_read_timeout\\s+(\\d+)s;")
	accessLogRE           = regexp.MustCompile(`(?m)^\s*access_log\s+("[^"]+"|'[^']+'|[^;\s]+)`)
	errorLogRE            = regexp.MustCompile(`(?m)^\s*error_log\s+("[^"]+"|'[^']+'|[^;\s]+)`)
)

type RuntimeInfo struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

type Layout struct {
	MainConfig   string `json:"main_config"`
	AvailableDir string `json:"available_dir"`
	EnabledDir   string `json:"enabled_dir"`
	Mode         string `json:"mode"`
	MimeTypes    string `json:"mime_types,omitempty"`
}

type Site struct {
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

type ReverseProxyRequest struct {
	ServerName            string `json:"server_name"`
	Upstream              string `json:"upstream"`
	WebSocket             bool   `json:"websocket"`
	MaxBodySizeMB         int    `json:"max_body_size_mb"`
	ConnectTimeoutSeconds int    `json:"connect_timeout_seconds"`
	ReadTimeoutSeconds    int    `json:"read_timeout_seconds"`
}

type UpdateReverseProxyRequest struct {
	ServerName            string `json:"server_name"`
	Upstream              string `json:"upstream"`
	WebSocket             bool   `json:"websocket"`
	MaxBodySizeMB         int    `json:"max_body_size_mb"`
	ConnectTimeoutSeconds int    `json:"connect_timeout_seconds"`
	ReadTimeoutSeconds    int    `json:"read_timeout_seconds"`
}

type ProxyOptions struct {
	MaxBodySizeMB         int
	ConnectTimeoutSeconds int
	ReadTimeoutSeconds    int
	AccessLog             string
	ErrorLog              string
}

type ApplyResult struct {
	Site       Site   `json:"site"`
	TestOutput string `json:"test_output"`
}

type Runner interface {
	Run(context.Context, string, ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, path string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

type Manager struct {
	Runtime RuntimeInfo
	Layout  Layout
	Runner  Runner
}

func New(ctx context.Context) (*Manager, error) {
	if _, _, err := LoadPathsConfig(); err != nil {
		return nil, err
	}
	runtime := DetectRuntime(ctx)
	if runtime.Path == "" {
		return nil, errors.New("nginx/openresty executable not found")
	}
	layout, err := DetectLayout(ctx, runtime, ExecRunner{})
	if err != nil {
		return nil, err
	}
	return &Manager{Runtime: runtime, Layout: layout, Runner: ExecRunner{}}, nil
}

func DetectRuntime(ctx context.Context) RuntimeInfo {
	for _, candidate := range []struct{ kind, name string }{{"nginx", "nginx"}, {"openresty", "openresty"}} {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			continue
		}
		output, _ := ExecRunner{}.Run(ctx, path, "-v")
		kind := candidate.kind
		if strings.Contains(strings.ToLower(output), "openresty") {
			kind = "openresty"
		}
		return RuntimeInfo{Kind: kind, Path: path, Version: output}
	}
	return RuntimeInfo{}
}

func DetectLayout(ctx context.Context, runtime RuntimeInfo, runner Runner) (Layout, error) {
	output, _ := runner.Run(ctx, runtime.Path, "-V")
	mainConfig := extractConfigureArg(output, "--conf-path=")
	if mainConfig == "" {
		if runtime.Kind == "openresty" {
			mainConfig = "/usr/local/openresty/nginx/conf/nginx.conf"
		} else {
			mainConfig = "/etc/nginx/nginx.conf"
		}
	}
	mainConfig = filepath.Clean(mainConfig)
	root := filepath.Dir(mainConfig)
	mimeTypes := filepath.Join(root, "mime.types")
	if _, err := os.Stat(mimeTypes); err != nil {
		mimeTypes = ""
	}
	available := filepath.Join(root, "sites-available")
	enabled := filepath.Join(root, "sites-enabled")
	if isDir(available) && isDir(enabled) {
		return Layout{MainConfig: mainConfig, AvailableDir: available, EnabledDir: enabled, Mode: "sites-enabled", MimeTypes: mimeTypes}, nil
	}
	confD := filepath.Join(root, "conf.d")
	if isDir(confD) {
		return Layout{MainConfig: mainConfig, AvailableDir: confD, EnabledDir: confD, Mode: "conf.d", MimeTypes: mimeTypes}, nil
	}
	return Layout{}, fmt.Errorf("unsupported nginx layout under %s", root)
}

func extractConfigureArg(output, prefix string) string {
	for _, field := range strings.Fields(output) {
		if strings.HasPrefix(field, prefix) {
			return strings.Trim(strings.TrimPrefix(field, prefix), "'\\\"")
		}
	}
	return ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (m *Manager) Status(ctx context.Context) (bool, string) {
	out, err := m.Runner.Run(ctx, m.Runtime.Path, "-t")
	return err == nil, out
}

func (m *Manager) ListSites() ([]Site, error) {
	entries, err := os.ReadDir(m.Layout.AvailableDir)
	if err != nil {
		return nil, fmt.Errorf("read sites: %w", err)
	}
	sites := make([]Site, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if m.Layout.Mode == "conf.d" &&
			!strings.HasSuffix(entry.Name(), ".conf") &&
			!strings.HasSuffix(entry.Name(), ".conf.disabled") {
			continue
		}
		path := filepath.Join(m.Layout.AvailableDir, entry.Name())
		data, err := m.readSiteConfigFile(path)
		if err != nil {
			continue
		}
		text := string(data)
		canonicalID := entry.Name()
		if m.Layout.Mode == "conf.d" && strings.HasSuffix(canonicalID, ".disabled") {
			canonicalID = strings.TrimSuffix(canonicalID, ".disabled")
		}
		site := Site{ID: canonicalID, Path: path, Managed: strings.Contains(text, managedMarker), Enabled: m.siteEnabled(entry.Name())}
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
		applyProxyOptionFields(&site, text)
		applySiteLogFields(&site, text)
		applyTLSFields(&site, text)
		sites = append(sites, site)
	}
	sort.Slice(sites, func(i, j int) bool {
		left, right := sites[i].ServerName, sites[j].ServerName
		if left == "" {
			left = sites[i].ID
		}
		if right == "" {
			right = sites[j].ID
		}
		return left < right
	})
	return sites, nil
}

func applyProxyOptionFields(site *Site, text string) {
	if match := clientMaxBodySizeRE.FindStringSubmatch(text); len(match) == 2 {
		site.MaxBodySizeMB, _ = strconv.Atoi(match[1])
	}
	if match := proxyConnectTimeoutRE.FindStringSubmatch(text); len(match) == 2 {
		site.ConnectTimeoutSeconds, _ = strconv.Atoi(match[1])
	}
	if match := proxyReadTimeoutRE.FindStringSubmatch(text); len(match) == 2 {
		site.ReadTimeoutSeconds, _ = strconv.Atoi(match[1])
	}
}

func proxyOptionsFromSite(site Site) ProxyOptions {
	return ProxyOptions{
		MaxBodySizeMB:         site.MaxBodySizeMB,
		ConnectTimeoutSeconds: site.ConnectTimeoutSeconds,
		ReadTimeoutSeconds:    site.ReadTimeoutSeconds,
		AccessLog:             site.AccessLog,
		ErrorLog:              site.ErrorLog,
	}
}

func applySiteLogFields(site *Site, text string) {
	if match := accessLogRE.FindStringSubmatch(text); len(match) == 2 {
		site.AccessLog = strings.Trim(strings.TrimSpace(match[1]), "'\"")
	}
	if match := errorLogRE.FindStringSubmatch(text); len(match) == 2 {
		site.ErrorLog = strings.Trim(strings.TrimSpace(match[1]), "'\"")
	}
}

func validateProxyOptions(options ProxyOptions) (ProxyOptions, error) {
	if options.MaxBodySizeMB < 0 || options.MaxBodySizeMB > 10240 {
		return ProxyOptions{}, errors.New("max_body_size_mb must be between 0 and 10240")
	}
	if options.ConnectTimeoutSeconds < 0 || options.ConnectTimeoutSeconds > 300 {
		return ProxyOptions{}, errors.New("connect_timeout_seconds must be between 0 and 300")
	}
	if options.ReadTimeoutSeconds < 0 || options.ReadTimeoutSeconds > 86400 {
		return ProxyOptions{}, errors.New("read_timeout_seconds must be between 0 and 86400")
	}
	return options, nil
}

func websocketEnabled(text string) bool {
	return strings.Contains(text, "proxy_set_header Upgrade $http_upgrade;") &&
		strings.Contains(text, "proxy_set_header Connection \"upgrade\";")
}

func (m *Manager) siteEnabled(name string) bool {
	if m.Layout.Mode == "conf.d" {
		return !strings.HasSuffix(name, ".disabled")
	}
	_, err := os.Lstat(filepath.Join(m.Layout.EnabledDir, name))
	return err == nil
}

func (m *Manager) CreateReverseProxy(ctx context.Context, req ReverseProxyRequest) (ApplyResult, error) {
	mutationMu.Lock()
	defer mutationMu.Unlock()
	serverName, err := validateServerName(req.ServerName)
	if err != nil {
		return ApplyResult{}, err
	}
	upstream, err := validateUpstream(req.Upstream)
	if err != nil {
		return ApplyResult{}, err
	}
	options, err := validateProxyOptions(ProxyOptions{
		MaxBodySizeMB:         req.MaxBodySizeMB,
		ConnectTimeoutSeconds: req.ConnectTimeoutSeconds,
		ReadTimeoutSeconds:    req.ReadTimeoutSeconds,
	})
	if err != nil {
		return ApplyResult{}, err
	}
	if err := ensureManagedSiteLogDir(); err != nil {
		return ApplyResult{}, err
	}
	options, err = withManagedSiteLogs(options, serverName)
	if err != nil {
		return ApplyResult{}, err
	}

	name := siteFileName(serverName, m.Layout.Mode)
	target := filepath.Join(m.Layout.AvailableDir, name)
	if _, err := os.Lstat(target); err == nil {
		return ApplyResult{}, fmt.Errorf("site %q already exists", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ApplyResult{}, fmt.Errorf("check site target: %w", err)
	}

	content := renderReverseProxyManaged(serverName, upstream, req.WebSocket, nil, "", options)
	if err := m.validateCandidate(ctx, name, content); err != nil {
		return ApplyResult{}, err
	}
	return m.applySite(ctx, name, content)
}

func validateServerName(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "", errors.New("server_name is required")
	}
	if strings.ContainsAny(value, " /\\;{}\t\r\n") || len(value) > 253 {
		return "", errors.New("invalid server_name")
	}
	if value == "_" {
		return value, nil
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", errors.New("server_name must be a DNS hostname")
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return "", errors.New("server_name must be a DNS hostname")
			}
		}
	}
	return value, nil
}

func validateUpstream(value string) (string, error) {
	value = strings.TrimSpace(value)
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", errors.New("upstream must be an http or https URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(value, "{};\r\n") {
		return "", errors.New("upstream contains unsupported components")
	}
	host := strings.ToLower(u.Hostname())
	if net.ParseIP(host) == nil {
		validated, err := validateServerName(host)
		if err != nil || validated == "_" {
			return "", errors.New("upstream host is invalid")
		}
	}
	return value, nil
}

func siteFileName(serverName, mode string) string {
	name := "nginx-manager-" + strings.ReplaceAll(serverName, "*", "wildcard")
	if mode == "conf.d" {
		name += ".conf"
	}
	return name
}

func renderReverseProxy(serverName, upstream string, websocket bool) []byte {
	return renderReverseProxyManaged(serverName, upstream, websocket, nil, "")
}

func renderReverseProxyManaged(serverName, upstream string, websocket bool, tls *TLSConfig, acmeRoot string, optionValues ...ProxyOptions) []byte {
	options := ProxyOptions{}
	if len(optionValues) > 0 {
		options = optionValues[0]
	}
	var b strings.Builder
	b.WriteString(managedMarker + "\n")
	b.WriteString("# generated; edit through nginx-manager\n")
	if tls != nil {
		b.WriteString(certificateMarkerPrefix + tls.CertificateName + "\n")
		if tls.RedirectHTTPS {
			b.WriteString(redirectHTTPSMarker + "\n")
		}
	}
	b.WriteString("\n")

	b.WriteString("server {\n")
	b.WriteString("    listen 80;\n")
	b.WriteString("    listen [::]:80;\n")
	b.WriteString("    server_name " + serverName + ";\n")
	writeServerProxyOptions(&b, options)
	b.WriteString("\n")
	writeACMELocation(&b, acmeRoot)
	if tls != nil && tls.RedirectHTTPS {
		b.WriteString("    location / {\n")
		b.WriteString("        return 301 https://$host$request_uri;\n")
		b.WriteString("    }\n")
	} else {
		writeProxyLocation(&b, upstream, websocket, options)
	}
	b.WriteString("}\n")

	if tls != nil {
		b.WriteString("\nserver {\n")
		b.WriteString("    listen 443 ssl;\n")
		b.WriteString("    listen [::]:443 ssl;\n")
		b.WriteString("    server_name " + serverName + ";\n")
		b.WriteString("    ssl_certificate " + tls.FullchainPath + ";\n")
		b.WriteString("    ssl_certificate_key " + tls.PrivateKeyPath + ";\n")
		writeServerProxyOptions(&b, options)
		b.WriteString("\n")
		writeProxyLocation(&b, upstream, websocket, options)
		b.WriteString("}\n")
	}
	return []byte(b.String())
}

func writeACMELocation(b *strings.Builder, acmeRoot string) {
	if strings.TrimSpace(acmeRoot) == "" {
		return
	}
	b.WriteString("    location ^~ /.well-known/acme-challenge/ {\n")
	b.WriteString("        root " + nginxQuote(acmeRoot) + ";\n")
	b.WriteString("        default_type text/plain;\n")
	b.WriteString("        try_files $uri =404;\n")
	b.WriteString("    }\n\n")
}

func writeServerProxyOptions(b *strings.Builder, options ProxyOptions) {
	if options.AccessLog != "" {
		b.WriteString("    access_log " + nginxQuote(options.AccessLog) + ";\n")
	}
	if options.ErrorLog != "" {
		b.WriteString("    error_log " + nginxQuote(options.ErrorLog) + ";\n")
	}
	if options.MaxBodySizeMB > 0 {
		b.WriteString(fmt.Sprintf("    client_max_body_size %dm;\n", options.MaxBodySizeMB))
	}
}

func writeProxyLocation(b *strings.Builder, upstream string, websocket bool, options ProxyOptions) {
	b.WriteString("    location / {\n")
	b.WriteString("        proxy_pass " + upstream + ";\n")
	b.WriteString("        proxy_http_version 1.1;\n")
	b.WriteString("        proxy_set_header Host $host;\n")
	b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	if options.ConnectTimeoutSeconds > 0 {
		b.WriteString(fmt.Sprintf("        proxy_connect_timeout %ds;\n", options.ConnectTimeoutSeconds))
	}
	if options.ReadTimeoutSeconds > 0 {
		b.WriteString(fmt.Sprintf("        proxy_read_timeout %ds;\n", options.ReadTimeoutSeconds))
	}
	if websocket {
		b.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		b.WriteString("        proxy_set_header Connection \"upgrade\";\n")
	}
	b.WriteString("    }\n")
}

func (m *Manager) validateCandidate(ctx context.Context, name string, content []byte) error {
	dir, err := os.MkdirTemp("", "nginx-manager-validate-*")
	if err != nil {
		return fmt.Errorf("create validation workspace: %w", err)
	}
	defer os.RemoveAll(dir)
	candidate := filepath.Join(dir, name+".conf")
	if err := os.WriteFile(candidate, content, 0o600); err != nil {
		return fmt.Errorf("write candidate: %w", err)
	}
	var config strings.Builder
	config.WriteString("worker_processes 1;\n")
	config.WriteString("pid " + nginxQuote(filepath.Join(dir, "nginx.pid")) + ";\n")
	config.WriteString("error_log stderr;\n")
	config.WriteString("events { worker_connections 64; }\n")
	config.WriteString("http {\n")
	if m.Layout.MimeTypes != "" {
		config.WriteString("    include " + nginxQuote(m.Layout.MimeTypes) + ";\n")
	}
	config.WriteString("    include " + nginxQuote(candidate) + ";\n")
	config.WriteString("}\n")
	stageMain := filepath.Join(dir, "nginx.conf")
	if err := os.WriteFile(stageMain, []byte(config.String()), 0o600); err != nil {
		return fmt.Errorf("write staging nginx.conf: %w", err)
	}
	output, runErr := m.Runner.Run(ctx, m.Runtime.Path, "-t", "-c", stageMain)
	if runErr != nil {
		return fmt.Errorf("candidate nginx -t failed: %s", output)
	}
	return nil
}

func nginxQuote(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
}

func (m *Manager) applySite(ctx context.Context, name string, content []byte) (ApplyResult, error) {
	target := filepath.Join(m.Layout.AvailableDir, name)
	enabled := filepath.Join(m.Layout.EnabledDir, name)
	if err := atomicfile.Write(target, content, 0o644); err != nil {
		return ApplyResult{}, fmt.Errorf("write site: %w", err)
	}
	enabledCreated := false
	rollback := func() {
		if enabledCreated && enabled != target {
			_ = os.Remove(enabled)
		}
		_ = os.Remove(target)
	}
	if m.Layout.Mode == "sites-enabled" {
		rel, err := filepath.Rel(m.Layout.EnabledDir, target)
		if err != nil {
			rollback()
			return ApplyResult{}, err
		}
		if err := os.Symlink(rel, enabled); err != nil {
			rollback()
			return ApplyResult{}, fmt.Errorf("enable site: %w", err)
		}
		enabledCreated = true
	}
	output, testErr := m.Runner.Run(ctx, m.Runtime.Path, "-t")
	if testErr != nil {
		rollback()
		return ApplyResult{}, fmt.Errorf("live nginx -t failed; rolled back: %s", output)
	}
	reloadOutput, reloadErr := m.Runner.Run(ctx, m.Runtime.Path, "-s", "reload")
	if reloadErr != nil {
		rollback()
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = m.Runner.Run(rollbackCtx, m.Runtime.Path, "-t")
		_, _ = m.Runner.Run(rollbackCtx, m.Runtime.Path, "-s", "reload")
		return ApplyResult{}, fmt.Errorf("nginx reload failed; rolled back: %s", reloadOutput)
	}
	return ApplyResult{
		Site:       parseManagedSite(name, target, true, content),
		TestOutput: output,
	}, nil
}
