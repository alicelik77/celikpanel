package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Website traffic collection, Agent side (alpha.118, D-044).
//
// nginx writes one small log per website (internal/services/traffic_log.go):
// "<msec> <bytes sent> <bytes received>" per request. The Agent reads each log
// from where it stopped (its inode and offset), follows a rotation to the
// renamed file (logrotate's "<id>.log.1", kept uncompressed for one cycle by
// delaycompress), and adds the bytes to per-day totals. Everything it knows
// lives in one root-only file, written atomically, so offsets and totals never
// disagree after a crash.
//
// The Panel collects every few minutes. Totals are sealed into numbered
// batches; a batch is answered until the Panel confirms it (the next request's
// AfterSeq), so a lost answer is sent again and a committed one is never
// counted twice. The store's random epoch tells the Panel when the store is
// new (a reinstalled Agent), so its sequence starts again.
//
// A log present when a new store is made is read from its end (its earlier
// bytes may already have been counted by a lost store; nothing is counted
// twice); a log that appears later is read from its start.
//
// Web trafiği toplama: Agent her sitenin küçük günlüğünü kaldığı yerden okur,
// döndürmeyi izler ve günlük toplamlara ekler. Toplamlar numaralı paketlere
// mühürlenir; Panel onaylayana dek yeniden gönderilir, onaylanan bir daha
// sayılmaz.

const (
	trafficStoreVersion = 1
	// trafficTickInterval: how often the Agent reads the logs on its own.
	trafficTickInterval = time.Minute
	// trafficPrevKeep: how long a rotated log is read again for lines nginx
	// still wrote to it before it reopened its logs.
	trafficPrevKeep = 10 * time.Minute
	// trafficReadLimit bounds one read of one log per tick.
	trafficReadLimit = 64 << 20
	// trafficMaxBatches: past this many unconfirmed batches new totals stay
	// pending (merged) instead of making more batches.
	trafficMaxBatches = 50
	// trafficLogrotateInterval: how often the logrotate file is checked.
	trafficLogrotateInterval = time.Hour
)

var (
	trafficStoreDir       = "/var/lib/celikpanel-agent-private/traffic"
	trafficLogrotatePath  = "/etc/logrotate.d/celikpanel-traffic"
	trafficLogrotateTools = []string{"/usr/sbin/logrotate", "/usr/bin/logrotate", "/sbin/logrotate"}
	trafficNow            = time.Now
	// trafficLogDir is where nginx writes the traffic logs (a test seam).
	trafficLogDir = services.TrafficLogDir
	trafficMu     sync.Mutex
	// trafficLastLogrotate is when the logrotate file was last reconciled.
	trafficLastLogrotate      time.Time
	trafficLastLogrotateState string
)

type trafficPrevFile struct {
	ID     trafficFileID `json:"id"`
	Offset int64         `json:"offset"`
	Since  int64         `json:"since"`
}

type trafficFileState struct {
	ID     trafficFileID    `json:"id"`
	Offset int64            `json:"offset"`
	Prev   *trafficPrevFile `json:"prev,omitempty"`
}

type trafficTotals struct {
	Out      int64 `json:"out"`
	In       int64 `json:"in"`
	Requests int64 `json:"requests"`
}

type trafficStore struct {
	Version int                          `json:"version"`
	Epoch   string                       `json:"epoch"`
	NextSeq int64                        `json:"next_seq"`
	Files   map[string]*trafficFileState `json:"files"`
	// Pending: domain id "|" day -> totals not yet in a batch.
	Pending map[string]*trafficTotals `json:"pending"`
	Batches []transport.TrafficBatch  `json:"batches"`
	// Gaps: domain id -> reason, since the last collection.
	Gaps map[string]string `json:"gaps,omitempty"`
	// fresh: made in this process (files present now start at their end).
	fresh bool
}

func trafficStorePath() string { return filepath.Join(trafficStoreDir, "state.json") }

func newTrafficEpoch() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(raw[:])
}

