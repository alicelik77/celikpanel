import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import type { TranslationKey } from './en';

// A small, dependency-free i18n layer. Two primary locales (tr, en) ship
// complete; adding another is just another catalog. Strings never live in
// components — they come from a key here (see docs/CONVENTIONS.md).
//
// The catalogue arrives in two parts, and the order matters (register R-060).
// The SHELL part is the copy the application cannot draw a first screen
// without: the login form, the navigation rail, the profile menu, an operation
// that is already running, and the refusal vocabulary any of those can hit. It
// is small, and the app waits for it. The SCREEN part is every other string in
// the product — 90% of the catalogue by weight — and it belongs to screens that
// are themselves loaded on demand, so it is fetched beside them instead of
// ahead of the login form. Before this split the boot payload carried the whole
// product's copy in one file: 2092 keys, of which 74 could be reached before a
// route had loaded.
//
// A screen never renders against a half-loaded catalogue: the route content
// waits for the screen part the same way it already waits for its own bundle,
// so there is no window in which a key can render as its own name.
//
// Küçük, bağımlılıksız bir i18n katmanı. İki öncelikli yerel ayar (tr, en)
// eksiksiz gelir; başka bir dil eklemek yalnızca yeni bir katalogdur.
// Metinler bileşenlerde yaşamaz — buradaki bir anahtardan gelir
// (bkz. docs/CONVENTIONS.md).
//
// Katalog iki parça hâlinde gelir ve sıra önemlidir (defter R-060). KABUK
// parçası, uygulamanın ilk ekranı çizebilmek için zorunlu olan metindir ve
// uygulama onu bekler. EKRAN parçası, ürünün geri kalan metnidir — ağırlıkça
// %90'ı — ve zaten istek üzerine yüklenen ekranlara aittir; bu yüzden giriş
// formundan önce değil, o ekranların yanında getirilir. Bir ekran yarı yüklü
// katalogla çizilmez: sayfa içeriği, kendi paketini beklediği gibi ekran
// parçasını da bekler.
export type Locale = 'tr' | 'en';
type Catalog = Partial<Record<TranslationKey, string>>;

const STORAGE_KEY = 'celikpanel.lang';
const bootstrapLabels: Record<Locale, { loading: string; reload: string }> = {
    en: { loading: 'Loading…', reload: 'Reload CelikPanel' },
    tr: { loading: 'Yükleniyor…', reload: 'CelikPanel’i yeniden yükle' },
};

function detectLocale(): Locale {
    const stored = readStoredLocale();
    if (stored === 'tr' || stored === 'en') return stored;
    return navigator.language?.toLowerCase().startsWith('tr') ? 'tr' : 'en';
}

function readStoredLocale(): string | null {
    try {
        return window.localStorage.getItem(STORAGE_KEY);
    } catch {
        return null;
    }
}

function persistLocale(locale: Locale) {
    try {
        window.localStorage.setItem(STORAGE_KEY, locale);
    } catch {
        // Language switching must still work when storage is blocked.
    }
}

interface I18nContextValue {
    locale: Locale;
    setLocale: (l: Locale) => void;
    t: (key: TranslationKey, vars?: Record<string, string | number>) => string;
    /** The screen copy has arrived; a route may render its own strings. */
    screensReady: boolean;
    /** It did not, and no amount of waiting will change that. */
    screensFailed: boolean;
}

const I18nContext = createContext<I18nContextValue | null>(null);

async function loadShell(locale: Locale): Promise<Catalog> {
    // The screen-part list is fetched beside the shell; loadScreens awaits it
    // (and reports its failure) once the shell is in.
    // Ekran parçası listesi kabukla birlikte getirilir.
    screenPartList().catch(() => undefined);
    if (locale === 'tr') {
        return (await import('./tr')).tr;
    }
    return (await import('./en')).en;
}

// The screen parts (one directory per group of screens under ./screens) and
// their loaders are listed in ./screenParts, fetched lazily beside the shell
// catalogue (v0.1.0-alpha.127: the list itself left the critical boot path).
// Every part is fetched in parallel and merged before `screens` becomes true;
// a part that fails fails the screen copy as a whole.
// Ekran parçaları ve yükleyicileri ./screenParts içindedir; kabuk kataloğuyla
// birlikte tembel getirilir. Bir parçanın hatası ekran metninin tamamını
// başarısız sayar.
const screenPartList = () => import('./screenParts');

async function loadScreens(locale: Locale): Promise<Catalog> {
    const { screenParts } = await screenPartList();
    const loaded = await Promise.all(screenParts[locale].map((load) => load()));
    return Object.assign({}, ...loaded.map((part) => Object.values(part)[0] as Catalog));
}

interface LoadedCatalog {
    locale: Locale;
    catalog: Catalog;
    screens: boolean;
}

