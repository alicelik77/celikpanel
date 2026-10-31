import { useEffect, useState } from 'react';
import { Network } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, RemoteGate, inputClass } from './ui';
import { readApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
import { decodePanelPortView, panelAddressOn, PANEL_PORT_URL } from '../lib/panelPort';

// The panel's own port (alpha.114, owner decision 2026-10-31). New
// installations use 1923; one made before keeps 2083 until the owner changes
// it here. A change never locks the owner out: the panel answers on both
// ports, the owner confirms from the new address, and only then the old port
// closes. Without that confirmation within ten minutes the panel stays on
// the old port by itself. The server decides every step; this card reads
// its state and polls while a change is open (a poll never starts anything).
// Panelin kendi portu. Değişiklik sahibi dışarıda bırakmaz: iki port da
// açıktır, sahip yeni adresten onaylar, ancak o zaman eski port kapanır.

const PANEL_PORT_POLL_MS = 5000;

export function PanelPortCard() {
    const { t } = useI18n();
    const { remote, reading, retry } = useRemote(PANEL_PORT_URL, decodePanelPortView);
    const [port, setPort] = useState('');
    const [busy, setBusy] = useState(false);
    const [refusal, setRefusal] = useState<{ code: string; message: string } | null>(null);
    const open = remote.state === 'known' && remote.value.change !== null && (remote.value.change.status === 'trial' || remote.value.change.status === 'confirming');

    // While a change is open the card follows it (its countdown, the
    // automatic return to the old port). Reading starts nothing.
    useEffect(() => {
        if (!open) return undefined;
        const timer = setInterval(() => void retry(), PANEL_PORT_POLL_MS);
        return () => clearInterval(timer);
    }, [open, retry]);

    const codeText = (code: string, vars: Record<string, string | number>, fallback: string) => {
        const key = `panelPort.code.${code}` as TranslationKey;
        const text = t(key, vars);
        return text === key ? fallback : text;
    };

    const send = async (body: Record<string, unknown>) => {
        if (busy) return;
        setBusy(true);
        setRefusal(null);
        try {
            const res = await fetch(PANEL_PORT_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });
            if (!res.ok) {
                const error = await readApiError(res);
                setRefusal({ code: error.code || '', message: error.message || '' });
            }
        } catch {
            // No answer: the server's state decides; it is read again below.
            setRefusal({ code: 'PANEL_PORT_NO_ANSWER', message: '' });
        } finally {
            setBusy(false);
            void retry();
        }
    };

    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-6" aria-labelledby="panel-port-title">
            <div className="mb-3 flex items-center gap-2">
                <Network className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 id="panel-port-title" className="text-base font-semibold text-fg">{t('panelPort.title')}</h2>
            </div>
            <RemoteGate
                remote={remote}
                checking={t('panelPort.checking')}
                failed={t('panelPort.unknown')}
                onRetry={() => void retry()}
                busy={reading}
            >
                {({ value: view }) => {
                    const change = view.change;
                    const vars = change
                        ? { from: change.from_port, to: change.to_port, port: view.port, minutes: Math.ceil((change.seconds_left ?? 0) / 60), address: panelAddressOn(change.to_port) }
                        : { port: view.port, from: view.port, to: Number(port) || 0, minutes: 10, address: '' };
                    return (
                        <div className="space-y-4 text-sm">
                            <p className="text-fg">{t('panelPort.current', { port: view.port })}</p>
                            {view.port === view.legacy_port && !open && (
                                <p className="max-w-[75ch] text-fg-muted">{t('panelPort.legacy', { legacy: view.legacy_port, port: view.default_port })}</p>
                            )}

                            {change?.status === 'trial' && view.request_port === change.to_port && (
                                <div role="status" className="rounded-lg border border-border bg-surface-2/50 p-4">
                                    <p className="font-semibold text-fg">{t('panelPort.trial.here.head', vars)}</p>
                                    <p className="mt-1 max-w-[75ch] text-fg-muted">{t('panelPort.trial.here.body', vars)}</p>
                                    <div className="mt-3 flex flex-wrap gap-2">
                                        <Button variant="primary" loading={busy} onClick={() => void send({ action: 'confirm', id: change.id })}>{t('panelPort.confirm', vars)}</Button>
                                        <Button disabled={busy} onClick={() => void send({ action: 'cancel', id: change.id })}>{t('panelPort.cancel', vars)}</Button>
                                    </div>
                                </div>
                            )}
                            {change?.status === 'trial' && view.request_port !== change.to_port && (
                                <div role="status" className="rounded-lg border border-border bg-surface-2/50 p-4">
                                    <p className="font-semibold text-fg">{t('panelPort.trial.head', vars)}</p>
                                    <dl className="mt-2 space-y-1 text-fg-muted">
                                        <div><dt className="inline font-medium text-fg">{t('panelPort.label.who')} </dt><dd className="inline">{t('panelPort.trial.who')}</dd></div>
                                        <div><dt className="inline font-medium text-fg">{t('panelPort.label.next')} </dt><dd className="inline">{t('panelPort.trial.next', vars)}</dd></div>
                                        <div><dt className="inline font-medium text-fg">{t('panelPort.label.then')} </dt><dd className="inline">{t('panelPort.trial.then', vars)}</dd></div>
                                    </dl>
                                    <div className="mt-3 flex flex-wrap items-center gap-2">
                                        <a href={vars.address} target="_blank" rel="noopener noreferrer" className="inline-flex items-center rounded-md border border-border-strong bg-surface px-3 py-1.5 text-sm font-medium text-fg hover:bg-surface-2">{t('panelPort.open', vars)}</a>
                                        <Button disabled={busy} onClick={() => void send({ action: 'cancel', id: change.id })}>{t('panelPort.cancel', vars)}</Button>
                                    </div>
                                    <p className="mt-2 break-all text-xs text-fg-subtle">{vars.address}</p>
                                </div>
                            )}
                            {change?.status === 'confirming' && (
                                <p role="status" className="rounded-lg border border-border bg-surface-2/50 p-4 text-fg">{t('panelPort.confirming', vars)}</p>
                            )}
                            {change && (change.status === 'confirmed' || change.status === 'rolled_back') && (
                                <div className="rounded-lg border border-border bg-surface-2/50 p-4">
                                    <p className="font-semibold text-fg">{change.status === 'confirmed' ? t('panelPort.done.head', vars) : t('panelPort.back.head', vars)}</p>
                                    <p className="mt-1 max-w-[75ch] text-fg-muted">
                                        {change.status === 'confirmed' && !change.reason_code
                                            ? t('panelPort.done.body', vars)
                                            : codeText(change.reason_code || '', vars, change.reason || '')}
                                    </p>
                                    {change.firewall_note && <p className="mt-2 max-w-[75ch] text-warning">{t('panelPort.firewallNote', vars)} {change.firewall_note}</p>}
                                </div>
                            )}

                            {refusal && (
                                <div role="alert" className="rounded-lg border border-danger/30 bg-danger/10 p-3 text-danger">
                                    {codeText(refusal.code, vars, refusal.message || t('panelPort.failed'))}
                                </div>
                            )}

                            {!open && view.blocked && (
                                <div className="rounded-lg border border-border bg-surface-2/50 p-4">
                                    <p className="font-semibold text-fg">{t('panelPort.blocked.head')}</p>
                                    <p className="mt-1 max-w-[75ch] text-fg-muted">{codeText(view.blocked.code, vars, view.blocked.reason)}</p>
                                </div>
                            )}

                            {!open && !view.blocked && (
                                <form
                                    className="space-y-3"
                                    onSubmit={(event) => {
                                        event.preventDefault();
                                        const chosen = Number(port);
                                        if (!Number.isInteger(chosen)) return;
                                        void send({ action: 'start', port: chosen });
                                    }}
                                >
                                    <p className="max-w-[75ch] text-fg-muted">{t('panelPort.how', { minutes: 10 })}</p>
                                    <label className="block max-w-xs" htmlFor="panel-port-input">
                                        <span className="mb-1 block font-medium text-fg">{t('panelPort.input')}</span>
                                        <input id="panel-port-input" inputMode="numeric" pattern="[0-9]*" value={port} onChange={(event) => setPort(event.target.value.replace(/[^0-9]/g, '').slice(0, 5))} placeholder={String(view.default_port)} className={inputClass} />
                                    </label>
                                    <p className="max-w-[75ch] text-xs text-fg-subtle">{t('panelPort.provider')}</p>
                                    <Button type="submit" variant="primary" loading={busy} disabled={!port}>{t('panelPort.start')}</Button>
                                </form>
                            )}
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}
