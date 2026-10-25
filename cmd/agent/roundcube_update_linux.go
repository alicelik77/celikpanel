//go:build linux

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/transport"
)

// roundcubeBackupDir keeps the archives of the trees an update replaced
// (the last two).
var roundcubeBackupDir = "/var/lib/celikpanel-agent-private/roundcube-backups"

const (
	roundcubeUpdateStepBound = 5 * time.Minute
	roundcubeDBRetries       = 3
	roundcubeBackupsKept     = 2
)

type roundcubeUpdateOps struct {
	baseDir     string
	check       func(ctx context.Context) transport.RoundcubeUpdateCheckResponse
	download    func(ctx context.Context, url string, limit int64, w io.Writer) error
	verify      func(archive io.Reader, signature []byte) error
	extract     func(ctx context.Context, archive, dir string) error
	updateDB    func(ctx context.Context, stage, from string) error
	permissions func(ctx context.Context, stage string) error
	// prepareProof puts CelikPanel's PHP check (script and nginx location)
	// in place before anything changes (alpha.109); nil in tests.
	prepareProof func(ctx context.Context) error
	backup       func(ctx context.Context, tree, from string) (string, error)
	exchange     func(a, b string) error
	prove        func(ctx context.Context) error
	retire       func(path string) error
}

func hostRoundcubeUpdateOps() roundcubeUpdateOps {
	return roundcubeUpdateOps{
		baseDir: webmailBaseDir,
		check: func(ctx context.Context) transport.RoundcubeUpdateCheckResponse {
			return roundcubeUpdateCheck(ctx, true)
		},
		download: func(ctx context.Context, url string, limit int64, w io.Writer) error {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return err
			}
			response, err := (&http.Client{Timeout: 5 * time.Minute}).Do(request)
			if err != nil {
				return err
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("HTTP %d", response.StatusCode)
			}
			written, err := io.Copy(w, io.LimitReader(response.Body, limit+1))
			if err == nil && written > limit {
				err = fmt.Errorf("larger than %d bytes", limit)
			}
			return err
		},
		verify: func(archive io.Reader, signature []byte) error {
			return verifyRoundcubeSignature(archive, signature, roundcubeSigningKey, roundcubeSigningPrimary)
		},
		extract: func(ctx context.Context, archive, dir string) error {
			if out, err := serviceMutationCommand(ctx, "tar", "-xzf", archive, "-C", dir, "--strip-components=1").CombinedOutput(); err != nil {
				return fmt.Errorf("%v: %s", err, firstLine(string(out)))
			}
			return nil
		},
		updateDB:    runRoundcubeDBUpdate,
		permissions: setRoundcubeUpdatePermissions,
		prepareProof: func(ctx context.Context) error {
			return preparePHPToolProof(ctx, phpToolWebmail)
		},
		backup: backupRoundcubeTree,
		exchange: func(a, b string) error {
			if err := unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE); err != nil {
				return err
			}
			return roundcubeSyncParent(filepath.Dir(b))
		},
		prove: func(ctx context.Context) error {
			version := phpToolStateFor(phpToolWebmail).version()
			return retryPHPToolProbe(ctx, phpToolWebmail, version, probePHPToolHTTP, phpToolProbeSettle, time.Second)
		},
		retire: func(path string) error {
			_, err := retireRoundcubeTree(path)
			return err
		},
	}
}

func roundcubeFileSHA256(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func copyTreeEntry(ctx context.Context, from, to string) error {
	if out, err := serviceMutationCommand(ctx, "cp", "-a", "--", from, to).CombinedOutput(); err != nil {
		return fmt.Errorf("copy %s: %v: %s", from, err, firstLine(string(out)))
	}
	return nil
}

// carryRoundcubeOwnerFiles copies into the new tree what is the server's,
// not Roundcube's: the configuration (config.inc.php, and any other file in
// config/ the new release does not ship), plugins and skins the release does
// not ship (the owner's additions), and the database directory.
func carryRoundcubeOwnerFiles(ctx context.Context, live, stage string) error {
	entries, err := os.ReadDir(filepath.Join(live, "config"))
	if err != nil {
		return fmt.Errorf("read the configuration: %w", err)
	}
	for _, entry := range entries {
		target := filepath.Join(stage, "config", entry.Name())
		if entry.Name() != "config.inc.php" && pathExists(target) {
			continue
		}
		_ = os.RemoveAll(target)
		if err := copyTreeEntry(ctx, filepath.Join(live, "config", entry.Name()), target); err != nil {
			return err
		}
	}
	for _, dir := range []string{"plugins", "skins"} {
		entries, err := os.ReadDir(filepath.Join(live, dir))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			target := filepath.Join(stage, dir, entry.Name())
			if pathExists(target) {
				continue
			}
			if err := copyTreeEntry(ctx, filepath.Join(live, dir, entry.Name()), target); err != nil {
				return err
			}
		}
	}
	return copyRoundcubeDB(ctx, live, stage)
}

func copyRoundcubeDB(ctx context.Context, live, stage string) error {
	target := filepath.Join(stage, "db")
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return copyTreeEntry(ctx, filepath.Join(live, "db"), target)
}

