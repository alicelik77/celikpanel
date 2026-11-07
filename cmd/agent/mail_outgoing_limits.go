package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Outgoing mail limits (alpha.128, gap G1; owner decisions of 7 Nov 2026).
//
// postfwd, from the distribution's own repository (Debian 12 bookworm
// 1.35-8, Debian 13 trixie 1.35-10, Ubuntu 22.04 jammy universe 1.35-7,
// Ubuntu 24.04 noble universe 1.35-8, Arch extra 2.03-3; evidence in
// docs/OUTGOING-MAIL-LIMITS.md), is a Postfix policy daemon with its own
// systemd unit and one native rule file. CelikPanel renders that file and
// its lists from the panel's wanted state, checks it with postfwd -C, makes
// the running daemon take it with its own reload while keeping the hourly
// counters (--keep_rates, set by a systemd drop-in), and proves with a
// self-test request that the daemon counts. Only then are the submission
// (587) and smtps (465) services told to ask it, at the end of each message,
// with Postfix's default action DUNNO and a 10 s timeout: a stopped or
// removed postfwd never stops mail. Port 25 (incoming mail) is not touched.
//
// Owner independence (D-022): the rule file, the lists, the drop-in and the
// master.cf settings are ordinary native configuration that keeps working
// with the panel stopped or removed. A rule file, drop-in or master.cf value
// changed on the server is kept and reported, never overwritten (D-031).
// Every write is checked before use and put back when the check, the reload
// or the self-test fails (D-024, D-025).
//
// Measured on the real Debian 13 package (postfwd2 1.35 with Debian's own
// arguments, user postfw): recipients are counted per key, a set() attribute
// works as a key (subscription totals), file: lists match without regard to
// case, and a plain reload clears the counters unless --keep_rates is given.
// The Arch 2.03 program answered identical end-of-message requests from its
// request cache without counting them (--cache=600 in Arch's unit), so the
// Arch drop-in starts it with --cache=0.
//
// Giden posta sınırları: postfwd dağıtımın kendi deposundan gelir; CelikPanel
// yerel kural dosyasını ve listelerini yazar, postfwd -C ile denetler,
// sayaçları koruyarak yeniden yükletir ve öz sınamayla saydığını kanıtlar;
// ancak o zaman 587/465 hizmetleri ileti sonunda, DUNNO varsayılan eylemiyle
// postfwd'a sorar. Sunucuda değiştirilen dosya korunur ve bildirilir.

const (
	outgoingPolicyAddress = "127.0.0.1:10040"
	// A comma separates the restriction from its table: postconf -P refuses
	// whitespace in a master.cf value ("-Pe does not accept whitespace").
	outgoingPolicyCheck     = "check_policy_service,inet:" + outgoingPolicyAddress
	outgoingPolicyTimeout   = "10s"
	outgoingMarkerLine      = "# Managed by CelikPanel: outgoing mail limits (recipients per hour)."
	outgoingBodyHashPrefix  = "# celikpanel-body-sha256: "
	outgoingDropInMarker    = "# Written by CelikPanel (outgoing mail limits)."
	outgoingSelfTestText    = "CelikPanel self-test limit"
	outgoingSelfTestDomain  = "selftest.invalid"
	outgoingStartWait       = 15 * time.Second
	outgoingCommandTimeout  = 60 * time.Second
	outgoingMailLockTimeout = 30 * time.Second

	outgoingConfigAbsent  = "absent"
	outgoingConfigPackage = "package"
	outgoingConfigPanel   = "panel"
	outgoingConfigOwner   = "owner"

	outgoingWiringOn      = "on"
	outgoingWiringOff     = "off"
	outgoingWiringPartial = "partial"
	outgoingWiringOwner   = "owner"
)

// archPostfwdPackageConfigSHA256 is Arch's packaged /etc/postfwd/postfwd.cf
// (postfwd 2.03-3: "id=DEFAULT; action=DUNNO" with a comment), measured from
// the package; a file with exactly these bytes is the distribution's, not
// the owner's.
const archPostfwdPackageConfigSHA256 = "9810fac0f73ee69dd5fb1685efc0deaa77e82f52e63871af0a02eed233758662"

// outgoingLayout is where postfwd's native files live on one family.
type outgoingLayout struct {
	family   string
	program  string
	config   string
	listDir  string
	dropIn   string
	unit     string
	execLine string
}

// outgoingLayoutFor is replaced by tests.
var outgoingLayoutFor = outgoingLayoutForFamily

func outgoingLayoutForFamily(family string) (outgoingLayout, bool) {
	switch family {
	case "apt":
		// Debian's unit reads /etc/default/postfwd (CONF, INET, PORT, RUNAS,
		// ARGS); the drop-in keeps that file the owner's and adds only
		// --keep_rates. /usr/sbin/postfwd is the package's alternative
		// (postfwd2, the prefork program).
		return outgoingLayout{
			family:  family,
			program: "/usr/sbin/postfwd",
			config:  "/etc/postfix/postfwd.cf",
			listDir: "/etc/postfix/celikpanel-postfwd",
			dropIn:  "/etc/systemd/system/postfwd.service.d/celikpanel.conf",
			unit:    "postfwd",
			execLine: "/usr/sbin/postfwd $ARGS --keep_rates --daemon --file $CONF --interface $INET --port $PORT " +
				"--user $RUNAS --group $RUNAS --pidfile $PIDFILE",
		}, true
	case "pacman":
		return outgoingLayout{
			family:  family,
			program: "/usr/bin/postfwd",
			config:  "/etc/postfwd/postfwd.cf",
			listDir: "/etc/postfwd/celikpanel",
			dropIn:  "/etc/systemd/system/postfwd.service.d/celikpanel.conf",
			unit:    "postfwd",
			// Arch's own line with --cache=0 and --keep_rates.
			execLine: "/usr/bin/postfwd --shortlog --summary=600 --cache=0 --cleanup-rates=1200 --keep_rates --daemon " +
				"--file=/etc/postfwd/postfwd.cf --interface=127.0.0.1 --port=10040 --pidfile=/var/run/postfwd.pid " +
				"--user=postfwd --group=postfwd",
		}, true
	}
	return outgoingLayout{}, false
}

