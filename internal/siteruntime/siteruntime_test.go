package siteruntime

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

func testLayout(t *testing.T) Layout {
	t.Helper()
	l, err := LayoutFor(3, 7)
	if err != nil {
		t.Fatal(err)
	}
	if l.Root != "/var/www/celikpanel/subscriptions/3/sites/7/app" {
		t.Fatalf("root %s", l.Root)
	}
	return l
}

func fullCaps(version int) sitesandbox.Caps {
	return sitesandbox.Caps{SystemdVersion: version, BPFFramework: true, CgroupV2: true, NFTSocketCgroup: true}
}

func params(t *testing.T) UnitParams {
	return UnitParams{AppID: 12, Kind: KindProgram, Domain: "shop.example.com", User: "shop_example_com", Group: "shop_example_com",
		Layout: testLayout(t), Entry: "server", Args: []string{"--listen", "127.0.0.1:{port}"}, Port: 30010,
		Limits: DefaultLimits(KindProgram), Level: sitesandbox.LevelFull, Caps: fullCaps(257),
		DropInDir: "/etc/systemd/system/celikrt-12.service.d"}
}

func TestKindsAndOfferings(t *testing.T) {
	if !Implemented(KindProgram) || !Implemented(KindPython) || !Known(KindPython) || Known("node") {
		t.Fatal("kinds")
	}
	if Permission(KindProgram) != "program_apps" || Permission(KindPython) != "python_apps" {
		t.Fatal("permissions")
	}
	if OfferingID(KindProgram) != "runtime:program" {
		t.Fatal("offering")
	}
	if k, ok := KindOfOffering("component:python:3.13"); !ok || k != KindPython {
		t.Fatal("python offering")
	}
	if _, ok := KindOfOffering("runtime:node"); ok {
		t.Fatal("unknown offering accepted")
	}
	if RiskDigest(KindProgram) == RiskDigest(KindPython) || len(RiskDigest(KindProgram)) != 16 {
		t.Fatal("digest")
	}
}

