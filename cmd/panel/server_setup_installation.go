package main

import (
	"context"
	"fmt"
	"regexp"
)

// serverSetupInstallationKey names the panel_settings row that holds this
// installation's identity.
const serverSetupInstallationKey = "installation_id"

var serverSetupInstallationPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// serverSetupInstallationID is the identity of this panel installation: a
// random value made the first time anything asks for it and kept in the
// panel's own database. A server that is wiped and installed again at the
// same address has a different one, so a browser that still holds a setup
// start marker from the earlier installation can tell the marker is not this
// server's and drop it (a marker never selects another installation's work).
// The value only distinguishes installations; it grants nothing and is not a
// secret. It is made with a conditional insert, so two requests at once end
// with the same value.
// Kurulum kimligi: ayni adreste silinip yeniden kurulan sunucu baska bir
// kimlik tasir; tarayicida kalan eski baslatma isareti bu sayede ayirt edilir.
func (p *Panel) serverSetupInstallationID(ctx context.Context) (string, error) {
	read := func() (string, error) {
		var value string
		err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, serverSetupInstallationKey).Scan(&value)
		return value, err
	}
	if value, err := read(); err == nil && serverSetupInstallationPattern.MatchString(value) {
		return value, nil
	}
	fresh, err := randomHexToken(16)
	if err != nil {
		return "", err
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `INSERT OR IGNORE INTO panel_settings(key,value) VALUES(?,?)`, serverSetupInstallationKey, fresh); err != nil {
		return "", err
	}
	value, err := read()
	if err != nil {
		return "", err
	}
	if !serverSetupInstallationPattern.MatchString(value) {
		return "", fmt.Errorf("invalid installation identity")
	}
	return value, nil
}