func (l outgoingLayout) dropInContent() string {
	lines := []string{outgoingDropInMarker, "# It keeps postfwd's hourly counters across a reload (--keep_rates)."}
	if l.family == "pacman" {
		lines = append(lines[:1],
			"# It keeps postfwd's hourly counters across a reload (--keep_rates) and",
			"# turns off the request cache, which on this postfwd answers repeated",
			"# end-of-message requests without counting them (--cache=0).")
	}
	lines = append(lines,
		"# Remove this file and run systemctl daemon-reload to return to the",
		"# package's own start line.",
		"[Service]", "ExecStart=", "ExecStart="+l.execLine, "")
	return strings.Join(lines, "\n")
}

// Swapped by tests.
var (
	outgoingFamily   = detectPkgFamily
	outgoingLookPath = exec.LookPath
	// The mail launcher (mail_command.go) with this file's own deadline.
	outgoingRun = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, outgoingCommandTimeout)
		defer cancel()
		return executeMailTLSCommand(ctx, name, args...)
	}
	outgoingReadFile = func(path string) ([]byte, error) {
		raw, err := os.ReadFile(path)
		return raw, err
	}
	outgoingWriteFile = func(path string, content []byte) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return secureWriteConfig(path, content, 0o644)
	}
	outgoingRemoveFile = func(path string) error {
		err := secureRemoveConfig(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	outgoingListDir = func(dir string) ([]string, error) {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var names []string
		for _, entry := range entries {
			if entry.Type().IsRegular() {
				names = append(names, entry.Name())
			}
		}
		return names, nil
	}
	outgoingPostfixReload = func() error {
		_, err := applyPostfixVerified(runMailTLSCommand, mailServiceReload)
		return err
	}
	outgoingDial = func(ctx context.Context) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "tcp", outgoingPolicyAddress)
	}
	outgoingSleep = time.Sleep
)

var outgoingMailMu sync.Mutex

// ---- Rendering --------------------------------------------------------------

var outgoingKeyRe = regexp.MustCompile(`^s[0-9]{1,12}$`)

type outgoingRendered struct {
	config string
	lists  map[string]string // file name in listDir -> content
	rules  int
}

func outgoingValidDomain(domain string) bool {
	if domain != strings.ToLower(domain) {
		return false
	}
	_, err := transport.CanonicalMailAddress("postmaster@" + domain)
	return err == nil
}

func outgoingValidAddress(address string) bool {
	canonical, err := transport.CanonicalMailAddress(address)
	return err == nil && canonical == address && address == strings.ToLower(address)
}

func validateOutgoingSpec(spec *transport.OutgoingMailLimitsSpec) error {
	if len(spec.MailboxLimits)+len(spec.Paused) > transport.OutgoingMailMaxMailboxes ||
		len(spec.DomainLimits) > transport.OutgoingMailMaxDomains || len(spec.Subscriptions) > transport.OutgoingMailMaxSubscription {
		return errors.New("the outgoing mail state is larger than this release supports")
	}
	for _, address := range spec.Paused {
		if !outgoingValidAddress(address) {
			return fmt.Errorf("a paused mailbox address is not valid")
		}
	}
	for address, limit := range spec.MailboxLimits {
		if !outgoingValidAddress(address) {
			return fmt.Errorf("a mailbox address is not valid")
		}
		if limit < 1 || limit > transport.OutgoingMailMaxPerHour {
			return fmt.Errorf("a mailbox limit is outside 1..%d", transport.OutgoingMailMaxPerHour)
		}
	}
	for domain, limit := range spec.DomainLimits {
		if !outgoingValidDomain(domain) {
			return fmt.Errorf("a domain name is not valid")
		}
		if limit < 1 || limit > transport.OutgoingMailMaxPerHour {
			return fmt.Errorf("a domain limit is outside 1..%d", transport.OutgoingMailMaxPerHour)
		}
	}
	seen := map[string]bool{}
	for _, sub := range spec.Subscriptions {
		if !outgoingKeyRe.MatchString(sub.Key) || seen[sub.Key] {
			return fmt.Errorf("a hosting account key is not valid")
		}
		seen[sub.Key] = true
		if sub.Limit < 1 || sub.Limit > transport.OutgoingMailMaxPerHour {
			return fmt.Errorf("a hosting account limit is outside 1..%d", transport.OutgoingMailMaxPerHour)
		}
		for _, domain := range sub.Domains {
			if !outgoingValidDomain(domain) {
				return fmt.Errorf("a domain name is not valid")
			}
		}
	}
	return nil
}

