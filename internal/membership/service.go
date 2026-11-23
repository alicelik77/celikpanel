// Package membership is the Go member service: a port of portal-membership/ (PHP)
// at exact parity. It opens the same SQLite file in place, signs with the same
// licence key, answers installed panels with byte-equal receipts and serves the
// same account pages in Turkish and English.
//
// Port of portal-membership/src/Service.php (origin/main 2584a566e, 2026-11-23).
// Every refusal code, limit, query and transaction boundary follows that file; the
// comments name the PHP line where the reason is not obvious. Central state only:
// never installed on a customer's panel server.
//
// Uye hizmeti: portal-membership/ (PHP) dizininin birebir Go karsiligi. Ayni SQLite
// dosyasini yerinde acar, ayni lisans anahtariyla imzalar, panellere bayt bayt ayni
// makbuzu verir.
package membership

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/nacl/secretbox"

	_ "modernc.org/sqlite"
)

// Problem is a refusal with the code PHP's Problem exception carries; the pages and
// the panel API show only the code.
type Problem string

func (p Problem) Error() string { return string(p) }

// The refusal codes of Service.php and http.php.
const (
	ErrRateLimited            Problem = "rate_limited"
	ErrInvalidEmail           Problem = "invalid_email"
	ErrPasswordLength         Problem = "password_length"
	ErrInvalidRequest         Problem = "invalid_request"
	ErrInvalidToken           Problem = "invalid_token"
	ErrInvalidLogin           Problem = "invalid_login"
	ErrVerifyEmail            Problem = "verify_email"
	ErrAuthenticationRequired Problem = "authentication_required"
	ErrLicenseNotFound        Problem = "license_not_found"
	ErrKeyNotSaved            Problem = "key_not_saved"
	ErrRenewalNotDue          Problem = "renewal_not_due"
	ErrInvalidServerIP        Problem = "invalid_server_ip"
	ErrInvalidActivation      Problem = "invalid_activation"
	ErrInvalidLicense         Problem = "invalid_license"
	ErrLicenseInUse           Problem = "license_in_use"
	ErrLicenseExpired         Problem = "license_expired"
	ErrMailUnavailable        Problem = "mail_unavailable"
	ErrSessionChanged         Problem = "session_changed"
	ErrAcceptTerms            Problem = "accept_terms"
)

// RefreshAfter: a panel in use asks again after this many seconds (Service.php
// REFRESH_AFTER). OfflineGrace: one successful check keeps an unreachable panel
// manageable this long, never past the annual expiry (D-039, OFFLINE_GRACE).
const (
	RefreshAfter = 3600
	OfflineGrace = 7 * 86400
)

// Sender delivers a verification or reset token to an address. An error becomes
// mail_unavailable for the member (the PHP closure throws Problem('mail_unavailable')).
type Sender func(email, purpose, token string) error

// Service is Service.php: central state only.
type Service struct {
	db         *sql.DB
	signingKey ed25519.PrivateKey
	send       Sender
	clock      func() int64
}

// schema is Service.php's CREATE TABLE text, byte for byte: SQLite stores each
// statement's text in sqlite_master, so a file created by either service is the same.
var schema = []string{
	"CREATE TABLE IF NOT EXISTS members (\n id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE, password TEXT NOT NULL,\n verified INTEGER NOT NULL DEFAULT 0, epoch INTEGER NOT NULL DEFAULT 1, created INTEGER NOT NULL,\n accepted_terms TEXT NOT NULL DEFAULT '2026-09-09'\n)",
	"CREATE TABLE IF NOT EXISTS tokens (\n hash TEXT PRIMARY KEY, member_id INTEGER NOT NULL REFERENCES members(id),\n purpose TEXT NOT NULL CHECK(purpose IN ('verify','reset')), expires INTEGER NOT NULL\n)",
	"CREATE TABLE IF NOT EXISTS licenses (\n id TEXT PRIMARY KEY, member_id INTEGER NOT NULL REFERENCES members(id),\n key_hash TEXT NOT NULL UNIQUE, key_suffix TEXT NOT NULL, created INTEGER NOT NULL,\n activated INTEGER, expires INTEGER, server_id TEXT, hostname TEXT,\n generation INTEGER NOT NULL DEFAULT 1\n)",
	"CREATE TABLE IF NOT EXISTS events (\n id INTEGER PRIMARY KEY, member_id INTEGER NOT NULL REFERENCES members(id),\n license_id TEXT, kind TEXT NOT NULL, created INTEGER NOT NULL\n)",
	"CREATE TABLE IF NOT EXISTS rate_limits (\n bucket TEXT PRIMARY KEY, count INTEGER NOT NULL, expires INTEGER NOT NULL\n)",
}

