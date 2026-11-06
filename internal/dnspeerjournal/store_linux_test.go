//go:build linux

package dnspeerjournal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeerproof"
)

const testNow int64 = 1_800_000_000

// fixtureMembers is the catalog member set of every fixture request.
var fixtureMembers = []string{"other.example.test"}

const enrollment = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func fixture(t *testing.T) (store, dnspeerproof.RequestV1, string) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root-owned journal fixture requires root")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s := store{path: filepath.Join(dir, "dns-peer-challenge-v1.json"), trustRoot: dir, now: func() time.Time { return time.Unix(testNow+1, 0) }}
	members, err := dnspeerproof.CatalogMembersSHA256("192.0.2.10", 12, fixtureMembers)
	if err != nil {
		t.Fatal(err)
	}
	r := dnspeerproof.RequestV1{
		Schema:            dnspeerproof.RequestSchemaV1,
		MutationRequestID: strings.Repeat("b", 32), MutationOwnerID: strings.Repeat("c", 32),
		DeletionGeneration: 17, DeletionQualifier: "dns-zone-sync/v3:sha256:" + strings.Repeat("d", 64),
		PrimaryIP: "192.0.2.10", PeerIP: "192.0.2.11", PeerIdentitySHA256: strings.Repeat("e", 64),
		CatalogName: "catalog-c000020a.celikpanel.invalid", CatalogSerial: 12,
		CatalogMembersSHA256: members, DeletedZone: "gone.example.test", View: dnspeerproof.DefaultView,
		Nonce: strings.Repeat("f", 64), Attempt: 1, IssuedAtUnix: testNow, ExpiresAtUnix: testNow + 60,
	}
	digest, err := dnspeerproof.RequestSHA256(r)
	if err != nil {
		t.Fatal(err)
	}
	return s, r, digest
}

func allow() error { return nil }

func TestDurablePublishConsumeReplayAndFreshAttempt(t *testing.T) {
	s, request, digest := fixture(t)
	if _, err := s.read(); !IsCode(err, Missing) {
		t.Fatalf("missing: %v", err)
	}
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.read()
	if err != nil || got.State != StateOutstanding || got.RequestSHA256 != digest {
		t.Fatalf("published: %+v %v", got, err)
	}
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); !IsCode(err, Replay) {
		t.Fatalf("duplicate publish: %v", err)
	}
	if err := s.consume(request, enrollment, digest, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.read()
	if err != nil || got.State != StateConsumed {
		t.Fatalf("consumed: %+v %v", got, err)
	}
	if err := s.consume(request, enrollment, digest, 1, allow, nil); !IsCode(err, Replay) {
		t.Fatalf("replay: %v", err)
	}
	next := request
	next.Nonce = strings.Repeat("1", 64)
	next.Attempt++
	if err := s.publish(next, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatalf("fresh attempt: %v", err)
	}
	if err := s.consume(request, enrollment, digest, 1, allow, nil); !IsCode(err, Mismatch) {
		t.Fatalf("old attempt accepted: %v", err)
	}
}

func TestContextAndOwnerEditRefusedWithoutChangingBytes(t *testing.T) {
	s, request, digest := fixture(t)
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*dnspeerproof.RequestV1){
		func(r *dnspeerproof.RequestV1) { r.MutationOwnerID = strings.Repeat("1", 32) },
		func(r *dnspeerproof.RequestV1) { r.DeletionGeneration++ },
		func(r *dnspeerproof.RequestV1) { r.CatalogSerial++ },
	} {
		candidate := request
		edit(&candidate)
		candidate.Attempt++
		candidate.Nonce = strings.Repeat("1", 64)
		if err := s.publish(candidate, fixtureMembers, enrollment, 1, allow, nil); !IsCode(err, Mismatch) {
			t.Fatalf("context edit: %v", err)
		}
	}
	if err := s.consume(request, strings.Repeat("2", 64), digest, 1, allow, nil); !IsCode(err, Mismatch) {
		t.Fatalf("enrollment edit: %v", err)
	}
	if err := s.consume(request, enrollment, digest, 1, func() error { return errors.New("owner changed") }, nil); !IsCode(err, Unknown) {
		t.Fatalf("owner edit: %v", err)
	}
	after, err := os.ReadFile(s.path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("journal changed after refusal: %v", err)
	}
}

