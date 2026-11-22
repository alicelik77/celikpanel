package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// signingPostfix is main.cf and master.cf as postconf shows them. main holds
// what main.cf sets; defaults what Postfix uses for a setting main.cf does not
// name.
type signingPostfix struct {
	main     map[string]string
	defaults map[string]string
	master   map[string]map[string]string // "smtp/inet" -> fields
	params   map[string]string            // "smtp/inet/smtpd_milters" -> value
	calls    []string
	failOn   string
}

func newSigningPostfix() *signingPostfix {
	return &signingPostfix{
		main: map[string]string{"smtpd_milters": "", "non_smtpd_milters": "", "milter_protocol": "6"},
		defaults: map[string]string{"milter_default_action": "tempfail", "authorized_submit_users": "static:anyone",
			"smtpd_milters": "", "non_smtpd_milters": "", "milter_protocol": "6"},
		master: map[string]map[string]string{
			"smtp/inet":       {"private": "n", "unprivileged": "-", "chroot": "y", "wakeup": "-", "process_limit": "-", "command": "smtpd"},
			"submission/inet": {"private": "n", "unprivileged": "-", "chroot": "y", "wakeup": "-", "process_limit": "-", "command": "smtpd"},
		},
		params: map[string]string{},
	}
}

func (f *signingPostfix) value(key string) string {
	if value, ok := f.main[key]; ok {
		return value
	}
	return f.defaults[key]
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
	switch {
	case len(args) == 3 && args[0] == "-n" && args[1] == "-h":
		if value, ok := f.main[args[2]]; ok {
			return []byte(value + "\n"), nil
		}
		return nil, nil
	case len(args) == 3 && args[0] == "-x" && args[1] == "-h":
		return []byte(f.value(args[2]) + "\n"), nil
	}
	switch args[0] {
	case "-h":
		return []byte(f.value(args[1]) + "\n"), nil
	case "-e":
		key, value, _ := strings.Cut(args[1], "=")
		f.main[key] = value
	case "-X":
		for _, key := range args[1:] {
			delete(f.main, key)
		}
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

// memoryPort25Record replaces the port-25 record file for one test.
func memoryPort25Record(t *testing.T) *map[string]string {
	t.Helper()
	record := map[string]string{}
	savedRead, savedWrite := readPort25Record, writePort25Record
	t.Cleanup(func() { readPort25Record, writePort25Record = savedRead, savedWrite })
	readPort25Record = func() (map[string]string, error) {
		out := map[string]string{}
		for k, v := range record {
			out[k] = v
		}
		return out, nil
	}
	writePort25Record = func(next map[string]string) error {
		record = map[string]string{}
		for k, v := range next {
			record[k] = v
		}
		return nil
	}
	return &record
}

// Path b: the smtpd that receives mail on port 25 reads a list without
// OpenDKIM (the spam filter stays); main.cf keeps OpenDKIM for submission,
// and the port-25 entry is written after main.cf.
func TestMailSigningPort25SmtpdDoesNotSign(t *testing.T) {
	memoryPort25Record(t)
	f := newSigningPostfix()
	note, undo, plan, err := composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil || len(plan.owner) > 0 || note != "" {
		t.Fatalf("note %q plan %+v err %v", note, plan, err)
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
	if len(undo.port25) != 1 || undo.port25[0].beforeSet {
		t.Fatalf("undo of the first run: %+v", undo)
	}
	// Without a spam filter port 25 has no milter at all.
	memoryPort25Record(t)
	f = newSigningPostfix()
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, ""); err != nil {
		t.Fatal(err)
	}
	if got, set := f.params["smtp/inet/smtpd_milters"]; !set || got != "" {
		t.Fatalf("port 25 without a filter: %q %v", got, set)
	}
}

// Behind postscreen the smtpd is smtpd/pass.
func TestMailSigningPort25BehindPostscreen(t *testing.T) {
	memoryPort25Record(t)
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

// The owner's own port-25 setting is kept; it is reported when it names
// OpenDKIM, and kept silently when it does not.
func TestMailSigningKeepsTheOwnersPort25Setting(t *testing.T) {
	memoryPort25Record(t)
	f := newSigningPostfix()
	f.params["smtp/inet/smtpd_milters"] = opendkimMilter + ", inet:127.0.0.1:9999"
	note, _, plan, err := composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil || len(plan.owner) != 1 || !strings.Contains(note, "inet:127.0.0.1:9999") {
		t.Fatalf("note %q plan %+v err %v", note, plan, err)
	}
	if f.params["smtp/inet/smtpd_milters"] != opendkimMilter+", inet:127.0.0.1:9999" || f.index("postconf -P smtp/inet/smtpd_milters=") >= 0 {
		t.Fatal("the owner's port-25 setting was changed")
	}
	if state := decideMailSigning(mailSigningInputs{confState: mailSigningBound, port25: plan}); state.State != mailSigningPort25Kept || state.Subject != "smtp/inet" {
		t.Fatalf("state %+v", state)
	}

	memoryPort25Record(t)
	f = newSigningPostfix()
	f.params["smtp/inet/smtpd_milters"] = "inet:127.0.0.1:9999"
	_, _, plan, err = composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil || len(plan.owner) != 0 || f.params["smtp/inet/smtpd_milters"] != "inet:127.0.0.1:9999" {
		t.Fatalf("an owner's port-25 list without OpenDKIM: plan %+v params %+v err %v", plan, f.params, err)
	}
}

// Review finding 3, scenario 1: the owner adds a filter to main.cf after
// CelikPanel wrote port 25's own list. The next start gives port 25 the
// owner's list without OpenDKIM, so the owner's filter sees mail arriving on
// port 25 too; nothing is reported as the owner's port-25 setting.
func TestMailSigningPort25FollowsTheOwnersLaterMainList(t *testing.T) {
	record := memoryPort25Record(t)
	f := newSigningPostfix()
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, rspamdMilter); err != nil {
		t.Fatal(err)
	}
	if (*record)["smtp/inet"] != rspamdMilter {
		t.Fatalf("record %+v", *record)
	}
	f.main["smtpd_milters"] = opendkimMilter + ", " + rspamdMilter + ", inet:127.0.0.1:7777"
	note, _, plan, err := composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.params["smtp/inet/smtpd_milters"]; got != rspamdMilter+",inet:127.0.0.1:7777" {
		t.Fatalf("port 25 reads %q", got)
	}
	if f.main["smtpd_milters"] != opendkimMilter+", "+rspamdMilter+", inet:127.0.0.1:7777" {
		t.Fatal("the owner's main.cf list was rewritten")
	}
	if state := decideMailSigning(mailSigningInputs{confState: mailSigningBound, port25: plan}); state.State != mailSigningBound {
		t.Fatalf("state %+v (note %q)", state, note)
	}
	// The owner takes his filter out again: port 25 follows.
	f.main["smtpd_milters"] = opendkimMilter + ", " + rspamdMilter
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, rspamdMilter); err != nil {
		t.Fatal(err)
	}
	if got := f.params["smtp/inet/smtpd_milters"]; got != rspamdMilter {
		t.Fatalf("port 25 after the owner's filter left: %q", got)
	}
}

// Review finding 3, scenario 2: OpenDKIM removed, CelikPanel's port-25 list
// goes (no leftover pointing at a removed filter); a main.cf list that names
// its filters through a variable is not split and is reported.
func TestMailSigningPort25SettingGoesWithOpenDKIM(t *testing.T) {
	record := memoryPort25Record(t)
	f := newSigningPostfix()
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, rspamdMilter); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := composeAndWriteMilterChain(f.run, false, rspamdMilter); err != nil {
		t.Fatal(err)
	}
	if _, set := f.params["smtp/inet/smtpd_milters"]; set || len(*record) != 0 {
		t.Fatalf("port 25 after OpenDKIM was removed: %+v record %+v", f.params, *record)
	}
	// An alpha.143 candidate's value without a record is still recognised.
	f.params["smtp/inet/smtpd_milters"] = rspamdMilter
	if _, _, _, err := composeAndWriteMilterChain(f.run, false, rspamdMilter); err != nil {
		t.Fatal(err)
	}
	if _, set := f.params["smtp/inet/smtpd_milters"]; set {
		t.Fatal("an earlier CelikPanel port-25 value stayed")
	}

	memoryPort25Record(t)
	f = newSigningPostfix()
	f.main["smtpd_milters"] = "$owner_milters"
	_, _, plan, err := composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil || !plan.variable || f.index("postconf -P smtp/inet/smtpd_milters=") >= 0 {
		t.Fatalf("variable main list: plan %+v err %v", plan, err)
	}
	// 22 Nov 2026 (B4): its own state, never the hand-changed entry's.
	if state := decideMailSigning(mailSigningInputs{confState: mailSigningBound, port25: plan}); state.State != mailSigningPort25Variable {
		t.Fatalf("state %+v", state)
	}
}

