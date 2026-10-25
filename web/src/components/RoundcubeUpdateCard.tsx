// Webmail updates (v0.1.0-alpha.107, D-038 §9). Roundcube is installed by
// the panel from its own release, so the operating system never updates it:
// this card says the installed version (the tree's own answer) and the newest
// release on its line from Roundcube's releases, and updates to it only when
// the administrator confirms, with visible waiting and the result: updated
// and answering, or put back exactly as it was with the reason.
//
// Web postası güncellemeleri: kurulu sürüm, hattın en yeni yayını; yalnız
// yönetici onaylayınca güncellenir, sonuç ya da geri alma söylenir.
import { Mail } from 'lucide-react';
import { useI18n } from '../i18n';
import { Button, RemoteGate } from './ui';
import { useRemote } from '../lib/remote';
import { LifeLine, lifeDate } from './LifeLine';
import { OpenWebmailLink, RoundcubeUpdateDialog, RoundcubeUpdateProgress, useRoundcubeUpdate } from './RoundcubeUpdateFlow';
import { decodeRoundcubeUpdate, ROUNDCUBE_UPDATE_URL, type RoundcubeUpdateCheck } from '../lib/roundcubeUpdate';

export { decodeRoundcubeUpdate } from '../lib/roundcubeUpdate';

export function RoundcubeUpdateCard() {
    const { t } = useI18n();
    const handle = useRemote(ROUNDCUBE_UPDATE_URL, decodeRoundcubeUpdate);
    return (
        <section className="rounded-2xl border border-border bg-surface" data-webmail-update="">
            <header className="flex items-center gap-2 border-b border-border px-5 py-3">
                <Mail className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                <h2 className="text-sm font-semibold text-fg">{t('life.webmail.title')}</h2>
            </header>
            <div className="space-y-3 p-5">
                <p className="text-sm text-fg-muted">{t('life.webmail.desc')}</p>
                <RemoteGate remote={handle.remote} checking={t('life.webmail.checking')} failed={t('life.webmail.unknown')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <WebmailFacts value={value} onChanged={() => void handle.retry()} />}
                </RemoteGate>
            </div>
        </section>
    );
}

function WebmailFacts({ value, onChanged }: { value: RoundcubeUpdateCheck; onChanged: () => void }) {
    const { t, locale } = useI18n();
    const latest = value.latest_on_line ?? '';
    // The same flow the Components row starts (RoundcubeUpdateFlow.tsx).
    // Bileşenler satırının başlattığı akışın aynısı.
    const flow = useRoundcubeUpdate({ current: value.version, latest, lastUpdate: value.last_update, onChanged });
    const { confirming, setConfirming, updating, phase, result, update } = flow;
    if (value.check_unknown) {
        return (
            <div className="space-y-3" data-webmail-state="unknown">
                <LifeLine tone="info" attr="webmail-check-unknown">{t('life.webmail.unknown')}</LifeLine>
                <RoundcubeUpdateProgress updating={updating} phase={phase} result={result} />
            </div>
        );
    }
    if (!value.installed) return <p className="text-sm text-fg-muted">{t('life.webmail.notInstalled')}</p>;
    return (
        <div className="space-y-3" data-webmail-state={value.update_available ? 'available' : value.error ? 'error' : 'current'}>
            <OpenWebmailLink ready={value.webmail_ready} />
            {value.error ? (
                <LifeLine tone="info" attr="webmail-error">{t('life.webmail.checkError', { reason: value.error })}</LifeLine>
            ) : value.update_available ? (
                <LifeLine tone="warn" attr="webmail-available">{t('life.webmail.available', { latest, line: value.line ?? '', version: value.version ?? '' })}</LifeLine>
            ) : (
                <LifeLine tone="ok" attr="webmail-current">{t('life.webmail.current', { version: value.version ?? '', line: value.line ?? '' })}</LifeLine>
            )}
            {value.latest_overall && value.line && !value.latest_overall.startsWith(value.line + '.') && (
                <p className="pl-6 text-sm text-fg-muted">{t('life.webmail.otherLine', { overall: value.latest_overall })}</p>
            )}
            {value.checked_at && <p className="pl-6 text-sm text-fg-subtle">{t('life.webmail.checkedAt', { date: lifeDate(value.checked_at, locale, true) })}</p>}
            {value.update_available && latest && (
                <div className="pl-6">
                    <Button variant="primary" onClick={() => setConfirming(true)} disabled={updating} className="max-sm:min-h-11" data-webmail-update-offer="">
                        {t('life.webmail.update', { latest })}
                    </Button>
                </div>
            )}
            <RoundcubeUpdateProgress updating={updating} phase={phase} result={result} />
            {confirming && (
                <RoundcubeUpdateDialog latest={latest} updating={updating} onCancel={() => setConfirming(false)} onConfirm={() => void update()} />
            )}
        </div>
    );
}
