package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// File access per hosting account (alpha.117; D-022, D-024, D-025).
//
// The account is the site's own system user: files uploaded over FTP or SFTP
// get the owner the site's PHP-FPM pool and Node unit run as, so nothing has
// to be re-owned. The account sees its subscription's sites folder as "/"
// (root-owned 0751, so it can enter only its own numbered folder):
//
//   - SFTP: an sshd drop-in Match block per account with ChrootDirectory and
//     ForceCommand internal-sftp. Keys come only from a root-owned file per
//     account. Every other hosting account (group celikpanel-site) is denied
//     SSH sign-in, tunnels included.
//   - SSH (with the ssh right): the same chroot without ForceCommand, and a
//     small jail (bash and basic file tools copied in, no setuid, no /proc).
//   - FTP: vsftpd with TLS required for login and data, the same chroot,
//     only listed accounts, its own PAM service.
//
// The drop-in's own entry lines are the record of who has access; there is
// no other copy. Every file carries a sealed header; a file edited by the
// owner is reported and kept. A change is verified (sshd -t, sshd -T for the
// account, an FTP TLS handshake) and the previous file is put back when the
// check fails.
//
// Site başına dosya erişimi. Hesap sitenin kendi kullanıcısıdır; kök,
// aboneliğin sites klasörüdür. Kayıt, sshd ekinin kendi satırlarıdır.

var fileAccessMu sync.Mutex

const fileAccessTimeout = 3 * time.Minute

// Seams: production values, replaced by component tests.
var (
	fileAccessReadFile = func(path string) ([]byte, bool, error) {
		content, err := secureReadConfig(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, false, nil
			}
			return nil, false, err
		}
		return content, true, nil
	}
	fileAccessWriteFile = func(path string, content []byte, mode os.FileMode) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if filepath.Dir(path) == fileaccess.KeysDir {
			if err := prepareFileAccessKeysDir(fileaccess.KeysDir); err != nil {
				return err
			}
		}
		return secureWriteConfig(path, content, mode)
	}
	fileAccessRemoveFile = func(path string) error {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	fileAccessRun            = runFileAccessCommand
	fileAccessLookupAccount  = lookupFileAccessAccount
	fileAccessListAccounts   = listFileAccessSiteAccounts
	fileAccessStatChain      = statFileAccessChain
	fileAccessEnsureJail     = ensureFileAccessJail
	fileAccessFamily         = detectPkgFamily
	fileAccessCommandPresent = func(name string) bool {
		_, err := fileAccessCommandPath(name)
		return err == nil
	}
	fileAccessUnitActive = func(ctx context.Context, unit string) bool {
		out, err := fileAccessRun(ctx, nil, "systemctl", "is-active", unit)
		return err == nil && strings.TrimSpace(string(out)) == "active"
	}
	fileAccessProbeFTP    = probeFileAccessFTP
	fileAccessCertificate = chooseFileAccessCertificate
	fileAccessEnsureDir   = func(path string, mode os.FileMode) error { return os.MkdirAll(path, mode) }
	// fileAccessEnsureLogFile makes vsftpd's log, which the Fail2ban jail
	// reads, before either service starts (root-only, 0600).
	fileAccessEnsureLogFile = func() {
		if _, err := os.Stat(fileaccess.VsftpdLogPath); errors.Is(err, os.ErrNotExist) {
			if file, err := os.OpenFile(fileaccess.VsftpdLogPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600); err == nil {
				_ = file.Close()
			}
		}
	}
	fileAccessSleep = time.Sleep
)

// fileAccessCommands are the only programs this feature runs, each from a
// trusted root-owned path.
var fileAccessCommands = map[string][]string{
	"sshd":            trustedSSHDExecutablePaths,
	"usermod":         {"/usr/sbin/usermod", "/usr/bin/usermod"},
	"groupadd":        {"/usr/sbin/groupadd", "/usr/bin/groupadd"},
	"chpasswd":        {"/usr/sbin/chpasswd", "/usr/bin/chpasswd"},
	"systemctl":       {"/usr/bin/systemctl", "/bin/systemctl"},
	"fail2ban-client": {"/usr/bin/fail2ban-client"},
	"vsftpd":          {"/usr/sbin/vsftpd", "/usr/bin/vsftpd"},
}

func fileAccessCommandPath(name string) (string, error) {
	candidates, ok := fileAccessCommands[name]
	if !ok {
		return "", fmt.Errorf("command %q is not allowed", name)
	}
	return firstTrustedExecutable(candidates, name)
}

