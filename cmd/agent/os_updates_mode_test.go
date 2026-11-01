package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.114: the owner's automatic/manual choice is apt's own file. The
// file is the panel's only when its text is exactly what the panel renders;
// the owner's own text is never replaced without a confirmation, and a file
// that changed after it was shown is never replaced at all.

func TestAutoUpgradesFileIsClassifiedByItsExactText(t *testing.T) {
	cases := []struct {
		name, text, state, mode string
		exists                  bool
	}{
		{"absent", "", "absent", "", false},
		{"panel automatic", renderAutoUpgrades("auto"), "panel", "auto", true},
		{"panel manual", renderAutoUpgrades("manual"), "panel", "manual", true},
		{"ubuntu stock", "APT::Periodic::Update-Package-Lists \"1\";\nAPT::Periodic::Unattended-Upgrade \"1\";\n", "stock", "auto", true},
		{"debian reconfigure answered no", "APT::Periodic::Update-Package-Lists \"1\";\nAPT::Periodic::Unattended-Upgrade \"0\";\n", "stock", "manual", true},
		{"stock in the other order with blank lines", "\nAPT::Periodic::Unattended-Upgrade \"1\";\n\nAPT::Periodic::Update-Package-Lists \"1\";\n", "stock", "auto", true},
		{"owner added a line", "APT::Periodic::Update-Package-Lists \"1\";\nAPT::Periodic::Unattended-Upgrade \"1\";\nAPT::Periodic::Download-Upgradeable-Packages \"1\";\n", "edited", "", true},
		{"owner edited the panel text", renderAutoUpgrades("auto") + "// mine\n", "edited", "", true},
		{"owner changed one value inside the header text", strings.Replace(renderAutoUpgrades("auto"), "\"1\";\nAPT::Periodic::Unattended", "\"2\";\nAPT::Periodic::Unattended", 1), "edited", "", true},
		{"owner interval", "APT::Periodic::Update-Package-Lists \"7\";\nAPT::Periodic::Unattended-Upgrade \"1\";\n", "edited", "", true},
		{"empty file", "", "edited", "", true},
	}
	for _, c := range cases {
		state, mode := classifyAutoUpgrades([]byte(c.text), c.exists)
		if state != c.state || mode != c.mode {
			t.Errorf("%s: got %s/%s, want %s/%s", c.name, state, mode, c.state, c.mode)
		}
	}
	// A header-only difference in the mode is still the panel's own text.
	if renderAutoUpgrades("auto") == renderAutoUpgrades("manual") {
		t.Fatal("the two modes render the same text")
	}
	if !strings.Contains(renderAutoUpgrades("manual"), `APT::Periodic::Unattended-Upgrade "0";`) || !strings.Contains(renderAutoUpgrades("auto"), `APT::Periodic::Unattended-Upgrade "1";`) {
		t.Fatal("the rendered values are wrong")
	}
}

type modeLab struct {
	dir, auto, legacy string
	dump              string // what apt-config dump answers after the write
	timerState        map[string]string
	commands          []string
}

func newModeLab(t *testing.T) *modeLab {
	t.Helper()
	lab := &modeLab{dir: t.TempDir(), timerState: map[string]string{"apt-daily.timer": "enabled", "apt-daily-upgrade.timer": "enabled"}}
	lab.auto = filepath.Join(lab.dir, "20auto-upgrades")
	lab.legacy = filepath.Join(lab.dir, "20celikpanel-auto-upgrades")
	status := filepath.Join(lab.dir, "status")
	os.WriteFile(status, []byte("Package: unattended-upgrades\nStatus: install ok installed\nVersion: 2.12\n\n"), 0o644)
	oldAuto, oldLegacy, oldStatus, oldCommand, oldConfig := autoUpgradesPath, legacyAutoUpgradesPath, dpkgStatusPath, osUpdatesCommand, aptConfigPath
	autoUpgradesPath, legacyAutoUpgradesPath, dpkgStatusPath, aptConfigPath = lab.auto, lab.legacy, status, "/usr/bin/apt-config"
	t.Cleanup(func() {
		autoUpgradesPath, legacyAutoUpgradesPath, dpkgStatusPath, osUpdatesCommand, aptConfigPath = oldAuto, oldLegacy, oldStatus, oldCommand, oldConfig
	})
	osUpdatesCommand = func(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
		lab.commands = append(lab.commands, name+" "+strings.Join(args, " "))
		switch {
		case strings.HasSuffix(name, "apt-config"):
			if lab.dump != "" {
				return []byte(lab.dump), nil
			}
			// apt reads the file as written.
			data, _ := os.ReadFile(lab.auto)
			value := "0"
			if strings.Contains(string(data), `Unattended-Upgrade "1"`) {
				value = "1"
			}
			if legacy, err := os.ReadFile(lab.legacy); err == nil && strings.Contains(string(legacy), `Unattended-Upgrade "1"`) {
				value = "1"
			}
			return []byte(`APT::Periodic::Unattended-Upgrade "` + value + `";` + "\n"), nil
		case strings.HasSuffix(name, "systemctl") && len(args) > 0 && args[0] == "show":
			return []byte(lab.timerState[args[1]] + "\n"), nil
		case strings.HasSuffix(name, "systemctl") && len(args) > 0 && args[0] == "enable":
			lab.timerState[args[2]] = "enabled"
			return nil, nil
		}
		return nil, errors.New("unexpected command " + name)
	}
	return lab
}

