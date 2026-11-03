package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/transport"
)

func fileAccessState(t *testing.T) transport.FileAccessStateResponse {
	t.Helper()
	var state transport.FileAccessStateResponse
	if err := (&Agent{}).FileAccessStateV1(&transport.FileAccessStateRequest{Site: exampleSite}, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

// The Access view names the missing tools and their packages, read only.
func TestFileAccessStateReportsMissingJailTools(t *testing.T) {
	host := installFileAccessFakeHost(t)
	host.missing = []string{"zip", "unzip", "rsync"}
	state := fileAccessState(t)
	server := state.Server
	if !server.JailChecked || !reflect.DeepEqual(server.JailMissing, []string{"zip", "unzip", "rsync"}) ||
		!reflect.DeepEqual(server.JailPackages, []string{"rsync", "unzip", "zip"}) || len(server.JailUnknown) != 0 || server.PackageFamily != "apt" {
		t.Fatalf("server %+v", server)
	}
	for _, command := range host.commands {
		if strings.HasPrefix(command, "write") || strings.HasPrefix(command, "usermod") || strings.HasPrefix(command, "remove") || command == "jail" {
			t.Fatalf("reading the state changed the host: %v", host.commands)
		}
	}
}

// Turning the command line on is refused while tools are missing; nothing
// on the host changes. FTP and SFTP alone still open.
func TestFileAccessSSHRefusedWhileJailToolsMissing(t *testing.T) {
	host := installFileAccessFakeHost(t)
	host.missing = []string{"zip", "unzip"}
	response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true, SSH: true})
	if response.Applied || response.Refusal != transport.FileAccessRefusedJailTools ||
		!strings.Contains(response.Detail, "zip, unzip") || !strings.Contains(response.Detail, "unzip, zip") {
		t.Fatalf("response %+v", response)
	}
	if host.index("write") >= 0 || host.jailCalls != 0 || host.index("usermod") >= 0 {
		t.Fatalf("a refused change touched the host: %v", host.commands)
	}
	files := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true})
	if !files.Applied || files.Refusal != "" {
		t.Fatalf("FTP/SFTP refused: %+v", files)
	}
}

// An account that already has the command line keeps it when tools go
// missing later; its saves still work and report the missing tools.
func TestFileAccessExistingSSHAccountKeepsTheCommandLine(t *testing.T) {
	host := installFileAccessFakeHost(t)
	if first := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true, SSH: true}); !first.Applied {
		t.Fatalf("first %+v", first)
	}
	host.missing = []string{"zip"}
	again := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true, SSH: true, Password: "a long secret"})
	if !again.Applied || again.Refusal != "" || !again.State.Account.SSH {
		t.Fatalf("existing SSH account cut off: %+v", again)
	}
	if !reflect.DeepEqual(again.State.Server.JailMissing, []string{"zip"}) {
		t.Fatalf("state %+v", again.State.Server)
	}
}

func installFakeJailPackageInstall(t *testing.T, host *fileAccessFakeHost, provides map[string][]string) *[][]string {
	t.Helper()
	var calls [][]string
	saved := fileAccessInstallPackages
	t.Cleanup(func() { fileAccessInstallPackages = saved })
	fileAccessInstallPackages = func(_ context.Context, family string, packages []string) (string, error) {
		if family != "apt" {
			t.Fatalf("family %s", family)
		}
		calls = append(calls, append([]string(nil), packages...))
		host.mu.Lock()
		defer host.mu.Unlock()
		var still []string
		for _, command := range host.missing {
			provided := false
			for _, pkg := range packages {
				for _, c := range provides[pkg] {
					provided = provided || c == command
				}
			}
			if !provided {
				still = append(still, command)
			}
		}
		host.missing = still
		return "", nil
	}
	return &calls
}

