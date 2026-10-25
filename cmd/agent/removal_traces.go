package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The system's own traces before a removal (alpha.108, D-038 §13). The owner
// may run things CelikPanel did not set up (D-022): an nginx server block
// that hands PHP to a version's socket, a service of their own that requires
// the database, a script connected to Redis. Before a component or one
// version of it is removed, the removal preview looks — bounded, read-only,
// and without any mutation — at three of the system's own traces:
//
//   - web-server configuration (nginx sites-enabled, conf.d and nginx.conf;
//     apache sites-enabled, conf-enabled and its main file) naming the
//     component's socket or a loopback address with its port, by file and
//     line;
//   - services in /etc/systemd/system (the owner's own units and drop-ins;
//     package units are the package manager's simulation's business) that
//     Require, Want, BindTo, are PartOf or come After its unit;
//   - local programs connected right now to its unix socket or, over
//     loopback, to its port (ss).
//
// What CelikPanel set up itself is not a trace: its own files (celikpanel-*)
// and units (celikpanel-*, celikapp-*), its processes, the component's own
// units and processes. A site's vhost is filtered by the Panel, which knows
// its sites. A source that cannot be read is said ("partial"), never taken as
// "nothing". The findings are warnings for the removal dialog; they never
// refuse a removal.
//
// Kaldırmadan önce sistemin kendi izleri: web sunucusu yapılandırması,
// bağımlı hizmetler, bağlı yerel programlar. Salt-okur ve sınırlı; uyarıdır,
// reddetmez.

const (
	removalTraceCap      = 40
	removalTraceFileCap  = 400
	removalTraceFileSize = 1 << 20
	removalTraceTextCap  = 160
	removalTraceSSBound  = 5 * time.Second
)

// Where the traces are read from (variables for the mounted tests).
var (
	removalTraceWebConfigGlobs = []string{
		"/etc/nginx/nginx.conf", "/etc/nginx/sites-enabled/*", "/etc/nginx/conf.d/*.conf",
		"/etc/apache2/apache2.conf", "/etc/apache2/sites-enabled/*", "/etc/apache2/conf-enabled/*",
		"/etc/httpd/conf/httpd.conf", "/etc/httpd/conf.d/*.conf",
	}
	removalTraceUnitDir = "/etc/systemd/system"
	// removalTraceSS runs ss with the given arguments, bounded.
	removalTraceSS = func(ctx context.Context, args ...string) (string, error) {
		path, err := exec.LookPath("ss")
		if err != nil {
			return "", err
		}
		callCtx, cancel := context.WithTimeout(ctx, removalTraceSSBound)
		defer cancel()
		out, err := exec.CommandContext(callCtx, path, args...).Output()
		return string(out), err
	}
)

var (
	unixAddressInLine = regexp.MustCompile(`unix:(/[^\s;"'|]+)`)
	ssUsers           = regexp.MustCompile(`\("([^"]+)",pid=([0-9]+)`)
	unitDependencyKey = regexp.MustCompile(`^\s*(Requires|Requisite|Wants|BindsTo|PartOf|After)\s*=\s*(.*)$`)
)

// removalTraceScope is what one removal looks for.
type removalTraceScope struct {
	facts core.RemovalTraceFacts
	// units are the component's own units (the removal's, its catalogue
	// names and helpers), with and without ".service".
	units map[string]bool
}

func newRemovalTraceScope(svc *core.ManagedService, version string, workUnits []string) removalTraceScope {
	scope := removalTraceScope{facts: core.RemovalTraceFactsFor(svc, version), units: map[string]bool{}}
	names := append(append([]string{}, workUnits...), svc.HelperUnits...)
	if version == "" {
		names = append(names, svc.SystemNames...)
	}
	for _, unit := range names {
		base := strings.TrimSuffix(unit, ".service")
		if base != "" {
			scope.units[base] = true
		}
	}
	return scope
}

func (s removalTraceScope) ownUnit(name string) bool {
	return s.units[strings.TrimSuffix(name, ".service")]
}

// ownedByCelikPanel: files, units and processes CelikPanel made itself.
func ownedByCelikPanel(name string) bool {
	name = filepath.Base(name)
	return strings.HasPrefix(name, "celikpanel") || strings.HasPrefix(name, "celikapp-")
}

