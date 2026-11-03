package serviceapps

import (
	"strings"
	"testing"
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
		"BindPaths=/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n\n",
		"ReadWritePaths=/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n\n",
		"MemoryMax=1536M\n", "CPUQuota=100%\n", "TasksMax=512\n",
		"EnvironmentFile=/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n/config/app.env\n",
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
