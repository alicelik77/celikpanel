package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/transport"
)

// fileAccessFakeHost replaces every host seam of file_access.go: files in a
// map, accounts in a map, and every command recorded in order.
type fileAccessFakeHost struct {
	mu       sync.Mutex
	files    map[string]string
	accounts map[string]fileAccessAccount
	commands []string
	// sshdEffective is what `sshd -T` prints; empty means "as rendered".
	sshdEffective string
	// sshdAuth comes first in every computed `sshd -T` answer (its values
	// win): the server's own password settings (alpha.128).
	sshdAuth      string
	sshdTest      error
	probe         error
	present       map[string]bool
	jailCalls     int
	// missing are the jail tools the host lacks (alpha.118).
	missing []string
}

func installFileAccessFakeHost(t *testing.T) *fileAccessFakeHost {
	t.Helper()
	host := &fileAccessFakeHost{
		files: map[string]string{},
		accounts: map[string]fileAccessAccount{
			"example_com": {Name: "example_com", UID: 1001, GID: 1001, Group: "example_com", Home: fileaccess.SiteHome(3, 5),
				Shell: "/bin/bash", Password: fileaccess.PasswordSet},
			"shop_test": {Name: "shop_test", UID: 1002, GID: 1002, Group: "shop_test", Home: fileaccess.SiteHome(3, 6),
				Shell: "/bin/bash", Password: fileaccess.PasswordDisabled},
		},
		present: map[string]bool{"sshd": true, "vsftpd": true, "fail2ban-client": true, "usermod": true},
	}
	saved := []any{fileAccessReadFile, fileAccessWriteFile, fileAccessRemoveFile, fileAccessRun, fileAccessLookupAccount,
		fileAccessListAccounts, fileAccessStatChain, fileAccessEnsureJail, fileAccessFamily, fileAccessCommandPresent,
		fileAccessUnitActive, fileAccessProbeFTP, fileAccessCertificate, fileAccessEnsureDir}
	t.Cleanup(func() {
		fileAccessReadFile = saved[0].(func(string) ([]byte, bool, error))
		fileAccessWriteFile = saved[1].(func(string, []byte, os.FileMode) error)
		fileAccessRemoveFile = saved[2].(func(string) error)
		fileAccessRun = saved[3].(func(context.Context, []byte, string, ...string) ([]byte, error))
		fileAccessLookupAccount = saved[4].(func(string) (fileAccessAccount, error))
		fileAccessListAccounts = saved[5].(func() ([]fileAccessAccount, error))
		fileAccessStatChain = saved[6].(func([]string) ([]fileaccess.DirInfo, error))
		fileAccessEnsureJail = saved[7].(func(string, []fileaccess.JailUser) ([]string, bool, error))
		fileAccessFamily = saved[8].(func() string)
		fileAccessCommandPresent = saved[9].(func(string) bool)
		fileAccessUnitActive = saved[10].(func(context.Context, string) bool)
		fileAccessProbeFTP = saved[11].(func(context.Context) error)
		fileAccessCertificate = saved[12].(func(bool) (string, string, string))
		fileAccessEnsureDir = saved[13].(func(string, os.FileMode) error)
	})
	fileAccessReadFile = func(path string) ([]byte, bool, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		content, ok := host.files[path]
		return []byte(content), ok, nil
	}
	fileAccessWriteFile = func(path string, content []byte, _ os.FileMode) error {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.files[path] = string(content)
		host.commands = append(host.commands, "write "+path)
		return nil
	}
	fileAccessRemoveFile = func(path string) error {
		host.mu.Lock()
		defer host.mu.Unlock()
		delete(host.files, path)
		host.commands = append(host.commands, "remove "+path)
		return nil
	}
	fileAccessRun = func(_ context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		line := strings.TrimSpace(name + " " + strings.Join(args, " "))
		host.commands = append(host.commands, line)
		switch {
		case name == "sshd" && len(args) == 1 && args[0] == "-t":
			return nil, host.sshdTest
		case name == "sshd" && len(args) > 0 && args[0] == "-T":
			if host.sshdEffective != "" {
				return []byte(host.sshdEffective), nil
			}
			return []byte(host.sshdAuth + effectiveFromFakeDropIn(host.files[fileaccess.SSHDropInPath], strings.TrimPrefix(strings.Split(args[2], ",")[0], "user="))), nil
		case name == "usermod" && len(args) == 3 && args[0] == "-s":
			account := host.accounts[args[2]]
			account.Shell = args[1]
			host.accounts[args[2]] = account
		case name == "usermod" && len(args) == 4 && args[0] == "-a":
			account := host.accounts[args[3]]
			account.InSiteGroup = true
			host.accounts[args[3]] = account
		case name == "chpasswd":
			if strings.Contains(line, "secret") {
				return nil, errors.New("the password reached the command line")
			}
			user, _, _ := strings.Cut(string(stdin), ":")
			account := host.accounts[user]
			account.Password = fileaccess.PasswordSet
			host.accounts[user] = account
		}
		return nil, nil
	}
	fileAccessLookupAccount = func(name string) (fileAccessAccount, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		account, ok := host.accounts[name]
		if !ok {
			return fileAccessAccount{}, errFileAccessNoAccount
		}
		return account, nil
	}
	fileAccessListAccounts = func() ([]fileAccessAccount, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		var out []fileAccessAccount
		for _, account := range host.accounts {
			out = append(out, account)
		}
		return out, nil
	}
	fileAccessStatChain = func(paths []string) ([]fileaccess.DirInfo, error) {
		var chain []fileaccess.DirInfo
		for _, p := range paths {
			chain = append(chain, fileaccess.DirInfo{Path: p, IsDir: true, Mode: 0o751})
		}
		return chain, nil
	}
	fileAccessEnsureJail = func(string, []fileaccess.JailUser) ([]string, bool, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.jailCalls++
		host.commands = append(host.commands, "jail")
		return nil, false, nil
	}
	fileAccessFamily = func() string { return "apt" }
	savedMissing := fileAccessJailMissing
	t.Cleanup(func() { fileAccessJailMissing = savedMissing })
	fileAccessJailMissing = func() []string {
		host.mu.Lock()
		defer host.mu.Unlock()
		return append([]string(nil), host.missing...)
	}
	fileAccessCommandPresent = func(name string) bool { return host.present[name] }
	fileAccessUnitActive = func(context.Context, string) bool { return true }
	fileAccessProbeFTP = func(context.Context) error { return host.probe }
	fileAccessCertificate = func(bool) (string, string, string) {
		return transport.FileAccessCertMailHost, fileaccess.MailHostCert, fileaccess.MailHostKey
	}
	fileAccessEnsureDir = func(string, os.FileMode) error { return nil }
	savedLog := fileAccessEnsureLogFile
	t.Cleanup(func() { fileAccessEnsureLogFile = savedLog })
	fileAccessEnsureLogFile = func() {}
	return host
}

