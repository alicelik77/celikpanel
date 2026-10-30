package main

import (
	"context"
	"testing"
	"time"
)

// alpha.113 (the owner's decision): a site added with SSL ticked redirects
// http to https once its first certificate is active. The activation turns
// force_https on in its own transaction; undoing the activation puts the
// previous setting back; an activation without the request leaves it alone.

func siteForceHTTPS(t *testing.T, p *Panel, domainID int) bool {
	t.Helper()
	var force bool
	if err := p.db.GetDB().QueryRow(`SELECT COALESCE(force_https, false) FROM sites WHERE domain_id = ?`, domainID).Scan(&force); err != nil {
		t.Fatal(err)
	}
	return force
}

func redirectTestInstall(redirect bool) certificateInstall {
	return certificateInstall{
		Type: "letsencrypt", CertPath: "/certs/new/fullchain.pem", KeyPath: "/certs/new/privkey.pem",
		ChainPath: "/certs/new/chain.pem", Issuer: "Let's Encrypt", Subject: "ssl-state.example",
		IssuedAt:  time.Date(2026, time.October, 30, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2027, time.January, 28, 0, 0, 0, 0, time.UTC),
		AutoRenew: true, RedirectHTTP: redirect,
	}
}

func TestFirstCertificateActivationTurnsHTTPSRedirectOnWhenAsked(t *testing.T) {
	p, domainID := newDomainAliasFixture(t)
	ctx := context.Background()
	if siteForceHTTPS(t, p, domainID) {
		t.Fatal("fixture starts with the redirect on")
	}

	activation, err := p.activateCertificate(ctx, domainID, redirectTestInstall(true))
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if !siteForceHTTPS(t, p, domainID) {
		t.Fatal("the redirect is off after an activation that asked for it")
	}
	if err := p.rollbackCertificateActivation(ctx, activation); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if siteForceHTTPS(t, p, domainID) {
		t.Fatal("undoing the activation left the redirect on")
	}

	if _, err := p.activateCertificate(ctx, domainID, redirectTestInstall(false)); err != nil {
		t.Fatalf("activate without the request: %v", err)
	}
	if siteForceHTTPS(t, p, domainID) {
		t.Fatal("an activation that did not ask turned the redirect on")
	}
}
