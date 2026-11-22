package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1, second fix round (22 Nov 2026): the findings of the second
// independent review of PR #348. Each test fails on 193b87001 (or does not
// build there).

// --- B1: the "in use" question fails only in the safe direction ---

// Any mention of the socket outside a full-line comment, or a *_pass whose
// address is a variable, keeps the pool on; nginx's grammar is never read to
// decide "not used". The cases the second review found (a customer's
// gzip_types text with a mid-word # and quote that made the earlier reader
// lose the owner's fastcgi_pass; set and map) fail on 193b87001.
func TestTheStockPoolInUseCheckFailsOnlyInTheSafeDirection(t *testing.T) {
	listen := "/run/php/php8.4-fpm.sock"
	customer := "# configuration file /etc/nginx/celikpanel/site3/server.conf:\n"
	owner := "# configuration file /etc/nginx/sites-enabled/owner.conf:\nserver {\n"
	ownerFile := []string{"/etc/nginx/sites-enabled/owner.conf"}
	cases := []struct {
		name string
		dump string
		used []string
	}{
		{"a customer's mid-word # and quote before the owner's fastcgi_pass",
			customer + "gzip_types a#\"b\nc\";\n" + owner +
				"    location ~ \\.php$ { fastcgi_pass unix:/run/php/php8.4-fpm.sock; }\n}\n" +
				"# configuration file /etc/nginx/x.conf:\nadd_header A \"b\";\n",
			ownerFile},
		{"set, then the variable", owner + "    set $s unix:/run/php/php8.4-fpm.sock;\n    fastcgi_pass $s;\n}\n", ownerFile},
		{"a map", "# configuration file /etc/nginx/conf.d/map.conf:\nmap $host $sock {\n    default unix:/var/run/php/php8.4-fpm.sock;\n}\n",
			[]string{"/etc/nginx/conf.d/map.conf"}},
		{"a quoted argument", owner + "    fastcgi_pass \"unix:/run/php/php8.4-fpm.sock\";\n}\n", ownerFile},
		{"an address built from a variable", owner + "    fastcgi_pass\n        unix:/run/php/php$v-fpm.sock;\n}\n", ownerFile},
		{"a word with # before the owner's fastcgi_pass", owner + "    set $x a#b; fastcgi_pass unix:/run/php/php8.4-fpm.sock;\n}\n", ownerFile},
		{"a customer's harmless header naming the socket", customer + "add_header X-Note \"unix:/run/php/php8.4-fpm.sock\";\n",
			[]string{"/etc/nginx/celikpanel/site3/server.conf"}},
		{"only full-line comments", owner + "    # fastcgi_pass unix:/run/php/php8.4-fpm.sock;\n\t# set $s x; fastcgi_pass $s;\n}\n", nil},
		{"nothing names it", owner + "    fastcgi_pass unix:/run/php/php8.4-fpm-site3.sock;\n}\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stockPoolUsers(tc.dump, listen, "", nil)
			if len(got) != len(tc.used) || (len(got) > 0 && !reflect.DeepEqual(got, tc.used)) {
				t.Fatalf("users %v, want %v", got, tc.used)
			}
		})
	}
}

// --- B6: one lock around a version's pools for the tool and site paths ---

