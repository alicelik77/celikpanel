// Ready bundles (alpha.94, D-034): shortcuts over the catalogue's own
// components, shown at the head of each Components category and summarised by
// one Dashboard card. The Panel derives every field (cmd/panel/bundles.go);
// this file checks the shape, names the bundles in the page's language and
// says what each chip of a category shows. It never invents a
// component list: what is installed is always what the Panel sent.
// Hazır paketler: kataloğun kendi bileşenleri üzerinde kısayollar. Her alanı
// Panel türetir; bu dosya biçimi denetler, paketleri sayfanın dilinde
// adlandırır ve bir bölümün her çipinin ne söylediğini belirler.
import type { TranslationKey } from '../i18n/en';
import { mailProfileName } from './mailProfileText';

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

export type BundleStatus = 'unknown' | 'available' | 'partial' | 'installed_unverified' | 'installed_verified' | 'blocked';
export type BundleAction = 'install' | 'continue' | 'verify' | 'repair' | 'none';
export type BundleRoute = 'mail_profile' | 'components';

export interface ReadyBundle {
    id: string;
    category: string;
    name_code: string;
    components: string[];
    status: BundleStatus;
    action: BundleAction;
    action_route: BundleRoute;
    profile_id?: string;
    parent?: string;
    addons: string[];
    blocked_code?: string;
    blocked_vars?: Record<string, string>;
    warning_code?: string;
    latest_attempt_status?: string;
    missing_components: string[];
}

const STATUSES: ReadonlySet<string> = new Set(['unknown', 'available', 'partial', 'installed_unverified', 'installed_verified', 'blocked']);
const MAIL_PROFILE_IDS: ReadonlySet<string> = new Set(['core-mail', 'webmail', 'protected-mail']);
// The actions each state may carry. A pair outside this table is not the
// contract, and the whole list is dropped: no strip offers an action the
// Panel did not mean. Repair belongs to the mail route (the profile install
// completes and proves a mail setup); Verify to the components route (it only
// reads the components again); "partial" only to mail, which has attempts.
// Her durumun taşıyabileceği eylemler; Onar posta yoluna, Doğrula bileşen
// yoluna aittir; "kısmen" yalnız postadadır.
const ACTIONS: Record<BundleStatus, readonly BundleAction[]> = {
    unknown: ['none'],
    available: ['install'],
    partial: ['continue'],
    installed_unverified: ['verify', 'repair', 'none'],
    installed_verified: ['repair', 'none'],
    blocked: ['none'],
};
const CODE = /^[a-z_]+$/;

const stringList = (value: unknown, known?: ReadonlySet<string>): string[] | null => {
    if (!Array.isArray(value)) return null;
    const seen = new Set<string>();
    for (const item of value) {
        if (typeof item !== 'string' || item.trim() !== item || item === '' || seen.has(item)) return null;
        if (known && !known.has(item)) return null;
        seen.add(item);
    }
    return value as string[];
};

