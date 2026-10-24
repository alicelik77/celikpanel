package transport

// Patched is not restarted (alpha.107, D-038 §10). Agent.RestartNeedsV1
// reads, without changing anything and bounded, which services still run
// code an update replaced (the operating system's own tool, needrestart,
// where it is installed; otherwise the processes' memory maps that name a
// deleted library) and whether the server waits for a reboot (Debian and
// Ubuntu's /var/run/reboot-required, a running kernel whose modules are
// gone, needrestart's kernel answer). The owner chooses when; nothing is
// restarted by itself.
// Yamalanmış, yeniden başlatılmış demek değildir: salt okunur.

type RestartNeedsService struct {
	Unit string `json:"unit"`
	// Component is the catalogue component whose unit it is, or empty: then
	// the panel offers no restart, only the owner's command.
	Component string `json:"component,omitempty"`
	// Interruption names what restarting it interrupts: web, php,
	// database, mail, dns, panel, ssh, other.
	Interruption string `json:"interruption"`
	// Libraries are a few of the replaced files it still uses (deleted-map
	// reading only).
	Libraries []string `json:"libraries,omitempty"`
}

type RestartNeedsResponse struct {
	// Source: needrestart, maps (the processes' memory maps), or empty.
	Source         string                `json:"source,omitempty"`
	Services       []RestartNeedsService `json:"services"`
	ServicesKnown  bool                  `json:"services_known"`
	RebootRequired bool                  `json:"reboot_required"`
	RebootKnown    bool                  `json:"reboot_known"`
	// RebootReason: reboot_required_file, kernel_modules_gone,
	// needrestart_kernel.
	RebootReason   string   `json:"reboot_reason,omitempty"`
	RebootPackages []string `json:"reboot_packages,omitempty"`
	RunningKernel  string   `json:"running_kernel,omitempty"`
	ExpectedKernel string   `json:"expected_kernel,omitempty"`
	// NeedrestartAutoRestart: needrestart restarts services by itself after
	// updates on this server (what the OS does, not the panel).
	NeedrestartAutoRestart bool   `json:"needrestart_auto_restart,omitempty"`
	Truncated              bool   `json:"truncated,omitempty"`
	Error                  string `json:"error,omitempty"`
}