func runFileAccessCommand(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
	path, err := fileAccessCommandPath(name)
	if err != nil {
		return nil, err
	}
	cmd := serviceMutationCommand(ctx, path, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}

// fileAccessAccount is one local account as /etc/passwd, /etc/group and
// /etc/shadow have it. The password hash is never kept, only its state.
type fileAccessAccount struct {
	Name        string
	UID, GID    int
	Group       string
	Home        string
	Shell       string
	Password    fileaccess.PasswordState
	InSiteGroup bool
}

var errFileAccessNoAccount = errors.New("no such account")

// sshUnits are the OpenSSH server units, Debian/Ubuntu first.
var fileAccessSSHUnits = []string{"ssh.service", "sshd.service"}

// ---------------------------------------------------------------------------
// RPCs

// FileAccessStateV1 reads one account and the server side (read only).
func (a *Agent) FileAccessStateV1(req *transport.FileAccessStateRequest, resp *transport.FileAccessStateResponse) error {
	if req == nil || resp == nil {
		return errors.New("file access state request and response are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	*resp = readFileAccessState(ctx, req.Site)
	return nil
}

// FileAccessPreviewV1 says what an apply would change (read only).
func (a *Agent) FileAccessPreviewV1(req *transport.FileAccessPreviewRequest, resp *transport.ApplyFileAccessResponse) error {
	if req == nil || resp == nil {
		return errors.New("file access preview request and response are required")
	}
	*resp = transport.ApplyFileAccessResponse{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	plan, refusal := buildFileAccessPlan(ctx, req.Change)
	if refusal != nil {
		resp.Refusal, resp.Detail = refusal.code, refusal.detail
	} else {
		resp.Changes = plan.changes
		resp.Warnings = plan.warnings
	}
	resp.State = readFileAccessState(ctx, req.Change.Site)
	return nil
}

// ApplyFileAccessV1 brings one account to the requested setting.
func (a *Agent) ApplyFileAccessV1(req *transport.ApplyFileAccessRequest, resp *transport.ApplyFileAccessResponse) error {
	if req == nil || resp == nil {
		return errors.New("file access request and response are required")
	}
	*resp = transport.ApplyFileAccessResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing file access"); err != nil {
		return err
	}
	fileAccessMu.Lock()
	defer fileAccessMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), fileAccessTimeout)
	defer cancel()
	plan, refusal := buildFileAccessPlan(ctx, *req)
	if refusal == nil {
		refusal = executeFileAccessPlan(ctx, plan)
	}
	if refusal != nil {
		resp.Refusal, resp.Detail = refusal.code, refusal.detail
	} else {
		resp.Applied = true
	}
	if plan != nil {
		resp.Changes = plan.changes
		resp.Warnings = plan.warnings
	}
	resp.State = readFileAccessState(ctx, req.Site)
	return nil
}

// ---------------------------------------------------------------------------
// Plan

type fileAccessRefusal struct {
	code   string
	detail string
}

func refuseFileAccess(code, format string, args ...any) *fileAccessRefusal {
	return &fileAccessRefusal{code: code, detail: fmt.Sprintf(format, args...)}
}

type fileAccessFileWrite struct {
	path    string
	content string
	mode    os.FileMode
	before  []byte
	existed bool
	remove  bool
}

type fileAccessAccountChange struct {
	name  string
	shell string // empty: unchanged
	// restrict runs before the drop-in: "lock" or "clear" (an empty
	// password becomes no password). grant runs after sshd proved the
	// confinement: "set", "unlock" or "star" (keys only).
	restrict string
	grant    string
	addGroup bool
}

type fileAccessPlan struct {
	req        transport.ApplyFileAccessRequest
	entry      fileaccess.Entry
	entries    []fileaccess.Entry
	dropIn     *fileAccessFileWrite
	keys       *fileAccessFileWrite
	accounts   []fileAccessAccountChange
	jail       *fileAccessJailPlan
	ftp        []*fileAccessFileWrite
	ftpRestart bool
	ftpManaged bool
	ftpFailed  bool
	fail2ban   *fileAccessFileWrite
	changes    []transport.FileAccessChange
	warnings   []string
}

type fileAccessJailPlan struct {
	chroot string
	users  []fileaccess.JailUser
}

func (p *fileAccessPlan) note(kind, target, action, detail string) {
	p.changes = append(p.changes, transport.FileAccessChange{Kind: kind, Target: target, Action: action, Detail: detail})
}

func (p *fileAccessPlan) warn(code string) {
	for _, existing := range p.warnings {
		if existing == code {
			return
		}
	}
	p.warnings = append(p.warnings, code)
}

// planFileWrite records a managed file's next content and what it is now.
// An owner's file is never part of a plan: the caller decides what that
// means (a refusal or a warning).
func planFileWrite(path, content string, mode os.FileMode) (*fileAccessFileWrite, fileaccess.ManagedState, error) {
	before, exists, err := fileAccessReadFile(path)
	if err != nil {
		return nil, "", err
	}
	state, _ := fileaccess.Classify(before, exists)
	if state == fileaccess.FileOwner {
		return nil, state, nil
	}
	if exists && string(before) == content {
		return nil, state, nil
	}
	return &fileAccessFileWrite{path: path, content: content, mode: mode, before: before, existed: exists}, state, nil
}

func planFileRemove(path string) (*fileAccessFileWrite, error) {
	before, exists, err := fileAccessReadFile(path)
	if err != nil || !exists {
		return nil, err
	}
	if state, _ := fileaccess.Classify(before, exists); state != fileaccess.FileManaged {
		return nil, nil
	}
	return &fileAccessFileWrite{path: path, before: before, existed: true, remove: true}, nil
}

func fileWriteAction(write *fileAccessFileWrite) string {
	switch {
	case write.remove:
		return "remove"
	case write.existed:
		return "update"
	default:
		return "create"
	}
}

// verifiedFileAccessAccount proves an account is the hosting account of the
// named site: it exists, is not a system account and its home is the site's.
func verifiedFileAccessAccount(site transport.FileAccessSite) (fileAccessAccount, *fileAccessRefusal) {
	entry := fileaccess.Entry{Username: site.Username, SubscriptionID: site.SubscriptionID, DomainID: site.DomainID}
	if err := entry.Validate(); err != nil {
		return fileAccessAccount{}, refuseFileAccess(transport.FileAccessRefusedInvalid, "%v", err)
	}
	account, err := fileAccessLookupAccount(site.Username)
	if errors.Is(err, errFileAccessNoAccount) {
		return fileAccessAccount{}, refuseFileAccess(transport.FileAccessRefusedNoAccount,
			"the site's system account %s does not exist on this server", site.Username)
	}
	if err != nil {
		return fileAccessAccount{}, refuseFileAccess(transport.FileAccessRefusedNoAccount, "read the account %s: %v", site.Username, err)
	}
	if account.UID < 1000 || filepath.Clean(account.Home) != fileaccess.SiteHome(site.SubscriptionID, site.DomainID) {
		return fileAccessAccount{}, refuseFileAccess(transport.FileAccessRefusedAccountMismatch,
			"the account %s is not this site's hosting account (its home is %s)", site.Username, account.Home)
	}
	return account, nil
}

func readFileAccessEntries() ([]fileaccess.Entry, fileaccess.ManagedState, []byte, bool, error) {
	before, exists, err := fileAccessReadFile(fileaccess.SSHDropInPath)
	if err != nil {
		return nil, "", nil, false, err
	}
	state, body := fileaccess.Classify(before, exists)
	if state != fileaccess.FileManaged {
		return nil, state, before, exists, nil
	}
	entries, err := fileaccess.ParseSSHDropInEntries(body)
	return entries, state, before, exists, err
}

func buildFileAccessPlan(ctx context.Context, req transport.ApplyFileAccessRequest) (*fileAccessPlan, *fileAccessRefusal) {
	plan := &fileAccessPlan{req: req}
	site := req.Site
	account, refusal := verifiedFileAccessAccount(site)
	if refusal != nil {
		return plan, refusal
	}
	ssh := req.Enabled && req.SSH
	plan.entry = fileaccess.Entry{Username: site.Username, SubscriptionID: site.SubscriptionID, DomainID: site.DomainID, SSH: ssh}
	// Revoking credentials closes access; it never sets a password or a key.
	if req.RevokeCredentials && (req.Enabled || req.Password != "" || len(req.AddKeys) > 0) {
		return plan, refuseFileAccess(transport.FileAccessRefusedInvalid, "revoking credentials closes access; it sets no password or key")
	}
	if req.Password != "" {
		if err := fileaccess.ValidatePassword(req.Password); err != nil {
			return plan, refuseFileAccess(transport.FileAccessRefusedInvalid, "%v", err)
		}
	}
	var added []fileaccess.Key
	for _, text := range req.AddKeys {
		key, err := fileaccess.ParsePublicKey(text)
		if err != nil {
			return plan, refuseFileAccess(transport.FileAccessRefusedInvalid, "%v", err)
		}
		added = append(added, key)
	}

	sshdPresent := fileAccessCommandPresent("sshd")
	if req.Enabled && !sshdPresent {
		return plan, refuseFileAccess(transport.FileAccessRefusedNoSSHD,
			"this server has no OpenSSH server, which SFTP needs; install it with the operating system's package manager (openssh-server, or openssh on Arch)")
	}

	// The drop-in and its record of every account with access.
	entries, state, before, exists, err := readFileAccessEntries()
	switch {
	case err != nil:
		return plan, refuseFileAccess(transport.FileAccessRefusedSSHDOwner, "read %s: %v", fileaccess.SSHDropInPath, err)
	case state == fileaccess.FileOwner:
		return plan, refuseFileAccess(transport.FileAccessRefusedSSHDOwner,
			"%s was changed by hand; CelikPanel keeps it and changes nothing. Remove it or put CelikPanel's version back to manage access here", fileaccess.SSHDropInPath)
	}
	// Accounts that are gone (a deleted site) leave the record.
	var kept []fileaccess.Entry
	for _, e := range entries {
		other, lookupErr := fileAccessLookupAccount(e.Username)
		if e.Username != site.Username && (lookupErr != nil || filepath.Clean(other.Home) != fileaccess.SiteHome(e.SubscriptionID, e.DomainID)) {
			plan.note("account", e.Username, "remove", "the account no longer exists; its access is removed")
			continue
		}
		kept = append(kept, e)
	}
	plan.entries = fileaccess.MergeEntry(kept, plan.entry, req.Enabled)
	if req.Enabled {
		chroot := fileaccess.ChrootDir(site.SubscriptionID)
		chain, statErr := fileAccessStatChain(fileaccess.ChrootChain(chroot))
		if statErr != nil {
			return plan, refuseFileAccess(transport.FileAccessRefusedChrootUnsafe, "inspect %s: %v", chroot, statErr)
		}
		if dir, reason := fileaccess.UnsafeChrootComponent(chain); dir != "" {
			return plan, refuseFileAccess(transport.FileAccessRefusedChrootUnsafe,
				"%s is %s, so OpenSSH and vsftpd cannot close accounts into %s; make it root-owned and not group/other writable (chown root:root %s; chmod go-w %s)",
				dir, reason, chroot, dir, dir)
		}
	}
	if sshdPresent {
		text, renderErr := fileaccess.RenderSSHDropIn(plan.entries)
		if renderErr != nil {
			return plan, refuseFileAccess(transport.FileAccessRefusedInvalid, "%v", renderErr)
		}
		if !exists || string(before) != text {
			plan.dropIn = &fileAccessFileWrite{path: fileaccess.SSHDropInPath, content: text, mode: 0o644, before: before, existed: exists}
			plan.note("file", fileaccess.SSHDropInPath, fileWriteAction(plan.dropIn), "OpenSSH: SFTP and SSH confinement")
		}
	}

	// Keys: a root-owned file per account.
	keysPath := filepath.Join(fileaccess.KeysDir, site.Username)
	keysBefore, keysExist, err := fileAccessReadFile(keysPath)
	if err != nil {
		return plan, refuseFileAccess(transport.FileAccessRefusedKeysOwner, "read %s: %v", keysPath, err)
	}
	keysState, keysBody := fileaccess.Classify(keysBefore, keysExist)
	if keysState == fileaccess.FileOwner {
		return plan, refuseFileAccess(transport.FileAccessRefusedKeysOwner,
			"%s was changed by hand; CelikPanel keeps it. Remove it or put CelikPanel's version back to manage keys here", keysPath)
	}
	keys, err := fileaccess.ParseKeysFile(keysBody)
	if err != nil {
		return plan, refuseFileAccess(transport.FileAccessRefusedKeysOwner, "read %s: %v", keysPath, err)
	}
	keys = mergeFileAccessKeys(keys, added, req.RemoveKeys)
	if req.RevokeCredentials {
		keys = nil
	}
	if len(keys) > fileaccess.MaxKeys {
		return plan, refuseFileAccess(transport.FileAccessRefusedInvalid, "at most %d keys per account", fileaccess.MaxKeys)
	}
	if len(keys) == 0 {
		if keysExist {
			plan.keys = &fileAccessFileWrite{path: keysPath, before: keysBefore, existed: true, remove: true}
		}
	} else if text := fileaccess.RenderKeysFile(keys, ssh); !keysExist || string(keysBefore) != text {
		plan.keys = &fileAccessFileWrite{path: keysPath, content: text, mode: 0o644, before: keysBefore, existed: keysExist}
	}
	if plan.keys != nil {
		plan.note("file", keysPath, fileWriteAction(plan.keys), fmt.Sprintf("%d SSH key(s)", len(keys)))
	}

	// The account itself.
	change := fileAccessAccountChange{name: account.Name, addGroup: !account.InSiteGroup}
	if shell := fileaccess.ShellFor(req.Enabled, ssh); account.Shell != shell {
		change.shell = shell
	}
	switch {
	case req.RevokeCredentials && (account.Password == fileaccess.PasswordSet || account.Password == fileaccess.PasswordLocked):
		// A locked hash would come back with a later unlock: replace it.
		change.restrict = "revoke"
	case !req.Enabled && account.Password == fileaccess.PasswordSet:
		change.restrict = "lock"
	case account.Password == fileaccess.PasswordEmpty:
		change.restrict = "clear"
	}
	switch {
	case !req.Enabled:
	case req.Password != "":
		change.grant = "set"
	case account.Password == fileaccess.PasswordLocked:
		change.grant = "unlock"
	case account.Password == fileaccess.PasswordDisabled:
		// Keys must work whatever UsePAM says: "*" is "no password",
		// "!" is "locked account".
		change.grant = "star"
	}
	plan.addAccountChange(change)

	// The subscription's other accounts: no command line without the right.
	for _, other := range req.Reconcile {
		if other.Username == site.Username || other.SubscriptionID != site.SubscriptionID {
			continue
		}
		otherAccount, otherRefusal := verifiedFileAccessAccount(other)
		if otherRefusal != nil {
			plan.note("account", other.Username, "keep", otherRefusal.detail)
			continue
		}
		existing, has := fileaccess.FindEntry(plan.entries, other.Username)
		reconcile := fileAccessAccountChange{name: otherAccount.Name, addGroup: !otherAccount.InSiteGroup}
		if shell := fileaccess.ShellFor(has, has && existing.SSH); otherAccount.Shell != shell {
			reconcile.shell = shell
		}
		if otherAccount.Password == fileaccess.PasswordEmpty {
			reconcile.restrict = "clear"
		}
		plan.addAccountChange(reconcile)
	}

	// The jail, when an account of this subscription has the command line.
	var jailUsers []fileaccess.JailUser
	for _, e := range plan.entries {
		if !e.SSH || e.SubscriptionID != site.SubscriptionID {
			continue
		}
		jailAccount, lookupErr := fileAccessLookupAccount(e.Username)
		if lookupErr != nil {
			continue
		}
		jailUsers = append(jailUsers, fileaccess.JailUser{Name: e.Username, UID: jailAccount.UID, GID: jailAccount.GID, Group: jailAccount.Group, DomainID: e.DomainID})
	}
	if len(jailUsers) > 0 {
		plan.jail = &fileAccessJailPlan{chroot: fileaccess.ChrootDir(site.SubscriptionID), users: jailUsers}
		plan.note("jail", plan.jail.chroot, "update", "command-line tools for the SSH accounts of this subscription")
	}

	planFileAccessFTP(ctx, plan)
	planFileAccessFail2ban(plan)
	return plan, nil
}

func (p *fileAccessPlan) addAccountChange(change fileAccessAccountChange) {
	if change.addGroup {
		p.note("account", change.name, "set", "group "+fileaccess.SiteGroup)
	}
	if change.shell != "" {
		p.note("account", change.name, "set", "login shell "+change.shell)
	}
	for _, op := range []string{change.restrict, change.grant} {
		switch op {
		case "set":
			p.note("account", change.name, "set", "new password")
		case "lock":
			p.note("account", change.name, "set", "password locked")
		case "unlock":
			p.note("account", change.name, "set", "password unlocked")
		case "clear":
			p.note("account", change.name, "set", "empty password replaced by no password")
		case "revoke":
			p.note("account", change.name, "set", "password removed (a new one is set when access is opened again)")
		case "star":
			p.note("account", change.name, "set", "locked account opened for keys only")
		}
	}
	if change.addGroup || change.shell != "" || change.restrict != "" || change.grant != "" {
		p.accounts = append(p.accounts, change)
	}
}

func mergeFileAccessKeys(current, added []fileaccess.Key, removeFingerprints []string) []fileaccess.Key {
	remove := map[string]bool{}
	for _, fingerprint := range removeFingerprints {
		remove[strings.TrimSpace(fingerprint)] = true
	}
	seen := map[string]bool{}
	var out []fileaccess.Key
	for _, key := range append(append([]fileaccess.Key(nil), current...), added...) {
		if remove[key.Fingerprint] || seen[key.Fingerprint] {
			continue
		}
		seen[key.Fingerprint] = true
		out = append(out, key)
	}
	return out
}

// planFileAccessFTP manages vsftpd only when its configuration is
// CelikPanel's (written when CelikPanel installed it) or absent. An owner's
// vsftpd.conf stays as it is; FTP is then reported, SFTP still works.
func planFileAccessFTP(ctx context.Context, plan *fileAccessPlan) {
	if !fileAccessCommandPresent("vsftpd") {
		plan.warn(transport.FileAccessWarnFTPNotInstalled)
		return
	}
	conf, confState, err := renderFileAccessVsftpd(ctx)
	if err != nil {
		plan.warn(transport.FileAccessWarnFTPRejected)
		plan.note("service", "vsftpd", "keep", err.Error())
		return
	}
	if confState == fileaccess.FileOwner {
		plan.warn(transport.FileAccessWarnFTPOwnerConfig)
		return
	}
	plan.ftpManaged = true
	if cert, _, _ := fileAccessCertificate(false); cert == transport.FileAccessCertSelfSigned {
		plan.warn(transport.FileAccessWarnFTPSelfSigned)
	}
	add := func(write *fileAccessFileWrite, restart bool, what string) {
		if write == nil {
			return
		}
		plan.ftp = append(plan.ftp, write)
		plan.ftpRestart = plan.ftpRestart || restart
		plan.note("file", write.path, fileWriteAction(write), what)
	}
	if write, _, err := planFileWrite(fileaccess.VsftpdConfPath, conf, 0o600); err == nil {
		add(write, true, "vsftpd: FTP over TLS")
	}
	if write, state, err := planFileWrite(fileaccess.VsftpdPAMPath, fileaccess.RenderVsftpdPAM(fileAccessFamily()), 0o644); err == nil {
		if state == fileaccess.FileOwner {
			plan.warn(transport.FileAccessWarnFTPOwnerConfig)
		}
		add(write, true, "PAM: FTP sign-in")
	}
	list, err := fileaccess.RenderVsftpdUserList(plan.entries)
	if err == nil {
		if write, _, err := planFileWrite(fileaccess.VsftpdUserList, list, 0o600); err == nil {
			add(write, false, "FTP accounts")
		}
	}
	wanted := map[string]bool{}
	for _, e := range plan.entries {
		path := filepath.Join(fileaccess.VsftpdUserConfDir, e.Username)
		wanted[path] = true
		text, renderErr := fileaccess.RenderVsftpdUserConf(e)
		if renderErr != nil {
			continue
		}
		if write, _, err := planFileWrite(path, text, 0o600); err == nil {
			add(write, false, "FTP folder of "+e.Username)
		}
	}
	if names, err := os.ReadDir(fileaccess.VsftpdUserConfDir); err == nil {
		for _, name := range names {
			path := filepath.Join(fileaccess.VsftpdUserConfDir, name.Name())
			if wanted[path] {
				continue
			}
			if write, err := planFileRemove(path); err == nil {
				add(write, false, "FTP folder of a removed account")
			}
		}
	}
}

func renderFileAccessVsftpd(ctx context.Context) (string, fileaccess.ManagedState, error) {
	before, exists, err := fileAccessReadFile(fileaccess.VsftpdConfPath)
	if err != nil {
		return "", "", err
	}
	state, _ := fileaccess.Classify(before, exists)
	_, certFile, keyFile := fileAccessCertificate(false)
	if certFile == "" {
		// No certificate yet: the self-signed one is made at apply time.
		certFile, keyFile = fileaccess.SelfSignedCert, fileaccess.SelfSignedKey
	}
	conf, err := fileaccess.RenderVsftpdConf(fileaccess.VsftpdParams{
		CertFile: certFile, KeyFile: keyFile, SecureChrootDir: fileaccess.SecureChrootDirFor(fileAccessFamily()),
	})
	return conf, state, err
}

func planFileAccessFail2ban(plan *fileAccessPlan) {
	if !fileAccessCommandPresent("fail2ban-client") {
		plan.warn(transport.FileAccessWarnFail2banMissing)
		return
	}
	if !plan.ftpManaged {
		return
	}
	write, state, err := planFileWrite(fileaccess.Fail2banJailPath, fileaccess.RenderFail2banJail(), 0o644)
	if err != nil || state == fileaccess.FileOwner {
		plan.warn(transport.FileAccessWarnFail2banOwner)
		return
	}
	if write != nil {
		plan.fail2ban = write
		plan.note("file", write.path, fileWriteAction(write), "Fail2ban: ban repeated FTP sign-in failures")
	}
}

// ---------------------------------------------------------------------------
// Apply

func writeFileAccessFile(write *fileAccessFileWrite) error {
	if write.remove {
		return fileAccessRemoveFile(write.path)
	}
	return fileAccessWriteFile(write.path, []byte(write.content), write.mode)
}

func restoreFileAccessFile(write *fileAccessFileWrite) error {
	if write == nil {
		return nil
	}
	if !write.existed {
		return fileAccessRemoveFile(write.path)
	}
	return fileAccessWriteFile(write.path, write.before, write.mode|0o600)
}

func executeFileAccessPlan(ctx context.Context, plan *fileAccessPlan) *fileAccessRefusal {
	if needGroup(plan.accounts) {
		if out, err := fileAccessRun(ctx, nil, "groupadd", "-r", "-f", fileaccess.SiteGroup); err != nil {
			return refuseFileAccess(transport.FileAccessRefusedAccountChange, "create group %s: %s", fileaccess.SiteGroup, firstLine(string(out)))
		}
	}
	// What takes a way in away (group, no-login shell, lock) comes before
	// the drop-in; what opens one (password, command line) only after sshd
	// has proved the confinement. An account is never open outside its jail.
	if refusal := applyFileAccessAccounts(ctx, plan, false); refusal != nil {
		return refusal
	}
	if plan.jail != nil {
		missing, noDevices, err := fileAccessEnsureJail(plan.jail.chroot, plan.jail.users)
		if err != nil {
			return refuseFileAccess(transport.FileAccessRefusedJail, "prepare the command-line jail in %s: %v", plan.jail.chroot, err)
		}
		if len(missing) > 0 {
			plan.warn(transport.FileAccessWarnJailMissing)
			plan.note("jail", plan.jail.chroot, "keep", "not on this server: "+strings.Join(missing, ", "))
		}
		if noDevices {
			plan.warn(transport.FileAccessWarnJailNoDevices)
		}
	}
	if plan.keys != nil {
		if err := writeFileAccessFile(plan.keys); err != nil {
			return refuseFileAccess(transport.FileAccessRefusedKeysOwner, "write %s: %v", plan.keys.path, err)
		}
	}
	if plan.dropIn != nil {
		if refusal := applyFileAccessDropIn(ctx, plan); refusal != nil {
			if restoreErr := restoreFileAccessFile(plan.keys); restoreErr != nil {
				log.Printf("file access: put the keys file back: %v", restoreErr)
			}
			return refusal
		}
	}
	if refusal := applyFileAccessAccounts(ctx, plan, true); refusal != nil {
		return refusal
	}
	if plan.req.Enabled {
		if out, err := fileAccessRun(ctx, nil, "sshd", "-T", "-C", sshdConnectionSpec(plan.entry.Username)); err == nil &&
			effectiveSSHDValue(string(out), "passwordauthentication") == "no" {
			plan.warn(transport.FileAccessWarnPasswordAuthOff)
		}
	}
	applyFileAccessFTP(ctx, plan)
	applyFileAccessFail2ban(ctx, plan)
	return nil
}

func needGroup(changes []fileAccessAccountChange) bool {
	for _, change := range changes {
		if change.addGroup {
			return true
		}
	}
	return false
}

// applyFileAccessAccounts runs the restricting changes (grant=false) or the
// granting ones (grant=true) of every account in the plan.
func applyFileAccessAccounts(ctx context.Context, plan *fileAccessPlan, grant bool) *fileAccessRefusal {
	for _, change := range plan.accounts {
		if change.addGroup && !grant {
			if out, err := fileAccessRun(ctx, nil, "usermod", "-a", "-G", fileaccess.SiteGroup, change.name); err != nil {
				return refuseFileAccess(transport.FileAccessRefusedAccountChange, "add %s to %s: %s", change.name, fileaccess.SiteGroup, firstLine(string(out)))
			}
		}
		if change.shell != "" && (change.shell != fileaccess.NoLoginShell) == grant {
			if out, err := fileAccessRun(ctx, nil, "usermod", "-s", change.shell, change.name); err != nil {
				return refuseFileAccess(transport.FileAccessRefusedAccountChange, "set the login shell of %s: %s", change.name, firstLine(string(out)))
			}
		}
		var out []byte
		var err error
		op := change.restrict
		if grant {
			op = change.grant
		}
		switch op {
		case "set":
			// chpasswd hashes with the system's own method (yescrypt or
			// SHA-512). The password reaches it on stdin only.
			out, err = fileAccessRun(ctx, []byte(change.name+":"+plan.req.Password+"\n"), "chpasswd")
		case "unlock":
			out, err = fileAccessRun(ctx, nil, "usermod", "-U", change.name)
		case "lock":
			out, err = fileAccessRun(ctx, nil, "usermod", "-L", change.name)
		case "star", "clear", "revoke":
			out, err = fileAccessRun(ctx, nil, "usermod", "-p", "*", change.name)
		}
		if err != nil {
			// chpasswd output never carries the password; keep one line.
			return refuseFileAccess(transport.FileAccessRefusedAccountChange, "change the password state of %s: %s", change.name, firstLine(string(out)))
		}
	}
	return nil
}

func sshdConnectionSpec(username string) string {
	return "user=" + username + ",host=localhost,addr=127.0.0.1"
}

func effectiveSSHDValue(output, key string) string {
	for _, line := range strings.Split(output, "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && strings.EqualFold(name, key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// applyFileAccessDropIn writes the drop-in, proves sshd accepts it and
// applies it to the account, and reloads sshd. Any failed proof puts the
// previous file back.
func applyFileAccessDropIn(ctx context.Context, plan *fileAccessPlan) *fileAccessRefusal {
	write := plan.dropIn
	if err := writeFileAccessFile(write); err != nil {
		return refuseFileAccess(transport.FileAccessRefusedSSHDRejected, "write %s: %v", write.path, err)
	}
	restore := func(code, format string, args ...any) *fileAccessRefusal {
		refusal := refuseFileAccess(code, format, args...)
		if err := restoreFileAccessFile(write); err != nil {
			refusal.detail += fmt.Sprintf("; putting the previous %s back failed: %v (check it with sshd -t)", write.path, err)
		} else if out, err := fileAccessRun(ctx, nil, "sshd", "-t"); err != nil {
			refusal.detail += "; the previous file is back but sshd -t still reports: " + firstLine(string(out))
		} else {
			refusal.detail += "; the previous file is back and sshd accepts it"
		}
		return refusal
	}
	if out, err := fileAccessRun(ctx, nil, "sshd", "-t"); err != nil {
		return restore(transport.FileAccessRefusedSSHDRejected, "sshd refused the configuration: %s", firstLine(string(out)))
	}
	out, err := fileAccessRun(ctx, nil, "sshd", "-T", "-C", sshdConnectionSpec(plan.entry.Username))
	if err != nil {
		return restore(transport.FileAccessRefusedSSHDRejected, "sshd could not show its effective configuration: %s", firstLine(string(out)))
	}
	if err := fileaccess.CheckSSHDEffective(string(out), fileaccess.ExpectedSSHDEffective(plan.entry, plan.req.Enabled)); err != nil {
		return restore(transport.FileAccessRefusedSSHDInclude,
			"%v. The server's /etc/ssh/sshd_config must read the drop-in folder: add the line \"Include /etc/ssh/sshd_config.d/*.conf\" at its top", err)
	}
	for _, unit := range fileAccessSSHUnits {
		if !fileAccessUnitActive(ctx, unit) {
			continue
		}
		if out, err := fileAccessRun(ctx, nil, "systemctl", "reload", unit); err != nil {
			return restore(transport.FileAccessRefusedSSHDRejected, "reload %s: %s", unit, firstLine(string(out)))
		}
		plan.note("service", unit, "reload", "")
	}
	return nil
}

func applyFileAccessFTP(ctx context.Context, plan *fileAccessPlan) {
	if !plan.ftpManaged || len(plan.ftp) == 0 {
		return
	}
	if _, certFile, _ := fileAccessCertificate(true); certFile == "" {
		plan.ftpFailed = true
		plan.warn(transport.FileAccessWarnFTPRejected)
		plan.note("service", "vsftpd", "keep", "no certificate for FTP over TLS could be prepared")
		return
	}
	_ = fileAccessEnsureDir(fileaccess.SecureChrootDirFor(fileAccessFamily()), 0o755)
	_ = fileAccessEnsureDir(fileaccess.VsftpdUserConfDir, 0o755)
	fileAccessEnsureLogFile()
	var written []*fileAccessFileWrite
	rollback := func(reason string) {
		plan.ftpFailed = true
		for i := len(written) - 1; i >= 0; i-- {
			if err := restoreFileAccessFile(written[i]); err != nil {
				log.Printf("file access: put %s back: %v", written[i].path, err)
			}
		}
		if plan.ftpRestart {
			_, _ = fileAccessRun(ctx, nil, "systemctl", "try-restart", "vsftpd.service")
		}
		plan.warn(transport.FileAccessWarnFTPRejected)
		plan.note("service", "vsftpd", "keep", reason+"; the previous FTP configuration is back")
	}
	for _, write := range plan.ftp {
		if err := writeFileAccessFile(write); err != nil {
			rollback(fmt.Sprintf("write %s: %v", write.path, err))
			return
		}
		written = append(written, write)
	}
	if !plan.ftpRestart {
		return
	}
	if out, err := fileAccessRun(ctx, nil, "systemctl", "enable", "vsftpd.service"); err != nil {
		log.Printf("file access: enable vsftpd: %s", firstLine(string(out)))
	}
	if out, err := fileAccessRun(ctx, nil, "systemctl", "restart", "vsftpd.service"); err != nil {
		rollback("vsftpd did not start with the new configuration: " + firstLine(string(out)))
		return
	}
	if err := fileAccessProbeFTP(ctx); err != nil {
		rollback("vsftpd did not answer FTP over TLS: " + err.Error())
		return
	}
	plan.note("service", "vsftpd.service", "restart", "answers FTP over TLS")
}

func applyFileAccessFail2ban(ctx context.Context, plan *fileAccessPlan) {
	// A jail for an FTP service that was just put back would watch nothing.
	if plan.fail2ban == nil || plan.ftpFailed {
		return
	}
	if err := writeFileAccessFile(plan.fail2ban); err != nil {
		plan.warn(transport.FileAccessWarnFail2banFailed)
		return
	}
	if out, err := fileAccessRun(ctx, nil, "fail2ban-client", "-t"); err != nil {
		_ = restoreFileAccessFile(plan.fail2ban)
		plan.warn(transport.FileAccessWarnFail2banFailed)
		plan.note("service", "fail2ban", "keep", "fail2ban refused the jail: "+firstLine(string(out)))
		return
	}
	if fileAccessUnitActive(ctx, "fail2ban.service") {
		if out, err := fileAccessRun(ctx, nil, "fail2ban-client", "reload"); err != nil {
			_ = restoreFileAccessFile(plan.fail2ban)
			_, _ = fileAccessRun(ctx, nil, "fail2ban-client", "reload")
			plan.warn(transport.FileAccessWarnFail2banFailed)
			plan.note("service", "fail2ban", "keep", "fail2ban did not reload: "+firstLine(string(out)))
			return
		}
		plan.note("service", "fail2ban.service", "reload", "")
	}
}

// probeFileAccessFTP proves vsftpd answers and upgrades the connection to
// TLS: 220 greeting, AUTH TLS -> 234, then a TLS handshake.
func probeFileAccessFTP(ctx context.Context) error {
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			fileAccessSleep(time.Second)
		}
		if last = probeFileAccessFTPOnce(ctx); last == nil {
			return nil
		}
	}
	return last
}

func probeFileAccessFTPOnce(ctx context.Context) error {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:21")
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(conn)
	if line, err := readFTPReply(reader); err != nil || !strings.HasPrefix(line, "220") {
		return fmt.Errorf("no FTP greeting (%q, %v)", line, err)
	}
	if _, err := conn.Write([]byte("AUTH TLS\r\n")); err != nil {
		return err
	}
	if line, err := readFTPReply(reader); err != nil || !strings.HasPrefix(line, "234") {
		return fmt.Errorf("AUTH TLS was not accepted (%q, %v)", line, err)
	}
	// Only the handshake is proved here; the certificate's name is checked
	// by the client, which knows the host name it connects to.
	client := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) //nolint:gosec
	if err := client.HandshakeContext(ctx); err != nil {
		return fmt.Errorf("TLS handshake: %w", err)
	}
	_, _ = client.Write([]byte("QUIT\r\n"))
	return nil
}

