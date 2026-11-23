//go:build linux

package sandboxbind

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
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
			return UnitInfo{User: "flows_example_com", AppDir: "/var/www/celikpanel/subscriptions/1/sites/2/apps/n8n"}, nil
		},
		NFT: func(args ...string) error { tr.nft = append(tr.nft, strings.Join(args, " ")); return nil },
		Log: &bytes.Buffer{},
	}
}

func startReq() StartRequest {
	return StartRequest{Unit: "celiksvc-4.service", AppDir: "/var/www/celikpanel/subscriptions/1/sites/2/apps/n8n",
		User: "flows_example_com"}
}

func TestStartBindsTheOpenedFolder(t *testing.T) {
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
	// The site's config/app.env is never copied: environment values come
	// only from root's own files outside the hosting tree.
	if _, err := os.Lstat(filepath.Join(tr.mountRoot, "celiksvc-4.service", "app.env")); !os.IsNotExist(err) {
		t.Fatalf("the helper copied a file of the site's folder: %v", err)
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

// TestEgressFollowsTheSitesSlice (S4 R2a, kept for going back one release):
// a unit in its site's slice celiksite_<id>.slice adds and removes the
// element of the cgroup it really runs in; a unit an owner's drop-in moved
// anywhere else does not start (TestEgressPlaceOrder has the order and the
// case in which systemd does not answer).
func TestEgressFollowsTheSitesSlice(t *testing.T) {
	tr := newTree(t)
	h := tr.helper(tr.uid)
	cgroup := "/celiksite_9.slice/celiksvc-4.service"
	h.ControlGroup = func(unit string) (string, error) {
		if unit != "celiksvc-4.service" {
			return "", errors.New("unexpected unit")
		}
		return cgroup, nil
	}
	if err := h.EgressAdd("celiksvc-4.service"); err != nil {
		t.Fatal(err)
	}
	if err := h.EgressRemove("celiksvc-4.service"); err != nil {
		t.Fatal(err)
	}
	want := []string{"-f /usr/lib/celikpanel-sandbox/egress.nft",
		`add element inet celikpanel_egress smtp_blocked { "celiksite_9.slice/celiksvc-4.service" }`,
		`delete element inet celikpanel_egress smtp_blocked { "celiksite_9.slice/celiksvc-4.service" }`}
	if strings.Join(tr.nft, "|") != strings.Join(want, "|") {
		t.Fatalf("nft = %q", tr.nft)
	}
	// The system slice: exactly the element this release always added.
	tr.nft = nil
	cgroup = "/system.slice/celiksvc-4.service"
	if err := h.EgressAdd("celiksvc-4.service"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tr.nft, "|"); got != `-f /usr/lib/celikpanel-sandbox/egress.nft|add element inet celikpanel_egress smtp_blocked { "system.slice/celiksvc-4.service" }` {
		t.Fatalf("system slice nft = %q", tr.nft)
	}
	tr.nft = nil
	cgroup = "/owner.slice/celiksvc-4.service"
	var place *PlaceError
	if err := h.EgressAdd("celiksvc-4.service"); !errors.As(err, &place) || len(tr.nft) != 0 {
		t.Fatalf("a unit in another slice was let start: %v %q", err, tr.nft)
	}
	for show, want := range map[string]string{
		"ControlGroup=/celiksite_9.slice/celiksvc-4.service\nSlice=celiksite_9.slice\n": "/celiksite_9.slice/celiksvc-4.service",
		"ControlGroup=\nSlice=celiksite_9.slice\n":                                      "/celiksite_9.slice/celiksvc-4.service",
		"ControlGroup=\nSlice=\n":                                                       "",
	} {
		if got := ParseControlGroup("celiksvc-4.service", show); got != want {
			t.Errorf("%q = %q", show, got)
		}
	}
	// The production helper reads its own cgroup first, then systemd.
	if p := ProductionHelper(nil); p.SelfCgroup == nil || p.ControlGroup == nil {
		t.Fatal("the production helper does not read the unit's cgroup")
	}
}

// fakeKernelNFT is nft as the kernel answers "add element" for a cgroup
// path: only a path that exists is taken (nft turns it into the cgroup's
// inode).
func fakeKernelNFT(tr *tree, exists map[string]bool) func(args ...string) error {
	return func(args ...string) error {
		if len(args) == 6 && args[0] == "add" {
			path := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(args[5], "{ "), " }"), `"`)
			if !exists[path] {
				return errors.New(`exit status 1: Error: cgroupv2 path fails: No such file or directory`)
			}
		}
		tr.nft = append(tr.nft, strings.Join(args, " "))
		return nil
	}
}

