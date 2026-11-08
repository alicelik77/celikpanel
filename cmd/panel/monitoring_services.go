package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Per-service health (alpha.127, monitoring stage 2). Once a minute the
// sampler asks the Agent for every installed managed service with ONE batched
// `systemctl show` (Agent.ServiceHealthV1, read only): state, running since,
// restart count, memory and CPU time from systemd's own cgroup accounting.
// The Monitoring page and the "Problems now" list read the stored answer; no
// page load waits for systemd. CPU is the share of the whole machine between
// two readings, so it reads like the CPU card; the first reading after a
// start has none.
//
// The same reading makes "service stopped" current (the component scan it
// replaces is only as new as the last scan) and adds one problem: a service
// systemd restarted three times or more within the last hour.
//
// Hizmet sağlığı: örnekleyici dakikada bir, kurulu her yönetilen hizmet için
// Agent'tan tek bir toplu `systemctl show` ister. Sayfa ve "Şu anki sorunlar"
// saklanan yanıtı okur; hiçbir sayfa systemd'yi beklemez. Son bir saatte üç
// ya da daha çok yeniden başlatılan hizmet bir sorundur.

const (
	serviceHealthFresh       = 3 * time.Minute
	serviceRestartWindow     = time.Hour
	serviceRestartProblemMin = 3
)

// monitoringFlight runs one job at a time; a call while it runs is dropped.
type monitoringFlight struct{ running atomic.Bool }

func (f *monitoringFlight) run(job func()) {
	if !f.running.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer f.running.Store(false)
		job()
	}()
}

var (
	monitoringMinuteFlight monitoringFlight
	siteCheckFlight        monitoringFlight
)

type serviceRestartMark struct {
	at       time.Time
	restarts int64
}

type serviceHealthState struct {
	mu       sync.Mutex
	readAt   time.Time
	failedAt time.Time
	units    map[string]transport.ServiceHealthUnit
	cpu      map[string]float64
	prevCPU  map[string]serviceCPUMark
	restarts map[string][]serviceRestartMark
}

type serviceCPUMark struct {
	at    time.Time
	nanos uint64
}

var serviceHealth serviceHealthState

// readServiceHealthFromAgent is the one Agent call. Variable for tests.
var readServiceHealthFromAgent = func(p *Panel, ctx context.Context, units []string) (transport.ServiceHealthResponse, error) {
	var resp transport.ServiceHealthResponse
	err := p.callAgentContext(ctx, "Agent.ServiceHealthV1", &transport.ServiceHealthRequest{Units: units}, &resp)
	return resp, err
}

// serviceHealthUnit is the unit whose state proves that a service's daemon
// runs (alpha.130). For most services it is the unit the panel acts on. On
// Debian and Ubuntu, Postfix is packaged as a oneshot wrapper postfix.service
// that stays "active (exited)" whatever happens to the mail system, and the
// real master process runs in the default instance postfix@-.service (the
// package's postfix-instance-generator always creates "-"). The wrapper's
// "exited" was read as "Postfix is not running" (Boston, 8 Nov 2026), and a
// dead instance behind an active wrapper would have been read as running.
// Arch and other systems ship postfix.service as the daemon itself.
//
// Start and stop still act on postfix.service: its instances are PartOf it,
// and the Agent acts only on catalogue units.
//
// serviceHealthUnit: bir hizmetin daemon'ının çalıştığını kanıtlayan birim.
// Debian/Ubuntu'da Postfix'in postfix.service'i yalnız bir sarmalayıcıdır
// (her zaman "active (exited)"); gerçek süreç postfix@-.service'tedir.
func serviceHealthUnit(serviceID, pkgFamily, unit string) string {
	if serviceID == "postfix" && pkgFamily == "apt" && unit == "postfix" {
		return postfixDefaultInstanceUnit
	}
	return unit
}

const postfixDefaultInstanceUnit = "postfix@-"

// postfixWrapperUnit is read next to the default instance on apt systems.
// Debian 13's Postfix 3.10 package ships postfix.service as the daemon
// itself (Type=forking, "active (running)") and leaves postfix@-.service
// "inactive (dead)"; reading only the instance called a running Postfix
// stopped (lab, alpha.130, Debian 13 — Frankfurt's system). When the wrapper
// reads "running" it is the daemon and its reading stands for the instance.
//
// Debian 13'te postfix.service daemon'ın kendisidir; "running" okunursa
// örneğin yerine o okunur.
const postfixWrapperUnit = "postfix"

