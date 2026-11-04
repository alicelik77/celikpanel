package siteruntime

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

func reqFiles(files map[string]string) ReadSourceFile {
	return func(rel string) ([]byte, error) {
		text, ok := files[rel]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(text), nil
	}
}

func refusalOf(t *testing.T, files map[string]string) *ReqRefusal {
	t.Helper()
	_, _, err := CheckRequirements(reqFiles(files))
	var refusal *ReqRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("not refused: %v (%v)", err, files)
	}
	return refusal
}

func TestRequirementsRefuseEveryOtherSourceNamingTheLine(t *testing.T) {
	cases := []struct {
		line string
		code string
	}{
		{"-e .", ReqEditable},
		{"--editable=git+https://example.com/x.git#egg=x", ReqEditable},
		{"git+https://github.com/org/pkg.git@v1#egg=pkg", ReqVCS},
		{"pkg @ git+https://github.com/org/pkg.git", ReqVCS},
		{"hg+https://hg.example.com/pkg", ReqVCS},
		{"https://files.example.com/pkg-1.0-py3-none-any.whl", ReqURL},
		{"pkg @ https://example.com/pkg.tar.gz", ReqURL},
		{"pkg@https://example.com/pkg.tar.gz", ReqURL},
		{"./vendor/pkg", ReqPath},
		{"../lib", ReqPath},
		{"/tmp/pkg.whl", ReqPath},
		{"pkg-1.0-py3-none-any.whl", ReqPath},
		{"vendor/pkg.tar.gz", ReqPath},
		{"-i https://mirror.example.com/simple", ReqIndexURL},
		{"--index-url=https://mirror.example.com/simple", ReqIndexURL},
		{"--no-index", ReqIndexURL},
		{"--extra-index-url https://evil.example.com/simple", ReqExtraIndex},
		{"--trusted-host evil.example.com", ReqTrustedHost},
		{"-f https://example.com/wheels/", ReqFindLinks},
		{"--find-links=/tmp", ReqFindLinks},
		{"-c /etc/passwd", ReqConstraintOut},
		{"--constraint=../outside.txt", ReqConstraintOut},
		{"-r ../../other-site/requirements.txt", ReqNestedOutside},
		{"-r /etc/shadow", ReqNestedOutside},
		{"--requirement https://example.com/r.txt", ReqNestedOutside},
		{"-r missing.txt", ReqNestedMissing},
		{"--no-binary :all:", ReqOption},
		{"--only-binary=:none:", ReqOption},
		{"--use-pep517", ReqOption},
		{"Django==4.2 --global-option=--x", ReqOption},
		{"Django==4.2 --config-settings=x=y", ReqOption},
		{"Django==${VERSION}", ReqSyntax},
		{"Django==4.2 --hash=md5:abc", ReqSyntax},
		{"Django (==4.2)", ReqSyntax},
		{"`id`", ReqSyntax},
	}
	for _, c := range cases {
		files := map[string]string{RequirementsFile: "flask==3.0.0\n# a comment\n\n" + c.line + "\n"}
		r := refusalOf(t, files)
		if r.Code != c.code || r.Line != 4 || r.File != RequirementsFile {
			t.Errorf("%q: %+v, want %s on line 4", c.line, r, c.code)
		}
		if !strings.HasPrefix(r.Message(), "requirements.txt:4 "+c.code+": ") {
			t.Errorf("%q: message %q", c.line, r.Message())
		}
	}
	// Credentials in a refused line are hidden.
	r := refusalOf(t, map[string]string{RequirementsFile: "--extra-index-url https://user:s3cret@private.example.com/simple\n"})
	if strings.Contains(r.Message(), "s3cret") || !strings.Contains(r.Message(), "***@") {
		t.Fatalf("credentials in %q", r.Message())
	}
	// Every code has words on screen (the web test reads the list).
	if len(ReqRefusalCodes) != 15 {
		t.Fatalf("codes %v", ReqRefusalCodes)
	}
}

