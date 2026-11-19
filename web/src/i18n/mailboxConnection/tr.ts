import type { MailboxConnectionKey } from './en';

// Bir posta kutusunun bağlantı bilgileri penceresinin Türkçe metni (alpha.138).

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
    'mconn.smtpAuth': 'Giden posta, aynı kullanıcı adı ve parolayla oturum açmayı ister.',
    'mconn.reach.notHere.title': 'Bu ayarlar bu posta kutusuna henüz ulaşmaz',
    'mconn.reach.notHere.why': 'Bu sunucuda posta (Postfix ve Dovecot) kurulu değil; burada posta programlarına yanıt veren bir şey yok.',
    'mconn.reach.notHere.next.admin': 'Postayı Bileşenler sayfasından kurun.',
    'mconn.reach.notHere.next.other': 'Sunucu yöneticisinden postayı kurmasını isteyin.',
    'mconn.reach.notHere.then': 'Bu pencere çalışan ayarları gösterir ve kopyalanabilir.',
    'mconn.reach.maybe.title': 'Bu ayarlar bu posta kutusuna ulaşmayabilir',
    'mconn.reach.nameUnset.why': 'Sunucunun posta adı henüz belirlenmedi; {name} yalnız alışılmış addır, bu sunucunun doğruladığı bir ad değildir.',
    'mconn.reach.nameUnset.next.admin': 'Posta adını posta ayarları sayfasından belirleyin.',
    'mconn.reach.nameUnset.next.other': 'Hangi sunucu adının kullanılacağını sunucu yöneticisine sorun.',
    'mconn.reach.nameUnset.then': 'Bu pencere sunucunun posta adını gösterir.',
    'mconn.reach.unconfirmed.why': '{name} bu alan adı için yalnız alışılmış addır; bu sunucu onu posta için seçmedi ve denetlemedi.',
    'mconn.reach.unconfirmed.next.admin': 'Gelen ve giden sunucu olarak bu sunucunun posta ayarları sayfasında görünen kendi posta adını verin.',
    'mconn.reach.unconfirmed.next.other': 'Hangi sunucu adının kullanılacağını sunucu yöneticisine sorun.',
    'mconn.text.unconfirmed': 'Not: aşağıdaki sunucu adı henüz doğrulanmadı. Kullanmadan önce bu ayarları size verene danışın.',
    'mconn.text.title': '{address} için posta ayarları',
    'mconn.text.password': 'Parola: size ayrıca iletilen parola (bu mesajda yok)',
    'mconn.text.smtpAuth': '  Oturum açma: gerekli, aynı kullanıcı adı ve parolayla',
};
