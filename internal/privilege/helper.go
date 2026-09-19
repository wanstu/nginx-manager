package privilege

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wanstu/nginx-manager/internal/nginxmgr"
)

const maxPrivilegedRequestBytes = 64 << 10

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
	switch request.Operation {
	case OperationProbe:
		if request.Create != nil || request.Update != nil || request.SiteID != "" || request.SnapshotID != "" || request.Limit != 0 {
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
	case OperationCreateReverseProxy:
		if request.Create == nil || request.Update != nil || request.SiteID != "" || request.SnapshotID != "" || request.Limit != 0 {
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
		if request.Create != nil || request.Update == nil || request.SiteID == "" || request.SnapshotID != "" || request.Limit != 0 {
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
		if request.Create != nil || request.Update != nil || request.SiteID == "" || request.SnapshotID != "" || request.Limit != 0 {
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
		if request.Create != nil || request.Update != nil || request.SiteID == "" || request.SnapshotID != "" || request.Limit != 0 {
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
		if request.Create != nil || request.Update != nil || request.SiteID != "" || request.SnapshotID != "" || request.Limit < 0 || request.Limit > 100 {
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
		if request.Create != nil || request.Update != nil || request.SiteID != "" || request.SnapshotID == "" || request.Limit != 0 {
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
	default:
		return fmt.Errorf("unsupported privileged operation %q", request.Operation)
	}

	encoder := json.NewEncoder(output)
	return encoder.Encode(response)
}
