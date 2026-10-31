package pdnspeerinspector

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/pdnspeerproof"
	"github.com/alicelik/celikpanel/internal/transport"
)

// scriptedReader returns errs[i] for read i (then the snapshot).
type scriptedReader struct {
	snapshot Snapshot
	errs     []error
	reads    int
}

func (r *scriptedReader) Read(context.Context, pdnspeerproof.RequestV1, OwnerPolicyV1) (Snapshot, error) {
	r.reads++
	if r.reads <= len(r.errs) && r.errs[r.reads-1] != nil {
		return Snapshot{}, r.errs[r.reads-1]
	}
	return r.snapshot, nil
}

func shortRetries(t *testing.T) {
	t.Helper()
	delay := nativeInspectionRetryDelay
	nativeInspectionRetryDelay = time.Millisecond
	t.Cleanup(func() { nativeInspectionRetryDelay = delay })
}

func serveNoted(t *testing.T, request pdnspeerproof.RequestV1, policy PolicyReader, reader Reader, at int64) (*bytes.Buffer, []string, error) {
	t.Helper()
	raw, err := pdnspeerproof.EncodeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var notes []string
	err = ServeNoted(context.Background(), bytes.NewReader(append(raw, '\n')), &out, policy, reader,
		func() time.Time { return time.Unix(at, 0) }, func(line string) { notes = append(notes, line) })
	return &out, notes, err
}

// Final alpha.113 acceptance run 4 (D9): a momentary native difference is
// read again inside the same request, and the secondary's own log keeps the
// cause; the answer still needs two complete, stable observations.
func TestUnclassifiedNativeFailureIsReadAgainAndNoted(t *testing.T) {
	shortRetries(t)
	request, policy, snapshot := fixture(t)
	busy := errors.New("PowerDNS SQLite integrity unverified: database is locked (5) (SQLITE_BUSY)")
	reader := &scriptedReader{snapshot: snapshot, errs: []error{busy}}
	out, notes, err := serveNoted(t, request, &testPolicy{p: policy, hash: "policy"}, reader, 1800000003)
	if err != nil {
		t.Fatalf("momentary native failure was not read again: %v", err)
	}
	response, decodeErr := pdnspeerproof.DecodeResponse(bytes.TrimSuffix(out.Bytes(), []byte("\n")))
	if decodeErr != nil || response.NativeState != "unloaded" || reader.reads != 3 {
		t.Fatalf("response %+v %v after %d reads", response, decodeErr, reader.reads)
	}
	joined := strings.Join(notes, "\n")
	if len(notes) != 2 || !strings.Contains(joined, "read 1 of 3 failed (unclassified): native observation 1: PowerDNS SQLite integrity unverified: database is locked") ||
		!strings.Contains(joined, "answered on read 2 of 3") {
		t.Fatalf("notes: %q", notes)
	}
	assertNoRequestSecrets(t, request, joined)
}

func TestPersistentNativeFailureIsBoundedAndGeneric(t *testing.T) {
	shortRetries(t)
	request, policy, snapshot := fixture(t)
	listener := errors.New("PowerDNS control socket framing unknown (row flags 00000000 type 0001 state 02)")
	reader := &scriptedReader{snapshot: snapshot, errs: []error{listener, listener, listener, listener, listener, listener}}
	out, notes, err := serveNoted(t, request, &testPolicy{p: policy, hash: "policy"}, reader, 1800000003)
	if err == nil || out.Len() != 0 || reader.reads != 3 || len(notes) != 3 {
		t.Fatalf("persistent failure: err=%v output=%q reads=%d notes=%q", err, out.String(), reader.reads, notes)
	}
	// The primary still receives no reviewed reason: the generic line.
	if _, ok := ReasonLine(err); ok {
		t.Fatalf("unclassified failure produced a reason line")
	}
	if !strings.Contains(notes[2], "read 3 of 3 failed (unclassified)") || !strings.Contains(notes[2], "state 02") {
		t.Fatalf("notes: %q", notes)
	}
	assertNoRequestSecrets(t, request, strings.Join(notes, "\n"))
}

func TestClassifiedPolicyAndExpiryFailuresAreNotReadAgain(t *testing.T) {
	shortRetries(t)
	request, policy, snapshot := fixture(t)
	t.Run("reviewed reason", func(t *testing.T) {
		reader := &scriptedReader{snapshot: snapshot, errs: []error{
			reasonError(transport.DNSPeerInspectorReasonConfigUnreviewed, "/etc/powerdns/pdns.d/zz.conf"),
		}}
		_, notes, err := serveNoted(t, request, &testPolicy{p: policy, hash: "policy"}, reader, 1800000003)
		if line, ok := ReasonLine(err); !ok || !strings.HasSuffix(line, " config_unreviewed") || reader.reads != 1 {
			t.Fatalf("reviewed reason: %q %t reads=%d", line, ok, reader.reads)
		}
		if len(notes) != 1 || !strings.Contains(notes[0], "(config_unreviewed)") {
			t.Fatalf("notes: %q", notes)
		}
	})
	t.Run("owner policy", func(t *testing.T) {
		reader := &scriptedReader{snapshot: snapshot}
		_, notes, err := serveNoted(t, request, &testPolicy{p: policy, hash: "policy", change: true}, reader, 1800000003)
		if err == nil || reader.reads != 2 || len(notes) != 1 || !strings.Contains(notes[0], "owner policy changed") {
			t.Fatalf("policy: err=%v reads=%d notes=%q", err, reader.reads, notes)
		}
	})
	t.Run("clock outside the request", func(t *testing.T) {
		reader := &scriptedReader{snapshot: snapshot}
		_, notes, err := serveNoted(t, request, &testPolicy{p: policy, hash: "policy"}, reader, 1800000000-20)
		if err == nil || reader.reads != 0 || len(notes) != 1 || !strings.Contains(notes[0], "inspector clock -20s from issue") {
			t.Fatalf("clock: err=%v reads=%d notes=%q", err, reader.reads, notes)
		}
	})
	t.Run("too close to expiry", func(t *testing.T) {
		reader := &scriptedReader{snapshot: snapshot, errs: []error{errors.New("PowerDNS process changed")}}
		_, notes, err := serveNoted(t, request, &testPolicy{p: policy, hash: "policy"}, reader, request.ExpiresAtUnix-3)
		if err == nil || reader.reads != 1 || len(notes) != 1 {
			t.Fatalf("expiry: err=%v reads=%d notes=%q", err, reader.reads, notes)
		}
	})
}

func TestDiagnosticTextIsBoundedAndSingleLine(t *testing.T) {
	text := DiagnosticText(errors.New("first\nsecond\x1b[31m" + strings.Repeat("x", 500) + "ü"))
	if strings.ContainsAny(text, "\n\x1b") || len(text) > 303 || !strings.HasPrefix(text, "first?second?[31m") {
		t.Fatalf("diagnostic text: %q", text)
	}
	if DiagnosticText(nil) != "(none)" {
		t.Fatal("nil error")
	}
}

func assertNoRequestSecrets(t *testing.T, request pdnspeerproof.RequestV1, notes string) {
	t.Helper()
	digest, err := pdnspeerproof.RequestSHA256(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{request.Nonce, request.MutationRequestID, request.MutationOwnerID, request.PeerIdentitySHA256, digest} {
		if strings.Contains(notes, secret) {
			t.Fatalf("note carries a request field: %q", notes)
		}
	}
	if !strings.Contains(notes, digest[:12]) {
		t.Fatalf("note lacks the request digest prefix: %q", notes)
	}
}
