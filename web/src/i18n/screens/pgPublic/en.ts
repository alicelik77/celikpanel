// SCREEN catalogue part "pgPublic" (20 Nov 2026, security lane S1b):
// PostgreSQL databases made before alpha.141 that every account of the server
// can enter, on the PostgreSQL page: what was found, what closing changes,
// what CelikPanel watches, closed or failed to close, and how to undo it
// (alpha.143 fix round: the watch, failures, databases opened again). Key
// prefix: pgpub.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "pgPublic": sunucudaki her hesabın girebildiği eski
// PostgreSQL veritabanları, kapatma ve geri alma.

export const enPgPublicScreens = {
    'pgpub.title': 'Databases other accounts can enter',
    'pgpub.checking': 'Reading which databases other accounts can enter…',
    'pgpub.failed': 'Which databases other accounts can enter could not be read. Nothing was changed.',
    'pgpub.open.title': 'Every account of this server can enter these databases: {n}',
    'pgpub.open.why': 'They were made before alpha.141, when PostgreSQL’s default let every database user of the server connect to every database and make temporary tables in it. One customer’s database user can sign in to another customer’s database and see how it is built.',
    'pgpub.open.who': 'The administrator',
    'pgpub.open.next': 'Read under each database what was found and what closing changes, then press Close to other accounts.',
    'pgpub.open.then': 'Each database closes in one step that changes nothing unless it is still as read here. You can undo it here at any time.',
    'pgpub.found.observing': 'Watched since {since}: if no account it does not record for it is seen connected at an hourly check, CelikPanel closes it by itself at the first check after {until}. You can close it now.',
    'pgpub.found.close_failed': 'Closing it failed ({time}): {reason}. Correct what this names, then press Close to other accounts. It is not tried every hour, only when something about it changes.',
    'pgpub.found.unknown_result': 'A close was sent and its result is not confirmed yet; it is not sent again. It is not closed by itself.',
    'pgpub.found.changed_rights': 'Its rights were changed outside CelikPanel: {acl}',
    'pgpub.found.other_roles': 'Accounts CelikPanel does not record for it own something or hold rights inside it: {roles}.',
    'pgpub.found.other_sessions': 'Accounts CelikPanel does not record for it were seen connected to it: {roles}.',
    'pgpub.found.missing_users': 'Users CelikPanel records for it have no PostgreSQL account: {roles}. Add them again or remove them from the database, then close it.',
    'pgpub.found.monitoring_roles': 'Monitoring accounts exist on this server: {roles}.',
    'pgpub.found.replication_roles': 'Replication accounts exist on this server and may read it through a connection: {roles}.',
    'pgpub.reach.both': 'Every account of this server can connect to it and make temporary tables in it.',
    'pgpub.reach.connect': 'Every account of this server can connect to it.',
    'pgpub.reach.temporary': 'Accounts that may connect by another right can make temporary tables in it.',
    'pgpub.owner': 'Owner: {owner}',
    'pgpub.gives': 'After closing, these may connect besides its owner: {users}',
    'pgpub.gives.none': 'After closing, only its owner may connect.',
    'pgpub.user.write': '{user} (also temporary tables)',
    'pgpub.close': 'Close to other accounts',
    'pgpub.confirm.title': 'Close {name} to other accounts?',
    'pgpub.confirm.body': 'PostgreSQL stops letting every account of this server connect to {name} and make temporary tables in it. Its owner and the users CelikPanel records for it keep their way in.',
    'pgpub.confirm.lost': 'Every other account of this server can no longer connect to it or make temporary tables in it.',
    'pgpub.confirm.lostRoles': 'These accounts can no longer connect to it: {roles}. Connections open now stay until they end.',
    'pgpub.confirm.sessions': 'Connected at the last reading: {roles}. Their connections stay until they end; they cannot connect again.',
    'pgpub.confirm.kept': 'Its data, its owner {owner} and everything inside it stay as they are.',
    'pgpub.confirm.keptUndo': 'You can undo it here, or with the statements shown under it.',
    'pgpub.progress': 'Closing {name} to other accounts',
    'pgpub.done': '{name} is closed to other accounts.',
    'pgpub.unknown': 'The server did not answer, so whether {name} changed is not known. The list is read again and shows what it holds now.',
    'pgpub.closed.title': 'Closed to other accounts',
    'pgpub.closed.panel': 'Closed by CelikPanel by itself, {time}',
    'pgpub.closed.administrator': 'Closed by an administrator, {time}',
    'pgpub.undo': 'Undo',
    'pgpub.undo.title': 'Open {name} to every account again?',
    'pgpub.undo.body': 'This gives back exactly what the close changed. CelikPanel then does not close it by itself again; it lists it as opened again by an administrator.',
    'pgpub.undo.lost': 'Every account of this server can connect to {name} again.',
    'pgpub.undo.kept': 'Its data, its owner and the rights inside it stay as they are.',
    'pgpub.undo.progress': 'Opening {name} again',
    'pgpub.undo.done': '{name} is open to every account again, as before the close.',
    'pgpub.statements': 'Undo without the panel',
    'pgpub.statements.how': 'Run these as the postgres account (sudo -u postgres psql -p {port}):',
    'pgpub.unread.title': 'Could not be read now: {name}',
    'pgpub.unread.next': 'It is read again at the next hourly check, or when you press Read again. Nothing was changed.',
    'pgpub.reread': 'Read again',
    'pgpub.readAt': 'Read {time}.',
    'pgpub.reopened.title': 'Opened again by an administrator',
    'pgpub.reopened.at': 'Opened again {time}: the administrator’s choice. CelikPanel does not close it by itself.',
    'err.POSTGRESQL_PUBLIC_WAITING': 'The panel and its Agent are different builds while an update finishes. Nothing was changed; try again in a minute.',
    'err.POSTGRESQL_PUBLIC_CHANGED': 'The database is no longer as the list showed it, so nothing was changed. The list is read again; read it, then decide.',
    'err.POSTGRESQL_PUBLIC_FAILED': 'The database server did not do it. Nothing was changed.',
    // alpha.144 (alpha.143 lab): the kind of cause the database server's
    // message names; its message follows quoted.
    'err.POSTGRESQL_PUBLIC_FAILED.refused': 'The database server refused it. Nothing was changed.',
    'err.POSTGRESQL_PUBLIC_FAILED.in_use': 'The database is in use: the database server did not grant the lock it needed in time. Nothing was changed. Try again when the connections that hold it have ended.',
    'err.POSTGRESQL_PUBLIC_FAILED.timeout': 'The database server took too long and stopped it. Nothing was changed. Try again.',
    'err.POSTGRESQL_PUBLIC_FAILED.connection': 'The connection to the database server was lost. Nothing was changed. Check that PostgreSQL runs, then try again.',
    'err.POSTGRESQL_PUBLIC_FAILED.rights': 'The database server did not allow it: a right was missing. Nothing was changed.',
    'err.POSTGRESQL_PUBLIC_FAILED.missing': 'Something it names does not exist on the database server (a user or a role). Nothing was changed.',
    'pgpub.step.notDone': 'Not done — nothing was changed',
    'pgpub.step.unknown': 'Result not confirmed yet — not sent again',
    'pgpub.why.server.in_use': 'the database was in use and the database server did not grant the lock in time; nothing was changed: “{detail}”',
    'pgpub.why.server.timeout': 'the database server took too long and stopped it; nothing was changed: “{detail}”',
    'pgpub.why.server.connection': 'the connection to the database server was lost; nothing was changed: “{detail}”',
    'pgpub.why.server.rights': 'the database server did not allow it (a right was missing); nothing was changed: “{detail}”',
    'pgpub.why.server.missing': 'something it names does not exist on the database server; nothing was changed: “{detail}”',
    'err.POSTGRESQL_PUBLIC_UNKNOWN': 'The database could not be reached, so whether it changed is not known. The list is read again and shows what it holds now.',
};

export type PgPublicScreenKey = keyof typeof enPgPublicScreens;
