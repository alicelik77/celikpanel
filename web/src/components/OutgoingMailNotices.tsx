import { useState } from 'react';
import { AlertTriangle, Check, ShieldAlert } from 'lucide-react';
import { useMailOutgoingWords } from '../i18n/mailOutgoing';
import { refusalText } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { sendRights } from '../lib/rights';
import { useNavigate } from '../router';
import { showToast } from './Toast';
import { Button } from './ui';

// "Limit reached" and "stopped for security" notices (alpha.128, gap G1):
// what happened, what keeps working, the next step and the button that takes
// it. A notice stays until it is marked as seen; it was also e-mailed when the
// panel's mail is set up.
//
// "Sınır doldu" ve "güvenlik için durduruldu" bildirimleri: ne oldu, ne
// çalışmaya devam ediyor, sonraki adım ve onu atan düğme.

export const MAIL_OUTGOING_NOTICES_URL = '/api/v1/mail-outgoing-notices';

interface OutgoingNotice {
    id: number;
    kind: 'limit_reached' | 'paused';
    audience: 'customer' | 'reseller' | 'administrator';
    created_at: string;
    body: {
        address: string;
        domain?: string;
        limit?: number;
        refused?: number;
        reason?: string;
        recipients?: number;
        normal?: number;
    };
}

function decodeNotices(raw: unknown): OutgoingNotice[] {
    const notices = (raw as { notices?: unknown } | null)?.notices;
    if (!Array.isArray(notices)) throw new Error('notices');
    return (notices as OutgoingNotice[]).filter((n) => n.kind === 'limit_reached' || n.kind === 'paused');
}

export function OutgoingMailNotices() {
    const { t } = useMailOutgoingWords();
    const navigate = useNavigate();
    const read = useRemote(MAIL_OUTGOING_NOTICES_URL, decodeNotices);
    const [busy, setBusy] = useState(false);
    if (read.remote.state !== 'known' || read.remote.value.length === 0) return null;
    const notices = read.remote.value;
    const seen = async (ids: number[]) => {
        if (busy) return;
        setBusy(true);
        try {
            for (const id of ids) {
                const result = await sendRights(`${MAIL_OUTGOING_NOTICES_URL}/${id}/seen`, 'POST');
                if (result.kind === 'lost') {
                    showToast('error', t('common.resultUnknown'));
                    break;
                }
                if (result.kind === 'refused') {
                    showToast('error', refusalText(result.error, t, 'rights.notice.ackFailed'));
                    break;
                }
            }
        } finally {
            void read.retry();
            invalidate('mail');
            setBusy(false);
        }
    };
    return (
        <section className="grid gap-2" data-outgoing-notices={notices.length} aria-label={t('mout.title')}>
            {notices.map((notice) => {
                const b = notice.body;
                const paused = notice.kind === 'paused';
                const title = t(paused ? 'mout.notice.pausedTitle' : 'mout.notice.limitTitle', { address: b.address });
                const body = paused
                    ? t(b.reason === 'refusals' ? 'mout.notice.pausedRefusals' : 'mout.notice.paused', { recipients: b.recipients ?? 0, normal: b.normal ?? 0, refused: b.refused ?? 0 })
                    : t('mout.notice.limit', { refused: b.refused ?? 0, limit: b.limit ?? 0 });
                const Icon = paused ? ShieldAlert : AlertTriangle;
                return (
                    <div key={notice.id} role="status" data-outgoing-notice={notice.kind}
                        className={`rounded-lg border p-3 text-sm leading-6 text-fg ${paused ? 'border-danger/40 bg-danger/10' : 'border-warning-mark/50 bg-warning-mark/15'}`}>
                        <p className="flex items-start gap-2 font-semibold">
                            <Icon className={`mt-1 h-4 w-4 shrink-0 ${paused ? 'text-danger' : 'text-warning'}`} aria-hidden="true" />
                            <span>{title}</span>
                        </p>
                        <p className="mt-1 max-w-[75ch]">{body}</p>
                        {b.domain && (
                            <Button type="button" variant={paused ? 'primary' : 'secondary'} className="mt-2 mr-2 max-sm:min-h-11"
                                onClick={() => navigate(`/domains/${encodeURIComponent(b.domain!)}?tab=mail`)}>
                                {t('mout.notice.open')}
                            </Button>
                        )}
                        <Button type="button" icon={Check} className="mt-2 max-sm:min-h-11" onClick={() => void seen([notice.id])} disabled={busy}>{t('mout.notice.seen')}</Button>
                    </div>
                );
            })}
        </section>
    );
}

export default OutgoingMailNotices;
