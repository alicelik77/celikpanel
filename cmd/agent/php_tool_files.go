package main

import (
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
// back exactly (S4 R1 fix round, 21 Nov 2026).
//
// The owner, group and mode every changed path had before are written to the
// Agent's private state (php-tool-files-<tool>.json, root 0600) BEFORE the
// change, so the undo is exact even after an interruption: a move cut by a
// power loss after the files were given to the account and before the web
// server was switched leaves webmail unreadable for the stock pool; the next
// start's retry then puts back what the FIRST attempt recorded, not what it
// finds. Whether a move's failure puts the files back is decided from what
// serves the tool now (the socket its web server configuration names), never
// from a file's group: a tool already served by its own pool keeps its files
// with the account (a move between PHP versions).
//
// The record is removed when the tool answers on its own pool (the move is
// proven) or when the undo has put everything back.
//
// Aracın dosyalarını kendi hesabına verirken her yolun önceki sahibi, grubu
// ve kipi değişiklikten ÖNCE kaydedilir; geri alma bir kesintiden sonra da
// tamdır. Geri alınıp alınmayacağına aracı şu an neyin sunduğuna bakılarak
// karar verilir, dosyanın grubuna bakılarak değil.

var phpToolFilesRecordDir = filepath.Dir(phpToolAccountRecordPath)

func phpToolFilesRecordPath(tool string) string {
	return filepath.Join(phpToolFilesRecordDir, "php-tool-files-"+tool+".json")
}

// phpToolFileState is one path as it was before CelikPanel changed it.
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
	seen   map[string]bool
}

