import { useEffect, useState } from 'react';
import { RotateCcw, Save } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { Button, Field, FormActions, FormSection, RemoteGate, inputClass } from './ui';
import { apiErrorText, readApiError } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { confirmAction } from '../lib/confirmAction';
import { followRequestProgress, resumeRequestProgress, type StepLabel } from '../lib/requestProgress';
import { answerBody, sendSettings, unitText, whoText } from '../lib/settingLimits';

// A site's PHP settings within its limits (alpha.128, docs/SITE-LIMITS.md):
// GET/PUT /api/v1/domains/{id}/php/settings. Each limited field is a choice of
// presets clipped to the limit, the server's value marked "(Default)", a
// custom value, and "Reset to default" when the site has its own value; the
// limit and who set it are said beside it. "Values PHP uses now" shows what
// PHP really uses and where each value comes from (what a phpinfo page is
// opened for, without publishing one). A save follows the server's steps.
//
// Sitenin sınırlar içindeki PHP ayarları: hazır değerler, "(Varsayılan)",
// özel değer, "Varsayılana dön"; sınır ve koyan yanında yazar. Kayıt
// sunucunun adımlarını izler.

type Say = (key: TranslationKey, vars?: Record<string, string | number>) => string;

interface LimitField {
    field: 'memory' | 'time' | 'upload' | 'input_vars' | 'processes';
    setting: string;
    unit: 'mb' | 's' | 'count' | 'min';
    own: number | null;
    server: number | null;
    server_text?: string;
    default: number | null;
    ceiling: number | null;
    set_by?: string;
    set_by_name?: string;
    presets: number[];
    min: number;
    max: number;
    fixed?: boolean;
    effective: number | null;
    source: 'site' | 'server' | 'plan' | 'nginx_file';
    owner_file?: string;
    server_above_ceiling?: boolean;
}

interface FreeField {
    field: 'display_errors' | 'error_level' | 'timezone' | 'short_open_tag';
    own: string;
    server: string;
    values?: string[];
}

interface LimitsView {
    domain: string;
    php_version: string;
    can_change: boolean;
    closed_by?: string;
    admin: boolean;
    limits: LimitField[];
    free: FreeField[];
    memory_at_once_mb?: number;
}

function decodeView(raw: unknown): LimitsView {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as Record<string, unknown>;
    if (!Array.isArray(body.limits) || !Array.isArray(body.free) || typeof body.domain !== 'string') throw new Error('shape');
    return body as unknown as LimitsView;
}

const FIELD_ORDER: LimitField['field'][] = ['memory', 'time', 'upload', 'input_vars', 'processes'];
const bodyKey: Record<LimitField['field'], string> = { memory: 'memory_mb', time: 'time_s', upload: 'upload_mb', input_vars: 'input_vars', processes: 'processes' };

const stepLabels = (t: Say): StepLabel[] => [
    { id: 'check', label: t('limits.step.check'), check: true },
    { id: 'pool', label: t('limits.step.pool') },
    { id: 'web_server', label: t('limits.step.web_server') },
];

