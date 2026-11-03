package remotebackup

// SFTP destination: a minimal SFTP version 3 client
// (draft-ietf-secsh-filexfer-02) over golang.org/x/crypto/ssh, so no extra
// module is needed. Requests are sent one at a time (no pipelining); data
// moves in 32 KiB READ/WRITE chunks and every inbound packet is bounded.
//
// The server's host key must be confirmed by a person first: ProbeSFTPHostKey
// learns it, the person confirms its fingerprint, and DialSFTP refuses any
// connection whose host key is not exactly the confirmed one, before any
// credential is sent.
//
// SFTP hedefi: ek modül gerektirmeyen küçük bir SFTP v3 istemcisi. Sunucunun
// anahtarı önce bir kişi tarafından onaylanır; onaylanmamış ya da değişmiş
// anahtarla hiçbir kimlik bilgisi gönderilmez.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
)

// SFTPConfig describes one SFTP destination.
type SFTPConfig struct {
	Host       string        // DNS name or IP, no scheme, no port, no user
	Port       int           // default 22 when 0
	User       string        // account on the SFTP server
	Password   string        // Password and/or PrivateKey; at least one
	PrivateKey []byte        // unencrypted PEM (OpenSSH, PKCS#8 or PKCS#1)
	BasePath   string        // absolute, clean remote folder; keys are relative to it
	HostKey    string        // confirmed host key, authorized_keys format; empty = not confirmed
	Timeout    time.Duration // dial + handshake + session start; default 20s
}

// SFTPHostKey is what a first connection learned, for a person to confirm.
type SFTPHostKey struct {
	Key         string // authorized_keys line without comment, e.g. "ssh-ed25519 AAAA..."
	Fingerprint string // ssh.FingerprintSHA256 form "SHA256:..."
}

// HostKeyMismatch is the underlying error (inside an *Error of kind
// KindHostKey) when the server's host key is not the confirmed one, or when
// none is confirmed yet (Pinned is then empty). Presented is the key the
// server showed; it is empty when the server no longer offers a key of the
// confirmed type at all. Nothing was sent to the server in either case.
//
//	var hm *HostKeyMismatch
//	if errors.As(err, &hm) { show(hm.Presented.Fingerprint) }
type HostKeyMismatch struct {
	Pinned    string
	Presented SFTPHostKey
}

func (m *HostKeyMismatch) Error() string {
	switch {
	case m.Pinned == "":
		return "sftp host key not confirmed yet (server presented " + m.Presented.Fingerprint + ")"
	case m.Presented.Key == "":
		return "sftp server does not offer a host key of the confirmed type"
	}
	return "sftp host key differs from the confirmed one (server presented " + m.Presented.Fingerprint + ")"
}

const (
	sftpChunk          = 32 * 1024
	sftpMaxPacket      = 256*1024 + 1024 // inbound bound: 256 KiB data + header room
	sftpMaxListEntries = 100000
	sftpDefaultTimeout = 20 * time.Second
	sftpMaxHandle      = 256 // the protocol allows at most 256 bytes
)

// Packet types (draft-ietf-secsh-filexfer-02, section 3).
const (
	fxpInit     = 1
	fxpVersion  = 2
	fxpOpen     = 3
	fxpClose    = 4
	fxpRead     = 5
	fxpWrite    = 6
	fxpLstat    = 7
	fxpFstat    = 8
	fxpOpendir  = 11
	fxpReaddir  = 12
	fxpRemove   = 13
	fxpMkdir    = 14
	fxpStat     = 17
	fxpRename   = 18
	fxpStatus   = 101
	fxpHandle   = 102
	fxpData     = 103
	fxpName     = 104
	fxpAttrs    = 105
	fxpExtended = 200
)

// Status codes. 14 and 15 come from later drafts; some v3 servers send them.
const (
	fxOK               = 0
	fxEOF              = 1
	fxNoSuchFile       = 2
	fxPermissionDenied = 3
	fxFailure          = 4
	fxBadMessage       = 5
	fxNoConnection     = 6
	fxConnectionLost   = 7
	fxOpUnsupported    = 8
	fxNoSpace          = 14
	fxQuotaExceeded    = 15
)

// OPEN flags.
const (
	fxfRead  = 0x01
	fxfWrite = 0x02
	fxfCreat = 0x08
	fxfTrunc = 0x10
	fxfExcl  = 0x20
)

// ATTRS flags and POSIX file type bits.
const (
	attrSize     = 0x00000001
	attrUIDGID   = 0x00000002
	attrPerms    = 0x00000004
	attrTimes    = 0x00000008
	attrExtended = 0x80000000

	modeType = 0o170000
	modeReg  = 0o100000
	modeDir  = 0o040000
	modeLink = 0o120000
)

const posixRenameExt = "posix-rename@openssh.com"

var (
	errSFTPMalformed  = errors.New("sftp: malformed packet")
	errHostKeyRefused = errors.New("sftp: host key not confirmed")
)

// probeHostKeyAlgorithms is the order used when no key is confirmed yet:
// modern key types first, never SHA-1 RSA signatures, never certificates.
var probeHostKeyAlgorithms = []string{
	ssh.KeyAlgoED25519,
	ssh.KeyAlgoECDSA256,
	ssh.KeyAlgoECDSA384,
	ssh.KeyAlgoECDSA521,
	ssh.KeyAlgoRSASHA512,
	ssh.KeyAlgoRSASHA256,
}

