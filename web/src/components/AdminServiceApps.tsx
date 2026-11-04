import { useEffect, useState } from 'react';
import { Bot, PauseCircle, PlayCircle, ShieldAlert, ShieldCheck, XCircle } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, ErrorBanner, Field, RemoteGate, inputClass } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
import { decodeAdminServiceApps, newOperationID, type AdminSandboxView, type AdminServiceAppEntry, type SandboxRefreshResult } from '../lib/serviceApps';
import { ServiceAppRisk } from './ServiceAppRisk';

// The administrator's offering of AI and automation apps (alpha.118, D-042:
// third-party software is gated at the "offer" step). Installed is not
// offered: each app stays closed until the administrator has read its risk
// summary, ticked that they read it and offered it; the request carries the
// summary's digest, so an offer made against an older summary is refused. A
// security hold stops every install at once and says which ones it could not
// stop.
//
// Yöneticinin yapay zekâ ve otomasyon uygulamalarını sunması. Risk özeti
// okunmadan hiçbir uygulama açılmaz; güvenlik beklemesi bütün kurulumları
// durdurur ve durduramadıklarını söyler.

const URL = '/api/v1/admin/service-apps';

export function AdminServiceApps() {
    const { t } = useI18n();
    const apps = useRemote(URL, decodeAdminServiceApps);
    const [failure, setFailure] = useState<ApiError | null>(null);
    return (
        <section className="space-y-3">
            <div>
                <h2 className="flex items-center gap-2 text-lg font-semibold text-fg"><Bot className="h-5 w-5 text-primary" />{t('svcapps.admin.title')}</h2>
                <p className="mt-1 text-sm text-fg-muted">{t('svcapps.admin.desc')}</p>
            </div>
            <ErrorBanner error={failure} />
            <RemoteGate remote={apps.remote} checking={t('svcapps.checking')} failed={t('svcapps.unknown')} onRetry={() => void apps.retry()} busy={apps.reading}>
                {(shown) => (
                    <div className="space-y-3">
                        {shown.value.sandbox && <SandboxPanel sandbox={shown.value.sandbox} onDone={() => void apps.retry()} onFailure={setFailure} />}
                        {shown.value.apps.filter((app) => !app.deferred).map((app) => (
                            <AdminAppCard key={app.id} app={app} onDone={() => void apps.retry()} onFailure={setFailure} />
                        ))}
                        {shown.value.apps.some((app) => app.deferred) && (
                            <details className="rounded-xl border border-border bg-surface-2 p-4">
                                <summary className="cursor-pointer text-sm font-medium text-fg-muted">{t('svcapps.admin.deferredTitle')}</summary>
                                <ul className="mt-2 space-y-2 text-sm text-fg-muted">
                                    {shown.value.apps.filter((app) => app.deferred).map((app) => (
                                        <li key={app.id}>
                                            <span className="font-medium text-fg">{app.name}</span> ({app.licence}) — {t(('svcapps.deferred.' + app.deferred) as TranslationKey)}
                                        </li>
                                    ))}
                                </ul>
                            </details>
                        )}
                    </div>
                )}
            </RemoteGate>
        </section>
    );
}

