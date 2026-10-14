// Loopback-only mock routes for the twelfth batch (2026-10-14): the waiting
// states of the alpha.85 branch. Loaded by mock.mjs before its own routes,
// because the owner's explicit setup check (GET /api/v1/setup?check=1&manual=1)
// is the same address as the plain snapshot read. Like mock.mjs, this answers
// constants and contacts nothing.
//
// Everything lives in `state.b12`; the routes answer only while a scenario of
// this batch has set it, and each scenario clears it at its end.
//   manualChecks  the checks the owner's explicit check returns (all ready:
//                 the page then shows "Finishing setup")
//   scan          what POST /api/v1/managed-services/scan answers:
//                 'stored' (the managed scan as it is), 'stale' (the managed
//                 scan with an old stamp), 'fail' (502 AGENT_UNREACHABLE),
//                 'held' (409 server_setup_busy)
//   scanDelay     ms before that answer (the checking state)
//
// On ikinci grup: alpha.85 dalindaki bekleme durumlari.
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

export async function batch12(req, res, path, query, { state, send, coded, snapshot }) {
    const b12 = state.b12;
    if (!b12) return false;
    const key = `${req.method} ${path}`;
    if (key === 'GET /api/v1/setup' && query.get('manual') === '1' && b12.manualChecks) {
        send(res, 200, { ...snapshot(), checks: b12.manualChecks });
        return true;
    }
    if (key === 'POST /api/v1/managed-services/scan' && b12.scan) {
        if (b12.scanDelay) await pause(b12.scanDelay);
        if (b12.scan === 'fail') { coded(res, 502, 'AGENT_UNREACHABLE', 'the CelikPanel Agent is not reachable'); return true; }
        if (b12.scan === 'held') { coded(res, 409, 'server_setup_busy', 'server setup is running'); return true; }
        const scan = state.managedScan || {};
        if (b12.scan === 'stale') {
            send(res, 200, { ...scan, scanned_at: new Date(Date.now() - 20 * 60_000).toISOString(), scan_age_seconds: 1200 });
            return true;
        }
        send(res, 200, { ...scan, scanned_at: new Date().toISOString(), scan_age_seconds: 0 });
        return true;
    }
    return false;
}
