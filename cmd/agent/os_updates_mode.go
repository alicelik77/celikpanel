package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The owner's choice between automatic and manual operating-system updates
// on Debian and Ubuntu (alpha.114). It is apt's own mechanism, in apt's own
// file: /etc/apt/apt.conf.d/20auto-upgrades, the file `dpkg-reconfigure
// unattended-upgrades` and Ubuntu's installer also write. apt's timers
// (apt-daily.timer, apt-daily-upgrade.timer) read it, so the choice keeps
// working with the panel stopped or removed. Nothing here is a CelikPanel
// service.
//
//   - Automatic: Update-Package-Lists "1" and Unattended-Upgrade "1".
//     unattended-upgrades then installs what its own configuration
//     (/etc/apt/apt.conf.d/50unattended-upgrades) allows: by default the
//     distribution's security updates. CelikPanel does not edit that file and
//     never turns on a reboot.
//   - Manual: Update-Package-Lists "1" (the lists, and so the waiting-update
//     count, stay fresh) and Unattended-Upgrade "0".
//
// The file is the panel's when its text is exactly what the panel renders for
// one of the two modes, a distribution's when it is exactly the two standard
// lines, and the owner's otherwise. An owner's file is never replaced unless
// the owner confirmed that, having seen its text. The earlier installer
// wrote 20celikpanel-auto-upgrades, which sorts after 20auto-upgrades and
// would override it; it is removed only when it is exactly what the installer
// wrote.
//
// Debian ve Ubuntu'da sahibin otomatik/elle güncelleme seçimi: apt'ın kendi
// dosyası ve zamanlayıcıları; panel olmadan da çalışır. Sahibin düzenlediği
// dosya onay olmadan değiştirilmez.

var (
	autoUpgradesPath       = "/etc/apt/apt.conf.d/20auto-upgrades"
	legacyAutoUpgradesPath = "/etc/apt/apt.conf.d/20celikpanel-auto-upgrades"
)

const (
	osUpdatesModeAuto   = "auto"
	osUpdatesModeManual = "manual"
	autoUpgradesReadCap = 8 << 10
)

// legacyAutoUpgradesText is exactly what install.sh wrote before alpha.114.
const legacyAutoUpgradesText = "APT::Periodic::Update-Package-Lists \"1\";\nAPT::Periodic::Unattended-Upgrade \"1\";\nAPT::Periodic::AutocleanInterval \"7\";\n"

func renderAutoUpgrades(mode string) string {
	value := "0"
	if mode == osUpdatesModeAuto {
		value = "1"
	}
	var b strings.Builder
	b.WriteString("// Written by CelikPanel at the owner's choice (operating-system updates: " + mode + ").\n")
	b.WriteString("// apt reads this file by itself; CelikPanel does not have to be running.\n")
	b.WriteString("// Edit it freely: CelikPanel notices and asks before it replaces your text.\n")
	b.WriteString("APT::Periodic::Update-Package-Lists \"1\";\n")
	b.WriteString("APT::Periodic::Unattended-Upgrade \"" + value + "\";\n")
	return b.String()
}

var (
	stockListsLine   = regexp.MustCompile(`^APT::Periodic::Update-Package-Lists\s+"([01])";$`)
	stockUnattendLin = regexp.MustCompile(`^APT::Periodic::Unattended-Upgrade\s+"([01])";$`)
)

// classifyAutoUpgrades says whose the file's text is: absent, panel (exactly
// one of the two texts the panel renders), stock (exactly the two standard
// lines a distribution writes, in either order) or edited (anything else).
// Mode is what a panel or stock file says.
func classifyAutoUpgrades(data []byte, exists bool) (state, mode string) {
	if !exists {
		return "absent", ""
	}
	text := string(data)
	for _, candidate := range []string{osUpdatesModeAuto, osUpdatesModeManual} {
		if text == renderAutoUpgrades(candidate) {
			return "panel", candidate
		}
	}
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 2 {
		lists, unattended := "", ""
		for _, line := range lines {
			if m := stockListsLine.FindStringSubmatch(line); m != nil {
				lists = m[1]
			}
			if m := stockUnattendLin.FindStringSubmatch(line); m != nil {
				unattended = m[1]
			}
		}
		if lists != "" && unattended != "" {
			if unattended == "1" {
				return "stock", osUpdatesModeAuto
			}
			return "stock", osUpdatesModeManual
		}
	}
	return "edited", ""
}

