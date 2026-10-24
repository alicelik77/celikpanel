// The repository card (v0.1.0-alpha.107, D-038 §12 and §1/§7): an enabled
// vendor repository on its component's page says first whether it is
// confined to that component (it adds versions the distribution does not
// ship and never replaces a distribution package), then who enabled it and
// when, its signing key and expiry, what apt last downloaded from it and
// apt's own errors about it, and the distribution's moving "default version"
// packages installed beside it with the owner's optional command. A read
// only, except Confine when it is not confined yet (the panel's first start
// on this release normally does that by itself).
//
// Depo kartı: etkin üretici deposunun kendi bileşeniyle sınırlı olup
// olmadığı, kim ne zaman açtı, imza anahtarı ve süresi, apt'ın son indirdiği
// ve hataları, "varsayılan sürüm" paketleri. Salt okunur; yalnız sınırlanmamış
// depo için Sınırla.
import { useState } from 'react';
import { AlertTriangle, CheckCircle2, Info, Package, XCircle } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, KeepWords, RemoteGate } from './ui';
import { useRemote } from '../lib/remote';
import { decodeRepoInfo, type RepoInfo } from '../lib/componentRepo';
import { componentName } from '../lib/componentName';

type Tone = 'ok' | 'warn' | 'bad' | 'info';

const TONE_ICON = { ok: CheckCircle2, warn: AlertTriangle, bad: XCircle, info: Info } as const;
const TONE_CLASS: Record<Tone, string> = {
    ok: 'text-success',
    warn: 'text-warning',
    bad: 'text-danger',
    info: 'text-fg-subtle',
};

// A time the server sent (RFC 3339, or SQLite's "YYYY-MM-DD HH:MM:SS" in UTC),
// said as a date; what cannot be read is said as it came.
export function repoDate(value: string | undefined, locale: string): string {
    if (!value) return '';
    const iso = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(value) ? value.replace(' ', 'T') + 'Z' : value;
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleDateString(locale, { year: 'numeric', month: 'long', day: 'numeric' });
}

// "15058500A0235D97F5D1…" → "1505 8500 A023 …", as fingerprints are read aloud.
export function groupedFingerprint(fingerprint: string): string {
    return (fingerprint.toUpperCase().match(/.{1,4}/g) ?? []).join(' ');
}

function Line({ tone, children, attr }: { tone: Tone; children: React.ReactNode; attr?: string }) {
    const Icon = TONE_ICON[tone];
    return (
        <p className="flex items-start gap-2 text-sm leading-6 text-fg" data-repo-line={attr}>
            <Icon className={`mt-1 h-4 w-4 shrink-0 ${TONE_CLASS[tone]}`} aria-hidden="true" />
            <span className="min-w-0 break-words">{children}</span>
        </p>
    );
}

export function RepoCard({ serviceId }: { serviceId: string }) {
    const url = `/api/v1/repo?service_id=${encodeURIComponent(serviceId)}`;
    const handle = useRemote(url, decodeRepoInfo);
    const { t } = useI18n();
    const remote = handle.remote;
    // Nothing is drawn for a component without a repository, nor for one whose
    // repository is not enabled: the install dialogue offers it there.
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

function RepoFacts({ serviceId, info, onChanged }: { serviceId: string; info: RepoInfo; onChanged: () => void }) {
    const { t, locale } = useI18n();
    const [confining, setConfining] = useState(false);
    const [failed, setFailed] = useState(false);
    const component = componentName({ id: serviceId, name: serviceId }, t);
    const confinement = info.confinement;
    const state = confinement?.state ?? 'unknown';
    const confine = async () => {
        setConfining(true);
        setFailed(false);
        try {
            const res = await fetch('/api/v1/repo', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ service_id: serviceId, action: 'confine' }),
            });
            if (!res.ok) setFailed(true);
        } catch {
            setFailed(true);
        } finally {
            setConfining(false);
            onChanged();
        }
    };
    const stateTone: Tone = state === 'confined' ? 'ok' : state === 'owner_rule' ? 'info' : 'warn';
    const stateText = t(`life.repo.state.${['confined', 'edited', 'owner_rule', 'not_written'].includes(state) ? state : 'unknown'}` as TranslationKey, {
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
            {state === 'not_written' && confinement?.reason && (
                <p className="pl-6 text-sm leading-6 text-fg-muted" data-repo-reason={confinement.reason}>
                    {t(`life.repo.reason.${confinement.reason === 'write_failed' ? 'write_failed' : 'lists_unreadable'}` as TranslationKey, { detail: confinement.detail ?? '' })}
                </p>
            )}
            {state === 'not_written' && (
                <div className="pl-6">
                    <Button variant="primary" onClick={() => void confine()} loading={confining} disabled={confining} className="max-sm:min-h-11" data-repo-confine="">
                        {confining ? t('life.repo.confining') : t('life.repo.confine')}
                    </Button>
                    {failed && <p className="mt-2 text-sm text-danger" role="alert">{t('life.repo.confineFailed')}</p>}
                </div>
            )}
            {state === 'confined' && confinement?.written_by && (
                <p className="pl-6 text-sm text-fg-muted">
                    {t(confinement.written_by === 'panel_upgrade' ? 'life.repo.writtenBy.panel_upgrade' : 'life.repo.writtenBy.enable', { when: repoDate(confinement.written_at, locale) })}
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
                <Line tone={key.state === 'expired' ? 'bad' : key.state === 'expiring' ? 'warn' : key.state === 'unknown' ? 'info' : 'ok'} attr={`key-${key.state}`}>
                    <span className="font-mono text-xs">{t('life.repo.key', { fingerprint: groupedFingerprint(key.fingerprint) })}</span>
                    {' · '}
                    {key.state === 'valid' && (key.expires_at ? t('life.repo.key.validUntil', { date: repoDate(key.expires_at, locale) }) : t('life.repo.key.valid'))}
                    {key.state === 'expiring' && t('life.repo.key.expiring', { date: repoDate(key.expires_at, locale) })}
                    {key.state === 'expired' && t('life.repo.key.expired', { date: repoDate(key.expires_at, locale) })}
                    {key.state === 'unknown' && t('life.repo.key.unknown')}
                </Line>
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
