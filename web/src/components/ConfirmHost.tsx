import { Suspense, lazy, useEffect, useRef, useState } from 'react';
import { AlertTriangle, HelpCircle } from 'lucide-react';
import { Button, Dialog } from './ui';
import { useI18n } from '../i18n';
import { captureToasts, type CapturedToast } from './Toast';
import { registerConfirmHost, type ConfirmAnswer, type ConfirmRequest, type OperationEnd, type OperationStep } from '../lib/confirmAction';

// The product's own confirmation and progress window (owner, 6 Nov 2026):
// mounted once at the application root, it answers every confirmAction() with
// the shared Dialog - the question, Cancel and the action's own button. Escape
// and the backdrop cancel, Tab stays inside, and focus goes back to the control
// that asked. A confirmed action turns the dialogue into the progress window
// (OperationProgress, fetched when first needed): the page is locked until the
// action ends, the steps it reports are shown with the elapsed time, and the
// end says "Completed" or the reason, who acts and the next step.
//
// Ürünün kendi onay ve ilerleme penceresi: uygulama kökünde bir kez bağlanır.
// Onaylanan iş pencereyi ilerleme penceresine çevirir; iş bitene dek sayfa
// kilitlidir.

const OperationProgress = lazy(() => import('./OperationProgress'));
// What is lost and what stays, the heavier choice and the typed name
// (alpha.132): fetched only for a question that has one of them.
// Ne kaybolur, ne değişmez, ağır seçim ve yazılan ad: yalnız bunlardan birini
// taşıyan soru için getirilir.
const ConfirmDetails = lazy(() => import('./kit/save/ConfirmDetails'));

const CANCEL = 'confirm-action-cancel';
const CONFIRM = 'confirm-action-confirm';

type Request = ConfirmRequest & { follow?: boolean };

interface Open {
    key: number;
    request: Request;
    answer: (ok: boolean) => void;
    /** True while its action runs: only then may that action ask a follow-up. */
    running: { current: boolean };
}

let nextKey = 0;

// How an action that tells no result of its own ended, from what it said:
// an error is a failure (or an unknown result when it says so), a success is
// done (with any warning after it), a warning alone waits for someone. An
// action that said nothing closes the window; the page shows the result.
// Kendi sonucunu söylemeyen işin sonucu, söylediklerinden çıkarılır.
function endFromToasts(said: CapturedToast[], unknownText: string): OperationEnd | undefined {
    const text = (type: CapturedToast['type']) => said.filter((toast) => toast.type === type).map((toast) => toast.message).join(' ');
    const errors = text('error');
    if (errors) return errors === unknownText ? { state: 'unknown', reason: errors } : { state: 'failed', reason: errors };
    const done = [text('success'), text('info'), text('warning')].filter(Boolean).join(' ');
    if (said.some((toast) => toast.type === 'success')) return { state: 'done', message: done };
    if (said.some((toast) => toast.type === 'warning')) return { state: 'blocked', reason: text('warning') };
    return said.length > 0 ? { state: 'done', message: done } : undefined;
}

export function ConfirmHost() {
    const [open, setOpen] = useState<Open[]>([]);
    const openRef = useRef<Open[]>([]);

    useEffect(() => registerConfirmHost((request) => new Promise<boolean>((resolve) => {
        const top = openRef.current[openRef.current.length - 1];
        // One question at a time: a second request while a question waits is
        // "no". A running action may ask its own follow-up; nothing may start
        // a second operation over a running one.
        if (top && (!top.running.current || request.follow)) {
            resolve(false);
            return;
        }
        const entry: Open = {
            key: nextKey++,
            request,
            running: { current: false },
            answer: (ok) => {
                openRef.current = openRef.current.filter((item) => item !== entry);
                setOpen(openRef.current);
                resolve(ok);
            },
        };
        openRef.current = [...openRef.current, entry];
        setOpen(openRef.current);
    })), []);

    return <>{open.map((entry) => <ConfirmDialog key={entry.key} request={entry.request} answer={entry.answer} running={entry.running} />)}</>;
}

