//go:build linux

// Package dnspeertransport obtains one bounded, authenticated, read-only BIND
// observation over an owner-enrolled SSH channel. It grants no mutation rights.
package dnspeertransport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeerproof"
	"github.com/alicelik/celikpanel/internal/transport"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
)

const (
	FixedCommand         = "celikpanel-bind-peer-inspect-v1"
	FixedPowerDNSCommand = "celikpanel-pdns-peer-inspect-v1"
	maxWireSize          = 4096
	maxStderr            = 1024
	maxDuration          = 10 * time.Second
)

// ExchangeLimit bounds one enrolled inspector exchange: TCP connect, pinned
// host key, key authentication, the forced command and its answer. Pair 6
// measured 2.5-4 s from the challenge write to the end of the inspection on
// QEMU guests; 2026-10-01 raised from 5 s to the transport maximum for
// headroom. An exchange that runs out of it is dns_peer_proof_timeout.
const ExchangeLimit = maxDuration

var usernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// Enrollment is reviewed owner configuration, never populated from a request.
// The server must enroll a dedicated non-root account with an OpenSSH forced
// command, prohibit PTY/forwarding, and permit only this inspector. The client
// cannot attest server-side authorized_keys restrictions by itself.
type Enrollment struct {
	PeerIP         string
	Username       string
	HostKeySHA256  string // lowercase hex SHA-256 of ssh.PublicKey.Marshal()
	PrivateKeyPath string // root-owned regular file, mode 0400 or 0600
	Timeout        time.Duration
}

type Code string

const (
	CodeEnrollment Code = "peer_enrollment_invalid"
	// CodeUnavailable: the enrolled peer answered, but not with an
	// observation this server can accept: another host key, the enrolled key
	// refused, the session refused, an oversized answer, or the forced
	// command exited unsuccessfully (Reason may name why).
	CodeUnavailable Code = "peer_inspection_unavailable"
	CodeMalformed   Code = "peer_inspection_malformed"
	// CodeNoAnswer (alpha.133 round 5): the peer gave no answer at all. No
	// TCP connection to its SSH port (refused, no route, timed out), the
	// connection ended before the peer presented any host key, or the
	// connection was lost or the exchange's time ran out before the forced
	// command reported a result. It proves nothing about the peer; it is
	// what a switched-off, restarting or unreachable peer looks like, and it
	// is classified the same however quickly or at whichever instant of the
	// time bound it came back.
	CodeNoAnswer Code = "peer_no_answer"
	// CodeHostKeyChanged (alpha.133 round 5 review): the peer answered with
	// another SSH host key than the pinned one. An answer, and a possible
	// security event; the Agent reports dns_peer_enrollment_changed:host_key.
	CodeHostKeyChanged Code = "peer_host_key_changed"
)

// PeerAnswered reports an inspection the enrolled peer did answer (with an
// answer this server cannot accept): CodeUnavailable or CodeMalformed. Such
// an outcome keeps its own reason even when it came back at the step's time
// bound; CodeNoAnswer, CodeEnrollment and anything else do not.
func (e Unknown) PeerAnswered() bool {
	return e.Code == CodeUnavailable || e.Code == CodeMalformed || e.Code == CodeHostKeyChanged
}

// Unknown never asserts absence, failure of the native service, or deletion.
// Details from SSH, including paths and stderr, are deliberately not returned.
// Reason is set only when the pinned, authenticated peer's forced command
// exited with exactly one reviewed reason line bound to this request
// (dnspeerproof.ParseInspectorReason); it is a token, never peer text.
type Unknown struct {
	Code   Code
	Reason string
}

// InspectorReason returns the reviewed reason an authenticated inspector
// reported for an incomplete observation, or "".
func InspectorReason(err error) string {
	var u Unknown
	if errors.As(err, &u) && u.Code == CodeUnavailable {
		return u.Reason
	}
	return ""
}

// commandFailure is an authenticated session whose fixed command exited
// unsuccessfully. Its bounded stderr is parsed only against the request digest.
type commandFailure struct{ stderr []byte }

func (commandFailure) Error() string { return string(CodeUnavailable) }

// CommandFailed is what an Exchanger returns when the pinned, authenticated
// peer ran the fixed forced command and it exited non-zero. Only the bounded
// stderr is kept, for digest-bound parsing of one reviewed reason token.
func CommandFailed(stderr []byte) error {
	return commandFailure{stderr: append([]byte(nil), stderr...)}
}