func TestEntryNames(t *testing.T) {
	l := testLayout(t)
	for _, good := range []string{"server", "api-v2", "release/api", "a/b/c/d", "_start", "app.bin"} {
		if full, err := ValidEntry(l, good); err != nil || !strings.HasPrefix(full, l.BinDir()+"/") {
			t.Errorf("%q refused: %v", good, err)
		}
	}
	for _, bad := range []string{"", "../x", "a/../../etc/passwd", "/bin/sh", ".hidden", "a b", "a$b", "a%b", "a/b/c/d/e",
		"x\nExecStartPre=+/bin/sh", "ser'ver", `ser"ver`, `a\b`, "a//b", "a/", strings.Repeat("a", 65), "-flag"} {
		if _, err := ValidEntry(l, bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func elf(machine uint16) []byte {
	h := make([]byte, 64)
	copy(h, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	h[18], h[19] = byte(machine), byte(machine>>8)
	return h
}

func TestEntryCheckEdgeCases(t *testing.T) {
	root := testLayout(t).Root
	ok := FileFacts{Regular: true, UID: 1001, Mode: 0o750, Header: elf(62), HostArch: MachineFor("amd64"), FolderUID: 1001}
	if code, _ := CheckEntry(ok, 1001, root); code != EntryOK {
		t.Fatalf("good ELF: %s", code)
	}
	cases := []struct {
		name string
		edit func(*FileFacts)
		want string
	}{
		{"link", func(f *FileFacts) { f.Link = true }, EntryLink},
		{"folder link", func(f *FileFacts) { f.FolderLink = true }, EntryFolderNotReal},
		{"folder of another account", func(f *FileFacts) { f.FolderUID = 0 }, EntryFolderNotOwned},
		{"directory", func(f *FileFacts) { f.Regular = false }, EntryNotRegular},
		{"root's file", func(f *FileFacts) { f.UID = 0 }, EntryNotOwned},
		{"setuid", func(f *FileFacts) { f.Mode = 0o4755 }, EntrySetID},
		{"setgid", func(f *FileFacts) { f.Mode = 0o2755 }, EntrySetID},
		{"world writable", func(f *FileFacts) { f.Mode = 0o757 }, EntryWritable},
		{"not executable", func(f *FileFacts) { f.Mode = 0o640 }, EntryNotExecutable},
		{"group-only exec", func(f *FileFacts) { f.Mode = 0o650 }, EntryNotExecutable},
		{"arm64 binary", func(f *FileFacts) { f.Header = elf(183) }, EntryWrongArch},
		{"text", func(f *FileFacts) { f.Header = []byte("hello world") }, EntryUnknownFormat},
		{"empty", func(f *FileFacts) { f.Header = nil }, EntryUnknownFormat},
		{"short ELF", func(f *FileFacts) { f.Header = []byte{0x7f, 'E', 'L', 'F'} }, EntryUnknownFormat},
		{"sh script", func(f *FileFacts) { f.Header = []byte("#!/bin/sh\necho hi\n") }, EntryInterpreter},
		{"env python", func(f *FileFacts) { f.Header = []byte("#!/usr/bin/env python3\n") }, EntryInterpreter},
		{"relative interpreter", func(f *FileFacts) { f.Header = []byte("#!bin/run\n") }, EntryInterpreter},
		{"escaping interpreter", func(f *FileFacts) { f.Header = []byte("#!" + root + "/../../x\n") }, EntryInterpreter},
		{"empty shebang", func(f *FileFacts) { f.Header = []byte("#!\n") }, EntryInterpreter},
		{"own interpreter", func(f *FileFacts) { f.Header = []byte("#!" + root + "/bin/myinterp -x\n") }, EntryOK},
	}
	for _, c := range cases {
		f := ok
		c.edit(&f)
		if code, why := CheckEntry(f, 1001, root); code != c.want || why == "" {
			t.Errorf("%s: %s (%s), want %s", c.name, code, why, c.want)
		}
	}
}

func TestArgsAreTokensWithOnlyThePortPlaceholder(t *testing.T) {
	got, err := ExpandArgs([]string{"--port={port}", "-addr", "127.0.0.1:{port}", "serve"}, 30010)
	if err != nil || strings.Join(got, " ") != "--port=30010 -addr 127.0.0.1:30010 serve" {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range [][]string{
		{"a b"}, {"$HOME"}, {"${PORT}"}, {"%h"}, {"'x'"}, {`"x"`}, {`a\b`}, {";"}, {"x;y"}, {"{host}"}, {"{port"}, {"{"},
		{""}, {"a\nb"}, {"`id`"}, {"$(id)"}, {"a|b"}, {"a&b"}, {">x"}, {strings.Repeat("a", 257)},
	} {
		if err := ValidateArgs(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	many := make([]string, MaxArgs+1)
	for i := range many {
		many[i] = "x"
	}
	if ValidateArgs(many) == nil {
		t.Error("too many arguments accepted")
	}
}

func TestEnvRefusals(t *testing.T) {
	for _, key := range []string{"PATH", "HOME", "PORT", "HOST", "PYTHONPATH", "PYTHONHOME", "LD_PRELOAD", "LD_LIBRARY_PATH", "LD_AUDIT",
		"CELIKPANEL_TOKEN", "SYSTEMD_X", "NOTIFY_SOCKET", "lower", "1KEY", "A-B", "", strings.Repeat("A", 65)} {
		if EnvKeyRefusal(key) == "" {
			t.Errorf("%q allowed", key)
		}
	}
	for _, key := range []string{"DATABASE_URL", "API_KEY", "LOG_LEVEL", "GOMAXPROCS", "RUST_LOG"} {
		if why := EnvKeyRefusal(key); why != "" {
			t.Errorf("%q refused: %s", key, why)
		}
	}
	if ValidateEnv([]EnvEntry{{Key: "A", Value: "x"}, {Key: "A", Value: "y"}}) == nil {
		t.Error("duplicate accepted")
	}
	if ValidateEnv([]EnvEntry{{Key: "A", Value: "line\nINJECTED=1"}}) == nil {
		t.Error("newline accepted")
	}
	if ValidateEnv([]EnvEntry{{Key: "A", Value: ""}}) == nil {
		t.Error("empty public value accepted")
	}
	if ValidateEnv([]EnvEntry{{Key: "TOKEN", Secret: true}}) != nil {
		t.Error("a kept secret without a value was refused")
	}
}

func TestEnvMergeKeepsSecretsWriteOnly(t *testing.T) {
	existing := map[string]string{"TOKEN": "s3cr3t-value", "OLD": "gone", "LOG_LEVEL": "debug"}
	merged, err := MergeEnv([]EnvEntry{{Key: "LOG_LEVEL", Value: "info"}, {Key: "TOKEN", Secret: true}, {Key: "NEW_SECRET", Value: "abc$def\"x", Secret: true}}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if merged["TOKEN"] != "s3cr3t-value" || merged["LOG_LEVEL"] != "info" || merged["OLD"] != "" || merged["NEW_SECRET"] != "abc$def\"x" {
		t.Fatalf("merged %v", merged)
	}
	text, err := RenderEnv(merged)
	if err != nil {
		t.Fatal(err)
	}
	back := ParseEnv(text)
	for k, v := range merged {
		if back[k] != v {
			t.Fatalf("%s round trip %q -> %q\n%s", k, v, back[k], text)
		}
	}
	if _, err := RenderEnv(map[string]string{"LD_PRELOAD": "/x.so"}); err == nil {
		t.Fatal("refused key rendered")
	}
	if got := ParseEnv("PATH=/evil\nLD_PRELOAD=/x\nOK=1\n"); got["PATH"] != "" || got["LD_PRELOAD"] != "" || got["OK"] != "1" {
		t.Fatalf("parse %v", got)
	}
}

func TestPortAllocation(t *testing.T) {
	port, err := AllocatePort(map[int]bool{30000: true, 30001: true, 21000: true})
	if err != nil || port != 30002 {
		t.Fatalf("%d %v", port, err)
	}
	taken := map[int]bool{}
	for p := PortMin; p <= PortMax; p++ {
		taken[p] = true
	}
	if _, err := AllocatePort(taken); err == nil {
		t.Fatal("full range allocated")
	}
	if ValidPort(29999) || ValidPort(35000) || !ValidPort(30000) || !ValidPort(34999) {
		t.Fatal("range")
	}
}

func TestUnitTextHasNoShellAndNoExpansion(t *testing.T) {
	for _, version := range []int{249, 252, 255, 257} {
		p := params(t)
		p.Caps = fullCaps(version)
		text, err := RenderUnit(p)
		if err != nil {
			t.Fatalf("%d: %v", version, err)
		}
		for _, want := range []string{
			"User=shop_example_com\n", "ExecStart=/var/www/celikpanel/subscriptions/3/sites/7/app/bin/server --listen 127.0.0.1:30010\n",
			"Restart=on-failure\n", "StartLimitBurst=5\n", "MemoryDenyWriteExecute=yes\n", "NoExecPaths=/\n", "ProcSubset=pid\n",
			"EnvironmentFile=-/etc/celikpanel-runtime/celikrt-12.service.env\n", "Environment=PORT=30010\n", "Environment=HOST=127.0.0.1\n",
			"ExecStartPre=+/usr/lib/celikpanel-sandbox/sandbox-helper egress-add --unit=celikrt-12.service\n",
			"ExecStopPost=-+/usr/lib/celikpanel-sandbox/sandbox-helper egress-remove --unit=celikrt-12.service\n",
			"Requires=celikpanel-sandbox@celikrt-12.service\n", "After=celikpanel-sandbox@celikrt-12.service\n",
			"BindPaths=/run/celikpanel-sandbox/celikrt-12.service/app:" + p.Layout.Root + "\n",
			"WorkingDirectory=" + p.Layout.Root + "\n",
			"SocketBindAllow=tcp:30010\n", "SocketBindDeny=any\n",
			"# celikpanel-sandbox-generation: 3\n", "# celikpanel-runtime-kind: program\n",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%d: missing %q\n%s", version, want, text)
			}
		}
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "#") {
				continue
			}
			if strings.Contains(line, "$") || (strings.Contains(line, "%") && !strings.HasPrefix(line, "CPUQuota=")) {
				t.Fatalf("expansion in %q", line)
			}
		}
		for _, banned := range []string{"/bin/sh", "sh -c", "www-data", "user=root"} {
			if strings.Contains(strings.ToLower(text), banned) {
				t.Fatalf("%d: %q in unit", version, banned)
			}
		}
		if UnitKind(text) != KindProgram {
			t.Fatal("kind marker")
		}
	}
}

func TestUnitRefusesBadInputs(t *testing.T) {
	cases := map[string]func(*UnitParams){
		"python kind":    func(p *UnitParams) { p.Kind = KindPython },
		"root":           func(p *UnitParams) { p.User = "root" },
		"www-data group": func(p *UnitParams) { p.Group = "www-data!" },
		"port":           func(p *UnitParams) { p.Port = 25000 },
		"entry":          func(p *UnitParams) { p.Entry = "../../../bin/sh" },
		"arg dollar":     func(p *UnitParams) { p.Args = []string{"$HOME"} },
		"arg percent":    func(p *UnitParams) { p.Args = []string{"%i"} },
		"domain":         func(p *UnitParams) { p.Domain = "a%b.com" },
		"limits":         func(p *UnitParams) { p.Limits.MemoryMB = 1 },
		"unknown host":   func(p *UnitParams) { p.Caps = sitesandbox.Caps{} },
		"folder":         func(p *UnitParams) { p.Layout.Root = "/etc/app" },
	}
	for name, edit := range cases {
		p := params(t)
		edit(&p)
		if _, err := RenderUnit(p); err == nil {
			t.Errorf("%s: rendered", name)
		}
	}
}

// TestReducedLevelNeverPassesSilently: a host without BPF renders a unit only
// when the offering accepted the reduced level, and then without the
// directives systemd would not enforce.
func TestReducedLevelNeverPassesSilently(t *testing.T) {
	p := params(t)
	p.Caps = sitesandbox.Caps{SystemdVersion: 249, CgroupV2: true, NFTSocketCgroup: true}
	if _, err := RenderUnit(p); err == nil || !strings.Contains(err.Error(), "sandbox_reduced") {
		t.Fatalf("full-level offering rendered on a reduced host: %v", err)
	}
	p.Level = sitesandbox.LevelReduced
	text, err := RenderUnit(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "SocketBind") || strings.Contains(text, "IPAddressDeny") {
		t.Fatal("unenforced BPF directives written")
	}
	a := sitesandbox.Assess(p.Caps)
	if GapsCovered(a.Gaps, nil) || !GapsCovered(a.Gaps, []string{"bpf_missing"}) {
		t.Fatalf("gaps %v", a.Gaps)
	}
	if _, err := NormalizeGaps([]string{"bpf_missing", "made_up"}); err == nil {
		t.Fatal("unknown gap accepted")
	}
	if got, _ := NormalizeGaps([]string{"nft_cgroup_missing", "bpf_missing", "bpf_missing"}); strings.Join(got, ",") != "bpf_missing,nft_cgroup_missing" {
		t.Fatalf("normalised %v", got)
	}
}

func TestHealthPath(t *testing.T) {
	for _, good := range []string{"/", "/healthz", "/api/v1/health"} {
		if !ValidHealthPath(good) {
			t.Errorf("%q refused", good)
		}
	}
	for _, bad := range []string{"", "health", "/a?b", "/%2e", "/../x", "/a b", "http://x/"} {
		if ValidHealthPath(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