func readFTPReply(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return line, err
	}
	// A multi-line reply ("220-...") ends with "220 ...".
	if len(line) >= 4 && line[3] == '-' {
		code := line[:3]
		for {
			next, err := reader.ReadString('\n')
			if err != nil {
				return next, err
			}
			if strings.HasPrefix(next, code+" ") {
				return next, nil
			}
		}
	}
	return line, nil
}

// ---------------------------------------------------------------------------
// State

func readFileAccessState(ctx context.Context, site transport.FileAccessSite) transport.FileAccessStateResponse {
	var resp transport.FileAccessStateResponse
	account := transport.FileAccessAccount{
		Username:  site.Username,
		Password:  string(fileaccess.PasswordNone),
		KeysFile:  transport.FileAccessFileAbsent,
		Keys:      []transport.FileAccessKey{},
		ChrootDir: fileaccess.ChrootDir(site.SubscriptionID),
		StartDir:  fileaccess.StartDir(site.DomainID),
	}
	entries, dropInState, _, _, entriesErr := readFileAccessEntries()
	if host, err := fileAccessLookupAccount(site.Username); err == nil {
		account.Exists = true
		account.HomeMatches = filepath.Clean(host.Home) == fileaccess.SiteHome(site.SubscriptionID, site.DomainID)
		account.ShellUsable = fileaccess.UsableShell(host.Shell)
		account.Password = string(host.Password)
		account.InSiteGroup = host.InSiteGroup
	} else if !errors.Is(err, errFileAccessNoAccount) {
		resp.Error = err.Error()
	}
	if entry, ok := fileaccess.FindEntry(entries, site.Username); ok && entry.SubscriptionID == site.SubscriptionID && entry.DomainID == site.DomainID {
		account.Enabled = true
		account.SSH = entry.SSH
	}
	keysBefore, keysExist, err := fileAccessReadFile(filepath.Join(fileaccess.KeysDir, site.Username))
	if err != nil {
		account.KeysFile = transport.FileAccessFileUnreadable
	} else {
		state, body := fileaccess.Classify(keysBefore, keysExist)
		account.KeysFile = string(state)
		if keys, err := fileaccess.ParseKeysFile(body); err == nil {
			for _, key := range keys {
				account.Keys = append(account.Keys, transport.FileAccessKey{Type: key.Type, Fingerprint: key.Fingerprint, Comment: key.Comment})
			}
		}
	}
	resp.Account = account

	server := transport.FileAccessServer{
		SSHDropIn:              string(dropInState),
		PasswordAuthentication: "unknown",
		FTPConfig:              transport.FileAccessFileAbsent,
		FTPCertificate:         transport.FileAccessCertNone,
		FTPPassiveMin:          fileaccess.PassiveMin,
		FTPPassiveMax:          fileaccess.PassiveMax,
	}
	if entriesErr != nil {
		server.SSHDropIn = transport.FileAccessFileUnreadable
	}
	server.SSHDPresent = fileAccessCommandPresent("sshd")
	if server.SSHDPresent {
		if out, err := fileAccessRun(ctx, nil, "sshd", "-T", "-C", sshdConnectionSpec(site.Username)); err == nil {
			if value := effectiveSSHDValue(string(out), "passwordauthentication"); value != "" {
				server.PasswordAuthentication = value
			}
			for _, line := range strings.Split(string(out), "\n") {
				var port int
				if _, err := fmt.Sscanf(strings.TrimSpace(line), "port %d", &port); err == nil && port > 0 {
					server.SSHPorts = append(server.SSHPorts, port)
				}
			}
		}
	}
	server.FTPInstalled = fileAccessCommandPresent("vsftpd")
	if server.FTPInstalled {
		before, exists, err := fileAccessReadFile(fileaccess.VsftpdConfPath)
		if err != nil {
			server.FTPConfig = transport.FileAccessFileUnreadable
		} else {
			state, _ := fileaccess.Classify(before, exists)
			server.FTPConfig = string(state)
		}
		server.FTPActive = fileAccessUnitActive(ctx, "vsftpd.service")
		if server.FTPConfig == transport.FileAccessFileManaged {
			kind, certFile, _ := fileAccessCertificate(false)
			server.FTPCertificate = kind
			if certFile != "" {
				server.FTPCertificateName, server.FTPCertificateSHA256 = describeFileAccessCertificate(certFile)
			}
		}
	}
	server.Fail2banInstalled = fileAccessCommandPresent("fail2ban-client")
	if server.Fail2banInstalled && fileAccessUnitActive(ctx, "fail2ban.service") {
		_, err := fileAccessRun(ctx, nil, "fail2ban-client", "status", "sshd")
		server.Fail2banSSHD = err == nil
		_, err = fileAccessRun(ctx, nil, "fail2ban-client", "status", "vsftpd")
		server.Fail2banFTP = err == nil
	}
	if accounts, err := fileAccessListAccounts(); err == nil {
		sshAccounts := map[string]bool{}
		for _, e := range entries {
			if e.SSH {
				sshAccounts[e.Username] = true
			}
		}
		for _, a := range accounts {
			server.SiteUsersTotal++
			if fileaccess.UsableShell(a.Shell) && !sshAccounts[a.Name] {
				server.SiteUsersWithShell++
			}
			if !a.InSiteGroup {
				server.SiteUsersOutsideGroup++
			}
		}
	}
	resp.Server = server
	return resp
}

