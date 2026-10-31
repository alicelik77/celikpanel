package pdnspeerinspector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeerproof"
	"github.com/alicelik/celikpanel/internal/pdnspeerproof"
)

// Serve handles one bounded canonical request and emits only one minimal proof
// document. An SSH forced-command wrapper and authenticated client are separate
// enrollment prerequisites; this package exposes no shell or mutation API.
func Serve(ctx context.Context, input io.Reader, output io.Writer, policy PolicyReader, native Reader, now func() time.Time) error {
	return ServeNoted(ctx, input, output, policy, native, now, nil)
}

// Bounded re-reads of an unclassified native observation (final alpha.113
// acceptance run 4, D9). Variables only so tests can shorten the delay.
var (
	nativeInspectionAttempts   = 3
	nativeInspectionRetryDelay = 750 * time.Millisecond
	// nativeInspectionExpiryRoom keeps a re-read clear of the request's
	// expiry, so the primary still receives a valid answer.
	nativeInspectionExpiryRoom = 5 * time.Second
)

// ServeNoted is Serve with a diagnostic note for the inspector's own host.
// note receives bounded, product-authored lines (DiagnosticText): the first
// twelve characters of the request digest, which inspection failed and why.
// They never carry the nonce or other request fields, keys or file content,
// and they never reach the primary, which receives only the reviewed reason
// line. Before 31 Oct 2026 an unclassified failure left no trace on the
// secondary at all.
func ServeNoted(ctx context.Context, input io.Reader, output io.Writer, policy PolicyReader, native Reader, now func() time.Time, note func(string)) error {
	if input == nil || output == nil {
		return errors.New("PowerDNS inspector stream unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(input, 4098))
	if err != nil || len(raw) > 4097 {
		return errors.New("PowerDNS inspector request exceeds safe bound")
	}
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	request, err := pdnspeerproof.DecodeRequest(raw)
	if err != nil {
		return err
	}
	digest, err := pdnspeerproof.RequestSHA256(request)
	if err != nil {
		return err
	}
	response, err := inspectNoted(ctx, request, digest, native, policy, now, note)
	if err != nil {
		return &InspectionFailure{RequestSHA256: digest, Err: err}
	}
	encoded, err := pdnspeerproof.EncodeResponse(response)
	if err != nil {
		return err
	}
	_, err = output.Write(append(encoded, '\n'))
	return err
}

// inspectNoted runs Inspect and, only after an unclassified failure of the
// native observation (nativeReadError without a reviewed reason), reads again
// a bounded number of times while the request stays valid. Every read is the
// complete two-observation Inspect: a re-read never relaxes what an answer
// must prove, it only gives a momentary native state (the daemon writing its
// database or serving its control socket while the inspector reads) a second
// look. Policy, request-validity and reviewed-reason failures are final.
func inspectNoted(ctx context.Context, request pdnspeerproof.RequestV1, digest string, native Reader, policy PolicyReader, now func() time.Time, note func(string)) (pdnspeerproof.ResponseV1, error) {
	short := digest
	if len(short) > 12 {
		short = short[:12]
	}
	attempts := max(nativeInspectionAttempts, 1)
	for attempt := 1; ; attempt++ {
		response, err := Inspect(ctx, request, native, policy, now)
		if err == nil {
			if attempt > 1 && note != nil {
				note(fmt.Sprintf("PowerDNS peer inspection for request %s answered on read %d of %d", short, attempt, attempts))
			}
			return response, nil
		}
		reason := Reason(err)
		if note != nil {
			label := reason
			if label == "" {
				label = "unclassified"
			}
			note(fmt.Sprintf("PowerDNS peer inspection for request %s: read %d of %d failed (%s): %s",
				short, attempt, attempts, label, DiagnosticText(err)))
		}
		var read *nativeReadError
		if reason != "" || !errors.As(err, &read) || attempt >= attempts {
			return pdnspeerproof.ResponseV1{}, err
		}
		if now().Add(nativeInspectionRetryDelay + nativeInspectionExpiryRoom).After(time.Unix(request.ExpiresAtUnix, 0)) {
			return pdnspeerproof.ResponseV1{}, err
		}
		select {
		case <-ctx.Done():
			return pdnspeerproof.ResponseV1{}, err
		case <-time.After(nativeInspectionRetryDelay):
		}
	}
}

// DiagnosticText is a bounded, single-line, printable form of an inspection
// error for the inspector host's own log. Inspection errors are
// product-authored sentences, optionally wrapping a local system or SQLite
// error; none carries request fields, keys or file content.
func DiagnosticText(err error) string {
	if err == nil {
		return "(none)"
	}
	var out strings.Builder
	for _, r := range err.Error() {
		if out.Len() >= 300 {
			out.WriteString("...")
			break
		}
		if r < 0x20 || r == 0x7f || r > 0x7e {
			out.WriteByte('?')
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// InspectionFailure keeps the decoded request's digest with an inspection
// error, so the only detail the primary can receive is a reviewed reason
// token bound to that exact challenge.
type InspectionFailure struct {
	RequestSHA256 string
	Err           error
}

func (f *InspectionFailure) Error() string { return f.Err.Error() }
func (f *InspectionFailure) Unwrap() error { return f.Err }

// ReasonLine returns the single reviewed stderr line
// (dnspeerproof.InspectorReasonPrefixV1) for a failed inspection that is bound
// to a decoded request and classified with a reviewed reason. Anything else
// yields false and the caller prints its fixed generic sentence; raw error
// text is never printed.
func ReasonLine(err error) (string, bool) {
	var failure *InspectionFailure
	if !errors.As(err, &failure) {
		return "", false
	}
	return dnspeerproof.FormatInspectorReason(failure.RequestSHA256, Reason(failure.Err))
}
