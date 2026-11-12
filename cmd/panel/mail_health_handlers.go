package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The deliverability health screen: every signal that decides "inbox or
// spam", one endpoint. Server facts (PTR/FCrDNS, HELO, TLS, port 25) come
// from the agent; domain facts (SPF/DKIM/DMARC via live DNS, blacklists,
// mail-certificate, DNSSEC) are assembled here. The UI paints it as one
// traffic-light card instead of five scattered tabs.
//
// Teslim edilebilirlik sağlık ekranı: "gelen kutusu mu spam mı"ya karar
// veren her sinyal, tek uçta. Sunucu gerçekleri (PTR/FCrDNS, HELO, TLS,
// port 25) agent'tan gelir; domain gerçekleri (canlı DNS'ten SPF/DKIM/DMARC,
// kara listeler, posta sertifikası, DNSSEC) burada birleştirilir. Arayüz
// bunu beş dağınık sekme yerine tek trafik-ışığı kartı olarak çizer.

type healthCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"` // ok | warn | fail | unknown
	Detail string `json:"detail,omitempty"`
	// Items: the names the detail is about (alpha.133: the blocklists of
	// the "rbl" check).
	Items []string `json:"items,omitempty"`
}

// mailDNSSECSecured reports PowerDNS signing state only when PowerDNS is the
// exact durable active publisher. A retained standby database must never make
// an active BIND zone look DNSSEC-secured.
func (p *Panel) mailDNSSECSecured(ctx context.Context, domain string) bool {
	mode, err := p.domainDNSManagementMode(ctx, domain)
	if err != nil || mode != setupDNSModeLocal {
		return false
	}
	// A read for the health card: the pair proof's last verified answer
	// (alpha.127, display_checks.go).
	publisher, ready, err := p.activeDNSPublisherForDisplay(ctx)
	if err != nil || !ready || publisher.Engine != transport.DNSEnginePowerDNS ||
		publisher.Epoch < 1 {
		return false
	}
	var status transport.DNSSECStatusResponse
	if err := p.callAgentContext(
		ctx, "Agent.DNSSECStatus",
		&transport.DNSSECRequest{Zone: domain}, &status,
	); err != nil || status.Error != "" {
		return false
	}
	return status.Secured
}

// mailHealthReport is one deliverability check, as the card shows it.
type mailHealthReport struct {
	ServerIP   string
	Myhostname string
	Overall    string
	Checks     []healthCheck
}

// mailHealthAgentError: the Agent's server facts could not be read; the card
// says it could not check (writeAgentError), as before.
type mailHealthAgentError struct{ err error }

func (e *mailHealthAgentError) Error() string { return e.err.Error() }
func (e *mailHealthAgentError) Unwrap() error { return e.err }

// handleMailHealth answers the card at once with the last verified check
// while one check runs again (alpha.127, display_checks.go); the card then
// reads again. The questions of one check run at the same time.
// Kart son dogrulanmis yaniti hemen alir; tek bir kontrol arkada yeniden
// kosar. Bir kontroldeki sorular ayni anda sorulur.
func (p *Panel) handleMailHealth(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()
	domain := p.domainNameByID(ctx, domainID)
	if domain == "" {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return
	}

	mode, err := p.domainDNSManagementMode(ctx, domain)
	if err != nil {
		writeServerError(w, err)
		return
	}
	report, view, err := p.displayChecks.ready().mailHealth.get(ctx, fmt.Sprintf("%d|%s|%s", domainID, domain, mode),
		func(ctx context.Context) (mailHealthReport, error) {
			return p.mailHealthReport(ctx, domainID, domain, mode)
		})
	var agentErr *mailHealthAgentError
	if errors.As(err, &agentErr) {
		writeAgentError(w, agentErr.err, "mail health")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"domain": domain, "server_ip": report.ServerIP, "myhostname": report.Myhostname,
		"expected_ptr": strings.ToLower(report.Myhostname),
		"overall":      report.Overall, "checks": report.Checks,
		"checked_at": view.CheckedAt.UTC().Format(time.RFC3339),
		"rechecking": view.Rechecking,
	})
}

