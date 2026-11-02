package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/transport"
)

var errInvalidCanonicalAuthIdentity = errors.New("invalid canonical authentication identity")
var errCanonicalAuthUnavailable = errors.New("canonical authentication identity unavailable")

type canonicalAuthIdentity struct {
	user        *core.User
	identity    core.EffectiveIdentity
	accountType core.AccountType
}

// authIdentityResponse is the single authentication identity contract shared
// by password login, TOTP login and /auth/me. Role remains as a compatibility
// alias, but always contains the canonical effective role.
type authIdentityResponse struct {
	Username      string           `json:"username"`
	Role          string           `json:"role"`
	EffectiveRole string           `json:"effective_role"`
	AccountType   core.AccountType `json:"account_type"`
	Email         string           `json:"email"`
	Impersonating bool             `json:"impersonating"`
	// Impersonator: the operator's username during "Log in as" (alpha.114).
	Impersonator string          `json:"impersonator,omitempty"`
	Features     map[string]bool `json:"features"`
	// Suspended: the account sees only the suspended screen (alpha.117).
	Suspended *meSuspension `json:"suspended,omitempty"`
}

func normalizedStoredAccountType(user *core.User) core.AccountType {
	if user == nil {
		return ""
	}
	if user.AccountType == "" {
		return core.AccountTypeAccount
	}
	return user.AccountType
}

// canonicalAuthIdentity reloads the complete account relationship before an
// authentication response is issued. This prevents stored customer roles on
// additional users, stale parents and malformed legacy rows from becoming an
// authorization identity.
func (p *Panel) canonicalAuthIdentity(ctx context.Context, userID int) (canonicalAuthIdentity, error) {
	if p.users == nil {
		return canonicalAuthIdentity{}, errCanonicalAuthUnavailable
	}
	user, err := p.users.GetByID(ctx, userID)
	if errors.Is(err, repositories.ErrUserNotFound) {
		return canonicalAuthIdentity{}, errInvalidCanonicalAuthIdentity
	}
	if err != nil || user == nil {
		return canonicalAuthIdentity{}, fmt.Errorf("%w: %v", errCanonicalAuthUnavailable, err)
	}
	if user.ID != userID || user.Status != "active" {
		return canonicalAuthIdentity{}, errInvalidCanonicalAuthIdentity
	}

	identity, ok := user.EffectiveIdentity()
	accountType := normalizedStoredAccountType(user)
	caller := Caller{
		ID:          identity.UserID,
		Role:        identity.Role,
		AccountType: accountType,
		CustomerID:  identity.CustomerID,
	}
	if !ok || identity.UserID != user.ID || !caller.validAuthorizationIdentity() {
		return canonicalAuthIdentity{}, errInvalidCanonicalAuthIdentity
	}

	if identity.Role == core.EffectiveRoleAdditionalUser {
		parent, err := p.users.GetByID(ctx, identity.CustomerID)
		if errors.Is(err, repositories.ErrUserNotFound) {
			return canonicalAuthIdentity{}, errInvalidCanonicalAuthIdentity
		}
		if err != nil || parent == nil {
			return canonicalAuthIdentity{}, fmt.Errorf("%w: %v", errCanonicalAuthUnavailable, err)
		}
		if parent.ID != identity.CustomerID || parent.Status != "active" {
			return canonicalAuthIdentity{}, errInvalidCanonicalAuthIdentity
		}
		parentIdentity, ok := parent.EffectiveIdentity()
		if !ok || normalizedStoredAccountType(parent) != core.AccountTypeAccount ||
			parentIdentity.UserID != parent.ID || parentIdentity.Role != roleCustomer ||
			parentIdentity.CustomerID != parent.ID {
			return canonicalAuthIdentity{}, errInvalidCanonicalAuthIdentity
		}
	}

	return canonicalAuthIdentity{
		user:        user,
		identity:    identity,
		accountType: accountType,
	}, nil
}

func (identity canonicalAuthIdentity) response(impersonating bool) authIdentityResponse {
	return authIdentityResponse{
		Username:      identity.user.Username,
		Role:          identity.identity.Role,
		EffectiveRole: identity.identity.Role,
		AccountType:   identity.accountType,
		Email:         identity.user.Email,
		Impersonating: impersonating,
		Features: map[string]bool{
			"team_members": identity.accountType == core.AccountTypeAccount && identity.identity.Role == roleCustomer,
		},
	}
}

// identityResponse is response() plus the features that need the panel's own
// data. The navigation reads them so it does not draw an entry that leads to
// a page the person can do nothing on.
// identityResponse, response() artı panelin kendi verisini isteyen özellikler.
// Gezinme bunları okur; kişinin hiçbir şey yapamayacağı sayfaya giden girdi
// çizilmez.
func (p *Panel) identityResponse(
	ctx context.Context, identity canonicalAuthIdentity, impersonating bool,
) authIdentityResponse {
	response := identity.response(impersonating)
	caller := &Caller{
		ID:          identity.identity.UserID,
		Role:        identity.identity.Role,
		AccountType: identity.accountType,
		CustomerID:  identity.identity.CustomerID,
	}
	response.Features["vpn"] = p.vpnOfferedTo(ctx, caller)
	return response
}

// vpnOfferedTo reports whether the VPN page has anything for this person. An
// administrator always manages it. Anyone else needs WireGuard installed on
// this server and a subscription they can reach with an active VPN add-on.
// Any doubt (no add-on, agent not answering) is "not offered".
// vpnOfferedTo, VPN sayfasında bu kişi için bir şey olup olmadığını söyler.
// Yönetici her zaman yönetir. Diğerleri için bu sunucuda WireGuard kurulu ve
// erişebildikleri bir abonelikte etkin VPN eklentisi gerekir. Şüphede sonuç
// "sunulmuyor"dur.
func (p *Panel) vpnOfferedTo(ctx context.Context, caller *Caller) bool {
	if caller.hasAccountRole(roleAdmin) {
		return true
	}
	if p.db == nil {
		return false
	}
	ids, all, err := p.visibleOwnerIDs(ctx, caller)
	if err != nil || all || len(ids) == 0 {
		return false
	}
	owners := make([]any, 0, len(ids))
	for id := range ids {
		owners = append(owners, id)
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT id FROM subscriptions WHERE owner_id IN (`+
		strings.TrimSuffix(strings.Repeat("?,", len(owners)), ",")+`)`, owners...)
	if err != nil {
		return false
	}
	var subscriptions []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return false
		}
		subscriptions = append(subscriptions, id)
	}
	failed := rows.Err() != nil
	rows.Close()
	if failed {
		return false
	}
	entitled := false
	for _, id := range subscriptions {
		if p.hasEntitlement(ctx, id, vpnProductID) {
			entitled = true
			break
		}
	}
	if !entitled {
		return false
	}
	var status transport.VPNStatusResponse
	if err := p.callAgentContext(ctx, "Agent.VPNStatus", &transport.Empty{}, &status); err != nil {
		return false
	}
	return status.Installed
}
