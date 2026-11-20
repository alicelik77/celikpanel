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
} as const;
