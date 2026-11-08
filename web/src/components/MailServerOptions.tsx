import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, Info, ShieldAlert, Trash2 } from 'lucide-react';
import { useMailConfigWords } from '../i18n/mailConfig';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction, type OperationEnd } from '../lib/confirmAction';
import { refreshRemote, useRemote } from '../lib/remote';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { Button, RemoteGate, inputClass } from './ui';

// The other mail server settings, one at a time (alpha.130), administrator
// only, server-wide: subaddressing and the sender allow/block list. The state
// comes first with who acts (the owner's own value kept, Dovecot's delimiter,
// a save the server did not take); every change asks first and runs with the
// server's real steps; the list refreshes by itself.
//
// Diğer posta sunucusu ayarları: alt adresleme ve gönderen listesi. Önce
// durum ve kimin yapacağı; her değişiklik sorulur ve gerçek adımlarla çalışır.

export const MAIL_SERVER_SETTINGS_URL = '/api/v1/mail/server-settings';

interface Apply { state?: string; code?: string; detail?: string }
interface Rule { id: number; value: string; action: 'allow' | 'block'; created_at: string }
interface Server {
    postfix: boolean; subaddressing?: string; recipient_delimiter?: string; dovecot_delimiter?: string;
    subaddressing_detail?: string; sender_list?: string; sender_list_detail?: string; error?: string;
}
interface View {
    subaddressing: { enabled: boolean; apply: Apply };
    sender: { rules: Rule[]; apply: Apply };
    server?: Server;
    server_unknown?: string;
}

function decodeView(raw: unknown): View {
    const value = raw as Partial<View> | null;
    if (!value || !value.subaddressing || typeof value.subaddressing.enabled !== 'boolean' || !value.sender || !Array.isArray(value.sender.rules)) {
        throw new Error('mail server settings');
    }
    return value as View;
}

type Tone = 'ok' | 'warn' | 'bad' | 'info';
const tones = {
    ok: { icon: CheckCircle2, surface: 'border-success/40 bg-success/10' },
    warn: { icon: AlertTriangle, surface: 'border-warning-mark/50 bg-warning-mark/15' },
    bad: { icon: ShieldAlert, surface: 'border-danger/40 bg-danger/10' },
    info: { icon: Info, surface: 'border-border bg-surface-2/40' },
} as const;

function StateLine({ tone, text, mark }: { tone: Tone; text: string; mark: string }) {
    const { icon: Icon, surface } = tones[tone];
    return (
        <div role="status" data-mail-setting-state={mark} className={`mb-3 flex items-start gap-2 rounded-lg border p-3 text-sm text-fg ${surface}`}>
            <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
            <p className="min-w-0 max-w-[75ch] flex-1 break-words">{text}</p>
        </div>
    );
}

type T = (key: TranslationKey, values?: Record<string, string | number>) => string;

function subaddressingWords(view: View, t: T): { tone: Tone; text: string; mark: string } {
    const server = view.server;
    const apply = view.subaddressing.apply;
    if (server?.subaddressing === 'owner' && (server.recipient_delimiter ?? '').includes('+')) {
        return { tone: 'ok', mark: 'owner-on', text: t('mcfg.sub.ownerOn', { detail: apply.state === 'refused' ? apply.detail ?? '' : '' }) };
    }
    if (server?.subaddressing === 'owner') return { tone: 'bad', mark: 'owner', text: t('mcfg.sub.owner', { detail: server.subaddressing_detail ?? '' }) };
    if (server?.subaddressing === 'edited') return { tone: 'bad', mark: 'edited', text: t('mcfg.sub.edited', { detail: server.subaddressing_detail ?? '' }) };
    if (apply.code === 'MAIL_SETTING_DOVECOT') return { tone: 'bad', mark: 'dovecot', text: t('mcfg.sub.dovecot', { detail: apply.detail ?? '' }) };
    if (apply.state === 'unknown') return { tone: 'warn', mark: 'lost', text: t('mcfg.settings.lost') };
    if (apply.state === 'refused' || apply.state === 'failed') return { tone: 'bad', mark: 'failed', text: t('mcfg.settings.failed', { detail: apply.detail ?? '' }) };
    if (server?.subaddressing === 'partial') return { tone: 'warn', mark: 'partial', text: t('mcfg.sub.partial') };
    if (server?.subaddressing === 'on') return { tone: 'ok', mark: 'on', text: t('mcfg.sub.on') };
    return { tone: 'info', mark: 'off', text: t('mcfg.sub.off') };
}

function senderWords(view: View, t: T): { tone: Tone; text: string; mark: string } | null {
    const server = view.server;
    const apply = view.sender.apply;
    if (server?.sender_list === 'edited') return { tone: 'bad', mark: 'edited', text: t('mcfg.sender.edited', { detail: server.sender_list_detail ?? '' }) };
    if (server?.sender_list === 'owner') return { tone: 'bad', mark: 'owner', text: t('mcfg.sender.owner', { detail: server.sender_list_detail ?? '' }) };
    if (apply.state === 'unknown') return { tone: 'warn', mark: 'lost', text: t('mcfg.settings.lost') };
    if (apply.state === 'refused' || apply.state === 'failed') return { tone: 'bad', mark: 'failed', text: t('mcfg.sender.notApplied', { detail: apply.detail ?? '' }) };
    if (view.sender.rules.length > 0 && server?.sender_list === 'on') return { tone: 'ok', mark: 'on', text: t('mcfg.sender.on') };
    return null;
}

