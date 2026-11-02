// The security notice a customer or reseller sees above every page while a
// security emergency (D-043) affects their sites: what happened, why, what
// happened to the sites, what to do, and a one-click switch of each site to
// the recommended version through the ordinary site settings path (the
// server checks the person's rights as for any other change). It stays until
// marked as seen; a "resolved" notice follows when the administrator
// re-enables the component.
//
// Güvenlik bildirimi: ne oldu, neden, sitelere ne oldu, ne yapmalı; her site
// için önerilen sürüme tek tıkla geçiş. Görüldü işaretlenene dek durur.
import { useState } from 'react';
import { AlertTriangle, Check } from 'lucide-react';
import { useI18n } from '../i18n';
import { useAuth } from '../auth/AuthContext';
import { Button, ErrorBanner } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';

export const SECURITY_NOTICES_URL = '/api/v1/security-notices';

export interface SecurityNotice {
    id: number;
    kind: 'emergency' | 'resolved';
    audience: 'customer' | 'reseller';
    body: { component: string; version: string; name: string; reason: string; recommended?: string; sites: { domain_id: number; domain: string; customer?: string; maintenance: boolean }[] };
    actions: { domain_id: number; domain: string; kind: 'php' | 'node'; to: string }[];
}

export function decodeSecurityNotices(raw: unknown): SecurityNotice[] {
    const body = raw as { notices?: unknown[] } | null;
    if (!body || !Array.isArray(body.notices)) throw new Error('shape');
    return (body.notices as SecurityNotice[]).filter((notice) => notice && typeof notice.id === 'number' && notice.body && Array.isArray(notice.body.sites))
        .map((notice) => ({ ...notice, actions: Array.isArray(notice.actions) ? notice.actions : [] }));
}

/** The request of a one-click switch: the site's ordinary version change. */
export function switchRequest(action: SecurityNotice['actions'][number]): { url: string; body: Record<string, string> } {
    return action.kind === 'php'
        ? { url: `/api/v1/domains/${action.domain_id}/php`, body: { php_version: action.to } }
        : { url: `/api/v1/domains/${action.domain_id}/app/node-version`, body: { version: action.to } };
}

const componentName = (component: string) => ({ php: 'PHP', node: 'Node.js', mariadb: 'MariaDB', postgresql: 'PostgreSQL' } as Record<string, string>)[component] ?? component;

export function SecurityNoticeBanner() {
    const { role } = useAuth();
    const { t } = useI18n();
    const read = useRemote(role === 'admin' ? null : SECURITY_NOTICES_URL, decodeSecurityNotices);
    const [busy, setBusy] = useState<string | null>(null);
    const [error, setError] = useState<ApiError | null>(null);
    const [done, setDone] = useState<string[]>([]);
    if (read.remote.state !== 'known' || read.remote.value.length === 0) return null;
    const act = async (key: string, url: string, body: unknown, after?: string) => {
        if (busy) return;
        setBusy(key);
        setError(null);
        try {
            const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
            if (!res.ok) setError(await readApiError(res));
            else if (after) setDone((list) => [...list, after]);
        } catch {
            setError({ message: after ? t('notice.emg.switchLost', { site: key }) : t('common.resultUnknown') });
        } finally {
            setBusy(null);
            void read.retry();
        }
    };
    return (
        <div className="mb-4 grid gap-3" data-security-notices={read.remote.value.length}>
            {done.map((text, index) => <p key={index} className="rounded-lg border border-success/40 bg-success/10 px-3 py-2 text-sm text-fg" role="status">{text}</p>)}
            <ErrorBanner error={error} />
            {read.remote.value.map((notice) => {
                const name = notice.body.name;
                if (notice.kind === 'resolved') {
                    return (
                        <section key={notice.id} className="rounded-lg border border-success/40 bg-success/10 p-4 text-sm leading-6 text-fg" role="status" data-security-notice="resolved">
                            <h3 className="font-semibold">{t('notice.emg.resolvedTitle', { name })}</h3>
                            <p className="mt-1 max-w-[75ch]">{t('notice.emg.resolved', { name })}</p>
                            <Button icon={Check} className="mt-2 max-sm:min-h-11" onClick={() => void act(`seen-${notice.id}`, `${SECURITY_NOTICES_URL}/${notice.id}/seen`, {})} disabled={busy !== null}>{t('notice.emg.seen')}</Button>
                        </section>
                    );
                }
                return (
                    <section key={notice.id} className="rounded-lg border border-danger/40 bg-danger/10 p-4 text-sm leading-6 text-fg" role="alert" data-security-notice="emergency">
                        <h3 className="flex items-start gap-2 text-base font-semibold">
                            <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-danger" aria-hidden="true" />
                            <span>{t('notice.emg.title', { name })}</span>
                        </h3>
                        <p className="mt-1 max-w-[75ch]">{t('notice.emg.what', { name })}</p>
                        <p className="max-w-[75ch]">{t('notice.emg.why', { reason: notice.body.reason })}</p>
                        <p className="mt-2 font-medium">{notice.audience === 'reseller' ? t('notice.emg.sitesReseller') : t('notice.emg.sites')}</p>
                        <ul className="ml-4 list-disc">
                            {notice.body.sites.map((site) => <li key={site.domain_id}>{site.domain}{site.customer ? ` (${site.customer})` : ''}</li>)}
                        </ul>
                        <p className="mt-2 max-w-[75ch]">{t('notice.emg.nothingLost')}</p>
                        <p className="max-w-[75ch]"><span className="font-semibold">{notice.body.version ? t('notice.emg.todo') : t('notice.emg.todoWhole', { name })}</span></p>
                        <div className="mt-2 flex flex-wrap gap-2">
                            {notice.actions.map((action) => {
                                const request = switchRequest(action);
                                const words = { site: action.domain, name: componentName(action.kind), version: action.to };
                                return (
                                    <Button key={action.domain_id} variant="primary" className="max-sm:min-h-11" loading={busy === action.domain}
                                        disabled={busy !== null} onClick={() => void act(action.domain, request.url, request.body, t('notice.emg.switched', words))}
                                        data-security-switch={action.domain_id}>
                                        {t('notice.emg.switch', words)}
                                    </Button>
                                );
                            })}
                            <Button icon={Check} className="max-sm:min-h-11" onClick={() => void act(`seen-${notice.id}`, `${SECURITY_NOTICES_URL}/${notice.id}/seen`, {})} disabled={busy !== null}>{t('notice.emg.seen')}</Button>
                        </div>
                    </section>
                );
            })}
        </div>
    );
}
