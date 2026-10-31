import { useState } from 'react';
import { ArrowRight, KeyRound, Lock, Plus, Trash2 } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, RemoteGate, ResultUnknown, inputClass } from './ui';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { decodeList, useRemote } from '../lib/remote';
import { useLostAnswer } from '../lib/lostAnswer';

// A domain's password-protected folders and redirects (alpha.114). Both are
// written into the site's nginx configuration by the server; nginx checks the
// password and sends the redirect itself, so neither needs CelikPanel to run.
// The checks below are the same short allow-lists the server applies (the
// server checks again): they only spare a round trip and say it in words.
//
// Alan adının parolalı klasörleri ve yönlendirmeleri. İkisi de sunucuda sitenin
// nginx yapılandırmasına yazılır; parolayı da yönlendirmeyi de nginx yapar,
// CelikPanel'in çalışması gerekmez. Buradaki denetimler sunucunun uyguladığı
// kısa izin listeleriyle aynıdır (sunucu yine denetler).

const PATH_PATTERN = /^\/[A-Za-z0-9_~-][A-Za-z0-9._~-]*(\/[A-Za-z0-9_~-][A-Za-z0-9._~-]*)*$/;
const USERNAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;
const TARGET_PATTERN = /^[A-Za-z0-9._~:/?&=%@+,!*()[\]-]+$/;

function validPath(value: string, allowRoot: boolean): boolean {
    const path = value.length > 1 ? value.replace(/\/$/, '') : value;
    if (allowRoot && path === '/') return true;
    return path.length <= 255 && PATH_PATTERN.test(path);
}

function validTarget(value: string): boolean {
    if (!value || value.length > 2048 || !TARGET_PATTERN.test(value)) return false;
    if (value.startsWith('/')) return !value.startsWith('//');
    return /^https?:\/\/[^/?]+/i.test(value);
}

// The sentence for a refusal. A refusal of the input carries the server's own
// reason after the localized sentence; every other code is fully localized.
// Girdi reddi, yerelleştirilmiş cümlenin ardından sunucunun nedenini taşır.
function refusalText(error: ApiError, t: ReturnType<typeof useI18n>['t']): string {
    const text = apiErrorText(error, t);
    return error.code === 'SITE_FEATURE_INVALID' && error.message ? `${text} (${error.message})` : text;
}

interface ProtectedDir {
    id: number;
    path: string;
    realm: string;
    users: string[];
}

interface Redirect {
    id: number;
    match: 'exact' | 'prefix';
    source: string;
    target: string;
    code: number;
    keepQuery: boolean;
}

interface SiteFeatures {
    protectAvailable: boolean;
    redirectsAvailable: boolean;
    httpsEnforced: boolean;
    dirs: ProtectedDir[];
    redirects: Redirect[];
}

function decodeSiteFeatures(raw: unknown): SiteFeatures {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as Record<string, unknown>;
    if (
        typeof body.protect_available !== 'boolean' ||
        typeof body.redirects_available !== 'boolean' ||
        typeof body.https_enforced !== 'boolean'
    ) {
        throw new Error('field');
    }
    const dirs = decodeList<Record<string, unknown>>(body.dirs).map((item) => {
        if (typeof item.id !== 'number' || typeof item.path !== 'string' || typeof item.realm !== 'string') {
            throw new Error('dir');
        }
        const users = decodeList<Record<string, unknown>>(item.users).map((user) => {
            if (typeof user.username !== 'string') throw new Error('user');
            return user.username;
        });
        return { id: item.id, path: item.path, realm: item.realm, users };
    });
    const redirects = decodeList<Record<string, unknown>>(body.redirects).map((item) => {
        if (
            typeof item.id !== 'number' ||
            (item.match !== 'exact' && item.match !== 'prefix') ||
            typeof item.source !== 'string' ||
            typeof item.target !== 'string' ||
            typeof item.code !== 'number' ||
            typeof item.keep_query !== 'boolean'
        ) {
            throw new Error('redirect');
        }
        return {
            id: item.id,
            match: item.match,
            source: item.source,
            target: item.target,
            code: item.code,
            keepQuery: item.keep_query,
        } as Redirect;
    });
    return {
        protectAvailable: body.protect_available,
        redirectsAvailable: body.redirects_available,
        httpsEnforced: body.https_enforced,
        dirs,
        redirects,
    };
}

