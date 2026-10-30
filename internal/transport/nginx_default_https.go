package transport

import "github.com/alicelik/celikpanel/internal/core"

// ReconcileDefaultHTTPSServerRequest asks the Agent to put CelikPanel's
// default HTTPS server in step with the sites once (the owner's "Try again"
// on the Nginx page, alpha.113).
type ReconcileDefaultHTTPSServerRequest struct {
	ExpectedBuildCommit string
}

// ReconcileDefaultHTTPSServerResponse is the attempt's outcome
// (services.DefaultHTTPS*) and the state read after it. Error is set only
// when the attempt did not run.
type ReconcileDefaultHTTPSServerResponse struct {
	Outcome string
	Status  core.NginxDefaultHTTPS
	Error   string
}