function ConfirmDialog({ request, answer, running }: Omit<Open, 'key'>) {
    const { t } = useI18n();
    const [phase, setPhase] = useState<'ask' | 'run' | 'end'>(request.follow ? 'run' : 'ask');
    const [steps, setSteps] = useState<OperationStep[] | null>(null);
    const [end, setEnd] = useState<OperationEnd | null>(null);
    const [startedAt, setStartedAt] = useState(() => Date.now());
    // A second press before the busy state is drawn must not run it twice.
    const answered = useRef(false);
    const mounted = useRef(true);
    const danger = request.tone === 'danger';
    // The choice and the typed name, kept by the structured part; the confirm
    // button is off until that part says the name is typed exactly.
    const chosen = useRef<ConfirmAnswer>({ option: false, typed: '' });
    const [ready, setReady] = useState(!request.typedName);

    // Focus the safe choice for a destructive question, the action otherwise;
    // give focus back to whatever asked when the dialogue closes.
    useEffect(() => {
        const before = document.activeElement as HTMLElement | null;
        document.getElementById(danger ? CANCEL : CONFIRM)?.focus();
        return () => {
            mounted.current = false;
            before?.focus?.();
        };
    }, [danger]);

    // Tab and Shift+Tab cycle inside the dialogue.
    useEffect(() => {
        const onKeyDown = (event: KeyboardEvent) => {
            if (event.key !== 'Tab') return;
            const panels = document.querySelectorAll('[data-confirm-host]');
            const panel = panels[panels.length - 1];
            if (!panel) return;
            const items = [...panel.querySelectorAll<HTMLElement>('button:not([disabled]),a[href],[tabindex]:not([tabindex="-1"])')];
            if (items.length === 0) {
                event.preventDefault();
                return;
            }
            const first = items[0];
            const last = items[items.length - 1];
            const at = document.activeElement;
            if (!panel.contains(at) || (event.shiftKey ? at === first : at === last)) {
                event.preventDefault();
                (event.shiftKey ? last : first).focus();
            }
        };
        document.addEventListener('keydown', onKeyDown);
        return () => document.removeEventListener('keydown', onKeyDown);
    }, []);

    const run = async () => {
        const action = request.action;
        if (!action) {
            answer(true);
            return;
        }
        running.current = true;
        setStartedAt(Date.now());
        setPhase('run');
        // What the action would say in toasts is said here instead.
        const said: CapturedToast[] = [];
        const release = captureToasts((toast) => said.push(toast));
        let result: OperationEnd | void;
        try {
            result = await action({ steps: (next) => { if (mounted.current) setSteps(next); } }, chosen.current);
        } catch (error) {
            // The action could not tell how it ended: the result is not known.
            console.error(error);
            result = { state: 'unknown', reason: t('common.resultUnknown') };
        } finally {
            release();
        }
        running.current = false;
        if (!mounted.current) return;
        result = result ?? endFromToasts(said, t('common.resultUnknown'));
        const fix = said.find((toast) => toast.action)?.action;
        if (result && result.state !== 'done' && fix && !result.fix) result = { ...result, fix };
        if (result) {
            setEnd(result);
            setPhase('end');
        } else {
            answer(true);
        }
    };

    // An operation followed without a question starts at once.
    useEffect(() => {
        if (request.follow && !answered.current) {
            answered.current = true;
            void run();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const confirm = () => {
        if (answered.current || !ready) return;
        answered.current = true;
        void run();
    };
    const cancel = () => {
        if (!answered.current) answer(false);
    };

    const title = request.title ?? request.message ?? '';
    const question = (
        <Dialog
            id="confirm-action"
            stacked
            tone={danger ? 'danger' : 'default'}
            icon={danger ? AlertTriangle : HelpCircle}
            busy={phase !== 'ask'}
            onDismiss={cancel}
            title={request.title === undefined
                ? <span className="block whitespace-pre-line text-base font-normal leading-6">{title}</span>
                : title}
            description={request.title === undefined || request.message === undefined ? undefined : <span className="whitespace-pre-line">{request.message}</span>}
            actions={
                <>
                    {!request.follow && (
                        <Button id={CANCEL} type="button" disabled={phase !== 'ask'} onClick={cancel}>
                            {request.cancelLabel ?? t('common.cancel')}
                        </Button>
                    )}
                    <Button id={CONFIRM} type="button" variant={danger ? 'danger' : 'primary'} loading={phase !== 'ask'} disabled={!ready} onClick={confirm} data-confirm-action="">
                        {request.follow ? t('operation.working') : request.confirmLabel ?? t('common.continue')}
                    </Button>
                </>
            }
        >
            {(request.lost || request.kept || request.option || request.typedName) && (
                <Suspense fallback={null}>
                    <ConfirmDetails request={request} disabled={phase !== 'ask'} onAnswer={(next, matches) => { chosen.current = next; setReady(matches); }} />
                </Suspense>
            )}
        </Dialog>
    );

    return (
        <div data-confirm-host={phase}>
            {phase === 'ask' ? question : (
                // Until the progress window's code has arrived the question
                // stays, busy: the page is locked either way.
                <Suspense fallback={question}>
                    <OperationProgress
                        title={request.progressTitle ?? title}
                        tone={request.tone}
                        steps={steps}
                        startedAt={startedAt}
                        end={phase === 'end' ? end : null}
                        onClose={() => answer(true)}
                    />
                </Suspense>
            )}
        </div>
    );
}
