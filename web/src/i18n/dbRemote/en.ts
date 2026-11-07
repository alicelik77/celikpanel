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
    'dbr.off.admin': 'Connections from outside are off on this server. When you turn them on, each customer whose plan includes “Remote access to databases” can list their own addresses.',
    'dbr.off.other': 'Connections from outside are off on this server. Ask your provider if you need them.',
    'dbr.turnOn': 'Turn on for this server',
    'dbr.turnOnConfirm': 'Turn on connections from outside for this server? Nothing opens yet: a database is reachable only after an address is listed for it, and only from that address.',
    'dbr.turnOnTitle': 'Turning on connections from outside',
    'dbr.notInPlan': 'Your plan does not include connections from outside. Ask {grantor} to include “Remote access to databases”.',
    'dbr.grantor.admin': 'the administrator',
    'dbr.remoteServer': 'This database is on another server; CelikPanel opens only the databases of this server.',
    'dbr.subscriptionInactive': 'This subscription is not active, so no address can be listed now.',

    'dbr.problem.RESTART_NEEDED.admin': '{engine} must restart once before it takes connections from outside. Restart it in Components; sites lose their database for a few seconds.',
    'dbr.problem.RESTART_NEEDED.other': 'Waiting for your provider to restart {engine}. You do not need to do anything; the address works after that.',
    'dbr.problem.OWNER_LISTEN.admin': '{engine}’s network address was set on the server by hand ({detail}), so CelikPanel leaves it alone and lists no address. Remove that setting to let CelikPanel manage connections from outside.',
    'dbr.problem.OWNER_LISTEN.other': '{engine} on this server is set up by hand, so addresses cannot be listed here. Ask your provider.',
    'dbr.problem.OWNER_HBA.admin': 'PostgreSQL already lets network connections in through a line of its own: {detail}. Until that line is removed or narrowed, CelikPanel opens nothing, so no customer can connect without encryption through it.',
    'dbr.problem.OWNER_HBA.other': 'PostgreSQL on this server is set up by hand for network connections, so addresses cannot be opened here. Ask your provider.',
    'dbr.problem.OWNER_FILE.admin': 'A CelikPanel file was changed by hand ({detail}); CelikPanel leaves it as it is. Put it back or remove it, then press “Apply again”.',
    'dbr.problem.NFT_MISSING.admin': 'The firewall (nftables) is not installed, so the database port cannot be limited to the listed addresses. Install the firewall in Components first.',
    'dbr.problem.FIREWALL_UNCHANGED.admin': 'The address filter of the database port could not be written: {detail}.',
    'dbr.problem.TLS_MISSING.admin': 'Encrypted connections could not be turned on: {detail}. Without them nobody can connect from outside.',
    'dbr.problem.ALTER_SYSTEM_OFF.admin': 'PostgreSQL refuses ALTER SYSTEM on this server (allow_alter_system = off), so CelikPanel cannot set its network address.',
    'dbr.problem.NO_SLOWDOWN.admin': 'Wrong passwords are not slowed down: {detail}. Install Fail2ban in Components for MariaDB.',
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
    'dbr.addedWaiting': '{address} is listed. It works once {engine} has restarted.',
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

    // Progress steps.
    'opstep.dbRemote.check': 'Checking the address and your plan',
    'opstep.dbRemote.save': 'Saving the address',
    'opstep.dbRemote.apply': 'Setting up the database server',
    'opstep.dbRemote.firewall': 'Updating the firewall',
} as const;

export type DbRemoteKey = keyof typeof enDbRemote;
