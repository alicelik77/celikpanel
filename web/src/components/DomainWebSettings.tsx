import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Plus, Server, Trash2 } from 'lucide-react';
import { showToast } from './Toast';
import { Button, FormSection, RemoteGate, ResultUnknown, inputClass } from './ui';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
import { useLostAnswer } from '../lib/lostAnswer';
import { followOperation } from '../lib/confirmAction';
import { followRequestProgress, type StepLabel } from '../lib/requestProgress';
import { useWebWords, type WebWords } from '../i18n/webSettings';
import type { TranslationKey } from '../i18n/en';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import type { SaveOutcome } from './kit/save/saveRun';
import { useSaveWords } from './kit/save/words';

// A domain's Hosting → Web server screen (alpha.128, PLESK-GAP G3, G19): index
// files, browser caching, extra file types, response headers, access by
// address, a Node.js site's public/ folder, and the additional nginx
// directives. The server writes them as plain nginx files of the site and
// checks them with `nginx -t` before they are used; when nginx refuses, the
// previous files stay. A save follows the server's own steps in the locked
// progress window; the settings are read again afterwards, so nothing needs a
// reload. The checks here are the server's short rules in words; the server
// checks again.
//
// Alan adının Barındırma → Web sunucusu ekranı: sunucu ayarları sitenin düz
// nginx dosyalarına yazar ve kullanmadan önce nginx -t ile denetler.

interface MIMEType { type: string; extensions: string[] }
interface Header { name: string; value: string }
interface Settings {
    index_files?: string[];
    mime_types?: MIMEType[];
    browser_cache_days?: number;
    headers?: Header[];
    access_mode?: '' | 'allow' | 'deny';
    access_list?: string[];
    node_public?: boolean;
    error_pages?: ErrorPage[];
    page_cache?: boolean;
    page_cache_minutes?: number;
}
interface ErrorPage { code: number; path: string }
interface Directives {
    may_edit: boolean;
    reason?: string;
    grantor?: string;
    offered?: boolean;
    set: boolean;
    text?: string;
    by_admin?: boolean;
    max_size: number;
}
interface WebView {
    project_type: string;
    index_applies: boolean;
    fields_apply: boolean;
    node_applies: boolean;
    page_cache_applies?: boolean;
    may_cache?: boolean;
    default_index?: string;
    settings: Settings;
    cache_choices: number[];
    error_codes?: number[];
    directives: Directives;
}

function decodeWebView(raw: unknown): WebView {
    const body = raw as WebView | null;
    if (!body || typeof body !== 'object' || typeof body.fields_apply !== 'boolean' || !body.settings || !body.directives ||
        typeof body.directives.may_edit !== 'boolean' || !Array.isArray(body.cache_choices)) throw new Error('shape');
    return body;
}

const INDEX_NAME = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;
const MIME_TYPE = /^[a-z0-9][a-z0-9.+-]{0,62}\/[a-z0-9][a-z0-9.+_-]{0,126}$/;
const MIME_EXT = /^[a-z0-9][a-z0-9_-]{0,15}$/;
const HEADER_NAME = /^[A-Za-z0-9][A-Za-z0-9-]{0,63}$/;
const ERROR_PAGE = /^\/[A-Za-z0-9._~/-]{0,250}\.html?$/;

const words = (text: string) => text.split(/\s+/).map((item) => item.trim()).filter(Boolean);

// The sentence for a refusal: the screen's words, then the server's own
// reason for a refused value (it names the field or the line).
function refusal(error: ApiError, t: WebWords): string {
    const text = apiErrorText(error, t as unknown as (key: TranslationKey, values?: Record<string, string | number>) => string, 'common.error');
    const detailed = error.code === 'WEB_ERROR_PAGE_MISSING' || error.code === 'WEB_SETTING_INVALID' || error.code === 'WEB_DIRECTIVES_INVALID' || error.code === 'WEB_SETTINGS_NGINX_REFUSED' || error.code === 'WEB_SETTINGS_NGINX_FAILING';
    return detailed && error.message ? `${text} (${error.message})` : text;
}

function steps(t: WebWords): StepLabel[] {
    return [
        { id: 'check', label: t('web.step.check'), check: true },
        { id: 'save', label: t('web.step.save') },
        { id: 'web_server', label: t('web.step.web_server'), note: t('web.step.web_server.note') },
    ];
}

