import { readRemote } from './remote';
import { decodeGuideState, GUIDE_STATE_URL, type GuideState, type TourState } from '../help/guide/tourState';

// The first-login tour's state on the server (alpha.119): the signed-in
// person's own row, GET and PUT /api/v1/me/guide-state.
// İlk giriş turunun sunucudaki durumu: kişinin kendi satırı.

/** The person's tour state, or null when it could not be read: unknown, never "pending". */
export async function readGuideState(): Promise<GuideState | null> {
    const read = await readRemote(GUIDE_STATE_URL, (raw) => {
        const state = decodeGuideState(raw);
        if (!state) throw new Error('not a guide state');
        return state;
    }, undefined, { credentials: 'same-origin', cache: 'no-store' });
    return read.state === 'known' ? read.value : null;
}

/** Saves the person's choice; false when the server did not confirm it. */
export async function saveGuideState(version: number, state: TourState): Promise<boolean> {
    try {
        const response = await fetch(GUIDE_STATE_URL, {
            method: 'PUT',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ tour_version: version, tour_state: state }),
        });
        return response.ok;
    } catch {
        return false;
    }
}
