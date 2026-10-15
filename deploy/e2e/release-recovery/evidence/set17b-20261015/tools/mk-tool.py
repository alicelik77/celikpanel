import sys
src, dst = sys.argv[1], sys.argv[2]
t = open(src, encoding='utf-8').read()
def rep(old, new, count=1):
    global t
    n = t.count(old)
    if n != count:
        raise SystemExit(f"expected {count} of {old[:80]!r}, found {n}")
    t = t.replace(old, new)

rep("""// node cold-load-set17.mjs <handover-dir> <out-dir> <cell[,cell...]>
// set17 (2026-10-15): the pre-publication measurement of the v0.1.0-alpha.86 candidate (commit 313f3eea2). A real""",
"""// node cold-load-set17b.mjs <handover-dir> <out-dir> <cell[,cell...]>
// set17b (2026-10-15): cold-load-set17.mjs for the update page through a LONG restart outage (the lab holds the
// update's own worker 150 s at its checkpoint while the Panel is stopped; deploy/e2e/release-recovery/
// guest_set17b_restart_delay.py). Changes against cold-load-set17.mjs, nothing else: the page is read until 12 min
// after the press (not 10); every visible access hold ([data-top-layer="hold"]: the licence gate's and the session
// gate's) is sampled with its title and whether the mounted pages are held ([data-access-hold="blocked"]); every /api/
// answer of 400 or more records its error code (PANEL_STARTING and the others), and the page's own availability reads
// record their state; the timeline adds the holds, the PANEL_STARTING answers, the availability states and the update
// dialog's navigation-lock line over time. The "ui" cell is not used by set17b.
//
// set17 (2026-10-15): the pre-publication measurement of the v0.1.0-alpha.86 candidate (commit 313f3eea2). A real""")
rep("const BODY_PATHS = ['/api/v1/panel/version', '/api/v1/audit-logs', '/api/v1/panel/update/start'];",
    "const BODY_PATHS = ['/api/v1/panel/version', '/api/v1/audit-logs', '/api/v1/panel/update/start', '/api/v1/panel/availability'];")
rep("""        if (res && req.method() === 'GET' && path0 === '/api/v1/managed-services' && !res.ok()) {""",
"""        // set17b: the error code of every answer of 400 or more (PANEL_STARTING, AUTH_*, LICENSE_*), never a body.
        if (res && res.status() >= 400 && !entry.body) {
            try { const text = await res.text(); const code = (JSON.parse(text) || {}).code; if (typeof code === 'string') entry.code = code.slice(0, 80); }
            catch { entry.code_unreadable = true; }
        } else if (res && res.status() >= 400 && entry.body) {
            try { const code = (JSON.parse(entry.body) || {}).code; if (typeof code === 'string') entry.code = code.slice(0, 80); } catch { /* not JSON */ }
        }
        if (res && req.method() === 'GET' && path0 === '/api/v1/managed-services' && !res.ok()) {""")
rep("""    return { t: Date.now(), path: location.pathname + location.search, vis: document.visibilityState, tokens, dialog, corner, card, buttons,
        card_current_version: currentVersion, alerts, sidebar_version: sidebarVersion, build_stamp: stamp ? txt(stamp) : null,
        app_aria_hidden: app ? app.getAttribute('aria-hidden') : null, html_lang: document.documentElement.lang || null, other_dialogs: others };""",
"""    // set17b: every visible access hold (licence gate, session gate) and whether the mounted pages are held.
    const holds = Array.from(document.querySelectorAll('[data-top-layer="hold"]')).filter(x => x.getBoundingClientRect().width > 0 || x.getClientRects().length)
        .map(x => { const h = x.querySelector('h1, h2, h3'); return { title: h ? txt(h) : null, text: (txt(x) || '').slice(0, 900), buttons: Array.from(x.querySelectorAll('button')).map(b => txt(b)) }; });
    const heldPages = document.querySelectorAll('[data-access-hold="blocked"]').length;
    return { t: Date.now(), path: location.pathname + location.search, vis: document.visibilityState, tokens, dialog, corner, card, buttons,
        card_current_version: currentVersion, alerts, sidebar_version: sidebarVersion, build_stamp: stamp ? txt(stamp) : null,
        app_aria_hidden: app ? app.getAttribute('aria-hidden') : null, html_lang: document.documentElement.lang || null, other_dialogs: others,
        holds, held_pages: heldPages };""")
rep("""                    other_dialogs: (s.other_dialogs || []).map(d => /The Panel is not answering|Panel yanıt vermiyor/.test(d.text || '') ? 'licence-gate-not-answering' : kindOf(d.text)) };""",
"""                    other_dialogs: (s.other_dialogs || []).map(d => /The Panel is not answering|Panel yanıt vermiyor/.test(d.text || '') ? 'licence-gate-not-answering' : kindOf(d.text)),
                    holds: (s.holds || []).map(h => holdKind(h)) };""")
