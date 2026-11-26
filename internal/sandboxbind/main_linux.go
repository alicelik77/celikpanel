//go:build linux

package sandboxbind

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// lookupAccount resolves a site account from the system's account files.
func lookupAccount(name string) (Account, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return Account{}, err
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return Account{}, err
	}
	return Account{UID: uint32(uid), Home: u.HomeDir}, nil
}

// cleanEnv is the only environment a program the helper starts gets.
var cleanEnv = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}

// runClean runs a root program with an empty environment (the unit's
// environment is the customer's and never reaches a dynamically linked root
// program), from "/", for at most a minute.
func runClean(program string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = cleanEnv
	cmd.Dir = "/"
	return cmd.Output()
}

func runNFT(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, sitesandbox.NFT, args...)
	cmd.Env = cleanEnv
	cmd.Dir = "/"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// systemctlPath is systemctl on every supported system (Debian's /bin is
// /usr/bin; Arch has only /usr/bin).
const systemctlPath = "/usr/bin/systemctl"

// readUnitInfo asks systemd for the sandboxed unit's settings.
func readUnitInfo(unit string) (UnitInfo, error) {
	out, err := runClean(systemctlPath, "show", unit, "--property=User,BindPaths")
	if err != nil {
		return UnitInfo{}, fmt.Errorf("systemctl show: %v", err)
	}
	return ParseUnitInfo(unit, string(out))
}

// systemctlRun runs systemctl for readControlGroup (tests replace it).
var systemctlRun = runClean

// readControlGroup asks systemd for the cgroup the unit runs in (its
// ControlGroup=, or "/<Slice=>/<unit>" when systemd shows none yet). A failure
// carries systemctl's own words.
func readControlGroup(unit string) (string, error) {
	out, err := systemctlRun(systemctlPath, "show", unit, "--property=ControlGroup,Slice")
	if err != nil {
		return "", fmt.Errorf("systemctl show %s: %s", unit, hostcmd.Diagnostic(nil, err))
	}
	return ParseControlGroup(unit, string(out)), nil
}

// selfCgroupPath is the helper's own cgroup file (tests replace it).
var selfCgroupPath = "/proc/self/cgroup"

// readSelfCgroup reads the helper's own cgroup in the unified hierarchy.
func readSelfCgroup() (string, error) {
	raw, err := os.ReadFile(selfCgroupPath)
	if err != nil {
		return "", err
	}
	return ParseSelfCgroup(string(raw)), nil
}

// cgroupRoot is the unified cgroup hierarchy (tests replace it).
var cgroupRoot = "/sys/fs/cgroup"

// cgroupExists reports whether a cgroup path exists under the hierarchy.
func cgroupExists(path string) bool {
	fi, err := os.Lstat(filepath.Join(cgroupRoot, filepath.Clean("/"+path)))
	return err == nil && fi.IsDir()
}

// ProductionHelper is the helper as the units run it.
func ProductionHelper(log io.Writer) Helper {
	return Helper{
		Plan: PlanFor, MountRoot: sitesandbox.MountRoot, RootUID: 0, Mounter: KernelMounter{},
		LookupUser: lookupAccount, UnitInfo: readUnitInfo, SelfCgroup: readSelfCgroup, ControlGroup: readControlGroup,
		CgroupExists: cgroupExists, NFT: runNFT, Procs: ReadCgroupProcs, Log: log,
	}
}

// egressGuidance is what the journal says after egress-add refused a start:
// what was being established, who acts, and the command that shows the
// unit's place.
func egressGuidance(unit string) []string {
	return []string{
		"celikpanel-sandbox: the SMTP block could not take " + unit + " where it runs, so it was not started. " +
			"The server's administrator checks its place with: systemctl show -p ControlGroup,Slice " + unit +
			" (it must be /system.slice/" + unit + " or its site's /celiksite_<site>.slice/" + unit + "), corrects it, then starts the program again.",
		"celikpanel-sandbox: posta engeli " + unit + " birimini çalıştığı yerde tutamadı, bu yüzden başlatılmadı. " +
			"Sunucu yöneticisi yerini şu komutla görür: systemctl show -p ControlGroup,Slice " + unit +
			" (/system.slice/" + unit + " ya da sitesinin /celiksite_<site>.slice/" + unit + " olmalı); düzeltir, sonra programı yeniden başlatır.",
	}
}

// CheckAppFolder is the Agent's check of an app folder before it writes or
// starts a unit: the helper's walk, without a mount.
func CheckAppFolder(appDir string, uid uint32) error {
	plan, err := PlanFor(appDir, uid)
	if err != nil {
		return err
	}
	return Check(plan)
}

const usage = "usage: sandbox-helper bind|unbind|egress-add|egress-remove --unit=<sandboxed unit> | ftp-bind|ftp-unbind --login=<login> ..."

