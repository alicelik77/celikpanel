package main

import (
	"context"
	"errors"
	"time"
)

// An issued panel certificate whose validity starts a moment after this
// server's clock (Beta-1 re-run P7, 2026-10-29).
//
// In the run the ACME server stamped notBefore with its own issue time and
// the backup's clock was about 1.1-1.5 s behind it. The agent read the leaf
// right after certbot returned, found notBefore still ahead and failed the
// step with panel_certificate_issue_failed although the certificate was
// issued and correct. (Let's Encrypt backdates notBefore by about an hour,
// so production meets this only with a far larger skew.) The read is not
// relaxed: issuance waits, bounded, until the leaf is valid by this clock,
// then reads it as before. A larger skew still fails, and its reason now
// names the clock.
//
// Sunucu saatinin birkaç saniye gerisinde kalan yeni sertifika: sınırlı
// süre beklenir, sonra her zamanki gibi okunur.

// panelCertificateSourceSkewWait bounds that wait.
const panelCertificateSourceSkewWait = 2 * time.Minute

type panelCertificateSourceNotYetValidError struct {
	NotBefore time.Time
	Now       time.Time
}

func (e *panelCertificateSourceNotYetValidError) Error() string {
	return "panel certificate source leaf is not valid yet by this server's clock (valid from " +
		e.NotBefore.UTC().Format(time.RFC3339) + ", this server's time " + e.Now.UTC().Format(time.RFC3339) +
		"); check the server's clock (timedatectl) and try again"
}

var (
	panelCertificateWaitNow   = time.Now
	panelCertificateWaitSleep = func(ctx context.Context, d time.Duration) error {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
)

// waitPanelCertificateSourceValid reads the issued source once and, when its
// leaf becomes valid within the bound, waits until it is. It never changes
// anything; the caller's own read decides as before.
func waitPanelCertificateSourceValid(ctx context.Context, domain string) {
	waitIssuedCertificateSourceValid(ctx, func() error {
		_, _, _, _, err := panelCertificateActivationReadSource(domain)
		return err
	})
}

// waitIssuedCertificateSourceValid is that wait for every certificate the
// setup issues (panel and mail host; Beta-1 third re-run N4): read once, and
// when the leaf becomes valid by this clock within the bound, wait for it.
func waitIssuedCertificateSourceValid(ctx context.Context, read func() error) {
	var early *panelCertificateSourceNotYetValidError
	if !errors.As(read(), &early) {
		return
	}
	wait := early.NotBefore.Sub(panelCertificateWaitNow()) + time.Second
	if wait <= 0 || wait > panelCertificateSourceSkewWait {
		return
	}
	_ = panelCertificateWaitSleep(ctx, wait)
}

// issuedCertificateClockSkew: the publication failed because the leaf is
// still not valid by this clock; seconds is how far the clock is behind
// (rounded up, bounded). Shared by every certificate the setup issues (N2).
func issuedCertificateClockSkew(err error) (int, bool) {
	var early *panelCertificateSourceNotYetValidError
	if !errors.As(err, &early) {
		return 0, false
	}
	seconds := int(early.NotBefore.Sub(early.Now) / time.Second)
	if early.NotBefore.Sub(early.Now)%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 10_000_000 {
		seconds = 10_000_000
	}
	return seconds, true
}
