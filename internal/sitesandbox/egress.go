package sitesandbox

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// The SMTP egress block. A program in the sandbox may not open a mail
// connection (ports 25, 465, 587) to any address, with one exception (S4
// R2b, 22 Nov 2026; narrowed in its fix round): submission to this server's
// own mail service on ports 465 and 587, at any of this server's own
// addresses (traffic to a local address leaves through the loopback
// interface, so `oifname "lo"` covers 127.0.0.1, ::1 and the server's public
// addresses, and a task that connects to the server's host name reaches the
// name its certificate carries). The Agent writes the exception ONLY while
// the listeners on 465 and 587 are CelikPanel's own unchanged submission
// entries that take nothing but SASL-authenticated clients
// (smtpd_client_restrictions=permit_sasl_authenticated,reject,
// mail_submission_rpc.go; cmd/agent/site_tasks_mail.go checks it); with an
// owner's own entry or another program there, the file has no exception and
// the screens say that sites cannot send mail through this server. Port 25
// stays refused on every address, loopback included (Postfix relayed from
// 127.0.0.1 without a password before alpha.125's permit_mynetworks
// change), and so do 465 and 587 to any address that is not this server's.
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

// SubmissionPorts are the mail ports a member may still reach at this
// server's own addresses, when the exception is written: this server's own
// submission services, which take only clients that log in with a mailbox's
// password (S4 R2b).
var SubmissionPorts = []int{465, 587}

// EgressRuleset is the text of EgressFile without the submission exception
// (without CelikPanel's header): every mail port refused for members.
func EgressRuleset() string { return EgressRulesetWith(false) }