// isFileAccessSiteHome: an account belongs to hosting when its home is under
// the subscriptions root.
func isFileAccessSiteHome(home string) bool {
	return hostingpath.IsSiteHome(filepath.Clean(home))
}

// ---------------------------------------------------------------------------
// Site deletion and vsftpd installation

// forgetFileAccessAccount removes a deleted site's account from the access
// record and its key and FTP files. Best effort: the next change of any
// account removes a stale entry too.
func forgetFileAccessAccount(username string) {
	if !fileaccess.ValidUsername(username) {
		return
	}
	fileAccessMu.Lock()
	defer fileAccessMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, path := range []string{filepath.Join(fileaccess.KeysDir, username), filepath.Join(fileaccess.VsftpdUserConfDir, username)} {
		if write, err := planFileRemove(path); err == nil && write != nil {
			if err := writeFileAccessFile(write); err != nil {
				log.Printf("file access: remove %s: %v", path, err)
			}
		}
	}
	entries, state, before, exists, err := readFileAccessEntries()
	if err != nil || state != fileaccess.FileManaged {
		return
	}
	if _, ok := fileaccess.FindEntry(entries, username); !ok {
		return
	}
	remaining := fileaccess.MergeEntry(entries, fileaccess.Entry{Username: username}, false)
	text, err := fileaccess.RenderSSHDropIn(remaining)
	if err != nil {
		return
	}
	write := &fileAccessFileWrite{path: fileaccess.SSHDropInPath, content: text, mode: 0o644, before: before, existed: exists}
	if err := writeFileAccessFile(write); err != nil {
		log.Printf("file access: remove %s from %s: %v", username, write.path, err)
		return
	}
	if out, err := fileAccessRun(ctx, nil, "sshd", "-t"); err != nil {
		log.Printf("file access: sshd refused the drop-in without %s (%s); the previous file is put back", username, firstLine(string(out)))
		_ = restoreFileAccessFile(write)
		return
	}
	if list, err := fileaccess.RenderVsftpdUserList(remaining); err == nil {
		if listWrite, _, err := planFileWrite(fileaccess.VsftpdUserList, list, 0o600); err == nil && listWrite != nil {
			_ = writeFileAccessFile(listWrite)
		}
	}
	for _, unit := range fileAccessSSHUnits {
		if fileAccessUnitActive(ctx, unit) {
			_, _ = fileAccessRun(ctx, nil, "systemctl", "reload", unit)
		}
	}
}