/** One limited field: presets within the limit, the default, a custom value. */
function LimitInput({ f, value, onChange, disabled, t }: { f: LimitField; value: number | null; onChange: (v: number | null) => void; disabled: boolean; t: Say }) {
    const isProcesses = f.field === 'processes';
    const choices = [...new Set([...(f.presets ?? []), ...(value !== null && !isProcesses ? [value] : []), ...(isProcesses && value !== null ? [value] : [])])].sort((a, b) => a - b);
    const [custom, setCustom] = useState(value !== null && !f.presets.includes(value) && !isProcesses);
    const selectValue = custom ? 'custom' : value === null ? 'default' : String(value);
    const top = f.ceiling ?? f.max;
    const invalid = value !== null && (value < f.min || value > top);
    return (
        <div>
            <div className="flex flex-wrap gap-2">
                <select
                    value={selectValue}
                    disabled={disabled}
                    className={inputClass}
                    onChange={(e) => {
                        const v = e.target.value;
                        if (v === 'custom') { setCustom(true); if (value === null) onChange(f.default ?? f.min); return; }
                        setCustom(false);
                        onChange(v === 'default' ? null : Number(v));
                    }}
                >
                    {!isProcesses && <option value="default">{t('limits.php.defaultOption', { value: f.default !== null ? unitText(t, f.unit, f.default) : '—' })}</option>}
                    {choices.map((n) => (
                        <option key={n} value={String(n)}>
                            {unitText(t, f.unit, n)}{n === f.default ? ` ${t('limits.php.defaultMark')}` : ''}
                        </option>
                    ))}
                    {!isProcesses && <option value="custom">{t('limits.php.custom')}</option>}
                </select>
                {(custom || (isProcesses && !choices.includes(value ?? -1))) && (
                    <input
                        type="number"
                        aria-label={t('limits.php.customLabel')}
                        min={f.min}
                        max={top}
                        value={value ?? ''}
                        disabled={disabled}
                        onChange={(e) => onChange(e.target.value === '' ? null : Number(e.target.value))}
                        className={`${inputClass} max-w-32`}
                    />
                )}
            </div>
            <p className="mt-1 text-xs text-fg-subtle">
                {f.ceiling !== null
                    ? t('limits.php.limitBy', { limit: unitText(t, f.unit, f.ceiling), who: whoText(t, f.set_by, f.set_by_name) })
                    : t('limits.php.noLimit', { max: unitText(t, f.unit, f.max) })}
            </p>
            {invalid && <p className="mt-1 text-xs text-danger-fg">{value !== null && value < f.min ? t('limits.php.belowMin', { min: unitText(t, f.unit, f.min) }) : t('limits.php.aboveLimit', { limit: unitText(t, f.unit, top) })}</p>}
            {f.fixed && <p className="mt-1 text-xs text-fg-muted">{t('limits.php.fixed')}</p>}
            {f.server_above_ceiling && f.server !== null && <p className="mt-1 text-xs text-fg-muted">{t('limits.php.serverAbove', { server: unitText(t, f.unit, f.server) })}</p>}
            {f.owner_file && <p className="mt-1 text-xs text-fg-muted">{t('limits.php.ownerFile', { file: f.owner_file })}</p>}
            {!isProcesses && value !== null && !disabled && (
                <button type="button" className="mt-1 inline-flex items-center gap-1 text-xs text-accent hover:underline" onClick={() => { setCustom(false); onChange(null); }}>
                    <RotateCcw className="h-3 w-3" aria-hidden />{t('limits.php.reset')}
                </button>
            )}
        </div>
    );
}

