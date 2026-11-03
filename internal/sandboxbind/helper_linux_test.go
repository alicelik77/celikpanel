//go:build linux

package sandboxbind

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// fakeMounter records what would be mounted: the path the bound descriptor
// really is (from /proc), never a name the helper chose.
type fakeMounter struct {
	bound   map[string]string
	unbinds int
	fail    error
}

func (f *fakeMounter) Bind(fd int, target string) error {
	if f.fail != nil {
		return f.fail
	}
	if f.bound == nil {
		f.bound = map[string]string{}
	}
	f.bound[target] = fdPath(fd)
	return nil
}

func (f *fakeMounter) Unmount(target string) error {
	if _, ok := f.bound[target]; !ok {
		return ErrNotMounted
	}
	delete(f.bound, target)
	f.unbinds++
	return nil
}

type tree struct {
	root, home, app, mountRoot string
	uid                        uint32
	mounter                    *fakeMounter
	nft                        []string
}

// newTree builds <tmp>/subs/1/sites/2/app (all the test account's) and a
// helper whose plan walks it as PlanFor walks the hosting tree.
func newTree(t *testing.T) *tree {
	t.Helper()
	base := t.TempDir()
	tr := &tree{root: filepath.Join(base, "subs"), uid: uint32(os.Getuid()), mounter: &fakeMounter{}}
	tr.home = filepath.Join(tr.root, "1", "sites", "2")
	tr.app = filepath.Join(tr.home, "app")
	tr.mountRoot = filepath.Join(base, "run")
	for _, dir := range []string{tr.root, filepath.Join(tr.root, "1"), filepath.Join(tr.root, "1", "sites"), tr.home, tr.app, filepath.Join(tr.app, "config")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tr.app, "config", "app.env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(tr.mountRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	return tr
}

func (tr *tree) plan(siteUID uint32) Plan {
	return Plan{
		Start: tr.root, StartUID: tr.uid, Home: tr.home, AppDir: tr.app,
		Steps: []Step{
			{Name: "1", UID: tr.uid, Private: true},
			{Name: "sites", UID: tr.uid, Private: true},
			{Name: "2", UID: siteUID},
			{Name: "app", UID: siteUID, SameMount: true},
		},
	}
}

func (tr *tree) helper(siteUID uint32) Helper {
	return Helper{
		Plan:      func(string, uint32) (Plan, error) { return tr.plan(siteUID), nil },
		MountRoot: tr.mountRoot, RootUID: tr.uid, Mounter: tr.mounter,
		LookupUser: func(string) (Account, error) { return Account{UID: siteUID, Home: tr.home}, nil },
		UnitInfo: func(unit string) (UnitInfo, error) {
			return UnitInfo{User: "flows_example_com", AppDir: "/var/www/celikpanel/subscriptions/1/sites/2/apps/n8n", Env: true}, nil
		},
		NFT: func(args ...string) error { tr.nft = append(tr.nft, strings.Join(args, " ")); return nil },
		Log: &bytes.Buffer{},
	}
}

func startReq() StartRequest {
	return StartRequest{Unit: "celiksvc-4.service", AppDir: "/var/www/celikpanel/subscriptions/1/sites/2/apps/n8n",
		User: "flows_example_com", EnvFrom: "config/app.env"}
}

func TestStartBindsTheOpenedFolderAndCopiesTheEnvironment(t *testing.T) {
	tr := newTree(t)
	h := tr.helper(tr.uid)
	if err := h.Start(startReq()); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tr.mountRoot, "celiksvc-4.service", "app")
	if got := tr.mounter.bound[target]; got != tr.app {
		t.Fatalf("bound %q onto %s, want %q", got, target, tr.app)
	}
	for _, dir := range []string{tr.mountRoot, filepath.Dir(target), target} {
		fi, err := os.Lstat(dir)
		if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", dir, fi.Mode(), err)
		}
	}
	env, err := os.ReadFile(filepath.Join(tr.mountRoot, "celiksvc-4.service", "app.env"))
	if err != nil || string(env) != "A=1\n" {
		t.Fatalf("env copy = %q, %v", env, err)
	}
	if fi, _ := os.Stat(filepath.Join(tr.mountRoot, "celiksvc-4.service", "app.env")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("env copy mode %v", fi.Mode())
	}
	if len(tr.nft) != 0 {
		t.Fatalf("the bind ran nft: %q", tr.nft)
	}
	// A second start (systemd restarted the unit without a stop) replaces
	// the earlier mount instead of stacking on it.
	if err := h.Start(startReq()); err != nil {
		t.Fatal(err)
	}
	if tr.mounter.unbinds != 1 || tr.mounter.bound[target] != tr.app {
		t.Fatalf("restart: unbinds=%d bound=%v", tr.mounter.unbinds, tr.mounter.bound)
	}
	if err := h.Stop("celiksvc-4.service"); err != nil {
		t.Fatal(err)
	}
	if len(tr.mounter.bound) != 0 {
		t.Fatalf("stop left %v", tr.mounter.bound)
	}
	if _, err := os.Lstat(filepath.Dir(target)); !os.IsNotExist(err) {
		t.Fatalf("the unit's folder is still there: %v", err)
	}
	// A stop with nothing to remove is not an error.
	if err := h.Stop("celiksvc-4.service"); err != nil {
		t.Fatal(err)
	}
	// The bind unit's start reads the unit's settings and binds the same way.
	if err := h.Bind("celiksvc-4.service"); err != nil || tr.mounter.bound[target] != tr.app {
		t.Fatalf("bind: %v %v", err, tr.mounter.bound)
	}
	h.UnitInfo = func(string) (UnitInfo, error) { return UnitInfo{}, errors.New("no such unit") }
	if err := h.Bind("celiksvc-4.service"); err == nil {
		t.Fatal("a bind without the unit's settings")
	}
}

