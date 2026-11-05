package dnsunitidentity

import (
	"strings"
	"testing"
)

const loadedNamedTarget = "Id=named.service\nNames=bind9.service named.service\nLoadState=loaded\nUnitFileState=enabled\n" +
	"FragmentPath=/usr/lib/systemd/system/named.service\nDropInPaths=\nSourcePath=\nTransient=no\n" +
	"ExecStart={ path=/usr/sbin/named ; argv[]=/usr/sbin/named -f $OPTIONS ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }\n"

// The masked reading is what systemctl reported for the package guard's
// persistent mask in the 2026-09-29 owner-inverse evidence: no ExecStart,
// fragment /etc/systemd/system/<unit>.
const maskedNamedTarget = "Id=named.service\nNames=named.service\nLoadState=masked\nUnitFileState=masked\n" +
	"FragmentPath=/etc/systemd/system/named.service\nDropInPaths=\nSourcePath=\nTransient=no\n"

const absentNamedTarget = "Id=named.service\nNames=named.service\nLoadState=not-found\nUnitFileState=\n" +
	"FragmentPath=\nDropInPaths=\nSourcePath=\nTransient=no\n"

func TestParseTargetObservationRepresentsNeverStartedTargets(t *testing.T) {
	loaded, err := ParseTargetObservation(loadedNamedTarget)
	if err != nil || loaded.State != TargetLoaded || loaded.Identity.ExecStartPath != "/usr/sbin/named" ||
		ValidateAPTBINDVendorNamedIdentity(loaded.Identity, true) != nil {
		t.Fatalf("loaded target lost its strict identity: %+v %v", loaded, err)
	}
	masked, err := ParseTargetObservation(maskedNamedTarget)
	if err != nil || masked.State != TargetPersistentMask || masked.ID != "named.service" ||
		masked.FragmentPath != "/etc/systemd/system/named.service" {
		t.Fatalf("guard mask not represented: %+v %v", masked, err)
	}
	// systemd may omit empty properties; absence and emptiness are the same.
	if got, err := ParseTargetObservation("Id=named.service\nNames=named.service\nLoadState=masked\nUnitFileState=masked\nFragmentPath=/etc/systemd/system/named.service\n"); err != nil || got.State != TargetPersistentMask {
		t.Fatalf("mask without empty properties refused: %+v %v", got, err)
	}
	absent, err := ParseTargetObservation(absentNamedTarget)
	if err != nil || absent.State != TargetAbsent || absent.ID != "named.service" {
		t.Fatalf("absent target not represented: %+v %v", absent, err)
	}
	// The strict reader still refuses the masked reading: its callers are
	// unchanged.
	if _, err := Parse(strings.Join(identityLinesOf(maskedNamedTarget), "\n")); err == nil ||
		!strings.Contains(err.Error(), "incomplete DNS unit identity") {
		t.Fatalf("strict identity reader accepted a masked unit: %v", err)
	}
}

func TestParseTargetObservationRefusesUnsafeOrAmbiguousStates(t *testing.T) {
	for name, output := range map[string]string{
		"runtime mask": strings.Replace(strings.Replace(maskedNamedTarget, "UnitFileState=masked", "UnitFileState=masked-runtime", 1),
			"/etc/systemd/system/", "/run/systemd/system/", 1),
		"runtime fragment":       strings.Replace(maskedNamedTarget, "/etc/systemd/system/", "/run/systemd/system/", 1),
		"mask with command":      maskedNamedTarget + "ExecStart={ path=/usr/sbin/named ; argv[]=/usr/sbin/named ; ignore_errors=no }\n",
		"mask with drop-in":      strings.Replace(maskedNamedTarget, "DropInPaths=", "DropInPaths=/etc/systemd/system/named.service.d/x.conf", 1),
		"transient mask":         strings.Replace(maskedNamedTarget, "Transient=no", "Transient=yes", 1),
		"mask alias names":       strings.Replace(maskedNamedTarget, "Names=named.service", "Names=bind9.service named.service", 1),
		"absent with fragment":   strings.Replace(absentNamedTarget, "FragmentPath=", "FragmentPath=/usr/lib/systemd/system/named.service", 1),
		"absent unit file state": strings.Replace(absentNamedTarget, "UnitFileState=", "UnitFileState=disabled", 1),
		"bad setting":            strings.Replace(maskedNamedTarget, "LoadState=masked", "LoadState=bad-setting", 1),
		"loaded incomplete":      strings.Replace(loadedNamedTarget, "SourcePath=\n", "", 1),
		"no load state":          strings.Replace(maskedNamedTarget, "LoadState=masked\n", "", 1),
		"duplicate":              maskedNamedTarget + "LoadState=masked\n",
		"unexpected property":    maskedNamedTarget + "MainPID=0\n",
		"malformed":              maskedNamedTarget + "garbage\n",
	} {
		if got, err := ParseTargetObservation(output); err == nil {
			t.Fatalf("%s accepted: %+v", name, got)
		}
	}
}

