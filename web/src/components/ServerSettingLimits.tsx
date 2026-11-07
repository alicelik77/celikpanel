import { useState } from 'react';
import { Save } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { Button, RemoteGate, inputClass } from './ui';
import { apiErrorText, readApiError } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { sendSettings, unitText } from '../lib/settingLimits';

// The server's limits of site settings (alpha.128, docs/SITE-LIMITS.md),
// administrator only: one row per key with the server limit (empty = no
// limit) and whether customers may change it. GET/PUT
// /api/v1/rights/setting-limits. Saving one row saves that key only.
// Also the administrator's password level (G21), on the Security page.
//
// Sunucunun site ayarı sınırları ve parola düzeyi; yalnız yönetici.

interface PolicyRow {
    key: string;
    server_ceiling: number | null;
    server_default: number | null;
    customer_editable: boolean;
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

function PolicyRowEditor({ row, limit }: { row: PolicyRow; limit: LimitRow | undefined }) {
    const { t } = useI18n();
    const [ceiling, setCeiling] = useState(row.server_ceiling === null ? '' : String(row.server_ceiling));
    const [editable, setEditable] = useState(row.customer_editable);
    const [busy, setBusy] = useState(false);
    const value = ceiling.trim() === '' ? null : Number(ceiling);
    const invalid = value !== null && limit !== undefined && (!Number.isFinite(value) || value < limit.min || value > limit.max);
    const changed = value !== row.server_ceiling || editable !== row.customer_editable;
    const label = t(`limits.key.${row.key}` as TranslationKey);
    const save = async () => {
        setBusy(true);
        try {
            const res = await sendSettings(URL_LIMITS, 'PUT', { key: row.key, server_ceiling: value, server_default: row.server_default, customer_editable: editable });
            if (!res) { showToast('error', t('common.resultUnknown')); return; }
            if (!res.ok) { showToast('error', apiErrorText(await readApiError(res), t, 'limits.server.failed')); return; }
            showToast('success', t('limits.server.saved', { key: label }));
            invalidate('rights');
        } finally {
            setBusy(false);
        }
    };
    return (
        <tr className="border-t border-border/60">
            <td className="px-3 py-2 text-sm text-fg">{label}</td>
            <td className="px-3 py-2">
                <input type="number" value={ceiling} min={limit?.min} max={limit?.max} placeholder={t('limits.server.none')}
                    aria-label={`${label}: ${t('limits.server.ceiling')}`} aria-invalid={invalid || undefined}
                    onChange={(e) => setCeiling(e.target.value)} className={`${inputClass} max-w-36`} />
                {value !== null && limit && !invalid && <span className="ml-2 text-xs text-fg-subtle">{unitText(t, limit.unit, value)}</span>}
            </td>
            <td className="px-3 py-2">
                <label className="inline-flex items-center gap-2 text-sm text-fg">
                    <input type="checkbox" checked={editable} onChange={(e) => setEditable(e.target.checked)} />
                    {t('limits.server.editable')}
                </label>
            </td>
            <td className="px-3 py-2 text-right">
                <Button type="button" icon={Save} disabled={!changed || invalid || busy} loading={busy} onClick={() => void save()}>{t('limits.server.save')}</Button>
            </td>
        </tr>
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
                    <div className="mt-3 overflow-x-auto">
                        <table className="w-full text-left">
                            <tbody>
                                {shown.value.policy.filter((row) => !group || row.key.startsWith(group + '.')).map((row) => (
                                    <PolicyRowEditor key={`${shown.observedAt}-${row.key}`} row={row} limit={shown.value.limits.find((l) => l.key === row.key)} />
                                ))}
                            </tbody>
                        </table>
                    </div>
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
