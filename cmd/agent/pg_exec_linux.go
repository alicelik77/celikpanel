//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The host side of the PostgreSQL clusters (alpha.108). Every PostgreSQL
// client runs as the cluster's own owner (peer authentication on the local
// socket), as that user directly (no sudo, no shell), in its own process
// group so a bound kills it whole, with a fixed environment. Statements are
// on standard input, never in the argument list (R-062).
//
// PostgreSQL istemcileri kümenin kendi sahibi olarak, kabuksuz, sınırlı ve
// ifadeler standart girdiden çalışır.

var pgLsclustersPath = "/usr/bin/pg_lsclusters"

// pgClientBinary is the newest installed major's client: a newer client
// reads and dumps an older server; an older one refuses a newer server.
func pgClientBinary(name string) (string, error) {
	entries, err := os.ReadDir(pgLibRoot)
	if err != nil {
		return "", fmt.Errorf("no PostgreSQL client is installed (%s)", pgLibRoot)
	}
	var majors []int
	for _, entry := range entries {
		if major, err := strconv.Atoi(entry.Name()); err == nil && entry.IsDir() {
			if info, err := os.Stat(filepath.Join(pgLibRoot, entry.Name(), "bin", name)); err == nil && info.Mode().IsRegular() {
				majors = append(majors, major)
			}
		}
	}
	if len(majors) == 0 {
		return "", fmt.Errorf("no PostgreSQL %s is installed under %s", name, pgLibRoot)
	}
	sort.Ints(majors)
	return filepath.Join(pgLibRoot, strconv.Itoa(majors[len(majors)-1]), "bin", name), nil
}

// pgCommandAs prepares one PostgreSQL client command run as owner.
func pgCommandAs(ctx context.Context, owner string, binary string, args ...string) (*exec.Cmd, error) {
	if !pgOwnerRe.MatchString(owner) {
		return nil, fmt.Errorf("the cluster owner %q is not an account CelikPanel runs as", owner)
	}
	account, err := user.Lookup(owner)
	if err != nil {
		return nil, fmt.Errorf("the cluster owner %s is not a user here: %w", owner, err)
	}
	uid, err1 := strconv.ParseUint(account.Uid, 10, 32)
	gid, err2 := strconv.ParseUint(account.Gid, 10, 32)
	if err1 != nil || err2 != nil || uid == 0 {
		return nil, fmt.Errorf("the cluster owner %s cannot be run as", owner)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = "/"
	cmd.Env = []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "LANG=C",
		"HOME=" + account.HomeDir, "USER=" + owner, "PGCONNECT_TIMEOUT=10",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:    true,
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{}},
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 5 * time.Second
	return cmd, nil
}

func pgCommandError(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return err
	}
	lines := strings.Split(stderr, "\n")
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return fmt.Errorf("%v: %s", err, boundedText(strings.Join(lines, " "), 300))
}

func hostPGQuery(ctx context.Context, owner string, port int, database, statement string) (string, error) {
	if port < 1 || port > 65535 || !pgDatabaseNameRe.MatchString(database) {
		return "", fmt.Errorf("invalid port or database")
	}
	psql, err := pgClientBinary("psql")
	if err != nil {
		return "", err
	}
	cmd, err := pgCommandAs(ctx, owner, psql,
		"--no-psqlrc", "--no-align", "--tuples-only", "--quiet", "--no-password",
		"--set", "ON_ERROR_STOP=on", "--port", strconv.Itoa(port), "--dbname", database)
	if err != nil {
		return "", err
	}
	cmd.Stdin = strings.NewReader(statement + "\n")
	stdout := &pgBuffer{limit: 4 << 20}
	stderr := &pgBuffer{limit: 16 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", pgCommandError(err, stderr.String())
	}
	return stdout.String(), nil
}

// hostPGStream runs one client (pg_dump, pg_restore, pg_dumpall) as owner
// with stdin and stdout as given.
func hostPGStream(ctx context.Context, owner, client string, stdin io.Reader, stdout io.Writer, args ...string) error {
	binary, err := pgClientBinary(client)
	if err != nil {
		return err
	}
	cmd, err := pgCommandAs(ctx, owner, binary, args...)
	if err != nil {
		return err
	}
	stderr := &pgBuffer{limit: 16 << 10}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s did not finish within its time: %w", client, ctx.Err())
		}
		return pgCommandError(err, stderr.String())
	}
	return nil
}

func hostPGLsclusters(ctx context.Context) (string, error) {
	if info, err := os.Stat(pgLsclustersPath); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: %w", pgLsclustersPath, errPGLsclustersMissing)
	}
	out, err := runPackageSimulationCommand(ctx, packageCommandEnv(), pgLsclustersPath, "--no-header")
	if err != nil {
		return "", pgCommandError(err, string(out))
	}
	return string(out), nil
}

func hostPGRestart(ctx context.Context, version, name string) error {
	if !validPGClusterRef(version, name) {
		return fmt.Errorf("invalid cluster")
	}
	profile, err := detectHostPlatform()
	if err != nil {
		return err
	}
	systemctl, err := executableForProfile(profile, string(profile.PackageManager), "systemctl")
	if err != nil {
		return err
	}
	unit := "postgresql@" + version + "-" + name + ".service"
	out, err := runServiceMutationCombinedOutput(ctx, systemctl, "restart", unit)
	if err != nil {
		return errors.New(commandFailureDetail("systemctl restart "+unit, out, err))
	}
	return nil
}

// hostPGWriteConf writes a cluster's configuration over the existing file,
// keeping its mode and owner (no-follow, atomic).
func hostPGWriteConf(path string, content []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return secureWriteConfigOwnedBy(path, content, info.Mode().Perm(), stat.Uid, stat.Gid)
}

func hostPGFamily() string {
	profile, err := detectHostPlatform()
	if err != nil {
		return ""
	}
	return string(profile.PackageManager)
}

func hostPGVendorIndex() (map[string]map[string]bool, bool) {
	loc, ok := enabledRepoLocations()["pgdg"]
	if !ok {
		return nil, false
	}
	index, err := readVendorPackageIndex(loc)
	if err != nil {
		return nil, true
	}
	return index, true
}

func defaultPGClusterHost() pgClusterHost {
	return pgClusterHost{
		family:         hostPGFamily,
		lsclusters:     hostPGLsclusters,
		query:          hostPGQuery,
		restart:        hostPGRestart,
		readFile:       secureReadConfig,
		writeFile:      hostPGWriteConf,
		listening:      hostListeningPorts,
		distroVersions: readDistributionVersions,
		vendorIndex:    hostPGVendorIndex,
		installed: func() map[string][]string {
			installed, _ := readDpkgStatusVersions(dpkgStatusPath)
			return installed
		},
		now:       time.Now,
		stream:    hostPGStream,
		freeBytes: hostFreeBytes,
	}
}

func hostFreeBytes(path string) (int64, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, err
	}
	return int64(fs.Bavail) * int64(fs.Bsize), nil
}
