// Who renews a website certificate and what the administrator does about it
// (D-040, D-024). A certificate issued since D-040 is renewed by the
// operating system's certbot timer, also while CelikPanel is stopped or
// removed; the Panel only reads what certbot did. One of an earlier release
// is renewed by the running Panel until it is reissued.
//
// Bir web sitesi sertifikasını kimin yenilediği ve yöneticinin ne yapacağı.

export interface SiteRenewalState {
    state: string;
    detail?: string;
    lineage_name: string;
    not_after?: string;
    renew_after?: string;
    scheduler?: { name?: string; enabled: boolean; active: boolean; known: boolean };
    hook?: string;
    last_deploy?: { completed_at: string; nginx: string; mail?: string; dovecot?: string } | null;
    last_failed?: { at: string; detail: string } | null;
    auto_renew?: boolean;
}

export interface RenewalCertificate {
    type: string;
    renewal_status: string;
    renewal_owner?: string;
    // automatic | reissue (additive; renewal_owner "panel" only): whether
    // the Panel moves it to the operating system's renewal by itself.
    legacy_move?: string;
    renewal?: SiteRenewalState | null;
    renewal_read_error?: string;
}

// renewal_status of a certificate of an earlier release whose move to the
// operating system's renewal failed: "move_failed_<kind>".
export const LEGACY_MOVE_FAILED_PREFIX = 'move_failed_';
const legacyMoveKinds = new Set(['authority_unreachable', 'validation', 'rate_limited', 'timeout', 'tool', 'other']);

export const NATIVE_RENEWAL_PREFIX = 'os_';

// The OS states that need the server administrator. Unknown is not one of
// them: it is not known.
const problemStates = new Set([
    'failed', 'overdue', 'expired', 'scheduler_missing', 'disabled',
    'deploy_incomplete', 'hook_missing', 'hook_changed', 'lineage_missing',
]);

export function isRenewalProblem(state: string): boolean {
    return problemStates.has(state);
}

// The OS state the page shows: the state read now, else the one the Panel
// recorded at its last reading ("os_<state>").
export function nativeRenewalState(cert: RenewalCertificate): string | null {
    if (cert.renewal_owner !== 'operating_system') return null;
    if (cert.renewal?.state) return cert.renewal.state;
    if (cert.renewal_status.startsWith(NATIVE_RENEWAL_PREFIX)) {
        return cert.renewal_status.slice(NATIVE_RENEWAL_PREFIX.length);
    }
    return 'unknown';
}

export type RenewalTone = 'ok' | 'info' | 'warning' | 'danger';

export interface RenewalGuidance {
    tone: RenewalTone;
    // Translation keys: the reason, then the concrete next action (absent
    // when nothing needs doing). The action is the server administrator's;
    // it names no command and no server path (alpha.134 lab run 6: a
    // customer read /etc/letsencrypt/renewal/… on the Security section).
    reasonKey: string;
    actionKey?: string;
    // What a reader who is not the administrator can do on this page
    // (absent: they are told to contact the server's administrator).
    selfKey?: string;
    // The action is on this page for every reader (an earlier release's
    // certificate: reissue it here).
    pageAction?: boolean;
    // How it resumes, when it is not the default line of its owner.
    resumeKey?: string;
    params: Record<string, string>;
    // For the administrator only, in closed technical details: the commands
    // to run on the server, the file concerned, certbot's own words.
    commands: string[];
    file?: string;
    detail?: string;
}

const certbotRecord = '/var/log/letsencrypt/letsencrypt.log';

// The administrator's commands and file per state (fixed text and the
// lineage's own name; nothing of an answer is run or shown in a sentence).
function adminDetails(state: string, lineage: string, timer: string): { commands: string[]; file?: string } {
    const renew = `sudo certbot renew --cert-name ${lineage}`;
    switch (state) {
        case 'failed':
            return { commands: [renew], file: certbotRecord };
        case 'overdue':
            return { commands: [`sudo systemctl status ${timer}`, renew] };
        case 'expired':
            return { commands: [renew] };
        case 'scheduler_missing':
            return { commands: ['sudo systemctl enable --now certbot.timer'] };
        case 'disabled':
            return { commands: [], file: `/etc/letsencrypt/renewal/${lineage}.conf` };
        case 'deploy_incomplete':
            return { commands: ['sudo nginx -t', 'sudo systemctl reload nginx'] };
        case 'deploy_incomplete_mail':
            return { commands: ['sudo postfix check', 'sudo doveconf -n', 'sudo postmap -F hash:/etc/postfix/celikpanel_sni', 'sudo systemctl reload postfix dovecot'] };
        case 'hook_missing':
            return { commands: ['sudo systemctl reload nginx'] };
        case 'hook_changed':
            return { commands: [], file: '/etc/letsencrypt/renewal-hooks/deploy/celikpanel-site-certificates' };
        case 'panel_move_failed':
            return { commands: ['sudo journalctl -u celikpanel-panel -n 50'], file: certbotRecord };
        default:
            return { commands: [] };
    }
}

