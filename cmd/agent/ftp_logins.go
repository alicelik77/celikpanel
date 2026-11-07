package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/sandboxbind"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Additional FTP/SFTP logins of a site (alpha.128, gap G16,
// docs/FTP-LOGINS.md; D-022, D-024, D-025).
//
// A login is a local account with the site account's uid and primary group
// (useradd -o), a no-login shell and no key, so its uploads belong to the
// site like the site account's own. It is closed into the root-owned jail
// /run/celikpanel-ftp/<login>, onto whose "files" folder the sandbox helper
// binds the chosen folder of the site by descriptor (internal/sandboxbind,
// ftp-bind). The unit celikpanel-ftp-<login>.service does that at every
// boot. sshd gets one Match block per login (ChrootDirectory, ForceCommand
// internal-sftp), vsftpd one user file and the user list entry. The sshd
// drop-in's login lines are the record of who has a login; there is no other
// copy on the server.
//
// A change runs in steps: check, account (locked), jail (unit, helper
// check, mount), sshd (sshd -t, sshd -T for the login, reload), FTP (vsftpd,
// TLS handshake), password, verify. A failed step puts back what the earlier
// steps did. The password is set last: until then the account is locked and
// nothing can sign in with it. Files an owner edited (the sshd drop-in, a
// login's unit) are never overwritten: the change is refused with the reason.
//
// Sitenin ek FTP/SFTP girişleri. Giriş sitenin uid'iyle ayrı bir hesaptır;
// seçilen klasör root'a ait hapse bağlanır. Değişiklik adım adımdır; başarısız
// adım öncekileri geri alır, parola en son verilir.

// Seams: production values, replaced by component tests.
var (
	ftpLoginCheckSiteFolder = sandboxbind.CheckSiteFolder
	ftpLoginCheckFolder     = sandboxbind.CheckLoginFolder
	ftpLoginEnsureHelper    = ensureSandboxHelper
	ftpLoginKillSessions    = killFTPLoginSessions
	ftpLoginJailMounted     = ftpLoginMountPresent
	ftpLoginProbeFTP        = probeFTPLogin
	ftpLoginToday           = func() int64 { return time.Now().Unix() / 86400 }
)

func init() {
	fileAccessCommands["useradd"] = []string{"/usr/sbin/useradd", "/usr/bin/useradd"}
	fileAccessCommands["userdel"] = []string{"/usr/sbin/userdel", "/usr/bin/userdel"}
	fileAccessCommands["chage"] = []string{"/usr/bin/chage", "/usr/sbin/chage"}
	fileAccessCommands["journalctl"] = []string{"/usr/bin/journalctl", "/bin/journalctl"}
}

type ftpLoginRun struct {
	ctx      context.Context
	site     transport.FTPLoginSite
	account  fileAccessAccount
	req      transport.ApplyFTPLoginRequest
	steps    []transport.FTPLoginStep
	warnings []string
	undo     []func() error
}

func (r *ftpLoginRun) step(name, status, detail string) {
	r.steps = append(r.steps, transport.FTPLoginStep{Step: name, Status: status, Detail: detail})
}

func (r *ftpLoginRun) warn(code string) {
	for _, existing := range r.warnings {
		if existing == code {
			return
		}
	}
	r.warnings = append(r.warnings, code)
}

// rollback runs the undo actions in reverse order and says how it went.
func (r *ftpLoginRun) rollback() string {
	var failed []string
	for i := len(r.undo) - 1; i >= 0; i-- {
		if err := r.undo[i](); err != nil {
			failed = append(failed, err.Error())
		}
	}
	r.undo = nil
	if len(failed) > 0 {
		r.step("cleanup", "failed", strings.Join(failed, "; "))
		return "; putting back the earlier steps failed: " + strings.Join(failed, "; ") + " (the login stays closed: it has no password)"
	}
	r.step("cleanup", "rolled_back", "")
	return "; the earlier steps were put back"
}

type ftpLoginRefusal struct {
	code   string
	detail string
}

func refuseFTPLogin(code, format string, args ...any) *ftpLoginRefusal {
	return &ftpLoginRefusal{code: code, detail: fmt.Sprintf(format, args...)}
}

// ---------------------------------------------------------------------------
// RPCs

// FTPLoginsStateV1 reads the logins of one site (read only).
func (a *Agent) FTPLoginsStateV1(req *transport.FTPLoginsStateRequest, resp *transport.FTPLoginsStateResponse) error {
	if req == nil || resp == nil {
		return errors.New("FTP logins state request and response are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	*resp = readFTPLoginsState(ctx, req.Site)
	return nil
}

// ApplyFTPLoginV1 adds, changes or removes one login.
func (a *Agent) ApplyFTPLoginV1(req *transport.ApplyFTPLoginRequest, resp *transport.ApplyFTPLoginResponse) error {
	if req == nil || resp == nil {
		return errors.New("FTP login request and response are required")
	}
	*resp = transport.ApplyFTPLoginResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing FTP logins"); err != nil {
		return err
	}
	fileAccessMu.Lock()
	defer fileAccessMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), fileAccessTimeout)
	defer cancel()
	run := &ftpLoginRun{ctx: ctx, site: req.Site, req: *req}
	var refusal *ftpLoginRefusal
	switch req.Action {
	case transport.FTPLoginAdd:
		refusal = run.add()
	case transport.FTPLoginUpdate:
		refusal = run.update()
	case transport.FTPLoginRemove:
		refusal = run.remove()
	default:
		refusal = refuseFTPLogin(transport.FTPLoginRefusedInvalid, "unknown action %q", req.Action)
	}
	if refusal != nil {
		resp.Refusal, resp.Detail = refusal.code, refusal.detail
	} else {
		resp.Applied = true
	}
	resp.Steps = run.steps
	resp.Warnings = run.warnings
	resp.State = readFTPLoginsState(ctx, req.Site)
	return nil
}

