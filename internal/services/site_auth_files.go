package services

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The password files of a site's protected folders (alpha.114).
//
// One file per protected folder, in nginx's own `auth_basic_user_file` format
// (user:hash lines), under /etc/nginx/celikpanel-auth.d/<domain>/. The files
// are outside every web root, mode 0640, root:<the web server's group>, so
// only root and nginx's workers read them. nginx reads them itself, at each
// request: a stopped or removed Panel changes nothing about who can open a
// folder (owner independence). The hash is bcrypt ($2y$), which nginx checks
// through the system's crypt(); Debian 12/13, Ubuntu 22.04/24.04 and Arch all
// ship libxcrypt, which has it.
//
// Each file starts with the same render header as a site's vhost (D-031), so a
// file the owner changed (a user added with htpasswd, say) is recognized and
// kept: the Panel then refuses the change and says why, never overwriting it.
//
// Parolalı klasörlerin parola dosyaları: klasör başına bir dosya, nginx'in
// kendi biçiminde, web kökü dışında, 0640 root:<web grubu>. nginx dosyayı her
// istekte kendisi okur; Panel durmuş ya da kaldırılmış olsa da kimin girebildiği
// değişmez. Sahibin değiştirdiği dosya tanınır, korunur ve üzerine yazılmaz.

const siteAuthDirName = "celikpanel-auth.d"

// SiteAuthUser is one line of a password file.
type SiteAuthUser struct {
	Username string
	Hash     string
}

// SiteAuthFile is one password file: the protected folder it belongs to and
// the people who may open it.
type SiteAuthFile struct {
	ID    int
	Path  string
	Users []SiteAuthUser
}

// SiteAuthDir is the directory of one site's password files.
func SiteAuthDir(domain string) string {
	return nginxBase() + "/" + siteAuthDirName + "/" + domain
}

// SiteAuthFilePath is the password file of one protected folder.
func SiteAuthFilePath(domain string, id int) string {
	return fmt.Sprintf("%s/protect-%d.htpasswd", SiteAuthDir(domain), id)
}

// siteAuthHeaderPrefix is the render header of a password file.
const siteAuthHeaderPrefix = managedRenderHeaderPrefix

// SiteAuthFileKeptError is a password file that is not the Panel's unchanged
// text: it was kept as it is and nothing of this change was written.
type SiteAuthFileKeptError struct {
	Path  string
	State string
}

func (e *SiteAuthFileKeptError) Error() string {
	return fmt.Sprintf("the password file %s is not CelikPanel's unchanged text (%s); it was kept as it is and this change was not applied", e.Path, e.State)
}

// RenderSiteAuthFile is the body of a password file (without the header):
// users sorted by name, so the same users always give the same bytes.
func RenderSiteAuthFile(domain string, file SiteAuthFile) (string, error) {
	users := append([]SiteAuthUser(nil), file.Users...)
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	var body strings.Builder
	fmt.Fprintf(&body, "# Password file of the protected folder %s of %s, written by CelikPanel.\n", file.Path, domain)
	body.WriteString("# CelikPanel keeps this file in step with the panel; a file you change is kept as it is.\n")
	body.WriteString("# CelikPanel yazdı. Dosyayı değiştirirseniz olduğu gibi korunur.\n")
	seen := make(map[string]bool, len(users))
	for _, entry := range users {
		if err := ValidateSiteProtectedUsername(entry.Username); err != nil {
			return "", err
		}
		if seen[entry.Username] {
			return "", fmt.Errorf("user %q is listed twice", entry.Username)
		}
		seen[entry.Username] = true
		if !validBcryptHash(entry.Hash) {
			return "", fmt.Errorf("the password of user %q is not a bcrypt hash", entry.Username)
		}
		body.WriteString(entry.Username + ":" + entry.Hash + "\n")
	}
	return body.String(), nil
}

// validBcryptHash accepts only the 60-character bcrypt text, so nothing but a
// hash (no colon, no newline, no space) reaches a password file's line.
func validBcryptHash(hash string) bool {
	if len(hash) != 60 || !(strings.HasPrefix(hash, "$2y$") || strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$")) {
		return false
	}
	if hash[6] != '$' {
		return false
	}
	for _, r := range hash[7:] {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '/':
		default:
			return false
		}
	}
	return true
}

// SiteAuthChange is a set of password files written for one vhost apply, and
// what is needed to put every file back or to finish the change.
type SiteAuthChange struct {
	domain  string
	desired map[string]bool
	touched []siteAuthBackup
}

type siteAuthBackup struct {
	path    string
	existed bool
	content []byte
}

// siteAuthOwnership is the web server's group, which the files and their
// directory belong to; not set in a development directory
// (CELIKPANEL_NGINX_DIR) or when the Agent does not run as root.
func siteAuthOwnership() (gid int, set bool) {
	if os.Geteuid() != 0 || nginxDevMode() {
		return 0, false
	}
	for _, name := range []string{"www-data", "nginx", "http"} {
		if group, err := user.LookupGroup(name); err == nil {
			if id, err := strconv.Atoi(group.Gid); err == nil {
				return id, true
			}
		}
	}
	return 0, false
}

