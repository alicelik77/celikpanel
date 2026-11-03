package serviceapps

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

func n8nParams(t *testing.T) UnitParams {
	t.Helper()
	app, _ := Lookup("n8n")
	layout, err := LayoutFor(3, 7, "n8n")
	if err != nil {
		t.Fatal(err)
	}
	return UnitParams{
		InstallID: 12, App: app, Version: app.Version, Domain: "flows.example.com",
		User: "flows_example_com", Group: "flows_example_com", Layout: layout,
		Port: 21010, AuxPort: 21011,
		Runtime:   RuntimeChoice{Available: true, Path: "/opt/celikpanel/runtimes/node/24.11.1/bin/node", NPMCLI: "/opt/celikpanel/runtimes/node/24.11.1/lib/node_modules/npm/bin/npm-cli.js", Version: "24.11.1", Source: "panel"},
		Limits:    app.Defaults,
		Caps:      sitesandbox.Caps{SystemdVersion: 252, BPFFramework: true, CgroupV2: true, NFTSocketCgroup: true},
		DropInDir: "/etc/systemd/system/celiksvc-12.service.d",
	}
}

func TestUnitCarriesTheSandbox(t *testing.T) {
	text, err := RenderUnit(n8nParams(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"User=flows_example_com\n", "NoNewPrivileges=yes\n", "ProtectSystem=strict\n", "ProtectHome=yes\n",
		"PrivateTmp=yes\n", "CapabilityBoundingSet=\n", "ProtectProc=invisible\n",
		"TemporaryFileSystem=/var/www/celikpanel:ro\n",
		"BindPaths=/run/celikpanel-sandbox/celiksvc-12.service/app:/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n\n",
		"WorkingDirectory=/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n\n",
		"MemoryMax=1536M\n", "CPUQuota=100%\n", "TasksMax=512\n",
		"EnvironmentFile=/run/celikpanel-sandbox/celiksvc-12.service/app.env\n",
		"ExecStart=/opt/celikpanel/runtimes/node/24.11.1/bin/node /var/www/celikpanel/subscriptions/3/sites/7/apps/n8n/app-2.46.4/node_modules/n8n/bin/n8n start\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("unit lacks %q", want)
		}
	}
	for _, refused := range []string{"User=root", "docker", "AmbientCapabilities=CAP", "0.0.0.0"} {
		if strings.Contains(text, refused) {
			t.Errorf("unit contains %q", refused)
		}
	}
	if UnitTextState(text) != "ours" {
		t.Fatalf("a rendered unit must read as CelikPanel's own")
	}
	if UnitTextState(strings.Replace(text, "MemoryMax=1536M", "MemoryMax=8G", 1)) != "edited" {
		t.Fatalf("an owner's edit must be detected")
	}
	if UnitTextState("[Unit]\nDescription=mine\n") != "foreign" {
		t.Fatalf("a unit without the header is not CelikPanel's")
	}
}

func TestUnitRefusesUnsafeInputs(t *testing.T) {
	cases := map[string]func(*UnitParams){
		"root user":         func(p *UnitParams) { p.User = "root;x" },
		"outside home":      func(p *UnitParams) { p.Layout = Layout{Root: "/etc"} },
		"privileged port":   func(p *UnitParams) { p.Port = 80 },
		"no runtime":        func(p *UnitParams) { p.Runtime = RuntimeChoice{} },
		"deferred app":      func(p *UnitParams) { p.App, _ = Lookup("openclaw") },
		"newline in domain": func(p *UnitParams) { p.Domain = "a.com\nExecStartPre=/bin/sh" },
	}
	for name, mutate := range cases {
		p := n8nParams(t)
		mutate(&p)
		if _, err := RenderUnit(p); err == nil {
			t.Errorf("%s: rendered", name)
		}
	}
}