// fileURI is the database address. SQLite reads it as a URI, so "%", "?" and "#"
// in the path are escaped; anything else stands as it is.
func fileURI(path string) string {
	return "file:" + strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(path)
}

// dsn opens the file the way PHP's PDO connection is set up (Service.php:26):
// foreign keys on, 5 s busy timeout, WAL, synchronous FULL; every transaction
// starts with BEGIN IMMEDIATE like Service::atomic(). mode=rw opens an existing
// file only; rwc may create it.
func dsn(path, mode string) string {
	return fileURI(path) + "?mode=" + mode + "&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(FULL)&_txlock=immediate"
}

// ErrNoDatabase: the file is missing, or holds no licences table. The service
// answers 503 (panels keep their receipts and retry) instead of making an empty
// database whose answers would reject every installed panel.
var ErrNoDatabase = errors.New("membership: the members database is missing or has no licences table")

// OpenExisting is the service's own way in (cgi, serve): the file and its licences
// table must already exist; nothing is created when they do not. A file the PHP
// service made is migrated exactly as PHP would (additive columns only).
func OpenExisting(path string, signingKey []byte, send Sender, clock func() int64) (*Service, error) {
	if len(signingKey) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid license signing key")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, ErrNoDatabase
	}
	db, err := sql.Open("sqlite", dsn(path, "rw"))
	if err != nil {
		return nil, ErrNoDatabase
	}
	db.SetMaxOpenConns(1)
	var tables int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='licenses'").Scan(&tables); err != nil || tables != 1 {
		db.Close()
		return nil, ErrNoDatabase
	}
	return finishOpen(db, signingKey, send, clock)
}