export function I18nProvider({ children }: { children: ReactNode }) {
    const [targetLocale, setTargetLocale] = useState<Locale>(detectLocale);
    const [loaded, setLoaded] = useState<LoadedCatalog | null>(null);
    const [loadFailed, setLoadFailed] = useState(false);
    const [screensFailed, setScreensFailed] = useState(false);
    const activeLocale = loaded?.locale;
    const boot = bootstrapLabels[targetLocale];

    useEffect(() => {
        if (activeLocale === targetLocale) return;

        let current = true;
        setLoadFailed(false);

        loadShell(targetLocale)
            .then((catalog) => {
                if (!current) return;
                persistLocale(targetLocale);
                setScreensFailed(false);
                setLoaded({ locale: targetLocale, catalog, screens: false });
            })
            .catch((error) => {
                if (!current) return;
                if (activeLocale) {
                    console.error(`Could not load locale ${targetLocale}`, error);
                    setTargetLocale(activeLocale);
                    return;
                }
                if (targetLocale !== 'en') {
                    setTargetLocale('en');
                    return;
                }
                setLoadFailed(true);
            });

        return () => {
            current = false;
        };
    }, [activeLocale, targetLocale]);

    // The screen copy follows the shell, in parallel with whichever route
    // bundle the browser is already fetching.
    // Ekran metni kabuğun ardından, tarayıcının zaten getirdiği sayfa paketiyle
    // paralel gelir.
    useEffect(() => {
        if (loaded === null || loaded.screens) return;

        let current = true;
        const forLocale = loaded.locale;

        loadScreens(forLocale)
            .then((screens) => {
                if (!current) return;
                setLoaded((previous) =>
                    previous !== null && previous.locale === forLocale && !previous.screens
                        ? { locale: forLocale, catalog: { ...previous.catalog, ...screens }, screens: true }
                        : previous,
                );
            })
            .catch((error) => {
                if (!current) return;
                console.error(`Could not load screen copy for ${forLocale}`, error);
                setScreensFailed(true);
            });

        return () => {
            current = false;
        };
    }, [loaded]);

    // The document's language follows the language the page is drawn in, so a
    // Turkish page is `lang="tr"` for screen readers, hyphenation and the
    // browser's translation offer. Before the first catalogue arrives the boot
    // screen is drawn in the target language; after a switch the old language
    // stays until the new catalogue has arrived, because the text does too.
    // Belgenin dili, sayfanın çizildiği dili izler: Türkçe sayfa `lang="tr"`
    // olur. İlk katalog gelmeden açılış ekranı hedef dilde çizilir; dil
    // değişince yeni katalog gelene dek eski dil kalır, çünkü metin de kalır.
    const drawnLocale = loaded?.locale ?? targetLocale;
    useEffect(() => {
        if (typeof document === 'undefined' || !document.documentElement) return;
        document.documentElement.lang = drawnLocale;
    }, [drawnLocale]);

    const setLocale = (l: Locale) => {
        setTargetLocale(l);
    };

    const t = (key: TranslationKey, vars?: Record<string, string | number>) => {
        let text = loaded?.catalog[key] ?? key;
        if (vars) {
            for (const [name, value] of Object.entries(vars)) {
                text = text.replace(new RegExp(`\\{${name}\\}`, 'g'), String(value));
            }
        }
        return text;
    };

    if (!loaded) {
        if (loadFailed) {
            return (
                <div className="flex min-h-screen items-center justify-center bg-bg" role="alert">
                    <button type="button" className="rounded-lg border border-border bg-surface px-4 py-2 text-sm text-fg" onClick={() => window.location.reload()}>
                        {boot.reload}
                    </button>
                </div>
            );
        }

        return (
            // The one spinner in the product that is NOT the shared one, and
            // it cannot be: this renders before I18nProvider has a value, and
            // the shared Spinner reads its accessible name from useI18n, which
            // throws outside the provider. A white screen on first load is a
            // high price for one fewer copy. The boot strings come from `boot`
            // for the same reason.
            // Uruedeki paylasilan olmayan tek gosterge, ve olamaz: bu,
            // I18nProvider'in bir degeri olmadan once cizilir.
            <div className="flex min-h-screen items-center justify-center bg-bg" role="status" aria-label={boot.loading}>
                <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-primary" />
            </div>
        );
    }

    return (
        <I18nContext.Provider
            value={{
                locale: loaded.locale,
                setLocale,
                t,
                screensReady: loaded.screens,
                screensFailed,
            }}
        >
            {children}
        </I18nContext.Provider>
    );
}

export function useI18n(): I18nContextValue {
    const ctx = useContext(I18nContext);
    if (!ctx) throw new Error('useI18n must be used within I18nProvider');
    return ctx;
}
