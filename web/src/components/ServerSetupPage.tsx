import { useI18n } from '../i18n';
import { PageHeader } from './PageHeader';
import { ServerSetupSettings } from './ServerSetupChoice';
import { OSReleaseCard } from './OSReleaseCard';

// Server → Server setup (v0.1.0-alpha.100, owner 2026-10-22): the server's
// purpose, the state of its setup, the setup wizard and extending what the
// server does. It was Settings → Server setup until v0.1.0-alpha.100 and is
// moved as it was; its old Settings address and the guidance links of earlier
// releases arrive here. The wizard itself stays at /setup.
//
// Sunucu → Sunucu kurulumu: sunucunun kullanım amacı, kurulumun durumu,
// kurulum sihirbazı ve sunucunun yaptıklarını genişletme. v0.1.0-alpha.100'e
// dek Ayarlar → Sunucu kurulumu idi; olduğu gibi taşındı. Eski adres ve önceki
// sürümlerin bağlantıları buraya ulaşır. Sihirbazın kendisi /setup'ta kalır.
export function ServerSetupPage() {
    const { t } = useI18n();
    return (
        <div className="p-4 sm:p-6 md:p-8">
            <PageHeader title={t('nav.serverSetup')} subtitle={t('setup.pageSubtitle')} breadcrumb={[t('common.home'), t('nav.serverSetup')]} />
            <div className="max-w-5xl">
                <ServerSetupSettings />
                {/* alpha.107 (D-038 §17): the release and what an OS upgrade changed. */}
                <OSReleaseCard />
            </div>
        </div>
    );
}
