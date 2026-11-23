package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// What giving a tool's files to its own account changes, and how that is put
// back exactly (S4 R1 fix round, 21 Nov 2026; second fix round, 22 Nov 2026).
//
// The owner, group, mode (with its set-user-id, set-group-id and sticky
// bits) and, for Roundcube's tree root, the access-control list every
// changed path had before are written to the Agent's private state
// (php-tool-files-<tool>.json, root 0600) BEFORE the change, together with
// what CelikPanel sets. A move cut by a power loss after the files were
// given to the account and before the web server was switched leaves webmail
// unreadable for the stock pool; the next start's retry then puts back what
// the FIRST attempt recorded, not what it finds. A path is put back only
// while it is still exactly as CelikPanel set it (or already as it was); a
// file the move itself created is removed while it holds exactly what the
// move wrote; one that is gone has nothing to put back; one that changed
// since (a Roundcube update, the server owner's own edit) is kept as it is
// and named as changed outside the move. Whether a move's
// failure puts the files back is decided from what serves the tool now (the
// socket its web server configuration names), never from a file's group: a
// tool already served by its own pool keeps its files with the account (a
// move between PHP versions), and that path and an update's staging tree
// keep no record.
//
// The record is removed when the tool answers on its own pool (the move is
// proven) or when the undo has put back or kept everything.
//
// Aracın dosyalarını kendi hesabına verirken her yolun önceki sahibi, grubu,
// kipi (özel bitleriyle) ve kök dizinin erişim listesi, CelikPanel'in
// koyacağıyla birlikte değişiklikten ÖNCE kaydedilir. Geri alma yalnız
// CelikPanel'in koyduğu gibi duran yolu geri koyar; o arada değişeni korur ve
// söyler.

var phpToolFilesRecordDir = filepath.Dir(phpToolAccountRecordPath)

func phpToolFilesRecordPath(tool string) string {
	return filepath.Join(phpToolFilesRecordDir, "php-tool-files-"+tool+".json")
}

// phpToolFileMeta is what CelikPanel sets on a path.
type phpToolFileMeta struct {
	UID int `json:"uid"`
	GID int `json:"gid"`
	// Mode: permission bits with set-user-id, set-group-id and sticky
	// (0o7777).
	Mode uint32 `json:"mode"`
	// ACLSet: this change sets the access-control list to ACL.
	ACLSet bool   `json:"acl_set,omitempty"`
	ACL    []byte `json:"acl,omitempty"`
	// ContentSHA: the digest of the bytes CelikPanel writes ("" when it
	// writes none).
	ContentSHA string `json:"content_sha,omitempty"`
}

// phpToolFileState is one path as it was before CelikPanel changed it, and
// what CelikPanel set (After).
type phpToolFileState struct {
	Path    string `json:"path"`
	Existed bool   `json:"existed"`
	Dir     bool   `json:"dir,omitempty"`
	UID     int    `json:"uid"`
	GID     int    `json:"gid"`
	Mode    uint32 `json:"mode"`
	// Content is kept only for a file CelikPanel rewrites (phpMyAdmin's
	// conf.d file); HasContent says it was read.
	Content    []byte `json:"content,omitempty"`
	HasContent bool   `json:"has_content,omitempty"`
	// ACL: the access-control list before (nil: none), read when the change
	// sets one.
	ACL     []byte           `json:"acl,omitempty"`
	ACLRead bool             `json:"acl_read,omitempty"`
	After   *phpToolFileMeta `json:"after,omitempty"`
}

// phpToolPlanned is one change about to be made: the path and what it gets.
type phpToolPlanned struct {
	Path        string
	WithContent bool
	After       phpToolFileMeta
}

type phpToolFilesRecord struct {
	Version int                `json:"version"`
	Tool    string             `json:"tool"`
	Before  []phpToolFileState `json:"before"`
}

// phpToolFileJournal is the record of one tool, loaded.
type phpToolFileJournal struct {
	tool   string
	before []phpToolFileState
	seen   map[string]int
}

