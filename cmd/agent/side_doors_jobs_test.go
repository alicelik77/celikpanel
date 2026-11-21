package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Existing installs (S4 R1): CelikPanel kept no record of a site's tasks, so
// the first step takes each crontab as it is and accounts for the lines its
// own writer can make; anything else is listed. After that first step a
// crontab of an account CelikPanel never wrote is listed whole.
func TestFirstStepTakesExistingCrontabsAsTheBaseline(t *testing.T) {
	panelShaped := "# CelikPanel: cron output of this site is not mailed (mail without a password is closed on this server)\nMAILTO=\"\"\n# nightly\n0 3 * * * /usr/bin/php /var/www/a/cron.php\n# DISABLED: */5 * * * * /usr/bin/true\n"
	fake := &fakeSiteCrontabs{content: map[string]string{
		"shop_example": panelShaped,
		"blog_example": panelShaped + "@reboot /tmp/run.sh\nPATH=/tmp:/usr/bin\n15 2 * * mon /usr/local/bin/report\n",
		"wiki_example": "* * * * * /tmp/x.sh\n",
	}}
	installFakeSiteCrontabs(t, fake)
	oldAccounts := siteCrontabAccounts
	t.Cleanup(func() { siteCrontabAccounts = oldAccounts })
	accounts := []passwdEntry{
		{Name: "blog_example", UID: 1002, Home: "/var/www/celikpanel/subscriptions/1/sites/12"},
		{Name: "shop_example", UID: 1001, Home: "/var/www/celikpanel/subscriptions/1/sites/11"},
	}
	siteCrontabAccounts = func() ([]passwdEntry, error) { return accounts, nil }

	rows := checkSiteCrontabs()
	if len(rows) != 1 || rows[0].Account != "blog_example" || rows[0].Outside != 3 || rows[0].SubscriptionID != 1 || rows[0].DomainID != 12 {
		t.Fatalf("first step rows %+v: want blog_example with its @reboot, PATH and named-weekday lines", rows)
	}
	if !siteCrontabBaselineTaken() {
		t.Fatal("the baseline is taken once every crontab was read")
	}
	// A line planted after the baseline, in a crontab that has a record.
	fake.content["shop_example"] += "*/2 * * * * /tmp/planted.sh\n"
	// An account CelikPanel never wrote, appearing later.
	accounts = append(accounts, passwdEntry{Name: "wiki_example", UID: 1003, Home: "/var/www/celikpanel/subscriptions/2/sites/20"})
	rows = checkSiteCrontabs()
	got := map[string]int{}
	for _, row := range rows {
		got[row.Account] = row.Outside
	}
	if got["shop_example"] != 1 || got["wiki_example"] != 1 || got["blog_example"] != 3 {
		t.Fatalf("second step %v", got)
	}
	// The record never takes in a line it did not account for, even when
	// CelikPanel rewrites the crontab.
	before := fake.content["blog_example"]
	if err := setSiteCrontab("blog_example", before, before+"0 1 * * * /usr/bin/true\n", []string{"0 1 * * * /usr/bin/true"}); err != nil {
		t.Fatal(err)
	}
	if outside := cronOutsideLines("blog_example", fake.content["blog_example"]); len(outside) != 3 {
		t.Fatalf("after a panel write: %q", outside)
	}
	// A site that is deleted takes its record with it.
	removeSiteCrontabRecord("blog_example")
	if _, err := os.Stat(siteCrontabRecordPath("blog_example")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("record left: %v", err)
	}
}

// A crontab that cannot be read leaves the baseline for the next start.
func TestBaselineWaitsForEveryCrontab(t *testing.T) {
	fake := &fakeSiteCrontabs{content: map[string]string{"shop_example": "0 3 * * * /bin/true\n"}, broken: map[string]bool{"blog_example": true}}
	installFakeSiteCrontabs(t, fake)
	oldAccounts := siteCrontabAccounts
	t.Cleanup(func() { siteCrontabAccounts = oldAccounts })
	siteCrontabAccounts = func() ([]passwdEntry, error) {
		return []passwdEntry{
			{Name: "blog_example", UID: 1002, Home: "/var/www/celikpanel/subscriptions/1/sites/12"},
			{Name: "shop_example", UID: 1001, Home: "/var/www/celikpanel/subscriptions/1/sites/11"},
		}, nil
	}
	if rows := checkSiteCrontabs(); len(rows) != 0 {
		t.Fatalf("rows %+v", rows)
	}
	if siteCrontabBaselineTaken() {
		t.Fatal("the baseline was taken although a crontab could not be read")
	}
	if _, found := readSiteCrontabRecord("shop_example"); !found {
		t.Fatal("the readable crontab's baseline is kept")
	}
}

