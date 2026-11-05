package main

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/systemd"
	"github.com/alicelik/celikpanel/internal/transport"
)

func TestUnitGlobPrefix(t *testing.T) {
	for pattern, want := range map[string]string{
		`^php[0-9]+\.[0-9]+-fpm$`: "php",
		`^postgresql@`:            "postgresql@",
		`^redis\.server$`:         "redis.server",
		`^php?-fpm`:               "ph",
		`^nginx*`:                 "ngin",
		`^a{0,1}b`:                "",
		`php-fpm`:                 "",
		`^php|nginx`:              "",
		`^.*fpm$`:                 "",
		`^(php|hhvm)`:             "",
	} {
		got, ok := unitGlobPrefix(pattern)
		if got != want || ok != (want != "") {
			t.Errorf("unitGlobPrefix(%q) = %q, %v; want %q", pattern, got, ok, want)
		}
	}
}

// Every unit name strict discovery and the runtime read can ask about is in
// the narrowed listing: a catalogue name exactly, and any name a catalogue
// regex can match by the regex's literal head. A name outside it would read
// as "not installed" and could close a firewall port a running service needs.
func TestInventoryUnitFilePatternsCoverTheCatalogue(t *testing.T) {
	patterns, ok := inventoryUnitFilePatterns()
	if !ok {
		t.Fatal("the catalogue's unit regexes must all have a literal head")
	}
	covered := func(name string) bool {
		for _, pattern := range patterns {
			if prefix, glob := strings.CutSuffix(pattern, "*"); glob && strings.HasPrefix(name+".service", prefix) {
				return true
			}
			if pattern == name+".service" {
				return true
			}
		}
		return false
	}
	for i := range core.ManagedServices {
		svc := &core.ManagedServices[i]
		for _, unit := range svc.SystemNames {
			lookup := unit
			if at := strings.IndexByte(unit, '@'); at >= 0 {
				lookup = unit[:at+1]
			}
			if !covered(lookup) {
				t.Errorf("%s: unit %s is not in the narrowed listing", svc.ID, lookup)
			}
		}
		if svc.SystemNamePattern != "" && !patternCoveredByInventory(svc.SystemNamePattern, patterns) {
			t.Errorf("%s: pattern %s is not covered", svc.ID, svc.SystemNamePattern)
		}
	}
	for _, example := range []string{"php8.4-fpm", "php8.5-fpm", "postgresql@17-main", "postgresql@"} {
		matched := false
		for i := range core.ManagedServices {
			if p := core.ManagedServices[i].SystemNamePattern; p != "" && regexp.MustCompile(p).MatchString(example) {
				matched = true
			}
		}
		if matched && !covered(example) {
			t.Errorf("%s matches a catalogue regex but is not in the narrowed listing", example)
		}
	}
	if !patternCoveredByInventory(phpFPMUnitPattern, patterns) {
		t.Fatal("the PHP-FPM instance pattern is not covered")
	}
	if patternCoveredByInventory(`^zz-not-in-catalogue`, patterns) {
		t.Fatal("an uncovered pattern was reported covered")
	}
}

// Golden answers of a batched `systemctl show` (systemd 252–258 print one
// blank line between units; a unit that does not exist still has its Id).
func TestParseShowBlocksGolden(t *testing.T) {
	output := "Id=nginx.service\nActiveState=active\nSubState=running\n\n" +
		"Id=named.service\nActiveState=inactive\nSubState=dead\n\n" +
		"Id=nothere.service\nActiveState=inactive\nSubState=dead\n"
	blocks, ok := systemd.ParseShowBlocks(output, 3)
	if !ok {
		t.Fatal("a well-formed answer was refused")
	}
	if got := firstValue(blocks[1], "Id"); got != "named.service" {
		t.Fatalf("second block = %q", got)
	}
	if got := firstValue(blocks[0], "SubState"); got != "running" {
		t.Fatalf("first block SubState = %q", got)
	}
	execStarts := "Id=a.service\nExecStart={ path=/usr/sbin/a ; argv[]=/usr/sbin/a -c /etc/a.conf ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }\nExecStart={ path=/bin/true ; argv[]=/bin/true ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }\n\nId=b.service\n"
	blocks, ok = systemd.ParseShowBlocks(execStarts, 2)
	if !ok || len(blocks[0]["ExecStart"]) != 2 || len(blocks[1]["ExecStart"]) != 0 {
		t.Fatalf("ExecStart blocks = %#v, %v", blocks, ok)
	}
	for name, bad := range map[string]struct {
		output string
		units  int
	}{
		"fewer blocks":     {"Id=a.service\n", 2},
		"more blocks":      {"Id=a.service\n\nId=b.service\n", 1},
		"block without id": {"Id=a.service\n\nActiveState=active\n", 2},
		"empty id":         {"Id=\n", 1},
		"two ids":          {"Id=a.service\nId=b.service\n", 1},
		"empty":            {"", 1},
		"garbage line":     {"Id=a.service\nnot a property\n", 1},
		"double blank":     {"Id=a.service\n\n\nId=b.service\n", 2},
	} {
		if _, ok := systemd.ParseShowBlocks(bad.output, bad.units); ok {
			t.Errorf("%s: a malformed answer was accepted", name)
		}
	}
}

