package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"github.com/alicelik/celikpanel/internal/appcatalog"
)

// The administrator's offer of catalog applications (alpha.118, D-042):
// panel_settings "admin_offered_apps", a JSON list of application ids. An
// application is offered only when the administrator chose it (third-party
// software is gated at the offer step); before the administrator ever chose,
// WordPress alone is offered, so what worked before this release keeps
// working. The Unlimited plans take what is offered (P4); other plans and
// Custom sets name "app:<id>" items.
//
// Yöneticinin sunduğu uygulamalar: yalnız seçilen sunulur; hiç seçim
// yapılmamışsa yalnız WordPress sunulur (önceki davranış korunur).

const settingOfferedApps = "admin_offered_apps"

// catalogOfferingIDs are the offering ids of every catalog application: the
// "installed" layer of an application is the catalog this release carries.
func catalogOfferingIDs() []string {
	out := []string{}
	for _, app := range appcatalog.All() {
		if app.Kind == appcatalog.KindPHPSite {
			out = append(out, appcatalog.OfferingID(app.ID))
		}
	}
	return out
}

func (p *Panel) loadOfferedApps(ctx context.Context) (map[string]bool, error) {
	offered := map[string]bool{}
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingOfferedApps).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		for _, id := range appcatalog.DefaultOffered {
			offered[id] = true
		}
		return offered, nil
	}
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		// An unreadable record offers nothing new (fail closed); what runs
		// keeps running.
		return offered, nil
	}
	for _, id := range list {
		if _, ok := appcatalog.Lookup(id); ok {
			offered[id] = true
		}
	}
	return offered, nil
}

func (p *Panel) saveOfferedApps(ctx context.Context, offered map[string]bool) error {
	list := []string{}
	for id, on := range offered {
		if on {
			list = append(list, id)
		}
	}
	sort.Strings(list)
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	_, err = p.db.GetDB().ExecContext(ctx, `INSERT INTO panel_settings(key,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP`, settingOfferedApps, string(raw))
	return err
}

func (p *Panel) offeredAppList(ctx context.Context) []string {
	offered, err := p.loadOfferedApps(ctx)
	out := []string{}
	if err != nil {
		return out
	}
	for id := range offered {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// changeOfferedApp offers or withdraws one application. Nothing installed
// changes: a withdrawn application keeps running where it is (D-038); only
// new installations are refused. The audit names the application.
func (p *Panel) changeOfferedApp(w http.ResponseWriter, r *http.Request, id string, offered bool) {
	app, ok := appcatalog.Lookup(id)
	if !ok {
		writeClientError(w, http.StatusBadRequest, "unknown application")
		return
	}
	current, err := p.loadOfferedApps(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	before := current[id]
	if before != offered {
		current[id] = offered
		if err := p.saveOfferedApps(r.Context(), current); err != nil {
			writeServerError(w, err)
			return
		}
		verb := "offered"
		if !offered {
			verb = "withdrawn"
		}
		installed, _ := p.liveCatalogAppCount(r.Context(), id)
		p.auditDetailed(r, "offering.admin_offer:app:"+id+":"+verb, "panel_settings", 0, map[string]any{
			"item": appcatalog.OfferingID(id), "name": app.Name, "version": app.Version,
			"offered_before": before, "offered": offered,
			// Installed copies keep running: nothing is removed.
			"installed_sites": installed, "sites_changed": 0,
		})
	}
	policy, err := p.loadOfferedVersions(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	hidden, err := p.loadHiddenOfferings(r.Context())
	if err != nil {
		hidden = map[string]bool{}
	}
	_ = json.NewEncoder(w).Encode(offeredVersionsView{Hidden: policy.Hidden, Restricted: false,
		HiddenOfferings: hiddenOfferingList(hidden), OfferedApps: p.offeredAppList(r.Context())})
}

// liveCatalogAppCount counts the sites that hold an application now.
func (p *Panel) liveCatalogAppCount(ctx context.Context, id string) (int, error) {
	var n int
	if id == "wordpress" {
		err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM application_install_operations WHERE app_id = 'wordpress' AND status = 'applied'`).Scan(&n)
		return n, err
	}
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM app_catalog_operations WHERE app_id = ? AND status NOT IN ('failed', 'removed')`, id).Scan(&n)
	return n, err
}