export function DomainWebSettings({ domainId, domainName, readOnly = false }: { domainId: number; domainName: string; readOnly?: boolean }) {
    const { t } = useWebWords();
    const url = `/api/v1/domains/${domainId}/web-settings`;
    const view = useRemote(url, decodeWebView);
    const answer = useLostAnswer(() => view.retry());
    const [busy, setBusy] = useState(false);
    const settled = view.remote.state === 'known' && !view.reading && !answer.holding;

    // One change, followed in the progress window with the server's steps.
    // How it ended: saved, refused (also when the window never opened and
    // nothing was sent), or unknown when the answer was lost.
    // Bir değişiklik, ilerleme penceresinde sunucunun adımlarıyla izlenir.
    const send = async (path: string, body: unknown, title: string, done: string, key: string, labels: StepLabel[] = steps(t)): Promise<SaveOutcome> => {
        const ended: { outcome: SaveOutcome } = { outcome: 'refused' };
        await followOperation({
            progressTitle: title,
            action: async (report) => {
                setBusy(true);
                const progress = followRequestProgress(report, labels, t as unknown as (key: TranslationKey) => string, key);
                ended.outcome = 'unknown';
                try {
                    const res = await answer.send(path, {
                        method: 'POST', headers: { 'Content-Type': 'application/json', ...progress.headers }, body: JSON.stringify(body),
                    });
                    progress.settle(res);
                    if (!res) return { state: 'unknown', reason: t('web.saveFailed') };
                    if (!res.ok) {
                        ended.outcome = 'refused';
                        const error = await readApiError(res);
                        await view.retry();
                        const blocked = res.status === 409 || res.status === 403;
                        return { state: blocked ? 'blocked' : 'failed', reason: refusal(error, t) };
                    }
                    answer.settle();
                    ended.outcome = 'saved';
                    await view.retry();
                    return { state: 'done', message: done };
                } finally {
                    progress.settle(null);
                    setBusy(false);
                }
            },
        });
        return ended.outcome;
    };

    return (
        <RemoteGate remote={view.remote} checking={t('web.checking')} failed={t('web.unknown')} onRetry={() => void view.retry()} busy={view.reading}>
            {(shown) => {
                const state = shown.value;
                const canChange = settled && !shown.stale && !readOnly && !busy;
                return (
                    <div className="space-y-5">
                        <div>
                            <div className="flex items-center gap-2">
                                <Server className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                                <h3 className="text-base font-semibold text-fg">{t('web.title')}</h3>
                            </div>
                            <p className="mt-1 text-sm text-fg-muted">{t('web.desc')}</p>
                        </div>
                        <ResultUnknown answer={answer} />
                        {!state.fields_apply ? (
                            <p className="rounded-lg border border-border bg-surface-2 px-3 py-2 text-sm text-fg-muted">{t('web.unavailable')}</p>
                        ) : (
                            <>
                                <WebFields
                                    domainId={domainId}
                                    state={state}
                                    disabled={!canChange}
                                    readOnly={readOnly}
                                    t={t}
                                    onSave={(settings) => send(url, { settings }, t('web.progress', { name: domainName }), t('web.saved'), `web.${domainId}`)}
                                    onClearCache={() => send(`${url}/cache-clear`, {}, t('web.pageCache.clearProgress', { name: domainName }), t('web.pageCache.cleared'), `webcache.${domainId}`,
                                        [{ id: 'check', label: t('web.step.check'), check: true }, { id: 'clear', label: t('web.pageCache.clearStep') }])}
                                />
                                <DirectivesCard
                                    domainId={domainId}
                                    directives={state.directives}
                                    disabled={!canChange}
                                    readOnly={readOnly}
                                    t={t}
                                    onSave={(text) => send(`${url}/directives`, { text }, t('web.directives.progress', { name: domainName }), t('web.directives.saved'), `webdir.${domainId}`)}
                                    onOffered={() => void view.retry()}
                                />
                            </>
                        )}
                    </div>
                );
            }}
        </RemoteGate>
    );
}

const CACHE_MINUTES = [5, 10, 30, 60, 240, 1440];