// ---------------------------------------------------------------------------
// Checks shared by every action

// checkCommon proves the site account and reads the record. The drop-in
// must be CelikPanel's (or absent): an owner's file is never overwritten.
func (r *ftpLoginRun) checkCommon() ([]fileaccess.Entry, []fileaccess.Login, []byte, bool, *ftpLoginRefusal) {
	account, refusal := verifiedFileAccessAccount(r.site)
	if refusal != nil {
		return nil, nil, nil, false, refuseFTPLogin(transport.FTPLoginRefusedSiteAccount, "%s", refusal.detail)
	}
	r.account = account
	if !fileAccessCommandPresent("sshd") {
		return nil, nil, nil, false, refuseFTPLogin(transport.FTPLoginRefusedNoSSHD,
			"this server has no OpenSSH server, which FTP and SFTP logins need; the administrator installs it with the operating system's package manager (openssh-server, or openssh on Arch)")
	}
	entries, logins, state, before, exists, err := readFileAccessEntries()
	switch {
	case err != nil:
		return nil, nil, nil, false, refuseFTPLogin(transport.FTPLoginRefusedSSHDOwner, "read %s: %v; nothing was changed", fileaccess.SSHDropInPath, err)
	case state == fileaccess.FileOwner:
		return nil, nil, nil, false, refuseFTPLogin(transport.FTPLoginRefusedSSHDOwner,
			"%s was changed by hand, so CelikPanel keeps it and changes nothing. The administrator removes it or puts CelikPanel's version back (the Access tab of a site shows it), then tries again", fileaccess.SSHDropInPath)
	}
	return entries, logins, before, exists, nil
}

// unitWrite plans a login's unit; an owner's unit is refused.
func ftpLoginUnitWrite(l fileaccess.Login) (*fileAccessFileWrite, *ftpLoginRefusal) {
	text, err := fileaccess.RenderLoginUnit(l, sitesandbox.HelperPath)
	if err != nil {
		return nil, refuseFTPLogin(transport.FTPLoginRefusedInvalid, "%v", err)
	}
	path := fileaccess.LoginUnitPath(l.Login)
	before, exists, err := fileAccessReadFile(path)
	if err != nil {
		return nil, refuseFTPLogin(transport.FTPLoginRefusedUnitOwner, "read %s: %v; nothing was changed", path, err)
	}
	if state, _ := fileaccess.Classify(before, exists); state == fileaccess.FileOwner {
		return nil, refuseFTPLogin(transport.FTPLoginRefusedUnitOwner,
			"%s was changed by hand, so CelikPanel keeps it and changes nothing. The administrator removes it or puts CelikPanel's version back, then tries again", path)
	}
	return &fileAccessFileWrite{path: path, content: text, mode: 0o644, before: before, existed: exists}, nil
}

// folderRefusal words a failed folder check (D-024).
func ftpLoginFolderRefusal(folder string, err error) *ftpLoginRefusal {
	if r, ok := sandboxbind.AsRefusal(err); ok {
		switch r.Code {
		case sandboxbind.CodeNotReal:
			return refuseFTPLogin(transport.FTPLoginRefusedFolder,
				"the folder %s cannot be used: %s. It must be a real folder of this site, with no link on its way; create it (or put the real folder back) with the file manager, then try again", folder, r.Error())
		case sandboxbind.CodeNotOwned:
			return refuseFTPLogin(transport.FTPLoginRefusedFolder,
				"the folder %s cannot be used: %s. The administrator gives it back to the site's account (and removes other accounts' write permission), then try again", folder, r.Error())
		}
		return refuseFTPLogin(transport.FTPLoginRefusedFolder, "the folder %s could not be checked: %s; nothing was changed", folder, r.Error())
	}
	return refuseFTPLogin(transport.FTPLoginRefusedFolder, "the folder %s could not be checked: %v; nothing was changed", folder, err)
}

// ---------------------------------------------------------------------------
// Add

