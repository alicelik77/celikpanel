// SCREEN catalogue part "siteTasksProblems" (alpha.150, split out of
// "siteTasks" and "problemsNow" when their words filled them): the Problems
// now lines of sites' scheduled tasks and their side doors - cron.allow,
// crontabs that came back, held tasks, lingering, a site account's own
// service manager, a move into a site's limits that did not finish (pnow.) -
// and "Move all", which their line opens (moveall.); since fix round 4 also
// the site card's way back into cron by hand for a kept tasks move record
// (slim.tasks.unresolved), which that line opens.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "siteTasksProblems": sitelerin zamanlanmış
// görevlerinin "Şu anki sorunlar" satırları ve "Hepsini taşı".

export const enSiteTasksProblemsScreens = {
    'pnow.site_move_kept.title': 'A move into a site’s limits did not finish: {domain}',
    'pnow.site_move_kept.body': 'A program may be stopped, or the site’s scheduled tasks may not run; no site is moved until this is settled. The site’s card says what to do.',
    'pnow.site_move_kept.action': 'Open the site',
    'pnow.site_tasks_held.title': 'Scheduled tasks held: {value} sites',
    'pnow.site_tasks_held.body': 'The scheduled tasks of these sites do not run while their limits file is missing ({other}), also after a restart of the server: {domain}. The administrator saves the subscription’s limits; the tasks run again at their next time once the file is back. A file put back by hand needs sudo systemctl daemon-reload too.',
    'pnow.site_tasks_held.action': 'Open Monitoring',
    'pnow.site_lingering.title': 'Site accounts keep their own programs running',
    'pnow.site_lingering.body': 'These accounts linger, so programs they started keep running outside their limits: {domain}. The server owner ends it with: {other}',
    'pnow.site_lingering.action': 'Open Monitoring',
    'pnow.site_user_manager_open.title': 'Site accounts can start their own service manager',
    'pnow.site_user_manager_open.body': 'When these accounts sign in with SFTP, their own service manager can start and run programs outside their limits: {domain}. The server owner closes it with: sudo systemctl mask {other}',
    'pnow.site_user_manager_open.action': 'Open Monitoring',
    'moveall.unread': 'The list of sites outside their limits could not be read, so nothing would be moved. Close this and press again.',
    'moveall.button': 'Move them all',
    'moveall.title': 'Moving every site into its limits',
    'moveall.confirm': 'Move {n} sites into their limits, one after another?',
    'moveall.body': 'Each site moves the way its own “Move into the limits” does: its tasks first, then its running programs restart once inside the limits. A site that cannot move is skipped and said why; the work stops only if a move that started fails.',
    'moveall.go': 'Move them all',
    'moveall.cancel': 'Cancel',
    'moveall.none': 'No site is outside its limits now.',
    'moveall.done': 'Done: {n} sites moved.',
    'moveall.failed': '{domain} did not move: {reason}. The sites after it were not touched.',
    'moveall.unknown': 'The server did not answer about {domain}, so its result is unknown. Read this page again before you go on.',
    'moveall.programs': '{n} programs restart inside the limits',
    'moveall.tasks': '{n} tasks become timers',
    'moveall.skip': '{domain}: skipped, {reason}',
    'moveall.tasksStay': '{domain}: skipped; its tasks stay in cron ({reason}), its {n} programs could move',
    'moveall.option': 'Also move the programs of the sites whose tasks stay in cron ({n})',
    'moveall.optionRow': '{domain}: its programs move; its tasks stay in cron',
    'moveall.previewUnknown': 'what its move would do could not be read',
    'moveall.notMoved': 'Not moved',
    'moveall.unresolved': 'An earlier move could not be put back completely, so no site is moved until it is settled. That site’s card has Try again.',
    'moveall.partial': 'its programs moved; its tasks stayed in cron ({reason})',
    'pnow.site_limits_outside.title': 'Sites outside their limits: {value}',
    'pnow.site_limits_outside.body': 'The running programs or scheduled tasks of these sites still run outside their limits: {domain}. Moving restarts each site’s programs once inside the limits and turns its tasks into timers there; nothing is half-moved. This line goes away by itself.',
    'pnow.site_limits_outside.action': 'Move them all',
    'pnow.cron_allow_lists_sites.title': 'Cron is open again to sites inside their limits',
    'pnow.cron_allow_lists_sites.body': 'The server owner’s {other} lets these accounts use cron: {domain}. Jobs they add run outside their limits. The server owner removes those names from the file.',
    'pnow.cron_allow_lists_sites.action': 'Open Monitoring',
    'pnow.cron_not_closed.title': 'Cron is not closed to sites inside their limits',
    'pnow.cron_not_closed.body': 'CelikPanel did not write {domain}, so these sites’ own code can still add cron jobs that run outside their limits ({other}). The server owner checks the file; CelikPanel tries again at its next start.',
    'pnow.cron_not_closed.action': 'Open Monitoring',
    'pnow.cron_owner_removed.title': '/etc/cron.allow was deleted on this server',
    'pnow.cron_owner_removed.body': 'The sites whose tasks run inside their limits can use cron again, outside them. CelikPanel does not write the file again by itself.',
    'pnow.cron_owner_removed.confirm': 'CelikPanel writes /etc/cron.allow again: every account that may use cron now keeps it, except these sites’ accounts.',
    'pnow.cron_owner_removed.action': 'Close it again',
    'pnow.site_crontab_returned.title': 'A crontab was made again for sites inside their limits',
    'pnow.site_crontab_returned.body': 'These accounts have a crontab again ({value} lines in all), made by root: {domain}. Its lines run outside the sites’ limits. The server owner moves them into the site’s Scheduled tasks or removes the file (crontab -r is refused for these accounts; it is in /var/spool/cron/crontabs, or /var/spool/cron with cronie).',
    'pnow.site_crontab_returned.action': 'Open Monitoring',
    'pnow.site_crontab_by_site.title': 'Sites inside their limits have a crontab again',
    'pnow.site_crontab_by_site.body': 'These accounts have a crontab again ({value} lines in all): {domain}. Cron let them in, so the sites’ own code can have made it, and its lines run outside the limits. The server owner keeps the accounts out of cron, then removes the file (in /var/spool/cron/crontabs, or /var/spool/cron with cronie).',
    'pnow.site_crontab_by_site.action': 'Open Monitoring',
    'pnow.linger_owner_removed.title': 'The lingering rule was deleted on this server',
    'pnow.linger_owner_removed.body': 'Site accounts can switch lingering on for themselves again and keep programs running outside their limits. CelikPanel does not write the rule again by itself.',
    'pnow.linger_owner_removed.confirm': 'CelikPanel writes its polkit rule again: only administrators and root may switch lingering on for themselves.',
    'pnow.linger_owner_removed.action': 'Close it again',
    'pnow.linger_not_closed.title': 'Lingering is not closed to site accounts',
    'pnow.linger_not_closed.body': 'CelikPanel’s polkit rule is not in place ({domain}): it was changed on the server, or could not be written ({other}). Site accounts may keep their own programs running outside their limits. The server owner checks the file.',
    'pnow.linger_not_closed.action': 'Open Monitoring',
    // The site card's way back into cron by hand for a kept tasks move record
    // (the record Problems now's site_move_kept line opens); moved here from
    // "siteTasks" in fix round 4, whose Turkish file had no room left.
    'slim.tasks.unresolved': 'It is this site’s scheduled tasks (step {step}): their record keeps the crontab, so nothing is lost. The administrator presses Try again; CelikPanel also tries at its next start. If Try again cannot finish it, the server owner puts the tasks back into cron by hand, each step only if the one before worked (the timers off, then the task file as it is now as the account’s crontab, so a task switched off on the server comes back as the task file has it; it stops, changing nothing, while the account has a crontab, and can be run again after a stop): {steps} Then the administrator presses Try again: it finds the crontab back, removes the timer files left and the record, and lets the account use cron, so sites can be moved again.',
};

export type SiteTasksProblemsScreenKey = keyof typeof enSiteTasksProblemsScreens;
