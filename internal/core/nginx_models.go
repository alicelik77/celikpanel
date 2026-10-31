package core

// NginxGlobalConfig represents global nginx settings
type NginxGlobalConfig struct {
	WorkerProcesses   string `json:"worker_processes"`
	WorkerConnections string `json:"worker_connections"`
	KeepaliveTimeout  string `json:"keepalive_timeout"`
	ClientMaxBodySize string `json:"client_max_body_size"`
	ServerTokens      string `json:"server_tokens"` // on/off
	Gzip              string `json:"gzip"`          // on/off
}

// NginxSSLConfig represents default SSL settings
type NginxSSLConfig struct {
	SSLCiphers             string `json:"ssl_ciphers"`
	SSLProtocols           string `json:"ssl_protocols"`
	SSLPreferServerCiphers string `json:"ssl_prefer_server_ciphers"`
}

// NginxRateLimit represents a rate limiting zone
type NginxRateLimit struct {
	Name string `json:"name"`
	Zone string `json:"zone"` // e.g. $binary_remote_addr
	Size string `json:"size"` // e.g. 10m
	Rate string `json:"rate"` // e.g. 10r/s
}

// NginxGlobalConfigRequest represents a request to update global config
type NginxGlobalConfigRequest struct {
	Config NginxGlobalConfig `json:"config"`
}

// NginxSSLConfigRequest represents a request to update SSL config
type NginxSSLConfigRequest struct {
	Config NginxSSLConfig `json:"config"`
}

// NginxInspectResult is the real, agent-sourced nginx configuration read
// from `nginx -T`. Installed is false when nginx is absent.
// NginxInspectResult, agent'ın `nginx -T` ile okuduğu gerçek nginx
// yapılandırmasıdır. nginx yoksa Installed false olur.
type NginxInspectResult struct {
	Installed  bool              `json:"installed"`
	Global     NginxGlobalConfig `json:"global"`
	SSL        NginxSSLConfig    `json:"ssl"`
	RateLimits []NginxRateLimit  `json:"rate_limits"`
	// DefaultHTTPS is CelikPanel's default HTTPS server (alpha.113).
	DefaultHTTPS NginxDefaultHTTPS `json:"default_https"`
}

// States of CelikPanel's default HTTPS server (NginxDefaultHTTPS.State).
const (
	// NginxDefaultHTTPSActive: CelikPanel's file is in place.
	NginxDefaultHTTPSActive = "active"
	// NginxDefaultHTTPSOwnerDefault: another server block (the owner's) is
	// the default server of port 443; CelikPanel writes nothing beside it.
	NginxDefaultHTTPSOwnerDefault = "owner_default"
	// NginxDefaultHTTPSOwnerFile: the file is there but is not CelikPanel's
	// unchanged text (the owner changed or replaced it); it is kept.
	NginxDefaultHTTPSOwnerFile = "owner_file"
	// NginxDefaultHTTPSNotNeeded: no site serves HTTPS yet.
	NginxDefaultHTTPSNotNeeded = "not_needed"
	// NginxDefaultHTTPSPending: a site serves HTTPS, nothing else is the
	// default server, CelikPanel's file is not there yet, and no attempt is
	// known to have failed.
	NginxDefaultHTTPSPending = "pending"
	// NginxDefaultHTTPSFailed: the last attempt of this Agent run failed
	// (Detail says why) and the file is still not in place.
	NginxDefaultHTTPSFailed = "failed"
	// NginxDefaultHTTPSUnknown: nginx's configuration or the file could not
	// be read; nothing is claimed.
	NginxDefaultHTTPSUnknown = "unknown"
)

// NginxDefaultHTTPS is what the Agent knows of CelikPanel's default HTTPS
// server (/etc/nginx/conf.d/celikpanel-default-https.conf): the server that
// answers an HTTPS visitor whose name has no site here, so that visitor is
// never shown another site's certificate.
//
// CelikPanel'in varsayılan HTTPS sunucusunun durumu.
type NginxDefaultHTTPS struct {
	State string `json:"state"`
	// Mode: "reject" (the handshake is refused, nginx >= 1.19.4) or
	// "placeholder" (a certificate that names no site, older nginx).
	Mode         string `json:"mode,omitempty"`
	NginxVersion string `json:"nginx_version,omitempty"`
	Path         string `json:"path"`
	// OwnerFile is the file whose server block is the default server of
	// port 443 (owner_default).
	OwnerFile string `json:"owner_file,omitempty"`
	// FileState is the file's classification (owner_file, unknown).
	FileState string `json:"file_state,omitempty"`
	// Detail is the reason in a sentence: nginx's own line for a failure.
	Detail string `json:"detail,omitempty"`
	// LastOutcome and LastAt are the last attempt of this Agent run, if any.
	LastOutcome string `json:"last_outcome,omitempty"`
	LastAt      string `json:"last_at,omitempty"`
}
