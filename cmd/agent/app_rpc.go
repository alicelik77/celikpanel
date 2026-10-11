package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Supervised app processes (Node projects) — roadmap 3A. Every app is a
// systemd unit (celikapp-<siteID>.service): start/stop/restart, restart on
// crash, coming back after reboot and journald logs all come from the init
// system instead of a process manager dependency (deliberately no PM2).
//
// In production the agent is root and manages system units. On a non-root
// development agent, CELIKPANEL_SYSTEMD_USER=1 switches to `systemctl --user`
// and ~/.config/systemd/user — the same code path, honestly supervised.
//
// Gözetimli uygulama süreçleri (Node projeleri) — yol haritası 3A. Her
// uygulama bir systemd unit'idir (celikapp-<siteID>.service): başlat/durdur/
// yeniden başlat, çökünce yeniden başlama, reboot sonrası ayağa kalkma ve
// journald logları bir süreç yöneticisi bağımlılığı yerine init sisteminden
// gelir (bilinçli olarak PM2 yok).

var systemdUserMode = os.Getenv("CELIKPANEL_SYSTEMD_USER") == "1"

const appUnitMutationLockStripes = 256

var appUnitMutationLocks [appUnitMutationLockStripes]sync.Mutex

// net/rpc cannot cancel a method that was already dispatched. If the panel
// times out and starts compensation, the original method may therefore still
// be running. Serializing mutations for the same site makes the compensation
// deterministically run after that original method, rather than racing it.
func lockAppUnitMutation(siteID int) func() {
	lock := &appUnitMutationLocks[siteID%appUnitMutationLockStripes]
	lock.Lock()
	return lock.Unlock
}

func systemctlArgs(args ...string) []string {
	if systemdUserMode {
		return append([]string{"--user"}, args...)
	}
	return args
}

// appUnitDirOverride replaces the unit directory in tests.
var appUnitDirOverride string

// appUnitSystemctl runs systemctl (a test seam).
var appUnitSystemctl = func(args ...string) ([]byte, error) {
	return exec.Command("systemctl", args...).CombinedOutput()
}

func unitDir() (string, error) {
	if appUnitDirOverride != "" {
		return appUnitDirOverride, nil
	}
	if systemdUserMode {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "systemd", "user"), nil
	}
	return "/etc/systemd/system", nil
}

// appUnitAccountExists reports whether a system account exists. Swapped by
// tests.
var appUnitAccountExists = func(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}

// appUnitAccount is the account an application unit runs as on this host
// (12 Oct 2026). The Panel asks for `www-data`, which is the web server's
// account on Debian and Ubuntu only: on Arch it is `http`, on the RHEL family
// `nginx`, and a unit with `User=www-data` cannot start there. When the
// account that was asked for is that one and does not exist, the web server's
// account that does exist is used, in the order the rest of the product reads
// it (hosting_layout.go). Anything else is written as asked.
//
// appUnitAccount, bir uygulama unit'inin bu sunucuda çalıştığı hesaptır. Panel
// `www-data` ister; bu yalnız Debian ve Ubuntu'da web sunucusunun hesabıdır.
// İstenen hesap o ise ve yoksa, var olan web sunucusu hesabı kullanılır.
func appUnitAccount(requested string) string {
	if requested != "www-data" || appUnitAccountExists(requested) {
		return requested
	}
	for _, name := range []string{"nginx", "http"} {
		if appUnitAccountExists(name) {
			return name
		}
	}
	return requested
}

var appUnitNameRe = regexp.MustCompile(`^celikapp-[0-9]+$`)

// appUnitName builds and validates the unit name for a site.
// appUnitName, bir site için unit adını üretir ve doğrular.
func appUnitName(siteID int) string { return fmt.Sprintf("celikapp-%d", siteID) }

type AppApplyRequest = transport.AppApplyRequest
type AppApplyResponse = transport.AppApplyResponse

