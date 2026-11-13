package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 deletion: every right naming the dropped database goes from
// every grantee; an administrator is told whose, everybody else how many;
// rights that could not all be taken keep the database listed.
func TestDatabaseDeleteTakesTheRightsNamingItFromEveryGrantee(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	drops := recordDatabaseDrops(t)
	f.panel.databaseAccessNative = &fakeDatabaseAccessNative{}
	drops.sweepAnswer = func(resp *transport.DroppedDatabaseRightsResponse) error {
		resp.Applied, resp.Grantees = true, []string{"hand@%", "reporting"}
		return nil
	}
	got := f.as(t, "customer", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	var answer map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &answer)
	if answer["rights_removed_count"] != float64(2) || answer["rights_removed_from"] != nil {
		t.Fatalf("customer answer %s", got.Body.String())
	}
	if len(drops.sweeps) != 1 || drops.sweeps[0].Database != "customer_matrix_test_shop" || drops.sweeps[0].ExpectedBuildCommit != strings.TrimSpace(buildCommit) {
		t.Fatalf("sweeps %+v", drops.sweeps)
	}
	got = f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9963", "")
	_ = json.Unmarshal(got.Body.Bytes(), &answer)
	if got.Code != http.StatusOK || answer["rights_removed_count"] != float64(2) || answer["rights_removed_from"] == nil {
		t.Fatalf("admin answer %d %s", got.Code, got.Body.String())
	}
	// PostgreSQL leaves nothing that names a dropped database: no sweep.
	before := len(drops.sweeps)
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9964", ""); got.Code != http.StatusOK || len(drops.sweeps) != before {
		t.Fatalf("PostgreSQL: %d, %d sweeps", got.Code, len(drops.sweeps)-before)
	}
}

func TestDatabaseDeleteStaysListedWhileRightsNamingItAreLeft(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer func(*transport.DroppedDatabaseRightsResponse) error
		reason string
	}{
		{"left", func(resp *transport.DroppedDatabaseRightsResponse) error {
			resp.RightsLeft, resp.Error = true, "1 right(s) naming the database are still loaded"
			return nil
		}, "sweep"},
		{"no answer", func(*transport.DroppedDatabaseRightsResponse) error { return errors.New("rpc timeout") }, "sweep_unknown"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newVisibleDatabasesFixture(t)
			drops := recordDatabaseDrops(t)
			f.panel.databaseAccessNative = &fakeDatabaseAccessNative{}
			drops.sweepAnswer = c.answer
			got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9963", "")
			var body struct {
				Code   string `json:"code"`
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(got.Body.Bytes(), &body)
			if got.Code != http.StatusConflict || body.Code != errCodeDatabaseDeleteUsersLeft || body.Reason != c.reason {
				t.Fatalf("%d %s", got.Code, got.Body.String())
			}
			if n := f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9963`); n != 1 {
				t.Fatal("the record went while rights naming the database were left")
			}
		})
	}
}
