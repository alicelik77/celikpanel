// The Turkish half of the screen catalogue part "siteJobs"; ./en.ts defines its keys.
// "siteJobs" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { SiteJobsScreenKey } from './en';

export const trSiteJobsScreens: Record<SiteJobsScreenKey, string> = {
    'pnow.site_crontab_outside.title': 'CelikPanel dışında zamanlanmış görev eklendi',
    'pnow.site_crontab_outside.body': 'Bu sitelerin crontab dosyalarındaki {value} satır CelikPanel’de yapılmadı: {domain}. Bunları sitenin bir programı ya da sunucuda çalışan biri ekledi; sitenin görev sınırları onlar için denetlenmedi. Her sitenin Zamanlanmış görevler bölümünü açın: satırlar orada işaretlidir. Yönetici ya da site sahibi kimsenin istemediklerini kaldırır; sunucu sahibinin bilerek koyduğu bir satırı (@reboot, MAILTO= ya da PATH= gibi) yönetici Kalsın ile tutar ve satır artık listelenmez.',
    'pnow.site_crontab_outside.action': 'Zamanlanmış görevleri aç',
    'pnow.site_at_jobs.title': 'Web sitesi hesaplarının bekleyen at işleri var',
    'pnow.site_at_jobs.body': '{value} at işi web sitesi hesaplarına ait: {domain}. CelikPanel dışında kuruldular; CelikPanel onları göstermez ve sınırlamaz, zamanı gelince yine çalışırlar. CelikPanel bunları silmez. Sunucu yöneticisi her birine sudo at -c <numara> ile bakar ve site sahibinin istemediklerini sudo atrm <numara> ile kaldırır. Bu liste her panel başlangıcında okunur.',
    'pnow.site_at_jobs.action': 'İzleme’yi aç',
    'pnow.at_allow_lists_sites.title': '/etc/at.allow web sitesi hesaplarına at izni veriyor',
    'pnow.at_allow_lists_sites.body': 'Dosya sunucu sahibinindir; CelikPanel onu olduğu gibi korur. Dosyada şu web sitesi hesapları yazılı: {domain}. Bu sitelerin her biri, CelikPanel’in göstermediği ve sınırlamadığı at işleri kurabilir. Sunucu yöneticisi bu adları dosyadan çıkarır.',
    'pnow.at_allow_lists_sites.action': 'İzleme’yi aç',
    'pnow.at_directory_accounts.title': 'Web sitesi hesapları hâlâ at kullanabiliyor',
    'pnow.at_directory_accounts.body': 'Bu sunucu hesaplarını bir dizin hizmetinden alıyor; CelikPanel bugün kimin at kullanabildiğini güvenle listeleyemediği için {domain} dosyasını yazmadı. Bir web sitesi, CelikPanel’in göstermediği ve sınırlamadığı at işleri kurabilir. Sunucu yöneticisi {domain} dosyasını at gereken hesaplarla, her satıra bir ad yazarak ve web sitesi hesaplarını dışarıda bırakarak yazar.',
    'pnow.at_directory_accounts.action': 'İzleme’yi aç',
    'pnow.at_not_closed.title': 'Web sitesi hesapları hâlâ at kullanabiliyor',
    'pnow.at_not_closed.body': 'CelikPanel {domain} dosyasını yazamadı; bu yüzden bir web sitesi, CelikPanel’in göstermediği ve sınırlamadığı at işleri kurabilir. Hiçbir şey değişmedi; CelikPanel panelin bir sonraki başlangıcından sonra yeniden dener. Sürekli başarısız olursa sunucu yöneticisi kesin nedeni İzleme’de okur: /etc yazılabilir, /etc/at.deny, /etc/passwd ve CelikPanel’in bu dosya kaydı okunabilir olmalı.',
    'pnow.at_not_closed.action': 'İzleme’yi aç',
    'cron.outside.badge': 'CelikPanel’de yapılmadı',
    'cron.outside.notice': 'Bu sitenin crontab dosyasındaki bazı satırlar CelikPanel’de yapılmadı: bunları sitenin bir programı ya da sunucuda çalışan biri ekledi. Diğer görevler gibi çalışırlar, ama sitenin görev sınırları onlar için denetlenmedi. Kimsenin istemediği satırı kaldırın. Sunucu sahibinin bilerek koyduğu bir satırı yönetici Kalsın ile tutabilir; satır artık işaretlenmez.',
    'cron.outside.otherTitle': 'CelikPanel’de yapılmamış diğer satırlar',
    'cron.outside.otherHint': 'Bu satırlar CelikPanel’in yazdığı türden görevler değil; örneğin sunucunun her açılışında çalışan bir görev ya da bütün görevler için bir ayar.',
    'cron.outside.deleteConfirm': 'Bu satır crontab’dan kaldırılsın mı?',
    'cron.outside.deleted': 'Satır kaldırıldı',
    'dashboard.audit.action.at.allow.written': 'web sitesi hesapları at kullanamasın diye /etc/at.allow dosyasını yazdı',
    'dashboard.audit.action.at.allow.failed': 'at’i web sitesi hesaplarına kapatamadı; hiçbir şey değişmedi',
};
