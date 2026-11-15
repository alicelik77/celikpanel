package main

import (
	"net/http"
	"strings"
)

// Licence route classes (D-047, docs/LICENCE-SUBSCRIPTION-DESIGN.md §b and
// stage 1 of §j). Every management route is sorted into one class. While the
// server's licence does not allow full use (missing, ended, rejected,
// unverifiable or unreadable), only the classes that need full use are
// refused; everything else works in every licence state, so a security
// action, a backup, a deletion, one's own account, a panel update, the
// licence screen and the other server of a DNS pair never wait for a licence.
//
// Stage 1 refuses "change" and "growth" exactly as the lock refused them
// before; stage 2 of the design opens "change" and keeps only "growth"
// closed. The licence states and their dates are unchanged.
//
// Security is decided by purpose (owner, 15 November 2026): every route whose
// purpose is to reduce exposure, restore security or finish a security update
// is security (S) or update (U), open in every licence state: restarting a
// service, rebooting, updating an installed component, setting a password or
// secret of something that exists, certificate work on what exists, blocking
// and protecting. Turning a protection off or opening access stays "change"
// where the route can tell; where one route does both, it is open, because
// refusing it could leave the server or a customer less safe. A route that
// only adds or reshapes capacity stays closed.
//
// A route the table does not know is open: a forgotten security action can
// never be refused for a licence reason. A forgotten creation of a domain, a
// database or a mailbox is still refused by the count check
// (licenceGrowthCheck in quota.go). The route-inventory test fails when a
// registered route, a domain subroute kind or a database subroute kind has no
// class, so the default is a safety net, not the design.
//
// Lisans rota sınıfları: lisans tam kullanıma izin vermezken yalnız
// "değişiklik" ve "büyüme" sınıfları reddedilir; güvenlik, yedek, silme, kendi
// hesap, güncelleme, lisans ekranı ve DNS eşi her lisans durumunda çalışır.
// Tabloda olmayan rota açıktır; envanter testi sınıfsız rotada başarısız olur.

type licenceRouteClass string

const (
	licenceClassRead     licenceRouteClass = "read"     // R: reading, previews that change nothing
	licenceClassAccount  licenceRouteClass = "account"  // A: signing in, one's own account, sessions, API tokens
	licenceClassUpdate   licenceRouteClass = "update"   // U: panel updates, the licence screen, recovery status
	licenceClassSecurity licenceRouteClass = "security" // S: security and closing access
	licenceClassBackup   licenceRouteClass = "backup"   // B: backups (making, restoring, downloading) and exports
	licenceClassDelete   licenceRouteClass = "delete"   // D: deleting what exists
	licenceClassChange   licenceRouteClass = "change"   // C: changing what exists, server administration
	licenceClassPair     licenceRouteClass = "pair"     // P: the DNS pair (never licence-checked)
	licenceClassGrowth   licenceRouteClass = "growth"   // G: a new account, domain, mailbox, database, login or application
)

// needsFullLicence: the classes a licence state without full use refuses.
// Stage 1: change and growth, as the lock did before.
func (c licenceRouteClass) needsFullLicence() bool {
	return c == licenceClassChange || c == licenceClassGrowth
}

// licenceRouteRule classifies the requests of one registered pattern. Reads
// (GET, HEAD) are always licenceClassRead except where a rule says otherwise
// (readsAre). Every other method takes methods[method], then sub, then write.
type licenceRouteRule struct {
	write   licenceRouteClass
	methods map[string]licenceRouteClass
	// sub decides from the rest of a prefix pattern's path ("" for the
	// pattern itself) and the method; ok false falls back to write.
	sub func(rest, method string) (licenceRouteClass, bool)
	// readsAre: a proxied application's GET is not a panel read.
	readsAre licenceRouteClass
}

func licenceRule(write licenceRouteClass) licenceRouteRule { return licenceRouteRule{write: write} }

var (
	licenceRead     = licenceRule(licenceClassRead)
	licenceAccount  = licenceRule(licenceClassAccount)
	licenceUpdate   = licenceRule(licenceClassUpdate)
	licenceSecurity = licenceRule(licenceClassSecurity)
	licenceBackup   = licenceRule(licenceClassBackup)
	licenceChange   = licenceRule(licenceClassChange)
	licencePair     = licenceRule(licenceClassPair)
	licenceGrowth   = licenceRule(licenceClassGrowth)
)

