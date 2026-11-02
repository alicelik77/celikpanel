package transport

// Credential rotation after a subscription transfer (alpha.117, D-042
// section 8). The Panel runs it as a durable operation; these are the two
// Agent calls it adds. A password travels only inside one request and is
// never logged, stored in plain text or returned.
//
// Devirden sonra parola ve anahtar yenileme. Parola yalnız bir istekte
// taşınır; günlüğe yazılmaz, düz metin saklanmaz, geri gönderilmez.

// Password bounds of a database user's new password.
const (
	MinDatabaseUserPasswordBytes = 12
	MaxDatabaseUserPasswordBytes = 128
)

// DatabaseUserApp is an application CelikPanel installed that reads this
// database user's password from its own configuration file. Today only
// WordPress (wp-config.php in the site's document root).
type DatabaseUserApp struct {
	Kind           string `json:"kind"`
	SubscriptionID int    `json:"subscription_id"`
	DomainID       int    `json:"domain_id"`
	// SiteUsername owns the configuration file.
	SiteUsername string `json:"site_username"`
	DBName       string `json:"db_name"`
	// Required: when the file does not name this database and user any more,
	// nothing is changed (a rotation must not break the site). Without it the
	// engine password changes and the file is left alone.
	Required bool `json:"required"`
}

// RotateDatabaseUserPasswordRequest sets a new password on the local engine,
// ends the account's open sessions and, with App, writes the same password
// into the application's configuration file.
type RotateDatabaseUserPasswordRequest struct {
	ExpectedBuildCommit string                 `json:"expected_build_commit"`
	User                SuspensionDatabaseUser `json:"user"`
	NewPassword         string                 `json:"new_password"`
	App                 *DatabaseUserApp       `json:"app,omitempty"`
}

// RotateDatabaseUserPasswordResponse says what happened, step by step.
type RotateDatabaseUserPasswordResponse struct {
	// Applied: every step finished.
	Applied bool `json:"applied"`
	// PasswordChanged: the engine has the new password now.
	PasswordChanged bool `json:"password_changed"`
	SessionsEnded   bool `json:"sessions_ended"`
	// AppUpdated: the configuration file holds the new password.
	AppUpdated bool `json:"app_updated"`
	// AppDetail says why the file was not changed.
	AppDetail string `json:"app_detail,omitempty"`
	// AppConflict: App was required and the file does not match; nothing
	// was changed.
	AppConflict bool `json:"app_conflict,omitempty"`
	// Absent: the user does not exist on the engine; nothing was changed.
	Absent bool   `json:"absent,omitempty"`
	Error  string `json:"error,omitempty"`
}

// EndMailSessionsRequest ends a mailbox's open IMAP/POP3 sessions, so a
// client that signed in before a password change cannot keep reading.
type EndMailSessionsRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Email               string `json:"email"`
}

type EndMailSessionsResponse struct {
	Applied bool   `json:"applied"`
	Ended   bool   `json:"ended"`
	Error   string `json:"error,omitempty"`
}
