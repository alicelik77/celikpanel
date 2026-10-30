//go:build linux && acceptance_license && !dns_kill_matrix

// This file compiles only with the acceptance_license test tag, which the lab
// uses for disposable QEMU acceptance guests
// (deploy/e2e/dns-pair-acceptance/scripts/build-dist.sh --acceptance-license).
// Release packaging refuses every program built with that tag
// (deploy/release-acceptance-license-guard.sh), so no CelikPanel release
// carries this cut.
//
// It makes a power cut land exactly at one DNS switch journal boundary instead
// of on a timer (Arch PowerDNS follow-up, 2026-10-30: the timed cuts of runs
// f and g landed after the unit was enabled, so the pre-start undo on btrfs
// had only unit-level proof). It acts only on a guest carrying the acceptance
// fixture marker and a root-owned arm file naming the boundary; it fires once,
// records what it observed, flushes, and resets the machine without a clean
// shutdown, exactly like pulling the power.
//
// Bu dosya yalnız acceptance_license test etiketiyle derlenir; sürüm paketleme
// bu etiketle derlenmiş her programı reddeder. Kesintiyi zamanlayıcı yerine
// tam bir günlük sınırına düşürür; yalnız deneme konuğunda ve kurulmuş bir
// tetik dosyasıyla, bir kez çalışır.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	acceptanceCutGuestMarker = "/etc/celikpanel-dns-kill-matrix"
	acceptanceCutArmPath     = "/etc/celikpanel-acceptance-dns-cut"
	acceptanceCutRecordPath  = "/var/lib/celikpanel-acceptance-dns-cut.json"
	acceptanceCutSchema      = "celikpanel-acceptance-dns-cut/v1"
)

type acceptanceCutSpec struct {
	Driver string
	Point  string
	Phase  string
	Action string // "reset" (power cut) or "kill" (SIGKILL the Agent only)
}

type acceptanceCutRecord struct {
	Schema            string `json:"schema"`
	Driver            string `json:"driver"`
	Point             string `json:"point"`
	Phase             string `json:"phase"`
	Action            string `json:"action"`
	JournalSchema     string `json:"journal_schema"`
	MutationRequestID string `json:"mutation_request_id"`
	ManifestQualifier string `json:"manifest_qualifier"`
	PID               int    `json:"pid"`
	RecordedAt        string `json:"recorded_at"`
}

type acceptanceCutOps struct {
	guestMarker func() error
	readArm     func() (acceptanceCutSpec, bool, error)
	disarm      func() error
	record      func(acceptanceCutRecord) error
	sync        func()
	reset       func() error
	kill        func() error
	now         func() time.Time
}

func init() {
	if mailRenewalOnlyBuild {
		return
	}
	ops := acceptanceCutDefaultOps()
	if ops.guestMarker() != nil {
		return
	}
	dnsEngineSwitchJournalFaultHook = acceptanceCutHook(ops)
}

func acceptanceCutDefaultOps() acceptanceCutOps {
	return acceptanceCutOps{
		guestMarker: acceptanceCutCheckGuestMarker,
		readArm:     acceptanceCutReadArm,
		disarm: func() error {
			if err := os.Remove(acceptanceCutArmPath); err != nil {
				return err
			}
			return acceptanceCutSyncDir(filepath.Dir(acceptanceCutArmPath))
		},
		record: acceptanceCutWriteRecord,
		sync:   unix.Sync,
		reset:  func() error { return unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART) },
		kill:   func() error { return unix.Kill(os.Getpid(), unix.SIGKILL) },
		now:    time.Now,
	}
}

