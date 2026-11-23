package main

import (
	"context"
	"crypto/md5" //nolint:gosec // dpkg records conffiles by MD5; this only compares with it.
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Agent.CloseSideDoorsV1 (S4 R1, 20 Nov 2026), run by the Panel once after
// each start, in its own job:
//
//  1. The database tools and webmail move to their own accounts
//     (php_tool_pool.go). A tool still in the stock pool is moved by the same
//     verified step the Manage window uses (bindPHPToolWith): own pool, nginx
//     validated and reloaded, the tool must answer as itself, otherwise the
//     previous configuration, the pool and the files' owners come back. A
//     failed move is recorded and not tried again on its own; choosing the
//     tool's PHP version in its Manage window tries it. A configuration the
//     owner changed is not touched (D-022).
//  2. The distribution's stock pool (www.conf) of each PHP version is turned
//     off when it is still the package's untouched file (dpkg's own MD5 of
//     the conffile), no web server configuration that is loaded names its
//     socket (any line outside a full-line comment, a unix: path that reaches
//     the same socket or cannot be followed, or a *_pass whose address is a
//     variable, keeps it on: stockPoolUsers; an Apache file only counts when
//     Apache is installed and loads it), and another pool remains (PHP-FPM
//     refuses to start without one). Turning off is a
//     rename to www.conf.off-by-celikpanel (dpkg keeps a removed conffile
//     removed on upgrades), checked with php-fpm -t and a reload, and put
//     back when either refuses. An owner-changed or used pool is kept and
//     listed.
//  3. `at` is closed to site accounts and their pending at jobs are listed;
//     site crontabs with lines not made in CelikPanel are counted
//     (side_doors_jobs.go, 21 Nov 2026).
//
// Panel her başlangıçtan sonra bir kez çalıştırır: araçları kendi hesaplarına
// doğrulanan adımla taşır, varsayılan havuzu dokunulmamış ve kullanılmıyorsa
// kapatır; sahibin değiştirdiği ya da kullanılan havuz korunur ve listelenir.

const stockPoolOffSuffix = services.StockPoolOffSuffix

// CloseSideDoorsV1 is the step; it holds the job's mutation step.
//
// With ReClose set it is the administrator's explicit "close it again" and
// does exactly what its confirmation says (S4 R1, second fix round, B4): only
// /etc/at.allow, or only that PHP version's stock pool, decided and changed
// as at a first start; no tool is moved, no other version is touched, no
// crontab is read. The owner's undo stays recorded until the door is closed
// again, so an attempt that is cut or refused never makes a later start
// close it by itself.
func (a *Agent) CloseSideDoorsV1(req *transport.SideDoorsRequest, resp *transport.SideDoorsResponse) error {
	*resp = transport.SideDoorsResponse{}
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	switch req.ReClose {
	case "", transport.SideDoorsReCloseAt:
	case transport.SideDoorsReCloseStockPool:
		if !phpVersionText.MatchString(req.Version) {
			resp.Error = fmt.Sprintf("invalid PHP version %q", req.Version)
			return nil
		}
	default:
		resp.Error = fmt.Sprintf("unknown side door %q", req.ReClose)
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepCloseSideDoors, sideDoorsMutationTarget, "", "close"),
	)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	defer finishStep()
	sideDoorsMu.Lock()
	defer sideDoorsMu.Unlock()
	state := transport.SideDoorsState{CheckedAt: sideDoorsNow()}
	switch req.ReClose {
	case transport.SideDoorsReCloseAt:
		state.Scope = transport.SideDoorsScopeReClose
		state.At = closeSiteAtFor(ctx, hostSiteAtOps(), atPass{reclose: true})
	case transport.SideDoorsReCloseStockPool:
		state.Scope = transport.SideDoorsScopeReClose
		state.StockPools = closeStockFPMPoolsFor(ctx, hostStockPoolOps(), stockPoolPass{only: req.Version, reclose: true})
	default:
		state.Tools = closeSideDoorsTools(ctx, hostPHPToolBindOps(), hostSideDoorsToolHooks(), afterPHPToolBound)
		state.StockPools = closeStockFPMPoolsWith(ctx, hostStockPoolOps())
		// 3 and 4 (21 Nov 2026, side_doors_jobs.go): at closed to site
		// accounts; site crontabs with lines not made in CelikPanel counted.
		state.At = closeSiteAtWith(ctx, hostSiteAtOps())
		state.SiteCrontabs = checkSiteCrontabs()
	}
	resp.State = &state
	return nil
}

// ReadSideDoorsV1 reads the side doors' states again without changing
// anything (S4 R1, second fix round, B7): the Panel asks after every move in
// a tool's Manage window and in its hourly round, so a tool that stopped
// answering has its Problems line without a Panel restart, and a line whose
// cause is gone goes by itself. Tools are asked whether they answer as they
// are configured; the stock pools and at say what can be read (off, the
// owner's undo, CelikPanel's file); site crontabs are counted (no record is
// written). A state only the step decides comes back empty and the Panel
// keeps what the step said.
//
// S4 R1, third fix round: the read takes the step's lock (sideDoorsMu, also
// held by a Manage-window move), so it never runs beside them; while one
// runs it answers that it waited in vain and changes nothing. Its time is
// taken once it holds the lock, and the Panel never lets an answer replace a
// newer one. A tool is said to be down only after two probes in a row failed.
func (a *Agent) ReadSideDoorsV1(_ *transport.SideDoorsReadRequest, resp *transport.SideDoorsResponse) error {
	*resp = transport.SideDoorsResponse{}
	if !lockSideDoorsWithin(sideDoorsReadWait) {
		resp.Error = "the side doors step or a tool's move is running; its own answer brings the states"
		return nil
	}
	defer sideDoorsMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	state := transport.SideDoorsState{CheckedAt: sideDoorsNow(), Scope: transport.SideDoorsScopeRead}
	state.Tools = readSideDoorsTools(ctx, hostPHPToolBindOps(), hostSideDoorsToolHooks())
	state.StockPools = closeStockFPMPoolsFor(ctx, hostStockPoolOps(), stockPoolPass{readOnly: true})
	state.At = closeSiteAtFor(ctx, hostSiteAtOps(), atPass{readOnly: true})
	state.SiteCrontabs = checkSiteCrontabsFor(true)
	resp.State = &state
	return nil
}

// sideDoorsMu is held by the side doors step, a Manage-window move and the
// read (S4 R1, third fix round).
var sideDoorsMu sync.Mutex

var (
	// sideDoorsReadWait bounds how long a read waits for the step or a move.
	sideDoorsReadWait = 20 * time.Second
	// sideDoorsProbeAgain is the pause before a read asks a tool a second
	// time.
	sideDoorsProbeAgain = 5 * time.Second
)

