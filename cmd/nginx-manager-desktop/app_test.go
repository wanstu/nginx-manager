package main

import (
	"strings"
	"testing"
	"time"
)

func TestApplyTrafficSummary(t *testing.T) {
	result := ServerOverview{}
	applyTrafficSummary(&result, RemoteLogTail{
		File: RemoteLogFile{Kind: "access", Path: "/var/log/nginx/access.log"},
		Content: "127.0.0.1 - - [20/Sep/2026:13:00:00 +0800] \"GET /ok HTTP/1.1\" 200 120 \"-\" \"ua\"\n" +
			"127.0.0.1 - - [20/Sep/2026:13:00:01 +0800] \"GET /redirect HTTP/1.1\" 302 10 \"-\" \"ua\"\n" +
			"127.0.0.1 - - [20/Sep/2026:13:00:02 +0800] \"GET /missing HTTP/1.1\" 404 33 \"-\" \"ua\"\n" +
			"127.0.0.1 - - [20/Sep/2026:13:00:03 +0800] \"GET /error HTTP/1.1\" 502 7 \"-\" \"ua\"\n" +
			"custom unparsed line",
	})

	if result.TrafficSampleLines != 5 ||
		result.TrafficParsedLines != 4 ||
		result.TrafficUnparsedLines != 1 ||
		result.Status2xx != 1 ||
		result.Status3xx != 1 ||
		result.Status4xx != 1 ||
		result.Status5xx != 1 ||
		result.ResponseBytes != 170 {
		t.Fatalf("unexpected traffic summary: %+v", result)
	}
}

func TestSelectOverviewAccessLogsPrioritizesSiteLogs(t *testing.T) {
	logs := []RemoteLogFile{
		{ID: "global", Kind: "access", Path: "/var/log/nginx/access.log"},
		{ID: "site-b", Kind: "access", Path: "/var/log/nginx/nginx-manager/b.example.com.access.log"},
		{ID: "error", Kind: "error", Path: "/var/log/nginx/error.log"},
		{ID: "site-a", Kind: "access", Path: "/var/log/nginx/nginx-manager/a.example.com.access.log"},
	}
	sites := []RemoteSite{
		{ServerName: "a.example.com", AccessLog: "/var/log/nginx/nginx-manager/a.example.com.access.log"},
		{ServerName: "b.example.com", AccessLog: "/var/log/nginx/nginx-manager/b.example.com.access.log"},
	}

	selected, available := selectOverviewAccessLogs(logs, sites, 2)
	if available != 3 || len(selected) != 2 {
		t.Fatalf("selected=%+v available=%d", selected, available)
	}
	if selected[0].ID != "site-a" || selected[1].ID != "site-b" {
		t.Fatalf("managed logs were not prioritized: %+v", selected)
	}
}

func TestSiteTrafficUsesDedicatedLogWithoutHostname(t *testing.T) {
	result := ServerOverview{}
	sites := []RemoteSite{{
		ServerName: "one.example.com",
		AccessLog:  "/var/log/nginx/nginx-manager/one.example.com.access.log",
	}}
	dedicated := []string{
		"1.1.1.1 - - [20/Sep/2026:13:00:00 +0800] \"GET /ok HTTP/1.1\" 200 10 \"-\" \"ua\"",
		"1.1.1.1 - - [20/Sep/2026:13:00:01 +0800] \"GET /bad HTTP/1.1\" 500 5 \"-\" \"ua\"",
	}
	appendSiteTrafficSummaryLines(
		&result,
		dedicated,
		sites,
		"/var/log/nginx/nginx-manager/one.example.com.access.log",
	)
	if len(result.SiteTraffic) != 1 ||
		result.SiteTraffic[0].MatchedLines != 2 ||
		result.SiteTraffic[0].Status2xx != 1 ||
		result.SiteTraffic[0].Status5xx != 1 ||
		result.SiteTraffic[0].ResponseBytes != 15 {
		t.Fatalf("dedicated summary = %+v", result.SiteTraffic)
	}

	appendSiteTrafficSummaryLines(
		&result,
		[]string{"1.1.1.1 - - [20/Sep/2026:13:00:02 +0800] \"GET /global HTTP/1.1\" 200 99 \"-\" \"ua\" host=one.example.com"},
		sites,
		"/var/log/nginx/access.log",
	)
	if result.SiteTraffic[0].MatchedLines != 2 || result.SiteTraffic[0].ResponseBytes != 15 {
		t.Fatalf("global log double-counted dedicated site: %+v", result.SiteTraffic[0])
	}
}

