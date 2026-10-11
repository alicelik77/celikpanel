import { useEffect, useRef, useState, type ReactNode } from 'react';
import { AlertTriangle, FileCode2, FilePen, FolderPlus, Info, RefreshCw } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, ErrorBanner, RemoteGate, ResultUnknown } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
import { useLostAnswer } from '../lib/lostAnswer';

// A site's nginx configuration file, as the server found it (D-031,
// 2026-10-10). The Panel never overwrites a file that is not its own unchanged
// text: this page says what the file is, shows the difference, and offers the
// owner's three choices. It reads first; nothing here changes the server until
// a button is pressed, and every choice is bound to the file the page showed.
//
// Bir sitenin nginx yapılandırma dosyası, sunucunun bulduğu hâliyle (D-031).
// Panel kendi değişmemiş metni olmayan bir dosyanın üzerine asla yazmaz: bu
// sayfa dosyanın ne olduğunu söyler, farkı gösterir ve sahibin üç seçimini
// sunar. Önce okur; bir düğmeye basılmadan sunucuda hiçbir şey değişmez.

export interface SiteConfigView {
    domain_id: number;
    domain: string;
    kind: string;
    path?: string;
    state: string;
    reason?: string;
    detail?: string;
    adopted_from?: string;
    include_dir?: string;
    file_sha256?: string;
    render_sha256?: string;
    pending_path?: string;
    diff?: string;
    diff_truncated?: boolean;
    actions: string[];
    decision?: { kind: string; decided_at: string; current: boolean };
    outcome?: string;
    backup_path?: string;
    // D-031 step 1b: CelikPanel's own include directory and the line a kept
    // file needs for certificate validation; the certificate reason, if any.
    managed_dir?: string;
    managed_include?: string;
    validation?: string;
    challenge_file?: string;
    pending_reason?: string;
    certificate?: {
        cert_path?: string;
        key_path?: string;
        expires_at?: string;
        served_expires_at?: string;
        served_days_left?: number;
        referenced: boolean;
    };
    // The reason this read ended (the probe found the file ready), and the
    // probe's first name not served with nginx's HTTP status.
    resolved_reason?: string;
    validation_name?: string;
    validation_status?: number;
    // The address the probe asked; with validation 'unknown' and
    // validation_unknown_reason 'address_not_local', the site's address
    // this server cannot reach from itself (set11 cell A6).
    validation_address?: string;
    validation_unknown_reason?: string;
    // set10 scenario g: what refuses replacing the file now, measured
    // read-only, and the system's error name a replacement gets ("EPERM").
    protection?: string;
    system_error?: string;
    // What the file's bytes are when `state` is a higher fact about the same
    // bytes: an owner-edited file whose replacement ("take") was refused is
    // `unwritable` with `file_state` "owner_edited" (or foreign, unknown_origin).
    file_state?: string;
    // D-031 second step (2026-10-11): every file of the site in this same
    // shape (the vhost repeats the top level), the worst of their states and
    // the file that decided it.
    files?: SiteConfigView[];
    overall_state?: string;
    overall_kind?: string;
}

const STATES = new Set([
    'managed_unchanged', 'owner_edited', 'foreign', 'unknown_origin', 'absent', 'unreadable', 'unwritable', 'unknown',
]);

export function decodeSiteConfig(raw: unknown): SiteConfigView {
    const body = raw as Partial<SiteConfigView> | null;
    if (!body || typeof body !== 'object' || typeof body.state !== 'string' || !STATES.has(body.state)
        || !Array.isArray(body.actions)) {
        throw new Error('shape');
    }
    if (body.files !== undefined) {
        if (!Array.isArray(body.files)) throw new Error('shape');
        for (const file of body.files) {
            if (!file || typeof file !== 'object' || typeof file.state !== 'string' || !STATES.has(file.state)
                || !Array.isArray(file.actions) || !FILE_KINDS.has(file.kind)) {
                throw new Error('shape');
            }
        }
    }
    return body as SiteConfigView;
}

const KEPT = new Set(['owner_edited', 'foreign', 'unknown_origin']);
const UNREADABLE_REASONS = new Set(['symlink', 'not_regular', 'permission', 'too_large', 'read_failed', 'write_refused']);
const PROTECTIONS = new Set(['immutable', 'append_only', 'directory_immutable', 'read_only_filesystem', 'none']);
const VALIDATION_STATES = new Set(['include_missing', 'names_missing', 'challenge_kept', 'challenge_failed', 'ready', 'unknown']);
// The suffix the API puts on an earlier release's creation-time text.
const CREATION_SUFFIX = ' (creation)';

