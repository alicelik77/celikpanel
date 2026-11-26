import type { ApiError } from './apiError';

// Its own module (not lib/apiError.ts, which is on the critical boot path):
// only the database user screens, loaded with their route, use it.

// Who acts, when a coded refusal's own words name them (alpha.149 lab W1):
// the progress window's heading then names that actor instead of "Waiting for
// someone to act". One rule for every flow that shows the Agent's refusal of
// a database account (DATABASE_ACCOUNT_REFUSED; the panel answers it for
// adding, removing and setting the password of a database user). The
// server's administrator is named only when all three hold:
// - the refusal is about an account that exists (removing it, its password).
//   Making a new account names no one: the person's next step may simply be
//   another name;
// - the reason's own sentence names the administrator: an allow-list of the
//   reasons whose sentences do (and the code with no reason, whose general
//   sentence does). An absent account and the system database name no one; a
//   reason added later keeps the general heading until it is listed here;
// - the viewer is the administrator. The sentence others read names the
//   administrator but does not tell them to ask their provider, so their
//   heading stays the general one, as before.
// Kodlu red mevcut bir hesap içinse, nedeni yöneticiyi adlandırıyorsa ve
// okuyan yöneticiyse başlık yöneticiyi adlandırır; diğer durumlar genel kalır.
const ADMINISTRATOR_NAMING_ACCOUNT_REFUSALS: readonly string[] = ['', 'privileged', 'member', 'unreadable', 'unsupported_engine', 'owns_objects'];
export type RefusedAccountChange = 'existing' | 'new';
export function refusalActor(e: ApiError | null, viewerRole: string | undefined, change: RefusedAccountChange): 'administrator' | undefined {
    if (change !== 'existing' || viewerRole !== 'admin') return undefined;
    if (e?.code === 'DATABASE_ACCOUNT_REFUSED' && ADMINISTRATOR_NAMING_ACCOUNT_REFUSALS.includes(e.reason ?? '')) return 'administrator';
    return undefined;
}
