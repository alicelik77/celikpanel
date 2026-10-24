package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// Every decision is recorded (alpha.107, D-038 §16): an install, a removal,
// a move of a PHP tool, a version choice records what it was about in
// audit_logs.details (migration 45): the component, the package, the
// version from and to, what was confirmed (notable installs, a version that
// no longer receives security fixes, a move) and, for a version, its
// security support at the time.
//
// Her karar kaydedilir: kurulum, kaldırma, taşıma ve sürüm seçimi neyle ilgili
// olduğunu denetim kaydının ayrıntısına yazar.

type auditDetailsKey struct{}

// withAuditDetails carries a row's details to an audit writer that only
// takes an action (the service operation runner's).
func withAuditDetails(ctx context.Context, details map[string]any) context.Context {
	if len(details) == 0 {
		return ctx
	}
	return context.WithValue(ctx, auditDetailsKey{}, details)
}

func auditDetailsFrom(ctx context.Context) map[string]any {
	details, _ := ctx.Value(auditDetailsKey{}).(map[string]any)
	return details
}

var (
	auditPHPPackage = regexp.MustCompile(`^php([0-9]+\.[0-9]+)-`)
	auditPGPackage  = regexp.MustCompile(`^postgresql-([0-9]+)$`)
)

// versionSupportDetails names a version and its security support, when the
// package or the runtime names one the table knows how to read.
func versionSupportDetails(serviceID, pkg string) map[string]any {
	component, version := "", ""
	switch {
	case serviceID == "node" || serviceID == "runtime_node":
		component, version = "node", pkg
	case auditPHPPackage.MatchString(pkg):
		component, version = "php", auditPHPPackage.FindStringSubmatch(pkg)[1]
	case auditPGPackage.MatchString(pkg):
		component, version = "postgresql", auditPGPackage.FindStringSubmatch(pkg)[1]
	}
	if component == "" || version == "" {
		return nil
	}
	support := supportStatus(component, version)
	details := map[string]any{"version": version, "support": support.Status}
	if support.Until != "" {
		details["support_until"] = support.Until
	}
	if support.Status == "unsupported" {
		// The interface asks before installing such a version (alpha.107).
		details["confirmed_unsupported"] = true
	}
	return details
}

// operationAuditDetails are an operation's details: the component and
// package, the PHP version a tool was installed for, the notable installs
// that were confirmed, a version's support.
func operationAuditDetails(op serviceOperation, result serviceOperationResult) map[string]any {
	details := map[string]any{"component": op.ServiceID, "operation": op.Kind}
	if op.PackageName != "" {
		details["package"] = op.PackageName
	}
	if strings.TrimSpace(op.OperationData) != "" {
		var data serviceInstallOperationData
		if json.Unmarshal([]byte(op.OperationData), &data) == nil {
			if data.PHPVersion != "" {
				details["php_version"] = data.PHPVersion
			}
			if len(data.ConfirmedInstalls) > 0 {
				details["confirmed_installs"] = data.ConfirmedInstalls
			}
		}
	}
	for key, value := range versionSupportDetails(op.ServiceID, op.PackageName) {
		details[key] = value
	}
	if version, ok := result["version"].(string); ok && version != "" {
		details["installed_version"] = version
	}
	return details
}

// auditHostingChange records a site's hosting change (alpha.107, D-038 §16):
// who, which site, the project type and the Node.js version from and to, and
// the chosen Node.js line's security support.
func (p *Panel) auditHostingChange(r *http.Request, domainID int, previous hostingRuntimeState, next hostingSettings) {
	fromType, toType := previous.ProjectType, next.ProjectType
	fromNode, toNode := "", ""
	if previous.ProjectType == "node" && previous.RuntimeVersion.Valid {
		fromNode = previous.RuntimeVersion.String
	}
	if next.ProjectType == "node" {
		toNode = strings.TrimSpace(next.RuntimeVersion)
	}
	if fromType == toType && fromNode == toNode {
		return
	}
	details := map[string]any{"site": previous.DomainName, "from_type": fromType, "to_type": toType}
	action := "site.hosting:" + previous.DomainName + ":" + fromType + "->" + toType
	if fromNode != toNode {
		details["component"] = "node"
		details["from"], details["to"] = fromNode, toNode
		action = "site.node_version:" + previous.DomainName + ":" + fromNode + "->" + toNode
		if toNode != "" {
			for key, value := range versionSupportDetails("node", toNode) {
				if key != "version" {
					details[key] = value
				}
			}
		}
	}
	p.auditDetailed(r, action, "domain", domainID, details)
}