func (r *ftpLoginRun) add() *ftpLoginRefusal {
	req := r.req
	if err := fileaccess.ValidateLoginName(req.Login); err != nil {
		return refuseFTPLogin(transport.FTPLoginRefusedInvalid, "%v", err)
	}
	folder, err := fileaccess.CleanLoginFolder(req.Folder)
	if err != nil {
		return refuseFTPLogin(transport.FTPLoginRefusedInvalid, "%v", err)
	}
	if req.Password == "" {
		return refuseFTPLogin(transport.FTPLoginRefusedInvalid, "a new login needs a password")
	}
	if err := fileaccess.ValidatePassword(req.Password); err != nil {
		return refuseFTPLogin(transport.FTPLoginRefusedInvalid, "%v", err)
	}
	entries, logins, before, exists, refusal := r.checkCommon()
	if refusal != nil {
		return refusal
	}
	login := fileaccess.Login{Login: req.Login, Site: r.site.Username, SubscriptionID: r.site.SubscriptionID,
		DomainID: r.site.DomainID, Folder: folder, ReadOnly: req.ReadOnly}
	if _, has := fileaccess.FindLogin(logins, login.Login); has {
		return refuseFTPLogin(transport.FTPLoginRefusedNameTaken, "the login %s exists already; choose another name", login.Login)
	}
	if _, has := fileaccess.FindEntry(entries, login.Login); has {
		return refuseFTPLogin(transport.FTPLoginRefusedNameTaken, "%s is the name of a site's account; choose another name", login.Login)
	}
	if _, err := fileAccessLookupAccount(login.Login); err == nil {
		return refuseFTPLogin(transport.FTPLoginRefusedNameTaken, "an account named %s exists on this server already; choose another name", login.Login)
	} else if !errors.Is(err, errFileAccessNoAccount) {
		return refuseFTPLogin(transport.FTPLoginRefusedNameTaken, "read the accounts: %v; nothing was changed", err)
	}
	if groups, _, err := readGroupFile(); err != nil {
		return refuseFTPLogin(transport.FTPLoginRefusedNameTaken, "read the groups: %v; nothing was changed", err)
	} else {
		for _, name := range groups {
			if name == login.Login {
				return refuseFTPLogin(transport.FTPLoginRefusedNameTaken, "a group named %s exists on this server already; choose another name", login.Login)
			}
		}
	}
	if len(logins) >= fileaccess.MaxLoginsPerServer {
		return refuseFTPLogin(transport.FTPLoginRefusedLimit, "this server has %d logins, the most one server keeps; remove one first", len(logins))
	}
	unit, refusal := ftpLoginUnitWrite(login)
	if refusal != nil {
		return refusal
	}
	folderPath := fileaccess.LoginFolderPath(login.SubscriptionID, login.DomainID, login.Folder)
	if err := ftpLoginCheckSiteFolder(login.Site, folderPath); err != nil {
		return ftpLoginFolderRefusal(login.Folder, err)
	}
	r.step("check", "done", "")

	// Account: locked until the very last step.
	if refusal := r.createAccount(login); refusal != nil {
		return refusal
	}
	if refusal := r.startJail(login, unit); refusal != nil {
		refusal.detail += r.rollback()
		return refusal
	}
	plan := &fileAccessPlan{req: transport.ApplyFileAccessRequest{Enabled: true}, entries: entries,
		logins: fileaccess.MergeLogin(logins, login, true)}
	if refusal := r.writeDropIn(plan, before, exists, login); refusal != nil {
		refusal.detail += r.rollback()
		return refusal
	}
	r.applyFTP(plan)
	if refusal := r.setPassword(login.Login); refusal != nil {
		refusal.detail += r.rollback()
		return refusal
	}
	r.undo = nil
	r.verify(login, true)
	return nil
}

func (r *ftpLoginRun) createAccount(login fileaccess.Login) *ftpLoginRefusal {
	ctx := r.ctx
	if out, err := fileAccessRun(ctx, nil, "groupadd", "-r", "-f", fileaccess.SiteGroup); err != nil {
		r.step("account", "failed", firstLine(string(out)))
		return refuseFTPLogin(transport.FTPLoginRefusedAccount, "create group %s: %s; nothing was changed", fileaccess.SiteGroup, firstLine(string(out)))
	}
	// -o: the site account's uid (files stay the site's); -M: no home is
	// made (the jail is the helper's); the password stays locked ("!").
	out, err := fileAccessRun(ctx, nil, "useradd", "-o", "-u", strconv.Itoa(r.account.UID), "-g", strconv.Itoa(r.account.GID),
		"-G", fileaccess.SiteGroup, "-M", "-d", fileaccess.LoginHome(login.Login), "-s", fileaccess.NoLoginShell,
		"-c", "CelikPanel FTP login of "+login.Site, login.Login)
	if err != nil {
		r.step("account", "failed", firstLine(string(out)))
		// useradd may have written part of the account: remove what is ours.
		if created, lookupErr := fileAccessLookupAccount(login.Login); lookupErr == nil && created.UID == r.account.UID && created.Home == fileaccess.LoginHome(login.Login) {
			_, _ = fileAccessRun(ctx, nil, "userdel", "-f", login.Login)
		}
		return refuseFTPLogin(transport.FTPLoginRefusedAccount, "create the account %s: %s; nothing was changed", login.Login, firstLine(string(out)))
	}
	r.undo = append(r.undo, func() error { return deleteFTPLoginAccount(ctx, login.Login, r.account.UID) })
	// A login of a suspended site is suspended too.
	if expiry, err := ftpLoginExpiry(r.site.Username); err == nil && expiry != "" {
		if out, err := fileAccessRun(ctx, nil, "chage", chageExpiryArgs(login.Login, expiry)...); err != nil {
			r.step("account", "failed", firstLine(string(out)))
			return refuseFTPLogin(transport.FTPLoginRefusedAccount, "give %s the site account's expiry: %s%s", login.Login, firstLine(string(out)), r.rollback())
		}
	}
	r.step("account", "done", "")
	return nil
}

// deleteFTPLoginAccount removes a login's account: only an account that is
// provably a login (its jail home and the given uid). userdel -f: the
// site's own processes run with the same uid; nothing else is removed (no
// -r, and the jail is never its home folder on disk).
func deleteFTPLoginAccount(ctx context.Context, login string, uid int) error {
	account, err := fileAccessLookupAccount(login)
	if errors.Is(err, errFileAccessNoAccount) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the account %s: %v", login, err)
	}
	if account.Home != fileaccess.LoginHome(login) || (uid >= 0 && account.UID != uid) {
		return fmt.Errorf("the account %s is not a login of this site (home %s); it is kept", login, account.Home)
	}
	if groups, _, err := readGroupFile(); err == nil {
		for _, name := range groups {
			if name == login {
				// userdel -f would remove a group of this name even when it
				// is another account's primary group.
				return fmt.Errorf("a group named %s exists; the administrator removes the account by hand (userdel %s)", login, login)
			}
		}
	}
	if out, err := fileAccessRun(ctx, nil, "userdel", "-f", login); err != nil {
		return fmt.Errorf("userdel %s: %s", login, firstLine(string(out)))
	}
	return nil
}

