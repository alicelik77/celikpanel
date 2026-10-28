import { useI18n } from '../i18n';
import { PageHeader } from './PageHeader';
import { DNSServerSettings } from './DNSServerSettings';
import { DNSOverview } from './DNSOverview';

// Server → DNS (D-037; the new page since v0.1.0-alpha.115, owner-approved
// draft 2026-10-26): one status sentence first, the DNS servers, the names
// with what the internet answers, the mail records and your own records.
// Today's technical screen (DNS software, pairing, the engine card) is kept
// whole under Advanced, collapsed. /settings?section=dns and the guidance
// links of earlier releases arrive here.
//
// Sunucu → DNS: once tek cumlelik durum, sunucular, adlar, posta ve kendi
// kayitlariniz; bugunku teknik ekran Gelismis altinda kapali ve oldugu gibi.
export function DNSPeersPage() {
    const { t } = useI18n();
    return (
        <div className="p-4 sm:p-6 md:p-8">
            <PageHeader title={t('nav.dnsPeers')} subtitle={t('dnsPeers.subtitle')} breadcrumb={[t('common.home'), t('nav.dnsPeers')]} />
            <div className="max-w-5xl">
                <DNSOverview advanced={<DNSServerSettings />} />
            </div>
        </div>
    );
}