// hostKeyAlgorithmsFor makes the server prove the confirmed key type, so a
// server that also has keys of other types still matches the confirmed one.
func hostKeyAlgorithmsFor(keyType string) []string {
	if keyType == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	}
	return []string{keyType}
}

// ---- configuration --------------------------------------------------------

func validSFTPHost(host string) (string, bool) {
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	if host == "" || len(host) > 253 || strings.ContainsAny(host, " /@\\[]") {
		return "", false
	}
	for i := 0; i < len(host); i++ {
		if host[i] < 0x21 || host[i] == 0x7f {
			return "", false
		}
	}
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return "", false // "host:22", "sftp://..." and similar
	}
	return host, true
}

func validSFTPPort(port int) (int, bool) {
	if port == 0 {
		return 22, true
	}
	return port, port >= 1 && port <= 65535
}

// normalized validates cfg without any network use. BasePath "/" is allowed:
// chrooted SFTP accounts often see their own area as "/".
func (cfg SFTPConfig) normalized() (SFTPConfig, error) {
	const op = "connect"
	c := cfg
	var ok bool
	if c.Host, ok = validSFTPHost(cfg.Host); !ok {
		return c, newError(KindConfig, op, "the SFTP server address must be a host name or IP address only (no scheme, user, port or path)", nil)
	}
	if c.Port, ok = validSFTPPort(cfg.Port); !ok {
		return c, newError(KindConfig, op, "the SFTP port must be between 1 and 65535", nil)
	}
	if c.User == "" || len(c.User) > 256 || strings.IndexFunc(c.User, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return c, newError(KindConfig, op, "the SFTP user name is missing or invalid", nil)
	}
	if c.Password == "" && len(c.PrivateKey) == 0 {
		return c, newError(KindConfig, op, "a password or a private key is required for the SFTP account", nil)
	}
	bp := c.BasePath
	if bp == "" || bp[0] != '/' || path.Clean(bp) != bp || len(bp) > 1024 ||
		strings.ContainsRune(bp, '\\') || strings.IndexFunc(bp, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return c, newError(KindConfig, op, "the remote folder must be an absolute, clean path such as /backups/celikpanel", nil)
	}
	if c.Timeout <= 0 {
		c.Timeout = sftpDefaultTimeout
	}
	return c, nil
}

func sftpAuthMethods(c SFTPConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if len(c.PrivateKey) > 0 {
		signer, err := ssh.ParsePrivateKey(c.PrivateKey)
		if err != nil {
			// The parser's error is not included: it is about key material.
			return nil, newError(KindConfig, "connect", "the private key could not be read; it must be an unencrypted PEM key (OpenSSH, PKCS#8 or PKCS#1)", nil)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if c.Password != "" {
		pw := c.Password
		methods = append(methods, ssh.Password(pw),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, echos []bool) ([]string, error) {
				// Only a single hidden prompt is answered with the password;
				// multi-step challenges (one-time codes) are not supported.
				switch {
				case len(questions) == 0:
					return nil, nil
				case len(questions) == 1 && !echos[0]:
					return []string{pw}, nil
				}
				return nil, errors.New("sftp: unsupported keyboard-interactive challenge")
			}))
	}
	return methods, nil
}

// ---- host key -------------------------------------------------------------

type hostKeyCapture struct {
	mu        sync.Mutex
	pinned    ssh.PublicKey
	presented ssh.PublicKey
	refused   bool
}

func (h *hostKeyCapture) callback(_ string, _ net.Addr, key ssh.PublicKey) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.presented = key
	if h.pinned != nil && bytes.Equal(key.Marshal(), h.pinned.Marshal()) {
		return nil
	}
	h.refused = true
	return errHostKeyRefused
}

func (h *hostKeyCapture) result() (ssh.PublicKey, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.presented, h.refused
}

func hostKeyInfo(key ssh.PublicKey) SFTPHostKey {
	if key == nil {
		return SFTPHostKey{}
	}
	return SFTPHostKey{
		Key:         strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))),
		Fingerprint: ssh.FingerprintSHA256(key),
	}
}

func hostKeyError(op string, pinned, presented ssh.PublicKey) *Error {
	m := &HostKeyMismatch{Presented: hostKeyInfo(presented)}
	if pinned == nil {
		return newError(KindHostKey, op, "the SFTP server's host key must be confirmed first: check that its fingerprint "+
			m.Presented.Fingerprint+" is the server's real key, then confirm it; nothing was sent", m)
	}
	m.Pinned = hostKeyInfo(pinned).Key
	if presented == nil {
		return newError(KindHostKey, op, "the SFTP server no longer offers a host key of the confirmed type; if the server was reinstalled, confirm its new key; nothing was sent", m)
	}
	return newError(KindHostKey, op, "the SFTP server presented a different host key ("+m.Presented.Fingerprint+
		") than the confirmed one; nothing was sent. If the server's key was changed on purpose, confirm the new fingerprint; otherwise the connection may be intercepted", m)
}