// Second review of alpha.143, B4: main.cf's list turns into a variable after
// CelikPanel wrote port 25's own value. That value stays in force (removing
// it could put OpenDKIM back on port 25), the state says exactly that, and
// never that the owner changed the port-25 entry by hand.
func TestMailSigningVariableListKeepsCelikPanelsValueAndSaysSo(t *testing.T) {
	record := memoryPort25Record(t)
	f := newSigningPostfix()
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, rspamdMilter); err != nil {
		t.Fatal(err)
	}
	f.main["smtpd_milters"] = "$owner_milters"
	note, _, plan, err := composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.params["smtp/inet/smtpd_milters"]; got != rspamdMilter || (*record)["smtp/inet"] != rspamdMilter {
		t.Fatalf("CelikPanel's value must stay in force and recorded: %q record %+v", got, *record)
	}
	if len(plan.owner) != 0 || plan.variable || len(plan.variableOwn) != 1 {
		t.Fatalf("plan %+v", plan)
	}
	state := decideMailSigning(mailSigningInputs{confState: mailSigningBound, port25: plan})
	if state.State != mailSigningPort25VariableOwn || state.Subject != "smtp/inet" || !strings.Contains(state.Detail, "still in force") ||
		strings.Contains(state.Detail, "changed by the server owner") || !strings.Contains(note, "still in force") {
		t.Fatalf("state %+v note %q", state, note)
	}
	// A risk that signs for another domain is said before it.
	if got := decideMailSigning(mailSigningInputs{confState: mailSigningBound, port25: plan, submitOpen: true}); got.State != mailSigningSubmitOpen {
		t.Fatalf("submit_open hidden by %s", got.State)
	}
	// The owner names his filters again: port 25 follows main.cf again.
	f.main["smtpd_milters"] = opendkimMilter + ", " + rspamdMilter + ", inet:127.0.0.1:7777"
	_, _, plan, err = composeAndWriteMilterChain(f.run, true, rspamdMilter)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.params["smtp/inet/smtpd_milters"]; got != rspamdMilter+",inet:127.0.0.1:7777" {
		t.Fatalf("port 25 after the owner named his filters: %q", got)
	}
	if state := decideMailSigning(mailSigningInputs{confState: mailSigningBound, port25: plan}); state.State != mailSigningBound {
		t.Fatalf("state %+v", state)
	}
}

