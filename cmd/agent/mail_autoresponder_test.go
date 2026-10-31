package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func TestAutoresponderSieveWithoutDatesRepliesUnconditionally(t *testing.T) {
	script := renderAutoresponderSieve(transport.MailAutoresponder{
		Enabled: true, Subject: "Away", Message: "Back on Monday.", IntervalDays: 1,
	})
	if !digestHeaderIntact([]byte(script), autoresponderScriptHeader) {
		t.Fatalf("digest header does not match:\n%s", script)
	}
	if !strings.Contains(script, `require ["vacation"];`) || strings.Contains(script, "currentdate") {
		t.Fatalf("unexpected requires:\n%s", script)
	}
	if !strings.Contains(script, "vacation :days 1 :subject \"Away\" \"Back on Monday.\";\n") {
		t.Fatalf("vacation action missing:\n%s", script)
	}
}

func TestAutoresponderSieveDatesAddressesAndQuoting(t *testing.T) {
	script := renderAutoresponderSieve(transport.MailAutoresponder{
		Enabled: true, Subject: `He said "hi" \o/`, Message: "Line 1\n.\nLine \"3\"",
		StartDate: "2026-11-01", EndDate: "2026-11-09", IntervalDays: 7,
		Addresses: []string{"info@example.test", "sales@example.test"},
	})
	for _, want := range []string{
		`require ["vacation", "date", "relational"];`,
		`if allof (currentdate :value "ge" "date" "2026-11-01", currentdate :value "le" "date" "2026-11-09") {`,
		`:days 7 :subject "He said \"hi\" \\o/"`,
		`:addresses ["info@example.test", "sales@example.test"]`,
		"\"Line 1\n.\nLine \\\"3\\\"\";",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("missing %q in:\n%s", want, script)
		}
	}
	only := renderAutoresponderSieve(transport.MailAutoresponder{Subject: "x", Message: "y", EndDate: "2026-12-31", IntervalDays: 1})
	if !strings.Contains(only, `if currentdate :value "le" "date" "2026-12-31" {`) || strings.Contains(only, "allof") {
		t.Fatalf("end date only:\n%s", only)
	}
}

