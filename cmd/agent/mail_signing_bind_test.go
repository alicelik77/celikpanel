package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// signingPostfix is main.cf and master.cf as postconf shows them.
type signingPostfix struct {
	main   map[string]string
	master map[string]map[string]string // "smtp/inet" -> fields
	params map[string]string            // "smtp/inet/smtpd_milters" -> value
	calls  []string
	failOn string
}

func newSigningPostfix() *signingPostfix {
	return &signingPostfix{
		main: map[string]string{"smtpd_milters": "", "non_smtpd_milters": "", "milter_default_action": "tempfail", "milter_protocol": "6"},
		master: map[string]map[string]string{
			"smtp/inet":       {"private": "n", "unprivileged": "-", "chroot": "y", "wakeup": "-", "process_limit": "-", "command": "smtpd"},
			"submission/inet": {"private": "n", "unprivileged": "-", "chroot": "y", "wakeup": "-", "process_limit": "-", "command": "smtpd"},
		},
		params: map[string]string{},
	}
}

func (f *signingPostfix) run(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if f.failOn != "" && strings.HasPrefix(call, f.failOn) {
		return []byte("postconf: fatal: refused"), errors.New("exit status 1")
	}
	if name != "postconf" || len(args) == 0 {
		return nil, nil
	}
	switch args[0] {
	case "-h":
		return []byte(f.main[args[1]] + "\n"), nil
	case "-e":
		key, value, _ := strings.Cut(args[1], "=")
		f.main[key] = value
	case "-F":
		fields, ok := f.master[args[1]]
		if !ok {
			return nil, nil
		}
		var lines []string
		for field, value := range fields {
			lines = append(lines, args[1]+"/"+field+" = "+value)
		}
		sort.Strings(lines)
		return []byte(strings.Join(lines, "\n") + "\n"), nil
	case "-P":
		if key, value, ok := strings.Cut(args[1], "="); ok {
			f.params[key] = value
			return nil, nil
		}
		if value, ok := f.params[args[1]]; ok {
			return []byte(args[1] + " = " + value + "\n"), nil
		}
	case "-PX":
		delete(f.params, args[1])
	}
	return nil, nil
}

func (f *signingPostfix) index(prefix string) int {
	for i, call := range f.calls {
		if strings.HasPrefix(call, prefix) {
			return i
		}
	}
	return -1
}

// Path b: the smtpd that receives mail on port 25 reads a list without
// OpenDKIM (the spam filter stays); main.cf keeps OpenDKIM for submission,
// and the port-25 entry is written after main.cf.
func TestMailSigningPort25SmtpdDoesNotSign(t *testing.T) {
	f := newSigningPostfix()
	note, undo, kept, err := composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil || kept || note != "" {
		t.Fatalf("note %q kept %v err %v", note, kept, err)
	}
	if got := f.params["smtp/inet/smtpd_milters"]; got != rspamdMilter {
		t.Fatalf("port 25 reads %q", got)
	}
	if !strings.Contains(f.main["smtpd_milters"], opendkimMilter) || !strings.Contains(f.main["non_smtpd_milters"], opendkimMilter) {
		t.Fatalf("main.cf lost OpenDKIM: %+v", f.main)
	}
	if _, set := f.params["submission/inet/smtpd_milters"]; set {
		t.Fatal("submission must keep following main.cf")
	}
	if main, port := f.index("postconf -e smtpd_milters="), f.index("postconf -P smtp/inet/smtpd_milters="); main < 0 || port < main {
		t.Fatalf("order: main.cf %d, port 25 %d\n%s", main, port, strings.Join(f.calls, "\n"))
	}
	// Without a spam filter port 25 has no milter at all.
	f = newSigningPostfix()
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, ""); err != nil {
		t.Fatal(err)
	}
	if got, set := f.params["smtp/inet/smtpd_milters"]; !set || got != "" {
		t.Fatalf("port 25 without a filter: %q %v", got, set)
	}
	if len(undo.port25) != 1 || undo.port25[0].beforeSet {
		t.Fatalf("undo of the first run: %+v", undo)
	}
}

// Behind postscreen the smtpd is smtpd/pass.
func TestMailSigningPort25BehindPostscreen(t *testing.T) {
	f := newSigningPostfix()
	f.master["smtp/inet"]["command"] = "postscreen"
	f.master["smtpd/pass"] = map[string]string{"private": "-", "unprivileged": "-", "chroot": "y", "wakeup": "-", "process_limit": "-", "command": "smtpd"}
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, rspamdMilter); err != nil {
		t.Fatal(err)
	}
	if _, set := f.params["smtp/inet/smtpd_milters"]; set {
		t.Fatal("postscreen's entry got a milter list")
	}
	if f.params["smtpd/pass/smtpd_milters"] != rspamdMilter {
		t.Fatalf("smtpd/pass: %+v", f.params)
	}
}