rep("""const kindOf = text => { if (!text) return null; for (const [name, re] of NOTICE_KINDS) if (re.test(text)) return name; return 'other'; };""",
"""const kindOf = text => { if (!text) return null; for (const [name, re] of NOTICE_KINDS) if (re.test(text)) return name; return 'other'; };
// set17b: the access holds by their English titles (web/src/i18n/en.ts recovery.*Title, screens/en.ts accessHold.*Title).
const HOLD_KINDS = [
    ['licence-gate-update', /The Panel is not answering; an update may be restarting it/],
    ['licence-gate-reload', /This page is about to reload/],
    ['licence-gate-license', /Panel access could not be confirmed just now/],
    ['licence-gate-availability', /The Panel did not answer just now/],
    ['licence-gate-auth', /Your session could not be confirmed just now/],
    ['session-gate-starting', /The panel is starting/],
    ['session-gate-checking', /Checking panel access/],
    ['session-gate-auth', /Your session could not be checked/],
    ['session-gate-availability', /Panel readiness could not be checked/],
    ['session-gate-license', /License status could not be checked/],
    ['session-gate-loading', /Opening CelikPanel/],
];
const holdKind = h => { const s = `${h?.title || ''} ${h?.text || ''}`; for (const [name, re] of HOLD_KINDS) if (re.test(s)) return name; return 'hold-other'; };""")
rep("""                const key = JSON.stringify([s.kind, !!s.dialog, !!s.corner, s.surface.dialog_icon?.background, s.surface.dialog_icon?.svg, s.surface.card?.icon_color, s.alerts.length, s.sidebar_version, s.card_current_version]);""",
"""                const key = JSON.stringify([s.kind, !!s.dialog, !!s.corner, s.surface.dialog_icon?.background, s.surface.dialog_icon?.svg, s.surface.card?.icon_color, s.alerts.length, s.sidebar_version, s.card_current_version, !!s.dialog?.lock_line, s.held_pages > 0]);""")
rep("""        // 4. the page is read until it shows the target version for 20 s (at most 10 min after the press)
        const deadline = clickAt + 600000;""",
"""        // 4. the page is read until it shows the target version for 20 s (set17b: at most 12 min after the press)
        const deadline = clickAt + 720000;""")
rep("""        summary.bounded_end = { at: iso(), reached_target: versionAt !== null, limit_ms: 600000 };""",
"""        summary.bounded_end = { at: iso(), reached_target: versionAt !== null, limit_ms: 720000 };""")
rep("""        const key = JSON.stringify([s.kind, !!s.dialog, !!s.corner, s.surface?.dialog_icon?.background, s.surface?.dialog_icon?.svg, s.surface?.card?.icon_color, (s.alerts || []).length, (s.other_dialogs || []).map(d => d.title)]);""",
"""        const key = JSON.stringify([s.kind, !!s.dialog, !!s.corner, s.surface?.dialog_icon?.background, s.surface?.dialog_icon?.svg, s.surface?.card?.icon_color, (s.alerts || []).length, (s.other_dialogs || []).map(d => d.title), !!s.dialog?.lock_line, (s.held_pages || 0) > 0]);""")
rep("""                other_dialogs: (s.other_dialogs || []).map(d => ({ title: d.title, text: d.text, buttons: d.buttons, surface: d.surface })),""",
"""                other_dialogs: (s.other_dialogs || []).map(d => ({ title: d.title, text: d.text, buttons: d.buttons, surface: d.surface })),
                holds: (s.holds || []).map(h => ({ kind: holdKind(h), title: h.title, text: h.text.slice(0, 600), buttons: h.buttons })), held_pages: s.held_pages ?? null,
                lock_line_present: !!s.dialog?.lock_line,""")