// withPostfixDaemonReading puts the wrapper's reading in place of the
// default instance's when the wrapper itself runs the daemon.
func withPostfixDaemonReading(units []transport.ServiceHealthUnit) []transport.ServiceHealthUnit {
	var wrapper *transport.ServiceHealthUnit
	for i := range units {
		if units[i].Unit == postfixWrapperUnit {
			wrapper = &units[i]
		}
	}
	if wrapper == nil || wrapper.ActiveState != "active" || wrapper.SubState != "running" {
		return units
	}
	out := make([]transport.ServiceHealthUnit, len(units))
	copy(out, units)
	for i := range out {
		if out[i].Unit == postfixDefaultInstanceUnit {
			reading := *wrapper
			reading.Unit = postfixDefaultInstanceUnit
			out[i] = reading
		}
	}
	return out
}

// serviceHealthReady is the scan's own rule (managedServiceUnitReady) applied
// to a live reading: any active state of the unit that proves the daemon is
// up. A oneshot unit such as wg-quick@wg0, or a unit systemd generated from
// an init script, reports "active (exited)" while it is up; PostgreSQL's apt
// wrapper never proves a cluster.
func serviceHealthReady(serviceID, pkgFamily, unit string, reading transport.ServiceHealthUnit) bool {
	status := reading.ActiveState
	if reading.SubState != "" {
		status += " (" + reading.SubState + ")"
	}
	return reading.ActiveState == "active" && managedServiceUnitReady(serviceID, pkgFamily, unit, status)
}

// monitoredServices is the installed managed services of the last component
// scan, each with the unit that proves it runs (serviceHealthUnit), in the
// Components page's order.
func (p *Panel) monitoredServices(ctx context.Context) ([]ManagedServiceResponse, bool) {
	observations, _, unknown := p.problemScanObservations(ctx)
	if unknown != nil {
		return nil, false
	}
	var services []ManagedServiceResponse
	seen := map[string]bool{}
	host := p.managedServiceHostProfile()
	for _, service := range catalogViewForHost(observations, host) {
		if service.Kind != core.KindService || service.IsInstalled == nil || !*service.IsInstalled {
			continue
		}
		if service.Unit == "" {
			service.Unit = service.ID
		}
		service.Unit = serviceHealthUnit(service.ID, host.PackageFamily, service.Unit)
		if !serviceUnitNameOK(service.Unit) || seen[service.Unit] {
			continue
		}
		seen[service.Unit] = true
		services = append(services, service)
	}
	if len(services) > transport.ServiceHealthMaxUnits {
		services = services[:transport.ServiceHealthMaxUnits]
	}
	return services, true
}

// serviceUnitNameOK mirrors the Agent's check so a name it would refuse is
// never sent (one bad name would refuse the whole batch).
func serviceUnitNameOK(name string) bool {
	if name == "" || len(name) > 128 || name[0] == '-' {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == '@', r == '\\', r == ':':
		default:
			return false
		}
	}
	return len(name) < 8 || name[len(name)-8:] != ".service"
}

// refreshServiceHealth reads every monitored unit once.
func (p *Panel) refreshServiceHealth(ctx context.Context, now time.Time) {
	services, ok := p.monitoredServices(ctx)
	if !ok || len(services) == 0 {
		return
	}
	units := make([]string, 0, len(services)+1)
	readsWrapper, readsInstance := false, false
	for _, service := range services {
		units = append(units, service.Unit)
		readsWrapper = readsWrapper || service.Unit == postfixWrapperUnit
		readsInstance = readsInstance || service.Unit == postfixDefaultInstanceUnit
	}
	if readsInstance && !readsWrapper && len(units) < transport.ServiceHealthMaxUnits {
		units = append(units, postfixWrapperUnit)
	}
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := readServiceHealthFromAgent(p, callCtx, units)
	if err == nil && !resp.Batched {
		err = errServiceHealthUnsplit
	}
	serviceHealth.record(now, resp.Units, err, vcpuCount())
}

type serviceHealthError string

