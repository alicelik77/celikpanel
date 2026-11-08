import { useState } from 'react';
import { KeyRound } from 'lucide-react';
import { useSSHPasswordWords, type SSHPasswordWordKey } from '../i18n/sshPassword';
import type { TranslationKey } from '../i18n/en';
import { confirmAction, type OperationEnd, type OperationReport } from '../lib/confirmAction';
import { sendWithSteps, type StepLabel } from '../lib/requestProgress';
import { useRemote } from '../lib/remote';
import { Button, KeepWords, RemoteGate } from './ui';
import { LifeLine, type Tone } from './LifeLine';
import { CopyCommand } from './CopyCommand';

// Security → SSH password sign-in (alpha.130, owner approval of 8 Nov 2026:
// "add the button; the owner presses it"). What sshd itself says (`sshd -T`),
// the file that decides it, and the accounts that can still sign in with a
// key. Turning passwords off asks first, with the instruction to keep the
// current session open and test a key login; the progress window follows the
// server's steps and the result is said as it came: done, refused (nothing
// changed), taken back, or not known. Reads change nothing; the card reads
// again by itself after every change (cmd/panel/ssh_password_login_handlers.go).
//
// Güvenlik → SSH parola ile giriş: sshd'nin kendi söylediği durum, karar veren
// dosya ve anahtarla girebilen hesaplar. Kapatma önce sorar; sonuç geldiği
// gibi söylenir.

export const SSH_PASSWORD_URL = '/api/v1/security/ssh-password-login';

export interface SSHPasswordStatus {
    state: 'read' | 'unsupported' | 'unreadable';
    reason?: string;
    detail?: string;
    unit?: string;
    unit_active: boolean;
    password_authentication: string;
    kbd_interactive_authentication: string;
    permit_root_login: string;
    pubkey_authentication: string;
    deciders: { setting: string; value?: string; file?: string; line?: number; default?: boolean }[];
    drop_in: { path: string; state: 'absent' | 'panel' | 'edited'; sha256?: string };
    key_accounts: { name: string; root?: boolean; keys: number }[];
    can_turn_off: boolean;
    can_turn_on: boolean;
}

export function decodeSSHPasswordStatus(raw: unknown): SSHPasswordStatus {
    const body = raw as Partial<SSHPasswordStatus> | null;
    if (!body || typeof body !== 'object' || !['read', 'unsupported', 'unreadable'].includes(body.state as string)
        || !Array.isArray(body.deciders) || !Array.isArray(body.key_accounts) || typeof body.can_turn_off !== 'boolean') {
        throw new Error('shape');
    }
    return body as SSHPasswordStatus;
}

const BLOCKED = new Set(['include_order', 'match_block', 'no_key_account', 'pubkey_off', 'drop_in_edited', 'drop_in_dir_missing', 'config_unreadable']);
const REASONS = new Set(['validate_failed', 'not_effective', 'reload_failed', 'confirm_failed', 'write_failed', 'remove_failed', 'changed', 'config_invalid']);

type T = ReturnType<typeof useSSHPasswordWords>['t'];

/** The blocked reason in plain words, with its fact. */
export function blockedText(status: Pick<SSHPasswordStatus, 'reason' | 'detail'>, t: T): string {
    const reason = status.reason ?? '';
    if (reason === 'include_order' && !status.detail) return t('sshpw.blocked.include_orderNone');
    if (BLOCKED.has(reason)) return t(`sshpw.blocked.${reason}` as SSHPasswordWordKey, { detail: status.detail ?? '' });
    return t('sshpw.blocked.other');
}

function reasonText(code: string, t: T): string {
    if (REASONS.has(code)) return t(`sshpw.reason.${code}` as SSHPasswordWordKey);
    if (code === 'MUTATION_REFUSED' || code.endsWith('BUSY')) return t('sshpw.reason.busy');
    if (BLOCKED.has(code)) return blockedText({ reason: code }, t);
    return t('sshpw.reason.other');
}

export function passwordState(status: SSHPasswordStatus): 'on' | 'off' | 'partly' | 'unknown' {
    const password = status.password_authentication;
    const kbd = status.kbd_interactive_authentication;
    if (password === 'no' && kbd === 'no') return 'off';
    if (password === 'yes' && kbd !== 'unknown') return 'on';
    if (password === 'no' && kbd === 'yes') return 'partly';
    return 'unknown';
}