// sshHandshake dials and runs the SSH handshake (and authentication, if cfg
// has auth methods) within timeout and ctx.
func sshHandshake(ctx context.Context, host string, port int, timeout time.Duration, cfg *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	nc, err := d.DialContext(dctx, "tcp", addr)
	if err != nil {
		return nil, nil, nil, err
	}
	if dl, ok := dctx.Deadline(); ok {
		_ = nc.SetDeadline(dl)
	}
	stop := context.AfterFunc(dctx, func() { _ = nc.Close() })
	c, chans, reqs, err := ssh.NewClientConn(nc, addr, cfg)
	if !stop() {
		if err == nil {
			_ = c.Close()
		}
		_ = nc.Close()
		return nil, nil, nil, dctx.Err()
	}
	if err != nil {
		_ = nc.Close()
		return nil, nil, nil, err
	}
	_ = nc.SetDeadline(time.Time{})
	return c, chans, reqs, nil
}

func classifyConnectErr(ctx context.Context, op string, c SFTPConfig, err error) *Error {
	where := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	msg := err.Error()
	var ne net.Error
	switch {
	case ctx.Err() != nil:
		return newError(KindNetwork, op, "connecting to the SFTP server at "+where+" was cancelled; the result is unknown", ctx.Err())
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return newError(KindNetwork, op, "the SFTP server at "+where+" did not answer in time; check the address, port and firewall, then try again", err)
	case strings.Contains(msg, "unable to authenticate") || strings.Contains(msg, "no supported methods remain"):
		return newError(KindCredentials, op, "the SFTP server refused the sign-in for user "+strconv.Quote(c.User)+"; the destination's owner checks the user name and the password or private key", nil)
	}
	return newError(KindNetwork, op, "could not connect to the SFTP server at "+where+"; check the address, port and firewall, then try again", err)
}

// ProbeSFTPHostKey connects only far enough to learn the server's host key
// (no authentication is attempted with any credentials), and returns it.
func ProbeSFTPHostKey(ctx context.Context, host string, port int, timeout time.Duration) (SFTPHostKey, error) {
	const op = "probe"
	h, ok := validSFTPHost(host)
	if !ok {
		return SFTPHostKey{}, newError(KindConfig, op, "the SFTP server address must be a host name or IP address only (no scheme, user, port or path)", nil)
	}
	p, ok := validSFTPPort(port)
	if !ok {
		return SFTPHostKey{}, newError(KindConfig, op, "the SFTP port must be between 1 and 65535", nil)
	}
	if timeout <= 0 {
		timeout = sftpDefaultTimeout
	}
	capture := &hostKeyCapture{} // nothing pinned: always aborts the handshake
	cfg := &ssh.ClientConfig{
		User:              "celikpanel-probe",
		HostKeyCallback:   capture.callback,
		HostKeyAlgorithms: probeHostKeyAlgorithms,
	}
	c, _, _, err := sshHandshake(ctx, h, p, timeout, cfg)
	if err == nil { // cannot happen: the callback refuses every key
		_ = c.Close()
	}
	if key, refused := capture.result(); refused && key != nil {
		return hostKeyInfo(key), nil
	}
	if err == nil {
		err = errors.New("sftp: no host key presented")
	}
	return SFTPHostKey{}, classifyConnectErr(ctx, op, SFTPConfig{Host: h, Port: p}, err)
}

// DialSFTP connects, verifies the host key against cfg.HostKey, authenticates,
// starts the sftp subsystem and checks BasePath exists and is a directory
// (it does NOT create BasePath; a missing BasePath is KindNotFound).
//
// An empty cfg.HostKey is refused (KindHostKey) with the presented key in a
// *HostKeyMismatch, so a person can confirm its fingerprint first.
//
// Cancelling ctx of any Store call while a request is in flight closes the
// connection; the Store must then be dialled again.
func DialSFTP(ctx context.Context, cfg SFTPConfig) (Store, error) {
	const op = "connect"
	c, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	var pinned ssh.PublicKey
	if c.HostKey != "" {
		pinned, _, _, _, err = ssh.ParseAuthorizedKey([]byte(c.HostKey))
		if err != nil {
			return nil, newError(KindConfig, op, "the confirmed host key is not in authorized_keys format (for example \"ssh-ed25519 AAAA...\")", nil)
		}
	}
	auth, err := sftpAuthMethods(c)
	if err != nil {
		return nil, err
	}
	capture := &hostKeyCapture{pinned: pinned}
	algos := probeHostKeyAlgorithms
	if pinned != nil {
		algos = hostKeyAlgorithmsFor(pinned.Type())
	}
	sc := &ssh.ClientConfig{
		User:              c.User,
		Auth:              auth,
		HostKeyCallback:   capture.callback,
		HostKeyAlgorithms: algos,
	}
	conn, chans, reqs, err := sshHandshake(ctx, c.Host, c.Port, c.Timeout, sc)
	if err != nil {
		if presented, refused := capture.result(); refused {
			return nil, hostKeyError(op, pinned, presented)
		}
		if pinned != nil && strings.Contains(err.Error(), "no common algorithm for host key") {
			return nil, hostKeyError(op, pinned, nil)
		}
		return nil, classifyConnectErr(ctx, op, c, err)
	}
	client := ssh.NewClient(conn, chans, reqs)
	st, err := startSFTP(ctx, client, c)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return st, nil
}

