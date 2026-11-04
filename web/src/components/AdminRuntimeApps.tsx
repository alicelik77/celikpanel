import { useState } from 'react';
import { Cpu, PauseCircle, PlayCircle, ShieldAlert, XCircle } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, ErrorBanner, Field, RemoteGate, inputClass } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
import { decodeRuntimeAdmin, type RuntimeAdminKind, type RuntimeSandboxLevel } from '../lib/runtimeApps';
import { RuntimeAppRisk } from './RuntimeAppRisk';

// The administrator's offering of uploaded programs (alpha.119, D-042:
// third-party code is gated at the "offer" step). Nothing is open until the
// administrator has read the risk summary, ticked that they read it and
// offered it; the request carries the summary's digest, so an offer made
// against an older summary is refused. On a server that enforces a reduced
// sandbox every missing protection is named and must be accepted one by one:
// a reduced level never passes silently. A security hold stops every program
// at once and says which ones it could not stop.
//
// Yöneticinin yüklenen programları sunması. Risk özeti okunmadan hiçbir şey
// açılmaz; eksik korumalı sunucuda her eksik tek tek kabul edilir; güvenlik
// beklemesi bütün programları durdurur.

const URL = '/api/v1/admin/runtime-apps';

export function AdminRuntimeApps() {
    const { t } = useI18n();
    const page = useRemote(URL, decodeRuntimeAdmin);
    const [failure, setFailure] = useState<ApiError | null>(null);
    return (
        <section className="space-y-3">
            <div>
                <h2 className="flex items-center gap-2 text-lg font-semibold text-fg"><Cpu className="h-5 w-5 text-primary" />{t('runtime.admin.title')}</h2>
                <p className="mt-1 text-sm text-fg-muted">{t('runtime.admin.desc')}</p>
            </div>
            <ErrorBanner error={failure} />
            <RemoteGate remote={page.remote} checking={t('runtime.checking')} failed={t('runtime.unknown')} onRetry={() => void page.retry()} busy={page.reading}>
                {(shown) => (
                    <div className="space-y-3">
                        {shown.value.kinds.filter((kind) => kind.implemented).map((kind) => (
                            <AdminKindCard key={kind.kind + ':' + kind.risk_digest} kind={kind} sandbox={shown.value.sandbox} portRange={shown.value.port_range}
                                onDone={() => void page.retry()} onFailure={setFailure} />
                        ))}
                    </div>
                )}
            </RemoteGate>
        </section>
    );
}

