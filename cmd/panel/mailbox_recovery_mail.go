package main

import "strings"

// The four messages of mailbox recovery (alpha.130). Every value placed in
// them is checked by the caller: the addresses passed the validators, the
// links are built from the panel's own address and a URL-safe token, the
// host is the panel's certificate name.
//
// Posta kutusu kurtarmanın dört iletisi. İçine giren her değer çağıranca
// doğrulanmıştır.

type mailboxRecoveryMailKind int

const (
	// To the recovery address: confirm that it may reset the mailbox.
	mailboxRecoveryMailConfirm mailboxRecoveryMailKind = iota
	// To the recovery address: the reset link.
	mailboxRecoveryMailReset
	// To the mailbox: a recovery address is now active.
	mailboxRecoveryMailActivated
	// To the mailbox: its password was reset with a recovery link.
	mailboxRecoveryMailWasReset
)

type mailboxRecoveryMail struct {
	kind        mailboxRecoveryMailKind
	lang        string
	host        string
	mailbox     string
	masked      string // the recovery address as the mailbox is shown it
	link        string
	requestPage string
}

// mailboxRecoveryMailText returns the subject and the body.
func mailboxRecoveryMailText(m mailboxRecoveryMail) (string, string) {
	tr := m.lang == "tr"
	switch m.kind {
	case mailboxRecoveryMailConfirm:
		if tr {
			return "Kurtarma adresi olarak onaylayın: " + m.mailbox, strings.Join([]string{
				m.host + " sunucusundaki " + m.mailbox + " posta kutusunu yöneten kişi, bu adresi kutunun kurtarma adresi olarak girdi.",
				"Onaylarsanız, posta kutusunun parolası unutulduğunda sıfırlama bağlantısı bu adrese gelir. Bu bağlantıyı alan, kutunun parolasını değiştirebilir.",
				"",
				"Onaylamak için bu bağlantıyı 24 saat içinde açın:",
				m.link,
				"",
				"Bağlantı yalnız bir kez çalışır. Bu posta kutusunu tanımıyorsanız bu iletiyi yok sayın; hiçbir şey değişmez.",
			}, "\n")
		}
		return "Confirm as recovery address: " + m.mailbox, strings.Join([]string{
			"Whoever manages the mailbox " + m.mailbox + " on " + m.host + " entered this address as its recovery address.",
			"If you confirm, a reset link is sent here when the mailbox's password is forgotten. Whoever receives that link can change the mailbox's password.",
			"",
			"To confirm, open this link within 24 hours:",
			m.link,
			"",
			"The link works once. If you do not know this mailbox, ignore this message; nothing changes.",
		}, "\n")
	case mailboxRecoveryMailReset:
		if tr {
			return "Posta kutusu parolasını sıfırlayın: " + m.mailbox, strings.Join([]string{
				m.host + " sunucusundaki " + m.mailbox + " posta kutusu için parola sıfırlama istendi. Bu adres, kutunun onaylı kurtarma adresidir.",
				"",
				"Yeni parolayı belirlemek için bu bağlantıyı 30 dakika içinde açın:",
				m.link,
				"",
				"Bağlantı yalnız bir kez çalışır. Bunu siz istemediyseniz bu iletiyi yok sayın; parola değişmez.",
				"Bağlantının süresi dolduysa yenisini isteyin: " + m.requestPage,
			}, "\n")
		}
		return "Reset the mailbox password: " + m.mailbox, strings.Join([]string{
			"Someone asked to reset the password of the mailbox " + m.mailbox + " on " + m.host + ". This address is the mailbox's confirmed recovery address.",
			"",
			"To choose a new password, open this link within 30 minutes:",
			m.link,
			"",
			"The link works once. If you did not ask for this, ignore this message; the password stays the same.",
			"If the link has expired, ask for a new one: " + m.requestPage,
		}, "\n")
	case mailboxRecoveryMailActivated:
		if tr {
			return "Posta kutunuzun kurtarma adresi etkin", strings.Join([]string{
				m.host + " sunucusundaki " + m.mailbox + " posta kutusunun artık bir kurtarma adresi var: " + m.masked,
				"Parolanızı unutursanız sıfırlama bağlantısını bu adrese isteyebilirsiniz: " + m.requestPage,
				"",
				"Bu adresi siz ya da posta kutunuzu yöneten kişi girmediyse, hizmet sağlayıcınızdan adresi kaldırmasını isteyin.",
			}, "\n")
		}
		return "Your mailbox has a recovery address", strings.Join([]string{
			"The mailbox " + m.mailbox + " on " + m.host + " now has a recovery address: " + m.masked,
			"If you forget your password, you can have a reset link sent there: " + m.requestPage,
			"",
			"If neither you nor whoever manages your mailbox set this address, ask your provider to remove it.",
		}, "\n")
	default: // mailboxRecoveryMailWasReset
		if tr {
			return "Posta kutunuzun parolası sıfırlandı", strings.Join([]string{
				m.host + " sunucusundaki " + m.mailbox + " posta kutusunun parolası, kurtarma adresine (" + m.masked + ") gönderilen bağlantıyla değiştirildi.",
				"Açık olan posta programı oturumları kapatıldı; programlar yeni parolayı ister.",
				"",
				"Bunu siz yaptıysanız bir şey yapmanız gerekmez. Siz yapmadıysanız hemen hizmet sağlayıcınıza başvurun.",
			}, "\n")
		}
		return "Your mailbox password was reset", strings.Join([]string{
			"The password of the mailbox " + m.mailbox + " on " + m.host + " was changed with a link sent to its recovery address (" + m.masked + ").",
			"Mail programs that were signed in were disconnected; they ask for the new password.",
			"",
			"If you did this, there is nothing to do. If you did not, contact your provider now.",
		}, "\n")
	}
}
