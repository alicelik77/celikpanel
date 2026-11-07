package sitesandbox

import (
	"reflect"
	"testing"
)

// Owner decision, 2026-11-05: user-uploaded programs, Python apps and
// service apps behave on Ubuntu 22.04 exactly as on Ubuntu 24.04. Both
// measured "reduced" in the alpha.123 lab (systemd built without BPF:
// 249 and 255; cgroup v2 and the nft egress table present). Nothing here is
// keyed to the distribution: the same capabilities give the same level, the
// same gaps, the same unit directives and the same proof.
func TestUbuntu2204SandboxIsTheSameAsUbuntu2404(t *testing.T) {
	ubuntu2204 := Caps{SystemdVersion: 249, BPFFramework: false, CgroupV2: true, NFTSocketCgroup: true}
	ubuntu2404 := Caps{SystemdVersion: 255, BPFFramework: false, CgroupV2: true, NFTSocketCgroup: true}
	a, b := Assess(ubuntu2204), Assess(ubuntu2404)
	if !reflect.DeepEqual(a, b) || a.Level != LevelReduced || !reflect.DeepEqual(a.Gaps, []Gap{GapBPF}) {
		t.Fatalf("22.04 %+v, 24.04 %+v", a, b)
	}
	for _, kind := range []Kind{KindProgram, KindPython, KindServiceApp, KindNode} {
		p := programProfile()
		p.Kind, p.Level = kind, LevelReduced
		for _, render := range []func(Profile, Caps) ([]string, error){Directives, TransientDirectives} {
			old, err := render(p, ubuntu2204)
			if err != nil {
				t.Fatalf("%s on 22.04: %v", kind, err)
			}
			current, err := render(p, ubuntu2404)
			if err != nil {
				t.Fatalf("%s on 24.04: %v", kind, err)
			}
			if !reflect.DeepEqual(old, current) {
				t.Fatalf("%s directives differ:\n22.04 %v\n24.04 %v", kind, old, current)
			}
		}
		if !reflect.DeepEqual(Expected(p, ubuntu2204, p.Unit), Expected(p, ubuntu2404, p.Unit)) {
			t.Fatalf("%s proof differs between 22.04 and 24.04", kind)
		}
	}
}