func TestRequirementsAcceptPyPILinesAndFlattenNestedFiles(t *testing.T) {
	hash := strings.Repeat("a", 64)
	files := map[string]string{
		RequirementsFile: "# web\nDjango>=4.2,<5 ; python_version >= \"3.10\"\n" +
			"psycopg[binary]==3.2.1 \\\n    --hash=sha256:" + hash + "\n" +
			"-r base/common.txt  # shared\n--prefer-binary\n-c constraints.txt\n",
		"base/common.txt": "requests==2.32.3\n-r ../extra.txt\n-r ../extra.txt\n",
		"extra.txt":       "gunicorn\n",
		"constraints.txt": "urllib3<3\n",
	}
	req, present, err := CheckRequirements(reqFiles(files))
	if err != nil || !present {
		t.Fatalf("%v %v", err, present)
	}
	want := "Django>=4.2,<5 ; python_version >= \"3.10\"\npsycopg[binary]==3.2.1 --hash=sha256:" + hash + "\nrequests==2.32.3\ngunicorn\n--prefer-binary\n"
	if req.Text != want || req.Constraints != "urllib3<3\n" || !req.Hashed || req.Packages != 4 {
		t.Fatalf("flattened %+v", req)
	}
	if _, present, err := CheckRequirements(reqFiles(map[string]string{})); present || err != nil {
		t.Fatal("no requirements.txt is not an error")
	}
	if r := refusalOf(t, map[string]string{RequirementsFile: "-r a.txt\n", "a.txt": "-r b.txt\n", "b.txt": "-r c.txt\n", "c.txt": "-r d.txt\n", "d.txt": "-r e.txt\n", "e.txt": "x\n"}); r.Code != ReqNestedTooDeep {
		t.Fatalf("%+v", r)
	}
	if r := refusalOf(t, map[string]string{RequirementsFile: strings.Repeat("x\n", maxRequirementsLines+1)}); r.Code != ReqTooLarge {
		t.Fatalf("%+v", r)
	}
	if r := refusalOf(t, map[string]string{RequirementsFile: "-c c.txt\n", "c.txt": "-r x.txt\n"}); r.Code != ReqOption || r.File != "c.txt" {
		t.Fatalf("a requirement file inside a constraints file: %+v", r)
	}
	if _, _, err := CheckRequirements(reqFiles(map[string]string{RequirementsFile: "--require-hashes\nx==1 --hash=sha256:" + hash + "\n"})); err != nil {
		t.Fatal(err)
	}
}

