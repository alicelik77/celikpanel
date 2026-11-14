package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Replaced website lineages are removed with certbot's own means (alpha.134).
//
// Every certificate request makes a new native lineage (D-040), so after
// "Cover the mail name too", "Get it again", an alias change or a detach the
// lineage that was served before stays in /etc/letsencrypt. Until alpha.133
// the Panel only switched its renewal off; certbot kept its files, its
// renewal configuration and its deploy record. Now the Panel asks, after the
// new certificate is installed and served and again at every periodic pass,
// and the Agent runs `certbot delete` for a replaced lineage only when:
//
//   - it is a native lineage of this domain and not the active one;
//   - its renewal configuration is still CelikPanel's (the same rule the
//     certificate page uses for "changed outside CelikPanel");
//   - no file under /etc other than certbot's own store names it (nginx,
//     Postfix, Dovecot, another site, anything the owner wired by hand).
//
// Anything it cannot establish keeps the lineage and says so; the next pass
// asks again. The active certificate is never touched.
//
// Değiştirilen web sitesi lineage'ları certbot'un kendi silme komutuyla
// kaldırılır; yalnız CelikPanel'in olan, sahibinin değiştirmediği ve /etc
// altında hiçbir dosyanın anmadığı lineage. Kurulamayan her şey korunur.

var (
	nativeSiteRetireScanRoot    = "/etc"
	nativeSiteRetireMaxFiles    = 50000
	nativeSiteRetireChunkBytes  = 1 << 20
	nativeSiteRetireMaxLineages = 100
	// The update and rescue snapshots update.sh keeps (each holds the panel
	// database of the moment before an update).
	nativeSiteRetireSnapshotRoots = []string{"/var/backups/celikpanel/update-snapshots", "/var/backups/celikpanel/recovery-snapshots"}
)

// RetireSiteCertificateLineagesV1 removes the replaced native lineages of
// one domain that nothing names any more.
func (a *Agent) RetireSiteCertificateLineagesV1(req *transport.RetireSiteCertificateLineagesRequest, resp *transport.RetireSiteCertificateLineagesResponse) error {
	if req == nil {
		resp.Error = "lineage retirement request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "removing replaced certificate lineages"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if len(req.Lineages) == 0 || len(req.Lineages) > nativeSiteRetireMaxLineages {
		resp.Error = "between one and 100 replaced lineages are required"
		return nil
	}
	active := strings.TrimSpace(req.ActiveLineage)
	if active != "" && !transport.NativeSiteLineageBelongsTo(active, req.DomainID) {
		resp.Error = "the active certificate lineage does not belong to this domain"
		return nil
	}
	for _, lineage := range req.Lineages {
		name := strings.TrimSpace(lineage.LineageName)
		if !transport.NativeSiteLineageBelongsTo(name, req.DomainID) {
			resp.Error = "a replaced certificate lineage does not belong to this domain"
			return nil
		}
		if name == active {
			resp.Error = "the active certificate lineage cannot be removed"
			return nil
		}
	}
	if !acquireSiteCertbot() {
		resp.Error = transport.SiteCertificateBusy
		return nil
	}
	defer releaseSiteCertbot()
	// Inside the Panel's three minutes for a certificate mutation.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	resp.Results = retireNativeSiteLineages(ctx, *req)
	return nil
}

