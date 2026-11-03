package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// A withdrawn version or engine cannot be installed until it is re-enabled
// (alpha.117, D-043 release decision b): a PHP version's packages, a Node.js
// runtime, a database engine. The refusal names what is withdrawn, who acts
// and what comes next (D-024); nothing is queued.
//
// Geri çekilen sürüm ya da motor yeniden açılana dek kurulamaz.

// errCodeSecurityEmergencyWithdrawn: an install of something an open
// security emergency withdrew.
const errCodeSecurityEmergencyWithdrawn = "SECURITY_EMERGENCY_WITHDRAWN"

// withdrawnInstallName names what an install would add that an open
// security emergency withdrew, or "" when nothing is withdrawn. pkg is the
// package (php8.5-fpm, php8.5-curl) or, for a Node.js runtime, the version.
func withdrawnInstallName(withdrawn map[string]map[string]bool, serviceID, pkg, phpVersion string) string {
	if serviceID == "node" {
		if versionWithdrawn(withdrawn, "node", pkg) {
			return "Node.js " + pkg
		}
		return ""
	}
	switch serviceID {
	case "mariadb", "postgresql":
		if versionWithdrawn(withdrawn, serviceID, "*") {
			return emergencyComponents[serviceID].Name
		}
	}
	if match := auditPHPPackage.FindStringSubmatch(pkg); match != nil && versionWithdrawn(withdrawn, "php", match[1]) {
		return "PHP " + match[1]
	}
	if phpVersion != "" && versionWithdrawn(withdrawn, "php", phpVersion) {
		return "PHP " + phpVersion
	}
	return ""
}

// withdrawnInstall reads the open emergencies for one install.
func (p *Panel) withdrawnInstall(ctx context.Context, serviceID, pkg, phpVersion string) (string, error) {
	withdrawn, err := withdrawnVersions(ctx, p.db.GetDB())
	if err != nil {
		return "", err
	}
	return withdrawnInstallName(withdrawn, serviceID, pkg, phpVersion), nil
}

func withdrawnInstallMessage(name string) string {
	return "Nothing was installed: " + name + " is withdrawn for security on this server. " +
		"The administrator re-enables it on the Security emergency screen once a patched version exists, then installs it again."
}

// refuseWithdrawnInstall answers an install of a withdrawn version or
// engine with the coded refusal.
func (p *Panel) refuseWithdrawnInstall(w http.ResponseWriter, r *http.Request, serviceID, pkg, phpVersion string) bool {
	name, err := p.withdrawnInstall(r.Context(), serviceID, pkg, phpVersion)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if name == "" {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error: withdrawnInstallMessage(name),
		Code:  errCodeSecurityEmergencyWithdrawn,
		Vars:  map[string]string{"name": name},
	})
	return true
}

// withdrawnInstallFailure is the same refusal inside an install operation
// (a server setup step, a recovered operation).
func (p *Panel) withdrawnInstallFailure(ctx context.Context, serviceID, pkg, phpVersion string) *serviceOperationFailure {
	name, err := p.withdrawnInstall(ctx, serviceID, pkg, phpVersion)
	if err != nil {
		return serviceInstallFailure(err)
	}
	if name == "" {
		return nil
	}
	message := withdrawnInstallMessage(name)
	return operationFailure("security_emergency_withdrawn", message, errors.New(message))
}
