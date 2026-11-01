//go:build linux

package main

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeLMTP answers like Dovecot's LMTP and records every command it gets.
func fakeLMTP(t *testing.T, rcptReply string) (string, func() []string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "lmtp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	var mu sync.Mutex
	var commands []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
		write("220 mail.example.test Dovecot ready.")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			mu.Lock()
			commands = append(commands, line)
			mu.Unlock()
			switch verb := strings.ToUpper(strings.Fields(line)[0]); verb {
			case "LHLO":
				write("250-mail.example.test")
				write("250-8BITMIME")
				write("250 PIPELINING")
			case "MAIL", "RSET":
				write("250 2.1.0 OK")
			case "RCPT":
				write(rcptReply)
			case "QUIT":
				write("221 2.0.0 Bye")
				return
			default:
				write("500 5.5.1 Unexpected " + verb)
			}
		}
	}()
	return path, func() []string {
		<-done
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), commands...)
	}
}

func TestProbeLMTPRecipientAcceptsWithoutSendingAMessage(t *testing.T) {
	path, commands := fakeLMTP(t, "250 2.1.5 OK")
	if err := probeLMTPRecipient(context.Background(), path, "bob@shop114.test"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(commands(), "|")
	if got != "LHLO celikpanel-agent.localhost|MAIL FROM:<>|RCPT TO:<bob@shop114.test>|RSET|QUIT" {
		t.Fatalf("conversation: %s", got)
	}
}

// The exact answer Debian 13's Dovecot 2.4 gave in the lab (E4).
func TestProbeLMTPRecipientReportsTheRefusal(t *testing.T) {
	path, commands := fakeLMTP(t, "550 5.1.1 <bob@shop114.test> User doesn't exist: bob@shop114.test")
	err := probeLMTPRecipient(context.Background(), path, "bob@shop114.test")
	if err == nil || !strings.Contains(err.Error(), "550") || !strings.Contains(err.Error(), "User doesn't exist") {
		t.Fatalf("refusal: %v", err)
	}
	if strings.Contains(strings.Join(commands(), "|"), "DATA") {
		t.Fatal("a message was sent")
	}
}

func TestProbeLMTPRecipientNoListenerAndBadAddress(t *testing.T) {
	if err := probeLMTPRecipient(context.Background(), filepath.Join(t.TempDir(), "missing"), "bob@shop114.test"); err == nil {
		t.Fatal("a missing socket was accepted")
	}
	for _, bad := range []string{"", "a@b>\r\nDATA", "a b@c"} {
		if err := probeLMTPRecipient(context.Background(), "/nonexistent", bad); err == nil || !strings.Contains(err.Error(), "invalid recipient") {
			t.Fatalf("%q: %v", bad, err)
		}
	}
}
