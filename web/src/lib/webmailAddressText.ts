// The words for webmail's address (alpha.110): what the read-only check found
// and how an enable or remove operation ended, in the owner's language.
//
// The server sends English sentences with its check and its results. Those are
// kept as a fallback only: every code the screen knows is said in the interface
// language, with the current reason first, who acts, the next action, and how
// the work resumes (D-024). A verified failure, an unmet prerequisite and an
// unknown result are three different things and are never said as one.
//
// Web postası adresinin sözleri: salt okunur denetimin bulduğu ve bir kurma ya
// da kaldırma işleminin nasıl bittiği, arayüz dilinde. Sunucunun İngilizce
// cümleleri yalnız yedektir.
import type { TranslationKey } from '../i18n/en';
import type { WebmailAddressCheck, WebmailAddressOutcome } from './webmailAddress';

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;
export type SaidTone = 'ok' | 'warn' | 'bad' | 'info';
export type CheckKind = 'ready' | 'unmet' | 'elsewhere' | 'unknown' | 'refused';

export interface CheckSaid {
    kind: CheckKind;
    tone: SaidTone;
    kindText: string;
    headline: string;
    who: string;
    next: string;
}

const CHECK_CODES = ['no_record', 'points_elsewhere', 'points_to_peer', 'dns_unreachable', 'server_address_unknown', 'invalid', 'panel_address', 'firewall_closed'] as const;
type CheckCode = (typeof CHECK_CODES)[number];
const isCheckCode = (code: string): code is CheckCode => (CHECK_CODES as readonly string[]).includes(code);

const MESSAGE_KEY: Record<CheckCode | 'ready', TranslationKey> = {
    ready: 'life.addr.msg.ready',
    no_record: 'life.addr.msg.no_record',
    points_elsewhere: 'life.addr.msg.points_elsewhere',
    points_to_peer: 'life.addr.msg.points_to_peer',
    dns_unreachable: 'life.addr.msg.dns_unreachable',
    server_address_unknown: 'life.addr.msg.server_address_unknown',
    invalid: 'life.addr.msg.invalid',
    panel_address: 'life.addr.msg.panel_address',
    firewall_closed: 'life.addr.msg.firewall_closed',
};

const NEXT_KEY: Record<Exclude<CheckCode, 'no_record'>, TranslationKey> = {
    points_elsewhere: 'life.addr.next.points_elsewhere',
    points_to_peer: 'life.addr.next.points_to_peer',
    dns_unreachable: 'life.addr.next.dns_unreachable',
    server_address_unknown: 'life.addr.next.server_address_unknown',
    invalid: 'life.addr.next.invalid',
    panel_address: 'life.addr.next.panel_address',
    firewall_closed: 'life.addr.next.firewall_closed',
};

// The server names who acts in a few fixed English phrases.
function whoText(who: string, t: Translate): string {
    switch (who.toLowerCase().replace(/’/g, "'")) {
        case 'administrator': return t('life.addr.who.administrator');
        case "whoever manages this name's dns": return t('life.addr.who.dns');
        case 'server owner': return t('life.addr.who.owner');
        case 'nobody yet': return t('life.addr.who.nobody');
        default: return who;
    }
}

export function checkKind(check: WebmailAddressCheck): CheckKind {
    if (check.state === 'ready') return check.ready ? 'ready' : 'unmet';
    if (check.state === 'not_found') return 'unmet';
    if (check.state === 'elsewhere') return 'elsewhere';
    if (check.state === 'refused') return 'refused';
    return 'unknown';
}

const KIND_TONE: Record<CheckKind, SaidTone> = { ready: 'ok', unmet: 'warn', elsewhere: 'warn', unknown: 'info', refused: 'warn' };
const KIND_KEY: Record<CheckKind, TranslationKey> = {
    ready: 'life.addr.kind.ready',
    unmet: 'life.addr.kind.unmet',
    elsewhere: 'life.addr.kind.elsewhere',
    unknown: 'life.addr.kind.unknown',
    refused: 'life.addr.kind.refused',
};

export function sayCheck(check: WebmailAddressCheck, t: Translate): CheckSaid {
    const kind = checkKind(check);
    const peerText = check.peer ? (check.peer.name ? `${check.peer.name} (${check.peer.address})` : check.peer.address) : '';
    const vars = {
        host: check.host,
        answered: check.answered.join(', '),
        expected: check.expected.join(', '),
        ports: check.firewall.closed.join(' / '),
        peer: peerText,
        zone: check.zone?.name ?? '',
    };
    const code = isCheckCode(check.code) ? check.code : null;
    const headline = kind === 'ready' ? t(MESSAGE_KEY.ready, vars) : code ? t(MESSAGE_KEY[code], vars) : check.message;
    let next = '';
    if (kind !== 'ready') {
        if (code === 'no_record') next = t(check.zone ? 'life.addr.next.no_record.zone' : 'life.addr.next.no_record.external', vars);
        else if (code) next = t(NEXT_KEY[code], vars);
        else next = check.ownerAction?.next ?? '';
    }
    return {
        kind,
        tone: KIND_TONE[kind],
        kindText: t(KIND_KEY[kind]),
        headline,
        who: kind !== 'ready' && check.ownerAction?.who ? whoText(check.ownerAction.who, t) : '',
        next,
    };
}

