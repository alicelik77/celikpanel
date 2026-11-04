package main

import (
	"errors"
	"time"

	sqlite "modernc.org/sqlite"
)

// The panel's own database was briefly busy (alpha.123 lab, Ubuntu 22.04,
// 2026-11-04).
//
// Setup step 01-dns failed 19 s after start with "database is locked (5)
// (SQLITE_BUSY)". Nothing had changed on the server: bind9 was never
// installed and the Agent had no DNS operation. The Panel's own write
// transaction (the DNS operation's record, begun DEFERRED: it read first and
// wrote after) met another panel writer and SQLite answered BUSY at once,
// without the pool's busy_timeout. The generic step branch then showed it as
// a verified DNS failure (server_setup_dns_failed) and released the draft.
//
// The DNS transactions now take the write lock first (beginDNSWriteTx). Any
// remaining busy or locked answer of the Panel's own database is a wait for
// every step, not a failure: the runner asks the same step again with a
// growing pause (2, 4, 8, 16, then 20 s). Every step re-enters through its
// saved request identity (the DNS step reads its saved switch, a component
// its saved operation), so a retry never starts a second Agent mutation. Only
// when the database stays busy for serverSetupPanelBusyAttempts passes
// (about ten minutes) does the step stop, with its own reason and the owner's
// next action, never the DNS guidance.
//
// Panelin kendi veritabanı kısa süre meşgulse adım başarısız sayılmaz; aynı
// adım kayıtlı işlemiyle, artan aralıklarla yeniden denenir.

const (
	serverSetupPanelBusyCode       = "server_setup_panel_busy"
	serverSetupPanelStayedBusyCode = "server_setup_panel_stayed_busy"
	serverSetupPanelBusyMessage    = "CelikPanel's own database was briefly busy with another task, so this step could not save its progress just now. This is not a problem with the server or with the software this step sets up. Setup tries the same step again by itself in a few seconds, using its saved operation, so nothing is started twice. Nobody needs to act."
	serverSetupPanelStayedBusyText = "CelikPanel's own database stayed busy for about ten minutes, so setup stopped this step instead of guessing its result. You, the owner: run `sudo systemctl restart celikpanel-panel` on this server (it restarts only the panel; websites, mail and DNS keep running), then choose Review a revised plan; completed steps are kept. If it happens again, `sudo journalctl -u celikpanel-panel -n 100` shows what kept the database busy."
)

// serverSetupPanelBusyAttempts bounds the consecutive busy passes of one
// step (2+4+8+16 s, then 20 s each: about ten minutes).
var serverSetupPanelBusyAttempts = 32

var errServerSetupPanelStayedBusy = errors.New("the panel database stayed busy")

// panelDatabaseBusy reports whether err is the Panel's own SQLite database
// answering busy or locked (SQLITE_BUSY / SQLITE_LOCKED, any extended code).
// Only the driver's own error type counts, so an Agent's or a remote
// server's message that merely mentions SQLITE_BUSY is not taken for it.
func panelDatabaseBusy(err error) bool {
	var coded *sqlite.Error
	if err == nil || !errors.As(err, &coded) {
		return false
	}
	switch coded.Code() & 0xff {
	case 5, 6: // SQLITE_BUSY, SQLITE_LOCKED
		return true
	}
	return false
}

// serverSetupPanelBusyWait records one more busy pass of the current step,
// or reports the bounded failure once the step has met it too often.
func serverSetupPanelBusyWait(execution *serverSetupExecution) bool {
	// A new episode starts whenever the run's last state was another one.
	if execution.Error == nil || execution.Error.Code != serverSetupPanelBusyCode {
		execution.PanelBusyAttempts = 0
	}
	execution.PanelBusyAttempts++
	if execution.PanelBusyAttempts > serverSetupPanelBusyAttempts {
		return false
	}
	execution.Status = "waiting"
	execution.Error = &serviceOperationError{Code: serverSetupPanelBusyCode, Message: serverSetupPanelBusyMessage}
	return true
}

// serverSetupPanelBusyDelay is the pause before the next pass of a run that
// waits for the Panel's database: 2, 4, 8, 16, then the usual 20 s.
func serverSetupPanelBusyDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 5 {
		return serverSetupWaitingInterval
	}
	delay := 2 * time.Second << (attempts - 1)
	if delay > serverSetupWaitingInterval {
		return serverSetupWaitingInterval
	}
	return delay
}

// serverSetupRunnerPause is the runner's pause after a pass that did not
// progress.
func serverSetupRunnerPause(execution *serverSetupExecution) time.Duration {
	if execution.Status != "waiting" {
		return 2 * time.Second
	}
	if execution.Error != nil && execution.Error.Code == serverSetupPanelBusyCode {
		return serverSetupPanelBusyDelay(execution.PanelBusyAttempts)
	}
	return serverSetupWaitingInterval
}

// serverSetupRunnerBusyPasses bounds how often the runner itself retries a
// pass whose own bookkeeping met a busy database (reading the run, or saving
// its state); the next panel start resumes the run after that.
const serverSetupRunnerBusyPasses = 8
