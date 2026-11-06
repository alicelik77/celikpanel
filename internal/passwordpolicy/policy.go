// Package passwordpolicy is the one rule every new password the panel sets
// must meet (owner, 6 Nov 2026): at least six characters, an uppercase
// letter, a lowercase letter and a special character (anything that is not a
// letter or a digit). A field that already asked for more keeps its stricter
// minimum and its byte maximum; see Fields. The web UI carries the same rule in
// web/src/lib/passwordPolicy.ts, and both read testdata/cases.json and
// testdata/fields.json in their tests, so the two cannot drift.
//
// Existing passwords are not checked: the rule applies only when a password is
// set or changed. A sign-in or "current password" is never checked here.
//
// Paket passwordpolicy, panelin koydugu her yeni parolanin uymasi gereken tek
// kuraldir (sahip, 6 Kas 2026): en az alti karakter, bir buyuk harf, bir kucuk
// harf ve bir ozel karakter. Daha siki en az uzunlugu olan alan onu korur.
// Arayuz ayni kurali web/src/lib/passwordPolicy.ts icinde tasir; iki taraf da
// testdata dosyalarini okur, birbirinden ayrilamaz.
package passwordpolicy

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// ErrorCode is the API refusal code for a password that breaks the rule. The
// refusal's reason is the first rule it broke; vars carry min and max.
const ErrorCode = "PASSWORD_WEAK"

// BaseMinChars is the owner's minimum for every field.
const BaseMinChars = 6

// The rules, in the order they are checked and reported.
const (
	RuleInvalidText  = "invalid_text"  // not valid UTF-8, or a control character (line break, tab, NUL)
	RuleTooShort     = "too_short"     // fewer characters than the field's minimum
	RuleTooLong      = "too_long"      // more bytes than the field's maximum
	RuleNeedsUpper   = "needs_upper"   // no uppercase letter
	RuleNeedsLower   = "needs_lower"   // no lowercase letter
	RuleNeedsSpecial = "needs_special" // no character that is neither a letter nor a digit
)

// Field is the length window of one kind of password. MinChars counts
// characters (Unicode code points) and is never below BaseMinChars; MaxBytes
// counts UTF-8 bytes, which is what the services receive (0: no maximum).
type Field struct {
	MinChars int `json:"min"`
	MaxBytes int `json:"max"`
}

// Fields is every kind of password the panel sets, with the stricter minimum
// each one already had before the owner's rule. The web UI has the same table
// (passwordFields in web/src/lib/passwordPolicy.ts); testdata/fields.json is
// the copy both test against.
var Fields = map[string]Field{
	// Panel accounts: create, change, reset link, team members, admin CLI.
	"account": {MinChars: 8, MaxBytes: 1024},
	// Mailbox create and change (doveadm receives bytes).
	"mailbox": {MinChars: 8, MaxBytes: 1024},
	// Database user create and change.
	"database_user": {MinChars: 12, MaxBytes: 128},
	// FTP/SFTP/SSH access account of a site.
	"file_access": {MinChars: 8, MaxBytes: 128},
	// Password-protected directory user (bcrypt reads at most 72 bytes).
	"site_protect": {MinChars: 8, MaxBytes: 72},
	// Owner account of a service application.
	"service_app_owner": {MinChars: 12, MaxBytes: 128},
}

// Error says which rules a password broke. Rules is never empty.
type Error struct {
	Rules    []string
	MinChars int
	MaxBytes int
}

// Reason is the first rule broken: the one the person is told about first.
func (e *Error) Reason() string { return e.Rules[0] }

// Vars are the values the localized sentence needs.
func (e *Error) Vars() map[string]string {
	return map[string]string{"min": fmt.Sprint(e.MinChars), "max": fmt.Sprint(e.MaxBytes)}
}

// Error is the English sentence for the first rule broken.
func (e *Error) Error() string { return Message(e.Reason(), e.MinChars, e.MaxBytes, "en") }

// MessageTR is the Turkish sentence for the first rule broken.
func (e *Error) MessageTR() string { return Message(e.Reason(), e.MinChars, e.MaxBytes, "tr") }

// Message is the plain sentence for one rule, in "en" or "tr".
func Message(rule string, minChars, maxBytes int, lang string) string {
	tr := lang == "tr"
	switch rule {
	case RuleInvalidText:
		if tr {
			return "Şifre satır sonu, sekme gibi denetim karakterleri içeremez."
		}
		return "Password cannot contain line breaks, tabs or other control characters."
	case RuleTooShort:
		if tr {
			return fmt.Sprintf("Şifre en az %d karakter olmalı.", minChars)
		}
		return fmt.Sprintf("Password must be at least %d characters.", minChars)
	case RuleTooLong:
		if tr {
			return fmt.Sprintf("Şifre en fazla %d bayt olabilir.", maxBytes)
		}
		return fmt.Sprintf("Password can be at most %d bytes.", maxBytes)
	case RuleNeedsUpper:
		if tr {
			return "Şifrede en az bir büyük harf olmalı."
		}
		return "Password must contain an uppercase letter."
	case RuleNeedsLower:
		if tr {
			return "Şifrede en az bir küçük harf olmalı."
		}
		return "Password must contain a lowercase letter."
	case RuleNeedsSpecial:
		if tr {
			return "Şifrede en az bir özel karakter (harf ya da rakam olmayan, örneğin - _ . !) olmalı."
		}
		return "Password must contain a special character (not a letter or digit, e.g. - _ . !)."
	}
	if tr {
		return "Şifre kurala uymuyor."
	}
	return "Password does not meet the rule."
}

// Check returns nil when password meets the rule for a field whose window is
// minChars characters to maxBytes bytes (0: no maximum), or the rules it broke.
// A minimum below BaseMinChars is raised to it.
func Check(password string, minChars, maxBytes int) *Error {
	if minChars < BaseMinChars {
		minChars = BaseMinChars
	}
	var rules []string
	invalid := !utf8.ValidString(password)
	var upper, lower, special bool
	for _, r := range password {
		switch {
		case r == utf8.RuneError || unicode.IsControl(r):
			invalid = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			special = true
		}
	}
	if invalid {
		rules = append(rules, RuleInvalidText)
	}
	if utf8.RuneCountInString(password) < minChars {
		rules = append(rules, RuleTooShort)
	}
	if maxBytes > 0 && len(password) > maxBytes {
		rules = append(rules, RuleTooLong)
	}
	if !upper {
		rules = append(rules, RuleNeedsUpper)
	}
	if !lower {
		rules = append(rules, RuleNeedsLower)
	}
	if !special {
		rules = append(rules, RuleNeedsSpecial)
	}
	if len(rules) == 0 {
		return nil
	}
	return &Error{Rules: rules, MinChars: minChars, MaxBytes: maxBytes}
}

// CheckField checks password against a named field in Fields. An unknown
// field name is a programming error and panics in tests and development.
func CheckField(password, field string) *Error {
	f, ok := Fields[field]
	if !ok {
		panic("passwordpolicy: unknown field " + field)
	}
	return Check(password, f.MinChars, f.MaxBytes)
}
