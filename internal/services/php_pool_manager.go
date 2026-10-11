package services

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

var phpEtcDir = "/etc/php"

// The pool directory is the host's own (php_layout.go): Debian and Sury keep
// one per PHP version, Arch one for its single PHP-FPM.
func poolFilePath(phpVersion, poolName string) string {
	return filepath.Join(phpLayoutFor(phpVersion).poolDir(), poolName+".conf")
}

func poolDirPath(phpVersion string) string {
	return phpLayoutFor(phpVersion).poolDir()
}

type PHPPoolManager struct{}

func NewPHPPoolManager() *PHPPoolManager { return &PHPPoolManager{} }

var (
	validPoolName   = regexp.MustCompile(`^site[0-9]+$`)
	validListenMode = regexp.MustCompile(`^0[0-7]{3}$`)
	validPMModes    = map[string]bool{"dynamic": true, "ondemand": true, "static": true}
)

func parsePoolInteger(key, value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid %s value %q", key, value)
	}
	return n, nil
}

func (pm *PHPPoolManager) GetPoolConfig(phpVersion, poolName string) (*core.PHPPoolConfig, error) {
	if err := ValidatePHPVersion(phpVersion); err != nil {
		return nil, err
	}
	if !validPoolName.MatchString(poolName) {
		return nil, fmt.Errorf("invalid pool name %q", poolName)
	}
	content, err := readManagedConfig(poolFilePath(phpVersion, poolName))
	if err != nil {
		return nil, fmt.Errorf("read pool file: %w", err)
	}
	config := &core.PHPPoolConfig{Name: poolName, PM: "dynamic", PMMaxChildren: 5, ListenMode: "0660"}
	sectionSeen := false
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if sectionSeen || strings.TrimSuffix(strings.TrimPrefix(line, "["), "]") != poolName {
				return nil, fmt.Errorf("pool file contains an unexpected section %q", line)
			}
			sectionSeen = true
			continue
		}
		key, value, ok := configPair(line)
		if !ok {
			return nil, fmt.Errorf("malformed pool directive %q", line)
		}
		switch key {
		case "user":
			config.User = value
		case "group":
			config.Group = value
		case "listen":
			config.Listen = value
		case "listen.owner":
			config.ListenOwner = value
		case "listen.group":
			config.ListenGroup = value
		case "listen.mode":
			config.ListenMode = value
		case "pm":
			config.PM = value
		case "pm.max_children":
			config.PMMaxChildren, err = parsePoolInteger(key, value)
		case "pm.start_servers":
			config.PMStartServers, err = parsePoolInteger(key, value)
		case "pm.min_spare_servers":
			config.PMMinSpareServers, err = parsePoolInteger(key, value)
		case "pm.max_spare_servers":
			config.PMMaxSpareServers, err = parsePoolInteger(key, value)
		case "pm.max_requests":
			config.PMMaxRequests, err = parsePoolInteger(key, value)
		}
		if err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan pool file: %w", err)
	}
	if !sectionSeen {
		return nil, fmt.Errorf("pool section %q is missing", poolName)
	}
	for key, value := range map[string]string{"user": config.User, "group": config.Group, "listen.owner": config.ListenOwner, "listen.group": config.ListenGroup} {
		if err := validatePoolIdentity(key, value); err != nil {
			return nil, err
		}
	}
	expectedListen := PHPFPMSocketPath(phpVersion, poolName)
	if config.Listen != expectedListen {
		return nil, fmt.Errorf("pool listen path %q does not match managed socket %q", config.Listen, expectedListen)
	}
	if !validListenMode.MatchString(config.ListenMode) {
		return nil, fmt.Errorf("invalid pool listen mode %q", config.ListenMode)
	}
	if !validPMModes[config.PM] || config.PMMaxChildren < 1 {
		return nil, fmt.Errorf("invalid process-manager configuration")
	}
	return config, nil
}