// decodeReadyBundles returns the bundles, or null when the answer is not the
// contract. An older Panel sends no "bundles" field: that is an empty list,
// not an error, and the page then simply shows no strips.
// Eski Panel alanı göndermez: boş listedir, hata değil.
export function decodeReadyBundles(value: unknown, serviceIDs: ReadonlySet<string>): ReadyBundle[] | null {
    if (value === undefined || value === null) return [];
    if (!Array.isArray(value)) return null;
    const ids = new Set<string>();
    const bundles: ReadyBundle[] = [];
    for (const candidate of value) {
        if (!candidate || typeof candidate !== 'object') return null;
        const bundle = candidate as Record<string, unknown>;
        const components = stringList(bundle.components, serviceIDs);
        const missing = stringList(bundle.missing_components, serviceIDs);
        const addons = stringList(bundle.addons);
        const status = bundle.status;
        const action = bundle.action;
        const route = bundle.action_route;
        if (
            typeof bundle.id !== 'string' || !/^[a-z0-9_.]+$/.test(bundle.id) || ids.has(bundle.id)
            || typeof bundle.category !== 'string' || bundle.category === ''
            || typeof bundle.name_code !== 'string' || !CODE.test(bundle.name_code)
            || components === null || components.length === 0
            || missing === null || !missing.every((id) => components.includes(id))
            || addons === null
            || typeof status !== 'string' || !STATUSES.has(status)
            || typeof action !== 'string' || !ACTIONS[status as BundleStatus].includes(action as BundleAction)
            || (route !== 'mail_profile' && route !== 'components')
            || (route === 'mail_profile'
                ? typeof bundle.profile_id !== 'string' || !MAIL_PROFILE_IDS.has(bundle.profile_id) || action === 'verify'
                : bundle.profile_id !== undefined || action === 'repair' || status === 'partial')
            || (bundle.parent !== undefined && (typeof bundle.parent !== 'string' || bundle.parent === ''))
            || (bundle.blocked_code !== undefined && (typeof bundle.blocked_code !== 'string' || !CODE.test(bundle.blocked_code)))
            || (bundle.warning_code !== undefined && (typeof bundle.warning_code !== 'string' || !CODE.test(bundle.warning_code)))
            || (bundle.blocked_vars !== undefined && (
                !bundle.blocked_vars || typeof bundle.blocked_vars !== 'object' || Array.isArray(bundle.blocked_vars)
                || !Object.values(bundle.blocked_vars).every((item) => typeof item === 'string')
            ))
            || (bundle.latest_attempt_status !== undefined && typeof bundle.latest_attempt_status !== 'string')
            // An installed bundle has nothing missing; an available or
            // partial one has something to install.
            || ((status === 'installed_verified') && missing.length > 0)
            || ((status === 'available' || status === 'partial') && missing.length === 0)
        ) {
            return null;
        }
        ids.add(bundle.id);
        bundles.push(bundle as unknown as ReadyBundle);
    }
    for (const bundle of bundles) {
        if (bundle.parent !== undefined && !ids.has(bundle.parent)) return null;
        if (!bundle.addons.every((id) => ids.has(id))) return null;
    }
    return bundles;
}

const said = (t: Translate, key: string, vars?: Record<string, string | number>): string | null => {
    const text = t(key as TranslationKey, vars);
    return text === key ? null : text;
};

// A bundle's name in the page language; a code this page has no words for
// is named by its components, never by the code. A mail bundle is named as
// the product names its mail profile everywhere else (mailProfileName).
// Bu sayfanın sözü olmayan bir kod bileşenleriyle adlandırılır; posta paketi
// ürünün her yerde kullandığı profil adıyla anılır.
export function bundleName(bundle: ReadyBundle, t: Translate, componentName: (id: string) => string): string {
    const own = said(t, `services.bundles.name.${bundle.name_code}`) ?? bundle.components.map(componentName).join(' + ');
    if (bundle.action_route === 'mail_profile' && bundle.profile_id) return mailProfileName({ id: bundle.profile_id, name: own }, t);
    return own;
}

// A known attempt of a mail bundle: one that failed, or one that is running.
// Its reason is said again wherever the bundle is drawn (D-024).
// Bir posta paketinin bilinen denemesi: başarısız ya da süren.
export const bundleAttempted = (bundle: ReadyBundle) => bundle.latest_attempt_status === 'failed' || bundle.latest_attempt_status === 'in_progress';

// What an installed bundle is, in a descriptive sentence ("PostgreSQL for
// your sites' databases."), for its dialogue; the install description is an
// imperative ("Installs …") that would misread there. Null without words.
// Kurulu paketin ne olduğu, betimleyen bir cümleyle; söz yoksa null.
export function bundleAbout(bundle: ReadyBundle, t: Translate): string | null {
    return said(t, `services.bundles.about.${bundle.name_code}`);
}

export function bundleDescription(bundle: ReadyBundle, t: Translate, componentName: (id: string) => string): string {
    return said(t, `services.bundles.description.${bundle.name_code}`)
        ?? t('services.bundles.descriptionFallback', { components: bundle.components.map(componentName).join(', ') });
}

