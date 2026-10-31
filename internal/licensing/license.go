// Package licensing verifies server entitlements independently of release trust.
package licensing

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const API = "https://celikpanel.net/account/"

// Offline verification policy (owner-approved 2026-10-29, D-039).
//
// OfflineGrace caps how long one successful check of the license service keeps
// management open while the service cannot be reached: the signed offline_until,
// never later than issued_at+OfflineGrace and never later than the annual
// expiry. A central outage therefore does not close every panel at once.
// RefreshInterval is how often a panel in use asks again (the earlier of it and
// the signed refresh_after, minus up to a fifth as per-server jitter), so an
// explicit rejection (removed, moved or replaced license) applies at the next
// check. There is still no timer on an idle server: only authenticated use, or
// the administrator's "Check again", contacts the service.
//
// Çevrimdışı doğrulama ilkesi (sahip onayı 2026-10-29, D-039): bir başarılı
// kontrol, lisans hizmetine ulaşılamazken yönetimi en çok 7 gün (ve hiçbir zaman
// yıllık bitişten sonra değil) açık tutar; kullanımdaki panel yaklaşık saatte bir
// yeniden sorar. Açık ret bir sonraki kontrolde hemen uygulanır. Boştaki sunucuda
// zamanlayıcı yoktur.
const OfflineGrace = 7 * 24 * time.Hour
const RefreshInterval = time.Hour

// RetryInterval is the retry delay once the receipt no longer opens management.
// While it still does, failures back off from MinRetryInterval, doubling up to
// MaxRetryInterval, each delay shortened by up to a fifth (per-server jitter so
// panels do not return together after a central outage), and never past the
// moment the receipt would lapse.
const RetryInterval = 30 * time.Second
const MinRetryInterval = time.Minute
const MaxRetryInterval = 30 * time.Minute

// ClockSkew is the tolerated difference between this server's clock and the
// license service's (the same five minutes Verify accepts), and the tolerated
// step back of this server's clock before it counts as set back.
const ClockSkew = 5 * time.Minute

// refreshWait is how long a request holding a still-valid receipt waits for a
// due check before it proceeds on that receipt. A reachable service answers well
// within it, so its rejection applies to that very request; an unreachable one
// delays one request by at most this per retry delay.
const refreshWait = 3 * time.Second

// Check: the latest refresh attempt since the last successful one failed this
// way (Status.Check). Empty means no failed attempt is known in this process.
const (
	CheckUnreachable   = "unreachable"
	CheckRateLimited   = "rate_limited"
	CheckInvalidAnswer = "invalid_answer"
	CheckClockBehind   = "clock_behind"
)

// Reasons for verification_unavailable (Status.Reason).
const (
	// The last successful check is older than its offline window.
	ReasonOfflineEnded = "offline_ended"
	// This server's clock is earlier than the receipt or was set back.
	ReasonClockBehind = "clock_behind"
)

// Production public key is provisioned separately from the signed-release key.
const PublicKeyHex = "aa7768257096ed17ae78077d1269b0b38f57aae9c544616ee10708d22b656f5e"

var hex64 = regexp.MustCompile(`^[a-f0-9]{64}$`)
var hex32 = regexp.MustCompile(`^[a-f0-9]{32}$`)
var keyPattern = regexp.MustCompile(`^CPK-[a-f0-9]{64}$`)

type Envelope struct {
	// Rejected is local durable state, never an entitlement from the service.
	Rejected bool `json:"rejected,omitempty"`
	// SeenAt is local durable state too: the latest wall-clock second this
	// server observed while checking this receipt (unsigned, never sent). A
	// clock more than ClockSkew below it makes the receipt unusable until the
	// service is reached again, so a restart with a clock set back cannot
	// replay the offline window. Older panels ignore the field.
	SeenAt          int64  `json:"seen_at,omitempty"`
	Payload         string `json:"payload"`
	Signature       string `json:"signature"`
	ActivationToken string `json:"activation_token"`
}
type Claims struct {
	Format       string `json:"format"`
	Product      string `json:"product"`
	LicenseID    string `json:"license_id"`
	ServerID     string `json:"server_id"`
	IssuedAt     int64  `json:"issued_at"`
	ActivatedAt  int64  `json:"activated_at"`
	ExpiresAt    int64  `json:"expires_at"`
	RefreshAfter int64  `json:"refresh_after"`
	OfflineUntil int64  `json:"offline_until"`
}

