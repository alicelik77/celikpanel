// The repository card (v0.1.0-alpha.107, D-038 §12 and §1/§7): an enabled
// vendor repository on its component's page says first whether it is
// confined to that component (it adds versions the distribution does not
// ship and never replaces a distribution package), then who enabled it and
// when, its signing key and expiry, what apt last downloaded from it and
// apt's own errors about it, and the distribution's moving "default version"
// packages installed beside it with the owner's optional command. A read
// only, except Confine…, the owner's own action: it shows the plan first
// (RepoConfineDialog) and nothing confines a repository by itself.
//
// Depo kartı: etkin üretici deposunun kendi bileşeniyle sınırlı olup
// olmadığı, kim ne zaman açtı, imza anahtarı ve süresi, apt'ın son indirdiği
// ve hataları, "varsayılan sürüm" paketleri. Salt okunur; yalnız sınırlanmamış
// depo için Sınırla.
import { useState } from 'react';
import { Package } from 'lucide-react';
import { LifeLine, lifeDate, type Tone } from './LifeLine';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, KeepWords, RemoteGate } from './ui';
import { useRemote } from '../lib/remote';
import { decodeRepoInfo, type RepoInfo } from '../lib/componentRepo';
import { componentName } from '../lib/componentName';
import { RepoConfineDialog } from './RepoConfineDialog';
import { VendorLaneDialog } from './VendorLaneDialog';
import { showToast } from './Toast';
import { decodeLaneRead, laneURL, missingSteps } from '../lib/vendorLane';

// A time the server sent, said as a date (the shared lifecycle reading).
export const repoDate = (value: string | undefined, locale: string): string => lifeDate(value, locale);

// "15058500A0235D97F5D1…" → "1505 8500 A023 …", as fingerprints are read aloud.
export function groupedFingerprint(fingerprint: string): string {
    return (fingerprint.toUpperCase().match(/.{1,4}/g) ?? []).join(' ');
}

const Line = LifeLine;

export function RepoCard({ serviceId }: { serviceId: string }) {
    const url = `/api/v1/repo?service_id=${encodeURIComponent(serviceId)}`;
    const handle = useRemote(url, decodeRepoInfo);
    const { t } = useI18n();
    const remote = handle.remote;
    // Nothing is drawn for a component without a repository. A vendor lane
    // that is not enabled says the default lane and offers the informed
    // choice (alpha.108); another repository is offered by the install
    // dialogue.
    if (remote.state === 'known' && remote.value.available && !remote.value.enabled && remote.value.lane) {
        return <DefaultLaneCard serviceId={serviceId} info={remote.value} onChanged={() => void handle.retry()} />;
    }
    if (remote.state === 'known' && (!remote.value.available || !remote.value.enabled)) return null;
    if (remote.state === 'loading') return null;
    return (
        <section className="rounded-2xl border border-border bg-surface" data-repo-card={serviceId}>
            <header className="flex items-center gap-2 border-b border-border px-5 py-3">
                <Package className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                <h2 className="text-sm font-semibold text-fg">
                    {t('life.repo.title', { name: remote.state === 'known' ? remote.value.name ?? '' : serviceId })}
                </h2>
            </header>
            <div className="p-5">
                <RemoteGate remote={remote} checking={t('life.repo.checking')} failed={t('life.repo.unknown')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <RepoFacts serviceId={serviceId} info={value} onChanged={() => void handle.retry()} />}
                </RemoteGate>
            </div>
        </section>
    );
}

