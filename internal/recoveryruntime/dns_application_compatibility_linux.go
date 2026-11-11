//go:build linux

package recoveryruntime

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/alicelik/celikpanel/internal/agentnativecontract"
	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/transport"
	"golang.org/x/sys/unix"
)

var errDNSSwitchJournalApplication = errors.New("active DNS switch journal requires an Agent that reads the exact v2 BIND inverse plan; preserve the evidence and choose a compatible release")
var errDNSSwitchJournalApplicationV4 = errors.New("active v4 PowerDNS target journal requires a proved independent recovery runtime; preserve the evidence and choose a compatible release")

// The text is the updater's diagnostic line: short enough to reach the owner whole.
var errDNSApplicationV3 = errors.New("this Agent does not declare support for the PowerDNS main server's v3 DNS state; preserve that state and use a release that does")

var errDNSApplication = errors.New("DNS evidence requires an Agent with separated acquisition/publication support; preserve the evidence and choose a compatible release")

// CheckDNSApplicationCompatibility observes protected DNS evidence and target
// Agent bytes. It changes no files, starts no daemon and grants no mutation.
// The caller supplies trusted release/snapshot provenance and release/host
// exclusion, and repeats this check before application publication.
func CheckDNSApplicationCompatibility(binaryDirectory, stateDirectory string) error {
	return checkDNSApplicationCompatibility(binaryDirectory, stateDirectory, nil)
}

func checkDNSApplicationCompatibility(bin, root string, beforeRevalidate func()) error {
	if os.Geteuid() != 0 || !filepath.IsAbs(bin) || filepath.Clean(bin) != bin || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fail(ReasonUnsafeMetadata)
	}
	state := &runtimeState{config: resolveConfig{anchor: "/", uid: 0, gid: 0, protectedDirectoryGroups: true}}
	defer state.close()
	parent, absence, err := optionalMailHookParent(state, root)
	if err != nil {
		return err
	}
	absences := []func() error{}
	if absence != nil {
		absences = append(absences, absence)
	}
	required := false
	requireSwitchV2 := false
	requireSwitchV4 := false
	// Markers of fresh PowerDNS main server (v3) evidence present on this host.
	freshV3 := map[string]bool{}
	requireSourceProof := false
	requireAdoptionProof := false
	if parent != nil {
		// Private evidence can retain its historical root:service group. Mode 0600
		// gives that group no access. Pin it exactly; never normalize group metadata.
		if parent.stat.Mode&07777 != 0700 {
			return fail(ReasonUnsafeMetadata)
		}
		for _, entry := range []struct {
			name   string
			engine transport.DNSEngine
		}{
			{"dns-engine-state.json", ""},
			{"dns-engine-ownership-bind.json", transport.DNSEngineBIND},
			{"dns-engine-ownership-pdns.json", transport.DNSEnginePowerDNS},
			{"dns-engine-switch-journal.json", ""},
		} {
			var st unix.Stat_t
			if e := unix.Fstatat(int(parent.file.Fd()), entry.name, &st, unix.AT_SYMLINK_NOFOLLOW); errors.Is(e, unix.ENOENT) {
				name := entry.name
				absences = append(absences, func() error { return requireMailPathAbsent(parent, name) })
				continue
			} else if e != nil {
				return asReadError(e)
			}
			limit := int64(64 << 10)
			if entry.name == "dns-engine-switch-journal.json" {
				limit = dnsengineartifact.SwitchJournalLimit
			}
			file, e := state.openFileWithGID(parent, entry.name, 0600, limit, st.Gid)
			if e != nil {
				return asReadError(e)
			}
			if !sameFile(st, file.stat) {
				return fail(ReasonChanged)
			}
			if e = refusePromotionXattrs(file); e != nil {
				return e
			}
			raw, e := file.readBounded()
			if e != nil {
				return e
			}
			file.digest = Digest(raw)
			if entry.name == "dns-engine-switch-journal.json" {
				policy := dnsengineartifact.JournalPolicy{
					StatePath: filepath.Join(root, "dns-engine-state.json"), StateUID: 0, StateGID: st.Gid, RequireOwner: true,
					PDNSMainPath:     "/etc/powerdns/pdns.conf",
					PDNSManagedPath:  "/etc/powerdns/pdns.d/celikpanel.conf",
					PDNSClusterPath:  "/etc/powerdns/pdns.d/celikpanel-cluster.conf",
					PDNSDatabasePath: "/var/lib/powerdns/pdns.sqlite3",
				}
				journal, decodeErr := policy.DecodeSwitchJournal(raw)
				if decodeErr != nil {
					return fail(ReasonUnsupported)
				}
				requireSwitchV2 = journal.Schema == dnsengineartifact.SwitchJournalSchemaV2
				requireSwitchV4 = journal.Schema == dnsengineartifact.SwitchJournalSchemaV4
				if journal.Schema == dnsengineartifact.SwitchJournalSchemaV3 {
					// The decoder proved the plan and its tested marker.
					freshV3[journal.PDNSFreshPlan.Kind] = true
				}
				requireSourceProof = requireSwitchV2 && journal.InversePlan != nil && journal.InversePlan.SourcePDNS != nil
				requireAdoptionProof = requireSwitchV2 && journal.InversePlan != nil && journal.InversePlan.SourceBIND != nil
				continue
			}
			var receipt dnsengineartifact.StateV1
			var separated bool
			if entry.engine == "" {
				receipt, separated, e = dnsengineartifact.DecodeStateDocument(raw)
			} else {
				receipt, separated, e = dnsengineartifact.DecodeOwnershipDocument(raw)
			}
			if e != nil || (entry.engine != "" && receipt.Engine != entry.engine) {
				return fail(ReasonUnsupported)
			}
			required = required || separated
			if receipt.NativeCatalogV3 != "" {
				freshV3[receipt.NativeCatalogV3] = true
			}
		}
	}
	var target *CompatibleMailAgent
	if required || requireSwitchV2 || len(freshV3) > 0 {
		target, err = InspectCompatibleMailAgent(bin)
		if err != nil {
			return err
		}
		defer target.Close()
		if required && target.Contract.DNSEvidencePolicy != agentnativecontract.DNSEvidencePolicy {
			return errDNSApplication
		}
		// V3 needs its own declaration; the separated-evidence marker alone
		// cannot stand in for it (alpha.81). This checker is the candidate's
		// independent runtime and decoded every marker itself above.
		if len(freshV3) > 0 && !readsNativeCatalogV3(target.Contract, freshV3) {
			return errDNSApplicationV3
		}
		if requireSwitchV2 && !supportsBINDJournalPolicy(target.Contract.DNSSwitchJournalPolicy, requireSourceProof, requireAdoptionProof) {
			return errDNSSwitchJournalApplication
		}
	}
	if requireSwitchV4 {
		return errDNSSwitchJournalApplicationV4
	}
	if beforeRevalidate != nil {
		beforeRevalidate()
	}
	if target != nil {
		if err = target.Revalidate(); err != nil {
			return err
		}
	}
	if err = state.revalidate(); err != nil {
		return err
	}
	for _, check := range absences {
		if err = check(); err != nil {
			return err
		}
	}
	return state.revalidate()
}

