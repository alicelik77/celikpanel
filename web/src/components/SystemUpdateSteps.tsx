import { CheckCircle2 } from 'lucide-react';
import { useI18n } from '../i18n';
import type { SystemUpdateStepID, SystemUpdateStepView, SystemUpdateStepsView } from '../lib/systemUpdateSteps';

// One bar per step of the update (owner design 2026-10-16): finished steps
// carry a check, the current one a bar (determinate when the server measured
// it, a sliding segment when it did not, time-based for the restart), the
// next ones stay quiet below. The current step's label is the only text; the
// restart adds its one sentence. Nothing here is a warning: the overdue and
// failure wordings stay where they were (D-024, D-033).
// Guncellemenin her adimi icin bir cubuk; yalniz gecerli adimin etiketi okunur.

const LABEL_KEYS = {
    download: 'panelUpdate.steps.download',
    verify: 'panelUpdate.steps.verify',
    prepare: 'panelUpdate.steps.prepare',
    restart: 'panelUpdate.steps.restart',
    confirm: 'panelUpdate.steps.confirm',
} as const satisfies Record<SystemUpdateStepID, string>;

const STATE_KEYS = {
    done: 'panelUpdate.steps.state.done',
    running: 'panelUpdate.steps.state.running',
    pending: 'panelUpdate.steps.state.pending',
    failed: 'panelUpdate.steps.state.pending',
} as const;

function StepBar({ step, label }: { step: SystemUpdateStepView; label: string }) {
    const { locale } = useI18n();
    const determinate = step.progress !== null;
    const percent = determinate ? Math.round((step.progress ?? 0) * 100) : null;
    // A measured value is printed beside its bar; the restart's time-based
    // fill is an estimate and is not given a number.
    const measured = determinate && !step.restarting;
    const figure = measured && percent !== null
        ? new Intl.NumberFormat(locale === 'tr' ? 'tr-TR' : 'en-US', { style: 'percent' }).format(percent / 100)
        : null;
    return (
        <div className="mt-2 flex items-center gap-3">
            <div
                role="progressbar"
                aria-label={label}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={measured && percent !== null ? percent : undefined}
                className="update-step-track relative h-1.5 flex-1 overflow-hidden rounded-full bg-surface-3"
                data-step-progress={determinate ? (step.restarting ? 'time' : 'measured') : 'indeterminate'}
            >
                {determinate
                    ? <div className="update-step-fill h-full rounded-full bg-primary" style={{ width: `${Math.max(2, (step.progress ?? 0) * 100)}%` }} />
                    : <div className="update-step-indeterminate absolute inset-y-0 left-0 w-1/3 rounded-full bg-primary" />}
            </div>
            {figure && <span className="w-12 shrink-0 text-right font-mono text-xs tabular-nums text-fg-muted">{figure}</span>}
        </div>
    );
}

function StepMark({ state }: { state: SystemUpdateStepView['state'] }) {
    if (state === 'done') return <CheckCircle2 className="h-5 w-5 text-success" aria-hidden="true" />;
    if (state === 'running') return <span className="h-2.5 w-2.5 rounded-full bg-primary ring-4 ring-primary/15" aria-hidden="true" />;
    return <span className="h-2.5 w-2.5 rounded-full border-2 border-border-strong" aria-hidden="true" />;
}

export function SystemUpdateSteps({ view, compact = false }: { view: SystemUpdateStepsView; compact?: boolean }) {
    const { t } = useI18n();
    const steps = compact ? view.steps.filter((step) => step.state === 'running') : view.steps;
    if (steps.length === 0) return null;
    return (
        <ol aria-label={t('panelUpdate.steps.label')} className="space-y-3 text-left" data-update-steps={compact ? 'compact' : 'full'}>
            {steps.map((step) => {
                const label = t(LABEL_KEYS[step.id]);
                return (
                    <li key={step.id} className="flex gap-3" data-update-step={step.id} data-step-state={step.state}>
                        {!compact && (
                            <span className="flex h-5 w-5 shrink-0 items-center justify-center">
                                <StepMark state={step.state} />
                            </span>
                        )}
                        <div className="min-w-0 flex-1">
                            <p className={step.state === 'running'
                                ? 'text-sm font-semibold text-fg'
                                : step.state === 'done' ? 'text-sm text-fg-muted' : 'text-sm text-fg-subtle'}
                            >
                                {label}
                                <span className="sr-only">{` (${t(STATE_KEYS[step.state])})`}</span>
                            </p>
                            {/* Past the restart window the bar and its sentence give way
                                to the overdue wording: a stilled bar near its end would
                                read as almost done (batch 17). */}
                            {step.state === 'running' && !step.still && <StepBar step={step} label={label} />}
                            {step.restarting && !step.still && (
                                <p className="mt-2 text-sm text-fg-muted" data-update-restart-note="">{t('panelUpdate.steps.restarting')}</p>
                            )}
                        </div>
                    </li>
                );
            })}
        </ol>
    );
}

// The status line a screen reader hears: the current step, or the result.
export function systemUpdateStepsAnnouncement(
    view: SystemUpdateStepsView,
    t: ReturnType<typeof useI18n>['t'],
    version: string,
): string {
    if (view.completed) return t('panelUpdate.steps.updated', { version });
    const current = view.steps.find((step) => step.state === 'running');
    if (!current) return t('panelUpdate.steps.title', { version });
    return current.restarting
        ? `${t(LABEL_KEYS[current.id])}. ${t('panelUpdate.steps.restarting')}`
        : t(LABEL_KEYS[current.id]);
}
