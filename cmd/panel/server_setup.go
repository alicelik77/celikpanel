package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

const serverSetupPath = "/api/v1/setup"

var errServerSetupConflict = errors.New("server setup changed; reload the current plan")

// Explicit zone scope is independent of the default DNS mode for new domains.
// Optional fields preserve the identity of previously accepted plans.
type serverSetupInfrastructureDNSDraft struct {
	Zone            string `json:"zone"`
	PeerPanelDomain string `json:"peer_panel_domain,omitempty"`
	// PeerMailHostname is additive (2026-10-16, the owner's decision that the
	// second server can be a full web and mail peer): the other server's mail
	// host name inside this zone, prepared as an A record to the peer address.
	// An older plan has no such field and keeps its identity.
	// Diger sunucunun posta adi; bu bolgede esin adresine A kaydi olarak
	// hazirlanir. Eski planda alan yoktur, kimligi degismez.
	PeerMailHostname string `json:"peer_mail_hostname,omitempty"`
}

type serverSetupDraft struct {
	InfrastructureDNS     *serverSetupInfrastructureDNSDraft `json:"infrastructure_dns,omitempty"`
	Purpose               string                             `json:"purpose"`
	DNSHostingManagement  string                             `json:"dns_hosting_management,omitempty"`
	DNSPublisherEndpoint  string                             `json:"dns_publisher_endpoint,omitempty"`
	RemoteDNSConnectionID string                             `json:"remote_dns_connection_id"`
	PanelDomain           string                             `json:"panel_domain"`
	MailHostname          string                             `json:"mail_hostname"`
	DNSMode               string                             `json:"dns_mode"`
	DNSEngine             string                             `json:"dns_engine"`
	DNSRole               string                             `json:"dns_role"`
	NS1                   string                             `json:"ns1"`
	NS2                   string                             `json:"ns2"`
	LocalIP               string                             `json:"local_ip"`
	PeerIP                string                             `json:"peer_ip"`
	PeerNS                string                             `json:"peer_ns"`
	NodeVersion           string                             `json:"node_version"`
	Database              string                             `json:"database"`
	Customization         *serverSetupCustomization          `json:"customization,omitempty"`
	// The wizard's own answers (alpha.111, setup wizard v2), all additive and
	// omitted when empty, so a draft saved before them serializes, and its
	// reviewed plan hashes, exactly as before. MainDomain is the owner's main
	// domain, asked once and first; with local DNS on the main DNS server it
	// is the infrastructure zone (canonicalServerSetupDraft). ServerCount is
	// 1 or 2. ShortName is the server's own short name ("Bu sunucunun kısa
	// adı"), never guessed by the panel. MainDNSServer is "this" when this
	// server is the main DNS server; no other value exists in this release.
	// Sihirbazin yeni yanitlari; bossa yazilmaz, eski taslagin plani ayni kalir.
	MainDomain    string `json:"main_domain,omitempty"`
	ServerCount   int    `json:"server_count,omitempty"`
	ShortName     string `json:"short_name,omitempty"`
	MainDNSServer string `json:"main_dns_server,omitempty"`
	// alpha.112: the main domain's public records the owner reviewed (each
	// imported or skipped) and the mail DNS choice (server_setup_mail_dns.go).
	ImportedRecords []serverSetupImportedRecord `json:"imported_records,omitempty"`
	MailDNS         *serverSetupMailDNS         `json:"mail_dns,omitempty"`
	// alpha.114: on the main DNS server, the other server's main-domain mail
	// (its mail host and MX/DMARC answers) when that server, not this one,
	// receives the main domain's mail. Taken from the join, never typed.
	PeerMail *serverSetupPeerMail `json:"peer_mail,omitempty"`
}

