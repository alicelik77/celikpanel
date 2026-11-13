// The Turkish half of the screen catalogue part "deletionGuide"; ./en.ts defines its keys.
// "deletionGuide" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { DeletionGuideScreenKey } from './en';

export const trDeletionGuideScreens: Record<DeletionGuideScreenKey, string> = {
    'domains.deletionGuide.journalUnknown.why': 'Alan adı bu sunucuda DNS’ten kaldırıldı; yedek DNS sunucusunun da kaldırdığı henüz doğrulanmadı, çünkü bu sunucu son denetimini tamamlayamadı.',
    'domains.deletionGuide.journalUnresolved.why': 'Alan adı bu sunucuda DNS’ten kaldırıldı; ancak yedek DNS sunucusunun da kaldırdığını doğrulamak için bu sunucunun tuttuğu kayıt, CelikPanel’in kendiliğinden düzeltemeyeceği bir durumda.',
    'domains.deletionGuide.proofTimeout.why': 'Alan adı bu sunucuda DNS’ten kaldırıldı; yedek DNS sunucusunun da kaldırdığı henüz doğrulanmadı, çünkü denetim zamanında bitmedi.',
    'domains.deletionGuide.catchingUp.why': 'Alan adı bu sunucuda DNS’ten kaldırıldı; yedek DNS sunucusu buna henüz yetişmedi.',
    'domains.deletionGuide.who.nobody': 'Sizin bir şey yapmanız gerekmiyor.',
    'domains.deletionGuide.who.youAdministrator': 'Siz, bu sunucunun yöneticisi olarak.',
    'domains.deletionGuide.who.administrator': 'Bu sunucunun yöneticisi.',
    'domains.deletionGuide.journalUnresolved.next': 'Ayrıntılar’daki iki komutun çıktısını CelikPanel desteğine gönderin; o dosyaları kendiniz silmeyin.',
    'domains.deletionGuide.journalUnresolved.nextWindow': 'Alan Adları sayfasının üstündeki bekleme kutusunda Ayrıntılar’ı açın ve gösterdiği iki komutun çıktısını CelikPanel desteğine gönderin; o dosyaları kendiniz silmeyin.',
    'domains.deletionGuide.journalUnresolved.nextOther': 'Bu sunucunun yöneticisine (barındırma sağlayıcınıza) haber verin; sizden başka bir şey gerekmiyor.',
    'domains.deletionGuide.stuck.nextOther': 'Bu durum sürerse bu sunucunun yöneticisine (barındırma sağlayıcınıza) haber verin.',
    'domains.deletionGuide.stopped.next': 'Ayrıntılar’daki komutla CelikPanel Agent’ın satırlarını okuyun ve adını verdikleri sorunu giderin.',
    'domains.deletionGuide.stopped.nextWindow': 'Alan Adları sayfasının üstündeki bekleme kutusunda Ayrıntılar’ı açın, gösterdiği komutla CelikPanel Agent’ın satırlarını okuyun ve adını verdikleri sorunu giderin.',
    'domains.deletionGuide.then.at': 'CelikPanel saat {time} civarında kendiliğinden yeniden denetler; doğrulanınca silme kendiliğinden biter.',
    'domains.deletionGuide.then.now': 'CelikPanel şu anda yeniden denetliyor; doğrulanınca silme kendiliğinden biter.',
    'domains.deletionGuide.then.soon': 'CelikPanel birkaç dakika içinde kendiliğinden yeniden denetler; doğrulanınca silme kendiliğinden biter.',
    'domains.deletionGuide.then.stuck': 'CelikPanel 5 dakikada bir denetlemeyi sürdürür; neden ortadan kalkınca silme kendiliğinden biter.',
    'domains.deletionGuide.then.person': 'CelikPanel bunu artık birkaç dakikada bir denetlemiyor. Kayıt düzeltilince bu alan adının satırındaki Diğer → Sil… aynı silmeyi kaldığı yerden sürdürür.',
    'domains.deletionGuide.then.personOther': 'CelikPanel bunu artık birkaç dakikada bir denetlemiyor; bu sunucunun yöneticisi düzeltince silme kaldığı yerden sürer.',
    'domains.deletionGuide.then.stopped': 'CelikPanel bu adımı kendiliğinden denetlemeyi bıraktı. Neden ortadan kalkınca bu alan adının satırındaki Diğer → Sil… aynı silmeyi sürdürür.',
    'domains.deletionGuide.then.stoppedOther': 'CelikPanel bu adımı kendiliğinden denetlemeyi bıraktı; bu sunucunun yöneticisi nedeni giderince silme sürer.',
    'domains.deletionGuide.then.stoppedSupport': 'CelikPanel bu adımı kendiliğinden denetlemeyi bıraktı ve yedek sunucu yetişirken bu sayfa yeniden deneme sunmuyor; neden giderilince CelikPanel desteği silmeyi sürdürmeye yardım eder.',
    'domains.deletionGuide.stepNote.person': 'Bu sunucunun yöneticisini bekliyor.',
    'domains.deletionGuide.details': 'Ayrıntılar',
    'domains.deletionGuide.journalUnknown.details': 'Bu sunucu, yedek DNS sunucusunu doğrulamak için her denetimde bir kez kullanılan bir denetim kaydı tutar. Son denetim bu kaydı tamamlayamadı; sonraki denetim yeni bir kayıtla başlar ve önceki bir denetimin yarım bıraktığı kayıt değişikliğini de kendisi tamamlar. Denetim takılı kalırsa sunucu yöneticisi CelikPanel Agent’ın bununla ilgili satırlarını şu komutla okur:',
    'domains.deletionGuide.journalUnresolved.details': 'Bu sunucu, yedek DNS sunucusunu doğrulamak için bir denetim kaydı tutar (/var/lib/celikpanel-agent-private içinde adında dns-peer geçen dosyalar). CelikPanel, önceki bir denetimin yarım bıraktığı kayıt değişikliğini kendisi tamamlar; burada bulduğu böyle bir değişiklik değil, bu yüzden dosyaları incelenmek üzere olduğu gibi bırakır. Sunucu yöneticisi bu sunucuda şu iki komutu çalıştırır ve çıktılarını gönderir:',
    'domains.deletionGuide.proofTimeout.details': 'Denetim yaklaşık çeyrek saat takılı kalırsa sunucu yöneticisine bir bildirim gider. CelikPanel Agent’ın bununla ilgili satırları şu komutla okunur:',
    'domains.deletionGuide.catchingUp.details': 'Bu durum yaklaşık çeyrek saat sürerse sunucu yöneticisine bir bildirim gider. Yedek DNS sunucusu açık olmalı ve bu sunucuya TCP 53 numaralı bağlantı noktasından ulaşabilmelidir. CelikPanel Agent’ın bununla ilgili satırları şu komutla okunur:',
} as const;
