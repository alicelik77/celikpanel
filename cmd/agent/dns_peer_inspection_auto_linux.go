//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alicelik/celikpanel/internal/binddns"
	"github.com/alicelik/celikpanel/internal/dnspeerenrollment"
	"github.com/alicelik/celikpanel/internal/dnspeerenrollowner"
	"github.com/alicelik/celikpanel/internal/dnspeertransport"
	"github.com/alicelik/celikpanel/internal/pdnspeerenrollment"
	"github.com/alicelik/celikpanel/internal/transport"
	"golang.org/x/crypto/ssh"
)

// Automatic peer inspection setup on a pair joined by code (2026-10-30).
//
// The owner tool dns-peer-enroll has four manual steps on two servers. Here
// the same library calls run in each server's Agent, driven by the backup's
// panel over the private setup link: primary-prepare and primary-activate on
// the main (DNSPeerInspectionPrimaryV1), secondary-install and
// secondary-host-key on the backup (DNSPeerInspectionSecondaryV1). The native
// result is byte-for-byte what the owner tool writes (the same files, the same
// locked celikpeer account, forced command, sudoers rule and sshd Include), so
// it keeps working without the panel and is revoked with
// dns-peer-enroll secondary-revoke / primary-revoke.
//
// Only this pair's own values are used: the addresses and engine come from
// this Agent's DNS engine state, never from the request; the request carries
// only public material (the main's public key, the backup's host-key digest).
// Any enrollment, channel file or owner SSH configuration this automation did
// not write is reported and never replaced. A channel it set up and the owner
// later revoked is never authorized again by it.
//
// Otomatik denetim kurulumu: sahip aracinin yazdigi dosyalarin aynisi; sahibin
// kaydina ve SSH yapilandirmasina dokunulmaz, yalniz bildirilir.

const (
	// dnsOwnerToolsDir is where install.sh places the packaged owner tools
	// (fresh installations from this release on).
	dnsOwnerToolsDir = "/opt/celikpanel/dns-owner-tools"
	// dnsPeerInspectionAutoMarkerPath records, on the backup, that this
	// automation installed (or is installing) the channel, for which key.
	// It tells an interrupted install (resume) from an owner's revocation
	// (never re-authorized) or an owner's own staged files (reported).
	dnsPeerInspectionAutoMarkerPath = "/var/lib/celikpanel-agent-private/dns-peer-inspection-auto-secondary-v1.json"
	// dnsPeerInspectionPrimaryKeyPath holds the main's public key while the
	// library installs it (root-only; public material).
	dnsPeerInspectionPrimaryKeyPath = "/var/lib/celikpanel-agent-private/dns-peer-inspection-primary-key.pub"
	dnsPeerInspectionAutoSchema     = "celikpanel-dns-peer-inspection-auto-secondary/v1"
	dnsPeerInspectionAutoInstalling = "installing"
	dnsPeerInspectionAutoInstalled  = "installed"
)

var dnsPeerInspectionMu sync.Mutex

type dnsPeerInspectionAutoMarker struct {
	Schema           string `json:"schema"`
	Engine           string `json:"engine"`
	PrimaryIP        string `json:"primary_ip"`
	PeerIP           string `json:"peer_ip"`
	CatalogName      string `json:"catalog_name"`
	PrimaryKeySHA256 string `json:"primary_key_sha256"`
	State            string `json:"state"`
}

// dnsPeerInspectionPrepared is the public result of a primary preparation.
type dnsPeerInspectionPrepared struct{ CredentialID, PublicKey string }

// dnsPeerInspectionRecord is the engine-neutral primary enrollment record.
type dnsPeerInspectionRecord struct {
	Engine, PrimaryIP, PeerIP, CatalogName, CredentialID, HostKeySHA256 string
	Transport                                                           dnspeertransport.Enrollment
}

