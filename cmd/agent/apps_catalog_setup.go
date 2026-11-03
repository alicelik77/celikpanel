package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alicelik/celikpanel/internal/appcatalog"
)

// The setup recipes of the catalog's applications. Each runs the
// application's own supported non-interactive installer as the site's user
// with the site's PHP. A secret is never on a command line (other users of
// the server can read command lines): the command line carries a token such
// as @@CELIKPANEL:DBPASS@@ and a small, fixed PHP file loaded before the
// installer (auto_prepend_file) reads the real values from standard input and
// puts them in place inside the process. Files a recipe needs (phpBB's
// configuration) are written by that file with mode 0600 and removed when
// the installer ends.
//
// Kurulum tarifleri: uygulamanın kendi etkileşimsiz kurucusu sitenin
// kullanıcısıyla çalışır. Gizli bilgi komut satırına yazılmaz; komut satırı
// yalnız bir belirteç taşır, gerçek değeri önceden yüklenen sabit bir PHP
// dosyası standart girişten okur.

// setupPrependPath is the fixed PHP file; it holds no secret.
var setupPrependPath = "/var/lib/celikpanel-app-setup/prepend.php"

const setupPrependPHP = `<?php
// CelikPanel application setup (alpha.118). Loaded before an application's own
// installer: replaces the @@CELIKPANEL:NAME@@ tokens of the command line with
// the values read from standard input, so no secret is ever on a command line.
// Holds no secret. CelikPanel rewrites this file; it is not configuration.
(function () {
    $in = json_decode((string) stream_get_contents(STDIN), true);
    if (!is_array($in)) {
        fwrite(STDERR, "CelikPanel setup: the setup input could not be read\n");
        exit(70);
    }
    $map = [];
    foreach (($in['secrets'] ?? []) as $name => $value) {
        $map['@@CELIKPANEL:' . $name . '@@'] = (string) $value;
    }
    $args = array_map(function ($arg) use ($map) {
        return is_string($arg) ? strtr($arg, $map) : $arg;
    }, $GLOBALS['argv'] ?? []);
    $GLOBALS['argv'] = $args;
    $_SERVER['argv'] = $args;
    $files = $in['files'] ?? [];
    foreach ($files as $path => $content) {
        $old = umask(077);
        $written = file_put_contents($path, strtr((string) $content, $map));
        umask($old);
        if ($written === false) {
            fwrite(STDERR, "CelikPanel setup: a setup file could not be written\n");
            exit(71);
        }
    }
    register_shutdown_function(function () use ($files) {
        foreach (array_keys($files) as $path) {
            @unlink($path);
        }
    });
})();
`

// ensureSetupPrepend writes the fixed PHP file (root-owned, readable by
// everyone, no secret in it).
func ensureSetupPrepend() error {
	dir := filepath.Dir(setupPrependPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("the setup helper folder is not a real directory")
	}
	// The site's user must reach the file: the agent runs with UMask=0027, so
	// MkdirAll alone left the folder 0750 root:root and PHP stopped with
	// "Failed opening required .../prepend.php: Permission denied" before the
	// application's installer started (alpha.118 lab, Joomla exit code 255).
	// The folder and the file hold no secret; they are readable by everyone.
	// Sitenin kullanıcısı dosyaya ulaşabilmeli: klasör ve dosya herkesçe okunur.
	if info.Mode().Perm() != 0o755 {
		if err := os.Chmod(dir, 0o755); err != nil {
			return fmt.Errorf("make the setup helper folder readable: %w", err)
		}
	}
	if current, err := os.ReadFile(setupPrependPath); err == nil && string(current) == setupPrependPHP {
		if fileInfo, err := os.Lstat(setupPrependPath); err == nil && fileInfo.Mode().IsRegular() && fileInfo.Mode().Perm() != 0o644 {
			return os.Chmod(setupPrependPath, 0o644)
		}
		return nil
	}
	tmp, err := os.CreateTemp(dir, ".prepend-*.php")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(setupPrependPHP); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), setupPrependPath)
}

