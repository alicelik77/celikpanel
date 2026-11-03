package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ClamAV's first start (alpha.118 lab, D6). Debian's clamav-daemon.service
// carries ConditionPathExistsGlob=/var/lib/clamav/daily.{c[vl]d,inc}: until
// freshclam has downloaded the first signatures, systemd skips the start.
// The install then read "did not become active" and reported a failure
// although nothing was wrong: the download was still running.
//
// The install now starts freshclam first and waits a bounded time for the
// signatures. When they arrive the daemon starts as before. When they do not
// arrive in time the install finishes as "installed, waiting for the first
// signatures": the daemon is enabled, and the distribution's own
// clamav-daemon.socket is started, so freshclam's own notification
// (NotifyClamd in Debian's freshclam.conf) connects to it and systemd starts
// the daemon once the signatures are on disk. Native systemd behaviour, no
// CelikPanel timer: it resumes without the panel (D-022).
//
// ClamAV'ın ilk başlatılması: imzalar inene dek systemd daemon'u atlar.
// Kurulum önce freshclam'ı başlatır ve imzaları sınırlı süre bekler; gelmezse
// "kuruldu, ilk imzalar bekleniyor" der, daemon'u etkinleştirir ve dağıtımın
// kendi soketini açar; freshclam indirmeyi bitirince sokete bağlanır ve systemd
// daemon'u kendiliğinden başlatır.

const clamAVDaemonSocket = "clamav-daemon.socket"

var (
	clamAVSignatureWait = 2 * time.Minute
	clamAVSignaturePoll = time.Second
	clamAVEnableUnit    = enableServiceForMutationWithExecutable
	clamAVUnitExists    = unitExistsWithExecutable
	clamAVUnitFileFound = func(systemctl, name string) bool {
		out, err := serviceMutationCommand(context.Background(), systemctl, "list-unit-files", name, "--no-legend").Output()
		return err == nil && strings.TrimSpace(string(out)) != ""
	}
	clamAVSignaturesPresent = func() bool { return clamAVSignaturesReady(clamAVDatabaseDir) }
)

// clamAVAwaitFirstSignatures starts the signature updater (the helper
// units) and waits up to clamAVSignatureWait for the first signatures. It
// reports whether they are there; an updater that cannot start is an error.
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

// clamAVArmFirstStart enables the daemon without starting it (systemd would
// skip the start) and starts the distribution's socket so freshclam's
// notification starts the daemon when the signatures are in. Without a
// socket unit the daemon starts at the next boot or by hand.
func clamAVArmFirstStart(ctx context.Context, systemctl, unit string) error {
	if err := clamAVEnableUnit(ctx, systemctl, unit, false); err != nil {
		return err
	}
	if !clamAVUnitFileFound(systemctl, clamAVDaemonSocket) {
		return nil
	}
	if err := clamAVEnableUnit(ctx, systemctl, clamAVDaemonSocket, true); err != nil {
		return fmt.Errorf("the ClamAV socket could not be started: %w", err)
	}
	return nil
}
