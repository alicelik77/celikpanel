package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Fail2ban for PostgreSQL (alpha.130, owner decision 8 Nov 2026): an EXTRA
// layer on top of auth_delay (internal/dbremote/pgfail2ban.go). It never
// holds a rule back: whatever is missing is reported in the answer's
// Fail2ban / Fail2banDetail and nothing else changes. Everything CelikPanel
// does for it is undone when the cluster's last rule goes or when Fail2ban
// is removed (no leftovers): its jail file, the shared filter when no
// CelikPanel PostgreSQL jail remains, and the log_line_prefix it set.
//
// PostgreSQL için Fail2ban: auth_delay'in üstünde ek katman; kuralı asla
// bekletmez. Son kural gidince ya da Fail2ban kaldırılınca geri alınır.

// pgRemoteLogSettings is what a cluster says about its logging.
type pgRemoteLogSettings struct {
	prefix           string
	prefixFile       string // file of the last log_line_prefix line ("" = built-in default)
	logHostname      bool
	loggingCollector bool
	lcMessages       string
}

const pgRemoteLogSQL = `SELECT concat_ws(chr(31),
 current_setting('log_line_prefix'),
 coalesce((SELECT sourcefile FROM pg_file_settings WHERE name = 'log_line_prefix' ORDER BY seqno DESC LIMIT 1), ''),
 current_setting('log_hostname'),
 current_setting('logging_collector'),
 current_setting('lc_messages'));`

func readPGRemoteLogSettings(port int) (pgRemoteLogSettings, error) {
	out, err := databaseRemotePGQuery(port, pgRemoteLogSQL)
	if err != nil {
		return pgRemoteLogSettings{}, err
	}
	fields := strings.Split(strings.TrimRight(out, "\r\n"), pgRemoteFieldSep)
	if len(fields) != 5 {
		return pgRemoteLogSettings{}, fmt.Errorf("PostgreSQL answered %d logging values, CelikPanel asked for 5", len(fields))
	}
	return pgRemoteLogSettings{prefix: fields[0], prefixFile: fields[1], logHostname: fields[2] == "on",
		loggingCollector: fields[3] == "on", lcMessages: fields[4]}, nil
}