func TestPythonSettingsAndWorkers(t *testing.T) {
	limits := DefaultLimits(KindPython)
	good := PythonSettings{Server: PythonWSGI, Callable: "mysite.wsgi:application", Workers: 4}
	if err := good.Validate(limits); err != nil {
		t.Fatal(err)
	}
	for _, callable := range []string{"main:app", "a.b.c:create", "_x:_y"} {
		if !ValidCallable(callable) {
			t.Errorf("%q refused", callable)
		}
	}
	for _, callable := range []string{"", "main", "main:", ":app", "main:create_app()", "main:app;id", "../x:app", "a b:app", "main:app:x",
		"main:$HOME", "main:%i", "1main:app", "main.:app", "a-b:app"} {
		if ValidCallable(callable) {
			t.Errorf("%q accepted", callable)
		}
	}
	if MaxWorkers(64) != 1 || MaxWorkers(512) != 4 || MaxWorkers(4096) != 8 || MaxWorkers(100000) != 8 {
		t.Fatal("worker cap")
	}
	for _, bad := range []PythonSettings{{Server: "cgi", Callable: "a:b", Workers: 1}, {Server: PythonASGI, Callable: "a:b", Workers: 5},
		{Server: PythonASGI, Callable: "a:b", Workers: 0}, {Server: PythonASGI, Callable: "a", Workers: 1}} {
		if bad.Validate(limits) == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	back, err := PythonSettingsFrom(good.Callable, good.Args())
	if err != nil || back != good {
		t.Fatalf("round trip %+v %v", back, err)
	}
	if ValidateArgs(good.Args()) != nil {
		t.Fatal("the settings tokens must be valid argument tokens")
	}
	if _, err := PythonSettingsFrom("a:b", []string{"server=wsgi", "shell=/bin/sh"}); err == nil {
		t.Fatal("an unknown token was read")
	}
}

func TestDistributionPythonPerSystem(t *testing.T) {
	cases := []struct {
		id, version, minor string
		rolling            bool
	}{
		{"debian", "12", "3.11", false}, {"debian", "13", "3.13", false},
		{"ubuntu", "22.04", "3.10", false}, {"ubuntu", "24.04", "3.12", false}, {"arch", "", "3.14", true},
	}
	for _, c := range cases {
		minor, rolling, ok := ExpectedPython(c.id, c.version)
		if !ok || minor != c.minor || rolling != c.rolling || !PinnedMinor(minor) {
			t.Errorf("%s %s: %s %v %v", c.id, c.version, minor, rolling, ok)
		}
	}
	if _, _, ok := ExpectedPython("debian", "11"); ok {
		t.Fatal("an unsupported system has a Python")
	}
	if strings.Join(PythonPackages("apt"), " ") != "python3 python3-venv" || strings.Join(PythonPackages("pacman"), " ") != "python" || PythonPackages("dnf") != nil {
		t.Fatal("packages")
	}
	if m, ok := PythonMinorFromLink("python3.13"); !ok || m != "3.13" {
		t.Fatal("link")
	}
	if _, ok := PythonMinorFromLink("/opt/pyenv/bin/python"); ok {
		t.Fatal("a link without a minor")
	}
	if PythonOffering("3.12") != "component:python:3.12" || PythonBinary("3.12") != "/usr/bin/python3.12" || PythonEnsurepip("3.12") != "/usr/lib/python3.12/ensurepip/__init__.py" {
		t.Fatal("names")
	}
	if _, ok := PythonMinorOfOffering("component:python:3.x"); ok {
		t.Fatal("bad minor accepted")
	}
}

func TestAppServerPinsPerMinor(t *testing.T) {
	for _, minor := range PinnedMinors {
		pins, err := AppServerPins(minor)
		if err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for _, p := range pins {
			if len(p.SHA256) != 64 || strings.Trim(p.SHA256, "0123456789abcdef") != "" || !strings.HasSuffix(p.Wheel, "-py3-none-any.whl") {
				t.Fatalf("%s: %+v", minor, p)
			}
			names = append(names, p.Name)
		}
		want := "click,gunicorn,h11,uvicorn"
		if minor == "3.10" {
			want = "click,gunicorn,h11,typing_extensions,uvicorn"
		}
		if strings.Join(names, ",") != want {
			t.Fatalf("%s: %v", minor, names)
		}
		text, _ := AppServerRequirements(minor)
		if !strings.Contains(text, "gunicorn==26.2.0 --hash=sha256:bd249d0b3f7972f7432f0a6b6ff3b3ee2d129f70cd1ff6c09a9dd9e29a2b88e3\n") ||
			!strings.Contains(text, "uvicorn==0.54.0 --hash=sha256:505bdb0f318731d45f1f712071fc781a8981f6847a31c902c9f5e652d4f67faf\n") {
			t.Fatalf("%s:\n%s", minor, text)
		}
	}
	if _, err := AppServerPins("3.15"); err == nil {
		t.Fatal("an unpinned minor")
	}
}

func pythonParams(t *testing.T) UnitParams {
	p := params(t)
	p.Kind, p.Entry, p.Args = KindPython, "mysite.wsgi:application", nil
	p.Python = &PythonUnit{Venv: 3, Settings: PythonSettings{Server: PythonWSGI, Callable: "mysite.wsgi:application", Workers: 2}}
	return p
}

func TestPythonUnitRunsTheAppServerWithoutAShell(t *testing.T) {
	root := testLayout(t).Root
	for _, version := range []int{249, 257} {
		p := pythonParams(t)
		p.Caps = fullCaps(version)
		text, err := RenderUnit(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"ExecStart=" + root + "/venv-3/bin/python -m gunicorn --bind 127.0.0.1:30010 --workers 2 mysite.wsgi:application\n",
			"WorkingDirectory=" + root + "/src\n", "Environment=PATH=" + root + "/venv-3/bin:/usr/bin:/bin\n",
			"Environment=PYTHONUNBUFFERED=1\n", "User=shop_example_com\n", "ProcSubset=pid\n", "SocketBindAllow=tcp:30010\n",
			"# celikpanel-runtime-kind: python\n", "Description=CelikPanel python web app of shop.example.com\n",
			"ExecStartPre=+/usr/lib/celikpanel-sandbox/sandbox-helper egress-add --unit=celikrt-12.service\n",
			"Requires=celikpanel-sandbox@celikrt-12.service\n",
			"BindPaths=/run/celikpanel-sandbox/celikrt-12.service/app:" + root + "\n",
			"EnvironmentFile=-/etc/celikpanel-runtime/celikrt-12.service.env\n",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%d: missing %q\n%s", version, want, text)
			}
		}
		// The base sandbox only: an interpreter executes its own extension
		// modules and may generate code.
		for _, banned := range []string{"MemoryDenyWriteExecute", "NoExecPaths", "ExecPaths=", "/bin/sh", "sh -c", "$", "%i", "%h"} {
			if strings.Contains(strings.Replace(text, "CPUQuota=100%", "", 1), banned) {
				t.Fatalf("%d: %q in the unit", version, banned)
			}
		}
		if UnitKind(text) != KindPython || ActiveVenv(text) != 3 {
			t.Fatalf("kind %s venv %d", UnitKind(text), ActiveVenv(text))
		}
	}
	p := pythonParams(t)
	p.Python.Settings.Server = PythonASGI
	text, err := RenderUnit(p)
	if err != nil || !strings.Contains(text, "ExecStart="+root+"/venv-3/bin/python -m uvicorn --host 127.0.0.1 --port 30010 --workers 2 mysite.wsgi:application\n") {
		t.Fatalf("%v\n%s", err, text)
	}
	cases := map[string]func(*UnitParams){
		"no settings":      func(p *UnitParams) { p.Python = nil },
		"no venv":          func(p *UnitParams) { p.Python.Venv = 0 },
		"callable dollar":  func(p *UnitParams) { p.Python.Settings.Callable = "a:$HOME" },
		"callable call":    func(p *UnitParams) { p.Python.Settings.Callable = "a:b()" },
		"too many workers": func(p *UnitParams) { p.Python.Settings.Workers = 7 },
		"server":           func(p *UnitParams) { p.Python.Settings.Server = "sh" },
	}
	for name, edit := range cases {
		p := pythonParams(t)
		edit(&p)
		if _, err := RenderUnit(p); err == nil {
			t.Errorf("%s: rendered", name)
		}
	}
}