// effectiveFromFakeDropIn prints what sshd would apply to one account from
// the rendered drop-in, enough for CheckSSHDEffective.
func effectiveFromFakeDropIn(dropIn, user string) string {
	_, body := fileaccess.Classify([]byte(dropIn), dropIn != "")
	entries, logins, _ := fileaccess.ParseSSHDropIn(body)
	if login, ok := fileaccess.FindLogin(logins, user); ok {
		var lines []string
		for key, value := range fileaccess.ExpectedLoginSSHDEffective(login) {
			lines = append(lines, key+" "+value)
		}
		return "passwordauthentication yes\nport 22\n" + strings.Join(lines, "\n") + "\n"
	}
	entry, ok := fileaccess.FindEntry(entries, user)
	if !ok {
		return "passwordauthentication yes\ndenygroups " + fileaccess.SiteGroup + "\n"
	}
	var lines []string
	for key, value := range fileaccess.ExpectedSSHDEffective(entry, true) {
		lines = append(lines, key+" "+value)
	}
	return "passwordauthentication yes\nport 22\n" + strings.Join(lines, "\n") + "\n"
}

func (h *fileAccessFakeHost) index(prefix string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, command := range h.commands {
		if strings.HasPrefix(command, prefix) {
			return i
		}
	}
	return -1
}

