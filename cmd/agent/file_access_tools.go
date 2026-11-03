package main

import (
	"context"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The command line's tools before the command line (alpha.118).
//
// The owner: "if some components must be installed for SSH, make sure they
// are installed before SSH is turned on." The jail copies the host's own
// programs; a program the host lacks cannot be in it. So the Access view
// reports the missing tools (read only, readFileAccessState), turning the
// command line on is refused while any is missing (buildFileAccessPlan),
// and the administrator installs exactly the official packages that provide
// them here, through the package lock and the operation's lease like every
// other package install. The packages stay the owner's: removing CelikPanel
// leaves them installed (D-022).
//
// Komut satırının araçları komut satırından önce: eksikler salt-okur
// bildirilir, eksik varken komut satırı açılmaz, yönetici tam o resmi
// paketleri kurar.

const (
	fileAccessToolsKind   = "file_access_tools"
	fileAccessToolsTarget = "file-access-tools"
)

// fileAccessInstallPackages is the package manager install; a seam for tests.
var fileAccessInstallPackages = installPackagesContext

// refuseMissingJailTools is the refusal that keeps the command line off
// while the server lacks some of its tools; nil when nothing is missing.
func refuseMissingJailTools() *fileAccessRefusal {
	missing := fileAccessJailMissing()
	if len(missing) == 0 {
		return nil
	}
	packages, _ := fileaccess.JailPackagesFor(fileAccessFamily(), missing)
	install := "the operating system's packages"
	if len(packages) > 0 {
		install = "the packages " + strings.Join(packages, ", ")
	}
	return refuseFileAccess(transport.FileAccessRefusedJailTools,
		"this server does not have these command-line tools: %s. The command line stays off until the server administrator installs %s (Access tab, Install the tools); then turn it on again. FTP and SFTP are not affected",
		strings.Join(missing, ", "), install)
}

// InstallJailToolsV1 installs the official packages that provide the
// missing tools, and only when they are the ones the administrator saw.
func (a *Agent) InstallJailToolsV1(req *transport.InstallJailToolsRequest, resp *transport.InstallJailToolsResponse) error {
	*resp = transport.InstallJailToolsResponse{}
	if req == nil {
		resp.ErrorCode, resp.Error = transport.JailToolsChanged, "missing request"
		return nil
	}
	family := fileAccessFamily()
	missing := fileAccessJailMissing()
	packages, unknown := fileaccess.JailPackagesFor(family, missing)
	resp.Packages = packages
	if len(unknown) > 0 {
		resp.StillMissing = missing
		resp.ErrorCode = transport.JailToolsUnsupported
		resp.Error = "CelikPanel knows no official package for " + strings.Join(unknown, ", ") +
			" on this system; the server administrator installs it with the system's package manager"
		return nil
	}
	requested := append([]string(nil), req.Packages...)
	sort.Strings(requested)
	for _, name := range requested {
		if !fileaccess.IsJailPackage(family, name) {
			resp.ErrorCode, resp.Error = transport.JailToolsChanged, "package "+name+" is not one of the command line's tools"
			return nil
		}
	}
	// Nothing missing any more (a repeated request after a lost answer):
	// the work is done and nothing is installed again.
	if len(packages) == 0 {
		return nil
	}
	qualifier := fileaccess.JailToolsQualifier(packages)
	if fileaccess.JailToolsQualifier(requested) != qualifier {
		resp.StillMissing = missing
		resp.ErrorCode = transport.JailToolsChanged
		resp.Error = "the missing tools changed since the page was read (now: " + strings.Join(packages, ", ") + "); nothing was installed"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepInstallJailTools, fileAccessToolsTarget, qualifier, "install"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = transport.JailToolsMutation, err.Error()
		return nil
	}
	defer finishStep()
	return installJailPackages(ctx, family, packages, resp)
}

func installJailPackages(ctx context.Context, family string, packages []string, resp *transport.InstallJailToolsResponse) error {
	if _, err := fileAccessInstallPackages(ctx, family, packages); err != nil {
		resp.ErrorCode, resp.Error = transport.JailToolsRefused, err.Error()
		resp.StillMissing = fileAccessJailMissing()
		return nil
	}
	resp.Installed = packages
	resp.StillMissing = fileAccessJailMissing()
	return nil
}
