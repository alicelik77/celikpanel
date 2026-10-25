package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/supportdata"
	"github.com/alicelik/celikpanel/internal/transport"
)

// PostgreSQL majors side by side (alpha.108; the owner's words of
// 2026-10-25: "postgresql 16 başka 17 başka 18 başka; farklı portlara
// kurulmalı; portlar düzenlenebilmeli; 16 varken 17'yi başka porta kurar,
// veritabanlarını taşır, sonra 16'yı kaldırır"). This replaces D-038 §7's
// major upgrade flow: no pg_upgrade, no conversion in place. On Debian and
// Ubuntu, postgresql-common runs every major as its own cluster
// (/etc/postgresql/<major>/<name>, /var/lib/postgresql/<major>/<name>) on its
// own port; a new postgresql-NN package's postinst creates NN/main on the
// next free port from 5432 up. This file reads the clusters (pg_lsclusters
// and each cluster's own postgresql.conf), says which majors can be
// installed now and on which port, and changes one cluster's port with proof
// and rollback. Moving a database is pg_move.go.
//
// Every command is bounded. A cluster is asked as its own owner (peer
// authentication over the local socket), statements go on standard input,
// never in the argument list (R-062), and no output that could hold a
// password is logged or returned.
//
// PostgreSQL ana sürümleri yan yana: her ana sürüm kendi portunda kendi
// kümesidir. Bu dosya kümeleri okur, kurulabilecek sürümleri ve alacakları
// portu söyler, bir kümenin portunu kanıtla ve geri almayla değiştirir.

const (
	pgDefaultPort      = 5432
	pgReadBound        = 20 * time.Second
	pgRestartBound     = 90 * time.Second
	pgSessionGroupsCap = 20
	pgDatabaseNamesCap = 200
)

// pgProofBound is how long a cluster is given to answer after a restart (a
// variable for tests).
var pgProofBound = 30 * time.Second

var (
	pgConfRoot          = "/etc/postgresql"
	pgCommonConfPath    = "/etc/postgresql-common/createcluster.conf"
	pgLibRoot           = "/usr/lib/postgresql"
	pgPhpPgAdminConf    = "/etc/phppgadmin/config.inc.php"
	pgProcNetTCP        = []string{"/proc/net/tcp", "/proc/net/tcp6"}
	pgVersionRe         = regexp.MustCompile(`^[0-9]{1,3}$`)
	pgClusterNameRe     = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,62}$`)
	pgOwnerRe           = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	pgDatabaseNameRe    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_$-]{0,62}$`)
	pgPortLineRe        = regexp.MustCompile(`^(\s*port\s*=?\s*)'?([0-9]+)'?(\s*(#.*)?)$`)
	pgCreateClusterRe   = regexp.MustCompile(`^\s*create_main_cluster\s*=\s*'?([A-Za-z0-9]+)'?`)
	pgPhpPgAdminPortRe  = regexp.MustCompile(`\$conf\['servers'\]\[0\]\['port'\]\s*=\s*'?([0-9]+)'?`)
	pgVersionPackageRe  = regexp.MustCompile(`^postgresql-([0-9]{1,3})$`)
	pgSystemDatabases   = map[string]bool{"template0": true, "template1": true, "postgres": true}
	pgClusterHostActive = defaultPGClusterHost
)

// pgClusterHost is the narrow seam of everything this file asks the host.
// Tests supply temporary directories and recorded answers.
type pgClusterHost struct {
	// family is the host's package manager ("apt" for postgresql-common).
	family func() string
	// lsclusters is pg_lsclusters' own text without its header line.
	lsclusters func(ctx context.Context) (string, error)
	// query runs one statement on the cluster on port as the cluster's
	// owner, connected to database, and returns psql's unaligned rows.
	query func(ctx context.Context, owner string, port int, database, statement string) (string, error)
	// restart restarts one cluster's unit.
	restart func(ctx context.Context, version, name string) error
	// readFile / writeFile read and write a cluster's configuration file
	// (no-follow; the write keeps the file's mode and owner).
	readFile  func(path string) ([]byte, error)
	writeFile func(path string, content []byte) error
	// listening is the set of TCP ports something listens on now.
	listening func() map[int]bool
	// distroVersions / vendorIndex / installed are the package lists.
	distroVersions func(names map[string]bool) (map[string]map[string]bool, error)
	vendorIndex    func() (map[string]map[string]bool, bool)
	installed      func() map[string][]string
	now            func() time.Time
	// stream runs one PostgreSQL client (pg_dump, pg_restore, pg_dumpall)
	// as owner with the given standard input and output.
	stream func(ctx context.Context, owner, client string, stdin io.Reader, stdout io.Writer, args ...string) error
	// freeBytes is the space left on the file system of a directory.
	freeBytes func(path string) (int64, error)
}