func loadPHPToolFileJournal(tool string) (*phpToolFileJournal, error) {
	journal := &phpToolFileJournal{tool: tool, seen: map[string]int{}}
	data, err := os.ReadFile(phpToolFilesRecordPath(tool))
	if errors.Is(err, fs.ErrNotExist) {
		return journal, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read CelikPanel's record of %s's files: %w", phpToolDisplayName(tool), err)
	}
	var record phpToolFilesRecord
	if json.Unmarshal(data, &record) != nil || record.Version != 1 || record.Tool != tool {
		return nil, fmt.Errorf("%s is not CelikPanel's record of %s's files; the server administrator moves it away, then tries again", phpToolFilesRecordPath(tool), phpToolDisplayName(tool))
	}
	for _, state := range record.Before {
		if _, seen := journal.seen[state.Path]; !seen {
			journal.seen[state.Path] = len(journal.before)
			journal.before = append(journal.before, state)
		}
	}
	return journal, nil
}

func (j *phpToolFileJournal) save() error {
	if err := os.MkdirAll(phpToolFilesRecordDir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(phpToolFilesRecord{Version: 1, Tool: j.tool, Before: j.before})
	if err != nil {
		return err
	}
	return writePrivateFileAtomic(phpToolFilesRecordPath(j.tool), data, ".php-tool-files-*")
}

// note records how each path is now, unless an earlier attempt recorded it
// already (that earlier state is the one to come back), and what this
// attempt sets on it. It is saved before the caller changes anything.
func (j *phpToolFileJournal) note(changes []phpToolPlanned) error {
	if len(changes) == 0 {
		return nil
	}
	for _, change := range changes {
		after := change.After
		if index, seen := j.seen[change.Path]; seen {
			j.before[index].After = &after
			continue
		}
		state, err := readPHPToolFileState(change.Path, change.WithContent, change.After.ACLSet)
		if err != nil {
			return err
		}
		state.After = &after
		j.seen[change.Path] = len(j.before)
		j.before = append(j.before, state)
	}
	return j.save()
}

// unixModeBits are a mode's permission bits with set-user-id, set-group-id
// and sticky, as the system stores them.
func unixModeBits(mode os.FileMode) uint32 {
	bits := uint32(mode.Perm())
	if mode&os.ModeSetuid != 0 {
		bits |= 0o4000
	}
	if mode&os.ModeSetgid != 0 {
		bits |= 0o2000
	}
	if mode&os.ModeSticky != 0 {
		bits |= 0o1000
	}
	return bits
}

// fileModeOfBits is unixModeBits the other way round.
func fileModeOfBits(bits uint32) os.FileMode {
	mode := os.FileMode(bits & 0o777)
	if bits&0o4000 != 0 {
		mode |= os.ModeSetuid
	}
	if bits&0o2000 != 0 {
		mode |= os.ModeSetgid
	}
	if bits&0o1000 != 0 {
		mode |= os.ModeSticky
	}
	return mode
}

// keptModeBits: a mode's permission and special bits, the rest dropped.
func keptModeBits(mode os.FileMode) os.FileMode {
	return mode & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
}

func contentDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func readPHPToolFileState(path string, withContent, withACL bool) (phpToolFileState, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return phpToolFileState{Path: path, ACLRead: withACL}, nil
	}
	if err != nil {
		return phpToolFileState{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return phpToolFileState{}, fmt.Errorf("the owner of %s could not be read", path)
	}
	state := phpToolFileState{Path: path, Existed: true, Dir: info.IsDir(), UID: int(stat.Uid), GID: int(stat.Gid), Mode: unixModeBits(info.Mode())}
	if withContent && info.Mode().IsRegular() {
		content, err := os.ReadFile(path)
		if err != nil {
			return phpToolFileState{}, err
		}
		state.Content, state.HasContent = content, true
	}
	if withACL && info.Mode()&os.ModeSymlink == 0 {
		acl, err := readPathACL(path)
		if err != nil {
			return phpToolFileState{}, err
		}
		state.ACL, state.ACLRead = acl, true
	}
	return state, nil
}

// currentPHPToolFileState reads a path as it is now; a variable so
// unprivileged tests can stand in for a change they did not make.
var currentPHPToolFileState = readPHPToolFileState

// asSet: the path is exactly as CelikPanel set it.
func (s phpToolFileState) asSet(current phpToolFileState) bool {
	after := s.After
	if after == nil || !current.Existed || current.UID != after.UID || current.GID != after.GID || current.Mode != after.Mode {
		return false
	}
	if after.ContentSHA != "" && (!current.HasContent || contentDigest(current.Content) != after.ContentSHA) {
		return false
	}
	return !after.ACLSet || sameACL(current.ACL, after.ACL)
}

// asBefore: the path is as it was before (nothing of CelikPanel's on it).
func (s phpToolFileState) asBefore(current phpToolFileState) bool {
	if current.Existed != s.Existed {
		return false
	}
	if !s.Existed {
		return true
	}
	if current.UID != s.UID || current.GID != s.GID || current.Mode != s.Mode {
		return false
	}
	if s.HasContent && string(current.Content) != string(s.Content) {
		return false
	}
	return !s.ACLRead || sameACL(current.ACL, s.ACL)
}

// errChangedOutsideTheMove names a path the undo kept because it changed
// after CelikPanel changed it.
type errChangedOutsideTheMove struct{ paths []string }

func (e *errChangedOutsideTheMove) Error() string {
	return strings.Join(e.paths, ", ") + " changed on the server after CelikPanel changed it, so it was kept as it is now"
}

// undo puts every recorded path back, the last changed first: a path still
// as CelikPanel set it gets its earlier state (a file the move itself wrote
// is removed while it still holds exactly what the move wrote), one already
// as it was is left, one that is gone now has nothing to put back (said as
// already gone, not as changed), and one that changed since is kept and
// named (never overwritten with an earlier state). The record goes when
// nothing failed.
func (j *phpToolFileJournal) undo() error {
	var errs []error
	kept := &errChangedOutsideTheMove{}
	var gone []string
	for i := len(j.before) - 1; i >= 0; i-- {
		state := j.before[i]
		if state.After != nil {
			// The content is read whenever CelikPanel wrote some, also for a
			// file the move created (S4 R1, third fix round).
			withContent := state.HasContent || state.After.ContentSHA != ""
			current, err := currentPHPToolFileState(state.Path, withContent, state.After.ACLSet)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if state.asBefore(current) {
				continue
			}
			if state.Existed && !current.Existed {
				gone = append(gone, state.Path)
				continue
			}
			if !state.asSet(current) {
				kept.paths = append(kept.paths, state.Path)
				continue
			}
		}
		if err := restorePHPToolFileState(state); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		if err := clearPHPToolFileJournal(j.tool); err != nil {
			errs = append(errs, err)
		}
	}
	if len(gone) > 0 {
		// Routine for webmail's temp/ files and SQLite journal: nothing is lost.
		log.Printf("%s's files: %s already gone, nothing to put back", phpToolDisplayName(j.tool), strings.Join(gone, ", "))
	}
	if len(kept.paths) > 0 {
		log.Printf("%s's files: %s", phpToolDisplayName(j.tool), kept.Error())
		errs = append(errs, kept)
	}
	return errors.Join(errs...)
}

// restorePHPToolFileState is a variable so unprivileged tests can record it.
var restorePHPToolFileState = func(state phpToolFileState) error {
	if !state.Existed {
		return removePHPToolFileNoFollow(state.Path)
	}
	if state.HasContent {
		if err := secureWriteConfigOwnedBy(state.Path, state.Content, fileModeOfBits(state.Mode), uint32(state.UID), uint32(state.GID)); err != nil {
			return err
		}
	}
	restoreACL := state.ACLRead || (state.After != nil && state.After.ACLSet)
	// No list before: it goes first, then the mode (removing a list leaves
	// the mode as the list made it). A list before: the mode, then the list.
	if restoreACL && len(state.ACL) == 0 {
		if err := writePathACL(state.Path, nil); err != nil {
			return err
		}
	}
	if err := setPHPToolPathMeta(state.Path, state.Dir, state.UID, state.GID, fileModeOfBits(state.Mode)); err != nil {
		return err
	}
	if restoreACL && len(state.ACL) > 0 {
		return writePathACL(state.Path, state.ACL)
	}
	return nil
}

// removePHPToolFileNoFollow removes a file CelikPanel made, never through a
// symbolic link anywhere on its path.
func removePHPToolFileNoFollow(path string) error {
	if err := rejectSymlinkPath(filepath.Dir(path)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not the file CelikPanel wrote; it was kept", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// setPHPToolPathMeta sets owner, group and mode (special bits included)
// without following a link; a variable so unprivileged tests can record it.
var setPHPToolPathMeta = func(path string, dir bool, uid, gid int, mode os.FileMode) error {
	if dir {
		return secureSetMailDirectoryMetadata(path, mode, uid, gid)
	}
	return secureSetMailFileMetadata(path, mode, uid, gid)
}

// clearPHPToolFileJournal forgets the record (the move is proven, or put
// back).
func clearPHPToolFileJournal(tool string) error {
	if err := os.Remove(phpToolFilesRecordPath(tool)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// --- Roundcube under its own account ---

// roundcubeMetaChange is one path's new owner, group and mode (special bits
// kept), and for the tree's root the access-control list it gets.
type roundcubeMetaChange struct {
	Path     string
	Dir      bool
	UID, GID int
	Mode     os.FileMode
	ACL      []byte
}

// roundcubePluginConfigName: the configuration a Roundcube plugin reads
// (rcube_plugin::load_config, config.inc.php by default; a plugin may name
// another config*.php). The .dist samples are published.
func roundcubePluginConfigName(name string) bool {
	return strings.HasPrefix(name, "config") && strings.HasSuffix(name, ".php")
}

// errWebmailNoACL: the tree's file system refuses access-control lists.
type errWebmailNoACL struct{ path string }

func (e *errWebmailNoACL) Error() string {
	return fmt.Sprintf("the file system of %s does not accept access-control lists, so webmail's own account cannot be let into its folder without opening that folder to every website account; webmail stays on the web server's account and nothing was changed. The server owner can mount that file system with access-control lists (the acl mount option; ext4, XFS and Btrfs have them by default), then choose webmail's PHP version again in its Manage window", e.path)
}

// roundcubeWalk walks root without following a link: an entry that vanished
// after it was listed (a temporary file, an SQLite journal) has nothing to
// change and is passed over; a root that is not there is nothing to walk;
// any other error ends the plan as a failure, never as a shorter plan.
func roundcubeWalk(root string, visit func(path string, info os.FileInfo)) error {
	absent := errors.New("absent")
	err := roundcubeWalkTree(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				if path == root {
					return absent
				}
				return nil
			}
			return err
		}
		visit(path, info)
		return nil
	})
	if errors.Is(err, absent) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", root, err)
	}
	return nil
}

// roundcubeWalkTree is filepath.Walk; a variable so a test can make an entry
// vanish or a directory fail in the middle of a walk.
var roundcubeWalkTree = filepath.Walk

// roundcubeOwnAccountPlan lists the changes that give a Roundcube tree to its
// own account, private parts first and the tree's root last. Only what must
// change is listed, and no link is followed:
//   - db/, temp/ and logs/ and what is in them become the account's, closed
//     to others (group, owner and special bits kept);
//   - config/ becomes root's with the account's group, 0750; what in it has
//     the web server's group gets the account's group, its mode kept;
//   - a plugin's configuration (plugins/<name>/config*.php) the web server's
//     group read gets the account's group, so the account reads it as the
//     stock pool did; one neither that group nor others read is kept and
//     named in notes;
//   - the tree's root stays closed to others (S4 R1, second fix round): only
//     webmail's own account gets through it, by the access-control-list
//     entry "user:<account>:--x". No website account enters the tree on
//     the file system, so nothing below it (nested plugin configurations,
//     .bak or ~ copies, a plugin folder that is a link) can be opened by
//     them directly.
//
// The web server keeps reaching public_html through the root's group (the
// web group, as the install leaves it). Nothing else changes: the published
// code keeps the owner, group and modes it has.
//
// Not closed by this (fourth fix round, stated as it is): webmail's server
// on the panel's local socket (/run/celikpanel-webmail.sock, mode 0666 as
// nginx creates it) still gives every local account, without a login, the
// files under public_html and runs its .php files with webmail's pool, as on
// main. alpha.145 gives that server the rules of webmail's own address; this
// branch takes them when it merges that release.
func roundcubeOwnAccountPlan(baseDir string, uid, gid, webGID int) ([]roundcubeMetaChange, []string, error) {
	var plan []roundcubeMetaChange
	var notes []string
	owner := func(info os.FileInfo) (int, int) {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			return int(stat.Uid), int(stat.Gid)
		}
		return -1, -1
	}
	add := func(path string, info os.FileInfo, newUID, newGID int, mode os.FileMode) {
		fileUID, fileGID := owner(info)
		if fileUID == newUID && fileGID == newGID && keptModeBits(info.Mode()) == keptModeBits(mode) {
			return
		}
		plan = append(plan, roundcubeMetaChange{Path: path, Dir: info.IsDir(), UID: newUID, GID: newGID, Mode: keptModeBits(mode)})
	}
	special := func(info os.FileInfo) os.FileMode {
		return info.Mode() & (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	}
	// The root first: a file system without access-control lists stops the
	// move before anything changes (it never falls back to opening the root).
	rootInfo, err := os.Lstat(baseDir)
	if err != nil {
		return nil, nil, err
	}
	rootACL, err := readPathACL(baseDir)
	if errors.Is(err, errACLUnsupported) {
		return nil, nil, &errWebmailNoACL{path: baseDir}
	}
	if err != nil {
		return nil, nil, err
	}
	for _, name := range []string{"db", "temp", "logs"} {
		root := filepath.Join(baseDir, name)
		err := roundcubeWalk(root, func(path string, info os.FileInfo) {
			if info.Mode()&os.ModeSymlink != 0 || !(info.IsDir() || info.Mode().IsRegular()) {
				return
			}
			mode := special(info) | info.Mode().Perm()&^0o007 | 0o600
			switch {
			case path == root:
				mode = special(info) | 0o700
			case info.IsDir():
				mode |= 0o700
			}
			add(path, info, uid, gid, mode)
		})
		if err != nil {
			return nil, nil, err
		}
	}
	configDir := filepath.Join(baseDir, "config")
	err = roundcubeWalk(configDir, func(path string, info os.FileInfo) {
		if info.Mode()&os.ModeSymlink != 0 {
			return
		}
		fileUID, fileGID := owner(info)
		if path == configDir {
			add(path, info, 0, gid, special(info)|0o750)
			return
		}
		if fileGID == webGID {
			add(path, info, fileUID, gid, info.Mode())
		}
	})
	if err != nil {
		return nil, nil, err
	}
	pluginsDir := filepath.Join(baseDir, "plugins")
	if info, err := os.Lstat(pluginsDir); err == nil && info.IsDir() {
		plugins, err := os.ReadDir(pluginsDir)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", pluginsDir, err)
		}
		for _, plugin := range plugins {
			if !plugin.IsDir() {
				continue // a file, or a link (never followed)
			}
			dir := filepath.Join(pluginsDir, plugin.Name())
			files, err := os.ReadDir(dir)
			if err != nil {
				return nil, nil, fmt.Errorf("read %s: %w", dir, err)
			}
			for _, file := range files {
				if !roundcubePluginConfigName(file.Name()) || !file.Type().IsRegular() {
					continue
				}
				path := filepath.Join(dir, file.Name())
				info, err := os.Lstat(path)
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return nil, nil, err
				}
				fileUID, fileGID := owner(info)
				mode := info.Mode().Perm()
				switch {
				case fileUID == uid || mode&0o004 != 0:
				case fileGID == webGID && mode&0o040 != 0:
					add(path, info, fileUID, gid, info.Mode())
				default:
					notes = append(notes, fmt.Sprintf("%s is kept as its owner set it (%04o); webmail's account does not read it", path, mode))
				}
			}
		}
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}
	rootUID, rootGID := owner(rootInfo)
	closed := special(rootInfo) | rootInfo.Mode().Perm()&^0o007
	acl, afterMode, err := aclWithUser(rootACL, closed, uid, aclExecute)
	if err != nil {
		return nil, nil, fmt.Errorf("the access-control list of %s cannot be read: %w", baseDir, err)
	}
	if keptModeBits(rootInfo.Mode()) != keptModeBits(afterMode) || !sameACL(rootACL, acl) {
		plan = append(plan, roundcubeMetaChange{Path: baseDir, Dir: true, UID: rootUID, GID: rootGID, Mode: keptModeBits(afterMode), ACL: acl})
	}
	return plan, notes, nil
}

