// SCREEN catalogue part "refusals": what a refused change says on its screen,
// the database forms in words (dbm., ddb.) and a password shown once
// (secret.), owner feedback 2026-10-31.
// "refusals" ekran parcasi: reddedilen degisikligin cumleleri, veritabani
// formlari ve bir kez gosterilen parola.

export const enRefusalsScreens = {
    "users.createFailed": "The user was not created",
    "users.changeFailed": "The change was not made",
    "plans.saveFailed": "The plan was not saved",
    "plans.deleteFailed": "The plan was not deleted",
    "dnssec.signFailed": "The zone was not signed",
    "domains.deleteFailed": "The domain was not deleted",
    "backup.createFailed": "The backup was not made",
    "backup.restoreFailed": "The backup was not restored",
    "backup.deleteFailed": "The backup was not deleted",
    "backup.downloadFailed": "The backup could not be downloaded",
    "backup.auto.saveFailed": "The backup schedule was not saved",
    "mailauth.autoFailed": "Automatic mail setup was not queued",
    "mailauth.keyFailed": "The DKIM key was not created",
    "mailauth.applyFailed": "The record was not added",
    "mail.quotaFailed": "The quota was not changed",
    "mail.deleteAccountFailed": "The mailbox was not deleted",
    "mail.deleteForwarderFailed": "The forwarder was not deleted",
    "databases.deleteDbFailed": "The database was not deleted",
    "addons.grantFailed": "The add-on was not given",
    "addons.revokeFailed": "The add-on was not taken back",
    "panelCert.failedTechnical": "The certificate for {domain} was not issued. The Panel keeps serving its current certificate. The reason is under Technical details: correct it, then request the certificate again.",

    "secret.shownOnce": "This password is shown only now. Copy it and keep it somewhere safe: it is not stored and cannot be shown again.",
    "secret.copy": "Copy password",
    "secret.saved": "I saved it, close",
    "secret.copied": "Copied.",
    "secret.copyFailed": "Could not copy. Select the password and copy it by hand.",


    "ddb.title": "Databases",
    "ddb.subtitle": "The databases of {domain}",
    "ddb.create": "Create database",
    "ddb.creating": "Creating…",
    "ddb.formTitle": "New database",
    "ddb.name": "Database name",
    "ddb.prefix": "Its full name will be {name}",
    "ddb.type": "Database engine",
    "ddb.password": "Password",
    "ddb.passwordPlaceholder": "A strong password",
    "ddb.user": "User:",
    "ddb.createdAt": "Created:",
    "ddb.namePasswordRequired": "Enter a name and a password.",
    "ddb.created": "Database {name} created",
    "ddb.createFailed": "The database was not created",
    "ddb.confirmDelete": "Delete the database {name}? All its data is lost; this cannot be undone.",
    "ddb.deleted": "Database {name} deleted",
    "ddb.deleteFailed": "The database was not deleted",
    // alpha.133 round 5: a database engine's refusals and a database deletion
    // that did not finish, in the order why, who acts, the next step, how it
    // goes on (D-024). ".administrator" is the administrator's own sentence:
    // they are the one who acts, and the button takes them there.
    "err.DATABASE_ENGINE_CREDENTIAL_REFUSED": "{engine} did not accept CelikPanel’s own account, so nothing was changed. Only the server administrator can fix this: ask them to give CelikPanel’s account on {engine} a new password. Then try again.",
    "err.DATABASE_ENGINE_CREDENTIAL_REFUSED.administrator": "{engine} did not accept the password CelikPanel holds for its own account there, so nothing was changed. You fix it: open Components → {engine} and, under “The account CelikPanel connects to this service with”, choose More → “New password…” (for an engine on another machine, set the account’s password there). Then try again.",
    "err.DATABASE_ENGINE_CREDENTIAL_REFUSED.action": "Open the service page",
    "err.DATABASE_ENGINE_UNREACHABLE": "{engine} did not answer, so nothing was changed. Only the server administrator can fix this: ask them to start {engine}. Then try again.",
    "err.DATABASE_ENGINE_UNREACHABLE.administrator": "{engine} did not answer at the address recorded for it, so nothing was changed. You fix it: start it on Components → {engine} (for an engine on another machine, start it there or correct its address). Then try again.",
    "err.DATABASE_ENGINE_UNREACHABLE.action": "Open the service page",
    "err.DATABASE_DELETE_FAILED": "The database server did not delete {name}, so nothing was changed. Try again; if it is refused again, the server administrator finds the reason in CelikPanel’s log.",
    "err.DATABASE_DELETE_UNCONFIRMED": "No answer came back, so it is not known whether {name} was deleted. Nothing is sent again by itself; the list is being read again. If {name} is still listed, press Delete again: it continues where it stopped.",
    "err.DATABASE_DELETE_USERS_LEFT": "{name} was deleted, but its users {users} were not removed yet, so it stays in the list. Press Delete again to finish; if it is refused again, the server administrator finds the reason in CelikPanel’s log.",
    "err.DATABASE_DELETE_USERS_LEFT.unknown": "{name} was deleted; for its users {users} no answer came back, so it is not known whether they were removed. It stays in the list until they are: press Delete again to finish.",
    "err.DATABASE_DELETE_RECORDS_LEFT": "{name} was deleted on the database server, but CelikPanel’s records of it were not removed, so it is still listed. Press Delete again to finish; nothing else is needed.",
} as const;

export type RefusalsScreenKey = keyof typeof enRefusalsScreens;
