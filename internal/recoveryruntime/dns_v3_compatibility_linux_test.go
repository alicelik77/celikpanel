//go:build linux

package recoveryruntime

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/agentnativecontract"
	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/transport"
)

const alpha113ReleaseCommit = "d3f3a323da46ce8f8dca5ad4385666c5428c353e"

// v3Agent is one target Agent's contract shape.
type v3Agent struct {
	name   string
	commit string
	edit   func(*agentnativecontract.Contract)
}

var (
	// The current producer declares the v3 reader.
	currentV3Agent = v3Agent{"current", strings.Repeat("b", 40), func(*agentnativecontract.Contract) {}}
	// Exactly what the published alpha.113 release's producer wrote.
	alpha113V3Agent = v3Agent{"alpha113", alpha113ReleaseCommit, func(c *agentnativecontract.Contract) { c.DNSNativeCatalogPolicy = "" }}
	// A separated-evidence Agent without the declaration is refused, even
	// though this checker's own decoder understands the evidence.
	priorV3Agent = v3Agent{"prior-separated", strings.Repeat("a", 40), func(c *agentnativecontract.Contract) { c.DNSNativeCatalogPolicy = "" }}
	// alpha.113's commit with another declaration shape is not that release.
	alpha113ForeignShape = v3Agent{"alpha113-without-adoption-reader", alpha113ReleaseCommit, func(c *agentnativecontract.Contract) {
		c.DNSNativeCatalogPolicy = ""
		c.DNSSwitchJournalPolicy = agentnativecontract.DNSSwitchJournalPolicy
	}}
)