// prepareInstalledVsftpd adopts vsftpd right after CelikPanel installed it:
// the package's own configuration (plain FTP, every local account) is
// replaced by CelikPanel's (TLS only, nobody listed yet) before the
// firewall opens port 21, and a running daemon is restarted with it. The
// distro file is kept next to it. held: the package start guard keeps the
// unit masked and stopped, so there is nothing to restart.
func prepareInstalledVsftpd(ctx context.Context, freshInstall, held bool) error {
	before, exists, err := fileAccessReadFile(fileaccess.VsftpdConfPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", fileaccess.VsftpdConfPath, err)
	}
	state, _ := fileaccess.Classify(before, exists)
	if state == fileaccess.FileOwner && !freshInstall {
		// vsftpd was there before CelikPanel installed anything: the
		// owner's configuration is kept; the Access tab says so.
		return nil
	}
	if state == fileaccess.FileOwner {
		backup := fileaccess.VsftpdConfPath + ".distro"
		if err := fileAccessWriteFile(backup, before, 0o600); err != nil {
			return fmt.Errorf("keep the distro's vsftpd.conf as %s: %w", backup, err)
		}
	}
	if _, certFile, _ := fileAccessCertificate(true); certFile == "" {
		return errors.New("no certificate for FTP over TLS could be prepared")
	}
	conf, _, err := renderFileAccessVsftpd(ctx)
	if err != nil {
		return err
	}
	entries, _, _, _, _ := readFileAccessEntries()
	list, err := fileaccess.RenderVsftpdUserList(entries)
	if err != nil {
		return err
	}
	_ = fileAccessEnsureDir(fileaccess.SecureChrootDirFor(fileAccessFamily()), 0o755)
	_ = fileAccessEnsureDir(fileaccess.VsftpdUserConfDir, 0o755)
	for _, write := range []struct {
		path, content string
		mode          os.FileMode
	}{
		{fileaccess.VsftpdUserList, list, 0o600},
		{fileaccess.VsftpdPAMPath, fileaccess.RenderVsftpdPAM(fileAccessFamily()), 0o644},
		{fileaccess.VsftpdConfPath, conf, 0o600},
	} {
		if err := fileAccessWriteFile(write.path, []byte(write.content), write.mode); err != nil {
			return fmt.Errorf("write %s: %w", write.path, err)
		}
	}
	for _, e := range entries {
		if text, err := fileaccess.RenderVsftpdUserConf(e); err == nil {
			_ = fileAccessWriteFile(filepath.Join(fileaccess.VsftpdUserConfDir, e.Username), []byte(text), 0o600)
		}
	}
	fileAccessEnsureLogFile()
	if held {
		return nil
	}
	// A vsftpd started before the guard existed (or by the owner) is
	// restarted with CelikPanel's configuration. Arch's package does not
	// start it.
	if out, err := fileAccessRun(ctx, nil, "systemctl", "try-restart", "vsftpd.service"); err != nil {
		return fmt.Errorf("restart vsftpd with the TLS-only configuration: %s", firstLine(string(out)))
	}
	return nil
}

// sortedFileAccessNames is a small helper for stable logs and tests.
func sortedFileAccessNames(accounts []fileAccessAccount) []string {
	names := make([]string, 0, len(accounts))
	for _, a := range accounts {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	return names
}

// prepareFileAccessKeysDir makes the keys directory root-owned and 0711.
// sshd reads AuthorizedKeysFile as the account itself, so the account must be
// able to pass through the directory; 0711 lets it open its own file by name
// without listing the others. The Agent runs with group celikpanel and umask
// 027, which made the directory 0750 root:celikpanel and key login
// impossible (lab run 1 of alpha.117).
func prepareFileAccessKeysDir(dir string) error {
	if err := os.Chmod(dir, 0o711); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		return os.Chown(dir, 0, 0)
	}
	return nil
}
