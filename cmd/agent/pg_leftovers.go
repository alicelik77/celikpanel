package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Leftover PostgreSQL clusters (alpha.130; the owner's rule of 8 Nov 2026:
// "no leftovers, ever"). Boston, 3 Nov 2026: PostgreSQL 16 was installed,
// removed with apt's "remove" (its cluster kept on disk, its packages in dpkg
// state "rc") and PostgreSQL 18 installed after it. pg_lsclusters then said
// "16 main 5432 down,binaries_missing" and "18 main 5433 online": 18 got 5433
// because 16's leftover postgresql.conf still claimed 5432, and the
// Components page edited 16's files. 16/main held only what initdb made.
//
// A cluster whose program is gone is read here from its own files, never
// guessed:
//
//   - Fresh: the data directory holds only what initdb made. Every database
//     directory under base/ and every relation file in it and in global/ is
//     numbered below 16384 (FirstNormalObjectId: initdb's objects are
//     numbered below it, everything made afterwards above it), pg_tblspc is
//     empty, and pg_authid (global/1260) names no role numbered from 16384
//     up other than CelikPanel's own account. The Agent removes such a
//     cluster itself; the Panel records it in the audit log.
//   - Otherwise it may hold the owner's data and is never removed silently:
//     the PostgreSQL page names it and offers "Archive and remove", which
//     keeps its files in /var/backups/celikpanel first.
//
// Removing a cluster removes exactly its own files: /etc/postgresql/V/N, its
// data directory when it is the standard /var/lib/postgresql/V/N, and its
// log files /var/log/postgresql/postgresql-V-N.log*; the major's directories
// go when they are empty. Nothing outside these paths is touched.
//
// Programı kaldırılmış bir kümenin dosyaları okunur: yalnız initdb'nin
// yaptıklarını tutan küme Agent tarafından kaldırılır ve denetim günlüğüne
// yazılır; veri tutabilecek küme asla sessizce silinmez, sahibi "Arşivle ve
// kaldır" ile açıkça seçer.

const (
	pgFirstNormalObjectID = 16384
	pgPageSize            = 8192
	// pgLeftoverWalkCap bounds how many entries a data directory may have
	// for the reading to be trusted (a fresh cluster has about 1 000).
	pgLeftoverWalkCap = 20000
)

var (
	pgDataRoot     = "/var/lib/postgresql"
	pgLogRoot      = "/var/log/postgresql"
	pgArchiveRoot  = "/var/backups/celikpanel"
	pgRelationFile = regexp.MustCompile(`^([0-9]+)(_(fsm|vm|init))?(\.[0-9]+)?$`)
	// pgLeftoverUnitReset forgets a removed cluster's unit (reset-failed and
	// daemon-reload; a variable for tests).
	pgLeftoverUnitReset = hostPGLeftoverUnitReset
	pgLeftoverNow       = time.Now
)

// pgCatalogueNames are the files initdb puts in a database directory or in
// global/ that are not numbered relation files.
var pgCatalogueNames = map[string]bool{
	"PG_VERSION": true, "pg_filenode.map": true, "pg_internal.init": true, "pg_control": true,
}

// pgServerBinary is the program a cluster of major version runs.
func pgServerBinary(version string) string {
	return filepath.Join(pgLibRoot, version, "bin", "postgres")
}

// pgStandardDataDirectory is where postgresql-common puts a cluster's data.
func pgStandardDataDirectory(version, name string) string {
	return filepath.Join(pgDataRoot, version, name)
}

// pgClusterIsLeftover: pg_lsclusters says its program is missing, it does
// not run, and the major's server program is really not on disk.
func pgClusterIsLeftover(cluster transport.PostgreSQLCluster) bool {
	if !pgClusterRemoved(cluster) || cluster.Online || !validPGClusterRef(cluster.Version, cluster.Name) {
		return false
	}
	_, err := os.Stat(pgServerBinary(cluster.Version))
	return errors.Is(err, fs.ErrNotExist)
}

func realDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

// classifyPGLeftover reads a leftover cluster's data directory.
func classifyPGLeftover(cluster transport.PostgreSQLCluster) *transport.PostgreSQLLeftover {
	result := &transport.PostgreSQLLeftover{}
	notFresh := func(reason string) *transport.PostgreSQLLeftover {
		result.Fresh, result.Reason = false, reason
		return result
	}
	dataDir := filepath.Clean(cluster.DataDirectory)
	if cluster.DataDirectory == "" || dataDir != pgStandardDataDirectory(cluster.Version, cluster.Name) {
		return notFresh(fmt.Sprintf("its data directory %s is not the standard %s", cluster.DataDirectory, pgStandardDataDirectory(cluster.Version, cluster.Name)))
	}
	if !realDirectory(dataDir) {
		if _, err := os.Lstat(dataDir); errors.Is(err, fs.ErrNotExist) {
			// Nothing of the data is left: only its configuration.
			result.Fresh = true
			return result
		}
		return notFresh("its data directory is not a plain directory")
	}
	entries := 0
	var size int64
	walkErr := filepath.WalkDir(dataDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries++
		if entries > pgLeftoverWalkCap {
			return errors.New("too many files")
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(dataDir, path)
			return fmt.Errorf("%s is a link", rel)
		}
		if entry.Type().IsRegular() {
			if info, err := entry.Info(); err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	result.DataBytes = size
	if walkErr != nil {
		return notFresh("its data directory could not be read completely (" + boundedText(walkErr.Error(), 120) + ")")
	}
	if _, err := os.Lstat(filepath.Join(dataDir, "postmaster.pid")); err == nil {
		return notFresh("it has a postmaster.pid, so it may still be running")
	}
	if data, err := os.ReadFile(filepath.Join(dataDir, "PG_VERSION")); err != nil || strings.TrimSpace(string(data)) != cluster.Version {
		return notFresh("its PG_VERSION does not say " + cluster.Version)
	}
	if major, _ := strconv.Atoi(cluster.Version); major < 12 {
		return notFresh("its roles cannot be read for PostgreSQL before 12")
	}
	if names, err := os.ReadDir(filepath.Join(dataDir, "pg_tblspc")); err != nil || len(names) > 0 {
		return notFresh("it has tablespaces")
	}
	bases, err := os.ReadDir(filepath.Join(dataDir, "base"))
	if err != nil || len(bases) == 0 {
		return notFresh("its databases could not be read")
	}
	for _, base := range bases {
		if base.Name() == "pgsql_tmp" && base.IsDir() {
			if temp, err := os.ReadDir(filepath.Join(dataDir, "base", base.Name())); err == nil && len(temp) == 0 {
				continue
			}
			return notFresh("it has temporary files")
		}
		oid, err := strconv.Atoi(base.Name())
		if err != nil || !base.IsDir() {
			return notFresh("base/" + base.Name() + " is not a database directory")
		}
		if oid >= pgFirstNormalObjectID {
			return notFresh("it has a database made after initdb")
		}
		if reason := pgCatalogueOnly(filepath.Join(dataDir, "base", base.Name())); reason != "" {
			return notFresh(reason)
		}
	}
	if reason := pgCatalogueOnly(filepath.Join(dataDir, "global")); reason != "" {
		return notFresh(reason)
	}
	authid, err := os.ReadFile(filepath.Join(dataDir, "global", "1260"))
	if err != nil {
		return notFresh("its roles could not be read")
	}
	roles, ok := pgAuthidUserRoles(authid)
	if !ok {
		return notFresh("its roles could not be read")
	}
	for _, role := range roles {
		if role != databaseAdminAccountName {
			return notFresh("it has roles made after initdb (" + boundedText(strings.Join(roles, ", "), 120) + ")")
		}
	}
	result.Fresh = true
	return result
}

// pgCatalogueOnly says why a database (or global/) directory holds more than
// initdb's own files, or "".
func pgCatalogueOnly(dir string) string {
	files, err := os.ReadDir(dir)
	if err != nil {
		return filepath.Base(dir) + " could not be read"
	}
	for _, file := range files {
		if file.IsDir() {
			return "it has an unexpected directory " + filepath.Join(filepath.Base(dir), file.Name())
		}
		if pgCatalogueNames[file.Name()] {
			continue
		}
		m := pgRelationFile.FindStringSubmatch(file.Name())
		if m == nil {
			return "it has an unexpected file " + filepath.Join(filepath.Base(dir), file.Name())
		}
		if number, err := strconv.Atoi(m[1]); err != nil || number >= pgFirstNormalObjectID {
			return "it has tables or other objects made after initdb"
		}
	}
	return ""
}

// pgAuthidUserRoles reads the heap file of pg_authid (PostgreSQL 12 and
// later, 8 KiB pages): the names of every role tuple numbered from 16384 up,
// whether live or not yet vacuumed away (a dropped role counts: when in
// doubt, not fresh). ok is false when the file does not read as heap pages.
func pgAuthidUserRoles(data []byte) (names []string, ok bool) {
	if len(data) == 0 || len(data)%pgPageSize != 0 {
		return nil, false
	}
	for start := 0; start < len(data); start += pgPageSize {
		page := data[start : start+pgPageSize]
		if binary.LittleEndian.Uint16(page[14:16]) == 0 {
			// A new, never written page (PageIsNew) holds nothing.
			continue
		}
		if binary.LittleEndian.Uint16(page[18:20]) != pgPageSize|4 {
			return nil, false
		}
		lower := int(binary.LittleEndian.Uint16(page[12:14]))
		upper := int(binary.LittleEndian.Uint16(page[14:16]))
		if lower < 24 || lower > pgPageSize || upper < lower || upper > pgPageSize {
			return nil, false
		}
		for item := 24; item+4 <= lower; item += 4 {
			id := binary.LittleEndian.Uint32(page[item : item+4])
			offset, flags, length := int(id&0x7fff), (id>>15)&3, int(id>>17)
			if flags != 1 { // LP_NORMAL only: unused, redirect and dead items hold no tuple
				continue
			}
			if length < 24 || offset < upper || offset+length > pgPageSize {
				return nil, false
			}
			tuple := page[offset : offset+length]
			hoff := int(tuple[22])
			if hoff < 23 || hoff+4 > len(tuple) {
				return nil, false
			}
			oid := binary.LittleEndian.Uint32(tuple[hoff : hoff+4])
			if oid < pgFirstNormalObjectID {
				continue
			}
			name := ""
			if end := hoff + 4 + 64; end <= len(tuple) {
				raw := tuple[hoff+4 : end]
				if nul := strings.IndexByte(string(raw), 0); nul >= 0 {
					raw = raw[:nul]
				}
				name = string(raw)
			}
			if name == "" {
				name = "oid " + strconv.FormatUint(uint64(oid), 10)
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, true
}

// pgLeftoverPaths are the files of one cluster that a removal deletes.
type pgLeftoverPaths struct {
	config string
	data   string // empty when it is not the standard directory
	logs   []string
}

func pgLeftoverFiles(cluster transport.PostgreSQLCluster) pgLeftoverPaths {
	paths := pgLeftoverPaths{}
	if config := filepath.Join(pgConfRoot, cluster.Version, cluster.Name); realDirectory(config) {
		paths.config = config
	}
	standard := pgStandardDataDirectory(cluster.Version, cluster.Name)
	if cluster.DataDirectory == "" || filepath.Clean(cluster.DataDirectory) == standard {
		if realDirectory(standard) {
			paths.data = standard
		}
	}
	matches, _ := filepath.Glob(filepath.Join(pgLogRoot, "postgresql-"+cluster.Version+"-"+cluster.Name+".log*"))
	for _, match := range matches {
		if info, err := os.Lstat(match); err == nil && info.Mode().IsRegular() {
			paths.logs = append(paths.logs, match)
		}
	}
	sort.Strings(paths.logs)
	return paths
}

// removePGLeftoverFiles deletes one cluster's own files and the major's
// directories that are then empty, and says what it removed.
func removePGLeftoverFiles(cluster transport.PostgreSQLCluster) ([]string, error) {
	paths := pgLeftoverFiles(cluster)
	var removed []string
	for _, path := range []string{paths.config, paths.data} {
		if path == "" {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return removed, fmt.Errorf("%s could not be removed: %v", path, err)
		}
		removed = append(removed, path)
	}
	for _, path := range paths.logs {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return removed, fmt.Errorf("%s could not be removed: %v", path, err)
		}
		removed = append(removed, path)
	}
	for _, dir := range []string{filepath.Join(pgConfRoot, cluster.Version), filepath.Join(pgDataRoot, cluster.Version)} {
		if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 && realDirectory(dir) {
			if os.Remove(dir) == nil {
				removed = append(removed, dir)
			}
		}
	}
	pgLeftoverUnitReset(cluster.Version, cluster.Name)
	return removed, nil
}

// archivePGLeftover keeps one cluster's files (configuration, data
// directory, logs) in one gzip'd tar under /var/backups/celikpanel, readable
// by root only, and returns its path. Links are kept as links, never
// followed. The archive is read back before it is trusted.
func archivePGLeftover(cluster transport.PostgreSQLCluster, freeBytes func(string) (int64, error)) (string, error) {
	paths := pgLeftoverFiles(cluster)
	sources := []string{}
	for _, path := range []string{paths.config, paths.data} {
		if path != "" {
			sources = append(sources, path)
		}
	}
	sources = append(sources, paths.logs...)
	if len(sources) == 0 {
		return "", errors.New("the cluster has no files left to archive")
	}
	if err := os.MkdirAll(pgArchiveRoot, 0o700); err != nil {
		return "", fmt.Errorf("%s could not be made: %v", pgArchiveRoot, err)
	}
	var need int64
	for _, source := range sources {
		_ = filepath.WalkDir(source, func(_ string, entry fs.DirEntry, err error) error {
			if err == nil && entry.Type().IsRegular() {
				if info, infoErr := entry.Info(); infoErr == nil {
					need += info.Size()
				}
			}
			return nil
		})
	}
	if freeBytes != nil {
		if free, err := freeBytes(pgArchiveRoot); err == nil && free < need+64<<20 {
			return "", fmt.Errorf("%s has %d MB free; the archive needs up to %d MB", pgArchiveRoot, free>>20, (need>>20)+64)
		}
	}
	final := filepath.Join(pgArchiveRoot, fmt.Sprintf("postgresql-%s-%s-%s.tar.gz", cluster.Version, cluster.Name, pgLeftoverNow().UTC().Format("20060102T150405Z")))
	temp, err := os.CreateTemp(pgArchiveRoot, ".postgresql-archive-*")
	if err != nil {
		return "", fmt.Errorf("the archive could not be started: %v", err)
	}
	tempPath := temp.Name()
	fail := func(err error) (string, error) {
		temp.Close()
		os.Remove(tempPath)
		return "", err
	}
	if err := temp.Chmod(0o600); err != nil {
		return fail(err)
	}
	zipped := gzip.NewWriter(temp)
	writer := tar.NewWriter(zipped)
	written := 0
	for _, source := range sources {
		err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			link := ""
			if info.Mode()&fs.ModeSymlink != 0 {
				if link, err = os.Readlink(path); err != nil {
					return err
				}
			}
			header, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			header.Name = strings.TrimPrefix(filepath.ToSlash(path), "/")
			if info.IsDir() {
				header.Name += "/"
			}
			if err := writer.WriteHeader(header); err != nil {
				return err
			}
			written++
			if info.Mode().IsRegular() {
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				_, err = io.CopyN(writer, file, info.Size())
				file.Close()
				if err != nil {
					return fmt.Errorf("%s: %v", path, err)
				}
			}
			return nil
		})
		if err != nil {
			return fail(fmt.Errorf("the archive could not be written: %v", err))
		}
	}
	if err := writer.Close(); err != nil {
		return fail(err)
	}
	if err := zipped.Close(); err != nil {
		return fail(err)
	}
	if err := temp.Sync(); err != nil {
		return fail(err)
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return "", err
	}
	if read, err := countTarEntries(tempPath); err != nil || read != written {
		os.Remove(tempPath)
		return "", fmt.Errorf("the archive could not be read back (%d of %d entries): %v", read, written, err)
	}
	if err := os.Rename(tempPath, final); err != nil {
		os.Remove(tempPath)
		return "", err
	}
	return final, nil
}

func countTarEntries(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	zipped, err := gzip.NewReader(file)
	if err != nil {
		return 0, err
	}
	reader := tar.NewReader(zipped)
	count := 0
	for {
		if _, err := reader.Next(); err == io.EOF {
			return count, nil
		} else if err != nil {
			return count, err
		}
		count++
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return count, err
		}
	}
}

// pgLeftoverClusters reads the clusters and classifies every leftover.
func pgLeftoverClusters(ctx context.Context, host pgClusterHost) ([]transport.PostgreSQLCluster, error) {
	if host.family() != "apt" {
		return nil, nil
	}
	readCtx, cancel := context.WithTimeout(ctx, pgReadBound)
	out, err := host.lsclusters(readCtx)
	cancel()
	if errors.Is(err, errPGLsclustersMissing) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pg_lsclusters could not be read: %s", boundedText(err.Error(), 160))
	}
	var leftovers []transport.PostgreSQLCluster
	for _, cluster := range parsePGLsClusters(out) {
		if pgClusterIsLeftover(cluster) {
			cluster.Leftover = classifyPGLeftover(cluster)
			leftovers = append(leftovers, cluster)
		}
	}
	return leftovers, nil
}

func pgLeftoverRemoval(cluster transport.PostgreSQLCluster) transport.PostgreSQLLeftoverRemoval {
	removal := transport.PostgreSQLLeftoverRemoval{Version: cluster.Version, Name: cluster.Name, Port: cluster.Port, DataDirectory: cluster.DataDirectory, Removed: []string{}}
	if cluster.Leftover != nil {
		removal.DataBytes = cluster.Leftover.DataBytes
	}
	return removal
}

// cleanPGLeftovers removes every leftover cluster that holds only what
// initdb made and names the others.
func cleanPGLeftovers(ctx context.Context, host pgClusterHost, resp *transport.PostgreSQLLeftoverCleanupResponse) {
	resp.Removed, resp.Kept = []transport.PostgreSQLLeftoverRemoval{}, []transport.PostgreSQLCluster{}
	leftovers, err := pgLeftoverClusters(ctx, host)
	if err != nil {
		resp.Error = err.Error() + "; nothing was removed"
		return
	}
	for _, cluster := range leftovers {
		if cluster.Leftover == nil || !cluster.Leftover.Fresh {
			resp.Kept = append(resp.Kept, cluster)
			continue
		}
		removal := pgLeftoverRemoval(cluster)
		removed, err := removePGLeftoverFiles(cluster)
		removal.Removed = append(removal.Removed, removed...)
		if len(removal.Removed) > 0 {
			resp.Removed = append(resp.Removed, removal)
		}
		if err != nil {
			resp.Error = fmt.Sprintf("cluster %s: %v", pgClusterRef(cluster.Version, cluster.Name), err)
			return
		}
	}
}

// PostgreSQLLeftoverCleanupV1 removes every leftover cluster that holds only
// what initdb made, in its own pg_leftover_cleanup job.
func (a *Agent) PostgreSQLLeftoverCleanupV1(req *transport.PostgreSQLLeftoverCleanupRequest, resp *transport.PostgreSQLLeftoverCleanupResponse) error {
	*resp = transport.PostgreSQLLeftoverCleanupResponse{Removed: []transport.PostgreSQLLeftoverRemoval{}, Kept: []transport.PostgreSQLCluster{}}
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepCleanPGLeftovers, pgLeftoversMutationTarget, "", "clean"),
	)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	defer finishStep()
	cleanPGLeftovers(ctx, pgClusterHostActive(), resp)
	return nil
}

