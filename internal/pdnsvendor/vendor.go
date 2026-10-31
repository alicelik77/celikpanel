package pdnsvendor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	UnitPath = "/usr/lib/systemd/system/pdns.service"
	ExecArgv = "/usr/sbin/pdns_server --guardian=no --daemon=no --disable-syslog --log-timestamp=no --write-pid=no"
)

const CertifiedDebian13Unit = "[Unit]\n" +
	"Description=PowerDNS Authoritative Server\n" +
	"Documentation=man:pdns_server(1) man:pdns_control(1)\n" +
	"Documentation=https://doc.powerdns.com\n" +
	"Wants=network-online.target\n" +
	"After=network-online.target mysql.service mysqld.service postgresql.service slapd.service mariadb.service time-sync.target\n\n" +
	"[Service]\n" +
	"ExecStart=" + ExecArgv + "\n" +
	"SyslogIdentifier=pdns_server\n" +
	"User=pdns\nGroup=pdns\nType=notify\nRestart=on-failure\nRestartSec=1\n" +
	"StartLimitInterval=0\nRuntimeDirectory=pdns\n\n" +
	"# Sandboxing\n" +
	"CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_CHOWN\n" +
	"AmbientCapabilities=CAP_NET_BIND_SERVICE CAP_CHOWN\n" +
	"LockPersonality=true\nNoNewPrivileges=true\nPrivateDevices=true\nPrivateTmp=true\n" +
	"# Setting PrivateUsers=true prevents us from opening our sockets\n" +
	"ProtectClock=true\nProtectControlGroups=true\nProtectHome=true\nProtectHostname=true\n" +
	"ProtectKernelLogs=true\nProtectKernelModules=true\nProtectKernelTunables=true\n" +
	"# ProtectSystem=full will disallow write access to /etc and /usr, possibly\n" +
	"# not being able to write slaved-zones into sqlite3 or zonefiles.\n" +
	"ProtectSystem=full\nRestrictAddressFamilies=AF_UNIX AF_INET AF_INET6\n" +
	"RestrictNamespaces=true\nRestrictRealtime=true\nRestrictSUIDSGID=true\n" +
	"SystemCallArchitectures=native\n" +
	"SystemCallFilter=~ @clock @debug @module @mount @raw-io @reboot @swap @cpu-emulation @obsolete\n" +
	"ProtectProc=invisible\nPrivateIPC=true\nRemoveIPC=true\nDevicePolicy=closed\n" +
	"# Not enabled by default because it does not play well with LuaJIT\n" +
	"# MemoryDenyWriteExecute=true\n\n" +
	"[Install]\nWantedBy=multi-user.target\n"

const (
	CertifiedDebianAfter = "After=network-online.target mysql.service mysqld.service postgresql.service slapd.service mariadb.service time-sync.target"
	CertifiedUbuntuAfter = "After=network-online.target mysqld.service postgresql.service slapd.service mariadb.service time-sync.target"
)

// Ubuntu 24.04 and Debian 13 ship the same hardened pdns.service contract.
// Ubuntu omits only the obsolete mysql.service ordering alias. Select between
// these reviewed package artifacts by exact bytes, never by os-release name.
var CertifiedUbuntu2404Unit = strings.Replace(
	CertifiedDebian13Unit,
	CertifiedDebianAfter,
	CertifiedUbuntuAfter,
	1,
)

const UnitPackageOwner = "pdns-server: /usr/lib/systemd/system/pdns.service\n"

// VerifyBytes accepts only the two reviewed APT vendor unit files.
func VerifyBytes(data []byte) error {
	if !bytes.Equal(data, []byte(CertifiedDebian13Unit)) &&
		!bytes.Equal(data, []byte(CertifiedUbuntu2404Unit)) {
		return errors.New("PowerDNS vendor unit bytes differ from the certified package unit")
	}
	return nil
}

// ArchExecArgv is the daemon command of Arch's extra/powerdns unit.
const ArchExecArgv = "/usr/bin/pdns_server --guardian=no --daemon=no --disable-syslog --log-timestamp=no --write-pid=no"

// ArchPackage is the Arch Linux package that owns the unit.
const ArchPackage = "powerdns"

// CertifiedArch51Unit is the pdns.service of Arch's extra/powerdns 5.1.5-1
// as measured on 2026-10-30 (SHA-256 4bc3129b…d420): the Debian contract
// with the daemon in /usr/bin and the powerdns service account. On Arch the
// unit is accepted when it is the installed package's own, unmodified file
// (its pacman mtree record) and keeps this command and account, so a
// legitimate package update inside the series is not read as tampering.
var CertifiedArch51Unit = strings.Replace(
	strings.Replace(CertifiedDebian13Unit, "ExecStart="+ExecArgv+"\n", "ExecStart="+ArchExecArgv+"\n", 1),
	"User=pdns\nGroup=pdns\n", "User=powerdns\nGroup=powerdns\n", 1,
)

// VerifyPacmanUnit accepts an Arch unit file whose SHA-256 is the digest the
// installed powerdns package recorded for it and that runs the certified
// daemon command under the powerdns account exactly once.
func VerifyPacmanUnit(data []byte, packageDigest string) error {
	sum := sha256.Sum256(data)
	if packageDigest == "" || hex.EncodeToString(sum[:]) != packageDigest {
		return errors.New("PowerDNS unit differs from the file the installed powerdns package recorded")
	}
	text := string(data)
	for _, line := range []string{"ExecStart=" + ArchExecArgv, "User=powerdns", "Group=powerdns"} {
		if strings.Count("\n"+text, "\n"+line+"\n") != 1 {
			return errors.New("PowerDNS package unit does not run the certified daemon command and account")
		}
	}
	if strings.Count("\n"+text, "\nExecStart") != 1 || strings.Contains(text, "\nExecStartPre") ||
		strings.Contains(text, "\nExecStartPost") {
		return errors.New("PowerDNS package unit has an uncertified start command")
	}
	return nil
}

// VerifyPackageOwner requires exact dpkg-query ownership of the fixed unit.
func VerifyPackageOwner(output []byte, commandErr error) error {
	if commandErr != nil {
		return commandErr
	}
	if string(output) != UnitPackageOwner {
		return errors.New("PowerDNS unit is not owned by the exact pdns-server package")
	}
	return nil
}