// Open is `new Service($database, $signingKey, $send, $clock)`: it creates the file
// and missing tables and applies the same additive migration. Tests and an
// explicit initialisation use it; the service itself uses OpenExisting. clock nil
// means the wall clock.
func Open(path string, signingKey []byte, send Sender, clock func() int64) (*Service, error) {
	if len(signingKey) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid license signing key")
	}
	db, err := sql.Open("sqlite", dsn(path, "rwc"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return finishOpen(db, signingKey, send, clock)
}

func finishOpen(db *sql.DB, signingKey []byte, send Sender, clock func() int64) (*Service, error) {
	if clock == nil {
		clock = func() int64 { return time.Now().Unix() }
	}
	s := &Service{db: db, signingKey: ed25519.PrivateKey(append([]byte(nil), signingKey...)), send: send, clock: clock}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database.
func (s *Service) Close() error { return s.db.Close() }

// DB is the open database, for tests and the read-only checks.
func (s *Service) DB() *sql.DB { return s.db }

func (s *Service) migrate() error {
	ctx := context.Background()
	for _, statement := range schema {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	columns, err := tableColumns(ctx, s.db, "members")
	if err != nil {
		return err
	}
	if !columns["accepted_terms"] {
		if _, err := s.db.ExecContext(ctx, "ALTER TABLE members ADD COLUMN accepted_terms TEXT NOT NULL DEFAULT '2026-09-09'"); err != nil {
			return err
		}
	}
	return s.atomic(func(tx *sql.Tx) error {
		columns, err := tableColumns(ctx, tx, "licenses")
		if err != nil {
			return err
		}
		for _, add := range []struct{ name, sql string }{
			{"key_encrypted", "ALTER TABLE licenses ADD COLUMN key_encrypted TEXT"},
			{"server_ip", "ALTER TABLE licenses ADD COLUMN server_ip TEXT"},
			{"removed", "ALTER TABLE licenses ADD COLUMN removed INTEGER"},
		} {
			if !columns[add.name] {
				if _, err := tx.ExecContext(ctx, add.sql); err != nil {
					return err
				}
			}
		}
		rows, err := tx.QueryContext(ctx, "SELECT id,created FROM licenses WHERE expires IS NULL")
		if err != nil {
			return err
		}
		type pending struct {
			id      string
			created int64
		}
		var fill []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.id, &p.created); err != nil {
				rows.Close()
				return err
			}
			fill = append(fill, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, p := range fill {
			if _, err := tx.ExecContext(ctx, "UPDATE licenses SET expires=? WHERE id=?", anniversary(p.created), p.id); err != nil {
				return err
			}
		}
		return nil
	})
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func tableColumns(ctx context.Context, q querier, table string) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// Now is the service clock (Service::now()).
func (s *Service) Now() int64 { return s.clock() }

func sha256Hex(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("membership: no randomness: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// atomic is Service::atomic(): BEGIN IMMEDIATE, then COMMIT or ROLLBACK.
func (s *Service) atomic(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Limit is Service::limit(): at most max hits per bucket in a window that starts
// with the first hit; the bucket is stored only as its SHA-256.
func (s *Service) Limit(bucket string, max int64, seconds int64) error {
	allowed := false
	err := s.atomic(func(tx *sql.Tx) error {
		ctx := context.Background()
		now := s.Now()
		if _, err := tx.ExecContext(ctx, "DELETE FROM rate_limits WHERE expires<=?", now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO rate_limits(bucket,count,expires) VALUES(?,1,?) ON CONFLICT(bucket) DO UPDATE SET count=count+1",
			sha256Hex(bucket), now+seconds); err != nil {
			return err
		}
		var count int64
		if err := tx.QueryRowContext(ctx, "SELECT count FROM rate_limits WHERE bucket=?", sha256Hex(bucket)).Scan(&count); err != nil {
			return err
		}
		allowed = count <= max
		return nil
	})
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}
	return nil
}

var printableASCII = regexp.MustCompile(`^[\x21-\x7e]*$`)

// normalEmail is Service::email(): lower-cased, trimmed, at most 254 bytes, valid
// for FILTER_VALIDATE_EMAIL and printable ASCII only.
func normalEmail(email string) (string, error) {
	email = phpLower(phpTrim(email))
	if len(email) > 254 || !validEmail(email) || !printableASCII.MatchString(email) {
		return "", ErrInvalidEmail
	}
	return email, nil
}

func checkPassword(password string) error {
	if len(password) < 12 || len(password) > 128 || strings.Contains(password, "\x00") {
		return ErrPasswordLength
	}
	return nil
}

// Register: the owner chooses the password only after proving the address. A
// duplicate registration never replaces the password of an unverified account.
func (s *Service) Register(email string) error {
	email, err := normalEmail(email)
	if err != nil {
		return err
	}
	hash, err := hashPassword(randomHex(32))
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(context.Background(),
		"INSERT INTO members(email,password,created,accepted_terms) VALUES(?,?,?,?) ON CONFLICT(email) DO NOTHING",
		email, hash, s.Now(), "2026-10-30"); err != nil {
		return err
	}
	return s.RequestToken(email, "verify")
}

// RequestToken mails a verify or reset token; it answers the same whether or not
// the address is eligible.
func (s *Service) RequestToken(email, purpose string) error {
	email, err := normalEmail(email)
	if err != nil {
		return err
	}
	if purpose != "verify" && purpose != "reset" {
		return ErrInvalidRequest
	}
	if err := s.Limit("mail:"+email, 3, 3600); err != nil {
		return err
	}
	ctx := context.Background()
	var id, verified int64
	err = s.db.QueryRowContext(ctx, "SELECT id,verified FROM members WHERE email=?", email).Scan(&id, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if purpose == "verify" && verified == 1 {
		return nil
	}
	token := randomHex(32)
	if err := s.atomic(func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM tokens WHERE (member_id=? AND purpose=?) OR expires<=?", id, purpose, s.Now()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO tokens(hash,member_id,purpose,expires) VALUES(?,?,?,?)", sha256Hex(token), id, purpose, s.Now()+1800)
		return err
	}); err != nil {
		return err
	}
	// The fragment never enters HTTP access logs. The browser submits it via POST.
	if s.send == nil {
		return nil
	}
	if err := s.send(email, purpose, token); err != nil {
		var problem Problem
		if errors.As(err, &problem) {
			return problem
		}
		return ErrMailUnavailable
	}
	return nil
}

var hex64 = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ConsumeToken verifies the address (choosing the password) or resets the
// password; either way every existing session ends (epoch+1).
func (s *Service) ConsumeToken(token, purpose, newPassword string) error {
	if !hex64.MatchString(token) || (purpose != "verify" && purpose != "reset") {
		return ErrInvalidToken
	}
	if err := checkPassword(newPassword); err != nil {
		return err
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		var member int64
		err := tx.QueryRowContext(ctx, "SELECT member_id FROM tokens WHERE hash=? AND purpose=? AND expires>?", sha256Hex(token), purpose, s.Now()).Scan(&member)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidToken
		}
		if err != nil {
			return err
		}
		if purpose == "verify" {
			_, err = tx.ExecContext(ctx, "UPDATE members SET verified=1,password=?,epoch=epoch+1 WHERE id=?", hash, member)
		} else {
			_, err = tx.ExecContext(ctx, "UPDATE members SET password=?,epoch=epoch+1 WHERE id=?", hash, member)
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM tokens WHERE member_id=? AND purpose=?", member, purpose)
		return err
	})
}

// Member is a signed-in member as the session carries it.
type Member struct {
	ID    int64
	Email string
	Epoch int64
}

// Login checks the password; an unknown address costs the same work.
func (s *Service) Login(email, password string) (Member, error) {
	email, err := normalEmail(email)
	if err != nil {
		return Member{}, err
	}
	if err := s.Limit("login:"+email, 10, 900); err != nil {
		return Member{}, err
	}
	var m Member
	var hash string
	var verified int64
	err = s.db.QueryRowContext(context.Background(), "SELECT id,email,epoch,password,verified FROM members WHERE email=?", email).
		Scan(&m.ID, &m.Email, &m.Epoch, &hash, &verified)
	found := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Member{}, err
	}
	candidate := password
	if len(candidate) > 129 {
		candidate = candidate[:129] // PHP: password_verify(substr($password,0,129), ...)
	}
	if !found {
		hash = dummyHash
	}
	valid, err := verifyPassword(candidate, hash)
	if err != nil {
		return Member{}, err
	}
	if !found || !valid || len(password) > 128 {
		return Member{}, ErrInvalidLogin
	}
	if verified == 0 {
		return Member{}, ErrVerifyEmail
	}
	return m, nil
}

// MemberByEpoch is Service::member(): the session is valid only while the
// member's epoch is unchanged and the address verified.
func (s *Service) MemberByEpoch(id, epoch int64) (*Member, error) {
	var m Member
	err := s.db.QueryRowContext(context.Background(), "SELECT id,email,epoch FROM members WHERE id=? AND epoch=? AND verified=1", id, epoch).
		Scan(&m.ID, &m.Email, &m.Epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func event(ctx context.Context, q querier, now int64, member int64, license *string, kind string) error {
	var lic any
	if license != nil {
		lic = *license
	}
	_, err := q.ExecContext(ctx, "INSERT INTO events(member_id,license_id,kind,created) VALUES(?,?,?,?)", member, lic, kind, now)
	return err
}

// License is one row of the member's list (Service::licenses()); no credential.
type License struct {
	ID           string
	KeySuffix    string
	Created      int64
	Activated    sql.NullInt64
	Expires      sql.NullInt64
	Hostname     sql.NullString
	ServerID     sql.NullString
	ServerIP     sql.NullString
	KeyAvailable int64
}

// Licenses lists the member's licences that were not removed, newest first.
func (s *Service) Licenses(member int64) ([]License, error) {
	rows, err := s.db.QueryContext(context.Background(),
		"SELECT id,key_suffix,created,activated,expires,hostname,server_id,server_ip,(key_encrypted IS NOT NULL) AS key_available FROM licenses WHERE member_id=? AND removed IS NULL ORDER BY created DESC,id", member)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []License
	for rows.Next() {
		var l License
		if err := rows.Scan(&l.ID, &l.KeySuffix, &l.Created, &l.Activated, &l.Expires, &l.Hostname, &l.ServerID, &l.ServerIP, &l.KeyAvailable); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Issued is a licence id with its key, shown to the member once.
type Issued struct {
	ID  string
	Key string
}

// Issue creates a free licence whose year starts now.
func (s *Service) Issue(member int64) (Issued, error) {
	if err := s.Limit("issue:"+strconv.FormatInt(member, 10), 10, 86400); err != nil {
		return Issued{}, err
	}
	var out Issued
	ctx := context.Background()
	err := s.atomic(func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM members WHERE id=? AND verified=1", member).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAuthenticationRequired
		}
		if err != nil {
			return err
		}
		licence := randomHex(16)
		key := "CPK-" + randomHex(32)
		now := s.Now()
		encrypted, err := s.encryptKey(licence, key)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO licenses(id,member_id,key_hash,key_suffix,created,expires,key_encrypted) VALUES(?,?,?,?,?,?,?)",
			licence, member, sha256Hex(key), key[len(key)-8:], now, anniversary(now), encrypted); err != nil {
			return err
		}
		if err := event(ctx, tx, s.Now(), member, &licence, "issued"); err != nil {
			return err
		}
		out = Issued{ID: licence, Key: key}
		return nil
	})
	return out, err
}

// row is SELECT * FROM licenses.
type row struct {
	ID           string
	Member       int64
	KeyHash      string
	KeySuffix    string
	Created      int64
	Activated    sql.NullInt64
	Expires      sql.NullInt64
	ServerID     sql.NullString
	Hostname     sql.NullString
	Generation   int64
	KeyEncrypted sql.NullString
	ServerIP     sql.NullString
	Removed      sql.NullInt64
}

const rowColumns = "id,member_id,key_hash,key_suffix,created,activated,expires,server_id,hostname,generation,key_encrypted,server_ip,removed"

func scanRow(r *sql.Row) (*row, error) {
	var l row
	err := r.Scan(&l.ID, &l.Member, &l.KeyHash, &l.KeySuffix, &l.Created, &l.Activated, &l.Expires, &l.ServerID, &l.Hostname,
		&l.Generation, &l.KeyEncrypted, &l.ServerIP, &l.Removed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// verifyMember is the password half of Service::owned(), done before any transaction so
// the database's write lock is never held while the password work runs or waits for a
// slot (second review of #361, 6). It answers the hash it verified.
func (s *Service) verifyMember(member int64, password string) (string, error) {
	var hash string
	err := s.db.QueryRowContext(context.Background(), "SELECT password FROM members WHERE id=? AND verified=1", member).Scan(&hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if errors.Is(err, sql.ErrNoRows) || len(password) > 128 {
		return "", ErrInvalidLogin
	}
	valid, err := verifyPassword(password, hash)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", ErrInvalidLogin
	}
	return hash, nil
}

// owned is the rest of Service::owned(): the member's password is still the one just
// verified (a reset in between refuses), then the member's own licence that was not
// removed. The codes and their order are PHP's: invalid_login before license_not_found.
func (s *Service) owned(ctx context.Context, q querier, member int64, id, verified string) (*row, error) {
	var hash string
	err := q.QueryRowContext(ctx, "SELECT password FROM members WHERE id=? AND verified=1", member).Scan(&hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) || !hmac.Equal([]byte(hash), []byte(verified)) {
		return nil, ErrInvalidLogin
	}
	l, err := scanRow(q.QueryRowContext(ctx, "SELECT "+rowColumns+" FROM licenses WHERE id=? AND member_id=? AND removed IS NULL", id, member))
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, ErrLicenseNotFound
	}
	return l, nil
}

// storageKey: storage encryption is domain-separated from receipt signing,
// BLAKE2b-256 keyed with the first 32 bytes of the signing secret
// (sodium_crypto_generichash). Only the private membership host holds it.
func (s *Service) storageKey() *[32]byte {
	h, err := blake2b.New256(s.signingKey[:32])
	if err != nil {
		panic("membership: blake2b: " + err.Error())
	}
	h.Write([]byte("celikpanel-license-key-storage-v1"))
	var key [32]byte
	copy(key[:], h.Sum(nil))
	return &key
}

// encryptKey is base64(nonce || secretbox("<id>:<key>")), as Service::encryptKey().
func (s *Service) encryptKey(id, key string) (string, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	sealed := secretbox.Seal(nonce[:], []byte(id+":"+key), &nonce, s.storageKey())
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptKey reverses encryptKey; any doubt is key_not_saved.
func (s *Service) decryptKey(id, stored, keyHash string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(stored)
	}
	if err != nil || len(raw) < 24+secretbox.Overhead {
		return "", ErrKeyNotSaved
	}
	var nonce [24]byte
	copy(nonce[:], raw[:24])
	plain, ok := secretbox.Open(nil, raw[24:], &nonce, s.storageKey())
	if !ok || !strings.HasPrefix(string(plain), id+":") {
		return "", ErrKeyNotSaved
	}
	key := string(plain[len(id)+1:])
	if !hmac.Equal([]byte(keyHash), []byte(sha256Hex(key))) {
		return "", ErrKeyNotSaved
	}
	return key, nil
}

// Reveal shows a saved key again after the member's password.
func (s *Service) Reveal(member int64, id, password string) (Issued, error) {
	if err := s.Limit("key-access:"+strconv.FormatInt(member, 10), 10, 900); err != nil {
		return Issued{}, err
	}
	verified, err := s.verifyMember(member, password)
	if err != nil {
		return Issued{}, err
	}
	ctx := context.Background()
	l, err := s.owned(ctx, s.db, member, id, verified)
	if err != nil {
		return Issued{}, err
	}
	if !l.KeyEncrypted.Valid || !phpTruthyString(l.KeyEncrypted.String) {
		return Issued{}, ErrKeyNotSaved
	}
	key, err := s.decryptKey(id, l.KeyEncrypted.String, l.KeyHash)
	if err != nil {
		return Issued{}, err
	}
	if err := event(ctx, s.db, s.Now(), member, &id, "key_viewed"); err != nil {
		return Issued{}, err
	}
	return Issued{ID: id, Key: key}, nil
}

// Release unbinds the server: the same key works on another server, the term
// stays, and the old activation stops (generation+1).
func (s *Service) Release(member int64, id, password string) error {
	if err := s.Limit("release:"+strconv.FormatInt(member, 10), 5, 86400); err != nil {
		return err
	}
	verified, err := s.verifyMember(member, password)
	if err != nil {
		return err
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		if _, err := s.owned(ctx, tx, member, id, verified); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE licenses SET server_id=NULL,server_ip=NULL,hostname=NULL,generation=generation+1 WHERE id=?", id); err != nil {
			return err
		}
		return event(ctx, tx, s.Now(), member, &id, "server_released")
	})
}

// Remove keeps the row and its events as the record; the key stops matching, the
// saved copy is erased and the bound server's next refresh is refused.
func (s *Service) Remove(member int64, id, password string) error {
	if err := s.Limit("remove:"+strconv.FormatInt(member, 10), 10, 86400); err != nil {
		return err
	}
	verified, err := s.verifyMember(member, password)
	if err != nil {
		return err
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		if _, err := s.owned(ctx, tx, member, id, verified); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE licenses SET removed=?,key_hash=?,key_encrypted=NULL,server_id=NULL,server_ip=NULL,generation=generation+1 WHERE id=?",
			s.Now(), "removed:"+id, id); err != nil {
			return err
		}
		return event(ctx, tx, s.Now(), member, &id, "removed")
	})
}

// Rotate replaces the key; binding and term stay, the old activation stops.
func (s *Service) Rotate(member int64, id, password string) (Issued, error) {
	if err := s.Limit("rotate:"+strconv.FormatInt(member, 10), 5, 86400); err != nil {
		return Issued{}, err
	}
	verified, err := s.verifyMember(member, password)
	if err != nil {
		return Issued{}, err
	}
	ctx := context.Background()
	var out Issued
	err = s.atomic(func(tx *sql.Tx) error {
		if _, err := s.owned(ctx, tx, member, id, verified); err != nil {
			return err
		}
		key := "CPK-" + randomHex(32)
		encrypted, err := s.encryptKey(id, key)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE licenses SET generation=generation+1,key_hash=?,key_suffix=?,key_encrypted=? WHERE id=?",
			sha256Hex(key), key[len(key)-8:], encrypted, id); err != nil {
			return err
		}
		if err := event(ctx, tx, s.Now(), member, &id, "key_rotated"); err != nil {
			return err
		}
		out = Issued{ID: id, Key: key}
		return nil
	})
	return out, err
}