// The form of the Web server card (alpha.132): one draft over the settings
// last read, one entry per section, so a changed section is marked and
// counted once. Nothing is sent until Save; Discard puts the server's values
// back; a section the server changed while it was edited shows the server's
// value beside it.
// Web sunucusu kartının formu: son okunan ayarların üzerinde tek taslak,
// bölüm başına bir alan; Kaydet'e dek hiçbir şey gönderilmez.
interface WebForm {
    index: { custom: boolean; files: string };
    cache: number;
    mimes: Array<{ type: string; exts: string }>;
    headers: Header[];
    access: { mode: '' | 'allow' | 'deny'; list: string };
    node_public: boolean;
    pages: ErrorPage[];
    page_cache: { on: boolean; minutes: number };
}

function formOf(saved: Settings): WebForm {
    return {
        index: { custom: (saved.index_files ?? []).length > 0, files: (saved.index_files ?? []).join(' ') },
        cache: saved.browser_cache_days ?? 0,
        mimes: (saved.mime_types ?? []).map((m) => ({ type: m.type, exts: m.extensions.join(' ') })),
        headers: saved.headers ?? [],
        access: { mode: saved.access_mode ?? '', list: (saved.access_list ?? []).join('\n') },
        node_public: !!saved.node_public,
        pages: saved.error_pages ?? [],
        page_cache: { on: !!saved.page_cache, minutes: saved.page_cache_minutes || 10 },
    };
}

// The settings the form stands for, or what is wrong with it in words.
// Formun karşılığı olan ayarlar ya da neyin yanlış olduğu.
function build(form: WebForm, state: WebView, t: WebWords): { settings: Settings } | { problem: string } {
    const settings: Settings = { browser_cache_days: form.cache };
    if (state.index_applies && form.index.custom) {
        const files = words(form.index.files);
        if (files.length === 0 || files.length > 8 || !files.every((name) => INDEX_NAME.test(name))) return { problem: t('web.problem.index') };
        settings.index_files = files;
    }
    const types: MIMEType[] = [];
    for (const row of form.mimes) {
        if (!row.type.trim() && !row.exts.trim()) continue;
        const exts = words(row.exts.toLowerCase()).map((ext) => ext.replace(/^\./, ''));
        if (!MIME_TYPE.test(row.type.trim().toLowerCase()) || exts.length === 0 || !exts.every((ext) => MIME_EXT.test(ext))) return { problem: t('web.problem.mime') };
        types.push({ type: row.type.trim().toLowerCase(), extensions: exts });
    }
    if (types.length) settings.mime_types = types;
    const sent: Header[] = [];
    for (const row of form.headers) {
        if (!row.name.trim() && !row.value.trim()) continue;
        if (!HEADER_NAME.test(row.name.trim()) || !row.value.trim() || row.value.includes('$') || /[\u0000-\u001f\u007f]/.test(row.value)) return { problem: t('web.problem.header') };
        sent.push({ name: row.name.trim(), value: row.value.trim() });
    }
    if (sent.length) settings.headers = sent;
    if (form.access.mode) {
        const entries = form.access.list.split('\n').map((line) => line.trim()).filter(Boolean);
        if (entries.length === 0) return { problem: t('web.problem.access') };
        settings.access_mode = form.access.mode;
        settings.access_list = entries;
    }
    if (state.node_applies && form.node_public) settings.node_public = true;
    if (state.page_cache_applies && form.page_cache.on) {
        settings.page_cache = true;
        settings.page_cache_minutes = form.page_cache.minutes;
    }
    if (state.index_applies) {
        const sentPages: ErrorPage[] = [];
        for (const page of form.pages) {
            if (!page.path.trim()) continue;
            const path = page.path.trim();
            if (!ERROR_PAGE.test(path) || path.includes('..') || sentPages.some((p) => p.code === page.code)) return { problem: t('web.problem.errorPage') };
            sentPages.push({ code: page.code, path });
        }
        if (sentPages.length) settings.error_pages = sentPages;
    }
    return { settings };
}

// A section's content, marked when the person changed it.
// Bir bölümün içeriği; kişi değiştirdiyse işaretli.
function Marked({ changed, moved, children }: { changed: boolean; moved?: ReactNode; children: ReactNode }) {
    return (
        <div className={`-mx-3 space-y-3 rounded-r px-3 py-1 ${changed ? changedRowClass : ''}`}>
            {changed && <p className="-mb-1"><ChangedMark /></p>}
            {children}
            {moved}
        </div>
    );
}

