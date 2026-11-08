// The Turkish half of the screen catalogue part "pgLeftovers"; ./en.ts defines its keys.
// "pgLeftovers" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { PgLeftoversScreenKey } from './en';

export const trPgLeftoversScreens: Record<PgLeftoversScreenKey, string> = {
    'pgc.leftover.badge': 'Kullanılmıyor — eski sürüm {version}',
    'pgc.leftover.fresh': 'PostgreSQL {version} bu sunucudan kaldırıldı. Bu kümede yalnız PostgreSQL’in kurulurken oluşturdukları var; bu yüzden CelikPanel onu açıldıktan kısa süre sonra kendisi kaldırır ve port {port} boşa çıkar. Sizin bir şey yapmanız gerekmiyor.',
    'pgc.leftover.kept': 'PostgreSQL {version} bu sunucudan kaldırıldı, ama bu kümenin dosyaları verilerinizi içerebileceği için yerinde bırakıldı. CelikPanel burada yöneticiye sormadan hiçbir şey silmez. Küme çalışamaz ve hiçbir site onu kullanmaz; yalnız port {port} hâlâ onun adına ayrılmış durumda. Artık gerekmiyorsa Arşivle ve kaldır, önce bütün dosyalarının bir kopyasını saklar.',
    'pgc.leftover.reason': 'Neden saklandı: {reason}.',
    'pgc.leftover.archive': 'Arşivle ve kaldır',
    'pgc.leftover.confirmTitle': '{cluster} kümesi arşivlenip kaldırılsın mı?',
    'pgc.leftover.confirmBody': 'Önce dosyaları — /etc/postgresql/{version}/{name} içindeki yapılandırma, {data} veri dizini ve günlükleri — /var/backups/celikpanel altında yalnız root’un okuyabildiği bir arşive alınır; arşivin tam adı iş bitince gösterilir. Sonra bu dosyalar sunucudan silinir.\nHâlâ ona ayrılmış olan port {port} boşa çıkar.\nHiçbir site onu kullanmıyor: programı olmadan çalışamaz.',
    'pgc.leftover.progress': '{cluster} kümesi arşivlenip kaldırılıyor',
    'pgc.leftover.done': '{cluster} kümesi {archive} arşivine alındı ve kaldırıldı. Port {port} artık boş.',
    'pgc.leftover.failed': 'Küme arşivlenmedi ve kaldırılmadı.',
    'pgc.leftover.unchanged': 'Hiçbir şey değiştirilmedi: {reason}',
    'pgc.leftover.partial': '{cluster} kümesi {archive} arşivine alındı, ama dosyaları silinirken iş yarıda kaldı; bazıları hâlâ sunucuda olabilir. Arşiv yerinde duruyor. Sonraki adım: Arşivle ve kaldır’ı yeniden deneyin ya da kalanları kendiniz silin. Sunucunun yanıtı: {reason}',
    'pgc.leftover.unknown': 'Yanıt gelmedi; bu yüzden {cluster} kümesinin arşivlenip kaldırıldığı bilinmiyor. Kümeler yeniden okunuyor; hiçbir şey ikinci kez gönderilmez.',
    'dashboard.audit.action.postgresql.cluster.leftover.removed': 'kaldırılmış bir PostgreSQL sürümünden kalan kümeyi kaldırdı ({target})',
    'dashboard.audit.action.postgresql.cluster.leftover.failed': 'kaldırılmış bir PostgreSQL sürümünden kalan kümeyi kaldıramadı ({target})',
    'pgc.pick.label': 'Dosyaları düzenlenen küme',
    'pgc.pick.cluster': 'PostgreSQL {version} · {name}',
    'opstep.pgLeftover.check': '{cluster} kümesinin hiçbir şeyin kullanmadığı bir kalıntı olduğu denetleniyor',
    'opstep.pgLeftover.archive': 'Dosyalar /var/backups/celikpanel altında arşive alınıyor, sonra sunucudan siliniyor',
    'err.PG_NOT_A_LEFTOVER': 'Hiçbir şey değiştirilmedi: bu küme artık kaldırılmış bir PostgreSQL sürümünden kalan küme değil (yeniden çalışıyor ya da artık yok). Liste kümeleri şu anki halleriyle gösteriyor.',
    'err.PG_LEFTOVER_HAS_RECORDS': 'Hiçbir şey değiştirilmedi: CelikPanel bu kümede hâlâ veritabanı kaydı tutuyor. Bu kayıtları Veritabanları sayfasından silin ya da başka bir kümeye taşıyın; sonra kümeyi arşivleyip kaldırın.',
    'err.PG_LEFTOVER_DATA_OUTSIDE_STANDARD': 'Hiçbir şey değiştirilmedi: bu küme verisini PostgreSQL’in standart yerinin dışında tutuyor ve CelikPanel kendi koymadığı veriyi silmez. O dizini saklamaya ya da silmeye siz karar verin.',
    'err.PG_CLUSTERS_UNKNOWN': 'Hiçbir şey değiştirilmedi: PostgreSQL kümeleri okunamadı. CelikPanel Agent’ın çalıştığını denetleyin (sudo systemctl status celikpanel-agent), sonra yeniden deneyin.',
};
