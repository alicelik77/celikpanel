// The security emergency of a component or one of its versions (D-043,
// alpha.117). Since alpha.120 it lives on the component's own page (PHP,
// Node.js, MariaDB, PostgreSQL), closed near the bottom, and the Security
// page links to each; the Components list only shows a short red badge while
// an emergency is open (owner's Boston screens: the tools looked like part of
// MariaDB itself). Four separate steps,
// lightest first: withdraw, stop, move the sites, remove. Each emergency
// shows its guidance first (what is true now, who acts, the next action, how
// it goes on: D-024), then the steps it allows, then the progress of the
// running step. Steps 2-4 open a confirmation that lists every affected
// site, customer, reseller and plan, asks for the reason, the exact name and
// the password. The screen only reads while a step runs; it never starts one.
//
// Güvenlik acil durumu: geri çek, durdur, taşı, kaldır. Önce yönlendirme,
// sonra adımlar; 2-4. adımlar etkilenen herkesi listeleyen ciddi bir onay ister.
import { useEffect, useState } from 'react';
import { ChevronDown, ChevronRight, Shield, ShieldOff, Trash2, RefreshCw } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, ErrorBanner } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
import { useLocation } from '../router';
import {
    SECURITY_EMERGENCIES_URL, EMERGENCY_ANCHOR, decodeEmergencyDetail, decodeEmergencyList, openEmergenciesFor,
    type EmergencyComponent, type EmergencyDetail, type EmergencySite,
} from '../lib/securityEmergency';

export { SECURITY_EMERGENCIES_URL, decodeEmergencyDetail, decodeEmergencyList, openEmergenciesFor };
export type { EmergencyComponent, EmergencyDetail, EmergencySite };

const stepKey: Record<'stop' | 'move' | 'remove' | 'reenable', TranslationKey> = {
    stop: 'emg.step.stop', move: 'emg.step.move', remove: 'emg.step.remove', reenable: 'emg.step.reenable',
};

const effectKey: Record<string, TranslationKey> = {
    done: 'emg.effect.done', pending: 'emg.effect.pending', captured: 'emg.effect.captured',
    failed: 'emg.effect.failed', kept: 'emg.effect.kept', manual: 'emg.effect.manual',
};

async function post(url: string, body: unknown): Promise<{ ok: true } | { ok: false; error: ApiError } | { ok: 'lost' }> {
    try {
        const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
        if (res.ok) return { ok: true };
        return { ok: false, error: await readApiError(res) };
    } catch {
        return { ok: 'lost' };
    }
}

/**
 * The emergency section of one component. versions: the installed versions
 * (PHP lines or Node.js versions); empty for a whole component. installed:
 * false once the component is gone (step 4 removes it): the section then
 * appears only while an emergency is still open, so it can be re-enabled,
 * and offers no new withdrawal.
 */
