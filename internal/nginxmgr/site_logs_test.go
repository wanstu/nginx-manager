package nginxmgr

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedSiteLogPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NGINX_MANAGER_SITE_LOG_DIR", root)

	accessLog, errorLog, err := managedSiteLogPaths("api.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if accessLog != filepath.Join(root, "api.example.com.access.log") {
		t.Fatalf("access log = %q", accessLog)
	}
	if errorLog != filepath.Join(root, "api.example.com.error.log") {
		t.Fatalf("error log = %q", errorLog)
	}

	options, err := withManagedSiteLogs(ProxyOptions{MaxBodySizeMB: 8}, "api.example.com")
	if err != nil {
		t.Fatal(err)
	}
	config := string(renderReverseProxyManaged(
		"api.example.com",
		"http://127.0.0.1:8002",
		false,
		nil,
		"",
		options,
	))
	for _, expected := range []string{
		"access_log " + nginxQuote(accessLog) + ";",
		"error_log " + nginxQuote(errorLog) + ";",
		"client_max_body_size 8m;",
	} {
		if !strings.Contains(config, expected) {
			t.Fatalf("config missing %q:\n%s", expected, config)
		}
	}

	site := Site{}
	applySiteLogFields(&site, config)
	if site.AccessLog != accessLog || site.ErrorLog != errorLog {
		t.Fatalf("parsed log fields = %+v", site)
	}
}

func TestManagedSiteLogRootMustBeAbsolute(t *testing.T) {
	t.Setenv("NGINX_MANAGER_SITE_LOG_DIR", "relative/logs")
	if _, _, err := managedSiteLogPaths("api.example.com"); err == nil {
		t.Fatal("relative log root was accepted")
	}
}
