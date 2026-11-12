// Route-scoped copy of remote access to databases (alpha.129,
// docs/DATABASE-REMOTE-ACCESS.md): the "Connections from outside" part of a
// database and its progress steps (opstep.dbRemote*). It ships with the
// database screen's own chunk, so the critical boot path does not grow;
// ../dbRemote.ts looks keys up in the I18nProvider's locale.
//
// Veritabanına dışarıdan bağlantı metni: ekranın paketiyle gelir.

export const enDbRemote = {
    'dbr.title': 'Connections from outside',
    'dbr.summary.off': 'off',
    'dbr.summary.count': '{n} addresses',
    'dbr.summary.one': '1 address',
    'dbr.intro': 'Connect with pgAdmin, DBeaver or HeidiSQL from your own computer. Only the addresses you list here can connect, and only with an encrypted connection.',
    'dbr.reading': 'Reading the connections from outside…',
    'dbr.unread': 'The connections from outside could not be read just now. Nothing was changed; try again in a moment.',
    'dbr.retry': 'Try again',

    // Who acts, and what to do.
    'dbr.off.admin': 'Connections from outside are off on this server. You turn them on in Security › Open to the outside; then each customer whose plan includes “Remote access to databases” can list their own addresses.',
    'dbr.openSwitch': 'Open the setting in Security',
    'dbr.off.other': 'Connections from outside are off on this server. Ask your provider if you need them.',
    'dbr.notInPlan': 'Your plan does not include connections from outside. Ask {grantor} to add “Remote access to databases” to your plan.',
    'dbr.remoteServer': 'This database is on another server; CelikPanel opens only the databases of this server.',
    'dbr.subscriptionInactive': 'This subscription is not active, so no address can be listed now.',

    'dbr.problem.RESTART_NEEDED.admin': '{engine} must restart once before it takes connections from outside. Restart it in Components; sites lose their database for a few seconds.',
    'dbr.problem.RESTART_NEEDED.other': 'Waiting for your provider to restart {engine}. You do not need to do anything; the address works after that.',
    'dbr.problem.OWNER_LISTEN.admin': '{engine}’s network address was set on the server by hand ({detail}), so CelikPanel leaves it alone and lists no address. Remove that setting to let CelikPanel manage connections from outside.',
    'dbr.problem.OWNER_LISTEN.other': 'Connections from outside are not available for this database right now. Your provider can set them up for you.',
    'dbr.problem.OWNER_HBA.admin': 'PostgreSQL already lets network connections in through a line of its own: {detail}. Until that line is removed or narrowed, CelikPanel opens nothing, so no customer can connect without encryption through it.',
    'dbr.problem.OWNER_HBA.other': 'Connections from outside are not available for this database right now. Your provider can set them up for you.',
    'dbr.problem.OWNER_FILE.admin': 'A CelikPanel file was changed by hand ({detail}); CelikPanel leaves it as it is. Put it back or remove it, then press “Apply again”.',
    'dbr.problem.NFT_MISSING.admin': 'The firewall (nftables) is not installed, so the database port cannot be limited to the listed addresses. Install the firewall in Components first.',
    'dbr.problem.FIREWALL_UNCHANGED.admin': 'The address filter of the database port could not be written: {detail}.',
    'dbr.problem.TLS_MISSING.admin': 'Encrypted connections could not be turned on: {detail}. Without them nobody can connect from outside.',
    'dbr.problem.ALTER_SYSTEM_OFF.admin': 'PostgreSQL refuses ALTER SYSTEM on this server (allow_alter_system = off), so CelikPanel cannot set its network address.',
    'dbr.problem.NO_SLOWDOWN.admin': 'Connections wait until wrong passwords are slowed down: {detail}.',
    'dbr.problem.NO_SLOWDOWN.other': 'Waiting for your provider to switch on protection against password guessing. You do not need to do anything; your addresses are kept and work after that.',
    'dbr.problem.ENGINE_DOWN.admin': '{engine} does not answer: {detail}. Start it in Components; the addresses apply when it runs.',
    'dbr.problem.ENGINE_DOWN.other': '{engine} does not answer right now. Your provider has been shown the problem.',
    'dbr.problem.FAILED.admin': 'A step did not finish: {detail}.',
    'dbr.problem.other': 'Something on the server needs your provider’s attention. Your addresses are kept.',
    'dbr.components': 'Open Components',
    'dbr.applyAgain': 'Apply again',
    'dbr.applyAgainTitle': 'Applying connections from outside again',

    // The list.
    'dbr.address': 'Address',
    'dbr.user': 'User',
    'dbr.status.active': 'Allowed',
    'dbr.status.waiting': 'Being applied',
    'dbr.status.refused': 'Not applied',
    'dbr.status.not_offered': 'Off on this server',
    'dbr.status.not_in_plan': 'Not in your plan',
    'dbr.status.no_access': 'User has no access to this database',
    'dbr.status.other': 'Not applied',
    'dbr.remove': 'Remove {address}',
    'dbr.removeConfirm': 'Stop connections to {database} from {address}? Programs connected from there lose the connection.',
    'dbr.removeTitle': 'Removing {address}',
    'dbr.removed': '{address} can no longer connect to {database}.',
    'dbr.empty': 'No address is listed: nobody can connect from outside.',

    // Adding.
    'dbr.addHeading': 'Allow an address',
    'dbr.addressPlaceholder': '203.0.113.7',
    'dbr.addressHint': 'Your computer’s or office’s public IP address, or a small range such as 203.0.113.0/24. At most {n} addresses per user.',
    'dbr.useMine': 'Use my current address ({address})',
    'dbr.add': 'Allow',
    'dbr.addTitle': 'Allowing {address}',
    'dbr.added': '{address} can connect to {database} now.',
    'dbr.addedWaiting': '{address} is listed. It works as soon as the server is ready; the note above says what is still missing.',
    'dbr.noUsers': 'This database has no user yet; create one first.',
    'dbr.addFailed': 'The address was not added.',
    'dbr.removeFailed': 'The address was not removed.',
    'dbr.turnOnFailed': 'Connections from outside were not turned on.',

    // Client settings.
    'dbr.settings': 'Settings for your program',
    'dbr.host': 'Host',
    'dbr.port': 'Port',
    'dbr.database': 'Database',
    'dbr.userName': 'User',
    'dbr.password': 'Password',
    'dbr.passwordValue': 'the password of this database user',
    'dbr.sslMode': 'SSL mode',
    'dbr.sslRequire': 'require',
    'dbr.tool.pgadmin': 'pgAdmin: Register → Server; Connection: host, port, database, user; Parameters: SSL mode = require.',
    'dbr.tool.dbeaver': 'DBeaver: New connection; Main: host, port, database, user; SSL: Use SSL, mode require (no certificate files needed).',
    'dbr.tool.heidisql': 'HeidiSQL: Network type MariaDB or MySQL (TCP/IP); host, user, port; Advanced: Use SSL ticked, certificate fields empty.',

    // alpha.130: timed access.
    'dbr.until': 'until {until}',
    'dbr.noEnd': 'no end date',
    'dbr.duration.1d': 'For 1 day',
    'dbr.duration.7d': 'For 7 days',
    'dbr.duration.permanent': 'No end date',
    'dbr.durationLabel': 'How long',
    'dbr.durationHint': 'When the chosen time is over, the address is removed by itself; you can extend it before then.',
    'dbr.extend': 'Change end…',
    'dbr.extendLabel': 'New end for {address}',
    'dbr.extendTitle': 'Changing how long {address} is allowed',
    'dbr.extended': '{address} is allowed until {until}.',
    'dbr.extendedPermanent': '{address} is allowed with no end date.',
    'dbr.extendFailed': 'The end was not changed.',
    // alpha.131: a new end is saved only with Apply.
    'dbr.extendApply': 'Apply',
    'dbr.extendDiscard': 'Discard',
    'dbr.status.expired': 'Ended; being removed',

    // alpha.130: a closing that waits for the server (D5).
    'dbr.closePending.admin': 'This server has not finished starting, so {engine} has not taken the last change yet. The removed addresses are already blocked at the firewall; their database access is removed as soon as the server is ready. If this lasts, run `systemctl is-system-running` and `systemctl list-jobs` on the server to see what keeps it from finishing.',
    'dbr.closePending.other': 'The removed addresses are already blocked. The rest finishes by itself when the server is ready; you do not need to do anything.',

    // alpha.130: MariaDB host name lookup.
    'dbr.problem.NAME_RESOLVE.admin': 'Connections wait: MariaDB looks up the host name of every client, so a wrong password is logged with a name and Fail2ban cannot block that address. Let CelikPanel turn host name lookup off; it takes effect when MariaDB restarts.',
    'dbr.problem.NAME_RESOLVE.other': 'Waiting for your provider to finish the protection against password guessing. You do not need to do anything; your addresses are kept and work after that.',
    'dbr.problem.OWNER_NAME_RESOLVE.admin': 'Connections wait: {detail}. Remove that setting (or turn skip-name-resolve on yourself) and restart MariaDB.',
    'dbr.problem.OWNER_NAME_RESOLVE.other': 'Waiting for your provider to finish the protection against password guessing. You do not need to do anything; your addresses are kept.',
    'dbr.problem.NAME_ACCOUNTS.admin': 'Connections wait: host name lookup cannot be turned off yet, because these MariaDB accounts name a host instead of an address and would stop working: {detail}. Change them to an address or remove them, then press “Apply again”.',
    'dbr.problem.NAME_ACCOUNTS.other': 'Waiting for your provider to finish the protection against password guessing. You do not need to do anything; your addresses are kept.',
    'dbr.nameLookupOff': 'Turn off host name lookup',
    'dbr.nameLookupTitle': 'Turning off MariaDB host name lookup',
    'dbr.nameLookupConfirm': 'Turn off host name lookup in MariaDB? MariaDB then recognises every connection by its address. Programs on this server that connect through “localhost” keep working; a program that connects through 127.0.0.1 or a host name needs an account for that address. It takes effect when you restart MariaDB in Components, and it is undone when connections from outside are turned off for this server.',

    // Progress steps.
    'opstep.dbRemote.check': 'Checking the address and your plan',
    'opstep.dbRemote.save': 'Saving the address',
    'opstep.dbRemote.apply': 'Setting up the database server',
    'opstep.dbRemote.firewall': 'Updating the firewall',
} as const;

export type DbRemoteKey = keyof typeof enDbRemote;
