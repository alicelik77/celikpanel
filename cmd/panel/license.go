package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alicelik/celikpanel/internal/licensing"
	"golang.org/x/term"
)

const panelLicensePath = "/api/v1/panel/license"
const pendingLicenseFile = "/var/lib/celikpanel-license/install.json"

func newServerLicense(file string) (*licensing.Manager, error) {
	key, err := hex.DecodeString(licensing.PublicKeyHex)
	if err != nil {
		return nil, errors.New("license verification key is not provisioned")
	}
	machine, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return nil, err
	}
	id, err := licensing.ServerID(machine)
	if err != nil {
		return nil, err
	}
	// NewServer is exactly licensing.New in every ordinary build. Only the
	// acceptance_license test build, which release packaging refuses, installs
	// the acceptance fixture seam there.
	return licensing.NewServer(file, key, id)
}

// This command runs before panel database initialization and never emits a key.
// The caller has already verified the immutable release and holds its lock.
func runLicenseCLI(args []string) (bool, error) {
	if len(args) != 1 || args[0] != "--activate-install-license" {
		return false, nil
	}
	if os.Geteuid() != 0 {
		return true, errors.New("license installation requires root")
	}
	installed, err := newServerLicense(filepath.Join(dataDir(), "license.json"))
	if err != nil {
		return true, err
	}
	if installed.Status().CanProvision {
		return true, nil
	}
	targetFile := pendingLicenseFile
	if _, statErr := os.Lstat(filepath.Join(dataDir(), "license.json")); statErr == nil {
		targetFile = filepath.Join(dataDir(), "license.json")
	}
	pending, err := newServerLicense(targetFile)
	if err != nil {
		return true, err
	}
	if pending.Status().CanProvision {
		return true, nil
	}
	// An existing binding can recover without asking for the original key.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = installed.Refresh(ctx, true)
	if installed.Status().CanProvision {
		return true, nil
	}
	_ = pending.Refresh(ctx, true)
	if pending.Status().CanProvision {
		return true, nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return true, errors.New("rerun installation in a terminal to enter your license from https://celikpanel.net/account/")
	}
	defer tty.Close()
	fmt.Fprintln(tty, "Get your free annual license at https://celikpanel.net/account/ / Yıllık ücretsiz lisansınızı websitesinden alın.")
	fmt.Fprint(tty, "License key (hidden) / Lisans anahtarı (gizli): ")
	terminalState, err := term.GetState(int(tty.Fd()))
	if err != nil {
		return true, err
	}
	interrupted := make(chan os.Signal, 1)
	finished := make(chan struct{})
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		select {
		case <-interrupted:
			_ = term.Restore(int(tty.Fd()), terminalState)
			fmt.Fprintln(tty, "\nInstallation interrupted; run the same command again / Kurulum kesildi; aynı komutu yeniden çalıştırın.")
			os.Exit(130)
		case <-finished:
		}
	}()
	secret, err := term.ReadPassword(int(tty.Fd()))
	signal.Stop(interrupted)
	close(finished)
	_ = term.Restore(int(tty.Fd()), terminalState)
	fmt.Fprintln(tty)
	if err != nil {
		return true, errors.New("license entry interrupted; run the same installation command again")
	}
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()
	hostname, err := localServerName()
	if err != nil {
		return true, err
	}
	activateCtx, activateCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer activateCancel()
	if err = pending.Activate(activateCtx, strings.TrimSpace(string(secret)), hostname); err != nil {
		return true, err
	}
	fmt.Fprintln(tty, "License activated / Lisans etkinleştirildi.")
	return true, nil
}

