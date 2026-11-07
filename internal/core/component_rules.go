package core

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// The component dependency rule (alpha.123, D-045): the Linux package rule
// applied at component level. Every component declares what it REQUIRES
// (another component by id, or a capability any provider meets), what it
// PROVIDES (a capability seat such as "web-server": two providers of a seat
// CONFLICT, they bind the same role or port) and what, besides components,
// can require it: customer data and objects (sites, databases, mailboxes,
// zones, apps, certificates, VPN peers, quarantined files) and the panel
// itself (the panel firewall needs nftables). This one map is read by
// install (requirements and conflicts), removal ("what requires the thing
// being removed?"), the screens and the bundle chips. ManagedService.Requires
// and ManagedService.ConflictGroup are filled from it at start-up; the
// catalogue entries no longer carry them, so the rule has one home.
//
// Removal asks one question. A dependent that is customer data, or is itself
// used by something else, refuses the removal and is named with a link to
// where it can be moved or removed. A dependent that is only a leaf
// component (Together, nothing else uses it) is offered "remove together",
// ticked, and runs on the server as one durable operation. Nothing requires
// it: a plain confirmation that says what stops. Offerings of a removed
// component or version are withdrawn as part of the removal.
//
// Bileşen bağımlılık kuralı: Linux paket kuralının bileşen düzeyinde
// uygulanması. Her bileşen neyi GEREKTİRDİĞİNİ, hangi koltuğu SAĞLADIĞINI
// (aynı koltuğun iki sağlayıcısı ÇAKIŞIR) ve bileşenler dışında onu neyin
// gerektirebileceğini (müşteri verisi, panelin kendisi) bildirir. Tek harita;
// kurulum, kaldırma, ekranlar ve paket çipleri onu okur.

// What, besides another component, can require a component. A kind is
// counted by the Panel from its own database (or a bounded read of the
// server); every kind refuses the removal while it is not empty.
// Bileşen dışında bir bileşeni gerektirebilecek şeyler; her tür, boş
// değilken kaldırmayı reddeder.
const (
	UseSites             = "sites"              // every hosted site except DNS-only domains
	UseApacheSites       = "apache_sites"       // sites recorded on Apache
	UsePHPSites          = "php_sites"          // PHP sites (by version for one version)
	UseNodeSites         = "node_sites"         // Node.js sites (by version for one version)
	UseNodeServiceApps   = "node_service_apps"  // service apps on Node.js (n8n …)
	UsePythonApps        = "python_apps"        // Python runtime apps and Python service apps
	UseDatabases         = "databases"          // databases the Panel records
	UseMailAddresses     = "mail_addresses"     // mailboxes and forwardings
	UseDNSZones          = "dns_zones"          // local and remote DNS zones
	UseCertificates      = "certificates"       // certificates the Panel renews with certbot
	UsePanelFirewall     = "panel_firewall"     // the panel firewall (the panel itself)
	UseVPNPeers          = "vpn_peers"          // VPN peers
	UseMalwareQuarantine = "malware_quarantine" // files held in malware quarantine
)

// What stops when a component goes, said in the confirmation. A stop never
// refuses: it is the plain sentence of what the owner gives up and what keeps
// working.
// Bileşen gidince duran şey; onayda söylenir, asla reddetmez.
const (
	StopWebmail       = "webmail"        // mailboxes lose webmail; mail keeps working
	StopSpamFiltering = "spam_filtering" // mail is delivered unfiltered
	StopBanProtection = "ban_protection" // repeated failed logins are no longer banned
	StopMalwareScans  = "malware_scans"  // manual and scheduled malware scans
	StopFTP           = "ftp"            // accounts lose FTP; SFTP keeps working
	StopMonitoring    = "monitoring"     // the live monitoring dashboard
	StopUnknownUse    = "unknown_use"    // the Panel records no usage (caches)
	StopToolchain     = "toolchain"      // the administrator's own compiler
	StopDatabaseTool  = "database_tool"  // the web database administration page
	// alpha.128: hourly outgoing mail limits and the security pause stop;
	// mail keeps flowing (Postfix asks postfwd with default action DUNNO).
	StopOutgoingLimits = "outgoing_limits"
)

