import type { TranslationKey } from '../i18n/en';
import { componentName } from './componentName';

// What an audit action says in Recent activity, after the user's name. The
// Panel writes an action as its name, then optionally ':' and a target, then
// optionally details after a space or ' — ' (cmd/panel auditActionNames lists
// every name; tests/audit-actions.test.mjs requires a sentence for each). The
// Dashboard showed the raw action for every name but two (set16 observation
// O3: server.setup.child:nginx, server_setup_mail_enrollment_requested on both
// installed servers). The details stay out of the sentence. An action this
// page has no words for (a newer Panel) is said as its words, never as its
// raw id: dots, underscores, colons and dashes become spaces.
//
// Bir denetim eyleminin Son etkinlik'teki cümlesi. Panel eylemi ad, isteğe
// bağlı ':' ve hedef, isteğe bağlı ayrıntı olarak yazar. Ayrıntı cümleye
// girmez. Sayfanın bilmediği bir eylem ham kimlik olarak değil, sözcükleri
// olarak gösterilir.

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

const profileNames: Record<string, TranslationKey> = {
    'core-mail': 'dashboard.audit.profile.coreMail',
    webmail: 'dashboard.audit.profile.webmail',
    'protected-mail': 'dashboard.audit.profile.protectedMail',
};

// The names whose target is a catalogue component (or a service unit named
// like one), said by its display name.
const componentTarget = /^(?:service\.(?:install|uninstall|start|stop|restart|reload)|server\.setup\.child)(?:\.|$)/;

export interface AuditActionParts {
    name: string;
    target: string;
}

export function auditActionParts(action: string): AuditActionParts {
    const event = action.split(' — ', 1)[0].trim();
    const match = /^([^:\s]+)(?::(\S*))?/.exec(event);
    if (!match) return { name: '', target: '' };
    // The target is the first ':' segment: domain.delete.pending:<domain>:<stage>,
    // service.uninstall:<component>:<package>.
    return { name: match[1], target: (match[2] ?? '').split(':', 1)[0] };
}

function targetText(name: string, target: string, t: Translate): string {
    if (!target) return '';
    if (name.startsWith('mail.profile.install')) {
        const key = profileNames[target];
        return key ? t(key) : target;
    }
    if (name === 'user.create') return target.split('/', 1)[0];
    if (componentTarget.test(name)) return componentName(target, t);
    return target;
}

// The name becomes words; its target is kept as written, after them, so a
// domain or an address stays readable ("example.com", not "example com").
// Ad sözcüklere çevrilir; hedef yazıldığı gibi arkasında kalır.
export function humaniseAuditAction(action: string): string {
    const { name, target } = auditActionParts(action);
    const words = name.replace(/[._:\-/]+/g, ' ').trim();
    if (words) return target ? `${words} ${target}` : words;
    const event = action.split(' — ', 1)[0].trim();
    return event.split(/\s/, 1)[0].replace(/[._:\-/]+/g, ' ').trim() || action.trim();
}

// Setup prepares a mail profile as one of its steps and writes the profile's
// id as the target (server.setup.child:protected-mail). The owner saw
// "prepared protected-mail during setup" on alpha.92 (2026-10-16); a profile
// is said by its name, and in Turkish as a profile, not as a component.
// Kurulum bir posta profilini adımlarından biri olarak hazırlar ve hedefe
// profilin kimliğini yazar; profil adıyla, Türkçede bileşen değil profil
// olarak söylenir.
const setupChildProfileSentence: Record<string, TranslationKey> = {
    'server.setup.child': 'dashboard.audit.setupChildProfile',
    'server.setup.child.failed': 'dashboard.audit.setupChildProfileFailed',
};

export function auditActionText(action: string, t: Translate): string {
    const { name, target } = auditActionParts(action);
    if (!/^[a-z0-9_]+(?:[.-][a-z0-9_]+)*$/.test(name)) return humaniseAuditAction(action);
    const profileSentence = setupChildProfileSentence[name];
    if (profileSentence && profileNames[target]) return t(profileSentence, { target: t(profileNames[target]) });
    const key = `dashboard.audit.action.${name}` as TranslationKey;
    const said = t(key, { target: targetText(name, target, t) });
    // No words for this name, or words that need a target the action lacks.
    if (said === key || (!target && t(key).includes('{target}'))) return humaniseAuditAction(action);
    return said;
}

// Who did it. The Panel writes the reserved name "system" for an action that
// has no signed-in user (cmd/panel/audit.go); that one name is said in the
// interface language, every other name is shown exactly as the user typed it.
//
// Eylemi kimin yaptığı. Panel, oturum açmış kullanıcısı olmayan eylem için
// "system" adını yazar; yalnızca bu ad arayüz dilinde söylenir, kullanıcı
// adları olduğu gibi gösterilir.
export function auditActorName(username: string, t: Translate): string {
    return username === 'system' ? t('dashboard.audit.actor.system') : username;
}
