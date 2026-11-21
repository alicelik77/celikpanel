package services

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// S4 R1 fix round (21 Nov 2026): a PHP-FPM version is never left without a
// pool. With the distribution's stock pool turned off by CelikPanel (the side
// doors step), deleting the last site of a version, or moving it to another
// version, used to leave PHP-FPM with no pool: its configuration test refused
// and the removal was refused with it. Now the stock pool is put back first.
// These fail on 123ddd738 (the pool removal is refused there).

const stockConf = "[www]\nuser = www-data\nlisten = /run/php/php8.3-fpm.sock\n"

func turnedOffStock(t *testing.T, version string) string {
	t.Helper()
	restore := SetStockPoolsOffRecordPathForTest(filepath.Join(t.TempDir(), "stock-pools-off.json"))
	t.Cleanup(restore)
	stock := filepath.Join(poolDirPath(version), "www.conf")
	if err := os.WriteFile(stock+StockPoolOffSuffix, []byte(stockConf), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetStockPoolTurnedOff(version, true); err != nil {
		t.Fatal(err)
	}
	return stock
}

// noPoolTest refuses a configuration with no pool file, as PHP-FPM does.
func noPoolTest(version string) error {
	entries, _ := os.ReadDir(poolDirPath(version))
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".conf" {
			return nil
		}
	}
	return errors.New("ERROR: No pool defined. at least one pool section must be specified in config file")
}

func TestDeletingAVersionsLastSitePutsTheStockPoolBack(t *testing.T) {
	path := withPoolTree(t, "8.3", "site42", seededPool)
	stock := turnedOffStock(t, "8.3")
	phpFPMConfigTest = noPoolTest
	if err := NewPHPPoolManager().DeletePoolByName("8.3", "site42"); err != nil {
		t.Fatalf("delete the last site's pool: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the site's pool stayed")
	}
	if data, err := os.ReadFile(stock); err != nil || string(data) != stockConf {
		t.Fatalf("PHP 8.3 has no pool: %v", err)
	}
	if off, _ := StockPoolsTurnedOff(); off["8.3"] {
		t.Fatal("put back by CelikPanel, but still recorded as turned off")
	}
}

func TestMovingAVersionsLastSiteAwayPutsTheStockPoolBack(t *testing.T) {
	oldPath := withPoolTree(t, "8.3", "site42", seededPool)
	if err := os.MkdirAll(poolDirPath("8.4"), 0o755); err != nil {
		t.Fatal(err)
	}
	stock := turnedOffStock(t, "8.3")
	phpFPMConfigTest = noPoolTest
	if err := NewPHPPoolManager().MigratePool("8.3", "8.4", "site42"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the old pool stayed")
	}
	if _, err := os.Stat(stock); err != nil {
		t.Fatalf("PHP 8.3 was left with no pool: %v", err)
	}
}

func TestARefusedRemovalTurnsTheStockPoolOffAgain(t *testing.T) {
	path := withPoolTree(t, "8.3", "site42", seededPool)
	stock := turnedOffStock(t, "8.3")
	phpFPMConfigTest = func(string) error { return errors.New("ERROR: something else") }
	if err := NewPHPPoolManager().DeletePoolByName("8.3", "site42"); err == nil {
		t.Fatal("a refused removal was accepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the site's pool did not come back: %v", err)
	}
	if _, err := os.Stat(stock); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the stock pool stayed on after the refusal")
	}
	if off, _ := StockPoolsTurnedOff(); !off["8.3"] {
		t.Fatal("the record of the stock pool did not come back")
	}
}
