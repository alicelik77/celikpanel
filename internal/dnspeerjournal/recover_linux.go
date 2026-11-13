//go:build linux

package dnspeerjournal

import (
	"errors"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

// Interrupted changes and what Recover does with them (alpha.133, lab of
// 12 Nov 2026: an alpha.132 retirement stopped by its step deadline left a
// 0-byte journal beside a .dns-peer-retired-* file holding the consumed
// record of an earlier, finished deletion; every later proof of the next
// deletion refused it, so that deletion never finished).
//
// Every file operation of a write (publish, consume-once) and of a
// retirement, in this release and in every earlier one, can stop at any
// point (process killed, power loss, an error after the commit point). The
// directory is then in one of these states; J is the journal, S the one
// private stage beside it:
//
//	J        S                         produced by                        Recover
//	absent   -                         -                                  nothing to do
//	record   -                         -                                  nothing to do
//	record   empty, any prefix         write or a.132 retirement stopped  remove S
//	absent   empty, any prefix           before writing anything           remove S
//	empty    retired, record           a.132 retirement after exchange    J, S removed if S's
//	absent   retired, record           a.132 after unlinking J; alpha.133   operation finished;
//	                                     after its rename                   else Unresolved
//	absent   challenge, record         first write before its rename      remove S (never sent)
//	record   challenge, record R       write after exchange (R = J's      remove S (the write
//	                                     before-image)                      is complete)
//	record   challenge, record R       write before exchange (J = R's     remove S (the write
//	                                     before-image)                      never happened)
//	anything else (two or more stages, a stage that is not a record, an
//	unsafe or foreign file, a 0-byte journal without a retired record, a
//	retired record beside a journal, unrelated records)              Unresolved
//
// Why no repair can weaken single use: an answer is accepted only by
// ConsumeOnce against the journal's current outstanding record, under the
// active attempt. Recover never writes or restores a record; it only removes
// a file that is not the journal (or the empty file a.132 left under the
// journal's name). Removing a record can only make fewer answers acceptable,
// and the next challenge always has a fresh random nonce. A staged record
// that never became the journal was never sent (Publish completes before any
// exchange); a consumed record of a finished operation is retired exactly as
// Retire would, under the same held locks and the same terminal ledger proof
// (FinishedCheck). What is removed is either empty, a superseded
// before-image of the journal, a never-published stage, or a record whose
// operation the ledger shows finished: none is evidence an invariant needs.
// A retired record whose operation the ledger does not show finished is not
// retired by this package, so its state is unexplained: it is kept.

type entryKind int

const (
	entryAbsent entryKind = iota
	// A regular 0600 root:root file with one link and no bytes.
	entryEmpty
	// A safe file holding one valid, canonical record.
	entryRecord
	// Anything else: another file type, owner, mode or link count, or bytes
	// that are not a record.
	entryForeign
)

type journalEntry struct {
	name   string
	kind   entryKind
	record RecordV1
	stat   unix.Stat_t
}

// observeAt classifies name in dir without following links. An error is a
// read that did not complete: the state is unknown, not foreign.
func observeAt(dir int, name string) (journalEntry, error) {
	e := journalEntry{name: name}
	err := unix.Fstatat(dir, name, &e.stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return e, nil
	}
	if err != nil {
		return journalEntry{}, StateError{Unknown}
	}
	if !safeFile(e.stat) || e.stat.Size > MaxBytes {
		e.kind = entryForeign
		return e, nil
	}
	if e.stat.Size == 0 {
		e.kind = entryEmpty
		return e, nil
	}
	fd, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return journalEntry{}, StateError{Unknown}
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	var opened, after unix.Stat_t
	if unix.Fstat(fd, &opened) != nil || !sameFile(opened, e.stat) {
		return journalEntry{}, StateError{Unknown}
	}
	raw, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil || len(raw) != int(opened.Size) || unix.Fstat(fd, &after) != nil ||
		!sameFile(opened, after) || opened.Mtim != after.Mtim || opened.Ctim != after.Ctim {
		return journalEntry{}, StateError{Unknown}
	}
	record, err := Decode(raw)
	if err != nil {
		e.kind = entryForeign
		return e, nil
	}
	e.kind, e.record = entryRecord, record
	return e, nil
}

// stageNames lists the private stages in dir from its first entry.
func stageNames(dir int) ([]string, error) {
	duplicate, err := unix.Dup(dir)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "dns-peer-journal-directory")
	defer file.Close()
	// The duplicate shares dir's file offset: start from the beginning.
	if _, err := unix.Seek(duplicate, 0, io.SeekStart); err != nil {
		return nil, err
	}
	names, err := file.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	var stages []string
	for _, name := range names {
		if strings.HasPrefix(name, challengePrefix) || strings.HasPrefix(name, retiredPrefix) {
			stages = append(stages, name)
		}
	}
	sort.Strings(stages)
	return stages, nil
}

type writeRelationKind int

const (
	relationUnrelated writeRelationKind = iota
	// The stage is the journal's before-image: the write's exchange happened.
	relationDisplacedBefore
	// The stage is the next record of the journal: the write stopped before
	// its exchange, so the journal never held it.
	relationUnpublishedNext
)

