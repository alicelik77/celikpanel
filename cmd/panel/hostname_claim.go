package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/user"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// One name rule (security release, the owner's rule of 20 November 2026:
// only the administrator ever has root; a reseller or a customer never
// reaches anything of another customer or of the server itself).
//
// A name a subscription receives (a new domain or subdomain, an alias, an
// imported domain, every name of a subscription that changes owner) is
// refused when it equals, lies under or lies above
//
//   - a name held by ANOTHER account: a domain, its implicit www and mail
//     names or an alias (hostname_reservations) whose subscription has a
//     different owner. "Account" is the owning account of the subscription
//     that receives the name, not the person asking: a reseller acting for
//     its customer is held to that customer's names, and a reseller's own
//     domains are another account for its customers;
//   - a name of the server itself: the infrastructure zones setup created,
//     every zone hosted here that no domain holds, the panel's and the
//     machine's host names, the nameserver, mail and webmail host names, the
//     paired server's names and the main domain.
//
// A subscription owned by an administrator account may hold any of these on
// purpose (the administrator's deliberate placement, for example the main
// domain's website that takes over the infrastructure zone); every other
// subscription never. Names of zones this server publishes for a connected
// server (remote DNS) are refused for everyone, as migration 042 already
// does. A name under one's own domain stays an ordinary subdomain.
//
// The same rule is enforced a second time by the database (migration 080)
// for the names it can see itself: other accounts' names, zones without a
// domain, the infrastructure zones and the stored nameserver and mail host
// names.
//
// A refusal never tells an ordinary account who holds a name or whether the
// server reserves it: "held by another account", "reserved by the server"
// and "its system account exists" get one equal answer with what the person
// can do. Only an administrator, who sees every account anyway, is told the
// exact reason and the other name.
//
// Tek ad kurali: bir ad baska bir hesabin adina ya da sunucunun kendi
// adlarina esit olamaz, altinda ya da ustunde kalamaz. Yonetici hesabinin
// aboneligi bilerek tutabilir; baska hicbir abonelik tutamaz. Ret, adi kimin
// tuttugunu soylemez.

const errCodeHostnameNotAvailable = "HOSTNAME_NOT_AVAILABLE"

// Reasons of a refused name. Only own_name and the administrator's answers
// carry a reason; an ordinary account's refusal carries none.
const (
	hostnameClaimOwnName      = "own_name"
	hostnameClaimOtherAccount = "other_account"
	hostnameClaimServerName   = "server_name"
	hostnameClaimRemoteZone   = "remote_zone"
	hostnameClaimAccountTaken = "account_taken"
)

// hostnameClaimRefusal is why a name cannot be given to a subscription.
type hostnameClaimRefusal struct {
	Reason string
	// Name is the requested name; Other the name it meets (another
	// account's name, a server name, a zone) or, for account_taken, the
	// system account; Holder the account that holds Other (administrator
	// answers only).
	Name, Other, Holder string
}

// Is: a refused alias is still an alias conflict for code that asks so.
func (r *hostnameClaimRefusal) Is(target error) bool { return target == errDomainAliasConflict }

func (r *hostnameClaimRefusal) Error() string {
	return "hostname namespace conflict: the name is not available (" + r.Reason + ")"
}

