package main

import (
	"encoding/json"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// TestTheUpdatesSiteMoveRefusalOnTheRealPath (alpha.150 third fix round, lab
// D5): the Agent's own refusal of the panel's update while a site is being
// moved, from its real handler through the real transport (net/rpc, gob, over
// a pipe), kept in web/tests/fixtures/real-path/agent-update-start-refusals.json
// as the panel receives it. The panel's test
// (cmd/panel/real_path_update_refusal_test.go) sends exactly that answer into
// its real admission and keeps the JSON the page receives; the web test feeds
// the page exactly that JSON. CELIKPANEL_WRITE_REAL_PATH=1 writes the fixture.
func TestTheUpdatesSiteMoveRefusalOnTheRealPath(t *testing.T) {
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", &Agent{}); err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	go server.ServeConn(serverConn)
	client := rpc.NewClient(clientConn)
	defer client.Close()

	siteMoveInFlight.Store(true)
	defer siteMoveInFlight.Store(false)
	var reply transport.SystemUpdateStartResponse
	if err := client.Call("Agent.StartSystemUpdate", &transport.SystemUpdateStartRequest{RequestID: strings.Repeat("b", 32)}, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Accepted || reply.Reason != transport.HostMutationReasonSiteMove || reply.Error == "" {
		t.Fatalf("%+v", reply)
	}

	path := filepath.Join("../../web/tests/fixtures/real-path", "agent-update-start-refusals.json")
	got, err := json.MarshalIndent(map[string]any{"site_move_running": reply}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("CELIKPANEL_WRITE_REAL_PATH") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want, have any
	if json.Unmarshal(raw, &want) != nil || json.Unmarshal(got, &have) != nil || !reflect.DeepEqual(want, have) {
		t.Fatalf("the Agent's answer on the real path is not the kept one (%s):\n%s", path, got)
	}
}
