package transport

// Side doors closed at each Panel start (S4 R1, 20 Nov 2026): the database
// tools and webmail under their own accounts, and the distribution's stock
// PHP-FPM pool turned off where it is the package's untouched file and
// nothing uses it. Agent.CloseSideDoorsV1 answers what it found and did; the
// Panel keeps the answer and shows what needs someone in Problems now.
//
// Her Panel başlangıcında kapatılan yan kapılar: araçlar ve web postası kendi
// hesaplarında, dağıtımın varsayılan PHP-FPM havuzu dokunulmamış ve
// kullanılmıyorsa kapalı.

// SideDoorsResponse is Agent.CloseSideDoorsV1's answer.
type SideDoorsResponse struct {
	State *SideDoorsState `json:"state,omitempty"`
	Error string          `json:"error,omitempty"`
}

// SideDoorsState is what the step found.
type SideDoorsState struct {
	CheckedAt  string               `json:"checked_at"`
	Tools      []SideDoorsTool      `json:"tools"`
	StockPools []SideDoorsStockPool `json:"stock_pools"`
}

// Tool states.
const (
	// The tool runs under its own account (already, or moved now).
	SideDoorsToolOwnAccount = "own_account"
	SideDoorsToolMoved      = "moved"
	// The move failed and was put back; it is tried again when the
	// administrator chooses the tool's PHP version in its Manage window.
	SideDoorsToolFailed = "failed"
	// The owner changed the tool's web server configuration; it is kept.
	SideDoorsToolOwnerConfig = "owner_config"
	// Not tried now (PHP-FPM not running, its PHP version gone); tried at the
	// next start.
	SideDoorsToolWaiting = "waiting"
	// This system runs one unversioned PHP (Arch): the tool stays in the
	// stock pool.
	SideDoorsToolUnsupported = "unsupported"
)

// SideDoorsTool is one installed tool.
type SideDoorsTool struct {
	Tool    string `json:"tool"`
	Version string `json:"version,omitempty"`
	Account string `json:"account,omitempty"`
	State   string `json:"state"`
	Detail  string `json:"detail,omitempty"`
}

// Stock pool states.
const (
	// Turned off now, or found turned off by CelikPanel before.
	SideDoorsStockTurnedOff = "turned_off"
	SideDoorsStockOff       = "off"
	// Kept: the owner changed it (it is not the package's file).
	SideDoorsStockOwnerChanged = "owner_changed"
	// Kept: a web server configuration hands PHP to it.
	SideDoorsStockInUse = "in_use"
	// Kept: PHP-FPM refuses to start without a pool, and it is the only one.
	SideDoorsStockOnlyPool = "only_pool"
	// Kept: whether it is untouched or used could not be read.
	SideDoorsStockUnknown = "unknown"
	// Turning it off failed; it was put back.
	SideDoorsStockFailed = "failed"
)

// SideDoorsStockPool is one PHP version's stock pool.
type SideDoorsStockPool struct {
	Version string   `json:"version"`
	Path    string   `json:"path"`
	State   string   `json:"state"`
	UsedBy  []string `json:"used_by,omitempty"`
	Detail  string   `json:"detail,omitempty"`
}