// CommandFailureReason reports whether err is such a command failure and, if
// so, the reviewed reason its stderr carries for exactly these request bytes
// ("" when the stderr is not one reviewed line bound to them).
func CommandFailureReason(err error, request []byte) (string, bool) {
	var failed commandFailure
	if err == nil || !errors.As(err, &failed) {
		return "", false
	}
	sum := sha256.Sum256(request)
	return dnspeerproof.ParseInspectorReason(failed.stderr, hex.EncodeToString(sum[:])), true
}

// bindInspectorReason keeps the reasons bind-peer-inspect can report. A
// PowerDNS-only token from the BIND channel is not trusted as a detail.
func bindInspectorReason(reason string) string {
	if reason == transport.DNSPeerInspectorReasonConfigUnreviewed {
		return ""
	}
	return reason
}

func (e Unknown) Error() string { return string(e.Code) }

func IsCode(err error, code Code) bool {
	var u Unknown
	return errors.As(err, &u) && u.Code == code
}

// Validate rejects an unreviewed SSH identity or credential location.
func (e Enrollment) Validate() error { return e.validate() }

func (e Enrollment) validate() error {
	ip := net.ParseIP(e.PeerIP)
	if ip == nil || ip.To4() == nil || ip.String() != e.PeerIP || !ip.IsGlobalUnicast() ||
		!usernamePattern.MatchString(e.Username) || e.Username == "root" ||
		len(e.HostKeySHA256) != 64 || !filepath.IsAbs(e.PrivateKeyPath) ||
		e.Timeout <= 0 || e.Timeout > maxDuration {
		return Unknown{Code: CodeEnrollment}
	}
	b, err := hex.DecodeString(e.HostKeySHA256)
	if err != nil || len(b) != 32 || hex.EncodeToString(b) != e.HostKeySHA256 {
		return Unknown{Code: CodeEnrollment}
	}
	return nil
}

// Exchanger makes the authenticated transport replaceable by a fixture. A
// production implementation must derive Authentication from its SSH handshake.
type Exchanger interface {
	Exchange(context.Context, Enrollment, []byte) ([]byte, dnspeerproof.PeerAuthentication, error)
}

// Inspect validates the exact request/enrollment binding, exchanges one proof
// document and rejects malformed output. A caller must still call
// dnspeerproof.Verify with a durable consume-once callback and current operation
// rechecks before treating the response as terminal evidence.
func Inspect(ctx context.Context, enrollment Enrollment, request dnspeerproof.RequestV1, exchanger Exchanger) (dnspeerproof.ResponseV1, dnspeerproof.PeerAuthentication, error) {
	if enrollment.validate() != nil || exchanger == nil || request.Validate() != nil ||
		request.PeerIP != enrollment.PeerIP || request.PeerIdentitySHA256 != enrollment.HostKeySHA256 {
		return dnspeerproof.ResponseV1{}, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeEnrollment}
	}
	raw, err := dnspeerproof.EncodeRequest(request)
	if err != nil || len(raw) > maxWireSize {
		return dnspeerproof.ResponseV1{}, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeEnrollment}
	}
	deadline := enrollment.Timeout
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	answer, auth, err := exchanger.Exchange(ctx, enrollment, raw)
	if reason, failed := CommandFailureReason(err, raw); failed {
		return dnspeerproof.ResponseV1{}, dnspeerproof.PeerAuthentication{}, Unknown{
			Code: CodeUnavailable, Reason: bindInspectorReason(reason),
		}
	}
	// No answer, another host key, and this server's own enrollment or key
	// file (alpha.133 round 5 review: an unreadable key here is not the
	// backup refusing) keep their own codes.
	for _, code := range []Code{CodeNoAnswer, CodeHostKeyChanged, CodeEnrollment} {
		if IsCode(err, code) {
			return dnspeerproof.ResponseV1{}, dnspeerproof.PeerAuthentication{}, Unknown{Code: code}
		}
	}
	if err != nil || !auth.Established || auth.PeerIP != enrollment.PeerIP || auth.IdentitySHA256 != enrollment.HostKeySHA256 {
		return dnspeerproof.ResponseV1{}, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeUnavailable}
	}
	if len(answer) == 0 || len(answer) > maxWireSize+1 {
		return dnspeerproof.ResponseV1{}, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeMalformed}
	}
	if answer[len(answer)-1] == '\n' {
		answer = answer[:len(answer)-1]
	}
	if len(answer) == 0 || len(answer) > maxWireSize {
		return dnspeerproof.ResponseV1{}, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeMalformed}
	}
	response, err := dnspeerproof.DecodeResponse(answer)
	if err != nil {
		return dnspeerproof.ResponseV1{}, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeMalformed}
	}
	return response, auth, nil
}