// The last site of a version is deleted while a tool's pool leaves that
// version, with the version's stock pool off. Without one lock both see
// "another pool remains" and the version is left with none. The site's
// deletion is started from inside the tool's removal and given time to run;
// with the lock it waits, then finds no other pool and puts the stock pool
// back. Does not build on 193b87001 (no shared lock).
func TestTheLastSiteDeletedWhileAToolMovesOffLeavesThatVersionAPool(t *testing.T) {
	host, toolPool, stock := lastPoolHost(t)
	poolDir := filepath.Dir(toolPool)
	sitePool := filepath.Join(poolDir, "site5.conf")
	if err := os.WriteFile(sitePool, []byte("[site5]\nuser = shop\ngroup = shop\nlisten = /run/php/php8.4-fpm-site5.sock\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pools := func() int {
		entries, _ := os.ReadDir(poolDir)
		count := 0
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".conf") {
				count++
			}
		}
		return count
	}
	var mu sync.Mutex
	reloadsWithoutPool := 0
	check := func(string) error {
		if pools() == 0 {
			return errors.New("ERROR: No pool defined")
		}
		return nil
	}
	reload := func(string) error {
		if pools() == 0 {
			mu.Lock()
			reloadsWithoutPool++
			mu.Unlock()
			return errors.New("PHP-FPM failed: no pool")
		}
		return nil
	}
	siteInTest := make(chan struct{})
	toolRemoved := make(chan struct{})
	var siteOnce sync.Once
	siteCheck := func(version string) error {
		siteOnce.Do(func() { close(siteInTest) })
		select {
		case <-toolRemoved:
		case <-time.After(500 * time.Millisecond):
		}
		return check(version)
	}
	t.Cleanup(services.SetPHPFPMForTest(host.etc, siteCheck, reload))
	siteDone := make(chan error, 1)
	fake := newFakePoolHost()
	ops := fake.ops()
	ops.lockPools = services.LockPHPFPMPools
	var started sync.Once
	ops.removeFile = func(path string) error {
		started.Do(func() {
			go func() { siteDone <- services.NewPHPPoolManager().DeletePoolByName("8.4", "site5") }()
			select {
			case <-siteInTest:
			case <-time.After(500 * time.Millisecond):
			}
		})
		err := os.Remove(path)
		select {
		case <-toolRemoved:
		default:
			close(toolRemoved)
		}
		return err
	}
	ops.fpmTest = func(_ context.Context, version string) error { return check(version) }
	ops.fpmReload = func(_ context.Context, version string) error { return reload(version) }
	toolErr := retirePHPToolPoolsWith(context.Background(), ops, phpToolPHPMyAdmin, "8.3")
	var siteErr error
	select {
	case siteErr = <-siteDone:
	case <-time.After(10 * time.Second):
		t.Fatal("the site's deletion never finished")
	}
	if toolErr != nil || siteErr != nil {
		t.Fatalf("tool %v, site %v", toolErr, siteErr)
	}
	if reloadsWithoutPool > 0 || pools() == 0 {
		t.Fatalf("PHP 8.4 was left without a pool (%d reloads saw none, %d pools now)", reloadsWithoutPool, pools())
	}
	if _, err := os.Stat(stock); err != nil {
		t.Fatalf("the distribution's pool was not put back: %v", err)
	}
	if _, err := os.Stat(sitePool); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the site's pool stayed")
	}
}

// --- B5: record first, then act; a crash between reads as CelikPanel's step ---

func recordedStockHost(t *testing.T) (*fakeStockHost, string, stockPoolOps) {
	t.Helper()
	t.Cleanup(services.SetStockPoolsOffRecordPathForTest(filepath.Join(t.TempDir(), "stock-pools-off.json")))
	host, path := newStockHost(t, true)
	ops := host.ops()
	ops.turnedOff = services.StockPoolsTurnedOff
	ops.turning = services.StockPoolsTurningOff
	ops.record = services.SetStockPoolTurnedOff
	ops.pending = services.SetStockPoolTurningOff
	ops.forgetPending = services.ForgetStockPoolTurningOff
	return host, path, ops
}

func TestTheStockPoolIsMarkedBeforeItsRename(t *testing.T) {
	_, _, ops := recordedStockHost(t)
	rename := ops.rename
	marked := false
	ops.rename = func(from, to string) error {
		turning, _ := services.StockPoolsTurningOff()
		marked = turning["8.4"]
		return rename(from, to)
	}
	if pools := closeStockFPMPoolsWith(context.Background(), ops); pools[0].State != transport.SideDoorsStockTurnedOff {
		t.Fatalf("%+v", pools)
	}
	if !marked {
		t.Fatal("the pool was renamed before CelikPanel's record said so")
	}
	if turning, _ := services.StockPoolsTurningOff(); turning["8.4"] {
		t.Fatal("the mark stayed after the step finished")
	}
}

func TestACutTurnOffOfTheStockPoolIsFinishedNotTakenAsTheOwnersUndo(t *testing.T) {
	t.Run("cut after the rename", func(t *testing.T) {
		host, path, ops := recordedStockHost(t)
		if err := services.SetStockPoolTurningOff("8.4"); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path, path+stockPoolOffSuffix); err != nil {
			t.Fatal(err)
		}
		pools := closeStockFPMPoolsWith(context.Background(), ops)
		if len(pools) != 1 || pools[0].State != transport.SideDoorsStockTurnedOff || !strings.Contains(pools[0].Detail, "finished now") {
			t.Fatalf("%+v", pools)
		}
		if got := strings.Join(host.steps, " "); got != "fpm-t:8.4 fpm-reload:8.4" {
			t.Fatalf("steps %s: the check and reload the cut never made run now", got)
		}
		if off, _ := services.StockPoolsTurnedOff(); !off["8.4"] {
			t.Fatal("not recorded off")
		}
	})
	t.Run("cut before the rename", func(t *testing.T) {
		_, _, ops := recordedStockHost(t)
		if err := services.SetStockPoolTurningOff("8.4"); err != nil {
			t.Fatal(err)
		}
		pools := closeStockFPMPoolsWith(context.Background(), ops)
		if len(pools) != 1 || pools[0].State != transport.SideDoorsStockTurnedOff {
			t.Fatalf("%+v: CelikPanel's own unfinished step is decided again, never read as the owner's undo", pools)
		}
	})
	t.Run("the administrator's turn-off-again cut before the rename", func(t *testing.T) {
		host, _, ops := recordedStockHost(t)
		if err := services.SetStockPoolTurnedOff("8.4", true); err != nil {
			t.Fatal(err)
		}
		if err := services.SetStockPoolTurningOff("8.4"); err != nil {
			t.Fatal(err)
		}
		pools := closeStockFPMPoolsWith(context.Background(), ops)
		if len(pools) != 1 || pools[0].State != transport.SideDoorsStockOwnerRestored || len(host.steps) != 0 {
			t.Fatalf("%+v steps %v: the owner's undo still stands", pools, host.steps)
		}
	})
}

