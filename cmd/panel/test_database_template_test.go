package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	paneldb "github.com/alicelik/celikpanel/internal/db"
)

// The panel tests open hundreds of fresh databases; each used to run every
// migration (about two seconds under the race detector). A fresh database is
// now a copy of one migrated once per test process, opened the same way
// (NewSQLiteDB still checks and applies migrations, finding none to apply).
// A path that already exists is never overwritten: tests that prepare an old
// schema at the path still migrate it themselves.
var panelDatabaseTemplate struct {
	once sync.Once
	dir  string
	path string
	err  error
}

func openTemplatePanelDatabase(path string) (*paneldb.SQLiteDB, error) {
	panelDatabaseTemplate.once.Do(func() {
		dir, err := os.MkdirTemp("", "celikpanel-test-database-")
		if err != nil {
			panelDatabaseTemplate.err = err
			return
		}
		panelDatabaseTemplate.dir = dir
		template := filepath.Join(dir, "panel.sqlite")
		database, err := paneldb.NewSQLiteDB(template)
		if err != nil {
			panelDatabaseTemplate.err = err
			return
		}
		if _, err := database.GetDB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			panelDatabaseTemplate.err = err
		}
		database.Close()
		panelDatabaseTemplate.path = template
	})
	if panelDatabaseTemplate.err != nil {
		return nil, fmt.Errorf("test database template: %w", panelDatabaseTemplate.err)
	}
	if _, err := os.Stat(path); err == nil {
		return paneldb.NewSQLiteDB(path)
	}
	source, err := os.Open(panelDatabaseTemplate.path)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return nil, err
	}
	if err := target.Close(); err != nil {
		return nil, err
	}
	return paneldb.NewSQLiteDB(path)
}

func TestMain(m *testing.M) {
	code := m.Run()
	if panelDatabaseTemplate.dir != "" {
		_ = os.RemoveAll(panelDatabaseTemplate.dir)
	}
	os.Exit(code)
}

// A database opened from the template is the same as a freshly migrated one.
func TestPanelTestDatabaseTemplateMatchesAFreshDatabase(t *testing.T) {
	fresh, err := paneldb.NewSQLiteDB(filepath.Join(t.TempDir(), "fresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	copied, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "copied.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(copied.Close)
	schema := func(database *paneldb.SQLiteDB) string {
		rows, err := database.GetDB().Query(`SELECT type||' '||name||' '||COALESCE(sql,'') FROM sqlite_master ORDER BY type,name`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := ""
		for rows.Next() {
			var line string
			rows.Scan(&line)
			out += line + "\n"
		}
		var origin, status string
		database.GetDB().QueryRow(`SELECT origin,status FROM server_setup_state WHERE id=1`).Scan(&origin, &status)
		return out + origin + " " + status
	}
	if schema(fresh) != schema(copied) {
		t.Fatal("the template database differs from a fresh one")
	}
}
