package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
	gossh "golang.org/x/crypto/ssh"
)

// A fake host: /etc/ssh in a temporary directory, an sshd whose -T answers by
// the same first-match reading of those files, and a systemctl that records.

var testRSAKey = func() string {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	key, err := gossh.NewPublicKey(public)
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(gossh.MarshalAuthorizedKey(key))) + " owner@laptop"
}()

type fakeSSHHost struct {
	t          *testing.T
	root       string
	home       string
	reloads    int
	failReload bool
	failTest   bool
	unitActive bool
	keys       map[string]string // home -> authorized_keys
	extraT     string
}

func newFakeSSHHost(t *testing.T, mainConfig string, dropIns map[string]string) *fakeSSHHost {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sshd_config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, text string) {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "sshd_config"), mainConfig)
	for name, text := range dropIns {
		write(filepath.Join(root, "sshd_config.d", name), text)
	}
	return &fakeSSHHost{t: t, root: root, unitActive: true, keys: map[string]string{"/home/ubuntu": testRSAKey + "\n"}}
}

const debianMain = "Include /etc/ssh/sshd_config.d/*.conf\nKbdInteractiveAuthentication no\nUsePAM yes\n"

func (h *fakeSSHHost) env() sshPasswordLoginEnv {
	// The files say /etc/ssh; the fake root stands in for it on disk.
	m := func(path string) string { return strings.Replace(path, "/etc/ssh", h.root, 1) }
	back := func(path string) string { return strings.Replace(path, h.root, "/etc/ssh", 1) }
	env := sshPasswordLoginEnv{
		root:       "/etc/ssh",
		readConfig: func(path string) ([]byte, error) { return os.ReadFile(m(path)) },
		glob: func(pattern string) ([]string, error) {
			matches, err := filepath.Glob(m(pattern))
			for i := range matches {
				matches[i] = back(matches[i])
			}
			return matches, err
		},
		dirExists: func(path string) bool {
			info, err := os.Stat(m(path))
			return err == nil && info.IsDir()
		},
		unit: "ssh",
		passwd: func() ([]byte, error) {
			return []byte("root:x:0:0:root:/root:/bin/bash\nubuntu:x:1000:1000::/home/ubuntu:/bin/bash\nwww-data:x:33:33::/var/www:/usr/sbin/nologin\n"), nil
		},
		group: func() ([]byte, error) { return []byte("root:x:0:\nsudo:x:27:ubuntu\nubuntu:x:1000:\n"), nil },
		keyFile: func(file, home string, uid int, strict bool) ([]byte, bool) {
			if file != filepath.Join(home, ".ssh/authorized_keys") {
				return nil, false
			}
			text, ok := h.keys[home]
			return []byte(text), ok
		},
		read: func(path string) ([]byte, bool, error) {
			raw, err := os.ReadFile(m(path))
			if errors.Is(err, os.ErrNotExist) {
				return nil, false, nil
			}
			return raw, true, err
		},
		write: func(path string, content []byte) error {
			file, err := os.OpenFile(m(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = file.Write(content)
			return err
		},
		remove: func(path string) (func() error, func(), error) {
			aside := m(path) + ".aside"
			if err := os.Rename(m(path), aside); err != nil {
				return nil, nil, err
			}
			return func() error { return os.Rename(aside, m(path)) }, func() { _ = os.Remove(aside) }, nil
		},
	}
	env.sshd = func(ctx context.Context, args ...string) ([]byte, error) {
		probe := env
		directives, err := walkSSHDConfig(probe, nil)
		if err != nil {
			return []byte(err.Error()), err
		}
		if args[0] == "-t" {
			for _, d := range directives {
				if d.key == "bogus" || h.failTest {
					return []byte("/etc/ssh/sshd_config line 3: Bad configuration option: bogus"), errors.New("exit status 255")
				}
			}
			return nil, nil
		}
		first, _ := sshdDeciders(directives, []string{"passwordauthentication", "kbdinteractiveauthentication", "permitrootlogin", "pubkeyauthentication"})
		value := func(key, fallback string) string {
			if d := first[key]; d != nil {
				return d.value
			}
			return fallback
		}
		out := fmt.Sprintf("passwordauthentication %s\nkbdinteractiveauthentication %s\npermitrootlogin %s\npubkeyauthentication %s\nauthorizedkeysfile .ssh/authorized_keys .ssh/authorized_keys2\nstrictmodes yes\n%s",
			value("passwordauthentication", "yes"), value("kbdinteractiveauthentication", "no"),
			value("permitrootlogin", "without-password"), value("pubkeyauthentication", "yes"), h.extraT)
		return []byte(out), nil
	}
	env.systemctl = func(ctx context.Context, args ...string) ([]byte, error) {
		switch args[0] {
		case "show":
			if h.unitActive {
				return []byte("active\n"), nil
			}
			return []byte("inactive\n"), nil
		case "reload":
			h.reloads++
			if h.failReload {
				return []byte("Job for ssh.service failed"), errors.New("exit status 1")
			}
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected systemctl %v", args)
	}
	return env
}

func (h *fakeSSHHost) dropIn() (string, bool) {
	raw, err := os.ReadFile(filepath.Join(h.root, "sshd_config.d", sshPasswordLoginDropInName))
	return string(raw), err == nil
}

func (h *fakeSSHHost) leftovers() []string {
	entries, _ := os.ReadDir(filepath.Join(h.root, "sshd_config.d"))
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), ".aside") {
			names = append(names, entry.Name())
		}
	}
	return names
}

