import type { ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, HelpCircle, Hourglass, Loader2, XCircle, type LucideIcon } from 'lucide-react';
import { useI18n } from '../../../i18n';

// The one guidance block (alpha.132, D-024, docs/OPERATION-GUIDANCE.md): a
// title that says what is going on, then Why · Who acts · Next step · Then,
// and at most one action button inside the block. The tone tells apart
// waiting for someone, an unmet prerequisite (blocked), a verified failure,
// an unknown result, work in progress, success and a neutral lead.
//
// Only the title is a live region, so a change is announced in one sentence
// and the rows are read when the person reaches them: polite by default, an
// alert for a verified failure, or `live="off"` for a block that is part of
// a page's own reading order.
//
// Tek yol gösterme kutusu: başlık, sonra Neden · Kim yapar · Sonraki adım ·
// Sonra ve kutunun içinde en çok bir düğme. Ton; bekleme, engel, doğrulanmış
// hata, bilinmeyen sonuç, sürüyor, tamam ve nötr arasını ayırır.

export type GuidanceTone = 'neutral' | 'progress' | 'waiting' | 'blocked' | 'failed' | 'unknown' | 'success';

const toneClass: Record<GuidanceTone, string> = {
    neutral: 'border-border bg-surface',
    progress: 'border-border bg-surface-2',
    waiting: 'border-warning-mark/50 bg-warning-mark/10',
    blocked: 'border-warning-mark/50 bg-warning-mark/20',
    failed: 'border-danger/30 bg-danger/10',
    unknown: 'border-warning-mark/50 bg-warning-mark/20',
    success: 'border-success/30 bg-success/10',
};

const toneIcon: Record<GuidanceTone, LucideIcon | null> = {
    neutral: null,
    progress: Loader2,
    waiting: Hourglass,
    blocked: AlertTriangle,
    failed: XCircle,
    unknown: HelpCircle,
    success: CheckCircle2,
};

export interface GuidanceProps {
    tone: GuidanceTone;
    /** What is going on, in one sentence. */
    title: ReactNode;
    /** Neden / Why: the current reason. */
    why?: ReactNode;
    /** Kim yapar / Who acts (whoWord() for "you", "the administrator", "nobody"). */
    who?: ReactNode;
    /** Sonraki adım / Next step: the concrete next action. */
    next?: ReactNode;
    /** Sonra / Then: how the work resumes. */
    then?: ReactNode;
    /** The one action button (or link) of the block. */
    action?: ReactNode;
    /** Anything the block still needs to show below its rows. */
    children?: ReactNode;
    /** The title's element: a page section's lead is a heading. */
    titleAs?: 'p' | 'h2' | 'h3';
    /** The title as a live region; by default an alert for a failure, otherwise polite. */
    live?: 'status' | 'alert' | 'off';
    className?: string;
    /** data-* attributes for the block (tests, lab checks). */
    [data: `data-${string}`]: string | undefined;
}

export function Guidance({ tone, title, why, who, next, then, action, children, titleAs = 'p', live, className, ...data }: GuidanceProps) {
    const { t } = useI18n();
    const Icon = toneIcon[tone];
    const Title = titleAs;
    const announce = live ?? (tone === 'failed' ? 'alert' : 'status');
    const rows: Array<[string, ReactNode]> = [];
    if (why) rows.push([t('kit.guidance.why'), why]);
    if (who) rows.push([t('kit.guidance.who'), who]);
    if (next) rows.push([t('kit.guidance.next'), next]);
    if (then) rows.push([t('kit.guidance.then'), then]);
    return (
        <div className={`rounded-lg border p-4 text-sm leading-6 text-fg ${toneClass[tone]} ${className ?? ''}`} data-guidance={tone} {...data}>
            <div role={announce === 'off' ? undefined : announce} className="flex items-start gap-2" data-guidance-live={announce}>
                {Icon && <Icon className={`mt-1 h-4 w-4 shrink-0 ${tone === 'progress' ? 'animate-spin' : ''}`} aria-hidden="true" />}
                <Title className={`min-w-0 break-words font-semibold ${titleAs === 'h2' ? 'text-base' : ''}`}>{title}</Title>
            </div>
            {rows.length > 0 && (
                <dl className={`mt-2 grid gap-x-4 gap-y-1 sm:grid-cols-[minmax(0,9rem)_minmax(0,1fr)] ${Icon ? 'sm:pl-6' : ''}`}>
                    {rows.map(([label, value]) => (
                        <div key={label} className="contents">
                            <dt className="text-fg-muted">{label}</dt>
                            <dd className="min-w-0 break-words max-sm:mb-1">{value}</dd>
                        </div>
                    ))}
                </dl>
            )}
            {(action || children) && (
                <div className={`mt-3 space-y-2 ${Icon ? 'sm:pl-6' : ''}`}>
                    {action && <div className="flex flex-wrap items-center gap-3">{action}</div>}
                    {children}
                </div>
            )}
        </div>
    );
}