func buildParams(t *testing.T) BuildParams {
	return BuildParams{AppID: 12, Domain: "shop.example.com", User: "shop_example_com", Group: "shop_example_com", Layout: testLayout(t),
		Minor: "3.13", Venv: 4, Limits: DefaultLimits(KindPython), Level: sitesandbox.LevelFull, Caps: fullCaps(257), HasRequirements: true}
}

func TestBuildUnitIsSandboxedPipFromPyPIWithoutAShell(t *testing.T) {
	root := testLayout(t).Root
	args, err := BuildRunArgs(buildParams(t))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(args, "\n")
	for _, want := range []string{
		"--unit=celikrtbuild-12.service", "--uid=shop_example_com", "--gid=shop_example_com", "--property=Type=oneshot",
		"--setenv=PIP_CONFIG_FILE=/dev/null", "--setenv=TMPDIR=" + root + "/.build-tmp", "--setenv=HOME=" + root + "/data",
		"--property=ExecStartPre=/usr/bin/python3.13 -m venv " + root + "/venv-4.partial",
		"--property=ExecStartPre=" + root + "/venv-4.partial/bin/python -m pip install --isolated --no-input --disable-pip-version-check --no-warn-script-location --progress-bar=off --index-url=https://pypi.org/simple --cache-dir=" + root + "/.pip-cache --only-binary=:all: --require-hashes --no-deps -r /run/celikpanel-rtbuild/12/server.txt",
		"--property=ExecStartPre=" + root + "/venv-4.partial/bin/python -m pip install --isolated --no-input --disable-pip-version-check --no-warn-script-location --progress-bar=off --index-url=https://pypi.org/simple --cache-dir=" + root + "/.pip-cache --only-binary=:all: -r /run/celikpanel-rtbuild/12/requirements.txt",
		"--property=ProtectSystem=strict", "--property=SocketBindDeny=any",
		// Sandbox generation 3: the app folder comes from the bind unit's
		// root-owned mount point; the only root commands are the static
		// helper's egress calls.
		"--property=BindPaths=/run/celikpanel-sandbox/celikrtbuild-12.service/app:" + root,
		"--property=Requires=celikpanel-sandbox@celikrtbuild-12.service", "--property=After=celikpanel-sandbox@celikrtbuild-12.service",
		"--property=ExecStartPre=+/usr/lib/celikpanel-sandbox/sandbox-helper egress-add --unit=celikrtbuild-12.service",
		"--property=ExecStopPost=-+/usr/lib/celikpanel-sandbox/sandbox-helper egress-remove --unit=celikrtbuild-12.service",
	} {
		if !containsLine(args, want) {
			t.Fatalf("missing %q\n%s", want, text)
		}
	}
	// The SMTP block is in place before pip runs any install script.
	egressAt, venvAt := -1, -1
	for i, a := range args {
		if strings.Contains(a, "sandbox-helper egress-add") && egressAt < 0 {
			egressAt = i
		}
		if strings.Contains(a, " -m venv ") && venvAt < 0 {
			venvAt = i
		}
	}
	if egressAt < 0 || venvAt < 0 || egressAt > venvAt {
		t.Fatalf("egress-add at %d, venv at %d", egressAt, venvAt)
	}
	tail := strings.Join(args[len(args)-6:], " ")
	if !strings.HasSuffix(tail, "-- "+root+"/venv-4.partial/bin/python -m pip check --disable-pip-version-check") {
		t.Fatalf("ExecStart %q", tail)
	}
	for _, banned := range []string{"/bin/sh", "sh -c", "$", "SocketBindAllow", "MemoryDenyWriteExecute", "NoExecPaths", "extra-index", "trusted-host"} {
		if strings.Contains(text, banned) {
			t.Fatalf("%q in the build", banned)
		}
	}
	// Hashes, constraints and source builds.
	p := buildParams(t)
	p.Hashed, p.HasConstraints, p.SourceBuilds = true, true, true
	args, _ = BuildRunArgs(p)
	text = strings.Join(args, "\n")
	own := ""
	for _, a := range args {
		if strings.Contains(a, "requirements.txt") {
			own = a
		}
	}
	if !strings.Contains(own, "--require-hashes -r /run/celikpanel-rtbuild/12/requirements.txt -c /run/celikpanel-rtbuild/12/constraints.txt") || strings.Contains(own, "--only-binary") {
		t.Fatalf("source-build line %q", own)
	}
	if !strings.Contains(text, "--only-binary=:all: --require-hashes --no-deps -r /run/celikpanel-rtbuild/12/server.txt") {
		t.Fatal("the pins are wheels whatever the offering allows")
	}
	// No requirements: only the pins and the check.
	p = buildParams(t)
	p.HasRequirements = false
	if steps, _ := BuildSteps(p); len(steps) != 3 {
		t.Fatalf("steps %v", steps)
	}
	for name, edit := range map[string]func(*BuildParams){
		"unpinned minor": func(p *BuildParams) { p.Minor = "3.15" },
		"no generation":  func(p *BuildParams) { p.Venv = 0 },
		"root":           func(p *BuildParams) { p.User = "root" },
		"reduced host":   func(p *BuildParams) { p.Caps = sitesandbox.Caps{SystemdVersion: 249, CgroupV2: true} },
		"domain":         func(p *BuildParams) { p.Domain = "a$b.com" },
	} {
		p := buildParams(t)
		edit(&p)
		if _, err := BuildRunArgs(p); err == nil {
			t.Errorf("%s: built", name)
		}
	}
	if !sitesandbox.ValidUnitName(BuildUnitName(12)) {
		t.Fatal("the build unit is not a sandboxed unit name")
	}
}

