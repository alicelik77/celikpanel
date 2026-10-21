// A stand-in for src/lib/panelUpdateDiscovery.ts in the mounted tests of the
// update card that are not about the stored update check (alpha.96). The
// card reads globalThis.__panelUpdateDiscoveryState when a test sets it, and
// nothing otherwise; no request is made.
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
export async function readPanelUpdateAttemptSummary(id) { const summary = globalThis.__panelUpdateAttemptSummary?.(id); return summary === undefined ? { read: false } : { read: true, summary }; }
`;
}
