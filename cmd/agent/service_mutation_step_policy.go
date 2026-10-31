package main

import (
	"errors"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/transport"
)

type serviceMutationStepMethod string

const (
	serviceMutationStepIssueMailHostCertificate                           = "Agent.IssueMailHostCertificateV1"
	serviceMutationStepConfigureDBTools         serviceMutationStepMethod = "Agent.ConfigureDBTools"
	serviceMutationStepConfigureDKIMSigning     serviceMutationStepMethod = "Agent.ConfigureDKIMSigning"
	serviceMutationStepSyncDNSZone              serviceMutationStepMethod = "Agent.SyncDNSZoneV2"
	serviceMutationStepSyncDNSZoneV3            serviceMutationStepMethod = "Agent.SyncDNSZoneV3"
	serviceMutationStepRecoverDNSZoneV3         serviceMutationStepMethod = "Agent.RecoverDNSZoneV3"
	serviceMutationStepSwitchDNSEngine          serviceMutationStepMethod = "Agent.SwitchDNSEngineV1"
	serviceMutationStepSecureDNSZone            serviceMutationStepMethod = "Agent.SecureDNSZoneV2"
	serviceMutationStepConfigureDNSCluster      serviceMutationStepMethod = "Agent.ConfigureDNSClusterV2"
	serviceMutationStepConfigurePowerDNSSQLite  serviceMutationStepMethod = "Agent.ConfigurePowerDNSSQLite"
	serviceMutationStepApplyFirewall            serviceMutationStepMethod = "Agent.ApplyFirewallV2"
	serviceMutationStepInstallService           serviceMutationStepMethod = "Agent.InstallService"
	serviceMutationStepUninstallService         serviceMutationStepMethod = "Agent.UninstallService"
	serviceMutationStepSetServerHostname        serviceMutationStepMethod = "Agent.SetServerHostname"
	serviceMutationStepConfigureMailStack       serviceMutationStepMethod = "Agent.ConfigureMailStack"
	serviceMutationStepWireMailFilters          serviceMutationStepMethod = "Agent.WireMailFilters"
	serviceMutationStepConfigureMailSubmission  serviceMutationStepMethod = "Agent.ConfigureMailSubmission"
	serviceMutationStepSyncMailTLS              serviceMutationStepMethod = "Agent.SyncMailTLSV2"
	serviceMutationStepServiceAction            serviceMutationStepMethod = "Agent.ServiceMutationAction"
	serviceMutationStepStartService             serviceMutationStepMethod = "Agent.StartServiceMutation"
	serviceMutationStepResetFailedUnit          serviceMutationStepMethod = "Agent.ResetFailedUnitMutation"
	serviceMutationStepEnsureNginxReady         serviceMutationStepMethod = "Agent.EnsureNginxReady"
	serviceMutationStepIssuePanelCertificate    serviceMutationStepMethod = "Agent.IssuePanelCertificateV2"
	serviceMutationStepEnableRepo               serviceMutationStepMethod = "Agent.EnableRepo"
	serviceMutationStepDisableRepo              serviceMutationStepMethod = "Agent.DisableRepo"
	serviceMutationStepConfineRepo              serviceMutationStepMethod = "Agent.ConfineRepoV1"
	serviceMutationStepWriteVendorOrigins       serviceMutationStepMethod = "Agent.WriteVendorUpdateOriginsV1"
	serviceMutationStepSetOSUpdatesMode         serviceMutationStepMethod = "Agent.SetOSUpdatesModeV1"
	serviceMutationStepVendorLane               serviceMutationStepMethod = "Agent.EnableVendorLaneV1"
	serviceMutationStepUpdateRoundcube          serviceMutationStepMethod = "Agent.UpdateRoundcubeV1"
	serviceMutationStepEnableWebmailAddress     serviceMutationStepMethod = "Agent.EnableWebmailAddressV1"
	serviceMutationStepRemoveWebmailAddress     serviceMutationStepMethod = "Agent.RemoveWebmailAddressV1"
	serviceMutationStepMoveAppFilesOwner        serviceMutationStepMethod = "Agent.MoveAppFilesOwnerV1"
	serviceMutationStepSetPGClusterPort         serviceMutationStepMethod = "Agent.SetPostgreSQLClusterPortV1"
	serviceMutationStepCopyPGDatabase           serviceMutationStepMethod = "Agent.CopyPostgreSQLDatabaseV1"
	serviceMutationStepDropPGDatabase           serviceMutationStepMethod = "Agent.DropPostgreSQLDatabaseV1"
	serviceMutationStepInstallNodeVersion       serviceMutationStepMethod = "Agent.InstallNodeVersion"
	serviceMutationStepRemoveNodeVersion        serviceMutationStepMethod = "Agent.RemoveNodeVersion"
	serviceMutationStepSetupVPN                 serviceMutationStepMethod = "Agent.SetupVPN"
	serviceMutationStepSyncVPNPeers             serviceMutationStepMethod = "Agent.SyncVPNPeersV2"
	serviceMutationStepInstallRoundcube         serviceMutationStepMethod = "Agent.InstallRoundcube"
	serviceMutationStepRemoveRoundcube          serviceMutationStepMethod = "Agent.RemoveRoundcube"
	serviceMutationStepConfigureWebmail         serviceMutationStepMethod = "Agent.ConfigureWebmail"
	serviceMutationStepBindPHPTool              serviceMutationStepMethod = "Agent.BindPHPToolV1"
	serviceMutationStepProvePHPTool             serviceMutationStepMethod = "Agent.ProvePHPToolV1"
	serviceMutationStepActivatePanelCertificate serviceMutationStepMethod = "agent.panel-certificate-activation"
)