func (p *Panel) mailHealthReport(ctx context.Context, domainID int, domain, mode string) (mailHealthReport, error) {
	var (
		wg        sync.WaitGroup
		external  healthCheck
		mh        transport.MailHealthResponse
		agentErr  error
		rbl       transport.CheckRBLResponse
		rblErr    error
		dnssecOK  bool
		liveNames = [3][2]string{
			{domain, "v=spf1"},
			{dkimSelector + "._domainkey." + domain, "v=DKIM1"},
			{"_dmarc." + domain, "v=DMARC1"},
		}
		live [3]string
	)
	run := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	if mode == setupDNSModeExternal || mode == setupDNSModeExisting {
		run(func() {
			external = externalMailDNSHealth(ctx, domain, serverPrimaryIP(), serverPrimaryIPv6(), net.DefaultResolver)
		})
	}
	// --- Server facts from the agent / Sunucu gerçekleri agent'tan
	run(func() { agentErr = p.callAgentContext(ctx, "Agent.MailHealth", &transport.Empty{}, &mh) })
	for i := range liveNames {
		run(func() {
			if v, ok := liveTXT(ctx, liveNames[i][0], liveNames[i][1]); ok {
				live[i] = v
			}
		})
	}
	// --- Blacklists / Kara listeler
	run(func() { rblErr = p.callAgentContext(ctx, "Agent.CheckRBL", &transport.Empty{}, &rbl) })
	// --- DNSSEC
	run(func() { dnssecOK = p.mailDNSSECSecured(ctx, domain) })

	// --- SPF / DKIM / DMARC: what the world resolves; if the record sits
	// in our zone but the world cannot see it yet (delegation pending), that
	// is a warn, not a fail.
	// SPF / DKIM / DMARC: dünyanın çözdüğü; kayıt bizim zone'daysa ama dünya
	// henüz göremiyorsa (delegasyon bekliyor) bu fail değil warn'dur.
	var zoneID int
	var desiredRemote []DNSRecord
	var desiredErr error
	if mode == setupDNSModeLocal {
		_ = p.db.GetDB().QueryRowContext(ctx, `SELECT id FROM pdns_domains WHERE name = ?`, domain).Scan(&zoneID)
	} else if mode == setupDNSModeExisting {
		desiredRemote, desiredErr = p.remoteDomainDNSRecords(ctx, domain)
	}
	wg.Wait()
	if agentErr != nil {
		return mailHealthReport{}, &mailHealthAgentError{err: agentErr}
	}
	if desiredErr != nil {
		return mailHealthReport{}, desiredErr
	}

	var checks []healthCheck
	add := func(id, status, detail string) {
		checks = append(checks, healthCheck{ID: id, Status: status, Detail: detail})
	}
	if mode == setupDNSModeExternal || mode == setupDNSModeExisting {
		checks = append(checks, external)
	}
	switch {
	case mh.PTR == "":
		add("ptr", "fail", mh.ServerIP)
	case mh.FCrDNS:
		add("ptr", "ok", mh.PTR)
	default:
		add("ptr", "warn", mh.PTR)
	}
	if mh.HostnameFQDN {
		add("helo", "ok", mh.Myhostname)
	} else {
		add("helo", "warn", mh.Myhostname)
	}
	if mh.TLSEnabled {
		add("tls", "ok", "")
	} else {
		add("tls", "fail", "")
	}
	add("port25", map[string]string{"open": "ok", "blocked": "fail"}[mh.OutboundPort25], "")
	if mh.OutboundPort25 == "unknown" {
		checks[len(checks)-1].Status = "unknown"
	}
	for i, id := range []string{"spf", "dkim", "dmarc"} {
		name, prefix := liveNames[i][0], liveNames[i][1]
		switch {
		case live[i] != "":
			add(id, "ok", "")
		case remoteMailDesiredTXT(desiredRemote, name, prefix) != "":
			add(id, "warn", "pending")
		case zoneID > 0 && strings.HasPrefix(p.zoneTXT(ctx, zoneID, name), prefix):
			add(id, "warn", "pending")
		default:
			add(id, "fail", "")
		}
	}
	checks = append(checks, mailHealthRBLCheck(rbl, rblErr))

	// --- Mail certificate / Posta sertifikası
	var secureMail int
	_ = p.db.GetDB().QueryRowContext(ctx, `
		SELECT COALESCE(MAX(secure_mail), 0) FROM ssl_certificates
		WHERE domain_id = ? AND status = 'active'`, domainID).Scan(&secureMail)
	if secureMail == 1 {
		add("mailcert", "ok", "")
	} else {
		add("mailcert", "warn", "")
	}
	if dnssecOK {
		add("dnssec", "ok", "")
	} else {
		add("dnssec", "warn", "")
	}

	return mailHealthReport{ServerIP: mh.ServerIP, Myhostname: mh.Myhostname, Overall: mailHealthOverall(checks), Checks: checks}, nil
}

// mailHealthOverall: the worst status wins, except that blocklists which
// could not be asked never colour the card (alpha.133): that says nothing
// about the address.
// Genel: en kötü durum kazanır; sorulamayan kara listeler kartı boyamaz.
func mailHealthOverall(checks []healthCheck) string {
	overall := "ok"
	for _, c := range checks {
		if c.Status == "fail" {
			return "fail"
		}
		if c.Status == "warn" || (c.Status == "unknown" && c.ID != "rbl") {
			overall = "warn"
		}
	}
	return overall
}

// --- Blocklists (alpha.133) / Kara listeler
//
// Each list's answer is listed, not listed, or could not be checked with a
// reason (transport.RBLResult). An answer from an Agent older than alpha.133
// carries no outcome: its "listed" may be a refused question (Spamhaus's
// 127.255.255.254 to a public resolver) and its "not listed" a failed lookup,
// so it is said as could not be checked (RBLReasonOlderAgent), never as a
// listing. Both screens (the deliverability card and "Check now") and the
// domain health strip read the same view.
//
// Her listenin yanıtı listede, listede değil ya da nedeniyle denetlenemedi
// olur. alpha.133'ten eski Agent'ın yanıtı denetlenemedi sayılır, asla
// listeleme değil.

