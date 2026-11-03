package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Mailbox quotas (alpha.118, D-044): every users line carries the limit in
// both Dovecot dialects, the drop-in is CelikPanel's own (digest), and the
// Postfix check goes after reject_unauth_destination and fails open.

func TestDovecotUsersLinesGetBothLimitFields(t *testing.T) {
	in := strings.Join([]string{
		"a@x.test:{CRYPT}h::::::userdb_quota_rule=*:storage=500M",
		"b@x.test:{CRYPT}h::::::nologin=y userdb_quota_rule=*:storage=10M",
		"c@x.test:{CRYPT}h::::::" + dovecotQuotaFields(20),
		"d@x.test:{CRYPT}h::::::",
	}, "\n") + "\n"
	out, changed := normalizeDovecotUsersQuota([]byte(in))
	if !changed {
		t.Fatal("nothing changed")
	}
	want := strings.Join([]string{
		"a@x.test:{CRYPT}h::::::userdb_quota_rule=*:storage=500M userdb_quota_storage_size=500M",
		"b@x.test:{CRYPT}h::::::nologin=y userdb_quota_rule=*:storage=10M userdb_quota_storage_size=10M",
		"c@x.test:{CRYPT}h::::::userdb_quota_rule=*:storage=20M userdb_quota_storage_size=20M",
		"d@x.test:{CRYPT}h::::::",
	}, "\n") + "\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
	if _, again := normalizeDovecotUsersQuota(out); again {
		t.Fatal("a second pass changed the file again")
	}
	// The suspension token stays in front of the quota tail (UpdateMailQuota
	// rewrites from userdb_quota_rule= to the end of the line).
	line, err := addSuspensionNologin("a@x.test:{CRYPT}h::::::" + dovecotQuotaFields(5))
	if err != nil || !strings.HasSuffix(line, dovecotQuotaFields(5)) {
		t.Fatalf("nologin line = %q, %v", line, err)
	}
}

func TestQuotaPolicyGoesAfterRejectUnauthDestinationAndFailsOpen(t *testing.T) {
	element := mailQuotaPolicyElement()
	if !strings.Contains(element, "default_action=DUNNO") || !strings.Contains(element, mailQuotaPolicySocket) {
		t.Fatalf("element = %q", element)
	}
	cases := map[string]string{
		"": element,
		"permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination, reject_rbl_client zen.spamhaus.org": "permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination, " + element + ", reject_rbl_client, zen.spamhaus.org",
		"check_recipient_access hash:/etc/postfix/x, permit_mynetworks, permit":                                       "check_recipient_access, hash:/etc/postfix/x, permit_mynetworks, " + element + ", permit",
	}
	for current, want := range cases {
		next, change, err := planQuotaPolicyRestriction(current)
		if err != nil || !change || next != want {
			t.Fatalf("%q -> %q (%v, %v), want %q", current, next, change, err, want)
		}
		if _, again, _ := planQuotaPolicyRestriction(next); again {
			t.Fatalf("a second pass changed %q", next)
		}
	}
	if _, _, err := planQuotaPolicyRestriction("permit, { broken"); err == nil {
		t.Fatal("unbalanced braces were accepted")
	}
}

func TestDovecotQuotaDropInIsCelikPanelsOwnTextInBothDialects(t *testing.T) {
	for _, is24 := range []bool{false, true} {
		conf := buildDovecotQuotaConf(is24)
		if !dovecotQuotaManagedIntact([]byte(conf)) {
			t.Fatalf("2.4=%v: own text not recognised", is24)
		}
		if dovecotQuotaManagedIntact([]byte(conf + "# owner\n")) {
			t.Fatalf("2.4=%v: an edited file reads as CelikPanel's", is24)
		}
		for _, want := range []string{"quota-status -p postfix", "/var/spool/postfix/" + mailQuotaPolicySocket, mailQuotaFullReply} {
			if !strings.Contains(conf, want) {
				t.Fatalf("2.4=%v: lacks %q", is24, want)
			}
		}
	}
	if !strings.Contains(buildDovecotQuotaConf(false), "quota = count:User quota") || !strings.Contains(buildDovecotQuotaConf(true), "quota_driver = count") {
		t.Fatal("the count driver is not used")
	}
}

// With Dovecot 2.4 installed (the Debian 13 development guest), its own
// parser accepts the drop-in on top of CelikPanel's virtual mailbox file.
func TestDovecot24AcceptsTheQuotaDropIn(t *testing.T) {
	doveconf, err := exec.LookPath("doveconf")
	if err != nil {
		t.Skip("doveconf is not installed")
	}
	version, err := exec.Command("dovecot", "--version").Output()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(version)), "2.4") {
		t.Skip("Dovecot 2.4 is not installed")
	}
	dir := t.TempDir()
	main := "dovecot_config_version = 2.4.1\ndovecot_storage_version = 2.4.1\n" +
		dovecotVirtualBody(true, filepath.Join(dir, "vhosts")) + dovecotQuotaBody(true)
	main = strings.ReplaceAll(main, dovecotUsersPath, filepath.Join(dir, "users"))
	path := filepath.Join(dir, "dovecot.conf")
	if err := os.WriteFile(path, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(doveconf, "-c", path, "-n").CombinedOutput()
	if err != nil {
		t.Fatalf("doveconf refused the drop-in: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "quota-status") || !strings.Contains(string(out), "quota_storage_size = 1024M") {
		t.Fatalf("doveconf output lacks the quota settings:\n%s", out)
	}
}