export function SecurityEmergencySection({ component, name, versions, installed = true }: { component: EmergencyComponent; name: string; versions: string[]; installed?: boolean }) {
    const { t } = useI18n();
    // A link to #security-emergency (the Components badge, the Security
    // page) opens the section and brings it into view.
    // #security-emergency bağlantısı bölümü açar ve görünür kılar.
    const { hash } = useLocation();
    const linked = hash === `#${EMERGENCY_ANCHOR}`;
    const [open, setOpen] = useState(linked);
    useEffect(() => {
        if (!linked) return;
        setOpen(true);
        document.getElementById(EMERGENCY_ANCHOR)?.scrollIntoView?.({ block: 'start' });
    }, [linked]);
    // alpha.118 (owner's Boston screens): the list is read while closed too,
    // only to know whether an emergency is open: the section then looks like
    // the alarm it is; otherwise it is a quiet tool. One shared GET for every
    // section of the page; reading never starts anything.
    // Kapalıyken de okunur, yalnız açık acil durum var mı diye; okumak hiçbir şey başlatmaz.
    const list = useRemote(SECURITY_EMERGENCIES_URL, decodeEmergencyList);
    const openOnes = list.remote.state === 'known' ? openEmergenciesFor(list.remote.value, component) : [];
    // While a step runs the list is read again every few seconds: the screen
    // moves by itself and never starts anything (D-033).
    const running = openOnes.some((item) => item.operation?.state === 'running');
    useEffect(() => {
        if (!open || !running) return;
        const timer = setInterval(() => void list.retry(), 3000);
        return () => clearInterval(timer);
    }, [open, running, list]);
    const busyVersions = new Set(openOnes.map((item) => item.emergency.version));
    const free = versions.filter((version) => !busyVersions.has(version));
    // Not installed and nothing open: nothing to show (alpha.120).
    if (!installed && openOnes.length === 0) return null;
    return (
        <section id={EMERGENCY_ANCHOR} className={`relative mt-4 scroll-mt-24 rounded-lg border p-3 ${openOnes.length > 0 ? 'border-danger/40' : 'border-border'}`} data-security-emergency={component} data-emergency-open={openOnes.length}>
            <button type="button" className="flex w-full items-center gap-2 text-left text-sm font-semibold text-fg max-sm:min-h-11" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
                {open ? <ChevronDown className="h-4 w-4" aria-hidden="true" /> : <ChevronRight className="h-4 w-4" aria-hidden="true" />}
                {openOnes.length > 0 ? <ShieldOff className="h-4 w-4 text-danger" aria-hidden="true" /> : <Shield className="h-4 w-4 text-fg-muted" aria-hidden="true" />}
                <span>{t(openOnes.length > 0 ? 'emg.title' : 'emg.toolsTitle')}</span>
                {openOnes.length > 0 && <span className="ml-auto rounded bg-danger/10 px-2 py-0.5 text-xs text-danger">{openOnes.map((item) => item.emergency.name).join(', ')}</span>}
            </button>
            {!open && openOnes.length === 0 && <p className="mt-1 pl-6 text-xs text-fg-muted">{t('emg.toolsHint', { name })}</p>}
            {open && (
                <div className="mt-3 space-y-4">
                    <p className="max-w-[75ch] text-sm leading-6 text-fg-muted">{t('emg.intro', { name })}</p>
                    {list.remote.state === 'loading' && <p className="text-sm text-fg-muted">{t('emg.reading')}</p>}
                    {list.remote.state === 'unknown' && <p className="text-sm text-warning">{t('emg.readFailed')}</p>}
                    {openOnes.map((item) => <EmergencyCard key={item.emergency.id} id={item.emergency.id} onChanged={() => void list.retry()} />)}
                    {list.remote.state === 'known' && installed && (versions.length === 0 ? openOnes.length === 0 : free.length > 0) && (
                        <WithdrawForm component={component} name={name} versions={free} alternatives={versions} onDone={() => void list.retry()} />
                    )}
                </div>
            )}
        </section>
    );
}

function WithdrawForm({ component, name, versions, alternatives, onDone }: { component: EmergencyComponent; name: string; versions: string[]; alternatives: string[]; onDone: () => void }) {
    const { t } = useI18n();
    const [version, setVersion] = useState(versions[0] ?? '');
    const [reason, setReason] = useState('');
    const [recommended, setRecommended] = useState('');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<ApiError | null>(null);
    const chosen = versions.includes(version) ? version : versions[0] ?? '';
    const submit = async () => {
        setBusy(true);
        setError(null);
        const result = await post(SECURITY_EMERGENCIES_URL, { component, version: chosen, reason, recommended });
        setBusy(false);
        if (result.ok === true) {
            setReason('');
            onDone();
        } else if (result.ok === 'lost') {
            setError({ message: t('emg.confirm.lost') });
            onDone();
        } else {
            setError(result.error);
        }
    };
    const label = versions.length > 0 ? `${name} ${chosen}` : name;
    return (
        <div className="space-y-2 border-t border-border/60 pt-3" data-emergency-withdraw="">
            {versions.length > 0 && (
                <label className="block text-sm">
                    <span className="text-fg-muted">{t('emg.version')}</span>
                    <select className="mt-1 block w-full max-w-xs rounded-md border border-border bg-surface px-2 py-1.5" value={chosen} onChange={(e) => setVersion(e.target.value)} data-emergency-version="">
                        {versions.map((item) => <option key={item} value={item}>{item}</option>)}
                    </select>
                </label>
            )}
            <label className="block text-sm">
                <span className="text-fg-muted">{t('emg.reason')}</span>
                <input className="mt-1 block w-full rounded-md border border-border bg-surface px-2 py-1.5" value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} data-emergency-reason="" />
            </label>
            {versions.length > 0 && (
                <label className="block text-sm">
                    <span className="text-fg-muted">{t('emg.recommended')}</span>
                    <select className="mt-1 block w-full max-w-xs rounded-md border border-border bg-surface px-2 py-1.5" value={recommended} onChange={(e) => setRecommended(e.target.value)} data-emergency-recommended="">
                        <option value="">{t('emg.recommendedNone')}</option>
                        {alternatives.filter((item) => item !== chosen).map((item) => <option key={item} value={item}>{item}</option>)}
                    </select>
                </label>
            )}
            <p className="text-sm text-fg-muted">{t('emg.withdrawHelp')}</p>
            <ErrorBanner error={error} />
            <Button variant="danger" icon={Shield} onClick={() => void submit()} loading={busy} disabled={busy || reason.trim() === ''} className="max-sm:min-h-11" data-emergency-withdraw-submit="">
                {t('emg.withdraw', { name: label })}
            </Button>
        </div>
    );
}

