package transport

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Website certificates renewed by the operating system (D-022, D-039).
//
// A website certificate issued by this release lives in the standard certbot
// store (/etc/letsencrypt) as its own lineage named
// celikpanel-site-<domain id>-<24 hex>. nginx names certbot's live files
// directly, certbot's own timer (or cron) renews it, and the deploy hook
// /etc/letsencrypt/renewal-hooks/deploy/celikpanel-site-certificates reloads
// nginx after a renewal. None of this needs the Panel or the Agent: removing
// both leaves a certificate that keeps renewing and being served. The Panel
// only reads what the operating system did (SiteCertificateRenewalStateV1).
//
// Certificates issued by earlier releases (lineage named after the domain, or
// cp-site-…, copied into an immutable snapshot under /etc/ssl/celikpanel) stay
// renewed by the Panel until they are reissued; see D-039.
//
// İşletim sisteminin yenilediği web sitesi sertifikaları: standart certbot
// deposunda, nginx certbot'un live dosyalarını adlandırır, certbot'un kendi
// zamanlayıcısı yeniler; panel ve agent gerekmez.

const (
	// NativeSiteCertbotConfigDir is the standard certbot store.
	NativeSiteCertbotConfigDir = "/etc/letsencrypt"
	// NativeSiteLineagePrefix starts every OS-renewed website lineage.
	NativeSiteLineagePrefix = "celikpanel-site-"
	// NativeSiteDeployHookPath is the deploy hook certbot runs after every
	// renewal (certbot's own renewal-hooks/deploy directory). It acts only
	// on lineages named NativeSiteLineagePrefix….
	NativeSiteDeployHookPath = "/etc/letsencrypt/renewal-hooks/deploy/celikpanel-site-certificates"
	// NativeSiteDeployStatusDir holds one line-based file per lineage that
	// the deploy hook writes after a renewal (format NativeSiteDeployFormat).
	NativeSiteDeployStatusDir = "/var/lib/celikpanel-site-certificates"
	// NativeSiteDeployFormat is the first line of a deploy status file.
	NativeSiteDeployFormat = "celikpanel-site-deploy-v1"
)

var nativeSiteLineageRE = regexp.MustCompile(`^celikpanel-site-([1-9][0-9]{0,18})-[a-f0-9]{24}$`)

// IsNativeSiteLineage reports whether name is an OS-renewed website lineage.
func IsNativeSiteLineage(name string) bool {
	return nativeSiteLineageRE.MatchString(name)
}

