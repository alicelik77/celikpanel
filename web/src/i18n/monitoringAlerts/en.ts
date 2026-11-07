// Route-scoped copy of the Monitoring page (alpha.127), like ../setupDNS:
// the alerts and their settings (malert.), the services (msvc.), the website
// checks (msite.), network and disk throughput and the busiest processes at a
// spike (mmach.). It ships with the page's own chunk (and the Dashboard's
// new-alerts line), so the critical boot path does not grow;
// ../monitoringAlerts.ts looks keys up in the I18nProvider's locale.
//
// İzleme sayfasının kendi metni: sayfanın paketiyle gelir, açılış yolu büyümez.

export const enMonitoringAlerts = {
    'malert.title': 'Alerts',
    'malert.newCount': '{n} new',
    'malert.none': 'No alerts in the last 30 days. A problem that lasts longer than {n} minutes is listed here.',
    'malert.checking': 'Reading the alerts…',
    'malert.readFailed': 'The alerts could not be read, so none are shown. This does not mean there are none. Nothing was changed.',
    'malert.retry': 'Try again',
    'malert.openMonitoring': 'Open Monitoring',
    'malert.ongoing': 'ongoing since {time}',
    'malert.ended': '{start} – {end}',
    'malert.markSeen': 'Mark all as seen',
    'malert.seenUnknown': 'Whether they were marked as seen is not known. The list is read again and shows what the server holds.',
    'malert.mail.sent': 'Emailed to the confirmed administrators.',
    'malert.mail.none': 'Shown here only: alert email is off.',
    'malert.mail.pending': 'Being emailed…',
    'malert.mail.suppressed': 'Not emailed again: the same problem was emailed in the last six hours, or four alert emails were sent this hour.',
    'malert.mail.not_confirmed': 'Not emailed: no administrator has confirmed an email address yet (Alert settings, below).',
    'malert.mail.not_available': 'Not emailed: this server has no mail system the panel can hand mail to.',
    'malert.mail.failed': 'Not emailed: the mail system refused it ({detail}). It is not tried again; the mail queue shows why.',
    'malert.panelDown': 'Alerts come from this panel. While the panel or the server is stopped, none are shown or sent, so also use an outside uptime checker for your important sites.',
    'malert.settings': 'Alert settings',
    'malert.delay': 'Alert when a problem lasts longer than',
    'malert.minutes': '{n} minutes',
    'malert.email': 'Email the alerts to administrators with a confirmed address',
    'malert.emailTo': 'Alerts are emailed to {n} confirmed administrator address(es), at most four emails an hour.',
    'malert.emailNone': 'No administrator has a confirmed email address, so alerts are shown here only. Confirm yours; the next alert is emailed.',
    'malert.emailNoAddress': 'Your account has no email address. Add one on My account, then confirm it here.',
    'malert.emailUnconfirmed': 'Your address is not confirmed yet. Confirm it on My account with one press; the next alert is then emailed to it.',
    'malert.openAccount': 'Open My account',
    'malert.mailUnavailable': 'This server has no mail system the panel can hand mail to, so alerts cannot be emailed; they are shown here. Installing the mail components (Postfix) makes email possible.',
    'malert.siteChecks': 'Check every hosted website every 5 minutes from this server',
    'malert.save': 'Save settings',
    'malert.saved': 'Saved. The next check, within a minute, uses these settings.',
    'malert.saveUnknown': 'Whether the settings were saved is not known. They are read again; check them before saving again.',

    'msvc.title': 'Services',
    'msvc.runningSince': 'running since {time}',
    'msvc.running': 'running',
    'msvc.notRunning': 'not running ({state})',
    'msvc.restarts': '{n} restarts in the last hour',
    'msvc.cpu': 'CPU {value}',
    'msvc.memory': 'memory {value}',
    'msvc.checking': 'The first reading arrives within a minute of the panel starting; this list updates by itself.',
    'msvc.unreadable': 'The services could not be read from the server just now, so their state is not shown. This does not mean they stopped. The panel reads them again every minute.',
    'msvc.notScanned': 'The components have not been checked yet. Open Components once; this list then fills by itself.',
    'msvc.openComponents': 'Open Components',

    'msite.title': 'Websites',
    'msite.hint': 'Each site is asked for its home page every 5 minutes from this server itself. That cannot see problems outside the server (DNS, network); an outside uptime checker does.',
    'msite.up': 'answers ({status}, {ms} ms)',
    'msite.down': 'not answering since {time}: {reason}',
    'msite.reason.timeout': 'no answer within 10 seconds',
    'msite.reason.refused': 'the web server refused the connection',
    'msite.reason.tls': 'the secure connection failed',
    'msite.reason.closed': 'the connection was closed',
    'msite.reason.http_5xx': 'it answered with a server error',
    'msite.reason.error': 'the request failed',
    'msite.certDays': 'certificate: {days} days left',
    'msite.certExpired': 'certificate expired',
    'msite.certInvalid': 'certificate not valid for this name',
    'msite.noCert': 'no certificate (HTTP)',
    'msite.none': 'No hosted websites to check.',
    'msite.off': 'Website checks are off (Alert settings).',
    'msite.waiting': 'The first check runs within five minutes of the panel starting; this list updates by itself.',
    'msite.stale': 'last checked {time}',
    'msite.readFailed': 'The website checks could not be read. Nothing was changed.',

    'mmach.net': 'network in {rx}/s · out {tx}/s',
    'mmach.diskIO': 'read {r}/s · write {w}/s',
    'mmach.busiest': 'Busiest processes at {time} ({value}):',
    'mmach.snapshotHint': 'Captured by itself when the 30-second average reached 90%; the marks on the chart show when.',
} as const;

export type MonitoringAlertsKey = keyof typeof enMonitoringAlerts;
