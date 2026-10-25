// Security → Security updates (v0.1.0-alpha.107, D-038 §4 and §9). Patches
// within a version are the operating system's job; this card says, from the
// operating system's own configuration and package manager, whether its
// automatic updates are on, when they last ran, how many updates and
// security updates are waiting, which installed packages come from vendor
// repositories they do not cover, and which components the operating system
// never updates. Off is a warning with the administrator's command; nothing
// here switches them off. The one change offered is a separate drop-in that
// adds the confined vendor repositories, shown in full and confirmed.
//
// Güvenlik → Güvenlik güncellemeleri: işletim sisteminin kendi otomatik
// güncellemeleri; kapalıysa yöneticinin komutuyla uyarı. Tek değişiklik,
// onaylanan ayrı ek dosyadır.
import { useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, KeepWords, RemoteGate } from './ui';
import { useRemote } from '../lib/remote';
import { LifeLine, lifeDate, type Tone } from './LifeLine';
import { VendorLaneDialog } from './VendorLaneDialog';
import { LANE_FOR_COMPONENT, LANE_SERVICE } from '../lib/vendorLane';
import { componentName } from '../lib/componentName';

export interface OSUpdates {
    state: 'read' | 'unknown';
    package_family: string;
    automatic: { state: string; tool?: string; reason?: string };
    last_run?: string;
    last_run_source?: string;
    pending: { state: string; total: number; security: number; security_names?: string[]; lists_at?: string };
    vendors: { repo_id: string; name: string; origin?: string; covered: boolean; confined: boolean; packages: number; names?: string[] }[];
    outside: { id: string; versions?: string[] }[];
    drop_in?: { path: string; state: string; origins?: string[]; proposed?: string; proposed_sha256?: string };
    needrestart_auto_restart?: boolean;
}

export function decodeOSUpdates(raw: unknown): OSUpdates {
    const body = raw as Partial<OSUpdates> | null;
    if (!body || typeof body !== 'object' || (body.state !== 'read' && body.state !== 'unknown')) throw new Error('shape');
    if (body.state === 'unknown') throw new Error('unknown');
    if (!body.automatic || typeof body.automatic.state !== 'string' || !body.pending || !Array.isArray(body.vendors) || !Array.isArray(body.outside)) {
        throw new Error('shape');
    }
    return body as OSUpdates;
}

// The administrator's own command to turn automatic updates on, per system.
export function enableCommand(family: string, reason?: string): string {
    if (family === 'dnf') return 'sudo dnf install dnf-automatic && sudo systemctl enable --now dnf-automatic-install.timer';
    if (reason === 'timer_disabled') return 'sudo systemctl enable --now apt-daily-upgrade.timer';
    if (reason === 'config_off') return 'sudo dpkg-reconfigure -plow unattended-upgrades';
    return 'sudo apt-get install unattended-upgrades && sudo dpkg-reconfigure -plow unattended-upgrades';
}

export function SecurityUpdatesCard() {
    const { t } = useI18n();
    const handle = useRemote('/api/v1/security/os-updates', decodeOSUpdates);
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-security-section="updates">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1 basis-64">
                    <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                        <ShieldCheck className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                        {t('life.updates.title')}
                    </h2>
                    <p className="mt-0.5 text-sm text-fg-muted">{t('life.updates.desc')}</p>
                </div>
                {handle.remote.state !== 'loading' && (
                    <Button onClick={() => void handle.retry()} loading={handle.reading} disabled={handle.reading} className="max-sm:min-h-11">
                        {t('life.updates.again')}
                    </Button>
                )}
            </div>
            <div className="mt-4">
                <RemoteGate remote={handle.remote} checking={t('life.updates.checking')} failed={t('life.updates.unknown')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <UpdatesFacts value={value} onChanged={() => void handle.retry()} />}
                </RemoteGate>
            </div>
        </section>
    );
}