// loadTrafficStore reads the store; a missing store is a new, empty one. A
// store that cannot be read is an error: it is never silently replaced (its
// unconfirmed batches would be lost).
func loadTrafficStore() (*trafficStore, error) {
	content, err := os.ReadFile(trafficStorePath())
	if errors.Is(err, os.ErrNotExist) {
		return &trafficStore{
			Version: trafficStoreVersion, Epoch: newTrafficEpoch(), NextSeq: 1,
			Files: map[string]*trafficFileState{}, Pending: map[string]*trafficTotals{}, fresh: true,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the traffic store: %w", err)
	}
	var store trafficStore
	if err := json.Unmarshal(content, &store); err != nil {
		return nil, fmt.Errorf("the traffic store %s is not readable (%v); it was left as it is", trafficStorePath(), err)
	}
	if store.Version != trafficStoreVersion || store.Epoch == "" || store.NextSeq < 1 {
		return nil, fmt.Errorf("the traffic store %s has an unknown version; it was left as it is", trafficStorePath())
	}
	if store.Files == nil {
		store.Files = map[string]*trafficFileState{}
	}
	if store.Pending == nil {
		store.Pending = map[string]*trafficTotals{}
	}
	return &store, nil
}

// save writes the store atomically (temporary file, fsync, rename, fsync of
// the directory), root-only.
func (s *trafficStore) save() error {
	if err := os.MkdirAll(trafficStoreDir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(trafficStoreDir, 0o700)
	content, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(trafficStoreDir, ".state-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, trafficStorePath()); err != nil {
		return err
	}
	if dir, err := os.Open(trafficStoreDir); err == nil {
		_ = dir.Sync()
		dir.Close()
	}
	s.fresh = false
	return nil
}

func (s *trafficStore) add(domainID, day string, totals trafficTotals) {
	key := domainID + "|" + day
	current := s.Pending[key]
	if current == nil {
		current = &trafficTotals{}
		s.Pending[key] = current
	}
	current.Out += totals.Out
	current.In += totals.In
	current.Requests += totals.Requests
}

func (s *trafficStore) gap(domainID, reason string) {
	if s.Gaps == nil {
		s.Gaps = map[string]string{}
	}
	s.Gaps[domainID] = reason
}

// parseTrafficLine reads "<msec> <bytes_sent> <request_length>". A line that
// is not in that form is skipped (ok=false), never guessed.
func parseTrafficLine(line string) (day string, out, in int64, ok bool) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return "", 0, 0, false
	}
	whole, _, _ := strings.Cut(fields[0], ".")
	seconds, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || seconds <= 0 {
		return "", 0, 0, false
	}
	out, err = strconv.ParseInt(fields[1], 10, 64)
	if err != nil || out < 0 {
		return "", 0, 0, false
	}
	in, err = strconv.ParseInt(fields[2], 10, 64)
	if err != nil || in < 0 {
		in = 0
	}
	return time.Unix(seconds, 0).In(time.Local).Format("2006-01-02"), out, in, true
}

// readTrafficFrom reads complete lines from offset (at most limit bytes) and
// returns the totals per day and the new offset (after the last complete
// line).
func readTrafficFrom(file *os.File, offset int64, limit int64) (map[string]trafficTotals, int64, error) {
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	reader := bufio.NewReaderSize(io.LimitReader(file, limit), 64<<10)
	days := map[string]trafficTotals{}
	consumed := offset
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// A line without its newline is not finished; it is read next time.
			if errors.Is(err, io.EOF) {
				return days, consumed, nil
			}
			return days, consumed, err
		}
		consumed += int64(len(line))
		day, out, in, ok := parseTrafficLine(strings.TrimRight(line, "\r\n"))
		if !ok {
			continue
		}
		t := days[day]
		t.Out += out
		t.In += in
		t.Requests++
		days[day] = t
	}
}

func (s *trafficStore) readInto(domainID string, path string, offset int64) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer file.Close()
	days, next, err := readTrafficFrom(file, offset, trafficReadLimit)
	for day, totals := range days {
		s.add(domainID, day, totals)
	}
	return next, err
}