func roundcubePlanned(plan []roundcubeMetaChange) []phpToolPlanned {
	planned := make([]phpToolPlanned, 0, len(plan))
	for _, change := range plan {
		planned = append(planned, phpToolPlanned{Path: change.Path, After: phpToolFileMeta{
			UID: change.UID, GID: change.GID, Mode: unixModeBits(change.Mode), ACLSet: change.ACL != nil, ACL: change.ACL,
		}})
	}
	return planned
}

func applyRoundcubePlan(plan []roundcubeMetaChange) error {
	for _, change := range plan {
		if err := setPHPToolPathMeta(change.Path, change.Dir, change.UID, change.GID, change.Mode); err != nil {
			return fmt.Errorf("give %s to webmail's account: %w", change.Path, err)
		}
		if change.ACL != nil {
			if err := writePathACL(change.Path, change.ACL); err != nil {
				if errors.Is(err, errACLUnsupported) {
					return &errWebmailNoACL{path: change.Path}
				}
				return fmt.Errorf("let webmail's account into %s: %w", change.Path, err)
			}
		}
	}
	return nil
}

func webServerGID() int {
	if group, err := user.LookupGroup(webServerGroup()); err == nil {
		if gid, err := strconv.Atoi(group.Gid); err == nil {
			return gid
		}
	}
	return -1
}