// --- B4: "turn it off again" is that one pool, and never arms a later close ---

func TestTurningOneStockPoolOffAgainTouchesOnlyItAndNeverArmsALaterClose(t *testing.T) {
	host, path, ops := recordedStockHost(t)
	ops.versions = func() []string { return []string{"8.3", "8.4"} }
	if err := services.SetStockPoolTurnedOff("8.4", true); err != nil {
		t.Fatal(err)
	}
	// The owner put it back; something uses it now.
	host.dump = "# configuration file /etc/nginx/sites-enabled/owner.conf:\n    fastcgi_pass unix:/run/php/php8.4-fpm.sock;\n"
	pools := closeStockFPMPoolsFor(context.Background(), ops, stockPoolPass{only: "8.4", reclose: true})
	if len(pools) != 1 || pools[0].Version != "8.4" || pools[0].State != transport.SideDoorsStockInUse || len(host.steps) != 0 {
		t.Fatalf("%+v steps %v", pools, host.steps)
	}
	// Refused by PHP-FPM: put back, and the owner's undo still recorded.
	host.dump = ""
	host.testErr = errors.New("ERROR: refused")
	pools = closeStockFPMPoolsFor(context.Background(), ops, stockPoolPass{only: "8.4", reclose: true})
	if pools[0].State != transport.SideDoorsStockFailed {
		t.Fatalf("%+v", pools)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("not put back")
	}
	host.testErr, host.steps = nil, nil
	pools = closeStockFPMPoolsWith(context.Background(), ops)
	if len(pools) != 1 || pools[0].State != transport.SideDoorsStockOwnerRestored || len(host.steps) != 0 {
		t.Fatalf("%+v steps %v: a cut or refused turn-off-again made the next start turn it off by itself", pools, host.steps)
	}
}

