// The Turkish half of the screen catalogue part "dbEncoding"; ./en.ts defines its keys.
// "dbEncoding" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { DbEncodingScreenKey } from './en';

export const trDbEncodingScreens: Record<DbEncodingScreenKey, string> = {
    'dbenc.fact': 'Metin: {encoding}',
    'dbenc.title.unchecked': 'Bu veritabanında metin denetlenmeden saklanıyor ({encoding})',
    'dbenc.title.limited': 'Bu veritabanı her dilin harflerini tutamıyor ({encoding})',
    'dbenc.why.unchecked': 'İngilizce dışındaki harfler (ç, ğ, ı, ş ve başka her dilin harfleri) denetlenmeden ham bayt olarak saklanıyor; bu yüzden sıralama, arama ve başka bir sunucuya taşıma yanlış sonuç verebilir.',
    'dbenc.why.limited': '{encoding} kodlamasıyla oluşturulmuş; bu kodlamada Türkçedeki ğ, ı ve ş gibi birçok harfe yer yok.',
    'dbenc.who.manager': 'Bu alan adının veritabanlarını yöneten kişi',
    'dbenc.next': 'Henüz boşsa silin ve yeniden oluşturun. Veri varsa bu karttaki Kopyala düğmesine basın: kopya, bütün verisiyle UTF-8 olarak oluşturulur. Sitenizin veritabanı ayarlarını kopyaya çevirin, siteyi deneyin, sonra bu veritabanını silin.',
    'dbenc.then': 'Kopyalama her harfi denetler. UTF-8 olmayan bir metinde durursa hiçbir şey değişmez, bu veritabanı olduğu gibi kalır; o metnin önce UTF-8’e çevrilmesi gerekir. Yeni veritabanları her zaman UTF-8 oluşturulur.',
};