// writeV3Fixture lays out one target Agent and one evidence scenario as the
// published release writes it, and returns the bin and private directories.
func writeV3Fixture(t *testing.T, agent v3Agent, scenario, kind string) (string, string) {
	t.Helper()
	root, err := os.MkdirTemp("/run", "celikpanel-dns-v3-compat-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	bin, private := filepath.Join(root, "bin"), filepath.Join(root, "private")
	for _, dir := range []string{bin, private} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	put := func(path string, raw []byte, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, raw, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	binary := []byte("target Agent " + agent.name + ": must only be read")
	put(filepath.Join(bin, "agent"), binary, 0755)
	contract, err := agentnativecontract.New(binary, agent.commit)
	if err != nil {
		t.Fatal(err)
	}
	agent.edit(&contract)
	encoded, err := agentnativecontract.Encode(contract)
	if err != nil {
		t.Fatal(err)
	}
	put(filepath.Join(bin, agentnativecontract.FileName), encoded, 0644)
	statePath := filepath.Join(private, "dns-engine-state.json")
	policy := dnsengineartifact.JournalPolicy{StatePath: statePath, StateUID: 0, StateGID: 0, RequireOwner: true,
		PDNSMainPath: "/etc/powerdns/pdns.conf", PDNSManagedPath: "/etc/powerdns/pdns.d/celikpanel.conf",
		PDNSClusterPath: "/etc/powerdns/pdns.d/celikpanel-cluster.conf", PDNSDatabasePath: "/var/lib/powerdns/pdns.sqlite3"}
	original, err := os.ReadFile("../dnsengineartifact/testdata/switch-journal/alpha81-pdns-switch.json")
	if err != nil {
		t.Fatal(err)
	}
	original = bytes.ReplaceAll(original, []byte("/var/lib/celikpanel-agent-private/dns-engine-state.json"), []byte(statePath))
	base, err := policy.DecodeSwitchJournal(original)
	if err != nil {
		t.Fatal(err)
	}
	role := transport.DNSPairRolePrimary
	if scenario == "joined-backup" {
		role = transport.DNSPairRoleSecondary
	}
	manifest, err := mutationpayload.CanonicalDNSEngineSwitchManifestWithPairIdentity(
		transport.DNSEngineSwitchModeSwitch, "", transport.DNSEnginePowerDNS, 0, 1, 0,
		transport.DNSTopologyPaired, role, "192.0.2.10", "ns1.example.test", "192.0.2.11", "ns2.example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	state := dnsengineartifact.StateV1{Schema: dnsengineartifact.StateSchemaV1, Mode: manifest.Mode,
		Engine: manifest.TargetEngine, EngineEpoch: 1, PairRole: manifest.PairRole, PairLocalIP: manifest.LocalIP, PairPeerIP: manifest.PeerIP,
		PrimaryCatalogSerial: 1790542951, ManifestQualifier: manifest.Qualifier, MutationRequestID: base.MutationRequestID, MutationOwnerID: base.MutationOwnerID,
		NativeCatalogV3: kind}
	switch scenario {
	case "state", "state-with-v1-journal", "state-and-ownership":
		raw, e := dnsengineartifact.CanonicalStateDocumentV3(state)
		if e != nil {
			t.Fatal(e)
		}
		put(statePath, raw, 0600)
		if scenario == "state-with-v1-journal" {
			put(filepath.Join(private, "dns-engine-switch-journal.json"), original, 0600)
		}
		if scenario == "state-and-ownership" {
			raw, e = dnsengineartifact.CanonicalOwnershipDocumentV3(state)
			if e != nil {
				t.Fatal(e)
			}
			put(filepath.Join(private, "dns-engine-ownership-pdns.json"), raw, 0600)
		}
	case "ownership":
		raw, e := dnsengineartifact.CanonicalOwnershipDocumentV3(state)
		if e != nil {
			t.Fatal(e)
		}
		put(filepath.Join(private, "dns-engine-ownership-pdns.json"), raw, 0600)
	case "joined-backup":
		// A joined PowerDNS backup has no native catalog tenure: v3 is a
		// main-server-only document, so its evidence is the separated v2 kind.
		if _, e := dnsengineartifact.CanonicalStateDocumentV3(state); e == nil {
			t.Fatal("a backup server's state was accepted as v3")
		}
		state.NativeCatalogV3 = ""
		state.PrimaryCatalogSerial = 0
		for _, write := range []struct {
			name   string
			encode func(dnsengineartifact.StateV1) ([]byte, error)
		}{{"dns-engine-state.json", dnsengineartifact.CanonicalStateDocumentV2}, {"dns-engine-ownership-pdns.json", dnsengineartifact.CanonicalOwnershipDocumentV2}} {
			raw, e := write.encode(state)
			if e != nil {
				t.Fatal(e)
			}
			put(filepath.Join(private, write.name), raw, 0600)
		}
	case "journal":
		base.SourceEngine = ""
		base.SourceEpoch = 0
		base.TargetEpoch = 1
		base.SourceRevision = 0
		base.Topology = manifest.Topology
		base.PairRole = manifest.PairRole
		base.LocalIP = manifest.LocalIP
		base.LocalNS = manifest.LocalNS
		base.PeerIP = manifest.PeerIP
		base.PeerNS = manifest.PeerNS
		base.ManifestQualifier = manifest.Qualifier
		base.SnapshotBytes = manifest.SnapshotBytes
		base.Zones = manifest.Zones
		base.PrimaryCatalogSerial = 1
		base.StateBefore = dnsengineartifact.FileSnapshot{Path: statePath}
		base.SourceUnitsBefore = nil
		base.TargetUnitsBefore = []dnsengineartifact.UnitSnapshot{{Name: "pdns.service", LoadState: "loaded", ActiveState: "inactive", UnitFileState: "disabled"}}
		after := append([]dnsengineartifact.FileSnapshot(nil), base.ConfigBefore...)
		for _, i := range []int{1, 2} {
			after[i] = dnsengineartifact.FileSnapshot{Path: base.ConfigBefore[i].Path, Exists: true, Mode: 0644, OwnerKnown: true, Data: []byte("managed=1\n"), SHA256: dnsengineartifact.DigestBytes([]byte("managed=1\n"))}
		}
		journal, e := policy.BuildPDNSFreshPrimaryJournalV3ForKind(base, after, kind)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := policy.EncodeSwitchJournal(journal)
		if e != nil {
			t.Fatal(e)
		}
		put(filepath.Join(private, "dns-engine-switch-journal.json"), raw, 0600)
	default:
		t.Fatal("unknown scenario", scenario)
	}
	return bin, private
}

// checkV3Fixture runs the checker and proves it changed no evidence byte.
func checkV3Fixture(t *testing.T, bin, private string) error {
	t.Helper()
	before := map[string][]byte{}
	entries, err := os.ReadDir(private)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		p := filepath.Join(private, entry.Name())
		raw, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		before[p] = raw
	}
	result := CheckDNSApplicationCompatibility(bin, private)
	for p, want := range before {
		got, e := os.ReadFile(p)
		if e != nil || !bytes.Equal(got, want) {
			t.Fatalf("evidence changed at %s: %v", p, e)
		}
	}
	return result
}

var v3Scenarios = []string{"state", "ownership", "state-and-ownership", "journal", "state-with-v1-journal"}

// alpha.114 E7: every PowerDNS main server installed on alpha.113 refused the
// update because no Agent could declare v3 support. The candidate (update)
// and the exact alpha.113 Agent (rollback) are admitted; an Agent without the
// declaration is still refused, and the evidence is never changed.
func TestDNSApplicationCompatibilityRequiresExplicitV3Support(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root descriptor fixture")
	}
	for _, kind := range dnsengineartifact.PDNSFreshPrimaryKinds() {
		for _, scenario := range v3Scenarios {
			for _, agent := range []v3Agent{currentV3Agent, alpha113V3Agent, priorV3Agent, alpha113ForeignShape} {
				want := agent.name == currentV3Agent.name || agent.name == alpha113V3Agent.name
				t.Run(kind+"/"+scenario+"/"+agent.name, func(t *testing.T) {
					bin, private := writeV3Fixture(t, agent, scenario, kind)
					err := checkV3Fixture(t, bin, private)
					if want && err != nil {
						t.Fatalf("compatible Agent refused: %v", err)
					}
					if !want && !errors.Is(err, errDNSApplicationV3) {
						t.Fatalf("got %v, want explicit V3 incompatibility", err)
					}
				})
			}
		}
	}
}