// The files CelikPanel writes for a site (D-031 second step, 2026-10-11): the
// nginx vhost, the PHP-FPM pool of a PHP site, the application unit of a Node
// site. The vhost's sentences are the page's own; the pool and the unit
// replace the ones that name nginx or the owner's place with their own.
// CelikPanel'in bir site için yazdığı dosyalar; havuz ve birim, nginx'i ya da
// sahibin yerini anan cümleleri kendi cümleleriyle değiştirir.
const FILE_KINDS = new Set(['nginx_vhost', 'php_pool', 'app_unit']);
const KIND_TEXTS = new Set([
    'title', 'managed.body', 'kept.body', 'unknownOrigin.body', 'keptByChoice', 'take.hint', 'take.confirmText',
    'merge.hint', 'missing.body', 'recreate.hint', 'done.take', 'done.recreate', 'done.apply', 'unwritable.body',
    'unwritable.bodyNoError', 'unwritable.next.none', 'held.auto', 'apply.hint', 'editedUnwritable.body', 'include',
    'adopted.start', 'owner',
]);

type Translate = ReturnType<typeof useI18n>['t'];

// kindText answers a sentence of the page for one file: the pool's or the
// unit's own where it has one, the page's otherwise.
export function kindText(t: Translate, kind: string | undefined) {
    return (key: string, vars?: Record<string, string | number>) => t(
        (kind && kind !== 'nginx_vhost' && FILE_KINDS.has(kind) && KIND_TEXTS.has(key)
            ? `siteConfig.${kind}.${key}`
            : `siteConfig.${key}`) as TranslationKey,
        vars,
    );
}

type Action = 'keep' | 'take' | 'recreate' | 'apply';
type Done = { action: Action; kind: string; backup?: string };
type Busy = { action: Action; kind: string };

// The file of `kind` in an answer: the vhost is also the answer's top level.
function fileOf(view: SiteConfigView, kind: string): SiteConfigView | undefined {
    if (view.files && view.files.length > 0) return view.files.find((file) => file.kind === kind);
    return kind === 'nginx_vhost' || !kind ? view : undefined;
}

