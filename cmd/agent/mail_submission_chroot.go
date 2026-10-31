package main

import (
	"fmt"
	"strings"
)

// Submission (587) and smtps (465) chroot — D3, final alpha.113 acceptance
// run 2 (30 Oct 2026). Releases up to alpha.113 wrote both entries with
// chroot "y" on every distribution. Debian and Ubuntu chroot their own smtp
// service too and their postfix unit copies hosts/resolv.conf/services into
// /var/spool/postfix/etc; upstream Postfix and Arch ship `smtp inet n - n`
// and nothing copies those files, so on Arch the chrooted submission smtpd
// could not resolve "localhost" for its milters and every message a user sent
// left unsigned and unfiltered (milter_default_action=accept). The entries
// now take the chroot the distribution gives its own smtp service, read from
// master.cf, and the milters are named by numeric address (dkim_signing_rpc.go)
// so neither depends on files inside a chroot.
//
// Submission (587) ve smtps (465) chroot'u — D3. alpha.113'e kadarki
// sürümler iki girdiyi de her dağıtımda chroot "y" ile yazıyordu. Debian ve
// Ubuntu kendi smtp hizmetini de chroot'lar ve chroot'a hosts/resolv.conf
// kopyalar; upstream Postfix ve Arch `smtp inet n - n` getirir ve hiçbir şey
// bu dosyaları kopyalamaz. Girdiler artık dağıtımın kendi smtp hizmetine
// verdiği chroot'u master.cf'ten okuyup izler.

// postconfRunner runs one postconf command and returns its combined output.
// postconfRunner tek bir postconf komutunu çalıştırır.
type postconfRunner func(name string, args ...string) ([]byte, error)

// legacySubmissionChroot is the chroot field releases up to alpha.113 wrote.
// legacySubmissionChroot, alpha.113'e kadarki sürümlerin yazdığı alandır.
const legacySubmissionChroot = "y"

// defaultSubmissionChroot is used when master.cf has no readable smtp/inet
// entry: Postfix's own default since 3.0, and safe because no milter name
// needs a resolver inside a chroot any more.
// defaultSubmissionChroot, master.cf'te okunur smtp/inet girdisi yoksa
// kullanılır: Postfix 3.0'dan beri kendi varsayılanı.
const defaultSubmissionChroot = "n"

// masterServiceFields reads one master.cf entry ("submission/inet") through
// `postconf -F`, which prints "name/type/field = value" per field. Lines that
// are postconf's own messages are ignored; an entry that does not exist
// yields ok=false.
// masterServiceFields, bir master.cf girdisini `postconf -F` ile okur.
func masterServiceFields(run postconfRunner, entry string) (map[string]string, bool) {
	out, err := run("postconf", "-F", entry)
	if err != nil {
		return nil, false
	}
	return parseMasterServiceFields(string(out), entry)
}

func parseMasterServiceFields(out, entry string) (map[string]string, bool) {
	fields := map[string]string{}
	prefix := entry + "/"
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		name, value, found := strings.Cut(strings.TrimPrefix(line, prefix), "=")
		if !found {
			continue
		}
		fields[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	if fields["chroot"] == "" || fields["command"] == "" {
		return nil, false
	}
	return fields, true
}

// distributionSubmissionChroot is the chroot field of the distribution's own
// smtp/inet service ("y" on Debian/Ubuntu, "n" on Arch and upstream).
// distributionSubmissionChroot, dağıtımın kendi smtp/inet hizmetinin chroot
// alanıdır.
func distributionSubmissionChroot(run postconfRunner) string {
	if smtp, ok := masterServiceFields(run, "smtp/inet"); ok {
		switch smtp["chroot"] {
		case "y", "n", "-":
			return smtp["chroot"]
		}
	}
	return defaultSubmissionChroot
}

// submissionEntryPanelForm reports whether an existing submission/smtps entry
// is one CelikPanel writes — `<name> inet n - <chroot> - - smtpd` with chroot
// one of the given values — rather than an entry the server owner changed
// (another command, process limit, privacy or chroot), which is kept.
// submissionEntryPanelForm, var olan girdinin CelikPanel'in yazdığı biçimde
// olup olmadığını söyler; sahibin değiştirdiği girdi korunur.
func submissionEntryPanelForm(fields map[string]string, chroots ...string) bool {
	if fields["private"] != "n" || fields["unprivileged"] != "-" || fields["wakeup"] != "-" ||
		fields["process_limit"] != "-" {
		return false
	}
	if command := strings.Fields(fields["command"]); len(command) == 0 || command[0] != "smtpd" {
		return false
	}
	for _, chroot := range chroots {
		if fields["chroot"] == chroot {
			return true
		}
	}
	return false
}

// submissionServices are the two entries ConfigureMailSubmission owns.
var submissionServices = []string{"submission", "smtps"}

// reconcileSubmissionChroot is the upgrade repair run by Agent.WireMailFilters
// at every Panel start: an entry still exactly as an earlier release wrote it
// (chroot "y") on a distribution whose smtp service is not chrooted is moved
// to the distribution's setting with `postconf -F`. Entries the owner changed
// are kept and named in the note. It returns a note, never an error: the
// caller's verified Postfix reload applies the change.
// reconcileSubmissionChroot, her Panel başlangıcında Agent.WireMailFilters'in
// çalıştırdığı yükseltme onarımıdır; sahibin değiştirdiği girdiler korunur ve
// notta adlandırılır.
func reconcileSubmissionChroot(run postconfRunner) string {
	want := distributionSubmissionChroot(run)
	if want == legacySubmissionChroot {
		return ""
	}
	var changed, kept, failed []string
	for _, service := range submissionServices {
		entry := service + "/inet"
		fields, ok := masterServiceFields(run, entry)
		if !ok || fields["chroot"] != legacySubmissionChroot {
			continue
		}
		if !submissionEntryPanelForm(fields, legacySubmissionChroot) {
			kept = append(kept, entry)
			continue
		}
		if out, err := run("postconf", "-F", entry+"/chroot="+want); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%s)", entry, firstLine(strings.TrimSpace(string(out)))))
			continue
		}
		changed = append(changed, entry)
	}
	var notes []string
	if len(changed) > 0 {
		notes = append(notes, fmt.Sprintf("%s now run with chroot %q like this distribution's smtp service, so their mail filters are reached without files inside the Postfix chroot",
			strings.Join(changed, " and "), want))
	}
	if len(kept) > 0 {
		notes = append(notes, fmt.Sprintf("%s in /etc/postfix/master.cf were changed by the server owner and kept as written (chrooted); CelikPanel's mail filters are named by numeric address and stay reachable there, and nothing else is needed unless the owner wants this distribution's setting (postconf -F %s/chroot=%s, then postfix reload)",
			strings.Join(kept, " and "), kept[0], want))
	}
	if len(failed) > 0 {
		notes = append(notes, fmt.Sprintf("the chroot of %s could not be changed to %q; mail sent on 587/465 may stay unsigned until the server owner runs postconf -F <entry>/chroot=%s and reloads Postfix, or the panel is restarted to retry",
			strings.Join(failed, " and "), want, want))
	}
	return joinNotes(notes...)
}

// joinNotes joins the non-empty notes of one operation with "; ".
// joinNotes, bir işlemin boş olmayan notlarını "; " ile birleştirir.
func joinNotes(notes ...string) string {
	var kept []string
	for _, note := range notes {
		if note = strings.TrimSpace(note); note != "" {
			kept = append(kept, note)
		}
	}
	return strings.Join(kept, "; ")
}