// lockSideDoorsWithin takes sideDoorsMu unless it stays held for wait.
func lockSideDoorsWithin(wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		if sideDoorsMu.TryLock() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// sideDoorsNow is an answer's time, to the millisecond, so the Panel can
// tell which of two answers is newer.
func sideDoorsNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

// probeTwice asks a tool, and once more after a pause when it did not
// answer: one slow answer is not "down". A read whose time ran out is not
// known (known false), never "down".
func probeTwice(ctx context.Context, probe func(context.Context, string, string) error, tool, version string) (known bool, err error) {
	if err = probe(ctx, tool, version); err == nil {
		return true, nil
	}
	select {
	case <-ctx.Done():
		return false, err
	case <-time.After(sideDoorsProbeAgain):
	}
	err = probe(ctx, tool, version)
	if err != nil && ctx.Err() != nil {
		return false, err
	}
	return true, err
}

// readSideDoorsTools is the read-only pass over the tools: nothing is moved,
// reloaded or rechecked; each tool is asked whether it answers as it is
// configured now. A tool whose answer could not be had in the read's time
// comes back without a state (the Panel keeps what it knew).
func readSideDoorsTools(ctx context.Context, ops phpToolBindOps, hooks sideDoorsToolHooks) []transport.SideDoorsTool {
	tools := []transport.SideDoorsTool{}
	failures := readPHPToolAccountFailures()
	for _, state := range phpToolStates() {
		if !state.Installed || state.Socket == "" {
			continue
		}
		row := transport.SideDoorsTool{Tool: state.Tool, Version: state.version()}
		if _, own := phpToolOwnSocket(state.Socket); own {
			row.State, row.Account = transport.SideDoorsToolOwnAccount, phpToolAccount(state.Tool)
			if user := hooks.poolUser(row.Tool, row.Version); user != "" && user != row.Account {
				if user == hooks.webAccount() {
					row.State, row.Account = transport.SideDoorsToolOwnerConfig, user
					row.Path = phpToolPoolPath(row.Tool, row.Version)
					row.Detail = fmt.Sprintf("%s was changed outside CelikPanel and runs as %s, the web server's account", row.Path, user)
					tools = append(tools, row)
					continue
				}
				row.Account = user
			}
			if known, err := probeTwice(ctx, ops.httpProbe, row.Tool, row.Version); !known {
				row.State, row.Detail = "", "its answer could not be read in time"
			} else if err != nil {
				row.State, row.Detail = transport.SideDoorsToolDown, "it does not answer on its own pool: "+err.Error()
			}
			if note := hooks.note(row.Tool); note != "" {
				row.Detail = strings.TrimPrefix(row.Detail+"; "+note, "; ")
			}
			tools = append(tools, row)
			continue
		}
		switch {
		case ops.family() != "apt":
			row.State = transport.SideDoorsToolUnsupported
		case row.Version == "" || !phpVersionInstalled(row.Version):
			row.State, row.Detail = transport.SideDoorsToolWaiting, "the PHP version it runs on is not installed"
		case refuseOwnerEditedConfig(phpToolConfPathFor(state.Tool)) != nil:
			row.State, row.Detail = transport.SideDoorsToolOwnerConfig, refuseOwnerEditedConfig(phpToolConfPathFor(state.Tool)).Error()
		default:
			row.State = transport.SideDoorsToolWaiting
			row.Detail = "it is moved to its own account at the panel's next start"
			if failure, failed := failures[state.Tool]; failed && failure.PHPVersion == row.Version {
				row.State, row.Detail = transport.SideDoorsToolFailed, failure.Detail
			}
			if known, err := probeTwice(ctx, ops.httpProbe, row.Tool, row.Version); known && err != nil {
				row.State = transport.SideDoorsToolDown
				row.Detail = strings.TrimPrefix(row.Detail+"; it does not answer as it is now: "+err.Error(), "; ")
			}
		}
		tools = append(tools, row)
	}
	return tools
}

const sideDoorsMutationTarget = "side-doors"

// sideDoorsToolHooks are what the step does for a tool already on its own
// pool; production fills them with host operations, tests with recorders.
type sideDoorsToolHooks struct {
	// poolUser reads the user line of the tool's own pool on a version
	// ("" when it cannot be read).
	poolUser func(tool, version string) string
	// webAccount is the web server's account.
	webAccount func() string
	// recheck puts right what a package upgrade gave back to the web group
	// (phpMyAdmin's config-db.php and secret), forgets the record of an
	// earlier move (proven now) and removes the tool's pools left on other
	// versions by an interrupted move.
	recheck func(ctx context.Context, tool, version string) error
	// treeNote says when what lets the tool's own account into its files is
	// gone (webmail: the access-control-list entry on its tree's root, B2);
	// "" when it is there. nil in tests that do not look at it.
	treeNote func(tool string) string
}

func (h sideDoorsToolHooks) note(tool string) string {
	if h.treeNote == nil {
		return ""
	}
	return h.treeNote(tool)
}

// closeSideDoorsTools moves each installed tool still in a stock pool to its
// own account, and checks each tool already on its own pool.
//
// What decides is what serves the tool now: the socket its web server
// configuration names. An interrupted move recovers at the next start:
//   - configuration still on the stock socket (cut before the web server was
//     switched): the move runs again; a failure puts back what the FIRST
//     attempt recorded of the files (php_tool_files.go), so the stock pool
//     reads them again;
//   - configuration on the own socket (cut after it was written): the tool
//     is asked; when it does not answer, the web server's configuration is
//     checked and reloaded once (a reload the cut never made) and it is asked
//     again.
//
// A recorded failure is not tried again by itself while the tool answers;
// a tool that does not answer is tried again, and when it still does not
// answer it is reported as down (Problems now says who acts).
//
// after runs what follows a verified move (webmail's own addresses follow its
// socket); nil in tests.
func closeSideDoorsTools(ctx context.Context, ops phpToolBindOps, hooks sideDoorsToolHooks, after func(context.Context, string) string) []transport.SideDoorsTool {
	tools := []transport.SideDoorsTool{}
	failures := readPHPToolAccountFailures()
	for _, state := range phpToolStates() {
		if !state.Installed || state.Socket == "" {
			continue
		}
		row := transport.SideDoorsTool{Tool: state.Tool, Version: state.version()}
		if _, own := phpToolOwnSocket(state.Socket); own {
			tools = append(tools, sideDoorsOwnTool(ctx, ops, hooks, row))
			continue
		}
		switch {
		case ops.family() != "apt":
			row.State = transport.SideDoorsToolUnsupported
		case row.Version == "" || !phpVersionInstalled(row.Version):
			row.State, row.Detail = transport.SideDoorsToolWaiting, "the PHP version it runs on is not installed"
		default:
			if failure, failed := failures[state.Tool]; failed && failure.PHPVersion == row.Version {
				if ops.httpProbe(ctx, state.Tool, row.Version) == nil {
					row.State, row.Detail = transport.SideDoorsToolFailed, failure.Detail
					break
				}
				// It does not answer as it is: trying again cannot make it
				// worse, and the move puts back what it cannot finish.
			}
			if err := refuseOwnerEditedConfig(phpToolConfPathFor(state.Tool)); err != nil {
				row.State, row.Detail = transport.SideDoorsToolOwnerConfig, err.Error()
				break
			}
			_, err := bindPHPToolWith(ctx, ops, state.Tool, row.Version)
			if err == nil {
				row.State, row.Account = transport.SideDoorsToolMoved, phpToolAccount(state.Tool)
				if after != nil {
					row.Detail = after(ctx, state.Tool)
				}
				break
			}
			row.Detail = err.Error()
			var typed *phpToolBindError
			if errors.As(err, &typed) && sideDoorsRetryLater(typed.Code) {
				row.State = transport.SideDoorsToolWaiting
				break
			}
			row.State = transport.SideDoorsToolFailed
			if recordErr := writePHPToolAccountFailure(state.Tool, row.Version, err.Error()); recordErr != nil {
				row.Detail += "; the failure could not be recorded: " + recordErr.Error()
			}
			if probeErr := ops.httpProbe(ctx, state.Tool, row.Version); probeErr != nil {
				row.State = transport.SideDoorsToolDown
				row.Detail += "; it does not answer as it is now: " + probeErr.Error()
			}
		}
		tools = append(tools, row)
	}
	return tools
}

// sideDoorsOwnTool checks a tool whose configuration names its own pool.
func sideDoorsOwnTool(ctx context.Context, ops phpToolBindOps, hooks sideDoorsToolHooks, row transport.SideDoorsTool) transport.SideDoorsTool {
	account := phpToolAccount(row.Tool)
	row.State, row.Account = transport.SideDoorsToolOwnAccount, account
	// The pool's own user line says whose account it runs as; an owner's
	// pool that runs as the web server's account is said as such.
	if user := hooks.poolUser(row.Tool, row.Version); user != "" && user != account {
		if user == hooks.webAccount() {
			row.State, row.Account = transport.SideDoorsToolOwnerConfig, user
			row.Path = phpToolPoolPath(row.Tool, row.Version)
			row.Detail = fmt.Sprintf("%s was changed outside CelikPanel and runs as %s, the web server's account", row.Path, user)
			return row
		}
		row.Account = user
	}
	var notes []string
	if err := hooks.recheck(ctx, row.Tool, row.Version); err != nil {
		notes = append(notes, err.Error())
	}
	// The owner's removal of the entry stands (it is not put back here); the
	// line says what it means.
	if note := hooks.note(row.Tool); note != "" {
		notes = append(notes, note)
	}
	if err := ops.httpProbe(ctx, row.Tool, row.Version); err != nil {
		// A move cut between writing the web server's configuration and
		// reloading it leaves the running web server on the old socket.
		retry := ops.nginxTest(ctx)
		if retry == nil {
			retry = ops.nginxReload(ctx)
		}
		if retry == nil {
			retry = retryPHPToolProbe(ctx, row.Tool, row.Version, ops.httpProbe, phpToolProbeSettle, 2*time.Second)
		}
		if retry != nil {
			row.State = transport.SideDoorsToolDown
			notes = append(notes, "it does not answer on its own pool: "+retry.Error())
		} else {
			notes = append(notes, "the web server's configuration was reloaded and it answers again")
		}
	}
	row.Detail = strings.Join(notes, "; ")
	return row
}

func hostSideDoorsToolHooks() sideDoorsToolHooks {
	return sideDoorsToolHooks{
		poolUser: func(tool, version string) string {
			data, err := os.ReadFile(phpToolPoolPath(tool, version))
			if err != nil {
				return ""
			}
			if match := phpToolPoolUserLine.FindSubmatch(data); match != nil {
				return string(match[1])
			}
			return ""
		},
		webAccount: webServerGroup,
		treeNote: func(tool string) string {
			if tool != phpToolWebmail {
				return ""
			}
			return webmailRootEntryNote()
		},
		recheck: func(ctx context.Context, tool, version string) error {
			var errs []error
			if err := clearPHPToolFileJournal(tool); err != nil {
				errs = append(errs, err)
			}
			if tool == phpToolPHPMyAdmin {
				if uid, gid, found, err := lookupSystemAccount(phpToolAccount(tool)); err == nil && found {
					if _, err := preparePHPMyAdminOwnAccount(uid, gid, true); err != nil {
						errs = append(errs, err)
					}
				}
			}
			if err := retirePHPToolPoolsWith(ctx, hostPHPToolPoolOps(), tool, version); err != nil {
				errs = append(errs, err)
			}
			return errors.Join(errs...)
		},
	}
}

var phpToolPoolUserLine = regexp.MustCompile(`(?m)^\s*user\s*=\s*(\S+)\s*$`)

// sideDoorsRetryLater: a refusal that changed nothing and says a
// prerequisite is missing is tried again at the next start.
func sideDoorsRetryLater(code string) bool {
	switch code {
	case errCodePHPVersionNotInstalled, errCodePHPVersionNotRunning, errCodePHPToolNotInstalled, errCodePHPToolExtensionsFailed:
		return true
	}
	return false
}

// --- the stock pool ---

type stockPoolOps struct {
	versions func() []string
	// turnedOff lists the versions CelikPanel turned off (its record);
	// record writes it. turning lists the versions whose turn-off began and
	// did not finish, pending marks one before its rename and forgetPending
	// ends the mark alone (nil in tests that do not look at them).
	turnedOff     func() (map[string]bool, error)
	turning       func() (map[string]bool, error)
	record        func(version string, off bool) error
	pending       func(version string) error
	forgetPending func(version string) error
	// lock is the host-wide lock of PHP-FPM pool files
	// (services.LockPHPFPMPools); nil in tests.
	lock        func() func()
	readFile    func(path string) ([]byte, error)
	conffileMD5 func(ctx context.Context, version, path string) (string, bool)
	nginxDump   func(ctx context.Context) (string, error)
	// apacheFiles lists what besides nginx may hand PHP to a stock pool:
	// the Apache files Apache loads and every other web server CelikPanel
	// does not read (webServerUsers); sameSocket answers, for the pool's socket, whether a
	// unix: path reaches it (nil in tests that compare text only).
	apacheFiles func() []stockPoolConfigText
	sameSocket  func(listen string) socketMatcher
	aliasFor    func(listen string) string
	rename      func(from, to string) error
	fpmTest     func(ctx context.Context, version string) error
	fpmActive   func(version string) bool
	fpmReload   func(ctx context.Context, version string) error
}

func (ops stockPoolOps) hold() func() {
	if ops.lock == nil {
		return func() {}
	}
	return ops.lock()
}

func (ops stockPoolOps) turningOff() (map[string]bool, error) {
	if ops.turning == nil {
		return map[string]bool{}, nil
	}
	return ops.turning()
}

// stockPoolPass says what one pass over the stock pools may do.
type stockPoolPass struct {
	// only: this version alone ("" every version).
	only string
	// reclose: the administrator's explicit "turn it off again" for only;
	// the owner's undo is passed over for it this once, and stays recorded
	// until the pool is off again (B4: a cut or refused attempt never makes
	// a later start turn it off by itself).
	reclose bool
	// readOnly: read again what can be read without changing anything (the
	// hourly round and after a Manage-window move, B7). A version whose state
	// only the step decides comes back with State "" (the Panel keeps what
	// the step said).
	readOnly bool
}

var stockPoolListen = regexp.MustCompile(`(?m)^\s*listen\s*=\s*(\S+)\s*$`)

func stockPoolPath(version string) string {
	return filepath.Join(phpEtcDir, version, "fpm", "pool.d", "www.conf")
}

// closeStockFPMPoolsWith decides and, where allowed, turns off each
// version's stock pool.
func closeStockFPMPoolsWith(ctx context.Context, ops stockPoolOps) []transport.SideDoorsStockPool {
	return closeStockFPMPoolsFor(ctx, ops, stockPoolPass{})
}

func closeStockFPMPoolsFor(ctx context.Context, ops stockPoolOps, pass stockPoolPass) []transport.SideDoorsStockPool {
	pools := []transport.SideDoorsStockPool{}
	var dump string
	var dumpErr error
	dumped := false
	for _, version := range ops.versions() {
		if pass.only != "" && version != pass.only {
			continue
		}
		path := stockPoolPath(version)
		row := transport.SideDoorsStockPool{Version: version, Path: path}
		if pass.readOnly {
			row.State, row.Detail = readStockPool(ops, version, path)
			if row.State != stockPoolGone {
				pools = append(pools, row)
			}
			continue
		}
		// The decision and the change run under the host-wide pool lock, so
		// a site's last pool cannot go between "another pool remains" and the
		// rename (B6).
		release := ops.hold()
		state, detail, decide := finishStockPoolStep(ctx, ops, version, path)
		if !decide {
			row.State, row.Detail = state, detail
			release()
			if row.State != stockPoolGone {
				pools = append(pools, row)
			}
			continue
		}
		turnedOff, recordErr := ops.turnedOff()
		content, err := ops.readFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if _, offErr := ops.readFile(path + stockPoolOffSuffix); offErr == nil {
					row.State = transport.SideDoorsStockOff
					if recordErr == nil && !turnedOff[version] {
						// Renamed under CelikPanel's name without its record (a
						// build before the "turning off" mark): the version's
						// configuration test and reload run before it is
						// recorded off, and a refusal puts it back (S4 R1,
						// third fix round).
						row.State, row.Detail = checkAndReloadAfterRename(ctx, ops, version, path, false)
						if row.State == transport.SideDoorsStockTurnedOff && row.Detail == "" {
							row.State, row.Detail = transport.SideDoorsStockOff, "found renamed without CelikPanel's record; checked and remembered now"
						}
					}
					release()
					pools = append(pools, row)
					continue
				}
				release()
				continue
			}
			release()
			row.State, row.Detail = transport.SideDoorsStockUnknown, "it could not be read: "+err.Error()
			pools = append(pools, row)
			continue
		}
		// The owner's undo stands (D-022): CelikPanel turned it off and it is
		// back under its own name, so the server owner put it back. It is left
		// on; only the administrator's explicit "turn it off again" passes
		// over it, for this version, once.
		if turnedOff[version] && !(pass.reclose && version == pass.only) {
			if _, offErr := ops.readFile(path + stockPoolOffSuffix); offErr != nil {
				release()
				row.State = transport.SideDoorsStockOwnerRestored
				pools = append(pools, row)
				continue
			}
		}
		if recordErr != nil {
			release()
			row.State, row.Detail = transport.SideDoorsStockUnknown, "CelikPanel's record of the pools it turned off could not be read: "+recordErr.Error()
			pools = append(pools, row)
			continue
		}
		sum := md5.Sum(content) //nolint:gosec
		packaged, known := ops.conffileMD5(ctx, version, path)
		switch {
		case !known:
			release()
			row.State, row.Detail = transport.SideDoorsStockUnknown, "the package's own copy of this file could not be read (dpkg)"
			pools = append(pools, row)
			continue
		case packaged != hex.EncodeToString(sum[:]):
			release()
			row.State = transport.SideDoorsStockOwnerChanged
			pools = append(pools, row)
			continue
		}
		if !dumped {
			dump, dumpErr = ops.nginxDump(ctx)
			dumped = true
		}
		if dumpErr != nil {
			release()
			row.State, row.Detail = transport.SideDoorsStockUnknown, "nginx -T did not answer: "+dumpErr.Error()
			pools = append(pools, row)
			continue
		}
		listen := ""
		if match := stockPoolListen.FindSubmatch(content); match != nil {
			listen = string(match[1])
		}
		var apache []stockPoolConfigText
		if ops.apacheFiles != nil {
			apache = ops.apacheFiles()
		}
		var same socketMatcher
		if ops.sameSocket != nil {
			same = ops.sameSocket(listen)
		}
		row.UsedBy = stockPoolUsers(dump, listen, ops.aliasFor(listen), apache, same)
		if len(row.UsedBy) > 0 {
			release()
			row.State = transport.SideDoorsStockInUse
			pools = append(pools, row)
			continue
		}
		if !otherPoolsBeside(path) {
			release()
			row.State = transport.SideDoorsStockOnlyPool
			pools = append(pools, row)
			continue
		}
		row.State, row.Detail = turnOffStockPool(ctx, ops, version, path, turnedOff[version])
		release()
		pools = append(pools, row)
	}
	return pools
}

