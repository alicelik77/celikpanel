import { useState } from 'react';
import { useMailOutgoingWords } from '../i18n/mailOutgoing';
import { apiErrorText, readApiError } from '../lib/apiError';
import { refreshRemote, useRemote } from '../lib/remote';
import { showToast } from './Toast';
import { Button, inputClass } from './ui';

// The mail settings of hosting plans (alpha.128, gap G12): the largest
// mailbox a plan allows and its outgoing values. Empty is the server's value.
// One row per plan, saved by itself; the outgoing values reach the server
// within a minute.
//
// Barındırma planlarının posta ayarları: en büyük posta kutusu ve giden
// posta değerleri. Boş, sunucunun değeridir.

const MAIL_OUTGOING_URL = '/api/v1/mail/outgoing';
const PLAN_LIMITS_URL = '/api/v1/mail/plan-limits';

interface Limits {
    mailbox_size_mb: number | null;
    outgoing_mailbox: number | null;
    outgoing_domain: number | null;
    outgoing_subscription: number | null;
}

interface Plan {
    id: number;
    name: string;
    limits: Limits;
}

function decodePlans(raw: unknown): Plan[] {
    const plans = (raw as { plans?: unknown } | null)?.plans;
    if (!Array.isArray(plans)) throw new Error('plans');
    return plans as Plan[];
}

const FIELDS: (keyof Limits)[] = ['mailbox_size_mb', 'outgoing_mailbox', 'outgoing_domain', 'outgoing_subscription'];

function PlanRow({ plan }: { plan: Plan }) {
    const { t } = useMailOutgoingWords();
    const [draft, setDraft] = useState<Record<keyof Limits, string>>(() => ({
        mailbox_size_mb: plan.limits.mailbox_size_mb?.toString() ?? '',
        outgoing_mailbox: plan.limits.outgoing_mailbox?.toString() ?? '',
        outgoing_domain: plan.limits.outgoing_domain?.toString() ?? '',
        outgoing_subscription: plan.limits.outgoing_subscription?.toString() ?? '',
    }));
    const [busy, setBusy] = useState(false);
    const value = (text: string) => (text.trim() === '' ? null : Math.max(0, parseInt(text, 10) || 0));
    const save = async () => {
        setBusy(true);
        try {
            const res = await fetch(PLAN_LIMITS_URL, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ owner_kind: 'plan', owner_id: plan.id, ...Object.fromEntries(FIELDS.map((f) => [f, value(draft[f])])) }),
            });
            if (!res.ok) {
                showToast('error', apiErrorText(await readApiError(res), t));
                return;
            }
            showToast('success', t('mout.plans.saved', { name: plan.name }));
        } catch {
            showToast('error', t('common.resultUnknown'));
        } finally {
            setBusy(false);
            refreshRemote(PLAN_LIMITS_URL);
            refreshRemote(MAIL_OUTGOING_URL);
        }
    };
    const labels: Record<keyof Limits, string> = {
        mailbox_size_mb: t('mout.plans.size'),
        outgoing_mailbox: t('mout.mailbox'),
        outgoing_domain: t('mout.domain'),
        outgoing_subscription: t('mout.subscription'),
    };
    return (
        <tr className="border-b border-border last:border-0">
            <td className="py-2 pr-3 font-medium text-fg">{plan.name}</td>
            {FIELDS.map((field) => (
                <td key={field} className="py-2 pr-3">
                    <input type="number" min={field === 'mailbox_size_mb' ? 1 : 0} value={draft[field]} placeholder="—"
                        aria-label={`${plan.name}: ${labels[field]}`}
                        onChange={(event) => setDraft((d) => ({ ...d, [field]: event.target.value }))}
                        className={`${inputClass} w-28 py-1`} />
                </td>
            ))}
            <td className="py-2 text-right">
                <Button type="button" disabled={busy} loading={busy} onClick={() => void save()}>{t('mout.save')}</Button>
            </td>
        </tr>
    );
}

export function MailPlanLimits() {
    const { t } = useMailOutgoingWords();
    const read = useRemote(PLAN_LIMITS_URL, decodePlans);
    if (read.remote.state !== 'known') return null;
    const plans = read.remote.value;
    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-5" data-mail-plan-limits>
            <h3 className="mb-1 text-sm font-semibold text-fg">{t('mout.plans.title')}</h3>
            <p className="mb-3 max-w-[75ch] text-sm text-fg-muted">{t('mout.plans.desc')}</p>
            {plans.length === 0 ? (
                <p className="text-sm text-fg-muted">{t('mout.plans.none')}</p>
            ) : (
                <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                        <thead>
                            <tr className="border-b border-border text-left text-xs text-fg-muted">
                                <th className="py-2 pr-3">{t('mout.plans.plan')}</th>
                                <th className="py-2 pr-3">{t('mout.plans.size')}</th>
                                <th className="py-2 pr-3">{t('mout.mailbox')}</th>
                                <th className="py-2 pr-3">{t('mout.domain')}</th>
                                <th className="py-2 pr-3">{t('mout.subscription')}</th>
                                <th className="py-2" />
                            </tr>
                        </thead>
                        <tbody>
                            {plans.map((plan) => <PlanRow key={`${plan.id}:${JSON.stringify(plan.limits)}`} plan={plan} />)}
                        </tbody>
                    </table>
                </div>
            )}
        </section>
    );
}
