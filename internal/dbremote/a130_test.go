package dbremote

import (
	"strings"
	"testing"
)

// alpha.130: MariaDB lines naming a host (host name lookup on) ban nobody;
// lines naming an address ban that address. The lines are MariaDB's own
// refused sign-ins as Fail2ban's systemd backend hands them over, the first
// two taken from the alpha.129 lab (Debian 13, MariaDB 11.8, Fail2ban 1.1),
// where '_gateway' was the client's resolved name.
func TestMariaDBFailRegexBansAddressesNotHostNames(t *testing.T) {
	names := []string{
		" 44 [Warning] Access denied for user 'cust1_rmy_user'@'_gateway' (using password: YES)\n",
		"host1 mariadbd[79962]: 2026-11-07 19:00:37 44 [Warning] Access denied for user 'u'@'_gateway' (using password: YES)\n",
		"srv1 mariadbd[812]: 2026-11-08 10:00:00 31 [Warning] Access denied for user 'shop_u'@'dsl-203-0-113-7.isp.example' (using password: YES)",
		"srv1 mariadbd[812]: 2026-11-08 10:00:00 31 [Warning] Access denied for user 'shop_u'@'localhost' (using password: YES)",
		"srv1 mariadbd[812]: 2026-11-08 10:00:00 31 [Warning] Access denied for user 'shop_u'@'cafe' (using password: YES)",
	}
	for _, multiline := range []bool{false, true} {
		pattern := failRegexForGo(multiline)
		for _, line := range names {
			if got := pattern.FindStringSubmatch(line); got != nil {
				t.Errorf("multiline %v: a host name line would ban %q: %q", multiline, got[1], line)
			}
		}
		for line, want := range map[string]string{
			" 44 [Warning] Access denied for user 'cust1_rmy_user'@'192.0.3.1' (using password: YES)\n":                                "192.0.3.1",
			"host1 mariadbd[79962]: 2026-11-08 19:00:37 44 [Warning] Access denied for user 'u'@'2001:db8::7' (using password: YES)\n": "2001:db8::7",
		} {
			if got := pattern.FindStringSubmatch(line); got == nil || got[1] != want {
				t.Errorf("multiline %v: %q -> %v, want %s", multiline, line, got, want)
			}
		}
	}
}

// The alpha.129 filter (with <HOST>) is CelikPanel's earlier one and is
// replaced on the next apply.
func TestMariaDBFilterWithHostIsAnEarlierVersion(t *testing.T) {
	earlier := strings.Replace(MariaDBFilter(), "<ADDR>", "<HOST>", 1)
	if !MariaDBFilterEarlier(earlier) || MariaDBFilterEarlier(MariaDBFilter()) {
		t.Fatal("the alpha.129 filter is not recognised as CelikPanel's earlier one")
	}
}

func TestMariaDBOptionFileWithNameLookupOff(t *testing.T) {
	cert, key := "/etc/ssl/celikpanel-database/mariadb/server.crt", "/etc/ssl/celikpanel-database/mariadb/server.key"
	for _, text := range []string{MariaDBOptionFileWith("", "", true), MariaDBOptionFileWith(cert, key, true)} {
		if !IsOwnMariaDBOptionFile(text, cert, key) || !MariaDBOptionFileSkipsNameResolve(text) || !strings.Contains(text, "\nskip-name-resolve\n") {
			t.Fatalf("not recognised as CelikPanel's: %q", text)
		}
	}
	if MariaDBOptionFileSkipsNameResolve(MariaDBOptionFile("", "")) || IsOwnMariaDBOptionFile(MariaDBOptionFileWith("", "", true)+"x = 1\n", cert, key) {
		t.Fatal("an edit or a file without the setting counts as one with it")
	}
}

