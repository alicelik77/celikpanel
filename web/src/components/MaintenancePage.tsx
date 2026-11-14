import { useI18n } from '../i18n';
import { PageHeader } from './PageHeader';
import { SystemSQLiteManager } from './SystemSQLiteManager';
import { RestartsCard } from './RestartsCard';
import { ServerClockCard, ServerProcessesCard, ServerRebootCard } from './ServerMaintenanceCards';
import { ServerCertificatesCard } from './ServerCertificatesCard';
import { DeletionHoldingCard } from './DeletionHoldingCard';

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
            <div className="grid max-w-7xl gap-5">
                {/* alpha.107 (D-038 §10): what waits for a restart or a reboot. */}
                {/* alpha.135: what deletions left, kept 30 days (the home page and the disk warning open it). */}
                <div className="max-w-5xl"><DeletionHoldingCard /></div>
                <div className="max-w-5xl"><RestartsCard /></div>
                {/* alpha.128 (G29): restarting the server, its clock and its busiest processes. */}
                <div className="max-w-5xl"><ServerRebootCard /></div>
                <div className="max-w-5xl"><ServerClockCard /></div>
                <div className="max-w-5xl"><ServerProcessesCard /></div>
                {/* alpha.128 (G31): every certificate in use, and certificate requests. */}
                <div className="max-w-5xl"><ServerCertificatesCard /></div>
                <SystemSQLiteManager />
            </div>
        </div>
    );
}
