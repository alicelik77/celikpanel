package core

import "testing"

// A runtime without a distribution package in any family is installed by
// version through its own path; everything else keeps the package install.
func TestInstalledByVersionFollowsTheCatalogue(t *testing.T) {
	want := map[string]bool{"node": true, "python": false, "php-fpm": false, "nginx": false, "roundcube": false}
	for id, expected := range want {
		svc := GetManagedServiceByID(id)
		if svc == nil {
			t.Fatalf("%s is not in the catalogue", id)
		}
		if got := svc.InstalledByVersion(); got != expected {
			t.Errorf("%s: InstalledByVersion() = %v, want %v", id, got, expected)
		}
	}
	byVersion := []string{}
	for i := range ManagedServices {
		if ManagedServices[i].InstalledByVersion() {
			byVersion = append(byVersion, ManagedServices[i].ID)
		}
	}
	if len(byVersion) != 1 || byVersion[0] != "node" {
		t.Fatalf("components installed by version = %v; a new one needs its own install path and words on the screen", byVersion)
	}
	var none *ManagedService
	if none.InstalledByVersion() {
		t.Fatal("a nil entry is not installed by version")
	}
}
