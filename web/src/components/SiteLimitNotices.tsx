import { useState } from 'react';
import { AlertTriangle, Check } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { Button } from './ui';
import { Link } from '../router';
import { refusalText } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { RIGHTS_NOTICES_URL, decodeNotices, sendRights, type RightsNotice } from '../lib/rights';
import { unitText, whoText } from '../lib/settingLimits';

// The notices of a lowered limit that brought a site's own PHP value down
// (alpha.128, notice kind site_limit_clamped): what changed, that the site
// keeps running at the new limit, who acts and the next step, once per
// person until it is marked seen. The customer and the administrator get a
// way to the site's PHP page.
//
// Düşürülen sınırın bir sitenin PHP değerini indirdiği bildirimler: ne
// değişti, site yeni sınırla çalışıyor, kim ne yapar; kişi başına bir kez.

const KIND = 'site_limit_clamped';

interface Clamped {
    domain: string;
    key: string;
    value: number;
    limit: number;
    set_by?: string;
    set_by_name?: string;
}

const UNITS: Record<string, string> = {
    'php.memory_limit_mb': 'mb', 'php.max_execution_time_s': 's', 'php.upload_max_mb': 'mb', 'php.max_input_vars': 'count', 'php.max_children': 'count',
};

function parse(notice: RightsNotice): Clamped | null {
    let value: unknown;
    try {
        value = JSON.parse(notice.detail);
    } catch {
        value = null;
    }
    const site = value as Clamped | null;
    return site && typeof site.domain === 'string' && typeof site.limit === 'number' && typeof site.value === 'number' ? site : null;
}

export function SiteLimitNotices({ audience }: { audience: 'customer' | 'reseller' | 'administrator' }) {
    const { t } = useI18n();
    const read = useRemote(RIGHTS_NOTICES_URL, decodeNotices);
    const [busy, setBusy] = useState(false);
    if (read.remote.state !== 'known') return null;
    const notices = read.remote.value.filter((notice) => notice.kind === KIND).map((notice) => ({ notice, site: parse(notice) }))
        .filter((entry): entry is { notice: RightsNotice; site: Clamped } => entry.site !== null);
    if (notices.length === 0) return null;
    const seen = async (id: number) => {
        setBusy(true);
        try {
            const result = await sendRights(`${RIGHTS_NOTICES_URL}/${id}/acknowledge`, 'POST');
            if (result.kind === 'lost') showToast('error', t('common.resultUnknown'));
            if (result.kind === 'refused') showToast('error', refusalText(result.error, t, 'rights.notice.ackFailed'));
        } finally {
            invalidate('rights');
            setBusy(false);
        }
    };
    return (
        <section className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-4 text-sm leading-6 text-fg" role="status" data-site-limit-notices={audience}>
            <h3 className="flex items-start gap-2 font-semibold">
                <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                <span>{t('limits.notice.title')}</span>
            </h3>
            <ul className="mt-2 divide-y divide-warning-mark/30">
                {notices.map(({ notice, site }) => {
                    const unit = UNITS[site.key] ?? 'count';
                    const who = whoText(t, site.set_by, site.set_by_name);
                    return (
                        <li key={notice.id} className="flex flex-wrap items-center gap-2 py-2" data-site-limit-notice={site.domain}>
                            <span className="mr-auto max-w-[75ch]">
                                {t('limits.notice.line', { domain: site.domain, key: t(`limits.key.${site.key}` as TranslationKey), value: unitText(t, unit, site.value), limit: unitText(t, unit, site.limit), who })}
                                {' '}
                                <span className="text-fg-muted">{t(audience === 'customer' ? 'limits.notice.todoCustomer' : 'limits.notice.todoProvider', { who })}</span>
                            </span>
                            {audience !== 'reseller' && (
                                <Link to={`/domains/${encodeURIComponent(site.domain)}?tab=php`} className="text-sm font-medium text-accent hover:underline">{t('limits.notice.open')}</Link>
                            )}
                            <Button type="button" icon={Check} onClick={() => void seen(notice.id)} disabled={busy} className="max-sm:min-h-11">{t('rights.notice.seen')}</Button>
                        </li>
                    );
                })}
            </ul>
        </section>
    );
}