func fileAccessApply(t *testing.T, request transport.ApplyFileAccessRequest) transport.ApplyFileAccessResponse {
	t.Helper()
	var response transport.ApplyFileAccessResponse
	if err := (&Agent{}).ApplyFileAccessV1(&request, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

var exampleSite = transport.FileAccessSite{SubscriptionID: 3, DomainID: 5, Username: "example_com"}

func TestFileAccessEnableConfinesBeforeTheAccountCanSignIn(t *testing.T) {
	host := installFileAccessFakeHost(t)
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{
		Site: exampleSite, Enabled: true, Password: "a long secret",
		Reconcile: []transport.FileAccessSite{{SubscriptionID: 3, DomainID: 6, Username: "shop_test"}},
	})
	if !response.Applied || response.Refusal != "" {
		t.Fatalf("response %+v", response)
	}
	_, body := fileaccess.Classify([]byte(host.files[fileaccess.SSHDropInPath]), true)
	if !strings.Contains(body, "Match User example_com\n\tChrootDirectory /var/www/celikpanel/subscriptions/3/sites") {
		t.Fatalf("drop-in:\n%s", host.files[fileaccess.SSHDropInPath])
	}
	// The confinement is in place and verified before the password is set.
	if write, verify, password := host.index("write "+fileaccess.SSHDropInPath), host.index("sshd -T"), host.index("chpasswd"); write < 0 || verify < write || password < verify {
		t.Fatalf("order write=%d verify=%d password=%d: %v", write, verify, password, host.commands)
	}
	if host.accounts["example_com"].Shell != fileaccess.NoLoginShell || !host.accounts["example_com"].InSiteGroup {
		t.Fatalf("account %+v", host.accounts["example_com"])
	}
	// The subscription's other account loses its shell and joins the group.
	if host.accounts["shop_test"].Shell != fileaccess.NoLoginShell || !host.accounts["shop_test"].InSiteGroup {
		t.Fatalf("reconciled %+v", host.accounts["shop_test"])
	}
	if !strings.Contains(host.files[fileaccess.VsftpdUserList], "\nexample_com\n") {
		t.Fatalf("FTP list:\n%s", host.files[fileaccess.VsftpdUserList])
	}
	if !strings.Contains(host.files[fileaccess.VsftpdConfPath], "force_local_logins_ssl=YES") || host.index("systemctl restart vsftpd.service") < 0 {
		t.Fatalf("vsftpd not configured: %v", host.commands)
	}
	if host.jailCalls != 0 {
		t.Fatal("a jail was built without the ssh right")
	}
}

func TestFileAccessSSHBuildsTheJailBeforeTheShell(t *testing.T) {
	host := installFileAccessFakeHost(t)
	// A site account made since alpha.117 has no login shell until SSH is
	// granted, so the grant has to set one.
	account := host.accounts["example_com"]
	account.Shell = fileaccess.NoLoginShell
	host.accounts["example_com"] = account
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true, SSH: true})
	if !response.Applied {
		t.Fatalf("response %+v", response)
	}
	if jail, dropIn, shell := host.index("jail"), host.index("write "+fileaccess.SSHDropInPath), host.index("usermod -s /bin/bash"); jail < 0 || dropIn < jail || shell < dropIn {
		t.Fatalf("order jail=%d drop-in=%d shell=%d: %v", jail, dropIn, shell, host.commands)
	}
}

func TestFileAccessDropInNotAppliedIsPutBack(t *testing.T) {
	host := installFileAccessFakeHost(t)
	previous := host.files[fileaccess.SSHDropInPath]
	host.sshdEffective = "port 22\nchrootdirectory none\n"
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true, Password: "a long secret"})
	if response.Applied || response.Refusal != transport.FileAccessRefusedSSHDInclude {
		t.Fatalf("response %+v", response)
	}
	if host.files[fileaccess.SSHDropInPath] != previous {
		t.Fatal("the drop-in was not put back")
	}
	if host.index("chpasswd") >= 0 {
		t.Fatal("the password was set although sshd does not confine the account")
	}
}

func TestFileAccessOwnerDropInIsKept(t *testing.T) {
	host := installFileAccessFakeHost(t)
	host.files[fileaccess.SSHDropInPath] = "Match User example_com\n\tChrootDirectory /srv\n"
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true})
	if response.Refusal != transport.FileAccessRefusedSSHDOwner {
		t.Fatalf("response %+v", response)
	}
	if host.index("write") >= 0 || host.index("usermod") >= 0 {
		t.Fatalf("something changed: %v", host.commands)
	}
}

func TestFileAccessDisableLocksBeforeTheConfinementGoes(t *testing.T) {
	host := installFileAccessFakeHost(t)
	if response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true}); !response.Applied {
		t.Fatalf("enable %+v", response)
	}
	host.commands = nil
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: false})
	if !response.Applied {
		t.Fatalf("disable %+v", response)
	}
	if lock, dropIn := host.index("usermod -L"), host.index("write "+fileaccess.SSHDropInPath); lock < 0 || dropIn < lock {
		t.Fatalf("order lock=%d drop-in=%d: %v", lock, dropIn, host.commands)
	}
	_, body := fileaccess.Classify([]byte(host.files[fileaccess.SSHDropInPath]), true)
	if strings.Contains(body, "Match User example_com") || !strings.Contains(body, "DenyGroups "+fileaccess.SiteGroup) {
		t.Fatalf("drop-in:\n%s", body)
	}
}