export interface OutcomeSaid {
    tone: SaidTone;
    text: string;
    extras: { tone: SaidTone; text: string }[];
}

const FAIL_CODES = ['ADDRESS_INVALID', 'ROUNDCUBE_NOT_INSTALLED', 'NGINX_UNAVAILABLE', 'PHP_UNAVAILABLE', 'CERTBOT_MISSING', 'NAME_IN_USE', 'CONFIG_OWNER_EDITED', 'NGINX_REJECTED', 'CERTIFICATE_FAILED', 'PROOF_FAILED', 'ROLLBACK_FAILED', 'BUSY', 'STATE_UNREADABLE', 'CHALLENGE_UNREACHABLE'] as const;
type FailCode = (typeof FAIL_CODES)[number];
const isFailCode = (code: string): code is FailCode => (FAIL_CODES as readonly string[]).includes(code);

const FAIL_KEY: Record<FailCode, TranslationKey> = {
    ADDRESS_INVALID: 'life.addr.fail.ADDRESS_INVALID',
    ROUNDCUBE_NOT_INSTALLED: 'life.addr.fail.ROUNDCUBE_NOT_INSTALLED',
    NGINX_UNAVAILABLE: 'life.addr.fail.NGINX_UNAVAILABLE',
    PHP_UNAVAILABLE: 'life.addr.fail.PHP_UNAVAILABLE',
    CERTBOT_MISSING: 'life.addr.fail.CERTBOT_MISSING',
    NAME_IN_USE: 'life.addr.fail.NAME_IN_USE',
    CONFIG_OWNER_EDITED: 'life.addr.fail.CONFIG_OWNER_EDITED',
    NGINX_REJECTED: 'life.addr.fail.NGINX_REJECTED',
    CERTIFICATE_FAILED: 'life.addr.fail.CERTIFICATE_FAILED',
    PROOF_FAILED: 'life.addr.fail.PROOF_FAILED',
    ROLLBACK_FAILED: 'life.addr.fail.ROLLBACK_FAILED',
    BUSY: 'life.addr.fail.BUSY',
    STATE_UNREADABLE: 'life.addr.fail.STATE_UNREADABLE',
    CHALLENGE_UNREACHABLE: 'life.addr.fail.CHALLENGE_UNREACHABLE',
};

// sayOutcome: what a finished operation says. Success is said only for an
// outcome the server's own result confirmed (see webmailAddressOutcome).
export function sayOutcome(o: WebmailAddressOutcome, t: Translate, formatDate: (value: string) => string): OutcomeSaid {
    const vars = { host: o.host, url: o.url, reason: o.reason, date: formatDate(o.certificateExpires) };
    const extras: OutcomeSaid['extras'] = [];
    if (o.roundcubeSettings === 'owner_edited') extras.push({ tone: 'info', text: t('life.addr.settingsEdited') });
    if (o.warning) extras.push({ tone: 'info', text: t('life.addr.note', { warning: o.warning }) });
    const said = (tone: SaidTone, text: string): OutcomeSaid => ({ tone, text, extras });
    if (o.remove) {
        switch (o.outcome) {
            case 'removed':
                return said('ok', `${t('life.addr.removed', vars)} ${t(o.certificateRemoved ? 'life.addr.removedCertDeleted' : 'life.addr.removedCertKept')}`);
            case 'not_removed':
                return said('warn', o.code === 'CONFIG_OWNER_EDITED' ? t('life.addr.removeFail.CONFIG_OWNER_EDITED', vars) : t('life.addr.removeFail.default', vars));
            case 'interrupted':
                return said('warn', t('life.addr.removeInterrupted', vars));
            default:
                return said('info', t('life.addr.removeUnknown', vars));
        }
    }
    switch (o.outcome) {
        case 'enabled': {
            const done = t('life.addr.done', vars);
            return said('ok', o.certificateExpires ? `${done} ${t('life.addr.doneCert', vars)}` : done);
        }
        case 'rollback_failed':
            return said('bad', t(FAIL_KEY.ROLLBACK_FAILED, vars));
        case 'not_enabled': {
            if (o.code === 'ROLLBACK_FAILED') return said('bad', t(FAIL_KEY.ROLLBACK_FAILED, vars));
            return said('warn', isFailCode(o.code) ? t(FAIL_KEY[o.code], vars) : t('life.addr.fail.default', vars));
        }
        case 'not_recorded':
            return said('warn', t('life.addr.notRecorded', vars));
        case 'interrupted':
            return said('warn', t('life.addr.interrupted', vars));
        default:
            return said('info', t('life.addr.unknownResult', vars));
    }
}