// trafficLogDomainIDs lists the domain ids that have a traffic log.
func trafficLogDomainIDs() ([]string, error) {
	entries, err := os.ReadDir(trafficLogDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") || !entry.Type().IsRegular() {
			continue
		}
		id := strings.TrimSuffix(name, ".log")
		if n, err := strconv.Atoi(id); err != nil || n <= 0 || strconv.Itoa(n) != id {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// tail reads every log once. It changes only the store in memory; the caller
// saves it.
func (s *trafficStore) tail() {
	ids, err := trafficLogDomainIDs()
	if err != nil {
		log.Printf("[traffic] list the traffic logs: %v", err)
		return
	}
	now := trafficNow()
	for _, id := range ids {
		path := filepath.Join(trafficLogDir(), id+".log")
		s.tailOne(id, path, now)
	}
}

func (s *trafficStore) tailOne(id, path string, now time.Time) {
	fileID, size, err := statTrafficFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.gap(id, transport.TrafficReasonLogUnreadable)
		}
		return
	}
	state := s.Files[id]
	if state == nil {
		start := int64(0)
		if s.fresh {
			// Bytes before this store may have been counted by a lost one.
			start = size
		}
		state = &trafficFileState{ID: fileID, Offset: start}
		s.Files[id] = state
	}
	rotated := path + ".1"
	// A file rotated away earlier: read the lines nginx wrote to it before
	// it reopened its logs.
	if state.Prev != nil {
		if prevID, _, err := statTrafficFile(rotated); err == nil && prevID == state.Prev.ID {
			if next, err := s.readInto(id, rotated, state.Prev.Offset); err == nil {
				state.Prev.Offset = next
			}
		}
		if now.Unix()-state.Prev.Since > int64(trafficPrevKeep/time.Second) {
			state.Prev = nil
		}
	}
	if fileID != state.ID {
		// Rotated: finish the old file from its new name, then start the new.
		finished := false
		if oldID, _, err := statTrafficFile(rotated); err == nil && oldID == state.ID {
			if next, err := s.readInto(id, rotated, state.Offset); err == nil {
				state.Prev = &trafficPrevFile{ID: state.ID, Offset: next, Since: now.Unix()}
				finished = true
			}
		}
		if !finished {
			s.gap(id, transport.TrafficReasonRotationGap)
		}
		state.ID, state.Offset = fileID, 0
	} else if size < state.Offset {
		// Truncated in place (copytruncate): start again from the top.
		s.gap(id, transport.TrafficReasonRotationGap)
		state.Offset = 0
	}
	next, err := s.readInto(id, path, state.Offset)
	if err != nil {
		s.gap(id, transport.TrafficReasonLogUnreadable)
	}
	state.Offset = next
}