// dnsPeerInspectionHost is every native effect of the automation; tests
// replace it.
type dnsPeerInspectionHost struct {
	engineState func() (dnsEngineStateReceipt, bool, error)
	// Primary side.
	readEnrollment func(engine string) (dnsPeerInspectionRecord, bool, error)
	prepare        func(engine string) (dnsPeerInspectionPrepared, error)
	preparedPublic func(engine, credentialID string) (dnsPeerInspectionPrepared, error)
	activate       func(engine string, input pdnspeerenrollment.OwnerActivation) error
	probe          func(ctx context.Context, enrollment dnspeertransport.Enrollment) dnspeertransport.ProbeResult
	// Secondary side.
	inspectorPresent func(engine string) bool
	sshPorts         func() ([]int, error)
	otherEngine      func(engine string) (string, error)
	status           func(engine string) (dnspeerenrollowner.Status, error)
	install          func(engine string, options dnspeerenrollowner.SecondaryOptions, resume bool) error
	hostKey          func() (string, error)
	readMarker       func() (dnsPeerInspectionAutoMarker, bool, error)
	writeMarker      func(dnsPeerInspectionAutoMarker) error
	writePublicKey   func(raw []byte) (string, error)
}

func nativeDNSPeerInspectionHost() dnsPeerInspectionHost {
	return dnsPeerInspectionHost{
		engineState:    readExactDNSEngineState,
		readEnrollment: readNativeDNSPeerEnrollment,
		prepare: func(engine string) (dnsPeerInspectionPrepared, error) {
			if engine == string(transport.DNSEnginePowerDNS) {
				p, err := pdnspeerenrollment.PrepareOwner()
				return dnsPeerInspectionPrepared{p.CredentialID, p.PublicKey}, err
			}
			p, err := dnspeerenrollment.PrepareOwner()
			return dnsPeerInspectionPrepared{p.CredentialID, p.PublicKey}, err
		},
		preparedPublic: func(engine, id string) (dnsPeerInspectionPrepared, error) {
			if engine == string(transport.DNSEnginePowerDNS) {
				p, err := pdnspeerenrollment.PreparedOwnerPublic(id)
				return dnsPeerInspectionPrepared{p.CredentialID, p.PublicKey}, err
			}
			p, err := dnspeerenrollment.PreparedOwnerPublic(id)
			return dnsPeerInspectionPrepared{p.CredentialID, p.PublicKey}, err
		},
		activate: func(engine string, in pdnspeerenrollment.OwnerActivation) error {
			if engine == string(transport.DNSEnginePowerDNS) {
				_, err := pdnspeerenrollment.ActivateOwner(in)
				return err
			}
			_, err := dnspeerenrollment.ActivateOwner(dnspeerenrollment.OwnerActivation(in))
			return err
		},
		probe: dnspeertransport.Probe,
		inspectorPresent: func(engine string) bool {
			info, err := os.Lstat(dnsOwnerToolsDir + "/" + engine + "-peer-inspect")
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0100 == 0 {
				return false
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			return ok && stat.Uid == 0
		},
		sshPorts: dnsPeerInspectionSSHPorts,
		otherEngine: func(engine string) (string, error) {
			return dnspeerenrollowner.OtherEngineArtifact(dnspeerenrollowner.Engine(engine))
		},
		status: func(engine string) (dnspeerenrollowner.Status, error) {
			return dnspeerenrollowner.SecondaryStatusEngine(dnspeerenrollowner.Engine(engine))
		},
		install: func(engine string, o dnspeerenrollowner.SecondaryOptions, resume bool) error {
			if resume {
				return dnspeerenrollowner.ResumeSecondaryEngine(dnspeerenrollowner.Engine(engine), o)
			}
			return dnspeerenrollowner.InstallSecondaryEngine(dnspeerenrollowner.Engine(engine), o)
		},
		hostKey:     dnspeerenrollowner.SecondaryHostKeySHA256,
		readMarker:  readDNSPeerInspectionAutoMarker,
		writeMarker: writeDNSPeerInspectionAutoMarker,
		writePublicKey: func(raw []byte) (string, error) {
			return dnsPeerInspectionPrimaryKeyPath, secureWriteConfig(dnsPeerInspectionPrimaryKeyPath, raw, 0600)
		},
	}
}

var dnsPeerInspectionNativeHost = nativeDNSPeerInspectionHost

func readNativeDNSPeerEnrollment(engine string) (dnsPeerInspectionRecord, bool, error) {
	if engine == string(transport.DNSEnginePowerDNS) {
		s, err := pdnspeerenrollment.Read()
		if pdnspeerenrollment.IsCode(err, pdnspeerenrollment.Disabled) {
			return dnsPeerInspectionRecord{}, false, nil
		}
		if err != nil {
			return dnsPeerInspectionRecord{}, false, err
		}
		r := s.Record
		return dnsPeerInspectionRecord{engine, r.PrimaryIP, r.PeerIP, r.CatalogName, r.CredentialID, r.HostKeySHA256, s.Transport}, true, nil
	}
	s, err := dnspeerenrollment.Read()
	if dnspeerenrollment.IsCode(err, dnspeerenrollment.Disabled) {
		return dnsPeerInspectionRecord{}, false, nil
	}
	if err != nil {
		return dnsPeerInspectionRecord{}, false, err
	}
	r := s.Record
	return dnsPeerInspectionRecord{engine, r.PrimaryIP, r.PeerIP, r.CatalogName, r.CredentialID, r.HostKeySHA256, s.Transport}, true, nil
}

// dnsPeerInspectionSSHPorts are the ports OpenSSH accepts connections on: the
// systemd socket's when socket activation is configured, else sshd -T's.
func dnsPeerInspectionSSHPorts() ([]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runner := hostFirewallCommandRunner{ctx: ctx}
	if ports, err := runner.ConfiguredSSHSocketPorts(); err == nil && len(ports) > 0 {
		return ports, nil
	}
	return runner.ConfiguredSSHPorts()
}

func readDNSPeerInspectionAutoMarker() (dnsPeerInspectionAutoMarker, bool, error) {
	raw, err := secureReadConfig(dnsPeerInspectionAutoMarkerPath)
	if errors.Is(err, os.ErrNotExist) {
		return dnsPeerInspectionAutoMarker{}, false, nil
	}
	if err != nil {
		return dnsPeerInspectionAutoMarker{}, false, err
	}
	var marker dnsPeerInspectionAutoMarker
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil || marker.Schema != dnsPeerInspectionAutoSchema ||
		(marker.State != dnsPeerInspectionAutoInstalling && marker.State != dnsPeerInspectionAutoInstalled) {
		return dnsPeerInspectionAutoMarker{}, false, errors.New("automatic peer inspection marker is not canonical")
	}
	return marker, true, nil
}

func writeDNSPeerInspectionAutoMarker(marker dnsPeerInspectionAutoMarker) error {
	marker.Schema = dnsPeerInspectionAutoSchema
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return secureWriteConfig(dnsPeerInspectionAutoMarkerPath, append(raw, '\n'), 0600)
}

// dnsPeerInspectionPair is this Agent's own view of its DNS pair.
type dnsPeerInspectionPair struct {
	engine, localIP, peerIP string
}

func (h dnsPeerInspectionHost) pair(role string) (dnsPeerInspectionPair, string) {
	state, found, err := h.engineState()
	if err != nil || !found {
		return dnsPeerInspectionPair{}, transport.DNSPeerInspectionPairNotReady
	}
	if state.PairRole == "" {
		return dnsPeerInspectionPair{}, transport.DNSPeerInspectionNotApplicable
	}
	if state.PairRole != role {
		return dnsPeerInspectionPair{}, transport.DNSPeerInspectionNotApplicable
	}
	if !canonicalPairReadinessIPv4(state.PairLocalIP) || !canonicalPairReadinessIPv4(state.PairPeerIP) ||
		state.PairLocalIP == state.PairPeerIP || !transport.ValidDNSEngine(state.Engine) {
		return dnsPeerInspectionPair{}, transport.DNSPeerInspectionPairNotReady
	}
	return dnsPeerInspectionPair{engine: string(state.Engine), localIP: state.PairLocalIP, peerIP: state.PairPeerIP}, ""
}

// DNSPeerInspectionPrimaryV1 runs the owner tool's primary-prepare or
// primary-activate for this server's own pair (main DNS server).
func (a *Agent) DNSPeerInspectionPrimaryV1(req *transport.DNSPeerInspectionPrimaryRequest, resp *transport.DNSPeerInspectionPrimaryResponse) error {
	if req == nil || resp == nil {
		return errors.New("peer inspection request is required")
	}
	dnsPeerInspectionMu.Lock()
	defer dnsPeerInspectionMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := dnsPeerInspectionNativeHost().primary(ctx, *req)
	if err != nil {
		return err
	}
	*resp = result
	return nil
}

// DNSPeerInspectionSecondaryV1 runs the owner tool's secondary-install and
// secondary-host-key for this server's own pair (backup DNS server), or only
// checks that it could (action check).
func (a *Agent) DNSPeerInspectionSecondaryV1(req *transport.DNSPeerInspectionSecondaryRequest, resp *transport.DNSPeerInspectionSecondaryResponse) error {
	if req == nil || resp == nil {
		return errors.New("peer inspection request is required")
	}
	dnsPeerInspectionMu.Lock()
	defer dnsPeerInspectionMu.Unlock()
	result, err := dnsPeerInspectionNativeHost().secondary(*req)
	if err != nil {
		return err
	}
	*resp = result
	return nil
}

func validDNSPeerInspectionDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}

