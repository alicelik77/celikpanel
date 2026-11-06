package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The panel's own account mail (password resets, invitations, address
// notices, security and usage notices) after passwordless mail is closed
// (phase 2, alpha.125). Until then the panel handed each message to Postfix
// over SMTP on 127.0.0.1:25 without a password, which only worked because
// loopback was in permit_mynetworks for relay: the very path the owner closed
// for sites. The panel process runs with NoNewPrivileges and cannot run the
// setgid sendmail itself, so the Agent runs the distribution's own sendmail
// command as root, which authorized_submit_users keeps. Postfix then signs
// and filters it like any other local mail (non_smtpd_milters).
//
// Panelin kendi hesap postası: parolasız posta kapandıktan sonra panel iletiyi
// 127.0.0.1:25'e parolasız vermez; Agent dağıtımın sendmail komutunu root
// olarak çalıştırır (authorized_submit_users root'a izin verir).

const (
	accountMailSendTimeout = 30 * time.Second
	accountMailMaxBytes    = 512 * 1024
)

var accountMailSendmailPaths = []string{"/usr/sbin/sendmail", "/usr/bin/sendmail", "/usr/lib/sendmail"}

// runAccountSendmail is replaced by tests.
var runAccountSendmail = func(ctx context.Context, path string, args []string, message []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = bytes.NewReader(message)
	cmd.WaitDelay = 2 * time.Second
	return cmd.CombinedOutput()
}

func accountMailSendmailPath() string {
	for _, path := range accountMailSendmailPaths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// SendAccountMailV1 hands one panel account message to Postfix.
func (a *Agent) SendAccountMailV1(req *transport.SendAccountMailRequest, resp *transport.SendAccountMailResponse) error {
	*resp = transport.SendAccountMailResponse{}
	if req == nil {
		resp.Error = "account mail request is required"
		return nil
	}
	// One plain address each: no option, no list, no header injection.
	for _, address := range []string{req.From, req.To} {
		if _, err := transport.CanonicalMailAddress(address); err != nil || strings.HasPrefix(address, "-") {
			resp.Error = "account mail address is not valid"
			return nil
		}
	}
	if len(req.Message) == 0 || len(req.Message) > accountMailMaxBytes {
		resp.Error = "account mail message is empty or too large"
		return nil
	}
	path := accountMailSendmailPath()
	if path == "" {
		resp.Error = "no sendmail command is installed (Postfix provides /usr/sbin/sendmail)"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), accountMailSendTimeout)
	defer cancel()
	// -i: a line with a single dot does not end the message; -f: the envelope
	// sender; "--" ends the options before the one recipient.
	out, err := runAccountSendmail(ctx, path, []string{"-i", "-f", req.From, "--", req.To}, req.Message)
	if err != nil {
		resp.Error = fmt.Sprintf("Postfix's sendmail did not accept the message: %s", boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
		return nil
	}
	resp.Sent = true
	return nil
}
