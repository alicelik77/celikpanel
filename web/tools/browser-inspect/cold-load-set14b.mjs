// node cold-load-set14b.mjs <handover-dir> <out-dir> <cell[,cell...]>
// set14b (2026-10-13): the short follow-up of set14 on commit 1a74eeda5 (after e6de64a5b): the managed-services answers
// carry scan_age_seconds and the Dashboard and the Components page date a scan from it; an unreachable Agent answers
// 503 AGENT_UNREACHABLE. A real headless Chrome against a REAL CelikPanel on a disposable lab guest, reached through the
// lab driver's loopback SSH forward (deploy/e2e/release-recovery/set14b_trial.py, set12's browser window "ui"). Nothing
// but the loopback address of the forward is opened. The driver prepares each state on request (prep-request-<name>-<n>
// -> prep-done) and reads the guest (probe). Copied from cold-load-set14.mjs; the recording is the same, plus: the
// card is also read on every DOM change (MutationObserver), alert toasts are recorded, and the answers of
// GET /api/v1/managed-services and POST /api/v1/managed-services/scan are recorded with scanned_at, scan_age_seconds
// (and its JSON type) and, when refused, the whole body.
//
//   cells: ui    phases, in this order:
//                1 stale-load    the stored scan older than 315 s (guest clock); the guest clock 1-2 s ahead of this
//                                browser (moved there only if it is not); one Dashboard load, 45 s
//                2 fresh-load    a second load, 45 s
//                3 clock-behind  the guest clock moved 420 s back; the owner's unconditional scan; the stored scan
//                                older than 315 s on the guest clock; one Dashboard load, 90 s; the clock moved 420 s
//                                forward again and NTP switched back on
//                4 components    the Components page (/services) once, stale, 45 s
//                5 agent         the Agent stopped by the owner, stale: Dashboard (first language) 40 s; Components page
//                                with its toast; the owner's nginx restart through the API (admission); Dashboard (the
//                                other language); the Agent started; "Scan services" pressed there
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
if (!handDir || !outRoot) { console.error('usage: node cold-load-set14b.mjs <handover-dir> <out-dir> <cells>'); process.exit(2); }
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
// The card's own strings (web/src/i18n, commit 1a74eeda5), to name what the card says.
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