export function SitePHPLimits({ domainId, domainName, readOnly = false }: { domainId: number; domainName: string; readOnly?: boolean }) {
    const { t } = useI18n();
    const url = `/api/v1/domains/${domainId}/php/settings`;
    const view = useRemote(url, decodeView);
    const [draft, setDraft] = useState<Record<string, number | string | null> | null>(null);
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        resumeRequestProgress({
            key: `phplimits.${domainId}`, labels: () => stepLabels(t), t,
            progressTitle: () => t('limits.step.title', { name: domainName }), refusedSentence: 'limits.php.saveFailed',
            ended: () => { void view.retry(); },
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [domainId]);

    return (
        <RemoteGate remote={view.remote} checking={t('limits.php.checking')} failed={t('limits.php.unknown')} onRetry={() => void view.retry()} busy={view.reading}>
            {(shown) => {
                const v = shown.value;
                const fields = FIELD_ORDER.map((name) => v.limits.find((f) => f.field === name)).filter((f): f is LimitField => !!f);
                const current: Record<string, number | string | null> = {};
                for (const f of fields) current[f.field] = f.own;
                for (const f of v.free) current[f.field] = f.own || null;
                const values = draft ?? current;
                const disabled = readOnly || !v.can_change || busy;
                const changed = JSON.stringify(values) !== JSON.stringify(current);
                const invalid = fields.some((f) => {
                    const value = values[f.field] as number | null;
                    return value !== null && (value < f.min || value > (f.ceiling ?? f.max));
                });
                const memory = fields.find((f) => f.field === 'memory');
                const processes = fields.find((f) => f.field === 'processes');
                const memoryNow = (values.memory as number | null) ?? memory?.default ?? null;
                const processesNow = (values.processes as number | null) ?? processes?.effective ?? null;

                const save = async () => {
                    const body: Record<string, number | string> = {};
                    for (const f of fields) body[bodyKey[f.field]] = (values[f.field] as number | null) ?? 0;
                    for (const f of v.free) body[f.field] = (values[f.field] as string | null) ?? '';
                    await confirmAction({
                        message: t('limits.php.save') + ' — ' + domainName,
                        progressTitle: t('limits.step.title', { name: domainName }),
                        action: async (report) => {
                            setBusy(true);
                            const progress = followRequestProgress(report, stepLabels(t), t, `phplimits.${domainId}`);
                            let res: Response | null = null;
                            try {
                                res = await sendSettings(url, 'PUT', body, progress.headers);
                                progress.settle(res);
                                if (!res) return { state: 'unknown', reason: t('common.resultUnknown') };
                                if (!res.ok) return { state: 'failed', reason: apiErrorText(await readApiError(res), t, 'limits.php.saveFailed') };
                                const answer = await answerBody(res);
                                setDraft(null);
                                invalidate('domains');
                                void view.retry();
                                if (answer.nginx === 'kept') showToast('info', t('limits.php.keptNginx'));
                                return { state: 'done', message: t('limits.php.saved', { name: domainName }) };
                            } finally {
                                progress.settle(res);
                                setBusy(false);
                            }
                        },
                    });
                };

                return (
                    <div data-php-limits="">
                        <FormSection title={t('limits.php.title')} description={t('limits.php.intro')}>
                            {!v.can_change && v.closed_by && (
                                <div className="mb-4 rounded-lg border border-warning-mark/60 bg-warning-mark/20 p-3 text-sm text-fg">
                                    {t('limits.php.closed', { who: v.closed_by === 'administrator' ? t('limits.who.server') : t('limits.who.reseller') })}
                                </div>
                            )}
                            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                                {fields.map((f) => (
                                    <Field key={f.field} label={t(`limits.php.field.${f.field}` as TranslationKey)}>
                                        <LimitInput key={`${shown.observedAt}-${f.field}`} f={f} t={t} disabled={disabled || (!!f.fixed && !v.admin)}
                                            value={values[f.field] as number | null}
                                            onChange={(next) => setDraft({ ...values, [f.field]: next })} />
                                        {f.field === 'upload' && <p className="mt-1 text-xs text-fg-subtle">{t('limits.php.hint.upload')}</p>}
                                        {f.field === 'time' && <p className="mt-1 text-xs text-fg-subtle">{t('limits.php.hint.time')}</p>}
                                        {f.field === 'processes' && memoryNow && processesNow && (
                                            <p className="mt-1 text-xs text-fg-subtle">{t('limits.php.processesHint', { n: processesNow, memory: unitText(t, 'mb', memoryNow), total: unitText(t, 'mb', memoryNow * processesNow) })}</p>
                                        )}
                                    </Field>
                                ))}
                            </div>
                        </FormSection>
                        <FormSection title={t('limits.php.free.title')}>
                            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                                {v.free.map((f) => {
                                    const value = (values[f.field] as string | null) ?? '';
                                    const serverOption = f.server ? t('limits.php.opt.server', { value: f.server }) : t('limits.php.opt.serverUnset');
                                    return (
                                        <Field key={f.field} label={t(`limits.php.free.${f.field}` as TranslationKey)}>
                                            {f.field === 'timezone' ? (
                                                <input value={value} disabled={disabled} placeholder={t('limits.php.timezonePlaceholder')} className={inputClass}
                                                    onChange={(e) => setDraft({ ...values, [f.field]: e.target.value.trim() || null })} />
                                            ) : (
                                                <select value={value} disabled={disabled} className={inputClass} onChange={(e) => setDraft({ ...values, [f.field]: e.target.value || null })}>
                                                    <option value="">{serverOption}</option>
                                                    {(f.values ?? []).map((option) => (
                                                        <option key={option} value={option}>
                                                            {t((f.field === 'error_level' ? `limits.php.level.${option}` : `limits.php.opt.${option}`) as TranslationKey)}
                                                        </option>
                                                    ))}
                                                </select>
                                            )}
                                        </Field>
                                    );
                                })}
                            </div>
                            {!readOnly && v.can_change && (
                                <FormActions>
                                    <Button type="button" variant="primary" icon={Save} disabled={!changed || invalid || busy || shown.stale} loading={busy} onClick={() => void save()} data-php-limits-save="">
                                        {t('limits.php.save')}
                                    </Button>
                                </FormActions>
                            )}
                        </FormSection>
                        <FormSection title={t('limits.php.effective.title')} description={t('limits.php.effective.intro')}>
                            <dl className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
                                {fields.map((f) => (
                                    <div key={f.field} className="flex justify-between gap-3 border-b border-border/60 py-1">
                                        <dt className="text-fg-muted">{t(`limits.php.field.${f.field}` as TranslationKey)}</dt>
                                        <dd className="text-fg">
                                            {f.effective !== null ? unitText(t, f.unit, f.effective) : '—'}{' '}
                                            <span className="text-xs text-fg-subtle">({t(`limits.php.source.${f.source}` as TranslationKey)})</span>
                                        </dd>
                                    </div>
                                ))}
                            </dl>
                        </FormSection>
                    </div>
                );
            }}
        </RemoteGate>
    );
}
