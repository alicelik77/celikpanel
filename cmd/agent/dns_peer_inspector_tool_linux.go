//go:build linux

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeerenrollowner"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The DNS check tool on a backup DNS server (6 Nov 2026, alpha.125 incident).
//
// The paired main server proves a deletion with a read-only program the
// backup runs for it (dnspeerenrollowner: /usr/local/libexec/celikpanel-
// <engine>-peer-inspect, behind a forced SSH command and a sudoers rule).
// Updates never replace that file: it is native configuration that keeps
// working without the panel. A backup set up with an earlier release keeps
// that release's program, and an inspector fix (alpha.126: named's
// duplicate rndc sockets) never reached it, so deletions stayed pending.
//
// It is CelikPanel's own helper. When the Agent starts (after the owner's
// update, a rollback or a reboot; never from polling) it puts the running
// release's copy in place, once, only when the installed file's SHA-256 is
// a program CelikPanel shipped (listed below or present in a verified
// release tree on this server) or the file is missing. The replaced program
// is kept and the replacement recorded first. An owner's own file is never
// replaced: the status names the one command that would do it.
//
// Denetim araci: Agent acilista, yalniz CelikPanel'in dagittigi bir surumun
// ya da eksik dosyanin yerine bu surumun kopyasini koyar; sahibin kendi
// dosyasina dokunulmaz.

const (
	dnsPeerInspectorReleasesRoot = "/var/backups/celikpanel/releases"
	dnsPeerInspectorMaxBytes     = 64 << 20
	dnsPeerAgentMaxBytes         = 512 << 20
)

// dnsPeerInspectorShippedSHA256 lists the check tools of published releases
// (reproducible builds: the same hash on every server). A file with one of
// these hashes is CelikPanel's, not the owner's. Measured read-only on the
// owner's two servers on 6 Nov 2026 (/opt/celikpanel/dns-owner-tools, the
// installed inspector and every release tree kept under
// /var/backups/celikpanel/releases). Later releases are recognised from
// their own release trees as well.
var dnsPeerInspectorShippedSHA256 = map[string]map[string]bool{
	"bind": {
		"e991d003e965c791ff09d37aaa67ca3babbdc76ff3d64239968feef38d2ed957": true,
		"dc67c6366d566e157a2620c9119fc0599869168a86a7ecb33f2ec9a10bd06cb5": true,
		"2156321ace63a2de361a8cabc01e9b97b6854070c5e0b627d68f15859b632343": true,
		"f102eefc4e654a71d9c7ce4e886de7a34bc24c0638af539251e4cc67831662ea": true,
		"6a4a82476cd7e51def29cb94405ee0838461609c2161f10e1fc3b234b398188a": true,
		"347c9481baa829fafb84d97350497d7c5943f2feeb491f397ad8cdcf5b2740e0": true,
		"87732bf8e050d70348e0ccbc3528033d4d3a432ccd4b0e8d0c728a3f659c1193": true,
	},
	"pdns": {
		"3225b16c639e4feeef4ec2a86229f222a69e5ecc175fc321b16a4c7d73feeb0c": true,
		"e354f763c096634b574425e3dfe63d4c6e257fad6eaba0b4230affcbe602b76b": true,
		"d253504d6c6ed0e5af6c4cfa564918bd1c14aaebd3beb827984bdeaefb4d3da4": true,
		"eb41c2e7e033d1ba1e542fd68910283f9086644a3ff095831379adc9c5079daa": true,
		"3c05c91b2f820c7610b35b48ee11853f36aae5314211b1303839b9a774dcd4f0": true,
		"6075a62e453a4f9bc12b32df30e1ea4cdceb6648aed20ebefaf2747ddf120b09": true,
		"09fab8ccc97f64fafaae9bc717a9b86a0b5b1d4a5775a4bffb05aa0bd200d7df": true,
	},
}

var dnsPeerInspectorReleaseName = regexp.MustCompile(`^[0-9a-f]{12}-[0-9a-f]{8,64}$`)

// dnsPeerInspectorToolHost holds every path and effect; tests replace it.
type dnsPeerInspectorToolHost struct {
	releasesRoot string
	ownerTools   string // the fresh-install copy, /opt/celikpanel/dns-owner-tools
	selfExe      string // the running Agent's executable
	tools        []dnsPeerInspectorToolPaths
	shipped      map[string]map[string]bool
	// trusted reports a root-owned file or directory nobody else may write.
	trusted func(os.FileInfo) bool
	// chown makes the new file root:root (tests run unprivileged paths).
	chown func(*os.File) error
	// stateDir keeps the replaced programs and the replacement record.
	stateDir string
	// writeFile durably writes a private file (secureWriteConfig in
	// production).
	writeFile func(path string, raw []byte, mode os.FileMode) error
}

