import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, Info, ShieldAlert } from 'lucide-react';
import { useMailConfigWords } from '../i18n/mailConfig';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction } from '../lib/confirmAction';
import { refreshRemote, useRemote } from '../lib/remote';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { PasswordInput } from './PasswordInput';
import { Button, RemoteGate, inputClass } from './ui';

// Sending outgoing mail through another server (alpha.130, smarthost),
// administrator only, server-wide. The state comes first with who acts (the
// owner's own relay kept, the login modules missing, a save the server did
// not take); then the settings. The password is never shown: an empty field
// keeps the saved one for the same relay and user name. The test signs in and
// out again and sends nothing.
//
// Postanın başka bir sunucu üzerinden gönderilmesi: önce durum ve kimin
// yapacağı, sonra ayarlar. Parola gösterilmez; deneme ileti göndermez.

export const MAIL_RELAY_URL = '/api/v1/mail/relay';
const TEST_URL = '/api/v1/mail/relay/test';
const PORTS = [587, 465, 2525, 25] as const;

interface Settings { enabled: boolean; host: string; port: number; username: string; has_password: boolean }
interface ServerStatus { postfix: boolean; state?: string; relayhost?: string; sasl_ready: boolean; detail?: string; error?: string }
interface View {
    settings: Settings;
    apply: { state?: string; code?: string; detail?: string; applied_at?: string };
    server?: ServerStatus;
    server_unknown?: string;
}
interface TestAnswer { ok: boolean; stage: string; verified: boolean; detail?: string }

function decodeView(raw: unknown): View {
    const value = raw as Partial<View> | null;
    if (!value || !value.settings || typeof value.settings.enabled !== 'boolean' || !value.apply) throw new Error('relay');
    return value as View;
}

type Words = { tone: 'ok' | 'warn' | 'bad' | 'info'; key: TranslationKey; vars?: Record<string, string> };

/** The one sentence that says where the relay stands, and who acts. */
function stateWords(view: View): Words {
    const server = view.server;
    const relay = `${view.settings.host}:${view.settings.port}`;
    if (server && !server.postfix) return { tone: 'info', key: 'mcfg.relay.state.noPostfix' };
    if (server?.state === 'owner') return { tone: 'bad', key: 'mcfg.relay.state.owner', vars: { detail: server.detail ?? '' } };
    if (server?.state === 'edited') return { tone: 'bad', key: 'mcfg.relay.state.edited', vars: { detail: server.detail ?? '' } };
    if (view.apply.code === 'MAIL_RELAY_SASL_MISSING') return { tone: 'bad', key: 'mcfg.relay.state.sasl' };
    if (view.apply.state === 'unknown') return { tone: 'warn', key: 'mcfg.relay.state.lost' };
    if (view.apply.state === 'refused' || view.apply.state === 'failed') return { tone: 'bad', key: 'mcfg.relay.failed', vars: { detail: view.apply.detail ?? '' } };
    if (view.server_unknown) return { tone: 'warn', key: 'mcfg.relay.state.unknown' };
    if (server?.state === 'partial') return { tone: 'warn', key: 'mcfg.relay.state.partial' };
    if (view.settings.enabled && server?.state === 'on') return { tone: 'ok', key: 'mcfg.relay.state.on', vars: { relay } };
    if (view.settings.enabled) return { tone: 'warn', key: 'mcfg.relay.state.pending', vars: { detail: '' } };
    return { tone: 'info', key: 'mcfg.relay.state.off' };
}

const tones = {
    ok: { icon: CheckCircle2, surface: 'border-success/40 bg-success/10' },
    warn: { icon: AlertTriangle, surface: 'border-warning-mark/50 bg-warning-mark/15' },
    bad: { icon: ShieldAlert, surface: 'border-danger/40 bg-danger/10' },
    info: { icon: Info, surface: 'border-border bg-surface-2/40' },
} as const;

