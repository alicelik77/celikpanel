package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// A PHP-FPM version is never left without a pool (S4 R1 fix round, 21 Nov
// 2026). PHP-FPM refuses a configuration with no [pool] at all ("No pool
// defined"), and a reload into it leaves the version failed. The Agent's side
// doors step (cmd/agent/side_doors.go) turns the distribution's untouched,
// unused stock pool (www.conf) off by renaming it to
// www.conf.off-by-celikpanel while another pool remains. When the last of
// those other pools is about to go (a site deleted or moved to another
// version, a database tool or webmail moved or removed), KeepAPoolBeside puts
// the distribution's pool back under its own name first, so the version's own
// configuration test still passes; the caller runs that test before every
// reload and undoes on a refusal.
//
// The versions CelikPanel turned off are recorded in the Agent's private state
// (stock-pools-off.json). A stock pool back under its own name while the
// record still names its version was put back by the server owner, and the
// side doors step leaves it on (D-022). The record is always written BEFORE
// the file is renamed (S4 R1, second fix round), so a crash between the two
// never reads as the owner's doing: putting the pool back here forgets the
// version first, then renames (a crash in between leaves it off and not
// recorded, which the next start records as CelikPanel's); turning it off
// marks the version "turning off" first, then renames, then records it off
// (a crash in between leaves the mark, and the next start finishes or
// forgets CelikPanel's own unfinished step).
//
// Every keep, remove, configuration test and reload of a version's pools runs
// under one host-wide lock, LockPHPFPMPools (the site paths'
// managedConfigMutationMu), shared by the site paths, the database tools' and
// webmail's pools and the side doors step: two removals can never both see
// "another pool remains" and leave the version with none.
//
// Bir PHP-FPM sürümü havuzsuz bırakılmaz: son havuz gitmeden önce dağıtımın
// CelikPanel'in kapattığı havuzu kendi adıyla geri konur.

// StockPoolOffSuffix is the name CelikPanel gives the stock pool it turned off.
const StockPoolOffSuffix = ".off-by-celikpanel"

// stockPoolsOffRecordPath is a variable so tests can point it at a
// temporary tree.
var stockPoolsOffRecordPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "stock-pools-off.json")

var stockPoolsOffMu sync.Mutex

// stockPoolRename renames the stock pool; a variable so a test can see what
// the record says at the moment of the rename.
var stockPoolRename = os.Rename

type stockPoolsOffRecord struct {
	Version int      `json:"version"`
	Off     []string `json:"off"`
	// TurningOff: versions whose turn-off CelikPanel began and has not
	// finished (S4 R1, second fix round).
	TurningOff []string `json:"turning_off,omitempty"`
}

// LockPHPFPMPools takes the one host-wide lock of PHP-FPM pool files and
// returns its release. The site paths hold the same lock (they take
// managedConfigMutationMu directly), so a caller must not call them while
// holding it.
func LockPHPFPMPools() func() {
	managedConfigMutationMu.Lock()
	return managedConfigMutationMu.Unlock
}

// SetStockPoolsOffRecordPathForTest points the record at a test file and
// returns the restore.
func SetStockPoolsOffRecordPathForTest(path string) func() {
	saved := stockPoolsOffRecordPath
	stockPoolsOffRecordPath = path
	return func() { stockPoolsOffRecordPath = saved }
}

func readStockPoolsOffLocked() (map[string]bool, map[string]bool, error) {
	out, turning := map[string]bool{}, map[string]bool{}
	data, err := os.ReadFile(stockPoolsOffRecordPath)
	if errors.Is(err, fs.ErrNotExist) {
		return out, turning, nil
	}
	if err != nil {
		return out, turning, err
	}
	if len(data) > 16*1024 {
		return out, turning, fmt.Errorf("%s is larger than CelikPanel writes it", stockPoolsOffRecordPath)
	}
	var record stockPoolsOffRecord
	if err := json.Unmarshal(data, &record); err != nil || record.Version != 1 {
		return out, turning, fmt.Errorf("%s is not CelikPanel's record", stockPoolsOffRecordPath)
	}
	for _, version := range record.Off {
		if ValidatePHPVersion(version) == nil {
			out[version] = true
		}
	}
	for _, version := range record.TurningOff {
		if ValidatePHPVersion(version) == nil {
			turning[version] = true
		}
	}
	return out, turning, nil
}

