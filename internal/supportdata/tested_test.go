package supportdata

import "testing"

func TestTheTestedVersionsFileNamesThisReleaseAndKnownLines(t *testing.T) {
	tested, err := LoadTested()
	if err != nil {
		t.Fatal(err)
	}
	if tested.Release == "" || tested.Evidence == "" {
		t.Fatalf("tested = %+v", tested)
	}
	table, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for distro, components := range tested.Tested {
		if _, ok := table.Distros[distro]; !ok {
			t.Fatalf("%s is not a distribution release of the support table", distro)
		}
		for component, lines := range components {
			for _, line := range lines {
				if _, ok := table.Components[component].Versions[line]; !ok {
					t.Fatalf("%s %s %s is not a version the support table knows", distro, component, line)
				}
			}
		}
	}
	if got := tested.For("ubuntu-24.04"); len(got) != 0 && got == nil {
		t.Fatal("For returned nil")
	}
}