// stockPoolGone: neither www.conf nor CelikPanel's renamed copy is there (a
// version without a stock pool); no row.
const stockPoolGone = "gone"

// finishStockPoolStep looks for CelikPanel's own unfinished turn-off of a
// version (marked before its rename, B5) and finishes or forgets it; decide is
// true when the ordinary decision is to follow. Called under the pool lock.
//   - www.conf still there and no renamed copy: the rename never happened;
//     the mark goes and the version is decided as usual (a version also
//     recorded off keeps reading as the owner's undo: the administrator's
//     "turn it off again" was cut before it changed anything).
//   - renamed and www.conf gone: the rename happened and the check or reload
//     may not have; they run now, and a refusal puts it back.
func finishStockPoolStep(ctx context.Context, ops stockPoolOps, version, path string) (string, string, bool) {
	turning, err := ops.turningOff()
	if err != nil || !turning[version] {
		return "", "", true
	}
	_, stockErr := ops.readFile(path)
	_, offErr := ops.readFile(path + stockPoolOffSuffix)
	switch {
	case stockErr == nil && errors.Is(offErr, os.ErrNotExist):
		if ops.forgetPending != nil {
			if err := ops.forgetPending(version); err != nil {
				return transport.SideDoorsStockUnknown, "CelikPanel's unfinished step on it could not be cleared from its record: " + err.Error(), false
			}
		}
		return "", "", true
	case errors.Is(stockErr, os.ErrNotExist) && offErr == nil:
		turnedOff, _ := ops.turnedOff()
		state, detail := checkAndReloadAfterRename(ctx, ops, version, path, turnedOff[version])
		if detail == "" {
			detail = "CelikPanel's earlier step on it was cut after the rename; it was finished now"
		}
		return state, detail, false
	case errors.Is(stockErr, os.ErrNotExist) && errors.Is(offErr, os.ErrNotExist):
		return stockPoolGone, "", false
	}
	return transport.SideDoorsStockUnknown, fmt.Sprintf("CelikPanel's step on it did not finish and %s and %s%s are both there (or could not be read); the server administrator keeps one of them", path, path, stockPoolOffSuffix), false
}