// setupInput is what the prepend file reads from standard input.
type setupInput struct {
	Secrets map[string]string `json:"secrets,omitempty"`
	Files   map[string]string `json:"files,omitempty"`
}

// setupStep is one command of a recipe. Exactly one of script, code or argv.
type setupStep struct {
	name string
	// script: a PHP file of the application (relative to the document root)
	// run with the prepend file; args may carry @@CELIKPANEL:NAME@@ tokens.
	script string
	args   []string
	// code: PHP code run with php -r; it reads its input from standard input.
	code string
	// argv: a plain command (mv, rm) with no secret.
	argv  []string
	input setupInput
}

func (s setupStep) command(site catalogSite) ([]string, []byte, error) {
	php := phpCLIForVersion(site.phpLine)
	limits := []string{"-d", "memory_limit=1024M", "-d", "max_execution_time=0"}
	switch {
	case s.script != "":
		if err := ensureSetupPrepend(); err != nil {
			return nil, nil, fmt.Errorf("prepare the setup helper: %w", err)
		}
		input, err := json.Marshal(s.input)
		if err != nil {
			return nil, nil, err
		}
		args := append([]string{php, "-d", "auto_prepend_file=" + setupPrependPath}, limits...)
		args = append(args, filepath.Join(site.docRoot, filepath.FromSlash(s.script)))
		return append(args, s.args...), input, nil
	case s.code != "":
		input, err := json.Marshal(s.input)
		if err != nil {
			return nil, nil, err
		}
		args := append([]string{php}, limits...)
		return append(args, "-r", s.code), input, nil
	case len(s.argv) > 0:
		return append([]string(nil), s.argv...), nil, nil
	}
	return nil, nil, errors.New("empty setup step")
}

// setupPlan is a recipe for one site.
type setupPlan struct {
	steps []setupStep
	// after runs whether or not this attempt ran the installer (it finishes
	// CelikPanel's part and is safe to repeat).
	after []setupStep
	// done says whether the application's own setup has finished.
	done      func() bool
	adminPath func(catalogSite) (string, error)
}

func (p setupPlan) alreadyDone() bool { return p.done != nil && p.done() }

func setupToken(name string) string { return "@@CELIKPANEL:" + name + "@@" }

// configHolds: the application's configuration file exists and holds text.
// Root reads it beneath the document root without following any link, never
// a FIFO or device, and at most 4 MiB (the site's account owns the folder).
func configHolds(site catalogSite, needle string) func() bool {
	return func() bool {
		content, err := readRegularBeneath(site.docRoot, site.app.ConfigFile, 4<<20)
		return err == nil && bytes.Contains(content, []byte(needle))
	}
}

var dbIdentPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// buildSetupPlan is the recipe of the site's application.
func buildSetupPlan(site catalogSite, req *SetupCatalogAppRequest) (setupPlan, error) {
	if !dbIdentPattern.MatchString(req.DBName) || !dbIdentPattern.MatchString(req.DBUser) {
		return setupPlan{}, errors.New("invalid database name or user")
	}
	dbHost := strings.TrimSpace(req.DBHost)
	if dbHost == "" {
		dbHost = "localhost"
	}
	if strings.ContainsAny(dbHost, " \t\r\n'\"") {
		return setupPlan{}, errors.New("invalid database host")
	}
	email := strings.TrimSpace(req.AdminEmail)
	if strings.ContainsAny(email, " \t\r\n'\"\\") {
		return setupPlan{}, errors.New("invalid administrator e-mail address")
	}
	scheme := "http"
	if req.HTTPS {
		scheme = "https"
	}
	secrets := map[string]string{"DBPASS": req.DBPass, "ADMINPASS": req.AdminPass}
	input := setupInput{Secrets: secrets}
	app := site.app
	switch app.Setup {
	case appcatalog.SetupNextcloud:
		dataDir := filepath.Join(site.siteHome, app.DataDir)
		occ := func(name string, args ...string) setupStep {
			return setupStep{name: name, script: "occ", args: append([]string{"--no-interaction"}, args...), input: setupInput{}}
		}
		return setupPlan{
			steps: []setupStep{{
				name: "occ maintenance:install", script: "occ", input: input,
				args: []string{"maintenance:install", "--no-interaction",
					"--database=mysql", "--database-host=" + dbHost,
					"--database-name=" + req.DBName, "--database-user=" + req.DBUser,
					"--database-pass=" + setupToken("DBPASS"),
					"--admin-user=admin", "--admin-pass=" + setupToken("ADMINPASS"),
					"--data-dir=" + dataDir},
			}},
			after: []setupStep{
				occ("occ trusted_domains", "config:system:set", "trusted_domains", "0", "--value="+site.domain),
				occ("occ trusted_domains www", "config:system:set", "trusted_domains", "1", "--value=www."+site.domain),
				occ("occ overwrite.cli.url", "config:system:set", "overwrite.cli.url", "--value="+scheme+"://"+site.domain),
			},
			done: configHolds(site, "'installed' => true"),
		}, nil

	case appcatalog.SetupDrupal:
		input.Files = nil
		vars := map[string]string{
			"root": site.docRoot, "db_name": req.DBName, "db_user": req.DBUser, "db_host": dbHost,
			"site_name": site.domain, "email": email, "domain": site.domain,
		}
		raw, err := json.Marshal(vars)
		if err != nil {
			return setupPlan{}, err
		}
		input.Secrets["VARS"] = string(raw)
		return setupPlan{
			steps: []setupStep{{name: "Drupal install_drupal()", code: drupalInstallPHP, input: input}},
			done:  configHolds(site, "$databases['default']['default'] = "),
		}, nil

	case appcatalog.SetupJoomla:
		prefix, err := randomLowerAlnum(5)
		if err != nil {
			return setupPlan{}, err
		}
		return setupPlan{
			steps: []setupStep{{
				name: "Joomla CLI installer", script: "installation/joomla.php", input: input,
				args: []string{"install", "--no-interaction",
					"--site-name=" + site.domain, "--admin-user=Administrator",
					"--admin-username=admin", "--admin-password=" + setupToken("ADMINPASS"),
					"--admin-email=" + email, "--db-type=mysqli", "--db-host=" + dbHost,
					"--db-user=" + req.DBUser, "--db-pass=" + setupToken("DBPASS"),
					"--db-name=" + req.DBName, "--db-prefix=" + "j" + prefix + "_",
					"--db-encryption=0"},
			}},
			after: []setupStep{{name: "remove the installation folder", argv: []string{"rm", "-rf", "--one-file-system", "--", filepath.Join(site.docRoot, "installation")}}},
			done:  configHolds(site, "public $password = "),
		}, nil

	case appcatalog.SetupPrestaShop:
		ssl := "0"
		if req.HTTPS {
			ssl = "1"
		}
		return setupPlan{
			steps: []setupStep{{
				name: "PrestaShop CLI installer", script: "install/index_cli.php", input: input,
				args: []string{"--domain=" + site.domain, "--db_server=" + dbHost,
					"--db_name=" + req.DBName, "--db_user=" + req.DBUser,
					"--db_password=" + setupToken("DBPASS"), "--prefix=ps_", "--db_create=0",
					"--name=" + site.domain, "--email=" + email, "--password=" + setupToken("ADMINPASS"),
					"--firstname=Shop", "--lastname=Administrator", "--language=en", "--country=us",
					"--ssl=" + ssl, "--rewrite=0", "--fixtures=0"},
			}},
			after: []setupStep{
				{name: "remove the install folder", argv: []string{"rm", "-rf", "--one-file-system", "--", filepath.Join(site.docRoot, "install")}},
				prestashopAdminRename(site),
				{name: "clear the cache", argv: []string{"sh", "-c", `if [ -d "$1" ] && [ ! -L "$1" ]; then find "$1" -xdev -mindepth 1 -delete; fi`, "sh", filepath.Join(site.docRoot, "var", "cache")}},
			},
			done:      configHolds(site, "'database_password' => "),
			adminPath: prestashopAdminPath,
		}, nil

	case appcatalog.SetupMediaWiki:
		return setupPlan{
			steps: []setupStep{{
				name: "MediaWiki installer", script: "maintenance/run.php", input: input,
				args: []string{"install", "--dbtype=mysql", "--dbserver=" + dbHost,
					"--dbname=" + req.DBName, "--dbuser=" + req.DBUser, "--dbpass=" + setupToken("DBPASS"),
					"--installdbuser=" + req.DBUser, "--installdbpass=" + setupToken("DBPASS"),
					"--server=" + scheme + "://" + site.domain, "--scriptpath=", "--lang=en",
					"--pass=" + setupToken("ADMINPASS"), site.domain, "Admin"},
			}},
			done: configHolds(site, "$wgDBpassword"),
		}, nil

	case appcatalog.SetupPhpBB:
		configPath := filepath.Join(site.siteHome, ".celikpanel-phpbb-install.yml")
		input.Files = map[string]string{configPath: phpbbInstallYAML(site.domain, email, dbHost, req.DBName, req.DBUser, req.HTTPS)}
		return setupPlan{
			steps: []setupStep{{
				name: "phpBB CLI installer", script: "install/phpbbcli.php", input: input,
				args: []string{"install", configPath},
			}},
			after: []setupStep{{name: "remove the install folder", argv: []string{"rm", "-rf", "--one-file-system", "--", filepath.Join(site.docRoot, "install")}}},
			done:  configHolds(site, "$dbpasswd = "),
		}, nil
	}
	return setupPlan{}, fmt.Errorf("%s has no setup recipe in this agent", app.Name)
}

