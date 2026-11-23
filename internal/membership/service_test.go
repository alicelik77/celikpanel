package membership

// Port of portal-membership/tests/service.php: the same steps in the same order on
// the same clock (2028-02-29T10:00:00Z), every check under its PHP label and every
// rejects() under "rejects <code>". The keys are throwaway keys made here; no test
// in this package reaches the live licence service.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/licensing"
)

type sentMail struct{ Email, Purpose, Token string }

type serviceHarness struct {
	t      *testing.T
	dir    string
	now    int64
	mu     sync.Mutex
	mail   []sentMail
	secret ed25519.PrivateKey
	public ed25519.PublicKey
	s      *Service
	labels []string
}

func newHarness(t *testing.T) *serviceHarness {
	t.Helper()
	public, secret, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	h := &serviceHarness{t: t, dir: t.TempDir(), now: time.Date(2028, 2, 29, 10, 0, 0, 0, time.UTC).Unix(), secret: secret, public: public}
	h.s = h.open()
	return h
}

func (h *serviceHarness) open() *Service {
	h.t.Helper()
	s, err := Open(filepath.Join(h.dir, "db.sqlite"), h.secret, func(email, purpose, token string) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.mail = append(h.mail, sentMail{email, purpose, token})
		return nil
	}, func() int64 { return h.now })
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { s.Close() })
	return s
}

func (h *serviceHarness) check(ok bool, label string) {
	h.t.Helper()
	h.labels = append(h.labels, label)
	h.t.Run(label, func(t *testing.T) {
		if !ok {
			t.Fatal(label)
		}
	})
	if !ok {
		h.t.FailNow()
	}
}

func (h *serviceHarness) rejects(code Problem, err error) {
	h.t.Helper()
	h.check(errors.Is(err, code), "rejects "+string(code))
}

func (h *serviceHarness) lastMail() sentMail {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.mail[len(h.mail)-1]
}

func (h *serviceHarness) claims(e Envelope) licensing.Claims {
	h.t.Helper()
	payload, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		h.t.Fatal(err)
	}
	signature, err := base64.StdEncoding.DecodeString(e.Signature)
	if err != nil || !ed25519.Verify(h.public, payload, signature) {
		h.t.Fatal("signature")
	}
	var c licensing.Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		h.t.Fatal(err)
	}
	return c
}

func (h *serviceHarness) exec(query string, args ...any) {
	h.t.Helper()
	if _, err := h.s.DB().Exec(query, args...); err != nil {
		h.t.Fatal(err)
	}
}

func (h *serviceHarness) expiresOf(member int64, id string) int64 {
	h.t.Helper()
	list, err := h.s.Licenses(member)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, l := range list {
		if l.ID == id {
			return l.Expires.Int64
		}
	}
	h.t.Fatal("licence not listed")
	return 0
}

