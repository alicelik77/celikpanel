package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/alicelik/celikpanel/internal/passwordpolicy"
)

// The password level (alpha.128, gap G21): the administrator may raise the
// owner's rule to Strong (10 characters and a digit) or Very strong (14 and a
// digit), never lower it. It applies when a password is set or changed, never
// to existing passwords. Stored in panel_settings; the policy package holds
// the level in force for every check.
//
// Parola düzeyi: yönetici sahibin kuralını yalnız sıkılaştırabilir; yeni ve
// değiştirilen parolalara uygulanır.

const settingPasswordLevel = "password_level"

// loadPasswordLevel puts the stored level in force (the owner's rule when
// none is stored or it cannot be read).
func (p *Panel) loadPasswordLevel(ctx context.Context) {
	var level string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingPasswordLevel).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("password level: %v (the owner's rule applies)", err)
		return
	}
	if err := passwordpolicy.SetLevel(level); err != nil {
		log.Printf("password level: %v (the owner's rule applies)", err)
	}
}

func (p *Panel) handlePasswordLevel(w http.ResponseWriter, r *http.Request) {
	c := currentCaller(r)
	switch r.Method {
	case http.MethodGet:
	case http.MethodPut:
		if !c.hasAccountRole(roleAdmin) {
			writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "only the administrator sets the password level", "")
			return
		}
		var req struct {
			Level string `json:"level"`
		}
		if err := decodeStrictJSON(w, r, &req); err != nil || !passwordpolicy.ValidLevel(req.Level) {
			writeClientError(w, http.StatusBadRequest, "level must be owner, strong or very_strong")
			return
		}
		if _, err := p.db.GetDB().ExecContext(r.Context(), `INSERT INTO panel_settings(key,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP`, settingPasswordLevel, req.Level); err != nil {
			writeServerError(w, err)
			return
		}
		_ = passwordpolicy.SetLevel(req.Level)
		p.auditDetailed(r, "settings.password_level:"+req.Level, "panel_settings", 0, map[string]any{"level": req.Level})
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut})
		return
	}
	level := passwordpolicy.Level()
	writeRightsJSON(w, http.StatusOK, map[string]any{
		"level": level, "min_chars": passwordpolicy.LevelMinChars(level),
		"levels": []string{passwordpolicy.LevelOwner, passwordpolicy.LevelStrong, passwordpolicy.LevelVeryStrong},
	})
}