type dnsPeerInspectorToolPaths struct {
	engine    string // "bind" or "pdns"
	installed string // the program the forced command runs
	wrapper   string // present when this engine's channel is set up
	released  string // its name in a release's dns-owner-tools
}

func nativeDNSPeerInspectorToolHost() dnsPeerInspectorToolHost {
	return dnsPeerInspectorToolHost{
		releasesRoot: dnsPeerInspectorReleasesRoot,
		ownerTools:   dnsOwnerToolsDir,
		selfExe:      "/proc/self/exe",
		tools: []dnsPeerInspectorToolPaths{
			{engine: "bind", installed: dnspeerenrollowner.InspectorPath, wrapper: dnspeerenrollowner.WrapperPath, released: "bind-peer-inspect"},
			{engine: "pdns", installed: dnspeerenrollowner.PDNSInspectorPath, wrapper: dnspeerenrollowner.PDNSWrapperPath, released: "pdns-peer-inspect"},
		},
		shipped: dnsPeerInspectorShippedSHA256,
		trusted: func(info os.FileInfo) bool {
			stat, ok := info.Sys().(*syscall.Stat_t)
			return ok && stat.Uid == 0 && info.Mode().Perm()&0o022 == 0
		},
		chown:     func(file *os.File) error { return file.Chown(0, 0) },
		stateDir:  "/var/lib/celikpanel-agent-private/dns-check-tool",
		writeFile: secureWriteConfig,
	}
}

var (
	dnsPeerInspectorToolMu sync.Mutex
	// dnsPeerInspectorHashCache keeps digests of release files by identity
	// so a status read does not hash every release tree each time.
	dnsPeerInspectorHashCache   = map[string]dnsPeerInspectorHashEntry{}
	dnsPeerInspectorHashCacheMu sync.Mutex
)

type dnsPeerInspectorHashEntry struct {
	size, mtime int64
	ino         uint64
	digest      string
}

