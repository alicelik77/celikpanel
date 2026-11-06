package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// --- Machine detail -----------------------------------------------------------

func TestNetworkAndDiskRatesSkipVirtualDevicesAndCounterResets(t *testing.T) {
	netDev := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 999999 1 0 0 0 0 0 0 999999 1 0 0 0 0 0 0
  eth0: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0
 veth1a2b: 500000 1 0 0 0 0 0 0 500000 1 0 0 0 0 0 0
  ens4: 300 1 0 0 0 0 0 0 400 1 0 0 0 0 0 0
`
	rx, tx, ok := parseNetDev(netDev)
	if !ok || rx != 1300 || tx != 2400 {
		t.Fatalf("rx %d tx %d ok %v", rx, tx, ok)
	}
	diskstats := `   8       0 sda 100 0 2000 0 50 0 4000 0 0 0 0
   8       1 sda1 100 0 2000 0 50 0 4000 0 0 0 0
 253       0 dm-0 100 0 2000 0 50 0 4000 0 0 0 0
   7       0 loop0 100 0 9999 0 50 0 9999 0 0 0 0
 259       0 nvme0n1 1 0 10 0 1 0 20 0 0 0 0
`
	whole := map[string]bool{"sda": true, "nvme0n1": true, "dm-0": true, "loop0": true}
	include := func(name string) bool {
		return whole[name] && !strings.HasPrefix(name, "dm-") && !strings.HasPrefix(name, "loop")
	}
	read, written, ok := parseDiskStats(diskstats, include)
	if !ok || read != 2010*512 || written != 4020*512 {
		t.Fatalf("read %d written %d", read, written)
	}
	prev := ioCounters{netRx: 1000, netTx: 1000, diskRead: 0, diskWrit: 3000, haveNet: true, haveDisk: true}
	cur := ioCounters{netRx: 4000, netTx: 500, diskRead: 6000, diskWrit: 6000, haveNet: true, haveDisk: true}
	rates := computeIORates(prev, cur, 30)
	if *rates.netRx != 100 || rates.netTx != nil || *rates.diskRead != 200 || *rates.diskWrite != 100 {
		t.Fatalf("rates %+v", rates)
	}
	if r := computeIORates(ioCounters{}, cur, 30); r.netRx != nil || r.diskRead != nil {
		t.Fatal("a missing first reading gave a rate")
	}
}

func TestProcessSnapshotNamesTheBusiestWithoutTheCommandLine(t *testing.T) {
	stat := func(pid int, name string, ticks, rss int) string {
		fields := make([]string, 22)
		for i := range fields {
			fields[i] = "0"
		}
		fields[0] = "S"
		fields[11] = strconv.Itoa(ticks)
		fields[21] = strconv.Itoa(rss)
		return strconv.Itoa(pid) + " (" + name + ") " + strings.Join(fields, " ")
	}
	parsed, ok := parseProcStat(stat(42, "php-fpm: pool (x)", 300, 10), 4096)
	if !ok || parsed.pid != 42 || parsed.name != "php-fpm: pool (x)" || parsed.ticks != 300 || parsed.rss != 40960 {
		t.Fatalf("parsed %+v %v", parsed, ok)
	}
	before := map[int]procStat{1: {pid: 1, name: "a", ticks: 0}, 2: {pid: 2, name: "b", ticks: 0}, 3: {pid: 3, name: "c", ticks: 0, rss: 900}}
	after := map[int]procStat{1: {pid: 1, name: "a", ticks: 50}, 2: {pid: 2, name: "b", ticks: 200}, 3: {pid: 3, name: "c", ticks: 10, rss: 900},
		4: {pid: 4, name: "d", ticks: 5}, 5: {pid: 5, name: "e"}, 6: {pid: 6, name: "f"}, 7: {pid: 7, name: "g", rss: 5}}
	restore := processOwner
	t.Cleanup(func() { processOwner = restore })
	processOwner = func(int) string { return "www-data" }
	top := topProcesses(before, after, 1, 2, "cpu")
	if len(top) != snapshotTop || top[0].PID != 2 || top[0].CPU != 100 || top[1].PID != 1 || top[1].CPU != 25 || top[0].User != "www-data" {
		t.Fatalf("top by cpu %+v", top)
	}
	if top := topProcesses(before, after, 1, 2, "memory"); top[0].PID != 3 || top[1].PID != 7 {
		t.Fatalf("top by memory %+v", top)
	}

	// The snapshotter: a crossing takes one snapshot, then waits ten minutes.
	root := t.TempDir()
	restoreRoot := machineProcRoot
	t.Cleanup(func() { machineProcRoot = restoreRoot })
	machineProcRoot = root
	for pid, line := range map[string]string{"1": stat(1, "systemd", 10, 100), "77": stat(77, "mysqld", 500, 9000)} {
		if err := os.MkdirAll(filepath.Join(root, pid), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, pid, "stat"), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &processSnapshotter{}
	now := time.Now()
	s.consider(now, 40, 50)
	if _, _, _, _, ok := s.finish(2); ok {
		t.Fatal("a snapshot without a crossing")
	}
	s.consider(now, 96, 50)
	reason, value, _, processes, ok := s.finish(2)
	if !ok || reason != "cpu" || value != 96 || len(processes) != 2 {
		t.Fatalf("snapshot %v %v %v %+v", ok, reason, value, processes)
	}
	s.consider(now.Add(5*time.Minute), 97, 50)
	if _, _, _, _, ok := s.finish(2); ok {
		t.Fatal("a second CPU snapshot within ten minutes")
	}
	s.consider(now.Add(5*time.Minute), 10, 95)
	if reason, _, _, _, ok := s.finish(2); !ok || reason != "memory" {
		t.Fatal("memory crossing took no snapshot")
	}
	// With /proc hiding other users' processes (no pid 1) nothing is stored.
	if err := os.RemoveAll(filepath.Join(root, "1")); err != nil {
		t.Fatal(err)
	}
	hidden := &processSnapshotter{}
	hidden.consider(now, 99, 99)
	if _, _, _, _, ok := hidden.finish(2); ok || !hidden.disabled {
		t.Fatal("a hidden process list gave a snapshot")
	}

	_, db := metricsTestDB(t)
	if err := insertProcessSnapshot(context.Background(), db, now, reason, value, processes); err != nil {
		t.Fatal(err)
	}
	views, err := readProcessSnapshots(context.Background(), db, now.Add(-time.Hour), 20)
	if err != nil || len(views) != 1 || len(views[0].Processes) != 2 || views[0].Processes[1].Name != "mysqld" {
		t.Fatalf("stored %+v %v", views, err)
	}
}

func TestLatestSampleCarriesNetworkAndDiskRates(t *testing.T) {
	_, db := metricsTestDB(t)
	rx, wr := 1250.0, 40960.0
	if err := insertMetricsSample(context.Background(), db, metricsRow{
		at: time.Now(), cpu: 3, memTotal: 1, diskTotal: 1, io: ioRates{netRx: &rx, diskWrite: &wr},
	}); err != nil {
		t.Fatal(err)
	}
	latest, err := readMetricsLatest(context.Background(), db)
	if err != nil || latest.NetRx == nil || *latest.NetRx != rx || latest.NetTx != nil || *latest.DiskWrite != wr {
		t.Fatalf("latest %+v %v", latest, err)
	}
}

// --- Services -----------------------------------------------------------------

func TestServiceHealthGivesCPUShareRestartsAndALiveStatus(t *testing.T) {
	serviceHealth = serviceHealthState{}
	t.Cleanup(func() { serviceHealth = serviceHealthState{} })
	u := func(state string, restarts int64, nanos uint64) []transport.ServiceHealthUnit {
		return []transport.ServiceHealthUnit{{Unit: "nginx", ActiveState: state, SubState: "running", Restarts: &restarts, CPUNanos: &nanos}}
	}
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	serviceHealth.record(now, u("active", 0, 0), nil, 2)
	if _, ok := serviceHealth.cpu["nginx"]; ok {
		t.Fatal("the first reading has a CPU share")
	}
	// 30 s of CPU time within one minute on 2 vCPU is 25 % of the machine.
	serviceHealth.record(now.Add(time.Minute), u("active", 1, uint64(30*time.Second)), nil, 2)
	if got := serviceHealth.cpu["nginx"]; got != 25 {
		t.Fatalf("cpu = %v", got)
	}
	serviceHealth.record(now.Add(2*time.Minute), u("active", 3, uint64(30*time.Second)), nil, 2)
	serviceHealth.record(now.Add(3*time.Minute), u("activating", 4, uint64(30*time.Second)), nil, 2)
	if n := restartsWithin(serviceHealth.restarts["nginx"], now.Add(3*time.Minute)); n != 4 {
		t.Fatalf("restarts in the hour = %d", n)
	}
	problems := serviceRestartProblems([]ManagedServiceResponse{{ID: "nginx", Name: "nginx", Unit: "nginx"}}, now.Add(3*time.Minute))
	if len(problems) != 1 || problems[0].Kind != "service_restarting" || problems[0].Value != 4 {
		t.Fatalf("problems %+v", problems)
	}
	observed := overlayLiveServiceStatus([]serviceObservation{{ID: "nginx", IsInstalled: true, Status: "active (running)", Unit: "nginx"}}, now.Add(3*time.Minute))
	if observed[0].Status != "activating (running)" {
		t.Fatalf("live status %q", observed[0].Status)
	}
	// A reading older than three minutes is not used.
	observed = overlayLiveServiceStatus([]serviceObservation{{ID: "nginx", IsInstalled: true, Status: "active (running)", Unit: "nginx"}}, now.Add(10*time.Minute))
	if observed[0].Status != "active (running)" {
		t.Fatal("a stale reading replaced the scan")
	}
	// A failed reading keeps what was known and says when it failed.
	serviceHealth.record(now.Add(4*time.Minute), nil, errors.New("agent unreachable"), 2)
	if serviceHealth.failedAt.IsZero() || serviceHealth.units["nginx"].ActiveState != "activating" {
		t.Fatal("a failed reading lost the last state")
	}
	// A counter that went back (started by hand) counts from there.
	marks := []serviceRestartMark{{now, 5}, {now.Add(time.Minute), 0}, {now.Add(2 * time.Minute), 1}}
	if n := restartsWithin(marks, now.Add(3*time.Minute)); n != 1 {
		t.Fatalf("after a reset = %d", n)
	}
}

func TestServiceHealthEndpointServesTheStoredReading(t *testing.T) {
	p, db := metricsTestDB(t)
	serviceHealth = serviceHealthState{}
	t.Cleanup(func() { serviceHealth = serviceHealthState{} })
	w := httptest.NewRecorder()
	p.handleMonitoringServices(w, httptest.NewRequest(http.MethodGet, "/api/v1/metrics/services", nil))
	var report serviceHealthReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil || report.State != "not_scanned" {
		t.Fatalf("no scan: %s %v", w.Body, err)
	}
	doc, _ := json.Marshal(scanCacheDoc{Observations: []serviceObservation{
		{ID: "nginx", IsInstalled: true, Status: "active (running)", Unit: "nginx"},
	}})
	if _, err := db.Exec(`INSERT INTO service_scan_cache (id, data, scanned_at) VALUES (1, ?, ?)`, string(doc), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	restore := readServiceHealthFromAgent
	t.Cleanup(func() { readServiceHealthFromAgent = restore })
	asked := []string{}
	readServiceHealthFromAgent = func(_ *Panel, _ context.Context, units []string) (transport.ServiceHealthResponse, error) {
		asked = append(asked, units...)
		mem := uint64(64 << 20)
		return transport.ServiceHealthResponse{Batched: true, Units: []transport.ServiceHealthUnit{
			{Unit: "nginx", ActiveState: "active", SubState: "running", ActiveSince: "2026-11-06T08:00:00Z", MemoryBytes: &mem},
		}}, nil
	}
	w = httptest.NewRecorder()
	p.handleMonitoringServices(w, httptest.NewRequest(http.MethodGet, "/api/v1/metrics/services", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil || report.State != "checking" {
		t.Fatalf("before the first reading: %s", w.Body)
	}
	if len(asked) != 0 {
		t.Fatal("a page read asked systemd")
	}
	p.refreshServiceHealth(context.Background(), time.Now())
	w = httptest.NewRecorder()
	p.handleMonitoringServices(w, httptest.NewRequest(http.MethodGet, "/api/v1/metrics/services", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil || report.State != "known" || len(report.Services) != 1 ||
		report.Services[0].ActiveSince == "" || *report.Services[0].MemoryBytes != 64<<20 {
		t.Fatalf("after: %s", w.Body)
	}
	if strings.Join(asked, ",") != "nginx" {
		t.Fatalf("asked %v", asked)
	}
}

// --- Site checks ----------------------------------------------------------------

func pointSiteChecksAt(t *testing.T, address string) {
	t.Helper()
	restoreAddress, restoreWait := siteCheckAddress, siteCheckRetryWait
	t.Cleanup(func() { siteCheckAddress, siteCheckRetryWait = restoreAddress, restoreWait })
	siteCheckAddress = func(string) string { return address }
	siteCheckRetryWait = func() {}
}

func TestSiteCheckTellsUpDownAndTheServedCertificate(t *testing.T) {
	status := http.StatusOK
	var hosts []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts = append(hosts, r.Host)
		w.WriteHeader(status)
	}))
	defer server.Close()
	pointSiteChecksAt(t, server.Listener.Addr().String())

	result := checkSite(context.Background(), hostedSite{domain: "example.com", https: true}, time.Now)
	if !result.up || result.status != 200 || result.scheme != "https" || hosts[0] != "example.com" {
		t.Fatalf("up: %+v %v", result, hosts)
	}
	// httptest's certificate names example.com and is valid now.
	if result.certValid == nil || !*result.certValid || result.certNotAfter.IsZero() {
		t.Fatalf("certificate: %+v", result)
	}
	if other := checkSite(context.Background(), hostedSite{domain: "other.test", https: true}, time.Now); other.certValid == nil || *other.certValid {
		t.Fatalf("a certificate for another name was valid: %+v", other)
	}
	status = http.StatusMovedPermanently
	if result := checkSite(context.Background(), hostedSite{domain: "example.com", https: true}, time.Now); !result.up {
		t.Fatal("a redirect is a serving site")
	}
	status = http.StatusBadGateway
	if result := checkSite(context.Background(), hostedSite{domain: "example.com", https: true}, time.Now); result.up || result.reason != "http_5xx" {
		t.Fatalf("502: %+v", result)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := listener.Addr().String()
	listener.Close()
	pointSiteChecksAt(t, closed)
	if result := checkSite(context.Background(), hostedSite{domain: "example.com"}, time.Now); result.up || result.reason != "refused" {
		t.Fatalf("refused: %+v", result)
	}
}

func TestSiteChecksAreStoredWithDownSinceAndBecomeProblems(t *testing.T) {
	p, db := metricsTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	valid, invalid := true, false
	write := func(at time.Time, results ...siteCheckResult) {
		t.Helper()
		if err := storeSiteChecks(ctx, db, results, at); err != nil {
			t.Fatal(err)
		}
	}
	write(now,
		siteCheckResult{domain: "down.example", scheme: "https", reason: "timeout"},
		siteCheckResult{domain: "soon.example", scheme: "https", up: true, status: 200, certNotAfter: now.Add(5 * 24 * time.Hour), certValid: &valid},
		siteCheckResult{domain: "wrong.example", scheme: "https", up: true, status: 200, certNotAfter: now.Add(60 * 24 * time.Hour), certValid: &invalid},
		siteCheckResult{domain: "fine.example", scheme: "https", up: true, status: 200, certNotAfter: now.Add(60 * 24 * time.Hour), certValid: &valid},
		siteCheckResult{domain: "gone.example", scheme: "http", up: true, status: 200},
	)
	later := now.Add(5 * time.Minute)
	write(later,
		siteCheckResult{domain: "down.example", scheme: "https", reason: "refused"},
		siteCheckResult{domain: "soon.example", scheme: "https", up: true, status: 200, certNotAfter: now.Add(5 * 24 * time.Hour), certValid: &valid},
		siteCheckResult{domain: "wrong.example", scheme: "https", up: true, status: 200, certNotAfter: now.Add(60 * 24 * time.Hour), certValid: &invalid},
		siteCheckResult{domain: "fine.example", scheme: "https", up: true, status: 200, certNotAfter: now.Add(60 * 24 * time.Hour), certValid: &valid},
	)
	if n := countRows(t, db, `SELECT COUNT(*) FROM site_uptime WHERE domain = 'gone.example'`); n != 0 {
		t.Fatal("a site no longer hosted kept its row")
	}
	// The certificate records already name soon.example: not repeated.
	known := []monitoringProblem{{Key: "certificate:soon.example", Subject: "soon.example"}}
	problems, unknown := siteProblems(ctx, db, later, known)
	if unknown != nil || len(problems) != 2 {
		t.Fatalf("problems %+v unknown %+v", problems, unknown)
	}
	down, wrong := problems[0], problems[1]
	if down.Kind != "site_down" || down.Since != metricsTime(now) || down.Name != "refused" || down.Severity != "critical" {
		t.Fatalf("down %+v", down)
	}
	if wrong.Kind != "site_certificate" || wrong.Name != "invalid" || wrong.Subject != "wrong.example" {
		t.Fatalf("wrong %+v", wrong)
	}
	if problems, _ := siteProblems(ctx, db, later, nil); len(problems) != 3 {
		t.Fatalf("without the certificate records: %+v", problems)
	}
	// Up again: down_since is cleared.
	write(later.Add(5*time.Minute), siteCheckResult{domain: "down.example", scheme: "https", up: true, status: 200})
	var since sql.NullString
	if err := db.QueryRow(`SELECT down_since FROM site_uptime WHERE domain = 'down.example'`).Scan(&since); err != nil || since.Valid {
		t.Fatalf("down_since %v %v", since, err)
	}
	// Results three rounds old are not the current state.
	if _, unknown := siteProblems(ctx, db, later.Add(time.Hour), nil); unknown == nil || unknown.Source != "sites" {
		t.Fatalf("stale results: %+v", unknown)
	}
	w := httptest.NewRecorder()
	p.handleMonitoringSites(w, httptest.NewRequest(http.MethodGet, "/api/v1/metrics/sites", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"enabled":true`) || !strings.Contains(w.Body.String(), "down.example") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestHostedSitesToCheckSkipsSuspendedAndDeletingDomains(t *testing.T) {
	_, db := metricsTestDB(t)
	sites, err := hostedSitesToCheck(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 0 {
		t.Fatalf("an empty server has sites: %+v", sites)
	}
}
