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
