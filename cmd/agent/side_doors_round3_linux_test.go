//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1, third fix round (23 Nov 2026): the findings of the third
// independent review of PR #348. Each test fails on the merge of alpha.144
// into the branch (b6d3c832e), or does not build there.

const packagedApacheSnippet = "<FilesMatch \".+\\.ph(?:ar|p|tml)$\">\n    SetHandler \"proxy:unix:/run/php/php%s-fpm.sock|fcgi://localhost\"\n</FilesMatch>\n"

func writeRound3File(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// apacheTree is Debian's layout: apache2.conf loads the enabled folders; the
// PHP-FPM package's own snippets lie in conf-available.
func apacheTree(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "apache2")
	writeRound3File(t, filepath.Join(root, "apache2.conf"),
		"# Include conf-available/never.conf\nInclude ports.conf\nIncludeOptional mods-enabled/*.load\nIncludeOptional mods-enabled/*.conf\nIncludeOptional conf-enabled/*.conf\nIncludeOptional sites-enabled/*.conf\n")
	writeRound3File(t, filepath.Join(root, "ports.conf"), "Listen 80\n")
	for _, v := range []string{"8.3", "8.4"} {
		writeRound3File(t, filepath.Join(root, "conf-available", "php"+v+"-fpm.conf"), strings.ReplaceAll(packagedApacheSnippet, "%s", v))
	}
	if err := os.MkdirAll(filepath.Join(root, "conf-enabled"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func apacheNames(files []stockPoolConfigText) []string {
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}
	return names
}

// Finding 1: the package's snippet that nothing enabled is not a user; one
// the owner enabled is, named by the link Apache loads; without the Apache
// program nothing under /etc/apache2 is loaded. Fails on b6d3c832e (the
// whole tree counted, so every version read as in use).
func TestOnlyApacheFilesApacheLoadsKeepTheStockPoolOn(t *testing.T) {
	root := apacheTree(t)
	listen := "/run/php/php8.4-fpm.sock"
	files := apacheLoadedFiles(root, true)
	if got := stockPoolUsers("", listen, "", files, nil); len(got) != 0 {
		t.Fatalf("a snippet nothing loads counted: %v (loaded %v)", got, apacheNames(files))
	}
	if got := apacheLoadedFiles(root, false); got != nil {
		t.Fatalf("Apache not installed, still loaded: %v", apacheNames(got))
	}
	// The owner enables 8.4's snippet (a2enconf): a user, named by the link.
	link := filepath.Join(root, "conf-enabled", "php8.4-fpm.conf")
	if err := os.Symlink("../conf-available/php8.4-fpm.conf", link); err != nil {
		t.Fatal(err)
	}
	files = apacheLoadedFiles(root, true)
	if got := stockPoolUsers("", listen, "", files, nil); !reflect.DeepEqual(got, []string{link}) {
		t.Fatalf("enabled snippet: %v", got)
	}
	if got := stockPoolUsers("", "/run/php/php8.3-fpm.sock", "", files, nil); len(got) != 0 {
		t.Fatalf("8.3's snippet is not enabled: %v", got)
	}
	// A site file that includes an available file by name loads it.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	writeRound3File(t, filepath.Join(root, "sites-enabled", "shop.conf"), "<VirtualHost *:80>\n  Include "+filepath.Join(root, "conf-available", "php8.4-fpm.conf")+"\n</VirtualHost>\n")
	if got := stockPoolUsers("", listen, "", apacheLoadedFiles(root, true), nil); !reflect.DeepEqual(got, []string{filepath.Join(root, "conf-available", "php8.4-fpm.conf")}) {
		t.Fatalf("included by name: %v", got)
	}
	// An include that cannot be followed makes every file count, as before.
	writeRound3File(t, filepath.Join(root, "sites-enabled", "shop.conf"), "Include ${SHOP_DIR}/php.conf\n")
	got := stockPoolUsers("", listen, "", apacheLoadedFiles(root, true), nil)
	if !reflect.DeepEqual(got, []string{filepath.Join(root, "conf-available", "php8.4-fpm.conf")}) {
		t.Fatalf("an include with a variable: %v", got)
	}
}

// Finding 1 through the step: a server with Apache installed and only the
// package's snippet turns the stock pool off. Does not build on b6d3c832e.
func TestTheStockPoolIsTurnedOffBesideThePackagesApacheSnippet(t *testing.T) {
	host, path := newStockHost(t, true)
	root := apacheTree(t)
	ops := host.ops()
	ops.apacheFiles = func() []stockPoolConfigText { return apacheLoadedFiles(root, true) }
	if pools := closeStockFPMPoolsWith(context.Background(), ops); len(pools) != 1 || pools[0].State != transport.SideDoorsStockTurnedOff {
		t.Fatalf("pools = %+v", pools)
	}
	if _, err := os.Stat(path + stockPoolOffSuffix); err != nil {
		t.Fatal(err)
	}
}

// Finding 1, the reverse-proxy line: CelikPanel's own unchanged vhost with
// "proxy_pass $celikpanel_upstream;" is not a user; the same text changed on
// the server, or without the seal, is. The generator's variable is only ever
// "http(s)://host[:port]", and a socket path in a site's target is named in
// the text either way. Fails on b6d3c832e (every such site kept every
// version's pool on).
func TestThePanelsOwnUpstreamLineDoesNotKeepTheStockPoolOn(t *testing.T) {
	ng, err := services.NewNginxGenerator()
	if err != nil {
		t.Fatal(err)
	}
	ng.SetHostResolution(services.HostResolution{Nameservers: []string{"192.0.2.53"}})
	render := func(target string) (string, error) {
		return ng.Render(services.VhostData{SiteID: 3, Domain: "proxy.example", DocumentRoot: "/var/www/proxy.example",
			ACMEChallengeRoot: "/var/lib/celikpanel/acme", ProjectType: "proxy", ForwardTo: target})
	}
	body, err := render("http://backend.example:8080/base/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "        "+panelUpstreamPass+"\n") {
		t.Fatalf("the generator's form changed:\n%s", body)
	}
	const name = "/etc/nginx/sites-enabled/proxy.example.conf"
	listen := "/run/php/php8.4-fpm.sock"
	dump := func(text string) string {
		return "# configuration file /etc/nginx/nginx.conf:\nhttp {\n    include /etc/nginx/sites-enabled/*;\n}\n\n# configuration file " + name + ":\n" + text + "\n"
	}
	sealed := services.SealManagedText(body)
	if got := stockPoolUsers(dump(sealed), listen, "", nil, nil); len(got) != 0 {
		t.Fatalf("CelikPanel's unchanged file counted: %v", got)
	}
	// As the last file of the dump, and followed by another file.
	if got := stockPoolUsers(dump(sealed)+"# configuration file /etc/nginx/mime.types:\ntypes {}\n", listen, "", nil, nil); len(got) != 0 {
		t.Fatalf("followed by another file: %v", got)
	}
	changed := strings.Replace(sealed, "proxy_read_timeout 60s;", "proxy_read_timeout 61s;", 1)
	for label, text := range map[string]string{"changed": changed, "unsealed": body} {
		if got := stockPoolUsers(dump(text), listen, "", nil, nil); !reflect.DeepEqual(got, []string{name}) {
			t.Fatalf("%s: %v", label, got)
		}
	}
	// A target that names the socket is named whatever form it is written in.
	if text, err := render("http://unix:" + listen + ":/"); err == nil {
		if got := stockPoolUsers(dump(services.SealManagedText(text)), listen, "", nil, nil); !reflect.DeepEqual(got, []string{name}) {
			t.Fatalf("a target naming the socket: %v\n%s", got, text)
		}
	}
}

// Finding 4: other spellings of the socket's path, a link of the owner's own
// (also while the pool is not listening), a relative path and a missing
// folder keep the pool on; another socket does not. Fails on b6d3c832e.
func TestUnixPathsAreComparedByWhatTheyResolveTo(t *testing.T) {
	dir := t.TempDir()
	run := filepath.Join(dir, "run", "php")
	listen := filepath.Join(run, "php8.4-fpm.sock")
	writeRound3File(t, listen, "")
	writeRound3File(t, filepath.Join(run, "php8.4-fpm-site3.sock"), "")
	if err := os.MkdirAll(filepath.Join(dir, "srv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(listen, filepath.Join(dir, "srv", "php.sock")); err != nil {
		t.Fatal(err)
	}
	pass := func(path string) string {
		return "# configuration file /etc/nginx/sites-enabled/owner.conf:\nserver {\n    location ~ \\.php$ { fastcgi_pass unix:" + path + "; }\n}\n"
	}
	owner := []string{"/etc/nginx/sites-enabled/owner.conf"}
	for path, used := range map[string]bool{
		strings.Replace(listen, "/run/php/", "/run//php/", 1):       true,
		strings.Replace(listen, "/run/php/", "/run/php/./", 1):      true,
		strings.Replace(listen, "/run/php/", "/run/php/../php/", 1): true,
		filepath.Join(dir, "srv", "php.sock"):                       true,
		"run/php/php8.4-fpm.sock":                                   true,
		filepath.Join(dir, "missing", "php8.4-fpm.sock"):            true,
		filepath.Join(run, "php8.4-fpm-site3.sock"):                 false,
		filepath.Join(run, "php8.3-fpm.sock"):                       false,
		filepath.Join(dir, "srv", "x.sock"):                         false,
	} {
		got := stockPoolUsers(pass(path), listen, "", nil, hostSameSocket(listen))
		if used != (len(got) > 0) {
			t.Errorf("%s: users %v, want used=%v", path, got, used)
		}
		if used && !reflect.DeepEqual(got, owner) {
			t.Errorf("%s: named %v", path, got)
		}
	}
	// The pool is not listening: a link to its path still reaches it.
	if err := os.Remove(listen); err != nil {
		t.Fatal(err)
	}
	if got := stockPoolUsers(pass(filepath.Join(dir, "srv", "php.sock")), listen, "", nil, hostSameSocket(listen)); !reflect.DeepEqual(got, owner) {
		t.Fatalf("a link while the pool is not listening: %v", got)
	}
	// Apache's handler form is followed the same way.
	apache := []stockPoolConfigText{{Name: "/etc/apache2/conf-enabled/owner.conf", Text: "SetHandler \"proxy:unix:" + filepath.Join(dir, "srv", "php.sock") + "|fcgi://localhost\"\n"}}
	if got := stockPoolUsers("", listen, "", apache, hostSameSocket(listen)); !reflect.DeepEqual(got, []string{"/etc/apache2/conf-enabled/owner.conf"}) {
		t.Fatalf("Apache's handler through a link: %v", got)
	}
}

// Finding 3: a refused first turn-off renames the pool back while the
// "turning off" mark is still kept, and only then forgets the version; a
// crash in between is CelikPanel's unfinished step. Fails on b6d3c832e
// (the mark went first).
func TestThePutBackKeepsTheTurningMarkUntilThePoolIsBack(t *testing.T) {
	host, path, ops := recordedStockHost(t)
	host.testErr = errors.New("ERROR: No pool defined")
	rename := ops.rename
	markedAtPutBack := false
	ops.rename = func(from, to string) error {
		if to == path {
			turning, _ := services.StockPoolsTurningOff()
			markedAtPutBack = turning["8.4"]
		}
		return rename(from, to)
	}
	record := ops.record
	ops.record = func(version string, off bool) error {
		if _, err := os.Stat(path); err != nil && !off {
			t.Errorf("the record was written before %s was back: %v", path, err)
		}
		return record(version, off)
	}
	pools := closeStockFPMPoolsWith(context.Background(), ops)
	if len(pools) != 1 || pools[0].State != transport.SideDoorsStockFailed {
		t.Fatalf("%+v", pools)
	}
	if !markedAtPutBack {
		t.Fatal("the mark was gone before the pool was renamed back")
	}
	if turning, _ := services.StockPoolsTurningOff(); turning["8.4"] {
		t.Fatal("the mark stayed after the put-back finished")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("not put back")
	}
}

// Finding 3: a pool found renamed under CelikPanel's name without its record
// is checked before it is recorded off; a refusal puts it back. Fails on
// b6d3c832e (recorded off without the check).
func TestAPoolFoundRenamedWithoutTheRecordIsCheckedFirst(t *testing.T) {
	host, path, ops := recordedStockHost(t)
	if err := os.Rename(path, path+stockPoolOffSuffix); err != nil {
		t.Fatal(err)
	}
	host.testErr = errors.New("ERROR: No pool defined")
	pools := closeStockFPMPoolsWith(context.Background(), ops)
	if len(pools) != 1 || pools[0].State != transport.SideDoorsStockFailed || !strings.HasPrefix(strings.Join(host.steps, " "), "fpm-t:8.4") {
		t.Fatalf("%+v steps %v", pools, host.steps)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("a refused pool was left off")
	}
	if off, _ := services.StockPoolsTurnedOff(); off["8.4"] {
		t.Fatal("recorded off although the check refused")
	}
	// Checked and accepted: recorded off.
	host2, path2, ops2 := recordedStockHost(t)
	if err := os.Rename(path2, path2+stockPoolOffSuffix); err != nil {
		t.Fatal(err)
	}
	pools = closeStockFPMPoolsWith(context.Background(), ops2)
	if len(pools) != 1 || pools[0].State != transport.SideDoorsStockOff || strings.Join(host2.steps, " ") != "fpm-t:8.4 fpm-reload:8.4" {
		t.Fatalf("%+v steps %v", pools, host2.steps)
	}
	if off, _ := services.StockPoolsTurnedOff(); !off["8.4"] {
		t.Fatal("not recorded off after the check")
	}
}