// must stops the test on an unexpected error (a panic fails the test with its stack).
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestServicePHPChecks(t *testing.T) {
	h := newHarness(t)
	s := h.s
	password := "test-password-never-production"
	utc := func(text string) int64 { return must(time.Parse(time.RFC3339, text)).Unix() }

	h.rejects(ErrInvalidEmail, s.Register("x@example.com\r\nBcc: other@example.com"))
	if err := s.Register("Alice@example.com"); err != nil {
		t.Fatal(err)
	}
	h.check(len(h.mail) == 1, "verification sent")
	_, err := s.Login("alice@example.com", password)
	h.rejects(ErrInvalidLogin, err)
	firstToken := h.mail[0].Token
	if err := s.Register("alice@example.com"); err != nil {
		t.Fatal(err)
	}
	h.rejects(ErrInvalidToken, s.ConsumeToken(firstToken, "verify", password))
	if err := s.ConsumeToken(h.mail[1].Token, "verify", password); err != nil {
		t.Fatal(err)
	}
	h.rejects(ErrInvalidToken, s.ConsumeToken(h.mail[1].Token, "verify", password))
	member := must(s.Login("alice@example.com", password))
	_, err = s.Login("alice@example.com", "attacker-password-123")
	h.rejects(ErrInvalidLogin, err)
	license := must(s.Issue(member.ID))
	server, other := strings.Repeat("a", 64), strings.Repeat("b", 64)
	h.check(h.expiresOf(member.ID, license.ID) == utc("2029-02-28T10:00:00Z"), "term starts on issue")
	h.check(must(s.Reveal(member.ID, license.ID, password)).Key == license.Key, "same key can be revealed")
	_, err = s.Reveal(member.ID, license.ID, "wrong")
	h.rejects(ErrInvalidLogin, err)
	_, err = s.Reveal(member.ID, strings.Repeat("f", 32), password)
	h.rejects(ErrLicenseNotFound, err)
	h.exec("UPDATE licenses SET key_encrypted=NULL WHERE id=?", license.ID)
	_, err = s.Reveal(member.ID, license.ID, password)
	h.rejects(ErrKeyNotSaved, err)
	oldKey, oldExpiry := license.Key, h.expiresOf(member.ID, license.ID)
	license = must(s.Rotate(member.ID, license.ID, password))
	h.check(license.Key != oldKey, "explicit replacement creates a new key")
	h.check(h.expiresOf(member.ID, license.ID) == oldExpiry, "legacy replacement preserves expiry")
	h.check(must(s.Reveal(member.ID, license.ID, password)).Key == license.Key, "replacement is saved automatically")
	var encrypted string
	if err := s.DB().QueryRow("SELECT key_encrypted FROM licenses").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	h.check(!strings.Contains(encrypted, license.Key), "stored key is encrypted")
	h.exec("UPDATE licenses SET key_encrypted=? WHERE id=?", "broken", license.ID)
	_, err = s.Reveal(member.ID, license.ID, password)
	h.rejects(ErrKeyNotSaved, err)
	h.exec("UPDATE licenses SET key_encrypted=? WHERE id=?", encrypted, license.ID)
	h.now += 86400 * 10

	e := must(s.Activate(license.Key, server, "frankfurt.example.com", "8.8.8.8"))
	c := h.claims(e)
	h.check(c.RefreshAfter == h.now+3600 && c.OfflineUntil == h.now+7*86400, "hourly check, seven-day offline window")
	h.check(c.ExpiresAt == utc("2029-02-28T10:00:00Z"), "calendar year leap date")
	h.now += 3600
	retry := must(s.Activate(license.Key, server, "frankfurt.example.com", "8.8.8.8"))
	r := h.claims(retry)
	h.check(c.ActivatedAt == r.ActivatedAt && c.ExpiresAt == r.ExpiresAt, "retry preserves term")
	h.check(retry.ActivationToken == e.ActivationToken, "retry preserves credential")
	_, err = s.Activate(license.Key, other, "boston.example.com", "9.9.9.9")
	h.rejects(ErrLicenseInUse, err)
	must(s.Refresh(license.ID, server, e.ActivationToken, "8.8.8.8"))
	_, err = s.Refresh(license.ID, other, e.ActivationToken, "9.9.9.9")
	h.rejects(ErrInvalidActivation, err)
	h.rejects(ErrRenewalNotDue, s.Renew(member.ID, license.ID, password))
	h.rejects(ErrInvalidLogin, s.Release(member.ID, license.ID, "wrong"))
	if err := s.Release(member.ID, license.ID, password); err != nil {
		t.Fatal(err)
	}
	replacement := license
	h.check(must(s.Reveal(member.ID, license.ID, password)).Key == license.Key, "transfer preserves the key")
	_, err = s.Refresh(license.ID, server, e.ActivationToken, "8.8.8.8")
	h.rejects(ErrInvalidActivation, err)
	moved := must(s.Activate(replacement.Key, other, "boston.example.com", "9.9.9.9"))
	mc := h.claims(moved)
	h.check(mc.ExpiresAt == c.ExpiresAt, "transfer preserves expiry")
	h.check(mc.ServerID == other, "transfer binds other server")

	// The key survives reinstall at the same verified IP, but not a change of IP.
	ipLicense := must(s.Issue(member.ID))
	ipEnvelope := must(s.Activate(ipLicense.Key, server, "ip.example.com", "8.8.4.4"))
	ipClaims := h.claims(ipEnvelope)
	_, err = s.Activate(ipLicense.Key, server, "ip.example.com", "9.9.9.9")
	h.rejects(ErrLicenseInUse, err)
	_, err = s.Refresh(ipLicense.ID, server, ipEnvelope.ActivationToken, "9.9.9.9")
	h.rejects(ErrLicenseInUse, err)
	_, err = s.Activate(ipLicense.Key, server, "ip.example.com", "127.0.0.1")
	h.rejects(ErrInvalidServerIP, err)
	_, err = s.Activate(ipLicense.Key, server, "ip.example.com", "10.1.1.1")
	h.rejects(ErrInvalidServerIP, err)
	reinstalled := must(s.Activate(ipLicense.Key, other, "ip.example.com", "::ffff:8.8.4.4"))
	reinstalledClaims := h.claims(reinstalled)
	h.check(reinstalledClaims.ExpiresAt == ipClaims.ExpiresAt, "reinstall preserves expiry")
	h.check(reinstalledClaims.ServerID == other, "reinstall updates receipt identity")
	_, err = s.Refresh(ipLicense.ID, server, ipEnvelope.ActivationToken, "8.8.4.4")
	h.rejects(ErrInvalidActivation, err)
	must(s.Refresh(ipLicense.ID, other, reinstalled.ActivationToken, "8.8.4.4"))

	// A legacy server proves its original machine/credential once before IP binding.
	h.exec("UPDATE licenses SET server_ip=NULL,key_encrypted=NULL WHERE id=?", ipLicense.ID)
	_, err = s.Activate(ipLicense.Key, server, "ip.example.com", "8.8.4.4")
	h.rejects(ErrLicenseInUse, err)
	must(s.Refresh(ipLicense.ID, other, reinstalled.ActivationToken, "8.8.4.4"))
	beforeRotation := must(s.Activate(ipLicense.Key, server, "ip.example.com", "8.8.4.4"))
	h.check(must(s.Reveal(member.ID, ipLicense.ID, password)).Key == ipLicense.Key, "activation makes legacy key available")
	rotated := must(s.Rotate(member.ID, ipLicense.ID, password))
	_, err = s.Refresh(ipLicense.ID, server, beforeRotation.ActivationToken, "8.8.4.4")
	h.rejects(ErrInvalidActivation, err)
	_, err = s.Activate(ipLicense.Key, server, "ip.example.com", "8.8.4.4")
	h.rejects(ErrInvalidLicense, err)
	_, err = s.Activate(rotated.Key, server, "ip.example.com", "9.9.9.9")
	h.rejects(ErrLicenseInUse, err)
	rotatedClaims := h.claims(must(s.Activate(rotated.Key, server, "ip.example.com", "8.8.4.4")))
	h.check(rotatedClaims.ExpiresAt == ipClaims.ExpiresAt, "rotation preserves expiry and IP")
	h.check(must(s.Reveal(member.ID, rotated.ID, password)).Key == rotated.Key, "replacement key can be revealed")
	// Copying encrypted bytes to another license cannot expose or swap the key.
	h.exec("UPDATE licenses SET key_encrypted=(SELECT key_encrypted FROM licenses WHERE id=?) WHERE id=?", license.ID, ipLicense.ID)
	_, err = s.Reveal(member.ID, ipLicense.ID, password)
	h.rejects(ErrKeyNotSaved, err)

	h.now = c.ExpiresAt - 86400
	near := h.claims(must(s.Refresh(license.ID, other, moved.ActivationToken, "9.9.9.9")))
	h.check(near.OfflineUntil == near.ExpiresAt && near.RefreshAfter == h.now+3600, "offline window never passes the annual expiry")
	h.now = c.ExpiresAt
	_, err = s.Activate(replacement.Key, other, "boston.example.com", "9.9.9.9")
	h.rejects(ErrLicenseExpired, err)
	expired := h.claims(must(s.Refresh(license.ID, other, moved.ActivationToken, "9.9.9.9")))
	h.check(expired.ExpiresAt <= h.now, "refresh reports expired term")
	if err := s.Renew(member.ID, license.ID, password); err != nil {
		t.Fatal(err)
	}
	renewed := h.claims(must(s.Refresh(license.ID, other, moved.ActivationToken, "9.9.9.9")))
	h.check(renewed.ExpiresAt > h.now, "renewal refreshed")
	h.rejects(ErrRenewalNotDue, s.Renew(member.ID, license.ID, password))
	if err := s.RequestToken("alice@example.com", "reset"); err != nil {
		t.Fatal(err)
	}
	reset := h.lastMail().Token
	h.rejects(ErrInvalidToken, s.ConsumeToken(reset, "verify", password))
	if err := s.ConsumeToken(reset, "reset", "a-new-password-12345"); err != nil {
		t.Fatal(err)
	}
	h.check(must(s.MemberByEpoch(member.ID, member.Epoch)) == nil, "reset revokes all sessions")
	_, err = s.Login("alice@example.com", password)
	h.rejects(ErrInvalidLogin, err)
	must(s.Login("alice@example.com", "a-new-password-12345"))
	h.rejects(ErrInvalidToken, s.ConsumeToken(reset, "reset", "another-password-12345"))
	if err := s.RequestToken("alice@example.com", "reset"); err != nil {
		t.Fatal(err)
	}
	h.now += 1801
	h.rejects(ErrInvalidToken, s.ConsumeToken(h.lastMail().Token, "reset", "another-password-12345"))
	raw := must(os.ReadFile(filepath.Join(h.dir, "db.sqlite")))
	wal, _ := os.ReadFile(filepath.Join(h.dir, "db.sqlite-wal"))
	h.check(!strings.Contains(string(raw)+string(wal), license.Key), "key not plaintext")
	listing := must(json.Marshal(must(s.Licenses(member.ID))))
	h.check(!strings.Contains(string(listing), "activation_token") && !strings.Contains(string(listing), license.Key), "listing does not expose credentials")

	// Removal: a bound license stops refreshing and activating, leaves the list,
	// keeps its row and event, and the owner can no longer act on it.
	gone := must(s.Issue(member.ID))
	goneEnvelope := must(s.Activate(gone.Key, strings.Repeat("c", 64), "gone.example.com", "8.8.8.8"))
	h.rejects(ErrInvalidLogin, s.Remove(member.ID, gone.ID, "wrong"))
	if err := s.Remove(member.ID, gone.ID, "a-new-password-12345"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Refresh(gone.ID, strings.Repeat("c", 64), goneEnvelope.ActivationToken, "8.8.8.8")
	h.rejects(ErrInvalidActivation, err)
	_, err = s.Activate(gone.Key, strings.Repeat("c", 64), "gone.example.com", "8.8.8.8")
	h.rejects(ErrInvalidLicense, err)
	ids := []string{}
	for _, l := range must(s.Licenses(member.ID)) {
		ids = append(ids, l.ID)
	}
	h.check(!slices.Contains(ids, gone.ID), "removed license leaves the list")
	_, err = s.Reveal(member.ID, gone.ID, "a-new-password-12345")
	h.rejects(ErrLicenseNotFound, err)
	h.rejects(ErrLicenseNotFound, s.Remove(member.ID, gone.ID, "a-new-password-12345"))
	var removedAt, keyEncrypted, serverID any
	if err := s.DB().QueryRow("SELECT removed,key_encrypted,server_id FROM licenses WHERE id=?", gone.ID).Scan(&removedAt, &keyEncrypted, &serverID); err != nil {
		t.Fatal(err)
	}
	h.check(removedAt != nil && keyEncrypted == nil && serverID == nil, "removal keeps the row without the key")
	var removedEvents int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM events WHERE kind='removed' AND license_id=?", gone.ID).Scan(&removedEvents); err != nil {
		t.Fatal(err)
	}
	h.check(removedEvents == 1, "removal is recorded")
	if err := s.Limit("one", 1, 60); err != nil {
		t.Fatal(err)
	}
	h.rejects(ErrRateLimited, s.Limit("one", 1, 60))
	h.now += 61
	if err := s.Limit("one", 1, 60); err != nil {
		t.Fatal(err)
	}

	// Migration fills only unstarted legacy terms and preserves granted expiries and key hashes.
	legacy := must(s.Issue(member.ID))
	before := dumpRows(t, s, "SELECT id,key_hash,expires FROM licenses ORDER BY id")
	h.exec("UPDATE licenses SET expires=NULL,key_encrypted=NULL WHERE id=?", legacy.ID)
	migrated := h.open()
	after := dumpRows(t, migrated, "SELECT id,key_hash,expires FROM licenses ORDER BY id")
	h.check(slices.Equal(before, after), "migration preserves keys and granted terms")
	var unsaved int
	if err := migrated.DB().QueryRow("SELECT COUNT(*) FROM licenses WHERE key_encrypted IS NULL AND removed IS NULL").Scan(&unsaved); err != nil {
		t.Fatal(err)
	}
	h.check(unsaved == 1, "legacy key is not silently replaced")

	// Two processes race different servers for the same unbound license.
	race := must(s.Issue(member.ID))
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		child := h.open()
		wg.Add(1)
		go func(i int, child *Service) {
			defer wg.Done()
			ip := "8.8.8.8"
			if i == 1 {
				ip = "9.9.9.9"
			}
			_, results[i] = child.Activate(race.Key, strings.Repeat(string(rune('1'+i)), 64), "race.example.com", ip)
		}(i, child)
	}
	wg.Wait()
	winners, inUse := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrLicenseInUse):
			inUse++
		}
	}
	h.check(winners == 1 && inUse == 1, "concurrent activation has one winner")

	// Every label of service.php is here (the plan's acceptance list).
	for _, want := range phpServiceLabels {
		if !slices.Contains(h.labels, want) {
			t.Errorf("service.php check %q has no Go twin", want)
		}
	}
}

