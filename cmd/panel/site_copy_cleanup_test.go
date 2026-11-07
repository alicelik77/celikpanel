package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.129 lab (Arch): the Agent refused to remove the copy's database
// (its ownership proof did not match) and the panel removed its records
// anyway: the database and its user stayed in MariaDB, unseen. The records
// now go only when the removal is proven.
func TestSiteCopyRemoveDatabaseKeepsRecordsUnlessTheRemovalIsProven(t *testing.T) {
	p, _, _, _ := newSiteFeatureFixture(t)
	db := p.db.GetDB()
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	insert := func(name string) *siteCopyNewDatabase {
		res, err := db.Exec(`INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (1, 1, 1, ?)`, name)
		if err != nil {
			t.Fatal(err)
		}
		dbID, _ := res.LastInsertId()
		res, err = db.Exec(`INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (1, 1, ?, 'sealed')`, name)
		if err != nil {
			t.Fatal(err)
		}
		userID, _ := res.LastInsertId()
		return &siteCopyNewDatabase{DatabaseID: dbID, UserID: userID, Name: name, User: name,
			Operation: "00112233445566778899aabbccddeeff", CleanupKey: strings.Repeat("ab", 32)}
	}
	count := func(name string) int {
		var n int
		if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM databases_v2 WHERE name = ?) + (SELECT COUNT(*) FROM database_users WHERE username = ?)`, name, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	saved := siteCopyAgent
	t.Cleanup(func() { siteCopyAgent = saved })
	for _, tc := range []struct {
		name   string
		answer func(*transport.DeleteDatabaseResponse) error
		gone   bool
	}{
		{"refused", func(r *transport.DeleteDatabaseResponse) error {
			r.Error = "database ownership proof did not match; destructive cleanup was refused"
			return nil
		}, false},
		{"lost", func(*transport.DeleteDatabaseResponse) error { return errors.New("agent connection lost") }, false},
		{"removed", func(r *transport.DeleteDatabaseResponse) error { r.Success = true; return nil }, true},
	} {
		created := insert("tgt_copy_" + tc.name)
		siteCopyAgent = func(_ *Panel, _ context.Context, method string, _, resp any) error {
			if method != "Agent.DeleteDatabase" {
				t.Fatalf("unexpected %s", method)
			}
			return tc.answer(resp.(*transport.DeleteDatabaseResponse))
		}
		removed := p.siteCopyRemoveDatabase(context.Background(), created)
		if removed != tc.gone {
			t.Fatalf("%s: removed = %v", tc.name, removed)
		}
		if left := count(created.Name); (left == 0) != tc.gone {
			t.Fatalf("%s: %d records left, removal proven %v", tc.name, left, tc.gone)
		}
	}
	if note := siteCopyLeftoverNote("tgt_copy_x", "tgt.example"); !strings.Contains(note, "tgt_copy_x") || !strings.Contains(note, "Databases") {
		t.Fatalf("leftover note: %q", note)
	}
}
