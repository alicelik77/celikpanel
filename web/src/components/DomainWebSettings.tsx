import { useEffect, useState } from 'react';
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
    const send = async (path: string, body: unknown, title: string, done: string, key: string): Promise<boolean> => {
        let made = false;
        await followOperation({
            progressTitle: title,
            action: async (report) => {
                setBusy(true);
                const progress = followRequestProgress(report, steps(t), t as unknown as (key: TranslationKey) => string, key);
                try {
                    const res = await answer.send(path, {
                        method: 'POST', headers: { 'Content-Type': 'application/json', ...progress.headers }, body: JSON.stringify(body),
                    });
                    progress.settle(res);
                    if (!res) return { state: 'unknown', reason: t('web.saveFailed') };
                    if (!res.ok) {
                        const error = await readApiError(res);
                        await view.retry();
                        const blocked = res.status === 409 || res.status === 403;
                        return { state: blocked ? 'blocked' : 'failed', reason: refusal(error, t) };
                    }
                    answer.settle();
                    made = true;
                    await view.retry();
                    return { state: 'done', message: done };
                } finally {
                    progress.settle(null);
                    setBusy(false);
                }
            },
        });
        return made;
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
                                    key={JSON.stringify(state.settings)}
                                    state={state}
                                    disabled={!canChange}
                                    readOnly={readOnly}
                                    t={t}
                                    onSave={(settings) => send(url, { settings }, t('web.progress', { name: domainName }), t('web.saved'), `web.${domainId}`)}
                                />
                                <DirectivesCard
                                    key={state.directives.text ?? ''}
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

function WebFields({ state, disabled, readOnly, t, onSave }: {
    state: WebView; disabled: boolean; readOnly: boolean; t: WebWords; onSave: (settings: Settings) => Promise<boolean>;
}) {
    const saved = state.settings;
    const [customIndex, setCustomIndex] = useState((saved.index_files ?? []).length > 0);
    const [index, setIndex] = useState((saved.index_files ?? []).join(' '));
    const [cache, setCache] = useState(saved.browser_cache_days ?? 0);
    const [mimes, setMimes] = useState<Array<{ type: string; exts: string }>>((saved.mime_types ?? []).map((m) => ({ type: m.type, exts: m.extensions.join(' ') })));
    const [headers, setHeaders] = useState<Header[]>(saved.headers ?? []);
    const [mode, setMode] = useState<'' | 'allow' | 'deny'>(saved.access_mode ?? '');
    const [list, setList] = useState((saved.access_list ?? []).join('\n'));
    const [nodePublic, setNodePublic] = useState(!!saved.node_public);
    const [pages, setPages] = useState<ErrorPage[]>(saved.error_pages ?? []);
    const [problem, setProblem] = useState('');

    const build = (): Settings | null => {
        const settings: Settings = { browser_cache_days: cache };
        if (state.index_applies && customIndex) {
            const files = words(index);
            if (files.length === 0 || files.length > 8 || !files.every((name) => INDEX_NAME.test(name))) return setProblem(t('web.problem.index')), null;
            settings.index_files = files;
        }
        const types: MIMEType[] = [];
        for (const row of mimes) {
            if (!row.type.trim() && !row.exts.trim()) continue;
            const exts = words(row.exts.toLowerCase()).map((ext) => ext.replace(/^\./, ''));
            if (!MIME_TYPE.test(row.type.trim().toLowerCase()) || exts.length === 0 || !exts.every((ext) => MIME_EXT.test(ext))) return setProblem(t('web.problem.mime')), null;
            types.push({ type: row.type.trim().toLowerCase(), extensions: exts });
        }
        if (types.length) settings.mime_types = types;
        const sent: Header[] = [];
        for (const row of headers) {
            if (!row.name.trim() && !row.value.trim()) continue;
            if (!HEADER_NAME.test(row.name.trim()) || !row.value.trim() || row.value.includes('$') || /[\u0000-\u001f\u007f]/.test(row.value)) return setProblem(t('web.problem.header')), null;
            sent.push({ name: row.name.trim(), value: row.value.trim() });
        }
        if (sent.length) settings.headers = sent;
        if (mode) {
            const entries = list.split('\n').map((line) => line.trim()).filter(Boolean);
            if (entries.length === 0) return setProblem(t('web.problem.access')), null;
            settings.access_mode = mode;
            settings.access_list = entries;
        }
        if (state.node_applies && nodePublic) settings.node_public = true;
        if (state.index_applies) {
            const sentPages: ErrorPage[] = [];
            for (const page of pages) {
                if (!page.path.trim()) continue;
                const path = page.path.trim();
                if (!ERROR_PAGE.test(path) || path.includes('..') || sentPages.some((p) => p.code === page.code)) return setProblem(t('web.problem.errorPage')), null;
                sentPages.push({ code: page.code, path });
            }
            if (sentPages.length) settings.error_pages = sentPages;
        }
        setProblem('');
        return settings;
    };

    const fieldDisabled = disabled || readOnly;
    return (
        <div className="rounded-xl border border-border bg-surface p-4">
            {state.index_applies && (
                <FormSection title={t('web.index.title')} description={t('web.index.desc')}>
                    <div className="flex flex-wrap gap-4 text-sm text-fg">
                        <label className="flex items-center gap-2"><input type="radio" className="accent-primary" checked={!customIndex} disabled={fieldDisabled} onChange={() => setCustomIndex(false)} />{t('web.index.default', { files: state.default_index ?? '' })}</label>
                        <label className="flex items-center gap-2"><input type="radio" className="accent-primary" checked={customIndex} disabled={fieldDisabled} onChange={() => setCustomIndex(true)} />{t('web.custom')}</label>
                    </div>
                    {customIndex && (
                        <div>
                            <input value={index} onChange={(e) => setIndex(e.target.value)} disabled={fieldDisabled} placeholder={t('web.index.placeholder')} spellCheck={false} autoComplete="off" className={`${inputClass} font-mono`} aria-label={t('web.index.title')} />
                            <p className="mt-1 text-xs text-fg-subtle">{t('web.index.hint')}</p>
                        </div>
                    )}
                </FormSection>
            )}
            <FormSection title={t('web.cache.title')} description={t('web.cache.desc')}>
                <select value={cache} onChange={(e) => setCache(Number(e.target.value))} disabled={fieldDisabled} className={`${inputClass} sm:w-60`} aria-label={t('web.cache.title')}>
                    {state.cache_choices.map((days) => (
                        <option key={days} value={days}>{days === 0 ? t('web.cache.off') : days === 1 ? t('web.cache.day') : t('web.cache.days', { n: days })}</option>
                    ))}
                </select>
            </FormSection>
            <FormSection title={t('web.mime.title')} description={t('web.mime.desc')}>
                {mimes.map((row, at) => (
                    <div key={at} className="flex flex-col gap-2 sm:flex-row">
                        <input value={row.type} onChange={(e) => setMimes(mimes.map((m, i) => i === at ? { ...m, type: e.target.value } : m))} disabled={fieldDisabled} placeholder={t('web.mime.type')} aria-label={t('web.mime.type')} spellCheck={false} className={`${inputClass} font-mono`} />
                        <input value={row.exts} onChange={(e) => setMimes(mimes.map((m, i) => i === at ? { ...m, exts: e.target.value } : m))} disabled={fieldDisabled} placeholder={t('web.mime.ext')} aria-label={t('web.mime.ext')} spellCheck={false} className={`${inputClass} font-mono`} />
                        {!readOnly && <Button type="button" icon={Trash2} disabled={disabled} onClick={() => setMimes(mimes.filter((_, i) => i !== at))} aria-label={t('web.remove')}>{t('web.remove')}</Button>}
                    </div>
                ))}
                {!readOnly && <Button type="button" icon={Plus} disabled={disabled || mimes.length >= 20} onClick={() => setMimes([...mimes, { type: '', exts: '' }])}>{t('web.mime.add')}</Button>}
            </FormSection>
            <FormSection title={t('web.headers.title')} description={t('web.headers.desc')}>
                {headers.map((row, at) => (
                    <div key={at} className="flex flex-col gap-2 sm:flex-row">
                        <input value={row.name} onChange={(e) => setHeaders(headers.map((h, i) => i === at ? { ...h, name: e.target.value } : h))} disabled={fieldDisabled} placeholder={t('web.headers.name')} aria-label={t('web.headers.name')} spellCheck={false} className={`${inputClass} font-mono`} />
                        <input value={row.value} onChange={(e) => setHeaders(headers.map((h, i) => i === at ? { ...h, value: e.target.value } : h))} disabled={fieldDisabled} placeholder={t('web.headers.value')} aria-label={t('web.headers.value')} spellCheck={false} className={`${inputClass} font-mono`} />
                        {!readOnly && <Button type="button" icon={Trash2} disabled={disabled} onClick={() => setHeaders(headers.filter((_, i) => i !== at))} aria-label={t('web.remove')}>{t('web.remove')}</Button>}
                    </div>
                ))}
                {!readOnly && <Button type="button" icon={Plus} disabled={disabled || headers.length >= 20} onClick={() => setHeaders([...headers, { name: '', value: '' }])}>{t('web.headers.add')}</Button>}
            </FormSection>
            <FormSection title={t('web.access.title')} description={t('web.access.desc')}>
                <select value={mode} onChange={(e) => setMode(e.target.value as '' | 'allow' | 'deny')} disabled={fieldDisabled} className={`${inputClass} sm:w-96`} aria-label={t('web.access.title')}>
                    <option value="">{t('web.access.everybody')}</option>
                    <option value="allow">{t('web.access.allow')}</option>
                    <option value="deny">{t('web.access.deny')}</option>
                </select>
                {mode && (
                    <div>
                        <textarea value={list} onChange={(e) => setList(e.target.value)} disabled={fieldDisabled} rows={4} spellCheck={false} className={`${inputClass} font-mono`} aria-label={t('web.access.list')} placeholder="203.0.113.7" />
                        <p className="mt-1 text-xs text-fg-subtle">{t('web.access.list')}</p>
                        {mode === 'allow' && <p className="mt-1 text-xs text-warning">{t('web.access.selfNote')}</p>}
                    </div>
                )}
            </FormSection>
            {state.index_applies && (state.error_codes ?? []).length > 0 && (
                <FormSection title={t('web.errors.title')} description={t('web.errors.desc')}>
                    {pages.map((row, at) => (
                        <div key={at} className="flex flex-col gap-2 sm:flex-row">
                            <select value={row.code} onChange={(e) => setPages(pages.map((p, i) => i === at ? { ...p, code: Number(e.target.value) } : p))} disabled={fieldDisabled} className={`${inputClass} sm:w-32`} aria-label={t('web.errors.code')}>
                                {(state.error_codes ?? []).map((code) => <option key={code} value={code}>{code}</option>)}
                            </select>
                            <input value={row.path} onChange={(e) => setPages(pages.map((p, i) => i === at ? { ...p, path: e.target.value } : p))} disabled={fieldDisabled} placeholder="/errors/404.html" aria-label={t('web.errors.path')} spellCheck={false} className={`${inputClass} font-mono`} />
                            {!readOnly && <Button type="button" icon={Trash2} disabled={disabled} onClick={() => setPages(pages.filter((_, i) => i !== at))} aria-label={t('web.remove')}>{t('web.remove')}</Button>}
                        </div>
                    ))}
                    {!readOnly && <Button type="button" icon={Plus} disabled={disabled || pages.length >= (state.error_codes ?? []).length} onClick={() => setPages([...pages, { code: (state.error_codes ?? []).find((code) => !pages.some((p) => p.code === code)) ?? 404, path: '' }])}>{t('web.errors.add')}</Button>}
                </FormSection>
            )}
            {state.node_applies && (
                <FormSection title={t('web.node.title')}>
                    <label className="flex cursor-pointer items-start gap-3">
                        <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={nodePublic} disabled={fieldDisabled} onChange={(e) => setNodePublic(e.target.checked)} />
                        <span>
                            <span className="block text-sm text-fg">{t('web.node.public')}</span>
                            <span className="block text-xs text-fg-subtle">{t('web.node.publicHint')}</span>
                        </span>
                    </label>
                </FormSection>
            )}
            {problem && <p role="alert" className="mt-3 text-sm text-danger">{problem}</p>}
            {!readOnly && (
                <div className="flex justify-end pt-4">
                    <Button type="button" variant="primary" disabled={disabled} onClick={() => { const settings = build(); if (settings) void onSave(settings); }}>{t('web.save')}</Button>
                </div>
            )}
        </div>
    );
}

function DirectivesCard({ directives, disabled, readOnly, t, onSave, onOffered }: {
    directives: Directives; disabled: boolean; readOnly: boolean; t: WebWords; onSave: (text: string) => Promise<boolean>; onOffered: () => void;
}) {
    const [text, setText] = useState(directives.text ?? '');
    const [offering, setOffering] = useState(false);
    useEffect(() => setText(directives.text ?? ''), [directives.text]);
    if (!directives.may_edit) {
        // Closed: one plain line when the site has directives, nothing else.
        return directives.set ? <p className="text-sm text-fg-muted">{t('web.directives.byAdmin')}</p> : null;
    }
    const bytes = new TextEncoder().encode(text).length;
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
            <textarea value={text} onChange={(e) => setText(e.target.value)} disabled={disabled || readOnly} rows={8} spellCheck={false}
                className={`${inputClass} mt-3 font-mono`} placeholder={t('web.directives.placeholder')} aria-label={t('web.directives.title')} />
            <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
                <span className={`text-xs ${bytes > directives.max_size ? 'text-danger' : 'text-fg-subtle'}`}>{t('web.directives.size', { used: (bytes / 1024).toFixed(1), max: directives.max_size / 1024 })}</span>
                {!readOnly && <Button type="button" variant="primary" disabled={disabled || bytes > directives.max_size || text === (directives.text ?? '')} onClick={() => void onSave(text)}>{t('web.directives.save')}</Button>}
            </div>
            {directives.offered !== undefined && (
                <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-border pt-3">
                    <p className="text-xs text-fg-muted">{directives.offered ? t('web.directives.offerOn') : t('web.directives.offerOff')}</p>
                    <Button type="button" loading={offering} disabled={disabled} onClick={() => void toggleOffer()}>{directives.offered ? t('web.directives.unoffer') : t('web.directives.offer')}</Button>
                </div>
            )}
        </div>
    );
}