func supportsBINDJournalPolicy(policy string, sourceProof, adoptionProof bool) bool {
	switch policy {
	case agentnativecontract.DNSSwitchAdoptionJournalPolicy:
		return true
	case agentnativecontract.DNSSwitchSourceJournalPolicy:
		return !adoptionProof
	case agentnativecontract.DNSSwitchJournalPolicy:
		return !sourceProof && !adoptionProof
	default:
		return false
	}
}

// nativeCatalogV3PolicyKinds pins the markers each declared Agent generation
// reads. A value never widens: a new marker or document schema gets a new
// agentnativecontract value, so an older Agent is refused for it instead of
// misreading it.
var nativeCatalogV3PolicyKinds = map[string][]string{
	agentnativecontract.DNSNativeCatalogPolicy: {dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3},
}

// NativeCatalogV3Release is a published release whose Agent writes fresh
// PowerDNS main server (v3) evidence, and how the checker admits that Agent:
// by its declared Policy, or, for a release published before the declaration
// existed, by the exact source commit its signed native contract names.
type NativeCatalogV3Release struct {
	Version      string
	SourceCommit string
	Policy       string
	Kinds        []string
}

// NativeCatalogV3Releases lists every published release whose Agent writes v3
// evidence; the release record adds each new one (a test requires the newest
// published release here). Releases before alpha.113 are not listed: their
// Agents stay refused for v3 evidence.
//
// alpha.113 (release commit d3f3a323d) wrote state-v3/ownership-v3 documents
// and v3 journals for both markers before any Agent could declare them. Its
// Agent is admitted by that exact commit: the DNS evidence package, the
// Agent's DNS engine code, the independent recovery runtime and the native
// contract were byte-identical between that commit and the alpha.114
// integration source that added the declaration, so it reads and keeps what
// alpha.114 writes, and the reverse. Application rollback keeps native DNS
// evidence in place; the restored alpha.113 Agent operates it unchanged.
var NativeCatalogV3Releases = []NativeCatalogV3Release{
	{Version: "v0.1.0-alpha.113", SourceCommit: "d3f3a323da46ce8f8dca5ad4385666c5428c353e",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	// alpha.114 (release commit e71a71bd3) is the first release whose Agent
	// declares the markers it writes.
	{Version: "v0.1.0-alpha.114", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.115", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.116", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.117", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.118", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.119", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.120", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.121", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.122", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.123", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.124", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.125", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.126", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.127", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.128", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.129", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.130", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
	{Version: "v0.1.0-alpha.131", Policy: "pdns-native-catalog-v3",
		Kinds: []string{dnsengineartifact.NativeCatalogDebian49V3, dnsengineartifact.NativeCatalogArch51V3}},
}

func readsNativeCatalogV3(contract agentnativecontract.Contract, present map[string]bool) bool {
	var kinds []string
	if contract.DNSNativeCatalogPolicy != "" {
		kinds = nativeCatalogV3PolicyKinds[contract.DNSNativeCatalogPolicy]
	} else if contract.DNSEvidencePolicy == agentnativecontract.DNSEvidencePolicy &&
		contract.DNSSwitchJournalPolicy == agentnativecontract.DNSSwitchAdoptionJournalPolicy {
		for _, release := range NativeCatalogV3Releases {
			if release.Policy == "" && release.SourceCommit == contract.SourceCommit {
				kinds = release.Kinds
			}
		}
	}
	if len(kinds) == 0 {
		return false
	}
	for kind := range present {
		if !slices.Contains(kinds, kind) {
			return false
		}
	}
	return true
}
