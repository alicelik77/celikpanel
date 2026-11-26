// SCREEN catalogue part "siteTasksProblems" (alpha.150, split out of
// "siteTasks" and "problemsNow" when their words filled them): the Problems
// now lines of sites' scheduled tasks and their side doors - cron.allow,
// crontabs that came back, held tasks, lingering, a site account's own
// service manager, a move into a site's limits that did not finish (pnow.) -
// and "Move all", which their line opens (moveall.).
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "siteTasksProblems": sitelerin zamanlanmış
// görevlerinin "Şu anki sorunlar" satırları ve "Hepsini taşı".

import type { SiteTasksProblemsScreenKey } from './en';

export const trSiteTasksProblemsScreens: Record<SiteTasksProblemsScreenKey, string> = {
    'pnow.site_move_kept.title': 'Sınırlara taşıma bitmedi: {domain}',
    'pnow.site_move_kept.body': 'Bir program durmuş ya da sitenin zamanlanmış görevleri çalışmıyor olabilir; çözülene dek hiçbir site taşınmaz. Yapılacak iş sitenin kartında.',
    'pnow.site_move_kept.action': 'Siteyi aç',
    'pnow.site_tasks_held.title': 'Zamanlanmış görevleri bekletilen siteler: {value}',
    'pnow.site_tasks_held.body': 'Bu sitelerin zamanlanmış görevleri sınır dosyaları yokken çalışmaz ({other}); sunucu yeniden başlasa da çalışmaz: {domain}. Yönetici aboneliğin sınırlarını kaydeder; dosya geri gelince görevler bir sonraki saatlerinde yeniden çalışır. Elle geri konan dosyadan sonra sudo systemctl daemon-reload da gerekir.',
    'pnow.site_tasks_held.action': 'İzleme’yi aç',
    'pnow.site_lingering.title': 'Site hesapları kendi programlarını çalışır tutuyor',
    'pnow.site_lingering.body': 'Bu hesaplar kalıcı oturumda; başlattıkları programlar sınırlarının dışında çalışmayı sürdürür: {domain}. Sunucu sahibi bunu şu komutla bitirir: {other}',
    'pnow.site_lingering.action': 'İzleme’yi aç',
    'pnow.site_user_manager_open.title': 'Site hesapları kendi hizmet yöneticilerini başlatabiliyor',
    'pnow.site_user_manager_open.body': 'Bu hesaplar SFTP ile girince kendi hizmet yöneticileri başlayabiliyor ve sınırlarının dışında program çalıştırabiliyor: {domain}. Sunucu sahibi bunu şu komutla kapatır: sudo systemctl mask {other}',
    'pnow.site_user_manager_open.action': 'İzleme’yi aç',
    'moveall.unread': 'Sınırlarının dışındaki sitelerin listesi okunamadı, bu yüzden hiçbir şey taşınmaz. Bunu kapatıp yeniden basın.',
    'moveall.button': 'Hepsini taşı',
    'moveall.title': 'Her site sınırlarının içine taşınıyor',
    'moveall.confirm': '{n} site sırayla sınırlarının içine taşınsın mı?',
    'moveall.body': 'Her site, kendi “Sınırların içine taşı” işlemi gibi taşınır: önce görevleri, sonra çalışan programları sınırların içinde bir kez yeniden başlar. Taşınamayan bir site atlanır ve nedeni söylenir; iş yalnız başlamış bir taşıma başarısız olursa durur.',
    'moveall.go': 'Hepsini taşı',
    'moveall.cancel': 'Vazgeç',
    'moveall.none': 'Şu an sınırlarının dışında site yok.',
    'moveall.done': 'Bitti: {n} site taşındı.',
    'moveall.failed': '{domain} taşınamadı: {reason}. Ondan sonraki sitelere dokunulmadı.',
    'moveall.unknown': 'Sunucu {domain} için yanıt vermedi, sonucu bilinmiyor. Devam etmeden önce bu sayfayı yeniden okuyun.',
    'moveall.programs': '{n} program sınırların içinde yeniden başlar',
    'moveall.tasks': '{n} görev zamanlayıcıya dönüşür',
    'moveall.skip': '{domain}: atlanır, {reason}',
    'moveall.tasksStay': '{domain}: atlanır; görevleri cron’da kalır ({reason}), {n} programı taşınabilir',
    'moveall.option': 'Görevleri cron’da kalan sitelerin programları da taşınsın ({n})',
    'moveall.optionRow': '{domain}: programları taşınır; görevleri cron’da kalır',
    'moveall.previewUnknown': 'taşımanın ne yapacağı okunamadı',
    'moveall.notMoved': 'Taşınmayanlar',
    'moveall.unresolved': 'Daha önceki bir taşıma tamamen geri konamadı; çözülene kadar hiçbir site taşınmaz. O sitenin kartında Tekrar dene var.',
    'moveall.partial': 'programları taşındı; görevleri cron’da kaldı ({reason})',
    'pnow.site_limits_outside.title': 'Sınırlarının dışındaki siteler: {value}',
    'pnow.site_limits_outside.body': 'Bu sitelerin çalışan programları ya da zamanlanmış görevleri hâlâ sınırlarının dışında çalışıyor: {domain}. Taşıma, her sitenin programlarını sınırların içinde bir kez yeniden başlatır ve görevlerini orada zamanlayıcıya çevirir; hiçbir şey yarım taşınmaz. Bu satır kendiliğinden kaybolur.',
    'pnow.site_limits_outside.action': 'Hepsini taşı',
    'pnow.cron_allow_lists_sites.title': 'Sınırları içindeki sitelere cron yeniden açık',
    'pnow.cron_allow_lists_sites.body': 'Sunucu sahibinin {other} dosyası şu hesaplara cron izni veriyor: {domain}. Ekledikleri işler sınırlarının dışında çalışır. Sunucu sahibi bu adları dosyadan çıkarır.',
    'pnow.cron_allow_lists_sites.action': 'İzleme’yi aç',
    'pnow.cron_not_closed.title': 'Cron, sınırları içindeki sitelere kapatılmadı',
    'pnow.cron_not_closed.body': 'CelikPanel {domain} dosyasını yazmadı; bu sitelerin kendi kodu sınırlarının dışında çalışan cron işleri ekleyebiliyor ({other}). Sunucu sahibi dosyaya bakar; CelikPanel bir sonraki açılışta yeniden dener.',
    'pnow.cron_not_closed.action': 'İzleme’yi aç',
    'pnow.cron_owner_removed.title': 'Bu sunucuda /etc/cron.allow silindi',
    'pnow.cron_owner_removed.body': 'Görevleri sınırları içinde çalışan siteler yeniden, sınırların dışında cron kullanabiliyor. CelikPanel dosyayı kendiliğinden yeniden yazmaz.',
    'pnow.cron_owner_removed.confirm': 'CelikPanel /etc/cron.allow dosyasını yeniden yazar: şu an cron kullanabilen her hesap bu izni korur, yalnız bu sitelerin hesapları korumaz.',
    'pnow.cron_owner_removed.action': 'Yeniden kapat',
    'pnow.site_crontab_returned.title': 'Sınırları içindeki siteler için yeniden crontab yapılmış',
    'pnow.site_crontab_returned.body': 'Bu hesapların root tarafından yapılmış yeni bir crontab’ı var (toplam {value} satır): {domain}. Satırları sitelerin sınırlarının dışında çalışır. Sunucu sahibi satırları sitenin Zamanlanmış görevler ekranına taşır ya da dosyayı kaldırır (bu hesaplar için crontab -r reddedilir; dosya /var/spool/cron/crontabs içinde, cronie’de /var/spool/cron içindedir).',
    'pnow.site_crontab_returned.action': 'İzleme’yi aç',
    'pnow.site_crontab_by_site.title': 'Sınırları içindeki sitelerin yeniden crontab’ı var',
    'pnow.site_crontab_by_site.body': 'Bu hesapların yeniden bir crontab’ı var (toplam {value} satır): {domain}. Cron onlara izin verdi, yani sitelerin kendi kodu yapmış olabilir ve satırları sınırların dışında çalışıyor. Sunucu sahibi hesapları cron’un dışında tutar, sonra dosyayı kaldırır (/var/spool/cron/crontabs içinde, cronie’de /var/spool/cron içinde).',
    'pnow.site_crontab_by_site.action': 'İzleme’yi aç',
    'pnow.linger_owner_removed.title': 'Kalıcı oturum kuralı bu sunucuda silindi',
    'pnow.linger_owner_removed.body': 'Site hesapları kalıcı oturumu kendileri yeniden açabiliyor ve programlarını sınırlarının dışında çalışır tutabiliyor. CelikPanel kuralı kendiliğinden yeniden yazmaz.',
    'pnow.linger_owner_removed.confirm': 'CelikPanel polkit kuralını yeniden yazar: kalıcı oturumu kendisi için yalnız yöneticiler ve root açabilir.',
    'pnow.linger_owner_removed.action': 'Yeniden kapat',
    'pnow.linger_not_closed.title': 'Kalıcı oturum site hesaplarına kapatılmadı',
    'pnow.linger_not_closed.body': 'CelikPanel’in polkit kuralı yerinde değil ({domain}): sunucuda değiştirildi ya da yazılamadı ({other}). Site hesapları kendi programlarını sınırlarının dışında çalışır tutabilir. Sunucu sahibi dosyaya bakar.',
    'pnow.linger_not_closed.action': 'İzleme’yi aç',
};
