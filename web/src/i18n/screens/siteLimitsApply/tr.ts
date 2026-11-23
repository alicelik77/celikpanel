// The Turkish half of the screen catalogue part "siteLimitsApply"; ./en.ts defines its keys.
// "siteLimitsApply" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { SiteLimitsApplyScreenKey } from './en';

export const trSiteLimitsApplyScreens: Record<SiteLimitsApplyScreenKey, string> = {
    'slim.applied': 'Şu an geçerli olan: {value}.',
    'slim.move.timeUp': 'Taşımanın süresi doldu. Tamamlandı işaretliler sınırların içinde; kalanlar için taşımaya yeniden basın.',
    'limits.preview.sitesStopped': '{n} sitenin programları şu an düşürülen bir sınırdan fazlasını kullanıyor. Kaydedince bellek sınırını aşan program hemen durdurulur; işlem sınırında yeni işlem başlamaz, çalışanlar sürer:',
    'pnow.site_memory_default.title': 'Sunucunun belleği değişti: sitelerin varsayılan bellek sınırı hâlâ {domain} MB',
    'pnow.site_memory_default.body': 'Sınır o zamanki belleğe göre konmuştu; bellek değişti ({other}). Kendiliğinden hiçbir şey değişmez. Yeni varsayılan {value} MB olur; uygulamadan önce daha fazla kullanan siteler gösterilir.',
    'pnow.site_memory_default.action': 'Yeni varsayılanı uygula',
    'pnow.site_memory_default.confirm': 'Kendi bellek sınırı olmayan siteler bundan sonra {value} MB alır.',
    'pnow.site_memory_default.step': 'Yeni varsayılan uygulanıyor',
    'pnow.site_memory_default.done': 'Yeni varsayılan geçerli. Siteler onu arka planda alır.',
    'pnow.site_memory_default.failed': 'Yeni varsayılan uygulanmadı.',
    'slim.applies.not_yet': 'Bu sınırlar henüz uygulanmıyor; bununla sunucunun yöneticisi ilgilenir, sizden bir şey gerekmez.',
    'slim.applies.pending': 'Yazıldı, henüz geçerli değil (geçerli olan her birinin altında). CelikPanel saatte bir yeniden dener; elle değişiklikte sudo systemctl daemon-reload, sonra iki dakika kadar.',
    'slim.applies.unknown': 'Bu sınırların geçerliliği bilinmiyor: çekirdeğin değerleri okunamadı. Sayfayı yeniden okuyun.',
    'slim.kept': 'Bu sitenin son taşıması bitmedi ({time}); bitene kadar hiçbir site taşınmaz.',
    'slim.kept.unit': '{unit}: {state}. Nedeni: sudo journalctl -u {name}. Sayfasından ya da sudo systemctl start {name} ile başlatıp Tekrar dene’ye basın.',
    'slim.kept.failed': 'durdu, yeniden başlamadı',
    'slim.kept.unknown': 'yeri bilinmiyor, yeniden başlatılmadı',
    'slim.kept.starting': 'hâlâ başlıyor',
};