// trustedFileSHA256 hashes a regular, trusted file of at most limit bytes.
func (h dnsPeerInspectorToolHost) trustedFileSHA256(path string, limit int64, cache bool) (string, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() || !h.trusted(info) || info.Size() > limit {
		return "", info, errors.New("file is not a trusted regular file")
	}
	stat, _ := info.Sys().(*syscall.Stat_t)
	var key dnsPeerInspectorHashEntry
	if stat != nil {
		key = dnsPeerInspectorHashEntry{size: info.Size(), mtime: info.ModTime().UnixNano(), ino: stat.Ino}
	}
	if cache && stat != nil {
		dnsPeerInspectorHashCacheMu.Lock()
		cached, ok := dnsPeerInspectorHashCache[path]
		dnsPeerInspectorHashCacheMu.Unlock()
		if ok && cached.size == key.size && cached.mtime == key.mtime && cached.ino == key.ino {
			return cached.digest, info, nil
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return "", info, err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, io.LimitReader(file, limit+1)); err != nil {
		return "", info, err
	}
	digest := hex.EncodeToString(sum.Sum(nil))
	if cache && stat != nil {
		key.digest = digest
		dnsPeerInspectorHashCacheMu.Lock()
		dnsPeerInspectorHashCache[path] = key
		dnsPeerInspectorHashCacheMu.Unlock()
	}
	return digest, info, nil
}

// releaseTrees lists the verified release trees kept on this server.
func (h dnsPeerInspectorToolHost) releaseTrees() []string {
	root, err := os.Lstat(h.releasesRoot)
	if err != nil || !root.IsDir() || !h.trusted(root) {
		return nil
	}
	entries, err := os.ReadDir(h.releasesRoot)
	if err != nil {
		return nil
	}
	var trees []string
	for _, entry := range entries {
		if !dnsPeerInspectorReleaseName.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(h.releasesRoot, entry.Name())
		info, err := os.Lstat(path)
		if err == nil && info.IsDir() && h.trusted(info) {
			trees = append(trees, path)
		}
	}
	sort.Strings(trees)
	return trees
}

// currentRelease is the release tree whose Agent is the running Agent. The
// release a fresh installation was made from may not be kept as a tree;
// then the fresh-install copy of the owner tools is the current one only
// when no tree matches.
func (h dnsPeerInspectorToolHost) currentReleaseToolsDir() string {
	self, err := os.Stat(h.selfExe)
	if err == nil {
		var selfDigest string
		for _, tree := range h.releaseTrees() {
			agent := filepath.Join(tree, "bin", "agent")
			info, statErr := os.Lstat(agent)
			if statErr != nil || info.Size() != self.Size() {
				continue
			}
			if selfDigest == "" {
				if selfDigest, err = h.selfSHA256(); err != nil {
					break
				}
			}
			digest, _, hashErr := h.trustedFileSHA256(agent, dnsPeerAgentMaxBytes, true)
			if hashErr == nil && digest == selfDigest {
				return filepath.Join(tree, "dns-owner-tools")
			}
		}
	}
	return h.ownerTools
}

func (h dnsPeerInspectorToolHost) selfSHA256() (string, error) {
	file, err := os.Open(h.selfExe)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, io.LimitReader(file, dnsPeerAgentMaxBytes+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// shippedDigest reports whether digest is a check tool CelikPanel shipped.
func (h dnsPeerInspectorToolHost) shippedDigest(tool dnsPeerInspectorToolPaths, digest string) bool {
	if h.shipped[tool.engine][digest] {
		return true
	}
	dirs := []string{h.ownerTools}
	for _, tree := range h.releaseTrees() {
		dirs = append(dirs, filepath.Join(tree, "dns-owner-tools"))
	}
	for _, dir := range dirs {
		if released, _, err := h.trustedFileSHA256(filepath.Join(dir, tool.released), dnsPeerInspectorMaxBytes, true); err == nil && released == digest {
			return true
		}
	}
	return false
}

func dnsPeerInspectorToolCommand(source, target string) string {
	return "sudo install -o root -g root -m 0755 " + source + " " + target
}

// inspect is one engine's state. It only reads.
func (h dnsPeerInspectorToolHost) inspect(tool dnsPeerInspectorToolPaths) transport.DNSPeerInspectorTool {
	state := transport.DNSPeerInspectorTool{Engine: tool.engine, InstalledPath: tool.installed}
	if _, err := os.Lstat(tool.wrapper); err != nil {
		state.State = transport.DNSPeerInspectorToolNotEnrolled
		if !errors.Is(err, os.ErrNotExist) {
			state.State = transport.DNSPeerInspectorToolUnknown
		}
		return state
	}
	source := filepath.Join(h.currentReleaseToolsDir(), tool.released)
	sourceDigest, sourceInfo, err := h.trustedFileSHA256(source, dnsPeerInspectorMaxBytes, true)
	if err != nil || !h.elf(source) || sourceInfo.Mode().Perm()&0o100 == 0 {
		state.State = transport.DNSPeerInspectorToolReleaseCopyUnavailable
		return state
	}
	state.ReleasePath, state.ReleaseSHA256 = source, sourceDigest
	installed, err := os.Lstat(tool.installed)
	if errors.Is(err, os.ErrNotExist) {
		state.State = transport.DNSPeerInspectorToolMissing
		return state
	}
	if err != nil {
		state.State = transport.DNSPeerInspectorToolUnknown
		return state
	}
	if !installed.Mode().IsRegular() {
		state.State = transport.DNSPeerInspectorToolOwnerChanged
		state.Command = dnsPeerInspectorToolCommand(source, tool.installed)
		return state
	}
	digest, _, err := h.digestAny(tool.installed)
	if err != nil {
		state.State = transport.DNSPeerInspectorToolUnknown
		return state
	}
	state.InstalledSHA256 = digest
	switch {
	case digest == sourceDigest:
		state.State = transport.DNSPeerInspectorToolCurrent
	case h.shippedDigest(tool, digest):
		state.State = transport.DNSPeerInspectorToolOutdated
	default:
		state.State = transport.DNSPeerInspectorToolOwnerChanged
		state.Command = dnsPeerInspectorToolCommand(source, tool.installed)
	}
	return state
}

// digestAny hashes the installed file whatever its owner (an owner's file
// is only read, to tell it apart).
func (h dnsPeerInspectorToolHost) digestAny(path string) (string, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > dnsPeerInspectorMaxBytes {
		return "", info, errors.New("installed check tool is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", info, err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, io.LimitReader(file, dnsPeerInspectorMaxBytes+1)); err != nil {
		return "", info, err
	}
	return hex.EncodeToString(sum.Sum(nil)), info, nil
}

func (h dnsPeerInspectorToolHost) elf(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	head := make([]byte, 4)
	_, err = io.ReadFull(file, head)
	return err == nil && bytes.Equal(head, []byte{0x7f, 'E', 'L', 'F'})
}

func (h dnsPeerInspectorToolHost) status() transport.DNSPeerInspectorToolStatusResponse {
	response := transport.DNSPeerInspectorToolStatusResponse{Tools: []transport.DNSPeerInspectorTool{}}
	for _, tool := range h.tools {
		state := h.inspect(tool)
		if state.State != transport.DNSPeerInspectorToolNotEnrolled {
			response.Tools = append(response.Tools, state)
		}
	}
	return response
}

// dnsPeerInspectorToolRecordV1 is the durable evidence of every
// replacement: which engine, from and to which program, the release the new
// copy came from and where the replaced program is kept.
type dnsPeerInspectorToolRecordV1 struct {
	Schema  string                           `json:"schema"`
	Entries []dnsPeerInspectorToolRecordItem `json:"entries"`
}

type dnsPeerInspectorToolRecordItem struct {
	Engine       string `json:"engine"`
	FromSHA256   string `json:"from_sha256,omitempty"`
	ToSHA256     string `json:"to_sha256"`
	Release      string `json:"release"`
	PreviousCopy string `json:"previous_copy,omitempty"`
	At           string `json:"at"`
}

const (
	dnsPeerInspectorToolRecordSchema = "celikpanel-dns-check-tool-refresh/v1"
	dnsPeerInspectorToolRecordLimit  = 32
)

// reconcile runs once when the Agent starts (after the owner's update, a
// rollback or a reboot; never from polling). For each engine whose channel
// is set up it puts the running release's copy in place when the installed
// program is one CelikPanel shipped or is missing. The replaced program is
// kept, and the replacement recorded, before the new copy is published. A
// rollback starts the earlier release's Agent, which puts that release's
// copy back by the same rule. An owner's own file is kept and logged with
// the one command that would replace it.
func (h dnsPeerInspectorToolHost) reconcile(now time.Time) []transport.DNSPeerInspectorTool {
	var results []transport.DNSPeerInspectorTool
	for _, tool := range h.tools {
		before := h.inspect(tool)
		switch before.State {
		case transport.DNSPeerInspectorToolOutdated, transport.DNSPeerInspectorToolMissing:
		case transport.DNSPeerInspectorToolOwnerChanged:
			log.Printf("DNS check tool for %s at %s was changed on this server (it is no program CelikPanel shipped); it is kept. To use this release's copy instead: %s",
				tool.engine, tool.installed, before.Command)
			results = append(results, before)
			continue
		default:
			if before.State != transport.DNSPeerInspectorToolNotEnrolled {
				results = append(results, before)
			}
			continue
		}
		item := dnsPeerInspectorToolRecordItem{Engine: tool.engine, FromSHA256: before.InstalledSHA256,
			ToSHA256: before.ReleaseSHA256, Release: filepath.Dir(before.ReleasePath), At: now.UTC().Format(time.RFC3339)}
		if before.State == transport.DNSPeerInspectorToolOutdated {
			copyPath, err := h.keepPrevious(tool, before)
			if err != nil {
				log.Printf("DNS check tool for %s: the installed program could not be kept before the update, so it was not replaced: %v", tool.engine, err)
				results = append(results, before)
				continue
			}
			item.PreviousCopy = copyPath
		}
		if err := h.appendRecord(item); err != nil {
			log.Printf("DNS check tool for %s: the replacement could not be recorded, so it was not made: %v", tool.engine, err)
			results = append(results, before)
			continue
		}
		if err := h.replace(tool, before); err != nil {
			log.Printf("DNS check tool for %s: not replaced (nothing was changed): %v", tool.engine, err)
			results = append(results, h.inspect(tool))
			continue
		}
		after := h.inspect(tool)
		log.Printf("DNS check tool for %s put in place from %s (%s -> %s); the replaced program is kept at %s",
			tool.engine, before.ReleasePath, shortDigest(before.InstalledSHA256), shortDigest(after.InstalledSHA256), item.PreviousCopy)
		results = append(results, after)
	}
	return results
}

// keepPrevious copies the installed program into the private state
// directory under its own digest (an existing identical copy is reused).
func (h dnsPeerInspectorToolHost) keepPrevious(tool dnsPeerInspectorToolPaths, before transport.DNSPeerInspectorTool) (string, error) {
	if err := os.MkdirAll(h.stateDir, 0o700); err != nil {
		return "", err
	}
	target := filepath.Join(h.stateDir, tool.engine+"-peer-inspect-"+before.InstalledSHA256)
	if digest, _, err := h.digestAny(target); err == nil && digest == before.InstalledSHA256 {
		return target, nil
	}
	source, err := os.Open(tool.installed)
	if err != nil {
		return "", err
	}
	defer source.Close()
	raw, err := io.ReadAll(io.LimitReader(source, dnsPeerInspectorMaxBytes+1))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != before.InstalledSHA256 {
		return "", errors.New("the installed program changed while it was kept")
	}
	if err := h.writeFile(target, raw, 0o700); err != nil {
		return "", err
	}
	return target, nil
}

func (h dnsPeerInspectorToolHost) appendRecord(item dnsPeerInspectorToolRecordItem) error {
	path := filepath.Join(h.stateDir, "dns-check-tool-refresh-v1.json")
	record := dnsPeerInspectorToolRecordV1{Schema: dnsPeerInspectorToolRecordSchema}
	if raw, err := os.ReadFile(path); err == nil {
		var existing dnsPeerInspectorToolRecordV1
		if json.Unmarshal(raw, &existing) == nil && existing.Schema == dnsPeerInspectorToolRecordSchema {
			record.Entries = existing.Entries
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	record.Entries = append(record.Entries, item)
	if len(record.Entries) > dnsPeerInspectorToolRecordLimit {
		record.Entries = record.Entries[len(record.Entries)-dnsPeerInspectorToolRecordLimit:]
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(h.stateDir, 0o700); err != nil {
		return err
	}
	return h.writeFile(path, raw, 0o600)
}

func shortDigest(digest string) string {
	if digest == "" {
		return "(missing)"
	}
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

// replace writes a private temporary file beside the target, checks it,
// and renames it over the target only if the target still is exactly what
// was inspected.
func (h dnsPeerInspectorToolHost) replace(tool dnsPeerInspectorToolPaths, before transport.DNSPeerInspectorTool) error {
	dir := filepath.Dir(tool.installed)
	dirInfo, err := os.Lstat(dir)
	if err != nil || !dirInfo.IsDir() || !h.trusted(dirInfo) {
		return errors.New("the check tool's directory is missing or not root's")
	}
	source, err := os.Open(before.ReleasePath)
	if err != nil {
		return err
	}
	defer source.Close()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temp := filepath.Join(dir, "."+filepath.Base(tool.installed)+".update-"+hex.EncodeToString(nonce[:]))
	out, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(temp)
		}
	}()
	sum := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, sum), io.LimitReader(source, dnsPeerInspectorMaxBytes+1))
	if copyErr == nil {
		copyErr = h.chown(out)
	}
	if copyErr == nil {
		copyErr = out.Chmod(0o755)
	}
	if copyErr == nil {
		copyErr = out.Sync()
	}
	if closeErr := out.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}
	if hex.EncodeToString(sum.Sum(nil)) != before.ReleaseSHA256 {
		return errors.New("the release copy changed while it was copied")
	}
	// The target must still be what was inspected (no owner edit since).
	if before.State == transport.DNSPeerInspectorToolMissing {
		if _, err := os.Lstat(tool.installed); !errors.Is(err, os.ErrNotExist) {
			return errors.New("a check tool appeared while the update ran")
		}
	} else if digest, _, err := h.digestAny(tool.installed); err != nil || digest != before.InstalledSHA256 {
		return errors.New("the installed check tool changed while the update ran")
	}
	if err := os.Rename(temp, tool.installed); err != nil {
		return err
	}
	keep = true
	if handle, err := os.Open(dir); err == nil {
		_ = handle.Sync()
		handle.Close()
	}
	return nil
}

// DNSPeerInspectorToolStatusV1 reads the check tool's state on this server.
func (a *Agent) DNSPeerInspectorToolStatusV1(_ *transport.DNSPeerInspectorToolStatusRequest, resp *transport.DNSPeerInspectorToolStatusResponse) error {
	if resp == nil {
		return errors.New("response is required")
	}
	*resp = dnsPeerInspectorToolHostFor().status()
	return nil
}

// reconcileDNSPeerInspectorToolsAtStartup is the single run per Agent
// start. It is bounded local file work and never fails the start.
func reconcileDNSPeerInspectorToolsAtStartup() {
	dnsPeerInspectorToolMu.Lock()
	defer dnsPeerInspectorToolMu.Unlock()
	dnsPeerInspectorToolHostFor().reconcile(time.Now())
}

// dnsPeerInspectorToolHostFor is replaced by tests.
var dnsPeerInspectorToolHostFor = nativeDNSPeerInspectorToolHost

// currentDNSOwnerTool is this release's copy of an owner tool: the one a new
// automatic setup installs (a fresh-install copy in /opt is never updated).
func currentDNSOwnerTool(name string) string {
	return filepath.Join(nativeDNSPeerInspectorToolHost().currentReleaseToolsDir(), name)
}