func startSFTP(ctx context.Context, client *ssh.Client, c SFTPConfig) (*sftpStore, error) {
	const op = "connect"
	sctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	stop := context.AfterFunc(sctx, func() { _ = client.Close() })
	fail := func(err error) (*sftpStore, error) {
		stop()
		if sctx.Err() != nil {
			return nil, newError(KindNetwork, op, "the SFTP server did not finish starting the session in time; try again", sctx.Err())
		}
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		return fail(newError(KindNetwork, op, "the SSH server did not open a session; try again", err))
	}
	w, err := session.StdinPipe()
	if err != nil {
		return fail(newError(KindNetwork, op, "the SSH session could not be set up", err))
	}
	r, err := session.StdoutPipe()
	if err != nil {
		return fail(newError(KindNetwork, op, "the SSH session could not be set up", err))
	}
	if err := session.RequestSubsystem("sftp"); err != nil {
		return fail(newError(KindProvider, op, "the server accepted the sign-in but does not offer SFTP for this account; the server's owner enables the sftp subsystem", err))
	}
	conn := &sftpConn{w: w, r: bufio.NewReaderSize(r, 64*1024)}
	if err := conn.init(op); err != nil {
		return fail(err)
	}
	a, err := conn.stat(op, c.BasePath, false)
	if err != nil {
		if KindOf(err) == KindNotFound {
			err = newError(KindNotFound, op, "the remote folder "+c.BasePath+" does not exist on the SFTP server; create it there first (CelikPanel does not create it)", err)
		}
		return fail(err)
	}
	if !a.isDir() {
		return fail(newError(KindConfig, op, "the remote path "+c.BasePath+" on the SFTP server is not a folder", nil))
	}
	if !stop() {
		return fail(nil)
	}
	return &sftpStore{conn: conn, client: client, session: session, base: c.BasePath}, nil
}

// ---- wire encoding ----------------------------------------------------------

type sftpAttrs struct {
	flags uint32
	size  uint64
	perms uint32
	atime uint32
	mtime uint32
}

func (a sftpAttrs) fileType() uint32 {
	if a.flags&attrPerms == 0 {
		return 0
	}
	return a.perms & modeType
}

func (a sftpAttrs) isDir() bool  { return a.fileType() == modeDir }
func (a sftpAttrs) isLink() bool { return a.fileType() == modeLink }

// isRegular: a regular file, or a server that does not report the type.
func (a sftpAttrs) isRegular() bool {
	t := a.fileType()
	return t == modeReg || a.flags&attrPerms == 0
}

func (a sftpAttrs) modified() time.Time {
	if a.flags&attrTimes == 0 {
		return time.Time{}
	}
	return time.Unix(int64(a.mtime), 0).UTC()
}

type sftpBuf []byte

func (b *sftpBuf) u8(v byte)      { *b = append(*b, v) }
func (b *sftpBuf) u32(v uint32)   { *b = binary.BigEndian.AppendUint32(*b, v) }
func (b *sftpBuf) u64(v uint64)   { *b = binary.BigEndian.AppendUint64(*b, v) }
func (b *sftpBuf) str(s string)   { b.u32(uint32(len(s))); *b = append(*b, s...) }
func (b *sftpBuf) bytes(p []byte) { b.u32(uint32(len(p))); *b = append(*b, p...) }
func (b *sftpBuf) attrs(a sftpAttrs) {
	b.u32(a.flags &^ (attrUIDGID | attrExtended))
	if a.flags&attrSize != 0 {
		b.u64(a.size)
	}
	if a.flags&attrPerms != 0 {
		b.u32(a.perms)
	}
	if a.flags&attrTimes != 0 {
		b.u32(a.atime)
		b.u32(a.mtime)
	}
}

type sftpReader struct {
	b   []byte
	bad bool
}

func (r *sftpReader) u8() byte {
	if len(r.b) < 1 {
		r.bad = true
		return 0
	}
	v := r.b[0]
	r.b = r.b[1:]
	return v
}

func (r *sftpReader) u32() uint32 {
	if len(r.b) < 4 {
		r.bad = true
		return 0
	}
	v := binary.BigEndian.Uint32(r.b)
	r.b = r.b[4:]
	return v
}

func (r *sftpReader) u64() uint64 {
	if len(r.b) < 8 {
		r.bad = true
		return 0
	}
	v := binary.BigEndian.Uint64(r.b)
	r.b = r.b[8:]
	return v
}

func (r *sftpReader) str() []byte {
	n := r.u32()
	if r.bad || uint64(n) > uint64(len(r.b)) {
		r.bad = true
		return nil
	}
	s := r.b[:n:n]
	r.b = r.b[n:]
	return s
}

func (r *sftpReader) attrs() sftpAttrs {
	var a sftpAttrs
	a.flags = r.u32()
	if a.flags&attrSize != 0 {
		a.size = r.u64()
	}
	if a.flags&attrUIDGID != 0 {
		r.u32()
		r.u32()
	}
	if a.flags&attrPerms != 0 {
		a.perms = r.u32()
	}
	if a.flags&attrTimes != 0 {
		a.atime = r.u32()
		a.mtime = r.u32()
	}
	if a.flags&attrExtended != 0 {
		n := r.u32()
		if uint64(n)*8 > uint64(len(r.b)) {
			r.bad = true
			return a
		}
		for i := uint32(0); i < n && !r.bad; i++ {
			r.str()
			r.str()
		}
	}
	return a
}

// ---- protocol connection ----------------------------------------------------

type sftpConn struct {
	mu     sync.Mutex
	w      io.Writer
	r      *bufio.Reader
	nextID uint32
	broken error
	exts   map[string]string
	wbuf   sftpBuf
}

type sftpEntry struct {
	name  string
	attrs sftpAttrs
}