// TestEgressPlaceOrder: the helper's own cgroup first (no question to
// systemd), then systemd, then the system slice's element as before: the
// SMTP helper adds no dependency on systemd answering, and no program ever
// starts outside the block.
func TestEgressPlaceOrder(t *testing.T) {
	unit := "celiksvc-4.service"
	asked := 0
	setup := func(self string, selfErr error, systemd string, systemdErr error, exists map[string]bool) (*tree, Helper, *bytes.Buffer) {
		tr := newTree(t)
		h := tr.helper(tr.uid)
		log := &bytes.Buffer{}
		h.Log = log
		h.SelfCgroup = func() (string, error) { return self, selfErr }
		h.ControlGroup = func(string) (string, error) { asked++; return systemd, systemdErr }
		h.NFT = fakeKernelNFT(tr, exists)
		return tr, h, log
	}
	down := errors.New(`systemctl show celiksvc-4.service: exit status 1: Failed to connect to bus: Connection refused`)
	inSystem := map[string]bool{"system.slice/" + unit: true}
	inSite := map[string]bool{"celiksite_9.slice/" + unit: true}

	// 1. Its own cgroup names the site's slice: systemd is not asked, and
	// nothing is logged.
	asked = 0
	tr, h, quiet := setup("/celiksite_9.slice/"+unit, nil, "", down, inSite)
	if err := h.EgressAdd(unit); err != nil || asked != 0 || !strings.Contains(strings.Join(tr.nft, "|"), `{ "celiksite_9.slice/`+unit+`" }`) || quiet.Len() != 0 {
		t.Fatalf("own cgroup in the site's slice: %v asked=%d %q %s", err, asked, tr.nft, quiet)
	}
	if err := h.EgressRemove(unit); err != nil || asked != 0 || !strings.Contains(tr.nft[len(tr.nft)-1], `delete element inet celikpanel_egress smtp_blocked { "celiksite_9.slice/`+unit+`" }`) {
		t.Fatalf("remove from the site's slice: %v %q", err, tr.nft)
	}
	// ... and the system slice: as on main, without asking systemd.
	tr, h, _ = setup("/system.slice/"+unit, nil, "", down, inSystem)
	if err := h.EgressAdd(unit); err != nil || asked != 0 ||
		strings.Join(tr.nft, "|") != `-f /usr/lib/celikpanel-sandbox/egress.nft|add element inet celikpanel_egress smtp_blocked { "system.slice/`+unit+`" }` {
		t.Fatalf("own cgroup in the system slice: %v %q", err, tr.nft)
	}

	// 2. Its own cgroup is not one of the two forms (a ".control" subgroup,
	// another unit, run by hand, unreadable): systemd is asked.
	for _, self := range []string{"/celiksite_9.slice/" + unit + "/.control", "/system.slice/celiksvc-5.service", "/", ""} {
		asked = 0
		var said *bytes.Buffer
		tr, h, said = setup(self, nil, "/celiksite_9.slice/"+unit, nil, inSite)
		if err := h.EgressAdd(unit); err != nil || asked != 1 || !strings.Contains(strings.Join(tr.nft, "|"), `"celiksite_9.slice/`+unit+`"`) {
			t.Fatalf("own cgroup %q: %v asked=%d %q", self, err, asked, tr.nft)
		}
		// One line says which step decided and the place taken.
		if !strings.Contains(said.String(), "systemd says it runs in /celiksite_9.slice/"+unit) || strings.Count(said.String(), "\n") != 1 {
			t.Fatalf("own cgroup %q: journal %q", self, said)
		}
	}
	asked = 0
	tr, h, _ = setup("", errors.New("no such file"), "/celiksite_9.slice/"+unit, nil, inSite)
	if err := h.EgressAdd(unit); err != nil || asked != 1 {
		t.Fatalf("unreadable own cgroup: %v asked=%d", err, asked)
	}
	// systemd names another place: refused before nft adds anything.
	tr, h, _ = setup("", nil, "/owner.slice/"+unit, nil, inSystem)
	var place *PlaceError
	if err := h.EgressAdd(unit); !errors.As(err, &place) || len(tr.nft) != 0 {
		t.Fatalf("another slice: %v %q", err, tr.nft)
	}

	// 3. systemd cannot be asked: a unit in the system slice starts as on
	// main, and the journal says why systemd was not used.
	tr, h, log := setup("", nil, "", down, inSystem)
	if err := h.EgressAdd(unit); err != nil || !strings.Contains(strings.Join(tr.nft, "|"), `{ "system.slice/`+unit+`" }`) ||
		!strings.Contains(log.String(), "Failed to connect to bus") {
		t.Fatalf("systemd down, system slice: %v %q %s", err, tr.nft, log)
	}
	log.Reset()
	if err := h.EgressRemove(unit); err != nil || !strings.Contains(log.String(), "removes the system slice's element") {
		t.Fatalf("systemd down, remove: %v %s", err, log)
	}
	// systemd answers with no place: the same.
	tr, h, _ = setup("", nil, "", nil, inSystem)
	if err := h.EgressAdd(unit); err != nil {
		t.Fatalf("no place named: %v", err)
	}
	// systemd cannot be asked and the unit is NOT in the system slice: nft
	// refuses the path, the unit does not start (never outside the block).
	tr, h, _ = setup("", nil, "", down, inSite)
	if err := h.EgressAdd(unit); !errors.As(err, &place) || !strings.Contains(err.Error(), "No such file or directory") ||
		strings.Contains(strings.Join(tr.nft, "|"), "add element") {
		t.Fatalf("systemd down, unit elsewhere: %v %q", err, tr.nft)
	}
	if lines := egressGuidance(unit); len(lines) != 2 || !strings.Contains(lines[0], "systemctl show -p ControlGroup,Slice "+unit) ||
		!strings.Contains(lines[0], "administrator") {
		t.Fatalf("guidance = %q", lines)
	}
}

