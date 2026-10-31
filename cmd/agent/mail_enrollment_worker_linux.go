//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/hostmutationlock"
	"github.com/alicelik/celikpanel/internal/mailrenewalkit"
	"github.com/alicelik/celikpanel/internal/recoveryruntime"
	"github.com/alicelik/celikpanel/internal/servicemutationledger"
)

// This is explicit owner dispatch, never polling or an update entrypoint. A
// successful handoff means systemd accepted the worker, not enrollment completion.
func launchIndependentMailEnrollment(ctx context.Context, accepted []string) error {
	if !mailRenewalOnlyBuild || os.Geteuid() != 0 || !validMailEnrollmentWorkerArgs(accepted) {
		return servicemutationledger.ErrMailEnrollment
	}
	if len(accepted) == 1 {
		proof, err := inspectRunningRetainedMailEnrollmentHelper()
		if err != nil {
			return err
		}
		defer proof.Close()
		owner := servicemutationledger.FileOwner{UID: serviceMutationRequiredOwnerUID, GID: serviceMutationRequiredOwnerGID}
		ledgerPath := filepath.Join(hostingpath.ServiceMutationStateRoot(), serviceMutationLedgerFileName)
		raw, found, err := servicemutationledger.ReadFile(ledgerPath, servicemutationledger.MaxSize, owner)
		if err != nil || !found {
			return errors.Join(servicemutationledger.ErrMailEnrollment, err)
		}
		id, err := recordedMailEnrollmentRuntimeIdentity(raw, accepted, proof.Generation())
		if err != nil {
			return err
		}
		observed, err := recoveryruntime.ObserveMailEnrollment(ctx, ledgerPath, owner, mailEnrollmentJournalRoot, id.RequestID, id.OwnerID, proof.Generation())
		if err != nil || !observed.Found || observed.Identity != id {
			return errors.Join(servicemutationledger.ErrMailEnrollment, err)
		}
		if err = proof.Revalidate(); err != nil {
			return err
		}
		return dispatchMailEnrollmentHelper(ctx, filepath.Join(mailrenewalkit.InstalledRoot, proof.Generation(), mailrenewalkit.BinaryName), accepted)
	}
	helper, err := verifiedMailEnrollmentHelper()
	if err != nil {
		return err
	}
	return dispatchMailEnrollmentHelper(ctx, helper, accepted)
}