// Reason and Vars are additive (2026-10-10): the typed cause of a check that
// needs an action, and the observed values the screen names (an address, a
// host name, the reverse DNS name found). A record written before them has
// neither and keeps its generic sentence.
// Reason ve Vars ekleme alanlardir: eylem isteyen kontrolun tipli nedeni ve
// ekranin andigi gozlenen degerler. Eski kayitta yoktur; genel cumle kalir.
type serverSetupCheck struct {
	ID     string            `json:"id"`
	State  string            `json:"state"`
	Code   string            `json:"code"`
	Reason string            `json:"reason,omitempty"`
	Vars   map[string]string `json:"vars,omitempty"`
	// Advisory (alpha.112, owner decision 2026-10-28): an open task setup does
	// not wait for. Reverse DNS (PTR) is set at the server provider, never by
	// CelikPanel; mail works without it but may be marked as spam, so a PTR
	// that is missing, differs or could not be read is said and kept open,
	// and setup completes without it.
	// Kurulumun beklemedigi acik is: ters DNS saglayicida ayarlanir.
	Advisory bool `json:"advisory,omitempty"`
}

type serverSetupState struct {
	Version     int                `json:"version"`
	Revision    int                `json:"revision"`
	Origin      string             `json:"origin"`
	Status      string             `json:"status"`
	Draft       serverSetupDraft   `json:"draft"`
	CompletedAt string             `json:"completed_at,omitempty"`
	Required    bool               `json:"required"`
	Guidance    string             `json:"guidance"`
	Checks      []serverSetupCheck `json:"checks"`
	ServerIP    string             `json:"server_ip,omitempty"`
	// Extension and ExtensionBase are additive (2026-10-16). A completed setup
	// that the owner extends keeps its completion record (completed_at) while
	// the new draft is reviewed and run: the server stays prepared, nothing
	// forces the owner back into setup, and ExtensionBase is the draft as it
	// was completed, so the review can show only what changes and stopping the
	// extension restores it. An older page ignores both fields.
	// Tamamlanmis kurulumu genisletme: tamamlanma kaydi korunur; taban taslak
	// yalniz degisenleri gostermek ve genisletmeyi durdurmak icin tutulur.
	Extension     bool              `json:"extension,omitempty"`
	ExtensionBase *serverSetupDraft `json:"extension_base,omitempty"`
	// ShortNameSuggestion (alpha.111) is the machine host name's first label
	// when it reads like a name a person chose; otherwise empty
	// (serverSetupShortNameSuggestion). Only the GET answer carries it.
	// Kisa ad onerisi; yalniz GET yanitinda.
	ShortNameSuggestion string `json:"short_name_suggestion,omitempty"`
	// ServerIPv6 (alpha.112) is the server's public IPv6 address, for the
	// reverse DNS rows the owner gives the provider; only the GET carries it.
	ServerIPv6 string `json:"server_ipv6,omitempty"`
	// InstallationID identifies this panel installation (the GET answer
	// carries it). A browser start marker from another installation, for
	// example one the same address had before it was wiped and installed
	// again, is dropped when this differs (serverSetupInstallationID).
	// Kurulum kimligi; baska kurulumdan kalan baslatma isaretini ayirt eder.
	InstallationID string `json:"installation_id,omitempty"`
}

func defaultServerSetupDraft() serverSetupDraft {
	return serverSetupDraft{Purpose: "web", DNSMode: "local", DNSEngine: "bind", DNSRole: "primary", Database: "mariadb"}
}

