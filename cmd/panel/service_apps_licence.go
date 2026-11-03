package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"

	"github.com/alicelik/celikpanel/internal/serviceapps"
)

// Apps closed to resellers by their licence (alpha.118, the owner's decision
// of 3 Nov 2026). n8n's Sustainable Use License limits offering n8n itself to
// others as a hosted service. Until the owner has answered that licence
// question, n8n is never available through a reseller: a reseller's rights,
// a reseller plan, a plan a reseller owns and a subscription in a reseller's
// tree cannot name it, and nothing in a reseller's tree installs or updates
// it. The administrator may still offer it to the administrator's own
// customers. Stop, logs and removal are never refused (an install made
// before stays the owner's to stop and remove).
//
// Lisansı nedeniyle bayilere kapalı uygulamalar: n8n bayi haklarına, bayi
// planlarına ve bayinin müşterilerine verilemez; yönetici kendi müşterilerine
// sunabilir. Durdurma, günlükler ve kaldırma hiçbir zaman reddedilmez.

const (
	// serviceAppRefusalLicenceReseller is the refusal reason
	// (err.SERVICE_APP.licence_reseller) and the domain view's refusing
	// layer (svcapps.closed.licence).
	serviceAppRefusalLicenceReseller = "licence_reseller"
	rightsLayerLicence               = "licence"
)

// serviceAppLicenceClosedError is a rights change that names a reseller-
// closed app for a reseller or an account under one.
type serviceAppLicenceClosedError struct {
	App serviceapps.App
}

func (e *serviceAppLicenceClosedError) Error() string {
	return serviceAppLicenceClosedMessage(e.App)
}

func serviceAppLicenceClosedMessage(app serviceapps.App) string {
	return "Nothing was changed: " + app.Name + "’s licence (" + app.Licence + ") restricts offering it to others as a hosted service, " +
		"so it is not available to resellers or to their customers. The administrator decides whether that changes; " +
		"until then there is nothing to do here, and every other app and right works as before."
}

// writeServiceAppLicenceClosed answers the refusal (409, coded).
func writeServiceAppLicenceClosed(w http.ResponseWriter, app serviceapps.App) {
	writeServiceAppRefusal(w, http.StatusConflict, serviceAppRefusalLicenceReseller, serviceAppLicenceClosedMessage(app),
		map[string]string{"app": app.Name, "licence": app.Licence})
}

// licenceClosedItem is the first reseller-closed app a set names explicitly
// (an "all software" flag names none: the install gate holds it).
func licenceClosedItem(s rightsSet) (serviceapps.App, bool) {
	items := make([]string, 0, len(s.Items))
	for item := range s.Items {
		items = append(items, item)
	}
	sort.Strings(items)
	for _, item := range items {
		if !s.Items[item] {
			continue
		}
		if app, closed := serviceapps.ResellerClosedOffering(item); closed {
			return app, true
		}
	}
	return serviceapps.App{}, false
}

// rightsOwnerUnderReseller: the set belongs to a reseller, a reseller plan,
// a plan a reseller owns, or a subscription in a reseller's tree.
func rightsOwnerUnderReseller(ctx context.Context, q rowQueryer, ownerKind string, ownerID int) (bool, error) {
	switch ownerKind {
	case rightsOwnerReseller, rightsOwnerResellerPlan:
		return true, nil
	case rightsOwnerSubscription:
		_, resellerID, err := subscriptionOwnerChain(ctx, q, ownerID)
		if errors.Is(err, errRightsSubscriptionMissing) {
			return false, nil
		}
		return resellerID != 0, err
	case rightsOwnerPlan:
		var owner sql.NullInt64
		err := q.QueryRowContext(ctx, `SELECT owner_id FROM service_plans WHERE id = ?`, ownerID).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !owner.Valid) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		resellerID, err := treeResellerID(ctx, q, int(owner.Int64))
		return resellerID != 0, err
	}
	return false, nil
}

// refuseLicenceClosedRights is saveRightsSet's check.
func refuseLicenceClosedRights(ctx context.Context, q rowQueryer, ownerKind string, ownerID int, s rightsSet) error {
	app, named := licenceClosedItem(s)
	if !named {
		return nil
	}
	under, err := rightsOwnerUnderReseller(ctx, q, ownerKind, ownerID)
	if err != nil {
		return err
	}
	if under {
		return &serviceAppLicenceClosedError{App: app}
	}
	return nil
}

// withoutLicenceClosed drops reseller-closed apps from a candidate list.
func withoutLicenceClosed(candidates []string) []string {
	out := make([]string, 0, len(candidates))
	for _, id := range candidates {
		if _, closed := serviceapps.ResellerClosedOffering(id); !closed {
			out = append(out, id)
		}
	}
	return out
}

// serviceAppClosedToReseller: new use (install, update) of a reseller-closed
// app in a subscription of a reseller's tree is refused, whoever asks.
func serviceAppClosedToReseller(app serviceapps.App, e *effectiveRights) bool {
	return app.ResellerClosed != "" && e != nil && e.ResellerID != 0
}