func validDNSPeerInspectionCredentialID(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 16 && hex.EncodeToString(raw) == value
}

func (h dnsPeerInspectionHost) primary(ctx context.Context, req transport.DNSPeerInspectionPrimaryRequest) (transport.DNSPeerInspectionPrimaryResponse, error) {
	answer, err := h.primaryStep(ctx, req)
	if err == nil {
		if pair, state := h.pair(transport.DNSPairRolePrimary); state == "" {
			answer.LocalIP, answer.PeerIP = pair.localIP, pair.peerIP
		}
	}
	return answer, err
}

func (h dnsPeerInspectionHost) primaryStep(ctx context.Context, req transport.DNSPeerInspectionPrimaryRequest) (transport.DNSPeerInspectionPrimaryResponse, error) {
	var zero transport.DNSPeerInspectionPrimaryResponse
	if (req.Action != transport.DNSPeerInspectionActionPrepare && req.Action != transport.DNSPeerInspectionActionActivate) ||
		!transport.ValidDNSEngine(transport.DNSEngine(req.Engine)) ||
		(req.CredentialID != "" && !validDNSPeerInspectionCredentialID(req.CredentialID)) ||
		(req.Action == transport.DNSPeerInspectionActionActivate && (!validDNSPeerInspectionCredentialID(req.CredentialID) || !validDNSPeerInspectionDigest(req.HostKeySHA256))) {
		return zero, errors.New("peer inspection request is invalid")
	}
	pair, state := h.pair(transport.DNSPairRolePrimary)
	if state != "" {
		return transport.DNSPeerInspectionPrimaryResponse{State: state}, nil
	}
	catalog, err := binddns.CatalogDomain(pair.localIP)
	if err != nil {
		return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionPairNotReady}, nil
	}
	// Two native engine enrollments are never chosen between: any existing
	// one, or one that cannot be read, is reported and kept.
	var present []dnsPeerInspectionRecord
	for _, engine := range []string{string(transport.DNSEngineBIND), string(transport.DNSEnginePowerDNS)} {
		record, found, err := h.readEnrollment(engine)
		if err != nil {
			log.Printf("peer inspection setup: the %s enrollment on this server cannot be read; it is kept for the owner: %v", engine, err)
			return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionOwnerConflict, Reason: transport.DNSPeerInspectionReasonUnreadable}, nil
		}
		if found {
			present = append(present, record)
		}
	}
	if len(present) > 1 {
		return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionOwnerConflict, Reason: transport.DNSPeerInspectionReasonOtherEngine}, nil
	}
	if len(present) == 1 {
		record := present[0]
		if record.Engine != req.Engine {
			return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionOwnerConflict, Reason: transport.DNSPeerInspectionReasonOtherEngine}, nil
		}
		if record.PrimaryIP != pair.localIP || record.PeerIP != pair.peerIP || record.CatalogName != catalog {
			return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionOwnerConflict, Reason: transport.DNSPeerInspectionReasonOtherPair}, nil
		}
		answer := transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionExisting, CredentialID: record.CredentialID}
		ours := req.CredentialID != "" && record.CredentialID == req.CredentialID
		if ours && (req.Action == transport.DNSPeerInspectionActionPrepare || record.HostKeySHA256 == req.HostKeySHA256) {
			// The same activation again (a lost answer): idempotent.
			answer.State = transport.DNSPeerInspectionActive
		} else if req.Action == transport.DNSPeerInspectionActionActivate && record.HostKeySHA256 != req.HostKeySHA256 {
			answer.State = transport.DNSPeerInspectionOwnerConflict
			answer.Reason = transport.DNSPeerInspectionReasonHostKey
			return answer, nil
		}
		answer.Probe = string(h.probe(ctx, record.Transport))
		return answer, nil
	}
	switch req.Action {
	case transport.DNSPeerInspectionActionPrepare:
		if req.CredentialID != "" {
			if prepared, err := h.preparedPublic(req.Engine, req.CredentialID); err == nil {
				return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionPrepared, CredentialID: prepared.CredentialID, PublicKey: prepared.PublicKey}, nil
			}
			log.Printf("peer inspection setup: the credential prepared earlier is not usable; preparing a new one")
		}
		prepared, err := h.prepare(req.Engine)
		if err != nil {
			log.Printf("peer inspection setup: primary-prepare refused: %v", err)
			return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionFailed, Reason: transport.DNSPeerInspectionReasonNativeStep}, nil
		}
		return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionPrepared, CredentialID: prepared.CredentialID, PublicKey: prepared.PublicKey}, nil
	default:
		if _, err := h.preparedPublic(req.Engine, req.CredentialID); err != nil {
			return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionFailed, Reason: transport.DNSPeerInspectionReasonCredentialStale}, nil
		}
		if err := h.activate(req.Engine, pdnspeerenrollment.OwnerActivation{
			CredentialID: req.CredentialID, PrimaryIP: pair.localIP, PeerIP: pair.peerIP,
			CatalogName: catalog, SSHUsername: dnspeerenrollowner.Account, HostKeySHA256: req.HostKeySHA256,
		}); err != nil {
			log.Printf("peer inspection setup: primary-activate refused: %v", err)
			return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionFailed, Reason: transport.DNSPeerInspectionReasonNativeStep}, nil
		}
		record, found, err := h.readEnrollment(req.Engine)
		if err != nil || !found || record.CredentialID != req.CredentialID {
			log.Printf("peer inspection setup: the new enrollment could not be read back: %v", err)
			return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionFailed, Reason: transport.DNSPeerInspectionReasonNativeStep}, nil
		}
		log.Printf("peer inspection setup: %s enrollment active for peer %s", req.Engine, pair.peerIP)
		return transport.DNSPeerInspectionPrimaryResponse{State: transport.DNSPeerInspectionActive, CredentialID: record.CredentialID,
			Probe: string(h.probe(ctx, record.Transport))}, nil
	}
}