function keyList(status: SSHPasswordStatus, t: T): string {
    return status.key_accounts.map((account) => t(account.keys === 1 ? 'sshpw.keyCount' : 'sshpw.keyCountMany', { name: account.name, n: account.keys })).join(', ');
}

export function SSHPasswordLoginCard() {
    const { t } = useSSHPasswordWords();
    const handle = useRemote(SSH_PASSWORD_URL, decodeSSHPasswordStatus);
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-security-section="ssh-password">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1 basis-64">
                    <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                        <KeyRound className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                        {t('sshpw.title')}
                    </h2>
                    <p className="mt-0.5 text-sm text-fg-muted">{t('sshpw.desc')}</p>
                </div>
                {handle.remote.state !== 'loading' && (
                    <Button onClick={() => void handle.retry()} loading={handle.reading} disabled={handle.reading} className="max-sm:min-h-11">
                        {t('sshpw.again')}
                    </Button>
                )}
            </div>
            <div className="mt-4">
                <RemoteGate remote={handle.remote} checking={t('sshpw.checking')} failed={t('sshpw.failed')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <SSHPasswordFacts status={value} onChanged={() => void handle.retry()} />}
                </RemoteGate>
            </div>
        </section>
    );
}

function SSHPasswordFacts({ status, onChanged }: { status: SSHPasswordStatus; onChanged: () => void }) {
    const { t } = useSSHPasswordWords();
    const [result, setResult] = useState<{ tone: Tone; text: string } | null>(null);
    if (status.state === 'unsupported') return <LifeLine tone="info" attr="ssh-unsupported">{t('sshpw.unsupported')}</LifeLine>;
    if (status.state === 'unreadable') {
        return <LifeLine tone="warn" attr="ssh-unreadable"><KeepWords text={t('sshpw.unknown', { detail: status.detail ?? '' })} /></LifeLine>;
    }
    const state = passwordState(status);
    const tone: Tone = state === 'off' ? 'ok' : state === 'unknown' ? 'info' : 'warn';
    const stateText = state === 'off' ? t('sshpw.off')
        : state === 'on' ? t('sshpw.on')
            : state === 'partly' ? t('sshpw.partly', { method: t('sshpw.method.kbd') })
                : t('sshpw.unknown', { detail: '' });
    const decider = status.deciders.find((d) => d.setting === (status.password_authentication === 'yes' || state !== 'partly' ? 'passwordauthentication' : 'kbdinteractiveauthentication'));
    const root = status.permit_root_login;
    const rootKey: SSHPasswordWordKey | null = root === 'yes' ? 'sshpw.root.yes'
        : root === 'prohibit-password' || root === 'without-password' ? 'sshpw.root.key'
            : root === 'no' ? 'sshpw.root.no' : root === 'forced-commands-only' ? 'sshpw.root.forced' : null;
    const accounts = keyList(status, t);
    const path = status.drop_in.path;
    const unit = status.unit || 'ssh';
    const steps: StepLabel[] = [
        { id: 'check', label: t('sshpw.step.check'), check: true },
        { id: 'apply', label: t('sshpw.step.apply') },
        { id: 'verify', label: t('sshpw.step.verify') },
    ];

    const change = async (action: 'turn_off' | 'turn_on', report: OperationReport): Promise<OperationEnd> => {
        setResult(null);
        try {
            const res = await sendWithSteps(report, steps, t as (key: TranslationKey, vars?: Record<string, string | number>) => string, 'security.ssh-password', async (headers): Promise<Response | null> => {
                try {
                    return await fetch(SSH_PASSWORD_URL, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json', ...headers },
                        body: JSON.stringify({ action, expected_drop_in_sha256: status.drop_in.sha256 ?? '' }),
                    });
                } catch {
                    return null;
                }
            });
            if (!res) {
                setResult({ tone: 'warn', text: t('sshpw.lost') });
                return { state: 'unknown', reason: t('sshpw.lost') };
            }
            type Answer = {
                outcome?: string; reason?: string; code?: string; detail?: string; error?: string;
                applies_at_next_start?: boolean; still_off_by?: string; password_authentication?: string;
            };
            let body: Answer;
            try {
                body = await res.json() as Answer;
            } catch {
                // An answer that cannot be read: what the change came to is not known.
                setResult({ tone: 'warn', text: t('sshpw.lost') });
                return { state: 'unknown', reason: t('sshpw.lost') };
            }
            const detail = body.detail ? ` (${body.detail})` : '';
            if (res.ok) {
                let text: string;
                if (body.outcome === 'unchanged') text = t('sshpw.done.unchanged', { state: action === 'turn_off' ? t('sshpw.off') : t('sshpw.on') });
                else if (action === 'turn_off') text = body.applies_at_next_start ? t('sshpw.done.offNextStart') : t('sshpw.done.off');
                else text = body.still_off_by ? t('sshpw.done.onStillOff', { file: body.still_off_by }) : t('sshpw.done.on');
                setResult({ tone: 'ok', text });
                return { state: 'done', message: text };
            }
            if (res.status === 502) {
                setResult({ tone: 'warn', text: t('sshpw.lost') });
                return { state: 'unknown', reason: t('sshpw.lost') };
            }
            const reason = reasonText(body.reason || body.code || '', t);
            if (body.outcome === 'rollback_failed') {
                const text = t('sshpw.rollbackFailed', { reason, detail, path });
                setResult({ tone: 'bad', text });
                return { state: 'failed', reason: text };
            }
            if (body.outcome === 'rolled_back') {
                const text = t('sshpw.rolledBack', { reason, detail });
                setResult({ tone: 'bad', text });
                return { state: 'failed', reason: text };
            }
            const text = t('sshpw.refused', { reason, detail });
            setResult({ tone: 'warn', text });
            return { state: 'blocked', reason: text };
        } finally {
            onChanged();
        }
    };

    return (
        <div className="space-y-3" data-ssh-password={state}>
            <LifeLine tone={tone} attr="ssh-state"><KeepWords text={stateText} /></LifeLine>
            {decider && (
                <p className="pl-6 text-sm text-fg-muted" data-ssh-decider="">
                    <KeepWords text={decider.default ? t('sshpw.decidedDefault') : t('sshpw.decidedBy', { file: decider.file ?? '', line: decider.line ?? 0 })} />
                </p>
            )}
            {rootKey && <p className="pl-6 text-sm text-fg-muted" data-ssh-root={root}>{t(rootKey)}</p>}
            <p className="pl-6 text-sm text-fg-muted" data-ssh-keys={status.key_accounts.length}>
                {status.key_accounts.length > 0 ? t('sshpw.keys', { list: accounts }) : t('sshpw.keysNone')}
            </p>
            {status.drop_in.state === 'panel' && <p className="pl-6 text-sm text-fg-muted"><KeepWords text={t('sshpw.dropIn.panel', { path })} /></p>}
            {state !== 'off' && !status.can_turn_off && status.reason && status.reason !== 'already_off' && (
                <LifeLine tone="warn" attr={`ssh-blocked-${status.reason}`}><KeepWords text={blockedText(status, t)} /></LifeLine>
            )}
            <div className="flex flex-wrap gap-2 pl-6">
                {state !== 'off' && (
                    <Button
                        variant="primary"
                        disabled={!status.can_turn_off}
                        className="max-sm:min-h-11"
                        data-ssh-turn-off=""
                        onClick={() => void confirmAction({
                            title: t('sshpw.confirmTitle'),
                            message: t('sshpw.confirmMessage', { accounts: accounts || '—', path }),
                            confirmLabel: t('sshpw.confirm'),
                            tone: 'danger',
                            progressTitle: t('sshpw.progress'),
                            action: (report) => change('turn_off', report),
                        })}
                    >
                        {t('sshpw.turnOff')}
                    </Button>
                )}
                {status.can_turn_on && (
                    <Button
                        className="max-sm:min-h-11"
                        data-ssh-turn-on=""
                        onClick={() => void confirmAction({
                            title: t('sshpw.onConfirmTitle'),
                            message: t('sshpw.onConfirmMessage', { path }),
                            confirmLabel: t('sshpw.onConfirm'),
                            progressTitle: t('sshpw.onProgress'),
                            action: (report) => change('turn_on', report),
                        })}
                    >
                        {t('sshpw.turnOn')}
                    </Button>
                )}
            </div>
            {result && <LifeLine tone={result.tone} attr="ssh-result"><KeepWords text={result.text} /></LifeLine>}
            {status.drop_in.state === 'panel' && (
                <div className="pl-6" data-ssh-without-panel="">
                    <p className="mb-1 text-sm text-fg-muted">{t('sshpw.withoutPanel')}</p>
                    <CopyCommand command={`sudo rm ${path} && sudo systemctl reload ${unit}`} />
                </div>
            )}
        </div>
    );
}
