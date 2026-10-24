package main

import (
	"regexp"
	"testing"
)

func TestExactInstallUnitForSelectedPostgreSQLMajor(t *testing.T) {
	unit, exact := exactInstallUnit("postgresql", "apt", "postgresql-17")
	if !exact || unit != "postgresql@17-main" {
		t.Fatalf("got (%q, %v), want exact postgresql@17-main", unit, exact)
	}

	unit, exact = exactInstallUnit("postgresql", "apt", "postgresql-17-client")
	if !exact || unit != "" {
		t.Fatalf("invalid selected package got (%q, %v), want exact rejection", unit, exact)
	}

	unit, exact = exactInstallUnit("php-fpm", "apt", "php8.3-fpm")
	if !exact || unit != "php8.3-fpm" {
		t.Fatalf("selected PHP package got (%q, %v), want exact php8.3-fpm", unit, exact)
	}

	unit, exact = exactInstallUnit("php-fpm", "pacman", "php8.3-fpm")
	if exact || unit != "" {
		t.Fatalf("non-apt package got (%q, %v), want normal fallback path", unit, exact)
	}
}

func TestTemplateNamesAreNeverUnitsAndInstancesAreFound(t *testing.T) {
	files := "postgresql.service enabled enabled\npostgresql@.service indirect enabled\nnginx.service enabled enabled\n"
	loaded := "postgresql@17-main.service loaded active running PostgreSQL Cluster 17-main\npostgresql.service loaded active exited PostgreSQL RDBMS\n"
	got := unitNamesMatching(regexp.MustCompile(`^postgresql@`), files, loaded)
	if len(got) != 1 || got[0] != "postgresql@17-main" {
		t.Fatalf("units = %v", got)
	}
	if !templateInstanceUnit("postgresql@17-main") || templateInstanceUnit("postgresql@") || templateInstanceUnit("nginx") {
		t.Fatal("template instance detection")
	}
}