rep("""    return {
        pressed_at: summary.start_pressed_at ?? null,""",
"""    // set17b: the holds, the navigation lock, PANEL_STARTING and the page's own availability states over time.
    const span = (list, f = s => s.t) => list.length ? { samples: list.length, first: { at: iso(f(list[0])), s: rel(f(list[0])) }, last: { at: iso(f(list[list.length - 1])), s: rel(f(list[list.length - 1])) } } : { samples: 0 };
    const holdSamples = ok.filter(s => (s.holds || []).length);
    const holdKinds = {};
    for (const s of holdSamples) for (const h of s.holds) { const k = holdKind(h); (holdKinds[k] ||= []).push(s); }
    const afterPress = ok.filter(s => pressed && s.t >= pressed);
    const versionT = versionSeen ? versionSeen.t : null;
    const untilVersion = afterPress.filter(s => !versionT || s.t < versionT);
    const lockOn = untilVersion.filter(s => s.dialog?.lock_line);
    const lockGaps = [];
    let gap = null;
    for (const s of untilVersion) {
        if (s.dialog?.lock_line) { if (gap) { lockGaps.push(gap); gap = null; } }
        else { if (!gap) gap = { from: iso(s.t), from_s: rel(s.t), to: iso(s.t), to_s: rel(s.t), samples: 0, dialog: !!s.dialog }; gap.to = iso(s.t); gap.to_s = rel(s.t); gap.samples += 1; }
    }
    if (gap) lockGaps.push(gap);
    const allReqs = summary.__all_requests || reqs;
    const starting = allReqs.filter(r => r.code === 'PANEL_STARTING').sort((a, b) => a.start_ms - b.start_ms);
    const coded = {};
    for (const r of allReqs.filter(r => r.code && (!pressed || r.start_ms >= pressed))) { const k = `${r.status} ${r.code}`; coded[k] = (coded[k] || 0) + 1; }
    const avail = allReqs.filter(r => r.url.split('?')[0] === '/api/v1/panel/availability' && (!pressed || r.start_ms >= pressed))
        .map(r => { let state = null; try { state = JSON.parse(r.body || 'null')?.state ?? null; } catch { /* none */ } return { t: r.start_ms, status: r.status, state, error: r.error ?? null }; });
    const availRuns = [];
    for (const a of avail) { const key = `${a.status}|${a.state}|${a.error}`; const last = availRuns[availRuns.length - 1];
        if (!last || last.key !== key) availRuns.push({ key, status: a.status, state: a.state, error: a.error, from: iso(a.t), from_s: rel(a.t), to: iso(a.t), to_s: rel(a.t), reads: 1 });
        else { last.to = iso(a.t); last.to_s = rel(a.t); last.reads += 1; } }
    availRuns.forEach(x => delete x.key);
    const set17b = {
        holds: { ...span(holdSamples), kinds: Object.fromEntries(Object.entries(holdKinds).map(([k, v]) => [k, span(v)])) },
        held_pages: span(ok.filter(s => (s.held_pages || 0) > 0)),
        licence_gate_update_dialog: span(holdKinds['licence-gate-update'] || []),
        session_gate_starting_hold: span(holdKinds['session-gate-starting'] || []),
        navigation_lock: { lock_line_samples_until_version: lockOn.length, samples_until_version: untilVersion.length, first: lockOn[0] ? { at: iso(lockOn[0].t), s: rel(lockOn[0].t) } : null,
            last: lockOn.length ? { at: iso(lockOn[lockOn.length - 1].t), s: rel(lockOn[lockOn.length - 1].t) } : null, gaps_before_the_version: lockGaps,
            note: 'the update dialog line "Navigation is temporarily locked", sampled from the press until the new version showed' },
        panel_starting_answers: { count: starting.length, first: starting[0] ? { at: starting[0].start, s: rel(starting[0].start_ms), url: starting[0].url, page: starting[0].page } : null,
            last: starting.length ? { at: starting[starting.length - 1].start, s: rel(starting[starting.length - 1].start_ms), url: starting[starting.length - 1].url } : null,
            paths: [...new Set(starting.map(r => r.url.split('?')[0]))] },
        error_codes_after_press: coded,
        page_availability_reads: availRuns,
    };
    return {
        set17b,
        pressed_at: summary.start_pressed_at ?? null,""")
rep("""        summary.timeline = timelineOf(samples, probes, requests.filter(r => r.page === 'update-page'), summary, navigations);""",
"""        summary.__all_requests = requests;
        summary.timeline = timelineOf(samples, probes, requests.filter(r => r.page === 'update-page'), summary, navigations);
        delete summary.__all_requests;""")
rep("""            note: 'the candidate shows the unknown wording only after 6 min from the start and at least 2 min from the first lost read' },""",
"""            note: 'the candidate shows the unknown wording only after 6 min from the start and at least 2 min from the first lost read; set17b: the licence gate keeps its positive decision and the session gate does not hold on PANEL_STARTING inside the same window' },""")
t = t.replace("by: 'cold-load-set17.mjs'", "by: 'cold-load-set17b.mjs'")
t = t.replace("usage: node cold-load-set17.mjs", "usage: node cold-load-set17b.mjs")
t = t.replace("schema: 'celikpanel/set17-start-answer/v1'", "schema: 'celikpanel/set17-start-answer/v1', tool: 'cold-load-set17b.mjs'")
open(dst, 'w', encoding='utf-8', newline='\n').write(t)
print("ok")