// The owner's own port-25 setting, or his main.cf list, is kept and said.
func TestMailSigningKeepsTheOwnersMilterSettings(t *testing.T) {
	f := newSigningPostfix()
	f.params["smtp/inet/smtpd_milters"] = "inet:127.0.0.1:9999"
	note, _, kept, err := composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil || !kept || !strings.Contains(note, "inet:127.0.0.1:9999") {
		t.Fatalf("note %q kept %v err %v", note, kept, err)
	}
	if f.params["smtp/inet/smtpd_milters"] != "inet:127.0.0.1:9999" || f.index("postconf -P smtp/inet/smtpd_milters=") >= 0 {
		t.Fatal("the owner's port-25 setting was changed")
	}

	f = newSigningPostfix()
	f.main["smtpd_milters"] = opendkimMilter + ", inet:127.0.0.1:7777"
	note, _, kept, err = composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil || !kept || note == "" {
		t.Fatalf("note %q kept %v err %v", note, kept, err)
	}
	if f.index("postconf -P") >= 0 {
		t.Fatal("port 25 was changed under the owner's main.cf list")
	}
}

// When Postfix does not take the change everything is put back.
func TestMailSigningMilterUndoPutsEverythingBack(t *testing.T) {
	f := newSigningPostfix()
	f.params["smtp/inet/smtpd_milters"] = opendkimMilter + ", " + rspamdMilter
	beforeMain := map[string]string{}
	for k, v := range f.main {
		beforeMain[k] = v
	}
	_, undo, _, err := composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil {
		t.Fatal(err)
	}
	if err := undo.undo(f.run); err != nil {
		t.Fatal(err)
	}
	for k, v := range beforeMain {
		if f.main[k] != v {
			t.Errorf("main.cf %s = %q, want %q", k, f.main[k], v)
		}
	}
	if f.params["smtp/inet/smtpd_milters"] != opendkimMilter+", "+rspamdMilter {
		t.Fatalf("port 25 not put back: %+v", f.params)
	}
	// A write that fails mid-way leaves nothing changed.
	f = newSigningPostfix()
	f.failOn = "postconf -P smtp/inet/smtpd_milters="
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, rspamdMilter); err == nil {
		t.Fatal("a failed write was not reported")
	}
	if f.main["smtpd_milters"] != "" || f.main["milter_default_action"] != "tempfail" {
		t.Fatalf("main.cf not put back after a failed write: %+v", f.main)
	}
}

// opendkimHost fakes the OpenDKIM side: the files are real temporary files,
// the commands are recorded.
type opendkimHost struct {
	calls      []string
	supported  bool
	refuseNew  bool
	startsWith func(conf string) bool
	conf       string
	active     bool
}

func installOpenDKIMHost(t *testing.T, layout opendkimLayout) (*opendkimHost, opendkimLayout) {
	t.Helper()
	dir := t.TempDir()
	layout.confPath = filepath.Join(dir, "opendkim.conf")
	saved := []any{opendkimConfRead, opendkimConfWrite, opendkimProbeDir}
	t.Cleanup(func() {
		opendkimConfRead = saved[0].(func(string) ([]byte, error))
		opendkimConfWrite = saved[1].(func(string, []byte, os.FileMode) error)
		opendkimProbeDir = saved[2].(func() string)
	})
	opendkimConfRead = os.ReadFile
	opendkimConfWrite = os.WriteFile
	opendkimProbeDir = func() string { return dir }
	h := &opendkimHost{supported: true, active: true, startsWith: func(string) bool { return true }}
	return h, layout
}

func (h *opendkimHost) run(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	h.calls = append(h.calls, call)
	switch {
	case name == "opendkim" && len(args) == 3 && args[0] == "-n":
		if strings.Contains(args[2], "celikpanel-opendkim-probe-") {
			if !h.supported {
				return []byte("opendkim: " + args[2] + ": configuration error at line 4: unrecognized parameter"), errors.New("exit status 78")
			}
			return nil, nil
		}
		if h.refuseNew {
			return []byte("opendkim: configuration error at line 9: illegal value"), errors.New("exit status 78")
		}
	case call == "systemctl is-active opendkim":
		if h.active {
			return []byte("active\n"), nil
		}
		return []byte("failed\n"), errors.New("exit status 3")
	case call == "systemctl restart opendkim":
		h.active = h.startsWith(h.conf)
		if !h.active {
			return nil, errors.New("exit status 1")
		}
	}
	return nil, nil
}

func aptLayout(t *testing.T) opendkimLayout {
	layout, ok := opendkimLayoutFor("apt")
	if !ok {
		t.Fatal("no apt layout")
	}
	return layout
}

