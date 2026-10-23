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
//
// alpha.104: a PHP tool (Roundcube, phpMyAdmin, phpPgAdmin) whose PHP version
// was not there at the last scan is "not ready", never plainly installed: it
// cannot answer (Frankfurt's phpPgAdmin after PHP 8.4 was removed).
// Bağlı olduğu PHP sürümü olmayan PHP aracı "hazır değil" der.
export type ComponentStateWord = 'notChecked' | 'notInstalled' | 'installed' | 'running' | 'stopped' | 'notReady';

export interface PHPBindingFacts { version: string; socket_ready: boolean; version_installed: boolean; extensions_present: boolean }

export function phpBindingOf(service: unknown): PHPBindingFacts | null {
    if (!service || typeof service !== 'object') return null;
    const binding = (service as { php_binding?: unknown }).php_binding;
    if (!binding || typeof binding !== 'object') return null;
    const b = binding as Record<string, unknown>;
    if (typeof b.version !== 'string' || !/^[0-9]{1,2}\.[0-9]{1,2}$/.test(b.version) || typeof b.socket_ready !== 'boolean') return null;
    return { version: b.version, socket_ready: b.socket_ready, version_installed: b.version_installed === true, extensions_present: b.extensions_present !== false };
}

// Why a PHP tool is not ready, said under its name (alpha.104): its PHP is
// gone, stopped, or lacks the tool's PHP extension packages (package
// presence is what is read, not whether PHP loads them).
// Bir PHP aracının neden hazır olmadığı.
export function phpNotReadyHintKey(binding: PHPBindingFacts): 'services.phpNotReadyStopped' | 'services.phpNotReadyGone' | 'services.phpNotReadyExtensions' {
    if (!binding.socket_ready) return binding.version_installed ? 'services.phpNotReadyStopped' : 'services.phpNotReadyGone';
    return 'services.phpNotReadyExtensions';
}

export function componentStateWord(service: { is_installed: boolean | null; kind?: string; status?: unknown; php_binding?: unknown }): ComponentStateWord {
    if (service.is_installed === null) return 'notChecked';
    if (!service.is_installed) return 'notInstalled';
    const binding = phpBindingOf(service);
    if (binding && (!binding.socket_ready || !binding.extensions_present)) return 'notReady';
    if (service.kind === 'tool' || service.status === 'installed') return 'installed';
    return isServiceRunning(service.status) ? 'running' : 'stopped';
}

export const COMPONENT_STATE_KEYS: Record<ComponentStateWord, string> = {
    notChecked: 'services.notChecked',
    notInstalled: 'services.notInstalled',
    installed: 'services.installedLabel',
    running: 'services.running',
    stopped: 'services.stopped',
    notReady: 'services.phpNotReady',
};
