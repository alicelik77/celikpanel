package licensing_test

// TestLicensePHPInterop's Go twin: receipts made by the Go member service
// (internal/membership) are accepted by the panel's own Verify, for activation and
// refresh, and a removed licence is refused. The service runs in process with a
// throwaway key; nothing reaches the live licence service.

import (
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/licensing"
	"github.com/alicelik/celikpanel/internal/membership"
)

func TestLicenseGoServiceInterop(t *testing.T) {
	public, secret, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	var token string
	s, err := membership.Open(filepath.Join(t.TempDir(), "members.sqlite"), secret, func(_, _, tk string) error { token = tk; return nil },
		func() int64 { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Register("interop@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeToken(token, "verify", "interop-password-1234"); err != nil {
		t.Fatal(err)
	}
	member, err := s.Login("interop@example.com", "interop-password-1234")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.Issue(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	machine := strings.Repeat("1", 32)
	server, err := licensing.ServerID([]byte(machine))
	if err != nil {
		t.Fatal(err)
	}
	activated, err := s.Activate(issued.Key, server, "interop.example.com", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	envelope := licensing.Envelope{Payload: activated.Payload, Signature: activated.Signature, ActivationToken: activated.ActivationToken}
	claims, err := licensing.Verify(envelope, public, server, time.Unix(now, 0))
	if err != nil || claims.LicenseID != issued.ID || claims.ServerID != server || claims.Product != "celikpanel" ||
		claims.OfflineUntil != now+7*86400 || claims.RefreshAfter != now+3600 {
		t.Fatalf("activation receipt: %+v %v", claims, err)
	}
	now += 3600
	refreshed, err := s.Refresh(issued.ID, server, activated.ActivationToken, "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	envelope = licensing.Envelope{Payload: refreshed.Payload, Signature: refreshed.Signature, ActivationToken: refreshed.ActivationToken}
	if claims, err = licensing.Verify(envelope, public, server, time.Unix(now, 0)); err != nil || claims.IssuedAt != now {
		t.Fatalf("refresh receipt: %+v %v", claims, err)
	}
	// Another key, another server: refused by the panel.
	otherPublic, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := licensing.Verify(envelope, otherPublic, server, time.Unix(now, 0)); err == nil {
		t.Fatal("a receipt verified with another key")
	}
	if _, err := licensing.Verify(envelope, public, strings.Repeat("f", 64), time.Unix(now, 0)); err == nil {
		t.Fatal("a receipt verified for another server")
	}
	if err := s.Remove(member.ID, issued.ID, "interop-password-1234"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(issued.ID, server, refreshed.ActivationToken, "8.8.8.8"); err != membership.ErrInvalidActivation {
		t.Fatalf("a removed licence refreshed: %v", err)
	}
}
