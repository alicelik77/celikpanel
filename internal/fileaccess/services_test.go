package fileaccess

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"golang.org/x/crypto/ssh"
)

func TestCatalogueOpensThePassiveRange(t *testing.T) {
	service := core.GetManagedServiceByID("vsftpd")
	if service == nil {
		t.Fatal("vsftpd missing from the catalogue")
	}
	open := map[int]bool{}
	for _, port := range service.FirewallPorts {
		if port.Proto == "tcp" {
			open[port.Port] = true
		}
	}
	for _, port := range append([]int{21}, PassivePorts()...) {
		if !open[port] {
			t.Fatalf("catalogue does not open %d/tcp", port)
		}
	}
	if len(service.FirewallPorts) != 1+len(PassivePorts()) {
		t.Fatalf("catalogue opens %d ports", len(service.FirewallPorts))
	}
	if kind, reason := core.ManagedServiceInstallBlock(service, "apt"); kind != core.ManagedServiceInstallBlockNone {
		t.Fatalf("vsftpd install closed: %s", reason)
	}
}

func TestVsftpdConfRequiresTLSAndChroot(t *testing.T) {
	text, err := RenderVsftpdConf(VsftpdParams{CertFile: MailHostCert, KeyFile: MailHostKey, SecureChrootDir: SecureChrootDirFor("apt")})
	if err != nil {
		t.Fatal(err)
	}
	state, body := Classify([]byte(text), true)
	if state != FileManaged {
		t.Fatal("not sealed")
	}
	for _, want := range []string{
		"anonymous_enable=NO", "ssl_enable=YES", "force_local_logins_ssl=YES", "force_local_data_ssl=YES",
		"allow_anon_ssl=NO", "ssl_sslv3=NO", "chroot_local_user=YES", "allow_writeable_chroot=NO",
		"userlist_enable=YES", "userlist_deny=NO", "userlist_file=/etc/vsftpd/celikpanel-users.list",
		"pam_service_name=celikpanel-vsftpd", "pasv_min_port=40000", "pasv_max_port=40049", "port_enable=NO",
		"rsa_cert_file=/etc/ssl/celikpanel/_mail/host/current/fullchain.pem",
		"secure_chroot_dir=/var/run/vsftpd/empty", "user_config_dir=/etc/vsftpd/celikpanel-users",
	} {
		if !strings.Contains(body, "\n"+want+"\n") {
			t.Fatalf("vsftpd.conf lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "allow_writeable_chroot=YES") || strings.Contains(body, "ssl_enable=NO") {
		t.Fatal("unsafe setting rendered")
	}
	if _, err := RenderVsftpdConf(VsftpdParams{CertFile: "relative.pem", KeyFile: MailHostKey, SecureChrootDir: "/x"}); err == nil {
		t.Fatal("relative certificate path accepted")
	}
	if SecureChrootDirFor("pacman") != "/usr/share/empty" {
		t.Fatal("arch secure chroot dir")
	}
}

func TestVsftpdUserFilesAndPAM(t *testing.T) {
	list, err := RenderVsftpdUserList([]Entry{{Username: "b_test", SubscriptionID: 1, DomainID: 2}, {Username: "a_test", SubscriptionID: 1, DomainID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, body := Classify([]byte(list), true); !strings.HasSuffix(body, "a_test\nb_test\n") {
		t.Fatalf("user list:\n%s", list)
	}
	conf, err := RenderVsftpdUserConf(Entry{Username: "a_test", SubscriptionID: 4, DomainID: 9})
	if err != nil || !strings.HasSuffix(conf, "\nlocal_root=/var/www/celikpanel/subscriptions/4/sites\n") {
		t.Fatalf("user conf %q %v", conf, err)
	}
	apt := RenderVsftpdPAM("apt")
	arch := RenderVsftpdPAM("pacman")
	for _, text := range []string{apt, arch} {
		if strings.Contains(text, "pam_shells") || !strings.Contains(text, "sense=allow file=/etc/vsftpd/celikpanel-users.list onerr=fail") {
			t.Fatalf("PAM:\n%s", text)
		}
	}
	if !strings.Contains(apt, "@include common-auth") || !strings.Contains(arch, "auth\tinclude\tsystem-auth") {
		t.Fatal("PAM family stacks")
	}
	if jail := RenderFail2banJail(); !strings.Contains(jail, "[vsftpd]\nenabled = true\n") || !strings.Contains(jail, "40000:40049") {
		t.Fatalf("jail:\n%s", jail)
	}
}

func TestPublicKeyParsing(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshKey, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshKey)))
	key, err := ParsePublicKey(line + " laptop$(rm -rf)@home")
	if err != nil {
		t.Fatal(err)
	}
	if key.Comment != "laptoprm-rf@home" || !strings.HasPrefix(key.Fingerprint, "SHA256:") || key.Type != "ssh-ed25519" {
		t.Fatalf("key %+v", key)
	}
	for _, bad := range []string{"", "not a key", `command="sh" ` + line, line + "\n" + line} {
		if _, err := ParsePublicKey(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	smallKey, _ := ssh.NewPublicKey(&small.PublicKey)
	if _, err := ParsePublicKey(string(ssh.MarshalAuthorizedKey(smallKey))); err == nil {
		t.Fatal("1024-bit RSA accepted")
	}

	file := RenderKeysFile([]Key{key}, false)
	state, body := Classify([]byte(file), true)
	if state != FileManaged || !strings.Contains(body, "\nrestrict ssh-ed25519 ") {
		t.Fatalf("keys file:\n%s", file)
	}
	if !strings.Contains(RenderKeysFile([]Key{key}, true), "\nrestrict,pty ssh-ed25519 ") {
		t.Fatal("ssh keys lack pty")
	}
	back, err := ParseKeysFile(body)
	if err != nil || len(back) != 1 || back[0].Fingerprint != key.Fingerprint {
		t.Fatalf("read back %+v %v", back, err)
	}
}

func TestJailFiles(t *testing.T) {
	users := []JailUser{{Name: "shop_test", UID: 1005, GID: 1005, Group: "shop_test", DomainID: 12}}
	passwd := RenderJailPasswd(users)
	if !strings.Contains(passwd, "shop_test:x:1005:1005::/12:/bin/bash\n") || !strings.HasPrefix(passwd, "root:x:0:0:") {
		t.Fatalf("passwd:\n%s", passwd)
	}
	group := RenderJailGroup(users, "www-data", 33)
	if !strings.Contains(group, "www-data:x:33:\n") || !strings.Contains(group, "shop_test:x:1005:\n") {
		t.Fatalf("group:\n%s", group)
	}
	profile := RenderJailProfile(users)
	if !strings.Contains(profile, "shop_test) HOME=/12 ;;") || !strings.Contains(profile, "umask 027") {
		t.Fatalf("profile:\n%s", profile)
	}
	for _, forbidden := range []string{"sudo", "su", "ps", "passwd", "gcc", "apt", "pacman", "mount"} {
		for _, command := range JailCommands {
			if command == forbidden {
				t.Fatalf("jail carries %s", forbidden)
			}
		}
	}
	if ports := PassivePorts(); len(ports) != 50 || ports[0] != 40000 || ports[49] != 40049 {
		t.Fatalf("passive ports %v", ports)
	}
}
