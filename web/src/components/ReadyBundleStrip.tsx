// The ready bundles strip (alpha.94, D-034; alpha.95: one fixed chip list per
// category): one line at the head of a Components category, between its
// header and its rows. A bundle is a
// shortcut over the rows below it, never a second install address: a mail
// bundle opens the existing mail install dialogue, and a database or web
// bundle installs its missing components one by one through the same
// component install the rows use.
// Hazır paketler şeridi: bir Bileşenler bölümünün başında tek satır. Paket,
// altındaki satırlar için bir kısayoldur, ikinci bir kurulum adresi değildir.
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Check, DownloadCloud, Layers, ScanSearch, Wrench } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Checking, Dialog, KeepWords, StatusDot } from './ui';
import { componentName } from '../lib/componentName';
import { mailProfileAttemptError, mailProfileBlocked, mailProfileWarning, requirementText } from '../lib/mailProfileText';
import {
    bundleAbout,
    bundleChipName,
    bundleDescription,
    bundleInstalled,
    bundleName,
    bundleStripModel,
    type BundleChip,
    type ReadyBundle,
} from '../lib/readyBundles';
import { componentIcon } from '../lib/componentIcons';
import type { InstallOperationRequest } from './ComponentOperation';

interface ComponentLite { id: string; name: string; category?: string; is_installed: boolean | null }
interface ProfileLite { id: string; name: string; warning?: string; latest_attempt_error?: string }

// Why a sequence stopped before its last component:
// - failed: the component's install failed; the tracker shows its guidance;
// - foreign: the operation that ran was not this step's (another screen or
//   tab started one), so nothing here is confirmed;
// - gated: changes are paused on this server (an unverified page state or a
//   busy host), checked again before every step;
// - refused: the install was not started; the tracker says why.
// Dizinin son bileşenden önce neden durduğu.
export type BundleStop = 'failed' | 'foreign' | 'gated' | 'refused';

// A bundle install of this page: the steps (missing components, in the
// Panel's order), the one being installed, and why it stopped if it did. It
// lives in this page's memory only, so a reload never continues it by itself:
// the bundle then reads as offered again and Install installs only what is
// missing (owner decision, 2026-10-17).
// Bu sayfanın paket kurulumu; yalnız sayfanın belleğinde yaşar.
export interface BundleRun {
    bundleId: string;
    steps: string[];
    index: number;
    phase: 'starting' | 'running' | 'stopped';
    stop?: BundleStop;
    reason?: string;
}

interface OperationView {
    operation: unknown;
    failure: unknown;
    catalogSnapshot: unknown;
    locked: boolean;
    startInstall: (request: InstallOperationRequest) => Promise<boolean>;
}

const operationOf = (value: unknown): { service_id: string; kind: string } | null => {
    if (!value || typeof value !== 'object') return null;
    const record = value as Record<string, unknown>;
    return { service_id: typeof record.service_id === 'string' ? record.service_id : '', kind: typeof record.kind === 'string' ? record.kind : '' };
};

// The components the tracker's fresh scan says are installed: the scan that
// ended the step, not a list kept from before it.
// İşlemi bitiren taze taramanın kurulu dediği bileşenler.
const installedIn = (snapshot: unknown): Set<string> => {
    const services = snapshot && typeof snapshot === 'object' ? (snapshot as { services?: unknown }).services : undefined;
    const ids = new Set<string>();
    if (Array.isArray(services)) {
        for (const item of services) {
            if (item && typeof item === 'object' && (item as Record<string, unknown>).is_installed === true) {
                const id = (item as Record<string, unknown>).id;
                if (typeof id === 'string') ids.add(id);
            }
        }
    }
    return ids;
};