// writeRelation tells, from content alone, which side of a write's exchange
// a challenge stage is on. Publish replaces a record of the same operation
// by the next proof attempt with a fresh nonce; ConsumeOnce replaces an
// outstanding record by the same record consumed. Nothing else is a write.
func writeRelation(stage, journal RecordV1) writeRelationKind {
	if !sameOperation(stage, journal) {
		return relationUnrelated
	}
	if stage.Request == journal.Request {
		if stage.RequestSHA256 != journal.RequestSHA256 || stage.LedgerAttempt != journal.LedgerAttempt {
			return relationUnrelated
		}
		switch {
		case stage.State == StateOutstanding && journal.State == StateConsumed:
			return relationDisplacedBefore
		case stage.State == StateConsumed && journal.State == StateOutstanding:
			return relationUnpublishedNext
		}
		return relationUnrelated
	}
	if stage.Request.Nonce == journal.Request.Nonce {
		return relationUnrelated
	}
	switch {
	case journal.State == StateOutstanding && journal.Request.Attempt == stage.Request.Attempt+1 &&
		journal.LedgerAttempt >= stage.LedgerAttempt:
		return relationDisplacedBefore
	case stage.State == StateOutstanding && stage.Request.Attempt == journal.Request.Attempt+1 &&
		stage.LedgerAttempt >= journal.LedgerAttempt:
		return relationUnpublishedNext
	}
	return relationUnrelated
}

// recoverAt is Recover with a checkpoint after each removal (tests only).
func (s store) recoverAt(verify VerifyCurrent, finished FinishedCheck, checkpoint func(string)) (Repair, error) {
	processMu.Lock()
	defer processMu.Unlock()
	if os.Geteuid() != 0 || verify == nil || finished == nil {
		return RepairNone, StateError{Unknown}
	}
	if verify() != nil {
		return RepairNone, StateError{Unknown}
	}
	dir, name, err := s.openDirectory()
	if err != nil {
		return RepairNone, StateError{Unknown}
	}
	defer unix.Close(dir)
	stages, err := stageNames(dir)
	if err != nil {
		return RepairNone, StateError{Unknown}
	}
	journal, err := observeAt(dir, name)
	if err != nil {
		return RepairNone, err
	}
	if len(stages) == 0 {
		if journal.kind == entryAbsent || journal.kind == entryRecord {
			return RepairNone, nil
		}
		return RepairNone, StateError{Unresolved}
	}
	if len(stages) > 1 {
		return RepairNone, StateError{Unresolved}
	}
	stage, err := observeAt(dir, stages[0])
	if err != nil {
		return RepairNone, err
	}
	if stage.kind == entryAbsent {
		return RepairNone, StateError{Unknown}
	}
	retired := strings.HasPrefix(stage.name, retiredPrefix)
	var (
		repair Repair
		remove []journalEntry
	)
	switch {
	case stage.kind == entryEmpty && (journal.kind == entryAbsent || journal.kind == entryRecord):
		repair, remove = RepairEmptyStageRemoved, []journalEntry{stage}
	case retired && stage.kind == entryRecord && (journal.kind == entryAbsent || journal.kind == entryEmpty):
		done, checkErr := finished(stage.record)
		if checkErr != nil {
			return RepairNone, StateError{Unknown}
		}
		if !done {
			return RepairNone, StateError{Unresolved}
		}
		repair = RepairRetirementCompleted
		// The journal's name first: a stop in between leaves the state a
		// retirement after its rename leaves, which is completed the same way.
		if journal.kind == entryEmpty {
			remove = append(remove, journal)
		}
		remove = append(remove, stage)
	case !retired && stage.kind == entryRecord && journal.kind == entryAbsent:
		repair, remove = RepairWriteRolledBack, []journalEntry{stage}
	case !retired && stage.kind == entryRecord && journal.kind == entryRecord:
		switch writeRelation(stage.record, journal.record) {
		case relationDisplacedBefore:
			repair = RepairWriteCompleted
		case relationUnpublishedNext:
			repair = RepairWriteRolledBack
		default:
			return RepairNone, StateError{Unresolved}
		}
		remove = []journalEntry{stage}
	default:
		return RepairNone, StateError{Unresolved}
	}
	if verify() != nil || s.verifyDirectory(dir) != nil {
		return RepairNone, StateError{Unknown}
	}
	for _, e := range remove {
		var now unix.Stat_t
		if unix.Fstatat(dir, e.name, &now, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameFile(e.stat, now) ||
			e.stat.Mtim != now.Mtim || unix.Unlinkat(dir, e.name, 0) != nil {
			return RepairNone, StateError{Unknown}
		}
		if checkpoint != nil {
			checkpoint("removed " + e.name)
		}
		if unix.Fsync(dir) != nil {
			return RepairNone, StateError{Unknown}
		}
	}
	// The result is a clean journal: no stage, and the journal unchanged.
	left, err := stageNames(dir)
	if err != nil || len(left) != 0 {
		return RepairNone, StateError{Unknown}
	}
	after, err := observeAt(dir, name)
	if err != nil {
		return RepairNone, err
	}
	switch journal.kind {
	case entryRecord:
		if after.kind != entryRecord || after.record != journal.record || !sameFile(after.stat, journal.stat) {
			return RepairNone, StateError{Unknown}
		}
	default:
		if after.kind != entryAbsent {
			return RepairNone, StateError{Unknown}
		}
	}
	return repair, nil
}