// readAutoUpgradesFile reads the file bounded; it refuses anything that is
// not a regular file (a symbolic link is the owner's own arrangement).
func readAutoUpgradesFile(path string) (data []byte, exists bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, true, fmt.Errorf("%s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, true, err
	}
	defer file.Close()
	data, err = io.ReadAll(io.LimitReader(file, autoUpgradesReadCap+1))
	if err != nil {
		return nil, true, err
	}
	if len(data) > autoUpgradesReadCap {
		// Longer than any file this choice has ever been: the owner's own.
		data = data[:autoUpgradesReadCap]
	}
	return data, true, nil
}

// unattendedCoverage lists the origins unattended-upgrades is set to install
// from, as apt reports them (bounded).
func unattendedCoverage(lists map[string][]string) []string {
	var out []string
	for _, key := range []string{"Unattended-Upgrade::Origins-Pattern", "Unattended-Upgrade::Allowed-Origins"} {
		for _, value := range lists[key] {
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
			}
		}
	}
	sort.Strings(out)
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

// osUpdatesAutoConfig reads, changing nothing, the state of the choice's file
// and its surroundings. scalars is apt-config's answer (nil when unreadable).
func osUpdatesAutoConfig(ctx context.Context, systemctl string, scalars map[string]string, lists map[string][]string) *transport.OSUpdatesAutoConfig {
	config := &transport.OSUpdatesAutoConfig{
		Path: autoUpgradesPath, LegacyPath: legacyAutoUpgradesPath, TimerState: "unknown",
		ProposedAuto: renderAutoUpgrades(osUpdatesModeAuto), ProposedManual: renderAutoUpgrades(osUpdatesModeManual),
	}
	data, exists, err := readAutoUpgradesFile(autoUpgradesPath)
	switch {
	case err != nil && exists:
		config.State = "edited"
		config.Unreadable = true
	case err != nil:
		config.State = "unknown"
	default:
		config.State, config.Mode = classifyAutoUpgrades(data, exists)
		if exists {
			config.CurrentSHA256 = sha256Hex(data)
			config.Current = string(data)
		}
	}
	if legacy, legacyExists, err := readAutoUpgradesFile(legacyAutoUpgradesPath); legacyExists {
		if err == nil && string(legacy) == legacyAutoUpgradesText {
			config.LegacyState = "panel"
		} else {
			config.LegacyState = "edited"
		}
	}
	if scalars != nil && (config.State == "panel" || config.State == "stock") && config.LegacyState != "panel" {
		// What apt actually uses differs from what this file says: another
		// file (the owner's) wins.
		effective := scalars["APT::Periodic::Unattended-Upgrade"]
		if (config.Mode == osUpdatesModeAuto) != (effective != "" && effective != "0") {
			config.Overridden = true
		}
	}
	if systemctl != "" {
		config.TimerState = aptTimersState(ctx, systemctl)
	}
	config.Covers = unattendedCoverage(lists)
	return config
}

// aptTimersState is the weakest state of apt's two timers: masked beats
// disabled beats enabled.
func aptTimersState(ctx context.Context, systemctl string) string {
	rank := map[string]int{"enabled": 0, "static": 0, "indirect": 0, "alias": 0, "disabled": 1, "masked": 2, "masked-runtime": 2}
	worst, worstName := -1, "unknown"
	for _, unit := range []string{"apt-daily.timer", "apt-daily-upgrade.timer"} {
		state := osUpdatesSystemctlProperty(ctx, systemctl, unit, "UnitFileState")
		level, known := rank[state]
		if !known {
			return "unknown"
		}
		if level > worst {
			worst = level
			switch level {
			case 0:
				worstName = "enabled"
			case 1:
				worstName = "disabled"
			default:
				worstName = "masked"
			}
		}
	}
	return worstName
}

