package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// What a site deletion removes besides the vhost, pool, account and files
// (alpha.134, owner's rule "no leftovers, ever"). Each was found on a real
// server after a deletion, or in the code: none of them is removed by
// `userdel -r` or by the vhost removal.
//
//   - The account's crontab. `userdel` leaves /var/spool/cron/crontabs/<user>
//     (Debian, Ubuntu) or /var/spool/cron/<user> (cronie) behind unless
//     USERDEL_CMD is set; cron then logs "ORPHAN (no passwd entry)" and a
//     later account of the same name would inherit the jobs.
//   - The site's web server logs: /var/log/nginx/<domain>-access.log and
//     -error.log, written by the site's vhost, with their rotated copies; and
//     CelikPanel's own traffic log /var/log/celikpanel/traffic/<id>.log.
//   - The site's HTTP-01 challenge folder under
//     /var/lib/celikpanel-agent/acme-http-01/subscriptions/<S>/domains/<id>.
//
// Only exact names derived from the deleted site's canonical domain and
// immutable identities are touched, only regular files (never a link), and a
// name that merely starts like the site's is left alone. Backups are not
// removed (the deletion keeps them on purpose); they are measured so the
// answer can say where they are, how many and how large.
//
// Silinen sitenin crontab'ı, web sunucusu günlükleri ve doğrulama klasörü de
// kaldırılır; yedekler silinmez, yalnız yeri, sayısı ve boyutu söylenir.

// siteNginxLogDir is where the site's vhost writes its logs (a seam for tests).
var siteNginxLogDir = "/var/log/nginx"

// siteCronSpoolPaths are the places a crontab lives: Debian/Ubuntu cron and
// cronie (Arch, Fedora). A seam for tests.
var siteCronSpoolPaths = func(username string) []string {
	return []string{
		filepath.Join("/var/spool/cron/crontabs", username),
		filepath.Join("/var/spool/cron", username),
	}
}

// siteChallengeRoot is hostingpath.ACMEChallengeRoot (a seam for tests).
var siteChallengeRoot = hostingpath.ACMEChallengeRoot

// siteBackupDir is the backup store's own folder of one domain (a seam).
var siteBackupDir = func(subscriptionID, domainID int) (string, error) {
	return scopeBackupDir(backupScope{
		ProtocolVersion: backupspec.ProtocolVersion,
		SubscriptionID:  subscriptionID,
		DomainID:        domainID,
	})
}

// rotatedSuffix matches what logrotate appends: .1, .2.gz, -20261112,
// -20261112.gz (dateext), and nothing at all.
var rotatedSuffix = regexp.MustCompile(`^((\.[0-9]{1,4})|(-[0-9]{8,10}))?(\.(gz|xz|bz2|zst))?$`)

// removeSiteCrontab removes the site account's crontab, then makes sure no
// spool file of that name is left (an account deleted by an earlier attempt
// can no longer be named to `crontab`).
func removeSiteCrontab(username string) error {
	if _, err := user.Lookup(username); err == nil {
		if _, lookErr := cronLookPath("crontab"); lookErr == nil {
			cmd := exec.Command("crontab", "-u", username, "-r")
			cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=C")
			output, runErr := cmd.CombinedOutput()
			said := strings.TrimSpace(string(output))
			if runErr != nil && said != "no crontab for "+username {
				return fmt.Errorf("crontab -r: %s", hostcmd.Bounded(strings.Join(strings.Fields(said), " "), 200))
			}
		}
	}
	return removeCrontabSpoolFiles(username)
}

func removeCrontabSpoolFiles(username string) error {
	var failures []string
	for _, path := range siteCronSpoolPaths(username) {
		if err := removeRegularFile(path); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

// removeRegularFile removes path when it is a regular file; absent is done,
// anything else (a directory, a link) is refused and named.
func removeRegularFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file; it was left as it is", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// removeLogFamily removes base and its rotated copies from dir.
func removeLogFamily(dir, base string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	var failures []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, base) || !rotatedSuffix.MatchString(name[len(base):]) {
			continue
		}
		if err := removeRegularFile(filepath.Join(dir, name)); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

// removeSiteLogs removes the site's nginx logs and its traffic log, with
// their rotated copies.
func removeSiteLogs(domain string, domainID int) error {
	var failures []string
	for _, base := range []string{domain + "-access.log", domain + "-error.log"} {
		if err := removeLogFamily(siteNginxLogDir, base); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if domainID > 0 {
		if err := removeLogFamily(services.TrafficLogDir(), strconv.Itoa(domainID)+".log"); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

// removeSiteChallengeRoot removes the site's HTTP-01 challenge folder (root's,
// outside every tenant home). A link in its place is refused.
func removeSiteChallengeRoot(subscriptionID, domainID int) error {
	root, err := siteChallengeRoot(subscriptionID, domainID)
	if err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a folder; it was left as it is", root)
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("remove %s: %w", root, err)
	}
	return nil
}

// measureKeptBackups says where the deleted site's backups stay, how many
// files and how many bytes. Read only; nil when there are none or they could
// not be read (Unknown then says so).
func measureKeptBackups(subscriptionID, domainID int) *transport.KeptBackups {
	dir, err := siteBackupDir(subscriptionID, domainID)
	if err != nil {
		return nil
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() {
		return &transport.KeptBackups{Path: dir, Unknown: true}
	}
	kept := &transport.KeptBackups{Path: dir}
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			fileInfo, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			kept.Files++
			kept.Bytes += fileInfo.Size()
		}
		return nil
	})
	if walkErr != nil {
		return &transport.KeptBackups{Path: dir, Unknown: true}
	}
	if kept.Files == 0 {
		return nil
	}
	return kept
}
