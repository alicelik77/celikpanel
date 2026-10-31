package main

import (
	"fmt"
	"strings"
	"testing"
)

// fakeMasterCF answers `postconf -F <entry>` from a table of entries
// ("service/type" -> "private unpriv chroot wakeup maxproc command") and
// records every other call.
type fakeMasterCF struct {
	entries map[string]string
	writes  [][]string
	failOn  string
}

func (f *fakeMasterCF) run(name string, args ...string) ([]byte, error) {
	if len(args) == 2 && args[0] == "-F" && !strings.Contains(args[1], "=") {
		fields, ok := f.entries[args[1]]
		if !ok {
			return []byte("postconf: warning: " + args[1] + ": unknown service\n"), nil
		}
		v := strings.Fields(fields)
		service, kind, _ := strings.Cut(args[1], "/")
		var b strings.Builder
		for i, field := range []string{"private", "unprivileged", "chroot", "wakeup", "process_limit"} {
			fmt.Fprintf(&b, "%s/%s/%s = %s\n", service, kind, field, v[i])
		}
		fmt.Fprintf(&b, "%s/%s/service = %s\n%s/%s/type = %s\n", service, kind, service, service, kind, kind)
		fmt.Fprintf(&b, "%s/%s/command = %s\n", service, kind, strings.Join(v[5:], " "))
		return []byte(b.String()), nil
	}
	f.writes = append(f.writes, append([]string{name}, args...))
	if f.failOn != "" && strings.Contains(strings.Join(args, " "), f.failOn) {
		return []byte("postconf: fatal: synthetic failure"), fmt.Errorf("exit status 1")
	}
	return nil, nil
}

// D3: the submission services follow the chroot of the distribution's own
// smtp service — "n" on Arch and upstream, "y" on Debian and Ubuntu.
func TestDistributionSubmissionChrootFollowsSMTPService(t *testing.T) {
	for _, tc := range []struct {
		why, smtp, want string
	}{
		{"Arch, upstream", "n - n - - smtpd", "n"},
		{"Debian, Ubuntu", "n - y - - smtpd", "y"},
		{"Postfix default spelled as -", "n - - - - smtpd", "-"},
		{"no smtp/inet entry: Postfix's own default", "", "n"},
		{"unreadable field", "n - x - - smtpd", "n"},
	} {
		f := &fakeMasterCF{entries: map[string]string{}}
		if tc.smtp != "" {
			f.entries["smtp/inet"] = tc.smtp
		}
		if got := distributionSubmissionChroot(f.run); got != tc.want {
			t.Errorf("%s: smtp %q: chroot = %q, want %q", tc.why, tc.smtp, got, tc.want)
		}
	}
}

// The Panel-start repair moves entries an earlier release wrote (chroot y)
// to the distribution's setting, keeps the owner's entries, and leaves a
// Debian-family server untouched.
func TestReconcileSubmissionChroot(t *testing.T) {
	t.Run("arch upgrade", func(t *testing.T) {
		f := &fakeMasterCF{entries: map[string]string{
			"smtp/inet":       "n - n - - smtpd",
			"submission/inet": "n - y - - smtpd",
			"smtps/inet":      "n - y - - smtpd",
		}}
		note := reconcileSubmissionChroot(f.run)
		want := [][]string{
			{"postconf", "-F", "submission/inet/chroot=n"},
			{"postconf", "-F", "smtps/inet/chroot=n"},
		}
		if fmt.Sprint(f.writes) != fmt.Sprint(want) {
			t.Fatalf("writes = %v, want %v", f.writes, want)
		}
		if !strings.Contains(note, `chroot "n"`) {
			t.Fatalf("note = %q", note)
		}
	})
	t.Run("debian untouched", func(t *testing.T) {
		f := &fakeMasterCF{entries: map[string]string{
			"smtp/inet":       "n - y - - smtpd",
			"submission/inet": "n - y - - smtpd",
		}}
		if note := reconcileSubmissionChroot(f.run); note != "" || len(f.writes) != 0 {
			t.Fatalf("Debian-family repair wrote %v, note %q", f.writes, note)
		}
	})
	t.Run("owner entry kept", func(t *testing.T) {
		f := &fakeMasterCF{entries: map[string]string{
			"smtp/inet":       "n - n - - smtpd",
			"submission/inet": "n - y - 20 smtpd",
			"smtps/inet":      "n - n - - smtpd",
		}}
		note := reconcileSubmissionChroot(f.run)
		if len(f.writes) != 0 {
			t.Fatalf("owner's entry was rewritten: %v", f.writes)
		}
		if !strings.Contains(note, "submission/inet") || !strings.Contains(note, "server owner") {
			t.Fatalf("note = %q", note)
		}
	})
	t.Run("already following", func(t *testing.T) {
		f := &fakeMasterCF{entries: map[string]string{
			"smtp/inet":       "n - n - - smtpd",
			"submission/inet": "n - n - - smtpd",
		}}
		if note := reconcileSubmissionChroot(f.run); note != "" || len(f.writes) != 0 {
			t.Fatalf("writes %v, note %q", f.writes, note)
		}
	})
	t.Run("write fails", func(t *testing.T) {
		f := &fakeMasterCF{failOn: "smtps/inet/chroot", entries: map[string]string{
			"smtp/inet":       "n - n - - smtpd",
			"submission/inet": "n - y - - smtpd",
			"smtps/inet":      "n - y - - smtpd",
		}}
		note := reconcileSubmissionChroot(f.run)
		if !strings.Contains(note, "could not be changed") || !strings.Contains(note, "smtps/inet") {
			t.Fatalf("note = %q", note)
		}
	})
}

