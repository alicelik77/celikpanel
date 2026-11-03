package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The SSH command line's tools before the command line (alpha.118).
//
// The owner: "if some components must be installed for SSH, make sure they
// are installed before SSH is turned on." The Access view reports the tools
// the server lacks (read only, from Agent.FileAccessStateV1); while any is
// missing the command line is not turned on (here and in the Agent), and
// the administrator installs exactly the official packages that provide
// them with this one action: an administrator-only, single-flight host
// mutation in its own durable Agent job (file_access_tools), audited as a
// package install. A lost answer is safe: the page reads the server again,
// and a repeated request finds nothing missing and installs nothing.
//
// SSH komut satırının araçları komut satırından önce: eksikler salt-okur
// bildirilir, eksik varken komut satırı açılmaz; yönetici tek tıkla tam o
// resmi paketleri kurar.

const (
	fileAccessToolsPath   = "/api/v1/service/file-access-tools"
	fileAccessToolsKind   = "file_access_tools"
	fileAccessToolsTarget = "file-access-tools"

	errCodeFileAccessSSHToolsMissing = "FILE_ACCESS_SSH_TOOLS_MISSING"
	errCodeFileAccessToolsChanged    = "FILE_ACCESS_TOOLS_CHANGED"
	errCodeFileAccessToolsUnknown    = "FILE_ACCESS_TOOLS_UNSUPPORTED"
	errCodeFileAccessToolsFailed     = "FILE_ACCESS_TOOLS_FAILED"
)

// sshToolsMissingRefusal is the server's own check before the command line
// is turned on: the screen hides the switch, this refuses the request. An
// account that already has the command line keeps it.
func sshToolsMissingRefusal(change transport.ApplyFileAccessRequest, before transport.FileAccessStateResponse) (string, []string, bool) {
	if !change.SSH || before.Account.SSH || len(before.Server.JailMissing) == 0 {
		return "", nil, false
	}
	message := "this server does not have these command-line tools: " + strings.Join(before.Server.JailMissing, ", ") +
		". The command line stays off until the server administrator installs them (Access tab, Install the tools); then turn it on again. FTP and SFTP are not affected"
	return message, before.Server.JailMissing, true
}

var installJailToolsThroughAgent = func(p *Panel, ctx context.Context, packages []string) (transport.InstallJailToolsResponse, error) {
	var response transport.InstallJailToolsResponse
	err := p.withStandaloneAgentMutation(ctx, fileAccessToolsKind, fileAccessToolsTarget, fileaccess.JailToolsQualifier(packages),
		func(callCtx context.Context, binding agentMutationBinding) error {
			request := transport.InstallJailToolsRequest{ServiceMutationBinding: binding, Packages: packages}
			if err := p.callAgentContext(callCtx, "Agent.InstallJailToolsV1", &request, &response); err != nil {
				return err
			}
			if response.Error != "" {
				return errors.New(response.Error)
			}
			return nil
		})
	if err != nil && response.Error == "" {
		return response, err
	}
	return response, nil
}

// agentLacksMethod: an Agent older than this Panel does not know the call
// (the paired update is not finished).
func agentLacksMethod(err error) bool {
	return err != nil && strings.Contains(err.Error(), "can't find method")
}

func (p *Panel) handleFileAccessTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if caller := currentCaller(r); caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "only the server administrator installs packages", "")
		return
	}
	if r.Method != http.MethodPost {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !p.requireSubsystemOperational(w, degradedSubsystemServiceOperations) {
		return
	}
	var req struct {
		Packages []string `json:"packages"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || len(req.Packages) == 0 || len(req.Packages) > 32 {
		writeClientError(w, http.StatusBadRequest, "the packages that were shown are required")
		return
	}
	packages := append([]string(nil), req.Packages...)
	sort.Strings(packages)
	for i, name := range packages {
		if !installPreviewPackagePattern.MatchString(name) || (i > 0 && packages[i-1] == name) {
			writeClientError(w, http.StatusBadRequest, "invalid package name")
			return
		}
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()
	details := map[string]any{"packages": packages, "purpose": "ssh command line"}
	action := "service.install:" + fileaccess.JailToolsQualifier(packages)
	response, err := installJailToolsThroughAgent(p, r.Context(), packages)
	if err != nil {
		if agentLacksMethod(err) {
			writeCodedError(w, http.StatusConflict, errCodeFileAccessAgentUpgrade,
				"the server's Agent cannot install these tools yet; finish the panel update first", "")
			return
		}
		p.auditDetailed(r, "service.install.failed:"+fileaccess.JailToolsQualifier(packages)+" — "+auditReason(err.Error()), "service", 0, details)
		writeAgentError(w, err, "command-line tools install")
		return
	}
	if response.ErrorCode != "" {
		details["code"] = response.ErrorCode
		code, status := errCodeFileAccessToolsFailed, http.StatusConflict
		switch response.ErrorCode {
		case transport.JailToolsChanged:
			code = errCodeFileAccessToolsChanged
		case transport.JailToolsUnsupported:
			code = errCodeFileAccessToolsUnknown
		}
		if response.ErrorCode != transport.JailToolsChanged {
			p.auditDetailed(r, "service.install.failed:"+fileaccess.JailToolsQualifier(packages)+" — "+auditReason(response.Error), "service", 0, details)
		}
		writeCodedErrorDetails(w, status, code, response.Error, "", response.StillMissing)
		return
	}
	if len(response.Installed) > 0 {
		details["still_missing"] = response.StillMissing
		p.auditDetailed(r, action, "service", 0, details)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"installed": response.Installed, "still_missing": response.StillMissing,
	})
}