func TestEnvBindsLoopbackAndKeepsFixedValues(t *testing.T) {
	app, _ := Lookup("n8n")
	layout, _ := LayoutFor(3, 7, "n8n")
	text, err := RenderEnv(EnvParams{
		App: app, Domain: "flows.example.com", Layout: layout, Port: 21010, AuxPort: 21011,
		OwnerEmail: "me@example.com", OwnerSecret: "$2a$10$abcdefghijklmnopqrstuuJ0123456789abcdefghijklmnopqrstu",
		Generated: map[string]string{"N8N_ENCRYPTION_KEY": strings.Repeat("ab", 32)},
	})
	if err != nil {
		t.Fatal(err)
	}
	env := ParseEnv(text)
	if env["N8N_LISTEN_ADDRESS"] != "127.0.0.1" || env["N8N_RUNNERS_BROKER_LISTEN_ADDRESS"] != "127.0.0.1" {
		t.Fatalf("n8n must listen on loopback only: %v", env)
	}
	if !strings.Contains(env["NODES_EXCLUDE"], "executeCommand") || env["N8N_COMMUNITY_PACKAGES_ENABLED"] != "false" || env["N8N_BLOCK_ENV_ACCESS_IN_NODE"] != "true" {
		t.Fatalf("host-command features must be disabled: %v", env)
	}
	if env["N8N_INSTANCE_OWNER_PASSWORD_HASH"] != "$2a$10$abcdefghijklmnopqrstuuJ0123456789abcdefghijklmnopqrstu" {
		t.Fatalf("bcrypt hash must survive quoting: %q", env["N8N_INSTANCE_OWNER_PASSWORD_HASH"])
	}

	ow, _ := Lookup("open-webui")
	owLayout, _ := LayoutFor(3, 8, "open-webui")
	// A customer value can neither reopen a feature nor be a key it does not own.
	if _, err := RenderEnv(EnvParams{App: ow, Domain: "chat.example.com", Layout: owLayout, Port: 21020,
		Generated: map[string]string{"WEBUI_SECRET_KEY": strings.Repeat("cd", 32)},
		Secrets:   map[string]string{"ENABLE_CODE_EXECUTION": "True"}}); err == nil {
		t.Fatalf("a fixed key must not be accepted as a customer secret")
	}
	text, err = RenderEnv(EnvParams{App: ow, Domain: "chat.example.com", Layout: owLayout, Port: 21020,
		Generated: map[string]string{"WEBUI_SECRET_KEY": strings.Repeat("cd", 32)},
		Secrets:   map[string]string{"OPENAI_API_KEY": "sk-test-\"quoted\"-value"}})
	if err != nil {
		t.Fatal(err)
	}
	env = ParseEnv(text)
	if env["OPENAI_API_KEY"] != "sk-test-\"quoted\"-value" || env["ENABLE_CODE_EXECUTION"] != "False" || env["SAFE_MODE"] != "True" {
		t.Fatalf("open webui env: %v", env)
	}
	if _, err := RenderEnv(EnvParams{App: ow, Domain: "chat.example.com", Layout: owLayout, Port: 21020,
		Generated: map[string]string{"WEBUI_SECRET_KEY": strings.Repeat("cd", 32)},
		Secrets:   map[string]string{"OPENAI_API_KEY": "a\nENABLE_CODE_EXECUTION=True"}}); err == nil {
		t.Fatalf("a newline must not reach the environment file")
	}
	params := n8nParams(t)
	params.App, params.Layout, params.Version = ow, owLayout, ow.Version
	params.Runtime = RuntimeChoice{Available: true, Path: "/usr/bin/python3.12", Version: "3.12", Source: "distribution"}
	unit, err := RenderUnit(params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unit, "serve --host 127.0.0.1 --port 21010\n") {
		t.Fatalf("open webui must be started on loopback:\n%s", unit)
	}
}

func TestRedactHidesSecrets(t *testing.T) {
	secrets := []string{"sk-proj-ABCDEFGHIJKLMNOPQRSTUVWX", "0123456789abcdef0123456789abcdef"}
	lines := map[string]string{
		"calling with key sk-proj-ABCDEFGHIJKLMNOPQRSTUVWX now": "calling with key [hidden] now",
		"encryption 0123456789abcdef0123456789abcdef loaded":    "encryption [hidden] loaded",
		"OPENAI_API_KEY=sk-other-zzzzzzzzzzzzzzzzzzzz":          "OPENAI_API_KEY=[hidden]",
		`{"password": "hunter2hunter2"}`:                        `{"password": "[hidden]"}`,
		"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.e30.abc":    "Authorization: [hidden]",
		"db postgres://user:topsecret@127.0.0.1/db":             "db postgres://user:[hidden]@127.0.0.1/db",
		"plain line stays": "plain line stays",
	}
	for in, want := range lines {
		got := Redact(in, secrets)
		if strings.Contains(got, "sk-proj") || strings.Contains(got, "hunter2") || strings.Contains(got, "topsecret") || strings.Contains(got, "0123456789abcdef0123") || strings.Contains(got, "eyJhbGci") {
			t.Errorf("secret left in %q -> %q", in, got)
		}
		if in == "plain line stays" && got != want {
			t.Errorf("plain line changed: %q", got)
		}
	}
}

