package sitesandbox

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// The SMTP egress block. A program in the sandbox may not open a mail
// connection (ports 25, 465, 587) to any address, the server's own mail
// server on 127.0.0.1 included: Postfix relays from 127.0.0.1 without a
// password (permit_mynetworks), so without this block every sandboxed
// program could send mail as the server.
//
// It is a separate nftables table (inet celikpanel_egress), never the
// firewall's own inet celikpanel_fw: the firewall policy (V2) is input-only
// and refuses anything else. The table holds one set of cgroups; every
// sandboxed unit's helper (helper.go, ExecStartPre=+, as root, outside the
// sandbox) loads the table and adds the unit's cgroup before it starts, and
// removes it after it stops. If nft fails, the unit does not start (fail
// closed). The table, its guard timer, the helper and the units are native
// files and keep working without CelikPanel (D-022).
//
// Not covered, and the screen says so: other mail ports (2525), mail sent
// over HTTP APIs, and the program's own outbound traffic in general.

const (
	// EgressDir holds the egress table and its guard script.
	EgressDir = "/usr/lib/celikpanel-sandbox"
	// EgressFile is the table definition every unit loads.
	EgressFile = EgressDir + "/egress.nft"
	// EgressGuardScript re-loads the table and re-adds running units.
	EgressGuardScript = EgressDir + "/egress-guard.sh"
	// EgressGuardUnit and EgressGuardTimer run the guard every 30 seconds.
	EgressGuardUnit  = "celikpanel-egress-guard.service"
	EgressGuardTimer = "celikpanel-egress-guard.timer"
	// EgressTable, EgressSet, EgressCounter name the nftables objects.
	EgressTable   = "celikpanel_egress"
	EgressSet     = "smtp_blocked"
	EgressCounter = "smtp_rejects"
	// NFT is the nft program (on Arch /usr/sbin is /usr/bin).
	NFT = "/usr/sbin/nft"
)

// SMTPPorts are the mail ports the block refuses.
var SMTPPorts = []int{25, 465, 587}

// EgressRuleset is the text of EgressFile (without CelikPanel's header).
// Loading it again is harmless and atomic: the table, set, counter and
// chains are declared (an existing one is kept with its elements and its
// count), the two chains are flushed and their rules added in the same
// transaction. Running units therefore stay in the set across every reload.
func EgressRuleset() string {
	ports := make([]string, len(SMTPPorts))
	for i, p := range SMTPPorts {
		ports[i] = strconv.Itoa(p)
	}
	portSet := "{ " + strings.Join(ports, ", ") + " }"
	return `# CelikPanel site sandbox: SMTP egress block (docs/SITE-SANDBOX.md).
# Members of @smtp_blocked are the cgroups of sandboxed programs (celiksvc-*,
# celiksvcinst-*, celikrt-*, celikrtbuild-*, celikapp-*, celikappinst-*). They cannot connect to ports 25, 465 and 587 on
# any address, 127.0.0.1 included, and nothing outside reaches a port they
# opened on another address than loopback. Each unit adds its own cgroup
# before it starts and removes it after it stops; celikpanel-egress-guard.timer
# loads this file again and re-adds running units every 30 seconds.
# It works without CelikPanel. Removing it: nft delete table inet celikpanel_egress
#
# CelikPanel site korumasi: korumadaki programlar 25, 465 ve 587 numarali
# posta baglantilarini hicbir adrese (127.0.0.1 dahil) acamaz.
table inet ` + EgressTable + ` {
	set ` + EgressSet + ` {
		typeof socket cgroupv2 level 2
	}
	counter ` + EgressCounter + ` {
	}
	chain output {
		type filter hook output priority filter; policy accept;
	}
	chain input {
		type filter hook input priority filter; policy accept;
	}
}
flush chain inet ` + EgressTable + ` output
flush chain inet ` + EgressTable + ` input
table inet ` + EgressTable + ` {
	chain output {
		meta l4proto tcp tcp dport ` + portSet + ` socket cgroupv2 level 2 @` + EgressSet + ` counter name "` + EgressCounter + `" reject with tcp reset
	}
	chain input {
		iifname != "lo" tcp flags & (fin | syn | rst | ack) == syn socket cgroupv2 level 2 @` + EgressSet + ` drop
	}
}
`
}