const testAtDeny = "alias\nbackup\nbin\ndaemon\nftp\ngames\ngnats\nguest\nirc\nlp\nmail\nman\nnobody\noperator\nproxy\nsync\nsys\nwww-data\nbob\n"

const testPasswd = `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
www-data:x:33:33:www-data:/var/www:/usr/sbin/nologin
nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin
backupsvc:x:998:998::/var/lib/backupsvc:/usr/sbin/nologin
alice:x:1000:1000:Alice:/home/alice:/bin/bash
bob:x:1004:1004::/home/bob:/bin/bash
shop_example_com:x:1001:1001::/var/www/celikpanel/subscriptions/3/sites/7:/bin/sh
shop_ftp:x:1005:1001::/var/www/celikpanel/subscriptions/3/sites/7/public_html:/usr/sbin/nologin
celikpanel-webmail:x:997:997::/nonexistent:/usr/sbin/nologin
`

type fakeAt struct {
	dir     string
	writes  []string
	atqOut  string
	scripts map[string]string
}

func (f *fakeAt) ops(installed bool) siteAtOps {
	return siteAtOps{
		installed: func() bool { return installed },
		readFile:  os.ReadFile,
		writeLike: func(path, like string, data []byte) error {
			f.writes = append(f.writes, path+"<-"+like)
			return os.WriteFile(path, data, 0o640)
		},
		atq: func(context.Context) (string, error) { return f.atqOut, nil },
		atJob: func(_ context.Context, id string) (string, error) {
			if script, ok := f.scripts[id]; ok {
				return script, nil
			}
			return "", errors.New("no such job")
		},
	}
}

