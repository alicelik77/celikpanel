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
    'mconn.smtpAuth': 'Outgoing mail requires sign-in with the same user name and password.',
    // Whether these settings reach this mailbox (D-024): mail not on this
    // server (verified), or a server name this server did not confirm.
    'mconn.reach.notHere.title': 'These settings do not reach this mailbox yet',
    'mconn.reach.notHere.why': 'Mail (Postfix and Dovecot) is not installed on this server, so nothing here answers mail programs.',
    'mconn.reach.notHere.next.admin': 'Install mail on Components.',
    'mconn.reach.notHere.next.other': 'Ask the server administrator to install mail.',
    'mconn.reach.notHere.then': 'This window shows settings that work, and they can be copied.',
    'mconn.reach.maybe.title': 'These settings may not reach this mailbox',
    'mconn.reach.nameUnset.why': 'This server’s mail name is not set yet, so {name} is only the usual name, not one this server confirmed.',
    'mconn.reach.nameUnset.next.admin': 'Set the mail name on the mail settings page.',
    'mconn.reach.nameUnset.next.other': 'Ask the server administrator which server name to use.',
    'mconn.reach.nameUnset.then': 'This window shows the server’s mail name.',
    'mconn.reach.unconfirmed.why': '{name} is only the usual name for this domain; this server did not choose or check it for mail.',
    'mconn.reach.unconfirmed.next.admin': 'Give this server’s own mail name, shown on the mail settings page, as the incoming and outgoing server.',
    'mconn.reach.unconfirmed.next.other': 'Ask the server administrator which server name to use.',
    // The plain text "Copy all" gives, ready to paste into a message.
    'mconn.text.unconfirmed': 'Note: the server name below is not confirmed yet. Ask whoever gave you these settings before using them.',
    'mconn.text.title': 'Mail settings for {address}',
    'mconn.text.password': 'Password: the one you were given separately (not in this message)',
    'mconn.text.smtpAuth': '  Sign-in: required, with the same user name and password',
};

export type MailboxConnectionKey = keyof typeof enMailboxConnection;