func (lab *modeLab) apply(req transport.SetOSUpdatesModeRequest) transport.SetOSUpdatesModeResponse {
	resp := transport.SetOSUpdatesModeResponse{Path: autoUpgradesPath, Mode: req.Mode}
	applyOSUpdatesMode(context.Background(), &req, &resp, "/usr/bin/systemctl")
	return resp
}

func (lab *modeLab) text(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(lab.auto)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTheChoiceIsWrittenAsAptsOwnFileAndProvedThroughApt(t *testing.T) {
	lab := newModeLab(t)
	resp := lab.apply(transport.SetOSUpdatesModeRequest{Mode: "auto"})
	if !resp.Written || !resp.Effective || lab.text(t) != renderAutoUpgrades("auto") || resp.PreviousState != "absent" {
		t.Fatalf("auto: %+v", resp)
	}
	status := osUpdatesAutoConfig(context.Background(), "/usr/bin/systemctl", map[string]string{"APT::Periodic::Unattended-Upgrade": "1"}, nil)
	if status.State != "panel" || status.Mode != "auto" || status.TimerState != "enabled" || status.Overridden || status.CurrentSHA256 != sha256Hex([]byte(renderAutoUpgrades("auto"))) {
		t.Fatalf("status = %+v", status)
	}
	// Back to manual, from the panel's own text: no confirmation needed.
	resp = lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual", ExpectedCurrentSHA256: status.CurrentSHA256})
	if !resp.Written || !resp.Effective || lab.text(t) != renderAutoUpgrades("manual") || resp.PreviousState != "panel" {
		t.Fatalf("manual: %+v", resp)
	}
}

func TestTheOwnersEditedFileIsNotReplacedWithoutTheirConfirmation(t *testing.T) {
	lab := newModeLab(t)
	owner := "APT::Periodic::Update-Package-Lists \"3\";\nAPT::Periodic::Unattended-Upgrade \"1\";\nAPT::Periodic::Verbose \"2\";\n"
	os.WriteFile(lab.auto, []byte(owner), 0o644)
	seen := sha256Hex([]byte(owner))
	status := osUpdatesAutoConfig(context.Background(), "/usr/bin/systemctl", map[string]string{"APT::Periodic::Unattended-Upgrade": "1"}, nil)
	if status.State != "edited" || status.Current != owner || status.CurrentSHA256 != seen {
		t.Fatalf("status = %+v", status)
	}
	resp := lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual", ExpectedCurrentSHA256: seen})
	if resp.Written || resp.ErrorCode != "OWNER_EDITED" || lab.text(t) != owner {
		t.Fatalf("unconfirmed: %+v", resp)
	}
	// The file changed after the owner saw it: nothing is written, even confirmed.
	resp = lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual", ExpectedCurrentSHA256: sha256Hex([]byte("other")), ConfirmOwnerEdit: true})
	if resp.Written || resp.ErrorCode != "CURRENT_CHANGED" || lab.text(t) != owner {
		t.Fatalf("changed: %+v", resp)
	}
	resp = lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual", ExpectedCurrentSHA256: seen, ConfirmOwnerEdit: true})
	if !resp.Written || resp.PreviousState != "edited" || lab.text(t) != renderAutoUpgrades("manual") || !resp.Effective {
		t.Fatalf("confirmed: %+v", resp)
	}
}

func TestAFileThatAppearedWhereNothingWasShownIsNotReplaced(t *testing.T) {
	lab := newModeLab(t)
	os.WriteFile(lab.auto, []byte("APT::Periodic::Unattended-Upgrade \"1\";\n"), 0o644)
	resp := lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual", ExpectedCurrentSHA256: ""})
	if resp.Written || resp.ErrorCode != "CURRENT_CHANGED" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestASymbolicLinkIsTheOwnersArrangementAndIsNotReplaced(t *testing.T) {
	lab := newModeLab(t)
	target := filepath.Join(lab.dir, "elsewhere")
	os.WriteFile(target, []byte("x"), 0o644)
	if err := os.Symlink(target, lab.auto); err != nil {
		t.Skip("symbolic links are not available: ", err)
	}
	resp := lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual", ConfirmOwnerEdit: true})
	if resp.Written || resp.ErrorCode != "NOT_REGULAR" {
		t.Fatalf("resp = %+v", resp)
	}
	if status := osUpdatesAutoConfig(context.Background(), "", nil, nil); status.State != "edited" || !status.Unreadable {
		t.Fatalf("status = %+v", status)
	}
}

func TestAutomaticNeedsUnattendedUpgradesButManualDoesNot(t *testing.T) {
	lab := newModeLab(t)
	os.WriteFile(dpkgStatusPath, []byte("Package: apt\nStatus: install ok installed\nVersion: 3.0\n\n"), 0o644)
	resp := lab.apply(transport.SetOSUpdatesModeRequest{Mode: "auto"})
	if resp.Written || resp.ErrorCode != "UNATTENDED_NOT_INSTALLED" {
		t.Fatalf("auto: %+v", resp)
	}
	if _, err := os.Stat(lab.auto); err == nil {
		t.Fatal("a file was written")
	}
	resp = lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual"})
	if !resp.Written {
		t.Fatalf("manual: %+v", resp)
	}
}

func TestTheInstallersEarlierFileIsRemovedOnlyWhenItIsExactlyWhatItWrote(t *testing.T) {
	lab := newModeLab(t)
	os.WriteFile(lab.legacy, []byte(legacyAutoUpgradesText), 0o644)
	status := osUpdatesAutoConfig(context.Background(), "", map[string]string{"APT::Periodic::Unattended-Upgrade": "1"}, nil)
	if status.LegacyState != "panel" || status.State != "absent" {
		t.Fatalf("status = %+v", status)
	}
	resp := lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual"})
	if !resp.Written || !resp.LegacyRemoved || !resp.Effective {
		t.Fatalf("manual: %+v", resp)
	}
	if _, err := os.Stat(lab.legacy); err == nil {
		t.Fatal("the earlier file is still there and would override the choice")
	}
	// The owner's own edit of that file stays, and the card says it wins.
	lab = newModeLab(t)
	os.WriteFile(lab.legacy, []byte(legacyAutoUpgradesText+"// mine\n"), 0o644)
	resp = lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual"})
	if !resp.Written || resp.LegacyRemoved || resp.Effective || resp.EffectiveReason != "overridden_by_other_file" {
		t.Fatalf("overridden: %+v", resp)
	}
	if _, err := os.Stat(lab.legacy); err != nil {
		t.Fatal("the owner's file was removed")
	}
}

func TestAnotherFileThatWinsIsReportedAsNotEffective(t *testing.T) {
	lab := newModeLab(t)
	lab.dump = "APT::Periodic::Unattended-Upgrade \"0\";\n"
	resp := lab.apply(transport.SetOSUpdatesModeRequest{Mode: "auto"})
	if !resp.Written || resp.Effective || resp.EffectiveReason != "overridden_by_other_file" {
		t.Fatalf("resp = %+v", resp)
	}
	// The card reads the same situation without a write.
	os.WriteFile(lab.auto, []byte(renderAutoUpgrades("auto")), 0o644)
	status := osUpdatesAutoConfig(context.Background(), "", map[string]string{"APT::Periodic::Unattended-Upgrade": "0"}, nil)
	if !status.Overridden {
		t.Fatalf("status = %+v", status)
	}
}

func TestADisabledTimerIsEnabledForAutomaticButAMaskedOneIsLeftAndSaid(t *testing.T) {
	lab := newModeLab(t)
	lab.timerState["apt-daily-upgrade.timer"] = "disabled"
	resp := lab.apply(transport.SetOSUpdatesModeRequest{Mode: "auto"})
	if !resp.Written || !resp.Effective || len(resp.TimersEnabled) != 1 || resp.TimersEnabled[0] != "apt-daily-upgrade.timer" {
		t.Fatalf("disabled: %+v", resp)
	}
	lab = newModeLab(t)
	lab.timerState["apt-daily.timer"] = "masked"
	resp = lab.apply(transport.SetOSUpdatesModeRequest{Mode: "auto"})
	if !resp.Written || resp.Effective || resp.EffectiveReason != "timer_masked" || len(resp.TimersEnabled) != 0 {
		t.Fatalf("masked: %+v", resp)
	}
	for _, command := range lab.commands {
		if strings.Contains(command, "unmask") {
			t.Fatalf("a masked timer was touched: %s", command)
		}
	}
	// Manual never touches the timers.
	lab = newModeLab(t)
	lab.timerState["apt-daily.timer"] = "disabled"
	resp = lab.apply(transport.SetOSUpdatesModeRequest{Mode: "manual"})
	if !resp.Written || len(resp.TimersEnabled) != 0 {
		t.Fatalf("manual: %+v", resp)
	}
}

func TestACoverageListIsReadFromAptAndBounded(t *testing.T) {
	_, lists := parseAptConfigDump(aptDump)
	covers := unattendedCoverage(lists)
	if len(covers) != 3 {
		t.Fatalf("covers = %v", covers)
	}
	many := map[string][]string{"Unattended-Upgrade::Origins-Pattern": {}}
	for i := 0; i < 30; i++ {
		many["Unattended-Upgrade::Origins-Pattern"] = append(many["Unattended-Upgrade::Origins-Pattern"], "origin=x"+string(rune('a'+i%26)))
	}
	if got := unattendedCoverage(many); len(got) != 12 {
		t.Fatalf("not bounded: %d", len(got))
	}
}
