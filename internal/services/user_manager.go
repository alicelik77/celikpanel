package services

import (
	"fmt"
	"os/exec"
)

type UserManager struct{}

func NewUserManager() *UserManager {
	return &UserManager{}
}

// SiteUserShell and SiteUserGroup: a new site account has no command line
// and is in the group whose SSH sign-in is refused unless the account is
// given file access (alpha.117, internal/fileaccess). Existing accounts are
// not changed on update; enabling file access reconciles them.
const (
	SiteUserShell = "/usr/sbin/nologin"
	SiteUserGroup = "celikpanel-site"
)

// CreateUser creates a new Linux system user for a site. It deliberately
// delegates the existence check to useradd so the check and creation are one
// atomic OS operation. Existing users are never adopted or password-reset.
// The bool reports whether useradd succeeded; callers use it to avoid deleting
// somebody else's account when compensating a failed concurrent create.
//
// Since alpha.117 the account gets no password (useradd leaves it locked)
// and no login shell; the password argument is ignored. A password is set
// only when file access is enabled for the site, by its owner.
func (um *UserManager) CreateUser(username string, homeDir string, _ string) (bool, error) {
	if output, err := exec.Command("groupadd", "-r", "-f", SiteUserGroup).CombinedOutput(); err != nil {
		return false, fmt.Errorf("failed to create group %s: %s", SiteUserGroup, string(output))
	}
	cmd := exec.Command("useradd", "-m", "-d", homeDir, "-s", SiteUserShell, "-G", SiteUserGroup, username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to create user: %s", string(output))
	}
	return true, nil
}

// DeleteUser removes a Linux system user
func (um *UserManager) DeleteUser(username string) error {
	cmd := exec.Command("userdel", "-r", username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete user: %s", string(output))
	}
	return nil
}

// SetOwnership sets directory ownership to user
func (um *UserManager) SetOwnership(path string, username string) error {
	cmd := exec.Command("chown", "-R", fmt.Sprintf("%s:%s", username, username), path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set ownership: %s", string(output))
	}
	return nil
}

// UserExists checks if a user exists
func (um *UserManager) UserExists(username string) bool {
	cmd := exec.Command("id", username)
	err := cmd.Run()
	return err == nil
}
