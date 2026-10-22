package main

import (
	"context"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Installed package versions on the Components page (v0.1.0-alpha.97, D-036).
//
// A version shown is the package database's own answer, and unknown stays
// unknown. The Agent reads every catalogue package's installed version in one
// bounded read-only query per component scan (Agent.InstalledPackageVersionsV1).
// The Panel keeps the answer with the scan's observations (an additive JSON
// field: a cache row written before alpha.97 has none and reads as unknown;
// no schema change) and derives what the row shows:
//
//   - PackageVersion: the full version exactly as printed (epoch and revision
//     kept), only when every installed package of the component has the same
//     readable version;
//   - PackageVersionShort: its upstream part for the row (1.26.3 of
//     1.26.3-3+deb13u1);
//   - PackageVersionsDiffer: the component's packages report different
//     versions; the page says "several versions", never one of them;
//   - PackageVersionKept: the package manager was busy at the last scan, so
//     the versions read at PackageVersionReadAt were kept, and said as such.
//
// The multi-version runtimes (PHP-FPM, Node.js) keep their per-copy versions
// and are not given a package version.
//
// Gösterilen sürüm paket veritabanının kendi yanıtıdır; bilinmeyen bilinmez
// kalır. Panel tam sürümü, satır için kısa (upstream) biçimi ve "birden çok
// sürüm" durumunu türetir; paket yöneticisi meşgulse önceki okuma korunur ve
// öyle söylenir.

// observedPackageVersions is what a scan stores per component.
type observedPackageVersions struct {
	Packages []transport.InstalledPackageVersion
	ReadAt   string
	Kept     bool
	// KeptLong: the reading kept is older than packageVersionKeptLongAfter;
	// the package manager has been busy at every check since then.
	KeptLong bool
}

// packageVersionKeptLongAfter is how old a kept reading is before the page
// adds a short guidance line (the package manager may be stuck).
const packageVersionKeptLongAfter = 24 * time.Hour

// packageVersionsForScan reads the previous scan's versions from the cache
// (for a busy answer) and asks the Agent once.
// Önceki taramanın sürümlerini önbellekten okur ve Agent'a bir kez sorar.
func (p *Panel) packageVersionsForScan(ctx context.Context) map[string]observedPackageVersions {
	var previousObservations []serviceObservation
	var previousData string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT data FROM service_scan_cache WHERE id = 1`).Scan(&previousData); err == nil {
		if snapshot, decodeErr := decodeScanCacheSnapshot(previousData); decodeErr == nil {
			previousObservations = snapshot.Observations
		}
	}
	return p.readPackageVersionsForScan(ctx, previousObservations, time.Now())
}

// readPackageVersionsForScan asks the Agent once. Busy keeps the previous
// scan's versions (marked kept, with the time they were read); a failed or
// unsupported read leaves every version unknown. It never fails the scan.
func (p *Panel) readPackageVersionsForScan(ctx context.Context, previous []serviceObservation, now time.Time) map[string]observedPackageVersions {
	var reply transport.InstalledPackageVersionsResponse
	if err := p.callAgentContext(ctx, "Agent.InstalledPackageVersionsV1", &transport.Empty{}, &reply); err != nil {
		log.Printf("[components] installed package versions were not read: %v", err)
		return map[string]observedPackageVersions{}
	}
	return packageVersionsFromReply(reply, previous, now)
}

func packageVersionsFromReply(reply transport.InstalledPackageVersionsResponse, previous []serviceObservation, now time.Time) map[string]observedPackageVersions {
	versions := map[string]observedPackageVersions{}
	switch reply.State {
	case transport.PackageVersionsRead:
		readAt := now.UTC().Format(time.RFC3339)
		for _, component := range reply.Components {
			packages := make([]transport.InstalledPackageVersion, 0, len(component.Packages))
			for _, pkg := range component.Packages {
				packages = append(packages, transport.InstalledPackageVersion{
					Package: strings.TrimSpace(pkg.Package),
					Version: panelPackageVersion(pkg.Version),
				})
			}
			if len(packages) > 0 && core.GetManagedServiceByID(component.ServiceID) != nil {
				versions[component.ServiceID] = observedPackageVersions{Packages: packages, ReadAt: readAt}
			}
		}
	case transport.PackageVersionsBusy:
		for _, o := range previous {
			if len(o.PackageVersions) > 0 && o.PackageVersionsReadAt != "" {
				keptLong := false
				if readAt, err := time.Parse(time.RFC3339, o.PackageVersionsReadAt); err == nil {
					keptLong = now.Sub(readAt) > packageVersionKeptLongAfter
				}
				versions[o.ID] = observedPackageVersions{Packages: o.PackageVersions, ReadAt: o.PackageVersionsReadAt, Kept: true, KeptLong: keptLong}
			}
		}
	default:
		if reply.Error != "" {
			log.Printf("[components] installed package versions were not read: %s", sanitizePanelUpdateSummary(reply.Error))
		}
	}
	return versions
}

// panelVersionText re-checks the Agent's text: what a package database prints,
// at most 64 characters, or nothing.
var panelVersionText = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+~:_-]{0,63}$`)

func panelPackageVersion(raw string) string {
	value := strings.TrimSpace(raw)
	if !panelVersionText.MatchString(value) {
		return ""
	}
	return value
}

// packageVersionView derives what a row shows from the stored packages. Any
// unreadable version makes the whole answer unknown.
func packageVersionView(packages []transport.InstalledPackageVersion) (full, short string, differ bool) {
	if len(packages) == 0 {
		return "", "", false
	}
	first := ""
	for _, pkg := range packages {
		version := panelPackageVersion(pkg.Version)
		if version == "" {
			return "", "", false
		}
		if first == "" {
			first = version
		} else if version != first {
			differ = true
		}
	}
	if differ {
		return "", "", true
	}
	return first, upstreamPackageVersion(first), false
}

// upstreamPackageVersion is the short form for the row: the epoch ("1:") and
// the packaging revision (after the last "-": Debian's "-3+deb13u1", Ubuntu's
// "-2ubuntu7.5", Arch's "-1") are removed, then a "+dfsg"/"~bpo" style suffix
// of the upstream part. When that would not leave a number, the full version
// is returned unchanged.
//
// A Debian "really" version (alpha.101, owner's screen on Frankfurt, where
// phpMyAdmin read "5.2.2-really…"): a package that had to go back to an older
// upstream keeps a higher number before "really" so the package manager still
// sees an upgrade, and the version actually shipped follows "really"
// ("5.2.2-really5.2.1-1" is 5.2.1; "2:4.1.0+really4.0.9-2" is 4.0.9). The
// short form is that real version. The short form used to cut only at "+"
// or "~", so "-really…" stayed in it and the row's narrow column clipped it.
// Debian "really" sürümünde gerçek sürüm "really" sözcüğünden sonra gelir;
// kısa biçim odur.
func upstreamPackageVersion(full string) string {
	value := full
	if colon := strings.IndexByte(value, ':'); colon > 0 && allDigits(value[:colon]) {
		value = value[colon+1:]
	}
	if dash := strings.LastIndexByte(value, '-'); dash > 0 {
		value = value[:dash]
	}
	if shipped := reallyVersion(value); shipped != "" {
		value = shipped
	}
	if cut := strings.IndexAny(value, "+~"); cut > 0 {
		value = value[:cut]
	}
	if value == "" || value[0] < '0' || value[0] > '9' {
		return full
	}
	return value
}

// reallyVersion returns what follows the last "really" of an upstream
// version when "really" stands after a version separator ("+", "-", "~" or
// ".") and is followed by a digit; otherwise "".
// "really" bir ayraçtan sonra gelip ardından rakam geliyorsa ondan sonrasını
// döndürür.
func reallyVersion(upstream string) string {
	const marker = "really"
	at := strings.LastIndex(upstream, marker)
	if at <= 0 || !strings.ContainsRune("+-~.", rune(upstream[at-1])) {
		return ""
	}
	shipped := upstream[at+len(marker):]
	if shipped == "" || shipped[0] < '0' || shipped[0] > '9' {
		return ""
	}
	return shipped
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