type inventoryCall struct {
	executable string
	args       []string
}

// stubInventory turns the cache on with fixed fingerprints and records every
// systemctl call.
func stubInventory(t *testing.T, run func(args []string) ([]byte, error)) *[]inventoryCall {
	t.Helper()
	calls := &[]inventoryCall{}
	oldEnabled, oldCommand := inventoryCacheEnabled.Load(), inventoryCommand
	t.Cleanup(func() {
		inventoryCacheEnabled.Store(oldEnabled)
		inventoryCommand = oldCommand
		unitFileInventoryCache.reset()
		canonicalUnitCache.reset()
		packageInventoryCache.reset()
		hostProfileCache.reset()
	})
	unitFileInventoryCache.reset()
	canonicalUnitCache.reset()
	packageInventoryCache.reset()
	inventoryCacheEnabled.Store(false)
	inventoryCommand = func(_ context.Context, executable string, args ...string) ([]byte, error) {
		*calls = append(*calls, inventoryCall{executable, append([]string(nil), args...)})
		return run(args)
	}
	return calls
}

var testInventoryProfile = hostplatform.Profile{
	PackageManager: hostplatform.PackageManagerAPT,
	Executables:    map[string]string{"systemctl": "/usr/bin/systemctl", "dpkg-query": "/usr/bin/dpkg-query"},
}

func TestReadUnitFilesNarrowsAndFallsBackToTheFullListing(t *testing.T) {
	// The narrowed listing shows systemd's own unit: it is used.
	calls := stubInventory(t, func(args []string) ([]byte, error) {
		return []byte("nginx.service enabled enabled\nsystemd-journald.service static -\nphp8.4-fpm.service enabled enabled\n"), nil
	})
	inventory, err := readUnitFiles(context.Background(), testInventoryProfile)
	if err != nil || inventory.patterns == nil {
		t.Fatalf("narrowed listing = %+v, %v", inventory, err)
	}
	if _, ok := inventory.names["php8.4-fpm"]; !ok || len(*calls) != 1 {
		t.Fatalf("names = %v, calls = %d", inventory.names, len(*calls))
	}
	args := (*calls)[0].args
	if args[0] != "list-unit-files" || args[len(args)-1] != inventorySentinelUnit || !inventoryArgsContain(args, "nginx.service") || !inventoryArgsContain(args, "php*") {
		t.Fatalf("narrowed argv = %v", args)
	}

	// It does not show systemd's own unit (or fails): the full listing is
	// read instead, exactly the command used before.
	for name, narrowed := range map[string]func() ([]byte, error){
		"no sentinel": func() ([]byte, error) { return []byte("nginx.service enabled enabled\n"), nil },
		"failed":      func() ([]byte, error) { return nil, errors.New("exit status 1") },
	} {
		calls = stubInventory(t, func(args []string) ([]byte, error) {
			if len(args) > 4 {
				return narrowed()
			}
			return []byte("nginx.service enabled enabled\nredis.service alias -\n"), nil
		})
		inventory, err = readUnitFiles(context.Background(), testInventoryProfile)
		if err != nil || inventory.patterns != nil || len(*calls) != 2 {
			t.Fatalf("%s: inventory = %+v, err = %v, calls = %d", name, inventory, err, len(*calls))
		}
		if want := []string{"list-unit-files", "--type=service", "--no-legend", "--no-pager"}; !reflect.DeepEqual((*calls)[1].args, want) {
			t.Fatalf("%s: full argv = %v", name, (*calls)[1].args)
		}
	}

	// Both fail: an error, never an empty set (fail closed).
	stubInventory(t, func([]string) ([]byte, error) { return nil, errors.New("bus unavailable") })
	if _, err := readUnitFiles(context.Background(), testInventoryProfile); err == nil {
		t.Fatal("a failed listing was read as no unit files")
	}
}