// appUnitFile is the unit of a request as a D-031 site file (second step,
// 2026-10-11): sealed with the render header ("# " comment), classified
// before every write, written through a temporary file and one rename that
// keeps the file's mode and owner, never through a symlink. The owner's own
// settings go in the unit's drop-in directory (<unit>.service.d/*.conf),
// which CelikPanel never writes or removes.
//
// appUnitFile, isteğin birimini D-031 site dosyası olarak verir; sahibin kendi
// ayarları birimin drop-in dizinine gider ve CelikPanel oraya hiç dokunmaz.
func appUnitFile(req *AppApplyRequest, dir string) services.ManagedSiteFile {
	name := appUnitName(req.SiteID)
	pathEnv := os.Getenv("PATH")
	if req.NodeVersion != "" {
		pathEnv = filepath.Dir(nodeBinPath(req.NodeVersion)) + ":" + pathEnv
	}
	params := services.AppUnitParams{
		Description:   req.Description,
		WorkDir:       req.WorkDir,
		Port:          req.Port,
		PathEnv:       pathEnv,
		Command:       req.Command,
		InstallTarget: installTarget(),
		DropInDir:     filepath.Join(dir, name+".service.d"),
	}
	if !systemdUserMode && req.RunAsUser != "" {
		params.User = appUnitAccount(req.RunAsUser)
		params.RequestedUser = req.RunAsUser
	}
	return services.ManagedSiteFile{
		Kind:                 transport.SiteFileKindAppUnit,
		Path:                 filepath.Join(dir, name+".service"),
		HeaderPrefix:         services.ManagedHeaderPrefixHash,
		Body:                 services.AppUnitBody(params),
		Trigger:              req.Trigger,
		RecordedSHA256:       req.RecordedSHA256,
		ExpectedFileSHA256:   req.ExpectedFileSHA256,
		ExpectedRenderSHA256: req.ExpectedRenderSHA256,
		Legacy:               services.LegacyAppUnitRenders(params),
		IncludeDir:           params.DropInDir,
	}
}

func validAppApplyRequest(req *AppApplyRequest) string {
	if req == nil || req.SiteID <= 0 || strings.TrimSpace(req.Command) == "" || req.Port <= 0 {
		return "site id, command and port are required"
	}
	if strings.ContainsAny(req.Command, "\n") || strings.ContainsAny(req.WorkDir, "\n") {
		return "invalid characters in command or workdir"
	}
	return ""
}

// appUnitHeldMessage is the error an older Panel reads for a unit the rule
// did not write (a newer Panel reads File).
func appUnitHeldMessage(result transport.SiteFileResult) string {
	switch result.Outcome {
	case transport.SiteFileOutcomeKept:
		return "the application unit file " + result.Path + " is not CelikPanel's unchanged text (" +
			result.State + "); it was kept and not changed"
	case transport.SiteFileOutcomeMissing:
		return "the application unit file " + result.Path + " is missing; it was not recreated"
	}
	return "the application unit file " + result.Path + " was not replaced (" + result.State + " " + result.Reason + ")"
}

// ApplyAppUnit writes the unit file under the D-031 rule, reloads systemd
// only when it wrote, and enables + (re)starts the app. "take" (the site
// configuration page's choice) reloads systemd and restarts the app only if
// it is running.
// ApplyAppUnit, birim dosyasını D-031 kuralıyla yazar; yalnız yazdıysa
// systemd'yi yeniden yükler ve uygulamayı etkinleştirip (yeniden) başlatır.
func (a *Agent) ApplyAppUnit(req *AppApplyRequest, resp *AppApplyResponse) error {
	if message := validAppApplyRequest(req); message != "" {
		resp.Error = message
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()

	name := appUnitName(req.SiteID)
	dir, err := unitDir()
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if req.NodeVersion != "" {
		if _, err := os.Stat(nodeBinPath(req.NodeVersion)); err != nil {
			// Honest: refuse to write a unit that cannot start.
			// Dürüstlük: başlayamayacak bir unit yazmayı reddet.
			resp.Error = fmt.Sprintf("node %s is not installed on this server", req.NodeVersion)
			return nil
		}
	}

	result, err := services.ApplyManagedSiteFile(appUnitFile(req, dir), nil, func() error {
		if out, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	})
	resp.File = &result
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if result.Outcome == transport.SiteFileOutcomeFailed {
		resp.Error = "application unit not activated, the previous file was put back: " + result.Detail
		return nil
	}
	if !services.SiteFileApplied(result.Outcome) {
		resp.Error = appUnitHeldMessage(result)
		return nil
	}
	if result.AdoptedFrom != "" || result.BackupPath != "" {
		log.Printf("ApplyAppUnit site %d: %s (adopted %q, owner's copy %q)",
			req.SiteID, result.Outcome, result.AdoptedFrom, result.BackupPath)
	}

	steps := [][]string{
		systemctlArgs("enable", name),
		systemctlArgs("restart", name),
	}
	if result.Outcome == transport.SiteFileOutcomeTaken {
		// The owner's choice changes the file, not whether the app runs.
		steps = [][]string{systemctlArgs("try-restart", name)}
	}
	for _, args := range steps {
		if out, err := appUnitSystemctl(args...); err != nil {
			resp.Error = fmt.Sprintf("systemctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			return nil
		}
	}
	resp.Unit = name
	return nil
}

// InspectAppUnit classifies a site's unit file and returns the difference
// from CelikPanel's text for the same inputs. It writes nothing (D-031).
// InspectAppUnit, sitenin birim dosyasını sınıflandırır; yazmaz.
func (a *Agent) InspectAppUnit(req *AppApplyRequest, resp *transport.InspectSiteFileResponse) error {
	if message := validAppApplyRequest(req); message != "" {
		resp.Error = message
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	dir, err := unitDir()
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.File, resp.Diff, resp.DiffTruncated = services.InspectManagedSiteFile(appUnitFile(req, dir))
	return nil
}

func installTarget() string {
	if systemdUserMode {
		return "default.target"
	}
	return "multi-user.target"
}

// sanitizeUnitValue strips newlines so a crafted value cannot inject unit
// directives.
// sanitizeUnitValue, satır sonlarını temizler; kurgulanmış bir değer unit
// yönergesi enjekte edemesin.
func sanitizeUnitValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s)
}

type AppControlRequest = transport.AppControlRequest

func (a *Agent) ControlAppUnit(req *AppControlRequest, resp *AppApplyResponse) error {
	if req == nil || req.SiteID <= 0 {
		resp.Error = "a positive site identity is required"
		return nil
	}
	switch req.Action {
	case "start", "stop", "restart":
	default:
		resp.Error = "action must be start, stop or restart"
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	name := appUnitName(req.SiteID)
	if out, err := exec.Command("systemctl", systemctlArgs(req.Action, name)...).CombinedOutput(); err != nil {
		resp.Error = fmt.Sprintf("%v: %s", err, strings.TrimSpace(string(out)))
		return nil
	}
	resp.Unit = name
	return nil
}

func appUnitSystemdState(name string) (loadState, activeState string, err error) {
	out, commandErr := appUnitSystemctl(systemctlArgs("show", name, "--property=LoadState,ActiveState")...)
	if commandErr != nil {
		return "", "", fmt.Errorf(
			"systemctl show: %v: %s",
			commandErr,
			strings.TrimSpace(string(out)),
		)
	}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "LoadState":
			loadState = strings.TrimSpace(value)
		case "ActiveState":
			activeState = strings.TrimSpace(value)
		}
	}
	if loadState == "" || activeState == "" {
		return "", "", errors.New("systemctl returned an incomplete unit state")
	}
	return loadState, activeState, nil
}