function EmergencyCard({ id, onChanged }: { id: string; onChanged: () => void }) {
    const { t } = useI18n();
    const read = useRemote(`${SECURITY_EMERGENCIES_URL}/${encodeURIComponent(id)}`, decodeEmergencyDetail);
    const [step, setStep] = useState<'stop' | 'move' | 'remove' | 'reenable' | null>(null);
    const [error, setError] = useState<ApiError | null>(null);
    const [busy, setBusy] = useState(false);
    const running = read.remote.state === 'known' && read.remote.value.operation?.state === 'running';
    useEffect(() => {
        if (!running) return;
        const timer = setInterval(() => void read.retry(), 3000);
        return () => clearInterval(timer);
    }, [running, read]);
    if (read.remote.state === 'loading') return <p className="text-sm text-fg-muted">{t('emg.reading')}</p>;
    if (read.remote.state === 'unknown') return <p className="text-sm text-warning">{t('emg.readFailed')}</p>;
    const detail = read.remote.value;
    const e = detail.emergency;
    const plain = async (action: 'cancel' | 'continue') => {
        setBusy(true);
        setError(null);
        const result = await post(`${SECURITY_EMERGENCIES_URL}/${encodeURIComponent(id)}/${action}`, {});
        setBusy(false);
        if (result.ok === false) setError(result.error);
        if (result.ok === 'lost') setError({ message: t('emg.confirm.lost') });
        void read.retry();
        onChanged();
    };
    const g = detail.guidance;
    const maintenance = detail.preview.sites.filter((site) => site.maintenance).length;
    return (
        <article className="space-y-3 rounded-lg border border-danger/40 bg-danger/5 p-3 text-sm" data-emergency-card={e.id} data-emergency-stage={e.stage}>
            <header className="flex flex-wrap items-center gap-2">
                <ShieldOff className="h-4 w-4 text-danger" aria-hidden="true" />
                <h4 className="font-semibold text-fg">{e.name}</h4>
                <span className="rounded bg-surface px-2 py-0.5 text-xs text-fg-muted">{t(`emg.stage.${e.stage}` as TranslationKey)}</span>
            </header>
            {/* Guidance first (D-024): what is true, who acts, what next, how it goes on. */}
            <dl className="grid gap-1 sm:grid-cols-[10rem_1fr]" data-emergency-guidance="">
                <dt className="text-fg-muted">{t('emg.guidance.now')}</dt>
                <dd className="text-fg">{g.result}</dd>
                <dt className="text-fg-muted">{t('emg.guidance.who')}</dt>
                <dd className="text-fg">{g.who_acts === 'nobody' ? t('emg.who.nobody') : t('emg.who.administrator')}</dd>
                <dt className="text-fg-muted">{t('emg.guidance.next')}</dt>
                <dd className="text-fg">{g.next_action}</dd>
                {g.resumes && <><dt className="text-fg-muted">{t('emg.guidance.resumes')}</dt><dd className="text-fg">{g.resumes}</dd></>}
            </dl>
            <p className="text-fg-muted">{t('emg.reasonLine', { reason: e.reason })}</p>
            <ErrorBanner error={error} />
            <div className="flex flex-wrap gap-2">
                {detail.can.stop && <Button variant="danger" icon={ShieldOff} onClick={() => setStep('stop')} disabled={busy} className="max-sm:min-h-11" data-emergency-step="stop">{t('emg.step.stop')}</Button>}
                {detail.can.move && <Button onClick={() => setStep('move')} disabled={busy || maintenance === 0} className="max-sm:min-h-11" data-emergency-step="move">{t('emg.step.move')}</Button>}
                {detail.can.remove && <Button variant="danger" icon={Trash2} onClick={() => setStep('remove')} disabled={busy} className="max-sm:min-h-11" data-emergency-step="remove">{t('emg.step.remove')}</Button>}
                {detail.can.reenable && <Button icon={RefreshCw} onClick={() => setStep('reenable')} disabled={busy} className="max-sm:min-h-11" data-emergency-step="reenable">{t('emg.step.reenable')}</Button>}
                {detail.can.cancel && <Button onClick={() => void plain('cancel')} disabled={busy} className="max-sm:min-h-11" data-emergency-step="cancel">{t('emg.step.cancel')}</Button>}
                {detail.can.continue && <Button variant="primary" onClick={() => void plain('continue')} disabled={busy} className="max-sm:min-h-11" data-emergency-step="continue">{t('emg.step.continue')}</Button>}
            </div>
            {detail.operation && (
                <div data-emergency-operation={detail.operation.state}>
                    <p className="font-medium text-fg">{t(stepKey[detail.operation.kind as keyof typeof stepKey] ?? 'emg.step.stop')} · {t('emg.progress', { done: detail.operation.done, total: detail.operation.total })}</p>
                    <ul className="mt-1 space-y-0.5">
                        {detail.operation.effects.map((effect, index) => (
                            <li key={index} className="flex flex-wrap gap-2" data-emergency-effect={effect.state}>
                                <span className="font-mono text-xs text-fg-muted">{effect.target}</span>
                                <span className={effect.state === 'failed' ? 'text-danger' : 'text-fg'}>{t(effectKey[effect.state] ?? 'emg.effect.pending')}</span>
                                {effect.detail && <span className="text-fg-muted">{effect.detail}</span>}
                            </li>
                        ))}
                    </ul>
                </div>
            )}
            <SiteList sites={detail.preview.sites} />
            {detail.notices.total > 0 && (
                <p className="text-fg-muted" data-emergency-notices="">
                    {t('emg.notices', { total: detail.notices.total, seen: detail.notices.seen, sent: detail.notices.mail_sent, pending: detail.notices.mail_pending, failed: detail.notices.mail_failed, none: detail.notices.no_address })}
                    {detail.notices.mail_not_available > 0 && <> {t('emg.noticesNoMail')}</>}
                </p>
            )}
            {step && <ConfirmDialog detail={detail} step={step} onClose={() => setStep(null)} onDone={() => { setStep(null); void read.retry(); onChanged(); }} />}
        </article>
    );
}

