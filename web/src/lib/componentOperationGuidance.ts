import type { TranslationKey } from '../i18n/en';

interface OperationObservation {
    status: string;
    phase: string;
}

export interface ComponentOperationGuidance {
    statusKey?: TranslationKey;
    hintKey: TranslationKey;
    failed: boolean;
}

// A lost connection is not evidence that installation is still running. A
// recorded failure must stay visible while the separate state scan retries.
// Baglanti kaybi kurulumun surdugunu kanitlamaz. Kaydedilmis hata, ayri durum
// taramasi yeniden denenirken gorunur kalmalidir.
export function componentOperationGuidance(
    operation: OperationObservation | null,
    interrupted: boolean,
    recovering: boolean,
    refreshing: boolean,
): ComponentOperationGuidance {
    if (operation?.status === 'failed') {
        return {
            statusKey: 'services.operation.failedChecking',
            hintKey: interrupted
                ? 'services.operation.failedConnectionHint'
                : 'services.operation.failedCheckingHint',
            failed: true,
        };
    }
    if (interrupted || recovering) {
        return {
            statusKey: interrupted
                ? 'services.operation.reconnecting'
                : 'services.operation.recoveringRequest',
            hintKey: 'services.operation.uncertainHint',
            failed: false,
        };
    }
    if (refreshing || operation?.status === 'succeeded') {
        return {
            statusKey: 'services.operation.refreshing',
            hintKey: 'services.operation.verifyingHint',
            failed: false,
        };
    }
    return { hintKey: 'services.operation.backgroundHint', failed: false };
}

const phaseKeys: Record<string, TranslationKey> = {
    queued: 'services.operation.phase.queued',
    running: 'services.operation.phase.running',
    preflight: 'services.operation.phase.preparing',
    preparing: 'services.operation.phase.preparing',
    downloading: 'services.operation.phase.downloading',
    installing: 'services.operation.phase.installing',
    configuring: 'services.operation.phase.configuring',
    starting: 'services.operation.phase.starting',
    verifying: 'services.operation.phase.verifying',
    scanning: 'services.operation.phase.scanning',
    syncing: 'services.operation.phase.syncing',
    syncing_dns: 'services.operation.phase.syncing_dns',
    firewall_waiting: 'services.operation.phase.firewall_waiting',
    panel_busy: 'services.operation.phase.panel_busy',
    panel_stayed_busy: 'services.operation.phase.panel_stayed_busy',
    finalizing: 'services.operation.phase.finalizing',
    'mail-stack': 'services.operation.phase.configuring',
    submission: 'services.operation.phase.configuring',
    'dkim-signing': 'services.operation.phase.configuring',
};

export function componentOperationPhaseKey(phase: string): TranslationKey {
    // Mail profiles prefix a known phase with their profile and component IDs.
    // Posta profilleri bilinen asamanin onune profil ve bilesen kimliklerini ekler.
    const profile = /^profile\/(core-mail|webmail|protected-mail)\/(?:[a-z0-9-]+\/)?([a-z_\-]+)$/.exec(phase);
    const key = profile ? profile[2] : phase;
    return Object.prototype.hasOwnProperty.call(phaseKeys, key) ? phaseKeys[key] : 'services.operation.running';
}

// alpha.123 (D-045): the server-side remove-together operation, as its lock
// says it: the step it is on ("removing:phppgadmin"), who acts (nobody) and
// how it resumes (it goes on on the server, and after a restart by itself).
// Sunucudaki birlikte kaldırma işleminin adımı, kimin yapacağı, nasıl sürdüğü.
export const REMOVE_TOGETHER_KIND = 'service_remove_together';
// alpha.124: install-together says the component it is installing now.
export const INSTALL_TOGETHER_KIND = 'service_install_together';

export function removalOperationPhase(phase: string): { key: TranslationKey; name: string } {
    if (phase.startsWith('removing:')) return { key: 'services.operation.phase.removing', name: phase.slice(9) };
    if (phase === 'setting:malware_schedule') return { key: 'services.operation.phase.removeSetting', name: '' };
    if (phase.startsWith('firewall')) return { key: 'services.operation.phase.syncing', name: '' };
    return { key: 'services.operation.starting', name: '' };
}
