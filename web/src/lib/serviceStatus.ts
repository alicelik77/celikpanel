// Whether a component (or one installed copy of it) is running, read from the
// stored scan's status. The scan writes systemd's words ("active (running)",
// "active (exited)", "inactive (dead)", "failed") or, for a few components,
// the Panel's own "running". Only a status that says running is running:
// "active (exited)" is a one-shot unit that has finished, not a daemon that
// serves. One rule for the Dashboard, Components, a component's page and
// Server → Security (v0.1.0-alpha.98 review; before it the Dashboard and a
// copy's row counted every "active …" as running).
//
// Bir bileşenin (ya da kurulu bir kopyasının) çalışıp çalışmadığı, kayıtlı
// taramanın durumundan. Yalnız "çalışıyor" diyen durum çalışıyordur: "active
// (exited)" bitmiş tek seferlik birimdir. Pano, Bileşenler, bileşen sayfası ve
// Güvenlik için tek kural.
export function isServiceRunning(status: unknown): boolean {
    if (typeof status !== 'string') return false;
    const normalized = status.trim().toLowerCase();
    return normalized === 'running' || normalized.includes('(running)');
}

// The word a component's row says for its state (alpha.101: the
// installed bundle's dialogue says the same word for each part, from the same
// data, so the two can never disagree). A tool, and a runtime whose copies
// have no units (Node.js), report "installed": for them "installed" is the
// whole truth; PHP-FPM has units and keeps running or stopped.
// Satırın bir bileşenin durumu için söylediği söz; kurulu paketin
// penceresi her parça için aynı sözü aynı veriden söyler.
export type ComponentStateWord = 'notChecked' | 'notInstalled' | 'installed' | 'running' | 'stopped';

export function componentStateWord(service: { is_installed: boolean | null; kind?: string; status?: unknown }): ComponentStateWord {
    if (service.is_installed === null) return 'notChecked';
    if (!service.is_installed) return 'notInstalled';
    if (service.kind === 'tool' || service.status === 'installed') return 'installed';
    return isServiceRunning(service.status) ? 'running' : 'stopped';
}

export const COMPONENT_STATE_KEYS: Record<ComponentStateWord, string> = {
    notChecked: 'services.notChecked',
    notInstalled: 'services.notInstalled',
    installed: 'services.installedLabel',
    running: 'services.running',
    stopped: 'services.stopped',
};
