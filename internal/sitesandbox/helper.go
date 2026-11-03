package sitesandbox

import (
	"errors"
	"strings"
)

// The sandbox helper and its bind unit (alpha.119, docs/SITE-SANDBOX.md
// §1a). systemd resolves the paths of a unit as root and follows links: the
// source of BindPaths=, EnvironmentFile= and the working folder. The site's
// account owns its site home, so it can rename its app folder and put a link
// in its place between the Agent's checks and a start, or before any later
// restart. The unit therefore never names a path inside the site home on the
// host.
//
// systemd sets up a unit's mount namespace (BindPaths=,
// TemporaryFileSystem=) for every command of the unit, "+" commands
// included; only the Protect* settings are skipped for them. So the folder
// cannot be bound by an ExecStartPre= of the unit itself. It is bound by a
// separate oneshot unit without any sandbox, celikpanel-sandbox@<unit>
// (BindTemplateText), which the sandboxed unit requires and starts after.
// Before the sandboxed unit starts, its bind unit runs the helper as root in
// the host's namespace:
//
//   - it reads the sandboxed unit's own settings (User=, BindPaths=,
//     EnvironmentFile=) from systemd;
//   - it opens the app folder from the hosting tree one folder at a time
//     with openat2 RESOLVE_NO_SYMLINKS (no link anywhere, no mount point
//     below the site home), and checks every folder's owner: root above the
//     site home, the site's account from the home down;
//   - it binds that opened folder, by its descriptor and never by its name,
//     onto the root-owned /run/celikpanel-sandbox/<unit>/app, nosuid and
//     nodev;
//   - it copies a service app's environment file, read the same way, to the
//     root-owned /run/celikpanel-sandbox/<unit>/app.env.
//
// The sandboxed unit binds only that root-owned path into the sandbox, at
// the folder's usual path. The mount holds the folder itself: renaming it or
// swapping it for a link later changes nothing for the running program.
// Every start, stop or restart of the unit starts or stops its bind unit
// with it (PartOf=), it stops when the unit no longer runs
// (StopWhenUnneeded=), and every start checks again. If a check or the mount
// fails, the bind unit fails and the sandboxed unit does not start.
//
// The SMTP egress block stays in the sandboxed unit (ExecStartPre=+ with
// --egress-add, ExecStopPost=-+): the unit's cgroup must exist when nft adds
// it. No privileged command of a sandboxed unit is a dynamically linked
// program: systemd gives every command of a unit the unit's environment, and
// a service app's environment file is the customer's. The helper is a static
// Go program (a copy of the Agent, installed under EgressDir so that it keeps
// working without CelikPanel, D-022), and it runs nft with an empty
// environment.
//
// Sandbox yardımcısı ve bağlama birimi: birim ana sistemde site klasöründeki
// hiçbir yolu anmaz. Ayrı, korumasız bir birim uygulama klasörünü bağlantı
// izlemeden açar, sahiplerini denetler ve root'a ait bir noktaya bağlar.

const (
	// HelperName is the helper's file name; the Agent binary acts as the
	// helper when it is started under this name.
	HelperName = "sandbox-helper"
	// HelperPath is where the Agent installs the helper.
	HelperPath = EgressDir + "/" + HelperName
	// MountRoot holds one root-only folder per running unit.
	MountRoot = "/run/celikpanel-sandbox"
	// BindTemplate is the bind unit's template file name.
	BindTemplate = "celikpanel-sandbox@.service"
	// ServiceAppEnvFrom is a service app's environment file inside its
	// folder; the helper copies it for EnvironmentFile=.
	ServiceAppEnvFrom = "config/app.env"
)

// UnitMountDir is the unit's root-only folder (root:root 0700).
func UnitMountDir(unit string) string { return MountRoot + "/" + unit }

// BindSource is the root-owned mount point the app folder is bound onto;
// the unit binds only this path into the sandbox.
func BindSource(unit string) string { return UnitMountDir(unit) + "/app" }

// EnvCopy is the root-owned copy of the app's environment file.
func EnvCopy(unit string) string { return UnitMountDir(unit) + "/app.env" }

// BindUnit is the bind unit of a sandboxed unit:
// celikpanel-sandbox@celikrt-5.service for celikrt-5.service.
func BindUnit(unit string) string {
	return "celikpanel-sandbox@" + strings.TrimSuffix(unit, ".service") + ".service"
}

// BindTemplateText is the bind unit's template (a native file the Agent
// writes under the D-031 rule). It has no sandbox on purpose: it runs the
// helper as root in the host's mount namespace.
func BindTemplateText() string {
	return `# CelikPanel site sandbox: binds a sandboxed unit's app folder (docs/SITE-SANDBOX.md §1a).
# The instance is the sandboxed unit's name without ".service" (celikrt-5, celiksvc-12,
# celiksvcinst-12). The sandboxed unit requires this unit and starts after it; a stop or
# restart of the sandboxed unit stops or restarts it too, and it stops when that unit no longer
# runs. It works without CelikPanel:
#   systemctl status celikpanel-sandbox@<name> ; journalctl -u celikpanel-sandbox@<name>
#
# CelikPanel site korumasi: korumali birimin uygulama klasorunu baglantisiz acip baglar.
[Unit]
Description=CelikPanel site sandbox: app folder of %i.service
PartOf=%i.service
StopWhenUnneeded=yes

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=` + HelperPath + ` bind --unit=%i.service
ExecStop=` + HelperPath + ` unbind --unit=%i.service
`
}

// EgressAddArgs and EgressRemoveArgs are the helper's arguments in the
// sandboxed unit itself (a host that holds the SMTP block).
func EgressAddArgs(unit string) []string    { return []string{"egress-add", "--unit=" + unit} }
func EgressRemoveArgs(unit string) []string { return []string{"egress-remove", "--unit=" + unit} }

// HelperCommand is the command text systemd shows for a helper call
// (`systemctl show`'s "argv[]=" part).
func HelperCommand(args []string) string {
	return HelperPath + " " + strings.Join(args, " ")
}

// HelperLines are a sandboxed unit's lines for its bind unit ([Unit]:
// Requires=, After=) and, on a host that holds the SMTP block, the egress
// lines ([Service]: ExecStartPre=+, which stops the start when it fails, and
// ExecStopPost=-+).
func HelperLines(p Profile, c Caps) (unitLines, pre, post []string, err error) {
	if err := p.Validate(); err != nil {
		return nil, nil, nil, err
	}
	bind := BindUnit(p.Unit)
	unitLines = []string{"Requires=" + bind, "After=" + bind}
	if c.EgressEnforced() {
		pre = []string{"ExecStartPre=+" + HelperCommand(EgressAddArgs(p.Unit))}
		post = []string{"ExecStopPost=-+" + HelperCommand(EgressRemoveArgs(p.Unit))}
	}
	for _, line := range append(append(append([]string{}, unitLines...), pre...), post...) {
		if err := checkLine(line); err != nil {
			return nil, nil, nil, err
		}
	}
	return unitLines, pre, post, nil
}

// EnvironmentFileLine is the unit's EnvironmentFile= line for a profile
// whose environment file lives in the app folder: the bind unit's root-owned
// copy. It has no "-": without the copy nothing starts.
func EnvironmentFileLine(p Profile) (string, error) {
	if p.EnvFrom == "" {
		return "", errors.New("the profile has no environment file")
	}
	if !ValidUnitName(p.Unit) || p.EnvFrom != ServiceAppEnvFrom {
		return "", errors.New("invalid environment file")
	}
	return "EnvironmentFile=" + EnvCopy(p.Unit), nil
}
