import { createContext, useContext } from 'react';
import type { AccessObservation } from './accessObservation';

/**
 * The access gate's current decision for the pages below it (LicenseOnboarding):
 * whether the licence allows full use, its state, and for administrators when the
 * licence term ends. Null until the first answer.
 *
 * Erişim kapısının alttaki sayfalar için güncel kararı: lisans tam kullanıma izin
 * veriyor mu, durumu ve yöneticiler için lisans süresinin bittiği an.
 */
export const LicenseAccessContext = createContext<AccessObservation | null>(null);

export function useLicenseAccess(): AccessObservation | null {
    return useContext(LicenseAccessContext);
}

/** After an activation or "Check again" the gate reads access at once. */
export const LICENSE_CHANGED_EVENT = 'celikpanel:license-changed';
