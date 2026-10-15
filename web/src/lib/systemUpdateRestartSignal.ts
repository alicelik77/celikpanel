import { useSyncExternalStore } from 'react';
import { SYSTEM_UPDATE_EXPECTED_RESTART_MS, systemUpdateRestartDeadline } from './systemUpdateTracking';

// The update this browser follows, as the licence gate needs to know it
// (native record set17, defect D1, 2026-10-15). During a real update's restart
// the Panel does not answer; once the outage outlasted the access validity of
// the last good access read (42 to 60 s in the lab, 3 minutes or more on the
// owner's server), the gate drew its own "The Panel is not answering; an update
// may be restarting it" dialog over the update's calm restart and, through the
// observation recovery it raised, paused the update tracking, which released
// the update's navigation lock for good.
//
// The provider publishes the followed update once the server has said it is
// active (start accepted, or a read answered queued or running), with the first
// lost read of the current outage. The gate combines that with its own lost
// reads. Every moment here is this browser's clock; the server's clock is never
// compared with it (D-032).
//
// Izlenen guncelleme, lisans kapisinin bilmesi gerektigi bicimiyle (set17 D1).
// Saglayici, sunucu guncellemenin etkin oldugunu soyledikten sonra bunu ve
// kesintinin ilk kayip okumasini yayimlar; kapi kendi kayip okumalariyla
// birlestirir. Butun anlar bu tarayicinin saatidir; sunucu saatiyle
// karsilastirilmaz.
export type SystemUpdateRestartSignal = {
    requestID: string;
    // When the update started (the marker's created_at, this browser's clock).
    startedAt: number;
    // The update tracking's first lost read of the current outage; null while its reads answer.
    lostSince: number | null;
};

let current: SystemUpdateRestartSignal | null = null;
const listeners = new Set<() => void>();

export function publishSystemUpdateRestart(signal: SystemUpdateRestartSignal | null): void {
    const same = signal === null ? current === null
        : current !== null && current.requestID === signal.requestID
            && current.startedAt === signal.startedAt && current.lostSince === signal.lostSince;
    if (same) return;
    current = signal === null ? null : { ...signal };
    for (const listener of [...listeners]) listener();
}

export function currentSystemUpdateRestart(): SystemUpdateRestartSignal | null {
    return current;
}

export function subscribeSystemUpdateRestart(listener: () => void): () => void {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
}

export function useSystemUpdateRestart(): SystemUpdateRestartSignal | null {
    return useSyncExternalStore(subscribeSystemUpdateRestart, currentSystemUpdateRestart, currentSystemUpdateRestart);
}

// Until when a positive access decision is kept while the followed update
// restarts the Panel, or null when the restart does not apply.
//
// It applies only with evidence of the restart: a lost read of the update
// tracking or of the gate itself (gateLostSince) during an update the server
// said is active. It ends with the update's own restart window, the same bound
// the calm-restart notice uses (systemUpdateRestartDeadline: 6 minutes from the
// start, at least 2 minutes from the first lost read), and never later than the
// decision's own end (decisionUntilMs: the moment the access was asked plus the
// server's valid_for_seconds) plus that 6-minute window. After it, the gate's
// honest dialog may appear.
//
// Olumlu erisim karari, izlenen guncelleme Paneli yeniden baslatirken ne
// zamana kadar korunur; uygulanmiyorsa null. Yalnizca yeniden baslamanin
// kaniti varken (guncelleme izlemesinin ya da kapinin kayip okumasi) gecerlidir;
// sakin bildirimle ayni pencereyle biter ve kararin kendi sonu arti 6 dakikayi
// asmaz.
export function systemUpdateAccessGraceEnd(
    signal: SystemUpdateRestartSignal | null,
    decisionUntilMs: number,
    gateLostSince: number | null,
): number | null {
    if (!signal || !Number.isFinite(decisionUntilMs) || decisionUntilMs <= 0) return null;
    const lost = [signal.lostSince, gateLostSince].filter((value): value is number => typeof value === 'number' && Number.isFinite(value));
    if (lost.length === 0) return null;
    const restartEnd = systemUpdateRestartDeadline(signal.startedAt, Math.min(...lost));
    return Math.min(restartEnd, decisionUntilMs + SYSTEM_UPDATE_EXPECTED_RESTART_MS);
}