// The default lane (alpha.108, D-038 amendment "two lanes"): this component
// uses the operating system's packages only; the vendor repository is the
// administrator's informed choice, enabled with its confinement and automatic
// updates. What a disabled repository left installed is said as no longer
// updated.
// Varsayılan yol: işletim sisteminin paketleri; dış depo bilinçli seçim.
function DefaultLaneCard({ serviceId, info, onChanged }: { serviceId: string; info: RepoInfo; onChanged: () => void }) {
    const { t, locale } = useI18n();
    const [open, setOpen] = useState(false);
    const component = componentName({ id: serviceId, name: serviceId }, t);
    const left = info.no_longer_updated ?? [];
    return (
        <section className="rounded-2xl border border-border bg-surface" data-repo-card={serviceId} data-lane-default="">
            <header className="flex items-center gap-2 border-b border-border px-5 py-3">
                <Package className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                <h2 className="text-sm font-semibold text-fg">{t('lane.default.title', { component })}</h2>
            </header>
            <div className="space-y-3 p-5">
                <Line tone="ok" attr="lane-default"><KeepWords text={t('lane.default.body', { component, name: info.name ?? '' })} /></Line>
                {left.length > 0 && (
                    <Line tone="warn" attr="lane-no-longer-updated">
                        <KeepWords text={t('lane.default.left', { names: left.join(', '), name: info.name ?? '', date: repoDate(info.disabled_at, locale) })} />
                    </Line>
                )}
                <div className="pl-6">
                    <Button onClick={() => setOpen(true)} className="max-sm:min-h-11" data-lane-enable="">
                        {t('lane.default.enable', { name: info.name ?? '' })}
                    </Button>
                </div>
            </div>
            {open && (
                <VendorLaneDialog
                    serviceId={serviceId}
                    component={component}
                    onClose={() => setOpen(false)}
                    onDone={() => {
                        setOpen(false);
                        onChanged();
                    }}
                />
            )}
        </section>
    );
}

// apt's own words for a key it refuses as expired (gpgv's EXPKEYSIG, or an
// "expired" line naming the repository): only then does the card say apt no
// longer accepts the repository (alpha.108, owner finding).
export const aptReportsExpiredKey = (lines: string[] | undefined): boolean =>
    (lines ?? []).some((line) => /EXPKEYSIG|KEYEXPIRED|expired/i.test(line));

