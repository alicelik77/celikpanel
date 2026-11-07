package fileaccess

import (
	"strings"
	"testing"
)

func testLogin() Login {
	return Login{Login: "anna-uploads", Site: "example_com", SubscriptionID: 3, DomainID: 5, Folder: "/public_html/uploads"}
}

func TestLoginNamesNeverTakeASiteAccountName(t *testing.T) {
	for _, ok := range []string{"anna-uploads", "a-b", "web-2024", "x_y-z9"} {
		if err := ValidateLoginName(ok); err != nil {
			t.Fatalf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"example_com", // a site account's form: no hyphen
		"anna", "ab", "-anna", "anna-", "Anna-x", "an na-x", "anna--x", "1anna-x", "anna.x-y",
		"celikpanel-x", "a*-b", "!a-b", "a,b-c", strings.Repeat("a", 31) + "-b",
	} {
		if err := ValidateLoginName(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestLoginFolderIsCleanedWithoutTouchingDisk(t *testing.T) {
	for in, want := range map[string]string{
		"": "/", "/": "/", "public_html": "/public_html", "/public_html/uploads/": "/public_html/uploads",
		".well-known": "/.well-known",
	} {
		got, err := CleanLoginFolder(in)
		if err != nil || got != want {
			t.Fatalf("%q -> %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"..", "/a/../b", "a/./b", "a//b", "a b", "a\nb", "a/\x00", "a;b", "a$b",
		strings.Repeat("a/", LoginFolderMaxDepth+1)} {
		if _, err := CleanLoginFolder(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestLoginFolderPathRoundTrips(t *testing.T) {
	l := testLogin()
	host := LoginFolderPath(l.SubscriptionID, l.DomainID, l.Folder)
	if host != "/var/www/celikpanel/subscriptions/3/sites/5/public_html/uploads" {
		t.Fatalf("host path %s", host)
	}
	sub, dom, folder, err := SplitLoginFolderPath(host)
	if err != nil || sub != 3 || dom != 5 || folder != l.Folder {
		t.Fatalf("split %d %d %q %v", sub, dom, folder, err)
	}
	if _, _, folder, err := SplitLoginFolderPath(SiteHome(3, 5)); err != nil || folder != "/" {
		t.Fatalf("home split %q %v", folder, err)
	}
	for _, bad := range []string{
		"/var/www/celikpanel/subscriptions/3/sites", "/var/www/celikpanel/subscriptions/3/x/5",
		"/var/www/celikpanel/subscriptions/03/sites/5", "/var/www/celikpanel/subscriptions/3/sites/5/../6",
		"/etc/passwd", "/var/www/celikpanel/subscriptions/3/sites/5/a b",
	} {
		if _, _, _, err := SplitLoginFolderPath(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestDropInConfinesLoginsAndDeniesThemWithoutTheirBlock(t *testing.T) {
	entries := []Entry{{Username: "example_com", SubscriptionID: 3, DomainID: 5}}
	text, err := RenderSSHDropIn(entries, []Login{testLogin()})
	if err != nil {
		t.Fatal(err)
	}
	state, body := Classify([]byte(text), true)
	if state != FileManaged {
		t.Fatalf("not sealed: %s", state)
	}
	for _, want := range []string{
		"# celikpanel-login login=anna-uploads site=example_com subscription=3 domain=5 folder=/public_html/uploads readonly=0\n" +
			"Match User anna-uploads\n\tChrootDirectory /run/celikpanel-ftp/anna-uploads\n\tPubkeyAuthentication no\n" +
			"\tForceCommand internal-sftp -d /files -u 0027\n\tPermitTTY no\n\tDisableForwarding yes\n\tPermitTunnel no\n\tPermitUserRC no\n",
		"Match Group celikpanel-site User !anna-uploads,!example_com,*\n\tDenyGroups celikpanel-site\n",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("drop-in lacks %q:\n%s", want, body)
		}
	}
	gotEntries, gotLogins, err := ParseSSHDropIn(body)
	if err != nil || len(gotEntries) != 1 || len(gotLogins) != 1 || gotLogins[0] != testLogin() {
		t.Fatalf("read back %+v %+v %v", gotEntries, gotLogins, err)
	}
	// Without logins the text is exactly alpha.117's.
	plain, _ := RenderSSHDropIn(entries, nil)
	if strings.Contains(plain, "celikpanel-login") || strings.Contains(plain, "Additional FTP") {
		t.Fatalf("logins section without logins:\n%s", plain)
	}
	// A login cannot carry a site account's name.
	clash := testLogin()
	clash.Login = "example-com"
	if _, err := RenderSSHDropIn([]Entry{{Username: "example-com", SubscriptionID: 1, DomainID: 1}}, []Login{clash}); err == nil {
		t.Fatal("clashing name accepted")
	}
	if _, err := RenderSSHDropIn(nil, []Login{testLogin(), testLogin()}); err == nil {
		t.Fatal("duplicate login accepted")
	}
}

func TestReadOnlyLoginIsReadOnlyOverSFTPAndFTP(t *testing.T) {
	l := testLogin()
	l.ReadOnly = true
	text, err := RenderSSHDropIn(nil, []Login{l})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "ForceCommand internal-sftp -R -d /files -u 0027\n") || !strings.Contains(text, " readonly=1\n") {
		t.Fatalf("read-only login:\n%s", text)
	}
	if want := ExpectedLoginSSHDEffective(l)["forcecommand"]; want != "internal-sftp -R -d /files -u 0027" {
		t.Fatalf("expected forcecommand %q", want)
	}
	conf, err := RenderVsftpdLoginConf(l)
	if err != nil || !strings.Contains(conf, "write_enable=NO\n") || strings.Contains(conf, "local_root") {
		t.Fatalf("vsftpd login conf %q %v", conf, err)
	}
	open, _ := RenderVsftpdLoginConf(testLogin())
	if strings.Contains(open, "write_enable") {
		t.Fatalf("open login is read-only: %q", open)
	}
}

func TestMalformedLoginRecordsAreRefused(t *testing.T) {
	for _, line := range []string{
		loginMarker + "login=anna-uploads site=example_com subscription=3 domain=5 folder=/../x readonly=0",
		loginMarker + "login=anna site=example_com subscription=3 domain=5 folder=/ readonly=0",
		loginMarker + "login=anna-uploads site=example_com subscription=x domain=5 folder=/ readonly=0",
		loginMarker + "login=anna-uploads site=example_com subscription=3 domain=5 folder=/ readonly=2",
		loginMarker + "login=anna-uploads site=Example subscription=3 domain=5 folder=/ readonly=0",
		loginMarker + "login=anna-uploads site=example_com subscription=3 domain=5 folder=public readonly=0",
	} {
		if _, _, err := ParseSSHDropIn(line + "\n"); err == nil {
			t.Fatalf("accepted %q", line)
		}
	}
}

func TestLoginUnitRunsTheHelperWithTheRecordedFolder(t *testing.T) {
	text, err := RenderLoginUnit(testLogin(), "/usr/lib/celikpanel-sandbox/sandbox-helper")
	if err != nil {
		t.Fatal(err)
	}
	if state, _ := Classify([]byte(text), true); state != FileManaged {
		t.Fatalf("unit not sealed: %s", state)
	}
	for _, want := range []string{
		"ExecStart=/usr/lib/celikpanel-sandbox/sandbox-helper ftp-bind --login=anna-uploads --site=example_com --folder=/var/www/celikpanel/subscriptions/3/sites/5/public_html/uploads\n",
		"ExecStop=/usr/lib/celikpanel-sandbox/sandbox-helper ftp-unbind --login=anna-uploads\n",
		"Type=oneshot\nRemainAfterExit=yes\n",
		"WantedBy=multi-user.target\n",
		"RequiresMountsFor=/var/www/celikpanel/subscriptions\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("unit lacks %q:\n%s", want, text)
		}
	}
	if _, err := RenderLoginUnit(testLogin(), "relative/helper"); err == nil {
		t.Fatal("relative helper accepted")
	}
}

func TestVsftpdListsLoginsAndChrootsThemThroughTheirHome(t *testing.T) {
	list, err := RenderVsftpdUserList([]Entry{{Username: "example_com", SubscriptionID: 3, DomainID: 5}}, []Login{testLogin()})
	if err != nil || !strings.Contains(list, "\nanna-uploads\nexample_com\n") {
		t.Fatalf("list %q %v", list, err)
	}
	params := VsftpdParams{CertFile: "/c.pem", KeyFile: "/k.pem", SecureChrootDir: "/var/run/vsftpd/empty"}
	without, _ := RenderVsftpdConf(params)
	params.Logins = true
	with, _ := RenderVsftpdConf(params)
	if strings.Contains(without, "passwd_chroot_enable") || !strings.Contains(with, "passwd_chroot_enable=YES\n") {
		t.Fatalf("passwd_chroot_enable: without=%v with=%v", strings.Contains(without, "passwd_chroot"), strings.Contains(with, "passwd_chroot"))
	}
	if !strings.Contains(with, "allow_writeable_chroot=NO\n") || !strings.Contains(with, "chroot_local_user=YES\n") {
		t.Fatal("chroot settings lost")
	}
	if LoginHome("anna-uploads") != "/run/celikpanel-ftp/anna-uploads/./files" {
		t.Fatalf("home %s", LoginHome("anna-uploads"))
	}
}

func TestVsftpdOnArchRunsWithoutTheSeccompSandbox(t *testing.T) {
	params := VsftpdParams{CertFile: "/c.pem", KeyFile: "/k.pem", SecureChrootDir: "/usr/share/empty", NoSeccomp: VsftpdNeedsNoSeccomp("pacman")}
	arch, _ := RenderVsftpdConf(params)
	params.NoSeccomp = VsftpdNeedsNoSeccomp("apt")
	debian, _ := RenderVsftpdConf(params)
	if !strings.Contains(arch, "\nseccomp_sandbox=NO\n") || strings.Contains(debian, "seccomp_sandbox") {
		t.Fatalf("seccomp: arch=%v debian=%v", strings.Contains(arch, "seccomp"), strings.Contains(debian, "seccomp"))
	}
}
