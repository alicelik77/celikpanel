package main

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A Postfix double: main.cf parameters, the celikpanel-relay master.cf
// service and the files the relay writes, all in memory. Nothing runs on the
// host and no real mail server is contacted.
type relayHost struct {
	main         map[string]string
	service      bool
	command      string
	params       map[string]string
	files        map[string][]byte
	calls        []string
	saslReady    bool
	failCmd      string
	reloadErr    error
	reloadAlways bool
	reloads      int
}

func newRelayHost() *relayHost {
	return &relayHost{main: map[string]string{}, params: map[string]string{}, files: map[string][]byte{}, saslReady: true, command: "smtp"}
}

func (h *relayHost) run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	h.calls = append(h.calls, call)
	if h.failCmd != "" && strings.HasPrefix(call, h.failCmd) {
		return []byte("refused"), errors.New("exit status 1")
	}
	switch {
	case name == "sh":
		target := args[len(args)-1]
		_, path, _ := strings.Cut(target, ":")
		h.files[path+".db"] = []byte("index")
		return nil, nil
	case name != "postconf":
		return nil, errors.New("unexpected command " + name)
	}
	switch args[0] {
	case "-n":
		// As the real postconf: a setting main.cf does not set prints
		// nothing at all; a set one (even empty) prints its line.
		value, set := h.main[args[2]]
		if !set {
			return nil, nil
		}
		return []byte(value + "\n"), nil
	case "-A":
		if h.saslReady {
			return []byte("cyrus\n"), nil
		}
		return []byte("\n"), nil
	case "-F":
		if !h.service {
			return nil, nil
		}
		e := mailRelayServiceEntry + "/"
		return []byte(e + "service = celikpanel-relay\n" + e + "type = unix\n" + e + "chroot = n\n" + e + "command = " + h.commandLine() + "\n"), nil
	case "-P":
		if args[1] == mailRelayServiceEntry {
			var out strings.Builder
			for k, v := range h.params {
				out.WriteString(mailRelayServiceEntry + "/" + k + " = " + v + "\n")
			}
			return []byte(out.String()), nil
		}
		key, value, _ := strings.Cut(strings.TrimPrefix(args[1], mailRelayServiceEntry+"/"), "=")
		h.params[key] = value
	case "-PX":
		delete(h.params, strings.TrimPrefix(args[1], mailRelayServiceEntry+"/"))
	case "-M":
		h.service = true
	case "-MX":
		h.service = false
		h.params = map[string]string{}
	case "-e":
		key, value, _ := strings.Cut(args[1], "=")
		h.main[key] = value
	case "-X":
		delete(h.main, args[1])
	}
	return nil, nil
}

// commandLine is what postconf -F prints: the command with its -o options.
func (h *relayHost) commandLine() string {
	keys := make([]string, 0, len(h.params))
	for k := range h.params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	line := h.command
	for _, k := range keys {
		line += " -o " + k + "=" + h.params[k]
	}
	return line
}

type relayMasterImage struct {
	Service bool
	Params  map[string]string
}

func (h *relayHost) snapshot(path string) (mailFileSnapshot, error) {
	switch path {
	case "/etc/postfix/main.cf":
		raw, _ := json.Marshal(h.main)
		return mailFileSnapshot{path: path, exists: true, content: raw}, nil
	case "/etc/postfix/master.cf":
		raw, _ := json.Marshal(relayMasterImage{h.service, h.params})
		return mailFileSnapshot{path: path, exists: true, content: raw}, nil
	}
	content, ok := h.files[path]
	return mailFileSnapshot{path: path, exists: ok, content: append([]byte(nil), content...)}, nil
}

func (h *relayHost) restore(s mailFileSnapshot) error {
	switch s.path {
	case "/etc/postfix/main.cf":
		h.main = map[string]string{}
		return json.Unmarshal(s.content, &h.main)
	case "/etc/postfix/master.cf":
		var image relayMasterImage
		if err := json.Unmarshal(s.content, &image); err != nil {
			return err
		}
		h.service, h.params = image.Service, image.Params
		if h.params == nil {
			h.params = map[string]string{}
		}
		return nil
	}
	if s.exists {
		h.files[s.path] = s.content
	} else {
		delete(h.files, s.path)
	}
	return nil
}