func TestFileAccessOwnerVsftpdConfigIsAWarning(t *testing.T) {
	host := installFileAccessFakeHost(t)
	host.files[fileaccess.VsftpdConfPath] = "listen=YES\nanonymous_enable=YES\n"
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true})
	if !response.Applied || !strings.Contains(strings.Join(response.Warnings, ","), transport.FileAccessWarnFTPOwnerConfig) {
		t.Fatalf("response %+v", response)
	}
	if host.files[fileaccess.VsftpdConfPath] != "listen=YES\nanonymous_enable=YES\n" || host.index("write "+fileaccess.VsftpdUserList) >= 0 {
		t.Fatalf("the owner's vsftpd was changed: %v", host.commands)
	}
}

func TestFileAccessFTPThatDoesNotAnswerIsPutBack(t *testing.T) {
	host := installFileAccessFakeHost(t)
	host.probe = errors.New("no greeting")
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true})
	if !response.Applied || !strings.Contains(strings.Join(response.Warnings, ","), transport.FileAccessWarnFTPRejected) {
		t.Fatalf("response %+v", response)
	}
	if _, ok := host.files[fileaccess.VsftpdConfPath]; ok {
		t.Fatal("the FTP configuration stayed although vsftpd did not answer")
	}
}

func TestFileAccessForeignAccountIsRefused(t *testing.T) {
	host := installFileAccessFakeHost(t)
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{
		Site: transport.FileAccessSite{SubscriptionID: 4, DomainID: 5, Username: "example_com"}, Enabled: true,
	})
	if response.Refusal != transport.FileAccessRefusedAccountMismatch || host.index("write") >= 0 {
		t.Fatalf("response %+v commands %v", response, host.commands)
	}
}

// After a transfer: the password is replaced by "no password" (a later
// unlock cannot bring the old one back), every key goes, access closes.
func TestFileAccessRevokeCredentialsRemovesPasswordAndKeys(t *testing.T) {
	host := installFileAccessFakeHost(t)
	key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOx0Cx3yZ6eXn0Qd3B1sJ8yQnQ6m3u5W9k4bH2pY7d1F old-owner"
	if response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true, SSH: true, Password: "a long secret", AddKeys: []string{key}}); !response.Applied {
		t.Fatalf("enable %+v", response)
	}
	keysPath := fileaccess.KeysDir + "/example_com"
	if _, ok := host.files[keysPath]; !ok {
		t.Fatal("the key was not written")
	}
	// Opening while revoking is refused before anything changes.
	host.commands = nil
	refused := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true, RevokeCredentials: true})
	if refused.Refusal != transport.FileAccessRefusedInvalid || host.index("usermod") >= 0 || host.index("write") >= 0 {
		t.Fatalf("revoke while enabling: %+v %v", refused, host.commands)
	}
	host.commands = nil
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: false, RevokeCredentials: true})
	if !response.Applied {
		t.Fatalf("revoke %+v", response)
	}
	if host.index("usermod -p * example_com") < 0 || host.index("usermod -L") >= 0 {
		t.Fatalf("password not replaced: %v", host.commands)
	}
	if _, ok := host.files[keysPath]; ok || host.index("remove "+keysPath) < 0 {
		t.Fatalf("keys stayed: %v", host.commands)
	}
	_, body := fileaccess.Classify([]byte(host.files[fileaccess.SSHDropInPath]), true)
	if strings.Contains(body, "Match User example_com") {
		t.Fatalf("access stayed open:\n%s", body)
	}
	// A locked password is replaced too.
	account := host.accounts["example_com"]
	account.Password = fileaccess.PasswordLocked
	host.accounts["example_com"] = account
	host.commands = nil
	if again := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: false, RevokeCredentials: true}); !again.Applied || host.index("usermod -p * example_com") < 0 {
		t.Fatalf("locked password kept: %+v %v", again, host.commands)
	}
}

func TestFileAccessKeysDirLetsTheAccountReachItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys")
	if err := os.Mkdir(keys, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := prepareFileAccessKeysDir(keys); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(keys)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o711 {
		t.Fatalf("keys directory mode %o, want 711 so sshd can read the file as the account", got)
	}
}
