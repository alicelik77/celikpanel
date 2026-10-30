//go:build linux

package dnspeertransport

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type probeServer struct {
	hostSigner ssh.Signer
	authorized ssh.PublicKey
	sessions   atomic.Int32
}

// dial serves one SSH connection on a loopback listener (an unbuffered
// net.Pipe would deadlock the simultaneous version exchange).
func (s *probeServer) dial(ctx context.Context, _ string) (net.Conn, error) {
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
		go ssh.DiscardRequests(reqs)
		for channel := range chans {
			s.sessions.Add(1)
			_ = channel.Reject(ssh.Prohibited, "test")
		}
		conn.Close()
	}()
	return (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
}

func probeKeyFile(t *testing.T) (string, ssh.PublicKey) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root-owned credential validation requires root")
	}
	dir, err := os.MkdirTemp("/root", "dns-peer-probe-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "id")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return path, key
}

func probeHostSigner(t *testing.T) (ssh.Signer, string) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(signer.PublicKey().Marshal())
	return signer, hex.EncodeToString(sum[:])
}

// The probe authenticates with the enrolled key against the pinned host key
// and closes: it never opens a session, so it cannot run the inspector.
func TestProbeReportsEachReviewedOutcomeWithoutASession(t *testing.T) {
	keyPath, clientKey := probeKeyFile(t)
	host, digest := probeHostSigner(t)
	enrollment := Enrollment{PeerIP: "192.0.2.11", Username: "celikpeer", HostKeySHA256: digest, PrivateKeyPath: keyPath, Timeout: 3 * time.Second}

	server := &probeServer{hostSigner: host, authorized: clientKey}
	if got := probeAt(context.Background(), enrollment, server.dial); got != ProbeReachable {
		t.Fatalf("reachable: %s", got)
	}
	if server.sessions.Load() != 0 {
		t.Fatal("the probe opened a session")
	}
	refused := &probeServer{hostSigner: host}
	if got := probeAt(context.Background(), enrollment, refused.dial); got != ProbeAuthRefused {
		t.Fatalf("refused key: %s", got)
	}
	other, _ := probeHostSigner(t)
	impostor := &probeServer{hostSigner: other, authorized: clientKey}
	if got := probeAt(context.Background(), enrollment, impostor.dial); got != ProbeHostKeyMismatch {
		t.Fatalf("another host key: %s", got)
	}
	down := func(context.Context, string) (net.Conn, error) { return nil, errors.New("connection refused") }
	if got := probeAt(context.Background(), enrollment, down); got != ProbeUnreachable {
		t.Fatalf("no connection: %s", got)
	}
	silent := func(context.Context, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() { server.Close() }()
		return client, nil
	}
	if got := probeAt(context.Background(), enrollment, silent); got != ProbeUnreachable {
		t.Fatalf("a port that is not SSH: %s", got)
	}
	missing := enrollment
	missing.PrivateKeyPath = keyPath + ".absent"
	if got := probeAt(context.Background(), missing, server.dial); got != ProbeCredential {
		t.Fatalf("missing credential: %s", got)
	}
}
