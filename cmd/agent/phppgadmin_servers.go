package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// phpPgAdmin's server list follows the running PostgreSQL clusters
// (alpha.127). The Debian/Ubuntu package ships config.inc.php with one
// server on port 5432; with PostgreSQL 16 removed (its kept cluster still
// claiming 5432) and PostgreSQL 18 on 5433, phpPgAdmin offered only a server
// where nothing answers. When CelikPanel installs phpPgAdmin or moves it to
// another PHP version, the list names every running cluster ("PostgreSQL 18
// (port 5433)"), the default one first. The file is changed only while it is
// the package's own (its dpkg conffile checksum) or exactly what CelikPanel
// wrote last (the checksum recorded beside it): a file the owner edited is
// never overwritten; the panel says what it would name and how the owner sets
// it. Only the server lines change; every other line stays as it is.
//
// phpPgAdmin'in sunucu listesi çalışan kümeleri izler. Dosya yalnız paketin
// kendi hâlindeyken ya da CelikPanel'in son yazdığıyken değişir; sahibin
// düzenlediği dosyanın üzerine asla yazılmaz.

var (
	phpPgAdminPanelRecord = "/var/lib/celikpanel-agent-private/phppgadmin-config.sha256"
	phpPgAdminServerLine  = regexp.MustCompile(`^(\s*)\$conf\['servers'\]\[([0-9]+)\]\['([A-Za-z_]+)'\](\s*=\s*)(.*?);\s*(//.*)?$`)
)

// phpPgAdminServerEntry is one server the file names.
type phpPgAdminServerEntry struct {
	Index int
	Port  int
	Desc  string
}

// parsePHPPgAdminServers reads the active server entries (a commented
// example is not one); a server without a port line is on 5432.
func parsePHPPgAdminServers(content string) []phpPgAdminServerEntry {
	byIndex := map[int]*phpPgAdminServerEntry{}
	for _, line := range strings.Split(content, "\n") {
		m := phpPgAdminServerLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		index, _ := strconv.Atoi(m[2])
		entry := byIndex[index]
		if entry == nil {
			entry = &phpPgAdminServerEntry{Index: index, Port: pgDefaultPort}
			byIndex[index] = entry
		}
		value := strings.Trim(strings.TrimSpace(m[5]), `'"`)
		switch m[3] {
		case "port":
			if port, err := strconv.Atoi(value); err == nil {
				entry.Port = port
			}
		case "desc":
			entry.Desc = value
		}
	}
	entries := make([]phpPgAdminServerEntry, 0, len(byIndex))
	for _, entry := range byIndex {
		entries = append(entries, *entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Index < entries[j].Index })
	return entries
}

// phpPgAdminTargets are the running clusters in the order the list names
// them: the one a client without a port reaches first, then newest first.
func phpPgAdminTargets(clusters []transport.PostgreSQLCluster) []transport.PostgreSQLCluster {
	var running []transport.PostgreSQLCluster
	first, ok := pgRunningCluster(clusters)
	if ok {
		running = append(running, first)
	}
	var rest []transport.PostgreSQLCluster
	for _, cluster := range clusters {
		if cluster.Online && (!ok || cluster.Port != first.Port) {
			rest = append(rest, cluster)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return pgClusterNewer(rest[i], rest[j]) })
	return append(running, rest...)
}

func phpPgAdminDesc(cluster transport.PostgreSQLCluster) string {
	return fmt.Sprintf("PostgreSQL %s (port %d)", cluster.Version, cluster.Port)
}

