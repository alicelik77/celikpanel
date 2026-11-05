// The Turkish half of the screen catalogue part "componentInstall"; ./en.ts defines its keys.
// "componentInstall" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { ComponentInstallScreenKey } from './en';

export const trComponentInstallScreens: Record<ComponentInstallScreenKey, string> = {
    'services.phpToolProof.whyCheck': 'PHP-FPM {version}, CelikPanel’in denetim dosyasını açamadı (HTTP {status}); Yönet penceresindeki Yeniden denetle bunu onarır',
    'services.phpToolProof.whySaid': 'kendisi olarak yanıt vermedi; sunucunun söylediği: {detail}',
    'services.installedWaitingSignatures': '{name} kuruldu ve ilk virüs imzalarını indiriyor; bu genellikle birkaç dakika sürer. Bir şey yapmanız gerekmiyor: indirme bitince tarayıcısı kendiliğinden başlar ve Bileşenler sayfası bir sonraki denetimde onu çalışıyor gösterir.',
    'services.phpToolProblem.versionGone': '{tool} artık kurulu olmayan PHP {version} sürümüne ayarlı; bu yüzden yanıt veremiyor. Bileşenler’de {tool} aracını açıp Yönet’te kurulu bir PHP sürümü seçin.',
    'services.phpToolProof.moved': '{tool} PHP {version} üzerinde çalışıyor: PHP {first} üzerinde çalışmadı ({why}); bu yüzden eklentileri olan sıradaki kurulu PHP sürümü kullanıldı. {tool} aracının Yönet penceresinden değiştirebilirsiniz.',
    'services.phpToolProof.notReady': '{tool} kuruldu, ama PHP {version} üzerinde yanıt vermedi ({why}). Hazır değil görünür: Bileşenler’de Yönet penceresini açın; neden orada yazar ve Yeniden denetle onu taşımadan yeniden sorar.',
    'services.phpToolProof.whyMissing': 'PHP {version} için {extensions} eksik',
    'services.phpToolProof.whyOther': 'kendisi olarak yanıt vermedi',
    'services.installRefused.notable': 'Hiçbir şey kurulmadı: onaylanmamış aşağıdaki paketleri de getirecekti; paket yöneticisinin yanıtı kurulum penceresi gösterildikten sonra değişti. Kur’a yeniden basın: pencere bu paketleri şimdi onaylamanız için listeler.',
    'services.notable.why.new_php_version': 'başka bir PHP sürümünün ({version}) parçası',
    'services.notable.why.apache': 'Apache web sunucusu',
    'services.notable.why.component': 'başka bir bileşen: {component}',
    'services.notable.why.database_server': 'bir veritabanı sunucusu',
    'services.notable.why.upgrade': 'zaten kurulu; kurulum istemediği hâlde yükseltilecek',
    'services.notable.why.vendor_shared': 'dağıtımın sürümü yerine {repo} deposunun sürümü',
    'services.notable.repoUnknown': 'üretici',
    'services.notable.repoName.sury': 'Sury PHP',
    'services.notable.repoName.pgdg': 'PostgreSQL (PGDG)',
    'services.notable.repoName.netdata': 'Netdata',
    'services.phpToolProblem.extensionsFailed': '{tool} aracının ihtiyaç duyduğu PHP eklentileri PHP {version} için kurulamadı; bu yüzden çalışmayabilir. Bileşenler’de {tool} aracını açıp Yönet’te PHP {version} sürümünü yeniden ya da başka bir sürümü seçin.',
    'services.mailProfiles.fallbackWarning': '{name} kuruldu. Hiçbir alan adında etkin ve güvenilir bir Güvenli Posta sertifikası yok; bu yüzden posta istemcilerine sunucunun varsayılan sertifikası sunulur. Posta ana bilgisayar adını kapsayan güvenilir bir sertifika yoksa istemciler güven uyarısı gösterir. Bir alan adına kendi sertifikasını vermek için Güvenli Posta açıkken güvenilir bir sertifika etkinleştirin.',
};
