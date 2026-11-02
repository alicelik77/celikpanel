package transport

// Security emergency (alpha.117, D-043), Agent side. The Panel names a
// component (and a PHP version); the Agent finds that component's own
// systemd units itself, so the Panel can never name an arbitrary unit.
//
// Güvenlik acil durumu: Panel bileşeni (ve PHP sürümünü) adlandırır; Agent
// bileşenin kendi systemd birimlerini kendisi bulur.

// EmergencyUnitState is one unit as systemd reports it.
type EmergencyUnitState struct {
	Present bool `json:"present"`
	// Active: running (active, activating or reloading).
	Active bool `json:"active"`
	// Masked: the unit cannot be started (masked or masked-runtime).
	Masked bool `json:"masked"`
}

// EmergencyUnitObservation is one unit of the component.
type EmergencyUnitObservation struct {
	Unit  string             `json:"unit"`
	State EmergencyUnitState `json:"state"`
}

// EmergencyUnitsInspectRequest asks for the component's units (read-only).
type EmergencyUnitsInspectRequest struct {
	// Service is php-fpm, mariadb or postgresql.
	Service string `json:"service"`
	// Version is the PHP line for php-fpm; empty otherwise.
	Version string `json:"version,omitempty"`
}

// EmergencyUnitsInspectResponse lists the units in a stable order. An empty
// list with no Error means the component has no unit on this server.
type EmergencyUnitsInspectResponse struct {
	Units []EmergencyUnitObservation `json:"units"`
	Error string                     `json:"error,omitempty"`
}

// SetEmergencyUnitLockRequest locks (stop, then mask) or unlocks (unmask,
// then start only what ran before) the component's units. Before is the
// captured before-image, unit -> state; an unlock is compare-and-set against
// it, so a unit the owner unmasked or started meanwhile is left as it is.
type SetEmergencyUnitLockRequest struct {
	ExpectedBuildCommit string                        `json:"expected_build_commit"`
	Service             string                        `json:"service"`
	Version             string                        `json:"version,omitempty"`
	Lock                bool                          `json:"lock"`
	Before              map[string]EmergencyUnitState `json:"before"`
}

// SetEmergencyUnitLockResponse: Applied when every unit reads as wanted
// afterwards (Changed when something was done); Conflict when a unit was
// changed by someone else (left alone, Error says which).
type SetEmergencyUnitLockResponse struct {
	SuspensionApplyResult
	Units []EmergencyUnitObservation `json:"units"`
}

// SetEmergencyAppLockRequest (alpha.117, D-043) writes or removes the start
// lock of one Node.js application: a systemd drop-in that refuses to start
// the application while its unit runs the withdrawn Node.js version. The
// Agent writes only its own file and removes it only while it is exactly
// what it wrote; a file changed on the server is left as it is.
type SetEmergencyAppLockRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	SiteID              int    `json:"site_id"`
	// NodeVersion is the withdrawn Node.js version the lock refuses.
	NodeVersion string `json:"node_version"`
	Lock        bool   `json:"lock"`
}

// SetEmergencyAppLockResponse: Applied when the lock reads as wanted
// (Changed when the file was written or removed); Conflict when the file
// was changed on the server and kept (Error and Path say which).
type SetEmergencyAppLockResponse struct {
	SuspensionApplyResult
	Path string `json:"path,omitempty"`
}