// webmailRootEntryNote says, for a tool already on its own account, when the
// entry that lets webmail's account into its tree is gone (the server owner
// removed it, or a restore brought the tree back without it): webmail then
// cannot read its files. It is not put back by itself; choosing webmail's PHP
// version again in its Manage window puts it back. "" when it is there.
var webmailRootEntryNote = func() string {
	uid, _, found, err := lookupSystemAccount(phpToolAccount(phpToolWebmail))
	if err != nil || !found {
		return ""
	}
	acl, err := readPathACL(webmailBaseDir)
	if err != nil || aclHasUser(acl, uid, aclExecute) {
		return ""
	}
	return fmt.Sprintf("the access-control-list entry that lets %s into %s is not there (removed on the server), so webmail cannot read its files; CelikPanel does not put it back by itself: choosing webmail's PHP version again in its Manage window puts it back", phpToolAccount(phpToolWebmail), webmailBaseDir)
}

// applyRoundcubeOwnAccountPermissions gives a tree to the account without a
// record (a staging tree of an update, or a tree already served by the
// account's own pool).
func applyRoundcubeOwnAccountPermissions(baseDir string, uid, gid int) error {
	baseDir, err := validateRoundcubeTreePath(baseDir)
	if err != nil {
		return err
	}
	plan, notes, err := roundcubeOwnAccountPlan(baseDir, uid, gid, webServerGID())
	if err != nil {
		return err
	}
	logRoundcubeNotes(notes)
	return applyRoundcubePlan(plan)
}

