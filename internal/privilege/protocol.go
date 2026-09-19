package privilege

import "github.com/wanstu/nginx-manager/internal/nginxmgr"

const (
	OperationProbe              = "probe"
	OperationCreateReverseProxy = "create_reverse_proxy"
	OperationSetSiteEnabled     = "set_site_enabled"
	OperationDeleteSite         = "delete_site"
)

type ApplyRequest struct {
	Operation string
	Create    *nginxmgr.ReverseProxyRequest
	SiteID    string
	Enabled   bool
}

type ApplyResponse struct {
	OK         bool
	Error      string
	Apply      *nginxmgr.ApplyResult
	Site       *nginxmgr.Site
	Runtime    *nginxmgr.RuntimeInfo
	Layout     *nginxmgr.Layout
	ConfigOK   bool
	TestOutput string
}
