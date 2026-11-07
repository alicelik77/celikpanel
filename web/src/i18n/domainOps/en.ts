// Route-scoped copy (alpha.128, workstream "ops"), like ../monitoringAlerts:
// the domain health strip on a domain's Overview (dhealth.). It ships with
// the screen's own chunk, so the critical boot path does not grow;
// ../domainOps.ts looks keys up in the I18nProvider's locale.
//
// Sayfaya bağlı metin: ekranın paketiyle gelir, açılış yolu büyümez.

export const enDomainOps = {
    'dhealth.title': 'Health of this domain',
    'dhealth.checking': 'Reading the last checks…',
    'dhealth.unknown': 'The last checks could not be read, so the health of this domain is not shown. This does not mean it is fine. Nothing was changed.',
    'dhealth.allGood': 'Everything checked is fine.',
    'dhealth.note': 'From the panel’s last checks; nothing is asked while this page opens.',
    'dhealth.earlier': '(earlier answer, being checked again)',
    'dhealth.unknownCode': 'This check gave an answer this page cannot read, so it is not shown as fine.',

    'dhealth.site.up': 'Website answers',
    'dhealth.site.down': 'The website did not answer at the last check: {reason}.',
    'dhealth.site.checks_off': 'Website checks are turned off, so whether the site answers is not known.',
    'dhealth.site.not_checked': 'Website not checked yet. The first check runs within five minutes.',
    'dhealth.site.stale': 'Website not checked recently; the panel’s checks may have stopped. Whether it answers now is not known.',
    'dhealth.site.unreadable': 'The last website check could not be read.',
    'dhealth.reason.timeout': 'no answer within 10 seconds',
    'dhealth.reason.refused': 'the web server refused the connection',
    'dhealth.reason.tls': 'the secure connection failed',
    'dhealth.reason.closed': 'the connection was closed without an answer',
    'dhealth.reason.http_5xx': 'the site answered with a server error',
    'dhealth.reason.error': 'the request failed',

    'dhealth.certificate.valid': 'Certificate valid for {days} more days',
    'dhealth.certificate.none': 'No certificate: visitors see a security warning.',
    'dhealth.certificate.expired': 'The certificate has expired: visitors see a security warning.',
    'dhealth.certificate.expiring': 'The certificate ends in {days} days and has not been renewed yet.',
    'dhealth.certificate.served_invalid': 'The site serves a certificate that is not valid for its name.',
    'dhealth.certificate.unreadable': 'The certificate’s end date could not be read.',

    'dhealth.dns.here': 'Domain points here',
    'dhealth.dns.notHere': 'The domain does not point to this server yet. The card above says what to change at your registrar.',
    'dhealth.dns.unknown': 'Where the domain points is not known yet. The card above checks it.',

    'dhealth.mail.complete': 'Mail sending and authentication complete',
    'dhealth.mail.auth_missing': 'Mail authentication is missing ({items}): mail from this domain may land in spam.',
    'dhealth.mail.auth_spreading': 'Mail authentication ({items}) is published and still spreading through DNS. Nothing to do.',
    'dhealth.mail.port25_blocked': 'This server cannot send mail out: outgoing port 25 is blocked.',
    'dhealth.mail.listed': 'This server’s address is on a spam blocklist.',
    'dhealth.mail.ptr_missing': 'This server’s address has no reverse name, so other mail servers may refuse its mail.',
    'dhealth.mail.not_checked': 'Mail not checked recently.',
    'dhealth.mail.unreadable': 'The last mail check could not be read.',

    'dhealth.disk.within': 'Disk {used} of {limit}',
    'dhealth.disk.no_limit': 'Disk {used}, no limit',
    'dhealth.disk.near': 'Disk {percent}% full ({used} of {limit}).',
    'dhealth.disk.over': 'Disk full ({used} of {limit}): new uploads and manual backups stop.',
    'dhealth.disk.not_measured': 'Disk use not measured yet.',
    'dhealth.disk.unreadable': 'The disk measurement could not be read.',
    'dhealth.disk.makeRoom': 'Delete what you no longer need, or ask {who} for more space.',

    'dhealth.backup.recent': 'Last backup {days} days ago',
    'dhealth.backup.recentToday': 'Last backup today',
    'dhealth.backup.recent_manual': 'Last backup {days} days ago (no automatic backup)',
    'dhealth.backup.recent_manualToday': 'Last backup today (no automatic backup)',
    'dhealth.backup.schedule_failed': 'The last automatic backup failed.',
    'dhealth.backup.none': 'No backup has been made.',
    'dhealth.backup.old': 'The last backup is {days} days old.',
    'dhealth.backup.old_no_schedule': 'The last backup is {days} days old and no automatic backup is set up.',
    'dhealth.backup.first_pending': 'The first automatic backup has not run yet.',
    'dhealth.backup.unreadable': 'The backup records could not be read.',

    'dhealth.fix.hosting.ssl': 'Open SSL/TLS',
    'dhealth.fix.advanced.logs': 'Open the logs',
    'dhealth.fix.advanced.backups': 'Open Backups',
    'dhealth.fix.mail': 'Open Mail',
    'dhealth.fix.files': 'Open Files',
    'dhealth.fix.page./monitoring': 'Open Monitoring',
    'dhealth.fix.page./users': 'Change the limit',
    'dhealth.contact.administrator': 'Contact the administrator to fix this.',
    'dhealth.contact.provider': 'Contact your provider to fix this.',
    'dhealth.contact.owner': 'Ask the account owner to fix this.',
    'dhealth.who.administrator': 'the administrator',
    'dhealth.who.provider': 'your provider',
    'dhealth.who.owner': 'the account owner',
};

export type DomainOpsKey = keyof typeof enDomainOps;
