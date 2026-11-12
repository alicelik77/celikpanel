//go:build linux

package pdnspeerjournal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/pdnspeerproof"
	"golang.org/x/sys/unix"
)

// Interrupted writes and retirements (alpha.133, lab of 12 Nov 2026). A stop
// at a file operation is simulated by copying the directory exactly as it is
// at that operation (a killed process runs no deferred cleanup; a power loss
// keeps at most what was made durable, which is always one of these copies
// because every step is followed by an fsync), then running Recover on the
// copy as the next attempt would.

const journalName = "pdns-peer-challenge-v1.json"

// copyDirectory copies the regular files of dir, with their mode and root
// ownership, into a new private directory: the disk at that instant.
func copyDirectory(t *testing.T, dir string) string {
	t.Helper()
	copied := t.TempDir()
	if err := os.Chmod(copied, 0700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("unexpected entry %s: %v", entry.Name(), err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(copied, entry.Name())
		if err := os.WriteFile(target, raw, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(target, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(target, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	return copied
}

func storeIn(s store, dir string) store {
	return store{path: filepath.Join(dir, journalName), trustRoot: dir, now: s.now}
}

// listing returns name:size:mode of every entry, sorted.
func listing(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%s:%d:%o", entry.Name(), info.Size(), info.Mode()))
	}
	sort.Strings(out)
	return out
}

func noStages(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != journalName {
			t.Fatalf("left behind after recovery: %s", entry.Name())
		}
	}
}

func finishedYes(RecordV1) (bool, error) { return true, nil }

func finishedNotAsked(t *testing.T) FinishedCheck {
	return func(RecordV1) (bool, error) {
		t.Error("a write's recovery asked the ledger")
		return false, errors.New("not expected")
	}
}

func writeRoot(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 0); err != nil {
		t.Fatal(err)
	}
}

// nextRequest is request as the next proof attempt mints it.
func nextRequest(t *testing.T, request pdnspeerproof.RequestV1, nonce string) (pdnspeerproof.RequestV1, string) {
	t.Helper()
	next := request
	next.Attempt++
	next.Nonce = strings.Repeat(nonce, 64)
	digest, err := pdnspeerproof.RequestSHA256(next)
	if err != nil {
		t.Fatal(err)
	}
	return next, digest
}

// continueAfter proves the journal admits the next challenge of an operation
// after recovery: the same operation's next attempt, or attempt 1 of a new
// operation when the journal is empty.
func continueAfter(t *testing.T, s store, request pdnspeerproof.RequestV1) {
	t.Helper()
	current, err := s.read()
	next := request
	switch {
	case err == nil:
		next = current.Request
		next.Attempt++
	case IsCode(err, Missing):
		next.MutationRequestID = strings.Repeat("7", 32)
		next.Attempt = 1
	default:
		t.Fatalf("journal after recovery: %v", err)
	}
	next.Nonce = strings.Repeat("6", 64)
	if err := s.publish(next, fixtureMembers, enrollment, 9, allow, nil); err != nil {
		t.Fatalf("next challenge after recovery: %v", err)
	}
	digest, err := pdnspeerproof.RequestSHA256(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.consume(next, enrollment, digest, 9, allow, nil); err != nil {
		t.Fatalf("its answer after recovery: %v", err)
	}
}

// The exact lab state (alpha.132; the lab of 12 Nov 2026 found it in the BIND-secondary
// journal, and this one had the same retirement): a 0-byte journal (the tombstone, mtime of the cut-off
// retirement) beside .pdns-peer-retired-* holding the consumed record of an
// earlier, finished deletion of another zone. One Recover completes the
// retirement; the next deletion's attempt 1 is admitted; nothing stays.
func TestRecoverCompletesTheLabsTornAlpha132Retirement(t *testing.T) {
	s, request, _ := fixture(t)
	dir := filepath.Dir(s.path)
	writeRoot(t, s.path, nil)
	writeRoot(t, filepath.Join(dir, ".pdns-peer-retired-1d291fcb0d73e582d8c9ae595505fb98"), earlierConsumedRecord(t))
	if _, err := s.read(); !IsCode(err, Unknown) {
		t.Fatalf("torn state read: %v", err)
	}
	var asked []string
	repair, err := s.recoverAt(allow, func(record RecordV1) (bool, error) {
		asked = append(asked, record.Request.DeletedZone)
		return true, nil
	}, nil)
	if err != nil || repair != RepairRetirementCompleted {
		t.Fatalf("repair=%q err=%v", repair, err)
	}
	if len(asked) != 1 || asked[0] != "earlier.example.test" {
		t.Fatalf("ledger asked about %v", asked)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left behind: %v", listing(t, dir))
	}
	if _, err := s.read(); !IsCode(err, Missing) {
		t.Fatalf("after repair: %v", err)
	}
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatalf("the next deletion's first challenge: %v", err)
	}
	// A second Recover finds nothing to do.
	if repair, err := s.recoverAt(allow, finishedNotAsked(t), nil); err != nil || repair != RepairNone {
		t.Fatalf("second recover: %q %v", repair, err)
	}
}

// The same torn state when the ledger does not show the retired record's
// operation finished (an operation still in flight, or one this ledger does
// not know): nothing in this package retires such a record, so the state is
// unexplained and kept for a person. A ledger that could not be read is not
// that answer: the state stays as it is and the attempt may simply repeat.
func TestRecoverKeepsATornRetirementWhoseOperationIsNotFinished(t *testing.T) {
	for _, tc := range []struct {
		name     string
		finished FinishedCheck
		want     Code
	}{
		{"not finished", func(RecordV1) (bool, error) { return false, nil }, Unresolved},
		{"ledger unreadable", func(RecordV1) (bool, error) { return false, errors.New("locks lost") }, Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := fixture(t)
			dir := filepath.Dir(s.path)
			writeRoot(t, s.path, nil)
			writeRoot(t, filepath.Join(dir, ".pdns-peer-retired-00"), earlierConsumedRecord(t))
			before := listing(t, dir)
			if _, err := s.recoverAt(allow, tc.finished, nil); !IsCode(err, tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if after := listing(t, dir); strings.Join(after, " ") != strings.Join(before, " ") {
				t.Fatalf("changed: %v -> %v", before, after)
			}
		})
	}
}

// The held check (locks, active attempt) is required before anything is
// read for a repair and again right before the first removal.
func TestRecoverNeedsTheHeldCheck(t *testing.T) {
	s, _, _ := fixture(t)
	dir := filepath.Dir(s.path)
	writeRoot(t, s.path, nil)
	writeRoot(t, filepath.Join(dir, ".pdns-peer-retired-00"), earlierConsumedRecord(t))
	for _, failAt := range []int{1, 2} {
		calls := 0
		verify := func() error {
			calls++
			if calls == failAt {
				return errors.New("lock lost")
			}
			return nil
		}
		if _, err := s.recoverAt(verify, finishedYes, nil); !IsCode(err, Unknown) {
			t.Fatalf("fail at %d: %v", failAt, err)
		}
		if got := listing(t, dir); len(got) != 2 {
			t.Fatalf("fail at %d changed the directory: %v", failAt, got)
		}
	}
	if _, err := s.recoverAt(nil, finishedYes, nil); !IsCode(err, Unknown) {
		t.Fatalf("nil verify: %v", err)
	}
	if _, err := s.recoverAt(allow, nil, nil); !IsCode(err, Unknown) {
		t.Fatalf("nil finished: %v", err)
	}
}

// Every file operation of the alpha.133 retirement, and of the alpha.132
// (and earlier) one, stopped: the next attempt's Recover always ends with no
// journal, nothing left behind, and the next deletion admitted. A stop of
// the recovery itself between its two removals is completed the same way.
func TestRecoverAfterAStopAtEveryRetirementOperation(t *testing.T) {
	type state struct {
		name string
		dir  string
	}
	var states []state
	// alpha.133: before the rename, after it, after its fsync, after the
	// retired record's removal (before the last fsync).
	{
		s, request, digest := fixture(t)
		if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
			t.Fatal(err)
		}
		if err := s.consume(request, enrollment, digest, 1, allow, nil); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(s.path)
		if err := s.retireAt(request, enrollment, digest, 1, allow, func(at string) {
			states = append(states, state{"alpha133/" + at, copyDirectory(t, dir)})
		}); err != nil {
			t.Fatal(err)
		}
		if len(states) != 4 {
			t.Fatalf("retirement operations seen: %d", len(states))
		}
	}
	// alpha.132: its file operations, replayed as it ran them.
	for _, st := range alpha132RetirementStates(t) {
		states = append(states, st)
	}
	for _, st := range states {
		t.Run(st.name, func(t *testing.T) {
			s, request, _ := fixture(t)
			s = storeIn(s, st.dir)
			repair, err := s.recoverAt(allow, finishedYes, nil)
			if err != nil {
				t.Fatalf("recover: %v", err)
			}
			switch repair {
			case RepairNone, RepairRetirementCompleted, RepairEmptyStageRemoved:
			default:
				t.Fatalf("repair %q", repair)
			}
			noStages(t, st.dir)
			if got, err := s.read(); err == nil {
				// Stopped before the record moved: the journal is the
				// record itself, which the caller's retirement now retires.
				if got.State != StateConsumed {
					t.Fatalf("journal %+v", got)
				}
				if err := s.retire(got.Request, got.EnrollmentSHA256, got.RequestSHA256, got.LedgerAttempt, allow); err != nil {
					t.Fatalf("retirement after recovery: %v", err)
				}
			} else if !IsCode(err, Missing) {
				t.Fatalf("journal after recovery: %v", err)
			}
			noStages(t, st.dir)
			continueAfter(t, s, request)
		})
	}
	// The recovery of the lab state stopped between its two removals.
	t.Run("recovery stopped between its removals", func(t *testing.T) {
		s, request, _ := fixture(t)
		dir := filepath.Dir(s.path)
		writeRoot(t, s.path, nil)
		writeRoot(t, filepath.Join(dir, ".pdns-peer-retired-00"), earlierConsumedRecord(t))
		var stopped string
		if _, err := s.recoverAt(allow, finishedYes, func(at string) {
			if stopped == "" {
				stopped = copyDirectory(t, dir)
			}
		}); err != nil {
			t.Fatal(err)
		}
		next := storeIn(s, stopped)
		if repair, err := next.recoverAt(allow, finishedYes, nil); err != nil || repair != RepairRetirementCompleted {
			t.Fatalf("second recovery: %q %v", repair, err)
		}
		noStages(t, stopped)
		continueAfter(t, next, request)
	})
}

// alpha132RetirementStates replays the retirement of alpha.132 and earlier
// releases (tombstone, exchange, unlink journal, unlink retired; each
// followed by an fsync) on a consumed record and returns the directory after
// each of its file operations.
func alpha132RetirementStates(t *testing.T) []struct {
	name string
	dir  string
} {
	t.Helper()
	s, request, digest := fixture(t)
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.consume(request, enrollment, digest, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(s.path)
	retired := filepath.Join(dir, ".pdns-peer-retired-0123456789abcdef0123456789abcdef")
	var out []struct {
		name string
		dir  string
	}
	at := func(name string) {
		out = append(out, struct {
			name string
			dir  string
		}{"alpha132/" + name, copyDirectory(t, dir)})
	}
	fd, err := unix.Open(retired, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Fchown(fd, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := unix.Fchmod(fd, 0600); err != nil {
		t.Fatal(err)
	}
	unix.Close(fd)
	at("tombstone_created")
	if err := unix.Renameat2(unix.AT_FDCWD, s.path, unix.AT_FDCWD, retired, unix.RENAME_EXCHANGE); err != nil {
		t.Fatal(err)
	}
	at("exchanged") // the lab state
	if err := os.Remove(s.path); err != nil {
		t.Fatal(err)
	}
	at("journal_unlinked")
	if err := os.Remove(retired); err != nil {
		t.Fatal(err)
	}
	at("retired_unlinked")
	return out
}

// Every file operation of a write (the first publish, a publish over an
// earlier challenge, consume-once) stopped: Recover never asks the ledger,
// leaves either the record from before the write or the written one, nothing
// behind, and the proof continues with the next challenge.
func TestRecoverAfterAStopAtEveryWriteOperation(t *testing.T) {
	type writeCase struct {
		name  string
		setup func(s store, request pdnspeerproof.RequestV1, digest string)
		write func(s store, request pdnspeerproof.RequestV1, digest string, at func(string)) error
	}
	cases := []writeCase{
		{"first_publish", func(store, pdnspeerproof.RequestV1, string) {},
			func(s store, r pdnspeerproof.RequestV1, _ string, at func(string)) error {
				return s.publish(r, fixtureMembers, enrollment, 1, allow, at)
			}},
		{"next_publish", func(s store, r pdnspeerproof.RequestV1, _ string) {
			if err := s.publish(r, fixtureMembers, enrollment, 1, allow, nil); err != nil {
				t.Fatal(err)
			}
		}, func(s store, r pdnspeerproof.RequestV1, _ string, at func(string)) error {
			next, _ := nextRequest(t, r, "5")
			return s.publish(next, fixtureMembers, enrollment, 2, allow, at)
		}},
		{"consume", func(s store, r pdnspeerproof.RequestV1, _ string) {
			if err := s.publish(r, fixtureMembers, enrollment, 1, allow, nil); err != nil {
				t.Fatal(err)
			}
		}, func(s store, r pdnspeerproof.RequestV1, digest string, at func(string)) error {
			return s.consume(r, enrollment, digest, 1, allow, at)
		}},
	}
	for _, wc := range cases {
		s, request, digest := fixture(t)
		wc.setup(s, request, digest)
		before, beforeErr := s.read()
		dir := filepath.Dir(s.path)
		type stop struct {
			at  string
			dir string
		}
		var stops []stop
		if err := wc.write(s, request, digest, func(at string) {
			stops = append(stops, stop{at, copyDirectory(t, dir)})
		}); err != nil {
			t.Fatalf("%s: %v", wc.name, err)
		}
		written, err := s.read()
		if err != nil {
			t.Fatal(err)
		}
		want := 7 // created, written, staged, before_exchange, published, parent_durable, stage_removed
		if beforeErr != nil {
			want = 6
		}
		if len(stops) != want {
			t.Fatalf("%s: write operations seen: %d", wc.name, len(stops))
		}
		for _, st := range stops {
			t.Run(wc.name+"/"+st.at, func(t *testing.T) {
				next := storeIn(s, st.dir)
				repair, err := next.recoverAt(allow, finishedNotAsked(t), nil)
				if err != nil {
					t.Fatalf("recover: %v", err)
				}
				noStages(t, st.dir)
				got, readErr := next.read()
				switch {
				case readErr == nil && got == written:
					if repair != RepairNone && repair != RepairWriteCompleted {
						t.Fatalf("written record with repair %q", repair)
					}
				case readErr == nil && beforeErr == nil && got == before,
					IsCode(readErr, Missing) && IsCode(beforeErr, Missing):
					if repair != RepairNone && repair != RepairWriteRolledBack && repair != RepairEmptyStageRemoved {
						t.Fatalf("earlier record with repair %q", repair)
					}
				default:
					t.Fatalf("journal after recovery: %+v %v (repair %q)", got, readErr, repair)
				}
				continueAfter(t, next, request)
			})
		}
	}
}

// What Recover cannot prove stays exactly as it is, as Unresolved.
func TestRecoverRefusesWhatItCannotProve(t *testing.T) {
	s0, request, digest := fixture(t)
	if err := s0.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	outstanding, err := os.ReadFile(s0.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s0.consume(request, enrollment, digest, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	consumed, err := os.ReadFile(s0.path)
	if err != nil {
		t.Fatal(err)
	}
	other := request
	other.MutationRequestID = strings.Repeat("8", 32)
	other.Nonce = strings.Repeat("8", 64)
	otherDigest, err := pdnspeerproof.RequestSHA256(other)
	if err != nil {
		t.Fatal(err)
	}
	unrelated := mustEncode(RecordV1{Schema: SchemaV1, State: StateOutstanding, LedgerAttempt: 1,
		EnrollmentSHA256: enrollment, RequestSHA256: otherDigest, Request: other})
	type file struct {
		name string
		raw  []byte
		mode os.FileMode
	}
	journal := func(raw []byte) file { return file{journalName, raw, 0600} }
	retired := func(raw []byte) file { return file{".pdns-peer-retired-01", raw, 0600} }
	challenge := func(raw []byte) file { return file{".pdns-peer-challenge-01", raw, 0600} }
	cases := []struct {
		name  string
		files []file
	}{
		{"empty journal alone", []file{journal(nil)}},
		{"journal not a record", []file{journal([]byte("owner text"))}},
		{"journal unsafe mode", []file{{journalName, consumed, 0644}}},
		{"two stages", []file{journal(nil), retired(consumed), {".pdns-peer-retired-02", consumed, 0600}}},
		{"retired stage not a record", []file{journal(nil), retired([]byte("owner terminal edit"))}},
		{"retired stage unsafe mode", []file{journal(nil), {".pdns-peer-retired-01", consumed, 0640}}},
		{"retired record beside a journal record", []file{journal(outstanding), retired(consumed)}},
		{"two empty files", []file{journal(nil), retired(nil)}},
		{"challenge stage not a record", []file{journal(outstanding), challenge([]byte("{\"schema\""))}},
		{"challenge record beside an empty journal", []file{journal(nil), challenge(consumed)}},
		{"challenge record of another operation", []file{journal(outstanding), challenge(unrelated)}},
		{"challenge stage equal to the journal", []file{journal(outstanding), challenge(outstanding)}},
		{"empty stage beside a foreign journal", []file{journal([]byte("x")), challenge(nil)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := fixture(t)
			dir := filepath.Dir(s.path)
			for _, f := range tc.files {
				path := filepath.Join(dir, f.name)
				writeRoot(t, path, f.raw)
				if err := os.Chmod(path, f.mode); err != nil {
					t.Fatal(err)
				}
			}
			before := listing(t, dir)
			repair, err := s.recoverAt(allow, finishedYes, nil)
			if !IsCode(err, Unresolved) || repair != RepairNone {
				t.Fatalf("repair=%q err=%v", repair, err)
			}
			if after := listing(t, dir); strings.Join(after, " ") != strings.Join(before, " ") {
				t.Fatalf("changed: %v -> %v", before, after)
			}
		})
	}
	// A symbolic link under a stage name is not followed or removed.
	t.Run("symlink stage", func(t *testing.T) {
		s, _, _ := fixture(t)
		dir := filepath.Dir(s.path)
		writeRoot(t, s.path, nil)
		if err := os.Symlink("/etc/passwd", filepath.Join(dir, ".pdns-peer-retired-01")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.recoverAt(allow, finishedYes, nil); !IsCode(err, Unresolved) {
			t.Fatalf("symlink: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(dir, ".pdns-peer-retired-01")); err != nil {
			t.Fatalf("symlink removed: %v", err)
		}
	})
}

// An owner edit in the instant before a retirement's rename is kept under
// the retired name and is not something Recover removes.
func TestOwnerEditAtRetirementIsUnresolvedForRecover(t *testing.T) {
	s, request, digest := fixture(t)
	if err := s.publish(request, fixtureMembers, enrollment, 1, allow, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.retireAt(request, enrollment, digest, 1, allow, func(at string) {
		if at == "before_rename" {
			if e := os.WriteFile(s.path, []byte("owner terminal edit"), 0600); e != nil {
				t.Fatal(e)
			}
		}
	}); !IsCode(err, Unknown) {
		t.Fatalf("owner edit retired: %v", err)
	}
	if _, err := s.recoverAt(allow, finishedYes, nil); !IsCode(err, Unresolved) {
		t.Fatalf("owner edit recovered: %v", err)
	}
}

// Until alpha.133 every stage listing after the first on one directory
// descriptor read from the end of the directory and saw nothing.
func TestEveryStageListingSeesTheWholeDirectory(t *testing.T) {
	s, _, _ := fixture(t)
	dir, _, err := s.openDirectory()
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(dir)
	if err := countOrphanStages(dir); err != nil {
		t.Fatalf("empty directory: %v", err)
	}
	writeRoot(t, filepath.Join(filepath.Dir(s.path), ".pdns-peer-challenge-late"), []byte("x"))
	if err := countOrphanStages(dir); !IsCode(err, Unknown) {
		t.Fatalf("second listing missed the stage: %v", err)
	}
	if err := countOrphanStages(dir); !IsCode(err, Unknown) {
		t.Fatalf("third listing missed the stage: %v", err)
	}
}

// earlierConsumedRecord is the consumed record of an earlier, finished
// deletion of another zone, as alpha.132 and every earlier release wrote it.
func earlierConsumedRecord(t *testing.T) []byte {
	t.Helper()
	s, request, _ := fixture(t)
	request.MutationRequestID = strings.Repeat("1", 32)
	request.MutationOwnerID = strings.Repeat("2", 32)
	request.DeletedZone = "earlier.example.test"
	request.Nonce = strings.Repeat("4", 64)
	digest, err := pdnspeerproof.RequestSHA256(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.publish(request, fixtureMembers, enrollment, 2, allow, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.consume(request, enrollment, digest, 2, allow, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