func TestTrafficLinesForWindow(t *testing.T) {
	since, err := time.Parse(time.RFC3339, "2026-09-20T12:30:00+08:00")
	if err != nil {
		t.Fatal(err)
	}
	content := "1.1.1.1 - - [20/Sep/2026:12:29:59 +0800] \"GET /old HTTP/1.1\" 200 1\n" +
		"1.1.1.1 - - [20/Sep/2026:12:30:00 +0800] \"GET /edge HTTP/1.1\" 200 2\n" +
		"1.1.1.1 - - [20/Sep/2026:13:00:00 +0800] \"GET /new HTTP/1.1\" 404 3\n" +
		"unparsed timestamp line"

	lines, readLines, unknown := trafficLinesForWindow(content, since)
	if readLines != 4 || unknown != 1 || len(lines) != 2 {
		t.Fatalf("window result: read=%d unknown=%d lines=%q", readLines, unknown, lines)
	}
	if !strings.Contains(lines[0], "/edge") || !strings.Contains(lines[1], "/new") {
		t.Fatalf("unexpected filtered lines: %q", lines)
	}

	result := ServerOverview{}
	applyTrafficSummaryLines(&result, "/var/log/nginx/access.log", lines, readLines, unknown)
	if result.TrafficReadLines != 4 ||
		result.TrafficSampleLines != 2 ||
		result.TrafficTimestampUnknown != 1 ||
		result.Status2xx != 1 ||
		result.Status4xx != 1 ||
		result.ResponseBytes != 5 {
		t.Fatalf("window summary = %+v", result)
	}
}

func TestApplySiteTrafficSummaries(t *testing.T) {
	result := ServerOverview{}
	tail := RemoteLogTail{
		Content: "1.1.1.1 - - [20/Sep/2026:13:00:00 +0800] \"GET / HTTP/1.1\" 200 100 \"-\" \"ua\" host=one.example.com\n" +
			"1.1.1.1 - - [20/Sep/2026:13:00:01 +0800] \"GET /x HTTP/1.1\" 404 20 \"-\" \"ua\" host=one.example.com\n" +
			"1.1.1.1 - - [20/Sep/2026:13:00:02 +0800] \"GET / HTTP/1.1\" 502 5 \"-\" \"ua\" host=two.example.com\n" +
			"1.1.1.1 - - [20/Sep/2026:13:00:03 +0800] \"GET / HTTP/1.1\" 200 10 \"https://notone.example.com/\" \"ua\"",
	}
	sites := []RemoteSite{
		{ServerName: "one.example.com"},
		{ServerName: "two.example.com"},
		{ServerName: "missing.example.com"},
	}
	applySiteTrafficSummaries(&result, tail, sites)

	if len(result.SiteTraffic) != 2 {
		t.Fatalf("site traffic = %+v", result.SiteTraffic)
	}
	one := result.SiteTraffic[0]
	if one.ServerName != "one.example.com" ||
		one.MatchedLines != 2 ||
		one.ParsedLines != 2 ||
		one.Status2xx != 1 ||
		one.Status4xx != 1 ||
		one.ResponseBytes != 120 {
		t.Fatalf("one summary = %+v", one)
	}
	two := result.SiteTraffic[1]
	if two.ServerName != "two.example.com" || two.MatchedLines != 1 || two.Status5xx != 1 {
		t.Fatalf("two summary = %+v", two)
	}
}