// phpPgAdminWithServers is content with its server list naming targets:
// server 0's lines are kept as the template (their host, SSL mode, dump
// paths), its port and description set; the other active server entries
// are replaced by a copy of server 0 per further cluster.
func phpPgAdminWithServers(content string, targets []transport.PostgreSQLCluster) (string, error) {
	if len(targets) == 0 {
		return "", errors.New("no PostgreSQL cluster runs")
	}
	lines := strings.Split(content, "\n")
	var template []string
	lastZero := -1
	var kept []string
	for _, line := range lines {
		m := phpPgAdminServerLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			if !strings.HasPrefix(strings.TrimSpace(line), "// CelikPanel: phpPgAdmin server ") {
				kept = append(kept, line)
			}
			continue
		}
		if m[2] != "0" {
			continue
		}
		template = append(template, line)
		kept = append(kept, line)
		lastZero = len(kept) - 1
	}
	if lastZero < 0 {
		return "", errors.New("the file names no server 0 to follow")
	}
	set := func(line string, index int, cluster transport.PostgreSQLCluster) string {
		m := phpPgAdminServerLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		value := m[5]
		switch m[3] {
		case "port":
			value = strconv.Itoa(cluster.Port)
		case "desc":
			value = "'" + phpPgAdminDesc(cluster) + "'"
		}
		out := m[1] + "$conf['servers'][" + strconv.Itoa(index) + "]['" + m[3] + "']" + m[4] + value + ";"
		if m[6] != "" {
			out += " " + m[6]
		}
		return out
	}
	hasPort := false
	for _, line := range template {
		if phpPgAdminServerLine.FindStringSubmatch(strings.TrimRight(line, "\r"))[3] == "port" {
			hasPort = true
		}
	}
	indent := phpPgAdminServerLine.FindStringSubmatch(strings.TrimRight(template[0], "\r"))[1]
	// Server 0 in place.
	for i, line := range kept {
		if m := phpPgAdminServerLine.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil && m[2] == "0" {
			kept[i] = set(line, 0, targets[0])
		}
	}
	var extra []string
	if !hasPort {
		extra = append(extra, indent+"$conf['servers'][0]['port'] = "+strconv.Itoa(targets[0].Port)+";")
	}
	for index, cluster := range targets[1:] {
		extra = append(extra, indent+"// CelikPanel: phpPgAdmin server "+strconv.Itoa(index+1)+" is "+phpPgAdminDesc(cluster))
		for _, line := range template {
			extra = append(extra, set(line, index+1, cluster))
		}
		if !hasPort {
			extra = append(extra, indent+"$conf['servers']["+strconv.Itoa(index+1)+"]['port'] = "+strconv.Itoa(cluster.Port)+";")
		}
	}
	out := append(append(append([]string{}, kept[:lastZero+1]...), extra...), kept[lastZero+1:]...)
	return strings.Join(out, "\n"), nil
}

// phpPgAdminPackageChecksum is the md5 dpkg recorded for the shipped
// config.inc.php (its conffile), "" when unknown.
func phpPgAdminPackageChecksum(status, path string) string {
	scanner := bufio.NewScanner(strings.NewReader(status))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	inPackage, inConffiles := false, false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			inPackage, inConffiles = false, false
			continue
		}
		if strings.HasPrefix(line, "Package: ") {
			inPackage = strings.TrimSpace(strings.TrimPrefix(line, "Package: ")) == "phppgadmin"
			inConffiles = false
			continue
		}
		if !inPackage {
			continue
		}
		if strings.HasPrefix(line, "Conffiles:") {
			inConffiles = true
			continue
		}
		if inConffiles && strings.HasPrefix(line, " ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == path {
				return fields[1]
			}
			continue
		}
		inConffiles = false
	}
	return ""
}

// phpPgAdminOwnership: "package" (the shipped file), "panel" (what
// CelikPanel wrote last) or "owner" (anything else: edited by hand).
func phpPgAdminOwnership(content []byte, packageMD5, recordedSHA string) string {
	md5sum := md5.Sum(content)
	if packageMD5 != "" && hex.EncodeToString(md5sum[:]) == packageMD5 {
		return "package"
	}
	if recordedSHA != "" && sha256Hex(content) == recordedSHA {
		return "panel"
	}
	return "owner"
}

// phpPgAdminHost is what this file asks the host (a variable for tests).
var phpPgAdminHost = struct {
	read        func(path string) ([]byte, error)
	write       func(path string, content []byte) error
	dpkgStatus  func() string
	readRecord  func() string
	writeRecord func(sha string) error
}{
	read:       func(path string) ([]byte, error) { return pgClusterHostActive().readFile(path) },
	write:      func(path string, content []byte) error { return pgClusterHostActive().writeFile(path, content) },
	dpkgStatus: func() string { data, _ := os.ReadFile(dpkgStatusPath); return string(data) },
	readRecord: func() string {
		data, err := os.ReadFile(phpPgAdminPanelRecord)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	},
	writeRecord: func(sha string) error {
		if err := os.MkdirAll(filepath.Dir(phpPgAdminPanelRecord), 0o700); err != nil {
			return err
		}
		return writePrivateFileAtomic(phpPgAdminPanelRecord, []byte(sha+"\n"), ".phppgadmin-config-*")
	},
}

