package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// A restore refusal is typed only where its cause is certain (D-024); any
// other error stays untyped, as the server's own error.
func TestRestoreRefusalsAreTypedOnlyWhereTheCauseIsCertain(t *testing.T) {
	enospc := &os.PathError{Op: "write", Path: "/var/backups/celikpanel/x", Err: syscall.ENOSPC}
	edquot := &os.PathError{Op: "write", Path: "/var/www/x", Err: syscall.EDQUOT}
	eio := &os.PathError{Op: "read", Path: "/var/backups/celikpanel/x", Err: syscall.EIO}
	missing := &os.PathError{Op: "lstat", Path: "/var/backups/celikpanel/subscriptions/3/domains/6", Err: syscall.ENOENT}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"find: no backup folder for this site", backupFindRefusal(missing), backupspec.RestoreCodeNotFound},
		{"find: no such file", backupFindRefusal(syscall.ENOENT), backupspec.RestoreCodeNotFound},
		{"find: not a backup name", backupFindRefusal(invalidBackupName()), backupspec.RestoreCodeNotFound},
		{"find: a link in the path", backupFindRefusal(errors.New("symlink path component: /var/backups")), ""},
		{"package: another site's", backupPackageRefusal(errManifestScopeMismatch), backupspec.RestoreCodeOtherSite},
		{"package: checksum", backupPackageRefusal(errors.New("payload hash mismatch")), backupspec.RestoreCodeInvalid},
		{"package: newer format", backupPackageRefusal(errors.New("manifest version mismatch")), backupspec.RestoreCodeInvalid},
		{"package: disk full", backupPackageRefusal(enospc), backupspec.RestoreCodeNoSpace},
		{"package: quota", backupPackageRefusal(edquot), backupspec.RestoreCodeNoSpace},
		{"package: read error", backupPackageRefusal(eio), ""},
		{"package: gone while read", backupPackageRefusal(syscall.ENOENT), ""},
		{"staging: disk full", backupStagingRefusal(fmt.Errorf("restore app: %w", enospc)), backupspec.RestoreCodeNoSpace},
		{"staging: unsafe target", backupStagingRefusal(errors.New("restore target is not a safe directory")), ""},
	}
	for _, c := range cases {
		if code, _ := restoreFailureOf(c.err); code != c.want {
			t.Errorf("%s: code %q, want %q (%v)", c.name, code, c.want, c.err)
		}
	}
	if backupPackageRefusal(nil) != nil {
		t.Fatal("no error became a refusal")
	}
	// The other-site, not-found and no-room answers carry no detail: no
	// server path reaches the screen.
	for _, err := range []error{backupFindRefusal(missing), backupPackageRefusal(errManifestScopeMismatch), backupPackageRefusal(enospc)} {
		if _, detail := restoreFailureOf(err); detail != "" {
			t.Errorf("%v carries the detail %q", err, detail)
		}
	}
}
