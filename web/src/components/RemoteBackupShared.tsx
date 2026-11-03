import { useState } from 'react';
import { AlertTriangle, CheckCircle2, HelpCircle, Info, Loader2, Pencil, XCircle, type LucideIcon } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import type { ApiError } from '../lib/apiError';
import { Button, ErrorBanner, inputClass } from './ui';
import {
    GPG_OPEN_COMMAND, MAX_KEEP_DAILY, MAX_KEEP_WEEKLY, guidanceCodeKeys, guidanceTone, isRemoteGuidanceCode,
    keepCountProblem, refusalResult, refusalWho, serverTime,
    type GuidanceTone, type RemoteGuidance, type RemotePolicy,
} from '../lib/remoteBackup';

// The pieces both remote backup screens share: the guidance of an operation,
// a test or a refusal (D-024: the reason, who acts, the next action and how
// it resumes, before anything else), the schedule editor and the help text.
// Loaded only with those screens.
//
// İki uzak yedek ekranının paylaştığı parçalar: bir işlemin, testin ya da
// reddin yönlendirmesi (neden, kim, sonraki adım, nasıl sürer), zamanlama
// düzenleyicisi ve yardım metni.

type T = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** A catalogue sentence, or null when the catalogue has none for the key. */
export function sayOrNull(t: T, key: string, vars?: Record<string, string | number>): string | null {
    const text = t(key as TranslationKey, vars);
    return text === key ? null : text;
}

export function formatServerTime(value: string, locale: string): string {
    const date = serverTime(value);
    return date ? date.toLocaleString(locale) : '';
}

const toneStyle: Record<GuidanceTone, { box: string; icon: LucideIcon; mark: string }> = {
    progress: { box: 'border-primary/30 bg-primary/10', icon: Loader2, mark: 'text-primary animate-spin' },
    done: { box: 'border-success/30 bg-success/10', icon: CheckCircle2, mark: 'text-success' },
    prerequisite: { box: 'border-warning-mark/50 bg-warning-mark/20', icon: AlertTriangle, mark: 'text-warning' },
    verified_failure: { box: 'border-danger/30 bg-danger/10', icon: XCircle, mark: 'text-danger' },
    unknown: { box: 'border-warning-mark/50 bg-warning-mark/20', icon: HelpCircle, mark: 'text-warning' },
};

export type GuidanceContext = 'operation' | 'test' | 'refusal';

/**
 * One guidance, drawn in the reader's language. The code chooses the
 * sentences; the server's English is the fallback and stays under Technical
 * details. Progress says what runs; the three failure classes stay apart.
 */
