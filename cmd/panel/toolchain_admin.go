package main

import (
	"net/http"

	"github.com/alicelik/celikpanel/internal/core"
)

// The developer toolchains (Go, Rust; alpha.119) are the server
// administrator's own tools: a compiler is never part of what a customer
// receives, because customers' programs are uploaded already built and run in
// a sandbox. Their install and removal ride the ordinary component install
// and uninstall routes (single-flight, audited as service.install:<id> and
// service.install.failed:<id>, with the lost-answer guard), which sit behind
// isAdminOnlyPath. This check is the second, handler-level wall: an
// administrator-only component is refused for any other caller even if a
// future route change dropped the path rule.
//
// Geliştirici araç zincirleri (Go, Rust) sunucu yöneticisinin kendi araçlarıdır;
// müşteri programları derlenmiş yüklenir ve kum havuzunda koşar. Kurulum ve
// kaldırma olağan bileşen yollarından geçer (tek uçuşlu, denetlenir, kayıp-yanıt
// korumalı) ve isAdminOnlyPath arkasındadır. Bu denetim ikinci, işleyici
// düzeyinde duvardır.

// requireAdministratorForComponent writes the refusal and returns false when
// the component is administrator-only and the caller is not an administrator
// account. Any other component passes: its own rules apply.
func requireAdministratorForComponent(w http.ResponseWriter, r *http.Request, componentID string) bool {
	if !core.IsAdminOnlyComponent(componentID) {
		return true
	}
	if caller := currentCaller(r); caller != nil && caller.hasAccountRole(roleAdmin) {
		return true
	}
	writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
	return false
}
