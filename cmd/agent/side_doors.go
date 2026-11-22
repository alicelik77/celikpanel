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
//     the conffile), no web server configuration hands PHP to it, and another
//     pool remains (PHP-FPM refuses to start without one). Turning off is a
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
func (a *Agent) CloseSideDoorsV1(req *transport.SideDoorsRequest, resp *transport.SideDoorsResponse) error {
	*resp = transport.SideDoorsResponse{}
	if req == nil {
		resp.Error = "missing request"
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
	// The administrator's explicit "close it again" for what the owner undid
	// (S4 R1 fix round): the owner's undo is forgotten, then the step decides
	// as at a first start.
	if err := forgetOwnerUndo(req.ReClose, req.Version); err != nil {
		resp.Error = err.Error()
		return nil
	}
	state := transport.SideDoorsState{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	state.Tools = closeSideDoorsTools(ctx, hostPHPToolBindOps(), hostSideDoorsToolHooks(), afterPHPToolBound)
	state.StockPools = closeStockFPMPoolsWith(ctx, hostStockPoolOps())
	// 3 and 4 (21 Nov 2026, side_doors_jobs.go): at closed to site accounts;
	// site crontabs with lines not made in CelikPanel counted.
	state.At = closeSiteAtWith(ctx, hostSiteAtOps())
	state.SiteCrontabs = checkSiteCrontabs()
	resp.State = &state
	return nil
}

const sideDoorsMutationTarget = "side-doors"

// forgetOwnerUndo is the explicit re-close: at forgets CelikPanel's record of
// its /etc/at.allow (a file then missing is written as at a first start); a
// stock pool forgets that CelikPanel turned that version's pool off.
func forgetOwnerUndo(target, version string) error {
	switch target {
	case "":
		return nil
	case transport.SideDoorsReCloseAt:
		if err := os.Remove(atAllowRecordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("forget the record of /etc/at.allow: %w", err)
		}
		return nil
	case transport.SideDoorsReCloseStockPool:
		if !phpVersionText.MatchString(version) {
			return fmt.Errorf("invalid PHP version %q", version)
		}
		return services.SetStockPoolTurnedOff(version, false)
	}
	return fmt.Errorf("unknown side door %q", target)
}

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
	// record writes it.
	turnedOff   func() (map[string]bool, error)
	record      func(version string, off bool) error
	readFile    func(path string) ([]byte, error)
	conffileMD5 func(ctx context.Context, version, path string) (string, bool)
	nginxDump   func(ctx context.Context) (string, error)
	apacheRefs  func(socket string) []string
	aliasFor    func(listen string) string
	rename      func(from, to string) error
	fpmTest     func(ctx context.Context, version string) error
	fpmActive   func(version string) bool
	fpmReload   func(ctx context.Context, version string) error
}

var stockPoolListen = regexp.MustCompile(`(?m)^\s*listen\s*=\s*(\S+)\s*$`)

func stockPoolPath(version string) string {
	return filepath.Join(phpEtcDir, version, "fpm", "pool.d", "www.conf")
}

// closeStockFPMPoolsWith decides and, where allowed, turns off each
// version's stock pool.
func closeStockFPMPoolsWith(ctx context.Context, ops stockPoolOps) []transport.SideDoorsStockPool {
	pools := []transport.SideDoorsStockPool{}
	var dump string
	var dumpErr error
	dumped := false
	turnedOff, recordErr := ops.turnedOff()
	for _, version := range ops.versions() {
		path := stockPoolPath(version)
		row := transport.SideDoorsStockPool{Version: version, Path: path}
		content, err := ops.readFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if _, offErr := ops.readFile(path + stockPoolOffSuffix); offErr == nil {
					row.State = transport.SideDoorsStockOff
					if recordErr == nil && !turnedOff[version] {
						// Turned off before the record was kept: remembered now,
						// so the owner putting it back is seen as such.
						if err := ops.record(version, true); err != nil {
							row.Detail = "CelikPanel's record of it could not be written: " + err.Error()
						}
					}
					pools = append(pools, row)
				}
				continue
			}
			row.State, row.Detail = transport.SideDoorsStockUnknown, err.Error()
			pools = append(pools, row)
			continue
		}
		// The owner's undo stands (D-022): CelikPanel turned it off and it is
		// back under its own name, so the server owner put it back. It is left
		// on; only the administrator's explicit "turn it off again" forgets
		// this (forgetOwnerUndo).
		if turnedOff[version] {
			if _, offErr := ops.readFile(path + stockPoolOffSuffix); offErr != nil {
				row.State = transport.SideDoorsStockOwnerRestored
				pools = append(pools, row)
				continue
			}
		}
		if recordErr != nil {
			row.State, row.Detail = transport.SideDoorsStockUnknown, "CelikPanel's record of the pools it turned off could not be read: "+recordErr.Error()
			pools = append(pools, row)
			continue
		}
		sum := md5.Sum(content) //nolint:gosec
		packaged, known := ops.conffileMD5(ctx, version, path)
		switch {
		case !known:
			row.State, row.Detail = transport.SideDoorsStockUnknown, "the package's own copy of this file could not be read (dpkg)"
			pools = append(pools, row)
			continue
		case packaged != hex.EncodeToString(sum[:]):
			row.State = transport.SideDoorsStockOwnerChanged
			pools = append(pools, row)
			continue
		}
		if !dumped {
			dump, dumpErr = ops.nginxDump(ctx)
			dumped = true
		}
		if dumpErr != nil {
			row.State, row.Detail = transport.SideDoorsStockUnknown, "nginx -T did not answer: "+dumpErr.Error()
			pools = append(pools, row)
			continue
		}
		listen := ""
		if match := stockPoolListen.FindSubmatch(content); match != nil {
			listen = string(match[1])
		}
		row.UsedBy = stockPoolUsers(dump, listen, ops.aliasFor(listen), ops.apacheRefs)
		if len(row.UsedBy) > 0 {
			row.State = transport.SideDoorsStockInUse
			pools = append(pools, row)
			continue
		}
		if !otherPoolsBeside(path) {
			row.State = transport.SideDoorsStockOnlyPool
			pools = append(pools, row)
			continue
		}
		row.State, row.Detail = turnOffStockPool(ctx, ops, version, path)
		pools = append(pools, row)
	}
	return pools
}

