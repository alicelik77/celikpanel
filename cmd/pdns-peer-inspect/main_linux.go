//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/syslog"
	"os"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeertransport"
	"github.com/alicelik/celikpanel/internal/pdnspeerinspector"
)

// journalTag names the inspector's lines in the secondary's journal:
// sudo journalctl -t celikpanel-pdns-peer-inspect
const journalTag = "celikpanel-pdns-peer-inspect"

func main() {
	note := journalNote()
	if err := run(note); err != nil {
		line := failureLine(err)
		if note != nil {
			// What the primary receives, with the cause already noted above
			// (final alpha.113 acceptance run 4, D9: before, an unclassified
			// failure left no trace on the secondary).
			note("PowerDNS peer inspection failed; the primary receives: " + line)
		}
		fmt.Fprintln(os.Stderr, line)
		os.Exit(1)
	}
}

// journalNote returns a writer of bounded diagnostic lines to the local
// journal through syslog, or nil when syslog is unavailable. The lines come
// from pdnspeerinspector.ServeNoted and failureLine only: no nonce, request
// field other than a digest prefix, key or file content. A logging failure
// never changes the inspection result.
func journalNote() func(string) {
	writer, err := syslog.New(syslog.LOG_WARNING|syslog.LOG_DAEMON, journalTag)
	if err != nil {
		return nil
	}
	return func(line string) { _ = writer.Warning(line) }
}

// failureLine is the single stderr line of a failed inspection: the reviewed
// reason line when the error is classified and bound to a decoded request,
// otherwise the fixed generic sentence. Raw error text is never printed.
func failureLine(err error) string {
	if line, ok := pdnspeerinspector.ReasonLine(err); ok {
		return line
	}
	return "native PowerDNS peer observation unavailable"
}

// An owner must install this as a separately reviewed, root-owned forced
// command for a dedicated non-root SSH account. The command itself does not
// enroll accounts or mutate DNS state.
func run(note func(string)) error {
	if len(os.Args) != 1 || (os.Getenv("SSH_ORIGINAL_COMMAND") != "" &&
		os.Getenv("SSH_ORIGINAL_COMMAND") != dnspeertransport.FixedPowerDNSCommand) {
		if note != nil {
			note("PowerDNS peer inspection refused: the inspector accepts no command or argument")
		}
		return errors.New("inspector accepts no command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return runWithNote(ctx, os.Stdin, os.Stdout, pdnspeerinspector.OwnerPolicyReader{}, pdnspeerinspector.NativeReader{}, time.Now, note)
}

func runWith(ctx context.Context, input io.Reader, output io.Writer, policy pdnspeerinspector.PolicyReader, native pdnspeerinspector.Reader, now func() time.Time) error {
	return runWithNote(ctx, input, output, policy, native, now, nil)
}

func runWithNote(ctx context.Context, input io.Reader, output io.Writer, policy pdnspeerinspector.PolicyReader, native pdnspeerinspector.Reader, now func() time.Time, note func(string)) error {
	err := pdnspeerinspector.ServeNoted(ctx, input, output, policy, native, now, note)
	var failure *pdnspeerinspector.InspectionFailure
	if err != nil && note != nil && !errors.As(err, &failure) {
		// Failures before a decoded request (stream, size, canonical form)
		// carry no request and are noted here; inspection failures were
		// noted by ServeNoted with their read number.
		note("PowerDNS peer inspection stopped before an inspection: " + pdnspeerinspector.DiagnosticText(err))
	}
	return err
}