// licenceRouteTable holds one rule per registered pattern (main.go,
// account_recovery_handlers.go, recovery_access.go). The keys are the same
// constants the registrations use. Domain and database subroutes are
// classified by their dispatcher kind (licenceDomainKindClasses,
// licenceDatabaseKindClasses).
var licenceRouteTable = map[string]licenceRouteRule{
	// Signing in, one's own account and sessions (A).
	panelAccessAddressPath:                 licenceAccount,
	"/api/v1/auth/login":                   licenceAccount,
	"/api/v1/auth/login/totp":              licenceAccount,
	"/api/v1/auth/demo":                    licenceAccount,
	"/api/v1/auth/logout":                  licenceAccount,
	"/api/v1/auth/me":                      licenceAccount,
	"/api/v1/auth/password":                licenceAccount,
	"/api/v1/auth/unimpersonate":           licenceAccount,
	sessionActivityPath:                    licenceAccount,
	ownSessionsPath:                        licenceAccount,
	ownSessionEndPath:                      licenceAccount,
	ownSessionsEndOthers:                   licenceAccount,
	allSessionsPath:                        licenceAccount,
	allSessionEndPath:                      licenceAccount,
	resellerSessionsPath:                   licenceAccount,
	resellerSessionEndPath:                 licenceAccount,
	ownEmailPath:                           licenceAccount,
	guideStatePath:                         licenceAccount,
	activityLogPath:                        licenceAccount,
	"/api/v1/auth/2fa/status":              licenceAccount,
	"/api/v1/auth/2fa/setup":               licenceAccount,
	"/api/v1/auth/2fa/enable":              licenceAccount,
	"/api/v1/auth/2fa/disable":             licenceAccount,
	"/api/v1/auth/2fa/recovery/regenerate": licenceAccount,
	apiTokensPath:                          licenceAccount,
	apiTokensPath + "/":                    licenceAccount,
	remoteAPISettingPath:                   licenceAccount,
	// Account links (public: the link's token is the credential).
	recoveryMailStatusPath:     licenceAccount,
	recoveryRequestPath:        licenceAccount,
	recoveryLinkPath:           licenceAccount,
	recoveryResetPath:          licenceAccount,
	recoveryAcceptInvitePath:   licenceAccount,
	recoveryVerifyEmailPath:    licenceAccount,
	ownEmailVerificationPath:   licenceAccount,
	mailboxRecoveryRequestPath: licenceAccount,
	mailboxRecoveryConfirmPath: licenceAccount,
	mailboxRecoveryLinkPath:    licenceAccount,
	mailboxRecoveryResetPath:   licenceAccount,
	invitationsPath:            licenceRead,
	invitationsPath + "/":      licenceChange, // resend an invitation

	// Panel updates, the licence screen, readiness and recovery status (U).
	panelLicenseAccessPath:          licenceUpdate,
	panelLicensePath:                licenceUpdate,
	panelAvailabilityPath:           licenceUpdate,
	panelRecoveryStatusPath:         licenceUpdate,
	"/api/v1/panel/version":         licenceUpdate,
	panelUpdateCheckPath:            licenceUpdate,
	panelUpdateStartPath:            licenceUpdate,
	panelUpdateAbandonPath:          licenceUpdate,
	panelUpdateStatusPath:           licenceUpdate,
	panelUpdateDiscoveryPath:        licenceUpdate,
	panelUpdateDiscoveryRefreshPath: licenceUpdate,
	hostMutationReadinessPath:       licenceUpdate,

	// Accounts, plans, rights.
	"/api/v1/users": licenceGrowth,
	"/api/v1/users/": {write: licenceClassChange, sub: func(rest, method string) (licenceRouteClass, bool) {
		_, action, _ := strings.Cut(rest, "/")
		switch {
		case action == "suspension":
			// Suspending an account stops its abuse (decided towards
			// security; not listed in the design's table).
			return licenceClassSecurity, true
		case action == "" && method == http.MethodDelete:
			return licenceClassDelete, true
		case action == "" && method == http.MethodPut:
			// An administrator's password reset or suspension of the
			// account (the same form also changes its address).
			return licenceClassSecurity, true
		}
		return licenceClassChange, true
	}},
	"/api/v1/team-members": licenceGrowth,
	// A team member's password, suspension and access are changed together.
	"/api/v1/team-members/":    {write: licenceClassChange, methods: map[string]licenceRouteClass{http.MethodPut: licenceClassSecurity, http.MethodDelete: licenceClassDelete}},
	"/api/v1/plans":            licenceChange,
	"/api/v1/plans/":           licenceChange,
	resellerPlansPath:          licenceChange,
	resellerPlansPath + "/":    licenceChange,
	rightsPathPrefix:           licenceChange,
	remoteBackupPathPrefix:     licenceBackup,
	"/api/v1/subscriptions":    licenceRead,
	"/api/v1/subscriptions/":   {write: licenceClassChange, sub: licenceSubscriptionClass},
	transferRequestsPath:       licenceChange,
	transferRequestsPath + "/": licenceChange,
	mySubscriptionPath:         licenceRead,
	suspensionDeletablePath:    licenceRead,

	// Security emergencies and their notices (S).
	securityEmergenciesPath:       licenceSecurity,
	securityEmergenciesPath + "/": licenceSecurity,
	securityNoticesPath:           licenceSecurity,
	securityNoticesPath + "/":     licenceSecurity,

	// Notices, notifications and their settings.
	usageNoticesPath:            licenceChange,
	usageNoticesPath + "/":      licenceChange,
	usageStatusPath:             licenceRead,
	notificationsPath:           licenceChange,
	notificationsPath + "/":     licenceChange,
	notificationPreferencesPath: licenceChange,
	notificationSettingsPath:    licenceChange,

	// Store and catalogue offerings.
	"/api/v1/products":             licenceRead,
	"/api/v1/store":                licenceRead,
	"/api/v1/store/":               licenceRead,
	"/api/v1/admin/store-catalog":  licenceChange,
	"/api/v1/admin/store-catalog/": licenceChange,
	serviceAppsAdminPath:           licenceChange,
	serviceAppsAdminPath + "/":     licenceChange,
	runtimeAppsAdminPath:           licenceChange,
	runtimeAppsAdminPath + "/":     licenceChange,
	"/api/v1/audit-logs":           licenceRead,
	"/api/v1/dashboard":            licenceRead,
	"/api/v1/apps":                 licenceRead,
	operationProgressPath:          licenceRead,
	"/api/v1/hosting/capabilities": licenceRead,
	supportTablePath:               licenceRead,
	offeredVersionsPath:            licenceChange,

	// The DNS pair: pairing administration and the machine endpoints (P).
	"/api/v1/dns/remote/enrollments":      licencePair,
	"/api/v1/dns/remote/clients":          licencePair,
	"/api/v1/dns/remote/connections":      licencePair,
	"/api/v1/dns/remote/accept":           licencePair,
	"/api/v1/dns/remote/receiver/status":  licencePair,
	"/api/v1/dns/remote/receiver/publish": licencePair,
	remoteDNSHostRecordPath:               licencePair,
	serverSetupJoinCodesPath:              licencePair,
	serverSetupPeerPath:                   licencePair,
	serverSetupPeerConnectPath:            licencePair,
	serverSetupPeerRetirePath:             licencePair,
	dnsPeerInspectorToolPath:              licencePair,

	// Server setup configures the server's own roles (C; the design's §b).
	serverSetupPath:                               licenceChange,
	serverSetupPath + "/guidance":                 licenceChange,
	serverSetupPath + "/components":               licenceChange,
	serverSetupProviderRecordsPath:                licenceChange,
	serverSetupPublicRecordsPath:                  licenceChange,
	serverSetupDelegationPath:                     licenceChange,
	serverSetupPath + "/complete":                 licenceChange,
	serverSetupPath + "/revise":                   licenceChange,
	serverSetupPath + "/package-updates":          licenceChange,
	serverSetupPath + "/extend":                   licenceChange,
	serverSetupPath + "/mail-enrollment/continue": licenceChange,
	serverSetupPath + "/mail-enrollment/retry":    licenceChange,
	serverSetupPath + "/publisher":                licenceChange,
	serverSetupPath + "/plan":                     licenceChange,
	serverSetupPath + "/start":                    licenceChange,
	serverSetupPath + "/operation":                licenceChange,

	// Server administration (C) and monitoring.
	"/api/v1/managed-services":          licenceChange,
	"/api/v1/managed-services/scan":     licenceChange,
	"/api/v1/system/check":              licenceChange,
	"/api/v1/system/stats":              licenceRead,
	"/api/v1/metrics/history":           licenceRead,
	"/api/v1/metrics/problems":          licenceRead,
	"/api/v1/metrics/services":          licenceRead,
	"/api/v1/metrics/sites":             licenceRead,
	"/api/v1/metrics/alerts":            licenceRead,
	"/api/v1/metrics/alerts/settings":   licenceChange,
	"/api/v1/metrics/alerts/seen":       licenceChange,
	"/api/v1/php/pools":                 licenceChange,
	"/api/v1/php/pool-config":           licenceChange,
	"/api/v1/php/extensions":            licenceChange,
	"/api/v1/php/config":                licenceChange,
	"/api/v1/php/extended-config":       licenceChange,
	"/api/v1/nginx/global":              licenceChange,
	"/api/v1/nginx/ssl":                 licenceSecurity, // the server's TLS protocols and ciphers
	nginxDefaultHTTPSPath:               licenceChange,
	"/api/v1/ssl/providers":             licenceChange,
	"/api/v1/settings/nameservers":      licenceChange,
	"/api/v1/settings/dns-cluster":      licenceChange,
	"/api/v1/settings/dns-setup":        licenceChange,
	"/api/v1/nginx/ratelimits":          licenceChange,
	nginxDirectivesOfferPath:            licenceChange,
	serverIPAddressesPath:               licenceChange,
	"/api/v1/postfix/queue":             licenceSecurity, // removing or holding spam in the queue
	"/api/v1/postfix/summary":           licenceRead,
	"/api/v1/dovecot/stats":             licenceChange,
	"/api/v1/config":                    licenceChange,
	"/api/v1/service/action":            licenceSecurity, // restart, reload, stop: completes a security update, stops a compromised service
	"/api/v1/service/logs":              licenceRead,
	"/api/v1/service/status":            licenceChange,
	"/api/v1/service/install":           licenceGrowth, // a new component
	mailProfileInstallPath:              licenceGrowth, // a new mail component set
	"/api/v1/service/operation":         licenceChange,
	"/api/v1/service/candidate":         licenceChange,
	serviceUninstallPath:                {write: licenceClassDelete}, // removing a component
	serviceRemovalImpactPath:            licenceRead,
	serviceInstallPreviewPath:           licenceRead,
	serviceInstallPlanPath:              licenceRead,
	roundcubeUpdatePath:                 licenceUpdate, // updating the installed webmail
	pgServiceAPIPrefix:                  licenceChange,
	phpToolsPath:                        licenceChange,
	phpToolBindPath:                     licenceChange,
	phpToolProvePath:                    licenceChange,
	"/api/v1/repo":                      licenceChange,
	repoConfinePreviewAPIPath:           licenceChange,
	vendorLanePath:                      licenceChange,
	fileAccessToolsPath:                 licenceChange,
	restartNeedsPath:                    licenceChange,
	serverClockPath:                     licenceChange,
	serverProcessesPath:                 licenceChange,
	serverRebootPath:                    licenceSecurity, // completes a kernel or library security update
	osReleaseAPIPath:                    licenceChange,
	osReleaseReviewedAPIPath:            licenceChange,
	holdingPath:                         licenceRead,
	holdingPath + "/":                   {write: licenceClassDelete}, // remove a held item; take left mail into holding
	"/webmail/":                         licenceChange,
	webmailAddressPath:                  licenceChange,
	webmailAddressCheckPath:             licenceChange,
	webmailAddressRemovePath:            licenceChange,
	"/api/v1/config/php":                licenceChange,
	"/api/v1/config/mysql":              licenceChange,
	"/api/v1/pdns/enable":               licenceChange,
	serverHostnameDNSPath:               licenceChange,
	infrastructureDNSZonesPath:          licenceChange,
	infrastructureDNSZonesPath + "/":    licenceChange,
	"/api/v1/dns/engine":                licenceChange,
	dnsOverviewPath:                     licenceRead,
	"/api/v1/dns/engine/reconcile":      licenceChange,
	"/api/v1/dns/engine/switch/preview": licenceChange,
	"/api/v1/dns/engine/switch":         licenceChange,
	"/api/v1/mail/configure":            licenceChange,
	"/api/v1/mail/policy":               licenceChange,
	mailOutgoingPath:                    licenceSecurity, // sending limits: throttling a spamming mailbox
	mailRelayPath:                       licenceSecurity, // the relay's credentials (a leaked password must be replaceable)
	mailRelayTestPath:                   licenceChange,
	mailServerSettingsPath:              licenceChange,
	mailSubaddressingPath:               licenceChange,
	mailSenderRulesPath:                 licenceChange,
	mailOutgoingReportPath:              licenceChange,
	mailOutgoingNoticesPath:             licenceChange,
	mailOutgoingNoticesPath + "/":       licenceChange,
	mailPlanLimitsPath:                  licenceChange,
	wordpressToolsPath:                  {write: licenceClassChange, sub: licenceWordPressClass},
	"/api/v1/system-databases":          licenceChange,
	"/api/v1/system-databases/":         licenceChange,
	"/api/v1/runtimes/node":             licenceGrowth, // installing a Node.js version
	"/api/v1/runtimes/node/":            {write: licenceClassChange, methods: map[string]licenceRouteClass{http.MethodDelete: licenceClassDelete}},
	"/api/v1/import/cpanel/inspect":     licenceRead,
	"/api/v1/import/cpanel/apply":       licenceGrowth,
	"/api/v1/database-servers":          licenceChange,
	databasesVisiblePath:                licenceRead,
	databaseGrantFormsPath:              licenceChange,
	// The database tool proxy fronts phpMyAdmin and phpPgAdmin: what their
	// GET does is not a panel read, so every method stays as before.
	"/dbtool/": {write: licenceClassChange, readsAre: licenceClassChange},

	// Security (S).
	panelAccessSettingPath:        licenceSecurity,
	"/api/v1/fail2ban/jails":      licenceSecurity,
	"/api/v1/fail2ban/banned":     licenceSecurity,
	"/api/v1/fail2ban/config":     licenceSecurity,
	passwordlessMailPath:          licenceSecurity,
	"/api/v1/firewall":            licenceSecurity,
	"/api/v1/panel/certificate":   licenceSecurity,
	panelPortPath:                 licenceSecurity,
	securityAuditPath:             licenceSecurity,
	osUpdatesPath:                 licenceSecurity,
	osUpdatesOriginsPath:          licenceSecurity,
	osUpdatesModePath:             licenceSecurity,
	osUpdatesSummaryPath:          licenceSecurity,
	sshPasswordLoginPath:          licenceSecurity,
	certificatesOverviewPath:      licenceSecurity,
	certificateRequestsPath:       licenceSecurity,
	certificateRequestsPath + "/": licenceSecurity,
	"/api/v1/vpn/status":          licenceSecurity,
	"/api/v1/vpn/setup":           licenceSecurity,
	"/api/v1/vpn/sync":            licenceSecurity,
	"/api/v1/vpn/peers":           licenceSecurity,
	"/api/v1/vpn/peers/":          licenceSecurity,
	malwarePath:                   licenceSecurity,
	// Closing remote database access must always work; adding a rule opens
	// access and stays a change. The server switch opens and closes in one
	// request, so it is open.
	databaseRemotePath: {write: licenceClassSecurity, sub: func(rest, method string) (licenceRouteClass, bool) {
		if segments := strings.Split(rest, "/"); len(segments) == 3 && segments[0] == "databases" && segments[2] == "rules" && method == http.MethodPost {
			return licenceClassChange, true
		}
		return "", false
	}},

	// Hosting content: domains and databases.
	"/api/v1/domains":           licenceRead,
	"/api/v1/domains/create":    licenceGrowth,
	"/api/v1/domains/":          licenceChange, // a matched subroute is classified by its kind
	"/api/v1/database-servers/": licenceChange,
	"/api/v1/databases/":        licenceChange,
	"/api/v1/database-users/":   licenceChange,
	"/api/v1/database-grants/":  licenceChange,

	// The interface's own files.
	"/": licenceRead,
}

