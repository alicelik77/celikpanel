package nodesite

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

const docRoot = "/var/www/celikpanel/subscriptions/3/sites/7/public_html"

var systems = []struct {
	name string
	caps sitesandbox.Caps
}{
	{"ubuntu-22.04", sitesandbox.Caps{SystemdVersion: 249, CgroupV2: true, NFTSocketCgroup: true}},
	{"debian-12", sitesandbox.Caps{SystemdVersion: 252, CgroupV2: true, NFTSocketCgroup: true}},
	{"ubuntu-24.04", sitesandbox.Caps{SystemdVersion: 255, CgroupV2: true, NFTSocketCgroup: true}},
	{"debian-13", sitesandbox.Caps{SystemdVersion: 257, BPFFramework: true, CgroupV2: true, NFTSocketCgroup: true}},
	{"arch", sitesandbox.Caps{SystemdVersion: 262, BPFFramework: true, CgroupV2: true, NFTSocketCgroup: true}},
	// A host without the nft egress table: no helper egress lines.
	{"no-nft", sitesandbox.Caps{SystemdVersion: 257, BPFFramework: true, CgroupV2: true}},
}

func params(caps sitesandbox.Caps) Params {
	return Params{
		SiteID: 12, Domain: "shop.example.com", User: "shop_example_com", Group: "shop_example_com", AppDir: docRoot,
		Port: 3000, NodeVersion: "22.11.0", Command: "npm start", Caps: caps,
		DropInDir: "/etc/systemd/system/celikapp-12.service.d",
	}
}

// systemdWord is how systemd reads one ExecStart word of a unit file: "%%"
// is "%" (specifiers are resolved on the whole line first), the quotes are
// removed and "\\" and "\"" unescaped, then "$$" is "$" when the command
// runs. It covers what ExecStartArg writes.
func systemdWord(t *testing.T, word string) string {
	t.Helper()
	word = strings.ReplaceAll(word, "%%", "%")
	if !strings.HasPrefix(word, `"`) || !strings.HasSuffix(word, `"`) || len(word) < 2 {
		t.Fatalf("not one quoted word: %s", word)
	}
	inner := word[1 : len(word)-1]
	var b strings.Builder
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c == '\\' {
			i++
			if i == len(inner) || (inner[i] != '\\' && inner[i] != '"') {
				t.Fatalf("unexpected escape in %s", word)
			}
			b.WriteByte(inner[i])
			continue
		}
		if c == '"' {
			t.Fatalf("an unescaped quote ends the word early: %s", word)
		}
		b.WriteByte(c)
	}
	return strings.ReplaceAll(b.String(), "$$", "$")
}