func (e serviceHealthError) Error() string { return string(e) }

const errServiceHealthUnsplit = serviceHealthError("systemd's answer could not be split per unit")

func (s *serviceHealthState) record(now time.Time, units []transport.ServiceHealthUnit, err error, vcpus int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		log.Printf("monitoring: service health could not be read: %v", err)
		s.failedAt = now
		return
	}
	if s.prevCPU == nil {
		s.prevCPU, s.restarts = map[string]serviceCPUMark{}, map[string][]serviceRestartMark{}
	}
	if vcpus < 1 {
		vcpus = 1
	}
	units = withPostfixDaemonReading(units)
	s.units, s.cpu = map[string]transport.ServiceHealthUnit{}, map[string]float64{}
	for _, unit := range units {
		s.units[unit.Unit] = unit
		if unit.CPUNanos != nil {
			if prev, ok := s.prevCPU[unit.Unit]; ok && *unit.CPUNanos >= prev.nanos && now.After(prev.at) {
				s.cpu[unit.Unit] = round1(float64(*unit.CPUNanos-prev.nanos) / float64(now.Sub(prev.at).Nanoseconds()) * 100 / float64(vcpus))
			}
			s.prevCPU[unit.Unit] = serviceCPUMark{at: now, nanos: *unit.CPUNanos}
		} else {
			delete(s.prevCPU, unit.Unit)
		}
		if unit.Restarts != nil {
			marks := append(s.restarts[unit.Unit], serviceRestartMark{at: now, restarts: *unit.Restarts})
			for len(marks) > 0 && now.Sub(marks[0].at) > serviceRestartWindow+time.Minute {
				marks = marks[1:]
			}
			s.restarts[unit.Unit] = marks
		}
	}
	s.readAt, s.failedAt = now, time.Time{}
}

// restartsWithin is how many restarts systemd counted over the last hour of
// readings. A counter that went back (systemd resets it when the service is
// started by hand) counts from there.
func restartsWithin(marks []serviceRestartMark, now time.Time) int64 {
	var total int64
	var prev *serviceRestartMark
	for i := range marks {
		mark := marks[i]
		if now.Sub(mark.at) > serviceRestartWindow {
			continue
		}
		if prev != nil && mark.restarts > prev.restarts {
			total += mark.restarts - prev.restarts
		}
		prev = &marks[i]
	}
	return total
}

// liveReading returns a fresh reading of a unit and when it was taken.
func (s *serviceHealthState) liveReading(unit string, now time.Time) (transport.ServiceHealthUnit, time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readAt.IsZero() || now.Sub(s.readAt) > serviceHealthFresh {
		return transport.ServiceHealthUnit{}, time.Time{}, false
	}
	reading, ok := s.units[unit]
	if !ok || reading.ActiveState == "" {
		return transport.ServiceHealthUnit{}, time.Time{}, false
	}
	return reading, s.readAt, true
}

// overlayLiveServiceStatus puts the fresh reading's state on the scan's
// observations, so "service stopped" is as new as the last minute. A unit
// that is up by the scan's own rule reads "active (running)", as the scan
// writes it; anything else keeps systemd's words. liveAt says, per service
// id, when the reading that decided it was taken; a service missing from it
// still carries only the scan's answer.
func overlayLiveServiceStatus(observations []serviceObservation, pkgFamily string, now time.Time) (out []serviceObservation, liveAt map[string]time.Time) {
	out = make([]serviceObservation, len(observations))
	copy(out, observations)
	liveAt = map[string]time.Time{}
	for i := range out {
		if !out[i].IsInstalled {
			continue
		}
		unit := out[i].Unit
		if unit == "" {
			unit = out[i].ID
		}
		healthUnit := serviceHealthUnit(out[i].ID, pkgFamily, unit)
		reading, at, ok := serviceHealth.liveReading(healthUnit, now)
		if !ok {
			continue
		}
		switch {
		case serviceHealthReady(out[i].ID, pkgFamily, healthUnit, reading):
			out[i].Status = "active (running)"
		case reading.SubState == "":
			out[i].Status = reading.ActiveState
		default:
			out[i].Status = reading.ActiveState + " (" + reading.SubState + ")"
		}
		liveAt[out[i].ID] = at
	}
	return out, liveAt
}

