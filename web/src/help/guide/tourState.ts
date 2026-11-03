import type { Role } from '../../auth/AuthContext';
import type { TranslationKey } from '../../i18n/en';

// The first-login tour: what decides whether it is shown, and its steps.
// The server keeps one row per person (GET/PUT /api/v1/me/guide-state); no
// row reads as "pending". The tour is shown only for a known pending state,
// offered (never forced) after TOUR_VERSION is raised, and never while the
// state could not be read: an unknown is not a reason to interrupt anyone.
// İlk giriş turu: ne zaman gösterileceği ve adımları. Bilinmeyen durumda
// gösterilmez; sürüm artınca yalnız önerilir.

/** Raise when the steps change enough to offer the tour again. */
export const TOUR_VERSION = 1;
export const GUIDE_STATE_URL = '/api/v1/me/guide-state';

export type TourState = 'pending' | 'completed' | 'skipped';
export interface GuideState {
    tour_version: number;
    tour_state: TourState;
}

export function decodeGuideState(value: unknown): GuideState | null {
    if (!value || typeof value !== 'object') return null;
    const raw = value as Record<string, unknown>;
    const version = raw.tour_version;
    const state = raw.tour_state;
    if (typeof version !== 'number' || !Number.isInteger(version) || version < 0) return null;
    if (state !== 'pending' && state !== 'completed' && state !== 'skipped') return null;
    return { tour_version: version, tour_state: state };
}

export type TourDecision = 'show' | 'offer' | 'none';

export function tourDecision(state: GuideState | null, impersonating: boolean, version = TOUR_VERSION): TourDecision {
    // Someone signed in as this person must not use up their tour.
    if (state === null || impersonating) return 'none';
    if (state.tour_state === 'pending') return 'show';
    return state.tour_version < version ? 'offer' : 'none';
}

export interface TourStep {
    /** The data-tour value of the element the step points at; none: centred. */
    anchor?: string;
    /** Left out when its element is not on screen (a hidden menu item, a phone). */
    optional?: boolean;
    title: TranslationKey;
    text: TranslationKey;
}

const welcome: TourStep = { title: 'guide.tour.welcome.title', text: 'guide.tour.welcome.text' };
const help: TourStep = { anchor: 'help', title: 'guide.tour.help.title', text: 'guide.tour.help.text' };
const account: TourStep = { anchor: 'account', title: 'guide.tour.account.title', text: 'guide.tour.account.text' };
const nav = (id: string, name: string): TourStep => ({
    anchor: `nav-${id}`,
    optional: true,
    title: `guide.tour.${name}.title` as TranslationKey,
    text: `guide.tour.${name}.text` as TranslationKey,
});

export function tourSteps(role: Role): TourStep[] {
    switch (role) {
        case 'admin':
            return [welcome, nav('serverSetup', 'setup'), nav('services', 'components'), nav('domains', 'domains'), help, account];
        case 'reseller':
            return [welcome, nav('users', 'users'), nav('domains', 'domains'), help, account];
        case 'customer':
            return [welcome, nav('dashboard', 'dashboard'), nav('domains', 'domains'), nav('databases', 'databases'), help, account];
        default:
            return [welcome, nav('domains', 'domains'), help, { ...account, text: 'guide.tour.accountUser.text' }];
    }
}

/** The steps whose element is on screen, or that need none. */
export function visibleSteps(steps: TourStep[], present: (anchor: string) => boolean): TourStep[] {
    return steps.filter((step) => !step.optional || (step.anchor !== undefined && present(step.anchor)));
}
