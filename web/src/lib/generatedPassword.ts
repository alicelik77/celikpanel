// The browser's password generator moved to lib/passwordPolicy.ts
// (generatePassword, alpha.127): one generator, one rule, for every field.
// This file keeps the shape of a password the server made up.
// Uretec lib/passwordPolicy.ts icine tasindi; burada sunucunun urettigi
// parolanin bicimi kalir.

/** A password the server made up and answers once, for the page to show until closed. */
export interface CreatedSecret {
    user: string;
    password: string;
    database?: string;
    /** A new password for an existing user (not a new user). */
    renewed?: boolean;
}
