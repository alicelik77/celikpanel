//go:build !linux

package main

import "github.com/alicelik/celikpanel/internal/transport"

// The native peer inspection channel is Linux-only (dns-peer-enroll). These
// answer that this server is not a side the automation applies to; nothing
// is written.
func (a *Agent) DNSPeerInspectionPrimaryV1(_ *transport.DNSPeerInspectionPrimaryRequest, resp *transport.DNSPeerInspectionPrimaryResponse) error {
	resp.State = transport.DNSPeerInspectionNotApplicable
	return nil
}

func (a *Agent) DNSPeerInspectionSecondaryV1(_ *transport.DNSPeerInspectionSecondaryRequest, resp *transport.DNSPeerInspectionSecondaryResponse) error {
	resp.State = transport.DNSPeerInspectionNotApplicable
	return nil
}
