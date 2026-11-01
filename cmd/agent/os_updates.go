package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Security patches within a version are the operating system's job (D-038
// §4): its own automatic update applies them, independently of the panel.
// This reads, without changing anything, what the operating system's own
// configuration says and what its package manager would do now; it starts
// no update. The one write (WriteVendorUpdateOriginsV1) is a separate
// drop-in at the owner's confirmation; the owner's own files are never
// edited and automatic updates are never switched off.
//
// Sürüm içi güvenlik yamaları işletim sisteminin işidir. Bu, işletim
// sisteminin kendi yapılandırmasını ve paket yöneticisinin şimdi ne
// yapacağını değiştirmeden okur; güncelleme başlatmaz.

var (
	aptConfigPath              = "/usr/bin/apt-config"
	unattendedUpgradesLogPath  = "/var/log/unattended-upgrades/unattended-upgrades.log"
	pacmanLogPath              = "/var/log/pacman.log"
	vendorOriginsDropInPath    = "/etc/apt/apt.conf.d/52celikpanel-vendor-origins"
	needrestartConfigDir       = "/etc/needrestart"
	osUpdatesCommand           = runPackageSimulationCommand
	osUpdatesSystemctlProperty = func(ctx context.Context, systemctl, unit, property string) string {
		out, err := osUpdatesCommand(ctx, packageCommandEnv(), systemctl, "show", unit, "-p", property, "--value")
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
)

const (
	osUpdatesBound            = 75 * time.Second
	osUpdatesLogTail          = 256 << 10
	osUpdatesSecurityNamesCap = 20
	osUpdatesVendorNamesCap   = 12
)

// aptConfigValues reads `apt-config dump`: every scalar value, and every
// list as its items (Unattended-Upgrade::Origins-Pattern:: "…").
func parseAptConfigDump(out string) (scalars map[string]string, lists map[string][]string) {
	scalars, lists = map[string]string{}, map[string][]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, ";") {
			continue
		}
		line = strings.TrimSuffix(line, ";")
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if strings.HasSuffix(key, "::") {
			name := strings.TrimSuffix(key, "::")
			lists[name] = append(lists[name], value)
			continue
		}
		scalars[key] = value
	}
	return scalars, lists
}

// unattendedCovers says whether unattended-upgrades' origins name a
// repository's Release Origin (Origins-Pattern "origin=…"/"o=…", or the older
// Allowed-Origins "Origin:archive").
func unattendedCovers(lists map[string][]string, origin string) bool {
	origin = strings.ToLower(strings.TrimSpace(origin))
	if origin == "" {
		return false
	}
	for _, pattern := range lists["Unattended-Upgrade::Origins-Pattern"] {
		for _, part := range strings.Split(strings.ToLower(pattern), ",") {
			part = strings.TrimSpace(part)
			if part == "origin="+origin || part == "o="+origin {
				return true
			}
		}
	}
	for _, allowed := range lists["Unattended-Upgrade::Allowed-Origins"] {
		if strings.HasPrefix(strings.ToLower(allowed), origin+":") {
			return true
		}
	}
	return false
}

