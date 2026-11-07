package main

import (
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 lab: a plan lowered from 384 to 256 MB replaced the site's own
// value and a raised plan did not give it back. The site's own count is the
// one kept above the limit, and a raised or removed PHP ceiling writes the
// plan's sites again.
func TestSitePHPOwnProcessesIsTheKeptCount(t *testing.T) {
	if got := sitePHPOwnProcesses(transport.SitePHPSettingsState{Processes: 4, Own: transport.SitePHPSettings{Processes: 6}}); got != 6 {
		t.Fatalf("kept count: %d", got)
	}
	if got := sitePHPOwnProcesses(transport.SitePHPSettingsState{Processes: 5}); got != 5 {
		t.Fatalf("pool count: %d", got)
	}
}

func TestPHPCeilingsChangedSeesRaisedAndRemoved(t *testing.T) {
	before := map[string]int{"php.memory_limit_mb": 256, "db.max_user_connections": 3}
	cases := []struct {
		after map[string]int
		want  bool
	}{
		{map[string]int{"php.memory_limit_mb": 512, "db.max_user_connections": 3}, true},
		{map[string]int{"db.max_user_connections": 3}, true},
		{map[string]int{"php.memory_limit_mb": 256, "db.max_user_connections": 5}, false},
		{map[string]int{"php.memory_limit_mb": 256, "db.max_user_connections": 3, "php.max_children": 4}, true},
	}
	for i, c := range cases {
		if got := phpCeilingsChanged(before, c.after); got != c.want {
			t.Errorf("case %d: %v, want %v", i, got, c.want)
		}
	}
}
