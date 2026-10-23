// A stand-in for src/lib/panelUpdateDiscovery.ts in the mounted tests of the
// update card that are not about the stored update check (alpha.96). The
// card reads globalThis.__panelUpdateDiscoveryState when a test sets it, and
// nothing otherwise; no request is made (the opening check of alpha.103 too).
export function panelUpdateDiscoveryStubSource() {
    return `
export const PANEL_UPDATE_DISCOVERY_URL = '/api/v1/panel/update/discovery';
const idle = { status: 'idle', value: null, readAt: 0 };
export function usePanelUpdateDiscovery() { return globalThis.__panelUpdateDiscoveryState ?? idle; }
export async function refreshPanelUpdateDiscovery() { globalThis.__panelUpdateDiscoveryRefreshes = (globalThis.__panelUpdateDiscoveryRefreshes ?? 0) + 1; }
export function publishPanelUpdateDiscovery() {}
export function decodePanelUpdateDiscovery() { return null; }
export function availablePanelUpdate(value) { return value?.target ?? null; }
export function relativeAge(moment, now, _locale, justNow) { return now - moment < 60000 ? justNow : '2 hours ago'; }
export function clockTime() { return '15:30'; }
export function wholeVersion(text) { return text; }
// alpha.103: the check made when the update screen opens; not due in these
// tests unless one sets globalThis.__panelUpdateOpenCheckDue.
export function panelUpdateDiscoveryState() { return globalThis.__panelUpdateDiscoveryState ?? idle; }
export function panelUpdateOpenCheckDue() { return globalThis.__panelUpdateOpenCheckDue === true; }
export async function refreshPanelUpdateDiscoveryOnOpen() { globalThis.__panelUpdateOpenChecks = (globalThis.__panelUpdateOpenChecks ?? 0) + 1; return { outcome: 'fresh', reason: null }; }
export async function readPanelUpdateAttemptSummary(id) { const summary = globalThis.__panelUpdateAttemptSummary?.(id); return summary === undefined ? { read: false } : { read: true, summary }; }
`;
}
