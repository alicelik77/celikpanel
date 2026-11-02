package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// fakeAppUnitHost is systemd for one app unit.
type fakeAppUnitHost struct {
	present, active, enabled bool
	calls                    []string
}

func (h *fakeAppUnitHost) systemctl(args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "--user" {
		args = args[1:]
	}
	h.calls = append(h.calls, strings.Join(args, " "))
	switch args[0] {
	case "show":
		if !h.present {
			return []byte("LoadState=not-found\nActiveState=inactive\nUnitFileState=\n"), nil
		}
		active, file := "inactive", "disabled"
		if h.active {
			active = "active"
		}
		if h.enabled {
			file = "enabled"
		}
		return []byte("LoadState=loaded\nActiveState=" + active + "\nUnitFileState=" + file + "\n"), nil
	case "stop":
		h.active = false
	case "start":
		h.active = true
	case "disable":
		h.enabled = false
	case "enable":
		h.enabled = true
	}
	return nil, nil
}

func useFakeAppUnitHost(t *testing.T, host *fakeAppUnitHost) {
	t.Helper()
	old := appUnitSystemctl
	appUnitSystemctl = host.systemctl
	t.Cleanup(func() { appUnitSystemctl = old })
}

func setAppSuspension(t *testing.T, suspend bool, before transport.AppUnitState) transport.SetAppUnitSuspensionResponse {
	t.Helper()
	var resp transport.SetAppUnitSuspensionResponse
	if err := (&Agent{}).SetAppUnitSuspensionV1(&transport.SetAppUnitSuspensionRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), SiteID: 7, Suspend: suspend, Before: before,
	}, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAppUnitSuspensionStopsAndRestoresExactly(t *testing.T) {
	host := &fakeAppUnitHost{present: true, active: true, enabled: true}
	useFakeAppUnitHost(t, host)
	before, err := inspectSuspensionAppUnit(7)
	if err != nil || before != (transport.AppUnitState{Present: true, Active: true, Enabled: true}) {
		t.Fatalf("before = %+v, %v", before, err)
	}
	if r := setAppSuspension(t, true, before); !r.Applied || !r.Changed || host.active || host.enabled {
		t.Fatalf("suspend = %+v, host %+v", r, host)
	}
	if r := setAppSuspension(t, true, before); !r.Applied || r.Changed {
		t.Fatalf("second suspend = %+v", r)
	}
	if r := setAppSuspension(t, false, before); !r.Applied || !r.Changed || !host.active || !host.enabled {
		t.Fatalf("resume = %+v, host %+v", r, host)
	}
	if r := setAppSuspension(t, false, before); !r.Applied || r.Changed {
		t.Fatalf("second resume = %+v", r)
	}
}

func TestAppUnitSuspensionKeepsAStoppedAppStopped(t *testing.T) {
	// Stopped but enabled before: resume enables it, does not start it.
	host := &fakeAppUnitHost{present: true, enabled: true}
	useFakeAppUnitHost(t, host)
	before, _ := inspectSuspensionAppUnit(7)
	setAppSuspension(t, true, before)
	if r := setAppSuspension(t, false, before); !r.Applied || host.active || !host.enabled {
		t.Fatalf("resume = %+v, host %+v", r, host)
	}
	for _, call := range host.calls {
		if strings.HasPrefix(call, "start") {
			t.Fatalf("a stopped app was started: %v", host.calls)
		}
	}
}

func TestAppUnitSuspensionOwnerChangeAndAbsence(t *testing.T) {
	host := &fakeAppUnitHost{present: true, active: true}
	useFakeAppUnitHost(t, host)
	before, _ := inspectSuspensionAppUnit(7)
	setAppSuspension(t, true, before)
	// The owner enabled it while suspended: left as they made it.
	host.enabled = true
	if r := setAppSuspension(t, false, before); !r.Conflict || r.Applied || host.active {
		t.Fatalf("owner change = %+v, host %+v", r, host)
	}
	host.present = false
	if r := setAppSuspension(t, true, before); !strings.HasPrefix(r.Error, "app unit is absent") || r.Applied {
		t.Fatalf("absent = %+v", r)
	}
}