const pgLeftoversMutationTarget = "pg-leftovers"

// removePGLeftover archives one leftover cluster and removes it.
func removePGLeftover(ctx context.Context, host pgClusterHost, req *transport.PostgreSQLLeftoverRemoveRequest, resp *transport.PostgreSQLLeftoverRemoveResponse) {
	ref := pgClusterRef(req.Version, req.Name)
	leftovers, err := pgLeftoverClusters(ctx, host)
	if err != nil {
		resp.ErrorCode, resp.Error = "CLUSTERS_UNREADABLE", err.Error()+"; nothing was changed"
		return
	}
	var cluster *transport.PostgreSQLCluster
	for i := range leftovers {
		if leftovers[i].Version == req.Version && leftovers[i].Name == req.Name {
			cluster = &leftovers[i]
		}
	}
	if cluster == nil {
		resp.ErrorCode, resp.Error = "NOT_A_LEFTOVER", fmt.Sprintf("cluster %s is not a removed PostgreSQL's cluster on this server (it runs, its program is installed, or it is gone already); nothing was changed", ref)
		return
	}
	resp.Removal = pgLeftoverRemoval(*cluster)
	standard := pgStandardDataDirectory(cluster.Version, cluster.Name)
	if cluster.DataDirectory != "" && filepath.Clean(cluster.DataDirectory) != standard {
		resp.ErrorCode = "DATA_OUTSIDE_STANDARD"
		resp.Error = fmt.Sprintf("cluster %s keeps its data in %s, outside %s; CelikPanel does not remove data it did not place. Keep or remove that directory yourself; nothing was changed", ref, cluster.DataDirectory, standard)
		return
	}
	archive, err := archivePGLeftover(*cluster, host.freeBytes)
	if err != nil {
		resp.ErrorCode, resp.Error = "ARCHIVE_FAILED", fmt.Sprintf("cluster %s could not be archived (%v); nothing was removed", ref, err)
		return
	}
	resp.Removal.Archive = archive
	resp.Changed = true
	removed, err := removePGLeftoverFiles(*cluster)
	resp.Removal.Removed = append(resp.Removal.Removed, removed...)
	if err != nil {
		resp.ErrorCode, resp.Error = "REMOVE_FAILED", fmt.Sprintf("cluster %s was archived in %s, but removing it stopped: %v; remove the rest yourself or try again", ref, archive, err)
		return
	}
	resp.Removed = true
}

