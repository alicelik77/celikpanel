package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

const (
	errCodeMailProfileDNSIdentityNotReady = "mail_profile_dns_identity_not_ready"
	mailProfileDNSIdentityMessage         = "DNS identity is not ready. Install a supported DNS server and save the shared nameserver names and operating mode in Server → DNS and peer servers before installing a mail profile."
)

var errMailProfileDNSIdentityNotReady = errors.New("mail profile DNS identity is not ready")

var mailProfileDNSIdentitySettingKeys = [...]string{
	settingNS1,
	settingNS2,
	settingDNSRole,
	settingDNSPeerIP,
	settingDNSPeerNS,
}

// savedDNSIdentityConfiguredStrict reads the complete saved tuple in one SQL
// snapshot. The ordinary settings helper intentionally turns missing rows and
// database failures into empty strings; an install preflight must distinguish
// an incomplete identity from a state it could not verify.
func (p *Panel) savedDNSIdentityConfiguredStrict(ctx context.Context) (bool, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT key, value
		FROM panel_settings
		WHERE key IN (?, ?, ?, ?, ?)`,
		mailProfileDNSIdentitySettingKeys[0],
		mailProfileDNSIdentitySettingKeys[1],
		mailProfileDNSIdentitySettingKeys[2],
		mailProfileDNSIdentitySettingKeys[3],
		mailProfileDNSIdentitySettingKeys[4],
	)
	if err != nil {
		return false, fmt.Errorf("read saved DNS identity: %w", err)
	}
	defer rows.Close()

	values := make(map[string]string, len(mailProfileDNSIdentitySettingKeys))
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return false, fmt.Errorf("decode saved DNS identity: %w", err)
		}
		values[key] = strings.TrimSpace(value)
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read saved DNS identity rows: %w", err)
	}

	ns1 := canonicalDNSName(values[settingNS1])
	ns2 := canonicalDNSName(values[settingNS2])
	if !validDNSHostname(ns1) || !validDNSHostname(ns2) || ns1 == ns2 {
		return false, nil
	}
	switch normalizeDNSRole(values[settingDNSRole]) {
	case "standalone":
		return true, nil
	case "paired":
		peerIP := net.ParseIP(values[settingDNSPeerIP])
		peerNS := canonicalDNSName(values[settingDNSPeerNS])
		return peerIP != nil && peerIP.To4() != nil && peerIP.IsGlobalUnicast() &&
			validDNSHostname(peerNS) && (peerNS == ns1 || peerNS == ns2), nil
	default:
		return false, nil
	}
}

// mailProfileDNSIdentityReady is the one strict prerequisite used by both the
// read-only Components contract and the mutation preflight: durable engine
// identity and strict runtime readiness must agree on exactly one managed
// publisher, and the complete identity must already be saved. Package presence
// alone is never publication authority. Public delegation is deliberately not
// required here; that is a later operational readiness check.
func (p *Panel) mailProfileDNSIdentityReady(ctx context.Context) (bool, error) {
	mode, err := p.setupDNSManagementMode(ctx)
	if err != nil {
		return false, err
	}
	if mode == setupDNSModeExternal {
		return true, nil
	}
	if mode == setupDNSModeExisting {
		_, err := p.remoteDNSConnectionForCreation(ctx, "")
		return err == nil, err
	}
	_, ready, err := p.activeDNSPublisher(ctx)
	if err != nil {
		return false, fmt.Errorf("verify active DNS publisher: %w", err)
	}
	if !ready {
		return false, nil
	}
	return p.savedDNSIdentityConfiguredStrict(ctx)
}

func (p *Panel) requireMailProfileDNSIdentity(ctx context.Context) error {
	ready, err := p.mailProfileDNSIdentityReady(ctx)
	if err != nil {
		return err
	}
	if !ready {
		return errMailProfileDNSIdentityNotReady
	}
	return nil
}

// managedServicesDNSIdentityReady fails closed for the read-only catalogue.
// A transient agent/database failure must never leave an enabled mail action;
// the mutation endpoint repeats the strict proof and retains the detailed
// server-only cause.
func (p *Panel) managedServicesDNSIdentityReady(ctx context.Context) bool {
	ready, err := p.mailProfileDNSIdentityReady(ctx)
	return err == nil && ready
}

func mailProfileCatalogBlockedReason(hostBlockedReason string, dnsIdentityReady bool) string {
	if hostBlockedReason != "" {
		return hostBlockedReason
	}
	if !dnsIdentityReady {
		return mailProfileDNSIdentityMessage
	}
	return ""
}

// mailProfileDNSGuidance is the truth the Components page says when the mail
// profiles wait for a DNS identity on a server whose DNS role is secondary
// (2026-10-16). A secondary serves copies of the primary's zones and cannot
// create a record, so "Configure DNS" on this server is a dead end: the record
// for the mail host name is created on the primary DNS server (named here from
// the saved DNS infrastructure settings) or at the external DNS provider. The
// exact record is given when it is known: the name the mail install would use
// and this server's public IPv4 address.
// Ikincil DNS sunucusu kayit olusturamaz; posta adinin kaydi birincilde ya da
// dis saglayicida olusturulur. Bilinen kayit adi, turu ve degeriyle soylenir.
type mailProfileDNSGuidance struct {
	Reason            string `json:"reason"`
	PrimaryNameserver string `json:"primary_nameserver,omitempty"`
	PrimaryIP         string `json:"primary_ip,omitempty"`
	RecordName        string `json:"record_name,omitempty"`
	RecordType        string `json:"record_type"`
	RecordValue       string `json:"record_value,omitempty"`
	// PeerRecordChannel is additive (2026-10-16): this server has an
	// authorized DNS connection to the primary, so setup asks the primary
	// for the record by itself while it checks public DNS; without it the
	// record above is added by hand.
	// Birincile yetkili DNS baglantisi var: kurulum kaydi kendisi ister.
	PeerRecordChannel bool `json:"peer_record_channel"`
}

const mailProfileDNSGuidanceSecondary = "dns_secondary"

// mailProfileDNSGuidanceFor answers nil unless the identity is not ready and
// this server's DNS engine is the secondary of a pair. Reads only.
func (p *Panel) mailProfileDNSGuidanceFor(ctx context.Context, dnsIdentityReady bool) *mailProfileDNSGuidance {
	if dnsIdentityReady {
		return nil
	}
	if mode, err := p.setupDNSManagementMode(ctx); err != nil || mode != setupDNSModeLocal {
		return nil
	}
	state, err := readDNSEngineDBState(ctx, p.db.GetDB())
	if err != nil || state.ActiveEngine == "" || state.PairRole != transport.DNSPairRoleSecondary {
		return nil
	}
	guidance := &mailProfileDNSGuidance{Reason: mailProfileDNSGuidanceSecondary, RecordType: "A"}
	if name := canonicalDNSName(p.setting(ctx, settingDNSPeerNS)); validDNSHostname(name) {
		guidance.PrimaryNameserver = name
	}
	if address, ok := canonicalIPv4(p.setting(ctx, settingDNSPeerIP)); ok {
		guidance.PrimaryIP = address
	}
	if identity := p.mailHostnameIdentity(ctx); identity.Hostname != "" {
		guidance.RecordName = identity.Hostname
	}
	candidates := []string{}
	if state, err := p.loadServerSetup(ctx); err == nil {
		candidates = append(candidates, state.Draft.LocalIP)
		_, guidance.PeerRecordChannel = p.serverSetupPeerHostRecordConnection(ctx, state.Draft)
	}
	candidates = append(candidates, serverPrimaryIP())
	for _, candidate := range candidates {
		address, ok := canonicalIPv4(candidate)
		if parsed := net.ParseIP(address); ok && parsed != nil && parsed.IsGlobalUnicast() && !parsed.IsPrivate() {
			guidance.RecordValue = address
			break
		}
	}
	return guidance
}
