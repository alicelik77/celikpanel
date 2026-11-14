package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// retireTestStore is a native store inside a temporary /etc, with certbot's
// `delete` replaced by one that removes what certbot removes.
func retireTestStore(t *testing.T) (etc string, calls *[][]string) {
	t.Helper()
	root := nativeSiteTestStore(t)
	etc = filepath.Join(root, "etc")
	nativeSiteConfigDir = filepath.Join(etc, "letsencrypt")
	savedRoot, savedSnapshots := nativeSiteRetireScanRoot, nativeSiteRetireSnapshotRoots
	nativeSiteRetireScanRoot = etc
	nativeSiteRetireSnapshotRoots = []string{filepath.Join(root, "backups", "update-snapshots"), filepath.Join(root, "backups", "recovery-snapshots")}
	t.Cleanup(func() { nativeSiteRetireScanRoot, nativeSiteRetireSnapshotRoots = savedRoot, savedSnapshots })
	var recorded [][]string
	nativeSiteCertbotRun = func(_ context.Context, args ...string) ([]byte, error) {
		recorded = append(recorded, append([]string(nil), args...))
		name := args[len(args)-2]
		for _, path := range []string{nativeSiteRenewalConfPath(name), filepath.Join(nativeSiteConfigDir, "live", name), filepath.Join(nativeSiteConfigDir, "archive", name)} {
			_ = os.RemoveAll(path)
		}
		return nil, nil
	}
	return etc, &recorded
}

func retireTestLineage(t *testing.T, lineage, conf string) {
	t.Helper()
	leaf, key := makeCertificatePair(t, "ecdsa", []string{"example.test"})
	writeNativeLineage(t, lineage, 1, leaf, key, leaf, strings.ReplaceAll(conf, testNativeLineage, lineage))
}