func inventoryArgsContain(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func TestInventoryCacheServesOnlyAnUnchangedHost(t *testing.T) {
	calls := 0
	read := func() (int, error) { calls++; return calls, nil }
	var entry inventoryEntry[int]
	old := inventoryCacheEnabled.Load()
	inventoryCacheEnabled.Store(true)
	oldClock := inventoryClock
	now := time.Unix(1_800_000_000, 0)
	inventoryClock = func() time.Time { return now }
	t.Cleanup(func() { inventoryCacheEnabled.Store(old); inventoryClock = oldClock })

	if v, _ := entry.get("k", "fp1", read); v != 1 {
		t.Fatal("first read")
	}
	if v, _ := entry.get("k", "fp1", read); v != 1 {
		t.Fatal("an unchanged host was read again")
	}
	if v, _ := entry.get("k", "fp2", read); v != 2 {
		t.Fatal("a changed fingerprint (dpkg status, a unit directory) was not read again")
	}
	invalidateHostInventory()
	if v, _ := entry.get("k", "fp2", read); v != 3 {
		t.Fatal("a mutation's invalidation was not honoured")
	}
	now = now.Add(inventoryTTL)
	if v, _ := entry.get("k", "fp2", read); v != 4 {
		t.Fatal("a reading older than the TTL was served")
	}
	if v, _ := entry.get("k", "", read); v != 5 {
		t.Fatal("a host without a fingerprint was served from the cache")
	}
	if v, _ := entry.get("other", "fp2", read); v != 6 {
		t.Fatal("another key was served this key's reading")
	}
	failing := func() (int, error) { return 0, errors.New("failed") }
	if _, err := entry.get("k", "fp3", failing); err == nil {
		t.Fatal("an error was swallowed")
	}
	if v, _ := entry.get("k", "fp3", read); v != 7 {
		t.Fatal("an error was kept as a reading")
	}
	inventoryCacheEnabled.Store(false)
	if v, _ := entry.get("k", "fp3", read); v != 8 {
		t.Fatal("the cache served while turned off")
	}
}

func TestStrictProbeResolvesEveryPresentUnitWithOneCall(t *testing.T) {
	calls := stubInventory(t, func(args []string) ([]byte, error) {
		switch args[0] {
		case "list-unit-files":
			return []byte("nginx.service enabled enabled\nredis.service alias -\nvalkey.service enabled enabled\nsystemd-journald.service static -\n"), nil
		case "show":
			var out []string
			for _, unit := range args[3:] {
				id := unit
				if unit == "redis.service" {
					id = "valkey.service" // Arch: redis.service is valkey's alias
				}
				out = append(out, "Id="+id)
			}
			return []byte(strings.Join(out, "\n\n") + "\n"), nil
		}
		return nil, errors.New("unexpected")
	})
	probe := newStrictServiceStateProbe(testInventoryProfile)
	units, err := probe.UnitFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range []string{"nginx", "redis", "valkey"} {
		canonical, err := probe.CanonicalUnit(unit)
		if err != nil {
			t.Fatal(err)
		}
		want := unit
		if unit == "redis" {
			want = "valkey"
		}
		if canonical != want {
			t.Fatalf("%s resolved to %s", unit, canonical)
		}
	}
	shows := 0
	for _, call := range *calls {
		if call.args[0] == "show" {
			shows++
		}
	}
	if shows != 1 || len(units) != 4 {
		t.Fatalf("show calls = %d, units = %v", shows, units)
	}

	// An answer that cannot be split per unit: asked unit by unit; a failure
	// there is an error (fail closed).
	stubInventory(t, func(args []string) ([]byte, error) {
		switch {
		case args[0] == "list-unit-files":
			return []byte("nginx.service enabled enabled\nsystemd-journald.service static -\n"), nil
		case len(args) == 3 && args[1] == "-p":
			return []byte("Id=nginx.service\nId=nginx.service\n"), nil
		}
		return nil, errors.New("forced show failure")
	})
	probe = newStrictServiceStateProbe(testInventoryProfile)
	if _, err := probe.UnitFiles(); err != nil {
		t.Fatal(err)
	}
	if _, err := probe.CanonicalUnit("nginx"); err == nil || !strings.Contains(err.Error(), "forced show failure") {
		t.Fatalf("CanonicalUnit error = %v, want the unit-by-unit failure", err)
	}
}

func TestPackageInventoryViewsMatchTheHistoricQueries(t *testing.T) {
	rows := parseDpkgInventory(strings.Join([]string{
		"nginx\tnginx\tii \t1.26.3-3+deb13u1\tinstall ok installed",
		"libc6:amd64\tlibc6\tii \t2.41-12\tinstall ok installed",
		"redis-server\tredis-server\thi \t5:8.0.2-3\thold ok installed",
		"certbot\tcertbot\trc \t2.11.0-1\tdeinstall ok config-files",
		"postgresql-17\tpostgresql-17\tiU \t17.6-0+deb13u1\tinstall ok unpacked",
		"short\tline",
		"",
	}, "\n"))
	inventory := packageInventory{family: "apt", dpkg: rows}
	if got := parseDpkgVersions(inventory.versionsText()); len(got["nginx"]) != 1 || len(got["libc6"]) != 1 || len(got["redis-server"]) != 1 || got["certbot"] != nil {
		t.Fatalf("versions view = %#v", got)
	}
	strict := map[string]struct{}{}
	for _, line := range strings.Split(inventory.statusText(), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.Join(fields[len(fields)-3:], " ") == "install ok installed" {
			strict[strings.SplitN(fields[0], ":", 2)[0]] = struct{}{}
		}
	}
	if _, held := strict["redis-server"]; held || len(strict) != 2 {
		t.Fatalf("strict view = %v (a held package was never strict proof)", strict)
	}
	repo := installedPackagesMatchingPattern(inventory.abbrevText(), regexp.MustCompile(`postgresql-[0-9]+`))
	if !reflect.DeepEqual(repo, []string{"postgresql-17"}) {
		t.Fatalf("repository view = %v (an unpacked package is still repairable)", repo)
	}
}

func TestStrictProbeReadsTheSharedPackageQueryOnce(t *testing.T) {
	stubInventory(t, func([]string) ([]byte, error) { return nil, errors.New("no systemctl here") })
	queries := 0
	oldQuery := packageVersionQuery
	t.Cleanup(func() { packageVersionQuery = oldQuery })
	packageVersionQuery = func(_ context.Context, executable string, args ...string) ([]byte, error) {
		queries++
		if executable != "/usr/bin/dpkg-query" || !reflect.DeepEqual(args, dpkgInventoryArgs()) {
			t.Fatalf("query = %s %v", executable, args)
		}
		return []byte("nginx\tnginx\tii \t1.26.3-3\tinstall ok installed\nfail2ban\tfail2ban\thi \t1.1.0-8\thold ok installed\n"), nil
	}
	probe := newStrictServiceStateProbe(testInventoryProfile)
	packages, err := probe.InstalledPackages("apt")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := packages["nginx"]; !ok || len(packages) != 1 || queries != 1 {
		t.Fatalf("packages = %v, queries = %d", packages, queries)
	}
	packageVersionQuery = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("forced dpkg-query failure")
	}
	if _, err := newStrictServiceStateProbe(testInventoryProfile).InstalledPackages("apt"); err == nil {
		t.Fatal("a failed package query was read as no packages")
	}
}