// Main is the helper's command line:
//
//	sandbox-helper bind --unit=<unit>           (celikpanel-sandbox@<name>.service ExecStart)
//	sandbox-helper unbind --unit=<unit>         (its ExecStop)
//	sandbox-helper egress-add --unit=<unit>     (the sandboxed unit's ExecStartPre=+)
//	sandbox-helper egress-remove --unit=<unit>  (its ExecStopPost=-+)
//
// The exit status is 0 when done, 1 when refused or failed, 2 for a usage
// error.
func Main(args []string, stderr io.Writer) int {
	// Nothing below reads the environment; a sandboxed unit's environment
	// (a service app's is the customer's) is dropped before anything runs.
	os.Clearenv()
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "ftp-bind", "ftp-unbind":
		return ftpMain(args, stderr)
	case "bind", "unbind", "egress-add", "egress-remove":
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
	fs := flag.NewFlagSet("sandbox-helper "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	unit := fs.String("unit", "", "the sandboxed unit")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || !sitesandbox.ValidUnitName(*unit) {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if unix.Geteuid() != 0 {
		fmt.Fprintln(stderr, "celikpanel-sandbox: the helper runs as root")
		return 1
	}
	h := ProductionHelper(stderr)
	var err error
	switch args[0] {
	case "unbind":
		err = h.Stop(*unit)
	case "egress-add":
		err = h.EgressAdd(*unit)
	case "egress-remove":
		err = h.EgressRemove(*unit)
	default:
		err = h.Bind(*unit)
	}
	if err == nil {
		return 0
	}
	if r, ok := AsRefusal(err); ok {
		fmt.Fprintf(stderr, "celikpanel-sandbox: %s was not started (%s): %s.\n", *unit, r.Code, r.Error())
		switch r.Code {
		case CodeNotReal:
			fmt.Fprintln(stderr, "celikpanel-sandbox: the app folder must be a real folder of this site, with no link on its way. Put the real folder back, then start the app again.")
			fmt.Fprintln(stderr, "celikpanel-sandbox: uygulama klasörü bu sitenin gerçek bir klasörü olmalı, yolunda bağlantı olmamalı. Gerçek klasörü geri koyun, sonra uygulamayı yeniden başlatın.")
		case CodeNotOwned:
			fmt.Fprintln(stderr, "celikpanel-sandbox: the folder must belong to the site's account (and folders above the site to root, not writable by others). The administrator corrects the owner, then starts the app again.")
			fmt.Fprintln(stderr, "celikpanel-sandbox: klasör sitenin hesabına (sitenin üstündeki klasörler root'a, başkalarınca yazılamaz) ait olmalı. Yönetici sahibini düzeltir, sonra uygulamayı yeniden başlatır.")
		default:
			fmt.Fprintln(stderr, "celikpanel-sandbox: the app folder could not be checked, so nothing was started. The administrator reads the message above, then starts the app again.")
			fmt.Fprintln(stderr, "celikpanel-sandbox: uygulama klasörü denetlenemedi; hiçbir şey başlatılmadı. Yönetici yukarıdaki iletiyi okur, sonra uygulamayı yeniden başlatır.")
		}
		return 1
	}
	fmt.Fprintf(stderr, "celikpanel-sandbox: %s %s: %v\n", args[0], *unit, err)
	var place *PlaceError
	if args[0] == "egress-add" && errors.As(err, &place) {
		for _, line := range egressGuidance(*unit) {
			fmt.Fprintln(stderr, line)
		}
	}
	return 1
}

const ftpUsage = "usage: sandbox-helper ftp-bind --login=<login> --site=<site account> --folder=<folder> | ftp-unbind --login=<login>"

// ftpMain is the login jail's command line (alpha.128, docs/FTP-LOGINS.md):
//
//	sandbox-helper ftp-bind --login=<l> --site=<s> --folder=<f>   (celikpanel-ftp-<l>.service ExecStart)
//	sandbox-helper ftp-unbind --login=<l>                         (its ExecStop)
func ftpMain(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("sandbox-helper "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	login := fs.String("login", "", "the login")
	site := fs.String("site", "", "the site account")
	folder := fs.String("folder", "", "the chosen folder")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || *login == "" ||
		(args[0] == "ftp-bind" && (*site == "" || *folder == "")) ||
		(args[0] == "ftp-unbind" && (*site != "" || *folder != "")) {
		fmt.Fprintln(stderr, ftpUsage)
		return 2
	}
	if unix.Geteuid() != 0 {
		fmt.Fprintln(stderr, "celikpanel-ftp: the helper runs as root")
		return 1
	}
	h := ProductionFTPHelper(stderr)
	if args[0] == "ftp-unbind" {
		if err := h.Unbind(*login); err != nil {
			fmt.Fprintf(stderr, "celikpanel-ftp: the jail of %s could not be removed: %v\n", *login, err)
			return 1
		}
		return 0
	}
	err := h.Bind(FTPRequest{Login: *login, Site: *site, Folder: *folder})
	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "celikpanel-ftp: the login %s has no jail, so it cannot sign in: %v.\n", *login, err)
	if r, ok := AsRefusal(err); ok {
		switch r.Code {
		case CodeNotReal:
			fmt.Fprintln(stderr, "celikpanel-ftp: the login's folder must be a real folder of its site, with no link on its way. Put the real folder back (or give the login another folder in the panel), then: systemctl restart "+fileaccess.LoginUnit(*login))
			fmt.Fprintln(stderr, "celikpanel-ftp: girişin klasörü sitenin gerçek bir klasörü olmalı, yolunda bağlantı olmamalı. Gerçek klasörü geri koyun (ya da panelden başka klasör verin), sonra birimi yeniden başlatın.")
		case CodeNotOwned:
			fmt.Fprintln(stderr, "celikpanel-ftp: the folder must belong to the site's account and the login must be that site's login (same uid and group, home in its jail, no shell). The administrator corrects it, then restarts the unit.")
			fmt.Fprintln(stderr, "celikpanel-ftp: klasör sitenin hesabına ait, giriş o sitenin girişi olmalı. Yönetici düzeltir, sonra birimi yeniden başlatır.")
		default:
			fmt.Fprintln(stderr, "celikpanel-ftp: the folder could not be checked, so no jail was made. The administrator reads the message above, then restarts the unit.")
			fmt.Fprintln(stderr, "celikpanel-ftp: klasör denetlenemedi; hapis kurulmadı. Yönetici yukarıdaki iletiyi okur, sonra birimi yeniden başlatır.")
		}
	}
	return 1
}