// anniversary is the same calendar moment one year later in UTC; 29 February
// becomes 28 February (Service::anniversary()).
func anniversary(timestamp int64) int64 {
	d := time.Unix(timestamp, 0).UTC()
	year, month := d.Year()+1, d.Month()
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := d.Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, d.Hour(), d.Minute(), d.Second(), 0, time.UTC).Unix()
}

// Renew is the free renewal: in the last 30 days or after expiry, one year from
// the later of now and the old end. A later fulfilled order grants the same.
func (s *Service) Renew(member int64, id, password string) error {
	verified, err := s.verifyMember(member, password)
	if err != nil {
		return err
	}
	ctx := context.Background()
	return s.atomic(func(tx *sql.Tx) error {
		l, err := s.owned(ctx, tx, member, id, verified)
		if err != nil {
			return err
		}
		now := s.Now()
		if !l.Expires.Valid || l.Expires.Int64 == 0 || l.Expires.Int64 > now+30*86400 {
			return ErrRenewalNotDue
		}
		base := now
		if l.Expires.Int64 > base {
			base = l.Expires.Int64
		}
		if _, err := tx.ExecContext(ctx, "UPDATE licenses SET expires=? WHERE id=?", anniversary(base), id); err != nil {
			return err
		}
		return event(ctx, tx, s.Now(), member, &id, "renewed_free")
	})
}