export function DomainSiteConfig({ domainId, onReasonEnded }: {
    domainId: number;
    domainName: string;
    // Called when a read ended a certificate reason, so the line above the
    // tabs (read from the domains list) is read again.
    onReasonEnded?: () => void;
}) {
    const { t } = useI18n();
    const url = `/api/v1/domains/${domainId}/site-config`;
    const config = useRemote(url, decodeSiteConfig);
    // Once per answer that ended a reason; a later answer carries none.
    const ended = config.remote.state === 'known' && config.remote.value.resolved_reason ? config.remote.value : null;
    const onReasonEndedRef = useRef(onReasonEnded);
    onReasonEndedRef.current = onReasonEnded;
    useEffect(() => {
        if (ended) onReasonEndedRef.current?.();
    }, [ended]);
    const answer = useLostAnswer(() => config.retry());
    const [busy, setBusy] = useState<Busy | null>(null);
    // The file whose "take" waits for its confirmation, and the files whose
    // merge help is open.
    const [confirming, setConfirming] = useState<string | null>(null);
    const [merging, setMerging] = useState<Set<string>>(() => new Set());
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const [done, setDone] = useState<Done | null>(null);
    const fileCount = config.remote.state === 'known' ? Math.max(1, config.remote.value.files?.length ?? 1) : 1;

    const act = async (action: Action, view: SiteConfigView) => {
        if (busy || answer.holding) return;
        const kind = view.kind || 'nginx_vhost';
        setBusy({ action, kind });
        setRefusal(null);
        setDone(null);
        try {
            const digests = action === 'recreate' ? {} : { file_sha256: view.file_sha256, render_sha256: view.render_sha256 };
            // The vhost's request is as it always was; another file names itself.
            const body = kind === 'nginx_vhost' ? digests : { ...digests, kind };
            const question = {
                // What the state read again shows about the choice.
                shows: ([read]: { value: unknown }[]) => {
                    const after = fileOf(read.value as SiteConfigView, kind);
                    if (!after) return false;
                    if (action === 'keep') return after.decision?.kind === 'keep_mine' && after.decision.current;
                    if (action === 'apply') return after.state === 'managed_unchanged' && !after.actions.includes('apply');
                    return after.state === 'managed_unchanged';
                },
            };
            const res = await answer.send(`${url}/${action}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            }, question);
            if (!res) return;
            if (!res.ok) {
                setRefusal(await readApiError(res));
                void config.retry();
                return;
            }
            let result: SiteConfigView;
            try {
                result = (await res.json()) as SiteConfigView;
            } catch {
                // An answer that is not the contract: the result is not known.
                answer.lose(question);
                return;
            }
            answer.settle();
            setConfirming(null);
            setDone({ action, kind, backup: result.backup_path });
            void config.retry();
        } finally {
            setBusy(null);
        }
    };

    const doneText = (finished: Done) => {
        const tk = kindText(t, finished.kind);
        return finished.action === 'take'
            ? tk('done.take', { backup: finished.backup ?? '' })
            : tk(finished.action === 'keep' ? 'done.keep' : finished.action === 'apply' ? 'done.apply' : 'done.recreate');
    };

    return (
        <div className="space-y-4" data-site-config>
            <div className="flex items-center gap-2">
                <FileCode2 className="h-5 w-5 text-fg-muted" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t(fileCount > 1 ? 'siteConfig.files.title' : 'siteConfig.title')}</h2>
            </div>
            <ResultUnknown answer={answer} />
            {done && (
                <p role="status" className="rounded-lg border border-border-strong bg-surface-2 p-3 text-sm text-fg" data-site-config-done={done.action} data-site-config-done-kind={done.kind}>
                    {doneText(done)}
                </p>
            )}
            <ErrorBanner error={refusal} />
            <RemoteGate
                remote={config.remote}
                checking={t('siteConfig.checking')}
                failed={t('siteConfig.unknownRead')}
                onRetry={() => void config.retry()}
                busy={config.reading}
            >
                {({ value, stale }) => {
                    const locked = stale || answer.holding || config.reading || busy !== null;
                    const files = value.files && value.files.length > 0 ? value.files : [value];
                    const many = files.length > 1;
                    return (
                        <div className="space-y-8">
                            {many && <FileList files={files} />}
                            {files.map((file) => {
                                const kind = file.kind || 'nginx_vhost';
                                const state = (
                                    <SiteConfigState
                                        view={file}
                                        locked={locked}
                                        busy={busy?.kind === kind ? busy.action : null}
                                        confirming={confirming === kind}
                                        merging={merging.has(kind)}
                                        onKeep={() => void act('keep', file)}
                                        onTake={() => (confirming === kind ? void act('take', file) : setConfirming(kind))}
                                        onCancel={() => setConfirming(null)}
                                        onMerge={() => setMerging((open) => {
                                            const next = new Set(open);
                                            if (next.has(kind)) next.delete(kind);
                                            else next.add(kind);
                                            return next;
                                        })}
                                        onRecreate={() => void act('recreate', file)}
                                        onApply={() => void act('apply', file)}
                                        onRecheck={() => void config.retry()}
                                        rechecking={config.reading}
                                    />
                                );
                                if (!many) return <div key={kind}>{state}</div>;
                                return (
                                    <section
                                        key={kind}
                                        id={`site-config-file-${kind}`}
                                        aria-labelledby={`site-config-title-${kind}`}
                                        className="space-y-3 border-t border-border pt-6 first:border-t-0 first:pt-0"
                                        data-site-config-file={kind}
                                    >
                                        <h3 id={`site-config-title-${kind}`} className="text-base font-semibold text-fg">
                                            {kindText(t, kind)('title')}
                                        </h3>
                                        {state}
                                    </section>
                                );
                            })}
                        </div>
                    );
                }}
            </RemoteGate>
        </div>
    );
}

// The site's files at a glance: what each is, its state, where it is. A
// state that needs the owner is marked; each row leads to its section.
// Sitenin dosyaları bir bakışta: her biri nedir, durumu ve yeri.
function FileList({ files }: { files: SiteConfigView[] }) {
    const { t } = useI18n();
    return (
        <ul className="divide-y divide-border rounded-lg border border-border bg-surface" data-site-config-files>
            {files.map((file) => {
                const kind = file.kind || 'nginx_vhost';
                const shown = file.state === 'managed_unchanged' && file.actions.includes('apply') ? 'held' : file.state;
                const calm = shown === 'managed_unchanged' || (KEPT.has(file.state) && file.decision?.kind === 'keep_mine' && file.decision.current);
                return (
                    <li key={kind} className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-3 py-2 text-sm" data-site-config-files-row={kind}>
                        <a href={`#site-config-file-${kind}`} className="font-medium text-fg underline-offset-2 hover:underline">
                            {t(`siteConfig.files.kind.${kind}` as TranslationKey)}
                        </a>
                        <span
                            className={calm ? 'text-fg-muted' : 'rounded-md border border-warning-mark/50 bg-warning-mark/20 px-1.5 py-0.5 text-xs font-medium text-fg'}
                            data-site-config-files-state={file.state}
                        >
                            {t(`siteConfig.files.state.${shown}` as TranslationKey)}
                        </span>
                        {file.path && <code className="min-w-0 basis-full break-all font-mono text-xs text-fg-subtle sm:basis-auto">{file.path}</code>}
                    </li>
                );
            })}
        </ul>
    );
}

