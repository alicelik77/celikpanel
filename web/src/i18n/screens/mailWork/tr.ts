// The Turkish half of the screen catalogue part "mailWork"; ./en.ts defines its keys.
// "mailWork" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { MailWorkScreenKey } from './en';

export const trMailWorkScreens: Record<MailWorkScreenKey, string> = {
    'audit.resource.mail_outgoing': 'Giden posta sınırları',
    'dashboard.audit.action.mail.outgoing.mailbox': 'bir posta kutusunun saatlik gönderim sınırını değiştirdi',
    'dashboard.audit.action.mail.outgoing.settings': 'giden posta sınırlarını değiştirdi',
    'dashboard.audit.action.mail.plan.limits': 'bir planın posta sınırlarını değiştirdi',
    'dashboard.audit.action.mail.account.empty': 'bir posta kutusunu boşalttı',
    'mail.empty.title': '{name} kutusunu boşalt',
    'mail.empty.confirm': '{name} kutusunun bütün klasörlerindeki bütün iletiler kalıcı olarak silinir. Posta kutusu, parolası, klasörleri, takma adları ve ayarları kalır; posta gelmeye devam eder. Geri alınamaz: gereken bir şey varsa önce bir posta programında kopyasını alın.',
    'mail.empty.confirmLabel': 'Bütün iletileri sil',
    'mail.empty.progress': '{name} boşaltılıyor',
    'mail.empty.done': 'Posta kutusu boş.',
    'mail.empty.doneCount': 'Posta kutusu boş: {n} ileti silindi.',
    'mail.aliases.label': 'Şunlara gelenleri de alır:',
    'mail.aliases.add': 'Takma ad ekle',
    'opstep.mailEmpty.check': 'Posta kutusu denetleniyor',
    'opstep.mailEmpty.empty': '{name} iletileri siliniyor (Dovecot)',
    'mail.sendingResumed': 'Güvenlik için durdurulan gönderim yeniden açıldı.',
};