func connError(op string, err error) *Error {
	if errors.Is(err, errSFTPMalformed) {
		return newError(KindProvider, op, "the SFTP server sent a reply that could not be read; the result of the last request is unknown", err)
	}
	return newError(KindNetwork, op, "the SFTP connection broke; the result of the last request is unknown", err)
}

func (c *sftpConn) readPacket() (byte, []byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(c.r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n < 1 || n > sftpMaxPacket {
		return 0, nil, errSFTPMalformed
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return 0, nil, err
	}
	return body[0], body[1:], nil
}

func (c *sftpConn) init(op string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var b sftpBuf
	b.u32(5)
	b.u8(fxpInit)
	b.u32(3)
	if _, err := c.w.Write(b); err != nil {
		c.broken = err
		return connError(op, err)
	}
	typ, body, err := c.readPacket()
	if err != nil {
		c.broken = err
		return connError(op, err)
	}
	r := &sftpReader{b: body}
	v := r.u32()
	if typ != fxpVersion || r.bad {
		c.broken = errSFTPMalformed
		return connError(op, errSFTPMalformed)
	}
	if v != 3 {
		c.broken = errSFTPMalformed
		return newError(KindProvider, op, fmt.Sprintf("the SFTP server speaks protocol version %d; version 3 is required", v), nil)
	}
	c.exts = map[string]string{}
	for len(r.b) > 0 && len(c.exts) < 64 {
		name, data := r.str(), r.str()
		if r.bad {
			break
		}
		c.exts[string(name)] = string(data)
	}
	return nil
}

// request sends one request and reads its reply; the reply's id is checked
// and consumed.
func (c *sftpConn) request(op string, typ byte, fill func(*sftpBuf)) (byte, *sftpReader, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken != nil {
		return 0, nil, newError(KindNetwork, op, "the SFTP connection was lost; connect again", c.broken)
	}
	c.nextID++
	id := c.nextID
	b := c.wbuf[:0]
	b.u32(0)
	b.u8(typ)
	b.u32(id)
	fill(&b)
	binary.BigEndian.PutUint32(b, uint32(len(b)-4))
	c.wbuf = b
	if _, err := c.w.Write(b); err != nil {
		c.broken = err
		return 0, nil, connError(op, err)
	}
	rtyp, body, err := c.readPacket()
	if err != nil {
		c.broken = err
		return 0, nil, connError(op, err)
	}
	r := &sftpReader{b: body}
	if rid := r.u32(); r.bad || rid != id {
		c.broken = errSFTPMalformed
		return 0, nil, connError(op, errSFTPMalformed)
	}
	return rtyp, r, nil
}

// protocolErr marks the connection unusable after an unexpected reply.
func (c *sftpConn) protocolErr(op string) error {
	c.mu.Lock()
	c.broken = errSFTPMalformed
	c.mu.Unlock()
	return connError(op, errSFTPMalformed)
}

func parseStatus(r *sftpReader) (uint32, string) {
	code := r.u32()
	msg := r.str() // older servers may omit message and language
	return code, string(msg)
}

func cleanServerText(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		n := 200
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		s = s[:n]
	}
	return s
}

func looksLikeQuota(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "quota") || strings.Contains(m, "space") || strings.Contains(m, "disk full")
}

func sftpStatusError(op, subject string, code uint32, msg string) *Error {
	msg = cleanServerText(msg)
	cause := fmt.Errorf("sftp status %d: %s", code, msg)
	switch {
	case code == fxNoSuchFile:
		return newError(KindNotFound, op, subject+" does not exist on the SFTP server", cause)
	case code == fxPermissionDenied:
		return newError(KindPermission, op, "the SFTP server does not allow this account to access "+subject+"; the server's owner fixes the folder permissions", cause)
	case code == fxNoSpace || code == fxQuotaExceeded || (code == fxFailure && looksLikeQuota(msg)):
		return newError(KindQuota, op, "the SFTP server has no space left or the account's quota is reached ("+subject+"); free space or raise the quota on the server", cause)
	case code == fxNoConnection || code == fxConnectionLost:
		return newError(KindNetwork, op, "the SFTP server lost its connection; the result is unknown", cause)
	}
	detail := ""
	if msg != "" {
		detail = " (server said: " + msg + ")"
	}
	return newError(KindProvider, op, "the SFTP server could not complete the request for "+subject+detail, cause)
}

// expectOK handles a reply that must be a STATUS.
func (c *sftpConn) expectOK(op, subject string, typ byte, r *sftpReader) error {
	if typ != fxpStatus {
		return c.protocolErr(op)
	}
	code, msg := parseStatus(r)
	if r.bad && code == 0 {
		return c.protocolErr(op)
	}
	if code == fxOK {
		return nil
	}
	return sftpStatusError(op, subject, code, msg)
}

func (c *sftpConn) simple(op, subject string, typ byte, fill func(*sftpBuf)) error {
	rt, r, err := c.request(op, typ, fill)
	if err != nil {
		return err
	}
	return c.expectOK(op, subject, rt, r)
}

func (c *sftpConn) handleReply(op, subject string, typ byte, r *sftpReader) (string, error) {
	switch typ {
	case fxpHandle:
		h := r.str()
		if r.bad || len(h) == 0 || len(h) > sftpMaxHandle {
			return "", c.protocolErr(op)
		}
		return string(h), nil
	case fxpStatus:
		code, msg := parseStatus(r)
		if code != fxOK {
			return "", sftpStatusError(op, subject, code, msg)
		}
	}
	return "", c.protocolErr(op)
}

