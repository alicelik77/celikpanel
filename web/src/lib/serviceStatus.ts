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
