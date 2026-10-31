package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Packages on Arch leave some first-run initialization to the operator. Run
// it only inside the explicit Install action, before reporting readiness.
func prepareInstalledService(serviceID, family string) error {
	return prepareInstalledServiceContext(context.Background(), serviceID, family)
}

// prepareInstalledServiceContext keeps first-run initialization in the same
// cancellable process group as the durable install mutation.
// prepareInstalledServiceContext, ilk çalıştırma hazırlığını kalıcı kurulum
// değişikliğiyle aynı iptal edilebilir süreç grubunda tutar.
func prepareInstalledServiceContext(ctx context.Context, serviceID, family string) error {
	if family != "pacman" {
		return nil
	}
	switch serviceID {
	case "postgresql":
		return prepareArchPostgreSQLContext(ctx)
	case "mariadb":
		return prepareArchMariaDBContext(ctx)
	case "clamav":
		return prepareArchClamAVContext(ctx)
	case "fail2ban":
		return prepareArchFail2ban()
	default:
		return nil
	}
}

// Debian's fail2ban package enables the sshd jail and bans through nftables
// (/etc/fail2ban/jail.d/defaults-debian.conf). Arch's package enables no
// jail at all and keeps the iptables ban action, so an installed Fail2ban
// protected nothing. CelikPanel writes the same defaults, once, as its own
// jail.d file, with Arch's unit name for the SSH journal match; a file
// already there (the owner's or an earlier one) is never rewritten.
// Debian'ın fail2ban paketi sshd hapsini açar ve nftables ile yasaklar;
// Arch'ınki hiçbir hapsi açmaz. CelikPanel aynı varsayılanları bir kez,
// kendi jail.d dosyası olarak yazar; var olan dosyaya dokunmaz.
var archFail2banDefaultsPath = "/etc/fail2ban/jail.d/celikpanel-defaults.conf"

const archFail2banDefaults = `# Written once by CelikPanel when Fail2ban was installed: the defaults Debian's
# package ships (defaults-debian.conf), with Arch Linux's sshd unit name.
# CelikPanel never rewrites this file; edit or remove it as you need.
[DEFAULT]
banaction = nftables
banaction_allports = nftables[type=allports]

[sshd]
backend = systemd
journalmatch = _SYSTEMD_UNIT=sshd.service + _COMM=sshd
enabled = true
`

func prepareArchFail2ban() error {
	dir := filepath.Dir(archFail2banDefaultsPath)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("fail2ban jail directory %s is missing; reinstall Fail2ban with the operating system's package manager", dir)
	}
	exists, err := secureMailFileExists(archFail2banDefaultsPath)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", archFail2banDefaultsPath, err)
	}
	if exists {
		return nil
	}
	if err := secureWriteConfig(archFail2banDefaultsPath, []byte(archFail2banDefaults), 0o644); err != nil {
		return fmt.Errorf("write fail2ban defaults: %w", err)
	}
	return nil
}

func prepareArchPostgreSQL() error {
	return prepareArchPostgreSQLContext(context.Background())
}

func prepareArchPostgreSQLContext(ctx context.Context) error {
	const dataDir = "/var/lib/postgres/data"
	if _, err := os.Stat(filepath.Join(dataDir, "PG_VERSION")); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("postgresql data directory cannot be inspected: %w", err)
	}
	if out, err := runServiceMutationCombinedOutput(ctx,
		"install", "-d", "-m", "0700", "-o", "postgres", "-g", "postgres", dataDir,
	); err != nil {
		return commandFailure("postgresql data directory setup", out, err)
	}
	out, err := runServiceMutationCombinedOutput(ctx,
		"runuser", "-u", "postgres", "--",
		"initdb", "--locale=C.UTF-8", "--encoding=UTF8", "-D", dataDir,
	)
	if err != nil {
		return commandFailure("postgresql cluster initialization", out, err)
	}
	return nil
}

func prepareArchMariaDB() error {
	return prepareArchMariaDBContext(context.Background())
}

// Debian's 50-server.cnf binds MariaDB to 127.0.0.1; Arch's server.cnf names
// no address, so MariaDB listened on every interface (seen on the Arch VM,
// 2026-10-30, port 3306 on 0.0.0.0; the firewall dropped it, but a database
// must not depend on that). CelikPanel writes Debian's default once as its
// own option file, unless one of /etc/my.cnf.d already names an address.
// Arch'ta MariaDB tüm arayüzleri dinliyordu; Debian'ın varsayılanı bir kez,
// başka bir dosya adres belirtmiyorsa yazılır.
var archMariaDBDropInDir = "/etc/my.cnf.d"

const archMariaDBBindFile = "celikpanel-bind-address.cnf"

const archMariaDBBindText = `# Written once by CelikPanel when MariaDB was installed: Debian's default,
# MariaDB answers on this server only. Edit or remove it as you need.
[mysqld]
bind-address = 127.0.0.1
`

func prepareArchMariaDBBindAddress() error {
	info, err := os.Lstat(archMariaDBDropInDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(archMariaDBDropInDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", archMariaDBDropInDir, err)
	}
	for _, entry := range entries {
		if entry.Name() == archMariaDBBindFile {
			return nil
		}
		data, err := os.ReadFile(filepath.Join(archMariaDBDropInDir, entry.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			key, _, _ := strings.Cut(strings.TrimSpace(line), "=")
			key = strings.ReplaceAll(strings.TrimSpace(key), "_", "-")
			if key == "bind-address" || key == "skip-networking" {
				return nil
			}
		}
	}
	if err := secureWriteConfig(filepath.Join(archMariaDBDropInDir, archMariaDBBindFile), []byte(archMariaDBBindText), 0o644); err != nil {
		return fmt.Errorf("write MariaDB bind address: %w", err)
	}
	return nil
}

func prepareArchMariaDBContext(ctx context.Context) error {
	if err := prepareArchMariaDBBindAddress(); err != nil {
		return err
	}
	const dataDir = "/var/lib/mysql"
	if _, err := os.Stat(filepath.Join(dataDir, "mysql")); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("mariadb data directory cannot be inspected: %w", err)
	}
	out, err := runServiceMutationCombinedOutput(ctx,
		"mariadb-install-db",
		"--user=mysql", "--basedir=/usr", "--datadir="+dataDir,
	)
	if err != nil {
		return commandFailure("mariadb data directory initialization", out, err)
	}
	return nil
}

var clamAVDatabaseDir = "/var/lib/clamav"

func prepareArchClamAV() error {
	return prepareArchClamAVContext(context.Background())
}

func prepareArchClamAVContext(ctx context.Context) error {
	if clamAVSignaturesReady(clamAVDatabaseDir) {
		return nil
	}
	out, err := runServiceMutationCombinedOutput(ctx, "freshclam")
	if !clamAVSignaturesReady(clamAVDatabaseDir) {
		if err != nil {
			return commandFailure("clamav signature download", out, err)
		}
		return fmt.Errorf("clamav signature download completed without creating a daily signature database")
	}
	return nil
}

func clamAVSignaturesReady(dir string) bool {
	for _, name := range []string{"daily.cvd", "daily.cld", "daily.inc"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func commandFailure(label string, out []byte, err error) error {
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		return fmt.Errorf("%s failed: %w", label, err)
	}
	return fmt.Errorf("%s failed: %w: %s", label, err, detail)
}
