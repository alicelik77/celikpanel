package main

import (
	"net/http"
)

// GET/PUT /api/v1/rights/setting-limits (alpha.128, docs/SITE-LIMITS.md).
//
// GET answers the limits the caller works within: the administrator the
// server level (with the stored policy rows), a reseller its own field (what
// its plans and Custom sets may hold), a customer the subscription's
// effective limits (?subscription_id= when it has several). PUT, for the
// administrator only, changes the server level of one key:
//
//	{"key": "php.memory_limit_mb", "server_ceiling": 512 | null,
//	 "server_default": null | n, "customer_editable": true}
//
// A lowered server ceiling reaches each site at its next write; nothing is
// rewritten at once (a plan edit previews and re-applies its own sites).
//
// Sunucu düzeyindeki sınırlar; yönetici değiştirir, herkes kendi alanını okur.

type settingPolicyChange struct {
	Key              string `json:"key"`
	ServerCeiling    *int   `json:"server_ceiling"`
	ServerDefault    *int   `json:"server_default"`
	CustomerEditable *bool  `json:"customer_editable"`
}

func (p *Panel) handleSettingLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := currentCaller(r)
	db := p.db.GetDB()
	switch r.Method {
	case http.MethodGet:
		subID, scoped, handled := p.listingSubscription(w, r)
		if handled {
			return
		}
		var limits settingLimits
		var err error
		scope := "administrator"
		switch {
		case scoped && r.URL.Query().Get("subscription_id") != "":
			scope = "subscription"
			limits, err = subscriptionSettingLimits(ctx, db, subID)
		case c.hasAccountRole(roleAdmin):
			limits, err = serverSettingLimits(ctx, db)
			if err == nil {
				limits = limits.settled()
			}
		case c.hasAccountRole(roleReseller):
			scope = "reseller"
			limits, err = resellerSettingLimits(ctx, db, c.ID)
		case scoped:
			scope = "subscription"
			limits, err = subscriptionSettingLimits(ctx, db, subID)
		default:
			writeCodedError(w, http.StatusConflict, errCodeChooseSubscription,
				"choose a subscription (subscription_id) to see its limits", "")
			return
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
		list := make([]settingLimit, 0, len(settingKeyOrder))
		for _, key := range settingKeyOrder {
			list = append(list, limits[key])
		}
		body := map[string]any{"scope": scope, "limits": list}
		if scope == "administrator" {
			policy, err := loadSettingPolicy(ctx, db)
			if err != nil {
				writeServerError(w, err)
				return
			}
			rows := make([]settingPolicyRow, 0, len(settingKeyOrder))
			for _, key := range settingKeyOrder {
				rows = append(rows, policy[key])
			}
			body["policy"] = rows
		}
		writeRightsJSON(w, http.StatusOK, body)
	case http.MethodPut:
		if !c.hasAccountRole(roleAdmin) {
			writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "only the administrator sets the server's limits", "")
			return
		}
		var req settingPolicyChange
		if err := decodeStrictJSON(w, r, &req); err != nil {
			writeClientError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		def, known := settingKeyDefs[req.Key]
		if !known {
			writeCodedError(w, http.StatusBadRequest, errCodeSettingUnknown, "Nothing was changed: unknown setting "+req.Key+".", "")
			return
		}
		inRange := func(v *int) bool { return v == nil || (*v >= def.Min && *v <= def.Max) }
		if !inRange(req.ServerCeiling) || !inRange(req.ServerDefault) {
			writeCodedError(w, http.StatusBadRequest, errCodeSettingUnknown, "Nothing was changed: a value is outside what this setting accepts.", "")
			return
		}
		if req.ServerCeiling != nil && req.ServerDefault != nil {
			if (!def.Floor && *req.ServerDefault > *req.ServerCeiling) || (def.Floor && *req.ServerDefault < *req.ServerCeiling) {
				writeCodedError(w, http.StatusBadRequest, errCodeSettingAboveLimit, "Nothing was changed: the default must be within the server's limit.", "")
				return
			}
		}
		editable := true
		if req.CustomerEditable != nil {
			editable = *req.CustomerEditable
		}
		boolInt := 0
		if editable {
			boolInt = 1
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO setting_policy (key, server_ceiling, server_default, customer_editable, updated_at)
			VALUES (?, ?, ?, ?, datetime('now'))
			ON CONFLICT(key) DO UPDATE SET server_ceiling = excluded.server_ceiling, server_default = excluded.server_default,
			  customer_editable = excluded.customer_editable, updated_at = excluded.updated_at`,
			req.Key, req.ServerCeiling, req.ServerDefault, boolInt); err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "settings.limit:"+req.Key, "settings", 0, map[string]any{
			"key": req.Key, "server_ceiling": req.ServerCeiling, "server_default": req.ServerDefault, "customer_editable": editable,
		})
		limits, err := serverSettingLimits(ctx, db)
		if err != nil {
			writeServerError(w, err)
			return
		}
		writeRightsJSON(w, http.StatusOK, map[string]any{"success": true, "limit": limits.settled()[req.Key]})
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut})
	}
}