interface PanelProps {
    domainId: number;
    domainName: string;
    readOnly?: boolean;
}

// Reads one list and sends its changes. A change whose answer did not arrive
// is not repeated (D-024): the list is read again and the controls stay off
// until it has answered.
function useSiteFeatureList(domainId: number, path: string) {
    const list = useRemote(`/api/v1/domains/${domainId}/${path}`, decodeSiteFeatures);
    const answer = useLostAnswer(() => list.retry());
    const [busy, setBusy] = useState(false);
    const { t } = useI18n();
    const settled = list.remote.state === 'known' && !list.reading && !answer.holding;

    // Sends one change; true when the server made it.
    const change = async (method: 'POST' | 'DELETE', url: string, body?: unknown, done?: string): Promise<boolean> => {
        setBusy(true);
        try {
            const res = await answer.send(url, {
                method,
                headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
                body: body === undefined ? undefined : JSON.stringify(body),
            });
            if (!res) return false;
            if (!res.ok) {
                const error = await readApiError(res);
                showToast('error', refusalText(error, t));
                // The entry was gone already, or the list changed meanwhile.
                if (res.status === 404 || res.status === 409) await list.retry();
                return false;
            }
            answer.settle();
            if (done) showToast('success', done);
            await list.retry();
            return true;
        } finally {
            setBusy(false);
        }
    };
    return { list, answer, busy, settled, change };
}

