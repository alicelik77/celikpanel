// The time left of an HSTS retirement, written as a duration ("1 h 30 min",
// "2 days 3 h"), not as a rounded relative time. Ninety minutes used to read
// "in 2 hours": the owner is told how long to keep the certificate and HTTPS
// host names available, so the figure must not be rounded up to the next unit.
// Under a day: hours and minutes. A day or more: days and hours. Under a minute:
// seconds, so the last minute of a running countdown still moves.
// Always rounds up, so the time shown is never shorter than what is left.
// HSTS kaldırma süresinden kalan zaman: 90 dakika "2 saat" değil "1 sa 30 dk".

export type DurationLocale = 'tr' | 'en';

const UNITS = {
    en: { day: ['day', 'days'], hour: ['h', 'h'], minute: ['min', 'min'], second: ['s', 's'] },
    tr: { day: ['gün', 'gün'], hour: ['sa', 'sa'], minute: ['dk', 'dk'], second: ['sn', 'sn'] },
} as const;

export function formatHSTSRemainingSeconds(totalSeconds: number, locale: DurationLocale): string {
    const units = UNITS[locale === 'tr' ? 'tr' : 'en'];
    const seconds = Math.max(1, Math.ceil(Number.isFinite(totalSeconds) ? totalSeconds : 1));
    const part = (value: number, [one, many]: readonly [string, string]) => `${value} ${value === 1 ? one : many}`;

    if (seconds < 60) return part(seconds, units.second);

    const totalMinutes = Math.ceil(seconds / 60);
    if (totalMinutes < 1_440) {
        const hours = Math.floor(totalMinutes / 60);
        const minutes = totalMinutes % 60;
        return [hours > 0 ? part(hours, units.hour) : '', minutes > 0 ? part(minutes, units.minute) : ''].filter(Boolean).join(' ');
    }

    const totalHours = Math.ceil(seconds / 3_600);
    const days = Math.floor(totalHours / 24);
    const hours = totalHours % 24;
    return [part(days, units.day), hours > 0 ? part(hours, units.hour) : ''].filter(Boolean).join(' ');
}