func TestEgressRunsNFTOnly(t *testing.T) {
	tr := newTree(t)
	h := tr.helper(tr.uid)
	if err := h.EgressAdd("celiksvc-4.service"); err != nil {
		t.Fatal(err)
	}
	if err := h.EgressRemove("celiksvc-4.service"); err != nil {
		t.Fatal(err)
	}
	want := []string{"-f /usr/lib/celikpanel-sandbox/egress.nft",
		`add element inet celikpanel_egress smtp_blocked { "system.slice/celiksvc-4.service" }`,
		`delete element inet celikpanel_egress smtp_blocked { "system.slice/celiksvc-4.service" }`}
	if strings.Join(tr.nft, "|") != strings.Join(want, "|") || len(tr.mounter.bound) != 0 {
		t.Fatalf("nft = %q", tr.nft)
	}
	if err := h.EgressAdd("sshd.service"); err == nil {
		t.Fatal("a foreign unit accepted")
	}
	h.NFT = func(...string) error { return errors.New("Error: Could not process rule") }
	if err := h.EgressAdd("celiksvc-4.service"); err == nil || !strings.Contains(err.Error(), "SMTP block") {
		t.Fatalf("nft failure: %v", err)
	}
}

// TestParseUnitInfo: the bind unit learns the account, the app folder and
// the environment copy only from the sandboxed unit's own settings.
func TestParseUnitInfo(t *testing.T) {
	show := "User=flows_example_com\nBindPaths=/run/celikpanel-sandbox/celiksvc-4.service/app:/var/www/celikpanel/subscriptions/1/sites/2/apps/n8n:rbind\n" +
		"EnvironmentFiles=/run/celikpanel-sandbox/celiksvc-4.service/app.env (ignore_errors=no)\n"
	info, err := ParseUnitInfo("celiksvc-4.service", show)
	if err != nil || info.User != "flows_example_com" || info.AppDir != "/var/www/celikpanel/subscriptions/1/sites/2/apps/n8n" || !info.Env {
		t.Fatalf("info = %+v, %v", info, err)
	}
	info, err = ParseUnitInfo("celikrt-9.service", "User=shop\nBindPaths=/run/celikpanel-sandbox/celikrt-9.service/app:/var/www/celikpanel/subscriptions/1/sites/2/app:rbind\nEnvironmentFiles=/etc/celikpanel-runtime/celikrt-9.service.env (ignore_errors=yes)\n")
	if err != nil || info.Env || info.AppDir != "/var/www/celikpanel/subscriptions/1/sites/2/app" {
		t.Fatalf("runtime info = %+v, %v", info, err)
	}
	for name, bad := range map[string]string{
		"no bind":        "User=shop\nBindPaths=/srv/a:/var/www/x:rbind\n",
		"another unit's": "User=shop\nBindPaths=/run/celikpanel-sandbox/celikrt-8.service/app:/var/www/x:rbind\n",
		"no user":        "BindPaths=/run/celikpanel-sandbox/celikrt-9.service/app:/var/www/x:rbind\n",
		"twice":          "User=shop\nBindPaths=/run/celikpanel-sandbox/celikrt-9.service/app:/a:rbind /run/celikpanel-sandbox/celikrt-9.service/app:/b:rbind\n",
	} {
		if _, err := ParseUnitInfo("celikrt-9.service", bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func wantRefusal(t *testing.T, tr *tree, err error, code, fragment string) {
	t.Helper()
	r, ok := AsRefusal(err)
	if !ok || r.Code != code || !strings.Contains(r.Error(), fragment) {
		t.Fatalf("got %v, want %s (%s)", err, code, fragment)
	}
	if len(tr.mounter.bound) != 0 || len(tr.nft) != 0 {
		t.Fatalf("a refused start changed something: bound=%v nft=%q", tr.mounter.bound, tr.nft)
	}
}

// TestALinkAnywhereIsRefused: a link in place of any folder on the way, the
// app folder included, stops the start before anything is changed.
func TestALinkAnywhereIsRefused(t *testing.T) {
	for _, part := range []string{"1", "sites", "2", "app"} {
		t.Run(part, func(t *testing.T) {
			tr := newTree(t)
			elsewhere := t.TempDir()
			var victim string
			switch part {
			case "1":
				victim = filepath.Join(tr.root, "1")
			case "sites":
				victim = filepath.Join(tr.root, "1", "sites")
			case "2":
				victim = tr.home
			case "app":
				victim = tr.app
			}
			// Keep a real copy of the tree below the link, so the link would
			// lead to something that passes every other check.
			if err := os.Rename(victim, filepath.Join(elsewhere, "real")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(elsewhere, "real"), victim); err != nil {
				t.Fatal(err)
			}
			err := tr.helper(tr.uid).Start(startReq())
			wantRefusal(t, tr, err, CodeNotReal, "is a link")
		})
	}
}

func TestWrongOwnerNonFolderAndSharedParentAreRefused(t *testing.T) {
	tr := newTree(t)
	// The site's account is someone else than the folders' owner.
	err := tr.helper(tr.uid + 1).Start(startReq())
	wantRefusal(t, tr, err, CodeNotOwned, "belongs to uid")

	tr = newTree(t)
	if err := os.RemoveAll(tr.app); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr.app, []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, tr, tr.helper(tr.uid).Start(startReq()), CodeNotReal, "is not a folder")

	tr = newTree(t)
	if err := os.RemoveAll(tr.app); err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, tr, tr.helper(tr.uid).Start(startReq()), CodeNotReal, "is missing")

	tr = newTree(t)
	if err := os.Chmod(filepath.Join(tr.root, "1", "sites"), 0o777); err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, tr, tr.helper(tr.uid).Start(startReq()), CodeNotOwned, "can be changed by other accounts")

	tr = newTree(t)
	h := tr.helper(tr.uid)
	h.LookupUser = func(string) (Account, error) { return Account{UID: tr.uid, Home: "/home/elsewhere"}, nil }
	wantRefusal(t, tr, h.Start(startReq()), CodeNotOwned, "is not in the home")
}