func pgClusterRef(version, name string) string { return version + "/" + name }

func validPGClusterRef(version, name string) bool {
	return pgVersionRe.MatchString(version) && pgClusterNameRe.MatchString(name)
}

// pgClusterTarget is the service-mutation target of one cluster.
func pgClusterTarget(version, name string) string {
	return "pg-" + version + "-" + strings.ToLower(name)
}

// parsePGLsClusters reads pg_lsclusters' text: "Ver Cluster Port Status Owner
// Data-directory Log-file". A line that does not read as a cluster is left
// out (a data directory with a space is not supported and is not guessed).
func parsePGLsClusters(out string) []transport.PostgreSQLCluster {
	var clusters []transport.PostgreSQLCluster
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[0] == "Ver" {
			continue
		}
		port, err := strconv.Atoi(fields[2])
		if err != nil || !validPGClusterRef(fields[0], fields[1]) || port < 1 || port > 65535 {
			continue
		}
		status := fields[3]
		clusters = append(clusters, transport.PostgreSQLCluster{
			Version:       fields[0],
			Name:          fields[1],
			Port:          port,
			Status:        status,
			Online:        strings.HasPrefix(status, "online"),
			Owner:         fields[4],
			DataDirectory: fields[5],
			ConfigPath:    filepath.Join(pgConfRoot, fields[0], fields[1], "postgresql.conf"),
			Databases:     []string{},
		})
	}
	sort.Slice(clusters, func(i, j int) bool {
		vi, _ := strconv.Atoi(clusters[i].Version)
		vj, _ := strconv.Atoi(clusters[j].Version)
		if vi != vj {
			return vi < vj
		}
		return clusters[i].Name < clusters[j].Name
	})
	return clusters
}

// pgConfPort is the effective port line of a postgresql.conf: the last
// active `port = N` (PostgreSQL's own rule), its index among the lines, or
// -1 and 0 when the file names none (the cluster then runs on 5432).
func pgConfPort(content string) (port int, index int) {
	index = -1
	for i, line := range strings.Split(content, "\n") {
		if m := pgPortLineRe.FindStringSubmatch(line); m != nil {
			if value, err := strconv.Atoi(m[2]); err == nil {
				port, index = value, i
			}
		}
	}
	return port, index
}

// pgConfWithPort is content with its effective port line set to port,
// keeping the line's own spacing and comment and every other line as it is.
// A file that names no port gets one line appended.
func pgConfWithPort(content string, port int) (string, string, string) {
	lines := strings.Split(content, "\n")
	_, index := pgConfPort(content)
	if index < 0 {
		after := fmt.Sprintf("port = %d", port)
		if strings.HasSuffix(content, "\n") || content == "" {
			return content + after + "\n", "", after
		}
		return content + "\n" + after + "\n", "", after
	}
	before := lines[index]
	m := pgPortLineRe.FindStringSubmatch(before)
	quoted := strings.Contains(before[len(m[1]):len(before)-len(m[3])], "'")
	value := strconv.Itoa(port)
	if quoted {
		value = "'" + value + "'"
	}
	lines[index] = m[1] + value + m[3]
	return strings.Join(lines, "\n"), before, lines[index]
}

// parseProcNetListening reads /proc/net/tcp{,6}: the local ports in LISTEN.
func parseProcNetListening(content string, into map[int]bool) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || fields[3] != "0A" {
			continue
		}
		_, portHex, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		if port, err := strconv.ParseInt(portHex, 16, 32); err == nil {
			into[int(port)] = true
		}
	}
}

func hostListeningPorts() map[int]bool {
	ports := map[int]bool{}
	for _, path := range pgProcNetTCP {
		if data, err := os.ReadFile(path); err == nil {
			parseProcNetListening(string(data), ports)
		}
	}
	return ports
}

// pgConfiguredPorts are the ports every cluster's own configuration names
// (5432 for one that names none), as postgresql-common's next_free_port
// counts them, plus what pg_lsclusters said.
func pgConfiguredPorts(host pgClusterHost, clusters []transport.PostgreSQLCluster) map[int]bool {
	used := map[int]bool{}
	for _, cluster := range clusters {
		used[cluster.Port] = true
	}
	versions, _ := os.ReadDir(pgConfRoot)
	for _, version := range versions {
		if !version.IsDir() || !pgVersionRe.MatchString(version.Name()) {
			continue
		}
		names, _ := os.ReadDir(filepath.Join(pgConfRoot, version.Name()))
		for _, name := range names {
			if !name.IsDir() || !pgClusterNameRe.MatchString(name.Name()) {
				continue
			}
			data, err := host.readFile(filepath.Join(pgConfRoot, version.Name(), name.Name(), "postgresql.conf"))
			if err != nil {
				continue
			}
			port, index := pgConfPort(string(data))
			if index < 0 {
				port = pgDefaultPort
			}
			used[port] = true
		}
	}
	return used
}