// SetOSUpdatesModeV1 writes the owner's choice. The owner saw the current
// text (its SHA-256 is ExpectedCurrentSHA256, empty when the file was
// absent); if the file changed since, or is the owner's own and was not
// confirmed, nothing is written.
func (a *Agent) SetOSUpdatesModeV1(req *transport.SetOSUpdatesModeRequest, resp *transport.SetOSUpdatesModeResponse) error {
	*resp = transport.SetOSUpdatesModeResponse{Path: autoUpgradesPath}
	if req == nil || (req.Mode != osUpdatesModeAuto && req.Mode != osUpdatesModeManual) {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "the choice must be automatic or manual"
		return nil
	}
	resp.Mode = req.Mode
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepSetOSUpdatesMode, "unattended-upgrades", "", "mode"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = "MUTATION_REFUSED", err.Error()
		return nil
	}
	defer finishStep()
	profile, err := verifiedHostProfile("apt")
	if err != nil {
		resp.ErrorCode, resp.Error = "UNSUPPORTED_SYSTEM", err.Error()
		return nil
	}
	packageOperationMu.Lock()
	defer packageOperationMu.Unlock()
	applyOSUpdatesMode(ctx, req, resp, profile.Executables["systemctl"])
	return nil
}

// applyOSUpdatesMode is the write, after the step and the host are proven.
func applyOSUpdatesMode(ctx context.Context, req *transport.SetOSUpdatesModeRequest, resp *transport.SetOSUpdatesModeResponse, systemctl string) {
	installed, _ := readDpkgStatusVersions(dpkgStatusPath)
	if req.Mode == osUpdatesModeAuto && len(installed["unattended-upgrades"]) == 0 {
		resp.ErrorCode, resp.Error = "UNATTENDED_NOT_INSTALLED", "unattended-upgrades is not installed, so automatic updates cannot run; nothing was written"
		return
	}
	data, exists, err := readAutoUpgradesFile(autoUpgradesPath)
	if err != nil {
		resp.ErrorCode, resp.Error = "NOT_REGULAR", err.Error()+"; CelikPanel does not replace it"
		return
	}
	currentSHA := ""
	if exists {
		currentSHA = sha256Hex(data)
	}
	if currentSHA != req.ExpectedCurrentSHA256 {
		resp.ErrorCode, resp.Error = "CURRENT_CHANGED", autoUpgradesPath+" changed after it was shown; nothing was written. Read the card again"
		return
	}
	state, _ := classifyAutoUpgrades(data, exists)
	if state == "edited" && !req.ConfirmOwnerEdit {
		resp.ErrorCode, resp.Error = "OWNER_EDITED", autoUpgradesPath+" was changed by the owner; CelikPanel replaces it only after you confirm having read its text"
		return
	}
	text := renderAutoUpgrades(req.Mode)
	if err := publishRepoPin(autoUpgradesPath, []byte(text)); err != nil {
		resp.ErrorCode, resp.Error = "WRITE_FAILED", err.Error()
		return
	}
	resp.Written = true
	resp.PreviousState = state
	// The installer's earlier file would override the new one; it goes only
	// when it is exactly what the installer wrote.
	if legacy, legacyExists, lerr := readAutoUpgradesFile(legacyAutoUpgradesPath); legacyExists && lerr == nil && string(legacy) == legacyAutoUpgradesText {
		if err := removeRepoFileVerified(legacyAutoUpgradesPath); err == nil {
			resp.LegacyRemoved = true
		}
	}
	if req.Mode == osUpdatesModeAuto && systemctl != "" {
		for _, unit := range []string{"apt-daily.timer", "apt-daily-upgrade.timer"} {
			if osUpdatesSystemctlProperty(ctx, systemctl, unit, "UnitFileState") == "disabled" {
				if _, err := osUpdatesCommand(ctx, packageCommandEnv(), systemctl, "enable", "--now", unit); err == nil {
					resp.TimersEnabled = append(resp.TimersEnabled, unit)
				}
			}
		}
	}
	resp.TimerState = "unknown"
	if systemctl != "" {
		resp.TimerState = aptTimersState(ctx, systemctl)
	}
	// Prove it the way apt reads it.
	out, err := osUpdatesCommand(ctx, packageCommandEnv(), aptConfigPath, "dump")
	if err != nil {
		resp.EffectiveReason = "apt_config_unreadable"
		return
	}
	scalars, _ := parseAptConfigDump(string(out))
	value := scalars["APT::Periodic::Unattended-Upgrade"]
	on := value != "" && value != "0"
	switch {
	case on != (req.Mode == osUpdatesModeAuto):
		resp.EffectiveReason = "overridden_by_other_file"
	case req.Mode == osUpdatesModeAuto && resp.TimerState == "masked":
		resp.EffectiveReason = "timer_masked"
	case req.Mode == osUpdatesModeAuto && resp.TimerState == "disabled":
		resp.EffectiveReason = "timer_disabled"
	default:
		resp.Effective = true
	}
	return
}
