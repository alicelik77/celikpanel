//go:build linux

package sandboxbind

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// Account is the site account the unit runs as.
type Account struct {
	UID  uint32
	Home string
}

// UnitInfo is what the helper reads of the sandboxed unit's own settings.
type UnitInfo struct {
	User   string
	AppDir string
}

// Helper is the helper with everything it touches on the host.
type Helper struct {
	// Plan is PlanFor in production.
	Plan func(appDir string, uid uint32) (Plan, error)
	// MountRoot is sitesandbox.MountRoot in production; RootUID owns it.
	MountRoot string
	RootUID   uint32
	Mounter   Mounter
	// LookupUser resolves the unit's account.
	LookupUser func(name string) (Account, error)
	// UnitInfo reads the sandboxed unit's settings from systemd.
	UnitInfo func(unit string) (UnitInfo, error)
	// NFT runs nft with the given arguments (an empty environment).
	NFT func(args ...string) error
	Log io.Writer
}

var userName = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

func (h Helper) unitDir(unit string) string { return filepath.Join(h.MountRoot, unit) }
func (h Helper) target(unit string) string  { return filepath.Join(h.unitDir(unit), "app") }

// ParseUnitInfo reads `systemctl show <unit> --property=User,BindPaths`:
// the account and the destination of the bind from the unit's mount point.
// The helper reads no environment file (an environment file is root's own,
// outside the hosting tree).
func ParseUnitInfo(unit, show string) (UnitInfo, error) {
	props := sitesandbox.ParseShow(show)
	info := UnitInfo{User: props["User"]}
	for _, field := range strings.Fields(props["BindPaths"]) {
		parts := strings.Split(strings.TrimPrefix(field, "-"), ":")
		if len(parts) >= 2 && parts[0] == sitesandbox.BindSource(unit) {
			if info.AppDir != "" {
				return UnitInfo{}, errors.New("the unit binds the app folder's mount point twice")
			}
			info.AppDir = parts[1]
		}
	}
	if info.User == "" || info.AppDir == "" {
		return UnitInfo{}, fmt.Errorf("%s names no account or binds no app folder from %s", unit, sitesandbox.BindSource(unit))
	}
	return info, nil
}

// Bind is the bind unit's start: it reads the sandboxed unit's settings and
// binds its app folder (Start).
func (h Helper) Bind(unit string) error {
	if !sitesandbox.ValidUnitName(unit) {
		return errors.New("not a sandboxed unit name")
	}
	info, err := h.UnitInfo(unit)
	if err != nil {
		return fmt.Errorf("the settings of %s could not be read: %v", unit, err)
	}
	return h.Start(StartRequest{Unit: unit, AppDir: info.AppDir, User: info.User})
}

// Start checks the app folder and binds it onto the unit's mount point. Any
// error means the unit must not start; a
// *Refusal is a verdict on the app folder. Nothing is changed before every
// check passed.
func (h Helper) Start(req StartRequest) error {
	if !sitesandbox.ValidUnitName(req.Unit) {
		return errors.New("not a sandboxed unit name")
	}
	if !userName.MatchString(req.User) || req.User == "root" {
		return errors.New("not a site account name")
	}
	account, err := h.LookupUser(req.User)
	if err != nil {
		return refused(CodeNotOwned, "", "the site's account %s could not be found: %v", req.User, err)
	}
	plan, err := h.Plan(req.AppDir, account.UID)
	if err != nil {
		return err
	}
	if filepath.Clean(account.Home) != plan.Home {
		return refused(CodeNotOwned, plan.AppDir, "is not in the home of the account %s", req.User)
	}
	appFD, _, err := Open(plan)
	if err != nil {
		return err
	}
	defer unix.Close(appFD)
	if err := h.rootDir(h.MountRoot); err != nil {
		return err
	}
	if err := h.rootDir(h.unitDir(req.Unit)); err != nil {
		return err
	}
	target := h.target(req.Unit)
	if err := h.unmountAll(target); err != nil {
		return err
	}
	if err := h.rootDir(target); err != nil {
		return err
	}
	if err := h.Mounter.Bind(appFD, target); err != nil {
		return fmt.Errorf("the app folder could not be bound onto %s: %v", target, err)
	}
	return nil
}

// Stop removes the mount (the bind unit's stop).
// It goes on after a failure and reports the first one.
func (h Helper) Stop(unit string) error {
	if !sitesandbox.ValidUnitName(unit) {
		return errors.New("not a sandboxed unit name")
	}
	dir := h.unitDir(unit)
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() {
		return nil
	}
	if err := h.unmountAll(h.target(unit)); err != nil {
		return err
	}
	_ = os.Remove(h.target(unit))
	_ = os.Remove(dir)
	return nil
}

// EgressAdd loads the SMTP block and adds the unit's cgroup (the sandboxed
// unit's ExecStartPre=+, run inside the unit's cgroup).
func (h Helper) EgressAdd(unit string) error {
	element, err := sitesandbox.CgroupElement(unit)
	if err != nil {
		return err
	}
	if err := h.NFT("-f", sitesandbox.EgressFile); err != nil {
		return fmt.Errorf("the SMTP block could not be loaded from %s: %v", sitesandbox.EgressFile, err)
	}
	if err := h.NFT("add", "element", "inet", sitesandbox.EgressTable, sitesandbox.EgressSet, element); err != nil {
		return fmt.Errorf("the unit's cgroup could not be added to the SMTP block: %v", err)
	}
	return nil
}

// EgressRemove removes the unit's cgroup from the SMTP block.
func (h Helper) EgressRemove(unit string) error {
	element, err := sitesandbox.CgroupElement(unit)
	if err != nil {
		return err
	}
	if err := h.NFT("delete", "element", "inet", sitesandbox.EgressTable, sitesandbox.EgressSet, element); err != nil {
		// The table may be gone (nft flush ruleset); the guard re-adds
		// only running units.
		return fmt.Errorf("the cgroup was not in the SMTP block: %v", err)
	}
	return nil
}

// unmountAll detaches every mount stacked at target.
func (h Helper) unmountAll(target string) error {
	for i := 0; i < 16; i++ {
		err := h.Mounter.Unmount(target)
		if errors.Is(err, ErrNotMounted) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("the earlier mount at %s could not be removed: %v", target, err)
		}
	}
	return fmt.Errorf("more than 16 mounts are stacked at %s", target)
}

// rootDir makes p a real folder of RootUID with mode 0700 (it is created
// when missing, and a link or anything else there is refused).
func (h Helper) rootDir(p string) error {
	if err := os.Mkdir(p, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("%s could not be made: %v", p, err)
	}
	var st unix.Stat_t
	if err := unix.Lstat(p, &st); err != nil {
		return fmt.Errorf("%s could not be read: %v", p, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != h.RootUID {
		return fmt.Errorf("%s is not a folder of root's", p)
	}
	if st.Mode&0o7777 != 0o700 {
		if err := unix.Fchmodat(unix.AT_FDCWD, p, 0o700, 0); err != nil {
			return fmt.Errorf("%s: %v", p, err)
		}
	}
	return nil
}
