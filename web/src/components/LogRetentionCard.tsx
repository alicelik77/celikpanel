import { useState } from 'react';
import { Button, inputClass } from './ui';
import { useRemote } from '../lib/remote';
import { apiErrorText, readApiError } from '../lib/apiError';
import { followOperation, type OperationEnd } from '../lib/confirmAction';
import { followRequestProgress } from '../lib/requestProgress';
import { useWebWords } from '../i18n/webSettings';
import type { TranslationKey } from '../i18n/en';

// How long a site's logs are kept (alpha.128, PLESK-GAP G27), on the domain's
// Logs tab. Everybody sees the limit, where it comes from and the system's
// own rotation; the administrator changes it for this subscription, its plan
// or the server. The system's logrotate rule stays the only rotation rule;
// a limit removes the site's older rotated files.
//
// Site günlüklerinin ne kadar saklandığı.

interface LogFile { name: string; bytes: number; mod_time: string; live?: boolean }
interface RetentionView {
    keep_days: number;
    source: 'subscription' | 'plan' | 'server' | 'system';
    subscription_days?: number;
    plan_id?: number;
    plan_name?: string;
    plan_days?: number;
    server_days?: number;
    may_change: boolean;
    files: LogFile[];
    system: { found: boolean; frequency?: string; rotate?: number; path?: string };
    files_unknown?: boolean;
}

function decodeRetention(raw: unknown): RetentionView {
    const body = raw as RetentionView | null;
    if (!body || typeof body.keep_days !== 'number' || typeof body.source !== 'string' || !Array.isArray(body.files) || !body.system) throw new Error('shape');
    return body;
}

const CHOICES = [0, 3, 7, 14, 30, 90, 180, 365];
type Scope = 'subscription' | 'plan' | 'server';

export function LogRetentionCard({ domainId }: { domainId: number }) {
    const { t } = useWebWords();
    const words = t as unknown as (key: TranslationKey, values?: Record<string, string | number>) => string;
    const url = `/api/v1/domains/${domainId}/logs/retention`;
    const view = useRemote(url, decodeRetention);
    const [scope, setScope] = useState<Scope>('subscription');
    const [days, setDays] = useState<number | null>(null);
    if (view.remote.state !== 'known') return null;
    const state = view.remote.value;
    const valueOf = (s: Scope) => (s === 'subscription' ? state.subscription_days : s === 'plan' ? state.plan_days : state.server_days) ?? 0;
    const chosen = days ?? valueOf(scope);
    const label = (n: number) => (n === 0 ? t('logs.keep.system') : t('logs.keep.days', { n }));
    const system = state.system.found
        ? t('logs.keep.systemRule', { frequency: state.system.frequency ?? '—', rotate: state.system.rotate ?? '—' })
        : t('logs.keep.systemUnknown');

    const save = async () => {
        await followOperation({
            progressTitle: t('logs.keep.progress'),
            action: async (report): Promise<OperationEnd> => {
                const progress = followRequestProgress(report, [
                    { id: 'check', label: t('web.step.check'), check: true },
                    { id: 'save', label: t('web.step.save') },
                    { id: 'sweep', label: t('logs.keep.sweep') },
                ], words, `logkeep.${domainId}`);
                try {
                    const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json', ...progress.headers }, body: JSON.stringify({ scope, keep_days: chosen }) });
                    progress.settle(res);
                    await view.retry();
                    setDays(null);
                    if (!res.ok) return { state: res.status === 502 ? 'unknown' : 'failed', reason: apiErrorText(await readApiError(res), words) };
                    return { state: 'done', message: t('logs.keep.saved') };
                } catch {
                    progress.settle(null);
                    return { state: 'unknown', reason: t('logs.keep.lost') };
                }
            },
        });
    };

    return (
        <div className="rounded-xl border border-border bg-surface p-4" data-log-retention="">
            <div className="flex items-center gap-2">
                <h4 className="text-sm font-semibold text-fg">{t('logs.keep.title')}</h4>
            </div>
            <p className="mt-1 text-sm text-fg">
                {state.keep_days > 0 ? t(`logs.keep.by.${state.source}` as TranslationKey, { n: state.keep_days, plan: state.plan_name ?? '' }) : t('logs.keep.bySystem')}
            </p>
            <p className="mt-0.5 text-xs text-fg-muted">{system}</p>
            {state.may_change && (
                <div className="mt-3 flex flex-col gap-2 sm:flex-row">
                    <select value={scope} onChange={(e) => { setScope(e.target.value as Scope); setDays(null); }} className={`${inputClass} field-md`} aria-label={t('logs.keep.scope')}>
                        <option value="subscription">{t('logs.keep.scope.subscription')}</option>
                        {state.plan_id ? <option value="plan">{t('logs.keep.scope.plan', { plan: state.plan_name ?? '' })}</option> : null}
                        <option value="server">{t('logs.keep.scope.server')}</option>
                    </select>
                    <select value={chosen} onChange={(e) => setDays(Number(e.target.value))} className={`${inputClass} field-md`} aria-label={t('logs.keep.title')}>
                        {(CHOICES.includes(chosen) ? CHOICES : [...CHOICES, chosen]).map((n) => <option key={n} value={n}>{label(n)}</option>)}
                    </select>
                    <Button type="button" variant="primary" disabled={chosen === valueOf(scope)} onClick={() => void save()}>{t('logs.keep.save')}</Button>
                </div>
            )}
        </div>
    );
}
