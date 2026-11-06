// Node.js sites in the site sandbox (v0.1.0-alpha.127, docs/SITE-SANDBOX.md
// §6), under Components → Node.js. The card reads: the server's sandbox
// level with its named gaps, how many sites run in the sandbox, the sites an
// earlier release wrote, and the ones that cannot move yet with the next
// action for each. Nothing moves until the administrator confirms the one
// action in the panel's own window; while it runs the progress window locks
// the page. A repeated press after a lost answer reuses the request id, so it
// never moves anything twice.
//
// Site korumasındaki Node.js siteleri: kart yalnız okur; taşıma tek işlemle,
// panelin kendi onay ve ilerleme penceresiyle yapılır.
import { useEffect, useState } from 'react';
import { Shield } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button } from './ui';
import { useRemote } from '../lib/remote';
import { readApiError, apiErrorText } from '../lib/apiError';
import { confirmAction, type OperationEnd } from '../lib/confirmAction';

interface Site {
    domain_id: number;
    name: string;
}

interface NodeSandboxView {
    known: boolean;
    level?: string;
    gaps: string[];
    systemd_version?: number;
    sandboxed: number;
    needs_move: Site[];
    running: number;
    edited: Site[];
    shared_account: Site[];
    busy: boolean;
}

interface Outcome {
    domain: string;
    reason: string;
}

interface MoveResult {
    moved: string[];
    rewritten: string[];
    put_back: Outcome[];
    skipped: Outcome[];
}

const URL = '/api/v1/runtimes/node/sandbox';

function decodeView(raw: unknown): NodeSandboxView {
    const v = raw as NodeSandboxView | null;
    if (!v || typeof v.known !== 'boolean' || !Array.isArray(v.needs_move) || !Array.isArray(v.gaps)) throw new Error('shape');
    return v;
}

const WHY = new Set(['not_running', 'not_listening', 'listens_elsewhere', 'port_taken', 'property_mismatch', 'unit_edited',
    'shared_account', 'app_folder_not_real', 'app_folder_not_owned', 'agent_unreachable']);

// A request identity for one press: kept after a lost answer, so a second
// press answers the first instead of starting again (D-029).
function newRequestID(): string {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return 'node-' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export function NodeSandboxCard() {
    const { t } = useI18n();
    const view = useRemote(URL, decodeView);
    const [requestID, setRequestID] = useState(newRequestID);
    // While a move runs elsewhere the card reads again by itself.
    const busy = view.remote.state === 'known' && view.remote.value.busy;
    useEffect(() => {
        if (!busy) return;
        const timer = window.setTimeout(() => void view.retry(), 5000);
        return () => window.clearTimeout(timer);
    }, [busy, view]);

    if (view.remote.state === 'loading') return null;
    if (view.remote.state === 'unknown' || !view.remote.value.known) {
        return view.remote.state === 'unknown' && view.remote.status === 403
            ? null
            : <p className="mt-3 border-t border-border/60 pt-3 text-sm text-fg-muted" data-node-sandbox="unknown">{t('nodesandbox.unknown')}</p>;
    }
    const v = view.remote.value;
    const names = (sites: Site[]) => sites.map((s) => s.name).join(', ');
    const why = (o: Outcome) => `${o.domain} (${t((WHY.has(o.reason) ? `nodesandbox.why.${o.reason}` : 'nodesandbox.why.other') as TranslationKey)})`;
    const reduced = v.level === 'reduced';

    const move = async () => {
        const count = v.needs_move.length;
        await confirmAction({
            title: t('nodesandbox.confirmTitle', { count }),
            message: t('nodesandbox.confirmBody'),
            confirmLabel: t('nodesandbox.confirm'),
            progressTitle: t('nodesandbox.progress'),
            action: async (report): Promise<OperationEnd> => {
                report.steps([{ id: 'move', label: t('nodesandbox.step'), state: 'running', since: Date.now() }]);
                try {
                    const res = await fetch(`${URL}-refresh`, {
                        method: 'POST', headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ request_id: requestID, expected: count }),
                    });
                    if (!res.ok) {
                        setRequestID(newRequestID());
                        return { state: 'blocked', reason: apiErrorText(await readApiError(res), t) };
                    }
                    const result = (await res.json()) as MoveResult;
                    setRequestID(newRequestID());
                    const done = t('nodesandbox.done', { moved: result.moved.length, rewritten: result.rewritten.length });
                    const problems = [
                        result.put_back.length ? t('nodesandbox.putBack', { sites: result.put_back.map(why).join(', ') }) : '',
                        result.skipped.length ? t('nodesandbox.skipped', { sites: result.skipped.map(why).join(', ') }) : '',
                    ].filter(Boolean);
                    report.steps([{ id: 'move', label: t('nodesandbox.step'), state: problems.length ? 'failed' : 'done' }]);
                    return problems.length ? { state: 'failed', reason: [done, ...problems].join(' ') } : { state: 'done', message: done };
                } catch {
                    // The answer was lost: the same request id is kept.
                    return { state: 'unknown', reason: t('nodesandbox.lost') };
                } finally {
                    void view.retry();
                }
            },
        });
    };

    return (
        <div className="mt-3 space-y-2 border-t border-border/60 pt-3 text-sm" data-node-sandbox={v.needs_move.length}>
            <div className="flex items-center gap-2 font-semibold text-fg">
                <Shield className={`h-4 w-4 ${reduced ? 'text-warning' : 'text-success'}`} aria-hidden="true" />
                {t('nodesandbox.title')}
            </div>
            <p className="text-fg-muted">
                {t(reduced ? 'nodesandbox.reduced' : 'nodesandbox.full', { systemd: v.systemd_version ?? '?' })}
                {reduced && ' ' + v.gaps.map((gap) => t(`svcapps.sandbox.gap.${gap}` as TranslationKey)).join(' ')}
            </p>
            {v.sandboxed > 0 && <p className="text-fg-muted">{t('nodesandbox.inside', { count: v.sandboxed })}</p>}
            {busy && <p className="text-fg-muted">{t('nodesandbox.busy')}</p>}
            {v.needs_move.length > 0 && (
                <div className="space-y-2 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2">
                    <p className="text-fg">{t('nodesandbox.needs', { count: v.needs_move.length, running: v.running })}</p>
                    <p className="text-fg-muted">{names(v.needs_move)}</p>
                    <Button variant="primary" icon={Shield} disabled={busy} onClick={() => void move()} data-node-sandbox-move="">
                        {t('nodesandbox.move', { count: v.needs_move.length })}
                    </Button>
                </div>
            )}
            {v.shared_account.length > 0 && <p className="text-warning">{t('nodesandbox.shared', { sites: names(v.shared_account) })}</p>}
            {v.edited.length > 0 && <p className="text-warning">{t('nodesandbox.edited', { sites: names(v.edited) })}</p>}
        </div>
    );
}
