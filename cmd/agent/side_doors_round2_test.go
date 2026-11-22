package main

import (
	"reflect"
	"testing"
)

// S4 R1, second fix round (22 Nov 2026): the findings of the second
// independent review of PR #348. Each test fails on 193b87001 (or does not
// build there).

// --- B1: the "in use" question fails only in the safe direction ---

// Any mention of the socket outside a full-line comment, or a *_pass whose
// address is a variable, keeps the pool on; nginx's grammar is never read to
// decide "not used". The cases the second review found (a customer's
// gzip_types text with a mid-word # and quote that made the earlier reader
// lose the owner's fastcgi_pass; set and map) fail on 193b87001.
func TestTheStockPoolInUseCheckFailsOnlyInTheSafeDirection(t *testing.T) {
	listen := "/run/php/php8.4-fpm.sock"
	customer := "# configuration file /etc/nginx/celikpanel/site3/server.conf:\n"
	owner := "# configuration file /etc/nginx/sites-enabled/owner.conf:\nserver {\n"
	ownerFile := []string{"/etc/nginx/sites-enabled/owner.conf"}
	cases := []struct {
		name string
		dump string
		used []string
	}{
		{"a customer's mid-word # and quote before the owner's fastcgi_pass",
			customer + "gzip_types a#\"b\nc\";\n" + owner +
				"    location ~ \\.php$ { fastcgi_pass unix:/run/php/php8.4-fpm.sock; }\n}\n" +
				"# configuration file /etc/nginx/x.conf:\nadd_header A \"b\";\n",
			ownerFile},
		{"set, then the variable", owner + "    set $s unix:/run/php/php8.4-fpm.sock;\n    fastcgi_pass $s;\n}\n", ownerFile},
		{"a map", "# configuration file /etc/nginx/conf.d/map.conf:\nmap $host $sock {\n    default unix:/var/run/php/php8.4-fpm.sock;\n}\n",
			[]string{"/etc/nginx/conf.d/map.conf"}},
		{"a quoted argument", owner + "    fastcgi_pass \"unix:/run/php/php8.4-fpm.sock\";\n}\n", ownerFile},
		{"an address built from a variable", owner + "    fastcgi_pass\n        unix:/run/php/php$v-fpm.sock;\n}\n", ownerFile},
		{"a word with # before the owner's fastcgi_pass", owner + "    set $x a#b; fastcgi_pass unix:/run/php/php8.4-fpm.sock;\n}\n", ownerFile},
		{"a customer's harmless header naming the socket", customer + "add_header X-Note \"unix:/run/php/php8.4-fpm.sock\";\n",
			[]string{"/etc/nginx/celikpanel/site3/server.conf"}},
		{"only full-line comments", owner + "    # fastcgi_pass unix:/run/php/php8.4-fpm.sock;\n\t# set $s x; fastcgi_pass $s;\n}\n", nil},
		{"nothing names it", owner + "    fastcgi_pass unix:/run/php/php8.4-fpm-site3.sock;\n}\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stockPoolUsers(tc.dump, listen, "", nil)
			if len(got) != len(tc.used) || (len(got) > 0 && !reflect.DeepEqual(got, tc.used)) {
				t.Fatalf("users %v, want %v", got, tc.used)
			}
		})
	}
}
