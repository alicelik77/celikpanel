package main

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A site account's own service manager (S4 R2b second fix round, review
// N11; docs/SITE-SANDBOX.md section 9.5).
//
// Measured on Debian 13 and Ubuntu 24.04 (facts-r2b2, N11): an SFTP login
// of a site account goes through sshd with UsePAM and pam_systemd, which
// starts user@<uid>.service in user.slice, outside the site's slice and the
// SMTP block; the site's own code can then run units of its own there
// (planted under ~/.config/systemd/user, or `systemd-run --user` while a
// session is open) for as long as the session stays open. With
// user@<uid>.service masked (a link to /dev/null in /etc/systemd/system),
// the SFTP login still works, /run/user/<uid> is still made, and no manager
// starts. FTP as the panel configures it starts none.
//
// So every site account's uid gets that mask: CelikPanel's own link,
// recorded in the sandbox ledger like the other side doors' files, written
// at site creation and by the side doors step after each Panel start for
// existing sites, and removed when its uid no longer belongs to a site
// account (the site was deleted). A file or link of that name that is not
// CelikPanel's is the server owner's: kept, and said (a /dev/null link of
// the owner's closes it too). The owner's removal of CelikPanel's link
// stands (said, never written again by itself). Never for a uid an
// administrator account shares (a member of sudo, admin or wheel) or root.
// After the panel is removed the masks stay; they are removed with
// `systemctl unmask user@<uid>.service`.
//
// Site hesabının kendi hizmet yöneticisi: SFTP girişinde sitenin sınırları
// dışında başlamasın diye user@<uid>.service maskelenir.

const userManagerMaskKind = "site_user_manager_mask"

// userManagerMaskTarget is what a mask links to.
const userManagerMaskTarget = "/dev/null"

var userManagerMu sync.Mutex

func userManagerUnit(uid int) string { return "user@" + strconv.Itoa(uid) + ".service" }

// userManagerUID reads the uid back from a mask's path ("" when it is not one).
func userManagerUID(path string) (int, bool) {
	name := filepath.Base(path)
	rest, ok := strings.CutPrefix(name, "user@")
	if !ok {
		return 0, false
	}
	digits, ok := strings.CutSuffix(rest, ".service")
	if !ok {
		return 0, false
	}
	uid, err := strconv.Atoi(digits)
	if err != nil || strconv.Itoa(uid) != digits {
		return 0, false
	}
	return uid, true
}

// administratorNames are the members of the administrator groups the system
// has (sudo, admin, wheel), as /etc/group lists them.
func administratorNames(groupText string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(groupText, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 4 {
			continue
		}
		admin := false
		for _, g := range lingerAdminGroups {
			if fields[0] == g {
				admin = true
			}
		}
		if !admin {
			continue
		}
		for _, member := range strings.Split(fields[3], ",") {
			if member = strings.TrimSpace(member); member != "" {
				out[member] = true
			}
		}
	}
	return out
}

