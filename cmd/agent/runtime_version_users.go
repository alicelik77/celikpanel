package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The second wall of the component dependency rule (alpha.123, D-045): a
// managed Node.js version is never deleted while a CelikPanel service unit
// still runs on it. Service apps (celiksvc-<id>, n8n on Node.js 24) and
// runtime apps (celikrt-<id>) carry the runtime's absolute path in their unit
// file or a drop-in; the Panel does not record which version an app chose,
// so the Agent reads the units themselves. Sites (celikapp-<id>) are the
// Panel's own check and the security emergency's maintenance lock, so they
// are not read here.
//
// Bağımlılık kuralının ikinci duvarı: bir CelikPanel hizmet birimi hâlâ
// üzerinde çalışırken yönetilen Node.js sürümü silinmez. Birimler okunur.

const errCodeRuntimeUnitsBound = "RUNTIME_UNITS_BOUND"

// runtimeUnitPrefixes are the unit families read.
var runtimeUnitPrefixes = []string{"celiksvc-", "celiksvcinst-", "celikrt-", "celikrtbuild-"}

// runtimeUnitReadCap bounds one unit file read; runtimeUnitFileCap bounds the
// number of files looked at.
const (
	runtimeUnitReadCap = 256 << 10
	runtimeUnitFileCap = 4096
)

var errRuntimeUnitsUnreadable = errors.New("the service units could not be read")

// runtimeVersionUnitUsers names the units whose unit file or drop-in refers
// to dir (the version's directory, matched as a whole path element).
func runtimeVersionUnitUsers(dir string) ([]string, error) {
	base, err := unitDir()
	if err != nil {
		return nil, errRuntimeUnitsUnreadable
	}
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, errRuntimeUnitsUnreadable
	}
	users := map[string]bool{}
	read := 0
	for _, entry := range entries {
		name := entry.Name()
		unit, isDropIn := strings.CutSuffix(name, ".service.d")
		if !isDropIn {
			var ok bool
			if unit, ok = strings.CutSuffix(name, ".service"); !ok {
				continue
			}
		}
		if !hasRuntimeUnitPrefix(unit) {
			continue
		}
		files := []string{filepath.Join(base, name)}
		if isDropIn {
			if !entry.IsDir() {
				continue
			}
			parts, err := os.ReadDir(filepath.Join(base, name))
			if err != nil {
				return nil, errRuntimeUnitsUnreadable
			}
			files = files[:0]
			for _, part := range parts {
				if strings.HasSuffix(part.Name(), ".conf") && part.Type().IsRegular() {
					files = append(files, filepath.Join(base, name, part.Name()))
				}
			}
		} else if !entry.Type().IsRegular() {
			continue
		}
		for _, file := range files {
			read++
			if read > runtimeUnitFileCap {
				return nil, errRuntimeUnitsUnreadable
			}
			refers, err := unitFileRefersTo(file, dir)
			if err != nil {
				return nil, errRuntimeUnitsUnreadable
			}
			if refers {
				users[unit] = true
			}
		}
	}
	out := make([]string, 0, len(users))
	for unit := range users {
		out = append(out, unit)
	}
	sort.Strings(out)
	return out, nil
}

func hasRuntimeUnitPrefix(unit string) bool {
	for _, prefix := range runtimeUnitPrefixes {
		if strings.HasPrefix(unit, prefix) {
			return true
		}
	}
	return false
}

// unitFileRefersTo: the file names dir followed by "/" or a separator that
// ends a path (space, quote, colon, end of line), so node/24.1.0 never
// matches node/24.1.01.
func unitFileRefersTo(path, dir string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, runtimeUnitReadCap))
	if err != nil {
		return false, err
	}
	text := string(data)
	for {
		index := strings.Index(text, dir)
		if index < 0 {
			return false, nil
		}
		rest := text[index+len(dir):]
		if rest == "" || strings.ContainsRune("/ \t\"':\n\r;", rune(rest[0])) {
			return true, nil
		}
		text = rest
	}
}

// RuntimeVersionUsersV1 answers read-only which units run on one managed
// Node.js version (the removal dialog's evidence).
func (a *Agent) RuntimeVersionUsersV1(req *transport.RuntimeVersionUsersRequest, resp *transport.RuntimeVersionUsersResponse) error {
	*resp = transport.RuntimeVersionUsersResponse{Units: []string{}}
	if req == nil || req.Runtime != "node" || !nodeVersionRe.MatchString(req.Version) {
		resp.Error = "not a managed runtime version"
		return nil
	}
	units, err := runtimeVersionUnitUsers(filepath.Join(runtimesBaseDir, "node", req.Version))
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Units = units
	return nil
}

// nodeVersionUnitsRefusal is RemoveNodeVersion's wall: a non-empty answer
// refuses the removal and names the units; an unreadable unit directory
// refuses too (the removal cannot prove nothing runs on it).
func nodeVersionUnitsRefusal(version string, resp *transport.NodeRemoveResponse) bool {
	units, err := runtimeVersionUnitUsers(filepath.Join(runtimesBaseDir, "node", version))
	if err != nil {
		resp.Error = fmt.Sprintf("Node.js %s was not removed: %v, so it cannot be shown that no service runs on it", version, err)
		return true
	}
	if len(units) > 0 {
		resp.ErrorCode = errCodeRuntimeUnitsBound
		resp.Units = units
		resp.Error = fmt.Sprintf("Node.js %s was not removed: %s still run on it; remove those apps or move them to another version first", version, strings.Join(units, ", "))
		return true
	}
	return false
}