// collectRemovalTraces reads the three traces for one removal.
func collectRemovalTraces(ctx context.Context, scope removalTraceScope) ([]transport.RemovalTrace, string, []string) {
	traces := []transport.RemovalTrace{}
	unread := []string{}
	if scope.facts.WebConfig && (len(scope.facts.Sockets) > 0 || len(scope.facts.Ports) > 0) {
		found, ok := webConfigTraces(scope)
		traces = append(traces, found...)
		if !ok {
			unread = append(unread, "web_config")
		}
	}
	if len(scope.units) > 0 {
		found, ok := unitTraces(scope)
		traces = append(traces, found...)
		if !ok {
			unread = append(unread, "units")
		}
	}
	if len(scope.facts.Sockets) > 0 || len(scope.facts.Ports) > 0 {
		found, ok := connectionTraces(ctx, scope)
		traces = append(traces, found...)
		if !ok {
			unread = append(unread, "connections")
		}
	}
	if len(traces) > removalTraceCap {
		traces = traces[:removalTraceCap]
	}
	state := "checked"
	if len(unread) > 0 {
		state = "partial"
	}
	return traces, state, unread
}

func boundedTraceText(line string) string {
	line = strings.TrimSpace(line)
	if len(line) > removalTraceTextCap {
		line = line[:removalTraceTextCap] + "…"
	}
	return line
}

// lineNamesEndpoint: a configuration line names one of the sockets, or a
// loopback address with one of the ports.
func (s removalTraceScope) lineNamesEndpoint(line string) string {
	for _, match := range unixAddressInLine.FindAllStringSubmatch(line, -1) {
		for _, glob := range s.facts.Sockets {
			if ok, _ := filepath.Match(glob, match[1]); ok {
				return match[1]
			}
		}
	}
	for _, port := range s.facts.Ports {
		p := strconv.Itoa(port)
		for _, host := range []string{"127.0.0.1:", "localhost:", "[::1]:"} {
			at := strings.Index(line, host+p)
			if at < 0 {
				continue
			}
			end := at + len(host+p)
			if end == len(line) || !strings.ContainsRune("0123456789", rune(line[end])) {
				return host + p
			}
		}
	}
	return ""
}

func webConfigTraces(scope removalTraceScope) ([]transport.RemovalTrace, bool) {
	traces := []transport.RemovalTrace{}
	ok := true
	files := 0
	seen := map[string]bool{}
	for _, glob := range removalTraceWebConfigGlobs {
		matches, err := filepath.Glob(glob)
		if err != nil {
			continue
		}
		sort.Strings(matches)
		for _, path := range matches {
			if seen[path] || ownedByCelikPanel(path) {
				continue
			}
			seen[path] = true
			info, err := os.Stat(path)
			if err != nil || info.IsDir() || !info.Mode().IsRegular() {
				continue
			}
			files++
			if files > removalTraceFileCap {
				return traces, false
			}
			file, err := os.Open(path)
			if err != nil {
				ok = false
				continue
			}
			scanner := bufio.NewScanner(io.LimitReader(file, removalTraceFileSize))
			scanner.Buffer(make([]byte, 64*1024), 1024*1024)
			number := 0
			for scanner.Scan() {
				number++
				line := scanner.Text()
				if strings.HasPrefix(strings.TrimSpace(line), "#") {
					continue
				}
				if endpoint := scope.lineNamesEndpoint(line); endpoint != "" {
					traces = append(traces, transport.RemovalTrace{Kind: "web_config", Path: path, Line: number, Text: boundedTraceText(line), Endpoint: endpoint})
					if len(traces) >= removalTraceCap {
						file.Close()
						return traces, ok
					}
				}
			}
			if scanner.Err() != nil {
				ok = false
			}
			file.Close()
		}
	}
	return traces, ok
}