// licenceSubscriptionClass sorts /api/v1/subscriptions/{id}/...: a product
// granted to an account is growth, its removal a deletion; suspending is
// decided towards security as for accounts; rights, end date and transfer
// change what exists.
func licenceSubscriptionClass(rest, method string) (licenceRouteClass, bool) {
	path := "/api/v1/subscriptions/" + rest
	if _, ok := parseSubscriptionSuspensionPath(path); ok {
		return licenceClassSecurity, true
	}
	if _, _, ok := parseSubscriptionRightsPath(path); ok {
		return licenceClassChange, true
	}
	if _, ok := parseSubscriptionEndsAtPath(path); ok {
		return licenceClassChange, true
	}
	if _, _, ok := parseSubscriptionTransferPath(path); ok {
		return licenceClassChange, true
	}
	if _, ok := parseEntitlementPath(path); ok {
		switch method {
		case http.MethodPost:
			return licenceClassGrowth, true
		case http.MethodDelete:
			return licenceClassDelete, true
		}
	}
	return "", false
}

// licenceWordPressClass sorts /api/v1/wordpress/{domain}/...: updating
// WordPress and switching its automatic updates finish security updates; a
// clone is a new site.
func licenceWordPressClass(rest, method string) (licenceRouteClass, bool) {
	segments := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segments) != 2 {
		return "", false
	}
	switch segments[1] {
	case "update", "auto-update":
		return licenceClassUpdate, true
	case "clone":
		return licenceClassGrowth, true
	}
	return "", false
}

