package core

import "testing"

func TestDistributionShipsSelectedPackage(t *testing.T) {
	php := GetManagedServiceByID("php-fpm")
	if php == nil {
		t.Fatal("no php-fpm in the catalogue")
	}
	cases := []struct {
		distro, version, pkg string
		want                 bool
	}{
		{"debian", "13", "php8.4-fpm", true},
		{"debian", "12", "php8.2-fpm", true},
		{"ubuntu", "24.04", "php8.3-fpm", true},
		{"ubuntu", "22.04", "php8.1-fpm", true},
		{"debian", "13", "php8.3-fpm", false}, // Sury's
		{"debian", "13", "php8.4-cli", false},
		{"debian", "13", "", false},
		{"ubuntu", "26.04", "php8.4-fpm", false}, // unknown release: the repository rule stays
		{"arch", "", "php8.4-fpm", false},
	}
	for _, c := range cases {
		if got := DistributionShipsSelectedPackage(php, c.distro, c.version, c.pkg); got != c.want {
			t.Errorf("%s %s %q = %v, want %v", c.distro, c.version, c.pkg, got, c.want)
		}
	}
	if DistributionShipsSelectedPackage(GetManagedServiceByID("postgresql"), "debian", "13", "php8.4-fpm") {
		t.Error("another component used PHP's rule")
	}
	if DistributionPHPLine("Debian", "13") != "8.4" {
		t.Error("the distribution ID is not case-insensitive")
	}
}