// readStockPool is the read-only pass: off, the owner's undo, a pool that
// could not be read, or "" (only the step decides the rest).
func readStockPool(ops stockPoolOps, version, path string) (string, string) {
	turnedOff, recordErr := ops.turnedOff()
	_, stockErr := ops.readFile(path)
	_, offErr := ops.readFile(path + stockPoolOffSuffix)
	switch {
	case errors.Is(stockErr, os.ErrNotExist) && offErr == nil:
		return transport.SideDoorsStockOff, ""
	case errors.Is(stockErr, os.ErrNotExist):
		return stockPoolGone, ""
	case stockErr != nil:
		return transport.SideDoorsStockUnknown, "it could not be read: " + stockErr.Error()
	case recordErr != nil:
		return transport.SideDoorsStockUnknown, "CelikPanel's record of the pools it turned off could not be read: " + recordErr.Error()
	case turnedOff[version] && errors.Is(offErr, os.ErrNotExist):
		return transport.SideDoorsStockOwnerRestored, ""
	}
	return "", ""
}

// stockPoolConfigText is one web server configuration file something loads
// (Err: it could not be read, so it counts as a user).
type stockPoolConfigText struct {
	Name, Text string
	Err        error
}

// socketMatcher answers whether a unix: path reaches the stock pool's socket
// (same) and whether that could be decided at all (known).
type socketMatcher func(path string) (same, known bool)