func TestCrashAtPublishAndConsumeCheckpoints(t *testing.T) {
	for _, point := range []string{"staged", "published", "parent_durable"} {
		t.Run("publish_"+point, func(t *testing.T) {
			s, request, _ := fixture(t)
			func() {
				defer func() { _ = recover() }()
				_ = s.publish(request, fixtureMembers, enrollment, 1, allow, func(at string) {
					if at == point {
						panic(at)
					}
				})
			}()
			got, err := s.read()
			if point == "staged" {
				if !IsCode(err, Missing) {
					t.Fatalf("stage became authority: %+v %v", got, err)
				}
				return
			}
			if err != nil || got.State != StateOutstanding {
				t.Fatalf("lost published challenge: %+v %v", got, err)
			}
		})
		t.Run("consume_"+point, func(t *testing.T) {
			s, request, digest := fixture(t)
			if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() { _ = recover() }()
				_ = s.consume(request, enrollment, digest, 1, allow, func(at string) {
					if at == point {
						panic(at)
					}
				})
			}()
			got, err := s.read()
			if point == "staged" {
				if err != nil || got.State != StateOutstanding {
					t.Fatalf("stage became authority: %+v %v", got, err)
				}
				return
			}
			if !IsCode(err, Unknown) {
				t.Fatalf("interrupted exchange did not block reuse: %+v %v", got, err)
			}
			raw, err := os.ReadFile(s.path)
			if err != nil {
				t.Fatal(err)
			}
			published, err := Decode(raw)
			if err != nil || published.State != StateConsumed {
				t.Fatalf("consumed checkpoint missing: %+v %v", published, err)
			}
			if !IsCode(s.consume(request, enrollment, digest, 1, allow, nil), Unknown) {
				t.Fatal("consumed proof reused through retained stage")
			}
		})
	}
}

func TestConcurrentConsumeOneWinner(t *testing.T) {
	s, request, digest := fixture(t)
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.consume(request, enrollment, digest, 1, allow, nil) == nil {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("successes: %d", won.Load())
	}
}

