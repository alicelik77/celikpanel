import { useI18n } from '../i18n';
import { PageHeader } from './PageHeader';
import { DNSServerSettings } from './DNSServerSettings';

// Server → DNS and peer servers (D-037, owner 2026-10-22): the shared
// nameservers, this server's role, the peer server and DNS replication. In a
// two-server layout it is a page that is looked at and operated, not a
// setting chosen once. It was Settings → DNS infrastructure until
// v0.1.0-alpha.98 and is moved as it was; /settings?section=dns and the
// guidance links of earlier releases arrive here.
//
// Sunucu → DNS ve eş sunucular: ortak ad sunucuları, bu sunucunun rolü, eş
// sunucu ve DNS çoğaltması. v0.1.0-alpha.98'e dek Ayarlar → DNS altyapısı idi;
// olduğu gibi taşındı. Eski adres ve önceki sürümlerin bağlantıları buraya ulaşır.
export function DNSPeersPage() {
    const { t } = useI18n();
    return (
        <div className="p-4 sm:p-6 md:p-8">
            <PageHeader title={t('nav.dnsPeers')} subtitle={t('dnsPeers.subtitle')} breadcrumb={[t('common.home'), t('nav.dnsPeers')]} />
            <div className="max-w-5xl">
                <DNSServerSettings />
            </div>
        </div>
    );
}
