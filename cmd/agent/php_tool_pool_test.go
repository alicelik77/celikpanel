package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
)

// S4 R1 (20 Nov 2026): the database tools and webmail under their own
// accounts. These fail on alpha.141, where all three ran in the
// distribution's stock pool as the web server's account.

// poolDirective reads one "key = value" line of a rendered pool.
func poolDirective(t *testing.T, body, key string) string {
	t.Helper()
	match := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + ` = (.*)$`).FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("pool has no %s line:\n%s", key, body)
	}
	return match[1]
}

func TestToolPoolsRunUnderTheirOwnAccounts(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	savedData := phpToolDataRoot
	phpToolDataRoot = "/var/lib/celikpanel-php-tools"
	t.Cleanup(func() { phpToolDataRoot = savedData })
	for _, tool := range []string{phpToolWebmail, phpToolPHPMyAdmin, phpToolPHPPgAdmin} {
		body := phpToolPoolBody(tool, "8.4", "www-data")
		account := "celikpanel-" + tool
		if !strings.Contains(body, "["+account+"]\n") {
			t.Fatalf("%s: pool name is not %s:\n%s", tool, account, body)
		}
		for key, want := range map[string]string{
			"user":                               account,
			"group":                              account,
			"listen":                             filepath.Join(host.run, "php8.4-fpm-"+account+".sock"),
			"listen.owner":                       "www-data",
			"listen.group":                       "www-data",
			"listen.mode":                        "0660",
			"php_admin_value[session.save_path]": "/var/lib/celikpanel-php-tools/" + tool + "/sessions",
			"php_admin_value[upload_tmp_dir]":    "/var/lib/celikpanel-php-tools/" + tool + "/tmp",
			"php_admin_value[sys_temp_dir]":      "/var/lib/celikpanel-php-tools/" + tool + "/tmp",
		} {
			if got := poolDirective(t, body, key); got != want {
				t.Fatalf("%s: %s = %q, want %q", tool, key, got, want)
			}
		}
		// The web server's account only owns the socket: no worker runs as it.
		if strings.Contains(poolDirective(t, body, "user")+poolDirective(t, body, "group"), "www-data") {
			t.Fatalf("%s pool still runs as the web server's account", tool)
		}
		if got := fpmSocketVersion(poolDirective(t, body, "listen")); got != "8.4" {
			t.Fatalf("%s: the own socket reads as PHP %q", tool, got)
		}
		if owner, own := phpToolOwnSocket(poolDirective(t, body, "listen")); !own || owner != tool {
			t.Fatalf("%s: own socket not recognised (%q %v)", tool, owner, own)
		}
	}
	if _, own := phpToolOwnSocket("/run/php/php8.4-fpm.sock"); own {
		t.Fatal("the stock socket was read as a tool's own pool")
	}
}

type fakePoolHost struct {
	accounts   map[string][2]int
	members    map[string][]string
	steps      []string
	testErr    error
	reloadErr  error
	prepareErr error
	sockets    map[string]bool
}

func newFakePoolHost() *fakePoolHost {
	return &fakePoolHost{
		accounts: map[string][2]int{"www-data": {33, 33}},
		members:  map[string][]string{},
		sockets:  map[string]bool{},
	}
}

func (h *fakePoolHost) ops() phpToolPoolOps {
	return phpToolPoolOps{
		webServer: func() string { return "www-data" },
		lookupAccount: func(name string) (int, int, bool, error) {
			ids, ok := h.accounts[name]
			return ids[0], ids[1], ok, nil
		},
		createAccount: func(_ context.Context, name string) error {
			h.steps = append(h.steps, "useradd:"+name)
			h.accounts[name] = [2]int{990, 990}
			return nil
		},
		removeAccount: func(_ context.Context, name string) error {
			h.steps = append(h.steps, "userdel:"+name)
			delete(h.accounts, name)
			return nil
		},
		groupMembers: func(group string) []string { return h.members[group] },
		prepareDirs: func(tool string, uid, gid int) error {
			h.steps = append(h.steps, "dirs:"+tool)
			return nil
		},
		prepareFiles: func(_ context.Context, tool string, uid, gid int) (func(context.Context) error, error) {
			h.steps = append(h.steps, "files:"+tool)
			if h.prepareErr != nil {
				return nil, h.prepareErr
			}
			return func(context.Context) error {
				h.steps = append(h.steps, "files-undo:"+tool)
				return nil
			}, nil
		},
		applyPool:  services.ApplyManagedSiteFile,
		readFile:   os.ReadFile,
		removeFile: os.Remove,
		fpmTest: func(_ context.Context, version string) error {
			h.steps = append(h.steps, "fpm-t:"+version)
			return h.testErr
		},
		fpmReload: func(_ context.Context, version string) error {
			h.steps = append(h.steps, "fpm-reload:"+version)
			return h.reloadErr
		},
		socketReady: func(path string) bool { return h.sockets[path] },
		wait:        func(context.Context, time.Duration) bool { return true },
	}
}