func retireNativeSiteLineages(ctx context.Context, req transport.RetireSiteCertificateLineagesRequest) []transport.RetireSiteLineageResult {
	results := make([]transport.RetireSiteLineageResult, 0, len(req.Lineages))
	keep := func(name, outcome, detail string) {
		results = append(results, transport.RetireSiteLineageResult{LineageName: name, Outcome: outcome, Detail: boundedNativeDetail(detail)})
	}
	expectWebroot, webrootErr := hostingpath.ACMEChallengeRoot(req.SubscriptionID, req.DomainID)
	// The server certbot recorded for CelikPanel's latest request of this
	// domain is CelikPanel's own (a certbot cli.ini, the lab's Pebble).
	activeServer := ""
	if active := strings.TrimSpace(req.ActiveLineage); active != "" {
		if content, err := readBounded(nativeSiteRenewalConfPath(active), nativeSiteMaxConfBytes); err == nil {
			if conf, err := parseNativeSiteRenewalConf(content); err == nil {
				activeServer = conf.Server
			}
		}
	}
	var candidates []string
	seen := map[string]bool{}
	for _, lineage := range req.Lineages {
		name := strings.TrimSpace(lineage.LineageName)
		if seen[name] {
			continue
		}
		seen[name] = true
		content, err := readBounded(nativeSiteRenewalConfPath(name), nativeSiteMaxConfBytes)
		if errors.Is(err, os.ErrNotExist) {
			// certbot no longer knows it: nothing renews it.
			keep(name, transport.SiteLineageAbsent, "")
			continue
		}
		if err != nil {
			keep(name, transport.SiteLineageNotChecked, "its renewal configuration could not be read: "+err.Error())
			continue
		}
		conf, err := parseNativeSiteRenewalConf(content)
		if err != nil {
			keep(name, transport.SiteLineageNotChecked, "its renewal configuration could not be read: "+err.Error())
			continue
		}
		if webrootErr != nil {
			keep(name, transport.SiteLineageNotChecked, "this domain's validation directory could not be derived: "+webrootErr.Error())
			continue
		}
		expectServer := lineage.ACMEServer
		if expectServer == "" {
			expectServer = letsEncryptDefaultDirectory
		}
		if nativeSiteConfOwnerChanged(conf, expectWebroot, expectServer, activeServer) {
			keep(name, transport.SiteLineageOwnerChanged, "certbot's renewal configuration of this lineage was changed outside CelikPanel; it is kept as it is")
			continue
		}
		candidates = append(candidates, name)
	}
	if len(candidates) == 0 {
		return results
	}
	references, err := nativeSiteLineageReferences(candidates)
	if err != nil {
		for _, name := range candidates {
			keep(name, transport.SiteLineageNotChecked, "what names it could not be established: "+err.Error())
		}
		return results
	}
	// A retained update snapshot whose panel database still serves the
	// lineage is a way back to it (D-025: going back is that snapshot's
	// restore); the lineage stays until the snapshot is gone.
	held, err := nativeSiteLineagesInSnapshots(ctx, candidates)
	if err != nil {
		for _, name := range candidates {
			if _, named := references[name]; !named {
				keep(name, transport.SiteLineageNotChecked, "the update snapshots could not be read: "+err.Error())
			}
		}
	}
	for _, name := range candidates {
		if file, named := references[name]; named {
			// Detail is the file alone: the page says the sentence.
			keep(name, transport.SiteLineageInUse, file)
			continue
		}
		if err != nil {
			continue
		}
		if snapshot, kept := held[name]; kept {
			keep(name, transport.SiteLineageHeldBySnapshot, snapshot)
			continue
		}
		args := []string{"delete", "--config-dir", nativeSiteConfigDir, "--cert-name", name, "--non-interactive"}
		if output, err := nativeSiteCertbotRun(ctx, args...); err != nil {
			keep(name, transport.SiteLineageFailed, "certbot delete: "+commandFailureDetail("certbot delete", output, err))
			continue
		}
		// The deploy hook's record of the lineage goes with it.
		if err := os.Remove(filepath.Join(nativeSiteStatusDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			keep(name, transport.SiteLineageRetired, "certbot removed it; its deploy record could not be removed: "+err.Error())
			continue
		}
		keep(name, transport.SiteLineageRetired, "")
	}
	return results
}

// nativeSiteLineageReferences reads every regular file under /etc, except
// certbot's own store, and returns the first file that names each lineage.
// A lineage name carries 24 random hexadecimal digits, so a file that
// contains it names that lineage. Bounded; an unreadable directory or file,
// or more files than the bound, is an error: nothing is concluded.
func nativeSiteLineageReferences(lineages []string) (map[string]string, error) {
	found := map[string]string{}
	skip := filepath.Clean(nativeSiteConfigDir)
	files := 0
	err := filepath.WalkDir(nativeSiteRetireScanRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if filepath.Clean(path) == skip {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			// A link names a lineage by what it points to, as written or
			// resolved: /etc/ssl/private/site.pem -> /etc/letsencrypt/live/<l>/privkey.pem
			// is not reached by the walk, which skips certbot's store.
			return linkNamesLineages(path, lineages, found)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		files++
		if files > nativeSiteRetireMaxFiles {
			return fmt.Errorf("more than %d files under %s", nativeSiteRetireMaxFiles, nativeSiteRetireScanRoot)
		}
		return scanFileForLineages(path, lineages, found)
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// linkNamesLineages records the link when its target, as written or
// resolved, names a lineage. A link that cannot be read is an error (nothing
// is concluded); a link whose target does not exist names only what its text
// names.
func linkNamesLineages(path string, lineages []string, found map[string]string) error {
	target, err := os.Readlink(path)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, lineage := range lineages {
		if _, named := found[lineage]; named {
			continue
		}
		if strings.Contains(target, lineage) || strings.Contains(resolved, lineage) {
			found[lineage] = path
		}
	}
	return nil
}

// nativeSiteLineagesInSnapshots returns, for each lineage that the panel
// database of a retained update or rescue snapshot names as an active
// certificate, that snapshot's directory. Read only (immutable). A snapshot
// database that predates the certificate ledger's lineage column cannot name
// a native lineage; any other unreadable snapshot is an error.
func nativeSiteLineagesInSnapshots(ctx context.Context, lineages []string) (map[string]string, error) {
	held := map[string]string{}
	for _, root := range nativeSiteRetireSnapshotRoots {
		entries, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				// A dot-named directory is a snapshot still being written,
				// never a restore point.
				continue
			}
			database := filepath.Join(root, entry.Name(), "celikpanel.db")
			info, err := os.Lstat(database)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("%s is not a regular file", database)
			}
			named, err := snapshotActiveLineages(ctx, database, lineages)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", database, err)
			}
			for _, lineage := range named {
				if _, kept := held[lineage]; !kept {
					held[lineage] = filepath.Join(root, entry.Name())
				}
			}
		}
	}
	return held, nil
}

