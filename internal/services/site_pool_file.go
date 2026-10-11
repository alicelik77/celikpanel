package services

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A site's PHP-FPM pool file under D-031 (second step, 2026-10-11).
//
// The file is INI, so its render header is a ";" comment line:
//
//	; celikpanel-render v2 sha256=<SHA-256 of every byte after this line>
//
// CelikPanel's text is the pool's identity (the site's system user, this
// host's socket for the version, the web server's account owning it, 0660)
// and the process-manager settings. No database column holds those settings:
// the pool page reads them from the file. So CelikPanel's text for an
// operation takes them from the request (a pool save), else from CelikPanel's
// own held text beside the file (<file>.celikpanel-pending, whose header
// verifies), else from the values the file carries for those keys, else the
// defaults, clamped as a save clamps them. PHP-FPM has no include mechanism
// for a pool; the owner's directives are supported in place: a file with
// CelikPanel's header and lines CelikPanel does not render is owner-edited,
// kept, and the page's difference shows exactly those lines. No save or switch
// drops them silently again.
//
// Havuz dosyası INI'dir; başlığı ";" yorum satırıdır. Sahibin yönergeleri
// dosyanın içinde desteklenir: başlıklı ve Panel'in üretmediği satırları olan
// dosya sahibin düzenlemesidir, korunur ve fark tam o satırları gösterir.

// poolValues are the process-manager settings of a pool.
type poolValues struct {
	pm                                                         string
	maxChildren, startServers, minSpare, maxSpare, maxRequests int
}

func defaultPoolValues() poolValues {
	return poolValues{pm: "dynamic", maxChildren: 5, startServers: 2, minSpare: 1, maxSpare: 3, maxRequests: 500}
}

// clamped bounds the values as a pool save always has (UpdatePoolConfig).
func (v poolValues) clamped() poolValues {
	if !validPMModes[v.pm] {
		v.pm = "dynamic"
	}
	v.maxChildren = clamp(v.maxChildren, 1, 200, 5)
	v.startServers = clamp(v.startServers, 1, v.maxChildren, 2)
	v.minSpare = clamp(v.minSpare, 1, v.maxChildren, 1)
	v.maxSpare = clamp(v.maxSpare, v.minSpare, v.maxChildren, 3)
	v.maxRequests = clamp(v.maxRequests, 1, 100000, 500)
	return v
}

func poolValuesFromRequest(request transport.SitePoolValues, fallback poolValues) poolValues {
	values := poolValues{
		pm: request.PM, maxChildren: request.PMMaxChildren, startServers: request.PMStartServers,
		minSpare: request.PMMinSpareServers, maxSpare: request.PMMaxSpareServers, maxRequests: request.PMMaxRequests,
	}
	if values.pm == "" {
		values.pm = fallback.pm
	}
	return values.clamped()
}

// poolValuesIn reads the process-manager keys a file carries (the last line
// of each wins, as in PHP-FPM); a key that is absent or not a number keeps the
// default. found says whether any key was read.
func poolValuesIn(content []byte) (poolValues, bool) {
	values := defaultPoolValues()
	found := false
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		key, value, ok := configPair(line)
		if !ok {
			continue
		}
		number, numberErr := strconv.Atoi(value)
		switch key {
		case "pm":
			values.pm, found = value, true
		case "pm.max_children":
			if numberErr == nil {
				values.maxChildren, found = number, true
			}
		case "pm.start_servers":
			if numberErr == nil {
				values.startServers, found = number, true
			}
		case "pm.min_spare_servers":
			if numberErr == nil {
				values.minSpare, found = number, true
			}
		case "pm.max_spare_servers":
			if numberErr == nil {
				values.maxSpare, found = number, true
			}
		case "pm.max_requests":
			if numberErr == nil {
				values.maxRequests, found = number, true
			}
		}
	}
	return values, found
}

// poolValuesWritten reads the values of a file in the exact form the earlier
// releases wrote (legacyPoolFormat), unclamped, so the legacy text can be
// rendered again from them. ok is false for any other form.
func poolValuesWritten(content []byte) (poolValues, bool) {
	lines := strings.Split(string(content), "\n")
	if len(lines) != 15 || lines[14] != "" {
		return poolValues{}, false
	}
	value := func(index int, key string) (string, bool) {
		prefix := key + " = "
		if !strings.HasPrefix(lines[index], prefix) {
			return "", false
		}
		return lines[index][len(prefix):], true
	}
	number := func(index int, key string) (int, bool) {
		text, ok := value(index, key)
		if !ok {
			return 0, false
		}
		parsed, err := strconv.Atoi(text)
		return parsed, err == nil && strconv.Itoa(parsed) == text
	}
	var values poolValues
	var ok [6]bool
	values.pm, ok[0] = value(7, "pm")
	values.maxChildren, ok[1] = number(8, "pm.max_children")
	values.startServers, ok[2] = number(9, "pm.start_servers")
	values.minSpare, ok[3] = number(10, "pm.min_spare_servers")
	values.maxSpare, ok[4] = number(11, "pm.max_spare_servers")
	values.maxRequests, ok[5] = number(12, "pm.max_requests")
	for _, each := range ok {
		if !each {
			return poolValues{}, false
		}
	}
	return values, true
}

