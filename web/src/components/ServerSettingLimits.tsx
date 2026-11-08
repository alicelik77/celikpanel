import { useEffect, useState } from 'react';
import { Save } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { Button, Checking, RemoteGate, inputClass } from './ui';
import { apiErrorText, readApiError } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { sendSettings, unitText } from '../lib/settingLimits';

// The server's limits of site settings (alpha.128, docs/SITE-LIMITS.md),
// administrator only: one row per key with the server limit (empty = no
// limit) and, for a setting whose value a customer chooses (the PHP
// settings), whether customers may change it. GET/PUT
// /api/v1/rights/setting-limits.
// alpha.130: one Save for the section. It names the rows that will change
// before it is pressed, saves only those, one PUT per row, and keeps a row's
// error beside the row (the other rows are saved). A limit-only key (cron,
// databases, FTP, mail) shows "Limit only" instead of a box that fixed
// nothing.
// Also the administrator's password level (G21), on the Security page.
//
// Sunucunun site ayarı sınırları ve parola düzeyi; yalnız yönetici. Bölümün
// tek Kaydet düğmesi yalnız değişen satırları kaydeder; satırın hatası
// satırın yanında kalır.

interface PolicyRow {
    key: string;
    server_ceiling: number | null;
    server_default: number | null;
    customer_editable: boolean;
    /** alpha.130: a customer chooses a value of its own (absent from older panels: the PHP keys). */
    customer_choice?: boolean;
}

interface LimitRow {
    key: string;
    unit: string;
    min: number;
    max: number;
}

function decodePolicy(raw: unknown): { limits: LimitRow[]; policy: PolicyRow[] } {
    const body = raw as { limits?: unknown; policy?: unknown };
    if (!body || !Array.isArray(body.limits) || !Array.isArray(body.policy)) throw new Error('shape');
    return body as { limits: LimitRow[]; policy: PolicyRow[] };
}

const URL_LIMITS = '/api/v1/rights/setting-limits';

interface Draft {
    ceiling: string;
    editable: boolean;
}

const draftOf = (row: PolicyRow): Draft => ({ ceiling: row.server_ceiling === null ? '' : String(row.server_ceiling), editable: row.customer_editable });
const valueOf = (draft: Draft): number | null => (draft.ceiling.trim() === '' ? null : Number(draft.ceiling));
const customerChoice = (row: PolicyRow) => row.customer_choice ?? row.key.startsWith('php.');
const invalidDraft = (draft: Draft, limit: LimitRow | undefined) => {
    const value = valueOf(draft);
    return value !== null && limit !== undefined && (!Number.isFinite(value) || value < limit.min || value > limit.max);
};
const changedDraft = (draft: Draft, row: PolicyRow) => valueOf(draft) !== row.server_ceiling || (customerChoice(row) && draft.editable !== row.customer_editable);