// SSH is the production Exchanger. It never invokes a shell locally, opens a
// PTY, requests forwarding, uses agent/password auth or accepts a new host key.
type SSH struct{}

func (SSH) Exchange(ctx context.Context, enrollment Enrollment, request []byte) ([]byte, dnspeerproof.PeerAuthentication, error) {
	return exchangeFixed(ctx, enrollment, request, FixedCommand)
}

// PowerDNSSSH uses a separate owner-enrolled forced command. The command is
// selected by the type, never by the peer request or an untrusted response.
type PowerDNSSSH struct{}

func (PowerDNSSSH) Exchange(ctx context.Context, enrollment Enrollment, request []byte) ([]byte, dnspeerproof.PeerAuthentication, error) {
	return exchangeFixed(ctx, enrollment, request, FixedPowerDNSCommand)
}

func exchangeFixed(ctx context.Context, enrollment Enrollment, request []byte, command string) ([]byte, dnspeerproof.PeerAuthentication, error) {
	return exchangeFixedAt(ctx, enrollment, request, command, func(ctx context.Context, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", address)
	})
}

// connectionLost reports a failure of the connection itself (closed, reset,
// ended, timed out), as opposed to something the peer said.
func connectionLost(err error) bool {
	var netErr net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, os.ErrDeadlineExceeded) || errors.As(err, &netErr)
}

// noAnswerOr is CodeNoAnswer when the exchange's time ran out or the
// connection itself failed, and CodeUnavailable (the peer answered) otherwise.
// After ctx ended the connection is closed by this exchange, so whatever the
// SSH layer reports then is the deadline's doing.
func noAnswerOr(ctx context.Context, err error) Unknown {
	if ctx.Err() != nil || connectionLost(err) {
		return Unknown{Code: CodeNoAnswer}
	}
	return Unknown{Code: CodeUnavailable}
}