// What a reader who is not the administrator can do here for a state.
const selfActions: Record<string, string> = {
    disabled: 'ssl.renewalGuide.disabled.self',
    expired: 'ssl.renewalGuide.lineage_missing.action',
    lineage_missing: 'ssl.renewalGuide.lineage_missing.action',
    hook_missing: 'ssl.renewalGuide.lineage_missing.action',
};

const guidance: Record<string, { tone: RenewalTone; action?: boolean }> = {
    scheduled: { tone: 'ok' },
    renewed: { tone: 'ok' },
    owner_configured: { tone: 'info' },
    unknown: { tone: 'info' },
    failed: { tone: 'danger', action: true },
    overdue: { tone: 'danger', action: true },
    expired: { tone: 'danger', action: true },
    lineage_missing: { tone: 'danger', action: true },
    scheduler_missing: { tone: 'danger', action: true },
    deploy_incomplete: { tone: 'warning', action: true },
    disabled: { tone: 'warning', action: true },
    hook_missing: { tone: 'warning', action: true },
    hook_changed: { tone: 'warning', action: true },
};

// renewalGuidance is what the certificate card says about renewal, the
// reason first and the next action after it; null for an uploaded one.
export function renewalGuidance(cert: RenewalCertificate): RenewalGuidance | null {
    if (cert.type !== 'letsencrypt') return null;
    if (cert.renewal_owner === 'panel') {
        // The move to the operating system's renewal failed at its last
        // try: why, the next action, and that the certificate keeps serving.
        if (cert.renewal_status.startsWith(LEGACY_MOVE_FAILED_PREFIX)) {
            const raw = cert.renewal_status.slice(LEGACY_MOVE_FAILED_PREFIX.length);
            const kind = legacyMoveKinds.has(raw) ? raw : 'other';
            return {
                tone: 'warning',
                reasonKey: `ssl.renewalGuide.panel_move_failed.${kind}`,
                actionKey: 'ssl.renewalGuide.panel_move_failed.action',
                resumeKey: 'ssl.renewalGuide.panel_move.resume',
                params: {},
                ...adminDetails('panel_move_failed', '', ''),
            };
        }
        if (cert.legacy_move === 'automatic') {
            return {
                tone: 'info',
                reasonKey: 'ssl.renewalGuide.panel',
                actionKey: 'ssl.renewalGuide.panel.automatic',
                pageAction: true,
                resumeKey: 'ssl.renewalGuide.panel_move.resume',
                params: {},
                commands: [],
            };
        }
        return { tone: 'info', reasonKey: 'ssl.renewalGuide.panel', actionKey: 'ssl.renewalGuide.panel.action', pageAction: true, params: {}, commands: [] };
    }
    const state = nativeRenewalState(cert);
    if (state === null) return null;
    const entry = guidance[state] ?? guidance.unknown;
    const known = guidance[state] ? state : 'unknown';
    const renewal = cert.renewal;
    const params: Record<string, string> = {
        lineage: renewal?.lineage_name || '',
        nginx: renewal?.last_deploy?.nginx || '',
        date: renewal?.renew_after ? renewal.renew_after.slice(0, 10) : '',
    };
    const lineage = renewal?.lineage_name || '';
    const timer = renewal?.scheduler?.name || 'certbot.timer';
    // certbot's own words: for the administrator, in the closed details.
    const detail = known === 'failed' || known === 'expired' ? renewal?.last_failed?.detail || renewal?.detail || undefined : undefined;
    // Renewed and nginx loaded it, but mail did not: its own sentence.
    const deploy = renewal?.last_deploy;
    const mailFailed = deploy && (deploy.mail === 'postmap_failed' || deploy.mail === 'reload_failed' || deploy.dovecot === 'reload_failed');
    if (known === 'deploy_incomplete' && mailFailed && (deploy.nginx === 'reloaded' || deploy.nginx === 'not_running' || deploy.nginx === 'not_installed')) {
        params.mail = deploy.mail || '';
        params.dovecot = deploy.dovecot || '';
        return {
            tone: 'warning', reasonKey: 'ssl.renewalGuide.deploy_incomplete_mail', actionKey: 'ssl.renewalGuide.deploy_incomplete_mail.action',
            params, ...adminDetails('deploy_incomplete_mail', lineage, timer),
        };
    }
    if (known === 'unknown' && cert.renewal_read_error) {
        return { tone: 'info', reasonKey: 'ssl.renewalGuide.unknown', params, commands: [] };
    }
    return {
        tone: entry.tone,
        reasonKey: `ssl.renewalGuide.${known}`,
        actionKey: entry.action ? `ssl.renewalGuide.${known}.action` : undefined,
        selfKey: entry.action ? selfActions[known] : undefined,
        params,
        detail,
        ...adminDetails(known, lineage, timer),
    };
}

// renewalLabelKey is the short label of the Renewal line.
export function renewalLabelKey(cert: RenewalCertificate): string | null {
    const state = nativeRenewalState(cert);
    if (state === null) return null;
    return guidance[state] ? `ssl.renewal.os.${state}` : 'ssl.renewal.os.unknown';
}
