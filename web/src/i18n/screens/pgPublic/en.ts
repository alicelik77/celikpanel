// SCREEN catalogue part "pgPublic" (20 Nov 2026, security lane S1b):
// PostgreSQL databases made before alpha.141 that every account of the server
// can enter, on the PostgreSQL page: what was found, what closing changes,
// what CelikPanel closed by itself and how to undo it. Key prefix: pgpub.
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
    'pgpub.found.waiting': 'Nothing stands in the way: CelikPanel closes it by itself at its next hourly check. You can close it now.',
    'pgpub.found.undone': 'An administrator opened it again after it was closed, so CelikPanel does not close it by itself.',
    'pgpub.found.changed_rights': 'Its rights were changed outside CelikPanel: {acl}',
    'pgpub.found.other_roles': 'Accounts CelikPanel does not record for it own something or hold rights inside it: {roles}.',
    'pgpub.found.other_sessions': 'Connected now with accounts CelikPanel does not record for it: {roles}.',
    'pgpub.found.monitoring_roles': 'Monitoring accounts exist on this server: {roles}.',
    'pgpub.owner': 'Owner: {owner}',
    'pgpub.gives': 'After closing, these may connect besides its owner: {users}',
    'pgpub.gives.none': 'After closing, only its owner may connect.',
    'pgpub.user.write': '{user} (also temporary tables)',
    'pgpub.close': 'Close to other accounts',
    'pgpub.confirm.title': 'Close {name} to other accounts?',
    'pgpub.confirm.body': 'PostgreSQL stops letting every account of this server connect to {name} and make temporary tables in it. Its owner and the users CelikPanel records for it keep their way in.',
    'pgpub.confirm.lost': 'Every other account of this server can no longer connect to it or make temporary tables in it.',
    'pgpub.confirm.lostRoles': 'These accounts can no longer connect to it: {roles}. Connections open now stay until they end.',
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
    'pgpub.undo.body': 'This gives back exactly what the close changed. CelikPanel then does not close it by itself again; it lists it here instead.',
    'pgpub.undo.lost': 'Every account of this server can connect to {name} again.',
    'pgpub.undo.kept': 'Its data, its owner and the rights inside it stay as they are.',
    'pgpub.undo.progress': 'Opening {name} again',
    'pgpub.undo.done': '{name} is open to every account again, as before the close.',
    'pgpub.statements': 'Undo without the panel',
    'pgpub.statements.how': 'Run these as the postgres account (sudo -u postgres psql -p {port}):',
    'pgpub.unread.title': 'Could not be read now: {name}',
    'pgpub.unread.next': 'It is read again at the next hourly check, or when you press Read again. Nothing was changed.',
    'pgpub.reread': 'Read again',
    'err.POSTGRESQL_PUBLIC_WAITING': 'The panel and its Agent are different builds while an update finishes. Nothing was changed; try again in a minute.',
    'err.POSTGRESQL_PUBLIC_CHANGED': 'The database is no longer as the list showed it, so nothing was changed. The list is read again; read it, then decide.',
    'err.POSTGRESQL_PUBLIC_FAILED': 'The database server did not do it. Nothing was changed.',
    'err.POSTGRESQL_PUBLIC_UNKNOWN': 'The database could not be reached, so whether it changed is not known. The list is read again and shows what it holds now.',
};

export type PgPublicScreenKey = keyof typeof enPgPublicScreens;
