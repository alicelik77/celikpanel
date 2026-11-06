// Package sandboxbind is the site sandbox's helper (alpha.119,
// docs/SITE-SANDBOX.md §1a). It runs as root outside any sandbox, in the
// bind unit celikpanel-sandbox@<unit>.service that every sandboxed unit
// requires: it reads the sandboxed unit's settings from systemd, opens the
// app folder without following any link, checks who owns every folder on
// the way, binds the opened folder by its descriptor onto a root-owned mount
// point the unit binds into the sandbox. It reads no environment file: the
// units' values are root's own files outside the hosting tree
// (/etc/celikpanel-runtime, /etc/celikpanel-svcapps). In the sandboxed unit
// itself it loads the
// SMTP egress block with nft in an empty environment.
//
// The helper is the Agent binary started as "sandbox-helper" (a copy the
// Agent installs beside the egress table, so it keeps working without
// CelikPanel, D-022). It reads nothing from its environment.
//
// Paket sandboxbind, site korumasının yardımcısıdır: uygulama klasörünü hiçbir
// bağlantıyı izlemeden açar, sahiplerini denetler ve açtığı klasörü root'a ait
// bir bağlama noktasına bağlar.
package sandboxbind

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// Refusal codes. They are the runtime's entry codes and the service apps'
// reasons; the screens word them (D-024).
const (
	// CodeNotReal: a folder on the way is a link, not a folder, missing, or
	// another file system's mount point below the site home.
	CodeNotReal = "app_folder_not_real"
	// CodeNotOwned: a folder belongs to another account than it must, or
	// others may change it.
	CodeNotOwned = "app_folder_not_owned"
	// CodeUnreadable: the folder could not be checked (an unknown result).
	CodeUnreadable = "app_folder_unreadable"
)

// Refusal is why the helper did not start a unit.
type Refusal struct {
	Code   string
	Path   string
	Reason string
}

func (r *Refusal) Error() string {
	if r.Path == "" {
		return r.Reason
	}
	return r.Path + " " + r.Reason
}

func refused(code, p, format string, args ...any) *Refusal {
	return &Refusal{Code: code, Path: p, Reason: fmt.Sprintf(format, args...)}
}

// AsRefusal returns the refusal inside err, if any.
func AsRefusal(err error) (*Refusal, bool) {
	var r *Refusal
	ok := errors.As(err, &r)
	return r, ok
}

// Step is one folder the walk opens beneath the previous one.
type Step struct {
	Name string
	// UID is the owner the folder must have.
	UID uint32
	// Private: neither its group nor others may write to it (a folder above
	// the site home: whoever can write to it can rename the site home).
	Private bool
	// SameMount: the folder may not be a mount point (below the site home,
	// where only the customer creates folders).
	SameMount bool
}

// Plan is the walk from a trusted root-owned folder to an app folder.
type Plan struct {
	// Start is opened by its name; it is root's (the hosting tree).
	Start    string
	StartUID uint32
	Steps    []Step
	// Home is the site home on the way (the account's home must be it).
	Home string
	// AppDir is the app folder's path, for messages.
	AppDir string
}

var (
	idPart  = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)
	appPart = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

// PlanFor is the walk to appDir, which must be a site's runtime folder
// (<subscriptions>/<s>/sites/<d>/app), a service app's folder
// (<subscriptions>/<s>/sites/<d>/apps/<app>) or a Node.js site's document
// root (<subscriptions>/<s>/sites/<d>/public_html, alpha.127): root owns the subscription and
// its "sites" folder (the SFTP chroot), the site's account owns its home and
// everything below it.
func PlanFor(appDir string, siteUID uint32) (Plan, error) {
	root := hostingpath.SubscriptionsRoot()
	clean := path.Clean(appDir)
	if clean != appDir || !strings.HasPrefix(clean, root+"/") {
		return Plan{}, fmt.Errorf("%s is not an app folder in the hosting tree", appDir)
	}
	parts := strings.Split(strings.TrimPrefix(clean, root+"/"), "/")
	ok := len(parts) >= 4 && idPart.MatchString(parts[0]) && parts[1] == "sites" && idPart.MatchString(parts[2])
	switch {
	case !ok:
	case len(parts) == 4 && (parts[3] == "app" || parts[3] == "public_html"):
	case len(parts) == 5 && parts[3] == "apps" && appPart.MatchString(parts[4]):
	default:
		ok = false
	}
	if !ok {
		return Plan{}, fmt.Errorf("%s is not an app folder of a site", appDir)
	}
	if siteUID < 1000 {
		return Plan{}, errors.New("the site's account is a system account")
	}
	plan := Plan{
		Start: root, StartUID: 0, AppDir: clean,
		Home: path.Join(root, parts[0], "sites", parts[2]),
		Steps: []Step{
			{Name: parts[0], UID: 0, Private: true},
			{Name: "sites", UID: 0, Private: true},
			{Name: parts[2], UID: siteUID},
		},
	}
	for _, name := range parts[3:] {
		plan.Steps = append(plan.Steps, Step{Name: name, UID: siteUID, SameMount: true})
	}
	return plan, nil
}

// StartRequest is one "start" call.
type StartRequest struct {
	Unit   string
	AppDir string
	User   string
}
