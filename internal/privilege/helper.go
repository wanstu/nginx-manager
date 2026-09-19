package privilege

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/wanstu/nginx-manager/internal/nginxmgr"
)

const maxPrivilegedRequestBytes = 64 << 10

func RunPrivilegedRenewCertificates(ctx context.Context, output io.Writer) error {
	if err := requireRoot(); err != nil {
		return err
	}
	manager, err := nginxmgr.New(ctx)
	if err != nil {
		return err
	}
	result, err := manager.RenewCertificates(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result) != "" {
		_, err = fmt.Fprintln(output, result)
	}
	return err
}

func RunPrivilegedApply(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := requireRoot(); err != nil {
		return err
	}

	decoder := json.NewDecoder(io.LimitReader(input, maxPrivilegedRequestBytes))
	decoder.DisallowUnknownFields()

	var request ApplyRequest
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("decode privileged request: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("privileged request must contain exactly one JSON value")
	}

	response := ApplyResponse{}
	hasLogArgs := request.LogID != "" || request.Lines != 0
	switch request.Operation {
	case OperationProbe:
		if request.Create != nil || request.Update != nil || request.Issue != nil || request.SiteID != "" || request.SnapshotID != "" || request.Limit != 0 || hasLogArgs {
			return errors.New("invalid probe request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		ok, output := manager.Status(ctx)
		response.OK = true
		response.Runtime = &manager.Runtime
		response.Layout = &manager.Layout
		response.ConfigOK = ok
		response.TestOutput = output
		certbot := nginxmgr.DetectCertbot(ctx)
		response.Certbot = &certbot
		renewalTimer := nginxmgr.DetectRenewalTimer(ctx)
		response.RenewalTimer = &renewalTimer
	case OperationCreateReverseProxy:
		if request.Create == nil || request.Update != nil || request.Issue != nil || request.SiteID != "" || request.SnapshotID != "" || request.Limit != 0 || hasLogArgs {
			return errors.New("invalid create_reverse_proxy request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		result, err := manager.CreateReverseProxy(ctx, *request.Create)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.Apply = &result
	case OperationUpdateReverseProxy:
		if request.Create != nil || request.Update == nil || request.Issue != nil || request.SiteID == "" || request.SnapshotID != "" || request.Limit != 0 || hasLogArgs {
			return errors.New("invalid update_reverse_proxy request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		result, err := manager.UpdateReverseProxy(ctx, request.SiteID, *request.Update)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.Apply = &result
	case OperationSetSiteEnabled:
		if request.Create != nil || request.Update != nil || request.Issue != nil || request.SiteID == "" || request.SnapshotID != "" || request.Limit != 0 || hasLogArgs {
			return errors.New("invalid set_site_enabled request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		site, err := manager.SetSiteEnabled(ctx, request.SiteID, request.Enabled)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.Site = &site
	case OperationDeleteSite:
		if request.Create != nil || request.Update != nil || request.Issue != nil || request.SiteID == "" || request.SnapshotID != "" || request.Limit != 0 || hasLogArgs {
			return errors.New("invalid delete_site request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		site, err := manager.DeleteSite(ctx, request.SiteID)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.Site = &site
	case OperationListSnapshots:
		if request.Create != nil || request.Update != nil || request.Issue != nil || request.SiteID != "" || request.SnapshotID != "" || request.Limit < 0 || request.Limit > 100 || hasLogArgs {
			return errors.New("invalid list_snapshots request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		snapshots, err := manager.ListSnapshots(request.Limit)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.Snapshots = snapshots
	case OperationRestoreSnapshot:
		if request.Create != nil || request.Update != nil || request.Issue != nil || request.SiteID != "" || request.SnapshotID == "" || request.Limit != 0 || hasLogArgs {
			return errors.New("invalid restore_snapshot request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		site, err := manager.RestoreSnapshot(ctx, request.SnapshotID)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.Site = &site
	case OperationListCertificates:
		if request.Create != nil || request.Update != nil || request.Issue != nil || request.SiteID != "" || request.SnapshotID != "" || request.Limit != 0 || hasLogArgs {
			return errors.New("invalid list_certificates request")
		}
		certificates, err := nginxmgr.ListCertificates()
		if err != nil {
			response.Error = err.Error()
			break
		}
		certbot := nginxmgr.DetectCertbot(ctx)
		renewalTimer := nginxmgr.DetectRenewalTimer(ctx)
		response.OK = true
		response.Certificates = certificates
		response.Certbot = &certbot
		response.RenewalTimer = &renewalTimer
	case OperationIssueCertificate:
		if request.Create != nil || request.Update != nil || request.Issue == nil || request.SiteID == "" || request.SnapshotID != "" || request.Limit != 0 || hasLogArgs {
			return errors.New("invalid issue_certificate request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		site, err := manager.IssueCertificate(ctx, request.SiteID, *request.Issue)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.Site = &site
	case OperationRenewCertificates:
		if request.Create != nil || request.Update != nil || request.Issue != nil || request.SiteID != "" || request.SnapshotID != "" || request.Limit != 0 || hasLogArgs {
			return errors.New("invalid renew_certificates request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		output, err := manager.RenewCertificates(ctx)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.Output = output
	case OperationListLogs:
		if request.Create != nil || request.Update != nil || request.Issue != nil ||
			request.SiteID != "" || request.SnapshotID != "" || request.Limit != 0 ||
			request.LogID != "" || request.Lines != 0 {
			return errors.New("invalid list_logs request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		logs, err := manager.ListLogs(ctx)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.Logs = logs
	case OperationTailLog:
		if request.Create != nil || request.Update != nil || request.Issue != nil ||
			request.SiteID != "" || request.SnapshotID != "" || request.Limit != 0 ||
			request.LogID == "" || request.Lines < 1 || request.Lines > nginxmgr.MaxLogLines {
			return errors.New("invalid tail_log request")
		}
		manager, err := nginxmgr.New(ctx)
		if err != nil {
			return err
		}
		tail, err := manager.TailLog(ctx, request.LogID, request.Lines)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.OK = true
		response.LogTail = &tail
	default:
		return fmt.Errorf("unsupported privileged operation %q", request.Operation)
	}

	encoder := json.NewEncoder(output)
	return encoder.Encode(response)
}
