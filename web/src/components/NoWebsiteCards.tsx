import { Globe, Plus } from 'lucide-react';
import { useNavigate } from '../router';
import { useI18n } from '../i18n';
import { useRemote } from '../lib/remote';
import { Button } from './ui';

// The Dashboard's "has no website yet" card for a zone whose DNS is here
// (alpha.114, lab run 2 / S-new-1 on the Dashboard). The main domain's website
// lives on the first server, so a server that joined it never offers one, and
// nothing is offered while the link state is unread. A first server and a
// standalone server (role "first" or "none") still get the card. The card
// leaves by itself when the website exists (the zone list is read again after
// Add domain).
// Panele "henuz web sitesi yok" karti: ilk sunucuya katilmis sunucu sunmaz;
// baglanti durumu okunmadiysa da sunulmaz.
const PEER_URL = '/api/v1/setup/peer';

export function decodePeerRole(raw: unknown): 'none' | 'first' | 'second' {
    const role = raw && typeof raw === 'object' ? (raw as { role?: unknown }).role : undefined;
    if (role === 'none' || role === 'first' || role === 'second') return role;
    throw new Error('invalid peer state');
}

export function NoWebsiteCards({ zones }: { zones: string[] }) {
    const { t } = useI18n();
    const navigate = useNavigate();
    const peer = useRemote(zones.length > 0 ? PEER_URL : null, decodePeerRole);
    const role = peer.remote.state === 'known' ? peer.remote.value : null;
    if (zones.length === 0 || role === null || role === 'second') return null;
    return <>
        {zones.map((zone) => (
            <section key={zone} aria-label={t('dashboard.noWebsite.title', { domain: zone })} data-dashboard-no-website={zone}
                className="mt-6 flex flex-wrap items-center gap-3 rounded-xl border border-border-strong bg-surface p-4">
                <Globe className="h-5 w-5 shrink-0 text-primary" aria-hidden="true" />
                <div className="min-w-0 flex-1">
                    <h2 className="break-words font-semibold">{t('dashboard.noWebsite.title', { domain: zone })}</h2>
                    <p className="mt-1 text-sm text-fg-muted">{t('dashboard.noWebsite.help', { domain: zone })}</p>
                </div>
                <Button variant="primary" icon={Plus} onClick={() => navigate(`/domains?host=${encodeURIComponent(zone)}`)}>{t('dashboard.noWebsite.action')}</Button>
            </section>
        ))}
    </>;
}
