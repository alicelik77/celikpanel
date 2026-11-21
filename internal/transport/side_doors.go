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

// SideDoorsRequest is Agent.CloseSideDoorsV1's request. ReClose (S4 R1 fix
// round) is the administrator's explicit "close it again" for something the
// server owner undid: "at" (CelikPanel's /etc/at.allow deleted) or
// "stock_pool" (the stock pool of Version put back under its own name). The
// step forgets that the owner undid it and then decides as at a first start.
// Empty at each Panel start: an owner's undo is then never redone.
type SideDoorsRequest struct {
	ServiceMutationBinding
	ReClose string `json:"reclose,omitempty"`
	Version string `json:"version,omitempty"`
}

// ReClose targets.
const (
	SideDoorsReCloseAt        = "at"
	SideDoorsReCloseStockPool = "stock_pool"
)

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
	// S4 R1 (21 Nov 2026): `at` closed to site accounts, and site crontabs
	// with lines not made in CelikPanel. Absent from an older Agent.
	At           *SideDoorsAt           `json:"at,omitempty"`
	SiteCrontabs []SideDoorsSiteCrontab `json:"site_crontabs,omitempty"`
	// S4 R2b (22 Nov 2026): /etc/cron.allow for the sites whose tasks run as
	// timers, crontabs that came back for them, and lingering.
	Cron   *SideDoorsCron   `json:"cron,omitempty"`
	Linger *SideDoorsLinger `json:"linger,omitempty"`
}

// `at` states.
const (
	// The at package is not installed: nothing is written.
	SideDoorsAtNotInstalled = "not_installed"
	// /etc/at.allow was written now (or brought up to date: an account made
	// since may use at, as before).
	SideDoorsAtWritten = "written"
	// CelikPanel's /etc/at.allow is there and current.
	SideDoorsAtCurrent = "current"
	// Neither at.allow nor at.deny exists: only root may use at already.
	SideDoorsAtOnlyRoot = "only_root"
	// /etc/at.allow is the owner's: kept as it is (SiteAccounts: the site
	// accounts it lets use at).
	SideDoorsAtOwnerFile = "owner_file"
	// Not written: the accounts come from a directory service (LDAP, SSSD,
	// NIS…), so who may use at today cannot be listed safely.
	SideDoorsAtDirectory = "directory_accounts"
	// Writing failed; nothing changed. Tried again at the next start.
	SideDoorsAtFailed = "failed"
	// S4 R1 fix round: CelikPanel wrote /etc/at.allow and the server owner
	// deleted it. Never written again by itself; the administrator may close
	// it again explicitly (SideDoorsRequest.ReClose).
	SideDoorsAtOwnerRemoved = "owner_removed"
)

// SideDoorsAt is what the step found and did about `at`.
type SideDoorsAt struct {
	State        string   `json:"state"`
	Path         string   `json:"path,omitempty"`
	SiteAccounts []string `json:"site_accounts,omitempty"`
	Detail       string   `json:"detail,omitempty"`
	// Jobs: pending at jobs of site accounts, never removed by CelikPanel.
	Jobs []SideDoorsAtJob `json:"jobs,omitempty"`
}

// SideDoorsAtJob is one pending at job of a site account: number, account
// and time only (S4 R1 fix round: its commands are the site's own text and
// are not copied; the administrator reads them with at -c <number>).
type SideDoorsAtJob struct {
	ID      string `json:"id"`
	Account string `json:"account"`
	When    string `json:"when"`
}

// SideDoorsSiteCrontab is a site account whose crontab has lines not made in
// CelikPanel.
type SideDoorsSiteCrontab struct {
	Account        string `json:"account"`
	SubscriptionID int    `json:"subscription_id"`
	DomainID       int    `json:"domain_id"`
	Outside        int    `json:"outside"`
	// Unreadable (S4 R1 fix round): why this account's crontab could not be
	// read; it holds no baseline for the others.
	Unreadable string `json:"unreadable,omitempty"`
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
	// S4 R1 fix round: the tool does not answer as it is configured now,
	// after what the step could do (a move tried again and put back, or the
	// web server's configuration reloaded once).
	SideDoorsToolDown = "down"
)

// SideDoorsTool is one installed tool.
type SideDoorsTool struct {
	Tool    string `json:"tool"`
	Version string `json:"version,omitempty"`
	Account string `json:"account,omitempty"`
	State   string `json:"state"`
	Detail  string `json:"detail,omitempty"`
	// Path: the file the owner changed, when it is not the tool's web server
	// configuration (an owner's own pool that runs as the web server's
	// account). Absent from an older Agent.
	Path string `json:"path,omitempty"`
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
	// Turning it off failed; it was put back (Detail says when putting it
	// back failed too).
	SideDoorsStockFailed = "failed"
	// S4 R1 fix round: turning it off failed and putting it back under its
	// own name failed too; the server owner puts it back.
	SideDoorsStockNotPutBack = "not_put_back"
	// S4 R1 fix round: CelikPanel turned it off and the server owner put it
	// back under its own name. Kept on; never turned off again by itself.
	SideDoorsStockOwnerRestored = "owner_restored"
)

// SideDoorsStockPool is one PHP version's stock pool.
type SideDoorsStockPool struct {
	Version string   `json:"version"`
	Path    string   `json:"path"`
	State   string   `json:"state"`
	UsedBy  []string `json:"used_by,omitempty"`
	Detail  string   `json:"detail,omitempty"`
}
