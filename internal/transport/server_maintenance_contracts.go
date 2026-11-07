package transport

// Server maintenance (alpha.128, G29): restarting the server is a typed
// Agent call with fixed native work and no shell.
//
//   - Agent.ServerRebootV1 restarts the operating system with
//     `systemctl reboot` (fixed argv) a few seconds after it answers, so the
//     answer reaches the panel first. The Agent first takes the host mutation
//     lock and proves that no privileged change, package manager run or panel
//     update is active; it then keeps that lock until the machine goes down,
//     so nothing new can start in those seconds. Never a shutdown.
//
// Sunucu bakımı: yeniden başlatma sabit argümanlı, kabuksuz bir Agent
// çağrısıdır; asla kapatma değildir.

// Reboot outcomes. Every refusal changed nothing.
const (
	ServerRebootScheduled = "scheduled"
	// ServerRebootAlreadyScheduled: an earlier request of this Agent run
	// already scheduled it; nothing was scheduled twice.
	ServerRebootAlreadyScheduled = "already_scheduled"
	// ServerRebootBusyMutation: a privileged change or the host lock is held.
	ServerRebootBusyMutation = "busy_mutation"
	// ServerRebootBusyPackageManager: apt, dnf or pacman is running.
	ServerRebootBusyPackageManager = "busy_package_manager"
	// ServerRebootBusyUpdate: a CelikPanel update is queued or running.
	ServerRebootBusyUpdate = "busy_update"
	// ServerRebootUnavailable: the Agent could not prove the host idle or
	// cannot run systemctl here; nothing was scheduled.
	ServerRebootUnavailable = "unavailable"
)

type ServerRebootRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	// RequestID is the panel request's identity, for the Agent's journal line.
	RequestID string `json:"request_id"`
}

type ServerRebootResponse struct {
	Outcome      string `json:"outcome"`
	DelaySeconds int    `json:"delay_seconds,omitempty"`
	// Detail is a short reason for a refusal, never a secret.
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}
