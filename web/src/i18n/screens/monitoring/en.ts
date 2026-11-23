// SCREEN catalogue part "monitoring" (alpha.126): the Monitoring page
// (monitoring., moved here from "security"). The "Problems now" list (pnow.)
// has its own part, "problemsNow", since the security release of 20 November
// 2026.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "monitoring" ekran parçası: İzleme sayfası ve "Şu anki sorunlar" listesi.

export const enMonitoringScreens = {
    'monitoring.subtitle': 'Processor, memory, disk and load. Each point is the average of 30 seconds; history is kept for 30 days.',
    'monitoring.cpu': 'Processor',
    'monitoring.vcpu': '{n} vCPU',
    'monitoring.memory': 'Memory',
    'monitoring.disk': 'Disk (/)',
    'monitoring.load': 'Load (1m)',
    'monitoring.range.1h': '1 hour',
    'monitoring.range.6h': '6 hours',
    'monitoring.range.24h': '24 hours',
    'monitoring.range.7d': '7 days',
    'monitoring.range.30d': '30 days',
    'monitoring.pct': '{n}%',
    'monitoring.highest': 'highest {value}',
    'monitoring.shortPeak': 'short peak {value}',
    'monitoring.cpuHint': 'The line is the average across all {n} vCPU. A short peak is the highest single second; brief peaks are normal and are not a problem by themselves.',
    'monitoring.empty': 'No samples yet. The first point appears about a minute after the panel starts.',
    'monitoring.checking': 'Reading the recorded measurements…',
    'monitoring.unknown': 'The recorded measurements could not be read from the server, so no charts are shown. This does not mean nothing was recorded. Nothing was changed. Try again.',

    'dashboard.audit.action.monitoring.alerts.settings': 'changed the monitoring alert settings',
    // alpha.128 ops (G18): notifications by recipient.
    'dashboard.audit.action.notifications.preferences': 'changed which notices they receive by email',
    'dashboard.audit.action.notifications.settings': 'changed the server-wide notification settings',
    // alpha.144 (alpha.143 lab): a failed PostgreSQL close names the kind of cause (pgpub.; in this part for its size)
    'err.POSTGRESQL_PUBLIC_FAILED.refused': 'The database server refused it. Nothing was changed.',
    'err.POSTGRESQL_PUBLIC_FAILED.in_use': 'The database is in use: the database server did not grant the lock it needed in time. Nothing was changed. Try again when the connections that hold it have ended.',
    'err.POSTGRESQL_PUBLIC_FAILED.timeout': 'The database server took too long and stopped it. Nothing was changed. Try again.',
    'err.POSTGRESQL_PUBLIC_FAILED.connection': 'The connection to the database server was lost. Nothing was changed. Check that PostgreSQL runs, then try again.',
    'err.POSTGRESQL_PUBLIC_FAILED.rights': 'The database server did not allow it: a right was missing. Nothing was changed.',
    'err.POSTGRESQL_PUBLIC_FAILED.missing': 'Something it names does not exist on the database server (a user or a role). Nothing was changed.',
    // alpha.145 (alpha.144 lab O2): the window's title follows the outcome.
    'pgpub.end.closed': '{name} is closed to other accounts',
    'pgpub.end.notClosed': '{name} was not closed to other accounts',
    'pgpub.end.unknown': 'Whether {name} was closed to other accounts is not known',
    'pgpub.undo.end.opened': '{name} is open to every account again',
    'pgpub.undo.end.notOpened': '{name} was not opened again',
    'pgpub.undo.end.unknown': 'Whether {name} was opened again is not known',
    'pgpub.step.notDone': 'Not done — nothing was changed',
    'pgpub.step.unknown': 'Result not confirmed yet — not sent again',
    'pgpub.why.server.in_use': 'the database was in use and the database server did not grant the lock in time; nothing was changed: “{detail}”',
    'pgpub.why.server.timeout': 'the database server took too long and stopped it; nothing was changed: “{detail}”',
    'pgpub.why.server.connection': 'the connection to the database server was lost; nothing was changed: “{detail}”',
    'pgpub.why.server.rights': 'the database server did not allow it (a right was missing); nothing was changed: “{detail}”',
    'pgpub.why.server.missing': 'something it names does not exist on the database server; nothing was changed: “{detail}”',
} as const;

export type MonitoringScreenKey = keyof typeof enMonitoringScreens;