// NativeSiteLineageDomainID returns the domain identity a native lineage
// name carries.
func NativeSiteLineageDomainID(name string) (int, bool) {
	match := nativeSiteLineageRE.FindStringSubmatch(name)
	if len(match) != 2 {
		return 0, false
	}
	id, err := strconv.Atoi(match[1])
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// NativeSiteLineageBelongsTo reports whether name is a native lineage of the
// domain identity.
func NativeSiteLineageBelongsTo(name string, domainID int) bool {
	id, ok := NativeSiteLineageDomainID(name)
	return ok && domainID > 0 && id == domainID
}

// NativeSiteLineagePaths are the stable files nginx names: certbot's live
// symbolic links, which certbot moves to each renewed generation.
func NativeSiteLineagePaths(lineage string) (fullchain, privkey, chain string) {
	dir := path.Join(NativeSiteCertbotConfigDir, "live", lineage)
	return path.Join(dir, "fullchain.pem"), path.Join(dir, "privkey.pem"), path.Join(dir, "chain.pem")
}

// NativeSiteLineageFromPath returns the native lineage whose live file the
// path names exactly (fullchain.pem, privkey.pem, chain.pem or cert.pem).
func NativeSiteLineageFromPath(candidate string) (string, bool) {
	if candidate == "" || candidate != strings.TrimSpace(candidate) || path.Clean(candidate) != candidate {
		return "", false
	}
	switch path.Base(candidate) {
	case "fullchain.pem", "privkey.pem", "chain.pem", "cert.pem":
	default:
		return "", false
	}
	dir := path.Dir(candidate)
	if path.Dir(dir) != path.Join(NativeSiteCertbotConfigDir, "live") {
		return "", false
	}
	lineage := path.Base(dir)
	if !IsNativeSiteLineage(lineage) {
		return "", false
	}
	return lineage, true
}

// Renewal states of an OS-renewed website certificate. Each names who acts
// and what they do (D-024); the page translates the code.
const (
	// The operating system's certbot schedule renews it; nothing to do.
	SiteRenewalScheduled = "scheduled"
	// A renewed certificate was observed since the Panel last looked.
	SiteRenewalRenewed = "renewed"
	// certbot's own log records a failed renewal of this lineage after its
	// newest certificate: the administrator checks the named cause.
	SiteRenewalFailed = "failed"
	// Renewal is due and has not happened, with no failure recorded: the
	// administrator checks certbot's schedule and runs a dry run.
	SiteRenewalOverdue = "overdue"
	// The certificate has expired.
	SiteRenewalExpired = "expired"
	// No certbot timer or cron entry was found: nothing renews it.
	SiteRenewalSchedulerMissing = "scheduler_missing"
	// The lineage's renewal configuration says not to renew it.
	SiteRenewalDisabled = "disabled"
	// The renewal configuration was changed outside CelikPanel (another
	// authenticator or validation directory). Kept as it is; renewal is
	// judged from the certificate's own dates.
	SiteRenewalOwnerConfigured = "owner_configured"
	// Renewed, but the deploy hook could not make nginx load it: the web
	// server still serves the previous certificate.
	SiteRenewalDeployIncomplete = "deploy_incomplete"
	// The deploy hook is missing or was changed: a renewed certificate is
	// not loaded by nginx until it is reloaded.
	SiteRenewalHookMissing = "hook_missing"
	SiteRenewalHookChanged = "hook_changed"
	// certbot's lineage (renewal configuration or live files) is gone.
	SiteRenewalLineageMissing = "lineage_missing"
	// The state could not be read; nothing is concluded.
	SiteRenewalUnknown = "unknown"
)

// SiteRenewalStateIsProblem reports whether a state needs the
// administrator. Unknown is not a problem: it is not known.
func SiteRenewalStateIsProblem(state string) bool {
	switch state {
	case SiteRenewalFailed, SiteRenewalOverdue, SiteRenewalExpired,
		SiteRenewalSchedulerMissing, SiteRenewalDisabled, SiteRenewalDeployIncomplete,
		SiteRenewalHookMissing, SiteRenewalHookChanged, SiteRenewalLineageMissing:
		return true
	}
	return false
}

// SiteCertificateRenewalStateRequest asks for the OS renewal state of one
// native lineage (read only).
type SiteCertificateRenewalStateRequest struct {
	Domain         string `json:"domain"`
	DomainID       int    `json:"domain_id"`
	SubscriptionID int    `json:"subscription_id"`
	LineageName    string `json:"lineage_name"`
	// ACMEServer is the directory the Panel issued from ("" for Let's
	// Encrypt's default); a different server in the renewal configuration
	// is reported as owner_configured.
	ACMEServer string `json:"acme_server,omitempty"`
}

// SiteRenewalScheduler is what runs `certbot renew` on this host.
type SiteRenewalScheduler struct {
	// Name is the timer unit (certbot.timer, certbot-renew.timer,
	// snap.certbot.renew.timer) or "cron" (/etc/cron.d/certbot); "" none.
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
	Active  bool   `json:"active"`
	// Known is false when the service manager could not be asked.
	Known bool `json:"known"`
}

// SiteRenewalDeploy is the deploy hook's last record for the lineage.
type SiteRenewalDeploy struct {
	CompletedAt time.Time `json:"completed_at"`
	// Nginx: reloaded, not_running, not_installed, config_test_failed,
	// reload_failed.
	Nginx string `json:"nginx"`
}

// SiteRenewalFailure is certbot's newest logged failure for the lineage.
type SiteRenewalFailure struct {
	At     time.Time `json:"at"`
	Detail string    `json:"detail"`
}

type SiteCertificateRenewalStateResponse struct {
	State string `json:"state"`
	// Detail is one bounded sentence for the log and the page's details.
	Detail      string    `json:"detail,omitempty"`
	LineageName string    `json:"lineage_name"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	// LeafSHA256 identifies the certificate certbot's live files name now.
	LeafSHA256 string `json:"leaf_sha256,omitempty"`
	// RenewBefore is when certbot renews it (renew_before_expiry, default
	// 30 days before expiry).
	RenewAfter time.Time            `json:"renew_after"`
	Scheduler  SiteRenewalScheduler `json:"scheduler"`
	// Hook: installed, missing, changed, unreadable.
	Hook       string              `json:"hook"`
	LastDeploy *SiteRenewalDeploy  `json:"last_deploy,omitempty"`
	LastFailed *SiteRenewalFailure `json:"last_failed,omitempty"`
	AutoRenew  bool                `json:"auto_renew"`
	// Authenticator and Server are the renewal configuration's own values.
	Authenticator string `json:"authenticator,omitempty"`
	Server        string `json:"server,omitempty"`
	Error         string `json:"error,omitempty"`
}

// SetSiteCertificateAutoRenewRequest switches certbot's own `autorenew`
// setting of one native lineage.
type SetSiteCertificateAutoRenewRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	DomainID            int    `json:"domain_id"`
	LineageName         string `json:"lineage_name"`
	AutoRenew           bool   `json:"auto_renew"`
}

type SetSiteCertificateAutoRenewResponse struct {
	Changed bool   `json:"changed"`
	Error   string `json:"error,omitempty"`
}

// SnapshotSiteCertificateRequest copies the certificate a native lineage's
// live files name now into the immutable snapshot store, for the mail SNI
// map (whose contract names immutable snapshots only).
type SnapshotSiteCertificateRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Domain              string `json:"domain"`
	DomainID            int    `json:"domain_id"`
	LineageName         string `json:"lineage_name"`
}

type SnapshotSiteCertificateResponse struct {
	CertPath  string `json:"cert_path"`
	KeyPath   string `json:"key_path"`
	ChainPath string `json:"chain_path"`
	Error     string `json:"error,omitempty"`
}

// ErrNativeSiteRenewal is the Agent's answer when the Panel asks it to renew
// an OS-renewed lineage: only certbot's own schedule renews it.
const ErrNativeSiteRenewal = "this certificate is renewed by the operating system's certbot schedule, not by CelikPanel"

// NativeSiteLineageName builds a lineage name (used by the Agent).
func NativeSiteLineageName(domainID int, random []byte) (string, error) {
	if domainID <= 0 || len(random) != 12 {
		return "", fmt.Errorf("invalid native site lineage identity")
	}
	return fmt.Sprintf("%s%d-%x", NativeSiteLineagePrefix, domainID, random), nil
}