// Drafts may be incomplete; review validates the complete executable plan.
// Taslak eksik olabilir; inceleme calistirilacak tam plani dogrular.
func canonicalServerSetupDraft(d serverSetupDraft) (serverSetupDraft, error) {
	if d.InfrastructureDNS != nil {
		selection := *d.InfrastructureDNS
		selection.Zone = strings.TrimSpace(selection.Zone)
		selection.PeerPanelDomain = strings.TrimSpace(selection.PeerPanelDomain)
		selection.PeerMailHostname = strings.TrimSpace(selection.PeerMailHostname)
		for _, value := range []*string{&selection.Zone, &selection.PeerPanelDomain, &selection.PeerMailHostname} {
			if *value == "" {
				continue
			}
			canonical, err := hostname.CanonicalFQDN(*value)
			if err != nil {
				return d, errors.New("infrastructure DNS names must be fully qualified")
			}
			*value = canonical
		}
		d.InfrastructureDNS = &selection
	}
	for _, item := range []*string{&d.Purpose, &d.PanelDomain, &d.MailHostname, &d.DNSMode, &d.DNSEngine, &d.DNSRole, &d.NS1, &d.NS2, &d.LocalIP, &d.PeerIP, &d.PeerNS, &d.NodeVersion, &d.Database, &d.RemoteDNSConnectionID, &d.DNSPublisherEndpoint, &d.DNSHostingManagement} {
		*item = strings.TrimSpace(*item)
		if len(*item) > 253 {
			return d, errors.New("setup input is too long")
		}
	}
	if d.DNSEngine == "" {
		d.DNSEngine = "bind"
	}
	if d.DNSRole == "" {
		d.DNSRole = "primary"
	}
	if !stringIn(d.Purpose, "web", "web_mail", "application", "dns", "custom") || !stringIn(d.DNSMode, "local", "existing", "external") || !stringIn(d.DNSEngine, "", "pdns", "bind") || !stringIn(d.DNSRole, "", "primary", "secondary") || !stringIn(d.Database, "", "mariadb", "postgresql") {
		return d, errors.New("invalid setup choice")
	}
	for _, value := range []*string{&d.PanelDomain, &d.MailHostname, &d.NS1, &d.NS2, &d.PeerNS} {
		if *value == "" {
			continue
		}
		name, err := hostname.CanonicalFQDN(*value)
		if err != nil {
			return d, errors.New("setup names must be fully qualified domain names")
		}
		*value = name
	}
	for _, value := range []*string{&d.LocalIP, &d.PeerIP} {
		if *value == "" {
			continue
		}
		ip := net.ParseIP(*value)
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
			return d, errors.New("invalid setup address")
		}
		*value = ip.String()
	}
	if !stringIn(d.DNSHostingManagement, "", "manual", "panel") {
		return d, errors.New("invalid hosting DNS management choice")
	}
	if d.DNSPublisherEndpoint != "" {
		endpoint, err := canonicalRemoteDNSEndpoint(d.DNSPublisherEndpoint)
		if err != nil {
			return d, errors.New("invalid DNS publisher HTTPS endpoint")
		}
		d.DNSPublisherEndpoint = endpoint
	}
	if d.RemoteDNSConnectionID != "" && !validServiceOperationID(d.RemoteDNSConnectionID) {
		return d, errors.New("invalid remote DNS connection")
	}
	for _, c := range d.NodeVersion {
		if !(c >= '0' && c <= '9') && c != '.' && c != 'v' {
			return d, errors.New("invalid runtime version")
		}
	}
	var err error
	d.Customization, err = canonicalServerSetupCustomization(d.Customization)
	if err != nil {
		return d, err
	}
	if d.Purpose == "custom" && d.Customization == nil {
		d.Customization = &serverSetupCustomization{Components: []string{}}
	}
	if err := serverSetupVersionChoicesSelected(d); err != nil {
		return d, err
	}
	if err := canonicalServerSetupWizardAnswers(&d); err != nil {
		return d, err
	}
	if err := canonicalServerSetupMailAnswers(&d); err != nil {
		return d, err
	}
	return d, nil
}

var serverSetupShortNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// canonicalServerSetupWizardAnswers checks the wizard v2 answers. A draft
// without a main domain (every draft saved before alpha.111) is left exactly
// as it is. With one, the main domain is the infrastructure zone whenever this
// server is the main DNS server with local DNS: the owner named it as the
// first question, so the zone is no longer an unticked option on another
// screen. A backup DNS server or another DNS home prepares no zone here.
// Sihirbaz v2 yanitlari: ana alan adi yoksa taslaga dokunulmaz; varsa ve bu
// sunucu yerel ana DNS sunucusuysa bolge ana alan adidir.
func canonicalServerSetupWizardAnswers(d *serverSetupDraft) error {
	d.MainDomain = strings.TrimSpace(d.MainDomain)
	d.ShortName = strings.ToLower(strings.TrimSpace(d.ShortName))
	d.MainDNSServer = strings.TrimSpace(d.MainDNSServer)
	if d.MainDomain != "" {
		canonical, err := hostname.CanonicalFQDN(d.MainDomain)
		if err != nil {
			return errors.New("the main domain must be a fully qualified domain name")
		}
		d.MainDomain = canonical
	}
	if d.ShortName != "" && !serverSetupShortNamePattern.MatchString(d.ShortName) {
		return errors.New("the server's short name must be one DNS label")
	}
	if d.ServerCount < 0 || d.ServerCount > 2 {
		return errors.New("setup supports one or two servers")
	}
	if !stringIn(d.MainDNSServer, "", "this", "peer") {
		return errors.New("invalid main DNS server choice")
	}
	if d.MainDomain == "" {
		return nil
	}
	// One server keeping its own DNS is standalone: no peer, both names here.
	// With two servers the main DNS server is this one or the second one
	// (alpha.114, Option A): ns1 is always the main, ns2 the backup.
	if d.DNSMode == setupDNSModeLocal && d.ServerCount == 1 {
		d.DNSRole, d.PeerIP, d.PeerNS, d.MainDNSServer = transport.DNSPairRolePrimary, "", "", "this"
		if d.InfrastructureDNS != nil {
			d.InfrastructureDNS.PeerPanelDomain, d.InfrastructureDNS.PeerMailHostname = "", ""
		}
	}
	if d.DNSMode == setupDNSModeLocal && d.ServerCount == 2 && d.MainDNSServer == "peer" {
		d.DNSRole = transport.DNSPairRoleSecondary
	}
	if d.PeerMail != nil {
		mail := *d.PeerMail
		canonical, err := hostname.CanonicalFQDN(strings.TrimSpace(mail.Hostname))
		if err != nil || !stringIn(mail.MX, "", "now", "later") || !stringIn(mail.DMARC, "", "keep", "add") {
			return errors.New("invalid peer mail answer")
		}
		mail.Hostname = canonical
		d.PeerMail = &mail
		// The join's shared answer is kept even when this server selects
		// mail too: the review then refuses two receivers
		// (server_setup_peer_mail_both) instead of silently dropping the
		// answer (Beta-1 third re-run N3/N5).
		if d.DNSMode != setupDNSModeLocal || d.DNSRole != transport.DNSPairRolePrimary {
			d.PeerMail = nil
		}
	}
	if d.DNSMode == setupDNSModeLocal && d.DNSRole == transport.DNSPairRolePrimary {
		selection := serverSetupInfrastructureDNSDraft{Zone: d.MainDomain}
		if d.InfrastructureDNS != nil {
			selection.PeerPanelDomain = d.InfrastructureDNS.PeerPanelDomain
			selection.PeerMailHostname = d.InfrastructureDNS.PeerMailHostname
		}
		d.InfrastructureDNS = &selection
	} else {
		d.InfrastructureDNS = nil
	}
	return nil
}

