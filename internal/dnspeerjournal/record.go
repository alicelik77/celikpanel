package dnspeerjournal

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/alicelik/celikpanel/internal/dnspeerproof"
)

const (
	SchemaV1         = "celikpanel-dns-peer-challenge-journal/v1"
	StateOutstanding = "outstanding"
	StateConsumed    = "consumed"
	MaxBytes         = 8192
)

// Path is a separate, agent-private sidecar. Missing storage is unknown for an
// active operation; no reader may reconstruct an outstanding challenge.
const Path = "/var/lib/celikpanel-agent-private/dns-peer-challenge-v1.json"

type Code string

const (
	Missing  Code = "peer_challenge_missing"
	Unknown  Code = "peer_challenge_unknown"
	Mismatch Code = "peer_challenge_context_mismatch"
	Replay   Code = "peer_challenge_replayed"
	// Unresolved: the journal directory holds a state no interrupted write or
	// retirement of this package can produce, or one whose completion cannot
	// be proved (Recover). Unlike Unknown it does not change by waiting: a
	// person must look at it.
	Unresolved Code = "peer_challenge_unresolved"
)

// The private names beside Path. A challenge stage is a write's temporary
// file (and, after an exchange, the displaced before-image); a retired stage
// holds a record between its retirement's rename and its removal.
const (
	challengePrefix = ".dns-peer-challenge-"
	retiredPrefix   = ".dns-peer-retired-"
)

// Repair names what Recover did to an interrupted change. RepairNone: the
// journal needed nothing.
type Repair string

const (
	RepairNone Repair = ""
	// A retirement whose rename (or, before alpha.133, whose exchange with
	// an empty tombstone) happened: the retired record of a finished
	// operation was removed (and the empty tombstone with it).
	RepairRetirementCompleted Repair = "retirement_completed"
	// An empty private file of a write or retirement that stopped before it
	// wrote anything was removed; the journal itself was not changed.
	RepairEmptyStageRemoved Repair = "empty_stage_removed"
	// A write whose exchange happened: its displaced before-image was removed.
	RepairWriteCompleted Repair = "write_completed"
	// A write that stopped before its record became the journal: its
	// never-published staged record was removed.
	RepairWriteRolledBack Repair = "write_rolled_back"
)

// FinishedCheck reports whether record's operation is terminal (published)
// in the durable ledger, read under the caller's held locks. An error means
// the answer is not known (the locks or the ledger could not be checked);
// false means the ledger was read and does not show it finished.
type FinishedCheck func(record RecordV1) (bool, error)

type StateError struct{ Code Code }

func (e StateError) Error() string { return string(e.Code) }
func IsCode(err error, code Code) bool {
	var value StateError
	return errors.As(err, &value) && value.Code == code
}

// RecordV1 freezes the exact challenge and reviewed enrollment digest. A
// consumed record remains durable; it is never interpreted as another success.
type RecordV1 struct {
	Schema           string                 `json:"schema"`
	State            string                 `json:"state"`
	LedgerAttempt    uint64                 `json:"ledger_attempt"`
	EnrollmentSHA256 string                 `json:"enrollment_sha256"`
	RequestSHA256    string                 `json:"request_sha256"`
	Request          dnspeerproof.RequestV1 `json:"request"`
}

func hex32(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (r RecordV1) Validate() error {
	if r.Schema != SchemaV1 || (r.State != StateOutstanding && r.State != StateConsumed) || r.LedgerAttempt == 0 ||
		!hex32(r.EnrollmentSHA256) || !hex32(r.RequestSHA256) {
		return StateError{Unknown}
	}
	digest, err := dnspeerproof.RequestSHA256(r.Request)
	if err != nil || digest != r.RequestSHA256 {
		return StateError{Unknown}
	}
	return nil
}

func Encode(r RecordV1) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > MaxBytes {
		return nil, StateError{Unknown}
	}
	return raw, nil
}

func Decode(raw []byte) (RecordV1, error) {
	var record RecordV1
	if len(raw) == 0 || len(raw) > MaxBytes {
		return RecordV1{}, StateError{Unknown}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || record.Validate() != nil {
		return RecordV1{}, StateError{Unknown}
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(raw, canonical) {
		return RecordV1{}, StateError{Unknown}
	}
	return record, nil
}

// sameOperation compares everything that binds a challenge to its operation,
// enrollment and endpoints. It deliberately excludes nonce, attempt,
// issue/expiry time and the catalog state (serial and member digest), which
// admissibleSuccessor judges separately.
func sameOperation(a, b RecordV1) bool {
	x, y := a.Request, b.Request
	return a.EnrollmentSHA256 == b.EnrollmentSHA256 &&
		x.MutationRequestID == y.MutationRequestID && x.MutationOwnerID == y.MutationOwnerID &&
		x.DeletionGeneration == y.DeletionGeneration && x.DeletionQualifier == y.DeletionQualifier &&
		x.PrimaryIP == y.PrimaryIP && x.PeerIP == y.PeerIP &&
		x.PeerIdentitySHA256 == y.PeerIdentitySHA256 && x.CatalogName == y.CatalogName &&
		x.DeletedZone == y.DeletedZone && x.View == y.View
}

// admissibleSuccessor decides whether next may supersede the stored previous
// challenge (the caller separately requires attempt+1, a new nonce and a
// non-decreasing ledger attempt). Enrollment, endpoints and the operation
// identity never change. The catalog state may either be identical, or have
// advanced exactly as the PowerDNS daemon re-stamps its producer catalog: a
// strictly higher serial over the identical member set. The store verifies
// that member set itself: members must reproduce both the stored and the new
// CatalogMembersSHA256 at their own serials.
//
// This is the BIND-secondary copy of the rule pdnspeerjournal received for
// final alpha.113 acceptance run 4 (D7). A PowerDNS main paired with a BIND
// backup uses this journal, and until 6 Nov 2026 it refused any serial
// change: a challenge minted just before the daemon's re-stamp left every
// later challenge of the same deletion refused as a context mismatch for
// ever (alpha.125, a joined server's domain deletion stuck at
// dns_peer_journal_unknown). Superseding cannot replay: the earlier request
// is no longer the journal's record, so ConsumeOnce refuses any answer bound
// to it, and its nonce is never reused. A lower serial, a changed member
// set, an advance without the member set, or any other field change stays a
// mismatch.
func admissibleSuccessor(previous, next RecordV1, members []string) bool {
	if !sameOperation(previous, next) {
		return false
	}
	x, y := previous.Request, next.Request
	if x.CatalogSerial == y.CatalogSerial {
		return x.CatalogMembersSHA256 == y.CatalogMembersSHA256
	}
	if y.CatalogSerial < x.CatalogSerial || x.CatalogMembersSHA256 == y.CatalogMembersSHA256 {
		return false
	}
	before, err := dnspeerproof.CatalogMembersSHA256(x.PrimaryIP, x.CatalogSerial, members)
	if err != nil || before != x.CatalogMembersSHA256 {
		return false
	}
	after, err := dnspeerproof.CatalogMembersSHA256(y.PrimaryIP, y.CatalogSerial, members)
	return err == nil && after == y.CatalogMembersSHA256
}
