package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClamAV's first start (alpha.118 lab, rounds 1 and 2).
//
// The distributions' clamav-daemon.service AND clamav-daemon.socket both
// carry two conditions (read from the packages, not assumed): Debian 13
// clamav-daemon 1.4.3+dfsg-1, Ubuntu 22.04/24.04 (the same Debian packaging)
// and Arch Linux (upstream's clamd/clamav-daemon.{service,socket}.in, which
// Arch installs unchanged):
//
//	ConditionPathExistsGlob=/var/lib/clamav/main.{c[vl]d,inc}
//	ConditionPathExistsGlob=/var/lib/clamav/daily.{c[vl]d,inc}
//
// and the service Requires= the socket. freshclam downloads daily first and
// main (about 90 MB, 160 MB unpacked) after it. Round 2 started the daemon as
// soon as daily was there: systemd skipped the daemon AND the socket on the
// main condition, a skipped socket never starts again by itself, freshclam
// logged "Clamd was NOT notified", and the daemon stayed inactive for good.
//
// Now:
//   - the install waits (bounded, sized for the whole first download) until
//     every file both units require is there, then starts the socket and the
//     daemon explicitly: a new start re-evaluates the conditions;
//   - when the download outlasts the wait, the install ends "installed,
//     waiting for the first signatures" and leaves a standard systemd path
//     unit that starts the daemon when the files are complete. It is plain
//     systemd, visible in /etc/systemd/system, needs no CelikPanel (D-022),
//     and disables itself after its one start;
//   - the generic Start action in Components (and "systemctl start
//     clamav-daemon") still works by hand at any time.
// Nothing here runs from a status read.
//
// ClamAV'ın ilk başlatılması: dağıtımların daemon VE soket birimleri hem main
// hem daily imzalarını ister. Kurulum ikisi de inene dek sınırlı süre bekler,
// sonra soketi ve daemon'u açıkça başlatır. İndirme daha uzun sürerse kurulum
// "imzalar bekleniyor" der ve imzalar tamamlanınca daemon'u bir kez başlatan,
// sonra kendini kapatan standart bir systemd path birimi bırakır; panel
// gerekmez. Durum okuma hiçbir şey başlatmaz.

const (
	clamAVDaemonSocket          = "clamav-daemon.socket"
	clamAVFirstStartPathUnit    = "celikpanel-clamav-first-start.path"
	clamAVFirstStartServiceUnit = "celikpanel-clamav-first-start.service"
	clamAVFirstStartHeader      = "# Managed by CelikPanel: starts ClamAV once after its first signatures. sha256="
)

var (
	// About 115 MB of signatures (daily ~25 MB, main ~90 MB, bytecode) plus
	// freshclam's test of each: six minutes is about 330 KB/s, enough for a
	// slow mirror without holding the install for long; a longer download
	// is handed to the path unit below.
	clamAVSignatureWait = 6 * time.Minute
	clamAVSignaturePoll = 2 * time.Second
	clamAVUnitDir       = "/etc/systemd/system"
	clamAVEnableUnit    = enableServiceForMutationWithExecutable
	clamAVUnitExists    = unitExistsWithExecutable
	clamAVUnitFileFound = func(systemctl, name string) bool {
		out, err := serviceMutationCommand(context.Background(), systemctl, "list-unit-files", name, "--no-legend").Output()
		return err == nil && strings.TrimSpace(string(out)) != ""
	}
	clamAVSignaturesPresent = func() bool { return clamAVSignaturesReady(clamAVDatabaseDir) }
	clamAVSystemctl         = func(ctx context.Context, systemctl string, args ...string) ([]byte, error) {
		return runServiceMutationCombinedOutput(ctx, systemctl, args...)
	}
	clamAVWriteUnit = func(path string, content []byte) error {
		return secureWriteConfig(path, content, 0o644)
	}
)

// clamAVSignatureSets are the two globs of the units' conditions:
// <set>.cvd, <set>.cld or the <set>.inc folder.
var clamAVSignatureSets = []string{"main", "daily"}

// clamAVSignaturesReady: every signature set the daemon's units require is
// on disk, so systemd will not skip their start.
func clamAVSignaturesReady(dir string) bool {
	for _, set := range clamAVSignatureSets {
		if !clamAVSignatureSetPresent(dir, set) {
			return false
		}
	}
	return true
}

