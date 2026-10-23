import { createElement, Fragment, useEffect, useState, type ReactNode } from 'react';

// The Panel's stored update check (D-035, owner request 2026-10-17): the
// Panel looks for a newer signed release by itself every 12 hours and keeps
// the last result. This file reads that stored result for administrators
// (GET /api/v1/panel/update/discovery: no Agent call, no network read on the
// server) and shares one copy between the sidebar link, the Dashboard row,
// the Settings item and the update card. The update card's "Check for
// updates" button asks the release server, through /update/check; since
// alpha.103 opening the update card also asks, through the Panel, when
// automatic checks are on and the known result is older than ten minutes
// (GET /api/v1/panel/update/discovery/refresh, once per opening; the Panel
// decides again and runs one check at a time).
//
// Panelin sakladigi guncelleme kontrolu: kenar cubugu, Pano, Ayarlar ve
// guncelleme karti tek okumayi paylasir. Yalniz karttaki dugme surum
// sunucusuna sorar.

export const PANEL_UPDATE_DISCOVERY_URL = '/api/v1/panel/update/discovery';
export const PANEL_UPDATE_DISCOVERY_TIMEOUT_MS = 8000;
// A shared copy younger than this is used as it is when a screen mounts.
const FRESH_MS = 5 * 60 * 1000;
// A visible page reads the stored result again this often (reads only; the
// result changes at most every few hours).
const VISIBLE_REREAD_MS = 30 * 60 * 1000;

export const discoveryReasons = [
    'release_unverified', 'timeout', 'not_supported', 'identity_mismatch', 'invalid_answer', 'agent_unavailable',
    'update_active', 'server_busy', 'setup_running', 'state_unverified',
] as const;
export type DiscoveryReason = typeof discoveryReasons[number];

export type DiscoveredTarget = {
    version: string;
    commit: string;
    sequence: string;
    os: string;
    arch: string;
    archive_sha256: string;
    archive_size: string;
    published_at?: string;
};

export type PanelUpdateDiscovery = {
    autoCheck: boolean;
    intervalHours: number;
    currentVersion: string;
    currentCommit: string;
    /** Browser time of the next automatic check, from the server's wait (D-032). */
    nextCheckAt: number | null;
    last: null | {
        outcome: 'available' | 'current' | 'failed';
        automatic: boolean;
        reason: DiscoveryReason | null;
        /** Browser time of the check: the moment of receipt minus the server's age. */
        checkedAt: number;
    };
    /** The newer release the panel found, kept across a failed or skipped check. */
    target: DiscoveredTarget | null;
    foundAt: number | null;
    /** The update check's previous_attempt, decoded by the update card. */
    previousAttempt: unknown;
    /**
     * The last scheduled attempt, when it was skipped after the result above
     * (the Panel keeps a skip in memory only; it never replaces a result).
     */
    lastSkip: null | { reason: DiscoveryReason | null; at: number };
    expectedRestartSeconds: number;
};

const versionPattern = /^v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
const commitPattern = /^[a-f0-9]{40}$/;
const shaPattern = /^[a-f0-9]{64}$/;
const decimalPattern = /^[1-9]\d{0,18}$/;

function seconds(value: unknown): number | null {
    return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null;
}

export function decodeDiscoveredTarget(value: unknown): DiscoveredTarget | null {
    if (!value || typeof value !== 'object') return null;
    const raw = value as Record<string, unknown>;
    if (typeof raw.version !== 'string' || raw.version.length > 80 || !versionPattern.test(raw.version)
        || typeof raw.commit !== 'string' || !commitPattern.test(raw.commit)
        || typeof raw.sequence !== 'string' || !decimalPattern.test(raw.sequence)
        || raw.os !== 'linux' || (raw.arch !== 'amd64' && raw.arch !== 'arm64')
        || typeof raw.archive_sha256 !== 'string' || !shaPattern.test(raw.archive_sha256)
        || typeof raw.archive_size !== 'string' || !decimalPattern.test(raw.archive_size)
        || (raw.published_at !== undefined && typeof raw.published_at !== 'string')) return null;
    return {
        version: raw.version, commit: raw.commit, sequence: raw.sequence, os: raw.os, arch: raw.arch,
        archive_sha256: raw.archive_sha256, archive_size: raw.archive_size,
        ...(typeof raw.published_at === 'string' ? { published_at: raw.published_at } : {}),
    };
}