// The short name a chip carries ("PostgreSQL", not "PostgreSQL bundle"): the
// strip's label already says these are bundles. A code without a short name
// falls back to the full name.
// Çipin taşıdığı kısa ad; şeridin etiketi bunların paket olduğunu zaten söyler.
export function bundleChipName(bundle: ReadyBundle, t: Translate, componentName: (id: string) => string): string {
    return said(t, `services.bundles.chip.${bundle.name_code}`) ?? bundleName(bundle, t, componentName);
}

export const bundleInstalled = (bundle: ReadyBundle) => bundle.status === 'installed_verified' || bundle.status === 'installed_unverified';

// The host-wide refusals of the mail bundles: said once for the category, not
// once per bundle.
// Posta paketlerinin sunucu geneli retleri bölüm için bir kez söylenir.
export const HOST_WIDE_BLOCKS: ReadonlySet<string> = new Set(['dns_identity_not_ready', 'hostname_unknown']);

// Containment is within one category: Nginx + PHP-FPM is not contained in
// Webmail although Webmail also runs them.
// Kapsama yalnız aynı bölüm içindedir.
const contains = (big: ReadyBundle, small: ReadyBundle) =>
    big.id !== small.id && big.category === small.category
    && big.components.length > small.components.length
    && small.components.every((id) => big.components.includes(id));

// The verified bundle that contains this one (Webmail contains Core Mail).
// Bunu kapsayan doğrulanmış paket.
export function bundleIncludedIn(bundle: ReadyBundle, all: readonly ReadyBundle[]): ReadyBundle | undefined {
    return all.find((other) => other.status === 'installed_verified' && contains(other, bundle));
}

// What one chip says (owner decision, 2026-10-17: every category shows the
// same bundles in the same order on every server; only the state on each
// changes). The Dashboard counts with the same function, so its numbers are
// the chips the owner sees.
// - included: a verified bundle contains it (Core Mail under Webmail), and
//   it is itself installed and verified with no attempt of its own that
//   failed or runs; such an attempt is never hidden under "included";
// - a bundle whose own last attempt failed or runs is never counted or drawn
//   as verified: it reads "not verified", and its reason is said under the
//   chips (alpha.95 review);
// - available also covers a mail bundle the Panel calls partial only because
//   an installed bundle shares its parts (Spam-Protected Mail beside Webmail
//   has Postfix and Dovecot and nothing of its own yet); one that was
//   attempted on its own keeps "n of m · Continue".
// Bir çipin söylediği; Pano da aynı işlevle sayar.
export type BundleChipState = 'verified' | 'included' | 'unverified' | 'available' | 'partial' | 'blocked' | 'unknown';

export function bundleChipState(bundle: ReadyBundle, all: readonly ReadyBundle[]): BundleChipState {
    if (bundle.status === 'unknown') return 'unknown';
    const attempted = bundleAttempted(bundle);
    if (!attempted && bundle.status === 'installed_verified' && bundleIncludedIn(bundle, all)) return 'included';
    switch (bundle.status) {
    case 'installed_verified': return attempted ? 'unverified' : 'verified';
    case 'installed_unverified': return 'unverified';
    case 'available': return 'available';
    case 'blocked': return 'blocked';
    case 'partial': {
        const installed = all.filter((other) => other.category === bundle.category && bundleInstalled(other));
        const present = bundle.components.filter((id) => !bundle.missing_components.includes(id));
        const sharedOnly = installed.length > 0 && present.every((id) => installed.some((other) => other.components.includes(id)));
        return sharedOnly && !bundleAttempted(bundle) ? 'available' : 'partial';
    }
    default: return 'unknown';
    }
}

// A state the owner can install from: the chips that carry Install, Add or
// Continue.
// Sahibin kurabileceği durum: Kur, Ekle ya da Devam et taşıyan çipler.
export const bundleInstallable = (state: BundleChipState) => state === 'available' || state === 'partial';

export interface BundleChip {
    bundle: ReadyBundle;
    state: BundleChipState;
    /** For "included": the verified bundle that contains it. */
    includedIn?: ReadyBundle;
}

