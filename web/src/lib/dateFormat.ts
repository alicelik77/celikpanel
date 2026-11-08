// One place for every date and time the interface shows, so they all follow the
// selected interface language instead of the browser's own locale (owner
// report, 2026-11-08: the Turkish Components page showed "11/8/2026 5:20:51 AM").
// Dependency-free on purpose: no React and no i18n import, so a boot file can
// import it for almost nothing.
//
// Arayüzde gösterilen her tarih ve saat seçili arayüz dilini izler; tarayıcının
// kendi dili kazanmaz.

export type DateInput = string | number | Date | undefined | null;

// 'tr' -> 'tr-TR'; every other interface language -> 'en-US'.
export function localeTag(locale: string): string {
    return locale === 'tr' ? 'tr-TR' : 'en-US';
}

// A string with no zone ("2026-11-08 05:20:51", SQLite style) is UTC.
const zoned = /(?:Z|[+-]\d{2}(?::?\d{2})?)$/i;

function toDate(value: DateInput): Date | null {
    if (value === undefined || value === null || value === '') return null;
    let date: Date;
    if (value instanceof Date) date = value;
    else if (typeof value === 'number') date = new Date(value);
    else {
        let text = value.trim();
        if (/^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}/.test(text)) {
            text = text.replace(' ', 'T');
            if (!zoned.test(text)) text += 'Z';
        }
        date = new Date(text);
    }
    return Number.isNaN(date.getTime()) ? null : date;
}

export function formatDateTime(value: DateInput, locale: string, options: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' }): string {
    const date = toDate(value);
    return date ? date.toLocaleString(localeTag(locale), options) : '';
}

export function formatDate(value: DateInput, locale: string, options: Intl.DateTimeFormatOptions = { dateStyle: 'medium' }): string {
    const date = toDate(value);
    return date ? date.toLocaleDateString(localeTag(locale), options) : '';
}

export function formatTime(value: DateInput, locale: string, options: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }): string {
    const date = toDate(value);
    return date ? date.toLocaleTimeString(localeTag(locale), options) : '';
}