function WebFields({ domainId, state, disabled, readOnly, t, onSave, onClearCache }: {
    domainId: number; state: WebView; disabled: boolean; readOnly: boolean; t: WebWords; onSave: (settings: Settings) => Promise<SaveOutcome>; onClearCache: () => Promise<SaveOutcome>;
}) {
    const say = useSaveWords();
    const saved = state.settings;
    const known = useMemo(() => formOf(saved), [saved]);
    const draft = useDraft(known);
    const resetDraft = draft.reset;
    // Another domain's settings are not this one's draft.
    useEffect(() => resetDraft(), [domainId, resetDraft]);
    const form = draft.values ?? known;
    const set = draft.set;
    const customIndex = form.index.custom;
    const mimes = form.mimes;
    const headers = form.headers;
    const mode = form.access.mode;
    const pages = form.pages;
    const pageCache = form.page_cache.on;
    const built = build(form, state, t);
    const problem = draft.count > 0 && 'problem' in built ? built.problem : '';
    // What the server holds now, in words, for a section it changed under an edit.
    const serverWords = (field: keyof WebForm): string => {
        const server = draft.base ?? known;
        const list = (items: string[]) => items.filter(Boolean).join(', ') || '—';
        switch (field) {
            case 'index': return server.index.custom ? server.index.files : t('web.index.default', { files: state.default_index ?? '' });
            case 'cache': return server.cache === 0 ? t('web.cache.off') : server.cache === 1 ? t('web.cache.day') : t('web.cache.days', { n: server.cache });
            case 'mimes': return list(server.mimes.map((m) => m.type));
            case 'headers': return list(server.headers.map((h) => h.name));
            case 'access': return server.access.mode === 'allow' ? t('web.access.allow') : server.access.mode === 'deny' ? t('web.access.deny') : t('web.access.everybody');
            case 'node_public': return say(server.node_public ? 'on' : 'off');
            case 'pages': return list(server.pages.map((p) => `${p.code} → ${p.path}`));
            case 'page_cache': return server.page_cache.on ? t('web.pageCache.keep', { n: server.page_cache.minutes }) : say('off');
        }
    };
    const moved = (field: keyof WebForm) => (draft.isMoved(field)
        ? <ServerMovedNote value={serverWords(field)} onTake={() => draft.take(field)} />
        : null);
    const save = () => void draft.save(async (_changes, values) => {
        const result = build(values, state, t);
        if (!('settings' in result)) return 'refused';
        return onSave(result.settings);
    });

    const fieldDisabled = disabled || readOnly || draft.saving;
    return (
        <div className="rounded-xl border border-border bg-surface p-4">
            {state.index_applies && (
                <FormSection title={t('web.index.title')} description={t('web.index.desc')}>
                    <Marked changed={draft.isChanged('index')} moved={moved('index')}>
                    <div className="flex flex-wrap gap-4 text-sm text-fg">
                        <label className="flex items-center gap-2"><input type="radio" className="accent-primary" checked={!customIndex} disabled={fieldDisabled} onChange={() => set('index', { ...form.index, custom: false })} />{t('web.index.default', { files: state.default_index ?? '' })}</label>
                        <label className="flex items-center gap-2"><input type="radio" className="accent-primary" checked={customIndex} disabled={fieldDisabled} onChange={() => set('index', { ...form.index, custom: true })} />{t('web.custom')}</label>
                    </div>
                    {customIndex && (
                        <div>
                            <input value={form.index.files} onChange={(e) => set('index', { ...form.index, files: e.target.value })} disabled={fieldDisabled} placeholder={t('web.index.placeholder')} spellCheck={false} autoComplete="off" className={`${inputClass} font-mono`} aria-label={t('web.index.title')} />
                            <p className="mt-1 text-xs text-fg-subtle">{t('web.index.hint')}</p>
                        </div>
                    )}
                    </Marked>
                </FormSection>
            )}
            <FormSection title={t('web.cache.title')} description={t('web.cache.desc')}>
                <Marked changed={draft.isChanged('cache')} moved={moved('cache')}>
                <select value={form.cache} onChange={(e) => set('cache', Number(e.target.value))} disabled={fieldDisabled} className={`${inputClass} sm:w-60`} aria-label={t('web.cache.title')}>
                    {state.cache_choices.map((days) => (
                        <option key={days} value={days}>{days === 0 ? t('web.cache.off') : days === 1 ? t('web.cache.day') : t('web.cache.days', { n: days })}</option>
                    ))}
                </select>
                </Marked>
            </FormSection>
            <FormSection title={t('web.mime.title')} description={t('web.mime.desc')}>
                <Marked changed={draft.isChanged('mimes')} moved={moved('mimes')}>
                {mimes.map((row, at) => (
                    <div key={at} className="flex flex-col gap-2 sm:flex-row">
                        <input value={row.type} onChange={(e) => set('mimes', mimes.map((m, i) => i === at ? { ...m, type: e.target.value } : m))} disabled={fieldDisabled} placeholder={t('web.mime.type')} aria-label={t('web.mime.type')} spellCheck={false} className={`${inputClass} font-mono`} />
                        <input value={row.exts} onChange={(e) => set('mimes', mimes.map((m, i) => i === at ? { ...m, exts: e.target.value } : m))} disabled={fieldDisabled} placeholder={t('web.mime.ext')} aria-label={t('web.mime.ext')} spellCheck={false} className={`${inputClass} font-mono`} />
                        {!readOnly && <Button type="button" icon={Trash2} disabled={fieldDisabled} onClick={() => set('mimes', mimes.filter((_, i) => i !== at))} aria-label={t('web.remove')}>{t('web.remove')}</Button>}
                    </div>
                ))}
                {!readOnly && <Button type="button" icon={Plus} disabled={fieldDisabled || mimes.length >= 20} onClick={() => set('mimes', [...mimes, { type: '', exts: '' }])}>{t('web.mime.add')}</Button>}
                </Marked>
            </FormSection>
            <FormSection title={t('web.headers.title')} description={t('web.headers.desc')}>
                <Marked changed={draft.isChanged('headers')} moved={moved('headers')}>
                {headers.map((row, at) => (
                    <div key={at} className="flex flex-col gap-2 sm:flex-row">
                        <input value={row.name} onChange={(e) => set('headers', headers.map((h, i) => i === at ? { ...h, name: e.target.value } : h))} disabled={fieldDisabled} placeholder={t('web.headers.name')} aria-label={t('web.headers.name')} spellCheck={false} className={`${inputClass} font-mono`} />
                        <input value={row.value} onChange={(e) => set('headers', headers.map((h, i) => i === at ? { ...h, value: e.target.value } : h))} disabled={fieldDisabled} placeholder={t('web.headers.value')} aria-label={t('web.headers.value')} spellCheck={false} className={`${inputClass} font-mono`} />
                        {!readOnly && <Button type="button" icon={Trash2} disabled={fieldDisabled} onClick={() => set('headers', headers.filter((_, i) => i !== at))} aria-label={t('web.remove')}>{t('web.remove')}</Button>}
                    </div>
                ))}
                {!readOnly && <Button type="button" icon={Plus} disabled={fieldDisabled || headers.length >= 20} onClick={() => set('headers', [...headers, { name: '', value: '' }])}>{t('web.headers.add')}</Button>}
                </Marked>
            </FormSection>
            <FormSection title={t('web.access.title')} description={t('web.access.desc')}>
                <Marked changed={draft.isChanged('access')} moved={moved('access')}>
                <select value={mode} onChange={(e) => set('access', { ...form.access, mode: e.target.value as '' | 'allow' | 'deny' })} disabled={fieldDisabled} className={`${inputClass} sm:w-96`} aria-label={t('web.access.title')}>
                    <option value="">{t('web.access.everybody')}</option>
                    <option value="allow">{t('web.access.allow')}</option>
                    <option value="deny">{t('web.access.deny')}</option>
                </select>
                {mode && (
                    <div>
                        <textarea value={form.access.list} onChange={(e) => set('access', { ...form.access, list: e.target.value })} disabled={fieldDisabled} rows={4} spellCheck={false} className={`${inputClass} font-mono`} aria-label={t('web.access.list')} placeholder="203.0.113.7" />
                        <p className="mt-1 text-xs text-fg-subtle">{t('web.access.list')}</p>
                        {mode === 'allow' && <p className="mt-1 text-xs text-warning">{t('web.access.selfNote')}</p>}
                    </div>
                )}
                </Marked>
            </FormSection>
            {state.index_applies && (state.error_codes ?? []).length > 0 && (
                <FormSection title={t('web.errors.title')} description={t('web.errors.desc')}>
                    <Marked changed={draft.isChanged('pages')} moved={moved('pages')}>
                    {pages.map((row, at) => (
                        <div key={at} className="flex flex-col gap-2 sm:flex-row">
                            <select value={row.code} onChange={(e) => set('pages', pages.map((p, i) => i === at ? { ...p, code: Number(e.target.value) } : p))} disabled={fieldDisabled} className={`${inputClass} sm:w-32`} aria-label={t('web.errors.code')}>
                                {(state.error_codes ?? []).map((code) => <option key={code} value={code}>{code}</option>)}
                            </select>
                            <input value={row.path} onChange={(e) => set('pages', pages.map((p, i) => i === at ? { ...p, path: e.target.value } : p))} disabled={fieldDisabled} placeholder="/errors/404.html" aria-label={t('web.errors.path')} spellCheck={false} className={`${inputClass} font-mono`} />
                            {!readOnly && <Button type="button" icon={Trash2} disabled={fieldDisabled} onClick={() => set('pages', pages.filter((_, i) => i !== at))} aria-label={t('web.remove')}>{t('web.remove')}</Button>}
                        </div>
                    ))}
                    {!readOnly && <Button type="button" icon={Plus} disabled={fieldDisabled || pages.length >= (state.error_codes ?? []).length} onClick={() => set('pages', [...pages, { code: (state.error_codes ?? []).find((code) => !pages.some((p) => p.code === code)) ?? 404, path: '' }])}>{t('web.errors.add')}</Button>}
                    </Marked>
                </FormSection>
            )}
            {state.page_cache_applies && (state.may_cache || saved.page_cache) && (
                <FormSection title={t('web.pageCache.title')} description={t('web.pageCache.desc')}>
                    {state.may_cache ? (
                        <Marked changed={draft.isChanged('page_cache')} moved={moved('page_cache')}>
                        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                            <label className="flex items-center gap-2 text-sm text-fg"><input type="checkbox" className="accent-primary" checked={pageCache} disabled={fieldDisabled} onChange={(e) => set('page_cache', { ...form.page_cache, on: e.target.checked })} />{t('web.pageCache.on')}</label>
                            {pageCache && (
                                <select value={form.page_cache.minutes} onChange={(e) => set('page_cache', { ...form.page_cache, minutes: Number(e.target.value) })} disabled={fieldDisabled} className={`${inputClass} sm:w-48`} aria-label={t('web.pageCache.minutes')}>
                                    {CACHE_MINUTES.map((m) => <option key={m} value={m}>{t('web.pageCache.keep', { n: m })}</option>)}
                                </select>
                            )}
                        </div>
                        </Marked>
                    ) : (
                        <p className="text-sm text-fg-muted">{t('web.pageCache.byAdmin', { n: saved.page_cache_minutes || 10 })}</p>
                    )}
                    {saved.page_cache && !readOnly && (
                        <div><Button type="button" disabled={disabled} onClick={() => void onClearCache()}>{t('web.pageCache.clear')}</Button></div>
                    )}
                </FormSection>
            )}
            {state.node_applies && (
                <FormSection title={t('web.node.title')}>
                    <Marked changed={draft.isChanged('node_public')} moved={moved('node_public')}>
                    <label className="flex cursor-pointer items-start gap-3">
                        <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={form.node_public} disabled={fieldDisabled} onChange={(e) => set('node_public', e.target.checked)} />
                        <span>
                            <span className="block text-sm text-fg">{t('web.node.public')}</span>
                            <span className="block text-xs text-fg-subtle">{t('web.node.publicHint')}</span>
                        </span>
                    </label>
                    </Marked>
                </FormSection>
            )}
            {!readOnly && (
                <SaveBar
                    className="-mx-4 rounded-b-xl px-4 pb-0"
                    count={draft.count}
                    saving={draft.saving}
                    blockedBy={disabled && !draft.saving ? say('stale') : problem || undefined}
                    onSave={save}
                    onDiscard={draft.reset}
                />
            )}
        </div>
    );
}