// nextFreePGPort is postgresql-common's rule: from 5432 up, the first port
// no cluster is configured for and nothing listens on.
func nextFreePGPort(configured, listening map[int]bool) int {
	for port := pgDefaultPort; port < 65536; port++ {
		if !configured[port] && !listening[port] {
			return port
		}
	}
	return 0
}

// pgUserDatabasesStatement lists the user databases of a cluster.
const pgUserDatabasesStatement = `SELECT datname FROM pg_database WHERE datistemplate = false AND datname <> 'postgres' ORDER BY datname;`

func pgRows(out string) []string {
	var rows []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimRight(line, "\r"); strings.TrimSpace(line) != "" {
			rows = append(rows, line)
		}
	}
	return rows
}

// fillPGClusterFacts adds each cluster's configured port and its user
// databases (asked of the cluster itself; a cluster that is down or does
// not answer is "not known", never "empty").
func fillPGClusterFacts(ctx context.Context, host pgClusterHost, clusters []transport.PostgreSQLCluster) {
	for i := range clusters {
		cluster := &clusters[i]
		if data, err := host.readFile(cluster.ConfigPath); err == nil {
			if port, index := pgConfPort(string(data)); index >= 0 {
				cluster.ConfiguredPort = port
			}
		}
		if !cluster.Online {
			cluster.DatabasesError = "the cluster is not running"
			continue
		}
		if !pgOwnerRe.MatchString(cluster.Owner) {
			cluster.DatabasesError = "the cluster's owner is not an account CelikPanel can ask as"
			continue
		}
		queryCtx, cancel := context.WithTimeout(ctx, pgReadBound)
		out, err := host.query(queryCtx, cluster.Owner, cluster.Port, "postgres", pgUserDatabasesStatement)
		cancel()
		if err != nil {
			cluster.DatabasesError = "the cluster did not answer: " + boundedText(err.Error(), 200)
			continue
		}
		names := pgRows(out)
		for _, name := range names {
			if !pgSystemDatabases[name] && len(cluster.Databases) < pgDatabaseNamesCap {
				cluster.Databases = append(cluster.Databases, name)
			}
		}
		cluster.DatabasesKnown = true
	}
}

func boundedText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) > limit {
		return text[:limit] + "…"
	}
	return text
}

// pgCreatesMainCluster: the owner's createcluster.conf may say
// create_main_cluster = false; an install then creates no cluster.
func pgCreatesMainCluster(read func(string) ([]byte, error)) bool {
	data, err := read(pgCommonConfPath)
	if err != nil {
		return true
	}
	creates := true
	for _, line := range strings.Split(string(data), "\n") {
		if m := pgCreateClusterRe.FindStringSubmatch(line); m != nil {
			switch strings.ToLower(m[1]) {
			case "false", "off", "no", "0":
				creates = false
			default:
				creates = true
			}
		}
	}
	return creates
}

// pgMajorOffers are the majors not installed: from the dated support table
// (still supported upstream) and whatever the package lists offer, each with
// where it would come from and the port its new cluster would get.
func pgMajorOffers(host pgClusterHost, nextPort int) ([]transport.PostgreSQLMajorOffer, bool) {
	majors := map[string]bool{}
	if table, err := supportdata.Load(); err == nil {
		now := host.now()
		for major, version := range table.Components["postgresql"].Versions {
			if until, err := time.Parse("2006-01-02", version.UpstreamUntil); err == nil && now.Before(until.AddDate(0, 0, 1)) {
				majors[major] = true
			}
		}
	}
	names := map[string]bool{}
	for major := 10; major <= 40; major++ {
		names["postgresql-"+strconv.Itoa(major)] = true
	}
	distro, _ := host.distroVersions(names)
	vendor, vendorOn := host.vendorIndex()
	for name := range names {
		if m := pgVersionPackageRe.FindStringSubmatch(name); m != nil && (len(distro[name]) > 0 || len(vendor[name]) > 0) {
			majors[m[1]] = true
		}
	}
	installed := host.installed()
	var offers []transport.PostgreSQLMajorOffer
	for major := range majors {
		pkg := "postgresql-" + major
		if len(installed[pkg]) > 0 {
			continue
		}
		offer := transport.PostgreSQLMajorOffer{Major: major, Package: pkg, Port: nextPort}
		if versions := distro[pkg]; len(versions) > 0 {
			offer.Origin, offer.Candidate = "distribution", newestDebVersion(versions)
		} else if versions := vendor[pkg]; vendorOn && len(versions) > 0 {
			offer.Origin, offer.Candidate = "pgdg", newestDebVersion(versions)
		} else if !vendorOn {
			offer.NeedsRepository = true
		}
		// A cluster of this major kept on disk from an earlier removal is used
		// again by the install, on its own configured port.
		if data, err := host.readFile(filepath.Join(pgConfRoot, major, "main", "postgresql.conf")); err == nil {
			offer.KeptCluster = true
			if port, index := pgConfPort(string(data)); index >= 0 {
				offer.Port = port
			} else {
				offer.Port = pgDefaultPort
			}
		}
		offers = append(offers, offer)
	}
	sort.Slice(offers, func(i, j int) bool {
		a, _ := strconv.Atoi(offers[i].Major)
		b, _ := strconv.Atoi(offers[j].Major)
		return a > b
	})
	return offers, vendorOn
}

