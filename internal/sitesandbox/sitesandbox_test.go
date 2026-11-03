package sitesandbox

import (
	"strings"
	"testing"
)

const appDir = "/var/www/celikpanel/subscriptions/3/sites/7/app"

func programProfile() Profile {
	return Profile{
		Kind: KindProgram, User: "shop_example_com", Group: "shop_example_com", AppDir: appDir,
		Port: 30010, Limits: Limits{MemoryMB: 512, CPUPercent: 100, Tasks: 128}, Level: LevelFull,
	}
}

func fullCaps(version int) Caps {
	return Caps{SystemdVersion: version, BPFFramework: true, CgroupV2: true, NFTSocketCgroup: true}
}

func has(lines []string, prefix string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

// TestDirectiveGatingPerSystemd: every supported systemd (Ubuntu 22.04 249,
// Debian 12 252, Ubuntu 24.04 255, Debian 13 257) gets exactly the
// directives it has, and never one newer than itself.
func TestDirectiveGatingPerSystemd(t *testing.T) {
	type row struct {
		version          int
		privatePIDs      bool
		strictCgroups    bool
		socketBind, proc bool
	}
	for _, r := range []row{
		{249, false, false, true, true},
		{252, false, false, true, true},
		{255, false, false, true, true},
		{257, true, true, true, true},
		{248, false, false, false, true},
		{246, false, false, false, false},
	} {
		lines, err := Directives(programProfile(), fullCaps(r.version))
		if err != nil {
			t.Fatalf("%d: %v", r.version, err)
		}
		if has(lines, "PrivatePIDs=yes") != r.privatePIDs {
			t.Errorf("%d: PrivatePIDs present=%v", r.version, !r.privatePIDs)
		}
		if has(lines, "ProtectControlGroups=strict") != r.strictCgroups || has(lines, "ProtectControlGroups=yes") == r.strictCgroups {
			t.Errorf("%d: ProtectControlGroups wrong: %v", r.version, lines)
		}
		if has(lines, "SocketBindAllow=tcp:30010") != r.socketBind || has(lines, "SocketBindDeny=any") != r.socketBind {
			t.Errorf("%d: SocketBind present=%v", r.version, !r.socketBind)
		}
		if has(lines, "ProcSubset=pid") != r.proc || has(lines, "ProtectProc=invisible") != r.proc {
			t.Errorf("%d: ProcSubset/ProtectProc wrong", r.version)
		}
		if has(lines, "PrivateIPC=yes") != (r.version >= 248) || has(lines, "NoExecPaths=/") != (r.version >= 248) {
			t.Errorf("%d: PrivateIPC/NoExecPaths wrong", r.version)
		}
		for _, l := range lines {
			name, _, _ := strings.Cut(l, "=")
			key := name
			if l == "ProtectControlGroups=strict" {
				key = l
			}
			if min, ok := DirectiveMinimums[key]; ok && r.version < min {
				t.Errorf("%d: %s needs systemd %d", r.version, l, min)
			}
		}
	}
}

func TestDirectivesWithoutBPFLeaveOutUnenforcedLines(t *testing.T) {
	c := fullCaps(249)
	c.BPFFramework = false
	lines, err := Directives(programProfile(), c)
	if err != nil {
		t.Fatal(err)
	}
	if has(lines, "SocketBind") || has(lines, "IPAddressDeny") {
		t.Fatalf("BPF directives written where they are not enforced: %v", lines)
	}
	a := Assess(c)
	if a.Level != LevelReduced || len(a.Gaps) != 1 || a.Gaps[0] != GapBPF {
		t.Fatalf("assessment = %+v", a)
	}
	if err := Permit(programProfile(), c); err == nil || !strings.HasPrefix(err.Error(), "sandbox_reduced") {
		t.Fatalf("a full-level profile on a reduced host: %v", err)
	}
	accepted := programProfile()
	accepted.Level = LevelReduced
	if err := Permit(accepted, c); err != nil {
		t.Fatalf("an accepted reduced level refused: %v", err)
	}
	if err := Permit(programProfile(), fullCaps(257)); err != nil {
		t.Fatal(err)
	}
}

func TestBaseSetAndKinds(t *testing.T) {
	lines, err := Directives(programProfile(), fullCaps(257))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n") + "\n"
	for _, want := range []string{
		"NoNewPrivileges=yes\n", "CapabilityBoundingSet=\n", "RestrictSUIDSGID=yes\n",
		"SystemCallFilter=@system-service\n", "SystemCallFilter=" + SyscallDeny + "\n",
		"RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK\n",
		"TemporaryFileSystem=/var/www/celikpanel:ro\n", "BindPaths=" + appDir + "\n", "ReadWritePaths=" + appDir + "\n",
		"InaccessiblePaths=-/run/celikpanel -/run/php -/run/php-fpm -/etc/celikpanel -/var/lib/celikpanel ",
		"-/root -/opt/celikpanel/toolchains -/var/spool/postfix\n",
		"IPAddressDeny=169.254.0.0/16 fe80::/10 ", "LogRateLimitBurst=1000\n",
		"MemoryMax=512M\n", "MemorySwapMax=0\n", "CPUQuota=100%\n", "TasksMax=128\n", "LimitNOFILE=8192\n",
		"MemoryDenyWriteExecute=yes\n", "NoExecPaths=/\n",
		"ExecPaths=" + appDir + " -/usr/lib -/usr/lib64 -/lib -/lib64\n",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("program kind lacks %q", want)
		}
	}
	py := programProfile()
	py.Kind = KindPython
	lines, _ = Directives(py, fullCaps(257))
	if has(lines, "MemoryDenyWriteExecute") || has(lines, "NoExecPaths") || !has(lines, "ProcSubset=pid") {
		t.Errorf("python kind: %v", lines)
	}
	svc := programProfile()
	svc.Kind = KindServiceApp
	svc.ExtraPorts = []int{30011}
	lines, _ = Directives(svc, fullCaps(252))
	if has(lines, "ProcSubset") || !has(lines, "SocketBindAllow=tcp:30011") {
		t.Errorf("service-app kind: %v", lines)
	}
	install := programProfile()
	install.Port = 0
	lines, _ = Directives(install, fullCaps(252))
	if has(lines, "SocketBindAllow") || !has(lines, "SocketBindDeny=any") {
		t.Errorf("a unit without a port binds nothing: %v", lines)
	}
}

