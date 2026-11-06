package transport

// Site sandbox (alpha.119, docs/SITE-SANDBOX.md): what the host enforces and
// the proof of one running sandboxed unit. SandboxCapsV1 is a read: it
// changes nothing on the server.
//
// Site koruması: sunucunun neyi uyguladığı ve çalışan bir birimin kanıtı.
// SandboxCapsV1 yalnız okur.

// SandboxCaps is what this host's systemd, kernel and nftables can enforce.
type SandboxCaps struct {
	SystemdVersion  int  `json:"systemd_version"`
	BPFFramework    bool `json:"bpf_framework"`
	CgroupV2        bool `json:"cgroup_v2"`
	NFTSocketCgroup bool `json:"nft_socket_cgroupv2"`
}

// SandboxUnit is one sandboxed unit file found on the server.
type SandboxUnit struct {
	InstallID int `json:"install_id"`
	// Kind is "" for a service app and "node" for a Node.js site
	// (alpha.127), whose SiteID is then set instead of InstallID.
	Kind   string `json:"kind,omitempty"`
	SiteID int    `json:"site_id,omitempty"`
	Unit   string `json:"unit"`
	// File: ours, edited (the owner changed it) or foreign.
	File       string `json:"file"`
	Generation int    `json:"generation"`
	// Active is systemd's ActiveState of a Node.js site's unit (alpha.127).
	Active string `json:"active,omitempty"`
}

// SandboxCapsResponse answers SandboxCapsV1.
type SandboxCapsResponse struct {
	Caps SandboxCaps `json:"caps"`
	// Level is full or reduced; Gaps name what a reduced host lacks and
	// Extras what a newer systemd adds.
	Level  string   `json:"level"`
	Gaps   []string `json:"gaps"`
	Extras []string `json:"extras"`
	// Generation is the sandbox generation this release renders.
	Generation int `json:"generation"`
	// EgressFile is the D-031 state of the egress table file (absent,
	// managed_unchanged, owner_edited, foreign, unknown_origin,
	// unreadable); EgressLoaded whether its rule is in the kernel now;
	// GuardTimer the guard timer's ActiveState; SMTPRejects the refused
	// mail connections since the table was made (-1: not read).
	EgressFile   string        `json:"egress_file"`
	EgressLoaded bool          `json:"egress_loaded"`
	GuardTimer   string        `json:"guard_timer"`
	SMTPRejects  int64         `json:"smtp_rejects"`
	Units        []SandboxUnit `json:"units"`
	Error        string        `json:"error,omitempty"`
}

// SandboxProof is the coded outcome of a unit's proof (sitesandbox.Prove).
type SandboxProof struct {
	OK     bool   `json:"ok"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}