// runRoundcubeDBUpdate runs Roundcube's own bin/updatedb.sh on the copied
// database: the stage's configuration is pointed at the stage's database for
// that one run, then put back byte for byte.
func runRoundcubeDBUpdate(ctx context.Context, stage, from string) error {
	configPath := filepath.Join(stage, "config", "config.inc.php")
	original, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(configPath)
	if err != nil {
		return err
	}
	override := fmt.Sprintf("%s\n$config['db_dsnw'] = 'sqlite:///%s?mode=0664';\n", strings.TrimRight(string(original), "\n"), filepath.Join(stage, "db", "roundcube.sqlite3"))
	if err := os.WriteFile(configPath, []byte(override), info.Mode().Perm()); err != nil {
		return err
	}
	defer func() { _ = os.WriteFile(configPath, original, info.Mode().Perm()) }()
	php := "php"
	if version := phpToolStateFor(phpToolWebmail).version(); version != "" {
		if path, err := exec.LookPath("php" + version); err == nil {
			php = path
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, roundcubeUpdateStepBound)
	defer cancel()
	out, err := serviceMutationCommand(runCtx, php, filepath.Join(stage, "bin", "updatedb.sh"),
		"--package=roundcube", "--version="+from, "--dir="+filepath.Join(stage, "SQL")).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Roundcube's database update failed: %v: %s", err, lastLines(string(out), 3))
	}
	if after, err := os.ReadFile(configPath); err == nil && string(after) != override {
		return fmt.Errorf("the configuration changed while the database was updated")
	}
	return nil
}

func setRoundcubeUpdatePermissions(ctx context.Context, stage string) error {
	group := webServerGroup()
	if group == "" {
		return fmt.Errorf("web server group could not be detected")
	}
	return applyRoundcubePermissions(ctx, stage, filepath.Join(stage, "db", "roundcube.sqlite3"), group, runServiceMutationCombinedOutput)
}

// backupRoundcubeTree archives the tree (code, configuration and database)
// before the swap; the last two archives are kept.
func backupRoundcubeTree(ctx context.Context, tree, from string) (string, error) {
	if err := os.MkdirAll(roundcubeBackupDir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("roundcube-%s-%s.tar.gz", from, time.Now().UTC().Format("20060102T150405Z"))
	path := filepath.Join(roundcubeBackupDir, name)
	runCtx, cancel := context.WithTimeout(ctx, roundcubeUpdateStepBound)
	defer cancel()
	if out, err := serviceMutationCommand(runCtx, "tar", "-czf", path, "-C", filepath.Dir(tree), filepath.Base(tree)).CombinedOutput(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("%v: %s", err, firstLine(string(out)))
	}
	_ = os.Chmod(path, 0o600)
	entries, _ := os.ReadDir(roundcubeBackupDir)
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "roundcube-") && strings.HasSuffix(entry.Name(), ".tar.gz") {
			names = append(names, entry.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		a, _ := os.Stat(filepath.Join(roundcubeBackupDir, names[i]))
		b, _ := os.Stat(filepath.Join(roundcubeBackupDir, names[j]))
		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})
	for i, old := range names {
		if i >= roundcubeBackupsKept && old != name {
			_ = os.Remove(filepath.Join(roundcubeBackupDir, old))
		}
	}
	return path, nil
}

