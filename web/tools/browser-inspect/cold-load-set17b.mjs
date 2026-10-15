// node cold-load-set17b.mjs <handover-dir> <out-dir> <cell[,cell...]>
// set17b (2026-10-15): cold-load-set17.mjs for the update page through a LONG restart outage (the lab holds the
// update's own worker 150 s at its checkpoint while the Panel is stopped; deploy/e2e/release-recovery/
// guest_set17b_restart_delay.py). Changes against cold-load-set17.mjs, nothing else: the page is read until 12 min
// after the press (not 10); every visible access hold ([data-top-layer="hold"]: the licence gate's and the session
// gate's) is sampled with its title and whether the mounted pages are held ([data-access-hold="blocked"]); every /api/
// answer of 400 or more records its error code (PANEL_STARTING and the others), and the page's own availability reads
// record their state; the timeline adds the holds, the PANEL_STARTING answers, the availability states and the update
// dialog's navigation-lock line over time. The "ui" cell is not used by set17b.
//
// set17 (2026-10-15): the pre-publication measurement of the v0.1.0-alpha.86 candidate (commit 313f3eea2). A real
// headless Chrome against a REAL CelikPanel on a disposable lab guest, reached through the lab driver's loopback SSH
// forward (deploy/e2e/release-recovery/set17_trial.py, set12's browser window protocol). Nothing but the loopback
// address of the forward is opened. The request recorder is cold-load-set16.mjs's (every /api/ request with its
// times), with the bodies of the update status reads, the audit list and the version route.
//
//   cells:
//     page   the update page during a REAL update (window "page", hand.mode "update-page"). The owner signs in and
//            opens Settings -> CelikPanel updates (desktop 1440x900, English). The driver prepares the update
//            ("page-ready": the read-only checks before the update check, the origin proved, GET update/check). The
//            owner presses "Check for updates", waits until the start button is enabled and presses it. The page's
//            own POST /api/v1/panel/update/start is HELD (request interception; its bytes are not changed) only
//            until the driver has armed its read-only observer for the page's own request id ("arm_<id>", at most
//            11 s; the page's own start timeout is 15 s), then released. The answer is written to
//            start-answer.json and done.json ends the driver's window; this tool keeps reading the page.
//            From before the press until the page shows the new version (and 20 s more; at most 10 min after the
//            press) the page is sampled every 500 ms: the update dialog (title, status line, hint, icon, colours of
//            its icon surface, T+, the navigation-lock line), the corner notice (the "toast"), the card's tracking
//            line (data-update-phase, its icon and colours, T+), every [role="alert"], the card's buttons, the
//            sidebar's version and the card's current version. Independently, every 500 ms this process asks the
//            Panel for GET /api/v1/panel/availability without a cookie (answered = any HTTP status; not answered =
//            no HTTP answer). Pictures of each new state (at most 24). Summary: page-summary.json.
//     ui     after a fresh install (hand.mode "fresh") or after the update or its return: sign-in; the sidebar's
//            build id at 1280x800 and 390x844; Components in English and Turkish (rows against the catalogue's
//            27 names and descriptions in web/src/i18n/screens/server/{en,tr}.ts; document language); the Dashboard
//            in English and Turkish (Recent activity sentences against the audit list's actions; document
//            language); the Store in English and Turkish (state badges and filter); (fresh only) a component's own
//            page on an unverifiable stored record (lab preparation "unverifiable-record", English then Turkish);
//            three direct reads; the journal.
//     done   writes done.json
//
//   BROWSER_INSPECT_MODULES  directory whose node_modules holds puppeteer-core (required; outside the repository)
//   CHROME_PATH              the installed Chrome (default: the usual Windows/Linux places)
//   LIVE_PROFILE_ROOT        a short directory for the browser profile (profile-dir.mjs; refused beyond 120 characters)
//
// Pictures are never taken of a page while it is hidden. The owner's password is read from the root-only file the
// driver names and typed into the sign-in form; every text this script writes is passed through the redaction list of
// that file first. Cookies are never read.
import { createHash } from 'node:crypto';
import { existsSync, readFileSync } from 'node:fs';
import { appendFile, mkdir, writeFile } from 'node:fs/promises';
import { request as httpsRequest } from 'node:https';
import { createRequire } from 'node:module';
import { dirname, join, resolve } from 'node:path';
import { connect as tlsConnect } from 'node:tls';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { shortProfileDir } from './profile-dir.mjs';

if (!process.env.BROWSER_INSPECT_MODULES) { console.error('Set BROWSER_INSPECT_MODULES (see README.md).'); process.exit(2); }
const requireFrom = createRequire(pathToFileURL(join(resolve(process.env.BROWSER_INSPECT_MODULES), 'package.json')));
const loaded = await import(pathToFileURL(requireFrom.resolve('puppeteer-core')).href);
const puppeteer = [loaded.default?.default, loaded.default, loaded].find(item => typeof item?.launch === 'function');
const chrome = process.env.CHROME_PATH || ['C:/Program Files/Google/Chrome/Application/chrome.exe', '/usr/bin/google-chrome', '/usr/bin/chromium'].find(candidate => existsSync(candidate));
if (!chrome) { console.error('No installed Chrome found. Set CHROME_PATH.'); process.exit(2); }

const [handDir, outRoot, cellsArg = 'ui,done'] = process.argv.slice(2);
if (!handDir || !outRoot) { console.error('usage: node cold-load-set17b.mjs <handover-dir> <out-dir> <cells>'); process.exit(2); }
const cells = cellsArg.split(',');
const hand = JSON.parse(readFileSync(join(handDir, 'ready.json'), 'utf8'));
const port = Number(hand.local_port);
if (!(port > 1024 && port < 65536)) throw new Error('bad port in the handover file');
const base = `https://127.0.0.1:${port}`;
const wslShare = process.env.LIVE_WSL_SHARE || '//wsl.localhost/archlinux';
const secretPath = process.platform === 'win32' ? wslShare + hand.password_file : hand.password_file;
const pause = ms => new Promise(done => setTimeout(done, ms));
const iso = (ms = Date.now()) => new Date(ms).toISOString();
const DESKTOP = { width: 1440, height: 900, deviceScaleFactor: 1 };
const DESKTOP_1280 = { width: 1280, height: 800, deviceScaleFactor: 1 };
const PHONE = { width: 390, height: 844, deviceScaleFactor: 2, isMobile: true, hasTouch: true };
const HERE = dirname(fileURLToPath(import.meta.url));

let secret = null;
const redactions = () => [secret?.password, ...(secret?.redact || [])].filter(value => typeof value === 'string' && value.length >= 6);
const redact = text => { let out = String(text ?? ''); for (const value of redactions()) out = out.split(value).join('[REDACTED]'); return out; };
const jsonl = async (file, value) => appendFile(file, redact(JSON.stringify(value)) + '\n');

