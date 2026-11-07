package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The independent review of alpha.129's remote database access: each item
// has its test here.

// fakePGCluster answers the statements the PostgreSQL side sends and keeps
// the order of what changed.
type fakePGCluster struct {
	ssl, sslTakes bool
	listen        string
	preload       string
	pending       []string
	events        []string
	hbaLines      []string
	hbaText       string
}

func (f *fakePGCluster) query(_ int, sql string) (string, error) {
	switch {
	case strings.Contains(sql, "current_setting('hba_file')"):
		ssl := "off"
		if f.ssl {
			ssl = "on"
		}
		return strings.Join([]string{f.listen, ssl, "/etc/postgresql/17/main/pg_hba.conf", f.preload, "/nonexistent",
			strings.Join(f.pending, ","), "", "<none>", "on"}, "\x1f"), nil
	case strings.Contains(sql, "FROM pg_authid"):
		return "shop_u\x1ffalse\x1ftrue\x1ftrue\ndb\x1fshop\n", nil
	case strings.Contains(sql, "ALTER SYSTEM"):
		f.events = append(f.events, "alter")
		if strings.Contains(sql, "ssl = 'on'") && f.sslTakes {
			f.ssl = true
		}
		if strings.Contains(sql, "listen_addresses = '*'") {
			f.pending = append(f.pending, "listen_addresses")
		}
	}
	return "", nil
}

func withFakePG(t *testing.T, f *fakePGCluster) {
	t.Helper()
	restoreQuery, restoreWrite, restoreWait, restoreRoot, restoreRead := databaseRemotePGQuery, databaseRemoteWriteHBA, databaseRemotePGReloadWait, databaseRemoteCertRoot, databaseRemoteReadHBA
	databaseRemotePGQuery = f.query
	databaseRemoteWriteHBA = func(_ string, lines []string) error {
		f.events = append(f.events, "hba")
		f.hbaLines = append([]string(nil), lines...)
		return nil
	}
	databaseRemoteReadHBA = func(string) ([]byte, error) { return []byte(f.hbaText), nil }
	databaseRemotePGReloadWait = 0
	root := t.TempDir()
	databaseRemoteCertRoot = root
	if err := os.MkdirAll(filepath.Join(root, "postgresql"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"server.crt", "server.key"} {
		if err := os.WriteFile(filepath.Join(root, "postgresql", name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		databaseRemotePGQuery, databaseRemoteWriteHBA, databaseRemotePGReloadWait, databaseRemoteCertRoot, databaseRemoteReadHBA = restoreQuery, restoreWrite, restoreWait, restoreRoot, restoreRead
	})
}

func shopTarget() dbremote.Target {
	return dbremote.Target{Engine: dbremote.EnginePostgreSQL, Port: 5432, Rules: []dbremote.Rule{{Database: "shop", User: "shop_u", Address: "203.0.113.7/32"}}}
}

func problemCodes(result transport.DatabaseRemoteTargetResult) map[string]bool {
	codes := map[string]bool{}
	for _, p := range result.Problems {
		codes[p.Code] = true
	}
	return codes
}

// L4: a cluster with ssl = off gets TLS before any hostssl line is written.
func TestPGRemoteTurnsTLSOnBeforeWritingTheBlock(t *testing.T) {
	f := &fakePGCluster{listen: "localhost", sslTakes: true, hbaText: "local all postgres peer\n"}
	withFakePG(t, f)
	var ts dbremote.TargetState
	(pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true}, &ts, "db.example.com")
	if strings.Join(f.events, ",") != "alter,hba" || !f.ssl {
		t.Fatalf("events = %v, ssl = %v", f.events, f.ssl)
	}
}

func TestPGRemoteWritesNoLineWhileTLSIsOff(t *testing.T) {
	f := &fakePGCluster{listen: "localhost", sslTakes: false, hbaText: "local all postgres peer\n"}
	withFakePG(t, f)
	var ts dbremote.TargetState
	result := (pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true}, &ts, "db.example.com")
	if len(f.hbaLines) != 0 || len(result.Active) != 0 || !problemCodes(result)[transport.DatabaseRemoteTLSMissing] || ts.TLS {
		t.Fatalf("lines %v, active %v, problems %+v, tls %v", f.hbaLines, result.Active, result.Problems, ts.TLS)
	}
}

// M2: an owner's network line in pg_hba.conf means nothing is opened, and
// what CelikPanel opened before is closed.
func TestPGRemoteOpensNothingBehindAnOwnersNetworkLine(t *testing.T) {
	f := &fakePGCluster{listen: "localhost", ssl: true, sslTakes: true, hbaText: "local all postgres peer\nhost all all 10.0.0.0/8 md5\n"}
	withFakePG(t, f)
	var ts dbremote.TargetState
	result := (pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true}, &ts, "db.example.com")
	if len(f.hbaLines) != 0 || len(result.Active) != 0 || ts.Listen || !problemCodes(result)[transport.DatabaseRemoteOwnerHBA] {
		t.Fatalf("lines %v, active %v, listen %v, problems %+v", f.hbaLines, result.Active, ts.Listen, result.Problems)
	}
	for _, e := range f.events {
		if e == "alter" {
			t.Fatal("a setting was changed behind an owner's network line")
		}
	}
	// Opened before the owner added the line: the listen address goes back.
	f.events, f.listen = nil, "*"
	ts = dbremote.TargetState{Engine: "postgresql", Port: 5432, Listen: true}
	(pgRemoteInstance{}).apply(shopTarget(), dbRemoteFacts{running: true, listenRemote: true}, &ts, "db.example.com")
	if ts.Listen || !ts.ListenPending || len(f.hbaLines) != 0 {
		t.Fatalf("not closed: %+v, lines %v", ts, f.hbaLines)
	}
}
