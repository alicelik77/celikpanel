// Server → Maintenance → Restarts and reboot (v0.1.0-alpha.107, D-038 §10
// and §3). A patch is in effect only after the services that use it restart;
// a kernel only after a reboot. This card says which services still run
// replaced code (the operating system's needrestart where it is installed,
// the processes' own memory maps otherwise) and whether the server waits for
// a reboot, each restart with what it interrupts, said before it starts. A
// component's restart goes through its own restart; anything else gets the
// administrator's command. Nothing is restarted by itself.
//
// Bakım → Yeniden başlatmalar: hangi servisler eski kodu çalıştırıyor,
// sunucu yeniden açılış bekliyor mu; her yeniden başlatma neyi keseceğini
// önce söyler. Hiçbir şey kendiliğinden yeniden başlatılmaz.
import { useState } from 'react';
import { RotateCw } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, KeepWords, RemoteGate } from './ui';
import { useRemote } from '../lib/remote';
import { LifeLine, type Tone } from './LifeLine';

export interface RestartNeeds {
    source?: string;
    services: { unit: string; component?: string; interruption: string; libraries?: string[] }[];
    services_known: boolean;
    reboot_required: boolean;
    reboot_known: boolean;
    reboot_reason?: string;
    reboot_packages?: string[];
    running_kernel?: string;
    expected_kernel?: string;
    needrestart_auto_restart?: boolean;
    truncated?: boolean;
}

export function decodeRestartNeeds(raw: unknown): RestartNeeds {
    const body = raw as { state?: string; needs?: RestartNeeds } | null;
    if (!body || body.state !== 'read' || !body.needs || !Array.isArray(body.needs.services) || typeof body.needs.reboot_required !== 'boolean') {
        throw new Error('shape');
    }
    return body.needs;
}

const INTERRUPTIONS = new Set(['web', 'php', 'database', 'mail', 'dns', 'panel', 'ssh', 'other']);
// DNS engines change only through their own workflow (D-020), and the panel
// itself is restarted by the owner: those get the command.
const PANEL_RESTARTS = (item: RestartNeeds['services'][number]) => Boolean(item.component) && item.interruption !== 'dns' && item.interruption !== 'panel';

export function RestartsCard() {
    const { t } = useI18n();
    const handle = useRemote('/api/v1/maintenance/restarts', decodeRestartNeeds);
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-maintenance-section="restarts">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1 basis-64">
                    <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                        <RotateCw className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                        {t('life.restarts.title')}
                    </h2>
                    <p className="mt-0.5 text-sm text-fg-muted">{t('life.restarts.desc')}</p>
                </div>
                {handle.remote.state !== 'loading' && (
                    <Button onClick={() => void handle.retry()} loading={handle.reading} disabled={handle.reading} className="max-sm:min-h-11">
                        {t('life.restarts.again')}
                    </Button>
                )}
            </div>
            <div className="mt-4">
                <RemoteGate remote={handle.remote} checking={t('life.restarts.checking')} failed={t('life.restarts.unknown')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <RestartFacts value={value} onChanged={() => void handle.retry()} />}
                </RemoteGate>
            </div>
        </section>
    );
}

