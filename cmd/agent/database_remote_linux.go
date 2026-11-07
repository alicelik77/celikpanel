//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// The engines' clients for remote access, on Linux: psql as the cluster's
// owner (the newest installed client on Debian's layout, the one on PATH
// elsewhere) and mysql as the engine's root over its socket. Statements go
// on standard input, never in the argument list (R-062).

func init() {
	databaseRemoteLockFilter = hostDatabaseRemoteLockFilter
	databaseRemotePGQuery = hostDatabaseRemotePGQuery
	databaseRemoteMySQLQuery = hostDatabaseRemoteMySQLQuery
	databaseRemoteFail2ban.active = func() bool {
		_, err := databaseRemoteRun("systemctl", []string{"is-active", "--quiet", "fail2ban"}, "")
		return err == nil
	}
	databaseRemoteFail2ban.jailActive = func(name string) bool {
		_, err := databaseRemoteRun("fail2ban-client", []string{"status", name}, "")
		return err == nil
	}
	databaseRemoteFail2ban.reload = func() error {
		out, err := databaseRemoteRun("fail2ban-client", []string{"reload"}, "")
		if err != nil {
			return errors.New("fail2ban-client reload: " + firstLine(string(out)))
		}
		return nil
	}
}

func hostDatabaseRemotePGQuery(port int, statement string) (string, error) {
	psql, err := pgClientBinary("psql")
	if err != nil {
		if psql, err = exec.LookPath("psql"); err != nil {
			return "", errors.New("psql is not installed")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd, err := pgCommandAs(ctx, "postgres", psql,
		"--no-psqlrc", "--no-align", "--tuples-only", "--quiet", "--no-password",
		"--set", "ON_ERROR_STOP=on", "--port", strconv.Itoa(port), "--dbname", "postgres")
	if err != nil {
		return "", err
	}
	cmd.Stdin = strings.NewReader(statement + "\n")
	stdout := &pgBuffer{limit: 1 << 20}
	stderr := &pgBuffer{limit: 16 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", pgCommandError(err, stderr.String())
	}
	return stdout.String(), nil
}

func hostDatabaseRemoteMySQLQuery(statement string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The same client as every other Agent statement (database_rpc.go):
	// "mariadb", so MariaDB 11+ does not put a deprecation line first on
	// stderr, where a failure's reason is read.
	cmd := exec.CommandContext(ctx, mysqlStatementProgram(), "--batch", "--raw", "--skip-column-names")
	cmd.Stdin = strings.NewReader(statement + "\n")
	stdout := &pgBuffer{limit: 1 << 20}
	stderr := &pgBuffer{limit: 16 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		// A syntax error quotes the statement after "near"; it can hold a
		// password hash, so that part is never kept.
		text := firstLine(stderr.String())
		if i := strings.Index(text, " near "); i >= 0 {
			text = text[:i]
		}
		if text == "" {
			text = err.Error()
		}
		return "", errors.New(text)
	}
	return stdout.String(), nil
}

// hostDatabaseRemoteLockFilter takes the filter's lock (flock, the same one
// the guard script's `flock 9` takes).
func hostDatabaseRemoteLockFilter() (func(), error) {
	file, err := os.OpenFile(databaseRemoteFilterLockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		file.Close()
	}, nil
}