func TestSplitAptPolicyBlocksAndBatchCandidates(t *testing.T) {
	output := "apache2:\n  Installed: (none)\n  Candidate: 2.4.65-2\n  Version table:\n     2.4.65-2 500\n        500 http://deb.debian.org/debian trixie/main amd64 Packages\n" +
		"exim4:\n  Installed: (none)\n  Candidate: (none)\n  Version table:\n" +
		"libc6:i386:\n  Installed: (none)\n  Candidate: 2.41-12\n"
	blocks := splitAptPolicyBlocks(output)
	if !strings.Contains(blocks["apache2"], "Candidate: 2.4.65-2") || !strings.Contains(blocks["exim4"], "(none)") || !strings.Contains(blocks["libc6"], "2.41-12") {
		t.Fatalf("blocks = %#v", blocks)
	}
	var services []*core.ManagedService
	for _, id := range []string{"apache", "exim", "nginx"} {
		if svc := core.GetManagedServiceByID(id); svc != nil && len(svc.Packages["apt"]) > 0 {
			services = append(services, svc)
		}
	}
	if len(services) < 2 {
		t.Skip("catalogue ids changed")
	}
	runs := 0
	reply := transport.ServiceCandidateVersionsResponse{Versions: map[string]string{}, Errors: map[string]string{}}
	candidateVersionsForServices(services, func(args ...string) ([]byte, error) {
		runs++
		if args[0] != "policy" || len(args) != len(services)+1 {
			t.Fatalf("argv = %v", args)
		}
		var out strings.Builder
		for _, svc := range services {
			pkg := svc.Packages["apt"][0]
			switch pkg {
			case "apache2":
				out.WriteString("apache2:\n  Installed: (none)\n  Candidate: 2.4.65-2\n")
			case "exim4", "exim4-daemon-light":
				out.WriteString(pkg + ":\n  Installed: (none)\n  Candidate: (none)\n")
			default:
				// A package apt does not know has no block at all.
			}
		}
		return []byte(out.String()), nil
	}, &reply)
	if runs != 1 {
		t.Fatalf("apt-cache ran %d times", runs)
	}
	for _, svc := range services {
		_, version := reply.Versions[svc.ID]
		_, failed := reply.Errors[svc.ID]
		if version == failed {
			t.Fatalf("%s: exactly one of version/error expected: %#v", svc.ID, reply)
		}
		if svc.Packages["apt"][0] == "apache2" && reply.Versions[svc.ID] != "2.4.65" {
			t.Fatalf("apache candidate = %q", reply.Versions[svc.ID])
		}
	}
}
