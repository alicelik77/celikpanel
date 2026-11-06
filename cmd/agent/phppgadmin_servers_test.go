package main

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
	"testing"
)

// The shape of Debian 13 / Ubuntu 24.04's phppgadmin 7.14.7 config.inc.php
// around its server list.
const debianPHPPgAdminConfig = `<?php
	// Display name for the server on the login screen
	$conf['servers'][0]['desc'] = 'PostgreSQL';

	// Hostname or IP address for server.  Use '' for UNIX domain socket.
	$conf['servers'][0]['host'] = 'localhost';

	// Database port on server (5432 is the PostgreSQL default)
	$conf['servers'][0]['port'] = 5432;

	$conf['servers'][0]['sslmode'] = 'allow';
	$conf['servers'][0]['defaultdb'] = 'template1';
	$conf['servers'][0]['pg_dump_path'] = '/usr/bin/pg_dump';
	$conf['servers'][0]['pg_dumpall_path'] = '/usr/bin/pg_dumpall';

	// Example for a second server (PostgreSQL for Windows)
	//$conf['servers'][1]['desc'] = 'Test Server';
	//$conf['servers'][1]['port'] = 5432;

	$conf['default_lang'] = 'auto';
?>
`

type fakePHPPgAdmin struct {
	file   string
	record string
	writes int
}

func useFakePHPPgAdmin(t *testing.T, content string, packaged bool) *fakePHPPgAdmin {
	t.Helper()
	fake := &fakePHPPgAdmin{file: content}
	sum := md5.Sum([]byte(debianPHPPgAdminConfig))
	status := "Package: phppgadmin\nStatus: install ok installed\nConffiles:\n /etc/phppgadmin/apache.conf 0123\n /etc/phppgadmin/config.inc.php " + hex.EncodeToString(sum[:]) + "\nDescription: x\n"
	if !packaged {
		status = ""
	}
	previous := phpPgAdminHost
	phpPgAdminHost.read = func(string) ([]byte, error) { return []byte(fake.file), nil }
	phpPgAdminHost.write = func(_ string, data []byte) error { fake.file = string(data); fake.writes++; return nil }
	phpPgAdminHost.dpkgStatus = func() string { return status }
	phpPgAdminHost.readRecord = func() string { return fake.record }
	phpPgAdminHost.writeRecord = func(sha string) error { fake.record = sha; return nil }
	t.Cleanup(func() { phpPgAdminHost = previous })
	return fake
}

func TestPHPPgAdminNamesTheRunningClusterNotTheRemovedOnesPort(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	fake := useFakePHPPgAdmin(t, debianPHPPgAdminConfig, true)
	if state := phpPgAdminServersState(); state.State != "differs" || state.Owner != "package" || state.Configured[0].Port != 5432 || state.Configured[0].Online {
		t.Fatalf("before = %+v", state)
	}
	if warning := alignPHPPgAdminServers(); warning != "" || fake.writes != 1 {
		t.Fatalf("warning=%q writes=%d", warning, fake.writes)
	}
	for _, want := range []string{"$conf['servers'][0]['port'] = 5433;", "$conf['servers'][0]['desc'] = 'PostgreSQL 18 (port 5433)';",
		"$conf['servers'][0]['host'] = 'localhost';", "//$conf['servers'][1]['desc'] = 'Test Server';", "$conf['default_lang'] = 'auto';"} {
		if !strings.Contains(fake.file, want) {
			t.Fatalf("missing %q in\n%s", want, fake.file)
		}
	}
	if state := phpPgAdminServersState(); state.State != "aligned" || state.Owner != "panel" {
		t.Fatalf("after = %+v", state)
	}
	if alignPHPPgAdminServers(); fake.writes != 1 {
		t.Fatal("an aligned file was written again")
	}
}

func TestPHPPgAdminNamesEveryRunningClusterAndStaysStable(t *testing.T) {
	usePGStatementClusters(t, "17 main 5432 online postgres /var/lib/postgresql/17/main log\n18 main 5433 online postgres /var/lib/postgresql/18/main log\n")
	fake := useFakePHPPgAdmin(t, debianPHPPgAdminConfig, true)
	alignPHPPgAdminServers()
	servers := parsePHPPgAdminServers(fake.file)
	if len(servers) != 2 || servers[0].Port != 5432 || servers[0].Desc != "PostgreSQL 17 (port 5432)" || servers[1].Port != 5433 || servers[1].Desc != "PostgreSQL 18 (port 5433)" {
		t.Fatalf("servers = %+v\n%s", servers, fake.file)
	}
	if !strings.Contains(fake.file, "$conf['servers'][1]['pg_dump_path'] = '/usr/bin/pg_dump';") {
		t.Fatalf("server 1 is not a copy of server 0:\n%s", fake.file)
	}
	written := fake.file
	// PostgreSQL 17 removed later: the panel's own file follows again.
	usePGStatementClusters(t, "17 main 5432 down,binaries_missing postgres /var/lib/postgresql/17/main log\n18 main 5433 online postgres /var/lib/postgresql/18/main log\n")
	if again, err := phpPgAdminWithServers(written, phpPgAdminTargets(parsePGLsClusters("18 main 5433 online postgres d log\n17 main 5432 online postgres d log\n"))); err != nil || again != written {
		t.Fatalf("not stable: %v\n%s", err, again)
	}
	alignPHPPgAdminServers()
	if servers := parsePHPPgAdminServers(fake.file); len(servers) != 1 || servers[0].Port != 5433 || strings.Contains(fake.file, "CelikPanel: phpPgAdmin server 1") {
		t.Fatalf("after 17 went = %+v\n%s", servers, fake.file)
	}
}

// A file the owner edited is never overwritten; the warning names both.
func TestPHPPgAdminEditedByTheOwnerIsNotOverwritten(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	edited := strings.Replace(debianPHPPgAdminConfig, "'localhost'", "'db.internal'", 1)
	fake := useFakePHPPgAdmin(t, edited, true)
	warning := alignPHPPgAdminServers()
	if fake.writes != 0 || !strings.Contains(warning, "edited on this server") || !strings.Contains(warning, "5432") || !strings.Contains(warning, "5433") {
		t.Fatalf("writes=%d warning=%q", fake.writes, warning)
	}
	if state := phpPgAdminServersState(); state.Owner != "owner" || state.State != "differs" {
		t.Fatalf("state = %+v", state)
	}
	// What CelikPanel wrote, edited afterwards, is the owner's too.
	fake = useFakePHPPgAdmin(t, debianPHPPgAdminConfig, true)
	alignPHPPgAdminServers()
	fake.file = strings.Replace(fake.file, "'allow'", "'require'", 1)
	usePGStatementClusters(t, "18 main 5434 online postgres /var/lib/postgresql/18/main log\n")
	if alignPHPPgAdminServers(); fake.writes != 1 {
		t.Fatal("an edit after CelikPanel's write was overwritten")
	}
	// No dpkg record and no panel record: unknown origin is the owner's.
	fake = useFakePHPPgAdmin(t, debianPHPPgAdminConfig, false)
	if alignPHPPgAdminServers(); fake.writes != 0 {
		t.Fatal("a file of unknown origin was written")
	}
}