func TestCloudInitPasswordYesIsShownWithTheFileThatDecides(t *testing.T) {
	// Boston: 50-cloud-init.conf says yes and is read before 60's no.
	h := newFakeSSHHost(t, debianMain, map[string]string{
		"50-cloud-init.conf":        "PasswordAuthentication yes\n",
		"60-cloudimg-settings.conf": "PasswordAuthentication no\n",
	})
	status := sshPasswordLoginStatus(context.Background(), h.env())
	if status.State != transport.SSHPasswordLoginStateRead || status.PasswordAuthentication != "yes" {
		t.Fatalf("status %+v", status)
	}
	var password transport.SSHSettingDecider
	for _, d := range status.Deciders {
		if d.Setting == "passwordauthentication" {
			password = d
		}
	}
	if !strings.HasSuffix(password.File, "/sshd_config.d/50-cloud-init.conf") || password.Line != 1 || password.Value != "yes" {
		t.Fatalf("decider %+v", password)
	}
	if !status.CanTurnOff || status.Reason != "" {
		t.Fatalf("turning off must be offered: %+v", status)
	}
	if len(status.KeyAccounts) != 1 || status.KeyAccounts[0].Name != "ubuntu" || status.KeyAccounts[0].Keys != 1 {
		t.Fatalf("key accounts %+v", status.KeyAccounts)
	}
}

func TestTurnOffWritesTheDropInValidatesConfirmsAndReloads(t *testing.T) {
	h := newFakeSSHHost(t, debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\n"})
	env := h.env()
	resp := setSSHPasswordLogin(context.Background(), env, &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOff})
	if resp.Outcome != transport.SSHPasswordLoginTurnedOff || !resp.Reloaded || resp.PasswordAuthentication != "no" || resp.KbdInteractiveAuthentication != "no" {
		t.Fatalf("resp %+v", resp)
	}
	if text, ok := h.dropIn(); !ok || text != sshPasswordLoginDropInText || h.reloads != 1 {
		t.Fatalf("drop-in %q %v reloads %d", text, ok, h.reloads)
	}
	// The owner's file is untouched.
	if raw, _ := os.ReadFile(filepath.Join(h.root, "sshd_config.d", "50-cloud-init.conf")); string(raw) != "PasswordAuthentication yes\n" {
		t.Fatalf("owner file changed: %q", raw)
	}
	// Turning back on removes only CelikPanel's drop-in.
	status := sshPasswordLoginStatus(context.Background(), env)
	if !status.CanTurnOn || status.DropIn.State != transport.SSHDropInPanel {
		t.Fatalf("status after %+v", status)
	}
	on := setSSHPasswordLogin(context.Background(), env, &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOn, ExpectedDropInSHA256: status.DropIn.SHA256})
	if on.Outcome != transport.SSHPasswordLoginTurnedOn || on.PasswordAuthentication != "yes" || h.reloads != 2 {
		t.Fatalf("on %+v reloads %d", on, h.reloads)
	}
	if _, ok := h.dropIn(); ok || len(h.leftovers()) != 0 {
		t.Fatalf("leftovers: drop-in %v, %v", ok, h.leftovers())
	}
}