// Observation separates entitlement evidence from unavailable observations.
// Observation, lisans kanıtını gözlemin yapılamamasından ayırır.
const (
	ObservationKnown       = "known"
	ObservationUnavailable = "unavailable"
)

var errInvalidState = errors.New("invalid license state")
var errIssuedInFuture = errors.New("license issued after this server's clock; check the server clock")

type Status struct {
	State        string `json:"state"`
	Observation  string `json:"observation"`
	Product      string `json:"product,omitempty"`
	LicenseID    string `json:"license_id,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	OfflineUntil int64  `json:"offline_until,omitempty"`
	CanProvision bool   `json:"can_provision"`
	// VerifiedAt is the receipt's issued_at: the service's clock at the last
	// successful check.
	VerifiedAt int64 `json:"verified_at,omitempty"`
	// Durations on this server's clock at the moment of the status, so a page
	// never compares its own clock with the server's (D-032).
	VerifiedSecondsAgo int64 `json:"verified_seconds_ago,omitempty"`
	OfflineForSeconds  int64 `json:"offline_for_seconds,omitempty"`
	// Check is the latest failed refresh since the last success (Check*).
	Check string `json:"check,omitempty"`
	// Reason explains verification_unavailable (Reason*).
	Reason string `json:"reason,omitempty"`
	// Empty in every ordinary build (acceptance_off.go), so the JSON is unchanged.
	// Only the acceptance_license test build labels its fixture license here.
	acceptanceStatus
}

func ServerID(machineID []byte) (string, error) {
	b := bytes.TrimSpace(machineID)
	if len(b) != 32 {
		return "", errors.New("invalid machine identity")
	}
	raw, err := hex.DecodeString(string(b))
	if err != nil || bytes.Equal(raw, make([]byte, 16)) {
		return "", errors.New("invalid machine identity")
	}
	sum := sha256.Sum256(append([]byte("celikpanel-license-server-v1:"), raw...))
	return hex.EncodeToString(sum[:]), nil
}

func Verify(e Envelope, key ed25519.PublicKey, server string, now time.Time) (Claims, error) {
	var c Claims
	payload, err := base64.StdEncoding.Strict().DecodeString(e.Payload)
	if err != nil || len(payload) > 4096 {
		return c, errors.New("invalid license payload")
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(e.Signature)
	if err != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, payload, sig) {
		return c, errors.New("invalid license signature")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&c); err != nil {
		return Claims{}, errors.New("invalid license claims")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return Claims{}, errors.New("trailing license data")
	}
	if c.Format != "celikpanel-license-v1" || c.Product != "celikpanel" || !hex32.MatchString(c.LicenseID) ||
		!hex64.MatchString(server) || c.ServerID != server || !hex64.MatchString(e.ActivationToken) ||
		c.ActivatedAt <= 0 || c.IssuedAt < c.ActivatedAt || c.ExpiresAt <= c.ActivatedAt ||
		c.RefreshAfter <= c.IssuedAt ||
		c.RefreshAfter > c.IssuedAt+86400 || c.OfflineUntil > c.IssuedAt+7*86400 ||
		c.OfflineUntil > c.ExpiresAt || c.OfflineUntil <= 0 {
		return Claims{}, errors.New("invalid license binding or dates")
	}
	// Signed and well formed, but dated after this server's clock: the clock
	// is behind (the service's issued_at is its own clock).
	if c.IssuedAt > now.Add(ClockSkew).Unix() {
		return Claims{}, errIssuedInFuture
	}
	return c, nil
}

type checkInfo struct {
	kind string
	at   int64
}

type Manager struct {
	mu       sync.Mutex
	file     string
	key      ed25519.PublicKey
	server   string
	endpoint string
	client   *http.Client
	clock    func() time.Time
	// mono is this process's monotonic elapsed time. With clock it detects a
	// wall clock set back (or held still) while the panel runs.
	mono func() time.Duration
	// wait is refreshWait; tests shorten it.
	wait time.Duration
	// retryAt is the wall-clock second before which an ordinary request does
	// not contact the service again; failures counts consecutive failures.
	retryAt  atomic.Int64
	failures int
	rejected atomic.Bool
	// bootWall is the highest observed wall clock minus monotonic elapsed time
	// (see observeClock); 0 until the first observation.
	bootWall atomic.Int64
	check    atomic.Pointer[checkInfo]
	// flight is the single shared refresh in progress started by AccessStatus.
	flightMu sync.Mutex
	flight   chan struct{}
	// seam is assigned only by NewServer in the acceptance_license test build
	// (acceptance_fixture.go). Ordinary builds have no implementation and no
	// assignment, so it is always nil there.
	seam acceptanceSeam
}

// acceptanceSeam replaces license verification only in the acceptance_license
// test build. It is an interface with no implementation in ordinary builds.
type acceptanceSeam interface {
	status() Status
	activate(ctx context.Context, key, hostname string) error
	refresh(ctx context.Context, force bool) error
}

func New(file string, key ed25519.PublicKey, server string) (*Manager, error) {
	if !filepath.IsAbs(file) || len(key) != 32 || !hex64.MatchString(server) {
		return nil, errors.New("invalid licensing configuration")
	}
	start := time.Now()
	return &Manager{file: file, key: append(ed25519.PublicKey(nil), key...), server: server, endpoint: API,
		client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, clock: time.Now,
		mono: func() time.Duration { return time.Since(start) }, wait: refreshWait}, nil
}
func (m *Manager) read() (Envelope, Claims, error) {
	var e Envelope
	f, err := openState(m.file)
	if err != nil {
		return e, Claims{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return e, Claims{}, err
	}
	if !st.Mode().IsRegular() || st.Size() > 8192 {
		return e, Claims{}, errInvalidState
	}
	b, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil {
		return e, Claims{}, err
	}
	if len(b) > 8192 {
		return e, Claims{}, errInvalidState
	}
	if err = json.Unmarshal(b, &e); err != nil {
		return e, Claims{}, fmt.Errorf("%w: malformed envelope", errInvalidState)
	}
	// A stored receipt is judged against the clock in Status, not here: a
	// clock that runs behind must read as "check the clock", not as invalid.
	c, err := Verify(e, m.key, m.server, time.Unix(1<<40, 0))
	if err != nil {
		return e, c, fmt.Errorf("%w: %v", errInvalidState, err)
	}
	return e, c, nil
}

// observeClock records this wall-clock reading and reports whether the clock
// was set back while this process runs. Wall time minus monotonic elapsed time
// stays constant on an untouched clock (it grows after a suspend or a forward
// correction); a drop of more than ClockSkew below its highest value means the
// wall clock went back, or stood still for that long. Only the refresh that
// succeeds resets it (resetClock).
func (m *Manager) observeClock(wall int64) (setBack bool) {
	boot := wall - int64(m.mono()/time.Second)
	for {
		high := m.bootWall.Load()
		if high != 0 && boot < high-int64(ClockSkew/time.Second) {
			return true
		}
		if high != 0 && boot <= high || m.bootWall.CompareAndSwap(high, boot) {
			return false
		}
	}
}
func (m *Manager) resetClock(wall int64) {
	m.bootWall.Store(wall - int64(m.mono()/time.Second))
}

func (m *Manager) Status() Status {
	if m.seam != nil {
		return m.seam.status()
	}
	s, _, _, _ := m.evaluate()
	return s
}

// evaluate is Status with the receipt it judged.
func (m *Manager) evaluate() (Status, Envelope, Claims, error) {
	e, c, err := m.read()
	if err != nil {
		if m.rejected.Load() || errors.Is(err, errInvalidState) {
			return Status{State: "invalid", Observation: ObservationKnown}, e, c, err
		}
		if os.IsNotExist(err) {
			return Status{State: "missing", Observation: ObservationKnown}, e, c, err
		}
		return Status{State: "status_unavailable", Observation: ObservationUnavailable}, e, c, err
	}
	now := m.clock().Unix()
	setBack := m.observeClock(now)
	skew := int64(ClockSkew / time.Second)
	s := Status{State: "active", Observation: ObservationKnown, Product: c.Product, LicenseID: c.LicenseID, ExpiresAt: c.ExpiresAt,
		OfflineUntil: min(c.OfflineUntil, c.IssuedAt+int64(OfflineGrace/time.Second), c.ExpiresAt), CanProvision: true, VerifiedAt: c.IssuedAt}
	if info := m.check.Load(); info != nil {
		s.Check = info.kind
	}
	if now >= c.ExpiresAt {
		s.State = "expired"
		s.CanProvision = false
	} else if e.Rejected || m.rejected.Load() {
		s.State = "invalid"
		s.CanProvision = false
	} else if now < c.IssuedAt-skew || now < e.SeenAt-skew || setBack {
		s.State, s.Reason = "verification_unavailable", ReasonClockBehind
		s.Observation = ObservationUnavailable
		s.CanProvision = false
	} else if now >= s.OfflineUntil {
		s.State, s.Reason = "verification_unavailable", ReasonOfflineEnded
		s.Observation = ObservationUnavailable
		s.CanProvision = false
	}
	if s.CanProvision {
		s.OfflineForSeconds = s.OfflineUntil - now
	}
	if now > c.IssuedAt {
		s.VerifiedSecondsAgo = now - c.IssuedAt
	}
	return s, e, c, nil
}

// spread is a deterministic per-server fraction in [0, 1) of the given label,
// so panels spread their checks without a random source.
func (m *Manager) spread(label string, issued int64) float64 {
	sum := sha256.Sum256([]byte(m.server + ":" + label + ":" + strconv.FormatInt(issued, 10)))
	return float64(binary.BigEndian.Uint64(sum[:8])>>11) / float64(uint64(1)<<53)
}

// refreshAt is when an ordinary request asks the service again: the earlier of
// the signed refresh_after and RefreshInterval after issue, minus up to a fifth.
func (m *Manager) refreshAt(c Claims) int64 {
	span := min(c.RefreshAfter, c.IssuedAt+int64(RefreshInterval/time.Second)) - c.IssuedAt
	return c.IssuedAt + span - int64(float64(span)/5*m.spread("refresh", c.IssuedAt))
}

// due: the check time has come, or the clock was found set back (then the
// service's answer is the only way to restore management before that time).
func (m *Manager) due(s Status, c Claims, now int64) bool {
	return now >= m.refreshAt(c) || s.Reason == ReasonClockBehind
}

func (m *Manager) save(e Envelope) error {
	if err := os.MkdirAll(filepath.Dir(m.file), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.file), ".license-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, m.file); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(m.file))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// noteCheck records how a refresh of this installation ended. An answer of the
// service about the license (success or an explicit refusal) clears it.
func (m *Manager) noteCheck(action, kind string) {
	if action != "refresh" {
		return
	}
	if kind == "" {
		m.check.Store(nil)
		return
	}
	m.check.Store(&checkInfo{kind: kind, at: m.clock().Unix()})
}

func (m *Manager) request(ctx context.Context, action string, input map[string]string) error {
	if m.seam != nil {
		return errors.New("license service is never contacted by this build")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint+"?action="+action, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if testing.Testing() && liveNetworkTo(m.client, req) {
		// A test once reached the live service with fake values (2026-10-30).
		// Tests must use their own endpoint or transport; never the real one.
		return errors.New("tests never contact the live license service")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		m.noteCheck(action, CheckUnreachable)
		return errors.New("license service unavailable; existing license retained")
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8193))
	if err != nil || len(b) > 8192 {
		m.noteCheck(action, CheckUnreachable)
		return errors.New("invalid license service response")
	}
	if resp.StatusCode != http.StatusOK {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &problem)
		switch problem.Error {
		case "invalid_license", "license_in_use", "license_expired", "invalid_activation":
			// Only an explicit rejection of this installation's refresh invalidates
			// its receipt. A mistyped replacement key must not revoke a valid license.
			if action == "refresh" && resp.StatusCode == http.StatusBadRequest {
				m.noteCheck(action, "")
				m.rejected.Store(true)
				if previous, _, readErr := m.read(); readErr == nil {
					previous.Rejected = true
					if saveErr := m.save(previous); saveErr != nil {
						return errors.New("license rejected; could not persist rejection")
					}
				}
			} else {
				m.noteCheck(action, CheckUnreachable)
			}
			return fmt.Errorf("license: %s", problem.Error)
		case "rate_limited":
			m.noteCheck(action, CheckRateLimited)
			return fmt.Errorf("license: %s", problem.Error)
		}
		m.noteCheck(action, CheckUnreachable)
		return errors.New("license service unavailable; existing license retained")
	}
	var e Envelope
	if err = json.Unmarshal(b, &e); err != nil {
		m.noteCheck(action, CheckInvalidAnswer)
		return errors.New("invalid license service response")
	}
	if _, err = Verify(e, m.key, m.server, m.clock()); err != nil {
		if errors.Is(err, errIssuedInFuture) {
			m.noteCheck(action, CheckClockBehind)
		} else {
			m.noteCheck(action, CheckInvalidAnswer)
		}
		return err
	}
	if e.Rejected {
		m.noteCheck(action, CheckInvalidAnswer)
		return errors.New("invalid license service response")
	}
	// The service has just vouched for this moment: the clock high-water marks
	// restart from it.
	now := m.clock().Unix()
	e.SeenAt = now
	if err = m.save(e); err != nil {
		return err
	}
	m.resetClock(now)
	m.noteCheck(action, "")
	m.rejected.Store(false)
	return nil
}
func (m *Manager) Activate(ctx context.Context, key, hostname string) error {
	if m.seam != nil {
		return m.seam.activate(ctx, key, hostname)
	}
	if !keyPattern.MatchString(key) {
		return errors.New("invalid license key")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.request(ctx, "activate", map[string]string{"key": key, "server_id": m.server, "hostname": hostname})
	if err == nil {
		m.failures = 0
		m.retryAt.Store(0)
		m.check.Store(nil)
	}
	return err
}
func (m *Manager) Refresh(ctx context.Context, force bool) error {
	if m.seam != nil {
		return m.seam.refresh(ctx, force)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refreshLocked(ctx, force)
}

// retryDelay is the wait after the n-th consecutive failure while the receipt
// still opens management, shortened by up to a fifth per server.
func (m *Manager) retryDelay(n int, issued int64) int64 {
	d := MinRetryInterval
	for i := 1; i < n && d < MaxRetryInterval; i++ {
		d *= 2
	}
	d = min(d, MaxRetryInterval)
	seconds := int64(d / time.Second)
	return seconds - int64(float64(seconds)/5*m.spread("retry:"+strconv.Itoa(n), issued))
}

func (m *Manager) refreshLocked(ctx context.Context, force bool) error {
	s, e, c, err := m.evaluate()
	if err != nil {
		return err
	}
	now := m.clock().Unix()
	if !force && !m.due(s, c, now) {
		return nil
	}
	if !force && now < m.retryAt.Load() {
		return errors.New("license refresh temporarily deferred after failure")
	}
	err = m.request(ctx, "refresh", map[string]string{"license_id": c.LicenseID, "server_id": m.server, "activation_token": e.ActivationToken})
	if err == nil {
		m.failures = 0
		m.retryAt.Store(0)
		return nil
	}
	m.failures++
	if s.CanProvision {
		// Still valid: back off, but allow the next attempt at the latest when
		// the receipt lapses, and keep the clock high-water mark current so a
		// restart with a clock set back cannot reuse the window.
		m.retryAt.Store(min(now+m.retryDelay(m.failures, c.IssuedAt), s.OfflineUntil))
		if now > e.SeenAt && !m.rejected.Load() {
			if current, _, readErr := m.read(); readErr == nil && current.Payload == e.Payload && !current.Rejected {
				current.SeenAt = now
				_ = m.save(current)
			}
		}
	} else {
		m.retryAt.Store(now + int64(RetryInterval/time.Second))
	}
	return err
}

// CanProvision also covers authenticated automation without a browser session.
func (m *Manager) CanProvision(ctx context.Context) bool {
	return m.AccessStatus(ctx).CanProvision
}

// AccessStatus is the status for one management request. When a check is due
// it starts (or joins) the single shared refresh. A receipt that no longer
// opens management waits for that refresh, so no request is admitted on a
// stale receipt; a still-valid receipt waits at most refreshWait, so a reachable
// service's rejection applies to this request while an unreachable one does
// not hold requests for the client timeout. Concurrent requests share one
// refresh and its result.
// AccessStatus, tek yönetim isteğinin durumudur. Kontrol zamanı geldiyse ortak
// tek yenilemeyi başlatır veya ona katılır. Geçersizleşmiş makbuz yenilemeyi
// bekler; hâlâ geçerli makbuz en çok refreshWait bekler.
func (m *Manager) AccessStatus(ctx context.Context) Status {
	if m.seam != nil {
		_ = m.Refresh(ctx, false)
		return m.Status()
	}
	s, _, c, err := m.evaluate()
	if err != nil {
		return s
	}
	now := m.clock().Unix()
	if !m.due(s, c, now) || now < m.retryAt.Load() {
		return s
	}
	done := m.startRefresh()
	if s.CanProvision {
		timer := time.NewTimer(m.wait)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			return s
		case <-ctx.Done():
			return s
		}
	} else {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return m.Status()
}

// startRefresh returns the shared refresh in progress, starting it if needed.
// It runs on its own bounded context: the request that started it may end first.
func (m *Manager) startRefresh() <-chan struct{} {
	m.flightMu.Lock()
	defer m.flightMu.Unlock()
	if m.flight != nil {
		return m.flight
	}
	done := make(chan struct{})
	m.flight = done
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = m.Refresh(ctx, false)
		m.flightMu.Lock()
		m.flight = nil
		m.flightMu.Unlock()
		close(done)
	}()
	return done
}

// initialDefaultTransport is the standard library's own network transport,
// captured before any test can swap http.DefaultTransport for a fake.
var initialDefaultTransport = http.DefaultTransport

// liveNetworkTo reports whether req would leave this machine for the real
// license service: the production host through the standard network
// transport, or through any *http.Transport that dials the network itself.
// A test fake (a swapped DefaultTransport or a custom dialer) is not live.
// liveNetworkTo, istegin gercek lisans hizmetine gercek agdan gidip gitmeyecegini soyler.
func liveNetworkTo(client *http.Client, req *http.Request) bool {
	if req.URL.Hostname() != "celikpanel.net" {
		return false
	}
	transport := http.DefaultTransport
	if client != nil && client.Transport != nil {
		transport = client.Transport
	}
	if transport == initialDefaultTransport {
		return true
	}
	plain, ok := transport.(*http.Transport)
	return ok && plain.DialContext == nil && plain.DialTLSContext == nil && plain.Dial == nil && plain.DialTLS == nil //nolint:staticcheck // the deprecated dialers only identify a test dialer
}