// exchangeFixedAt is exchangeFixed with the TCP dial replaceable by a test.
//
// Which failure is which (alpha.133 round 5, lab w9: a backup whose SSH port
// was blocked once came back as "the inspection could not complete" because
// the dialer's own poller deadline and the step's context deadline are two
// timers for the same instant, and whichever fired first decided the code):
//
//	no answer (CodeNoAnswer)       the dial failed (refused, no route, timed
//	                               out, cancelled); the connection ended before
//	                               any host key was presented; the connection
//	                               was lost, ended without an exit status, or
//	                               the exchange's time ran out after that
//	an answer (CodeUnavailable)    another host key; the enrolled key refused;
//	                               the session refused; an oversized answer;
//	                               the forced command's non-zero exit
//	                               (CommandFailed, which wins over a deadline)
//
// The dial is classified as no answer whatever error value it returns, so the
// outcome no longer depends on which of the two timers fired first.
func exchangeFixedAt(ctx context.Context, enrollment Enrollment, request []byte, command string,
	dial func(context.Context, string) (net.Conn, error),
) ([]byte, dnspeerproof.PeerAuthentication, error) {
	if enrollment.validate() != nil || len(request) == 0 || len(request) > maxWireSize {
		return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeEnrollment}
	}
	key, err := readOwnerKey(enrollment.PrivateKeyPath)
	if err != nil {
		return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeEnrollment}
	}
	signer, err := ssh.ParsePrivateKey(key)
	for i := range key {
		key[i] = 0
	}
	if err != nil {
		return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeEnrollment}
	}
	var hostKeySeen, matched bool
	config := &ssh.ClientConfig{
		User: enrollment.Username,
		// V1 enrollment pins an Ed25519 host key digest. Never let the SSH
		// default preference select another key from the same server.
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
	conn, err := dial(ctx, net.JoinHostPort(enrollment.PeerIP, "22"))
	if err != nil {
		// Refused, no route, timed out or cancelled: nothing answered.
		return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeNoAnswer}
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(enrollment.Timeout)); err != nil {
		return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeNoAnswer}
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
	banner := &bannerConn{Conn: conn}
	clientConn, chans, reqs, err := ssh.NewClientConn(banner, net.JoinHostPort(enrollment.PeerIP, "22"), config)
	switch {
	case hostKeySeen && !matched:
		// Another host key on the peer's address.
		return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeHostKeyChanged}
	case err != nil && !hostKeySeen && !banner.notSSH():
		// Before any host key was accepted for checking: a connection that
		// ended, was reset or ran out of time is no answer; a failed key
		// exchange (no common algorithm, a host key signature that does not
		// verify, a protocol error) is an answer this server cannot accept
		// (alpha.133 round 5 review: x/crypto verifies the signature before
		// it calls HostKeyCallback).
		return nil, dnspeerproof.PeerAuthentication{}, noAnswerOr(ctx, err)
	case err != nil && !hostKeySeen:
		// Another service answered on the SSH port.
		return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeUnavailable}
	case err != nil:
		// The pinned host refused the enrolled key, unless the connection
		// itself was lost or the time ran out during authentication.
		return nil, dnspeerproof.PeerAuthentication{}, noAnswerOr(ctx, err)
	case !matched:
		return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeUnavailable}
	}
	client := ssh.NewClient(clientConn, chans, reqs)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		var refused *ssh.OpenChannelError
		if errors.As(err, &refused) {
			return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeUnavailable}
		}
		return nil, dnspeerproof.PeerAuthentication{}, noAnswerOr(ctx, err)
	}
	defer session.Close()
	var stdout limitedBuffer
	stdout.limit = maxWireSize + 1
	var stderr limitedBuffer
	stderr.limit = maxStderr
	session.Stdin = bytes.NewReader(request)
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run(command); err != nil || stdout.overflow || stderr.overflow {
		var exit *ssh.ExitError
		var missing *ssh.ExitMissingError
		switch {
		case stdout.overflow || stderr.overflow:
			return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeUnavailable}
		case errors.As(err, &exit) && matched:
			// The pinned host ran the owner's forced command, which exited
			// non-zero. Only its bounded stderr is kept, for digest-bound
			// parsing of one reviewed reason token.
			return nil, dnspeerproof.PeerAuthentication{}, CommandFailed(stderr.Bytes())
		case errors.As(err, &missing):
			// The connection ended before the command reported a result.
			return nil, dnspeerproof.PeerAuthentication{}, Unknown{Code: CodeNoAnswer}
		}
		return nil, dnspeerproof.PeerAuthentication{}, noAnswerOr(ctx, err)
	}
	auth := dnspeerproof.PeerAuthentication{Established: true, IdentitySHA256: enrollment.HostKeySHA256, PeerIP: enrollment.PeerIP}
	return stdout.Bytes(), auth, nil
}

// bannerConn keeps the first bytes the peer sent, so that a service other
// than SSH on the peer's port (its first bytes are not "SSH-") is told from
// an sshd that went away before its host key.
type bannerConn struct {
	net.Conn
	first []byte
}

func (c *bannerConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if need := 4 - len(c.first); need > 0 && n > 0 {
		c.first = append(c.first, p[:min(need, n)]...)
	}
	return n, err
}

// notSSH reports that the peer sent bytes and they do not start an SSH
// identification line. RFC 4253 allows other lines before it, so this is only
// read when the handshake failed.
func (c *bannerConn) notSSH() bool {
	return len(c.first) > 0 && !bytes.HasPrefix([]byte("SSH-"), c.first) && !bytes.HasPrefix(c.first, []byte("SSH-"))
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		b.overflow = true
		return 0, io.ErrShortWrite
	}
	return b.Buffer.Write(p)
}

func readOwnerKey(path string) ([]byte, error) {
	if filepath.Clean(path) != path || !filepath.IsAbs(path) {
		return nil, fmt.Errorf("unsafe enrolled SSH credential")
	}
	for parent := filepath.Dir(path); parent != "/"; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
			return nil, fmt.Errorf("unsafe enrolled SSH credential")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			return nil, fmt.Errorf("unsafe enrolled SSH credential")
		}
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 16*1024 ||
		info.Mode().Perm()&0077 != 0 || info.Mode().Perm()&0111 != 0 {
		return nil, fmt.Errorf("unsafe enrolled SSH credential")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Nlink != 1 {
		return nil, fmt.Errorf("unsafe enrolled SSH credential")
	}
	return io.ReadAll(io.LimitReader(f, 16*1024+1))
}
