package sitesandbox

import (
	"reflect"
	"strings"
	"testing"
)

// Node.js sites in the site sandbox (alpha.127).

const nodeDocRoot = "/var/www/celikpanel/subscriptions/3/sites/7/public_html"

func nodeProfile() Profile {
	return Profile{
		Kind: KindNode, Unit: "celikapp-12.service", User: "shop_example_com", Group: "shop_example_com", AppDir: nodeDocRoot,
		RuntimeDir: RuntimesBase + "/node/22.11.0", Port: 3000,
		Limits: Limits{MemoryMB: 1536, CPUPercent: 200, Tasks: 512, NOFILE: 8192}, Level: LevelFull,
	}
}

// The five supported systems as the alpha.123 lab measured them: Ubuntu
// 22.04 and 24.04 and Debian 12 without BPF (reduced), Debian 13 and Arch
// full.
var nodeSystems = []struct {
	name string
	caps Caps
	want Level
}{
	{"ubuntu-22.04", Caps{SystemdVersion: 249, CgroupV2: true, NFTSocketCgroup: true}, LevelReduced},
	{"debian-12", Caps{SystemdVersion: 252, CgroupV2: true, NFTSocketCgroup: true}, LevelReduced},
	{"ubuntu-24.04", Caps{SystemdVersion: 255, CgroupV2: true, NFTSocketCgroup: true}, LevelReduced},
	{"debian-13", fullCaps(257), LevelFull},
	{"arch", fullCaps(262), LevelFull},
}

func TestNodeDirectivesOnEverySystem(t *testing.T) {
	for _, sys := range nodeSystems {
		p := nodeProfile()
		p.Level = Assess(sys.caps).Level
		if p.Level != sys.want {
			t.Fatalf("%s: level %s, want %s", sys.name, p.Level, sys.want)
		}
		lines, err := Directives(p, sys.caps)
		if err != nil {
			t.Fatalf("%s: %v", sys.name, err)
		}
		text := strings.Join(lines, "\n")
		for _, want := range []string{
			"NoNewPrivileges=yes", "ProtectSystem=strict", "ProtectHome=yes", "PrivateTmp=yes", "PrivateDevices=yes",
			"CapabilityBoundingSet=", "RestrictSUIDSGID=yes", "ProtectProc=invisible", "PrivateIPC=yes",
			"TemporaryFileSystem=/var/www/celikpanel:ro",
			"BindPaths=/run/celikpanel-sandbox/celikapp-12.service/app:" + nodeDocRoot,
			"TemporaryFileSystem=/opt/celikpanel/runtimes:ro",
			"BindReadOnlyPaths=/opt/celikpanel/runtimes/node/22.11.0",
			"InaccessiblePaths=-/run/celikpanel ", "MemoryMax=1536M", "MemorySwapMax=0", "TasksMax=512",
		} {
			if !strings.Contains(text+"\n", want) {
				t.Errorf("%s: missing %q", sys.name, want)
			}
		}
		// V8 compiles code at run time and npm runs /bin/sh: no W^X and
		// no exec allow-list for Node.js; the full /proc stays (os.cpus()).
		for _, absent := range []string{"MemoryDenyWriteExecute", "NoExecPaths", "ExecPaths", "ProcSubset"} {
			if has(lines, absent) {
				t.Errorf("%s: %s must not be set for Node.js", sys.name, absent)
			}
		}
		bpf := sys.caps.BPFFramework
		if has(lines, "SocketBindAllow=tcp:3000") != bpf || has(lines, "SocketBindDeny=any") != bpf || has(lines, "IPAddressDeny=") != bpf {
			t.Errorf("%s: BPF lines must follow the BPF framework (%v)", sys.name, bpf)
		}
		if has(lines, "PrivatePIDs=yes") != (sys.caps.SystemdVersion >= 257) {
			t.Errorf("%s: PrivatePIDs gating", sys.name)
		}
		// The proof expects what was rendered.
		e := Expected(p, sys.caps, p.Unit)
		if e["BindReadOnlyPaths"].Value != p.RuntimeDir || e["ProcSubset"].Value != "" {
			t.Errorf("%s: proof expectation %+v", sys.name, e)
		}
	}
}

// Owner decision 2026-11-05 holds for Node.js too: Ubuntu 22.04 is Ubuntu
// 24.04.
func TestNodeUbuntu2204IsUbuntu2404(t *testing.T) {
	p := nodeProfile()
	p.Level = LevelReduced
	for _, render := range []func(Profile, Caps) ([]string, error){Directives, TransientDirectives} {
		a, _ := render(p, nodeSystems[0].caps)
		b, _ := render(p, nodeSystems[2].caps)
		if !reflect.DeepEqual(a, b) || len(a) == 0 {
			t.Fatalf("22.04 %v\n24.04 %v", a, b)
		}
	}
}