func (c *sftpConn) stat(op, p string, lstat bool) (sftpAttrs, error) {
	t := byte(fxpStat)
	if lstat {
		t = fxpLstat
	}
	rt, r, err := c.request(op, t, func(b *sftpBuf) { b.str(p) })
	if err != nil {
		return sftpAttrs{}, err
	}
	switch rt {
	case fxpAttrs:
		a := r.attrs()
		if r.bad {
			return sftpAttrs{}, c.protocolErr(op)
		}
		return a, nil
	case fxpStatus:
		code, msg := parseStatus(r)
		if code != fxOK {
			return sftpAttrs{}, sftpStatusError(op, p, code, msg)
		}
	}
	return sftpAttrs{}, c.protocolErr(op)
}

func (c *sftpConn) open(op, p string, pflags uint32, perm uint32) (string, error) {
	rt, r, err := c.request(op, fxpOpen, func(b *sftpBuf) {
		b.str(p)
		b.u32(pflags)
		if perm != 0 {
			b.attrs(sftpAttrs{flags: attrPerms, perms: perm})
		} else {
			b.attrs(sftpAttrs{})
		}
	})
	if err != nil {
		return "", err
	}
	return c.handleReply(op, p, rt, r)
}

func (c *sftpConn) opendir(op, p string) (string, error) {
	rt, r, err := c.request(op, fxpOpendir, func(b *sftpBuf) { b.str(p) })
	if err != nil {
		return "", err
	}
	return c.handleReply(op, p, rt, r)
}

func (c *sftpConn) closeHandle(op, subject, h string) error {
	return c.simple(op, subject, fxpClose, func(b *sftpBuf) { b.str(h) })
}

func (c *sftpConn) write(op, subject, h string, off uint64, data []byte) error {
	return c.simple(op, subject, fxpWrite, func(b *sftpBuf) {
		b.str(h)
		b.u64(off)
		b.bytes(data)
	})
}

// read returns up to n bytes at off; eof reports the end of the file.
func (c *sftpConn) read(op, subject, h string, off uint64, n uint32) ([]byte, bool, error) {
	rt, r, err := c.request(op, fxpRead, func(b *sftpBuf) {
		b.str(h)
		b.u64(off)
		b.u32(n)
	})
	if err != nil {
		return nil, false, err
	}
	switch rt {
	case fxpData:
		d := r.str()
		if r.bad || len(d) > int(n) {
			return nil, false, c.protocolErr(op)
		}
		return d, len(d) == 0, nil
	case fxpStatus:
		code, msg := parseStatus(r)
		if code == fxEOF {
			return nil, true, nil
		}
		if code != fxOK {
			return nil, false, sftpStatusError(op, subject, code, msg)
		}
	}
	return nil, false, c.protocolErr(op)
}

func (c *sftpConn) readdir(op, subject, h string) ([]sftpEntry, bool, error) {
	rt, r, err := c.request(op, fxpReaddir, func(b *sftpBuf) { b.str(h) })
	if err != nil {
		return nil, false, err
	}
	switch rt {
	case fxpName:
		count := r.u32()
		if r.bad || uint64(count) > uint64(len(r.b)/12) { // 3 length/flag words per entry at least
			return nil, false, c.protocolErr(op)
		}
		out := make([]sftpEntry, 0, count)
		for i := uint32(0); i < count; i++ {
			name := r.str()
			r.str() // longname, for humans only
			a := r.attrs()
			if r.bad {
				return nil, false, c.protocolErr(op)
			}
			out = append(out, sftpEntry{name: string(name), attrs: a})
		}
		return out, false, nil
	case fxpStatus:
		code, msg := parseStatus(r)
		if code == fxEOF {
			return nil, true, nil
		}
		if code != fxOK {
			return nil, false, sftpStatusError(op, subject, code, msg)
		}
	}
	return nil, false, c.protocolErr(op)
}

func (c *sftpConn) remove(op, p string) error {
	return c.simple(op, p, fxpRemove, func(b *sftpBuf) { b.str(p) })
}

func (c *sftpConn) mkdir(op, p string) error {
	return c.simple(op, p, fxpMkdir, func(b *sftpBuf) {
		b.str(p)
		b.attrs(sftpAttrs{flags: attrPerms, perms: 0o700})
	})
}

// rename replaces to with from: atomically with posix-rename@openssh.com
// when the server offers it, otherwise by removing the target first (a
// failed rename then leaves nothing under the key, never a partial object).
func (c *sftpConn) rename(op, from, to string) error {
	if c.exts[posixRenameExt] == "1" {
		return c.simple(op, to, fxpExtended, func(b *sftpBuf) {
			b.str(posixRenameExt)
			b.str(from)
			b.str(to)
		})
	}
	if err := c.remove(op, to); err != nil && KindOf(err) != KindNotFound {
		return err
	}
	return c.simple(op, to, fxpRename, func(b *sftpBuf) {
		b.str(from)
		b.str(to)
	})
}

// ---- store ------------------------------------------------------------------

type sftpStore struct {
	conn      *sftpConn
	client    *ssh.Client
	session   *ssh.Session
	base      string
	closeOnce sync.Once
}

func (s *sftpStore) remote(key string) string { return path.Join(s.base, key) }

// abort breaks off an in-flight request when the caller's context ends.
func (s *sftpStore) abort() { _ = s.client.Close() }

func ctxError(op string, err error) *Error {
	return newError(KindNetwork, op, "the operation was cancelled or timed out; the result is unknown", err)
}