// lastUnattendedRun is the time of the last "Starting unattended upgrades
// script" line in unattended-upgrades' own log.
var unattendedStartLine = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}),\d+ INFO Starting unattended upgrades script`)

func lastUnattendedRun(r io.Reader) string {
	last := ""
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		if match := unattendedStartLine.FindStringSubmatch(scanner.Text()); match != nil {
			last = match[1]
		}
	}
	if last == "" {
		return ""
	}
	// The log writes local time without a zone; it is said as written.
	if at, err := time.ParseInLocation("2006-01-02 15:04:05", last, time.Local); err == nil {
		return at.UTC().Format(time.RFC3339)
	}
	return ""
}

var pacmanUpgradeLine = regexp.MustCompile(`^\[([0-9T:+\-]+)\] \[PACMAN\] starting full system upgrade`)

func lastPacmanUpgrade(r io.Reader) string {
	last := ""
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		if match := pacmanUpgradeLine.FindStringSubmatch(scanner.Text()); match != nil {
			last = match[1]
		}
	}
	if at, err := time.Parse("2006-01-02T15:04:05-0700", last); err == nil {
		return at.UTC().Format(time.RFC3339)
	}
	return ""
}

func readFileTail(path string, limit int64) (io.Reader, func(), bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, false
	}
	if info, err := file.Stat(); err == nil && info.Size() > limit {
		_, _ = file.Seek(info.Size()-limit, io.SeekStart)
	}
	return file, func() { file.Close() }, true
}

// parsePendingUpgrades reads `apt-get -s dist-upgrade`: every Inst line is a
// waiting update; one whose release names a "-security" archive is a
// security update.
func parsePendingUpgrades(out string) transport.OSUpdatesPending {
	pending := transport.OSUpdatesPending{State: "known"}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "Inst" {
			continue
		}
		pending.Total++
		if strings.Contains(line, "-security") || strings.Contains(line, "Debian-Security") {
			pending.Security++
			if len(pending.SecurityNames) < osUpdatesSecurityNamesCap {
				pending.SecurityNames = append(pending.SecurityNames, fields[1])
			}
		}
	}
	return pending
}

// renderVendorOriginsDropIn is the exact text the confirmation writes.
func renderVendorOriginsDropIn(origins []string) string {
	if len(origins) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("// Written by CelikPanel at the owner's confirmation (D-038). It adds the vendor\n")
	b.WriteString("// repositories CelikPanel confined to their component to the origins the\n")
	b.WriteString("// operating system's automatic updates install from. Delete this file to undo\n")
	b.WriteString("// it. CelikPanel does not edit your own configuration.\n")
	b.WriteString("Unattended-Upgrade::Origins-Pattern {\n")
	for _, origin := range origins {
		fmt.Fprintf(&b, "\t\"origin=%s\";\n", origin)
	}
	b.WriteString("};\n")
	return b.String()
}

var validReleaseOrigin = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._+-]{0,79}$`)

// osUpdatesVendors reads every enabled vendor repository: its Release
// Origin, whether it is confined and covered, and how many installed
// packages come from it.
func osUpdatesVendors(lists map[string][]string, installed map[string][]string) []transport.OSUpdatesVendorOrigin {
	out := []transport.OSUpdatesVendorOrigin{}
	locations := enabledRepoLocations()
	ids := make([]string, 0, len(locations))
	for id := range locations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		repo := repoFromCatalogue(id)
		if repo == nil {
			continue
		}
		loc := locations[id]
		release, _ := readRepoReleaseInfo(loc)
		item := transport.OSUpdatesVendorOrigin{RepoID: id, Name: repo.Name, Origin: release.Origin}
		item.Confined = repoConfinementOf(repo, loc).State == repoConfinementConfined
		item.Covered = unattendedCovers(lists, release.Origin)
		if vendor, err := readVendorPackageIndex(loc); err == nil && installed != nil {
			names := repoKeptPackages(installed, vendor)
			item.Packages = len(names)
			if len(names) > osUpdatesVendorNamesCap {
				names = names[:osUpdatesVendorNamesCap]
			}
			item.Names = names
		}
		out = append(out, item)
	}
	return out
}

// vendorOriginsToAdd are the confined repositories with a readable Origin
// that the automatic updates do not cover yet, plus the ones CelikPanel's
// own drop-in already adds (so a rewrite keeps them).
func vendorOriginsToAdd(vendors []transport.OSUpdatesVendorOrigin, current []string) []string {
	set := map[string]bool{}
	for _, origin := range current {
		set[origin] = true
	}
	for _, vendor := range vendors {
		if vendor.Confined && validReleaseOrigin.MatchString(vendor.Origin) && (!vendor.Covered || set[vendor.Origin]) {
			set[vendor.Origin] = true
		}
	}
	out := make([]string, 0, len(set))
	for origin := range set {
		out = append(out, origin)
	}
	sort.Strings(out)
	return out
}