// stockPoolUsers names the configuration files that hand PHP to the stock
// pool's socket (or to the php-fpm.sock alias when it points there): nginx's
// effective configuration (dump) and the Apache files Apache loads (apache).
func stockPoolUsers(dump, listen, alias string, apache []stockPoolConfigText, same socketMatcher) []string {
	if listen == "" || !strings.HasPrefix(listen, "/") {
		// A TCP listen address: anything could reach it; it is not turned off.
		return []string{"listen " + listen}
	}
	targets := []string{listen}
	if strings.HasPrefix(listen, "/run/") {
		targets = append(targets, "/var"+listen)
	}
	if alias != "" {
		targets = append(targets, alias)
		if strings.HasPrefix(alias, "/run/") {
			targets = append(targets, "/var"+alias)
		}
	}
	if strings.HasPrefix(listen, "/var/run/") {
		targets = append(targets, strings.TrimPrefix(listen, "/var"))
	}
	seen := map[string]bool{}
	// The question may only ever fail in the safe direction (S4 R1, second
	// fix round): turning the pool off while something uses it stops that
	// site. So nothing reads nginx's grammar to decide "not used". The pool
	// counts as used when:
	//   - any line that is not a full-line comment names its socket (either
	//     spelling) anywhere: a directive, an argument, a set, a map, an
	//     upstream, quoted or not;
	//   - or any *_pass on such a line is followed, before the statement can
	//     have ended, by a "$": the address is a variable whose value cannot
	//     be known here.
	//   - or a unix: path on such a line reaches the same socket by another
	//     spelling ("//", "./", "..", a link of the owner's own), or cannot be
	//     followed at all (a relative path, a missing folder): socketMatcher
	//     compares what the paths resolve to (S4 R1, third fix round).
	// A customer's harmless directive that names the socket keeps the pool on
	// and gives the customer nothing: the socket is www-data's, mode 0660,
	// and a site account is not in that group; customers may not write any
	// *_pass (internal/services/site_directives.go).
	// The file a mention is in is named from the "# configuration file" line
	// nginx -T prints before each file. That name is only a label for the
	// administrator: a quoted value that fakes such a line can mislabel a
	// mention, never hide one.
	mentions := func(line string) bool {
		for _, target := range targets {
			if strings.Contains(line, target) {
				return true
			}
		}
		return same != nil && unixPathReaches(line, same)
	}
	lines := strings.Split(dump, "\n")
	files := make([]string, len(lines))
	file := "nginx"
	for i, line := range lines {
		files[i] = ""
		trimmed := strings.TrimLeft(line, " \t\r")
		if strings.HasPrefix(trimmed, "#") {
			if name, ok := strings.CutPrefix(strings.TrimRight(trimmed, " \t\r"), "# configuration file "); ok && strings.HasSuffix(name, ":") {
				file = strings.TrimSuffix(name, ":")
			}
			continue
		}
		files[i] = file
		if mentions(line) {
			seen[file] = true
		}
	}
	sealed := sealedPanelLines(lines)
	for i, line := range lines {
		if files[i] == "" {
			continue
		}
		// The generator's reverse-proxy line in CelikPanel's own unchanged
		// file: its variable is only ever "http(s)://host[:port]"
		// (services.runtimeResolvedProxy), never a socket; a socket path in
		// the site's target would still be named on the lines above it.
		if sealed[i] && strings.TrimSpace(line) == panelUpstreamPass {
			continue
		}
		for offset := 0; ; {
			at := strings.Index(line[offset:], "_pass")
			if at < 0 {
				break
			}
			offset += at + len("_pass")
			if passAddressHasVariable(lines, files, i, offset) {
				seen[files[i]] = true
				break
			}
		}
	}
	// Apache's files: a mention or a reaching unix: path outside a comment
	// line (Apache has no comment after a directive); one that could not be
	// read counts.
	for _, f := range apache {
		if f.Err != nil {
			seen[f.Name] = true
			continue
		}
		for _, line := range strings.Split(f.Text, "\n") {
			if !strings.HasPrefix(strings.TrimLeft(line, " \t\r"), "#") && mentions(line) {
				seen[f.Name] = true
				break
			}
		}
	}
	users := make([]string, 0, len(seen))
	for name := range seen {
		users = append(users, name)
	}
	sort.Strings(users)
	return users
}

