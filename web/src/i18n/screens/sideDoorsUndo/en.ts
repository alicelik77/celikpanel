// SCREEN catalogue part "sideDoorsUndo" (21 Nov 2026, S4 R1 fix round):
// what the side doors step says beyond the first round. A database tool or
// webmail that does not answer, the distribution's PHP-FPM pool that could
// not be turned off, put back or read, the server owner's undo (a pool put
// back, CelikPanel's /etc/at.allow deleted) with the administrator's
// explicit "close it again", a site crontab that cannot be read, the
// administrator's Keep on a crontab line not made in CelikPanel, and a tool
// that could not get its own account at its install. A new part beside
// sideDoors and siteJobs, which have no room.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "sideDoorsUndo": yan kapılar adımının ikinci tur
// satırları; sahibin geri aldığı ve yöneticinin açıkça yeniden kapattığı.

export const enSideDoorsUndoScreens = {
    'pnow.tool_down.title': '{name} does not answer',
    'pnow.tool_down.body': 'It runs on PHP {domain} and does not answer as it is set up now. CelikPanel tried what it could: it moved it again or reloaded the web server once, and put back what it could not finish. The administrator opens it and chooses its PHP version again: CelikPanel moves it once more and says what stopped it.',
    'pnow.tool_down.action': 'Open it',
    'pnow.stock_pool_not_put_back.title': 'The distribution’s PHP {name} pool could not be put back',
    'pnow.stock_pool_not_put_back.body': 'Turning it off failed, and putting it back under its own name failed too, so {domain} is missing and a configuration that used it may not answer. The server administrator renames {domain}.off-by-celikpanel back to {domain} and reloads PHP-FPM {name}.',
    'pnow.stock_pool_not_put_back.action': 'Open PHP-FPM',
    'pnow.stock_pool_failed.title': 'The distribution’s PHP {name} pool is still on',
    'pnow.stock_pool_failed.body': 'PHP-FPM {name} did not take the change when CelikPanel turned {domain} off, so it was put back as it was. It runs as the web server’s account, which can read every site. CelikPanel tries again after the panel’s next start; if it keeps failing, the server administrator runs php-fpm{name} -t to see why.',
    'pnow.stock_pool_failed.action': 'Open PHP-FPM',
    'pnow.stock_pool_unknown.title': 'The distribution’s PHP {name} pool is still on',
    'pnow.stock_pool_unknown.body': 'CelikPanel could not tell whether {domain} is untouched and unused (the package’s record of it, the web server’s configuration, or a {domain}.off-by-celikpanel already beside it), so it keeps it on. The server administrator checks that nginx -t passes and keeps only one of the two files; CelikPanel decides again after the panel’s next start.',
    'pnow.stock_pool_unknown.action': 'Open PHP-FPM',
    'pnow.stock_pool_owner_restored.title': 'The distribution’s PHP {name} pool was put back on the server',
    'pnow.stock_pool_owner_restored.body': 'CelikPanel had turned {domain} off because nothing used it. It is back under its own name, so someone on the server put it back, and CelikPanel leaves it on. It runs as the web server’s account, which can read every site. If that was not meant, the administrator turns it off again: CelikPanel checks first that nothing uses it.',
    'pnow.stock_pool_owner_restored.action': 'Turn it off again',
    'pnow.stock_pool_owner_restored.confirm': 'CelikPanel checks that {domain} is still the package’s untouched file and that no configuration uses it, then renames it to {domain}.off-by-celikpanel, checks PHP-FPM {name} and reloads it. If PHP-FPM refuses, it is put back as it is now.',
    'pnow.at_owner_removed.title': 'Website accounts can use at again',
    'pnow.at_owner_removed.body': 'CelikPanel’s {domain} was deleted on the server, so /etc/at.deny applies again and a website can plant at jobs that CelikPanel neither shows nor limits. CelikPanel does not write the file again by itself. To close at again, the administrator chooses Close again.',
    'pnow.at_owner_removed.action': 'Close again',
    'pnow.at_owner_removed.confirm': 'CelikPanel writes {domain} anew: every account that may use at today except the website accounts, with /etc/at.deny’s owner and mode. To undo it later, delete the file.',
    'pnow.site_crontab_unreadable.title': 'Some site crontabs cannot be read',
    'pnow.site_crontab_unreadable.body': 'CelikPanel could not read the crontab of {domain} ({name}), so lines added to it outside CelikPanel cannot be seen. The other sites were checked. The server administrator makes it readable (for example, removes the account from /etc/cron.deny); after the panel’s next start its lines are listed until they are kept or removed.',
    'pnow.site_crontab_unreadable.action': 'Open Monitoring',
    'pnow.reclose.progress': 'Closing it again',
    'pnow.reclose.step': 'Checking and changing it on the server',
    'pnow.reclose.done': 'It is closed again.',
    'pnow.reclose.still': 'It was not closed: Problems now says why.',
    'pnow.reclose.failed': 'It could not be closed again now',
    'cron.outside.keep': 'Keep',
    'cron.outside.keepConfirm': 'Keep this line? It stays in the crontab and is no longer marked or listed as not made in CelikPanel. Keep only a line you know.',
    'cron.outside.kept': 'Line kept',
    'services.phpToolProblem.ownAccountFailed': '{tool} could not be given its own account on PHP {version}, so it runs under the web server’s account, which can read every site. Open it and choose its PHP version again to try once more.',
    'dashboard.audit.action.side_doors.reclose': 'closed again what the server owner had undone ({target})',
    'dashboard.audit.action.side_doors.reclose.failed': 'could not close again what the server owner had undone ({target})',
    'dashboard.audit.action.cron.line.keep': 'kept a crontab line not made in CelikPanel',
    'dashboard.audit.action.cron.line.keep.failed': 'could not keep a crontab line not made in CelikPanel',
};

export type SideDoorsUndoScreenKey = keyof typeof enSideDoorsUndoScreens;