func useRelayHost(t *testing.T, h *relayHost) {
	t.Helper()
	saved := []any{mailRelayRun, mailRelayLookPath, mailRelayRead, mailRelayWriteSecret, mailRelayChmodSecret, mailRelayRemove, mailRelaySnapshot, mailRelayRestore, mailRelayMapType, mailRelayReload, mailRelayGlob}
	t.Cleanup(func() {
		mailRelayRun = saved[0].(func(context.Context, string, ...string) ([]byte, error))
		mailRelayLookPath = saved[1].(func(string) (string, error))
		mailRelayRead = saved[2].(func(string) ([]byte, bool, error))
		mailRelayWriteSecret = saved[3].(func(string, []byte) error)
		mailRelayChmodSecret = saved[4].(func(string) error)
		mailRelayRemove = saved[5].(func(string) error)
		mailRelaySnapshot = saved[6].(func(string) (mailFileSnapshot, error))
		mailRelayRestore = saved[7].(func(mailFileSnapshot) error)
		mailRelayMapType = saved[8].(func(context.Context) string)
		mailRelayReload = saved[9].(func() error)
		mailRelayGlob = saved[10].(func(string) ([]string, error))
	})
	mailRelayRun = h.run
	mailRelayLookPath = func(string) (string, error) { return "/usr/sbin/postconf", nil }
	mailRelayRead = func(path string) ([]byte, bool, error) {
		raw, ok := h.files[path]
		return raw, ok, nil
	}
	mailRelayWriteSecret = func(path string, content []byte) error {
		h.files[path] = append([]byte(nil), content...)
		return nil
	}
	mailRelayChmodSecret = func(string) error { return nil }
	mailRelayRemove = func(path string) error { delete(h.files, path); return nil }
	mailRelaySnapshot = h.snapshot
	mailRelayRestore = h.restore
	mailRelayMapType = func(context.Context) string { return "hash" }
	mailRelayReload = func() error {
		h.reloads++
		if h.reloadErr != nil && (h.reloads == 1 || h.reloadAlways) {
			return h.reloadErr
		}
		return nil
	}
	mailRelayGlob = func(string) ([]string, error) {
		if h.saslReady {
			return []string{"/usr/lib/x86_64-linux-gnu/sasl2/libplain.so.2"}, nil
		}
		return nil, nil
	}
}

const relayTestPassword = "s3cret pass:word"

func relayApply(req transport.MailRelayApplyRequest) transport.MailRelayApplyResponse {
	var resp transport.MailRelayApplyResponse
	applyMailRelay(context.Background(), &req, &resp)
	return resp
}

func relayOn(port int) transport.MailRelayApplyRequest {
	return transport.MailRelayApplyRequest{Enabled: true, Host: "Smtp.Relay.Example.", Port: port, Username: "user@example.test", Password: relayTestPassword}
}

func assertNoSecret(t *testing.T, resp transport.MailRelayApplyResponse, h *relayHost) {
	t.Helper()
	raw, _ := json.Marshal(resp)
	if strings.Contains(string(raw), "s3cret") {
		t.Fatalf("the password reached the answer: %s", raw)
	}
	for _, call := range h.calls {
		if strings.Contains(call, "s3cret") {
			t.Fatalf("the password reached a command line: %s", call)
		}
	}
}

