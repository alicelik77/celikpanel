// Server → Server setup → Operating system (v0.1.0-alpha.107, D-038 §17).
// This server's release and its place in the supported list (a data file in
// the repository), how long the distribution patches it, and, after an OS
// release upgrade, what changed underneath CelikPanel with the next action
// for each, until the administrator marks it reviewed. Moving to a new
// release is the owner's step: CelikPanel never starts it.
//
// İşletim sistemi: bu sunucunun sürümü ve destek listesindeki yeri; sürüm
// değiştiyse neyin değiştiği ve sonraki adım. Yükseltmeyi sahibi yapar.
import { useState } from 'react';
import { Server } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, KeepWords, RemoteGate } from './ui';
import { useRemote } from '../lib/remote';
import { LifeLine, lifeDate, type Tone } from './LifeLine';

type Level = 'tested' | 'accepted' | 'development' | 'preview' | 'not_listed';
interface OSChange { kind: string; subject?: string; from?: string; to?: string }
export interface OSReleaseView {
    checked: string;
    current: { id: string; version_id?: string; codename?: string; pretty_name?: string; level: Level; name?: string; patched_until?: string };
    releases: { name: string; level: Level; patched_until?: string }[];
    recorded: { pretty_name?: string; codename?: string; version_id?: string; recorded_at?: string };
    changed: boolean;
    changes: OSChange[];
}

const LEVELS = new Set<Level>(['tested', 'accepted', 'development', 'preview', 'not_listed']);
const CHANGES = new Set(['repo_suite', 'pg_cluster_new', 'pg_cluster_gone', 'php_added', 'php_removed', 'mariadb_version']);

export function decodeOSRelease(raw: unknown): OSReleaseView {
    const body = raw as (OSReleaseView & { state?: string }) | null;
    if (!body || body.state !== 'read' || !body.current || !LEVELS.has(body.current.level) || !Array.isArray(body.releases) || !Array.isArray(body.changes)) {
        throw new Error('shape');
    }
    return { ...body, changes: body.changes.filter((change) => CHANGES.has(change.kind)) };
}

export function OSReleaseCard() {
    const { t } = useI18n();
    const handle = useRemote('/api/v1/server/os-release', decodeOSRelease);
    return (
        <section className="mt-6 rounded-xl border border-border bg-surface p-5" data-server-os-release="">
            <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                <Server className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                {t('life.os.title')}
            </h2>
            <div className="mt-3">
                <RemoteGate remote={handle.remote} checking={t('life.os.checking')} failed={t('life.os.unknown')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <OSReleaseFacts value={value} onChanged={() => void handle.retry()} />}
                </RemoteGate>
            </div>
        </section>
    );
}

const levelTone: Record<Level, Tone> = { tested: 'ok', accepted: 'info', development: 'info', preview: 'warn', not_listed: 'warn' };

function OSReleaseFacts({ value, onChanged }: { value: OSReleaseView; onChanged: () => void }) {
    const { t, locale } = useI18n();
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState('');
    const current = value.current;
    const name = current.name ?? current.pretty_name ?? current.id;
    const review = async () => {
        setBusy(true);
        setFailure('');
        try {
            const res = await fetch('/api/v1/server/os-release/reviewed', { method: 'POST' });
            if (!res.ok) {
                const raw = await res.text();
                let message = raw.slice(0, 200);
                try {
                    message = (JSON.parse(raw) as { error?: string }).error ?? message;
                } catch {
                    // the answer was not JSON; its first words are said as they are
                }
                setFailure(t('life.os.reviewFailed', { reason: message }));
            }
        } catch {
            setFailure(t('life.os.reviewUnknown'));
        } finally {
            setBusy(false);
            onChanged();
        }
    };
    return (
        <div className="space-y-2 text-sm" data-os-level={current.level} data-os-changed={value.changed ? 'yes' : 'no'}>
            <LifeLine tone={levelTone[current.level]} attr={`os-level-${current.level}`}>
                {t(`life.os.level.${current.level}` as TranslationKey, { name, pretty: current.pretty_name ?? current.id })}
                {current.patched_until && <> {t('life.os.patchedUntil', { date: lifeDate(current.patched_until, locale) })}</>}
            </LifeLine>
            {value.changed && (
                <LifeLine tone="warn" attr="os-changed">
                    {t('life.os.changed', { from: value.recorded.pretty_name ?? value.recorded.codename ?? '', to: current.pretty_name ?? name, date: lifeDate(value.recorded.recorded_at, locale) })}
                </LifeLine>
            )}
            {value.changes.length > 0 && (
                <ul className="space-y-1.5 pl-6" data-os-changes={value.changes.length}>
                    {value.changes.map((change) => (
                        <li key={`${change.kind}-${change.subject ?? ''}`} className="text-fg" data-os-change={change.kind}>
                            <KeepWords text={t(`life.os.change.${change.kind}` as TranslationKey, { subject: change.subject ?? '', from: change.from ?? '', to: change.to ?? '' })} />
                        </li>
                    ))}
                </ul>
            )}
            {value.changed ? (
                <div className="flex flex-wrap items-center gap-3 pl-6">
                    <Button onClick={() => void review()} loading={busy} disabled={busy} className="max-sm:min-h-11" data-os-review="">
                        {t('life.os.review')}
                    </Button>
                    <span className="text-fg-subtle">{t('life.os.reviewHint')}</span>
                </div>
            ) : (
                value.recorded.recorded_at && <p className="text-fg-subtle">{t('life.os.recorded', { date: lifeDate(value.recorded.recorded_at, locale) })}</p>
            )}
            {failure && <LifeLine tone="bad" attr="os-review-failed">{failure}</LifeLine>}
            <p className="text-fg-muted">{t('life.os.ownerStep')}</p>
            <details className="rounded-lg border border-border/60 px-3 py-2">
                <summary className="cursor-pointer text-fg">{t('life.os.list', { date: lifeDate(value.checked, locale) })}</summary>
                <ul className="mt-2 space-y-1">
                    {value.releases.map((release) => (
                        <li key={release.name} className="flex flex-wrap gap-x-2 text-fg-muted" data-os-release-entry={release.level}>
                            <span className="font-medium text-fg">{release.name}</span>
                            <span>{t(`life.os.short.${release.level}` as TranslationKey)}</span>
                            {release.patched_until && <span>{t('life.os.shortPatched', { date: lifeDate(release.patched_until, locale) })}</span>}
                        </li>
                    ))}
                </ul>
            </details>
        </div>
    );
}
