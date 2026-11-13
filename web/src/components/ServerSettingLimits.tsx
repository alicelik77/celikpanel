import { useMemo, useState } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { RemoteGate, inputClass } from './ui';
import { apiErrorText, readApiError } from '../lib/apiError';
import { invalidate, lastKnown, useRemote } from '../lib/remote';
import { answerWasLost } from '../lib/requestIdentity';
import { sendSettings, unitText } from '../lib/settingLimits';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

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
// alpha.132: choosing a level no longer saves it, and the limits' Save is
// the shared save bar: changes are marked and saved with it
// (components/kit/save).
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

/**
 * The section's rows with one save bar; exported for the tests.
 * alpha.132: the shared draft (components/kit/save): a changed row is marked,
 * Discard gives every edit up, leaving asks, and a row the server changed
 * while it was being edited says so. Save stays one PUT per changed row; a
 * row's error stays beside the row and the row stays a marked change.
 */
export function PolicyEditor({ policy, limits, observedAt, onSaved, stale = false, reading = false }: {
    policy: PolicyRow[];
    limits: LimitRow[];
    observedAt: number;
    onSaved: () => Promise<unknown>;
    /** What is shown is an earlier answer: Save waits for a new read. */
    stale?: boolean;
    /** The limits are being read again. */
    reading?: boolean;
}) {
    const { t } = useI18n();
    const say = useSaveWords();
    const keys = policy.map((row) => row.key).join(' ');
    // A new read is a new answer; the same read filtered again is not.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    const known = useMemo(() => Object.fromEntries(policy.map((row) => [row.key, draftOf(row)])), [observedAt, keys]);
    const draft = useDraft<Record<string, Draft>>(known);
    const [errors, setErrors] = useState<Record<string, string>>({});
    const label = (key: string) => t(`limits.key.${key}` as TranslationKey);
    const limitOf = (key: string) => limits.find((l) => l.key === key);
    const rowDraft = (row: PolicyRow) => draft.values?.[row.key] ?? draftOf(row);
    const edit = (key: string, next: Draft) => {
        draft.set(key, next);
        if (errors[key]) setErrors(Object.fromEntries(Object.entries(errors).filter(([k]) => k !== key)));
    };
    const invalid = policy.filter((row) => draft.isChanged(row.key) && invalidDraft(rowDraft(row), limitOf(row.key)));
    const rowWords = (d: Draft) => (d.ceiling.trim() === '' ? t('limits.server.none') : d.ceiling);

    const save = () => void draft.save(async (changes) => {
        const failed: Record<string, string> = {};
        const saved: string[] = [];
        let unknown = false;
        for (const row of policy) {
            const d = changes[row.key];
            if (!d) continue;
            const res = await sendSettings(URL_LIMITS, 'PUT', {
                key: row.key, server_ceiling: valueOf(d), server_default: row.server_default,
                customer_editable: customerChoice(row) ? d.editable : true,
            });
            if (!res || answerWasLost(res)) { unknown = true; failed[row.key] = t('common.resultUnknown'); continue; }
            if (!res.ok) { failed[row.key] = apiErrorText(await readApiError(res), t, 'limits.server.failed'); continue; }
            saved.push(label(row.key));
        }
        setErrors(failed);
        if (saved.length > 0) invalidate('rights');
        await onSaved();
        const notSaved = Object.keys(failed).map(label);
        if (notSaved.length === 0) {
            showToast('success', t('limits.server.savedAll', { keys: saved.join(', ') }));
            return 'saved';
        }
        showToast('error', unknown && notSaved.length === 1 ? t('common.resultUnknown') : t('limits.server.partFailed', { keys: notSaved.join(', ') }));
        return unknown ? 'unknown' : 'refused';
    });

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
                            const d = rowDraft(row);
                            const limit = limitOf(row.key);
                            const value = valueOf(d);
                            const bad = invalidDraft(d, limit);
                            const name = label(row.key);
                            const changed = draft.isChanged(row.key);
                            const error = bad && limit ? t('limits.server.invalid', { min: limit.min, max: limit.max }) : errors[row.key];
                            return (
                                <tr key={row.key} className={`border-t border-border/60 ${changed ? changedRowClass : ''}`} data-limit-row={row.key}>
                                    <td className="px-3 py-2 text-sm text-fg">
                                        {name}
                                        {changed && <ChangedMark />}
                                        {draft.isMoved(row.key) && draft.base && (
                                            <ServerMovedNote value={rowWords(draft.base[row.key])} onTake={() => draft.take(row.key)} />
                                        )}
                                    </td>
                                    <td className="px-3 py-2">
                                        <input type="number" value={d.ceiling} min={limit?.min} max={limit?.max} placeholder={t('limits.server.none')}
                                            aria-label={`${name}: ${t('limits.server.ceiling')}`} aria-invalid={bad || undefined} disabled={draft.saving}
                                            onChange={(e) => edit(row.key, { ...d, ceiling: e.target.value })} className={`${inputClass} field-sm`} />
                                        {value !== null && limit && !bad && <span className="ml-2 text-xs text-fg-subtle">{unitText(t, limit.unit, value)}</span>}
                                        {error && <span className="mt-1 block text-xs text-danger" role="alert" data-limit-error={row.key}>{error}</span>}
                                    </td>
                                    <td className="px-3 py-2">
                                        {customerChoice(row) ? (
                                            <label className="inline-flex items-center gap-2 text-sm text-fg">
                                                <input type="checkbox" checked={d.editable} disabled={draft.saving}
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
            <SaveBar
                className="-mx-4 px-4 pb-0"
                count={draft.count}
                saving={draft.saving}
                blockedBy={stale ? say('stale') : invalid.length > 0 ? t('limits.server.fixFirst') : undefined}
                unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: reading, onCheck: () => void onSaved() } : undefined}
                onSave={save}
                onDiscard={() => { setErrors({}); draft.reset(); }}
            />
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
                        stale={shown.stale}
                        reading={view.reading}
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
    const say = useSaveWords();
    const view = useRemote('/api/v1/rights/password-level', decodeLevel);
    const draft = useDraft(lastKnown(view.remote)?.value);
    const levelText = (level: string) => t(`limits.password.${level}` as TranslationKey);
    const save = () => void draft.save(async (changes) => {
        const res = await sendSettings('/api/v1/rights/password-level', 'PUT', { level: changes.level });
        // No answer, or a gateway's in the Panel's place: not known.
        if (!res || answerWasLost(res)) return 'unknown';
        if (!res.ok) {
            showToast('error', apiErrorText(await readApiError(res), t, 'limits.password.failed'));
            return 'refused';
        }
        showToast('success', t('limits.password.saved'));
        await view.retry();
        return 'saved';
    });
    return (
        <section className="rounded-xl border border-border bg-surface p-4" data-password-level="">
            <h3 className="text-sm font-semibold text-fg">{t('limits.password.title')}</h3>
            <p className="mt-1 max-w-[75ch] text-xs text-fg-subtle">{t('limits.password.intro')}</p>
            <RemoteGate remote={view.remote} checking={t('limits.checking')} failed={t('limits.unread')} onRetry={() => void view.retry()} busy={view.reading}>
                {(shown) => {
                    const chosen = draft.values?.level ?? shown.value.level;
                    const changed = draft.isChanged('level');
                    return (
                        <div className="mt-2">
                            {PASSWORD_LEVELS.map((level) => (
                                <label key={level} className={`-mx-2 flex items-start gap-2 rounded-r px-2 py-1 text-sm text-fg ${changed && chosen === level ? changedRowClass : ''}`}>
                                    <input type="radio" name="password-level" checked={chosen === level} disabled={draft.saving}
                                        onChange={() => draft.set('level', level)} className="mt-1" />
                                    <span>
                                        {levelText(level)}
                                        {changed && chosen === level && <ChangedMark />}
                                    </span>
                                </label>
                            ))}
                            {draft.isMoved('level') && draft.base && (
                                <ServerMovedNote value={levelText(draft.base.level)} onTake={() => draft.take('level')} />
                            )}
                            <SaveBar
                                className="-mx-4 px-4 pb-0"
                                count={draft.count}
                                saving={draft.saving}
                                blockedBy={shown.stale ? say('stale') : undefined}
                                unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: view.reading, onCheck: () => void view.retry() } : undefined}
                                onSave={save}
                                onDiscard={draft.reset}
                            />
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}
