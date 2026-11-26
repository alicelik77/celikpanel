package dbremote

import (
	"strings"
	"testing"
)

// alpha.149 fix round B (lab D9): a remote copy of a MariaDB user carries the
// local account's MAX_STATEMENT_TIME, as it carries its connection limit; a
// value that is not MariaDB's own number is refused, never written.
func TestARemoteCopyCarriesTheStatementTime(t *testing.T) {
	credential := MariaDBCredential{Plugin: "mysql_native_password", Auth: "*0123456789ABCDEF0123456789ABCDEF01234567", MaxUserConnections: 15, MaxStatementTime: "5.000000"}
	create, err := CreateMariaDBAccount("shop_u", "203.0.113.7", credential)
	// alpha.149 (review of PR #370, finding 19): the integer form, the one
	// MariaDB's documentation leaves no doubt about.
	if err != nil || !strings.HasSuffix(create, " REQUIRE SSL WITH MAX_USER_CONNECTIONS 15 MAX_STATEMENT_TIME 5 ACCOUNT UNLOCK;") {
		t.Fatalf("create %q (%v)", create, err)
	}
	alter, err := AlterMariaDBAccount("shop_u", "203.0.113.7", credential)
	if err != nil || !strings.Contains(alter, "MAX_STATEMENT_TIME 5 ") {
		t.Fatalf("alter %q (%v)", alter, err)
	}
	for in, want := range map[string]string{"0.000000": "0", "5.000000": "5", "2.500000": "3", "86400.000000": "86400", "7": "7"} {
		if got := MariaDBStatementTimeLiteral(in); got != want {
			t.Errorf("%s written as %s, want %s", in, got, want)
		}
	}
	for _, bad := range []string{"5; DROP USER x", "-1", "1e3", "5.0000001", " 5"} {
		credential.MaxStatementTime = bad
		if _, err := CreateMariaDBAccount("shop_u", "203.0.113.7", credential); err == nil {
			t.Errorf("statement time %q was written", bad)
		}
	}
}
