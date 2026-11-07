import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, Clock, HelpCircle, Loader2, XCircle } from 'lucide-react';
import { Button, Dialog } from './ui';
import { useI18n } from '../i18n';
import type { OperationEnd, OperationStep } from '../lib/confirmAction';

// The progress window of a confirmed operation (owner's firm order, 6 Nov
// 2026): "while work runs in the background the user must never wait blankly;
// lock the page and, like the update screen, say what is being done". It has
// the update screen's step marks (SystemUpdateSteps): a check for a finished
// step, a moving bar under the current one, a quiet dot for the next ones,
// and the elapsed time of the step that runs or waits. A step that waits for
// someone else says for what. While it runs there is no way out (no button,
// no Escape, no backdrop); polling only reads. The end is "Completed", or the
// reason with who acts and the next step: a verified failure, an unmet
// prerequisite or another party, or a result that is not known (D-024).
//
// Onaylanan işin ilerleme penceresi: güncelleme ekranının adım işaretleri,
// geçen süre, çalışırken çıkış yok; sonunda "Tamamlandı" ya da neden, kimin
// ne yapacağı.

const elapsedText = (ms: number) => {
    const seconds = Math.max(0, Math.floor(ms / 1000));
    const minutes = Math.floor(seconds / 60);
    return `${minutes}:${String(seconds % 60).padStart(2, '0')}`;
};

function StepMark({ state }: { state: OperationStep['state'] }) {
    if (state === 'done') return <CheckCircle2 className="h-5 w-5 text-success" aria-hidden="true" />;
    if (state === 'failed') return <XCircle className="h-5 w-5 text-danger" aria-hidden="true" />;
    if (state === 'waiting') return <Clock className="h-4 w-4 text-warning" aria-hidden="true" />;
    if (state === 'running') return <span className="h-2.5 w-2.5 rounded-full bg-primary ring-4 ring-primary/15" aria-hidden="true" />;
    return <span className="h-2.5 w-2.5 rounded-full border-2 border-border-strong" aria-hidden="true" />;
}

const END_ICON = { done: CheckCircle2, failed: XCircle, blocked: AlertTriangle, unknown: HelpCircle } as const;
const END_TITLE = {
    done: 'operation.completed',
    failed: 'operation.failed',
    blocked: 'operation.blocked',
    unknown: 'operation.unknown',
} as const;

export default function OperationProgress({ title, tone, steps, startedAt, end: said, onClose }: {
    title: string;
    tone?: 'default' | 'danger';
    steps: OperationStep[] | null;
    startedAt: number;
    end: OperationEnd | null;
    onClose: () => void;
}) {
    const { t } = useI18n();
    // A refusal at a check that runs before anything is changed is an unmet
    // prerequisite, not a failure (D-024): nothing needs repairing.
    const end: OperationEnd | null = said && said.state === 'failed' && steps?.some((step) => step.state === 'failed' && step.prerequisite)
        ? { ...said, state: 'blocked' }
        : said;
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        if (end) return;
        const timer = window.setInterval(() => setNow(Date.now()), 1000);
        return () => window.clearInterval(timer);
    }, [end]);

    // Where the server reports no steps, one honest step: the request is with
    // the server, with the time it has taken so far.
    const shown: OperationStep[] = steps ?? [{ id: 'server', label: t('operation.working'), state: end ? (end.state === 'done' ? 'done' : 'failed') : 'running', since: startedAt }];
    const current = shown.find((step) => step.state === 'running' || step.state === 'waiting');
    const announcement = end
        ? t(END_TITLE[end.state])
        : current ? (current.note ? `${current.label}. ${current.note}` : current.label) : t('operation.working');
    const EndIcon = end ? END_ICON[end.state] : null;

    return (
        <Dialog
            id="operation-progress"
            stacked
            width="lg"
            tone={end && end.state !== 'done' ? 'danger' : tone === 'danger' && !end ? 'danger' : 'default'}
            iconSlot={
                <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${
                    !end ? 'bg-primary/10 text-primary'
                        : end.state === 'done' ? 'bg-success/10 text-success'
                            : end.state === 'failed' ? 'bg-danger/10 text-danger' : 'bg-warning-mark/20 text-warning'
                }`}>
                    {EndIcon ? <EndIcon className="h-5 w-5" /> : <Loader2 className="h-5 w-5 motion-safe:animate-spin" />}
                </span>
            }
            busy={!end}
            // Only the result can be closed; the running operation cannot.
            onDismiss={end ? onClose : undefined}
            title={title}
            description={end ? t(END_TITLE[end.state]) : t('operation.wait')}
            actions={end
                ? (
                    <>
                        {/* The fix the server named (a refusal's own action) closes the window and runs. */}
                        {end.state !== 'done' && end.fix && (
                            <Button type="button" onClick={() => { onClose(); end.fix!.run(); }} data-operation-fix="">{end.fix.label}</Button>
                        )}
                        <Button type="button" variant="primary" onClick={onClose} data-operation-close="" autoFocus>{t('operation.ok')}</Button>
                    </>
                )
                : <Button type="button" variant="primary" loading data-operation-running="">{t('operation.working')}</Button>}
        >
            <p className="sr-only" role="status" aria-live="polite" data-operation-announce="">{announcement}</p>
            <ol className="space-y-3 text-left" data-operation-steps="">
                {shown.map((step) => {
                    const active = step.state === 'running' || step.state === 'waiting';
                    return (
                        <li key={step.id} className="flex gap-3" data-operation-step={step.id} data-step-state={step.state}>
                            <span className="flex h-5 w-5 shrink-0 items-center justify-center"><StepMark state={step.state} /></span>
                            <div className="min-w-0 flex-1">
                                <p className={`flex items-baseline justify-between gap-3 text-sm ${active ? 'font-semibold text-fg' : step.state === 'done' ? 'text-fg-muted' : step.state === 'failed' ? 'text-danger' : 'text-fg-subtle'}`}>
                                    <span>{step.label}</span>
                                    {active && !end && step.since !== undefined && (
                                        <span className="shrink-0 font-mono text-xs tabular-nums text-fg-muted" data-operation-elapsed="">{elapsedText(now - step.since)}</span>
                                    )}
                                </p>
                                {active && !end && (
                                    <div className="update-step-track relative mt-2 h-1.5 overflow-hidden rounded-full bg-surface-3" aria-hidden="true">
                                        <div className={`update-step-indeterminate absolute inset-y-0 left-0 w-1/3 rounded-full ${step.state === 'waiting' ? 'bg-warning' : 'bg-primary'}`} />
                                    </div>
                                )}
                                {step.note && <p className="mt-1 text-sm text-fg-muted">{step.note}</p>}
                            </div>
                        </li>
                    );
                })}
            </ol>
            {end && end.state !== 'done' && (
                <p
                    role={end.state === 'failed' ? 'alert' : 'status'}
                    className={`mt-4 rounded-lg border px-4 py-3 text-sm leading-6 text-fg ${end.state === 'failed' ? 'border-danger/40 bg-danger/5' : 'border-warning-mark/50 bg-warning-mark/10'}`}
                    data-operation-end={end.state}
                >
                    {end.reason}
                </p>
            )}
            {end?.state === 'done' && end.message && <p className="mt-4 text-sm text-fg-muted" data-operation-end="done">{end.message}</p>}
        </Dialog>
    );
}