const (
	serviceMutationFirewallEnableLive       = "enable-live"
	serviceMutationFirewallEnablePersisted  = "enable-persisted"
	serviceMutationFirewallDisableLive      = "disable-live"
	serviceMutationFirewallDisablePersisted = "disable-persisted"
)

var errServiceMutationStepUnauthorized = errors.New(
	"active service mutation lease does not authorize this privileged step",
)

type serviceMutationStepClaim struct {
	method      serviceMutationStepMethod
	target      string
	packageName string
	action      string
}

func newServiceMutationStepClaim(
	method serviceMutationStepMethod,
	target, packageName, action string,
) serviceMutationStepClaim {
	return serviceMutationStepClaim{
		method:      method,
		target:      strings.ToLower(strings.TrimSpace(target)),
		packageName: strings.TrimSpace(packageName),
		action:      strings.ToLower(strings.TrimSpace(action)),
	}
}

func serviceMutationFirewallAction(enabled, persist bool) string {
	switch {
	case enabled && persist:
		return serviceMutationFirewallEnablePersisted
	case enabled:
		return serviceMutationFirewallEnableLive
	case persist:
		return serviceMutationFirewallDisablePersisted
	default:
		return serviceMutationFirewallDisableLive
	}
}

func authorizeServiceMutationStep(
	job *ServiceMutationJob,
	claim serviceMutationStepClaim,
) error {
	if job == nil || claim.method == "" ||
		claim.target != strings.ToLower(strings.TrimSpace(claim.target)) ||
		claim.packageName != strings.TrimSpace(claim.packageName) ||
		claim.action != strings.ToLower(strings.TrimSpace(claim.action)) ||
		job.Kind != strings.ToLower(strings.TrimSpace(job.Kind)) ||
		job.Target != strings.ToLower(strings.TrimSpace(job.Target)) ||
		job.PackageName != strings.TrimSpace(job.PackageName) ||
		!serviceMutationStepAllowed(job, claim) {
		return errServiceMutationStepUnauthorized
	}
	return nil
}

func serviceMutationJobMatches(job *ServiceMutationJob, kind, target, packageName string) bool {
	return job != nil && job.Kind == kind && job.Target == target && job.PackageName == packageName
}

func serviceMutationMailProfileKnown(job *ServiceMutationJob) bool {
	if job == nil || job.Kind != "mail_profile_install" || job.PackageName != "" {
		return false
	}
	_, ok := core.MailProfileServiceIDs(job.Target)
	return ok
}

func serviceMutationMailProfileContains(job *ServiceMutationJob, serviceID string) bool {
	return serviceMutationMailProfileKnown(job) &&
		core.MailProfileContainsService(job.Target, serviceID)
}

func serviceMutationSpamFilter(serviceID string) bool {
	service := core.GetManagedServiceByID(serviceID)
	return service != nil && service.ConflictGroup == "spam-filter"
}

func serviceMutationDNSEngineTarget(target string) bool {
	target = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(target)), ".service")
	service := core.GetManagedServiceByID(target)
	if service == nil {
		service = core.ServiceForUnit(target)
	}
	return service != nil && service.ConflictGroup == "dns-server"
}