// passAddressHasVariable reads on from just after a "_pass" (line i, byte
// offset) over the lines that are not full-line comments: a "$" before the
// statement can have ended means the address may be a variable. The statement
// can only have ended at a ";" with no "#" before it on its line ("#" may
// begin a comment that holds that ";"). A ";" inside quotes ends the window
// early only where the address then carries a literal ";", which no stock
// socket path has. Nothing here decides "not used" on its own: it only adds
// users.
func passAddressHasVariable(lines, files []string, i, offset int) bool {
	for line := i; line < len(lines); line++ {
		if files[line] == "" {
			continue
		}
		text := lines[line]
		if line == i {
			text = text[offset:]
		}
		hash := false
		for _, r := range text {
			switch r {
			case '$':
				return true
			case '#':
				hash = true
			case ';':
				if !hash {
					return false
				}
			}
		}
	}
	return false
}

// panelUpstreamPass is the reverse-proxy line CelikPanel's vhost writes for
// an application addressed by a host name (templates/nginx/vhost.conf.tmpl).
const panelUpstreamPass = "proxy_pass $celikpanel_upstream;"

// sealedPanelLines marks the dump's lines that belong to a file carrying
// CelikPanel's render header whose digest is still its body's
// (services.SealedUnchanged): nginx -T prints each file's text after its
// "# configuration file <name>:" line, followed by a line end. A fake header
// line inside a quoted value splits a file, so neither half is sealed.
func sealedPanelLines(lines []string) []bool {
	sealed := make([]bool, len(lines))
	start := -1
	mark := func(end int) {
		if start < 0 || start >= end {
			return
		}
		text := strings.Join(lines[start:end], "\n")
		if services.SealedUnchanged([]byte(text)) || services.SealedUnchanged([]byte(strings.TrimSuffix(text, "\n"))) {
			for i := start; i < end; i++ {
				sealed[i] = true
			}
		}
	}
	for i, line := range lines {
		if strings.HasPrefix(line, "# configuration file ") && strings.HasSuffix(strings.TrimRight(line, "\r"), ":") {
			mark(i)
			start = i + 1
		}
	}
	mark(len(lines))
	return sealed
}

// unixPathReaches: a unix: path on the line reaches the stock pool's socket,
// or cannot be followed (same == nil: never asked).
func unixPathReaches(line string, same socketMatcher) bool {
	for rest := line; ; {
		at := strings.Index(rest, "unix:")
		if at < 0 {
			return false
		}
		rest = rest[at+len("unix:"):]
		end := strings.IndexAny(rest, " \t\r;\"'|{}:")
		path := rest
		if end >= 0 {
			path = rest[:end]
		}
		if path == "" {
			continue
		}
		if reaches, known := same(path); reaches || !known {
			return true
		}
	}
}

