package passwordpolicy

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The same cases are read by web/tests/password-policy.test.mjs: the panel
// and the browser must refuse exactly the same passwords.
func TestCasesShared(t *testing.T) {
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Password string   `json:"password"`
		Min      int      `json:"min"`
		Max      int      `json:"max"`
		Rules    []string `json:"rules"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("only %d cases", len(cases))
	}
	for _, c := range cases {
		got := Check(c.Password, c.Min, c.Max)
		if len(c.Rules) == 0 {
			if got != nil {
				t.Errorf("%q (min %d max %d): refused %v, want accepted", c.Password, c.Min, c.Max, got.Rules)
			}
			continue
		}
		if got == nil {
			t.Errorf("%q (min %d max %d): accepted, want %v", c.Password, c.Min, c.Max, c.Rules)
			continue
		}
		if !reflect.DeepEqual(got.Rules, c.Rules) {
			t.Errorf("%q (min %d max %d): rules %v, want %v", c.Password, c.Min, c.Max, got.Rules, c.Rules)
		}
	}
}

func TestFieldsShared(t *testing.T) {
	raw, err := os.ReadFile("testdata/fields.json")
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]Field
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fields, Fields) {
		t.Fatalf("testdata/fields.json %v differs from Fields %v", fields, Fields)
	}
	for name, f := range Fields {
		if f.MinChars < BaseMinChars {
			t.Errorf("%s: minimum %d is below the owner's %d", name, f.MinChars, BaseMinChars)
		}
	}
}

func TestErrorSaysTheFirstRuleInBothLanguages(t *testing.T) {
	err := CheckField("abc", "mailbox")
	if err == nil || err.Reason() != RuleTooShort {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "at least 8 characters") || !strings.Contains(err.MessageTR(), "en az 8 karakter") {
		t.Fatalf("messages %q / %q", err.Error(), err.MessageTR())
	}
	if v := err.Vars(); v["min"] != "8" || v["max"] != "1024" {
		t.Fatalf("vars %v", v)
	}
	for _, rule := range []string{RuleInvalidText, RuleTooShort, RuleTooLong, RuleNeedsUpper, RuleNeedsLower, RuleNeedsSpecial} {
		if Message(rule, 8, 128, "en") == Message("unknown", 8, 128, "en") || Message(rule, 8, 128, "tr") == Message("unknown", 8, 128, "tr") {
			t.Errorf("rule %s has no sentence of its own", rule)
		}
	}
}

func TestUnknownFieldPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	CheckField("Ab-defgh", "nope")
}