// RemoveAppUnit stops, disables and deletes the unit (project type changed
// or site deleted).
// RemoveAppUnit, unit'i durdurur, devre dışı bırakır ve siler (proje tipi
// değişti ya da site silindi).
func (a *Agent) RemoveAppUnit(req *AppControlRequest, resp *AppApplyResponse) error {
	if req == nil || req.SiteID <= 0 {
		resp.Error = "a positive site identity is required"
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	name := appUnitName(req.SiteID)
	dir, err := unitDir()
	if err != nil {
		log.Printf("RemoveAppUnit site %d: locate unit directory: %v", req.SiteID, err)
		resp.Error = "application unit cleanup incomplete"
		return nil
	}
	unitPath := filepath.Join(dir, name+".service")
	info, statErr := os.Lstat(unitPath)
	unitFileExists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		log.Printf("RemoveAppUnit site %d: inspect unit: %v", req.SiteID, statErr)
		resp.Error = "application unit cleanup incomplete"
		return nil
	}
	if unitFileExists && !info.Mode().IsRegular() {
		log.Printf("RemoveAppUnit site %d: refusing non-regular unit file", req.SiteID)
		resp.Error = "application unit cleanup refused"
		return nil
	}

	loadState, activeState, stateErr := appUnitSystemdState(name)
	if !unitFileExists && stateErr == nil && loadState == "not-found" && activeState == "inactive" {
		resp.Unit = name
		return nil
	}

	// Even when the unit file is already gone, systemd may still have the unit
	// loaded and its process running. Try every cleanup step, then use a final
	// filesystem + systemd proof to decide success. Intermediate command errors
	// are retained for diagnostics but are not fatal if that final proof shows
	// the requested state was nevertheless reached.
	var intermediateFailures []error
	if stateErr != nil {
		intermediateFailures = append(intermediateFailures, stateErr)
	}
	if output, err := appUnitSystemctl(systemctlArgs("stop", name)...); err != nil {
		intermediateFailures = append(intermediateFailures, fmt.Errorf("stop: %v: %s", err, strings.TrimSpace(string(output))))
	}
	if output, err := appUnitSystemctl(systemctlArgs("disable", name)...); err != nil {
		intermediateFailures = append(intermediateFailures, fmt.Errorf("disable: %v: %s", err, strings.TrimSpace(string(output))))
	}
	// D-031: a unit that is not CelikPanel's unchanged text is copied beside
	// it first; the drop-in directory (<unit>.service.d) is never touched.
	if unitFileExists {
		backup, err := services.BackupUnlessManaged(unitPath, services.ManagedHeaderPrefixHash, nil)
		if err != nil {
			log.Printf("RemoveAppUnit site %d: keep a copy of the edited unit: %v", req.SiteID, err)
			resp.Error = "application unit cleanup refused: a copy of the edited unit could not be kept"
			return nil
		}
		if backup != "" {
			log.Printf("RemoveAppUnit site %d: the unit was not CelikPanel's unchanged text; a copy is kept as %s", req.SiteID, backup)
		}
	}
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		intermediateFailures = append(intermediateFailures, fmt.Errorf("remove unit file: %w", err))
	}
	services.RemoveSiteFilePending(unitPath)
	if output, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
		intermediateFailures = append(intermediateFailures, fmt.Errorf("daemon-reload: %v: %s", err, strings.TrimSpace(string(output))))
	}

	var finalFailures []error
	if _, err := os.Lstat(unitPath); err == nil {
		finalFailures = append(finalFailures, errors.New("unit file still exists"))
	} else if !os.IsNotExist(err) {
		finalFailures = append(finalFailures, fmt.Errorf("verify unit file removal: %w", err))
	}
	finalLoadState, finalActiveState, finalStateErr := appUnitSystemdState(name)
	if finalStateErr != nil {
		finalFailures = append(finalFailures, fmt.Errorf("verify unit state: %w", finalStateErr))
	} else if finalLoadState != "not-found" || finalActiveState != "inactive" {
		finalFailures = append(finalFailures, fmt.Errorf(
			"unit remains load=%s active=%s",
			finalLoadState,
			finalActiveState,
		))
	}
	if finalErr := errors.Join(finalFailures...); finalErr != nil {
		detail := errors.Join(append(intermediateFailures, finalErr)...)
		log.Printf("RemoveAppUnit site %d: %v", req.SiteID, detail)
		resp.Error = "application unit cleanup incomplete"
	} else if warning := errors.Join(intermediateFailures...); warning != nil {
		log.Printf("RemoveAppUnit site %d: cleanup completed after intermediate errors: %v", req.SiteID, warning)
	}
	resp.Unit = name
	return nil
}

