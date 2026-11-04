//go:build linux

package sandboxbind

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

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

// ProductionHelper is the helper as the units run it.
func ProductionHelper(log io.Writer) Helper {
	return Helper{
		Plan: PlanFor, MountRoot: sitesandbox.MountRoot, RootUID: 0, Mounter: KernelMounter{},
		LookupUser: lookupAccount, UnitInfo: readUnitInfo, NFT: runNFT, Log: log,
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

const usage = "usage: sandbox-helper bind|unbind|egress-add|egress-remove --unit=<sandboxed unit>"

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
	return 1
}
