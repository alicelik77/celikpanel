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
	// SelfCgroup reads the helper's own cgroup (/proc/self/cgroup), and
	// ControlGroup asks systemd for the cgroup the unit runs in: the system
	// slice, or its site's slice where a later release placed it
	// (egressElement says in which order).
	SelfCgroup   func() (string, error)
	ControlGroup func(unit string) (string, error)
	// CgroupExists reports whether a cgroup path (as the SMTP set's element
	// names it) exists under the cgroup root.
	CgroupExists func(path string) bool
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

// ParseControlGroup reads `systemctl show <unit> --property=ControlGroup,Slice`:
// the unit's cgroup, or "/<slice>/<unit>" when systemd shows no cgroup (a unit
// whose processes have all gone).
func ParseControlGroup(unit, show string) string {
	props := sitesandbox.ParseShow(show)
	if cgroup := props["ControlGroup"]; cgroup != "" {
		return cgroup
	}
	if slice := props["Slice"]; slice != "" {
		return "/" + slice + "/" + unit
	}
	return ""
}

// ParseSelfCgroup reads /proc/self/cgroup: the unified hierarchy's path (the
// "0::" line), or "" when there is none.
func ParseSelfCgroup(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if rest, ok := strings.CutPrefix(line, "0::"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// egressElement is the SMTP set's element of the unit: the cgroup it runs in,
// the system slice or its site's slice (depth two, where the set matches).
// It is established in this order, so that the SMTP helper adds no
// dependency on systemd answering. (The bind unit's own read of the unit's
// settings, unchanged from earlier releases, still stops a start while
// systemd does not answer.)
//
//  1. The helper's own cgroup (SelfCgroup): egress-add and egress-remove run
//     as the unit's own ExecStartPre=+ and ExecStopPost=-+, and systemd runs a
//     unit's control processes in the unit's own cgroup. Only a unit with
//     Delegate= gets a ".control" subgroup for them (systemd's "Control Group
//     APIs and Delegation"); a sandboxed unit never has Delegate=, so the
//     line is exactly /system.slice/<unit> or /celiksite_<id>.slice/<unit>.
//     Anything else - the helper run by hand, a cgroup namespace - is not
//     taken, and systemd is asked.
//  2. systemd (ControlGroup): the unit's ControlGroup=, or its Slice=. A
//     place that is neither slice is refused, so the unit does not start.
//  3. When systemd cannot be asked or names no place: the system slice's
//     element, the one this release always added. nft turns the path into
//     the cgroup's inode when it adds the element, so for a unit that does
//     not run there the add fails and the unit does not start either: no
//     program ever starts outside the block.
//
// When its own cgroup line is not used, one journal line says why, which
// step decided and the place taken, so every server shows which step it
// uses. removing words that line for egress-remove.
func (h Helper) egressElement(unit string, removing bool) (string, error) {
	if !sitesandbox.ValidUnitName(unit) {
		return "", errors.New("not a sandboxed unit name")
	}
	own := "not read"
	if h.SelfCgroup != nil {
		cgroup, err := h.SelfCgroup()
		if err == nil {
			if _, ok := sitesandbox.UnitControlGroupAllowed(cgroup, unit); ok {
				return sitesandbox.CgroupElementFor(cgroup, unit)
			}
			own = fmt.Sprintf("%q", cgroup)
		} else {
			own = "unreadable: " + err.Error()
		}
	}
	reason := "systemd was not asked"
	if h.ControlGroup != nil {
		cgroup, err := h.ControlGroup(unit)
		if err == nil && cgroup != "" {
			element, err := sitesandbox.CgroupElementFor(cgroup, unit)
			if err != nil {
				h.logf("celikpanel-sandbox: %s: its own cgroup line (%s) is not one of the two places; systemd says it runs in %q, which the SMTP block cannot hold", unit, own, cgroup)
				return "", &PlaceError{Unit: unit, Detail: err.Error()}
			}
			h.logf("celikpanel-sandbox: %s: its own cgroup line (%s) is not one of the two places; systemd says it runs in %s, so the SMTP block uses %s", unit, own, cgroup, element)
			return element, nil
		}
		reason = "systemd named no place"
		if err != nil {
			reason = "systemd could not be asked: " + err.Error()
		}
	}
	action := "adds it in the system slice, as before (nft refuses that path if the unit runs elsewhere)"
	if removing {
		action = "removes the system slice's element, as before"
	}
	h.logf("celikpanel-sandbox: %s: its own cgroup line (%s) is not one of the two places and %s; the SMTP block %s", unit, own, reason, action)
	return sitesandbox.CgroupElement(unit)
}

func (h Helper) logf(format string, args ...any) {
	if h.Log != nil {
		fmt.Fprintf(h.Log, format+"\n", args...)
	}
}

// placeFailure reports whether nft's refusal of the element is about the
// unit's place: nft says the cgroup path fails, or that path does not exist
// under the cgroup root. Any other failure (the table flushed in between,
// nft killed) is not the unit's place.
func (h Helper) placeFailure(element string, err error) bool {
	if strings.Contains(err.Error(), "cgroupv2 path fails") {
		return true
	}
	if h.CgroupExists == nil {
		return false
	}
	path := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(element, "{ "), " }"), `"`)
	return !h.CgroupExists(path)
}

// PlaceError: the unit runs somewhere the SMTP block cannot hold it (neither
// the system slice nor its site's slice), so it is not started.
type PlaceError struct {
	Unit   string
	Detail string
}

func (e *PlaceError) Error() string { return e.Detail }

// EgressAdd loads the SMTP block and adds the unit's cgroup (the sandboxed
// unit's ExecStartPre=+, run inside the unit's cgroup).
func (h Helper) EgressAdd(unit string) error {
	element, err := h.egressElement(unit, false)
	if err != nil {
		return err
	}
	if err := h.NFT("-f", sitesandbox.EgressFile); err != nil {
		return fmt.Errorf("the SMTP block could not be loaded from %s: %v", sitesandbox.EgressFile, err)
	}
	if err := h.NFT("add", "element", "inet", sitesandbox.EgressTable, sitesandbox.EgressSet, element); err != nil {
		detail := fmt.Sprintf("the unit's cgroup %s could not be added to the SMTP block: %v", element, err)
		if h.placeFailure(element, err) {
			// For a unit that does not run at that path, the path does not
			// exist: a problem of the unit's place.
			return &PlaceError{Unit: unit, Detail: detail}
		}
		return errors.New(detail)
	}
	return nil
}

// EgressRemove removes the unit's cgroup from the SMTP block.
func (h Helper) EgressRemove(unit string) error {
	element, err := h.egressElement(unit, true)
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
