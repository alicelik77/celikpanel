import { useEffect, useRef, useState } from 'react';
import { AlertTriangle, HelpCircle } from 'lucide-react';
import { Button, Dialog } from './ui';
import { useI18n } from '../i18n';
import { registerConfirmHost, type ConfirmRequest } from '../lib/confirmAction';

// The product's own confirmation (owner report, 6 Nov 2026): mounted once at
// the application root, it answers every confirmAction() with the shared
// Dialog - the question, Cancel and the action's own button. Escape and the
// backdrop cancel, Tab stays inside, and focus goes back to the control that
// asked. While the confirmed action runs the button is busy and nothing else
// can close the dialogue.
//
// Ürünün kendi onay penceresi: uygulama kökünde bir kez bağlanır ve her
// confirmAction() çağrısını paylaşılan Dialog ile yanıtlar. Escape ve arka plan
// iptal eder, Tab pencerenin içinde kalır, odak soran denetime döner.

const CANCEL = 'confirm-action-cancel';
const CONFIRM = 'confirm-action-confirm';

interface Open {
    request: ConfirmRequest;
    answer: (ok: boolean) => void;
}

export function ConfirmHost() {
    const [open, setOpen] = useState<Open | null>(null);
    const openRef = useRef(false);

    useEffect(() => registerConfirmHost((request) => new Promise<boolean>((resolve) => {
        // One question at a time: a second request while one is open is "no".
        if (openRef.current) {
            resolve(false);
            return;
        }
        openRef.current = true;
        setOpen({
            request,
            answer: (ok) => {
                openRef.current = false;
                setOpen(null);
                resolve(ok);
            },
        });
    })), []);

    return open ? <ConfirmDialog {...open} /> : null;
}

function ConfirmDialog({ request, answer }: Open) {
    const { t } = useI18n();
    const [busy, setBusy] = useState(false);
    // A second press before the busy state is drawn must not run it twice.
    const answered = useRef(false);
    const danger = request.tone === 'danger';

    // Focus the safe choice for a destructive question, the action otherwise;
    // give focus back to whatever asked when the dialogue closes.
    useEffect(() => {
        const before = document.activeElement as HTMLElement | null;
        document.getElementById(danger ? CANCEL : CONFIRM)?.focus();
        return () => before?.focus?.();
    }, [danger]);

    // Tab and Shift+Tab cycle inside the dialogue.
    useEffect(() => {
        const onKeyDown = (event: KeyboardEvent) => {
            if (event.key !== 'Tab') return;
            const panel = document.getElementById(CANCEL)?.closest('[role="dialog"]');
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

    const confirm = async () => {
        if (answered.current) return;
        answered.current = true;
        if (request.action) {
            setBusy(true);
            try {
                await request.action();
            } catch (error) {
                // The action reports its own failure on the page; the question
                // was answered, so the dialogue closes either way.
                console.error(error);
            }
        }
        answer(true);
    };

    const text = request.title ?? request.message ?? '';
    const description = request.title === undefined ? undefined : request.message;

    return (
        <Dialog
            id="confirm-action"
            stacked
            tone={danger ? 'danger' : 'default'}
            icon={danger ? AlertTriangle : HelpCircle}
            busy={busy}
            onDismiss={() => { if (!answered.current) answer(false); }}
            title={request.title === undefined
                ? <span className="block whitespace-pre-line text-base font-normal leading-6">{text}</span>
                : text}
            description={description === undefined ? undefined : <span className="whitespace-pre-line">{description}</span>}
            actions={
                <>
                    <Button id={CANCEL} type="button" disabled={busy} onClick={() => { if (!answered.current) answer(false); }}>
                        {t('common.cancel')}
                    </Button>
                    <Button id={CONFIRM} type="button" variant={danger ? 'danger' : 'primary'} loading={busy} onClick={() => void confirm()} data-confirm-action="">
                        {request.confirmLabel ?? t('common.continue')}
                    </Button>
                </>
            }
        />
    );
}