function DirectivesCard({ domainId, directives, disabled, readOnly, t, onSave, onOffered }: {
    domainId: number; directives: Directives; disabled: boolean; readOnly: boolean; t: WebWords; onSave: (text: string) => Promise<SaveOutcome>; onOffered: () => void;
}) {
    const say = useSaveWords();
    // alpha.132: the text is a draft over the text last read; Save sends it,
    // Discard puts the server's text back, and a newer answer never replaces
    // what is being typed unseen.
    // Metin son okunan metnin üzerindeki taslaktır; yeni yanıt yazılanı görünmeden ezmez.
    const known = useMemo(() => ({ text: directives.text ?? '' }), [directives]);
    const draft = useDraft(known);
    const resetDraft = draft.reset;
    useEffect(() => resetDraft(), [domainId, resetDraft]);
    const text = draft.values?.text ?? known.text;
    const [offering, setOffering] = useState(false);
    if (!directives.may_edit) {
        // Closed: one plain line when the site has directives, nothing else.
        return directives.set ? <p className="text-sm text-fg-muted">{t('web.directives.byAdmin')}</p> : null;
    }
    const bytes = new TextEncoder().encode(text).length;
    const save = () => void draft.save(async (changes) => onSave(changes.text ?? ''));
    const toggleOffer = async () => {
        setOffering(true);
        try {
            const res = await fetch('/api/v1/nginx/directives-offer', {
                method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ offered: !directives.offered }), cache: 'no-store', credentials: 'same-origin',
            });
            if (!res.ok) showToast('error', refusal(await readApiError(res), t));
            else showToast('success', t('web.directives.offerSaved'));
            onOffered();
        } finally {
            setOffering(false);
        }
    };
    return (
        <div className="rounded-xl border border-border bg-surface p-4">
            <h4 className="text-sm font-semibold text-fg">{t('web.directives.title')}</h4>
            <p className="mt-0.5 text-xs text-fg-muted">{t('web.directives.desc')}</p>
            <p className="mt-0.5 text-xs text-fg-subtle">{directives.offered !== undefined ? t('web.directives.adminDesc') : t('web.directives.customerDesc')}</p>
            <div className={`-mx-3 mt-3 rounded-r px-3 py-1 ${draft.isChanged('text') ? changedRowClass : ''}`}>
                {draft.isChanged('text') && <p className="mb-1"><ChangedMark /></p>}
                <textarea value={text} onChange={(e) => draft.set('text', e.target.value)} disabled={disabled || readOnly || draft.saving} rows={8} spellCheck={false}
                    className={`${inputClass} font-mono`} placeholder={t('web.directives.placeholder')} aria-label={t('web.directives.title')} />
                {draft.isMoved('text') && draft.base && (
                    <ServerMovedNote value={draft.base.text.length > 60 ? `${draft.base.text.slice(0, 60)}…` : draft.base.text || '—'} onTake={() => draft.take('text')} />
                )}
            </div>
            <p className={`mt-2 text-xs ${bytes > directives.max_size ? 'text-danger' : 'text-fg-subtle'}`}>{t('web.directives.size', { used: (bytes / 1024).toFixed(1), max: directives.max_size / 1024 })}</p>
            {!readOnly && (
                <SaveBar
                    className="-mx-4 px-4 pb-0"
                    count={draft.count}
                    saving={draft.saving}
                    blockedBy={disabled && !draft.saving ? say('stale') : bytes > directives.max_size ? t('web.directives.tooLarge') : undefined}
                    onSave={save}
                    onDiscard={draft.reset}
                />
            )}
            {directives.offered !== undefined && (
                <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-border pt-3">
                    <p className="text-xs text-fg-muted">{directives.offered ? t('web.directives.offerOn') : t('web.directives.offerOff')}</p>
                    <Button type="button" loading={offering} disabled={disabled} onClick={() => void toggleOffer()}>{directives.offered ? t('web.directives.unoffer') : t('web.directives.offer')}</Button>
                </div>
            )}
        </div>
    );
}
