package services

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A site's own PHP values in its pool (alpha.128, docs/SITE-LIMITS.md).
//
// The values live in the native pool file, never in the Panel's database
// (D-022): CelikPanel's text of the pool carries them in one block after the
// pool's own lines, and reads them back from there. The limits are written
// as php_admin_value, so ini_set() and .user.ini cannot raise them; the free
// choices as php_value/php_flag. request_terminate_timeout (run time + 10 s)
// is the pool's own backstop. A site without values of its own gets no block,
// so its pool stays byte-identical to what earlier releases wrote.
//
// When a plan's (or reseller's) ceiling is below what the server's php.ini
// gives a site that chose nothing, the ceiling is written in a second block
// marked as the plan's, so the limit holds for every site of the plan and a
// raised ceiling takes it away again (it is never read back as the site's
// own value).
//
// Sitenin kendi PHP değerleri havuz dosyasındadır; sınırlar php_admin_value
// ile yazılır, uygulama yükseltemez. Kendi değeri olmayan sitenin havuzu
// eskisiyle bayt bayt aynı kalır.

const (
	sitePHPOwnComment = "; PHP settings of this site, chosen in CelikPanel. An application cannot raise the php_admin_value limits.\n" +
		"; Bu sitenin CelikPanel'de seçilen PHP ayarları; uygulama php_admin_value sınırlarını yükseltemez.\n"
	sitePHPPlanComment = "; Limits of this site's plan (the site chose no value of its own; the server's php.ini is above them).\n" +
		"; Sitenin planının sınırları (site kendi değerini seçmedi; sunucunun php.ini değeri bunların üstünde).\n"
	sitePHPOwnMarker  = "; PHP settings of this site, chosen in CelikPanel."
	sitePHPPlanMarker = "; Limits of this site's plan"
)

// Error level presets as numbers: a pool passes the value to PHP as text, and
// a number means the same on every PHP version.
var sitePHPErrorLevels = map[string]string{
	"all":        "32767", // E_ALL
	"production": "22527", // E_ALL & ~E_DEPRECATED & ~E_STRICT
	"errors":     "4437",  // E_ERROR | E_PARSE | E_CORE_ERROR | E_COMPILE_ERROR | E_USER_ERROR | E_RECOVERABLE_ERROR
}

var sitePHPTimezonePattern = regexp.MustCompile(`^(UTC|[A-Z][A-Za-z_]+(/[A-Za-z0-9_+\-]+){1,2})$`)

// ValidateSitePHPSettings checks a site's values against the fixed bounds
// of every key and the closed sets of the free choices.
func ValidateSitePHPSettings(own transport.SitePHPSettings) error {
	bound := func(name string, value, min, max int) error {
		if value != 0 && (value < min || value > max) {
			return fmt.Errorf("%s must be between %d and %d", name, min, max)
		}
		return nil
	}
	for _, err := range []error{
		bound("memory", own.MemoryMB, 16, 16384),
		bound("run time", own.TimeS, 5, 3600),
		bound("upload size", own.UploadMB, 1, 10240),
		bound("input fields", own.InputVars, 100, 100000),
		bound("processes", own.Processes, 1, 200),
	} {
		if err != nil {
			return err
		}
	}
	for name, flag := range map[string]string{"display_errors": own.DisplayErrors, "short_open_tag": own.ShortOpenTag} {
		if flag != "" && flag != "on" && flag != "off" {
			return fmt.Errorf("%s must be on or off", name)
		}
	}
	if _, ok := sitePHPErrorLevels[own.ErrorLevel]; own.ErrorLevel != "" && !ok {
		return fmt.Errorf("unknown error level %q", own.ErrorLevel)
	}
	if own.Timezone != "" {
		if !sitePHPTimezonePattern.MatchString(own.Timezone) {
			return fmt.Errorf("unknown time zone %q", own.Timezone)
		}
		if _, err := time.LoadLocation(own.Timezone); err != nil {
			return fmt.Errorf("unknown time zone %q", own.Timezone)
		}
	}
	return nil
}

// clampedSitePHPSettings keeps the site's values within the ceilings.
func clampedSitePHPSettings(own transport.SitePHPSettings, limits transport.SitePHPLimits) transport.SitePHPSettings {
	if !limits.Known {
		return own
	}
	within := func(value, ceiling int) int {
		if value > 0 && ceiling > 0 && value > ceiling {
			return ceiling
		}
		return value
	}
	own.MemoryMB = within(own.MemoryMB, limits.MemoryMB)
	own.TimeS = within(own.TimeS, limits.TimeS)
	own.UploadMB = within(own.UploadMB, limits.UploadMB)
	own.InputVars = within(own.InputVars, limits.InputVars)
	return own
}