func TestRuntimeChoiceRefusesUnsupportedSystems(t *testing.T) {
	n8n, _ := Lookup("n8n")
	// Debian 12's Node.js 18 and Debian 13's 20: n8n needs 24, which the
	// administrator installs as a Panel runtime; no extra package source.
	choice := ChooseRuntime(n8n, []NodeCandidate{{Path: "/usr/bin/node", Version: "20.19.2", NPMCLI: "/usr/share/nodejs/npm/bin/npm-cli.js", Source: "distribution"}}, nil)
	if choice.Available || choice.Reason != ReasonRuntimeAbsent || !strings.Contains(choice.Detail, "20.19.2") {
		t.Fatalf("old node: %+v", choice)
	}
	choice = ChooseRuntime(n8n, []NodeCandidate{
		{Path: "/usr/bin/node", Version: "24.20.0", NPMCLI: "/usr/lib/node_modules/npm/bin/npm-cli.js", Source: "distribution"},
		{Path: "/opt/celikpanel/runtimes/node/24.11.1/bin/node", Version: "24.11.1", NPMCLI: "/opt/x/npm-cli.js", Source: "panel"},
	}, nil)
	if !choice.Available || choice.Source != "panel" {
		t.Fatalf("the panel runtime is preferred: %+v", choice)
	}

	ow, _ := Lookup("open-webui")
	for _, version := range []string{"3.13", "3.10", "3.14"} {
		choice = ChooseRuntime(ow, nil, []PythonCandidate{{Path: "/usr/bin/python3", Version: version, Venv: true}})
		if choice.Available || choice.Reason != ReasonOSUnsupported {
			t.Fatalf("python %s must be refused: %+v", version, choice)
		}
	}
	choice = ChooseRuntime(ow, nil, []PythonCandidate{{Path: "/usr/bin/python3", Version: "3.11", Venv: false}})
	if choice.Reason != ReasonVenvMissing {
		t.Fatalf("missing venv is a prerequisite: %+v", choice)
	}
	choice = ChooseRuntime(ow, nil, []PythonCandidate{{Path: "/usr/bin/python3", Version: "3.12", Venv: true}})
	if !choice.Available {
		t.Fatalf("python 3.12: %+v", choice)
	}
	claw, _ := Lookup("openclaw")
	if ChooseRuntime(claw, nil, nil).Reason != ReasonDeferred || claw.Installable() {
		t.Fatalf("command-running agents are deferred in this release")
	}
}

func TestListenProof(t *testing.T) {
	out := `LISTEN 0 511 127.0.0.1:21010 0.0.0.0:* users:(("node",pid=400,fd=21))
LISTEN 0 511 127.0.0.1:21011 0.0.0.0:* users:(("node",pid=400,fd=22))
LISTEN 0 4096 0.0.0.0:22 0.0.0.0:* users:(("sshd",pid=1,fd=3))`
	proof := ProveListening(ParseSS(out), map[int]bool{400: true}, 21010)
	if !proof.Serving || len(proof.Exposed) != 0 || proof.Squatted {
		t.Fatalf("loopback-only app: %+v", proof)
	}
	exposed := out + "\nLISTEN 0 511 [::]:21012 [::]:* users:((\"node\",pid=401,fd=9))"
	proof = ProveListening(ParseSS(exposed), map[int]bool{400: true, 401: true}, 21010)
	if len(proof.Exposed) != 1 {
		t.Fatalf("a wildcard listener of the unit must be reported: %+v", proof)
	}
	proof = ProveListening(ParseSS(out), map[int]bool{999: true}, 21010)
	if proof.Serving || !proof.Squatted {
		t.Fatalf("another process on the port is squatting: %+v", proof)
	}
}