export function GuidanceBox({
    guidance,
    context,
    kind,
    phase,
    nextAttemptAt,
    resumesText,
    className,
}: {
    guidance: RemoteGuidance;
    context: GuidanceContext;
    /** The operation's kind, for the done sentence. */
    kind?: string;
    phase?: string;
    nextAttemptAt?: string;
    /** The screen's own "then" sentence, for guidance the screen makes itself. */
    resumesText?: string;
    className?: string;
}) {
    const { t, locale } = useI18n();
    const g = guidance;
    const tone = guidanceTone(g.result);
    const style = toneStyle[tone];
    const Icon = style.icon;
    const keys = guidanceCodeKeys(g.code);
    const codeWhat = keys ? sayOrNull(t, keys.what) : null;
    const codeTodo = keys ? sayOrNull(t, keys.todo) : null;
    const phaseText = sayOrNull(t, `rbak.phase.${phase || 'waiting'}`) ?? phase ?? '';

    let what: string;
    let todo: string;
    let resumes: string | null;
    switch (g.result) {
        case 'waiting':
            what = t('rbak.waiting.what');
            todo = t('rbak.waiting.todo');
            resumes = t('rbak.resumes.waiting');
            break;
        case 'in_progress':
            what = t('rbak.running.what', { phase: phaseText });
            todo = t('rbak.waiting.todo');
            resumes = t('rbak.resumes.in_progress');
            break;
        case 'retrying': {
            what = t('rbak.retrying.what', { what: codeWhat ?? g.reason });
            const at = formatServerTime(nextAttemptAt ?? '', locale);
            todo = at ? `${t('rbak.retrying.todo')} ${t('rbak.retrying.at', { time: at })}` : t('rbak.retrying.todo');
            resumes = t('rbak.resumes.retrying');
            break;
        }
        case 'done':
            what = t(kind === 'restore' ? 'rbak.done.restore' : 'rbak.done.upload');
            todo = t('rbak.done.todo');
            resumes = null;
            break;
        default:
            what = codeWhat ?? (g.reason || t(`rbak.result.${g.result}` as TranslationKey));
            todo = codeTodo ?? (g.nextAction || t('rbak.next.generic'));
            resumes = resumesText ?? (g.code ? sayOrNull(t, `rbak.code.${g.code}.resumes`) : null)
                ?? t(context === 'test' ? 'rbak.resumes.test' : context === 'refusal' ? 'rbak.resumes.refusal' : 'rbak.resumes.failure');
    }
    const who = sayOrNull(t, `rbak.who.${g.whoActs}`) ?? g.whoActs;
    const detail = codeWhat && g.reason ? g.reason : '';
    const failing = tone === 'prerequisite' || tone === 'verified_failure' || tone === 'unknown';
    return (
        <div
            role={failing ? 'alert' : 'status'}
            data-remote-result={g.result}
            className={`rounded-lg border p-3 text-sm leading-relaxed text-fg ${style.box} ${className ?? ''}`}
        >
            <div className="flex items-start gap-2">
                <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${style.mark}`} aria-hidden="true" />
                <div className="min-w-0 flex-1 space-y-1">
                    <p className="max-w-[75ch] break-words">
                        <span className="mr-1.5 inline-block rounded bg-surface px-1.5 py-0.5 text-xs font-semibold">
                            {t(`rbak.result.${g.result}` as TranslationKey)}
                        </span>
                        {what}
                    </p>
                    <dl className="grid gap-x-3 gap-y-0.5 text-xs sm:grid-cols-[auto_minmax(0,1fr)]">
                        <dt className="font-medium text-fg-muted">{t('rbak.who.label')}</dt>
                        <dd className="break-words">{who}</dd>
                        <dt className="font-medium text-fg-muted">{t('rbak.next.label')}</dt>
                        <dd className="break-words">{todo}</dd>
                        {resumes && <>
                            <dt className="font-medium text-fg-muted">{t('rbak.resumes.label')}</dt>
                            <dd className="break-words">{resumes}</dd>
                        </>}
                    </dl>
                    {(detail || (failing && g.code)) && (
                        <details className="text-xs text-fg-muted">
                            <summary className="cursor-pointer">{t('problem.details')}</summary>
                            {detail && <p className="break-words">{detail}</p>}
                            {g.code && <code className="break-words">{g.code}</code>}
                        </details>
                    )}
                </div>
            </div>
        </div>
    );
}

/**
 * A refused change. A remote backup code gets its own guidance; any other
 * refusal (a right that is not granted, a missing record) is the product's
 * one error banner.
 */
export function RemoteRefusal({ problem, sentence, className }: { problem: ApiError | null; sentence: TranslationKey; className?: string }) {
    if (!problem) return null;
    if (isRemoteGuidanceCode(problem.code)) {
        const guidance: RemoteGuidance = {
            result: refusalResult(problem.reason), code: problem.code, reason: problem.message,
            whoActs: refusalWho(problem.code), nextAction: '', resumes: '',
        };
        return <GuidanceBox guidance={guidance} context="refusal" className={className} />;
    }
    return <ErrorBanner error={problem} sentence={sentence} className={className} />;
}

export interface ScheduleBody {
    destination_id: number;
    frequency: 'daily' | 'weekly';
    backup_type: 'files' | 'full';
    keep_daily: number;
    keep_weekly: number;
    enabled: boolean;
}

/** A schedule in one line: how often, what, how many copies are kept. */
export function scheduleSummary(t: T, policy: Pick<RemotePolicy, 'frequency' | 'backupType' | 'keepDaily' | 'keepWeekly'>): string {
    return t('rbak.sched.summary', {
        frequency: t(policy.frequency === 'weekly' ? 'rbak.sched.weekly' : 'rbak.sched.daily'),
        type: t(policy.backupType === 'full' ? 'rbak.sched.full' : 'rbak.sched.files'),
        daily: policy.keepDaily,
        weekly: policy.keepWeekly,
    });
}

/** The schedule form: a site's schedule or the administrator's every-site one. */
export function ScheduleEditor({
    idPrefix,
    destinations,
    initial,
    busy,
    onSave,
    onCancel,
}: {
    idPrefix: string;
    destinations: { id: number; name: string }[];
    /** An existing schedule: its destination is fixed. */
    initial?: RemotePolicy;
    busy: boolean;
    onSave: (body: ScheduleBody) => void;
    onCancel: () => void;
}) {
    const { t } = useI18n();
    const [destination, setDestination] = useState(String(initial?.destinationId ?? destinations[0]?.id ?? ''));
    const [frequency, setFrequency] = useState<'daily' | 'weekly'>(initial?.frequency ?? 'daily');
    const [backupType, setBackupType] = useState<'files' | 'full'>(initial?.backupType ?? 'files');
    const [keepDaily, setKeepDaily] = useState(String(initial?.keepDaily ?? 7));
    const [keepWeekly, setKeepWeekly] = useState(String(initial?.keepWeekly ?? 4));
    const [enabled, setEnabled] = useState(initial?.enabled ?? true);
    const daily = Number(keepDaily);
    const weekly = Number(keepWeekly);
    const problem = keepDaily.trim() === '' || keepWeekly.trim() === '' ? 'rbak.sched.keepNone' : keepCountProblem(daily, weekly);
    const destinationId = Number(destination);
    const id = (name: string) => `${idPrefix}-${name}`;
    return (
        <form
            className="space-y-3 rounded-xl border border-border bg-surface-2 p-4"
            onSubmit={(event) => {
                event.preventDefault();
                if (problem || !destinationId) return;
                onSave({ destination_id: destinationId, frequency, backup_type: backupType, keep_daily: daily, keep_weekly: weekly, enabled });
            }}
        >
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <label className="text-sm" htmlFor={id('destination')}>
                    <span className="mb-1 block text-xs text-fg-muted">{t('rbak.sched.destination')}</span>
                    <select id={id('destination')} className={inputClass} value={destination} disabled={busy || !!initial}
                        onChange={(event) => setDestination(event.target.value)}>
                        {initial && !destinations.some((d) => d.id === initial.destinationId) && (
                            <option value={initial.destinationId}>{initial.destinationName}</option>
                        )}
                        {destinations.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                    </select>
                </label>
                <label className="text-sm" htmlFor={id('frequency')}>
                    <span className="mb-1 block text-xs text-fg-muted">{t('rbak.sched.frequency')}</span>
                    <select id={id('frequency')} className={inputClass} value={frequency} disabled={busy}
                        onChange={(event) => setFrequency(event.target.value === 'weekly' ? 'weekly' : 'daily')}>
                        <option value="daily">{t('rbak.sched.daily')}</option>
                        <option value="weekly">{t('rbak.sched.weekly')}</option>
                    </select>
                </label>
                <label className="text-sm" htmlFor={id('type')}>
                    <span className="mb-1 block text-xs text-fg-muted">{t('rbak.sched.type')}</span>
                    <select id={id('type')} className={inputClass} value={backupType} disabled={busy}
                        onChange={(event) => setBackupType(event.target.value === 'full' ? 'full' : 'files')}>
                        <option value="files">{t('rbak.sched.files')}</option>
                        <option value="full">{t('rbak.sched.full')}</option>
                    </select>
                </label>
                <div className="grid grid-cols-2 gap-3">
                    <label className="text-sm" htmlFor={id('daily')}>
                        <span className="mb-1 block text-xs text-fg-muted">{t('rbak.sched.keepDaily', { max: MAX_KEEP_DAILY })}</span>
                        <input id={id('daily')} type="number" inputMode="numeric" min={0} max={MAX_KEEP_DAILY} step={1}
                            className={inputClass} value={keepDaily} disabled={busy}
                            aria-invalid={problem === 'rbak.sched.keepDailyRange' || undefined}
                            onChange={(event) => setKeepDaily(event.target.value)} />
                    </label>
                    <label className="text-sm" htmlFor={id('weekly')}>
                        <span className="mb-1 block text-xs text-fg-muted">{t('rbak.sched.keepWeekly', { max: MAX_KEEP_WEEKLY })}</span>
                        <input id={id('weekly')} type="number" inputMode="numeric" min={0} max={MAX_KEEP_WEEKLY} step={1}
                            className={inputClass} value={keepWeekly} disabled={busy}
                            aria-invalid={problem === 'rbak.sched.keepWeeklyRange' || undefined}
                            onChange={(event) => setKeepWeekly(event.target.value)} />
                    </label>
                </div>
            </div>
            <label className="flex items-start gap-3">
                <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={enabled} disabled={busy}
                    onChange={(event) => setEnabled(event.target.checked)} />
                <span>
                    <span className="block text-sm font-medium text-fg">{t('rbak.sched.enabled')}</span>
                    <span className="block text-xs text-fg-subtle">{t('rbak.sched.enabledHint')}</span>
                </span>
            </label>
            {problem && <p id={id('problem')} className="text-xs text-warning" role="note">{t(problem as TranslationKey)}</p>}
            <div className="flex flex-wrap justify-end gap-2">
                <Button type="button" onClick={onCancel} disabled={busy} className="max-sm:min-h-11">{t('rbak.cancel')}</Button>
                <Button type="submit" variant="primary" loading={busy} disabled={!!problem || !destinationId}
                    aria-describedby={problem ? id('problem') : undefined} className="max-sm:min-h-11">
                    {t('rbak.sched.save')}
                </Button>
            </div>
        </form>
    );
}

/** How remote copies work, and how to open one without CelikPanel. */
export function RemoteHelp() {
    const { t } = useI18n();
    return (
        <details className="rounded-xl border border-border bg-surface p-4 text-sm text-fg-muted" data-remote-help="">
            <summary className="flex cursor-pointer items-center gap-2 font-medium text-fg">
                <Info className="h-4 w-4 text-primary" aria-hidden="true" />
                {t('rbak.help.title')}
            </summary>
            <div className="mt-3 space-y-2">
                <p>{t('rbak.help.encrypted')}</p>
                <p>{t('rbak.help.open')}</p>
                <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg bg-surface-2 p-3 font-mono text-xs text-fg">{GPG_OPEN_COMMAND}</pre>
                <p>{t('rbak.help.openNote')}</p>
                <p>{t('rbak.help.automation')}</p>
            </div>
        </details>
    );
}

/** Why an own destination cannot be added, and who can change that. */
export function AddRefusedNote({ layer, className }: { layer: string; className?: string }) {
    const { t } = useI18n();
    const text = sayOrNull(t, `rbak.addRefused.${layer}`) ?? t('rbak.addRefused.role');
    return (
        <div className={`flex items-start gap-2 rounded-lg border border-border-strong bg-surface-2 p-3 text-sm text-fg ${className ?? ''}`} role="note" data-add-refused={layer}>
            <Info className="mt-0.5 h-4 w-4 shrink-0 text-fg-muted" aria-hidden="true" />
            <div className="min-w-0 space-y-1">
                <p className="break-words">{text}</p>
                <p className="break-words text-xs text-fg-muted">{t('rbak.addRefused.tail')}</p>
            </div>
        </div>
    );
}

/** One schedule in a list: its destination, what it does, and its controls when the caller may change it. */
export function PolicyLine({ policy, disabled, busy, onEdit, onOff, readOnly = false }: {
    policy: RemotePolicy;
    disabled: boolean;
    busy: boolean;
    onEdit: () => void;
    onOff: () => void;
    readOnly?: boolean;
}) {
    const { t } = useI18n();
    return (
        <div className="flex flex-wrap items-center justify-between gap-2" data-policy={policy.destinationId}>
            <div className="min-w-0">
                <p className="break-words text-sm font-medium text-fg">{policy.destinationName}</p>
                <p className="text-xs text-fg-muted">{scheduleSummary(t, policy)}</p>
                <div className="mt-1 flex flex-wrap gap-1.5 text-xs">
                    {!policy.enabled && <span className="rounded bg-warning/15 px-1.5 py-0.5 text-warning">{t('rbak.sched.paused')}</span>}
                    {policy.everySite && <span className="rounded bg-surface-2 px-1.5 py-0.5 text-fg-muted">{t('rbak.sched.everySite')}</span>}
                </div>
                {!policy.editable && !readOnly && <p className="mt-1 text-xs text-fg-subtle">{t('rbak.sched.heldByAdmin')}</p>}
            </div>
            {policy.editable && !readOnly && (
                <div className="flex gap-1">
                    <Button icon={Pencil} disabled={disabled} onClick={onEdit} className="max-sm:min-h-11">{t('rbak.edit')}</Button>
                    <Button variant="danger" loading={busy} disabled={disabled} onClick={onOff} className="max-sm:min-h-11">{t('rbak.sched.off')}</Button>
                </div>
            )}
        </div>
    );
}
