package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Beta-1 re-run P7: the backup's clock was about 1.5 s behind the ACME
// server, so the leaf certbot had just issued was "not yet valid" and the
// step failed. Issuance now waits until the leaf is valid by this clock,
// within a bound; a larger skew is left to fail with its clock reason.
func TestPanelCertificateIssueWaitsForALeafValidMomentsAfterThisClock(t *testing.T) {
	previousRead, previousNow, previousSleep := panelCertificateActivationReadSource, panelCertificateWaitNow, panelCertificateWaitSleep
	t.Cleanup(func() {
		panelCertificateActivationReadSource, panelCertificateWaitNow, panelCertificateWaitSleep = previousRead, previousNow, previousSleep
	})
	now := time.Date(2026, 10, 29, 3, 14, 7, 577_000_000, time.UTC)
	panelCertificateWaitNow = func() time.Time { return now }
	for _, test := range []struct {
		name string
		err  error
		want time.Duration
	}{
		{name: "issued 1.5 s ahead", err: &panelCertificateSourceNotYetValidError{NotBefore: now.Add(1500 * time.Millisecond), Now: now}, want: 2500 * time.Millisecond},
		{name: "beyond the bound", err: &panelCertificateSourceNotYetValidError{NotBefore: now.Add(time.Hour), Now: now}},
		{name: "already valid", err: nil},
		{name: "another failure", err: errors.New("verify panel certificate source trust chain")},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads, slept := 0, time.Duration(0)
			panelCertificateActivationReadSource = func(domain string) ([]byte, []byte, []byte, time.Time, error) {
				reads++
				if domain != "panel.example.test" {
					t.Fatalf("read %q", domain)
				}
				return nil, nil, nil, time.Time{}, test.err
			}
			panelCertificateWaitSleep = func(_ context.Context, d time.Duration) error {
				slept += d
				return nil
			}
			waitPanelCertificateSourceValid(context.Background(), "panel.example.test")
			if reads != 1 || slept != test.want {
				t.Fatalf("reads=%d slept=%v, want 1 and %v", reads, slept, test.want)
			}
		})
	}
}