function AdminAppCard({ app, onDone, onFailure }: { app: AdminServiceAppEntry; onDone: () => void; onFailure: (e: ApiError | null) => void }) {
    const { t } = useI18n();
    const [read, setRead] = useState(false);
    const [memory, setMemory] = useState(String(app.limits.memory_mb));
    const [cpu, setCPU] = useState(String(app.limits.cpu_percent));
    const [tasks, setTasks] = useState(String(app.limits.tasks_max));
    const [reason, setReason] = useState('');
    const [busy, setBusy] = useState('');

    const post = async (action: string, body: unknown) => {
        setBusy(action);
        try {
            const res = await fetch(`${URL}/${app.id}/${action}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
            if (!res.ok) {
                onFailure(await readApiError(res));
                return;
            }
            onFailure(null);
            if (action === 'hold') {
                const result = (await res.json()) as { stopped?: string[]; not_stopped?: string[] };
                if (result.not_stopped?.length) showToast('error', t('svcapps.admin.held.notStopped', { list: result.not_stopped.join(', ') }));
                else showToast('success', t('svcapps.admin.held.stopped', { list: (result.stopped ?? []).join(', ') || '—' }));
            } else {
                showToast('success', t('svcapps.actionDone'));
            }
        } catch {
            onFailure({ message: t('svcapps.reason.agent_unreachable') });
        } finally {
            setBusy('');
            onDone();
        }
    };

    const unavailable = !app.available
        ? t(('svcapps.unavailable.' + (app.reason ?? 'probe_failed')) as TranslationKey, { detail: app.detail ?? '' })
        : '';
    return (
        <div className="space-y-3 rounded-xl border border-border bg-surface p-4">
            <div className="flex flex-wrap items-start justify-between gap-2">
                <div>
                    <div className="font-semibold text-fg">{app.name} <span className="text-sm font-normal text-fg-subtle">{app.vendor}</span></div>
                    <div className="text-sm text-fg-muted">
                        {app.offered && !app.offer_stale ? t('svcapps.admin.offered') : t('svcapps.admin.notOffered')} · {t('svcapps.admin.installs', { total: app.installs, running: app.running })}
                    </div>
                </div>
            </div>
            {app.held && <div className="rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">{t('svcapps.admin.held', { reason: app.held_reason ?? '' })}</div>}
            {app.offer_stale && <div className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">{t('svcapps.admin.offerStale')}</div>}
            {unavailable && <p className="text-sm text-warning">{unavailable}</p>}
            {app.available && app.runtime_source && (
                <p className="text-xs text-fg-subtle">{t(('svcapps.admin.runtime.' + app.runtime_source) as TranslationKey)}</p>
            )}
            <ServiceAppRisk app={app} />
            {(!app.offered || app.offer_stale) && (
                <div className="space-y-3 rounded-lg border border-border bg-surface-2 p-3">
                    <div className="text-sm font-medium text-fg">{t('svcapps.admin.limits')}</div>
                    <div className="grid gap-3 sm:grid-cols-3">
                        <Field label={t('svcapps.admin.memory')} htmlFor={`svc-mem-${app.id}`}><input id={`svc-mem-${app.id}`} inputMode="numeric" className={inputClass} value={memory} onChange={(e) => setMemory(e.target.value)} /></Field>
                        <Field label={t('svcapps.admin.cpu')} htmlFor={`svc-cpu-${app.id}`}><input id={`svc-cpu-${app.id}`} inputMode="numeric" className={inputClass} value={cpu} onChange={(e) => setCPU(e.target.value)} /></Field>
                        <Field label={t('svcapps.admin.tasks')} htmlFor={`svc-tasks-${app.id}`}><input id={`svc-tasks-${app.id}`} inputMode="numeric" className={inputClass} value={tasks} onChange={(e) => setTasks(e.target.value)} /></Field>
                    </div>
                    <label className="flex items-start gap-2 text-sm text-fg">
                        <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={read} onChange={(e) => setRead(e.target.checked)} />
                        {t('svcapps.readRisks', { name: app.name })}
                    </label>
                    <Button variant="primary" icon={PlayCircle} loading={busy === 'offer'} disabled={!read || !!busy}
                        onClick={() => void post('offer', { risk_digest: app.risk_digest, memory_mb: Number(memory) || 0, cpu_percent: Number(cpu) || 0, tasks_max: Number(tasks) || 0 })}>
                        {t('svcapps.admin.offer')}
                    </Button>
                </div>
            )}
            <div className="flex flex-wrap items-end gap-2">
                {app.offered && (
                    <Button icon={XCircle} loading={busy === 'withdraw'} disabled={!!busy} title={t('svcapps.admin.withdrawNote')} onClick={() => void post('withdraw', {})}>{t('svcapps.admin.withdraw')}</Button>
                )}
                {!app.held ? (
                    <>
                        <div className="min-w-56 flex-1">
                            <Field label={t('svcapps.admin.holdReason')} htmlFor={`svc-hold-${app.id}`} hint={t('svcapps.admin.holdNote')}>
                                <input id={`svc-hold-${app.id}`} className={inputClass} value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} />
                            </Field>
                        </div>
                        <Button variant="danger" icon={ShieldAlert} loading={busy === 'hold'} disabled={!!busy || reason.trim() === ''} onClick={() => void post('hold', { reason })}>{t('svcapps.admin.hold')}</Button>
                    </>
                ) : (
                    <Button icon={PauseCircle} loading={busy === 'release'} disabled={!!busy} title={t('svcapps.admin.releaseNote')} onClick={() => void post('release', {})}>{t('svcapps.admin.release')}</Button>
                )}
            </div>
        </div>
    );
}

// The site sandbox on this server (alpha.119, docs/SITE-SANDBOX.md): the
// level and its named gaps, the mail-port block, and the apps that still run
// in an earlier release's sandbox. Nothing here changes anything until the
// administrator presses the one button; a repeated press (a lost answer)
// reuses the same request id, so it never starts a second refresh.
//
// Bu sunucudaki site koruması: düzey, eksikler, posta engeli ve eski korumada
// çalışan uygulamalar. Düğmeye basılmadan hiçbir şey değişmez.
function SandboxPanel({ sandbox, onDone, onFailure }: { sandbox: AdminSandboxView; onDone: () => void; onFailure: (e: ApiError | null) => void }) {
    const { t } = useI18n();
    const [requestID, setRequestID] = useState(newOperationID);
    const [busy, setBusy] = useState(false);
    const working = busy || sandbox.busy;
    // While a refresh runs on the server the page reads again by itself.
    useEffect(() => {
        if (!sandbox.busy) return;
        const timer = window.setTimeout(onDone, 5000);
        return () => window.clearTimeout(timer);
    }, [sandbox.busy, onDone]);

    const apply = async () => {
        setBusy(true);
        try {
            const res = await fetch(`${URL}/sandbox-refresh`, {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ request_id: requestID, expected: sandbox.needs_refresh }),
            });
            if (!res.ok) {
                onFailure(await readApiError(res));
                return;
            }
            const result = (await res.json()) as SandboxRefreshResult;
            onFailure(null);
            setRequestID(newOperationID());
            showToast(result.skipped.length ? 'error' : 'success', t('svcapps.sandbox.result', {
                started: result.started.length, rewritten: result.rewritten.length,
                skipped: result.skipped.map((s) => `${s.domain} (${t(('svcapps.sandbox.skip.' + s.reason) as TranslationKey)})`).join(', ') || '—',
            }));
        } catch {
            // The answer was lost: the same request id is kept, so pressing
            // again returns the first answer instead of starting again.
            onFailure({ message: t('svcapps.sandbox.lost') });
        } finally {
            setBusy(false);
            onDone();
        }
    };

    if (!sandbox.known) {
        return <div className="rounded-xl border border-border bg-surface-2 p-4 text-sm text-fg-muted">{t('svcapps.sandbox.unknown')}</div>;
    }
    const reduced = sandbox.level === 'reduced';
    return (
        <div className="space-y-2 rounded-xl border border-border bg-surface p-4 text-sm">
            <div className="flex items-center gap-2 font-semibold text-fg">
                {reduced ? <ShieldAlert className="h-4 w-4 text-warning" /> : <ShieldCheck className="h-4 w-4 text-success" />}
                {t(reduced ? 'svcapps.sandbox.reduced' : 'svcapps.sandbox.full', { systemd: sandbox.systemd_version ?? '?' })}
            </div>
            {reduced && (
                <ul className="list-disc space-y-1 pl-5 text-warning">
                    {sandbox.gaps.map((gap) => <li key={gap}>{t(('svcapps.sandbox.gap.' + gap) as TranslationKey)}</li>)}
                </ul>
            )}
            <p className="text-fg-muted">
                {sandbox.egress_loaded ? t('svcapps.sandbox.smtp', { count: Math.max(0, sandbox.smtp_rejects) }) : t('svcapps.sandbox.smtpIdle')}
                {' '}{t('svcapps.sandbox.smtpNotCovered')}
            </p>
            {(sandbox.egress_file === 'owner_edited' || sandbox.egress_file === 'foreign' || sandbox.egress_file === 'unknown_origin') && (
                <p className="text-warning">{t('svcapps.sandbox.egressKept')}</p>
            )}
            {sandbox.edited > 0 && <p className="text-warning">{t('svcapps.sandbox.edited', { count: sandbox.edited })}</p>}
            {working && <p className="text-fg-muted">{t('svcapps.sandbox.busy')}</p>}
            {sandbox.needs_refresh > 0 && (
                <div className="space-y-2 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2">
                    <p className="text-fg">{t('svcapps.sandbox.needs', { count: sandbox.needs_refresh, running: sandbox.running })}</p>
                    <Button variant="primary" icon={ShieldCheck} loading={working} disabled={working} onClick={() => void apply()}>
                        {t('svcapps.sandbox.apply', { count: sandbox.needs_refresh })}
                    </Button>
                </div>
            )}
        </div>
    );
}
