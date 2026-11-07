package main

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Emptying a mailbox (alpha.128, gap G20). Dovecot removes the messages of
// every folder itself (doveadm expunge ... mailbox '*' all), so its indexes,
// quota and any client connected stay consistent; the mailbox, its password,
// folders and settings stay. The quota is recalculated afterwards. Nothing
// here touches a file of the mailbox directly.
//
// Posta kutusunu boşaltma: Dovecot bütün klasörlerin iletilerini kendisi
// siler; kutu, parolası, klasörleri ve ayarları kalır; kota yeniden
// hesaplanır.

const emptyMailboxTimeout = 9 * time.Minute

// emptyMailboxRun is replaced by tests; production uses the mail launcher.
var emptyMailboxRun = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return executeMailTLSCommand(ctx, name, args...)
}

// countDoveadmMessages adds up "messages=N" values of
// `doveadm -f flow mailbox status -u U messages '*'`.
func countDoveadmMessages(out []byte) (int, bool) {
	total, seen := 0, false
	for _, field := range strings.Fields(string(out)) {
		value, ok := strings.CutPrefix(field, "messages=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return 0, false
		}
		total += n
		seen = true
	}
	return total, seen
}

// EmptyMailboxV1 removes every message of one mailbox through Dovecot.
// EmptyMailboxV1 bir posta kutusunun bütün iletilerini Dovecot ile siler.
func (a *Agent) EmptyMailboxV1(req *transport.EmptyMailboxRequest, resp *transport.EmptyMailboxResponse) error {
	if req == nil || resp == nil {
		return errors.New("empty mailbox request and response are required")
	}
	*resp = transport.EmptyMailboxResponse{Messages: -1}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "emptying a mailbox"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	email, _, _, err := canonicalAgentMailbox(req.Email)
	if err != nil {
		resp.Error = "invalid mailbox address"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), emptyMailboxTimeout)
	defer cancel()
	notFound := func(err error) bool {
		var exit *exec.ExitError
		return errors.As(err, &exit) && doveadmNobodyKicked(exit.ExitCode())
	}
	if out, err := emptyMailboxRun(ctx, "doveadm", "-f", "flow", "mailbox", "status", "-u", email, "messages", "*"); err == nil {
		if n, ok := countDoveadmMessages(out); ok {
			resp.Messages = n
		}
	} else if notFound(err) {
		resp.Unknown = true
		resp.Error = "Dovecot does not know this mailbox, so nothing was removed"
		return nil
	}
	out, err := emptyMailboxRun(ctx, "doveadm", "expunge", "-u", email, "mailbox", "*", "all")
	if err != nil {
		if notFound(err) {
			resp.Unknown = true
			resp.Error = "Dovecot does not know this mailbox, so nothing was removed"
			return nil
		}
		resp.Error = hostcmd.Fail("Dovecot could not remove the mailbox's messages", out, err, nil).Error()
		return nil
	}
	resp.Applied = true
	if _, err := emptyMailboxRun(ctx, "doveadm", "quota", "recalc", "-u", email); err == nil {
		resp.QuotaRecalculated = true
	}
	return nil
}