export function DomainSiteProtect({ domainId, readOnly = false }: PanelProps) {
    const { t } = useI18n();
    const { list, answer, busy, settled, change } = useSiteFeatureList(domainId, 'protected-dirs');
    const [path, setPath] = useState('');
    const [realm, setRealm] = useState('');
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [problem, setProblem] = useState('');

    const addDir = async () => {
        const wanted = path.trim().replace(/(.)\/$/, '$1');
        if (!validPath(wanted, false)) return setProblem(t('siteFeatures.check.path'));
        if (!USERNAME_PATTERN.test(username)) return setProblem(t('siteFeatures.check.username'));
        if (password.length < 8 || password.length > 72) return setProblem(t('siteFeatures.check.password'));
        setProblem('');
        const made = await change('POST', `/api/v1/domains/${domainId}/protected-dirs`,
            { path: wanted, realm: realm.trim(), username, password }, t('siteFeatures.protect.added', { path: wanted }));
        if (made) {
            setPath('');
            setRealm('');
            setUsername('');
            setPassword('');
        }
    };

    return (
        <RemoteGate
            remote={list.remote}
            checking={t('siteFeatures.protect.checking')}
            failed={t('siteFeatures.protect.unknown')}
            onRetry={() => void list.retry()}
            busy={list.reading}
        >
            {(shown) => {
                const state = shown.value;
                const canChange = settled && !shown.stale && !readOnly && !busy;
                return (
                    <div className="space-y-5">
                        <div>
                            <div className="flex items-center gap-2">
                                <Lock className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                                <h3 className="text-base font-semibold text-fg">{t('siteFeatures.protect.title')}</h3>
                            </div>
                            <p className="mt-1 text-sm text-fg-muted">{t('siteFeatures.protect.desc')}</p>
                        </div>
                        <ResultUnknown answer={answer} />
                        {!state.protectAvailable ? (
                            <p className="rounded-lg border border-border bg-surface-2 px-3 py-2 text-sm text-fg-muted">
                                {t('siteFeatures.protect.unavailable')}
                            </p>
                        ) : (
                            <>
                                {!state.httpsEnforced && state.dirs.length > 0 && (
                                    <p className="rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-sm text-fg">
                                        {t('siteFeatures.protect.httpNote')}
                                    </p>
                                )}
                                {state.dirs.length === 0 ? (
                                    <p className="text-sm text-fg-subtle">{t('siteFeatures.protect.empty')}</p>
                                ) : (
                                    <ul className="space-y-3">
                                        {state.dirs.map((dir) => (
                                            <ProtectedDirCard
                                                key={dir.id}
                                                dir={dir}
                                                disabled={!canChange}
                                                readOnly={readOnly}
                                                domainId={domainId}
                                                change={change}
                                            />
                                        ))}
                                    </ul>
                                )}
                                {!readOnly && (
                                    <div className="rounded-xl border border-border bg-surface p-4">
                                        <h4 className="text-sm font-semibold text-fg">{t('siteFeatures.protect.addTitle')}</h4>
                                        <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
                                            <label className="space-y-1 text-sm font-medium text-fg">
                                                <span>{t('siteFeatures.protect.path')}</span>
                                                <input
                                                    value={path}
                                                    onChange={(e) => setPath(e.target.value)}
                                                    placeholder="/private"
                                                    autoComplete="off"
                                                    spellCheck={false}
                                                    className={`${inputClass} font-mono`}
                                                />
                                                <span className="block text-xs font-normal text-fg-muted">{t('siteFeatures.protect.pathHint')}</span>
                                            </label>
                                            <label className="space-y-1 text-sm font-medium text-fg">
                                                <span>{t('siteFeatures.protect.realm')}</span>
                                                <input
                                                    value={realm}
                                                    onChange={(e) => setRealm(e.target.value)}
                                                    maxLength={64}
                                                    placeholder="Restricted area"
                                                    className={inputClass}
                                                />
                                                <span className="block text-xs font-normal text-fg-muted">{t('siteFeatures.protect.realmHint')}</span>
                                            </label>
                                            <label className="space-y-1 text-sm font-medium text-fg">
                                                <span>{t('siteFeatures.protect.username')}</span>
                                                <input
                                                    value={username}
                                                    onChange={(e) => setUsername(e.target.value)}
                                                    autoComplete="off"
                                                    className={inputClass}
                                                />
                                            </label>
                                            <label className="space-y-1 text-sm font-medium text-fg">
                                                <span>{t('siteFeatures.protect.password')}</span>
                                                <input
                                                    type="password"
                                                    value={password}
                                                    onChange={(e) => setPassword(e.target.value)}
                                                    autoComplete="new-password"
                                                    className={inputClass}
                                                />
                                                <span className="block text-xs font-normal text-fg-muted">{t('siteFeatures.protect.passwordHint')}</span>
                                            </label>
                                        </div>
                                        {problem && <p role="alert" className="mt-3 text-sm text-danger">{problem}</p>}
                                        <p className="mt-3 text-xs text-fg-muted">{t('siteFeatures.protect.nginxNote')}</p>
                                        <div className="mt-3">
                                            <Button
                                                variant="primary"
                                                icon={Plus}
                                                loading={busy}
                                                disabled={!canChange || !path || !username || !password}
                                                onClick={() => void addDir()}
                                            >
                                                {busy ? t('siteFeatures.protect.adding') : t('siteFeatures.protect.add')}
                                            </Button>
                                        </div>
                                    </div>
                                )}
                            </>
                        )}
                    </div>
                );
            }}
        </RemoteGate>
    );
}

