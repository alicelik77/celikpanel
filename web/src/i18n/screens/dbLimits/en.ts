// SCREEN catalogue part "dbLimits" (alpha.149 fix round B): the Problems now
// line of database users the database limits did not reach, its Apply again,
// and what a plan's Save says about them (pnow.database_limits_unapplied.,
// plans.dbLimits.). A new part, as "problemsNow" and "siteLimitsApply" have
// no room; no other part's budget is raised.
//
// EKRAN katalogu parçası "dbLimits": sınırları ulaşmayan veritabanı
// kullanıcıları, Yeniden uygula ve planın Kaydet yanıtı.

export const enDbLimitsScreens = {
    'pnow.database_limits_unapplied.title': 'Database limits did not reach {value} database users: {domain}',
    'pnow.database_limits_unapplied.failed': 'The database server or CelikPanel’s Agent refused the change, so these users keep their earlier limits. Why: sudo journalctl -u celikpanel-panel. Remove that cause, then press Apply again; the next change of a plan’s database limits also tries again.',
    'pnow.database_limits_unapplied.prerequisite': 'Their database server or CelikPanel’s Agent was not running, so nothing was changed and they keep their earlier limits. The server owner starts it on the server (for example sudo systemctl start mariadb or postgresql); then press Apply again.',
    'pnow.database_limits_unapplied.unknown': 'The answer was lost, so whether these users have their limits is not known. Press Apply again: it reads each user first and writes only what is missing.',
    'pnow.database_limits_unapplied.action': 'Apply again',
    'pnow.database_limits_unapplied.step': 'Giving the database users their limits',
    'pnow.database_limits_unapplied.done': 'The listed database users have their limits now.',
    'pnow.database_limits_unapplied.still': '{n} database users still do not have their limits; the line says why.',
    'plans.dbLimits.notApplied': 'Saved. The plan’s database limits did not reach {n} database users ({users}); they keep their earlier limits. Problems now names them, with Apply again.',
    'plans.dbLimits.running': 'Saved. {n} database users are still being given the plan’s database limits; Problems now names any they do not reach.',
};

export type DbLimitsScreenKey = keyof typeof enDbLimitsScreens;
