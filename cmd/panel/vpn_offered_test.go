package main

import (
	"context"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// vpnNotInstalledAgent answers VPN status as "WireGuard is not installed".
type vpnNotInstalledAgent struct {
	*serviceOperationTestAgent
}

func (a *vpnNotInstalledAgent) VPNStatus(_ *transport.Empty, out *transport.VPNStatusResponse) error {
	*out = transport.VPNStatusResponse{}
	return nil
}

// The /auth/me "vpn" feature: a non-admin is offered the VPN page only when
// WireGuard is installed and they reach a subscription with the VPN add-on.
func TestVPNOfferedToNonAdminNeedsInstalledWireGuardAndAddOn(t *testing.T) {
	fixture, ownerID, otherID, subscriptionID := newVPNSecurityFixture(t)
	owner := &Caller{ID: ownerID, Role: roleCustomer, CustomerID: ownerID}
	other := &Caller{ID: otherID, Role: roleCustomer, CustomerID: otherID}
	admin := &Caller{ID: 1, Role: roleAdmin}
	ctx := context.Background()

	attachRequestIdentityAgent(t, fixture.panel, &requestIdentityVPNAgent{serviceOperationTestAgent: fixture.agent})
	if !fixture.panel.vpnOfferedTo(ctx, owner) {
		t.Fatal("customer with the add-on and WireGuard installed must be offered VPN")
	}
	if fixture.panel.vpnOfferedTo(ctx, other) {
		t.Fatal("customer without any VPN subscription was offered VPN")
	}
	if !fixture.panel.vpnOfferedTo(ctx, admin) {
		t.Fatal("administrator must always be offered VPN")
	}

	// The feature reaches the identity response.
	identity := canonicalAuthIdentity{
		user:        &core.User{ID: ownerID, Username: "vpn-owner"},
		identity:    core.EffectiveIdentity{UserID: ownerID, Role: roleCustomer, CustomerID: ownerID},
		accountType: core.AccountTypeAccount,
	}
	if got, ok := fixture.panel.identityResponse(ctx, identity, false).Features["vpn"]; !ok || !got {
		t.Fatalf("features.vpn = %v (present %v), want true", got, ok)
	}

	// Add-on removed: not offered even though WireGuard is installed.
	if _, err := fixture.database.GetDB().Exec(
		`DELETE FROM subscription_entitlements WHERE subscription_id = ?`, subscriptionID,
	); err != nil {
		t.Fatal(err)
	}
	if fixture.panel.vpnOfferedTo(ctx, owner) {
		t.Fatal("customer without the VPN add-on was offered VPN")
	}
	if _, err := fixture.database.GetDB().Exec(`
		INSERT INTO subscription_entitlements (subscription_id, product_id, status)
		VALUES (?, 'vpn', 'active')`, subscriptionID); err != nil {
		t.Fatal(err)
	}

	// WireGuard not installed: not offered to a customer, still to the admin.
	attachRequestIdentityAgent(t, fixture.panel, &vpnNotInstalledAgent{serviceOperationTestAgent: fixture.agent})
	if fixture.panel.vpnOfferedTo(ctx, owner) {
		t.Fatal("customer was offered VPN while WireGuard is not installed")
	}
	if !fixture.panel.vpnOfferedTo(ctx, admin) {
		t.Fatal("administrator must see VPN even when WireGuard is not installed")
	}
}