func clamp(v, min, max, def int) int {
	if v == 0 {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// UpdatePoolConfig saves a pool's process-manager values (the PHP page's
// pool form, addressed by pool name). Since D-031's second step it is the
// site pool save: the file is classified first and a file that is not
// CelikPanel's unchanged text is kept (SiteFileHeldError), never rewritten.
func (pm *PHPPoolManager) UpdatePoolConfig(phpVersion string, config *core.PHPPoolConfig) error {
	if err := ValidatePHPVersion(phpVersion); err != nil {
		return err
	}
	if config == nil || !validPoolName.MatchString(config.Name) {
		return fmt.Errorf("invalid pool name")
	}
	if config.PM != "" && !validPMModes[config.PM] {
		return fmt.Errorf("invalid process-manager mode %q", config.PM)
	}
	managedConfigMutationMu.Lock()
	defer managedConfigMutationMu.Unlock()

	current, err := pm.GetPoolConfig(phpVersion, config.Name)
	if err != nil {
		return fmt.Errorf("pool %s does not exist for PHP %s: %w", config.Name, phpVersion, err)
	}
	siteID, _ := strconv.Atoi(strings.TrimPrefix(config.Name, "site"))
	return siteFileHeld(applySitePoolLocked(transport.SitePoolRequest{
		SiteID: siteID, Username: current.User, PHPVersion: phpVersion,
		Operation: transport.SitePoolOperationSave,
		Values: &transport.SitePoolValues{
			PM: config.PM, PMMaxChildren: config.PMMaxChildren, PMStartServers: config.PMStartServers,
			PMMinSpareServers: config.PMMinSpareServers, PMMaxSpareServers: config.PMMaxSpareServers,
			PMMaxRequests: config.PMMaxRequests,
		},
	}))
}

func renderPool(name, user, group, listen, listenOwner, listenGroup, listenMode, pmMode string,
	maxChildren, startServers, minSpare, maxSpare, maxRequests int) string {
	return fmt.Sprintf(`[%s]
user = %s
group = %s
listen = %s
listen.owner = %s
listen.group = %s
listen.mode = %s
pm = %s
pm.max_children = %d
pm.start_servers = %d
pm.min_spare_servers = %d
pm.max_spare_servers = %d
pm.max_requests = %d
chdir = /
`, name, user, group, listen, listenOwner, listenGroup, listenMode,
		pmMode, maxChildren, startServers, minSpare, maxSpare, maxRequests)
}

// DeletePoolByName removes a pool (the site is deleted). A file that is not
// CelikPanel's unchanged text is copied beside it first (a dated copy no
// include pattern reads); CelikPanel's pending text is removed with it.
func (pm *PHPPoolManager) DeletePoolByName(phpVersion, poolName string) error {
	if err := ValidatePHPVersion(phpVersion); err != nil {
		return err
	}
	if !validPoolName.MatchString(poolName) {
		return fmt.Errorf("invalid pool name %q", poolName)
	}
	managedConfigMutationMu.Lock()
	defer managedConfigMutationMu.Unlock()
	path := poolFilePath(phpVersion, poolName)
	var legacy func() ([]LegacyVhostRender, error)
	if current, err := pm.GetPoolConfig(phpVersion, poolName); err == nil {
		if content, readErr := readManagedConfig(path); readErr == nil {
			legacy = legacyPoolRenders(phpVersion, poolName, current.User, content)
		}
	}
	if _, err := BackupUnlessManaged(path, ManagedHeaderPrefixINI, legacy); err != nil {
		return fmt.Errorf("keep a copy of the edited pool before removing it: %w", err)
	}
	err := deleteManagedConfigLocked(path, 0o644,
		func() error { return phpFPMConfigTest(phpVersion) },
		func() error { return reloadPHPFPM(phpVersion) })
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err == nil {
		removeStalePending(path)
	}
	return err
}

func (pm *PHPPoolManager) ListPoolNames(phpVersion string) ([]string, error) {
	if err := ValidatePHPVersion(phpVersion); err != nil {
		return nil, err
	}
	files, err := os.ReadDir(poolDirPath(phpVersion))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".conf") {
			continue
		}
		name := strings.TrimSuffix(file.Name(), ".conf")
		if validPoolName.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// MigratePool moves a pool to another PHP version (addressed by pool name).
// Since D-031's second step it is the site pool switch: a source file that is
// not CelikPanel's unchanged text is kept (SiteFileHeldError) and nothing is
// written.
func (pm *PHPPoolManager) MigratePool(oldVersion, newVersion, poolName string) error {
	if err := ValidatePHPVersion(oldVersion); err != nil {
		return err
	}
	if err := ValidatePHPVersion(newVersion); err != nil {
		return err
	}
	if oldVersion == newVersion {
		return fmt.Errorf("source and target PHP versions are identical")
	}
	if !validPoolName.MatchString(poolName) {
		return fmt.Errorf("invalid pool name %q", poolName)
	}
	managedConfigMutationMu.Lock()
	defer managedConfigMutationMu.Unlock()

	oldConfig, err := pm.GetPoolConfig(oldVersion, poolName)
	if err != nil {
		return fmt.Errorf("get pool config from PHP %s: %w", oldVersion, err)
	}
	siteID, _ := strconv.Atoi(strings.TrimPrefix(poolName, "site"))
	return siteFileHeld(applySitePoolLocked(transport.SitePoolRequest{
		SiteID: siteID, Username: oldConfig.User, PHPVersion: oldVersion,
		NewPHPVersion: newVersion, Operation: transport.SitePoolOperationSwitch,
	}))
}
