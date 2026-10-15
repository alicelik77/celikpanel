import type { TranslationKey } from '../i18n/en';

// A server catalogue component's name in the interface language. The server
// sends each component with its catalogue name, which is English
// (internal/core ManagedServices); a Turkish Components page showed "Firewall
// engine" (set16 observation O2). The page names a component by its catalogue
// id; a component this page has no name for (a newer Panel) keeps the server's
// name, and without one its id.
//
// Sunucu kataloğundaki bir bileşenin arayüz dilindeki adı. Sayfa bileşeni
// katalog kimliğiyle adlandırır; adını bilmediği bileşen (daha yeni bir Panel)
// sunucunun adını, o da yoksa kimliğini korur.

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

export function componentName(component: { id: string; name?: string } | string, t: Translate): string {
    const id = typeof component === 'string' ? component : component.id;
    const fallback = (typeof component === 'string' ? '' : component.name?.trim()) || id;
    if (!/^[a-z0-9][a-z0-9-]*$/.test(id)) return fallback;
    const key = `services.component.name.${id}` as TranslationKey;
    const said = t(key);
    return said === key ? fallback : said;
}

// A catalogue component's one-line description in the interface language
// ("Intrusion Prevention" is English on a Turkish page otherwise). Same rule as
// the name: a component this page has no description for keeps the server's
// own text.
//
// Bileşenin tek satırlık açıklaması arayüz dilinde; bilinmeyen bileşen
// sunucunun metnini korur.
export function componentDescription(component: { id: string; description?: string } | string, t: Translate): string {
    const id = typeof component === 'string' ? component : component.id;
    const fallback = (typeof component === 'string' ? '' : component.description?.trim()) || '';
    if (!/^[a-z0-9][a-z0-9-]*$/.test(id)) return fallback;
    const key = `services.component.description.${id}` as TranslationKey;
    const said = t(key);
    return said === key ? fallback : said;
}