async function newPage(label, lang = LANG) {
    const page = await browser.newPage();
    page.__label = label;
    page.__lang = lang;
    await page.setViewport(VIEWPORT);
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
            await shoot(page, `${phase}${tag}-${state}`, events, { lang: page.__lang, viewport: VIEWPORT_NAME });
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

    // 1 stale load, the guest clock 1-2 s ahead of this browser (D1 of set14)
    phase = 'stale-load';
    const stale1 = await waitStale(events, 'stale-load');
    const a = await prep('probe', events);
    let moved = null;
    const read0 = await clockRead(events, 'stale-before');
    const off0 = read0 ? read0.mid : offsetOf(a);
    if (typeof off0 === 'number' && (off0 < 1000 || off0 > 2200)) {
        const shift = 1500 - off0;
        moved = await prep(`clockshift_${shift >= 0 ? 'p' : 'm'}${Math.abs(shift)}`, events);
    }
    const read1 = moved ? await clockRead(events, 'stale-after-move') : read0;
    t = Date.now();
    const p1 = await newPage('p1-stale');
    await toFront(p1);
    await p1.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(p1, 45000, events);
    const b = await prep('probe', events);
    await p1.close();
    const seq1 = sequenceOf('p1-stale', 'stale-load');
    await phaseDone('stale-load', t, { stale: { turn: stale1.turn, age_s: stale1.age ?? null, absent: !!stale1.absent },
        clock_read_first: read0, clock_moved: moved ? moved.result : null, clock_read_at_load: read1, offset_ms_probe_before: offsetOf(a), offset_ms_probe_after: offsetOf(b),
        scan_before: scanOf(a), states: statesOf(seq1), card_sequence: seq1, answers: answers(t), probes: [a.result?.file, b.result?.file] });

    // 2 fresh load
    phase = 'fresh-load';
    const c = await prep('probe', events);
    t = Date.now();
    const p2 = await newPage('p2-fresh');
    await toFront(p2);
    await p2.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(p2, 45000, events);
    const d = await prep('probe', events);
    await p2.close();
    const seq2 = sequenceOf('p2-fresh', 'fresh-load');
    await phaseDone('fresh-load', t, { scan_age_at_start_s: c.result?.scan_cache?.age_seconds, offset_ms_before: offsetOf(c), offset_ms_after: offsetOf(d),
        states: statesOf(seq2), card_sequence: seq2, answers: answers(t), probes: [c.result?.file, d.result?.file] });

    // 3 the guest clock about 7 minutes behind this browser; a scan that is fresh on the server's clock must read fresh
    phase = 'clock-behind';
    const e0 = await prep('probe', events);
    const back = await prep('clockshift_m420000', events);
    const e1 = await prep('probe', events);
    const readBehind = await clockRead(events, 'behind');
    const owner = await prep('scan-now', events);
    const stale3 = await waitStale(events, 'clock-behind');
    const e2 = await prep('probe', events);
    t = Date.now();
    const p3 = await newPage('p3-behind');
    await toFront(p3);
    await p3.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(p3, 90000, events);
    const e3 = await prep('probe', events);
    await p3.close();
    const behindEnded = Date.now();
    const forward = await prep('clockshift_p420000', events);
    const ntp = await prep('ntp-on', events);
    const e4 = await prep('probe', events);
    const readRestored = await clockRead(events, 'restored');
    const seq3 = sequenceOf('p3-behind', 'clock-behind');
    const posts3 = scanPosts(t, behindEnded);
    await phaseDone('clock-behind', t, { offset_ms_before_shift: offsetOf(e0), shift_back: back.result, offset_ms_after_shift: offsetOf(e1),
        owner_scan: owner.result, stale: { turn: stale3.turn, age_s: stale3.age ?? null }, offset_ms_at_load: offsetOf(e2), offset_ms_at_end: offsetOf(e3),
        first_post_end: posts3[0]?.end ?? null, posts_after_first: posts3.length ? scanPosts(posts3[0].start_ms + 1, behindEnded).length : null,
        observed_after_first_post_s: posts3.length ? Math.round((behindEnded - posts3[0].end_ms) / 100) / 10 : null,
        clock_read_behind: readBehind, shift_forward: forward.result, ntp_on: ntp.result, offset_ms_after_restore: offsetOf(e4), clock_read_restored: readRestored,
        states: statesOf(seq3), card_sequence: seq3, answers: answers(t), probes: [e0.result?.file, e1.result?.file, e2.result?.file, e3.result?.file, e4.result?.file] });

    // 4 the Components page once, stale
    phase = 'components';
    const stale4 = await waitStale(events, 'components');
    const f0 = await prep('probe', events);
    t = Date.now();
    const p4 = await newPage('p4-components');
    await toFront(p4);
    await p4.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await pause(1500);
    await shoot(p4, 'components-loaded', events);
    await pause(43000);
    await cardLog(p4);
    await shoot(p4, 'components-45s', events);
    await p4.close();
    await phaseDone('components', t, { stale: { turn: stale4.turn, age_s: stale4.age ?? null }, offset_ms: offsetOf(f0), answers: answers(t) });

    // 5 the Agent stopped by the owner
    phase = 'agent';
    const stale5 = await waitStale(events, 'agent');
    t = Date.now();
    const stopped = await prep('agent-stop', events);
    const p5 = await newPage('p5-agent-' + LANG);
    await toFront(p5);
    const loadEn = Date.now();
    await p5.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(p5, 40000, events, { tag: '-' + LANG });
    await p5.close();
    const loadComponents = Date.now();
    const p6 = await newPage('p6-agent-components');
    await toFront(p6);
    await p6.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
    const toast = await shootToast(p6, 'agent-components-toast', events, 30000);
    await pause(5000);
    await cardLog(p6);
    await shoot(p6, 'agent-components-after', events);
    await p6.close();
    const admitAt = Date.now();
    const admission = await prep('admission', events);
    const loadOther = Date.now();
    const p7 = await newPage('p7-agent-' + OTHER_LANG, OTHER_LANG);
    await toFront(p7);
    await p7.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(p7, 30000, events, { tag: '-' + OTHER_LANG });
    const started = await prep('agent-start', events);
    await pause(5000);
    await observe(p7, 10000, events, { tag: '-agent-back' });
    const clickAt = Date.now();
    const scanLabel = TEXT.scanNow[OTHER_LANG];
    const clicked = await p7.evaluate(label => {
        const c = window.__set14find ? window.__set14find() : null;
        const button = c ? Array.from(c.querySelectorAll('button')).find(x => (x.innerText || '').trim() === label) : null;
        if (!button) return null;
        button.click();
        return label;
    }, scanLabel);
    await jsonl(events, { at: iso(), ev: 'click', what: 'scan-services', clicked });
    await observe(p7, 40000, events, { tag: '-after-click' });
    const h = await prep('probe', events);
    await p7.close();
    await phaseDone('agent', t, { stale: { turn: stale5.turn, age_s: stale5.age ?? null }, agent_stop: stopped.result, agent_start: started.result,
        posts_dashboard_first_lang: scanPosts(loadEn, loadComponents).length, posts_components: scanPosts(loadComponents, admitAt).length,
        posts_dashboard_other_lang_before_click: scanPosts(loadOther, clickAt).length, posts_after_click: scanPosts(clickAt).length,
        toast, admission: admission.result, clicked, click_at: iso(clickAt),
        states_first_lang: statesOf(sequenceOf('p5-agent-' + LANG, 'agent')), states_other_lang: statesOf(sequenceOf('p7-agent-' + OTHER_LANG, 'agent')),
        card_sequence_first_lang: sequenceOf('p5-agent-' + LANG, 'agent'), card_sequence_other_lang: sequenceOf('p7-agent-' + OTHER_LANG, 'agent'),
        toasts: toastRows.filter(r => r.phase === 'agent'), answers: answers(t), probe_after: h.result?.file });

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
        else if (cell === 'done') await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set14b.mjs' }) + '\n');
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