// Second review of alpha.143, B4: the record file is lost. CelikPanel still
// recognises its own value by its content (main.cf's list without OpenDKIM,
// joined by commas), so it is followed and removed with OpenDKIM instead of
// being kept as the owner's.
func TestMailSigningPort25ValueIsRecognisedWithoutTheRecord(t *testing.T) {
	record := memoryPort25Record(t)
	f := newSigningPostfix()
	f.main["smtpd_milters"] = opendkimMilter + ", " + rspamdMilter + ", inet:127.0.0.1:7777"
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, rspamdMilter); err != nil {
		t.Fatal(err)
	}
	if got := f.params["smtp/inet/smtpd_milters"]; got != rspamdMilter+",inet:127.0.0.1:7777" {
		t.Fatalf("port 25 reads %q", got)
	}
	for k := range *record {
		delete(*record, k)
	}
	// OpenDKIM is removed: the value goes, record or not.
	if _, _, _, err := composeAndWriteMilterChain(f.run, false, rspamdMilter); err != nil {
		t.Fatal(err)
	}
	if got, set := f.params["smtp/inet/smtpd_milters"]; set {
		t.Fatalf("CelikPanel's own value was taken for the owner's without its record: %q", got)
	}
}

// Second review of alpha.143, B9: OpenDKIM's endpoint is recognised in every
// spelling Postfix accepts for it (milter8.c, inet_connect.c, host_port.c).
func TestMailSigningOpenDKIMEndpointSpellings(t *testing.T) {
	for entry, want := range map[string]bool{
		"inet:127.0.0.1:8891":               true,
		"inet:localhost:8891":               true,
		"inet:[127.0.0.1]:8891":             true,
		"inet:[localhost]:8891":             true,
		"inet:LocalHost:8891":               true,
		"inet::8891":                        true,
		"inet:8891":                         true,
		"inet:127.0.0.1:08891":              true,
		"inet:[IPv6:::ffff:127.0.0.1]:8891": true,
		"inet:[ipv6:::ffff:127.0.0.1]:8891": true,
		"inet:::ffff:127.0.0.1:8891":        true,
		"inet:127.0.0.1:8892":               false,
		"inet:127.0.0.2:8891":               false,
		"inet:[::1]:8891":                   false,
		"inet:[127.0.0.1]":                  false,
		"inet:[127.0.0.1]8891":              false,
		"INET:127.0.0.1:8891":               false,
		"unix:/run/opendkim/opendkim.sock":  false,
		"inet:8891@127.0.0.1":               false,
		"inet:127.0.0.1:opendkim":           false,
		"inet:":                             false,
	} {
		if got := isOpenDKIMMilter(entry); got != want {
			t.Errorf("%s: %v, want %v", entry, got, want)
		}
	}
	// In a list: port 25 leaves the other spelling out, keeps the rest.
	rest, named, ok := milterListWithoutOpenDKIM("inet:[127.0.0.1]:8891, inet:127.0.0.1:7777")
	if !ok || !named || rest != "inet:127.0.0.1:7777" {
		t.Fatalf("rest %q named %v ok %v", rest, named, ok)
	}
}

