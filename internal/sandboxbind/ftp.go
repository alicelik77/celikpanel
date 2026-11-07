package sandboxbind

import (
	"errors"
	"fmt"
	"path"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// The FTP login jail (alpha.128, gap G16, docs/FTP-LOGINS.md). An additional
// FTP/SFTP login of a site is confined to one folder of the site. sshd and
// vsftpd need a root-owned chroot, which a folder of the site is not, and the
// site account could swap the folder for a link. So the helper opens the
// folder from the hosting tree one folder at a time without following any
// link, checks its owners, and binds it by descriptor onto
// /run/celikpanel-ftp/<login>/files; the login's chroot is
// /run/celikpanel-ftp/<login> (root:<site group> 0750). The unit
// celikpanel-ftp-<login>.service runs "sandbox-helper ftp-bind" at every
// boot; when a check fails there is no jail, and sshd and vsftpd refuse the
// login.
//
// FTP giriş hapsi: seçilen klasör bağlantı izlenmeden açılır, sahipleri
// denetlenir ve root'a ait hapis klasörüne tanımlayıcıyla bağlanır.

// FTPRequest is one ftp-bind call: the values of the login's unit.
type FTPRequest struct {
	Login  string
	Site   string
	Folder string // the host path of the chosen folder
}

// FTPAccount is what the helper reads of a local account.
type FTPAccount struct {
	Name  string
	UID   uint32
	GID   uint32
	Home  string
	Shell string
}

// PlanForLoginFolder is the walk to a login's folder: root owns the
// subscription and its "sites" folder, the site account owns its home and
// every folder below it, and no folder below the home is a mount point.
func PlanForLoginFolder(folder string, siteUID uint32) (Plan, error) {
	subscriptionID, domainID, rel, err := fileaccess.SplitLoginFolderPath(folder)
	if err != nil {
		return Plan{}, err
	}
	if siteUID < 1000 {
		return Plan{}, errors.New("the site's account is a system account")
	}
	root := hostingpath.SubscriptionsRoot()
	home := fileaccess.SiteHome(subscriptionID, domainID)
	plan := Plan{
		Start: root, StartUID: 0, AppDir: folder, Home: home,
		Steps: []Step{
			{Name: fmt.Sprint(subscriptionID), UID: 0, Private: true},
			{Name: "sites", UID: 0, Private: true},
			{Name: path.Base(home), UID: siteUID},
		},
	}
	for _, name := range fileaccess.FolderParts(rel) {
		plan.Steps = append(plan.Steps, Step{Name: name, UID: siteUID, SameMount: true})
	}
	return plan, nil
}

// CheckLoginAccounts proves that login is an additional login of site: both
// accounts exist, the site account is a hosting account whose home is the
// home on the folder's way, the login has the site's uid and primary group,
// the login's home is its jail and its shell gives no command line.
func CheckLoginAccounts(req FTPRequest, plan Plan, site, login FTPAccount) error {
	switch {
	case site.UID < 1000:
		return refused(CodeNotOwned, "", "the site account %s is a system account", req.Site)
	case path.Clean(site.Home) != plan.Home:
		return refused(CodeNotOwned, plan.AppDir, "is not in the home of the site account %s", req.Site)
	case login.UID != site.UID || login.GID != site.GID:
		return refused(CodeNotOwned, "", "the login %s does not have the uid and group of the site account %s", req.Login, req.Site)
	case login.Home != fileaccess.LoginHome(req.Login):
		return refused(CodeNotOwned, "", "the home of the login %s is %s, not its jail %s", req.Login, login.Home, fileaccess.LoginHome(req.Login))
	case fileaccess.UsableShell(login.Shell):
		return refused(CodeNotOwned, "", "the login %s has a command line shell (%s)", req.Login, login.Shell)
	}
	return nil
}

// ValidFTPRequest checks the names before anything is looked up.
func ValidFTPRequest(req FTPRequest) error {
	if err := fileaccess.ValidateLoginName(req.Login); err != nil {
		return fmt.Errorf("not a login name: %v", err)
	}
	if !fileaccess.ValidUsername(req.Site) || req.Site == "root" {
		return errors.New("not a site account name")
	}
	if _, _, _, err := fileaccess.SplitLoginFolderPath(req.Folder); err != nil {
		return err
	}
	return nil
}