// guard runs fn with ctx able to break off a blocked request, and reports a
// cancellation as such rather than as a broken connection.
func (s *sftpStore) guard(ctx context.Context, op string, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return ctxError(op, err)
	}
	stop := context.AfterFunc(ctx, s.abort)
	err := fn()
	stop()
	if err != nil && ctx.Err() != nil {
		return ctxError(op, ctx.Err())
	}
	return err
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// tempName is the hidden sibling an upload is written to before it is
// renamed onto its key; List skips every name starting with '.'.
func tempName(name string) string {
	if len(name) > 200 {
		n := 200
		for n > 0 && !utf8.RuneStart(name[n]) {
			n--
		}
		name = name[:n]
	}
	return "." + name + ".partial-" + randomHex(8)
}

func (s *sftpStore) mkdirAll(op, key string) error {
	parts := strings.Split(key, "/")
	cur := s.base
	for _, p := range parts[:len(parts)-1] {
		cur = path.Join(cur, p)
		a, err := s.conn.stat(op, cur, false)
		if err == nil {
			if !a.isDir() {
				return newError(KindConfig, op, cur+" exists on the SFTP server but is not a folder", nil)
			}
			continue
		}
		if KindOf(err) != KindNotFound {
			return err
		}
		if merr := s.conn.mkdir(op, cur); merr != nil {
			// Another writer may have created it meanwhile.
			if a, err := s.conn.stat(op, cur, false); err != nil || !a.isDir() {
				return merr
			}
		}
	}
	return nil
}

func (s *sftpStore) Put(ctx context.Context, key string, r io.Reader) (PutResult, error) {
	const op = "put"
	if !validKey(key) {
		return PutResult{}, newError(KindConfig, op, "invalid object key", nil)
	}
	var res PutResult
	err := s.guard(ctx, op, func() error {
		var err error
		res, err = s.put(ctx, op, key, r)
		return err
	})
	return res, err
}

func (s *sftpStore) put(ctx context.Context, op, key string, r io.Reader) (PutResult, error) {
	if err := s.mkdirAll(op, key); err != nil {
		return PutResult{}, err
	}
	final := s.remote(key)
	tmp := path.Join(path.Dir(final), tempName(path.Base(final)))
	h, err := s.conn.open(op, tmp, fxfWrite|fxfCreat|fxfTrunc, 0o600)
	if err != nil {
		return PutResult{}, err
	}
	handleOpen, committed := true, false
	defer func() {
		if committed {
			return
		}
		if handleOpen {
			_ = s.conn.closeHandle(op, tmp, h)
		}
		_ = s.conn.remove(op, tmp) // best effort; fails harmlessly on a broken connection
	}()

	sum := sha256.New()
	buf := make([]byte, sftpChunk)
	var off uint64
	for {
		if err := ctx.Err(); err != nil {
			return PutResult{}, ctxError(op, err)
		}
		n, rerr := io.ReadFull(r, buf)
		if n > 0 {
			sum.Write(buf[:n])
			if err := s.conn.write(op, final, h, off, buf[:n]); err != nil {
				return PutResult{}, err
			}
			off += uint64(n)
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			return PutResult{}, newError(KindIntegrity, op, "the backup could not be read on this server while uploading; nothing was stored under the key", rerr)
		}
	}
	handleOpen = false
	if err := s.conn.closeHandle(op, final, h); err != nil {
		return PutResult{}, err
	}
	want := hex.EncodeToString(sum.Sum(nil))

	a, err := s.conn.stat(op, tmp, true)
	if err != nil {
		return PutResult{}, err
	}
	if a.flags&attrSize == 0 || a.size != off {
		return PutResult{}, newError(KindIntegrity, op, fmt.Sprintf("the SFTP server stored %d bytes instead of %d for %s; the upload was discarded", a.size, off, key), nil)
	}
	got, n, err := s.hashRemote(ctx, op, tmp, off)
	if err != nil {
		return PutResult{}, err
	}
	if n != off || got != want {
		return PutResult{}, newError(KindIntegrity, op, "the copy read back from the SFTP server differs from what was sent for "+key+"; the upload was discarded", nil)
	}
	if err := s.conn.rename(op, tmp, final); err != nil {
		return PutResult{}, err
	}
	committed = true
	return PutResult{Size: int64(off), SHA256: want, Verified: "size+sha256"}, nil
}

// hashRemote reads p back (at most limit+1 bytes) and hashes it.
func (s *sftpStore) hashRemote(ctx context.Context, op, p string, limit uint64) (string, uint64, error) {
	h, err := s.conn.open(op, p, fxfRead, 0)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = s.conn.closeHandle(op, p, h) }()
	sum := sha256.New()
	var off uint64
	for off <= limit {
		if err := ctx.Err(); err != nil {
			return "", 0, ctxError(op, err)
		}
		data, eof, err := s.conn.read(op, p, h, off, sftpChunk)
		if err != nil {
			return "", 0, err
		}
		if eof {
			break
		}
		sum.Write(data)
		off += uint64(len(data))
	}
	return hex.EncodeToString(sum.Sum(nil)), off, nil
}

