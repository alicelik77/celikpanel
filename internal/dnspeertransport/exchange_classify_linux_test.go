//go:build linux

package dnspeertransport

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// badSignatureSigner presents the pinned public key but signs with another
// key: the host key signature does not verify.
type badSignatureSigner struct {
	pinned ssh.PublicKey
	other  ssh.Signer
}

func (s badSignatureSigner) PublicKey() ssh.PublicKey { return s.pinned }
func (s badSignatureSigner) Sign(r io.Reader, data []byte) (*ssh.Signature, error) {
	return s.other.Sign(r, data)
}

// lineServer answers on a loopback port with fixed bytes and closes: a
// service that is not SSH.
func lineServer(t *testing.T, payload string) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, _ string) (net.Conn, error) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		go func() {
			defer listener.Close()
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_, _ = io.WriteString(conn, payload)
			time.Sleep(50 * time.Millisecond)
			conn.Close()
		}()
		return (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
	}
}

// exchangeServer serves one SSH connection whose session behaves as mode
// says: "reject" refuses the session channel, "exit1" runs the command and
// exits 1 with a stderr line, "drop" closes the connection once the command
// was asked for, "ok" answers and exits 0.
type exchangeServer struct {
	probeServer
	mode string
}

func (s *exchangeServer) dial(ctx context.Context, _ string) (net.Conn, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	config := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if s.authorized != nil && bytes.Equal(key.Marshal(), s.authorized.Marshal()) {
			return &ssh.Permissions{}, nil
		}
		return nil, errors.New("refused")
	}}
	config.AddHostKey(s.hostSigner)
	go func() {
		defer listener.Close()
		server, err := listener.Accept()
		if err != nil {
			return
		}
		conn, chans, reqs, err := ssh.NewServerConn(server, config)
		if err != nil {
			server.Close()
			return
		}
		defer conn.Close()
		go ssh.DiscardRequests(reqs)
		for newChannel := range chans {
			if s.mode == "reject" {
				_ = newChannel.Reject(ssh.Prohibited, "test")
				continue
			}
			channel, requests, err := newChannel.Accept()
			if err != nil {
				return
			}
			for req := range requests {
				if req.Type != "exec" {
					_ = req.Reply(false, nil)
					continue
				}
				_ = req.Reply(true, nil)
				switch s.mode {
				case "drop":
					conn.Close()
					return
				case "exit1":
					// The request first, as the inspector reads it.
					_, _ = io.Copy(io.Discard, channel)
					_, _ = io.WriteString(channel.Stderr(), "not a reviewed line\n")
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
				default:
					_, _ = io.Copy(io.Discard, channel)
					_, _ = io.WriteString(channel, "answer\n")
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				}
				channel.Close()
				// Let the client read the result and close first.
				_ = conn.Wait()
				return
			}
		}
	}()
	return (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
}

func exchangeCode(err error) Code {
	var unknown Unknown
	if errors.As(err, &unknown) {
		return unknown.Code
	}
	var failed commandFailure
	if errors.As(err, &failed) {
		return "command_failed"
	}
	if err == nil {
		return "answered"
	}
	return "unclassified"
}

