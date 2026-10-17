// Batch 21 (2026-10-17): the Panel's stored update check of alpha.96 (D-035).
// Answers only while `state.b21` is set. It holds what the Panel stores after
// its own scheduled release read (GET /api/v1/panel/update/discovery) and the
// owner's switch (PUT, the same address), in the exact closed shape of
// web/src/lib/panelUpdateDiscovery.ts `decodePanelUpdateDiscovery`:
//
//   auto_check, interval_hours, expected_restart_seconds, current_version,
//   current_commit, next_check_in_seconds, checked_age_seconds,
//   found_age_seconds, available, last{checked_at, outcome, automatic,
//   reason_code?, target?, found_at?, current_version, current_commit},
//   previous_attempt?
//
// The build the mock "runs" is v0.1.0-alpha.95 (commit 5f3cb1a2...); the found
// release is v0.1.0-alpha.96 (made-up commit and SHA-256: nothing is verified
// or downloaded anywhere). It also answers /panel/version with that build (the
// base mock still says alpha.81) and the readiness read (`readiness`: 'ready' |
// a busy reason code). The manual "Check for updates" press is answered with
// the same stored target, so a press stays on this page's loopback mock.
//
//   b21.autoCheck  true | false
//   b21.outcome    'available' | 'current' | 'failed' | 'skipped'
//   b21.reason     reason_code of a failed or skipped check
//   b21.automatic  true | false
//   b21.target     true: the found release is part of the record
//   b21.previous   a previous_attempt object, or null
//   b21.readiness  'ready' | 'package_manager_busy' ...
//
// On yirmi birinci grup: alpha.96 saklanan guncelleme kontrolu; hepsi sahte.
export const B21_CURRENT = { version: 'v0.1.0-alpha.95', commit: '5f3cb1a2fa336c529740a20e9a543b8ac086dc8d' };
export const B21_TARGET = {
    version: 'v0.1.0-alpha.96',
    commit: '3c9d1e7a40b25f68a1d7c0e94b3f2a85d6e1b704',
    sequence: '96', os: 'linux', arch: 'amd64',
    archive_sha256: '7f3a9c1d5e2b84f60a1c3d9e7b52f4a86c0d1e3f5a7b9c2d4e6f8a0b1c3d5e7f',
    archive_size: '66259462',
    published_at: '2026-10-17T07:40:00Z',
};
const HOUR = 3600;

export function batch21State(options = {}) {
    return {
        autoCheck: true, outcome: 'current', reason: null, automatic: true, target: false, previous: null, readiness: 'ready',
        checkedAge: 2 * HOUR, foundAge: 2 * HOUR, nextIn: 10 * HOUR,
        ...options,
    };
}

function discovery(b21) {
    const checkedAt = new Date(Date.now() - b21.checkedAge * 1000).toISOString().replace(/\.\d+Z$/, 'Z');
    const foundAt = new Date(Date.now() - b21.foundAge * 1000).toISOString().replace(/\.\d+Z$/, 'Z');
    const last = {
        checked_at: checkedAt, outcome: b21.outcome, automatic: b21.automatic,
        ...(b21.reason ? { reason_code: b21.reason } : {}),
        ...(b21.target ? { target: B21_TARGET, found_at: foundAt } : {}),
        current_version: B21_CURRENT.version, current_commit: B21_CURRENT.commit,
    };
    return {
        auto_check: b21.autoCheck, interval_hours: 12, expected_restart_seconds: 90,
        current_version: B21_CURRENT.version, current_commit: B21_CURRENT.commit,
        next_check_in_seconds: b21.autoCheck ? b21.nextIn : 0,
        checked_age_seconds: b21.checkedAge,
        ...(b21.target ? { found_age_seconds: b21.foundAge } : {}),
        available: !!b21.target,
        last,
        ...(b21.target && b21.previous ? { previous_attempt: b21.previous } : {}),
    };
}

export async function batch21(req, res, path, query, { state, send, readBody }) {
    const b21 = state.b21;
    if (!b21) return false;
    const key = `${req.method} ${path}`;
    if (key === 'GET /api/v1/panel/update/discovery') { send(res, 200, discovery(b21)); return true; }
    if (key === 'PUT /api/v1/panel/update/discovery') {
        const body = await readBody(req);
        if (typeof body.auto_check !== 'boolean') { send(res, 400, { error: 'auto_check must be true or false', code: 'invalid_request' }); return true; }
        b21.autoCheck = body.auto_check;
        send(res, 200, discovery(b21));
        return true;
    }
    if (key === 'GET /api/v1/panel/version') {
        send(res, 200, { version: B21_CURRENT.version, commit: B21_CURRENT.commit, agent_commit: B21_CURRENT.commit, agent_matches: true, hostname: 'server1', ipv4: '203.0.113.10' });
        return true;
    }
    if (key === 'GET /api/v1/panel/update/check') {
        send(res, 200, b21.target
            ? { supported: true, available: true, current_version: B21_CURRENT.version, current_commit: B21_CURRENT.commit, target: B21_TARGET, ...(b21.previous ? { previous_attempt: b21.previous } : {}) }
            : { supported: true, available: false, current_version: B21_CURRENT.version, current_commit: B21_CURRENT.commit });
        return true;
    }
    if (key === 'GET /api/v1/host-mutation-readiness') {
        send(res, 200, b21.readiness === 'ready' ? { ready: true } : { ready: false, code: 'HOST_MUTATION_BUSY', reason: b21.readiness });
        return true;
    }
    return false;
}