// TestValuesAreRefusedNotEscaped: a value systemd would expand, unquote or
// split never reaches a line.
func TestValuesAreRefusedNotEscaped(t *testing.T) {
	for _, bad := range []string{"", "a b", "a$b", "a%b", `a"b`, "a'b", `a\b`, "a\nb", "a;b", "ü"} {
		if CheckValue(bad) == nil {
			t.Errorf("value %q accepted", bad)
		}
	}
	if CheckValue("/var/www/celikpanel/subscriptions/3/sites/7/app") != nil {
		t.Error("a plain path refused")
	}
	cases := map[string]func(*Profile){
		"dollar in folder":   func(p *Profile) { p.AppDir = appDir + "/$HOME" },
		"percent in folder":  func(p *Profile) { p.AppDir = appDir + "/%h" },
		"space in folder":    func(p *Profile) { p.AppDir = appDir + "/a b" },
		"outside site home":  func(p *Profile) { p.AppDir = "/etc" },
		"dotdot":             func(p *Profile) { p.AppDir = appDir + "/../../../../x" },
		"root":               func(p *Profile) { p.User, p.Group = "root", "root" },
		"read-only hosting":  func(p *Profile) { p.ReadOnlyPaths = []string{"/var/www/celikpanel/subscriptions/4"} },
		"read-only with $":   func(p *Profile) { p.ReadOnlyPaths = []string{"/usr/$x"} },
		"privileged port":    func(p *Profile) { p.Port = 25 },
		"repeated port":      func(p *Profile) { p.ExtraPorts = []int{30010} },
		"no level":           func(p *Profile) { p.Level = "" },
		"tiny memory":        func(p *Profile) { p.Limits.MemoryMB = 8 },
		"unknown kind":       func(p *Profile) { p.Kind = "shell" },
		"extra without main": func(p *Profile) { p.Port = 0; p.ExtraPorts = []int{30011} },
	}
	for name, mutate := range cases {
		p := programProfile()
		mutate(&p)
		if lines, err := Directives(p, fullCaps(257)); err == nil {
			t.Errorf("%s: rendered %v", name, lines)
		}
	}
	lines, _ := Directives(programProfile(), fullCaps(257))
	for _, l := range lines {
		if err := checkLine(l); err != nil {
			t.Errorf("%v", err)
		}
		if strings.Contains(l, "$") {
			t.Errorf("line with $: %q", l)
		}
	}
}