// PostgreSQL's refused sign-ins with CelikPanel's client marker, in the two
// package-default prefixes, from a Debian log file (date cut by Fail2ban)
// and from the journal (Arch).
func TestPGFailRegexReadsRefusedSignIns(t *testing.T) {
	matches := map[string]string{
		// Debian/Ubuntu '%m [%p] %q%u@%d ' + marker; Fail2ban cut the date.
		" UTC [41234] shop_user@shop_db celikpanel-client=203.0.113.7 FATAL:  password authentication failed for user \"shop_user\"": "203.0.113.7",
		" [41234] shop_user@shop_db celikpanel-client=203.0.113.7 FATAL:  password authentication failed for user \"shop_user\"\n":   "203.0.113.7",
		" +03 [41234] shop_user@shop_db celikpanel-client=2001:db8::7 FATAL:  password authentication failed for user \"shop_user\"":  "2001:db8::7",
		// With the date still there.
		"2026-11-08 10:00:00.123 UTC [41234] shop_user@shop_db celikpanel-client=198.51.100.4 FATAL:  password authentication failed for user \"shop_user\"": "198.51.100.4",
		// PostgreSQL's default '%m [%p] ' + '%q' + marker, from the journal.
		"arch postgres[812]: 2026-11-08 10:00:00.123 UTC [812] celikpanel-client=203.0.113.9 FATAL:  password authentication failed for user \"shop_user\"": "203.0.113.9",
	}
	for _, multiline := range []bool{false, true} {
		pattern := failRegexOf(PGFailRegex, multiline)
		for line, want := range matches {
			if got := pattern.FindStringSubmatch(line); got == nil || got[1] != want {
				t.Errorf("multiline %v: %q -> %v, want %s", multiline, line, got, want)
			}
		}
		for _, line := range []string{
			// Not a wrong password.
			" UTC [41234] shop_user@shop_db celikpanel-client=203.0.113.7 FATAL:  no pg_hba.conf entry for host \"203.0.113.7\", user \"shop_user\", database \"shop_db\", no encryption",
			" UTC [41234] shop_user@shop_db celikpanel-client=203.0.113.7 LOG:  connection authorized: user=shop_user database=shop_db SSL enabled",
			// log_hostname on: a name, never banned.
			" UTC [41234] shop_user@shop_db celikpanel-client=host.example FATAL:  password authentication failed for user \"shop_user\"",
			// A forged marker inside a crafted user name cannot reach the end.
			" UTC [41234] x@shop_db celikpanel-client=203.0.113.7 FATAL:  password authentication failed for user \"a celikpanel-client=6.6.6.6 FATAL:  password authentication failed for user \"b\"",
			// The owner's own prefix (no marker).
			"2026-11-08 10:00:00 UTC [41234]: user=shop_user,db=shop_db,client=203.0.113.7 FATAL:  password authentication failed for user \"shop_user\"",
		} {
			if got := pattern.FindStringSubmatch(line); got != nil {
				t.Errorf("multiline %v: matched %q (ban %s)", multiline, line, got[1])
			}
		}
	}
}

// A line forged into a log FILE by a crafted name must hold the date, the
// process number, the marker and the whole message: more than PostgreSQL's
// 63-byte limit of a startup name.
func TestPGFailRegexNeedsMoreThanOneStartupName(t *testing.T) {
	shortest := "[1] celikpanel-client=1.2.3.4 FATAL: password authentication failed for user \"a\""
	if !failRegexOf(PGFailRegex, false).MatchString(shortest) {
		t.Fatalf("test setup: %q does not match", shortest)
	}
	if len(shortest) <= 63 {
		t.Fatal("a forged line fits into one startup name")
	}
}

func TestPGPrefixMarkerOnlyOverPackageDefaults(t *testing.T) {
	for current, want := range map[string]string{
		"%m [%p] ":            "%m [%p] %qcelikpanel-client=%h ",
		"%m [%p] %q%u@%d ":    "%m [%p] %q%u@%d celikpanel-client=%h ",
		"%t [%p]: user=%u ":   "",
		"%m [%p] %q%u@%d %h ": "",
		"":                    "",
	} {
		if got := PGPrefixWithMarker(current); got != want {
			t.Errorf("%q -> %q, want %q", current, got, want)
		}
	}
	if !PGPrefixCarriesMarker("%m [%p] %q%u@%d celikpanel-client=%h ") || PGPrefixCarriesMarker("%m [%p] ") {
		t.Fatal("marker detection")
	}
}

func TestPGJailRendering(t *testing.T) {
	file, err := PGJail(5432, PGLogSource{File: "/var/log/postgresql/postgresql-17-main.log"})
	if err != nil || !strings.Contains(file, "[celikpanel-postgresql-auth-5432]\nenabled = true\nfilter = celikpanel-postgresql-auth\nport = 5432\nbackend = auto\nlogpath = /var/log/postgresql/postgresql-17-main.log\n") {
		t.Fatalf("file jail = %s, %v", file, err)
	}
	journal, err := PGJail(5433, PGLogSource{Unit: "postgresql.service"})
	if err != nil || !strings.Contains(journal, "backend = systemd\njournalmatch = _SYSTEMD_UNIT=postgresql.service\n") {
		t.Fatalf("journal jail = %s, %v", journal, err)
	}
	for _, bad := range []PGLogSource{{File: "/etc/passwd"}, {File: "/var/log/postgresql/../x.log"}, {Unit: "x;y"}, {}} {
		if _, err := PGJail(5432, bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if port, ok := PGJailPort("celikpanel-database-remote-postgresql-5433.conf"); !ok || port != 5433 {
		t.Fatal("jail file name not read")
	}
	if _, ok := PGJailPort("celikpanel-database-remote.conf"); ok {
		t.Fatal("the MariaDB jail counted as a PostgreSQL one")
	}
	if !strings.Contains(PGFilter(), "failregex = "+PGFailRegex+"\n") || !strings.Contains(PGFilter(), "datepattern = ") {
		t.Fatal("filter text")
	}
	for v, want := range map[string]bool{"C": true, "C.UTF-8": true, "en_US.UTF-8": true, "": true, "tr_TR.UTF-8": false, "de_DE": false} {
		if PGLocaleEnglish(v) != want {
			t.Errorf("lc_messages %q", v)
		}
	}
}

func TestStateCountsTheNewChanges(t *testing.T) {
	for _, ts := range []TargetState{{SkipNameResolve: true}, {LogPrefix: true}, {PGJail: true}} {
		if ts.Empty() {
			t.Fatalf("%+v counted as empty: the change would be forgotten", ts)
		}
	}
}