func TestTurnOffRefusesWhenTheDropInWouldNotDecide(t *testing.T) {
	cases := []struct {
		name, main string
		dropIns    map[string]string
		reason     string
	}{
		{"no include", "PasswordAuthentication yes\n", nil, "include_order"},
		{"setting before the include", "PasswordAuthentication yes\nInclude /etc/ssh/sshd_config.d/*.conf\n", nil, "include_order"},
		{"an owner's drop-in sorts first", debianMain, map[string]string{"00-aaa.conf": "PasswordAuthentication yes\n"}, "include_order"},
		{"a match block", debianMain + "Match User deploy\n  PasswordAuthentication yes\n", map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\n"}, "match_block"},
		{"pubkey off", debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\nPubkeyAuthentication no\n"}, "pubkey_off"},
		{"owner edited the drop-in", debianMain, map[string]string{sshPasswordLoginDropInName: "PasswordAuthentication yes\n"}, "drop_in_edited"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newFakeSSHHost(t, c.main, c.dropIns)
			env := h.env()
			status := sshPasswordLoginStatus(context.Background(), env)
			if status.CanTurnOff || status.Reason != c.reason {
				t.Fatalf("status %+v", status)
			}
			resp := setSSHPasswordLogin(context.Background(), env, &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOff, ExpectedDropInSHA256: status.DropIn.SHA256})
			if resp.Outcome != transport.SSHPasswordLoginRefused || resp.Code != c.reason || h.reloads != 0 {
				t.Fatalf("resp %+v reloads %d", resp, h.reloads)
			}
			if c.reason != "drop_in_edited" {
				if _, ok := h.dropIn(); ok {
					t.Fatal("a refused change wrote the drop-in")
				}
			} else if text, _ := h.dropIn(); text != "PasswordAuthentication yes\n" {
				t.Fatal("the owner's drop-in was changed")
			}
		})
	}
}