// The joined PowerDNS backup of the same lab updated: its evidence is the
// separated v2 kind, admitted by the explicit v2 declaration both releases
// carry, not by luck. A backup can never hold v3 evidence.
func TestDNSApplicationCompatibilityJoinedPowerDNSBackupIsV2(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root descriptor fixture")
	}
	for _, agent := range []v3Agent{currentV3Agent, alpha113V3Agent, priorV3Agent} {
		bin, private := writeV3Fixture(t, agent, "joined-backup", dnsengineartifact.NativeCatalogDebian49V3)
		if err := checkV3Fixture(t, bin, private); err != nil {
			t.Fatalf("%s: %v", agent.name, err)
		}
	}
	noV2 := v3Agent{"no-v2-reader", alpha113ReleaseCommit, func(c *agentnativecontract.Contract) {
		c.DNSNativeCatalogPolicy = ""
		c.DNSEvidencePolicy = ""
	}}
	bin, private := writeV3Fixture(t, noV2, "joined-backup", dnsengineartifact.NativeCatalogDebian49V3)
	if err := checkV3Fixture(t, bin, private); !errors.Is(err, errDNSApplication) {
		t.Fatalf("got %v, want the v2 refusal", err)
	}
}

// publishedReleaseRow is the release record's "Published" row in ROADMAP.md.
var publishedReleaseRow = regexp.MustCompile("Release review of `(v0\\.1\\.0-alpha\\.([0-9]+))`[^|]*?\\*\\*Published [0-9]{4}-[0-9]{2}-[0-9]{2} \\(release commit `([0-9a-f]{7,40})`")

// A release's declared v3 compatibility must cover every marker the previous
// published release writes, in both directions: the candidate must accept the
// installed release's evidence (update) and the installed release's Agent must
// be admitted for the evidence after the update (rollback). The newest
// published release must be listed, so recording a release without declaring
// what its Agent writes fails here instead of on an owner's server.
func TestNativeCatalogV3DeclarationsCoverPublishedReleases(t *testing.T) {
	roadmap, err := os.ReadFile("../../ROADMAP.md")
	if err != nil {
		t.Fatal(err)
	}
	newest, newestVersion, newestCommit := -1, "", ""
	for _, match := range publishedReleaseRow.FindAllStringSubmatch(string(roadmap), -1) {
		if n, e := strconv.Atoi(match[2]); e == nil && n > newest {
			newest, newestVersion, newestCommit = n, match[1], match[3]
		}
	}
	if newest < 113 {
		t.Fatalf("no published release row found (newest %d)", newest)
	}
	listed := false
	for _, release := range NativeCatalogV3Releases {
		if release.Version == newestVersion {
			listed = true
			if release.SourceCommit != "" && !strings.HasPrefix(release.SourceCommit, newestCommit) {
				t.Fatalf("%s is recorded with release commit %s, listed with %s", newestVersion, newestCommit, release.SourceCommit)
			}
		}
	}
	if !listed {
		t.Fatalf("newest published release %s is not in NativeCatalogV3Releases: declare which v3 markers its Agent writes and how it is admitted", newestVersion)
	}
	current, _ := agentnativecontract.New([]byte("agent"), strings.Repeat("b", 40))
	currentKinds := nativeCatalogV3PolicyKinds[current.DNSNativeCatalogPolicy]
	// The current producer reads every marker this source can write.
	for _, kind := range dnsengineartifact.PDNSFreshPrimaryKinds() {
		if !slices.Contains(currentKinds, kind) {
			t.Fatalf("marker %s is written by this source but not declared by its Agent; add a new agentnativecontract value", kind)
		}
	}
	// The policy value never widens.
	if want := []string{"pdns-fresh-paired-primary/debian-4.9/v1", "pdns-fresh-paired-primary/arch-5.1/v1"}; !slices.Equal(nativeCatalogV3PolicyKinds["pdns-native-catalog-v3"], want) {
		t.Fatalf("pdns-native-catalog-v3 changed its markers: %v", nativeCatalogV3PolicyKinds["pdns-native-catalog-v3"])
	}
	for _, release := range NativeCatalogV3Releases {
		if (release.SourceCommit == "") == (release.Policy == "") || len(release.Kinds) == 0 {
			t.Fatalf("%s needs exactly one of a source commit or a policy, and its markers", release.Version)
		}
		contract := current
		contract.SourceCommit = strings.Repeat("c", 40)
		if release.SourceCommit != "" {
			contract.SourceCommit = release.SourceCommit
			contract.DNSNativeCatalogPolicy = ""
		} else {
			contract.DNSNativeCatalogPolicy = release.Policy
		}
		for _, kind := range release.Kinds {
			present := map[string]bool{kind: true}
			if !readsNativeCatalogV3(current, present) {
				t.Fatalf("this release's Agent cannot take over %s evidence written by %s", kind, release.Version)
			}
			if !readsNativeCatalogV3(contract, present) {
				t.Fatalf("%s's Agent is refused for its own %s evidence: rollback to it would be refused", release.Version, kind)
			}
		}
	}
}