// hostnamesOverlap: a equals b, lies under it or above it.
func hostnamesOverlap(a, b string) bool {
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

// serverHostname is one name of the server itself and where it was read.
type serverHostname struct {
	Name   string
	Source string
}

// Seams: tests never read the machine's own names or accounts.
var (
	claimLocalServerName       = func() (string, error) { return localServerName() }
	claimPanelCertificateName  = func() string { return panelCertificateHostname() }
	claimSystemAccountExists   = func(name string) bool { _, err := user.Lookup(name); return err == nil }
	errHostnameClaimNoOwner    = errors.New("the subscription that receives the name was not found")
	hostnameClaimSettingSource = []string{settingNS1, settingNS2, settingDNSPeerNS, settingMailHostname}
)

// serverOwnHostnames lists the names of the server itself. Every source is
// read; one that cannot be parsed is skipped (it names nothing), one that
// cannot be read is an error, so a refusal is never skipped by a failed read.
func (p *Panel) serverOwnHostnames(ctx context.Context) ([]serverHostname, error) {
	seen := map[string]bool{}
	var names []serverHostname
	add := func(raw, source string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		name, err := hostname.CanonicalFQDN(raw)
		if err != nil || !strings.Contains(name, ".") || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, serverHostname{Name: name, Source: source})
	}
	db := p.db.GetDB()
	for _, key := range hostnameClaimSettingSource {
		var value string
		err := db.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, key).Scan(&value)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		add(value, key)
	}
	if webmail, ok := p.webmailAddress(ctx); ok {
		add(webmail.Host, settingWebmailAddress)
	}
	rows, err := db.QueryContext(ctx, `SELECT key FROM panel_settings WHERE key LIKE ?`, serverSetupInfrastructureDNSOwnerPrefix+"%")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		add(strings.TrimPrefix(key, serverSetupInfrastructureDNSOwnerPrefix), "infrastructure_zone")
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	// A zone hosted here that no domain holds is the server's: an
	// infrastructure zone, a zone adopted from an earlier DNS server, or
	// one a connected server publishes here.
	rows, err = db.QueryContext(ctx, `SELECT z.name FROM pdns_domains z WHERE NOT EXISTS (SELECT 1 FROM domains d WHERE d.name = z.name)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var zone string
		if err := rows.Scan(&zone); err != nil {
			rows.Close()
			return nil, err
		}
		add(zone, "zone_without_domain")
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	// Setup's names: the draft, the completed draft an extension started
	// from and the latest run's plan (a union that protects more).
	drafts := []serverSetupDraft{}
	state, err := p.loadServerSetup(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		drafts = append(drafts, state.Draft)
		if state.ExtensionBase != nil {
			drafts = append(drafts, *state.ExtensionBase)
		}
		if execution, err := p.latestServerSetupExecution(ctx); err == nil && execution != nil {
			if plan, err := p.loadServerSetupPlan(ctx, execution.PlanID); err == nil {
				drafts = append(drafts, plan.Draft)
			}
		}
	}
	for _, d := range drafts {
		add(d.PanelDomain, "setup")
		add(d.MailHostname, "setup")
		add(d.NS1, "setup")
		add(d.NS2, "setup")
		add(d.PeerNS, "setup")
		add(d.MainDomain, "setup")
		add(d.HostName, "setup")
		if d.InfrastructureDNS != nil {
			add(d.InfrastructureDNS.Zone, "setup")
			add(d.InfrastructureDNS.PeerPanelDomain, "setup")
			add(d.InfrastructureDNS.PeerMailHostname, "setup")
		}
		if d.PeerMail != nil {
			add(d.PeerMail.Hostname, "setup")
		}
	}
	if name, err := claimLocalServerName(); err == nil {
		add(name, "machine")
	}
	add(claimPanelCertificateName(), "panel_certificate")
	return names, nil
}

// subscriptionOwner is the account that owns subscriptionID and whether it
// is an administrator account.
func subscriptionOwner(ctx context.Context, q rowQueryer, subscriptionID int) (int, bool, error) {
	var owner int
	var role string
	err := q.QueryRowContext(ctx, `SELECT s.owner_id, COALESCE(u.role, '') FROM subscriptions s
		LEFT JOIN users u ON u.id = s.owner_id WHERE s.id = ?`, subscriptionID).Scan(&owner, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, errHostnameClaimNoOwner
	}
	if err != nil {
		return 0, false, err
	}
	return owner, role == roleAdmin, nil
}

// hostnameClaim is a name to be given to the subscriptions of OwnerID.
type hostnameClaim struct {
	Name         string
	OwnerID      int
	OwnerIsAdmin bool
	// SiteAccount: the name gets a site, so its derived system account
	// (services.SiteUsername) must be free too.
	SiteAccount bool
}

type claimReservation struct {
	hostname     string
	subscription int
	domainName   string
	ownerID      int
	ownerAdmin   bool
	ownerName    string
}

// reservationsOverlapping reads the names of every account that equal, lie
// under or lie above name.
func reservationsOverlapping(ctx context.Context, q rowQueryer, name string) ([]claimReservation, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT r.hostname, d.subscription_id, d.name, s.owner_id, COALESCE(u.role, ''), COALESCE(u.username, '')
		FROM hostname_reservations r
		JOIN domains d ON d.id = r.domain_id
		JOIN subscriptions s ON s.id = d.subscription_id
		LEFT JOIN users u ON u.id = s.owner_id
		WHERE r.hostname = ?1
		   OR substr(?1, -(length(r.hostname) + 1)) = '.' || r.hostname
		   OR substr(r.hostname, -(length(?1) + 1)) = '.' || ?1
		ORDER BY length(r.hostname), r.hostname`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []claimReservation
	for rows.Next() {
		var r claimReservation
		var role string
		if err := rows.Scan(&r.hostname, &r.subscription, &r.domainName, &r.ownerID, &role, &r.ownerName); err != nil {
			return nil, err
		}
		r.hostname = strings.ToLower(r.hostname)
		r.ownerAdmin = role == roleAdmin
		out = append(out, r)
	}
	return out, rows.Err()
}

