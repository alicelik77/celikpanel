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
    renewal?: SiteRenewalState | null;
    renewal_read_error?: string;
}

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
    // when nothing needs doing).
    reasonKey: string;
    actionKey?: string;
    params: Record<string, string>;
}

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
        return { tone: 'info', reasonKey: 'ssl.renewalGuide.panel', actionKey: 'ssl.renewalGuide.panel.action', params: {} };
    }
    const state = nativeRenewalState(cert);
    if (state === null) return null;
    const entry = guidance[state] ?? guidance.unknown;
    const known = guidance[state] ? state : 'unknown';
    const renewal = cert.renewal;
    const params: Record<string, string> = {
        lineage: renewal?.lineage_name || '',
        timer: renewal?.scheduler?.name || 'certbot.timer',
        detail: renewal?.last_failed?.detail || renewal?.detail || '',
        nginx: renewal?.last_deploy?.nginx || '',
        date: renewal?.renew_after ? renewal.renew_after.slice(0, 10) : '',
    };
    // Renewed and nginx loaded it, but mail did not: its own sentence.
    const deploy = renewal?.last_deploy;
    const mailFailed = deploy && (deploy.mail === 'postmap_failed' || deploy.mail === 'reload_failed' || deploy.dovecot === 'reload_failed');
    if (known === 'deploy_incomplete' && mailFailed && (deploy.nginx === 'reloaded' || deploy.nginx === 'not_running' || deploy.nginx === 'not_installed')) {
        params.mail = deploy.mail || '';
        params.dovecot = deploy.dovecot || '';
        return { tone: 'warning', reasonKey: 'ssl.renewalGuide.deploy_incomplete_mail', actionKey: 'ssl.renewalGuide.deploy_incomplete_mail.action', params };
    }
    if (known === 'unknown' && cert.renewal_read_error) {
        return { tone: 'info', reasonKey: 'ssl.renewalGuide.unknown', params };
    }
    return {
        tone: entry.tone,
        reasonKey: `ssl.renewalGuide.${known}`,
        actionKey: entry.action ? `ssl.renewalGuide.${known}.action` : undefined,
        params,
    };
}

// renewalLabelKey is the short label of the Renewal line.
export function renewalLabelKey(cert: RenewalCertificate): string | null {
    const state = nativeRenewalState(cert);
    if (state === null) return null;
    return guidance[state] ? `ssl.renewal.os.${state}` : 'ssl.renewal.os.unknown';
}
