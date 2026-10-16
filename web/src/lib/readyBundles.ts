// Ready bundles (alpha.94, D-034): shortcuts over the catalogue's own
// components, shown at the head of each Components category and summarised by
// one Dashboard card. The Panel derives every field (cmd/panel/bundles.go);
// this file checks the shape, names the bundles in the page's language and
// decides which bundles a category's strip shows. It never invents a
// component list: what is installed is always what the Panel sent.
// Hazır paketler: kataloğun kendi bileşenleri üzerinde kısayollar. Her alanı
// Panel türetir; bu dosya biçimi denetler, paketleri sayfanın dilinde
// adlandırır ve bir bölümün şeridinin neyi göstereceğine karar verir.
import type { TranslationKey } from '../i18n/en';

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
// The one action each state may carry. A pair outside this table is not the
// contract, and the whole list is dropped: no strip offers an action the
// Panel did not mean.
// Her durumun taşıyabileceği tek eylem; tablo dışı bir çift sözleşme değildir.
const ACTIONS: Record<BundleStatus, readonly BundleAction[]> = {
    unknown: ['none'],
    available: ['install'],
    partial: ['continue'],
    installed_unverified: ['verify'],
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
                ? typeof bundle.profile_id !== 'string' || !MAIL_PROFILE_IDS.has(bundle.profile_id)
                : bundle.profile_id !== undefined || action === 'repair')
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
// is named by its components, never by the code.
// Bu sayfanın sözü olmayan bir kod, kodla değil bileşenleriyle adlandırılır.
export function bundleName(bundle: ReadyBundle, t: Translate, componentName: (id: string) => string): string {
    return said(t, `services.bundles.name.${bundle.name_code}`) ?? bundle.components.map(componentName).join(' + ');
}

export function bundleDescription(bundle: ReadyBundle, t: Translate, componentName: (id: string) => string): string {
    return said(t, `services.bundles.description.${bundle.name_code}`)
        ?? t('services.bundles.descriptionFallback', { components: bundle.components.map(componentName).join(', ') });
}

// The label of a bundle offered after another one is installed ("Add spam
// protection").
export function bundleAddLabel(bundle: ReadyBundle, t: Translate, componentName: (id: string) => string): string {
    return said(t, `services.bundles.addon.${bundle.name_code}`)
        ?? t('services.bundles.addGeneric', { name: bundleName(bundle, t, componentName) });
}

export const bundleInstalled = (bundle: ReadyBundle) => bundle.status === 'installed_verified' || bundle.status === 'installed_unverified';

// The host-wide refusals of the mail bundles: said once for the category, not
// once per bundle.
// Posta paketlerinin sunucu geneli retleri bölüm için bir kez söylenir.
export const HOST_WIDE_BLOCKS: ReadonlySet<string> = new Set(['dns_identity_not_ready', 'hostname_unknown']);

export interface BundleStripModel {
    /** Installed bundles, without those another installed bundle contains. */
    installed: ReadyBundle[];
    /** Started but not finished bundles that are not merely shared parts. */
    partial: ReadyBundle[];
    /** Offers: [bundle, shown as an "add …" offer]. */
    offers: { bundle: ReadyBundle; add: boolean }[];
    /** Refusals said one per bundle. */
    blocked: ReadyBundle[];
    /** A host-wide refusal shared by every bundle of the category. */
    hostBlocked: string | null;
    /** Nothing in this category is known yet. */
    unknown: boolean;
    empty: boolean;
}

// Containment is within one category: Nginx + PHP-FPM is not hidden because
// Webmail also runs them.
// Kapsama yalnız aynı bölüm içindedir.
const contains = (big: ReadyBundle, small: ReadyBundle) =>
    big.id !== small.id && big.category === small.category && small.components.every((id) => big.components.includes(id));

