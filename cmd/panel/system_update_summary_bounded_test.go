package main

import (
	"strings"
	"testing"
)

// upd4 F4: a reviewed updater failure line that is too long or carries a path
// keeps a bounded, Panel-built form (code, state, the preflight step and reason
// class) instead of disappearing. Since alpha.97 a preflight stop also keeps
// its diagnostic: the recovery tool's first line after the step, or the
// checker's line as the detail, sanitized and at most 240 characters
// (Frankfurt's update to alpha.95 stopped and the owner saw no reason because
// this line was dropped). Paths outside the product's own, URLs, controls and
// assignment values never cross the API.
func TestPanelUpdateSummaryKeepsABoundedReviewedForm(t *testing.T) {
	const prefix = "reviewed updater failed: exit status 1: "
	diagnostic := " detail=2026/09/30 23:11:31 Starting CelikPanel Backend... 2026/09/30 23:11:31 WAL-aware service operation idle check failed: service operations are not idle: SQLite sidecar -shm changed after pinning"
	for _, test := range []struct{ name, raw, want string }{
		{"refused preflight",
			prefix + "!! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason=update preflight step=idle_probe class=concurrent_write: panel service operations are not idle; update refused; quiesce was safely aborted, rerun the exact trusted update" + diagnostic,
			"!! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason=update preflight step=idle_probe class=concurrent_write detail=2026/09/30 23:11:31 Starting CelikPanel Backend... 2026/09/30 23:11:31 WAL-aware service operation idle check failed: service operations are not idle: SQLite sidecar -shm changed after pinning"},
		{"refused preflight with a slash in its reason",
			prefix + "!! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason=update preflight step=agent_idle class=check_failed: agent/package mutations are not idle; update refused detail=",
			"!! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason=update preflight step=agent_idle class=check_failed detail="},
		{"recovery runtime preflight",
			prefix + "!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step=panel_database_check: Recovery database check: service operations are not idle: verify panel database path: lstat /proc/self/fd/7/celikpanel.db: no such file detail=",
			"!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step=panel_database_check: Recovery database check: service operations are not idle: verify panel database path: lstat [path]: no such file detail="},
		{"snapshot cause",
			prefix + "!! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=transaction-consistent panel database snapshot failed detail=Create service operation snapshot failed: process 4242 still uses the celikpanel UID",
			"!! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=transaction-consistent panel database snapshot failed detail="},
		{"generic reason with a path",
			prefix + "!! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=panel database is missing: /var/lib/celikpanel/celikpanel.db detail=",
			"!! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason= detail="},
		{"unknown code", prefix + "!! CELIKPANEL_UPDATE_FAILURE code=private_thing state=unchanged reason=x/y detail=", ""},
		{"forged step token", prefix + "!! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason=update preflight step=../../etc class=x detail=/z", "!! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason= detail="},
		{"not an updater line", "dial unix /run/celikpanel/agent.sock: connection refused", ""},
	} {
		got := sanitizePanelUpdateSummary(test.raw)
		if got != test.want {
			t.Fatalf("%s:\n got %q\nwant %q", test.name, got, test.want)
		}
		if got != "" && (strings.ContainsAny(got, "\\\r\n\t") || strings.Contains(got, "://")) {
			t.Fatalf("%s: unsafe bounded form %q", test.name, got)
		}
	}
	// A short, plain summary is unchanged, as before.
	plain := prefix + "!! CELIKPANEL_UPDATE_FAILURE code=update_failed state=unchanged reason=offline panel database migration failed detail="
	if got := sanitizePanelUpdateSummary(plain); got != plain {
		t.Fatalf("plain summary changed: %q", got)
	}
}

// A recovery runtime preflight stop longer than 240 bytes keeps its step and
// the first diagnostic line, sanitized and bounded; nothing of the second line
// and no environment value, URL or foreign path crosses.
func TestPanelUpdateRecoveryPreflightKeepsStepAndDiagnostic(t *testing.T) {
	const marker = "!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step="
	launcher := "compatibility: recovery launcher metadata is not trusted: /usr/libexec/celikpanel/recovery mode 0775, want 0755; parent /usr/libexec owned by uid 1000"
	raw := "reviewed updater failed: exit status 1: " + marker + launcher + " detail="
	got := sanitizePanelUpdateSummary(raw)
	want := marker + "compatibility: recovery launcher metadata is not trusted: /usr/libexec/celikpanel/recovery mode 0775, want 0755; parent [path] owned by uid 1000 detail="
	if got != want {
		t.Fatalf("launcher check:\n got %q\nwant %q", got, want)
	}

	long := marker + "material_support: " + strings.Repeat("checker said the material layout is not supported ", 12) +
		"TOKEN=abc123 https://example.invalid/x /home/alice/.ssh/id_ed25519 " + strings.Repeat("a1", 30) + " detail="
	got = sanitizePanelUpdateSummary("reviewed updater failed: exit status 1: " + long)
	if !strings.HasPrefix(got, marker+"material_support: checker said") || !strings.HasSuffix(got, " detail=") {
		t.Fatalf("step or diagnostic lost: %q", got)
	}
	diagnosticPart := strings.TrimSuffix(strings.TrimPrefix(got, marker+"material_support: "), " detail=")
	if len(diagnosticPart) > 240 || !strings.HasSuffix(diagnosticPart, "...") {
		t.Fatalf("diagnostic is not bounded at 240 characters: %d %q", len(diagnosticPart), diagnosticPart)
	}

	for raw, wantDiagnostic := range map[string]string{
		"TOKEN=abc123 failed":                                 "TOKEN=[hidden] failed",
		"see https://example.invalid/a?b=c for details":       "see [address] for details",
		"open /home/alice/secret: permission denied":          "open [path]: permission denied",
		"open /var/lib/celikpanel-release-state/x.json: EOF":  "open /var/lib/celikpanel-release-state/x.json: EOF",
		"key " + strings.Repeat("Ab3", 20) + " rejected":      "key [hidden] rejected",
		"first line\nsecond line with /etc/shadow":            "first line",
		"ünïcode\x01control\tand\ttabs":                       "ncodecontrol and tabs",
		"forged !! CELIKPANEL_UPDATE_FAILURE code=x detail=y": "forged code=[hidden] detail=[hidden]",
		"step=database_support class=check_failed still ours": "step=database_support class=check_failed still ours",
		"escape /var/lib/celikpanel/../../etc/passwd":         "escape [path]",
		"2026/10/17 12:25:23 check failed":                    "2026/10/17 12:25:23 check failed",
	} {
		if got := panelUpdateDiagnostic(raw); got != wantDiagnostic {
			t.Errorf("panelUpdateDiagnostic(%q) = %q, want %q", raw, got, wantDiagnostic)
		}
	}

	// The busy package manager stop that reached Frankfurt (2026-10-17) is
	// short and plain, and still crosses unchanged.
	busy := "!! CELIKPANEL_UPDATE_FAILURE code=package_manager_busy state=unchanged reason=the host package manager is active detail="
	if got := sanitizePanelUpdateSummary(busy); got != busy {
		t.Fatalf("package manager busy changed: %q", got)
	}
}
