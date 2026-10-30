//go:build linux

package dnspeertransport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// ProbeResult is the reviewed outcome of one reachability check of an
// enrolled inspector channel. It never carries SSH or peer text.
type ProbeResult string

const (
	// ProbeReachable: TCP connect to the peer's SSH port, the pinned host
	// key and key authentication of the enrolled account all succeeded.
	ProbeReachable ProbeResult = "reachable"
	// ProbeUnreachable: no TCP connection to the peer's SSH port 22.
	ProbeUnreachable ProbeResult = "unreachable"
	// ProbeHostKeyMismatch: the peer answered with another host key than
	// the pinned one; nothing was authenticated.
	ProbeHostKeyMismatch ProbeResult = "host_key_mismatch"
	// ProbeAuthRefused: the pinned host refused the enrolled key, or the SSH
	// handshake did not complete after the host key matched.
	ProbeAuthRefused ProbeResult = "auth_refused"
	// ProbeCredential: the local enrolled credential could not be read.
	ProbeCredential ProbeResult = "credential"
)

// Probe makes one bounded SSH connection with the enrolled identity and
// closes it after authentication. It opens no session, runs no command, sends
// no request and so cannot observe or change DNS state; it only tells the
// owner whether the channel a later deletion proof needs is reachable.
func Probe(ctx context.Context, enrollment Enrollment) ProbeResult {
	return probeAt(ctx, enrollment, func(ctx context.Context, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", address)
	})
}

func probeAt(ctx context.Context, enrollment Enrollment, dial func(context.Context, string) (net.Conn, error)) ProbeResult {
	if enrollment.validate() != nil {
		return ProbeCredential
	}
	key, err := readOwnerKey(enrollment.PrivateKeyPath)
	if err != nil {
		return ProbeCredential
	}
	signer, err := ssh.ParsePrivateKey(key)
	for i := range key {
		key[i] = 0
	}
	if err != nil {
		return ProbeCredential
	}
	var hostKeySeen, matched bool
	config := &ssh.ClientConfig{
		User:              enrollment.Username,
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
		Auth:              []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: func(_ string, _ net.Addr, actual ssh.PublicKey) error {
			hostKeySeen = true
			sum := sha256.Sum256(actual.Marshal())
			matched = hex.EncodeToString(sum[:]) == enrollment.HostKeySHA256
			if !matched {
				return errors.New("pinned SSH host identity mismatch")
			}
			return nil
		},
		Timeout: enrollment.Timeout,
	}
	ctx, cancel := context.WithTimeout(ctx, enrollment.Timeout)
	defer cancel()
	address := net.JoinHostPort(enrollment.PeerIP, "22")
	conn, err := dial(ctx, address)
	if err != nil {
		return ProbeUnreachable
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(enrollment.Timeout)); err != nil {
		return ProbeUnreachable
	}
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	switch {
	case hostKeySeen && !matched:
		return ProbeHostKeyMismatch
	case err != nil && !hostKeySeen:
		// The port answered but no SSH handshake reached the host key.
		return ProbeUnreachable
	case err != nil:
		return ProbeAuthRefused
	}
	go ssh.DiscardRequests(reqs)
	go func() {
		for channel := range chans {
			_ = channel.Reject(ssh.Prohibited, "no channels")
		}
	}()
	_ = clientConn.Close()
	return ProbeReachable
}