/** Decodes the server's answer; null when it is not the closed shape. */
export function decodePanelUpdateDiscovery(payload: unknown, receivedAt: number): PanelUpdateDiscovery | null {
    if (!payload || typeof payload !== 'object') return null;
    const raw = payload as Record<string, unknown>;
    if (typeof raw.auto_check !== 'boolean' || typeof raw.current_version !== 'string' || raw.current_version.length > 80
        || typeof raw.current_commit !== 'string' || raw.current_commit.length > 80) return null;
    const next = seconds(raw.next_check_in_seconds);
    let last: PanelUpdateDiscovery['last'] = null;
    let target: DiscoveredTarget | null = null;
    let foundAt: number | null = null;
    if (raw.last !== undefined && raw.last !== null) {
        if (typeof raw.last !== 'object') return null;
        const record = raw.last as Record<string, unknown>;
        const age = seconds(raw.checked_age_seconds);
        if (age === null || (record.outcome !== 'available' && record.outcome !== 'current'
            && record.outcome !== 'failed')
            || typeof record.automatic !== 'boolean') return null;
        const reason = (discoveryReasons as readonly unknown[]).includes(record.reason_code)
            ? record.reason_code as DiscoveryReason : null;
        last = { outcome: record.outcome, automatic: record.automatic, reason, checkedAt: receivedAt - age * 1000 };
        if (record.target !== undefined) {
            target = decodeDiscoveredTarget(record.target);
            const foundAge = seconds(raw.found_age_seconds);
            if (!target || foundAge === null || raw.available !== true) return null;
            foundAt = receivedAt - foundAge * 1000;
        }
    }
    let lastSkip: PanelUpdateDiscovery['lastSkip'] = null;
    if (raw.last_skip !== undefined && raw.last_skip !== null) {
        if (typeof raw.last_skip !== 'object') return null;
        const skip = raw.last_skip as Record<string, unknown>;
        const age = seconds(skip.age_seconds);
        if (age === null) return null;
        const reason = (discoveryReasons as readonly unknown[]).includes(skip.reason_code) ? skip.reason_code as DiscoveryReason : null;
        lastSkip = { reason, at: receivedAt - age * 1000 };
    }
    const interval = typeof raw.interval_hours === 'number' && Number.isInteger(raw.interval_hours) && raw.interval_hours > 0
        ? raw.interval_hours : 12;
    const restart = typeof raw.expected_restart_seconds === 'number' && Number.isInteger(raw.expected_restart_seconds)
        && raw.expected_restart_seconds > 0 && raw.expected_restart_seconds <= 3600 ? raw.expected_restart_seconds : 90;
    return {
        autoCheck: raw.auto_check,
        intervalHours: interval,
        currentVersion: raw.current_version,
        currentCommit: raw.current_commit,
        nextCheckAt: raw.auto_check && next !== null ? receivedAt + next * 1000 : null,
        last,
        target,
        foundAt,
        previousAttempt: target ? raw.previous_attempt : undefined,
        expectedRestartSeconds: restart,
        lastSkip,
    };
}

export type DiscoveryState = {
    status: 'idle' | 'loading' | 'ready' | 'failed';
    value: PanelUpdateDiscovery | null;
    readAt: number;
};

let state: DiscoveryState = { status: 'idle', value: null, readAt: 0 };
let inFlight: Promise<void> | null = null;
let refreshInFlight: Promise<DiscoveryRefresh> | null = null;
const listeners = new Set<(next: DiscoveryState) => void>();

function publish(next: DiscoveryState) {
    state = next;
    for (const listener of listeners) listener(state);
}

export function panelUpdateDiscoveryState(): DiscoveryState {
    return state;
}

/** Shares an answer read elsewhere (the card's switch returns one). */
export function publishPanelUpdateDiscovery(value: PanelUpdateDiscovery) {
    publish({ status: 'ready', value, readAt: Date.now() });
}

/** Tests only: forget the shared copy. */
export function resetPanelUpdateDiscoveryForTests() {
    state = { status: 'idle', value: null, readAt: 0 };
    inFlight = null;
    refreshInFlight = null;
    listeners.clear();
}

/**
 * Reads the stored result once; a read already on its way is that read. A
 * failed read keeps an earlier answer (D-032 read banner rule: a failed read
 * is said where it matters, the update card; nothing else changes).
 */
export function refreshPanelUpdateDiscovery(): Promise<void> {
    if (inFlight) return inFlight;
    publish({ ...state, status: state.value ? state.status : 'loading' });
    const controller = new AbortController();
    const timer = globalThis.setTimeout(() => controller.abort(), PANEL_UPDATE_DISCOVERY_TIMEOUT_MS);
    inFlight = (async () => {
        try {
            const response = await fetch(PANEL_UPDATE_DISCOVERY_URL, {
                cache: 'no-store', credentials: 'same-origin', signal: controller.signal,
            });
            if (!response.ok) throw new Error(String(response.status));
            const value = decodePanelUpdateDiscovery(await response.json(), Date.now());
            if (!value) throw new Error('invalid');
            publish({ status: 'ready', value, readAt: Date.now() });
        } catch {
            publish({ status: state.value ? 'ready' : 'failed', value: state.value, readAt: state.readAt });
        } finally {
            globalThis.clearTimeout(timer);
            inFlight = null;
        }
    })();
    return inFlight;
}

