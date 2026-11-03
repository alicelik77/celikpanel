package remotebackup

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// ---- in-process fake SSH/SFTP server ---------------------------------------

const (
	fakeUser     = "backup"
	fakePassword = "correct horse battery"
)

type fakeSFTPServer struct {
	t           *testing.T
	root        string
	port        int
	hostSigner  ssh.Signer
	clientPub   ssh.PublicKey
	clientPEM   []byte
	posixRename bool
	cfg         *ssh.ServerConfig
	ln          net.Listener

	accepted     atomic.Int32
	authAttempts atomic.Int32
	subsystems   atomic.Int32
	corruptWrite atomic.Bool

	mu    sync.Mutex
	conns []net.Conn
}

func newEd25519Signer(t *testing.T) (ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s, priv
}

func startFakeSFTP(t *testing.T, posixRename bool) *fakeSFTPServer {
	t.Helper()
	s := &fakeSFTPServer{t: t, root: t.TempDir(), posixRename: posixRename}
	s.hostSigner, _ = newEd25519Signer(t)
	clientSigner, clientPriv := newEd25519Signer(t)
	s.clientPub = clientSigner.PublicKey()
	blk, err := ssh.MarshalPrivateKey(clientPriv, "")
	if err != nil {
		t.Fatal(err)
	}
	s.clientPEM = pem.EncodeToMemory(blk)
	if err := os.MkdirAll(filepath.Join(s.root, "backups"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.cfg = &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if c.User() == fakeUser && string(pw) == fakePassword {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == fakeUser && bytes.Equal(key.Marshal(), s.clientPub.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
		AuthLogCallback: func(ssh.ConnMetadata, string, error) { s.authAttempts.Add(1) },
	}
	s.cfg.AddHostKey(s.hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.ln = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	go s.acceptLoop()
	t.Cleanup(func() {
		_ = ln.Close()
		s.mu.Lock()
		for _, c := range s.conns {
			_ = c.Close()
		}
		s.mu.Unlock()
	})
	return s
}

func (s *fakeSFTPServer) acceptLoop() {
	for {
		nc, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.accepted.Add(1)
		s.mu.Lock()
		s.conns = append(s.conns, nc)
		s.mu.Unlock()
		go s.serveConn(nc)
	}
}

func (s *fakeSFTPServer) serveConn(nc net.Conn) {
	defer nc.Close()
	sc, chans, reqs, err := ssh.NewServerConn(nc, s.cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "session" {
			_ = nch.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		ch, chReqs, err := nch.Accept()
		if err != nil {
			continue
		}
		go func() {
			for req := range chReqs {
				ok := req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp"
				if ok {
					s.subsystems.Add(1)
				}
				if req.WantReply {
					_ = req.Reply(ok, nil)
				}
				if ok {
					go s.serveSFTP(ch)
				}
			}
		}()
	}
}

func (s *fakeSFTPServer) local(p string) string {
	return filepath.Join(s.root, filepath.FromSlash(path.Clean("/"+p)))
}

type fakeHandle struct {
	f       *os.File
	entries []os.DirEntry
	dir     bool
	done    bool
}

func fileAttrs(fi fs.FileInfo) sftpAttrs {
	a := sftpAttrs{flags: attrSize | attrPerms | attrTimes, size: uint64(fi.Size())}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		a.perms = modeLink | 0o777
	case fi.IsDir():
		a.perms = modeDir | 0o755
	default:
		a.perms = modeReg | uint32(fi.Mode().Perm())
	}
	a.atime = uint32(fi.ModTime().Unix())
	a.mtime = a.atime
	return a
}

func errCode(err error) uint32 {
	switch {
	case err == nil:
		return fxOK
	case errors.Is(err, fs.ErrNotExist):
		return fxNoSuchFile
	case errors.Is(err, fs.ErrPermission):
		return fxPermissionDenied
	}
	return fxFailure
}

// serveSFTP implements just the SFTP v3 subset the client uses.
func (s *fakeSFTPServer) serveSFTP(ch ssh.Channel) {
	defer ch.Close()
	handles := map[string]*fakeHandle{}
	defer func() {
		for _, h := range handles {
			if h.f != nil {
				_ = h.f.Close()
			}
		}
	}()
	next := 0
	send := func(typ byte, id uint32, fill func(*sftpBuf)) bool {
		var b sftpBuf
		b.u32(0)
		b.u8(typ)
		b.u32(id)
		if fill != nil {
			fill(&b)
		}
		binary.BigEndian.PutUint32(b, uint32(len(b)-4))
		_, err := ch.Write(b)
		return err == nil
	}
	status := func(id uint32, code uint32) bool {
		return send(fxpStatus, id, func(b *sftpBuf) { b.u32(code); b.str("status " + strconv.Itoa(int(code))); b.str("") })
	}
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(ch, hdr[:]); err != nil {
			return
		}
		body := make([]byte, binary.BigEndian.Uint32(hdr[:]))
		if _, err := io.ReadFull(ch, body); err != nil {
			return
		}
		r := &sftpReader{b: body}
		typ := r.u8()
		if typ == fxpInit {
			var b sftpBuf
			b.u32(0)
			b.u8(fxpVersion)
			b.u32(3)
			if s.posixRename {
				b.str(posixRenameExt)
				b.str("1")
			}
			binary.BigEndian.PutUint32(b, uint32(len(b)-4))
			if _, err := ch.Write(b); err != nil {
				return
			}
			continue
		}
		id := r.u32()
		ok := true
		switch typ {
		case fxpOpen:
			name := string(r.str())
			pf := r.u32()
			r.attrs()
			fl := 0
			switch {
			case pf&fxfRead != 0 && pf&fxfWrite != 0:
				fl = os.O_RDWR
			case pf&fxfWrite != 0:
				fl = os.O_WRONLY
			}
			if pf&fxfCreat != 0 {
				fl |= os.O_CREATE
			}
			if pf&fxfTrunc != 0 {
				fl |= os.O_TRUNC
			}
			if pf&fxfExcl != 0 {
				fl |= os.O_EXCL
			}
			f, err := os.OpenFile(s.local(name), fl, 0o600)
			if err != nil {
				ok = status(id, errCode(err))
				break
			}
			next++
			hid := "h" + strconv.Itoa(next)
			handles[hid] = &fakeHandle{f: f}
			ok = send(fxpHandle, id, func(b *sftpBuf) { b.str(hid) })
		case fxpOpendir:
			name := string(r.str())
			ents, err := os.ReadDir(s.local(name))
			if err != nil {
				ok = status(id, errCode(err))
				break
			}
			next++
			hid := "d" + strconv.Itoa(next)
			handles[hid] = &fakeHandle{dir: true, entries: ents}
			ok = send(fxpHandle, id, func(b *sftpBuf) { b.str(hid) })
		case fxpClose:
			hid := string(r.str())
			h := handles[hid]
			if h == nil {
				ok = status(id, fxFailure)
				break
			}
			delete(handles, hid)
			var err error
			if h.f != nil {
				err = h.f.Close()
			}
			ok = status(id, errCode(err))
		case fxpRead:
			h := handles[string(r.str())]
			off, n := r.u64(), r.u32()
			if h == nil || h.f == nil {
				ok = status(id, fxFailure)
				break
			}
			if n > 64*1024 {
				n = 64 * 1024
			}
			buf := make([]byte, n)
			got, err := h.f.ReadAt(buf, int64(off))
			if got == 0 {
				if err == io.EOF {
					ok = status(id, fxEOF)
				} else {
					ok = status(id, errCode(err))
				}
				break
			}
			ok = send(fxpData, id, func(b *sftpBuf) { b.bytes(buf[:got]) })
		case fxpWrite:
			h := handles[string(r.str())]
			off := r.u64()
			data := append([]byte(nil), r.str()...)
			if h == nil || h.f == nil {
				ok = status(id, fxFailure)
				break
			}
			if len(data) > 0 && s.corruptWrite.CompareAndSwap(true, false) {
				data[len(data)/2] ^= 0xff
			}
			_, err := h.f.WriteAt(data, int64(off))
			ok = status(id, errCode(err))
		case fxpStat, fxpLstat:
			name := string(r.str())
			var fi fs.FileInfo
			var err error
			if typ == fxpStat {
				fi, err = os.Stat(s.local(name))
			} else {
				fi, err = os.Lstat(s.local(name))
			}
			if err != nil {
				ok = status(id, errCode(err))
				break
			}
			ok = send(fxpAttrs, id, func(b *sftpBuf) { b.attrs(fileAttrs(fi)) })
		case fxpReaddir:
			h := handles[string(r.str())]
			if h == nil || !h.dir {
				ok = status(id, fxFailure)
				break
			}
			if h.done {
				ok = status(id, fxEOF)
				break
			}
			h.done = true
			ok = send(fxpName, id, func(b *sftpBuf) {
				b.u32(uint32(len(h.entries)))
				for _, e := range h.entries {
					fi, err := e.Info()
					a := sftpAttrs{}
					if err == nil {
						a = fileAttrs(fi)
					}
					b.str(e.Name())
					b.str(e.Name())
					b.attrs(a)
				}
			})
		case fxpRemove:
			p := s.local(string(r.str()))
			fi, err := os.Lstat(p)
			if err == nil && fi.IsDir() {
				ok = status(id, fxFailure)
				break
			}
			if err == nil {
				err = os.Remove(p)
			}
			ok = status(id, errCode(err))
		case fxpMkdir:
			p := s.local(string(r.str()))
			r.attrs()
			ok = status(id, errCode(os.Mkdir(p, 0o755)))
		case fxpRename:
			from, to := s.local(string(r.str())), s.local(string(r.str()))
			if _, err := os.Lstat(to); err == nil {
				ok = status(id, fxFailure) // v3 RENAME never replaces
				break
			}
			ok = status(id, errCode(os.Rename(from, to)))
		case fxpExtended:
			name := string(r.str())
			if name != posixRenameExt || !s.posixRename {
				ok = status(id, fxOpUnsupported)
				break
			}
			from, to := s.local(string(r.str())), s.local(string(r.str()))
			ok = status(id, errCode(os.Rename(from, to)))
		default:
			ok = status(id, fxOpUnsupported)
		}
		if !ok {
			return
		}
	}
}

func (s *fakeSFTPServer) hostKeyLine() string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.hostSigner.PublicKey())))
}

