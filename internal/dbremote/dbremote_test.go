package dbremote

import (
	"errors"
	"net/netip"
	"regexp"
	"strings"
	"testing"
)

func TestParseAddressAcceptsAndCanonicalises(t *testing.T) {
	cases := []struct{ engine, in, want string }{
		{EngineMySQL, "203.0.113.7", "203.0.113.7/32"},
		{EngineMySQL, " 203.0.113.7/32 ", "203.0.113.7/32"},
		{EngineMySQL, "198.51.100.9/24", "198.51.100.0/24"},
		{EnginePostgreSQL, "2001:db8::7", "2001:db8::7/128"},
		{EnginePostgreSQL, "2001:db8:1:2:3::/64", "2001:db8:1:2::/64"},
		{EngineMySQL, "2001:DB8::7", "2001:db8::7/128"},
		{EngineMySQL, "10.0.0.5", "10.0.0.5/32"}, // private: VPN and office networks
	}
	for _, c := range cases {
		got, err := CanonicalAddress(c.engine, c.in)
		if err != nil || got != c.want {
			t.Errorf("%s %q = %q, %v; want %q", c.engine, c.in, got, err, c.want)
		}
	}
}

func TestParseAddressRefuses(t *testing.T) {
	cases := []struct{ engine, in, code string }{
		{EngineMySQL, "0.0.0.0/0", AddressTooWide},
		{EnginePostgreSQL, "::/0", AddressTooWide},
		{EngineMySQL, "10.0.0.0/8", AddressTooWide},
		{EngineMySQL, "203.0.113.0/23", AddressTooWide},
		{EnginePostgreSQL, "2001:db8::/48", AddressTooWide},
		{EngineMySQL, "127.0.0.1", AddressLocal},
		{EnginePostgreSQL, "::1", AddressLocal},
		{EngineMySQL, "0.0.0.0", AddressLocal},
		{EngineMySQL, "169.254.1.1", AddressSpecial},
		{EnginePostgreSQL, "fe80::1", AddressSpecial},
		{EngineMySQL, "224.0.0.1", AddressSpecial},
		{EngineMySQL, "255.255.255.255", AddressSpecial},
		{EnginePostgreSQL, "::ffff:203.0.113.7", AddressMappedIPv6},
		{EngineMySQL, "2001:db8::/64", AddressIPv6Range},
		{EngineMySQL, "example.com", AddressInvalid},
		{EngineMySQL, "%", AddressInvalid},
		{EngineMySQL, "", AddressInvalid},
		{EnginePostgreSQL, "fe80::1%eth0", AddressInvalid},
	}
	for _, c := range cases {
		_, err := ParseAddress(c.engine, c.in)
		var refusal *AddressError
		if !errors.As(err, &refusal) || refusal.Code != c.code {
			t.Errorf("%s %q: got %v, want code %s", c.engine, c.in, err, c.code)
		}
	}
}

func TestNormalizeRefusesEngineAndPanelAccounts(t *testing.T) {
	for _, user := range []string{"postgres", "root", "all", "replication", "celikpanel_admin", "pg_monitor", "sameuser"} {
		target := Target{Engine: EnginePostgreSQL, Port: 5432, Rules: []Rule{{Database: "shop", User: user, Address: "203.0.113.7"}}}
		if _, err := target.Normalize(); err == nil {
			t.Errorf("user %q accepted", user)
		}
	}
	target := Target{Engine: EnginePostgreSQL, Port: 5432, Rules: []Rule{{Database: "all", User: "shop", Address: "203.0.113.7"}}}
	if _, err := target.Normalize(); err == nil {
		t.Error("database all accepted")
	}
}

