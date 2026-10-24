package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// alpha.107 (D-038 §8): a database engine is removed with apt's "remove",
// never "purge": its clusters, databases and configuration stay, and the
// dialogue says where. An install that finds the data again says so.

func TestADatabaseEngineIsRemovedNotPurged(t *testing.T) {
	if !removalKeepsData(core.GetManagedServiceByID("postgresql")) || !removalKeepsData(core.GetManagedServiceByID("mariadb")) || removalKeepsData(core.GetManagedServiceByID("redis")) {
		t.Fatal("which components keep their data")
	}
	var calls [][]string
	run := func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{}, args...))
		joined := strings.Join(args, " ")
		switch {
		case strings.HasPrefix(joined, "-s autoremove"):
			return []byte(""), nil
		case strings.HasPrefix(joined, "-s remove"):
			return []byte("Remv postgresql-17 [17.6-0+deb13u1]\n"), nil
		}
		t.Fatalf("unexpected %q", joined)
		return nil, nil
	}
	request := packageRemovalRequest{Packages: []string{"postgresql-17"}, VersionPrefixes: []string{"postgresql-17"}, KeepData: true}
	plan, err := planAptRemoval(context.Background(), run, "/usr/bin/apt-get", request)
	if err != nil || !plan.KeepData || !reflect.DeepEqual(plan.Remove, []string{"postgresql-17"}) {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	for _, call := range calls {
		if strings.Contains(strings.Join(call, " "), "purge") && call[1] != "autoremove" {
			t.Fatalf("a keep-data removal simulated a purge: %v", call)
		}
	}
	var mutated []string
	executor := packageRemovalExecutor{family: "apt", tool: "/usr/bin/apt-get", run: run, mutate: func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
		mutated = args
		return nil, nil
	}}
	if _, err := executor.remove(context.Background(), plan); err != nil || mutated[0] != "remove" {
		t.Fatalf("real removal = %v, %v", mutated, err)
	}
	if got := aptFinalRemovalArgsFor([]string{"redis-server"}, false)[0]; got != "purge" {
		t.Fatalf("an ordinary removal is %q", got)
	}
}

func TestTheDataThatStaysIsNamedAndFoundAgain(t *testing.T) {
	root := t.TempDir()
	old := dataRoots
	dataRoots = map[string]map[string]string{
		"postgresql": {"apt": filepath.Join(root, "postgresql")},
		"mariadb":    {"apt": filepath.Join(root, "mysql")},
	}
	t.Cleanup(func() { dataRoots = old })
	os.MkdirAll(filepath.Join(root, "postgresql", "17", "main"), 0o700)
	os.MkdirAll(filepath.Join(root, "postgresql", "16"), 0o700)
	os.MkdirAll(filepath.Join(root, "mysql", "mysql"), 0o700)
	pg := core.GetManagedServiceByID("postgresql")
	if got := removalDataPaths(pg, "apt", "postgresql-17"); len(got) != 1 || !strings.HasSuffix(got[0], filepath.Join("postgresql", "17")) {
		t.Fatalf("one major: %v", got)
	}
	if got := removalDataPaths(pg, "apt", ""); len(got) != 1 {
		t.Fatalf("every major with data (16 is empty): %v", got)
	}
	if got := existingEngineData(core.GetManagedServiceByID("mariadb"), "apt", ""); len(got) != 1 || got[0].Component != "mariadb" {
		t.Fatalf("mariadb: %+v", got)
	}
	if got := removalDataPaths(pg, "apt", "postgresql-18"); len(got) != 0 {
		t.Fatalf("absent major: %v", got)
	}
}