// TestAddFailureIsAPlaceProblemOnlyWhenItIs: nft's refusal of the element
// is reported as a problem of the unit's place (with the place guidance)
// only when nft says the cgroup path fails or that path does not exist under
// the cgroup root; any other failure keeps the plain error.
func TestAddFailureIsAPlaceProblemOnlyWhenItIs(t *testing.T) {
	unit := "celiksvc-4.service"
	run := func(nftErr string, exists func(string) bool) error {
		tr := newTree(t)
		h := tr.helper(tr.uid)
		h.SelfCgroup = func() (string, error) { return "/system.slice/" + unit, nil }
		h.CgroupExists = exists
		h.NFT = func(args ...string) error {
			if args[0] == "add" {
				return errors.New(nftErr)
			}
			return nil
		}
		return h.EgressAdd(unit)
	}
	there := func(string) bool { return true }
	gone := func(path string) bool {
		if path != "system.slice/"+unit {
			t.Errorf("checked %q", path)
		}
		return false
	}
	flushed := "exit status 1: Error: Could not process rule: No such file or directory add element inet celikpanel_egress smtp_blocked { \"system.slice/" + unit + "\" }"
	var place *PlaceError
	for name, c := range map[string]struct {
		nftErr string
		exists func(string) bool
		place  bool
	}{
		"nft says the path fails":       {"exit status 1: Error: cgroupv2 path fails: No such file or directory", there, true},
		"the path is not there":         {flushed, gone, true},
		"the table was flushed between": {flushed, there, false},
		"nft was killed":                {"signal: killed", nil, false},
	} {
		err := run(c.nftErr, c.exists)
		if err == nil || errors.As(err, &place) != c.place || !strings.Contains(err.Error(), "could not be added to the SMTP block") {
			t.Errorf("%s: %v (place error %v)", name, err, errors.As(err, &place))
		}
	}
	// The production check reads the cgroup root.
	old := cgroupRoot
	t.Cleanup(func() { cgroupRoot = old })
	cgroupRoot = t.TempDir()
	if err := os.MkdirAll(filepath.Join(cgroupRoot, "system.slice", unit), 0o755); err != nil {
		t.Fatal(err)
	}
	if !cgroupExists("system.slice/"+unit) || cgroupExists("celiksite_9.slice/"+unit) || cgroupExists("../../etc") {
		t.Fatal("cgroupExists")
	}
	if ProductionHelper(nil).CgroupExists == nil {
		t.Fatal("the production helper does not check the path")
	}
}

