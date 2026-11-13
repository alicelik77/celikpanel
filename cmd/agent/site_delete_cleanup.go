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
//     /var/lib/celikpanel-agent/acme-http-01/subscriptions/<S>/domains/<id>
//     (found on Frankfurt after a deletion, 12 Nov 2026), and the
//     subscription's folders above it once they are empty.
//
// Every removal is idempotent (absent is done), so an interrupted deletion
// run again finishes the same work; what cannot be removed is named in the
// answer's error and the deletion stops at the site step (D-024).
//
// Only exact names derived from the deleted site's canonical domain and
// immutable identities are touched, only regular files (never a link), and a
// name that merely starts like the site's is left alone. Backups are not
// removed (the deletion keeps them on purpose); they are measured so the
// answer can say where they are, how many and how large.
//
// Silinen sitenin crontab'ı, web sunucusu günlükleri ve doğrulama klasörü de
// kaldırılır; yedekler silinmez, yalnız yeri, sayısı ve boyutu söylenir.

// The site's nginx logs are the ones site_logs.go names (siteLogDir,
// siteLogLiveName, siteLogRotatedPattern): one definition of which files
// belong to a site, for the log page, the retention and this removal.

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

// trafficLogFamily matches CelikPanel's traffic log of one site and the
// copies its logrotate rule makes (<id>.log.1, <id>.log.2.gz; traffic_collect.go).
func trafficLogFamily(domainID int) func(string) bool {
	pattern := regexp.MustCompile(`^` + strconv.Itoa(domainID) + `\.log(\.[0-9]{1,4}(\.(gz|xz|bz2|zst))?)?$`)
	return pattern.MatchString
}

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

// removeMatchingFiles removes the regular files of dir that match names.
func removeMatchingFiles(dir string, match func(string) bool) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	var failures []string
	for _, entry := range entries {
		if !match(entry.Name()) {
			continue
		}
		if err := removeRegularFile(filepath.Join(dir, entry.Name())); err != nil {
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
	rotated := siteLogRotatedPattern(domain)
	if err := removeMatchingFiles(siteLogDir, func(name string) bool {
		return siteLogLiveName(domain, name) || rotated.MatchString(name)
	}); err != nil {
		failures = append(failures, err.Error())
	}
	if domainID > 0 {
		if err := removeMatchingFiles(services.TrafficLogDir(), trafficLogFamily(domainID)); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

// removeSiteChallengeRoot removes the site's HTTP-01 challenge folder (root's,
// outside every tenant home), then its "domains" and subscription folders
// when nothing else is left in them. A link in place of a folder is refused.
func removeSiteChallengeRoot(subscriptionID, domainID int) error {
	root, err := siteChallengeRoot(subscriptionID, domainID)
	if err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect %s: %w", root, err)
	}
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a folder; it was left as it is", root)
		}
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("remove %s: %w", root, err)
		}
	}
	// .../subscriptions/<S>/domains, then .../subscriptions/<S>: removed only
	// while empty (another domain of the subscription keeps them).
	parent := filepath.Dir(root)
	for _, dir := range []string{parent, filepath.Dir(parent)} {
		if err := removeEmptyFolder(dir); err != nil {
			return err
		}
	}
	return nil
}

// removeEmptyFolder removes dir when it is a folder with nothing in it;
// absent or not empty is left alone, a link is refused.
func removeEmptyFolder(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a folder; it was left as it is", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Something was put there meanwhile (a certificate of another domain
		// of the subscription): it stays.
		if entries, readErr := os.ReadDir(dir); readErr == nil && len(entries) > 0 {
			return nil
		}
		return fmt.Errorf("remove %s: %w", dir, err)
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
	bytes, files, unknown := measureHeldTree(dir)
	if unknown {
		return &transport.KeptBackups{Path: dir, Unknown: true}
	}
	if files == 0 {
		return nil
	}
	return &transport.KeptBackups{Path: dir, Files: files, Bytes: bytes}
}