// socketIdentity is what a socket path resolves to: the device and inode of
// the file when it exists, and of its folder with the last name (a socket
// not listening yet, or a link to one). ok is false when it cannot be
// followed: a relative path, a folder that is missing or cannot be read.
func socketIdentity(path string) (ids []string, ok bool) {
	if !filepath.IsAbs(path) {
		return nil, false
	}
	if info, err := os.Stat(path); err == nil {
		if id, has := fileIdentity(info); has {
			ids = append(ids, "file:"+id)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false
	}
	target := path
	if followed := followSymlinks(path); followed != "" {
		target = followed
	}
	cut := strings.LastIndex(target, "/")
	dir, name := target[:cut], target[cut+1:]
	if dir == "" {
		dir = "/"
	}
	if name == "" || name == "." || name == ".." {
		return ids, len(ids) > 0
	}
	info, err := os.Stat(dir)
	if err != nil {
		return ids, len(ids) > 0
	}
	if id, has := fileIdentity(info); has {
		ids = append(ids, "entry:"+id+"/"+name)
	}
	return ids, len(ids) > 0
}

// hostSameSocket compares every unix: path with the pool's socket by what
// both resolve to; with the socket itself unresolvable, nothing is known.
func hostSameSocket(listen string) socketMatcher {
	want, wantOK := socketIdentity(listen)
	return func(path string) (bool, bool) {
		if !wantOK {
			return false, false
		}
		got, ok := socketIdentity(path)
		if !ok {
			return false, false
		}
		for _, a := range got {
			for _, b := range want {
				if a == b {
					return true, true
				}
			}
		}
		return false, true
	}
}

// apacheIncludeLine reads an Include or IncludeOptional directive (Apache's
// directive names are case-insensitive).
var apacheIncludeLine = regexp.MustCompile(`(?i)^\s*include(?:optional)?\s+(.+?)\s*$`)

// apacheServerRootLine reads a ServerRoot directive.
var apacheServerRootLine = regexp.MustCompile(`(?i)^\s*serverroot\s+(.+?)\s*$`)

// errApacheIncludeUnfollowed marks an Apache include CelikPanel cannot follow.
var errApacheIncludeUnfollowed = errors.New("an Include that cannot be followed: what it loads is not known")

// apacheLoadedFiles lists the Apache files Apache loads (S4 R1, third fix
// round): none when the Apache program is not installed; otherwise every
// file under root outside the *-available folders (the enabled links
// followed and named as the link), and what any of them includes, wherever
// it is. A file under *-available counts only through an enabled link or an
// include: the PHP-FPM package's own conf-available/php<V>-fpm.conf that
// nobody enabled is not a user. An include that cannot be followed (a
// variable, a line continued on the next, a quoted path with spaces) is
// itself a user, named by its file and line (fourth fix round): what it
// loads is not known, so the pool is kept; every other include is still
// followed. A file that cannot be read counts.
func apacheLoadedFiles(root string, installed bool) []stockPoolConfigText {
	if !installed {
		return nil
	}
	if _, err := os.Lstat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return []stockPoolConfigText{{Name: root, Err: err}}
	}
	loaded := map[string]stockPoolConfigText{}
	var queue []string
	add := func(name, path string) {
		if _, done := loaded[name]; done {
			return
		}
		data, err := os.ReadFile(path)
		loaded[name] = stockPoolConfigText{Name: name, Text: string(data), Err: err}
		queue = append(queue, name)
	}
	// addTree adds a file, or every file below a folder (links followed, a
	// few levels deep).
	var addTree func(name, path string, depth int)
	addTree = func(name, path string, depth int) {
		info, err := os.Stat(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				loaded[name] = stockPoolConfigText{Name: name, Err: err}
			}
			return
		}
		if !info.IsDir() {
			add(name, path)
			return
		}
		if depth > 8 {
			loaded[name] = stockPoolConfigText{Name: name, Err: errors.New("folders nested too deep")}
			return
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			loaded[name] = stockPoolConfigText{Name: name, Err: err}
			return
		}
		for _, entry := range entries {
			addTree(filepath.Join(name, entry.Name()), filepath.Join(path, entry.Name()), depth+1)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return []stockPoolConfigText{{Name: root, Err: err}}
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), "-available") {
			continue
		}
		addTree(filepath.Join(root, entry.Name()), filepath.Join(root, entry.Name()), 0)
	}
	serverRoots := []string{root}
	unfollowed := func(file, line string) {
		name := file + ": " + strings.TrimSpace(line)
		loaded[name] = stockPoolConfigText{Name: name, Err: errApacheIncludeUnfollowed}
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		f := loaded[name]
		if f.Err != nil {
			continue
		}
		for _, line := range strings.Split(f.Text, "\n") {
			if strings.HasPrefix(strings.TrimLeft(line, " \t\r"), "#") {
				continue
			}
			if m := apacheServerRootLine.FindStringSubmatch(line); m != nil {
				serverRoots = append(serverRoots, strings.Trim(m[1], `"'`))
			}
			m := apacheIncludeLine.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			arg := strings.Trim(m[1], `"'`)
			if strings.Contains(arg, "$") || strings.HasSuffix(arg, `\`) || strings.ContainsAny(arg, " \t") {
				unfollowed(name, line)
				continue
			}
			patterns := []string{arg}
			if !filepath.IsAbs(arg) {
				patterns = patterns[:0]
				for _, base := range serverRoots {
					patterns = append(patterns, filepath.Join(base, arg))
				}
			}
			for _, pattern := range patterns {
				matches, err := filepath.Glob(pattern)
				if err != nil {
					unfollowed(name, line)
					break
				}
				for _, match := range matches {
					addTree(match, match, 0)
				}
			}
		}
	}
	files := make([]stockPoolConfigText, 0, len(loaded))
	for _, f := range loaded {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files
}

// apacheInstalled: the Apache program is there (an answer other than "not
// there" counts as installed).
func apacheInstalled() bool {
	_, err := os.Stat("/usr/sbin/apache2")
	return !errors.Is(err, os.ErrNotExist)
}

// webServerUnit is one web server unit's state.
type webServerUnit struct {
	Enabled, Active bool
}

// otherWebServerUnits are the units of PHP front ends whose configuration
// CelikPanel does not read (S4 R1, fourth fix round): enabled or active, one
// keeps every stock pool on.
var otherWebServerUnits = []string{"httpd.service", "lighttpd.service", "caddy.service", "lshttpd.service", "lsws.service", "openlitespeed.service"}

var (
	errOtherWebServer = errors.New("a web server whose configuration CelikPanel does not read")
	errApacheOverride = errors.New("the Apache configuration lets .htaccess files change how PHP is handled")
	// apacheOverrideLine: an AllowOverride or AllowOverrideList that is not
	// None (Apache 2.4's default is None).
	apacheOverrideLine = regexp.MustCompile(`(?i)^\s*allowoverride(?:list)?\s+(.+?)\s*$`)
)

// webServerUsers is what keeps the stock pools on besides nginx (S4 R1,
// fourth fix round):
//   - the Apache files of /etc/apache2 that Apache loads, when the Apache
//     program is installed or its unit is enabled or active, and of
//     /etc/apache2-<name> for each enabled or active apache2@<name>;
//   - for an enabled or active Apache, a loaded line that lets .htaccess
//     files override anything (they may hand PHP to the socket, and no
//     .htaccess is read here);
//   - any enabled or active httpd, lighttpd, Caddy or OpenLiteSpeed unit;
//   - the units that could not be listed at all.
//
// units maps each unit name to its state; load reads one Apache root.
func webServerUsers(units map[string]webServerUnit, unitsErr error, apacheBinary bool, load func(root string, installed bool) []stockPoolConfigText) []stockPoolConfigText {
	var out []stockPoolConfigText
	if unitsErr != nil {
		out = append(out, stockPoolConfigText{Name: "systemctl", Err: fmt.Errorf("the web servers' units could not be listed: %w", unitsErr)})
	}
	running := func(unit string) (string, bool) {
		state, ok := units[unit]
		switch {
		case !ok:
			return "", false
		case state.Active:
			return "active", true
		case state.Enabled:
			return "enabled", true
		}
		return "", false
	}
	roots := map[string]bool{}
	if _, on := running("apache2.service"); on || apacheBinary {
		roots["/etc/apache2"] = on
	}
	names := make([]string, 0, len(units))
	for unit := range units {
		names = append(names, unit)
	}
	sort.Strings(names)
	for _, unit := range names {
		if instance, ok := strings.CutPrefix(unit, "apache2@"); ok {
			if _, on := running(unit); on {
				roots["/etc/apache2-"+strings.TrimSuffix(instance, ".service")] = true
			}
		}
	}
	rootNames := make([]string, 0, len(roots))
	for root := range roots {
		rootNames = append(rootNames, root)
	}
	sort.Strings(rootNames)
	for _, root := range rootNames {
		files := load(root, true)
		out = append(out, files...)
		if !roots[root] {
			continue
		}
		for _, f := range files {
			if f.Err != nil {
				continue
			}
			for _, line := range strings.Split(f.Text, "\n") {
				if strings.HasPrefix(strings.TrimLeft(line, " \t\r"), "#") {
					continue
				}
				if m := apacheOverrideLine.FindStringSubmatch(line); m != nil && !strings.EqualFold(strings.Trim(m[1], `"'`), "none") {
					out = append(out, stockPoolConfigText{Name: f.Name + ": " + strings.TrimSpace(line), Err: errApacheOverride})
				}
			}
		}
	}
	for _, unit := range otherWebServerUnits {
		if state, on := running(unit); on {
			out = append(out, stockPoolConfigText{Name: unit + " (" + state + ")", Err: errOtherWebServer})
		}
	}
	for _, unit := range names {
		if strings.HasPrefix(unit, "httpd@") {
			if state, on := running(unit); on {
				out = append(out, stockPoolConfigText{Name: unit + " (" + state + ")", Err: errOtherWebServer})
			}
		}
	}
	return out
}

// hostWebServerUnits reads the states of the web server units: Apache's own
// and its instances (one per /etc/apache2-<name>), and the other front ends.
func hostWebServerUnits(ctx context.Context) (map[string]webServerUnit, error) {
	units := []string{"apache2.service"}
	if dirs, err := filepath.Glob("/etc/apache2-*"); err == nil {
		for _, dir := range dirs {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				units = append(units, "apache2@"+strings.TrimPrefix(filepath.Base(dir), "apache2-")+".service")
			}
		}
	}
	units = append(units, otherWebServerUnits...)
	args := append([]string{"show", "--no-pager", "-p", "Id", "-p", "UnitFileState", "-p", "ActiveState"}, units...)
	out, err := serviceMutationCommand(ctx, "systemctl", args...).Output()
	if err != nil {
		return nil, errors.New(strings.TrimSpace(firstLine(string(out)) + " " + err.Error()))
	}
	return parseWebServerUnits(string(out)), nil
}

