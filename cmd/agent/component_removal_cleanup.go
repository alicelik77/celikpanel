package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// What a mail component's removal takes with it (alpha.130; the owner's rule
// of 8 Nov 2026: "no leftovers, ever"). Purging Postfix, Dovecot or postfwd
// removes the package's own files; the files CelikPanel wrote beside them
// stayed: the hourly mailbox 90% warning timer kept firing after Dovecot was
// gone, CelikPanel's Dovecot drop-ins, postfwd's systemd drop-in and
// CelikPanel's Postfix maps (some with a relay password) stayed on disk.
//
// After the package removal succeeded, each such file is removed when it is
// still CelikPanel's by the same rule its writer uses:
//
//   - a file with a digest line is CelikPanel's while the digest matches;
//   - a file with CelikPanel's marker line is CelikPanel's while it starts
//     with it (the writer rewrites such a file and keeps one without it);
//   - a file CelikPanel rewrites whole every time (no marker: the TLS
//     drop-in, the SNI and login maps, the transport map) is CelikPanel's.
//
// A file the owner changed is kept and named. A unit is stopped and disabled
// before its file goes; systemd is reloaded once. CelikPanel's own state
// folders (the 90% markers, postfwd's saved counters, CelikPanel's postfwd
// lists) hold no user data and go whole. The mail itself (/var/vmail), the
// mailbox password file (/etc/dovecot/users) and the DKIM keys are user data
// and are never touched here (docs/LEFTOVERS-AUDIT.md).
//
// Bir posta bileşeni kaldırılınca CelikPanel'in onun yanına yazdığı ve hâlâ
// kendisine ait dosyalar da gider; sahibin değiştirdiği dosya kalır ve adı
// söylenir. Posta, parola dosyası ve DKIM anahtarları kullanıcı verisidir.

// ownedRemovalFile is one file CelikPanel wrote for a component.
type ownedRemovalFile struct {
	path string
	// owned says whether content is still CelikPanel's; nil means CelikPanel
	// rewrites the file whole every time, so it is CelikPanel's.
	owned func(content []byte) bool
	// unit is the systemd unit this file defines, stopped and disabled
	// before the file goes ("" for none).
	unit string
	// packages says the file is the package's own text (left to the
	// package manager, not reported as the owner's).
	packages func(content []byte) bool
}

// componentRemovalCleanup is what a removal of one component takes along.
type componentRemovalCleanup struct {
	files []ownedRemovalFile
	// dirs are CelikPanel's own folders without user data, removed whole.
	dirs []string
	// parents are removed when they are empty afterwards.
	parents []string
}

// postfixMapIndexSuffixes are the indexes postmap builds beside a map.
var postfixMapIndexSuffixes = []string{".db", ".lmdb", ".cdb"}

func startsWithLine(line string) func([]byte) bool {
	return func(content []byte) bool { return bytes.HasPrefix(content, []byte(line+"\n")) }
}

func digestLine(header string) func([]byte) bool {
	return func(content []byte) bool { return digestHeaderIntact(content, header) }
}

// postfixMap is one Postfix map of CelikPanel's with the indexes postmap
// built from it. An index goes only with its map (or when its map is gone).
func postfixMap(path string, owned func([]byte) bool) []ownedRemovalFile {
	files := []ownedRemovalFile{{path: path, owned: owned}}
	for _, suffix := range postfixMapIndexSuffixes {
		files = append(files, ownedRemovalFile{path: path + suffix, owned: indexOf(path, owned)})
	}
	return files
}

// indexOf: an index is CelikPanel's while its map is (or is gone).
func indexOf(mapPath string, owned func([]byte) bool) func([]byte) bool {
	return func([]byte) bool {
		content, err := os.ReadFile(mapPath)
		if errors.Is(err, fs.ErrNotExist) {
			return true
		}
		return err == nil && (owned == nil || owned(content))
	}
}

// Paths of the mail configuration lane's files (alpha.130, feat/a130-mail-
// config), named here by their paths so the removal takes them too.
var (
	removalPostfixRelayMap       = "/etc/postfix/celikpanel-relay-sasl"
	removalPostfixRelayHeader    = "# CelikPanel: the sign-in of the relay server for Postfix (root only). Turn the relay off in CelikPanel to remove this file."
	removalPostfixSubaddressing  = "/etc/postfix/celikpanel-subaddressing"
	removalPostfixSubaddressText = "# CelikPanel turned on Postfix subaddressing (recipient_delimiter = +). Turn it off in CelikPanel to remove the line and this file.\n"
	removalPostfixSenderMap      = "/etc/postfix/celikpanel_sender_access"
	removalPostfixSenderMarker   = "# Managed by CelikPanel: the server's sender allow and block list."
)

