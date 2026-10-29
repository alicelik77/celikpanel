package main

import (
	"errors"
	"testing"
)

// Beta-1 VM cell a: on a fresh Debian 13, installing MariaDB brought point
// updates of installed base packages (perl-base, libpcre2-8-0) and the setup
// step refused it. A point update needs all three: the same epoch and
// upstream, a newer version, and the distribution's own archive. Anything
// else keeps the owner's confirmation.
func TestPointUpdatesOfInstalledPackagesAreToldApart(t *testing.T) {
	for from, to := range map[string]string{"5.40.1-6": "5.40.1-6+deb13u1", "10.46-1~deb13u1": "10.46-1~deb13u3", "1:2.3-1ubuntu1": "1:2.3-1ubuntu1.1"} {
		if !aptPointUpdate(from, to) {
			t.Errorf("%s -> %s is a point update", from, to)
		}
	}
	for from, to := range map[string]string{"5.40.1-6": "5.42.0-1", "1:2.3-1": "2:2.3-1", "2.3": "2.3+deb13u1", "": "1.0-1", "1.0-1": "1.0-1", "5.40.1-6+deb13u1": "5.40.1-6"} {
		if aptPointUpdate(from, to) {
			t.Errorf("%s -> %s is not a point update", from, to)
		}
	}
	security := aptInstOrigins("Inst perl-base [5.40.1-6] (5.40.1-6+deb13u1 Debian:13.1/stable, Debian-Security:13/stable-security [amd64])", "5.40.1-6+deb13u1")
	if !aptOfficialStableArchive(security) || len(security) != 2 || security[1].Origin != "Debian-Security" || security[1].Suite != "stable-security" {
		t.Fatalf("official archives: %+v", security)
	}
	if !aptOfficialStableArchive(aptInstOrigins("Inst libssl3t64 [3.0.13-0ubuntu3.4] (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-updates [amd64])", "3.0.13-0ubuntu3.5")) {
		t.Fatal("an Ubuntu updates pocket is the distribution's own")
	}
	for name, line := range map[string]string{
		"sury":       "Inst php-common [2:93] (2:93+0~20250101.1+debian13~1.gbp1 deb.sury.org:13/trixie [all])",
		"ppa":        "Inst php-common [2:93] (2:93ubuntu1 LP-PPA-ondrej-php:24.04/noble [all])",
		"backports":  "Inst pdns-server [4.9.4-1] (4.9.4-1~bpo13+1 Debian Backports:13-backports/trixie-backports [amd64])",
		"mixed":      "Inst perl-base [5.40.1-6] (5.40.1-6+deb13u1 Debian:13.1/stable, mirror.example:13/stable [amd64])",
		"unreadable": "Inst perl-base [5.40.1-6] (5.40.1-6+deb13u1 [amd64])",
	} {
		sim := parseAptSimulation([]byte(line + "\n"))
		for _, upgrade := range sim.Upgrades {
			if aptOfficialStableArchive(upgrade.Origins) {
				t.Errorf("%s read as the distribution's own archive: %+v", name, upgrade.Origins)
			}
		}
	}
	official := []aptOrigin{{Origin: "Debian-Security", Suite: "stable-security"}}
	c := newInstallClassification("apt", []string{"mariadb-server"}, []string{"mariadb"}, nil, nil)
	sim := aptSimulation{
		Installed: []string{"mariadb-server", "perl", "perl-base", "libpcre2-8-0"},
		Upgrades: map[string]aptUpgrade{
			"perl-base":    {From: "5.40.1-6", To: "5.40.1-6+deb13u1", Origins: official},
			"libpcre2-8-0": {From: "10.46-1~deb13u1", To: "10.46-1~deb13u3", Origins: official},
		},
	}
	for _, item := range c.classifySimulation(sim) {
		if item.Reason != notableReasonUpgrade || !item.PointUpdate {
			t.Fatalf("not a listed point update: %+v", item)
		}
	}
	// The same versions from a PPA, a downgrade, or an unreadable index are
	// not point updates.
	for name, upgrade := range map[string]aptUpgrade{
		"ppa":        {From: "5.40.1-6", To: "5.40.1-6+deb13u1", Origins: []aptOrigin{{Origin: "LP-PPA-someone", Suite: "noble"}}},
		"downgrade":  {From: "5.40.1-6+deb13u1", To: "5.40.1-6", Origins: official},
		"unreadable": {From: "5.40.1-6", To: "5.40.1-6+deb13u1"},
	} {
		one := aptSimulation{Installed: []string{"mariadb-server", "perl-base"}, Upgrades: map[string]aptUpgrade{"perl-base": upgrade}}
		for _, item := range c.classifySimulation(one) {
			if item.PointUpdate {
				t.Fatalf("%s marked as a point update: %+v", name, item)
			}
		}
	}
	var refusal *packageInstallNotableRefusal
	if err := notableInstallGuard(c, []string{"mariadb-server"}, nil)(sim); !errors.As(err, &refusal) {
		t.Fatalf("an unconfirmed point update outside setup is still refused: %v", err)
	}
	// Pacman has no distribution revision: never a point update.
	pacman := newInstallClassification("pacman", []string{"mariadb"}, []string{"mariadb"}, nil, nil)
	for _, item := range pacman.classifySimulation(aptSimulation{Upgrades: map[string]aptUpgrade{"perl": {From: "5.40.1-6", To: "5.40.1-7", Origins: official}}}) {
		if item.PointUpdate {
			t.Fatalf("pacman upgrade marked as a point update: %+v", item)
		}
	}
}