func newestDebVersion(set map[string]bool) string {
	newest := ""
	for version := range set {
		if newest == "" || debVersionCompare(version, newest) > 0 {
			newest = version
		}
	}
	return newest
}

func pgClustersRead(ctx context.Context, host pgClusterHost) transport.PostgreSQLClustersResponse {
	resp := transport.PostgreSQLClustersResponse{Clusters: []transport.PostgreSQLCluster{}, Offers: []transport.PostgreSQLMajorOffer{}, UsedPorts: []int{}}
	if host.family() != "apt" {
		resp.Reason = "Several PostgreSQL majors side by side need Debian or Ubuntu's postgresql-common, which runs each major as its own cluster on its own port. This system runs one PostgreSQL, so CelikPanel does not offer it here."
		return resp
	}
	readCtx, cancel := context.WithTimeout(ctx, pgReadBound)
	out, err := host.lsclusters(readCtx)
	cancel()
	if err != nil {
		resp.Reason = "pg_lsclusters is not available, so PostgreSQL (postgresql-common) is not installed here; install PostgreSQL first."
		resp.Error = boundedText(err.Error(), 200)
		return resp
	}
	resp.Supported = true
	resp.Clusters = parsePGLsClusters(out)
	fillPGClusterFacts(ctx, host, resp.Clusters)
	configured := pgConfiguredPorts(host, resp.Clusters)
	listening := host.listening()
	resp.NextPort = nextFreePGPort(configured, listening)
	for port := range configured {
		resp.UsedPorts = append(resp.UsedPorts, port)
	}
	for port := range listening {
		if !configured[port] && port >= pgDefaultPort && port < pgDefaultPort+100 {
			resp.UsedPorts = append(resp.UsedPorts, port)
		}
	}
	sort.Ints(resp.UsedPorts)
	resp.CreatesCluster = pgCreatesMainCluster(host.readFile)
	resp.Offers, resp.RepositoryOn = pgMajorOffers(host, resp.NextPort)
	return resp
}

// PostgreSQLClustersV1 answers, read only and bounded, every PostgreSQL
// cluster on this server and the majors that can be installed beside them.
func (a *Agent) PostgreSQLClustersV1(_ *transport.Empty, resp *transport.PostgreSQLClustersResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*pgReadBound+10*time.Second)
	defer cancel()
	*resp = pgClustersRead(ctx, pgClusterHostActive())
	return nil
}

// findPGCluster reads the clusters and returns the one named.
func findPGCluster(ctx context.Context, host pgClusterHost, version, name string) (transport.PostgreSQLCluster, []transport.PostgreSQLCluster, error) {
	readCtx, cancel := context.WithTimeout(ctx, pgReadBound)
	out, err := host.lsclusters(readCtx)
	cancel()
	if err != nil {
		return transport.PostgreSQLCluster{}, nil, fmt.Errorf("pg_lsclusters could not be read: %s", boundedText(err.Error(), 200))
	}
	clusters := parsePGLsClusters(out)
	for _, cluster := range clusters {
		if cluster.Version == version && cluster.Name == name {
			return cluster, clusters, nil
		}
	}
	return transport.PostgreSQLCluster{}, clusters, fmt.Errorf("there is no PostgreSQL cluster %s on this server", pgClusterRef(version, name))
}

// pgPortOverrides names the other places that could set the port and win
// over postgresql.conf: a conf.d file and ALTER SYSTEM's postgresql.auto.conf.
func pgPortOverrides(host pgClusterHost, cluster transport.PostgreSQLCluster) []string {
	var found []string
	dir := filepath.Join(filepath.Dir(cluster.ConfigPath), "conf.d")
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if data, err := host.readFile(path); err == nil {
				if _, index := pgConfPort(string(data)); index >= 0 {
					found = append(found, path)
				}
			}
		}
	}
	if cluster.DataDirectory != "" {
		path := filepath.Join(cluster.DataDirectory, "postgresql.auto.conf")
		if data, err := host.readFile(path); err == nil {
			if _, index := pgConfPort(string(data)); index >= 0 {
				found = append(found, path)
			}
		}
	}
	sort.Strings(found)
	return found
}