// sitePoolIdentity is the pool identity CelikPanel writes for a site on this
// host (alpha.82's creation identity).
func sitePoolIdentity(version, poolName, username string) poolIdentity {
	account := phpWebServerAccount()
	return poolIdentity{
		user: username, group: username, listen: PHPFPMSocketPath(version, poolName),
		listenOwner: account, listenGroup: account, listenMode: "0660",
	}
}

// SitePoolBody is CelikPanel's text of a site's pool (the body under the
// header): a comment that says what the file is, then the pool.
func sitePoolBody(poolName string, identity poolIdentity, values poolValues) string {
	return "; The PHP-FPM pool of " + poolName + ", written by CelikPanel. A line you add or change here is kept:\n" +
		"; CelikPanel then shows the difference and asks before writing this file again.\n" +
		"; Buraya eklediğiniz ya da değiştirdiğiniz satır korunur; CelikPanel yeniden yazmadan önce sorar.\n" +
		legacyPoolText(poolName, identity, values)
}

func sitePoolName(siteID int) string { return fmt.Sprintf("site%d", siteID) }

// sitePoolFile is the pool of one site at one PHP version with the given
// values (nil: CelikPanel's held text, else the file's values, else defaults).
func sitePoolFile(request transport.SitePoolRequest, version string, values *poolValues, trigger string) ManagedSiteFile {
	poolName := sitePoolName(request.SiteID)
	path := poolFilePath(version, poolName)
	var content []byte
	if data, err := readManagedConfig(path); err == nil && len(data) <= maxManagedSiteFileBytes {
		content = data
	}
	chosen := defaultPoolValues()
	switch {
	case values != nil:
		chosen = *values
	default:
		if held, ok := HeldPendingBody(path, ManagedHeaderPrefixINI); ok {
			if parsed, found := poolValuesIn([]byte(held)); found {
				chosen = parsed.clamped()
				break
			}
		}
		if parsed, found := poolValuesIn(content); found {
			chosen = parsed.clamped()
		}
	}
	return ManagedSiteFile{
		Kind:                 transport.SiteFileKindPHPPool,
		Path:                 path,
		HeaderPrefix:         ManagedHeaderPrefixINI,
		Body:                 sitePoolBody(poolName, sitePoolIdentity(version, poolName, request.Username), chosen),
		Trigger:              trigger,
		RecordedSHA256:       request.RecordedSHA256,
		ExpectedFileSHA256:   request.ExpectedFileSHA256,
		ExpectedRenderSHA256: request.ExpectedRenderSHA256,
		Legacy:               legacyPoolRenders(version, poolName, request.Username, content),
	}
}

func validateSitePoolRequest(request transport.SitePoolRequest) error {
	if request.SiteID < 1 {
		return fmt.Errorf("a positive site identity is required")
	}
	if err := ValidatePHPVersion(request.PHPVersion); err != nil {
		return err
	}
	return validatePoolIdentity("user", request.Username)
}

// InspectSitePool classifies a site's pool and computes the difference from
// CelikPanel's text. It writes nothing.
func (pm *PHPPoolManager) InspectSitePool(request transport.SitePoolRequest) (transport.SiteFileResult, string, bool, error) {
	if err := validateSitePoolRequest(request); err != nil {
		return transport.SiteFileResult{}, "", false, err
	}
	managedConfigMutationMu.Lock()
	defer managedConfigMutationMu.Unlock()
	result, diff, truncated := InspectManagedSiteFile(sitePoolFile(request, request.PHPVersion, nil, transport.SiteFileTriggerChange))
	return result, diff, truncated, nil
}

// ApplySitePool is every write of a site's pool: a save of the pool page's
// values, a PHP version switch, and the site configuration page's take,
// recreate and apply. The file is classified first; only the render rule's
// writes happen (ApplyManagedSiteFile).
func (pm *PHPPoolManager) ApplySitePool(request transport.SitePoolRequest) (transport.SiteFileResult, error) {
	if err := validateSitePoolRequest(request); err != nil {
		return transport.SiteFileResult{}, err
	}
	managedConfigMutationMu.Lock()
	defer managedConfigMutationMu.Unlock()
	return applySitePoolLocked(request)
}

func phpValidate(version string) func() error {
	return func() error { return phpFPMConfigTest(version) }
}

func phpReload(version string) func() error {
	return func() error { return reloadPHPFPM(version) }
}

