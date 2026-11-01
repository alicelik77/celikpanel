package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

var errDNSEngineWorkflowRequired = errors.New("DNS engine changes require the reviewed DNS infrastructure workflow")

// How long a mutation waits for a paired primary's backup to confirm the
// current catalog before it is refused with DNS_PEER_CATCHING_UP. Variables
// only so tests can shorten them.
var (
	dnsPeerCatchUpAttempts = 5
	dnsPeerCatchUpInterval = 3 * time.Second
)

func managedDNSEngineService(service *core.ManagedService) bool {
	return service != nil && service.ConflictGroup == "dns-server"
}

func managedDNSEngineServiceID(serviceID string) bool {
	return managedDNSEngineService(core.GetManagedServiceByID(serviceID))
}

func writeDNSEngineWorkflowRequired(w http.ResponseWriter) {
	writeCodedError(
		w,
		http.StatusConflict,
		errCodeDNSEngineWorkflowRequired,
		"BIND and PowerDNS must be installed, activated, switched, or removed from Server → DNS and peer servers",
		"/dns-infrastructure",
	)
}

// requireDNSPublisherOrHeldChange gates a zone's record and zone changes.
// While an exact publication is pending on a paired main, its backup usually
// has not caught up, so the pair is not ready: the change is then saved and
// published as soon as the earlier publication completes (one bounded attempt
// to complete it is made when this change is published), instead of being
// refused with "nothing was changed" (2026-10-30). Without a pending
// publication the gate is requireActiveDNSPublisherForMutation unchanged.
// Bir yayın beklerken eşli birincilde değişiklik kaydedilir, sonra yayımlanır.
func (p *Panel) requireDNSPublisherOrHeldChange(
	w http.ResponseWriter,
	ctx context.Context,
) bool {
	identity, ready, err := p.activeDNSPublisher(ctx)
	if err == nil && ready {
		return true
	}
	if err == nil && identity.PairRole == transport.DNSPairRolePrimary {
		if leases, listErr := listDNSZoneEngineLeases(ctx, p.db.GetDB()); listErr == nil && len(leases) > 0 {
			return true
		}
	}
	_, ok := p.requireActiveDNSPublisherForMutation(w, ctx)
	return ok
}

func (p *Panel) requireActiveDNSPublisherForMutation(
	w http.ResponseWriter,
	ctx context.Context,
) (dnsPublisherIdentity, bool) {
	identity, ready, err := p.activeDNSPublisher(ctx)
	// A paired primary whose backup has not yet confirmed the current
	// catalog is a running DNS server, not "no DNS engine": the backup
	// usually catches up within seconds of the previous change. Ask again
	// (bounded, read only) before refusing, and then say what it is
	// (Arch PowerDNS follow-up drill, 2026-10-30: a record deletion eight
	// seconds after an addition was refused as "no DNS engine is active").
	// Eşli birincilin yedeği son değişikliği henüz doğrulamadıysa bu "DNS
	// motoru yok" değildir: kısa süre yeniden sorulur, sonra nedeni söylenir.
	for attempt := 0; err == nil && !ready && identity.PairRole == transport.DNSPairRolePrimary &&
		attempt < dnsPeerCatchUpAttempts; attempt++ {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-time.After(dnsPeerCatchUpInterval):
			identity, ready, err = p.activeDNSPublisher(ctx)
		}
	}
	if err != nil {
		writeServerError(w, err)
		return dnsPublisherIdentity{}, false
	}
	if !ready && identity.PairRole == transport.DNSPairRolePrimary {
		writeCodedError(
			w,
			http.StatusConflict,
			errCodeDNSPeerCatchingUp,
			"Nothing was changed: the backup DNS server has not yet confirmed this server's current zone list, so CelikPanel does not publish another change yet. It usually catches up within a minute of the previous change; try again then. If this keeps happening, the server administrator checks that the backup DNS server is running and reachable from this server on TCP port 53 (Server → DNS and peer servers shows the pair state).",
			"/dns-infrastructure",
		)
		return dnsPublisherIdentity{}, false
	}
	if !ready {
		// Say why (a backup server, a switch, port 53…); the refusal is the same.
		status, statusErr := p.dnsPublisherStatus(ctx)
		if statusErr != nil {
			status = dnsPublisherStatusView{}
		}
		writeDNSServerRequired(w, status.State, string(status.Engine), status.Peer,
			"no single managed authoritative DNS engine is active and verified")
		return dnsPublisherIdentity{}, false
	}
	return identity, true
}
