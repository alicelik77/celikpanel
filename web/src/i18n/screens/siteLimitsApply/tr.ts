// The Turkish half of the screen catalogue part "siteLimitsApply"; ./en.ts defines its keys.
// "siteLimitsApply" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { SiteLimitsApplyScreenKey } from './en';

export const trSiteLimitsApplyScreens: Record<SiteLimitsApplyScreenKey, string> = {
    'slim.applied': 'Şu an geçerli olan: {value}.',
    'slim.move.timeUp': 'Taşımanın süresi doldu. Tamamlandı işaretli programlar sınırların içinde; kalanlarla sürdürmek için taşımaya yeniden basın.',
    'limits.preview.sitesStopped': '{n} sitenin programları şu an düşürülen bir sınırdan fazlasını kullanıyor. Kaydedince bellek sınırını aşan program hemen durdurulur; işlem sınırında yeni işlem başlamaz, çalışanlar sürer:',
    'pnow.site_limits_unapplied.title': 'Yeni site sınırları henüz {value} siteye ulaşmadı',
    'pnow.site_limits_unapplied.body': 'Bir sınır değişikliğinden ({domain}) sonra {value} sitede hâlâ önceki sınırlar geçerli ({other}). CelikPanel her saat yeniden dener; her sitenin kartı geçerli sınırları gösterir. Her site yenilerini alınca bu satır kaybolur.',
    'pnow.site_limits_unapplied.action': 'Planları aç',
    'pnow.site_memory_default.title': 'Sunucunun belleği değişti: sitelerin varsayılan bellek sınırı hâlâ {domain} MB',
    'pnow.site_memory_default.body': 'Bu sınır o zamanki sunucu belleğine göre konmuştu; bellek değişti ({other}). Kendiliğinden hiçbir şey değişmez. Yeni varsayılan {value} MB olur; uygulamadan önce şu an daha fazla kullanan siteler gösterilir.',
    'pnow.site_memory_default.action': 'Yeni varsayılanı uygula',
    'pnow.site_memory_default.confirm': 'Kendi bellek sınırı olmayan siteler bundan sonra {value} MB alır.',
    'pnow.site_memory_default.step': 'Yeni varsayılan uygulanıyor',
    'pnow.site_memory_default.done': 'Yeni varsayılan geçerli. Siteler onu arka planda alır.',
    'pnow.site_memory_default.failed': 'Yeni varsayılan uygulanmadı.',
};
