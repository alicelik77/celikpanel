package services

import (
	"encoding/json"
	"os"
	"testing"
)

// The panel's and the Agent's name rule reads the same cases as the browser
// half (web/tests/database-name-limits.test.mjs).
type databaseNameCases struct {
	Lengths []struct {
		Note        string   `json:"note"`
		Engine      string   `json:"engine"`
		Database    string   `json:"database"`
		Users       []string `json:"users"`
		By          int      `json:"by"`
		Underscores bool     `json:"underscores"`
	} `json:"lengths"`
	// Characters: the part a person chooses, put after a generated prefix
	// (which begins with a letter or `_`): valid when the whole name is one
	// the validator accepts.
	Characters []struct {
		Name  string `json:"name"`
		Valid bool   `json:"valid"`
	} `json:"characters"`
}

func readDatabaseNameCases(t *testing.T) databaseNameCases {
	t.Helper()
	raw, err := os.ReadFile("testdata/database-name-limits.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases databaseNameCases
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases.Lengths) < 10 || len(cases.Characters) < 8 {
		t.Fatalf("too few cases: %d lengths, %d characters", len(cases.Lengths), len(cases.Characters))
	}
	return cases
}

func TestDatabaseNamesOverSharedCases(t *testing.T) {
	for _, c := range readDatabaseNameCases(t).Lengths {
		got := DatabaseNamesOver(c.Engine, c.Database, c.Users...)
		if got.By != c.By || got.Underscores != c.Underscores {
			t.Errorf("%s: got %+v, want by %d underscores %v", c.Note, got, c.By, c.Underscores)
		}
		// A name the rule lets through is one the validator every driver and
		// the Agent call accepts, on both engines.
		if got.By == 0 {
			for _, name := range append([]string{c.Database}, c.Users...) {
				if name != "" && ValidateSQLIdentifier(name) != nil {
					t.Errorf("%s: %q fits the rule but not the validator", c.Note, name)
				}
			}
		}
	}
}

func TestDatabaseNameCharactersSharedCases(t *testing.T) {
	for _, c := range readDatabaseNameCases(t).Characters {
		if valid := ValidateSQLIdentifier("p_"+c.Name) == nil; valid != c.Valid {
			t.Errorf("%q: valid %v, want %v", c.Name, valid, c.Valid)
		}
	}
}
