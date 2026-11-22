package main

import "testing"

// alpha.144: what the server holds for one address, read before the
// administrator's adoption; only CelikPanel's own form is adoptable.
func TestMailboxOnServerReadsOnlyCelikPanelsOwnForm(t *testing.T) {
	users := []byte("satis@example.com:{SHA512-CRYPT}$6$abc::::::" + dovecotQuotaFields(300) + "\n" +
		"home@example.com:{SHA512-CRYPT}$6$abc::::/var/mail/other::" + dovecotQuotaFields(100) + "\n" +
		"uid@example.com:{SHA512-CRYPT}$6$abc:5000:5000::::" + dovecotQuotaFields(100) + "\n" +
		"nolimit@example.com:{SHA512-CRYPT}$6$abc::::::\n" +
		"half@example.com:{SHA512-CRYPT}$6$abc::::::" + dovecotQuotaFields(100) + "\n" +
		"twice@example.com:{SHA512-CRYPT}$6$abc::::::" + dovecotQuotaFields(100) + "\n" +
		"other@example.com:{SHA512-CRYPT}$6$abc::::::" + dovecotQuotaFields(100) + "\n")
	vboxes := []byte("satis@example.com example.com/satis/\n" +
		"home@example.com example.com/home/\n" +
		"uid@example.com example.com/uid/\n" +
		"nolimit@example.com example.com/nolimit/\n" +
		"twice@example.com example.com/twice/\n" +
		"twice@example.com other.test/bob/\n" +
		"other@example.com other.test/bob/\n")
	for email, want := range map[string]struct {
		users, inMap, own bool
		quota             int
	}{
		"satis@example.com":   {true, true, true, 300},
		"home@example.com":    {true, true, false, 100},
		"uid@example.com":     {true, true, false, 100},
		"nolimit@example.com": {true, true, true, 0},
		"half@example.com":    {true, false, false, 100},
		"twice@example.com":   {true, true, false, 100},
		"other@example.com":   {true, true, false, 100},
		"gone@example.com":    {false, false, false, 0},
	} {
		email, local, domain, err := canonicalAgentMailbox(email)
		if err != nil {
			t.Fatal(err)
		}
		got := mailboxOnServer(users, vboxes, email, local, domain)
		if got.InUsers != want.users || got.InMap != want.inMap || got.OwnFolder != want.own || got.QuotaMB != want.quota {
			t.Errorf("%s: %+v, want %+v", email, got, want)
		}
	}
}
