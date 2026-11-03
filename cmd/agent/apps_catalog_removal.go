package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// A catalog application's removal is recorded in a root-private file before
// its first file is deleted, and the record is cleared when the document root
// is empty again. A removal that stopped part way (a folder the application
// had made read-only, a file of another owner) has usually deleted the files
// that identify the application, and without the record the next attempt
// would refuse the document root as "something else". The record names the
// site, the application and the document root it was started for, and the
// names (only the names) of the document root's top-level entries at that
// moment: a continued removal deletes only those, so whatever the owner put
// there after the removal started stays (D-022). It holds no secret.
//
// Uygulama kaldırma, ilk dosya silinmeden önce köke özel bir kayda yazılır ve
// belge kökü boşalınca silinir; yarıda kalan kaldırma bu kayıtla sürdürülür.

var catalogRemovalStateDir = "/var/lib/celikpanel-agent-private/app-removals"

type catalogRemovalRecord struct {
	SiteID  int    `json:"site_id"`
	AppID   string `json:"app_id"`
	Domain  string `json:"domain"`
	DocRoot string `json:"doc_root"`
	// Entries are the top-level names of the document root when the
	// removal started.
	Entries []string `json:"entries"`
}

// maxCatalogRemovalEntries bounds the record (and one command line).
const maxCatalogRemovalEntries = 5000

func catalogRemovalPath(siteID int, site catalogSite) string {
	return filepath.Join(catalogRemovalStateDir, strconv.Itoa(siteID)+"-"+site.app.ID+".json")
}

func catalogRemovalRecordFor(siteID int, site catalogSite) catalogRemovalRecord {
	return catalogRemovalRecord{SiteID: siteID, AppID: site.app.ID, Domain: site.domain, DocRoot: site.docRoot}
}

// loadCatalogRemoval: a removal of exactly this application on exactly this
// site and document root was started and has not finished; its record.
func loadCatalogRemoval(siteID int, site catalogSite) (catalogRemovalRecord, bool) {
	path := catalogRemovalPath(siteID, site)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return catalogRemovalRecord{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return catalogRemovalRecord{}, false
	}
	var record catalogRemovalRecord
	if json.Unmarshal(raw, &record) != nil {
		return catalogRemovalRecord{}, false
	}
	want := catalogRemovalRecordFor(siteID, site)
	if record.SiteID != want.SiteID || record.AppID != want.AppID || record.Domain != want.Domain ||
		record.DocRoot != want.DocRoot || len(record.Entries) == 0 || len(record.Entries) > maxCatalogRemovalEntries {
		return catalogRemovalRecord{}, false
	}
	for _, name := range record.Entries {
		if !validCatalogEntryName(name) {
			return catalogRemovalRecord{}, false
		}
	}
	return record, true
}

func validCatalogEntryName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

// catalogTopLevelNames lists the names of the document root's entries.
func catalogTopLevelNames(docRoot string) ([]string, error) {
	entries, err := os.ReadDir(docRoot)
	if err != nil {
		return nil, err
	}
	if len(entries) > maxCatalogRemovalEntries {
		return nil, fmt.Errorf("the document root holds more than %d entries", maxCatalogRemovalEntries)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !validCatalogEntryName(entry.Name()) {
			return nil, fmt.Errorf("the document root holds an entry with an unusable name")
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// catalogRemovalTargets are the recorded entries that still exist.
func catalogRemovalTargets(docRoot string, recorded []string) []string {
	targets := []string{}
	for _, name := range recorded {
		if !validCatalogEntryName(name) {
			continue
		}
		path := filepath.Join(docRoot, name)
		if _, err := os.Lstat(path); err == nil {
			targets = append(targets, path)
		}
	}
	return targets
}

// catalogKeptEntries are the document root's entries that were not there
// when the removal started (the placeholder page excepted).
func catalogKeptEntries(docRoot string, recorded []string, placeholder string) []string {
	known := map[string]bool{placeholder: true}
	for _, name := range recorded {
		known[name] = true
	}
	entries, err := os.ReadDir(docRoot)
	if err != nil {
		return nil
	}
	kept := []string{}
	for _, entry := range entries {
		if !known[entry.Name()] {
			kept = append(kept, entry.Name())
		}
	}
	sort.Strings(kept)
	return kept
}

// removeCatalogRemovalTargets deletes the given entries of the document root
// as the site's user. Applications make folders read-only for their own
// safety (Drupal's sites/default is r-x after setup), so the site's user
// first gives itself write permission back on its own folders (alpha.118
// lab). find never follows links and stays on one file system.
// Uygulamalar klasörleri salt okunur yapar; silmeden önce sitenin kullanıcısı
// kendi klasörlerine yazma iznini geri verir.
func removeCatalogRemovalTargets(ctx context.Context, site catalogSite, targets []string) error {
	_, _ = catalogRunAsSiteUser(ctx, site.username, site.siteHome, catalogRemovalPermissionArgv(targets...), nil)
	argv := append(append([]string{"find"}, targets...), "-xdev", "-delete")
	output, err := catalogRunAsSiteUser(ctx, site.username, site.siteHome, argv, nil)
	if err != nil {
		if last := lastNonEmptyLine(output); last != "" {
			return fmt.Errorf("%v: %s", err, last)
		}
		return err
	}
	return nil
}

func markCatalogRemoval(siteID int, site catalogSite, record catalogRemovalRecord) error {
	if err := os.MkdirAll(catalogRemovalStateDir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(catalogRemovalStateDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("the removal record folder is not a real directory")
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	path := catalogRemovalPath(siteID, site)
	tmp, err := os.CreateTemp(catalogRemovalStateDir, ".removal-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("record the removal: %w", err)
	}
	return nil
}

func clearCatalogRemoval(siteID int, site catalogSite) {
	_ = os.Remove(catalogRemovalPath(siteID, site))
}

// catalogRemovalPermissionArgv gives the owner (the site's user, who runs
// it) read, write and search permission back on every folder of the document
// root that lacks one. find does not follow links and stays on one file
// system; files need no change (deleting a file needs only its folder).
func catalogRemovalPermissionArgv(paths ...string) []string {
	argv := append([]string{"find"}, paths...)
	return append(argv, "-xdev", "-type", "d", "!", "-perm", "-u=rwx", "-exec", "chmod", "u+rwx", "--", "{}", "+")
}

// lastNonEmptyLine is the last line of a command's output that says
// something (find names the path it could not delete there).
func lastNonEmptyLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}