function ProtectedDirCard({
    dir,
    disabled,
    readOnly,
    domainId,
    change,
}: {
    dir: ProtectedDir;
    disabled: boolean;
    readOnly: boolean;
    domainId: number;
    change: (method: 'POST' | 'DELETE', url: string, body?: unknown, done?: string) => Promise<boolean>;
}) {
    const { t } = useI18n();
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [problem, setProblem] = useState('');
    const base = `/api/v1/domains/${domainId}/protected-dirs`;

    const saveUser = async () => {
        if (!USERNAME_PATTERN.test(username)) return setProblem(t('siteFeatures.check.username'));
        if (password.length < 8 || password.length > 72) return setProblem(t('siteFeatures.check.password'));
        setProblem('');
        const made = await change('POST', `${base}/users`, { dir_id: dir.id, username, password },
            t('siteFeatures.protect.userSaved', { path: dir.path }));
        if (made) {
            setUsername('');
            setPassword('');
        }
    };

    return (
        <li className="rounded-xl border border-border bg-surface p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="min-w-0">
                    <p className="break-all font-mono text-sm font-semibold text-fg">{dir.path}</p>
                    <p className="text-xs text-fg-muted">{dir.realm}</p>
                </div>
                {!readOnly && (
                    <Button
                        variant="danger"
                        icon={Trash2}
                        disabled={disabled}
                        onClick={() => {
                            if (!confirm(t('siteFeatures.protect.removeDirConfirm', { path: dir.path }))) return;
                            void change('DELETE', `${base}?id=${dir.id}`, undefined, t('siteFeatures.protect.dirRemoved', { path: dir.path }));
                        }}
                    >
                        {t('siteFeatures.protect.removeDir')}
                    </Button>
                )}
            </div>
            <p className="mt-3 text-xs font-medium uppercase tracking-wide text-fg-subtle">{t('siteFeatures.protect.users')}</p>
            <ul className="mt-1 space-y-1">
                {dir.users.map((name) => (
                    <li key={name} className="flex items-center justify-between gap-2 rounded-lg border border-border bg-surface-2/50 px-3 py-1.5">
                        <span className="flex min-w-0 items-center gap-2 break-all font-mono text-sm text-fg">
                            <KeyRound className="h-4 w-4 shrink-0 text-fg-subtle" aria-hidden="true" />
                            {name}
                        </span>
                        {!readOnly && dir.users.length > 1 && (
                            <button
                                disabled={disabled}
                                aria-label={t('siteFeatures.protect.removeUser', { name })}
                                title={t('siteFeatures.protect.removeUser', { name })}
                                onClick={() => {
                                    if (!confirm(t('siteFeatures.protect.removeUserConfirm', { name, path: dir.path }))) return;
                                    void change('DELETE', `${base}/users?id=${dir.id}&user=${encodeURIComponent(name)}`, undefined,
                                        t('siteFeatures.protect.userRemoved'));
                                }}
                                className="shrink-0 rounded-md p-1.5 text-fg-subtle transition-colors hover:bg-surface-3 hover:text-danger disabled:cursor-not-allowed disabled:opacity-40"
                            >
                                <Trash2 className="h-4 w-4" />
                            </button>
                        )}
                    </li>
                ))}
            </ul>
            {!readOnly && (
                <div className="mt-3">
                    <p className="text-xs font-medium text-fg-muted">{t('siteFeatures.protect.addUser')}</p>
                    <div className="mt-1 grid grid-cols-1 gap-2 sm:grid-cols-[1fr_1fr_auto]">
                        <input
                            value={username}
                            onChange={(e) => setUsername(e.target.value)}
                            placeholder={t('siteFeatures.protect.username')}
                            aria-label={t('siteFeatures.protect.username')}
                            autoComplete="off"
                            className={inputClass}
                        />
                        <input
                            type="password"
                            value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            placeholder={t('siteFeatures.protect.password')}
                            aria-label={t('siteFeatures.protect.password')}
                            autoComplete="new-password"
                            className={inputClass}
                        />
                        <Button variant="secondary" disabled={disabled || !username || !password} onClick={() => void saveUser()}>
                            {t('siteFeatures.protect.saveUser')}
                        </Button>
                    </div>
                    {problem && <p role="alert" className="mt-2 text-sm text-danger">{problem}</p>}
                </div>
            )}
        </li>
    );
}

const REDIRECT_CODES = [301, 302, 307, 308] as const;