// seal moves the pending totals into a new numbered batch.
func (s *trafficStore) seal() {
	if len(s.Pending) == 0 || len(s.Batches) >= trafficMaxBatches {
		return
	}
	keys := make([]string, 0, len(s.Pending))
	for key := range s.Pending {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	batch := transport.TrafficBatch{Seq: s.NextSeq}
	for _, key := range keys {
		domain, day, _ := strings.Cut(key, "|")
		domainID, err := strconv.Atoi(domain)
		if err != nil {
			continue
		}
		totals := s.Pending[key]
		batch.Items = append(batch.Items, transport.TrafficDayBytes{
			DomainID: domainID, Day: day, BytesOut: totals.Out, BytesIn: totals.In, Requests: totals.Requests,
		})
	}
	s.NextSeq++
	s.Batches = append(s.Batches, batch)
	s.Pending = map[string]*trafficTotals{}
}

// confirm drops the batches the Panel committed.
func (s *trafficStore) confirm(epoch string, afterSeq int64) {
	if epoch != s.Epoch || afterSeq <= 0 {
		return
	}
	kept := s.Batches[:0]
	for _, batch := range s.Batches {
		if batch.Seq > afterSeq {
			kept = append(kept, batch)
		}
	}
	s.Batches = kept
}

// trafficSourceFor is the measurement state of one website.
func trafficSourceFor(domain transport.TrafficDomain, formatState string, gaps map[string]string) transport.TrafficSourceState {
	state := transport.TrafficSourceState{DomainID: domain.DomainID}
	if domain.Blocked {
		state.State = transport.TrafficSourceNotCounted
		return state
	}
	if formatState != transport.SiteFileChallengeUnchanged {
		state.State, state.Reason = transport.TrafficSourceUnknown, transport.TrafficReasonFormatKept
		if formatState == transport.SiteFileChallengeAbsent {
			state.Reason = transport.TrafficReasonIncludeMissing
		}
		return state
	}
	if !services.SiteReadsTrafficInclude(domain.Domain) {
		state.State, state.Reason = transport.TrafficSourceUnknown, transport.TrafficReasonVhostKept
		return state
	}
	if services.TrafficIncludeState(domain.Domain, domain.DomainID) != "counting" {
		state.State, state.Reason = transport.TrafficSourceUnknown, transport.TrafficReasonIncludeMissing
		return state
	}
	state.State = transport.TrafficSourceMeasuring
	if reason := gaps[strconv.Itoa(domain.DomainID)]; reason != "" {
		state.Reason = reason
	}
	return state
}

// renderTrafficLogrotate is CelikPanel's logrotate file for the traffic logs:
// daily, three days kept, never more than 100 MB before rotating. nginx is
// told to reopen its logs with its own signal through systemd, which works
// whatever nginx's pid file is called.
func renderTrafficLogrotate() string {
	return "# The small per-site traffic logs nginx writes for CelikPanel's monthly\n" +
		"# traffic limit. Rotated by the operating system, with or without CelikPanel.\n" +
		"# CelikPanel writes this file only while it is CelikPanel's unchanged text.\n" +
		"# CelikPanel'in trafik günlükleri; işletim sistemi döndürür.\n" +
		services.TrafficLogDir() + "/*.log {\n" +
		"    daily\n" +
		"    maxsize 100M\n" +
		"    rotate 3\n" +
		"    missingok\n" +
		"    notifempty\n" +
		"    compress\n" +
		"    delaycompress\n" +
		"    create\n" +
		"    sharedscripts\n" +
		"    postrotate\n" +
		"        systemctl kill --kill-who=main --signal=USR1 nginx.service >/dev/null 2>&1 || true\n" +
		"    endscript\n" +
		"}\n"
}

// trafficLogrotatePresent: the logrotate program is on this server.
func trafficLogrotatePresent() bool {
	for _, candidate := range trafficLogrotateTools {
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}
	return false
}

// withLogrotateCompanion adds the official logrotate package to a web
// server install on a server without it (the same name on apt and pacman).
// It never changes the catalogue's slice.
func withLogrotateCompanion(serviceID string, pkgs []string, present bool) []string {
	if serviceID != "nginx" || present {
		return pkgs
	}
	for _, pkg := range pkgs {
		if pkg == "logrotate" {
			return pkgs
		}
	}
	return append(append([]string(nil), pkgs...), "logrotate")
}

// reconcileTrafficLogrotate keeps the logrotate file (at most hourly).
func reconcileTrafficLogrotate(now time.Time) string {
	if !trafficLastLogrotate.IsZero() && now.Sub(trafficLastLogrotate) < trafficLogrotateInterval {
		return trafficLastLogrotateState
	}
	trafficLastLogrotate = now
	tool := trafficLogrotatePresent()
	if info, err := os.Stat(filepath.Dir(trafficLogrotatePath)); err != nil || !info.IsDir() {
		trafficLastLogrotateState = "missing_tool"
		return trafficLastLogrotateState
	}
	state, err := services.ReconcileManagedFile(trafficLogrotatePath, renderTrafficLogrotate())
	if err != nil {
		log.Printf("[traffic] logrotate file: %v", err)
		trafficLastLogrotateState = "failed"
		return trafficLastLogrotateState
	}
	if !tool {
		state = "missing_tool"
	}
	trafficLastLogrotateState = state
	return state
}

// trafficTick reads every log once and saves the store.
func trafficTick() error {
	trafficMu.Lock()
	defer trafficMu.Unlock()
	store, err := loadTrafficStore()
	if err != nil {
		return err
	}
	store.tail()
	reconcileTrafficLogrotate(trafficNow())
	return store.save()
}

// runTrafficWorker reads the logs every minute.
func runTrafficWorker() {
	for {
		if err := trafficTick(); err != nil {
			log.Printf("[traffic] %v", err)
		}
		time.Sleep(trafficTickInterval)
	}
}

// TrafficCollectV1 answers the batches the Panel has not confirmed and every
// requested website's measurement state. It reads the logs first, so the
// answer is current. Polling never changes anything but the Agent's own
// store: confirmed batches are dropped, pending totals are sealed.
//
// TrafficCollectV1, Panel'in onaylamadığı paketleri ve her sitenin ölçüm
// durumunu verir. Yalnız Agent'ın kendi deposu değişir.
func (a *Agent) TrafficCollectV1(req *transport.TrafficCollectRequest, resp *transport.TrafficCollectResponse) error {
	if resp == nil {
		return nil
	}
	*resp = transport.TrafficCollectResponse{Batches: []transport.TrafficBatch{}, Sources: []transport.TrafficSourceState{}}
	if req == nil {
		resp.Error = "traffic collection request is required"
		return nil
	}
	trafficMu.Lock()
	defer trafficMu.Unlock()
	store, err := loadTrafficStore()
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	store.confirm(req.Epoch, req.AfterSeq)
	store.tail()
	store.seal()
	gaps := store.Gaps
	store.Gaps = nil
	if err := store.save(); err != nil {
		resp.Error = "the traffic store could not be saved: " + err.Error()
		return nil
	}
	resp.Epoch = store.Epoch
	for _, batch := range store.Batches {
		if req.Epoch != store.Epoch || batch.Seq > req.AfterSeq {
			resp.Batches = append(resp.Batches, batch)
		}
	}
	formatState := services.TrafficHTTPConfState()
	for _, domain := range req.Domains {
		if domain.DomainID <= 0 || domain.Domain == "" || strings.ContainsAny(domain.Domain, "/\\") {
			continue
		}
		resp.Sources = append(resp.Sources, trafficSourceFor(domain, formatState, gaps))
	}
	resp.Logrotate = reconcileTrafficLogrotate(trafficNow())
	return nil
}