// remoteZonesOverlapping: zones this server publishes for a connected
// server that equal, lie under or above name (refused for everyone, 042).
func remoteZonesOverlapping(ctx context.Context, q rowQueryer, name string) (string, error) {
	var zone string
	err := q.QueryRowContext(ctx, `SELECT zone_name FROM remote_dns_zone_ownership
		WHERE zone_name = ?1 OR substr(?1, -(length(zone_name) + 1)) = '.' || zone_name
		   OR substr(zone_name, -(length(?1) + 1)) = '.' || ?1
		ORDER BY length(zone_name) LIMIT 1`, name).Scan(&zone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return zone, err
}

// checkHostnameClaim applies the rule to one name. nil: the name may be
// given. Reads only.
func (p *Panel) checkHostnameClaim(ctx context.Context, claim hostnameClaim) (*hostnameClaimRefusal, error) {
	name, err := hostname.CanonicalFQDN(claim.Name)
	if err != nil {
		return nil, err
	}
	db := p.db.GetDB()
	if zone, err := remoteZonesOverlapping(ctx, db, name); err != nil {
		return nil, err
	} else if zone != "" {
		return &hostnameClaimRefusal{Reason: hostnameClaimRemoteZone, Name: name, Other: zone}, nil
	}
	held, err := reservationsOverlapping(ctx, db, name)
	if err != nil {
		return nil, err
	}
	for _, r := range held {
		if r.hostname == name && r.ownerID == claim.OwnerID {
			return &hostnameClaimRefusal{Reason: hostnameClaimOwnName, Name: name, Other: r.hostname}, nil
		}
	}
	for _, r := range held {
		// An exact name is one name whoever asks; a name around another
		// account's name only an administrator's subscription may hold.
		if r.ownerID != claim.OwnerID && (r.hostname == name || !claim.OwnerIsAdmin) {
			return &hostnameClaimRefusal{Reason: hostnameClaimOtherAccount, Name: name, Other: r.hostname, Holder: r.ownerName}, nil
		}
	}
	if !claim.OwnerIsAdmin {
		server, err := p.serverOwnHostnames(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range server {
			if hostnamesOverlap(name, s.Name) {
				return &hostnameClaimRefusal{Reason: hostnameClaimServerName, Name: name, Other: s.Name}, nil
			}
		}
	}
	if claim.SiteAccount {
		if refusal, err := p.siteAccountClaim(ctx, name); err != nil || refusal != nil {
			return refusal, err
		}
	}
	return nil, nil
}

// siteAccountClaim: the system account a site of name would get is free —
// no other domain derives the same account (services.SiteUsername maps "."
// and "-" to "_" and keeps 32 characters, so different names can share it)
// and no account of that name exists on this machine.
func (p *Panel) siteAccountClaim(ctx context.Context, name string) (*hostnameClaimRefusal, error) {
	account := services.SiteUsername(name)
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT name FROM domains`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return nil, err
		}
		if other != name && services.SiteUsername(strings.ToLower(other)) == account {
			return &hostnameClaimRefusal{Reason: hostnameClaimAccountTaken, Name: name, Other: account}, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if claimSystemAccountExists(account) {
		return &hostnameClaimRefusal{Reason: hostnameClaimAccountTaken, Name: name, Other: account}, nil
	}
	return nil, nil
}

// claimForSubscription checks name for the owner of subscriptionID.
func (p *Panel) claimForSubscription(ctx context.Context, subscriptionID int, name string, siteAccount bool) (*hostnameClaimRefusal, error) {
	owner, admin, err := subscriptionOwner(ctx, p.db.GetDB(), subscriptionID)
	if err != nil {
		return nil, err
	}
	return p.checkHostnameClaim(ctx, hostnameClaim{Name: name, OwnerID: owner, OwnerIsAdmin: admin, SiteAccount: siteAccount})
}

// hostnameClaimMessage is the English sentence of a refusal for this caller.
func hostnameClaimMessage(refusal *hostnameClaimRefusal, adminCaller bool) (reason, message string, vars map[string]string) {
	vars = map[string]string{"name": refusal.Name}
	if refusal.Reason == hostnameClaimOwnName {
		return hostnameClaimOwnName, fmt.Sprintf("You already have %s: it is one of your domains, its www or mail name, or an alias. Nothing was changed.", refusal.Name), vars
	}
	if !adminCaller {
		return "", fmt.Sprintf("%s cannot be added here: this name, a name above it or a name under it is already in use on this server or reserved by it. "+
			"Nothing was changed. If the name is yours, ask the server's administrator to check it. A subdomain of one of your own domains can always be added.", refusal.Name), vars
	}
	vars["other"] = refusal.Other
	if refusal.Holder != "" {
		vars["holder"] = refusal.Holder
	}
	switch refusal.Reason {
	case hostnameClaimOtherAccount:
		if refusal.Other == refusal.Name {
			return refusal.Reason, fmt.Sprintf("%s already belongs to the account %s. Nothing was changed. Remove or move it there first.", refusal.Name, refusal.Holder), vars
		}
		return refusal.Reason, fmt.Sprintf("%s meets %s of the account %s: a customer's or reseller's subscription never holds a name equal to, above or under another account's name. "+
			"Nothing was changed. Add it to the account that holds %s, or to your own subscription.", refusal.Name, refusal.Other, refusal.Holder, refusal.Other), vars
	case hostnameClaimServerName:
		return refusal.Reason, fmt.Sprintf("%s meets %s, one of this server's own names: only an administrator's own subscription may hold it. Nothing was changed.", refusal.Name, refusal.Other), vars
	case hostnameClaimRemoteZone:
		return refusal.Reason, fmt.Sprintf("%s meets %s, a zone this server publishes for a connected server. Nothing was changed. Use that zone on the server that publishes it.", refusal.Name, refusal.Other), vars
	case hostnameClaimAccountTaken:
		vars["account"] = refusal.Other
		return refusal.Reason, fmt.Sprintf("A site of %s would run as the system account %s, which another site or the system already has. Nothing was changed. Choose another name, or remove what uses that account first.", refusal.Name, refusal.Other), vars
	}
	return "", fmt.Sprintf("%s cannot be added here. Nothing was changed.", refusal.Name), vars
}

// writeHostnameClaimRefusal answers 409 HOSTNAME_NOT_AVAILABLE. An ordinary
// account's answer is the same whatever the cause (no reason, no other name).
func writeHostnameClaimRefusal(w http.ResponseWriter, caller *Caller, refusal *hostnameClaimRefusal) {
	reason, message, vars := hostnameClaimMessage(refusal, caller.hasAccountRole(roleAdmin))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: errCodeHostnameNotAvailable, Reason: reason, Vars: vars})
}

// genericHostnameClaimRefusal: the database refused the name (migration 080
// or 018) after the panel's own check passed; the cause is not known here.
func genericHostnameClaimRefusal(name string) *hostnameClaimRefusal {
	return &hostnameClaimRefusal{Reason: hostnameClaimOtherAccount, Name: name}
}

// --- Existing installations: overlaps made before this rule ---------------

// nameOverlapFinding is one name an ordinary account holds that the rule
// refuses today: another account's name or a server name meets it.
type nameOverlapFinding struct {
	DomainID   int
	DomainName string
	Name       string
	Other      string
	Server     bool
}

// existingNameOverlaps finds, read only, every name of an ordinary
// account's subscription that the rule would refuse today: one given after
// another account's name it meets (of two overlapping names the later one is
// the name that should not have been given; the reservation's row order is
// the order the names were given), or one that meets a server name. An
// administrator's deliberate placement is not listed. Nothing is changed:
// the administrator decides (Problems now).
func (p *Panel) existingNameOverlaps(ctx context.Context) ([]nameOverlapFinding, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT r.rowid, r.hostname, r.source_kind, d.id, d.name, s.owner_id, COALESCE(u.role, '')
		FROM hostname_reservations r
		JOIN domains d ON d.id = r.domain_id
		JOIN subscriptions s ON s.id = d.subscription_id
		LEFT JOIN users u ON u.id = s.owner_id`)
	if err != nil {
		return nil, err
	}
	type entry struct {
		order    int64
		name     string
		kind     string
		domainID int
		domain   string
		owner    int
		admin    bool
	}
	var all []entry
	byName := map[string][]int{}
	for rows.Next() {
		var e entry
		var role string
		if err := rows.Scan(&e.order, &e.name, &e.kind, &e.domainID, &e.domain, &e.owner, &role); err != nil {
			rows.Close()
			return nil, err
		}
		e.name = strings.ToLower(e.name)
		e.admin = role == roleAdmin
		byName[e.name] = append(byName[e.name], len(all))
		all = append(all, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	server, err := p.serverOwnHostnames(ctx)
	if err != nil {
		return nil, err
	}
	serverSet := map[string]bool{}
	for _, s := range server {
		serverSet[s.Name] = true
	}
	found := map[int]nameOverlapFinding{}
	report := func(e entry, other string, isServer bool) {
		if _, done := found[e.domainID]; done || e.admin {
			return
		}
		found[e.domainID] = nameOverlapFinding{DomainID: e.domainID, DomainName: e.domain, Name: e.name, Other: other, Server: isServer}
	}
	given := func(e entry) bool { return e.kind == "primary" || e.kind == "alias" }
	// Each name's ancestors: an ancestor of another account, or a server
	// name. The pair is reported on the side given later.
	for _, e := range all {
		if !given(e) {
			continue
		}
		for parent := e.name; strings.Contains(parent, "."); parent = parent[strings.Index(parent, ".")+1:] {
			if serverSet[parent] {
				report(e, parent, true)
			}
			if parent == e.name {
				continue
			}
			for _, i := range byName[parent] {
				o := all[i]
				if o.owner == e.owner {
					continue
				}
				if e.order > o.order {
					report(e, o.name, false)
				} else if given(o) {
					report(o, e.name, false)
				}
			}
		}
	}
	// A server name under an ordinary account's name.
	for _, s := range server {
		for parent := s.Name; strings.Contains(parent, "."); parent = parent[strings.Index(parent, ".")+1:] {
			if parent == s.Name {
				continue
			}
			for _, i := range byName[parent] {
				if given(all[i]) {
					report(all[i], s.Name, true)
				}
			}
		}
	}
	out := make([]nameOverlapFinding, 0, len(found))
	for _, f := range found {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DomainID < out[j].DomainID })
	return out, nil
}

// --- A subscription that changes owner --------------------------------------

const errCodeTransferNameOverlap = "TRANSFER_NAME_OVERLAP"

// transferNameOverlaps lists, read only through q, every name of
// subscription subID that the receiving account may not hold: it equals,
// lies under or above a name of another subscription whose owner is not
// the receiver, or one of the server's own names (a receiver is never an
// administrator). One line per name: "<name> meets <other>".
func (p *Panel) transferNameOverlaps(ctx context.Context, q rowQueryer, subID, receiverID int) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT r.hostname FROM hostname_reservations r JOIN domains d ON d.id = r.domain_id
		WHERE d.subscription_id = ? AND r.source_kind IN ('primary', 'alias') ORDER BY r.hostname`, subID)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, strings.ToLower(name))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(names) == 0 {
		return nil, nil
	}
	server, err := p.serverOwnHostnames(ctx)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, name := range names {
		held, err := reservationsOverlapping(ctx, q, name)
		if err != nil {
			return nil, err
		}
		other := ""
		for _, r := range held {
			if r.subscription != subID && r.ownerID != receiverID {
				other = r.hostname
				break
			}
		}
		if other == "" {
			for _, s := range server {
				if hostnamesOverlap(name, s.Name) {
					other = s.Name
					break
				}
			}
		}
		if other != "" {
			lines = append(lines, name+" meets "+other)
		}
	}
	return lines, nil
}

// transferNameRefusal: nil when every name may go to the receiver.
func (p *Panel) transferNameRefusal(ctx context.Context, q rowQueryer, c *Caller, subID, receiverID int) (*transferRefusal, error) {
	lines, err := p.transferNameOverlaps(ctx, q, subID, receiverID)
	if err != nil || len(lines) == 0 {
		return nil, err
	}
	r := refuseTransfer(409, errCodeTransferNameOverlap,
		"Nothing was changed: a name of this subscription equals, lies above or lies under a name that another account holds or that this server keeps for itself, so the receiving account may not hold it.")
	if c.hasAccountRole(roleAdmin) {
		r.Details = lines
		r.WaysOut = []string{"move or remove the names listed first", "keep the subscription where it is"}
	} else {
		r.WaysOut = []string{"ask the administrator to look at the subscription's names", "keep the subscription where it is"}
	}
	return r, nil
}

// --- Problems now: what an existing installation already holds -------------

// namespaceProblems lists, read only, the names ordinary accounts hold that
// the rule refuses today and the Node.js sites whose port belongs to a
// service of this server. Made before this rule (or by hand), each stays as
// it is until the administrator decides: nothing is changed or removed by
// an update, and the list empties by itself once a problem is solved.
func (p *Panel) namespaceProblems(ctx context.Context) ([]monitoringProblem, *monitoringUnknown) {
	findings, err := p.existingNameOverlaps(ctx)
	if err != nil {
		return nil, &monitoringUnknown{Source: "names", Reason: "unreadable"}
	}
	var problems []monitoringProblem
	for _, f := range findings {
		kind := "name_overlap"
		if f.Server {
			kind = "name_server_reserved"
		}
		problems = append(problems, monitoringProblem{
			Key: kind + ":" + strconv.Itoa(f.DomainID), Kind: kind, Severity: "critical", Actor: "administrator",
			Subject: f.DomainName, Name: f.Other,
			Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(f.DomainName)},
		})
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT d.id, d.name, s.app_port FROM sites s JOIN domains d ON d.id = s.domain_id
		WHERE s.project_type = 'node' AND s.app_port IS NOT NULL ORDER BY d.id`)
	if err != nil {
		return problems, &monitoringUnknown{Source: "names", Reason: "unreadable"}
	}
	defer rows.Close()
	panel := panelPort()
	for rows.Next() {
		var id, port int
		var name string
		if err := rows.Scan(&id, &name, &port); err != nil {
			return problems, &monitoringUnknown{Source: "names", Reason: "unreadable"}
		}
		if sitesandbox.NodePortRefusal(port, panel) == "" {
			continue
		}
		problems = append(problems, monitoringProblem{
			Key: "node_port_reserved:" + strconv.Itoa(id), Kind: "node_port_reserved", Severity: "critical", Actor: "administrator",
			Subject: name, Value: float64(port),
			Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(name)},
		})
	}
	if rows.Err() != nil {
		return problems, &monitoringUnknown{Source: "names", Reason: "unreadable"}
	}
	return problems, nil
}

// logNamespaceProblemsAtStart is the read-only check at start: it writes
// what it finds to the panel's log; the administrator sees the same list in
// Problems now (Dashboard and Monitoring).
func (p *Panel) logNamespaceProblemsAtStart(ctx context.Context) {
	problems, unknown := p.namespaceProblems(ctx)
	if unknown != nil {
		log.Printf("names and ports check at start: could not be read")
		return
	}
	for _, problem := range problems {
		log.Printf("names and ports check at start: %s %s (%s %v); see Problems now", problem.Kind, problem.Subject, problem.Name, problem.Value)
	}
}