// licenceDomainKindClasses sorts every kind matchDomainSubroute assigns, by
// method. GET and HEAD are reads and are not repeated. "mail" and
// "ftp-logins" are sorted by their path as well (licenceDomainClass).
var licenceDomainKindClasses = map[string]map[string]licenceRouteClass{
	"delete":                   {http.MethodDelete: licenceClassDelete},
	"deletion-status":          {},
	"hosting":                  {http.MethodPut: licenceClassChange},
	"app":                      {http.MethodPost: licenceClassChange}, // stop and restart: licenceDomainClass
	"app-account":              {http.MethodPost: licenceClassChange},
	"app-node-version":         {http.MethodPost: licenceClassUpdate}, // a Node.js patch with its fixes
	"app-packages":             {http.MethodPost: licenceClassChange},
	"app-env":                  {http.MethodPost: licenceClassSecurity}, // where the application's secrets live
	"php":                      {http.MethodPost: licenceClassChange},
	"php-pool":                 {http.MethodPost: licenceClassChange},
	"php-settings":             {http.MethodPut: licenceClassChange},
	"general":                  {http.MethodPost: licenceClassChange},
	"aliases":                  {http.MethodPost: licenceClassGrowth, http.MethodDelete: licenceClassDelete},
	"dnssec":                   {http.MethodPost: licenceClassChange},
	"ssl-mail":                 {http.MethodPut: licenceClassSecurity},
	"ssl-coverage":             {},
	"ssl-retry":                {http.MethodPost: licenceClassSecurity},
	"ssl-renewal":              {http.MethodPut: licenceClassSecurity},
	"ssl-letsencrypt":          {http.MethodPost: licenceClassSecurity},
	"ssl-upload":               {http.MethodPost: licenceClassSecurity},
	"ssl-settings":             {http.MethodPost: licenceClassSecurity},
	"ssl":                      {http.MethodDelete: licenceClassChange}, // removing a certificate turns a protection off
	"git":                      {http.MethodPost: licenceClassChange},
	"git-key":                  {http.MethodPost: licenceClassSecurity}, // a new deploy key
	"git-deploy":               {http.MethodPost: licenceClassChange},
	"site-copy":                {http.MethodPost: licenceClassGrowth},
	"logs-retention":           {http.MethodPost: licenceClassChange},
	"logs":                     {http.MethodDelete: licenceClassDelete},
	"databases":                {http.MethodPost: licenceClassGrowth},
	"database-delete":          {http.MethodDelete: licenceClassDelete},
	"database-users":           {http.MethodPost: licenceClassGrowth},
	"database-user":            {http.MethodDelete: licenceClassDelete},
	"database-user-password":   {http.MethodPost: licenceClassSecurity},
	"database-tools":           {},
	"database-export":          {http.MethodPost: licenceClassBackup},
	"database-check":           {http.MethodPost: licenceClassBackup},
	"database-import":          {http.MethodPost: licenceClassChange},
	"database-import-upload":   {http.MethodPost: licenceClassChange},
	"database-copy":            {http.MethodPost: licenceClassGrowth},
	"files":                    {http.MethodPost: licenceClassChange},
	"files-download":           {},
	"backup-schedule":          {http.MethodPut: licenceClassBackup, http.MethodDelete: licenceClassBackup},
	"backup-restore":           {http.MethodPost: licenceClassBackup},
	"backup-download":          {},
	"backup-contents":          {},
	"backups":                  {http.MethodPost: licenceClassBackup, http.MethodDelete: licenceClassBackup},
	"backup-remote":            {},
	"backup-remote-policy":     {http.MethodPut: licenceClassBackup, http.MethodDelete: licenceClassBackup},
	"backup-remote-objects":    {},
	"backup-remote-send":       {http.MethodPost: licenceClassBackup},
	"backup-remote-restore":    {http.MethodPost: licenceClassBackup},
	"backup-remote-operation":  {},
	"cron":                     {http.MethodPost: licenceClassChange, http.MethodPut: licenceClassChange, http.MethodDelete: licenceClassChange},
	"mail-health":              {},
	"mail":                     {http.MethodPost: licenceClassChange, http.MethodPut: licenceClassChange, http.MethodDelete: licenceClassChange},
	"app-install":              {http.MethodPost: licenceClassGrowth},
	"app-catalog":              {},
	"app-operation":            {http.MethodPost: licenceClassChange, http.MethodDelete: licenceClassDelete}, // credentials: licenceDomainClass
	"service-apps":             {},
	"service-apps-install":     {http.MethodPost: licenceClassGrowth},
	"service-apps-update":      {http.MethodPost: licenceClassUpdate},
	"service-apps-control":     {http.MethodPost: licenceClassSecurity}, // stop and restart (start rides along)
	"service-apps-logs":        {},
	"service-apps-secrets":     {http.MethodPut: licenceClassSecurity},
	"service-apps-uninstall":   {http.MethodPost: licenceClassDelete},
	"runtime-apps":             {},
	"runtime-apps-create":      {http.MethodPost: licenceClassGrowth},
	"runtime-apps-settings":    {http.MethodPut: licenceClassChange},
	"runtime-apps-deploy":      {http.MethodPost: licenceClassChange},
	"runtime-apps-control":     {http.MethodPost: licenceClassSecurity}, // stop and restart (start rides along)
	"runtime-apps-env":         {http.MethodPut: licenceClassSecurity},  // where the program's secrets live
	"runtime-apps-logs":        {},
	"runtime-apps-remove":      {http.MethodPost: licenceClassDelete},
	"usage":                    {},
	"visitor-stats":            {},
	"connection":               {},
	"file-access":              {http.MethodPut: licenceClassSecurity}, // the account's password and keys (switching on rides along)
	"file-access-preview":      {http.MethodPost: licenceClassRead},
	"ftp-logins":               {http.MethodPost: licenceClassGrowth, http.MethodPut: licenceClassSecurity, http.MethodDelete: licenceClassDelete},
	"site-config":              {},
	"site-config-keep":         {http.MethodPost: licenceClassChange},
	"site-config-take":         {http.MethodPost: licenceClassChange},
	"site-config-recreate":     {http.MethodPost: licenceClassChange},
	"site-config-apply":        {http.MethodPost: licenceClassChange},
	"site-protect":             {http.MethodPost: licenceClassSecurity, http.MethodDelete: licenceClassChange}, // removing turns a protection off
	"site-protect-users":       {http.MethodPost: licenceClassSecurity, http.MethodDelete: licenceClassSecurity},
	"site-redirects":           {http.MethodPost: licenceClassChange, http.MethodDelete: licenceClassChange},
	"web-settings":             {http.MethodPost: licenceClassChange},
	"web-settings-directives":  {http.MethodPost: licenceClassChange},
	"web-settings-cache-clear": {http.MethodPost: licenceClassChange},
	"dns":                      {http.MethodPost: licenceClassChange, http.MethodPut: licenceClassChange, http.MethodDelete: licenceClassChange},
	domainHealthKind:           {},
}

