package passwordpolicy

import (
	"strings"
	"testing"
)

func TestGenerateMeetsTheRuleWithSafeCharacters(t *testing.T) {
	for _, length := range []int{6, 16, 20, 24, 40} {
		for i := 0; i < 300; i++ {
			password, err := Generate(length)
			if err != nil {
				t.Fatal(err)
			}
			if len(password) != length || Check(password, BaseMinChars, 0) != nil {
				t.Fatalf("Generate(%d) = %q", length, password)
			}
			if strings.Trim(password, GeneratedAlphabet) != "" {
				t.Fatalf("Generate(%d) = %q: a character outside the alphabet", length, password)
			}
			if strings.ContainsAny(password[:1]+password[length-1:], GeneratedSpecials) {
				t.Fatalf("Generate(%d) = %q: starts or ends with a special", length, password)
			}
		}
	}
}

func TestGenerateFromRefusesWhatCannotMeetTheRule(t *testing.T) {
	for _, alphabet := range []string{
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", // no special
		"abcdefghijklmnopqrstuvwxyz0123456789-_.",                        // no uppercase
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.",                        // no lowercase
		"Aa-\n", // a control character
	} {
		if _, err := GenerateFrom(alphabet, 16); err == nil {
			t.Fatalf("GenerateFrom(%q) drew a password", alphabet)
		}
	}
	if _, err := Generate(BaseMinChars - 1); err == nil {
		t.Fatal("a password shorter than the rule was drawn")
	}
}
