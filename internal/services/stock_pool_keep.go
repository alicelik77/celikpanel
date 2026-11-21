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
// side doors step leaves it on (D-022); putting it back here removes the
// version from the record first, so this is never mistaken for the owner's
// undo.
//
// Bir PHP-FPM sürümü havuzsuz bırakılmaz: son havuz gitmeden önce dağıtımın
// CelikPanel'in kapattığı havuzu kendi adıyla geri konur.

// StockPoolOffSuffix is the name CelikPanel gives the stock pool it turned off.
const StockPoolOffSuffix = ".off-by-celikpanel"

// stockPoolsOffRecordPath is a variable so tests can point it at a
// temporary tree.
var stockPoolsOffRecordPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "stock-pools-off.json")

var stockPoolsOffMu sync.Mutex

type stockPoolsOffRecord struct {
	Version int      `json:"version"`
	Off     []string `json:"off"`
}

// SetStockPoolsOffRecordPathForTest points the record at a test file and
// returns the restore.
func SetStockPoolsOffRecordPathForTest(path string) func() {
	saved := stockPoolsOffRecordPath
	stockPoolsOffRecordPath = path
	return func() { stockPoolsOffRecordPath = saved }
}

func readStockPoolsOffLocked() (map[string]bool, error) {
	out := map[string]bool{}
	data, err := os.ReadFile(stockPoolsOffRecordPath)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if len(data) > 16*1024 {
		return out, fmt.Errorf("%s is larger than CelikPanel writes it", stockPoolsOffRecordPath)
	}
	var record stockPoolsOffRecord
	if err := json.Unmarshal(data, &record); err != nil || record.Version != 1 {
		return out, fmt.Errorf("%s is not CelikPanel's record", stockPoolsOffRecordPath)
	}
	for _, version := range record.Off {
		if ValidatePHPVersion(version) == nil {
			out[version] = true
		}
	}
	return out, nil
}

func writeStockPoolsOffLocked(off map[string]bool) error {
	versions := make([]string, 0, len(off))
	for version := range off {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	data, err := json.Marshal(stockPoolsOffRecord{Version: 1, Off: versions})
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
	return readStockPoolsOffLocked()
}

// SetStockPoolTurnedOff records (off) or forgets a version's stock pool.
func SetStockPoolTurnedOff(version string, off bool) error {
	if err := ValidatePHPVersion(version); err != nil {
		return err
	}
	stockPoolsOffMu.Lock()
	defer stockPoolsOffMu.Unlock()
	current, err := readStockPoolsOffLocked()
	if err != nil && off {
		return err
	}
	if current[version] == off {
		return nil
	}
	if off {
		current[version] = true
	} else {
		delete(current, version)
	}
	return writeStockPoolsOffLocked(current)
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

// KeepAPoolBeside is called before the pool file removing is removed. When it
// is the last pool file of its version and CelikPanel turned that version's
// stock pool off, the stock pool is put back under its own name first and the
// version leaves CelikPanel's record; undo turns it off again (for a caller
// whose removal was refused and put back). With another pool left, or no pool
// of CelikPanel's to put back, nothing changes and undo is nil: the caller's
// configuration test decides, as before.
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
	if err := os.Rename(off, stock); err != nil {
		return nil, fmt.Errorf("put the distribution's PHP %s pool back (%s) before its last other pool goes: %w", version, stock, err)
	}
	if ValidatePHPVersion(version) == nil {
		if err := SetStockPoolTurnedOff(version, false); err != nil {
			if backErr := os.Rename(stock, off); backErr != nil {
				return nil, fmt.Errorf("record that the PHP %s pool is back: %v; turning it off again also failed: %w", version, err, backErr)
			}
			return nil, fmt.Errorf("record that the PHP %s pool is back: %w; nothing was changed", version, err)
		}
	}
	undo := func() error {
		if err := os.Rename(stock, off); err != nil {
			return fmt.Errorf("turn the PHP %s pool off again: %w", version, err)
		}
		if ValidatePHPVersion(version) == nil {
			return SetStockPoolTurnedOff(version, true)
		}
		return nil
	}
	return undo, nil
}