func TestOwnPHPToolPoolCreatesTheAccountAndPoolAndUndoesThem(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	if err := os.MkdirAll(filepath.Join(host.etc, "8.4", "fpm", "pool.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := newFakePoolHost()
	fake.sockets[phpToolPoolSocket(phpToolPHPMyAdmin, "8.4")] = true
	socket, undo, err := ownPHPToolPoolWith(context.Background(), fake.ops(), phpToolPHPMyAdmin, "8.4")
	if err != nil {
		t.Fatal(err)
	}
	if socket != filepath.Join(host.run, "php8.4-fpm-celikpanel-phpmyadmin.sock") || undo == nil {
		t.Fatalf("socket = %q undo=%v", socket, undo != nil)
	}
	want := "useradd:celikpanel-phpmyadmin dirs:phpmyadmin files:phpmyadmin fpm-t:8.4 fpm-reload:8.4"
	if got := strings.Join(fake.steps, " "); got != want {
		t.Fatalf("steps = %s\nwant    %s", got, want)
	}
	path := phpToolPoolPath(phpToolPHPMyAdmin, "8.4")
	written, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(written), services.ManagedHeaderPrefixINI) ||
		!strings.Contains(string(written), "user = celikpanel-phpmyadmin\n") {
		t.Fatalf("pool file %s = %q, %v", path, written, err)
	}
	// Called again: the same text, nothing reloaded, nothing to undo.
	fake.steps = nil
	again, undoAgain, err := ownPHPToolPoolWith(context.Background(), fake.ops(), phpToolPHPMyAdmin, "8.4")
	if err != nil || again != socket {
		t.Fatalf("second call = %q, %v", again, err)
	}
	if got := strings.Join(fake.steps, " "); got != "dirs:phpmyadmin files:phpmyadmin" || undoAgain == nil {
		t.Fatalf("second call steps = %s (undo %v)", got, undoAgain != nil)
	}
	// The first call's undo: pool, files, account, in that order.
	fake.steps = nil
	if err := undo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(fake.steps, " "); got != "fpm-reload:8.4 files-undo:phpmyadmin userdel:celikpanel-phpmyadmin" {
		t.Fatalf("undo steps = %s", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the pool is still there after the undo: %v", err)
	}
}

func TestOwnPHPToolPoolRefusesAnAccountInTheWebServersGroup(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	fake := newFakePoolHost()
	fake.accounts["celikpanel-webmail"] = [2]int{991, 991}
	fake.members["www-data"] = []string{"celikpanel-webmail"}
	_, _, err := ownPHPToolPoolWith(context.Background(), fake.ops(), phpToolWebmail, "8.4")
	if err == nil || !strings.Contains(err.Error(), "gpasswd -d celikpanel-webmail www-data") {
		t.Fatalf("err = %v", err)
	}
	if len(fake.steps) != 0 {
		t.Fatalf("something was changed: %v", fake.steps)
	}
}

func TestOwnPHPToolPoolPutsEverythingBackWhenPHPFPMRefusesIt(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	if err := os.MkdirAll(filepath.Join(host.etc, "8.4", "fpm", "pool.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := newFakePoolHost()
	fake.testErr = errors.New("ERROR: [pool celikpanel-phppgadmin] invalid")
	_, _, err := ownPHPToolPoolWith(context.Background(), fake.ops(), phpToolPHPPgAdmin, "8.4")
	if err == nil {
		t.Fatal("a refused pool was accepted")
	}
	if _, statErr := os.Stat(phpToolPoolPath(phpToolPHPPgAdmin, "8.4")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the refused pool stayed: %v", statErr)
	}
	got := strings.Join(fake.steps, " ")
	if !strings.HasSuffix(got, "files-undo:phppgadmin userdel:celikpanel-phppgadmin") {
		t.Fatalf("steps = %s", got)
	}
}

func TestOwnPHPToolPoolKeepsAnOwnerEditedPoolAndUsesItsSocket(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	if err := os.MkdirAll(filepath.Join(host.etc, "8.4", "fpm", "pool.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := newFakePoolHost()
	fake.accounts["celikpanel-webmail"] = [2]int{991, 991}
	path := phpToolPoolPath(phpToolWebmail, "8.4")
	ownerSocket := filepath.Join(host.run, "owner-webmail.sock")
	body := strings.Replace(phpToolPoolBody(phpToolWebmail, "8.4", "www-data"),
		"listen = "+phpToolPoolSocket(phpToolWebmail, "8.4"), "listen = "+ownerSocket, 1) + "pm.max_children = 20\n"
	edited := services.SealManagedTextWith(services.ManagedHeaderPrefixINI, phpToolPoolBody(phpToolWebmail, "8.4", "www-data"))
	edited = strings.Replace(edited, phpToolPoolBody(phpToolWebmail, "8.4", "www-data"), body, 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	fake.sockets[ownerSocket] = true
	socket, _, err := ownPHPToolPoolWith(context.Background(), fake.ops(), phpToolWebmail, "8.4")
	if err != nil || socket != ownerSocket {
		t.Fatalf("socket = %q, %v", socket, err)
	}
	if now, _ := os.ReadFile(path); string(now) != edited {
		t.Fatal("the owner's pool was rewritten")
	}
	if _, err := os.Stat(path + services.SiteFilePendingSuffix); err != nil {
		t.Fatalf("CelikPanel's text is not waiting beside the owner's pool: %v", err)
	}
	for _, step := range fake.steps {
		if strings.HasPrefix(step, "fpm-") {
			t.Fatalf("PHP-FPM was touched for a kept pool: %v", fake.steps)
		}
	}
}

func TestRetirePHPToolPoolsKeepsTheBoundVersion(t *testing.T) {
	host := newPHPToolHost(t)
	for _, version := range []string{"8.3", "8.4"} {
		host.installVersion(t, version, true)
		if err := os.MkdirAll(filepath.Join(host.etc, version, "fpm", "pool.d"), 0o755); err != nil {
			t.Fatal(err)
		}
		content := services.SealManagedTextWith(services.ManagedHeaderPrefixINI, phpToolPoolBody(phpToolPHPPgAdmin, version, "www-data"))
		if err := os.WriteFile(phpToolPoolPath(phpToolPHPPgAdmin, version), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fake := newFakePoolHost()
	if err := retirePHPToolPoolsWith(context.Background(), fake.ops(), phpToolPHPPgAdmin, "8.4"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(phpToolPoolPath(phpToolPHPPgAdmin, "8.3")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the pool on the old version stayed")
	}
	if _, err := os.Stat(phpToolPoolPath(phpToolPHPPgAdmin, "8.4")); err != nil {
		t.Fatal("the bound version's pool went")
	}
	if got := strings.Join(fake.steps, " "); got != "fpm-reload:8.3" {
		t.Fatalf("steps = %s", got)
	}
}

func TestBindPHPToolMovesToItsOwnPoolAndPutsItBackWhenTheProofFails(t *testing.T) {
	own := "/run/php/php8.6-fpm-celikpanel-phpmyadmin.sock"
	newHost := func() (*fakeBindHost, phpToolBindOps) {
		host := newFakeBindHost()
		for _, s := range []string{"mysql", "mbstring", "xml"} {
			host.packages["php8.6-"+s] = true
		}
		ops := host.ops()
		ops.ownPool = func(_ context.Context, tool, version string) (string, func(context.Context) error, error) {
			host.steps = append(host.steps, "own:"+tool+"@"+version)
			return own, func(context.Context) error {
				host.steps = append(host.steps, "own-undo")
				return nil
			}, nil
		}
		ops.afterBound = func(_ context.Context, tool, version string) error {
			host.steps = append(host.steps, "after:"+version)
			return nil
		}
		return host, ops
	}

	host, ops := newHost()
	if _, err := bindPHPToolWith(context.Background(), ops, phpToolPHPMyAdmin, "8.6"); err != nil {
		t.Fatal(err)
	}
	want := "cli:8.6 own:phpmyadmin@8.6 write:new:" + own + " nginx-t reload-nginx probe after:8.6"
	if got := strings.Join(host.steps, " "); got != want {
		t.Fatalf("steps = %s\nwant    %s", got, want)
	}

	host, ops = newHost()
	host.probeErr = &phpToolProbeError{Kind: "http_status", Status: 502, Message: "phpMyAdmin answered HTTP 502"}
	_, err := bindPHPToolWith(context.Background(), ops, phpToolPHPMyAdmin, "8.6")
	if err == nil {
		t.Fatal("a failed proof was accepted")
	}
	// nginx gets its previous configuration first, then the pool and files go.
	want = "cli:8.6 own:phpmyadmin@8.6 write:new:" + own + " nginx-t reload-nginx probe write:old nginx-t reload-nginx own-undo"
	if got := strings.Join(host.steps, " "); got != want {
		t.Fatalf("steps = %s\nwant    %s", got, want)
	}
}

func TestBindPHPToolChangesNothingWhenTheOwnAccountCannotBeMade(t *testing.T) {
	host := newFakeBindHost()
	for _, s := range []string{"mysql", "mbstring", "xml"} {
		host.packages["php8.6-"+s] = true
	}
	ops := host.ops()
	ops.ownPool = func(context.Context, string, string) (string, func(context.Context) error, error) {
		return "", nil, errors.New("the account celikpanel-phpmyadmin is a member of the web server's group www-data")
	}
	_, err := bindPHPToolWith(context.Background(), ops, phpToolPHPMyAdmin, "8.6")
	var typed *phpToolBindError
	if !errors.As(err, &typed) || typed.Code != errCodePHPToolAccountFailed || typed.Cause.Kind != "own_account_failed" {
		t.Fatalf("err = %v", err)
	}
	for _, step := range host.steps {
		if strings.HasPrefix(step, "write:") || strings.HasPrefix(step, "reload-nginx") {
			t.Fatalf("the web server was touched: %v", host.steps)
		}
	}
}

func TestPHPMyAdminOwnAccountConfiguration(t *testing.T) {
	savedData := phpToolDataRoot
	phpToolDataRoot = "/var/lib/celikpanel-php-tools"
	t.Cleanup(func() { phpToolDataRoot = savedData })
	secret, err := randomAlphanumeric(32)
	if err != nil {
		t.Fatal(err)
	}
	conf := renderPHPMyAdminOwnAccountConf(secret)
	match := phpMyAdminOwnSecretLine.FindStringSubmatch(conf)
	if match == nil || match[1] != secret {
		t.Fatalf("secret line not readable back:\n%s", conf)
	}
	if !strings.Contains(conf, "$cfg['TempDir'] = '/var/lib/celikpanel-php-tools/phpmyadmin/tmp';") {
		t.Fatalf("no temp directory of its own:\n%s", conf)
	}
	// A configuration that does not read conf.d is refused before anything is written.
	dir := t.TempDir()
	savedMain, savedConfD := phpMyAdminMainConfig, phpMyAdminConfDir
	phpMyAdminMainConfig, phpMyAdminConfDir = filepath.Join(dir, "config.inc.php"), filepath.Join(dir, "conf.d")
	t.Cleanup(func() { phpMyAdminMainConfig, phpMyAdminConfDir = savedMain, savedConfD })
	if err := os.WriteFile(phpMyAdminMainConfig, []byte("<?php\n$cfg['Servers'][1]['host'] = 'localhost';\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := preparePHPMyAdminOwnAccount(990, 990); err == nil || !strings.Contains(err.Error(), "does not read") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(phpMyAdminConfDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("something was written for a configuration that does not read conf.d")
	}
}

func TestToolAccountFailureRecord(t *testing.T) {
	saved := phpToolAccountRecordPath
	phpToolAccountRecordPath = filepath.Join(t.TempDir(), "php-tool-accounts.json")
	t.Cleanup(func() { phpToolAccountRecordPath = saved })
	if err := writePHPToolAccountFailure(phpToolWebmail, "8.4", "it did not answer"); err != nil {
		t.Fatal(err)
	}
	if row := readPHPToolAccountFailures()[phpToolWebmail]; row.PHPVersion != "8.4" || row.Detail != "it did not answer" {
		t.Fatalf("row = %+v", row)
	}
	if err := writePHPToolAccountFailure(phpToolWebmail, "", ""); err != nil {
		t.Fatal(err)
	}
	if len(readPHPToolAccountFailures()) != 0 {
		t.Fatal("the failure was not cleared")
	}
}

// With the stock pool off (side_doors.go), a version still runs through its
// site and tool pools: the tools' default PHP, the readiness of a move and
// the proof's candidates read that, not the stock socket alone.
func TestAVersionWithItsStockPoolOffStillRuns(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", false)
	host.installVersion(t, "8.3", false)
	if phpFPMRunning("8.4") || phpFPMSocketReady(versionedFPMSocketPath("8.4")) {
		t.Fatal("a version with no socket at all reads as running")
	}
	if err := os.WriteFile(filepath.Join(host.run, "php8.4-fpm-site12.sock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(host.run, "php8.3-fpm-celikpanel-webmail.sock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !phpFPMRunning("8.4") || !phpFPMSocketReady(versionedFPMSocketPath("8.4")) || !phpFPMRunning("8.3") {
		t.Fatal("a version listening only on a site's or a tool's pool reads as stopped")
	}
	if got := versionedFPMSockets(); len(got) != 2 || got[0] != versionedFPMSocketPath("8.4") || got[1] != versionedFPMSocketPath("8.3") {
		t.Fatalf("versionedFPMSockets = %v", got)
	}
	if got := defaultFPMSocket(phpToolPHPPgAdmin); got != versionedFPMSocketPath("8.4") {
		t.Fatalf("defaultFPMSocket = %q", got)
	}
}
