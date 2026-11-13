import { Suspense, lazy, useMemo, useState } from 'react';
import { FileCode, Puzzle, FileText, type LucideIcon } from 'lucide-react';
import { ServiceShell } from './ServiceShell';
import { RepoCard } from './RepoCard';
import { OfferedVersionsCard } from './OfferedVersionsCard';
import { SecurityEmergencySection } from './SecurityEmergency';
import { PHPExtendedConfig } from './PHPExtendedConfig';
import { KnownEmpty, RemoteGate } from './ui';
import { decodeList, lastKnown, useRemote } from '../lib/remote';
import { useI18n } from '../i18n';
import { apiErrorText, readApiError } from '../lib/apiError';
import { answerWasLost } from '../lib/requestIdentity';
import { confirmAction, followOperation, type OperationStep } from '../lib/confirmAction';
import { useDraft } from './kit/save/draft';
import type { SaveOutcome } from './kit/save/saveRun';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

// alpha.128: the server's limits of PHP settings for customers' sites.
const ServerSettingLimits = lazy(() => import('./ServerSettingLimits').then((module) => ({ default: module.ServerSettingLimits })));

interface PHPManagementProps {
    versions: string[];
    onBack: () => void;
}

interface PHPExtension {
    name: string;
    enabled: boolean;
}

