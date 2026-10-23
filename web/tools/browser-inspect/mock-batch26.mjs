// Batch 26 (2026-10-23): the update screen of the alpha.103 candidate. Answers
// only while `state.b26` is set: the Panel's stored update check
// (GET /api/v1/panel/update/discovery), the check made when the update screen
// opens (GET /api/v1/panel/update/discovery/refresh, answered after
// `b26.refreshDelay` ms and then with `b26.after` as the stored result), the
// running build (v0.1.0-alpha.100), the readiness read (`b26.readiness`:
// 'ready' or a busy reason code) and one stopped update request's status (the
// updater's package-manager-busy summary). The releases it "finds"
// (v0.1.0-alpha.101 and v0.1.0-alpha.102, made-up commits and SHA-256) are
// never downloaded or verified; nothing is started. Every answer has the exact
// closed shape web/src/lib/panelUpdateDiscovery.ts decodes.
//
//   b26.stored        { target: 'a101' | 'a102' | null, checkedAge, foundAge, autoCheck }
//   b26.after         the stored result after the opening check (same shape), or null
//   b26.refresh       the opening check's outcome ('checked' | 'fresh' | 'skipped' | 'unknown')
//   b26.refreshDelay  ms before the opening check answers
//   b26.readiness     'ready' | 'package_manager_active' ...
//
// Yirmi altinci grup: alpha.103 guncelleme ekrani; hepsi sahte, hicbir sey baslatilmaz.
export const B26_CURRENT = { version: 'v0.1.0-alpha.100', commit: '3eeff24561799fb028c3c14f5da55a5de5bba866' };
export const B26_TARGETS = {
    a101: {
        version: 'v0.1.0-alpha.101', commit: '1a1b1c1d1e1f20212223242526272829303132a1', sequence: '101', os: 'linux', arch: 'amd64',
        archive_sha256: '101a'.padEnd(64, '0'), archive_size: '66301440', published_at: '2026-10-22T23:13:34Z',
    },
    a102: {
        version: 'v0.1.0-alpha.102', commit: '085cc400131072f3bcda81b560bfcf98dad47f03', sequence: '102', os: 'linux', arch: 'amd64',
        archive_sha256: '102b'.padEnd(64, '0'), archive_size: '66350990', published_at: '2026-10-23T09:08:45Z',
    },
};
export const B26_STOPPED_ID = 'b0570a1c2d3e4f5061728394a5b6c7d8';
export const B26_BUSY_SUMMARY = 'reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=package_manager_busy state=unchanged reason=the host package manager is active detail=';
const HOUR = 3600;
const iso = ageSeconds => new Date(Date.now() - ageSeconds * 1000).toISOString().replace(/\.\d+Z$/, 'Z');

export function batch26State(options = {}) {
    return {
        stored: { target: 'a101', checkedAge: 13 * HOUR, foundAge: 13 * HOUR, autoCheck: true },
        after: null, refresh: 'fresh', refreshDelay: 0, readiness: 'ready',
        ...options,
    };
}

function discovery(stored) {
    const target = stored.target ? B26_TARGETS[stored.target] : null;
    return {
        auto_check: stored.autoCheck, interval_hours: 12, expected_restart_seconds: 90,
        current_version: B26_CURRENT.version, current_commit: B26_CURRENT.commit,
        next_check_in_seconds: stored.autoCheck ? 11 * HOUR : 0,
        checked_age_seconds: stored.checkedAge,
        ...(target ? { found_age_seconds: stored.foundAge } : {}),
        available: !!target,
        last: {
            checked_at: iso(stored.checkedAge), outcome: target ? 'available' : 'current', automatic: true,
            ...(target ? { target, found_at: iso(stored.foundAge) } : {}),
            current_version: B26_CURRENT.version, current_commit: B26_CURRENT.commit,
        },
    };
}

const pause = ms => new Promise(resolve => setTimeout(resolve, ms));

export async function batch26(req, res, path, query, { state, send }) {
    const b26 = state.b26;
    if (!b26) return false;
    const key = `${req.method} ${path}`;
    if (key === 'GET /api/v1/panel/update/discovery') { send(res, 200, discovery(b26.stored)); return true; }
    if (key === 'GET /api/v1/panel/update/discovery/refresh') {
        if (b26.refreshDelay) await pause(b26.refreshDelay);
        if (b26.after) b26.stored = b26.after;
        send(res, 200, { ...discovery(b26.stored), refresh: { outcome: b26.refresh } });
        return true;
    }
    if (key === 'GET /api/v1/panel/version') {
        send(res, 200, { version: B26_CURRENT.version, commit: B26_CURRENT.commit, agent_commit: B26_CURRENT.commit, agent_matches: true, hostname: 'server1', ipv4: '203.0.113.10' });
        return true;
    }
    if (key === 'GET /api/v1/host-mutation-readiness') {
        send(res, 200, b26.readiness === 'ready' ? { ready: true } : { ready: false, code: 'HOST_MUTATION_BUSY', reason: b26.readiness });
        return true;
    }
    if (key === 'GET /api/v1/panel/update/status') {
        const id = query.get('request_id') || '';
        const target = B26_TARGETS.a102;
        send(res, 200, id === B26_STOPPED_ID
            ? { found: true, request_id: id, status: 'failed', target, created_at: iso(240), updated_at: iso(122), summary: B26_BUSY_SUMMARY }
            : { found: false, request_id: id });
        return true;
    }
    return false;
}