func TestNodeTransientFormParsesOnEverySystemd(t *testing.T) {
	install := nodeProfile()
	install.Unit, install.Port = "celikappinst-12.service", 0
	for _, version := range transientSystemdVersions {
		for _, caps := range []Caps{fullCaps(version), {SystemdVersion: version, CgroupV2: true, NFTSocketCgroup: true}} {
			install.Level = Assess(caps).Level
			lines, err := TransientDirectives(install, caps)
			if err != nil {
				t.Fatalf("%d: %v", version, err)
			}
			unitLines, pre, post, _ := HelperLines(install, caps)
			for _, line := range append(append(append(lines, unitLines...), pre...), post...) {
				if why := TransientPropertyRefusal(line, version); why != "" {
					t.Errorf("systemd %d: --property=%s: %s", version, line, why)
				}
			}
		}
	}
}

func TestNodeProfileValidation(t *testing.T) {
	p := nodeProfile()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Profile){
		"privileged port":      func(p *Profile) { p.Port = 80 },
		"runtime outside":      func(p *Profile) { p.RuntimeDir = "/usr/local/node" },
		"runtime not clean":    func(p *Profile) { p.RuntimeDir = RuntimesBase + "/node/../python" },
		"runtime with space":   func(p *Profile) { p.RuntimeDir = RuntimesBase + "/node/2 2" },
		"web server account":   func(p *Profile) { p.User = "root" },
		"unit of another kind": func(p *Profile) { p.Unit = "celikweb-12.service" },
	} {
		q := nodeProfile()
		change(&q)
		if q.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A Node.js site keeps a port outside the runtime apps' range; the
	// other kinds do not get it.
	q := programProfile()
	q.Port = 3000
	if q.Validate() == nil {
		t.Fatal("an uploaded program accepted port 3000")
	}
	for _, unit := range []string{"celikapp-1.service", "celikappinst-99.service"} {
		if !ValidUnitName(unit) {
			t.Errorf("%s is a sandboxed unit", unit)
		}
	}
	for _, unit := range []string{"celikapp-0.service", "celikapp-1", "celikappx-1.service"} {
		if ValidUnitName(unit) {
			t.Errorf("%s is not a sandboxed unit", unit)
		}
	}
}

// Node.js listens on every address by default. Where the egress block
// drops outside connections to sandboxed sockets, that counts as serving on
// the site's own port; a wildcard on another port, or a named public
// address, is still exposed; without the block a wildcard is exposed.
func TestNodeListenProof(t *testing.T) {
	ss := "LISTEN 0 511 *:3000 *:* users:((\"node\",pid=10,fd=20))\n"
	pids := map[int]bool{10: true}
	if proof := ProveNodeListening(ParseSS(ss), pids, 3000, true); !proof.Serving || len(proof.Exposed) != 0 {
		t.Fatalf("wildcard own port with the block: %+v", proof)
	}
	if proof := ProveNodeListening(ParseSS(ss), pids, 3000, false); proof.Serving || len(proof.Exposed) != 1 {
		t.Fatalf("wildcard without the block: %+v", proof)
	}
	other := ss + "LISTEN 0 511 0.0.0.0:9229 0.0.0.0:* users:((\"node\",pid=10,fd=21))\nLISTEN 0 511 203.0.113.5:3001 0.0.0.0:* users:((\"node\",pid=10,fd=22))\n"
	if proof := ProveNodeListening(ParseSS(other), pids, 3000, true); !proof.Serving || len(proof.Exposed) != 2 {
		t.Fatalf("other ports: %+v", proof)
	}
	loop := "LISTEN 0 511 127.0.0.1:3000 0.0.0.0:* users:((\"node\",pid=10,fd=20))\n"
	if proof := ProveNodeListening(ParseSS(loop), pids, 3000, false); !proof.Serving || len(proof.Exposed) != 0 {
		t.Fatalf("loopback: %+v", proof)
	}
	squat := "LISTEN 0 511 [::]:3000 [::]:* users:((\"other\",pid=99,fd=3))\n"
	if proof := ProveNodeListening(ParseSS(squat), pids, 3000, true); proof.Serving || !proof.Squatted {
		t.Fatalf("another program on the port: %+v", proof)
	}
}

// The guard re-adds Node.js units that the helper loads; an earlier
// release's celikapp unit (no egress-add line) is never added silently.
func TestEgressGuardCoversNodeUnits(t *testing.T) {
	script := EgressGuardScriptText()
	for _, want := range []string{"'celikapp-*.service'", "'celikappinst-*.service'", "celikapp-[0-9]*.service|celikappinst-[0-9]*.service", "egress-add "} {
		if !strings.Contains(script, want) {
			t.Errorf("guard lacks %q", want)
		}
	}
}
