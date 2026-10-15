import { useCallback, useEffect, useRef } from 'react';
import {
    currentSystemUpdateRestart,
    subscribeSystemUpdateRestart,
    systemUpdateSessionGraceEnd,
} from './systemUpdateRestartSignal';

// How often the session gate reads the Panel's readiness while it lets the
// followed update's restart own the page.
export const UPDATE_RESTART_SESSION_READ_MS = 5000;
const READ_LIMIT_MS = 10000;
const MAX_TIMER_MS = 2147483647;

type Deferral = { requestID: string; until: number; timer: number; interval: number };

// The session gate during the followed update's own restart (2026-10-15, the
// open item of set17 D1). While the new Panel starts, its management answers
// 503 PANEL_STARTING. Before this, the first such answer to any request marked
// the session unavailable: AccessHold held the pages with "The Panel is
// starting", App paused the update tracking (publishSystemUpdateAuthentication),
// and the update's navigation lease released its engaged lock for good.
//
// While the followed update restarts the Panel (its own restart window,
// lib/systemUpdateRestartSignal.ts), such an answer does not mark the session.
// The update's calm restart owns the page; the gate reads the Panel's readiness
// every 5 s while the tab is visible, once more when the tab is shown again, and
// once at once when the update tracking reaches the Panel again. A ready answer
// ends the wait. A 401 is still applied by the interception as before. When the
// window ends with no ready answer, the session is marked as before (the read,
// the hold and its explanation). Reads only; nothing is retried or started.
// Browser clock only (D-032).
//
// Guncellemenin kendi yeniden baslamasi surerken PANEL_STARTING yaniti oturumu
// kullanilamaz saymaz: sayfa guncellemenin sakin bildiriminde kalir, hazir olma
// durumu 5 sn'de bir okunur, pencere biterken hazir yaniti yoksa eski davranis
// uygulanir. Yalnizca okuma yapilir.
export function useUpdateRestartStarting(ready: boolean, markUnavailable: () => void): () => boolean {
    const readyRef = useRef(ready);
    readyRef.current = ready;
    const markRef = useRef(markUnavailable);
    markRef.current = markUnavailable;
    const deferral = useRef<Deferral | null>(null);
    // This document's first PANEL_STARTING answer for the followed update. Never
    // moved later: repeated answers cannot extend the bound.
    const first = useRef<{ requestID: string; at: number } | null>(null);
    const reading = useRef(false);

    const stop = useCallback(() => {
        const current = deferral.current;
        if (!current) return;
        deferral.current = null;
        window.clearTimeout(current.timer);
        window.clearInterval(current.interval);
    }, []);

    const read = useCallback(async () => {
        const owner = deferral.current;
        if (!owner || reading.current || document.visibilityState !== 'visible') return;
        reading.current = true;
        const request = new AbortController();
        const limit = window.setTimeout(() => request.abort(), READ_LIMIT_MS);
        // Only a ready answer ends the wait. "Still starting", any other answer
        // and no answer are the update's restart until its window ends; a 401 is
        // applied by the interception. Nothing is drawn from this read.
        let readiness: 'ready' | 'not_ready' | 'unanswered';
        try {
            const response = await fetch('/api/v1/panel/availability', { signal: request.signal, cache: 'no-store' });
            if (response.ok) {
                const result = await response.json();
                readiness = result?.schema === 'celikpanel-panel-availability/v1' && result.state === 'ready' ? 'ready' : 'not_ready';
            } else readiness = 'not_ready';
        } catch (error) {
            readiness = 'unanswered';
        } finally {
            window.clearTimeout(limit);
            reading.current = false;
        }
        if (readiness === 'ready' && deferral.current === owner) stop();
    }, [stop]);

    // Called by the interception for a PANEL_STARTING answer. True when the
    // followed update's restart owns it; false when the session is marked as before.
    const defer = useCallback((): boolean => {
        if (!readyRef.current) return false;
        const signal = currentSystemUpdateRestart();
        if (!signal) return false;
        const now = Date.now();
        if (first.current?.requestID !== signal.requestID) first.current = { requestID: signal.requestID, at: now };
        const until = systemUpdateSessionGraceEnd(signal, first.current.at);
        if (until === null || until <= now) return false;
        if (deferral.current?.requestID === signal.requestID) return true;
        stop();
        const own: Deferral = {
            requestID: signal.requestID,
            until,
            timer: window.setTimeout(() => {
                if (deferral.current !== own) return;
                stop();
                markRef.current();
            }, Math.min(MAX_TIMER_MS, until - now)),
            interval: window.setInterval(() => void read(), UPDATE_RESTART_SESSION_READ_MS),
        };
        deferral.current = own;
        return true;
    }, [read, stop]);

    // A session that is no longer ready is owned by its own reads again.
    useEffect(() => { if (!ready) stop(); }, [ready, stop]);

    useEffect(() => {
        const shown = () => { if (document.visibilityState === 'visible') void read(); };
        document.addEventListener('visibilitychange', shown);
        // The update tracking reached the Panel again: read its readiness at once.
        let lost = currentSystemUpdateRestart()?.lostSince ?? null;
        const unsubscribe = subscribeSystemUpdateRestart(() => {
            const now = currentSystemUpdateRestart()?.lostSince ?? null;
            if (lost !== null && now === null) void read();
            lost = now;
        });
        return () => {
            document.removeEventListener('visibilitychange', shown);
            unsubscribe();
            stop();
        };
    }, [read, stop]);

    return defer;
}