func outgoingListContent(values []string) string {
	sort.Strings(values)
	var b strings.Builder
	b.WriteString("# Written by CelikPanel from the panel's outgoing mail settings.\n")
	for _, value := range values {
		b.WriteString(value)
		b.WriteByte('\n')
	}
	return b.String()
}

func outgoingRule(id, match, action string) string {
	return "id=" + id + " ; protocol_state==END-OF-MESSAGE ; " + match + " ; action=" + action + "\n"
}

// renderOutgoingConfig builds postfwd's rule file and lists. Rules stop at the
// first postfix action, so the order is the meaning: the self-test, the
// security pause, the subscription marks, then the mailbox, domain and
// subscription counters, and DUNNO for everything else. A rule is written
// only for a list that is not empty.
func renderOutgoingConfig(layout outgoingLayout, spec *transport.OutgoingMailLimitsSpec) outgoingRendered {
	out := outgoingRendered{lists: map[string]string{}}
	var body strings.Builder
	add := func(line string) {
		body.WriteString(line)
		out.rules++
	}
	list := func(name string, values []string) string {
		out.lists[name] = outgoingListContent(values)
		return "file:" + layout.listDir + "/" + name
	}
	add(outgoingRule("CP_SELFTEST",
		`sasl_username=~/^celikpanel-selftest-[a-z0-9]+@selftest\.invalid$/`,
		"rcpt(sasl_username/1/600/450 4.7.1 "+outgoingSelfTestText+")"))
	if len(spec.Paused) > 0 {
		add(outgoingRule("CP_PAUSED", "sasl_username=="+list("paused.list", append([]string(nil), spec.Paused...)),
			"550 5.7.1 "+transport.OutgoingMailPausedText))
	}
	subs := append([]transport.OutgoingMailSubscription(nil), spec.Subscriptions...)
	sort.Slice(subs, func(i, j int) bool { return subs[i].Key < subs[j].Key })
	subByLimit := map[int][]string{}
	for _, sub := range subs {
		if len(sub.Domains) == 0 {
			continue
		}
		add(outgoingRule("CP_MARK_"+sub.Key, "sender_domain=="+list("subscription-"+sub.Key+".domains", append([]string(nil), sub.Domains...)),
			"set(cp_subscription="+sub.Key+")"))
		subByLimit[sub.Limit] = append(subByLimit[sub.Limit], sub.Key)
	}
	byLimit := func(values map[string]int) (map[int][]string, []int) {
		grouped := map[int][]string{}
		for value, limit := range values {
			grouped[limit] = append(grouped[limit], value)
		}
		limits := make([]int, 0, len(grouped))
		for limit := range grouped {
			limits = append(limits, limit)
		}
		sort.Ints(limits)
		return grouped, limits
	}
	mailboxes, limits := byLimit(spec.MailboxLimits)
	for _, limit := range limits {
		add(outgoingRule(fmt.Sprintf("CP_MAILBOX_%d", limit),
			"sasl_username=="+list(fmt.Sprintf("mailbox-%d.list", limit), mailboxes[limit]),
			fmt.Sprintf("rcpt(sasl_username/%d/3600/450 4.7.1 "+transport.OutgoingMailMailboxText+")", limit, limit)))
	}
	domains, limits := byLimit(spec.DomainLimits)
	for _, limit := range limits {
		add(outgoingRule(fmt.Sprintf("CP_DOMAIN_%d", limit),
			"sender_domain=="+list(fmt.Sprintf("domain-%d.list", limit), domains[limit]),
			fmt.Sprintf("rcpt(sender_domain/%d/3600/450 4.7.1 "+transport.OutgoingMailDomainText+")", limit, limit)))
	}
	subLimits := make([]int, 0, len(subByLimit))
	for limit := range subByLimit {
		subLimits = append(subLimits, limit)
	}
	sort.Ints(subLimits)
	for _, limit := range subLimits {
		add(outgoingRule(fmt.Sprintf("CP_SUBSCRIPTION_%d", limit),
			"cp_subscription=="+list(fmt.Sprintf("subscription-%d.list", limit), subByLimit[limit]),
			fmt.Sprintf("rcpt(cp_subscription/%d/3600/450 4.7.1 "+transport.OutgoingMailSubscriptionText+")", limit, limit)))
	}
	body.WriteString("id=CP_DEFAULT ; action=DUNNO\n")
	out.rules++
	text := body.String()
	sum := sha256.Sum256([]byte(text))
	out.config = outgoingMarkerLine + "\n" +
		"# Mail keeps flowing if postfwd stops: Postfix asks it from the submission\n" +
		"# and smtps services only, with the default action DUNNO.\n" +
		"# A change made here by hand is kept: CelikPanel then stops writing this file\n" +
		"# and the panel says so. To hand it back, delete this file and save the\n" +
		"# outgoing mail settings in the panel again.\n" +
		outgoingBodyHashPrefix + hex.EncodeToString(sum[:]) + "\n" + text
	return out
}

