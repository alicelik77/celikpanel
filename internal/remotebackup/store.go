// Package remotebackup sends finished CelikPanel backups to storage the owner
// controls elsewhere: S3-compatible object storage or an SFTP server. It is
// pure Go (standard library and golang.org/x/crypto), so the Panel never
// shells out to a third-party tool.
//
// Every object is encrypted before it leaves the server (OpenPGP symmetric,
// see crypt.go), so it can be opened without CelikPanel by the distribution's
// own `gpg`. The layout and the side file are documented in
// docs/REMOTE-BACKUPS.md.
//
// Uzak yedek: biten yedekler sahibin başka yerdeki depolamasına (S3 uyumlu
// nesne depolama ya da SFTP) gönderilir. Her nesne sunucudan çıkmadan
// şifrelenir ve CelikPanel olmadan dağıtımın kendi gpg aracıyla açılabilir.
package remotebackup

import (
	"context"
	"errors"
	"io"
	"time"
)

// Object is one stored object, as a listing reports it.
type Object struct {
	Key      string    // relative to the destination's base (prefix or path)
	Size     int64     // bytes
	Modified time.Time // as the storage reports it; may be zero
}

// PutResult is what an upload proved.
type PutResult struct {
	Size   int64  // bytes stored, confirmed by the storage after the upload
	SHA256 string // lowercase hex SHA-256 of exactly the bytes uploaded
	// Verified names how the content was checked after the upload:
	// "size+sha256" (read back and hashed), "size+md5" (the storage's own
	// checksum of the parts), or "size" when only the size could be confirmed.
	Verified string
}

// Store is one remote destination. Keys are '/'-separated, relative to the
// destination's base, never start with '/' and never contain "..".
//
// Put stores exactly the bytes of r under key. It must not leave a partial
// object under key: a failed Put either leaves the previous object (if any)
// or nothing. It verifies the result before it returns (at least the size).
// Get opens the object; the caller closes it. Stat and Get return an error of
// kind KindNotFound for a missing key. List returns every object whose key
// starts with prefix (recursively). Delete of a missing key is not an error.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader) (PutResult, error)
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Stat(ctx context.Context, key string) (Object, error)
	List(ctx context.Context, prefix string) ([]Object, error)
	Delete(ctx context.Context, key string) error
	Close() error
}

// Kind classifies a failure for the operation guidance (D-024): who must act
// and whether the result is known.
type Kind string

const (
	// KindCredentials: the storage refused the key, secret, password or
	// private key. The destination's owner fixes the credentials.
	KindCredentials Kind = "credentials"
	// KindPermission: signed in, but not allowed (bucket policy, folder
	// permissions). The destination's owner fixes the rights at the provider.
	KindPermission Kind = "permission"
	// KindNotFound: the bucket, folder or object does not exist.
	KindNotFound Kind = "not_found"
	// KindHostKey: the SFTP server's host key is not the pinned one, or none
	// is pinned yet. Nothing was sent. A person confirms the new fingerprint.
	KindHostKey Kind = "host_key"
	// KindNetwork: the storage could not be reached or the connection broke;
	// the result of the last request is unknown. Retried automatically.
	KindNetwork Kind = "network"
	// KindProvider: the storage answered with a server-side error (5xx);
	// unknown result, retried automatically.
	KindProvider Kind = "provider"
	// KindQuota: the storage is full or the account's limit is reached.
	KindQuota Kind = "quota"
	// KindClock: the storage refused the request time (this server's clock).
	KindClock Kind = "clock"
	// KindIntegrity: what is stored is not what was sent (size or checksum),
	// or a downloaded object fails its check. A verified failure.
	KindIntegrity Kind = "integrity"
	// KindConfig: the destination's settings are invalid (bad endpoint,
	// bucket name, path, key format). Checked before any network use.
	KindConfig Kind = "config"
	// KindDecrypt: the passphrase does not open the object, or the object is
	// damaged.
	KindDecrypt Kind = "decrypt"
)

// Error is a classified failure. Message is safe to show and to log: it
// never carries a secret, a passphrase, a signature or a password.
type Error struct {
	Kind    Kind
	Op      string // "put", "get", "list", "delete", "stat", "connect"...
	Message string
	Err     error // the underlying error; may be nil. Must be secret-free too.
}

func (e *Error) Error() string {
	if e.Op != "" {
		return e.Op + ": " + e.Message
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// KindOf returns the kind of err, KindNetwork for a deadline or cancellation
// (the result is unknown), and "" for nil.
func KindOf(err error) Kind {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return KindNetwork
	}
	return KindNetwork
}

// Transient reports whether retrying the same request may succeed without a
// person acting first.
func Transient(err error) bool {
	switch KindOf(err) {
	case KindNetwork, KindProvider:
		return true
	}
	return false
}

func newError(kind Kind, op, message string, err error) *Error {
	return &Error{Kind: kind, Op: op, Message: message, Err: err}
}

// validKey: relative, '/'-separated, no empty, "." or ".." element, bounded.
func validKey(key string) bool {
	if key == "" || len(key) > 900 || key[0] == '/' || key[len(key)-1] == '/' {
		return false
	}
	start := 0
	for i := 0; i <= len(key); i++ {
		if i == len(key) || key[i] == '/' {
			part := key[start:i]
			if part == "" || part == "." || part == ".." {
				return false
			}
			start = i + 1
			continue
		}
		c := key[i]
		if c < 0x20 || c == 0x7f || c == '\\' {
			return false
		}
	}
	return true
}

// validPrefix: like validKey but may be empty (list everything).
func validPrefix(prefix string) bool {
	if prefix == "" {
		return true
	}
	if prefix[len(prefix)-1] == '/' {
		prefix = prefix[:len(prefix)-1]
	}
	return validKey(prefix)
}
