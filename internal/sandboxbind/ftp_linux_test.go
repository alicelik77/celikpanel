//go:build linux

package sandboxbind

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicelik/celikpanel/internal/fileaccess"
)

type ftpTree struct {
	root, home, folder, jailRoot string
	uid, gid                     uint32
	mounter                      *fakeMounter
	site, login                  FTPAccount
	groupErr                     error
}

// newFTPTree builds <tmp>/subs/3/sites/5/public_html/uploads, all the test
// account's, and a helper whose plan walks it as PlanForLoginFolder walks the
// hosting tree.
func newFTPTree(t *testing.T) *ftpTree {
	t.Helper()
	base := t.TempDir()
	tr := &ftpTree{root: filepath.Join(base, "subs"), uid: uint32(os.Getuid()), gid: uint32(os.Getgid()), mounter: &fakeMounter{}}
	tr.home = filepath.Join(tr.root, "3", "sites", "5")
	tr.folder = filepath.Join(tr.home, "public_html", "uploads")
	tr.jailRoot = filepath.Join(base, "run-ftp")
	for _, dir := range []string{tr.root, filepath.Join(tr.root, "3"), filepath.Join(tr.root, "3", "sites"), tr.home,
		filepath.Join(tr.home, "public_html"), tr.folder} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The site account is a hosting account (uid 1000 or more): as root the
	// site's part of the tree is given to such an account.
	siteUID, siteGID := tr.uid, tr.gid
	if siteUID < 1000 {
		if siteUID != 0 {
			t.Skip("needs root or an account with uid 1000 or more")
		}
		siteUID, siteGID = 4242, 4242
		for _, dir := range []string{tr.home, filepath.Join(tr.home, "public_html"), tr.folder} {
			if err := os.Lchown(dir, int(siteUID), int(siteGID)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tr.site = FTPAccount{Name: "example_com", UID: siteUID, GID: siteGID, Home: tr.home, Shell: "/usr/sbin/nologin"}
	tr.login = FTPAccount{Name: "anna-uploads", UID: siteUID, GID: siteGID, Home: fileaccess.LoginHome("anna-uploads"), Shell: "/usr/sbin/nologin"}
	return tr
}

func (tr *ftpTree) plan(siteUID uint32) Plan {
	plan := Plan{Start: tr.root, StartUID: tr.uid, Home: tr.home, AppDir: tr.folder,
		Steps: []Step{
			{Name: "3", UID: tr.uid, Private: true},
			{Name: "sites", UID: tr.uid, Private: true},
			{Name: "5", UID: siteUID},
			{Name: "public_html", UID: siteUID, SameMount: true},
			{Name: "uploads", UID: siteUID, SameMount: true},
		}}
	return plan
}

func (tr *ftpTree) helper() FTPHelper {
	return FTPHelper{
		JailRoot: tr.jailRoot, RootUID: tr.uid, Mounter: tr.mounter,
		Plan: func(string, uint32) (Plan, error) { return tr.plan(tr.site.UID), nil },
		LookupAccount: func(name string) (FTPAccount, error) {
			switch name {
			case tr.site.Name:
				return tr.site, nil
			case tr.login.Name:
				return tr.login, nil
			}
			return FTPAccount{}, errors.New("no such account")
		},
		GroupPrivate: func(uint32, uint32) error { return tr.groupErr },
		Log:          &bytes.Buffer{},
	}
}

func ftpReq() FTPRequest {
	return FTPRequest{Login: "anna-uploads", Site: "example_com",
		Folder: "/var/www/celikpanel/subscriptions/3/sites/5/public_html/uploads"}
}

func TestFTPBindBindsTheOpenedFolderOntoARootJail(t *testing.T) {
	tr := newFTPTree(t)
	h := tr.helper()
	if err := h.Bind(ftpReq()); err != nil {
		t.Fatal(err)
	}
	files := filepath.Join(tr.jailRoot, "anna-uploads", "files")
	if got := tr.mounter.bound[files]; got != tr.folder {
		t.Fatalf("bound %q onto %s, want %q", got, files, tr.folder)
	}
	for dir, mode := range map[string]os.FileMode{tr.jailRoot: 0o711, filepath.Dir(files): 0o750, files: 0o700} {
		fi, err := os.Lstat(dir)
		if err != nil || !fi.IsDir() || fi.Mode().Perm() != mode {
			t.Fatalf("%s: %v %v, want %v", dir, fi.Mode(), err, mode)
		}
	}
	// A second start binds again (one mount, not two stacked).
	if err := h.Bind(ftpReq()); err != nil || len(tr.mounter.bound) != 1 || tr.mounter.unbinds != 1 {
		t.Fatalf("rebind: %v bound=%v unbinds=%d", err, tr.mounter.bound, tr.mounter.unbinds)
	}
	if err := h.Unbind("anna-uploads"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Dir(files)); !os.IsNotExist(err) || len(tr.mounter.bound) != 0 {
		t.Fatalf("jail left behind: %v %v", err, tr.mounter.bound)
	}
}

func TestFTPBindRefusesALinkAndLeavesNoJail(t *testing.T) {
	for _, swap := range []string{"folder", "parent"} {
		t.Run(swap, func(t *testing.T) {
			tr := newFTPTree(t)
			h := tr.helper()
			if err := h.Bind(ftpReq()); err != nil {
				t.Fatal(err)
			}
			// The site account swaps a folder on the way for a link to a
			// folder it must not hand out (here: the hosting root).
			victim := tr.folder
			if swap == "parent" {
				victim = filepath.Dir(tr.folder)
			}
			if err := os.Rename(victim, victim+".real"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(tr.root, victim); err != nil {
				t.Fatal(err)
			}
			err := h.Bind(ftpReq())
			if r, ok := AsRefusal(err); !ok || r.Code != CodeNotReal {
				t.Fatalf("link accepted or wrong refusal: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(tr.jailRoot, "anna-uploads")); !os.IsNotExist(err) || len(tr.mounter.bound) != 0 {
				t.Fatalf("a jail outlived the failed check: %v %v", err, tr.mounter.bound)
			}
		})
	}
}

func TestFTPBindProvesTheLoginBelongsToTheSite(t *testing.T) {
	cases := map[string]func(tr *ftpTree){
		"other uid":         func(tr *ftpTree) { tr.login.UID++ },
		"other group":       func(tr *ftpTree) { tr.login.GID++ },
		"home not the jail": func(tr *ftpTree) { tr.login.Home = tr.home },
		"a shell":           func(tr *ftpTree) { tr.login.Shell = "/bin/bash" },
		"site home elsewhere": func(tr *ftpTree) {
			tr.site.Home = filepath.Join(tr.root, "3", "sites", "6")
		},
		"shared group": func(tr *ftpTree) { tr.groupErr = errors.New("www-data is a member") },
		"system site":  func(tr *ftpTree) { tr.site.UID, tr.login.UID = 33, 33 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			tr := newFTPTree(t)
			change(tr)
			if err := tr.helper().Bind(ftpReq()); err == nil {
				t.Fatal("bound")
			}
			if len(tr.mounter.bound) != 0 {
				t.Fatalf("mounted: %v", tr.mounter.bound)
			}
		})
	}
}

func TestFTPBindRefusesAFolderOfAnotherOwner(t *testing.T) {
	tr := newFTPTree(t)
	h := tr.helper()
	h.Plan = func(string, uint32) (Plan, error) {
		plan := tr.plan(tr.site.UID)
		plan.Steps[4].UID = tr.site.UID + 1 // the folder must be the site's
		return plan, nil
	}
	if r, ok := AsRefusal(h.Bind(ftpReq())); !ok || r.Code != CodeNotOwned {
		t.Fatalf("foreign folder: %v", r)
	}
}

func TestFTPRequestsAndPlansAcceptOnlySiteFolders(t *testing.T) {
	if err := ValidFTPRequest(ftpReq()); err != nil {
		t.Fatal(err)
	}
	for _, req := range []FTPRequest{
		{Login: "anna", Site: "example_com", Folder: ftpReq().Folder},
		{Login: "anna-uploads", Site: "root", Folder: ftpReq().Folder},
		{Login: "anna-uploads", Site: "example_com", Folder: "/etc"},
		{Login: "anna-uploads", Site: "example_com", Folder: "/var/www/celikpanel/subscriptions/3/sites/5/../6"},
		{Login: "anna-uploads", Site: "example_com", Folder: "/var/www/celikpanel/subscriptions/3/sites"},
	} {
		if err := ValidFTPRequest(req); err == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
	plan, err := PlanForLoginFolder("/var/www/celikpanel/subscriptions/3/sites/5/public_html/uploads", 1001)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Home != "/var/www/celikpanel/subscriptions/3/sites/5" || len(plan.Steps) != 5 ||
		plan.Steps[0] != (Step{Name: "3", UID: 0, Private: true}) || plan.Steps[2] != (Step{Name: "5", UID: 1001}) ||
		plan.Steps[4] != (Step{Name: "uploads", UID: 1001, SameMount: true}) {
		t.Fatalf("plan %+v", plan)
	}
	if home, err := PlanForLoginFolder("/var/www/celikpanel/subscriptions/3/sites/5", 1001); err != nil || len(home.Steps) != 3 {
		t.Fatalf("home plan %+v %v", home, err)
	}
	if _, err := PlanForLoginFolder("/var/www/celikpanel/subscriptions/3/sites/5/x", 33); err == nil {
		t.Fatal("system account accepted")
	}
}

func TestLocalGroupPrivate(t *testing.T) {
	dir := t.TempDir()
	oldPasswd, oldGroup := ftpPasswdPath, ftpGroupPath
	t.Cleanup(func() { ftpPasswdPath, ftpGroupPath = oldPasswd, oldGroup })
	ftpPasswdPath, ftpGroupPath = filepath.Join(dir, "passwd"), filepath.Join(dir, "group")
	write := func(path, text string) {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(ftpPasswdPath, "root:x:0:0::/root:/bin/bash\nexample_com:x:1001:1001::/s:/usr/sbin/nologin\nanna-uploads:x:1001:1001::/run/celikpanel-ftp/anna-uploads/./files:/usr/sbin/nologin\nwww-data:x:33:33::/var/www:/usr/sbin/nologin\n")
	write(ftpGroupPath, "example_com:x:1001:\nwww-data:x:33:\n")
	if err := LocalGroupPrivate(1001, 1001); err != nil {
		t.Fatal(err)
	}
	write(ftpGroupPath, "example_com:x:1001:www-data\n")
	if err := LocalGroupPrivate(1001, 1001); err == nil {
		t.Fatal("a member from outside accepted")
	}
	write(ftpGroupPath, "example_com:x:1001:\n")
	write(ftpPasswdPath, "example_com:x:1001:1001::/s:/usr/sbin/nologin\nother:x:1002:1001::/o:/usr/sbin/nologin\n")
	if err := LocalGroupPrivate(1001, 1001); err == nil {
		t.Fatal("another account with the group accepted")
	}
	if _, err := LookupLocalAccount("example_com"); err != nil {
		t.Fatal(err)
	}
	write(ftpPasswdPath, "example_com:x:1001:1001::/s:/usr/sbin/nologin\nexample_com:x:0:0::/:/bin/sh\n")
	if _, err := LookupLocalAccount("example_com"); err == nil {
		t.Fatal("a name listed twice accepted")
	}
}
