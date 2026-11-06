package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/services"
)

const maxBackupCommandStderr = 64 << 10

type backupCommand struct {
	Name string
	Args []string
}

var backupCommandContext = exec.CommandContext
var backupDatabaseTimeout = 2 * time.Hour

var dumpDatabaseToFile = dumpDatabase
var restoreDatabaseFromFile = restoreDatabase

func validateDatabaseIdentity(database backupspec.DatabaseIdentity) (backupspec.DatabaseIdentity, error) {
	if database.ID < 1 {
		return backupspec.DatabaseIdentity{}, errors.New("invalid database ID")
	}
	if err := services.ValidateSQLIdentifier(database.Name); err != nil {
		return backupspec.DatabaseIdentity{}, fmt.Errorf("invalid database name: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(database.Type)) {
	case "", "mysql", "mariadb":
		database.Type = "mysql"
	case "postgresql", "postgres":
		database.Type = "postgresql"
	default:
		return backupspec.DatabaseIdentity{}, errors.New("unsupported database type")
	}
	if err := validDatabaseCluster(database); err != nil {
		return backupspec.DatabaseIdentity{}, err
	}
	return database, nil
}

func validateDatabaseSet(databases []backupspec.DatabaseIdentity) ([]backupspec.DatabaseIdentity, error) {
	result := make([]backupspec.DatabaseIdentity, 0, len(databases))
	seenIDs := make(map[int]bool, len(databases))
	seenTargets := make(map[string]bool, len(databases))
	for _, database := range databases {
		normalized, err := validateDatabaseIdentity(database)
		if err != nil {
			return nil, err
		}
		if seenIDs[normalized.ID] {
			return nil, errors.New("duplicate database ID")
		}
		targetKey := normalized.Type + "\x00" + normalized.Name
		if seenTargets[targetKey] {
			return nil, errors.New("duplicate database target")
		}
		seenIDs[normalized.ID] = true
		seenTargets[targetKey] = true
		result = append(result, normalized)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func databaseDumpCommand(database backupspec.DatabaseIdentity) (backupCommand, error) {
	database, err := validateDatabaseIdentity(database)
	if err != nil {
		return backupCommand{}, err
	}
	if database.Type == "postgresql" {
		args := []string{"-u", "postgres", "pg_dump", "--no-owner", "--no-privileges", "--dbname", database.Name}
		port, err := pgBackupPortFor(database)
		if err != nil {
			return backupCommand{}, err
		}
		if port > 0 {
			args = append(args, "--port", strconv.Itoa(port))
		}
		return backupCommand{Name: "sudo", Args: args}, nil
	}
	return backupCommand{
		Name: "mysqldump",
		Args: []string{
			"--protocol=socket", "--user=root", "--single-transaction",
			"--routines", "--triggers", "--skip-lock-tables", database.Name,
		},
	}, nil
}

// pgRestorePrelude makes a PostgreSQL restore work on a database that still
// holds its tables (alpha.127 lab: every restore of a non-empty database
// stopped at "relation already exists", and so did the safety rollback). A
// dump is plain SQL made with --no-owner and without DROP statements, so the
// restore session first becomes the database's owner and drops what that
// owner owns in this database; the dump then recreates it, owned by the
// owner as before. A database owned by a superuser is left as it was (DROP
// OWNED BY postgres is refused by the server). Objects of other roles are
// not touched. On a failure the caller reapplies the safety dump the same way.
//
// pgRestorePrelude, tabloları duran bir veritabanına geri yüklemeyi çalıştırır:
// oturum önce veritabanının sahibi olur ve sahibin bu veritabanındaki
// nesnelerini düşürür; döküm onları yine sahibine ait olarak yeniden kurar.
const pgRestorePrelude = `DO $celikpanel_restore$
DECLARE
  owner_name name := (SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database());
BEGIN
  IF NOT (SELECT rolsuper FROM pg_roles WHERE rolname = owner_name) THEN
    EXECUTE format('SET ROLE %I', owner_name);
    EXECUTE 'DROP OWNED BY CURRENT_USER';
  END IF;
END
$celikpanel_restore$`

func databaseRestoreCommand(database backupspec.DatabaseIdentity) (backupCommand, error) {
	database, err := validateDatabaseIdentity(database)
	if err != nil {
		return backupCommand{}, err
	}
	if database.Type == "postgresql" {
		args := []string{"-u", "postgres", "psql", "--set", "ON_ERROR_STOP=on", "--dbname", database.Name}
		port, err := pgBackupPortFor(database)
		if err != nil {
			return backupCommand{}, err
		}
		if port > 0 {
			args = append(args, "--port", strconv.Itoa(port))
		}
		// The prelude runs first in the same session, then the dump from stdin.
		args = append(args, "--command", pgRestorePrelude, "--file", "-")
		return backupCommand{Name: "sudo", Args: args}, nil
	}
	return backupCommand{
		Name: "mysql",
		Args: []string{"--protocol=socket", "--user=root", "--database", database.Name},
	}, nil
}

type limitedBuffer struct {
	buffer bytes.Buffer
	left   int
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	if b.left > 0 {
		keep := len(data)
		if keep > b.left {
			keep = b.left
		}
		_, _ = b.buffer.Write(data[:keep])
		b.left -= keep
	}
	return original, nil
}

func runBackupCommand(ctx context.Context, command backupCommand, stdin io.Reader, stdout io.Writer) error {
	cmd := backupCommandContext(ctx, command.Name, command.Args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	stderr := &limitedBuffer{left: maxBackupCommandStderr}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.buffer.String())
		if message != "" {
			return fmt.Errorf("%s failed: %w: %s", command.Name, err, message)
		}
		return fmt.Errorf("%s failed: %w", command.Name, err)
	}
	return nil
}

func dumpDatabase(database backupspec.DatabaseIdentity, destination string) (returnErr error) {
	command, err := databaseDumpCommand(database)
	if err != nil {
		return err
	}
	out, err := openPrivateExclusive(destination)
	if err != nil {
		return err
	}
	defer func() {
		if returnErr != nil {
			_ = out.Close()
			_ = os.Remove(destination)
		}
	}()
	gz := gzip.NewWriter(out)
	ctx, cancel := context.WithTimeout(context.Background(), backupDatabaseTimeout)
	defer cancel()
	if err := runBackupCommand(ctx, command, nil, gz); err != nil {
		_ = gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}

func restoreDatabase(database backupspec.DatabaseIdentity, source string) error {
	command, err := databaseRestoreCommand(database)
	if err != nil {
		return err
	}
	file, _, err := secureOpenRegular(source)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	ctx, cancel := context.WithTimeout(context.Background(), backupDatabaseTimeout)
	defer cancel()
	return runBackupCommand(ctx, command, gz, io.Discard)
}

func validateRestoreDatabase(expected, requested backupspec.DatabaseIdentity) (backupspec.DatabaseIdentity, error) {
	expected, err := validateDatabaseIdentity(expected)
	if err != nil {
		return backupspec.DatabaseIdentity{}, err
	}
	requested, err = validateDatabaseIdentity(requested)
	if err != nil {
		return backupspec.DatabaseIdentity{}, err
	}
	// alpha.127: the same database, whichever cluster each side names; the
	// requested identity (the database's cluster now) is the one returned.
	if !expected.SameDatabase(requested) {
		return backupspec.DatabaseIdentity{}, errors.New("database identity mismatch")
	}
	return requested, nil
}

func validateFullRestoreSet(expected, requested []backupspec.DatabaseIdentity) ([]backupspec.DatabaseIdentity, error) {
	expected, err := validateDatabaseSet(expected)
	if err != nil {
		return nil, err
	}
	requested, err = validateDatabaseSet(requested)
	if err != nil {
		return nil, err
	}
	if len(expected) != len(requested) {
		return nil, errors.New("full restore database set mismatch")
	}
	for i := range expected {
		if !expected[i].SameDatabase(requested[i]) {
			return nil, errors.New("full restore database set mismatch")
		}
	}
	return requested, nil
}

// pgBackupPortFor is the cluster a PostgreSQL backup or restore talks to
// (alpha.127): the identity's cluster port when it names one (resolved
// before, backup_pg_cluster.go), else the running cluster psql would choose
// without a port, never a removed version's kept cluster that still claims
// the default port: on Boston, psql without a port went to 16/main, which had
// no program left.
func pgBackupPortFor(database backupspec.DatabaseIdentity) (int, error) {
	return resolvePGStatementPort(database.ClusterPort)
}
