package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// errCodeDatabaseAccountRefused: the Agent refused to change a database
// account (alpha.149; cmd/agent database_account_guard.go). Second review of
// PR #370, finding 16: the refusal travels as its code - Reason is the
// Agent's refusal code (transport.DatabaseAccountRefused*), vars.user the
// account it is about - and the screens that can show it (the Databases
// screens) word it in the person's language from their own route-scoped
// copy (web/src/i18n/dataTools, err.DATABASE_ACCOUNT_REFUSED.<reason>). The
// English sentence stays the message for any other reader.
//
// Agent'ın veritabanı hesabı reddi kodu ve adıyla gider; ekranlar kişinin
// dilinde söyler.
const errCodeDatabaseAccountRefused = "DATABASE_ACCOUNT_REFUSED"

// writeDatabaseAccountRefusal answers an Agent's account refusal (409).
func writeDatabaseAccountRefusal(w http.ResponseWriter, code, username string) {
	log.Printf("[409][database] %s: %s (%s)", errCodeDatabaseAccountRefused, code, username)
	var vars map[string]string
	if username != "" {
		vars = map[string]string{"user": username}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: databaseAccountRefusalSentence(code, username), Code: errCodeDatabaseAccountRefused, Reason: code, Vars: vars})
}
