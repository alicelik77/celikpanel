// The Turkish half of the screen catalogue part "sideDoors"; ./en.ts defines its keys.
// "sideDoors" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { SideDoorsScreenKey } from './en';

export const trSideDoorsScreens: Record<SideDoorsScreenKey, string> = {
    'pnow.tool_account_failed.title': '{name} hâlâ web sunucusunun hesabıyla çalışıyor',
    'pnow.tool_account_failed.body': 'Bu hesap her sitenin dosyalarını okuyabilir. {name} kendi hesabına taşınamadı; her şey eski hâline geri kondu. Açın ve PHP sürümünü yeniden seçin: CelikPanel taşımayı bir kez daha dener ve neyin durdurduğunu söyler.',
    'pnow.tool_account_failed.action': 'Aç',
    'pnow.tool_account_owner_config.title': '{name} hâlâ web sunucusunun hesabıyla çalışıyor',
    'pnow.tool_account_owner_config.body': '{domain} sunucuda değiştirildiği için CelikPanel dosyayı korur ve {name} uygulamasını kendi hesabına taşımaz. Sunucu yöneticisi bu değişiklikleri /etc/nginx/conf.d içinde başka bir dosyaya taşır ve bunu siler; CelikPanel dosyayı yeniden yazar ve panelin bir sonraki başlangıcından sonra {name} uygulamasını taşır.',
    'pnow.tool_account_owner_config.action': 'Aç',
    'pnow.stock_pool_in_use.title': 'Dağıtımın PHP {name} havuzu hâlâ açık',
    'pnow.stock_pool_in_use.body': 'Her siteyi okuyabilen web sunucusu hesabıyla çalışır ve {domain} PHP’yi hâlâ ona veriyor olabilir; bu yüzden CelikPanel onu korur. Sunucu yöneticisi o yapılandırmaya kendi havuzunu verir ya da siteyi CelikPanel’e taşır. Ardından havuz panelin bir sonraki başlangıcından sonra kapatılır; sunucuda biri tarafından geri açılmış bir havuz ise yönetici Yeniden kapat’ı seçene kadar açık kalır.',
    'pnow.stock_pool_in_use.action': 'PHP-FPM’i aç',
    'pnow.stock_pool_owner_changed.title': 'Dağıtımın PHP {name} havuzu hâlâ açık',
    'pnow.stock_pool_owner_changed.body': '{domain} sunucuda değiştirildiği için CelikPanel onu korur. Her siteyi okuyabilen web sunucusu hesabıyla çalışır. Gerekmiyorsa sunucu yöneticisi dosyanın adını www.conf.off yapar ve PHP-FPM {name} hizmetini yeniden yükler.',
    'pnow.stock_pool_owner_changed.action': 'PHP-FPM’i aç',
    'component.phpTool.cause.own_account_failed': 'PHP {version} üzerinde kendi hesabı verilemedi; olduğu gibi bırakıldı',
    'dashboard.audit.action.php_tool.account.moved': 'bir veritabanı aracını ya da web postasını kendi hesabına taşıdı ({target})',
    'dashboard.audit.action.php_tool.account.failed': 'bir veritabanı aracını ya da web postasını kendi hesabına taşıyamadı; eski hâline kondu ({target})',
    'dashboard.audit.action.php_fpm.stock_pool.turned_off': 'dağıtımın kullanılmayan PHP {target} PHP-FPM havuzunu kapattı',
    'dashboard.audit.action.php_fpm.stock_pool.failed': 'dağıtımın PHP {target} PHP-FPM havuzunu kapatamadı; eski hâline kondu',
};