// Opening the update screen (alpha.103, owner observation 2026-10-23:
// Frankfurt's screen offered alpha.101 from the 12-hour schedule while
// alpha.102 was published, until "Check for updates" was pressed).
// Guncelleme ekrani acilinca: otomatik denetim aciksa ve bilinen sonuc on
// dakikadan eskiyse Panel bir kez daha bakar; kapaliyken hicbir yere sorulmaz.
export const PANEL_UPDATE_DISCOVERY_REFRESH_URL = '/api/v1/panel/update/discovery/refresh';
/** The Panel bounds the whole opening check at 50 seconds; this is that and the way back. */
export const PANEL_UPDATE_DISCOVERY_REFRESH_TIMEOUT_MS = 55_000;
/** A known result younger than this is not checked again on opening (the Panel's window). */
export const PANEL_UPDATE_OPEN_FRESH_MS = 10 * 60 * 1000;

export const discoveryRefreshOutcomes = ['checked', 'fresh', 'off', 'skipped', 'unknown'] as const;
/**
 * What opening the update screen did: checked (the result is the stored one
 * now), fresh (no read: the known result is recent), off (automatic checks
 * are off: nothing was asked), skipped (a condition was not met; reason),
 * unknown (no answer in time, or the answer could not be read).
 */
export type DiscoveryRefresh = { outcome: typeof discoveryRefreshOutcomes[number]; reason: DiscoveryReason | null };

/** Whether opening the update screen asks the Panel to check again. */
export function panelUpdateOpenCheckDue(value: PanelUpdateDiscovery | null, now: number): boolean {
    return !!value && value.autoCheck && (!value.last || now - value.last.checkedAt >= PANEL_UPDATE_OPEN_FRESH_MS);
}

/**
 * The check made when the update screen opens. The answer is the stored
 * result after it, shared with the sidebar, the Dashboard row and the
 * Settings item. One request at a time; it never starts anything. A failed
 * read keeps the shared copy and says the result is not known.
 */
export function refreshPanelUpdateDiscoveryOnOpen(): Promise<DiscoveryRefresh> {
    if (refreshInFlight) return refreshInFlight;
    const controller = new AbortController();
    const timer = globalThis.setTimeout(() => controller.abort(), PANEL_UPDATE_DISCOVERY_REFRESH_TIMEOUT_MS);
    refreshInFlight = (async (): Promise<DiscoveryRefresh> => {
        try {
            const response = await fetch(PANEL_UPDATE_DISCOVERY_REFRESH_URL, {
                cache: 'no-store', credentials: 'same-origin', signal: controller.signal,
            });
            if (!response.ok) throw new Error(String(response.status));
            const body = await response.json() as Record<string, unknown> | null;
            const value = decodePanelUpdateDiscovery(body, Date.now());
            if (!value || !body) throw new Error('invalid');
            const raw = body.refresh as Record<string, unknown> | undefined;
            const outcome = raw && (discoveryRefreshOutcomes as readonly unknown[]).includes(raw.outcome)
                ? raw.outcome as DiscoveryRefresh['outcome'] : 'unknown';
            const reason = raw && (discoveryReasons as readonly unknown[]).includes(raw.reason_code)
                ? raw.reason_code as DiscoveryReason : null;
            publish({ status: 'ready', value, readAt: Date.now() });
            return { outcome, reason };
        } catch {
            return { outcome: 'unknown', reason: null };
        } finally {
            globalThis.clearTimeout(timer);
            refreshInFlight = null;
        }
    })();
    return refreshInFlight;
}

function visible(): boolean {
    return typeof document === 'undefined' || document.visibilityState === 'visible';
}

/**
 * The shared stored result for an administrator; nothing is read otherwise.
 * A screen that mounts reads it when the shared copy is older than five
 * minutes; a visible page reads it again every 30 minutes and when the tab
 * is shown after that long. Reads only.
 */
export function usePanelUpdateDiscovery(enabled: boolean): DiscoveryState {
    const [current, setCurrent] = useState<DiscoveryState>(state);
    useEffect(() => {
        if (!enabled) return undefined;
        listeners.add(setCurrent);
        setCurrent(state);
        if (state.status !== 'loading' && Date.now() - state.readAt > FRESH_MS) void refreshPanelUpdateDiscovery();
        const stale = () => Date.now() - state.readAt > VISIBLE_REREAD_MS;
        const onVisibility = () => {
            if (visible() && stale()) void refreshPanelUpdateDiscovery();
        };
        const interval = globalThis.setInterval(() => {
            if (visible() && stale()) void refreshPanelUpdateDiscovery();
        }, VISIBLE_REREAD_MS);
        if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibility);
        return () => {
            listeners.delete(setCurrent);
            globalThis.clearInterval(interval);
            if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility);
        };
    }, [enabled]);
    return enabled ? current : { status: 'idle', value: null, readAt: 0 };
}