func TestSubmissionEntryPanelForm(t *testing.T) {
	parse := func(entry string) map[string]string {
		f := &fakeMasterCF{entries: map[string]string{"submission/inet": entry}}
		fields, ok := masterServiceFields(f.run, "submission/inet")
		if !ok {
			t.Fatalf("entry %q not parsed", entry)
		}
		return fields
	}
	if !submissionEntryPanelForm(parse("n - y - - smtpd"), "y") {
		t.Fatal("the entry alpha.113 wrote is CelikPanel's")
	}
	if !submissionEntryPanelForm(parse("n - y - - smtpd -o syslog_name=postfix/submission"), "y") {
		t.Fatal("-o arguments do not make the entry the owner's")
	}
	for _, owner := range []string{"y - y - - smtpd", "n - y - 20 smtpd", "n - y - - /usr/local/bin/filter", "n - n - - smtpd"} {
		if submissionEntryPanelForm(parse(owner), "y") {
			t.Fatalf("owner entry %q taken as CelikPanel's", owner)
		}
	}
	f := &fakeMasterCF{entries: map[string]string{}}
	if _, ok := masterServiceFields(f.run, "submission/inet"); ok {
		t.Fatal("a missing entry was read as present")
	}
}

// D3 and D-022: the milter lists are rewritten only while they hold what a
// CelikPanel release composes; the earlier "localhost" names count as the
// panel's so an upgrade moves them to the numeric address.
func TestMilterListPanelComposed(t *testing.T) {
	spamass := "unix:/spamass/spamass.sock"
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", true},
		{opendkimMilter, true},
		{opendkimMilter + ", " + rspamdMilter, true},
		{legacyOpenDKIMMilter + ", " + legacyRspamdMilter, true},
		{legacyOpenDKIMMilter + "," + spamass, true},
		{"inet:127.0.0.1:8891 inet:localhost:11332", true},
		{opendkimMilter + ", inet:127.0.0.1:10025", false},
		{"$my_milters", false},
		{"{ inet:127.0.0.1:8891, default_action=tempfail }", false},
	} {
		if got := milterListPanelComposed(tc.value, spamass); got != tc.want {
			t.Errorf("milterListPanelComposed(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
	if !strings.Contains(ownerMilterListNote([]string{"smtpd_milters = inet:127.0.0.1:10025"}), "postconf -X") {
		t.Fatal("the owner note must say how to hand the list back")
	}
}

// The addresses CelikPanel writes need no name resolution (D3).
func TestMilterAddressesAreNumeric(t *testing.T) {
	for _, address := range []string{opendkimMilter, rspamdMilter, opendkimSocket} {
		if strings.Contains(address, "localhost") || !strings.Contains(address, "127.0.0.1") {
			t.Fatalf("milter address %q depends on name resolution", address)
		}
	}
	if !strings.Contains(renderOpenDKIMConf(opendkimLayout{confPath: opendkimAPTConfPath}), "Socket\t\t\tinet:8891@127.0.0.1\n") {
		t.Fatal("OpenDKIM must listen on the numeric loopback address Postfix names")
	}
}
