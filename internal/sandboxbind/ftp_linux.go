//go:build linux

package sandboxbind

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/fileaccess"
)

// FTPHelper binds login jails (ftp-bind, ftp-unbind).
type FTPHelper struct {
	// JailRoot is fileaccess.LoginJailRoot in production; RootUID owns it.
	JailRoot string
	RootUID  uint32
	Mounter  Mounter
	// Plan is PlanForLoginFolder in production.
	Plan func(folder string, siteUID uint32) (Plan, error)
	// LookupAccount reads one local account.
	LookupAccount func(name string) (FTPAccount, error)
	// GroupPrivate proves that only accounts with uid may use gid: no other
	// account has it as primary group, and it has no other member.
	GroupPrivate func(gid, uid uint32) error
	Log          io.Writer
}

func (h FTPHelper) jail(login string) string  { return h.JailRoot + "/" + login }
func (h FTPHelper) files(login string) string { return h.jail(login) + "/files" }

// Check proves everything Bind proves, without changing anything: the
// Agent's own check before it writes a login's unit (the helper checks
// again at every start).
func (h FTPHelper) Check(req FTPRequest) error {
	fd, _, err := h.open(req)
	if err != nil {
		return err
	}
	unix.Close(fd)
	return nil
}

func (h FTPHelper) open(req FTPRequest) (int, FTPAccount, error) {
	if err := ValidFTPRequest(req); err != nil {
		return -1, FTPAccount{}, err
	}
	site, err := h.LookupAccount(req.Site)
	if err != nil {
		return -1, FTPAccount{}, refused(CodeNotOwned, "", "the site account %s could not be found: %v", req.Site, err)
	}
	login, err := h.LookupAccount(req.Login)
	if err != nil {
		return -1, FTPAccount{}, refused(CodeNotOwned, "", "the login %s could not be found: %v", req.Login, err)
	}
	plan, err := h.Plan(req.Folder, site.UID)
	if err != nil {
		return -1, FTPAccount{}, err
	}
	if err := CheckLoginAccounts(req, plan, site, login); err != nil {
		return -1, FTPAccount{}, err
	}
	if err := h.GroupPrivate(site.GID, site.UID); err != nil {
		return -1, FTPAccount{}, refused(CodeNotOwned, "", "the group of the site account %s is not its own: %v", req.Site, err)
	}
	fd, _, err := Open(plan)
	if err != nil {
		return -1, FTPAccount{}, err
	}
	return fd, site, nil
}

// Bind checks the login and its folder and binds the folder onto the jail.
// Any error leaves no jail behind: sshd and vsftpd then refuse the login.
func (h FTPHelper) Bind(req FTPRequest) (err error) {
	fd, site, err := h.open(req)
	if err != nil {
		// A jail of an earlier start must not outlive a failed check.
		if cleanErr := h.Unbind(req.Login); cleanErr != nil && h.Log != nil {
			fmt.Fprintf(h.Log, "celikpanel-ftp: removing the earlier jail of %s: %v\n", req.Login, cleanErr)
		}
		return err
	}
	defer unix.Close(fd)
	defer func() {
		if err != nil {
			if cleanErr := h.Unbind(req.Login); cleanErr != nil && h.Log != nil {
				fmt.Fprintf(h.Log, "celikpanel-ftp: removing the jail of %s: %v\n", req.Login, cleanErr)
			}
		}
	}()
	if err := h.jailDir(h.JailRoot, 0o711, h.RootUID); err != nil {
		return err
	}
	if err := h.jailDir(h.jail(req.Login), 0o750, site.GID); err != nil {
		return err
	}
	target := h.files(req.Login)
	if err := h.unmountAll(target); err != nil {
		return err
	}
	if err := h.jailDir(target, 0o700, h.RootUID); err != nil {
		return err
	}
	if err := h.Mounter.Bind(fd, target); err != nil {
		return fmt.Errorf("the folder could not be bound onto %s: %v", target, err)
	}
	return nil
}

// Unbind removes the mount and the jail (the unit's stop). A missing jail is
// nothing to do.
func (h FTPHelper) Unbind(login string) error {
	if err := fileaccess.ValidateLoginName(login); err != nil {
		return fmt.Errorf("not a login name: %v", err)
	}
	dir := h.jail(login)
	fi, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a folder; remove it by hand", dir)
	}
	if err := h.unmountAll(h.files(login)); err != nil {
		return err
	}
	if err := os.Remove(h.files(login)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (h FTPHelper) unmountAll(target string) error {
	for i := 0; i < 16; i++ {
		err := h.Mounter.Unmount(target)
		if errors.Is(err, ErrNotMounted) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("the earlier mount at %s could not be removed: %v", target, err)
		}
	}
	return fmt.Errorf("more than 16 mounts are stacked at %s", target)
}