// canonicalDNSPeerInspectionKey accepts exactly one unqualified Ed25519
// public key and returns its canonical line and digest.
func canonicalDNSPeerInspectionKey(raw string) ([]byte, string, bool) {
	if raw == "" || len(raw) > 1024 || strings.ContainsAny(raw, "\r\n") {
		return nil, "", false
	}
	key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil || key.Type() != ssh.KeyAlgoED25519 || len(options) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return nil, "", false
	}
	sum := sha256.Sum256(key.Marshal())
	return ssh.MarshalAuthorizedKey(key), hex.EncodeToString(sum[:]), true
}

func (h dnsPeerInspectionHost) secondary(req transport.DNSPeerInspectionSecondaryRequest) (transport.DNSPeerInspectionSecondaryResponse, error) {
	var zero transport.DNSPeerInspectionSecondaryResponse
	var keyLine []byte
	var keyDigest string
	switch req.Action {
	case transport.DNSPeerInspectionActionCheck:
		if req.PublicKey != "" {
			return zero, errors.New("peer inspection request is invalid")
		}
	case transport.DNSPeerInspectionActionInstall:
		var ok bool
		if keyLine, keyDigest, ok = canonicalDNSPeerInspectionKey(req.PublicKey); !ok {
			return zero, errors.New("peer inspection request is invalid")
		}
	default:
		return zero, errors.New("peer inspection request is invalid")
	}
	pair, state := h.pair(transport.DNSPairRoleSecondary)
	if state != "" {
		return transport.DNSPeerInspectionSecondaryResponse{State: state}, nil
	}
	answer := func(state, reason string) (transport.DNSPeerInspectionSecondaryResponse, error) {
		return transport.DNSPeerInspectionSecondaryResponse{State: state, Engine: pair.engine, Reason: reason,
			PrimaryIP: pair.peerIP, LocalIP: pair.localIP}, nil
	}
	catalog, err := binddns.CatalogDomain(pair.peerIP)
	if err != nil {
		return answer(transport.DNSPeerInspectionPairNotReady, "")
	}
	options := dnspeerenrollowner.SecondaryOptions{PrimaryIP: pair.peerIP, PeerIP: pair.localIP, CatalogName: catalog,
		InspectorBinaryPath: dnsOwnerToolsDir + "/" + pair.engine + "-peer-inspect"}
	if pair.engine == string(transport.DNSEnginePowerDNS) {
		options.CatalogAccount = pdnsPeerCatalogAccount
	}
	if !h.inspectorPresent(pair.engine) {
		return answer(transport.DNSPeerInspectionInspectorUnavailable, "")
	}
	ports, err := h.sshPorts()
	if err != nil {
		log.Printf("peer inspection setup: OpenSSH ports could not be read: %v", err)
		return answer(transport.DNSPeerInspectionFailed, transport.DNSPeerInspectionReasonSSHInactive)
	}
	if !slices.Contains(ports, 22) {
		return transport.DNSPeerInspectionSecondaryResponse{State: transport.DNSPeerInspectionSSHPort, Engine: pair.engine, SSHPorts: ports,
			PrimaryIP: pair.peerIP, LocalIP: pair.localIP}, nil
	}
	if name, err := h.otherEngine(pair.engine); err != nil || name != "" {
		return answer(transport.DNSPeerInspectionOwnerConflict, transport.DNSPeerInspectionReasonOtherEngine)
	}
	marker, markerFound, err := h.readMarker()
	if err != nil {
		log.Printf("peer inspection setup: the automation's own marker cannot be read; nothing is changed: %v", err)
		return answer(transport.DNSPeerInspectionOwnerConflict, transport.DNSPeerInspectionReasonUnreadable)
	}
	ourPair := markerFound && marker.Engine == pair.engine && marker.PrimaryIP == options.PrimaryIP &&
		marker.PeerIP == options.PeerIP && marker.CatalogName == options.CatalogName
	status, err := h.status(pair.engine)
	if err != nil {
		log.Printf("peer inspection setup: secondary-status failed: %v", err)
		return answer(transport.DNSPeerInspectionOwnerConflict, transport.DNSPeerInspectionReasonUnreadable)
	}
	policyAbsent := status.PrimaryIP == "" && status.PeerIP == "" && status.CatalogName == ""
	policyMatches := status.PrimaryIP == options.PrimaryIP && status.PeerIP == options.PeerIP &&
		status.CatalogName == options.CatalogName && status.CatalogAccount == options.CatalogAccount
	withHostKey := func(state string) (transport.DNSPeerInspectionSecondaryResponse, error) {
		digest, err := h.hostKey()
		if err != nil || !validDNSPeerInspectionDigest(digest) {
			log.Printf("peer inspection setup: the OpenSSH Ed25519 host key cannot be read: %v", err)
			return answer(transport.DNSPeerInspectionFailed, transport.DNSPeerInspectionReasonHostKey)
		}
		return transport.DNSPeerInspectionSecondaryResponse{State: state, Engine: pair.engine, HostKeySHA256: digest,
			PrimaryIP: pair.peerIP, LocalIP: pair.localIP}, nil
	}
	resume := false
	switch status.State {
	case "configured":
		if !policyMatches {
			return answer(transport.DNSPeerInspectionOwnerConflict, transport.DNSPeerInspectionReasonOtherPair)
		}
		ours := ourPair && marker.PrimaryKeySHA256 == status.PrimaryKeySHA256
		if req.Action == transport.DNSPeerInspectionActionCheck {
			if ours {
				return withHostKey(transport.DNSPeerInspectionInstalled)
			}
			return withHostKey(transport.DNSPeerInspectionExisting)
		}
		if status.PrimaryKeySHA256 != keyDigest {
			return answer(transport.DNSPeerInspectionOwnerConflict, transport.DNSPeerInspectionReasonOtherKey)
		}
		if ourPair && marker.State != dnsPeerInspectionAutoInstalled {
			marker.State, marker.PrimaryKeySHA256 = dnsPeerInspectionAutoInstalled, keyDigest
			if err := h.writeMarker(marker); err != nil {
				log.Printf("peer inspection setup: marker not updated: %v", err)
			}
		}
		return withHostKey(transport.DNSPeerInspectionInstalled)
	case "disabled":
		switch {
		case policyAbsent && !markerFound:
			// Nothing of this channel exists yet.
		case ourPair && marker.State == dnsPeerInspectionAutoInstalled && policyMatches:
			// This automation finished earlier and the key is gone: the
			// owner revoked it. Never authorize it again.
			return answer(transport.DNSPeerInspectionRevoked, "")
		case ourPair && marker.State == dnsPeerInspectionAutoInstalling && (policyAbsent || policyMatches):
			resume = !policyAbsent
		default:
			return answer(transport.DNSPeerInspectionOwnerConflict, transport.DNSPeerInspectionReasonStaged)
		}
	case "incomplete":
		if !(ourPair && marker.State == dnsPeerInspectionAutoInstalling && policyMatches) {
			return answer(transport.DNSPeerInspectionOwnerConflict, transport.DNSPeerInspectionReasonStaged)
		}
		resume = true
	default:
		reason := transport.DNSPeerInspectionReasonUnreadable
		if strings.Contains(status.Reason, "sshd_config changed") {
			reason = transport.DNSPeerInspectionReasonOwnerSSHConfig
		}
		return answer(transport.DNSPeerInspectionOwnerConflict, reason)
	}
	if req.Action == transport.DNSPeerInspectionActionCheck {
		return answer(transport.DNSPeerInspectionReady, "")
	}
	if err := h.writeMarker(dnsPeerInspectionAutoMarker{Engine: pair.engine, PrimaryIP: options.PrimaryIP, PeerIP: options.PeerIP,
		CatalogName: options.CatalogName, PrimaryKeySHA256: keyDigest, State: dnsPeerInspectionAutoInstalling}); err != nil {
		log.Printf("peer inspection setup: marker could not be written; nothing else was changed: %v", err)
		return answer(transport.DNSPeerInspectionFailed, transport.DNSPeerInspectionReasonNativeStep)
	}
	keyPath, err := h.writePublicKey(keyLine)
	if err != nil {
		log.Printf("peer inspection setup: the main's public key could not be staged: %v", err)
		return answer(transport.DNSPeerInspectionFailed, transport.DNSPeerInspectionReasonNativeStep)
	}
	options.PrimaryPublicKeyPath = keyPath
	if err := h.install(pair.engine, options, resume); err != nil {
		log.Printf("peer inspection setup: secondary-%s refused; staged files are kept for owner review: %v", map[bool]string{false: "install", true: "resume"}[resume], err)
		return answer(transport.DNSPeerInspectionFailed, dnsPeerInspectionInstallReason(err))
	}
	if err := h.writeMarker(dnsPeerInspectionAutoMarker{Engine: pair.engine, PrimaryIP: options.PrimaryIP, PeerIP: options.PeerIP,
		CatalogName: options.CatalogName, PrimaryKeySHA256: keyDigest, State: dnsPeerInspectionAutoInstalled}); err != nil {
		log.Printf("peer inspection setup: marker not updated after install: %v", err)
	}
	log.Printf("peer inspection setup: %s inspector channel installed for primary %s", pair.engine, options.PrimaryIP)
	return withHostKey(transport.DNSPeerInspectionInstalled)
}

// dnsPeerInspectionInstallReason maps a library refusal to one reviewed
// token. The library's own text stays in the Agent log.
func dnsPeerInspectionInstallReason(err error) string {
	text := err.Error()
	switch {
	case strings.Contains(text, "active OpenSSH service") || strings.Contains(text, "OpenSSH server is required") || strings.Contains(text, "SSH reload failed"):
		return transport.DNSPeerInspectionReasonSSHInactive
	case strings.Contains(text, "active native"):
		return transport.DNSPeerInspectionReasonDNSInactive
	case strings.Contains(text, "sshd") || strings.Contains(text, "Include"):
		return transport.DNSPeerInspectionReasonSSHDRejected
	case strings.Contains(text, "changed") || strings.Contains(text, "exists") || strings.Contains(text, "already"):
		return transport.DNSPeerInspectionReasonStaged
	}
	return transport.DNSPeerInspectionReasonNativeStep
}