func TestNormalizeLimitsAddressesPerUser(t *testing.T) {
	target := Target{Engine: EngineMySQL, Port: 3306}
	for i := 1; i <= MaxAddressesPerUser; i++ {
		target.Rules = append(target.Rules, Rule{Database: "shop", User: "shop_u", Address: netip.AddrFrom4([4]byte{203, 0, 113, byte(i)}).String()})
	}
	if _, err := target.Normalize(); err != nil {
		t.Fatalf("ten addresses refused: %v", err)
	}
	// The same address for another database does not count twice.
	target.Rules = append(target.Rules, Rule{Database: "blog", User: "shop_u", Address: "203.0.113.1"})
	if _, err := target.Normalize(); err != nil {
		t.Fatalf("a repeated address counted: %v", err)
	}
	target.Rules = append(target.Rules, Rule{Database: "shop", User: "shop_u", Address: "203.0.113.200"})
	if _, err := target.Normalize(); err == nil {
		t.Fatal("an eleventh address accepted")
	}
}

func TestHBALinesAreTLSOnlyAndQuoted(t *testing.T) {
	lines, err := HBALines(Target{Engine: EnginePostgreSQL, Port: 5432, Rules: []Rule{
		{Database: "shop", User: "shop_u", Address: "2001:db8::7"},
		{Database: "shop", User: "shop_u", Address: "203.0.113.9/24"},
		{Database: "shop", User: "shop_u", Address: "203.0.113.0/24"}, // same after masking
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`hostssl "shop" "shop_u" 2001:db8::7/128 scram-sha-256`,
		`hostssl "shop" "shop_u" 203.0.113.0/24 scram-sha-256`,
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lines:\n%s", strings.Join(lines, "\n"))
	}
	if _, err := HBALines(Target{Engine: EngineMySQL, Port: 3306}); err == nil {
		t.Fatal("MariaDB target rendered as pg_hba")
	}
}

const ownerHBA = `# owner's file
local   all             postgres                                peer
host    all             all             127.0.0.1/32            scram-sha-256
`

func TestSpliceHBAAppendsReplacesAndRemoves(t *testing.T) {
	lines := []string{`hostssl "shop" "shop_u" 203.0.113.7/32 scram-sha-256`}
	added, err := SpliceHBA(ownerHBA, lines)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(added, ownerHBA) || !strings.Contains(added, lines[0]) || !strings.HasSuffix(added, "\n") {
		t.Fatalf("appended:\n%s", added)
	}
	current, err := CurrentHBALines(added)
	if err != nil || len(current) != 1 || current[0] != lines[0] {
		t.Fatalf("current = %v, %v", current, err)
	}
	again, err := SpliceHBA(added, lines)
	if err != nil || again != added {
		t.Fatalf("same lines changed the file:\n%s", again)
	}
	two := append(lines, `hostssl "shop" "shop_u" 198.51.100.7/32 scram-sha-256`)
	replaced, err := SpliceHBA(added, two)
	if err != nil || strings.Count(replaced, HBABeginMarker) != 1 || !strings.Contains(replaced, "198.51.100.7") {
		t.Fatalf("replaced:\n%s (%v)", replaced, err)
	}
	removed, err := SpliceHBA(replaced, nil)
	if err != nil || removed != ownerHBA {
		t.Fatalf("removed:\n%q (%v)", removed, err)
	}
	if same, err := SpliceHBA(ownerHBA, nil); err != nil || same != ownerHBA {
		t.Fatal("no block and no lines changed the file")
	}
}

func TestSpliceHBALeavesAHandEditedBlock(t *testing.T) {
	added, _ := SpliceHBA(ownerHBA, []string{`hostssl "shop" "shop_u" 203.0.113.7/32 scram-sha-256`})
	edited := strings.Replace(added, "203.0.113.7/32", "0.0.0.0/0", 1)
	if _, err := SpliceHBA(edited, nil); !errors.Is(err, ErrHBABlockEdited) {
		t.Fatalf("edited block rewritten: %v", err)
	}
	twice := added + added
	if _, err := SpliceHBA(twice, nil); !errors.Is(err, ErrHBABlockEdited) {
		t.Fatalf("two blocks accepted: %v", err)
	}
	orphan := ownerHBA + HBABeginMarker + "\n"
	if _, err := SpliceHBA(orphan, nil); !errors.Is(err, ErrHBABlockEdited) {
		t.Fatalf("marker without pair accepted: %v", err)
	}
}

func TestMariaDBAccountsAndHosts(t *testing.T) {
	accounts, err := MariaDBAccounts(Target{Engine: EngineMySQL, Port: 3306, Rules: []Rule{
		{Database: "shop", User: "shop_u", Address: "203.0.113.7"},
		{Database: "blog", User: "shop_u", Address: "203.0.113.7"},
		{Database: "shop", User: "shop_u", Address: "198.51.100.0/24"},
		{Database: "shop", User: "shop_u", Address: "2001:db8::7"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, a := range accounts {
		got = append(got, a.ID()+":"+strings.Join(a.Databases, ","))
	}
	want := "shop_u@198.51.100.0/255.255.255.0:shop shop_u@2001:db8::7:shop shop_u@203.0.113.7:blog,shop"
	if strings.Join(got, " ") != want {
		t.Fatalf("accounts = %v", got)
	}
}

func TestMariaDBStatementsNeverUseWildcardsAndRequireTLS(t *testing.T) {
	credential := MariaDBCredential{Plugin: "mysql_native_password", Auth: "*0123456789ABCDEF0123456789ABCDEF01234567"}
	create, err := CreateMariaDBAccount("shop_u", "203.0.113.7", credential)
	if err != nil {
		t.Fatal(err)
	}
	if create != "CREATE USER 'shop_u'@'203.0.113.7' IDENTIFIED VIA mysql_native_password USING '*0123456789ABCDEF0123456789ABCDEF01234567' REQUIRE SSL WITH MAX_USER_CONNECTIONS 0 ACCOUNT UNLOCK;" {
		t.Fatalf("create = %s", create)
	}
	for _, host := range []string{"%", "localhost", "127.0.0.1", "example.com", "10.%", "10.0.0.0/255.0.0.0", "10.0.0.0/255.255.0.255", ""} {
		if _, err := CreateMariaDBAccount("shop_u", host, credential); err == nil {
			t.Errorf("host %q accepted", host)
		}
	}
	if _, err := CreateMariaDBAccount("root", "203.0.113.7", credential); err == nil {
		t.Error("root accepted")
	}
	for _, bad := range []MariaDBCredential{{Plugin: "unix_socket", Auth: ""}, {Plugin: "mysql_native_password", Auth: ""}, {Plugin: "mysql_native_password", Auth: "x' OR '1"}} {
		if _, err := CreateMariaDBAccount("shop_u", "203.0.113.7", bad); err == nil {
			t.Errorf("credential %+v accepted", bad.Plugin)
		}
	}
	limited, _ := AlterMariaDBAccount("shop_u", "203.0.113.7", MariaDBCredential{Plugin: "ed25519", Auth: "ZIgUREUg5PVgQ6LskhXmO+eZLS0nC8be6HPjYWR4YJY", MaxUserConnections: 15, Locked: true})
	if !strings.HasSuffix(limited, " REQUIRE SSL WITH MAX_USER_CONNECTIONS 15 ACCOUNT LOCK;") {
		t.Fatalf("limit and lock not copied: %s", limited)
	}
	if _, err := CreateMariaDBAccount("shop_u", "203.0.113.7", MariaDBCredential{Plugin: "ed25519", Auth: "x", MaxUserConnections: -1}); err == nil {
		t.Fatal("a negative limit accepted")
	}
	grant, err := GrantMariaDB("SELECT, INSERT", "shop", "shop_u", "198.51.100.0/255.255.255.0")
	if err != nil || grant != "GRANT SELECT, INSERT ON `shop`.* TO 'shop_u'@'198.51.100.0/255.255.255.0';" {
		t.Fatalf("grant = %s, %v", grant, err)
	}
	if _, err := GrantMariaDB("SUPER", "shop", "shop_u", "203.0.113.7"); err == nil {
		t.Fatal("SUPER copied")
	}
}

func TestParseMariaDBGrantsCopiesOnlyDatabaseGrants(t *testing.T) {
	lines := []string{
		"GRANT USAGE ON *.* TO `shop_u`@`localhost` IDENTIFIED BY PASSWORD '*0123456789ABCDEF0123456789ABCDEF01234567'",
		"GRANT ALL PRIVILEGES ON `shop`.* TO `shop_u`@`localhost`",
		"GRANT SELECT ON `blog`.* TO `shop_u`@`localhost`",
		"GRANT SELECT (id) ON `shop`.`t` TO `shop_u`@`localhost`",
		"GRANT ALL PRIVILEGES ON `other`.* TO `someone`@`localhost`",
	}
	grants, err := ParseMariaDBGrants(lines, "shop_u", "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if grants.Global || len(grants.Databases) != 2 || grants.Databases["shop"] != "ALL PRIVILEGES" || grants.Databases["blog"] != "SELECT" {
		t.Fatalf("grants = %+v", grants)
	}
	global, _ := ParseMariaDBGrants([]string{"GRANT ALL PRIVILEGES ON *.* TO `shop_u`@`localhost`"}, "shop_u", "localhost")
	if !global.Global {
		t.Fatal("a server-wide grant was not noticed")
	}
	option, _ := ParseMariaDBGrants([]string{"GRANT SELECT ON `shop`.* TO `shop_u`@`localhost` WITH GRANT OPTION"}, "shop_u", "localhost")
	if !option.Global {
		t.Fatal("WITH GRANT OPTION was not noticed")
	}
}

func TestMariaDBOptionFileAndOwnership(t *testing.T) {
	plain := MariaDBOptionFile("", "")
	if !strings.Contains(plain, "[mysqld]\nbind-address = *\n") || strings.Contains(plain, "ssl_cert") {
		t.Fatalf("plain = %s", plain)
	}
	tls := MariaDBOptionFile("/etc/ssl/celikpanel-database/mariadb/server.crt", "/etc/ssl/celikpanel-database/mariadb/server.key")
	if !strings.Contains(tls, "ssl_cert = /etc/ssl/celikpanel-database/mariadb/server.crt\n") {
		t.Fatalf("tls = %s", tls)
	}
	if !IsOwnMariaDBOptionFile(plain, "c", "k") || !IsOwnMariaDBOptionFile(tls, "/etc/ssl/celikpanel-database/mariadb/server.crt", "/etc/ssl/celikpanel-database/mariadb/server.key") {
		t.Fatal("own file not recognised")
	}
	if IsOwnMariaDBOptionFile(plain+"max_connections = 5\n", "c", "k") {
		t.Fatal("an edited file was taken as CelikPanel's")
	}
}

func TestNFTFileAllowsOnlyListedAddresses(t *testing.T) {
	v4 := []netip.Prefix{netip.MustParsePrefix("203.0.113.7/32"), netip.MustParsePrefix("198.51.100.0/24")}
	v6 := []netip.Prefix{netip.MustParsePrefix("2001:db8::7/128")}
	text, err := NFTFile([]NFTPort{{Port: 5432, V4: v4, V6: v6}, {Port: 3306}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"table inet celikpanel_dbremote\ndelete table inet celikpanel_dbremote\ntable inet celikpanel_dbremote {\n",
		"type filter hook input priority -10; policy accept;",
		"iif \"lo\" accept",
		"tcp dport 3306 drop\n",
		"tcp dport 5432 ip saddr { 203.0.113.7, 198.51.100.0/24 } accept\n",
		"tcp dport 5432 ip6 saddr { 2001:db8::7 } accept\n",
		"tcp dport 5432 drop\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	if strings.Index(text, "tcp dport 3306") > strings.Index(text, "tcp dport 5432") {
		t.Error("ports not in order")
	}
	if empty, err := NFTFile(nil); err != nil || empty != "" {
		t.Fatal("no ports must render nothing")
	}
	if _, err := NFTFile([]NFTPort{{Port: 5432, V4: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}}}); err == nil {
		t.Fatal("0.0.0.0/0 rendered")
	}
}

func TestNFTUnitRunsOnlyNft(t *testing.T) {
	unit, err := NFTUnit("/usr/sbin/nft")
	if err != nil || !strings.Contains(unit, "ExecStart=/bin/sh /etc/celikpanel/database-remote-guard.sh\n") || !strings.Contains(unit, "Before=network-pre.target") ||
		strings.Contains(unit, "RemainAfterExit") {
		t.Fatalf("unit = %s, %v", unit, err)
	}
	guard, err := NFTGuardScript("/usr/sbin/nft")
	if err != nil || !strings.Contains(guard, "nft=/usr/sbin/nft\n") ||
		!strings.Contains(guard, `if ! "$nft" list chain inet celikpanel_dbremote input 2>/dev/null | grep -q ' drop'; then`) ||
		!strings.Contains(guard, `"$nft" -f /etc/celikpanel/database-remote.nft`) {
		t.Fatalf("guard = %s, %v", guard, err)
	}
	timer := NFTTimer()
	if !strings.Contains(timer, "OnUnitActiveSec=30s\n") || !strings.Contains(timer, "Unit=celikpanel-database-remote-firewall.service\n") {
		t.Fatalf("timer = %s", timer)
	}
	if _, err := NFTUnit("nft; rm -rf /"); err == nil {
		t.Fatal("unsafe program path accepted")
	}
	if _, err := NFTGuardScript("/usr/sbin/nft $(x)"); err == nil {
		t.Fatal("unsafe program path accepted in the guard")
	}
}

func TestMariaDBJail(t *testing.T) {
	jail, err := MariaDBJail(3306, "mariadb.service")
	if err != nil || !strings.Contains(jail, "[celikpanel-mariadb-auth]\nenabled = true\nfilter = celikpanel-mariadb-auth\nport = 3306\n") ||
		!strings.Contains(jail, "backend = systemd\njournalmatch = _SYSTEMD_UNIT=mariadb.service\n") ||
		strings.Contains(jail, "[mysqld-auth]") || strings.Contains(jail, "logpath") {
		t.Fatalf("jail = %s, %v", jail, err)
	}
	if _, err := MariaDBJail(3306, "x;y"); err == nil {
		t.Fatal("bad unit accepted")
	}
	if !strings.Contains(MariaDBFilter(), "failregex = "+MariaDBFailRegex+"\n") {
		t.Fatal("the filter does not carry the failregex")
	}
}

// failRegexForGo reads the failregex the way Fail2ban's Python re does:
// \Z (end of the entry) is Go's \z; <HOST> becomes a plain capture.
func failRegexForGo(multiline bool) *regexp.Regexp {
	text := strings.Replace(strings.Replace(MariaDBFailRegex, `\Z`, `\z`, 1), "<HOST>", `(?P<host>[^'\s]+)`, 1)
	if multiline {
		text = "(?m)" + text
	}
	return regexp.MustCompile(text)
}

// The filter's failregex against MariaDB's refused sign-in entries as
// Fail2ban's systemd backend hands them over (host, program[pid]: message;
// the program is mariadbd on MariaDB 10.5+ as packaged by Debian 12/13,
// Ubuntu 22.04/24.04 and Arch), also without the prefix or the timestamp in
// case a Fail2ban version strips them.
func TestMariaDBFailRegexMatchesRefusedSignIns(t *testing.T) {
	matches := map[string]string{
		"srv1 mariadbd[812]: 2026-11-07 10:00:00 31 [Warning] Access denied for user 'shop_u'@'203.0.113.7' (using password: YES)":  "203.0.113.7",
		"srv2 mariadbd[1203]: 2026-11-07 10:00:01 7 [Warning] Access denied for user 'shop_u'@'198.51.100.20' (using password: NO)": "198.51.100.20",
		"arch mariadbd[455]: 2026-11-07 10:00:02 12 [Warning] Access denied for user 'shop_u'@'2001:db8::7' (using password: YES)":  "2001:db8::7",
		"srv3 mysqld[99]: 2026-11-07 10:00:03 140234 [Warning] Access denied for user 'x'@'203.0.113.9' (using password: YES)":      "203.0.113.9",
		"2026-11-07 10:00:04 55 [Warning] Access denied for user 'shop_u'@'203.0.113.10' (using password: YES)":                     "203.0.113.10",
		"[Warning] Access denied for user 'shop_u'@'203.0.113.11' (using password: YES)":                                            "203.0.113.11",
		"srv1 mariadbd[812]: 2026-11-07 10:00:00 31 [Warning] Access denied for user ''@'203.0.113.12' (using password: NO)":        "203.0.113.12",
		// alpha.129 lab, Fail2ban 1.1 on Debian 13: the timestamp is cut out
		// and the space after it stays; an entry keeps its final newline.
		" 44 [Warning] Access denied for user 'cust1_rmy_user'@'192.0.3.1' (using password: YES)\n":                              "192.0.3.1",
		"host1 mariadbd[79962]: 2026-11-07 19:00:37 44 [Warning] Access denied for user 'u'@'192.0.3.1' (using password: YES)\n": "192.0.3.1",
	}
	for _, multiline := range []bool{false, true} {
		pattern := failRegexForGo(multiline)
		for line, host := range matches {
			got := pattern.FindStringSubmatch(line)
			if got == nil || got[1] != host {
				t.Errorf("multiline %v: not matched or wrong host: %q -> %v", multiline, line, got)
			}
		}
		for _, line := range []string{
			// An authorization refusal after a correct password: not guessing.
			"srv1 mariadbd[812]: 2026-11-07 10:00:00 31 [Warning] Access denied for user 'shop_u'@'203.0.113.7' to database 'other'",
			"srv1 mariadbd[812]: 2026-11-07 10:00:00 31 [Note] Aborted connection 31 to db: 'shop' user: 'shop_u' host: '203.0.113.7'",
			// Not at the start of the entry.
			"srv1 kernel: something Access denied for user 'x'@'203.0.113.7' (using password: YES)",
		} {
			if pattern.MatchString(line) {
				t.Errorf("multiline %v: matched %q", multiline, line)
			}
		}
	}
}

// Re-review LOW 1: the user name is the client's; a name with a newline and a
// forged suffix must never ban an address other than the one MariaDB itself
// appended (198.51.100.5, the attacker's own).
func TestMariaDBFailRegexIgnoresACraftedMultiLineName(t *testing.T) {
	const prefix = "srv1 mariadbd[812]: 2026-11-07 10:00:00 31 [Warning] Access denied for user '"
	const tail = "'@'198.51.100.5' (using password: YES)"
	for _, name := range []string{
		"aaa'@'203.0.113.99' (using password: YES)\nx",
		"aaa'@'203.0.113.99' (using password: YES)\n",
		"a\n2026-11-07 10:00:00 1 [Warning] Access denied for user 'z'@'203.0.113.99' (using password: YES",
		"a\n[Warning] Access denied for user 'z'@'203.0.113.99' (using password: YES)\n[Warning] Access denied for user 'q",
	} {
		entry := prefix + name + tail
		for _, multiline := range []bool{false, true} {
			for _, got := range failRegexForGo(multiline).FindAllStringSubmatch(entry, -1) {
				if got[1] != "198.51.100.5" {
					t.Errorf("multiline %v: %q banned %s", multiline, entry, got[1])
				}
			}
		}
	}
}

func TestStateRoundTripAndStrictness(t *testing.T) {
	state := State{Targets: map[string]TargetState{
		"postgresql:5432": {Engine: EnginePostgreSQL, Port: 5432, Listen: true, Addresses: []string{"203.0.113.7/32"}},
		"mysql:3306":      {Engine: EngineMySQL, Port: 3306}, // empty: dropped
	}}
	data, err := EncodeState(state)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeState(data)
	if err != nil || len(back.Targets) != 1 || !back.Targets["postgresql:5432"].Listen {
		t.Fatalf("back = %+v, %v", back, err)
	}
	if _, err := DecodeState([]byte(`{"version":1,"targets":{},"extra":1}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := DecodeState([]byte(`{"version":1,"targets":{"mysql:1":{"engine":"mysql","port":2}}}`)); err == nil {
		t.Fatal("mismatched key accepted")
	}
	if _, err := DecodeState([]byte(`not json`)); err == nil {
		t.Fatal("damaged record read as empty")
	}
	if empty, err := DecodeState(nil); err != nil || len(empty.Targets) != 0 {
		t.Fatal("absent record is not empty")
	}
}

func TestForeignHBAHostLines(t *testing.T) {
	debian := `# Database administrative login by Unix domain socket
local   all             postgres                                peer
local   all             all                                     peer
host    all             all             127.0.0.1/32            scram-sha-256
host    all             all             ::1/128                 scram-sha-256
local   replication     all                                     peer
host    replication     all             127.0.0.1/32            scram-sha-256
host    replication     all             ::1/128                 scram-sha-256
host    all             all             127.0.0.1 255.255.255.255 md5
host    all             all             localhost               md5
`
	if got := ForeignHBAHostLines(debian); len(got) != 0 {
		t.Fatalf("Debian's stock file has foreign lines: %v", got)
	}
	withBlock, _ := SpliceHBA(debian, []string{`hostssl "shop" "shop_u" 203.0.113.7/32 scram-sha-256`})
	if got := ForeignHBAHostLines(withBlock); len(got) != 0 {
		t.Fatalf("CelikPanel's own block counted: %v", got)
	}
	for _, line := range []string{
		"host all all 10.0.0.0/8 md5",
		"hostnossl all all 0.0.0.0/0 trust",
		"hostssl all all all scram-sha-256",
		"host all all samenet md5",
		"host all all 10.0.0.1 255.0.0.0 md5",
		"host all all 127.0.0.1 0.0.0.0 md5",
		"host all all 127.0.0.0/0 md5",
		"host all all db.example.com md5",
		"include_dir 'hba.d'",
		"host all all ::/0 md5",
	} {
		if got := ForeignHBAHostLines(debian + line + "\n"); len(got) != 1 {
			t.Errorf("%q not recognised: %v", line, got)
		}
	}
}

func TestMariaDBPrivilegeChangeTouchesOnlyWhatDiffers(t *testing.T) {
	host := "203.0.113.7"
	cases := []struct {
		current, want string
		statements    []string
	}{
		{"ALL PRIVILEGES", "ALL PRIVILEGES", nil},
		{"SELECT, INSERT", "INSERT, SELECT", nil},
		{"", "", nil},
		{"", "SELECT", []string{"GRANT SELECT ON `shop`.* TO 'shop_u'@'203.0.113.7';"}},
		{"SELECT", "", []string{"REVOKE ALL PRIVILEGES ON `shop`.* FROM 'shop_u'@'203.0.113.7';"}},
		{"SELECT", "SELECT, INSERT", []string{"GRANT INSERT ON `shop`.* TO 'shop_u'@'203.0.113.7';"}},
		{"SELECT, INSERT", "SELECT", []string{"REVOKE INSERT ON `shop`.* FROM 'shop_u'@'203.0.113.7';"}},
		{"SELECT", "ALL PRIVILEGES", []string{"GRANT ALL PRIVILEGES ON `shop`.* TO 'shop_u'@'203.0.113.7';"}},
		{"ALL PRIVILEGES", "SELECT", []string{"REVOKE ALL PRIVILEGES ON `shop`.* FROM 'shop_u'@'203.0.113.7';", "GRANT SELECT ON `shop`.* TO 'shop_u'@'203.0.113.7';"}},
	}
	for _, c := range cases {
		got, err := MariaDBPrivilegeChange(c.current, c.want, "shop", "shop_u", host)
		if err != nil || strings.Join(got, "|") != strings.Join(c.statements, "|") {
			t.Errorf("%q -> %q = %q, %v", c.current, c.want, got, err)
		}
	}
	if _, err := MariaDBPrivilegeChange("SELECT", "SUPER", "shop", "shop_u", host); err == nil {
		t.Fatal("SUPER accepted")
	}
}