// stockPoolUsers names the configuration files that hand PHP to the stock
// pool's socket (or to the php-fpm.sock alias when it points there).
func stockPoolUsers(dump, listen, alias string, apacheRefs func(string) []string) []string {
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
	// A customer's harmless directive that names the socket keeps the pool on
	// and gives the customer nothing: the socket is www-data's, mode 0660,
	// and a site account is not in that group; customers may not write any
	// *_pass (internal/services/site_directives.go).
	// The file a mention is in is named from the "# configuration file" line
	// nginx -T prints before each file. That name is only a label for the
	// administrator: a quoted value that fakes such a line can mislabel a
	// mention, never hide one.
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
		for _, target := range targets {
			if strings.Contains(line, target) {
				seen[file] = true
			}
		}
	}
	for i, line := range lines {
		if files[i] == "" {
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
	if apacheRefs != nil {
		for _, target := range targets {
			for _, ref := range apacheRefs(target) {
				seen[ref] = true
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

// turnOffStockPool renames the pool away, checks and reloads, and puts it
// back when PHP-FPM refuses.
func turnOffStockPool(ctx context.Context, ops stockPoolOps, version, path string) (string, string) {
	off := path + stockPoolOffSuffix
	if _, err := ops.readFile(off); err == nil {
		return transport.SideDoorsStockUnknown, off + " already exists; the server owner decides which file to keep"
	}
	if err := ops.rename(path, off); err != nil {
		return transport.SideDoorsStockFailed, err.Error()
	}
	putBack := func(cause error) (string, string) {
		if err := ops.rename(off, path); err != nil {
			return transport.SideDoorsStockNotPutBack, fmt.Sprintf("%v; putting %s back also failed: %v", cause, path, err)
		}
		if ops.fpmActive(version) {
			if err := ops.fpmReload(ctx, version); err != nil {
				return transport.SideDoorsStockFailed, fmt.Sprintf("%v; %s was put back but PHP-FPM %s did not reload: %v", cause, path, version, err)
			}
		}
		return transport.SideDoorsStockFailed, cause.Error() + "; it was put back"
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
		return transport.SideDoorsStockTurnedOff, "CelikPanel's record of it could not be written, so the server owner putting it back would not be told from a first start: " + err.Error()
	}
	return transport.SideDoorsStockTurnedOff, ""
}

func hostStockPoolOps() stockPoolOps {
	poolOps := hostPHPToolPoolOps()
	return stockPoolOps{
		versions:  installedPHPFPMVersions,
		turnedOff: services.StockPoolsTurnedOff,
		record:    services.SetStockPoolTurnedOff,
		readFile:  os.ReadFile,
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
		apacheRefs: func(socket string) []string {
			var refs []string
			_ = filepath.WalkDir("/etc/apache2", func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
					return nil
				}
				if data, readErr := os.ReadFile(path); readErr == nil && strings.Contains(string(data), socket) {
					refs = append(refs, path)
				}
				return nil
			})
			return refs
		},
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
