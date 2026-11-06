package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// alpha.127: a refusal caused by a missing component gives the administrator
// that component's own install window and gives everyone else no button.
func TestAdminInstallActionIsTheAdministratorsOnly(t *testing.T) {
	as := func(caller *Caller) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/domains", nil)
		if caller == nil {
			return r
		}
		return r.WithContext(context.WithValue(r.Context(), callerKey, caller))
	}
	if got := adminInstallAction(as(&Caller{ID: 1, Role: roleAdmin}), "php-fpm"); got != "/components?component=php-fpm&install=" {
		t.Fatalf("administrator action = %q", got)
	}
	for _, caller := range []*Caller{nil, {ID: 2, Role: roleReseller}, {ID: 3, Role: roleCustomer}} {
		if got := adminInstallAction(as(caller), "mariadb"); got != "" {
			t.Fatalf("caller %+v got an action %q", caller, got)
		}
	}
	if got := adminInstallAction(nil, "nginx"); got != "" {
		t.Fatalf("no request: %q", got)
	}
}
