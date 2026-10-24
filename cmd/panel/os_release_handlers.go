package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/supportdata"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Server → Server setup → Operating system (alpha.107, D-038 §17). The page
// names this server's release with its place in the supported list (a data
// file, internal/supportdata/os-releases.json) and how long the release is
// patched. The Panel records the release and what an OS release upgrade
// changes underneath it (vendor repository suites, PostgreSQL clusters,
// PHP-FPM lines, MariaDB's version) on its first read. When the release
// differs from the recorded one, the same read-only comparison says what
// changed, each with its next action, until the administrator marks it
// reviewed. CelikPanel never starts an OS release upgrade.
//
// İşletim sistemi: sürüm, destek listesindeki yeri; sürüm değiştiyse neyin
// değiştiği ve sonraki adım. Yükseltmeyi CelikPanel başlatmaz.

const (
	osReleaseAPIPath         = "/api/v1/server/os-release"
	osReleaseReviewedAPIPath = "/api/v1/server/os-release/reviewed"
	settingOSReleaseBaseline = "os_release_baseline"
	osReleaseReadBound       = 30 * time.Second
)

var readOSReleaseFactsFromAgent = func(p *Panel, ctx context.Context) (transport.OSReleaseFactsResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, osReleaseReadBound)
	defer cancel()
	var response transport.OSReleaseFactsResponse
	err := p.callAgentContext(callCtx, "Agent.OSReleaseFactsV1", &transport.Empty{}, &response)
	return response, err
}

type osReleaseBaseline struct {
	Facts      transport.OSReleaseFactsResponse `json:"facts"`
	RecordedAt string                           `json:"recorded_at"`
	Reviewed   bool                             `json:"reviewed,omitempty"`
}

type osReleaseChange struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject,omitempty"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
}

type osReleaseEntry struct {
	supportdata.OSRelease
	PatchedUntil string `json:"patched_until,omitempty"`
}

func osReleaseEntryOf(release supportdata.OSRelease) osReleaseEntry {
	entry := osReleaseEntry{OSRelease: release}
	if release.DistroKey != "" {
		if table, err := supportdata.Load(); err == nil {
			entry.PatchedUntil = table.Distros[release.DistroKey].PatchedUntil
		}
	}
	return entry
}

func (p *Panel) loadOSReleaseBaseline(ctx context.Context) (osReleaseBaseline, bool, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingOSReleaseBaseline).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return osReleaseBaseline{}, false, nil
	}
	if err != nil {
		return osReleaseBaseline{}, false, err
	}
	var baseline osReleaseBaseline
	if err := json.Unmarshal([]byte(raw), &baseline); err != nil {
		// An unreadable record is replaced by the current one, as a first read.
		return osReleaseBaseline{}, false, nil
	}
	return baseline, true, nil
}

