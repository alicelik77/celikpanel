// Security → Security updates → Automatic or manual updates (alpha.114).
// On Debian and Ubuntu the owner chooses whether apt installs operating-system
// updates by itself. The setting is apt's own file
// (/etc/apt/apt.conf.d/20auto-upgrades) and apt's own timers, so it keeps
// working with CelikPanel stopped or removed. The current state is what apt
// reports, not what this page last wrote. A file the owner edited is shown in
// full and replaced only after the owner confirms having read it. Reading
// never changes anything; the one change is the button's, once.
//
// Debian ve Ubuntu'da sahip, apt'nin güncellemeleri kendiliğinden kurup
// kurmayacağını seçer. Ayar apt'nin kendi dosyası ve zamanlayıcılarıdır;
// CelikPanel olmadan da çalışır. Sahibin düzenlediği dosya, okunduğu
// onaylanmadan değiştirilmez.
import { useState } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, KeepWords } from './ui';
import { LifeLine, type Tone } from './LifeLine';

export interface OSUpdatesAutoConfig {
    path: string;
    state: 'absent' | 'panel' | 'stock' | 'edited' | 'unknown';
    mode?: 'auto' | 'manual';
    current_sha256?: string;
    current?: string;
    unreadable?: boolean;
    legacy_path?: string;
    legacy_state?: '' | 'panel' | 'edited';
    overridden?: boolean;
    timer_state: string;
    covers?: string[];
    proposed_auto?: string;
    proposed_manual?: string;
}

type Mode = 'auto' | 'manual';
type Outcome = { tone: Tone; text: string } | null;

export const MANUAL_COMMAND = 'sudo apt-get update && sudo apt-get upgrade';
export const FIND_OVERRIDE_COMMAND = 'grep -R "Periodic::Unattended-Upgrade" /etc/apt/apt.conf.d/';
export const UNMASK_COMMAND = 'sudo systemctl unmask apt-daily.timer apt-daily-upgrade.timer';
export const ENABLE_TIMER_COMMAND = 'sudo systemctl enable --now apt-daily.timer apt-daily-upgrade.timer';
export const RECONFIGURE_COMMAND = 'sudo dpkg-reconfigure -plow unattended-upgrades';
export const INSTALL_COMMAND = 'sudo apt-get install unattended-upgrades';

// What apt does now, from the system's own answer: on, or anything else.
export function currentMode(automaticState: string): Mode | 'unknown' {
    if (automaticState === 'on') return 'auto';
    if (automaticState === 'off' || automaticState === 'not_installed') return 'manual';
    return 'unknown';
}

