package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// The first-login tour's state (alpha.119, migration 062). Every signed-in
// person reads and writes only their own row: the caller's identity comes
// from the session, never from the request. No row reads as "pending" at
// version 0. The guide itself is public content in the web bundle; this is
// the only thing the server keeps for it, and nothing else depends on it.
//
// İlk giriş turunun durumu: her kişi yalnız kendi satırını okur ve yazar;
// kimlik oturumdan gelir. Satır yoksa "pending" (sürüm 0) okunur.

const (
	guideStatePath         = "/api/v1/me/guide-state"
	guideStateMaxBodyBytes = 1024
	guideTourVersionMax    = 1000
)

type guideStateView struct {
	TourVersion int    `json:"tour_version"`
	TourState   string `json:"tour_state"`
}

func validGuideTourState(state string) bool {
	return state == "pending" || state == "completed" || state == "skipped"
}

func (p *Panel) handleGuideState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	caller := currentCaller(r)
	if caller == nil || caller.ID <= 0 {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		view, err := p.readGuideState(r, caller.ID)
		if err != nil {
			writeServerErrorFor(w, r, err)
			return
		}
		_ = json.NewEncoder(w).Encode(view)
	case http.MethodPut:
		// Someone signed in as this person ("Log in as") must not use up
		// their tour; the screen does not offer it then, and the server
		// keeps the row as it is.
		if caller.ImpersonatorID > 0 {
			writeClientError(w, http.StatusConflict, "the tour state is kept for the person themselves; nothing was changed")
			return
		}
		var body guideStateView
		decoder := json.NewDecoder(io.LimitReader(r.Body, guideStateMaxBodyBytes))
		decoder.DisallowUnknownFields()
		var trailing any
		if err := decoder.Decode(&body); err != nil || !errors.Is(decoder.Decode(&trailing), io.EOF) ||
			body.TourVersion < 0 || body.TourVersion > guideTourVersionMax || !validGuideTourState(body.TourState) {
			writeClientError(w, http.StatusBadRequest, "tour_version must be 0 to 1000 and tour_state pending, completed or skipped; nothing was changed")
			return
		}
		if _, err := p.db.GetDB().ExecContext(r.Context(), `
			INSERT INTO user_guide_state (user_id, tour_version, tour_state, updated_at)
			VALUES (?, ?, ?, datetime('now'))
			ON CONFLICT(user_id) DO UPDATE SET
				tour_version = excluded.tour_version,
				tour_state = excluded.tour_state,
				updated_at = excluded.updated_at`,
			caller.ID, body.TourVersion, body.TourState); err != nil {
			writeServerErrorFor(w, r, err)
			return
		}
		view, err := p.readGuideState(r, caller.ID)
		if err != nil {
			writeServerErrorFor(w, r, err)
			return
		}
		_ = json.NewEncoder(w).Encode(view)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (p *Panel) readGuideState(r *http.Request, userID int) (guideStateView, error) {
	view := guideStateView{TourVersion: 0, TourState: "pending"}
	err := p.db.GetDB().QueryRowContext(r.Context(),
		`SELECT tour_version, tour_state FROM user_guide_state WHERE user_id = ?`, userID).
		Scan(&view.TourVersion, &view.TourState)
	if errors.Is(err, sql.ErrNoRows) {
		return guideStateView{TourVersion: 0, TourState: "pending"}, nil
	}
	return view, err
}
