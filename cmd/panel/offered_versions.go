package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// Versions offered to customers (alpha.108, D-038 §14). The administrator
// decides, per component and per version line, which versions a customer may
// choose for a site: PHP major.minor ("8.4") for a site's PHP, and the Node.js
// major ("22") for a site's application. Nothing is hidden by default, which
// is the behaviour before this release: everything installed is offered.
//
// What it does and does not do:
//   - A customer, reseller or team member choosing a hidden version is
//     refused (VERSION_NOT_OFFERED) and nothing changes; the refusal says the
//     administrator decides and how to ask.
//   - An administrator is never restricted.
//   - A choice already made is never changed: a site on a version hidden
//     later keeps it, keeps running, and may keep saving its own settings with
//     it. A Node.js site may also move to another patch of the line it already
//     runs (a security patch is never refused).
//   - Every change is audited with its details.
//
// The choice lives in the existing panel_settings key/value table (migration
// 017); no schema change.
//
// Müşteriye sunulan sürümler: yönetici, müşterilerin seçebileceği sürümleri
// bileşen ve sürüm hattı başına belirler. Varsayılan: kurulu her şey sunulur.
// Gizlenen sürümü seçen müşteri reddedilir; yönetici kısıtlanmaz; yapılmış
// bir seçim asla değiştirilmez.

const (
	offeredVersionsPath    = "/api/v1/offered-versions"
	settingOfferedVersions = "customer_offered_versions"

	errCodeVersionNotOffered = "VERSION_NOT_OFFERED"
	errCodeNoVersionOffered  = "NO_VERSION_OFFERED"
)

// offeredComponents are the version choices a customer makes, with the name
// a sentence uses.
var offeredComponents = map[string]string{"php": "PHP", "node": "Node.js"}

var (
	offeredPHPLine  = regexp.MustCompile(`^([0-9]{1,2}\.[0-9]{1,2})(\.[0-9]+)?$`)
	offeredNodeLine = regexp.MustCompile(`^v?([0-9]{1,3})(\.[0-9]+){0,2}$`)
)

// offeredVersionLine is the line a version belongs to: PHP "8.4", Node.js
// "22" (the same keys the support table uses). "" for a malformed version.
func offeredVersionLine(component, version string) string {
	version = strings.TrimSpace(version)
	switch component {
	case "php":
		if match := offeredPHPLine.FindStringSubmatch(version); match != nil {
			return match[1]
		}
	case "node":
		if match := offeredNodeLine.FindStringSubmatch(version); match != nil {
			return match[1]
		}
	}
	return ""
}

// offeredVersions is the stored choice: the lines hidden from customers.
type offeredVersions struct {
	Hidden map[string][]string `json:"hidden"`
}

func (o offeredVersions) normalized() offeredVersions {
	out := offeredVersions{Hidden: map[string][]string{}}
	for component := range offeredComponents {
		lines := []string{}
		seen := map[string]bool{}
		for _, line := range o.Hidden[component] {
			if offeredVersionLine(component, line) == line && line != "" && !seen[line] {
				seen[line] = true
				lines = append(lines, line)
			}
		}
		sort.Strings(lines)
		out.Hidden[component] = lines
	}
	return out
}

// offered: whether a version is offered to customers.
func (o offeredVersions) offered(component, version string) bool {
	line := offeredVersionLine(component, version)
	for _, hidden := range o.Hidden[component] {
		if hidden == line {
			return false
		}
	}
	return true
}

// filter keeps the offered versions of a list, in order.
func (o offeredVersions) filter(component string, versions []string) []string {
	out := make([]string, 0, len(versions))
	for _, version := range versions {
		if o.offered(component, version) {
			out = append(out, version)
		}
	}
	return out
}

var errOfferedVersionsUnreadable = errors.New("the versions offered to customers could not be read")

func (p *Panel) loadOfferedVersions(ctx context.Context) (offeredVersions, error) {
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingOfferedVersions).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return offeredVersions{}.normalized(), nil
	}
	if err != nil {
		return offeredVersions{}, err
	}
	var stored offeredVersions
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return offeredVersions{}, errOfferedVersionsUnreadable
	}
	return stored.normalized(), nil
}