func logRoundcubeNotes(notes []string) {
	sort.Strings(notes)
	for _, note := range notes {
		log.Printf("webmail own account: %s", note)
	}
}

// prepareRoundcubeOwnAccountWith gives Roundcube's tree to the account.
// servedOwn: the web server already hands webmail to its own pool, so a
// failure later in this move must not take the files from the account; the
// earlier attempt's record (if any) is stale and goes. Otherwise every change
// is recorded first and the undo puts back what the record holds.
func prepareRoundcubeOwnAccountWith(baseDir string, uid, gid int, servedOwn bool) (func() error, error) {
	baseDir, err := validateRoundcubeTreePath(baseDir)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(baseDir)
	if err != nil {
		return nil, fmt.Errorf("inspect Roundcube tree: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("Roundcube tree is not a real directory: %s", baseDir)
	}
	plan, notes, err := roundcubeOwnAccountPlan(baseDir, uid, gid, webServerGID())
	if err != nil {
		return nil, err
	}
	logRoundcubeNotes(notes)
	if servedOwn {
		if err := clearPHPToolFileJournal(phpToolWebmail); err != nil {
			return nil, err
		}
		return nil, applyRoundcubePlan(plan)
	}
	journal, err := loadPHPToolFileJournal(phpToolWebmail)
	if err != nil {
		return nil, err
	}
	if err := journal.note(roundcubePlanned(plan)); err != nil {
		return nil, fmt.Errorf("record webmail's files before changing them: %w", err)
	}
	if err := applyRoundcubePlan(plan); err != nil {
		if undoErr := journal.undo(); undoErr != nil {
			return nil, fmt.Errorf("%w; putting back what was changed: %v", err, undoErr)
		}
		return nil, err
	}
	if len(journal.before) == 0 {
		return nil, nil
	}
	return journal.undo, nil
}