func TestLockOutGuardNeedsAnAdministratorWithAKey(t *testing.T) {
	h := newFakeSSHHost(t, debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\n"})
	h.keys = map[string]string{}
	status := sshPasswordLoginStatus(context.Background(), h.env())
	if status.CanTurnOff || status.Reason != "no_key_account" {
		t.Fatalf("no keys: %+v", status)
	}
	// A key restricted to one command is not a way back in.
	h.keys["/home/ubuntu"] = `command="/usr/bin/backup" ` + testRSAKey + "\n"
	if status = sshPasswordLoginStatus(context.Background(), h.env()); status.Reason != "no_key_account" {
		t.Fatalf("forced command counted: %+v", status)
	}
	// AllowUsers that leaves the administrator out.
	h.keys["/home/ubuntu"] = testRSAKey + "\n"
	h.extraT = "allowusers deploy\n"
	if status = sshPasswordLoginStatus(context.Background(), h.env()); status.Reason != "no_key_account" {
		t.Fatalf("AllowUsers ignored: %+v", status)
	}
	// A host-qualified pattern cannot be judged here: not counted.
	h.extraT = "allowusers ubuntu@203.0.113.*\n"
	if status = sshPasswordLoginStatus(context.Background(), h.env()); status.Reason != "no_key_account" {
		t.Fatalf("host pattern counted: %+v", status)
	}
	// root with a key and PermitRootLogin prohibit-password counts.
	h.extraT = ""
	h.keys = map[string]string{"/root": testRSAKey + "\n"}
	if status = sshPasswordLoginStatus(context.Background(), h.env()); !status.CanTurnOff || len(status.KeyAccounts) != 1 || !status.KeyAccounts[0].Root {
		t.Fatalf("root key: %+v", status)
	}
	// ...but not when root may not sign in.
	h2 := newFakeSSHHost(t, debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\nPermitRootLogin no\n"})
	h2.keys = map[string]string{"/root": testRSAKey + "\n"}
	if status = sshPasswordLoginStatus(context.Background(), h2.env()); status.Reason != "no_key_account" {
		t.Fatalf("root barred: %+v", status)
	}
}

func TestEveryFailedStepTakesTheChangeBack(t *testing.T) {
	// sshd refuses the configuration with the drop-in.
	h := newFakeSSHHost(t, debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\n"})
	env := h.env()
	calls := 0
	realSSHD := env.sshd
	env.sshd = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "-t" {
			calls++
			if calls == 2 { // first -t is the check before writing
				return []byte("line 5: Bad configuration option"), errors.New("exit status 255")
			}
		}
		return realSSHD(ctx, args...)
	}
	resp := setSSHPasswordLogin(context.Background(), env, &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOff})
	if resp.Outcome != transport.SSHPasswordLoginRolledBack || resp.Code != "validate_failed" || h.reloads != 0 {
		t.Fatalf("validate: %+v", resp)
	}
	if _, ok := h.dropIn(); ok || len(h.leftovers()) != 0 {
		t.Fatal("the drop-in stayed after a failed validation")
	}

	// The reload fails: removed and reloaded again.
	h = newFakeSSHHost(t, debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\n"})
	h.failReload = true
	resp = setSSHPasswordLogin(context.Background(), h.env(), &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOff})
	if resp.Code != "reload_failed" || resp.Outcome != transport.SSHPasswordLoginRollbackFailed && resp.Outcome != transport.SSHPasswordLoginRolledBack {
		t.Fatalf("reload: %+v", resp)
	}
	if _, ok := h.dropIn(); ok {
		t.Fatal("the drop-in stayed after a failed reload")
	}

	// sshd still says yes with the drop-in (an sshd that ignores it).
	h = newFakeSSHHost(t, debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\n"})
	env = h.env()
	inner := env.sshd
	written := false
	env.sshd = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "-T" && written {
			return []byte("passwordauthentication yes\nkbdinteractiveauthentication no\n"), nil
		}
		return inner(ctx, args...)
	}
	innerWrite := env.write
	env.write = func(path string, content []byte) error { written = true; return innerWrite(path, content) }
	resp = setSSHPasswordLogin(context.Background(), env, &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOff})
	if resp.Outcome != transport.SSHPasswordLoginRolledBack || resp.Code != "not_effective" {
		t.Fatalf("not effective: %+v", resp)
	}
	if _, ok := h.dropIn(); ok {
		t.Fatal("an ineffective drop-in stayed")
	}

	// Turning on: sshd refuses the configuration without the drop-in, so
	// it is put back.
	h = newFakeSSHHost(t, debianMain, map[string]string{sshPasswordLoginDropInName: sshPasswordLoginDropInText, "50-cloud-init.conf": "PasswordAuthentication yes\n"})
	env = h.env()
	status := sshPasswordLoginStatus(context.Background(), env)
	h.failTest = true
	resp = setSSHPasswordLogin(context.Background(), env, &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOn, ExpectedDropInSHA256: status.DropIn.SHA256})
	if resp.Outcome != transport.SSHPasswordLoginRolledBack || resp.Code != "validate_failed" {
		t.Fatalf("on validate: %+v", resp)
	}
	if text, ok := h.dropIn(); !ok || text != sshPasswordLoginDropInText || len(h.leftovers()) != 0 {
		t.Fatalf("the drop-in was not put back: %v %v", ok, h.leftovers())
	}
}

