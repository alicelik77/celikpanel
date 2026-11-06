package services

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alicelik/celikpanel/internal/passwordpolicy"
)

// GeneratePassword draws a password of length characters (16 below 8) with
// crypto/rand that meets the owner's password rule: an uppercase and a
// lowercase letter and one of -_., which no shell, URL or config file reads
// specially. It used to append "!" to a base64 draw, which neither guaranteed
// both letter cases nor stayed shell-safe.
// GeneratePassword, sahibin kuralina uyan bir parola cizer.
func GeneratePassword(length int) (string, error) {
	if length < 8 {
		length = 16
	}
	return passwordpolicy.Generate(length)
}

// updateOrAddSetting updates or adds a setting in config content
func updateOrAddSetting(content, key, value string) string {
	// Try to update existing setting (handles commented and uncommented lines)
	re := regexp.MustCompile(fmt.Sprintf(`(?m)^([ \t]*[;#]?[ \t]*)%s([ \t]*=[ \t]*.*)$`, regexp.QuoteMeta(key)))
	replacement := fmt.Sprintf("%s = %s", key, value)
	if re.MatchString(content) {
		return re.ReplaceAllStringFunc(content, func(string) string { return replacement })
	}
	// If not found, add at the end
	return strings.TrimRight(content, "\n") + "\n" + replacement + "\n"
}