// TogetherMalwareSchedule is a setting turned off together with ClamAV: the
// weekly scheduled scan would otherwise fail every week.
// ClamAV ile birlikte kapatılan ayar: haftalık zamanlanmış tarama.
const TogetherMalwareSchedule = "setting:malware_schedule"

// ComponentRule is one component's entry in the map.
type ComponentRule struct {
	// Requires: component ids or capability seats ("web-server"). A seat is
	// met by any installed provider.
	Requires []string
	// Provides: the capability seat this component fills. At most one seat
	// per component; providers of one seat conflict.
	Provides []string
	// Conflicts: component ids that cannot be installed beside this one.
	// Always symmetric; providers of a seat list each other.
	Conflicts []string
	// Together: when this component is a dependent of a component being
	// removed and nothing else uses it, it is offered "remove together",
	// ticked. Runtimes, protection and anything holding data never are.
	Together bool
	// UsedBy: the kinds of customer data, objects and panel features that
	// require this component (Use* constants).
	UsedBy []string
	// Stops: what stops when it goes (Stop* constants).
	Stops []string
	// TogetherSettings: Panel settings turned off as part of its removal.
	TogetherSettings []string
	// NeverRemoved: the component is never removed by the Panel (cron: every
	// scheduled job needs it, including ones made outside the Panel).
	NeverRemoved bool
	// OwnWorkflow: removal goes through its own workflow (DNS engines).
	OwnWorkflow bool
}

// Capability seats.
const (
	SeatWebServer  = "web-server"
	SeatSMTPServer = "smtp-server"
	SeatIMAPServer = "imap-server"
	SeatSpamFilter = "spam-filter"
	SeatDNSServer  = "dns-server"
	SeatKVStore    = "kv-store"
)