func identityLinesOf(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if line != "" && !strings.HasPrefix(line, "LoadState=") && !strings.HasPrefix(line, "UnitFileState=") {
			lines = append(lines, line)
		}
	}
	return lines
}

// debian12NamedIdentity is what systemd 252 reported for bind9
// 1:9.18.49-1~deb12u3 on a fresh Debian 12 image (lab evidence
// lab-images-d12-u2204-20261104, raw/bind-unit-identity-d12.txt).
const debian12NamedIdentity = "Id=named.service\nNames=named.service\n" +
	"FragmentPath=/lib/systemd/system/named.service\nDropInPaths=\nSourcePath=\nTransient=no\n" +
	"ExecStart={ path=/usr/sbin/named ; argv[]=/usr/sbin/named -f $OPTIONS ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }\n"

func withLibAlias(t *testing.T, alias bool) {
	t.Helper()
	previous := libIsUsrLibAlias
	libIsUsrLibAlias = func() bool { return alias }
	t.Cleanup(func() { libIsUsrLibAlias = previous })
}

func TestAPTBINDVendorIdentityAcceptsMergedUsrLibName(t *testing.T) {
	named, err := Parse(debian12NamedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	withLibAlias(t, true)
	if err := ValidateAPTBINDVendorNamedIdentity(named, false); err != nil {
		t.Fatalf("Debian 12 vendor identity refused on a merged-/usr host: %v", err)
	}
	usr := named
	usr.FragmentPath = "/usr/lib/systemd/system/named.service"
	if err := ValidateAPTBINDVendorNamedIdentity(usr, false); err != nil {
		t.Fatalf("/usr/lib vendor identity refused: %v", err)
	}
	aliased := named
	aliased.Names = []string{"bind9.service", "named.service"}
	if err := ValidateAPTBINDVendorAliasIdentity(aliased, aliased); err != nil {
		t.Fatalf("Debian 12 enabled alias identity refused: %v", err)
	}
	for name, mutate := range map[string]func(*Identity){
		"etc fragment":       func(i *Identity) { i.FragmentPath = "/etc/systemd/system/named.service" },
		"usr local fragment": func(i *Identity) { i.FragmentPath = "/usr/local/lib/systemd/system/named.service" },
		"run fragment":       func(i *Identity) { i.FragmentPath = "/run/systemd/system/named.service" },
		"other unit name":    func(i *Identity) { i.FragmentPath = "/lib/systemd/system/bind9.service" },
		"non-canonical /lib": func(i *Identity) { i.FragmentPath = "/lib/systemd/system/../system/named.service" },
		"drop-in":            func(i *Identity) { i.DropInPaths = []string{"/etc/systemd/system/named.service.d/x.conf"} },
		"different command":  func(i *Identity) { i.ExecStartArgv = "/usr/sbin/named -f -u root" },
		"different binary":   func(i *Identity) { i.ExecStartPath = "/usr/local/sbin/named" },
		"generated source":   func(i *Identity) { i.SourcePath = "/etc/init.d/named" },
		"transient":          func(i *Identity) { i.Transient = "yes" },
		"unexpected alias":   func(i *Identity) { i.Names = []string{"bind9.service", "named.service"} },
	} {
		candidate := named
		mutate(&candidate)
		if err := ValidateAPTBINDVendorNamedIdentity(candidate, false); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	// Arch is merged-/usr in packaging too; its unit is never under /lib.
	pacman := named
	pacman.ExecStartPath, pacman.ExecStartArgv = "/usr/bin/named", "/usr/bin/named -f -u named"
	if err := ValidatePacmanBINDVendorIdentity(pacman); err == nil {
		t.Fatal("pacman BIND identity accepted the /lib name")
	}
}

func TestAPTBINDVendorIdentityRefusesLibNameWithoutMergedUsr(t *testing.T) {
	named, err := Parse(debian12NamedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	withLibAlias(t, false)
	if err := ValidateAPTBINDVendorNamedIdentity(named, false); err == nil {
		t.Fatal("/lib unit accepted where /lib is not the merged-/usr alias")
	}
	named.FragmentPath = "/usr/lib/systemd/system/named.service"
	if err := ValidateAPTBINDVendorNamedIdentity(named, false); err != nil {
		t.Fatalf("/usr/lib vendor identity refused: %v", err)
	}
}

func TestIsAPTVendorUnitFragmentRefusesMalformedUnitNames(t *testing.T) {
	withLibAlias(t, true)
	for _, unit := range []string{"", "../named.service", "system/named.service"} {
		if IsAPTVendorUnitFragment(SplitUsrVendorUnitDir+unit, unit) ||
			IsAPTVendorUnitFragment(VendorUnitDir+unit, unit) {
			t.Fatalf("malformed unit name %q accepted", unit)
		}
	}
}

// ubuntu2204NamedIdentity is what systemd 249 reported for bind9
// 1:9.18.39-0ubuntu0.22.04.8 on a fresh Ubuntu 22.04 image (alpha.123 lab,
// 2026-11-05): Type=forking, named without -f.
const ubuntu2204NamedIdentity = "Id=named.service\nNames=named.service\n" +
	"FragmentPath=/lib/systemd/system/named.service\nDropInPaths=\nSourcePath=\nTransient=no\n" +
	"ExecStart={ path=/usr/sbin/named ; argv[]=/usr/sbin/named $OPTIONS ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }\n"

func withForkingHost(t *testing.T, ubuntu2204 bool) {
	t.Helper()
	previous := aptNamedForkingHost
	aptNamedForkingHost = func() bool { return ubuntu2204 }
	t.Cleanup(func() { aptNamedForkingHost = previous })
}

func TestAPTBINDVendorIdentityAcceptsUbuntu2204ForkingCommandOnlyThere(t *testing.T) {
	named, err := Parse(ubuntu2204NamedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	withLibAlias(t, true)
	withForkingHost(t, true)
	if err := ValidateAPTBINDVendorNamedIdentity(named, false); err != nil {
		t.Fatalf("Ubuntu 22.04 vendor identity refused on Ubuntu 22.04: %v", err)
	}
	aliased := named
	aliased.Names = []string{"bind9.service", "named.service"}
	if err := ValidateAPTBINDVendorAliasIdentity(aliased, aliased); err != nil {
		t.Fatalf("Ubuntu 22.04 enabled alias identity refused: %v", err)
	}
	foreground, err := Parse(debian12NamedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAPTBINDVendorNamedIdentity(foreground, false); err != nil {
		t.Fatalf("foreground identity refused on Ubuntu 22.04: %v", err)
	}
	for name, argv := range map[string]string{
		"extra option": "/usr/sbin/named -u root $OPTIONS",
		"no options":   "/usr/sbin/named",
		"other order":  "/usr/sbin/named $OPTIONS -f",
	} {
		candidate := named
		candidate.ExecStartArgv = argv
		if err := ValidateAPTBINDVendorNamedIdentity(candidate, false); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	withForkingHost(t, false)
	if err := ValidateAPTBINDVendorNamedIdentity(named, false); err == nil {
		t.Fatal("forking BIND command accepted on a host that is not Ubuntu 22.04")
	}
	if err := ValidateAPTBINDVendorNamedIdentity(foreground, false); err != nil {
		t.Fatalf("foreground identity refused elsewhere: %v", err)
	}
}