// alpha.133 round 5 (lab w9): every way the backup can fail to answer is
// "no answer", whatever error value the dial or the SSH layer returns and at
// whichever instant of the bound it comes back; every way it answers with
// something this server cannot accept stays "unavailable" (or a command
// failure), even at the bound.
func TestExchangeTellsNoAnswerFromAnAnswerWeCannotAccept(t *testing.T) {
	keyPath, clientKey := probeKeyFile(t)
	host, digest := probeHostSigner(t)
	enrollment := Enrollment{PeerIP: "192.0.2.11", Username: "celikpeer", HostKeySHA256: digest, PrivateKeyPath: keyPath, Timeout: 10 * time.Second}
	request := []byte("request")
	run := func(t *testing.T, timeout time.Duration, dial func(context.Context, string) (net.Conn, error)) (Code, time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		started := time.Now()
		answer, _, err := exchangeFixedAt(ctx, enrollment, request, FixedCommand, dial)
		if err == nil && string(answer) != "answer\n" {
			t.Fatalf("answer %q", answer)
		}
		return exchangeCode(err), time.Since(started)
	}

	// A closed port on this host: a real "connection refused", at once.
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddress := closed.Addr().String()
	closed.Close()
	refused := func(ctx context.Context, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", closedAddress)
	}
	// The dialer's own deadline for the step's instant returning a moment
	// before the step's context timer fired (the racing shape of w9): ctx.Err()
	// is still nil when the error comes back.
	racing := func(ctx context.Context, _ string) (net.Conn, error) {
		deadline, _ := ctx.Deadline()
		time.Sleep(time.Until(deadline) - 20*time.Millisecond)
		if ctx.Err() != nil {
			return nil, errors.New("test: context ended before the racing return")
		}
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}
	}
	// The context's timer fired first (the other eight of nine).
	expired := func(ctx context.Context, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: ctx.Err()}
	}
	noRoute := func(context.Context, string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}
	}
	// A port that accepts and never says anything (a stalled sshd).
	silent := func(ctx context.Context, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() { <-ctx.Done(); time.Sleep(50 * time.Millisecond); server.Close() }()
		return client, nil
	}
	// A port that closes before any host key (a restarting sshd, a reset).
	reset := func(context.Context, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() { server.Close() }()
		return client, nil
	}
	for _, tc := range []struct {
		name    string
		timeout time.Duration
		dial    func(context.Context, string) (net.Conn, error)
	}{
		{"connection refused", 3 * time.Second, refused},
		{"dial timer before the context timer", 300 * time.Millisecond, racing},
		{"context timer first", 300 * time.Millisecond, expired},
		{"no route", 3 * time.Second, noRoute},
		{"silent port until the bound", 300 * time.Millisecond, silent},
		{"closed before any host key", 3 * time.Second, reset},
		{"connection lost after the command was asked for", 3 * time.Second, (&exchangeServer{probeServer{hostSigner: host, authorized: clientKey}, "drop"}).dial},
	} {
		t.Run("no answer: "+tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.timeout, tc.dial); got != CodeNoAnswer {
				t.Fatalf("got %s, want %s", got, CodeNoAnswer)
			}
		})
	}
	other, _ := probeHostSigner(t)
	_, otherPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherSigner, err := ssh.NewSignerFromKey(otherPrivate)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaSigner, err := ssh.NewSignerFromKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		dial func(context.Context, string) (net.Conn, error)
		want Code
	}{
		{"another host key", (&exchangeServer{probeServer{hostSigner: other, authorized: clientKey}, "ok"}).dial, CodeHostKeyChanged},
		// Review of round 5 (W1): before HostKeyCallback, x/crypto verifies
		// the host key signature and agrees on algorithms; those failures are
		// answers, not "the backup does not answer".
		{"the pinned key with a bad signature", (&exchangeServer{probeServer{hostSigner: badSignatureSigner{host.PublicKey(), otherSigner}, authorized: clientKey}, "ok"}).dial, CodeUnavailable},
		{"no ed25519 host key (RSA only)", (&exchangeServer{probeServer{hostSigner: rsaSigner, authorized: clientKey}, "ok"}).dial, CodeUnavailable},
		{"another service on the SSH port", lineServer(t, "HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n"), CodeUnavailable},
		{"the enrolled key refused", (&exchangeServer{probeServer{hostSigner: host}, "ok"}).dial, CodeUnavailable},
		{"the session refused", (&exchangeServer{probeServer{hostSigner: host, authorized: clientKey}, "reject"}).dial, CodeUnavailable},
		{"the forced command exited 1", (&exchangeServer{probeServer{hostSigner: host, authorized: clientKey}, "exit1"}).dial, "command_failed"},
		{"an answer", (&exchangeServer{probeServer{hostSigner: host, authorized: clientKey}, "ok"}).dial, "answered"},
	} {
		t.Run("answer: "+tc.name, func(t *testing.T) {
			if got, _ := run(t, 10*time.Second, tc.dial); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
	// Inspect passes "no answer" through and keeps an answer an answer.
	fixtureEnrollment, fixtureRequest, _, _ := proofFixture(t)
	for _, code := range []Code{CodeNoAnswer, CodeHostKeyChanged, CodeEnrollment} {
		// W2: this server's own unusable key (CodeEnrollment from
		// readOwnerKey) is not the backup refusing.
		if _, _, err := Inspect(context.Background(), fixtureEnrollment, fixtureRequest,
			&fakeExchange{err: Unknown{Code: code}}); !IsCode(err, code) {
			t.Fatalf("Inspect turned %s into %v", code, err)
		}
	}
	// The real exchange: an enrolled key file that is missing is this
	// server's own enrollment problem, before any connection.
	missing := enrollment
	missing.PrivateKeyPath = keyPath + ".absent"
	if _, _, err := exchangeFixedAt(context.Background(), missing, request, FixedCommand, refused); !IsCode(err, CodeEnrollment) {
		t.Fatalf("missing key: %v", err)
	}
	if !(Unknown{Code: CodeUnavailable}).PeerAnswered() || !(Unknown{Code: CodeMalformed}).PeerAnswered() ||
		!(Unknown{Code: CodeHostKeyChanged}).PeerAnswered() ||
		(Unknown{Code: CodeNoAnswer}).PeerAnswered() || (Unknown{Code: CodeEnrollment}).PeerAnswered() {
		t.Fatal("PeerAnswered does not follow the codes")
	}
}
