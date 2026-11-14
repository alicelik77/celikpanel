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
    "ddb.tooLong": "This name is too long: make it at least {over} characters shorter.",
    "ddb.tooLongUnderscores": "This name is too long: make it at least {over} characters shorter, or use fewer underscores (each underscore counts as two characters).",
    "ddb.nameCharacters": "The name may hold only the letters a–z, digits and underscores (no accented letters, hyphens or spaces).",
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
} as const;

export type RefusalsScreenKey = keyof typeof enRefusalsScreens;