// phpbbInstallYAML is phpBB's installer configuration (docs/install-config.sample.yml).
func phpbbInstallYAML(domain, email, dbHost, dbName, dbUser string, https bool) string {
	protocol, port, secure := "http://", "80", "false"
	if https {
		protocol, port, secure = "https://", "443", "true"
	}
	return "installer:\n" +
		"    admin:\n" +
		"        name: admin\n" +
		"        password: '" + setupToken("ADMINPASS") + "'\n" +
		"        email: '" + email + "'\n" +
		"    board:\n" +
		"        lang: en\n" +
		"        name: '" + domain + "'\n" +
		"        description: ''\n" +
		"    database:\n" +
		"        dbms: mysqli\n" +
		"        dbhost: '" + dbHost + "'\n" +
		"        dbport: ~\n" +
		"        dbuser: '" + dbUser + "'\n" +
		"        dbpasswd: '" + setupToken("DBPASS") + "'\n" +
		"        dbname: '" + dbName + "'\n" +
		"        table_prefix: phpbb_\n" +
		"    email:\n" +
		"        enabled: false\n" +
		"        smtp_delivery: ~\n" +
		"        smtp_host: ~\n" +
		"        smtp_port: ~\n" +
		"        smtp_auth: ~\n" +
		"        smtp_user: ~\n" +
		"        smtp_pass: ~\n" +
		"    server:\n" +
		"        cookie_secure: " + secure + "\n" +
		"        server_protocol: " + protocol + "\n" +
		"        force_server_vars: false\n" +
		"        server_name: '" + domain + "'\n" +
		"        server_port: " + port + "\n" +
		"        script_path: /\n" +
		"    extensions: []\n"
}

// PrestaShop 9's own installer already renames the folder (admin + about
// 20 random letters and digits); ours adds 12 when it did not. admin-api and
// other hyphenated folders never match.
var prestashopAdminDir = regexp.MustCompile(`^admin[a-z0-9]{8,40}$`)

