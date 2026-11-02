package fileaccess

import (
	"strings"
	"testing"
)

func TestSealAndClassifyDetectOwnerEdits(t *testing.T) {
	sealed := Seal("x", "a=1\n")
	if state, body := Classify([]byte(sealed), true); state != FileManaged || body != "a=1\n" {
		t.Fatalf("sealed text classified %s %q", state, body)
	}
	if state, _ := Classify([]byte(sealed+"b=2\n"), true); state != FileOwner {
		t.Fatalf("edited text classified %s", state)
	}
	if state, _ := Classify([]byte("listen=YES\n"), true); state != FileOwner {
		t.Fatalf("distro text classified %s", state)
	}
	if state, _ := Classify(nil, false); state != FileAbsent {
		t.Fatalf("missing file classified %s", state)
	}
}

func TestSSHDropInConfinesEveryAccountAndDeniesTheRest(t *testing.T) {
	text, err := RenderSSHDropIn([]Entry{
		{Username: "shop_test", SubscriptionID: 7, DomainID: 12, SSH: true},
		{Username: "example_com", SubscriptionID: 3, DomainID: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	state, body := Classify([]byte(text), true)
	if state != FileManaged {
		t.Fatalf("rendered drop-in is not sealed: %s", state)
	}
	for _, want := range []string{
		"Match User example_com\n\tChrootDirectory /var/www/celikpanel/subscriptions/3/sites\n\tAuthorizedKeysFile /etc/ssh/celikpanel-keys/%u\n\tForceCommand internal-sftp -d /5 -u 0027\n\tPermitTTY no\n\tDisableForwarding yes\n",
		"Match User shop_test\n\tChrootDirectory /var/www/celikpanel/subscriptions/7/sites\n\tAuthorizedKeysFile /etc/ssh/celikpanel-keys/%u\n\tDisableForwarding yes\n",
		"Match Group celikpanel-site User !example_com,!shop_test,*\n\tDenyGroups celikpanel-site\n",
		"\nMatch all\n",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("drop-in lacks %q:\n%s", want, body)
		}
	}
	// The command-line account has no ForceCommand: its block ends before
	// the next marker without one.
	shop := body[strings.Index(body, "Match User shop_test"):]
	shop = shop[:strings.Index(shop, "\n\n")]
	if strings.Contains(shop, "ForceCommand") {
		t.Fatalf("ssh account is forced to SFTP:\n%s", shop)
	}
	entries, err := ParseSSHDropInEntries(body)
	if err != nil || len(entries) != 2 || entries[0].Username != "example_com" || entries[1].SSH != true || entries[1].DomainID != 12 {
		t.Fatalf("entries read back %+v %v", entries, err)
	}
}

func TestSSHDropInWithoutAccountsStillDeniesHostingAccounts(t *testing.T) {
	text, err := RenderSSHDropIn(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Match Group celikpanel-site\n\tDenyGroups celikpanel-site\n") {
		t.Fatalf("empty drop-in:\n%s", text)
	}
}

func TestSSHDropInRefusesUnsafeNamesAndDuplicates(t *testing.T) {
	for _, name := range []string{"a*", "!root", "a,b", "a b", "", "UPPER", strings.Repeat("a", 33)} {
		if _, err := RenderSSHDropIn([]Entry{{Username: name, SubscriptionID: 1, DomainID: 1}}); err == nil {
			t.Fatalf("name %q accepted", name)
		}
	}
	if _, err := RenderSSHDropIn([]Entry{{Username: "a", SubscriptionID: 1, DomainID: 1}, {Username: "a", SubscriptionID: 1, DomainID: 2}}); err == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestEffectiveConfigCheck(t *testing.T) {
	entry := Entry{Username: "example_com", SubscriptionID: 3, DomainID: 5}
	output := "port 22\nchrootdirectory /var/www/celikpanel/subscriptions/3/sites\nauthorizedkeysfile /etc/ssh/celikpanel-keys/%u\n" +
		"disableforwarding yes\npermittunnel no\nforcecommand internal-sftp -d /5 -u 0027\n"
	if err := CheckSSHDEffective(output, ExpectedSSHDEffective(entry, true)); err != nil {
		t.Fatal(err)
	}
	// A server whose sshd_config does not include the drop-in folder.
	if err := CheckSSHDEffective("port 22\nchrootdirectory none\n", ExpectedSSHDEffective(entry, true)); err == nil {
		t.Fatal("missing chroot accepted")
	}
	if err := CheckSSHDEffective("denygroups other celikpanel-site\n", ExpectedSSHDEffective(entry, false)); err != nil {
		t.Fatal(err)
	}
}

func TestMergeEntry(t *testing.T) {
	entries := []Entry{{Username: "a", SubscriptionID: 1, DomainID: 1}}
	added := MergeEntry(entries, Entry{Username: "b", SubscriptionID: 1, DomainID: 2, SSH: true}, true)
	if len(added) != 2 {
		t.Fatalf("add: %+v", added)
	}
	changed := MergeEntry(added, Entry{Username: "b", SubscriptionID: 1, DomainID: 2}, true)
	if e, _ := FindEntry(changed, "b"); e.SSH {
		t.Fatalf("change: %+v", changed)
	}
	removed := MergeEntry(changed, Entry{Username: "a"}, false)
	if _, ok := FindEntry(removed, "a"); ok || len(removed) != 1 {
		t.Fatalf("remove: %+v", removed)
	}
}

func TestShellAndPasswordDecisions(t *testing.T) {
	if ShellFor(true, true) != JailShell || ShellFor(true, false) != NoLoginShell || ShellFor(false, true) != NoLoginShell {
		t.Fatal("shell decision")
	}
	for shell, usable := range map[string]bool{"/bin/bash": true, "/usr/sbin/nologin": false, "/bin/false": false, "": false, "/bin/sh": true} {
		if UsableShell(shell) != usable {
			t.Fatalf("UsableShell(%q)", shell)
		}
	}
	for field, want := range map[string]PasswordState{
		"": PasswordEmpty, "*": PasswordNone, "!": PasswordDisabled, "!!": PasswordDisabled,
		"$y$j9T$abc": PasswordSet, "!$6$abc": PasswordLocked,
	} {
		if got := ClassifyShadowPassword(field); got != want {
			t.Fatalf("shadow %q = %s, want %s", field, got, want)
		}
	}
	for _, bad := range []string{"short", "line\nbreak", "tab\there!!", strings.Repeat("x", 129)} {
		if ValidatePassword(bad) == nil {
			t.Fatalf("password %q accepted", bad)
		}
	}
	if err := ValidatePassword("correct horse: battery"); err != nil {
		t.Fatal(err)
	}
}

func TestChrootSafety(t *testing.T) {
	chain := ChrootChain("/var/www/celikpanel/subscriptions/3/sites")
	if len(chain) != 7 || chain[0] != "/" || chain[6] != "/var/www/celikpanel/subscriptions/3/sites" {
		t.Fatalf("chain %v", chain)
	}
	safe := []DirInfo{{Path: "/", IsDir: true, Mode: 0o755}, {Path: "/var", IsDir: true, Mode: 0o751}}
	if dir, _ := UnsafeChrootComponent(safe); dir != "" {
		t.Fatalf("safe chain refused at %s", dir)
	}
	if dir, reason := UnsafeChrootComponent(append(safe, DirInfo{Path: "/var/www", IsDir: true, UID: 33, Mode: 0o755})); dir != "/var/www" || reason != "not owned by root" {
		t.Fatalf("foreign owner: %s %s", dir, reason)
	}
	if dir, _ := UnsafeChrootComponent(append(safe, DirInfo{Path: "/var/www", IsDir: true, Mode: 0o775})); dir != "/var/www" {
		t.Fatal("group-writable accepted")
	}
}