// EgressRulesetWith is the text of EgressFile; submission adds the accept
// rule for this server's own submission services (see above). Loading it
// again is harmless and atomic: the table, set, counter and chains are
// declared (an existing one is kept with its elements and its count), the
// two chains are flushed and their rules added in the same transaction.
// Running units therefore stay in the set across every reload, and the
// exception comes or goes at once.
func EgressRulesetWith(submission bool) string {
	ports := make([]string, len(SMTPPorts))
	for i, p := range SMTPPorts {
		ports[i] = strconv.Itoa(p)
	}
	portSet := "{ " + strings.Join(ports, ", ") + " }"
	subPorts := make([]string, len(SubmissionPorts))
	for i, p := range SubmissionPorts {
		subPorts[i] = strconv.Itoa(p)
	}
	submissionSet := "{ " + strings.Join(subPorts, ", ") + " }"
	mailSentence := `# They cannot connect to port 25, 465 or 587 on any address, 127.0.0.1 included.
# (Sites cannot send mail through this server: CelikPanel opens 465 and 587 to them only
# while those ports are its own submission services that take a mailbox's password.)`
	mailSentenceTR := `# CelikPanel site korumasi: korumadaki programlar 25, 465 ve 587 numarali portlara
# hicbir adrese (127.0.0.1 dahil) baglanamaz.`
	acceptRule := ""
	if submission {
		mailSentence = `# They cannot connect to port 25 on any address, 127.0.0.1 included, nor to ports 465
# and 587 anywhere but this server's own addresses (its own submission services, which
# take only a mailbox's name and password; traffic to a local address leaves through "lo").
# CelikPanel decided this when it last checked those services (at the panel's start and
# after its mail sending step); it stays so until the panel's next start, and for good
# without the panel. If you put another program on 465 or 587, delete the "oifname" line
# below and run: nft -f ` + EgressFile
		mailSentenceTR = `# CelikPanel site korumasi: korumadaki programlar 25 numarali porta hicbir adrese
# (127.0.0.1 dahil) baglanamaz; 465 ve 587'ye yalniz bu sunucunun kendi adreslerindeki
# posta hizmetine (posta kutusu parolasiyla) baglanabilir.`
		acceptRule = "\t\toifname \"lo\" tcp dport " + submissionSet + " accept\n"
	}
	return `# CelikPanel site sandbox: SMTP egress block (docs/SITE-SANDBOX.md).
# Members of @smtp_blocked are the cgroups of sandboxed programs (celiksvc-*,
# celiksvcinst-*, celikrt-*, celikrtbuild-*, celikapp-*, celikappinst-*, celikgit-*) and of
# sites' scheduled tasks (celikcron-*).
` + mailSentence + `
# Nothing outside reaches a port they opened on another address than loopback. Each unit
# adds its own cgroup before it starts and removes it after it stops (a task's cgroup stays
# while processes it left run in it); celikpanel-egress-guard.timer loads this file again
# and re-adds running units every 30 seconds.
# It works without CelikPanel. Removing it: nft delete table inet celikpanel_egress
#
` + mailSentenceTR + `
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
` + acceptRule + `		meta l4proto tcp tcp dport ` + portSet + ` socket cgroupv2 level 2 @` + EgressSet + ` counter name "` + EgressCounter + `" reject with tcp reset
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
// /system.slice/<unit> or /celiksite_<site id>.slice/<unit> (its site's slice,
// S4 R2a) is named in the journal and left out. A finished
// install step that stays "active (exited)" has no cgroup and is skipped
// (alpha.127 lab: after a Node.js "Install packages and build" the guard
// failed every 30 seconds on celikappinst-N, "runs in , not ...").
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
for unit in $(systemctl list-units --type=service --state=active,activating,reloading --plain --no-legend --no-pager 'celiksvc-*.service' 'celiksvcinst-*.service' 'celikrt-*.service' 'celikrtbuild-*.service' 'celikapp-*.service' 'celikappinst-*.service' 'celikgit-*.service' 'celikcron-*.service' | awk '{print $1}'); do
	case "$unit" in
		celiksvc-[0-9]*.service|celiksvcinst-[0-9]*.service|celikrt-[0-9]*.service|celikrtbuild-[0-9]*.service|celikapp-[0-9]*.service|celikappinst-[0-9]*.service|celikgit-[0-9]*.service|celikcron-[0-9]*-[0-9]*.service) ;;
		*) continue ;;
	esac
	systemctl show -P ExecStartPre "$unit" | grep -q -F -e 'argv[]=` + HelperPath + ` egress-add ' -e 'argv[]=` + TaskHelperPath + ` egress-add ' || continue
	cgroup=$(systemctl show -P ControlGroup "$unit")
	# A finished install step (RemainAfterExit, "active (exited)") has no
	# cgroup any more: nothing runs there, so there is nothing to block.
	[ -n "$cgroup" ] || continue
	# The system slice, or the site's own slice celiksite_<site id>.slice
	# (docs/SITE-SANDBOX.md section 8): both are depth two, as the set matches.
	allowed=no
	case "$cgroup" in
		"/system.slice/$unit") allowed=yes ;;
		/celiksite_*.slice/"$unit")
			site=${cgroup#/celiksite_}
			site=${site%%.slice/*}
			case "$site" in
				''|0*|*[!0-9]*) ;;
				*) [ "$cgroup" = "/celiksite_$site.slice/$unit" ] && allowed=yes ;;
			esac
			;;
	esac
	if [ "$allowed" != yes ]; then
		echo "celikpanel-egress: $unit runs in $cgroup, not /system.slice/$unit or its site's slice; it is not blocked"
		status=1
		continue
	fi
	"$nft" add element inet ` + EgressTable + ` ` + EgressSet + ` "{ \"${cgroup#/}\" }" || { echo "celikpanel-egress: $unit could not be added"; status=1; }
done
# A site's task whose run ended can leave processes running in its cgroup
# (KillMode=process, as under cron); the unit is inactive then, so the loop
# above does not see it. Every task cgroup in a site's slice that still holds
# a process is added too, by its path alone: celikcron-<site>-<n>.service in
# celiksite_<site>.slice can only be a task's, also after its unit file was
# removed (systemd then shows no ExecStartPre for it).
for dir in /sys/fs/cgroup/celiksite_*.slice/celikcron-*-*.service; do
	[ -d "$dir" ] || continue
	read -r first 2>/dev/null < "$dir/cgroup.procs" || continue
	[ -n "$first" ] || continue
	unit=${dir##*/}
	slice=${dir%/*}
	slice=${slice##*/}
	site=${slice#celiksite_}
	site=${site%.slice}
	case "$site" in
		''|0*|*[!0-9]*) continue ;;
	esac
	case "$unit" in
		celikcron-"$site"-[0-9]*.service) ;;
		*) continue ;;
	esac
	case "${unit#celikcron-$site-}" in
		*[!0-9]*.service|0*) continue ;;
	esac
	"$nft" add element inet ` + EgressTable + ` ` + EgressSet + ` "{ \"$slice/$unit\" }" || { echo "celikpanel-egress: the processes $unit left could not be added"; status=1; }
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

// InSet reports whether unit's cgroup in the system slice is among the
// elements.
func InSet(elements []string, unit string) bool {
	return InSetCgroup(elements, "/"+cgroupElement(unit))
}

// InSetCgroup reports whether a unit's cgroup (its ControlGroup=, with the
// leading "/") is among the elements (S4 R2a: the system slice or the site's
// slice).
func InSetCgroup(elements []string, cgroup string) bool {
	want := strings.TrimPrefix(cgroup, "/")
	if want == "" {
		return false
	}
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