// rblListView is one list's answer as the screens show it.
type rblListView struct {
	Zone    string `json:"zone"`
	Name    string `json:"name"`
	Listed  bool   `json:"listed"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
	// Sublists and Policy: the documented parts of a listing; Policy when
	// every part is a policy entry (Spamhaus PBL), not a spam report.
	Sublists []string `json:"sublists,omitempty"`
	Policy   bool     `json:"policy,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

// rblSummary is the whole answer in one: Status ok | fail | unknown; Code
// clean, listed, policy or the reason a list could not be checked; Items the
// names of the lists the code is about.
type rblSummary struct {
	Status string   `json:"status"`
	Code   string   `json:"code"`
	Items  []string `json:"items,omitempty"`
}

type rblReportView struct {
	IP      string        `json:"ip"`
	Results []rblListView `json:"results"`
	Summary rblSummary    `json:"summary"`
}

var rblListNames = map[string]string{
	"zen.spamhaus.org":       "Spamhaus",
	"bl.spamcop.net":         "SpamCop",
	"b.barracudacentral.org": "Barracuda",
}

// rblReasonRank orders the reasons by how much they tell the administrator.
var rblReasonRank = map[string]int{
	transport.RBLReasonPublicResolver: 5,
	transport.RBLReasonTooManyQueries: 4,
	transport.RBLReasonOlderAgent:     3,
	transport.RBLReasonUnknownAnswer:  2,
	transport.RBLReasonLookupFailed:   1,
}

func rblView(resp transport.CheckRBLResponse) rblReportView {
	view := rblReportView{IP: resp.IP, Results: make([]rblListView, 0, len(resp.Results))}
	for _, res := range resp.Results {
		name := rblListNames[res.Zone]
		if name == "" {
			name = res.Zone
		}
		list := rblListView{Zone: res.Zone, Name: name, Outcome: res.Outcome, Reason: res.Reason}
		switch res.Outcome {
		case transport.RBLListed:
			list.Listed, list.Detail = true, res.Detail
			list.Sublists = append([]string(nil), res.Sublists...)
			list.Policy = len(res.Sublists) > 0
			for _, part := range res.Sublists {
				if part != "pbl" {
					list.Policy = false
				}
			}
		case transport.RBLNotListed:
			list.Reason = ""
		case "":
			// An Agent older than alpha.133. / alpha.133'ten eski Agent.
			list.Outcome, list.Reason = transport.RBLNotChecked, transport.RBLReasonOlderAgent
		default:
			list.Outcome = transport.RBLNotChecked
			if rblReasonRank[list.Reason] == 0 {
				list.Reason = transport.RBLReasonUnknownAnswer
			}
		}
		view.Results = append(view.Results, list)
	}
	view.Summary = summarizeRBL(view.Results)
	return view
}

// rblItemName names a list in a sentence, with the parts of its listing.
func rblItemName(list rblListView) string {
	if len(list.Sublists) == 0 {
		return list.Name
	}
	return list.Name + " (" + strings.ToUpper(strings.Join(list.Sublists, ", ")) + ")"
}

func summarizeRBL(lists []rblListView) rblSummary {
	var spam, policy, unchecked []string
	reason := ""
	for _, list := range lists {
		switch {
		case list.Outcome == transport.RBLListed && list.Policy:
			policy = append(policy, rblItemName(list))
		case list.Outcome == transport.RBLListed:
			spam = append(spam, rblItemName(list))
		case list.Outcome == transport.RBLNotChecked:
			unchecked = append(unchecked, list.Name)
			if rblReasonRank[list.Reason] > rblReasonRank[reason] {
				reason = list.Reason
			}
		}
	}
	switch {
	case len(spam) > 0:
		return rblSummary{Status: "fail", Code: transport.RBLListed, Items: append(spam, policy...)}
	case len(policy) > 0:
		return rblSummary{Status: "fail", Code: "policy", Items: policy}
	case len(unchecked) > 0:
		return rblSummary{Status: "unknown", Code: reason, Items: unchecked}
	case len(lists) == 0:
		return rblSummary{Status: "unknown"}
	}
	return rblSummary{Status: "ok", Code: "clean"}
}

// mailHealthRBLCheck is the card's blocklist line. A list that could not be
// asked is "unknown" with its reason, never "fail".
func mailHealthRBLCheck(resp transport.CheckRBLResponse, err error) healthCheck {
	if err != nil || resp.Error != "" {
		return healthCheck{ID: "rbl", Status: "unknown"}
	}
	summary := rblView(resp).Summary
	check := healthCheck{ID: "rbl", Status: summary.Status, Detail: summary.Code, Items: summary.Items}
	if summary.Status == "ok" {
		check.Detail = ""
	}
	return check
}