func TestTheEnvironmentFileIsReadWithoutLinks(t *testing.T) {
	envPath := func(tr *tree) string { return filepath.Join(tr.app, "config", "app.env") }
	tr := newTree(t)
	secret := filepath.Join(t.TempDir(), "panel.env")
	if err := os.WriteFile(secret, []byte("SECRET=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Remove(envPath(tr))
	if err := os.Symlink(secret, envPath(tr)); err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, tr, tr.helper(tr.uid).Start(startReq()), CodeNotReal, "is a link")

	tr = newTree(t)
	os.Remove(envPath(tr))
	if err := syscall.Mkfifo(envPath(tr), 0o600); err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, tr, tr.helper(tr.uid).Start(startReq()), CodeNotReal, "is not a regular file")

	tr = newTree(t)
	os.Remove(envPath(tr))
	wantRefusal(t, tr, tr.helper(tr.uid).Start(startReq()), CodeNotReal, "is missing")

	tr = newTree(t)
	if err := os.Rename(filepath.Join(tr.app, "config"), filepath.Join(tr.app, "config.real")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(tr.app, "config.real"), filepath.Join(tr.app, "config")); err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, tr, tr.helper(tr.uid).Start(startReq()), CodeNotReal, "is a link")

	tr = newTree(t)
	if err := os.WriteFile(envPath(tr), bytes.Repeat([]byte("A"), MaxEnvBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, tr, tr.helper(tr.uid).Start(startReq()), CodeNotReal, "is larger")

	// Without an environment file nothing is read and no copy is left.
	tr = newTree(t)
	req := startReq()
	req.EnvFrom = ""
	os.Remove(envPath(tr))
	if err := tr.helper(tr.uid).Start(req); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(tr.mountRoot, req.Unit, "app.env")); !os.IsNotExist(err) {
		t.Fatalf("an environment copy without an environment file: %v", err)
	}
}

