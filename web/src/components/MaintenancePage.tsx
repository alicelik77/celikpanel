import { useI18n } from '../i18n';
import { PageHeader } from './PageHeader';
import { SystemSQLiteManager } from './SystemSQLiteManager';

// Server → Maintenance (D-037, owner 2026-10-22): inspecting and safely
// maintaining the SQLite files CelikPanel and the installed services keep.
// It was Settings → System databases until v0.1.0-alpha.98 and is moved as it
// was; /settings?section=system-databases arrives here. The page promises
// nothing that does not exist yet.
//
// Sunucu → Bakım: CelikPanel'in ve kurulu servislerin tuttuğu SQLite
// dosyalarını incelemek ve güvenle bakımını yapmak. v0.1.0-alpha.98'e dek
// Ayarlar → Sistem veritabanları idi; olduğu gibi taşındı. Sayfa henüz
// olmayan bir şeyi vaat etmez.
export function MaintenancePage() {
    const { t } = useI18n();
    return (
        <div className="p-4 sm:p-6 md:p-8">
            {/* The card below says what the page is for (its heading and subtitle);
                the header does not repeat it (alpha.98 review). */}
            <PageHeader title={t('nav.maintenance')} breadcrumb={[t('common.home'), t('nav.maintenance')]} />
            <div className="max-w-7xl">
                <SystemSQLiteManager />
            </div>
        </div>
    );
}
