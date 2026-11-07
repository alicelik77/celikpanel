package passwordpolicy

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// GeneratedSpecials are the only special characters a generated password
// carries: none of them means anything to a shell, in a URL (RFC 3986
// unreserved), in a quoted or unquoted config value, or in SQL string syntax.
// GeneratedSpecials, uretilen parolanin tasidigi tek ozel karakterlerdir;
// kabukta, URL'de, yapilandirma dosyasinda ya da SQL'de anlam tasimazlar.
const GeneratedSpecials = "-_."

// GeneratedAlphabet is the default alphabet of a generated password.
const GeneratedAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" + GeneratedSpecials

// maxGenerateAttempts bounds the redraws. A draw that misses the rule is
// thrown away whole, so every password that meets it is equally likely; at
// the lengths used (16 and more) fewer than three draws are needed on average.
const maxGenerateAttempts = 1000

// Generate draws a password of length characters from GeneratedAlphabet with
// crypto/rand that meets the rule: an uppercase letter, a lowercase letter
// and a special character, and it neither starts nor ends with a special
// character (so it reads cleanly when shown in a sentence).
// Generate, kurala uyan bir parolayi crypto/rand ile cizer.
func Generate(length int) (string, error) {
	return GenerateFrom(GeneratedAlphabet, length)
}

// GenerateFrom is Generate over a caller's alphabet, which must hold an
// uppercase letter, a lowercase letter and at least one of GeneratedSpecials.
// GenerateFrom, Generate'in cagiranin alfabesiyle olan bicimidir.
func GenerateFrom(alphabet string, length int) (string, error) {
	if length < BaseMinChars {
		return "", fmt.Errorf("passwordpolicy: generated password length %d is below %d", length, BaseMinChars)
	}
	if !usableAlphabet(alphabet) {
		return "", errors.New("passwordpolicy: alphabet cannot meet the rule")
	}
	limit := big.NewInt(int64(len(alphabet)))
	password := make([]byte, length)
	for attempt := 0; attempt < maxGenerateAttempts; attempt++ {
		for i := range password {
			index, err := rand.Int(rand.Reader, limit)
			if err != nil {
				return "", fmt.Errorf("generate password: %w", err)
			}
			password[i] = alphabet[index.Int64()]
		}
		candidate := string(password)
		if strings.ContainsRune(GeneratedSpecials, rune(candidate[0])) ||
			strings.ContainsRune(GeneratedSpecials, rune(candidate[length-1])) {
			continue
		}
		// alpha.128: a digit too when the alphabet has digits, so a
		// generated password meets every password level (Strong, Very strong).
		if Check(candidate, BaseMinChars, 0) == nil && (strings.ContainsAny(candidate, "0123456789") || !strings.ContainsAny(alphabet, "0123456789")) {
			return candidate, nil
		}
	}
	return "", errors.New("passwordpolicy: could not draw a password that meets the rule")
}

// usableAlphabet: printable ASCII only (the draw indexes bytes), with an
// uppercase letter, a lowercase letter and one of GeneratedSpecials.
func usableAlphabet(alphabet string) bool {
	var upper, lower bool
	for i := 0; i < len(alphabet); i++ {
		c := alphabet[i]
		switch {
		case c < 0x21 || c > 0x7e:
			return false
		case c >= 'A' && c <= 'Z':
			upper = true
		case c >= 'a' && c <= 'z':
			lower = true
		}
	}
	return upper && lower && strings.ContainsAny(alphabet, GeneratedSpecials)
}
