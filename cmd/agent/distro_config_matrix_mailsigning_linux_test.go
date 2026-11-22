//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/configmatrix"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail signing decisions against each release's real programs (21 Nov 2026,
// independent review of alpha.143). The product's own decisions run, at
// render time, against the release's real postconf on a scratch copy of the
// stock /etc/postfix (postconf -c: real main.cf/master.cf syntax, real -n,
// -P, -PX and -X), and against the release's real `opendkim -n`; nothing
// under /etc changes. A decision that comes out wrong is a render failure of
// its case; the files that come out are validated by the release's Postfix.

// matrixScratchPostfix copies the stock main.cf and master.cf into a scratch
// directory and returns a runner whose postconf works on it.
func matrixScratchPostfix(t *testing.T) (string, postconfRunner, error) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"main.cf", "master.cf"} {
		data, err := os.ReadFile(filepath.Join("/etc/postfix", name))
		if err != nil {
			return "", nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return "", nil, err
		}
	}
	run := func(name string, args ...string) ([]byte, error) {
		if name != "postconf" {
			return nil, nil
		}
		return exec.Command("postconf", append([]string{"-c", dir}, args...)...).CombinedOutput()
	}
	return dir, run, nil
}

// matrixPostfixFilesCase is a postfix case whose main.cf and master.cf are
// the scratch directory's.
func matrixPostfixFilesCase(t *testing.T, dir, id, variant, source string) (*configmatrix.Case, error) {
	c := newPostfixCase(id, variant, source)
	for _, name := range []string{"main.cf", "master.cf"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		c.File(filepath.Join("/etc/postfix", name), data, 0o644)
	}
	return c, nil
}