function RestartFacts({ value, onChanged }: { value: RestartNeeds; onChanged: () => void }) {
    const { t } = useI18n();
    const [confirming, setConfirming] = useState<RestartNeeds['services'][number] | null>(null);
    const [busy, setBusy] = useState(false);
    const [result, setResult] = useState<{ tone: Tone; text: string } | null>(null);
    const restart = async (unit: string) => {
        setBusy(true);
        setResult(null);
        try {
            let res: Response;
            try {
                res = await fetch('/api/v1/service/action', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name: unit, action: 'restart' }),
                });
            } catch {
                // No answer: whether it restarted is not known; nothing is sent twice.
                setResult({ tone: 'info', text: t('life.restarts.resultUnknown', { unit }) });
                return;
            }
            if (res.ok) {
                setResult({ tone: 'ok', text: t('life.restarts.done', { unit }) });
            } else {
                const raw = await res.text();
                let reason = raw.slice(0, 200);
                try {
                    reason = (JSON.parse(raw) as { error?: string }).error ?? reason;
                } catch (error) {
                    reason = reason || String(error);
                }
                setResult({ tone: 'bad', text: t('life.restarts.failed', { unit, reason }) });
            }
        } finally {
            setBusy(false);
            setConfirming(null);
            onChanged();
        }
    };
    const reboot = value.reboot_known
        ? value.reboot_required
            ? <LifeLine tone="warn" attr="reboot-required">
                <KeepWords text={t('life.restarts.reboot.required', {
                    packages: (value.reboot_packages ?? []).length > 0 ? t('life.restarts.reboot.packages', { packages: (value.reboot_packages ?? []).join(', ') }) : '',
                })} />
                {value.expected_kernel && value.running_kernel && <> {t('life.restarts.reboot.kernel', { running: value.running_kernel, expected: value.expected_kernel })}</>}
            </LifeLine>
            : <LifeLine tone="ok" attr="reboot-none">{t('life.restarts.reboot.none')}</LifeLine>
        : <LifeLine tone="info" attr="reboot-unknown">{t('life.restarts.reboot.unknown')}</LifeLine>;
    const interruption = (kind: string) => t(`life.restarts.interruption.${INTERRUPTIONS.has(kind) ? kind : 'other'}` as TranslationKey);
    return (
        <div className="space-y-3" data-restarts={value.services.length}>
            {reboot}
            {!value.services_known ? (
                <LifeLine tone="info" attr="services-unknown">{t('life.restarts.unknown')}</LifeLine>
            ) : value.services.length === 0 ? (
                <LifeLine tone="ok" attr="services-none">{t(value.source === 'needrestart' ? 'life.restarts.none.needrestart' : 'life.restarts.none.maps')}</LifeLine>
            ) : (
                <div>
                    <LifeLine tone="warn" attr="services-some">{t('life.restarts.some')}</LifeLine>
                    <ul className="mt-2 space-y-2 pl-6">
                        {value.services.map((item) => (
                            <li key={item.unit} className="rounded-lg border border-border bg-surface-2/50 p-3 text-sm" data-restart-unit={item.unit}>
                                <p className="font-mono text-sm text-fg">{item.unit}</p>
                                <p className="mt-0.5 text-fg-muted">{interruption(item.interruption)}</p>
                                {(item.libraries ?? []).length > 0 && (
                                    <p className="mt-0.5 break-all font-mono text-xs text-fg-subtle">{t('life.restarts.libraries', { libraries: (item.libraries ?? []).join(', ') })}</p>
                                )}
                                {PANEL_RESTARTS(item) ? (
                                    <Button onClick={() => setConfirming(item)} disabled={busy} className="mt-2 max-sm:min-h-11" data-restart-offer={item.unit}>
                                        {t('life.restarts.restart')}
                                    </Button>
                                ) : (
                                    <p className="mt-1 text-fg-muted"><KeepWords text={t('life.restarts.command', { command: `sudo systemctl restart ${item.unit}` })} /></p>
                                )}
                            </li>
                        ))}
                    </ul>
                    {value.truncated && <p className="mt-2 pl-6 text-sm text-fg-subtle">{t('life.restarts.truncated')}</p>}
                </div>
            )}
            {value.needrestart_auto_restart && <LifeLine tone="info" attr="needrestart-auto">{t('life.restarts.needrestartAuto')}</LifeLine>}
            {busy && confirming && <LifeLine tone="info" attr="restarting">{t('life.restarts.restarting', { unit: confirming.unit })}</LifeLine>}
            {result && <LifeLine tone={result.tone} attr="restart-result">{result.text}</LifeLine>}
            {confirming && (
                <Dialog
                    id="restart-unit"
                    title={t('life.restarts.confirmTitle', { unit: confirming.unit })}
                    busy={busy}
                    onDismiss={() => setConfirming(null)}
                    actions={
                        <>
                            <Button onClick={() => setConfirming(null)} disabled={busy}>{t('common.cancel')}</Button>
                            <Button variant="primary" onClick={() => void restart(confirming.unit)} loading={busy} disabled={busy} data-restart-confirm="">
                                {t('life.restarts.confirm')}
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 text-fg">{interruption(confirming.interruption)}</p>
                </Dialog>
            )}
        </div>
    );
}
