package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A Node.js application moves to its site's own user (alpha.107, D-038 §6).
// Applications made before this release run as the shared web user; nothing
// moves by itself. At the administrator's request the Panel moves one
// application: this RPC gives the files the web user owns inside the
// application's directory to the site's user and records exactly which paths
// it changed; the Panel then rewrites the unit (User=), which restarts the
// application, proves that it answers, and on failure puts the unit and,
// through Restore, exactly these paths back. Symbolic links are changed as
// links, never followed; a directory outside the hosting root is refused.
//
// Bir Node.js uygulaması sitenin kendi kullanıcısına taşınır: web
// kullanıcısına ait dosyalar site kullanıcısına verilir, değişen yollar
// kaydedilir; başarısızlıkta tam bu yollar geri konur.

const appFilesOwnerCap = 200_000

var appFilesOwnerRecordDir = hostingpath.ServiceMutationStateRoot()

func appFilesOwnerRecordPath(siteID int) string {
	return filepath.Join(appFilesOwnerRecordDir, fmt.Sprintf("app-owner-%d.json", siteID))
}

type appFilesOwnerRecord struct {
	Version  int      `json:"version"`
	SiteID   int      `json:"site_id"`
	WorkDir  string   `json:"work_dir"`
	FromUID  int      `json:"from_uid"`
	FromGID  int      `json:"from_gid"`
	ToUID    int      `json:"to_uid"`
	ToGID    int      `json:"to_gid"`
	Paths    []string `json:"paths"`
	GroupToo []bool   `json:"group_too"`
}

func lookupAccountIDs(name string) (int, int, error) {
	account, err := user.Lookup(name)
	if err != nil {
		return 0, 0, fmt.Errorf("account %s: %w", name, err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

var appFilesLchown = os.Lchown

// moveAppFilesOwner gives every path under workDir owned by from to to
// (its group too, when it is from's group) and returns the record.
func moveAppFilesOwner(siteID int, workDir string, fromUID, fromGID, toUID, toGID int) (appFilesOwnerRecord, error) {
	record := appFilesOwnerRecord{Version: 1, SiteID: siteID, WorkDir: workDir, FromUID: fromUID, FromGID: fromGID, ToUID: toUID, ToGID: toGID}
	err := filepath.WalkDir(workDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != fromUID {
			return nil
		}
		if len(record.Paths) >= appFilesOwnerCap {
			return fmt.Errorf("more than %d files belong to the web user here; nothing was changed", appFilesOwnerCap)
		}
		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return err
		}
		record.Paths = append(record.Paths, rel)
		record.GroupToo = append(record.GroupToo, int(stat.Gid) == fromGID)
		return nil
	})
	if err != nil {
		return appFilesOwnerRecord{}, err
	}
	for i, rel := range record.Paths {
		gid := -1
		if record.GroupToo[i] {
			gid = toGID
		}
		if err := appFilesLchown(filepath.Join(workDir, rel), toUID, gid); err != nil {
			// Put back what this pass already changed.
			restoreAppFilesOwner(appFilesOwnerRecord{WorkDir: workDir, FromUID: fromUID, FromGID: fromGID, Paths: record.Paths[:i], GroupToo: record.GroupToo[:i]})
			return appFilesOwnerRecord{}, fmt.Errorf("change the owner of %s: %w; what was changed was put back", rel, err)
		}
	}
	return record, nil
}

func restoreAppFilesOwner(record appFilesOwnerRecord) error {
	var first error
	for i, rel := range record.Paths {
		gid := -1
		if i < len(record.GroupToo) && record.GroupToo[i] {
			gid = record.FromGID
		}
		if err := appFilesLchown(filepath.Join(record.WorkDir, rel), record.FromUID, gid); err != nil && first == nil && !os.IsNotExist(err) {
			first = err
		}
	}
	return first
}

func validAppWorkDir(workDir string) error {
	clean := filepath.Clean(workDir)
	if clean != workDir || !filepath.IsAbs(clean) || !hostingpath.IsSiteHome(clean) {
		return fmt.Errorf("the application directory is not inside the hosting root")
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("the application directory is not a plain directory")
	}
	return nil
}

// MoveAppFilesOwnerV1 moves (or, with Restore, puts back) an application's
// files between the shared web user and the site's own user.
func (a *Agent) MoveAppFilesOwnerV1(req *transport.AppFilesOwnerRequest, resp *transport.AppFilesOwnerResponse) error {
	*resp = transport.AppFilesOwnerResponse{}
	if req == nil || req.SiteID <= 0 {
		resp.Error = "invalid request"
		return nil
	}
	action := "move"
	if req.Restore {
		action = "restore"
	}
	_, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepMoveAppFilesOwner, fmt.Sprintf("site-%d", req.SiteID), "", action),
	)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	defer finishStep()
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	if req.Restore {
		data, err := os.ReadFile(appFilesOwnerRecordPath(req.SiteID))
		if os.IsNotExist(err) {
			return nil
		}
		var record appFilesOwnerRecord
		if err != nil || json.Unmarshal(data, &record) != nil || record.SiteID != req.SiteID {
			resp.Error = "the record of the moved files could not be read; nothing was put back"
			return nil
		}
		if err := restoreAppFilesOwner(record); err != nil {
			resp.Error = "some files could not be given back: " + err.Error()
			return nil
		}
		resp.Changed = len(record.Paths)
		_ = os.Remove(appFilesOwnerRecordPath(req.SiteID))
		return nil
	}
	if err := validAppWorkDir(req.WorkDir); err != nil {
		resp.Error = err.Error()
		return nil
	}
	fromUID, fromGID, err := lookupAccountIDs(appUnitAccount(req.FromUser))
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	toUID, toGID, err := lookupAccountIDs(req.ToUser)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if toUID == 0 {
		resp.Error = "an application is never moved to root"
		return nil
	}
	record, err := moveAppFilesOwner(req.SiteID, req.WorkDir, fromUID, fromGID, toUID, toGID)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	data, err := json.Marshal(record)
	if err == nil {
		err = writePrivateFileAtomic(appFilesOwnerRecordPath(req.SiteID), data, ".app-owner-*")
	}
	if err != nil {
		_ = restoreAppFilesOwner(record)
		resp.Error = "the moved files could not be recorded, so they were given back: " + err.Error()
		return nil
	}
	resp.Changed = len(record.Paths)
	return nil
}
