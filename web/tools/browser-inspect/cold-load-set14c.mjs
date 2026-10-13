// node cold-load-set14c.mjs <handover-dir> <out-dir> <cell[,cell...]>
// set14c (2026-10-13): the native record after set14b, on commit 68b79b372 (7024534bc): GET /api/v1/license/access
// carries valid_for_seconds and the page dates its access deadline from the moment it asked (set14b D2); reads that
// need the Agent answer 503 AGENT_UNREACHABLE (set14b O3); a failed read on the Components page says the state could
// not be read, not that an action may have completed (set14b O2). A real headless Chrome against a REAL CelikPanel on a
// disposable lab guest, reached through the lab driver's loopback SSH forward (deploy/e2e/release-recovery/
// set14c_trial.py, set12's browser window "ui"). Nothing but the loopback address of the forward is opened. Copied from
// cold-load-set14b.mjs; the recording is the same, plus: every answer of GET /api/v1/license/access is recorded whole
// (can_use_panel, valid_until, valid_for_seconds, state, observation) with the browser's send and receipt times, the
// deadline the page takes from it and the guest-minus-browser bound it gives (valid_until - valid_for_seconds is the
// server's clock when it answered); GET /api/v1/firewall and /api/v1/system/check are recorded with their bodies when
// refused; a page can have its own viewport (the Turkish phone page).
//
//   cells: ui    phases, in this order:
//                baseline      the clock as it is; the stored scan older than 315 s; one Dashboard load, 90 s
//                agent         the Agent stopped by the owner, stale: the owner's reads of the firewall and the system
//                              check and the nginx restart through the API (admission); the Dashboard (first language,
//                              desktop) 40 s with a full-page picture; the Components page in the first language on
//                              the desktop and in the other on a phone, each banner pictured; the Agent started; the
//                              banner's Rescan pressed on the phone page
//                clock-behind  the guest clock moved 420 s back; the owner's unconditional scan; the stored scan older
//                              than 315 s on the guest clock; one Dashboard load, 90 s
//                clock-ahead   the guest clock moved 840 s forward (about 420 s ahead); one Dashboard load, 90 s
//                restore       the guest clock moved 420 s back and NTP switched on; two clock reads 30 s apart
//          done  writes done.json
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
import { createRequire } from 'node:module';
import { join, resolve } from 'node:path';
import { connect as tlsConnect } from 'node:tls';
import { pathToFileURL } from 'node:url';
import { shortProfileDir } from './profile-dir.mjs';

if (!process.env.BROWSER_INSPECT_MODULES) { console.error('Set BROWSER_INSPECT_MODULES (see README.md).'); process.exit(2); }
const requireFrom = createRequire(pathToFileURL(join(resolve(process.env.BROWSER_INSPECT_MODULES), 'package.json')));
const loaded = await import(pathToFileURL(requireFrom.resolve('puppeteer-core')).href);
const puppeteer = [loaded.default?.default, loaded.default, loaded].find(item => typeof item?.launch === 'function');
const chrome = process.env.CHROME_PATH || ['C:/Program Files/Google/Chrome/Application/chrome.exe', '/usr/bin/google-chrome', '/usr/bin/chromium'].find(candidate => existsSync(candidate));
if (!chrome) { console.error('No installed Chrome found. Set CHROME_PATH.'); process.exit(2); }

const [handDir, outRoot, cellsArg = 'ui,done'] = process.argv.slice(2);
if (!handDir || !outRoot) { console.error('usage: node cold-load-set14c.mjs <handover-dir> <out-dir> <cells>'); process.exit(2); }
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
const PHONE = { width: 390, height: 844, deviceScaleFactor: 2, isMobile: true, hasTouch: true };
const LANG = hand.browser?.lang === 'tr' ? 'tr' : 'en';
const OTHER_LANG = LANG === 'tr' ? 'en' : 'tr';
const VIEWPORT_NAME = hand.browser?.viewport === 'phone' ? 'phone' : 'desktop';
const VIEWPORT = VIEWPORT_NAME === 'phone' ? PHONE : DESKTOP;
const WINDOW_S = 300;
const STALE_S = 315;
// The card's own strings (web/src/i18n, commit 68b79b372), to name what the card says.
const TEXT = {
    label: { en: 'System services', tr: 'Sistem servisleri' },
    scanNow: { en: 'Scan services', tr: 'Servisleri tara' },
    states: [
        ['checking', /Checking components|Bileşenler kontrol ediliyor/],
        ['held', /A change is in progress|Bir değişiklik sürüyor/],
        ['failed', /Could not check:|Kontrol edilemedi:/],
        ['not-recent', /Not checked in the last 5 minutes|Son 5 dakikada kontrol edilmedi/],
        ['never', /Not checked yet|Henüz bakılmadı/],
        ['known', /\d+\s*\/\s*\d+/],
    ],
};
const stateOf = text => { if (!text) return 'absent'; for (const [name, re] of TEXT.states) if (re.test(text)) return name; return 'other'; };

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

