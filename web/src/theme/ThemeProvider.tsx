import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import type { TranslationKey } from '../i18n/en';

// The user picks light, dark, or "system" (follow the OS). The choice is
// persisted; "system" also reacts live to OS changes. Default is "system",
// which resolves to light unless the OS prefers dark.
//
// Kullanıcı açık, koyu ya da "sistem"i (işletim sistemini izle) seçer. Seçim
// kalıcıdır; "sistem" ayrıca işletim sistemi değişikliklerine canlı tepki
// verir. Varsayılan "sistem"dir ve işletim sistemi koyu tercih etmedikçe
// açığa çözülür.
export type ThemePreference = 'light' | 'dark' | 'system';
type ResolvedTheme = 'light' | 'dark';

// Skins are full palettes on top of the light/dark axis: every component
// reads semantic tokens, so a skin is nothing but a CSS-variable set (see
// index.css [data-theme=…] blocks). The two axes compose — any skin works
// in light and dark.
// Skin'ler açık/koyu ekseninin üstünde tam paletlerdir: her bileşen semantik
// token okuduğundan bir skin yalnızca bir CSS-değişken setidir (index.css
// [data-theme=…] blokları). İki eksen bileşir — her skin açıkta da koyuda da
// çalışır.
// The four styles (owner, 20 Nov 2026: no style carries another company's
// product name). Each name is a text of the catalogue, in both languages.
// Dört stil (sahip, 20 Kasım 2026: hiçbir stil başka bir firmanın ürün adını
// taşımaz); adları metin kataloğundan, iki dilde.
export type SkinId = 'celik' | 'sky' | 'forest' | 'amber';

export const SKINS: { id: SkinId; nameKey: TranslationKey; swatch: string }[] = [
    { id: 'celik', nameKey: 'theme.skin.celik', swatch: 'rgb(37 99 235)' },
    { id: 'sky', nameKey: 'theme.skin.sky', swatch: 'rgb(2 132 199)' },
    { id: 'forest', nameKey: 'theme.skin.forest', swatch: 'rgb(32 165 58)' },
    { id: 'amber', nameKey: 'theme.skin.amber', swatch: 'rgb(234 88 12)' },
];

// The ids alpha.137 and earlier stored for the same three styles: a choice
// made then is read as its new id and written back once, so it is kept.
// alpha.137 ve öncesinin aynı üç stil için sakladığı kimlikler: o zamanki
// seçim yeni kimliğiyle okunur ve bir kez geri yazılır.
const EARLIER_SKIN_IDS: Readonly<Record<string, SkinId>> = { plesk: 'sky', aapanel: 'forest', cpanel: 'amber' };

const STORAGE_KEY = 'celikpanel.theme';
const SKIN_STORAGE_KEY = 'celikpanel.skin';

interface ThemeContextValue {
    preference: ThemePreference;
    resolved: ResolvedTheme;
    setPreference: (p: ThemePreference) => void;
    skin: SkinId;
    setSkin: (s: SkinId) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function systemPrefersDark(): boolean {
    return window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false;
}

function readStoredPreference(): ThemePreference {
    const stored = localStorage.getItem(STORAGE_KEY);
    return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system';
}

export function readStoredSkin(): SkinId {
    let stored: string | null = null;
    try {
        stored = localStorage.getItem(SKIN_STORAGE_KEY);
    } catch {
        return 'celik';
    }
    if (stored && Object.prototype.hasOwnProperty.call(EARLIER_SKIN_IDS, stored)) {
        const renamed = EARLIER_SKIN_IDS[stored];
        try {
            localStorage.setItem(SKIN_STORAGE_KEY, renamed);
        } catch {
            // Kept for this visit; asked again at the next one.
        }
        return renamed;
    }
    return SKINS.some((sk) => sk.id === stored) ? (stored as SkinId) : 'celik';
}

export function ThemeProvider({ children }: { children: ReactNode }) {
    const [preference, setPreferenceState] = useState<ThemePreference>(readStoredPreference);
    const [resolved, setResolved] = useState<ResolvedTheme>(() =>
        readStoredPreference() === 'dark' || (readStoredPreference() === 'system' && systemPrefersDark())
            ? 'dark'
            : 'light',
    );

    // Apply the resolved theme to <html> and keep it in sync with the
    // preference and, for "system", with live OS changes.
    // Çözülmüş temayı <html>'e uygula ve tercihle, "sistem" içinse canlı
    // işletim sistemi değişiklikleriyle senkron tut.
    useEffect(() => {
        const apply = () => {
            const dark = preference === 'dark' || (preference === 'system' && systemPrefersDark());
            setResolved(dark ? 'dark' : 'light');
            document.documentElement.classList.toggle('dark', dark);
        };
        apply();

        if (preference !== 'system') return;
        const media = window.matchMedia('(prefers-color-scheme: dark)');
        media.addEventListener('change', apply);
        return () => media.removeEventListener('change', apply);
    }, [preference]);

    const setPreference = (p: ThemePreference) => {
        localStorage.setItem(STORAGE_KEY, p);
        setPreferenceState(p);
    };

    const [skin, setSkinState] = useState<SkinId>(readStoredSkin);
    useEffect(() => {
        // 'celik' is the bare default palette — no attribute needed.
        // 'celik' çıplak varsayılan palettir — attribute gerekmez.
        if (skin === 'celik') delete document.documentElement.dataset.theme;
        else document.documentElement.dataset.theme = skin;
    }, [skin]);
    const setSkin = (sk: SkinId) => {
        localStorage.setItem(SKIN_STORAGE_KEY, sk);
        setSkinState(sk);
    };

    return (
        <ThemeContext.Provider value={{ preference, resolved, setPreference, skin, setSkin }}>
            {children}
        </ThemeContext.Provider>
    );
}

export function useTheme(): ThemeContextValue {
    const ctx = useContext(ThemeContext);
    if (!ctx) throw new Error('useTheme must be used within ThemeProvider');
    return ctx;
}
