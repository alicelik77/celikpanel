package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S1b (20 November 2026): the Agent's close and undo of a PostgreSQL
// database's default rights, without an engine (pg_public_access_engine_test.go
// runs them on one).

func pgPublicRunner(t *testing.T, answer func(script string) ([]byte, error)) *[]string {
	t.Helper()
	var scripts []string
	saved := pgPublicAccessRunner
	pgPublicAccessRunner = func(int) (func(string) ([]byte, error), error) {
		return func(script string) ([]byte, error) {
			scripts = append(scripts, script)
			return answer(script)
		}, nil
	}
	t.Cleanup(func() { pgPublicAccessRunner = saved })
	return &scripts
}

func TestPGPublicCloseIsOneGuardedTransactionThatReadsBack(t *testing.T) {
	script, err := pgPublicCloseScript(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectACL: "", ExpectRoles: []string{"shop_owner", "reader"},
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}, {Role: "writer", Temporary: true}}, NoOtherSessions: true, Recorded: []string{"shop_owner", "reader", "writer"}})
	if err != nil {
		t.Fatal(err)
	}
	order := []string{`\connect "shop"`, "BEGIN;", "'before='", "'had='", "'celikpanel-changed'", "ARRAY['shop_owner','reader']::pg_catalog.text[]",
		"pg_stat_activity", `GRANT CONNECT ON DATABASE "shop" TO "reader";`, `GRANT CONNECT, TEMPORARY ON DATABASE "shop" TO "writer";`,
		`REVOKE CONNECT, TEMPORARY ON DATABASE "shop" FROM PUBLIC;`, "'celikpanel-readback'", "COMMIT;", "'after='"}
	at := 0
	for _, part := range order {
		i := strings.Index(script[at:], part)
		if i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", part, script)
		}
		at += i + len(part)
	}
	if strings.Contains(script, "REVOKE ALL") {
		t.Fatal("the close takes more from PUBLIC than an undo gives back")
	}
	for _, bad := range []transport.PostgreSQLPublicAccessCloseRequest{
		{Name: "shop", ExpectRoles: []string{"x'; DROP ROLE y; --"}},
		{Name: "shop", Grants: []transport.PostgreSQLPublicAccessGrant{{Role: `a"b`}}},
		{Name: `sh"op`},
	} {
		if _, err := pgPublicCloseScript(&bad); err == nil {
			t.Fatalf("a request with a name CelikPanel does not carry was scripted: %+v", bad)
		}
	}
}

func TestPGPublicCloseAnswersWhatItChanged(t *testing.T) {
	scripts := pgPublicRunner(t, func(string) ([]byte, error) {
		return []byte("before=\nhad=writer:CONNECT\nafter={shop_owner=CTc/shop_owner,writer=Tc/shop_owner,reader=c/shop_owner}\n"), nil
	})
	var resp transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectRoles: []string{"shop_owner"},
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}, {Role: "writer", Temporary: true}}, Recorded: []string{"shop_owner"}}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Closed || resp.Change == nil || !reflect.DeepEqual(resp.Change.Added, []string{"reader:CONNECT", "writer:TEMPORARY"}) || len(*scripts) != 1 {
		t.Fatalf("close = %+v %+v", resp, resp.Change)
	}

	pgPublicRunner(t, func(string) ([]byte, error) {
		return []byte("before=\nERROR:  celikpanel-changed\n"), errors.New("exit status 3")
	})
	resp = transport.PostgreSQLPublicAccessCloseResponse{}
	_ = (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectRoles: []string{}, Recorded: []string{}}, &resp)
	if resp.Closed || !resp.Changed {
		t.Fatalf("a database no longer as read = %+v", resp)
	}
}

func TestPGPublicUndoGivesBackExactlyWhatWasRecorded(t *testing.T) {
	script, err := pgPublicUndoScript(&transport.PostgreSQLPublicAccessUndoRequest{Name: "shop", ExpectACL: "{shop_owner=CTc/shop_owner,reader=c/shop_owner}",
		PublicConnect: true, PublicTemporary: true, Added: []string{"reader:CONNECT"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"BEGIN;", "'celikpanel-changed'", `GRANT CONNECT, TEMPORARY ON DATABASE "shop" TO PUBLIC;`, `REVOKE CONNECT ON DATABASE "shop" FROM "reader";`, "'celikpanel-readback'", "COMMIT;"} {
		if !strings.Contains(script, part) {
			t.Fatalf("%q missing in:\n%s", part, script)
		}
	}
	if _, err := pgPublicUndoScript(&transport.PostgreSQLPublicAccessUndoRequest{Name: "shop", Added: []string{"reader:SUPERUSER"}}); err == nil {
		t.Fatal("an undo took away a right a close never gives")
	}
}

func TestPGPublicScanReadsEachDatabase(t *testing.T) {
	pgPublicRunner(t, func(script string) ([]byte, error) {
		switch {
		case strings.Contains(script, "monitor="):
			return []byte("monitor=metrics\n"), nil
		case strings.Contains(script, "'exists='") && strings.Contains(script, "'gone'"):
			return []byte("exists=0\n"), nil
		case strings.Contains(script, "'exists='"):
			return []byte("exists=1\n"), nil
		default:
			return []byte("owner=shop_owner\nacl=\npublic=true,true\nrole=other\nrole=shop_owner\nsession=other\n"), nil
		}
	})
	var resp transport.PostgreSQLPublicAccessScanResponse
	_ = (&Agent{}).PostgreSQLPublicAccessScanV1(&transport.PostgreSQLPublicAccessScanRequest{Databases: []transport.PostgreSQLPublicAccessDatabase{{Name: "shop"}, {Name: "gone"}}}, &resp)
	if resp.Error != "" || !reflect.DeepEqual(resp.Monitoring, []string{"metrics"}) || len(resp.Databases) != 2 {
		t.Fatalf("scan = %+v", resp)
	}
	shop := resp.Databases[0]
	if !shop.Exists || shop.Owner != "shop_owner" || shop.ACL != "" || !shop.PublicConnect || !shop.PublicTemporary ||
		!reflect.DeepEqual(shop.Roles, []string{"other", "shop_owner"}) || !reflect.DeepEqual(shop.Sessions, []string{"other"}) {
		t.Fatalf("shop = %+v", shop)
	}
	if resp.Databases[1].Exists {
		t.Fatalf("gone = %+v", resp.Databases[1])
	}
}