func TestExecStartArgRoundTrip(t *testing.T) {
	for _, command := range []string{
		"npm start",
		"node server.js --port $PORT",
		`node -e "console.log('100%')"`,
		`npm run serve -- --dir 'C:\path' && echo "done"`,
		"PORT=${PORT} node dist/index.js; echo $$",
		"node app.js # comment with ü and 🚀",
		`echo \"`,
	} {
		word, err := ExecStartArg(command)
		if err != nil {
			t.Fatalf("%q: %v", command, err)
		}
		if got := systemdWord(t, word); got != command {
			t.Fatalf("round trip of %q gave %q", command, got)
		}
		if strings.Contains(strings.ReplaceAll(word, "%%", ""), "%") || strings.Contains(strings.ReplaceAll(word, "$$", ""), "$") {
			t.Fatalf("an unescaped specifier or variable in %s", word)
		}
	}
	for _, bad := range []string{"", "   ", "npm start\nExecStartPre=+/bin/sh", "a\rb", "a\x00b", "a\tb", strings.Repeat("x", MaxCommand+1)} {
		if _, err := ExecStartArg(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestRenderUnitOnEverySystem(t *testing.T) {
	for _, sys := range systems {
		text, err := RenderUnit(params(sys.caps))
		if err != nil {
			t.Fatalf("%s: %v", sys.name, err)
		}
		for _, want := range []string{
			"# celikpanel-sandbox-generation: 3\n",
			Marker + "22.11.0\n",
			"Description=CelikPanel app: shop.example.com\n",
			"Requires=celikpanel-sandbox@celikapp-12.service\n",
			"After=celikpanel-sandbox@celikapp-12.service\n",
			"StartLimitBurst=5\n",
			"User=shop_example_com\nGroup=shop_example_com\n",
			"WorkingDirectory=" + docRoot + "\n",
			"Environment=HOME=" + docRoot + "/.home\n",
			"Environment=PATH=/opt/celikpanel/runtimes/node/22.11.0/bin:/usr/bin:/bin\n",
			"Environment=PORT=3000\nEnvironment=HOST=127.0.0.1\nEnvironment=NODE_ENV=production\n",
			"ExecStart=/bin/sh -c \"npm start\"\n",
			"Restart=on-failure\n",
			"BindPaths=/run/celikpanel-sandbox/celikapp-12.service/app:" + docRoot + "\n",
			"TemporaryFileSystem=/opt/celikpanel/runtimes:ro\n",
			"BindReadOnlyPaths=/opt/celikpanel/runtimes/node/22.11.0\n",
			"MemoryMax=1536M\n",
			"WantedBy=multi-user.target\n",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: missing %q", sys.name, want)
			}
		}
		egress := sys.caps.NFTSocketCgroup && sys.caps.CgroupV2
		if strings.Contains(text, "ExecStartPre=+/usr/lib/celikpanel-sandbox/sandbox-helper egress-add --unit=celikapp-12.service\n") != egress ||
			strings.Contains(text, "ExecStopPost=-+/usr/lib/celikpanel-sandbox/sandbox-helper egress-remove --unit=celikapp-12.service\n") != egress {
			t.Errorf("%s: egress lines must follow the host (%v)", sys.name, egress)
		}
		// No line names a path in the site home on the host as root:
		// no EnvironmentFile, no BindPaths source in the hosting tree.
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "EnvironmentFile=") {
				t.Errorf("%s: %s", sys.name, line)
			}
			if rest, ok := strings.CutPrefix(line, "BindPaths="); ok && strings.HasPrefix(rest, "/var/www/") {
				t.Errorf("%s: bind source in the hosting tree: %s", sys.name, line)
			}
			if strings.HasPrefix(line, "ExecStart=") && (strings.HasPrefix(line, "ExecStart=+") || strings.HasPrefix(line, "ExecStart=!")) {
				t.Errorf("%s: privileged start: %s", sys.name, line)
			}
		}
		if UnitGeneration(text) != sitesandbox.Generation || !Sandboxed(text) {
			t.Errorf("%s: generation not read back", sys.name)
		}
	}
}

func TestUbuntu2204RendersAsUbuntu2404(t *testing.T) {
	a, err := RenderUnit(params(systems[0].caps))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := RenderUnit(params(systems[2].caps))
	if a != b {
		t.Fatalf("22.04 and 24.04 differ:\n%s\n---\n%s", a, b)
	}
}

func TestRenderUnitRefusals(t *testing.T) {
	caps := systems[3].caps
	for name, change := range map[string]func(*Params){
		"unknown systemd":       func(p *Params) { p.Caps = sitesandbox.Caps{} },
		"folder outside":        func(p *Params) { p.AppDir = "/var/www/html" },
		"site home itself":      func(p *Params) { p.AppDir = "/var/www/celikpanel/subscriptions/3/sites/7" },
		"sub folder":            func(p *Params) { p.AppDir = docRoot + "/dist" },
		"link-like path":        func(p *Params) { p.AppDir = docRoot + "/../app" },
		"web account":           func(p *Params) { p.User = "root" },
		"privileged port":       func(p *Params) { p.Port = 443 },
		"bad version":           func(p *Params) { p.NodeVersion = "22" },
		"version path":          func(p *Params) { p.NodeVersion = "../../bin" },
		"bad domain":            func(p *Params) { p.Domain = "Shop Example" },
		"command line break":    func(p *Params) { p.Command = "npm start\n[Service]" },
		"drop-in with a space":  func(p *Params) { p.DropInDir = "/etc/systemd/system/a b" },
		"no site":               func(p *Params) { p.SiteID = 0 },
		"empty start command":   func(p *Params) { p.Command = " " },
		"specifier in drop-ins": func(p *Params) { p.DropInDir = "/etc/systemd/%n" },
	} {
		p := params(caps)
		change(&p)
		if _, err := RenderUnit(p); err == nil {
			t.Errorf("%s: rendered", name)
		}
	}
}

