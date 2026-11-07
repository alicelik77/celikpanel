package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/sandboxbind"
	"github.com/alicelik/celikpanel/internal/transport"
)

// installFTPLoginSeams puts the login's own host seams on top of the file
// access fake host: the folder checks pass, the helper is in place, the jail
// mounts, and the local group file has no group of the login's name.
func installFTPLoginSeams(t *testing.T, host *fileAccessFakeHost) {
	t.Helper()
	savedSite, savedFolder, savedHelper, savedMounted, savedProbe, savedKill, savedGroup :=
		ftpLoginCheckSiteFolder, ftpLoginCheckFolder, ftpLoginEnsureHelper, ftpLoginJailMounted, ftpLoginProbeFTP, ftpLoginKillSessions, fileAccessGroupPath
	t.Cleanup(func() {
		ftpLoginCheckSiteFolder, ftpLoginCheckFolder, ftpLoginEnsureHelper, ftpLoginJailMounted, ftpLoginProbeFTP, ftpLoginKillSessions, fileAccessGroupPath =
			savedSite, savedFolder, savedHelper, savedMounted, savedProbe, savedKill, savedGroup
	})
	ftpLoginCheckSiteFolder = func(string, string) error { return nil }
	ftpLoginCheckFolder = func(sandboxbind.FTPRequest) error { return nil }
	ftpLoginEnsureHelper = func() error { return nil }
	ftpLoginJailMounted = func(string) bool { return true }
	ftpLoginProbeFTP = func(context.Context, string, string) error { return nil }
	ftpLoginKillSessions = func(login string) error {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.commands = append(host.commands, "kill-sessions "+login)
		return nil
	}
	fileAccessGroupPath = filepath.Join(t.TempDir(), "group")
	if err := os.WriteFile(fileAccessGroupPath, []byte("example_com:x:1001:\ncelikpanel-site:x:990:example_com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func ftpLoginApply(t *testing.T, request transport.ApplyFTPLoginRequest) transport.ApplyFTPLoginResponse {
	t.Helper()
	var response transport.ApplyFTPLoginResponse
	if err := (&Agent{}).ApplyFTPLoginV1(&request, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func annaLogin() fileaccess.Login {
	return fileaccess.Login{Login: "anna-up", Site: "example_com", SubscriptionID: 3, DomainID: 5, Folder: "/public_html/uploads"}
}

func TestFTPLoginAddClosesTheLoginBeforeItGetsAPassword(t *testing.T) {
	host := installFileAccessFakeHost(t)
	installFTPLoginSeams(t, host)
	response := ftpLoginApply(t, transport.ApplyFTPLoginRequest{Site: exampleSite, Action: transport.FTPLoginAdd,
		Login: "anna-up", Folder: "public_html/uploads", Password: "a long secret"})
	if !response.Applied {
		t.Fatalf("add: %+v", response)
	}
	useradd, unit, dropIn, check, password := host.index("useradd -o -u 1001 -g 1001 -G celikpanel-site -M -d /run/celikpanel-ftp/anna-up/./files -s /usr/sbin/nologin"),
		host.index("systemctl restart celikpanel-ftp-anna-up.service"), host.index("write "+fileaccess.SSHDropInPath),
		host.index("sshd -T -C user=anna-up"), host.index("chpasswd")
	if useradd < 0 || unit < useradd || dropIn < unit || check < dropIn || password < check {
		t.Fatalf("order: useradd %d, unit %d, drop-in %d, sshd -T %d, password %d\n%s", useradd, unit, dropIn, check, password, strings.Join(host.commands, "\n"))
	}
	_, body := fileaccess.Classify([]byte(host.files[fileaccess.SSHDropInPath]), true)
	_, logins, err := fileaccess.ParseSSHDropIn(body)
	if err != nil || len(logins) != 1 || logins[0] != annaLogin() {
		t.Fatalf("record %+v %v", logins, err)
	}
	unitText := host.files[fileaccess.LoginUnitPath("anna-up")]
	if !strings.Contains(unitText, "ftp-bind --login=anna-up --site=example_com --folder=/var/www/celikpanel/subscriptions/3/sites/5/public_html/uploads") {
		t.Fatalf("unit:\n%s", unitText)
	}
	if !strings.Contains(host.files[fileaccess.VsftpdUserList], "\nanna-up\n") || !strings.Contains(host.files[fileaccess.VsftpdConfPath], "passwd_chroot_enable=YES") {
		t.Fatal("vsftpd does not know the login")
	}
	for _, command := range host.commands {
		if strings.Contains(command, "a long secret") {
			t.Fatalf("the password reached a command line: %s", command)
		}
	}
}

func TestFTPLoginOwnerDropInChangesNothing(t *testing.T) {
	host := installFileAccessFakeHost(t)
	installFTPLoginSeams(t, host)
	host.files[fileaccess.SSHDropInPath] = "Match User example_com\n\tChrootDirectory /srv\n"
	response := ftpLoginApply(t, transport.ApplyFTPLoginRequest{Site: exampleSite, Action: transport.FTPLoginAdd,
		Login: "anna-up", Folder: "/", Password: "a long secret"})
	if response.Refusal != transport.FTPLoginRefusedSSHDOwner {
		t.Fatalf("response %+v", response)
	}
	if host.index("useradd") >= 0 || host.index("write") >= 0 || host.index("systemctl") >= 0 {
		t.Fatalf("something changed: %v", host.commands)
	}
}

func TestFTPLoginFailedSSHDPutsEverythingBack(t *testing.T) {
	host := installFileAccessFakeHost(t)
	installFTPLoginSeams(t, host)
	host.sshdEffective = "chrootdirectory none\n"
	response := ftpLoginApply(t, transport.ApplyFTPLoginRequest{Site: exampleSite, Action: transport.FTPLoginAdd,
		Login: "anna-up", Folder: "/", Password: "a long secret"})
	if response.Refusal != transport.FTPLoginRefusedSSHDInclude {
		t.Fatalf("response %+v", response)
	}
	if host.index("chpasswd") >= 0 {
		t.Fatal("a password was set for a login sshd does not confine")
	}
	if _, ok := host.files[fileaccess.LoginUnitPath("anna-up")]; ok {
		t.Fatal("the unit outlived the refusal")
	}
	if host.index("systemctl disable --now celikpanel-ftp-anna-up.service") < 0 {
		t.Fatalf("the jail was not stopped: %v", host.commands)
	}
}

func TestFileAccessChangesKeepTheLogins(t *testing.T) {
	host := installFileAccessFakeHost(t)
	text, err := fileaccess.RenderSSHDropIn(nil, []fileaccess.Login{annaLogin()})
	if err != nil {
		t.Fatal(err)
	}
	host.files[fileaccess.SSHDropInPath] = text
	if response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true}); !response.Applied {
		t.Fatalf("enable %+v", response)
	}
	_, body := fileaccess.Classify([]byte(host.files[fileaccess.SSHDropInPath]), true)
	entries, logins, err := fileaccess.ParseSSHDropIn(body)
	if err != nil || len(entries) != 1 || len(logins) != 1 || logins[0] != annaLogin() {
		t.Fatalf("after a site change: %+v %+v %v", entries, logins, err)
	}
	if !strings.Contains(body, "User !anna-up,!example_com,*") {
		t.Fatalf("the deny block lost the login:\n%s", body)
	}
	// A full disk makes the subscription's logins read-only too.
	var resp transport.SetFileAccessReadOnlyResponse
	if err := (&Agent{}).SetFileAccessReadOnlyV1(&transport.SetFileAccessReadOnlyRequest{SubscriptionID: 3, ReadOnly: true}, &resp); err != nil || !resp.Applied {
		t.Fatalf("read-only %+v %v", resp, err)
	}
	_, body = fileaccess.Classify([]byte(host.files[fileaccess.SSHDropInPath]), true)
	if !strings.Contains(body, "folder=/public_html/uploads readonly=1") || !strings.Contains(body, "ForceCommand internal-sftp -R -d /files -u 0027") {
		t.Fatalf("read-only login:\n%s", body)
	}
	if !strings.Contains(host.files[filepath.Join(fileaccess.VsftpdUserConfDir, "anna-up")], "write_enable=NO") {
		t.Fatal("FTP stays writable for the login")
	}
}

func TestFTPLoginRemoveClosesStepByStep(t *testing.T) {
	host := installFileAccessFakeHost(t)
	installFTPLoginSeams(t, host)
	text, _ := fileaccess.RenderSSHDropIn(nil, []fileaccess.Login{annaLogin()})
	host.files[fileaccess.SSHDropInPath] = text
	host.accounts["anna-up"] = fileAccessAccount{Name: "anna-up", UID: 1001, GID: 1001, Home: fileaccess.LoginHome("anna-up"), Shell: fileaccess.NoLoginShell, Password: fileaccess.PasswordSet}
	unit, _ := fileaccess.RenderLoginUnit(annaLogin(), "/usr/lib/celikpanel-sandbox/sandbox-helper")
	host.files[fileaccess.LoginUnitPath("anna-up")] = unit
	ftpLoginJailMounted = func(string) bool { return false }
	response := ftpLoginApply(t, transport.ApplyFTPLoginRequest{Site: exampleSite, Action: transport.FTPLoginRemove, Login: "anna-up"})
	if !response.Applied {
		t.Fatalf("remove %+v", response)
	}
	lock, kill, stop, dropIn, userdel := host.index("usermod -L anna-up"), host.index("kill-sessions anna-up"),
		host.index("systemctl disable --now celikpanel-ftp-anna-up.service"), host.index("write "+fileaccess.SSHDropInPath), host.index("userdel -f anna-up")
	if lock < 0 || kill < lock || stop < kill || dropIn < stop || userdel < dropIn {
		t.Fatalf("order: lock %d, kill %d, stop %d, drop-in %d, userdel %d\n%s", lock, kill, stop, dropIn, userdel, strings.Join(host.commands, "\n"))
	}
	if strings.Contains(host.files[fileaccess.SSHDropInPath], "anna-up") {
		t.Fatal("the login is still in the drop-in")
	}
	if _, ok := host.files[fileaccess.LoginUnitPath("anna-up")]; ok {
		t.Fatal("the unit is still there")
	}
	// Another site cannot remove it.
	host.files[fileaccess.SSHDropInPath] = text
	other := transport.FileAccessSite{SubscriptionID: 3, DomainID: 6, Username: "shop_test"}
	if response := ftpLoginApply(t, transport.ApplyFTPLoginRequest{Site: other, Action: transport.FTPLoginRemove, Login: "anna-up"}); response.Refusal != transport.FTPLoginRefusedNoLogin {
		t.Fatalf("another site removed it: %+v", response)
	}
}
