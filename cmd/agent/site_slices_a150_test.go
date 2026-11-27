package main

import "testing"

// alpha.149 lab W2: the move's English answer said "it is failed failed 90
// seconds after the restart". The answer names the state in one plain word;
// the reason's State keeps systemd's words (the page reads auto-restart there).
func TestSiteUnitPlainStateIsOneWord(t *testing.T) {
	cases := []struct {
		active, sub, want string
	}{
		{"failed", "failed", "failed"},
		{"inactive", "dead", "stopped"},
		{"activating", "start", "starting"},
		{"activating", "auto-restart", "restarting"},
		{"deactivating", "stop-sigterm", "stopping"},
		{"active", "running", "running"},
		{"reloading", "reload", "reloading"},
		{"maintenance", "", "in maintenance"},
		{"", "", "in an unknown state"},
	}
	for _, c := range cases {
		props := map[string]string{"ActiveState": c.active, "SubState": c.sub}
		if got := siteUnitPlainState(props); got != c.want {
			t.Errorf("siteUnitPlainState(%q, %q) = %q, want %q", c.active, c.sub, got, c.want)
		}
	}
	props := map[string]string{"ActiveState": "activating", "SubState": "auto-restart"}
	if got := siteUnitStateWords(props); got != "activating auto-restart" {
		t.Errorf("the reason's State changed: %q", got)
	}
}