// An earlier release's unit (alpha.82 and later wrote no sandbox marker)
// reads as generation 0: it is never treated as moved.
func TestEarlierUnitIsNotSandboxed(t *testing.T) {
	earlier := "# celikpanel-render v2 sha256=00\n# The application unit of this site, written by CelikPanel.\n[Unit]\nDescription=CelikPanel app: shop.example.com\n\n[Service]\nUser=shop_example_com\nExecStart=/bin/sh -c 'npm start'\n"
	if UnitGeneration(earlier) != 0 || Sandboxed(earlier) {
		t.Fatal("an earlier unit read as sandboxed")
	}
	if UnitGeneration("# celikpanel-sandbox-generation: x\n") != 0 || UnitGeneration("# celikpanel-sandbox-generation: 2\n") != 2 || Sandboxed("# celikpanel-sandbox-generation: 2\n") {
		t.Fatal("generation parsing")
	}
}

func TestInstallRunArgs(t *testing.T) {
	for _, sys := range systems {
		p := InstallParams{SiteID: 12, Domain: "shop.example.com", User: "shop_example_com", Group: "shop_example_com",
			AppDir: docRoot, NodeVersion: "22.11.0", Clean: true, Build: true, Caps: sys.caps}
		args, err := InstallRunArgs(p)
		if err != nil {
			t.Fatalf("%s: %v", sys.name, err)
		}
		joined := strings.Join(args, "\n")
		for _, want := range []string{
			"--unit=celikappinst-12.service", "--uid=shop_example_com", "--gid=shop_example_com",
			"--working-directory=" + docRoot, "--setenv=HOME=" + docRoot + "/.home",
			"--setenv=npm_config_cache=" + docRoot + "/.home/.npm",
			"--setenv=PATH=/opt/celikpanel/runtimes/node/22.11.0/bin:/usr/bin:/bin",
			"--property=Type=oneshot", "--no-block",
			"--property=Requires=celikpanel-sandbox@celikappinst-12.service",
			"--property=BindPaths=/run/celikpanel-sandbox/celikappinst-12.service/app:" + docRoot,
			"--property=BindReadOnlyPaths=/opt/celikpanel/runtimes/node/22.11.0",
			"--property=MemoryMax=2048M",
			"--property=ExecStartPost=/opt/celikpanel/runtimes/node/22.11.0/bin/node /opt/celikpanel/runtimes/node/22.11.0/lib/node_modules/npm/bin/npm-cli.js run build --if-present",
		} {
			if !strings.Contains(joined+"\n", want+"\n") {
				t.Errorf("%s: missing %q", sys.name, want)
			}
		}
		tail := args[len(args)-7:]
		want := []string{"--", "/opt/celikpanel/runtimes/node/22.11.0/bin/node", "/opt/celikpanel/runtimes/node/22.11.0/lib/node_modules/npm/bin/npm-cli.js", "ci", "--no-audit", "--no-fund", "--loglevel=warn"}
		if strings.Join(tail, " ") != strings.Join(want, " ") {
			t.Errorf("%s: command %v", sys.name, tail)
		}
		// No listening port for the install step; no "$" or "%" anywhere.
		if strings.Contains(joined, "SocketBindAllow") || strings.ContainsAny(joined, "$%") && !strings.Contains(joined, "CPUQuota=200%") {
			t.Errorf("%s: unexpected lines", sys.name)
		}
		for _, arg := range args {
			if strings.HasPrefix(arg, "--property=") {
				if why := sitesandbox.TransientPropertyRefusal(strings.TrimPrefix(arg, "--property="), sys.caps.SystemdVersion); why != "" {
					t.Errorf("%s: %s: %s", sys.name, arg, why)
				}
			}
		}
	}
	p := InstallParams{SiteID: 12, Domain: "shop.example.com", User: "shop_example_com", Group: "shop_example_com",
		AppDir: docRoot, NodeVersion: "22.11.0", Caps: systems[3].caps}
	args, _ := InstallRunArgs(p)
	if strings.Contains(strings.Join(args, " "), "run build") || args[len(args)-4] != "install" {
		t.Fatalf("without a lock file and without build: %v", args[len(args)-6:])
	}
}

func TestInstallOutcome(t *testing.T) {
	for _, c := range []struct {
		found               bool
		active, result, job string
		want                string
	}{
		{false, "", "", "", InstallFailed},
		{true, "inactive", "success", "42", InstallRunning},
		{true, "activating", "success", "", InstallRunning},
		{true, "active", "success", "", InstallSucceeded},
		{true, "failed", "exit-code", "", InstallFailed},
		{true, "inactive", "exit-code", "", InstallFailed},
		{true, "inactive", "success", "", InstallSucceeded},
	} {
		if got := InstallOutcome(c.found, c.active, c.result, c.job); got != c.want {
			t.Errorf("%+v: %s", c, got)
		}
	}
}
