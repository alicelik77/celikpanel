// The Turkish half of the screen catalogue part "dbLimits"; ./en.ts defines its keys.
// "dbLimits" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { DbLimitsScreenKey } from './en';

export const trDbLimitsScreens: Record<DbLimitsScreenKey, string> = {
    'pnow.database_limits_unapplied.title': 'Veritabanı sınırları {value} veritabanı kullanıcısına ulaşmadı: {domain}',
    'pnow.database_limits_unapplied.failed': 'Veritabanı sunucusu ya da CelikPanel Agent değişikliği reddetti; bu kullanıcılar önceki sınırlarını koruyor. Nedeni: sudo journalctl -u celikpanel-panel. Bu nedeni giderin, sonra Yeniden uygula’ya basın; bir planın veritabanı sınırlarının sonraki değişikliği de yeniden dener.',
    'pnow.database_limits_unapplied.prerequisite': 'Veritabanı sunucuları ya da CelikPanel Agent çalışmıyordu; hiçbir şey değişmedi, önceki sınırlarını koruyorlar. Sunucu sahibi onu sunucuda başlatır (örneğin sudo systemctl start mariadb ya da postgresql); sonra Yeniden uygula’ya basın.',
    'pnow.database_limits_unapplied.unknown': 'Yanıt alınamadı; bu kullanıcıların sınırlarını alıp almadığı bilinmiyor. Yeniden uygula’ya basın: önce her kullanıcıyı okur, yalnız eksik olanı yazar.',
    'pnow.database_limits_unapplied.action': 'Yeniden uygula',
    'pnow.database_limits_unapplied.step': 'Veritabanı kullanıcılarına sınırları veriliyor',
    'pnow.database_limits_unapplied.done': 'Listelenen veritabanı kullanıcıları artık sınırlarını taşıyor.',
    'pnow.database_limits_unapplied.still': '{n} veritabanı kullanıcısı hâlâ sınırlarını almadı; satır nedenini söyler.',
    'plans.dbLimits.notApplied': 'Kaydedildi. Planın veritabanı sınırları {n} veritabanı kullanıcısına ulaşmadı ({users}); önceki sınırlarını koruyorlar. Şu anki sorunlar onları Yeniden uygula ile birlikte gösterir.',
    'plans.dbLimits.running': 'Kaydedildi. {n} veritabanı kullanıcısına planın veritabanı sınırları hâlâ veriliyor; ulaşmadığı olursa Şu anki sorunlar gösterir.',
};