func TestBadArgumentsAndMountFailures(t *testing.T) {
	tr := newTree(t)
	h := tr.helper(tr.uid)
	for name, mutate := range map[string]func(*StartRequest){
		"unit":       func(r *StartRequest) { r.Unit = "sshd.service" },
		"user":       func(r *StartRequest) { r.User = "root" },
		"user shape": func(r *StartRequest) { r.User = "a b" },
		"env":        func(r *StartRequest) { r.EnvFrom = "../../etc/shadow" },
	} {
		req := startReq()
		mutate(&req)
		if err := h.Start(req); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := h.Stop("../x"); err == nil {
		t.Error("a stop of a foreign unit accepted")
	}
	// A link where the unit's mount point belongs is refused, not followed.
	if err := os.Symlink(t.TempDir(), filepath.Join(tr.mountRoot, "celiksvc-4.service")); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(startReq()); err == nil || !strings.Contains(err.Error(), "is not a folder of root's") {
		t.Fatalf("link at the mount point: %v", err)
	}
	tr = newTree(t)
	tr.mounter.fail = errors.New("EPERM")
	if err := tr.helper(tr.uid).Start(startReq()); err == nil || !strings.Contains(err.Error(), "could not be bound") {
		t.Fatalf("mount failure: %v", err)
	}
}

// TestMainRefusesUsage: the command line refuses unknown words and flags.
func TestMainRefusesUsage(t *testing.T) {
	keep := os.Environ()
	defer func() {
		for _, kv := range keep {
			if k, v, ok := strings.Cut(kv, "="); ok {
				os.Setenv(k, v)
			}
		}
	}()
	var out bytes.Buffer
	if code := Main(nil, &out); code != 2 {
		t.Fatalf("no arguments: %d", code)
	}
	if code := Main([]string{"mount", "--unit=celikrt-1.service"}, &out); code != 2 {
		t.Fatalf("unknown command: %d", code)
	}
	if code := Main([]string{"bind", "--shell=/bin/sh"}, &out); code != 2 {
		t.Fatalf("unknown flag: %d", code)
	}
	if code := Main([]string{"unbind", "--unit=celikrt-1.service", "extra"}, &out); code != 2 {
		t.Fatalf("extra word: %d", code)
	}
	if code := Main([]string{"bind", "--unit=sshd.service"}, &out); code != 2 {
		t.Fatalf("a foreign unit: %d", code)
	}
}
