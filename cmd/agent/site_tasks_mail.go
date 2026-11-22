package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Whether the SMTP block may let sites reach this server's own submission
// services (S4 R2b fix round, 22 Nov 2026; docs/SITE-SANDBOX.md section 3).
//
// The exception (ports 465 and 587 at this server's own addresses) is
// written into the block only while those ports are CelikPanel's own,
// unchanged submission entries that take nothing but a mailbox's login:
//
//   - master.cf has submission/inet and smtps/inet in the form
//     ConfigureMailSubmission writes (submissionEntryPanelForm), each with
//     smtpd_sasl_auth_enable=yes and
//     smtpd_client_restrictions=permit_sasl_authenticated,reject;
//   - no other master.cf inet entry listens on 465 or 587 (a "587", a
//     "127.0.0.1:submission", a "submissions" entry is the owner's);
//   - every socket listening on 465 or 587 belongs to Postfix's master
//     process (an msmtpd, an stunnel, another MTA there is the owner's).
//
// Otherwise the block keeps 465 and 587 closed to sites, and the screens say
// that sites' tasks and programs cannot send mail through this server, and
// why. Password guessing over this path is not limited by CelikPanel yet
// (a known limit for the reach slice, section 3).
//
// Site görevleri ve programları bu sunucunun posta gönderim hizmetine yalnız
// o hizmet CelikPanel'in kendi, parola isteyen girdisiyken ulaşabilir.

// siteSubmissionPostconf runs postconf with fixed read-only flags (a test
// seam).
var siteSubmissionPostconf = func(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "postconf", args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}
	return cmd.Output()
}

// siteSubmissionListeners lists the commands of the processes that listen
// on 465 or 587 (a test seam).
var siteSubmissionListeners = submissionListenerCommands

// siteSubmissionState decides MailSubmission* now (read-only).
func siteSubmissionState() (string, string) {
	listeners, listenErr := siteSubmissionListeners()
	if listenErr != nil {
		return transport.MailSubmissionOwner, "the listeners on 465 and 587 could not be read: " + listenErr.Error()
	}
	run := func(name string, args ...string) ([]byte, error) { return siteSubmissionPostconf(args...) }
	master, err := siteSubmissionPostconf("-M")
	if err != nil {
		if len(listeners) == 0 {
			return transport.MailSubmissionAbsent, "Postfix is not installed or its services cannot be read"
		}
		return transport.MailSubmissionOwner, "a program other than CelikPanel's Postfix listens on 465 or 587: " + strings.Join(listeners, ", ")
	}
	ours := map[string]bool{}
	for _, line := range strings.Split(string(master), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[1] != "inet" {
			continue
		}
		service := fields[0]
		port := service
		if i := strings.LastIndexByte(service, ':'); i >= 0 {
			port = service[i+1:]
		}
		switch submissionPortOf(port) {
		case 0:
			continue
		default:
			if service != "submission" && service != "smtps" {
				return transport.MailSubmissionOwner, "the server owner's master.cf entry " + service + " listens on a submission port"
			}
			ours[service] = true
		}
	}
	for _, l := range listeners {
		if l != "master" {
			return transport.MailSubmissionOwner, "a program other than Postfix listens on 465 or 587 (" + l + ")"
		}
	}
	if len(ours) == 0 {
		return transport.MailSubmissionAbsent, "no submission service is set up on this server"
	}
	chroot := distributionSubmissionChroot(run)
	params, err := siteSubmissionPostconf("-P")
	if err != nil {
		return transport.MailSubmissionOwner, "the submission services' settings could not be read"
	}
	for _, service := range submissionServices {
		fields, ok := masterServiceFields(run, service+"/inet")
		if !ok || !submissionEntryPanelForm(fields, legacySubmissionChroot, chroot) {
			return transport.MailSubmissionOwner, "the server owner changed (or made) the " + service + " entry in /etc/postfix/master.cf"
		}
		if postconfParam(string(params), service+"/inet/smtpd_client_restrictions") != "permit_sasl_authenticated,reject" ||
			postconfParam(string(params), service+"/inet/smtpd_sasl_auth_enable") != "yes" {
			return transport.MailSubmissionOwner, "the " + service + " entry does not take only a mailbox's login (smtpd_client_restrictions, smtpd_sasl_auth_enable)"
		}
	}
	return transport.MailSubmissionOurs, ""
}

// submissionPortOf maps a master.cf service port to 465, 587 or 0.
func submissionPortOf(port string) int {
	switch port {
	case "submission", "587":
		return 587
	case "smtps", "submissions", "urd", "ssmtp", "465":
		return 465
	}
	return 0
}

// postconfParam reads "name = value" from postconf's output.
func postconfParam(out, name string) string {
	for _, line := range strings.Split(out, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok && strings.TrimSpace(key) == name {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// submissionListenerCommands reads /proc/net/tcp and tcp6 for sockets
// listening on 465 or 587 and names the process (its comm) of each, from
// /proc/<pid>/fd. No program is started.
func submissionListenerCommands() ([]string, error) {
	inodes := map[string]bool{}
	for _, table := range []string{"net/tcp", "net/tcp6"} {
		f, err := os.Open(filepath.Join(procRoot, table))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 || fields[3] != "0A" { // LISTEN
				continue
			}
			_, portHex, ok := strings.Cut(fields[1], ":")
			if !ok {
				continue
			}
			port, err := strconv.ParseUint(portHex, 16, 16)
			if err != nil || (port != 465 && port != 587) {
				continue
			}
			inodes[fields[9]] = true
		}
		f.Close()
	}
	if len(inodes) == 0 {
		return nil, nil
	}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	found := 0
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		fds, err := os.ReadDir(filepath.Join(procRoot, e.Name(), "fd"))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(procRoot, e.Name(), "fd", fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			if !inodes[inode] {
				continue
			}
			found++
			comm, _ := os.ReadFile(filepath.Join(procRoot, e.Name(), "comm"))
			name := strings.TrimSpace(string(comm))
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	if found == 0 {
		out = append(out, "unknown")
	}
	return out, nil
}

// siteSubmissionRecordPath keeps what the Agent last decided (and wrote into
// the SMTP block), so the screens can say it without asking Postfix again.
var siteSubmissionRecordPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "mail-submission.json")

type siteSubmissionRecord struct {
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	At     string `json:"at"`
}

func recordMailSubmission(state, detail string) {
	raw, err := json.Marshal(siteSubmissionRecord{State: state, Detail: detail, At: time.Now().UTC().Format(time.RFC3339)})
	if err == nil {
		err = writePrivateFile(siteSubmissionRecordPath, raw)
	}
	if err != nil {
		log.Printf("site sandbox: record the mail submission state: %v", err)
	}
}

// recordedMailSubmission is the state the SMTP block was last written with
// ("" when it never was).
func recordedMailSubmission() string {
	raw, err := os.ReadFile(siteSubmissionRecordPath)
	if err != nil {
		return ""
	}
	var r siteSubmissionRecord
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	return r.State
}

// refreshSandboxEgressIfPresent writes the SMTP block again where it is in
// use (its file exists): the submission exception follows the mail
// services' state (each Panel start, and after the mail submission step).
func refreshSandboxEgressIfPresent(ctx context.Context) {
	if _, err := os.Stat(filepath.Join(sandboxEgressDir, filepath.Base(sitesandbox.EgressFile))); err != nil {
		return
	}
	if err := ensureSandboxEgress(sandboxCaps(ctx)); err != nil {
		log.Printf("site sandbox: the SMTP block could not be brought up to date: %v", err)
	}
}
