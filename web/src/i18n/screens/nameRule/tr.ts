// The Turkish half of the screen catalogue part "nameRule"; ./en.ts defines its keys.
// "nameRule" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { NameRuleScreenKey } from './en';

export const trNameRuleScreens: Record<NameRuleScreenKey, string> = {
    'err.HOSTNAME_NOT_AVAILABLE': '{name} buraya eklenemez: bu ad, üstündeki ya da altındaki bir ad bu sunucuda zaten kullanılıyor ya da sunucuya ayrılmış. Hiçbir şey değiştirilmedi. Ad sizinse sunucu yöneticisinden bakmasını isteyin. Kendi alan adlarınızın alt alan adlarını her zaman ekleyebilirsiniz.',
    'err.HOSTNAME_NOT_AVAILABLE.own_name': '{name} zaten sizin: alan adlarınızdan biri, onun www ya da posta adı veya bir takma adınız. Hiçbir şey değiştirilmedi.',
    'err.HOSTNAME_NOT_AVAILABLE.other_account': '{name}, {holder} hesabının {other} adıyla çakışıyor: bir müşterinin ya da bayinin aboneliği başka bir hesabın adına eşit, onun üstünde ya da altında bir ad tutamaz. Hiçbir şey değiştirilmedi. Adı {other} adını tutan hesaba ya da kendi aboneliğinize ekleyin.',
    'err.HOSTNAME_NOT_AVAILABLE.server_name': '{name}, bu sunucunun kendi adlarından {other} ile çakışıyor: bunu yalnız bir yöneticinin kendi aboneliği tutabilir. Hiçbir şey değiştirilmedi.',
    'err.HOSTNAME_NOT_AVAILABLE.remote_zone': '{name}, bu sunucunun bağlı bir sunucu için yayımladığı {other} bölgesiyle çakışıyor. Hiçbir şey değiştirilmedi. O bölgeyi, onu yayımlayan sunucuda kullanın.',
    'err.HOSTNAME_NOT_AVAILABLE.account_taken': '{name} sitesi {account} sistem hesabıyla çalışırdı; bu hesap başka bir sitede ya da sistemde zaten var. Hiçbir şey değiştirilmedi. Başka bir ad seçin ya da önce o hesabı kullananı kaldırın.',
    'err.APP_PORT_NOT_ALLOWED': 'Bu port sitenin uygulama portu olamaz. Bir Node.js sitesi, {min} ile {max} arasından başka hiçbir sitenin ya da bu sunucunun hiçbir hizmetinin kullanmadığı boş bir port alır. Hiçbir şey değiştirilmedi. Portu boş bırakın; panel boş bir port seçer.',
};