// RemovePostgreSQLLeftoverV1 archives one leftover cluster under
// /var/backups/celikpanel and removes it (the owner's explicit choice), in
// its own pg_leftover_remove job.
func (a *Agent) RemovePostgreSQLLeftoverV1(req *transport.PostgreSQLLeftoverRemoveRequest, resp *transport.PostgreSQLLeftoverRemoveResponse) error {
	*resp = transport.PostgreSQLLeftoverRemoveResponse{}
	if req == nil || !validPGClusterRef(req.Version, req.Name) {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "the cluster is not named correctly"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepRemovePGLeftover, pgClusterTarget(req.Version, req.Name), "", "remove"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = "MUTATION_REFUSED", err.Error()
		return nil
	}
	defer finishStep()
	removePGLeftover(ctx, pgClusterHostActive(), req, resp)
	return nil
}

const errCodeRemovalArchiveFailed = "REMOVAL_ARCHIVE_FAILED"

var pgClientVersionPackage = regexp.MustCompile(`^postgresql-([0-9]{1,3})$`)

// pgVersionClientPackages are the installed postgresql-client-NN of the
// majors being removed (alpha.130): Boston kept postgresql-client-16 after
// PostgreSQL 16 was removed, because nothing named it.
func pgVersionClientPackages(svc *core.ManagedService, packages []string, installed func(string) bool) []string {
	if svc == nil || svc.ID != "postgresql" || installed == nil {
		return nil
	}
	var out []string
	for _, pkg := range packages {
		if m := pgClientVersionPackage.FindStringSubmatch(pkg); m != nil {
			if client := "postgresql-client-" + m[1]; installed(client) {
				out = append(out, client)
			}
		}
	}
	return out
}

