package mariadbgrant

import (
	"errors"
	"strings"
	"testing"
)

// The cross-customer pair the alpha.134 brief names: subscription 5 asked
// for "_x" and got s5__x; subscription 51 asked for "x" and got s51_x. In
// the pattern form the first grant covers the second database; in the exact
// form it does not.
func TestPatternFormReachesAnotherCustomerExactFormDoesNot(t *testing.T) {
	if !Matches("s5__x", "s51_x", false) {
		t.Fatal("the pattern form s5__x should cover s51_x (that is the defect)")
	}
	if Matches(Exact("s5__x"), "s51_x", false) {
		t.Fatal("the exact form covers another database")
	}
	if !Matches(Exact("s5__x"), "s5__x", false) {
		t.Fatal("the exact form does not cover its own database")
	}
	if !Matches("labd_1__wp", "labd_12_wp", false) || Matches(Exact("labd_1__wp"), "labd_12_wp", false) {
		t.Fatal("the lab's labd_1__wp / labd_12_wp pair is not reproduced")
	}
}

func TestFormsTargetsAndDecode(t *testing.T) {
	if got := Exact("a_b%c"); got != `a\_b\%c` {
		t.Fatalf("Exact = %q", got)
	}
	if got := Forms("shop_a"); len(got) != 2 || got[0] != "shop_a" || got[1] != `shop\_a` {
		t.Fatalf("Forms = %q", got)
	}
	if got := Forms("shop"); len(got) != 1 {
		t.Fatalf("a name without _ has one form: %q", got)
	}
	if target, err := Target("s5__x"); err != nil || target != "`s5\\_\\_x`" {
		t.Fatalf("Target = %q %v", target, err)
	}
	if target, err := PatternTarget("s5__x"); err != nil || target != "`s5__x`" {
		t.Fatalf("PatternTarget = %q %v", target, err)
	}
	for _, bad := range []string{"", "1abc", "a-b", "a`b", `a\b`, "a%b", strings.Repeat("a", 65)} {
		if _, err := Target(bad); !errors.Is(err, ErrName) {
			t.Fatalf("Target(%q) accepted", bad)
		}
	}
	for _, c := range []struct {
		stored  string
		name    string
		pattern bool
		ok      bool
	}{
		{"shop_a", "shop_a", true, true},
		{`shop\_a`, "shop_a", false, true},
		{"shop", "shop", false, true},
		{`shop\_%`, "", false, false},
		{"shop%", "", false, false},
		{`s5\__x`, "", false, false}, // half escaped: neither form of one name
	} {
		name, pattern, ok := Decode(c.stored)
		if name != c.name || pattern != c.pattern || ok != c.ok {
			t.Fatalf("Decode(%q) = %q %v %v", c.stored, name, pattern, ok)
		}
	}
	if !HasWildcard("a_b") || HasWildcard(`a\_b`) || !HasWildcard(`a\_%`) || HasWildcard("ab") {
		t.Fatal("HasWildcard")
	}
	if !Matches("Shop_a", "shop1a", true) || Matches("Shop_a", "shop1a", false) {
		t.Fatal("case folding follows lower_case_table_names")
	}
	if !Matches("a%", "abc", false) || Matches(`a\%`, "abc", false) || !Matches(`a\%`, "a%", false) {
		t.Fatal("% handling")
	}
}

// Values go to the engine as hexadecimal literals: the same under every
// sql_mode, NO_BACKSLASH_ESCAPES included.
func TestHexLiteralsCarryTheBackslash(t *testing.T) {
	if got := Hex(`a\_b`); got != "X'615C5F62'" {
		t.Fatalf("Hex = %q", got)
	}
	if Hex("") != "''" {
		t.Fatal("empty")
	}
	if back, err := FromHex("615C5F62"); err != nil || back != `a\_b` {
		t.Fatalf("FromHex = %q %v", back, err)
	}
	if _, err := AccountLiteral("u", "h'x"); err == nil {
		t.Fatal("a quote in a host was accepted")
	}
	if got, err := AccountLiteral("u", "10.%"); err != nil || got != "'u'@'10.%'" {
		t.Fatalf("AccountLiteral = %q %v", got, err)
	}
}
