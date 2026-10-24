// One line of a lifecycle card (alpha.107): a mark that says its tone (ok,
// a warning, a failure, a plain fact) and the sentence beside it.
// Bir yaşam döngüsü kartının tek satırı: tonunu söyleyen işaret ve cümle.
import type { ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, Info, XCircle } from 'lucide-react';

export type Tone = 'ok' | 'warn' | 'bad' | 'info';

const TONE_ICON = { ok: CheckCircle2, warn: AlertTriangle, bad: XCircle, info: Info } as const;
const TONE_CLASS: Record<Tone, string> = {
    ok: 'text-success',
    warn: 'text-warning',
    bad: 'text-danger',
    info: 'text-fg-subtle',
};

export function LifeLine({ tone, children, attr }: { tone: Tone; children: ReactNode; attr?: string }) {
    const Icon = TONE_ICON[tone];
    return (
        <p className="flex items-start gap-2 text-sm leading-6 text-fg" data-life-line={attr} data-tone={tone}>
            <Icon className={`mt-1 h-4 w-4 shrink-0 ${TONE_CLASS[tone]}`} aria-hidden="true" />
            <span className="min-w-0 break-words">{children}</span>
        </p>
    );
}

// A time the server sent (RFC 3339, or SQLite's "YYYY-MM-DD HH:MM:SS" in UTC),
// said as a date; what cannot be read is said as it came.
export function lifeDate(value: string | undefined, locale: string, withTime = false): string {
    if (!value) return '';
    // A month as the source gives it ("2027-05") is said as a month.
    if (/^\d{4}-\d{2}$/.test(value)) {
        const month = new Date(value + '-01T00:00:00Z');
        return Number.isNaN(month.getTime()) ? value : month.toLocaleDateString(locale, { year: 'numeric', month: 'long', timeZone: 'UTC' });
    }
    const iso = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(value) ? value.replace(' ', 'T') + 'Z' : value;
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return value;
    return withTime
        ? date.toLocaleString(locale, { year: 'numeric', month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit' })
        : date.toLocaleDateString(locale, { year: 'numeric', month: 'long', day: 'numeric' });
}