// phpServiceLabels are the check() labels of portal-membership/tests/service.php and
// one entry per distinct rejects() code in it.
var phpServiceLabels = []string{
	"verification sent", "term starts on issue", "same key can be revealed", "explicit replacement creates a new key",
	"legacy replacement preserves expiry", "replacement is saved automatically", "stored key is encrypted",
	"hourly check, seven-day offline window", "calendar year leap date", "retry preserves term", "retry preserves credential",
	"transfer preserves the key", "transfer preserves expiry", "transfer binds other server", "reinstall preserves expiry",
	"reinstall updates receipt identity", "activation makes legacy key available", "rotation preserves expiry and IP",
	"replacement key can be revealed", "offline window never passes the annual expiry", "refresh reports expired term",
	"renewal refreshed", "reset revokes all sessions", "key not plaintext", "listing does not expose credentials",
	"removed license leaves the list", "removal keeps the row without the key", "removal is recorded",
	"migration preserves keys and granted terms", "legacy key is not silently replaced", "concurrent activation has one winner",
	"rejects invalid_email", "rejects invalid_login", "rejects invalid_token", "rejects license_not_found", "rejects key_not_saved",
	"rejects license_in_use", "rejects invalid_activation", "rejects renewal_not_due", "rejects invalid_server_ip",
	"rejects invalid_license", "rejects license_expired", "rejects rate_limited",
}

func dumpRows(t *testing.T, s *Service, query string) []string {
	t.Helper()
	rows, err := s.DB().Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := must(rows.Columns())
	var out []string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		out = append(out, string(must(json.Marshal(values))))
	}
	return out
}
