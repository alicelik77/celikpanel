package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// S4 R1 fix round: only the administrator keeps a crontab line not made in
// CelikPanel; a customer (or a reseller) keeps only Delete, and nothing is
// asked of the Agent for them.
func TestKeepingACrontabLineIsTheAdministrators(t *testing.T) {
	p := &Panel{}
	for _, role := range []string{roleCustomer, roleReseller} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/domains/34/cron?keep=0000abcd&version=ct1-test", nil)
		r = r.WithContext(context.WithValue(r.Context(), callerKey, &Caller{ID: 5, Role: role}))
		w := httptest.NewRecorder()
		p.handleKeepCronLine(w, r, testCronTenant)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: status %d, want 403", role, w.Code)
		}
	}
}