function SiteList({ sites }: { sites: EmergencySite[] }) {
    const { t } = useI18n();
    if (sites.length === 0) return <p className="text-fg-muted">{t('emg.sitesNone')}</p>;
    return (
        <details>
            <summary className="cursor-pointer text-fg">{t('emg.sites', { n: sites.length })}</summary>
            <ul className="mt-1 space-y-0.5" data-emergency-sites={sites.length}>
                {sites.map((site) => (
                    <li key={site.domain_id} className="flex flex-wrap gap-2">
                        <span className="font-medium text-fg">{site.domain}</span>
                        <span className="text-fg-muted">{site.customer}{site.reseller ? ` · ${site.reseller}` : ''}</span>
                        {site.maintenance && <span className="text-warning">{t('emg.siteMaintenance')}</span>}
                        {site.moved_to && <span className="text-fg-muted">{t('emg.siteMoved', { version: site.moved_to })}</span>}
                    </li>
                ))}
            </ul>
        </details>
    );
}

function ConfirmDialog({ detail, step, onClose, onDone }: { detail: EmergencyDetail; step: 'stop' | 'move' | 'remove' | 'reenable'; onClose: () => void; onDone: () => void }) {
    const { t } = useI18n();
    const e = detail.emergency;
    const [reason, setReason] = useState(e.reason);
    const [typed, setTyped] = useState('');
    const [password, setPassword] = useState('');
    const [target, setTarget] = useState(e.recommended && detail.alternatives.includes(e.recommended) ? e.recommended : detail.alternatives[0] ?? '');
    const [keepWithdrawn, setKeepWithdrawn] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<ApiError | null>(null);
    const needsName = step !== 'reenable';
    const ready = reason.trim() !== '' && password !== '' && (!needsName || typed.trim() === detail.confirm_name) && (step !== 'move' || target !== '');
    const submit = async () => {
        if (!ready || busy) return;
        setBusy(true);
        setError(null);
        const result = await post(`${SECURITY_EMERGENCIES_URL}/${encodeURIComponent(e.id)}/${step}`, {
            reason, confirm_name: typed.trim(), password, target_version: step === 'move' ? target : undefined, keep_withdrawn: step === 'reenable' ? keepWithdrawn : undefined,
        });
        setBusy(false);
        setPassword('');
        if (result.ok === true) onDone();
        else if (result.ok === 'lost') {
            setError({ message: t('emg.confirm.lost') });
            onDone();
        } else setError(result.error);
    };
    const sites = detail.preview.sites.filter((site) => step === 'stop' || site.maintenance);
    return (
        <Dialog
            id={`emergency-${step}`}
            tone="danger"
            icon={step === 'reenable' ? RefreshCw : ShieldOff}
            dismissible={false}
            busy={busy}
            width="lg"
            title={t('emg.confirm.title', { step: t(stepKey[step]), name: e.name })}
            onSubmit={(event) => { event.preventDefault(); void submit(); }}
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={busy}>{t('common.cancel')}</Button>
                    <Button type="submit" variant="danger" loading={busy} disabled={!ready || busy} data-emergency-confirm={step}>{t('emg.confirm.submit')}</Button>
                </>
            }
        >
            <div className="space-y-3 text-sm">
                <p className="text-fg">{t(`emg.step.${step}Help` as TranslationKey)}</p>
                <div data-emergency-preview="">
                    <p className="font-medium text-fg">{t('emg.confirm.affects')}</p>
                    {sites.length === 0 ? <p className="text-fg-muted">{t('emg.sitesNone')}</p> : (
                        <ul className="mt-1 max-h-40 overflow-auto rounded border border-border p-2">
                            {sites.map((site) => <li key={site.domain_id}>{site.domain} <span className="text-fg-muted">— {site.customer}{site.reseller ? ` · ${site.reseller}` : ''}</span></li>)}
                        </ul>
                    )}
                    <p className="mt-1 text-fg-muted">{t('emg.confirm.customers', { n: detail.preview.customers.length })}: {detail.preview.customers.map((c) => c.name).join(', ') || '—'}</p>
                    <p className="text-fg-muted">{t('emg.confirm.resellers', { n: detail.preview.resellers.length })}: {detail.preview.resellers.map((c) => c.name).join(', ') || '—'}</p>
                    {detail.preview.offered_by.length > 0 && <p className="text-fg-muted">{t('emg.confirm.offeredBy')}: {detail.preview.offered_by.join(', ')}</p>}
                    {detail.preview.notes.map((note, index) => <p key={index} className="text-fg-muted">{note}</p>)}
                </div>
                {step === 'move' && (
                    <label className="block">
                        <span className="text-fg-muted">{t('emg.confirm.target')}</span>
                        {detail.alternatives.length === 0 ? <p className="text-warning">{t('emg.confirm.targetNone')}</p> : (
                            <select className="mt-1 block w-full max-w-xs rounded-md border border-border bg-surface px-2 py-1.5" value={target} onChange={(ev) => setTarget(ev.target.value)} data-emergency-target="">
                                {detail.alternatives.map((item) => <option key={item} value={item}>{item}</option>)}
                            </select>
                        )}
                    </label>
                )}
                <label className="block">
                    <span className="text-fg-muted">{t('emg.confirm.reason')}</span>
                    <input className="mt-1 block w-full rounded-md border border-border bg-surface px-2 py-1.5" value={reason} maxLength={500} onChange={(ev) => setReason(ev.target.value)} data-emergency-confirm-reason="" />
                </label>
                {needsName && (
                    <label className="block">
                        <span className="text-fg-muted">{t('emg.confirm.typeName', { name: detail.confirm_name })}</span>
                        <input className="mt-1 block w-full rounded-md border border-border bg-surface px-2 py-1.5 font-mono" value={typed} autoComplete="off" spellCheck={false} onChange={(ev) => setTyped(ev.target.value)} data-emergency-confirm-name="" />
                    </label>
                )}
                {step === 'reenable' && (
                    <label className="flex items-center gap-2">
                        <input type="checkbox" checked={keepWithdrawn} onChange={(ev) => setKeepWithdrawn(ev.target.checked)} />
                        <span>{t('emg.confirm.keepWithdrawn')}</span>
                    </label>
                )}
                <label className="block">
                    <span className="text-fg-muted">{t('emg.confirm.password')}</span>
                    <input type="password" autoComplete="current-password" className="mt-1 block w-full rounded-md border border-border bg-surface px-2 py-1.5" value={password} onChange={(ev) => setPassword(ev.target.value)} data-emergency-confirm-password="" />
                </label>
                <ErrorBanner error={error} />
            </div>
        </Dialog>
    );
}
