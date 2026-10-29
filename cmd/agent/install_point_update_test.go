package main

import (
	"errors"
	"testing"
)

// Beta-1 VM cell a: on a fresh Debian 13, installing MariaDB brought point
// updates of installed base packages (perl-base, libpcre2-8-0) and the setup
// step refused it. Those are told apart as point updates; a reviewed setup
// step that accepts them is not refused, everything else still is.
func TestPointUpdatesOfInstalledPackagesAreToldApart(t *testing.T) {
	for from, to := range map[string]string{"5.40.1-6": "5.40.1-6+deb13u1", "10.46-1~deb13u1": "10.46-1~deb13u3", "1:2.3-1ubuntu1": "1:2.3-1ubuntu1.1"} {
		if !aptPointUpdate(from, to) {
			t.Errorf("%s -> %s is a point update", from, to)
		}
	}
	for from, to := range map[string]string{"5.40.1-6": "5.42.0-1", "1:2.3-1": "2:2.3-1", "2.3": "2.3+deb13u1", "": "1.0-1", "1.0-1": "1.0-1"} {
		if aptPointUpdate(from, to) {
			t.Errorf("%s -> %s is not a point update", from, to)
		}
	}
	c := newInstallClassification("apt", []string{"mariadb-server"}, []string{"mariadb"}, nil, nil)
	sim := aptSimulation{
		Installed: []string{"mariadb-server", "perl", "perl-base", "libpcre2-8-0"},
		Upgrades: map[string]aptUpgrade{
			"perl-base":    {From: "5.40.1-6", To: "5.40.1-6+deb13u1"},
			"libpcre2-8-0": {From: "10.46-1~deb13u1", To: "10.46-1~deb13u3"},
		},
	}
	for _, item := range c.classifySimulation(sim) {
		if item.Reason != notableReasonUpgrade || !item.PointUpdate {
			t.Fatalf("not a listed point update: %+v", item)
		}
	}
	var refusal *packageInstallNotableRefusal
	if err := notableInstallGuard(c, []string{"mariadb-server"}, nil)(sim); !errors.As(err, &refusal) {
		t.Fatalf("an unconfirmed point update outside setup is still refused: %v", err)
	}
	if err := notableInstallGuardAccepting(c, []string{"mariadb-server"}, nil, true)(sim); err != nil {
		t.Fatalf("the setup's confirmation covers point updates: %v", err)
	}
	// A new upstream version and a database server stay refused.
	sim.Upgrades["libssl3"] = aptUpgrade{From: "3.5.1-1", To: "3.6.0-1"}
	if err := notableInstallGuardAccepting(c, []string{"mariadb-server"}, nil, true)(sim); !errors.As(err, &refusal) || len(refusal.Notable) != 1 || refusal.Notable[0].Package != "libssl3" {
		t.Fatalf("a major change is accepted: %v", err)
	}
	delete(sim.Upgrades, "libssl3")
	other := newInstallClassification("apt", []string{"phpmyadmin"}, []string{"phpmyadmin"}, nil, nil)
	sim.Installed = append(sim.Installed, "phpmyadmin")
	if err := notableInstallGuardAccepting(other, []string{"phpmyadmin"}, nil, true)(sim); !errors.As(err, &refusal) {
		t.Fatalf("a database server brought along is accepted: %v", err)
	}
	// Not on a family without a distribution revision.
	pacman := newInstallClassification("pacman", []string{"mariadb"}, []string{"mariadb"}, nil, nil)
	for _, item := range pacman.classifySimulation(aptSimulation{Upgrades: map[string]aptUpgrade{"perl": {From: "5.40.1-6", To: "5.40.1-7"}}}) {
		if item.PointUpdate {
			t.Fatalf("pacman upgrade marked as a point update: %+v", item)
		}
	}
}