// classifyOutgoingConfig says whose the rule file is.
func classifyOutgoingConfig(raw []byte, exists bool) string {
	if !exists {
		return outgoingConfigAbsent
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) == archPostfwdPackageConfigSHA256 {
		return outgoingConfigPackage
	}
	text := string(raw)
	if !strings.HasPrefix(text, outgoingMarkerLine+"\n") {
		return outgoingConfigOwner
	}
	index := strings.Index(text, "\n"+outgoingBodyHashPrefix)
	if index < 0 {
		return outgoingConfigOwner
	}
	rest := text[index+1+len(outgoingBodyHashPrefix):]
	hash, body, found := strings.Cut(rest, "\n")
	if !found {
		return outgoingConfigOwner
	}
	sum = sha256.Sum256([]byte(body))
	if hex.EncodeToString(sum[:]) != strings.TrimSpace(hash) {
		return outgoingConfigOwner
	}
	return outgoingConfigPanel
}

// ---- Native reads -----------------------------------------------------------

func outgoingReadOptional(path string) ([]byte, bool, error) {
	raw, err := outgoingReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

// outgoingServiceParams reads a master.cf service's -o parameters through
// `postconf -P <service>/inet`, one "service/inet/name = value" per line.
func outgoingServiceParams(ctx context.Context, service string) (map[string]string, error) {
	out, err := outgoingRun(ctx, "postconf", "-P", service+"/inet")
	if err != nil {
		return nil, fmt.Errorf("postconf -P %s/inet: %s", service, boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
	}
	params := map[string]string{}
	prefix := service + "/inet/"
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		name, value, found := strings.Cut(strings.TrimPrefix(line, prefix), "=")
		if found {
			params[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	return params, nil
}

// outgoingWiringPlan is what one service needs and what blocks it.
type outgoingWiringPlan struct {
	state  string            // on, off, partial, owner
	writes map[string]string // "service/inet/name" -> value
	clears []string          // "service/inet/name"
	before map[string]string // previous -o values ("" = not set)
	owner  []string
	absent []string // services that do not exist
}

var outgoingWiringValues = [][2]string{
	{"smtpd_end_of_data_restrictions", outgoingPolicyCheck},
	{"smtpd_policy_service_default_action", "DUNNO"},
	{"smtpd_policy_service_timeout", outgoingPolicyTimeout},
}

func planOutgoingWiring(ctx context.Context, enable bool) (outgoingWiringPlan, error) {
	plan := outgoingWiringPlan{writes: map[string]string{}, before: map[string]string{}}
	// A main.cf value would be replaced for 587/465 by a per-service -o: the
	// owner's own end-of-data checks are then kept and reported instead.
	globalOut, err := outgoingRun(ctx, "postconf", "-n", "-h", "smtpd_end_of_data_restrictions")
	if err != nil {
		return plan, fmt.Errorf("postconf -n -h smtpd_end_of_data_restrictions: %s", boundedNativeDetail(firstNonEmpty(string(globalOut), err.Error())))
	}
	global, _ := postconfOneValue(globalOut)
	on, off := 0, 0
	for _, service := range submissionServices {
		if fields, ok := masterServiceFields(func(name string, args ...string) ([]byte, error) {
			return outgoingRun(ctx, name, args...)
		}, service+"/inet"); !ok || len(fields) == 0 {
			plan.absent = append(plan.absent, service)
			continue
		}
		params, err := outgoingServiceParams(ctx, service)
		if err != nil {
			return plan, err
		}
		current := params["smtpd_end_of_data_restrictions"]
		switch {
		case sameRestrictions(current, outgoingPolicyCheck):
			on++
		case current == "" && global == "":
			off++
		default:
			plan.owner = append(plan.owner, service)
			continue
		}
		for _, kv := range outgoingWiringValues {
			key := service + "/inet/" + kv[0]
			value, set := params[kv[0]]
			if set {
				plan.before[key] = value
			} else {
				plan.before[key] = ""
			}
			same := value == kv[1] || (kv[0] == "smtpd_end_of_data_restrictions" && sameRestrictions(value, kv[1]))
			if enable {
				if !same {
					plan.writes[key] = kv[1]
				}
			} else if set && same {
				plan.clears = append(plan.clears, key)
			}
		}
	}
	switch {
	case len(plan.owner) > 0:
		plan.state = outgoingWiringOwner
	case on > 0 && off == 0 && len(plan.absent) == 0:
		plan.state = outgoingWiringOn
	case on > 0:
		plan.state = outgoingWiringPartial
	default:
		plan.state = outgoingWiringOff
	}
	return plan, nil
}

func outgoingWiringDetail(plan outgoingWiringPlan) string {
	var notes []string
	if len(plan.owner) > 0 {
		notes = append(notes, fmt.Sprintf("the server owner set other end-of-message checks for %s (master.cf or main.cf smtpd_end_of_data_restrictions); CelikPanel keeps them and does not add the limit there",
			strings.Join(plan.owner, " and ")))
	}
	if len(plan.absent) > 0 {
		notes = append(notes, fmt.Sprintf("%s is not set up in master.cf yet (mail sending is set up with the mail server)", strings.Join(plan.absent, " and ")))
	}
	return joinNotes(notes...)
}

// ---- Self-test --------------------------------------------------------------

// outgoingSelfTest sends two end-of-message requests for a name no mailbox
// can have (selftest.invalid) to the running postfwd. The first must pass and
// the second must be refused by the self-test rule: then the daemon reads
// CelikPanel's rules and counts recipients.
func outgoingSelfTest(ctx context.Context) (bool, string) {
	token := make([]byte, 6)
	if _, err := rand.Read(token); err != nil {
		return false, "no random name for the self-test"
	}
	user := "celikpanel-selftest-" + hex.EncodeToString(token) + "@" + outgoingSelfTestDomain
	answers := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		answer, err := outgoingPolicyAsk(ctx, user, i)
		if err != nil {
			return false, "postfwd did not answer on " + outgoingPolicyAddress + ": " + boundedNativeDetail(err.Error())
		}
		answers = append(answers, answer)
	}
	if strings.EqualFold(answers[0], "dunno") && strings.Contains(answers[1], outgoingSelfTestText) {
		return true, ""
	}
	return false, fmt.Sprintf("postfwd answered %q and %q instead of passing the first test message and refusing the second; it does not count with CelikPanel's rules", boundedNativeDetail(answers[0]), boundedNativeDetail(answers[1]))
}

func outgoingPolicyAsk(ctx context.Context, user string, n int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := outgoingDial(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	request := strings.Join([]string{
		"request=smtpd_access_policy", "protocol_state=END-OF-MESSAGE", "protocol_name=ESMTP",
		"client_address=127.0.0.1", "client_name=localhost", "reverse_client_name=localhost",
		"helo_name=localhost", "sasl_method=PLAIN", "sasl_username=" + user, "sender=" + user,
		"recipient=", "recipient_count=1", "size=100", "queue_id=",
		fmt.Sprintf("instance=celikpanel.selftest.%d", n), "",
	}, "\n") + "\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		return "", err
	}
	reader := bufio.NewReader(conn)
	var answer string
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			if answer != "" || err != nil {
				break
			}
			continue
		}
		if value, ok := strings.CutPrefix(line, "action="); ok {
			answer = value
		}
		if err != nil {
			break
		}
	}
	if answer == "" {
		return "", errors.New("no action in the answer")
	}
	return answer, nil
}

// ---- Status -----------------------------------------------------------------

func outgoingServiceActive(ctx context.Context, unit string) bool {
	out, err := outgoingRun(ctx, "systemctl", "is-active", unit)
	return err == nil && strings.TrimSpace(string(out)) == "active"
}

func readOutgoingStatus(ctx context.Context, verify bool) transport.OutgoingMailLimitsStatus {
	status := transport.OutgoingMailLimitsStatus{}
	if _, err := outgoingLookPath("postconf"); err != nil {
		status.Detail = "no Postfix is installed on this server"
		return status
	}
	status.Postfix = true
	family := outgoingFamily()
	status.Family = family
	layout, ok := outgoingLayoutFor(family)
	if !ok {
		status.Error = "this server's package family is not known"
		return status
	}
	status.ConfigPath = layout.config
	if _, err := outgoingLookPath(layout.program); err != nil {
		status.Detail = "postfwd is not installed"
		return status
	}
	status.Installed = true
	raw, exists, err := outgoingReadOptional(layout.config)
	if err != nil {
		status.Error = "postfwd's rule file could not be read: " + boundedNativeDetail(err.Error())
		return status
	}
	status.ConfigState = classifyOutgoingConfig(raw, exists)
	status.ServiceActive = outgoingServiceActive(ctx, layout.unit)
	plan, err := planOutgoingWiring(ctx, true)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Wiring = plan.state
	status.WiringDetail = outgoingWiringDetail(plan)
	if verify && status.ServiceActive && status.ConfigState == outgoingConfigPanel {
		status.Verified, status.Detail = outgoingSelfTest(ctx)
	}
	return status
}

// OutgoingMailLimitsStatusV1 reads, without changing anything, whether the
// outgoing mail limits work on this server now. The self-test it makes counts
// only a name no mailbox can have.
// OutgoingMailLimitsStatusV1, giden posta sınırlarının şu an çalışıp
// çalışmadığını hiçbir şeyi değiştirmeden okur.
func (a *Agent) OutgoingMailLimitsStatusV1(_ *transport.Empty, resp *transport.OutgoingMailLimitsStatus) error {
	*resp = readOutgoingStatus(context.Background(), true)
	return nil
}

// ---- Apply ------------------------------------------------------------------

type outgoingSnapshot struct {
	path    string
	content []byte
	exists  bool
}

func takeOutgoingSnapshot(path string) (outgoingSnapshot, error) {
	raw, exists, err := outgoingReadOptional(path)
	if err != nil {
		return outgoingSnapshot{}, err
	}
	return outgoingSnapshot{path: path, content: raw, exists: exists}, nil
}

func (s outgoingSnapshot) restore() error {
	if !s.exists {
		return outgoingRemoveFile(s.path)
	}
	return outgoingWriteFile(s.path, s.content)
}

func lockOutgoingMailMutex(ctx context.Context) bool {
	deadline := time.Now().Add(outgoingMailLockTimeout)
	for {
		if mailMutex.TryLock() {
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		outgoingSleep(200 * time.Millisecond)
	}
}

// ApplyOutgoingMailLimitsV1 makes the server hold the wanted outgoing mail
// state (see the file comment). It is idempotent: the same spec again writes
// nothing, still checks postfwd and answers with the status.
// ApplyOutgoingMailLimitsV1 sunucuyu istenen giden posta durumuna getirir;
// aynı istek tekrar geldiğinde hiçbir şey yazmaz.
func (a *Agent) ApplyOutgoingMailLimitsV1(req *transport.OutgoingMailLimitsSpec, resp *transport.OutgoingMailLimitsApplyResponse) error {
	*resp = transport.OutgoingMailLimitsApplyResponse{}
	if req == nil {
		resp.Code, resp.Error = transport.OutgoingMailInvalidSpec, "the outgoing mail request is empty"
		return nil
	}
	outgoingMailMu.Lock()
	defer outgoingMailMu.Unlock()
	ctx := context.Background()
	applyOutgoingMailLimits(ctx, req, resp)
	resp.Status = readOutgoingStatus(ctx, false)
	if resp.Applied && req.Enabled {
		resp.Status.Verified = resp.Verified
	}
	return nil
}

func applyOutgoingMailLimits(ctx context.Context, spec *transport.OutgoingMailLimitsSpec, resp *transport.OutgoingMailLimitsApplyResponse) {
	refuse := func(code, text string) {
		resp.Applied, resp.Code, resp.Error = false, code, text
	}
	step := func(text string) { resp.Steps = append(resp.Steps, text) }
	if _, err := outgoingLookPath("postconf"); err != nil {
		refuse(transport.OutgoingMailNoPostfix, "no Postfix is installed on this server, so there is no outgoing mail to limit")
		return
	}
	if err := validateOutgoingSpec(spec); err != nil {
		refuse(transport.OutgoingMailInvalidSpec, err.Error())
		return
	}
	family := outgoingFamily()
	layout, ok := outgoingLayoutFor(family)
	if !spec.Enabled {
		if !lockOutgoingMailMutex(ctx) {
			refuse(transport.OutgoingMailBusy, "another mail change is running; nothing was changed. It is tried again in a few minutes.")
			return
		}
		defer mailMutex.Unlock()
		changed, err := outgoingSetWiring(ctx, false, step)
		if err != nil {
			refuse(transport.OutgoingMailPostfixFailed, err.Error())
			return
		}
		resp.Applied, resp.Changed = true, changed
		step("Postfix no longer asks postfwd; outgoing mail is not limited")
		return
	}
	if !ok {
		refuse(transport.OutgoingMailNotInstalled, "this server's package family is not known, so postfwd's files cannot be placed")
		return
	}
	if _, err := outgoingLookPath(layout.program); err != nil {
		refuse(transport.OutgoingMailNotInstalled, "postfwd is not installed. The administrator installs \"Outgoing mail limits (postfwd)\" from Components; nothing was changed and mail keeps flowing.")
		return
	}

	// 1. Whose rule file is it?
	raw, exists, err := outgoingReadOptional(layout.config)
	if err != nil {
		refuse(transport.OutgoingMailCheckFailed, "postfwd's rule file could not be read: "+boundedNativeDetail(err.Error()))
		return
	}
	if classifyOutgoingConfig(raw, exists) == outgoingConfigOwner {
		refuse(transport.OutgoingMailOwnerConfig, fmt.Sprintf("%s was changed on the server, so CelikPanel keeps it and does not write the limits. To let CelikPanel write it, the server owner moves the own rules elsewhere, deletes the file and saves the outgoing mail settings again.", layout.config))
		return
	}
	rendered := renderOutgoingConfig(layout, spec)

	// 2. Snapshots of everything this may write.
	var snapshots []outgoingSnapshot
	snap := func(path string) error {
		s, err := takeOutgoingSnapshot(path)
		if err != nil {
			return err
		}
		snapshots = append(snapshots, s)
		return nil
	}
	if err := snap(layout.config); err != nil {
		refuse(transport.OutgoingMailCheckFailed, "postfwd's rule file could not be read: "+boundedNativeDetail(err.Error()))
		return
	}
	existingLists, err := outgoingListDir(layout.listDir)
	if err != nil {
		refuse(transport.OutgoingMailCheckFailed, "postfwd's list folder could not be read: "+boundedNativeDetail(err.Error()))
		return
	}
	listNames := map[string]bool{}
	for _, name := range existingLists {
		listNames[name] = true
	}
	for name := range rendered.lists {
		listNames[name] = true
	}
	for name := range listNames {
		if err := snap(filepath.Join(layout.listDir, name)); err != nil {
			refuse(transport.OutgoingMailCheckFailed, "a postfwd list could not be read: "+boundedNativeDetail(err.Error()))
			return
		}
	}
	dropInRaw, dropInExists, err := outgoingReadOptional(layout.dropIn)
	if err != nil {
		refuse(transport.OutgoingMailCheckFailed, "postfwd's systemd drop-in could not be read: "+boundedNativeDetail(err.Error()))
		return
	}
	dropInOwner := dropInExists && !strings.HasPrefix(string(dropInRaw), outgoingDropInMarker+"\n")
	dropInChanged := !dropInOwner && string(dropInRaw) != layout.dropInContent()
	if dropInChanged {
		if err := snap(layout.dropIn); err != nil {
			refuse(transport.OutgoingMailCheckFailed, err.Error())
			return
		}
	}

	filesChanged := false
	restore := func() []error {
		var errs []error
		for i := len(snapshots) - 1; i >= 0; i-- {
			if err := snapshots[i].restore(); err != nil {
				errs = append(errs, err)
			}
		}
		return errs
	}
	fail := func(code string, cause error) {
		detail := cause.Error()
		if filesChanged {
			if errs := restore(); len(errs) > 0 {
				detail += "; the earlier postfwd files could not all be put back (" + boundedNativeDetail(errors.Join(errs...).Error()) +
					"); the server owner checks " + layout.config + " and " + layout.listDir
			} else {
				detail += "; the earlier postfwd files were put back"
				if dropInChanged {
					_, _ = outgoingRun(ctx, "systemctl", "daemon-reload")
				}
				_, _ = outgoingRun(ctx, "systemctl", "try-reload-or-restart", layout.unit)
			}
		}
		detail += ". Mail keeps flowing: Postfix was not told to ask postfwd."
		refuse(code, detail)
	}

	// 3. Lists, then the checked rule file.
	for name, content := range rendered.lists {
		path := filepath.Join(layout.listDir, name)
		current, exists, _ := outgoingReadOptional(path)
		if exists && string(current) == content {
			continue
		}
		filesChanged = true
		if err := outgoingWriteFile(path, []byte(content)); err != nil {
			fail(transport.OutgoingMailCheckFailed, fmt.Errorf("write %s: %s", path, boundedNativeDetail(err.Error())))
			return
		}
	}
	for _, name := range existingLists {
		if _, keep := rendered.lists[name]; keep || !strings.HasSuffix(name, ".list") && !strings.HasSuffix(name, ".domains") {
			continue
		}
		filesChanged = true
		if err := outgoingRemoveFile(filepath.Join(layout.listDir, name)); err != nil {
			fail(transport.OutgoingMailCheckFailed, fmt.Errorf("remove an old list: %s", boundedNativeDetail(err.Error())))
			return
		}
	}
	configChanged := string(raw) != rendered.config
	if configChanged {
		candidate := layout.config + ".celikpanel-check"
		if err := outgoingWriteFile(candidate, []byte(rendered.config)); err != nil {
			fail(transport.OutgoingMailCheckFailed, fmt.Errorf("write the rule file for its check: %s", boundedNativeDetail(err.Error())))
			return
		}
		out, err := outgoingRun(ctx, layout.program, "-f", candidate, "-C")
		_ = outgoingRemoveFile(candidate)
		if err != nil || strings.Count(string(out), "Rule ") < rendered.rules {
			fail(transport.OutgoingMailCheckFailed, fmt.Errorf("postfwd refused the new rules (postfwd -C): %s", boundedNativeDetail(firstNonEmpty(string(out), errString(err)))))
			return
		}
		filesChanged = true
		if err := outgoingWriteFile(layout.config, []byte(rendered.config)); err != nil {
			fail(transport.OutgoingMailCheckFailed, fmt.Errorf("write %s: %s", layout.config, boundedNativeDetail(err.Error())))
			return
		}
		step("postfwd's rules were written and checked (postfwd -C)")
	}

	// 4. The drop-in that keeps the counters across a reload.
	if dropInChanged {
		filesChanged = true
		if err := outgoingWriteFile(layout.dropIn, []byte(layout.dropInContent())); err != nil {
			fail(transport.OutgoingMailServiceFailed, fmt.Errorf("write %s: %s", layout.dropIn, boundedNativeDetail(err.Error())))
			return
		}
		if out, err := outgoingRun(ctx, "systemctl", "daemon-reload"); err != nil {
			fail(transport.OutgoingMailServiceFailed, fmt.Errorf("systemctl daemon-reload: %s", boundedNativeDetail(firstNonEmpty(string(out), err.Error()))))
			return
		}
		step("postfwd keeps its hourly counters when it reloads (systemd drop-in)")
	}

	// 5. The running daemon takes the rules.
	active := outgoingServiceActive(ctx, layout.unit)
	var action []string
	switch {
	case !active:
		action = []string{"enable", "--now", layout.unit}
	case dropInChanged:
		action = []string{"restart", layout.unit}
	case configChanged || filesChanged:
		action = []string{"reload", layout.unit}
	}
	if !active {
		// Started at boot from now on, like any service the owner enables.
		if out, err := outgoingRun(ctx, "systemctl", action...); err != nil {
			fail(transport.OutgoingMailServiceFailed, fmt.Errorf("postfwd did not start (systemctl %s): %s", strings.Join(action, " "), boundedNativeDetail(firstNonEmpty(string(out), err.Error()))))
			return
		}
		step("postfwd was started and enabled at boot")
	} else if action != nil {
		if out, err := outgoingRun(ctx, "systemctl", action...); err != nil {
			fail(transport.OutgoingMailServiceFailed, fmt.Errorf("postfwd did not take the rules (systemctl %s): %s", strings.Join(action, " "), boundedNativeDetail(firstNonEmpty(string(out), err.Error()))))
			return
		}
		step("postfwd took the new rules and kept its counters")
	} else {
		_, _ = outgoingRun(ctx, "systemctl", "enable", layout.unit)
	}

	// 6. Proof that it counts, waiting for a starting daemon.
	var verified bool
	var why string
	deadline := time.Now().Add(outgoingStartWait)
	for {
		verified, why = outgoingSelfTest(ctx)
		if verified || time.Now().After(deadline) {
			break
		}
		outgoingSleep(time.Second)
	}
	if !verified {
		fail(transport.OutgoingMailSelfTestFailed, errors.New(why))
		return
	}
	resp.Verified = true
	step("a test message was counted and the second one refused, so the limits work")

	// 7. Postfix asks postfwd from 587 and 465.
	if !lockOutgoingMailMutex(ctx) {
		refuse(transport.OutgoingMailBusy, "another mail change is running; postfwd holds the new rules, and Postfix is told to ask it on the next try in a few minutes.")
		resp.Verified = true
		return
	}
	defer mailMutex.Unlock()
	wired, err := outgoingSetWiring(ctx, true, step)
	if err != nil {
		var owner *outgoingOwnerWiringError
		if errors.As(err, &owner) {
			refuse(transport.OutgoingMailOwnerWiring, err.Error())
			return
		}
		refuse(transport.OutgoingMailPostfixFailed, err.Error()+". postfwd holds the rules; mail keeps flowing without the limit.")
		return
	}
	resp.Applied = true
	resp.Changed = filesChanged || wired
	if dropInOwner {
		resp.Detail = joinNotes(resp.Detail, layout.dropIn+" was changed on the server and kept; if postfwd forgets its counters on a reload, the server owner adds --keep_rates there")
	}
}

type outgoingOwnerWiringError struct{ detail string }

func (e *outgoingOwnerWiringError) Error() string { return e.detail }

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// outgoingSetWiring adds (enable) or removes the policy check on submission
// and smtps, then makes Postfix take it with its verified reload. A failed
// reload puts the earlier values back.
func outgoingSetWiring(ctx context.Context, enable bool, step func(string)) (bool, error) {
	plan, err := planOutgoingWiring(ctx, enable)
	if err != nil {
		return false, err
	}
	if enable && plan.state == outgoingWiringOwner {
		return false, &outgoingOwnerWiringError{detail: "Postfix was not told to ask postfwd: " + outgoingWiringDetail(plan)}
	}
	if enable && len(plan.absent) == len(submissionServices) {
		return false, errors.New("mail sending (submission on 587 and 465) is not set up on this server yet")
	}
	if len(plan.writes) == 0 && len(plan.clears) == 0 {
		return false, nil
	}
	var done []string
	putBack := func() error {
		var errs []error
		for _, key := range done {
			var out []byte
			var err error
			if before := plan.before[key]; before != "" {
				out, err = outgoingRun(ctx, "postconf", "-P", key+"="+before)
			} else {
				out, err = outgoingRun(ctx, "postconf", "-PX", key)
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %s", key, boundedNativeDetail(firstNonEmpty(string(out), err.Error()))))
			}
		}
		return errors.Join(errs...)
	}
	keys := make([]string, 0, len(plan.writes))
	for key := range plan.writes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if out, err := outgoingRun(ctx, "postconf", "-P", key+"="+plan.writes[key]); err != nil {
			_ = putBack()
			return false, fmt.Errorf("postconf -P %s: %s", key, boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
		}
		done = append(done, key)
	}
	for _, key := range plan.clears {
		if out, err := outgoingRun(ctx, "postconf", "-PX", key); err != nil {
			_ = putBack()
			return false, fmt.Errorf("postconf -PX %s: %s", key, boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
		}
		done = append(done, key)
	}
	if err := outgoingPostfixReload(); err != nil {
		cause := fmt.Errorf("Postfix did not take the change (%s)", boundedNativeDetail(err.Error()))
		if restoreErr := putBack(); restoreErr != nil {
			return false, fmt.Errorf("%w, and the earlier master.cf values could not all be put back (%s); the server owner runs postconf -P submission/inet and postfix check", cause, boundedNativeDetail(restoreErr.Error()))
		}
		if reloadErr := outgoingPostfixReload(); reloadErr != nil {
			return false, fmt.Errorf("%w; the earlier master.cf values were put back but Postfix did not reload them either (%s)", cause, boundedNativeDetail(reloadErr.Error()))
		}
		return false, fmt.Errorf("%w; the earlier master.cf values were put back", cause)
	}
	if enable {
		step("Postfix asks postfwd at the end of each message on 587 and 465 (mail flows if postfwd stops)")
	} else {
		step("the policy check was removed from 587 and 465")
	}
	return true, nil
}

// ---- Install preparation ----------------------------------------------------

// preparePostfwd runs inside the explicit component install, before the unit
// is enabled and started: a rule file that only says DUNNO (Debian ships
// none, so its unit would fail) and the drop-in. An existing file the owner
// wrote is never replaced.
func preparePostfwd(family string) error {
	layout, ok := outgoingLayoutFor(family)
	if !ok {
		return nil
	}
	raw, exists, err := outgoingReadOptional(layout.config)
	if err != nil {
		return fmt.Errorf("read %s: %w", layout.config, err)
	}
	switch classifyOutgoingConfig(raw, exists) {
	case outgoingConfigAbsent, outgoingConfigPackage:
		rendered := renderOutgoingConfig(layout, &transport.OutgoingMailLimitsSpec{})
		if err := outgoingWriteFile(layout.config, []byte(rendered.config)); err != nil {
			return fmt.Errorf("write %s: %w", layout.config, err)
		}
	}
	dropInRaw, dropInExists, err := outgoingReadOptional(layout.dropIn)
	if err != nil {
		return fmt.Errorf("read %s: %w", layout.dropIn, err)
	}
	if !dropInExists || (strings.HasPrefix(string(dropInRaw), outgoingDropInMarker+"\n") && string(dropInRaw) != layout.dropInContent()) {
		if err := outgoingWriteFile(layout.dropIn, []byte(layout.dropInContent())); err != nil {
			return fmt.Errorf("write %s: %w", layout.dropIn, err)
		}
		if out, err := outgoingRun(context.Background(), "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %s", boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
		}
	}
	return nil
}