func (s *sftpStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	const op = "get"
	if !validKey(key) {
		return nil, newError(KindConfig, op, "invalid object key", nil)
	}
	p := s.remote(key)
	var h string
	err := s.guard(ctx, op, func() error {
		a, err := s.conn.stat(op, p, true)
		if err != nil {
			return err
		}
		if !a.isRegular() {
			return newError(KindNotFound, op, p+" is not a file on the SFTP server", nil)
		}
		h, err = s.conn.open(op, p, fxfRead, 0)
		return err
	})
	if err != nil {
		return nil, err
	}
	rc := &sftpReadCloser{s: s, ctx: ctx, path: p, h: h}
	rc.stop = context.AfterFunc(ctx, s.abort)
	return rc, nil
}

type sftpReadCloser struct {
	s       *sftpStore
	ctx     context.Context
	stop    func() bool
	path    string
	h       string
	off     uint64
	pending []byte
	eof     bool
	closed  bool
}

func (r *sftpReadCloser) Read(p []byte) (int, error) {
	const op = "get"
	if r.closed {
		return 0, newError(KindConfig, op, "read after close", nil)
	}
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		if r.eof {
			return 0, io.EOF
		}
		if err := r.ctx.Err(); err != nil {
			return 0, ctxError(op, err)
		}
		data, eof, err := r.s.conn.read(op, r.path, r.h, r.off, sftpChunk)
		if err != nil {
			if r.ctx.Err() != nil {
				return 0, ctxError(op, r.ctx.Err())
			}
			return 0, err
		}
		if eof {
			r.eof = true
			return 0, io.EOF
		}
		r.off += uint64(len(data))
		r.pending = data
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *sftpReadCloser) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	r.stop()
	return r.s.conn.closeHandle("get", r.path, r.h)
}

func (s *sftpStore) Stat(ctx context.Context, key string) (Object, error) {
	const op = "stat"
	if !validKey(key) {
		return Object{}, newError(KindConfig, op, "invalid object key", nil)
	}
	p := s.remote(key)
	var obj Object
	err := s.guard(ctx, op, func() error {
		a, err := s.conn.stat(op, p, true)
		if err != nil {
			return err
		}
		if !a.isRegular() {
			return newError(KindNotFound, op, p+" is not a file on the SFTP server", nil)
		}
		obj = Object{Key: key, Size: int64(a.size), Modified: a.modified()}
		return nil
	})
	return obj, err
}

// dirCouldMatch: may the folder at key dir hold keys starting with prefix?
func dirCouldMatch(dir, prefix string) bool {
	d := dir + "/"
	return strings.HasPrefix(d, prefix) || strings.HasPrefix(prefix, d)
}

// List walks only the folders that can hold matching keys. Names starting
// with '.' (upload temp files among them) and symbolic links are skipped.
func (s *sftpStore) List(ctx context.Context, prefix string) ([]Object, error) {
	const op = "list"
	if !validPrefix(prefix) {
		return nil, newError(KindConfig, op, "invalid key prefix", nil)
	}
	var out []Object
	scanned := 0
	var walk func(rel string) error
	walk = func(rel string) error {
		dir := s.base
		if rel != "" {
			dir = path.Join(s.base, rel)
		}
		entries, err := s.readDirAll(ctx, op, dir, &scanned)
		if err != nil {
			if rel != "" && KindOf(err) == KindNotFound {
				return nil // removed while listing
			}
			return err
		}
		for _, e := range entries {
			if e.name == "" || e.name[0] == '.' {
				continue
			}
			child := e.name
			if rel != "" {
				child = rel + "/" + e.name
			}
			if !validKey(child) {
				continue
			}
			a := e.attrs
			if a.flags&attrPerms == 0 {
				if la, err := s.conn.stat(op, path.Join(s.base, child), true); err == nil {
					a = la
				}
			}
			switch {
			case a.isLink():
			case a.isDir():
				if dirCouldMatch(child, prefix) {
					if err := walk(child); err != nil {
						return err
					}
				}
			case a.isRegular():
				if strings.HasPrefix(child, prefix) {
					out = append(out, Object{Key: child, Size: int64(a.size), Modified: a.modified()})
				}
			}
		}
		return nil
	}
	if err := s.guard(ctx, op, func() error { return walk("") }); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (s *sftpStore) readDirAll(ctx context.Context, op, dir string, scanned *int) ([]sftpEntry, error) {
	h, err := s.conn.opendir(op, dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.conn.closeHandle(op, dir, h) }()
	var all []sftpEntry
	for {
		if err := ctx.Err(); err != nil {
			return nil, ctxError(op, err)
		}
		batch, eof, err := s.conn.readdir(op, dir, h)
		if err != nil {
			return nil, err
		}
		if eof {
			return all, nil
		}
		for _, e := range batch {
			if e.name == "." || e.name == ".." {
				continue
			}
			*scanned++
			if *scanned > sftpMaxListEntries {
				return nil, newError(KindProvider, op, fmt.Sprintf("too many objects on the SFTP server (more than %d entries under the folder); remove old backups or use a narrower folder", sftpMaxListEntries), nil)
			}
			all = append(all, e)
		}
	}
}

func (s *sftpStore) Delete(ctx context.Context, key string) error {
	const op = "delete"
	if !validKey(key) {
		return newError(KindConfig, op, "invalid object key", nil)
	}
	return s.guard(ctx, op, func() error {
		err := s.conn.remove(op, s.remote(key))
		if KindOf(err) == KindNotFound {
			return nil
		}
		return err
	})
}

func (s *sftpStore) Close() error {
	s.closeOnce.Do(func() {
		_ = s.session.Close()
		_ = s.client.Close()
	})
	return nil
}
