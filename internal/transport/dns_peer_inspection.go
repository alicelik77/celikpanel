package transport

// Automatic peer inspection setup (2026-10-30). When two CelikPanel servers
// are paired with a join code, the setup link carries the owner tool's four
// manual steps (primary-prepare, secondary-install, secondary-host-key,
// primary-activate) between the two Agents. Each Agent writes exactly the
// native files the dns-peer-enroll owner tool writes, so the owner can read,
// keep or revoke them without the panel. Only public material crosses the
// link: the main's SSH public key one way, the backup's SSH host-key digest
// the other. The private key never leaves the main's Agent state directory.

const (
	DNSPeerInspectionActionCheck    = "check"
	DNSPeerInspectionActionPrepare  = "prepare"
	DNSPeerInspectionActionInstall  = "install"
	DNSPeerInspectionActionActivate = "activate"
)

// States an Agent reports for one step. Every value is a reviewed token; no
// native command output or file content is carried.
const (
	// DNSPeerInspectionReady: this side can take the next step (check).
	DNSPeerInspectionReady = "ready"
	// DNSPeerInspectionPrepared: the main holds a prepared key (prepare).
	DNSPeerInspectionPrepared = "prepared"
	// DNSPeerInspectionInstalled: the backup's channel is configured for the
	// supplied key (install).
	DNSPeerInspectionInstalled = "installed"
	// DNSPeerInspectionActive: the main's enrollment is active for this pair.
	DNSPeerInspectionActive = "active"
	// DNSPeerInspectionExisting: an enrollment for this exact pair already
	// exists that this automation did not write; it is kept as it is.
	DNSPeerInspectionExisting = "existing"
	// DNSPeerInspectionOwnerConflict: an owner enrollment, channel file or
	// changed owner SSH configuration exists that does not match this pair;
	// nothing is changed.
	DNSPeerInspectionOwnerConflict = "owner_conflict"
	// DNSPeerInspectionRevoked: the channel this automation set up was
	// revoked by the owner; it is never authorized again automatically.
	DNSPeerInspectionRevoked = "revoked"
	// DNSPeerInspectionPairNotReady: this server's DNS pair is not set up
	// yet (a wait: the caller asks again).
	DNSPeerInspectionPairNotReady = "pair_not_ready"
	// DNSPeerInspectionNotApplicable: this server is not the expected side
	// of a managed DNS pair.
	DNSPeerInspectionNotApplicable = "not_applicable"
	// DNSPeerInspectionSSHPort: OpenSSH on the backup does not listen on
	// port 22, the only port the inspection channel uses.
	DNSPeerInspectionSSHPort = "ssh_port"
	// DNSPeerInspectionInspectorUnavailable: the packaged inspector program
	// is not installed on this server.
	DNSPeerInspectionInspectorUnavailable = "inspector_unavailable"
	// DNSPeerInspectionFailed: a native step was refused (for example sshd
	// rejected the configuration); staged files are kept for the owner.
	DNSPeerInspectionFailed = "failed"
)

// DNSPeerInspectionSecondaryRequest is sent to the backup's own Agent.
// PublicKey is the main's public key (install only).
type DNSPeerInspectionSecondaryRequest struct {
	Action    string `json:"action"`
	PublicKey string `json:"public_key,omitempty"`
}

type DNSPeerInspectionSecondaryResponse struct {
	State string `json:"state"`
	// Engine is the backup's native DNS engine ("bind" or "pdns").
	Engine string `json:"engine,omitempty"`
	// PrimaryIP and LocalIP are this pair's addresses as this Agent's DNS
	// engine state records them (for the owner's guidance).
	PrimaryIP string `json:"primary_ip,omitempty"`
	LocalIP   string `json:"local_ip,omitempty"`
	// HostKeySHA256 is the backup's OpenSSH Ed25519 host key digest
	// (install, installed or existing).
	HostKeySHA256 string `json:"host_key_sha256,omitempty"`
	// Reason is a reviewed refinement for failed/owner_conflict.
	Reason string `json:"reason,omitempty"`
	// SSHPorts lists the ports OpenSSH listens on (ssh_port only).
	SSHPorts []int `json:"ssh_ports,omitempty"`
}

// DNSPeerInspectionPrimaryRequest is sent to the main's own Agent. Engine is
// the backup's native engine; CredentialID names a credential prepared
// earlier for this pair (reuse); HostKeySHA256 is the backup's host key
// digest (activate).
type DNSPeerInspectionPrimaryRequest struct {
	Action        string `json:"action"`
	Engine        string `json:"engine"`
	CredentialID  string `json:"credential_id,omitempty"`
	HostKeySHA256 string `json:"host_key_sha256,omitempty"`
}

type DNSPeerInspectionPrimaryResponse struct {
	State        string `json:"state"`
	CredentialID string `json:"credential_id,omitempty"`
	PublicKey    string `json:"public_key,omitempty"`
	// Probe is the reviewed result of one SSH reachability check after
	// activation (dnspeertransport.ProbeResult).
	Probe  string `json:"probe,omitempty"`
	Reason string `json:"reason,omitempty"`
	// LocalIP and PeerIP are this pair's addresses as the main's DNS engine
	// state records them (for the owner's guidance).
	LocalIP string `json:"local_ip,omitempty"`
	PeerIP  string `json:"peer_ip,omitempty"`
}

// Reviewed refinements carried in Reason.
const (
	DNSPeerInspectionReasonOtherEngine     = "other_engine"
	DNSPeerInspectionReasonOtherKey        = "other_key"
	DNSPeerInspectionReasonOtherPair       = "other_pair"
	DNSPeerInspectionReasonOwnerSSHConfig  = "owner_ssh_config"
	DNSPeerInspectionReasonStaged          = "staged_by_owner"
	DNSPeerInspectionReasonUnreadable      = "unreadable"
	DNSPeerInspectionReasonSSHDRejected    = "sshd_rejected"
	DNSPeerInspectionReasonSSHInactive     = "ssh_inactive"
	DNSPeerInspectionReasonDNSInactive     = "dns_inactive"
	DNSPeerInspectionReasonHostKey         = "host_key"
	DNSPeerInspectionReasonNativeStep      = "native_step"
	DNSPeerInspectionReasonCredentialStale = "credential_stale"
)