// An earlier release's file becomes this release's text, checked before use,
// and OpenDKIM is restarted with it.
func TestMailSigningOpenDKIMConfIsCheckedThenReplaced(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	earlier := renderOpenDKIMConfWithSocket(layout, opendkimSocket)
	if err := os.WriteFile(layout.confPath, []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	h.startsWith = func(string) bool { return true }
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningBound || note != "" {
		t.Fatalf("state %s note %q", state, note)
	}
	content, _ := os.ReadFile(layout.confPath)
	if string(content) != renderOpenDKIMConf(layout) {
		t.Fatalf("file:\n%s", content)
	}
	check, restart := -1, -1
	for i, call := range h.calls {
		if strings.HasSuffix(call, opendkimCandidateSuffix) && strings.HasPrefix(call, "opendkim -n -x ") {
			check = i
		}
		if call == "systemctl restart opendkim" {
			restart = i
		}
	}
	if check < 0 || restart < check {
		t.Fatalf("checked %d restarted %d:\n%s", check, restart, strings.Join(h.calls, "\n"))
	}
	if _, err := os.Stat(layout.confPath + opendkimCandidateSuffix); !os.IsNotExist(err) {
		t.Fatal("the candidate was left behind")
	}
	// Run again: nothing to do.
	h.calls = nil
	if state, _ := reconcileOpenDKIMConf(h.run, layout); state != mailSigningBound {
		t.Fatalf("second run %s", state)
	}
	for _, call := range h.calls {
		if call == "systemctl restart opendkim" {
			t.Fatal("restarted although nothing changed")
		}
	}
}

// A refused new text changes nothing; one OpenDKIM does not start with is
// undone, and OpenDKIM runs with the earlier file again.
func TestMailSigningOpenDKIMRefusalAndUndoKeepMailFlowing(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	earlier := renderOpenDKIMConfWithSocket(layout, opendkimSocket)
	if err := os.WriteFile(layout.confPath, []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	h.refuseNew = true
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningFailed || !strings.Contains(note, "was not changed") {
		t.Fatalf("state %s note %q", state, note)
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != earlier {
		t.Fatal("a refused text was put in place")
	}
	if _, err := os.Stat(layout.confPath + opendkimCandidateSuffix); !os.IsNotExist(err) {
		t.Fatal("the refused candidate was left behind")
	}

	h.refuseNew = false
	// OpenDKIM starts only with the earlier text.
	h.startsWith = func(string) bool {
		content, _ := os.ReadFile(layout.confPath)
		return string(content) == earlier
	}
	state, note = reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningFailed || !strings.Contains(note, "put back") {
		t.Fatalf("state %s note %q", state, note)
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != earlier || !h.active {
		t.Fatalf("not undone: active %v\n%s", h.active, content)
	}
}

// The owner's opendkim.conf is kept, with CelikPanel's text beside it.
func TestMailSigningKeepsTheOwnersOpenDKIMConf(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	owner := renderOpenDKIMConfWithSocket(layout, opendkimSocket) + "SignatureAlgorithm\trsa-sha256\n"
	if err := os.WriteFile(layout.confPath, []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningConfKept || !strings.Contains(note, opendkimPendingSuffix) {
		t.Fatalf("state %s note %q", state, note)
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != owner {
		t.Fatal("the owner's file was changed")
	}
	if pending, _ := os.ReadFile(layout.confPath + opendkimPendingSuffix); string(pending) != renderOpenDKIMConf(layout) {
		t.Fatalf("pending:\n%s", pending)
	}
	for _, call := range h.calls {
		if strings.HasPrefix(call, "systemctl restart") {
			t.Fatal("OpenDKIM was restarted for an owner's file")
		}
	}
}

// An OpenDKIM built without SenderMacro keeps the earlier text and says so.
func TestMailSigningWithoutSenderMacroKeepsTheEarlierText(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	earlier := renderOpenDKIMConfWithSocket(layout, opendkimSocket)
	if err := os.WriteFile(layout.confPath, []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	h.supported = false
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningUnsupported || !strings.Contains(note, "SenderMacro") {
		t.Fatalf("state %s note %q", state, note)
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != earlier {
		t.Fatal("the file was changed for an OpenDKIM without SenderMacro")
	}
	// A file this release wrote with the line on such a host goes back.
	if err := os.WriteFile(layout.confPath, []byte(renderOpenDKIMConf(layout)), 0o644); err != nil {
		t.Fatal(err)
	}
	if state, _ := reconcileOpenDKIMConf(h.run, layout); state != mailSigningUnsupported {
		t.Fatalf("state %s", state)
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != earlier {
		t.Fatalf("file:\n%s", content)
	}
}

func TestMailSigningOutcomeNamesTheState(t *testing.T) {
	cases := []struct {
		conf   string
		port25 bool
		err    error
		want   string
	}{
		{mailSigningBound, false, nil, mailSigningBound},
		{mailSigningBound, true, nil, mailSigningPort25Kept},
		{mailSigningBound, false, errors.New("reload"), mailSigningFailed},
		{mailSigningConfKept, false, nil, mailSigningConfKept},
		{mailSigningUnsupported, false, nil, mailSigningUnsupported},
		{mailSigningNoDKIM, true, nil, mailSigningNoDKIM},
	}
	for _, c := range cases {
		if got := mailSigningOutcome(c.conf, "", c.port25, c.err); got.State != c.want {
			t.Errorf("%+v: %s", c, got.State)
		}
	}
}
