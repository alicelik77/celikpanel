package main

import (
	"context"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
)

// An app that cannot run on this server's operating system (alpha.140):
// Open WebUI on Debian 13, whose Python 3.13 is outside the versions the app
// supports and which no component install changes. The administrator sees it
// in Catalog management with the reason and what would change it; it cannot
// be offered here, so no plan, reseller or account can be given it, and the
// customers' and resellers' screens do not list it. An app a domain already
// holds keeps its card, so it can be managed and removed.
//
// Unknown is not unsupported: when the Agent cannot say, nothing is hidden or
// closed by this rule (the install still asks the Agent and refuses then).
//
// Bu sunucunun işletim sisteminde çalışamayan uygulama burada sunulamaz,
// verilemez ve müşteri ile bayi ekranlarında listelenmez; bilinmeyen durum
// hiçbir şeyi gizlemez.

// serviceAppOSProbeBound bounds the probe on the offer layer's path.
const serviceAppOSProbeBound = 5 * time.Second

// serviceAppCannotRunHere: the availability says no runtime this server can
// install would let the app run (reason os_unsupported).
func serviceAppCannotRunHere(reason string) bool {
	return reason == serviceapps.ReasonOSUnsupported
}

// serviceAppsUnsupportedHere: the apps the Agent reported as unable to run on
// this operating system. Empty when nobody asked yet and the Agent cannot be
// asked now, or when the probe failed within the last minute.
func (p *Panel) serviceAppsUnsupportedHere(ctx context.Context) map[string]bool {
	probeCtx, cancel := context.WithTimeout(ctx, serviceAppOSProbeBound)
	defer cancel()
	out := map[string]bool{}
	availability, err := p.serviceAppAvailabilityRetrying(probeCtx, false)
	if err != nil {
		return out
	}
	for id, a := range availability {
		if !a.Available && serviceAppCannotRunHere(a.Reason) {
			out[id] = true
		}
	}
	return out
}

// withoutServiceAppsUnsupportedHere drops the apps that cannot run here from
// the administrator's offered set. The probe is asked only when something is
// offered.
func (p *Panel) withoutServiceAppsUnsupportedHere(ctx context.Context, offered map[string]bool) map[string]bool {
	if len(offered) == 0 {
		return offered
	}
	unsupported := p.serviceAppsUnsupportedHere(ctx)
	if len(unsupported) == 0 {
		return offered
	}
	out := make(map[string]bool, len(offered))
	for id, open := range offered {
		if open && !unsupported[id] {
			out[id] = true
		}
	}
	return out
}

// serviceAppCandidateOfferingsHere: the service apps a plan or a rights set
// can name on this server, without those that cannot run on its operating
// system (the rights screens list these candidates).
func (p *Panel) serviceAppCandidateOfferingsHere(ctx context.Context) []string {
	candidates := serviceAppCandidateOfferings()
	unsupported := p.serviceAppsUnsupportedHere(ctx)
	if len(unsupported) == 0 {
		return candidates
	}
	out := make([]string, 0, len(candidates))
	for _, offering := range candidates {
		if appID, ok := serviceAppOfferingApp(offering); ok && unsupported[appID] {
			continue
		}
		out = append(out, offering)
	}
	return out
}

// serviceAppsShownToAccount: the domain screen's list for a customer or a
// reseller, without the apps that cannot run on this operating system,
// except the one the domain holds.
func serviceAppsShownToAccount(apps []serviceAppAppView, installedAppID string) []serviceAppAppView {
	out := make([]serviceAppAppView, 0, len(apps))
	for _, app := range apps {
		if !app.Available && serviceAppCannotRunHere(app.Reason) && app.ID != installedAppID {
			continue
		}
		out = append(out, app)
	}
	return out
}