func TestInactiveSocketActivatedUnitAppliesAtNextStart(t *testing.T) {
	h := newFakeSSHHost(t, debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\n"})
	h.unitActive = false
	resp := setSSHPasswordLogin(context.Background(), h.env(), &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOff})
	if resp.Outcome != transport.SSHPasswordLoginTurnedOff || resp.Reloaded || !resp.AppliesAtNextStart || h.reloads != 0 {
		t.Fatalf("resp %+v", resp)
	}
}

func TestStaleDigestAndAlreadyOffChangeNothing(t *testing.T) {
	h := newFakeSSHHost(t, debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication yes\n"})
	resp := setSSHPasswordLogin(context.Background(), h.env(), &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOff, ExpectedDropInSHA256: strings.Repeat("a", 64)})
	if resp.Outcome != transport.SSHPasswordLoginRefused || resp.Code != "changed" {
		t.Fatalf("stale digest: %+v", resp)
	}
	off := newFakeSSHHost(t, debianMain, map[string]string{"50-cloud-init.conf": "PasswordAuthentication no\n"})
	resp = setSSHPasswordLogin(context.Background(), off.env(), &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOff})
	if resp.Outcome != transport.SSHPasswordLoginUnchanged || off.reloads != 0 {
		t.Fatalf("already off: %+v", resp)
	}
	if _, ok := off.dropIn(); ok {
		t.Fatal("already off wrote a file")
	}
	// On with no drop-in: nothing to remove, said as unchanged.
	resp = setSSHPasswordLogin(context.Background(), h.env(), &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOn})
	if resp.Outcome != transport.SSHPasswordLoginUnchanged {
		t.Fatalf("on without drop-in: %+v", resp)
	}
}

func TestTurnOnSaysWhichOwnerFileKeepsPasswordsOff(t *testing.T) {
	h := newFakeSSHHost(t, debianMain, map[string]string{
		sshPasswordLoginDropInName:  sshPasswordLoginDropInText,
		"60-cloudimg-settings.conf": "PasswordAuthentication no\n",
	})
	env := h.env()
	status := sshPasswordLoginStatus(context.Background(), env)
	resp := setSSHPasswordLogin(context.Background(), env, &transport.SetSSHPasswordLoginRequest{Action: transport.SSHPasswordLoginActionOn, ExpectedDropInSHA256: status.DropIn.SHA256})
	if resp.Outcome != transport.SSHPasswordLoginTurnedOn || resp.PasswordAuthentication != "no" || !strings.Contains(resp.StillOffBy, "60-cloudimg-settings.conf:1") {
		t.Fatalf("resp %+v", resp)
	}
}

func TestSSHDParsingHelpers(t *testing.T) {
	key, values := splitSSHDLine("  PasswordAuthentication=no # comment")
	if key != "passwordauthentication" || len(values) != 1 || values[0] != "no" {
		t.Fatalf("%q %v", key, values)
	}
	if key, _ := splitSSHDLine("# PasswordAuthentication yes"); key != "" {
		t.Fatal("comment read as a directive")
	}
	effective := parseSSHDEffective([]byte("allowusers alice\nallowusers bob\npasswordauthentication no\n"))
	if len(effective["allowusers"]) != 2 || effectiveOne(effective, "passwordauthentication") != "no" || effectiveOne(effective, "missing") != "unknown" {
		t.Fatalf("%v", effective)
	}
	account := sshAccount{name: "ubuntu", uid: 1000, home: "/home/ubuntu"}
	paths := authorizedKeyPaths([]string{"%h/.ssh/authorized_keys", "/etc/ssh/keys/%u", "/x/%Z"}, account)
	if len(paths) != 2 || paths[0] != "/home/ubuntu/.ssh/authorized_keys" || paths[1] != "/etc/ssh/keys/ubuntu" {
		t.Fatalf("paths %v", paths)
	}
	if countAuthorizedKeys([]byte("# comment\n\n"+testRSAKey+"\nnot a key\n")) != 1 {
		t.Fatal("key count")
	}
	if sshLoginShell("/usr/sbin/nologin") || sshLoginShell("/bin/false") || !sshLoginShell("/bin/bash") {
		t.Fatal("login shell")
	}
}