func applySitePoolLocked(request transport.SitePoolRequest) (transport.SiteFileResult, error) {
	version := request.PHPVersion
	switch request.Operation {
	case transport.SitePoolOperationSave:
		if request.Values == nil {
			return transport.SiteFileResult{}, fmt.Errorf("a pool save needs its values")
		}
		current := sitePoolFile(request, version, nil, transport.SiteFileTriggerChange)
		fallback, _ := poolValuesIn([]byte(current.Body))
		values := poolValuesFromRequest(*request.Values, fallback)
		return ApplyManagedSiteFile(sitePoolFile(request, version, &values, transport.SiteFileTriggerChange),
			phpValidate(version), phpReload(version))
	case transport.SitePoolOperationTake:
		return ApplyManagedSiteFile(sitePoolFile(request, version, nil, transport.SiteFileTriggerTake),
			phpValidate(version), phpReload(version))
	case transport.SitePoolOperationRecreate:
		return ApplyManagedSiteFile(sitePoolFile(request, version, nil, transport.SiteFileTriggerRecreate),
			phpValidate(version), phpReload(version))
	case transport.SitePoolOperationApply:
		return ApplyManagedSiteFile(sitePoolFile(request, version, nil, transport.SiteFileTriggerChange),
			phpValidate(version), phpReload(version))
	case transport.SitePoolOperationSwitch:
		return switchSitePoolLocked(request)
	}
	return transport.SiteFileResult{}, fmt.Errorf("unknown pool operation %q", request.Operation)
}

// switchSitePoolLocked moves a site's pool to another PHP version. The source
// file must be CelikPanel's unchanged text (or an adopted earlier text): any
// other source is kept with CelikPanel's text held beside it and nothing is
// written. The target is written only where no file is; then the source is
// removed. A failure on either side puts both back.
func switchSitePoolLocked(request transport.SitePoolRequest) (transport.SiteFileResult, error) {
	oldVersion, newVersion := request.PHPVersion, request.NewPHPVersion
	if err := ValidatePHPVersion(newVersion); err != nil {
		return transport.SiteFileResult{}, err
	}
	if oldVersion == newVersion {
		return transport.SiteFileResult{}, fmt.Errorf("source and target PHP versions are identical")
	}
	source := sitePoolFile(request, oldVersion, nil, transport.SiteFileTriggerChange)
	sourceInspected := source.inspect()
	if sourceInspected.state != transport.SiteFileManagedUnchanged {
		// Kept, missing or refused: the rule writes nothing here; a kept file
		// gets CelikPanel's text beside it.
		return ApplyManagedSiteFile(source, nil, nil)
	}
	values, found := poolValuesIn(sourceInspected.content)
	if !found {
		values = defaultPoolValues()
	}
	values = values.clamped()
	targetRequest := request
	targetRequest.PHPVersion = newVersion
	targetRequest.RecordedSHA256 = ""
	target := sitePoolFile(targetRequest, newVersion, &values, transport.SiteFileTriggerCreate)
	targetInspected := target.inspect()
	if targetInspected.state != transport.SiteFileAbsent {
		result := target.result(targetInspected)
		result.Outcome = transport.SiteFileOutcomeRefused
		result.Reason = transport.SiteFileReasonNotApplicable
		return result, nil
	}
	result, err := ApplyManagedSiteFile(target, phpValidate(newVersion), phpReload(newVersion))
	if err != nil || !SiteFileApplied(result.Outcome) {
		return result, err
	}
	result.AdoptedFrom = sourceInspected.adoptedFrom
	removeErr := removeInspectedSiteFile(source.Path, sourceInspected)
	if removeErr == nil {
		removeErr = phpFPMConfigTest(oldVersion)
		if removeErr == nil {
			removeErr = reloadPHPFPM(oldVersion)
		}
		if removeErr != nil {
			removeErr = errors.Join(removeErr, restoreManagedSiteFile(source.Path, sourceInspected), reloadPHPFPM(oldVersion))
		}
	}
	if removeErr != nil {
		cleanup := errors.Join(os.Remove(target.Path), reloadPHPFPM(newVersion))
		result.Outcome = transport.SiteFileOutcomeFailed
		result.Reason = transport.SiteFileReasonReloadFailed
		result.Detail = boundedSiteFileDetail("remove the PHP " + oldVersion + " pool: " + removeErr.Error())
		if cleanup != nil {
			return result, fmt.Errorf("remove source PHP %s pool: %w; rollback target pool: %v", oldVersion, removeErr, cleanup)
		}
		return result, nil
	}
	removeStalePending(source.Path)
	return result, nil
}

// removeInspectedSiteFile removes the file only when it is still the bytes
// that were classified.
func removeInspectedSiteFile(path string, inspected siteFileInspection) error {
	current, err := readManagedConfig(path)
	if err != nil {
		return err
	}
	if sha256Hex(current) != inspected.fileSHA {
		return errSiteFileChanged
	}
	return removeManagedConfigFile(path)
}

// siteFileHeld turns a result the rule did not apply into an error for the
// pool RPCs that answer only with an error.
func siteFileHeld(result transport.SiteFileResult, err error) error {
	if err != nil {
		return err
	}
	if SiteFileApplied(result.Outcome) {
		return nil
	}
	if result.Outcome == transport.SiteFileOutcomeFailed {
		return fmt.Errorf("%s: %s; previous configuration restored and activated", result.Reason, result.Detail)
	}
	return &SiteFileHeldError{File: result}
}
