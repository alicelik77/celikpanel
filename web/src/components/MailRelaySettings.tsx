import { useMemo, useState } from 'react';
import { AlertTriangle, CheckCircle2, Info, ShieldAlert } from 'lucide-react';
import { useMailConfigWords } from '../i18n/mailConfig';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction } from '../lib/confirmAction';
import { lastKnown, refreshRemote, useRemote } from '../lib/remote';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { PasswordInput } from './PasswordInput';
import { Button, RemoteGate, inputClass } from './ui';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import type { SaveOutcome } from './kit/save/saveRun';
import { useSaveWords } from './kit/save/words';

// Sending outgoing mail through another server (alpha.130, smarthost),
// administrator only, server-wide. The state comes first with who acts (the
// owner's own relay kept, the login modules missing, a save the server did
// not take); then the settings. The password is never shown: an empty field
// keeps the saved one for the same relay and user name. The test signs in and
// out again and sends nothing.
//
// alpha.133: the one way of saving (components/kit/save). The fields are a
// draft over the server's last answer; a changed field is marked; Save, in
// the shared bar, is on only when something changed and the form can be sent;
// Discard gives the changes up; leaving with changes asks. "Test the
// connection" is an action: it tries the typed values, saves nothing, and says
// in plain words which step failed (reaching the server, encryption, signing
// in). A save whose answer was lost holds Save until the state is read again.
//
// Postanın başka bir sunucu üzerinden gönderilmesi: önce durum ve kimin
// yapacağı, sonra ayarlar. Parola gösterilmez; deneme ileti göndermez ve
// hiçbir şeyi kaydetmez. Alanlar bir taslaktır; yalnız Kaydet yazar.

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
/** The form: the saved settings and a new password ('' keeps the saved one; nothing is read back). */
interface Form { enabled: boolean; host: string; port: number; username: string; password: string }
/** The steps a sign-in test can stop at (cmd/panel/mail_relay.go testMailRelaySignIn). */
const FAILED_STAGES = ['connect', 'greeting', 'tls', 'auth'] as const;

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
    const say = useSaveWords();
    const read = useRemote(MAIL_RELAY_URL, decodeView);
    const last = lastKnown(read.remote)?.value;
    // The server's last answer as the form; a new answer is a new object, so
    // the draft sees the server change under it. The password is always ''.
    const known = useMemo<Form | null>(() => (last
        ? { enabled: last.settings.enabled, host: last.settings.host, port: last.settings.port || 587, username: last.settings.username, password: '' }
        : null), [last]);
    const draft = useDraft(known);
    const [testing, setTesting] = useState(false);
    const [test, setTest] = useState<{ answer?: TestAnswer; error?: string; relay: string } | null>(null);
    const form = draft.values;
    const set = <K extends keyof Form>(key: K, value: Form[K]) => {
        draft.set(key, value);
        setTest(null);
    };
    const sameRelay = (values: Form) => !!last && last.settings.has_password
        && last.settings.host.toLowerCase() === values.host.trim().toLowerCase().replace(/\.$/, '')
        && last.settings.username === values.username.trim();
    const body = (values: Form) => JSON.stringify({ enabled: values.enabled, host: values.host.trim(), port: values.port, username: values.username.trim(), password: values.password });
    const relayOf = (values: Form) => `${values.host.trim()}:${values.port}`;
    // Why the typed values cannot be sent or tried, in the form's own words.
    const missing = (values: Form) => {
        if (!values.enabled) return undefined;
        if (values.host.trim() === '' || values.username.trim() === '') return t('mcfg.relay.missing');
        if (values.password === '' && !sameRelay(values)) return t('mcfg.relay.passwordRequired');
        return undefined;
    };

    // The test tries what is typed; it saves nothing and sends no message.
    const runTest = async () => {
        if (!form || missing(form)) return;
        const relay = relayOf(form);
        setTesting(true);
        setTest(null);
        try {
            const res = await fetch(TEST_URL, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: body(form) });
            if (!res.ok) {
                const error = await readApiError(res);
                setTest({ relay, error: error.code === 'MAIL_RELAY_PASSWORD_REQUIRED' ? t('mcfg.relay.passwordRequired') : t('mcfg.relay.invalid', { detail: apiErrorText(error, t) }) });
                return;
            }
            setTest({ relay, answer: await res.json() as TestAnswer });
        } catch {
            setTest({ relay, error: t('mcfg.relay.testUnknown') });
        } finally {
            setTesting(false);
        }
    };

    const save = () => void draft.save(async (_changes, values) => {
        const relayName = relayOf(values);
        const end: { outcome: SaveOutcome } = { outcome: 'refused' };
        await confirmAction({
            title: t('mcfg.relay.title'),
            message: values.enabled ? t('mcfg.relay.confirmOn', { relay: relayName }) : t('mcfg.relay.confirmOff'),
            confirmLabel: t('mcfg.relay.save'),
            tone: values.enabled ? 'default' : 'danger',
            progressTitle: t('mcfg.relay.progress'),
            action: async (report) => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'mailRelay', ['check!', 'save', 'apply']), t, 'mailRelay',
                        (headers) => fetch(MAIL_RELAY_URL, { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: body(values) }));
                    if (!res.ok) {
                        const error = await readApiError(res);
                        if (error.code === 'MAIL_RELAY_PASSWORD_REQUIRED') return { state: 'blocked', reason: t('mcfg.relay.passwordRequired') };
                        return { state: 'failed', reason: t('mcfg.relay.invalid', { detail: apiErrorText(error, t) }) };
                    }
                    const answer = await res.json() as { unknown?: string; result?: { applied?: boolean; code?: string; error?: string } };
                    if (answer.unknown) {
                        end.outcome = 'unknown';
                        return { state: 'unknown', reason: t('mcfg.relay.state.lost') };
                    }
                    if (answer.result?.applied) {
                        end.outcome = 'saved';
                        setTest(null);
                        return { state: 'done', message: values.enabled ? t('mcfg.relay.done', { relay: relayName }) : t('mcfg.relay.doneOff') };
                    }
                    switch (answer.result?.code) {
                        case 'MAIL_RELAY_SASL_MISSING': return { state: 'blocked', reason: t('mcfg.relay.state.sasl') };
                        case 'MAIL_RELAY_OWNER_CONFIG': return { state: 'blocked', reason: t('mcfg.relay.state.owner', { detail: answer.result.error ?? '' }) };
                        case 'MAIL_RELAY_EDITED_CONFIG': return { state: 'blocked', reason: t('mcfg.relay.state.edited', { detail: answer.result.error ?? '' }) };
                        case 'MAIL_RELAY_BUSY': return { state: 'blocked', reason: t('mcfg.relay.busy') };
                    }
                    return { state: 'failed', reason: t('mcfg.relay.failed', { detail: answer.result?.error ?? '' }) };
                } catch {
                    end.outcome = 'unknown';
                    return { state: 'unknown', reason: t('common.resultUnknown') };
                } finally {
                    refreshRemote(MAIL_RELAY_URL);
                }
            },
        });
        return end.outcome;
    });

    const marked = (field: keyof Form) => (draft.isChanged(field) ? `-mx-2 rounded-r px-2 ${changedRowClass}` : '');
    const moved = (field: keyof Form, words: string) => (draft.isMoved(field) ? <ServerMovedNote value={words} onTake={() => draft.take(field)} /> : null);
    const portWords = (port: number) => t(`mcfg.relay.port${port}` as TranslationKey);
    // A test's outcome in plain words: which step failed, for the relay that was tried.
    const testWords = (shown: NonNullable<typeof test>) => {
        if (shown.error) return shown.error;
        const answer = shown.answer!;
        if (answer.ok) return `${t('mcfg.relay.testOk', { relay: shown.relay })} ${answer.verified ? t('mcfg.relay.testVerified') : t('mcfg.relay.testUnverified')}`;
        const stage = FAILED_STAGES.find((name) => name === answer.stage) ?? 'connect';
        return t(`mcfg.relay.fail.${stage}` as TranslationKey, { relay: shown.relay });
    };
    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-5" data-mail-relay>
            <h3 className="mb-1 text-sm font-semibold text-fg">{t('mcfg.relay.title')}</h3>
            <p className="mb-4 max-w-[75ch] text-sm text-fg-muted">{t('mcfg.relay.desc')}</p>
            <RemoteGate remote={read.remote} checking={t('mcfg.relay.reading')} failed={t('mcfg.relay.readFailed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value: view, stale }) => {
                    const words = stateWords(view);
                    const { icon: Icon, surface } = tones[words.tone];
                    const saslMissing = view.server?.postfix && !view.server.sasl_ready && words.key !== 'mcfg.relay.state.sasl';
                    const base = draft.base;
                    if (!form || !base) return null;
                    const locked = draft.saving;
                    const reason = missing(form);
                    return (
                        <>
                            <div role="status" data-mail-relay-state={words.key} className={`mb-4 flex flex-wrap items-start gap-2 rounded-lg border p-3 text-sm text-fg ${surface}`}>
                                <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                                <p className="min-w-0 max-w-[75ch] flex-1 break-words">{t(words.key, words.vars)}</p>
                            </div>
                            <div className={marked('enabled')}>
                                <label className="flex min-h-[1.75rem] flex-wrap items-center gap-2 text-sm text-fg">
                                    <input type="checkbox" className="h-4 w-4" checked={form.enabled} disabled={locked} onChange={(event) => set('enabled', event.target.checked)} />
                                    {t('mcfg.relay.enabled')}
                                    {draft.isChanged('enabled') && <ChangedMark switched={form.enabled} />}
                                </label>
                                {moved('enabled', say(base.enabled ? 'on' : 'off'))}
                            </div>
                            {form.enabled && saslMissing && (
                                <p className="mt-2 max-w-[75ch] text-xs text-danger" data-mail-relay-sasl>{t('mcfg.relay.state.sasl')}</p>
                            )}
                            <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
                                <div className={`text-sm sm:col-span-2 ${marked('host')}`}>
                                    <label className="block">
                                        <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.relay.host')}{draft.isChanged('host') && <ChangedMark />}</span>
                                        <input type="text" inputMode="url" autoComplete="off" spellCheck={false} placeholder="smtp.example.com" value={form.host} disabled={!form.enabled || locked}
                                            onChange={(event) => set('host', event.target.value)} className={`${inputClass} field-lg`} data-mail-relay-field="host" />
                                    </label>
                                    {moved('host', base.host)}
                                </div>
                                <div className={`text-sm ${marked('port')}`}>
                                    <label className="block">
                                        <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.relay.port')}{draft.isChanged('port') && <ChangedMark from={portWords(base.port)} to={portWords(form.port)} />}</span>
                                        <select className={`${inputClass} field-md`} value={form.port} disabled={!form.enabled || locked} onChange={(event) => set('port', Number(event.target.value))} data-mail-relay-field="port">
                                            {PORTS.map((port) => <option key={port} value={port}>{portWords(port)}</option>)}
                                        </select>
                                    </label>
                                    {moved('port', portWords(base.port))}
                                </div>
                                <div className={`text-sm ${marked('username')}`}>
                                    <label className="block">
                                        <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.relay.username')}{draft.isChanged('username') && <ChangedMark />}</span>
                                        <input type="text" autoComplete="off" spellCheck={false} value={form.username} disabled={!form.enabled || locked}
                                            onChange={(event) => set('username', event.target.value)} className={`${inputClass} field-lg`} data-mail-relay-field="username" />
                                    </label>
                                    {moved('username', base.username)}
                                </div>
                                <div className={`text-sm sm:col-span-2 ${marked('password')}`}>
                                    <span className="mb-1 block text-xs text-fg-muted">{t('mcfg.relay.password')}{draft.isChanged('password') && <ChangedMark />}</span>
                                    {/* Only what is typed here is shown; a saved password is never read back. */}
                                    <PasswordInput value={draft.isChanged('password') ? form.password : ''} onChange={(next) => set('password', next)} autoComplete="new-password" disabled={!form.enabled || locked}
                                        aria-label={t('mcfg.relay.password')} />
                                    <span className="mt-1 block text-xs text-fg-subtle">{sameRelay(form) ? t('mcfg.relay.passwordKept') : t('mcfg.relay.passwordNew')}</span>
                                </div>
                            </div>
                            <p className="mt-2 max-w-[75ch] text-xs text-fg-subtle">{t('mcfg.relay.limitsNote')}</p>
                            {form.enabled && (
                                <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2">
                                    <Button variant="secondary" disabled={!!reason || testing || locked} loading={testing} onClick={() => void runTest()} data-mail-relay-test-button="">
                                        {testing ? t('mcfg.relay.testing') : t('mcfg.relay.test')}
                                    </Button>
                                    <span className="min-w-0 max-w-[60ch] text-xs text-fg-subtle">{t('mcfg.relay.testHint')}</span>
                                </div>
                            )}
                            {test && (
                                <div role="status" data-mail-relay-test={test.answer?.ok ? 'ok' : 'failed'} className={`mt-3 max-w-[75ch] rounded-lg border p-3 text-sm text-fg ${test.answer?.ok ? 'border-success/40 bg-success/10' : 'border-danger/40 bg-danger/10'}`}>
                                    <p>{testWords(test)}{draft.count > 0 ? ` ${t('mcfg.relay.testUnsaved')}` : ''}</p>
                                    {test.answer && !test.answer.ok && test.answer.detail && (
                                        <p className="mt-1 break-words text-xs text-fg-muted">{t('mcfg.relay.testDetail', { detail: test.answer.detail })}</p>
                                    )}
                                </div>
                            )}
                            <SaveBar
                                className="-mx-5 px-5"
                                count={draft.count}
                                saving={draft.saving}
                                onSave={save}
                                onDiscard={() => { draft.reset(); setTest(null); }}
                                blockedBy={stale ? say('stale') : reason}
                                unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: read.reading, onCheck: () => void read.retry() } : undefined}
                            />
                        </>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

export default MailRelaySettings;
