// Route-scoped copy of one mailbox's connection settings window (alpha.139,
// like ../mailboxRecovery): the window opened from a mailbox row's "More"
// menu and from the success notice after a mailbox is made. It ships with
// that lazy chunk, so neither the critical boot path nor the "mailDatabases"
// screen part grows; ../mailboxConnection.ts looks keys up in the
// I18nProvider's locale. The window's field names (server, port, security,
// user name) are the "Settings" tab's own (mail.setup.*).
//
// Bir posta kutusunun bağlantı bilgileri penceresinin kendi metni: tembel
// paketle gelir; alan adları Ayarlar sekmesininkilerdir.

export const enMailboxConnection = {
    'mconn.lead': 'What a mail program (Outlook, Thunderbird, Apple Mail, a phone) needs to add this mailbox.',
    'mconn.incoming': 'Incoming mail',
    'mconn.incomingAlt': 'Incoming mail, alternative',
    'mconn.outgoing': 'Outgoing mail',
    'mconn.password': 'The password is the one set for this mailbox; it is never shown here. To set a new one, use “Password…” on the mailbox’s row.',
    'mconn.passwordReadOnly': 'The password is the one set for this mailbox; it is never shown here. Only someone who may change this domain’s mail can set a new one.',
    'mconn.copyAll': 'Copy all',
    'mconn.copied': 'Copied.',
    'mconn.copiedAll': 'Everything above was copied, without the password: paste it into a message.',
    'mconn.copyFailed': 'The browser did not allow copying. Select the text and copy it by hand.',
    // The plain text "Copy all" gives, ready to paste into a message.
    'mconn.text.title': 'Mail settings for {address}',
    'mconn.text.password': 'Password: the one you were given separately (not in this message)',
};

export type MailboxConnectionKey = keyof typeof enMailboxConnection;