// removalCleanupFor is the cleanup of one component on a package family.
func removalCleanupFor(serviceID, family string) componentRemovalCleanup {
	switch serviceID {
	case "postfix":
		var files []ownedRemovalFile
		files = append(files, postfixMap(autoresponderTransportPath, nil)...)
		files = append(files, postfixMap(postfixDomainOptionsPath, startsWithLine(domainOptionsMarker))...)
		files = append(files, postfixMap(postfixLoginMapPath, nil)...)
		files = append(files, postfixMap(postfixSNIPath, nil)...)
		files = append(files, postfixMap(removalPostfixSenderMap, startsWithLine(removalPostfixSenderMarker))...)
		files = append(files, postfixMap(removalPostfixRelayMap, startsWithLine(removalPostfixRelayHeader))...)
		files = append(files, ownedRemovalFile{path: removalPostfixSubaddressing, owned: func(content []byte) bool {
			return string(content) == removalPostfixSubaddressText
		}})
		return componentRemovalCleanup{files: files}
	case "dovecot":
		files := []ownedRemovalFile{
			{path: quotaWarningTimerPath, owned: quotaWarningIntact, unit: "celikpanel-quota-warning.timer"},
			{path: quotaWarningServicePath, owned: quotaWarningIntact, unit: "celikpanel-quota-warning.service"},
			{path: quotaWarningScriptPath, owned: quotaWarningIntact},
			{path: dovecotQuotaDropInPath, owned: dovecotQuotaManagedIntact},
			{path: autoresponderDovecotConfPath, owned: digestLine(autoresponderDovecotHead)},
			{path: dovecotSubmissionConf, owned: nil},
			{path: dovecotTLSConf, owned: nil},
			// CelikPanel's mailbox password file (the panel writes it again
			// from its records when Dovecot is installed again).
			{path: dovecotUsersPath, owned: nil},
			{path: dovecotVirtualDropInPath, owned: func(content []byte) bool {
				return classifyDovecotVirtualDropIn(content, true, dovecotVirtualEarlierRoots(mailRootDir)) != dovecotVirtualDropInOwnerEdited
			}},
		}
		return componentRemovalCleanup{
			files:   files,
			dirs:    []string{quotaWarningStateDir},
			parents: []string{dovecotConfDirPath, filepath.Dir(dovecotConfDirPath)},
		}
	case "postfwd":
		layout, ok := outgoingLayoutFor(family)
		if !ok {
			return componentRemovalCleanup{}
		}
		cleanup := componentRemovalCleanup{
			files: []ownedRemovalFile{
				{path: layout.dropIn, owned: startsWithLine(outgoingDropInMarker)},
				{path: layout.config, owned: func(content []byte) bool {
					return classifyOutgoingConfig(content, true) == outgoingConfigPanel
				}, packages: func(content []byte) bool {
					return classifyOutgoingConfig(content, true) == outgoingConfigPackage
				}},
			},
			dirs:    []string{layout.listDir},
			parents: []string{filepath.Dir(layout.dropIn)},
		}
		if family == "apt" {
			cleanup.dirs = append(cleanup.dirs, aptPostfwdRateDir)
		}
		return cleanup
	}
	return componentRemovalCleanup{}
}

// removalCleanupOps is the host side (a variable for tests).
type removalCleanupOps struct {
	systemctl func(ctx context.Context, args ...string) error
}

// runRemovalCleanup removes what is still CelikPanel's and says what it
// kept. removed and kept name paths; notes are failures in words.
func runRemovalCleanup(ctx context.Context, cleanup componentRemovalCleanup, ops removalCleanupOps) (removed, kept, notes []string) {
	unitsChanged := false
	var units []string
	for _, file := range cleanup.files {
		info, err := os.Lstat(file.path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			notes = append(notes, file.path+" could not be read: "+err.Error())
			continue
		}
		if !info.Mode().IsRegular() {
			kept = append(kept, file.path)
			continue
		}
		content, err := os.ReadFile(file.path)
		if err != nil {
			notes = append(notes, file.path+" could not be read: "+err.Error())
			continue
		}
		if file.packages != nil && file.packages(content) {
			continue
		}
		if file.owned != nil && !file.owned(content) {
			kept = append(kept, file.path)
			continue
		}
		if file.unit != "" && ops.systemctl != nil {
			// A unit that is not loaded any more has nothing to stop.
			_ = ops.systemctl(ctx, "disable", "--now", file.unit)
			units = append(units, file.unit)
		}
		if err := os.Remove(file.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			notes = append(notes, file.path+" could not be removed: "+err.Error())
			continue
		}
		if strings.HasPrefix(file.path, "/etc/systemd/") || file.unit != "" {
			unitsChanged = true
		}
		removed = append(removed, file.path)
	}
	for _, dir := range cleanup.dirs {
		if !realDirectory(dir) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			notes = append(notes, dir+" could not be removed: "+err.Error())
			continue
		}
		removed = append(removed, dir)
	}
	for _, dir := range cleanup.parents {
		if removeEmptyDir(dir) {
			removed = append(removed, dir)
		}
	}
	if unitsChanged && ops.systemctl != nil {
		if err := ops.systemctl(ctx, "daemon-reload"); err != nil {
			notes = append(notes, "systemctl daemon-reload: "+err.Error())
		}
		if len(units) > 0 {
			_ = ops.systemctl(ctx, append([]string{"reset-failed"}, units...)...)
		}
	}
	sort.Strings(kept)
	return removed, kept, notes
}

// removalCleanupDetail is one sentence for the removal's answer.
func removalCleanupDetail(removed, kept, notes []string) string {
	var parts []string
	if len(removed) > 0 {
		parts = append(parts, "CelikPanel's own files for it were removed too ("+strings.Join(removed, ", ")+")")
	}
	if len(kept) > 0 {
		parts = append(parts, "kept because they were changed outside CelikPanel: "+strings.Join(kept, ", "))
	}
	parts = append(parts, notes...)
	return strings.Join(parts, "; ")
}

// hostRemovalCleanupOps runs systemctl on this host.
func hostRemovalCleanupOps() removalCleanupOps {
	return removalCleanupOps{systemctl: func(ctx context.Context, args ...string) error {
		systemctl, err := serviceMutationSystemctlResolver()
		if err != nil {
			return err
		}
		out, err := runServiceMutationCombinedOutput(ctx, systemctl, args...)
		if err != nil {
			return errors.New(commandFailureDetail("systemctl "+strings.Join(args, " "), out, err))
		}
		return nil
	}}
}