// phpPgAdminServersState reads, without changing anything, what phpPgAdmin
// names and what runs; nil where it does not apply (no phpPgAdmin file, no
// postgresql-common).
func phpPgAdminServersState() *transport.PHPPgAdminServers {
	content, err := phpPgAdminHost.read(pgPhpPgAdminConf)
	if err != nil {
		return nil
	}
	clusters, ok := pgStatementClusters()
	if !ok {
		return nil
	}
	state := &transport.PHPPgAdminServers{ConfigPath: pgPhpPgAdminConf, Configured: []transport.PHPPgAdminServer{}, Running: []transport.PHPPgAdminServer{}}
	for _, entry := range parsePHPPgAdminServers(string(content)) {
		server := transport.PHPPgAdminServer{Port: entry.Port}
		if cluster, found := pgClusterOnPort(clusters, entry.Port); found {
			server.Version, server.Online = cluster.Version, cluster.Online
		}
		state.Configured = append(state.Configured, server)
	}
	targets := phpPgAdminTargets(clusters)
	for _, cluster := range targets {
		state.Running = append(state.Running, transport.PHPPgAdminServer{Version: cluster.Version, Port: cluster.Port, Online: true})
	}
	state.Owner = phpPgAdminOwnership(content, phpPgAdminPackageChecksum(phpPgAdminHost.dpkgStatus(), pgPhpPgAdminConf), phpPgAdminHost.readRecord())
	state.State = "aligned"
	if len(targets) == 0 {
		state.State = "none_running"
	} else if want, err := phpPgAdminWithServers(string(content), targets); err != nil || want != string(content) {
		state.State = "differs"
		if err != nil {
			state.State = "unreadable"
		}
	}
	return state
}

// alignPHPPgAdminServers sets phpPgAdmin's server list to the running
// clusters when the file is the package's or CelikPanel's own. It returns a
// warning for the operator when it did not ("" when nothing needed saying).
func alignPHPPgAdminServers() string {
	state := phpPgAdminServersState()
	if state == nil || state.State == "aligned" || state.State == "none_running" {
		return ""
	}
	if state.Owner == "owner" {
		return fmt.Sprintf("phpPgAdmin's server list in %s was edited on this server, so CelikPanel did not change it; it names port(s) %s while PostgreSQL runs on %s", pgPhpPgAdminConf, phpPgAdminPorts(state.Configured), phpPgAdminPorts(state.Running))
	}
	content, err := phpPgAdminHost.read(pgPhpPgAdminConf)
	if err != nil {
		return "phpPgAdmin's configuration could not be read again: " + boundedText(err.Error(), 160)
	}
	clusters, _ := pgStatementClusters()
	want, err := phpPgAdminWithServers(string(content), phpPgAdminTargets(clusters))
	if err != nil {
		return "phpPgAdmin's server list could not be followed: " + err.Error()
	}
	if err := phpPgAdminHost.write(pgPhpPgAdminConf, []byte(want)); err != nil {
		return "phpPgAdmin's server list could not be written: " + boundedText(err.Error(), 160)
	}
	if err := phpPgAdminHost.writeRecord(sha256Hex([]byte(want))); err != nil {
		return "phpPgAdmin's server list was written, but CelikPanel could not record it, so it will treat the file as yours next time: " + boundedText(err.Error(), 160)
	}
	return ""
}

func phpPgAdminPorts(servers []transport.PHPPgAdminServer) string {
	ports := make([]string, 0, len(servers))
	for _, server := range servers {
		ports = append(ports, strconv.Itoa(server.Port))
	}
	if len(ports) == 0 {
		return "none"
	}
	return strings.Join(ports, ", ")
}