func (s *Service) activationToken(id, serverID string, generation int64) string {
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte("activation-v1:" + id + ":" + serverID + ":" + strconv.FormatInt(generation, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Envelope is the panel's receipt: base64 payload, base64 Ed25519 signature over
// the raw payload bytes, and the activation token for later refreshes.
type Envelope struct {
	Payload         string
	Signature       string
	ActivationToken string
}

// JSON is PHP's json_encode() of the envelope, byte for byte: without
// JSON_UNESCAPED_SLASHES, so every "/" of the base64 text is written "\/".
func (e Envelope) JSON() []byte {
	esc := func(s string) string { return strings.ReplaceAll(s, "/", `\/`) }
	return []byte(`{"payload":"` + esc(e.Payload) + `","signature":"` + esc(e.Signature) + `","activation_token":"` + esc(e.ActivationToken) + `"}`)
}

// Payload is the claims text exactly as Service::entitlement() encodes it.
func payloadJSON(id, serverID string, issued, activated, expires int64) []byte {
	offline := issued + OfflineGrace
	if expires < offline {
		offline = expires
	}
	return []byte(`{"format":"celikpanel-license-v1","product":"celikpanel","license_id":` + jsonString(id) +
		`,"server_id":` + jsonString(serverID) + `,"issued_at":` + strconv.FormatInt(issued, 10) +
		`,"activated_at":` + strconv.FormatInt(activated, 10) + `,"expires_at":` + strconv.FormatInt(expires, 10) +
		`,"refresh_after":` + strconv.FormatInt(issued+RefreshAfter, 10) + `,"offline_until":` + strconv.FormatInt(offline, 10) + `}`)
}

// jsonString is json_encode() of a string with JSON_UNESCAPED_SLASHES: licence and
// server ids are hex, but anything else is escaped as PHP does for ASCII.
func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				b.WriteString(`\u00` + hex.EncodeToString([]byte{c}))
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func (s *Service) entitlement(id, serverID string, activated, expires, generation int64) Envelope {
	payload := payloadJSON(id, serverID, s.Now(), activated, expires)
	return Envelope{
		Payload:         base64.StdEncoding.EncodeToString(payload),
		Signature:       base64.StdEncoding.EncodeToString(ed25519.Sign(s.signingKey, payload)),
		ActivationToken: s.activationToken(id, serverID, generation),
	}
}

var (
	keyPattern      = regexp.MustCompile(`^CPK-[a-f0-9]{64}$`)
	hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{0,252}$`)
)

// Activate binds a key to a server at the address the web server saw.
func (s *Service) Activate(key, server, hostname, observedIP string) (Envelope, error) {
	ip, err := publicIP(observedIP)
	if err != nil {
		return Envelope{}, err
	}
	if !keyPattern.MatchString(key) || !hex64.MatchString(server) || !hostnamePattern.MatchString(hostname) {
		return Envelope{}, ErrInvalidActivation
	}
	ctx := context.Background()
	var out Envelope
	err = s.atomic(func(tx *sql.Tx) error {
		l, err := scanRow(tx.QueryRowContext(ctx, "SELECT "+rowColumns+" FROM licenses WHERE key_hash=?", sha256Hex(key)))
		if err != nil {
			return err
		}
		if l == nil {
			return ErrInvalidLicense
		}
		if l.ServerIP.Valid && !hmac.Equal([]byte(l.ServerIP.String), []byte(ip)) {
			return ErrLicenseInUse
		}
		// Legacy bindings prove the old installation once, or the owner releases them.
		if !l.ServerIP.Valid && l.ServerID.Valid && !hmac.Equal([]byte(l.ServerID.String), []byte(server)) {
			return ErrLicenseInUse
		}
		generation := l.Generation
		if l.ServerID.Valid && !hmac.Equal([]byte(l.ServerID.String), []byte(server)) {
			generation++
		}
		now := s.Now()
		if l.Expires.Valid && l.Expires.Int64 <= now {
			return ErrLicenseExpired
		}
		activated, expires := l.Activated, l.Expires
		if !activated.Valid {
			activated = sql.NullInt64{Int64: now, Valid: true}
			if !expires.Valid {
				expires = sql.NullInt64{Int64: anniversary(l.Created), Valid: true}
			}
		}
		first := !l.ServerID.Valid
		encrypted, err := s.encryptKey(l.ID, key)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE licenses SET activated=?,expires=?,server_id=?,hostname=?,server_ip=?,generation=?,key_encrypted=? WHERE id=?",
			activated, expires, server, hostname, ip, generation, encrypted, l.ID); err != nil {
			return err
		}
		if first {
			if err := event(ctx, tx, s.Now(), l.Member, &l.ID, "activated"); err != nil {
				return err
			}
		}
		out = s.entitlement(l.ID, server, activated.Int64, expires.Int64, generation)
		return nil
	})
	return out, err
}

// Refresh renews the receipt of the bound server, at the bound address.
func (s *Service) Refresh(id, server, token, observedIP string) (Envelope, error) {
	ip, err := publicIP(observedIP)
	if err != nil {
		return Envelope{}, err
	}
	ctx := context.Background()
	var out Envelope
	err = s.atomic(func(tx *sql.Tx) error {
		l, err := scanRow(tx.QueryRowContext(ctx, "SELECT "+rowColumns+" FROM licenses WHERE id=?", id))
		if err != nil {
			return err
		}
		if l == nil || !l.ServerID.Valid || !phpTruthyString(l.ServerID.String) || !hmac.Equal([]byte(l.ServerID.String), []byte(server)) ||
			!hmac.Equal([]byte(s.activationToken(l.ID, l.ServerID.String, l.Generation)), []byte(token)) {
			if s.tokenAhead(l, id, server, token) {
				return ErrDatabaseBehind // a deliberate difference from PHP (which answers 400 here)
			}
			return ErrInvalidActivation
		}
		if l.ServerIP.Valid && !hmac.Equal([]byte(l.ServerIP.String), []byte(ip)) {
			return ErrLicenseInUse
		}
		if !l.ServerIP.Valid {
			if _, err := tx.ExecContext(ctx, "UPDATE licenses SET server_ip=? WHERE id=?", ip, id); err != nil {
				return err
			}
			if err := event(ctx, tx, s.Now(), l.Member, &id, "ip_bound"); err != nil {
				return err
			}
		}
		out = s.entitlement(l.ID, l.ServerID.String, l.Activated.Int64, l.Expires.Int64, l.Generation)
		return nil
	})
	return out, err
}

// ErrDatabaseBehind: a panel presented an activation token that our signing key made, for
// a state this database does not know yet (the licence row is missing, or its generation
// is lower, or it is unbound or bound elsewhere at the token's generation). The database is
// older than the licence, for example a stale copy after a move. The service answers 503 so
// the panel keeps its receipt, instead of PHP's 400 that would lock it. Second review of #361,
// finding 1(b): a deliberate difference from the PHP service.
var ErrDatabaseBehind = errors.New("membership: the database is older than a licence our key signed")

// tokenAhead reports whether token is ours for a generation this database has not reached.
// Every deliberate change of a licence -- rotate, release, remove, a reinstall at the same
// address -- raises the row's generation, so the token of a revoked state is always for a
// LOWER generation and keeps its rejection. Only a token for the row's generation or later,
// on a row that is missing, has a lower generation, or is unbound or bound elsewhere at that
// generation, means the database is behind. A first activation binds an unbound row without
// raising the generation, which is why "unbound at the same generation" counts as behind.
func (s *Service) tokenAhead(l *row, id, server, token string) bool {
	if !hex32.MatchString(id) || !hex64.MatchString(server) || !hex64.MatchString(token) {
		return false
	}
	from := int64(1)
	if l != nil && l.Generation > 0 {
		from = l.Generation
	}
	for g := from; g <= from+64; g++ {
		if !hmac.Equal([]byte(s.activationToken(id, server, g)), []byte(token)) {
			continue
		}
		return l == nil || g > l.Generation || !l.ServerID.Valid || l.ServerID.String != server
	}
	return false
}

var hex32 = regexp.MustCompile(`^[a-f0-9]{32}$`)

// HasLicences reports whether the file holds at least one licence row. A file without any
// (a fresh initialisation, or the wrong file) cannot answer panels: the API answers 503.
func (s *Service) HasLicences() (bool, error) {
	var exists int
	err := s.db.QueryRowContext(context.Background(), "SELECT EXISTS(SELECT 1 FROM licenses)").Scan(&exists)
	return exists == 1, err
}

// phpTruthyString is PHP's truthiness of a string: "" and "0" are false.
func phpTruthyString(s string) bool { return s != "" && s != "0" }

// PublicKey is the public half of the signing key.
func (s *Service) PublicKey() ed25519.PublicKey { return s.signingKey.Public().(ed25519.PublicKey) }
