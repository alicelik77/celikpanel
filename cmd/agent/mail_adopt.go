package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A mailbox found on the server without a record in the Panel (alpha.144,
// alpha.143 lab): a creation the Panel could not finish recording left the
// mailbox in Dovecot's users file and Postfix's mailbox map. The
// administrator may adopt it into its domain; the Panel first reads here
// what the server holds for the address. This changes nothing: no file, no
// folder, no mail.
//
// Panelde kaydı olmadan sunucuda bulunan posta kutusu: Panel, yöneticinin
// sahiplenmesinden önce sunucunun bu adres için ne tuttuğunu burada okur.
// Hiçbir şey değiştirilmez.

// dovecotQuotaRule is the size limit in the form dovecotQuotaFields writes.
var dovecotQuotaRule = regexp.MustCompile(`(?:^|\s)userdb_quota_rule=\*:storage=([0-9]+)M(?:\s|$)`)

// MailboxOnServerV1 answers, read only, what the server's mail files hold
// for one address.
func (a *Agent) MailboxOnServerV1(req *transport.MailboxOnServerRequest, resp *transport.MailboxOnServerResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("mailbox reading request and response are required")
	}
	*resp = transport.MailboxOnServerResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "reading a mailbox on the server"); err != nil {
		return err
	}
	email, local, domain, err := canonicalAgentMailbox(req.Email)
	if err != nil {
		return err
	}
	mailMutex.Lock()
	defer mailMutex.Unlock()
	users, err := readMailFile(dovecotUsersPath)
	if err != nil {
		return err
	}
	vboxes, err := readMailFile(postfixVBoxPath)
	if err != nil {
		return err
	}
	*resp = mailboxOnServer(users, vboxes, email, local, domain)
	if root, err := managedMailRootPath(); err != nil {
		resp.MessagesUnknown = true
	} else if count, err := mailboxMessageCount(root, domain, local); err != nil {
		resp.MessagesUnknown = true
	} else {
		resp.Messages = count
	}
	return nil
}

// mailboxMessageCount counts the messages in <root>/<domain>/<local>: the
// regular files in any cur/ or new/ folder under it (Maildir and its
// subfolders). No folder is 0. A link anywhere on the way or inside, or an
// entry that cannot be read, is an error: the count is not known.
func mailboxMessageCount(root, domain, local string) (int, error) {
	for _, path := range []string{filepath.Join(root, domain), filepath.Join(root, domain, local)} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		if !info.IsDir() {
			return 0, fmt.Errorf("%s is not a folder", path)
		}
	}
	count := 0
	err := filepath.WalkDir(filepath.Join(root, domain, local), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a link", path)
		}
		if entry.Type().IsRegular() {
			if folder := filepath.Base(filepath.Dir(path)); folder == "cur" || folder == "new" {
				count++
			}
		}
		return nil
	})
	return count, err
}

// mailboxOnServer reads one address's lines. OwnFolder only when the map
// delivers to <domain>/<local>/ (the folder CelikPanel gives the address)
// and the users line is CelikPanel's form: no uid, gid, comment, home or
// shell of its own, so Dovecot's own default places its mail in that folder.
func mailboxOnServer(users, vboxes []byte, email, local, domain string) transport.MailboxOnServerResponse {
	var out transport.MailboxOnServerResponse
	mapOwn, usersOwn := false, false
	for _, line := range splitMailFile(vboxes) {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != email {
			continue
		}
		if out.InMap {
			// Two lines for one address: Postfix uses one of them; which
			// folder its mail reaches is not certain.
			mapOwn = false
			continue
		}
		out.InMap = true
		mapOwn = len(fields) == 2 && fields[1] == domain+"/"+local+"/"
	}
	for _, line := range splitMailFile(users) {
		if !strings.HasPrefix(line, email+":") {
			continue
		}
		if out.InUsers {
			usersOwn = false
			continue
		}
		out.InUsers = true
		parts := strings.SplitN(line, ":", 8)
		usersOwn = len(parts) == 8 && parts[1] != ""
		for _, field := range parts[2:min(len(parts), 7)] {
			if field != "" {
				usersOwn = false
			}
		}
		if len(parts) == 8 {
			if match := dovecotQuotaRule.FindStringSubmatch(parts[7]); match != nil {
				if quota, err := strconv.Atoi(match[1]); err == nil && validateAgentQuota(quota) == nil {
					out.QuotaMB = quota
				}
			}
		}
	}
	out.OwnFolder = mapOwn && usersOwn
	return out
}
