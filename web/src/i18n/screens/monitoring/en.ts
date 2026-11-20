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
} as const;

export type MonitoringScreenKey = keyof typeof enMonitoringScreens;
