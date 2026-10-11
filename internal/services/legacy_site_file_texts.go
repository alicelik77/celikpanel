package services

import (
	"fmt"
	"strings"
)

// Frozen texts of the PHP-FPM pool and the application unit as the releases
// before the render header wrote them (D-031 point 6, second step,
// 2026-10-11). They are copies, never edited, of:
//
//   - the pool: `renderPool`'s format in internal/services/php_pool_manager.go
//     (identical in v0.1.0-alpha.81, blob 237ff2c89388be78df96e77fbba522b69750d67b,
//     and v0.1.0-alpha.82, blob 0aa01b2fea70f8fa364c0ce32280898ac8b50251), which
//     every pool save and PHP switch wrote, and the creation template
//     `phpPoolTemplate` in internal/services/php_manager.go, which renders the
//     same lines with the default process-manager values; alpha.81 (blob
//     fa8becda5cb7405e5bed4c0676935dd1ff474cbf) created the socket as
//     /var/run/php/php<v>-fpm-<pool>.sock owned by www-data, alpha.82 (blob
//     2a8f6bfb221cbf305f476a31bd051e3d155822b4) as this host's layout's socket
//     owned by the web server's account;
//   - the unit: the text `ApplyAppUnit` built in cmd/agent/app_rpc.go;
//     alpha.81 (blob 303a5a08584040b4e5103eef6b203538ad9c06d9) wrote `User=`
//     as asked, alpha.82 (blob 7c83f7be4a3bbcecca442b48a3254a679065b8ca) the
//     web server's account that exists on the host.
//
// TestLegacySiteFileTextsAreFrozen pins their digests. A file with no header is
// adopted only when it is byte for byte one of these texts for the site's
// identity; any other file is "unknown origin" and is never overwritten.
//
// Başlıksız havuz ve uygulama birimi yazan sürümlerin dondurulmuş metinleri;
// asla düzenlenmez.

const (
	legacyReleaseAlpha81 = "v0.1.0-alpha.81"
	legacyReleaseAlpha82 = "v0.1.0-alpha.82"
)

// legacyPoolFormat is renderPool's format in alpha.81 and alpha.82.
const legacyPoolFormat = `[%s]
user = %s
group = %s
listen = %s
listen.owner = %s
listen.group = %s
listen.mode = %s
pm = %s
pm.max_children = %d
pm.start_servers = %d
pm.min_spare_servers = %d
pm.max_spare_servers = %d
pm.max_requests = %d
chdir = /
`

// legacyUnitComment is the comment the unit of alpha.81 and alpha.82 began with.
const legacyUnitComment = "# Managed by CelikPanel — do not edit by hand.\n" +
	"# CelikPanel tarafından yönetilir — elle düzenlemeyin.\n"

// poolIdentity is who a pool runs as and where it listens.
type poolIdentity struct {
	user, group, listen, listenOwner, listenGroup, listenMode string
}

func legacyPoolText(name string, identity poolIdentity, values poolValues) string {
	return fmt.Sprintf(legacyPoolFormat, name, identity.user, identity.group, identity.listen,
		identity.listenOwner, identity.listenGroup, identity.listenMode, values.pm,
		values.maxChildren, values.startServers, values.minSpare, values.maxSpare, values.maxRequests)
}

// legacyPoolRenders returns the texts alpha.82 and alpha.81 would have left
// for this site's pool with the process-manager values the file carries: the
// creation identity of each release (a save kept the file's identity, so
// every pool those releases wrote has it) and the values as written. Newest
// first.
func legacyPoolRenders(version, poolName, username string, content []byte) func() ([]LegacyVhostRender, error) {
	return func() ([]LegacyVhostRender, error) {
		values, ok := poolValuesWritten(content)
		if !ok {
			return nil, nil
		}
		alpha82 := sitePoolIdentity(version, poolName, username)
		alpha81 := poolIdentity{
			user: username, group: username,
			listen:      fmt.Sprintf("/var/run/php/php%s-fpm-%s.sock", version, poolName),
			listenOwner: "www-data", listenGroup: "www-data", listenMode: "0660",
		}
		return []LegacyVhostRender{
			{Release: legacyReleaseAlpha82, Text: legacyPoolText(poolName, alpha82, values)},
			{Release: legacyReleaseAlpha81, Text: legacyPoolText(poolName, alpha81, values)},
		}, nil
	}
}

