package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The database tools and webmail under their own accounts (S4 R1, 20 Nov
// 2026, tenant isolation). Until now phpMyAdmin, phpPgAdmin and Roundcube
// ran in the distribution's stock PHP-FPM pool, as the web server's account
// (www-data). That account's group can read every site's home (site homes
// are <account>:<web group> 0750), so a fault in any of the three tools read
// every site's files.
//
// Each tool now has its own system account (celikpanel-<tool>, no login, no
// home, a member of no other group) and its own pool on the PHP version it is
// bound to: /etc/php/<ver>/fpm/pool.d/celikpanel-<tool>.conf, listening on
// /run/php/php<ver>-fpm-celikpanel-<tool>.sock, owned by the web server's
// account with mode 0660 so only nginx reaches it. The pool file carries the
// render header (D-031): an owner's change is kept and only its socket is
// used; the Panel's text then waits beside it.
//
// What each account reads and writes:
//   - Webmail: the tree's root stays closed to others (root:<web group>
//     0750); the account gets through it by an access-control-list entry
//     and reads the published code (php_tool_files.go, second fix round);
//     config/ is root:<account> 0750 with its files 0640 (des_key);
//     db/ (SQLite and its journal: sessions and settings), temp/
//     (attachments being written) and logs/ belong to the account, 0700.
//   - phpMyAdmin (distribution package): its code and /etc/phpmyadmin/
//     config.inc.php are world-readable. The package's secret file
//     (/var/lib/phpmyadmin/blowfish_secret.inc.php) and the control user's
//     file (/etc/phpmyadmin/config-db.php, dbconfig-common), both
//     root:www-data 0640 from the package, are given the account's group
//     while they have the package's owner and mode; Debian's config.inc.php
//     skips an unreadable one. CelikPanel's
//     /etc/phpmyadmin/conf.d/celikpanel-own-account.php (root:<account> 0640,
//     read last) gives a cookie secret and a temp directory of the account's
//     own, so phpMyAdmin signs in even when a package upgrade gave those
//     files back to the web group.
//   - phpPgAdmin: code and configuration are world-readable and hold no
//     secret.
//   - All three: PHP sessions, uploads and temporary files in
//     /var/lib/celikpanel-php-tools/<tool>/ (the account's, 0700).
//
// Araçlar ve web postası kendi hesaplarıyla çalışır: her biri için sistem
// hesabı ve bağlı olduğu PHP sürümünde kendi havuzu. Web sunucusunun grubu
// (her sitenin evini okuyan) artık bu araçlarda yoktur.

const phpToolAccountPrefix = "celikpanel-"

// Paths are variables so tests can point them at a temporary tree.
var (
	phpToolDataRoot           = "/var/lib/celikpanel-php-tools"
	phpToolAccountRecordPath  = "/var/lib/celikpanel-agent-private/php-tool-accounts.json"
	phpMyAdminMainConfig      = "/etc/phpmyadmin/config.inc.php"
	phpMyAdminConfDir         = "/etc/phpmyadmin/conf.d"
	phpMyAdminOwnAccountConf  = "/etc/phpmyadmin/conf.d/celikpanel-own-account.php"
	phpMyAdminDBConfig        = "/etc/phpmyadmin/config-db.php"
	phpMyAdminBlowfishSecret  = "/var/lib/phpmyadmin/blowfish_secret.inc.php"
	etcGroupPath              = "/etc/group"
	phpToolPoolSocketName     = regexp.MustCompile(`^php([0-9]{1,2}\.[0-9]{1,2})-fpm-celikpanel-(webmail|phpmyadmin|phppgadmin)\.sock$`)
	phpToolPoolListenLine     = regexp.MustCompile(`(?m)^\s*listen\s*=\s*(/\S+)\s*$`)
	phpMyAdminOwnSecretLine   = regexp.MustCompile(`(?m)^\$cfg\['blowfish_secret'\] = '([A-Za-z0-9]{32})';$`)
	phpToolSocketAppearBound  = 10 * time.Second
	phpToolSocketAppearPeriod = 200 * time.Millisecond
)

func phpToolAccount(tool string) string { return phpToolAccountPrefix + tool }

// phpToolPoolSocket is the socket a tool's own pool listens on.
func phpToolPoolSocket(tool, version string) string {
	return filepath.Join(phpFPMRunDir, "php"+version+"-fpm-"+phpToolAccount(tool)+".sock")
}

// phpToolPoolPath is the tool's pool file on one PHP version.
func phpToolPoolPath(tool, version string) string {
	return filepath.Join(phpEtcDir, version, "fpm", "pool.d", phpToolAccount(tool)+".conf")
}

// phpToolOwnSocket says whether a socket is a tool's own pool (and whose).
func phpToolOwnSocket(socket string) (string, bool) {
	if match := phpToolPoolSocketName.FindStringSubmatch(filepath.Base(socket)); match != nil {
		return match[2], true
	}
	return "", false
}

func phpToolDataDir(tool string) string { return filepath.Join(phpToolDataRoot, tool) }

// phpToolPoolBody is CelikPanel's text of a tool's pool (under the header).
func phpToolPoolBody(tool, version, webServer string) string {
	account := phpToolAccount(tool)
	data := phpToolDataDir(tool)
	var b strings.Builder
	fmt.Fprintf(&b, "; The PHP-FPM pool of %s under its own account, written by CelikPanel. A line you add or change here is kept:\n", phpToolDisplayName(tool))
	b.WriteString("; CelikPanel then uses this pool's socket as it is and does not write this file again.\n")
	b.WriteString("; Buraya eklediğiniz ya da değiştirdiğiniz satır korunur; CelikPanel bu dosyayı yeniden yazmaz.\n")
	fmt.Fprintf(&b, "[%s]\n", account)
	fmt.Fprintf(&b, "user = %s\ngroup = %s\n", account, account)
	fmt.Fprintf(&b, "listen = %s\n", phpToolPoolSocket(tool, version))
	fmt.Fprintf(&b, "listen.owner = %s\nlisten.group = %s\nlisten.mode = 0660\n", webServer, webServer)
	b.WriteString("pm = ondemand\npm.max_children = 5\npm.process_idle_timeout = 60s\npm.max_requests = 500\n")
	b.WriteString("chdir = /\nsecurity.limit_extensions = .php\n")
	fmt.Fprintf(&b, "php_admin_value[session.save_path] = %s\n", filepath.Join(data, "sessions"))
	b.WriteString("php_admin_value[session.gc_probability] = 1\n")
	fmt.Fprintf(&b, "php_admin_value[upload_tmp_dir] = %s\n", filepath.Join(data, "tmp"))
	fmt.Fprintf(&b, "php_admin_value[sys_temp_dir] = %s\n", filepath.Join(data, "tmp"))
	b.WriteString("php_admin_flag[display_errors] = off\n")
	return b.String()
}

// phpToolPoolOps is the seam of ownPHPToolPoolWith; production fills it with
// host operations, tests with recorders.
type phpToolPoolOps struct {
	webServer     func() string
	lookupAccount func(name string) (uid, gid int, found bool, err error)
	createAccount func(ctx context.Context, name string) error
	removeAccount func(ctx context.Context, name string) error
	groupMembers  func(group string) []string
	prepareDirs   func(tool string, uid, gid int) error
	removeDirs    func(tool string) error
	// prepareFiles gives the tool's files to its account. servedOwn: the web
	// server already hands the tool to its own pool (a move between PHP
	// versions), so nothing of the files is put back on a later failure.
	prepareFiles func(ctx context.Context, tool string, uid, gid int, servedOwn bool) (func(context.Context) error, error)
	applyPool    func(file services.ManagedSiteFile, validate, activate func() error) (transport.SiteFileResult, error)
	snapshotFile func(path string) (mailFileSnapshot, error)
	restoreFile  func(mailFileSnapshot) error
	readFile     func(path string) ([]byte, error)
	removeFile   func(path string) error
	fpmTest      func(ctx context.Context, version string) error
	fpmReload    func(ctx context.Context, version string) error
	socketReady  func(path string) bool
	wait         func(ctx context.Context, d time.Duration) bool
	// lockPools is the host-wide lock of PHP-FPM pool files
	// (services.LockPHPFPMPools), held around every keep, write, remove,
	// configuration test and reload of a version's pools (S4 R1, second fix
	// round, B6); nil in tests that do not look at it.
	lockPools func() func()
}

func (ops phpToolPoolOps) holdPools() func() {
	if ops.lockPools == nil {
		return func() {}
	}
	return ops.lockPools()
}

// ownPHPToolPoolWith gives a tool its own account and pool on one PHP
// version. It returns the socket nginx hands the tool's PHP to and the undo
// of what this call changed (the pool file it wrote, the files it gave the
// account, the account it created); undo is nil when nothing changed. On an
// error, what it changed has already been put back.
// Bir araca kendi hesabını ve havuzunu verir; değiştirdiğinin geri alınışını
// döndürür. Hata durumunda değiştirdiği zaten geri konmuştur.
func ownPHPToolPoolWith(ctx context.Context, ops phpToolPoolOps, tool, version string, servedOwn bool) (string, func(context.Context) error, error) {
	if !validPHPTool(tool) || !phpVersionText.MatchString(version) {
		return "", nil, fmt.Errorf("invalid PHP tool or version %q %q", tool, version)
	}
	webServer := ops.webServer()
	if webServer == "" {
		return "", nil, fmt.Errorf("the web server's account was not found, so no pool can be given to nginx")
	}
	account := phpToolAccount(tool)
	var undos []func(context.Context) error
	undoAll := func(ctx context.Context) error {
		var errs []error
		for i := len(undos) - 1; i >= 0; i-- {
			if err := undos[i](ctx); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	fail := func(err error) (string, func(context.Context) error, error) {
		if undoErr := undoAll(ctx); undoErr != nil {
			return "", nil, fmt.Errorf("%w; putting back what was changed also failed: %v", err, undoErr)
		}
		return "", nil, err
	}

	uid, gid, found, err := ops.lookupAccount(account)
	if err != nil {
		return "", nil, fmt.Errorf("read the account %s: %w", account, err)
	}
	created := !found
	if !found {
		if err := ops.createAccount(ctx, account); err != nil {
			return "", nil, fmt.Errorf("create the account %s: %w", account, err)
		}
		undos = append(undos, func(ctx context.Context) error { return ops.removeAccount(ctx, account) })
		uid, gid, found, err = ops.lookupAccount(account)
		if err != nil || !found {
			return fail(fmt.Errorf("the account %s is not readable after it was created: %v", account, err))
		}
	}
	if uid == 0 || gid == 0 {
		return fail(fmt.Errorf("the account %s is root's (uid %d, gid %d); CelikPanel runs no tool as root", account, uid, gid))
	}
	for _, member := range ops.groupMembers(webServer) {
		if member == account {
			return fail(fmt.Errorf("the account %s is a member of the web server's group %s, which reads every site; the server owner removes it (gpasswd -d %s %s) and tries again", account, webServer, account, webServer))
		}
	}
	if webUID, webGID, webFound, _ := ops.lookupAccount(webServer); webFound && (webUID == uid || webGID == gid) {
		return fail(fmt.Errorf("the account %s shares its id with the web server's account %s", account, webServer))
	}
	if created {
		// An account made by this call goes on an undo, and its data folder
		// with it, before it: no folder is left owned by an id userdel freed.
		undos = append(undos, func(context.Context) error { return ops.removeDirs(tool) })
	}
	if err := ops.prepareDirs(tool, uid, gid); err != nil {
		return fail(fmt.Errorf("prepare the directories of %s: %w", account, err))
	}
	undoFiles, err := ops.prepareFiles(ctx, tool, uid, gid, servedOwn)
	if err != nil {
		return fail(fmt.Errorf("give %s's files to %s: %w", phpToolDisplayName(tool), account, err))
	}
	if undoFiles != nil {
		undos = append(undos, undoFiles)
	}

	path := phpToolPoolPath(tool, version)
	file := services.ManagedSiteFile{
		Kind:         transport.SiteFileKindPHPPool,
		Path:         path,
		HeaderPrefix: services.ManagedHeaderPrefixINI,
		Body:         phpToolPoolBody(tool, version, webServer),
		Trigger:      transport.SiteFileTriggerCreate,
	}
	validate := func() error { return ops.fpmTest(ctx, version) }
	activate := func() error { return ops.fpmReload(ctx, version) }
	// The bytes before the write: a CelikPanel pool the write replaced (a new
	// web server account, a later release's text) comes back as it was, so
	// the socket the restored web server configuration names still answers.
	release := ops.holdPools()
	previous, err := ops.snapshotFile(path)
	if err != nil {
		release()
		return fail(fmt.Errorf("read the pool %s: %w", path, err))
	}
	result, err := ops.applyPool(file, validate, activate)
	release()
	if err != nil {
		return fail(fmt.Errorf("write the pool %s: %w", path, err))
	}
	socket := phpToolPoolSocket(tool, version)
	switch result.Outcome {
	case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeRecreated:
		undos = append(undos, func(ctx context.Context) error {
			defer ops.holdPools()()
			if previous.exists {
				if err := ops.restoreFile(previous); err != nil {
					return fmt.Errorf("put %s back: %w", path, err)
				}
			} else if err := ops.removeFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove %s: %w", path, err)
			}
			services.RemoveSiteFilePending(path)
			if err := ops.fpmTest(ctx, version); err != nil {
				return fmt.Errorf("%s was put back, but PHP-FPM %s refuses its configuration, so it was not reloaded: %w", path, version, err)
			}
			return ops.fpmReload(ctx, version)
		})
	case transport.SiteFileOutcomeUnchanged:
	case transport.SiteFileOutcomeKept:
		// The owner's pool (D-022): kept as it is; nginx is pointed at the
		// socket it names.
		content, readErr := ops.readFile(path)
		match := phpToolPoolListenLine.FindSubmatch(content)
		if readErr != nil || match == nil {
			return fail(fmt.Errorf("%s was changed outside CelikPanel and names no Unix socket to listen on, so CelikPanel does not use it", path))
		}
		socket = string(match[1])
	default:
		detail := result.Reason
		if result.Detail != "" {
			detail += ": " + result.Detail
		}
		return fail(fmt.Errorf("the pool %s was not written (%s %s)", path, result.Outcome, detail))
	}
	deadline := phpToolSocketAppearBound
	for !ops.socketReady(socket) {
		if deadline <= 0 || !ops.wait(ctx, phpToolSocketAppearPeriod) {
			return fail(fmt.Errorf("PHP-FPM %s did not open %s after it reloaded", version, socket))
		}
		deadline -= phpToolSocketAppearPeriod
	}
	if len(undos) == 0 {
		return socket, nil, nil
	}
	return socket, undoAll, nil
}

// retirePHPToolPoolsWith removes a tool's own pools on every PHP version but
// keep ("" removes all of them), reloading each version whose pool went. A
// pool the owner changed is copied beside it first (as a site's pool is when
// the site goes). A version is never left without a pool (S4 R1 fix round):
// when the tool's pool is its last one, the distribution's pool CelikPanel
// turned off is put back first (services.KeepAPoolBeside); the version's own
// configuration test runs before every reload, and a refusal puts the pool
// (and the stock pool's state) back and says so. It returns what it could not
// remove.
func retirePHPToolPoolsWith(ctx context.Context, ops phpToolPoolOps, tool, keep string) error {
	var errs []error
	for _, version := range installedPHPFPMVersions() {
		if version == keep {
			continue
		}
		if err := retirePHPToolPoolOn(ctx, ops, tool, version); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// retirePHPToolPoolOn removes a tool's pool on one version under the
// host-wide pool lock: whether another pool remains, the stock pool put back,
// the removal, the configuration test and the reload are one step that no
// site deletion or switch can interleave with (B6).
func retirePHPToolPoolOn(ctx context.Context, ops phpToolPoolOps, tool, version string) error {
	defer ops.holdPools()()
	path := phpToolPoolPath(tool, version)
	if _, err := os.Lstat(path); err != nil {
		services.RemoveSiteFilePending(path)
		return nil
	}
	previous, err := ops.snapshotFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if _, err := services.BackupUnlessManaged(path, services.ManagedHeaderPrefixINI, nil); err != nil {
		return fmt.Errorf("keep a copy of %s: %w", path, err)
	}
	undoKeep, err := services.KeepAPoolBeside(path)
	if err != nil {
		return fmt.Errorf("%s is kept: %w", path, err)
	}
	if err := ops.removeFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		if undoKeep != nil {
			err = errors.Join(err, undoKeep())
		}
		return fmt.Errorf("remove %s: %w", path, err)
	}
	if err := ops.fpmTest(ctx, version); err != nil {
		putBack := ops.restoreFile(previous)
		if putBack == nil && undoKeep != nil {
			putBack = undoKeep()
		}
		if putBack != nil {
			return fmt.Errorf("PHP-FPM %s refuses its configuration without %s's pool (%v), and putting the pool back failed: %v; PHP-FPM %s was not reloaded, the server administrator checks %s", version, phpToolDisplayName(tool), err, putBack, version, filepath.Dir(path))
		}
		return fmt.Errorf("PHP-FPM %s refuses its configuration without %s's pool (%v); the pool was put back and PHP-FPM %s was not reloaded", version, phpToolDisplayName(tool), err, version)
	}
	services.RemoveSiteFilePending(path)
	if err := ops.fpmReload(ctx, version); err != nil {
		return fmt.Errorf("reload PHP-FPM %s after its %s pool went: %w", version, phpToolDisplayName(tool), err)
	}
	return nil
}

// retireUninstalledPHPToolWith removes everything a tool's own account had
// once the tool itself is gone: its pools, its data directory and the
// account.
func retireUninstalledPHPToolWith(ctx context.Context, ops phpToolPoolOps, tool string) error {
	var errs []error
	if err := retirePHPToolPoolsWith(ctx, ops, tool, ""); err != nil {
		errs = append(errs, err)
	}
	if tool == phpToolPHPMyAdmin {
		if err := ops.removeFile(phpMyAdminOwnAccountConf); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := ops.removeDirs(tool); err != nil {
		errs = append(errs, err)
	}
	if err := clearPHPToolFileJournal(tool); err != nil {
		errs = append(errs, err)
	}
	if _, _, found, _ := ops.lookupAccount(phpToolAccount(tool)); found && len(errs) == 0 {
		if err := ops.removeAccount(ctx, phpToolAccount(tool)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ownSocketForUnboundTool: a tool whose configuration names no socket yet
// (a new install, or one installed again) gets its own account and pool on
// the version chosen for it. A failure leaves it on that version's stock
// pool, is recorded (the step at the next Panel start says it) and is
// returned as a warning. A tool already served keeps its socket: moving it is
// the verified step (closeSideDoorsTools) or the Manage window.
// Henüz soketi olmayan araç kendi hesabını ve havuzunu alır; başarısızlık
// kaydedilir ve uyarı olarak döner.
func ownSocketForUnboundTool(ctx context.Context, state phpToolState, socket string) (string, string) {
	if state.Source == "config" || detectPkgFamily() != "apt" {
		return socket, ""
	}
	if _, own := phpToolOwnSocket(socket); own {
		return socket, ""
	}
	version := fpmSocketVersion(socket)
	if version == "" || !phpVersionInstalled(version) {
		return socket, ""
	}
	own, _, err := ownPHPToolPoolWith(ctx, hostPHPToolPoolOps(), state.Tool, version, false)
	if err == nil {
		// The new configuration names the own pool from now on.
		if clearErr := clearPHPToolFileJournal(state.Tool); clearErr != nil {
			log.Printf("clear %s's file record: %v", state.Tool, clearErr)
		}
	}
	if err != nil {
		if recordErr := writePHPToolAccountFailure(state.Tool, version, err.Error()); recordErr != nil {
			log.Printf("record %s account failure: %v", state.Tool, recordErr)
		}
		if !pathExists(socket) {
			// Its caller then does not point it there (S4 R1, second fix
			// round, B3): webmail's configuration is refused, a database tool
			// is left out of the configuration.
			return socket, fmt.Sprintf("%s could not be given its own account on PHP %s (%v), and that version's stock pool is off, so nothing would answer for it: it is not pointed there and does not open until it is moved. The administrator opens it under Components and chooses its PHP version, which tries its own account again", phpToolDisplayName(state.Tool), version, err)
		}
		return socket, fmt.Sprintf("%s runs under the web server's account on PHP %s, which can read every site, because it could not be given its own account (%v). The administrator opens it under Components and chooses its PHP version to try again", phpToolDisplayName(state.Tool), version, err)
	}
	if err := writePHPToolAccountFailure(state.Tool, "", ""); err != nil {
		log.Printf("clear %s account failure: %v", state.Tool, err)
	}
	return own, ""
}

// retireUninstalledPHPTool removes a gone tool's account, pools and data,
// when it left any. Its error is a warning: the tool is gone either way.
func retireUninstalledPHPTool(ctx context.Context, tool string) error {
	ops := hostPHPToolPoolOps()
	_, _, accountFound, _ := ops.lookupAccount(phpToolAccount(tool))
	left := accountFound || pathExists(phpToolDataDir(tool))
	for _, version := range installedPHPFPMVersions() {
		if pathExists(phpToolPoolPath(tool, version)) {
			left = true
		}
	}
	if !left {
		return nil
	}
	return retireUninstalledPHPToolWith(ctx, ops, tool)
}

// --- host operations ---

func hostPHPToolPoolOps() phpToolPoolOps {
	return phpToolPoolOps{
		webServer:     webServerGroup,
		lookupAccount: lookupSystemAccount,
		createAccount: func(ctx context.Context, name string) error {
			out, err := runServiceMutationCombinedOutput(ctx, "useradd", "--system", "--user-group", "--no-create-home",
				"--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", "--comment", "CelikPanel tool account", name)
			if err != nil {
				return errors.New(commandFailureDetail("useradd", out, err))
			}
			return nil
		},
		removeAccount: func(ctx context.Context, name string) error {
			out, err := runServiceMutationCombinedOutput(ctx, "userdel", name)
			if err != nil {
				return errors.New(commandFailureDetail("userdel", out, err))
			}
			if _, lookupErr := user.LookupGroup(name); lookupErr == nil {
				if out, err := runServiceMutationCombinedOutput(ctx, "groupdel", name); err != nil {
					return errors.New(commandFailureDetail("groupdel", out, err))
				}
			}
			return nil
		},
		groupMembers: etcGroupMembers,
		prepareDirs:  preparePHPToolDataDirs,
		removeDirs:   func(tool string) error { return os.RemoveAll(phpToolDataDir(tool)) },
		prepareFiles: preparePHPToolFiles,
		applyPool:    services.ApplyManagedSiteFile,
		snapshotFile: snapshotMailFile,
		restoreFile:  restoreMailFile,
		readFile:     os.ReadFile,
		removeFile:   os.Remove,
		fpmTest: func(ctx context.Context, version string) error {
			out, err := serviceMutationCommand(ctx, "php-fpm"+version, "-t").CombinedOutput()
			if err != nil {
				return fmt.Errorf("PHP-FPM %s refused its configuration: %s", version, firstLine(string(out)))
			}
			return nil
		},
		fpmReload: func(ctx context.Context, version string) error {
			out, err := runServiceMutationCombinedOutput(ctx, "systemctl", "reload", "php"+version+"-fpm")
			if err != nil {
				return errors.New(commandFailureDetail("systemctl reload", out, err))
			}
			return nil
		},
		socketReady: pathExists,
		lockPools:   services.LockPHPFPMPools,
		wait: func(ctx context.Context, d time.Duration) bool {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(d):
				return true
			}
		},
	}
}

func lookupSystemAccount(name string) (int, int, bool, error) {
	account, err := user.Lookup(name)
	if err != nil {
		var unknown user.UnknownUserError
		if errors.As(err, &unknown) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	uid, uidErr := strconv.Atoi(account.Uid)
	gid, gidErr := strconv.Atoi(account.Gid)
	if uidErr != nil || gidErr != nil {
		return 0, 0, false, fmt.Errorf("account %s has a non-numeric id", name)
	}
	return uid, gid, true, nil
}

// etcGroupMembers lists a group's supplementary members from /etc/group.
func etcGroupMembers(group string) []string {
	data, err := os.ReadFile(etcGroupPath)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ":")
		if len(fields) == 4 && fields[0] == group {
			members := []string{}
			for _, member := range strings.Split(fields[3], ",") {
				if member = strings.TrimSpace(member); member != "" {
					members = append(members, member)
				}
			}
			return members
		}
	}
	return nil
}

// preparePHPToolDataDirs makes /var/lib/celikpanel-php-tools/<tool>/ with
// sessions/ and tmp/, the account's alone.
func preparePHPToolDataDirs(tool string, uid, gid int) error {
	if err := os.MkdirAll(phpToolDataRoot, 0o755); err != nil {
		return err
	}
	if err := secureSetMailDirectoryMetadata(phpToolDataRoot, 0o755, 0, 0); err != nil {
		return err
	}
	base := phpToolDataDir(tool)
	for _, dir := range []string{base, filepath.Join(base, "sessions"), filepath.Join(base, "tmp")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := secureSetMailDirectoryMetadata(dir, 0o700, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

// preparePHPToolFiles gives a tool's own files to its account and returns
// the undo (nil when nothing is to be put back). servedOwn: the web server
// already hands the tool to its own pool, so the files stay the account's
// whatever happens later in this move (php_tool_files.go).
func preparePHPToolFiles(_ context.Context, tool string, uid, gid int, servedOwn bool) (func(context.Context) error, error) {
	var undo func() error
	var err error
	switch tool {
	case phpToolWebmail:
		undo, err = prepareRoundcubeOwnAccountWith(webmailBaseDir, uid, gid, servedOwn)
	case phpToolPHPMyAdmin:
		undo, err = preparePHPMyAdminOwnAccount(uid, gid, servedOwn)
	}
	if err != nil || undo == nil {
		return nil, err
	}
	return func(context.Context) error { return undo() }, nil
}

// preparePHPMyAdminOwnAccount writes CelikPanel's conf.d file (the cookie
// secret and a temp directory of the account's own) and gives the package's
// secret and the control user's file the account's group while they have the
// package's owner and mode. Every change is recorded first (unless servedOwn)
// so its undo is exact. Also run at each start for phpMyAdmin already on its
// own account: a package upgrade (dbconfig-common) that gave config-db.php
// back to the web group is put right there.
func preparePHPMyAdminOwnAccount(uid, gid int, servedOwn bool) (func() error, error) {
	main, err := os.ReadFile(phpMyAdminMainConfig)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", phpMyAdminMainConfig, err)
	}
	if !strings.Contains(string(main), phpMyAdminConfDir+"/") {
		return nil, fmt.Errorf("%s does not read %s, so phpMyAdmin cannot be given a secret and a temp directory of its own account", phpMyAdminMainConfig, phpMyAdminConfDir)
	}
	var journal *phpToolFileJournal
	if servedOwn {
		if err := clearPHPToolFileJournal(phpToolPHPMyAdmin); err != nil {
			return nil, err
		}
	} else if journal, err = loadPHPToolFileJournal(phpToolPHPMyAdmin); err != nil {
		return nil, err
	}
	note := func(path string, content bool, after phpToolFileMeta) error {
		if journal == nil {
			return nil
		}
		return journal.note([]phpToolPlanned{{Path: path, WithContent: content, After: after}})
	}
	fail := func(err error) (func() error, error) {
		if journal != nil && len(journal.before) > 0 {
			if undoErr := journal.undo(); undoErr != nil {
				return nil, fmt.Errorf("%w; putting back what was changed also failed: %v", err, undoErr)
			}
		}
		return nil, err
	}
	if err := os.MkdirAll(phpMyAdminConfDir, 0o755); err != nil {
		return nil, err
	}
	previous, snapErr := snapshotMailFile(phpMyAdminOwnAccountConf)
	if snapErr != nil {
		return nil, snapErr
	}
	secret := ""
	if previous.exists {
		if match := phpMyAdminOwnSecretLine.FindSubmatch(previous.content); match != nil {
			secret = string(match[1])
		}
	}
	// The owner's own file under CelikPanel's name is kept as it is.
	ownerFile := previous.exists && (secret == "" || string(previous.content) != renderPHPMyAdminOwnAccountConf(secret))
	if !ownerFile {
		if secret == "" {
			if secret, err = randomAlphanumeric(32); err != nil {
				return nil, err
			}
		}
		content := []byte(renderPHPMyAdminOwnAccountConf(secret))
		if !previous.exists || string(previous.content) != string(content) || previous.uid != 0 || previous.gid != gid || previous.mode.Perm() != 0o640 {
			if err := note(phpMyAdminOwnAccountConf, true, phpToolFileMeta{UID: 0, GID: gid, Mode: 0o640, ContentSHA: contentDigest(content)}); err != nil {
				return nil, fmt.Errorf("record phpMyAdmin's files before changing them: %w", err)
			}
			if err := secureWriteConfigOwnedBy(phpMyAdminOwnAccountConf, content, 0o640, 0, uint32(gid)); err != nil {
				return fail(err)
			}
		}
	}
	webGID := webServerGID()
	// The package's own secret and the control user's file, while they have
	// the package's owner and mode (root:<web group> 0640), are given the
	// account's group: Debian's config.inc.php then reads them as before and
	// logs no "Failed to load" line on every request. Any other owner, group
	// or mode is the owner's and is kept.
	for _, path := range []string{phpMyAdminBlowfishSecret, phpMyAdminDBConfig} {
		file, err := snapshotMailFile(path)
		if err != nil || !file.exists || file.uid != 0 || file.gid != webGID || file.mode.Perm() != 0o640 {
			continue
		}
		if err := note(path, false, phpToolFileMeta{UID: 0, GID: gid, Mode: 0o640}); err != nil {
			return fail(fmt.Errorf("record phpMyAdmin's files before changing them: %w", err))
		}
		if err := secureSetMailFileMetadata(path, 0o640, 0, gid); err != nil {
			return fail(err)
		}
	}
	if journal == nil || len(journal.before) == 0 {
		return nil, nil
	}
	return journal.undo, nil
}

// renderPHPMyAdminOwnAccountConf is CelikPanel's conf.d file for phpMyAdmin.
func renderPHPMyAdminOwnAccountConf(secret string) string {
	return fmt.Sprintf(`<?php
// Managed by CelikPanel: phpMyAdmin runs under its own account (%s), which
// does not read the package's secret file. Read after config.inc.php.
$cfg['blowfish_secret'] = '%s';
$cfg['TempDir'] = '%s';
`, phpToolAccount(phpToolPHPMyAdmin), secret, filepath.Join(phpToolDataDir(phpToolPHPMyAdmin), "tmp"))
}

func randomAlphanumeric(n int) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	out := make([]byte, n)
	limit := big.NewInt(int64(len(alphabet)))
	for i := range out {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[index.Int64()]
	}
	return string(out), nil
}

// --- the account step's record ---

// phpToolAccountRecord remembers a failed move of a tool to its own account
// at one PHP version, so the step at each Panel start does not try it again
// on its own; choosing the tool's PHP version in its Manage window does.
type phpToolAccountRecord struct {
	Version int                                `json:"version"`
	Failed  map[string]phpToolAccountRecordRow `json:"failed"`
}

type phpToolAccountRecordRow struct {
	PHPVersion string `json:"php_version"`
	Detail     string `json:"detail"`
	At         string `json:"at"`
}

func readPHPToolAccountFailures() map[string]phpToolAccountRecordRow {
	out := map[string]phpToolAccountRecordRow{}
	data, err := os.ReadFile(phpToolAccountRecordPath)
	if err != nil || len(data) > 32*1024 {
		return out
	}
	var record phpToolAccountRecord
	if json.Unmarshal(data, &record) != nil || record.Version != 1 {
		return out
	}
	for tool, row := range record.Failed {
		if validPHPTool(tool) {
			out[tool] = row
		}
	}
	return out
}

// writePHPToolAccountFailure records (detail != "") or clears a tool's
// failed move.
func writePHPToolAccountFailure(tool, version, detail string) error {
	failed := readPHPToolAccountFailures()
	if detail == "" {
		if _, ok := failed[tool]; !ok {
			return nil
		}
		delete(failed, tool)
	} else {
		if len(detail) > 512 {
			detail = detail[:512]
		}
		failed[tool] = phpToolAccountRecordRow{PHPVersion: version, Detail: detail, At: time.Now().UTC().Format(time.RFC3339)}
	}
	data, err := json.Marshal(phpToolAccountRecord{Version: 1, Failed: failed})
	if err != nil {
		return err
	}
	dir := filepath.Dir(phpToolAccountRecordPath)
	tmp, err := os.CreateTemp(dir, ".php-tool-accounts-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, phpToolAccountRecordPath)
}