// updateRoundcubeTo is the update under its held mutation step.
func updateRoundcubeTo(ctx context.Context, version string, ops roundcubeUpdateOps) (resp transport.RoundcubeUpdateResponse) {
	resp.To = version
	fail := func(code string, err error) transport.RoundcubeUpdateResponse {
		resp.ErrorCode, resp.Error = code, err.Error()
		return resp
	}
	if err := ensureRoundcubeLifecycleSupported(); err != nil {
		return fail("UNSUPPORTED", err)
	}
	check := ops.check(ctx)
	if !check.Installed {
		return fail("NOT_INSTALLED", errors.New("Roundcube is not installed"))
	}
	resp.From = check.Version
	if check.Error != "" {
		return fail("CHECK_FAILED", errors.New(check.Error))
	}
	if check.LatestOnLine != version || !roundcubeVersionLess(check.Version, version) {
		return fail("NOT_LATEST", fmt.Errorf("the newest release on the %s line is %s, not %s; nothing was changed", check.Line, check.LatestOnLine, version))
	}
	// alpha.109 (Frankfurt): a webmail configuration written by an older
	// release had no PHP check location, so the proof after the swap read
	// HTTP 404 and a working Roundcube was rolled back. The check is put in
	// place first; an owner-edited configuration stops the update here,
	// before anything of Roundcube changes.
	// Kanıt denetimi önce yerine konur; sahibin değiştirdiği yapılandırma
	// güncellemeyi burada, hiçbir şey değişmeden durdurur.
	if ops.prepareProof != nil {
		if err := ops.prepareProof(ctx); err != nil {
			var edited *errPHPToolConfigOwnerEdited
			if errors.As(err, &edited) {
				return fail("CONFIG_OWNER_EDITED", fmt.Errorf("%v; Roundcube was not changed", err))
			}
			return fail("PROOF_PREPARE_FAILED", fmt.Errorf("CelikPanel's PHP check for webmail could not be put in place (%v); Roundcube was not changed", err))
		}
	}
	live := ops.baseDir
	if err := reconcileRoundcubeArtifacts(live, ""); err != nil {
		return fail("STAGE_FAILED", err)
	}
	stage, err := createRoundcubeInstallStage(live)
	if err != nil {
		return fail("STAGE_FAILED", err)
	}
	discardStage := true
	defer func() {
		if discardStage {
			if err := ops.retire(stage); err != nil && resp.Error != "" {
				resp.Error += "; the staging tree could not be cleaned: " + err.Error()
			}
		}
	}()
	archive, err := os.CreateTemp(stage, "rc-dl-*")
	if err != nil {
		return fail("DOWNLOAD_FAILED", err)
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)
	if err := ops.download(ctx, roundcubeReleaseURL(version, ""), roundcubeMaxSize, archive); err != nil {
		archive.Close()
		return fail("DOWNLOAD_FAILED", fmt.Errorf("the release could not be downloaded; nothing was changed: %v", err))
	}
	var signature strings.Builder
	if err := ops.download(ctx, roundcubeReleaseURL(version, ".asc"), roundcubeSignatureMax, &signature); err != nil {
		archive.Close()
		return fail("DOWNLOAD_FAILED", fmt.Errorf("the release's signature could not be downloaded; nothing was changed: %v", err))
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		archive.Close()
		return fail("DOWNLOAD_FAILED", err)
	}
	verifyErr := ops.verify(archive, []byte(signature.String()))
	archive.Close()
	if verifyErr != nil {
		return fail("SIGNATURE_INVALID", fmt.Errorf("%v; the download was discarded and nothing was changed", verifyErr))
	}
	if err := ops.extract(ctx, archivePath, stage); err != nil {
		return fail("EXTRACT_FAILED", err)
	}
	_ = os.Remove(archivePath)
	if got := roundcubeTreeVersion(stage); got != version {
		return fail("EXTRACT_FAILED", fmt.Errorf("the archive says it is Roundcube %q, not %s; nothing was changed", got, version))
	}
	if err := carryRoundcubeOwnerFiles(ctx, live, stage); err != nil {
		return fail("CARRY_FAILED", fmt.Errorf("%v; nothing was changed", err))
	}
	liveDB := filepath.Join(live, "db", "roundcube.sqlite3")
	backedUp := false
	for attempt := 1; ; attempt++ {
		before := roundcubeFileSHA256(liveDB)
		if err := ops.updateDB(ctx, stage, resp.From); err != nil {
			return fail("DB_UPDATE_FAILED", fmt.Errorf("%v; nothing was changed", err))
		}
		if err := ops.permissions(ctx, stage); err != nil {
			return fail("PERMISSIONS_FAILED", fmt.Errorf("%v; nothing was changed", err))
		}
		if !backedUp {
			backup, err := ops.backup(ctx, live, resp.From)
			if err != nil {
				return fail("BACKUP_FAILED", fmt.Errorf("the current webmail could not be backed up, so nothing was changed: %v", err))
			}
			resp.Backup, backedUp = backup, true
		}
		// Anything webmail wrote to its database meanwhile (a contact, a
		// setting) is carried by copying it again.
		if roundcubeFileSHA256(liveDB) == before {
			break
		}
		if attempt == roundcubeDBRetries {
			return fail("DB_CHANGING", errors.New("webmail kept writing to its database while the update was prepared; nothing was changed. Try again at a quieter moment"))
		}
		if err := copyRoundcubeDB(ctx, live, stage); err != nil {
			return fail("CARRY_FAILED", err)
		}
	}
	if err := ops.exchange(stage, live); err != nil {
		return fail("SWAP_FAILED", fmt.Errorf("the new tree could not be put in place; nothing was changed: %v", err))
	}
	// From here the stage path holds the previous tree, byte for byte.
	proofErr := ops.prove(ctx)
	if proofErr == nil {
		if got := roundcubeTreeVersion(live); got != version {
			proofErr = fmt.Errorf("the live tree says it is Roundcube %q", got)
		}
	}
	if proofErr == nil {
		resp.Updated, resp.Proved = true, true
		discardStage = true // the previous tree goes; its archive is the backup
		return resp
	}
	if err := ops.exchange(stage, live); err != nil {
		discardStage = false
		return fail("ROLLBACK_FAILED", fmt.Errorf("webmail did not answer on %s (%v), and the previous tree could not be put back (%v); it is kept at %s and archived in %s", version, proofErr, err, stage, resp.Backup))
	}
	resp.RolledBack = true
	if err := ops.prove(ctx); err != nil {
		return fail("PROOF_FAILED", fmt.Errorf("webmail did not answer on %s (%v); the previous %s tree was put back byte for byte, and it does not answer either: %v", version, proofErr, resp.From, err))
	}
	return fail("PROOF_FAILED", fmt.Errorf("webmail did not answer on %s (%v); the previous %s tree was put back byte for byte and answers again", version, proofErr, resp.From))
}
