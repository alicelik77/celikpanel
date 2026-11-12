import { useState } from 'react';
import { AlertTriangle, CheckCircle2, ShieldAlert } from 'lucide-react';
import { useMailOutgoingWords } from '../i18n/mailOutgoing';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction } from '../lib/confirmAction';
import { lastKnown, refreshRemote, useRemote } from '../lib/remote';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { useNavigate } from '../router';
import { AdminInstallLink } from './AdminInstallLink';
import { Button, RemoteGate, inputClass } from './ui';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

// The administrator's outgoing mail limits (alpha.128, gap G1). The state
// comes first, with its fix beside it (install the limiter, the owner's file
// kept, the reason a save did not take); then the settings; then who is
// stopped for security and who sent mail. The settings are a draft
// (alpha.132): a changed field is marked, the save bar's Save asks and then
// runs on the server with its real steps, Discard puts the server's values back.
//
// Yöneticinin giden posta sınırları: önce durum ve yanındaki çözüm, sonra
// ayarlar, sonra güvenlik için durdurulanlar ve kimin gönderdiği.

export const MAIL_OUTGOING_URL = '/api/v1/mail/outgoing';
const REPORT_URL = '/api/v1/mail/outgoing/report';

interface Settings {
    enabled: boolean;
    mailbox_per_hour: number;
    domain_per_hour: number;
    subscription_per_hour: number;
    new_mailbox_per_hour: number;
    new_mailbox_hours: number;
    spike_factor: number;
    spike_minimum: number;
    refusal_pause: number;
}

interface ServerStatus {
    postfix: boolean;
    installed: boolean;
    family?: string;
    config_path?: string;
    config_state?: string;
    service_active: boolean;
    wiring?: string;
    wiring_detail?: string;
    verified?: boolean;
    detail?: string;
    error?: string;
}

interface AdminView {
    settings: Settings;
    apply: { state?: string; code?: string; detail?: string; applied_at?: string; verified_at?: string };
    server?: ServerStatus;
    server_unknown?: string;
    paused: { id: number; address: string; domain_id: number; domain?: string; paused_at: string; reason: string; detail?: string }[];
}

interface ReportRow {
    address: string;
    domain_id?: number;
    messages: number;
    recipients: number;
    limit_refusals: number;
    paused_refusals: number;
    mismatch_refusals: number;
}

function decodeView(raw: unknown): AdminView {
    const value = raw as Partial<AdminView> | null;
    if (!value || !value.settings || typeof value.settings.enabled !== 'boolean' || !Array.isArray(value.paused) || !value.apply) {
        throw new Error('outgoing');
    }
    return value as AdminView;
}

function decodeReport(raw: unknown): ReportRow[] {
    const senders = (raw as { senders?: unknown } | null)?.senders;
    if (!Array.isArray(senders)) throw new Error('report');
    return senders as ReportRow[];
}

type StateWords = { tone: 'ok' | 'warn' | 'bad'; key: TranslationKey; vars?: Record<string, string>; install?: boolean };

/** The one sentence that says where the limits stand, and who acts. */
function stateWords(view: AdminView): StateWords {
    const server = view.server;
    const code = view.apply.code ?? '';
    if (view.server_unknown) return { tone: 'warn', key: 'mout.state.unknown' };
    if (server && !server.postfix) return { tone: 'warn', key: 'mout.state.noPostfix' };
    if (!view.settings.enabled) return { tone: 'warn', key: 'mout.state.off' };
    if (code === 'OUTGOING_MAIL_NOT_INSTALLED' || (server && !server.installed)) return { tone: 'bad', key: 'mout.state.notInstalled', install: true };
    if (view.apply.state === 'applied') return { tone: 'ok', key: 'mout.state.on' };
    switch (code) {
        case 'OUTGOING_MAIL_OWNER_CONFIG':
            return { tone: 'bad', key: 'mout.state.owner', vars: { path: server?.config_path ?? '' } };
        case 'OUTGOING_MAIL_OWNER_WIRING':
            return { tone: 'bad', key: 'mout.state.ownerWiring' };
        case 'OUTGOING_MAIL_BUSY':
            return { tone: 'warn', key: 'mout.state.busy' };
        case 'OUTGOING_MAIL_NO_POSTFIX':
            return { tone: 'warn', key: 'mout.state.noPostfix' };
    }
    if (view.apply.state === 'refused' || view.apply.state === 'failed') {
        if ((view.apply.detail ?? '').includes('is not set up on this server yet')) return { tone: 'bad', key: 'mout.state.noSending' };
        return { tone: 'bad', key: 'mout.state.failed', vars: { detail: view.apply.detail ?? '' } };
    }
    return { tone: 'warn', key: 'mout.state.pending' };
}