const pgSessionsStatement = `SELECT COALESCE(datname, ''), COALESCE(usename, ''), COALESCE(host(client_addr), 'local'), COALESCE(application_name, ''), count(*) FROM pg_stat_activity WHERE pid <> pg_backend_pid() AND backend_type = 'client backend' GROUP BY 1, 2, 3, 4 ORDER BY 5 DESC, 1, 2 LIMIT 20;`

func parsePGSessions(out string) []transport.PostgreSQLSession {
	sessions := []transport.PostgreSQLSession{}
	for _, row := range pgRows(out) {
		fields := strings.Split(row, "|")
		if len(fields) != 5 {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(fields[4]))
		if err != nil {
			continue
		}
		sessions = append(sessions, transport.PostgreSQLSession{
			Database: fields[0], User: fields[1], Client: fields[2],
			Application: boundedText(fields[3], 64), Count: count,
		})
		if len(sessions) == pgSessionGroupsCap {
			break
		}
	}
	return sessions
}

// pgPortPlan is the read-only plan of a port change, shared by the preview
// and the change itself so the change cannot do what the preview did not say.
func pgPortPlan(ctx context.Context, host pgClusterHost, version, name string, port int) (transport.PostgreSQLPortPreviewResponse, []byte, string) {
	resp := transport.PostgreSQLPortPreviewResponse{To: port, Sessions: []transport.PostgreSQLSession{}}
	if host.family() != "apt" {
		resp.RefusalCode, resp.Refusal = "UNSUPPORTED_SYSTEM", "changing a PostgreSQL cluster's port needs Debian or Ubuntu's postgresql-common; nothing was changed"
		return resp, nil, ""
	}
	if !validPGClusterRef(version, name) {
		resp.RefusalCode, resp.Refusal = "INVALID_REQUEST", "the cluster is not named correctly"
		return resp, nil, ""
	}
	cluster, clusters, err := findPGCluster(ctx, host, version, name)
	if err != nil {
		resp.RefusalCode, resp.Refusal = "CLUSTER_NOT_FOUND", err.Error()
		return resp, nil, ""
	}
	resp.Cluster = cluster
	resp.ConfigPath = cluster.ConfigPath
	resp.From = cluster.Port
	content, err := host.readFile(cluster.ConfigPath)
	if err != nil {
		resp.RefusalCode, resp.Refusal = "CONFIG_UNREADABLE", fmt.Sprintf("%s could not be read (%s), so nothing was changed", cluster.ConfigPath, boundedText(err.Error(), 160))
		return resp, nil, ""
	}
	resp.ConfigSHA = sha256Hex(content)
	_, resp.LineBefore, resp.LineAfter = pgConfWithPort(string(content), port)
	if data, err := host.readFile(pgPhpPgAdminConf); err == nil {
		if m := pgPhpPgAdminPortRe.FindStringSubmatch(string(data)); m != nil {
			resp.ToolPort, _ = strconv.Atoi(m[1])
		}
	}
	if cluster.Online && pgOwnerRe.MatchString(cluster.Owner) {
		queryCtx, cancel := context.WithTimeout(ctx, pgReadBound)
		out, err := host.query(queryCtx, cluster.Owner, cluster.Port, "postgres", pgSessionsStatement)
		cancel()
		if err == nil {
			resp.Sessions, resp.SessionsOK = parsePGSessions(out), true
		}
	}
	refuse := func(code, text string) (transport.PostgreSQLPortPreviewResponse, []byte, string) {
		resp.RefusalCode, resp.Refusal = code, text
		return resp, content, resp.ConfigSHA
	}
	switch {
	case port < 1024 || port > 65535:
		return refuse("PORT_INVALID", "choose a port from 1024 to 65535; ports below 1024 are for system services")
	case port == cluster.Port:
		return refuse("PORT_UNCHANGED", fmt.Sprintf("cluster %s already runs on port %d", pgClusterRef(version, name), port))
	}
	for _, other := range clusters {
		if other.Port == port {
			return refuse("PORT_IN_USE", fmt.Sprintf("port %d is the port of cluster %s; choose another", port, pgClusterRef(other.Version, other.Name)))
		}
	}
	configured := pgConfiguredPorts(host, clusters)
	if configured[port] {
		return refuse("PORT_IN_USE", fmt.Sprintf("port %d is configured for another PostgreSQL cluster on this server; choose another", port))
	}
	if host.listening()[port] {
		return refuse("PORT_IN_USE", fmt.Sprintf("something on this server already listens on port %d; choose another", port))
	}
	if overrides := pgPortOverrides(host, cluster); len(overrides) > 0 {
		return refuse("PORT_SET_ELSEWHERE", fmt.Sprintf("the port of cluster %s is also set in %s, which wins over postgresql.conf; CelikPanel does not edit that file, so change the port there yourself", pgClusterRef(version, name), strings.Join(overrides, ", ")))
	}
	if !cluster.Online {
		return refuse("CLUSTER_DOWN", fmt.Sprintf("cluster %s is not running, so CelikPanel could not prove a new port; start it first (its own Start on this page), then change the port", pgClusterRef(version, name)))
	}
	return resp, content, resp.ConfigSHA
}