func TestInstallScriptPinsTheArtifact(t *testing.T) {
	for _, id := range []string{"n8n", "open-webui"} {
		app, _ := Lookup(id)
		algorithm, digest, err := app.ArtifactHex()
		if err != nil || len(digest) == 0 {
			t.Fatalf("%s pin: %v", id, err)
		}
		if (app.Runtime == RuntimeNode) != (algorithm == "sha512") {
			t.Fatalf("%s algorithm %s", id, algorithm)
		}
		script := InstallScript(app)
		if !strings.Contains(script, `"${algo}sum" -c`) {
			t.Fatalf("%s script does not re-check the pin", id)
		}
		if strings.Contains(script, "curl") || strings.Contains(script, "sudo") {
			t.Fatalf("%s script fetches or escalates", id)
		}
	}
	app, _ := Lookup("n8n")
	if !strings.Contains(InstallScript(app), "audit signatures") {
		t.Fatalf("npm installs check registry signatures")
	}
}

// TestNPMInstallCommandLine: n8n failed with EBADPLATFORM (alpha.118 lab
// round 2) because n8n-nodes-base 2.46.x lists Alpine (musl) binaries as
// ordinary dependencies; npm refuses those on glibc unless --force. The
// command stays npm's standard install of the pinned tarball otherwise: no
// platform overrides, no lifecycle-script or registry changes, and the
// registry signatures are checked after it.
func TestNPMInstallCommandLine(t *testing.T) {
	app, _ := Lookup("n8n")
	if !app.NPMPlatformDepsRequired {
		t.Fatal("n8n 2.46.x requires other platforms' binaries")
	}
	script := InstallScript(app)
	want := `"$runtime" "$npmcli" install --prefix "$target.partial" --omit=dev --no-save --force "$root/dist/$file"` + "\n"
	if !strings.Contains(script, want) {
		t.Fatalf("n8n install line missing:\n%s", script)
	}
	for _, forbidden := range []string{"--os", "--cpu", "--libc", "--ignore-scripts", "--registry", "--legacy-peer-deps", "--no-package-lock", "--omit=optional", "--include="} {
		if strings.Contains(script, forbidden) {
			t.Errorf("n8n install uses %s", forbidden)
		}
	}
	if !strings.Contains(script, `"$runtime" "$npmcli" audit signatures --prefix "$target.partial"`) {
		t.Fatal("registry signatures are no longer checked")
	}
	// An app without the defect keeps the plain install.
	plain := app
	plain.NPMPlatformDepsRequired = false
	if strings.Contains(InstallScript(plain), "--force") {
		t.Fatal("--force only for an app that needs it")
	}
	if !strings.Contains(InstallScript(plain), `--omit=dev --no-save "$root/dist/$file"`) {
		t.Fatal("plain npm install line changed")
	}
}

// systemdExpand follows systemd's replace_env() for one command argument:
// "$$" is one "$", "${NAME}" is the variable (empty when unset), and a whole
// argument "$NAME" is the variable too. Other "$" stay literal.
func systemdExpand(arg string, env map[string]string) string {
	if len(arg) > 1 && arg[0] == '$' && arg[1] != '$' && arg[1] != '{' {
		name := arg[1:]
		valid := true
		for _, r := range name {
			if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
				valid = false
			}
		}
		if valid {
			return env[name]
		}
	}
	var b strings.Builder
	for i := 0; i < len(arg); i++ {
		if arg[i] != '$' || i+1 >= len(arg) {
			b.WriteByte(arg[i])
			continue
		}
		switch arg[i+1] {
		case '$':
			b.WriteByte('$')
			i++
		case '{':
			end := strings.IndexByte(arg[i:], '}')
			if end < 0 {
				b.WriteByte(arg[i])
				continue
			}
			b.WriteString(env[arg[i+2:i+end]])
			i += end
		default:
			b.WriteByte(arg[i])
		}
	}
	return b.String()
}

