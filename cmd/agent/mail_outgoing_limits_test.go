package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// fakeOutgoingHost is postconf, systemctl, postfwd and the policy socket as
// the Agent sees them.
type fakeOutgoingHost struct {
	mu         sync.Mutex
	commands   []string
	params     map[string]map[string]string // service -> -o params
	global     string
	active     bool
	countWorks bool
	checkFails bool
	// checkExitsOne: postfwd 1.x ends --showconfig with "exit 1" after
	// printing the rules (Debian 13 postfwd 1.35-10).
	checkExitsOne bool
	reloadFails   bool
	reloads       int
	seen          map[string]int
	// stalePaused: answers in which a resumed mailbox is still answered as
	// paused (a prefork child with the old rules); -1: always.
	stalePaused map[string]int
}

func (h *fakeOutgoingHost) run(_ context.Context, name string, args ...string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	line := name + " " + strings.Join(args, " ")
	h.commands = append(h.commands, line)
	switch {
	case name == "postconf" && len(args) == 3 && args[0] == "-n" && args[1] == "-h":
		return []byte(h.global + "\n"), nil
	case name == "postconf" && len(args) == 2 && args[0] == "-F":
		service := strings.TrimSuffix(args[1], "/inet")
		if _, ok := h.params[service]; !ok {
			return nil, nil
		}
		return []byte(fmt.Sprintf("%[1]s/inet/service = %[1]s\n%[1]s/inet/type = inet\n%[1]s/inet/private = n\n%[1]s/inet/unprivileged = -\n%[1]s/inet/chroot = y\n%[1]s/inet/wakeup = -\n%[1]s/inet/process_limit = -\n%[1]s/inet/command = smtpd\n", service)), nil
	case name == "postconf" && len(args) == 2 && args[0] == "-P" && !strings.Contains(args[1], "="):
		service := strings.TrimSuffix(args[1], "/inet")
		var b strings.Builder
		for key, value := range h.params[service] {
			fmt.Fprintf(&b, "%s/inet/%s = %s\n", service, key, value)
		}
		return []byte(b.String()), nil
	case name == "postconf" && args[0] == "-P":
		key, value, _ := strings.Cut(args[1], "=")
		parts := strings.SplitN(key, "/", 3)
		h.params[parts[0]][parts[2]] = value
		return nil, nil
	case name == "postconf" && args[0] == "-PX":
		parts := strings.SplitN(args[1], "/", 3)
		delete(h.params[parts[0]], parts[2])
		return nil, nil
	case name == "systemctl" && args[0] == "is-active":
		if h.active {
			return []byte("active\n"), nil
		}
		return []byte("inactive\n"), errors.New("exit status 3")
	case name == "systemctl" && (args[0] == "enable" && len(args) == 3 || args[0] == "restart"):
		h.active = true
		return nil, nil
	case name == "systemctl":
		return nil, nil
	case strings.HasSuffix(name, "postfwd") && len(args) == 3 && args[2] == "-C":
		if h.checkFails {
			return []byte("syntax error"), errors.New("exit status 1")
		}
		raw, err := os.ReadFile(args[1])
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		n := 0
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "id=") {
				fmt.Fprintf(&b, "Rule %3d: %s\n", n, line)
				n++
			}
		}
		return []byte(b.String()), nil
	}
	return nil, nil
}

func (h *fakeOutgoingHost) dial(_ context.Context) (net.Conn, error) {
	h.mu.Lock()
	active := h.active
	h.mu.Unlock()
	if !active {
		return nil, errors.New("connection refused")
	}
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		reader := bufio.NewReader(server)
		user := ""
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if value, ok := strings.CutPrefix(line, "sasl_username="); ok {
				user = value
			}
			if line == "" {
				break
			}
		}
		h.mu.Lock()
		h.seen[user]++
		n := h.seen[user]
		works := h.countWorks
		h.mu.Unlock()
		answer := "action=DUNNO\n\n"
		if works && n > 1 {
			answer = "action=450 4.7.1 " + outgoingSelfTestText + "\n\n"
		}
		h.mu.Lock()
		if left, ok := h.stalePaused[user]; ok && left != 0 {
			answer = "action=550 5.7.1 " + transport.OutgoingMailPausedText + "\n\n"
			if left > 0 {
				h.stalePaused[user] = left - 1
			}
		}
		h.mu.Unlock()
		_, _ = server.Write([]byte(answer))
	}()
	return client, nil
}