func TestEgressFileAndLines(t *testing.T) {
	text := EgressRuleset()
	for _, want := range []string{
		"table inet celikpanel_egress {", "set smtp_blocked {", "typeof socket cgroupv2 level 2",
		"counter smtp_rejects {", "type filter hook output priority filter; policy accept;",
		"flush chain inet celikpanel_egress output",
		`tcp dport { 25, 465, 587 } socket cgroupv2 level 2 @smtp_blocked counter name "smtp_rejects" reject with tcp reset`,
		`iifname != "lo"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("egress file lacks %q", want)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "celikpanel_fw") || strings.Contains(line, "flush ruleset") || strings.Contains(line, "delete") {
			t.Fatalf("the egress file must never touch the firewall table or the whole ruleset: %q", line)
		}
	}
	// The flush comes after the declarations, so the set's elements survive.
	if strings.Index(text, "flush chain") < strings.Index(text, "set smtp_blocked") {
		t.Fatal("the chain is flushed before the set is declared")
	}

	pre, post, err := EgressExecLines("celikrt-42.service")
	if err != nil {
		t.Fatal(err)
	}
	if len(pre) != 2 || pre[0] != "ExecStartPre=+/usr/sbin/nft -f /usr/lib/celikpanel-sandbox/egress.nft" ||
		pre[1] != `ExecStartPre=+/usr/sbin/nft add element inet celikpanel_egress smtp_blocked '{ "system.slice/celikrt-42.service" }'` {
		t.Fatalf("pre = %q", pre)
	}
	if len(post) != 1 || post[0] != `ExecStopPost=-+/usr/sbin/nft delete element inet celikpanel_egress smtp_blocked '{ "system.slice/celikrt-42.service" }'` {
		t.Fatalf("post = %q", post)
	}
	for _, l := range append(pre, post...) {
		if strings.ContainsAny(l, "$%") {
			t.Errorf("exec line with an expansion character: %q", l)
		}
	}
	for _, bad := range []string{"nginx.service", "celikrt-1", "celikrt-1.service; rm", "celikrt-0.service", "../celikrt-1.service"} {
		if _, _, err := EgressExecLines(bad); err == nil {
			t.Errorf("unit %q accepted", bad)
		}
	}
	guard := EgressGuardScriptText()
	if !strings.Contains(guard, "/system.slice/$unit") || !strings.Contains(guard, EgressFile) || strings.Contains(guard, "flush ruleset") {
		t.Fatalf("guard script:\n%s", guard)
	}
	if !strings.Contains(EgressGuardServiceText(), "ExecStart=/bin/sh "+EgressGuardScript) || !strings.Contains(EgressGuardTimerText(), "OnUnitActiveSec=30s") {
		t.Fatal("guard units")
	}
}

func TestProbeParsing(t *testing.T) {
	deb12 := "systemd 252 (252.30-1~deb12u2)\n+PAM +AUDIT +SELINUX +APPARMOR +IMA +SMACK +SECCOMP +GCRYPT -GNUTLS +OPENSSL +ACL +BLKID +CURL +ELFUTILS +FIDO2 +IDN2 -IDN +IPTC +KMOD +LIBCRYPTSETUP +LIBFDISK +PCRE2 -PWQUALITY +P11KIT +QRENCODE +TPM2 +BZIP2 +LZ4 +XZ +ZLIB +ZSTD -BPF_FRAMEWORK -XKBCOMMON +UTMP +SYSVINIT default-hierarchy=unified\n"
	if v, bpf := ParseSystemctlVersion(deb12); v != 252 || bpf {
		t.Fatalf("debian 12: %d %v", v, bpf)
	}
	deb13 := "systemd 257 (257.8-1~deb13u2)\n+PAM +AUDIT +SELINUX +APPARMOR +IMA +IPE +SMACK +SECCOMP +GCRYPT -GNUTLS +OPENSSL +ACL +BLKID +CURL +ELFUTILS +FIDO2 +IDN2 -IDN +IPTC +KMOD +LIBCRYPTSETUP +LIBCRYPTSETUP_PLUGINS +LIBFDISK +PCRE2 +PWQUALITY +P11KIT +QRENCODE +TPM2 +BZIP2 +LZ4 +XZ +ZLIB +ZSTD +BPF_FRAMEWORK +BTF -XKBCOMMON -UTMP +SYSVINIT +LIBARCHIVE\n"
	if v, bpf := ParseSystemctlVersion(deb13); v != 257 || !bpf {
		t.Fatalf("debian 13: %d %v", v, bpf)
	}
	if v, _ := ParseSystemctlVersion("something else\n+BPF_FRAMEWORK"); v != 0 {
		t.Fatalf("unknown output gave %d", v)
	}
	// The minus form is not the feature.
	if _, bpf := ParseSystemctlVersion("systemd 249 (249.11-0ubuntu3.12)\n-BPF_FRAMEWORK"); bpf {
		t.Fatal("-BPF_FRAMEWORK read as present")
	}

	set := "table inet celikpanel_egress {\n\tset smtp_blocked {\n\t\ttypeof socket cgroupv2 level 2\n\t\telements = { \"system.slice/celiksvc-12.service\",\n\t\t\t     \"system.slice/celikrt-3.service\", 4711 }\n\t}\n}\n"
	elements := ParseSetElements(set)
	if len(elements) != 3 || !InSet(elements, "celiksvc-12.service") || !InSet(elements, "celikrt-3.service") || InSet(elements, "celikrt-4.service") {
		t.Fatalf("elements = %v", elements)
	}
	if got := ParseSetElements("table inet celikpanel_egress {\n\tset smtp_blocked {\n\t\ttypeof socket cgroupv2 level 2\n\t}\n}\n"); got == nil || len(got) != 0 {
		t.Fatalf("an empty set = %v", got)
	}
	if ParseSetElements("Error: No such file or directory") != nil {
		t.Fatal("an error read as a set")
	}
	if n := ParseCounter("table inet celikpanel_egress {\n\tcounter smtp_rejects {\n\t\tpackets 7 bytes 420\n\t}\n}\n"); n != 7 {
		t.Fatalf("counter = %d", n)
	}
	if ParseCounter("") != -1 {
		t.Fatal("no counter read as a number")
	}
}

func showFor(p Profile, unit string, c Caps) string {
	var b strings.Builder
	for name, e := range Expected(p, c, unit) {
		value := e.value
		if e.contains {
			value = "{ x " + e.value + " y }"
		}
		b.WriteString(name + "=" + value + "\n")
	}
	return b.String()
}

func TestProofComparison(t *testing.T) {
	p := programProfile()
	c := fullCaps(257)
	unit := "celikrt-5.service"
	chain := "table inet celikpanel_egress {\n\tchain output {\n\t\ttcp dport { 25, 465, 587 } socket cgroupv2 level 2 @smtp_blocked counter name \"smtp_rejects\" reject with tcp reset\n\t}\n}\n"
	good := ProofInput{
		Unit: unit, Profile: p, Caps: c, Show: showFor(p, unit, c),
		SetElements: []string{"system.slice/celikrt-5.service"}, OutputChain: chain,
		Listen: &ListenProof{Serving: true},
	}
	if r := Prove(good); !r.OK || r.Code != ProofOK {
		t.Fatalf("good proof = %+v", r)
	}
	cases := map[string]struct {
		mutate func(*ProofInput)
		code   string
	}{
		"nothing read": {func(in *ProofInput) { in.Show = "" }, ProofUnreadable},
		"other cgroup": {func(in *ProofInput) {
			in.Show = strings.Replace(in.Show, "/system.slice/celikrt-5.service", "/my.slice/celikrt-5.service", 1)
		}, ProofCgroup},
		"owner dropped NNP": {func(in *ProofInput) {
			in.Show = strings.Replace(in.Show, "NoNewPrivileges=yes", "NoNewPrivileges=no", 1)
		}, ProofProperty},
		"memory raised": {func(in *ProofInput) {
			in.Show = strings.Replace(in.Show, "MemoryMax=536870912", "MemoryMax=infinity", 1)
		}, ProofProperty},
		"property missing": {func(in *ProofInput) { in.Show = strings.Replace(in.Show, "PrivatePIDs=yes\n", "", 1) }, ProofProperty},
		"not in set":       {func(in *ProofInput) { in.SetElements = []string{"system.slice/celikrt-6.service"} }, ProofNotInSet},
		"set unreadable":   {func(in *ProofInput) { in.SetElements = nil }, ProofUnreadable},
		"rule flushed":     {func(in *ProofInput) { in.OutputChain = "table inet celikpanel_egress {\n\tchain output {\n\t}\n}\n" }, ProofEgressMissing},
		"exposed":          {func(in *ProofInput) { in.Listen = &ListenProof{Serving: true, Exposed: []string{"0.0.0.0:30010"}} }, ProofListensElsewhere},
		"squatted":         {func(in *ProofInput) { in.Listen = &ListenProof{Squatted: true} }, ProofPortTaken},
		"silent":           {func(in *ProofInput) { in.Listen = &ListenProof{} }, ProofNotListening},
		"bad unit":         {func(in *ProofInput) { in.Unit = "sshd.service" }, ProofUnreadable},
	}
	for name, tc := range cases {
		in := good
		tc.mutate(&in)
		r := Prove(in)
		if r.OK || r.Code != tc.code {
			t.Errorf("%s: %+v, want %s", name, r, tc.code)
		}
	}
	// A host without nft cgroup support: no egress lines rendered, so none
	// proved (the reduced level names the gap).
	noNFT := good
	noNFT.Caps.NFTSocketCgroup = false
	noNFT.Show = showFor(p, unit, noNFT.Caps)
	noNFT.SetElements, noNFT.OutputChain = nil, ""
	if r := Prove(noNFT); !r.OK {
		t.Fatalf("proof without egress support = %+v", r)
	}
}

func TestListenProofStillDetectsExposure(t *testing.T) {
	listeners := ParseSS("LISTEN 0 511 127.0.0.1:30010 0.0.0.0:* users:((\"prog\",pid=10,fd=3))\nLISTEN 0 511 0.0.0.0:30010 0.0.0.0:* users:((\"prog\",pid=10,fd=4))\n")
	proof := ProveListening(listeners, map[int]bool{10: true}, 30010)
	if !proof.Serving || len(proof.Exposed) != 1 {
		t.Fatalf("proof = %+v", proof)
	}
}