// PostgreSQLClusterPortPreviewV1 is the read-only plan of a port change.
func (a *Agent) PostgreSQLClusterPortPreviewV1(req *transport.PostgreSQLPortPreviewRequest, resp *transport.PostgreSQLPortPreviewResponse) error {
	if req == nil {
		*resp = transport.PostgreSQLPortPreviewResponse{RefusalCode: "INVALID_REQUEST", Refusal: "missing request", Sessions: []transport.PostgreSQLSession{}}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*pgReadBound)
	defer cancel()
	*resp, _, _ = pgPortPlan(ctx, pgClusterHostActive(), req.Version, req.Name, req.Port)
	return nil
}

// pgProveCluster asks the cluster on port, as its owner, which port, major
// and data directory it runs with, until it answers with the expected ones
// or the bound passes.
func pgProveCluster(ctx context.Context, host pgClusterHost, cluster transport.PostgreSQLCluster, port int) error {
	deadline := time.Now().Add(pgProofBound)
	var last error
	for {
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, err := host.query(queryCtx, cluster.Owner, port, "postgres", `SELECT current_setting('port') || '|' || current_setting('server_version_num') || '|' || current_setting('data_directory');`)
		cancel()
		if err == nil {
			last = pgProofMatches(out, cluster, port)
			if last == nil {
				return nil
			}
		} else {
			last = fmt.Errorf("it does not answer on port %d: %s", port, boundedText(err.Error(), 160))
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return last
		}
		select {
		case <-ctx.Done():
			return last
		case <-time.After(time.Second):
		}
	}
}

func pgProofMatches(out string, cluster transport.PostgreSQLCluster, port int) error {
	rows := pgRows(out)
	if len(rows) != 1 {
		return fmt.Errorf("its answer could not be read")
	}
	fields := strings.Split(rows[0], "|")
	if len(fields) != 3 {
		return fmt.Errorf("its answer could not be read")
	}
	if fields[0] != strconv.Itoa(port) {
		return fmt.Errorf("the server on port %d says its port is %s", port, fields[0])
	}
	num, err := strconv.Atoi(fields[1])
	if err != nil || strconv.Itoa(num/10000) != cluster.Version {
		return fmt.Errorf("the server on port %d is not PostgreSQL %s (it says %s)", port, cluster.Version, fields[1])
	}
	if cluster.DataDirectory != "" && filepath.Clean(fields[2]) != filepath.Clean(cluster.DataDirectory) {
		return fmt.Errorf("the server on port %d runs from %s, not from cluster %s's %s", port, fields[2], pgClusterRef(cluster.Version, cluster.Name), cluster.DataDirectory)
	}
	return nil
}

// SetPostgreSQLClusterPortV1 changes one cluster's port in its own
// pg_cluster_port job: only over the configuration file the administrator
// saw (its SHA-256), the single port line changed and every other line kept;
// the cluster is restarted (its connections are interrupted, which the
// dialogue said before) and must answer on the new port as itself. Otherwise
// the previous file is written back, the cluster restarted and proved on its
// old port, and the answer says so.
func (a *Agent) SetPostgreSQLClusterPortV1(req *transport.PostgreSQLSetPortRequest, resp *transport.PostgreSQLSetPortResponse) error {
	*resp = transport.PostgreSQLSetPortResponse{}
	if req == nil || !validPGClusterRef(req.Version, req.Name) || len(req.ExpectedSHA256) != 64 {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "the cluster, the port and the confirmed file are required"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepSetPGClusterPort, pgClusterTarget(req.Version, req.Name), strconv.Itoa(req.Port), "port"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = "MUTATION_REFUSED", err.Error()
		return nil
	}
	defer finishStep()
	setPGClusterPort(ctx, pgClusterHostActive(), req, resp)
	return nil
}