// unitTraces reads the owner's unit files and drop-ins, and the
// <unit>.wants / .requires directories, for a dependency on an own unit.
func unitTraces(scope removalTraceScope) ([]transport.RemovalTrace, bool) {
	traces := []transport.RemovalTrace{}
	entries, err := os.ReadDir(removalTraceUnitDir)
	if err != nil {
		if os.IsNotExist(err) {
			return traces, true
		}
		return traces, false
	}
	ok := true
	add := func(unit, relation, path string) {
		for _, existing := range traces {
			if existing.Unit == unit && existing.Relation == relation {
				return
			}
		}
		traces = append(traces, transport.RemovalTrace{Kind: "unit", Unit: unit, Relation: relation, Path: path})
	}
	readUnitFile := func(unit, path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			ok = false
			return
		}
		if len(data) > removalTraceFileSize {
			data = data[:removalTraceFileSize]
		}
		for _, line := range strings.Split(string(data), "\n") {
			match := unitDependencyKey.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			for _, named := range strings.Fields(match[2]) {
				if scope.ownUnit(named) {
					add(unit, match[1], path)
				}
			}
		}
	}
	for _, entry := range entries {
		name := entry.Name()
		full := filepath.Join(removalTraceUnitDir, name)
		switch {
		case ownedByCelikPanel(name):
			continue
		case entry.Type().IsRegular() && isDependentUnitName(name):
			unit := name
			if !scope.ownUnit(unit) {
				readUnitFile(unit, full)
			}
		case entry.IsDir() && strings.HasSuffix(name, ".d"):
			unit := strings.TrimSuffix(name, ".d")
			if !isDependentUnitName(unit) || scope.ownUnit(unit) || ownedByCelikPanel(unit) {
				continue
			}
			dropins, err := os.ReadDir(full)
			if err != nil {
				ok = false
				continue
			}
			for _, dropin := range dropins {
				if strings.HasSuffix(dropin.Name(), ".conf") {
					readUnitFile(unit, filepath.Join(full, dropin.Name()))
				}
			}
		case entry.IsDir() && (strings.HasSuffix(name, ".wants") || strings.HasSuffix(name, ".requires")):
			// <unit>.wants/<own unit>: the owner's unit wants it. A target's
			// .wants is the system starting it at boot, not a dependent.
			unit := strings.TrimSuffix(strings.TrimSuffix(name, ".wants"), ".requires")
			if !isDependentUnitName(unit) || scope.ownUnit(unit) || ownedByCelikPanel(unit) {
				continue
			}
			relation := "Wants"
			if strings.HasSuffix(name, ".requires") {
				relation = "Requires"
			}
			links, err := os.ReadDir(full)
			if err != nil {
				ok = false
				continue
			}
			for _, link := range links {
				if scope.ownUnit(link.Name()) {
					add(unit, relation, filepath.Join(full, link.Name()))
				}
			}
		}
		if len(traces) >= removalTraceCap {
			break
		}
	}
	sort.SliceStable(traces, func(i, j int) bool { return traces[i].Unit < traces[j].Unit })
	return traces, ok
}