// pgMovingPackages are the distribution's "default major" packages
// (postgresql-client, postgresql-contrib) installed beside the component's
// own: a removal of the whole component takes them too.
func pgMovingPackages(svc *core.ManagedService, installed func(string) bool) []string {
	if svc == nil || svc.ID != "postgresql" || svc.Repo == nil || installed == nil {
		return nil
	}
	pattern, err := regexp.Compile(svc.Repo.MovingPackagePattern)
	if err != nil {
		return nil
	}
	var out []string
	for _, name := range []string{"postgresql", "postgresql-client", "postgresql-contrib"} {
		if pattern.MatchString(name) && installed(name) {
			out = append(out, name)
		}
	}
	return out
}

// pgClustersOfMajor are the clusters of one major ("" = every major).
func pgClustersOfMajor(ctx context.Context, host pgClusterHost, major string) ([]transport.PostgreSQLCluster, error) {
	readCtx, cancel := context.WithTimeout(ctx, pgReadBound)
	out, err := host.lsclusters(readCtx)
	cancel()
	if errors.Is(err, errPGLsclustersMissing) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pg_lsclusters: %s", boundedText(err.Error(), 160))
	}
	var clusters []transport.PostgreSQLCluster
	for _, cluster := range parsePGLsClusters(out) {
		if major == "" || cluster.Version == major {
			clusters = append(clusters, cluster)
		}
	}
	return clusters, nil
}

// preparePGClusterRemoval reads a stopped cluster's files: one that holds
// only what initdb made needs no archive; any other is archived first.
func preparePGClusterRemoval(cluster transport.PostgreSQLCluster, freeBytes func(string) (int64, error)) (string, error) {
	if leftover := classifyPGLeftover(cluster); leftover.Fresh {
		return "", nil
	}
	return archivePGLeftover(cluster, freeBytes)
}
