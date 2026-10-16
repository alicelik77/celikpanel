// The ready bundles strip (alpha.94, D-034): one line at the head of a
// Components category, between its header and its rows. A bundle is a
// shortcut over the rows below it, never a second install address: a mail
// bundle opens the existing mail install dialogue, and a database or web
// bundle installs its missing components one by one through the same
// component install the rows use.
// Hazır paketler şeridi: bir Bileşenler bölümünün başında tek satır. Paket,
// altındaki satırlar için bir kısayoldur, ikinci bir kurulum adresi değildir.
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Check, DownloadCloud, Layers, Plus } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Checking, Dialog, KeepWords, StatusDot } from './ui';
import { componentName } from '../lib/componentName';
import { mailProfileAttemptError, mailProfileBlocked, mailProfileWarning } from '../lib/mailProfileText';
import {
    bundleAddLabel,
    bundleDescription,
    bundleInstalled,
    bundleName,
    bundleStripModel,
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

const linkClass = 'font-semibold text-primary underline decoration-1 underline-offset-[3px] hover:decoration-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary disabled:cursor-not-allowed disabled:text-fg-muted disabled:no-underline max-sm:inline-flex max-sm:min-h-11 max-sm:items-center';

export function ReadyBundleStrip({
    category,
    bundles,
    services,
    profiles,
    run,
    disabled,
    readDisabled,
    scanning,
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
    /** Verify of a database or web bundle only reads: it stays usable while
        changes are held, because reading again is what lifts the hold. */
    readDisabled: boolean;
    scanning: boolean;
    /** One line said for the whole category (mail DNS); installed bundles stay beside it. */
    notice?: ReactNode;
    onMail: (bundle: ReadyBundle) => void;
    onComponents: (bundle: ReadyBundle) => void;
    onVerify: (bundle: ReadyBundle) => void;
}) {
    const { t } = useI18n();
    const [open, setOpen] = useState<string | null>(null);
    const model = bundleStripModel(bundles, category);
    if (model.empty) return null;
    const nameOf = (id: string) => componentName(services.find((service) => service.id === id) ?? id, t);
    const name = (bundle: ReadyBundle) => bundleName(bundle, t, nameOf);
    const act = (bundle: ReadyBundle) => {
        setOpen(null);
        if (bundle.action === 'verify' && bundle.action_route === 'components') onVerify(bundle);
        else if (bundle.action_route === 'mail_profile') onMail(bundle);
        else onComponents(bundle);
    };
    const own = run ? bundles.find((bundle) => bundle.id === run.bundleId && bundle.category === category) : undefined;
    const running = own && run && run.phase !== 'stopped' ? own : undefined;
    const stopped = own && run && run.phase === 'stopped' && !bundleInstalled(own) ? own : undefined;
    const opened = open ? model.offers.find((offer) => offer.bundle.id === open) : undefined;
    const installedSet = new Set(services.filter((service) => service.is_installed === true).map((service) => service.id));

    // What a mail bundle says again (D-024): its last attempt that failed or
    // is running, else the Panel's typed warning. Only a webmail runtime that
    // is installed but not ready is drawn in Amber Ink.
    // Bir posta paketinin yeniden söylediği: başarısız ya da süren son deneme,
    // yoksa Panel'in türlü uyarısı.
    const reason = (bundle: ReadyBundle): ReactNode => {
        if (bundle.action_route !== 'mail_profile') return null;
        const profile = profiles.find((item) => item.id === bundle.profile_id);
        let text = '';
        if (bundle.latest_attempt_status === 'in_progress') text = t('services.bundles.attemptRunning');
        else if (bundle.latest_attempt_status === 'failed') text = (profile ? mailProfileAttemptError(profile, t) : '') || t('services.bundles.lastAttemptFailed');
        else if (bundle.warning_code) text = mailProfileWarning({ id: bundle.id, name: name(bundle), warning_code: bundle.warning_code, warning: profile?.warning }, t);
        if (!text) return null;
        return (
            <span className={`mt-0.5 block text-xs leading-5 ${bundle.warning_code === 'roundcube_not_ready' && !bundle.latest_attempt_status?.match(/failed|in_progress/) ? 'text-warning' : 'text-fg-muted'}`} data-bundle-reason={bundle.id}>
                {text}
            </span>
        );
    };

    const installedLines = model.installed.map((bundle) => (
        <p key={bundle.id} className='flex min-w-0 basis-full items-start gap-2 text-sm leading-6 text-fg sm:basis-auto sm:flex-1' data-bundle={bundle.id} data-bundle-status={bundle.status}>
            {bundle.status === 'installed_verified'
                ? <Check className='mt-1 h-4 w-4 shrink-0 text-success' aria-hidden='true' />
                : <span className='mt-2 shrink-0'><StatusDot tone='unknown' /></span>}
            <span className='min-w-0'>
                {bundle.status === 'installed_verified'
                    ? t('services.bundles.verified', { name: name(bundle) })
                    : t('services.bundles.unverified', { name: name(bundle) })}
                {bundle.status === 'installed_verified' && bundle.action === 'repair' && (
                    <>
                        <span className='mx-1.5 text-fg-subtle' aria-hidden='true'>·</span>
                        <button type='button' onClick={() => act(bundle)} disabled={disabled} className={linkClass}>
                            {t('services.bundles.repair')}
                        </button>
                    </>
                )}
                {reason(bundle)}
            </span>
            {bundle.status === 'installed_unverified' && bundle.action === 'verify' && (
                <Button variant='secondary' className='ml-auto shrink-0 max-sm:min-h-11' disabled={readDisabled} onClick={() => act(bundle)}>
                    {t('services.bundles.verify')}
                </Button>
            )}
            {bundle.status === 'installed_unverified' && bundle.action === 'repair' && (
                <Button variant='secondary' className='ml-auto shrink-0 max-sm:min-h-11' disabled={disabled} onClick={() => act(bundle)}>
                    {t('services.bundles.repair')}
                </Button>
            )}
        </p>
    ));

    const done = (bundle: ReadyBundle) => bundle.components.filter((id) => installedSet.has(id)).length;

    let body: ReactNode;
    if (running && run) {
        // D-033: the wait moves and says what it waits for.
        const component = run.steps[run.index];
        body = (
            <p role='status' aria-live='polite' className='flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1 text-sm text-fg' data-bundle-running={running.id}>
                <span className='font-medium'>{name(running)}</span>
                <span className='text-fg-muted'>
                    {t('services.bundles.step', { n: run.index + 1, total: run.steps.length, component: nameOf(component) })}
                </span>
                <span className='relative h-1 w-24 overflow-hidden rounded-full bg-surface-3' aria-hidden='true'>
                    <span className='update-step-indeterminate absolute inset-y-0 left-0 w-1/3 rounded-full bg-primary' />
                </span>
            </p>
        );
    } else if (notice || model.hostBlocked) {
        const sample = bundles.find((bundle) => bundle.category === category && bundle.blocked_code === model.hostBlocked);
        body = (
            <>
                {installedLines}
                {notice ?? (
                    <p role='status' className='min-w-0 basis-full text-sm text-fg-muted'>
                        {sample ? mailProfileBlocked({ id: sample.id, name: name(sample), blocked_code: sample.blocked_code, blocked_vars: sample.blocked_vars }, t) : ''}
                    </p>
                )}
            </>
        );
    } else if (model.unknown) {
        body = scanning
            ? <Checking label={t('services.bundles.checking')} className='text-sm' />
            : <p className='min-w-0 flex-1 text-sm text-fg-muted'>{t('services.bundles.unknown')}</p>;
    } else {
        const offers = model.offers.filter(({ bundle }) => bundle.id !== stopped?.id);
        body = (
            <>
                {installedLines}
                {stopped && run && (
                    // The page's own sequence stopped: how far it got, why, and
                    // Continue, which installs only what is still missing.
                    <p className='flex min-w-0 basis-full flex-wrap items-center gap-2 text-sm text-fg' data-bundle={stopped.id} data-bundle-status='stopped' data-bundle-stop={run.stop}>
                        <StatusDot tone='idle' />
                        <span className='min-w-0'>
                            {t('services.bundles.partial', { name: name(stopped), n: done(stopped), total: stopped.components.length })}
                            <span className='mt-0.5 block text-xs leading-5 text-fg-muted' role='status'>
                                {run.stop === 'gated'
                                    ? t('services.bundles.stopped.gated', { reason: run.reason ?? '' })
                                    : t(`services.bundles.stopped.${run.stop ?? 'failed'}` as TranslationKey, { component: nameOf(run.steps[run.index]) })}
                            </span>
                        </span>
                        <Button variant='secondary' className='ml-auto max-sm:min-h-11' disabled={disabled} onClick={() => onComponents(stopped)}>
                            {t('services.bundles.continue')}
                        </Button>
                    </p>
                )}
                {model.partial.map((bundle) => (
                    <p key={bundle.id} className='flex min-w-0 basis-full flex-wrap items-center gap-2 text-sm text-fg' data-bundle={bundle.id} data-bundle-status={bundle.status}>
                        <StatusDot tone='idle' />
                        <span className='min-w-0'>
                            {t('services.bundles.partial', { name: name(bundle), n: bundle.components.length - bundle.missing_components.length, total: bundle.components.length })}
                            {reason(bundle)}
                        </span>
                        <Button variant='secondary' className='ml-auto max-sm:min-h-11' disabled={disabled} onClick={() => act(bundle)}>
                            {t('services.bundles.continue')}
                        </Button>
                    </p>
                ))}
                {model.blocked.map((bundle) => (
                    <p key={bundle.id} className='min-w-0 basis-full text-sm text-fg-muted' data-bundle={bundle.id} data-bundle-status='blocked'>
                        <span className='font-medium text-fg-muted'>{name(bundle)}:</span>{' '}
                        {mailProfileBlocked({ id: bundle.id, name: name(bundle), blocked_code: bundle.blocked_code, blocked_vars: bundle.blocked_vars }, t)}
                        {reason(bundle)}
                    </p>
                ))}
                {offers.map(({ bundle, add }) => (
                    <button
                        key={bundle.id}
                        type='button'
                        aria-expanded={open === bundle.id}
                        aria-controls={`bundle-detail-${category}`}
                        onClick={() => setOpen(open === bundle.id ? null : bundle.id)}
                        className={`inline-flex min-h-[30px] items-center gap-1.5 rounded-lg border border-border-strong bg-surface px-3 py-1 text-[13px] font-semibold text-fg transition-colors hover:bg-surface-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary max-sm:min-h-11 aria-expanded:border-primary aria-expanded:bg-surface-2 aria-expanded:shadow-[inset_0_0_0_1px_rgb(var(--primary))] ${model.installed.length > 0 ? 'sm:ml-auto' : ''}`}
                        data-bundle={bundle.id}
                        data-bundle-status={bundle.status}
                    >
                        {add ? <Plus className='h-3.5 w-3.5 text-fg-subtle' aria-hidden='true' /> : <Layers className='h-3.5 w-3.5 text-fg-subtle' aria-hidden='true' />}
                        {add ? bundleAddLabel(bundle, t, nameOf) : name(bundle)}
                    </button>
                ))}
                {model.noted.filter((bundle) => bundle.id !== stopped?.id).map((bundle) => (
                    <p key={`noted-${bundle.id}`} className='min-w-0 basis-full text-sm text-fg' data-bundle-noted={bundle.id}>
                        <span className='font-medium'>{name(bundle)}:</span>
                        {reason(bundle)}
                    </p>
                ))}
                {/* The hint stands only while nothing is installed; beside an
                    installed bundle the line stays one line until an offer
                    is chosen (browser batch 19).
                    İpucu yalnız hiçbir şey kurulu değilken durur. */}
                {offers.length > 0 && (opened || model.installed.length === 0) && (
                    <div id={`bundle-detail-${category}`} aria-live='polite' className='flex min-h-[30px] basis-full flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-fg-subtle'>
                        {opened ? (
                            <>
                                <span className='max-w-[75ch] text-[13px] text-fg'>
                                    {/* A name such as PHP-FPM never breaks at its hyphen. */}
                                    <KeepWords text={bundleDescription(opened.bundle, t, nameOf)} />
                                    {kept(opened.bundle, installedSet).length > 0 && (
                                        <> <KeepWords text={t('services.bundles.skips', { components: kept(opened.bundle, installedSet).map(nameOf).join(', ') })} /></>
                                    )}
                                    {notNeeded(opened.bundle, installedSet).length > 0 && (
                                        <> <KeepWords text={t('services.bundles.notNeeded', { components: notNeeded(opened.bundle, installedSet).map(nameOf).join(', ') })} /></>
                                    )}
                                </span>
                                <span className='inline-flex gap-2'>
                                    <Button variant='secondary' icon={DownloadCloud} className='max-sm:min-h-11' disabled={disabled} onClick={() => act(opened.bundle)}>
                                        {opened.bundle.status === 'partial' ? t('services.bundles.continueEllipsis') : t('services.bundles.installEllipsis')}
                                    </Button>
                                    <button type='button' onClick={() => setOpen(null)} className='rounded-md px-1.5 py-1 text-xs font-semibold text-fg-muted hover:text-fg max-sm:min-h-11'>
                                        {t('common.cancel')}
                                    </button>
                                </span>
                            </>
                        ) : (
                            <span className='max-w-[75ch]'>{t('services.bundles.hint')}</span>
                        )}
                    </div>
                )}
            </>
        );
    }

    return (
        <div
            className='flex flex-wrap items-center gap-x-2.5 gap-y-2 border-t border-border bg-surface-subtle px-4 py-2.5'
            data-bundle-strip={category}
        >
            <span className='text-[13px] font-semibold text-fg-muted'>{t('services.bundles.label')}</span>
            {body}
        </div>
    );
}

// The parts of a bundle that are installed on this server, and the parts it
// does not install because their role is already filled (an add-on under
// Apache needs no Nginx). A part is never said to be installed when it is not.
// Paketin kurulu parçaları ve rolü zaten dolu olduğu için kurmayacağı parçalar.
const kept = (bundle: ReadyBundle, installed: ReadonlySet<string>) => bundle.components.filter((id) => installed.has(id));
const notNeeded = (bundle: ReadyBundle, installed: ReadonlySet<string>) => bundle.components.filter((id) => !installed.has(id) && !bundle.missing_components.includes(id));

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
            description={bundleDescription(bundle, t, nameOf)}
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