export interface BundleStripModel {
    /** Every bundle of the category, in the Panel's (registry) order. */
    chips: BundleChip[];
    /** Bundles whose reason is said on the line under the chips (D-024). */
    noted: ReadyBundle[];
    /** A host-wide refusal shared by every bundle that waits. */
    hostBlocked: string | null;
    /** The scan is not fresh: no chip may act on a stale state. */
    unknown: boolean;
    empty: boolean;
}

// The scan is stale when any bundle says so; then nothing in the category is
// known, and the whole strip says one muted line instead of chips.
// Tarama bayatsa bölümde hiçbir şey bilinmez; şerit tek soluk satır söyler.
const stale = (bundle: ReadyBundle) => bundle.status === 'unknown' && bundle.blocked_code === 'state_unverified';

export function bundleStripModel(all: readonly ReadyBundle[], category: string): BundleStripModel {
    const own = all.filter((bundle) => bundle.category === category);
    const model: BundleStripModel = { chips: [], noted: [], hostBlocked: null, unknown: false, empty: own.length === 0 };
    if (own.length === 0) return model;
    if (own.every((bundle) => bundle.status === 'unknown') || own.some(stale)) {
        model.unknown = true;
        return model;
    }
    model.chips = own.map((bundle) => {
        const state = bundleChipState(bundle, all);
        const includedIn = state === 'included' ? bundleIncludedIn(bundle, all) : undefined;
        return { bundle, state, ...(includedIn ? { includedIn } : {}) };
    });
    // A host-wide refusal (the DNS identity, the host name) is said once for
    // the category; what is installed stays visible beside it.
    // Sunucu geneli ret bölüm için bir kez söylenir.
    const waiting = model.chips.filter((chip) => chip.state === 'blocked' || bundleInstallable(chip.state)).map((chip) => chip.bundle);
    const hostCodes = new Set(waiting.map((bundle) => bundle.blocked_code));
    if (waiting.length > 0 && hostCodes.size === 1
        && waiting.every((bundle) => bundle.status === 'blocked' && HOST_WIDE_BLOCKS.has(bundle.blocked_code ?? ''))) {
        model.hostBlocked = waiting[0].blocked_code ?? null;
    }
    // Said again under the chips (D-024): a mail attempt that failed or runs,
    // and a typed warning on a chip that is not verified or only partly
    // installed. The "some parts are already present" note is never said
    // here: beside an offer it is a reuse remark, not a problem (alpha.95
    // review).
    // Çiplerin altında yeniden söylenen: başarısız ya da süren deneme ve
    // doğrulanmamış ya da kısmen kurulu çipin uyarısı; "parçalar zaten var"
    // notu burada söylenmez.
    model.noted = model.chips
        .filter((chip) => chip.state !== 'unknown'
            && (bundleAttempted(chip.bundle)
                || ((chip.state === 'unverified' || chip.state === 'partial')
                    && chip.bundle.warning_code !== undefined && chip.bundle.warning_code !== 'partly_present')))
        .map((chip) => chip.bundle);
    return model;
}

// The Dashboard card's numbers, counted as the chips are drawn: every bundle
// of the registry, add-ons included; "installed and verified" are the
// verified and included chips, "can be installed" those that carry Install,
// Add or Continue (a blocked bundle is not counted).
// Panel kartının sayıları, çiplerin çizildiği gibi sayılır.
export function bundlesSummary(all: readonly ReadyBundle[]): { known: boolean; verified: number; installable: number; unverified: ReadyBundle[] } {
    const known = all.some((bundle) => bundle.status !== 'unknown') && !all.some(stale);
    const states = all.map((bundle) => ({ bundle, state: bundleChipState(bundle, all) }));
    return {
        known,
        verified: states.filter(({ state }) => state === 'verified' || state === 'included').length,
        installable: states.filter(({ state }) => bundleInstallable(state)).length,
        unverified: states.filter(({ state }) => state === 'unverified').map(({ bundle }) => bundle),
    };
}
