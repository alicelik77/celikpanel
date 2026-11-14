// SCREEN catalogue part "certCoverage" (alpha.134): what a domain's
// certificate protects - the certificate screen's list "What is protected",
// its reasons and actions, the request form's mail name, and the mail
// screen's sentence under the server name (docs/CERTIFICATE-COVERAGE.md).
// Key prefix kept here: certcover., and the progress step
// opstep.mailCert.site.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "certCoverage": alan adının sertifikasının neyi koruduğu.

export const enCertCoverageScreens = {
    'certcover.title': 'What is protected',
    'certcover.hint': 'Everything of this domain that visitors and mail programs connect to, and whether a trusted certificate protects it.',
    'certcover.checking': 'Checking what is protected…',
    'certcover.unknown': 'What is protected could not be checked right now. This does not mean it is unprotected.',
    'certcover.kind.domain': 'Web site',
    'certcover.kind.www': 'Web site with www',
    'certcover.kind.alias': 'Another name of the web site',
    'certcover.kind.webmail': 'Web mail',
    'certcover.kind.mail_programs': 'Mail programs (the name they are told)',
    'certcover.kind.mail_name': 'Mail name (MX record and hand-typed settings)',
    'certcover.kind.mail_setup': 'Mail programs’ automatic setup',
    'certcover.state.protected': 'Protected',
    'certcover.state.not_protected': 'Not protected',
    'certcover.state.unknown': 'Not known',
    'certcover.state.points_elsewhere': 'Points elsewhere',
    'certcover.state.no_address': 'No address in DNS',
    'certcover.next.mail': 'If this domain’s mail is on this server, point this name’s A record to {ip} {where}. Once it points here, “Cover the mail name too” appears here. If the mail is elsewhere, nothing needs to be done.',
    'certcover.next.site': 'If this name should reach this server, point its A record to {ip} {where}. Once it points here, “Get it again with this name” appears here.',
    'certcover.where.here': 'in this server’s DNS records',
    'certcover.where.elsewhere': 'where the domain’s DNS is run (the company you bought the domain from, or your DNS service)',
    'certcover.ip.unknown': 'this server’s address',
    'certcover.by.domain_certificate': 'By this domain’s certificate, until {date}.',
    'certcover.by.server_mail_certificate': 'By the server’s mail certificate, until {date}.',
    'certcover.by.webmail_certificate': 'By web mail’s own certificate, until {date}.',
    'certcover.reason.no_certificate': 'This domain has no certificate yet. The certificate you get below includes this name.',
    'certcover.reason.not_in_certificate': 'The name points to this server but is not in the certificate, so programs that use it see a warning.',
    'certcover.reason.points_elsewhere': 'The name points to {ips}, not to this server. This is not a fault: only a name that points here can be in this server’s certificate.',
    'certcover.reason.no_address': 'The name has no address in DNS yet, so it cannot be in the certificate.',
    'certcover.reason.certificate_not_usable': 'The certificate includes the name but cannot be used right now; its state is shown above.',
    'certcover.reason.mail_not_using': 'The certificate includes the name, but mail still answers on it with the server’s certificate.',
    'certcover.reason.server_certificate_not_ready': 'The server’s mail certificate is not ready yet. The server administrator gets it on the mail settings page.',
    'certcover.reason.mail_name_unknown': 'The server’s mail name is not set yet. The server administrator sets it on the mail settings page.',
    'certcover.reason.webmail_no_certificate': 'Web mail answers on this address without a certificate for it. The server administrator fixes it on the web mail address page.',
    'certcover.reason.not_checked': 'Could not be checked right now. This does not mean it is unprotected.',
    'certcover.action.cover_mail': 'Cover the mail name too',
    'certcover.action.reissue': 'Get it again with this name',
    'certcover.action.upload': 'Upload a certificate with this name',
    'certcover.action.use_for_mail': 'Use it for mail too',
    'certcover.action.dns': 'Open DNS records',
    'certcover.action.mail_setup': 'Open mail settings',
    'certcover.action.recheck': 'Check again',
    'certcover.confirm.cover': 'Request the certificate of {name} again, now including {mailName}? Mail then uses it for {mailName}; mail programs keep the settings they have. If the request does not succeed, the current certificate and mail stay as they are.',
    'certcover.confirm.reissue': 'Request the certificate of {name} again? Every name of the site that points to this server will be in it. If the request does not succeed, the current certificate stays as it is.',
    'certcover.confirm.useForMail': 'Should mail use this certificate for {mailName}? Mail programs set up with the server’s name keep working as they are.',
    'certcover.done.covered': 'The certificate now includes {mailName}, and mail uses it.',
    'certcover.done.useForMail': 'Mail now uses this certificate for {mailName}.',
    'certcover.note.mail_not_switched': 'The certificate includes {mailName}, but mail could not start using it, so mail works as before. Try again with “Use it for mail too” in the list.',
    'certcover.note.site_file_kept': '{mailName} was left out because the server owner keeps the site’s web server file.',
    'certcover.form.mail': 'Include {name} (mail)',
    'certcover.form.mailHint': 'Programs that connect to {name} (the MX record and hand-typed settings) then get a certificate for that name. The name mail programs are told does not change.',
    'certcover.form.mailElsewhere': '{name} points to {ips}, not to this server, so it is left out.',
    'certcover.form.mailNoAddress': '{name} has no address in DNS, so it is left out.',
    'certcover.form.mailUnknown': 'Whether {name} points to this server could not be checked; the request includes it only if it does.',
    'certcover.mailHost.unknown': 'Whether a trusted certificate protects this name could not be checked right now.',
    'certcover.mailHost.protected.server_mail_certificate': 'A trusted certificate protects this name (the server’s mail certificate, until {date}).',
    'certcover.mailHost.protected.domain_certificate': 'A trusted certificate protects this name (this domain’s certificate, until {date}).',
    'certcover.mailHost.fix': 'Open Security',
    'opstep.mailCert.site': 'Writing the mail programs’ automatic setup answers again',
    // After a request: what was kept of the certificate the site used before (administrator only).
    'certcover.previousKept.in_use': "The previous certificate (certbot's {lineage}) is kept because {file} on this server still names it, so certbot keeps renewing it. CelikPanel removes it at a later check once nothing names it.",
    'certcover.previousKept.owner_changed': "The previous certificate (certbot's {lineage}) is kept because its renewal settings were changed outside CelikPanel. If nothing uses it, remove it on the server: sudo certbot delete --cert-name {lineage}",
    'certcover.previousKept.retry': "The previous certificate could not be removed now. Nothing needs to be done: CelikPanel tries again at its next check, within 12 hours, and writes the reason to its log.",
    'certcover.previousKept.held_by_snapshot': "The previous certificate is kept while a saved update snapshot still uses it, so going back to that update keeps working; CelikPanel removes it once that snapshot is gone.",
} as const;

export type CertCoverageScreenKey = keyof typeof enCertCoverageScreens;
