package transport

// Service health for the Monitoring page (alpha.127, monitoring stage 2).
// The panel asks for the managed services' units; the Agent answers them with
// ONE batched `systemctl show` (systemd's own cgroup accounting), read only.
//
// İzleme sayfası için hizmet sağlığı: Agent, yönetilen hizmet birimlerini tek
// bir toplu `systemctl show` ile yanıtlar; yalnız okur.

// ServiceHealthMaxUnits bounds one request.
const ServiceHealthMaxUnits = 64

type ServiceHealthRequest struct {
	// Units are unit names without ".service".
	Units []string `json:"units"`
}

// ServiceHealthUnit is one unit as systemd reports it. A value systemd does
// not know (no accounting, never started) is nil, never zero.
type ServiceHealthUnit struct {
	Unit        string  `json:"unit"`
	LoadState   string  `json:"load_state"`
	ActiveState string  `json:"active_state"`
	SubState    string  `json:"sub_state"`
	ActiveSince string  `json:"active_since,omitempty"` // RFC 3339 UTC
	Restarts    *int64  `json:"restarts,omitempty"`
	MemoryBytes *uint64 `json:"memory_bytes,omitempty"`
	CPUNanos    *uint64 `json:"cpu_nanos,omitempty"`
}

type ServiceHealthResponse struct {
	// Batched is false when systemd's answer could not be split per unit;
	// Units is then empty and the reading is unknown.
	Batched bool                `json:"batched"`
	Units   []ServiceHealthUnit `json:"units"`
}