// serviceRestartProblems: a service systemd restarted three times or more
// within the last hour is failing and coming back.
func serviceRestartProblems(services []ManagedServiceResponse, now time.Time) []monitoringProblem {
	serviceHealth.mu.Lock()
	defer serviceHealth.mu.Unlock()
	if serviceHealth.readAt.IsZero() || now.Sub(serviceHealth.readAt) > serviceHealthFresh {
		return nil
	}
	var problems []monitoringProblem
	for _, service := range services {
		count := restartsWithin(serviceHealth.restarts[service.Unit], now)
		if count < serviceRestartProblemMin {
			continue
		}
		problems = append(problems, monitoringProblem{
			Key: "restarts:" + service.ID, Kind: "service_restarting", Severity: "warning", Actor: "administrator",
			Subject: service.ID, Name: service.Name, Unit: service.Unit, Value: float64(count), ObservedAt: metricsTime(serviceHealth.readAt),
			Action: monitoringProblemAction{Kind: "open", To: "/components/" + url.PathEscape(service.ID)},
		})
	}
	return problems
}

// --- GET /api/v1/metrics/services ------------------------------------------

type serviceHealthView struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Unit        string   `json:"unit"`
	ActiveState string   `json:"active_state,omitempty"`
	SubState    string   `json:"sub_state,omitempty"`
	ActiveSince string   `json:"active_since,omitempty"`
	Restarts    *int64   `json:"restarts,omitempty"`
	RestartsHr  int64    `json:"restarts_last_hour"`
	MemoryBytes *uint64  `json:"memory_bytes,omitempty"`
	CPU         *float64 `json:"cpu,omitempty"`
	// Running is the scan's own rule applied to the reading (alpha.130): a
	// oneshot unit "active (exited)" is up, PostgreSQL's apt wrapper is not.
	Running *bool `json:"running,omitempty"`
}

type serviceHealthReport struct {
	// State: known, checking (no reading yet), unreadable (the last reading
	// failed and none is fresh) or not_scanned (no component scan yet).
	State    string              `json:"state"`
	ReadAt   string              `json:"read_at,omitempty"`
	Services []serviceHealthView `json:"services"`
}

func (p *Panel) serviceHealthReport(ctx context.Context, now time.Time) serviceHealthReport {
	report := serviceHealthReport{Services: []serviceHealthView{}}
	services, ok := p.monitoredServices(ctx)
	if !ok {
		report.State = "not_scanned"
		return report
	}
	pkgFamily := p.managedServiceHostProfile().PackageFamily
	s := &serviceHealth
	s.mu.Lock()
	defer s.mu.Unlock()
	fresh := !s.readAt.IsZero() && now.Sub(s.readAt) <= serviceHealthFresh
	switch {
	case fresh:
		report.State, report.ReadAt = "known", metricsTime(s.readAt)
	case !s.failedAt.IsZero():
		report.State = "unreadable"
	default:
		report.State = "checking"
	}
	for _, service := range services {
		view := serviceHealthView{ID: service.ID, Name: service.Name, Unit: service.Unit}
		if fresh {
			if reading, ok := s.units[service.Unit]; ok {
				view.ActiveState, view.SubState, view.ActiveSince = reading.ActiveState, reading.SubState, reading.ActiveSince
				view.Restarts, view.MemoryBytes = reading.Restarts, reading.MemoryBytes
				view.RestartsHr = restartsWithin(s.restarts[service.Unit], now)
				if cpu, ok := s.cpu[service.Unit]; ok {
					value := cpu
					view.CPU = &value
				}
				if reading.ActiveState != "" {
					running := serviceHealthReady(service.ID, pkgFamily, service.Unit, reading)
					view.Running = &running
				}
			}
		}
		report.Services = append(report.Services, view)
	}
	// Not running first, then the busiest.
	sort.SliceStable(report.Services, func(i, j int) bool {
		a, b := report.Services[i], report.Services[j]
		aDown, bDown := a.Running != nil && !*a.Running, b.Running != nil && !*b.Running
		if aDown != bDown {
			return aDown
		}
		return false
	})
	return report
}

func (p *Panel) handleMonitoringServices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = json.NewEncoder(w).Encode(p.serviceHealthReport(r.Context(), time.Now()))
}
