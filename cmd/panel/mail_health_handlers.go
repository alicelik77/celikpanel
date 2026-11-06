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
	if rblErr == nil && rbl.Error == "" && len(rbl.Results) > 0 {
		listed := 0
		for _, res := range rbl.Results {
			if res.Listed {
				listed++
			}
		}
		if listed == 0 {
			add("rbl", "ok", "")
		} else {
			add("rbl", "fail", "")
		}
	} else {
		add("rbl", "unknown", "")
	}

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

	// Overall: worst status wins. / Genel: en kötü durum kazanır.
	overall := "ok"
	for _, c := range checks {
		if c.Status == "fail" {
			overall = "fail"
			break
		}
		if c.Status == "warn" || c.Status == "unknown" {
			overall = "warn"
		}
	}
	return mailHealthReport{ServerIP: mh.ServerIP, Myhostname: mh.Myhostname, Overall: overall, Checks: checks}, nil
}
