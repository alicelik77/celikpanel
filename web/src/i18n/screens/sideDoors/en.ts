// SCREEN catalogue part "sideDoors" (20 Nov 2026, S4 R1): what the side
// doors step at each panel start left for someone in Problems now: a
// database tool or webmail still under the web server's account, and the
// distribution's PHP-FPM pool kept on (pnow.), and why a tool's move to
// its own account failed (component.phpTool.cause.own_account_failed).
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "sideDoors": araçların kendi hesaplarına taşınması
// ve dağıtımın varsayılan PHP-FPM havuzu için Şu anki sorunlar satırları.

export const enSideDoorsScreens = {
    'pnow.tool_account_failed.title': '{name} still runs under the web server’s account',
    'pnow.tool_account_failed.body': 'That account can read every site’s files. Moving {name} to an account of its own did not work, so everything was put back as it was. Open it and choose its PHP version again: CelikPanel tries the move once more and says what stopped it.',
    'pnow.tool_account_failed.action': 'Open it',
    'pnow.tool_account_owner_config.title': '{name} still runs under the web server’s account',
    'pnow.tool_account_owner_config.body': '{domain} was changed on the server, so CelikPanel keeps it and does not move {name} to an account of its own. The server administrator moves those changes to another file in /etc/nginx/conf.d and deletes this one; CelikPanel writes it again and moves {name} after the panel’s next start.',
    'pnow.tool_account_owner_config.action': 'Open it',
    'pnow.stock_pool_in_use.title': 'The distribution’s PHP {name} pool is still on',
    'pnow.stock_pool_in_use.body': 'It runs as the web server’s account, which can read every site, and {domain} may still hand PHP to it, so CelikPanel keeps it. The server administrator gives that configuration a pool of its own or moves the site into CelikPanel. Then the pool is turned off after the panel’s next start; a pool someone put back on the server stays on until the administrator chooses Turn it off again.',
    'pnow.stock_pool_in_use.action': 'Open PHP-FPM',
    'pnow.stock_pool_owner_changed.title': 'The distribution’s PHP {name} pool is still on',
    'pnow.stock_pool_owner_changed.body': '{domain} was changed on the server, so CelikPanel keeps it. It runs as the web server’s account, which can read every site. If nothing needs it, the server administrator renames it to www.conf.off and reloads PHP-FPM {name}.',
    'pnow.stock_pool_owner_changed.action': 'Open PHP-FPM',
    'component.phpTool.cause.own_account_failed': 'it could not be given an account of its own on PHP {version}, so it was left as it was',
    'dashboard.audit.action.php_tool.account.moved': 'moved a database tool or webmail to its own account ({target})',
    'dashboard.audit.action.php_tool.account.failed': 'could not move a database tool or webmail to its own account; it was put back ({target})',
    'dashboard.audit.action.php_fpm.stock_pool.turned_off': 'turned off the distribution’s unused PHP-FPM pool of PHP {target}',
    'dashboard.audit.action.php_fpm.stock_pool.failed': 'could not turn off the distribution’s PHP-FPM pool of PHP {target}; it was put back',
};

export type SideDoorsScreenKey = keyof typeof enSideDoorsScreens;
