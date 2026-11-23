package membership

// Password hashes in the format PHP's password_hash(PASSWORD_ARGON2ID) writes and
// password_verify() reads, so a member hashed by either service signs in on both:
//
//	$argon2id$v=19$m=65536,t=4,p=1$<16-byte salt>$<32-byte hash>   (base64, no padding)
//
// m, t and p are PHP 8.3's defaults (PASSWORD_ARGON2_DEFAULT_MEMORY_COST 65536 KiB,
// TIME_COST 4, THREADS 1). Verification reads the parameters from the stored text, so
// a hash made with other costs still verifies. bcrypt ($2y$) is read too, because
// password_verify() accepts it; the service itself never writes one.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const (
	argonMemory  = 65536
	argonTime    = 4
	argonThreads = 1
	argonSaltLen = 16
	argonKeyLen  = 32
)

// passwordSlots bounds the password work running at once (review of #361, 5): each
// Argon2id costs 64 MiB, and the loopback server has no process limit in front of it
// the way CGI has. A burst waits for a slot instead of exhausting memory; every check
// still costs the same work, so an unknown address stays indistinguishable.
var passwordSlots = make(chan struct{}, 4)

// passwordWorkHook lets a test observe the work inside a slot.
var passwordWorkHook func()

// The wait for a slot is bounded (second review of #361, 6): at most passwordQueueLimit
// callers wait, each at most passwordWait; a caller that cannot get a slot is answered
// with the existing refusal (rate_limited). Nobody waits for a slot while holding the
// database's write lock: the services check the password before their transaction.
var (
	passwordQueueLimit int32 = 32
	passwordWait             = 10 * time.Second
	passwordWaiting    atomic.Int32
	errPasswordBusy    = errors.New("membership: every password slot is busy")
)

func withPasswordSlot(work func()) error {
	select {
	case passwordSlots <- struct{}{}:
	default:
		if passwordWaiting.Add(1) > passwordQueueLimit {
			passwordWaiting.Add(-1)
			return errPasswordBusy
		}
		timer := time.NewTimer(passwordWait)
		select {
		case passwordSlots <- struct{}{}:
			timer.Stop()
			passwordWaiting.Add(-1)
		case <-timer.C:
			passwordWaiting.Add(-1)
			return errPasswordBusy
		}
	}
	defer func() { <-passwordSlots }()
	if passwordWorkHook != nil {
		passwordWorkHook()
	}
	work()
	return nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	var key []byte
	if err := withPasswordSlot(func() { key = argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen) }); err != nil {
		return "", ErrRateLimited
	}
	return "$argon2id$v=19$m=" + strconv.Itoa(argonMemory) + ",t=" + strconv.Itoa(argonTime) + ",p=" +
		strconv.Itoa(argonThreads) + "$" + base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(key), nil
}

// dummyHash keeps the cost of a sign-in for an unknown address equal to a real one.
// PHP uses a bcrypt dummy (cheaper than its Argon2id hashes, so an unknown address
// answers faster there); this port spends the same Argon2id work either way.
var dummyHash = "$argon2id$v=19$m=65536,t=4,p=1$" + base64.RawStdEncoding.EncodeToString(make([]byte, argonSaltLen)) +
	"$" + base64.RawStdEncoding.EncodeToString(make([]byte, argonKeyLen))

// verifyPassword is password_verify($password, $hash). The error is ErrRateLimited when no
// password slot could be had in time.
func verifyPassword(password, hash string) (bool, error) {
	switch {
	case strings.HasPrefix(hash, "$argon2id$"), strings.HasPrefix(hash, "$argon2i$"):
		return verifyArgon2(password, hash)
	case strings.HasPrefix(hash, "$2y$"), strings.HasPrefix(hash, "$2a$"), strings.HasPrefix(hash, "$2b$"):
		p := []byte(password)
		if len(p) > 72 {
			p = p[:72] // crypt_blowfish reads at most 72 bytes
		}
		ok := false
		if err := withPasswordSlot(func() { ok = bcrypt.CompareHashAndPassword([]byte(hash), p) == nil }); err != nil {
			return false, ErrRateLimited
		}
		return ok, nil
	}
	return false, nil
}

func verifyArgon2(password, hash string) (bool, error) {
	parts := strings.Split(hash, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[2] != "v=19" {
		return false, nil
	}
	var m, t, p uint64
	for _, field := range strings.Split(parts[3], ",") {
		name, value, ok := strings.Cut(field, "=")
		n, err := strconv.ParseUint(value, 10, 32)
		if !ok || err != nil {
			return false, nil
		}
		switch name {
		case "m":
			m = n
		case "t":
			t = n
		case "p":
			p = n
		default:
			return false, nil
		}
	}
	if m < 8 || m > 1<<22 || t < 1 || t > 64 || p < 1 || p > 255 || m < 8*p {
		return false, nil
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return false, nil
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 4 || len(want) > 1024 {
		return false, nil
	}
	var got []byte
	if err := withPasswordSlot(func() {
		if parts[1] == "argon2id" {
			got = argon2.IDKey([]byte(password), salt, uint32(t), uint32(m), uint8(p), uint32(len(want)))
		} else {
			got = argon2.Key([]byte(password), salt, uint32(t), uint32(m), uint8(p), uint32(len(want)))
		}
	}); err != nil {
		return false, ErrRateLimited
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
