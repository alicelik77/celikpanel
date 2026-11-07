package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A suspended site account carries an account expiry in the past (the 8th
// field of /etc/shadow, set with chage -E). PAM's account stage (pam_unix)
// refuses an expired account, so cron (Debian/Ubuntu cron and Arch cronie run
// jobs through PAM) skips the user's jobs and SSH, SFTP and FTP refuse its
// login. PHP-FPM pools and systemd units do not go through PAM and are not
// affected; the site itself is stopped by its vhost. The password field is
// never touched, so restoring the old expiry restores the account exactly.
//
// Askıya alınmış site hesabının hesap bitiş tarihi geçmiştedir (chage -E).
// PAM süresi dolmuş hesabı reddeder: cron işleri çalışmaz, SSH/SFTP/FTP
// girişi reddedilir. Parola alanına dokunulmaz.

var (
	// suspensionShadowPath is read for the expiry field only (a seam for tests).
	suspensionShadowPath = "/etc/shadow"
	// suspensionSiteUser derives and proves the site's system account exactly
	// as cron does: the derived name, a tenant uid and this tenant's home.
	suspensionSiteUser = func(account transport.SuspensionSiteAccount) (string, error) {
		return cronTenantUser(transport.CronTenant{
			SubscriptionID: account.SubscriptionID,
			DomainID:       account.DomainID,
			Domain:         account.Domain,
		})
	}
	// suspensionRunChage runs chage with an agent-owned deadline (a seam so
	// tests capture the exact argv).
	suspensionRunChage = func(ctx context.Context, args ...string) ([]byte, error) {
		return serviceMutationCommand(ctx, "chage", args...).CombinedOutputLimited(4096)
	}
	suspensionCommandTimeout = 30 * time.Second
	// suspensionSiteMutex keeps one site-account compare-and-set at a time.
	suspensionSiteMutex sync.Mutex
)

// chageOwnWords: chage's own messages name only the login and a lock or usage
// problem; it never prints a shadow field.
const chageOwnWords = "chage reports only the login name and lock or usage problems; it never prints a shadow field"

// errSuspensionSiteAccountAbsent: the derived system account does not exist.
var errSuspensionSiteAccountAbsent = errors.New("site account is absent")

// suspensionSiteAccountUser proves the account; a missing system user is
// errSuspensionSiteAccountAbsent, anything else is an error (unknown).
func suspensionSiteAccountUser(account transport.SuspensionSiteAccount) (string, error) {
	username, err := suspensionSiteUser(account)
	if err != nil {
		var unknown user.UnknownUserError
		if errors.As(err, &unknown) {
			return "", errSuspensionSiteAccountAbsent
		}
		return "", fmt.Errorf("site account of %s could not be proven: %w", account.Domain, err)
	}
	return username, nil
}

// validShadowExpiry: "" (never expires) or a day count.
func validShadowExpiry(value string) bool {
	if len(value) > 10 {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

// shadowExpiryFor returns the 8th field of username's /etc/shadow line.
// found=false means the file has no line for that user. No other field is
// returned or put into an error: the second field is a password hash.
func shadowExpiryFor(content []byte, username string) (expiry string, found bool, err error) {
	matches := 0
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSuffix(line, "\r")
		name, _, ok := strings.Cut(line, ":")
		if !ok || name != username {
			continue
		}
		matches++
		fields := strings.Split(line, ":")
		if len(fields) != 9 {
			return "", false, fmt.Errorf("the shadow entry of %s is not in the expected 9-field form", username)
		}
		expiry = fields[7]
	}
	if matches == 0 {
		return "", false, nil
	}
	if matches > 1 {
		return "", false, fmt.Errorf("the shadow file lists %s %d times", username, matches)
	}
	if !validShadowExpiry(expiry) {
		return "", false, fmt.Errorf("the account expiry of %s is not a day count", username)
	}
	return expiry, true, nil
}

func readSuspensionSiteAccountState(username string) (transport.SiteAccountState, error) {
	content, err := os.ReadFile(suspensionShadowPath)
	if err != nil {
		return transport.SiteAccountState{}, fmt.Errorf("the shadow file could not be read: %w", err)
	}
	expiry, found, err := shadowExpiryFor(content, username)
	if err != nil {
		return transport.SiteAccountState{}, err
	}
	if !found {
		return transport.SiteAccountState{}, fmt.Errorf("site account %s exists but has no shadow entry", username)
	}
	return transport.SiteAccountState{Present: true, Expiry: expiry}, nil
}

func inspectSuspensionSiteAccount(account transport.SuspensionSiteAccount) (transport.SiteAccountState, error) {
	username, err := suspensionSiteAccountUser(account)
	if errors.Is(err, errSuspensionSiteAccountAbsent) {
		return transport.SiteAccountState{Present: false}, nil
	}
	if err != nil {
		return transport.SiteAccountState{}, err
	}
	return readSuspensionSiteAccountState(username)
}

// chageExpiryArgs is the exact chage argv that sets an expiry; "" (never) is
// written as -1, which chage stores as an empty field.
func chageExpiryArgs(username, expiry string) []string {
	if expiry == "" {
		return []string{"-E", "-1", username}
	}
	return []string{"-E", expiry, username}
}

func describeShadowExpiry(expiry string) string {
	if expiry == "" {
		return "never"
	}
	return expiry
}

// SetSiteAccountSuspensionV1 suspends or restores one site account's login
// and cron by its shadow account expiry.
//
// SetSiteAccountSuspensionV1 bir site hesabının girişini ve cron'unu
// hesap bitiş tarihiyle askıya alır ya da geri verir.
func (a *Agent) SetSiteAccountSuspensionV1(
	req *transport.SetSiteAccountSuspensionRequest,
	resp *transport.SetSiteAccountSuspensionResponse,
) error {
	if req == nil || resp == nil {
		return fmt.Errorf("site account suspension request and response are required")
	}
	*resp = transport.SetSiteAccountSuspensionResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "suspending or restoring a site account"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if !validShadowExpiry(req.Before.Expiry) {
		resp.Error = "the recorded account expiry is not a day count; nothing was changed"
		return nil
	}

	suspensionSiteMutex.Lock()
	defer suspensionSiteMutex.Unlock()

	username, err := suspensionSiteAccountUser(req.Account)
	if errors.Is(err, errSuspensionSiteAccountAbsent) {
		// Nothing to suspend or restore; the Panel treats this as nothing to do.
		resp.Error = fmt.Sprintf("site account is absent: %s has no system account, so nothing was changed", req.Account.Domain)
		return nil
	}
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	current, err := readSuspensionSiteAccountState(username)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	resp.State = current

	target, from := transport.SuspensionSiteAccountExpiry, req.Before.Expiry
	if !req.Suspend {
		target, from = req.Before.Expiry, transport.SuspensionSiteAccountExpiry
	}
	if !req.Before.Present {
		resp.Conflict = true
		resp.Error = fmt.Sprintf(
			"site account %s exists, but the state recorded before suspension says it did not; nothing was changed. "+
				"CelikPanel inspects the account again before retrying",
			username,
		)
		return nil
	}
	switch suspensionDecide(current.Expiry, target, from) {
	case suspensionAlready:
		resp.Applied = true
		followSiteAccountLogins(username, target, resp)
		return nil
	case suspensionConflict:
		resp.Conflict = true
		resp.Error = fmt.Sprintf(
			"site account %s has account expiry %s, which is neither the value recorded before suspension (%s) nor the suspended value (%s); "+
				"it was changed outside CelikPanel, so nothing was changed. The server owner decides which expiry is right "+
				"(chage -E), then the operation can be retried",
			username,
			describeShadowExpiry(current.Expiry),
			describeShadowExpiry(req.Before.Expiry),
			transport.SuspensionSiteAccountExpiry,
		)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), suspensionCommandTimeout)
	defer cancel()
	output, runErr := suspensionRunChage(ctx, chageExpiryArgs(username, target)...)
	after, readErr := readSuspensionSiteAccountState(username)
	if readErr == nil {
		resp.State = after
	}
	if runErr != nil {
		resp.Error = hostcmd.FailVerbatim("chage could not set the account expiry of "+username, output, runErr, chageOwnWords).Error()
		if readErr == nil && after.Expiry != current.Expiry {
			resp.Changed = true
		}
		return nil
	}
	resp.Changed = true
	if readErr != nil {
		resp.Error = "chage finished but the account expiry could not be read back, so the result is unknown: " + readErr.Error()
		return nil
	}
	if after.Expiry != target {
		resp.Error = fmt.Sprintf(
			"chage finished but site account %s reads expiry %s instead of %s; the result is not the wanted state",
			username, describeShadowExpiry(after.Expiry), describeShadowExpiry(target),
		)
		return nil
	}
	resp.Applied = true
	followSiteAccountLogins(username, target, resp)
	return nil
}

// followSiteAccountLogins gives the site's additional FTP logins (alpha.128)
// the site account's expiry. A failure leaves the step unfinished, so the
// operation repeats it (the site account is then already in place).
var followSiteAccountLogins = func(username, expiry string, resp *transport.SetSiteAccountSuspensionResponse) {
	ctx, cancel := context.WithTimeout(context.Background(), suspensionCommandTimeout)
	defer cancel()
	if err := syncFTPLoginsExpiry(ctx, username, expiry); err != nil {
		resp.Applied = false
		resp.Error = err.Error() + "; the site account itself is in place"
	}
}