// -- recording -------------------------------------------------------------------------------------------------------

let phase = 'start';
let dir = outRoot;
const requests = [];   // every /api/ request of every page, in memory for the phase summaries
const phases = [];
const front = { page: null };

// In the page, read only: the card's text every 100 ms and every visibility change.
const CARD_LOGGER = `(() => {
    const labels = ${JSON.stringify(Object.values(TEXT.label))};
    window.__set14card = []; let last = null;
    const find = () => {
        for (const s of document.querySelectorAll('span.font-medium')) {
            if (labels.includes((s.textContent || '').trim())) { const c = s.closest('section[role="status"], button'); if (c) return c; }
        }
        return null;
    };
    window.__set14find = find;
    const tick = () => {
        const c = find();
        const text = c ? c.innerText.replace(/\\s+/g, ' ').trim() : null;
        const key = text + '|' + document.visibilityState;
        if (key !== last) { last = key; window.__set14card.push({ t: Date.now(), text, tag: c ? c.tagName : null, busy: c ? c.getAttribute('aria-busy') : null, vis: document.visibilityState, path: location.pathname }); }
    };
    setInterval(tick, 100);
    window.__set14toast = []; const toastSeen = new Set();
    const toasts = () => {
        for (const a of document.querySelectorAll('[role="alert"]')) {
            const text = (a.innerText || '').replace(/\\s+/g, ' ').trim();
            if (text && !toastSeen.has(text)) { toastSeen.add(text); window.__set14toast.push({ t: Date.now(), text, path: location.pathname }); }
        }
    };
    const watch = () => { if (!document.documentElement) return setTimeout(watch, 10); new MutationObserver(() => { tick(); toasts(); }).observe(document.documentElement, { subtree: true, childList: true, characterData: true }); };
    watch();
    document.addEventListener('visibilitychange', () => { window.__set14card.push({ t: Date.now(), ev: 'visibility', vis: document.visibilityState }); tick(); });
})();`;