// databaseRemoteListDir lists a folder's names; swapped by tests.
var databaseRemoteListDir = func(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// databaseRemotePGLogSource answers where the cluster on port writes its
// log: Debian's and Ubuntu's pg_ctlcluster file, else the unit's journal.
var databaseRemotePGLogSource = func(target dbremote.Target) (dbremote.PGLogSource, error) {
	if cluster, ok := pgClusterForPort(target.Port); ok {
		file := "/var/log/postgresql/postgresql-" + cluster + ".log"
		if !fileIsRegular(file) {
			return dbremote.PGLogSource{}, errors.New(file + " does not exist, so CelikPanel does not know where this cluster logs")
		}
		return dbremote.PGLogSource{File: file}, nil
	}
	return dbremote.PGLogSource{Unit: dbRemoteUnit(target) + ".service"}, nil
}

// pgOwnJailText: current is exactly a jail CelikPanel renders for this port
// and some log source (the source may have moved since, e.g. a renamed
// cluster); any edit makes it the owner's.
func pgOwnJailText(port int, current string) bool {
	for _, line := range strings.Split(current, "\n") {
		if strings.HasPrefix(line, "logpath = ") {
			jail, err := dbremote.PGJail(port, dbremote.PGLogSource{File: strings.TrimPrefix(line, "logpath = ")})
			return err == nil && jail == current
		}
		if strings.HasPrefix(line, "journalmatch = _SYSTEMD_UNIT=") {
			jail, err := dbremote.PGJail(port, dbremote.PGLogSource{Unit: strings.TrimPrefix(line, "journalmatch = _SYSTEMD_UNIT=")})
			return err == nil && jail == current
		}
	}
	return false
}

// applyPGJail brings the extra layer of the cluster in line with want (the
// cluster has rules) and reports it in result.
func applyPGJail(target dbremote.Target, want bool, ts *dbremote.TargetState, result *transport.DatabaseRemoteTargetResult) {
	unavailable := func(detail string) {
		result.Fail2ban, result.Fail2banDetail = "unavailable", detail
	}
	jailPath := dbremote.PGJailPath(target.Port)
	current, readErr := databaseRemoteFiles.read(jailPath)
	jailExists := readErr == nil
	jailOwn := jailExists && pgOwnJailText(target.Port, string(current))
	installed := fail2banInstalled()

	if !want || !installed {
		changed := false
		if jailOwn {
			changed = databaseRemoteFiles.remove(jailPath) == nil
		}
		ts.PGJail = false
		if pgRemoveSharedFilterIfUnused() {
			changed = true
		}
		if ts.LogPrefix {
			if err := pgAlterSystem(target.Port, "ALTER SYSTEM RESET log_line_prefix;"); err == nil {
				ts.LogPrefix = false
			}
		}
		if changed && installed {
			_ = databaseRemoteFail2ban.reload()
		}
		if !want {
			result.Fail2ban = "off"
		} else {
			unavailable("Fail2ban is not installed; install it in Components to also ban addresses that keep guessing (auth_delay already slows every wrong password down)")
		}
		return
	}
	if !databaseRemoteFail2ban.active() {
		unavailable("Fail2ban is installed but does not run; start it in Components")
		return
	}
	logs, err := readPGRemoteLogSettings(target.Port)
	switch {
	case err != nil:
		unavailable("PostgreSQL's logging settings could not be read: " + boundedText(err.Error(), 160))
		return
	case logs.logHostname:
		unavailable("log_hostname is on, so PostgreSQL logs host names instead of addresses; CelikPanel leaves your setting as it is")
		return
	case logs.loggingCollector:
		unavailable("PostgreSQL writes its log through logging_collector into files of its own; CelikPanel reads only the cluster's standard log")
		return
	case !dbremote.PGLocaleEnglish(logs.lcMessages):
		unavailable("lc_messages = '" + boundedText(logs.lcMessages, 40) + "' writes PostgreSQL's messages in another language, which Fail2ban cannot read")
		return
	}
	source, err := databaseRemotePGLogSource(target)
	if err != nil {
		unavailable(err.Error())
		return
	}
	jail, err := dbremote.PGJail(target.Port, source)
	if err != nil {
		unavailable(err.Error())
		return
	}

	// The client address in each line: CelikPanel's marker in
	// log_line_prefix, set only over a package default.
	if !dbremote.PGPrefixCarriesMarker(logs.prefix) {
		wanted := dbremote.PGPrefixWithMarker(logs.prefix)
		ownerAuto := strings.HasSuffix(logs.prefixFile, "postgresql.auto.conf") && !ts.LogPrefix
		if wanted == "" || ownerAuto {
			unavailable("log_line_prefix ('" + boundedText(logs.prefix, 80) + "') was set on this server, so CelikPanel leaves PostgreSQL's logging as it is and does not use Fail2ban here")
			return
		}
		if err := pgAlterSystem(target.Port, "ALTER SYSTEM SET log_line_prefix = '"+wanted+"';"); err != nil {
			unavailable("PostgreSQL did not accept the log line prefix: " + boundedText(err.Error(), 160))
			return
		}
		ts.LogPrefix = true
	}

	// The filter (shared) and this cluster's jail.
	wrote := false
	filterCurrent, filterErr := databaseRemoteFiles.read(dbremote.PGFilterPath)
	switch {
	case filterErr != nil:
		if err := databaseRemoteFiles.write(dbremote.PGFilterPath, []byte(dbremote.PGFilter())); err != nil {
			unavailable("the Fail2ban filter could not be written: " + err.Error())
			return
		}
		wrote = true
	case string(filterCurrent) != dbremote.PGFilter():
		// The owner's version: used as it is.
	}
	switch {
	case !jailExists:
		if err := databaseRemoteFiles.write(jailPath, []byte(jail)); err != nil {
			unavailable("the Fail2ban jail could not be written: " + err.Error())
			return
		}
		wrote = true
	case jailOwn && string(current) != jail:
		// The log moved (a renamed cluster): CelikPanel's own file follows.
		if err := databaseRemoteFiles.write(jailPath, []byte(jail)); err != nil {
			unavailable("the Fail2ban jail could not be written: " + err.Error())
			return
		}
		wrote = true
	}
	if jailOwn || !jailExists {
		ts.PGJail = true
	}
	if wrote {
		if err := databaseRemoteFail2ban.reload(); err != nil {
			if ts.PGJail {
				_ = databaseRemoteFiles.remove(jailPath)
				ts.PGJail = false
			}
			pgRemoveSharedFilterIfUnused()
			_ = databaseRemoteFail2ban.reload()
			unavailable("Fail2ban did not accept the PostgreSQL jail, so it was taken back; its log says why")
			return
		}
	}
	if !databaseRemoteFail2ban.jailActive(dbremote.PGJailName(target.Port)) {
		unavailable("Fail2ban does not run the " + dbremote.PGJailName(target.Port) + " jail; its log says why")
		return
	}
	result.Fail2ban = "on"
}

// pgRemoveSharedFilterIfUnused removes CelikPanel's PostgreSQL filter when
// no CelikPanel PostgreSQL jail file is left; it answers whether it did.
func pgRemoveSharedFilterIfUnused() bool {
	names, err := databaseRemoteListDir(filepath.Dir(dbremote.PGJailPath(5432)))
	if err == nil {
		for _, name := range names {
			if _, ok := dbremote.PGJailPort(name); ok {
				return false
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false
	}
	current, err := databaseRemoteFiles.read(dbremote.PGFilterPath)
	if err != nil || string(current) != dbremote.PGFilter() {
		return false
	}
	return databaseRemoteFiles.remove(dbremote.PGFilterPath) == nil
}