func (p *Panel) saveOSReleaseBaseline(ctx context.Context, baseline osReleaseBaseline) error {
	raw, err := json.Marshal(baseline)
	if err != nil {
		return err
	}
	_, err = p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_settings(key,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP`, settingOSReleaseBaseline, string(raw))
	return err
}

func sameOSRelease(a, b transport.OSReleaseFactsResponse) bool {
	return a.ID == b.ID && a.VersionID == b.VersionID && a.Codename == b.Codename
}

// osReleaseChanges compares the recorded facts with the current ones. A
// vendor repository whose suite does not name the current codename is said
// whether or not the release changed: its packages are for another release.
func osReleaseChanges(before, now transport.OSReleaseFactsResponse, releaseChanged bool) []osReleaseChange {
	changes := []osReleaseChange{}
	repos := make([]string, 0, len(now.RepoSuites))
	for id := range now.RepoSuites {
		repos = append(repos, id)
	}
	sort.Strings(repos)
	for _, id := range repos {
		suite := now.RepoSuites[id]
		if now.Codename != "" && !strings.Contains(suite, now.Codename) {
			changes = append(changes, osReleaseChange{Kind: "repo_suite", Subject: id, From: suite, To: now.Codename})
		}
	}
	if !releaseChanged {
		return changes
	}
	added, gone := setDifference(before.PostgreSQLClusters, now.PostgreSQLClusters)
	for _, cluster := range added {
		changes = append(changes, osReleaseChange{Kind: "pg_cluster_new", Subject: cluster})
	}
	for _, cluster := range gone {
		changes = append(changes, osReleaseChange{Kind: "pg_cluster_gone", Subject: cluster})
	}
	added, gone = setDifference(before.PHPVersions, now.PHPVersions)
	for _, version := range added {
		changes = append(changes, osReleaseChange{Kind: "php_added", Subject: version})
	}
	for _, version := range gone {
		changes = append(changes, osReleaseChange{Kind: "php_removed", Subject: version})
	}
	if before.MariaDBVersion != now.MariaDBVersion && (before.MariaDBVersion != "" || now.MariaDBVersion != "") {
		changes = append(changes, osReleaseChange{Kind: "mariadb_version", From: before.MariaDBVersion, To: now.MariaDBVersion})
	}
	return changes
}

func setDifference(before, now []string) (added, gone []string) {
	in := func(list []string, value string) bool {
		for _, item := range list {
			if item == value {
				return true
			}
		}
		return false
	}
	for _, value := range now {
		if !in(before, value) {
			added = append(added, value)
		}
	}
	for _, value := range before {
		if !in(now, value) {
			gone = append(gone, value)
		}
	}
	return added, gone
}

func (p *Panel) handleOSRelease(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if caller := currentCaller(r); caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "administrator access is required", "")
		return
	}
	reviewed := r.URL.Path == osReleaseReviewedAPIPath
	if (!reviewed && r.Method != http.MethodGet) || (reviewed && r.Method != http.MethodPost) {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	list, err := supportdata.LoadOSReleases()
	if err != nil {
		writeServerError(w, err)
		return
	}
	facts, err := readOSReleaseFactsFromAgent(p, r.Context())
	if err != nil || facts.Error != "" {
		log.Printf("operating-system release: could not be read: %v %s", err, facts.Error)
		if reviewed {
			writeCodedError(w, http.StatusServiceUnavailable, "OS_RELEASE_UNREAD", "the operating-system release could not be read, so nothing was recorded; try again", "")
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": "unknown"})
		return
	}
	baseline, recorded, err := p.loadOSReleaseBaseline(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if reviewed {
		if recorded && !sameOSRelease(baseline.Facts, facts) {
			p.auditDetailed(r, "server.os_release_reviewed:"+osReleaseLabel(baseline.Facts)+"->"+osReleaseLabel(facts), "server", 0,
				map[string]any{"from": osReleaseLabel(baseline.Facts), "to": osReleaseLabel(facts), "changes": len(osReleaseChanges(baseline.Facts, facts, true))})
		}
		baseline = osReleaseBaseline{Facts: facts, RecordedAt: now, Reviewed: true}
		if err := p.saveOSReleaseBaseline(r.Context(), baseline); err != nil {
			writeServerError(w, err)
			return
		}
		recorded = true
	} else if !recorded {
		baseline = osReleaseBaseline{Facts: facts, RecordedAt: now}
		if err := p.saveOSReleaseBaseline(r.Context(), baseline); err != nil {
			writeServerError(w, err)
			return
		}
	}
	changed := !sameOSRelease(baseline.Facts, facts)
	releases := make([]osReleaseEntry, 0, len(list.Releases))
	for _, release := range list.Releases {
		releases = append(releases, osReleaseEntryOf(release))
	}
	current := map[string]any{"id": facts.ID, "version_id": facts.VersionID, "codename": facts.Codename, "pretty_name": facts.PrettyName, "level": supportdata.LevelNotListed}
	if entry, ok := list.Find(facts.ID, facts.VersionID); ok {
		view := osReleaseEntryOf(entry)
		current["level"], current["name"], current["patched_until"] = view.Level, view.Name, view.PatchedUntil
	}
	json.NewEncoder(w).Encode(map[string]any{
		"state":    "read",
		"checked":  list.Checked,
		"current":  current,
		"releases": releases,
		"recorded": map[string]any{"pretty_name": baseline.Facts.PrettyName, "codename": baseline.Facts.Codename, "version_id": baseline.Facts.VersionID, "recorded_at": baseline.RecordedAt},
		"changed":  changed,
		"changes":  osReleaseChanges(baseline.Facts, facts, changed),
	})
}

func osReleaseLabel(facts transport.OSReleaseFactsResponse) string {
	label := facts.ID + " " + facts.VersionID
	if facts.Codename != "" {
		label += " (" + facts.Codename + ")"
	}
	return strings.TrimSpace(label)
}