func (r *ftpLoginRun) startJail(login fileaccess.Login, unit *fileAccessFileWrite) *ftpLoginRefusal {
	ctx := r.ctx
	folderPath := fileaccess.LoginFolderPath(login.SubscriptionID, login.DomainID, login.Folder)
	if err := ftpLoginCheckFolder(sandboxbind.FTPRequest{Login: login.Login, Site: login.Site, Folder: folderPath}); err != nil {
		r.step("jail", "failed", err.Error())
		return ftpLoginFolderRefusal(login.Folder, err)
	}
	if err := ftpLoginEnsureHelper(); err != nil {
		r.step("jail", "failed", err.Error())
		return refuseFTPLogin(transport.FTPLoginRefusedJail, "install the sandbox helper: %v", err)
	}
	unitName := fileaccess.LoginUnit(login.Login)
	if err := writeFileAccessFile(unit); err != nil {
		r.step("jail", "failed", err.Error())
		return refuseFTPLogin(transport.FTPLoginRefusedJail, "write %s: %v", unit.path, err)
	}
	r.undo = append(r.undo, func() error {
		_, _ = fileAccessRun(ctx, nil, "systemctl", "disable", "--now", unitName)
		if err := restoreFileAccessFile(unit); err != nil {
			return fmt.Errorf("put %s back: %v", unit.path, err)
		}
		_, _ = fileAccessRun(ctx, nil, "systemctl", "daemon-reload")
		if unit.existed {
			_, _ = fileAccessRun(ctx, nil, "systemctl", "restart", unitName)
		}
		return nil
	})
	if out, err := fileAccessRun(ctx, nil, "systemctl", "daemon-reload"); err != nil {
		r.step("jail", "failed", firstLine(string(out)))
		return refuseFTPLogin(transport.FTPLoginRefusedJail, "systemctl daemon-reload: %s", firstLine(string(out)))
	}
	if out, err := fileAccessRun(ctx, nil, "systemctl", "enable", unitName); err != nil {
		r.step("jail", "failed", firstLine(string(out)))
		return refuseFTPLogin(transport.FTPLoginRefusedJail, "systemctl enable %s: %s", unitName, firstLine(string(out)))
	}
	// restart: a unit that ran with another folder binds anew.
	if _, err := fileAccessRun(ctx, nil, "systemctl", "restart", unitName); err != nil || !fileAccessUnitActive(ctx, unitName) || !ftpLoginJailMounted(login.Login) {
		reason := ftpLoginJailProblem(ctx, login.Login)
		r.step("jail", "failed", reason)
		return refuseFTPLogin(transport.FTPLoginRefusedJail, "the jail of %s could not be made: %s", login.Login, reason)
	}
	r.step("jail", "done", fileaccess.LoginJail(login.Login))
	return nil
}

// ftpLoginJailProblem is the helper's own words from the unit's journal.
func ftpLoginJailProblem(ctx context.Context, login string) string {
	out, err := fileAccessRun(ctx, nil, "journalctl", "-u", fileaccess.LoginUnit(login), "-n", "8", "-o", "cat", "--no-pager")
	if err == nil {
		var lines, other []string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case line == "":
			case strings.HasPrefix(line, "celikpanel-ftp: the login ") && strings.Contains(line, " has no jail"):
				// The helper's reason; the screen words who acts.
				lines = append(lines, strings.TrimPrefix(line, "celikpanel-ftp: "))
			case strings.Contains(line, ".service: ") && !strings.Contains(line, "Failed with result") && !strings.Contains(line, "Main process exited"):
				// systemd's own words (a missing helper, an exec failure).
				other = append(other, line[strings.Index(line, ".service: ")+len(".service: "):])
			}
		}
		if len(lines) > 0 {
			return lines[len(lines)-1]
		}
		if len(other) > 0 {
			return other[0]
		}
	}
	return "the unit " + fileaccess.LoginUnit(login) + " did not start; see systemctl status " + fileaccess.LoginUnit(login)
}

// writeDropIn writes the drop-in with the login and proves sshd applies it.
func (r *ftpLoginRun) writeDropIn(plan *fileAccessPlan, before []byte, exists bool, login fileaccess.Login) *ftpLoginRefusal {
	text, err := fileaccess.RenderSSHDropIn(plan.entries, plan.logins)
	if err != nil {
		r.step("sshd", "failed", err.Error())
		return refuseFTPLogin(transport.FTPLoginRefusedInvalid, "%v", err)
	}
	if exists && string(before) == text {
		r.step("sshd", "done", "unchanged")
		return nil
	}
	plan.dropIn = &fileAccessFileWrite{path: fileaccess.SSHDropInPath, content: text, mode: 0o644, before: before, existed: exists}
	var refusal *fileAccessRefusal
	if r.req.Action == transport.FTPLoginRemove {
		refusal = applySSHDropInWrite(r.ctx, plan, login.Login, map[string]string{"denygroups": fileaccess.SiteGroup})
	} else {
		refusal = applySSHDropInWrite(r.ctx, plan, login.Login, fileaccess.ExpectedLoginSSHDEffective(login))
	}
	if refusal != nil {
		r.step("sshd", "failed", refusal.detail)
		code := transport.FTPLoginRefusedSSHDRejected
		if refusal.code == transport.FileAccessRefusedSSHDInclude {
			code = transport.FTPLoginRefusedSSHDInclude
		}
		return refuseFTPLogin(code, "%s", refusal.detail)
	}
	write := plan.dropIn
	ctx := r.ctx
	r.undo = append(r.undo, func() error {
		if err := restoreFileAccessFile(write); err != nil {
			return fmt.Errorf("put %s back: %v", write.path, err)
		}
		for _, unit := range fileAccessSSHUnits {
			if fileAccessUnitActive(ctx, unit) {
				_, _ = fileAccessRun(ctx, nil, "systemctl", "reload", unit)
			}
		}
		return nil
	})
	r.step("sshd", "done", "")
	return nil
}

