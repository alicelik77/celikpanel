// Home → Operating-system updates (alpha.114). A compact card on the
// administrator's Dashboard: how many updates are waiting, how many of them
// fix security problems, when that was last read, and a link to the full
// Security updates page. It only reads the Panel's last reading; asking again
// never starts a package operation or any change. While the first reading is
// under way it asks again every few seconds (a read) and says nothing needs
// doing. Unknown is never shown as "nothing waiting".
//
// Gösterge Paneli: bekleyen işletim sistemi güncellemeleri. Yalnız okur;
// yeniden sorulması hiçbir paket işlemi ya da değişiklik başlatmaz.
import { useEffect, useRef } from 'react';
import { ShieldCheck } from 'lucide-react';
import { Link } from '../router';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useRemote } from '../lib/remote';
import { KeepWords } from './ui';
import { LifeLine, lifeDate, type Tone } from './LifeLine';

export interface OSUpdatesSummary {
    state: 'ready' | 'checking' | 'unknown';
    stale?: boolean;
    checked_at?: string;
    package_family?: string;
    pending?: { state: string; total: number; security: number; lists_at?: string; reason?: string; security_state?: string };
    automatic?: { state: string; tool?: string; reason?: string };
}

export const SUMMARY_URL = '/api/v1/security/os-updates/summary';
// A reading is asked for again this often while one is under way, at most this many times.
const POLL_MS = 5000;
const POLL_LIMIT = 30;

export function decodeOSUpdatesSummary(raw: unknown): OSUpdatesSummary {
    const body = raw as Partial<OSUpdatesSummary> | null;
    if (!body || typeof body !== 'object' || (body.state !== 'ready' && body.state !== 'checking' && body.state !== 'unknown')) throw new Error('shape');
    if (body.state === 'ready' && (!body.pending || typeof body.pending.total !== 'number' || typeof body.pending.state !== 'string')) throw new Error('shape');
    return body as OSUpdatesSummary;
}

export function PendingUpdatesCard() {
    const { t, locale } = useI18n();
    const handle = useRemote(SUMMARY_URL, decodeOSUpdatesSummary);
    const value = handle.remote.state === 'known' ? handle.remote.value : undefined;
    const underway = value?.state === 'checking' || (value?.state === 'ready' && value.stale === true);
    const polls = useRef(0);
    const observed = handle.remote.state === 'known' ? handle.remote.observedAt : 0;
    useEffect(() => {
        if (!underway || polls.current >= POLL_LIMIT) return;
        const timer = setTimeout(() => {
            polls.current += 1;
            void handle.retry();
        }, POLL_MS);
        return () => clearTimeout(timer);
    }, [underway, observed]);

    let body: JSX.Element;
    if (handle.remote.state === 'loading' || value?.state === 'checking') {
        body = <LifeLine tone="info" attr="pending-checking">{t('life.pending.checking')}</LifeLine>;
    } else if (!value || value.state === 'unknown') {
        body = <LifeLine tone="warn" attr="pending-unknown">{t('life.pending.unknown')}</LifeLine>;
    } else {
        body = <Ready value={value} locale={locale} />;
    }
    return (
        <section className="mb-4 rounded-xl border border-border bg-surface p-5" data-pending-updates={value?.state ?? 'loading'}>
            <div className="flex flex-wrap items-center justify-between gap-3">
                <h2 className="flex items-center gap-2 text-sm font-medium text-fg-muted">
                    <ShieldCheck className="h-4 w-4" aria-hidden="true" />
                    {t('life.pending.title')}
                </h2>
                <Link to="/security" className="text-sm font-semibold text-primary" data-pending-updates-link="">
                    {t('life.pending.open')}
                </Link>
            </div>
            <div className="mt-3 space-y-2">{body}</div>
        </section>
    );
}

function Ready({ value, locale }: { value: OSUpdatesSummary; locale: string }) {
    const { t } = useI18n();
    const pending = value.pending!;
    const arch = value.package_family === 'pacman';
    const auto = value.automatic?.state ?? 'unknown';
    const securityUnknown = pending.security_state === 'arch_audit_missing' || pending.security_state === 'arch_audit_failed';
    const date = lifeDate(pending.lists_at, locale, true);
    const read = lifeDate(value.checked_at, locale, true);
    const when = date ? t('life.pending.lists', { date }) : read ? t('life.pending.read', { date: read }) : '';

    let tone: Tone = 'ok';
    let headline: string;
    if (pending.state !== 'known') {
        tone = 'info';
        headline = pending.reason === 'checkupdates_missing'
            ? t('life.updates.pending.checkupdatesMissing')
            : pending.reason === 'checkupdates_failed'
              ? t('life.updates.pending.checkupdatesFailed')
              : t(pending.state === 'busy' ? 'life.updates.pending.busy' : 'life.updates.pending.unknown');
    } else if (pending.total === 0) {
        headline = t('life.pending.none');
    } else {
        tone = pending.security > 0 ? 'warn' : 'info';
        headline = t('life.pending.total', { count: pending.total });
    }
    const modeKey: TranslationKey = arch
        ? 'life.pending.mode.arch'
        : auto === 'on'
          ? 'life.pending.mode.on'
          : auto === 'off' || auto === 'not_installed' || auto === 'not_applicable'
            ? 'life.pending.mode.off'
            : 'life.pending.mode.unknown';
    return (
        <div className="space-y-1.5" data-pending-ready="">
            <LifeLine tone={tone} attr="pending-count">
                <KeepWords text={headline} />
            </LifeLine>
            {pending.state === 'known' && pending.total > 0 && (
                <p className="pl-6 text-sm text-fg" data-pending-security="">
                    {securityUnknown ? t('life.pending.securityUnknown') : t('life.pending.security', { count: pending.security })}
                </p>
            )}
            <p className="pl-6 text-sm text-fg-muted" data-pending-mode={auto}>{t(modeKey)}</p>
            {when && <p className="pl-6 text-xs text-fg-subtle" data-pending-when="">{when}</p>}
            {value.stale && <p className="pl-6 text-xs text-fg-subtle" data-pending-stale="">{t('life.pending.stale')}</p>}
        </div>
    );
}
