// SCREEN catalogue part "domainDelete" (alpha.134): the question that
// deletes a domain - what goes, what stays, the choice for its databases, a
// retry of a deletion that stopped - the words of its result (what was
// deleted and what stayed) and the reasons a deletion stops at one of its
// own steps. Split out of "sites" and "errors", which were full. Key
// prefixes: domains.delete., domains.deleted., domains.deletion.,
// domains.step.done.databases*, err.DOMAIN_DELETION_ (except the mail step's
// reason, which stays in "errors"). The database lines of the question and
// of the result are the Databases page's own (databases., part
// "mailDatabases"). One part of the screen copy; ../../index.tsx fetches
// every part in parallel and no screen renders before all of them have
// arrived.
//
// EKRAN katalogu parçası "domainDelete": alan adı silme sorusu, sonucu ve
// silmenin kendi adımlarında durma nedenleri.

export const enDomainDeleteScreens = {
    'domains.delete.lost.site': 'The website, its files, extra FTP logins, scheduled tasks and logs',
    'domains.delete.lost.mail': 'Its mailboxes and forwarding: mail to these addresses is no longer accepted',
    'domains.delete.lost.dns': 'Its DNS records in CelikPanel and its SSL certificates',
    'domains.delete.lost.schedule': 'Its backup schedule',
    'domains.delete.kept.databasesUnread': 'The domain’s databases could not be read just now, so none is deleted: they stay on the Databases page, bound to no domain.',
    'domains.delete.kept.databasesUnbound': 'Their databases are not deleted: they stay on the Databases page, bound to no domain, where you can manage or delete them.',
    'domains.delete.kept.databasesUsersUnknown': 'Its databases ({n}) stay: which of their users would be removed could not be read just now, so deleting them is not offered here. They appear on the Databases page bound to no domain, where you can delete them later.',
    'domains.delete.databasesOption': 'Also delete its databases ({n})',
    'domains.delete.databasesStay': 'Not ticked: its {n} databases stay, with their users and passwords, on the Databases page, bound to no domain.',
    'domains.delete.databasesGo': 'Ticked: its {n} databases and everything in them are deleted, as listed below.',
    'domains.delete.retryKeep': 'This deletion was started earlier and continues where it stopped. Its databases stay, as chosen then: they appear on the Databases page bound to no domain.',
    'domains.delete.retryDelete': 'This deletion was started earlier and continues where it stopped. Its databases are deleted, as chosen then, with the users nothing else uses.',
    'domains.delete.kept.backups': 'Its backups on this server are kept 30 days, then deleted; copies at a remote backup destination are not touched.',
    'domains.delete.kept.mail': 'The messages in its mailboxes are kept 30 days, then deleted.',
    'domains.delete.kept.dns': 'Records at another DNS provider are not touched.',
    'domains.delete.typeName': 'Type {name} to delete it',
    'domains.step.done.databasesKept': '{n} databases stayed',
    'domains.step.done.databasesDeleted': '{n} databases deleted',
    'domains.step.done.noDatabases': 'No databases to handle',
    'domains.deleted.databasesDeleted': 'Deleted databases: {names}.',
    'domains.deleted.keptTitle': 'Kept',
    'domains.deleted.row.databases': 'Databases: {names}, on the Databases page, bound to no domain',
    'domains.deleted.row.mail': 'Mail: {size}, kept until {date}, then deleted',
    'domains.deleted.row.backups': 'Backups: {n} files, {size}, kept until {date}, then deleted',
    'domains.deleted.row.holdingAdmin': '{size} went to Maintenance → What deletions left, where you can download it or delete it earlier.',
    'domains.deleted.row.askProvider': 'Until {date} your provider can give the mail and the backups back.',
    'domains.deleted.detailsTitle': 'Details',
    'domains.deleted.detail.mail': 'Mail: {path}',
    'domains.deleted.detail.backups': 'Backups: {path}',
    'domains.deleted.sizeUnknown': 'size not known',
    'domains.deletion.stoppedAskAdmin': 'The deletion stopped on the server before it finished; what is already removed stays removed. Your server administrator finds and fixes the cause on the server. After that, use “Retry this deletion” (Delete on this domain): it continues where it stopped.',
    'domains.deletion.stoppedAtDatabases': 'The deletion stopped at the domain’s databases; the website is already removed and the DNS zone is unchanged.',
    'err.DOMAIN_DELETION_FAILED.mail_lists_cleanup_failed': 'Deletion stopped while taking this domain out of the server’s sender-limit lists. The website, the DNS zone and the databases are unchanged. The server administrator opens Mail → Sending limits on this server, fixes what it reports, then uses “Retry this deletion”. Nothing retries by itself.',
    'err.DOMAIN_DELETION_FAILED.site_cleanup_failed': 'Deletion stopped while removing this domain’s website from this server: a part of it (for example its scheduled tasks, its logs or its files) could not be removed. The DNS zone and the databases are unchanged. The server administrator finds the part in the panel log (sudo journalctl -u celikpanel-panel | grep "domain deletion"), checks it on this server, then uses “Retry this deletion”, which repeats the removal and continues. Nothing retries by itself.',
    'err.DOMAIN_DELETION_FAILED.database_cleanup_failed': 'The deletion stopped at the domain’s databases: the website is already removed, the DNS zone is unchanged, and a database not yet deleted is still listed. Use “Retry this deletion” (Delete on this domain): it deletes what is left and continues. If it stops here again, the server administrator makes sure the database server runs (Services page). Nothing retries by itself.',
    'err.DOMAIN_DELETION_PLAN_FIXED': 'This deletion already runs with the other choice for the domain’s databases, so nothing was changed. Use “Retry this deletion” without changing that choice; it continues as it was started.',
} as const;

export type DomainDeleteScreenKey = keyof typeof enDomainDeleteScreens;