func sortedVersions(set map[string]bool) []string {
	versions := make([]string, 0, len(set))
	for version, on := range set {
		if on {
			versions = append(versions, version)
		}
	}
	sort.Strings(versions)
	return versions
}

func writeStockPoolsOffLocked(off, turning map[string]bool) error {
	data, err := json.Marshal(stockPoolsOffRecord{Version: 1, Off: sortedVersions(off), TurningOff: sortedVersions(turning)})
	if err != nil {
		return err
	}
	dir := filepath.Dir(stockPoolsOffRecordPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".stock-pools-off-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(0o600)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, stockPoolsOffRecordPath)
}

// StockPoolsTurnedOff lists the versions whose stock pool CelikPanel turned
// off and has not put back.
func StockPoolsTurnedOff() (map[string]bool, error) {
	stockPoolsOffMu.Lock()
	defer stockPoolsOffMu.Unlock()
	off, _, err := readStockPoolsOffLocked()
	return off, err
}

// StockPoolsTurningOff lists the versions whose turn-off CelikPanel began
// and did not finish (a crash between the record and the rename).
func StockPoolsTurningOff() (map[string]bool, error) {
	stockPoolsOffMu.Lock()
	defer stockPoolsOffMu.Unlock()
	_, turning, err := readStockPoolsOffLocked()
	return turning, err
}

// SetStockPoolTurnedOff records (off) or forgets a version's stock pool; both
// end an unfinished turn-off of it. A record that cannot be read is an error
// either way: it is never rewritten from empty, which would forget the other
// versions and the owner's undo of them (S4 R1, fourth fix round).
func SetStockPoolTurnedOff(version string, off bool) error {
	if err := ValidatePHPVersion(version); err != nil {
		return err
	}
	stockPoolsOffMu.Lock()
	defer stockPoolsOffMu.Unlock()
	current, turning, err := readStockPoolsOffLocked()
	if err != nil {
		return err
	}
	if current[version] == off && !turning[version] {
		return nil
	}
	if off {
		current[version] = true
	} else {
		delete(current, version)
	}
	delete(turning, version)
	return writeStockPoolsOffLocked(current, turning)
}

// SetStockPoolTurningOff marks that CelikPanel is about to rename a
// version's stock pool away; it is written before the rename. Whether the
// version is recorded off is left as it is (the administrator's explicit
// "turn it off again" keeps the owner's undo recorded until it is done).
func SetStockPoolTurningOff(version string) error {
	if err := ValidatePHPVersion(version); err != nil {
		return err
	}
	stockPoolsOffMu.Lock()
	defer stockPoolsOffMu.Unlock()
	current, turning, err := readStockPoolsOffLocked()
	if err != nil {
		return err
	}
	if turning[version] {
		return nil
	}
	turning[version] = true
	return writeStockPoolsOffLocked(current, turning)
}

// ForgetStockPoolTurningOff ends an unfinished turn-off without changing
// whether the version is recorded off.
func ForgetStockPoolTurningOff(version string) error {
	if err := ValidatePHPVersion(version); err != nil {
		return err
	}
	stockPoolsOffMu.Lock()
	defer stockPoolsOffMu.Unlock()
	current, turning, err := readStockPoolsOffLocked()
	if err != nil {
		return err
	}
	if !turning[version] {
		return nil
	}
	delete(turning, version)
	return writeStockPoolsOffLocked(current, turning)
}

// poolDirVersion reads the version from /etc/php/<version>/fpm/pool.d.
func poolDirVersion(dir string) string {
	return filepath.Base(filepath.Dir(filepath.Dir(dir)))
}

// otherPoolFiles counts the pool files PHP-FPM reads in dir (*.conf, the
// distribution's include pattern) other than except.
func otherPoolFiles(dir, except string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if name == filepath.Base(except) || !strings.HasSuffix(name, ".conf") {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && info.Mode().IsRegular() {
			count++
		}
	}
	return count, nil
}

// KeepAPoolBeside is called before the pool file removing is removed, under
// LockPHPFPMPools (the caller holds it until its configuration test and
// reload are done). When it is the last pool file of its version and
// CelikPanel turned that version's stock pool off, the version leaves
// CelikPanel's record first and the stock pool is then put back under its own
// name; undo turns it off again the same way round (marked "turning off",
// renamed, recorded off), for a caller whose removal was refused and put
// back. With another pool left, or no pool of CelikPanel's to put back,
// nothing changes and undo is nil: the caller's configuration test decides,
// as before.
// Son havuz gitmeden önce CelikPanel'in kapattığı varsayılan havuz geri konur.
func KeepAPoolBeside(removing string) (func() error, error) {
	if _, err := os.Lstat(removing); err != nil {
		return nil, nil
	}
	dir := filepath.Dir(removing)
	others, err := otherPoolFiles(dir, removing)
	if err != nil {
		return nil, fmt.Errorf("read the pools of %s: %w", dir, err)
	}
	if others > 0 {
		return nil, nil
	}
	stock := filepath.Join(dir, "www.conf")
	off := stock + StockPoolOffSuffix
	if info, err := os.Lstat(off); err != nil || !info.Mode().IsRegular() {
		return nil, nil
	}
	if _, err := os.Lstat(stock); err == nil {
		return nil, fmt.Errorf("%s and %s are both there; the server administrator keeps one of them, then tries again", stock, off)
	}
	version := poolDirVersion(dir)
	recorded := ValidatePHPVersion(version) == nil
	if recorded {
		if err := SetStockPoolTurnedOff(version, false); err != nil {
			return nil, fmt.Errorf("record that the PHP %s pool is put back: %w; nothing was changed", version, err)
		}
	}
	if err := stockPoolRename(off, stock); err != nil {
		cause := fmt.Errorf("put the distribution's PHP %s pool back (%s) before its last other pool goes: %w", version, stock, err)
		if recorded {
			if recordErr := SetStockPoolTurnedOff(version, true); recordErr != nil {
				return nil, fmt.Errorf("%w; recording it as turned off again also failed: %v", cause, recordErr)
			}
		}
		return nil, cause
	}
	undo := func() error {
		if recorded {
			if err := SetStockPoolTurningOff(version); err != nil {
				return fmt.Errorf("turn the PHP %s pool off again: record it: %w", version, err)
			}
		}
		if err := stockPoolRename(stock, off); err != nil {
			if recorded {
				_ = ForgetStockPoolTurningOff(version)
			}
			return fmt.Errorf("turn the PHP %s pool off again: %w", version, err)
		}
		if recorded {
			return SetStockPoolTurnedOff(version, true)
		}
		return nil
	}
	return undo, nil
}

// SetPHPFPMForTest points the PHP-FPM configuration directory and the
// version's configuration test and reload at a test's own, and returns the
// restore (for cmd/agent tests that run a site path beside a tool path).
func SetPHPFPMForTest(etcDir string, test, reload func(version string) error) func() {
	savedDir, savedTest, savedReload := phpEtcDir, phpFPMConfigTest, reloadPHPFPM
	phpEtcDir, phpFPMConfigTest, reloadPHPFPM = etcDir, test, reload
	return func() { phpEtcDir, phpFPMConfigTest, reloadPHPFPM = savedDir, savedTest, savedReload }
}