// ComponentRules is the map. Every catalogue component has exactly one entry
// (a test pins it).
var ComponentRules = map[string]ComponentRule{
	"php-fpm": {UsedBy: []string{UsePHPSites}},
	"node":    {Requires: []string{SeatWebServer}, UsedBy: []string{UseNodeSites, UseNodeServiceApps}},
	"python":  {Requires: []string{SeatWebServer}, UsedBy: []string{UsePythonApps}},

	"nginx":  {Provides: []string{SeatWebServer}, Conflicts: []string{"apache"}, UsedBy: []string{UseSites}},
	"apache": {Provides: []string{SeatWebServer}, Conflicts: []string{"nginx"}, UsedBy: []string{UseApacheSites}},

	"postgresql": {UsedBy: []string{UseDatabases}},
	"mariadb":    {UsedBy: []string{UseDatabases}},
	"phpmyadmin": {Requires: []string{"mariadb", SeatWebServer, "php-fpm"}, Together: true, Stops: []string{StopDatabaseTool}},
	"phppgadmin": {Requires: []string{"postgresql", SeatWebServer, "php-fpm"}, Together: true, Stops: []string{StopDatabaseTool}},

	"postfix":      {Provides: []string{SeatSMTPServer}, Conflicts: []string{"exim"}, UsedBy: []string{UseMailAddresses}},
	"exim":         {Provides: []string{SeatSMTPServer}, Conflicts: []string{"postfix"}, UsedBy: []string{UseMailAddresses}},
	"dovecot":      {Provides: []string{SeatIMAPServer}, UsedBy: []string{UseMailAddresses}},
	"rspamd":       {Requires: []string{SeatSMTPServer}, Provides: []string{SeatSpamFilter}, Conflicts: []string{"spamassassin"}, Together: true, Stops: []string{StopSpamFiltering}},
	"spamassassin": {Requires: []string{SeatSMTPServer}, Provides: []string{SeatSpamFilter}, Conflicts: []string{"rspamd"}, Together: true, Stops: []string{StopSpamFiltering}},
	"roundcube":    {Requires: []string{SeatSMTPServer, SeatIMAPServer, SeatWebServer, "php-fpm"}, Together: true, Stops: []string{StopWebmail}},
	// alpha.128: postfwd is only asked by Postfix's submission services.
	"postfwd": {Requires: []string{SeatSMTPServer}, Together: true, Stops: []string{StopOutgoingLimits}},

	"wireguard": {UsedBy: []string{UseVPNPeers}},
	"certbot":   {UsedBy: []string{UseCertificates}},
	// alpha.128: Git deployment's client; nothing else depends on it.
	"git": {},
	// Fail2ban bans through nftables (banaction = nftables).
	"fail2ban": {Requires: []string{"nftables"}, Stops: []string{StopBanProtection}},
	// The panel firewall is the panel's own requirement.
	"nftables": {UsedBy: []string{UsePanelFirewall}},
	"clamav":   {UsedBy: []string{UseMalwareQuarantine}, Stops: []string{StopMalwareScans}, TogetherSettings: []string{TogetherMalwareSchedule}},

	"bind": {Provides: []string{SeatDNSServer}, Conflicts: []string{"pdns"}, UsedBy: []string{UseDNSZones}, OwnWorkflow: true},
	"pdns": {Provides: []string{SeatDNSServer}, Conflicts: []string{"bind"}, UsedBy: []string{UseDNSZones}, OwnWorkflow: true},

	"vsftpd": {Stops: []string{StopFTP}},

	"redis":     {Provides: []string{SeatKVStore}, Conflicts: []string{"valkey"}, Stops: []string{StopUnknownUse}},
	"valkey":    {Provides: []string{SeatKVStore}, Conflicts: []string{"redis"}, Stops: []string{StopUnknownUse}},
	"memcached": {Stops: []string{StopUnknownUse}},

	"netdata": {Stops: []string{StopMonitoring}},

	NativeCronServiceID: {NeverRemoved: true},

	"go":   {Stops: []string{StopToolchain}},
	"rust": {Stops: []string{StopToolchain}},
}

func init() {
	for i := range ManagedServices {
		rule := ComponentRules[ManagedServices[i].ID]
		ManagedServices[i].Requires = slices.Clone(rule.Requires)
		ManagedServices[i].ConflictGroup = ""
		if len(rule.Provides) > 0 {
			ManagedServices[i].ConflictGroup = rule.Provides[0]
		}
	}
}

// RuleFor returns a component's entry.
func RuleFor(id string) ComponentRule {
	return ComponentRules[id]
}

// isSeat reports whether a requirement names a capability seat.
func isSeat(requirement string) bool {
	for _, rule := range ComponentRules {
		if slices.Contains(rule.Provides, requirement) {
			return true
		}
	}
	return false
}

// SeatProviders lists the catalogue components that provide a seat, in
// catalogue order.
func SeatProviders(seat string) []string {
	out := []string{}
	for i := range ManagedServices {
		if slices.Contains(ComponentRules[ManagedServices[i].ID].Provides, seat) {
			out = append(out, ManagedServices[i].ID)
		}
	}
	return out
}

// RequiringComponents answers "which installed components require the
// target?": a component that names the target by id, or names a seat the
// target provides while no other installed provider fills it. Catalogue
// order.
// Hedefi gerektiren kurulu bileşenler: hedefi kimliğiyle adlandıran ya da
// hedefin sağladığı koltuğu, başka kurulu sağlayıcı yokken isteyen.
func RequiringComponents(target string, installed map[string]bool) []string {
	provides := ComponentRules[target].Provides
	out := []string{}
	for i := range ManagedServices {
		id := ManagedServices[i].ID
		if id == target || !installed[id] {
			continue
		}
		for _, requirement := range ComponentRules[id].Requires {
			if requirement == target {
				out = append(out, id)
				break
			}
			if slices.Contains(provides, requirement) && !seatFilledWithout(requirement, target, installed) {
				out = append(out, id)
				break
			}
		}
	}
	return out
}