func withAtFiles(t *testing.T, deny string, nss string) *fakeAt {
	t.Helper()
	dir := t.TempDir()
	saved := []string{atAllowPath, atDenyPath, sideDoorsPasswd, sideDoorsNSSwitch, atAllowRecordPath}
	t.Cleanup(func() {
		atAllowPath, atDenyPath, sideDoorsPasswd, sideDoorsNSSwitch, atAllowRecordPath = saved[0], saved[1], saved[2], saved[3], saved[4]
	})
	atAllowPath = filepath.Join(dir, "at.allow")
	atDenyPath = filepath.Join(dir, "at.deny")
	sideDoorsPasswd = filepath.Join(dir, "passwd")
	sideDoorsNSSwitch = filepath.Join(dir, "nsswitch.conf")
	atAllowRecordPath = filepath.Join(dir, "at-allow.sha256")
	if err := os.WriteFile(sideDoorsPasswd, []byte(testPasswd), 0o644); err != nil {
		t.Fatal(err)
	}
	if deny != "" {
		if err := os.WriteFile(atDenyPath, []byte(deny), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if nss != "" {
		if err := os.WriteFile(sideDoorsNSSwitch, []byte(nss), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &fakeAt{dir: dir}
}

const atJobScript = `#!/bin/sh
# atrun uid=1001 gid=1001
# mail shop_example_com 0
umask 22
PATH=/usr/bin:/bin; export PATH
cd /var/www/celikpanel/subscriptions/3/sites/7 || {
	 echo 'Execution directory inaccessible' >&2
	 exit 1
}
${SHELL:-/bin/sh} << 'marcinDELIMITER2b1f6c3e'
/usr/bin/php /var/www/celikpanel/subscriptions/3/sites/7/public_html/x.php
marcinDELIMITER2b1f6c3e
`

// at closed to site accounts: whoever may use at today keeps it, the website
// accounts (homes under the hosting root, the tools' own accounts) lose it;
// root always may. The file has at.deny's owner and mode.
func TestAtAllowKeepsTodaysRuleMinusWebsiteAccounts(t *testing.T) {
	fake := withAtFiles(t, testAtDeny, "passwd: files systemd\ngroup: files systemd\n")
	fake.atqOut = "3\tSat Nov 22 10:00:00 2026 a shop_example_com\n4\tSat Nov 22 11:00:00 2026 a alice\n"
	fake.scripts = map[string]string{"3": atJobScript}
	at := closeSiteAtWith(context.Background(), fake.ops(true))
	if at.State != transport.SideDoorsAtWritten {
		t.Fatalf("state %+v", at)
	}
	data, _ := os.ReadFile(atAllowPath)
	if !strings.HasPrefix(string(data), atAllowMark+"\n") || !strings.Contains(string(data), "To undo: delete this file") {
		t.Fatalf("header missing:\n%s", data)
	}
	if names := strings.Join(nameLines(string(data)), ","); names != "root,backupsvc,alice" {
		t.Fatalf("at.allow names %q: want root, backupsvc and alice (not denied, not a website account)", names)
	}
	if len(fake.writes) != 1 || fake.writes[0] != atAllowPath+"<-"+atDenyPath {
		t.Fatalf("writes %v: want at.allow written with at.deny's owner and mode", fake.writes)
	}
	if len(at.Jobs) != 1 || at.Jobs[0].Account != "shop_example_com" || at.Jobs[0].ID != "3" || at.Jobs[0].When != "Sat Nov 22 10:00:00 2026" ||
		at.Jobs[0].Command != "/usr/bin/php /var/www/celikpanel/subscriptions/3/sites/7/public_html/x.php" {
		t.Fatalf("jobs %+v: want the site account's job with what, whose and when", at.Jobs)
	}

	// The next start: CelikPanel's own file is current.
	if again := closeSiteAtWith(context.Background(), fake.ops(true)); again.State != transport.SideDoorsAtCurrent {
		t.Fatalf("second start %+v", again)
	}
	// An account made later may use at, as before.
	if err := os.WriteFile(sideDoorsPasswd, []byte(testPasswd+"carol:x:1006:1006::/home/carol:/bin/bash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if updated := closeSiteAtWith(context.Background(), fake.ops(true)); updated.State != transport.SideDoorsAtWritten {
		t.Fatalf("new account %+v", updated)
	}
	data, _ = os.ReadFile(atAllowPath)
	if !strings.HasSuffix(string(data), "alice\ncarol\n") {
		t.Fatalf("carol not added:\n%s", data)
	}
	// Changed by the owner: it is the owner's; the site account it names is
	// listed, nothing is written.
	if err := os.WriteFile(atAllowPath, append(data, []byte("shop_example_com\n")...), 0o640); err != nil {
		t.Fatal(err)
	}
	writes := len(fake.writes)
	owner := closeSiteAtWith(context.Background(), fake.ops(true))
	if owner.State != transport.SideDoorsAtOwnerFile || strings.Join(owner.SiteAccounts, ",") != "shop_example_com" || len(fake.writes) != writes {
		t.Fatalf("owner's file %+v, writes %v", owner, fake.writes)
	}
}

func TestAtAllowIsNotWrittenWhereItWouldChangeSomethingElse(t *testing.T) {
	t.Run("at not installed", func(t *testing.T) {
		fake := withAtFiles(t, testAtDeny, "")
		if at := closeSiteAtWith(context.Background(), fake.ops(false)); at.State != transport.SideDoorsAtNotInstalled || len(fake.writes) != 0 {
			t.Fatalf("%+v %v", at, fake.writes)
		}
	})
	t.Run("neither file: only root may use at already", func(t *testing.T) {
		fake := withAtFiles(t, "", "")
		if at := closeSiteAtWith(context.Background(), fake.ops(true)); at.State != transport.SideDoorsAtOnlyRoot || len(fake.writes) != 0 {
			t.Fatalf("%+v %v", at, fake.writes)
		}
	})
	t.Run("accounts from a directory service", func(t *testing.T) {
		fake := withAtFiles(t, testAtDeny, "passwd:         files ldap\n")
		if at := closeSiteAtWith(context.Background(), fake.ops(true)); at.State != transport.SideDoorsAtDirectory || len(fake.writes) != 0 {
			t.Fatalf("%+v %v", at, fake.writes)
		}
	})
	t.Run("an owner's at.allow is kept", func(t *testing.T) {
		fake := withAtFiles(t, testAtDeny, "")
		if err := os.WriteFile(atAllowPath, []byte("alice\n"), 0o640); err != nil {
			t.Fatal(err)
		}
		at := closeSiteAtWith(context.Background(), fake.ops(true))
		data, _ := os.ReadFile(atAllowPath)
		if at.State != transport.SideDoorsAtOwnerFile || len(at.SiteAccounts) != 0 || string(data) != "alice\n" || len(fake.writes) != 0 {
			t.Fatalf("%+v %q %v", at, data, fake.writes)
		}
	})
}