// TestInstallCommandSurvivesSystemdExpansion: the n8n install unit failed
// with "sum: invalid option -- 'c'" because systemd expanded "${algo}sum"
// (alpha.118 lab). After systemd's expansion every argument must be exactly
// the shell text and arguments CelikPanel meant; no bare "$" and no "%".
func TestInstallCommandSurvivesSystemdExpansion(t *testing.T) {
	for _, id := range []string{"n8n", "open-webui"} {
		app, _ := Lookup(id)
		layout, err := LayoutFor(3, 7, id)
		if err != nil {
			t.Fatal(err)
		}
		runtime := RuntimeChoice{Available: true, Path: "/usr/bin/python3", NPMCLI: "/opt/npm/bin/npm-cli.js"}
		args, err := InstallArgs(app, layout, app.Version, runtime)
		if err != nil {
			t.Fatal(err)
		}
		command, err := InstallCommand(app, args)
		if err != nil {
			t.Fatal(err)
		}
		want := append([]string{"/bin/sh", "-c", InstallScript(app), "sh"}, args...)
		if len(command) != len(want) {
			t.Fatalf("%s: %d arguments, want %d", id, len(command), len(want))
		}
		env := map[string]string{"algo": "", "root": "/wrong", "HOME": "/wrong"}
		for i := range command {
			if strings.ContainsRune(command[i], '%') {
				t.Errorf("%s argument %d has a specifier character", id, i)
			}
			if strings.Contains(strings.ReplaceAll(command[i], "$$", ""), "$") {
				t.Errorf("%s argument %d has a bare $: %q", id, i, command[i])
			}
			if got := systemdExpand(command[i], env); got != want[i] {
				t.Errorf("%s argument %d after systemd expansion:\n%s\nwant\n%s", id, i, got, want[i])
			}
		}
		if !strings.Contains(InstallScript(app), `"${algo}sum" -c --status -`) {
			t.Fatalf("%s: the checksum step is missing", id)
		}
	}
	if _, err := SystemdExecArg("50%"); err == nil {
		t.Fatal("a specifier character must be refused")
	}
}

// TestRenderedUnitHasNoExpansionSurprises: the long-running unit's text has
// no "$" at all and "%" only as CPUQuota's own percent sign.
func TestRenderedUnitHasNoExpansionSurprises(t *testing.T) {
	text, err := RenderUnit(n8nParams(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "$") {
			t.Errorf("unit line has a $: %q", line)
		}
		if strings.Contains(line, "%") && !(strings.HasPrefix(line, "CPUQuota=") && strings.Count(line, "%") == 1 && strings.HasSuffix(line, "%")) {
			t.Errorf("unit line has a specifier: %q", line)
		}
	}
}

// TestUnitClosesTheAlpha118Gaps is the rendering difference of the retrofit:
// the service-app unit now renders through sitesandbox and gains the hidden
// sockets, the metadata block, the bind rules, the log limit and the SMTP
// block; an alpha.118 unit (no marker) reads as generation 1.
func TestUnitClosesTheAlpha118Gaps(t *testing.T) {
	text, err := RenderUnit(n8nParams(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# celikpanel-sandbox-generation: 3\n",
		"StartLimitBurst=5\n",
		"InaccessiblePaths=-/run/celikpanel -/run/php -/run/php-fpm ",
		"-/var/spool/postfix\n",
		"IPAddressDeny=169.254.0.0/16 fe80::/10",
		"SocketBindAllow=tcp:21010\n", "SocketBindAllow=tcp:21011\n", "SocketBindDeny=any\n",
		"LogRateLimitIntervalSec=30s\n", "LogRateLimitBurst=1000\n",
		"SystemCallFilter=~@debug @mount @raw-io",
		"PrivateIPC=yes\n",
		"ExecStartPre=+/usr/lib/celikpanel-sandbox/sandbox-helper egress-add --unit=celiksvc-12.service\n",
		"ExecStopPost=-+/usr/lib/celikpanel-sandbox/sandbox-helper egress-remove --unit=celiksvc-12.service\n",
		"Requires=celikpanel-sandbox@celiksvc-12.service\n", "After=celikpanel-sandbox@celiksvc-12.service\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("unit lacks %q", want)
		}
	}
	unitSection, _, _ := strings.Cut(text, "[Service]")
	if !strings.Contains(unitSection, "StartLimitIntervalSec=300") {
		t.Errorf("StartLimit must be in [Unit]")
	}
	// Node.js reads /proc/cpuinfo; the service apps keep the full /proc view
	// and are not an uploaded program (no MemoryDenyWriteExecute for V8).
	for _, refused := range []string{"ProcSubset=", "MemoryDenyWriteExecute=", "NoExecPaths=", "PrivatePIDs="} {
		if strings.Contains(text, refused) {
			t.Errorf("service-app unit has %q", refused)
		}
	}
	if UnitGeneration(text) != sitesandbox.Generation {
		t.Fatalf("generation = %d", UnitGeneration(text))
	}
	if UnitGeneration("# celikpanel-render-sha256: x\n[Unit]\n") != 1 {
		t.Fatalf("an alpha.118 unit is generation 1")
	}

	// A host without nft cgroup matching gets no egress lines (they would
	// stop every start); its reduced level names the gap instead.
	p := n8nParams(t)
	p.Caps.NFTSocketCgroup = false
	text, err = RenderUnit(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "egress-add") || !strings.Contains(text, "Requires=celikpanel-sandbox@celiksvc-12.service") {
		t.Fatalf("egress asked for without nft support, or no bind unit:\n%s", text)
	}
	// No privileged command of the unit is a dynamically linked program:
	// every command gets the unit's environment, which is the customer's.
	for _, line := range strings.Split(text, "\n") {
		for _, prefix := range []string{"ExecStartPre=", "ExecStartPost=", "ExecStop=", "ExecStopPost=", "ExecReload=", "ExecCondition="} {
			if rest, ok := strings.CutPrefix(line, prefix); ok && strings.Contains(strings.TrimLeft(rest, "-@:!"), "+") &&
				!strings.HasPrefix(strings.TrimLeft(rest, "-@:!+"), sitesandbox.HelperPath+" ") {
				t.Errorf("a privileged command other than the helper: %q", line)
			}
		}
	}
}

