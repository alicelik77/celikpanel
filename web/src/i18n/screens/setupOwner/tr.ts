// The Turkish half of the screen catalogue part "setupOwner"; ./en.ts defines its keys.
// "setupOwner" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { SetupOwnerScreenKey } from './en';

export const trSetupOwnerScreens: Record<SetupOwnerScreenKey, string> = {
    // ---- ana alan adının web sitesi (yalnız ilk sunucu) ----
    'setup.v7.site.title': '{domain} web sitesi bu sunucuda mı olacak?',
    'setup.v7.site.here': 'Evet, burada barındır',
    'setup.v7.site.here.tag': 'Önerilen',
    'setup.v7.site.here.help': 'Kurulum sonunda {domain} ve www.{domain} bu sunucudan açılır; ücretsiz Let’s Encrypt sertifikasıyla korunur.',
    'setup.v7.site.elsewhere': 'Hayır, web sitesi başka yerde',
    'setup.v7.site.elsewhere.help': '{domain} burada yalnız DNS kayıtlarını tutar; web sitesi başka yerdedir. Web sitesini sonra “Web sitesi barındır” ile ekleyebilirsiniz.',
    'setup.v7.review.siteHere': '{domain} ve www.{domain}: kurulum son adımında web sitesini bu sunucuda ücretsiz sertifikayla oluşturur. Alan adı ad sunucularınızı gösterdiğinde açılır.',
    'setup.v7.review.siteElsewhere': '{domain} ve www.{domain} bu sunucudan açılmaz: burada yalnız DNS kayıtları tutulur ve onlar için adres içe aktarmadınız. Web sitesi başka yerde çalışıyorsa adresini Mevcut kayıtlar adımında içe aktarın; yoksa ad sunucusu değişikliği yayılınca site açılmaz.',
    'setup.kind.main_site': '{target} web sitesini ve sertifikasını oluştur',
    'setup.kind.infrastructure_dns': 'Sunucu adreslerinin DNS kayıtlarını hazırla: {target}',
    'setup.kind.access_dns': 'Sertifikadan önce genel DNS’i doğrula: {target}',

    // ---- hazır, kalan işlerle ----
    'setup.v7.ready.tasksOne': '{server} hazır; size 1 iş kaldı',
    'setup.v7.ready.tasksMany': '{server} hazır; size {count} iş kaldı',
    'setup.v7.ready.tasksHelp': 'Kurulum bitti ve hizmetler çalışıyor. Bunları siz yaparsınız; her biri ne yapılacağını ve nerede yapılacağını söyler.',
    'setup.v7.site.done': '{domain} ve www.{domain} bu sunucuda sertifikayla korunan bir web sitesi.',
    'setup.v7.site.certPending': '{domain} web sitesi oluşturuldu, ama sertifikası henüz alınamadı: {reason} {domain} bu sunucudan açıldığında sertifikayı alan adının SSL/TLS sayfasından isteyin.',
    'setup.v7.site.certWaitingOwner': '{domain} sertifikası alındı, ama sitenin yapılandırma dosyası sizindir ve onu henüz kullanmıyor. SSL/TLS sayfası neyin değişeceğini söyler.',
    'setup.v7.site.failed': '{domain} web sitesi oluşturulamadı: {reason} Sorun giderilince “Web sitesi barındır” ile ekleyin.',
    'setup.v7.site.dnsOnly': '{domain} burada web sitesi olmadan zaten var; kurulum ona dokunmadı. Barındırma eklemek için alan adını açın.',
    'setup.v7.site.reasonUnknown': 'neden bildirilmedi.',
    'setup.v7.task.registrar': '{domain} adını, kaydettiğiniz firmada ad sunucularınıza yönlendirin: {servers}. O zamana dek {domain} ve adları bu sunuculardan açılmaz.',
    'setup.v7.action.ssl': 'SSL/TLS’i aç',
    'setup.v7.action.domain': 'Alan adını aç',
    'setup.v7.action.dns': 'DNS sayfasını aç',

    // ---- sihirbaz bir amaçla açıldığında ----
    'setup.v7.purpose.mail': 'Buraya bu sunucuya posta eklemek için geldiniz; yalnız posta adımları değişir.',
    'setup.v7.purpose.dkim': 'Buraya postanızın imza anahtarını (DKIM) oluşturmak için geldiniz; yalnız posta adımları değişir.',
    'setup.v7.purpose.back': 'Geldiğiniz yere dönün',

    // ---- zaten DNS yazılımı çalıştıran sunucu ----
    'setup.v7.engine.fixed': 'DNS yazılımı: {engine}, zaten çalışıyor; burada değiştirilmez.',

    // ---- Bileşenler: postayı yerinde ekleme ----
    'svc.mailSetup.title': '{name} kur',
    'svc.mailSetup.titleDKIM': 'Posta imza anahtarını (DKIM) oluştur',
    'svc.mailSetup.intro': 'Posta, bu sunucunun tamamlanmış kurulumuna bu pencerede eklenir. Kurulumdaki başka hiçbir şey sorulmaz ya da değişmez.',
    'svc.mailSetup.hostname': 'Posta sunucusu adı',
    'svc.mailSetup.hostnameHelp': 'Posta programlarının bağlandığı ad, örneğin {proposal}. Kurulum onu bu sunucuya yönlendirir.',
    'svc.mailSetup.hostnameJoined': '{first} ilk sunucunun posta adıdır ve {domain} için kalır. Bu sunucunun postası kendi adını kullanır, örneğin {proposal}.',
    'svc.mailSetup.hostnameTaken': '{name} ilk sunucu ya da ad sunucularınız tarafından kullanılıyor. Başka bir ad kullanın, örneğin {proposal}.',
    'svc.mailSetup.hostnameInvalid': 'Tam bir ad yazın, örneğin {proposal}.',
    'svc.mailSetup.hostnameFixed': 'Posta sunucusu adı: {name}',
    'svc.mailSetup.review': 'Adımları göster',
    'svc.mailSetup.reading': 'Kurulumun ne yapacağı okunuyor. Henüz hiçbir şey değişmedi.',
    'svc.mailSetup.stepsTitle': 'Sırasıyla ne olacak',
    'svc.mailSetup.blocked': 'Bu adımlar henüz başlayamaz:',
    'svc.mailSetup.confirm': 'Adımları okudum. Şimdi başlat.',
    'svc.mailSetup.start': 'Başlat',
    'svc.mailSetup.running': 'Posta kuruluyor. Bu pencereyi kapatabilirsiniz; iş sürer ve bu sayfa onu yeniden gösterir.',
    'svc.mailSetup.done': 'Posta kuruldu ve {host} ile korunuyor.',
    'svc.mailSetup.doneDKIM': 'Posta adımları bitti. DNS sayfası imza anahtarını ve kaydını gösterir.',
    'svc.mailSetup.failed': 'Posta kurulumu durdu.',
    'svc.mailSetup.failedNext': 'Başka bir şey başlatılmaz. Nedeni görmek ve planı değiştirmek için sunucu kurulumunu açın ya da durdurup sunucuyu bu pencereden önceki hâliyle bırakın.',
    'svc.mailSetup.stop': 'Durdur ve sunucuyu olduğu gibi bırak',
    'svc.mailSetup.openSetup': 'Sunucu kurulumunu aç',
    'svc.mailSetup.readFailed': 'Adımlar okunamadı; hiçbir şey değişmedi. Yeniden deneyin.',
    'svc.mailSetup.busy': 'Bu sunucuda başka bir kurulum çalışması sürüyor. Bitmesini bekleyip yeniden deneyin.',
    'svc.mailSetup.unknown': 'Sunucunun yanıtı gelmedi. Bu pencere durumu yeniden okur; hiçbir şey iki kez başlatılmaz.',
    'svc.mailSetup.close': 'Kapat',
};