func seatFilledWithout(seat, without string, installed map[string]bool) bool {
	for _, provider := range SeatProviders(seat) {
		if provider != without && installed[provider] {
			return true
		}
	}
	return false
}

// RuleRequires reports whether member's rule requires target, by id or by a
// seat target provides (whatever else is installed).
func RuleRequires(member, target string) bool {
	provides := ComponentRules[target].Provides
	for _, requirement := range ComponentRules[member].Requires {
		if requirement == target || slices.Contains(provides, requirement) {
			return true
		}
	}
	return false
}

// MayBeRequired reports whether any catalogue component's rule requires
// target, by id or by a seat it provides. When none does, the installed set
// cannot change a removal plan of target.
func MayBeRequired(target string) bool {
	for id := range ComponentRules {
		if id != target && RuleRequires(id, target) {
			return true
		}
	}
	return false
}

// RemovableTogether reports whether member may be removed together with
// target by the rule: member is a Together component that requires target.
// The Agent asks this too before it accepts a remove-together lease; the
// Panel additionally checks that nothing else uses member.
// RemovableTogether: üye, hedefle birlikte kaldırılabilir mi.
func RemovableTogether(target, member string, installed map[string]bool) bool {
	if target == member || !ComponentRules[member].Together {
		return false
	}
	return slices.Contains(RequiringComponents(target, installed), member)
}

// RemoveTogetherQualifier is the canonical form of a remove-together member
// list: sorted, unique, comma separated. It is the lease's qualifier.
func RemoveTogetherQualifier(members []string) string {
	set := map[string]bool{}
	for _, member := range members {
		if member = strings.TrimSpace(member); member != "" {
			set[member] = true
		}
	}
	out := make([]string, 0, len(set))
	for member := range set {
		out = append(out, member)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// ParseRemoveTogetherQualifier is the inverse; it refuses a list that is not
// canonical, names an unknown component or a component that is never a
// together member, or holds more than eight.
func ParseRemoveTogetherQualifier(qualifier string) ([]string, error) {
	if qualifier == "" {
		return nil, fmt.Errorf("empty remove-together list")
	}
	members := strings.Split(qualifier, ",")
	if len(members) > 8 || RemoveTogetherQualifier(members) != qualifier {
		return nil, fmt.Errorf("remove-together list is not canonical")
	}
	for _, member := range members {
		if GetManagedServiceByID(member) == nil || !ComponentRules[member].Together {
			return nil, fmt.Errorf("%s is not removed together with another component", member)
		}
	}
	return members, nil
}

// ValidateComponentRules checks the map's shape: every catalogue component
// has an entry and nothing else does; requirements name a component or a
// seat; at most one seat per component; conflicts are symmetric and providers
// of one seat conflict; requirements have no cycle. A test runs it.
func ValidateComponentRules() error {
	ids := map[string]bool{}
	for i := range ManagedServices {
		ids[ManagedServices[i].ID] = true
		if _, ok := ComponentRules[ManagedServices[i].ID]; !ok {
			return fmt.Errorf("%s has no dependency rule", ManagedServices[i].ID)
		}
	}
	for id, rule := range ComponentRules {
		if !ids[id] {
			return fmt.Errorf("rule %s names no catalogue component", id)
		}
		if len(rule.Provides) > 1 {
			return fmt.Errorf("%s provides more than one seat", id)
		}
		for _, requirement := range rule.Requires {
			if !ids[requirement] && !isSeat(requirement) {
				return fmt.Errorf("%s requires unknown %s", id, requirement)
			}
			if requirement == id {
				return fmt.Errorf("%s requires itself", id)
			}
		}
		for _, other := range rule.Conflicts {
			if !ids[other] || other == id {
				return fmt.Errorf("%s conflicts with unknown %s", id, other)
			}
			if !slices.Contains(ComponentRules[other].Conflicts, id) {
				return fmt.Errorf("conflict %s→%s is not symmetric", id, other)
			}
		}
		for _, seat := range rule.Provides {
			for _, provider := range SeatProviders(seat) {
				if provider != id && !slices.Contains(rule.Conflicts, provider) {
					return fmt.Errorf("%s and %s provide %s but do not conflict", id, provider, seat)
				}
			}
		}
		if rule.Together && len(rule.UsedBy) > 0 {
			return fmt.Errorf("%s holds data and cannot be removed together", id)
		}
	}
	// Cycles: depth-first over requirements, a seat expanding to its providers.
	state := map[string]int{}
	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("requirement cycle: %s", strings.Join(append(path, id), " → "))
		case 2:
			return nil
		}
		state[id] = 1
		for _, requirement := range ComponentRules[id].Requires {
			next := []string{requirement}
			if !ids[requirement] {
				next = SeatProviders(requirement)
			}
			for _, n := range next {
				if err := visit(n, append(path, id)); err != nil {
					return err
				}
			}
		}
		state[id] = 2
		return nil
	}
	for i := range ManagedServices {
		if err := visit(ManagedServices[i].ID, nil); err != nil {
			return err
		}
	}
	return nil
}