// PrepareSiteAuthFiles writes the site's password files (those whose bytes
// differ) and returns the change. A file that is there and is not the Panel's
// unchanged text is kept: nothing is written and the error names it. Call
// Commit when the vhost that reads the files was applied, Rollback when it was
// not.
func PrepareSiteAuthFiles(domain string, files []SiteAuthFile) (*SiteAuthChange, error) {
	nginxMutationMu.Lock()
	defer nginxMutationMu.Unlock()
	change := &SiteAuthChange{domain: domain, desired: make(map[string]bool, len(files))}
	type planned struct {
		path    string
		content []byte
		same    bool
		backup  siteAuthBackup
	}
	var plans []planned
	for _, file := range files {
		body, err := RenderSiteAuthFile(domain, file)
		if err != nil {
			return nil, err
		}
		path := SiteAuthFilePath(domain, file.ID)
		change.desired[path] = true
		sealed := []byte(SealManagedText(body))
		inspected := inspectSiteFileWith(path, siteAuthHeaderPrefix, nil, "")
		switch inspected.state {
		case transport.SiteFileAbsent:
			plans = append(plans, planned{path: path, content: sealed})
		case transport.SiteFileManagedUnchanged:
			same := string(inspected.content) == string(sealed) && inspected.mode == 0o640
			plans = append(plans, planned{path: path, content: sealed, same: same,
				backup: siteAuthBackup{path: path, existed: true, content: inspected.content}})
		default:
			return nil, &SiteAuthFileKeptError{Path: path, State: inspected.state}
		}
	}
	for _, plan := range plans {
		if plan.same {
			continue
		}
		if err := writeSiteAuthFile(plan.path, plan.content); err != nil {
			rollbackErr := change.rollbackLocked()
			return nil, errors.Join(fmt.Errorf("write the password file %s: %w", plan.path, err), rollbackErr)
		}
		backup := plan.backup
		if backup.path == "" {
			backup = siteAuthBackup{path: plan.path}
		}
		change.touched = append(change.touched, backup)
	}
	return change, nil
}

func writeSiteAuthFile(path string, content []byte) error {
	directory := filepath.Dir(path)
	gid, owned := siteAuthOwnership()
	if err := ensureSiteAuthDir(directory, gid, owned); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".celikpanel-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	published := false
	defer func() {
		_ = temporary.Close()
		if !published {
			_ = os.Remove(name)
		}
	}()
	if owned {
		if err := temporary.Chown(0, gid); err != nil {
			return fmt.Errorf("set the file's group: %w", err)
		}
	}
	if err := temporary.Chmod(0o640); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	published = true
	return syncDirectory(directory)
}

// ensureSiteAuthDir makes the directory (and its parent) 0750 root:<web
// group>, whatever the umask.
func ensureSiteAuthDir(directory string, gid int, owned bool) error {
	for _, dir := range []string{filepath.Dir(directory), directory} {
		if info, err := os.Lstat(dir); err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s exists and is not a directory", dir)
			}
			continue
		}
		if err := os.Mkdir(dir, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := os.Chmod(dir, 0o750); err != nil {
			return err
		}
		if owned {
			if err := os.Chown(dir, 0, gid); err != nil {
				return err
			}
		}
	}
	return nil
}

// Rollback puts every file this change wrote back as it was (a file that did
// not exist is removed).
func (c *SiteAuthChange) Rollback() error {
	if c == nil {
		return nil
	}
	nginxMutationMu.Lock()
	defer nginxMutationMu.Unlock()
	return c.rollbackLocked()
}

func (c *SiteAuthChange) rollbackLocked() error {
	var errs []error
	for i := len(c.touched) - 1; i >= 0; i-- {
		backup := c.touched[i]
		if !backup.existed {
			if err := os.Remove(backup.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
			continue
		}
		if err := writeSiteAuthFile(backup.path, backup.content); err != nil {
			errs = append(errs, err)
		}
	}
	c.touched = nil
	return errors.Join(errs...)
}

// Commit finishes the change after the vhost was applied: password files of
// this site that nothing asks for any more are removed, when they are still
// the Panel's unchanged text (anything else is left), and the directory when
// it is empty.
func (c *SiteAuthChange) Commit() {
	if c == nil {
		return
	}
	nginxMutationMu.Lock()
	defer nginxMutationMu.Unlock()
	directory := SiteAuthDir(c.domain)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "protect-") || !strings.HasSuffix(name, ".htpasswd") {
			continue
		}
		path := directory + "/" + name
		if c.desired[path] {
			continue
		}
		if inspectSiteFileWith(path, siteAuthHeaderPrefix, nil, "").state == transport.SiteFileManagedUnchanged {
			_ = os.Remove(path)
		}
	}
	_ = os.Remove(directory) // fails, and leaves it, when not empty
}

// RemoveSiteAuthFiles removes a deleted site's password files and directory:
// the Panel's unchanged files only; anything else is left.
func RemoveSiteAuthFiles(domain string) {
	(&SiteAuthChange{domain: domain, desired: map[string]bool{}}).Commit()
}
