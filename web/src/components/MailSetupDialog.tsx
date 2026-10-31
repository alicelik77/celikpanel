import { useCallback, useEffect, useRef, useState } from 'react';
import { Check, Circle, Loader2 } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useNavigate } from '../router';
import { componentName } from '../lib/componentName';
import { readApiError } from '../lib/apiError';
import { decodeServerSetup, setupJoinedMailProposal, setupJoinedMailTaken, setupProposedNames, validSetupDomain, type ServerSetupSnapshot } from '../lib/serverSetup';
import { decodeSetupExecution, decodeSetupPlan, newSetupRequestID, type ServerSetupExecution, type ServerSetupPlan, type SetupStartMarker } from '../lib/serverSetupOperation';
import { setupComponentName, setupExecutionGuidance, setupStepState, setupStepStateLabel, type SetupGuidanceText } from '../lib/serverSetupGuidance';
import { setupStartMarkerKey } from '../lib/panelHandover';
import { startVisibleReads } from '../lib/visibleReads';
import { useServerSetup } from './ServerSetupGate';
import { SetupPlanProblems } from './SetupPlanProblems';
import { invalidate, readRemote, useRemote } from '../lib/remote';
import { Button, Dialog, inputClass } from './ui';

// Adding mail in place (alpha.114, owner decision 2026-10-31).
//
// On a server whose setup has finished, installing a mail profile from
// Components (or making the main domain's missing DKIM key from the DNS page)
// no longer sends the owner into the setup wizard. This dialog asks for the
// mail host name only, begins a mail-only extension of the finished setup
// (POST /api/v1/setup/extend, scope "mail"), shows the steps the server plans
// (the mail name's address record, its certificate, the packages, the
// independent renewal, the final check), starts them after one confirmation
// and shows their progress and outcome here. The server's runner does the
// work; closing the window stops nothing, and every read is a read: the start
// is sent once per confirmation under one request identity (D-024).
//
// Yerinde posta ekleme: yalniz posta adi sorulur, adimlar gosterilir, onaydan
// sonra burada ilerler. Pencereyi kapatmak isi durdurmaz.

// What a joined server needs to propose its own mail name (P1): the first
// server's names, read from the setup link (GET /api/v1/setup/peer).
interface MailSetupPeer { main_domain: string; ns1: string; ns2: string; first: { panel_domain: string; mail_hostname?: string } }
function decodeMailSetupPeer(value: unknown): MailSetupPeer | null {
    const record = (item: unknown): item is Record<string, unknown> => !!item && typeof item === 'object' && !Array.isArray(item);
    const text = (item: unknown) => typeof item === 'string' && item.length <= 253;
    if (!record(value) || !['none', 'first', 'second'].includes(String(value.role))) throw new Error('invalid peer state');
    const link = value.link;
    if (value.role !== 'second' || !record(link)) return null;
    if (!text(link.main_domain) || !text(link.ns1) || !text(link.ns2) || !record(link.first) || !text(link.first.panel_domain)
        || (link.first.mail_hostname !== undefined && !text(link.first.mail_hostname))) throw new Error('invalid peer state');
    return link as unknown as MailSetupPeer;
}
function forgetMarker(username: string) {
    try { localStorage.removeItem(setupStartMarkerKey(username)); } catch { /* the marker only selects what to read */ }
}

function useMailSetupPeer(): { link: MailSetupPeer | null; known: boolean } {
    const remote = useRemote('/api/v1/setup/peer', decodeMailSetupPeer);
    return { link: remote.remote.state === 'known' ? remote.remote.value : null, known: remote.remote.state !== 'loading' };
}

// Reads the setup again; an unreadable answer is "not known" (null), never a guess.
async function reloadSetup(setup: { reload: () => Promise<ServerSetupSnapshot> }): Promise<ServerSetupSnapshot | null> {
    try { return await setup.reload(); }
    catch { return null; }
}

