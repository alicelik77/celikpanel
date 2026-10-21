package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/transport"
)

type packageVersionCall struct {
	executable string
	args       []string
}

// stubPackageVersionRead replaces the host profile, the busy probe and the
// query runner for one test and records every query.
func stubPackageVersionRead(t *testing.T, family hostplatform.PackageManager, busy func() (bool, error), query func(context.Context) ([]byte, error)) *[]packageVersionCall {
	t.Helper()
	calls := &[]packageVersionCall{}
	oldProfile, oldBusy, oldQuery := packageVersionHostProfile, packageVersionBusyProbe, packageVersionQuery
	t.Cleanup(func() {
		packageVersionHostProfile, packageVersionBusyProbe, packageVersionQuery = oldProfile, oldBusy, oldQuery
	})
	packageVersionHostProfile = func() (hostplatform.Profile, error) {
		return hostplatform.Profile{
			PackageManager: family,
			Executables:    map[string]string{"dpkg-query": "/usr/bin/dpkg-query", "pacman": "/usr/bin/pacman"},
		}, nil
	}
	packageVersionBusyProbe = busy
	packageVersionQuery = func(ctx context.Context, executable string, args ...string) ([]byte, error) {
		*calls = append(*calls, packageVersionCall{executable, append([]string(nil), args...)})
		return query(ctx)
	}
	return calls
}

func idle() (bool, error) { return false, nil }

func versionsOf(answer transport.InstalledPackageVersionsResponse, id string) []transport.InstalledPackageVersion {
	for _, component := range answer.Components {
		if component.ServiceID == id {
			return component.Packages
		}
	}
	return nil
}