// acceptanceCutHook fires at most once per arm file. A missing or invalid arm
// file never fires; an armed cut that cannot record or disarm refuses the
// journal write instead of continuing past the named boundary.
func acceptanceCutHook(ops acceptanceCutOps) func(string, string, dnsEngineSwitchJournal) error {
	return func(driver, point string, journal dnsEngineSwitchJournal) error {
		spec, armed, err := ops.readArm()
		if err != nil {
			return fmt.Errorf("acceptance DNS cut arm file is invalid: %w", err)
		}
		if !armed || spec.Driver != driver || spec.Point != point || spec.Phase != journal.Phase {
			return nil
		}
		record := acceptanceCutRecord{
			Schema: acceptanceCutSchema, Driver: driver, Point: point, Phase: journal.Phase,
			Action: spec.Action, JournalSchema: journal.Schema,
			MutationRequestID: journal.MutationRequestID, ManifestQualifier: journal.ManifestQualifier,
			PID: os.Getpid(), RecordedAt: ops.now().UTC().Format(time.RFC3339Nano),
		}
		if err := ops.record(record); err != nil {
			return fmt.Errorf("acceptance DNS cut could not record its boundary: %w", err)
		}
		if err := ops.disarm(); err != nil {
			return fmt.Errorf("acceptance DNS cut could not disarm: %w", err)
		}
		log.Printf("acceptance DNS cut fires at %s %s %s (%s)", driver, point, journal.Phase, spec.Action)
		ops.sync()
		if spec.Action == "reset" {
			if err := ops.reset(); err != nil {
				log.Printf("acceptance DNS cut reset failed, killing the Agent instead: %v", err)
			}
		}
		if err := ops.kill(); err != nil {
			return fmt.Errorf("acceptance DNS cut could not stop the Agent: %w", err)
		}
		// Never continue past the boundary.
		for {
			time.Sleep(time.Hour)
		}
	}
}

func acceptanceCutCheckGuestMarker() error {
	info, err := os.Lstat(acceptanceCutGuestMarker)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || st.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
		return errors.New("acceptance guest marker is not a root-owned regular file")
	}
	return nil
}

// acceptanceCutReadArm reads "<driver> <point> <phase> <reset|kill>" from a
// root-owned 0600 regular file.
func acceptanceCutReadArm() (acceptanceCutSpec, bool, error) {
	info, err := os.Lstat(acceptanceCutArmPath)
	if errors.Is(err, os.ErrNotExist) {
		return acceptanceCutSpec{}, false, nil
	}
	if err != nil {
		return acceptanceCutSpec{}, false, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || st.Uid != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > 256 {
		return acceptanceCutSpec{}, false, errors.New("arm file must be a small root-owned 0600 regular file")
	}
	raw, err := os.ReadFile(acceptanceCutArmPath)
	if err != nil {
		return acceptanceCutSpec{}, false, err
	}
	return parseAcceptanceCutSpec(string(raw))
}

func parseAcceptanceCutSpec(raw string) (acceptanceCutSpec, bool, error) {
	fields := strings.Fields(raw)
	if len(fields) != 4 {
		return acceptanceCutSpec{}, false, errors.New("arm file needs driver, point, phase and action")
	}
	spec := acceptanceCutSpec{Driver: fields[0], Point: fields[1], Phase: fields[2], Action: fields[3]}
	switch spec.Driver {
	case dnsEngineSwitchFaultDriverBIND, dnsEngineSwitchFaultDriverPDNSSwitch,
		dnsEngineSwitchFaultDriverPDNSAdopt, dnsEngineSwitchFaultDriverPDNSSecondaryReconfigure:
	default:
		return acceptanceCutSpec{}, false, errors.New("unknown driver")
	}
	if spec.Point != dnsEngineSwitchJournalFaultBeforeWrite && spec.Point != dnsEngineSwitchJournalFaultAfterWrite {
		return acceptanceCutSpec{}, false, errors.New("unknown point")
	}
	if !validDNSSwitchPhase(spec.Phase) {
		return acceptanceCutSpec{}, false, errors.New("unknown journal phase")
	}
	if spec.Action != "reset" && spec.Action != "kill" {
		return acceptanceCutSpec{}, false, errors.New("action must be reset or kill")
	}
	return spec, true, nil
}

func acceptanceCutWriteRecord(record acceptanceCutRecord) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(acceptanceCutRecordPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return acceptanceCutSyncDir(filepath.Dir(acceptanceCutRecordPath))
}

func acceptanceCutSyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