// TestProductionPlaceReads: the production reads behind the seams - the
// helper's own /proc/self/cgroup and `systemctl show` with its argv, its
// output and its failure in systemctl's own words.
func TestProductionPlaceReads(t *testing.T) {
	for content, want := range map[string]string{
		"0::/system.slice/celiksvc-4.service\n": "/system.slice/celiksvc-4.service",
		"12:pids:/system.slice/x\n1:name=systemd:/system.slice/x\n0::/celiksite_9.slice/celiksvc-4.service\n": "/celiksite_9.slice/celiksvc-4.service",
		"1:name=systemd:/system.slice/celiksvc-4.service\n":                                                   "",
		"": "",
	} {
		if got := ParseSelfCgroup(content); got != want {
			t.Errorf("%q = %q, want %q", content, got, want)
		}
	}
	dir := t.TempDir()
	oldPath, oldRun := selfCgroupPath, systemctlRun
	t.Cleanup(func() { selfCgroupPath, systemctlRun = oldPath, oldRun })
	selfCgroupPath = filepath.Join(dir, "cgroup")
	if err := os.WriteFile(selfCgroupPath, []byte("0::/celiksite_9.slice/celiksvc-4.service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := readSelfCgroup(); err != nil || got != "/celiksite_9.slice/celiksvc-4.service" {
		t.Fatalf("own cgroup = %q %v", got, err)
	}
	var argv []string
	systemctlRun = func(program string, args ...string) ([]byte, error) {
		argv = append([]string{program}, args...)
		return []byte("ControlGroup=/celiksite_9.slice/celiksvc-4.service\nSlice=celiksite_9.slice\n"), nil
	}
	if got, err := readControlGroup("celiksvc-4.service"); err != nil || got != "/celiksite_9.slice/celiksvc-4.service" ||
		strings.Join(argv, " ") != "/usr/bin/systemctl show celiksvc-4.service --property=ControlGroup,Slice" {
		t.Fatalf("systemd read = %q %v %q", got, err, argv)
	}
	// systemctl's failure, with its own words (a real failed command).
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh here")
	}
	systemctlRun = func(string, ...string) ([]byte, error) {
		return exec.Command(sh, "-c", "echo 'Failed to connect to bus: Connection refused' >&2; exit 1").Output()
	}
	if _, err := readControlGroup("celiksvc-4.service"); err == nil || !strings.Contains(err.Error(), "Failed to connect to bus: Connection refused") {
		t.Fatalf("systemd failure = %v", err)
	}
}

// TestParseUnitInfo: the bind unit learns the account and the app folder
// only from the sandboxed unit's own settings; it reads no environment file.
func TestParseUnitInfo(t *testing.T) {
	show := "User=flows_example_com\nBindPaths=/run/celikpanel-sandbox/celiksvc-4.service/app:/var/www/celikpanel/subscriptions/1/sites/2/apps/n8n:rbind\n" +
		"EnvironmentFiles=/etc/celikpanel-svcapps/celiksvc-4.env (ignore_errors=no)\n"
	info, err := ParseUnitInfo("celiksvc-4.service", show)
	if err != nil || info.User != "flows_example_com" || info.AppDir != "/var/www/celikpanel/subscriptions/1/sites/2/apps/n8n" {
		t.Fatalf("info = %+v, %v", info, err)
	}
	info, err = ParseUnitInfo("celikrt-9.service", "User=shop\nBindPaths=/run/celikpanel-sandbox/celikrt-9.service/app:/var/www/celikpanel/subscriptions/1/sites/2/app:rbind\nEnvironmentFiles=/etc/celikpanel-runtime/celikrt-9.service.env (ignore_errors=yes)\n")
	if err != nil || info.AppDir != "/var/www/celikpanel/subscriptions/1/sites/2/app" {
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

// TestTheHelperReadsNoEnvironmentFile: whatever the site's account puts at
// config/app.env (a link to a root-only file, a FIFO that would block a
// reader), the helper neither reads nor copies it. A service app's values
// are root's own file in /etc/celikpanel-svcapps (the alpha.118 security
// fix), a runtime app's in /etc/celikpanel-runtime.
func TestTheHelperReadsNoEnvironmentFile(t *testing.T) {
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
	if err := tr.helper(tr.uid).Start(startReq()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(tr.mountRoot, "celiksvc-4.service"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "app" {
		t.Fatalf("unit folder = %v, %v; want only the mount point", entries, err)
	}

	tr = newTree(t)
	os.Remove(envPath(tr))
	if err := syscall.Mkfifo(envPath(tr), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := tr.helper(tr.uid).Start(startReq()); err != nil {
		t.Fatalf("a FIFO at config/app.env must not matter: %v", err)
	}
}

func TestBadArgumentsAndMountFailures(t *testing.T) {
	tr := newTree(t)
	h := tr.helper(tr.uid)
	for name, mutate := range map[string]func(*StartRequest){
		"unit":       func(r *StartRequest) { r.Unit = "sshd.service" },
		"user":       func(r *StartRequest) { r.User = "root" },
		"user shape": func(r *StartRequest) { r.User = "a b" },
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