func dispatchMailEnrollmentHelper(ctx context.Context, helper string, accepted []string) error {
	if ctx == nil {
		return servicemutationledger.ErrMailEnrollment
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runner, err := resolveFixedRootExecutable("/usr/bin/systemd-run")
	if err != nil {
		return err
	}
	args, err := mailEnrollmentWorkerUnitArgs(helper, accepted)
	if err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err = runFixedSystemUpdateCommand(bounded, runner, args)
	if err != nil {
		return errors.New("mail enrollment handoff is unconfirmed; inspect the same request's native worker unit before retrying")
	}
	return nil
}

func validMailEnrollmentWorkerArgs(args []string) bool {
	return (len(args) == 1 && validMutationIdentity(args[0])) ||
		(len(args) == 3 && validMutationIdentity(args[0]) && validMutationIdentity(args[1]) && recoveryruntime.ValidDigest(args[2]))
}
func mailEnrollmentWorkerUnitArgs(helper string, accepted []string) ([]string, error) {
	if !validMailEnrollmentWorkerArgs(accepted) || !filepath.IsAbs(helper) || filepath.Clean(helper) != helper || filepath.Base(helper) != mailrenewalkit.BinaryName ||
		filepath.Dir(filepath.Dir(helper)) != mailrenewalkit.InstalledRoot || !recoveryruntime.ValidDigest(filepath.Base(filepath.Dir(helper))) {
		return nil, servicemutationledger.ErrMailEnrollment
	}
	if len(accepted) == 3 && accepted[2] != filepath.Base(filepath.Dir(helper)) {
		return nil, servicemutationledger.ErrMailEnrollment
	}
	args := []string{"--unit=celikpanel-mail-enrollment-" + accepted[0] + ".service", "--property=Type=exec", "--property=User=root", "--property=Group=root", "--property=UMask=0077", "--property=KillMode=control-group", "--property=TimeoutStartSec=30s", "--property=RuntimeMaxSec=3min", "--property=TimeoutStopSec=15s", "--collect", "--no-block", "--", helper, "--enrollment-worker"}
	return append(args, accepted...), nil
}

// The detached worker owns exclusion before executing the consumer. Re-exec via
// a pinned descriptor assigns fd8/fd9 without overwriting Go runtime descriptors.
// Recorded owner continuation can restore absent volatile locks from verified
// durable evidence under the release lock; existing owner paths are preserved.
func runIndependentMailEnrollmentWorker(ctx context.Context, accepted []string) error {
	return runIndependentMailEnrollmentWorkerMode(ctx, accepted, false)
}
func runIndependentMailEnrollmentWorkerMode(ctx context.Context, accepted []string, automatic bool) error {
	if !mailRenewalOnlyBuild || os.Geteuid() != 0 || !validMailEnrollmentWorkerArgs(accepted) || automatic && len(accepted) != 1 {
		return servicemutationledger.ErrMailEnrollment
	}
	gid, ok := lookupGroupID("celikpanel")
	if !ok || gid < 0 || uint32(gid) != serviceMutationRequiredOwnerGID {
		return servicemutationledger.ErrMailEnrollment
	}
	args := []string{"--resume-enrollment", accepted[0]}
	if automatic {
		args[0] = "--boot-enrollment-under-lock"
	}
	if len(accepted) == 3 {
		args = append([]string{"--enroll-under-lock"}, accepted...)
	}
	return runMailEnrollmentWithPreparedLocks(ctx, mailEnrollmentReleaseLockPath, mailEnrollmentHostLockPath, serviceMutationLockOwner(), args, func(release *os.File) error {
		return prepareRecordedMailEnrollmentRuntime(ctx, accepted, int(release.Fd()))
	})
}

const (
	mailEnrollmentReleaseLockPath = "/var/lib/celikpanel-release-transaction/transaction.lock"
	mailEnrollmentHostLockPath    = "/run/celikpanel/service-mutation.lock"
	mailEnrollmentWorkerLockPoll  = 250 * time.Millisecond
)

// mailEnrollmentWorkerLockWait bounds how long a worker waits for the two
// locks while another task holds one of them (2026-10-30, Arch run ap-e: a
// worker started while the host lock was briefly held and gave up at once, so
// setup waited for the owner). Every readiness check holds the host lock for an
// instant; a short wait outlasts such holds. Nothing is held between tries and
// nothing changes before both locks are held; past the bound the refusal is the
// same as before.
// İşçi, kilitlerden biri kısa süre tutuluyorsa sınırlı bir süre bekler.
var mailEnrollmentWorkerLockWait = 15 * time.Second

// mailEnrollmentExclusionBusy is the advisory read, just before a handoff, of
// the two locks the worker takes. It holds neither past the read; the worker's
// own acquisition stays authoritative. A lock that cannot be read is not
// reported as busy: that is left to the worker's own verified refusal. The read
// takes no lock (hostmutationlock.ProbeIdle), so it cannot itself make the
// worker or any other writer refuse as busy. When busy, holder names the lock
// and the process holding it.
// İşçinin alacağı iki kilidin devirden hemen önceki salt okunur denetimi.
var mailEnrollmentExclusionBusy = func() (bool, string) {
	for _, lock := range []struct {
		path  string
		owner hostmutationlock.Owner
	}{{mailEnrollmentReleaseLockPath, hostmutationlock.Owner{}}, {mailEnrollmentHostLockPath, serviceMutationLockOwner()}} {
		if errors.Is(hostmutationlock.ProbeIdle(lock.path, lock.owner), hostmutationlock.ErrBusy) {
			return true, (&mailEnrollmentLockHeld{lock.path, mailEnrollmentLockHolder(lock.path, lock.owner)}).Error()
		}
	}
	return false, ""
}

// mailEnrollmentLockHeld names a lock found held and, read from the kernel's
// lock table, who held it (2026-10-30: the Arch run ap-e could not establish
// which operation held the lock when the worker was refused). It is evidence
// for the journal and the setup screen only; no decision depends on it.
// Tutulan kilit ve onu tutan süreç: yalnız günlük ve ekran için kanıt.
type mailEnrollmentLockHeld struct{ lock, holder string }

func (e *mailEnrollmentLockHeld) Error() string {
	name := "the host lock"
	if e.lock == mailEnrollmentReleaseLockPath || filepath.Base(e.lock) == filepath.Base(mailEnrollmentReleaseLockPath) {
		name = "the release lock"
	}
	return name + " (" + e.lock + ") was held by " + e.holder
}

// heldBy lets the platform-neutral worker guidance name the holder.
func (e *mailEnrollmentLockHeld) heldBy() string { return e.Error() }

// mailEnrollmentLockHolder reads who holds a lock, without taking it.
func mailEnrollmentLockHolder(path string, owner hostmutationlock.Owner) string {
	holders, err := hostmutationlock.Holders(path, owner)
	switch {
	case err != nil:
		return "a process that could not be named (" + err.Error() + ")"
	case len(holders) == 0:
		return "a process that released it before it could be named"
	}
	return hostmutationlock.DescribeHolders(holders)
}

func mailEnrollmentLockBusy(path string, owner hostmutationlock.Owner, err error) error {
	return errors.Join(errMailEnrollmentWorkerBusy, err, &mailEnrollmentLockHeld{path, mailEnrollmentLockHolder(path, owner)})
}

// mailEnrollmentLockHolderOf is the named holder carried by a busy refusal.
func mailEnrollmentLockHolderOf(err error) string {
	var held *mailEnrollmentLockHeld
	if errors.As(err, &held) {
		return held.Error()
	}
	return ""
}

// mailEnrollmentWorkerLog writes to the worker unit's journal.
var mailEnrollmentWorkerLog = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func runMailEnrollmentWithLocks(ctx context.Context, releasePath, hostPath string, owner hostmutationlock.Owner, args []string) error {
	return runMailEnrollmentWithPreparedLocks(ctx, releasePath, hostPath, owner, args, nil)
}
func runMailEnrollmentWithPreparedLocks(ctx context.Context, releasePath, hostPath string, owner hostmutationlock.Owner, args []string, prepare func(*os.File) error) error {
	if ctx == nil {
		return servicemutationledger.ErrMailEnrollment
	}
	release, host, err := waitForMailEnrollmentLocks(ctx, releasePath, hostPath, owner, prepare)
	if err != nil {
		return err
	}
	defer release.Close()
	defer host.Close()
	self, err := os.Open("/proc/self/exe")
	if err != nil {
		return err
	}
	defer self.Close()
	command := exec.CommandContext(ctx, "/proc/self/fd/3", args...)
	command.ExtraFiles = []*os.File{self, nil, nil, nil, nil, host, release}
	command.Env = []string{"PATH=" + independentMailPath, "LANG=C", "LC_ALL=C"}
	command.Dir = "/"
	command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	command.WaitDelay = 2 * time.Second
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

// waitForMailEnrollmentLocks retries only a busy refusal, for at most
// mailEnrollmentWorkerLockWait; any other error is returned at once.
func waitForMailEnrollmentLocks(ctx context.Context, releasePath, hostPath string, owner hostmutationlock.Owner, prepare func(*os.File) error) (*os.File, *os.File, error) {
	deadline := time.Now().Add(mailEnrollmentWorkerLockWait)
	for tries := 1; ; tries++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		release, host, err := acquireMailEnrollmentLocks(releasePath, hostPath, owner, prepare)
		busy := errors.Is(err, errMailEnrollmentWorkerBusy)
		switch {
		case err == nil && tries > 1:
			mailEnrollmentWorkerLog("Mail enrollment obtained the release and host locks after %d tries; continuing.", tries)
		case busy && tries == 1:
			mailEnrollmentWorkerLog("Mail enrollment is waiting up to %s and has changed nothing: %s.", mailEnrollmentWorkerLockWait, mailEnrollmentLockHolderOf(err))
		case busy && !time.Now().Before(deadline):
			mailEnrollmentWorkerLog("Mail enrollment stopped waiting after %d tries and changed nothing: %s.", tries, mailEnrollmentLockHolderOf(err))
		}
		if err == nil || !busy || !time.Now().Before(deadline) {
			return release, host, err
		}
		timer := time.NewTimer(mailEnrollmentWorkerLockPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// acquireMailEnrollmentLocks takes the release lock, prepares, then takes the
// host lock. A busy lock releases whatever this attempt took.
func acquireMailEnrollmentLocks(releasePath, hostPath string, owner hostmutationlock.Owner, prepare func(*os.File) error) (*os.File, *os.File, error) {
	release, err := hostmutationlock.AcquireExisting(releasePath, hostmutationlock.Owner{})
	if err != nil {
		if errors.Is(err, hostmutationlock.ErrBusy) {
			return nil, nil, mailEnrollmentLockBusy(releasePath, hostmutationlock.Owner{}, err)
		}
		return nil, nil, err
	}
	if prepare != nil {
		if err = prepare(release); err != nil {
			release.Close()
			return nil, nil, err
		}
	}
	host, err := hostmutationlock.AcquireExisting(hostPath, owner)
	if err != nil {
		release.Close()
		if errors.Is(err, hostmutationlock.ErrBusy) {
			return nil, nil, mailEnrollmentLockBusy(hostPath, owner, err)
		}
		return nil, nil, err
	}
	return release, host, nil
}

// Dispatch only the installed content-addressed helper corresponding to this
// executing image and the current verified Agent declaration. Never execute an
// Agent with a helper flag or accept a caller-supplied executable path.
func verifiedMailEnrollmentHelper() (string, error) {
	agent, err := recoveryruntime.InspectCompatibleMailAgent("/opt/celikpanel/bin")
	if err != nil {
		return "", err
	}
	defer agent.Close()
	self, err := os.Open("/proc/self/exe")
	if err != nil {
		return "", err
	}
	defer self.Close()
	raw, err := io.ReadAll(io.LimitReader(self, mailrenewalkit.MaxBinarySize+1))
	if err != nil {
		return "", err
	}
	kit, _, err := mailrenewalkit.Payload(raw)
	if err != nil || kit.Generation != agent.Contract.MailRenewalGeneration {
		return "", servicemutationledger.ErrMailEnrollment
	}
	path := filepath.Join(mailrenewalkit.InstalledRoot, kit.Generation, mailrenewalkit.BinaryName)
	path, err = resolveFixedRootExecutable(path)
	if err != nil {
		return "", err
	}
	running, err := self.Stat()
	if err != nil {
		return "", err
	}
	named, err := os.Lstat(path)
	if err != nil || !os.SameFile(running, named) {
		return "", servicemutationledger.ErrMailEnrollment
	}
	if err = agent.Revalidate(); err != nil {
		return "", err
	}
	return path, nil
}

// Proves the actual executing retained helper without consulting ordinary
// management files. This is a source proof only, never fresh owner admission.
func inspectRunningRetainedMailEnrollmentHelper() (*recoveryruntime.RetainedMailEnrollmentHelper, error) {
	if !mailRenewalOnlyBuild || os.Geteuid() != 0 {
		return nil, servicemutationledger.ErrMailEnrollment
	}
	self, err := os.Open("/proc/self/exe")
	if err != nil {
		return nil, err
	}
	defer self.Close()
	raw, err := io.ReadAll(io.LimitReader(self, mailrenewalkit.MaxBinarySize+1))
	if err != nil {
		return nil, err
	}
	kit, _, err := mailrenewalkit.Payload(raw)
	if err != nil {
		return nil, err
	}
	proof, err := recoveryruntime.InspectRetainedMailEnrollmentHelper(mailrenewalkit.InstalledRoot, kit.Generation)
	if err != nil {
		return nil, err
	}
	if err = proof.VerifyExecutable(self); err != nil {
		proof.Close()
		return nil, err
	}
	return proof, nil
}