func stringIn(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func (p *Panel) loadServerSetup(ctx context.Context) (serverSetupState, error) {
	var s serverSetupState
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT version,revision,origin,status,draft_json,completed_at,COALESCE((SELECT value FROM panel_settings WHERE key='server_setup_guidance'),'') FROM server_setup_state WHERE id=1`).Scan(&s.Version, &s.Revision, &s.Origin, &s.Status, &raw, &s.CompletedAt, &s.Guidance)
	if err != nil {
		return s, fmt.Errorf("read server setup: %w", err)
	}
	if s.Version != 1 || s.Revision < 0 || !stringIn(s.Origin, "fresh", "legacy") || !stringIn(s.Status, "new", "legacy", "draft", "running", "waiting", "failed", "ready") {
		return s, errors.New("invalid persisted setup state")
	}
	s.Draft = defaultServerSetupDraft()
	if err := json.Unmarshal([]byte(raw), &s.Draft); err != nil {
		return s, errors.New("invalid persisted setup draft")
	}
	s.Draft, err = canonicalServerSetupDraft(s.Draft)
	if err != nil {
		return s, err
	}
	s.Extension = s.CompletedAt != "" && stringIn(s.Status, "draft", "running", "waiting", "failed")
	s.Required = s.Status != "legacy" && s.Status != "ready" && !s.Extension
	if s.Extension {
		s.ExtensionBase = p.serverSetupExtensionBase(ctx)
	}
	if s.Guidance == "" {
		s.Guidance = "guided"
		if stringIn(s.Status, "new", "legacy") {
			s.Guidance = "undecided"
		}
	}
	if !stringIn(s.Guidance, "undecided", "guided", "manual") {
		return s, errors.New("invalid persisted setup guidance")
	}
	s.Checks = []serverSetupCheck{}
	return s, nil
}

func (p *Panel) saveServerSetupDraft(ctx context.Context, revision int, draft serverSetupDraft) (serverSetupState, error) {
	p.serverSetupMu.Lock()
	defer p.serverSetupMu.Unlock()
	canonical, err := canonicalServerSetupDraft(draft)
	if err != nil {
		return serverSetupState{}, err
	}
	active, err := p.serverSetupExecutionActive(ctx)
	if err != nil {
		return serverSetupState{}, err
	}
	if active {
		return serverSetupState{}, errServerSetupConflict
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return serverSetupState{}, err
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return serverSetupState{}, err
	}
	defer tx.Rollback()
	// A draft saved over a completed setup extends it: the completed draft is
	// kept, as stored, as the extension's base (see handleServerSetupExtend).
	// Tamamlanmis kurulumun uzerine kaydedilen taslak onu genisletir.
	var currentStatus, currentDraft string
	if err := tx.QueryRowContext(ctx, `SELECT status,draft_json FROM server_setup_state WHERE id=1 AND revision=?`, revision).Scan(&currentStatus, &currentDraft); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return serverSetupState{}, errServerSetupConflict
		}
		return serverSetupState{}, err
	}
	if currentStatus == "ready" {
		if err := saveServerSetupExtensionBase(ctx, tx, currentDraft); err != nil {
			return serverSetupState{}, err
		}
	}
	// The state CAS also protects against another panel process starting the
	// reviewed plan. / CAS baska panel isleminin ayni plani baslatmasini da korur.
	result, err := tx.ExecContext(ctx, `UPDATE server_setup_state SET draft_json=?,revision=revision+1,status='draft',updated_at=datetime('now') WHERE id=1 AND revision=? AND status IN ('new','legacy','draft','failed','ready')`, string(raw), revision)
	if err != nil {
		return serverSetupState{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return serverSetupState{}, err
	}
	if count != 1 {
		return serverSetupState{}, errServerSetupConflict
	}
	if err := tx.Commit(); err != nil {
		return serverSetupState{}, err
	}
	return p.loadServerSetup(ctx)
}

func serverSetupChecksReady(checks []serverSetupCheck) bool {
	blocking := 0
	for _, check := range checks {
		if check.Advisory {
			continue
		}
		blocking++
		if check.State != "ready" {
			return false
		}
	}
	return blocking > 0
}

func (p *Panel) completeServerSetup(ctx context.Context, revision int) error {
	return p.completeServerSetupOwned(ctx, revision, "")
}

func (p *Panel) completeServerSetupExecution(ctx context.Context, revision int, executionID string) error {
	if !validServiceOperationID(executionID) {
		return errServerSetupConflict
	}
	return p.completeServerSetupOwned(ctx, revision, executionID)
}

func (p *Panel) completeServerSetupOwned(ctx context.Context, revision int, executionID string) error {
	p.serverSetupMu.Lock()
	defer p.serverSetupMu.Unlock()
	s, err := p.loadServerSetup(ctx)
	if err != nil {
		return err
	}
	if s.Revision != revision {
		return errServerSetupConflict
	}
	if s.Status == "ready" {
		// Ready here; a backup whose main was not told yet tells it now
		// (review fix, item 6), then its link side ends.
		return p.tellMainAndSettleServerSetupLink(ctx, s.Draft)
	}
	active, err := p.serverSetupExecutionActive(ctx)
	if err != nil {
		return err
	}
	if executionID != "" {
		var own int
		err := p.db.GetDB().QueryRowContext(ctx, `SELECT count(*) FROM server_setup_executions e JOIN server_setup_plans p ON p.id=e.plan_id WHERE e.id=? AND e.status IN ('running','waiting') AND p.revision=?`, executionID, revision).Scan(&own)
		if err != nil {
			return err
		}
		if own != 1 {
			return errServerSetupConflict
		}
		var children int
		if err := p.db.GetDB().QueryRowContext(ctx, `SELECT count(*) FROM service_operations WHERE status IN ('queued','running')`).Scan(&children); err != nil {
			return err
		}
		if children != 0 {
			return errServerSetupConflict
		}
	} else if active {
		return errServerSetupConflict
	}
	checked, err := p.serverSetupCompletionDraft(ctx, s, executionID)
	if err != nil {
		return err
	}
	checks, err := p.serverSetupCompletionChecks(ctx, checked)
	if err != nil {
		return err
	}
	if !serverSetupChecksReady(checks) {
		return errors.New("setup prerequisites are not ready")
	}
	// Recheck ownership in the committing statement: another panel process
	// may have admitted an execution while readiness was being read.
	result, err := p.db.GetDB().ExecContext(ctx, `UPDATE server_setup_state SET status='ready',completed_at=?,updated_at=datetime('now')
		WHERE id=1 AND revision=?
		AND NOT EXISTS (SELECT 1 FROM server_setup_executions WHERE status IN ('running','waiting') AND id != ?)
		AND NOT EXISTS (SELECT 1 FROM service_operations WHERE status IN ('queued','running'))`, time.Now().UTC().Format(time.RFC3339), revision, executionID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errServerSetupConflict
	}
	// A finished extension has no base any more. A leftover row is harmless
	// (the next extension replaces it), so a failed delete is only logged.
	// Biten genisletmenin tabani silinir; kalan satir zararsizdir.
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, settingServerSetupExtensionBase); err != nil {
		log.Printf("server setup extension base not cleared: %v", err)
	}
	// Durably ready here: only now the backup tells the main DNS server,
	// and only once it was told does its side of the link end. A report that
	// fails is the wait "ready here, main not told yet"; the runner (or the
	// owner's own completion) reports again (review fix, item 6).
	return p.tellMainAndSettleServerSetupLink(ctx, checked)
}

// tellMainAndSettleServerSetupLink runs only after this server's setup is
// ready. The setup link ends with the setup (alpha.114), except on the main
// DNS server while its backup has not reported ready (Beta-1 re-run P7). A
// failed settle is logged; the next completion or a new join removes it.
func (p *Panel) tellMainAndSettleServerSetupLink(ctx context.Context, d serverSetupDraft) error {
	if err := p.reportServerSetupReadyToMain(ctx, d); err != nil {
		return err
	}
	if err := p.settleServerSetupLink(ctx, d); err != nil {
		log.Printf("server setup link not revoked: %v", err)
	}
	return nil
}

// serverSetupCompletionDraft is the draft completion is checked on. A run
// whose second server is bound (alpha.114) is checked on the plan that
// binding completed (the peer's address is not in the saved draft); it never
// falls back to the unbound draft. A plan that no longer matches the saved
// answers, or one that cannot be read, is not complete: a wait with its
// reason (D-024), checked again by the runner.
func (p *Panel) serverSetupCompletionDraft(ctx context.Context, s serverSetupState, executionID string) (serverSetupDraft, error) {
	if executionID == "" {
		return s.Draft, nil
	}
	unknown := &serverSetupPeerWait{Code: "server_setup_completion_unknown", Message: "Whether setup is complete could not be checked: its plan or the second server's binding could not be read. Nothing to do here; setup checks again by itself."}
	execution, err := p.latestServerSetupExecution(ctx)
	if err != nil || execution == nil || execution.ID != executionID {
		return s.Draft, unknown
	}
	plan, err := p.loadServerSetupPlan(ctx, execution.PlanID)
	if err != nil {
		return s.Draft, unknown
	}
	if !serverSetupPlanHasStep(plan, "peer_join") {
		return s.Draft, nil
	}
	if !reflect.DeepEqual(plan.Draft, s.Draft) {
		return s.Draft, &serverSetupPeerWait{Code: "server_setup_completion_plan_changed", Message: "This server's saved answers no longer match the plan this setup runs, so setup is not marked complete. Review the plan again on this screen; setup checks again by itself."}
	}
	bound, err := p.effectiveServerSetupPlan(ctx, plan, execution.ID)
	if err != nil || !bound.peerBound {
		return s.Draft, unknown
	}
	return bound.Draft, nil
}

func requireServerSetupAdmin(w http.ResponseWriter, r *http.Request) bool {
	c := currentCaller(r)
	if c == nil || !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return false
	}
	return true
}

func (p *Panel) handleServerSetup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s, err := p.loadServerSetup(r.Context())
		if err != nil {
			writeServerError(w, err)
			return
		}
		if address := net.ParseIP(serverPrimaryIP()); address != nil && address.IsGlobalUnicast() && !address.IsPrivate() && !address.IsLoopback() {
			s.ServerIP = address.String()
		}
		s.ShortNameSuggestion = currentServerSetupShortNameSuggestion()
		// Without an identity (an unreadable settings row) the field is left
		// out and the page keeps its older behaviour; it is never invented.
		if id, err := p.serverSetupInstallationID(r.Context()); err == nil {
			s.InstallationID = id
		}
		if address := net.ParseIP(setupMailIPv6()); address != nil && address.To4() == nil && address.IsGlobalUnicast() && !address.IsPrivate() {
			s.ServerIPv6 = address.String()
		}
		if r.URL.Query().Get("check") == "1" {
			ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
			defer cancel()
			s.Checks, err = p.serverSetupCompletionChecks(ctx, s.Draft)
			if err != nil {
				writeServerError(w, err)
				return
			}
			// Only the owner's explicit "Check requirements again" (manual=1)
			// shortens the waiting run's sleep, and only when every check
			// passed. It never completes setup here: the runner rechecks and
			// completes in its own single pass. Page polls do not send manual=1.
			// Yalniz sahibin acik kontrolu bekleyen calisani uyandirir.
			if r.URL.Query().Get("manual") == "1" {
				p.wakeServerSetupVerification(ctx, s.Checks)
			}
		}
		json.NewEncoder(w).Encode(s)
	case http.MethodPut:
		var req struct {
			Revision int              `json:"revision"`
			Draft    serverSetupDraft `json:"draft"`
		}
		if err := decodeServiceOperationJSON(w, r, &req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid setup draft")
			return
		}
		if _, err := canonicalServerSetupDraft(req.Draft); err != nil {
			writeClientError(w, http.StatusBadRequest, err.Error())
			return
		}
		s, err := p.saveServerSetupDraft(r.Context(), req.Revision, req.Draft)
		if errors.Is(err, errServerSetupConflict) {
			writeCodedError(w, http.StatusConflict, "setup_conflict", err.Error(), "/setup")
			return
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
		p.audit(r, "server.setup.draft", "", 0)
		json.NewEncoder(w).Encode(s)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (p *Panel) handleServerSetupComplete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Revision int `json:"revision"`
	}
	if err := decodeServiceOperationJSON(w, r, &req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := p.completeServerSetup(r.Context(), req.Revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeServerError(w, err)
			return
		}
		// A wait keeps its own reason (review fix, item 9): for example a
		// backup ready here whose main DNS server was not told yet.
		var wait *serverSetupPeerWait
		if errors.As(err, &wait) {
			writeCodedError(w, http.StatusConflict, wait.Code, wait.Message, "/setup")
			return
		}
		writeCodedError(w, http.StatusConflict, "setup_not_ready", "complete the outstanding setup checks before continuing", "/setup")
		return
	}
	p.audit(r, "server.setup.complete", "", 0)
	state, err := p.loadServerSetup(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	json.NewEncoder(w).Encode(state)
}