// AppUnitParams are the inputs of an application unit's text.
type AppUnitParams struct {
	Description string
	// User is the account this release writes in User= ("" writes no line:
	// a user-mode development agent, or no account asked for); RequestedUser
	// is the account the Panel asked for, which alpha.81 wrote as it was.
	User          string
	RequestedUser string
	WorkDir       string
	Port          int
	// PathEnv is this release's PATH line: the Panel-installed Node
	// directory and the system directories, never the Agent's environment
	// (set12 observation: a bare `node` exited 127).
	PathEnv string
	// LegacyPathEnv is the PATH alpha.82 and alpha.81 wrote: the Node
	// directory and the Agent's own PATH ("" takes PathEnv).
	LegacyPathEnv string
	Command       string
	InstallTarget string
	// DropInDir is the unit's drop-in directory, the owner's place.
	DropInDir string
}

func unitValue(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, value)
}

// appUnitSections is the unit's text after its comment, as this release
// (legacy false) or alpha.82 and alpha.81 (legacy true) write it, for the
// given User= account.
//
// This release's start command runs under `/bin/sh -c`, not `/bin/sh -lc`
// (set12 observation, Debian 13: a login shell reads /etc/profile, which sets
// PATH again, so the unit's Environment=PATH with the Panel's Node directory
// did not reach the command and a bare `node` exited 127). A shell is still
// needed because the start command is a shell command line (quoting, &&,
// $PORT); no CelikPanel feature needs a login shell. The earlier releases'
// texts are frozen as they were written (adoption compares them byte for byte).
func appUnitSections(p AppUnitParams, user string, legacy bool) string {
	pathEnv, shell := p.PathEnv, "/bin/sh -c"
	if legacy {
		shell = "/bin/sh -lc"
		if p.LegacyPathEnv != "" {
			pathEnv = p.LegacyPathEnv
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\nDescription=CelikPanel app: %s\nAfter=network.target\n\n", unitValue(p.Description))
	fmt.Fprintf(&b, "[Service]\n")
	if user != "" {
		fmt.Fprintf(&b, "User=%s\n", unitValue(user))
	}
	fmt.Fprintf(&b, "WorkingDirectory=%s\n", p.WorkDir)
	fmt.Fprintf(&b, "Environment=PORT=%d\n", p.Port)
	fmt.Fprintf(&b, "Environment=NODE_ENV=production\n")
	fmt.Fprintf(&b, "Environment=PATH=%s\n", pathEnv)
	fmt.Fprintf(&b, "ExecStart=%s '%s'\n", shell, strings.ReplaceAll(p.Command, "'", `'\''`))
	fmt.Fprintf(&b, "Restart=on-failure\nRestartSec=3\n\n")
	fmt.Fprintf(&b, "[Install]\nWantedBy=%s\n", p.InstallTarget)
	return b.String()
}

// AppUnitBody is this release's text of the unit (the body under the header).
// Its comment names the owner's place: the unit's drop-in directory, which
// CelikPanel never writes or removes.
func AppUnitBody(p AppUnitParams) string {
	return "# The application unit of this site, written by CelikPanel.\n" +
		"# Your own settings go in " + p.DropInDir + "/*.conf (systemd drop-ins); CelikPanel never writes or removes them.\n" +
		"# An edit to this file is kept: CelikPanel then shows the difference and asks before writing it again.\n" +
		"# Kendi ayarlarınız " + p.DropInDir + "/*.conf içine; CelikPanel onlara dokunmaz. Bu dosyadaki bir değişiklik korunur.\n" +
		appUnitSections(p, p.User, false)
}

// LegacyAppUnitRenders returns the texts alpha.82 and alpha.81 wrote for the
// same inputs, newest first.
func LegacyAppUnitRenders(p AppUnitParams) func() ([]LegacyVhostRender, error) {
	return func() ([]LegacyVhostRender, error) {
		requested := p.RequestedUser
		if p.User == "" {
			requested = ""
		}
		return []LegacyVhostRender{
			{Release: legacyReleaseAlpha82, Text: legacyUnitComment + appUnitSections(p, p.User, true)},
			{Release: legacyReleaseAlpha81, Text: legacyUnitComment + appUnitSections(p, requested, true)},
		}, nil
	}
}
