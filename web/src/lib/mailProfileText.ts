import type { TranslationKey } from '../i18n/en';

// What a mail profile is called and what it says, in the interface language.
// The server sends its three compiled profiles with an English name and
// description, and its state sentences as English text (cmd/panel/
// mail_profiles.go). A Turkish page showed them in English (sixteenth record,
// alpha.84). The profile id names its words; a server sentence this page has
// words for is said in the interface language, and any other is shown as the
// server wrote it. Matching the server's sentence is an interim: a typed
// reason on the answer would replace it (a server change, not made yet).
//
// Bir posta profilinin adı ve söyledikleri, arayüz dilinde. Sunucu üç profili
// İngilizce ad ve açıklamayla, durum cümlelerini İngilizce metin olarak
// gönderir. Profil kimliği kendi sözlerini adlandırır; bu sayfanın sözü olan
// sunucu cümlesi arayüz dilinde söylenir, olmayan sunucunun yazdığı gibi kalır.

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

const profileWords: Record<string, { name: TranslationKey; description: TranslationKey }> = {
    'core-mail': { name: 'dashboard.audit.profile.coreMail', description: 'services.mailProfiles.description.coreMail' },
    webmail: { name: 'dashboard.audit.profile.webmail', description: 'services.mailProfiles.description.webmail' },
    'protected-mail': { name: 'dashboard.audit.profile.protectedMail', description: 'services.mailProfiles.description.protectedMail' },
};

// The server's sentences, exactly as cmd/panel/mail_profiles.go writes them.
const serverSentences: Record<string, TranslationKey> = {
    'Components are running. Run Repair to verify and reconcile the complete mail profile, including mail TLS and authenticated submission.': 'services.mailProfiles.text.repairToVerify',
    'Webmail runtime readiness is unverified; run a fresh scan.': 'services.mailProfiles.text.webmailUnverified',
    'Roundcube is installed, but its Unix-socket runtime is not ready.': 'services.mailProfiles.text.roundcubeNotReady',
    'Some components are already present. Installing this profile will reconcile the full plan without removing completed services.': 'services.mailProfiles.text.partlyPresent',
    'Service state is unverified; run a fresh scan.': 'services.mailProfiles.text.stateUnverified',
    'Fresh service state is incomplete; run another scan.': 'services.mailProfiles.text.stateIncomplete',
    'The server profile catalogue is incomplete.': 'services.mailProfiles.text.catalogueIncomplete',
};

interface ProfileIdentity {
    id: string;
    name: string;
    description?: string;
}

export function mailProfileName(profile: ProfileIdentity, t: Translate): string {
    const words = profileWords[profile.id];
    return words ? t(words.name) : profile.name;
}

export function mailProfileDescription(profile: ProfileIdentity, t: Translate): string {
    const words = profileWords[profile.id];
    return words ? t(words.description) : profile.description ?? '';
}

export function mailProfileSentence(text: string, t: Translate): string {
    const key = serverSentences[text.trim()];
    return key ? t(key) : text;
}
