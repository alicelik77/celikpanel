package membership

// Member sessions: the behaviour of http.php's PHP session (files in
// <private>/sessions, strict mode, cookie only, 12-hour lifetime, a new id at sign-in
// and sign-out). PHP's own session files are not read, so a member signs in once
// after the switch; the Go files are named go-sess_<id> and never collide with
// PHP's sess_<id>. Writes go to a temporary file renamed into place (0600).

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const sessionLifetime = 43200 // session.gc_maxlifetime and the 'created' rule

type sessionData struct {
	Created *int64  `json:"created,omitempty"`
	CSRF    string  `json:"csrf,omitempty"`
	Lang    string  `json:"lang,omitempty"`
	Member  *int64  `json:"member,omitempty"`
	Epoch   *int64  `json:"epoch,omitempty"`
	Notice  string  `json:"notice,omitempty"`
	Issued  *Issued `json:"issued,omitempty"`
}

type session struct {
	dir      string
	id       string
	incoming string // the id the browser sent, "" for none
	data     sessionData
	now      func() time.Time
	unlock   func() // releases the lock on the browser's id after the save
}

var sessionID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func newSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("membership: no randomness: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func (s *session) path(id string) string { return filepath.Join(s.dir, "go-sess_"+id) }

// startSession is session_start() with session.use_strict_mode=1: an unknown,
// expired or malformed id gets a fresh one.
func startSession(dir, cookie string, now func() time.Time) *session {
	s := &session{dir: dir, incoming: cookie, now: now, unlock: func() {}}
	if sessionID.MatchString(cookie) {
		if _, err := os.Stat(s.path(cookie)); err == nil {
			// Held until the end of the request; the file is read again under the lock,
			// so a sign-out that finished meanwhile is seen.
			s.unlock = lockSession(dir, cookie)
		}
		if info, err := os.Stat(s.path(cookie)); err == nil && now().Sub(info.ModTime()) <= sessionLifetime*time.Second {
			if raw, err := os.ReadFile(s.path(cookie)); err == nil && json.Unmarshal(raw, &s.data) == nil {
				s.id = cookie
				return s
			}
		} else if err == nil {
			_ = os.Remove(s.path(cookie))
		}
	}
	s.id = newSessionID()
	s.data = sessionData{}
	s.collect()
	return s
}

// regenerate is session_regenerate_id(true): a new id, the old file deleted.
func (s *session) regenerate() {
	_ = os.Remove(s.path(s.id))
	s.id = newSessionID()
}

// cookieChanged says whether the browser must receive the id (PHP sends the cookie
// whenever the id it ends with is not the one it was sent).
func (s *session) cookieChanged() bool { return s.id != s.incoming }

// save writes the session at the end of the request, as PHP does at shutdown.
func (s *session) save() error {
	raw, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".go-sess-tmp-")
	if err != nil {
		return err
	}
	name := temp.Name()
	if _, err := temp.Write(raw); err != nil {
		temp.Close()
		os.Remove(name)
		return err
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		os.Remove(name)
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, s.path(s.id))
}

// collect removes this service's sessions older than the lifetime, on about one in
// a hundred new sessions (PHP's session.gc_probability/gc_divisor default).
func (s *session) collect() {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil || b[0] > 2 {
		return
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "go-sess_") && !strings.HasPrefix(name, ".go-sess-tmp-") && !strings.HasPrefix(name, "go-lock_") {
			continue
		}
		if info, err := entry.Info(); err == nil && s.now().Sub(info.ModTime()) > sessionLifetime*time.Second {
			_ = os.Remove(filepath.Join(s.dir, name))
		}
	}
}