func renderMatrixPostfixMailSigning(t *testing.T, h *matrixHost) {
	const source = "cmd/agent/mail_signing_bind.go planPort25Milters / localSubmitOpen, mail_stack_rpc.go composeAndWriteMilterChain"
	record := map[string]string{}
	savedRead, savedWrite := readPort25Record, writePort25Record
	t.Cleanup(func() { readPort25Record, writePort25Record = savedRead, savedWrite })
	readPort25Record = func() (map[string]string, error) {
		out := map[string]string{}
		for k, v := range record {
			out[k] = v
		}
		return out, nil
	}
	writePort25Record = func(next map[string]string) error {
		record = map[string]string{}
		for k, v := range next {
			record[k] = v
		}
		return nil
	}
	owner := "inet:127.0.0.1:7777"

	// 1. The owner's later filter on port 25 (review finding 3), then
	// OpenDKIM removed.
	t.Run("owner-filter-port25", func(t *testing.T) {
		const id = "postfix.mail-signing-owner-filter-port25"
		fail := func(err error) { h.renderFailure(t, id, "postfix", "owner's later filter on port 25", source, err) }
		dir, run, err := matrixScratchPostfix(t)
		if err != nil {
			fail(err)
			return
		}
		if _, _, _, err := composeAndWriteMilterChain(run, true, rspamdMilter); err != nil {
			fail(err)
			return
		}
		if out, err := run("postconf", "-e", "smtpd_milters="+opendkimMilter+", "+rspamdMilter+", "+owner); err != nil {
			fail(fmt.Errorf("the owner's edit: %s", out))
			return
		}
		if _, _, _, err := composeAndWriteMilterChain(run, true, rspamdMilter); err != nil {
			fail(err)
			return
		}
		entries := port25SMTPDEntries(run)
		if len(entries) == 0 {
			fail(fmt.Errorf("no smtpd receives mail on port 25 in the stock master.cf"))
			return
		}
		want := rspamdMilter + "," + owner
		for _, entry := range entries {
			if got, set, err := masterServiceParam(run, entry, "smtpd_milters"); err != nil || !set || got != want {
				fail(fmt.Errorf("%s smtpd_milters = %q (set %v, %v), want %q: the owner's filter must see port 25 and OpenDKIM must not", entry, got, set, err, want))
				return
			}
		}
		c, err := matrixPostfixFilesCase(t, dir, "mail-signing-owner-filter-port25", "port 25 follows the owner's later main.cf filter, without OpenDKIM", source)
		if err != nil {
			fail(err)
			return
		}
		// OpenDKIM removed: CelikPanel's port-25 setting goes.
		if _, _, _, err := composeAndWriteMilterChain(run, false, rspamdMilter); err != nil {
			fail(err)
			return
		}
		for _, entry := range entries {
			if got, set, err := masterServiceParam(run, entry, "smtpd_milters"); err != nil || set {
				fail(fmt.Errorf("%s smtpd_milters = %q stayed after OpenDKIM was removed (%v)", entry, got, err))
				return
			}
		}
		h.write(t, c)
	})

	// 2. A rollback restores main.cf exactly (review finding 8).
	t.Run("rollback-exact", func(t *testing.T) {
		const id = "postfix.mail-signing-rollback-exact"
		fail := func(err error) { h.renderFailure(t, id, "postfix", "rollback restores defaults", source, err) }
		record = map[string]string{}
		dir, run, err := matrixScratchPostfix(t)
		if err != nil {
			fail(err)
			return
		}
		beforeN, _ := run("postconf", "-n")
		beforeP, _ := run("postconf", "-P")
		_, undo, _, err := composeAndWriteMilterChain(run, true, rspamdMilter)
		if err != nil {
			fail(err)
			return
		}
		if err := undo.undo(run); err != nil {
			fail(err)
			return
		}
		afterN, _ := run("postconf", "-n")
		afterP, _ := run("postconf", "-P")
		if string(beforeN) != string(afterN) || string(beforeP) != string(afterP) {
			fail(fmt.Errorf("after the rollback postconf -n/-P differ from before:\n-n before:\n%s\n-n after:\n%s", beforeN, afterN))
			return
		}
		c, err := matrixPostfixFilesCase(t, dir, "mail-signing-rollback-exact", "main.cf and master.cf after the milter chain was put back", source)
		if err != nil {
			fail(err)
			return
		}
		h.write(t, c)
	})

	// 3. The owner's authorized_submit_users (review finding 2).
	t.Run("owner-submit-users", func(t *testing.T) {
		const id = "postfix.mail-signing-owner-submit-users"
		fail := func(err error) { h.renderFailure(t, id, "postfix", "owner's authorized_submit_users", source, err) }
		record = map[string]string{}
		dir, run, err := matrixScratchPostfix(t)
		if err != nil {
			fail(err)
			return
		}
		if _, _, _, err := composeAndWriteMilterChain(run, true, rspamdMilter); err != nil {
			fail(err)
			return
		}
		for value, want := range map[string]bool{"": true, "static:anyone": true, "root, vmail, www-data": true, passwordlessSubmitUsers: false, "root": false} {
			args := []string{"-X", passwordlessSubmitSetting}
			if value != "" {
				args = []string{"-e", passwordlessSubmitSetting + "=" + value}
			}
			if out, err := run("postconf", args...); err != nil {
				fail(fmt.Errorf("postconf %v: %s", args, out))
				return
			}
			open, _, err := localSubmitOpen(run)
			if err != nil || open != want {
				fail(fmt.Errorf("authorized_submit_users %q: reported open %v (%v), want %v", value, open, err, want))
				return
			}
			state := decideMailSigning(mailSigningInputs{confState: mailSigningBound, submitOpen: open})
			if (state.State == mailSigningSubmitOpen) != want {
				fail(fmt.Errorf("authorized_submit_users %q: state %s", value, state.State))
				return
			}
		}
		if out, err := run("postconf", "-e", passwordlessSubmitSetting+"=static:anyone"); err != nil {
			fail(fmt.Errorf("postconf: %s", out))
			return
		}
		c, err := matrixPostfixFilesCase(t, dir, "mail-signing-owner-submit-users", "the owner's authorized_submit_users (reported as submit_open)", source)
		if err != nil {
			fail(err)
			return
		}
		h.write(t, c)
	})

	// 4. 22 Nov 2026 (second review of alpha.143, B4, B9): the owner's main.cf
	// list names its filters through a variable. With CelikPanel's earlier
	// port-25 value in place it is kept and reported as still in force
	// (port25_variable_own); without one, port 25 is left alone and reported
	// as following the list (port25_variable). And an owner's readable list
	// that spells OpenDKIM's endpoint another way Postfix accepts
	// (inet:[127.0.0.1]:8891) still has OpenDKIM left out of port 25.
	t.Run("variable-list", func(t *testing.T) {
		const id = "postfix.mail-signing-variable-list"
		fail := func(err error) {
			h.renderFailure(t, id, "postfix", "main.cf milter list through a variable", source, err)
		}
		record = map[string]string{}
		dir, run, err := matrixScratchPostfix(t)
		if err != nil {
			fail(err)
			return
		}
		if _, _, _, err := composeAndWriteMilterChain(run, true, rspamdMilter); err != nil {
			fail(err)
			return
		}
		entries := port25SMTPDEntries(run)
		if len(entries) == 0 {
			fail(fmt.Errorf("no smtpd receives mail on port 25 in the stock master.cf"))
			return
		}
		for _, args := range [][]string{
			{"-e", "owner_milters=" + opendkimMilter + ", " + rspamdMilter + ", " + owner},
			{"-e", "smtpd_milters=$owner_milters"},
		} {
			if out, err := run("postconf", args...); err != nil {
				fail(fmt.Errorf("the owner's edit %v: %s", args, out))
				return
			}
		}
		_, _, plan, err := composeAndWriteMilterChain(run, true, rspamdMilter)
		if err != nil {
			fail(err)
			return
		}
		for _, entry := range entries {
			if got, set, err := masterServiceParam(run, entry, "smtpd_milters"); err != nil || !set || got != rspamdMilter {
				fail(fmt.Errorf("%s smtpd_milters = %q (set %v, %v): CelikPanel's earlier value must stay in force", entry, got, set, err))
				return
			}
		}
		if state := decideMailSigning(mailSigningInputs{confState: mailSigningBound, port25: plan}); state.State != mailSigningPort25VariableOwn ||
			!strings.Contains(state.Detail, "still in force") {
			fail(fmt.Errorf("variable list with CelikPanel's value: state %+v", state))
			return
		}
		c, err := matrixPostfixFilesCase(t, dir, "mail-signing-variable-list", "main.cf's milter list through a variable; CelikPanel's earlier port-25 value kept", source)
		if err != nil {
			fail(err)
			return
		}

		// Without a value of CelikPanel's: port 25 is not touched.
		record = map[string]string{}
		_, fresh, err := matrixScratchPostfix(t)
		if err != nil {
			fail(err)
			return
		}
		for _, args := range [][]string{
			{"-e", "owner_milters=" + opendkimMilter + ", " + owner},
			{"-e", "smtpd_milters=$owner_milters"},
		} {
			if out, err := fresh("postconf", args...); err != nil {
				fail(fmt.Errorf("the owner's edit %v: %s", args, out))
				return
			}
		}
		_, _, plan, err = composeAndWriteMilterChain(fresh, true, rspamdMilter)
		if err != nil {
			fail(err)
			return
		}
		for _, entry := range port25SMTPDEntries(fresh) {
			if got, set, err := masterServiceParam(fresh, entry, "smtpd_milters"); err != nil || set {
				fail(fmt.Errorf("%s smtpd_milters = %q was written for a list CelikPanel cannot read (%v)", entry, got, err))
				return
			}
		}
		if state := decideMailSigning(mailSigningInputs{confState: mailSigningBound, port25: plan}); state.State != mailSigningPort25Variable {
			fail(fmt.Errorf("variable list without CelikPanel's value: state %+v", state))
			return
		}

		// Another spelling of OpenDKIM's endpoint in a readable owner's list.
		record = map[string]string{}
		_, spelled, err := matrixScratchPostfix(t)
		if err != nil {
			fail(err)
			return
		}
		if out, err := spelled("postconf", "-e", "smtpd_milters=inet:[127.0.0.1]:8891, "+owner); err != nil {
			fail(fmt.Errorf("the owner's edit: %s", out))
			return
		}
		if _, _, _, err := composeAndWriteMilterChain(spelled, true, rspamdMilter); err != nil {
			fail(err)
			return
		}
		for _, entry := range port25SMTPDEntries(spelled) {
			if got, set, err := masterServiceParam(spelled, entry, "smtpd_milters"); err != nil || !set || got != owner {
				fail(fmt.Errorf("%s smtpd_milters = %q (set %v, %v), want %q: OpenDKIM spelled inet:[127.0.0.1]:8891 must be left out", entry, got, set, err, owner))
				return
			}
		}
		h.write(t, c)
	})
}