type AppStatusResponse = transport.AppStatusResponse

// AppUnitStatus reads live state from systemd (PID, memory, CPU time) — the
// same honest numbers `systemctl status` would show.
// AppUnitStatus, systemd'den canlı durumu okur (PID, bellek, CPU süresi) —
// `systemctl status`un göstereceği dürüst sayıların aynısı.
func (a *Agent) AppUnitStatus(req *AppControlRequest, resp *AppStatusResponse) error {
	name := appUnitName(req.SiteID)
	out, err := exec.Command("systemctl", systemctlArgs("show", name,
		"--property=LoadState,ActiveState,MainPID,MemoryCurrent,CPUUsageNSec,ActiveEnterTimestamp")...).Output()
	if err != nil {
		return nil
	}

	props := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			props[k] = v
		}
	}

	resp.Exists = props["LoadState"] == "loaded"
	resp.Active = props["ActiveState"]
	resp.PID, _ = strconv.Atoi(props["MainPID"])
	if mem, err := strconv.ParseInt(props["MemoryCurrent"], 10, 64); err == nil && mem >= 0 {
		resp.MemoryMB = mem / (1024 * 1024)
	}
	if cpu, err := strconv.ParseInt(props["CPUUsageNSec"], 10, 64); err == nil && cpu >= 0 {
		resp.CPUUsec = cpu / 1000
	}
	resp.Uptime = props["ActiveEnterTimestamp"]
	return nil
}

type AppLogsRequest = transport.AppLogsRequest
type AppLogsResponse = transport.AppLogsResponse

// AppUnitLogs tails the app's journal.
// AppUnitLogs, uygulamanın journal'ını okur.
func (a *Agent) AppUnitLogs(req *AppLogsRequest, resp *AppLogsResponse) error {
	if req.Lines <= 0 || req.Lines > 1000 {
		req.Lines = 100
	}
	name := appUnitName(req.SiteID)
	if !appUnitNameRe.MatchString(name) {
		resp.Error = "invalid unit"
		return nil
	}

	args := []string{"-u", name + ".service", "-n", strconv.Itoa(req.Lines), "--no-pager", "-o", "short-iso"}
	if systemdUserMode {
		args = append([]string{"--user"}, args...)
	}
	out, err := exec.Command("journalctl", args...).CombinedOutput()
	if err != nil {
		resp.Error = strings.TrimSpace(string(out))
		return nil
	}

	resp.Lines = []string{}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" && !strings.HasPrefix(line, "-- ") {
			resp.Lines = append(resp.Lines, line)
		}
	}
	return nil
}