export function DomainSiteRedirects({ domainId, readOnly = false }: PanelProps) {
    const { t } = useI18n();
    const { list, answer, busy, settled, change } = useSiteFeatureList(domainId, 'redirects');
    const [match, setMatch] = useState<'exact' | 'prefix'>('exact');
    const [source, setSource] = useState('');
    const [target, setTarget] = useState('');
    const [code, setCode] = useState<number>(301);
    const [keepQuery, setKeepQuery] = useState(false);
    const [problem, setProblem] = useState('');

    const addRedirect = async () => {
        const from = source.trim().replace(/(.)\/$/, '$1');
        const to = target.trim();
        if (!validPath(from, match === 'exact')) return setProblem(t('siteFeatures.check.source'));
        if (!validTarget(to)) return setProblem(t('siteFeatures.check.target'));
        setProblem('');
        const made = await change('POST', `/api/v1/domains/${domainId}/redirects`,
            { match, source: from, target: to, code, keep_query: keepQuery }, t('siteFeatures.redirect.added', { path: from }));
        if (made) {
            setSource('');
            setTarget('');
            setKeepQuery(false);
        }
    };

    return (
        <RemoteGate
            remote={list.remote}
            checking={t('siteFeatures.redirect.checking')}
            failed={t('siteFeatures.redirect.unknown')}
            onRetry={() => void list.retry()}
            busy={list.reading}
        >
            {(shown) => {
                const state = shown.value;
                const canChange = settled && !shown.stale && !readOnly && !busy;
                return (
                    <div className="space-y-5">
                        <div>
                            <div className="flex items-center gap-2">
                                <ArrowRight className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                                <h3 className="text-base font-semibold text-fg">{t('siteFeatures.redirect.title')}</h3>
                            </div>
                            <p className="mt-1 text-sm text-fg-muted">{t('siteFeatures.redirect.desc')}</p>
                        </div>
                        <ResultUnknown answer={answer} />
                        {!state.redirectsAvailable ? (
                            <p className="rounded-lg border border-border bg-surface-2 px-3 py-2 text-sm text-fg-muted">
                                {t('siteFeatures.redirect.unavailable')}
                            </p>
                        ) : (
                            <>
                                {state.redirects.length === 0 ? (
                                    <p className="text-sm text-fg-subtle">{t('siteFeatures.redirect.empty')}</p>
                                ) : (
                                    <ul className="space-y-2">
                                        {state.redirects.map((item) => (
                                            <li key={item.id} className="flex items-start justify-between gap-2 rounded-lg border border-border bg-surface-2/50 px-3 py-2">
                                                <div className="min-w-0">
                                                    <p className="flex flex-wrap items-center gap-x-2 break-all font-mono text-sm text-fg">
                                                        <span>{item.source}</span>
                                                        <ArrowRight className="h-4 w-4 shrink-0 text-fg-subtle" aria-hidden="true" />
                                                        <span>{item.target}</span>
                                                    </p>
                                                    <p className="mt-0.5 text-xs text-fg-muted">
                                                        {item.code} · {t(`siteFeatures.redirect.badge.${item.match}` as TranslationKey)}
                                                        {item.keepQuery ? ` · ${t('siteFeatures.redirect.badge.query')}` : ''}
                                                    </p>
                                                </div>
                                                {!readOnly && (
                                                    <button
                                                        disabled={!canChange}
                                                        aria-label={t('siteFeatures.redirect.remove', { path: item.source })}
                                                        title={t('siteFeatures.redirect.remove', { path: item.source })}
                                                        onClick={() => {
                                                            if (!confirm(t('siteFeatures.redirect.removeConfirm', { path: item.source }))) return;
                                                            void change('DELETE', `/api/v1/domains/${domainId}/redirects?id=${item.id}`, undefined,
                                                                t('siteFeatures.redirect.removed'));
                                                        }}
                                                        className="shrink-0 rounded-md p-1.5 text-fg-subtle transition-colors hover:bg-surface-3 hover:text-danger disabled:cursor-not-allowed disabled:opacity-40"
                                                    >
                                                        <Trash2 className="h-4 w-4" />
                                                    </button>
                                                )}
                                            </li>
                                        ))}
                                    </ul>
                                )}
                                {!readOnly && (
                                    <div className="rounded-xl border border-border bg-surface p-4">
                                        <h4 className="text-sm font-semibold text-fg">{t('siteFeatures.redirect.addTitle')}</h4>
                                        <fieldset className="mt-3">
                                            <legend className="text-sm font-medium text-fg">{t('siteFeatures.redirect.match')}</legend>
                                            <div className="mt-1 flex flex-wrap gap-x-5 gap-y-1">
                                                {(['exact', 'prefix'] as const).map((value) => (
                                                    <label key={value} className="flex items-center gap-2 text-sm text-fg">
                                                        <input
                                                            type="radio"
                                                            name={`redirect-match-${domainId}`}
                                                            checked={match === value}
                                                            onChange={() => setMatch(value)}
                                                        />
                                                        {t(`siteFeatures.redirect.match.${value}` as TranslationKey)}
                                                    </label>
                                                ))}
                                            </div>
                                        </fieldset>
                                        <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
                                            <label className="space-y-1 text-sm font-medium text-fg">
                                                <span>{t('siteFeatures.redirect.source')}</span>
                                                <input
                                                    value={source}
                                                    onChange={(e) => setSource(e.target.value)}
                                                    placeholder="/old-page"
                                                    autoComplete="off"
                                                    spellCheck={false}
                                                    className={`${inputClass} font-mono`}
                                                />
                                                <span className="block text-xs font-normal text-fg-muted">{t('siteFeatures.redirect.sourceHint')}</span>
                                            </label>
                                            <label className="space-y-1 text-sm font-medium text-fg">
                                                <span>{t('siteFeatures.redirect.target')}</span>
                                                <input
                                                    value={target}
                                                    onChange={(e) => setTarget(e.target.value)}
                                                    placeholder="https://example.org/new"
                                                    autoComplete="off"
                                                    spellCheck={false}
                                                    className={`${inputClass} font-mono`}
                                                />
                                                <span className="block text-xs font-normal text-fg-muted">{t('siteFeatures.redirect.targetHint')}</span>
                                            </label>
                                            <label className="space-y-1 text-sm font-medium text-fg">
                                                <span>{t('siteFeatures.redirect.code')}</span>
                                                <select value={code} onChange={(e) => setCode(Number(e.target.value))} className={inputClass}>
                                                    {REDIRECT_CODES.map((value) => (
                                                        <option key={value} value={value}>
                                                            {t(`siteFeatures.redirect.code.${value}` as TranslationKey)}
                                                        </option>
                                                    ))}
                                                </select>
                                            </label>
                                            <label className="flex items-start gap-2 self-end pb-2 text-sm text-fg">
                                                <input type="checkbox" checked={keepQuery} onChange={(e) => setKeepQuery(e.target.checked)} className="mt-0.5" />
                                                {t('siteFeatures.redirect.keepQuery')}
                                            </label>
                                        </div>
                                        {problem && <p role="alert" className="mt-3 text-sm text-danger">{problem}</p>}
                                        <p className="mt-3 text-xs text-fg-muted">{t('siteFeatures.protect.nginxNote')}</p>
                                        <div className="mt-3">
                                            <Button
                                                variant="primary"
                                                icon={Plus}
                                                loading={busy}
                                                disabled={!canChange || !source || !target}
                                                onClick={() => void addRedirect()}
                                            >
                                                {busy ? t('siteFeatures.redirect.adding') : t('siteFeatures.redirect.add')}
                                            </Button>
                                        </div>
                                    </div>
                                )}
                            </>
                        )}
                    </div>
                );
            }}
        </RemoteGate>
    );
}
