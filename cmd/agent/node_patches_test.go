package main

import "testing"

// alpha.107 (D-038 §9): the newest release on each installed Node.js line.
func TestTheNewestPatchOnEachInstalledLineIsSaid(t *testing.T) {
	index := []byte(`[
		{"version":"v24.22.1","lts":"Krypton"},
		{"version":"v22.23.4","lts":"Jod"},
		{"version":"v22.14.0","lts":"Jod"},
		{"version":"v25.9.0","lts":false},
		{"version":"v20.20.2","lts":"Iron"}
	]`)
	patches, err := nodePatchesFrom(index, []string{"22.14.0", "24.22.1", "19.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if patches[0].Newest != "22.23.4" || !patches[0].Newer || patches[0].LTS != "Jod" {
		t.Fatalf("22 = %+v", patches[0])
	}
	if patches[1].Newer || patches[1].Newest != "24.22.1" {
		t.Fatalf("24 = %+v", patches[1])
	}
	if patches[2].Newest != "" || patches[2].Newer {
		t.Fatalf("19 = %+v", patches[2])
	}
	if !nodeVersionLess("22.9.0", "22.10.0") || nodeVersionLess("22.10.0", "22.9.0") {
		t.Fatal("order")
	}
}