// parseWebServerUnits reads systemctl show's blocks (Id=, UnitFileState=,
// ActiveState=, separated by blank lines). A unit that does not exist has no
// unit file state and is inactive.
func parseWebServerUnits(text string) map[string]webServerUnit {
	units := map[string]webServerUnit{}
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n\n") {
		id, state := "", webServerUnit{}
		for _, line := range strings.Split(block, "\n") {
			key, value, _ := strings.Cut(line, "=")
			switch key {
			case "Id":
				id = value
			case "UnitFileState":
				switch value {
				case "enabled", "enabled-runtime", "linked", "linked-runtime", "alias", "generated", "transient":
					state.Enabled = true
				}
			case "ActiveState":
				switch value {
				case "active", "activating", "reloading", "deactivating", "refreshing":
					state.Active = true
				}
			}
		}
		if id != "" {
			units[id] = state
		}
	}
	return units
}

// otherPoolsBeside: another pool file is in the same pool directory.
func otherPoolsBeside(path string) bool {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".conf") && entry.Name() != filepath.Base(path) {
			return true
		}
	}
	return false
}

// turnOffStockPool marks the version as being turned off, renames the pool
// away, checks and reloads, then records it off; it puts the pool back when
// PHP-FPM refuses. The mark is written BEFORE the rename (B5): a crash in
// between reads at the next start as CelikPanel's unfinished step, never as
// the owner's undo. Called under the pool lock.
func turnOffStockPool(ctx context.Context, ops stockPoolOps, version, path string, wasOff bool) (string, string) {
	off := path + stockPoolOffSuffix
	if _, err := ops.readFile(off); err == nil {
		return transport.SideDoorsStockUnknown, off + " already exists; the server owner decides which file to keep"
	}
	if ops.pending != nil {
		if err := ops.pending(version); err != nil {
			return transport.SideDoorsStockFailed, "CelikPanel's record of it could not be written, so nothing was changed: " + err.Error()
		}
	}
	if err := ops.rename(path, off); err != nil {
		if ops.forgetPending != nil {
			_ = ops.forgetPending(version)
		}
		return transport.SideDoorsStockFailed, err.Error() + "; nothing was changed"
	}
	return checkAndReloadAfterRename(ctx, ops, version, path, wasOff)
}

// checkAndReloadAfterRename checks PHP-FPM without the renamed pool, reloads
// it and records the version off. A refusal renames it back, and only then
// writes the record as it was before the step (S4 R1, third fix round): the
// "turning off" mark stays until the pool is back under its name, so a crash
// in between reads at the next start as CelikPanel's unfinished step (checked
// again, put back again), never as a pool that is off. A first turn-off then
// forgets the version; the administrator's "turn it off again" of a pool the
// owner had put back keeps it recorded, so the owner's undo still stands and
// no later start turns it off by itself.
func checkAndReloadAfterRename(ctx context.Context, ops stockPoolOps, version, path string, wasOff bool) (string, string) {
	off := path + stockPoolOffSuffix
	putBack := func(cause error) (string, string) {
		if err := ops.rename(off, path); err != nil {
			return transport.SideDoorsStockNotPutBack, fmt.Sprintf("%v; putting %s back also failed: %v", cause, path, err)
		}
		detail := cause.Error() + "; it was put back"
		if ops.fpmActive(version) {
			if err := ops.fpmReload(ctx, version); err != nil {
				detail = fmt.Sprintf("%v; %s was put back but PHP-FPM %s did not reload: %v", cause, path, version, err)
			}
		}
		var recordErr error
		if wasOff {
			if ops.forgetPending != nil {
				recordErr = ops.forgetPending(version)
			}
		} else {
			recordErr = ops.record(version, false)
		}
		if recordErr != nil {
			detail += fmt.Sprintf("; CelikPanel's record of it could not be written, so the next start decides it again: %v", recordErr)
		}
		return transport.SideDoorsStockFailed, detail
	}
	if err := ops.fpmTest(ctx, version); err != nil {
		return putBack(err)
	}
	if ops.fpmActive(version) {
		if err := ops.fpmReload(ctx, version); err != nil {
			return putBack(err)
		}
	}
	if err := ops.record(version, true); err != nil {
		return transport.SideDoorsStockTurnedOff, "CelikPanel's record of it could not be written, so the next start finishes this step again: " + err.Error()
	}
	return transport.SideDoorsStockTurnedOff, ""
}

func hostStockPoolOps() stockPoolOps {
	poolOps := hostPHPToolPoolOps()
	return stockPoolOps{
		versions:      installedPHPFPMVersions,
		turnedOff:     services.StockPoolsTurnedOff,
		turning:       services.StockPoolsTurningOff,
		record:        services.SetStockPoolTurnedOff,
		pending:       services.SetStockPoolTurningOff,
		forgetPending: services.ForgetStockPoolTurningOff,
		lock:          services.LockPHPFPMPools,
		readFile:      os.ReadFile,
		conffileMD5: func(ctx context.Context, version, path string) (string, bool) {
			out, err := serviceMutationCommand(ctx, "dpkg-query", "-W", "-f", "${Conffiles}\n", "php"+version+"-fpm").Output()
			if err != nil {
				return "", false
			}
			return conffileMD5From(string(out), path)
		},
		nginxDump: func(ctx context.Context) (string, error) {
			out, err := serviceMutationCommand(ctx, "nginx", "-T").Output()
			if err != nil {
				return "", errors.New(firstLine(string(out)))
			}
			return string(out), nil
		},
		apacheFiles: func() []stockPoolConfigText {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			units, err := hostWebServerUnits(ctx)
			return webServerUsers(units, err, apacheInstalled(), apacheLoadedFiles)
		},
		sameSocket: hostSameSocket,
		aliasFor: func(listen string) string {
			alias := filepath.Join(phpFPMRunDir, "php-fpm.sock")
			if followSymlinks(alias) == listen {
				return alias
			}
			return ""
		},
		rename:  os.Rename,
		fpmTest: poolOps.fpmTest,
		fpmActive: func(version string) bool {
			return phpFPMUnitState(version) == "active"
		},
		fpmReload: poolOps.fpmReload,
	}
}

// conffileMD5From reads one file's MD5 from dpkg's Conffiles field
// (" /etc/php/8.4/fpm/pool.d/www.conf <md5>[ obsolete]").
func conffileMD5From(field, path string) (string, bool) {
	for _, line := range strings.Split(field, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == path {
			if len(fields) > 2 && fields[2] == "obsolete" {
				return "", false
			}
			return fields[1], true
		}
	}
	return "", false
}

// followSymlinks follows a chain of symbolic links (the alternatives system's
// /run/php/php-fpm.sock → /etc/alternatives/php-fpm.sock → the versioned
// socket) without requiring the last target to exist; "" when path is not a
// link.
func followSymlinks(path string) string {
	current := path
	for hop := 0; hop < 8; hop++ {
		target, err := os.Readlink(current)
		if err != nil {
			if hop == 0 {
				return ""
			}
			return current
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		current = filepath.Clean(target)
	}
	return current
}
