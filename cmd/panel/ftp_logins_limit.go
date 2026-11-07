package main

import "context"

// The plan's number of additional FTP/SFTP logins (alpha.128, G16) is a key
// of the limits chain (docs/SITE-LIMITS.md): server ⊇ reseller ⊇ plan or
// Custom. Its server row is in migration 072 (10 per subscription); a plan,
// reseller plan or Custom set may lower it, 0 gives none. The count is
// checked on the server when a login is added (ftp_logins_handlers.go).
//
// Ek FTP/SFTP girişlerinin plan sayısı, sınır zincirinin bir anahtarıdır.

const settingFTPLogins = "ftp.max_logins"

func init() {
	registerSettingKey(settingKeyDef{Key: settingFTPLogins, Group: "ftp", Unit: "count", Min: 0, Max: 500, Presets: []int{1, 3, 5, 10}})
	ftpLoginsCeiling = func(p *Panel, ctx context.Context, subID int) (ftpLoginLimit, error) {
		if p.administratorWorkspace(ctx, subID) {
			return ftpLoginLimit{}, nil
		}
		limits, err := subscriptionSettingLimits(ctx, p.db.GetDB(), subID)
		if err != nil {
			return ftpLoginLimit{}, err
		}
		limit := limits[settingFTPLogins]
		return ftpLoginLimit{Ceiling: limit.Ceiling, SetBy: limit.SetBy, SetName: limit.SetByName}, nil
	}
}
