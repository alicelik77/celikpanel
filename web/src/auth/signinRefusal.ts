/**
 * Why the sign-in form is shown, when the Panel said why (security lane A1, 24 November 2026). The sign-in page
 * shows it in its notice: a 401 with one of the sign-in cookies' codes, or a sign-in the browser did not keep (the
 * sign-in answered, and the next read had no session). Cleared by the next sign-in and by signing out.
 *
 * Giriş formunun neden gösterildiği (Panel söylediyse): giriş sayfası bunu bildiriminde gösterir.
 */
export type SigninRefusal = 'duplicate' | 'beforeUpdate' | 'notKept';

const byCode = new Map<unknown, SigninRefusal>([
    ['SIGNIN_COOKIES_DUPLICATE', 'duplicate'],
    ['SIGNIN_FROM_BEFORE_UPDATE', 'beforeUpdate'],
]);

let current: SigninRefusal | null = null;
const listeners = new Set<() => void>();

export const signinRefusal = (): SigninRefusal | null => current;

export function setSigninRefusal(next: SigninRefusal | null): void {
    if (current === next) return;
    current = next;
    listeners.forEach((listener) => listener());
}

export function subscribeSigninRefusal(listener: () => void): () => void {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
}

/** The refusal a 401 names, read from a copy of its body; null for any other answer. */
export async function refusalOf(response: Response): Promise<SigninRefusal | null> {
    if (response.status !== 401) return null;
    try {
        const body: unknown = await response.clone().json();
        return byCode.get((body as { code?: unknown } | null)?.code) ?? null;
    } catch {
        return null;
    }
}