// useBundleSequence installs a bundle's missing components in order, one
// component install per step. The next install is sent only when the
// operation that just ended was this step's own component install and the
// tracker has read the components again (one active operation per host), and
// only after the page's change gate is checked again. The first failure, an
// operation that was not this step's, or a closed gate stops the sequence;
// only the owner's Continue starts it again. Each step is sent once: a
// re-render or a poll never sends it again.
// Eksik bileşenleri sırayla kurar; sonraki kurulum ancak biten işlem bu adımın
// kendi kurulumuysa, izleyici taze tarama okuduysa ve değişiklik kapısı açıksa
// gönderilir.
export function useBundleSequence(
    tracker: OperationView,
    installedNow: () => Set<string>,
    label: (bundleId: string, componentId: string, index: number, total: number) => string,
    gate: () => Promise<string | null>,
) {
    const [run, setRun] = useState<BundleRun | null>(null);
    const sentRef = useRef('');
    const baselineRef = useRef<unknown>(null);
    const seenRef = useRef<{ service_id: string; kind: string } | null>(null);
    const snapshotRef = useRef(tracker.catalogSnapshot);
    snapshotRef.current = tracker.catalogSnapshot;

    const stop = (current: BundleRun, why: BundleStop, reason?: string) => setRun({ ...current, phase: 'stopped', stop: why, reason });

    const send = async (current: BundleRun, index: number) => {
        const key = `${current.bundleId}:${current.steps.join(',')}:${index}`;
        if (sentRef.current === key) return;
        sentRef.current = key;
        const at = { ...current, index };
        setRun({ ...at, phase: 'starting', stop: undefined, reason: undefined });
        const closed = await gate();
        if (closed) {
            stop(at, 'gated', closed);
            return;
        }
        baselineRef.current = snapshotRef.current;
        seenRef.current = null;
        const componentId = current.steps[index];
        const started = await tracker.startInstall({
            serviceId: componentId,
            name: label(current.bundleId, componentId, index, current.steps.length),
        });
        if (!started) {
            stop(at, 'refused');
            return;
        }
        setRun((previous) => (previous && previous.bundleId === current.bundleId && previous.phase === 'starting' ? { ...previous, phase: 'running' } : previous));
    };

    const begin = (bundle: ReadyBundle) => {
        if ((run !== null && run.phase !== 'stopped') || tracker.locked) return;
        const installed = installedNow();
        const steps = bundle.missing_components.filter((id) => !installed.has(id));
        if (steps.length === 0) return;
        sentRef.current = '';
        void send({ bundleId: bundle.id, steps, index: 0, phase: 'starting' }, 0);
    };

    useEffect(() => {
        if (!run || run.phase !== 'running') return;
        const step = run.steps[run.index];
        const operation = operationOf(tracker.operation);
        if (operation) {
            seenRef.current = operation;
            // The tracker follows an operation that is not this step's (a
            // busy host made it adopt another one): nothing here is ours.
            if (operation.service_id !== step || operation.kind !== 'service_install') stop(run, 'foreign');
            return;
        }
        if (tracker.locked) return;
        if (tracker.failure) {
            stop(run, 'failed');
            return;
        }
        // The step ended without a fresh scan from the tracker: nothing is
        // confirmed, so nothing more is sent.
        if (tracker.catalogSnapshot === baselineRef.current) return;
        const seen = seenRef.current;
        if (!seen || seen.service_id !== step || seen.kind !== 'service_install') {
            stop(run, 'foreign');
            return;
        }
        const installed = installedIn(tracker.catalogSnapshot);
        let index = run.index + 1;
        while (index < run.steps.length && installed.has(run.steps[index])) index += 1;
        if (index >= run.steps.length) {
            setRun(null);
            return;
        }
        void send(run, index);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [run, tracker.operation, tracker.failure, tracker.catalogSnapshot, tracker.locked]);

    return { run, begin };
}

// The chips of a strip (owner decision, 2026-10-17): every category shows the
// same bundles in the same order on every server, and only the state on each
// chip changes. A state is said in words and in form, never by colour alone.
// Şeridin çipleri: her bölüm her sunucuda aynı paketleri aynı sırayla
// gösterir; yalnız her çipin üzerindeki durum değişir.
const chipBase = 'inline-flex min-h-[30px] max-w-full items-center gap-1.5 rounded-lg px-2.5 py-1 text-[13px] leading-5 max-sm:min-h-11';
const chipFocus = 'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary';
// An installed chip: quiet, a door to its dialogue.
const installedChip = `${chipBase} ${chipFocus} whitespace-nowrap border border-border bg-surface text-fg transition-colors hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-60`;
// A chip that installs: an outlined secondary button, never filled.
const actionChip = `${chipBase} ${chipFocus} whitespace-nowrap border border-border-strong bg-surface font-semibold text-fg transition-colors hover:bg-surface-2 disabled:cursor-not-allowed disabled:border-dashed disabled:bg-transparent disabled:text-fg-muted`;
// A chip that cannot act: calm muted text, no button.
const quietChip = `${chipBase} text-fg-muted`;

function Dot() {
    return <span className='text-fg-subtle' aria-hidden='true'>·</span>;
}

export function ReadyBundleStrip({
    category,
    bundles,
    services,
    profiles,
    run,
    disabled,
    scanning,
    verifyHeld,
    notice,
    onMail,
    onComponents,
    onVerify,
}: {
    category: string;
    bundles: readonly ReadyBundle[];
    services: readonly ComponentLite[];
    /** The mail profiles, for the reasons a mail bundle says again. */
    profiles: readonly ProfileLite[];
    run: BundleRun | null;
    /** Controls that change the server. */
    disabled: boolean;
    scanning: boolean;
    /** Why Verify (a rescan) cannot be pressed now, or null when it can. */
    verifyHeld: string | null;
    /** One line said for the whole category (mail DNS); the chips stay beside it. */
    notice?: ReactNode;
    onMail: (bundle: ReadyBundle) => void;
    onComponents: (bundle: ReadyBundle) => void;
    onVerify: (bundle: ReadyBundle) => void;
}) {
    const { t } = useI18n();
    const [details, setDetails] = useState<string | null>(null);
    // The blocked chip whose full reason is said under the chips: a chip is
    // a button, so the reason is reachable by keyboard and touch, not only by
    // a pointer's tooltip (alpha.95 review).
    // Tam nedeni çiplerin altında söylenen engelli çip.
    const [reasonShown, setReasonShown] = useState<string | null>(null);
    const model = bundleStripModel(bundles, category);
    if (model.empty) return null;
    const nameOf = (id: string) => componentName(services.find((service) => service.id === id) ?? id, t);
    const name = (bundle: ReadyBundle) => bundleName(bundle, t, nameOf);
    const chipName = (bundle: ReadyBundle) => bundleChipName(bundle, t, nameOf);
    const installedSet = new Set(services.filter((service) => service.is_installed === true).map((service) => service.id));
    // While a bundle of this page installs, every other chip waits.
    const runActive = run !== null && run.phase !== 'stopped';
    const own = run ? bundles.find((bundle) => bundle.id === run.bundleId && bundle.category === category) : undefined;
    const stopped = own && run && run.phase === 'stopped' && !bundleInstalled(own) ? own : undefined;

    // What a mail bundle says again (D-024): its last attempt that failed or
    // is running, else the Panel's typed warning. Only a webmail runtime that
    // is installed but not ready is drawn in Amber Ink.
    // Bir posta paketinin yeniden söylediği: başarısız ya da süren son deneme,
    // yoksa Panel'in türlü uyarısı.
    const reasonText = (bundle: ReadyBundle): string => {
        if (bundle.action_route !== 'mail_profile') return '';
        const profile = profiles.find((item) => item.id === bundle.profile_id);
        if (bundle.latest_attempt_status === 'in_progress') return t('services.bundles.attemptRunning');
        if (bundle.latest_attempt_status === 'failed') return (profile ? mailProfileAttemptError(profile, t) : '') || t('services.bundles.lastAttemptFailed');
        if (bundle.warning_code) return mailProfileWarning({ id: bundle.id, name: name(bundle), warning_code: bundle.warning_code, warning: profile?.warning }, t);
        return '';
    };
    const reason = (bundle: ReadyBundle): ReactNode => {
        const text = reasonText(bundle);
        if (!text) return null;
        return (
            <span className={`text-xs leading-5 ${bundle.warning_code === 'roundcube_not_ready' && !bundle.latest_attempt_status?.match(/failed|in_progress/) ? 'text-warning' : 'text-fg-muted'}`} data-bundle-reason={bundle.id}>
                {text}
            </span>
        );
    };
    const blockedSentence = (bundle: ReadyBundle) => mailProfileBlocked({ id: bundle.id, name: name(bundle), blocked_code: bundle.blocked_code, blocked_vars: bundle.blocked_vars }, t);
    // The short reason a blocked chip carries; the full sentence is its title.
    const blockedShort = (bundle: ReadyBundle): string => {
        const vars = bundle.blocked_vars ?? {};
        if (bundle.blocked_code === 'component_requires' && vars.required) return t('services.bundles.short.component_requires', { required: requirementText(vars.required, t) });
        if (bundle.blocked_code === 'component_conflict' && vars.conflicting) return t('services.bundles.short.component_conflict', { conflicting: vars.conflicting });
        if (bundle.blocked_code === 'component_not_installable') return t('services.bundles.short.component_not_installable');
        if (bundle.blocked_code === 'dns_identity_not_ready') return t('services.bundles.short.dns_identity_not_ready');
        if (bundle.blocked_code === 'hostname_unknown') return t('services.bundles.short.hostname_unknown');
        return t('services.bundles.short.other');
    };
    const act = (bundle: ReadyBundle) => {
        if (bundle.action_route === 'mail_profile') onMail(bundle);
        else onComponents(bundle);
    };

    const chip = ({ bundle, state, includedIn }: BundleChip): ReactNode => {
        const key = bundle.id;
        const common = { 'data-bundle': bundle.id, 'data-bundle-status': bundle.status, 'data-bundle-state': state } as const;
        const label = <KeepWords text={chipName(bundle)} />;
        if (runActive && run && run.bundleId === bundle.id) {
            // D-033: the wait moves and says what it waits for. "Installing"
            // is said only once the server accepted the install; before
            // that, the page is reading whether the server may change.
            // Bekleme hareket eder; "kuruluyor" yalnız sunucu kabul edince.
            const component = nameOf(run.steps[run.index]);
            return (
                <span key={key} role='status' aria-live='polite' className={`${chipBase} border border-border-strong bg-surface text-fg`} {...common} data-bundle-running={bundle.id} data-bundle-phase={run.phase}>
                    <span className='font-semibold'>{label}</span>
                    <Dot />
                    <span className='text-fg-muted'>
                        <KeepWords text={run.phase === 'running'
                            ? t('services.bundles.step', { n: run.index + 1, total: run.steps.length, component })
                            : t('services.bundles.checkingBefore', { component })} />
                    </span>
                    <span className='relative h-1 w-10 overflow-hidden rounded-full bg-surface-3' aria-hidden='true'>
                        <span className='update-step-indeterminate absolute inset-y-0 left-0 w-1/3 rounded-full bg-primary' />
                    </span>
                </span>
            );
        }
        if (stopped && stopped.id === bundle.id && run) {
            // The page's own sequence stopped: how far it got, and Continue,
            // which installs only what is still missing. Why is said below.
            const done = bundle.components.filter((id) => installedSet.has(id)).length;
            return (
                <button key={key} type='button' className={actionChip} disabled={disabled || runActive} onClick={() => onComponents(bundle)} {...common} data-bundle-status='stopped' data-bundle-stop={run.stop}>
                    {label}
                    <Dot />
                    <span className='font-normal text-fg-muted'>{t('services.bundles.chip.progress', { n: done, total: bundle.components.length })}</span>
                    <Dot />
                    <span className='text-primary'>{t('services.bundles.continue')}</span>
                </button>
            );
        }
        switch (state) {
        case 'verified':
        case 'included':
            return (
                <button
                    key={key}
                    type='button'
                    aria-haspopup='dialog'
                    aria-label={state === 'included' && includedIn
                        ? t('services.bundles.chip.includedLabel', { name: name(bundle), other: name(includedIn) })
                        : t('services.bundles.chip.verifiedLabel', { name: name(bundle) })}
                    className={installedChip}
                    disabled={runActive}
                    onClick={() => setDetails(bundle.id)}
                    {...common}
                >
                    <Check className='h-3.5 w-3.5 shrink-0 text-success' aria-hidden='true' />
                    {label}
                    {state === 'included' && <><Dot /><span className='text-fg-muted'>{t('services.bundles.chip.included')}</span></>}
                </button>
            );
        case 'unverified':
            return (
                <button key={key} type='button' aria-haspopup='dialog' className={installedChip} disabled={runActive} onClick={() => setDetails(bundle.id)} {...common}>
                    <StatusDot tone='unknown' />
                    {label}
                    <Dot />
                    <span className='text-fg-muted'>{t('services.bundles.chip.notVerified')}</span>
                </button>
            );
        case 'available':
            return (
                <button key={key} type='button' aria-haspopup='dialog' className={actionChip} disabled={disabled || runActive} onClick={() => act(bundle)} {...common}>
                    {label}
                    <Dot />
                    <span className='text-primary'>{t(bundle.parent !== undefined ? 'services.bundles.chip.add' : 'services.bundles.chip.install')}</span>
                </button>
            );
        case 'partial':
            return (
                <button key={key} type='button' aria-haspopup='dialog' className={actionChip} disabled={disabled || runActive} onClick={() => act(bundle)} {...common}>
                    {label}
                    <Dot />
                    <span className='font-normal text-fg-muted'>{t('services.bundles.chip.progress', { n: bundle.components.length - bundle.missing_components.length, total: bundle.components.length })}</span>
                    <Dot />
                    <span className='text-primary'>{t('services.bundles.continue')}</span>
                </button>
            );
        case 'blocked':
            return (
                <button
                    key={key}
                    type='button'
                    className={`${quietChip} ${chipFocus} rounded-lg underline decoration-dotted decoration-1 underline-offset-[3px] hover:text-fg`}
                    title={blockedSentence(bundle)}
                    aria-expanded={reasonShown === bundle.id}
                    aria-controls={`bundle-reason-${bundle.id.replace(/\./g, '-')}`}
                    onClick={() => setReasonShown(reasonShown === bundle.id ? null : bundle.id)}
                    {...common}
                >
                    <span className='font-medium'>{label}</span>
                    <Dot />
                    <KeepWords text={blockedShort(bundle)} />
                </button>
            );
        default:
            return (
                <span key={key} className={quietChip} {...common}>
                    <span className='font-medium'>{label}</span>
                    <Dot />
                    {t('services.bundles.chip.unknown')}
                </span>
            );
        }
    };

    // The lines under the chips: the category's one notice, the host-wide
    // refusal said once, a stopped sequence's reason, and every known
    // failure or warning with its bundle's name (D-024).
    // Çiplerin altındaki satırlar: bölümün tek notu, sunucu geneli ret, duran
    // dizinin nedeni ve bilinen her hata ya da uyarı.
    const lines: ReactNode[] = [];
    if (!model.unknown) {
        if (notice) lines.push(<div key='notice' className='min-w-0 basis-full'>{notice}</div>);
        else if (model.hostBlocked) {
            const sample = bundles.find((bundle) => bundle.category === category && bundle.blocked_code === model.hostBlocked);
            if (sample) lines.push(<p key='host' role='status' className='min-w-0 basis-full text-sm text-fg-muted'>{blockedSentence(sample)}</p>);
        }
        const shown = reasonShown ? model.chips.find((chip) => chip.bundle.id === reasonShown && chip.state === 'blocked') : undefined;
        if (shown) {
            lines.push(
                <p key='blocked-reason' id={`bundle-reason-${shown.bundle.id.replace(/\./g, '-')}`} role='status' className='min-w-0 basis-full text-xs leading-5 text-fg-muted' data-bundle-blocked-reason={shown.bundle.id}>
                    <span className='font-medium text-fg'>{name(shown.bundle)}:</span>{' '}
                    <KeepWords text={blockedSentence(shown.bundle)} />
                </p>,
            );
        }
        if (stopped && run) {
            lines.push(
                <p key='stopped' role='status' className='min-w-0 basis-full text-xs leading-5 text-fg-muted' data-bundle-stopped-reason={run.stop}>
                    <span className='font-medium text-fg'>{name(stopped)}:</span>{' '}
                    {run.stop === 'gated'
                        ? t('services.bundles.stopped.gated', { reason: run.reason ?? '' })
                        : t(`services.bundles.stopped.${run.stop ?? 'failed'}` as TranslationKey, { component: nameOf(run.steps[run.index]) })}
                </p>,
            );
        }
        for (const bundle of model.noted) {
            const said = reason(bundle);
            if (!said) continue;
            lines.push(
                <p key={`noted-${bundle.id}`} className='min-w-0 basis-full text-xs leading-5' data-bundle-noted={bundle.id}>
                    <span className='font-medium text-fg'>{name(bundle)}:</span> {said}
                </p>,
            );
        }
    }

    const open = details ? model.chips.find((item) => item.bundle.id === details) : undefined;
    return (
        <div
            className='flex flex-wrap items-center gap-x-2 gap-y-2 border-t border-border bg-surface-subtle px-4 py-2.5'
            data-bundle-strip={category}
        >
            <span className='mr-1 text-[13px] font-semibold text-fg-muted'>{t('services.bundles.label')}</span>
            {model.unknown
                ? (scanning
                    ? <Checking label={t('services.bundles.checking')} className='text-sm' />
                    : <p className='min-w-0 flex-1 text-sm text-fg-muted' data-bundle-unknown=''>{t('services.bundles.unknown')}</p>)
                : model.chips.map(chip)}
            {lines}
            {open && (
                <BundleInstalledDialog
                    bundle={open.bundle}
                    includedIn={open.includedIn}
                    verified={open.state !== 'unverified'}
                    services={services}
                    reason={reasonText(open.bundle)}
                    repairDisabled={disabled || runActive}
                    verifyHeld={verifyHeld}
                    onClose={() => setDetails(null)}
                    onRepair={() => { setDetails(null); onMail(open.bundle); }}
                    onVerify={() => { if (verifyHeld !== null) return; setDetails(null); onVerify(open.bundle); }}
                />
            )}
        </div>
    );
}

// An installed bundle's dialogue: what it is made of, that it is installed
// and verified (or not yet verified, and why), and the one thing that can be
// done with it: Repair for a mail bundle (the profile install completes and
// proves a mail setup) or Verify for a component bundle (it reads the
// components again and changes nothing). This replaces the "· Repair" links
// that stood on the strip.
// Kurulu paketin penceresi: neden oluştuğu, kurulu ve doğrulandığı ve onunla
// yapılabilecek tek şey: posta paketinde Onar, bileşen paketinde Doğrula.
export function BundleInstalledDialog({
    bundle,
    includedIn,
    verified,
    services,
    reason,
    repairDisabled,
    verifyHeld,
    onClose,
    onRepair,
    onVerify,
}: {
    bundle: ReadyBundle;
    includedIn?: ReadyBundle;
    verified: boolean;
    services: readonly ComponentLite[];
    reason: string;
    repairDisabled: boolean;
    verifyHeld: string | null;
    onClose: () => void;
    onRepair: () => void;
    onVerify: () => void;
}) {
    const { t } = useI18n();
    const nameOf = (id: string) => componentName(services.find((service) => service.id === id) ?? id, t);
    const name = bundleName(bundle, t, nameOf);
    const installed = new Set(services.filter((service) => service.is_installed === true).map((service) => service.id));
    const mail = bundle.action_route === 'mail_profile';
    const state = includedIn
        ? t('services.bundles.installed.included', { name, other: bundleName(includedIn, t, nameOf) })
        : verified
            ? t('services.bundles.installed.verified', { name })
            : t('services.bundles.installed.unverified', { name });
    return (
        <Dialog
            id='bundle-installed'
            icon={Layers}
            title={name}
            description={bundleAbout(bundle, t) ? <KeepWords text={bundleAbout(bundle, t) ?? ''} /> : undefined}
            onDismiss={onClose}
            actions={
                <>
                    <Button variant='secondary' autoFocus onClick={onClose}>{t('common.close')}</Button>
                    {mail && !includedIn && (
                        <Button variant='secondary' icon={Wrench} disabled={repairDisabled} onClick={onRepair}>
                            {t('services.bundles.repairEllipsis')}
                        </Button>
                    )}
                    {!mail && (
                        <Button variant='secondary' icon={ScanSearch} disabled={verifyHeld !== null} title={verifyHeld ?? undefined} onClick={onVerify}>
                            {t('services.bundles.verify')}
                        </Button>
                    )}
                </>
            }
        >
            <p className='mb-3 flex items-start gap-2 text-sm text-fg' role='status' data-bundle-installed-state={includedIn ? 'included' : verified ? 'verified' : 'unverified'}>
                {verified || includedIn
                    ? <Check className='mt-0.5 h-4 w-4 shrink-0 text-success' aria-hidden='true' />
                    : <span className='mt-1.5'><StatusDot tone='unknown' /></span>}
                <span>{state}{reason && <span className='mt-0.5 block text-xs leading-5 text-fg-muted'>{reason}</span>}</span>
            </p>
            <ul className='mb-3 overflow-hidden rounded-lg border border-border'>
                {bundle.components.map((id, index) => {
                    const service = services.find((candidate) => candidate.id === id);
                    const Icon = componentIcon(id, service?.category ?? bundle.category);
                    const present = installed.has(id);
                    return (
                        <li key={id} className={`flex items-center gap-3 px-3 py-2 text-sm ${index > 0 ? 'border-t border-border' : ''}`} data-bundle-part={present ? 'installed' : 'notNeeded'}>
                            <Icon className='h-4 w-4 shrink-0 text-fg-muted' aria-hidden='true' />
                            <span className='text-fg'>{nameOf(id)}</span>
                            <span className='ml-auto text-xs text-fg-subtle'>
                                {present ? t('services.bundles.dialog.alreadyInstalled') : t('services.bundles.dialog.notNeeded')}
                            </span>
                        </li>
                    );
                })}
            </ul>
            <p className='text-sm leading-6 text-fg-muted'>
                {includedIn
                    ? t('services.bundles.installed.includedHow', { other: bundleName(includedIn, t, nameOf) })
                    : mail ? t('services.bundles.installed.repairHow') : t('services.bundles.installed.verifyHow')}
            </p>
            {!mail && verifyHeld && <p className='mt-2 text-sm leading-6 text-fg-muted' data-bundle-verify-held=''>{verifyHeld}</p>}
        </Dialog>
    );
}

// The confirmation of a database or web bundle: what it installs, what is
// already there, what is not needed, how it runs and how it resumes. Mail
// bundles keep their own dialogue.
// Bir veritabanı ya da web paketinin onayı: ne kurar, ne zaten var, ne
// gerekmiyor, nasıl sürer.
export function BundleInstallDialog({
    bundle,
    services,
    continuing = false,
    onCancel,
    onConfirm,
}: {
    bundle: ReadyBundle;
    services: readonly ComponentLite[];
    /** Opened from a sequence of this page that stopped. */
    continuing?: boolean;
    onCancel: () => void;
    onConfirm: () => void;
}) {
    const { t } = useI18n();
    const nameOf = (id: string) => componentName(services.find((service) => service.id === id) ?? id, t);
    const installed = new Set(services.filter((service) => service.is_installed === true).map((service) => service.id));
    const toInstall = bundle.missing_components.filter((id) => !installed.has(id));
    const title: TranslationKey = continuing ? 'services.bundles.dialog.titleContinue' : 'services.bundles.dialog.title';
    return (
        <Dialog
            id='bundle-install-confirm'
            icon={Layers}
            title={t(title, { name: bundleName(bundle, t, nameOf) })}
            description={<KeepWords text={bundleDescription(bundle, t, nameOf)} />}
            onDismiss={onCancel}
            actions={
                <>
                    <Button variant='secondary' autoFocus onClick={onCancel}>{t('common.cancel')}</Button>
                    <Button variant='primary' icon={DownloadCloud} disabled={toInstall.length === 0} onClick={onConfirm}>
                        {t(continuing ? 'services.bundles.dialog.confirmContinue' : 'services.bundles.dialog.confirm', { n: toInstall.length })}
                    </Button>
                </>
            }
        >
            <ol className='mb-3 overflow-hidden rounded-lg border border-border'>
                {bundle.components.map((id, index) => {
                    const service = services.find((candidate) => candidate.id === id);
                    const Icon = componentIcon(id, service?.category ?? bundle.category);
                    const state = toInstall.includes(id) ? 'install' : installed.has(id) ? 'installed' : 'notNeeded';
                    return (
                        <li key={id} className={`flex items-center gap-3 px-3 py-2 text-sm ${index > 0 ? 'border-t border-border' : ''}`} data-bundle-part={state}>
                            <Icon className={`h-4 w-4 shrink-0 ${state === 'install' ? 'text-fg-muted' : 'text-fg-subtle'}`} aria-hidden='true' />
                            <span className={state === 'install' ? 'font-medium text-fg' : 'text-fg-muted'}>{nameOf(id)}</span>
                            <span className='ml-auto text-xs text-fg-subtle'>
                                {state === 'install'
                                    ? t('services.bundles.dialog.willInstall')
                                    : state === 'installed'
                                        ? t('services.bundles.dialog.alreadyInstalled')
                                        : t('services.bundles.dialog.notNeeded')}
                            </span>
                        </li>
                    );
                })}
            </ol>
            <p className='text-sm leading-6 text-fg-muted'>{t('services.bundles.dialog.howItRuns')}</p>
        </Dialog>
    );
}
