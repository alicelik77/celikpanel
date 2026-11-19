import type { MailboxConnectionKey } from './en';

// Bir posta kutusunun bağlantı bilgileri penceresinin Türkçe metni (alpha.139).

export const trMailboxConnection: Record<MailboxConnectionKey, string> = {
    'mconn.lead': 'Bu posta kutusunu bir posta programına (Outlook, Thunderbird, Apple Mail, telefon) eklemek için gerekenler.',
    'mconn.incoming': 'Gelen posta',
    'mconn.incomingAlt': 'Gelen posta, diğer seçenek',
    'mconn.outgoing': 'Giden posta',
    'mconn.password': 'Parola, bu posta kutusu için belirlenen paroladır; burada hiç gösterilmez. Yenisini belirlemek için kutunun satırındaki “Parola…” düğmesini kullanın.',
    'mconn.passwordReadOnly': 'Parola, bu posta kutusu için belirlenen paroladır; burada hiç gösterilmez. Yenisini yalnız bu alan adının postasını değiştirebilen biri belirleyebilir.',
    'mconn.copyAll': 'Tümünü kopyala',
    'mconn.copied': 'Kopyalandı.',
    'mconn.copiedAll': 'Yukarıdakilerin hepsi parola olmadan kopyalandı; bir mesaja yapıştırabilirsiniz.',
    'mconn.copyFailed': 'Tarayıcı kopyalamaya izin vermedi. Metni seçip elle kopyalayın.',
    'mconn.text.title': '{address} için posta ayarları',
    'mconn.text.password': 'Parola: size ayrıca iletilen parola (bu mesajda yok)',
};
