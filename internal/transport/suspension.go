package transport

// Subscription suspension (alpha.117, PLANS-AND-RIGHTS 9A). The Panel decides
// which subscription is suspended and keeps the operation's checkpoints; the
// Agent changes one native resource per call and reports what it found.
//
// Every change is compare-and-set against a before-image the Panel captured
// with SuspensionInspectV1 and stored before the change (D-025): a resource
// whose current state is neither the before-image nor the target was changed
// by someone else, so the Agent changes nothing and says Conflict (D-022).
//
// Abonelik askıya alma. Panel hangi aboneliğin askıda olduğuna karar verir ve
// işlemin kontrol noktalarını tutar; Agent her çağrıda tek bir yerel kaynağı
// değiştirir. Her değişiklik, Panel'in önceden kaydettiği ön görüntüye göre
// karşılaştır-ve-yaz ile yapılır; başkasının değiştirdiği kaynağa dokunulmaz.

// SuspensionSiteAccountExpiry is the account expiry (days since 1970-01-01,
// the 8th field of /etc/shadow) a suspended site account carries: the day
// after the epoch, long past. PAM's account check refuses an expired account,
// so cron does not run its jobs and SSH/SFTP/FTP refuse its login.
const SuspensionSiteAccountExpiry = "1"

// SuspensionSiteAccount names a site's system account by its tenant identity;
// the Agent derives and proves the user name, as for cron.
type SuspensionSiteAccount struct {
	SubscriptionID int    `json:"subscription_id"`
	DomainID       int    `json:"domain_id"`
	Domain         string `json:"domain"`
}

// SiteAccountState is the native state of a site account.
type SiteAccountState struct {
	// Present is false when the system account does not exist.
	Present bool `json:"present"`
	// Expiry is the shadow expiry field exactly as stored: "" (never) or a
	// day count.
	Expiry string `json:"expiry"`
}

// SuspensionMailDomain is one mail domain and its mailboxes.
type SuspensionMailDomain struct {
	Domain    string   `json:"domain"`
	Mailboxes []string `json:"mailboxes"`
}

// MailDomainState is the native state of a mail domain.
type MailDomainState struct {
	// DomainListed: the domain has a line in Postfix's virtual mailbox domain
	// map; DomainLine is that line exactly.
	DomainListed bool   `json:"domain_listed"`
	DomainLine   string `json:"domain_line,omitempty"`
	// LoginRefused maps each mailbox found in Dovecot's users file to whether
	// its line carries the nologin field. A mailbox without a line is absent.
	LoginRefused map[string]bool `json:"login_refused"`
}

// SuspensionDatabaseUser is one database account on the local engine.
type SuspensionDatabaseUser struct {
	// Engine is "mysql" (MariaDB) or "postgresql".
	Engine string `json:"engine"`
	// Port selects a PostgreSQL cluster; 0 is the default one.
	Port     int    `json:"port,omitempty"`
	Username string `json:"username"`
}

// DatabaseUserState is the native state of a database account.
type DatabaseUserState struct {
	Present bool `json:"present"`
	// Locked: MariaDB ACCOUNT LOCK, or PostgreSQL NOLOGIN.
	Locked bool `json:"locked"`
}

// SuspensionInspectRequest asks for the current state of resources. It is
// read-only.
type SuspensionInspectRequest struct {
	SiteAccounts  []SuspensionSiteAccount  `json:"site_accounts,omitempty"`
	MailDomains   []SuspensionMailDomain   `json:"mail_domains,omitempty"`
	DatabaseUsers []SuspensionDatabaseUser `json:"database_users,omitempty"`
}

// SiteAccountObservation is one inspected site account; Error is set when it
// could not be read (unknown, not absent).
type SiteAccountObservation struct {
	Account SuspensionSiteAccount `json:"account"`
	State   SiteAccountState      `json:"state"`
	Error   string                `json:"error,omitempty"`
}

// MailDomainObservation is one inspected mail domain.
type MailDomainObservation struct {
	Domain SuspensionMailDomain `json:"domain"`
	State  MailDomainState      `json:"state"`
	Error  string               `json:"error,omitempty"`
}

// DatabaseUserObservation is one inspected database account.
type DatabaseUserObservation struct {
	User  SuspensionDatabaseUser `json:"user"`
	State DatabaseUserState      `json:"state"`
	Error string                 `json:"error,omitempty"`
}

// SuspensionInspectResponse answers in the request's order.
type SuspensionInspectResponse struct {
	SiteAccounts  []SiteAccountObservation  `json:"site_accounts"`
	MailDomains   []MailDomainObservation   `json:"mail_domains"`
	DatabaseUsers []DatabaseUserObservation `json:"database_users"`
	Error         string                    `json:"error,omitempty"`
}

// SuspensionApplyResult is the common answer of the three change calls.
//   - Applied: the resource is now in the wanted state (changed or already so).
//   - Changed: this call changed it.
//   - Conflict: the current state is neither the before-image nor the target;
//     nothing was changed and Error says what was found.
//   - Error without Conflict: the change failed or its result is unknown.
type SuspensionApplyResult struct {
	Applied  bool   `json:"applied"`
	Changed  bool   `json:"changed"`
	Conflict bool   `json:"conflict"`
	Error    string `json:"error,omitempty"`
}

// SetSiteAccountSuspensionRequest suspends (Suspend) or restores one site
// account. Before is the captured before-image.
type SetSiteAccountSuspensionRequest struct {
	ExpectedBuildCommit string                `json:"expected_build_commit"`
	Account             SuspensionSiteAccount `json:"account"`
	Suspend             bool                  `json:"suspend"`
	Before              SiteAccountState      `json:"before"`
}

// SetSiteAccountSuspensionResponse carries the state after the call.
type SetSiteAccountSuspensionResponse struct {
	SuspensionApplyResult
	State SiteAccountState `json:"state"`
}

// SetMailDomainSuspensionRequest suspends or restores one mail domain:
// suspended means its line is out of the virtual mailbox domain map (Postfix
// refuses its mail) and every listed mailbox carries nologin (Dovecot refuses
// IMAP, POP3 and SMTP authentication). Nothing is deleted.
type SetMailDomainSuspensionRequest struct {
	ExpectedBuildCommit string               `json:"expected_build_commit"`
	Domain              SuspensionMailDomain `json:"domain"`
	Suspend             bool                 `json:"suspend"`
	Before              MailDomainState      `json:"before"`
}

// SetMailDomainSuspensionResponse carries the state after the call.
type SetMailDomainSuspensionResponse struct {
	SuspensionApplyResult
	State MailDomainState `json:"state"`
}

// SetDatabaseUserSuspensionRequest locks (and disconnects) or restores one
// database account on the local engine.
type SetDatabaseUserSuspensionRequest struct {
	ExpectedBuildCommit string                 `json:"expected_build_commit"`
	User                SuspensionDatabaseUser `json:"user"`
	Suspend             bool                   `json:"suspend"`
	Before              DatabaseUserState      `json:"before"`
}

// SetDatabaseUserSuspensionResponse carries the state after the call.
type SetDatabaseUserSuspensionResponse struct {
	SuspensionApplyResult
	State DatabaseUserState `json:"state"`
}
