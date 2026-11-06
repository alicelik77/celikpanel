package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func stubServiceHealth(t *testing.T, answer func(args []string) ([]byte, error)) *[][]string {
	t.Helper()
	oldCommand, oldSystemctl := inventoryCommand, serviceHealthSystemctl
	t.Cleanup(func() { inventoryCommand, serviceHealthSystemctl = oldCommand, oldSystemctl })
	serviceHealthSystemctl = func() (string, error) { return "/usr/bin/systemctl", nil }
	calls := &[][]string{}
	inventoryCommand = func(_ context.Context, executable string, args ...string) ([]byte, error) {
		if executable != "/usr/bin/systemctl" {
			t.Fatalf("executable %q", executable)
		}
		*calls = append(*calls, args)
		return answer(args)
	}
	return calls
}

func TestServiceHealthReadsEveryUnitWithOneBatchedShow(t *testing.T) {
	calls := stubServiceHealth(t, func([]string) ([]byte, error) {
		return []byte(strings.Join([]string{
			"Id=nginx.service\nLoadState=loaded\nActiveState=active\nSubState=running\nActiveEnterTimestamp=@1730800000\nNRestarts=2\nMemoryCurrent=52428800\nCPUUsageNSec=1500000000",
			"Id=named.service\nLoadState=loaded\nActiveState=failed\nSubState=failed\nActiveEnterTimestamp=\nNRestarts=5\nMemoryCurrent=[not set]\nCPUUsageNSec=18446744073709551615",
		}, "\n\n") + "\n"), nil
	})
	var resp transport.ServiceHealthResponse
	if err := (&Agent{}).ServiceHealthV1(&transport.ServiceHealthRequest{Units: []string{"nginx", "named"}}, &resp); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0][0] != "show" || (*calls)[0][1] != "--timestamp=unix" ||
		(*calls)[0][len((*calls)[0])-1] != "named.service" {
		t.Fatalf("calls = %v", *calls)
	}
	if !resp.Batched || len(resp.Units) != 2 {
		t.Fatalf("resp = %+v", resp)
	}
	nginx, named := resp.Units[0], resp.Units[1]
	if nginx.ActiveSince != "2024-11-05T09:46:40Z" || *nginx.Restarts != 2 || *nginx.MemoryBytes != 52428800 || *nginx.CPUNanos != 1500000000 {
		t.Fatalf("nginx = %+v", nginx)
	}
	if named.ActiveState != "failed" || named.ActiveSince != "" || named.MemoryBytes != nil || named.CPUNanos != nil || *named.Restarts != 5 {
		t.Fatalf("named = %+v", named)
	}
}

func TestServiceHealthAsksAnOlderSystemdWithoutTheTimestampOption(t *testing.T) {
	calls := stubServiceHealth(t, func(args []string) ([]byte, error) {
		if args[1] == "--timestamp=unix" {
			return nil, errors.New("unknown option")
		}
		return []byte("Id=nginx.service\nLoadState=loaded\nActiveState=active\nSubState=running\nActiveEnterTimestamp=Tue 2026-11-05 10:00:00 UTC\nNRestarts=0\nMemoryCurrent=1\nCPUUsageNSec=1\n"), nil
	})
	var resp transport.ServiceHealthResponse
	if err := (&Agent{}).ServiceHealthV1(&transport.ServiceHealthRequest{Units: []string{"nginx"}}, &resp); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || !resp.Batched || resp.Units[0].ActiveSince != "2026-11-05T10:00:00Z" {
		t.Fatalf("calls %v resp %+v", *calls, resp)
	}
}

func TestServiceHealthRefusesBadNamesAndReportsAnUnsplittableAnswer(t *testing.T) {
	calls := stubServiceHealth(t, func([]string) ([]byte, error) { return []byte("Id=nginx.service\nLoadState=loaded\n"), nil })
	for _, units := range [][]string{{"--now"}, {"nginx.service"}, {"a b"}, {"nginx", "nginx"}} {
		var resp transport.ServiceHealthResponse
		if err := (&Agent{}).ServiceHealthV1(&transport.ServiceHealthRequest{Units: units}, &resp); err == nil {
			t.Fatalf("%v accepted", units)
		}
	}
	if len(*calls) != 0 {
		t.Fatal("a refused request ran systemctl")
	}
	var resp transport.ServiceHealthResponse
	if err := (&Agent{}).ServiceHealthV1(&transport.ServiceHealthRequest{Units: []string{"nginx", "named"}}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Batched || len(resp.Units) != 0 {
		t.Fatalf("two units answered by one block were read: %+v", resp)
	}
}
