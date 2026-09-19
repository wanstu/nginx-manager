package privilege

import "github.com/wanstu/nginx-manager/internal/nginxmgr"

const (
	OperationProbe              = "probe"
	OperationCreateReverseProxy = "create_reverse_proxy"
	OperationUpdateReverseProxy = "update_reverse_proxy"
	OperationSetSiteEnabled     = "set_site_enabled"
	OperationDeleteSite         = "delete_site"
	OperationListSnapshots      = "list_snapshots"
	OperationRestoreSnapshot    = "restore_snapshot"
	OperationListCertificates   = "list_certificates"
	OperationIssueCertificate   = "issue_certificate"
	OperationRenewCertificates  = "renew_certificates"
)

type ApplyRequest struct {
	Operation  string
	Create     *nginxmgr.ReverseProxyRequest
	Update     *nginxmgr.UpdateReverseProxyRequest
	Issue      *nginxmgr.IssueCertificateRequest
	SiteID     string
	Enabled    bool
	SnapshotID string
	Limit      int
}

type ApplyResponse struct {
	OK           bool
	Error        string
	Apply        *nginxmgr.ApplyResult
	Site         *nginxmgr.Site
	Runtime      *nginxmgr.RuntimeInfo
	Layout       *nginxmgr.Layout
	ConfigOK     bool
	TestOutput   string
	Snapshots    []nginxmgr.SnapshotMeta
	Certificates []nginxmgr.Certificate
	Certbot      *nginxmgr.CertbotStatus
	Output       string
}
