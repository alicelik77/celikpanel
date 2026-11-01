// SCREEN catalogue part "setupPublishing" (Turkce): DNS yayinlayici ve DKIM
// beklemeleri (setup.v5.wait.server_setup_dns_publisher_, ..._peer_dkim_,
// setup.v5.check.peerDKIM); "setupLink" boyut butcesi icin ayrildi.
import type { SetupPublishingScreenKey } from './en';

export const trSetupPublishingScreens: Record<SetupPublishingScreenKey, string> = {
    "setup.v5.wait.server_setup_dns_publisher_waiting": "Ana DNS sunucusunun bu sunucunun yayımlama bağlantısını kabul etmesi bekleniyor. DNS çifti hazır olunca ve ad sunucuları genel DNS'te görününce kabul eder; kurulum kendiliğinden yeniden sorar.",
    "setup.v5.wait.server_setup_dns_publisher_local_waiting": "Bu yedek DNS sunucusu, ana DNS sunucusunun listesini sunana ve iki ad sunucusu adı da genel DNS'te görünene kadar bekler. Kurulum kendiliğinden yeniden sorar.",
    "setup.v5.wait.server_setup_peer_dkim_zone_waiting": "Ana DNS sunucusu ana alan adının bölgesini henüz hazırlamadı. Kurulum bu sunucunun DKIM kaydını kendiliğinden yeniden gönderir.",
    "setup.v5.wait.server_setup_peer_dkim_publishing": "Ana DNS sunucusu bu sunucunun DKIM kaydını kaydetti ve yayımlıyor. Kurulum kendiliğinden yeniden bakar.",
    "setup.v5.wait.server_setup_peer_dkim_unreachable": "Bu sunucunun DKIM kaydını göndermek için ana DNS sunucusuna ulaşılamıyor. Kurulum kendiliğinden yeniden sorar.",
    "setup.v5.wait.server_setup_peer_dkim_unknown": "Ana DNS sunucusunun yanıtı gelmedi. Kurulum aynı kaydı yeniden gönderir; hiçbir şey iki kez yapılmaz.",
    "setup.v5.wait.server_setup_peer_dkim_removed": "Kurulumun bu sunucu için yazdığı DKIM kaydını ana DNS sunucusunun sahibi sildi ve sahibin silmesi korunur. Siz, sunucu sahibi: kaydı ana DNS sunucusunda Alan adları'ndan kendiniz geri ekleyin ya da silinmiş bırakın; o zaman bu sunucudan giden posta ana alan adı için imzalanmaz.",
    "setup.v5.wait.server_setup_dns_publisher_refused": "Ana DNS sunucusu bu sunucunun yayımlama bağlantısını reddetti: kayıt kodunun süresi doldu, geri alındı ya da zaten kullanıldı. Ana DNS sunucusunda yeni bir katılma kodu oluşturun, bu sunucuyu yeniden katın ve planı yeniden gözden geçirin.",
    "setup.v5.wait.server_setup_peer_dkim_conflict": "Ana DNS sunucusunda bu adda kurulumun yazmadığı farklı bir DKIM kaydı var. Onu orada Alan adları'ndan değiştirin ya da silin, sonra bu sunucunun planını yeniden gözden geçirin.",
    "setup.v5.wait.server_setup_peer_dkim_not_running": "Ana DNS sunucusunun kurulumu çalışmıyor. Birazdan başlayacaksa burada bir şey yapmanız gerekmez; değilse kurulumu ana DNS sunucusunda başlatın ya da sürdürün. Bu sunucu DKIM kaydını kendiliğinden yeniden gönderir.",
    "setup.v5.wait.server_setup_peer_dkim_not_bound": "Ana DNS sunucusunun kurulumu bu sunucuyu henüz almadı; bu sunucunun adları ona ulaşınca alır (katılma adımında bekliyorsa orayı kontrol edin). Kurulum DKIM kaydını kendiliğinden yeniden gönderir.",
    "setup.v5.check.peerDKIM": "Eşleşen sunucunun DKIM kaydı bekleniyor: o sunucunun postası kurulunca buraya gönderilir. Burada yapılacak bir şey yok; kurulum kendiliğinden yeniden bakar.",
    "setup.v5.wait.server_setup_dns_publisher_peer_busy": "Ana DNS sunucusu {server} meşgul, yeniden başlıyor ya da kendi kurulumunu bitiriyor; bu yüzden bu sunucunun yayımlama bağlantısını henüz alamadı. Kimsenin bir şey yapması gerekmiyor: kurulum kendiliğinden yeniden sorar.",
    "setup.v5.wait.server_setup_dns_publisher_peer_license": "Ana DNS sunucusu {server}, panel lisansının etkin olmadığını ya da denetlenemediğini bildirdi; bu yüzden şu an bağlantı almıyor. Sunucunun sahibi {server} üzerinde lisansı etkinleştirir ya da yeniler; kurulum sonra kendiliğinden devam eder.",
    "setup.v5.wait.server_setup_dns_publisher_peer_unexplained": "Ana DNS sunucusu {server} bu sunucunun yayımlama bağlantısını kabul etmedi ve nedenini söylemedi. Bu, sunucu yeniden başlarken ya da kendi kurulumunu bitirirken olur. Şimdilik kimsenin bir şey yapması gerekmiyor: kurulum kendiliğinden yeniden sorar, 30 dakika sonra neyin kontrol edileceğini söyler.",
    "setup.v5.wait.server_setup_dns_publisher_unconfirmed": "Ana DNS sunucusu {server} 30 dakika boyunca bu sunucunun yayımlama bağlantısını nedenini söylemeden reddetti. Siz, sunucu sahibi: {server} panelini açın ve kendi kurulumunun bittiğini kontrol edin. Bittiyse orada yeni bir katılma kodu oluşturun, bu sunucuyu o kodla yeniden katın, sonra bu planı yeniden gözden geçirin.",
};