// applyFTP writes vsftpd's side. FTP is optional: SFTP works without it, so
// a failure is a warning (the earlier FTP configuration is put back).
func (r *ftpLoginRun) applyFTP(plan *fileAccessPlan) {
	planFileAccessFTP(r.ctx, plan)
	applyFileAccessFTP(r.ctx, plan)
	for _, code := range plan.warnings {
		switch code {
		case transport.FileAccessWarnFTPNotInstalled:
			r.warn(transport.FTPLoginWarnFTPNotInstalled)
		case transport.FileAccessWarnFTPOwnerConfig:
			r.warn(transport.FTPLoginWarnFTPOwnerConfig)
		case transport.FileAccessWarnFTPRejected:
			r.warn(transport.FTPLoginWarnFTPRejected)
		}
	}
	switch {
	case !plan.ftpManaged:
		r.step("ftp", "skipped", "")
	case plan.ftpFailed:
		r.step("ftp", "failed", "the previous FTP configuration is back; SFTP works")
	default:
		r.step("ftp", "done", "")
	}
}

func (r *ftpLoginRun) setPassword(login string) *ftpLoginRefusal {
	// chpasswd hashes with the system's own method; the password reaches it
	// on stdin only and its output never carries it.
	if out, err := fileAccessRun(r.ctx, []byte(login+":"+r.req.Password+"\n"), "chpasswd"); err != nil {
		r.step("password", "failed", firstLine(string(out)))
		return refuseFTPLogin(transport.FTPLoginRefusedAccount, "set the password of %s: %s", login, firstLine(string(out)))
	}
	r.step("password", "done", "")
	return nil
}

// verify reads back what a client meets: sshd's effective setting for the
// login, the jail, and an FTP sign-in as the login when vsftpd is CelikPanel's.
func (r *ftpLoginRun) verify(login fileaccess.Login, withPassword bool) {
	ctx := r.ctx
	var notes []string
	if out, err := fileAccessRun(ctx, nil, "sshd", "-T", "-C", sshdConnectionSpec(login.Login)); err == nil &&
		effectiveSSHDValue(string(out), "passwordauthentication") == "no" {
		r.warn(transport.FTPLoginWarnPasswordAuthOff)
	}
	if !ftpLoginJailMounted(login.Login) {
		notes = append(notes, "the jail is not mounted")
	}
	if withPassword && r.req.Password != "" && fileAccessCommandPresent("vsftpd") {
		if before, exists, err := fileAccessReadFile(fileaccess.VsftpdConfPath); err == nil {
			if state, _ := fileaccess.Classify(before, exists); state == fileaccess.FileManaged {
				if err := ftpLoginProbeFTP(ctx, login.Login, r.req.Password); err != nil {
					r.warn(transport.FTPLoginWarnFTPRejected)
					notes = append(notes, "FTP sign-in: "+err.Error())
				}
			}
		}
	}
	if len(notes) > 0 {
		r.step("verify", "failed", strings.Join(notes, "; "))
		return
	}
	r.step("verify", "done", "")
}

// ---------------------------------------------------------------------------
// Update: a new password and/or a new folder

func (r *ftpLoginRun) update() *ftpLoginRefusal {
	req := r.req
	if err := fileaccess.ValidateLoginName(req.Login); err != nil {
		return refuseFTPLogin(transport.FTPLoginRefusedInvalid, "%v", err)
	}
	if req.Password != "" {
		if err := fileaccess.ValidatePassword(req.Password); err != nil {
			return refuseFTPLogin(transport.FTPLoginRefusedInvalid, "%v", err)
		}
	}
	entries, logins, before, exists, refusal := r.checkCommon()
	if refusal != nil {
		return refusal
	}
	current, has := fileaccess.FindLogin(logins, req.Login)
	if !has || current.Site != r.site.Username || current.DomainID != r.site.DomainID {
		return refuseFTPLogin(transport.FTPLoginRefusedNoLogin, "this site has no login %s", req.Login)
	}
	account, err := fileAccessLookupAccount(current.Login)
	if err != nil || account.UID != r.account.UID || account.Home != fileaccess.LoginHome(current.Login) {
		return refuseFTPLogin(transport.FTPLoginRefusedNoLogin,
			"the account of the login %s is missing or changed on the server; remove the login and add it again", current.Login)
	}
	next := current
	next.ReadOnly = req.ReadOnly
	if strings.TrimSpace(req.Folder) != "" {
		folder, err := fileaccess.CleanLoginFolder(req.Folder)
		if err != nil {
			return refuseFTPLogin(transport.FTPLoginRefusedInvalid, "%v", err)
		}
		next.Folder = folder
	}
	if next == current && req.Password == "" {
		r.step("check", "done", "nothing to change")
		return nil
	}
	unit, refusal := ftpLoginUnitWrite(next)
	if refusal != nil {
		return refusal
	}
	if next.Folder != current.Folder {
		folderPath := fileaccess.LoginFolderPath(next.SubscriptionID, next.DomainID, next.Folder)
		if err := ftpLoginCheckFolder(sandboxbind.FTPRequest{Login: next.Login, Site: next.Site, Folder: folderPath}); err != nil {
			return ftpLoginFolderRefusal(next.Folder, err)
		}
	}
	r.step("check", "done", "")
	if next.Folder != current.Folder {
		// Open sessions see the old folder until they end: end them.
		if err := ftpLoginKillSessions(next.Login); err != nil {
			r.step("jail", "failed", err.Error())
			return refuseFTPLogin(transport.FTPLoginRefusedJail, "end the open sessions of %s: %v; nothing was changed", next.Login, err)
		}
		if refusal := r.startJail(next, unit); refusal != nil {
			refusal.detail += r.rollback()
			return refusal
		}
	}
	if next != current {
		plan := &fileAccessPlan{req: transport.ApplyFileAccessRequest{Enabled: true}, entries: entries,
			logins: fileaccess.MergeLogin(logins, next, true)}
		if refusal := r.writeDropIn(plan, before, exists, next); refusal != nil {
			refusal.detail += r.rollback()
			return refusal
		}
		r.applyFTP(plan)
	}
	if req.Password != "" {
		if refusal := r.setPassword(next.Login); refusal != nil {
			refusal.detail += r.rollback()
			return refusal
		}
	}
	r.undo = nil
	r.verify(next, req.Password != "")
	return nil
}