function RepoFacts({ serviceId, info, onChanged }: { serviceId: string; info: RepoInfo; onChanged: () => void }) {
    const { t, locale } = useI18n();
    const [asking, setAsking] = useState(false);
    // alpha.108: a vendor lane's missing steps, completed as one confirmed
    // operation (repository, confinement, automatic updates).
    const lane = useRemote(info.lane ? laneURL(serviceId) : null, decodeLaneRead);
    const lanePlan = lane.remote.state === 'known' && lane.remote.value.state === 'read' ? lane.remote.value.plan : null;
    const missing = lanePlan ? missingSteps(lanePlan) : [];
    const [laneOpen, setLaneOpen] = useState(false);
    const [keyAsk, setKeyAsk] = useState(false);
    const [keyBusy, setKeyBusy] = useState(false);
    // Fetching the current key is the idempotent enable of an enabled
    // repository: the pinned key is fetched and checked again; the source,
    // the confinement and the automatic updates stay.
    const refreshKey = async () => {
        setKeyBusy(true);
        try {
            const res = await fetch('/api/v1/repo', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ service_id: serviceId, action: 'enable' }),
            });
            showToast(res.ok ? 'success' : 'error', res.ok ? t('lane.card.refreshKeyDone') : t('life.repo.confineFailed'));
        } catch {
            showToast('error', t('common.resultUnknown'));
        } finally {
            setKeyBusy(false);
            setKeyAsk(false);
            onChanged();
        }
    };
    const component = componentName({ id: serviceId, name: serviceId }, t);
    const confinement = info.confinement;
    const state = confinement?.state ?? 'unknown';
    const stateTone: Tone = state === 'confined' ? 'ok' : state === 'owner_rule' ? 'info' : 'warn';
    const stateText = t(`life.repo.state.${['confined', 'edited', 'owner_rule', 'not_written', 'removed_by_owner'].includes(state) ? state : 'unknown'}` as TranslationKey, {
        component,
        path: confinement?.pin_path ?? '',
        files: (confinement?.owner_files ?? []).join(', '),
    });
    const kept = confinement?.kept ?? [];
    const keptTotal = confinement?.kept_total ?? kept.length;
    const keptNames = keptTotal > kept.length
        ? t('life.repo.keptMore', { names: kept.join(', '), more: keptTotal - kept.length })
        : kept.join(', ');
    const key = info.key;
    const lists = info.lists;
    const moving = info.moving_packages ?? [];
    return (
        <div className="space-y-3" data-repo-state={state}>
            <Line tone={stateTone} attr="confinement"><KeepWords text={stateText} /></Line>
            {(state === 'not_written' || state === 'removed_by_owner') && confinement?.reason && (
                <p className="pl-6 text-sm leading-6 text-fg-muted" data-repo-reason={confinement.reason}>
                    {t(`life.repo.reason.${['write_failed', 'plan_changed'].includes(confinement.reason) ? confinement.reason : 'lists_unreadable'}` as TranslationKey, { detail: confinement.detail ?? '' })}
                </p>
            )}
            {info.lane && missing.length > 0 && (
                <div data-lane-missing={missing.map((step) => step.step).join(',')}>
                    <Line tone={lanePlan?.ready ? 'warn' : 'bad'} attr="lane-missing">
                        <KeepWords text={lanePlan?.ready
                            ? t('lane.card.missing', { steps: missing.map((step) => t(`lane.stepName.${step.step}` as TranslationKey)).join(', ') })
                            : t('lane.card.blocked')} />
                    </Line>
                    <div className="mt-2 pl-6">
                        <Button variant="primary" onClick={() => setLaneOpen(true)} className="max-sm:min-h-11" data-lane-complete="">
                            {t('lane.card.complete')}
                        </Button>
                    </div>
                </div>
            )}
            {laneOpen && (
                <VendorLaneDialog
                    serviceId={serviceId}
                    component={component}
                    onClose={() => setLaneOpen(false)}
                    onDone={() => {
                        setLaneOpen(false);
                        void lane.retry();
                        onChanged();
                    }}
                />
            )}
            {!info.lane && (state === 'not_written' || state === 'removed_by_owner') && (
                <div className="pl-6">
                    <Button variant={state === 'not_written' ? 'primary' : undefined} onClick={() => setAsking(true)} className="max-sm:min-h-11" data-repo-confine="">
                        {state === 'not_written' ? t('life.repo.confineAsk') : t('life.repo.confineAgain')}
                    </Button>
                </div>
            )}
            {asking && (
                <RepoConfineDialog
                    serviceId={serviceId}
                    repoName={info.name ?? serviceId}
                    component={component}
                    onClose={() => setAsking(false)}
                    onDone={() => {
                        setAsking(false);
                        onChanged();
                    }}
                />
            )}
            {state === 'confined' && confinement?.written_by && (
                <p className="pl-6 text-sm text-fg-muted">
                    {t(confinement.written_by === 'owner' ? 'life.repo.writtenBy.owner' : confinement.written_by === 'panel_upgrade' ? 'life.repo.writtenBy.panel_upgrade' : 'life.repo.writtenBy.enable', { when: repoDate(confinement.written_at, locale) })}
                </p>
            )}
            {keptTotal > 0 && (
                <p className="pl-6 text-sm leading-6 text-fg-muted" data-repo-kept={keptTotal}>
                    <KeepWords text={t('life.repo.kept', { count: keptTotal, names: keptNames })} />
                </p>
            )}
            <Line tone="info" attr="enabled">
                {info.enabled_by
                    ? t('life.repo.enabledBy', { who: info.enabled_by, when: repoDate(info.enabled_at, locale) })
                    : info.enabled_at
                      ? t('life.repo.enabledAt', { when: repoDate(info.enabled_at, locale) })
                      : t('life.repo.enabledUnknown')}
            </Line>
            {key && (
                // alpha.108 (owner findings): the fingerprint and its state as their
                // own sentences; "apt no longer accepts" only when apt said so.
                <div data-repo-key={key.state}>
                    <Line tone={key.state === 'expired' ? 'bad' : key.state === 'expiring' ? 'warn' : key.state === 'unknown' ? 'info' : 'ok'} attr={`key-${key.state}`}>
                        <span className="font-mono text-xs">{t('life.repo.key', { fingerprint: groupedFingerprint(key.fingerprint) })}</span>
                        {key.state === 'valid' && <span>{'\u00a0· '}{key.expires_at ? t('life.repo.key.validUntil', { date: repoDate(key.expires_at, locale) }) : t('life.repo.key.valid')}</span>}
                    </Line>
                    {key.state !== 'valid' && (
                        <p className="mt-1 pl-6 text-sm leading-6 text-fg-muted" data-repo-key-state={key.state}>
                            <KeepWords text={
                                key.state === 'expiring' ? t('life.repo.key.expiring', { date: repoDate(key.expires_at, locale) })
                                    : key.state === 'expired' ? (aptReportsExpiredKey(info.apt_errors) ? t('life.repo.key.expired', { date: repoDate(key.expires_at, locale) }) : t('lane.key.expiredFile', { date: repoDate(key.expires_at, locale) }))
                                        : t('life.repo.key.unknown')
                            } />
                        </p>
                    )}
                    {(key.state === 'expiring' || key.state === 'expired') && (
                        <div className="mt-2 pl-6">
                            <Button onClick={() => setKeyAsk(true)} disabled={keyBusy} loading={keyBusy} className="max-sm:min-h-11" data-repo-refresh-key="">
                                {t('lane.card.refreshKey')}
                            </Button>
                        </div>
                    )}
                    {keyAsk && (
                        <Dialog
                            id="repo-refresh-key"
                            title={t('lane.card.refreshKey')}
                            busy={keyBusy}
                            onDismiss={() => setKeyAsk(false)}
                            actions={
                                <>
                                    <Button onClick={() => setKeyAsk(false)} disabled={keyBusy}>{t('common.cancel')}</Button>
                                    <Button variant="primary" onClick={() => void refreshKey()} loading={keyBusy} disabled={keyBusy}>{t('lane.card.refreshKey')}</Button>
                                </>
                            }
                        >
                            <p className="text-sm leading-6 text-fg"><KeepWords text={t('lane.card.refreshKeyAsk', { name: info.name ?? serviceId })} /></p>
                        </Dialog>
                    )}
                </div>
            )}
            {lists && (
                <Line tone={lists.state === 'fresh' ? 'ok' : lists.state === 'expired' ? 'bad' : 'warn'} attr={`lists-${lists.state}`}>
                    {lists.state === 'fresh' && t('life.repo.lists.fresh', { date: repoDate(lists.date, locale) || lists.date || '' })}
                    {lists.state === 'missing' && <KeepWords text={t('life.repo.lists.missing')} />}
                    {lists.state === 'expired' && <KeepWords text={t('life.repo.lists.expired', { date: lists.valid_until ?? '' })} />}
                </Line>
            )}
            {(info.apt_errors ?? []).length > 0 && (
                <div className="rounded-lg border border-danger/40 bg-danger/10 p-3 text-sm" role="alert" data-repo-apt-errors="">
                    <p className="font-medium text-fg">{t('life.repo.aptErrors', { when: repoDate(info.apt_errors_at, locale) })}</p>
                    <ul className="mt-1.5 space-y-1">
                        {(info.apt_errors ?? []).map((line) => (
                            <li key={line} className="break-words font-mono text-xs text-fg">{line}</li>
                        ))}
                    </ul>
                </div>
            )}
            {moving.length > 0 && (
                <div className="border-t border-border/60 pt-3" data-repo-moving={moving.length}>
                    <p className="text-sm font-medium text-fg">{t('life.repo.moving.title')}</p>
                    <ul className="mt-1.5 space-y-1 text-sm leading-6 text-fg-muted">
                        {moving.map((item) => (
                            <li key={item.package}>
                                <KeepWords text={item.exact ? t('life.repo.moving.item', { package: item.package, exact: item.exact }) : t('life.repo.moving.itemNoExact', { package: item.package })} />
                            </li>
                        ))}
                    </ul>
                    <p className="mt-1.5 text-sm leading-6 text-fg-muted">
                        {t(state === 'confined' || state === 'owner_rule' ? 'life.repo.moving.confined' : 'life.repo.moving.notConfined')}
                    </p>
                    {moving.some((item) => item.exact) && (
                        <p className="mt-1.5 text-sm leading-6 text-fg-muted">
                            <KeepWords
                                text={t('life.repo.moving.next', {
                                    command: moving
                                        .filter((item) => item.exact)
                                        .map((item) => `sudo apt-mark manual ${item.exact} && sudo apt-get remove ${item.package}`)
                                        .join('; '),
                                })}
                            />
                        </p>
                    )}
                </div>
            )}
            <p className="text-sm leading-6 text-fg-subtle">{t('life.repo.outsideUpdates')}</p>
        </div>
    );
}
