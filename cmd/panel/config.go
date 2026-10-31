package main

import (
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/alicelik/celikpanel/internal/panellisten"
)

// Runtime paths and address. Defaults keep `./bin/panel` working straight
// from the repo for development; a system install (see install.sh) overrides
// them via environment so nothing is hard-coded to one layout.
//
// Çalışma yolları ve adres. Varsayılanlar `./bin/panel`in depodan doğrudan
// çalışmasını sağlar; sistem kurulumu (bkz. install.sh) bunları ortamla
// geçersiz kılar; böylece hiçbir şey tek bir düzene gömülü kalmaz.

// dataDir is where the SQLite database lives (CELIKPANEL_DATA_DIR).
// dataDir, SQLite veritabanının yaşadığı yerdir (CELIKPANEL_DATA_DIR).
func dataDir() string {
	if d := os.Getenv("CELIKPANEL_DATA_DIR"); d != "" {
		return d
	}
	return "./data"
}

func databaseFile() string {
	return filepath.Join(dataDir(), "celikpanel.db")
}

// webDir is the built frontend to serve (CELIKPANEL_WEB_DIR).
// webDir, sunulacak derlenmiş ön yüzdür (CELIKPANEL_WEB_DIR).
func webDir() string {
	if d := os.Getenv("CELIKPANEL_WEB_DIR"); d != "" {
		return d
	}
	return "./web/dist"
}

// listenAddr is the HTTP bind address (CELIKPANEL_LISTEN, e.g. ":1923" or
// "127.0.0.1:1923"). New installations use 1923 (owner decision 2026-10-31:
// 2083 is cPanel's port); installations made before alpha.114 keep the 2083
// their panel.env already names. After a confirmed panel port change the
// running process answers with the new address (panel_port.go), the same one
// panel.env then names.
// listenAddr, HTTP bağlanma adresidir (CELIKPANEL_LISTEN). Yeni kurulum
// 1923'ü kullanır; alpha.114 öncesi kurulumlar panel.env'deki 2083'ü korur.
// confirmedPanelListen is the listen address after a confirmed panel port
// change in this process; listenAddr prefers it over the environment the
// process started with.
var confirmedPanelListen atomic.Pointer[string]

func listenAddr() string {
	if a := confirmedPanelListen.Load(); a != nil && *a != "" {
		return *a
	}
	if a := os.Getenv("CELIKPANEL_LISTEN"); a != "" {
		return a
	}
	return panellisten.DefaultListen
}
