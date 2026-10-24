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
import { Check, DownloadCloud, Layers, Wrench } from 'lucide-react';
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
import { COMPONENT_STATE_KEYS, componentStateWord } from '../lib/serviceStatus';
import { componentVersionText } from '../lib/packageVersion';
import type { InstallOperationRequest } from './ComponentOperation';
import { useRemote } from '../lib/remote';
import { decodeCandidateVersion, decodeRepoInfo } from '../lib/componentRepo';
import {
    DISTRIBUTION_PHP_TOOLS,
    decodeInstallPreview,
    installPreviewURL,
    phpPastSecuritySupport,
    shortCandidate,
    versionOfPackage,
    type InstallStep,
} from '../lib/installPreview';
import { InstallPreviewNotes, installPreviewGate } from './InstallPreviewNotes';

// A component as the rows have it: the installed bundle's dialogue says each
// part's state and version from these same fields (alpha.101).
// Satırlardaki bileşen; kurulu paket penceresi durumu ve sürümü aynı alanlardan söyler.
interface ComponentLite {
    id: string;
    name: string;
    category?: string;
    is_installed: boolean | null;
    kind?: string;
    status?: string;
    versions?: string[];
}
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
    /** alpha.105: the versions, the tool's PHP and the confirmations chosen in the dialogue. */
    choices?: BundleChoices;
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
        const choices = current.choices;
        const pkg = choices?.packages[componentId] ?? '';
        const started = await tracker.startInstall({
            serviceId: componentId,
            name: label(current.bundleId, componentId, index, current.steps.length),
            ...(pkg ? { package: pkg } : {}),
            ...(choices?.phpVersion && DISTRIBUTION_PHP_TOOLS.includes(componentId) ? { phpVersion: choices.phpVersion } : {}),
            // alpha.105: the server checks what each install brings against
            // what the owner confirmed, right before it runs.
            checkInstalls: true,
            confirmInstalls: choices?.confirmed ?? [],
        });
        if (!started) {
            stop(at, 'refused');
            return;
        }
        setRun((previous) => (previous && previous.bundleId === current.bundleId && previous.phase === 'starting' ? { ...previous, phase: 'running' } : previous));
    };

    const begin = (bundle: ReadyBundle, choices?: BundleChoices) => {
        if ((run !== null && run.phase !== 'stopped') || tracker.locked) return;
        const installed = installedNow();
        const steps = bundle.missing_components.filter((id) => !installed.has(id));
        if (steps.length === 0) return;
        sentRef.current = '';
        void send({ bundleId: bundle.id, steps, index: 0, phase: 'starting', ...(choices ? { choices } : {}) }, 0);
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
    notice,
    onMail,
    onComponents,
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
    /** One line said for the whole category (mail DNS); the chips stay beside it. */
    notice?: ReactNode;
    onMail: (bundle: ReadyBundle) => void;
    onComponents: (bundle: ReadyBundle) => void;
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
        // alpha.101: a bundle is blocked whole when one part is not offered
        // here (phpPgAdmin on Arch); the chip names that part plainly.
        // Bir parça burada sunulmuyorsa paketin tamamı engellidir; çip o parçayı adlandırır.
        if (bundle.blocked_code === 'component_not_installable') {
            return vars.component
                ? t('services.bundles.short.component_not_installable_named', { component: vars.component })
                : t('services.bundles.short.component_not_installable');
        }
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
            // Present: what the Panel did not call missing (role-aware: Nginx
            // where Apache serves is not missing) and what was installed since.
            // Var olan: Panel'in eksik demediği (rol bilinçli) ve sonradan kurulan.
            const done = bundle.components.filter((id) => !bundle.missing_components.includes(id) || installedSet.has(id)).length;
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
                    onClose={() => setDetails(null)}
                    onRepair={() => { setDetails(null); onMail(open.bundle); }}
                />
            )}
        </div>
    );
}