// closeSiteUserManagers masks the service manager of every site account's
// uid and removes CelikPanel's masks of uids that no longer belong to one.
func closeSiteUserManagers() *transport.SideDoorsUserManagers {
	userManagerMu.Lock()
	defer userManagerMu.Unlock()
	out := &transport.SideDoorsUserManagers{}
	dir, err := sandboxUnitDir()
	if err != nil {
		out.Detail = err.Error()
		return out
	}
	entries, _, err := readPasswd(sideDoorsPasswd)
	if err != nil {
		out.Detail = "the account list could not be read: " + err.Error()
		return out
	}
	groupText, _ := os.ReadFile(sideDoorsGroup)
	admins := administratorNames(string(groupText))
	siteUIDs := map[int]string{}
	adminUIDs := map[int]bool{0: true}
	for _, e := range entries {
		if admins[e.Name] {
			adminUIDs[e.UID] = true
		}
		if hostingpath.IsSiteHome(e.Home) && e.UID >= 1000 {
			if _, seen := siteUIDs[e.UID]; !seen {
				siteUIDs[e.UID] = e.Name
			}
		}
	}
	uids := make([]int, 0, len(siteUIDs))
	for uid := range siteUIDs {
		if !adminUIDs[uid] {
			uids = append(uids, uid)
		}
	}
	sort.Ints(uids)
	ledger := readSandboxLedger()
	reload := false
	for _, uid := range uids {
		name := siteUIDs[uid]
		path := filepath.Join(dir, userManagerUnit(uid))
		_, recorded := ledger[path]
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if recorded {
				// CelikPanel's mask was removed on the server: the owner's
				// choice stands.
				out.Removed = append(out.Removed, name)
				out.OpenUnits = append(out.OpenUnits, userManagerUnit(uid))
				continue
			}
			if err := writeUserManagerMask(path); err != nil {
				out.Failed = append(out.Failed, name)
				out.OpenUnits = append(out.OpenUnits, userManagerUnit(uid))
				out.Detail = err.Error()
				continue
			}
			if err := setSandboxLedgerEntry(path, userManagerMaskKind, userManagerMaskTarget); err != nil {
				log.Printf("side doors: record %s: %v", path, err)
			}
			out.Masked++
			reload = true
		case err != nil:
			out.Failed = append(out.Failed, name)
			out.OpenUnits = append(out.OpenUnits, userManagerUnit(uid))
			out.Detail = err.Error()
		case isUserManagerMask(path, info):
			if recorded {
				out.Masked++
			} else {
				out.OwnerMasked++ // the owner's own mask: closed, kept
			}
		default:
			// A file or another link of that name: the server owner's, kept.
			out.Owner = append(out.Owner, name)
			out.OpenUnits = append(out.OpenUnits, userManagerUnit(uid))
		}
	}
	// CelikPanel's masks of uids that are no longer a site account's (a
	// deleted site; never an administrator's) go.
	keep := map[int]bool{}
	for _, uid := range uids {
		keep[uid] = true
	}
	for path, entry := range ledger {
		if entry.Kind != userManagerMaskKind {
			continue
		}
		uid, ok := userManagerUID(path)
		if !ok || keep[uid] {
			continue
		}
		if info, err := os.Lstat(path); err == nil {
			if !isUserManagerMask(path, info) {
				// Replaced by the owner meanwhile: theirs now, left as it is.
				_ = forgetUserManagerMask(path)
				continue
			}
			if err := os.Remove(path); err != nil {
				log.Printf("side doors: remove %s: %v", path, err)
				continue
			}
			reload = true
		}
		if err := forgetUserManagerMask(path); err != nil {
			log.Printf("side doors: forget %s: %v", path, err)
		}
	}
	if reload {
		if err := sandboxDaemonReload(); err != nil {
			out.Detail = err.Error()
		}
	}
	return out
}

// isUserManagerMask: the path is a link to /dev/null (a mask).
func isUserManagerMask(path string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	target, err := os.Readlink(path)
	return err == nil && target == userManagerMaskTarget
}

// writeUserManagerMask makes the link beside its place and renames it in.
func writeUserManagerMask(path string) error {
	tmp := filepath.Join(filepath.Dir(path), ".celikpanel-"+filepath.Base(path)+".tmp")
	_ = os.Remove(tmp)
	if err := os.Symlink(userManagerMaskTarget, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func forgetUserManagerMask(path string) error {
	return updateSandboxLedger(func(l map[string]sandboxLedgerEntry) bool {
		if _, ok := l[path]; !ok {
			return false
		}
		delete(l, path)
		return true
	})
}

// syncSiteUserManagers is the step at a site's creation and deletion: the
// new account's mask written, a deleted account's removed.
func syncSiteUserManagers() error {
	out := closeSiteUserManagers()
	if len(out.Failed) > 0 {
		return errors.New("the service manager of " + strings.Join(out.Failed, ", ") + " could not be closed: " + out.Detail)
	}
	return nil
}