// jailDir makes p a real folder owned by RootUID with group gid and the
// given mode. Only root can create anything in the folders above it.
func (h FTPHelper) jailDir(p string, mode uint32, gid uint32) error {
	if err := os.Mkdir(p, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("%s could not be made: %v", p, err)
	}
	var st unix.Stat_t
	if err := unix.Lstat(p, &st); err != nil {
		return fmt.Errorf("%s could not be read: %v", p, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != h.RootUID {
		return fmt.Errorf("%s is not a folder of root's", p)
	}
	if st.Gid != gid {
		if err := unix.Lchown(p, int(h.RootUID), int(gid)); err != nil {
			return fmt.Errorf("%s: %v", p, err)
		}
	}
	if st.Mode&0o7777 != mode {
		if err := unix.Fchmodat(unix.AT_FDCWD, p, mode, 0); err != nil {
			return fmt.Errorf("%s: %v", p, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Local account files. Hosting accounts are always local (useradd), so the
// helper reads the files directly, never through NSS.

var (
	ftpPasswdPath = "/etc/passwd"
	ftpGroupPath  = "/etc/group"
)

type passwdEntry struct {
	name       string
	uid, gid   uint32
	home, shel string
}

func readPasswdEntries() ([]passwdEntry, error) {
	content, err := os.ReadFile(ftpPasswdPath)
	if err != nil {
		return nil, err
	}
	var out []passwdEntry
	for _, raw := range strings.Split(string(content), "\n") {
		fields := strings.Split(raw, ":")
		if len(fields) != 7 {
			continue
		}
		uid, err1 := strconv.ParseUint(fields[2], 10, 32)
		gid, err2 := strconv.ParseUint(fields[3], 10, 32)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, passwdEntry{name: fields[0], uid: uint32(uid), gid: uint32(gid), home: fields[5], shel: fields[6]})
	}
	return out, nil
}

// LookupLocalAccount reads one account from /etc/passwd. A name listed twice
// is refused.
func LookupLocalAccount(name string) (FTPAccount, error) {
	entries, err := readPasswdEntries()
	if err != nil {
		return FTPAccount{}, err
	}
	var found []passwdEntry
	for _, e := range entries {
		if e.name == name {
			found = append(found, e)
		}
	}
	switch len(found) {
	case 0:
		return FTPAccount{}, fmt.Errorf("%s is not in %s", name, ftpPasswdPath)
	case 1:
		e := found[0]
		return FTPAccount{Name: e.name, UID: e.uid, GID: e.gid, Home: e.home, Shell: e.shel}, nil
	}
	return FTPAccount{}, fmt.Errorf("%s is listed %d times in %s", name, len(found), ftpPasswdPath)
}

// LocalGroupPrivate proves gid is private to uid: every account whose
// primary group is gid has uid, and every member listed for gid in
// /etc/group has uid too.
func LocalGroupPrivate(gid, uid uint32) error {
	entries, err := readPasswdEntries()
	if err != nil {
		return err
	}
	uidOf := map[string]uint32{}
	for _, e := range entries {
		uidOf[e.name] = e.uid
		if e.gid == gid && e.uid != uid {
			return fmt.Errorf("the account %s (uid %d) also has group %d", e.name, e.uid, gid)
		}
	}
	content, err := os.ReadFile(ftpGroupPath)
	if err != nil {
		return err
	}
	for _, raw := range strings.Split(string(content), "\n") {
		fields := strings.Split(raw, ":")
		if len(fields) != 4 || fields[2] != strconv.FormatUint(uint64(gid), 10) {
			continue
		}
		for _, member := range strings.Split(fields[3], ",") {
			member = strings.TrimSpace(member)
			if member == "" {
				continue
			}
			if memberUID, ok := uidOf[member]; !ok || memberUID != uid {
				return fmt.Errorf("%s is a member of group %s", member, fields[0])
			}
		}
	}
	return nil
}

// ProductionFTPHelper is the helper as the login units run it.
func ProductionFTPHelper(log io.Writer) FTPHelper {
	return FTPHelper{
		JailRoot: fileaccess.LoginJailRoot, RootUID: 0, Mounter: KernelMounter{},
		Plan: PlanForLoginFolder, LookupAccount: LookupLocalAccount, GroupPrivate: LocalGroupPrivate, Log: log,
	}
}

// CheckLoginFolder is the Agent's check before it writes or starts a login's
// unit: everything ftp-bind checks, nothing mounted.
func CheckLoginFolder(req FTPRequest) error {
	return ProductionFTPHelper(nil).Check(req)
}

// CheckSiteFolder is the Agent's check of a chosen folder before the login
// exists: the site account, its group and the walk to the folder.
func CheckSiteFolder(site, folder string) error {
	h := ProductionFTPHelper(nil)
	if !fileaccess.ValidUsername(site) || site == "root" {
		return errors.New("not a site account name")
	}
	account, err := h.LookupAccount(site)
	if err != nil {
		return refused(CodeNotOwned, "", "the site account %s could not be found: %v", site, err)
	}
	plan, err := h.Plan(folder, account.UID)
	if err != nil {
		return err
	}
	if account.UID < 1000 {
		return refused(CodeNotOwned, "", "the site account %s is a system account", site)
	}
	if strings.TrimSuffix(account.Home, "/") != plan.Home {
		return refused(CodeNotOwned, plan.AppDir, "is not in the home of the site account %s", site)
	}
	if err := h.GroupPrivate(account.GID, account.UID); err != nil {
		return refused(CodeNotOwned, "", "the group of the site account %s is not its own: %v", site, err)
	}
	return Check(plan)
}
