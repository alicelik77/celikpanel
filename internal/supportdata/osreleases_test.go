package supportdata

import "testing"

func TestTheSupportedReleasesAreReadAndFound(t *testing.T) {
	list, err := LoadOSReleases()
	if err != nil {
		t.Fatal(err)
	}
	table, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, release := range list.Releases {
		if release.DistroKey != "" {
			if _, ok := table.Distros[release.DistroKey]; !ok {
				t.Fatalf("%s names %s, which the support table does not date", release.Name, release.DistroKey)
			}
		}
	}
	for _, c := range []struct{ id, version, level string }{
		{"debian", "13", LevelTested}, {"debian", "13.1", LevelTested}, {"ubuntu", "24.04", LevelTested},
		{"debian", "12", LevelAccepted}, {"ubuntu", "22.04", LevelAccepted},
		{"arch", "", LevelDevelopment}, {"rocky", "9.4", LevelPreview},
	} {
		release, ok := list.Find(c.id, c.version)
		if !ok || release.Level != c.level {
			t.Fatalf("%s %s = %+v %v", c.id, c.version, release, ok)
		}
	}
	for _, c := range [][2]string{{"debian", "14"}, {"ubuntu", "24.10"}, {"fedora", "41"}} {
		if release, ok := list.Find(c[0], c[1]); ok {
			t.Fatalf("%v should not be listed: %+v", c, release)
		}
	}
}