// sitePHPSettingsIn reads the site's own values from CelikPanel's block of a
// pool text. Lines of the plan's block, and lines outside the block, are not
// the site's values.
func sitePHPSettingsIn(content []byte) transport.SitePHPSettings {
	var own transport.SitePHPSettings
	block := ""
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		switch {
		case strings.HasPrefix(line, sitePHPOwnMarker):
			block = "own"
			continue
		case strings.HasPrefix(line, sitePHPPlanMarker):
			block = "plan"
			continue
		case line == "" || strings.HasPrefix(line, ";"):
			continue
		}
		if block != "own" {
			continue
		}
		key, value, ok := configPair(line)
		if !ok {
			continue
		}
		mb := func() int {
			n, err := strconv.Atoi(strings.TrimSuffix(value, "M"))
			if err != nil || !strings.HasSuffix(value, "M") {
				return 0
			}
			return n
		}
		number := func() int {
			n, err := strconv.Atoi(value)
			if err != nil {
				return 0
			}
			return n
		}
		switch key {
		case "php_admin_value[memory_limit]":
			own.MemoryMB = mb()
		case "php_admin_value[max_execution_time]":
			own.TimeS = number()
		case "php_admin_value[upload_max_filesize]":
			own.UploadMB = mb()
		case "php_admin_value[max_input_vars]":
			own.InputVars = number()
		case "php_flag[display_errors]":
			own.DisplayErrors = value
		case "php_flag[short_open_tag]":
			own.ShortOpenTag = value
		case "php_value[date.timezone]":
			own.Timezone = value
		case "php_value[error_reporting]":
			for name, number := range sitePHPErrorLevels {
				if number == value {
					own.ErrorLevel = name
				}
			}
		}
	}
	return own
}

func sitePHPSettingsEmpty(own transport.SitePHPSettings) bool {
	own.Processes = 0
	return own == transport.SitePHPSettings{}
}

// sitePHPLimitLines renders the limit lines of memory, time, upload and
// input fields (time also carries the pool's terminate backstop).
func sitePHPLimitLines(memoryMB, timeS, uploadMB, inputVars int) string {
	var b strings.Builder
	if memoryMB > 0 {
		fmt.Fprintf(&b, "php_admin_value[memory_limit] = %dM\n", memoryMB)
	}
	if timeS > 0 {
		fmt.Fprintf(&b, "php_admin_value[max_execution_time] = %d\n", timeS)
		fmt.Fprintf(&b, "php_admin_value[max_input_time] = %d\n", timeS)
		fmt.Fprintf(&b, "request_terminate_timeout = %ds\n", timeS+10)
	}
	if uploadMB > 0 {
		fmt.Fprintf(&b, "php_admin_value[upload_max_filesize] = %dM\n", uploadMB)
		fmt.Fprintf(&b, "php_admin_value[post_max_size] = %dM\n", uploadMB)
	}
	if inputVars > 0 {
		fmt.Fprintf(&b, "php_admin_value[max_input_vars] = %d\n", inputVars)
	}
	return b.String()
}

// sitePHPSettingsBlock is the text CelikPanel renders for a site's values
// ("" when the site has none and no plan ceiling applies).
func sitePHPSettingsBlock(own transport.SitePHPSettings, limits transport.SitePHPLimits, server func() transport.SitePHPServerValues) string {
	own = clampedSitePHPSettings(own, limits)
	var b strings.Builder
	if !sitePHPSettingsEmpty(own) {
		b.WriteString(sitePHPOwnComment)
		b.WriteString(sitePHPLimitLines(own.MemoryMB, own.TimeS, own.UploadMB, own.InputVars))
		if own.DisplayErrors != "" {
			fmt.Fprintf(&b, "php_flag[display_errors] = %s\n", own.DisplayErrors)
		}
		if own.ErrorLevel != "" {
			fmt.Fprintf(&b, "php_value[error_reporting] = %s\n", sitePHPErrorLevels[own.ErrorLevel])
		}
		if own.Timezone != "" {
			fmt.Fprintf(&b, "php_value[date.timezone] = %s\n", own.Timezone)
		}
		if own.ShortOpenTag != "" {
			fmt.Fprintf(&b, "php_flag[short_open_tag] = %s\n", own.ShortOpenTag)
		}
	}
	if limits.Known && (limits.InheritMemoryMB > 0 || limits.InheritTimeS > 0 || limits.InheritUploadMB > 0 || limits.InheritInputVars > 0) {
		values := server()
		forced := func(ownValue, ceiling int, serverValue string, builtin int, size bool) int {
			if ownValue > 0 || ceiling <= 0 {
				return 0
			}
			current := builtin
			if serverValue != "" {
				parsed, ok := PHPININumber(serverValue, size)
				if !ok {
					return ceiling // unreadable: the plan's limit holds
				}
				current = parsed
			}
			if current < 0 || current == 0 && !size || current > ceiling {
				return ceiling
			}
			return 0
		}
		memory := forced(own.MemoryMB, limits.InheritMemoryMB, values.MemoryLimit, 128, true)
		runTime := forced(own.TimeS, limits.InheritTimeS, values.MaxExecutionTime, 30, false)
		upload := forced(own.UploadMB, limits.InheritUploadMB, values.UploadMaxFilesize, 2, true)
		inputs := forced(own.InputVars, limits.InheritInputVars, values.MaxInputVars, 1000, false)
		if lines := sitePHPLimitLines(memory, runTime, upload, inputs); lines != "" {
			b.WriteString(sitePHPPlanComment)
			b.WriteString(lines)
		}
	}
	return b.String()
}