func (s *fakeSFTPServer) config() SFTPConfig {
	return SFTPConfig{
		Host:     "127.0.0.1",
		Port:     s.port,
		User:     fakeUser,
		Password: fakePassword,
		BasePath: "/backups",
		HostKey:  s.hostKeyLine(),
		Timeout:  5 * time.Second,
	}
}

func testCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func wantKind(t *testing.T, err error, k Kind) {
	t.Helper()
	if err == nil {
		t.Fatalf("got no error, want kind %q", k)
	}
	if got := KindOf(err); got != k {
		t.Fatalf("got kind %q (%v), want %q", got, err, k)
	}
}

// ---- tests --------------------------------------------------------------------

func TestSFTPProbeReturnsHostKey(t *testing.T) {
	srv := startFakeSFTP(t, true)
	got, err := ProbeSFTPHostKey(testCtx(t), "127.0.0.1", srv.port, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pub := srv.hostSigner.PublicKey()
	if got.Fingerprint != ssh.FingerprintSHA256(pub) {
		t.Fatalf("fingerprint %q, want %q", got.Fingerprint, ssh.FingerprintSHA256(pub))
	}
	if got.Key != srv.hostKeyLine() {
		t.Fatalf("key %q, want %q", got.Key, srv.hostKeyLine())
	}
	if n := srv.authAttempts.Load(); n != 0 {
		t.Fatalf("probe attempted authentication %d times", n)
	}
}

func TestSFTPUnpinnedHostKeyRefused(t *testing.T) {
	srv := startFakeSFTP(t, true)
	ctx := testCtx(t)
	probe, err := ProbeSFTPHostKey(ctx, "127.0.0.1", srv.port, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cfg := srv.config()
	cfg.HostKey = ""
	st, err := DialSFTP(ctx, cfg)
	if st != nil {
		t.Fatal("store returned with an unpinned host key")
	}
	wantKind(t, err, KindHostKey)
	var hm *HostKeyMismatch
	if !errors.As(err, &hm) {
		t.Fatalf("no HostKeyMismatch in %v", err)
	}
	if hm.Pinned != "" || hm.Presented != probe {
		t.Fatalf("mismatch %+v, want presented %+v", hm, probe)
	}
	if strings.Contains(err.Error(), fakePassword) {
		t.Fatal("error leaks the password")
	}
	if n := srv.authAttempts.Load(); n != 0 {
		t.Fatalf("%d authentication attempts reached the server", n)
	}
	if n := srv.subsystems.Load(); n != 0 {
		t.Fatalf("%d sftp subsystem requests reached the server", n)
	}
}

func TestSFTPChangedHostKeyRefused(t *testing.T) {
	srv := startFakeSFTP(t, true)
	other, _ := newEd25519Signer(t)
	cfg := srv.config()
	cfg.HostKey = string(ssh.MarshalAuthorizedKey(other.PublicKey())) // with newline, as pasted
	_, err := DialSFTP(testCtx(t), cfg)
	wantKind(t, err, KindHostKey)
	var hm *HostKeyMismatch
	if !errors.As(err, &hm) {
		t.Fatalf("no HostKeyMismatch in %v", err)
	}
	if hm.Presented.Fingerprint != ssh.FingerprintSHA256(srv.hostSigner.PublicKey()) {
		t.Fatalf("presented %+v", hm.Presented)
	}
	if hm.Pinned != strings.TrimSpace(string(ssh.MarshalAuthorizedKey(other.PublicKey()))) {
		t.Fatalf("pinned %q", hm.Pinned)
	}
	if n := srv.authAttempts.Load(); n != 0 {
		t.Fatalf("%d authentication attempts reached the server", n)
	}
}

func TestSFTPWrongPassword(t *testing.T) {
	srv := startFakeSFTP(t, true)
	cfg := srv.config()
	cfg.Password = "wrong-password-xyz"
	_, err := DialSFTP(testCtx(t), cfg)
	wantKind(t, err, KindCredentials)
	if strings.Contains(err.Error(), "wrong-password-xyz") {
		t.Fatal("error leaks the password")
	}
}

func TestSFTPRoundTrip(t *testing.T) {
	cases := []struct {
		name        string
		posixRename bool
		privateKey  bool
	}{
		{"posix-rename/password", true, false},
		{"plain-rename/password", false, false},
		{"posix-rename/private-key", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := startFakeSFTP(t, tc.posixRename)
			cfg := srv.config()
			if tc.privateKey {
				cfg.Password = ""
				cfg.PrivateKey = srv.clientPEM
			}
			ctx := testCtx(t)
			st, err := DialSFTP(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()

			payload := make([]byte, 200*1024+123)
			if _, err := rand.Read(payload); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(payload)
			key := "sites/a.example-d1/manual/x.cpbak.gpg"

			// Write once with other content, then overwrite: exercises the
			// replace path of both rename variants.
			if _, err := st.Put(ctx, key, bytes.NewReader([]byte("old"))); err != nil {
				t.Fatal(err)
			}
			res, err := st.Put(ctx, key, bytes.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			if res.Size != int64(len(payload)) || res.SHA256 != hex.EncodeToString(sum[:]) || res.Verified != "size+sha256" {
				t.Fatalf("put result %+v", res)
			}

			// Noise the listing must skip or exclude.
			dir := filepath.Join(srv.root, "backups", "sites", "a.example-d1", "manual")
			if err := os.WriteFile(filepath.Join(dir, ".x.cpbak.gpg.partial-deadbeef"), []byte("tmp"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(srv.root, "backups", "other"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(srv.root, "backups", "other", "z"), []byte("z"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(dir, "x.cpbak.gpg"), filepath.Join(dir, "link.gpg")); err != nil {
				t.Logf("symlink not created (%v); symlink skipping not exercised", err)
			}

			obj, err := st.Stat(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			if obj.Key != key || obj.Size != int64(len(payload)) || obj.Modified.IsZero() {
				t.Fatalf("stat %+v", obj)
			}

			for _, prefix := range []string{"sites/", "sites/a", ""} {
				list, err := st.List(ctx, prefix)
				if err != nil {
					t.Fatal(err)
				}
				var keys []string
				for _, o := range list {
					keys = append(keys, o.Key)
				}
				want := []string{key}
				if prefix == "" {
					want = []string{"other/z", key}
				}
				if strings.Join(keys, ",") != strings.Join(want, ",") {
					t.Fatalf("List(%q) = %v, want %v", prefix, keys, want)
				}
			}
			if list, err := st.List(ctx, "sites/b"); err != nil || len(list) != 0 {
				t.Fatalf("List(sites/b) = %v, %v", list, err)
			}

			rc, err := st.Get(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatal(err)
			}
			if err := rc.Close(); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("get returned %d bytes, differing from the %d sent", len(got), len(payload))
			}

			if err := st.Delete(ctx, key); err != nil {
				t.Fatal(err)
			}
			if err := st.Delete(ctx, key); err != nil {
				t.Fatalf("second delete: %v", err)
			}
			_, err = st.Stat(ctx, key)
			wantKind(t, err, KindNotFound)
			_, err = st.Get(ctx, key)
			wantKind(t, err, KindNotFound)
		})
	}
}

func TestSFTPMissingBasePath(t *testing.T) {
	srv := startFakeSFTP(t, true)
	cfg := srv.config()
	cfg.BasePath = "/missing/folder"
	_, err := DialSFTP(testCtx(t), cfg)
	wantKind(t, err, KindNotFound)
	if _, statErr := os.Stat(filepath.Join(srv.root, "missing")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatal("DialSFTP created the base path")
	}
}

func TestSFTPConfigValidation(t *testing.T) {
	srv := startFakeSFTP(t, true)
	cases := map[string]func(*SFTPConfig){
		"port too big":     func(c *SFTPConfig) { c.Port = 70000 },
		"negative port":    func(c *SFTPConfig) { c.Port = -1 },
		"empty host":       func(c *SFTPConfig) { c.Host = "" },
		"host with scheme": func(c *SFTPConfig) { c.Host = "sftp://127.0.0.1" },
		"host with user":   func(c *SFTPConfig) { c.Host = "u@127.0.0.1" },
		"host with port":   func(c *SFTPConfig) { c.Host = "localhost:22" },
		"host with space":  func(c *SFTPConfig) { c.Host = "127.0.0.1 x" },
		"relative base":    func(c *SFTPConfig) { c.BasePath = "backups" },
		"unclean base":     func(c *SFTPConfig) { c.BasePath = "/backups/../etc" },
		"trailing slash":   func(c *SFTPConfig) { c.BasePath = "/backups/" },
		"empty user":       func(c *SFTPConfig) { c.User = "" },
		"no credentials":   func(c *SFTPConfig) { c.Password = "" },
		"bad private key": func(c *SFTPConfig) {
			c.Password = ""
			c.PrivateKey = []byte("-----BEGIN X-----\nnope\n-----END X-----\n")
		},
		"bad pinned key": func(c *SFTPConfig) { c.HostKey = "ssh-ed25519 not-base64" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := srv.config()
			mutate(&cfg)
			_, err := DialSFTP(testCtx(t), cfg)
			wantKind(t, err, KindConfig)
		})
	}
	if _, err := ProbeSFTPHostKey(testCtx(t), "127.0.0.1", 0x10000, time.Second); KindOf(err) != KindConfig {
		t.Fatalf("probe with bad port: %v", err)
	}
	if n := srv.accepted.Load(); n != 0 {
		t.Fatalf("validation failures still connected %d times", n)
	}
}

func TestSFTPIntegrityFailure(t *testing.T) {
	srv := startFakeSFTP(t, true)
	ctx := testCtx(t)
	st, err := DialSFTP(ctx, srv.config())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	payload := bytes.Repeat([]byte("celikpanel"), 10000)
	key := "sites/b/manual/y.cpbak.gpg"
	srv.corruptWrite.Store(true)
	_, err = st.Put(ctx, key, bytes.NewReader(payload))
	wantKind(t, err, KindIntegrity)
	_, err = st.Stat(ctx, key)
	wantKind(t, err, KindNotFound)
	err = filepath.WalkDir(filepath.Join(srv.root, "backups"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			t.Errorf("left behind %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The store stays usable after a verified failure.
	if _, err := st.Put(ctx, key, bytes.NewReader(payload)); err != nil {
		t.Fatalf("put after integrity failure: %v", err)
	}
}

func TestSFTPInvalidKeys(t *testing.T) {
	srv := startFakeSFTP(t, true)
	ctx := testCtx(t)
	st, err := DialSFTP(ctx, srv.config())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, k := range []string{"", "/abs", "a/../b", "a//b", "a/"} {
		if _, err := st.Put(ctx, k, bytes.NewReader(nil)); KindOf(err) != KindConfig {
			t.Fatalf("Put(%q): %v", k, err)
		}
	}
	if _, err := st.List(ctx, "../"); KindOf(err) != KindConfig {
		t.Fatalf("List(../): %v", err)
	}
}