async function newPage(label, lang = LANG, vp = VIEWPORT_NAME) {
    const page = await browser.newPage();
    page.__label = label;
    page.__lang = lang;
    page.__vp = vp === 'phone' ? 'phone' : 'desktop';
    await page.setViewport(page.__vp === 'phone' ? PHONE : DESKTOP);
    await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }]);
    await page.evaluateOnNewDocument(value => { try { localStorage.setItem('celikpanel.lang', value); localStorage.setItem('celikpanel.theme', 'light'); } catch { /* none */ } }, lang);
    await page.evaluateOnNewDocument(CARD_LOGGER);
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
        const scanRead = req.method() === 'GET' && entry.url.split('?')[0] === '/api/v1/managed-services';
        const scanPost = req.method() === 'POST' && entry.url.startsWith('/api/v1/managed-services/scan');
        if (res && (scanRead || scanPost)) {
            try {
                const text = await res.text();
                const body = JSON.parse(text);
                const age = body && Object.prototype.hasOwnProperty.call(body, 'scan_age_seconds') ? body.scan_age_seconds : undefined;
                entry.answer = { code: body?.code ?? null, scanned_at: body?.scanned_at ?? null,
                    scan_age_seconds: age === undefined ? '<absent>' : age,
                    scan_age_seconds_type: age === undefined ? 'absent' : age === null ? 'null' : typeof age,
                    services: Array.isArray(body?.services) ? body.services.length : null,
                    error: typeof body?.error === 'string' ? body.error.slice(0, 600) : null,
                    message: typeof body?.message === 'string' ? body.message.slice(0, 600) : null };
                // The server's own clock at the answer is scanned_at + scan_age_seconds; it lies between this
                // request's start and end on this browser's clock, so the guest-minus-browser offset is bounded.
                if (typeof body?.scanned_at === 'string' && typeof age === 'number' && Number.isFinite(Date.parse(body.scanned_at))) {
                    const serverNow = Date.parse(body.scanned_at) + age * 1000;
                    entry.answer.server_now = iso(serverNow);
                    entry.answer.guest_minus_browser_ms = { low: Math.round(serverNow - t1), high: Math.round(serverNow - t0) };
                }
                if (!res.ok()) {
                    entry.answer.body = text.slice(0, 3000);
                    entry.answer.body_mentions_socket = /agent\.sock|\/run\/celikpanel|dial unix|connect:/i.test(text);
                }
            } catch (error) { entry.answer = { unreadable: String(error.message).slice(0, 120) }; }
        }
        const path0 = entry.url.split('?')[0];
        if (res && req.method() === 'GET' && path0 === '/api/v1/license/access') {
            try {
                const body = JSON.parse(await res.text());
                const vf = body && Object.prototype.hasOwnProperty.call(body, 'valid_for_seconds') ? body.valid_for_seconds : undefined;
                entry.access = { can_use_panel: body?.can_use_panel, valid_until: body?.valid_until,
                    valid_for_seconds: vf === undefined ? '<absent>' : vf, valid_for_seconds_type: vf === undefined ? 'absent' : vf === null ? 'null' : typeof vf,
                    state: body?.state, observation: body?.observation, keys: Object.keys(body || {}).sort(), sent: iso(t0), received: iso(t1) };
                if (Number.isSafeInteger(body?.valid_until)) {
                    // The page's deadline (accessObservation.ts): the moment it asked plus valid_for_seconds; the old
                    // rule compared valid_until (the server's clock) with this browser's clock.
                    entry.access.valid_until_iso = iso(body.valid_until * 1000);
                    entry.access.old_rule_accepts_at_receipt = body.valid_until * 1000 > t1;
                    if (typeof vf === 'number' && Number.isFinite(vf)) {
                        entry.access.browser_deadline = iso(t0 + vf * 1000);
                        entry.access.old_rule_deadline_minus_new_ms = Math.round(body.valid_until * 1000 - (t0 + vf * 1000));
                        if (vf > 0) {
                            const serverNow = body.valid_until * 1000 - vf * 1000;
                            entry.access.server_now = iso(serverNow);
                            entry.access.guest_minus_browser_ms = { low: Math.round(serverNow - t1), high: Math.round(serverNow - t0) };
                        }
                    }
                }
            } catch (error) { entry.access = { unreadable: String(error.message).slice(0, 120) }; }
        }
        if (res && req.method() === 'GET' && ['/api/v1/firewall', '/api/v1/system/check'].includes(path0)) {
            entry.agent_read = { ok: res.ok() };
            if (!res.ok()) {
                try {
                    const text = await res.text();
                    let body = null; try { body = JSON.parse(text); } catch { /* not JSON */ }
                    entry.agent_read = { ok: false, code: body?.code ?? null, error: typeof body?.error === 'string' ? body.error.slice(0, 600) : null,
                        keys: body && typeof body === 'object' ? Object.keys(body).sort() : null, body: text.slice(0, 3000),
                        body_mentions_socket: /agent\.sock|\/run\/celikpanel|dial unix|connect:/i.test(text) };
                } catch (error) { entry.agent_read = { ok: false, unreadable: String(error.message).slice(0, 120) }; }
            }
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

const cardRows = [];
const toastRows = [];
async function cardLog(page) {
    const rows = await page.evaluate(() => (window.__set14card || []).splice(0)).catch(() => []);
    for (const row of rows) {
        const value = { page: page.__label, lang: page.__lang, phase, at: iso(row.t), ...row, state: row.text === undefined ? undefined : stateOf(row.text) };
        cardRows.push(value);
        await jsonl(join(dir, 'cardlog.jsonl'), value);
    }
    const toasts = await page.evaluate(() => (window.__set14toast || []).splice(0)).catch(() => []);
    for (const row of toasts) {
        const value = { page: page.__label, lang: page.__lang, phase, at: iso(row.t), ...row };
        toastRows.push(value);
        await jsonl(join(dir, 'toasts.jsonl'), value);
    }
    return rows;
}

// The card's states of one page in this phase, in order, each with how long it lasted (to the next change).
function sequenceOf(label, name) {
    const rows = cardRows.filter(r => r.page === label && !r.ev);
    const out = [];
    rows.forEach((r, i) => {
        if (r.phase !== name) return;
        const next = rows[i + 1];
        out.push({ at: r.at, state: r.state, vis: r.vis, lasted_ms: next ? next.t - r.t : null, text: r.text });
    });
    return out;
}

async function cardNow(page) {
    return page.evaluate(() => {
        const c = window.__set14find ? window.__set14find() : null;
        return { text: c ? c.innerText.replace(/\s+/g, ' ').trim() : null, vis: document.visibilityState, path: location.pathname };
    }).catch(error => ({ error: String(error.message).slice(0, 120) }));
}

async function pageText(page) {
    return page.evaluate(() => (document.querySelector('main') || document.body).innerText.replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n').trim().slice(0, 8000)).catch(() => '');
}

const shotNames = new Set();
async function shoot(page, name, events, extra = {}) {
    if (front.page !== page) { await jsonl(events, { at: iso(), ev: 'shot-skipped', name, why: 'page not in front (hidden)' }); return; }
    let n = name; let i = 2; while (shotNames.has(n)) n = `${name}-${i++}`; shotNames.add(n);
    const card = await page.evaluateHandle(() => (window.__set14find ? window.__set14find() : null)).catch(() => null);
    const element = card ? card.asElement() : null;
    let cardShot = false;
    if (element) { try { await element.screenshot({ path: join(dir, `${n}-card.png`) }); cardShot = true; } catch { /* the card moved */ } }
    await page.screenshot({ path: join(dir, `${n}-viewport.png`), fullPage: false });
    await writeFile(join(dir, `${n}.txt`), redact(await pageText(page)));
    const now = await cardNow(page);
    const facts = await page.evaluate(() => ({ password_fields_with_text: Array.from(document.querySelectorAll('input[type="password"]')).filter(i => i.value).length,
        scroll_y: Math.round(window.scrollY) })).catch(() => ({}));
    await jsonl(events, { at: iso(), ev: 'shot', name: n, card_picture: cardShot, card: now.text, state: stateOf(now.text), vis: now.vis, path: now.path, ...facts, ...extra });
}

// Watch one page: sample the card every 250 ms and picture each state the first time it appears in this phase.
async function observe(page, ms, events, { shots = true, until = null, tag = '' } = {}) {
    const seen = new Set();
    const deadline = Date.now() + ms;
    let lastFlush = Date.now();
    while (Date.now() < deadline) {
        if (until && until()) break;
        const now = await cardNow(page);
        const state = stateOf(now.text);
        if (shots && state !== 'absent' && !seen.has(state) && now.vis === 'visible' && front.page === page) {
            seen.add(state);
            await shoot(page, `${phase}${tag}-${state}`, events, { lang: page.__lang, viewport: page.__vp });
        }
        if (Date.now() - lastFlush > 5000) { await cardLog(page); lastFlush = Date.now(); }
        await pause(250);
    }
    await cardLog(page);
}

async function prep(name, events) {
    const key = `${name}-${Date.now()}`;
    const asked = Date.now();
    await writeFile(join(handDir, `prep-request-${key}`), iso() + '\n');
    await jsonl(events, { at: iso(), ev: 'prep-requested', name, phase });
    for (let i = 0; i < 9000 && !existsSync(join(handDir, `prep-done-${key}.json`)); i++) await pause(100);
    const record = existsSync(join(handDir, `prep-done-${key}.json`)) ? JSON.parse(readFileSync(join(handDir, `prep-done-${key}.json`), 'utf8')) : { missing: true };
    const got = Date.now();
    const out = { ...record, host_asked_ms: asked, host_got_ms: got };
    const guest = record?.result?.epoch;
    if (typeof guest === 'number') {
        out.guest_minus_host_ms_approx = Math.round(guest * 1000 - (asked + got) / 2);
        // The guest read its clock after this browser asked and before it saw the answer.
        out.guest_minus_host_ms_bounds = { low: Math.round(guest * 1000 - got), high: Math.round(guest * 1000 - asked) };
    }
    await jsonl(events, { at: iso(), ev: 'prep-done', name, phase, record: out });
    return out;
}

async function waitStale(events, label) {
    for (let turn = 0; turn < 6; turn++) {
        const probe = await prep('probe', events);
        const sc = probe.result?.scan_cache || {};
        if (sc.exists === false) return { turn, absent: true, probe };
        const age = Number(sc.age_seconds);
        if (Number.isFinite(age) && age >= STALE_S) return { turn, age, probe };
        const wait = Math.min(Math.max(Number.isFinite(age) ? STALE_S - age + 1 : 60, 1), 340);
        await jsonl(events, { at: iso(), ev: 'wait-stale', label, age, wait_s: wait });
        await pause(wait * 1000);
    }
    throw new Error(`stale condition not reached for ${label} after 6 turns`);
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

const scanPosts = (from, to = Infinity) => requests.filter(r => r.method === 'POST' && r.url.startsWith('/api/v1/managed-services/scan') && r.start_ms >= from && r.start_ms <= to);
const brief = r => ({ page: r.page, url: r.url, status: r.status, start: r.start, end: r.end, ms: r.ms, answer: r.answer, error: r.error });

async function phaseDone(name, started, extra = {}) {
    const ended = Date.now();
    const posts = scanPosts(started, ended);
    const value = { phase: name, started: iso(started), ended: iso(ended), seconds: Math.round((ended - started) / 100) / 10,
        scan_posts: posts.length, posts: posts.map(brief), ...extra };
    phases.push(value);
    await writeFile(join(dir, 'phases.json'), redact(JSON.stringify({ lang: LANG, viewport: VIEWPORT_NAME, phases }, null, 2)));
    return value;
}

const cpu = (a, b) => {
    const pick = p => p?.result || {};
    const x = pick(a), y = pick(b);
    const d = (k, u) => (y.cgroup_cpu?.[u]?.[k] ?? NaN) - (x.cgroup_cpu?.[u]?.[k] ?? NaN);
    const n = (u, f) => Number(y.units?.[u]?.[f]) - Number(x.units?.[u]?.[f]);
    return { wall_s: Math.round(((y.epoch ?? NaN) - (x.epoch ?? NaN)) * 10) / 10,
        agent_usage_usec: d('usage_usec', 'agent'), agent_user_usec: d('user_usec', 'agent'), agent_system_usec: d('system_usec', 'agent'),
        panel_usage_usec: d('usage_usec', 'panel'), agent_cpu_usage_nsec: n('agent', 'CPUUsageNSec'), panel_cpu_usage_nsec: n('panel', 'CPUUsageNSec'),
        agent_main_pid: [x.units?.agent?.MainPID, y.units?.agent?.MainPID], loadavg: [x.loadavg, y.loadavg] };
};

const offsetOf = probe => probe?.guest_minus_host_ms_approx ?? null;
const scanOf = probe => ({ scanned_at: probe?.result?.scan_cache?.scanned_at ?? null, age_s: probe?.result?.scan_cache?.age_seconds ?? null });
const answers = (from, to = Infinity) => requests.filter(r => r.answer && r.start_ms >= from && r.start_ms <= to)
    .map(r => ({ page: r.page, method: r.method, url: r.url, status: r.status, start: r.start, end: r.end, ms: r.ms, answer: r.answer }));
const statesOf = seq => seq.map(x => x.state).filter((x, i, all) => i === 0 || x !== all[i - 1]);

// Wait (bounded, in this process) until an alert toast is on the page, then picture the viewport at once.
async function shootToast(page, name, events, limitMs) {
    const deadline = Date.now() + limitMs;
    while (Date.now() < deadline) {
        const text = await page.evaluate(() => Array.from(document.querySelectorAll('[role="alert"]')).map(a => (a.innerText || '').replace(/\s+/g, ' ').trim()).filter(Boolean)).catch(() => []);
        if (text.length) {
            await page.screenshot({ path: join(dir, `${name}-viewport.png`), fullPage: false });
            await writeFile(join(dir, `${name}.txt`), redact(await pageText(page)));
            await jsonl(events, { at: iso(), ev: 'shot', name, toast: text, lang: page.__lang, viewport: VIEWPORT_NAME });
            return { at: iso(), text };
        }
        await pause(100);
    }
    await jsonl(events, { at: iso(), ev: 'toast-not-seen', name, limit_ms: limitMs });
    return null;
}

// A read of the stored records (GET /api/v1/managed-services, the Dashboard's own first request) in a page that is
// not the app: its answer bounds the guest-minus-browser offset (scanned_at + scan_age_seconds lies between the
// request's start and end). It reads only; no scan is asked for.
async function clockRead(events, label) {
    const page = await newPage('clock-' + label);
    const from = Date.now();
    await page.goto(base + '/api/v1/managed-services', { waitUntil: 'load', timeout: 60000 }).catch(error => jsonl(events, { at: iso(), ev: 'clock-read-error', label, error: String(error.message).slice(0, 200) }));
    await pause(300);
    await page.close();
    const row = requests.filter(r => r.page === 'clock-' + label && r.start_ms >= from && r.answer?.guest_minus_browser_ms).pop();
    const value = row ? { at: row.start, ms: row.ms, status: row.status, bounds: row.answer.guest_minus_browser_ms,
        mid: Math.round((row.answer.guest_minus_browser_ms.low + row.answer.guest_minus_browser_ms.high) / 2),
        scanned_at: row.answer.scanned_at, scan_age_seconds: row.answer.scan_age_seconds } : null;
    await jsonl(events, { at: iso(), ev: 'clock-read', label, value });
    return value;
}

// -- set14c: the licence access reads of one page, the Components page's banners -----------------------------------

const ACCESS = '/api/v1/license/access';
// The access reads of one page from its load, with the time between them and what each answer said.
function accessReads(label, from, windowMs) {
    const rows = requests.filter(r => r.page === label && r.method === 'GET' && r.url.split('?')[0] === ACCESS && r.start_ms >= from && r.start_ms - from <= windowMs);
    return rows.map((r, i) => ({ start: r.start, end: r.end, ms: r.ms, status: r.status, after_load_ms: r.start_ms - from,
        since_previous_ms: i ? r.start_ms - rows[i - 1].start_ms : null, access: r.access ?? null, error: r.error ?? null }));
}
// When the page mounted: the first Dashboard read (managed-services) and the first card text that is not absent.
function mountOf(label, from) {
    const read = requests.find(r => r.page === label && r.method === 'GET' && r.url.split('?')[0] === '/api/v1/managed-services' && r.start_ms >= from);
    const card = cardRows.find(r => r.page === label && !r.ev && r.state && r.state !== 'absent' && r.t >= from);
    return { first_managed_services_read_ms: read ? read.start_ms - from : null, first_card_ms: card ? card.t - from : null, first_card_state: card?.state ?? null };
}

async function loadAndCount(label, name, events, ms, { lang = LANG, vp = VIEWPORT_NAME } = {}) {
    const page = await newPage(label, lang, vp);
    await toFront(page);
    const at = Date.now();
    await page.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(page, 10000, events, { tag: '-' + name });
    await shoot(page, `${name}-10s`, events, { lang, viewport: vp });
    await observe(page, Math.max(ms - (Date.now() - at), 0), events, { tag: '-' + name, shots: false });
    await shoot(page, `${name}-end`, events, { lang, viewport: vp });
    await page.close();
    const ended = Date.now();
    const posts = scanPosts(at, ended);
    const reads = accessReads(label, at, ms);
    return { load_at: iso(at), observed_s: Math.round((ended - at) / 100) / 10, mount: mountOf(label, at),
        access_reads_in_window: reads.length, access_window_s: ms / 1000, access_reads: reads,
        access_reads_whole_page: requests.filter(r => r.page === label && r.url.split('?')[0] === ACCESS).length,
        scan_posts: posts.length, posts: posts.map(brief), first_post_end: posts[0]?.end ?? null,
        posts_within_60s_after_first: posts.length ? scanPosts(posts[0].start_ms + 1, posts[0].end_ms + 60000).length : null,
        observed_after_first_post_s: posts.length ? Math.round((ended - posts[0].end_ms) / 100) / 10 : null,
        states: statesOf(sequenceOf(label, phase)), card_sequence: sequenceOf(label, phase), answers: answers(at, ended) };
}

const BANNER = {
    unread: /Component state could not be read|Bileşen durumu okunamadı/,
    unverified: /Service state needs verification|Servis durumu doğrulanmalı/,
    mayHave: /may have completed|tamamlanmış olabilir/,
};
// The page's alert banners as they stand (text only).
async function bannersOf(page) {
    return page.evaluate(src => {
        const re = Object.fromEntries(Object.entries(src).map(([k, v]) => [k, new RegExp(v)]));
        const all = Array.from(document.querySelectorAll('[role="alert"]')).map(a => (a.innerText || '').replace(/\s+/g, ' ').trim()).filter(Boolean);
        const body = (document.body.innerText || '');
        return { alerts: all, unread: all.filter(t => re.unread.test(t)), unverified_in_page: re.unverified.test(body), may_have_in_page: re.mayHave.test(body) };
    }, Object.fromEntries(Object.entries(BANNER).map(([k, v]) => [k, v.source]))).catch(error => ({ error: String(error.message).slice(0, 120) }));
}

// Wait (bounded) for the "could not be read" banner, scroll it to the middle of the viewport and picture it.
async function shootBanner(page, name, events, limitMs) {
    const deadline = Date.now() + limitMs;
    const from = Date.now();
    while (Date.now() < deadline) {
        const seen = await page.evaluate(src => {
            const re = new RegExp(src);
            const a = Array.from(document.querySelectorAll('[role="alert"]')).find(x => re.test(x.innerText || ''));
            if (!a) return null;
            a.scrollIntoView({ block: 'center' });
            const button = Array.from(a.querySelectorAll('button')).map(b => (b.innerText || '').trim());
            return { text: (a.innerText || '').replace(/\s+/g, ' ').trim(), buttons: button };
        }, BANNER.unread.source).catch(() => null);
        if (seen) {
            await pause(500);
            await page.screenshot({ path: join(dir, `${name}-viewport.png`), fullPage: false });
            await writeFile(join(dir, `${name}.txt`), redact(await pageText(page)));
            const all = await bannersOf(page);
            const value = { at: iso(), after_load_ms: Date.now() - from, banner: seen.text, buttons: seen.buttons, ...all, lang: page.__lang, viewport: page.__vp };
            await jsonl(events, { ev: 'shot', name, ...value });
            return value;
        }
        await pause(100);
    }
    const all = await bannersOf(page);
    await page.screenshot({ path: join(dir, `${name}-no-banner-viewport.png`), fullPage: false });
    await jsonl(events, { at: iso(), ev: 'banner-not-seen', name, limit_ms: limitMs, ...all });
    return { banner: null, ...all };
}

async function componentsPage(label, name, events, lang, vp) {
    const page = await newPage(label, lang, vp);
    await toFront(page);
    const at = Date.now();
    await page.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
    const banner = await shootBanner(page, name, events, 30000);
    await pause(5000);
    await cardLog(page);
    const later = await bannersOf(page);
    return { page, at, value: { load_at: iso(at), banner, banners_5s_later: later,
        posts: scanPosts(at).map(brief), toasts: toastRows.filter(r => r.page === label) } };
}

async function cellUI() {
    const events = join(dir, 'events.jsonl');
    await jsonl(events, { at: iso(), ev: 'start', lang: LANG, other_lang: OTHER_LANG, viewport: VIEWPORT_NAME, chrome: await browser.version() });

    phase = 'sign-in';
    let t = Date.now();
    const signer = await newPage('sign-in');
    await toFront(signer);
    await signIn(signer, events);
    await cardLog(signer);
    await signer.close();
    await phaseDone('sign-in', t);

    // 1a the clock as it is: what "normal" is (a stale load, 90 s)
    phase = 'baseline';
    const stale1 = await waitStale(events, 'baseline');
    const readBase = await clockRead(events, 'baseline');
    t = Date.now();
    const base1 = await loadAndCount('p1-baseline', 'baseline', events, 90000);
    await phaseDone('baseline', t, { stale: { turn: stale1.turn, age_s: stale1.age ?? null }, clock_read: readBase, ...base1 });

    // 3 + 4 the Agent stopped by the owner, the stored scan stale
    phase = 'agent';
    const stale2 = await waitStale(events, 'agent');
    t = Date.now();
    const stopped = await prep('agent-stop', events);
    const reads = await prep('agent-reads', events);
    const admission = await prep('admission', events);
    const dashLabel = 'p2-agent-dashboard-' + LANG;
    const p2 = await newPage(dashLabel);
    await toFront(p2);
    const dashAt = Date.now();
    await p2.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(p2, 30000, events, { tag: '-dashboard-' + LANG });
    await p2.screenshot({ path: join(dir, `agent-dashboard-${LANG}-full.png`), fullPage: true });
    await writeFile(join(dir, `agent-dashboard-${LANG}-full.txt`), redact(await pageText(p2)));
    await jsonl(events, { at: iso(), ev: 'shot', name: `agent-dashboard-${LANG}-full`, full_page: true, card: (await cardNow(p2)).text });
    await observe(p2, 10000, events, { tag: '-dashboard-' + LANG, shots: false });
    await p2.close();
    const dashEnded = Date.now();
    const compEn = await componentsPage('p3-components-' + LANG + '-desktop', `agent-components-${LANG}-desktop-banner`, events, LANG, 'desktop');
    await compEn.page.close();
    const compTr = await componentsPage('p4-components-' + OTHER_LANG + '-phone', `agent-components-${OTHER_LANG}-phone-banner`, events, OTHER_LANG, 'phone');
    const p4 = compTr.page;
    const started = await prep('agent-start', events);
    const quietFrom = Date.now();
    await pause(5000);
    const sentByItself = scanPosts(quietFrom).length;
    const clickAt = Date.now();
    const clicked = await p4.evaluate(src => {
        const re = new RegExp(src);
        const a = Array.from(document.querySelectorAll('[role="alert"]')).find(x => re.test(x.innerText || ''));
        const button = a ? a.querySelector('button') : null;
        if (!button) return null;
        const label = (button.innerText || '').trim();
        button.click();
        return label;
    }, BANNER.unread.source).catch(error => ({ error: String(error.message).slice(0, 120) }));
    await jsonl(events, { at: iso(), ev: 'click', what: 'banner-rescan', clicked });
    let after = null;
    for (let i = 0; i < 300; i++) {
        await pause(100);
        const done = scanPosts(clickAt).filter(r => r.status !== null);
        after = await bannersOf(p4);
        if (done.length && !(after.unread || []).length) break;
    }
    const goneAt = Date.now();
    await pause(1500);
    await p4.evaluate(() => window.scrollTo(0, 0)).catch(() => null);
    await pause(400);
    await p4.screenshot({ path: join(dir, 'agent-components-after-rescan-viewport.png'), fullPage: false });
    await p4.screenshot({ path: join(dir, 'agent-components-after-rescan-full.png'), fullPage: true });
    await writeFile(join(dir, 'agent-components-after-rescan.txt'), redact(await pageText(p4)));
    await pause(10000);
    await cardLog(p4);
    const after10 = await bannersOf(p4);
    await p4.close();
    const clickPosts = scanPosts(clickAt);
    await phaseDone('agent', t, { stale: { turn: stale2.turn, age_s: stale2.age ?? null }, agent_stop: stopped.result, agent_reads: reads.result,
        admission: admission.result, agent_start: started.result,
        dashboard: { load_at: iso(dashAt), posts: scanPosts(dashAt, dashEnded).map(brief), states: statesOf(sequenceOf(dashLabel, 'agent')),
            card_sequence: sequenceOf(dashLabel, 'agent'), page_reads: answers(dashAt, dashEnded).length,
            agent_reads_of_the_page: requests.filter(r => r.page === dashLabel && r.agent_read).map(r => ({ url: r.url, status: r.status, agent_read: r.agent_read })) },
        components_first: compEn.value, components_other: compTr.value,
        posts_sent_by_itself_after_agent_start_5s: sentByItself, clicked, click_at: iso(clickAt),
        banner_gone_ms_after_click: (after && !(after.unread || []).length) ? goneAt - clickAt : null, banners_after_click: after, banners_10s_later: after10,
        posts_after_click: clickPosts.length, posts_after_click_list: clickPosts.map(brief),
        page_agent_reads: requests.filter(r => r.agent_read && r.start_ms >= t).map(r => ({ page: r.page, url: r.url, status: r.status, agent_read: r.agent_read })),
        toasts: toastRows.filter(r => r.phase === 'agent') });

    // 1b the guest clock about 7 minutes behind this browser
    phase = 'clock-behind';
    const e0 = await prep('probe', events);
    const back = await prep('clockshift_m420000', events);
    const readBehind = await clockRead(events, 'behind');
    const owner = await prep('scan-now', events);
    const stale3 = await waitStale(events, 'clock-behind');
    const readBehind2 = await clockRead(events, 'behind-at-load');
    t = Date.now();
    const behind = await loadAndCount('p5-behind', 'behind', events, 90000);
    await phaseDone('clock-behind', t, { offset_ms_probe_before_shift: offsetOf(e0), shift_back: back.result, clock_read_after_shift: readBehind,
        owner_scan: owner.result, stale: { turn: stale3.turn, age_s: stale3.age ?? null }, clock_read_at_load: readBehind2, ...behind });

    // 1c the guest clock about 7 minutes ahead of this browser
    phase = 'clock-ahead';
    const fwd = await prep('clockshift_p840000', events);
    const readAhead = await clockRead(events, 'ahead');
    const stale4 = await waitStale(events, 'clock-ahead');
    t = Date.now();
    const ahead = await loadAndCount('p6-ahead', 'ahead', events, 90000);
    const readAhead2 = await clockRead(events, 'ahead-after-load');
    await phaseDone('clock-ahead', t, { shift_forward: fwd.result, clock_read_after_shift: readAhead, stale: { turn: stale4.turn, age_s: stale4.age ?? null },
        clock_read_after_load: readAhead2, ...ahead });

    // the clock put back: 420 s back, NTP on
    phase = 'restore';
    t = Date.now();
    const undo = await prep('clockshift_m420000', events);
    const ntp = await prep('ntp-on', events);
    const readRestored = await clockRead(events, 'restored');
    await pause(30000);
    const readRestored30 = await clockRead(events, 'restored-30s');
    const last = await prep('probe', events);
    await phaseDone('restore', t, { shift_back: undo.result, ntp_on: ntp.result, clock_read_restored: readRestored, clock_read_restored_30s: readRestored30,
        probe_clock: last.result?.clock ?? null, offset_ms_probe: offsetOf(last) });

    phase = 'end';
    await prep('journal', events);
}

const results = {};
for (const cell of cells) {
    dir = join(outRoot, cell);
    await mkdir(dir, { recursive: true });
    const started = iso();
    try {
        if (cell === 'ui') await cellUI();
        else if (cell === 'done') await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set14c.mjs' }) + '\n');
        else throw new Error(`unknown cell ${cell}`);
        results[cell] = { started, ended: iso(), ok: true };
    } catch (error) {
        results[cell] = { started, ended: iso(), ok: false, phase, error: redact(String(error.stack || error)).slice(0, 1500) };
    }
    await writeFile(join(dir, 'run.json'), redact(JSON.stringify({ cell, chrome: await browser.version(), lang: LANG, viewport: VIEWPORT_NAME,
        handover: { ...hand, password_file: '(root-only file, not copied)' }, ...results[cell] }, null, 2)));
    console.log(JSON.stringify({ cell, ...results[cell] }));
}
await browser.close();
