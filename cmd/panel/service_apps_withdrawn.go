package main

import (
	"net/http"

	"github.com/alicelik/celikpanel/internal/serviceapps"
)

// A withdrawn app (alpha.141; the owner withdrew Open WebUI on 20 November
// 2026): nobody can choose it, offer it, be given it, install it or update
// it, and no list shows it. An install that exists keeps running, and its
// domain keeps the app's card with stop, start, log, keys and removal, so
// whatever a domain holds stays removable (alpha.138) and the panel never
// stops a hosted workload by itself (AGENTS.md).
//
// Geri çekilen uygulama hiçbir listede yoktur; sunulamaz, verilemez, kurulamaz
// ve güncellenemez. Var olan kurulum çalışmaya devam eder ve alan adında
// kaldırılabilir.

// writeServiceAppWithdrawn answers a request to install, update or offer a
// withdrawn app, also one sent by an older page. It reports whether it did.
func writeServiceAppWithdrawn(w http.ResponseWriter, app serviceapps.App) bool {
	if app.Withdrawn == "" {
		return false
	}
	writeServiceAppRefusal(w, http.StatusConflict, serviceapps.ReasonWithdrawn,
		"Nothing was changed: "+app.Name+" was withdrawn from CelikPanel's catalogue on "+app.Withdrawn+
			", so it is no longer offered, installed or updated. An install that already exists keeps running "+
			"and can be removed in its domain's Programs section.",
		map[string]string{"app": app.Name, "date": app.Withdrawn})
	return true
}

// serviceAppsListedFor: the domain screen's list without withdrawn apps,
// except the one the domain holds (its card needs the entry).
func serviceAppsListedFor(apps []serviceAppAppView, installedAppID string) []serviceAppAppView {
	out := make([]serviceAppAppView, 0, len(apps))
	for _, app := range apps {
		if !app.Listed() && app.ID != installedAppID {
			continue
		}
		out = append(out, app)
	}
	return out
}