func clamAVSignatureSetPresent(dir, set string) bool {
	for _, ext := range []string{".cvd", ".cld"} {
		if info, err := os.Stat(filepath.Join(dir, set+ext)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	// The .inc form is a folder of incremental updates.
	if info, err := os.Stat(filepath.Join(dir, set+".inc")); err == nil && info.IsDir() {
		return true
	}
	return false
}

// clamAVAwaitFirstSignatures starts the signature updater (the helper
// units) and waits up to clamAVSignatureWait for the first signatures. It
// reports whether they are complete; an updater that cannot start is an
// error.
func clamAVAwaitFirstSignatures(ctx context.Context, systemctl string, helpers []string) (bool, error) {
	if clamAVSignaturesPresent() {
		return true, nil
	}
	for _, helper := range helpers {
		if !clamAVUnitExists(systemctl, helper) {
			continue
		}
		if err := clamAVEnableUnit(ctx, systemctl, helper, true); err != nil {
			return false, err
		}
	}
	deadline := time.Now().Add(clamAVSignatureWait)
	for {
		if clamAVSignaturesPresent() {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			// The install was cancelled: whatever freshclam does next, the
			// result is not "ready".
			return false, nil
		case <-time.After(clamAVSignaturePoll):
		}
	}
}

// clamAVStartWithSignatures starts the distribution's socket, then the
// daemon, now that their conditions hold. An explicit start re-evaluates the
// conditions, also after an earlier start skipped them. A first-start path
// unit left by an earlier install is retired: the daemon runs.
func clamAVStartWithSignatures(ctx context.Context, systemctl, unit string) (string, error) {
	if clamAVUnitFileFound(systemctl, clamAVDaemonSocket) {
		if err := clamAVEnableUnit(ctx, systemctl, clamAVDaemonSocket, true); err != nil {
			return "", fmt.Errorf("the ClamAV socket could not be started: %w", err)
		}
	}
	if err := clamAVEnableUnit(ctx, systemctl, unit, true); err != nil {
		return "", err
	}
	return clamAVRetireFirstStart(ctx, systemctl), nil
}

// clamAVArmFirstStart is the install's answer when the first download
// outlasts the wait: the daemon and its socket are enabled (not started:
// systemd would skip them), and the path unit starts them once the
// signatures are complete. waiting is false when the signatures completed
// meanwhile and the daemon was started here. note names an owner-changed
// unit file that was kept.
func clamAVArmFirstStart(ctx context.Context, systemctl, unit string) (waiting bool, note string, err error) {
	if err := clamAVEnableUnit(ctx, systemctl, unit, false); err != nil {
		return false, "", err
	}
	socket := clamAVUnitFileFound(systemctl, clamAVDaemonSocket)
	if socket {
		if err := clamAVEnableUnit(ctx, systemctl, clamAVDaemonSocket, false); err != nil {
			return false, "", fmt.Errorf("the ClamAV socket could not be enabled: %w", err)
		}
	}
	note, err = clamAVWriteFirstStartUnits(ctx, systemctl, unit, socket)
	if err != nil {
		return false, "", err
	}
	if out, err := clamAVSystemctl(ctx, systemctl, "enable", "--now", clamAVFirstStartPathUnit); err != nil {
		return false, "", fmt.Errorf("the unit that starts ClamAV after its first signatures could not be turned on: %v: %s", err, strings.TrimSpace(string(out)))
	}
	// The download may have completed between the last check and the path
	// unit's start, and a path unit sees only later changes: look once more.
	if clamAVSignaturesPresent() {
		retired, err := clamAVStartWithSignatures(ctx, systemctl, unit)
		if err != nil {
			return false, "", err
		}
		return false, joinNotes(note, retired), nil
	}
	return true, note, nil
}

func clamAVSeal(body string) string {
	sum := sha256.Sum256([]byte(body))
	return clamAVFirstStartHeader + hex.EncodeToString(sum[:]) + "\n" + body
}

// clamAVSealIntact: the file is exactly as CelikPanel wrote it.
func clamAVSealIntact(content []byte) bool {
	first, body, ok := strings.Cut(string(content), "\n")
	if !ok || !strings.HasPrefix(first, clamAVFirstStartHeader) {
		return false
	}
	sum := sha256.Sum256([]byte(body))
	return strings.TrimPrefix(first, clamAVFirstStartHeader) == hex.EncodeToString(sum[:])
}

// renderClamAVFirstStartPath watches the signature folder; freshclam moves
// each finished database into it, so every new file triggers one check.
func renderClamAVFirstStartPath() string {
	return clamAVSeal(`# Starts the ClamAV daemon once its first virus signatures are complete,
# then turns itself off. Standard systemd; it works without CelikPanel and
# can be removed (systemctl disable --now ` + clamAVFirstStartPathUnit + `).
[Unit]
Description=Start ClamAV when its first virus signatures are complete
Documentation=man:systemd.path(5)

[Path]
PathChanged=` + clamAVDatabaseDir + `
Unit=` + clamAVFirstStartServiceUnit + `

[Install]
WantedBy=paths.target
`)
}

// renderClamAVFirstStartService checks the daemon's own two conditions
// (main.{cvd,cld,inc} and daily.{cvd,cld,inc}), so it starts the daemon only
// when systemd will not skip it. The checks are ExecCondition= lines, not
// [Unit] Condition*=: a skipped ExecCondition still passes through a state
// change, so the path unit goes back to watching; a [Unit] condition leaves
// the unit inactive without one. It starts nothing the owner disabled
// meanwhile, and it turns the path unit off after its start so a later
// owner's stop is never undone.
func renderClamAVFirstStartService(systemctl, unit string, socket bool) string {
	daemon := strings.TrimSuffix(unit, ".service") + ".service"
	targets := daemon
	if socket {
		targets = clamAVDaemonSocket + " " + daemon
	}
	return clamAVSeal(`# Run by ` + clamAVFirstStartPathUnit + `: starts the ClamAV daemon once its first
# virus signatures are complete (the same files clamav-daemon's own
# ConditionPathExistsGlob= lines require), then turns the path unit off.
# Standard systemd; it works without CelikPanel.
[Unit]
Description=Start ClamAV after its first virus signatures

[Service]
Type=oneshot
ExecCondition=/bin/sh -c 'for set in main daily; do [ -f ` + clamAVDatabaseDir + `/$${set}.cvd ] || [ -f ` + clamAVDatabaseDir + `/$${set}.cld ] || [ -d ` + clamAVDatabaseDir + `/$${set}.inc ] || exit 1; done'
ExecCondition=` + systemctl + ` -q is-enabled ` + daemon + `
ExecStart=` + systemctl + ` --no-block start ` + targets + `
ExecStartPost=` + systemctl + ` --no-block disable --now ` + clamAVFirstStartPathUnit + `
`)
}

// clamAVWriteFirstStartUnits writes both units unless the owner changed
// them (then they are kept, and said). Identical files are not rewritten.
func clamAVWriteFirstStartUnits(ctx context.Context, systemctl, unit string, socket bool) (string, error) {
	notes := []string{}
	changed := false
	for _, file := range []struct{ name, want string }{
		{clamAVFirstStartServiceUnit, renderClamAVFirstStartService(systemctl, unit, socket)},
		{clamAVFirstStartPathUnit, renderClamAVFirstStartPath()},
	} {
		path := filepath.Join(clamAVUnitDir, file.name)
		current, err := os.ReadFile(path)
		switch {
		case err == nil && string(current) == file.want:
			continue
		case err == nil && !clamAVSealIntact(current):
			notes = append(notes, path+" was changed outside CelikPanel and was kept as it is")
			continue
		case err != nil && !errors.Is(err, os.ErrNotExist):
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		if err := clamAVWriteUnit(path, []byte(file.want)); err != nil {
			return "", fmt.Errorf("write %s: %w", path, err)
		}
		changed = true
	}
	if changed {
		if out, err := clamAVSystemctl(ctx, systemctl, "daemon-reload"); err != nil {
			return "", fmt.Errorf("systemctl daemon-reload: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return strings.Join(notes, "; "), nil
}

// clamAVRetireFirstStart turns the path unit off and removes the two files
// CelikPanel wrote (never ones the owner changed). Best effort: a failure
// is a note, the daemon already runs or was removed.
func clamAVRetireFirstStart(ctx context.Context, systemctl string) string {
	notes := []string{}
	present := false
	removed := false
	for _, name := range []string{clamAVFirstStartPathUnit, clamAVFirstStartServiceUnit} {
		path := filepath.Join(clamAVUnitDir, name)
		current, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		present = true
		if !clamAVSealIntact(current) {
			notes = append(notes, path+" was changed outside CelikPanel and was kept as it is")
			continue
		}
		if name == clamAVFirstStartPathUnit {
			if out, err := clamAVSystemctl(ctx, systemctl, "disable", "--now", name); err != nil {
				// Its service stays too, so the path unit never names a
				// missing unit.
				notes = append(notes, name+" could not be turned off: "+strings.TrimSpace(string(out)))
				break
			}
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			notes = append(notes, path+" could not be removed: "+err.Error())
			continue
		}
		removed = true
	}
	if present && removed {
		if out, err := clamAVSystemctl(ctx, systemctl, "daemon-reload"); err != nil {
			notes = append(notes, "systemctl daemon-reload: "+strings.TrimSpace(string(out)))
		}
	}
	return strings.Join(notes, "; ")
}