function leafOf() {
    return new Promise((done, fail) => {
        const socket = tlsConnect({ host: '127.0.0.1', port, rejectUnauthorized: false, servername: 'localhost' }, () => {
            const cert = socket.getPeerX509Certificate();
            socket.end();
            if (!cert) return fail(new Error('no peer certificate'));
            const spki = cert.publicKey.export({ type: 'spki', format: 'der' });
            done({ leaf_sha256: createHash('sha256').update(cert.raw).digest('hex'), spki_sha256_base64: createHash('sha256').update(spki).digest('base64') });
        });
        socket.setTimeout(8000, () => { socket.destroy(); fail(new Error('timeout')); });
        socket.on('error', fail);
    });
}

const leaf = await leafOf();
await mkdir(outRoot, { recursive: true });
await writeFile(join(outRoot, `leaf-${Date.now()}.json`), JSON.stringify({ at: iso(), ...leaf, handover_leaf_sha256: hand.tls_leaf_sha256 }, null, 2));
secret = JSON.parse(readFileSync(secretPath, 'utf8'));
const profile = shortProfileDir(outRoot);
const browser = await puppeteer.launch({
    executablePath: chrome, headless: true, userDataDir: profile,
    args: ['--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--disable-component-update',
        '--disable-sync', '--disable-domain-reliability', '--force-color-profile=srgb', `--ignore-certificate-errors-spki-list=${leaf.spki_sha256_base64}`],
});

// -- recording (cold-load-set16.mjs's recorder, reduced) --------------------------------------------------------------

let phase = 'start';
let dir = outRoot;
const requests = [];
const front = { page: null };
const BODY_PATHS = ['/api/v1/panel/version', '/api/v1/audit-logs', '/api/v1/panel/update/start', '/api/v1/panel/availability'];

async function newPage(label, lang = 'en', vp = 'desktop') {
    const page = await browser.newPage();
    page.__label = label;
    page.__lang = lang;
    page.__vp = vp;
    await page.setViewport(vp === 'phone' ? PHONE : vp === '1280' ? DESKTOP_1280 : DESKTOP);
    await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }]);
    await page.evaluateOnNewDocument(value => { try { localStorage.setItem('celikpanel.lang', value); localStorage.setItem('celikpanel.theme', 'light'); } catch { /* none */ } }, lang);
    const started = new Map();
    page.on('request', req => { if (req.url().includes('/api/')) started.set(req, Date.now()); });
    const finish = async (req, failed) => {
        if (!req.url().includes('/api/')) return;
        const t0 = started.get(req) ?? Date.now();
        started.delete(req);
        const t1 = Date.now();
        const res = failed ? null : req.response();
        const entry = { page: label, phase, method: req.method(), url: req.url().replace(base, ''), status: res ? res.status() : null,
            start: iso(t0), end: iso(t1), start_ms: t0, end_ms: t1, ms: t1 - t0 };
        if (failed) entry.error = req.failure()?.errorText;
        const path0 = entry.url.split('?')[0];
        if (res && (BODY_PATHS.includes(path0) || path0 === '/api/v1/panel/update/status')) {
            try {
                const text = await res.text();
                if (path0 === '/api/v1/audit-logs') {
                    const body = JSON.parse(text);
                    entry.audit = (body?.entries || []).map(e => ({ action: e.action, username: e.username, created_at: e.created_at }));
                } else {
                    entry.body = text.slice(0, 1500);
                }
            } catch (error) { entry.body_unreadable = String(error.message).slice(0, 120); }
        }
        // set17b: the error code of every answer of 400 or more (PANEL_STARTING, AUTH_*, LICENSE_*), never a body.
        if (res && res.status() >= 400 && !entry.body) {
            try { const text = await res.text(); const code = (JSON.parse(text) || {}).code; if (typeof code === 'string') entry.code = code.slice(0, 80); }
            catch { entry.code_unreadable = true; }
        } else if (res && res.status() >= 400 && entry.body) {
            try { const code = (JSON.parse(entry.body) || {}).code; if (typeof code === 'string') entry.code = code.slice(0, 80); } catch { /* not JSON */ }
        }
        if (res && req.method() === 'GET' && path0 === '/api/v1/managed-services' && !res.ok()) {
            try { entry.body = (await res.text()).slice(0, 1500); } catch { /* gone */ }
        }
        requests.push(entry);
        await jsonl(join(dir, 'requests.jsonl'), entry);
    };
    page.on('requestfinished', req => void finish(req, false));
    page.on('requestfailed', req => void finish(req, true));
    page.on('console', message => { if (['error', 'warn', 'warning'].includes(message.type())) void jsonl(join(dir, 'console.jsonl'), { at: iso(), page: label, phase, type: message.type(), text: message.text().slice(0, 400) }); });
    return page;
}

async function toFront(page) { await page.bringToFront(); front.page = page; await pause(300); }