// EgressGuardScriptText is the guard's shell text (run by EgressGuardUnit
// with /bin/sh, so the header CelikPanel puts on line one is a comment). It
// loads the table only when it is missing or its rule is gone (someone ran
// "nft flush ruleset" or restarted nftables.service), then adds the cgroup
// of every running sandboxed unit whose helper loads the block (its
// ExecStartPre runs the helper's egress-add) again. A unit whose cgroup is not
// /system.slice/<unit> is named in the journal and left out.
func EgressGuardScriptText() string {
	return `# CelikPanel site sandbox: SMTP egress guard (docs/SITE-SANDBOX.md).
# Run by ` + EgressGuardUnit + ` every 30 seconds; works without CelikPanel.
set -u
nft=` + NFT + `
if ! "$nft" list chain inet ` + EgressTable + ` output 2>/dev/null | grep -q '@` + EgressSet + `'; then
	echo "celikpanel-egress: the SMTP block was missing; loading ` + EgressFile + ` again"
	"$nft" -f ` + EgressFile + ` || { echo "celikpanel-egress: nft could not load the SMTP block"; exit 1; }
fi
status=0
for unit in $(systemctl list-units --type=service --state=active,activating,reloading --plain --no-legend --no-pager 'celiksvc-*.service' 'celiksvcinst-*.service' 'celikrt-*.service' 'celikrtbuild-*.service' 'celikapp-*.service' 'celikappinst-*.service' | awk '{print $1}'); do
	case "$unit" in
		celiksvc-[0-9]*.service|celiksvcinst-[0-9]*.service|celikrt-[0-9]*.service|celikrtbuild-[0-9]*.service|celikapp-[0-9]*.service|celikappinst-[0-9]*.service) ;;
		*) continue ;;
	esac
	systemctl show -P ExecStartPre "$unit" | grep -q -F 'argv[]=` + HelperPath + ` egress-add ' || continue
	cgroup=$(systemctl show -P ControlGroup "$unit")
	if [ "$cgroup" != "/system.slice/$unit" ]; then
		echo "celikpanel-egress: $unit runs in $cgroup, not /system.slice/$unit; it is not blocked"
		status=1
		continue
	fi
	"$nft" add element inet ` + EgressTable + ` ` + EgressSet + ` "{ \"system.slice/$unit\" }" || { echo "celikpanel-egress: $unit could not be added"; status=1; }
done
exit $status
`
}

// EgressGuardServiceText is the guard's oneshot unit.
func EgressGuardServiceText() string {
	return `# CelikPanel site sandbox: keeps the SMTP egress block in place (docs/SITE-SANDBOX.md).
[Unit]
Description=CelikPanel site sandbox SMTP egress guard
After=network-pre.target

[Service]
Type=oneshot
ExecStart=/bin/sh ` + EgressGuardScript + `
LogLevelMax=notice
NoNewPrivileges=yes
ProtectHome=yes
PrivateTmp=yes
`
}

// EgressGuardTimerText runs the guard at boot and every 30 seconds.
func EgressGuardTimerText() string {
	return `# CelikPanel site sandbox: keeps the SMTP egress block in place (docs/SITE-SANDBOX.md).
[Unit]
Description=CelikPanel site sandbox SMTP egress guard

[Timer]
OnBootSec=20s
OnUnitActiveSec=30s
AccuracySec=5s

[Install]
WantedBy=timers.target
`
}

// cgroupElement is the set element of a unit: its cgroup path under the
// hierarchy root. "socket cgroupv2 level 2" compares the socket's ancestor
// at depth two, which is exactly /system.slice/<unit>.
func cgroupElement(unit string) string { return "system.slice/" + unit }

// CgroupElement is the set element of a unit, as the helper and the guard
// add it: '{ "system.slice/<unit>" }' in nft's syntax.
func CgroupElement(unit string) (string, error) {
	if !ValidUnitName(unit) {
		return "", errors.New("not a sandboxed unit name")
	}
	return `{ "` + cgroupElement(unit) + `" }`, nil
}

var setElementToken = regexp.MustCompile(`"([^"]*)"|\b([0-9]+)\b`)

// ParseSetElements reads `nft list set inet celikpanel_egress smtp_blocked`:
// the cgroup paths (nft prints a live cgroup as its path, a vanished one as
// its number). It returns nil when the output has no set.
func ParseSetElements(output string) []string {
	start := strings.Index(output, "elements = {")
	if start < 0 {
		if strings.Contains(output, "set "+EgressSet) {
			return []string{}
		}
		return nil
	}
	rest := output[start+len("elements = {"):]
	end := strings.Index(rest, "}")
	if end < 0 {
		return nil
	}
	out := []string{}
	for _, m := range setElementToken.FindAllStringSubmatch(rest[:end], -1) {
		if m[1] != "" {
			out = append(out, m[1])
		} else {
			out = append(out, m[2])
		}
	}
	return out
}

// InSet reports whether unit's cgroup is among the elements.
func InSet(elements []string, unit string) bool {
	want := cgroupElement(unit)
	for _, e := range elements {
		if e == want || e == "/"+want {
			return true
		}
	}
	return false
}

var counterPackets = regexp.MustCompile(`packets ([0-9]+) bytes ([0-9]+)`)

// ParseCounter reads `nft list counter inet celikpanel_egress smtp_rejects`:
// the number of refused mail connection attempts since the table was made
// (-1: no counter in the output).
func ParseCounter(output string) int64 {
	m := counterPackets.FindStringSubmatch(output)
	if m == nil {
		return -1
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return -1
	}
	return n
}