func serviceMutationValidInstallJob(job *ServiceMutationJob) bool {
	if job == nil || job.Kind != "service_install" {
		return false
	}
	service := core.GetManagedServiceByID(job.Target)
	if service == nil || service.ConflictGroup == "dns-server" {
		return false
	}
	if job.PackageName == "" {
		return true
	}
	_, err := validateRepoPackageSelection(service, job.PackageName)
	return err == nil
}

func serviceMutationStepAllowed(job *ServiceMutationJob, claim serviceMutationStepClaim) bool {
	switch claim.method {
	case serviceMutationStepInstallService,
		serviceMutationStepUninstallService,
		serviceMutationStepServiceAction,
		serviceMutationStepStartService,
		serviceMutationStepResetFailedUnit:
		if serviceMutationDNSEngineTarget(claim.target) ||
			(job != nil && serviceMutationDNSEngineTarget(job.Target)) {
			return false
		}
	}

	switch claim.method {
	case serviceMutationStepInstallService:
		return claim.action == "install" && claim.target != "roundcube" &&
			(serviceMutationJobMatches(job, "service_install", claim.target, claim.packageName) ||
				(claim.packageName == "" && serviceMutationMailProfileContains(job, claim.target)))

	case serviceMutationStepUninstallService:
		return claim.action == "uninstall" && claim.target != "roundcube" &&
			serviceMutationJobMatches(job, "service_uninstall", claim.target, claim.packageName)

	case serviceMutationStepInstallRoundcube:
		return claim.target == "roundcube" && claim.packageName == "" && claim.action == "install" &&
			(serviceMutationJobMatches(job, "service_install", "roundcube", "") ||
				serviceMutationMailProfileContains(job, "roundcube"))

	case serviceMutationStepRemoveRoundcube:
		return claim.target == "roundcube" && claim.packageName == "" && claim.action == "remove" &&
			serviceMutationJobMatches(job, "service_uninstall", "roundcube", "")

	case serviceMutationStepConfigureWebmail:
		return claim.target == "roundcube" && claim.packageName == "" && claim.action == "configure" &&
			(serviceMutationJobMatches(job, "service_install", "roundcube", "") ||
				serviceMutationJobMatches(job, "service_uninstall", "roundcube", "") ||
				serviceMutationMailProfileContains(job, "roundcube"))

	case serviceMutationStepInstallNodeVersion:
		return claim.target == "node" && claim.action == "install" &&
			serviceMutationJobMatches(job, "runtime_install", "node", claim.packageName)

	case serviceMutationStepRemoveNodeVersion:
		return claim.target == "node" && claim.packageName != "" && claim.action == "remove" &&
			serviceMutationJobMatches(job, "runtime_remove", "node:"+claim.packageName, "")

	case serviceMutationStepEnableRepo:
		return claim.packageName == "" && claim.action == "enable" &&
			serviceMutationJobMatches(job, "repo_enable", claim.target, "")

	case serviceMutationStepDisableRepo:
		return claim.packageName == "" && claim.action == "disable" &&
			serviceMutationJobMatches(job, "repo_disable", claim.target, "")

	case serviceMutationStepSetPGClusterPort:
		// alpha.108: one PostgreSQL cluster's port, in its own job; the
		// qualifier is the new port.
		return strings.HasPrefix(claim.target, "pg-") && claim.packageName != "" && claim.action == "port" &&
			serviceMutationJobMatches(job, "pg_cluster_port", claim.target, claim.packageName)

	case serviceMutationStepCopyPGDatabase:
		// alpha.108: one database copied between two clusters (source and
		// target ports are the qualifier).
		return strings.HasPrefix(claim.target, "pgdb-") && claim.packageName != "" && claim.action == "copy" &&
			serviceMutationJobMatches(job, "pg_database_copy", claim.target, claim.packageName)

	case serviceMutationStepDropPGDatabase:
		// alpha.108: one database dropped on one cluster (its port), the
		// administrator's explicit last step of a move.
		return strings.HasPrefix(claim.target, "pgdb-") && claim.packageName != "" && claim.action == "drop" &&
			serviceMutationJobMatches(job, "pg_database_drop", claim.target, claim.packageName)

	case serviceMutationStepMoveAppFilesOwner:
		// alpha.107: one application's files, inside its own app_account job.
		return strings.HasPrefix(claim.target, "site-") && claim.packageName == "" &&
			(claim.action == "move" || claim.action == "restore") &&
			serviceMutationJobMatches(job, "app_account", claim.target, "")

	case serviceMutationStepUpdateRoundcube:
		// alpha.107: webmail updated to one release in its own job.
		return claim.target == "roundcube" && claim.packageName != "" && claim.action == "update" &&
			serviceMutationJobMatches(job, "webmail_update", "roundcube", claim.packageName)

	case serviceMutationStepEnableWebmailAddress:
		// alpha.110: webmail on its own address (the name is the qualifier),
		// in its own job.
		return claim.target == "roundcube" && claim.packageName != "" && claim.action == "enable" &&
			serviceMutationJobMatches(job, "webmail_address", "roundcube", claim.packageName)

	case serviceMutationStepRemoveWebmailAddress:
		return claim.target == "roundcube" && claim.packageName != "" && claim.action == "remove" &&
			serviceMutationJobMatches(job, "webmail_address_remove", "roundcube", claim.packageName)

	case serviceMutationStepWriteVendorOrigins:
		// alpha.107: the owner confirmed the drop-in that adds the confined
		// vendor repositories to the OS's automatic updates.
		return claim.target == "unattended-upgrades" && claim.packageName == "" && claim.action == "origins" &&
			serviceMutationJobMatches(job, "os_updates_origins", "unattended-upgrades", "")

	case serviceMutationStepSetOSUpdatesMode:
		// alpha.114: the owner's automatic/manual choice for the OS updates.
		return claim.target == "unattended-upgrades" && claim.packageName == "" && claim.action == "mode" &&
			serviceMutationJobMatches(job, "os_updates_mode", "unattended-upgrades", "")

	case serviceMutationStepVendorLane:
		// alpha.108: a vendor repository's lane (repository, confinement,
		// automatic updates) written as one confirmed operation.
		return claim.packageName == "" && claim.action == "lane" &&
			serviceMutationJobMatches(job, "vendor_lane", claim.target, "")

	case serviceMutationStepConfineRepo:
		// alpha.107: an already enabled repository is confined to its
		// component in its own job, once, after the Panel's upgrade.
		return claim.packageName == "" && claim.action == "confine" &&
			serviceMutationJobMatches(job, "repo_confine", claim.target, "")

	case serviceMutationStepServiceAction:
		if claim.packageName != "" ||
			(claim.action != "start" && claim.action != "stop" &&
				claim.action != "restart" && claim.action != "reload") {
			return false
		}
		return serviceMutationJobMatches(job, "service_"+claim.action, claim.target, "")

	case serviceMutationStepStartService, serviceMutationStepResetFailedUnit:
		expectedAction := "start"
		if claim.method == serviceMutationStepResetFailedUnit {
			expectedAction = "reset-failed"
		}
		if claim.packageName != "" || claim.action != expectedAction ||
			(claim.target != "postfix" && claim.target != "dovecot") {
			return false
		}
		return serviceMutationJobMatches(job, "service_install", claim.target, "") ||
			serviceMutationMailProfileContains(job, claim.target)

	case serviceMutationStepConfigurePowerDNSSQLite:
		return claim.target == "pdns" && claim.packageName == "" && claim.action == "configure" &&
			serviceMutationJobMatches(job, "pdns_configure", "pdns", "")

	case serviceMutationStepSyncDNSZone:
		if !serviceMutationCanonicalFQDN(claim.target) ||
			!mutationpayload.ValidDNSZoneSyncQualifier(claim.packageName) ||
			(claim.action != "sync" && claim.action != "delete") {
			return false
		}
		return serviceMutationJobMatches(
			job, "dns_zone_sync", claim.target, claim.packageName,
		)

	case serviceMutationStepSyncDNSZoneV3:
		if !serviceMutationCanonicalFQDN(claim.target) ||
			!mutationpayload.ValidDNSZoneSyncV3Qualifier(claim.packageName) ||
			(claim.action != "sync" && claim.action != "delete") {
			return false
		}
		return serviceMutationJobMatches(
			job, "dns_zone_sync", claim.target, claim.packageName,
		)

	case serviceMutationStepRecoverDNSZoneV3:
		return serviceMutationCanonicalFQDN(claim.target) &&
			mutationpayload.ValidDNSZoneSyncV3Qualifier(claim.packageName) &&
			claim.action == "recover" &&
			serviceMutationJobMatches(
				job, "dns_zone_sync", claim.target, claim.packageName,
			)

	case serviceMutationStepSwitchDNSEngine:
		// A reinstall rides the same durable job kind as a switch, and nothing
		// is lost by that: the job's package name is the manifest qualifier,
		// and the qualifier digest commits the mode. A lease taken out for one
		// manifest can therefore never be spent on the other, whatever the
		// action string says. Only BIND has a reinstall transaction.
		//
		// Yeniden kurulum, geçişle aynı kalıcı iş türünü kullanır ve bununla
		// kaybedilen bir şey yoktur: işin paket adı bildirge niteleyicisidir ve
		// niteleyici özeti kipi de bağlar. Bir bildirge için alınan kira,
		// eylem dizesi ne derse desin, diğeri için harcanamaz. Yeniden kurulum
		// işlemi yalnız BIND'de vardır.
		if claim.action == transport.DNSEngineSwitchModeReinstall &&
			claim.target != "bind" {
			return false
		}
		return (claim.target == "bind" || claim.target == "pdns") &&
			(claim.action == transport.DNSEngineSwitchModeSwitch ||
				claim.action == transport.DNSEngineSwitchModeAdopt ||
				claim.action == transport.DNSEngineSwitchModeReinstall) &&
			mutationpayload.ValidDNSEngineSwitchQualifier(claim.packageName) &&
			serviceMutationJobMatches(
				job, "dns_engine_switch", claim.target, claim.packageName,
			)

	case serviceMutationStepSecureDNSZone:
		return serviceMutationCanonicalFQDN(claim.target) &&
			claim.packageName == "" && claim.action == "secure" &&
			serviceMutationJobMatches(
				job, "dnssec_secure", claim.target, "",
			)

	case serviceMutationStepConfigureDNSCluster:
		return claim.target == "pdns" && claim.action == "configure" &&
			mutationpayload.ValidDNSClusterConfigQualifier(claim.packageName) &&
			serviceMutationJobMatches(
				job, "dns_cluster_configure", "pdns", claim.packageName,
			)

	case serviceMutationStepEnsureNginxReady:
		return claim.target == "nginx" && claim.packageName == "" && claim.action == "ready" &&
			(serviceMutationJobMatches(job, "service_install", "nginx", "") ||
				serviceMutationMailProfileContains(job, "nginx"))

	case serviceMutationStepSetServerHostname:
		// Mail installation has no authority to rename the operating system.
		// Keep the old RPC recognizable, but deny it even to recovered mail jobs.
		return false

	case serviceMutationStepConfigureMailStack:
		if claim.target != "mail-stack" || claim.packageName != "" || claim.action != "configure" {
			return false
		}
		return serviceMutationJobMatches(job, "mail_configure", "mail-stack", "") ||
			serviceMutationJobMatches(job, "service_install", "postfix", "") ||
			serviceMutationJobMatches(job, "service_install", "dovecot", "") ||
			(serviceMutationMailProfileContains(job, "postfix") &&
				serviceMutationMailProfileContains(job, "dovecot"))

	case serviceMutationStepWireMailFilters:
		if claim.target != "mail-filters" || claim.packageName != "" || claim.action != "wire" {
			return false
		}
		return (job.Kind == "mail_filter_wire" && job.PackageName == "" &&
			(job.Target == "startup" || serviceMutationSpamFilter(job.Target))) ||
			(job.Kind == "service_install" && job.PackageName == "" &&
				serviceMutationSpamFilter(job.Target)) ||
			serviceMutationJobMatches(job, "mail_profile_install", core.MailProfileProtected, "")

	case serviceMutationStepConfigureMailSubmission:
		return claim.target == "postfix" && claim.packageName == "" && claim.action == "configure" &&
			(serviceMutationJobMatches(job, "mail_submission_configure", "postfix", "") ||
				(serviceMutationMailProfileContains(job, "postfix") &&
					serviceMutationMailProfileContains(job, "dovecot")))

	case serviceMutationStepSyncMailTLS:
		return claim.target == "mail-tls" &&
			claim.action == "sync" &&
			mutationpayload.ValidMailTLSSyncQualifier(claim.packageName) &&
			serviceMutationJobMatches(
				job,
				"mail_tls_sync",
				"mail-tls",
				claim.packageName,
			)

	case serviceMutationStepConfigureDKIMSigning:
		// A mail profile with Postfix sets up signing itself (30 Oct 2026):
		// installed from Services, mail is signed without a first manual
		// "Generate DKIM key".
		// Postfix içeren posta profili imzalamayı kendisi kurar.
		return claim.target == "opendkim" && claim.packageName == "" && claim.action == "configure" &&
			(serviceMutationJobMatches(job, "dkim_signing_configure", "opendkim", "") ||
				serviceMutationMailProfileContains(job, "postfix"))

	case serviceMutationStepBindPHPTool:
		// One tool to one PHP version, inside its own "php_tool_bind" job
		// (alpha.104).
		return claim.action == "bind" && claim.packageName != "" &&
			(claim.target == "webmail" || claim.target == "phpmyadmin" || claim.target == "phppgadmin") &&
			serviceMutationJobMatches(job, "php_tool_bind", claim.target, claim.packageName)

	case serviceMutationStepProvePHPTool:
		// The proof of a tool just installed (alpha.105), inside that
		// tool's own install job or the webmail mail profile's.
		if claim.action != "prove" || claim.packageName != "" {
			return false
		}
		// alpha.108: "Check again" in a tool's Manage window proves it on
		// the version it runs on, inside its own "php_tool_prove" job (no
		// fallback: a proof never moves a tool).
		// "Yeniden denetle" aracı kendi işinde, çalıştığı sürümde kanıtlar.
		if (claim.target == "webmail" || claim.target == "phpmyadmin" || claim.target == "phppgadmin") &&
			serviceMutationJobMatches(job, "php_tool_prove", claim.target, "") {
			return true
		}
		switch claim.target {
		case "phpmyadmin", "phppgadmin":
			return serviceMutationJobMatches(job, "service_install", claim.target, "")
		case "webmail":
			return serviceMutationJobMatches(job, "service_install", "roundcube", "") ||
				serviceMutationMailProfileContains(job, "roundcube")
		}
		return false

	case serviceMutationStepConfigureDBTools:
		if claim.target != "dbtools" || claim.packageName != "" || claim.action != "configure" ||
			(job.Target != "phpmyadmin" && job.Target != "phppgadmin") {
			return false
		}
		return job.PackageName == "" &&
			(job.Kind == "service_install" || job.Kind == "dbtools_configure")

	case serviceMutationStepSetupVPN:
		return claim.target == "wireguard" && claim.packageName == "" && claim.action == "setup" &&
			(serviceMutationJobMatches(job, "vpn_setup", "wireguard", "") ||
				serviceMutationJobMatches(job, "service_install", "wireguard", ""))

	case serviceMutationStepSyncVPNPeers:
		if claim.target != "wireguard" || claim.action != "sync" ||
			!mutationpayload.ValidVPNPeerSyncQualifier(claim.packageName) {
			return false
		}
		return serviceMutationJobMatches(job, "vpn_peer_sync", "wireguard", claim.packageName)

	case serviceMutationStepApplyFirewall:
		if claim.target != "nftables" ||
			!mutationpayload.ValidFirewallApplyQualifier(claim.packageName) {
			return false
		}
		if serviceMutationJobMatches(
			job,
			"firewall_sync",
			"nftables",
			claim.packageName,
		) {
			return claim.action == serviceMutationFirewallEnableLive
		}
		return serviceMutationJobMatches(
			job,
			"firewall_apply",
			"nftables",
			claim.packageName,
		) && (claim.action == serviceMutationFirewallEnableLive ||
			claim.action == serviceMutationFirewallEnablePersisted ||
			claim.action == serviceMutationFirewallDisablePersisted)

	case serviceMutationStepIssuePanelCertificate:
		return serviceMutationCanonicalFQDN(claim.target) &&
			mutationpayload.ValidPanelCertificateIssueQualifier(claim.packageName) &&
			claim.action == "issue" &&
			serviceMutationJobMatches(
				job,
				"panel_certificate_issue",
				claim.target,
				claim.packageName,
			)

	case serviceMutationStepIssueMailHostCertificate:
		return serviceMutationCanonicalFQDN(claim.target) &&
			mutationpayload.ValidMailHostCertificateQualifier(claim.packageName) &&
			claim.action == "issue" &&
			serviceMutationJobMatches(
				job,
				"mail_host_certificate",
				claim.target,
				claim.packageName,
			)

	case serviceMutationStepActivatePanelCertificate:
		return serviceMutationCanonicalFQDN(claim.target) &&
			claim.packageName == "" && claim.action == "activate" &&
			serviceMutationJobMatches(job, panelCertificateActivationKind, claim.target, "")
	}
	return false
}