async function pageText(page) {
    return page.evaluate(() => (document.querySelector('main') || document.body).innerText.replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n').trim().slice(0, 12000)).catch(() => '');
}

const shotNames = new Set();
async function shot(page, name, events, extra = {}, full = false) {
    if (front.page !== page) { await jsonl(events, { at: iso(), ev: 'shot-skipped', name, why: 'page not in front (hidden)' }); return null; }
    let n = name; let i = 2; while (shotNames.has(n)) n = `${name}-${i++}`; shotNames.add(n);
    try {
        await page.screenshot({ path: join(dir, `${n}-viewport.png`), fullPage: false });
        if (full) await page.screenshot({ path: join(dir, `${n}-full.png`), fullPage: true });
    } catch (error) { await jsonl(events, { at: iso(), ev: 'shot-failed', name: n, error: String(error.message).slice(0, 160) }); return null; }
    await writeFile(join(dir, `${n}.txt`), redact(await pageText(page)));
    await jsonl(events, { at: iso(), ev: 'shot', name: n, ...extra });
    return n;
}

async function prep(name, events, limitMs = 900000) {
    const key = `${name}-${Date.now()}`;
    const asked = Date.now();
    await writeFile(join(handDir, `prep-request-${key}`), iso() + '\n');
    await jsonl(events, { at: iso(), ev: 'prep-requested', name, phase });
    const file = join(handDir, `prep-done-${key}.json`);
    while (Date.now() - asked < limitMs && !existsSync(file)) await pause(100);
    const record = existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : { missing: true, waited_ms: Date.now() - asked };
    const out = { ...record, host_asked_ms: asked, host_got_ms: Date.now() };
    await jsonl(events, { at: iso(), ev: 'prep-done', name, phase, record: out });
    return out;
}

async function signIn(page, events) {
    await page.goto(base + '/domains', { waitUntil: 'networkidle2', timeout: 60000 });
    const shown = await page.waitForFunction(() => document.getElementById('username') ? 'form' : document.querySelector('nav, aside') ? 'signed-in' : null, { timeout: 60000 }).then(handle => handle.jsonValue());
    await jsonl(events, { at: iso(), ev: 'sign-in-page', shown });
    if (shown === 'signed-in') return;
    await page.type('#username', secret.username);
    await page.type('#password', secret.password);
    await Promise.all([page.keyboard.press('Enter'), pause(500)]);
    await page.waitForFunction(() => !document.getElementById('password'), { timeout: 60000 });
    await pause(2500);
}

// -- the update page during a real update ------------------------------------------------------------------------------

// The notice texts of both versions (web/src/i18n/en.ts and screens/server/en.ts at v0.1.0-alpha.85 and 313f3eea2).
const NOTICE_KINDS = [
    ['overdue', /has not answered for longer than an update normally needs/],
    ['unknown', /could not be read just now/],
    ['restarting', /is restarting as part of the update/],
    ['connection-lost', /The connection was interrupted/],
    ['may-be-restarting', /may be restarting; the same operation is still being tracked/],
    ['reloading', /Reloading this page with/],
    ['succeeded', /The update completed/],
    ['verifying', /being verified|is installed and this panel is running it/],
    ['reading', /Reading the state of/],
    ['applying', /is being applied/],
    ['accepted', /accepted/i],
    ['failed', /failed|could not be completed|did not complete/i],
];
const kindOf = text => { if (!text) return null; for (const [name, re] of NOTICE_KINDS) if (re.test(text)) return name; return 'other'; };
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
const holdKind = h => { const s = `${h?.title || ''} ${h?.text || ''}`; for (const [name, re] of HOLD_KINDS) if (re.test(s)) return name; return 'hold-other'; };

const PAGE_SAMPLE = () => {
    const txt = el => el ? (el.innerText || '').replace(/\s+/g, ' ').trim() : null;
    const cs = el => { if (!el) return null; const s = getComputedStyle(el); return { border: s.borderTopColor, background: s.backgroundColor, color: s.color }; };
    const root = getComputedStyle(document.documentElement);
    const tokens = {};
    for (const name of ['warning-mark', 'warning', 'primary', 'danger', 'success']) tokens[name] = root.getPropertyValue('--' + name).trim();
    const svgOf = el => { const s = el ? el.querySelector('svg') : null; return s ? { class: s.getAttribute('class') || '', color: getComputedStyle(s).color, spinning: /animate-spin/.test(s.getAttribute('class') || '') } : null; };
    const dlg = document.querySelector('[role="dialog"][aria-labelledby="system-update-operation-title"]');
    const panel = dlg ? dlg.firstElementChild : null;
    const icon = panel ? panel.querySelector(':scope > span') : null;
    const ddAfter = (scope, label) => { if (!scope) return null; const dt = Array.from(scope.querySelectorAll('dt')).find(x => (x.textContent || '').trim() === label); return dt && dt.nextElementSibling ? txt(dt.nextElementSibling) : null; };
    const dialog = dlg ? { title: txt(document.getElementById('system-update-operation-title')), status: txt(document.getElementById('system-update-operation-status')),
        text: (txt(panel) || '').slice(0, 1200), icon: svgOf(icon), icon_surface: cs(icon), panel_surface: cs(panel), t_plus: ddAfter(panel, 'T+'),
        lock_line: Array.from(panel ? panel.querySelectorAll('p') : []).map(txt).find(x => /Navigation is temporarily locked|Gezinme geçici olarak kilitli/.test(x || '')) || null } : null;
    const cornerEl = document.querySelector('aside[aria-labelledby="system-update-background-title"]');
    const corner = cornerEl ? { text: (txt(cornerEl) || '').slice(0, 1200), surface: cs(cornerEl), t_plus: ((txt(cornerEl) || '').match(/T\+(\d{2}:\d{2}(?::\d{2})?)/) || [])[1] || null } : null;
    const cardEl = document.querySelector('[data-update-phase]');
    const card = cardEl ? { phase: cardEl.getAttribute('data-update-phase'), text: (txt(cardEl) || '').slice(0, 1200), surface: cs(cardEl), icon: svgOf(cardEl),
        t_plus: txt(cardEl.querySelector('[data-update-elapsed] span')), overdue: !!cardEl.querySelector('[data-update-overdue]') } : null;
    const section = document.querySelector('section[aria-labelledby="panel-update-title"]');
    const buttons = section ? Array.from(section.querySelectorAll('button')).map(b => ({ text: txt(b), disabled: b.disabled, id: b.id || null })) : null;
    const currentVersion = section ? (() => { const dd = section.querySelector('dl dd'); return dd ? txt(dd) : null; })() : null;
    const alerts = Array.from(document.querySelectorAll('[role="alert"]')).filter(a => a.getBoundingClientRect().width > 0).map(a => ({ text: (txt(a) || '').slice(0, 600), surface: cs(a) }));
    const sidebar = Array.from(document.querySelectorAll('aside')).find(a => !a.hasAttribute('aria-labelledby'));
    const sidebarVersion = sidebar ? ((txt(sidebar) || '').match(/v0\.1\.0-alpha\.\d+/) || [])[0] || null : null;
    const stamp = document.querySelector('[data-build-stamp]');
    const app = document.querySelector('div.contents');
    // set17 run: every other visible dialog (the licence access gate drew one over the update's restart in cell u86).
    const others = Array.from(document.querySelectorAll('[role="dialog"], [role="alertdialog"], [aria-modal="true"]'))
        .filter(x => x !== dlg && !(dlg && dlg.contains(x)) && x.getBoundingClientRect().width > 0)
        .map(x => { const box = x.querySelector('h1, h2, h3') ; const panelEl = x.firstElementChild || x; return { title: box ? txt(box) : null, text: (txt(x) || '').slice(0, 900), surface: cs(panelEl), buttons: Array.from(x.querySelectorAll('button')).map(b => txt(b)) }; });
    // set17b: every visible access hold (licence gate, session gate) and whether the mounted pages are held.
    const holds = Array.from(document.querySelectorAll('[data-top-layer="hold"]')).filter(x => x.getBoundingClientRect().width > 0 || x.getClientRects().length)
        .map(x => { const h = x.querySelector('h1, h2, h3'); return { title: h ? txt(h) : null, text: (txt(x) || '').slice(0, 900), buttons: Array.from(x.querySelectorAll('button')).map(b => txt(b)) }; });
    const heldPages = document.querySelectorAll('[data-access-hold="blocked"]').length;
    return { t: Date.now(), path: location.pathname + location.search, vis: document.visibilityState, tokens, dialog, corner, card, buttons,
        card_current_version: currentVersion, alerts, sidebar_version: sidebarVersion, build_stamp: stamp ? txt(stamp) : null,
        app_aria_hidden: app ? app.getAttribute('aria-hidden') : null, html_lang: document.documentElement.lang || null, other_dialogs: others,
        holds, held_pages: heldPages };
};

// Which theme token a computed colour is (light theme): the rgb triple against each token's "r g b".
function tokenOf(color, tokens) {
    if (!color || !tokens) return null;
    const m = color.match(/\d+(\.\d+)?/g) || [];
    const rgb = m.slice(0, 3).join(' ');
    const alpha = m.length > 3 ? Number(m[3]) : 1;
    if (alpha === 0) return 'transparent';
    for (const [name, value] of Object.entries(tokens)) if (value && value.split(/\s+/).join(' ') === rgb) return alpha < 1 ? `${name}/${alpha}` : name;
    return null;
}
function surfaceOf(sample) {
    const tk = sample.tokens;
    const out = {};
    if (sample.dialog) out.dialog_icon = { background: tokenOf(sample.dialog.icon_surface?.background, tk), color: tokenOf(sample.dialog.icon_surface?.color, tk),
        svg: (sample.dialog.icon?.class || '').match(/lucide-[a-z0-9-]+/g)?.filter(x => x !== 'lucide')?.join(' ') || null, spinning: sample.dialog.icon?.spinning ?? null,
        raw: sample.dialog.icon_surface };
    if (sample.card) out.card = { border: tokenOf(sample.card.surface?.border, tk), background: tokenOf(sample.card.surface?.background, tk), icon_color: tokenOf(sample.card.icon?.color, tk),
        svg: (sample.card.icon?.class || '').match(/lucide-[a-z0-9-]+/g)?.join(' ') || null, spinning: sample.card.icon?.spinning ?? null, raw: sample.card.surface };
    if (sample.corner) out.corner = { border: tokenOf(sample.corner.surface?.border, tk), background: tokenOf(sample.corner.surface?.background, tk), raw: sample.corner.surface };
    out.alerts = (sample.alerts || []).map(a => ({ border: tokenOf(a.surface?.border, tk), background: tokenOf(a.surface?.background, tk), text: a.text.slice(0, 160) }));
    return out;
}
const warningIn = s => {
    const v = JSON.stringify([s.dialog_icon?.background, s.dialog_icon?.color, s.card?.icon_color, s.card?.border, s.card?.background, s.corner?.border, s.corner?.background, ...(s.alerts || []).map(a => [a.border, a.background])]);
    return /warning/.test(v);
};
const dangerIn = s => /danger/.test(JSON.stringify([s.dialog_icon, s.card, s.corner, s.alerts]));

// One unauthenticated read of the Panel through the forward: any HTTP status is an answer.
function availability() {
    return new Promise(done => {
        const t0 = Date.now();
        const req = httpsRequest({ host: '127.0.0.1', port, path: '/api/v1/panel/availability', method: 'GET', rejectUnauthorized: false, servername: 'localhost', timeout: 2500 }, res => {
            res.resume();
            res.on('end', () => done({ t: t0, ms: Date.now() - t0, answered: true, status: res.statusCode }));
        });
        req.on('timeout', () => { req.destroy(new Error('timeout')); });
        req.on('error', error => done({ t: t0, ms: Date.now() - t0, answered: false, error: String(error.code || error.message).slice(0, 80) }));
        req.end();
    });
}

async function cellPage() {
    const events = join(dir, 'events.jsonl');
    const target = hand.target_version || null;
    await jsonl(events, { at: iso(), ev: 'start', mode: hand.mode, target, baseline: hand.baseline_version || null, chrome: await browser.version() });
    const summary = { target_version: target, baseline_version: hand.baseline_version || null, cell: hand.cell };
    const write = async () => writeFile(join(dir, 'page-summary.json'), redact(JSON.stringify(summary, null, 2)));

    phase = 'sign-in';
    const page = await newPage('update-page', 'en', 'desktop');
    await toFront(page);
    await signIn(page, events);
    phase = 'update-page';
    await page.goto(base + '/settings?section=updates', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await page.waitForSelector('#panel-update-title', { timeout: 60000 });
    await pause(2000);
    summary.opened_at = iso();
    await shot(page, 'page-opened', events);

    // The sampler and the availability probe run from here until the end of the cell.
    const samples = [];
    const probes = [];
    let sampling = true;
    const navigations = [];
    page.on('framenavigated', frame => { if (frame === page.mainFrame()) navigations.push({ at: iso(), t: Date.now(), url: frame.url().replace(base, '') }); });
    const shotKeys = new Set();
    let shots = 0;
    const sampler = (async () => {
        while (sampling) {
            const t0 = Date.now();
            let s;
            try { s = await page.evaluate(PAGE_SAMPLE); } catch (error) { s = { t: t0, error: String(error.message).slice(0, 120) }; }
            if (!s.error) {
                s.surface = surfaceOf(s);
                s.kind = { dialog: kindOf(s.dialog ? `${s.dialog.title} ${s.dialog.status}` : null), corner: kindOf(s.corner?.text), card: s.card?.phase ?? null, card_text: kindOf(s.card?.text),
                    other_dialogs: (s.other_dialogs || []).map(d => /The Panel is not answering|Panel yanıt vermiyor/.test(d.text || '') ? 'licence-gate-not-answering' : kindOf(d.text)),
                    holds: (s.holds || []).map(h => holdKind(h)) };
                const key = JSON.stringify([s.kind, !!s.dialog, !!s.corner, s.surface.dialog_icon?.background, s.surface.dialog_icon?.svg, s.surface.card?.icon_color, s.alerts.length, s.sidebar_version, s.card_current_version, !!s.dialog?.lock_line, s.held_pages > 0]);
                if (!shotKeys.has(key) && shots < 24 && s.vis === 'visible') {
                    shotKeys.add(key); shots++;
                    const name = await shot(page, `state-${String(shots).padStart(2, '0')}`, events, { kind: s.kind, t_plus: s.dialog?.t_plus ?? s.card?.t_plus ?? s.corner?.t_plus ?? null });
                    s.picture = name;
                }
            }
            samples.push(s);
            await jsonl(join(dir, 'page-samples.jsonl'), s);
            await pause(Math.max(0, 500 - (Date.now() - t0)));
        }
    })();
    const prober = (async () => {
        while (sampling) {
            const t0 = Date.now();
            const r = await availability();
            probes.push(r);
            await jsonl(join(dir, 'availability.jsonl'), r);
            await pause(Math.max(0, 500 - (Date.now() - t0)));
        }
    })();
    const stop = async () => { sampling = false; await sampler; await prober; };

    try {
        // 1. the driver prepares the update (read-only checks, the origin, GET update/check)
        const ready = await prep('page-ready', events, 600000);
        summary.page_ready = ready.result ?? ready;
        if (!ready.result?.ok) {
            summary.not_started = 'the driver did not prepare the update (page-ready)';
            await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set17b.mjs', not_started: true }) + '\n');
            await stop(); await write(); return;
        }
        // 2. the owner presses "Check for updates" and waits for the start button
        const checkAt = Date.now();
        const pressed = await page.evaluate(() => {
            const s = document.querySelector('section[aria-labelledby="panel-update-title"]');
            const b = s ? s.querySelector('button') : null;
            if (!b || b.disabled) return false;
            b.click(); return (b.innerText || '').trim();
        }).catch(() => false);
        summary.check_pressed = { at: iso(checkAt), button: pressed };
        let enabled = false;
        for (let i = 0; i < 360 && !enabled; i++) {
            enabled = await page.evaluate(() => { const b = document.getElementById('panel-update-start-button'); return !!b && !b.disabled; }).catch(() => false);
            if (!enabled) await pause(500);
        }
        summary.start_enabled = { enabled, after_ms: Date.now() - checkAt };
        await shot(page, 'before-start', events);
        if (!enabled) {
            summary.not_started = 'the start button did not become enabled within 180 s';
            await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set17b.mjs', not_started: true }) + '\n');
            await stop(); await write(); return;
        }
        // 3. the owner presses start; the page's own POST is held until the driver has armed its observer
        let held = null;
        let heldResolve;
        const heldPromise = new Promise(r => { heldResolve = r; });
        await page.setRequestInterception(true);
        const onRequest = req => {
            if (req.isInterceptResolutionHandled()) return;
            if (req.method() === 'POST' && req.url().replace(base, '').split('?')[0] === '/api/v1/panel/update/start' && !held) {
                held = { req, at: Date.now() };
                heldResolve(held);
                return;
            }
            req.continue().catch(() => {});
        };
        page.on('request', onRequest);
        let answerResolve;
        const answerPromise = new Promise(r => { answerResolve = r; });
        page.on('response', async res => {
            if (res.request().method() === 'POST' && res.url().replace(base, '').split('?')[0] === '/api/v1/panel/update/start') {
                let text = null; try { text = await res.text(); } catch { /* gone */ }
                answerResolve({ at: Date.now(), status: res.status(), text });
            }
        });
        page.on('requestfailed', req => {
            if (req.method() === 'POST' && req.url().replace(base, '').split('?')[0] === '/api/v1/panel/update/start') answerResolve({ at: Date.now(), failed: req.failure()?.errorText ?? 'failed' });
        });
        const clickAt = Date.now();
        summary.start_pressed_at = iso(clickAt);
        await page.click('#panel-update-start-button');
        const got = await Promise.race([heldPromise, pause(30000).then(() => null)]);
        if (!got) {
            summary.not_started = 'the page sent no start request within 30 s of the press';
            page.off('request', onRequest);
            await page.setRequestInterception(false).catch(() => {});
            await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set17b.mjs', not_started: true }) + '\n');
            await stop(); await write(); return;
        }
        let body = null; try { body = JSON.parse(got.req.postData() || '{}'); } catch { body = null; }
        const rid = body?.request_id;
        summary.start_request = { held_at: iso(got.at), after_press_ms: got.at - clickAt, request_id: rid,
            body_keys: body ? Object.keys(body).sort() : null, target_version: body?.version ?? null, confirmed: body?.confirmed ?? null };
        let armed = null;
        if (typeof rid === 'string' && /^[0-9a-f]{32}$/.test(rid)) armed = await prep('arm_' + rid, events, 11000);
        const releasedAt = Date.now();
        await got.req.continue().catch(error => { summary.release_error = String(error.message).slice(0, 160); });
        summary.release = { released_at: iso(releasedAt), held_ms: releasedAt - got.at, armed: armed?.result ?? armed };
        // set17 run: the answer of the held request is not always reported by the browser's network events (the
        // request leaves through the page's service worker); 5 s, then the page's own dialog line is recorded.
        const answer = await Promise.race([answerPromise, pause(5000).then(() => null)]);
        const pageSaid = await page.evaluate(() => { const s = document.getElementById('system-update-operation-status'); return s ? (s.innerText || '').trim() : null; }).catch(() => null);
        page.off('request', onRequest);
        await page.setRequestInterception(false).catch(() => {});
        let answerBody = null; try { answerBody = answer?.text ? JSON.parse(answer.text) : null; } catch { answerBody = answer?.text?.slice(0, 600) ?? null; }
        const startAnswer = { schema: 'celikpanel/set17-start-answer/v1', tool: 'cold-load-set17b.mjs', request_id: rid, pressed_ms: clickAt, held_ms: got.at, released_ms: releasedAt,
            answered_ms: answer?.at ?? null, status: answer?.status ?? null, failed: answer?.failed ?? null, body: answerBody, armed: armed?.result ?? armed,
            answer_seen_by_the_browser: answer !== null, page_dialog_line_after_release: pageSaid,
            note: 'the page\'s own POST, held unchanged until the observer was armed, then released' };
        summary.start_answer = startAnswer;
        await writeFile(join(handDir, 'start-answer.json'), redact(JSON.stringify(startAnswer, null, 2)) + '\n');
        await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set17b.mjs', started: true }) + '\n');
        await write();
        // 4. the page is read until it shows the target version for 20 s (set17b: at most 12 min after the press)
        const deadline = clickAt + 720000;
        let versionAt = null;
        while (Date.now() < deadline) {
            const last = samples[samples.length - 1];
            if (target && last && !last.error && (last.sidebar_version === target || last.card_current_version === target) && !last.dialog) {
                versionAt = versionAt ?? last.t;
                if (Date.now() - versionAt >= 20000) break;
            } else if (last && !last.error) versionAt = null;
            await pause(500);
        }
        summary.bounded_end = { at: iso(), reached_target: versionAt !== null, limit_ms: 720000 };
        await shot(page, 'end', events, {}, true);
    } finally {
        await stop();
        summary.__all_requests = requests;
        summary.timeline = timelineOf(samples, probes, requests.filter(r => r.page === 'update-page'), summary, navigations);
        delete summary.__all_requests;
        await write();
    }
}

// The key moments of the update page, from the samples, the probe and the page's own reads.
function timelineOf(samples, probes, reqs, summary, navigations) {
    const pressed = summary.start_pressed_at ? Date.parse(summary.start_pressed_at) : null;
    const rel = t => (pressed && t ? Math.round((t - pressed) / 100) / 10 : null);
    const ok = samples.filter(s => !s.error);
    // states: a new row each time the visible kind/surface changes
    const states = [];
    for (const s of ok) {
        const key = JSON.stringify([s.kind, !!s.dialog, !!s.corner, s.surface?.dialog_icon?.background, s.surface?.dialog_icon?.svg, s.surface?.card?.icon_color, (s.alerts || []).length, (s.other_dialogs || []).map(d => d.title), !!s.dialog?.lock_line, (s.held_pages || 0) > 0]);
        const last = states[states.length - 1];
        if (!last || last.key !== key) {
            states.push({ key, from: iso(s.t), from_s: rel(s.t), to: iso(s.t), to_s: rel(s.t), samples: 1,
                dialog: s.dialog ? { title: s.dialog.title, status: s.dialog.status, t_plus_first: s.dialog.t_plus, lock_line: s.dialog.lock_line } : null,
                dialog_kind: s.kind?.dialog, corner_present: !!s.corner, corner_kind: s.kind?.corner, corner_text: s.corner?.text?.slice(0, 400) ?? null,
                card_phase: s.kind?.card, card_text: s.card?.text?.slice(0, 400) ?? null, surface: { dialog_icon: s.surface?.dialog_icon ? { background: s.surface.dialog_icon.background, color: s.surface.dialog_icon.color, svg: s.surface.dialog_icon.svg, spinning: s.surface.dialog_icon.spinning } : null,
                    card: s.surface?.card ? { border: s.surface.card.border, background: s.surface.card.background, icon_color: s.surface.card.icon_color, svg: s.surface.card.svg, spinning: s.surface.card.spinning } : null,
                    corner: s.surface?.corner ? { border: s.surface.corner.border, background: s.surface.corner.background } : null, alerts: s.surface?.alerts },
                warning_surface: warningIn(s.surface || {}), danger_surface: dangerIn(s.surface || {}),
                other_dialogs: (s.other_dialogs || []).map(d => ({ title: d.title, text: d.text, buttons: d.buttons, surface: d.surface })),
                holds: (s.holds || []).map(h => ({ kind: holdKind(h), title: h.title, text: h.text.slice(0, 600), buttons: h.buttons })), held_pages: s.held_pages ?? null,
                lock_line_present: !!s.dialog?.lock_line,
                app_aria_hidden: s.app_aria_hidden, sidebar_version: s.sidebar_version, card_current_version: s.card_current_version, buttons: s.buttons, picture: s.picture ?? null });
        } else {
            last.to = iso(s.t); last.to_s = rel(s.t); last.samples += 1;
            if (s.dialog?.t_plus) last.t_plus_last = s.dialog.t_plus; else if (s.card?.t_plus) last.t_plus_last = s.card.t_plus;
            if (!last.picture && s.picture) last.picture = s.picture;
        }
    }
    states.forEach(x => delete x.key);
    // the probe: answered / not answered windows after the press
    const after = probes.filter(p => !pressed || p.t >= pressed);
    const windows = [];
    for (const p of after) {
        const last = windows[windows.length - 1];
        if (!last || last.answered !== p.answered) windows.push({ answered: p.answered, from: iso(p.t), from_s: rel(p.t), to: iso(p.t), to_s: rel(p.t), reads: 1, first: p.status ?? p.error });
        else { last.to = iso(p.t); last.to_s = rel(p.t); last.reads += 1; }
    }
    const outages = windows.filter(w => !w.answered);
    const longest = outages.reduce((m, w) => Math.max(m, (Date.parse(w.to) - Date.parse(w.from)) / 1000), 0);
    // the page's own update status reads
    const statusReads = reqs.filter(r => r.url.startsWith('/api/v1/panel/update/status'));
    const lost = statusReads.filter(r => r.status === null || r.status >= 500);
    const firstLost = lost[0] ?? null;
    const answeredAfter = firstLost ? statusReads.find(r => r.start_ms > firstLost.start_ms && r.status !== null && r.status < 500) : null;
    const versionSeen = summary.target_version ? ok.find(s => s.sidebar_version === summary.target_version || s.card_current_version === summary.target_version) : null;
    const notKnown = ok.filter(s => /could not be read just now|not known/.test(`${s.dialog?.title} ${s.dialog?.status} ${s.dialog?.text} ${s.corner?.text} ${s.card?.text}`));
    const restarting = ok.filter(s => s.kind?.dialog === 'restarting' || s.kind?.card_text === 'restarting' || s.kind?.corner === 'restarting');
    const corners = ok.filter(s => s.corner);
    const warn = ok.filter(s => warningIn(s.surface || {}));
    const otherDialogs = ok.filter(s => (s.other_dialogs || []).length);
    const outageS = firstLost && answeredAfter ? (answeredAfter.start_ms - firstLost.start_ms) / 1000 : null;
    // set17b: the holds, the navigation lock, PANEL_STARTING and the page's own availability states over time.
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
        pressed_at: summary.start_pressed_at ?? null,
        samples: samples.length, sample_errors: samples.length - ok.length, probes: probes.length,
        states,
        availability_windows: windows, availability_outages: outages.length, availability_longest_outage_s: longest,
        page_status_reads: statusReads.length, page_status_reads_lost: lost.length,
        page_first_lost_read: firstLost ? { at: firstLost.start, s: rel(firstLost.start_ms), status: firstLost.status, error: firstLost.error ?? null } : null,
        page_first_answer_after_loss: answeredAfter ? { at: answeredAfter.start, s: rel(answeredAfter.start_ms), status: answeredAfter.status } : null,
        page_outage_s: outageS,
        version_changed: versionSeen ? { at: iso(versionSeen.t), s: rel(versionSeen.t), sidebar: versionSeen.sidebar_version, card: versionSeen.card_current_version } : null,
        navigations: navigations.map(n => ({ ...n, s: rel(n.t) })),
        not_known_wording: { samples: notKnown.length, first: notKnown[0] ? { at: iso(notKnown[0].t), s: rel(notKnown[0].t) } : null },
        restarting_wording: { samples: restarting.length, first: restarting[0] ? { at: iso(restarting[0].t), s: rel(restarting[0].t) } : null, last: restarting.length ? { at: iso(restarting[restarting.length - 1].t), s: rel(restarting[restarting.length - 1].t) } : null },
        corner_notice: { samples: corners.length, first: corners[0] ? { at: iso(corners[0].t), s: rel(corners[0].t), text: corners[0].corner.text.slice(0, 400) } : null },
        other_dialogs: { samples: otherDialogs.length, first: otherDialogs[0] ? { at: iso(otherDialogs[0].t), s: rel(otherDialogs[0].t), titles: otherDialogs[0].other_dialogs.map(d => d.title) } : null,
            last: otherDialogs.length ? { at: iso(otherDialogs[otherDialogs.length - 1].t), s: rel(otherDialogs[otherDialogs.length - 1].t) } : null },
        warning_surface: { samples: warn.length, first: warn[0] ? { at: iso(warn[0].t), s: rel(warn[0].t) } : null },
        alerts_seen: [...new Set(ok.flatMap(s => (s.alerts || []).map(a => a.text.slice(0, 200))))],
        bound: { window_from_start_s: 360, window_from_first_lost_read_s: 120,
            first_lost_read_after_press_s: firstLost ? rel(firstLost.start_ms) : null,
            note: 'the candidate shows the unknown wording only after 6 min from the start and at least 2 min from the first lost read; set17b: the licence gate keeps its positive decision and the session gate does not hold on PANEL_STARTING inside the same window' },
    };
}

// -- after the install / the update / the return ---------------------------------------------------------------------

// The catalogue's display names and descriptions (web/src/i18n/screens/server/{en,tr}.ts of the checked-out tree).
function catalogue(lang) {
    const text = readFileSync(join(HERE, '..', '..', 'src', 'i18n', 'screens', 'server', `${lang}.ts`), 'utf8');
    const out = { name: {}, description: {} };
    const re = /'services\.component\.(name|description)\.([a-z0-9_.-]+)':\s*('(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*")/g;
    for (const m of text.matchAll(re)) {
        const raw = m[3];
        const value = raw.startsWith('"') ? JSON.parse(raw) : raw.slice(1, -1).replace(/\\'/g, "'").replace(/\\\\/g, '\\');
        out[m[1]][m[2]] = value;
    }
    return out;
}

const STAMP_FACTS = () => {
    const units = Array.from(document.querySelectorAll('[data-build-stamp]')).filter(u => u.getClientRects().length && u.getBoundingClientRect().width > 0);
    const u = units[0] || null;
    if (!u) return { found: false };
    const id = u.querySelector('[aria-hidden="true"]');
    const idRects = id ? Array.from(id.getClientRects()).map(r => ({ top: Math.round(r.top), left: Math.round(r.left), width: Math.round(r.width) })) : [];
    const box = u.getBoundingClientRect();
    const host = u.closest('aside, footer') || u.parentElement;
    const hostBox = host ? host.getBoundingClientRect() : null;
    return { found: true, class: u.className, id_text: id ? id.textContent : null, id_rects: idRects, id_on_one_line: idRects.length === 1,
        stamp_line: u.parentElement ? (u.parentElement.innerText || '').replace(/\s+/g, ' ').trim() : null,
        overflows_container: hostBox ? box.right > hostBox.right + 0.5 : null, white_space: getComputedStyle(u).whiteSpace };
};

async function sidebarWidth(events, name, vp) {
    const page = await newPage('sidebar-' + name, 'en', vp);
    await toFront(page);
    await page.goto(base + '/settings', { waitUntil: 'networkidle2', timeout: 60000 });
    await pause(2500);
    let menu = null;
    if (vp === 'phone') {
        menu = await page.evaluate(() => { const b = document.querySelector('button[aria-label="Menu"]'); if (!b) return false; b.click(); return true; }).catch(() => false);
        await pause(1200);
    }
    const facts = await page.evaluate(STAMP_FACTS).catch(error => ({ error: String(error.message).slice(0, 160) }));
    await shot(page, `sidebar-${name}`, events);
    await page.close();
    const value = { at: iso(), name, menu_opened: menu, ...facts };
    await jsonl(events, { ev: 'sidebar-width', ...value });
    return value;
}

async function componentsPage(events, lang) {
    const label = 'components-' + lang;
    const page = await newPage(label, lang);
    await toFront(page);
    const at = Date.now();
    await page.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await pause(20000);
    const rows = await page.evaluate(() => Array.from(document.querySelectorAll('main a[href^="/services/"]')).map(a => ({
        id: decodeURIComponent(a.getAttribute('href').slice('/services/'.length)), name: (a.innerText || '').trim(),
        description: a.nextElementSibling ? (a.nextElementSibling.innerText || '').trim() : null, installed_look: /text-fg(\s|$)/.test(a.className) }))).catch(() => []);
    const lang0 = await page.evaluate(() => document.documentElement.lang || null).catch(() => null);
    await shot(page, `components-${lang}`, events, { lang }, true);
    await page.close();
    const cat = catalogue(lang);
    const other = catalogue(lang === 'tr' ? 'en' : 'tr');
    const checked = rows.map(r => ({ ...r, catalogue_name: cat.name[r.id] ?? null, catalogue_description: cat.description[r.id] ?? null,
        name_matches: cat.name[r.id] !== undefined ? r.name === cat.name[r.id] : null,
        description_matches: cat.description[r.id] !== undefined ? r.description === cat.description[r.id] : null,
        description_is_other_language: other.description[r.id] !== undefined && other.description[r.id] !== cat.description[r.id] ? r.description === other.description[r.id] : false }));
    const ids = Object.keys(cat.name);
    const served = requests.filter(r => r.page === label && r.url.split('?')[0] === '/api/v1/managed-services' && r.method === 'GET');
    return { lang, load_at: iso(at), html_lang: lang0, rows: rows.length, catalogue_ids: ids.length,
        rows_in_catalogue: checked.filter(r => r.catalogue_name !== null).length,
        names_match: checked.filter(r => r.name_matches === true).length, names_differ: checked.filter(r => r.name_matches === false).map(r => ({ id: r.id, shown: r.name, catalogue: r.catalogue_name })),
        descriptions_match: checked.filter(r => r.description_matches === true).length,
        descriptions_differ: checked.filter(r => r.description_matches === false).map(r => ({ id: r.id, shown: r.description, catalogue: r.catalogue_description, other_language: r.description_is_other_language })),
        catalogue_ids_not_on_page: ids.filter(id => !rows.some(r => r.id === id)), rows_not_in_catalogue: checked.filter(r => r.catalogue_name === null).map(r => ({ id: r.id, name: r.name })),
        managed_services_reads: served.map(r => ({ status: r.status, ms: r.ms })), rows_detail: checked };
}

const RAW_ID = /\b[a-z][a-z0-9]*(?:[._:][a-z0-9*-]+){1,}\b/;
async function dashboardPage(events, lang) {
    const label = 'dashboard-' + lang;
    const page = await newPage(label, lang);
    await toFront(page);
    const at = Date.now();
    await page.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await pause(15000);
    const facts = await page.evaluate(() => {
        const sections = Array.from(document.querySelectorAll('section'));
        const s = sections.find(x => /^(Recent activity|Son etkinlik)/.test((x.innerText || '').trim()));
        const items = s ? Array.from(s.querySelectorAll('li')).map(li => (li.innerText || '').replace(/\s+/g, ' ').trim()) : [];
        return { found: !!s, items, html_lang: document.documentElement.lang || null };
    }).catch(error => ({ error: String(error.message).slice(0, 160) }));
    await shot(page, `dashboard-${lang}`, events, { lang }, true);
    await page.close();
    const audit = requests.filter(r => r.page === label && r.audit).map(r => r.audit).pop() || [];
    const items = facts.items || [];
    return { lang, load_at: iso(at), html_lang: facts.html_lang, section_found: facts.found, items,
        items_with_a_raw_id: items.filter(x => RAW_ID.test(x)),
        items_with_an_underscore_word: items.filter(x => /\b[a-z]+_[a-z_]+\b/.test(x)),
        audit_actions_read: audit.slice(0, 28).map(e => `${e.username}: ${e.action}`) };
}

async function storePage(events, lang) {
    const label = 'store-' + lang;
    const page = await newPage(label, lang);
    await toFront(page);
    const at = Date.now();
    await page.goto(base + '/addons', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await pause(12000);
    const facts = await page.evaluate(() => ({ path: location.pathname, html_lang: document.documentElement.lang || null,
        options: Array.from(document.querySelectorAll('main select option')).map(o => (o.textContent || '').trim()),
        buttons: Array.from(document.querySelectorAll('main [role="tab"], main [aria-pressed]')).map(b => (b.innerText || '').trim()).filter(Boolean).slice(0, 40),
        badges: Array.from(document.querySelectorAll('main span.rounded-full, main span[class*="badge"]')).map(b => (b.innerText || '').trim()).filter(Boolean).slice(0, 120) })).catch(error => ({ error: String(error.message).slice(0, 160) }));
    await shot(page, `store-${lang}`, events, { lang }, true);
    await page.close();
    const counts = {};
    for (const b of facts.badges || []) counts[b] = (counts[b] || 0) + 1;
    return { lang, load_at: iso(at), ...facts, badge_counts: counts, reads: requests.filter(r => r.page === label).map(r => ({ url: r.url, status: r.status })).slice(0, 30) };
}

// A component's own page on an unverifiable stored record (lab preparation), read every 250 ms for 45 s.
const COMPONENT_LINES = [
    ['checking-stored', /CelikPanel is checking the components now|CelikPanel bileşenleri şimdi denetliyor|bileşenleri şimdi kontrol/],
    ['check-held', /A change is in progress on this server, so the stored record|Bu sunucuda bir değişiklik sürüyor/],
    ['could-not-read', /record could not be read|kaydı okunamadı/i],
    ['could-not-check', /could not check|kontrol edilemedi/i],
];
async function componentUnverified(events, lang, id) {
    const label = `component-unverified-${lang}`;
    const preparation = await prep('unverifiable-record', events, 120000);
    const page = await newPage(label, lang);
    await toFront(page);
    const at = Date.now();
    await page.goto(base + '/services/' + encodeURIComponent(id), { waitUntil: 'domcontentloaded', timeout: 60000 });
    const seen = [];
    let last = null;
    while (Date.now() - at < 45000) {
        const text = await page.evaluate(() => (document.querySelector('main')?.innerText || '').replace(/\s+/g, ' ').trim()).catch(() => '');
        const kinds = COMPONENT_LINES.filter(([, re]) => re.test(text)).map(([k]) => k);
        const toast = await page.evaluate(() => Array.from(document.querySelectorAll('[role="alert"]')).map(a => (a.innerText || '').replace(/\s+/g, ' ').trim()).filter(Boolean)).catch(() => []);
        const key = JSON.stringify([kinds, toast]);
        if (key !== last) {
            last = key;
            const name = await shot(page, `component-unverified-${lang}-${seen.length + 1}`, events, { kinds });
            seen.push({ at: iso(), after_ms: Date.now() - at, kinds, alerts: toast, text_head: text.slice(0, 600), picture: name });
        }
        await pause(250);
    }
    const lang0 = await page.evaluate(() => document.documentElement.lang || null).catch(() => null);
    await page.close();
    const reqs = requests.filter(r => r.page === label).map(r => ({ method: r.method, url: r.url, status: r.status, start: r.start, ms: r.ms, body: r.url.startsWith('/api/v1/managed-services') && r.status >= 400 ? r.body : undefined }));
    return { lang, id, html_lang: lang0, preparation: preparation.result ?? preparation, sequence: seen,
        unverified_answers: reqs.filter(r => r.status === 503), scan_posts: reqs.filter(r => r.method === 'POST' && r.url.startsWith('/api/v1/managed-services/scan')), requests: reqs };
}

async function directReads(events) {
    const out = {};
    for (const path of ['/api/v1/panel/version', '/api/v1/license/access', '/api/v1/managed-services']) {
        const page = await newPage('read-' + path.split('/').pop());
        const from = Date.now();
        try {
            const res = await page.goto(base + path, { waitUntil: 'load', timeout: 60000 });
            const text = res ? await res.text() : null;
            let body = null; try { body = JSON.parse(text); } catch { /* not JSON */ }
            if (path === '/api/v1/managed-services' && body && Array.isArray(body.services)) body = { ...body, services: `<${body.services.length} entries>`, profiles: Array.isArray(body.profiles) ? `<${body.profiles.length} entries>` : body.profiles };
            out[path] = { status: res ? res.status() : null, at: iso(from), body };
        } catch (error) { out[path] = { error: String(error.message).slice(0, 200) }; }
        await pause(300);
        await page.close();
    }
    await writeFile(join(dir, 'direct-reads.json'), redact(JSON.stringify(out, null, 2)));
    await jsonl(events, { at: iso(), ev: 'direct-reads', paths: Object.keys(out) });
    return out;
}

async function cellUI() {
    const events = join(dir, 'events.jsonl');
    const fresh = hand.mode === 'fresh';
    await jsonl(events, { at: iso(), ev: 'start', mode: hand.mode, chrome: await browser.version() });
    const result = { mode: hand.mode, phases: {} };
    const save = async () => writeFile(join(dir, 'ui-summary.json'), redact(JSON.stringify(result, null, 2)));
    const run = async (name, fn) => {
        phase = name;
        const t = Date.now();
        try { result.phases[name] = { started: iso(t), ...(await fn()), seconds: Math.round((Date.now() - t) / 100) / 10 }; }
        catch (error) { result.phases[name] = { started: iso(t), error: redact(String(error.stack || error)).slice(0, 1200) }; }
        await save();
    };
    phase = 'sign-in';
    const signer = await newPage('sign-in');
    await toFront(signer);
    await signIn(signer, events);
    await signer.close();
    await run('sidebar', async () => ({ w1280: await sidebarWidth(events, '1280x800', '1280'), w390: await sidebarWidth(events, '390x844', 'phone') }));
    await run('components', async () => ({ en: await componentsPage(events, 'en'), tr: await componentsPage(events, 'tr') }));
    await run('dashboard', async () => ({ en: await dashboardPage(events, 'en'), tr: await dashboardPage(events, 'tr') }));
    await run('store', async () => ({ en: await storePage(events, 'en'), tr: await storePage(events, 'tr') }));
    if (fresh) {
        await run('component-unverified', async () => {
            const comp = result.phases.components?.en?.rows_detail || [];
            const id = (comp.find(r => r.id === 'nginx') || comp.find(r => r.installed_look) || comp[0] || {}).id || 'nginx';
            return { en: await componentUnverified(events, 'en', id), tr: await componentUnverified(events, 'tr', id) };
        });
    }
    await run('reads', async () => ({ reads: await directReads(events) }));
    phase = 'end';
    await prep('journal', events, 300000);
}

const results = {};
for (const cell of cells) {
    dir = join(outRoot, cell);
    await mkdir(dir, { recursive: true });
    const started = iso();
    try {
        if (cell === 'page') await cellPage();
        else if (cell === 'ui') await cellUI();
        else if (cell === 'done') { if (!existsSync(join(handDir, 'done.json'))) await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set17b.mjs' }) + '\n'); }
        else throw new Error(`unknown cell ${cell}`);
        results[cell] = { started, ended: iso(), ok: true };
    } catch (error) {
        results[cell] = { started, ended: iso(), ok: false, phase, error: redact(String(error.stack || error)).slice(0, 1500) };
        if (cell === 'page' && !existsSync(join(handDir, 'done.json'))) await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set17b.mjs', error: true }) + '\n').catch(() => {});
    }
    await writeFile(join(dir, 'run.json'), redact(JSON.stringify({ cell, chrome: await browser.version(),
        handover: { ...hand, password_file: '(root-only file, not copied)' }, ...results[cell] }, null, 2)));
    console.log(JSON.stringify({ cell, ...results[cell] }));
}
await browser.close();