// licenceDatabaseKindClasses sorts every kind matchDatabaseSubroute assigns.
var licenceDatabaseKindClasses = map[string]map[string]licenceRouteClass{
	"server-delete":        {http.MethodDelete: licenceClassDelete},
	"server-databases":     {http.MethodPost: licenceClassGrowth},
	"server-users":         {http.MethodPost: licenceClassGrowth},
	"server-admin-account": {http.MethodPost: licenceClassSecurity, http.MethodDelete: licenceClassSecurity},
	"database-delete":      {http.MethodDelete: licenceClassDelete},
	"database-grants":      {http.MethodPost: licenceClassChange},
	"user-delete":          {http.MethodDelete: licenceClassDelete},
	"user-password":        {http.MethodPost: licenceClassSecurity},
	// Taking a right away is a deletion (decided towards security; the
	// design's table lists database grants as change).
	"grant-delete": {http.MethodDelete: licenceClassDelete},
}

func licenceIsRead(method string) bool { return method == http.MethodGet || method == http.MethodHead }

// licenceDomainClass classifies a matched domain subroute.
func licenceDomainClass(match domainSubroute, tail, method string) (licenceRouteClass, bool) {
	if match.kind == "mail" && !licenceIsRead(method) {
		switch {
		case tail == "mail/accounts" && method == http.MethodPost, tail == "mail/forwardings" && method == http.MethodPost:
			return licenceClassGrowth, true
		case tail == "mail/accounts" && method == http.MethodDelete, tail == "mail/forwardings" && method == http.MethodDelete,
			tail == "mail/accounts/empty":
			return licenceClassDelete, true
		case tail == "mail/accounts/password", tail == "mail/accounts/recovery", tail == "mail/outgoing",
			tail == "mail/auth/dkim", tail == "mail/auth/apply", tail == "mail/auth/automatic":
			// A mailbox's password and recovery address, its sending limit,
			// and the domain's mail authentication (SPF, DKIM, DMARC).
			return licenceClassSecurity, true
		}
	}
	if match.kind == "app" && (tail == "app/stop" || tail == "app/restart") && method == http.MethodPost {
		return licenceClassSecurity, true
	}
	if match.kind == "app-operation" && strings.HasSuffix(tail, "/credentials") && method == http.MethodPost {
		// Shows the application's first credentials again; changes nothing.
		return licenceClassRead, true
	}
	if licenceIsRead(method) {
		return licenceClassRead, true
	}
	class, ok := licenceDomainKindClasses[match.kind][method]
	return class, ok
}