func setPGClusterPort(ctx context.Context, host pgClusterHost, req *transport.PostgreSQLSetPortRequest, resp *transport.PostgreSQLSetPortResponse) {
	plan, content, sha := pgPortPlan(ctx, host, req.Version, req.Name, req.Port)
	resp.From, resp.To = plan.From, req.Port
	if plan.RefusalCode != "" {
		resp.ErrorCode, resp.Error = plan.RefusalCode, plan.Refusal+"; nothing was changed"
		return
	}
	if sha != req.ExpectedSHA256 {
		resp.ErrorCode = "CONFIG_CHANGED"
		resp.Error = fmt.Sprintf("%s changed after you looked at it, so nothing was written; look at the change again and confirm it again", plan.ConfigPath)
		return
	}
	cluster := plan.Cluster
	updated, _, _ := pgConfWithPort(string(content), req.Port)
	if err := host.writeFile(plan.ConfigPath, []byte(updated)); err != nil {
		resp.ErrorCode, resp.Error = "CONFIG_WRITE_FAILED", fmt.Sprintf("%s could not be written (%s); nothing was changed", plan.ConfigPath, boundedText(err.Error(), 160))
		return
	}
	resp.Changed = true
	restartCtx, cancel := context.WithTimeout(ctx, pgRestartBound)
	restartErr := host.restart(restartCtx, cluster.Version, cluster.Name)
	cancel()
	var proofErr error
	if restartErr == nil {
		proofErr = pgProveCluster(ctx, host, cluster, req.Port)
	}
	if restartErr == nil && proofErr == nil {
		resp.Proved = true
		return
	}
	cause := restartErr
	if cause == nil {
		cause = proofErr
	}
	// Put the previous file back, byte for byte, and prove the old port.
	resp.Changed = false
	resp.ErrorCode = "PROOF_FAILED"
	if err := host.writeFile(plan.ConfigPath, content); err != nil {
		resp.Error = fmt.Sprintf("cluster %s did not come back on port %d (%s), and writing the previous %s back failed too (%s); put back the line %q yourself and restart the cluster",
			pgClusterRef(cluster.Version, cluster.Name), req.Port, boundedText(cause.Error(), 200), plan.ConfigPath, boundedText(err.Error(), 120), plan.LineBefore)
		return
	}
	resp.RolledBack = true
	restartCtx, cancel = context.WithTimeout(ctx, pgRestartBound)
	restartErr = host.restart(restartCtx, cluster.Version, cluster.Name)
	cancel()
	if restartErr == nil {
		if err := pgProveCluster(ctx, host, cluster, cluster.Port); err == nil {
			resp.Restored = true
		}
	}
	if resp.Restored {
		resp.Error = fmt.Sprintf("cluster %s did not answer on port %d (%s); the previous configuration was written back and it answers on port %d again",
			pgClusterRef(cluster.Version, cluster.Name), req.Port, boundedText(cause.Error(), 200), cluster.Port)
		return
	}
	resp.Error = fmt.Sprintf("cluster %s did not answer on port %d (%s); the previous configuration was written back, but the cluster does not answer on port %d either; read its log (journalctl -u postgresql@%s-%s) and start it",
		pgClusterRef(cluster.Version, cluster.Name), req.Port, boundedText(cause.Error(), 200), cluster.Port, cluster.Version, cluster.Name)
}

// pgRemovalHolds says which clusters of the PostgreSQL removal (one major,
// or every major when major is "") still hold user databases, by name, and
// which could not be asked (D-038 §2: nothing in use is removed).
func pgRemovalHolds(ctx context.Context, host pgClusterHost, major string) (held []string, unknown []string) {
	readCtx, cancel := context.WithTimeout(ctx, pgReadBound)
	out, err := host.lsclusters(readCtx)
	cancel()
	if err != nil {
		// No postgresql-common: no cluster this check can see.
		return nil, nil
	}
	clusters := parsePGLsClusters(out)
	selected := clusters[:0]
	for _, cluster := range clusters {
		if major == "" || cluster.Version == major {
			selected = append(selected, cluster)
		}
	}
	fillPGClusterFacts(ctx, host, selected)
	for _, cluster := range selected {
		ref := pgClusterRef(cluster.Version, cluster.Name)
		switch {
		case !cluster.DatabasesKnown:
			unknown = append(unknown, ref)
		case len(cluster.Databases) > 0:
			held = append(held, ref+": "+strings.Join(cluster.Databases, ", "))
		}
	}
	return held, unknown
}

// pgRemovalRefusal is the removal's refusal while a cluster of the removed
// major(s) holds databases or cannot be asked.
func pgRemovalRefusal(held, unknown []string) (code, message string) {
	switch {
	case len(held) > 0:
		return errCodeRemovalHoldsDatabases, fmt.Sprintf("PostgreSQL still holds databases (%s); move them to another cluster (PostgreSQL → Clusters → Move) or remove them first; nothing was changed", strings.Join(held, "; "))
	case len(unknown) > 0:
		return errCodeRemovalDatabasesUnknown, fmt.Sprintf("cluster %s is not running or did not answer, so CelikPanel cannot see which databases it holds; start it and try again; nothing was changed", strings.Join(unknown, ", "))
	}
	return "", ""
}

const (
	errCodeRemovalHoldsDatabases   = "REMOVAL_HOLDS_DATABASES"
	errCodeRemovalDatabasesUnknown = "REMOVAL_DATABASES_UNKNOWN"
)

