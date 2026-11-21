// SCREEN catalogue part "siteLimits" (21 Nov 2026, S4 R2a): the limits of a
// site's programs together (its slice) on the domain's Overview, the plan
// keys of those limits and of the database users' time limits
// (limits.key.), when a site reached a limit, the administrator's move of
// the running programs into the limits (slim.), and the two Problems now
// lines (pnow.). One part of the screen copy; ../../index.tsx fetches every
// part in parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "siteLimits": bir sitenin programlarının birlikte
// sınırları, ne zaman aşıldıkları ve yöneticinin taşıma işlemi.

export const enSiteLimitsScreens = {
    'limits.key.site.memory_mb': 'Memory of a site’s programs together (MB)',
    'limits.key.site.tasks': 'Processes of a site’s programs together',
    'limits.key.site.cpu_percent': 'Processor for a site’s programs (percent of one core)',
    'limits.key.db.statement_seconds': 'Longest database statement (seconds)',
    'limits.key.db.idle_in_transaction_seconds': 'Longest idle open transaction, PostgreSQL (seconds)',
    'limits.key.db.temp_file_mb': 'Temporary files of one database session, PostgreSQL (MB)',
    'limits.unit.percent': '{n} %',
    'slim.plan.default': 'the default, {value}',

    'slim.title': 'Limits of this site’s programs',
    'slim.intro': 'Everything this site runs as a program shares these limits: its Node.js application, its programs and AI apps, and their installs and builds. Other sites and the server keep what is theirs.',
    'slim.checking': 'Reading the site’s limits…',
    'slim.unreadLimits': 'The site’s limits could not be read.',
    'slim.memory': 'Memory',
    'slim.tasks': 'Processes',
    'slim.cpu': 'Processor',
    'slim.value.tasks': '{n} at once',
    'slim.value.cpu': '{n} % of one core',
    'slim.using': 'In use now: {memory} of memory, {tasks} processes.',
    'slim.who.customer': 'Your provider can raise these limits.',
    'slim.who.line': 'Set by {who}.',
    'slim.who.default': 'the server’s default (Plans, server limits)',
    'slim.noEvents': 'The programs have stayed within their limits.',
    'slim.event.memory': 'The programs used all their memory (noticed {time}). The kernel stopped one of them to keep the server going; {count} times in all. The plan allows {limit}.',
    'slim.event.tasks': 'The programs reached their process limit (noticed {time}). New processes could not start; {count} times in all. The plan allows {limit}.',
    'slim.event.cpu': 'The programs were slowed down at their processor limit (noticed {time}). Nothing was stopped. The plan allows {limit}.',
    'slim.event.repeated': 'This keeps happening.',
    'slim.fix.customer': 'Make the program use less, or ask your provider to raise the limit.',
    'slim.fix.admin': 'Raise the limit where it is set ({who}), or ask the site’s owner to make the program use less.',
    'slim.unread': 'The server did not answer, so what the programs use now and whether they reached a limit is not known. The limits above still hold.',
    'slim.sliceEdited': 'The site’s limits file was changed on the server, so its own values apply and CelikPanel does not write it. To give CelikPanel the file back, the server owner deletes it; the next change writes it again.',
    'slim.units.title': 'Where each program runs',
    'slim.place.in_slice': 'inside these limits',
    'slim.place.outside': 'outside them, as before',
    'slim.place.stopped': 'stopped; it starts inside the limits',
    'slim.place.stopped_outside': 'stopped; it would start outside them, so the move puts it inside',
    'slim.place.elsewhere': 'in another slice the server owner chose',
    'slim.place.absent': 'not on the server',
    'slim.lower': 'On its own it may use {own}; inside the site’s limits it gets at most {site}.',
    'slim.move.title': 'Move the running programs into these limits',
    'slim.move.body': 'Each program running outside the limits restarts once, inside them, and must run and answer again within 20 seconds. If one does not, every program of this site is put back where it ran. A stopped program only gets its new place and stays stopped. Other sites are not touched.',
    'slim.move.button': 'Move into the limits',
    'slim.move.confirm': 'Move {count} programs into this site’s limits?',
    'slim.move.lowerWarn': 'These will have less memory than now: {list}. Raise the site’s memory first if they need it.',
    'slim.move.cancel': 'Cancel',
    'slim.move.go': 'Move them',
    'slim.move.step.write': 'Writing the site’s limits',
    'slim.move.step.restart': 'Restarting each program inside them',
    'slim.move.step.check': 'Checking that each one runs and answers again',
    'slim.move.done': 'Done: the running programs of this site are inside its limits.',
    'slim.move.rolledBack': '{unit} did not run inside the limits, so every program of this site was put back where it ran. {detail}',
    'slim.move.notMoved': 'Not moved: {list}.',
    'slim.move.kept': '{unit} (its file was changed on the server and is kept)',
    'slim.move.older': '{unit} (it runs in an earlier protection; move it into the site sandbox on its own page first)',
    'slim.move.unknown': 'The server did not answer, so the result is unknown. Read this page again before you try again.',
    'slim.move.busy': 'A move is running. This page updates by itself when it is done.',

    'pnow.site_memory_repeated.title': '{domain} keeps using all of its programs’ memory',
    'pnow.site_memory_repeated.body': 'The kernel stopped one of its programs at {value} readings in the last day. The site’s page says what its plan allows: raise the limit there or in the plan, or ask the site’s owner to make the program use less.',
    'pnow.site_memory_repeated.action': 'Open the site',
    'pnow.site_tasks_repeated.title': '{domain} keeps reaching its programs’ process limit',
    'pnow.site_tasks_repeated.body': 'New processes of its programs could not start at {value} readings in the last day. The site’s page says what its plan allows: raise the limit there or in the plan, or ask the site’s owner to look at the program.',
    'pnow.site_tasks_repeated.action': 'Open the site',
    'pnow.disk_limits_counted.title': 'Disk limits are counted, not enforced',
    'pnow.disk_limits_counted.body': 'The file system that holds the sites ({domain}) has no project quotas, so it cannot refuse a write of a full hosting account by itself. CelikPanel measures each account and stops its uploads and, with the “Close” rule, the account. For enforcement by the file system, the server owner puts the sites on a separate volume made with project quotas (ext4 with the quota and project features, or XFS mounted with prjquota). CelikPanel never changes file systems.',
    'pnow.disk_limits_counted.action': 'Open Plans',
    'dashboard.audit.action.site.limits_move': 'moved a site’s running programs into its limits ({target})',
};

export type SiteLimitsScreenKey = keyof typeof enSiteLimitsScreens;
