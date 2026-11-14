import { useMemo } from 'react';
import { useMailOutgoingWords } from '../i18n/mailOutgoing';
import { apiErrorText, readApiError } from '../lib/apiError';
import { lastKnown, refreshRemote, useRemote } from '../lib/remote';
import { showToast } from './Toast';
import { inputClass } from './ui';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

// The mail settings of hosting plans (alpha.128, gap G12): the largest
// mailbox a plan allows and its outgoing values. Empty is the server's value.
// One draft for the whole table (alpha.132): a changed value is marked, the
// save bar's Save sends each changed plan once; the outgoing values reach the
// server within a minute.
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

// The draft's keys: "<plan id>:<field>", the value as typed ('' = the server's value).
type Values = Record<string, string>;
const key = (plan: Plan, field: keyof Limits) => `${plan.id}:${field}`;
const number = (text: string) => (text.trim() === '' ? null : Math.max(0, parseInt(text, 10) || 0));

export function MailPlanLimits() {
    const { t } = useMailOutgoingWords();
    const say = useSaveWords();
    const read = useRemote(PLAN_LIMITS_URL, decodePlans);
    const plans = lastKnown(read.remote)?.value;
    const known = useMemo(() => (plans
        ? Object.fromEntries(plans.flatMap((plan) => FIELDS.map((field) => [key(plan, field), plan.limits[field]?.toString() ?? ''])))
        : null), [plans]);
    const draft = useDraft<Values>(known);
    const labels: Record<keyof Limits, string> = {
        mailbox_size_mb: t('mout.plans.size'),
        outgoing_mailbox: t('mout.mailbox'),
        outgoing_domain: t('mout.domain'),
        outgoing_subscription: t('mout.subscription'),
    };
    // Each changed plan is sent once, whole; a refusal stops the rest and keeps them marked.
    const save = () => void draft.save(async (changes, values) => {
        const changed = (plans ?? []).filter((plan) => FIELDS.some((field) => key(plan, field) in changes));
        try {
            for (const plan of changed) {
                const res = await fetch(PLAN_LIMITS_URL, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ owner_kind: 'plan', owner_id: plan.id, ...Object.fromEntries(FIELDS.map((field) => [field, number(values[key(plan, field)] ?? '')])) }),
                });
                if (!res.ok) {
                    showToast('error', apiErrorText(await readApiError(res), t));
                    return 'refused';
                }
                showToast('success', t('mout.plans.saved', { name: plan.name }));
            }
            return 'saved';
        } catch {
            return 'unknown';
        } finally {
            refreshRemote(PLAN_LIMITS_URL);
            refreshRemote(MAIL_OUTGOING_URL);
        }
    });
    if (!plans || !draft.values) return null;
    const values = draft.values;
    return (
        <section id="mail-plan-limits" className="mt-5 scroll-mt-24 rounded-xl border border-border bg-surface p-5" data-mail-plan-limits>
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
                                {FIELDS.map((field) => <th key={field} className="py-2 pr-3">{labels[field]}</th>)}
                            </tr>
                        </thead>
                        <tbody>
                            {plans.map((plan) => {
                                const rowChanged = FIELDS.some((field) => draft.isChanged(key(plan, field)));
                                return (
                                    <tr key={plan.id} className={`border-b border-border last:border-0 ${rowChanged ? changedRowClass : ''}`} data-mail-plan={plan.id}>
                                        <td className="py-2 pl-2 pr-3 font-medium text-fg">{plan.name}{rowChanged && <ChangedMark />}</td>
                                        {FIELDS.map((field) => (
                                            <td key={field} className="py-2 pr-3">
                                                <input type="number" min={field === 'mailbox_size_mb' ? 1 : 0} value={values[key(plan, field)] ?? ''} placeholder="—"
                                                    aria-label={`${plan.name}: ${labels[field]}`} disabled={draft.saving}
                                                    onChange={(event) => draft.set(key(plan, field), event.target.value)}
                                                    className={`${inputClass} py-1 field-xs`} />
                                            </td>
                                        ))}
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </div>
            )}
            <SaveBar
                className="-mx-5 px-5"
                count={draft.count}
                saving={draft.saving}
                blockedBy={read.remote.state !== 'known' ? say('stale') : undefined}
                unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: read.reading, onCheck: () => void read.retry() } : undefined}
                onSave={save}
                onDiscard={draft.reset}
            />
        </section>
    );
}