function AdminKindCard({ kind, sandbox, portRange, onDone, onFailure }: {
    kind: RuntimeAdminKind;
    sandbox: RuntimeSandboxLevel;
    portRange: [number, number];
    onDone: () => void;
    onFailure: (e: ApiError | null) => void;
}) {
    const { t } = useI18n();
    const [read, setRead] = useState(false);
    const [memory, setMemory] = useState(String(kind.limits.memory_mb));
    const [cpu, setCPU] = useState(String(kind.limits.cpu_percent));
    const [tasks, setTasks] = useState(String(kind.limits.tasks_max));
    const [accepted, setAccepted] = useState<string[]>([]);
    const [reason, setReason] = useState('');
    const [busy, setBusy] = useState('');

    const reduced = sandbox.known && sandbox.level === 'reduced';
    // Every gap the server has now must be ticked; none is ticked for the
    // administrator, not even one accepted before.
    const allAccepted = !reduced || sandbox.gaps.every((gap) => accepted.includes(gap));
    const needsOffer = !kind.offered || kind.offer_stale || kind.level_gap;
    const limitsOK = inRange(memory, 64, 65536) && inRange(cpu, 10, 1600) && inRange(tasks, 16, 8192);

    const post = async (action: string, body: unknown) => {
        setBusy(action);
        try {
            const res = await fetch(`${URL}/${kind.kind}/${action}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
            if (!res.ok) {
                onFailure(await readApiError(res));
                return;
            }
            onFailure(null);
            if (action === 'hold' || action === 'release') {
                const result = (await res.json()) as { stopped?: string[]; not_stopped?: string[] };
                if (result.not_stopped?.length) showToast('error', t('runtime.admin.held.notStopped', { list: result.not_stopped.join(', ') }));
                else if (action === 'hold') showToast('success', t('runtime.admin.held.stopped', { list: (result.stopped ?? []).join(', ') || '—' }));
                else showToast('success', t('runtime.admin.released'));
            } else {
                showToast('success', t('runtime.actionDone'));
            }
        } catch {
            // No answer: whether it changed is not known. The page reads again.
            onFailure({ message: t('runtime.admin.lost') });
        } finally {
            setBusy('');
            onDone();
        }
    };

    return (
        <div className="space-y-3 rounded-xl border border-border bg-surface p-4" data-runtime-kind={kind.kind}>
            <div>
                <div className="font-semibold text-fg">{t(('runtime.admin.kind.' + kind.kind) as TranslationKey)}</div>
                <div className="text-sm text-fg-muted">
                    {kind.offered && !kind.offer_stale ? t('runtime.admin.offered') : t('runtime.admin.notOffered')}
                    {' · '}{t('runtime.admin.counts', { total: kind.apps, running: kind.running })}
                    {kind.offered && kind.accepted_level === 'reduced' ? ' · ' + t('runtime.admin.acceptedReduced', { gaps: kind.accepted_gaps.join(', ') }) : ''}
                </div>
            </div>
            {kind.held && <div className="rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">{t('runtime.admin.held', { reason: kind.held_reason ?? '' })}</div>}
            {kind.offer_stale && <div className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">{t('runtime.admin.offerStale')}</div>}
            {kind.level_gap && <div className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">{t('runtime.admin.levelGap')}</div>}
            <RuntimeAppRisk risk={kind.risk} limits={kind.limits} sandbox={sandbox} />
            <p className="text-xs text-fg-subtle">{t('runtime.admin.portRange', { from: portRange[0], to: portRange[1] })}</p>
            {needsOffer && (
                <div className="space-y-3 rounded-lg border border-border bg-surface-2 p-3">
                    <div className="text-sm font-medium text-fg">{t('runtime.admin.limitsTitle')}</div>
                    <div className="grid gap-3 sm:grid-cols-3">
                        <Field label={t('runtime.admin.memory')} htmlFor={`rt-mem-${kind.kind}`}><input id={`rt-mem-${kind.kind}`} inputMode="numeric" className={inputClass} value={memory} onChange={(e) => setMemory(e.target.value)} /></Field>
                        <Field label={t('runtime.admin.cpu')} htmlFor={`rt-cpu-${kind.kind}`}><input id={`rt-cpu-${kind.kind}`} inputMode="numeric" className={inputClass} value={cpu} onChange={(e) => setCPU(e.target.value)} /></Field>
                        <Field label={t('runtime.admin.tasks')} htmlFor={`rt-tasks-${kind.kind}`}><input id={`rt-tasks-${kind.kind}`} inputMode="numeric" className={inputClass} value={tasks} onChange={(e) => setTasks(e.target.value)} /></Field>
                    </div>
                    {!limitsOK && <p className="text-xs text-warning">{t('err.RUNTIME_APP.limits')}</p>}
                    {!sandbox.known && <p className="text-sm text-warning">{t('runtime.admin.levelUnknown')}</p>}
                    {reduced && (
                        <div className="space-y-2 rounded-lg border border-warning/30 bg-warning/10 p-3" data-runtime-gaps="">
                            <p className="text-sm text-fg">{t('runtime.admin.acceptGaps')}</p>
                            {sandbox.gaps.map((gap) => (
                                <label key={gap} className="flex items-start gap-2 text-sm text-fg">
                                    <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={accepted.includes(gap)}
                                        onChange={(e) => setAccepted((list) => (e.target.checked ? [...list, gap] : list.filter((g) => g !== gap)))} />
                                    {t('runtime.admin.acceptGap', { gap: t(('svcapps.sandbox.gap.' + gap) as TranslationKey) })}
                                </label>
                            ))}
                        </div>
                    )}
                    <label className="flex items-start gap-2 text-sm text-fg">
                        <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={read} onChange={(e) => setRead(e.target.checked)} />
                        {t('runtime.admin.readRisks')}
                    </label>
                    <Button variant="primary" icon={PlayCircle} loading={busy === 'offer'} disabled={!read || !allAccepted || !limitsOK || !sandbox.known || !!busy}
                        onClick={() => void post('offer', {
                            risk_digest: kind.risk_digest, memory_mb: Number(memory), cpu_percent: Number(cpu), tasks_max: Number(tasks),
                            accepted_gaps: reduced ? sandbox.gaps.filter((gap) => accepted.includes(gap)) : [],
                        })}>
                        {t('runtime.admin.offer')}
                    </Button>
                </div>
            )}
            <div className="flex flex-wrap items-end gap-2">
                {kind.offered && (
                    <Button icon={XCircle} loading={busy === 'withdraw'} disabled={!!busy} title={t('runtime.admin.withdrawNote')} onClick={() => void post('withdraw', {})}>{t('runtime.admin.withdraw')}</Button>
                )}
                {!kind.held ? (
                    <>
                        <div className="min-w-56 flex-1">
                            <Field label={t('runtime.admin.holdReason')} htmlFor={`rt-hold-${kind.kind}`} hint={t('runtime.admin.holdNote')}>
                                <input id={`rt-hold-${kind.kind}`} className={inputClass} value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} />
                            </Field>
                        </div>
                        <Button variant="danger" icon={ShieldAlert} loading={busy === 'hold'} disabled={!!busy || reason.trim() === ''} onClick={() => void post('hold', { reason })}>{t('runtime.admin.hold')}</Button>
                    </>
                ) : (
                    <Button icon={PauseCircle} loading={busy === 'release'} disabled={!!busy} title={t('runtime.admin.releaseNote')} onClick={() => void post('release', {})}>{t('runtime.admin.release')}</Button>
                )}
            </div>
        </div>
    );
}

function inRange(text: string, min: number, max: number): boolean {
    const value = Number(text);
    return /^\d+$/.test(text.trim()) && value >= min && value <= max;
}