func (h *fakeOutgoingHost) count(prefix string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, c := range h.commands {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func installFakeOutgoingHost(t *testing.T) (*fakeOutgoingHost, outgoingLayout) {
	t.Helper()
	dir := t.TempDir()
	layout := outgoingLayout{
		family: "apt", program: filepath.Join(dir, "usr/sbin/postfwd"), config: filepath.Join(dir, "etc/postfix/postfwd.cf"),
		listDir: filepath.Join(dir, "etc/postfix/celikpanel-postfwd"), dropIn: filepath.Join(dir, "etc/systemd/postfwd.service.d/celikpanel.conf"),
		unit: "postfwd", execLine: "/usr/sbin/postfwd $ARGS --keep_rates --daemon",
	}
	host := &fakeOutgoingHost{
		params: map[string]map[string]string{
			"submission": {"syslog_name": "postfix/submission"},
			"smtps":      {"syslog_name": "postfix/smtps"},
		},
		countWorks: true,
		seen:       map[string]int{},
	}
	saved := []any{outgoingFamily, outgoingLookPath, outgoingRun, outgoingWriteFile, outgoingRemoveFile, outgoingPostfixReload, outgoingDial, outgoingSleep, outgoingLayoutFor}
	t.Cleanup(func() {
		outgoingFamily = saved[0].(func() string)
		outgoingLookPath = saved[1].(func(string) (string, error))
		outgoingRun = saved[2].(func(context.Context, string, ...string) ([]byte, error))
		outgoingWriteFile = saved[3].(func(string, []byte) error)
		outgoingRemoveFile = saved[4].(func(string) error)
		outgoingPostfixReload = saved[5].(func() error)
		outgoingDial = saved[6].(func(context.Context) (net.Conn, error))
		outgoingSleep = saved[7].(func(time.Duration))
		outgoingLayoutFor = saved[8].(func(string) (outgoingLayout, bool))
	})
	outgoingFamily = func() string { return "apt" }
	outgoingLookPath = func(name string) (string, error) { return name, nil }
	outgoingRun = host.run
	outgoingWriteFile = func(path string, content []byte) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, content, 0o644)
	}
	outgoingRemoveFile = func(path string) error {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	outgoingPostfixReload = func() error {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.reloads++
		if host.reloadFails {
			return errors.New("postfix check failed")
		}
		return nil
	}
	outgoingDial = host.dial
	outgoingSleep = func(time.Duration) {}
	outgoingLayoutFor = func(string) (outgoingLayout, bool) { return layout, true }
	return host, layout
}

func sampleOutgoingSpec() *transport.OutgoingMailLimitsSpec {
	return &transport.OutgoingMailLimitsSpec{
		Enabled:       true,
		Paused:        []string{"bad@example.com"},
		MailboxLimits: map[string]int{"a@example.com": 500, "b@example.com": 500, "new@example.com": 50},
		DomainLimits:  map[string]int{"example.com": 750},
		Subscriptions: []transport.OutgoingMailSubscription{{Key: "s7", Domains: []string{"example.com", "example.org"}, Limit: 1000}},
	}
}

func TestRenderOutgoingConfigOrderAndTexts(t *testing.T) {
	layout, _ := outgoingLayoutForFamily("apt")
	rendered := renderOutgoingConfig(layout, sampleOutgoingSpec())
	var ids []string
	for _, line := range strings.Split(rendered.config, "\n") {
		if strings.HasPrefix(line, "id=") {
			id, _, _ := strings.Cut(strings.TrimPrefix(line, "id="), " ")
			ids = append(ids, id)
			_, action, _ := strings.Cut(line, "action=")
			// postfwd splits items at a semicolon; a rate action's fields at a slash or comma.
			if strings.Contains(action, ";") {
				t.Fatalf("action holds a semicolon: %s", line)
			}
			if strings.HasPrefix(action, "rcpt(") && strings.Count(action, "/") != 3 {
				t.Fatalf("rate action fields are not four: %s", line)
			}
		}
	}
	want := []string{"CP_SELFTEST", "CP_PAUSED", "CP_MARK_s7", "CP_MAILBOX_50", "CP_MAILBOX_500", "CP_DOMAIN_750", "CP_SUBSCRIPTION_1000", "CP_DEFAULT"}
	if strings.Join(ids, ",") != strings.Join(want, ",") || rendered.rules != len(want) {
		t.Fatalf("rule order = %v (rules %d), want %v", ids, rendered.rules, want)
	}
	if got := rendered.lists["mailbox-500.list"]; !strings.Contains(got, "a@example.com\nb@example.com\n") {
		t.Fatalf("mailbox list = %q", got)
	}
	if !strings.Contains(rendered.lists["subscription-s7.domains"], "example.com\nexample.org\n") || !strings.Contains(rendered.lists["subscription-1000.list"], "s7\n") {
		t.Fatalf("subscription lists = %v", rendered.lists)
	}
	if !strings.Contains(rendered.config, "550 5.7.1 "+transport.OutgoingMailPausedText) ||
		!strings.Contains(rendered.config, "450 4.7.1 This mailbox reached its sending limit of 500 recipients per hour.") {
		t.Fatalf("texts missing:\n%s", rendered.config)
	}
	empty := renderOutgoingConfig(layout, &transport.OutgoingMailLimitsSpec{Enabled: true})
	if empty.rules != 2 || len(empty.lists) != 1 || empty.lists["selftest.domains"] == "" {
		t.Fatalf("an empty state writes rules for empty lists: %d %v", empty.rules, empty.lists)
	}
}

func TestClassifyOutgoingConfig(t *testing.T) {
	layout, _ := outgoingLayoutForFamily("pacman")
	rendered := renderOutgoingConfig(layout, sampleOutgoingSpec())
	archPackage := "#\n# Sample ruleset for postfwd\n#\n\n# default: log the request and finish\nid=DEFAULT; action=DUNNO\n\n"
	cases := []struct {
		name   string
		raw    string
		exists bool
		want   string
	}{
		{"absent", "", false, outgoingConfigAbsent},
		{"arch package file", archPackage, true, outgoingConfigPackage},
		{"CelikPanel's file", rendered.config, true, outgoingConfigPanel},
		{"edited under the marker", strings.Replace(rendered.config, "CP_DEFAULT", "MY_DEFAULT", 1), true, outgoingConfigOwner},
		{"owner's own file", "id=X ; action=DUNNO\n", true, outgoingConfigOwner},
	}
	for _, c := range cases {
		if got := classifyOutgoingConfig([]byte(c.raw), c.exists); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestValidateOutgoingSpecRefusesWhatPostfwdCannotHold(t *testing.T) {
	bad := []*transport.OutgoingMailLimitsSpec{
		{MailboxLimits: map[string]int{"A@example.com": 5}},
		{MailboxLimits: map[string]int{"a@example.com": 0}},
		{DomainLimits: map[string]int{"exa mple.com": 5}},
		{Paused: []string{"x;y@example.com"}},
		{Subscriptions: []transport.OutgoingMailSubscription{{Key: "s1;x", Limit: 5}}},
		{Subscriptions: []transport.OutgoingMailSubscription{{Key: "s1", Limit: 5, Domains: []string{"a/b"}}}},
	}
	for i, spec := range bad {
		if validateOutgoingSpec(spec) == nil {
			t.Errorf("spec %d was accepted", i)
		}
	}
	if err := validateOutgoingSpec(sampleOutgoingSpec()); err != nil {
		t.Fatal(err)
	}
}

func TestApplyOutgoingMailLimitsWritesChecksProvesAndWires(t *testing.T) {
	host, layout := installFakeOutgoingHost(t)
	var resp transport.OutgoingMailLimitsApplyResponse
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Applied || !resp.Verified || !resp.Changed || resp.Code != "" {
		t.Fatalf("apply = %+v", resp)
	}
	raw, err := os.ReadFile(layout.config)
	if err != nil || classifyOutgoingConfig(raw, true) != outgoingConfigPanel {
		t.Fatalf("rule file not CelikPanel's: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.listDir, "paused.list")); err != nil {
		t.Fatal(err)
	}
	dropIn, _ := os.ReadFile(layout.dropIn)
	if !strings.Contains(string(dropIn), "--keep_rates") || !strings.HasPrefix(string(dropIn), outgoingDropInMarker+"\n") {
		t.Fatalf("drop-in = %q", dropIn)
	}
	if host.count("systemctl enable --now postfwd") != 1 {
		t.Fatalf("postfwd not started: %v", host.commands)
	}
	for _, service := range submissionServices {
		if got := host.params[service]["smtpd_end_of_data_restrictions"]; got != "check_policy_service,inet:127.0.0.1:10040" {
			t.Fatalf("%s not wired: %q", service, got)
		}
		if host.params[service]["smtpd_policy_service_default_action"] != "DUNNO" {
			t.Fatalf("%s is not fail-open: %v", service, host.params[service])
		}
	}
	if host.reloads != 1 {
		t.Fatalf("postfix reloads = %d", host.reloads)
	}

	// The same state again writes nothing and reloads nothing.
	before := len(host.commands)
	resp = transport.OutgoingMailLimitsApplyResponse{}
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Applied || resp.Changed || host.reloads != 1 {
		t.Fatalf("second apply changed something: %+v reloads %d", resp, host.reloads)
	}
	for _, c := range host.commands[before:] {
		if strings.HasPrefix(c, "systemctl reload") || strings.HasPrefix(c, "systemctl restart") || strings.Contains(c, "postconf -P submission/inet/") {
			t.Fatalf("second apply ran %q", c)
		}
	}

	// A smaller state removes the lists it no longer needs and reloads postfwd.
	resp = transport.OutgoingMailLimitsApplyResponse{}
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(&transport.OutgoingMailLimitsSpec{Enabled: true, MailboxLimits: map[string]int{"a@example.com": 500}}, &resp); err != nil || !resp.Applied {
		t.Fatalf("smaller apply: %+v %v", resp, err)
	}
	if _, err := os.Stat(filepath.Join(layout.listDir, "paused.list")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the old paused list stayed: %v", err)
	}
	if host.count("systemctl reload postfwd") != 1 {
		t.Fatalf("postfwd not reloaded: %v", host.commands)
	}
}

// alpha.128 lab (Debian 13): the Agent's umask made the list folder 0750
// root:celikpanel, postfwd (user postfw) could not read any list and no limit
// applied. The folder is passable for postfwd after an apply, also when it
// existed 0750 and no list changed.
func TestApplyOutgoingMailLimitsListFolderIsReadableForPostfwd(t *testing.T) {
	_, layout := installFakeOutgoingHost(t)
	if err := os.MkdirAll(layout.listDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(layout.listDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		var resp transport.OutgoingMailLimitsApplyResponse
		if err := (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp); err != nil || !resp.Applied {
			t.Fatalf("round %d: %+v %v", round, resp, err)
		}
		info, err := os.Stat(layout.listDir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("round %d: list folder is %o, want 0755", round, info.Mode().Perm())
		}
		// The second round changes no list: the folder is checked anyway.
		_ = os.Chmod(layout.listDir, 0o750)
	}
}

// alpha.128 lab (Debian 13, postfwd 1.35-10): postfwd2 --showconfig always
// ends with exit status 1, so the printed rules are the check; turning the
// limits on must not fail on a valid file.
func TestApplyOutgoingMailLimitsAcceptsPostfwd1ShowconfigExitStatus(t *testing.T) {
	host, _ := installFakeOutgoingHost(t)
	host.checkExitsOne = true
	var resp transport.OutgoingMailLimitsApplyResponse
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Applied || !resp.Verified || resp.Code != "" {
		t.Fatalf("apply = %+v", resp)
	}
}

func TestApplyOutgoingMailLimitsKeepsTheOwnersRuleFile(t *testing.T) {
	host, layout := installFakeOutgoingHost(t)
	owner := "id=MINE ; action=DUNNO\n"
	if err := outgoingWriteFile(layout.config, []byte(owner)); err != nil {
		t.Fatal(err)
	}
	var resp transport.OutgoingMailLimitsApplyResponse
	_ = (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp)
	if resp.Applied || resp.Code != transport.OutgoingMailOwnerConfig {
		t.Fatalf("apply = %+v", resp)
	}
	raw, _ := os.ReadFile(layout.config)
	if string(raw) != owner || host.count("postconf -P submission/inet/") != 0 || host.reloads != 0 {
		t.Fatalf("the owner's file or Postfix was changed: %q %v", raw, host.commands)
	}
}

func TestApplyOutgoingMailLimitsPutsBackWhenPostfwdDoesNotCount(t *testing.T) {
	host, layout := installFakeOutgoingHost(t)
	host.countWorks = false
	var resp transport.OutgoingMailLimitsApplyResponse
	_ = (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp)
	if resp.Applied || resp.Code != transport.OutgoingMailSelfTestFailed || !strings.Contains(resp.Error, "Mail keeps flowing") {
		t.Fatalf("apply = %+v", resp)
	}
	if _, err := os.Stat(layout.config); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the rule file was not put back (absent before): %v", err)
	}
	if host.params["submission"]["smtpd_end_of_data_restrictions"] != "" || host.reloads != 0 {
		t.Fatalf("Postfix was told to ask a postfwd that does not count: %v", host.params)
	}
}

func TestApplyOutgoingMailLimitsRefusedCheckChangesNothing(t *testing.T) {
	host, layout := installFakeOutgoingHost(t)
	host.checkFails = true
	var resp transport.OutgoingMailLimitsApplyResponse
	_ = (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp)
	if resp.Code != transport.OutgoingMailCheckFailed {
		t.Fatalf("apply = %+v", resp)
	}
	if _, err := os.Stat(layout.config); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused rule file was put in place")
	}
	if entries, _ := os.ReadDir(layout.listDir); len(entries) != 0 {
		t.Fatalf("lists stayed after a refused check: %v", entries)
	}
}

func TestApplyOutgoingMailLimitsKeepsTheOwnersPostfixCheck(t *testing.T) {
	host, _ := installFakeOutgoingHost(t)
	host.params["submission"]["smtpd_end_of_data_restrictions"] = "check_policy_service,unix:private/mine"
	var resp transport.OutgoingMailLimitsApplyResponse
	_ = (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp)
	if resp.Applied || resp.Code != transport.OutgoingMailOwnerWiring {
		t.Fatalf("apply = %+v", resp)
	}
	if host.params["submission"]["smtpd_end_of_data_restrictions"] != "check_policy_service,unix:private/mine" || host.reloads != 0 {
		t.Fatalf("the owner's check was changed: %v", host.params)
	}
}

func TestApplyOutgoingMailLimitsPutsMasterCfBackWhenPostfixRefuses(t *testing.T) {
	host, _ := installFakeOutgoingHost(t)
	host.reloadFails = true
	var resp transport.OutgoingMailLimitsApplyResponse
	_ = (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp)
	if resp.Applied || resp.Code != transport.OutgoingMailPostfixFailed {
		t.Fatalf("apply = %+v", resp)
	}
	for _, service := range submissionServices {
		if _, set := host.params[service]["smtpd_end_of_data_restrictions"]; set {
			t.Fatalf("%s kept the new check after Postfix refused it: %v", service, host.params[service])
		}
	}
}

func TestDisablingOutgoingMailLimitsRemovesOnlyCelikPanelsCheck(t *testing.T) {
	host, _ := installFakeOutgoingHost(t)
	var resp transport.OutgoingMailLimitsApplyResponse
	_ = (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp)
	if !resp.Applied {
		t.Fatalf("apply = %+v", resp)
	}
	host.params["smtps"]["smtpd_policy_service_timeout"] = "30s" // the owner's own value
	resp = transport.OutgoingMailLimitsApplyResponse{}
	_ = (&Agent{}).ApplyOutgoingMailLimitsV1(&transport.OutgoingMailLimitsSpec{Enabled: false}, &resp)
	if !resp.Applied || !resp.Changed {
		t.Fatalf("disable = %+v", resp)
	}
	if _, set := host.params["submission"]["smtpd_end_of_data_restrictions"]; set {
		t.Fatalf("the check stayed: %v", host.params)
	}
	if host.params["smtps"]["smtpd_policy_service_timeout"] != "30s" {
		t.Fatalf("an owner's value was removed: %v", host.params["smtps"])
	}
}

func TestPreparePostfwdWritesADunnoFileOnlyWhereNoneIsTheOwners(t *testing.T) {
	_, layout := installFakeOutgoingHost(t)
	if err := preparePostfwd("apt"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(layout.config)
	if classifyOutgoingConfig(raw, true) != outgoingConfigPanel || !strings.Contains(string(raw), "id=CP_DEFAULT ; action=DUNNO") {
		t.Fatalf("prepared file = %q", raw)
	}
	owner := "id=MINE ; action=DUNNO\n"
	_ = outgoingWriteFile(layout.config, []byte(owner))
	if err := preparePostfwd("apt"); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(layout.config); string(raw) != owner {
		t.Fatal("the owner's file was replaced")
	}
}

func TestArchDropInTurnsOffTheRequestCache(t *testing.T) {
	arch, _ := outgoingLayoutForFamily("pacman")
	debian, _ := outgoingLayoutForFamily("apt")
	if !strings.Contains(arch.dropInContent(), "--cache=0") || !strings.Contains(arch.dropInContent(), "--keep_rates") {
		t.Fatalf("arch drop-in = %q", arch.dropInContent())
	}
	if strings.Contains(debian.dropInContent(), "--cache=0") || !strings.Contains(debian.dropInContent(), "$ARGS --keep_rates") {
		t.Fatalf("debian drop-in = %q", debian.dropInContent())
	}
	for _, layout := range []outgoingLayout{arch, debian} {
		if !strings.HasSuffix(layout.dropInContent(), "\n") || strings.Count(layout.dropInContent(), "ExecStart=") != 2 {
			t.Fatalf("drop-in shape: %q", layout.dropInContent())
		}
	}
}

// alpha.128 lab: after a password change the first real message was still
// refused 550 by a postfwd child holding the old rules. The apply asks the
// resumed mailbox until postfwd answers it as not paused; a mailbox still
// answered as paused when the time is up is named.
func TestApplyOutgoingMailLimitsConfirmsResumedMailboxes(t *testing.T) {
	host, _ := installFakeOutgoingHost(t)
	var resp transport.OutgoingMailLimitsApplyResponse
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp); err != nil || !resp.Applied {
		t.Fatalf("paused apply: %+v %v", resp, err)
	}
	resumedSpec := sampleOutgoingSpec()
	resumedSpec.Paused = nil
	host.mu.Lock()
	host.stalePaused = map[string]int{"bad@example.com": 3}
	host.mu.Unlock()
	resp = transport.OutgoingMailLimitsApplyResponse{}
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(resumedSpec, &resp); err != nil || !resp.Applied || len(resp.ResumeUnconfirmed) != 0 {
		t.Fatalf("resume apply: %+v %v", resp, err)
	}
	host.mu.Lock()
	asked := host.seen["bad@example.com"]
	host.mu.Unlock()
	if asked < 3+outgoingResumeClean {
		t.Fatalf("the resumed mailbox was asked %d times", asked)
	}
	// A child that never takes the rules: named, not claimed open.
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(sampleOutgoingSpec(), &resp); err != nil {
		t.Fatal(err)
	}
	saved := outgoingResumeWait
	outgoingResumeWait = 0
	t.Cleanup(func() { outgoingResumeWait = saved })
	host.mu.Lock()
	host.stalePaused = map[string]int{"bad@example.com": -1}
	host.mu.Unlock()
	resp = transport.OutgoingMailLimitsApplyResponse{}
	if err := (&Agent{}).ApplyOutgoingMailLimitsV1(resumedSpec, &resp); err != nil || !resp.Applied ||
		len(resp.ResumeUnconfirmed) != 1 || resp.ResumeUnconfirmed[0] != "bad@example.com" {
		t.Fatalf("stale resume apply: %+v %v", resp, err)
	}
}
