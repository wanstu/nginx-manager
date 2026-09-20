package nginxmgr

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type UpstreamHealth struct {
	SiteID     string `json:"site_id"`
	ServerName string `json:"server_name"`
	Upstream   string `json:"upstream"`
	Reachable  bool   `json:"reachable"`
	Healthy    bool   `json:"healthy"`
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

func (m *Manager) CheckUpstream(ctx context.Context, siteID string) (UpstreamHealth, error) {
	state, err := m.loadManagedSite(siteID)
	if err != nil {
		return UpstreamHealth{}, err
	}
	if strings.TrimSpace(state.Site.ProxyPass) == "" {
		return UpstreamHealth{}, errors.New("managed site is not a reverse proxy")
	}
	upstream, err := validateUpstream(state.Site.ProxyPass)
	if err != nil {
		return UpstreamHealth{}, err
	}
	target, err := url.Parse(upstream)
	if err != nil {
		return UpstreamHealth{}, err
	}

	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(checkCtx, http.MethodHead, target.String(), nil)
	if err != nil {
		return UpstreamHealth{}, err
	}
	request.Header.Set("User-Agent", "nginx-manager-upstream-check/1")

	dialer := &net.Dialer{Timeout: 3 * time.Second}
	client := &http.Client{
		Timeout: 4 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 3 * time.Second,
		},
	}

	started := time.Now()
	response, requestErr := client.Do(request)
	latency := time.Since(started).Milliseconds()
	if latency < 0 {
		latency = 0
	}

	result := UpstreamHealth{
		SiteID:     state.Site.ID,
		ServerName: state.Site.ServerName,
		Upstream:   target.String(),
		LatencyMS:  latency,
	}
	if requestErr != nil {
		result.Error = requestErr.Error()
		return result, nil
	}
	defer response.Body.Close()

	result.Reachable = true
	result.StatusCode = response.StatusCode
	result.Healthy = response.StatusCode >= 200 && response.StatusCode < 400
	if !result.Healthy {
		result.Error = "HTTP " + strconv.Itoa(response.StatusCode)
	}
	return result, nil
}
