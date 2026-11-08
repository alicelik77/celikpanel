package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Readiness (alpha.130, status only): what a server still needs before the
// first rule of an engine could be in force, read without changing anything,
// so the administrator's Databases page can show it before any customer
// lists an address (each item: what is missing and the step).
//
// Hazırlık: bir motorun ilk kuralı yürürlüğe girmeden önce eksik olanlar;
// hiçbir şey değiştirilmeden okunur.

func readinessProblem(code, detail string) transport.DatabaseRemoteProblem {
	return transport.DatabaseRemoteProblem{Code: code, Detail: detail}
}

// readiness of a MariaDB instance.
func (m mariadbRemoteInstance) readiness(target dbremote.Target, ts dbremote.TargetState, result *transport.DatabaseRemoteTargetResult) {
	s, err := m.read()
	if err != nil {
		return
	}
	add := func(code, detail string) { result.Readiness = append(result.Readiness, readinessProblem(code, detail)) }
	result.Fail2ban = "unavailable"
	switch {
	case !fail2banInstalled():
		add(transport.DatabaseRemoteNoSlowDown, "Fail2ban is not installed; install it in Components")
	case !databaseRemoteFail2ban.active():
		add(transport.DatabaseRemoteNoSlowDown, "Fail2ban is installed but does not run; start it in Components")
	case databaseRemoteFail2ban.jailActive(dbremote.MariaDBJailName):
		result.Fail2ban = "on"
	default:
		result.Fail2ban = "off" // written with the first rule
	}
	if logError := strings.TrimSpace(s.logError); logError != "" && logError != "stderr" {
		add(transport.DatabaseRemoteNoSlowDown, "MariaDB writes its log to "+logError+" (log_error); remove log_error from MariaDB's settings and restart MariaDB")
	}
	if s.logWarnings < 2 {
		add(transport.DatabaseRemoteNoSlowDown, fmt.Sprintf("MariaDB does not log refused sign-ins (log_warnings = %d); set log_warnings = 2 and restart MariaDB", s.logWarnings))
	}
	switch {
	case s.skipNameResolve:
		result.NameResolve = "off"
	case ts.SkipNameResolve:
		result.NameResolve = "pending_restart"
		add(transport.DatabaseRemoteRestartNeeded, "MariaDB turns host name lookup off when it restarts")
	default:
		result.NameResolve = "on"
		path, _ := mariadbOptionPath()
		if value, file := mariadbOwnerNameResolve(path); value == "lookup_on" {
			add(transport.DatabaseRemoteOwnerNameResolve, file+" turns MariaDB's host name lookup on (skip-name-resolve off); CelikPanel does not change your setting")
		} else if accounts, err := mariadbHostNameAccounts(); err == nil && len(accounts) > 0 {
			if len(accounts) > 5 {
				accounts = append(accounts[:5:5], fmt.Sprintf("and %d more", len(accounts)-5))
			}
			add(transport.DatabaseRemoteNameAccounts, strings.Join(accounts, ", "))
		} else {
			add(transport.DatabaseRemoteNameResolve, "MariaDB looks up the host name of every client, so a refused sign-in names a host and Fail2ban cannot ban it")
		}
	}
}

// readiness of a PostgreSQL cluster.
func (p pgRemoteInstance) readiness(target dbremote.Target, ts dbremote.TargetState, result *transport.DatabaseRemoteTargetResult) {
	s, err := p.read(target.Port)
	if err != nil {
		return
	}
	add := func(code, detail string) { result.Readiness = append(result.Readiness, readinessProblem(code, detail)) }
	if !s.alterSystem {
		add(transport.DatabaseRemoteAlterSystemOff, "allow_alter_system is off on this cluster, so CelikPanel cannot set its listen address or TLS")
	}
	if !pgPreloadHasAuthDelay(s.preload) {
		owner := strings.TrimSpace(s.preload)
		if owner == "" && s.preloadFile != "<none>" {
			owner = strings.Trim(s.preloadFile, "' ")
		}
		switch {
		case ts.AuthDelay:
			add(transport.DatabaseRemoteRestartNeeded, "auth_delay (slowing wrong passwords down) loads when PostgreSQL restarts")
		case owner != "":
			add(transport.DatabaseRemoteNoSlowDown, "shared_preload_libraries is set on this server ('"+boundedText(owner, 80)+"'); add auth_delay to it in postgresql.conf ('"+
				boundedText(owner, 80)+",auth_delay') and auth_delay.milliseconds = "+pgAuthDelayMilliseconds+", then restart PostgreSQL")
		case s.pkgLibDir == "" || !fileIsRegular(filepath.Join(s.pkgLibDir, "auth_delay.so")):
			add(transport.DatabaseRemoteNoSlowDown, "auth_delay is not among this PostgreSQL's installed modules; install the PostgreSQL contrib modules of this version")
		}
	}
	if current, err := databaseRemoteReadHBA(s.hbaFile); err == nil {
		if foreign := dbremote.ForeignHBAHostLines(string(current)); len(foreign) > 0 {
			add(transport.DatabaseRemoteOwnerHBA, s.hbaFile+" already lets network connections in (\""+boundedText(foreign[0], 120)+"\"); remove or narrow that line to 127.0.0.1/32 or ::1/128")
		}
	}
	result.Fail2ban, result.Fail2banDetail = pgFail2banAssessment(target, ts)
	if result.Fail2ban == "unavailable" {
		add(transport.DatabaseRemotePGFail2ban, result.Fail2banDetail)
	}
}

// pgFail2banAssessment reads (only) whether the extra layer could run on
// this cluster: "on" (its jail runs), "off" (it is set up with the first
// rule), or "unavailable" with the reason.
func pgFail2banAssessment(target dbremote.Target, ts dbremote.TargetState) (string, string) {
	switch {
	case !fail2banInstalled():
		return "unavailable", "Fail2ban is not installed; install it in Components to also ban addresses that keep guessing (auth_delay already slows every wrong password down)"
	case !databaseRemoteFail2ban.active():
		return "unavailable", "Fail2ban is installed but does not run; start it in Components"
	case databaseRemoteFail2ban.jailActive(dbremote.PGJailName(target.Port)):
		return "on", ""
	}
	logs, err := readPGRemoteLogSettings(target.Port)
	switch {
	case err != nil:
		return "unavailable", "PostgreSQL's logging settings could not be read: " + boundedText(err.Error(), 160)
	case logs.logHostname:
		return "unavailable", "log_hostname is on, so PostgreSQL logs host names instead of addresses; CelikPanel leaves your setting as it is"
	case logs.loggingCollector:
		return "unavailable", "PostgreSQL writes its log through logging_collector into files of its own; CelikPanel reads only the cluster's standard log"
	case !dbremote.PGLocaleEnglish(logs.lcMessages):
		return "unavailable", "lc_messages = '" + boundedText(logs.lcMessages, 40) + "' writes PostgreSQL's messages in another language, which Fail2ban cannot read"
	case !dbremote.PGPrefixCarriesMarker(logs.prefix) &&
		(dbremote.PGPrefixWithMarker(logs.prefix) == "" || (strings.HasSuffix(logs.prefixFile, "postgresql.auto.conf") && !ts.LogPrefix)):
		return "unavailable", "log_line_prefix ('" + boundedText(logs.prefix, 80) + "') was set on this server, so CelikPanel leaves PostgreSQL's logging as it is and does not use Fail2ban here"
	}
	if _, err := databaseRemotePGLogSource(target); err != nil {
		return "unavailable", err.Error()
	}
	return "off", ""
}