function SiteConfigState({
    view, locked, busy, confirming, merging, onKeep, onTake, onCancel, onMerge, onRecreate, onApply, onRecheck, rechecking,
}: {
    view: SiteConfigView;
    locked: boolean;
    busy: string | null;
    confirming: boolean;
    merging: boolean;
    onKeep: () => void;
    onTake: () => void;
    onCancel: () => void;
    onMerge: () => void;
    onRecreate: () => void;
    onApply: () => void;
    onRecheck: () => void;
    rechecking: boolean;
}) {
    const { t, locale } = useI18n();
    const tk = kindText(t, view.kind);
    const suffix = view.kind || 'nginx_vhost';
    const path = view.path ?? '';
    const include = view.kind === 'php_pool' ? (
        <p className="text-sm text-fg-muted" data-site-config-include="php_pool">{tk('owner')}</p>
    ) : view.include_dir || view.managed_dir ? (
        <div className="space-y-1 text-sm text-fg-muted" data-site-config-include>
            {view.include_dir && <p>{tk('include', { dir: view.include_dir })}</p>}
            {view.managed_dir && <p data-site-config-managed-dir>{tk('managedDir', { dir: view.managed_dir })}</p>}
        </div>
    ) : null;
    const adopted = view.adopted_from ? (
        <p className="mt-1">
            {view.adopted_from.endsWith(CREATION_SUFFIX)
                ? tk('adopted.creation', { release: view.adopted_from.slice(0, -CREATION_SUFFIX.length) })
                : tk('adopted.start', { release: view.adopted_from })}
        </p>
    ) : null;

    if (view.state === 'managed_unchanged' && view.actions.includes('apply')) {
        // The protection that refused the last change is gone (a "check
        // again" measured none on the same bytes): CelikPanel's own text, its
        // last change held. Written at the next render, or now on request.
        return (
            <div className="space-y-4" data-site-config-held>
                <Plain icon={Info} title={tk('held.title')}>
                    <p>{tk('held.body')}</p>
                    <p className="mt-1">{tk('held.auto')}</p>
                </Plain>
                <FileLine label={tk('file')} value={path} />
                <div className="flex flex-wrap items-center gap-3">
                    <Button type="button" variant="primary" icon={FilePen} loading={busy === 'apply'} disabled={locked} onClick={onApply} data-site-config-apply>
                        {tk('apply')}
                    </Button>
                    <span className="min-w-0 flex-[1_1_16rem] text-sm text-fg-muted">{tk('apply.hint')}</span>
                </div>
                <DiffBlock suffix={suffix} view={view} title={tk('held.diff')} />
                {include}
            </div>
        );
    }
    if (view.state === 'managed_unchanged') {
        return (
            <div className="space-y-3">
                <Plain icon={Info} title={tk('managed.title')}>
                    <p>{tk('managed.body')}</p>
                    {adopted}
                </Plain>
                <FileLine label={tk('file')} value={path} />
                {include}
            </div>
        );
    }
    if (view.state === 'unknown') {
        return (
            <div className="space-y-3">
                <Attention title={tk('unknownState.title')}>
                    <p>{tk(view.reason === 'not_read' ? 'unknownRead' : 'unknownState.body')}</p>
                </Attention>
                {view.actions.includes('recheck') && (
                    <Button type="button" icon={RefreshCw} loading={rechecking} disabled={locked} onClick={onRecheck} data-site-config-recheck>
                        {tk('recheck')}
                    </Button>
                )}
            </div>
        );
    }
    // "Check again" reads this page's answer again; it changes nothing.
    const recheck = view.actions.includes('recheck') ? (
        <div className="flex flex-wrap items-center gap-3">
            <Button type="button" icon={RefreshCw} loading={rechecking} disabled={locked} onClick={onRecheck} data-site-config-recheck>
                {tk('recheck')}
            </Button>
        </div>
    ) : null;
    // The owner's three choices on a kept file; also on an owner-edited file
    // whose replacement was refused ("take" is then answered unwritable while
    // the protection stands).
    const keptByChoice = view.decision?.kind === 'keep_mine' && view.decision.current;
    const choices = KEPT.has(view.state) || view.actions.includes('take') ? (
        <section aria-labelledby={`site-config-choose-${suffix}`} className="space-y-3">
            <h3 id={`site-config-choose-${suffix}`} className="text-sm font-semibold text-fg">{tk('choose')}</h3>
            <div className="grid gap-3 lg:grid-cols-3">
                <Choice
                    hint={tk('take.hint')}
                    action={(
                        <Button type="button" variant="primary" loading={busy === 'take'} disabled={locked} onClick={onTake} data-site-config-take>
                            {confirming ? tk('take.confirm') : tk('take')}
                        </Button>
                    )}
                >
                    {confirming && (
                        <div className="mt-2 space-y-2" data-site-config-confirm>
                            <p className="text-sm text-fg">{tk('take.confirmText', { path })}</p>
                            <Button type="button" onClick={onCancel} disabled={busy === 'take'}>{tk('cancel')}</Button>
                        </div>
                    )}
                </Choice>
                <Choice
                    hint={tk('keep.hint')}
                    action={(
                        <Button type="button" loading={busy === 'keep'} disabled={locked || keptByChoice} onClick={onKeep} data-site-config-keep>
                            {tk('keep')}
                        </Button>
                    )}
                />
                <Choice
                    hint={tk('merge.short')}
                    action={(
                        <Button type="button" onClick={onMerge} aria-expanded={merging} data-site-config-merge>
                            {tk('merge')}
                        </Button>
                    )}
                >
                    {merging && (
                        <p className="mt-2 break-words text-sm text-fg" data-site-config-merge-text>
                            {view.pending_path
                                ? tk('merge.hint', { path, pending: view.pending_path })
                                : tk('merge.noPending', { path })}
                        </p>
                    )}
                </Choice>
            </div>
        </section>
    ) : null;
    if (view.state === 'unwritable') {
        // Read fine, not replaced (set10 scenario g): what refused it, who
        // decides, the next step and how the work resumes; then the text
        // CelikPanel wanted to write.
        const protection = view.protection && PROTECTIONS.has(view.protection) ? view.protection : '';
        const flag = protection === 'immutable' || protection === 'append_only' || protection === 'directory_immutable';
        const command = protection === 'append_only' ? `sudo chattr -a ${path}`
            : protection === 'directory_immutable' ? `sudo chattr -i ${path.slice(0, path.lastIndexOf('/')) || path}`
                : `sudo chattr -i ${path}`;
        // The owner's file whose "take" was refused: both facts, the edit
        // first, then the refusal; the owner's choices stay below.
        const edited = !!view.file_state && KEPT.has(view.file_state);
        return (
            <div className="space-y-4" data-site-config-file-state={edited ? view.file_state : undefined}>
                <Attention title={tk(edited ? 'editedUnwritable.title' : 'unwritable.title')}>
                    {edited && <p>{tk('editedUnwritable.body')}</p>}
                    <p className={edited ? 'mt-1' : undefined}>{edited
                        ? view.system_error
                            ? tk('editedUnwritable.refused', { error: view.system_error })
                            : tk('editedUnwritable.refusedNoError')
                        : view.system_error
                            ? tk('unwritable.body', { error: view.system_error })
                            : tk('unwritable.bodyNoError')}</p>
                    {protection && (
                        <p className="mt-1" data-site-config-protection={protection}>
                            {tk(`unwritable.protection.${protection}`)}
                        </p>
                    )}
                    <p className="mt-1 break-words" data-site-config-next={protection || 'unknown'}>
                        {flag ? tk(edited ? 'editedUnwritable.next.flag' : 'unwritable.next.flag', { command })
                            : protection === 'read_only_filesystem' ? tk('unwritable.next.readOnly')
                                : protection === 'none' ? tk('unwritable.next.none')
                                    : tk('unwritable.next.unknown')}
                    </p>
                    {protection !== 'none' && <p className="mt-1">{tk('unwritable.resume')}</p>}
                </Attention>
                <FileLine label={tk('file')} value={path} />
                {recheck}
                {choices}
                <DiffBlock suffix={suffix} view={view} title={tk(edited ? 'diff.title' : 'unwritable.diff')} />
                {include}
            </div>
        );
    }
    if (view.state === 'unreadable') {
        const reason = view.reason && UNREADABLE_REASONS.has(view.reason) ? view.reason : 'read_failed';
        return (
            <div className="space-y-3">
                <Attention title={tk('unreadable.title')}>
                    <p>{tk(`unreadable.${reason}`)}</p>
                    {/* Who acts and the next step for this reason; the server's
                        own line for any other reason. */}
                    <p className="mt-1" data-site-config-next={reason}>
                        {tk(reason === 'symlink' ? 'unreadable.next.symlink'
                            : reason === 'permission' ? 'unreadable.next.permission'
                                : 'unreadable.next.other')}
                    </p>
                    {view.detail && reason !== 'symlink' && reason !== 'permission' && (
                        <p className="mt-1 break-words">{tk('unreadable.next.detail', { detail: view.detail })}</p>
                    )}
                    <p className="mt-1">{tk('unreadable.meanwhile')}</p>
                </Attention>
                <FileLine label={tk('file')} value={path} />
                {recheck}
            </div>
        );
    }
    if (view.state === 'absent') {
        return (
            <div className="space-y-3">
                <Attention title={tk('missing.title')}>
                    <p>{tk('missing.body')}</p>
                </Attention>
                <FileLine label={tk('file')} value={path} />
                <div className="flex flex-wrap items-center gap-3">
                    <Button type="button" variant="primary" icon={FolderPlus} loading={busy === 'recreate'} disabled={locked} onClick={onRecreate}>
                        {tk('recreate')}
                    </Button>
                    <span className="text-sm text-fg-muted">{tk('recreate.hint')}</span>
                </div>
            </div>
        );
    }
    // Kept: owner-edited, foreign or unknown origin.
    const kept = KEPT.has(view.state);
    if (!kept) return null;
    const decidedAt = view.decision?.decided_at
        ? new Date(view.decision.decided_at).toLocaleString(locale, { dateStyle: 'medium', timeStyle: 'short' })
        : '';
    const title = tk(view.state === 'foreign' ? 'foreign.title'
        : view.state === 'unknown_origin' ? 'unknownOrigin.title' : 'ownerEdited.title');
    const body = tk(view.state === 'unknown_origin' ? 'unknownOrigin.body' : 'kept.body');
    return (
        <div className="space-y-4">
            {keptByChoice ? (
                <Plain icon={Info} title={title}>
                    <p>{tk('keptByChoice', { date: decidedAt })}</p>
                </Plain>
            ) : (
                <Attention title={title}>
                    <p>{body}</p>
                </Attention>
            )}
            <FileLine label={tk('file')} value={path} />
            <CertificatePart view={view} />

            {choices}

            <DiffBlock suffix={suffix} view={view} title={tk('diff.title')} />
            {include}
        </div>
    );
}