func TestACutAtAllowWriteIsCelikPanelsUnfinishedStep(t *testing.T) {
	fake := withAtFiles(t, testAtDeny, "passwd: files\n")
	ops := fake.ops(true)
	write := ops.writeLike
	recordFirst := false
	ops.writeLike = func(path, like string, data []byte) error {
		record, _ := os.ReadFile(atAllowRecordPath)
		recordFirst = strings.HasPrefix(string(record), "pending ")
		return write(path, like, data)
	}
	if at := closeSiteAtWith(context.Background(), ops); at.State != transport.SideDoorsAtWritten || !recordFirst {
		t.Fatalf("%+v record first %v", at, recordFirst)
	}
	content, _ := os.ReadFile(atAllowPath)
	digest := atDigest(content)
	// A crash after the file and before the finished record.
	if err := os.WriteFile(atAllowRecordPath, []byte("pending "+digest+" -\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if at := closeSiteAtWith(context.Background(), ops); at.State != transport.SideDoorsAtCurrent {
		t.Fatalf("%+v: CelikPanel's own file was taken as the owner's", at)
	}
	if record, _ := os.ReadFile(atAllowRecordPath); string(record) != digest+"\n" {
		t.Fatalf("record %q not finished", record)
	}
	// A crash after the record and before a first file.
	if err := os.Remove(atAllowPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(atAllowRecordPath, []byte("pending "+digest+" -\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if at := closeSiteAtWith(context.Background(), ops); at.State != transport.SideDoorsAtWritten {
		t.Fatalf("%+v: a first write that never happened was taken as the owner's undo", at)
	}
}

func TestClosingAtAgainThatFailsKeepsTheOwnersUndo(t *testing.T) {
	fake := withAtFiles(t, testAtDeny, "passwd: files\n")
	ops := fake.ops(true)
	if at := closeSiteAtWith(context.Background(), ops); at.State != transport.SideDoorsAtWritten {
		t.Fatalf("%+v", at)
	}
	if err := os.Remove(atAllowPath); err != nil {
		t.Fatal(err)
	}
	ops.writeLike = func(string, string, []byte) error { return errors.New("read-only file system") }
	if at := closeSiteAtFor(context.Background(), ops, atPass{reclose: true}); at.State != transport.SideDoorsAtFailed {
		t.Fatalf("%+v", at)
	}
	if at := closeSiteAtWith(context.Background(), fake.ops(true)); at.State != transport.SideDoorsAtOwnerRemoved {
		t.Fatalf("%+v: a failed close-again made a later start write it by itself", at)
	}
}

// --- B2: no access-control lists, no move; nothing changed ---

func TestWebmailStaysOnTheWebAccountWhereTheFileSystemRefusesAccessControlLists(t *testing.T) {
	savedDir, savedSet, savedRead := phpToolFilesRecordDir, setPHPToolPathMeta, readPathACL
	t.Cleanup(func() { phpToolFilesRecordDir, setPHPToolPathMeta, readPathACL = savedDir, savedSet, savedRead })
	phpToolFilesRecordDir = t.TempDir()
	base := filepath.Join(t.TempDir(), "roundcube")
	for _, dir := range []string{"config", "db", "temp", "logs"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o770); err != nil {
			t.Fatal(err)
		}
	}
	changed := 0
	setPHPToolPathMeta = func(string, bool, int, int, os.FileMode) error { changed++; return nil }
	readPathACL = func(path string) ([]byte, error) { return nil, errors.Join(errors.New(path), errACLUnsupported) }
	undo, err := prepareRoundcubeOwnAccountWith(base, 4242, 4243, false)
	var refused *errWebmailNoACL
	if !errors.As(err, &refused) || undo != nil {
		t.Fatalf("err %v: the move must stop with the reason", err)
	}
	if changed != 0 {
		t.Fatalf("%d paths changed: nothing may change, and the root is never opened instead", changed)
	}
	if _, err := os.Stat(phpToolFilesRecordPath(phpToolWebmail)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a record was written for a move that changed nothing")
	}
	if !strings.Contains(err.Error(), "acl mount option") || !strings.Contains(err.Error(), "stays on the web server's account") {
		t.Fatalf("the reason does not say what the owner can do: %v", err)
	}
}

// --- B10: the undo never puts an earlier state over a path changed since ---

func TestTheUndoKeepsAPathChangedSinceAndNamesIt(t *testing.T) {
	savedDir, savedSet, savedRestore, savedCurrent := phpToolFilesRecordDir, setPHPToolPathMeta, restorePHPToolFileState, currentPHPToolFileState
	t.Cleanup(func() {
		phpToolFilesRecordDir, setPHPToolPathMeta, restorePHPToolFileState, currentPHPToolFileState = savedDir, savedSet, savedRestore, savedCurrent
	})
	phpToolFilesRecordDir = t.TempDir()
	base := filepath.Join(t.TempDir(), "roundcube")
	for _, dir := range []string{"config", "db", "temp", "logs"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o770); err != nil {
			t.Fatal(err)
		}
	}
	set := map[string][3]int{}
	setPHPToolPathMeta = func(path string, _ bool, uid, gid int, mode os.FileMode) error {
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
	var restored []string
	restorePHPToolFileState = func(state phpToolFileState) error {
		restored = append(restored, filepath.Base(state.Path))
		return nil
	}
	undo, err := prepareRoundcubeOwnAccountWith(base, 4242, 4243, false)
	if err != nil || undo == nil {
		t.Fatalf("move: %v", err)
	}
	// A Roundcube update (or the owner) changes temp/ after CelikPanel did.
	set[filepath.Join(base, "temp")] = [3]int{1, 1, 0o755}
	err = undo()
	var kept *errChangedOutsideTheMove
	if !errors.As(err, &kept) || len(kept.paths) != 1 || kept.paths[0] != filepath.Join(base, "temp") {
		t.Fatalf("err %v: the changed path is kept and named", err)
	}
	for _, name := range restored {
		if name == "temp" {
			t.Fatal("an earlier state was put over a path that changed since")
		}
	}
	if len(restored) == 0 {
		t.Fatal("the paths still as CelikPanel set them were not put back")
	}
}

// --- C5: set-group-id and sticky bits are kept in every change and undo ---

func TestSetGroupIDAndStickyBitsAreKept(t *testing.T) {
	base := filepath.Join(t.TempDir(), "roundcube")
	for _, dir := range []string{"config", "db", "temp", "logs"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o770); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(base, "db"), 0o770|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(base, "temp"), 0o770|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(base, "db"))
	if info.Mode()&os.ModeSetgid == 0 {
		t.Skip("this file system does not keep set-group-id on a directory")
	}
	plan, _, err := roundcubeOwnAccountPlan(base, 4242, 4243, -1)
	if err != nil {
		t.Fatal(err)
	}
	modes := map[string]os.FileMode{}
	for _, change := range plan {
		modes[filepath.Base(change.Path)] = change.Mode
	}
	if modes["db"]&os.ModeSetgid == 0 || modes["temp"]&os.ModeSticky == 0 {
		t.Fatalf("plan modes %v: the special bits were dropped", modes)
	}
	state, err := readPHPToolFileState(filepath.Join(base, "db"), false, false)
	if err != nil || state.Mode&0o2000 == 0 || fileModeOfBits(state.Mode)&os.ModeSetgid == 0 {
		t.Fatalf("recorded %o, %v: the undo would drop set-group-id", state.Mode, err)
	}
}

// --- B2 (low): a walk that ends early is a failure, never a shorter plan ---

func TestARoundcubeWalkThatEndsEarlyIsAFailure(t *testing.T) {
	base := filepath.Join(t.TempDir(), "roundcube")
	for _, rel := range []string{"config", "db", "temp", "logs"} {
		if err := os.MkdirAll(filepath.Join(base, rel), 0o770); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"temp/a", "temp/b"} {
		if err := os.WriteFile(filepath.Join(base, rel), []byte("x"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	saved := roundcubeWalkTree
	t.Cleanup(func() { roundcubeWalkTree = saved })
	// temp/a vanishes between the listing and its read; temp/b must still be
	// in the plan.
	roundcubeWalkTree = func(root string, fn filepath.WalkFunc) error {
		return saved(root, func(path string, info os.FileInfo, err error) error {
			if filepath.Base(path) == "a" && filepath.Base(root) == "temp" {
				return fn(path, nil, &os.PathError{Op: "lstat", Path: path, Err: os.ErrNotExist})
			}
			return fn(path, info, err)
		})
	}
	plan, _, err := roundcubeOwnAccountPlan(base, 4242, 4243, -1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, change := range plan {
		if filepath.Base(change.Path) == "b" {
			found = true
		}
	}
	if !found {
		t.Fatal("a vanished file ended the walk early and the plan was shorter")
	}
	// Any other error ends the plan as a failure.
	roundcubeWalkTree = func(root string, fn filepath.WalkFunc) error {
		return saved(root, func(path string, info os.FileInfo, err error) error {
			if filepath.Base(root) == "temp" && path != root {
				return fn(path, nil, &os.PathError{Op: "open", Path: path, Err: os.ErrPermission})
			}
			return fn(path, info, err)
		})
	}
	if _, _, err := roundcubeOwnAccountPlan(base, 4242, 4243, -1); err == nil {
		t.Fatal("a walk that failed was taken as a complete plan")
	}
}

// --- B3: a database tool without its own account is never pointed at nothing ---

func TestADatabaseToolThatCannotGetItsAccountIsNeverPointedAtAMissingSocket(t *testing.T) {
	stock := "/run/php/php8.4-fpm.sock"
	missing := func(string) bool { return false }
	if !dbToolLeftOut(stock, "phpMyAdmin could not be given its own account", missing) {
		t.Fatal("a tool whose stock socket does not exist would be pointed at it")
	}
	if dbToolLeftOut(stock, "phpMyAdmin could not be given its own account", func(string) bool { return true }) {
		t.Fatal("a tool whose stock pool answers is left out")
	}
	if dbToolLeftOut(stock, "", missing) {
		t.Fatal("a tool that got its own account is left out")
	}
	served := dbToolsServed([]string{phpToolPHPMyAdmin, phpToolPHPPgAdmin}, map[string]bool{phpToolPHPMyAdmin: true})
	conf := renderDBToolsConfig(served, map[string]string{phpToolPHPMyAdmin: "/usr/share/phpmyadmin", phpToolPHPPgAdmin: "/usr/share/phppgadmin"},
		map[string]string{phpToolPHPPgAdmin: "/run/php/php8.4-fpm-celikpanel-phppgadmin.sock"})
	if strings.Contains(conf, "/dbtool/phpmyadmin/") || strings.Contains(conf, "unix:"+stock+";") || !strings.Contains(conf, "/dbtool/phppgadmin/") {
		t.Fatalf("configuration:\n%s", conf)
	}
}