// prestashopAdminRename gives the administration folder a name nobody can
// guess (PrestaShop asks for it); a folder renamed by an earlier attempt is
// kept.
func prestashopAdminRename(site catalogSite) setupStep {
	suffix, err := randomLowerAlnum(12)
	if err != nil {
		suffix = "celikpanel00"
	}
	from := filepath.Join(site.docRoot, "admin")
	to := filepath.Join(site.docRoot, "admin"+suffix)
	// mv only when "admin" is still there; -n never replaces.
	return setupStep{name: "rename the administration folder",
		argv: []string{"sh", "-c", `if [ -d "$1" ] && [ ! -L "$1" ]; then mv -n -- "$1" "$2"; fi`, "sh", from, to}}
}

func prestashopAdminPath(site catalogSite) (string, error) {
	entries, err := os.ReadDir(site.docRoot)
	if err != nil {
		return "", err
	}
	var found []string
	for _, entry := range entries {
		if entry.IsDir() && prestashopAdminDir.MatchString(entry.Name()) {
			found = append(found, entry.Name())
		}
	}
	switch len(found) {
	case 1:
		return "/" + found[0] + "/", nil
	case 0:
		return "", errors.New("no renamed administration folder was found")
	default:
		return "", fmt.Errorf("more than one administration folder was found (%s); keep the one PrestaShop uses and remove the others", strings.Join(found, ", "))
	}
}

// drupalInstallPHP installs Drupal through its own install API, the way
// Drupal's own "drupal install" command does (core/lib/Drupal/Core/Command/
// InstallCommand.php), with MySQL/MariaDB instead of SQLite. Its input
// arrives on standard input: {"secrets":{"DBPASS","ADMINPASS","VARS"}}.
const drupalInstallPHP = `$in=json_decode((string)stream_get_contents(STDIN),true);` +
	`if(!is_array($in)||!isset($in['secrets']['VARS'])){fwrite(STDERR,"setup input unreadable\n");exit(70);}` +
	`$v=json_decode($in['secrets']['VARS'],true);$dbpass=$in['secrets']['DBPASS'];$pass=$in['secrets']['ADMINPASS'];unset($in);` +
	`chdir($v['root']);$class_loader=require $v['root'].'/autoload.php';` +
	`$site='sites/default';` +
	`if(!file_exists("$site/settings.php")&&!copy("$site/default.settings.php","$site/settings.php")){fwrite(STDERR,"settings.php could not be created\n");exit(72);}` +
	`require_once 'core/includes/install.core.inc';` +
	`$ns='Drupal\\mysql\\Driver\\Database\\mysql';` +
	`install_drupal($class_loader,['interactive'=>FALSE,'site_path'=>$site,` +
	`'parameters'=>['profile'=>'standard','langcode'=>'en'],` +
	`'forms'=>['install_settings_form'=>['driver'=>$ns,$ns=>['database'=>$v['db_name'],'username'=>$v['db_user'],'password'=>$dbpass,'host'=>$v['db_host'],'port'=>'3306','prefix'=>'']],` +
	`'install_configure_form'=>['site_name'=>$v['site_name'],'site_mail'=>$v['email'],` +
	`'account'=>['name'=>'admin','mail'=>$v['email'],'pass'=>['pass1'=>$pass,'pass2'=>$pass]],` +
	`'enable_update_status_module'=>TRUE,'enable_update_status_emails'=>NULL]]]);` +
	`$d=preg_quote($v['domain']);@chmod("$site/settings.php",0640);` +
	`if(file_put_contents("$site/settings.php","\n\$settings['trusted_host_patterns'] = ['^".$d."$', '^www\\\\.".$d."$'];\n",FILE_APPEND)===false){fwrite(STDERR,"trusted host patterns could not be added to settings.php\n");}` +
	`@chmod("$site/settings.php",0440);` +
	`echo "Drupal installed\n";`