// PHPININumber reads a php.ini number: a size in MB (K, M, G suffixes;
// -1 is unlimited) or a plain count. ok is false for anything else.
func PHPININumber(value string, size bool) (int, bool) {
	value = strings.Trim(strings.TrimSpace(value), `"'`)
	if value == "-1" {
		return -1, true
	}
	if !size {
		n, err := strconv.Atoi(value)
		return n, err == nil
	}
	if value == "" {
		return 0, false
	}
	unit := strings.ToUpper(value[len(value)-1:])
	digits := value
	scale := 0.0
	switch unit {
	case "K":
		digits, scale = value[:len(value)-1], 1.0/1024
	case "M":
		digits, scale = value[:len(value)-1], 1
	case "G":
		digits, scale = value[:len(value)-1], 1024
	default:
		scale = 1.0 / (1024 * 1024)
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 {
		return 0, false
	}
	mb := int(float64(n) * scale)
	if mb == 0 && n > 0 {
		mb = 1
	}
	return mb, true
}

// ReadPHPServerValues reads the keys the site screen shows from a PHP
// version's php.ini (the last line of each wins, as in PHP).
func ReadPHPServerValues(version string) (transport.SitePHPServerValues, error) {
	var values transport.SitePHPServerValues
	if err := ValidatePHPVersion(version); err != nil {
		return values, err
	}
	content, err := readManagedConfig(phpINIPath(version))
	if err != nil {
		return values, fmt.Errorf("read php.ini: %w", err)
	}
	return phpServerValuesIn(content), nil
}

func phpServerValuesIn(content []byte) transport.SitePHPServerValues {
	var values transport.SitePHPServerValues
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
			continue
		}
		key, value, ok := configPair(line)
		if !ok {
			continue
		}
		value = strings.Trim(trimInlineComment(value, ';'), `"`)
		if len(value) > 120 {
			value = value[:120]
		}
		switch key {
		case "memory_limit":
			values.MemoryLimit = value
		case "max_execution_time":
			values.MaxExecutionTime = value
		case "upload_max_filesize":
			values.UploadMaxFilesize = value
		case "post_max_size":
			values.PostMaxSize = value
		case "max_input_vars":
			values.MaxInputVars = value
		case "display_errors":
			values.DisplayErrors = value
		case "error_reporting":
			values.ErrorReporting = value
		case "date.timezone":
			values.Timezone = value
		case "short_open_tag":
			values.ShortOpenTag = value
		}
	}
	return values
}

// withinProcesses lowers the process count to the ceiling (0: none).
func (v poolValues) withinProcesses(limits transport.SitePHPLimits) poolValues {
	if limits.Known && limits.Processes > 0 && v.maxChildren > limits.Processes {
		v.maxChildren = limits.Processes
		v = v.clamped()
	}
	return v
}

// ReadSitePHPSettings answers one site for Agent.SitePHPSettingsV1: the pool
// file's state, its own values and process count, the version's php.ini and
// who sets the nginx upload size and read timeout.
func (pm *PHPPoolManager) ReadSitePHPSettings(site transport.SitePHPSettingsSite) transport.SitePHPSettingsState {
	state := transport.SitePHPSettingsState{SiteID: site.SiteID}
	request := transport.SitePoolRequest{SiteID: site.SiteID, Username: site.Username, PHPVersion: site.PHPVersion,
		Operation: transport.SitePoolOperationInspect}
	if err := validateSitePoolRequest(request); err != nil {
		state.Error = err.Error()
		return state
	}
	managedConfigMutationMu.Lock()
	file := sitePoolFile(request, site.PHPVersion, nil, nil, transport.SiteFileTriggerChange)
	inspected := file.inspect()
	managedConfigMutationMu.Unlock()
	state.PoolState = inspected.state
	content := inspected.content
	state.Own = sitePHPSettingsIn(content)
	if values, found := poolValuesIn(content); found {
		state.Processes = values.maxChildren
	} else {
		state.Processes = defaultPoolValues().maxChildren
	}
	if server, err := ReadPHPServerValues(site.PHPVersion); err == nil {
		state.Server = server
	} else {
		state.Error = boundedSiteFileDetail(err.Error())
	}
	if site.Domain != "" {
		owners := siteSettingsDirectiveOwners(site.Domain)
		state.NginxUploadOwner, state.NginxUploadMB = owners.uploadFile, owners.uploadMB
		state.NginxTimeoutOwner, state.NginxTimeoutS = owners.timeoutFile, owners.timeoutS
		state.SettingsFileState = SiteSettingsIncludeState(site.Domain)
	}
	return state
}