func (p *Panel) saveOfferedVersions(ctx context.Context, value offeredVersions) error {
	raw, err := json.Marshal(value.normalized())
	if err != nil {
		return err
	}
	_, err = p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_settings(key,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP`, settingOfferedVersions, string(raw))
	return err
}

func callerIsAdministrator(r *http.Request) bool {
	return currentCaller(r).hasAccountRole(roleAdmin)
}

// versionChoiceKeeps: the requested version is not a new choice — it is the
// version the site already runs, or (Node.js) another patch of that line.
func versionChoiceKeeps(component, current, requested string) bool {
	if strings.TrimSpace(current) == "" {
		return false
	}
	if current == requested {
		return true
	}
	return component == "node" && offeredVersionLine(component, current) != "" &&
		offeredVersionLine(component, current) == offeredVersionLine(component, requested)
}

// refuseVersionNotOffered answers a customer's choice of a version the
// administrator does not offer, and says true; it says false (and writes
// nothing) when the choice may go on. An administrator is never refused, and
// a site keeping what it runs is never refused.
func (p *Panel) refuseVersionNotOffered(w http.ResponseWriter, r *http.Request, component, current, requested string) bool {
	if callerIsAdministrator(r) || versionChoiceKeeps(component, current, requested) {
		return false
	}
	policy, err := p.loadOfferedVersions(r.Context())
	if err != nil {
		writeServerError(w, fmt.Errorf("versions offered to customers: %w", err))
		return true
	}
	if policy.offered(component, requested) {
		return false
	}
	writeVersionNotOffered(w, component, requested)
	return true
}

func writeVersionNotOffered(w http.ResponseWriter, component, version string) {
	name := offeredComponents[component]
	version = boundedAgentDiagnostic(version)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error: "Nothing was changed: " + name + " " + version + " is not offered to customers on this server. " +
			"Choose one of the offered versions, or ask your administrator to offer this one; the administrator decides which versions are offered.",
		Code: errCodeVersionNotOffered,
		Vars: map[string]string{"name": name, "version": version},
	})
}

func writeNoVersionOffered(w http.ResponseWriter, component string) {
	name := offeredComponents[component]
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error: "Nothing was created: no " + name + " version is offered to customers on this server. Ask your administrator to offer one.",
		Code:  errCodeNoVersionOffered,
		Vars:  map[string]string{"name": name},
	})
}

// customerSitePHPVersion is the PHP version a new site gets when a customer
// creates it: the one asked for, or the newest offered one when none was
// asked for. installed is newest first. offered=false refuses the request.
func customerSitePHPVersion(requested string, installed []string, policy offeredVersions) (version string, offered bool) {
	if requested != "" {
		return requested, policy.offered("php", requested)
	}
	candidates := policy.filter("php", installed)
	if len(candidates) == 0 {
		return "", false
	}
	return candidates[0], true
}

type offeredVersionsView struct {
	Hidden map[string][]string `json:"hidden"`
}

type offeredVersionChange struct {
	Component string `json:"component"`
	Version   string `json:"version"`
	Offered   bool   `json:"offered"`
}

// handleOfferedVersions: GET (every signed-in person: a customer's own page
// marks a version that is no longer offered) and PUT (administrator only): one
// line offered or hidden at a time, audited. A PUT changes no site.
func (p *Panel) handleOfferedVersions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		policy, err := p.loadOfferedVersions(r.Context())
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(offeredVersionsView{Hidden: policy.Hidden})
	case http.MethodPut:
		if !callerIsAdministrator(r) {
			writeClientError(w, http.StatusForbidden, "administrator access required")
			return
		}
		var change offeredVersionChange
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&change); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid request")
			return
		}
		name, known := offeredComponents[change.Component]
		line := offeredVersionLine(change.Component, change.Version)
		if !known || line == "" {
			writeClientError(w, http.StatusBadRequest, "component must be php or node, with a version of it")
			return
		}
		policy, err := p.loadOfferedVersions(r.Context())
		replaced := false
		if errors.Is(err, errOfferedVersionsUnreadable) {
			// The administrator's own change replaces a record nobody can
			// read; the audit says so.
			policy, replaced, err = offeredVersions{}.normalized(), true, nil
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
		before := policy.offered(change.Component, line)
		hidden := []string{}
		for _, existing := range policy.Hidden[change.Component] {
			if existing != line {
				hidden = append(hidden, existing)
			}
		}
		if !change.Offered {
			hidden = append(hidden, line)
		}
		policy.Hidden[change.Component] = hidden
		policy = policy.normalized()
		if before != change.Offered || replaced {
			if err := p.saveOfferedVersions(r.Context(), policy); err != nil {
				writeServerError(w, err)
				return
			}
			sites, _ := p.sitesOnVersionLine(r.Context(), change.Component, line)
			verb := "offered"
			if !change.Offered {
				verb = "hidden"
			}
			details := map[string]any{
				"component": change.Component, "name": name, "version": line,
				"offered_before": before, "offered": change.Offered,
				// Sites already on it keep it: nothing is moved.
				"sites_on_version": sites, "sites_changed": 0,
			}
			if replaced {
				details["replaced_unreadable"] = true
			}
			p.auditDetailed(r, "versions.customer_offer:"+change.Component+":"+line+":"+verb, "panel_settings", 0, details)
		}
		_ = json.NewEncoder(w).Encode(offeredVersionsView{Hidden: policy.Hidden})
	default:
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// sitesOnVersionLine counts the sites on a version line (for the audit: they
// keep it).
func (p *Panel) sitesOnVersionLine(ctx context.Context, component, line string) (int, error) {
	var rows *sql.Rows
	var err error
	switch component {
	case "php":
		rows, err = p.db.GetDB().QueryContext(ctx, `SELECT php_version FROM sites WHERE COALESCE(project_type,'php') = 'php' AND php_version IS NOT NULL`)
	case "node":
		rows, err = p.db.GetDB().QueryContext(ctx, `SELECT runtime_version FROM sites WHERE COALESCE(project_type,'php') = 'node' AND runtime_version IS NOT NULL`)
	default:
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version string
		if rows.Scan(&version) == nil && offeredVersionLine(component, version) == line {
			count++
		}
	}
	return count, rows.Err()
}
