// The Turkish half of the screen catalogue part "monitoring"; ./en.ts defines its keys.
// "monitoring" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { MonitoringScreenKey } from './en';

export const trMonitoringScreens: Record<MonitoringScreenKey, string> = {
    'monitoring.subtitle': 'İşlemci, bellek, disk ve yük. Her nokta 30 saniyenin ortalamasıdır; geçmiş 30 gün saklanır.',
    'monitoring.cpu': 'İşlemci',
    'monitoring.vcpu': '{n} sanal çekirdek',
    'monitoring.memory': 'Bellek',
    'monitoring.disk': 'Disk (/)',
    'monitoring.load': 'Yük (1dk)',
    'monitoring.range.1h': '1 saat',
    'monitoring.range.6h': '6 saat',
    'monitoring.range.24h': '24 saat',
    'monitoring.range.7d': '7 gün',
    'monitoring.range.30d': '30 gün',
    'monitoring.pct': '%{n}',
    'monitoring.highest': 'en yüksek {value}',
    'monitoring.shortPeak': 'kısa tepe {value}',
    'monitoring.cpuHint': 'Çizgi, {n} sanal çekirdeğin hepsinin ortalamasıdır. Kısa tepe, en yüksek tek saniyedir; kısa tepeler olağandır ve tek başına sorun değildir.',
    'monitoring.empty': 'Henüz örnek yok. İlk nokta, panel başladıktan yaklaşık bir dakika sonra görünür.',
    'monitoring.checking': 'Kaydedilen ölçümler okunuyor…',
    'monitoring.unknown': 'Kaydedilen ölçümler sunucudan okunamadı; bu yüzden grafik gösterilmiyor. Bu, hiçbir şey kaydedilmediği anlamına gelmez. Hiçbir şey değiştirilmedi. Tekrar deneyin.',

    'dashboard.audit.action.monitoring.alerts.settings': 'izleme uyarı ayarlarını değiştirdi',
    // alpha.128 ops (G18): alıcıya göre bildirimler.
    'dashboard.audit.action.notifications.preferences': 'hangi bildirimleri e-postayla alacağını değiştirdi',
    'dashboard.audit.action.notifications.settings': 'sunucu geneli bildirim ayarlarını değiştirdi',
    // alpha.144 (alpha.143 lab): a failed PostgreSQL close names the kind of cause (pgpub.; in this part for its size)
    'err.POSTGRESQL_PUBLIC_FAILED.refused': 'Veritabanı sunucusu bunu reddetti. Hiçbir şey değiştirilmedi.',
    'err.POSTGRESQL_PUBLIC_FAILED.in_use': 'Veritabanı kullanımda: veritabanı sunucusu gereken kilidi zamanında vermedi. Hiçbir şey değiştirilmedi. Onu tutan bağlantılar bitince yeniden deneyin.',
    'err.POSTGRESQL_PUBLIC_FAILED.timeout': 'Veritabanı sunucusu çok uzun sürdü ve işi durdurdu. Hiçbir şey değiştirilmedi. Yeniden deneyin.',
    'err.POSTGRESQL_PUBLIC_FAILED.connection': 'Veritabanı sunucusuyla bağlantı koptu. Hiçbir şey değiştirilmedi. PostgreSQL’in çalıştığını kontrol edip yeniden deneyin.',
    'err.POSTGRESQL_PUBLIC_FAILED.rights': 'Veritabanı sunucusu buna izin vermedi: bir yetki eksikti. Hiçbir şey değiştirilmedi.',
    'err.POSTGRESQL_PUBLIC_FAILED.missing': 'Adı geçen bir şey veritabanı sunucusunda yok (bir kullanıcı ya da rol). Hiçbir şey değiştirilmedi.',
    // alpha.145 (alpha.144 laboratuvarı O2): pencere başlığı sonuca uyar.
    'pgpub.end.closed': '{name} başka hesaplara kapatıldı',
    'pgpub.end.notClosed': '{name} başka hesaplara kapatılamadı',
    'pgpub.end.unknown': '{name} başka hesaplara kapatıldı mı bilinmiyor',
    'pgpub.undo.end.opened': '{name} yeniden her hesaba açık',
    'pgpub.undo.end.notOpened': '{name} yeniden açılamadı',
    'pgpub.undo.end.unknown': '{name} yeniden açıldı mı bilinmiyor',
    'pgpub.step.notDone': 'Yapılmadı — hiçbir şey değiştirilmedi',
    'pgpub.step.unknown': 'Sonuç henüz doğrulanmadı — yeniden gönderilmez',
    'pgpub.why.server.in_use': 'veritabanı kullanımdaydı ve veritabanı sunucusu kilidi zamanında vermedi; hiçbir şey değişmedi: “{detail}”',
    'pgpub.why.server.timeout': 'veritabanı sunucusu çok uzun sürdü ve işi durdurdu; hiçbir şey değişmedi: “{detail}”',
    'pgpub.why.server.connection': 'veritabanı sunucusuyla bağlantı koptu; hiçbir şey değişmedi: “{detail}”',
    'pgpub.why.server.rights': 'veritabanı sunucusu izin vermedi (bir yetki eksikti); hiçbir şey değişmedi: “{detail}”',
    'pgpub.why.server.missing': 'adı geçen bir şey veritabanı sunucusunda yok; hiçbir şey değişmedi: “{detail}”',
} as const;