// An installed bundle's dialogue: what it is made of, each part with the
// state and version its row says (alpha.101, owner's screens on Frankfurt:
// "Already installed" beside every part said nothing), that it is installed
// and verified (or not yet verified, with the part that is not running
// visible), and the one thing that can be done with it: Repair for a mail
// bundle (the profile install completes and proves a mail setup). A component
// bundle has no button of its own: the page's Rescan reads the components
// again, and a stopped part is started from its row.
// Kurulu paketin penceresi: her parça satırının söylediği durum ve sürümle;
// posta paketinde Onar. Bileşen paketinin kendi düğmesi yoktur: sayfanın
// Yeniden tara'sı bileşenleri yeniden okur.
export function BundleInstalledDialog({
    bundle,
    includedIn,
    verified,
    services,
    reason,
    repairDisabled,
    onClose,
    onRepair,
}: {
    bundle: ReadyBundle;
    includedIn?: ReadyBundle;
    verified: boolean;
    services: readonly ComponentLite[];
    reason: string;
    repairDisabled: boolean;
    onClose: () => void;
    onRepair: () => void;
}) {
    const { t } = useI18n();
    const nameOf = (id: string) => componentName(services.find((service) => service.id === id) ?? id, t);
    const name = bundleName(bundle, t, nameOf);
    const mail = bundle.action_route === 'mail_profile';
    const state = includedIn
        ? t('services.bundles.installed.included', { name, other: bundleName(includedIn, t, nameOf) })
        : verified
            ? t('services.bundles.installed.verified', { name })
            : t('services.bundles.installed.unverified', { name });
    // What a part says: its row's own word and version ("Running · 1.26.3",
    // "Installed · 7.14.7", "Stopped"), or "Not needed" for a part whose role
    // another installed component fills (Nginx where Apache serves).
    // Bir parçanın söylediği: satırın kendi sözü ve sürümü ya da "Gerekmiyor".
    // The version is set apart as the row sets it: in mono, after a dot, and
    // it alone gives way (an ellipsis, the whole text in the tooltip).
    // Sürüm satırdaki gibi ayrılır; yalnız o kısalır, tamamı ipucundadır.
    const partText = (service: ComponentLite | undefined): { word: string; said: string; version?: string; mono?: boolean; title?: string } => {
        // A part never observed is "not checked yet", never "not needed".
        // Hiç gözlenmemiş parça "henüz bakılmadı"dır, "gerekmiyor" değil.
        if (!service || service.is_installed === null) return { word: 'notChecked', said: t('services.notChecked') };
        if (service.is_installed !== true) return { word: 'notNeeded', said: t('services.bundles.dialog.notNeeded') };
        const word = componentStateWord(service);
        const said = t(COMPONENT_STATE_KEYS[word] as TranslationKey);
        const version = componentVersionText(service);
        if (!version) return { word, said };
        if (version.kind === 'several') return { word, said, version: t('services.versionSeveral') };
        return { word, said, version: version.text, mono: true, title: version.kind === 'package' ? version.full : version.text };
    };
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
                </>
            }
        >
            <p className='mb-3 flex items-start gap-2 text-sm text-fg' role='status' data-bundle-installed-state={includedIn ? 'included' : verified ? 'verified' : 'unverified'}>
                {verified || includedIn
                    ? <Check className='mt-0.5 h-4 w-4 shrink-0 text-success' aria-hidden='true' />
                    : <span className='mt-1.5'><StatusDot tone='unknown' /></span>}
                <span>{state}{reason && <span className='mt-0.5 block text-xs leading-5 text-fg-muted'>{reason}</span>}</span>
            </p>
            <ul className='overflow-hidden rounded-lg border border-border'>
                {bundle.components.map((id, index) => {
                    const service = services.find((candidate) => candidate.id === id);
                    const Icon = componentIcon(id, service?.category ?? bundle.category);
                    const part = partText(service);
                    return (
                        <li
                            key={id}
                            className={`flex items-center gap-3 px-3 py-2 text-sm ${index > 0 ? 'border-t border-border' : ''}`}
                            data-bundle-part={part.word === 'notNeeded' || part.word === 'notChecked' ? part.word : 'installed'}
                            data-bundle-part-state={part.word}
                        >
                            <Icon className='h-4 w-4 shrink-0 text-fg-muted' aria-hidden='true' />
                            <span className='shrink-0 text-fg'>{nameOf(id)}</span>
                            <span
                                className={`ml-auto flex min-w-0 items-center justify-end gap-1.5 text-xs ${part.word === 'notNeeded' ? 'text-fg-subtle' : 'text-fg-muted'}`}
                                title={part.title}
                            >
                                {/* Only the state word carries the red of a stopped
                                    part; the version stays muted (DESIGN.md: colour
                                    carries a state, with its word).
                                    Durdu kırmızısını yalnız durum sözcüğü taşır. */}
                                <span className={`shrink-0 ${part.word === 'stopped' ? 'font-medium text-danger' : ''}`} data-bundle-part-word=''>{part.said}</span>
                                {part.version !== undefined && (
                                    <>
                                        {' '}<span className='shrink-0 text-fg-subtle' aria-hidden='true'>·</span>{' '}
                                        <span className={`min-w-0 truncate text-fg-muted ${part.mono ? 'font-mono' : ''}`} data-bundle-part-version=''>{part.version}</span>
                                    </>
                                )}
                            </span>
                        </li>
                    );
                })}
            </ul>
            {(includedIn || mail || !verified) && (
                <p className='mt-3 text-sm leading-6 text-fg-muted' data-bundle-installed-how=''>
                    {includedIn
                        ? t('services.bundles.installed.includedHow', { other: bundleName(includedIn, t, nameOf) })
                        : mail
                            ? t('services.bundles.installed.repairHow')
                            : t('services.bundles.installed.unverifiedHow', { rescan: t('services.rescan') })}
                </p>
            )}
        </Dialog>
    );
}