func loadPHPToolFileJournal(tool string) (*phpToolFileJournal, error) {
	journal := &phpToolFileJournal{tool: tool, seen: map[string]bool{}}
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
		if !journal.seen[state.Path] {
			journal.seen[state.Path] = true
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

// note records how path is now, unless an earlier attempt recorded it
// already (that earlier state is the one to come back). It is saved before
// the caller changes anything.
func (j *phpToolFileJournal) note(paths []string, withContent map[string]bool) error {
	added := false
	for _, path := range paths {
		if j.seen[path] {
			continue
		}
		state, err := readPHPToolFileState(path, withContent[path])
		if err != nil {
			return err
		}
		j.seen[path] = true
		j.before = append(j.before, state)
		added = true
	}
	if !added {
		return nil
	}
	return j.save()
}

func readPHPToolFileState(path string, withContent bool) (phpToolFileState, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return phpToolFileState{Path: path}, nil
	}
	if err != nil {
		return phpToolFileState{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return phpToolFileState{}, fmt.Errorf("the owner of %s could not be read", path)
	}
	state := phpToolFileState{Path: path, Existed: true, Dir: info.IsDir(), UID: int(stat.Uid), GID: int(stat.Gid), Mode: uint32(info.Mode().Perm())}
	if withContent && info.Mode().IsRegular() {
		content, err := os.ReadFile(path)
		if err != nil {
			return phpToolFileState{}, err
		}
		state.Content, state.HasContent = content, true
	}
	return state, nil
}

// undo puts every recorded path back, the last changed first, and removes
// the record when all came back.
func (j *phpToolFileJournal) undo() error {
	var errs []error
	for i := len(j.before) - 1; i >= 0; i-- {
		if err := restorePHPToolFileState(j.before[i]); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return clearPHPToolFileJournal(j.tool)
}

// restorePHPToolFileState is a variable so unprivileged tests can record it.
var restorePHPToolFileState = func(state phpToolFileState) error {
	if !state.Existed {
		if err := os.Remove(state.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if state.HasContent {
		if err := secureWriteConfigOwnedBy(state.Path, state.Content, os.FileMode(state.Mode), uint32(state.UID), uint32(state.GID)); err != nil {
			return err
		}
	}
	return setPHPToolPathMeta(state.Path, state.Dir, state.UID, state.GID, os.FileMode(state.Mode))
}

// setPHPToolPathMeta sets owner, group and mode without following a link; a
// variable so unprivileged tests can record it.
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

// roundcubePublished are the top-level entries of Roundcube's complete
// release (its Makefile, target "complete": the repository's tree without
// tests and build files, plus vendor/ and composer.json/.lock), apart from
// the private config/, db/, temp/ and logs/. They are its published code:
// the web server reads the static parts through public_html (whose plugins
// and skins are links to ../plugins and ../skins, and whose program/js and
// program/resources are links into ../program), and webmail's own account
// reads the PHP. A site account may still read them by name (the tree's
// own directory lets everyone through but not list it): they are the files
// anyone can download from roundcube.net. Anything else at the top is not
// known and is closed to others.
var roundcubePublished = map[string]bool{
	".htaccess": true, "CHANGELOG.md": true, "INSTALL": true, "LICENSE": true, "README.md": true,
	"SECURITY.md": true, "UPGRADING": true, "SQL": true, "bin": true, "composer.json": true,
	"composer.json-dist": true, "composer.lock": true, "index.php": true, "installer": true,
	"plugins": true, "program": true, "public_html": true, "skins": true, "vendor": true,
}

// roundcubePrivateDirs are the parts only the account opens; config/ is
// root's with the account's group.
var roundcubePrivateDirs = []string{"db", "temp", "logs"}

// roundcubeMetaChange is one path's new owner, group and mode.
type roundcubeMetaChange struct {
	Path     string
	Dir      bool
	UID, GID int
	Mode     os.FileMode
}

// roundcubePluginConfigName: the configuration a Roundcube plugin reads
// (rcube_plugin::load_config, config.inc.php by default; a plugin may name
// another config*.php). The .dist samples are published.
func roundcubePluginConfigName(name string) bool {
	return strings.HasPrefix(name, "config") && strings.HasSuffix(name, ".php")
}

// roundcubeOwnAccountPlan lists the changes that give a Roundcube tree to its
// own account, private parts first and the tree's own directory last, so
// nothing private is open to others on the way. Only what must change is
// listed:
//   - db/, temp/ and logs/ and what is in them become the account's, closed
//     to others (group and owner bits kept);
//   - config/ becomes root's with the account's group, 0750; what in it has
//     the web server's group gets the account's group, its mode kept;
//   - a plugin's configuration (plugins/<name>/config*.php): one the web
//     server's group read gets the account's group, one everyone read is
//     closed to others and given the account's group; one the owner closed
//     to both is kept as it is and named in notes;
//   - an unknown top-level entry readable by others is closed to them (the
//     entry itself, not what is under it);
//   - the tree's own directory lets everyone through but not list it (o=x).
//
// Nothing else changes: the published code keeps the owner, group and modes
// it has.
func roundcubeOwnAccountPlan(baseDir string, uid, gid, webGID int) ([]roundcubeMetaChange, []string, error) {
	var plan []roundcubeMetaChange
	var notes []string
	add := func(path string, info os.FileInfo, newUID, newGID int, mode os.FileMode) {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if ok && int(stat.Uid) == newUID && int(stat.Gid) == newGID && info.Mode().Perm() == mode.Perm() {
			return
		}
		plan = append(plan, roundcubeMetaChange{Path: path, Dir: info.IsDir(), UID: newUID, GID: newGID, Mode: mode.Perm()})
	}
	owner := func(info os.FileInfo) (int, int) {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			return int(stat.Uid), int(stat.Gid)
		}
		return -1, -1
	}
	for _, name := range roundcubePrivateDirs {
		root := filepath.Join(baseDir, name)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || !(info.IsDir() || info.Mode().IsRegular()) {
				return nil
			}
			mode := info.Mode().Perm()&^0o007 | 0o600
			switch {
			case path == root:
				mode = 0o700
			case info.IsDir():
				mode |= 0o700
			}
			add(path, info, uid, gid, mode)
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
	}
	configDir := filepath.Join(baseDir, "config")
	err := filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		fileUID, fileGID := owner(info)
		if path == configDir {
			add(path, info, 0, gid, 0o750)
			return nil
		}
		if fileGID == webGID {
			add(path, info, fileUID, gid, info.Mode().Perm())
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}
	plugins, err := os.ReadDir(filepath.Join(baseDir, "plugins"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}
	for _, plugin := range plugins {
		if !plugin.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(baseDir, "plugins", plugin.Name()))
		if err != nil {
			return nil, nil, err
		}
		for _, file := range files {
			if !roundcubePluginConfigName(file.Name()) || !file.Type().IsRegular() {
				continue
			}
			path := filepath.Join(baseDir, "plugins", plugin.Name(), file.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return nil, nil, err
			}
			fileUID, fileGID := owner(info)
			mode := info.Mode().Perm()
			switch {
			case fileUID == uid:
				add(path, info, fileUID, fileGID, mode&^0o007)
			case fileGID == webGID && mode&0o040 != 0:
				add(path, info, fileUID, gid, mode&^0o007)
			case mode&0o004 != 0:
				add(path, info, fileUID, gid, (mode&^0o007)|0o040)
			default:
				notes = append(notes, fmt.Sprintf("%s is kept as its owner set it (%04o); webmail's account does not read it", path, mode))
			}
		}
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if roundcubePublished[name] || name == "config" || name == "db" || name == "temp" || name == "logs" {
			continue
		}
		path := filepath.Join(baseDir, name)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.Mode().Perm()&0o007 != 0 {
			fileUID, fileGID := owner(info)
			add(path, info, fileUID, fileGID, info.Mode().Perm()&^0o007)
		}
	}
	rootInfo, err := os.Lstat(baseDir)
	if err != nil {
		return nil, nil, err
	}
	rootUID, rootGID := owner(rootInfo)
	add(baseDir, rootInfo, rootUID, rootGID, (rootInfo.Mode().Perm()&^0o007)|0o001)
	return plan, notes, nil
}

func roundcubePlanPaths(plan []roundcubeMetaChange) []string {
	paths := make([]string, 0, len(plan))
	for _, change := range plan {
		paths = append(paths, change.Path)
	}
	return paths
}

func applyRoundcubePlan(plan []roundcubeMetaChange) error {
	for _, change := range plan {
		if err := setPHPToolPathMeta(change.Path, change.Dir, change.UID, change.GID, change.Mode); err != nil {
			return fmt.Errorf("give %s to webmail's account: %w", change.Path, err)
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
	if err := journal.note(roundcubePlanPaths(plan), nil); err != nil {
		return nil, fmt.Errorf("record webmail's files before changing them: %w", err)
	}
	if err := applyRoundcubePlan(plan); err != nil {
		if undoErr := journal.undo(); undoErr != nil {
			return nil, fmt.Errorf("%w; putting back what was changed also failed: %v", err, undoErr)
		}
		return nil, err
	}
	if len(journal.before) == 0 {
		return nil, nil
	}
	return journal.undo, nil
}
