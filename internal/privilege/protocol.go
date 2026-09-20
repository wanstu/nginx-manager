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
	OperationListLogs           = "list_logs"
	OperationTailLog            = "tail_log"
	OperationTailLogs           = "tail_logs"
	OperationSafeReload         = "safe_reload"
	OperationUpdateSiteTLS      = "update_site_tls"
)

type ApplyRequest struct {
	Operation  string
	Create     *nginxmgr.ReverseProxyRequest
	Update     *nginxmgr.UpdateReverseProxyRequest
	Issue      *nginxmgr.IssueCertificateRequest
	TLS        *nginxmgr.UpdateTLSRequest
	SiteID     string
	Enabled    bool
	SnapshotID string
	Limit      int
	LogID      string
	LogIDs     []string
	Lines      int
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
	RenewalTimer *nginxmgr.RenewalTimerStatus
	Output       string
	Logs         []nginxmgr.LogFile
	LogTail      *nginxmgr.LogTail
	LogTails     []nginxmgr.LogTailResult
	Reload       *nginxmgr.ReloadResult
}