func TestApplyFleetCertificateHealth(t *testing.T) {
	now, err := time.Parse(time.RFC3339, "2026-09-20T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	result := FleetServerOverview{Warnings: []string{}}
	sites := []RemoteSite{
		{ServerName: "ok.example.com", HTTPS: true},
		{ServerName: "missing.example.com", HTTPS: true},
		{ServerName: "http.example.com", HTTPS: false},
	}
	certificates := CertificateListResult{
		Certificates: []RemoteCertificate{
			{Name: "ok.example.com", NotAfter: "2026-12-01T00:00:00Z"},
			{Name: "expired.example.com", NotAfter: "2026-09-19T00:00:00Z"},
			{Name: "soon.example.com", NotAfter: "2026-10-01T00:00:00Z"},
		},
		Certbot:      RemoteCertbotStatus{Available: true},
		RenewalTimer: RemoteRenewalTimerStatus{Enabled: true, Active: true},
	}
	applyFleetCertificateHealth(&result, sites, certificates, now)
	if result.Certificates != 3 ||
		result.CertificatesExpired != 1 ||
		result.CertificatesExpiring != 1 ||
		result.HTTPSWithoutCertificate != 1 {
		t.Fatalf("fleet certificate health = %+v", result)
	}
	if len(result.Warnings) != 3 {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestAggregateFleetServers(t *testing.T) {
	result := aggregateFleetServers([]FleetServerOverview{
		{Status: "healthy", TotalSites: 3, HTTPSSites: 2},
		{Status: "attention", TotalSites: 5, HTTPSSites: 1, CertificatesExpiring: 2, HTTPSWithoutCertificate: 1},
		{Status: "unreachable", CertificatesExpired: 1},
	})
	if result.Total != 3 || result.Healthy != 1 || result.Attention != 1 || result.Unreachable != 1 {
		t.Fatalf("fleet totals = %+v", result)
	}
	if result.TotalSites != 8 || result.HTTPSSites != 3 || result.CertificatesExpired != 1 ||
		result.CertificatesExpiring != 2 || result.HTTPSWithoutCertificate != 1 {
		t.Fatalf("fleet aggregate = %+v", result)
	}
}

func TestFleetCapabilitySupported(t *testing.T) {
	if !fleetCapabilitySupported(TestResult{CapabilitiesKnown: false}, "sites_read") {
		t.Fatal("legacy CLI should use compatibility mode")
	}
	if fleetCapabilitySupported(TestResult{CapabilitiesKnown: true, APICompatible: false, Capabilities: []string{"sites_read"}}, "sites_read") {
		t.Fatal("newer incompatible API should not be queried")
	}
	if !fleetCapabilitySupported(TestResult{CapabilitiesKnown: true, APICompatible: true, Capabilities: []string{"sites_read"}}, "sites_read") {
		t.Fatal("declared capability should be supported")
	}
}

func TestCertificateCoversHost(t *testing.T) {
	certificate := RemoteCertificate{
		Name:    "example.com",
		Domains: []string{"example.com", "*.example.net"},
	}
	cases := []struct {
		host string
		want bool
	}{
		{"example.com", true},
		{"EXAMPLE.COM", true},
		{"api.example.net", true},
		{"deep.api.example.net", false},
		{"example.net", false},
		{"other.example.org", false},
	}
	for _, tc := range cases {
		if got := certificateCoversHost(certificate, tc.host); got != tc.want {
			t.Fatalf("certificateCoversHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestAPIVersionCompatible(t *testing.T) {
	cases := []struct {
		version int
		want    bool
	}{
		{-1, true},
		{0, true},
		{1, true},
		{2, false},
		{99, false},
	}
	for _, tc := range cases {
		if got := apiVersionCompatible(tc.version); got != tc.want {
			t.Fatalf("apiVersionCompatible(%d) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://example.com/", "https://example.com", false},
		{"http://127.0.0.1:8020/", "http://127.0.0.1:8020", false},
		{"http://localhost:8020", "http://localhost:8020", false},
		{"http://192.168.1.20:8020", "", true},
		{"http://example.com", "", true},
		{"ftp://example.com", "", true},
	}
	for _, tt := range tests {
		got, err := normalizeEndpoint(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("normalizeEndpoint(%q) succeeded: %q", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("normalizeEndpoint(%q) = %q, %v; want %q, nil", tt.in, got, err, tt.want)
		}
	}
}