func licenceDatabaseClass(match databaseSubroute, method string) (licenceRouteClass, bool) {
	if licenceIsRead(method) {
		return licenceClassRead, true
	}
	class, ok := licenceDatabaseKindClasses[match.kind][method]
	return class, ok
}

// licenceRoutePattern returns the registered pattern that serves path: the
// exact pattern, or the longest prefix pattern (ending in "/").
func licenceRoutePattern(path string) (string, bool) {
	if _, ok := licenceRouteTable[path]; ok {
		return path, true
	}
	best := ""
	for pattern := range licenceRouteTable {
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(path, pattern) && len(pattern) > len(best) {
			best = pattern
		}
	}
	return best, best != ""
}

// licenceRouteClassOf classifies a request. ok is false only for a request
// the table cannot sort; such a request is open (see the package comment).
func licenceRouteClassOf(r *http.Request) (licenceRouteClass, bool) {
	path := r.URL.Path
	// An escaped path is never one the interface sends to a management
	// route; it is not sorted towards an open class.
	if r.URL.RawPath != "" && !licenceIsRead(r.Method) {
		return licenceClassChange, true
	}
	if match, ok := matchDomainSubroute(r); ok {
		segments, _ := strictRouteSegments(r, "/api/v1/domains/")
		return licenceDomainClass(match, strings.Join(segments[1:], "/"), r.Method)
	}
	for _, prefix := range []string{"/api/v1/database-servers/", "/api/v1/databases/", "/api/v1/database-users/", "/api/v1/database-grants/"} {
		if strings.HasPrefix(path, prefix) {
			if match, ok := matchDatabaseSubroute(r, prefix); ok {
				return licenceDatabaseClass(match, r.Method)
			}
		}
	}
	pattern, ok := licenceRoutePattern(path)
	if !ok {
		return "", false
	}
	rule := licenceRouteTable[pattern]
	if licenceIsRead(r.Method) {
		if rule.readsAre != "" {
			return rule.readsAre, true
		}
		return licenceClassRead, true
	}
	if class, ok := rule.methods[r.Method]; ok {
		return class, true
	}
	if rule.sub != nil {
		if class, ok := rule.sub(strings.TrimPrefix(path, pattern), r.Method); ok {
			return class, true
		}
	}
	return rule.write, true
}
