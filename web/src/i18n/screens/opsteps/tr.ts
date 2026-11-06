// The Turkish half of the screen catalogue part "opsteps"; ./en.ts defines its keys.
// "opsteps" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { OpstepsScreenKey } from './en';

export const trOpstepsScreens: Record<OpstepsScreenKey, string> = {
    'opstep.stoppedHere': 'Bu adımda durdu. Sonraki adımlar çalışmadı.',
    'opstep.stoppedBeforeChange': 'Bu denetimde, hiçbir şey değiştirilmeden durdu.',
    'opstep.stepUnknown': 'Bu adımın bitip bitmediği bilinmiyor.',
    'opstep.unread': 'Sunucu bir dakikadır bu değişiklik hakkında yanıt vermedi; nasıl bittiği bilinmiyor. Hiçbir şey yeniden gönderilmedi. Yeniden denemeden önce sayfayı yenileyip şimdiki durumu kontrol edin.',
    'opstep.gone': 'Sunucu bu değişikliğin kaydını artık tutmuyor; nasıl bittiği bilinmiyor. Hiçbir şey yeniden gönderilmedi. Bu sayfada şimdiki durumu kontrol edin.',
    'opstep.interrupted': 'Bu değişiklik sürerken CelikPanel yeniden başladı; bitip bitmediği bilinmiyor. Kendiliğinden yeniden çalışmaz. Bu sayfada şimdiki durumu kontrol edin; değişikliği yalnız eksikse yeniden yapın.',
    'opstep.refusedNotKept': 'Değişiklik, metni saklanmayan bir hatayla bitti. Hiçbir şey yeniden gönderilmedi. Bu sayfada şimdiki durumu kontrol edip yeniden deneyin.',

    'opstep.ssl.title': '{name} için sertifika alınıyor',
    'opstep.ssl.check': 'İstek denetleniyor',
    'opstep.ssl.dns_check': '{name} adının bu sunucuyu gösterdiği denetleniyor',
    'opstep.ssl.prepare_validation': 'Web sunucusu yetkili kuruluşun denetimine hazırlanıyor',
    'opstep.ssl.acme': '{authority} kuruluşuna başvuruluyor ve alan adının denetimi kanıtlanıyor',
    'opstep.ssl.acmeNote': 'Bu genellikle bir dakikadan kısa sürer: yetkili kuruluş denetlemek için siteyi ziyaret eder.',
    'opstep.ssl.verify': 'Yeni sertifika denetleniyor',
    'opstep.ssl.install': 'Sertifika kuruluyor',
    'opstep.ssl.reload': 'Web sunucusu yeniden yükleniyor',
    'opstep.ssl.mail': 'Posta şifrelemesi yeni sertifikaya geçiriliyor',

    'opstep.restore.title': '{name} geri yükleniyor',
    'opstep.restore.check': 'Disk alanı sınırı denetleniyor',
    'opstep.restore.inspect': 'Yedek okunuyor ve güvenle geri yüklenebildiği denetleniyor',
    'opstep.restore.restore': 'Dosyalar ve veritabanları geri konuyor',
    'opstep.restore.restoreNote': 'Büyük bir sitede birkaç dakika sürebilir. Sizin bir şey yapmanız gerekmiyor.',
    'opstep.restore.finish': 'Bitiriliyor',

    'opstep.backup.title': '{name} yedekleniyor',
    'opstep.backup.check': 'Disk alanı sınırı denetleniyor',
    'opstep.backup.create': 'Yedek oluşturuluyor',
    'opstep.backup.createNote': 'Büyük bir sitede birkaç dakika sürebilir. Sizin bir şey yapmanız gerekmiyor.',
    'opstep.backup.finish': 'Bitiriliyor',

    'opstep.php.title': '{name}, PHP {version} sürümüne geçiriliyor',
    'opstep.php.check': 'PHP {version} sürümünün kullanılabildiği denetleniyor',
    'opstep.php.pool': 'Site PHP {version} sürümüne taşınıyor',
    'opstep.php.save': 'Değişiklik kaydediliyor',
    'opstep.php.web_server': 'Web sunucusu PHP {version} sürümüne yönlendirilip yeniden yükleniyor',

    'opstep.app.title': '{name} kuruluyor',
    'opstep.app.check': '{name} için gerekenler denetleniyor',
    'opstep.app.database': 'Veritabanı oluşturuluyor',
    'opstep.app.files': '{name} indiriliyor, denetleniyor ve dosyaları yerleştiriliyor',
    'opstep.app.setup': '{name} kendini kuruyor',
    'opstep.app.finish': 'Bitiriliyor',
    'opstep.app.reloadedNote': 'Sayfa açıldığında bu kurulum zaten sürüyordu; burada izleniyor.',

    'opstep.wp.start': 'İstek gönderiliyor',
    'opstep.wp.queued': 'Bu sunucudaki başka bir WordPress işinin bitmesi bekleniyor',
    'opstep.wp.backing_up': 'Önce sitenin yedeği alınıyor',
    'opstep.wp.updating': 'WordPress güncelleniyor',
    'opstep.wp.cloning': 'Site {target} adresine kopyalanıyor',
    'opstep.wp.restoring': 'Güncelleme denetimden geçmedi: yedek geri konuyor',
    'opstep.wp.finish': 'Sonuç denetleniyor',
};
