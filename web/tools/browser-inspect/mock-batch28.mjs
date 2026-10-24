// Batch 28 (2026-10-24): the bundle dialogue of the alpha.105 candidate.
// Answers only while `state.b28` is set: the version the package manager
// would install for each component (GET /api/v1/service/candidate?id=), each
// component's managed repository (GET /api/v1/repo?service_id=) and the
// install simulation (GET /api/v1/service/install-preview), whose answer is
// `b28.preview` for every step list, or `b28.previewFor[<steps joined by
// "|">]` when that is set. SAMPLE answers shaped as the Panel answers (the
// versions are those of the alpha.105 real-system run); nothing is installed.
//
// Yirmi sekizinci grup: alpha.105 paket penceresi; hepsi sahte.
export async function batch28(req, res, path, query, { state, send }) {
    const b28 = state.b28;
    if (!b28 || req.method !== 'GET') return false;
    if (path === '/api/v1/service/candidate') {
        send(res, 200, { version: b28.candidates?.[query.get('id')] ?? '' });
        return true;
    }
    if (path === '/api/v1/repo') {
        send(res, 200, b28.repos?.[query.get('service_id')] ?? { available: false, enabled: false });
        return true;
    }
    if (path === '/api/v1/service/install-preview') {
        const key = query.getAll('step').join('|');
        const answer = b28.previewFor?.[key] ?? b28.preview;
        if (b28.previewDelay) await new Promise((resolve) => setTimeout(resolve, b28.previewDelay));
        send(res, 200, answer);
        return true;
    }
    return false;
}