// PHP on ServiceShell. The shell shows the default php-fpm status + start/stop;
// the version selector below drives per-version config: real extension toggles
// and the ini editor. No mock data — extensions come from the live install.
//
// PHP, ServiceShell üzerinde. Kabuk, varsayılan php-fpm durumunu + başlat/durdur
// gösterir; alttaki sürüm seçici, sürüm-başına yapılandırmayı sürer: gerçek
// eklenti anahtarları ve ini düzenleyici. Sahte veri yok — eklentiler canlı
// kurulumdan gelir.
export function PHPManagement({ versions, onBack }: PHPManagementProps) {
    const { t } = useI18n();
    const say = useSaveWords();
    // versions[] now carries only REAL versions (B3b: the "default" sentinel
    // is dead — on Arch the single php-fpm reports its true version too).
    // Empty means PHP is not installed; the shell's install button handles it.
    // versions[] artık yalnız GERÇEK sürümler taşır (B3b: "default" sentinel'i
    // öldü — Arch'ta tek php-fpm de gerçek sürümünü bildirir). Boşsa PHP
    // kurulu değildir; kabuğun kurulum düğmesi bunu karşılar.
    const [version, setVersion] = useState(versions[0] ?? '');
    const [tab, setTab] = useState<'extensions' | 'config'>('extensions');
    // The extensions of the chosen version are being read, could not be read,
    // or known (9 Oct 2026). Before, a read that failed or had not answered
    // was an empty list: the page said "No extensions found" on a server that
    // has them. A switch exists only for an extension the server listed, and
    // it is off while the list is the earlier answer or is being read again.
    // Seçili sürümün eklentileri okunuyor, okunamadı ya da biliniyor. Önceden
    // başarısız ya da yanıtlanmamış okuma boş listeydi. Anahtar yalnız
    // sunucunun listelediği eklenti için vardır.
    const extensions = useRemote(
        version ? `/api/v1/php/extensions?version=${encodeURIComponent(version)}` : null,
        decodeList<PHPExtension>,
    );
    // alpha.132 (owner, 8 Nov 2026): a switch no longer changes the server.
    // The switched extensions are marked and saved together with the save
    // bar; each change is a server change, so Save applies them one after
    // another in the panel's progress window and then reads the list again.
    // A change that was not made stays a marked change. Choosing another
    // version or tab with unsaved switches asks first.
    // Anahtar sunucuyu değiştirmez; değişen eklentiler işaretlenir ve Kaydet
    // onları ilerleme penceresinde tek tek uygular.
    const list = lastKnown(extensions.remote)?.value;
    const known = useMemo(() => (list ? Object.fromEntries(list.map((ext) => [ext.name, ext.enabled])) : null), [list]);
    const draft = useDraft<Record<string, boolean>>(known);

    // Only this page's switches are asked about: the cards below keep theirs.
    const leaveSwitches = async () => draft.count === 0 || confirmAction({
        title: say('leaveTitle'),
        tone: 'danger',
        lost: [draft.count === 1 ? say('leaveLostOne') : say('leaveLost', { n: draft.count })],
        kept: [say('leaveKept')],
        confirmLabel: say('leave'),
        cancelLabel: say('stay'),
    });
    const switchTo = async (next: { version?: string; tab?: 'extensions' | 'config' }) => {
        if (!(await leaveSwitches())) return;
        draft.reset();
        if (next.version !== undefined) setVersion(next.version);
        if (next.tab) setTab(next.tab);
    };

    const save = () => void draft.save(async (changes) => {
        const changed = Object.entries(changes) as [string, boolean][];
        // Nothing is sent unless the progress window runs it.
        let outcome: SaveOutcome = 'refused';
        await followOperation({
            progressTitle: t('php.ext.applying', { version }),
            action: async (report) => {
                const steps: OperationStep[] = [
                    ...changed.map(([name, enabled]): OperationStep => ({ id: 'ext-' + name, label: t(enabled ? 'php.ext.turnOn' : 'php.ext.turnOff', { name }), state: 'pending' })),
                    { id: 'read', label: t('php.ext.reading', { version }), state: 'pending' },
                ];
                const step = (at: number, next: Partial<OperationStep>) => {
                    steps[at] = { ...steps[at], ...next };
                    report.steps(steps.map((item) => ({ ...item })));
                };
                const done: string[] = [];
                let stop: { state: 'failed' | 'unknown'; reason: string } | null = null;
                for (const [at, [name, enabled]] of changed.entries()) {
                    step(at, { state: 'running', since: Date.now() });
                    let res: Response | null;
                    try {
                        res = await fetch('/api/v1/php/extensions', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify({ version, extension: name, enabled }),
                        });
                    } catch {
                        res = null;
                    }
                    if (!res || answerWasLost(res)) {
                        // Not known whether it changed: nothing more is sent.
                        step(at, { state: 'failed', note: t('common.resultUnknown') });
                        stop = { state: 'unknown', reason: t('php.ext.unknown', { name }) };
                        break;
                    }
                    if (!res.ok) {
                        const reason = apiErrorText(await readApiError(res), t, 'php.toggleFailed');
                        step(at, { state: 'failed', note: reason });
                        stop = { state: 'failed', reason: t('php.ext.failed', { name }) };
                        break;
                    }
                    step(at, { state: 'done' });
                    done.push(name);
                }
                // What the server now says is what the switches show.
                const read = steps.length - 1;
                step(read, { state: 'running', since: Date.now() });
                const reread = await extensions.retry();
                step(read, reread.state === 'known' ? { state: 'done' } : { state: 'failed', note: t('php.extensions.unknown', { version }) });
                if (!stop) {
                    outcome = 'saved';
                    return { state: 'done', message: t('php.ext.done') };
                }
                outcome = stop.state === 'unknown' ? 'unknown' : 'refused';
                const before = done.length > 0 ? ' ' + t('php.ext.doneBefore', { names: done.join(', ') }) : '';
                return { state: stop.state, reason: stop.reason + before + ' ' + t('php.ext.next') };
            },
        });
        return outcome;
    });

    return (
        <ServiceShell serviceId="php-fpm" name="PHP-FPM" icon={FileCode} onBack={onBack}>
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-1 border-b border-border">
                    <Tab active={tab === 'extensions'} onClick={() => void switchTo({ tab: 'extensions' })} icon={Puzzle} label={t('php.tab.extensions')} />
                    <Tab active={tab === 'config'} onClick={() => void switchTo({ tab: 'config' })} icon={FileText} label={t('php.tab.config')} />
                </div>
                {versions.length > 1 && (
                    <label className="flex items-center gap-2 text-sm text-fg-muted">
                        {t('php.version')}
                        <select
                            value={version}
                            onChange={(e) => void switchTo({ version: e.target.value })}
                            className="rounded-lg border border-border bg-surface-2 px-3 py-1.5 text-sm font-medium text-fg outline-none focus:border-primary field-sm"
                        >
                            {versions.map((v) => (
                                <option key={v} value={v}>
                                    PHP {v}
                                </option>
                            ))}
                        </select>
                    </label>
                )}
            </div>

            {tab === 'extensions' ? (
                <div className="min-h-[11rem]">
                    <RemoteGate
                        remote={extensions.remote}
                        checking={t('php.extensions.checking', { version })}
                        failed={t('php.extensions.unknown', { version })}
                        onRetry={() => void extensions.retry()}
                        busy={extensions.reading}
                        className="py-2"
                    >
                        {(shown) => (shown.value.length === 0 ? (
                            <KnownEmpty of={shown} icon={Puzzle} title={t('php.emptyExtensions')} />
                        ) : (
                            <div className="rounded-xl border border-border bg-surface p-5">
                                <h3 className="mb-4 text-sm font-semibold text-fg">{t('php.installedExtensions')}</h3>
                                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
                                    {shown.value.map((ext) => {
                                        const enabled = draft.values?.[ext.name] ?? ext.enabled;
                                        const changed = draft.isChanged(ext.name);
                                        return (
                                            <div key={ext.name} className={`rounded-lg border border-border bg-surface-2/50 ${changed ? changedRowClass : ''}`} data-php-extension={ext.name}>
                                                <label className="flex min-h-[2.75rem] cursor-pointer items-center justify-between gap-3 px-3 py-2 hover:bg-surface-2">
                                                    <span className="flex min-w-0 flex-wrap items-center text-sm text-fg-muted">
                                                        <span className="min-w-0 truncate font-mono">{ext.name}</span>
                                                        {changed && <ChangedMark switched={enabled} />}
                                                    </span>
                                                    <input
                                                        type="checkbox"
                                                        checked={enabled}
                                                        disabled={shown.stale || draft.saving}
                                                        onChange={(e) => draft.set(ext.name, e.target.checked)}
                                                        className="h-4 w-8 shrink-0 cursor-pointer appearance-none rounded-full bg-surface-3 transition-colors checked:bg-primary relative before:absolute before:top-0.5 before:left-0.5 before:h-3 before:w-3 before:rounded-full before:bg-white before:transition-transform checked:before:translate-x-4 disabled:cursor-default disabled:opacity-60"
                                                    />
                                                </label>
                                                {draft.isMoved(ext.name) && draft.base && (
                                                    <div className="px-3 pb-2"><ServerMovedNote value={say(draft.base[ext.name] ? 'on' : 'off')} onTake={() => draft.take(ext.name)} /></div>
                                                )}
                                            </div>
                                        );
                                    })}
                                </div>
                                <SaveBar
                                    className="-mx-5 px-5 pb-0"
                                    count={draft.count}
                                    saving={draft.saving}
                                    blockedBy={shown.stale ? say('stale') : undefined}
                                    unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: extensions.reading, onCheck: () => void extensions.retry() } : undefined}
                                    onSave={save}
                                    onDiscard={draft.reset}
                                />
                            </div>
                        ))}
                    </RemoteGate>
                </div>
            ) : (
                <PHPExtendedConfig version={version} />
            )}
            <div className="mt-6"><Suspense fallback={null}><ServerSettingLimits group="php" /></Suspense></div>
            {/* alpha.108 (D-038 §14): which versions customers are offered. */}
            <div className="mt-6 empty:hidden"><OfferedVersionsCard component="php" installed={versions} /></div>
            {/* alpha.107: the vendor PHP repository's card (D-038 §12). */}
            <div className="mt-6 empty:hidden"><RepoCard serviceId="php-fpm" /></div>
            {/* alpha.120 (D-043): PHP's security emergency, closed near the bottom of its own page. */}
            <SecurityEmergencySection component="php" name="PHP" versions={versions} installed={versions.length > 0} />
        </ServiceShell>
    );
}

function Tab({ active, onClick, icon: Icon, label }: { active: boolean; onClick: () => void; icon: LucideIcon; label: string }) {
    return (
        <button
            onClick={onClick}
            className={`-mb-px flex items-center gap-2 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
                active ? 'border-primary text-primary' : 'border-transparent text-fg-muted hover:text-fg'
            }`}
        >
            <Icon className="h-4 w-4" />
            {label}
        </button>
    );
}