// The difference between the file on disk and CelikPanel's text.
// Diskteki dosya ile CelikPanel'in metni arasındaki fark.
function DiffBlock({ view, title, suffix }: { view: SiteConfigView; title: string; suffix: string }) {
    const { t } = useI18n();
    const id = `site-config-diff-${suffix}`;
    return (
        <section aria-labelledby={id} className="space-y-2">
            <h3 id={id} className="text-sm font-semibold text-fg">{title}</h3>
            {view.diff ? (
                <pre
                    tabIndex={0}
                    aria-labelledby={id}
                    className="max-h-96 overflow-auto rounded-lg border border-border bg-surface-2 p-3 font-mono text-xs leading-relaxed text-fg"
                    data-site-config-diff
                >
                    {/* One block as wide as the longest line, so a marked
                        line stays marked when the frame scrolls sideways.
                        En uzun satır kadar geniş tek blok. */}
                    <span className="inline-block min-w-full">
                        {localizeDiffCaptions(view.diff, t).split('\n').map((line, index) => (
                            <span
                                key={index}
                                className={`block whitespace-pre ${line.startsWith('+') && !line.startsWith('+++')
                                    ? 'bg-success/10'
                                    : line.startsWith('-') && !line.startsWith('---') ? 'bg-warning-mark/20' : ''}`}
                            >
                                {line || ' '}
                            </span>
                        ))}
                    </span>
                </pre>
            ) : (
                <p className="text-sm text-fg-muted">{t('siteConfig.diff.none')}</p>
            )}
            {view.diff_truncated && <p className="text-sm text-fg-muted">{t('siteConfig.diff.truncated')}</p>}
        </section>
    );
}