var dropInOriginLine = regexp.MustCompile(`^\s*"origin=([^"]+)";\s*$`)

// readVendorOriginsDropIn reads CelikPanel's drop-in: absent, present (its
// text is what CelikPanel renders for its own origins) or edited.
func readVendorOriginsDropIn() (state string, origins []string) {
	data, err := os.ReadFile(vendorOriginsDropInPath)
	if err != nil {
		return "absent", nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if match := dropInOriginLine.FindStringSubmatch(line); match != nil {
			origins = append(origins, match[1])
		}
	}
	if string(data) != renderVendorOriginsDropIn(origins) {
		return "edited", origins
	}
	return "present", origins
}

func needrestartRestartsAutomatically() bool {
	entries, err := os.ReadDir(filepath.Join(needrestartConfigDir, "conf.d"))
	files := []string{filepath.Join(needrestartConfigDir, "needrestart.conf")}
	if err == nil {
		for _, entry := range entries {
			files = append(files, filepath.Join(needrestartConfigDir, "conf.d", entry.Name()))
		}
	}
	mode := ""
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil || len(data) > 1<<20 {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "$nrconf{restart}") {
				if _, value, ok := strings.Cut(line, "="); ok {
					mode = strings.Trim(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), ";")), `'"`)
				}
			}
		}
	}
	return mode == "a"
}

// OSUpdatesStatusV1 answers the security updates card (alpha.107). Read
// only and bounded; it never waits for the package lock.
func (a *Agent) OSUpdatesStatusV1(_ *transport.Empty, resp *transport.OSUpdatesStatusResponse) error {
	*resp = transport.OSUpdatesStatusResponse{Vendors: []transport.OSUpdatesVendorOrigin{}, Outside: []transport.OSUpdatesOutside{}}
	ctx, cancel := context.WithTimeout(context.Background(), osUpdatesBound)
	defer cancel()
	profile, err := detectHostPlatform()
	if err != nil {
		resp.Error = "detect host platform: " + err.Error()
		return nil
	}
	family := string(profile.PackageManager)
	resp.PackageFamily = family
	resp.Automatic = transport.OSUpdatesAutomatic{State: "unknown"}
	resp.Pending = transport.OSUpdatesPending{State: "unknown"}
	systemctl := profile.Executables["systemctl"]
	switch family {
	case "apt":
		osUpdatesApt(ctx, profile.Executables["apt-get"], systemctl, resp)
	case "dnf":
		resp.Automatic.Tool = "dnf-automatic"
		resp.Automatic.State = "not_installed"
		for _, timer := range []string{"dnf-automatic-install.timer", "dnf5-automatic.timer", "dnf-automatic.timer"} {
			if systemctl == "" {
				break
			}
			switch osUpdatesSystemctlProperty(ctx, systemctl, timer, "UnitFileState") {
			case "enabled":
				resp.Automatic.State = "on"
				resp.LastRun = systemdTime(osUpdatesSystemctlProperty(ctx, systemctl, timer, "LastTriggerUSec"))
				resp.LastRunSource = "timer"
			case "disabled":
				if resp.Automatic.State != "on" {
					resp.Automatic.State, resp.Automatic.Reason = "off", "timer_disabled"
				}
			}
		}
	case "pacman":
		// Arch has no automatic updates by design, and CelikPanel does not
		// schedule one (osUpdatesPacman): the owner runs pacman -Syu.
		resp.Automatic.State = "not_applicable"
		resp.Automatic.Reason = "arch_full_upgrade_by_owner"
		if r, done, ok := readFileTail(pacmanLogPath, osUpdatesLogTail); ok {
			resp.LastRun = lastPacmanUpgrade(r)
			if resp.LastRun != "" {
				resp.LastRunSource = "log"
			}
			done()
		}
		osUpdatesPacman(ctx, resp)
	}
	if roundcubeInstalled() {
		item := transport.OSUpdatesOutside{ID: "roundcube"}
		if version := roundcubeInstalledVersion(); version != "" {
			item.Versions = []string{version}
		}
		resp.Outside = append(resp.Outside, item)
	}
	var nodes transport.NodeVersionsResponse
	if err := a.ListNodeVersions(&transport.Empty{}, &nodes); err == nil && len(nodes.Installed) > 0 {
		resp.Outside = append(resp.Outside, transport.OSUpdatesOutside{ID: "node", Versions: nodes.Installed})
	}
	return nil
}