type Phase = 'name' | 'reading' | 'review' | 'starting' | 'progress';
const post = (body: unknown) => ({ method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
const plainName = (value: string) => value.trim().toLowerCase().replace(/\.$/, '');

export interface MailSetupDialogProps {
    /** The profile to add; absent: run the mail steps again (the DKIM key). */
    profile?: { id: string; name: string };
    profileName?: string;
    /** The name saved for the mail services, when one is. */
    savedHostname?: string;
    onClose: () => void;
}

export function MailSetupDialog({ profile, profileName, savedHostname, onClose }: MailSetupDialogProps) {
    const { t } = useI18n();
    const setup = useServerSetup();
    const { user } = useAuth();
    const navigate = useNavigate();
    const peer = useMailSetupPeer();
    const snapshot = setup?.snapshot ?? null;
    const draft = snapshot?.draft;
    const repairOnly = !profile;
    // The proposal (P1): mail.<panel name> on a joined server, whose main
    // domain's mail stays with the first server; mail.<main domain> otherwise.
    const joined = peer.link;
    const mainDomain = plainName(joined?.main_domain || draft?.main_domain || '');
    const general = setupProposedNames(validSetupDomain(mainDomain) ? mainDomain : '', draft?.short_name || '');
    const proposal = joined ? setupJoinedMailProposal(general, draft?.panel_domain || '').mail : general.mail;
    const joinedNames = joined ? { firstMail: joined.first.mail_hostname || '', firstPanel: joined.first.panel_domain, mainDomain: joined.main_domain, ns1: joined.ns1, ns2: joined.ns2 } : null;
    const known = plainName(draft?.mail_hostname || savedHostname || '');
    const [hostname, setHostname] = useState(known);
    const [touched, setTouched] = useState(false);
    useEffect(() => {
        // The proposal arrives with the peer read; a name the owner typed is kept.
        if (!touched && !known && proposal) setHostname(proposal);
    }, [proposal, known, touched]);
    const name = plainName(hostname);
    const taken = !!joinedNames && setupJoinedMailTaken(name, joinedNames);
    const invalid = name !== '' && !validSetupDomain(name);
    const hostnameReady = repairOnly ? !!known : !!name && !taken && !invalid && peer.known;

    const [phase, setPhase] = useState<Phase>('name');
    const [problem, setProblem] = useState<TranslationKey | null>(null);
    const [plan, setPlan] = useState<ServerSetupPlan | null>(null);
    const [acknowledged, setAcknowledged] = useState(false);
    const [execution, setExecution] = useState<ServerSetupExecution | null>(null);
    const [marker, setMarker] = useState<SetupStartMarker | null>(null);
    const begun = useRef<ServerSetupSnapshot | null>(null);
    const inFlight = useRef(false);
    const announced = useRef(false);
    const alive = useRef(true);
    useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

    const label = (kind: string, target: string) => {
        if (kind === 'mail_profile') return t(target === 'protected-mail' ? 'dashboard.audit.profile.protectedMail' : target === 'core-mail' ? 'dashboard.audit.profile.coreMail' : 'dashboard.audit.profile.webmail');
        if (kind === 'service') return componentName({ id: target, name: setupComponentName(target) }, t);
        return target;
    };
    const stepText = (kind: string, target: string) => t(`setup.kind.${kind}` as TranslationKey, { target: label(kind, target) });
    const say = (message: SetupGuidanceText) => t(message.key, message.name ? { ...message.values, check: t(message.name) } : message.values);

    // Begin the mail-only extension, then read its plan. A lost answer is
    // settled by reading the setup again, never by sending it twice.
    async function review() {
        if (!setup || !snapshot || inFlight.current || !hostnameReady) return;
        inFlight.current = true; setPhase('reading'); setProblem(null);
        try {
            let next: ServerSetupSnapshot | null = null;
            let code = 'unanswered';
            try {
                const response = await fetch('/api/v1/setup/extend', post({ revision: snapshot.revision, action: 'begin', scope: 'mail', ...(profile ? { mail_profile: profile.id } : {}), ...(!repairOnly && name !== known ? { mail_hostname: name } : {}) }));
                if (response.ok) { next = decodeServerSetup(await response.json()); code = ''; }
                else code = (await readApiError(response)).code || 'refused';
            } catch { code = 'unanswered'; }
            if (!next?.extension) {
                const latest = await reloadSetup(setup);
                if (code === 'server_setup_busy') throw new Error('svc.mailSetup.busy');
                if (code === 'mail_profile_server_hostname_invalid' || code === 'mail_profile_server_hostname_required') throw new Error('svc.mailSetup.hostnameInvalidShort');
                if (!latest?.extension || latest.extension_scope !== 'mail' || code !== 'unanswered') throw new Error('svc.mailSetup.readFailed');
                next = latest;
            }
            setup.accept(next); begun.current = next;
            const response = await fetch('/api/v1/setup/plan', post({ revision: next.revision }));
            if (!response.ok) throw new Error('svc.mailSetup.readFailed');
            const reviewed = decodeSetupPlan(await response.json(), next.revision);
            if (!reviewed) throw new Error('svc.mailSetup.readFailed');
            if (!alive.current) return;
            setPlan(reviewed); setAcknowledged(false); setPhase('review');
        } catch (failure) {
            if (!alive.current) return;
            const key = failure instanceof Error ? failure.message : '';
            setProblem((key === 'svc.mailSetup.hostnameInvalidShort' ? 'svc.mailSetup.readFailed' : key.startsWith('svc.mailSetup.') ? key : 'svc.mailSetup.readFailed') as TranslationKey);
            setPhase(begun.current ? 'review' : 'name');
        } finally { inFlight.current = false; }
    }

    async function start() {
        if (!plan?.can_start || !acknowledged || inFlight.current) return;
        inFlight.current = true; setPhase('starting'); setProblem(null);
        const started: SetupStartMarker = { request_id: newSetupRequestID(), plan_id: plan.id, panel_domain: snapshot?.draft.panel_domain || '', ...(plan.package_updates_digest ? { package_updates_digest: plan.package_updates_digest } : {}), ...(snapshot?.installation_id ? { installation: snapshot.installation_id } : {}) };
        // The same start marker the wizard keeps, so Server setup shows this run too.
        try { localStorage.setItem(setupStartMarkerKey(user.username), JSON.stringify(started)); } catch { /* the marker only selects what to read */ }
        setMarker(started);
        try {
            const response = await fetch('/api/v1/setup/start', post({ plan_id: started.plan_id, request_id: started.request_id, confirmed: true, package_updates_digest: started.package_updates_digest ?? '' }));
            if (response.ok) {
                const accepted = decodeSetupExecution(await response.json(), started);
                if (accepted && alive.current) setExecution(accepted);
            } else if (response.status === 409) {
                const code = (await readApiError(response)).code;
                if (code && !['server_setup_request_conflict'].includes(code)) {
                    // Refused before any run: the review is read again.
                    forgetMarker(user.username);
                    setMarker(null);
                    if (alive.current) { setProblem(code === 'server_setup_host_busy' || code === 'server_setup_busy' ? 'svc.mailSetup.busy' : 'svc.mailSetup.readFailed'); setPhase('review'); }
                    return;
                }
            }
            if (alive.current) setPhase('progress');
        } catch {
            // No answer: the run is read under the same request identity.
            if (alive.current) { setProblem('svc.mailSetup.unknown'); setPhase('progress'); }
        } finally { inFlight.current = false; }
    }

    // One read of this run under its own request identity. A read that fails
    // is said ("not known yet, read again"); it never starts anything.
    const read = useCallback(async () => {
        if (!marker) return;
        const answer = await readRemote(`/api/v1/setup/operation?request_id=${encodeURIComponent(marker.request_id)}`, (value: unknown) => {
            const next = value === null ? null : decodeSetupExecution(value, marker);
            if (value !== null && !next) throw new Error('operation identity');
            return next;
        });
        if (!alive.current) return;
        if (answer.state !== 'known') { setProblem('svc.mailSetup.unknown'); return; }
        const next = answer.value;
        if (!next) return;
        setExecution(next); setProblem(null);
        // A finished run (done or stopped) changed components, mail and DNS
        // records: every screen showing them reads again, once. Reads only.
        // Biten calisma bilesenleri, postayi ve DNS'i degistirdi; bir kez okunur.
        if ((next.status === 'succeeded' || next.status === 'failed') && !announced.current) {
            announced.current = true;
            invalidate('components', 'mail', 'dns');
        }
        if (next.status === 'succeeded' && setup) {
            const latest = await reloadSetup({ reload: () => setup.reload(true) });
            if (latest) setup.accept(latest);
            forgetMarker(user.username);
        }
    }, [marker, setup, user.username]);
    useEffect(() => {
        if (phase !== 'progress' || !marker || execution?.status === 'succeeded' || execution?.status === 'failed') return;
        return startVisibleReads(read, 3000, { setInterval: (callback, ms) => window.setInterval(callback, ms), clearInterval: id => window.clearInterval(id) }, { immediate: true });
    }, [phase, marker, execution?.status, read]);

    // Closing before the start stops the extension: the setup is back as it was.
    async function close() {
        if (inFlight.current) return;
        const current = setup?.snapshot;
        if (begun.current && !marker && current?.extension && setup) {
            inFlight.current = true;
            try {
                const response = await fetch('/api/v1/setup/extend', post({ revision: current.revision, action: 'cancel' }));
                const next = response.ok && decodeServerSetup(await response.json());
                if (next) setup.accept(next); else await reloadSetup(setup);
            } catch { await reloadSetup(setup); }
            finally { inFlight.current = false; }
        }
        onClose();
    }
    async function stopAfterFailure() {
        const current = setup?.snapshot;
        if (!setup || !current?.extension || inFlight.current) return;
        inFlight.current = true;
        try {
            const response = await fetch('/api/v1/setup/extend', post({ revision: current.revision, action: 'cancel' }));
            const next = response.ok && decodeServerSetup(await response.json());
            if (next) setup.accept(next); else await reloadSetup(setup);
            forgetMarker(user.username);
        } catch {
            // No answer: the setup is read again; the owner sees it as it is.
            const latest = await reloadSetup(setup);
            if (latest) setup.accept(latest);
        } finally { inFlight.current = false; }
        onClose();
    }

    const title = repairOnly ? t('svc.mailSetup.titleDKIM') : t('svc.mailSetup.title', { name: profileName || profile?.name || '' });
    const guidance = execution && execution.status !== 'succeeded' ? setupExecutionGuidance(execution, id => componentName({ id, name: setupComponentName(id) }, t)) : null;
    const done = execution?.status === 'succeeded';
    const failed = execution?.status === 'failed';
    const steps = execution ? execution.steps : plan?.steps ?? [];
    const busy = phase === 'reading' || phase === 'starting';

    return <Dialog id="mail-setup" width="xl" title={title} description={t('svc.mailSetup.intro')} onDismiss={() => void close()} busy={busy}
        footerLead={phase === 'review' && plan?.can_start ? <label className="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3 text-sm leading-5">
            <input type="checkbox" checked={acknowledged} onChange={event => setAcknowledged(event.target.checked)} className="mt-0.5 h-4 w-4 rounded-md border-border-strong text-primary" />
            <span>{t('svc.mailSetup.confirm')}</span></label> : undefined}
        actions={<>
            {(phase === 'progress' && failed) && <Button variant="secondary" onClick={() => void stopAfterFailure()}>{t('svc.mailSetup.stop')}</Button>}
            {(phase === 'progress' && failed) && <Button variant="secondary" onClick={() => navigate('/setup?purpose=mail')}>{t('svc.mailSetup.openSetup')}</Button>}
            <Button variant="secondary" disabled={busy} onClick={() => void close()}>{t(phase === 'progress' ? 'svc.mailSetup.close' : 'common.cancel')}</Button>
            {(phase === 'name' || phase === 'reading') && <Button variant="primary" disabled={!hostnameReady || busy} loading={phase === 'reading'} onClick={() => void review()}>{t('svc.mailSetup.review')}</Button>}
            {(phase === 'review' || phase === 'starting') && <Button variant="primary" disabled={!plan?.can_start || !acknowledged || busy} loading={phase === 'starting'} onClick={() => void start()}>{t('svc.mailSetup.start')}</Button>}
        </>}>
        <div className="space-y-4" data-mail-setup={phase}>
            {phase === 'name' || phase === 'reading' ? repairOnly
                ? <p className="text-sm leading-6" data-mail-setup-hostname="fixed">{t('svc.mailSetup.hostnameFixed', { name: known })}</p>
                : <div className="space-y-2">
                    <label className="block" htmlFor="mail-setup-hostname"><span className="mb-2 block text-sm font-medium">{t('svc.mailSetup.hostname')}</span>
                        <input id="mail-setup-hostname" value={hostname} disabled={busy} onChange={event => { setTouched(true); setHostname(event.target.value); }} placeholder={proposal} autoCapitalize="none" autoComplete="off" spellCheck={false}
                            aria-invalid={taken || invalid ? true : undefined} aria-describedby="mail-setup-hostname-help" className={`${inputClass} w-full font-mono`} /></label>
                    <p id="mail-setup-hostname-help" className={`text-sm leading-6 ${taken || invalid ? 'text-danger' : 'text-fg-muted'}`} data-mail-setup-hostname={taken ? 'taken' : invalid ? 'invalid' : 'ok'}>
                        {taken ? t('svc.mailSetup.hostnameTaken', { name, proposal }) : invalid ? t('svc.mailSetup.hostnameInvalid', { proposal })
                            : joined && joined.first.mail_hostname ? t('svc.mailSetup.hostnameJoined', { first: joined.first.mail_hostname, domain: joined.main_domain, proposal }) : t('svc.mailSetup.hostnameHelp', { proposal: proposal || 'mail.example.com' })}
                    </p>
                </div> : null}
            {phase === 'reading' && <p role="status" className="flex items-center gap-2 text-sm text-fg-muted"><Loader2 className="h-4 w-4 motion-safe:animate-spin" aria-hidden="true" />{t('svc.mailSetup.reading')}</p>}
            {problem && <p role="alert" className="rounded-lg border border-border bg-surface p-3 text-sm leading-6">{t(problem)}</p>}
            {phase === 'review' && plan && plan.blockers.length > 0 && <div className="space-y-2" data-mail-setup-blocked="">
                <p className="text-sm font-medium">{t('svc.mailSetup.blocked')}</p>
                <SetupPlanProblems blockers={plan.blockers} fallback={() => t('setup.blocker.unknown')} name={id => componentName({ id, name: setupComponentName(id) }, t)} busy={busy}
                    onFix={fix => { if (fix.kind === 'recheck') void review(); else navigate('/setup?purpose=mail'); }} />
            </div>}
            {phase === 'progress' && <div role={failed ? 'alert' : 'status'} className="space-y-2 rounded-lg border border-border bg-surface p-4 text-sm leading-6" data-mail-setup-outcome={done ? 'done' : failed ? 'failed' : execution?.status || 'unknown'}>
                <p className="font-medium">{done ? (repairOnly ? t('svc.mailSetup.doneDKIM') : t('svc.mailSetup.done', { host: name || known })) : failed ? t('svc.mailSetup.failed') : t('svc.mailSetup.running')}</p>
                {guidance && <>{guidance.title && <p className="font-medium">{t(guidance.title)}</p>}{guidance.messages.map((message, index) => <p key={index} className="break-words text-fg-muted">{say(message)}</p>)}</>}
                {failed && <p className="text-fg-muted">{t('svc.mailSetup.failedNext')}</p>}
            </div>}
            {steps.length > 0 && phase !== 'name' && phase !== 'reading' && <section aria-labelledby="mail-setup-steps">
                <h3 id="mail-setup-steps" className="text-sm font-semibold">{t('svc.mailSetup.stepsTitle')}</h3>
                <ol className="mt-2 divide-y divide-border border-y border-border" aria-live="polite">{steps.map(item => {
                    const state = execution ? setupStepState(execution, item as ServerSetupExecution['steps'][number]) : null;
                    return <li key={item.id} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm" data-mail-setup-step={item.kind} data-step-state={state || 'planned'}>
                        <span className="min-w-0 break-words">{stepText(item.kind, item.target)}</span>
                        {state && <span className="flex shrink-0 items-center gap-2 text-fg-muted">{state === 'succeeded' ? <Check className="h-4 w-4 text-success" aria-hidden="true" /> : state === 'running' ? <Loader2 className="h-4 w-4 motion-safe:animate-spin" aria-hidden="true" /> : <Circle className="h-3 w-3" aria-hidden="true" />}{t(setupStepStateLabel[state])}</span>}
                    </li>;
                })}</ol>
            </section>}
        </div>
    </Dialog>;
}