// Install together (alpha.124, D-045 follow-up). When a component's
// requirement is missing and can be installed here on its own, the install
// installs it first, in the same durable operation (service_install_together:
// one Agent lease, target = the component, qualifier = the canonical member
// list). Only these components are installed together: plain distribution
// components with nothing of their own to choose at that moment. Mail engines
// go through the mail setup (they need its host name), DNS engines through
// their workflow and runtimes through their version list; a missing
// requirement of that kind is said, not installed.
// Birlikte kurulum: eksik ve burada tek başına kurulabilen gereksinim, aynı
// kalıcı işlemde önce kurulur.
var installTogetherMembers = map[string]bool{
	"nginx":      true,
	"php-fpm":    true,
	"mariadb":    true,
	"postgresql": true,
	"nftables":   true,
}

// InstallableTogether reports whether id may be installed as another
// component's requirement in an install-together operation.
func InstallableTogether(id string) bool {
	return installTogetherMembers[id]
}

// InstallTogetherQualifier is the canonical member list (the lease's
// qualifier): sorted, unique, comma separated.
func InstallTogetherQualifier(members []string) string {
	return RemoveTogetherQualifier(members)
}

// ParseInstallTogetherQualifier is the inverse for one target: a canonical
// list of at most eight members, each installable together, none the target,
// and each required by the target or by another member (directly). The
// Agent asks this before it accepts a step of an install-together lease.
func ParseInstallTogetherQualifier(target, qualifier string) ([]string, error) {
	if qualifier == "" {
		return nil, fmt.Errorf("empty install-together list")
	}
	if GetManagedServiceByID(target) == nil {
		return nil, fmt.Errorf("unknown install-together target %s", target)
	}
	members := strings.Split(qualifier, ",")
	if len(members) > 8 || InstallTogetherQualifier(members) != qualifier {
		return nil, fmt.Errorf("install-together list is not canonical")
	}
	for _, member := range members {
		if member == target || GetManagedServiceByID(member) == nil || !InstallableTogether(member) {
			return nil, fmt.Errorf("%s is not installed together with another component", member)
		}
		needed := RuleRequires(target, member)
		for _, other := range members {
			if other != member && RuleRequires(other, member) {
				needed = true
			}
		}
		if !needed {
			return nil, fmt.Errorf("%s is not a requirement of %s", member, target)
		}
	}
	return members, nil
}