// The two caption lines of the difference come from the server in English;
// they are drawn in the page's language.
// Farkın iki başlık satırı sunucudan İngilizce gelir; sayfanın dilinde çizilir.
const DIFF_HERE = ' (on this server)';
const DIFF_PANEL = "+++ CelikPanel's text";

function localizeDiffCaptions(diff: string, t: ReturnType<typeof useI18n>['t']): string {
    const lines = diff.split('\n');
    if (lines[0]?.startsWith('--- ') && lines[0].endsWith(DIFF_HERE)) {
        lines[0] = `${lines[0].slice(0, -DIFF_HERE.length)} ${t('siteConfig.diff.here')}`;
    }
    if (lines[1] === DIFF_PANEL) {
        lines[1] = `+++ ${t('siteConfig.diff.panel')}`;
    }
    return lines.join('\n');
}

// The certificate part of a kept file (D-031 step 1b): a new certificate that
// the file does not use yet, or a validation the file does not let run.
// Korunan dosyanın sertifika bölümü.
function CertificatePart({ view }: { view: SiteConfigView }) {
    const { t, locale } = useI18n();
    const certificate = view.certificate;
    if (view.resolved_reason === 'certificate_validation') {
        // The probe found the file ready and the wait ended with this read.
        return (
            <div data-site-config-certificate="ready_again">
                <Plain icon={Info} title={t('siteConfig.certificate.validation.readyTitle')}>
                    <p>{t('siteConfig.certificate.validation.ready')}</p>
                </Plain>
            </div>
        );
    }
    if (!view.pending_reason || !certificate) return null;
    const served = certificate.served_expires_at ? (
        <p className="mt-1">
            {certificate.served_days_left !== undefined && certificate.served_days_left < 0
                ? t('siteConfig.certificate.servedExpired', { date: formatDay(certificate.served_expires_at, locale) })
                : t('siteConfig.certificate.servedDays', {
                    date: formatDay(certificate.served_expires_at, locale),
                    days: certificate.served_days_left ?? 0,
                })}
        </p>
    ) : null;
    if (view.pending_reason === 'certificate') {
        return (
            <div data-site-config-certificate="certificate">
                <Attention title={t('siteConfig.certificate.ready.title')}>
                    <p>{t(certificate.referenced ? 'siteConfig.certificate.ready.referenced' : 'siteConfig.certificate.ready.body')}</p>
                    {served}
                    {!certificate.referenced && certificate.cert_path && certificate.key_path && (
                        <div className="mt-2">
                            <p className="text-fg-muted">{t('siteConfig.certificate.lines')}</p>
                            <pre className="mt-1 overflow-x-auto rounded-md border border-border bg-surface p-2 font-mono text-xs text-fg" data-site-config-cert-lines>
                                {`ssl_certificate ${certificate.cert_path};\nssl_certificate_key ${certificate.key_path};`}
                            </pre>
                        </div>
                    )}
                </Attention>
            </div>
        );
    }
    const validation = view.validation && VALIDATION_STATES.has(view.validation) ? view.validation : 'include_missing';
    return (
        <div data-site-config-certificate="certificate_validation" data-site-config-validation={validation}>
            {/* An unanswered probe is not "cannot be validated": its own title. */}
            <Attention title={t(validation === 'unknown' ? 'siteConfig.certificate.validation.unknownTitle' : 'siteConfig.certificate.validation.title')}>
                <p>{t(validation === 'unknown' && view.validation_unknown_reason === 'address_not_local' && view.validation_address
                    ? 'siteConfig.certificate.validation.unknownAddress'
                    : `siteConfig.certificate.validation.${validation}` as TranslationKey, {
                    domain: view.domain, dir: view.managed_dir ?? '', address: view.validation_address ?? '',
                })}</p>
                {validation === 'include_missing' && view.managed_include && (
                    <div className="mt-2">
                        <p className="text-fg-muted">{t('siteConfig.certificate.includeLine')}</p>
                        <pre className="mt-1 overflow-x-auto rounded-md border border-border bg-surface p-2 font-mono text-xs text-fg" data-site-config-include-line>
                            {view.managed_include}
                        </pre>
                    </div>
                )}
                {validation !== 'ready' && validation !== 'unknown' && view.validation_name && view.validation_status ? (
                    <p className="mt-1 break-words" data-site-config-probe>
                        {t('siteConfig.certificate.validation.probe', { name: view.validation_name, status: view.validation_status })}
                    </p>
                ) : null}
                {served}
                <p className="mt-1">{t('siteConfig.certificate.schedule')}</p>
            </Attention>
        </div>
    );
}

