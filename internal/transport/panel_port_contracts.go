package transport

// Panel port (alpha.114, owner decision 2026-10-31). The panel's listen
// address lives in the root-only /etc/celikpanel/panel.env, which only the
// Agent may read or write. The Panel reads it before a change (an owner's
// own edit is detected, never overwritten) and, after the owner confirmed
// signing in on the new port, asks the Agent to change that single line.
//
// Panel portu: dinleme adresi root'a ait panel.env'dedir. Panel değişiklikten
// önce okur (sahibin kendi düzenlemesi algılanır) ve sahip yeni portta oturum
// açtığını onayladıktan sonra Agent'tan yalnız o satırı değiştirmesini ister.

// PanelListenConfigResponse is panel.env as the Agent reads it now.
// Present is false when the file does not exist (a development or demo run).
// SHA256 binds a later change to exactly this file.
type PanelListenConfigResponse struct {
	Present bool   `json:"present"`
	Listen  string `json:"listen"`
	SHA256  string `json:"sha256"`
	Error   string `json:"error,omitempty"`
}

// PanelListenSetRequest changes CELIKPANEL_LISTEN from From to To, only over
// the file whose SHA-256 the Panel saw.
type PanelListenSetRequest struct {
	ServiceMutationBinding
	From           string `json:"from"`
	To             string `json:"to"`
	ExpectedSHA256 string `json:"expected_sha256"`
}

// PanelListenSetResponse: Changed when panel.env now says To. ErrorCode is
// CONFIG_CHANGED when the file is not the one the Panel saw (nothing was
// written), CONFIG_UNSAFE when its owner or mode is not root-only,
// WRITE_FAILED when the new file could not be published (the old one stays).
type PanelListenSetResponse struct {
	Changed   bool   `json:"changed"`
	SHA256    string `json:"sha256,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
}
