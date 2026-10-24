// Webmail updates (v0.1.0-alpha.107, D-038 §9). Roundcube is installed by
// the panel from its own release, so the operating system never updates it:
// this card says the installed version (the tree's own answer) and the newest
// release on its line from Roundcube's releases, and updates to it only when
// the administrator confirms, with visible waiting and the result: updated
// and answering, or put back exactly as it was with the reason.
//
// Web postası güncellemeleri: kurulu sürüm, hattın en yeni yayını; yalnız
// yönetici onaylayınca güncellenir, sonuç ya da geri alma söylenir.
import { useState } from 'react';
import { Mail } from 'lucide-react';
import { useI18n } from '../i18n';
import { Button, Dialog, KeepWords, RemoteGate } from './ui';
import { useRemote } from '../lib/remote';
import { LifeLine, lifeDate, type Tone } from './LifeLine';
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
    const [confirming, setConfirming] = useState(false);
    const [updating, setUpdating] = useState(false);
    const [result, setResult] = useState<{ tone: Tone; text: string } | null>(null);
    if (!value.installed) return <p className="text-sm text-fg-muted">{t('life.webmail.notInstalled')}</p>;
    const latest = value.latest_on_line ?? '';
    const update = async () => {
        setUpdating(true);
        setResult(null);
        try {
            const res = await fetch('/api/v1/service/roundcube/update', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ version: latest }),
            });
            const raw = await res.text();
            let body: { from?: string; to?: string; backup?: string; error?: string; rolled_back?: boolean };
            try {
                body = JSON.parse(raw);
            } catch (error) {
                body = { error: raw.slice(0, 200) || String(error) };
            }
            if (res.ok) {
                setResult({ tone: 'ok', text: t('life.webmail.done', { from: body.from ?? value.version ?? '', to: body.to ?? latest, backup: body.backup ?? '' }) });
            } else if (body.rolled_back) {
                setResult({ tone: 'warn', text: t('life.webmail.rolledBack', { from: body.from ?? value.version ?? '', to: latest, reason: body.error ?? '' }) });
            } else {
                setResult({ tone: 'bad', text: t('life.webmail.failed', { reason: body.error ?? '' }) });
            }
        } catch {
            setResult({ tone: 'bad', text: t('life.webmail.failed', { reason: '' }) });
        } finally {
            setUpdating(false);
            setConfirming(false);
            onChanged();
        }
    };
    return (
        <div className="space-y-3" data-webmail-state={value.update_available ? 'available' : value.error ? 'error' : 'current'}>
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
            {updating && <LifeLine tone="info" attr="webmail-updating">{t('life.webmail.updating')}</LifeLine>}
            {result && <LifeLine tone={result.tone} attr="webmail-result"><KeepWords text={result.text} /></LifeLine>}
            {confirming && (
                <Dialog
                    id="webmail-update"
                    title={t('life.webmail.dialogTitle', { latest })}
                    busy={updating}
                    width="lg"
                    onDismiss={() => setConfirming(false)}
                    actions={
                        <>
                            <Button onClick={() => setConfirming(false)} disabled={updating}>{t('common.cancel')}</Button>
                            <Button variant="primary" onClick={() => void update()} loading={updating} disabled={updating} data-webmail-update-confirm="">
                                {t('life.webmail.confirm')}
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 text-fg">{t('life.webmail.dialogBody', { latest })}</p>
                    {updating && <p className="mt-3 text-sm text-fg-muted">{t('life.webmail.updating')}</p>}
                </Dialog>
            )}
        </div>
    );
}