function formatDay(value: string, locale: string): string {
    const date = new Date(value);
    return Number.isFinite(date.getTime()) ? date.toLocaleDateString(locale, { dateStyle: 'medium' }) : value;
}

function Attention({ title, children }: { title: string; children: ReactNode }) {
    return (
        <div role="status" className="flex items-start gap-2 rounded-lg border border-warning-mark/50 bg-warning-mark/20 p-3 text-sm leading-relaxed text-fg" data-site-config-state="attention">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <div className="min-w-0 max-w-[75ch]">
                <p className="font-semibold">{title}</p>
                <div className="mt-1">{children}</div>
            </div>
        </div>
    );
}

function Plain({ icon: Icon, title, children }: { icon: typeof Info; title: string; children: ReactNode }) {
    return (
        <div className="flex items-start gap-2 rounded-lg border border-border-strong bg-surface-2 p-3 text-sm leading-relaxed text-fg" data-site-config-state="plain">
            <Icon className="mt-0.5 h-4 w-4 shrink-0 text-fg-muted" aria-hidden="true" />
            <div className="min-w-0 max-w-[75ch]">
                <p className="font-semibold">{title}</p>
                <div className="mt-1">{children}</div>
            </div>
        </div>
    );
}

function FileLine({ label, value }: { label: string; value: string }) {
    if (!value) return null;
    return (
        <p className="flex flex-wrap items-baseline gap-x-2 text-sm">
            <span className="text-fg-subtle">{label}</span>
            <code className="break-all font-mono text-xs text-fg">{value}</code>
        </p>
    );
}

