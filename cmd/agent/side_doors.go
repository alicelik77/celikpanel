package main

import (
	"bufio"
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

const stockPoolOffSuffix = ".off-by-celikpanel"

// CloseSideDoorsV1 is the step; it holds the job's mutation step.
func (a *Agent) CloseSideDoorsV1(req *transport.ServiceMutationRequest, resp *transport.SideDoorsResponse) error {
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
	state := transport.SideDoorsState{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	state.Tools = closeSideDoorsTools(ctx, hostPHPToolBindOps(), afterPHPToolBound)
	state.StockPools = closeStockFPMPoolsWith(ctx, hostStockPoolOps())
	// 3 and 4 (21 Nov 2026, side_doors_jobs.go): at closed to site accounts;
	// site crontabs with lines not made in CelikPanel counted.
	state.At = closeSiteAtWith(ctx, hostSiteAtOps())
	state.SiteCrontabs = checkSiteCrontabs()
	resp.State = &state
	return nil
}

const sideDoorsMutationTarget = "side-doors"

// closeSideDoorsTools moves each installed tool still in a stock pool to its
// own account.
// after runs what follows a verified move (webmail's own addresses follow its
// socket); nil in tests.
func closeSideDoorsTools(ctx context.Context, ops phpToolBindOps, after func(context.Context, string) string) []transport.SideDoorsTool {
	tools := []transport.SideDoorsTool{}
	failures := readPHPToolAccountFailures()
	for _, state := range phpToolStates() {
		if !state.Installed || state.Socket == "" {
			continue
		}
		row := transport.SideDoorsTool{Tool: state.Tool, Version: state.version()}
		if _, own := phpToolOwnSocket(state.Socket); own {
			row.State, row.Account = transport.SideDoorsToolOwnAccount, phpToolAccount(state.Tool)
			tools = append(tools, row)
			continue
		}
		switch {
		case ops.family() != "apt":
			row.State = transport.SideDoorsToolUnsupported
		case row.Version == "" || !phpVersionInstalled(row.Version):
			row.State, row.Detail = transport.SideDoorsToolWaiting, "the PHP version it runs on is not installed"
		default:
			if failure, failed := failures[state.Tool]; failed && failure.PHPVersion == row.Version {
				row.State, row.Detail = transport.SideDoorsToolFailed, failure.Detail
				break
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
		}
		tools = append(tools, row)
	}
	return tools
}

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
	versions    func() []string
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
	for _, version := range ops.versions() {
		path := stockPoolPath(version)
		row := transport.SideDoorsStockPool{Version: version, Path: path}
		content, err := ops.readFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if _, offErr := ops.readFile(path + stockPoolOffSuffix); offErr == nil {
					row.State = transport.SideDoorsStockOff
					pools = append(pools, row)
				}
				continue
			}
			row.State, row.Detail = transport.SideDoorsStockUnknown, err.Error()
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
	seen := map[string]bool{}
	file := "nginx"
	scanner := bufio.NewScanner(strings.NewReader(dump))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# configuration file ") {
			file = strings.TrimSuffix(strings.TrimPrefix(line, "# configuration file "), ":")
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, target := range targets {
			if strings.Contains(trimmed, "unix:"+target) {
				seen[file] = true
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
			return transport.SideDoorsStockFailed, fmt.Sprintf("%v; putting %s back also failed: %v", cause, path, err)
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
	return transport.SideDoorsStockTurnedOff, ""
}

func hostStockPoolOps() stockPoolOps {
	poolOps := hostPHPToolPoolOps()
	return stockPoolOps{
		versions: installedPHPFPMVersions,
		readFile: os.ReadFile,
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
