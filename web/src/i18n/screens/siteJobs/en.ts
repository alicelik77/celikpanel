// SCREEN catalogue part "siteJobs" (21 Nov 2026, S4 R1): jobs a site's own
// code can make outside CelikPanel. Problems now lines (pnow.) for crontab
// lines not made in CelikPanel and for at (website accounts' waiting jobs,
// an at.allow that names them, at left open), the Scheduled tasks list's
// mark and its other lines (cron.outside.), and the audit sentences.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "siteJobs": bir sitenin kendi kodunun CelikPanel
// dışında kurabileceği zamanlanmış işler.

export const enSiteJobsScreens = {
    'pnow.site_crontab_outside.title': 'Scheduled tasks were added outside CelikPanel',
    'pnow.site_crontab_outside.body': '{value} lines in these sites’ crontabs were not made in CelikPanel: {domain}. A program of the site or someone working on the server added them, and the site’s limits on tasks were not checked for them. Open each site’s Scheduled tasks: the lines are marked there. The administrator or the site’s owner removes the ones nobody asked for; a line the server owner put there on purpose (such as @reboot, MAILTO= or PATH=) the administrator keeps with Keep, and it is no longer listed.',
    'pnow.site_crontab_outside.action': 'Open Scheduled tasks',
    'pnow.site_at_jobs.title': 'Website accounts have at jobs waiting',
    'pnow.site_at_jobs.body': '{value} at jobs belong to website accounts: {domain}. They were made outside CelikPanel, which neither shows nor limits them, and they still run at their time. CelikPanel does not remove them. The server administrator looks at each with sudo at -c <number> and removes the ones the site’s owner did not ask for with sudo atrm <number>. This list is read at each panel start.',
    'pnow.site_at_jobs.action': 'Open Monitoring',
    'pnow.at_allow_lists_sites.title': '/etc/at.allow lets website accounts use at',
    'pnow.at_allow_lists_sites.body': 'The file is the server owner’s, so CelikPanel keeps it as it is. It names these website accounts: {domain}. Each of those websites can plant at jobs that CelikPanel neither shows nor limits. The server administrator removes those names from the file.',
    'pnow.at_allow_lists_sites.action': 'Open Monitoring',
    'pnow.at_directory_accounts.title': 'Website accounts can still use at',
    'pnow.at_directory_accounts.body': 'This server takes its accounts from a directory service, so CelikPanel cannot safely list who may use at today and did not write {domain}. A website can plant at jobs that CelikPanel neither shows nor limits. The server administrator writes {domain} with the accounts that need at, one name per line, leaving website accounts out.',
    'pnow.at_directory_accounts.action': 'Open Monitoring',
    'pnow.at_not_closed.title': 'Website accounts can still use at',
    'pnow.at_not_closed.body': 'CelikPanel could not write {domain}, so a website can plant at jobs that CelikPanel neither shows nor limits. Nothing was changed; CelikPanel tries again after the panel’s next start. If it keeps failing, the server administrator reads the exact reason in Monitoring: /etc must be writable, and /etc/at.deny, /etc/passwd and CelikPanel’s record of the file readable.',
    'pnow.at_not_closed.action': 'Open Monitoring',
    'cron.outside.badge': 'Not made in CelikPanel',
    'cron.outside.notice': 'Some lines of this site’s crontab were not made in CelikPanel: a program of the site or someone working on the server added them. They run like the other tasks, but the site’s limits on tasks were not checked for them. Remove any line nobody asked for. A line the server owner put there on purpose can be kept by the administrator with Keep; it is then no longer marked.',
    'cron.outside.otherTitle': 'Other lines not made in CelikPanel',
    'cron.outside.otherHint': 'These lines are not tasks of the kind CelikPanel writes, for example a task that runs at every start of the server, or a setting for every task.',
    'cron.outside.deleteConfirm': 'Remove this line from the crontab?',
    'cron.outside.deleted': 'Line removed',
    'dashboard.audit.action.at.allow.written': 'wrote /etc/at.allow so that website accounts cannot use at',
    'dashboard.audit.action.at.allow.failed': 'could not close at to website accounts; nothing was changed',
};

export type SiteJobsScreenKey = keyof typeof enSiteJobsScreens;