// The install installs exactly the packages that provide the missing tools,
// inside its own job, and nothing when the list changed or nothing is
// missing any more.
func TestInstallJailToolsInstallsOnlyTheListedPackages(t *testing.T) {
	host := installFileAccessFakeHost(t)
	host.missing = []string{"zip", "unzip", "gunzip"}
	calls := installFakeJailPackageInstall(t, host, map[string][]string{"zip": {"zip"}, "unzip": {"unzip"}, "gzip": {"gzip", "gunzip", "zcat"}})
	manager, _ := newMutationTestManager(t)
	installGlobalMutationTestManager(t, manager)
	binding := ServiceMutationBinding{MutationRequestID: testMutationRequestID, MutationOwnerID: testMutationOwnerID}

	// A list other than what is missing installs nothing.
	var changed transport.InstallJailToolsResponse
	if err := (&Agent{}).InstallJailToolsV1(&transport.InstallJailToolsRequest{ServiceMutationBinding: binding, Packages: []string{"zip"}}, &changed); err != nil {
		t.Fatal(err)
	}
	if changed.ErrorCode != transport.JailToolsChanged || len(*calls) != 0 {
		t.Fatalf("changed %+v calls %v", changed, *calls)
	}
	// A package outside the mapping is refused.
	var foreign transport.InstallJailToolsResponse
	_ = (&Agent{}).InstallJailToolsV1(&transport.InstallJailToolsRequest{ServiceMutationBinding: binding, Packages: []string{"gcc", "gzip", "unzip", "zip"}}, &foreign)
	if foreign.ErrorCode != transport.JailToolsChanged || len(*calls) != 0 {
		t.Fatalf("foreign %+v calls %v", foreign, *calls)
	}
	// Without the operation's job the step is refused.
	var unbound transport.InstallJailToolsResponse
	_ = (&Agent{}).InstallJailToolsV1(&transport.InstallJailToolsRequest{ServiceMutationBinding: binding, Packages: []string{"zip", "unzip", "gzip"}}, &unbound)
	if unbound.ErrorCode != transport.JailToolsMutation || len(*calls) != 0 {
		t.Fatalf("unbound %+v calls %v", unbound, *calls)
	}

	beginMutationTestJobWithIdentity(t, manager, fileAccessToolsKind, fileAccessToolsTarget, "gzip,unzip,zip")
	var done transport.InstallJailToolsResponse
	if err := (&Agent{}).InstallJailToolsV1(&transport.InstallJailToolsRequest{ServiceMutationBinding: binding, Packages: []string{"zip", "unzip", "gzip"}}, &done); err != nil {
		t.Fatal(err)
	}
	if done.ErrorCode != "" || !reflect.DeepEqual(*calls, [][]string{{"gzip", "unzip", "zip"}}) ||
		!reflect.DeepEqual(done.Installed, []string{"gzip", "unzip", "zip"}) || len(done.StillMissing) != 0 {
		t.Fatalf("done %+v calls %v", done, *calls)
	}
	// The same request again (a lost answer) finds nothing missing.
	var repeat transport.InstallJailToolsResponse
	_ = (&Agent{}).InstallJailToolsV1(&transport.InstallJailToolsRequest{ServiceMutationBinding: binding, Packages: []string{"zip", "unzip", "gzip"}}, &repeat)
	if repeat.ErrorCode != "" || len(repeat.Installed) != 0 || len(*calls) != 1 {
		t.Fatalf("repeat %+v calls %v", repeat, *calls)
	}
	// And the command line can now be turned on.
	if response := fileAccessApply(t, transport.ApplyFileAccessRequest{Site: exampleSite, Enabled: true, SSH: true}); !response.Applied {
		t.Fatalf("after install %+v", response)
	}
}

func TestInstallJailToolsStepNeedsItsOwnJob(t *testing.T) {
	claim := newServiceMutationStepClaim(serviceMutationStepInstallJailTools, fileAccessToolsTarget, "unzip,zip", "install")
	own := &ServiceMutationJob{Kind: fileAccessToolsKind, Target: fileAccessToolsTarget, PackageName: "unzip,zip"}
	if err := authorizeServiceMutationStep(own, claim); err != nil {
		t.Fatal(err)
	}
	for _, job := range []*ServiceMutationJob{
		{Kind: fileAccessToolsKind, Target: fileAccessToolsTarget, PackageName: "zip"},
		{Kind: "service_install", Target: fileAccessToolsTarget, PackageName: "unzip,zip"},
		{Kind: "service_install", Target: "nginx"},
	} {
		if err := authorizeServiceMutationStep(job, claim); err == nil {
			t.Fatalf("job %+v authorized the jail tools install", job)
		}
	}
}

func TestJailCompanionsAreNotJailCommands(t *testing.T) {
	for command, companions := range fileaccess.JailCompanions {
		for _, companion := range companions {
			for _, listed := range fileaccess.JailCommands {
				if listed == companion {
					t.Fatalf("%s's companion %s is itself a jail command", command, companion)
				}
			}
		}
	}
}