func TestMailRelayTurnsOnInTheOrderThatNeverLooksLikeTheOwners(t *testing.T) {
	h := newRelayHost()
	useRelayHost(t, h)
	resp := relayApply(relayOn(587))
	if !resp.Applied || resp.State != transport.MailRelayStateOn || resp.Error != "" {
		t.Fatalf("apply = %+v", resp)
	}
	assertNoSecret(t, resp, h)
	content := string(h.files[mailRelayMapPath])
	if content != mailRelayMapHeader+"\n[smtp.relay.example]:587 user@example.test:"+relayTestPassword+"\n" {
		t.Fatalf("map = %q", content)
	}
	if _, ok := h.files[mailRelayMapPath+".db"]; !ok {
		t.Fatal("postmap did not build the index")
	}
	if h.main["relayhost"] != "[smtp.relay.example]:587" || h.main["default_transport"] != mailRelayService {
		t.Fatalf("main.cf = %v", h.main)
	}
	if h.params["smtp_tls_security_level"] != "encrypt" || h.params["smtp_sasl_password_maps"] != "hash:"+mailRelayMapPath || h.params["smtp_tls_wrappermode"] != "" {
		t.Fatalf("service options = %v", h.params)
	}
	order := strings.Join(h.calls, "\n")
	if strings.Index(order, "default_transport=") > strings.Index(order, "relayhost=") {
		t.Fatal("relayhost was written before default_transport; an interrupted run would look like the owner's relay")
	}
	if !strings.Contains(order, `sh -c umask 077 && exec postmap "$1" sh hash:`+mailRelayMapPath) {
		t.Fatalf("the index was not built under umask 077:\n%s", order)
	}
	if h.reloads != 1 {
		t.Fatalf("reloads = %d", h.reloads)
	}
	status := readMailRelayStatus(context.Background())
	if status.State != transport.MailRelayStateOn || !status.SASLReady || status.Relayhost != "[smtp.relay.example]:587" {
		t.Fatalf("status = %+v", status)
	}
}

func TestMailRelaySwitchingTo587RemovesTheWrapperMode(t *testing.T) {
	h := newRelayHost()
	useRelayHost(t, h)
	if resp := relayApply(relayOn(465)); !resp.Applied || h.params["smtp_tls_wrappermode"] != "yes" {
		t.Fatalf("465: %+v %v", resp, h.params)
	}
	if resp := relayApply(relayOn(587)); !resp.Applied {
		t.Fatalf("587: %+v", resp)
	}
	if _, set := h.params["smtp_tls_wrappermode"]; set {
		t.Fatalf("wrapper mode stayed: %v", h.params)
	}
}

func TestMailRelayNeverOverwritesTheOwnersRelay(t *testing.T) {
	for name, main := range map[string]map[string]string{
		"relayhost":         {"relayhost": "[mx.isp.example]:25"},
		"default_transport": {"default_transport": "smtp:[other.example]"},
		"sender dependent":  {"sender_dependent_relayhost_maps": "hash:/etc/postfix/sdr"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newRelayHost()
			h.main = main
			useRelayHost(t, h)
			resp := relayApply(relayOn(587))
			if resp.Applied || resp.Code != transport.MailRelayOwnerConfig || !strings.Contains(resp.Error, "keeps it") {
				t.Fatalf("apply = %+v", resp)
			}
			if len(h.files) != 0 || h.service || h.reloads != 0 {
				t.Fatalf("something was written: files=%v service=%v reloads=%d", h.files, h.service, h.reloads)
			}
			// Turning off leaves the owner's line alone.
			off := relayApply(transport.MailRelayApplyRequest{})
			if off.Error != "" || !off.Applied || off.Changed || h.reloads != 0 {
				t.Fatalf("off = %+v main=%v reloads=%d", off, h.main, h.reloads)
			}
			for key, value := range main {
				if h.main[key] != value {
					t.Fatalf("the owner's %s changed: %v", key, h.main)
				}
			}
		})
	}
}

