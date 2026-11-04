// The Turkish half of the screen catalogue part "hostName"; ./en.ts defines its keys.
// "hostName" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { HostNameScreenKey } from './en';

export const trHostNameScreens: Record<HostNameScreenKey, string> = {
    'hostDNS.missing': 'Bu sunucunun ana bilgisayar adı {host} genel DNS’te çözülmüyor: ona yanıt veren bir adres kaydı yok.',
    'hostDNS.other': 'Bu sunucunun ana bilgisayar adı {host}, genel DNS’te bu sunucuyu değil başka bir adresi gösteriyor.',
    'hostDNS.zoneNext': '{zone} bölgesi bu panelde. Yapacak olan: siz. Sonraki adım: bu sunucunun adres kaydını ekleyin; genel DNS birkaç dakika içinde gösterir.',
    'hostDNS.presentNext': 'Bu paneldeki {zone} bölgesinde onun için {records} zaten var, ama genel DNS henüz bununla yanıt vermiyor. Genellikle bölge henüz bu sunucunun ad sunucularına devredilmemiştir ya da değişiklik yoldadır. Yapacak olan: siz, alan adının kayıt firmasında. Sonraki adım: oradaki {zone} ad sunucularını denetleyin, sonra Yeniden denetle’ye basın.',
    'hostDNS.externalNext': '{domain} DNS’i bu panelde değil. Yapacak olan: o DNS’i yöneten kişi. Sonraki adım: orada {host} → {ip} adres kaydını ekleyin, sonra Yeniden denetle’ye basın.',
    'hostDNS.otherNext': 'Genel DNS {addresses} yanıtını veriyor; bu sunucu {ip}. Yapacak olan: o DNS’i yöneten kişi. Sonraki adım: bu istenmiyorsa adres kaydını değiştirin, sonra Yeniden denetle’ye basın.',
    'hostDNS.why': 'Bu sunucudan giden posta ve bu adın sertifikası, adın çözülmesine ihtiyaç duyar. Sunucuda hiçbir şey değiştirilmedi.',
    'hostDNS.add': 'Kaydı ekle…',
    'hostDNS.recheck': 'Yeniden denetle',
    'hostDNS.openZone': '{zone} bölgesini aç',
    'hostDNS.dialogTitle': 'Adres kaydını {zone} bölgesine ekle',
    'hostDNS.dialogBody': '{host} bu sunucuya çözülsün diye şu kayıtlar bu paneldeki {zone} bölgesine eklenip yayımlanacak:',
    'hostDNS.dialogWait': 'Genel çözümleyiciler bundan sonra birkaç dakika eski “yok” yanıtını tutabilir.',
    'hostDNS.confirm': 'Ekle ve yayımla',
    'hostDNS.added': 'Kayıt {zone} bölgesine eklendi ve yayımlandı. Genel DNS {host} adını birkaç dakika içinde gösterir; o zaman Yeniden denetle’ye basın.',
    'hostDNS.addFailed': '{record} kaydı {zone} bölgesine eklenemedi: {reason}',
    'hostDNS.addLost': '{record} için yanıt kayboldu; eklenmiş de olabilir, eklenmemiş de. Hiçbir şey kendiliğinden yeniden gönderilmez: Yeniden denetle’ye basın ya da bölgeyi açıp bakın.',
};