// renderMatrixOpenDKIMDecisions runs the OpenDKIM decisions with the release's
// real `opendkim -n`: the interrupted restart (review finding 4) and an
// owner's file on an OpenDKIM that refuses the login setting (finding 6).
// systemctl is recorded, not run.
func renderMatrixOpenDKIMDecisions(t *testing.T, h *matrixHost, layout opendkimLayout, supported bool) error {
	dir := t.TempDir()
	saved := []any{opendkimConfRead, opendkimConfWrite, opendkimProbeDir, opendkimRestartOwedPath}
	t.Cleanup(func() {
		opendkimConfRead = saved[0].(func(string) ([]byte, error))
		opendkimConfWrite = saved[1].(func(string, []byte, os.FileMode) error)
		opendkimProbeDir = saved[2].(func() string)
		opendkimRestartOwedPath = saved[3].(string)
	})
	opendkimConfRead, opendkimConfWrite = os.ReadFile, os.WriteFile
	opendkimProbeDir = func() string { return dir }
	opendkimRestartOwedPath = filepath.Join(dir, "restart-owed")
	scratch := layout
	scratch.confPath = filepath.Join(dir, "opendkim.conf")
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		switch {
		case name == "opendkim":
			return exec.Command(name, args...).CombinedOutput()
		case call == "systemctl is-active opendkim":
			return []byte("active\n"), nil
		case call == "postconf -h milter_default_action":
			return []byte("accept\n"), nil
		}
		return nil, nil
	}
	// The interrupted restart: the file is current, the restart is owed.
	if err := os.WriteFile(scratch.confPath, desiredOpenDKIMConf(scratch, supported), 0o644); err != nil {
		return err
	}
	if err := markOpenDKIMRestartOwed(scratch.confPath); err != nil {
		return err
	}
	state, _ := reconcileOpenDKIMConf(run, scratch)
	restarted := false
	for _, call := range calls {
		restarted = restarted || call == "systemctl restart opendkim"
	}
	if !restarted || openDKIMRestartIsOwed() || (state != mailSigningBound && state != mailSigningUnsupported) {
		return fmt.Errorf("interrupted restart: state %s, restarted %v, still owed %v", state, restarted, openDKIMRestartIsOwed())
	}
	// An owner's file: on a build without SenderMacro nothing with the line is
	// put beside it and the line is never advised.
	owner := renderOpenDKIMConfWithSocket(scratch, opendkimSocket) + "SignatureAlgorithm\trsa-sha256\n"
	if err := os.WriteFile(scratch.confPath, []byte(owner), 0o644); err != nil {
		return err
	}
	state, note := reconcileOpenDKIMConf(run, scratch)
	pending, pendingErr := os.ReadFile(scratch.confPath + opendkimPendingSuffix)
	if supported {
		if state != mailSigningConfKept || pendingErr != nil || string(pending) != renderOpenDKIMConf(scratch) {
			return fmt.Errorf("owner's file with SenderMacro known: state %s, pending %v", state, pendingErr)
		}
	} else if state != mailSigningUnsupported || pendingErr == nil || strings.Contains(note, "adds the line") {
		return fmt.Errorf("owner's file on an OpenDKIM that refuses SenderMacro: state %s, pending present %v, note %q", state, pendingErr == nil, note)
	}
	return nil
}

// matrixAutoresponderReplyFrom runs the product's own address rule for the
// out-of-office reply (review finding 1): with another account's address
// forwarded to the mailbox and an alias of the mailbox's own domain, the
// former is refused, so the reply's From: is one the account holds.
func matrixAutoresponderReplyFrom() (string, error) {
	forwardings := []transport.MailForwarding{
		{Source: "ceo@b.matrix.test", Destination: "anna@a.matrix.test"},
		{Source: "info@a.matrix.test", Destination: "anna@a.matrix.test"},
	}
	foreign := transport.ForeignAutoresponderAddresses("anna@a.matrix.test", []string{"ceo@b.matrix.test", "info@a.matrix.test"},
		nil, forwardings, func(string) bool { return false })
	if len(foreign) != 1 || foreign[0] != "ceo@b.matrix.test" {
		return "", fmt.Errorf("another account's address was not refused: %v", foreign)
	}
	return "Anna <info@a.matrix.test>", nil
}