function UpdatesFacts({ value, onChanged }: { value: OSUpdates; onChanged: () => void }) {
    const { t, locale } = useI18n();
    const [confirming, setConfirming] = useState(false);
    const [writing, setWriting] = useState(false);
    const [result, setResult] = useState<{ tone: Tone; text: string } | null>(null);
    // alpha.108 (owner finding: "ne ekleyeyim"): an uncovered or unconfined
    // vendor repository gets its direct action, the lane's missing steps.
    const [laneFor, setLaneFor] = useState<string | null>(null);
    const auto = value.automatic;
    const autoTone: Tone = auto.state === 'on' ? 'ok' : auto.state === 'not_applicable' || auto.state === 'unknown' ? 'info' : 'warn';
    const command = enableCommand(value.package_family, auto.reason);
    const autoText = auto.state === 'on'
        ? t('life.updates.auto.on', { tool: auto.tool ?? '' })
        : auto.state === 'off'
          ? t(`life.updates.auto.off.${auto.reason === 'timer_disabled' ? 'timer_disabled' : 'config_off'}` as TranslationKey, { command })
          : auto.state === 'not_installed'
            ? t('life.updates.auto.not_installed', { command })
            : auto.state === 'not_applicable'
              ? t('life.updates.auto.not_applicable')
              : t('life.updates.auto.unknown');
    const lastRun = value.last_run
        ? t(value.package_family === 'pacman' ? 'life.updates.lastRun.pacman' : value.last_run_source === 'timer' ? 'life.updates.lastRun.timer' : 'life.updates.lastRun.log', { date: lifeDate(value.last_run, locale, true) })
        : auto.state === 'unknown' ? '' : t('life.updates.lastRun.none');
    const pending = value.pending;
    const listsAt = lifeDate(pending.lists_at, locale, true);
    const pendingTone: Tone = pending.state !== 'known' ? 'info' : pending.security > 0 ? 'warn' : 'ok';
    const pendingText = pending.state === 'known'
        ? pending.total > 0
            ? t(listsAt ? 'life.updates.pending.some' : 'life.updates.pending.someNoDate', { total: pending.total, security: pending.security, date: listsAt })
            : t(listsAt ? 'life.updates.pending.none' : 'life.updates.pending.noneNoDate', { date: listsAt })
        : t(pending.state === 'busy' ? 'life.updates.pending.busy' : 'life.updates.pending.unknown');
    const dropIn = value.drop_in;
    const write = async () => {
        if (!dropIn?.proposed_sha256) return;
        setWriting(true);
        setResult(null);
        try {
            const res = await fetch('/api/v1/security/os-updates/origins', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ expected_sha256: dropIn.proposed_sha256 }),
            });
            const raw = await res.text();
            let body: { effective?: boolean; origins?: string[]; error?: string };
            try {
                body = JSON.parse(raw);
            } catch (error) {
                // An answer that is not JSON is said as it came.
                body = { error: raw.slice(0, 200) || String(error) };
            }
            if (!res.ok) {
                setResult({ tone: 'bad', text: t('life.updates.dropIn.failed', { reason: body.error ?? '' }) });
            } else if (body.effective) {
                setResult({ tone: 'ok', text: t('life.updates.dropIn.done', { origins: (body.origins ?? []).join(', ') }) });
            } else {
                setResult({ tone: 'warn', text: t('life.updates.dropIn.notEffective') });
            }
        } catch {
            setResult({ tone: 'bad', text: t('life.updates.dropIn.failed', { reason: '' }) });
        } finally {
            setWriting(false);
            setConfirming(false);
            onChanged();
        }
    };
    return (
        <div className="space-y-3" data-os-updates={auto.state}>
            <LifeLine tone={autoTone} attr="automatic"><KeepWords text={autoText} /></LifeLine>
            {lastRun && <p className="pl-6 text-sm text-fg-muted" data-os-updates-last-run="">{lastRun}</p>}
            <LifeLine tone={pendingTone} attr="pending">{pendingText}</LifeLine>
            {pending.state === 'known' && (pending.security_names ?? []).length > 0 && (
                <p className="pl-6 text-sm text-fg-muted"><KeepWords text={t('life.updates.pending.securityNames', { names: (pending.security_names ?? []).join(', ') })} /></p>
            )}
            {value.needrestart_auto_restart && <LifeLine tone="info" attr="needrestart">{t('life.updates.needrestart')}</LifeLine>}
            {value.vendors.map((vendor) => (
                <div key={vendor.repo_id} data-os-updates-vendor={vendor.repo_id} data-covered={vendor.covered ? 'yes' : 'no'}>
                    <LifeLine tone={vendor.covered ? 'ok' : 'warn'} attr={`vendor-${vendor.repo_id}`}>
                        <KeepWords
                            text={vendor.covered
                                ? t('life.updates.vendor.covered', { name: vendor.name })
                                : vendor.packages > 0
                                  ? t('life.updates.vendor.uncovered', { name: vendor.name, count: vendor.packages, names: (vendor.names ?? []).length ? ': ' + (vendor.names ?? []).join(', ') : '' })
                                  : t('life.updates.vendor.uncoveredNone', { name: vendor.name })}
                        />
                    </LifeLine>
                    {!vendor.covered && !vendor.confined && <p className="pl-6 text-sm text-fg-muted">{t('life.updates.vendor.notConfined', { name: vendor.name })}</p>}
                    {(!vendor.covered || !vendor.confined) && LANE_SERVICE[vendor.repo_id] && (
                        <div className="mt-2 pl-6">
                            <Button onClick={() => setLaneFor(vendor.repo_id)} className="max-sm:min-h-11" data-os-updates-lane={vendor.repo_id}>
                                {t('lane.updates.fix', { name: LANE_FOR_COMPONENT[LANE_SERVICE[vendor.repo_id]]?.name ?? vendor.name })}
                            </Button>
                            <p className="mt-1 text-xs text-fg-subtle">{t('lane.updates.fixHint')}</p>
                        </div>
                    )}
                </div>
            ))}
            {dropIn?.state === 'present' && (dropIn.origins ?? []).length > 0 && (
                <LifeLine tone="info" attr="drop-in-present"><KeepWords text={t('life.updates.dropIn.present', { path: dropIn.path, origins: (dropIn.origins ?? []).join(', ') })} /></LifeLine>
            )}
            {dropIn?.state === 'edited' && <LifeLine tone="warn" attr="drop-in-edited"><KeepWords text={t('life.updates.dropIn.edited', { path: dropIn.path })} /></LifeLine>}
            {dropIn?.proposed && (
                <div className="pl-6">
                    <Button variant="primary" onClick={() => setConfirming(true)} disabled={writing} className="max-sm:min-h-11" data-os-updates-offer="">
                        {t('life.updates.dropIn.offer')}
                    </Button>
                </div>
            )}
            {result && <LifeLine tone={result.tone} attr="drop-in-result">{result.text}</LifeLine>}
            {value.outside.length > 0 && (
                <div className="border-t border-border/60 pt-3" data-os-updates-outside="">
                    <p className="text-sm font-medium text-fg">{t('life.updates.outside.title')}</p>
                    <p className="mt-0.5 text-sm text-fg-muted">{t('life.updates.outside.desc')}</p>
                    <ul className="mt-1.5 space-y-1 text-sm text-fg">
                        {value.outside.map((item) => (
                            <li key={item.id}>
                                {t(item.id === 'roundcube' ? 'life.updates.outside.roundcube' : 'life.updates.outside.node', { versions: (item.versions ?? []).join(', ') })}
                            </li>
                        ))}
                    </ul>
                </div>
            )}
            {laneFor && LANE_SERVICE[laneFor] && (
                <VendorLaneDialog
                    serviceId={LANE_SERVICE[laneFor]}
                    component={componentName({ id: LANE_SERVICE[laneFor], name: LANE_SERVICE[laneFor] }, t)}
                    onClose={() => setLaneFor(null)}
                    onDone={() => {
                        setLaneFor(null);
                        onChanged();
                    }}
                />
            )}
            {confirming && dropIn?.proposed && (
                <Dialog
                    id="os-updates-origins"
                    title={t('life.updates.dropIn.dialogTitle')}
                    busy={writing}
                    width="lg"
                    onDismiss={() => setConfirming(false)}
                    actions={
                        <>
                            <Button onClick={() => setConfirming(false)} disabled={writing}>{t('common.cancel')}</Button>
                            <Button variant="primary" onClick={() => void write()} loading={writing} disabled={writing} data-os-updates-confirm="">
                                {writing ? t('life.updates.dropIn.writing') : t('life.updates.dropIn.confirm')}
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 text-fg">{t('life.updates.dropIn.dialogBody')}</p>
                    <p className="mt-3 break-all font-mono text-xs text-fg-muted">{dropIn.path}</p>
                    <pre className="mt-1.5 max-h-64 overflow-auto rounded-lg border border-border bg-surface-2 p-3 text-xs leading-5 text-fg" data-os-updates-text="">{dropIn.proposed}</pre>
                </Dialog>
            )}
        </div>
    );
}