export function OSUpdatesChoice({ config, automaticState, onChanged }: { config: OSUpdatesAutoConfig; automaticState: string; onChanged: () => void }) {
    const { t } = useI18n();
    const [target, setTarget] = useState<Mode | null>(null);
    const [confirmEdit, setConfirmEdit] = useState(false);
    const [writing, setWriting] = useState(false);
    const [outcome, setOutcome] = useState<Outcome>(null);
    const now = currentMode(automaticState);
    const edited = config.state === 'edited';
    const covers = (config.covers ?? []).join(', ');
    const notInstalled = automaticState === 'not_installed';

    const stateLine = ((): { tone: Tone; text: string } | null => {
        if (config.unreadable) return { tone: 'warn', text: t('life.choice.state.unreadable', { path: config.path }) };
        if (edited) return { tone: 'warn', text: t('life.choice.state.edited', { path: config.path }) };
        if (config.state === 'panel') return { tone: 'info', text: t('life.choice.state.panel', { path: config.path }) };
        if (config.state === 'stock') return { tone: 'info', text: t('life.choice.state.stock', { path: config.path }) };
        if (config.state === 'absent') {
            return config.legacy_state === 'panel'
                ? { tone: 'info', text: t('life.choice.state.legacy', { legacy: config.legacy_path ?? '', path: config.path }) }
                : { tone: 'info', text: t('life.choice.state.absent', { path: config.path }) };
        }
        return null;
    })();

    const open = (mode: Mode) => {
        setTarget(mode);
        setConfirmEdit(false);
        setOutcome(null);
    };

    const write = async () => {
        if (!target || writing) return;
        const mode = target;
        setWriting(true);
        setOutcome(null);
        try {
            const res = await fetch('/api/v1/security/os-updates/mode', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ mode, expected_current_sha256: config.current_sha256 ?? '', confirm_owner_edit: edited && confirmEdit }),
            });
            const raw = await res.text();
            let body: { written?: boolean; effective?: boolean; effective_reason?: string; timers_enabled?: string[]; legacy_removed?: boolean; error?: string; code?: string } = {};
            try {
                body = JSON.parse(raw);
            } catch {
                // An answer that is not JSON is said as it came.
                body = { error: raw.slice(0, 200) };
            }
            if (!res.ok) {
                const code = (body.code ?? '').replace(/^OS_UPDATES_MODE_/, '');
                const known = ['OWNER_EDITED', 'CURRENT_CHANGED', 'UNATTENDED_NOT_INSTALLED', 'NOT_REGULAR'].includes(code);
                const text = known
                    ? t(`life.choice.failed.${code}` as TranslationKey, { path: config.path, command: INSTALL_COMMAND })
                    : t('life.choice.failed.other', { reason: body.error ?? '', command: RECONFIGURE_COMMAND });
                setOutcome({ tone: 'bad', text });
            } else {
                const parts: string[] = [];
                const reason = body.effective_reason ?? '';
                if (body.effective) {
                    parts.push(t(mode === 'auto' ? 'life.choice.result.auto' : 'life.choice.result.manual', { path: config.path }));
                } else {
                    const command = reason === 'timer_masked' ? UNMASK_COMMAND : reason === 'timer_disabled' ? ENABLE_TIMER_COMMAND : reason === 'apt_config_unreadable' ? 'apt-config dump' : FIND_OVERRIDE_COMMAND;
                    const key = reason === 'timer_masked' || reason === 'timer_disabled' || reason === 'apt_config_unreadable' ? reason : 'overridden';
                    parts.push(t(`life.choice.result.${key}` as TranslationKey, { command }));
                }
                if ((body.timers_enabled ?? []).length > 0) parts.push(t('life.choice.result.timers', { units: (body.timers_enabled ?? []).join(', ') }));
                if (body.legacy_removed) parts.push(t('life.choice.result.legacy'));
                setOutcome({ tone: body.effective ? 'ok' : 'warn', text: parts.join(' ') });
            }
        } catch {
            // No answer: whether it was written is not known. Nothing is sent again.
            setOutcome({ tone: 'warn', text: t('life.choice.unknownResult') });
        } finally {
            setWriting(false);
            setTarget(null);
            onChanged();
        }
    };

    const modeButton = (mode: Mode) => (
        <Button
            key={mode}
            variant={now !== mode && now !== 'unknown' ? 'primary' : undefined}
            onClick={() => open(mode)}
            disabled={writing || now === mode || (mode === 'auto' && notInstalled)}
            className="max-sm:min-h-11"
            data-os-choice-switch={mode}
        >
            {t(mode === 'auto' ? 'life.choice.switch.auto' : 'life.choice.switch.manual')}
        </Button>
    );

    return (
        <div className="space-y-3 border-t border-border/60 pt-3" data-os-choice={now}>
            <p className="text-sm font-medium text-fg">{t('life.choice.title')}</p>
            <p className="text-sm text-fg-muted">{t('life.choice.intro')}</p>
            <LifeLine tone={now === 'unknown' ? 'info' : 'ok'} attr="choice-now">
                {now === 'auto' ? t('life.choice.now.auto') : now === 'manual' ? t(notInstalled ? 'life.choice.now.notInstalled' : 'life.choice.now.manual') : t('life.choice.now.unknown')}
            </LifeLine>
            <p className="pl-6 text-sm text-fg-muted" data-os-choice-auto="">
                <KeepWords text={covers ? t('life.choice.auto.means', { origins: covers }) : t('life.choice.auto.meansDefault')} />
            </p>
            <p className="pl-6 text-sm text-fg-muted" data-os-choice-manual="">
                <KeepWords text={t('life.choice.manual.means', { command: MANUAL_COMMAND })} />
            </p>
            <p className="pl-6 text-sm text-fg-muted" data-os-choice-major="">{t('life.choice.major')}</p>
            {stateLine && <LifeLine tone={stateLine.tone} attr={`choice-file-${config.unreadable ? 'unreadable' : config.state}`}><KeepWords text={stateLine.text} /></LifeLine>}
            {config.legacy_state === 'edited' && (
                <LifeLine tone="warn" attr="choice-legacy-edited"><KeepWords text={t('life.choice.state.legacyEdited', { legacy: config.legacy_path ?? '', path: config.path })} /></LifeLine>
            )}
            {config.overridden && (
                <LifeLine tone="warn" attr="choice-overridden"><KeepWords text={t('life.choice.state.overridden', { path: config.path, command: FIND_OVERRIDE_COMMAND })} /></LifeLine>
            )}
            {config.timer_state === 'masked' && (
                <LifeLine tone="warn" attr="choice-timer-masked"><KeepWords text={t('life.choice.timer.masked', { command: UNMASK_COMMAND })} /></LifeLine>
            )}
            {config.timer_state === 'disabled' && <LifeLine tone="info" attr="choice-timer-disabled">{t('life.choice.timer.disabled')}</LifeLine>}
            <div className="flex flex-wrap gap-2 pl-6">
                {modeButton('auto')}
                {modeButton('manual')}
            </div>
            {outcome && <LifeLine tone={outcome.tone} attr="choice-result"><KeepWords text={outcome.text} /></LifeLine>}
            {target && (
                <Dialog
                    id="os-updates-mode"
                    title={t(target === 'auto' ? 'life.choice.dialog.title.auto' : 'life.choice.dialog.title.manual')}
                    busy={writing}
                    width="lg"
                    onDismiss={() => setTarget(null)}
                    actions={
                        <>
                            <Button onClick={() => setTarget(null)} disabled={writing}>{t('common.cancel')}</Button>
                            <Button
                                variant="primary"
                                onClick={() => void write()}
                                loading={writing}
                                disabled={writing || (edited && !confirmEdit)}
                                data-os-choice-confirm=""
                            >
                                {writing ? t('life.choice.dialog.writing') : t('life.choice.dialog.confirm')}
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 text-fg">{t(target === 'auto' ? 'life.choice.dialog.body.auto' : 'life.choice.dialog.body.manual')}</p>
                    <p className="mt-3 break-all font-mono text-xs text-fg-muted">{config.path}</p>
                    <p className="mt-2 text-xs font-medium text-fg-muted">{t('life.choice.dialog.current')}</p>
                    {config.state === 'absent' ? (
                        <p className="mt-1 text-sm text-fg-muted">{t('life.choice.dialog.currentNone')}</p>
                    ) : (
                        <pre className="mt-1 max-h-48 overflow-auto rounded-lg border border-border bg-surface-2 p-3 text-xs leading-5 text-fg" data-os-choice-current="">{config.current ?? ''}</pre>
                    )}
                    <p className="mt-2 text-xs font-medium text-fg-muted">{t('life.choice.dialog.after')}</p>
                    <pre className="mt-1 max-h-48 overflow-auto rounded-lg border border-border bg-surface-2 p-3 text-xs leading-5 text-fg" data-os-choice-text="">{target === 'auto' ? config.proposed_auto : config.proposed_manual}</pre>
                    {edited && (
                        <div className="mt-3 space-y-2" data-os-choice-owner-edit="">
                            <LifeLine tone="warn" attr="choice-dialog-owner">{t('life.choice.dialog.ownerWarn')}</LifeLine>
                            <label className="flex cursor-pointer items-start gap-3 text-sm text-fg">
                                <input
                                    type="checkbox"
                                    checked={confirmEdit}
                                    onChange={(event) => setConfirmEdit(event.target.checked)}
                                    disabled={writing}
                                    className="mt-0.5 h-4 w-4 accent-primary"
                                    data-os-choice-confirm-edit=""
                                />
                                <span>{t('life.choice.dialog.ownerConfirm')}</span>
                            </label>
                        </div>
                    )}
                    <p className="mt-3 text-xs text-fg-subtle">{t('life.choice.dialog.undo')}</p>
                    {writing && <p className="mt-2 text-sm text-fg-muted" role="status" data-os-choice-wait="">{t('life.choice.dialog.wait')}</p>}
                </Dialog>
            )}
        </div>
    );
}