// The confirmation of a database or web bundle: what it installs, what is
// already there, what is not needed, how it runs and how it resumes. Mail
// bundles keep their own dialogue.
// alpha.105 (D-034 amendment, owner's words: recommend, warn, never impose):
// each component it installs says the version that will come, marked when it
// is the distribution's own (recommended), with Change listing the versions
// that component's own install offers (its managed repository); a part
// already installed says its version and is not changed. The package
// manager's simulation of the whole bundle says what else would be
// installed, and notable packages need the owner's confirmation; a chosen
// PHP version that cannot serve its tool is refused, other combinations are
// a plain warning.
// Bir veritabanı ya da web paketinin onayı; her bileşen gelecek sürümü
// söyler, Değiştir onun kendi sürüm listesini sunar.
export interface BundleChoices {
    /** A version package per component ('' or absent: the distribution's). */
    packages: Record<string, string>;
    /** The PHP version the bundle's PHP tool runs on, when one was chosen. */
    phpVersion?: string;
    /** The notable packages the owner confirmed. */
    confirmed: string[];
}

interface VersionOption { pkg: string; version: string; recommended: boolean }

// One part the bundle installs: its version and, when its own install offers
// more than one, Change.
// Paketin kurduğu bir parça: sürümü ve birden çoksa Değiştir.
function BundleVersionPart({
    id,
    chosen,
    open,
    onToggle,
    onChoose,
    name,
}: {
    id: string;
    chosen: string;
    open: boolean;
    onToggle: () => void;
    onChoose: (pkg: string) => void;
    name: string;
}) {
    const { t } = useI18n();
    const candidate = useRemote(`/api/v1/service/candidate?id=${encodeURIComponent(id)}`, decodeCandidateVersion);
    const repoRead = useRemote(`/api/v1/repo?service_id=${encodeURIComponent(id)}`, decodeRepoInfo);
    const repo = repoRead.remote.state === 'known' && repoRead.remote.value.available && repoRead.remote.value.enabled ? repoRead.remote.value : null;
    const defaultVersion = candidate.remote.state === 'known' ? shortCandidate(candidate.remote.value) : '';
    const options: VersionOption[] = [
        { pkg: '', version: defaultVersion, recommended: true },
        ...(repo?.packages ?? []).map((pkg) => ({ pkg, version: versionOfPackage(pkg), recommended: false })),
    ];
    const repoLabel = repo?.name || t('services.bundles.version.vendorRepo');
    const said = chosen
        ? t('services.bundles.version.vendor', { version: versionOfPackage(chosen), repo: repoLabel })
        : candidate.remote.state === 'loading'
            ? '…'
            : defaultVersion
                ? t('services.bundles.version.recommended', { version: defaultVersion })
                : t('services.bundles.version.recommendedUnknown');
    const legend = `bundle-version-${id}`;
    return (
        <>
            <span className='ml-auto flex min-w-0 flex-wrap items-center justify-end gap-x-2 gap-y-0.5 text-right text-xs text-fg-muted'>
                <span data-bundle-version={chosen || 'default'}><KeepWords text={said} /></span>
                {options.length > 1 && (
                    <button
                        type='button'
                        className='rounded text-primary underline decoration-dotted underline-offset-2 hover:text-fg focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary max-sm:min-h-11'
                        aria-expanded={open}
                        aria-controls={legend}
                        onClick={onToggle}
                        data-bundle-version-change={id}
                    >
                        {t('services.bundles.version.change')}
                    </button>
                )}
            </span>
            {open && options.length > 1 && (
                <fieldset id={legend} className='basis-full pl-7 pt-1.5' data-bundle-version-options={id}>
                    <legend className='sr-only'>{t('services.bundles.version.legend', { name })}</legend>
                    <div className='flex flex-col gap-1'>
                        {options.map((option) => (
                            <label key={option.pkg || 'default'} className='flex cursor-pointer items-center gap-2 text-sm text-fg max-sm:min-h-11'>
                                <input
                                    type='radio'
                                    name={legend}
                                    checked={chosen === option.pkg}
                                    onChange={() => onChoose(option.pkg)}
                                    className='h-4 w-4 accent-primary'
                                    data-bundle-version-option={option.pkg || 'default'}
                                />
                                <span>
                                    {option.recommended
                                        ? (option.version
                                            ? t('services.bundles.version.optionDefault', { version: option.version })
                                            : t('services.bundles.version.optionDefaultUnknown'))
                                        : t('services.bundles.version.optionVendor', { version: option.version, repo: repoLabel })}
                                </span>
                            </label>
                        ))}
                    </div>
                </fieldset>
            )}
        </>
    );
}

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
    onConfirm: (choices: BundleChoices) => void;
}) {
    const { t } = useI18n();
    const nameOf = (id: string) => componentName(services.find((service) => service.id === id) ?? id, t);
    const installed = new Set(services.filter((service) => service.is_installed === true).map((service) => service.id));
    const toInstall = bundle.missing_components.filter((id) => !installed.has(id));
    const [packages, setPackages] = useState<Record<string, string>>({});
    const [openPart, setOpenPart] = useState<string | null>(null);
    const [acknowledgedFor, setAcknowledgedFor] = useState<string | null>(null);
    // The PHP version the bundle's tool will run on: the one chosen for
    // PHP-FPM when the bundle installs it; otherwise the tool takes the
    // distribution's default (alpha.104), changed later in its Manage window.
    const phpChosen = toInstall.includes('php-fpm') && packages['php-fpm'] ? versionOfPackage(packages['php-fpm']) : '';
    const tools = toInstall.filter((id) => DISTRIBUTION_PHP_TOOLS.includes(id));
    const steps: InstallStep[] = toInstall.map((id) => ({
        id,
        ...(packages[id] ? { package: packages[id] } : {}),
        ...(phpChosen && DISTRIBUTION_PHP_TOOLS.includes(id) ? { phpVersion: phpChosen } : {}),
    }));
    const previewURL = installPreviewURL(steps);
    const preview = useRemote(previewURL, decodeInstallPreview);
    const acknowledged = previewURL !== null && acknowledgedFor === previewURL;
    const gate = installPreviewGate(previewURL ? preview : null, acknowledged);
    const previewValue = preview.remote.state === 'known' ? preview.remote.value : null;
    const refusals = previewURL ? previewValue?.refusals ?? [] : [];
    // The distribution's PHP, to warn when a tool built for it is put on
    // another one (the same read the PHP-FPM part makes).
    const phpDefault = useRemote(phpChosen ? '/api/v1/service/candidate?id=php-fpm' : null, decodeCandidateVersion);
    const phpDefaultVersion = phpChosen && phpDefault.remote.state === 'known' ? shortCandidate(phpDefault.remote.value) : '';
    const warnings: string[] = [];
    if (phpChosen) {
        for (const tool of tools) {
            if (phpDefaultVersion && phpDefaultVersion !== phpChosen) {
                warnings.push(t('services.bundles.warn.distroTool', { tool: nameOf(tool), default: phpDefaultVersion, version: phpChosen }));
            }
        }
        if (phpPastSecuritySupport(phpChosen)) warnings.push(t('services.bundles.warn.eol', { version: phpChosen }));
    }
    const title: TranslationKey = continuing ? 'services.bundles.dialog.titleContinue' : 'services.bundles.dialog.title';
    const choose = (id: string, pkg: string) => setPackages((previous) => ({ ...previous, [id]: pkg }));
    const avoidable = Object.values(packages).some(Boolean);
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
                    <Button
                        variant='primary'
                        icon={DownloadCloud}
                        disabled={toInstall.length === 0 || gate.hold || refusals.length > 0}
                        onClick={() => onConfirm({
                            packages: { ...packages },
                            ...(phpChosen && tools.length > 0 ? { phpVersion: phpChosen } : {}),
                            confirmed: gate.confirmed,
                        })}
                    >
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
                    const version = state === 'installed' ? componentVersionText(service) : null;
                    const installedVersion = version ? (version.kind === 'several' ? t('services.versionSeveral') : version.text) : '';
                    return (
                        <li key={id} className={`flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm ${index > 0 ? 'border-t border-border' : ''}`} data-bundle-part={state}>
                            <Icon className={`h-4 w-4 shrink-0 ${state === 'install' ? 'text-fg-muted' : 'text-fg-subtle'}`} aria-hidden='true' />
                            <span className={state === 'install' ? 'font-medium text-fg' : 'text-fg-muted'}>
                                {nameOf(id)}
                                {state === 'install' && phpChosen && DISTRIBUTION_PHP_TOOLS.includes(id) && (
                                    <span className='block text-xs font-normal text-fg-muted' data-bundle-tool-php={phpChosen}>{t('services.bundles.version.runsOn', { version: phpChosen })}</span>
                                )}
                            </span>
                            {state === 'install' ? (
                                <BundleVersionPart
                                    id={id}
                                    name={nameOf(id)}
                                    chosen={packages[id] ?? ''}
                                    open={openPart === id}
                                    onToggle={() => setOpenPart(openPart === id ? null : id)}
                                    onChoose={(pkg) => choose(id, pkg)}
                                />
                            ) : (
                                <span className='ml-auto text-xs text-fg-subtle' data-bundle-installed-version={installedVersion || undefined}>
                                    {state === 'installed'
                                        ? (installedVersion
                                            ? t('services.bundles.version.installed', { version: installedVersion })
                                            : t('services.bundles.dialog.alreadyInstalled'))
                                        : t('services.bundles.dialog.notNeeded')}
                                </span>
                            )}
                        </li>
                    );
                })}
            </ol>
            {refusals.map((refusal) => (
                <p key={`${refusal.id}:${refusal.code}`} role='alert' className='mb-2 text-sm leading-6 text-danger' data-bundle-refusal={refusal.code}>
                    <KeepWords text={t(`services.bundles.refusal.${refusal.code === 'php_extensions_unavailable' || refusal.code === 'php_version_unsupported' ? refusal.code : 'php_version_unavailable'}` as TranslationKey, {
                        tool: nameOf(refusal.id),
                        version: refusal.version ?? '',
                        packages: refusal.packages.join(', '),
                    })} />
                </p>
            ))}
            {warnings.map((warning) => (
                <p key={warning} className='mb-2 rounded-lg border border-warning-mark/50 bg-warning-mark/20 px-3 py-2 text-sm leading-6 text-fg' data-bundle-warning=''>
                    <KeepWords text={warning} />
                </p>
            ))}
            {previewURL && (
                <InstallPreviewNotes
                    handle={preview}
                    acknowledged={acknowledged}
                    onAcknowledge={(value) => setAcknowledgedFor(value ? previewURL : null)}
                    componentLabel={nameOf}
                    avoidable={avoidable}
                />
            )}
            <p className='mt-3 text-sm leading-6 text-fg-muted'>{t('services.bundles.dialog.howItRuns')}</p>
        </Dialog>
    );
}
