// What the update notice and the update card say about an update this browser
// is following, from the last read of its exact record (owner report
// 2026-10-10: the card showed the new version installed while the notice still
// said the update was being applied).
//
// The Agent's record stays "running" after the new Panel has started, until the
// updater has exited and its final proof has passed. The Panel's status answer
// says "verifying" in that window: the Panel answering is already the target
// build (cmd/panel panelUpdateStatusPhase). A read that failed is unknown; it
// is never said as "being applied".
//
// Bu tarayicinin izledigi guncelleme icin bildirim ve kartin soyledigi: son
// okumadan gelir. Yeni Panel basladiktan sonra kayit, guncelleyici bitip son
// kanit gecene kadar "running" kalir; Panel bu aralikta "verifying" der.
// Basarisiz okuma bilinmiyor demektir, "uygulaniyor" denmez.

export type SystemUpdateTrackingPhase = 'reading' | 'queued' | 'applying' | 'verifying' | 'restarting' | 'unknown';

export type SystemUpdateTracking = {
    version: string;
    phase: SystemUpdateTrackingPhase;
    // Additive (2026-10-15): the T+ counter and the exact request id, so the
    // update page can carry them when it is the one notice on screen.
    elapsed?: string;
    requestID?: string;
    // True when an unknown state follows a restart that outlasted its window.
    overdue?: boolean;
};

// The expected restart window (owner report 2026-10-15, D-033, D-024). An
// active update's own plan restarts the Panel and the Agent: the reviewed
// updater runs while the Agent's record is "running" (cmd/agent
// system_update_worker_linux.go, runInstaller), and the Panel's status answer
// knows no finer phase than queued, running and verifying. So a read that loses
// the connection after the server last answered queued or running (or accepted
// the start) is the planned restart, shown as calm progress, until the window
// ends. Only after it, or on a verified failure, does the notice say that the
// state is not known.
//
// The figures. set16 (deploy/e2e/release-recovery/evidence/set16-20261014,
// */upd84-good-a/driver/steps/12-owner-start and 13-track): a good update
// alpha.84 -> alpha.85 took 51 s (Debian 13, start 22:55:07Z, update_verified
// 22:55:58Z) and 48 s (Ubuntu 24.04, 23:33:10Z -> 23:33:58Z), with the Panel's
// API unanswered for at most 32 s (22:55:19Z -> 22:55:51Z) and 25 s
// (23:33:26Z -> 23:33:52Z); the lab served the archive from a local origin.
// The owner's real update (2026-10-15, Frankfurt, archive fetched over the
// internet) still had no answer from the Panel at T+02:59 (the toast captured
// then) and completed normally afterwards. The window is about twice the
// longest of these (2 x 3 min): 6 minutes from the update's start. A restart that
// begins late in a slow update still gets 2 minutes from the first lost read,
// about four times the longest measured outage (32 s). Not measured natively
// for this rule; a normal update in the lab never comes near either figure.
//
// Beklenen yeniden baslama penceresi: guncellemenin kendi plani Panel ve
// Agent'i yeniden baslatir; baglanti bu pencerede koparsa sakin ilerleme
// gosterilir, "bilinmiyor" ancak pencere asilinca ya da dogrulanmis hatada.
export const SYSTEM_UPDATE_EXPECTED_RESTART_MS = 6 * 60_000;
export const SYSTEM_UPDATE_RESTART_AFTER_LOSS_MS = 2 * 60_000;

// When the expected restart window of an update started at startedAt ends,
// given the first read of the current outage (lostSince). Both are this
// browser's clock; nothing is compared with the server's clock.
export function systemUpdateRestartDeadline(startedAt: number, lostSince: number): number {
    const fromStart = Number.isFinite(startedAt) && startedAt > 0 ? startedAt + SYSTEM_UPDATE_EXPECTED_RESTART_MS : 0;
    const fromLoss = Number.isFinite(lostSince) ? lostSince + SYSTEM_UPDATE_RESTART_AFTER_LOSS_MS : 0;
    return Math.max(fromStart, fromLoss);
}

// What this document has learned about the followed request: whether the
// server has said it is active (start accepted, or a read answered queued or
// running), and when the current outage began (null while reads succeed).
export type SystemUpdateReadMemory = { lastKnownActive: boolean; lostSince: number | null };
export const SYSTEM_UPDATE_READ_MEMORY: SystemUpdateReadMemory = { lastKnownActive: false, lostSince: null };