// Arch Linux (decision of 2026-10-30, security before freedom). Arch is a
// rolling release with no security-only channel: an update is a full
// `pacman -Syu`, and a partial upgrade (-Sy without -u) is unsupported and can
// break the system. A full upgrade can also move PostgreSQL to a new major
// version whose data directory must then be converted by hand before the
// server starts again, change PHP's version under every site, or require a
// manual step announced on archlinux.org/news. An unattended full upgrade
// can therefore take hosted services down without anyone present, so
// CelikPanel never schedules one and never enables a timer silently. What it
// does instead is read, without changing the system's package database:
//
//   - waiting updates with pacman-contrib's checkupdates, which synchronises
//     a separate copy of the package databases (never /var/lib/pacman/sync,
//     so no partial upgrade can follow from it);
//   - which of them fix known vulnerabilities with arch-audit -u (the Arch
//     security tracker), when it is installed;
//   - when the last full upgrade ran, from pacman's own log.
//
// The card says that applying them is the owner's `sudo pacman -Syu`, after
// reading the Arch news; install.sh installs pacman-contrib and arch-audit
// from the official extra repository so the count is available.
//
// Arch'ta güvenlik-yalnız kanal yoktur; güncelleme tam -Syu'dur ve gözetimsiz
// tam yükseltme PostgreSQL'i elle dönüştürülmesi gereken yeni ana sürüme
// taşıyabilir. CelikPanel bu yüzden otomatik yükseltme zamanlamaz; bekleyen
// güncellemeleri sistemin paket veritabanını değiştirmeden okur.
var (
	checkupdatesPath = "/usr/bin/checkupdates"
	fakerootPath     = "/usr/bin/fakeroot"
	archAuditPath    = "/usr/bin/arch-audit"
)

var checkupdatesLine = regexp.MustCompile(`^([a-z0-9@._+-]+) (\S+) -> (\S+)$`)

// parseCheckupdates reads `checkupdates --nocolor`: "name old -> new" lines.
func parseCheckupdates(out string) []string {
	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		if match := checkupdatesLine.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			names = append(names, match[1])
		}
	}
	return names
}

var archAuditName = regexp.MustCompile(`^[a-z0-9@._+-]+$`)

// parseArchAuditNames reads `arch-audit -u -f %n`: one package per line.
func parseArchAuditNames(out string) []string {
	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); archAuditName.MatchString(line) {
			names = append(names, line)
		}
	}
	return names
}

type osUpdatesExitCoder interface{ ExitCode() int }

