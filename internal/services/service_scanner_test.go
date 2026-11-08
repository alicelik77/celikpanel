package services

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverDebianPostgreSQLPathsSupportsAnyMajorAndCluster(t *testing.T) {
	root := t.TempDir()
	want := []string{
		writePostgreSQLConfig(t, root, "18", "analytics", "postgresql.conf"),
		writePostgreSQLConfig(t, root, "18", "analytics", "pg_hba.conf"),
		writePostgreSQLConfig(t, root, "19", "main", "postgresql.conf"),
		writePostgreSQLConfig(t, root, "19", "main", "pg_hba.conf"),
	}

	got := discoverDebianPostgreSQLPaths(root, "postgresql")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverDebianPostgreSQLPaths() = %q, want %q", got, want)
	}
}

func TestDiscoverDebianPostgreSQLPathsRestrictsExactInstance(t *testing.T) {
	root := t.TempDir()
	want := []string{
		writePostgreSQLConfig(t, root, "18", "main", "postgresql.conf"),
		writePostgreSQLConfig(t, root, "18", "main", "pg_hba.conf"),
	}
	writePostgreSQLConfig(t, root, "17", "main", "postgresql.conf")
	writePostgreSQLConfig(t, root, "18", "other", "postgresql.conf")

	got := discoverDebianPostgreSQLPaths(root, "postgresql@18-main")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverDebianPostgreSQLPaths() = %q, want %q", got, want)
	}
}

func TestDiscoverDebianPostgreSQLPathsRejectsMalformedInstance(t *testing.T) {
	root := t.TempDir()
	writePostgreSQLConfig(t, root, "18", "main", "postgresql.conf")

	for _, serviceName := range []string{
		"postgresql@18-../../etc",
		"postgresql@18-",
		"postgresql@latest-main",
	} {
		t.Run(serviceName, func(t *testing.T) {
			if got := discoverDebianPostgreSQLPaths(root, serviceName); len(got) != 0 {
				t.Fatalf("discoverDebianPostgreSQLPaths(%q) = %q, want no paths", serviceName, got)
			}
		})
	}
}

func TestDiscoverDebianPostgreSQLPathsIgnoresNonRegularConfig(t *testing.T) {
	root := t.TempDir()
	clusterPath := filepath.Join(root, "18", "main")
	if err := os.MkdirAll(filepath.Join(clusterPath, "postgresql.conf"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := discoverDebianPostgreSQLPaths(root, "postgresql@18-main"); len(got) != 0 {
		t.Fatalf("discoverDebianPostgreSQLPaths() = %q, want no paths", got)
	}
}

func TestDiscoverDebianPostgreSQLPathsRejectsSymlinkedCluster(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "postgresql.conf"), []byte("# outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	majorPath := filepath.Join(root, "18")
	if err := os.MkdirAll(majorPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(majorPath, "main")); err != nil {
		t.Skipf("symlinks are unavailable on this test host: %v", err)
	}

	if got := discoverDebianPostgreSQLPaths(root, "postgresql@18-main"); len(got) != 0 {
		t.Fatalf("discoverDebianPostgreSQLPaths() = %q, want no paths", got)
	}
}

func writePostgreSQLConfig(t *testing.T, root, major, cluster, name string) string {
	t.Helper()
	installPostgreSQLMajor(t, major)
	path := filepath.Join(root, major, cluster, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// installPostgreSQLMajor gives the test a server program for major in a
// temporary postgresqlBinRoot (one per test).
func installPostgreSQLMajor(t *testing.T, major string) {
	t.Helper()
	if !postgresqlBinRootIsTemp[t.Name()] {
		old := postgresqlBinRoot
		postgresqlBinRoot = t.TempDir()
		postgresqlBinRootIsTemp[t.Name()] = true
		t.Cleanup(func() { postgresqlBinRoot = old; delete(postgresqlBinRootIsTemp, t.Name()) })
	}
	bin := filepath.Join(postgresqlBinRoot, major, "bin", "postgres")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/true"), 0o755); err != nil {
		t.Fatal(err)
	}
}

var postgresqlBinRootIsTemp = map[string]bool{}

// alpha.130 (Boston, 8 Nov 2026): the files of a removed major (its program
// gone, pg_lsclusters "binaries_missing") are never offered for editing;
// the cluster PostgreSQL runs is.
func TestDiscoverDebianPostgreSQLPathsSkipsRemovedMajor(t *testing.T) {
	root := t.TempDir()
	want := []string{
		writePostgreSQLConfig(t, root, "18", "main", "postgresql.conf"),
		writePostgreSQLConfig(t, root, "18", "main", "pg_hba.conf"),
	}
	writePostgreSQLConfig(t, root, "16", "main", "postgresql.conf")
	writePostgreSQLConfig(t, root, "16", "main", "pg_hba.conf")
	if err := os.RemoveAll(filepath.Join(postgresqlBinRoot, "16")); err != nil {
		t.Fatal(err)
	}

	if got := discoverDebianPostgreSQLPaths(root, "postgresql"); !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverDebianPostgreSQLPaths() = %q, want only 18/main %q", got, want)
	}
	if got := discoverDebianPostgreSQLPaths(root, "postgresql@16-main"); len(got) != 0 {
		t.Fatalf("discoverDebianPostgreSQLPaths(postgresql@16-main) = %q, want nothing", got)
	}
}
