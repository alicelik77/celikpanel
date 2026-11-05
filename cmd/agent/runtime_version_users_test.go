package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func useRuntimeUnitDirs(t *testing.T) (units, runtimes string) {
	t.Helper()
	units, runtimes = t.TempDir(), t.TempDir()
	oldDir, oldBase := appUnitDirOverride, runtimesBaseDir
	appUnitDirOverride, runtimesBaseDir = units, runtimes
	t.Cleanup(func() { appUnitDirOverride, runtimesBaseDir = oldDir, oldBase })
	return units, runtimes
}

func writeRuntimeUnitFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// n8n's unit runs on the panel's Node.js 24: removing that version is refused
// and names the unit; another version and an unrelated unit are not.
func TestRemoveNodeVersionRefusesWhileAServiceUnitRunsOnIt(t *testing.T) {
	units, runtimes := useRuntimeUnitDirs(t)
	node24 := filepath.Join(runtimes, "node", "24.11.0")
	node22 := filepath.Join(runtimes, "node", "22.20.0")
	writeRuntimeUnitFile(t, filepath.Join(node24, "bin", "node"), "")
	writeRuntimeUnitFile(t, filepath.Join(node22, "bin", "node"), "")
	writeRuntimeUnitFile(t, filepath.Join(units, "celiksvc-12.service"), "[Service]\nExecStart="+node24+"/bin/node /srv/n8n/node_modules/n8n/bin/n8n start\n")
	// A site unit is the Panel's own check, not this wall.
	writeRuntimeUnitFile(t, filepath.Join(units, "celikapp-7.service"), "[Service]\nExecStart="+node22+"/bin/node app.js\n")
	// A longer version that merely starts with the same text is not a match.
	writeRuntimeUnitFile(t, filepath.Join(units, "celikrt-3.service.d", "10-path.conf"), "[Service]\nEnvironment=PATH="+node24+"1/bin\n")

	var resp transport.NodeRemoveResponse
	if err := (&Agent{}).removeNodeVersion(&transport.NodeRemoveRequest{Version: "24.11.0"}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Removed || resp.ErrorCode != errCodeRuntimeUnitsBound || strings.Join(resp.Units, ",") != "celiksvc-12" {
		t.Fatalf("response = %+v", resp)
	}
	if _, err := os.Stat(node24); err != nil {
		t.Fatalf("Node.js 24 was deleted: %v", err)
	}

	resp = transport.NodeRemoveResponse{}
	if err := (&Agent{}).removeNodeVersion(&transport.NodeRemoveRequest{Version: "22.20.0"}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Removed || resp.Error != "" {
		t.Fatalf("Node.js 22 removal = %+v", resp)
	}

	var users transport.RuntimeVersionUsersResponse
	if err := (&Agent{}).RuntimeVersionUsersV1(&transport.RuntimeVersionUsersRequest{Runtime: "node", Version: "24.11.0"}, &users); err != nil {
		t.Fatal(err)
	}
	if strings.Join(users.Units, ",") != "celiksvc-12" || users.Error != "" {
		t.Fatalf("users = %+v", users)
	}
	// A drop-in that names the version counts too.
	writeRuntimeUnitFile(t, filepath.Join(units, "celikrt-5.service.d", "10-path.conf"), "Environment=PATH="+node24+"/bin:/usr/bin\n")
	if err := (&Agent{}).RuntimeVersionUsersV1(&transport.RuntimeVersionUsersRequest{Runtime: "node", Version: "24.11.0"}, &users); err != nil {
		t.Fatal(err)
	}
	if strings.Join(users.Units, ",") != "celikrt-5,celiksvc-12" {
		t.Fatalf("users with drop-in = %+v", users)
	}
	if err := (&Agent{}).RuntimeVersionUsersV1(&transport.RuntimeVersionUsersRequest{Runtime: "node", Version: "../x"}, &users); err != nil || users.Error == "" {
		t.Fatalf("bad version accepted: %+v", users)
	}
}
