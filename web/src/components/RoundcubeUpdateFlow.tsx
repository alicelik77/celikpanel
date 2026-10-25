// The one confirmed webmail (Roundcube) update flow (alpha.107, D-038 §9; moved
// here in alpha.108 so the Components row and the webmail card start the SAME
// operation): the confirmation dialog, the visible waiting, and the result in
// the owner's words - updated and answering, or put back exactly as it was,
// with the reason. A second start is refused while one is running, and the
// request is never sent twice.
//
// Tek onaylı web postası güncelleme akışı: Bileşenler satırı ve web postası
// kartı AYNI işlemi başlatır; onay penceresi, görünür bekleme ve sonuç burada.
import { useRef, useState } from 'react';
import { useI18n } from '../i18n';
import { Button, Dialog, KeepWords } from './ui';
import { LifeLine, type Tone } from './LifeLine';

export interface RoundcubeUpdateOutcome {
    tone: Tone;
    text: string;
}

export function useRoundcubeUpdate({ current, latest, onChanged }: { current?: string; latest: string; onChanged: () => void }) {
    const { t } = useI18n();
    const [confirming, setConfirming] = useState(false);
    const [updating, setUpdating] = useState(false);
    const [result, setResult] = useState<RoundcubeUpdateOutcome | null>(null);
    const running = useRef(false);
    const update = async () => {
        if (running.current) return;
        running.current = true;
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
                setResult({ tone: 'ok', text: t('life.webmail.done', { from: body.from ?? current ?? '', to: body.to ?? latest, backup: body.backup ?? '' }) });
            } else if (body.rolled_back) {
                setResult({ tone: 'warn', text: t('life.webmail.rolledBack', { from: body.from ?? current ?? '', to: latest, reason: body.error ?? '' }) });
            } else {
                setResult({ tone: 'bad', text: t('life.webmail.failed', { reason: body.error ?? '' }) });
            }
        } catch {
            setResult({ tone: 'bad', text: t('life.webmail.failed', { reason: '' }) });
        } finally {
            running.current = false;
            setUpdating(false);
            setConfirming(false);
            onChanged();
        }
    };
    return { confirming, setConfirming, updating, result, update };
}

export function RoundcubeUpdateProgress({ updating, result }: { updating: boolean; result: RoundcubeUpdateOutcome | null }) {
    const { t } = useI18n();
    return (
        <>
            {updating && <LifeLine tone="info" attr="webmail-updating">{t('life.webmail.updating')}</LifeLine>}
            {result && <LifeLine tone={result.tone} attr="webmail-result"><KeepWords text={result.text} /></LifeLine>}
        </>
    );
}

export function RoundcubeUpdateDialog({ latest, updating, onCancel, onConfirm }: { latest: string; updating: boolean; onCancel: () => void; onConfirm: () => void }) {
    const { t } = useI18n();
    return (
        <Dialog
            id="webmail-update"
            title={t('life.webmail.dialogTitle', { latest })}
            busy={updating}
            width="lg"
            onDismiss={onCancel}
            actions={
                <>
                    <Button onClick={onCancel} disabled={updating}>{t('common.cancel')}</Button>
                    <Button variant="primary" onClick={onConfirm} loading={updating} disabled={updating} data-webmail-update-confirm="">
                        {t('life.webmail.confirm')}
                    </Button>
                </>
            }
        >
            <p className="text-sm leading-6 text-fg">{t('life.webmail.dialogBody', { latest })}</p>
            {updating && <p className="mt-3 text-sm text-fg-muted">{t('life.webmail.updating')}</p>}
        </Dialog>
    );
}