func (p *Panel) handleLicense(w http.ResponseWriter, r *http.Request) {
	c := currentCaller(r)
	if c == nil || !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, "admin_only", "administrator access required", "")
		return
	}
	if p.license == nil {
		writeCodedError(w, http.StatusServiceUnavailable, errCodeLicenseStatusUnavailable, "license status could not be checked", "")
		return
	}
	var status licensing.Status
	switch r.Method {
	case http.MethodGet:
		// Opening the license page joins the shared check when one is due; a
		// still-valid receipt answers after at most a few seconds even while the
		// license service is unreachable. "Check again" (POST refresh) forces it.
		status = p.license.AccessStatus(r.Context())
	case http.MethodPost:
		var input struct {
			Action string `json:"action"`
			Key    string `json:"key"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		dec.DisallowUnknownFields()
		if dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF {
			writeCodedError(w, 400, "invalid_request", "invalid license request", "")
			return
		}
		var err error
		switch input.Action {
		case "activate":
			hostname, e := localServerName()
			if e != nil {
				err = e
			} else {
				err = p.license.Activate(r.Context(), input.Key, hostname)
			}
		case "refresh":
			err = p.license.Refresh(r.Context(), true)
		default:
			writeCodedError(w, 400, "invalid_request", "choose activate or refresh", "")
			return
		}
		if err != nil {
			writeCodedError(w, 400, "license_action_failed", err.Error(), "")
			return
		}
		status = p.license.Status()
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(405)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(status)
}

const panelLicenseAccessPath = "/api/v1/license/access"

// All authenticated roles can read access, but only admins can see or change
// the license itself. No key, binding, or account identity is exposed here.
func (p *Panel) handleLicenseAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	status := p.panelLicenseStatus(r.Context())
	now := time.Now()
	answer := newLicenseAccessAnswer(status, now)
	// The term's end is the administrator's to act on (renewal); resellers
	// and customers are never told a licence date. API tokens cannot reach
	// this route at all (api_token_auth.go lists no licence route).
	if c := currentCaller(r); c != nil && c.hasAccountRole(roleAdmin) && status.ExpiresAt > 0 {
		left := max(0, status.ExpiresAt-now.Unix())
		answer.TermEndsInSeconds = &left
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(answer)
}

// licenseAccessAnswer is the browser's access observation. ValidUntil is the
// absolute deadline on THIS server's clock (kept for older pages);
// ValidForSeconds is the same deadline as the time left at the moment of the
// answer, measured on the same clock: ValidUntil minus now, rounded to the
// millisecond and never below 0 (0 when there is no deadline or it has
// passed). The page takes its deadline as the moment it asked plus this time
// left, so it never compares its own clock with the server's (D-032's clock
// rule). Without it, a server clock more than about a minute behind the
// browser's made every positive answer look expired and no page loaded
// (set14b D2, Debian 13, 13 Oct 2026); a server clock ahead made access look
// valid longer than the server allowed.
// licenseAccessAnswer, tarayıcının erişim gözlemidir. ValidUntil BU sunucunun
// saatiyle mutlak son andır (eski sayfalar için korunur); ValidForSeconds aynı
// son anın yanıt anındaki kalan süresidir, aynı saatle ölçülür: ValidUntil
// eksi şimdi, milisaniyeye yuvarlanır ve 0'ın altına inmez (son an yoksa ya da
// geçtiyse 0). Sayfa son anı sorduğu an artı bu kalan süre olarak alır; kendi
// saatini sunucununkiyle hiç karşılaştırmaz (D-032 saat kuralı). Bu alan
// yokken sunucu saati tarayıcınınkinden yaklaşık bir dakikadan fazla gerideyse
// her olumlu yanıt süresi dolmuş görünüyor ve hiçbir sayfa açılmıyordu (set14b
// D2); sunucu saati ilerideyse erişim sunucunun izninden uzun geçerli görünüyordu.
type licenseAccessAnswer struct {
	CanUsePanel     bool    `json:"can_use_panel"`
	ValidUntil      int64   `json:"valid_until"`
	ValidForSeconds float64 `json:"valid_for_seconds"`
	State           string  `json:"state"`
	Observation     string  `json:"observation"`
	// Reason is present only for verification_unavailable with a known cause
	// (offline_ended, clock_behind), so the page can say who acts and how.
	// Reason, yalnız nedeni bilinen verification_unavailable için gelir.
	Reason string `json:"reason,omitempty"`
	// Level is what the panel allows now (D-047, stage 1): "full", or
	// "limited" while the licence does not allow full use: creating new
	// things and changing settings are refused, everything else works
	// (license_routes.go). CanUsePanel keeps its meaning, full use, for the
	// scripts that read it as "the licence is active"; the interface mounts
	// its pages at both levels.
	// Level: "full" ya da lisans tam kullanıma izin vermezken "limited".
	Level string `json:"level"`
	// TermEndsInSeconds is the time left until the licence term ends, on the
	// server's clock at the moment of the answer (0 once it has ended), for
	// administrators only: the interface warns 30, 7 and 1 day before.
	TermEndsInSeconds *int64 `json:"term_ends_in_seconds,omitempty"`
}

const (
	licenceLevelFull    = "full"
	licenceLevelLimited = "limited"
)

// newLicenseAccessAnswer builds the answer from the licence status at `now`,
// the moment the answer is written.
// newLicenseAccessAnswer, yanıtı yanıtın yazıldığı `now` anındaki lisans
// durumundan kurar.
func newLicenseAccessAnswer(status licensing.Status, now time.Time) licenseAccessAnswer {
	until := min(status.ExpiresAt, status.OfflineUntil)
	validFor := 0.0
	if until > 0 {
		left := time.Unix(until, 0).Sub(now).Seconds()
		validFor = max(0, math.Round(left*1000)/1000)
	}
	level := licenceLevelLimited
	if status.CanProvision {
		level = licenceLevelFull
	}
	return licenseAccessAnswer{
		CanUsePanel: status.CanProvision, ValidUntil: until, ValidForSeconds: validFor,
		State: status.State, Observation: status.Observation, Reason: status.Reason, Level: level,
	}
}

// allowLicensedPanel admits an authenticated management request by its
// licence route class (license_routes.go). A class that does not need full
// use passes without reading the licence at all, so a security action, a
// backup, a deletion, one's own account, a panel update or the licence screen
// never waits for the licence service and is never refused for a licence
// reason (D-047, stage 1). Only "change" and "growth" need full use; they are
// refused as before, with the reason in words for the caller's role. This
// gate runs on authenticated HTTP requests, never on agents or schedulers.
//
// allowLicensedPanel, kimliği doğrulanmış yönetim isteğini lisans rota
// sınıfına göre kabul eder. Tam kullanım gerektirmeyen sınıf lisansı hiç
// okumadan geçer; yalnız "değişiklik" ve "büyüme" tam kullanım ister.
func (p *Panel) allowLicensedPanel(w http.ResponseWriter, r *http.Request) bool {
	if class, ok := licenceRouteClassOf(r); !ok || !class.needsFullLicence() {
		return true
	}
	status := p.panelLicenseStatus(r.Context())
	if status.CanProvision {
		return true
	}
	writeLicenceRefusal(w, status, licenceAudienceOf(currentCaller(r)))
	return false
}

// Who a licence refusal speaks to. Resellers and customers cannot act on the
// server's licence: their sentence never names it (D-047 design §a).
const (
	licenceAudienceAdministrator = "administrator"
	licenceAudienceReseller      = "reseller"
	licenceAudienceCustomer      = "customer"
)

func licenceAudienceOf(c *Caller) string {
	switch {
	case c == nil, c.hasAccountRole(roleAdmin):
		return licenceAudienceAdministrator
	case c.hasAccountRole(roleReseller):
		return licenceAudienceReseller
	}
	return licenceAudienceCustomer
}

// licenceRefusal is the answer to a request that needs full use while the
// licence does not allow it: a known state (missing, ended, rejected) is 403
// license_required; an unknown one keeps its 503 code, because an unknown
// licence is not a verdict (D-024, D-025). The codes are unchanged; only the
// sentence is new and true.
func licenceRefusal(status licensing.Status, audience string) (int, string, string) {
	code, httpStatus := "license_required", http.StatusForbidden
	if status.Observation != licensing.ObservationKnown {
		httpStatus, code = http.StatusServiceUnavailable, errCodeLicenseStatusUnavailable
		if status.State == "verification_unavailable" {
			code = errCodeLicenseVerificationUnavailable
		}
	}
	switch audience {
	case licenceAudienceReseller:
		return httpStatus, code, "New customers, domains and mailboxes cannot be created and settings cannot be changed on this server at the moment; this request did not proceed. Your customers' sites, mail and other services keep working, and viewing, backups and deleting still work. Contact the server administrator."
	case licenceAudienceCustomer:
		return httpStatus, code, "New domains, mailboxes and databases cannot be created and settings cannot be changed on this server at the moment; this request did not proceed. Everything you already have keeps working, and viewing, backups and deleting still work. Contact your provider if you need a change now."
	}
	const works = " Sites, mail, DNS, databases and scheduled tasks keep running, and security, backups, deleting, panel updates and your own account still work."
	switch code {
	case errCodeLicenseVerificationUnavailable:
		return httpStatus, code, "The server license could not be verified just now, so creating new things and changing settings wait; this request did not proceed. This does not mean the license is missing or ended." + works + " The server administrator can check again under Settings, License; this continues by itself after the next successful check."
	case errCodeLicenseStatusUnavailable:
		return httpStatus, code, "The server license status could not be read just now, so creating new things and changing settings wait; this request did not proceed. This does not mean the license is missing or ended." + works + " The panel tries again by itself; the server administrator can check again under Settings, License."
	}
	return httpStatus, code, "Creating new things and changing settings need an active server license; this request did not proceed." + works + " The server administrator activates or renews the license under Settings, License (a renewal is made in the celikpanel.net account); the panel notices it by itself."
}

func writeLicenceRefusal(w http.ResponseWriter, status licensing.Status, audience string) {
	httpStatus, code, message := licenceRefusal(status, audience)
	w.Header().Set("Cache-Control", "no-store")
	writeCodedError(w, httpStatus, code, message, "")
}

func (p *Panel) panelLicenseStatus(ctx context.Context) licensing.Status {
	if p.license == nil {
		return licensing.Status{State: "status_unavailable", Observation: licensing.ObservationUnavailable}
	}
	return p.license.AccessStatus(ctx)
}
