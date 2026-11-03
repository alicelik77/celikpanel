package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A catalog application's removal is recorded in a root-private file before
// its first file is deleted, and the record is cleared when the document root
// is empty again. A removal that stopped part way (a folder the application
// had made read-only, a file of another owner) has usually deleted the files
// that identify the application, and without the record the next attempt
// would refuse the document root as "something else". The record names the
// site, the application and the document root it was started for; it holds no
// secret.
//
// Uygulama kaldırma, ilk dosya silinmeden önce köke özel bir kayda yazılır ve
// belge kökü boşalınca silinir; yarıda kalan kaldırma bu kayıtla sürdürülür.

var catalogRemovalStateDir = "/var/lib/celikpanel-agent-private/app-removals"

type catalogRemovalRecord struct {
	SiteID  int    `json:"site_id"`
	AppID   string `json:"app_id"`
	Domain  string `json:"domain"`
	DocRoot string `json:"doc_root"`
}

func catalogRemovalPath(siteID int, site catalogSite) string {
	return filepath.Join(catalogRemovalStateDir, strconv.Itoa(siteID)+"-"+site.app.ID+".json")
}

func catalogRemovalRecordFor(siteID int, site catalogSite) catalogRemovalRecord {
	return catalogRemovalRecord{SiteID: siteID, AppID: site.app.ID, Domain: site.domain, DocRoot: site.docRoot}
}

// catalogRemovalStarted: a removal of exactly this application on exactly
// this site and document root was started and has not finished.
func catalogRemovalStarted(siteID int, site catalogSite) bool {
	path := catalogRemovalPath(siteID, site)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var record catalogRemovalRecord
	if json.Unmarshal(raw, &record) != nil {
		return false
	}
	return record == catalogRemovalRecordFor(siteID, site)
}

func markCatalogRemoval(siteID int, site catalogSite) error {
	if err := os.MkdirAll(catalogRemovalStateDir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(catalogRemovalStateDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("the removal record folder is not a real directory")
	}
	raw, err := json.Marshal(catalogRemovalRecordFor(siteID, site))
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
func catalogRemovalPermissionArgv(docRoot string) []string {
	return []string{"find", docRoot, "-xdev", "-type", "d", "!", "-perm", "-u=rwx", "-exec", "chmod", "u+rwx", "--", "{}", "+"}
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