// isDependentUnitName: a unit that can depend on another as a workload
// (services, sockets, timers, mounts, paths); targets and slices group units
// and are not dependents.
func isDependentUnitName(name string) bool {
	for _, suffix := range []string{".service", ".socket", ".timer", ".mount", ".path"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

type ssUser struct {
	name string
	pid  int
}

func parseSSUsers(field string) []ssUser {
	users := []ssUser{}
	for _, match := range ssUsers.FindAllStringSubmatch(field, -1) {
		pid, _ := strconv.Atoi(match[2])
		users = append(users, ssUser{name: match[1], pid: pid})
	}
	return users
}

func loopbackAddress(address string) bool {
	host := address
	if at := strings.LastIndex(address, ":"); at >= 0 {
		host = address[:at]
	}
	host = strings.Trim(host, "[]")
	host = strings.TrimPrefix(host, "::ffff:")
	return strings.HasPrefix(host, "127.") || host == "::1" || host == "localhost"
}

func addressPort(address string) int {
	at := strings.LastIndex(address, ":")
	if at < 0 {
		return 0
	}
	port, _ := strconv.Atoi(address[at+1:])
	return port
}

// connectionTraces names the local programs connected right now to the
// component's sockets (ss -Hxp) and, over loopback, to its ports
// (ss -Htnp). A program at the component's end of a connection is the
// component itself and is not named.
func connectionTraces(ctx context.Context, scope removalTraceScope) ([]transport.RemovalTrace, bool) {
	traces := []transport.RemovalTrace{}
	ok := true
	ownName := func(name string) bool { return ownedByCelikPanel(name) || scope.ownUnit(name) }
	add := func(user ssUser, endpoint string, servers map[int]bool) {
		if servers[user.pid] || ownName(user.name) {
			return
		}
		for _, existing := range traces {
			if existing.PID == user.pid && existing.Endpoint == endpoint {
				return
			}
		}
		traces = append(traces, transport.RemovalTrace{Kind: "process", Process: user.name, PID: user.pid, Endpoint: endpoint})
	}
	if len(scope.facts.Sockets) > 0 {
		out, err := removalTraceSS(ctx, "-Hxp")
		if err != nil {
			ok = false
		} else {
			// Server ends: local path is the component's socket. Their peer
			// inodes are the clients' local inodes.
			type row struct {
				local, localInode, peerInode string
				users                        []ssUser
			}
			rows := []row{}
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Fields(line)
				if len(fields) < 8 {
					continue
				}
				users := []ssUser{}
				if len(fields) > 8 {
					users = parseSSUsers(strings.Join(fields[8:], " "))
				}
				rows = append(rows, row{local: fields[4], localInode: fields[5], peerInode: fields[7], users: users})
			}
			clients := map[string]string{} // client inode -> socket path
			servers := map[int]bool{}
			for _, r := range rows {
				for _, glob := range scope.facts.Sockets {
					if matched, _ := filepath.Match(glob, r.local); matched {
						if r.peerInode != "0" && r.peerInode != "*" {
							clients[r.peerInode] = r.local
						}
						for _, user := range r.users {
							servers[user.pid] = true
						}
					}
				}
			}
			for _, r := range rows {
				if socket, isClient := clients[r.localInode]; isClient {
					for _, user := range r.users {
						add(user, socket, servers)
					}
				}
			}
		}
	}
	if len(scope.facts.Ports) > 0 {
		out, err := removalTraceSS(ctx, "-Htnp")
		if err != nil {
			ok = false
		} else {
			ports := map[int]bool{}
			for _, port := range scope.facts.Ports {
				ports[port] = true
			}
			type row struct {
				local, peer string
				users       []ssUser
			}
			rows := []row{}
			servers := map[int]bool{}
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Fields(line)
				// State Recv-Q Send-Q Local Peer [users]
				if len(fields) < 5 || fields[0] != "ESTAB" {
					continue
				}
				users := []ssUser{}
				if len(fields) > 5 {
					users = parseSSUsers(strings.Join(fields[5:], " "))
				}
				rows = append(rows, row{local: fields[3], peer: fields[4], users: users})
				if ports[addressPort(fields[3])] {
					for _, user := range users {
						servers[user.pid] = true
					}
				}
			}
			for _, r := range rows {
				port := addressPort(r.peer)
				if !ports[port] || !loopbackAddress(r.peer) || !loopbackAddress(r.local) {
					continue
				}
				for _, user := range r.users {
					add(user, r.peer, servers)
				}
			}
		}
	}
	sort.SliceStable(traces, func(i, j int) bool {
		if traces[i].Process != traces[j].Process {
			return traces[i].Process < traces[j].Process
		}
		return traces[i].PID < traces[j].PID
	})
	return traces, ok
}

// removalTraceReader reads the traces for the preview (a variable for tests).
var removalTraceReader = collectRemovalTraces

// removalTraceVersion is the version a per-version package names ("8.4" for
// php8.4-fpm, "17" for postgresql-17), or "" for the whole component.
func removalTraceVersion(svc *core.ManagedService, pkg string) string {
	if pkg == "" {
		return ""
	}
	name := pkg
	if svc != nil && svc.Repo != nil && svc.Repo.PackagePattern != "" {
		if re, err := regexp.Compile(svc.Repo.PackagePattern); err == nil {
			if match := re.FindStringSubmatch(pkg); len(match) > 1 {
				name = match[1]
			}
		}
	}
	return versionDigits.FindString(name)
}

var versionDigits = regexp.MustCompile(`[0-9]+(.[0-9]+)*`)