func TestParseDpkgVersions(t *testing.T) {
	output := strings.Join([]string{
		"nginx\tii \t1.26.3-3+deb13u1",
		"mariadb-server\tii \t1:10.11.13-0+deb13u1",
		"certbot\trc \t2.11.0-1",                // removed, config left: not installed
		"postfix\tiU \t3.10.4-1~deb13u1",        // unpacked, not configured: no version
		"libc6:amd64\tii \t2.41-12",             // architecture qualifier dropped
		"libc6:i386\tii \t2.41-11",              // a second architecture keeps its own version
		"odd\tii \t1.0$(touch x)",               // characters no package database prints
		"long\tii \t" + strings.Repeat("1", 65), // longer than the ceiling
		"redis-server\thi \t5:8.0.2-3",          // held: still installed
		"fail2ban\tri \t1.1.0-8",                // marked for removal: still installed
		"bind9\tpi \t1:9.20.11-4+deb13u1",       // marked for purge: still installed
		"exim4-daemon-light\tiiR\t4.98-1",       // reinstall required: no version
		"nftables\tiH \t1.1.3-1",                // half-installed: no version
		"dovecot-core\tii\t1:2.4.1+dfsg1-6",     // the blank error flag lost: installed
		"short-line",
		"",
	}, "\n")
	got := parseDpkgVersions(output)
	want := map[string][]string{
		"nginx":          {"1.26.3-3+deb13u1"},
		"mariadb-server": {"1:10.11.13-0+deb13u1"},
		"libc6":          {"2.41-12", "2.41-11"},
		"odd":            {""},
		"long":           {""},
		"redis-server":   {"5:8.0.2-3"},
		"fail2ban":       {"1.1.0-8"},
		"bind9":          {"1:9.20.11-4+deb13u1"},
		"dovecot-core":   {"1:2.4.1+dfsg1-6"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDpkgVersions = %#v, want %#v", got, want)
	}
}

func TestParsePacmanVersions(t *testing.T) {
	got := parsePacmanVersions("nginx 1.28.0-1\nmariadb 11.8.2-1\npython-acme 3:1.0-2\nbad name 1\nweird 1.0;x\n" +
		"huge " + strings.Repeat("9", 70) + "\n")
	want := map[string][]string{
		"nginx":       {"1.28.0-1"},
		"mariadb":     {"11.8.2-1"},
		"python-acme": {"3:1.0-2"},
		"weird":       {""},
		"huge":        {""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePacmanVersions = %#v, want %#v", got, want)
	}
}

func TestComponentPackageVersionsMapsTheCatalogue(t *testing.T) {
	installed := map[string][]string{
		"nginx":          {"1.26.3-3+deb13u1"},
		"dovecot-imapd":  {"1:2.4.1+dfsg1-6"},
		"dovecot-lmtpd":  {"1:2.4.1+dfsg1-6"},
		"postgresql":     {"17+278"},
		"postgresql-17":  {"17.6-0+deb13u1"},
		"spamassassin":   {"4.0.1-1"},
		"spamass-milter": {"0.4.0-2"},
	}
	components := componentPackageVersions(installed, "apt")
	answer := transport.InstalledPackageVersionsResponse{Components: components}
	if got := versionsOf(answer, "nginx"); !reflect.DeepEqual(got, []transport.InstalledPackageVersion{{Package: "nginx", Version: "1.26.3-3+deb13u1"}}) {
		t.Fatalf("nginx = %#v", got)
	}
	if got := versionsOf(answer, "dovecot"); len(got) != 2 {
		t.Fatalf("dovecot: only the installed packages are listed, got %#v", got)
	}
	// The versioned server package is the answer, not the meta-package.
	if got := versionsOf(answer, "postgresql"); !reflect.DeepEqual(got, []transport.InstalledPackageVersion{{Package: "postgresql-17", Version: "17.6-0+deb13u1"}}) {
		t.Fatalf("postgresql = %#v", got)
	}
	if got := versionsOf(answer, "spamassassin"); len(got) != 2 {
		t.Fatalf("spamassassin: both packages, with their own versions: %#v", got)
	}
	// A component with no installed package is not listed at all.
	for _, absent := range []string{"certbot", "mariadb", "redis"} {
		if got := versionsOf(answer, absent); got != nil {
			t.Fatalf("%s is absent but was listed: %#v", absent, got)
		}
	}
}

func TestInstalledPackageVersionsReadsWithOneFixedQuery(t *testing.T) {
	calls := stubPackageVersionRead(t, hostplatform.PackageManagerAPT, idle, func(context.Context) ([]byte, error) {
		return []byte("nginx\tii \t1.26.3-3+deb13u1\ncertbot\tii \t4.0.0-2\n"), nil
	})
	var answer transport.InstalledPackageVersionsResponse
	if err := (&Agent{}).InstalledPackageVersionsV1(&transport.Empty{}, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.State != transport.PackageVersionsRead || answer.Family != "apt" || answer.Error != "" {
		t.Fatalf("answer = %+v", answer)
	}
	if got := versionsOf(answer, "certbot"); len(got) != 1 || got[0].Version != "4.0.0-2" {
		t.Fatalf("certbot = %#v", got)
	}
	// Exactly one query, the fixed read-only argv: no package names from the
	// caller, no install/remove/configure verb.
	want := []packageVersionCall{{"/usr/bin/dpkg-query", []string{"-W", "-f=${binary:Package}\t${db:Status-Abbrev}\t${Version}\n"}}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("queries = %#v, want %#v", *calls, want)
	}

	calls = stubPackageVersionRead(t, hostplatform.PackageManagerPacman, idle, func(context.Context) ([]byte, error) {
		return []byte("nginx 1.28.0-1\n"), nil
	})
	answer = readInstalledPackageVersions(context.Background())
	if answer.State != transport.PackageVersionsRead || len(versionsOf(answer, "nginx")) != 1 {
		t.Fatalf("pacman answer = %+v", answer)
	}
	if want := []packageVersionCall{{"/usr/bin/pacman", []string{"-Q"}}}; !reflect.DeepEqual(*calls, want) {
		t.Fatalf("pacman queries = %#v", *calls)
	}
}

func TestInstalledPackageVersionsSkipsWhileThePackageManagerIsBusy(t *testing.T) {
	for name, probe := range map[string]func() (bool, error){
		"busy":       func() (bool, error) { return true, nil },
		"unreadable": func() (bool, error) { return false, errors.New("read /proc/locks: permission denied") },
	} {
		calls := stubPackageVersionRead(t, hostplatform.PackageManagerAPT, probe, func(context.Context) ([]byte, error) {
			t.Fatalf("%s: the package database was queried while it may be in use", name)
			return nil, nil
		})
		answer := readInstalledPackageVersions(context.Background())
		if answer.State != transport.PackageVersionsBusy || len(answer.Components) != 0 || len(*calls) != 0 {
			t.Fatalf("%s: answer = %+v, calls = %d", name, answer, len(*calls))
		}
	}
}

func TestInstalledPackageVersionsIsBoundedInTime(t *testing.T) {
	old := packageVersionQueryTimeout
	packageVersionQueryTimeout = 20 * time.Millisecond
	t.Cleanup(func() { packageVersionQueryTimeout = old })
	stubPackageVersionRead(t, hostplatform.PackageManagerAPT, idle, func(ctx context.Context) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	started := time.Now()
	answer := readInstalledPackageVersions(context.Background())
	if time.Since(started) > 5*time.Second {
		t.Fatal("the query was not bounded")
	}
	if answer.State != transport.PackageVersionsFailed || answer.Error != "the package database did not answer in time" || len(answer.Components) != 0 {
		t.Fatalf("answer = %+v", answer)
	}
}

func TestInstalledPackageVersionsFailuresCarryNoOutput(t *testing.T) {
	stubPackageVersionRead(t, hostplatform.PackageManagerAPT, idle, func(context.Context) ([]byte, error) {
		return []byte("secret partial output"), errors.New("exit status 2: dpkg-query: error: secret")
	})
	answer := readInstalledPackageVersions(context.Background())
	if answer.State != transport.PackageVersionsFailed || answer.Error != "the package database query failed" || len(answer.Components) != 0 {
		t.Fatalf("answer = %+v", answer)
	}
	stubPackageVersionRead(t, hostplatform.PackageManagerDNF, idle, func(context.Context) ([]byte, error) {
		t.Fatal("an unsupported family was queried")
		return nil, nil
	})
	if answer := readInstalledPackageVersions(context.Background()); answer.State != transport.PackageVersionsUnsupported {
		t.Fatalf("dnf answer = %+v", answer)
	}
}

func TestInstalledPackageVersionsSaysAnAnswerWasTooLarge(t *testing.T) {
	stubPackageVersionRead(t, hostplatform.PackageManagerAPT, idle, func(context.Context) ([]byte, error) {
		return nil, errPackageVersionOutputTooLarge
	})
	answer := readInstalledPackageVersions(context.Background())
	if answer.State != transport.PackageVersionsFailed || answer.Error != errPackageVersionOutputTooLarge.Error() || len(answer.Components) != 0 {
		t.Fatalf("answer = %+v", answer)
	}
}
