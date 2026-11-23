package membership

// The private configuration, read exactly where the PHP service reads it:
// <private>/config.json (origin, sender, signing_key, optional smtp, and the
// loopback-only test_mail_dir), next to members.sqlite and sessions/. The Go service
// never writes this file and never creates a signing key (portal-membership/init.php
// does when the file is missing: the migration hazard this port removes).

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/alicelik/celikpanel/internal/licensing"
)

// Config is the parsed config.json.
type Config struct {
	Origin     string
	Sender     string
	SigningKey ed25519.PrivateKey
	// SMTPPresent mirrors array_key_exists('smtp', $config): a present but unusable
	// block refuses mail; it never falls back to the local transport.
	SMTPPresent bool
	SMTP        *SMTPConfig
	TestMailDir string
}

// ErrUnpinnedKey: the configured signing key is neither the production key panels
// trust nor the throwaway key a test named.
var ErrUnpinnedKey = errors.New("membership: the signing key is not the pinned production key")

// ProductionPublicKey is the public key every released panel trusts
// (internal/licensing.PublicKeyHex).
func ProductionPublicKey() ed25519.PublicKey {
	key, err := hex.DecodeString(licensing.PublicKeyHex)
	if err != nil || len(key) != ed25519.PublicKeySize {
		panic("membership: pinned public key is malformed")
	}
	return key
}

// PublicHalf derives the public key from the secret's seed and requires the stored
// second half to be the same key (a sodium secret key is seed || public key).
func PublicHalf(secret ed25519.PrivateKey) (ed25519.PublicKey, error) {
	if len(secret) != ed25519.PrivateKeySize {
		return nil, errors.New("membership: invalid license signing key")
	}
	derived := ed25519.NewKeyFromSeed(secret[:32]).Public().(ed25519.PublicKey)
	if !bytes.Equal(derived, secret[32:]) {
		return nil, errors.New("membership: the signing key's halves do not match")
	}
	return derived, nil
}

// CheckSigningKey allows the pinned production key, or exactly the throwaway public
// key a test named (allowTest). Anything else is ErrUnpinnedKey.
func CheckSigningKey(secret ed25519.PrivateKey, allowTest ed25519.PublicKey) error {
	public, err := PublicHalf(secret)
	if err != nil {
		return err
	}
	if bytes.Equal(public, ProductionPublicKey()) {
		return nil
	}
	if len(allowTest) == ed25519.PublicKeySize && bytes.Equal(public, allowTest) {
		return nil
	}
	return ErrUnpinnedKey
}

// LoadConfig reads <private>/config.json the way http.php does (depth 8, JSON
// object). It never logs a value.
func LoadConfig(private string) (Config, error) {
	raw, err := os.ReadFile(filepath.Join(private, "config.json"))
	if err != nil {
		return Config{}, errors.New("membership: config unreadable")
	}
	return ParseConfig(raw)
}

// ParseConfig parses the configuration text.
func ParseConfig(raw []byte) (Config, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil || fields == nil || jsonDepth(raw) > 8 {
		return Config{}, errors.New("membership: config is not a JSON object")
	}
	var c Config
	text := func(name string) (string, bool) {
		var s string
		value, ok := fields[name]
		if !ok || json.Unmarshal(value, &s) != nil {
			return "", false
		}
		return s, true
	}
	var ok bool
	if c.Origin, ok = text("origin"); !ok {
		return Config{}, errors.New("membership: config has no origin")
	}
	c.Sender, _ = text("sender")
	c.TestMailDir, _ = text("test_mail_dir")
	encoded, ok := text("signing_key")
	if !ok {
		return Config{}, errors.New("membership: config has no signing key")
	}
	// base64_decode($v, true): whitespace skipped, padding optional.
	key, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(strings.Join(strings.Fields(encoded), ""), "="))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return Config{}, errors.New("membership: invalid license signing key")
	}
	c.SigningKey = ed25519.PrivateKey(key)
	if value, present := fields["smtp"]; present {
		c.SMTPPresent = true
		c.SMTP = parseSMTP(value)
	}
	return c, nil
}

// parseSMTP keeps only a block whose fields have PHP's types (is_string, is_int);
// anything else is nil, which refuses mail like Mail.php's validation does.
func parseSMTP(value json.RawMessage) *SMTPConfig {
	var block map[string]json.RawMessage
	if json.Unmarshal(value, &block) != nil || block == nil {
		return nil
	}
	var c SMTPConfig
	for name, target := range map[string]*string{"host": &c.Host, "username": &c.Username, "password": &c.Password} {
		if json.Unmarshal(block[name], target) != nil {
			return nil
		}
	}
	port := strings.TrimSpace(string(block["port"]))
	if port == "" || len(port) > 6 || strings.ContainsAny(port, ".eE+") {
		return nil // PHP's is_int: a JSON integer, not 465.0
	}
	n := 0
	for i, ch := range port {
		if ch == '-' && i == 0 {
			return nil
		}
		if ch < '0' || ch > '9' {
			return nil
		}
		n = n*10 + int(ch-'0')
	}
	c.Port = n
	return &c
}

// jsonDepth is the nesting depth json_decode($raw, true, 8) counts.
func jsonDepth(raw []byte) int {
	depth, max := 0, 0
	inString, escaped := false, false
	for _, c := range raw {
		switch {
		case inString:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '{' || c == '[':
			depth++
			if depth > max {
				max = depth
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	return max // json_decode('[[1]]', true, 1) fails: each array or object is one level
}
