package main

import (
	"github.com/alicelik/celikpanel/internal/transport"
)

// Subscription suspension, Agent side (alpha.117). Each call changes one
// native resource through that service's own configuration, so a suspension
// holds without the Panel (D-022), and every change is compare-and-set
// against the before-image the Panel stored first (D-025). Calling a change
// again is a no-op that answers Applied=true, Changed=false.
//
// Abonelik askıya alma, Agent tarafı. Her çağrı tek bir yerel kaynağı o
// hizmetin kendi yapılandırmasıyla değiştirir; değişiklik Panel'in önceden
// kaydettiği ön görüntüye göre karşılaştır-ve-yaz ile yapılır.

// suspensionStep is the compare-and-set decision for one resource.
type suspensionStep int

const (
	// suspensionAlready: the resource is already in the wanted state.
	suspensionAlready suspensionStep = iota
	// suspensionChange: the resource is in the state the change starts from.
	suspensionChange
	// suspensionConflict: someone else changed it; nothing may be written.
	suspensionConflict
)

// suspensionDecide compares the current state with the wanted state and with
// the state the change starts from (the before-image when suspending, the
// suspended state when restoring).
func suspensionDecide[T comparable](current, target, from T) suspensionStep {
	if current == target {
		return suspensionAlready
	}
	if current == from {
		return suspensionChange
	}
	return suspensionConflict
}

// SuspensionInspectV1 reports the current native state of the named
// resources, in request order. It is read-only; an item that could not be
// read carries an Error and its state is unknown, not absent.
//
// SuspensionInspectV1 salt-okurdur; okunamayan kaynak "yok" değil
// "bilinmiyor" olarak Error ile döner.
func (a *Agent) SuspensionInspectV1(
	req *transport.SuspensionInspectRequest,
	resp *transport.SuspensionInspectResponse,
) error {
	if resp == nil {
		return nil
	}
	resp.SiteAccounts = []transport.SiteAccountObservation{}
	resp.MailDomains = []transport.MailDomainObservation{}
	resp.DatabaseUsers = []transport.DatabaseUserObservation{}
	if req == nil {
		resp.Error = "suspension inspection request is required"
		return nil
	}

	for _, account := range req.SiteAccounts {
		observation := transport.SiteAccountObservation{Account: account}
		state, err := inspectSuspensionSiteAccount(account)
		if err != nil {
			observation.Error = err.Error()
		} else {
			observation.State = state
		}
		resp.SiteAccounts = append(resp.SiteAccounts, observation)
	}

	resp.MailDomains = inspectSuspensionMailDomains(req.MailDomains)

	for _, user := range req.DatabaseUsers {
		observation := transport.DatabaseUserObservation{User: user}
		state, err := inspectSuspensionDatabaseUser(user)
		if err != nil {
			observation.Error = err.Error()
		} else {
			observation.State = state
		}
		resp.DatabaseUsers = append(resp.DatabaseUsers, observation)
	}
	return nil
}
