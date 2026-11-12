import { useI18n } from '../../../i18n';

// The words of the shared saving parts (alpha.132): the save bar, the changed
// mark, the note of a setting the server changed under an edit, and the
// question before leaving unsaved changes. Kept beside the parts instead of
// in the shell catalogue, like the route-scoped copy of ../../../i18n/domainOps.ts:
// they arrive with the first screen that saves and cost the boot path nothing.
// Language follows the I18nProvider's locale.
//
// Ortak kaydetme parçalarının sözleri. Kabuk kataloğu yerine parçaların
// yanında durur; kaydeden ilk ekranla gelir ve açılış yükünü büyütmez.

const en = {
    pendingOne: '1 change not saved. Nothing changes until you press Save.',
    pending: '{n} changes not saved. Nothing changes until you press Save.',
    pendingSome: 'Changes not saved. Nothing changes until you press Save.',
    save: 'Save',
    saving: 'Saving…',
    savingNow: 'Your changes are being saved.',
    discard: 'Discard',
    changed: 'Changed',
    changedFrom: 'Changed: {from} → {to}',
    on: 'on',
    off: 'off',
    moved: 'This was changed on the server while you were editing; it is now {value}. Save writes your choice over it.',
    takeServer: 'Use the server’s value',
    stale: 'Save is off until the settings have been read again.',
    unknown: 'The connection dropped before the answer arrived, so it is not known whether this was saved. Nothing was sent a second time and your changes are still here. Check the current settings before saving again.',
    leaveTitle: 'Leave without saving?',
    leaveLostOne: '1 change on this page that was not saved',
    leaveLost: '{n} changes on this page that were not saved',
    leaveLostSome: 'The changes on this page that were not saved',
    leaveKept: 'Everything on the server stays as it was; nothing is saved.',
    leave: 'Leave without saving',
    stay: 'Stay on this page',
    partTitle: 'Close without saving?',
    partLostOne: 'This part has 1 unsaved change',
    partLost: 'This part has {n} unsaved changes',
    partLostSome: 'This part has unsaved changes',
    partClose: 'Close without saving',
    partStay: 'Keep it open',
    lostMark: 'Lost:',
    keptMark: 'Stays:',
} as const;

export type SaveWord = keyof typeof en;

const tr: Record<SaveWord, string> = {
    pendingOne: '1 değişiklik kaydedilmedi. Kaydet’e basana dek hiçbir şey değişmez.',
    pending: '{n} değişiklik kaydedilmedi. Kaydet’e basana dek hiçbir şey değişmez.',
    pendingSome: 'Değişiklikler kaydedilmedi. Kaydet’e basana dek hiçbir şey değişmez.',
    save: 'Kaydet',
    saving: 'Kaydediliyor…',
    savingNow: 'Değişiklikleriniz kaydediliyor.',
    discard: 'Vazgeç',
    changed: 'Değişti',
    changedFrom: 'Değişti: {from} → {to}',
    on: 'açık',
    off: 'kapalı',
    moved: 'Siz düzenlerken bu ayar sunucuda değişti; şu an: {value}. Kaydet’e basarsanız sizin seçiminiz yazılır.',
    takeServer: 'Sunucudakini kullan',
    stale: 'Ayarlar yeniden okunana dek Kaydet kapalı.',
    unknown: 'Yanıt gelmeden bağlantı koptu; kaydedilip kaydedilmediği bilinmiyor. Hiçbir şey ikinci kez gönderilmedi, değişiklikleriniz duruyor. Yeniden kaydetmeden önce güncel ayarlara bakın.',
    leaveTitle: 'Kaydetmeden çıkılsın mı?',
    leaveLostOne: 'Bu sayfada kaydedilmemiş 1 değişiklik',
    leaveLost: 'Bu sayfada kaydedilmemiş {n} değişiklik',
    leaveLostSome: 'Bu sayfadaki kaydedilmemiş değişiklikler',
    leaveKept: 'Sunucudaki her şey olduğu gibi kalır; hiçbir şey kaydedilmez.',
    leave: 'Kaydetmeden çık',
    stay: 'Sayfada kal',
    partTitle: 'Kaydetmeden kapatılsın mı?',
    partLostOne: 'Bu bölümde kaydedilmemiş 1 değişiklik var',
    partLost: 'Bu bölümde kaydedilmemiş {n} değişiklik var',
    partLostSome: 'Bu bölümde kaydedilmemiş değişiklikler var',
    partClose: 'Kaydetmeden kapat',
    partStay: 'Açık kalsın',
    lostMark: 'Kaybolur:',
    keptMark: 'Değişmez:',
};

/** Both catalogues, for the parity test. */
export const saveWords = { en, tr };

export function saveWord(locale: string | undefined, key: SaveWord, values?: Record<string, string | number>): string {
    let value: string = (locale === 'tr' ? tr : en)[key];
    for (const [name, replacement] of Object.entries(values || {})) value = value.split(`{${name}}`).join(String(replacement));
    return value;
}

/** The saving parts' words in the I18nProvider's language. */
export function useSaveWords() {
    const { locale } = useI18n();
    return (key: SaveWord, values?: Record<string, string | number>) => saveWord(locale, key, values);
}
