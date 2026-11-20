package main

import (
	"context"
	"crypto/md5" //nolint:gosec // dpkg's conffile digest
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1 (20 Nov 2026): the distribution's stock PHP-FPM pool is turned off
// only when it is the package's untouched file, nothing hands PHP to it and
// another pool remains; and the tools move to their own accounts at each
// Panel start through the verified step. These fail on alpha.141, which had
// neither.

const stockWWWConf = "[www]\nuser = www-data\ngroup = www-data\nlisten = /run/php/php8.4-fpm.sock\nlisten.owner = www-data\npm = dynamic\n"

type fakeStockHost struct {
	packaged string
	known    bool
	dump     string
	dumpErr  error
	alias    string
	apache   []string
	testErr  error
	active   bool
	steps    []string
}

func newStockHost(t *testing.T, withSitePool bool) (*fakeStockHost, string) {
	t.Helper()
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	poolDir := filepath.Join(host.etc, "8.4", "fpm", "pool.d")
	if err := os.MkdirAll(poolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(poolDir, "www.conf"), []byte(stockWWWConf), 0o644); err != nil {
		t.Fatal(err)
	}
	if withSitePool {
		if err := os.WriteFile(filepath.Join(poolDir, "site7.conf"), []byte("[site7]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sum := md5.Sum([]byte(stockWWWConf)) //nolint:gosec
	return &fakeStockHost{packaged: hex.EncodeToString(sum[:]), known: true, active: true,
		dump: "# configuration file /etc/nginx/nginx.conf:\nhttp {\n}\n# configuration file /etc/nginx/conf.d/celikpanel-dbtools.conf:\n    fastcgi_pass unix:/run/php/php8.4-fpm-celikpanel-phpmyadmin.sock;\n"}, filepath.Join(poolDir, "www.conf")
}

func (h *fakeStockHost) ops() stockPoolOps {
	return stockPoolOps{
		versions: func() []string { return []string{"8.4"} },
		readFile: os.ReadFile,
		conffileMD5: func(context.Context, string, string) (string, bool) {
			return h.packaged, h.known
		},
		nginxDump: func(context.Context) (string, error) { return h.dump, h.dumpErr },
		apacheRefs: func(string) []string {
			return h.apache
		},
		aliasFor: func(string) string { return h.alias },
		rename: func(from, to string) error {
			h.steps = append(h.steps, "rename:"+filepath.Base(from)+">"+filepath.Base(to))
			return os.Rename(from, to)
		},
		fpmTest: func(_ context.Context, version string) error {
			h.steps = append(h.steps, "fpm-t:"+version)
			return h.testErr
		},
		fpmActive: func(string) bool { return h.active },
		fpmReload: func(_ context.Context, version string) error {
			h.steps = append(h.steps, "fpm-reload:"+version)
			return nil
		},
	}
}

func TestStockPoolTurnedOffWhenUntouchedAndUnused(t *testing.T) {
	host, path := newStockHost(t, true)
	pools := closeStockFPMPoolsWith(context.Background(), host.ops())
	if len(pools) != 1 || pools[0].State != transport.SideDoorsStockTurnedOff {
		t.Fatalf("pools = %+v", pools)
	}
	if got := strings.Join(host.steps, " "); got != "rename:www.conf>www.conf.off-by-celikpanel fpm-t:8.4 fpm-reload:8.4" {
		t.Fatalf("steps = %s", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("www.conf is still read by PHP-FPM")
	}
	if data, err := os.ReadFile(path + stockPoolOffSuffix); err != nil || string(data) != stockWWWConf {
		t.Fatalf("the package's file is not kept beside it: %v", err)
	}
	// The next start finds it off and changes nothing.
	host.steps = nil
	pools = closeStockFPMPoolsWith(context.Background(), host.ops())
	if len(pools) != 1 || pools[0].State != transport.SideDoorsStockOff || len(host.steps) != 0 {
		t.Fatalf("second start: %+v %v", pools, host.steps)
	}
}

func TestStockPoolKeptWhenTheOwnerChangedIt(t *testing.T) {
	host, path := newStockHost(t, true)
	host.packaged = "0123456789abcdef0123456789abcdef"
	pools := closeStockFPMPoolsWith(context.Background(), host.ops())
	if len(pools) != 1 || pools[0].State != transport.SideDoorsStockOwnerChanged || len(host.steps) != 0 {
		t.Fatalf("pools = %+v steps=%v", pools, host.steps)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("the owner's pool was moved")
	}
}

func TestStockPoolKeptWhileAConfigurationUsesIt(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*fakeStockHost)
		used   []string
	}{
		{"its socket", func(h *fakeStockHost) {
			h.dump += "# configuration file /etc/nginx/sites-enabled/legacy:\n    location ~ \\.php$ { fastcgi_pass unix:/run/php/php8.4-fpm.sock; }\n"
		}, []string{"/etc/nginx/sites-enabled/legacy"}},
		{"the /var/run spelling", func(h *fakeStockHost) {
			h.dump += "# configuration file /etc/nginx/conf.d/old.conf:\n    fastcgi_pass unix:/var/run/php/php8.4-fpm.sock;\n"
		}, []string{"/etc/nginx/conf.d/old.conf"}},
		{"the php-fpm.sock alias pointing at it", func(h *fakeStockHost) {
			h.alias = "/run/php/php-fpm.sock"
			h.dump += "# configuration file /etc/nginx/snippets/fastcgi-php.conf:\n    fastcgi_pass unix:/run/php/php-fpm.sock;\n"
		}, []string{"/etc/nginx/snippets/fastcgi-php.conf"}},
		{"an Apache configuration", func(h *fakeStockHost) {
			h.apache = []string{"/etc/apache2/conf-enabled/php8.4-fpm.conf"}
		}, []string{"/etc/apache2/conf-enabled/php8.4-fpm.conf"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, _ := newStockHost(t, true)
			tc.mutate(host)
			pools := closeStockFPMPoolsWith(context.Background(), host.ops())
			if len(pools) != 1 || pools[0].State != transport.SideDoorsStockInUse || !reflect.DeepEqual(pools[0].UsedBy, tc.used) {
				t.Fatalf("pools = %+v", pools)
			}
			if len(host.steps) != 0 {
				t.Fatalf("a used pool was touched: %v", host.steps)
			}
		})
	}
	// A commented line and the php-fpm.sock alias pointing elsewhere do not count.
	host, _ := newStockHost(t, true)
	host.dump += "# configuration file /etc/nginx/conf.d/x.conf:\n    # fastcgi_pass unix:/run/php/php8.4-fpm.sock;\n    fastcgi_pass unix:/run/php/php-fpm.sock;\n"
	if pools := closeStockFPMPoolsWith(context.Background(), host.ops()); pools[0].State != transport.SideDoorsStockTurnedOff {
		t.Fatalf("pools = %+v", pools)
	}
}

func TestStockPoolKeptWhenItIsTheOnlyPoolOrUnknown(t *testing.T) {
	host, _ := newStockHost(t, false)
	if pools := closeStockFPMPoolsWith(context.Background(), host.ops()); pools[0].State != transport.SideDoorsStockOnlyPool || len(host.steps) != 0 {
		t.Fatalf("only pool: %+v %v", pools, host.steps)
	}
	host, _ = newStockHost(t, true)
	host.known = false
	if pools := closeStockFPMPoolsWith(context.Background(), host.ops()); pools[0].State != transport.SideDoorsStockUnknown || len(host.steps) != 0 {
		t.Fatalf("no dpkg digest: %+v %v", pools, host.steps)
	}
	host, _ = newStockHost(t, true)
	host.dumpErr = errors.New("nginx: [emerg] unknown directive")
	if pools := closeStockFPMPoolsWith(context.Background(), host.ops()); pools[0].State != transport.SideDoorsStockUnknown || len(host.steps) != 0 {
		t.Fatalf("nginx -T failed: %+v %v", pools, host.steps)
	}
}

func TestStockPoolPutBackWhenPHPFPMRefusesTheChange(t *testing.T) {
	host, path := newStockHost(t, true)
	host.testErr = errors.New("ERROR: no pool defined")
	pools := closeStockFPMPoolsWith(context.Background(), host.ops())
	if len(pools) != 1 || pools[0].State != transport.SideDoorsStockFailed || !strings.Contains(pools[0].Detail, "it was put back") {
		t.Fatalf("pools = %+v", pools)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != stockWWWConf {
		t.Fatalf("www.conf is not back: %v", err)
	}
	if got := strings.Join(host.steps, " "); got != "rename:www.conf>www.conf.off-by-celikpanel fpm-t:8.4 rename:www.conf.off-by-celikpanel>www.conf fpm-reload:8.4" {
		t.Fatalf("steps = %s", got)
	}
}

func TestConffileMD5FromDpkg(t *testing.T) {
	field := " /etc/php/8.4/fpm/php-fpm.conf 1111\n /etc/php/8.4/fpm/pool.d/www.conf 2222\n /etc/php/8.4/fpm/old.conf 3333 obsolete\n"
	if got, ok := conffileMD5From(field, "/etc/php/8.4/fpm/pool.d/www.conf"); !ok || got != "2222" {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := conffileMD5From(field, "/etc/php/8.4/fpm/old.conf"); ok {
		t.Fatal("an obsolete conffile was read as the package's")
	}
	if _, ok := conffileMD5From(field, "/etc/php/8.3/fpm/pool.d/www.conf"); ok {
		t.Fatal("a file dpkg does not list was read as the package's")
	}
}

func TestFollowSymlinksAcrossTheAlternativesChain(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "php8.4-fpm.sock")
	alternative := filepath.Join(dir, "alternatives-php-fpm.sock")
	alias := filepath.Join(dir, "php-fpm.sock")
	if err := os.Symlink(final, alternative); err != nil {
		t.Skip("symbolic links are not available here")
	}
	if err := os.Symlink(alternative, alias); err != nil {
		t.Fatal(err)
	}
	if got := followSymlinks(alias); got != final {
		t.Fatalf("followSymlinks = %q, want %q (the socket need not exist)", got, final)
	}
	if got := followSymlinks(final); got != "" {
		t.Fatalf("a non-link answered %q", got)
	}
}

func TestCloseSideDoorsMovesAToolOnceAndRecordsAFailure(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	savedRecord := phpToolAccountRecordPath
	phpToolAccountRecordPath = filepath.Join(t.TempDir(), "php-tool-accounts.json")
	t.Cleanup(func() { phpToolAccountRecordPath = savedRecord })
	phpToolInstalledCheck = func(tool string) bool { return tool == phpToolPHPPgAdmin }
	stock := filepath.Join(host.run, "php8.4-fpm.sock")
	conf := renderDBToolsConfig([]string{phpToolPHPPgAdmin}, map[string]string{phpToolPHPPgAdmin: "/usr/share/phppgadmin"},
		map[string]string{phpToolPHPPgAdmin: stock})
	if err := os.WriteFile(host.dbToolsConf, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}

	bind := newFakeBindHost()
	bind.packages["php8.4-pgsql"], bind.packages["php8.4-mbstring"], bind.packages["php8.4-xml"] = true, true, true
	bind.probeErr = &phpToolProbeError{Kind: "no_answer", Message: "phpPgAdmin did not answer"}
	ops := bind.ops()
	ops.ownPool = func(context.Context, string, string) (string, func(context.Context) error, error) {
		bind.steps = append(bind.steps, "own")
		return phpToolPoolSocket(phpToolPHPPgAdmin, "8.4"), nil, nil
	}
	tools := closeSideDoorsTools(context.Background(), ops, nil)
	if len(tools) != 1 || tools[0].State != transport.SideDoorsToolFailed || tools[0].Version != "8.4" {
		t.Fatalf("tools = %+v", tools)
	}
	if row := readPHPToolAccountFailures()[phpToolPHPPgAdmin]; row.PHPVersion != "8.4" {
		t.Fatalf("the failure was not recorded: %+v", row)
	}
	// The next start does not try again on its own.
	bind.steps = nil
	tools = closeSideDoorsTools(context.Background(), ops, nil)
	if len(tools) != 1 || tools[0].State != transport.SideDoorsToolFailed || len(bind.steps) != 0 {
		t.Fatalf("second start: %+v steps=%v", tools, bind.steps)
	}
	// Once the record is cleared (the Manage window's move), it is moved.
	if err := writePHPToolAccountFailure(phpToolPHPPgAdmin, "", ""); err != nil {
		t.Fatal(err)
	}
	bind.probeErr = nil
	tools = closeSideDoorsTools(context.Background(), ops, nil)
	if len(tools) != 1 || tools[0].State != transport.SideDoorsToolMoved || tools[0].Account != "celikpanel-phppgadmin" {
		t.Fatalf("third start: %+v steps=%v", tools, bind.steps)
	}
	// A tool already on its own pool is only reported.
	if err := os.WriteFile(host.dbToolsConf, []byte(renderDBToolsConfig([]string{phpToolPHPPgAdmin},
		map[string]string{phpToolPHPPgAdmin: "/usr/share/phppgadmin"},
		map[string]string{phpToolPHPPgAdmin: phpToolPoolSocket(phpToolPHPPgAdmin, "8.4")})), 0o644); err != nil {
		t.Fatal(err)
	}
	bind.steps = nil
	tools = closeSideDoorsTools(context.Background(), ops, nil)
	if len(tools) != 1 || tools[0].State != transport.SideDoorsToolOwnAccount || len(bind.steps) != 0 {
		t.Fatalf("own pool: %+v steps=%v", tools, bind.steps)
	}
}

func TestCloseSideDoorsLeavesAnOwnerEditedToolConfiguration(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	savedRecord := phpToolAccountRecordPath
	phpToolAccountRecordPath = filepath.Join(t.TempDir(), "php-tool-accounts.json")
	t.Cleanup(func() { phpToolAccountRecordPath = savedRecord })
	phpToolInstalledCheck = func(tool string) bool { return tool == phpToolPHPPgAdmin }
	conf := renderDBToolsConfig([]string{phpToolPHPPgAdmin}, map[string]string{phpToolPHPPgAdmin: "/usr/share/phppgadmin"},
		map[string]string{phpToolPHPPgAdmin: filepath.Join(host.run, "php8.4-fpm.sock")}) + "# the owner's line\n"
	if err := os.WriteFile(host.dbToolsConf, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	bind := newFakeBindHost()
	tools := closeSideDoorsTools(context.Background(), bind.ops(), nil)
	if len(tools) != 1 || tools[0].State != transport.SideDoorsToolOwnerConfig || len(bind.steps) != 0 {
		t.Fatalf("tools = %+v steps=%v", tools, bind.steps)
	}
}
