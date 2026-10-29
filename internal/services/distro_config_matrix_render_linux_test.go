//go:build linux

package services

// TestRenderDistroConfigMatrix renders the PHP-FPM pool, php.ini and MariaDB
// server settings this package writes, with the package's own generators and
// editors, into CELIKPANEL_CONFIG_MATRIX_OUT. See the cmd/agent test of the
// same name and deploy/test-distro-config-matrix.sh: it runs inside a clean
// container of each supported release, so the PHP-FPM layout, the web server
// group and the stock php.ini / 50-server.cnf are the distribution's own.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alicelik/celikpanel/internal/configmatrix"
	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

func TestRenderDistroConfigMatrix(t *testing.T) {
	out := configmatrix.OutDir()
	if out == "" {
		t.Skip("set CELIKPANEL_CONFIG_MATRIX_OUT (deploy/test-distro-config-matrix.sh) to render the distro config matrix")
	}
	version := os.Getenv("CELIKPANEL_CONFIG_MATRIX_PHP_VERSION")
	write := func(c *configmatrix.Case) {
		if err := c.Write(out); err != nil {
			t.Fatalf("write case %s: %v", c.ID, err)
		}
	}
	failed := func(id, program, variant, source string, err error) {
		write(&configmatrix.Case{ID: id, Program: program, Variant: variant, Validator: "render-failed",
			Source: source, Extra: map[string]string{"REASON": err.Error()}})
	}
	const user = "cpmatrix42"

	// PHP-FPM site pools: sitePoolFile (site_pool_file.go:176) on this host's
	// own layout, sealed as ApplyManagedSiteFile writes it.
	for _, v := range []struct {
		id, variant string
		values      *poolValues
	}{
		{"pool-default", "site pool, CelikPanel defaults (pm=dynamic)", nil},
		{"pool-ondemand", "site pool, pm=ondemand", &poolValues{pm: "ondemand", maxChildren: 10, startServers: 2, minSpare: 1, maxSpare: 3, maxRequests: 1000}},
		{"pool-static", "site pool, pm=static", &poolValues{pm: "static", maxChildren: 4, startServers: 2, minSpare: 1, maxSpare: 3, maxRequests: 0}},
	} {
		file := sitePoolFile(transport.SitePoolRequest{SiteID: 42, Username: user, PHPVersion: version}, version, v.values, transport.SiteFileTriggerCreate)
		c := &configmatrix.Case{ID: "php-fpm." + v.id, Program: "php-fpm", Variant: v.variant, Validator: "php-fpm",
			Source: "internal/services/site_pool_file.go:165 sitePoolBody / legacy_site_file_texts.go:65 legacyPoolText",
			Target: file.Path, Extra: map[string]string{"USERS": user, "PHP_VERSION": version}}
		c.File(file.Path, file.sealed(), 0o644)
		write(c)
	}

	// php.ini: UpdateExtendedConfig (php_config_manager.go:157) on a scratch
	// copy of the release's stock php.ini.
	iniTarget := phpINIPath(version)
	stockINI, err := os.ReadFile(iniTarget)
	if err != nil {
		write(&configmatrix.Case{ID: "php-fpm.php-ini", Program: "php-fpm", Variant: "php.ini settings", Validator: "not-applicable",
			Source: "internal/services/php_config_manager.go:157", Extra: map[string]string{"REASON": "no stock php.ini at " + iniTarget}})
	} else {
		singleUnit := phpLayoutFor(version).singleUnit // the real host's layout
		state := withConfigTestState(t, string(stockINI), "")
		if err := os.RemoveAll(filepath.Join(state.phpRoot, "8.3")); err != nil {
			t.Fatal(err)
		}
		scratch := filepath.Join(state.phpRoot, version, "fpm", "php.ini")
		if singleUnit {
			scratch = filepath.Join(state.phpRoot, "php.ini")
			if err := os.MkdirAll(filepath.Join(state.phpRoot, "php-fpm.d"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(scratch), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(scratch, stockINI, 0o644); err != nil {
			t.Fatal(err)
		}
		config := &core.ExtendedPHPConfig{
			MemoryLimit: "256M", MaxExecutionTime: "60", MaxInputTime: "60", PostMaxSize: "64M",
			UploadMaxFilesize: "64M", OpcacheEnable: "1", DisableFunctions: "exec,passthru,shell_exec,system",
			IncludePath: ".:/usr/share/php", SessionSavePath: "/var/lib/php/sessions", RealpathCacheSize: "4096k",
			OpenBasedir: "/var/www/celikpanel:/tmp", ErrorReporting: "E_ALL & ~E_DEPRECATED", DisplayErrors: "Off",
			LogErrors: "On", AllowUrlFopen: "On", FileUploads: "On", ShortOpenTag: "Off",
			AdditionalDirectives: "max_input_vars = 3000\ndate.timezone = UTC",
		}
		source := "internal/services/php_config_manager.go:157 UpdateExtendedConfig"
		if err := NewPHPConfigManager().UpdateExtendedConfig(version, config); err != nil {
			failed("php-fpm.php-ini", "php-fpm", "php.ini settings", source, err)
		} else if edited, err := os.ReadFile(scratch); err != nil {
			t.Fatal(err)
		} else {
			c := &configmatrix.Case{ID: "php-fpm.php-ini", Program: "php-fpm", Variant: "php.ini extended settings + additional directives",
				Validator: "php-ini", Source: source, Target: iniTarget, Extra: map[string]string{"PHP_VERSION": version}}
			c.File(iniTarget, edited, 0o644)
			write(c)
		}
	}

	// MariaDB: UpdateMySQLSettings (config_manager.go:160) edits the Debian
	// 50-server.cnf only.
	const mysqlTarget = "/etc/mysql/mariadb.conf.d/50-server.cnf"
	stockCnf, err := os.ReadFile(mysqlTarget)
	if err != nil {
		write(&configmatrix.Case{ID: "mariadb.server-settings", Program: "mariadb", Variant: "server settings", Validator: "not-applicable",
			Source: "internal/services/config_manager.go:14,160 UpdateMySQLSettings",
			Extra:  map[string]string{"REASON": "UpdateMySQLSettings edits the fixed path " + mysqlTarget + ", which this release does not have"}})
		return
	}
	state := withConfigTestState(t, "", string(stockCnf))
	source := "internal/services/config_manager.go:160 UpdateMySQLSettings"
	if err := NewConfigManager().UpdateMySQLSettings(&MySQLSettings{MaxConnections: 300, InnodbBufferPool: "512M", QueryCacheSize: "0", MaxAllowedPacket: "64M"}); err != nil {
		failed("mariadb.server-settings", "mariadb", "server settings", source, err)
		return
	}
	edited, err := os.ReadFile(state.mysqlPath)
	if err != nil {
		t.Fatal(err)
	}
	c := &configmatrix.Case{ID: "mariadb.server-settings", Program: "mariadb", Variant: "max_connections/innodb_buffer_pool_size/query_cache_size/max_allowed_packet",
		Validator: "mariadb", Source: source, Target: mysqlTarget}
	c.File(mysqlTarget, edited, 0o644)
	write(c)
}