/** The release the owner can update to, or null. */
export function availablePanelUpdate(value: PanelUpdateDiscovery | null): DiscoveredTarget | null {
    return value?.target ?? null;
}

// "Remind me later" on the Dashboard: hidden for 24 hours in this browser,
// for the version it was pressed for. Storage may be blocked; the row then
// simply shows again (the sidebar link always stays).
// "Sonra hatırlat": bu tarayıcıda o sürüm için 24 saat gizlenir.
export const UPDATE_NOTICE_SNOOZE_KEY = 'celikpanel.update-notice.snoozed.v1';
export const UPDATE_NOTICE_SNOOZE_MS = 24 * 60 * 60 * 1000;

export function updateNoticeSnoozed(version: string, now: number): boolean {
    try {
        const raw = globalThis.localStorage?.getItem(UPDATE_NOTICE_SNOOZE_KEY);
        if (!raw) return false;
        const value = JSON.parse(raw) as { version?: unknown; until?: unknown };
        return value.version === version && typeof value.until === 'number' && value.until > now
            && value.until - now <= UPDATE_NOTICE_SNOOZE_MS;
    } catch {
        return false;
    }
}

export function snoozeUpdateNotice(version: string, now: number): void {
    try {
        globalThis.localStorage?.setItem(UPDATE_NOTICE_SNOOZE_KEY, JSON.stringify({ version, until: now + UPDATE_NOTICE_SNOOZE_MS }));
    } catch {
        // Blocked storage: the row is hidden for this page only.
    }
}

/** "2 hours ago" in the interface language; under a minute is "just now". */
export function relativeAge(moment: number, now: number, locale: string, justNow: string): string {
    const elapsed = Math.max(0, now - moment);
    if (elapsed < 60_000) return justNow;
    const format = new Intl.RelativeTimeFormat(locale === 'tr' ? 'tr-TR' : 'en-US', { numeric: 'auto' });
    const minutes = Math.floor(elapsed / 60_000);
    if (minutes < 60) return format.format(-minutes, 'minute');
    const hours = Math.floor(minutes / 60);
    if (hours < 48) return format.format(-hours, 'hour');
    return format.format(-Math.floor(hours / 24), 'day');
}

/** The clock time of a moment, with the weekday when it is not today. */
export function clockTime(moment: number, now: number, locale: string): string {
    const tag = locale === 'tr' ? 'tr-TR' : 'en-US';
    const date = new Date(moment);
    const sameDay = date.toDateString() === new Date(now).toDateString();
    return date.toLocaleString(tag, sameDay
        ? { hour: '2-digit', minute: '2-digit' }
        : { weekday: 'short', hour: '2-digit', minute: '2-digit' });
}

/**
 * A sentence with the version kept whole: "v0.1.0-alpha.96" never breaks at
 * its hyphen on a phone (owner's rule: nothing breaks inside a word). The
 * text is unchanged; the line still wraps at spaces.
 * Sürüm tirede bölünmez; metin aynı kalır.
 */
export function wholeVersion(text: string, version: string): ReactNode {
    const at = version === '' ? -1 : text.indexOf(version);
    if (at < 0) return text;
    return createElement(Fragment, null,
        text.slice(0, at),
        createElement('span', { className: 'whitespace-nowrap', 'data-version': '' }, version),
        text.slice(at + version.length));
}

/**
 * The stored summary of one earlier update request, read once by the update
 * card when the recovery record kept no cause for it (alpha.97: Frankfurt's
 * stop of 2026-10-17, a busy package manager). A GET that never starts
 * anything. An unread answer is said as unread, never as "no summary".
 * Önceki bir güncelleme isteğinin saklı özeti; okunamayan yanıt okunamadı denir.
 */
export type PanelUpdateAttemptSummary = { read: true; summary: string | null } | { read: false };

export async function readPanelUpdateAttemptSummary(requestID: string, signal: AbortSignal): Promise<PanelUpdateAttemptSummary> {
    if (!/^[a-f0-9]{32}$/.test(requestID)) return { read: false };
    try {
        const response = await fetch(`/api/v1/panel/update/status?request_id=${requestID}`, {
            cache: 'no-store', credentials: 'same-origin', signal,
        });
        if (!response.ok) return { read: false };
        const body = await response.json() as { found?: unknown; request_id?: unknown; status?: unknown; summary?: unknown };
        const failed = body.found === true && body.request_id === requestID && body.status === 'failed' && typeof body.summary === 'string';
        return { read: true, summary: failed ? String(body.summary) : null };
    } catch {
        return { read: false };
    }
}
