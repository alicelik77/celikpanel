import { useState } from 'react';
import { useI18n } from '../i18n';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction } from '../lib/confirmAction';
import { invalidate, useRemote } from '../lib/remote';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { Button, RemoteGate, inputClass } from './ui';

// Per-domain mail service options (alpha.128, gap G25): "send only" and the
// reply unknown addresses get. What changes is said before it is saved; the
// mailboxes and their mail are never touched.
//
// Alan adı başına posta seçenekleri: kaydetmeden önce neyin değişeceği
// söylenir; posta kutuları ve postaları hiç değişmez.

interface Options {
    incoming_off: boolean;
    unknown_reply: string;
}

interface OptionsView {
    options: Options;
    catch_all: boolean;
}

function decodeOptions(raw: unknown): OptionsView {
    const value = raw as Partial<OptionsView> | null;
    if (!value || !value.options || typeof value.options.incoming_off !== 'boolean' || typeof value.catch_all !== 'boolean') throw new Error('options');
    return { options: { incoming_off: value.options.incoming_off, unknown_reply: value.options.unknown_reply ?? '' }, catch_all: value.catch_all };
}

export function DomainMailOptions({ domainId, domainName, readOnly }: { domainId: number; domainName: string; readOnly?: boolean }) {
    const { t } = useI18n();
    const url = `/api/v1/domains/${domainId}/mail/options`;
    const read = useRemote(url, decodeOptions);
    const [draft, setDraft] = useState<Options | null>(null);
    const save = async (current: OptionsView) => {
        const next = draft ?? current.options;
        const lines = [
            next.incoming_off ? t('mailopt.preview.sendOnly', { domain: domainName }) : t('mailopt.preview.receives', { domain: domainName }),
            !next.incoming_off && next.unknown_reply.trim() ? t('mailopt.preview.reply', { reply: next.unknown_reply.trim() }) : t('mailopt.preview.defaultReply'),
            t('mailopt.preview.kept'),
        ];
        await confirmAction({
            title: t('mailopt.title'),
            message: lines.join('\n'),
            confirmLabel: t('mailopt.save'),
            progressTitle: t('mailopt.progress', { domain: domainName }),
            action: async (report) => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'mailOptions', ['save', 'apply']), t, `mailOptions.${domainId}`,
                        (headers) => fetch(url, { method: 'PUT', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify(next) }));
                    if (!res.ok) return { state: 'failed', reason: apiErrorText(await readApiError(res), t) };
                    setDraft(null);
                    return { state: 'done', message: t('mailopt.saved') };
                } catch {
                    return { state: 'unknown', reason: t('common.resultUnknown') };
                } finally {
                    invalidate('mail');
                }
            },
        });
    };
    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-4" data-mail-options>
            <h3 className="mb-1 text-sm font-semibold text-fg">{t('mailopt.title')}</h3>
            <RemoteGate remote={read.remote} checking={t('mailopt.reading')} failed={t('mailopt.failed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value, stale }) => {
                    const shown = draft ?? value.options;
                    const set = (patch: Partial<Options>) => setDraft({ ...shown, ...patch });
                    return (
                        <>
                            <label className="mt-2 flex items-start gap-2 text-sm text-fg">
                                <input type="checkbox" className="mt-1 h-4 w-4" checked={shown.incoming_off} disabled={readOnly || stale}
                                    onChange={(event) => set({ incoming_off: event.target.checked })} />
                                <span>{t('mailopt.sendOnly')}<span className="block text-xs text-fg-subtle">{t('mailopt.sendOnlyHint')}</span></span>
                            </label>
                            {!shown.incoming_off && (
                                <label className="mt-3 block text-sm">
                                    <span className="mb-1 block text-xs text-fg-muted">{t('mailopt.reply')}</span>
                                    {value.catch_all ? (
                                        <span className="block text-xs text-fg-subtle">{t('mailopt.catchAll')}</span>
                                    ) : (
                                        <input type="text" maxLength={200} value={shown.unknown_reply} disabled={readOnly || stale} placeholder={t('mailopt.replyPlaceholder')}
                                            onChange={(event) => set({ unknown_reply: event.target.value })} className={inputClass} />
                                    )}
                                    <span className="mt-1 block text-xs text-fg-subtle">{t('mailopt.replyHint')}</span>
                                </label>
                            )}
                            {!readOnly && (
                                <div className="mt-3">
                                    <Button type="button" variant="primary" disabled={stale || draft === null} onClick={() => void save(value)}>{t('mailopt.save')}</Button>
                                </div>
                            )}
                        </>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

export default DomainMailOptions;