func TestUnsafeSidecarFailsClosed(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "mode"} {
		t.Run(kind, func(t *testing.T) {
			s, request, _ := fixture(t)
			if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink":
				if err := os.Remove(s.path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("/etc/passwd", s.path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(s.path, s.path+".link"); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(s.path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.read(); !IsCode(err, Unknown) {
				t.Fatalf("unsafe file read: %v", err)
			}
		})
	}
}

func TestNewAdmissionSupersedesOutstandingAndRejectsLateResponse(t *testing.T) {
	s, old, oldDigest := fixture(t)
	if err := s.publish(old, fixtureMembers, enrollment, 7, allow, nil); err != nil {
		t.Fatal(err)
	}
	next := old
	next.Nonce = strings.Repeat("1", 64)
	next.Attempt++
	if err := s.publish(next, fixtureMembers, enrollment, 8, allow, nil); err != nil {
		t.Fatalf("fresh admission: %v", err)
	}
	if err := s.consume(old, enrollment, oldDigest, 7, allow, nil); !IsCode(err, Mismatch) {
		t.Fatalf("late response accepted: %v", err)
	}
	nextDigest, err := dnspeerproof.RequestSHA256(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.consume(next, enrollment, nextDigest, 7, allow, nil); !IsCode(err, Mismatch) {
		t.Fatalf("wrong ledger admission accepted: %v", err)
	}
	if err := s.consume(next, enrollment, nextDigest, 8, allow, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRestartMustMintNewChallengeEvenWithSameLedgerAttempt(t *testing.T) {
	s, old, oldDigest := fixture(t)
	if err := s.publish(old, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	restarted := store{path: s.path, trustRoot: s.trustRoot, now: s.now}
	if _, err := restarted.read(); err != nil {
		t.Fatal(err)
	}
	next := old
	next.Nonce = strings.Repeat("1", 64)
	next.Attempt++
	if err := restarted.publish(next, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	if err := restarted.consume(old, enrollment, oldDigest, 1, allow, nil); !IsCode(err, Mismatch) {
		t.Fatalf("prior response accepted after fresh challenge: %v", err)
	}
}

func TestOwnerEditDuringExchangeIsPreservedAsUnknown(t *testing.T) {
	s, request, digest := fixture(t)
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	next := request
	next.Nonce = strings.Repeat("1", 64)
	next.Attempt++
	err := s.publish(next, fixtureMembers, enrollment, 1, allow, func(at string) {
		if at != "before_exchange" {
			return
		}
		if e := os.WriteFile(s.path, []byte("owner changed"), 0600); e != nil {
			t.Fatal(e)
		}
	})
	if !IsCode(err, Unknown) {
		t.Fatalf("owner edit was accepted: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(s.path))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".dns-peer-challenge-") {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(filepath.Dir(s.path), entry.Name()))
		if e == nil && string(raw) == "owner changed" {
			found = true
		}
	}
	if !found {
		t.Fatal("displaced owner edit was not retained")
	}
	if err := s.consume(request, enrollment, digest, 1, allow, nil); !IsCode(err, Unknown) {
		t.Fatalf("prior response accepted after uncertain exchange: %v", err)
	}
}

func TestTerminalRetirementIsExactAndDurable(t *testing.T) {
	s, request, digest := fixture(t)
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.retire(request, enrollment, digest, 1, func() error { return errors.New("not terminal") }); !IsCode(err, Unknown) {
		t.Fatalf("nonterminal retirement: %v", err)
	}
	if _, err := s.read(); err != nil {
		t.Fatal(err)
	}
	if err := s.retire(request, enrollment, digest, 2, allow); !IsCode(err, Mismatch) {
		t.Fatalf("wrong attempt retirement: %v", err)
	}
	if err := s.retire(request, enrollment, digest, 1, allow); err != nil {
		t.Fatal(err)
	}
	if _, err := s.read(); !IsCode(err, Missing) {
		t.Fatalf("retired journal remained: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(s.path))
	if err != nil || len(entries) != 0 {
		t.Fatalf("retirement left history: %d %v", len(entries), err)
	}
	other := request
	other.MutationRequestID = strings.Repeat("1", 32)
	other.Nonce = strings.Repeat("2", 64)
	if err := s.publish(other, fixtureMembers, enrollment, 2, allow, nil); err != nil {
		t.Fatalf("later operation blocked: %v", err)
	}
}

func TestUnsafeAncestorRefused(t *testing.T) {
	s, request, _ := fixture(t)
	root := s.trustRoot
	parent := filepath.Join(root, "nested")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(parent, "journal.json")
	if err := os.Chmod(parent, 0770); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); !IsCode(err, Unknown) {
		t.Fatalf("writable ancestor accepted: %v", err)
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, parent); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); !IsCode(err, Unknown) {
		t.Fatalf("symlink ancestor accepted: %v", err)
	}
}

func TestSingleAbandonedStageBlocksFurtherProof(t *testing.T) {
	s, request, _ := fixture(t)
	name := filepath.Join(filepath.Dir(s.path), ".dns-peer-challenge-interrupted")
	if err := os.WriteFile(name, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); !IsCode(err, Unknown) {
		t.Fatalf("interrupted stage accepted: %v", err)
	}
	if _, err := s.read(); !IsCode(err, Unknown) {
		t.Fatalf("read ignored interrupted stage: %v", err)
	}
}

func TestOwnerEditAtTerminalRetirementExchangeIsPreserved(t *testing.T) {
	s, request, digest := fixture(t)
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	err := s.retireAt(request, enrollment, digest, 1, allow, func(at string) {
		if at != "before_exchange" {
			return
		}
		if e := os.WriteFile(s.path, []byte("owner terminal edit"), 0600); e != nil {
			t.Fatal(e)
		}
	})
	if !IsCode(err, Unknown) {
		t.Fatalf("owner edit retired as success: %v", err)
	}
	if _, err := s.read(); !IsCode(err, Unknown) {
		t.Fatalf("uncertain retirement read: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(s.path))
	if err != nil {
		t.Fatal(err)
	}
	retained := false
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".dns-peer-retired-") {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(filepath.Dir(s.path), entry.Name()))
		if e == nil && string(raw) == "owner terminal edit" {
			retained = true
		}
	}
	if !retained {
		t.Fatal("owner terminal edit was not retained")
	}
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); !IsCode(err, Unknown) {
		t.Fatalf("uncertain stage allowed another proof: %v", err)
	}
}

// restamped returns request as the same operation mints it after the
// PowerDNS daemon re-stamped its producer catalog to serial: the identical
// member set, a fresh nonce and the next proof attempt.
func restamped(t *testing.T, request dnspeerproof.RequestV1, serial uint32, nonce string) dnspeerproof.RequestV1 {
	t.Helper()
	digest, err := dnspeerproof.CatalogMembersSHA256(request.PrimaryIP, serial, fixtureMembers)
	if err != nil {
		t.Fatal(err)
	}
	next := request
	next.CatalogSerial = serial
	next.CatalogMembersSHA256 = digest
	next.Nonce = strings.Repeat(nonce, 64)
	next.Attempt++
	return next
}

// 6 Nov 2026, alpha.125 (a joined server's domain deletion on a PowerDNS
// main with a BIND backup): attempt 3's challenge was written at catalog
// serial S; nine seconds later the PowerDNS daemon re-stamped the producer
// catalog (same single member) and every later challenge of the same
// deletion, bound to the re-stamped serial, was refused as
// peer_challenge_context_mismatch for ever. The same operation now
// supersedes it, and the superseded challenge can never be consumed.
func TestDaemonRestampAdvanceSupersedesOutstandingChallenge(t *testing.T) {
	s, old, oldDigest := fixture(t)
	if err := s.publish(old, fixtureMembers, enrollment, 3, allow, nil); err != nil {
		t.Fatal(err)
	}
	next := restamped(t, old, old.CatalogSerial+334981, "1")
	if err := s.publish(next, fixtureMembers, enrollment, 3, allow, nil); err != nil {
		t.Fatalf("re-stamped challenge of the same operation refused: %v", err)
	}
	if err := s.consume(old, enrollment, oldDigest, 3, allow, nil); !IsCode(err, Mismatch) {
		t.Fatalf("superseded challenge consumed: %v", err)
	}
	nextDigest, err := dnspeerproof.RequestSHA256(next)
	if err != nil {
		t.Fatal(err)
	}
	// A further advance under a later ledger attempt (the Agent restarted
	// by an update, or the owner's retry) is accepted too.
	third := restamped(t, next, next.CatalogSerial+1, "2")
	if err := s.publish(third, fixtureMembers, enrollment, 4, allow, nil); err != nil {
		t.Fatalf("second re-stamp advance refused: %v", err)
	}
	if err := s.consume(next, enrollment, nextDigest, 3, allow, nil); !IsCode(err, Mismatch) {
		t.Fatalf("second superseded challenge consumed: %v", err)
	}
	thirdDigest, err := dnspeerproof.RequestSHA256(third)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.consume(third, enrollment, thirdDigest, 4, allow, nil); err != nil {
		t.Fatalf("current challenge: %v", err)
	}
	if err := s.consume(third, enrollment, thirdDigest, 4, allow, nil); !IsCode(err, Replay) {
		t.Fatalf("consumed challenge replayed: %v", err)
	}
	fourth := third
	fourth.Nonce = strings.Repeat("3", 64)
	fourth.Attempt++
	if err := s.publish(fourth, fixtureMembers, enrollment, 4, allow, nil); err != nil {
		t.Fatalf("fresh challenge after consumption: %v", err)
	}
}

func TestCatalogAdvanceRefusedUnlessVerifiedRestampOfSameOperation(t *testing.T) {
	s, old, _ := fixture(t)
	if err := s.publish(old, fixtureMembers, enrollment, 3, allow, nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	advance := func(edit func(*dnspeerproof.RequestV1)) dnspeerproof.RequestV1 {
		r := restamped(t, old, old.CatalogSerial+1, "1")
		if edit != nil {
			edit(&r)
		}
		return r
	}
	changedMembers := []string{"new.example.test", "other.example.test"}
	changedDigest, err := dnspeerproof.CatalogMembersSHA256(old.PrimaryIP, old.CatalogSerial+1, changedMembers)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		label      string
		request    dnspeerproof.RequestV1
		members    []string
		enrollment string
		ledger     uint64
		want       Code
	}{
		{"lower serial", restamped(t, old, old.CatalogSerial-1, "1"), fixtureMembers, enrollment, 3, Mismatch},
		{"changed members", advance(func(r *dnspeerproof.RequestV1) { r.CatalogMembersSHA256 = changedDigest }), changedMembers, enrollment, 3, Mismatch},
		{"unbound members", advance(nil), []string{"x.example.test"}, enrollment, 3, Mismatch},
		{"no members", advance(nil), nil, enrollment, 3, Mismatch},
		{"serial without digest", advance(func(r *dnspeerproof.RequestV1) { r.CatalogMembersSHA256 = old.CatalogMembersSHA256 }), fixtureMembers, enrollment, 3, Mismatch},
		{"foreign enrollment", advance(nil), fixtureMembers, strings.Repeat("2", 64), 3, Mismatch},
		{"other request", advance(func(r *dnspeerproof.RequestV1) { r.MutationRequestID = strings.Repeat("1", 32) }), fixtureMembers, enrollment, 3, Mismatch},
		{"other owner", advance(func(r *dnspeerproof.RequestV1) { r.MutationOwnerID = strings.Repeat("1", 32) }), fixtureMembers, enrollment, 3, Mismatch},
		{"other generation", advance(func(r *dnspeerproof.RequestV1) { r.DeletionGeneration++ }), fixtureMembers, enrollment, 3, Mismatch},
		{"other peer identity", advance(func(r *dnspeerproof.RequestV1) { r.PeerIdentitySHA256 = strings.Repeat("9", 64) }), fixtureMembers, enrollment, 3, Mismatch},
		{"other zone", advance(func(r *dnspeerproof.RequestV1) { r.DeletedZone = "other-gone.example.test" }), fixtureMembers, enrollment, 3, Mismatch},
		{"old nonce", advance(func(r *dnspeerproof.RequestV1) { r.Nonce = old.Nonce }), fixtureMembers, enrollment, 3, Replay},
		{"same attempt", advance(func(r *dnspeerproof.RequestV1) { r.Attempt = old.Attempt }), fixtureMembers, enrollment, 3, Replay},
		{"skipped attempt", advance(func(r *dnspeerproof.RequestV1) { r.Attempt++ }), fixtureMembers, enrollment, 3, Replay},
		{"older ledger attempt", advance(nil), fixtureMembers, enrollment, 2, Replay},
	}
	for _, c := range cases {
		if err := s.publish(c.request, c.members, c.enrollment, c.ledger, allow, nil); !IsCode(err, c.want) {
			t.Fatalf("%s: got %v, want %s", c.label, err, c.want)
		}
	}
	after, err := os.ReadFile(s.path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("journal changed after a refused advance: %v", err)
	}
}