// ---------------------------------------------------------------------------
// Remove

func (r *ftpLoginRun) remove() *ftpLoginRefusal {
	if err := fileaccess.ValidateLoginName(r.req.Login); err != nil {
		return refuseFTPLogin(transport.FTPLoginRefusedInvalid, "%v", err)
	}
	entries, logins, before, exists, refusal := r.checkCommon()
	if refusal != nil {
		return refusal
	}
	current, recorded := fileaccess.FindLogin(logins, r.req.Login)
	if recorded && (current.Site != r.site.Username || current.DomainID != r.site.DomainID) {
		return refuseFTPLogin(transport.FTPLoginRefusedNoLogin, "this site has no login %s", r.req.Login)
	}
	if !recorded {
		// An unfinished login (the Agent stopped half way): only an account
		// that is provably this site's login is cleaned up.
		account, err := fileAccessLookupAccount(r.req.Login)
		if errors.Is(err, errFileAccessNoAccount) {
			r.step("check", "done", "no such login: nothing to remove")
			return nil
		}
		if err != nil || account.UID != r.account.UID || account.Home != fileaccess.LoginHome(r.req.Login) {
			return refuseFTPLogin(transport.FTPLoginRefusedNoLogin, "this site has no login %s", r.req.Login)
		}
		current = fileaccess.Login{Login: r.req.Login, Site: r.site.Username, SubscriptionID: r.site.SubscriptionID, DomainID: r.site.DomainID, Folder: "/"}
	}
	if _, refusal := ftpLoginUnitWrite(current); refusal != nil && refusal.code == transport.FTPLoginRefusedUnitOwner {
		return refusal
	}
	r.step("check", "done", "")
	plan := &fileAccessPlan{req: transport.ApplyFileAccessRequest{Enabled: true}, entries: entries,
		logins: fileaccess.MergeLogin(logins, current, false)}
	return removeFTPLogin(r, plan, before, exists, current, recorded)
}

// removeFTPLogin closes and removes one login: lock, end its sessions, stop
// the jail, the sshd and FTP records, the account. Each step leaves the
// login closed, so a failure half way is safe and the removal can be
// repeated.
func removeFTPLogin(r *ftpLoginRun, plan *fileAccessPlan, before []byte, exists bool, login fileaccess.Login, recorded bool) *ftpLoginRefusal {
	ctx := r.ctx
	if _, err := fileAccessLookupAccount(login.Login); err == nil {
		if out, err := fileAccessRun(ctx, nil, "usermod", "-L", login.Login); err != nil {
			r.step("account", "failed", firstLine(string(out)))
			return refuseFTPLogin(transport.FTPLoginRefusedAccount, "lock %s: %s; nothing else was changed", login.Login, firstLine(string(out)))
		}
	}
	if err := ftpLoginKillSessions(login.Login); err != nil {
		r.step("jail", "failed", err.Error())
		return refuseFTPLogin(transport.FTPLoginRefusedJail, "end the open sessions of %s: %v; the login is locked", login.Login, err)
	}
	unitName := fileaccess.LoginUnit(login.Login)
	_, _ = fileAccessRun(ctx, nil, "systemctl", "disable", "--now", unitName)
	if ftpLoginJailMounted(login.Login) {
		r.step("jail", "failed", "the jail is still mounted")
		return refuseFTPLogin(transport.FTPLoginRefusedJail, "the jail of %s could not be removed (systemctl stop %s); the login is locked", login.Login, unitName)
	}
	if write, err := planFileRemove(fileaccess.LoginUnitPath(login.Login)); err == nil && write != nil {
		if err := writeFileAccessFile(write); err != nil {
			r.step("jail", "failed", err.Error())
			return refuseFTPLogin(transport.FTPLoginRefusedJail, "remove %s: %v; the login is locked", write.path, err)
		}
	}
	_, _ = fileAccessRun(ctx, nil, "systemctl", "daemon-reload")
	_, _ = fileAccessRun(ctx, nil, "systemctl", "reset-failed", unitName)
	r.step("jail", "done", "")
	if recorded {
		if refusal := r.writeDropIn(plan, before, exists, login); refusal != nil {
			refusal.detail += "; the login is locked and has no jail"
			return refusal
		}
		r.undo = nil
		r.applyFTP(plan)
	}
	if err := deleteFTPLoginAccount(ctx, login.Login, r.account.UID); err != nil {
		r.step("account", "failed", err.Error())
		return refuseFTPLogin(transport.FTPLoginRefusedAccount, "%v; the login is closed (no jail, no sshd or FTP entry)", err)
	}
	r.step("account", "done", "removed")
	return nil
}

// ---------------------------------------------------------------------------
// State