export function MailRelaySettings() {
    const { t } = useMailConfigWords();
    const read = useRemote(MAIL_RELAY_URL, decodeView);
    const known = read.remote.state === 'known' ? read.remote.value : null;
    const [draft, setDraft] = useState<Omit<Settings, 'has_password'> | null>(null);
    const [password, setPassword] = useState('');
    const [testing, setTesting] = useState(false);
    const [test, setTest] = useState<{ answer?: TestAnswer; error?: string } | null>(null);
    useEffect(() => {
        if (known && !draft) setDraft({ enabled: known.settings.enabled, host: known.settings.host, port: known.settings.port || 587, username: known.settings.username });
    }, [known, draft]);
    const set = <K extends keyof Omit<Settings, 'has_password'>>(key: K, value: Settings[K]) => {
        setDraft((d) => (d ? { ...d, [key]: value } : d));
        setTest(null);
    };
    const sameRelay = !!known && !!draft && known.settings.has_password
        && known.settings.host.toLowerCase() === draft.host.trim().toLowerCase().replace(/\.$/, '')
        && known.settings.username === draft.username.trim();
    const body = () => JSON.stringify({ ...draft, host: draft?.host.trim(), username: draft?.username.trim(), password });
    const relayName = draft ? `${draft.host.trim()}:${draft.port}` : '';

    const runTest = async () => {
        if (!draft) return;
        setTesting(true);
        setTest(null);
        try {
            const res = await fetch(TEST_URL, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: body() });
            if (!res.ok) {
                const error = await readApiError(res);
                setTest({ error: error.code === 'MAIL_RELAY_PASSWORD_REQUIRED' ? t('mcfg.relay.passwordRequired') : t('mcfg.relay.invalid', { detail: apiErrorText(error, t) }) });
                return;
            }
            setTest({ answer: await res.json() as TestAnswer });
        } catch {
            setTest({ error: t('mcfg.relay.testUnknown') });
        } finally {
            setTesting(false);
        }
    };

    const save = async () => {
        if (!draft) return;
        await confirmAction({
            title: t('mcfg.relay.title'),
            message: draft.enabled ? t('mcfg.relay.confirmOn', { relay: relayName }) : t('mcfg.relay.confirmOff'),
            confirmLabel: t('mcfg.relay.save'),
            tone: draft.enabled ? 'default' : 'danger',
            progressTitle: t('mcfg.relay.progress'),
            action: async (report) => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'mailRelay', ['check!', 'save', 'apply']), t, 'mailRelay',
                        (headers) => fetch(MAIL_RELAY_URL, { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: body() }));
                    if (!res.ok) {
                        const error = await readApiError(res);
                        if (error.code === 'MAIL_RELAY_PASSWORD_REQUIRED') return { state: 'blocked', reason: t('mcfg.relay.passwordRequired') };
                        return { state: 'failed', reason: t('mcfg.relay.invalid', { detail: apiErrorText(error, t) }) };
                    }
                    const answer = await res.json() as { unknown?: string; result?: { applied?: boolean; code?: string; error?: string } };
                    if (answer.unknown) return { state: 'unknown', reason: t('mcfg.relay.state.lost') };
                    if (answer.result?.applied) {
                        setPassword('');
                        return { state: 'done', message: draft.enabled ? t('mcfg.relay.done', { relay: relayName }) : t('mcfg.relay.doneOff') };
                    }
                    switch (answer.result?.code) {
                        case 'MAIL_RELAY_SASL_MISSING': return { state: 'blocked', reason: t('mcfg.relay.state.sasl') };
                        case 'MAIL_RELAY_OWNER_CONFIG': return { state: 'blocked', reason: t('mcfg.relay.state.owner', { detail: answer.result.error ?? '' }) };
                        case 'MAIL_RELAY_EDITED_CONFIG': return { state: 'blocked', reason: t('mcfg.relay.state.edited', { detail: answer.result.error ?? '' }) };
                        case 'MAIL_RELAY_BUSY': return { state: 'blocked', reason: t('mcfg.relay.busy') };
                    }
                    return { state: 'failed', reason: t('mcfg.relay.failed', { detail: answer.result?.error ?? '' }) };
                } catch {
                    return { state: 'unknown', reason: t('common.resultUnknown') };
                } finally {
                    refreshRemote(MAIL_RELAY_URL);
                }
            },
        });
    };

    const canSend = !!draft && (!draft.enabled || (draft.host.trim() !== '' && draft.username.trim() !== '' && (password !== '' || sameRelay)));
    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-5" data-mail-relay>
            <h3 className="mb-1 text-sm font-semibold text-fg">{t('mcfg.relay.title')}</h3>
            <p className="mb-4 max-w-[75ch] text-sm text-fg-muted">{t('mcfg.relay.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('mcfg.relay.reading')} failed={t('mcfg.relay.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value: view, stale }) => {
                    const words = stateWords(view);
                    const { icon: Icon, surface } = tones[words.tone];
                    const saslMissing = view.server?.postfix && !view.server.sasl_ready && words.key !== 'mcfg.relay.state.sasl';
                    return (
                        <>
                            <div role="status" data-mail-relay-state={words.key} className={`mb-4 flex flex-wrap items-start gap-2 rounded-lg border p-3 text-sm text-fg ${surface}`}>
                                <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                                <p className="min-w-0 max-w-[75ch] flex-1 break-words">{t(words.key, words.vars)}</p>
                            </div>
                            {draft && (
                                <>
                                    <label className="flex min-h-[1.75rem] items-center gap-2 text-sm text-fg">
                                        <input type="checkbox" className="h-4 w-4" checked={draft.enabled} onChange={(event) => set('enabled', event.target.checked)} />
                                        {t('mcfg.relay.enabled')}
                                    </label>
                                    {draft.enabled && saslMissing && (
                                        <p className="mt-2 max-w-[75ch] text-xs text-danger" data-mail-relay-sasl>{t('mcfg.relay.state.sasl')}</p>
                                    )}
                                    <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
                                        <label className="block text-sm sm:col-span-2">
                                            <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.relay.host')}</span>
                                            <input type="text" inputMode="url" autoComplete="off" spellCheck={false} placeholder="smtp.example.com" value={draft.host} disabled={!draft.enabled}
                                                onChange={(event) => set('host', event.target.value)} className={inputClass} />
                                        </label>
                                        <label className="block text-sm">
                                            <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.relay.port')}</span>
                                            <select className={inputClass} value={draft.port} disabled={!draft.enabled} onChange={(event) => set('port', Number(event.target.value))}>
                                                {PORTS.map((port) => <option key={port} value={port}>{t(`mcfg.relay.port${port}` as TranslationKey)}</option>)}
                                            </select>
                                        </label>
                                        <label className="block text-sm">
                                            <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.relay.username')}</span>
                                            <input type="text" autoComplete="off" spellCheck={false} value={draft.username} disabled={!draft.enabled}
                                                onChange={(event) => set('username', event.target.value)} className={inputClass} />
                                        </label>
                                        <div className="block text-sm sm:col-span-2">
                                            <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.relay.password')}</span>
                                            <PasswordInput value={password} onChange={(next) => { setPassword(next); setTest(null); }} autoComplete="new-password" disabled={!draft.enabled}
                                                aria-label={t('mcfg.relay.password')} />
                                            <span className="mt-1 block text-xs text-fg-subtle">{sameRelay ? t('mcfg.relay.passwordKept') : t('mcfg.relay.passwordNew')}</span>
                                        </div>
                                    </div>
                                    <p className="mt-2 max-w-[75ch] text-xs text-fg-subtle">{t('mcfg.relay.limitsNote')}</p>
                                    {test && (
                                        <div role="status" data-mail-relay-test={test.answer?.ok ? 'ok' : 'failed'} className={`mt-3 max-w-[75ch] rounded-lg border p-3 text-sm ${test.answer?.ok ? 'border-success/40 bg-success/10' : 'border-danger/40 bg-danger/10'}`}>
                                            {test.error ?? (test.answer?.ok
                                                ? `${t('mcfg.relay.testOk', { relay: relayName })} ${test.answer.verified ? t('mcfg.relay.testVerified') : t('mcfg.relay.testUnverified')}`
                                                : t('mcfg.relay.testFailed', {
                                                    stage: t(`mcfg.relay.stage.${['connect', 'greeting', 'tls', 'auth'].includes(test.answer?.stage ?? '') ? test.answer?.stage : 'connect'}` as TranslationKey),
                                                    detail: test.answer?.detail ?? '',
                                                }))}
                                        </div>
                                    )}
                                    <div className="mt-4 flex flex-wrap gap-2">
                                        <Button variant="primary" disabled={stale || !canSend} onClick={() => void save()}>{t('mcfg.relay.save')}</Button>
                                        {draft.enabled && (
                                            <Button variant="secondary" disabled={!canSend || testing} loading={testing} onClick={() => void runTest()}>
                                                {testing ? t('mcfg.relay.testing') : t('mcfg.relay.test')}
                                            </Button>
                                        )}
                                    </div>
                                </>
                            )}
                        </>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

export default MailRelaySettings;
