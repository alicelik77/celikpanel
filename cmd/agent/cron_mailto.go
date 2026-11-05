package main

import "strings"

// Site cron output is not mailed (D-046, alpha.125). With mail without a
// password closed, cron can no longer hand a site job's output to sendmail as
// the site user (authorized_submit_users = root, vmail): every run with output
// would leave a refused-mail error in the system log. So a site crontab the
// Panel writes starts with MAILTO="" and cron mails nothing. A crontab that
// already sets MAILTO anywhere is the owner's choice and is left exactly as
// it is; root's crontab is never touched (root still sends mail).
//
// Site cron çıktısı postalanmaz: Panel'in yazdığı site crontab'ı MAILTO=""
// ile başlar. MAILTO'yu zaten ayarlayan crontab sahibinindir ve olduğu gibi
// kalır; root'un crontab'ına dokunulmaz.

const cronMailtoComment = "# CelikPanel: cron output of this site is not mailed (mail without a password is closed on this server)"

// cronSetsMailto reports whether any line assigns MAILTO (spaces around "="
// allowed, as cron allows them).
func cronSetsMailto(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		name, _, ok := strings.Cut(trimmed, "=")
		if ok && strings.TrimSpace(name) == "MAILTO" {
			return true
		}
	}
	return false
}

// withSiteCronMailto returns the crontab a Panel write stores for username.
func withSiteCronMailto(username, content string) string {
	if username == "root" || cronSetsMailto(content) {
		return content
	}
	return cronMailtoComment + "\nMAILTO=\"\"\n" + content
}