func osUpdatesPacman(ctx context.Context, resp *transport.OSUpdatesStatusResponse) {
	// checkupdates runs pacman under fakeroot even for root ("Cannot find the
	// fakeroot binary", Arch VM 2026-10-30): both are the tool.
	if !systemExecutable(checkupdatesPath) || !systemExecutable(fakerootPath) {
		resp.Pending.State, resp.Pending.Reason = "unknown", "checkupdates_missing"
		return
	}
	out, err := osUpdatesCommand(ctx, append(packageCommandEnv(), "CHECKUPDATES_DB=/var/lib/celikpanel-agent-private/checkupdates-db"), checkupdatesPath, "--nocolor")
	var names []string
	if err != nil {
		coded, ok := err.(osUpdatesExitCoder)
		if !ok || coded.ExitCode() != 2 {
			// 1 is checkupdates' own failure (no network, mirror refused):
			// unknown, never "nothing waiting".
			resp.Pending.State, resp.Pending.Reason = "unknown", "checkupdates_failed"
			return
		}
	} else {
		names = parseCheckupdates(string(out))
	}
	resp.Pending = transport.OSUpdatesPending{State: "known", Total: len(names), ListsAt: time.Now().UTC().Format(time.RFC3339)}
	if !systemExecutable(archAuditPath) {
		resp.Pending.SecurityState = "arch_audit_missing"
		return
	}
	auditOut, err := osUpdatesCommand(ctx, packageCommandEnv(), archAuditPath, "-u", "-f", "%n")
	if err != nil {
		resp.Pending.SecurityState = "arch_audit_failed"
		return
	}
	waiting := map[string]bool{}
	for _, name := range names {
		waiting[name] = true
	}
	resp.Pending.SecurityState = "known"
	for _, name := range parseArchAuditNames(string(auditOut)) {
		if !waiting[name] {
			continue
		}
		resp.Pending.Security++
		if len(resp.Pending.SecurityNames) < osUpdatesSecurityNamesCap {
			resp.Pending.SecurityNames = append(resp.Pending.SecurityNames, name)
		}
	}
}

// systemExecutable is a regular executable in a system directory that
// neither its group nor others can write (/usr/bin is the package manager's).
func systemExecutable(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0 && info.Mode().Perm()&0o111 != 0
}

func systemdTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "n/a" || value == "0" {
		return ""
	}
	for _, layout := range []string{"Mon 2006-01-02 15:04:05 MST", "Mon 2006-01-02 15:04:05 -0700"} {
		if at, err := time.Parse(layout, value); err == nil {
			return at.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func osUpdatesApt(ctx context.Context, aptGet, systemctl string, resp *transport.OSUpdatesStatusResponse) {
	resp.Automatic.Tool = "unattended-upgrades"
	installed, _ := readDpkgStatusVersions(dpkgStatusPath)
	var lists map[string][]string
	var dumpScalars map[string]string
	if _, err := os.Stat(aptConfigPath); err == nil {
		if out, err := osUpdatesCommand(ctx, packageCommandEnv(), aptConfigPath, "dump"); err == nil {
			var scalars map[string]string
			scalars, lists = parseAptConfigDump(string(out))
			dumpScalars = scalars
			switch {
			case len(installed["unattended-upgrades"]) == 0:
				resp.Automatic.State = "not_installed"
			case scalars["APT::Periodic::Unattended-Upgrade"] == "0" || scalars["APT::Periodic::Unattended-Upgrade"] == "":
				resp.Automatic.State, resp.Automatic.Reason = "off", "config_off"
			default:
				resp.Automatic.State = "on"
			}
		}
	}
	if resp.Automatic.State == "on" && systemctl != "" {
		if state := osUpdatesSystemctlProperty(ctx, systemctl, "apt-daily-upgrade.timer", "UnitFileState"); state == "disabled" || state == "masked" {
			resp.Automatic.State, resp.Automatic.Reason = "off", "timer_disabled"
		}
	}
	if r, done, ok := readFileTail(unattendedUpgradesLogPath, osUpdatesLogTail); ok {
		resp.LastRun = lastUnattendedRun(r)
		if resp.LastRun != "" {
			resp.LastRunSource = "log"
		}
		done()
	}
	if resp.LastRun == "" && systemctl != "" {
		resp.LastRun = systemdTime(osUpdatesSystemctlProperty(ctx, systemctl, "apt-daily-upgrade.timer", "LastTriggerUSec"))
		if resp.LastRun != "" {
			resp.LastRunSource = "timer"
		}
	}
	if aptGet != "" {
		if packageOperationMu.TryLock() {
			out, err := osUpdatesCommand(ctx, packageCommandEnv(), aptGet, "-s", "-o", "Debug::NoLocking=1", "dist-upgrade")
			packageOperationMu.Unlock()
			if err == nil {
				resp.Pending = parsePendingUpgrades(string(out))
			}
		} else {
			resp.Pending.State = "busy"
		}
		if age, ok := aptListsAge(); ok {
			resp.Pending.ListsAt = time.Now().Add(-age).UTC().Format(time.RFC3339)
		}
	}
	resp.Vendors = osUpdatesVendors(lists, installed)
	state, current := readVendorOriginsDropIn()
	dropIn := &transport.OSUpdatesDropIn{Path: vendorOriginsDropInPath, State: state, Origins: current}
	if state != "edited" && lists != nil {
		proposed := renderVendorOriginsDropIn(vendorOriginsToAdd(resp.Vendors, current))
		if proposed != "" && !(state == "present" && proposed == renderVendorOriginsDropIn(current)) {
			dropIn.Proposed, dropIn.ProposedSHA256 = proposed, sha256Hex([]byte(proposed))
		}
	}
	resp.DropIn = dropIn
	resp.AutoConfig = osUpdatesAutoConfig(ctx, systemctl, dumpScalars, lists)
	resp.NeedrestartAutoRestart = needrestartRestartsAutomatically()
}

// WriteVendorUpdateOriginsV1 writes the drop-in the owner confirmed: the
// text is computed again and must be the one confirmed; an edited drop-in is
// never rewritten. apt-config dump afterwards proves apt reads it.
func (a *Agent) WriteVendorUpdateOriginsV1(req *transport.WriteVendorUpdateOriginsRequest, resp *transport.WriteVendorUpdateOriginsResponse) error {
	*resp = transport.WriteVendorUpdateOriginsResponse{Path: vendorOriginsDropInPath}
	if req == nil || len(req.ExpectedSHA256) != 64 {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "the confirmed text is missing"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepWriteVendorOrigins, "unattended-upgrades", "", "origins"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = "MUTATION_REFUSED", err.Error()
		return nil
	}
	defer finishStep()
	if _, err := verifiedHostProfile("apt"); err != nil {
		resp.ErrorCode, resp.Error = "UNSUPPORTED_SYSTEM", err.Error()
		return nil
	}
	packageOperationMu.Lock()
	defer packageOperationMu.Unlock()
	installed, _ := readDpkgStatusVersions(dpkgStatusPath)
	out, err := osUpdatesCommand(ctx, packageCommandEnv(), aptConfigPath, "dump")
	if err != nil {
		resp.ErrorCode, resp.Error = "APT_CONFIG_UNREADABLE", "apt-config could not be read: "+err.Error()
		return nil
	}
	_, lists := parseAptConfigDump(string(out))
	state, current := readVendorOriginsDropIn()
	if state == "edited" {
		resp.ErrorCode, resp.Error = "DROP_IN_EDITED", vendorOriginsDropInPath+" was changed by someone; CelikPanel does not rewrite it"
		return nil
	}
	origins := vendorOriginsToAdd(osUpdatesVendors(lists, installed), current)
	text := renderVendorOriginsDropIn(origins)
	if text == "" || sha256Hex([]byte(text)) != req.ExpectedSHA256 {
		resp.ErrorCode, resp.Error = "PLAN_CHANGED", "what would be written is not what was confirmed; nothing was written. Read the card again"
		return nil
	}
	if err := publishRepoPin(vendorOriginsDropInPath, []byte(text)); err != nil {
		resp.ErrorCode, resp.Error = "WRITE_FAILED", err.Error()
		return nil
	}
	resp.Written, resp.Origins = true, origins
	if out, err := osUpdatesCommand(ctx, packageCommandEnv(), aptConfigPath, "dump"); err == nil {
		_, after := parseAptConfigDump(string(out))
		resp.Effective = true
		for _, origin := range origins {
			if !unattendedCovers(after, origin) {
				resp.Effective = false
			}
		}
	}
	return nil
}