// TestNoSitePathOnTheHost: the paths systemd resolves as root on the host
// (bind sources, the environment file) never name the site home.
func TestNoSitePathOnTheHost(t *testing.T) {
	text, err := RenderUnit(n8nParams(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(text, "\n") {
		name, value, _ := strings.Cut(line, "=")
		switch name {
		case "BindPaths", "BindReadOnlyPaths":
			for _, field := range strings.Fields(value) {
				if source, _, _ := strings.Cut(strings.TrimPrefix(field, "-"), ":"); strings.HasPrefix(source, "/var/www/") {
					t.Errorf("bind source in the site home: %q", line)
				}
			}
		case "EnvironmentFile", "ReadWritePaths", "RootDirectory":
			if strings.Contains(value, "/var/www/") {
				t.Errorf("%s names the site home: %q", name, line)
			}
		}
	}
}

func TestInstallUnitPropertiesBlockSMTPAndBindNothing(t *testing.T) {
	app, _ := Lookup("n8n")
	layout, _ := LayoutFor(3, 7, "n8n")
	caps := sitesandbox.Caps{SystemdVersion: 255, BPFFramework: true, CgroupV2: true, NFTSocketCgroup: true}
	props, err := InstallUnitProperties(12, layout, "flows_example_com", "flows_example_com", app.Defaults, app.Defaults.InstallMemoryMB, caps)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(props, "\n")
	for _, want := range []string{"SocketBindDeny=any", "MemoryMax=3072M",
		"ExecStartPre=+/usr/lib/celikpanel-sandbox/sandbox-helper egress-add --unit=celiksvcinst-12.service",
		"ExecStopPost=-+/usr/lib/celikpanel-sandbox/sandbox-helper egress-remove --unit=celiksvcinst-12.service",
		"Requires=celikpanel-sandbox@celiksvcinst-12.service", "After=celikpanel-sandbox@celiksvcinst-12.service",
		"BindPaths=/run/celikpanel-sandbox/celiksvcinst-12.service/app:/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n"} {
		if !strings.Contains(joined, want) {
			t.Errorf("install properties lack %q", want)
		}
	}
	if strings.Contains(joined, "SocketBindAllow") || strings.Contains(joined, "StartLimit") || strings.Contains(joined, "EnvironmentFile") {
		t.Errorf("the install unit binds nothing and has no [Unit] lines:\n%s", joined)
	}
	if _, err := InstallUnitProperties(12, layout, "root", "root", app.Defaults, 3072, caps); err == nil {
		t.Fatal("root accepted")
	}
}
