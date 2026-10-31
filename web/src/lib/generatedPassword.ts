// A password made in the browser for a new database user: 20 characters from
// the cryptographic generator (not the plain pseudo-random one), without characters a
// shell or a connection string would need quoted. It is shown in its field
// and never logged.
// Tarayicida uretilen parola: kriptografik uretecten 20 karakter; kayda yazilmaz.
const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789-_.';

export function generatedPassword(length = 20): string {
    const bytes = new Uint32Array(length);
    crypto.getRandomValues(bytes);
    let out = '';
    for (const value of bytes) out += alphabet[value % alphabet.length];
    return out;
}

/** A password the server made up and answers once, for the page to show until closed. */
export interface CreatedSecret {
    user: string;
    password: string;
    database?: string;
}