// Second review of alpha.143, B9: ConfigureDKIMSigning puts a new
// opendkim.conf in place the way the start does: checked with `opendkim -n`
// before it is used (a refusal changes nothing), and put back when OpenDKIM
// does not run with it.
func TestMailSigningConfigureDKIMSigningChecksAndPutsBack(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	earlier := renderOpenDKIMConfWithSocket(layout, opendkimSocket)
	next := []byte(renderOpenDKIMConf(layout))
	if err := os.WriteFile(layout.confPath, []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	// Refused by the check: nothing changes, no restart is owed.
	h.refuseNew = true
	if err := installCheckedOpenDKIMConf(h.run, layout.confPath, next); err == nil || !strings.Contains(err.Error(), "was not changed") {
		t.Fatalf("refusal: %v", err)
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != earlier || openDKIMRestartIsOwed() {
		t.Fatalf("a refused text changed something (owed %v)", openDKIMRestartIsOwed())
	}
	if _, err := os.Stat(layout.confPath + opendkimCandidateSuffix); !os.IsNotExist(err) {
		t.Fatal("the refused candidate was left behind")
	}
	// Accepted, but OpenDKIM starts only with the earlier text: put back.
	h.refuseNew = false
	h.startsWith = func(string) bool {
		content, _ := os.ReadFile(layout.confPath)
		return string(content) == earlier
	}
	if err := installCheckedOpenDKIMConf(h.run, layout.confPath, next); err != nil {
		t.Fatal(err)
	}
	if !openDKIMRestartIsOwed() {
		t.Fatal("the owed restart was not recorded before the file was put in place")
	}
	err := restartOpenDKIMOrPutBack(h.run, layout.confPath, []byte(earlier), true, true)
	if err == nil || !strings.Contains(err.Error(), "put back") {
		t.Fatalf("put back: %v", err)
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != earlier || !h.active || openDKIMRestartIsOwed() {
		t.Fatalf("not put back: active %v owed %v", h.active, openDKIMRestartIsOwed())
	}
	// Accepted and running: the restart owed is cleared.
	h.startsWith = func(string) bool { return true }
	if err := installCheckedOpenDKIMConf(h.run, layout.confPath, next); err != nil {
		t.Fatal(err)
	}
	if err := restartOpenDKIMOrPutBack(h.run, layout.confPath, []byte(earlier), true, true); err != nil || openDKIMRestartIsOwed() {
		t.Fatalf("restart: %v owed %v", err, openDKIMRestartIsOwed())
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != string(next) {
		t.Fatal("the new text is not in place")
	}
	// ConfigureDKIMSigning takes this path: tables first (without the
	// configuration), then the checked text, then the restart that puts back.
	source, err := os.ReadFile("dkim_signing_rpc.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func (a *Agent) ConfigureDKIMSigning(")
	body := text[start:]
	body = body[:strings.Index(body, "\n}\n")]
	tables := strings.Index(body, "writeDKIMTables(ctx, domains, layout.confPath, nil)")
	check := strings.Index(body, "installCheckedOpenDKIMConf(run, layout.confPath, conf)")
	restart := strings.Index(body, "restartOpenDKIMOrPutBack(run, layout.confPath, current, exists, changed)")
	if tables < 0 || check < tables || restart < check || strings.Contains(body, `"systemctl", "restart", "opendkim"`) {
		t.Fatalf("ConfigureDKIMSigning order: tables %d check %d restart %d", tables, check, restart)
	}
}

// Review finding 8: when Postfix does not take the change everything is put
// back exactly: a setting that was at its default is removed again, not
// written out.
func TestMailSigningMilterUndoPutsEverythingBack(t *testing.T) {
	record := memoryPort25Record(t)
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
	if f.main["milter_default_action"] != "accept" {
		t.Fatalf("accept was not written: %+v", f.main)
	}
	if err := undo.undo(f.run); err != nil {
		t.Fatal(err)
	}
	if len(f.main) != len(beforeMain) {
		t.Fatalf("main.cf after undo %+v, before %+v", f.main, beforeMain)
	}
	for k, v := range beforeMain {
		if got, ok := f.main[k]; !ok || got != v {
			t.Errorf("main.cf %s = %q (set %v), want %q", k, got, ok, v)
		}
	}
	if _, set := f.main["milter_default_action"]; set || f.index("postconf -X milter_default_action") < 0 {
		t.Fatal("a setting that was at its default was written out instead of removed")
	}
	if f.params["smtp/inet/smtpd_milters"] != opendkimMilter+", "+rspamdMilter || len(*record) != 0 {
		t.Fatalf("port 25 not put back: %+v record %+v", f.params, *record)
	}
	// A write that fails mid-way leaves nothing changed.
	memoryPort25Record(t)
	f = newSigningPostfix()
	f.failOn = "postconf -P smtp/inet/smtpd_milters="
	if _, _, _, err := composeAndWriteMilterChain(f.run, true, rspamdMilter); err == nil {
		t.Fatal("a failed write was not reported")
	}
	if f.main["smtpd_milters"] != "" {
		t.Fatalf("main.cf not put back after a failed write: %+v", f.main)
	}
	if _, set := f.main["milter_default_action"]; set {
		t.Fatalf("milter_default_action left written after a failed write: %+v", f.main)
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
	// status overrides the answer of `systemctl is-active` while active.
	status        string
	defaultAction string
}

func installOpenDKIMHost(t *testing.T, layout opendkimLayout) (*opendkimHost, opendkimLayout) {
	t.Helper()
	dir := t.TempDir()
	layout.confPath = filepath.Join(dir, "opendkim.conf")
	saved := []any{opendkimConfRead, opendkimConfWrite, opendkimProbeDir, opendkimRestartOwedPath}
	t.Cleanup(func() {
		opendkimConfRead = saved[0].(func(string) ([]byte, error))
		opendkimConfWrite = saved[1].(func(string, []byte, os.FileMode) error)
		opendkimProbeDir = saved[2].(func() string)
		opendkimRestartOwedPath = saved[3].(string)
	})
	opendkimConfRead = os.ReadFile
	opendkimConfWrite = os.WriteFile
	opendkimProbeDir = func() string { return dir }
	opendkimRestartOwedPath = filepath.Join(dir, "private", "opendkim-restart-owed")
	h := &opendkimHost{supported: true, active: true, startsWith: func(string) bool { return true }, defaultAction: "accept"}
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
			if h.status != "" {
				return []byte(h.status + "\n"), errors.New("exit status 3")
			}
			return []byte("active\n"), nil
		}
		return []byte("failed\n"), errors.New("exit status 3")
	case call == "systemctl restart opendkim":
		h.status = ""
		h.active = h.startsWith(h.conf)
		if !h.active {
			return nil, errors.New("exit status 1")
		}
	case call == "postconf -h milter_default_action":
		return []byte(h.defaultAction + "\n"), nil
	}
	return nil, nil
}

func (h *opendkimHost) restarts() int {
	n := 0
	for _, call := range h.calls {
		if call == "systemctl restart opendkim" {
			n++
		}
	}
	return n
}

func aptLayout(t *testing.T) opendkimLayout {
	layout, ok := opendkimLayoutFor("apt")
	if !ok {
		t.Fatal("no apt layout")
	}
	return layout
}

// An earlier release's file becomes this release's text, checked before use,
// and OpenDKIM is restarted with it; the owed restart is recorded before the
// file is put in place and cleared after the verified restart.
func TestMailSigningOpenDKIMConfIsCheckedThenReplaced(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	earlier := renderOpenDKIMConfWithSocket(layout, opendkimSocket)
	if err := os.WriteFile(layout.confPath, []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	markedBeforeRename := false
	savedRename := opendkimConfRename
	t.Cleanup(func() { opendkimConfRename = savedRename })
	opendkimConfRename = func(from, to string) error {
		markedBeforeRename = openDKIMRestartIsOwed()
		return os.Rename(from, to)
	}
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningBound || note != "" {
		t.Fatalf("state %s note %q", state, note)
	}
	if !markedBeforeRename {
		t.Fatal("the owed restart was not recorded before the file was put in place")
	}
	if openDKIMRestartIsOwed() {
		t.Fatal("the owed restart stayed recorded after a verified restart")
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
	if state, _ := reconcileOpenDKIMConf(h.run, layout); state != mailSigningBound || h.restarts() != 0 {
		t.Fatalf("second run %s, restarts %d", state, h.restarts())
	}
}

// Review finding 4: the Agent stopped between the rename and the restart (or
// systemd answered "reloading"): the next start finds the file current and
// restarts OpenDKIM anyway, because the owed restart was recorded; until a
// verified restart the state is not "in place".
func TestMailSigningOwedRestartIsMadeAtTheNextStart(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	if err := os.WriteFile(layout.confPath, []byte(renderOpenDKIMConf(layout)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := markOpenDKIMRestartOwed(layout.confPath); err != nil {
		t.Fatal(err)
	}
	h.status = "reloading"
	// The restart fails: not in place, and the record stays for the next start.
	h.startsWith = func(string) bool { return false }
	if state, _ := reconcileOpenDKIMConf(h.run, layout); state != mailSigningFailed || !openDKIMRestartIsOwed() {
		t.Fatalf("state %s owed %v", state, openDKIMRestartIsOwed())
	}
	if h.restarts() != 1 {
		t.Fatalf("restarts %d", h.restarts())
	}
	// The next start: it runs now.
	h.calls, h.active = nil, true
	h.startsWith = func(string) bool { return true }
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningBound || h.restarts() != 1 || openDKIMRestartIsOwed() || !strings.Contains(note, "restarted") {
		t.Fatalf("state %s restarts %d owed %v note %q", state, h.restarts(), openDKIMRestartIsOwed(), note)
	}
	// A stopped OpenDKIM owes nothing: it reads the file when it starts.
	if err := markOpenDKIMRestartOwed(layout.confPath); err != nil {
		t.Fatal(err)
	}
	h.calls, h.active = nil, false
	if state, _ := reconcileOpenDKIMConf(h.run, layout); state != mailSigningBound || h.restarts() != 0 || openDKIMRestartIsOwed() {
		t.Fatalf("stopped: state %s restarts %d", state, h.restarts())
	}
	// observeOpenDKIMConf (ConfigureMailStack, ConfigureDKIMSigning) never
	// says "in place" while the restart is owed.
	if err := markOpenDKIMRestartOwed(layout.confPath); err != nil {
		t.Fatal(err)
	}
	if state, _ := observeOpenDKIMConf(h.run, layout); state != mailSigningFailed {
		t.Fatalf("observed %s with a restart owed", state)
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
	if openDKIMRestartIsOwed() {
		t.Fatal("a restart was recorded for a refused text")
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

// The owner's opendkim.conf is kept, with CelikPanel's text beside it, and the
// step names the exact line.
func TestMailSigningKeepsTheOwnersOpenDKIMConf(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	owner := renderOpenDKIMConfWithSocket(layout, opendkimSocket) + "SignatureAlgorithm\trsa-sha256\n"
	if err := os.WriteFile(layout.confPath, []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningConfKept || !strings.Contains(note, opendkimPendingSuffix) || !strings.Contains(note, "SenderMacro {auth_authen}") {
		t.Fatalf("state %s note %q", state, note)
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != owner {
		t.Fatal("the owner's file was changed")
	}
	if pending, _ := os.ReadFile(layout.confPath + opendkimPendingSuffix); string(pending) != renderOpenDKIMConf(layout) {
		t.Fatalf("pending:\n%s", pending)
	}
	if h.restarts() != 0 {
		t.Fatal("OpenDKIM was restarted for an owner's file")
	}
}

// Review finding 6: on an OpenDKIM without SenderMacro an owner's file is
// kept, nothing the owner's OpenDKIM would refuse is put beside it (a pending
// text an earlier candidate left is removed), the owner is never told to add
// the line, and the state is unsupported.
func TestMailSigningUnsupportedOwnerIsNeverAdvisedTheLine(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	owner := renderOpenDKIMConfWithSocket(layout, opendkimSocket) + "SignatureAlgorithm\trsa-sha256\n"
	if err := os.WriteFile(layout.confPath, []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.confPath+opendkimPendingSuffix, []byte(renderOpenDKIMConf(layout)), 0o644); err != nil {
		t.Fatal(err)
	}
	h.supported = false
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningUnsupported || strings.Contains(note, "adds the line") || !strings.Contains(note, "From: header") {
		t.Fatalf("state %s note %q", state, note)
	}
	if _, err := os.Stat(layout.confPath + opendkimPendingSuffix); !os.IsNotExist(err) {
		t.Fatal("a text with SenderMacro was left beside the owner's file on an OpenDKIM that refuses it")
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != owner {
		t.Fatal("the owner's file was changed")
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

// Review finding 7: with the server owner's milter list and his own
// milter_default_action, what happens to mail during the restart is said;
// with accept nothing needs saying.
func TestMailSigningRestartWindowIsSaid(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	if err := os.WriteFile(layout.confPath, []byte(renderOpenDKIMConfWithSocket(layout, opendkimSocket)), 0o644); err != nil {
		t.Fatal(err)
	}
	h.defaultAction = "tempfail"
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningBound || !strings.Contains(note, "temporary error") || !strings.Contains(note, "tempfail") {
		t.Fatalf("state %s note %q", state, note)
	}
	h.defaultAction = "accept"
	if note := milterDefaultActionNote(h.run); note != "" {
		t.Fatalf("accept: %q", note)
	}
	h.defaultAction = "reject"
	if note := milterDefaultActionNote(h.run); !strings.Contains(note, "undeliverable") {
		t.Fatalf("reject: %q", note)
	}
}

// Review finding 7: the milter chain (which ensures accept where the lists
// are CelikPanel's) is written before OpenDKIM is restarted at a start.
func TestMailSigningChainIsWrittenBeforeOpenDKIMRestarts(t *testing.T) {
	source, err := os.ReadFile("mail_stack_rpc.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func wireMailFilters(")
	body := text[start:]
	body = body[:strings.Index(body, "\n}\n")]
	chain, conf := strings.Index(body, "applyMilterChainDetailed(ctx)"), strings.Index(body, "reconcileOpenDKIMConf(run, layout)")
	if chain < 0 || conf < 0 || chain > conf {
		t.Fatalf("milter chain at %d, OpenDKIM's configuration at %d", chain, conf)
	}
}

// Review finding 2: who may hand mail over without a login decides whether
// local mail can be signed for any hosted domain.
func TestMailSigningLocalSubmitDecidesTheState(t *testing.T) {
	f := newSigningPostfix()
	f.main["non_smtpd_milters"] = opendkimMilter + ", " + rspamdMilter
	// Postfix's default (static:anyone): sites can submit.
	open, note, err := localSubmitOpen(f.run)
	if err != nil || !open || !strings.Contains(note, "authorized_submit_users = root, vmail") {
		t.Fatalf("default: open %v note %q err %v", open, note, err)
	}
	if state := decideMailSigning(mailSigningInputs{confState: mailSigningBound, submitOpen: open, submitNote: note}); state.State != mailSigningSubmitOpen {
		t.Fatalf("state %+v", state)
	}
	for value, want := range map[string]bool{
		"root, vmail":           false,
		"root":                  false,
		"vmail root":            false,
		"root, vmail, www-data": true,
		"static:anyone":         true,
		"hash:/etc/postfix/who": true,
	} {
		f.main["authorized_submit_users"] = value
		if open, _, err := localSubmitOpen(f.run); err != nil || open != want {
			t.Errorf("%q: open %v err %v, want %v", value, open, err, want)
		}
	}
	// No OpenDKIM on local mail: nothing is signed by its From:.
	f.main["authorized_submit_users"] = "static:anyone"
	f.main["non_smtpd_milters"] = rspamdMilter
	if open, _, err := localSubmitOpen(f.run); err != nil || open {
		t.Fatalf("without OpenDKIM on local mail: open %v err %v", open, err)
	}
}

// Review finding 5: a step that did not reach signing is unknown, never an
// earlier "in place".
func TestMailSigningStateNamesEveryOutcome(t *testing.T) {
	cases := []struct {
		in   mailSigningInputs
		want string
	}{
		{mailSigningInputs{confState: mailSigningBound}, mailSigningBound},
		{mailSigningInputs{confState: mailSigningBound, port25: port25Plan{owner: []string{"smtp/inet smtpd_milters = x"}}}, mailSigningPort25Kept},
		{mailSigningInputs{confState: mailSigningBound, milterErr: errors.New("reload")}, mailSigningFailed},
		{mailSigningInputs{confState: mailSigningConfKept}, mailSigningConfKept},
		{mailSigningInputs{confState: mailSigningUnsupported}, mailSigningUnsupported},
		{mailSigningInputs{confState: mailSigningUnsupported, submitOpen: true}, mailSigningSubmitOpen},
		{mailSigningInputs{confState: mailSigningUnknown}, mailSigningUnknown},
		{mailSigningInputs{confState: mailSigningBound, submitErr: errors.New("unreadable")}, mailSigningUnknown},
		{mailSigningInputs{confState: mailSigningNoDKIM, submitOpen: true}, mailSigningNoDKIM},
	}
	for _, c := range cases {
		if got := decideMailSigning(c.in); got.State != c.want {
			t.Errorf("%+v: %s, want %s", c.in, got.State, c.want)
		}
	}
	if got := mailSigningNotRun(errors.New("postmap failed")); got.State != mailSigningUnknown || !strings.Contains(got.Detail, "postmap failed") {
		t.Fatalf("not run: %+v", got)
	}
}

// alpha.143 lab (22 Nov 2026): the owner did what the kept-file line says
// (added "SenderMacro {auth_authen}" to his own opendkim.conf). His file is
// then in place: bound, so the other states (authorized_submit_users) are
// decided normally; CelikPanel's text beside it goes; his file is not
// written. A file whose SenderMacro is another macro, or one that pulls in
// another file, stays kept.
func TestMailSigningOwnersFileWithTheLoginSettingIsInPlace(t *testing.T) {
	h, layout := installOpenDKIMHost(t, aptLayout(t))
	owner := renderOpenDKIMConfWithSocket(layout, opendkimSocket) + "SignatureAlgorithm\trsa-sha256\n"
	if err := os.WriteFile(layout.confPath, []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	if state, _ := reconcileOpenDKIMConf(h.run, layout); state != mailSigningConfKept {
		t.Fatalf("an owner's file without the line: %s", state)
	}
	if _, err := os.Stat(layout.confPath + opendkimPendingSuffix); err != nil {
		t.Fatal("no text beside the owner's file")
	}
	owner += "# signing by login, as the panel said\nsendermacro   {auth_authen}   # the login\n"
	if err := os.WriteFile(layout.confPath, []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	state, note := reconcileOpenDKIMConf(h.run, layout)
	if state != mailSigningBound || !strings.Contains(note, "kept as it is") {
		t.Fatalf("an owner's file with the line: %s %q", state, note)
	}
	if _, err := os.Stat(layout.confPath + opendkimPendingSuffix); !os.IsNotExist(err) {
		t.Fatal("the text beside the owner's file stayed although it is no longer needed")
	}
	if content, _ := os.ReadFile(layout.confPath); string(content) != owner {
		t.Fatal("the owner's file was changed")
	}
	if observed, _ := observeOpenDKIMConf(h.run, layout); observed != mailSigningBound {
		t.Fatalf("observed: %s", observed)
	}
	if got := decideMailSigning(mailSigningInputs{confState: state, submitOpen: true}); got.State != mailSigningSubmitOpen {
		t.Fatalf("the submit state is hidden: %s", got.State)
	}
	for _, other := range []string{
		"SenderMacro {auth_authen}\nSenderMacro {mail_addr}\n",
		"SenderMacro {auth_authen}\nInclude /etc/opendkim.d/more.conf\n",
		"SenderMacro\n",
	} {
		if ownerOpenDKIMConfBindsLogin([]byte(other)) {
			t.Fatalf("judged in place: %q", other)
		}
	}
}