func readFTPLoginsState(ctx context.Context, site transport.FTPLoginSite) transport.FTPLoginsStateResponse {
	resp := transport.FTPLoginsStateResponse{Logins: []transport.FTPLoginState{}, SSHDropIn: transport.FileAccessFileAbsent, FTPConfig: transport.FileAccessFileAbsent}
	_, logins, state, _, _, err := readFileAccessEntries()
	if err != nil {
		resp.SSHDropIn = transport.FileAccessFileUnreadable
		resp.Error = err.Error()
	} else {
		resp.SSHDropIn = string(state)
	}
	resp.ServerLogins = len(logins)
	resp.SSHDPresent = fileAccessCommandPresent("sshd")
	if resp.SSHDPresent {
		if out, err := fileAccessRun(ctx, nil, "sshd", "-T", "-C", sshdConnectionSpec(site.Username)); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				var port int
				if _, err := fmt.Sscanf(strings.TrimSpace(line), "port %d", &port); err == nil && port > 0 {
					resp.SSHPorts = append(resp.SSHPorts, port)
				}
			}
		}
	}
	resp.FTPInstalled = fileAccessCommandPresent("vsftpd")
	if resp.FTPInstalled {
		before, exists, err := fileAccessReadFile(fileaccess.VsftpdConfPath)
		if err != nil {
			resp.FTPConfig = transport.FileAccessFileUnreadable
		} else {
			state, _ := fileaccess.Classify(before, exists)
			resp.FTPConfig = string(state)
		}
	}
	site.Username = strings.TrimSpace(site.Username)
	siteAccount, siteErr := fileAccessLookupAccount(site.Username)
	seen := map[string]bool{}
	for _, l := range logins {
		if l.Site != site.Username || l.DomainID != site.DomainID || l.SubscriptionID != site.SubscriptionID {
			continue
		}
		seen[l.Login] = true
		resp.Logins = append(resp.Logins, ftpLoginStateOf(ctx, l, siteAccount, siteErr == nil))
	}
	// Unfinished logins: an account with this site's uid and a jail home
	// that is not in the record (the Agent stopped half way).
	if siteErr == nil {
		if lines, err := readPasswdLines(); err == nil {
			for _, line := range lines {
				if line.uid != siteAccount.UID || line.name == site.Username || seen[line.name] ||
					line.home != fileaccess.LoginHome(line.name) {
					continue
				}
				resp.Logins = append(resp.Logins, transport.FTPLoginState{Login: line.name, Unfinished: true, UnitFile: transport.FileAccessFileAbsent,
					JailProblem: "the login was not finished; remove it and add it again"})
			}
		}
	}
	return resp
}

func ftpLoginStateOf(ctx context.Context, l fileaccess.Login, site fileAccessAccount, siteKnown bool) transport.FTPLoginState {
	out := transport.FTPLoginState{Login: l.Login, Folder: l.Folder, ReadOnly: l.ReadOnly, Password: string(fileaccess.PasswordNone), UnitFile: transport.FileAccessFileAbsent}
	if account, err := fileAccessLookupAccount(l.Login); err == nil {
		out.AccountOK = siteKnown && account.UID == site.UID && account.GID == site.GID &&
			account.Home == fileaccess.LoginHome(l.Login) && !fileaccess.UsableShell(account.Shell)
		out.Password = string(account.Password)
		if expiry, err := ftpLoginExpiry(l.Login); err == nil && expiry != "" {
			if day, err := strconv.ParseInt(expiry, 10, 64); err == nil && day <= ftpLoginToday() {
				out.Suspended = true
			}
		}
	}
	if before, exists, err := fileAccessReadFile(fileaccess.LoginUnitPath(l.Login)); err != nil {
		out.UnitFile = transport.FileAccessFileUnreadable
	} else {
		state, _ := fileaccess.Classify(before, exists)
		out.UnitFile = string(state)
	}
	out.JailActive = fileAccessUnitActive(ctx, fileaccess.LoginUnit(l.Login)) && ftpLoginJailMounted(l.Login)
	if !out.JailActive {
		out.JailProblem = ftpLoginJailProblem(ctx, l.Login)
	}
	return out
}

// ftpLoginExpiry is an account's expiry field ("" never).
func ftpLoginExpiry(username string) (string, error) {
	content, err := os.ReadFile(suspensionShadowPath)
	if err != nil {
		return "", err
	}
	expiry, found, err := shadowExpiryFor(content, username)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%s has no shadow entry", username)
	}
	return expiry, nil
}

// ---------------------------------------------------------------------------
// Site suspension and deletion

