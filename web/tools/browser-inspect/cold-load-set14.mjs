// node cold-load-set14.mjs <handover-dir> <out-dir> <cell[,cell...]>
// set14 (2026-10-12): commit a5830dd3e's Dashboard "System services" card, which checks the components by itself
// (POST /api/v1/managed-services/scan?max_age_seconds=300), in a real headless Chrome against a REAL CelikPanel on a
// disposable lab guest, reached through the lab driver's loopback SSH forward (deploy/e2e/release-recovery/
// set14_trial.py, set12's browser window "ui"). Nothing but the loopback address of the forward is opened. The driver
// prepares each state on request (prep-request-<name>-<n> -> prep-done) and reads the guest (probe).
//
//   cells: ui    phases, in this order (the language and viewport come from the handover's "browser"):
//                1 stale-load   the stored scan older than 315 s (guest clock) or absent; one Dashboard load
//                2 fresh-load   a second load within the window
//                3 window       that page kept open and visible past the window
//                4 hidden       that page hidden (another tab brought to the front) past the window, then shown
//                5 failure      the Agent stopped by the owner; a stale load; the Agent started; "Scan services"
//                6 components   the Components page (/services) once after the same stale condition
//                7 held         the owner reloads nginx from the Panel while its reload takes 90 s (lab); a stale
//                               Dashboard load meanwhile; the follow-up after the host is ready again
//          done  writes done.json
//
//   BROWSER_INSPECT_MODULES  directory whose node_modules holds puppeteer-core (required; outside the repository)
//   CHROME_PATH              the installed Chrome (default: the usual Windows/Linux places)
//   LIVE_PROFILE_ROOT        a short directory for the browser profile (profile-dir.mjs; refused beyond 120 characters)
//
// Every request to /api/ is recorded (method, path with query, status, start and end time, duration); for the scan
// POST also the answer's code and scanned_at. The card's text is sampled in the page every 100 ms (read only) and
// every change is recorded with the page's visibility. Pictures are never taken of a page while it is hidden.
// The owner's password is read from the root-only file the driver names and typed into the sign-in form; every text
// this script writes is passed through the redaction list of that file first. Cookies are never read.
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
if (!handDir || !outRoot) { console.error('usage: node cold-load-set14.mjs <handover-dir> <out-dir> <cells>'); process.exit(2); }
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
const VIEWPORT_NAME = hand.browser?.viewport === 'phone' ? 'phone' : 'desktop';
const VIEWPORT = VIEWPORT_NAME === 'phone' ? PHONE : DESKTOP;
const WINDOW_S = 300;
const STALE_S = 315;
// The card's own strings (web/src/i18n, commit a5830dd3e), to name what the card says.
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
    document.addEventListener('visibilitychange', () => { window.__set14card.push({ t: Date.now(), ev: 'visibility', vis: document.visibilityState }); tick(); });
})();`;

async function newPage(label) {
    const page = await browser.newPage();
    page.__label = label;
    await page.setViewport(VIEWPORT);
    await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }]);
    await page.evaluateOnNewDocument(value => { try { localStorage.setItem('celikpanel.lang', value); localStorage.setItem('celikpanel.theme', 'light'); } catch { /* none */ } }, LANG);
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
        if (res && req.method() === 'POST' && entry.url.startsWith('/api/v1/managed-services/scan')) {
            try {
                const body = JSON.parse(await res.text());
                entry.answer = { code: body?.code ?? null, scanned_at: body?.scanned_at ?? null, services: Array.isArray(body?.services) ? body.services.length : null,
                    error: typeof body?.error === 'string' ? body.error.slice(0, 300) : null };
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

async function cardLog(page) {
    const rows = await page.evaluate(() => (window.__set14card || []).splice(0)).catch(() => []);
    for (const row of rows) await jsonl(join(dir, 'cardlog.jsonl'), { page: page.__label, phase, at: iso(row.t), ...row, state: row.text === undefined ? undefined : stateOf(row.text) });
    return rows;
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
            await shoot(page, `${phase}${tag}-${state}`, events, { lang: LANG, viewport: VIEWPORT_NAME });
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
    for (let i = 0; i < 900 && !existsSync(join(handDir, `prep-done-${key}.json`)); i++) await pause(1000);
    const record = existsSync(join(handDir, `prep-done-${key}.json`)) ? JSON.parse(readFileSync(join(handDir, `prep-done-${key}.json`), 'utf8')) : { missing: true };
    const got = Date.now();
    const out = { ...record, host_asked_ms: asked, host_got_ms: got };
    const guest = record?.result?.epoch;
    if (typeof guest === 'number') out.guest_minus_host_ms_approx = Math.round(guest * 1000 - (asked + got) / 2);
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

async function cellUI() {
    const events = join(dir, 'events.jsonl');
    await jsonl(events, { at: iso(), ev: 'start', lang: LANG, viewport: VIEWPORT_NAME, chrome: await browser.version() });

    phase = 'sign-in';
    let t = Date.now();
    const signer = await newPage('sign-in');
    await toFront(signer);
    await signIn(signer, events);
    await cardLog(signer);
    await signer.close();
    await phaseDone('sign-in', t);

    // 1 stale load
    phase = 'stale-load';
    const stale1 = await waitStale(events, 'stale-load');
    const a = await prep('probe', events);
    t = Date.now();
    const p1 = await newPage('p1-stale');
    await toFront(p1);
    await p1.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(p1, 45000, events);
    const b = await prep('probe', events);
    await p1.close();
    await phaseDone('stale-load', t, { stale: { turn: stale1.turn, age_s: stale1.age ?? null, absent: !!stale1.absent }, cpu: cpu(a, b), probes: [a.result?.file, b.result?.file] });

    // 2 fresh load, the page kept for 3 and 4
    phase = 'fresh-load';
    const c = await prep('probe', events);
    t = Date.now();
    const p2 = await newPage('p2-fresh');
    await toFront(p2);
    await p2.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(p2, 45000, events);
    const d = await prep('probe', events);
    await phaseDone('fresh-load', t, { scan_age_at_start_s: c.result?.scan_cache?.age_seconds, cpu: cpu(c, d), probes: [c.result?.file, d.result?.file] });

    // 3 window: p2 stays open and visible until the stored scan is 375 s old
    phase = 'window';
    t = Date.now();
    const ageD = Number(d.result?.scan_cache?.age_seconds);
    const windowMs = Math.min(Math.max((WINDOW_S + 75 - (Number.isFinite(ageD) ? ageD : 0)) * 1000, 30000), 420000);
    const scannedBefore = d.result?.scan_cache?.scanned_epoch;
    await observe(p2, windowMs, events);
    const e = await prep('probe', events);
    await phaseDone('window', t, { observed_ms: windowMs, scan_epoch_before: scannedBefore, scan_epoch_after: e.result?.scan_cache?.scanned_epoch,
        due_epoch: typeof scannedBefore === 'number' ? scannedBefore + WINDOW_S : null });

    // 4 hidden: another tab in front until the stored scan is 360 s old, then p2 shown again
    phase = 'hidden';
    t = Date.now();
    const blank = await newPage('p3-blank');
    await toFront(blank);
    await blank.goto('about:blank');
    const hiddenState = await cardNow(p2);
    const ageE = Number(e.result?.scan_cache?.age_seconds);
    const hideMs = Math.min(Math.max((WINDOW_S + 60 - (Number.isFinite(ageE) ? ageE : 0)) * 1000, 30000), 420000);
    await observe(p2, hideMs, events, { shots: false });
    const f = await prep('probe', events);
    const shownAt = Date.now();
    await toFront(p2);
    await observe(p2, 40000, events, { tag: '-shown' });
    await blank.close();
    const g = await prep('probe', events);
    await p2.close();
    await phaseDone('hidden', t, { hidden_ms: shownAt - t, visibility_while_hidden: hiddenState.vis, posts_while_hidden: scanPosts(t, shownAt).length,
        posts_after_shown: scanPosts(shownAt).length, shown_at: iso(shownAt), scan_age_when_shown_s: f.result?.scan_cache?.age_seconds,
        scan_epoch_after: g.result?.scan_cache?.scanned_epoch });

    // 5 failure: the Agent stopped; a stale load; started; the button
    phase = 'failure';
    await waitStale(events, 'failure');
    t = Date.now();
    const stopped = await prep('agent-stop', events);
    const p4 = await newPage('p4-failure');
    await toFront(p4);
    const loadAt = Date.now();
    await p4.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await observe(p4, 40000, events);
    const started = await prep('agent-start', events);
    await pause(5000);
    await observe(p4, 10000, events, { tag: '-agent-back' });
    const clickAt = Date.now();
    const clicked = await p4.evaluate(label => {
        const c = window.__set14find ? window.__set14find() : null;
        const button = c ? Array.from(c.querySelectorAll('button')).find(b => (b.innerText || '').trim() === label) : null;
        if (!button) return null;
        button.click();
        return label;
    }, TEXT.scanNow[LANG]);
    await jsonl(events, { at: iso(), ev: 'click', what: 'scan-services', clicked });
    await observe(p4, 40000, events, { tag: '-after-click' });
    const h = await prep('probe', events);
    await p4.close();
    await phaseDone('failure', t, { agent_stop: stopped.result, agent_start: started.result, clicked, load_at: iso(loadAt), click_at: iso(clickAt),
        posts_before_click: scanPosts(loadAt, clickAt).length, posts_after_click: scanPosts(clickAt).length, probe_after: h.result?.file });

    // 6 the Components page once, stale
    phase = 'components';
    await waitStale(events, 'components');
    t = Date.now();
    const p5 = await newPage('p5-components');
    await toFront(p5);
    await p5.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await pause(1500);
    await shoot(p5, 'components-loaded', events);
    await pause(43000);
    await shoot(p5, 'components-45s', events);
    await p5.close();
    await phaseDone('components', t);

    // 7 held: a Panel operation holds the change lease while the Dashboard loads stale
    phase = 'held';
    await waitStale(events, 'held');
    t = Date.now();
    const slow = await prep('slow-on', events);
    const action = await prep('action-start', events);
    const p6 = await newPage('p6-held');
    await toFront(p6);
    const heldLoad = Date.now();
    await p6.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    let waited = null; let waitedAt = null;
    const waiting = prep('action-wait', events).then(value => { waited = value; waitedAt = Date.now(); });
    await observe(p6, 300000, events, { until: () => waitedAt !== null && Date.now() - waitedAt > 45000 });
    await waiting;
    const off = await prep('slow-off', events);
    const k = await prep('probe', events);
    await p6.close();
    await phaseDone('held', t, { slow_on: slow.result, action_start: action.result, action_end: waited?.result, action_end_seen: waitedAt ? iso(waitedAt) : null,
        load_at: iso(heldLoad), posts_while_held: scanPosts(heldLoad, waitedAt ?? Date.now()).length, posts_after_ready: waitedAt ? scanPosts(waitedAt).length : null,
        slow_off: off.result, probe_after: k.result?.file });

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
        else if (cell === 'done') await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set14.mjs' }) + '\n');
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
