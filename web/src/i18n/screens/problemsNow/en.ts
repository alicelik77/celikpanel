// SCREEN catalogue part "problemsNow" (security release, 20 November 2026,
// split out of "monitoring" when its words filled it): the "Problems now"
// list shown on Monitoring and the Dashboard (pnow.). Each problem says what
// is wrong, who acts, the one next action and how it clears (D-024).
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "problemsNow" ekran parçası: İzleme ve Pano'daki "Şu anki sorunlar" listesi.

export const enProblemsNowScreens = {
    'pnow.title': 'Problems now',
    'pnow.checking': 'Checking for problems…',
    'pnow.none': 'No problems now. This list checks again every minute.',
    'pnow.failed': 'The list of problems could not be read, so it is not shown. This does not mean there are none. Nothing was changed.',
    'pnow.retry': 'Try again',
    'pnow.since': 'since {time}',
    'pnow.seen': 'last checked {time}',
    'pnow.notChecked': 'Not checked right now: {list}. That is not the same as "no problem"; this list checks again every minute.',
    'pnow.source.services': 'components (open Components once to check them)',
    'pnow.source.metrics': 'processor and memory',
    'pnow.source.disk': 'disk space',
    'pnow.source.certificates': 'certificates',
    'pnow.source.mail_queue': 'mail queue',
    'pnow.source.security_updates': 'security updates',
    'pnow.source.sites': 'website checks',
    'pnow.source.names': 'domain names and Node.js ports',
    'pnow.reason.checking': 'being read',
    'pnow.reason.unreadable': 'could not be read',
    'pnow.reason.not_scanned': 'not checked yet',
    'pnow.reason.stale': 'only an old reading, waiting for a new one',
    'pnow.unknownResult': 'Whether {name} started is not known yet. Nothing is sent twice; this list reads its state again.',
    'pnow.startFailed': 'It could not be started. Nothing else was changed. Its page on Components shows the service’s own reason.',

    'pnow.service_stopped.title': '{name} is not running',
    'pnow.service_stopped.body': 'What it serves (websites, mail or DNS) may not answer. You can start it here. If it stops again, its page on Components shows the service’s own reason. This line goes away by itself once it runs.',
    'pnow.service_stopped.action': 'Start',
    'pnow.disk_full.title': 'The disk is {value} full',
    'pnow.disk_full.body': 'When it is full, sites, mail and databases can no longer write. The server owner frees space: old backups, large logs and mailboxes no longer needed, or adds disk. The Domains list shows how much each site uses. This line goes away by itself below 85%.',
    'pnow.disk_full.action': 'See use by site',
    'pnow.certificate_expiring.title': 'The certificate of {domain} expires in {days} days',
    'pnow.certificate_expiring.body': 'Automatic renewal has not replaced it yet. When it expires, visitors see a security warning. Renew it in the domain’s Security section; this line goes away once the new certificate is in place.',
    'pnow.certificate_expiring.action': 'Renew',
    'pnow.certificate_expired.title': 'The certificate of {domain} has expired',
    'pnow.certificate_expired.body': 'Visitors see a security warning now. Renew it in the domain’s Security section; this line goes away once the new certificate is in place.',
    'pnow.certificate_expired.action': 'Renew',
    'pnow.certificate_renewal_failed.title': 'Renewing the certificate of {domain} failed',
    'pnow.certificate_renewal_failed.body': '{days} days are left. The domain’s Security section shows the reason and what to do; after a successful renewal this line goes away.',
    'pnow.certificate_renewal_failed.action': 'Open the certificate',
    'pnow.cpu_sustained.title': 'The processor has been {value} busy for over 10 minutes',
    'pnow.cpu_sustained.body': 'Sites may answer slowly. Monitoring shows the five busiest processes, captured when the average reached 90%, under the processor chart: the server owner stops or fixes that program. This line goes away by itself once the 30-second averages drop below 90%.',
    'pnow.cpu_sustained.action': 'Open Monitoring',
    'pnow.memory_sustained.title': 'Memory has been {value} used for over 10 minutes',
    'pnow.memory_sustained.body': 'When memory runs out, the system stops programs to free it. Monitoring shows the five largest processes, captured when use reached 90%, under the memory chart: the server owner restarts or limits that program, or adds memory. This line goes away by itself once use drops below 90%.',
    'pnow.memory_sustained.action': 'Open Monitoring',
    'pnow.mail_queue_deferred.title': '{value} messages are waiting in the mail queue',
    'pnow.mail_queue_deferred.body': 'The mail server could not deliver them yet and keeps retrying. The queue shows the receiving server’s answer for each. Fix the cause, or remove unwanted messages there; the count is read again every five minutes.',
    'pnow.mail_queue_deferred.action': 'Open the mail queue',
    'pnow.security_updates.title': '{value} security updates are waiting',
    'pnow.security_updates.body': 'They fix known security problems in the operating system’s packages. Install them under Security, System updates; the count is read again within ten minutes.',
    'pnow.security_updates.action': 'Open System updates',
    'pnow.alertsNew': '{n} new alerts on Monitoring',
    'pnow.site_down.title': '{domain} does not answer',
    'pnow.site_down.body': 'Asked from this server, the site gave no answer or a server error. Check that the web server and PHP run (Services on Monitoring), then open the site. This line goes away by itself at the next good check, every 5 minutes.',
    'pnow.site_down.action': 'Open the site',
    'pnow.site_certificate.title': 'The certificate {domain} serves needs attention',
    'pnow.site_certificate.body': 'It expires within {days} days or is not valid for this name. If it was renewed, the web server may still serve the old one. Renew it in the domain’s Security section; this line goes away at the next check.',
    'pnow.site_certificate.action': 'Open the certificate',
    'pnow.service_restarting.title': '{name} keeps restarting',
    'pnow.service_restarting.body': 'systemd restarted it {value} times in the last hour: it fails and comes back. Its page on Components shows its log with the reason. This line goes away once it stays up.',
    'pnow.service_restarting.action': 'Open its page',
    // Security release (20 November 2026): names and ports an installation
    // already held before the one name rule. Nothing is changed by itself.
    'pnow.name_overlap.title': '{domain} meets a name of another account',
    'pnow.name_overlap.body': '{domain} belongs to a customer or reseller and lies under, above or on {other}, a name of another account. Whoever holds {domain} can take over names, certificates and mail of {other}. Nothing was changed. Remove {domain}, or transfer the subscription that holds it to the account that holds {other}; this line goes away by itself.',
    'pnow.name_overlap.action': 'Open the domain',
    'pnow.name_server_reserved.title': '{domain} meets one of this server’s own names',
    'pnow.name_server_reserved.body': '{domain} belongs to a customer or reseller and lies under, above or on {other}, a name of this server. Whoever holds it can point that name elsewhere or get its certificate. Nothing was changed. Unless that account may truly hold the server’s name, remove {domain}: only an administrator’s own subscription holds it. This line goes away by itself.',
    'pnow.name_server_reserved.action': 'Open the domain',
    'pnow.node_port_reserved.title': 'The Node.js site {domain} uses port {value} of a server service',
    'pnow.node_port_reserved.body': 'Its visitors are passed to port {value}, which a service of this server uses, so that service may be reachable from the internet. Nothing was changed. Save the site’s Node.js settings again: it gets a free port, and its program must listen on that port.',
    'pnow.node_port_reserved.action': 'Open the domain',
    // alpha.143 (lane S1b): PostgreSQL databases made before alpha.141 that
    // every account of the server can still enter.
    'pnow.postgresql_databases_open.title': '{value} PostgreSQL databases are still open to the server’s other accounts',
    'pnow.postgresql_databases_open.body': 'Made before alpha.141: {name}. Every account of the server may connect to them, or make temporary tables where it may already connect. CelikPanel found something there it does not decide by itself, or its close failed. On the PostgreSQL page, read what was found, then close each one; it can be undone.',
    'pnow.postgresql_databases_open.action': 'Review and close',
    // alpha.143 (lane S3b): mail signed only for the sender's own domain.
    'pnow.mail_signing.title': 'Mail may still be signed for another customer’s domain',
    // 21 Nov 2026: one body per state, each with the server owner's concrete step.
    'pnow.mail_signing.conf_kept': '{domain} was changed by hand and kept. The server owner adds the line SenderMacro {auth_authen} to it (the panel’s full text is in {domain}.celikpanel-pending) and restarts OpenDKIM.',
    'pnow.mail_signing.port25_kept': 'The port 25 milter setting ({domain}) was changed by hand and can still sign arriving mail. The server owner takes OpenDKIM out of it, or removes it so the panel writes it, and restarts the panel.',
    'pnow.mail_signing.submit_open': 'Programs of sites can send mail signed for any domain on this server. The server owner sets authorized_submit_users = root, vmail in /etc/postfix/main.cf and reloads Postfix.',
    'pnow.mail_signing.unsupported': 'This server’s OpenDKIM cannot sign by the mailbox login, so mail sent with a mailbox password is signed by its From: line. No setting here changes that.',
    'pnow.mail_signing.failed': 'A signing change was put back, or OpenDKIM did not start again. Mail is delivered. Check that OpenDKIM runs on its component page, then restart the panel.',
    'pnow.mail_signing.unknown': 'The last mail change could not confirm how mail is signed. Restart the panel to check again.',
    'pnow.mail_signing.action': 'Open the mail server',
} as const;

export type ProblemsNowScreenKey = keyof typeof enProblemsNowScreens;
