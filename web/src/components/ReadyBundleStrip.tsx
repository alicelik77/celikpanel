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
import { mailProfileBlocked, mailProfileWarning } from '../lib/mailProfileText';
import {
    bundleAddLabel,
    bundleDescription,
    bundleName,
    bundleStripModel,
    type ReadyBundle,
} from '../lib/readyBundles';
import { componentIcon } from '../lib/componentIcons';
import type { InstallOperationRequest } from './ComponentOperation';

interface ComponentLite { id: string; name: string; category?: string; is_installed: boolean | null }

// A running bundle install: the steps (missing components, in the Panel's
// order) and which one is being installed. It lives in this page's memory
// only, so a reload never continues it by itself: the strip then reads
// "partial" and the owner presses Continue (owner decision, 2026-10-17).
// Süren bir paket kurulumu; yalnız sayfanın belleğinde yaşar, yeniden
// yükleme onu kendiliğinden sürdürmez.
export interface BundleRun {
    bundleId: string;
    steps: string[];
    index: number;
    phase: 'starting' | 'running';
}

interface OperationView {
    operation: unknown;
    failure: unknown;
    catalogSnapshot: unknown;
    locked: boolean;
    startInstall: (request: InstallOperationRequest) => Promise<boolean>;
}

// useBundleSequence installs a bundle's missing components in order, one
// component install per step. The next install is sent only after the
// operation tracker has ended the previous one with a fresh scan (one active
// operation per host); the first failure stops the sequence and leaves the
// tracker's own guidance for that component on the screen. Each step is sent
// once: a re-render or a poll never sends it again.
// Eksik bileşenleri sırayla kurar; bir sonraki kurulum ancak öncekini izleyici
// taze taramayla bitirdikten sonra gönderilir. İlk hata diziyi durdurur.
export function useBundleSequence(
    tracker: OperationView,
    services: readonly ComponentLite[],
    label: (bundleId: string, componentId: string, index: number, total: number) => string,
) {
    const [run, setRun] = useState<BundleRun | null>(null);
    const sentRef = useRef('');
    const baselineRef = useRef<unknown>(null);
    const servicesRef = useRef(services);
    servicesRef.current = services;
    const snapshotRef = useRef(tracker.catalogSnapshot);
    snapshotRef.current = tracker.catalogSnapshot;

    const send = async (current: BundleRun, index: number) => {
        const key = `${current.bundleId}:${current.steps.join(',')}:${index}`;
        if (sentRef.current === key) return;
        sentRef.current = key;
        baselineRef.current = snapshotRef.current;
        const componentId = current.steps[index];
        setRun({ ...current, index, phase: 'starting' });
        const started = await tracker.startInstall({
            serviceId: componentId,
            name: label(current.bundleId, componentId, index, current.steps.length),
        });
        if (!started) {
            // Refused or not started: the tracker says why. Nothing more is sent.
            setRun(null);
            return;
        }
        setRun((previous) => (previous && previous.bundleId === current.bundleId ? { ...previous, phase: 'running' } : previous));
    };

    const begin = (bundle: ReadyBundle) => {
        if (run !== null || tracker.locked) return;
        const steps = bundle.missing_components.filter((id) => {
            const service = servicesRef.current.find((candidate) => candidate.id === id);
            return service?.is_installed !== true;
        });
        if (steps.length === 0) return;
        const next: BundleRun = { bundleId: bundle.id, steps, index: 0, phase: 'starting' };
        sentRef.current = '';
        void send(next, 0);
    };

    useEffect(() => {
        if (!run || run.phase !== 'running') return;
        if (tracker.operation !== null || tracker.locked) return;
        if (tracker.failure) {
            setRun(null);
            return;
        }
        // The step ended without a fresh scan from the tracker: nothing is
        // confirmed, so nothing more is sent.
        if (tracker.catalogSnapshot === baselineRef.current) return;
        let index = run.index + 1;
        while (index < run.steps.length && servicesRef.current.find((service) => service.id === run.steps[index])?.is_installed === true) {
            index += 1;
        }
        if (index >= run.steps.length) {
            setRun(null);
            return;
        }
        void send(run, index);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [run, tracker.operation, tracker.failure, tracker.catalogSnapshot, tracker.locked]);

    return { run, begin };
}

export function ReadyBundleStrip({
    category,
    bundles,
    services,
    run,
    disabled,
    scanning,
    notice,
    onMail,
    onComponents,
    onVerify,
}: {
    category: string;
    bundles: readonly ReadyBundle[];
    services: readonly ComponentLite[];
    run: BundleRun | null;
    disabled: boolean;
    scanning: boolean;
    /** One line said for the whole category instead of the bundles (mail DNS). */
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
    const running = run ? bundles.find((bundle) => bundle.id === run.bundleId && bundle.category === category) : undefined;
    const opened = open ? model.offers.find((offer) => offer.bundle.id === open) : undefined;

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
    } else if (notice) {
        body = notice;
    } else if (model.hostBlocked) {
        const sample = bundles.find((bundle) => bundle.category === category && bundle.blocked_code === model.hostBlocked);
        body = <p className='min-w-0 flex-1 text-sm text-fg-muted'>{sample ? mailProfileBlocked({ id: sample.id, name: name(sample), blocked_code: sample.blocked_code, blocked_vars: sample.blocked_vars }, t) : ''}</p>;
    } else if (model.unknown) {
        body = scanning
            ? <Checking label={t('services.bundles.checking')} className='text-sm' />
            : <p className='min-w-0 flex-1 text-sm text-fg-muted'>{t('services.bundles.unknown')}</p>;
    } else {
        body = (
            <>
                {model.installed.map((bundle) => (
                    <p key={bundle.id} className='flex min-w-0 basis-full items-start gap-2 text-sm leading-6 text-fg sm:basis-auto sm:flex-1' data-bundle={bundle.id} data-bundle-status={bundle.status}>
                        {bundle.status === 'installed_verified'
                            ? <Check className='mt-1 h-4 w-4 shrink-0 text-success' aria-hidden='true' />
                            : <span className='mt-2 shrink-0'><StatusDot tone='unknown' /></span>}
                        <span className='min-w-0'>
                            {bundle.status === 'installed_verified'
                                ? t('services.bundles.verified', { name: name(bundle) })
                                : t('services.bundles.unverified', { name: name(bundle) })}
                            {bundle.warning_code === 'roundcube_not_ready' && (
                                <span className='ml-1 text-warning'>{mailProfileWarning({ id: bundle.id, name: name(bundle), warning_code: bundle.warning_code }, t)}</span>
                            )}
                            {bundle.latest_attempt_status === 'failed' && (
                                <span className='ml-1 text-fg-muted'>{t('services.bundles.lastAttemptFailed')}</span>
                            )}
                            {bundle.action === 'repair' && (
                                <>
                                    <span className='mx-1.5 text-fg-subtle' aria-hidden='true'>·</span>
                                    <button
                                        type='button'
                                        onClick={() => act(bundle)}
                                        disabled={disabled}
                                        className='font-semibold text-primary underline decoration-1 underline-offset-[3px] hover:decoration-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary disabled:cursor-not-allowed disabled:text-fg-muted disabled:no-underline max-sm:inline-flex max-sm:min-h-11 max-sm:items-center'
                                    >
                                        {t('services.bundles.repair')}
                                    </button>
                                </>
                            )}
                        </span>
                        {bundle.action === 'verify' && (
                            <Button variant='secondary' className='ml-auto shrink-0 max-sm:min-h-11' disabled={disabled} onClick={() => act(bundle)}>
                                {t('services.bundles.verify')}
                            </Button>
                        )}
                    </p>
                ))}
                {model.partial.map((bundle) => (
                    <p key={bundle.id} className='flex min-w-0 basis-full flex-wrap items-center gap-2 text-sm text-fg' data-bundle={bundle.id} data-bundle-status={bundle.status}>
                        <StatusDot tone='idle' />
                        <span className='min-w-0'>{t('services.bundles.partial', {
                            name: name(bundle),
                            n: bundle.components.length - bundle.missing_components.length,
                            total: bundle.components.length,
                        })}</span>
                        <Button variant='secondary' className='ml-auto max-sm:min-h-11' disabled={disabled} onClick={() => act(bundle)}>
                            {t('services.bundles.continue')}
                        </Button>
                    </p>
                ))}
                {model.blocked.map((bundle) => (
                    <p key={bundle.id} className='min-w-0 basis-full text-sm text-fg-muted' data-bundle={bundle.id} data-bundle-status='blocked'>
                        <span className='font-medium text-fg-muted'>{name(bundle)}:</span>{' '}
                        {mailProfileBlocked({ id: bundle.id, name: name(bundle), blocked_code: bundle.blocked_code, blocked_vars: bundle.blocked_vars }, t)}
                    </p>
                ))}
                {model.offers.map(({ bundle, add }) => (
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
                {/* The hint stands only while nothing is installed; beside an
                    installed bundle the line stays one line until an offer
                    is chosen (browser batch 19).
                    İpucu yalnız hiçbir şey kurulu değilken durur. */}
                {model.offers.length > 0 && (opened || model.installed.length === 0) && (
                    <div id={`bundle-detail-${category}`} aria-live='polite' className='flex min-h-[30px] basis-full flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-fg-subtle'>
                        {opened ? (
                            <>
                                <span className='max-w-[75ch] text-[13px] text-fg'>
                                    {/* A name such as PHP-FPM never breaks at its hyphen. */}
                                    <KeepWords text={bundleDescription(opened.bundle, t, nameOf)} />
                                    {opened.bundle.missing_components.length < opened.bundle.components.length && (
                                        <> <KeepWords text={t('services.bundles.skips', {
                                            components: opened.bundle.components.filter((id) => !opened.bundle.missing_components.includes(id)).map(nameOf).join(', '),
                                        })} /></>
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

// The confirmation of a database or web bundle: what it installs, what it
// skips, how it runs and how it resumes. Mail bundles keep their own dialogue.
// Bir veritabanı ya da web paketinin onayı: ne kurar, neyi atlar, nasıl sürer.
export function BundleInstallDialog({
    bundle,
    services,
    onCancel,
    onConfirm,
}: {
    bundle: ReadyBundle;
    services: readonly ComponentLite[];
    onCancel: () => void;
    onConfirm: () => void;
}) {
    const { t } = useI18n();
    const nameOf = (id: string) => componentName(services.find((service) => service.id === id) ?? id, t);
    const continuing = bundle.status === 'partial';
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
                    <Button variant='primary' icon={DownloadCloud} onClick={onConfirm}>
                        {t(continuing ? 'services.bundles.dialog.confirmContinue' : 'services.bundles.dialog.confirm', { n: bundle.missing_components.length })}
                    </Button>
                </>
            }
        >
            <ol className='mb-3 overflow-hidden rounded-lg border border-border'>
                {bundle.components.map((id, index) => {
                    const service = services.find((candidate) => candidate.id === id);
                    const Icon = componentIcon(id, service?.category ?? bundle.category);
                    const missing = bundle.missing_components.includes(id);
                    return (
                        <li key={id} className={`flex items-center gap-3 px-3 py-2 text-sm ${index > 0 ? 'border-t border-border' : ''}`}>
                            <Icon className={`h-4 w-4 shrink-0 ${missing ? 'text-fg-muted' : 'text-fg-subtle'}`} aria-hidden='true' />
                            <span className={missing ? 'font-medium text-fg' : 'text-fg-muted'}>{nameOf(id)}</span>
                            <span className='ml-auto text-xs text-fg-subtle'>
                                {missing ? t('services.bundles.dialog.willInstall') : t('services.bundles.dialog.alreadyInstalled')}
                            </span>
                        </li>
                    );
                })}
            </ol>
            <p className='text-sm leading-6 text-fg-muted'>{t('services.bundles.dialog.howItRuns')}</p>
        </Dialog>
    );
}
