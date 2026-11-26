// The words of database users the database limits did not reach (alpha.149,
// review of PR #370 finding 18): the Problems now line, its Apply again and
// what a plan's Save says. Fetched with the screens that show them
// (../dbLimits.ts), never with every screen.
//
// Sınırları ulaşmayan veritabanı kullanıcılarının metinleri; yalnız onları
// gösteren ekranlarla getirilir.

export const enDbLimitsWords = {
    'pnow.database_limits_unapplied.title': 'Database limits did not reach {value} database users: {domain}',
    'pnow.database_limits_unapplied.title_record': 'The list of database users the database limits did not reach could not be read',
    'pnow.database_limits_unapplied.agent': 'CelikPanel’s Agent did not answer, so nothing was sent and these users keep the limits they had. The administrator checks the Agent (sudo systemctl status celikpanel-agent), then presses Apply again; the hourly round also tries again.',
    'pnow.database_limits_unapplied.prerequisite': 'Their database server is not running, so nothing was changed and they keep the limits they had. The server owner starts it on the server (for example sudo systemctl start postgresql or mariadb), then presses Apply again; the hourly round also tries again.',
    'pnow.database_limits_unapplied.connection': 'Their database server is running but refused the connection, so nothing was changed. The administrator checks the database server, then presses Apply again; the hourly round also tries again.',
    'pnow.database_limits_unapplied.refused': 'CelikPanel’s Agent did not change these users, because they are not ordinary site database users; nothing was changed. The server’s administrator decides about these accounts; Apply again tries once more.',
    'pnow.database_limits_unapplied.failed': 'The database server refused a limit for these users; a refused limit was not written. Remove the cause, then press Apply again; the hourly round also tries again.',
    'pnow.database_limits_unapplied.unknown': 'Whether these users have the plan’s limits is not known. Press Apply again: it reads each user first and writes again only a limit that is not as the plan says; this line stays until the values are read back.',
    'pnow.database_limits_unapplied.why': 'Why: {reason}. The detail is in the panel’s journal.',
    'pnow.database_limits_unapplied.reason.agent_unreachable': 'the Agent did not answer',
    'pnow.database_limits_unapplied.reason.server_not_running': 'the database server is not running',
    'pnow.database_limits_unapplied.reason.connection_refused': 'the database server refused the connection (sign-in refused, too many connections, or still starting)',
    'pnow.database_limits_unapplied.reason.refused_privileged': 'the user holds administrator rights on the database server',
    'pnow.database_limits_unapplied.reason.refused_member': 'the user is a member of another PostgreSQL role',
    'pnow.database_limits_unapplied.reason.refused_not_recorded': 'the user is not one of CelikPanel’s site database users on that server',
    'pnow.database_limits_unapplied.reason.refused_absent': 'the user is not on the database server',
    'pnow.database_limits_unapplied.reason.engine_refused': 'the database server refused a statement',
    'pnow.database_limits_unapplied.reason.result_unknown': 'the answer was lost',
    'pnow.database_limits_unapplied.reason.not_known_yet': 'the panel restarted before it reached these users',
    'pnow.database_limits_unapplied.reason.server_record_unreadable': 'the database server’s record could not be read',
    'pnow.database_limits_unapplied.reason.record_unreadable': 'the panel’s list of these users could not be read; Apply again builds it again from every database user',
    'pnow.database_limits_unapplied.action': 'Apply again',
    'pnow.database_limits_unapplied.step': 'Giving the database users their limits',
    'pnow.database_limits_unapplied.done': 'The listed database users have their limits now.',
    'pnow.database_limits_unapplied.still': '{n} database users still do not have their limits; the line says why.',
    'pnow.database_limits_unapplied.notKnownYet': 'The panel no longer runs the Apply again this window followed (it was restarted), so whether the users have their limits is not known yet. Press Apply again.',
    'plans.dbLimits.title': 'Saving the plan',
    'plans.dbLimits.step': 'Applying the limits to the database users…',
    'plans.dbLimits.confirmFirst': 'Nothing was saved yet: confirm what the change does first.',
    'plans.dbLimits.lost': 'The answer was lost, so whether the plan was saved is not known; the list is read again before anything is sent again.',
    'plans.dbLimits.saved': 'The plan is saved, and every database user has its database limits.',
    'plans.dbLimits.notApplied': 'Saved. The plan’s database limits did not reach {n} database users ({users}). Problems now names them, says why and offers Apply again.',
    'plans.dbLimits.notAppliedReseller': 'Saved. The plan’s database limits did not reach {n} database users ({users}) yet. The server’s administrator sees to it; nothing is needed from you.',
    'plans.dbLimits.running': 'Saved. {n} database users are still being given the plan’s database limits; Problems now names any they do not reach.',
    'plans.dbLimits.runningReseller': 'Saved. {n} database users are still being given the plan’s database limits; the server’s administrator sees to any they do not reach; nothing is needed from you.',
};

export type DbLimitsWordKey = keyof typeof enDbLimitsWords;