// What the strip of one category shows (owner decision, 2026-10-17):
// - with nothing installed, every top-level bundle is offered;
// - an installed bundle collapses to one line, and a bundle it contains is
//   not shown (Spam-Protected Mail contains Core Mail);
// - after something is installed, at most one more offer is shown: the
//   registry's add-on first, otherwise the first bundle still offered, as an
//   "add …" offer;
// - a bundle that was started on its own (not only through parts it shares
//   with an installed bundle) says how far it got and continues.
// Bir bölümün şeridi ne gösterir: hiçbir şey kurulu değilken bütün paketler
// sunulur; kurulu paket tek satıra iner ve kapsadığı paket gösterilmez; bir
// şey kurulduktan sonra en fazla bir öneri daha gösterilir.
export function bundleStripModel(all: readonly ReadyBundle[], category: string): BundleStripModel {
    const own = all.filter((bundle) => bundle.category === category);
    const model: BundleStripModel = { installed: [], partial: [], offers: [], blocked: [], hostBlocked: null, unknown: false, empty: own.length === 0 };
    if (own.length === 0) return model;
    if (own.every((bundle) => bundle.status === 'unknown')) {
        model.unknown = true;
        return model;
    }
    const top = own.filter((bundle) => bundle.parent === undefined);
    const hostCodes = new Set(top.map((bundle) => bundle.blocked_code));
    if (top.length > 0 && top.every((bundle) => bundle.status === 'blocked' && HOST_WIDE_BLOCKS.has(bundle.blocked_code ?? ''))
        && hostCodes.size === 1) {
        model.hostBlocked = top[0].blocked_code ?? null;
        return model;
    }
    const installed = own.filter(bundleInstalled);
    model.installed = installed.filter((bundle) => !installed.some((other) => contains(other, bundle)));
    const covered = (bundle: ReadyBundle) => installed.some((other) => contains(other, bundle));
    // A bundle whose present parts all belong to an installed bundle was not
    // started on its own: it is an offer to add, not a half-finished install.
    const sharedOnly = (bundle: ReadyBundle) => {
        const present = bundle.components.filter((id) => !bundle.missing_components.includes(id));
        return installed.length > 0 && present.every((id) => installed.some((other) => other.components.includes(id)));
    };
    const open = top.filter((bundle) => !bundleInstalled(bundle) && !covered(bundle));
    model.partial = open.filter((bundle) => bundle.status === 'partial' && !sharedOnly(bundle));
    if (installed.length === 0) {
        model.offers = open.filter((bundle) => bundle.status === 'available').map((bundle) => ({ bundle, add: false }));
        model.blocked = open.filter((bundle) => bundle.status === 'blocked');
        return model;
    }
    const byID = new Map(own.map((bundle) => [bundle.id, bundle]));
    const shown = new Set([...model.installed, ...model.partial].map((bundle) => bundle.id));
    const addonIDs = model.installed.flatMap((bundle) => bundle.addons);
    const addon = addonIDs.map((id) => byID.get(id)).find((bundle): bundle is ReadyBundle => (
        bundle !== undefined && !bundleInstalled(bundle) && !covered(bundle) && !shown.has(bundle.id)
    ));
    if (addon) {
        if (addon.status === 'blocked') model.blocked = [addon];
        else if (addon.status === 'available' || addon.status === 'partial') model.offers = [{ bundle: addon, add: true }];
        return model;
    }
    const next = open.find((bundle) => !shown.has(bundle.id) && (bundle.status === 'available' || bundle.status === 'partial'));
    if (next) model.offers = [{ bundle: next, add: true }];
    return model;
}

// The Dashboard card's summary: what is installed (no contained bundle twice)
// and how many bundles can be installed now.
// Panel kartının özeti: ne kurulu ve şimdi kaç paket kurulabilir.
export function bundlesSummary(all: readonly ReadyBundle[]): { installed: ReadyBundle[]; installable: number; known: boolean } {
    const known = all.some((bundle) => bundle.status !== 'unknown');
    const installed = all.filter(bundleInstalled);
    return {
        known,
        installed: installed.filter((bundle) => !installed.some((other) => contains(other, bundle))),
        installable: all.filter((bundle) => bundle.parent === undefined && (bundle.status === 'available' || bundle.status === 'partial')).length,
    };
}
