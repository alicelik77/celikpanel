package main

import (
	"net/http"
	"net/url"
)

// adminInstallAction is a refusal's one button when a missing component is
// the reason (alpha.127; owner on Boston, 6 Nov 2026: the viewer was the
// administrator, yet the screen only said "the administrator installs it from
// Components"). For the administrator it opens that component's own install
// window on Components, where what comes with it is reviewed and nothing
// starts before it is confirmed. Anyone else gets no button: Components is the
// administrator's page, and the refusal's sentence says who acts.
//
// Eksik bileşen nedeniyle verilen reddin tek düğmesi: yöneticiye bileşenin
// kendi kurulum penceresi; başkasına düğme yok, cümle kimin yapacağını söyler.
func adminInstallAction(r *http.Request, component string) string {
	if r == nil || !callerIsAdministrator(r) {
		return ""
	}
	return "/components?component=" + url.QueryEscape(component) + "&install="
}