// syncFTPLoginsExpiry gives every login of a site account the site account's
// expiry: a suspended site's logins are suspended with it, a restored site's
// logins open again (D-025). The logins have no expiry of their own.
func syncFTPLoginsExpiry(ctx context.Context, siteUser, expiry string) error {
	site, err := fileAccessLookupAccount(siteUser)
	if errors.Is(err, errFileAccessNoAccount) {
		return nil
	}
	if err != nil {
		return err
	}
	lines, err := readPasswdLines()
	if err != nil {
		return err
	}
	var failed []string
	for _, line := range lines {
		if line.uid != site.UID || line.name == siteUser || line.home != fileaccess.LoginHome(line.name) {
			continue
		}
		current, err := ftpLoginExpiry(line.name)
		if err != nil {
			failed = append(failed, err.Error())
			continue
		}
		if current == expiry {
			continue
		}
		if out, err := fileAccessRun(ctx, nil, "chage", chageExpiryArgs(line.name, expiry)...); err != nil {
			failed = append(failed, line.name+": "+firstLine(string(out)))
			continue
		}
		if expiry != "" {
			// A suspended login's open sessions end now.
			_ = ftpLoginKillSessions(line.name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("the site's FTP logins could not follow it: %s", strings.Join(failed, "; "))
	}
	return nil
}

// revokeFTPLoginsOfSite takes the password of every login of a site account
// ("*": no password, which no unlock brings back) and ends their sessions:
// the subscription was transferred (docs/PLANS-AND-RIGHTS.md section 8).
// The caller holds fileAccessMu.
func revokeFTPLoginsOfSite(ctx context.Context, siteUser string) error {
	site, err := fileAccessLookupAccount(siteUser)
	if errors.Is(err, errFileAccessNoAccount) {
		return nil
	}
	if err != nil {
		return err
	}
	lines, err := readPasswdLines()
	if err != nil {
		return err
	}
	var logins []string
	for _, line := range lines {
		if line.uid == site.UID && line.name != siteUser && line.home == fileaccess.LoginHome(line.name) {
			logins = append(logins, line.name)
		}
	}
	if len(logins) == 0 {
		return nil
	}
	states, err := readShadowStates()
	if err != nil {
		return err
	}
	var failed []string
	for _, name := range logins {
		line := passwdLine{name: name}
		if states[line.name] != fileaccess.PasswordNone {
			if out, err := fileAccessRun(ctx, nil, "usermod", "-p", "*", line.name); err != nil {
				failed = append(failed, line.name+": "+firstLine(string(out)))
				continue
			}
		}
		if err := ftpLoginKillSessions(line.name); err != nil {
			failed = append(failed, err.Error())
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("the passwords of the site's FTP logins could not all be removed: %s", strings.Join(failed, "; "))
	}
	return nil
}

// forgetFTPLoginsOfSite removes every login of a site account before the
// account itself is deleted: logins share its uid and group, so they must
// not outlive it.
func forgetFTPLoginsOfSite(username string) error {
	if !fileaccess.ValidUsername(username) {
		return nil
	}
	fileAccessMu.Lock()
	defer fileAccessMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), fileAccessTimeout)
	defer cancel()
	site, err := fileAccessLookupAccount(username)
	if errors.Is(err, errFileAccessNoAccount) {
		return nil
	}
	if err != nil {
		return err
	}
	entries, logins, state, before, exists, err := readFileAccessEntries()
	if err != nil {
		return err
	}
	var names []string
	recorded := map[string]bool{}
	for _, l := range fileaccess.LoginsOfSite(logins, username) {
		names = append(names, l.Login)
		recorded[l.Login] = true
	}
	if lines, err := readPasswdLines(); err == nil {
		for _, line := range lines {
			if line.uid == site.UID && line.name != username && line.home == fileaccess.LoginHome(line.name) && !recorded[line.name] {
				names = append(names, line.name)
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	if recordedAny := len(recorded) > 0; recordedAny && state != fileaccess.FileManaged {
		return fmt.Errorf("%s was changed by hand; the site's FTP logins are kept", fileaccess.SSHDropInPath)
	}
	var failed []string
	for _, name := range names {
		login, isRecorded := fileaccess.FindLogin(logins, name)
		if !isRecorded {
			login = fileaccess.Login{Login: name, Site: username, Folder: "/"}
		}
		run := &ftpLoginRun{ctx: ctx, account: site, req: transport.ApplyFTPLoginRequest{Action: transport.FTPLoginRemove, Login: name}}
		plan := &fileAccessPlan{req: transport.ApplyFileAccessRequest{Enabled: true}, entries: entries,
			logins: fileaccess.MergeLogin(logins, login, false)}
		if refusal := removeFTPLogin(run, plan, before, exists, login, isRecorded); refusal != nil {
			failed = append(failed, name+": "+refusal.detail)
			continue
		}
		logins = plan.logins
		if isRecorded {
			// The next removal starts from the drop-in just written.
			if b, e, err := fileAccessReadFile(fileaccess.SSHDropInPath); err == nil {
				before, exists = b, e
			}
		}
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}

// ---------------------------------------------------------------------------
// FTP sign-in probe

// probeFTPLogin signs in over FTP with TLS as the login and checks that it
// starts in its jail's /files. The password goes only to vsftpd on
// 127.0.0.1, inside TLS.
func probeFTPLogin(ctx context.Context, login, password string) error {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:21")
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	reader := bufio.NewReader(conn)
	if line, err := readFTPReply(reader); err != nil || !strings.HasPrefix(line, "220") {
		return fmt.Errorf("no FTP greeting (%v)", err)
	}
	if _, err := conn.Write([]byte("AUTH TLS\r\n")); err != nil {
		return err
	}
	if line, err := readFTPReply(reader); err != nil || !strings.HasPrefix(line, "234") {
		return fmt.Errorf("AUTH TLS was not accepted (%v)", err)
	}
	client := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) //nolint:gosec // local handshake proof only
	if err := client.HandshakeContext(ctx); err != nil {
		return fmt.Errorf("TLS handshake: %w", err)
	}
	tlsReader := bufio.NewReader(client)
	send := func(command string) (string, error) {
		if _, err := client.Write([]byte(command + "\r\n")); err != nil {
			return "", err
		}
		return readFTPReply(tlsReader)
	}
	if line, err := send("USER " + login); err != nil || !strings.HasPrefix(line, "331") {
		return fmt.Errorf("USER was not accepted (%s)", strings.TrimSpace(line))
	}
	if line, err := send("PASS " + password); err != nil || !strings.HasPrefix(line, "230") {
		return fmt.Errorf("the sign-in was refused (%s)", strings.TrimSpace(line))
	}
	line, err := send("PWD")
	_, _ = send("QUIT")
	if err != nil || !strings.HasPrefix(line, "257") || !strings.Contains(line, `"`+fileaccess.LoginStartDir+`"`) {
		return fmt.Errorf("the login does not start in %s (%s)", fileaccess.LoginStartDir, strings.TrimSpace(line))
	}
	return nil
}
