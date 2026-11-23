package membership

// Fixed answers of PHP 8.3.6's parse_str (the parser behind $_GET and $_POST), taken
// from the first CI run of TestPHPCompatCorpus on 2026-11-23, so the rules hold where
// no PHP is installed.

import (
	"reflect"
	"testing"
)

func TestParsePHPQueryMatchesPHP83(t *testing.T) {
	for query, want := range map[string]map[string]string{
		"a[b.c=1":                  {"a_b_c": "s:1"},
		"%5B=%2E%5B":               {},
		"action+.a[+%zz=action]":   {"action__a__%zz": "s:action]"},
		"[=.":                      {},
		"%5B+%5B%00]":              {},
		" .[.id":                   {"___id": "s:"},
		"+%2Eid%5B[":               {"_id__": "s:"},
		"+x%5Bx+ .%2Ex":            {"x_x____x": "s:"},
		"a a&%2E%5B id":            {"___id": "s:", "a_a": "s:"},
		"xax[%5Baction%5B":         {"xax__action_": "s:"},
		"%5B&%zzaction%5Bid=+%zz":  {"%zzaction_id": "s: %zz"},
		"%5B[&]&%2E":               {"]": "s:", "_": "s:"},
		"]&%5B=x+.":                {"]": "s:"},
		"&id[a%2E&. ":              {"__": "s:", "id_a_": "s:"},
		"action=home&action=login": {"action": "s:login"},
		"action[]=x":               {"action": "a"},
		"action=x&action[]=y":      {"action": "a"},
		"action[]=y&action=x":      {"action": "s:x"},
		"act.ion=x":                {"act_ion": "s:x"},
		"action%00junk=x":          {"action": "s:x"},
	} {
		vars := phpVars{}
		parsePHPQuery(query, vars)
		got := map[string]string{}
		for name, v := range vars {
			if v.array {
				got[name] = "a"
			} else {
				got[name] = "s:" + v.str
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parse_str(%q): PHP %v, Go %v", query, want, got)
		}
	}
}
