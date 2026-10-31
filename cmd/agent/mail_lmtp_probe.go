package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"time"
)

// lmtpProbeTimeout bounds one whole LMTP conversation.
const lmtpProbeTimeout = 15 * time.Second

// probeLMTPRecipient asks the LMTP server on socketPath whether it accepts
// email as a recipient, the way Postfix's lmtp(8) client will: LHLO, MAIL
// FROM:<>, RCPT TO:<email>. No DATA is sent, so nothing is delivered; RSET
// and QUIT end the session. A nil error means "250 for this recipient"; any
// other answer is returned as the server's own reply line.
//
// probeLMTPRecipient, postanın LMTP'ye yönlendirilmesinden ÖNCE Dovecot'un
// adresi kabul edip etmediğini Postfix'in soracağı biçimde sorar (ileti
// gönderilmez).
func probeLMTPRecipient(ctx context.Context, socketPath, email string) error {
	if strings.ContainsAny(email, "\r\n<> ") || email == "" {
		return fmt.Errorf("invalid recipient for the LMTP check")
	}
	ctx, cancel := context.WithTimeout(ctx, lmtpProbeTimeout)
	defer cancel()
	var dialer net.Dialer
	raw, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect to Dovecot LMTP: %w", err)
	}
	defer raw.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = raw.SetDeadline(time.Now()) })
	defer stop()

	conn := textproto.NewConn(raw)
	step := func(expect int, command string) error {
		if command != "" {
			if err := conn.PrintfLine("%s", command); err != nil {
				return fmt.Errorf("LMTP %s: %w", strings.Fields(command)[0], err)
			}
		}
		code, message, err := conn.ReadResponse(expect)
		if err == nil {
			return nil
		}
		var protocolErr *textproto.Error
		if errors.As(err, &protocolErr) {
			return fmt.Errorf("%d %s", code, firstLine(message))
		}
		return fmt.Errorf("LMTP conversation: %w", err)
	}
	if err := step(220, ""); err != nil {
		return err
	}
	if err := step(250, "LHLO celikpanel-agent.localhost"); err != nil {
		return err
	}
	if err := step(250, "MAIL FROM:<>"); err != nil {
		return err
	}
	rcptErr := step(250, "RCPT TO:<"+email+">")
	// Best effort: the session ends without a message either way.
	_ = conn.PrintfLine("RSET")
	_, _, _ = conn.ReadResponse(250)
	_ = conn.PrintfLine("QUIT")
	return rcptErr
}
