package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"
)

// How a joined server reads the main DNS server's answer to its publishing
// connection (alpha.114 lab E1). The lab's joined server failed with
// "create a new join code" while its join code was valid: the main DNS
// server was in its own setup (its panel restarted, its database busy with
// the mail install) and its refusal said nothing more. Now:
//
//   - a refusal the main DNS server verified and typed (the code is unknown
//     there, expired, or used by another connection) is a failure whose
//     next action is a new join code;
//   - a main that is busy, restarting, still in its own setup, not ready, or
//     whose license is not active, and every 5xx, is a wait with its reason;
//   - a refusal without a reason (an alpha.113 main answers every refusal
//     alike, 401/409 without a typed code) is a wait for at most
//     serverSetupPublisherUnconfirmedLimit; past it the step fails and says
//     what the owner checks on the main DNS server.
//
// Polling reads only: the same connection request is asked again, nothing
// is created twice (the connection row is reused by endpoint and code).
// Ana DNS sunucusunun dogruladigi ret basarisizliktir; mesgul, yeniden
// baslayan ya da kendi kurulumundaki sunucu beklemedir; nedensiz ret en
// fazla 30 dakika beklenir, sonra ne kontrol edilecegi soylenir.

const serverSetupPublisherUnconfirmedLimit = 30 * time.Minute

var serverSetupPublisherNow = time.Now

func serverSetupPublisherRefusedSinceKey(executionID string) string {
	return "server_setup_dns_publisher_refused_since:" + executionID
}

// serverSetupPublisherHost is the main DNS server's panel name, as the
// reviewed endpoint names it; the endpoint itself when it cannot be read.
func serverSetupPublisherHost(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return endpoint
}

func (p *Panel) clearServerSetupPublisherRefusedSince(ctx context.Context, executionID string) {
	if executionID == "" {
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupPublisherRefusedSinceKey(executionID)); err != nil {
		log.Printf("server setup: clear the publishing refusal time: %v", err)
	}
}

// serverSetupPublisherRefusedFor records when the unexplained refusals of
// this run began (once) and says how long they have lasted.
func (p *Panel) serverSetupPublisherRefusedFor(ctx context.Context, executionID string) (time.Duration, error) {
	now := serverSetupPublisherNow().UTC()
	key := serverSetupPublisherRefusedSinceKey(executionID)
	if _, err := p.db.GetDB().ExecContext(ctx, `INSERT OR IGNORE INTO panel_settings(key,value) VALUES(?,?)`, key, now.Format(time.RFC3339)); err != nil {
		return 0, err
	}
	var raw string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, key).Scan(&raw); err != nil {
		return 0, err
	}
	since, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0, err
	}
	return now.Sub(since), nil
}

func (p *Panel) serverSetupPublisherRefusal(ctx context.Context, executionID, endpoint string, err error, waiting *serverSetupPeerWait) error {
	host := serverSetupPublisherHost(endpoint)
	var answer *remoteDNSHTTPError
	if !errors.As(err, &answer) {
		// No answer at all (unreachable, restarting, not ready yet, an
		// unreadable reply): the ordinary wait.
		return waiting
	}
	switch answer.Code {
	case errCodeRemoteDNSEnrollmentUnknown, errCodeRemoteDNSEnrollmentExpired, errCodeRemoteDNSEnrollmentUsed:
		reason := map[string]string{
			errCodeRemoteDNSEnrollmentUnknown: "it no longer knows this server's enrollment code (a newer join code replaced it, or it was withdrawn)",
			errCodeRemoteDNSEnrollmentExpired: "this server's enrollment code expired",
			errCodeRemoteDNSEnrollmentUsed:    "this server's enrollment code was already used by another connection",
		}[answer.Code]
		return &serverSetupChildFailure{Code: "server_setup_dns_publisher_refused", Message: "The main DNS server " + host + " refused this server's publishing connection: " + reason + ". You, the owner: on " + host + " create a new join code, join this server again with it, then review this plan again."}
	case "REMOTE_DNS_AUTHORITY_NOT_READY":
		return waiting
	case "license_required", errCodeLicenseStatusUnavailable, errCodeLicenseVerificationUnavailable:
		return &serverSetupPeerWait{Code: "server_setup_dns_publisher_peer_license", Message: "The main DNS server " + host + " answered that its panel license is not active or could not be checked, so it takes no connection now. Its owner activates or renews the license on " + host + "; this server asks again by itself."}
	}
	if answer.StatusCode >= 500 || answer.StatusCode == http.StatusTooManyRequests || answer.StatusCode == http.StatusRequestTimeout || answer.StatusCode < 400 {
		return &serverSetupPeerWait{Code: "server_setup_dns_publisher_peer_busy", Message: "The main DNS server " + host + " is busy, restarting or finishing its own setup, so it could not take this server's publishing connection yet. Nobody needs to act; this server asks again by itself."}
	}
	// A refusal without a reason. An alpha.113 main answers this way while
	// its database is busy in its own setup as well as for an expired code,
	// so it is not taken as proof; it is waited on, bounded.
	lasted, readErr := p.serverSetupPublisherRefusedFor(ctx, executionID)
	if readErr == nil && lasted >= serverSetupPublisherUnconfirmedLimit {
		return &serverSetupChildFailure{Code: "server_setup_dns_publisher_unconfirmed", Message: "For 30 minutes the main DNS server " + host + " refused this server's publishing connection without saying why. You, the owner: open " + host + "'s panel and check that its own setup has finished. If it has, create a new join code there, join this server again with it, then review this plan again."}
	}
	return &serverSetupPeerWait{Code: "server_setup_dns_publisher_peer_unexplained", Message: "The main DNS server " + host + " did not accept this server's publishing connection and did not say why; this happens while it restarts or finishes its own setup. Nobody needs to act yet: this server asks again by itself, and after 30 minutes it says what to check."}
}
