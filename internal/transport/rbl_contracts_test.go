package transport

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"
)

// alpha.133: the blocklist answer gained Outcome, Reason and Sublists. The
// Agent RPC is gob, so both update directions are crossed here with the
// exact pre-alpha.133 shape.

type preA133RBLResult struct {
	Zone   string
	Listed bool
	Detail string
}

type preA133CheckRBLResponse struct {
	IP      string
	Results []preA133RBLResult
	Error   string
}

func gobCross(t *testing.T, in, out any) {
	t.Helper()
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(in); err != nil {
		t.Fatal(err)
	}
	if err := gob.NewDecoder(&buf).Decode(out); err != nil {
		t.Fatal(err)
	}
}

// A current Agent answering an older Panel: the older Panel reads Listed,
// which is true only for a documented listing — a refused question arrives
// as not listed, never as listed.
func TestRBLCurrentAgentToOlderPanel(t *testing.T) {
	current := CheckRBLResponse{IP: "203.0.113.7", Results: []RBLResult{
		{Zone: "zen.spamhaus.org", Outcome: RBLNotChecked, Reason: RBLReasonPublicResolver},
		{Zone: "bl.spamcop.net", Listed: true, Detail: "Blocked", Outcome: RBLListed},
		{Zone: "b.barracudacentral.org", Outcome: RBLNotListed},
	}}
	var old preA133CheckRBLResponse
	gobCross(t, current, &old)
	want := preA133CheckRBLResponse{IP: "203.0.113.7", Results: []preA133RBLResult{
		{Zone: "zen.spamhaus.org"},
		{Zone: "bl.spamcop.net", Listed: true, Detail: "Blocked"},
		{Zone: "b.barracudacentral.org"},
	}}
	if !reflect.DeepEqual(old, want) {
		t.Fatalf("older panel read %+v, want %+v", old, want)
	}
}

// An older Agent answering a current Panel: the new fields arrive empty,
// which is how the Panel recognises the older answer (an empty Outcome).
func TestRBLOlderAgentToCurrentPanel(t *testing.T) {
	old := preA133CheckRBLResponse{IP: "203.0.113.7", Results: []preA133RBLResult{
		{Zone: "zen.spamhaus.org", Listed: true, Detail: "open resolver"},
		{Zone: "dnsbl.sorbs.net"},
	}}
	var current CheckRBLResponse
	gobCross(t, old, &current)
	if current.IP != old.IP || len(current.Results) != 2 {
		t.Fatalf("current panel read %+v", current)
	}
	for i, res := range current.Results {
		if res.Outcome != "" || res.Reason != "" || res.Sublists != nil ||
			res.Zone != old.Results[i].Zone || res.Listed != old.Results[i].Listed {
			t.Fatalf("result %d = %+v", i, res)
		}
	}
}