function Choice({ hint, action, children }: { hint: string; action: ReactNode; children?: ReactNode }) {
    return (
        <div className="flex flex-col gap-2 border-t border-border pt-3">
            <div>{action}</div>
            <p className="text-sm text-fg-muted">{hint}</p>
            {children}
        </div>
    );
}

// The line above the domain's tabs when its file needs the owner, or was kept
// by the owner's choice (the domains list carries the state for an
// administrator). A certificate waiting on the file outranks the choice.
// Dosya sahibin kararını beklediğinde ya da seçimiyle korunduğunda alan adının
// sekmelerinin üstündeki satır.
export function SiteConfigNotice({ state, keptByChoice, pendingReason, kind, onOpen }: {
    state: string;
    keptByChoice?: boolean;
    pendingReason?: string;
    // The file whose state this is, when it is not the vhost (php_pool,
    // app_unit; D-031 second step).
    kind?: string;
    onOpen: () => void;
}) {
    const { t } = useI18n();
    const kept = KEPT.has(state);
    const base: TranslationKey | null = kept && pendingReason === 'certificate' ? 'siteConfig.notice.certificate'
        : kept && pendingReason === 'certificate_validation' ? 'siteConfig.notice.certificateValidation'
            : kept && keptByChoice ? 'siteConfig.notice.keptByChoice'
                : kept ? 'siteConfig.notice.kept'
                    : state === 'missing' ? 'siteConfig.notice.missing'
                        : state === 'unreadable' ? 'siteConfig.notice.unreadable'
                            : state === 'unwritable' ? 'siteConfig.notice.unwritable' : null;
    if (!base) return null;
    // The pool's and the unit's own sentence names their file.
    const other = (kind === 'php_pool' || kind === 'app_unit') && !pendingReason;
    const key = (other ? `siteConfig.notice.${kind}.${base.split('.').pop()}` : base) as TranslationKey;
    // Kept as chosen and nothing waiting on it: a plain line, not a warning.
    const plain = base === 'siteConfig.notice.keptByChoice';
    return (
        <div
            role="status"
            className={`mb-4 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border p-3 text-sm text-fg ${plain
                ? 'border-border-strong bg-surface-2'
                : 'border-warning-mark/50 bg-warning-mark/20'}`}
            data-site-config-notice={state}
            data-site-config-notice-file={kind || 'nginx_vhost'}
            data-site-config-notice-kind={plain ? 'kept_by_choice' : pendingReason || 'attention'}
        >
            {plain
                ? <Info className="h-4 w-4 shrink-0 text-fg-muted" aria-hidden="true" />
                : <AlertTriangle className="h-4 w-4 shrink-0 text-warning" aria-hidden="true" />}
            <span className="min-w-0 flex-[1_1_16rem]">{t(key)}</span>
            <Button type="button" onClick={onOpen}>{t('siteConfig.notice.open')}</Button>
        </div>
    );
}