function NumberField({ label, value, onChange, min = 0, max = 1000000, hint, changed, disabled }: {
    label: string; value: number; onChange: (n: number) => void; min?: number; max?: number; hint?: string; changed?: boolean; disabled?: boolean;
}) {
    return (
        <label className={`block rounded-r text-sm ${changed ? `${changedRowClass} -ml-2 pl-2` : ''}`}>
            <span className="mb-1 block text-xs text-fg-muted">{label}{changed && <ChangedMark />}</span>
            <input type="number" min={min} max={max} value={value} disabled={disabled}
                onChange={(event) => onChange(Math.max(min, Math.min(max, parseInt(event.target.value, 10) || 0)))}
                className={inputClass} />
            {hint && <span className="mt-1 block text-xs text-fg-subtle">{hint}</span>}
        </label>
    );
}

export function OutgoingMailSettings() {
    const { t, locale } = useMailOutgoingWords();
    const navigate = useNavigate();
    const read = useRemote(MAIL_OUTGOING_URL, decodeView);
    const say = useSaveWords();
    const draft = useDraft<Settings>(lastKnown(read.remote)?.value.settings);
    const [days, setDays] = useState<1 | 7 | 30>(1);
    const report = useRemote(`${REPORT_URL}?days=${days}`, decodeReport);
    const set = <K extends keyof Settings>(key: K, value: Settings[K]) => draft.set(key, value);
    const time = (text: string) => {
        const at = Date.parse(text.includes('T') ? text : `${text.replace(' ', 'T')}Z`);
        return Number.isFinite(at) ? new Date(at).toLocaleString(locale, { dateStyle: 'short', timeStyle: 'short' }) : text;
    };

    // Save asks (turning the limits on or off is a server change), then runs
    // with its real steps. A "no" sends nothing and keeps the changes.
    const save = () => void draft.save(async (_changes, values) => {
        const end: { outcome: 'saved' | 'refused' | 'unknown' } = { outcome: 'refused' };
        await confirmAction({
            title: t('mout.title'),
            message: values.enabled ? t('mout.confirmOn') : t('mout.confirmOff'),
            confirmLabel: t('mout.save'),
            progressTitle: t('mout.progressTitle'),
            action: async (report) => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'mailOutgoing', ['save', 'apply']), t, 'mailOutgoing',
                        (headers) => fetch(MAIL_OUTGOING_URL, {
                            method: 'POST',
                            headers: { ...headers, 'Content-Type': 'application/json' },
                            body: JSON.stringify(values),
                        }));
                    if (!res.ok) return { state: 'failed', reason: apiErrorText(await readApiError(res), t) };
                    const answer = await res.json() as { unknown?: string; result?: { applied?: boolean; code?: string; error?: string } };
                    // The settings are stored once the server answered; whether the
                    // limiter took them is said in the window and in the state above.
                    end.outcome = 'saved';
                    if (answer.unknown) return { state: 'unknown', reason: t('mout.state.unknown') };
                    if (!values.enabled) return { state: 'done', message: t('mout.savedOff') };
                    if (answer.result?.applied) return { state: 'done', message: t('mout.done') };
                    if (answer.result?.code === 'OUTGOING_MAIL_NOT_INSTALLED') {
                        return { state: 'blocked', reason: t('mout.state.notInstalled'), fix: { label: t('mout.state.install'), run: () => navigate('/components?component=postfwd&install=') } };
                    }
                    if (answer.result?.code === 'OUTGOING_MAIL_BUSY') return { state: 'blocked', reason: t('mout.state.busy') };
                    return { state: 'failed', reason: t('mout.state.failed', { detail: answer.result?.error ?? '' }) };
                } catch {
                    end.outcome = 'unknown';
                    return { state: 'unknown', reason: t('common.resultUnknown') };
                } finally {
                    refreshRemote(MAIL_OUTGOING_URL);
                }
            },
        });
        if (end.outcome === 'saved') await read.retry();
        return end.outcome;
    });
    const values = draft.values;

    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-5" data-outgoing-settings>
            <h3 className="mb-1 text-sm font-semibold text-fg">{t('mout.title')}</h3>
            <p className="mb-4 max-w-[75ch] text-sm text-fg-muted">{t('mout.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('mout.reading')} failed={t('mout.failed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value: view, stale }) => {
                    const words = stateWords(view);
                    const Icon = words.tone === 'ok' ? CheckCircle2 : words.tone === 'bad' ? ShieldAlert : AlertTriangle;
                    const surface = words.tone === 'ok' ? 'border-success/40 bg-success/10' : words.tone === 'bad' ? 'border-danger/40 bg-danger/10' : 'border-warning-mark/50 bg-warning-mark/15';
                    return (
                        <>
                            <div role="status" data-outgoing-state={words.key} className={`mb-4 flex flex-wrap items-start gap-2 rounded-lg border p-3 text-sm text-fg ${surface}`}>
                                <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                                <p className="min-w-0 max-w-[75ch] flex-1">{t(words.key, words.vars)}</p>
                                {words.install && <AdminInstallLink component="postfwd" label={t('mout.state.install')} />}
                            </div>
                            {view.paused.length > 0 && (
                                <div className="mb-4 rounded-lg border border-danger/40 p-3" data-outgoing-paused={view.paused.length}>
                                    <h4 className="mb-2 text-sm font-semibold text-fg">{t('mout.paused.title')}</h4>
                                    <ul className="space-y-2 text-sm">
                                        {view.paused.map((item) => (
                                            <li key={item.id} className="flex flex-wrap items-center justify-between gap-2">
                                                <span className="min-w-0">
                                                    <span className="font-medium text-fg">{item.address}</span>
                                                    <span className="text-fg-muted"> · {t(`mout.paused.reason.${item.reason}` as TranslationKey)} · {time(item.paused_at)}</span>
                                                    {item.detail && <span className="block text-xs text-fg-subtle">{item.detail}</span>}
                                                    {item.reason === 'manual' && <span className="block text-xs text-fg-subtle">{t('mout.paused.nextManual')}</span>}
                                                </span>
                                                {item.domain && <Button type="button" onClick={() => navigate(`/domains/${encodeURIComponent(item.domain!)}?tab=mail`)}>{t('mout.paused.open')}</Button>}
                                            </li>
                                        ))}
                                    </ul>
                                    <p className="mt-2 text-xs text-fg-muted">{t('mout.paused.next')}</p>
                                </div>
                            )}
                            {values && (
                                <>
                                    <label className={`flex min-h-[1.75rem] items-center gap-2 rounded-r text-sm text-fg ${draft.isChanged('enabled') ? `${changedRowClass} -ml-2 pl-2` : ''}`}>
                                        <input type="checkbox" className="h-4 w-4" checked={values.enabled} disabled={draft.saving} onChange={(event) => set('enabled', event.target.checked)} />
                                        {t('mout.enabled')}
                                        {draft.isChanged('enabled') && <ChangedMark switched={values.enabled} />}
                                    </label>
                                    <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
                                        <NumberField label={`${t('mout.mailbox')} (${t('mout.perHour')})`} value={values.mailbox_per_hour} changed={draft.isChanged('mailbox_per_hour')} disabled={draft.saving} onChange={(n) => set('mailbox_per_hour', n)} />
                                        <NumberField label={`${t('mout.domain')} (${t('mout.perHour')})`} value={values.domain_per_hour} changed={draft.isChanged('domain_per_hour')} disabled={draft.saving} onChange={(n) => set('domain_per_hour', n)} />
                                        <NumberField label={`${t('mout.subscription')} (${t('mout.perHour')})`} value={values.subscription_per_hour} changed={draft.isChanged('subscription_per_hour')} disabled={draft.saving} onChange={(n) => set('subscription_per_hour', n)} />
                                    </div>
                                    <p className="mt-1 text-xs text-fg-subtle">{t('mout.zeroHint')}</p>
                                    <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
                                        <NumberField label={`${t('mout.newMailbox')} (${t('mout.perHour')})`} value={values.new_mailbox_per_hour} changed={draft.isChanged('new_mailbox_per_hour')} disabled={draft.saving} onChange={(n) => set('new_mailbox_per_hour', n)} />
                                        <NumberField label={t('mout.newHours')} value={values.new_mailbox_hours} changed={draft.isChanged('new_mailbox_hours')} disabled={draft.saving} max={720} onChange={(n) => set('new_mailbox_hours', n)} />
                                    </div>
                                    <h4 className="mt-4 text-sm font-medium text-fg">{t('mout.spike')}</h4>
                                    <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-3">
                                        <NumberField label={t('mout.spikeFactor')} value={values.spike_factor} changed={draft.isChanged('spike_factor')} disabled={draft.saving} min={2} max={100} onChange={(n) => set('spike_factor', n)} />
                                        <NumberField label={t('mout.spikeMinimum')} value={values.spike_minimum} changed={draft.isChanged('spike_minimum')} disabled={draft.saving} min={10} onChange={(n) => set('spike_minimum', n)} />
                                        <NumberField label={t('mout.refusalPause')} value={values.refusal_pause} changed={draft.isChanged('refusal_pause')} disabled={draft.saving} onChange={(n) => set('refusal_pause', n)} />
                                    </div>
                                    <p className="mt-1 max-w-[75ch] text-xs text-fg-subtle">{t('mout.spikeHint')}</p>
                                    <p className="mt-1 max-w-[75ch] text-xs text-fg-subtle">{t('mout.counters')} {t('mout.ubuntu')}</p>
                                    {draft.base && (Object.keys(draft.base) as (keyof Settings)[]).filter((key) => draft.isMoved(key)).map((key) => (
                                        <ServerMovedNote
                                            key={key}
                                            value={typeof draft.base![key] === 'boolean' ? say(draft.base![key] ? 'on' : 'off') : String(draft.base![key])}
                                            onTake={() => draft.take(key)}
                                        />
                                    ))}
                                    <SaveBar
                                        className="-mx-5 px-5"
                                        count={draft.count}
                                        saving={draft.saving}
                                        blockedBy={stale ? say('stale') : undefined}
                                        unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: read.reading, onCheck: () => void read.retry() } : undefined}
                                        onSave={save}
                                        onDiscard={draft.reset}
                                    />
                                </>
                            )}
                        </>
                    );
                }}
            </RemoteGate>
            <div className="mt-6" data-outgoing-report>
                <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                    <h4 className="text-sm font-semibold text-fg">{t('mout.report.title')}</h4>
                    <select className={`${inputClass} w-auto`} value={days} onChange={(event) => setDays(Number(event.target.value) as 1 | 7 | 30)} aria-label={t('mout.report.title')}>
                        <option value={1}>{t('mout.report.days1')}</option>
                        <option value={7}>{t('mout.report.days7')}</option>
                        <option value={30}>{t('mout.report.days30')}</option>
                    </select>
                </div>
                <RemoteGate remote={report.remote} checking={t('mout.report.reading')} failed={t('mout.report.failed')} onRetry={() => void report.retry()} busy={report.reading}>
                    {({ value: rows }) => rows.length === 0 ? (
                        <p className="text-sm text-fg-muted">{t('mout.report.empty')}</p>
                    ) : (
                        <>
                            <div className="overflow-x-auto">
                                <table className="w-full text-sm">
                                    <thead>
                                        <tr className="border-b border-border text-left text-xs text-fg-muted">
                                            <th className="py-2 pr-3">{t('mout.report.mailbox')}</th>
                                            <th className="py-2 pr-3 text-right">{t('mout.report.recipients')}</th>
                                            <th className="py-2 pr-3 text-right">{t('mout.report.messages')}</th>
                                            <th className="py-2 pr-3 text-right">{t('mout.report.limit')}</th>
                                            <th className="py-2 pr-3 text-right">{t('mout.report.paused')}</th>
                                            <th className="py-2 text-right">{t('mout.report.mismatch')}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {rows.slice(0, 50).map((row) => (
                                            <tr key={row.address} className="border-b border-border last:border-0">
                                                <td className="py-2 pr-3 font-medium text-fg">{row.address}</td>
                                                <td className="py-2 pr-3 text-right tabular-nums">{row.recipients}</td>
                                                <td className="py-2 pr-3 text-right tabular-nums">{row.messages}</td>
                                                <td className={`py-2 pr-3 text-right tabular-nums ${row.limit_refusals > 0 ? 'text-warning' : ''}`}>{row.limit_refusals}</td>
                                                <td className={`py-2 pr-3 text-right tabular-nums ${row.paused_refusals > 0 ? 'text-danger' : ''}`}>{row.paused_refusals}</td>
                                                <td className="py-2 text-right tabular-nums">{row.mismatch_refusals}</td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                            <p className="mt-2 max-w-[75ch] text-xs text-fg-subtle">{t('mout.report.mismatchHint')}</p>
                        </>
                    )}
                </RemoteGate>
            </div>
        </section>
    );
}

export default OutgoingMailSettings;
