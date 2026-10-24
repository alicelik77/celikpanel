package main

import (
	"errors"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// PHPToolBindingsV1 reads, without changing anything, which PHP version
// each tool is served from and the installed PHP-FPM versions (alpha.104).
// PHPToolBindingsV1, her aracın hangi PHP sürümünden sunulduğunu salt-okur
// bildirir.
func (a *Agent) PHPToolBindingsV1(_ *transport.Empty, resp *transport.PHPToolBindingsResponse) error {
	*resp = phpToolBindingsView()
	return nil
}

// BindPHPToolV1 moves one tool to one installed PHP version: the tool's
// extensions are ensured for it, its nginx configuration is rewritten and
// validated, nginx reloads, and the tool must answer as itself through its
// own socket; otherwise the previous configuration comes back (alpha.104).
// BindPHPToolV1, bir aracı kurulu bir PHP sürümüne taşır ve orada yanıt
// verdiğini kanıtlar; kanıt olmazsa önceki yapılandırma geri gelir.
func (a *Agent) BindPHPToolV1(req *transport.BindPHPToolRequest, resp *transport.BindPHPToolResponse) error {
	*resp = transport.BindPHPToolResponse{}
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	tool := strings.TrimSpace(req.Tool)
	version := strings.TrimSpace(req.Version)
	resp.Tool, resp.Version = tool, version
	if !validPHPTool(tool) || !phpVersionText.MatchString(version) {
		resp.Error, resp.ErrorCode = "invalid PHP tool or version", errCodePHPToolUnsupported
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepBindPHPTool, tool, version, "bind"),
	)
	if err != nil {
		*resp = transport.BindPHPToolResponse{Tool: tool, Version: version, Error: err.Error()}
		return nil
	}
	defer finishStep()
	result, err := bindPHPToolWith(ctx, hostPHPToolBindOps(), tool, version)
	resp.Previous = result.Previous
	resp.InstalledPackages = result.InstalledPackages
	resp.MutationApplied = result.MutationApplied
	resp.Restored = result.Restored
	resp.Warning = result.RecordWarning
	if err != nil {
		resp.Error = err.Error()
		var typed *phpToolBindError
		if errors.As(err, &typed) {
			resp.ErrorCode = typed.Code
			resp.Detail = typed.Detail
			cause := typed.Cause
			resp.Cause = &cause
		}
		return nil
	}
	resp.Bound = true
	// A proof passed: a failed proof after the install no longer stands.
	if err := writePHPToolProof(tool, nil, false); err != nil && resp.Warning == "" {
		resp.Warning = err.Error()
	}
	return nil
}
