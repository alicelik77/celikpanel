// Only a reviewed updater's explicit pre-mutation outcome supports retry
// guidance. Historical generic failures and recovery-required states retain
// their diagnostic text; never infer safety from a package-manager substring.
export function systemUpdateFailureMessage(
    diagnostic: string,
    t: (key: 'panelUpdate.packageManagerBusy') => string,
): string {
    return systemUpdatePackageManagerBusy(diagnostic)
        ? t('panelUpdate.packageManagerBusy')
        : diagnostic;
}

/**
 * The reviewed updater's terminal preflight stop, parsed from its summary line.
 * reasonClass is present only for a refused update check (update_preflight_refused).
 */
export type SystemUpdatePreflightStop = { step: string; diagnostic: string; reasonClass?: string };

// The updater emits, both with state=unchanged:
//   code=update_preflight_refused reason=update preflight step=<step> class=<class>: <reason> detail=<checker line>
//   code=recovery_runtime_preflight_failed reason=recovery runtime preflight step=<step>: <diagnostic> detail=
// The Panel may shorten either to its bounded form (no reason text, empty detail).
// Only these exact unchanged outcomes are recognised; nothing is inferred.
export function systemUpdatePreflightStop(diagnostic: string): SystemUpdatePreflightStop | undefined {
    const refused = /(?:^|: )!! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason=update preflight step=([a-z_]{1,40}) class=([a-z_]{1,40})(?:: .*?)? detail=(.*)$/.exec(diagnostic);
    if (refused) return { step: refused[1], reasonClass: refused[2], diagnostic: refused[3].trim() };
    const match = /(?:^|: )!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step=([a-z_]{1,40})(?:: (.*))? detail=/.exec(diagnostic);
    if (!match) return undefined;
    return { step: match[1], diagnostic: (match[2] ?? '').trim() };
}

/**
 * The server's summary without the updater's internal tokens (the marker,
 * code= and state= values, and the reason=/detail= labels). Empty when nothing
 * readable remains, so the caller omits the line.
 */
export function withoutInternalTokens(message: string): string {
    const cleaned = message
        .replace(/!! CELIKPANEL_UPDATE_FAILURE/g, ' ')
        .replace(/\b(?:code|state)=\S*/g, ' ')
        .replace(/\b(?:reason|detail)=/g, ' ')
        .replace(/\s+/g, ' ')
        .replace(/[\s:;,]+$/, '')
        .trim();
    return /[A-Za-z0-9]/.test(cleaned) ? cleaned : '';
}

/**
 * The reviewed updater's transient stop: the server's package manager was
 * busy (for example with automatic security updates), so the update stopped
 * before changing anything (state=unchanged). Frankfurt's update to alpha.95
 * stopped this way on 2026-10-17, and the page drew it as a generic failure.
 * Sunucunun paket yöneticisi meşguldü; güncelleme hiçbir şeyi değiştirmeden durdu.
 */
export function systemUpdatePackageManagerBusy(diagnostic: string): boolean {
    return /(?:^|: )!! CELIKPANEL_UPDATE_FAILURE code=package_manager_busy state=unchanged reason=/.test(diagnostic);
}

/**
 * The updater's own untyped stop when the recovery tool could not confirm its
 * preparation for the new version (update.sh: "independent recovery runtime
 * preparation is unconfirmed; panel services have not been stopped"). It comes
 * before any release transaction and before any service is stopped, so the
 * server's recovery has nothing of it to finish. alpha.137 lab: a package
 * install that began seven seconds after the press ended this way, and the
 * page waited for a recovery that never comes, with no way to start again.
 * Kurtarma aracı hazırlığını doğrulayamadı; hiçbir hizmet durdurulmadı.
 */
export function systemUpdateRecoveryPreparationUnconfirmed(diagnostic: string): boolean {
    return /(?:^|: )!! independent recovery runtime preparation is unconfirmed; panel services have not been stopped(?:;|$)/.test(diagnostic);
}

/**
 * The reviewed updater's typed summary of a stop before any change: the busy
 * package manager, a refused read-only check or the recovery tool's preflight,
 * each with state=unchanged. Any other code keeps its general words, even
 * when unchanged. Güncelleyicinin tipli ön denetim duruşu özeti.
 */
export function systemUpdateStoppedUnchangedSummary(diagnostic: string): boolean {
    return /(?:^|: )!! CELIKPANEL_UPDATE_FAILURE code=(?:package_manager_busy|update_preflight_refused|recovery_runtime_preflight_failed) state=unchanged reason=/.test(diagnostic);
}