func TestMailRelayTurnsOffWithoutLeftovers(t *testing.T) {
	h := newRelayHost()
	h.main["smtp_tls_security_level"] = "may"
	// Debian's and Ubuntu's main.cf carry an empty `relayhost =`: not the
	// owner's relay.
	h.main["relayhost"] = ""
	useRelayHost(t, h)
	if resp := relayApply(relayOn(587)); !resp.Applied {
		t.Fatal(resp)
	}
	resp := relayApply(transport.MailRelayApplyRequest{})
	if !resp.Applied || resp.State != transport.MailRelayStateOff {
		t.Fatalf("off = %+v", resp)
	}
	if len(h.files) != 0 || h.service || len(h.params) != 0 {
		t.Fatalf("leftovers: files=%v service=%v params=%v", h.files, h.service, h.params)
	}
	keys := make([]string, 0, len(h.main))
	for key := range h.main {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "smtp_tls_security_level" || h.main["smtp_tls_security_level"] != "may" {
		t.Fatalf("main.cf after off = %v (the distribution's own line must stay)", h.main)
	}
	order := strings.Join(h.calls, "\n")
	if strings.Index(order, "-X relayhost") > strings.Index(order, "-X default_transport") {
		t.Fatal("default_transport was removed before relayhost")
	}
	// A second off changes nothing and reloads nothing.
	reloads := h.reloads
	if again := relayApply(transport.MailRelayApplyRequest{}); !again.Applied || again.Changed || h.reloads != reloads {
		t.Fatalf("second off = %+v", again)
	}
}

func TestMailRelayPutsTheFilesBackWhenPostfixRefuses(t *testing.T) {
	h := newRelayHost()
	h.main["smtp_tls_security_level"] = "may"
	useRelayHost(t, h)
	h.reloadErr = errors.New("postfix check failed")
	resp := relayApply(relayOn(587))
	if resp.Applied || resp.Code != transport.MailRelayPostfixFailed || !strings.Contains(resp.Error, "put back") {
		t.Fatalf("apply = %+v", resp)
	}
	assertNoSecret(t, resp, h)
	if len(h.files) != 0 || h.service || h.main["relayhost"] != "" || h.main["default_transport"] != "" {
		t.Fatalf("not put back: files=%v service=%v main=%v", h.files, h.service, h.main)
	}
	// Postfix refusing the earlier files too is said, with what to run.
	h.reloads, h.reloadAlways = 0, true
	resp = relayApply(relayOn(587))
	if resp.Code != transport.MailRelayRollbackFailed || !strings.Contains(resp.Error, "postfix check") {
		t.Fatalf("rollback failure = %+v", resp)
	}
}

func TestMailRelayKeepsAnEditedServiceOrMap(t *testing.T) {
	h := newRelayHost()
	useRelayHost(t, h)
	if resp := relayApply(relayOn(587)); !resp.Applied {
		t.Fatal(resp)
	}
	h.params["smtp_tls_security_level"] = "may"
	for _, req := range []transport.MailRelayApplyRequest{relayOn(587), {}} {
		resp := relayApply(req)
		if resp.Applied || resp.Code != transport.MailRelayEditedConfig || !strings.Contains(resp.Error, "postconf -MX") {
			t.Fatalf("apply = %+v", resp)
		}
	}
	if h.params["smtp_tls_security_level"] != "may" {
		t.Fatal("the edited option was overwritten")
	}
	h.params["smtp_tls_security_level"] = "encrypt"
	h.files[mailRelayMapPath] = []byte("[x]:587 a:b\n")
	if resp := relayApply(relayOn(587)); resp.Code != transport.MailRelayEditedConfig {
		t.Fatalf("edited map: %+v", resp)
	}
}

func TestMailRelayRefusesBadInputAndMissingSASL(t *testing.T) {
	h := newRelayHost()
	useRelayHost(t, h)
	for name, req := range map[string]transport.MailRelayApplyRequest{
		"loopback":  {Enabled: true, Host: "127.0.0.1", Port: 587, Username: "u", Password: "p"},
		"localhost": {Enabled: true, Host: "localhost", Port: 587, Username: "u", Password: "p"},
		"port":      {Enabled: true, Host: "relay.example", Port: 26, Username: "u", Password: "p"},
		"newline":   {Enabled: true, Host: "relay.example", Port: 587, Username: "u", Password: "p\nrelayhost=x"},
		"user":      {Enabled: true, Host: "relay.example", Port: 587, Username: "u ser", Password: "p"},
		"no pass":   {Enabled: true, Host: "relay.example", Port: 587, Username: "u"},
		"injection": {Enabled: true, Host: "relay.example]:25 x", Port: 587, Username: "u", Password: "p"},
	} {
		if resp := relayApply(req); resp.Code != transport.MailRelayInvalid {
			t.Fatalf("%s: %+v", name, resp)
		}
	}
	h.saslReady = false
	if resp := relayApply(relayOn(587)); resp.Code != transport.MailRelaySASLMissing || !strings.Contains(resp.Error, "libsasl2-modules") {
		t.Fatalf("sasl: %+v", resp)
	}
	if len(h.files) != 0 || h.service {
		t.Fatal("something was written")
	}
}