func containsLine(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestVenvGenerationsSwapAndRollBack(t *testing.T) {
	// First deploy: nothing on disk.
	s := NewVenvSet([]string{"src", "data", "bin"}, 0)
	if n, _ := s.Next(); n != 1 || len(s.Stale()) != 0 {
		t.Fatalf("first %d", n)
	}
	if _, ok := RollbackTarget(0, 1, NewVenvSet([]string{"venv-1"}, 0)); ok {
		t.Fatal("a first deploy has nothing to go back to")
	}
	// The app runs venv-2; venv-1 and a leftover partial are there.
	s = NewVenvSet([]string{"venv-1", "venv-2", "venv-3.partial", "venv-x", "venv-01", "venv-1234567", "src"}, 2)
	n, _ := s.Next()
	if n != 4 || strings.Join(s.Stale(), ",") != "venv-3.partial" {
		t.Fatalf("next %d stale %v", n, s.Stale())
	}
	// The build of venv-4 succeeded and was renamed: venv-4 and the
	// running venv-2 stay; venv-1 and the leftover go.
	after := NewVenvSet([]string{"venv-1", "venv-2", "venv-3.partial", "venv-4"}, 2)
	if got := strings.Join(after.AfterBuild(4), ","); got != "venv-1,venv-3.partial" {
		t.Fatalf("after build %s", got)
	}
	kept := NewVenvSet([]string{"venv-2", "venv-4"}, 4)
	if back, ok := RollbackTarget(2, 4, kept); !ok || back != 2 {
		t.Fatal("one step back to venv-2")
	}
	if _, ok := RollbackTarget(2, 4, NewVenvSet([]string{"venv-4"}, 4)); ok {
		t.Fatal("a removed generation is not a target")
	}
	// The unit names a generation the folder no longer has: the next build
	// still never reuses its number.
	if n, _ := NewVenvSet(nil, 9).Next(); n != 10 {
		t.Fatalf("%d", n)
	}
	if _, err := NewVenvSet([]string{"venv-999999"}, 0).Next(); err == nil {
		t.Fatal("generation overflow")
	}
	for _, c := range []struct {
		found          bool
		active, result string
		phase          string
		lost           bool
	}{
		{true, "activating", "", BuildPhaseRunning, false},
		{true, "active", "success", BuildPhaseSucceeded, false},
		{true, "failed", "exit-code", BuildPhaseFailed, false},
		{true, "inactive", "success", BuildPhaseFailed, true},
		{false, "", "", BuildPhaseFailed, true},
		{true, "failed", "timeout", BuildPhaseFailed, false},
	} {
		phase, lost := BuildOutcome(c.found, c.active, c.result)
		if phase != c.phase || lost != c.lost {
			t.Errorf("%+v: %s %v", c, phase, lost)
		}
	}
	if ActiveVenv("ExecStart=/x/app/venv-12.partial/bin/python -m gunicorn\n") != 0 || ActiveVenv("ExecStart=/x/app/venv-12/bin/python -m gunicorn a:b\n") != 12 {
		t.Fatal("active venv")
	}
}

func TestBuildFailureNamesThePackageAndHidesSecrets(t *testing.T) {
	lines := []string{
		"Collecting numpy==1.21.0",
		"ERROR: Could not find a version that satisfies the requirement numpy==1.21.0 (from versions: 1.26.4, 2.0.0)",
		"ERROR: No matching distribution found for numpy==1.21.0",
	}
	if reason, pkg := ClassifyBuildFailure(lines); reason != WheelUnavailable || pkg != "numpy" {
		t.Fatalf("%s %s", reason, pkg)
	}
	if reason, _ := ClassifyBuildFailure([]string{"WARNING: Retrying ... Failed to establish a new connection: [Errno -3] Temporary failure in name resolution"}); reason != PyPIUnreachable {
		t.Fatal(reason)
	}
	if reason, _ := ClassifyBuildFailure([]string{"ERROR: THESE PACKAGES DO NOT MATCH THE HASHES FROM THE REQUIREMENTS FILE."}); reason != BuildFailed {
		t.Fatal(reason)
	}
	got := strings.Join(BuildTail([]string{"Looking in https://bob:hunter2@pypi.example.com/simple", "DB_PASSWORD=topsecret value", "", "done"}, 10, 1000), "\n")
	if strings.Contains(got, "hunter2") || strings.Contains(got, "topsecret") || !strings.Contains(got, "done") {
		t.Fatalf("%s", got)
	}
	if tail := BuildTail([]string{"a", "b", "c"}, 2, 100); strings.Join(tail, ",") != "b,c" {
		t.Fatalf("%v", tail)
	}
	if RiskDigestFor(KindPython, true) == RiskDigestFor(KindPython, false) || RiskDigestFor(KindProgram, true) != RiskDigest(KindProgram) {
		t.Fatal("digests")
	}
	if containsLine(RiskOf(KindPython).Blocked, "system_interpreters") || !containsLine(RiskOfOffer(KindPython, true).Remains, "source_builds") {
		t.Fatal("python risk")
	}
}