/** How a settings change ended, from the panel's answer. */
async function settingsEnd(res: Response, t: T, done: string, refusedWords: (code: string, detail: string) => string | null): Promise<OperationEnd> {
    if (!res.ok) {
        const error = await readApiError(res);
        const own = refusedWords(error.code ?? '', apiErrorText(error, t));
        return { state: res.status === 409 ? 'blocked' : 'failed', reason: own ?? apiErrorText(error, t) };
    }
    const answer = await res.json() as { unknown?: string; result?: { applied?: boolean; code?: string; error?: string } };
    if (answer.unknown) return { state: 'unknown', reason: t('mcfg.settings.lost') };
    if (answer.result?.applied) return { state: 'done', message: done };
    if (answer.result?.code === 'MAIL_SETTING_BUSY') return { state: 'blocked', reason: t('mcfg.settings.busy') };
    const own = refusedWords(answer.result?.code ?? '', answer.result?.error ?? '');
    return { state: 'blocked', reason: own ?? t('mcfg.settings.failed', { detail: answer.result?.error ?? '' }) };
}

export function MailServerOptions() {
    const { t, locale } = useMailConfigWords();
    const read = useRemote(MAIL_SERVER_SETTINGS_URL, decodeView);
    const known = read.remote.state === 'known' ? read.remote.value : null;
    const [subDraft, setSubDraft] = useState<boolean | null>(null);
    const [value, setValue] = useState('');
    const [action, setAction] = useState<'block' | 'allow'>('block');
    useEffect(() => {
        // What the server does: a "+" set outside CelikPanel is on too.
        if (known && subDraft === null) setSubDraft(known.subaddressing.enabled || known.server?.subaddressing === 'on' || (known.server?.subaddressing === 'owner' && (known.server.recipient_delimiter ?? '').includes('+')));
    }, [known, subDraft]);
    const time = (text: string) => {
        const at = Date.parse(text.includes('T') ? text : `${text.replace(' ', 'T')}Z`);
        return Number.isFinite(at) ? new Date(at).toLocaleDateString(locale, { dateStyle: 'medium' }) : text;
    };

    const saveSubaddressing = async () => {
        if (subDraft === null) return;
        await confirmAction({
            title: t('mcfg.sub.title'),
            message: subDraft ? t('mcfg.sub.confirmOn') : t('mcfg.sub.confirmOff'),
            confirmLabel: t('mcfg.sub.save'),
            progressTitle: t('mcfg.sub.progress'),
            action: async (report) => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'mailSubaddressing', ['check!', 'save', 'apply']), t, 'mailSubaddressing',
                        (headers) => fetch('/api/v1/mail/subaddressing', { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ enabled: subDraft }) }));
                    return await settingsEnd(res, t, subDraft ? t('mcfg.sub.doneOn') : t('mcfg.sub.doneOff'), (code, detail) => {
                        if (code === 'MAIL_SETTING_DOVECOT') return t('mcfg.sub.dovecot', { detail });
                        if (code === 'MAIL_SETTING_OWNER_CONFIG') return t('mcfg.sub.owner', { detail });
                        if (code === 'MAIL_SETTING_EDITED_CONFIG') return t('mcfg.sub.edited', { detail });
                        return null;
                    });
                } catch {
                    return { state: 'unknown', reason: t('common.resultUnknown') };
                } finally {
                    refreshRemote(MAIL_SERVER_SETTINGS_URL);
                }
            },
        });
    };

    const changeRule = async (body: Record<string, unknown>, message: string, tone: 'default' | 'danger') => {
        await confirmAction({
            title: t('mcfg.sender.title'),
            message,
            tone,
            confirmLabel: body.op === 'add' ? t('mcfg.sender.add') : t('mcfg.sender.remove', { value: String(body.value ?? '') }),
            progressTitle: t('mcfg.sender.progress'),
            action: async (report) => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'mailSender', ['check!', 'save', 'apply']), t, 'mailSender',
                        (headers) => fetch('/api/v1/mail/sender-rules', { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify(body) }));
                    const end = await settingsEnd(res, t, t('mcfg.sender.done'), (code, detail) => {
                        switch (code) {
                            case 'MAIL_SENDER_INVALID': return t('mcfg.sender.invalid', { detail });
                            case 'MAIL_SENDER_EXISTS': return t('mcfg.sender.exists');
                            case 'MAIL_SENDER_TOO_MANY': return t('mcfg.sender.full');
                            case 'MAIL_SETTING_EDITED_CONFIG': return t('mcfg.sender.edited', { detail });
                            case 'MAIL_SETTING_OWNER_CONFIG': return t('mcfg.sender.owner', { detail });
                        }
                        return null;
                    });
                    if (end.state === 'done' && body.op === 'add') setValue('');
                    return end;
                } catch {
                    return { state: 'unknown', reason: t('common.resultUnknown') };
                } finally {
                    refreshRemote(MAIL_SERVER_SETTINGS_URL);
                }
            },
        });
    };

    const addRule = () => {
        const typed = value.trim();
        if (!typed) return;
        const label = action === 'block' ? t('mcfg.sender.block') : t('mcfg.sender.allow');
        void changeRule({ op: 'add', value: typed, action }, t('mcfg.sender.confirmAdd', { value: typed, action: label }), 'default');
    };

    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-5" data-mail-server-options>
            <h3 className="mb-3 text-sm font-semibold text-fg">{t('mcfg.opts.title')}</h3>
            <RemoteGate remote={read.remote} checking={t('mcfg.opts.reading')} failed={t('mcfg.opts.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value: view, stale }) => {
                    const sub = subaddressingWords(view, t);
                    const sender = senderWords(view, t);
                    const serverOn = view.server?.subaddressing === 'on' || (view.server?.subaddressing === 'owner' && (view.server.recipient_delimiter ?? '').includes('+'));
                    const changed = subDraft !== null && subDraft !== serverOn;
                    return (
                        <>
                            {view.server_unknown && <StateLine tone="warn" mark="unknown" text={t('mcfg.opts.unknown')} />}
                            <div data-mail-subaddressing>
                                <h4 className="mb-1 text-sm font-medium text-fg">{t('mcfg.sub.title')}</h4>
                                <p className="mb-3 max-w-[75ch] text-sm text-fg-muted">{t('mcfg.sub.desc')}</p>
                                <StateLine {...sub} />
                                <label className="flex min-h-[1.75rem] items-center gap-2 text-sm text-fg">
                                    <input type="checkbox" className="h-4 w-4" checked={subDraft ?? false} onChange={(event) => setSubDraft(event.target.checked)} />
                                    {t('mcfg.sub.enabled')}
                                </label>
                                <div className="mt-3">
                                    <Button variant="primary" disabled={stale || subDraft === null || !changed}
                                        onClick={() => void saveSubaddressing()}>{t('mcfg.sub.save')}</Button>
                                </div>
                            </div>

                            <div className="mt-6" data-mail-sender-list>
                                <h4 className="mb-1 text-sm font-medium text-fg">{t('mcfg.sender.title')}</h4>
                                <p className="mb-3 max-w-[75ch] text-sm text-fg-muted">{t('mcfg.sender.desc')}</p>
                                {sender && <StateLine {...sender} />}
                                <div className="flex flex-wrap items-end gap-3">
                                    <label className="min-w-0 flex-1 basis-64 text-sm">
                                        <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.sender.value')}</span>
                                        <input type="text" autoComplete="off" spellCheck={false} placeholder="spam.example · someone@example.com" value={value}
                                            onChange={(event) => setValue(event.target.value)} className={inputClass} />
                                    </label>
                                    <label className="text-sm">
                                        <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.sender.action')}</span>
                                        <select className={`${inputClass} w-auto`} value={action} onChange={(event) => setAction(event.target.value as 'block' | 'allow')}>
                                            <option value="block">{t('mcfg.sender.block')}</option>
                                            <option value="allow">{t('mcfg.sender.allow')}</option>
                                        </select>
                                    </label>
                                    <Button variant="primary" disabled={stale || value.trim() === ''} onClick={addRule}>{t('mcfg.sender.add')}</Button>
                                </div>
                                {view.sender.rules.length === 0 ? (
                                    <p className="mt-3 text-sm text-fg-muted">{t('mcfg.sender.empty')}</p>
                                ) : (
                                    <ul className="mt-3 divide-y divide-border rounded-lg border border-border">
                                        {view.sender.rules.map((rule) => (
                                            <li key={rule.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                                                <span className="min-w-0 break-all">
                                                    <span className="font-medium text-fg">{rule.value}</span>
                                                    <span className={rule.action === 'block' ? 'text-danger' : 'text-success'}> · {rule.action === 'block' ? t('mcfg.sender.blocked') : t('mcfg.sender.allowed')}</span>
                                                    <span className="text-fg-subtle"> · {time(rule.created_at)}</span>
                                                </span>
                                                <button type="button" disabled={stale}
                                                    onClick={() => void changeRule({ op: 'remove', id: rule.id, value: rule.value }, t('mcfg.sender.confirmRemove', { value: rule.value }), 'danger')}
                                                    aria-label={t('mcfg.sender.remove', { value: rule.value })} title={t('mcfg.sender.remove', { value: rule.value })}
                                                    className="rounded-md p-1.5 text-fg-subtle transition-colors hover:bg-surface-2 hover:text-danger disabled:pointer-events-none disabled:opacity-50">
                                                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                                                </button>
                                            </li>
                                        ))}
                                    </ul>
                                )}
                            </div>
                        </>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

export default MailServerOptions;
