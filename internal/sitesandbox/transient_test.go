package sitesandbox

import (
	"strings"
	"testing"
)

// TransientSystemdVersions are the systemd versions the transient property
// lists are pinned against: Ubuntu 22.04 (249), Debian 12 (252), Ubuntu
// 24.04 (255), 256, Debian 13 (257), 258 and Arch (262).
var transientSystemdVersions = []int{249, 252, 255, 256, 257, 258, 262}

// The alpha.119 lab round 2 defect: systemd 257's systemd-run refuses
// ProtectControlGroups=strict as a --property, so a transient unit with the
// unit-file form never started. Every kind's transient form parses on every
// pinned version, and only the property name differs from the unit file.
func TestTransientDirectivesParseOnEverySystemd(t *testing.T) {
	python := programProfile()
	python.Kind, python.Unit = KindPython, "celikrtbuild-5.service"
	python.Port = 0
	service := programProfile()
	service.Kind, service.Unit, service.ExtraPorts = KindServiceApp, "celiksvcinst-5.service", nil
	service.Port = 0
	for _, version := range transientSystemdVersions {
		for _, caps := range []Caps{fullCaps(version), {SystemdVersion: version, CgroupV2: true}} {
			for _, p := range []Profile{programProfile(), python, service} {
				p.Level = Assess(caps).Level
				lines, err := TransientDirectives(p, caps)
				if err != nil {
					t.Fatalf("%d %s: %v", version, p.Kind, err)
				}
				unitLines, pre, post, err := HelperLines(p, caps)
				if err != nil {
					t.Fatalf("%d %s: %v", version, p.Kind, err)
				}
				for _, line := range append(append(append(lines, unitLines...), pre...), post...) {
					if why := TransientPropertyRefusal(line, version); why != "" {
						t.Errorf("systemd %d, %s: --property=%s: %s", version, p.Kind, line, why)
					}
				}
				file, _ := Directives(p, caps)
				if len(file) != len(lines) {
					t.Fatalf("%d %s: the transient form has %d lines, the unit file %d", version, p.Kind, len(lines), len(file))
				}
				for i := range file {
					if file[i] != lines[i] && !(file[i] == "ProtectControlGroups=strict" && lines[i] == "ProtectControlGroupsEx=strict") {
						t.Errorf("%d %s: %q became %q", version, p.Kind, file[i], lines[i])
					}
				}
			}
		}
	}
	// The table itself refuses what systemd 257's systemd-run refused.
	if TransientPropertyRefusal("ProtectControlGroups=strict", 257) == "" || TransientPropertyRefusal("PrivatePIDs=yes", 255) == "" ||
		TransientPropertyRefusal("NoSuchSetting=yes", 262) == "" || TransientPropertyRefusal("ProtectControlGroupsEx=strict", 257) != "" {
		t.Fatal("the transient table does not hold what systemd-run was seen to do")
	}
	lines, _ := TransientDirectives(programProfile(), fullCaps(257))
	if !has(lines, "ProtectControlGroupsEx=strict") || has(lines, "ProtectControlGroups=") {
		t.Fatalf("257 transient: %s", strings.Join(lines, "\n"))
	}
}