/** The section's rows with one Save; exported for the tests. */
export function PolicyEditor({ policy, limits, observedAt, onSaved }: { policy: PolicyRow[]; limits: LimitRow[]; observedAt: number; onSaved: () => Promise<unknown> }) {
    const { t } = useI18n();
    const [drafts, setDrafts] = useState<Record<string, Draft>>(() => Object.fromEntries(policy.map((row) => [row.key, draftOf(row)])));
    const [errors, setErrors] = useState<Record<string, string>>({});
    const [saving, setSaving] = useState<string | null>(null);
    // A new read replaces what was saved; a row that failed keeps what was typed.
    useEffect(() => {
        setDrafts((previous) => Object.fromEntries(policy.map((row) => [row.key, errors[row.key] && previous[row.key] ? previous[row.key] : draftOf(row)])));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [observedAt]);
    const edit = (key: string, next: Draft) => {
        setDrafts({ ...drafts, [key]: next });
        if (errors[key]) setErrors(Object.fromEntries(Object.entries(errors).filter(([k]) => k !== key)));
    };
    const label = (key: string) => t(`limits.key.${key}` as TranslationKey);
    const limitOf = (key: string) => limits.find((l) => l.key === key);
    const draft = (row: PolicyRow) => drafts[row.key] ?? draftOf(row);
    const changed = policy.filter((row) => changedDraft(draft(row), row));
    const invalid = changed.filter((row) => invalidDraft(draft(row), limitOf(row.key)));

    const save = async () => {
        const failed: Record<string, string> = {};
        const saved: string[] = [];
        let unknown = false;
        for (const row of changed) {
            const d = draft(row);
            setSaving(label(row.key));
            const res = await sendSettings(URL_LIMITS, 'PUT', {
                key: row.key, server_ceiling: valueOf(d), server_default: row.server_default,
                customer_editable: customerChoice(row) ? d.editable : true,
            });
            if (!res) { unknown = true; failed[row.key] = t('common.resultUnknown'); continue; }
            if (!res.ok) { failed[row.key] = apiErrorText(await readApiError(res), t, 'limits.server.failed'); continue; }
            saved.push(label(row.key));
        }
        setSaving(null);
        setErrors(failed);
        if (saved.length > 0) invalidate('rights');
        await onSaved();
        const notSaved = Object.keys(failed).map(label);
        if (notSaved.length > 0) showToast('error', unknown && notSaved.length === 1 ? t('common.resultUnknown') : t('limits.server.partFailed', { keys: notSaved.join(', ') }));
        else showToast('success', t('limits.server.savedAll', { keys: saved.join(', ') }));
    };

    return (
        <div className="mt-3">
            <div className="overflow-x-auto">
                <table className="w-full text-left">
                    <thead>
                        <tr className="text-xs font-semibold text-fg-muted">
                            <th className="px-3 py-1.5" />
                            <th className="px-3 py-1.5">{t('limits.server.ceiling')}</th>
                            <th className="px-3 py-1.5" />
                        </tr>
                    </thead>
                    <tbody>
                        {policy.map((row) => {
                            const d = draft(row);
                            const limit = limitOf(row.key);
                            const value = valueOf(d);
                            const bad = invalidDraft(d, limit);
                            const name = label(row.key);
                            const error = bad && limit ? t('limits.server.invalid', { min: limit.min, max: limit.max }) : errors[row.key];
                            return (
                                <tr key={row.key} className={`border-t border-border/60 ${changedDraft(d, row) ? 'bg-primary/5' : ''}`} data-limit-row={row.key}>
                                    <td className="px-3 py-2 text-sm text-fg">{name}</td>
                                    <td className="px-3 py-2">
                                        <input type="number" value={d.ceiling} min={limit?.min} max={limit?.max} placeholder={t('limits.server.none')}
                                            aria-label={`${name}: ${t('limits.server.ceiling')}`} aria-invalid={bad || undefined} disabled={saving !== null}
                                            onChange={(e) => edit(row.key, { ...d, ceiling: e.target.value })} className={`${inputClass} max-w-36`} />
                                        {value !== null && limit && !bad && <span className="ml-2 text-xs text-fg-subtle">{unitText(t, limit.unit, value)}</span>}
                                        {error && <span className="mt-1 block text-xs text-danger" role="alert" data-limit-error={row.key}>{error}</span>}
                                    </td>
                                    <td className="px-3 py-2">
                                        {customerChoice(row) ? (
                                            <label className="inline-flex items-center gap-2 text-sm text-fg">
                                                <input type="checkbox" checked={d.editable} disabled={saving !== null}
                                                    onChange={(e) => edit(row.key, { ...d, editable: e.target.checked })} />
                                                {t('limits.server.editable')}
                                            </label>
                                        ) : (
                                            <span className="text-xs text-fg-subtle" data-limit-only={row.key}>{t('limits.server.limitOnly')}</span>
                                        )}
                                    </td>
                                </tr>
                            );
                        })}
                    </tbody>
                </table>
            </div>
            <div className="mt-3 flex flex-wrap items-center justify-end gap-3">
                {saving !== null && <Checking label={t('limits.server.saving', { key: saving })} />}
                {saving === null && changed.length > 0 && (
                    <span className="mr-auto text-xs text-fg-muted" data-limits-pending="">{t('limits.server.pending', { keys: changed.map((row) => label(row.key)).join(', ') })}</span>
                )}
                <Button type="button" variant="primary" icon={Save} disabled={changed.length === 0 || invalid.length > 0 || saving !== null} loading={saving !== null}
                    onClick={() => void save()} data-limits-save="">
                    {t('limits.server.saveAll')}
                </Button>
            </div>
        </div>
    );
}

export function ServerSettingLimits({ group }: { group?: string }) {
    const { t } = useI18n();
    const view = useRemote(URL_LIMITS, decodePolicy);
    return (
        <section className="rounded-xl border border-border bg-surface p-4" data-server-limits="">
            <h3 className="text-sm font-semibold text-fg">{t('limits.server.title')}</h3>
            <p className="mt-1 max-w-[75ch] text-xs text-fg-subtle">{t('limits.server.intro')}</p>
            <RemoteGate remote={view.remote} checking={t('limits.checking')} failed={t('limits.unread')} onRetry={() => void view.retry()} busy={view.reading}>
                {(shown) => (
                    <PolicyEditor
                        policy={shown.value.policy.filter((row) => !group || row.key.startsWith(group + '.'))}
                        limits={shown.value.limits}
                        observedAt={shown.observedAt}
                        onSaved={() => view.retry()}
                    />
                )}
            </RemoteGate>
        </section>
    );
}

const PASSWORD_LEVELS = ['owner', 'strong', 'very_strong'] as const;

function decodeLevel(raw: unknown): { level: string } {
    const body = raw as { level?: unknown };
    if (!body || typeof body.level !== 'string') throw new Error('shape');
    return { level: body.level };
}

export function PasswordLevelCard() {
    const { t } = useI18n();
    const view = useRemote('/api/v1/rights/password-level', decodeLevel);
    const [busy, setBusy] = useState(false);
    const choose = async (level: string) => {
        setBusy(true);
        try {
            const res = await sendSettings('/api/v1/rights/password-level', 'PUT', { level });
            if (!res) { showToast('error', t('common.resultUnknown')); return; }
            if (!res.ok) { showToast('error', apiErrorText(await readApiError(res), t, 'limits.password.failed')); return; }
            showToast('success', t('limits.password.saved'));
            void view.retry();
        } finally {
            setBusy(false);
        }
    };
    return (
        <section className="rounded-xl border border-border bg-surface p-4" data-password-level="">
            <h3 className="text-sm font-semibold text-fg">{t('limits.password.title')}</h3>
            <p className="mt-1 max-w-[75ch] text-xs text-fg-subtle">{t('limits.password.intro')}</p>
            <RemoteGate remote={view.remote} checking={t('limits.checking')} failed={t('limits.unread')} onRetry={() => void view.retry()} busy={view.reading}>
                {(shown) => (
                    <div className="mt-2">
                        {PASSWORD_LEVELS.map((level) => (
                            <label key={level} className="flex items-start gap-2 py-1 text-sm text-fg">
                                <input type="radio" name="password-level" checked={shown.value.level === level} disabled={busy}
                                    onChange={() => void choose(level)} className="mt-1" />
                                <span>{t(`limits.password.${level}` as TranslationKey)}</span>
                            </label>
                        ))}
                    </div>
                )}
            </RemoteGate>
        </section>
    );
}