func TestClassifyMailboxSieveNeverClaimsTheOwnersScript(t *testing.T) {
	managed := []byte(renderAutoresponderSieve(transport.MailAutoresponder{Subject: "a", Message: "b", IntervalDays: 1}))
	edited := append([]byte(nil), managed...)
	edited = append(edited, []byte("discard;\n")...)
	cases := []struct {
		name  string
		state mailboxSieveState
		want  string
	}{
		{"nothing", mailboxSieveState{LinkKind: "none"}, transport.AutoresponderScriptNone},
		{"ours active", mailboxSieveState{LinkKind: "link", LinkTarget: autoresponderLinkTarget, ScriptExists: true, ScriptContent: managed}, transport.AutoresponderScriptManaged},
		{"ours dangling", mailboxSieveState{LinkKind: "link", LinkTarget: autoresponderLinkTarget}, transport.AutoresponderScriptNone},
		{"ours inactive", mailboxSieveState{LinkKind: "none", ScriptExists: true, ScriptContent: managed}, transport.AutoresponderScriptNone},
		{"ours edited", mailboxSieveState{LinkKind: "link", LinkTarget: autoresponderLinkTarget, ScriptExists: true, ScriptContent: edited}, transport.AutoresponderScriptOwner},
		{"ours edited inactive", mailboxSieveState{LinkKind: "none", ScriptExists: true, ScriptContent: edited}, transport.AutoresponderScriptOwner},
		{"roundcube", mailboxSieveState{LinkKind: "link", LinkTarget: "sieve/roundcube.sieve"}, transport.AutoresponderScriptOwner},
		{"plain file", mailboxSieveState{LinkKind: "file"}, transport.AutoresponderScriptOwner},
	}
	for _, c := range cases {
		if got := classifyMailboxSieve(c.state); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestAutoresponderDovecotDropInSpeaksBothDialects(t *testing.T) {
	v23 := renderAutoresponderDovecotConf(false)
	v24 := renderAutoresponderDovecotConf(true)
	for name, conf := range map[string]string{"2.3": v23, "2.4": v24} {
		if !digestHeaderIntact([]byte(conf), autoresponderDovecotHead) {
			t.Fatalf("%s: digest header does not match", name)
		}
		if !strings.Contains(conf, "unix_listener /var/spool/postfix/private/dovecot-lmtp {") ||
			!strings.Contains(conf, "user = postfix") || !strings.Contains(conf, "mode = 0600") {
			t.Fatalf("%s: LMTP listener missing:\n%s", name, conf)
		}
	}
	if !strings.Contains(v23, "mail_plugins = $mail_plugins sieve") || !strings.Contains(v23, "sieve = file:~/sieve;active=~/.dovecot.sieve") {
		t.Fatalf("2.3 dialect:\n%s", v23)
	}
	if strings.Contains(v24, "$mail_plugins") || !strings.Contains(v24, "sieve = yes") ||
		!strings.Contains(v24, "sieve_script personal {") || !strings.Contains(v24, "active_path = ~/.dovecot.sieve") {
		t.Fatalf("2.4 dialect:\n%s", v24)
	}
	if digestHeaderIntact([]byte(strings.Replace(v24, "0600", "0666", 1)), autoresponderDovecotHead) {
		t.Fatal("an edited drop-in still counts as CelikPanel's")
	}
}

func TestTransportMapsIncludesOurs(t *testing.T) {
	for value, want := range map[string]bool{
		"":                                   false,
		"hash:" + autoresponderTransportPath: true,
		"lmdb:" + autoresponderTransportPath: true,
		"hash:/etc/postfix/transport, lmdb:" + autoresponderTransportPath: true,
		"hash:/etc/postfix/transport":                                     false,
		"hash:" + autoresponderTransportPath + ".old":                     false,
	} {
		if got := transportMapsIncludesOurs(value); got != want {
			t.Errorf("%q: %v, want %v", value, got, want)
		}
	}
}

func TestAutoresponderSievePackagesAreOfficialPerFamily(t *testing.T) {
	if p, ok := autoresponderSievePackages("apt"); !ok || strings.Join(p, " ") != "dovecot-sieve dovecot-lmtpd" {
		t.Fatalf("apt: %v %v", p, ok)
	}
	if p, ok := autoresponderSievePackages("pacman"); !ok || strings.Join(p, " ") != "pigeonhole" {
		t.Fatalf("pacman: %v %v", p, ok)
	}
	if _, ok := autoresponderSievePackages("dnf"); ok {
		t.Fatal("an unsupported family got packages")
	}
}

func TestNormalizeMailAutoresponderRefusesBadInput(t *testing.T) {
	ok := transport.MailAutoresponder{Enabled: true, Subject: "a", Message: "b"}
	got, err := transport.NormalizeMailAutoresponder(ok, "me@example.test")
	if err != nil || got.IntervalDays != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	for name, in := range map[string]transport.MailAutoresponder{
		"empty message":  {Enabled: true, Subject: "a"},
		"subject break":  {Enabled: true, Subject: "a\nBcc: x@y", Message: "b"},
		"interval":       {Enabled: true, Subject: "a", Message: "b", IntervalDays: 31},
		"date":           {Enabled: true, Subject: "a", Message: "b", StartDate: "01.11.2026"},
		"end before":     {Enabled: true, Subject: "a", Message: "b", StartDate: "2026-11-02", EndDate: "2026-11-01"},
		"address":        {Enabled: true, Subject: "a", Message: "b", Addresses: []string{"not an address"}},
		"control in msg": {Enabled: true, Subject: "a", Message: "b\x00"},
	} {
		if _, err := transport.NormalizeMailAutoresponder(in, "me@example.test"); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	got, err = transport.NormalizeMailAutoresponder(transport.MailAutoresponder{
		Enabled: true, Subject: "a", Message: "b\r\nc", Addresses: []string{"ME@example.test", "Info@Example.test", "info@example.test"},
	}, "me@example.test")
	if err != nil || got.Message != "b\nc" || len(got.Addresses) != 1 || got.Addresses[0] != "info@example.test" {
		t.Fatalf("%+v %v", got, err)
	}
}