func snapshotActiveLineages(ctx context.Context, database string, lineages []string) ([]string, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(database)+"?mode=ro&immutable=1")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var named []string
	for _, lineage := range lineages {
		var count int
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ssl_certificates WHERE status = 'active' AND lineage_name = ?`, lineage).Scan(&count)
		if err != nil {
			text := err.Error()
			if strings.Contains(text, "no such column: lineage_name") || strings.Contains(text, "no such table: ssl_certificates") {
				return nil, nil
			}
			return nil, err
		}
		if count > 0 {
			named = append(named, lineage)
		}
	}
	return named, nil
}

// scanFileForLineages reads one file in bounded pieces (a large file is read
// whole, never skipped), keeping enough of each piece to find a name that
// spans two of them.
func scanFileForLineages(path string, lineages []string, found map[string]string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	longest := 0
	for _, lineage := range lineages {
		longest = max(longest, len(lineage))
	}
	buffer := make([]byte, 0, nativeSiteRetireChunkBytes+longest)
	chunk := make([]byte, nativeSiteRetireChunkBytes)
	for {
		n, readErr := file.Read(chunk)
		buffer = append(buffer, chunk[:n]...)
		for _, lineage := range lineages {
			if _, named := found[lineage]; !named && bytes.Contains(buffer, []byte(lineage)) {
				found[lineage] = path
			}
		}
		if len(buffer) > longest {
			buffer = append(buffer[:0], buffer[len(buffer)-longest:]...)
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