// pgRemovalMajor is the major a PostgreSQL removal names ("" for all).
func pgRemovalMajor(svc *core.ManagedService, pkg string) (string, bool) {
	if svc == nil || svc.ID != "postgresql" {
		return "", false
	}
	if pkg == "" {
		return "", true
	}
	if m := pgVersionPackageRe.FindStringSubmatch(pkg); m != nil {
		return m[1], true
	}
	return "", false
}

// pgDistributionOffers reports whether the distribution's own lists offer a
// postgresql-NN package (alpha.108): such a version needs no vendor
// repository, so installing it by its exact name is not refused for want of
// one.
func pgDistributionOffers(pkg string) bool {
	if !pgVersionPackageRe.MatchString(pkg) {
		return false
	}
	distro, err := readDistributionVersions(map[string]bool{pkg: true})
	return err == nil && len(distro[pkg]) > 0
}

// pgCreateClusterPath is postgresql-common's own cluster creator.
var pgCreateClusterPath = "/usr/bin/pg_createcluster"

// pgCreateMainCluster creates NN/main on port with postgresql-common's
// pg_createcluster, without starting it (the install starts its unit).
var pgCreateMainCluster = func(ctx context.Context, version string, port int) error {
	if !pgVersionRe.MatchString(version) || port < 1024 || port > 65535 {
		return fmt.Errorf("invalid cluster %s on port %d", version, port)
	}
	if info, err := os.Stat(pgCreateClusterPath); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not installed", pgCreateClusterPath)
	}
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := serviceMutationCommand(callCtx, pgCreateClusterPath, version, "main", "-p", strconv.Itoa(port))
	cmd.Env = packageCommandEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_createcluster %s main -p %d: %v: %s", version, port, err, boundedText(lastLines(string(out), 3), 300))
	}
	return nil
}

// pgEnsureMainCluster (alpha.108, a real-system finding): with PGDG confined,
// the distribution's postgresql-common stays (Debian 13: 278), and a newer
// major's own install from PGDG (postgresql-18) creates no cluster, so its
// unit cannot start. Unless the owner's createcluster.conf says
// create_main_cluster = false, the Agent then creates NN/main itself, with
// postgresql-common's pg_createcluster, on the port the clusters card showed:
// the first one from 5432 up that no cluster is configured for and nothing
// listens on. An existing NN/main is never touched.
func pgEnsureMainCluster(ctx context.Context, host pgClusterHost, pkg string, create func(context.Context, string, int) error) (created bool, port int, err error) {
	m := pgVersionPackageRe.FindStringSubmatch(pkg)
	if m == nil || host.family() != "apt" {
		return false, 0, nil
	}
	readCtx, cancel := context.WithTimeout(ctx, pgReadBound)
	out, err := host.lsclusters(readCtx)
	cancel()
	if err != nil {
		return false, 0, err
	}
	clusters := parsePGLsClusters(out)
	for _, cluster := range clusters {
		if cluster.Version == m[1] && cluster.Name == "main" {
			return false, cluster.Port, nil
		}
	}
	if !pgCreatesMainCluster(host.readFile) {
		return false, 0, nil
	}
	port = nextFreePGPort(pgConfiguredPorts(host, clusters), host.listening())
	if port == 0 {
		return false, 0, fmt.Errorf("no free port from %d up", pgDefaultPort)
	}
	if err := create(ctx, m[1], port); err != nil {
		return false, port, err
	}
	return true, port, nil
}

// pgInstallProof proves, after postgresql-NN was installed, that its cluster
// NN/main answers on its port, and names the port. Without the cluster (the
// owner's createcluster.conf creates none) it says so.
var pgInstallProof = func(ctx context.Context, pkg string) (string, error) {
	return pgInstallProofWith(ctx, pgClusterHostActive(), pkg)
}

func pgInstallProofWith(ctx context.Context, host pgClusterHost, pkg string) (string, error) {
	m := pgVersionPackageRe.FindStringSubmatch(pkg)
	if m == nil {
		return "", nil
	}
	cluster, _, err := findPGCluster(ctx, host, m[1], "main")
	if err != nil {
		if !pgCreatesMainCluster(host.readFile) {
			return fmt.Sprintf("no cluster was created for PostgreSQL %s (your %s says create_main_cluster = false)", m[1], pgCommonConfPath), nil
		}
		return "", fmt.Errorf("PostgreSQL %s was installed but its cluster %s/main is not there: %v", m[1], m[1], err)
	}
	if err := pgProveCluster(ctx, host, cluster, cluster.Port); err != nil {
		return "", fmt.Errorf("PostgreSQL %s was installed but its cluster %s/main does not answer on port %d: %v", m[1], m[1], cluster.Port, err)
	}
	return fmt.Sprintf("cluster %s/main answers on port %d", m[1], cluster.Port), nil
}

// pgBuffer keeps the first limit bytes of what is written to it.
type pgBuffer struct {
	bytes.Buffer
	limit int
}

func (b *pgBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}