// One exact read of the followed update, as the notice shows it. A record that
// was read names its phase. A read that lost the connection (or met a 5xx
// while the Panel restarts) after the server last said this update is active
// is the update's own restart until its window ends. Any other failed read is
// unknown. A verified failure never comes here: it is a terminal outcome.
export function systemUpdateFollowRead(
    memory: SystemUpdateReadMemory,
    read: { operation?: { status?: string; phase?: string } | null; disconnected: boolean; at: number },
    startedAt: number,
): { phase: SystemUpdateTrackingPhase; restartDeadline: number | null; memory: SystemUpdateReadMemory } {
    if (read.operation) {
        const active = read.operation.status === 'queued' || read.operation.status === 'running';
        return {
            phase: systemUpdateOperationPhase(read.operation),
            restartDeadline: null,
            memory: { lastKnownActive: memory.lastKnownActive || active, lostSince: null },
        };
    }
    if (!read.disconnected) return { phase: 'unknown', restartDeadline: null, memory };
    const lostSince = memory.lostSince ?? read.at;
    const next = { lastKnownActive: memory.lastKnownActive, lostSince };
    if (!memory.lastKnownActive) return { phase: 'unknown', restartDeadline: null, memory: next };
    return { phase: 'restarting', restartDeadline: systemUpdateRestartDeadline(startedAt, lostSince), memory: next };
}

// One notice, not two (owner report 2026-10-15): while a page that shows the
// followed update itself is mounted (the update card, with its T+ counter
// and ID), the corner notice does not repeat it. A known outcome, which the
// card does not show, keeps the corner notice.
export function systemUpdateCornerNoticeRepeats(inlineNotices: number, followedTracking: boolean): boolean {
    return inlineNotices > 0 && followedTracking;
}

// What is shown now: a restart past its window becomes unknown, by itself,
// without waiting for the next read.
export function systemUpdateDisplayedPhase(phase: SystemUpdateTrackingPhase, restartDeadline: number | null, now: number): SystemUpdateTrackingPhase {
    if (phase !== 'restarting') return phase;
    if (restartDeadline === null || !Number.isFinite(restartDeadline) || !(now < restartDeadline)) return 'unknown';
    return 'restarting';
}

// The phase of a record that was read. A record that is not queued or running
// is terminal and handled by its own outcome, not by this phase.
export function systemUpdateOperationPhase(operation: { status?: string; phase?: string } | null | undefined): SystemUpdateTrackingPhase {
    if (!operation) return 'unknown';
    if (operation.status === 'queued') return 'queued';
    if (operation.status === 'running') return operation.phase === 'verifying' ? 'verifying' : 'applying';
    return 'unknown';
}

// The status answer's optional phase: only the one known value is kept.
export function decodeSystemUpdatePhase(value: unknown): 'verifying' | undefined {
    return value === 'verifying' ? 'verifying' : undefined;
}

// The notice's lines for a followed update. title null keeps the notice's own
// heading; message replaces the last read's sentence only where the phase says
// more than it; hint is why a read failed, said under the unknown line.
// Izlenen guncelleme icin bildirim satirlari.
// A restart inside its window is progress, said once, with no failed-read
// reason under it; after the window the unknown line names why it is unknown.
type Translate = (key: 'panelUpdate.tracking.verifyingTitle' | 'panelUpdate.tracking.verifying' | 'panelUpdate.tracking.unknown' | 'panelUpdate.tracking.restarting' | 'panelUpdate.tracking.restartOverdue', values?: Record<string, string>) => string;
export function systemUpdateTrackingText(tracking: SystemUpdateTracking, lastMessage: string, t: Translate): { title: string | null; message: string; hint: string | null } {
    if (tracking.phase === 'verifying') {
        return { title: t('panelUpdate.tracking.verifyingTitle'), message: t('panelUpdate.tracking.verifying', { version: tracking.version }), hint: null };
    }
    if (tracking.phase === 'restarting') return { title: null, message: t('panelUpdate.tracking.restarting'), hint: null };
    if (tracking.phase === 'unknown' && tracking.overdue) {
        return { title: null, message: t('panelUpdate.tracking.unknown'), hint: t('panelUpdate.tracking.restartOverdue') };
    }
    if (tracking.phase === 'unknown') return { title: null, message: t('panelUpdate.tracking.unknown'), hint: lastMessage || null };
    return { title: null, message: lastMessage, hint: null };
}

// A clock time on the update screens is the browser's local time, named with
// its zone, in the interface language (as the update card's other times).
// Guncelleme ekranlarindaki saat tarayicinin yerel saatidir ve dilimiyle yazilir.
export function systemUpdateClockTime(at: number, locale: string): string {
    const tag = locale === 'tr' ? 'tr-TR' : 'en-US';
    try {
        return new Date(at).toLocaleTimeString(tag, { timeZoneName: 'short' });
    } catch {
        return new Date(at).toLocaleTimeString(tag);
    }
}
