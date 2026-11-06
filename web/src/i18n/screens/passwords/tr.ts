// The Turkish half of the screen catalogue part "passwords"; ./en.ts defines its keys.
// "passwords" ekran parçasının Türkçe yarısı; anahtarlarını ./en.ts tanımlar.
import type { PasswordsScreenKey } from './en';

export const trPasswordsScreens: Record<PasswordsScreenKey, string> = {
    'pw.show': 'Şifreyi göster',
    'pw.hide': 'Şifreyi gizle',
    'pw.generate': 'Güçlü şifre üret',
    'pw.copy': 'Kopyala',
    'pw.copied': 'Kopyalandı',
    'pw.copyFailed': 'Kopyalanamadı. Şifreyi seçip kendiniz kopyalayın.',
    'pw.generated': 'Alanda güçlü bir şifre var. Kaydetmeden önce kopyalayıp güvenli bir yerde saklayın.',
    'pw.rule': 'En az {min} karakter; bir büyük harf, bir küçük harf ve bir özel karakter (örneğin - _ . !) içermeli.',
    'pw.strength': 'Güç: {level}',
    'pw.level.tooWeak': 'Çok zayıf',
    'pw.level.acceptable': 'Kabul edilebilir',
    'pw.level.strong': 'Güçlü',
    'pw.level.veryStrong': 'Çok güçlü',
    'pw.missing': 'Eksik: {list}.',
    'pw.need.invalid_text': 'satır sonu, sekme gibi denetim karakteri olmamalı',
    'pw.need.too_short': 'en az {min} karakter',
    'pw.need.too_long': 'en fazla {max} bayt',
    'pw.need.needs_upper': 'bir büyük harf',
    'pw.need.needs_lower': 'bir küçük harf',
    'pw.need.needs_special': 'bir özel karakter (örneğin - _ . !)',
    'pw.mismatch': 'İki şifre aynı değil.',
    'err.PASSWORD_WEAK': 'Şifre kurala uymuyor. Hiçbir şey değiştirilmedi.',
    'err.PASSWORD_WEAK.invalid_text': 'Şifre satır sonu, sekme gibi denetim karakterleri içeremez. Hiçbir şey değiştirilmedi.',
    'err.PASSWORD_WEAK.too_short': 'Şifre en az {min} karakter olmalı. Hiçbir şey değiştirilmedi.',
    'err.PASSWORD_WEAK.too_long': 'Şifre en fazla {max} bayt olabilir. Hiçbir şey değiştirilmedi.',
    'err.PASSWORD_WEAK.needs_upper': 'Şifrede en az bir büyük harf olmalı. Hiçbir şey değiştirilmedi.',
    'err.PASSWORD_WEAK.needs_lower': 'Şifrede en az bir küçük harf olmalı. Hiçbir şey değiştirilmedi.',
    'err.PASSWORD_WEAK.needs_special': 'Şifrede en az bir özel karakter (harf ya da rakam olmayan, örneğin - _ . !) olmalı. Hiçbir şey değiştirilmedi.',
};
