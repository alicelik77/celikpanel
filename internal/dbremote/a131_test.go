package dbremote

import (
	"strings"
	"testing"
)

// alpha.131: the record of a PostgreSQL cluster follows it to its new port.

func TestStateVersionOneIsMigrated(t *testing.T) {
	v1 := `{"version":1,"targets":{"postgresql:5433":{"engine":"postgresql","port":5433,"listen":true,"auth_delay":true}},"applied":"f97b"}`
	state, err := DecodeState([]byte(v1))
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != StateVersion || state.Applied != "" || !state.Targets["postgresql:5433"].Listen {
		t.Fatalf("migrated: %+v", state)
	}
	data, err := EncodeState(state)
	if err != nil || !strings.Contains(string(data), `"version": 2`) {
		t.Fatalf("encoded: %v %s", err, data)
	}
	if _, err := DecodeState([]byte(`{"version":3,"targets":{}}`)); err == nil {
		t.Fatal("a newer record was read")
	}
	for _, bad := range []string{
		`{"version":2,"targets":{"mysql:3306":{"engine":"mysql","port":3306,"listen":true,"data_dir":"/var/lib/mysql"}}}`,
		`{"version":2,"targets":{"postgresql:5432":{"engine":"postgresql","port":5432,"listen":true,"data_dir":"relative/dir"}}}`,
	} {
		if _, err := DecodeState([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	good := `{"version":2,"targets":{"postgresql:5432":{"engine":"postgresql","port":5432,"listen":true,"data_dir":"/var/lib/postgresql/18/main"}}}`
	if s, err := DecodeState([]byte(good)); err != nil || s.Targets["postgresql:5432"].DataDir != "/var/lib/postgresql/18/main" {
		t.Fatalf("v2: %v %+v", err, s)
	}
}

func TestMovePostgreSQLTarget(t *testing.T) {
	state := State{Applied: "x", Targets: map[string]TargetState{
		"postgresql:5433": {Engine: EnginePostgreSQL, Port: 5433, Listen: true},
		"postgresql:5434": {Engine: EnginePostgreSQL, Port: 5434, TLS: true},
		"mysql:3306":      {Engine: EngineMySQL, Port: 3306, Listen: true},
	}}
	if MovePostgreSQLTarget(&state, 5433, 5434) || MovePostgreSQLTarget(&state, 3306, 5440) || MovePostgreSQLTarget(&state, 5433, 5433) || MovePostgreSQLTarget(&state, 5499, 5440) {
		t.Fatal("a move that must be refused was made")
	}
	if state.Applied != "x" {
		t.Fatal("a refused move dropped the digest")
	}
	if !MovePostgreSQLTarget(&state, 5433, 5432) {
		t.Fatal("not moved")
	}
	if _, old := state.Targets["postgresql:5433"]; old || state.Targets["postgresql:5432"].Port != 5432 || !state.Targets["postgresql:5432"].Listen || state.Applied != "" {
		t.Fatalf("after: %+v", state)
	}
	if _, err := EncodeState(state); err != nil {
		t.Fatal(err)
	}
}

func TestRequestDigestCoversTheServerSwitch(t *testing.T) {
	targets := []Target{{Engine: EnginePostgreSQL, Port: 5432, Rules: []Rule{{Database: "shop", User: "shop_u", Address: "176.88.93.168/32"}}}}
	if RequestDigestOffered(targets, false, false) != RequestDigest(targets, false) {
		t.Fatal("the switch off changed alpha.130's digest")
	}
	if RequestDigestOffered(targets, false, true) == RequestDigest(targets, false) || RequestDigestOffered(nil, false, true) == RequestDigest(nil, false) {
		t.Fatal("the switch does not change the digest")
	}
}
