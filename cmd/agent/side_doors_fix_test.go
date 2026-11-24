package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1 fix round (21 Nov 2026): the tests the independent review of PR #348
// required. Each fails (or does not build) on 123ddd738 and passes here.

// --- Finding 1: a PHP-FPM version is never left without a pool ---

func lastPoolHost(t *testing.T) (phpToolHost, string, string) {
	t.Helper()
	host := newPHPToolHost(t)
	restore := services.SetStockPoolsOffRecordPathForTest(filepath.Join(t.TempDir(), "stock-pools-off.json"))
	t.Cleanup(restore)
	for _, version := range []string{"8.3", "8.4"} {
		host.installVersion(t, version, true)
		if err := os.MkdirAll(filepath.Join(host.etc, version, "fpm", "pool.d"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 8.4: the tool's pool is the last one; CelikPanel turned the stock pool off.
	pool := phpToolPoolPath(phpToolPHPMyAdmin, "8.4")
	content := services.SealManagedTextWith(services.ManagedHeaderPrefixINI, phpToolPoolBody(phpToolPHPMyAdmin, "8.4", "www-data"))
	if err := os.WriteFile(pool, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stock := filepath.Join(host.etc, "8.4", "fpm", "pool.d", "www.conf")
	if err := os.WriteFile(stock+services.StockPoolOffSuffix, []byte(stockWWWConf), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := services.SetStockPoolTurnedOff("8.4", true); err != nil {
		t.Fatal(err)
	}
	return host, pool, stock
}

func TestRetiringAVersionsLastPoolPutsTheStockPoolBackFirst(t *testing.T) {
	_, pool, stock := lastPoolHost(t)
	fake := newFakePoolHost()
	if err := retirePHPToolPoolsWith(context.Background(), fake.ops(), phpToolPHPMyAdmin, "8.3"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pool); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the tool's pool on 8.4 stayed")
	}
	if data, err := os.ReadFile(stock); err != nil || string(data) != stockWWWConf {
		t.Fatalf("PHP 8.4 was left with no pool: www.conf %v", err)
	}
	if got := strings.Join(fake.steps, " "); got != "fpm-t:8.4 fpm-reload:8.4" {
		t.Fatalf("steps = %s: the configuration test must run before the reload", got)
	}
	if off, _ := services.StockPoolsTurnedOff(); off["8.4"] {
		t.Fatal("CelikPanel put the pool back itself, so its record must not read it as the owner's undo")
	}
}

func TestRetiringALastPoolThatPHPFPMRefusesPutsEverythingBack(t *testing.T) {
	_, pool, stock := lastPoolHost(t)
	before, _ := os.ReadFile(pool)
	fake := newFakePoolHost()
	fake.testErr = errors.New("ERROR: No pool defined")
	err := retirePHPToolPoolsWith(context.Background(), fake.ops(), phpToolPHPMyAdmin, "8.3")
	if err == nil || !strings.Contains(err.Error(), "was put back") {
		t.Fatalf("err = %v: a refusal is said", err)
	}
	if now, _ := os.ReadFile(pool); string(now) != string(before) {
		t.Fatal("the tool's pool did not come back byte for byte")
	}
	if _, err := os.Stat(stock); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the stock pool stayed on after the undo")
	}
	if off, _ := services.StockPoolsTurnedOff(); !off["8.4"] {
		t.Fatal("the record of the stock pool did not come back")
	}
	for _, step := range fake.steps {
		if step == "fpm-reload:8.4" {
			t.Fatalf("PHP-FPM 8.4 was reloaded into a refused configuration: %v", fake.steps)
		}
	}
}

// --- Finding 7: the pool undo restores bytes; finding 14: no folder left for a freed id ---

func TestOwnPoolUndoRestoresTheEarlierPoolAndRemovesANewAccountsFolder(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	if err := os.MkdirAll(filepath.Join(host.etc, "8.4", "fpm", "pool.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := phpToolPoolPath(phpToolWebmail, "8.4")
	// CelikPanel's own pool, written when the web server's account was nginx.
	earlier := services.SealManagedTextWith(services.ManagedHeaderPrefixINI, phpToolPoolBody(phpToolWebmail, "8.4", "nginx"))
	if err := os.WriteFile(path, []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := newFakePoolHost()
	fake.sockets[phpToolPoolSocket(phpToolWebmail, "8.4")] = true
	_, undo, err := ownPHPToolPoolWith(context.Background(), fake.ops(), phpToolWebmail, "8.4", false)
	if err != nil || undo == nil {
		t.Fatalf("own pool: %v (undo %v)", err, undo != nil)
	}
	if now, _ := os.ReadFile(path); string(now) == earlier {
		t.Fatal("the pool was not rewritten for the new web server account")
	}
	fake.steps = nil
	if err := undo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if now, err := os.ReadFile(path); err != nil || string(now) != earlier {
		t.Fatalf("the undo did not restore the earlier pool (%v): the restored web server configuration names its socket", err)
	}
	want := "fpm-t:8.4 fpm-reload:8.4 files-undo:webmail rmdirs:webmail userdel:celikpanel-webmail"
	if got := strings.Join(fake.steps, " "); got != want {
		t.Fatalf("undo steps = %s\nwant         %s", got, want)
	}
}

// --- Finding 2: Roundcube's tree is not opened to every account ---

func TestRoundcubeOwnAccountPlanChangesOnlyWhatMust(t *testing.T) {
	base := t.TempDir()
	write := func(rel string, mode os.FileMode) {
		t.Helper()
		path := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	write("program/include/iniset.php", 0o644)
	write("public_html/index.php", 0o644)
	write("skins/elastic/styles/styles.min.css", 0o644)
	write("config/config.inc.php", 0o640)
	write("config/defaults.inc.php", 0o644)
	write("db/roundcube.sqlite3", 0o660)
	write("temp/attachment", 0o640)
	write("logs/errors.log", 0o640)
	write("plugins/password/config.inc.php", 0o640)    // the web group read it
	write("plugins/managesieve/config.inc.php", 0o644) // everyone read it
	write("plugins/private/config.inc.php", 0o600)     // the owner closed it
	write("plugins/password/config.inc.php.dist", 0o644)
	write("owner-notes.txt", 0o644) // a top-level entry Roundcube does not ship
	if err := os.Chmod(base, 0o750); err != nil {
		t.Fatal(err)
	}
	gid := os.Getgid() // the web server's group in this test
	plan, notes, err := roundcubeOwnAccountPlan(base, 4242, 4243, gid)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	var order []string
	for _, change := range plan {
		rel, _ := filepath.Rel(base, change.Path)
		got[rel] = strings.TrimSpace(strings.Join([]string{fixItoa(change.UID), fixItoa(change.GID), modeText(change.Mode)}, ":"))
		order = append(order, rel)
	}
	uid := fixItoa(os.Getuid())
	want := map[string]string{
		"db": "4242:4243:0700", "db/roundcube.sqlite3": "4242:4243:0660",
		"temp": "4242:4243:0700", "temp/attachment": "4242:4243:0640",
		"logs": "4242:4243:0700", "logs/errors.log": "4242:4243:0640",
		"config": "0:4243:0750", "config/config.inc.php": uid + ":4243:0640", "config/defaults.inc.php": uid + ":4243:0644",
		"plugins/password/config.inc.php": uid + ":4243:0640",
		// Second fix round (B2): the root stays closed to others; webmail's
		// account alone gets through it by an access-control-list entry.
		".": uid + ":" + fixItoa(gid) + ":0750",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plan:\n%v\nwant:\n%v", got, want)
	}
	for _, rel := range []string{"program/include/iniset.php", "public_html/index.php", "skins/elastic/styles/styles.min.css", "plugins/password/config.inc.php.dist", "plugins/private/config.inc.php"} {
		if _, changed := got[rel]; changed {
			t.Fatalf("%s was changed: the published code and the owner's own mode are kept", rel)
		}
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "plugins/private/config.inc.php is kept as its owner set it (0600)") {
		t.Fatalf("notes %v: the owner's mode is kept and named", notes)
	}
	// The private parts first, the tree's own directory last.
	if order[len(order)-1] != "." || order[0] != "db" {
		t.Fatalf("order %v", order)
	}
	if root := plan[len(plan)-1]; !aclHasUser(root.ACL, 4242, aclExecute) || aclHasUser(root.ACL, 4243, aclExecute) {
		t.Fatal("the root's access-control list does not let webmail's account (and only it) through")
	}
	for _, change := range plan {
		rel, _ := filepath.Rel(base, change.Path)
		private := rel == "config" || rel == "db" || rel == "temp" || rel == "logs" || strings.HasPrefix(rel, "db/") || strings.HasSuffix(rel, "config.inc.php")
		if change.Mode&0o004 != 0 && private {
			t.Fatalf("%s left readable to others", change.Path)
		}
	}
}

func fixItoa(n int) string { return strconv.Itoa(n) }

func modeText(mode os.FileMode) string { return fmt.Sprintf("%04o", uint32(mode.Perm())) }

// --- Finding 4: an interrupted move recovers ---

// The files' undo puts back what the FIRST attempt recorded, decided from
// what serves the tool, not from a file's group.
func TestAnInterruptedMovesRetryPutsBackWhatTheFirstAttemptRecorded(t *testing.T) {
	savedDir, savedSet, savedRestore, savedCurrent := phpToolFilesRecordDir, setPHPToolPathMeta, restorePHPToolFileState, currentPHPToolFileState
	t.Cleanup(func() {
		phpToolFilesRecordDir, setPHPToolPathMeta, restorePHPToolFileState, currentPHPToolFileState = savedDir, savedSet, savedRestore, savedCurrent
	})
	phpToolFilesRecordDir = t.TempDir()
	base := filepath.Join(t.TempDir(), "roundcube")
	for _, dir := range []string{"config", "db", "temp", "logs", "program"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "config", "config.inc.php"), []byte("<?php"), 0o640); err != nil {
		t.Fatal(err)
	}
	var applied []string
	// What the (not made) changes would leave, so the undo sees each path as
	// CelikPanel set it.
	set := map[string][3]int{}
	setPHPToolPathMeta = func(path string, _ bool, uid, gid int, mode os.FileMode) error {
		applied = append(applied, path)
		set[path] = [3]int{uid, gid, int(unixModeBits(mode))}
		return nil
	}
	currentPHPToolFileState = func(path string, content, acl bool) (phpToolFileState, error) {
		state, err := readPHPToolFileState(path, content, acl)
		if meta, ok := set[path]; ok {
			state.UID, state.GID, state.Mode = meta[0], meta[1], uint32(meta[2])
		}
		return state, err
	}
	var restored []phpToolFileState
	restorePHPToolFileState = func(state phpToolFileState) error {
		restored = append(restored, state)
		return nil
	}
	// Attempt 1: the files are given to the account, then the power goes
	// (its undo never runs; the record stays).
	if _, err := prepareRoundcubeOwnAccountWith(base, 4242, 4243, false); err != nil {
		t.Fatal(err)
	}
	first, err := loadPHPToolFileJournal(phpToolWebmail)
	if err != nil || len(first.before) == 0 {
		t.Fatalf("attempt 1 recorded nothing: %v", err)
	}
	// What a real chown would leave: the config file now looks like the
	// account's. The retry must not decide from that.
	// Attempt 2, the web server still on the stock socket: its undo puts
	// back attempt 1's record.
	undo, err := prepareRoundcubeOwnAccountWith(base, 4242, 4243, false)
	if err != nil || undo == nil {
		t.Fatalf("retry: %v (undo %v)", err, undo != nil)
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	paths := func(states []phpToolFileState) []string {
		var out []string
		for _, state := range states {
			out = append(out, state.Path)
		}
		sort.Strings(out)
		return out
	}
	if !reflect.DeepEqual(paths(restored), paths(first.before)) {
		t.Fatalf("restored %v, want attempt 1's record %v", paths(restored), paths(first.before))
	}
	for i, state := range restored {
		for _, original := range first.before {
			if original.Path == state.Path && !reflect.DeepEqual(original, state) {
				t.Fatalf("restore %d %+v differs from the first record %+v", i, state, original)
			}
		}
	}
	if _, err := os.Stat(phpToolFilesRecordPath(phpToolWebmail)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the record stayed after everything came back")
	}
	// Served by the own pool already (a move between versions): no undo.
	restored = nil
	undo, err = prepareRoundcubeOwnAccountWith(base, 4242, 4243, true)
	if err != nil || undo != nil {
		t.Fatalf("a tool already on its own pool keeps its files: undo %v, %v", undo != nil, err)
	}
}

// The interruption table of the verified move (bindPHPToolWith step by step)
// and what the next start does.
func TestInterruptedMoveTable(t *testing.T) {
	savedSettle := phpToolProbeSettle
	phpToolProbeSettle = 10 * time.Millisecond
	t.Cleanup(func() { phpToolProbeSettle = savedSettle })
	own := func(host phpToolHost) string { return phpToolPoolSocket(phpToolPHPPgAdmin, "8.4") }
	stock := func(host phpToolHost) string { return filepath.Join(host.run, "php8.4-fpm.sock") }
	type row struct {
		name    string
		conf    func(phpToolHost) string // the socket the configuration names after the cut
		failed  bool                     // a failure was recorded at an earlier start
		answers []bool                   // the probe's answers, in order
		state   string
		steps   string
	}
	rows := []row{
		{name: "cut before the files changed, or after them but before the web server (files back by the record)", conf: stock,
			answers: []bool{false, true}, state: transport.SideDoorsToolFailed,
			steps: "cli:8.4 own write:new nginx-t reload-nginx probe write:old nginx-t reload-nginx own-undo probe"},
		{name: "same, the retry works", conf: stock, answers: []bool{true}, state: transport.SideDoorsToolMoved,
			steps: "cli:8.4 own write:new nginx-t reload-nginx probe"},
		{name: "a recorded failure while the tool answers is not tried again", conf: stock, failed: true, answers: []bool{true},
			state: transport.SideDoorsToolFailed, steps: "probe"},
		{name: "a recorded failure while the tool does not answer is tried again", conf: stock, failed: true, answers: []bool{false, true},
			state: transport.SideDoorsToolMoved, steps: "probe cli:8.4 own write:new nginx-t reload-nginx probe"},
		{name: "a retry that cannot bring it back reports it down", conf: stock, answers: []bool{false, false},
			state: transport.SideDoorsToolDown, steps: "cli:8.4 own write:new nginx-t reload-nginx probe write:old nginx-t reload-nginx own-undo probe"},
		{name: "cut after the configuration was written, before nginx reloaded it", conf: own, answers: []bool{false, true},
			state: transport.SideDoorsToolOwnAccount, steps: "recheck probe nginx-t reload-nginx probe"},
		{name: "the own pool does not answer after a reload either", conf: own, answers: []bool{false, false, false},
			state: transport.SideDoorsToolDown, steps: "recheck probe nginx-t reload-nginx probe"},
		{name: "cut after the proof (record or old pools left)", conf: own, answers: []bool{true},
			state: transport.SideDoorsToolOwnAccount, steps: "recheck probe"},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			host := newPHPToolHost(t)
			host.installVersion(t, "8.4", true)
			savedRecord := phpToolAccountRecordPath
			phpToolAccountRecordPath = filepath.Join(t.TempDir(), "php-tool-accounts.json")
			t.Cleanup(func() { phpToolAccountRecordPath = savedRecord })
			phpToolInstalledCheck = func(tool string) bool { return tool == phpToolPHPPgAdmin }
			conf := renderDBToolsConfig([]string{phpToolPHPPgAdmin}, map[string]string{phpToolPHPPgAdmin: "/usr/share/phppgadmin"},
				map[string]string{phpToolPHPPgAdmin: tc.conf(host)})
			if err := os.WriteFile(host.dbToolsConf, []byte(conf), 0o644); err != nil {
				t.Fatal(err)
			}
			if tc.failed {
				if err := writePHPToolAccountFailure(phpToolPHPPgAdmin, "8.4", "it did not answer"); err != nil {
					t.Fatal(err)
				}
			}
			bind := newFakeBindHost()
			bind.packages["php8.4-pgsql"], bind.packages["php8.4-mbstring"], bind.packages["php8.4-xml"] = true, true, true
			answers := append([]bool(nil), tc.answers...)
			ops := bind.ops()
			ops.httpProbe = func(context.Context, string, string) error {
				bind.steps = append(bind.steps, "probe")
				ok := len(answers) > 0 && answers[0]
				if len(answers) > 1 {
					answers = answers[1:]
				}
				if ok {
					return nil
				}
				return &phpToolProbeError{Kind: "no_answer", Message: "phpPgAdmin did not answer"}
			}
			ops.render = func(tool, socket string) (string, []byte, error) {
				return "/etc/nginx/conf.d/celikpanel-dbtools.conf", []byte("new"), nil
			}
			ops.ownPool = func(context.Context, string, string, string) (string, func(context.Context) error, error) {
				bind.steps = append(bind.steps, "own")
				return phpToolPoolSocket(phpToolPHPPgAdmin, "8.4"), func(context.Context) error {
					bind.steps = append(bind.steps, "own-undo")
					return nil
				}, nil
			}
			hooks := sideDoorsToolHooks{
				poolUser:   func(string, string) string { return "" },
				webAccount: func() string { return "www-data" },
				recheck: func(context.Context, string, string) error {
					bind.steps = append(bind.steps, "recheck")
					return nil
				},
			}
			tools := closeSideDoorsTools(context.Background(), ops, hooks, nil)
			if len(tools) != 1 || tools[0].State != tc.state {
				t.Fatalf("tools = %+v steps = %v", tools, bind.steps)
			}
			// Probe retries inside the own-pool check are bounded; only the
			// first steps are compared.
			got := strings.Join(bind.steps, " ")
			if !strings.HasPrefix(got, tc.steps) {
				t.Fatalf("steps = %s\nwant    %s…", got, tc.steps)
			}
		})
	}
}

// --- Finding 14: an owner's tool pool that runs as the web server's account ---

func TestAnOwnersToolPoolRunningAsTheWebAccountIsSaidSo(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	phpToolInstalledCheck = func(tool string) bool { return tool == phpToolPHPPgAdmin }
	conf := renderDBToolsConfig([]string{phpToolPHPPgAdmin}, map[string]string{phpToolPHPPgAdmin: "/usr/share/phppgadmin"},
		map[string]string{phpToolPHPPgAdmin: phpToolPoolSocket(phpToolPHPPgAdmin, "8.4")})
	if err := os.WriteFile(host.dbToolsConf, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	bind := newFakeBindHost()
	hooks := sideDoorsToolHooks{
		poolUser:   func(string, string) string { return "www-data" },
		webAccount: func() string { return "www-data" },
		recheck:    func(context.Context, string, string) error { return nil },
	}
	tools := closeSideDoorsTools(context.Background(), bind.ops(), hooks, nil)
	if len(tools) != 1 || tools[0].State != transport.SideDoorsToolOwnerConfig || tools[0].Account != "www-data" || tools[0].Path != phpToolPoolPath(phpToolPHPPgAdmin, "8.4") {
		t.Fatalf("tools = %+v: a pool named after the tool that runs as www-data is not its own account", tools)
	}
}

// --- Finding 3: the owner's undo stands ---

func TestTheOwnersUndoOfTheStockPoolStands(t *testing.T) {
	restore := services.SetStockPoolsOffRecordPathForTest(filepath.Join(t.TempDir(), "stock-pools-off.json"))
	t.Cleanup(restore)
	host, path := newStockHost(t, true)
	ops := host.ops()
	ops.turnedOff = services.StockPoolsTurnedOff
	ops.record = services.SetStockPoolTurnedOff
	if pools := closeStockFPMPoolsWith(context.Background(), ops); pools[0].State != transport.SideDoorsStockTurnedOff {
		t.Fatalf("first start %+v", pools)
	}
	// The owner renames it back.
	if err := os.Rename(path+stockPoolOffSuffix, path); err != nil {
		t.Fatal(err)
	}
	host.steps = nil
	pools := closeStockFPMPoolsWith(context.Background(), ops)
	if pools[0].State != transport.SideDoorsStockOwnerRestored || len(host.steps) != 0 {
		t.Fatalf("after the owner's undo: %+v steps %v: it must not be turned off again by itself", pools, host.steps)
	}
	// The administrator's explicit "turn it off again" (second fix round:
	// scoped to that version, the owner's undo kept until it is off).
	if pools := closeStockFPMPoolsFor(context.Background(), ops, stockPoolPass{only: "8.4", reclose: true}); pools[0].State != transport.SideDoorsStockTurnedOff {
		t.Fatalf("closing again: %+v", pools)
	}
}

func TestTheOwnersDeletionOfAtAllowStands(t *testing.T) {
	fake := withAtFiles(t, testAtDeny, "passwd: files\n")
	if at := closeSiteAtWith(context.Background(), fake.ops(true)); at.State != transport.SideDoorsAtWritten {
		t.Fatalf("first start %+v", at)
	}
	if err := os.Remove(atAllowPath); err != nil {
		t.Fatal(err)
	}
	writes := len(fake.writes)
	at := closeSiteAtWith(context.Background(), fake.ops(true))
	if at.State != transport.SideDoorsAtOwnerRemoved || len(fake.writes) != writes {
		t.Fatalf("after the owner deleted it: %+v writes %v", at, fake.writes)
	}
	if _, err := os.Stat(atAllowPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("at.allow was written again by itself")
	}
	if !strings.Contains(atAllowHeader, "does not write it again by itself") {
		t.Fatal("the header does not tell the truth about deleting it")
	}
	if at := closeSiteAtFor(context.Background(), fake.ops(true), atPass{reclose: true}); at.State != transport.SideDoorsAtWritten {
		t.Fatalf("closing again: %+v", at)
	}
}

// --- Finding 6: accounts sharing a website account's id ---

func TestAtAllowLeavesOutLoginsThatShareASitesID(t *testing.T) {
	fake := withAtFiles(t, testAtDeny, "passwd: files\n")
	// An extra FTP login made with useradd -o: the site's uid, a home outside
	// the hosting root.
	extra := testPasswd + "shop_extra:x:1001:1001::/run/celikpanel-ftp/shop_extra/./files:/usr/sbin/nologin\n"
	if err := os.WriteFile(sideDoorsPasswd, []byte(extra), 0o644); err != nil {
		t.Fatal(err)
	}
	if at := closeSiteAtWith(context.Background(), fake.ops(true)); at.State != transport.SideDoorsAtWritten {
		t.Fatalf("%+v", at)
	}
	data, _ := os.ReadFile(atAllowPath)
	if names := strings.Join(nameLines(string(data)), ","); names != "root,backupsvc,alice" {
		t.Fatalf("at.allow names %q: the login sharing the site's uid is left out", names)
	}
	// An owner's file that names it is flagged like a site account.
	if err := os.WriteFile(atAllowPath, []byte("alice\nshop_extra\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	at := closeSiteAtWith(context.Background(), fake.ops(true))
	if at.State != transport.SideDoorsAtOwnerFile || strings.Join(at.SiteAccounts, ",") != "shop_extra" {
		t.Fatalf("%+v", at)
	}
}

// --- Finding 14: a pending at job's text is not copied ---

func TestPendingAtJobsAreNumberAccountAndTimeOnly(t *testing.T) {
	fake := withAtFiles(t, testAtDeny, "passwd: files\n")
	fake.atqOut = "3\tSat Nov 22 10:00:00 2026 a shop_example_com\n"
	at := closeSiteAtWith(context.Background(), fake.ops(true))
	if len(at.Jobs) != 1 || !reflect.DeepEqual(at.Jobs[0], transport.SideDoorsAtJob{ID: "3", Account: "shop_example_com", When: "Sat Nov 22 10:00:00 2026"}) {
		t.Fatalf("jobs %+v", at.Jobs)
	}
	if _, has := reflect.TypeOf(transport.SideDoorsAtJob{}).FieldByName("Command"); has {
		t.Fatal("the job's command text is still carried")
	}
}

// --- Finding 8: webmail without its own account says so ---

func TestWebmailWithoutItsOwnAccountIsSaidAndNeverPointedAtAMissingSocket(t *testing.T) {
	socket := "/run/php/php8.4-fpm.sock"
	stop, _ := webmailOwnAccountOutcome(socket, "Webmail (Roundcube) could not be given its own account", func(string) bool { return false })
	if stop == "" {
		t.Fatal("webmail would be pointed at a socket that does not exist and reported configured")
	}
	stop, problem := webmailOwnAccountOutcome(socket, "w", func(string) bool { return true })
	if stop != "" || problem != (transport.PHPToolProblem{Tool: phpToolWebmail, Version: "8.4", Kind: "own_account_failed"}) {
		t.Fatalf("stop %q problem %+v: the answer carries the problem", stop, problem)
	}
}
