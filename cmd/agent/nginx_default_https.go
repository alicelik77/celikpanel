package main

import (
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The default HTTPS server's state on the Nginx page (alpha.113).
//
// Its reconciliation (services.ReconcileDefaultHTTPSServer) runs after a
// site's vhost is written and after a webmail address changes; until now
// what it did was only in the Agent's journal, so an owner whose own
// default_server made CelikPanel step back, or whose nginx refused the file,
// saw nothing on the screen. The last attempt of this Agent run is kept here
// and NginxInspect reports the state read from nginx's configuration
// (services.DefaultHTTPSServerStatus): reading never changes anything.
// ReconcileDefaultHTTPSServerV1 is the owner's "Try again": the same
// reconciliation, once per press, idempotent (an unchanged file is not
// written, nginx is not reloaded) and serialized with every nginx change.
//
// Varsayılan HTTPS sunucusunun durumu Nginx sayfasında görünür; okuma hiçbir
// şeyi değiştirmez, "Yeniden dene" aynı uzlaştırmayı bir kez çalıştırır.

var (
	defaultHTTPSAttemptMu sync.Mutex
	defaultHTTPSAttempt   *services.DefaultHTTPSServerResult
	defaultHTTPSAttemptAt time.Time
)

func rememberDefaultHTTPSAttempt(result services.DefaultHTTPSServerResult) {
	defaultHTTPSAttemptMu.Lock()
	defer defaultHTTPSAttemptMu.Unlock()
	defaultHTTPSAttempt, defaultHTTPSAttemptAt = &result, time.Now().UTC()
}

func lastDefaultHTTPSAttempt() (*services.DefaultHTTPSServerResult, time.Time) {
	defaultHTTPSAttemptMu.Lock()
	defer defaultHTTPSAttemptMu.Unlock()
	if defaultHTTPSAttempt == nil {
		return nil, time.Time{}
	}
	copied := *defaultHTTPSAttempt
	return &copied, defaultHTTPSAttemptAt
}

// defaultHTTPSStatusFor reads the state from a `nginx -T` dump.
func (a *Agent) defaultHTTPSStatusFor(dump string, dumpErr error) core.NginxDefaultHTTPS {
	if a == nil || a.nginxGen == nil {
		return core.NginxDefaultHTTPS{State: core.NginxDefaultHTTPSUnknown, Path: services.DefaultHTTPSServerPath()}
	}
	last, at := lastDefaultHTTPSAttempt()
	status := a.nginxGen.DefaultHTTPSServerStatus(dump, dumpErr, last)
	if last != nil {
		status.LastAt = at.Format(time.RFC3339)
	}
	return status
}

// ReconcileDefaultHTTPSServerV1 runs the default HTTPS server's
// reconciliation once and answers the state after it.
func (a *Agent) ReconcileDefaultHTTPSServerV1(req *transport.ReconcileDefaultHTTPSServerRequest, resp *transport.ReconcileDefaultHTTPSServerResponse) error {
	*resp = transport.ReconcileDefaultHTTPSServerResponse{}
	if req == nil {
		resp.Error = "default HTTPS server request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "setting up the default HTTPS server"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if a == nil || a.nginxGen == nil {
		resp.Error = "the nginx generator is not available"
		return nil
	}
	result := reconcileDefaultHTTPSFor(a)
	resp.Outcome = result.Outcome
	dump, dumpErr := runNginxDump()
	resp.Status = a.defaultHTTPSStatusFor(dump, dumpErr)
	return nil
}
