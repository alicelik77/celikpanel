//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/alicelik/celikpanel/internal/hostplatform"
)

// productionSSHPasswordLoginEnv is the real host: /etc/ssh read the way the
// security audit reads it (root-owned, bounded, no links), the verified sshd
// and systemctl, and the distribution's unit name (Debian and Ubuntu: ssh;
// Arch and others: sshd).
func productionSSHPasswordLoginEnv() (sshPasswordLoginEnv, error) {
	sshd, err := trustedSSHDExecutablePath()
	if err != nil {
		return sshPasswordLoginEnv{}, fmt.Errorf("OpenSSH server is not installed or not trusted: %w", err)
	}
	systemctl, err := trustedCommandExecutablePath("systemctl")
	if err != nil {
		return sshPasswordLoginEnv{}, fmt.Errorf("systemctl is not available: %w", err)
	}
	unit := "sshd"
	if profile, err := readHostProfile(); err == nil && profile.PackageManager == hostplatform.PackageManagerAPT {
		unit = "ssh"
	}
	run := func(binary string) func(ctx context.Context, args ...string) ([]byte, error) {
		return func(ctx context.Context, args ...string) ([]byte, error) {
			return serviceMutationCommand(ctx, binary, args...).CombinedOutputLimited(2 << 20)
		}
	}
	return sshPasswordLoginEnv{
		root:       "/etc/ssh",
		readConfig: readPinnedSecurityAuditSSHDFile,
		glob:       filepath.Glob,
		dirExists: func(path string) bool {
			info, err := os.Lstat(path)
			return err == nil && info.IsDir()
		},
		sshd:      run(sshd),
		systemctl: run(systemctl),
		unit:      unit,
		passwd:    func() ([]byte, error) { return readBoundedRegular("/etc/passwd", 4<<20) },
		group:     func() ([]byte, error) { return readBoundedRegular("/etc/group", 4<<20) },
		keyFile:   readAuthorizedKeysFile,
		write:     publishSSHDropIn,
		read:      readSSHDropIn,
		remove:    removeSSHDropIn,
	}, nil
}

func readBoundedRegular(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, fmt.Errorf("%s is larger than expected", path)
	}
	return raw, nil
}

// readAuthorizedKeysFile reads an authorized_keys file the way sshd would
// accept it under StrictModes: a regular file (sshd follows a link, so the
// target counts) owned by the account or root and not writable by group or
// others, in a directory and home directory with the same rule.
func readAuthorizedKeysFile(file, home string, uid int, strict bool) ([]byte, bool) {
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() > sshPasswordLoginKeyFileMax {
		return nil, false
	}
	if strict {
		for _, path := range []string{file, filepath.Dir(file), home} {
			stat, err := os.Stat(path)
			if err != nil {
				return nil, false
			}
			sys, ok := stat.Sys().(*syscall.Stat_t)
			if !ok || (int(sys.Uid) != uid && sys.Uid != 0) || stat.Mode().Perm()&0o022 != 0 {
				return nil, false
			}
		}
	}
	raw, err := readBoundedRegular(file, sshPasswordLoginKeyFileMax)
	if err != nil {
		return nil, false
	}
	return raw, true
}

func readSSHDropIn(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if !info.Mode().IsRegular() {
		return nil, true, fmt.Errorf("%s is not a regular file", path)
	}
	raw, err := readBoundedRegular(path, 64<<10)
	return raw, true, err
}

func sshTempName(path, purpose string) (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	// A dot name without .conf: sshd's "*.conf" Include never reads it.
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+purpose+"-"+hex.EncodeToString(random)), nil
}

// publishSSHDropIn writes the file whole (temporary file, flushed, then
// linked into place); an existing file at path is never replaced.
func publishSSHDropIn(path string, content []byte) error {
	temp, err := sshTempName(path, "new")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	// root:root, like every file sshd reads: the Agent's own group would
	// otherwise be inherited, and the pinned reader (and the security audit)
	// refuses a group-owned sshd file, so the status after "turn off" read
	// "could not be read safely" (lab, alpha.130, Ubuntu 24.04).
	// root:root: aksi hâlde Agent'ın grubu devralınır ve okuyucu reddeder.
	if err := file.Chown(0, 0); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0o644); err != nil {
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
	if err := os.Link(temp, path); err != nil {
		return fmt.Errorf("%s could not be put in place: %w", path, err)
	}
	syncSSHDirectory(filepath.Dir(path))
	return nil
}

// removeSSHDropIn moves the drop-in aside. restore puts it back (never over
// a file that appeared meanwhile); commit deletes the moved copy.
func removeSSHDropIn(path string) (func() error, func(), error) {
	aside, err := sshTempName(path, "restore")
	if err != nil {
		return nil, nil, err
	}
	if err := os.Rename(path, aside); err != nil {
		return nil, nil, err
	}
	syncSSHDirectory(filepath.Dir(path))
	restore := func() error {
		if err := os.Link(aside, path); err != nil {
			return fmt.Errorf("%s could not be put back (the copy is %s): %w", path, aside, err)
		}
		_ = os.Remove(aside)
		syncSSHDirectory(filepath.Dir(path))
		return nil
	}
	commit := func() {
		_ = os.Remove(aside)
		syncSSHDirectory(filepath.Dir(path))
	}
	return restore, commit, nil
}

func syncSSHDirectory(path string) {
	if dir, err := os.Open(path); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
}
