package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 round 5 review (W3): the stuck threshold counts consecutive
// pending re-checks whatever their reason, and the one notification is shown
// and mailed again only when a person must act now; a stuck cause that
// changes only brings its words up to date in place.

type alternatingNotice struct {
	reason, mail string
	seen, open   bool
	created      string
}

func readAlternatingNotice(t *testing.T, p *Panel) (rows int, notice alternatingNotice) {
	t.Helper()
	db := p.db.GetDB()
	if err := db.QueryRow(`SELECT COUNT(*) FROM account_notices WHERE kind = ?`, noticeDNSPublicationStuck).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows == 0 {
		return 0, notice
	}
	if err := db.QueryRow(`SELECT COALESCE(json_extract(body, '$.reason'), ''), COALESCE(mail_state, ''), seen_at IS NOT NULL, created_at
		FROM account_notices WHERE kind = ?`, noticeDNSPublicationStuck).Scan(&notice.reason, &notice.mail, &notice.seen, &notice.created); err != nil {
		t.Fatal(err)
	}
	notice.open = !notice.seen
	return rows, notice
}

func alternatingPanel(t *testing.T, zone string, code func() string) (*Panel, func()) {
	t.Helper()
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, zone)
	ctx := context.Background()
	if _, err := panel.db.GetDB().Exec(`INSERT INTO users (username, password_hash, email, role) VALUES
		('alt-admin', 'x', 'admin@example.test', 'admin')`); err != nil {
		t.Fatal(err)
	}
	configurePendingWithCode(t, agent, code())
	recoverOutcome(t, agent, func() bool { return true }, code)
	if err := panel.syncZoneToDNS(ctx, zone, false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	lease, err := readDNSZoneEngineLease(ctx, panel.db.GetDB(), zone)
	if err != nil {
		t.Fatal(err)
	}
	panel.reconcilePendingDNSPublicationsOnce(ctx) // discovery
	return panel, func() {
		entry, _ := scheduler.entry(lease.RequestID)
		clock.Advance(entry.NextAt.Sub(clock.Now()))
		panel.reconcilePendingDNSPublicationsOnce(ctx)
	}
}

func TestStuckCountsReChecksWhateverTheReason(t *testing.T) {
	noAnswer := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutPeerExchange)
	catchingUp := transport.DNSPeerPendingCatchingUp
	attempt := 0
	panel, recheck := alternatingPanel(t, "alternating.example", func() string {
		if attempt%2 == 0 {
			return noAnswer
		}
		return catchingUp
	})
	for i := 1; i <= dnsPendingPublicationStuckAfter; i++ {
		attempt = i
		recheck()
		rows, _ := readAlternatingNotice(t, panel)
		if want := 0; i < dnsPendingPublicationStuckAfter && rows != want {
			t.Fatalf("check %d: %d notice(s) before the threshold", i, rows)
		}
	}
	if rows, notice := readAlternatingNotice(t, panel); rows != 1 || !notice.open {
		t.Fatalf("a reason alternating every check was never reported: rows=%d %+v", rows, notice)
	}
}

func TestAlternatingStuckCauseIsToldOnceAndKeptCurrent(t *testing.T) {
	noAnswer := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutPeerExchange)
	catchingUp := transport.DNSPeerPendingCatchingUp
	code := noAnswer
	panel, recheck := alternatingPanel(t, "abab.example", func() string { return code })
	for i := 0; i < dnsPendingPublicationStuckAfter; i++ {
		recheck()
	}
	rows, first := readAlternatingNotice(t, panel)
	if rows != 1 || first.reason != transport.DNSPeerPendingProofTimeout || !first.open {
		t.Fatalf("first: rows=%d %+v", rows, first)
	}
	// It was mailed and the administrator read it.
	if _, err := panel.db.GetDB().Exec(`UPDATE account_notices SET mail_state = 'sent', seen_at = datetime('now') WHERE kind = ?`,
		noticeDNSPublicationStuck); err != nil {
		t.Fatal(err)
	}
	for _, next := range []string{catchingUp, noAnswer} {
		code = next
		for i := 0; i < dnsPendingPublicationStuckAfter; i++ {
			recheck()
		}
		rows, notice := readAlternatingNotice(t, panel)
		reason, _, _ := transport.SplitDNSPeerPendingCode(next)
		if rows != 1 || notice.reason != reason {
			t.Fatalf("%s: rows=%d %+v (the words are not current)", next, rows, notice)
		}
		if notice.open || notice.mail != "sent" || notice.created != first.created {
			t.Fatalf("%s: shown or mailed again: %+v", next, notice)
		}
	}
}