// alpha.134 lab: after "Cover the mail name too" the lineage served before
// stayed and certbot kept renewing it. A replaced lineage is removed with
// `certbot delete` only when it is CelikPanel's, unchanged by the owner, and
// no file on the server names it; anything else is kept and said.
func TestRetireSiteCertificateLineagesRemovesOnlyWhatNothingNames(t *testing.T) {
	etc, calls := retireTestStore(t)
	const (
		removed   = "celikpanel-site-42-aaaaaaaaaaaaaaaaaaaaaaaa"
		named     = "celikpanel-site-42-bbbbbbbbbbbbbbbbbbbbbbbb"
		owners    = "celikpanel-site-42-cccccccccccccccccccccccc"
		gone      = "celikpanel-site-42-dddddddddddddddddddddddd"
		sameAsNew = "celikpanel-site-42-eeeeeeeeeeeeeeeeeeeeeeee"
		active    = "celikpanel-site-42-ffffffffffffffffffffffff"
		elsewhere = "celikpanel-site-42-111111111111111111111111"
	)
	pebble := strings.ReplaceAll(testRenewalConf, "https://acme-v02.api.letsencrypt.org/directory", "https://pebble.test:14000/dir")
	retireTestLineage(t, removed, testRenewalConf)
	retireTestLineage(t, named, testRenewalConf)
	retireTestLineage(t, owners, strings.ReplaceAll(testRenewalConf, "authenticator = webroot", "authenticator = dns-cloudflare"))
	retireTestLineage(t, sameAsNew, pebble)
	retireTestLineage(t, active, pebble)
	retireTestLineage(t, elsewhere, strings.ReplaceAll(testRenewalConf, "https://acme-v02.api.letsencrypt.org/directory", "https://other.test/dir"))
	// The owner's own site names one of them; certbot's store names all of
	// them and is not read.
	site := filepath.Join(etc, "nginx", "sites-enabled", "own.conf")
	if err := os.MkdirAll(filepath.Dir(site), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(site, []byte("ssl_certificate /etc/letsencrypt/live/"+named+"/fullchain.pem;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nativeSiteStatusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(nativeSiteStatusDir, removed)
	if err := os.WriteFile(record, []byte("format="+transport.NativeSiteDeployFormat+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var resp transport.RetireSiteCertificateLineagesResponse
	request := &transport.RetireSiteCertificateLineagesRequest{
		DomainID: 42, SubscriptionID: 7, ActiveLineage: active,
		Lineages: []transport.RetireSiteLineage{
			{LineageName: removed}, {LineageName: named}, {LineageName: owners},
			{LineageName: gone}, {LineageName: sameAsNew}, {LineageName: elsewhere},
		},
	}
	if err := (&Agent{}).RetireSiteCertificateLineagesV1(request, &resp); err != nil || resp.Error != "" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	outcomes := map[string]string{}
	for _, result := range resp.Results {
		outcomes[result.LineageName] = result.Outcome
	}
	want := map[string]string{
		removed:   transport.SiteLineageRetired,
		named:     transport.SiteLineageInUse,
		owners:    transport.SiteLineageOwnerChanged,
		gone:      transport.SiteLineageAbsent,
		sameAsNew: transport.SiteLineageRetired,
		elsewhere: transport.SiteLineageOwnerChanged,
	}
	if !reflect.DeepEqual(outcomes, want) {
		t.Fatalf("outcomes = %v, want %v", outcomes, want)
	}
	for _, result := range resp.Results {
		if result.LineageName == named && !strings.Contains(result.Detail, site) {
			t.Fatalf("in use, without the file that names it: %+v", result)
		}
	}
	if len(*calls) != 2 {
		t.Fatalf("certbot calls = %v", *calls)
	}
	for _, call := range *calls {
		if call[0] != "delete" || call[len(call)-1] != "--non-interactive" || (call[len(call)-2] != removed && call[len(call)-2] != sameAsNew) {
			t.Fatalf("certbot call = %v", call)
		}
	}
	if _, err := os.Stat(record); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the removed lineage's deploy record stayed: %v", err)
	}
	if _, err := os.Stat(nativeSiteRenewalConfPath(active)); err != nil {
		t.Fatalf("the active lineage was touched: %v", err)
	}
	if _, err := os.Stat(nativeSiteRenewalConfPath(named)); err != nil {
		t.Fatalf("a named lineage was removed: %v", err)
	}
}

// Anything that cannot be established keeps every lineage: a failed
// certbot, and a server whose files could not all be read.
func TestRetireSiteCertificateLineagesKeepsWhatItCannotEstablish(t *testing.T) {
	etc, _ := retireTestStore(t)
	const lineage = "celikpanel-site-42-aaaaaaaaaaaaaaaaaaaaaaaa"
	retireTestLineage(t, lineage, testRenewalConf)
	request := &transport.RetireSiteCertificateLineagesRequest{DomainID: 42, SubscriptionID: 7,
		Lineages: []transport.RetireSiteLineage{{LineageName: lineage}}}

	nativeSiteCertbotRun = func(context.Context, ...string) ([]byte, error) {
		return []byte("Error: simulated certbot delete failure"), errors.New("exit status 1")
	}
	var resp transport.RetireSiteCertificateLineagesResponse
	_ = (&Agent{}).RetireSiteCertificateLineagesV1(request, &resp)
	if len(resp.Results) != 1 || resp.Results[0].Outcome != transport.SiteLineageFailed ||
		!strings.Contains(resp.Results[0].Detail, "simulated certbot delete failure") {
		t.Fatalf("resp = %+v", resp)
	}

	saved := nativeSiteRetireMaxFiles
	nativeSiteRetireMaxFiles = 0
	t.Cleanup(func() { nativeSiteRetireMaxFiles = saved })
	if err := os.WriteFile(filepath.Join(etc, "hosts"), []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp = transport.RetireSiteCertificateLineagesResponse{}
	_ = (&Agent{}).RetireSiteCertificateLineagesV1(request, &resp)
	if len(resp.Results) != 1 || resp.Results[0].Outcome != transport.SiteLineageNotChecked {
		t.Fatalf("resp = %+v", resp)
	}
	if _, err := os.Stat(nativeSiteRenewalConfPath(lineage)); err != nil {
		t.Fatalf("a lineage was removed although nothing was established: %v", err)
	}
}

// A file is read whole in pieces: a name that spans two pieces is found,
// and a large file is never skipped.
func TestScanFileForLineagesFindsANameAcrossPieces(t *testing.T) {
	saved := nativeSiteRetireChunkBytes
	nativeSiteRetireChunkBytes = 7
	t.Cleanup(func() { nativeSiteRetireChunkBytes = saved })
	const lineage = "celikpanel-site-42-aaaaaaaaaaaaaaaaaaaaaaaa"
	path := filepath.Join(t.TempDir(), "big.conf")
	content := strings.Repeat("# padding\n", 50) + "ssl_certificate /etc/letsencrypt/live/" + lineage + "/fullchain.pem;\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	if err := scanFileForLineages(path, []string{lineage, "celikpanel-site-42-bbbbbbbbbbbbbbbbbbbbbbbb"}, found); err != nil {
		t.Fatal(err)
	}
	if found[lineage] != path || len(found) != 1 {
		t.Fatalf("found = %v", found)
	}
}

// Only replaced lineages of the named domain, never the active one.
func TestRetireSiteCertificateLineagesRefusesWhatIsNotAReplacedLineageOfTheDomain(t *testing.T) {
	retireTestStore(t)
	for name, request := range map[string]*transport.RetireSiteCertificateLineagesRequest{
		"nothing":        {DomainID: 42},
		"another domain": {DomainID: 42, Lineages: []transport.RetireSiteLineage{{LineageName: "celikpanel-site-43-aaaaaaaaaaaaaaaaaaaaaaaa"}}},
		"the active one": {DomainID: 42, ActiveLineage: testNativeLineage, Lineages: []transport.RetireSiteLineage{{LineageName: testNativeLineage}}},
		"not a lineage":  {DomainID: 42, Lineages: []transport.RetireSiteLineage{{LineageName: "../renewal/x"}}},
	} {
		var resp transport.RetireSiteCertificateLineagesResponse
		if err := (&Agent{}).RetireSiteCertificateLineagesV1(request, &resp); err != nil || resp.Error == "" || len(resp.Results) != 0 {
			t.Fatalf("%s: resp=%+v err=%v", name, resp, err)
		}
	}
}

// alpha.134 review: a link under /etc that points into certbot's store names
// the lineage (the walk skips the store, so only the link can say so), as
// written and as resolved; a link that cannot be followed is not concluded.
func TestRetireSiteCertificateLineagesReadsLinks(t *testing.T) {
	etc, calls := retireTestStore(t)
	const (
		written  = "celikpanel-site-42-aaaaaaaaaaaaaaaaaaaaaaaa"
		resolved = "celikpanel-site-42-bbbbbbbbbbbbbbbbbbbbbbbb"
		free     = "celikpanel-site-42-cccccccccccccccccccccccc"
	)
	for _, lineage := range []string{written, resolved, free} {
		retireTestLineage(t, lineage, testRenewalConf)
	}
	private := filepath.Join(etc, "ssl", "private")
	if err := os.MkdirAll(private, 0o755); err != nil {
		t.Fatal(err)
	}
	// As written: the link's text names the lineage's live key.
	if err := os.Symlink(filepath.Join(nativeSiteConfigDir, "live", written, "privkey.pem"), filepath.Join(private, "site.pem")); err != nil {
		t.Fatal(err)
	}
	// Resolved only: a link to a neutral name that itself points into the archive.
	neutral := filepath.Join(nativeSiteConfigDir, "current-site.pem")
	if err := os.Symlink(filepath.Join(nativeSiteConfigDir, "archive", resolved, "fullchain1.pem"), neutral); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(neutral, filepath.Join(private, "chain.pem")); err != nil {
		t.Fatal(err)
	}
	var resp transport.RetireSiteCertificateLineagesResponse
	request := &transport.RetireSiteCertificateLineagesRequest{DomainID: 42, SubscriptionID: 7,
		Lineages: []transport.RetireSiteLineage{{LineageName: written}, {LineageName: resolved}, {LineageName: free}}}
	_ = (&Agent{}).RetireSiteCertificateLineagesV1(request, &resp)
	outcomes := map[string]string{}
	for _, result := range resp.Results {
		outcomes[result.LineageName] = result.Outcome + " " + result.Detail
	}
	if outcomes[written] != transport.SiteLineageInUse+" "+filepath.Join(private, "site.pem") ||
		outcomes[resolved] != transport.SiteLineageInUse+" "+filepath.Join(private, "chain.pem") ||
		outcomes[free] != transport.SiteLineageRetired+" " || len(*calls) != 1 {
		t.Fatalf("outcomes = %v calls = %v", outcomes, *calls)
	}

	// A link that cannot be followed (a loop): nothing is removed.
	loop := filepath.Join(private, "loop.pem")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	retireTestLineage(t, free, testRenewalConf)
	resp = transport.RetireSiteCertificateLineagesResponse{}
	_ = (&Agent{}).RetireSiteCertificateLineagesV1(&transport.RetireSiteCertificateLineagesRequest{DomainID: 42, SubscriptionID: 7,
		Lineages: []transport.RetireSiteLineage{{LineageName: free}}}, &resp)
	if len(resp.Results) != 1 || resp.Results[0].Outcome != transport.SiteLineageNotChecked || len(*calls) != 1 {
		t.Fatalf("resp = %+v calls = %v", resp, *calls)
	}
}

// A retained update snapshot whose panel database serves the lineage keeps
// it: restoring that snapshot (going back) must find it. A lineage the
// snapshot only names as replaced goes; an unreadable snapshot keeps all.
func TestRetireSiteCertificateLineagesKeepsWhatAnUpdateSnapshotServes(t *testing.T) {
	_, calls := retireTestStore(t)
	const (
		served   = "celikpanel-site-42-aaaaaaaaaaaaaaaaaaaaaaaa"
		replaced = "celikpanel-site-42-bbbbbbbbbbbbbbbbbbbbbbbb"
	)
	for _, lineage := range []string{served, replaced} {
		retireTestLineage(t, lineage, testRenewalConf)
	}
	snapshot := filepath.Join(nativeSiteRetireSnapshotRoots[0], "20261113T200000Z-from-133-to-134-0123")
	writeSnapshotDatabase(t, filepath.Join(snapshot, "celikpanel.db"),
		`CREATE TABLE ssl_certificates (id INTEGER PRIMARY KEY, lineage_name TEXT, status TEXT);
		 INSERT INTO ssl_certificates (lineage_name, status) VALUES ('`+served+`', 'active'), ('`+replaced+`', 'revoked');`)
	// An older snapshot without the lineage column cannot name one; a
	// snapshot still being written is not a restore point.
	writeSnapshotDatabase(t, filepath.Join(nativeSiteRetireSnapshotRoots[1], "20250101T000000Z-old", "celikpanel.db"),
		`CREATE TABLE ssl_certificates (id INTEGER PRIMARY KEY, status TEXT);`)
	if err := os.MkdirAll(filepath.Join(nativeSiteRetireSnapshotRoots[0], ".release-snapshot.incomplete.1.ab"), 0o700); err != nil {
		t.Fatal(err)
	}
	request := &transport.RetireSiteCertificateLineagesRequest{DomainID: 42, SubscriptionID: 7,
		Lineages: []transport.RetireSiteLineage{{LineageName: served}, {LineageName: replaced}}}
	var resp transport.RetireSiteCertificateLineagesResponse
	_ = (&Agent{}).RetireSiteCertificateLineagesV1(request, &resp)
	outcomes := map[string]string{}
	for _, result := range resp.Results {
		outcomes[result.LineageName] = result.Outcome + " " + result.Detail
	}
	if outcomes[served] != transport.SiteLineageHeldBySnapshot+" "+snapshot || outcomes[replaced] != transport.SiteLineageRetired+" " || len(*calls) != 1 {
		t.Fatalf("outcomes = %v calls = %v", outcomes, *calls)
	}

	// An unreadable snapshot database: nothing is removed.
	if err := os.WriteFile(filepath.Join(snapshot, "celikpanel.db"), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	retireTestLineage(t, replaced, testRenewalConf)
	resp = transport.RetireSiteCertificateLineagesResponse{}
	_ = (&Agent{}).RetireSiteCertificateLineagesV1(request, &resp)
	for _, result := range resp.Results {
		if result.Outcome != transport.SiteLineageNotChecked {
			t.Fatalf("resp = %+v", resp)
		}
	}
	if len(*calls) != 1 {
		t.Fatalf("calls = %v", *calls)
	}
}

func writeSnapshotDatabase(t *testing.T, path, schema string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
}
