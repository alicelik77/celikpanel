// node cold-load-set16.mjs <handover-dir> <out-dir> <cell[,cell...]>
// set16 (2026-10-14): the pre-publication measurement of the v0.1.0-alpha.85 candidate (commit 89cf88397). A real
// headless Chrome against a REAL CelikPanel on a disposable lab guest, reached through the lab driver's loopback SSH
// forward (deploy/e2e/release-recovery/set16_trial.py, set12's browser window "ui"), after a fresh install or after
// the owner's update (or its automatic return). Nothing but the loopback address of the forward is opened. The
// recording is cold-load-set15.mjs's, copied (every /api/ request with its times; the answers of GET
// /api/v1/managed-services, POST /api/v1/managed-services/scan and GET /api/v1/license/access with valid_for_seconds;
// the bodies of GET /api/v1/domains and GET /api/v1/panel/version), plus the Dashboard's Mail stacks card text every
// 100 ms, the Dashboard's "Needs attention" section, and the bodies of failed GET /api/v1/firewall reads.
//
//   cells: ui    phases, in this order (hand.mode "fresh"; after an update only the ones marked *):
//                sign-in *      the owner signs in (password from the root-only file the driver names)
//                domains *      the Domains list (desktop): page text, picture, the sidebar's text and picture
//                sidebar *      the sidebar build id at 1280x800 and at 390x844 (the menu opened): whether the
//                               12-character id is drawn on one line (its client rectangles), whether "build" and
//                               the id share a line, whether the unit overflows the sidebar; a picture of each
//                dashboard *    the stored scan older than 315 s (probe); one Dashboard load, 90 s: the scan POSTs,
//                               the System services card's states and the Mail stacks card's states
//                components *   the Components page right after (no POST expected), then (fresh only) with the stored
//                               scan older than 315 s
//                clock          lab preparation: the guest clock moved +90 s, then to -90 s, against its own reading
//                               (NTP off); at each, one Dashboard load watched for 75 s (licence access reads with
//                               valid_for_seconds, the mount, the card) and the offset read from the answers; then
//                               the clock moved back by the same total and NTP switched on
//                agent          the owner stops the Agent; the Components page in Turkish (the firewall banner's text,
//                               40 s), the Dashboard in Turkish ("İlgi istiyor" and the cards, 40 s) and in English
//                               (40 s); the owner starts the Agent again; on the still-open Turkish Components page
//                               "Yeniden tara" is pressed once, and the banner is read again for 20 s
//                reads *        three direct reads in a page that is not the app: /api/v1/panel/version,
//                               /api/v1/license/access, /api/v1/managed-services (bodies recorded)
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
if (!handDir || !outRoot) { console.error('usage: node cold-load-set16.mjs <handover-dir> <out-dir> <cells>'); process.exit(2); }
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
    // set16: the Dashboard's Mail stacks card (section labelled by its heading), every 100 ms and on each change.
    window.__set16mail = []; let lastMail = null;
    const mailTick = () => {
        const m = document.querySelector('section[aria-labelledby="dashboard-mail-stacks-heading"]');
        const text = m ? m.innerText.replace(/\\s+/g, ' ').trim() : null;
        const spin = m ? m.querySelectorAll('[role="status"], .animate-spin').length : null;
        const key = text + '|' + spin + '|' + document.visibilityState;
        if (key !== lastMail) { lastMail = key; window.__set16mail.push({ t: Date.now(), text, spinners: spin, vis: document.visibilityState, path: location.pathname }); }
    };
    setInterval(mailTick, 100);
    const watchMail = () => { if (!document.documentElement) return setTimeout(watchMail, 10); new MutationObserver(mailTick).observe(document.documentElement, { subtree: true, childList: true, characterData: true }); };
    watchMail();
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
        if (res && req.method() === 'GET' && ['/api/v1/domains', '/api/v1/panel/version'].includes(path0)) {
            try { entry.body = (await res.text()).slice(0, 3000); } catch (error) { entry.body_unreadable = String(error.message).slice(0, 120); }
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
// set16: the Mail stacks card's states (web/src/i18n/screens/{en,tr}.ts, dashboard.mailStacks.* and
// dashboard.systemServicesChecking at commit 0bea00fa0). The order matters: checking first.
const MAIL_STATES = [
    ['checking', /Checking components|Bileşenler kontrol ediliyor|Checking…|Kontrol ediliyor…/],
    ['check-failed', /could not be checked just now|şu anda kontrol edilemedi/],
    ['never', /Nothing on this server has been checked yet|henüz hiçbir şeye bakılmadı/],
    ['stale', /Not checked recently|Yakın zamanda kontrol edilmedi/],
    ['held', /change is in progress|değişiklik sürüyor/],
    ['ready', /\bReady\b(?! to set up)|\bHazır\b/],
    ['available', /Ready to set up|Kuruluma hazır/],
    ['partial', /Partially installed|Kısmen kurulu/],
    ['attention', /Needs attention|İlgi istiyor/],
];
const OLD_MAIL = /Open Components to rescan|rescan before trusting|Status needs refresh/;
const mailStateOf = (text, spinners) => {
    if (!text) return 'absent';
    for (const [name, re] of MAIL_STATES) if (re.test(text)) return name + (name === 'checking' && !spinners ? '-no-spinner' : '');
    return 'other';
};
const mailRows = [];
function mailSequenceOf(label, name) {
    const rows = mailRows.filter(r => r.page === label && r.phase === name);
    return rows.map((r, i) => ({ at: r.at, state: r.state, spinners: r.spinners, vis: r.vis,
        lasted_ms: rows[i + 1] ? rows[i + 1].t - r.t : null, old_wording: OLD_MAIL.test(r.text || ''), text: r.text }));
}
async function cardLog(page) {
    const rows = await page.evaluate(() => (window.__set14card || []).splice(0)).catch(() => []);
    for (const row of rows) {
        const value = { page: page.__label, lang: page.__lang, phase, at: iso(row.t), ...row, state: row.text === undefined ? undefined : stateOf(row.text) };
        cardRows.push(value);
        await jsonl(join(dir, 'cardlog.jsonl'), value);
    }
    const mails = await page.evaluate(() => (window.__set16mail || []).splice(0)).catch(() => []);
    for (const row of mails) {
        const value = { page: page.__label, lang: page.__lang, phase, at: iso(row.t), ...row, state: mailStateOf(row.text, row.spinners) };
        mailRows.push(value);
        await jsonl(join(dir, 'maillog.jsonl'), value);
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


// -- set15: the Domains list, the sidebar, the Dashboard after a stale scan, the Components page, three reads ------

const FIRST_STATE = /Site configuration files: .*recognised as CelikPanel|Site yapılandırma dosyaları: .*devralındı/;
const RESCAN = /Open Components to rescan|Yeniden taramak için Bileşenleri aç/;

// The sidebar (the app's <aside>): its text, its footer stamp (the line with "build"), a picture of the element.
async function sidebar(page, name, events) {
    const facts = await page.evaluate(() => {
        const aside = document.querySelector('aside');
        if (!aside) return { aside: false };
        const lines = (aside.innerText || '').split('\n').map(x => x.replace(/\s+/g, ' ').trim()).filter(Boolean);
        return { aside: true, text: lines.join('\n'), stamp: lines.filter(x => / build /.test(x) || /\bbuild\b/.test(x)) };
    }).catch(error => ({ error: String(error.message).slice(0, 120) }));
    const handle = await page.$('aside').catch(() => null);
    let pictured = false;
    if (handle && front.page === page) { try { await handle.screenshot({ path: join(dir, `${name}-sidebar.png`) }); pictured = true; } catch { /* moved */ } }
    await writeFile(join(dir, `${name}-sidebar.txt`), redact(facts.text || ''));
    const value = { at: iso(), ...facts, picture: pictured ? `${name}-sidebar.png` : null };
    await jsonl(events, { ev: 'sidebar', name, ...value });
    return value;
}

async function domainsPage(events) {
    const label = 'p1-domains';
    const page = await newPage(label);
    await toFront(page);
    const at = Date.now();
    await page.goto(base + '/domains', { waitUntil: 'networkidle2', timeout: 60000 });
    await pause(4000);
    const text = await pageText(page);
    await page.screenshot({ path: join(dir, 'domains-viewport.png'), fullPage: false });
    await writeFile(join(dir, 'domains.txt'), redact(text));
    const bar = await sidebar(page, 'domains', events);
    await cardLog(page);
    await page.close();
    const rows = requests.filter(r => r.page === label && r.method === 'GET' && r.url.split('?')[0] === '/api/v1/domains')
        .map(r => ({ status: r.status, start: r.start, ms: r.ms, body: r.body ?? null }));
    return { load_at: iso(at), first_state_row_shown: FIRST_STATE.test(text), page_text_head: redact(text).slice(0, 1200),
        domains_reads: rows, scan_posts: scanPosts(at).length, sidebar: bar };
}

async function dashboardLoad(label, name, events, ms, lang = LANG) {
    const page = await newPage(label, lang);
    await toFront(page);
    const at = Date.now();
    await page.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
    const rescanSeen = [];
    const watch = async () => { if (await page.evaluate(src => new RegExp(src).test(document.body.innerText || ''), RESCAN.source).catch(() => false)) rescanSeen.push(iso()); };
    await observe(page, 10000, events, { tag: '-' + name });
    await watch();
    await shoot(page, `${name}-10s`, events, { lang, viewport: VIEWPORT_NAME });
    while (Date.now() - at < ms) {
        await observe(page, Math.min(5000, Math.max(ms - (Date.now() - at), 0)), events, { tag: '-' + name, shots: false });
        await watch();
    }
    await shoot(page, `${name}-end`, events, { lang, viewport: VIEWPORT_NAME });
    await page.screenshot({ path: join(dir, `${name}-full.png`), fullPage: true });
    await writeFile(join(dir, `${name}-full.txt`), redact(await pageText(page)));
    const bar = await sidebar(page, name, events);
    await page.close();
    const ended = Date.now();
    const posts = scanPosts(at, ended);
    const reads = accessReads(label, at, ms);
    return { load_at: iso(at), observed_s: Math.round((ended - at) / 100) / 10, mount: mountOf(label, at),
        scan_posts: posts.length, posts: posts.map(brief),
        posts_conditional: posts.filter(p => /max_age_seconds=/.test(p.url)).length,
        rescan_link_seen: rescanSeen.length > 0, rescan_link_seen_at: rescanSeen.slice(0, 5), rescan_link_checks_note: 'body text sampled every ~5 s and at 10 s',
        access_reads: reads, states: statesOf(sequenceOf(label, phase)), card_sequence: sequenceOf(label, phase),
        mail_states: statesOf(mailSequenceOf(label, phase)), mail_sequence: mailSequenceOf(label, phase),
        answers: answers(at, ended), sidebar: bar };
}

async function componentsLoad(events, tag, lang = LANG) {
    const label = 'p3-components-' + tag;
    const page = await newPage(label, lang);
    await toFront(page);
    const at = Date.now();
    await page.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await pause(30000);
    await page.screenshot({ path: join(dir, `components-${tag}-viewport.png`), fullPage: false });
    await writeFile(join(dir, `components-${tag}.txt`), redact(await pageText(page)));
    const alerts = await page.evaluate(() => Array.from(document.querySelectorAll('[role="alert"]')).map(a => (a.innerText || '').replace(/\s+/g, ' ').trim()).filter(Boolean)).catch(() => []);
    await cardLog(page);
    await page.close();
    const ended = Date.now();
    const posts = scanPosts(at, ended);
    return { load_at: iso(at), scan_posts: posts.length, posts: posts.map(brief),
        posts_conditional: posts.filter(p => /max_age_seconds=/.test(p.url)).length, alerts,
        toasts: toastRows.filter(r => r.page === label), answers: answers(at, ended) };
}

// Direct reads in a page that is not the app: the body as the browser receives it (redacted).
async function directReads(events) {
    const out = {};
    for (const path of ['/api/v1/panel/version', '/api/v1/license/access', '/api/v1/managed-services']) {
        const page = await newPage('read-' + path.split('/').pop());
        const from = Date.now();
        let text = null;
        try {
            const res = await page.goto(base + path, { waitUntil: 'load', timeout: 60000 });
            text = res ? await res.text() : null;
            let body = null; try { body = JSON.parse(text); } catch { /* not JSON */ }
            if (path === '/api/v1/managed-services' && body && Array.isArray(body.services)) body = { ...body, services: `<${body.services.length} entries>` };
            out[path] = { status: res ? res.status() : null, at: iso(from), body };
        } catch (error) { out[path] = { error: String(error.message).slice(0, 200) }; }
        await pause(300);
        await page.close();
    }
    await writeFile(join(dir, 'direct-reads.json'), redact(JSON.stringify(out, null, 2)));
    await jsonl(events, { at: iso(), ev: 'direct-reads', paths: Object.keys(out) });
    return out;
}

// -- set16: the sidebar build id at two widths, the licence gate with the guest clock skewed, the Agent stopped ------

const DESKTOP_1280 = { width: 1280, height: 800, deviceScaleFactor: 1 };
// The build id unit (web/src/components/Layout.tsx BuildStamp, data-build-stamp since 942623e1d): its visible id span
// (aria-hidden) drawn on one line has one client rectangle; "build" and the id on one line share a top.
const STAMP_FACTS = () => {
    const units = Array.from(document.querySelectorAll('[data-build-stamp]')).filter(u => u.getClientRects().length && u.getBoundingClientRect().width > 0);
    const legacy = Array.from(document.querySelectorAll('span.break-all.font-mono')).filter(u => /\bbuild\b/.test(u.textContent || ''));
    const u = units[0] || legacy[0] || null;
    if (!u) return { found: false, units: units.length, legacy: legacy.length };
    const id = u.querySelector('[aria-hidden="true"]');
    const idRects = id ? Array.from(id.getClientRects()).map(r => ({ top: Math.round(r.top), left: Math.round(r.left), width: Math.round(r.width) })) : [];
    const range = document.createRange();
    const firstText = Array.from(u.childNodes).find(n => n.nodeType === 3 && /build/.test(n.textContent));
    let buildTop = null;
    if (firstText) { range.selectNodeContents(firstText); const r = range.getClientRects()[0]; buildTop = r ? Math.round(r.top) : null; }
    const box = u.getBoundingClientRect();
    const host = u.closest('aside, footer') || u.parentElement;
    const hostBox = host ? host.getBoundingClientRect() : null;
    const line = u.parentElement ? (u.parentElement.innerText || '').replace(/\s+/g, ' ').trim() : null;
    const style = getComputedStyle(u);
    return { found: true, via: units.length ? 'data-build-stamp' : 'legacy break-all span', class: u.className,
        white_space: style.whiteSpace, word_break: style.wordBreak, display: style.display,
        id_text: id ? id.textContent : null, id_rects: idRects, id_on_one_line: idRects.length === 1,
        build_word_top: buildTop, build_and_id_share_a_line: buildTop !== null && idRects.length > 0 && Math.abs(buildTop - idRects[0].top) <= 2,
        unit_box: { left: Math.round(box.left), right: Math.round(box.right), top: Math.round(box.top), width: Math.round(box.width) },
        container: host ? host.tagName : null,
        container_box: hostBox ? { left: Math.round(hostBox.left), right: Math.round(hostBox.right), width: Math.round(hostBox.width) } : null,
        overflows_container: hostBox ? box.right > hostBox.right + 0.5 : null, stamp_line: line, title: u.getAttribute('title') };
};

async function sidebarWidth(events, name, vp) {
    const phone = vp === PHONE;
    const page = await newPage('sidebar-' + name, LANG, phone ? 'phone' : 'desktop');
    await page.setViewport(vp);
    await toFront(page);
    await page.goto(base + '/domains', { waitUntil: 'networkidle2', timeout: 60000 });
    await pause(2500);
    let menu = null;
    if (phone) {
        menu = await page.evaluate(() => { const b = document.querySelector('button[aria-label="Menu"]'); if (!b) return false; b.click(); return true; }).catch(() => false);
        await pause(1200);
    }
    const facts = await page.evaluate(STAMP_FACTS).catch(error => ({ error: String(error.message).slice(0, 160) }));
    await page.screenshot({ path: join(dir, `sidebar-${name}-viewport.png`), fullPage: false });
    let unitShot = false;
    try {
        const handle = await page.evaluateHandle(() => {
            const u = Array.from(document.querySelectorAll('[data-build-stamp]')).find(x => x.getBoundingClientRect().width > 0);
            return u ? (u.closest('aside') || u.parentElement) : null;
        });
        const element = handle.asElement();
        if (element) { await element.screenshot({ path: join(dir, `sidebar-${name}-stamp.png`) }); unitShot = true; }
    } catch { /* moved */ }
    await page.close();
    const value = { at: iso(), name, viewport: { width: vp.width, height: vp.height, phone }, menu_opened: menu, ...facts, stamp_picture: unitShot };
    await jsonl(events, { ev: 'sidebar-width', ...value });
    return value;
}

// The licence gate with the guest clock moved: one Dashboard load, watched, and its access reads.
async function skewedLoad(events, tag) {
    const before = await clockRead(events, tag + '-before');
    const at = Date.now();
    const load = await dashboardLoad('p5-dashboard-' + tag, 'dashboard-' + tag, events, 75000);
    const after = await clockRead(events, tag + '-after');
    const pageReads = requests.filter(r => r.page === 'p5-dashboard-' + tag && r.start_ms >= at);
    const accessRows = pageReads.filter(r => r.method === 'GET' && r.url.split('?')[0] === ACCESS);
    return { tag, clock_before: before, clock_after: after,
        access_offsets_ms: accessRows.map(r => r.access?.guest_minus_browser_ms).filter(Boolean),
        mounted: load.mount.first_managed_services_read_ms !== null,
        other_api_reads: pageReads.filter(r => r.url.split('?')[0] !== ACCESS).length,
        access_reads_whole_page: accessRows.length,
        old_rule_would_refuse: accessRows.filter(r => r.access?.old_rule_accepts_at_receipt === false).length, ...load };
}

async function clockPhase(events) {
    const out = { steps: [] };
    const step = async (what, fn) => { const value = await fn(); out.steps.push({ what, at: iso(), value }); return value; };
    await step('prep clockshift_p90000', () => prep('clockshift_p90000', events));
    out.ahead = await skewedLoad(events, 'ahead90');
    await step('prep clockshift_m180000', () => prep('clockshift_m180000', events));
    out.behind = await skewedLoad(events, 'behind90');
    await step('prep clockshift_p90000 (back to the start)', () => prep('clockshift_p90000', events));
    await step('prep ntp-on', () => prep('ntp-on', events));
    out.restored = await clockRead(events, 'restored');
    return out;
}

const AGENT_TR = /CelikPanel Agent’ına ulaşılamıyor/;
const AGENT_EN = /The CelikPanel Agent on this server is not reachable/;

async function bannerFacts(page) {
    return page.evaluate(() => {
        const main = document.querySelector('main') || document.body;
        const lines = (main.innerText || '').split('\n').map(x => x.trim()).filter(Boolean);
        const alerts = Array.from(document.querySelectorAll('[role="alert"]')).map(a => (a.innerText || '').replace(/\s+/g, ' ').trim()).filter(Boolean);
        return { firewall_lines: lines.filter(x => /Firewall|Güvenlik duvarı/i.test(x)).slice(0, 8), alerts, html_lang: document.documentElement.lang || null };
    }).catch(error => ({ error: String(error.message).slice(0, 160) }));
}

async function attentionFacts(page) {
    return page.evaluate(() => {
        const s = Array.from(document.querySelectorAll('section')).find(x => /^(Needs attention|İlgi istiyor)/.test((x.innerText || '').trim()));
        return s ? { found: true, text: (s.innerText || '').replace(/\s+/g, ' ').trim().slice(0, 2000), busy: s.getAttribute('aria-busy') } : { found: false };
    }).catch(error => ({ error: String(error.message).slice(0, 160) }));
}

// set16 rerun (2026-10-15): what the surface of a refused read looks like, measured, not judged here: the computed
// border, background and text colours of each [role="alert"] in the given root (and of every element inside it whose
// text colour is the theme's failure colour, --danger), and for each "celikpanel-agent" in its text the line boxes
// of exactly that token (one top = drawn unbroken). Light theme: --danger 179 38 30, --warning-mark 245 179 1.
const SURFACE_FACTS = (rootSelector) => {
    const root = rootSelector ? document.querySelector(rootSelector) : document;
    if (!root) return { found: false, root: rootSelector };
    const danger = getComputedStyle(document.documentElement).getPropertyValue('--danger').trim();
    const dangerRgb = danger ? 'rgb(' + danger.split(/\s+/).join(', ') + ')' : null;
    const alerts = Array.from(root.querySelectorAll('[role="alert"]')).filter(a => a.getBoundingClientRect().width > 0);
    const tokenRuns = (el, token) => {
        const out = [];
        const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
        for (let n = walker.nextNode(); n; n = walker.nextNode()) {
            let i = n.textContent.indexOf(token);
            while (i >= 0) {
                const r = document.createRange(); r.setStart(n, i); r.setEnd(n, i + token.length);
                const tops = Array.from(r.getClientRects()).filter(x => x.width > 0).map(x => Math.round(x.top));
                const parent = n.parentElement;
                out.push({ tops, unbroken: new Set(tops).size === 1, nowrap_parent: parent ? getComputedStyle(parent).whiteSpace : null });
                i = n.textContent.indexOf(token, i + token.length);
            }
        }
        return out;
    };
    return { found: true, root: rootSelector, danger_rgb: dangerRgb, alerts: alerts.map(a => {
        const s = getComputedStyle(a);
        const inDanger = Array.from(a.querySelectorAll('*')).filter(x => danger && (getComputedStyle(x).color.match(/\d+/g) || []).slice(0, 3).join(' ') === danger.split(/\s+/).join(' ')).map(x => x.tagName + ':' + (x.textContent || '').trim().slice(0, 60));
        const tokens = tokenRuns(a, 'celikpanel-agent');
        return { text: (a.innerText || '').replace(/\s+/g, ' ').trim().slice(0, 300), class: a.className, data_firewall_unread: a.hasAttribute('data-firewall-unread'),
            border_color: s.borderTopColor, background_color: s.backgroundColor, color: s.color,
            failure_colour_on_alert: danger ? [s.borderTopColor, s.backgroundColor, s.color].some(c => (c.match(/\d+/g) || []).slice(0, 3).join(' ') === danger.split(/\s+/).join(' ')) : null,
            elements_in_failure_colour: inDanger, width: Math.round(a.getBoundingClientRect().width),
            celikpanel_agent_tokens: tokens.length, celikpanel_agent_all_unbroken: tokens.length ? tokens.every(x => x.unbroken) : null, token_runs: tokens };
    }) };
};
async function surfaceFacts(page, rootSelector = null) {
    return page.evaluate(SURFACE_FACTS, rootSelector).catch(error => ({ error: String(error.message).slice(0, 160) }));
}

async function agentPhase(events) {
    const out = {};
    out.stop = await prep('agent-stop', events);
    // the Components page in Turkish, kept open until after the Agent is back
    const label = 'p6-components-tr-agent-stopped';
    const comp = await newPage(label, 'tr');
    await toFront(comp);
    const at = Date.now();
    await comp.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await pause(30000);
    await comp.screenshot({ path: join(dir, 'components-tr-agent-stopped-viewport.png'), fullPage: false });
    const compText = redact(await pageText(comp));
    await writeFile(join(dir, 'components-tr-agent-stopped.txt'), compText);
    out.components_tr_surface = await surfaceFacts(comp, 'main');
    out.components_tr = { load_at: iso(at), ...(await bannerFacts(comp)), agent_sentence_tr: AGENT_TR.test(compText), agent_sentence_en: AGENT_EN.test(compText),
        firewall_reads: requests.filter(r => r.page === label && r.url.split('?')[0] === '/api/v1/firewall').map(r => ({ start: r.start, status: r.status, ms: r.ms, agent_read: r.agent_read })),
        scan_posts: scanPosts(at).filter(r => r.page === label).map(brief) };
    await cardLog(comp);
    {   // set16 rerun: the same page at 390x844 (Turkish), the surface and the command tokens at phone width
        const pl = 'p6-components-tr-phone-agent-stopped';
        const pp = await newPage(pl, 'tr', 'phone');
        await toFront(pp);
        await pp.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
        await pause(20000);
        await pp.screenshot({ path: join(dir, 'components-tr-phone-agent-stopped-viewport.png'), fullPage: false });
        await pp.screenshot({ path: join(dir, 'components-tr-phone-agent-stopped-full.png'), fullPage: true });
        out.components_tr_phone_surface = await surfaceFacts(pp, 'main');
        await pp.close();
    }
    await toFront(comp);
    // the Dashboard in Turkish and in English with the Agent stopped (the Components page stays open behind them)
    for (const [lang, tag] of [['tr', 'tr'], ['en', 'en']]) {
        const dl = 'p6-dashboard-' + tag + '-agent-stopped';
        const page = await newPage(dl, lang);
        await toFront(page);
        const t0 = Date.now();
        await page.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
        await observe(page, 40000, events, { tag: '-agent-' + tag });
        await shoot(page, `dashboard-${tag}-agent-stopped`, events, { lang, viewport: VIEWPORT_NAME });
        await page.screenshot({ path: join(dir, `dashboard-${tag}-agent-stopped-full.png`), fullPage: true });
        const text = redact(await pageText(page));
        await writeFile(join(dir, `dashboard-${tag}-agent-stopped-full.txt`), text);
        const attention = await attentionFacts(page);
        const attentionSurface = await page.evaluate(() => { const s = Array.from(document.querySelectorAll('section')).find(x => /^(Needs attention|İlgi istiyor)/.test((x.innerText || '').trim())); if (s) s.setAttribute('data-set16-attention', ''); return !!s; }).then(found => found ? surfaceFacts(page, '[data-set16-attention]') : { found: false }).catch(error => ({ error: String(error.message).slice(0, 160) }));
        await cardLog(page);
        out['dashboard_' + tag] = { load_at: iso(t0), attention, attention_surface: attentionSurface,
            attention_names_agent: attention.found ? (lang === 'tr' ? AGENT_TR : AGENT_EN).test(attention.text) : false,
            agent_sentence_other_language: lang === 'tr' ? AGENT_EN.test(text) : AGENT_TR.test(text),
            states: statesOf(sequenceOf(dl, phase)), mail_states: statesOf(mailSequenceOf(dl, phase)),
            scan_posts: scanPosts(t0).filter(r => r.page === dl).map(brief),
            agent_reads: requests.filter(r => r.page === dl && r.agent_read).map(r => ({ url: r.url, status: r.status, agent_read: r.agent_read })) };
        await page.close();
    }
    out.start = await prep('agent-start', events);
    await pause(3000);
    // back on the Turkish Components page: one press of "Yeniden tara", then the banner read again for 20 s
    await toFront(comp);
    const pressAt = Date.now();
    const pressed = await comp.evaluate(() => {
        const b = Array.from(document.querySelectorAll('button')).find(x => (x.innerText || '').trim() === 'Yeniden tara' && !x.disabled);
        if (!b) return false;
        b.click();
        return true;
    }).catch(() => false);
    await pause(20000);
    await comp.screenshot({ path: join(dir, 'components-tr-after-rescan-viewport.png'), fullPage: false });
    const afterText = redact(await pageText(comp));
    await writeFile(join(dir, 'components-tr-after-rescan.txt'), afterText);
    out.components_tr_after_rescan = { pressed, press_at: iso(pressAt), ...(await bannerFacts(comp)), agent_sentence_tr: AGENT_TR.test(afterText),
        firewall_reads_after_press: requests.filter(r => r.page === label && r.url.split('?')[0] === '/api/v1/firewall' && r.start_ms >= pressAt).map(r => ({ start: r.start, status: r.status, ms: r.ms })),
        scan_posts_after_press: scanPosts(pressAt).filter(r => r.page === label).map(brief) };
    await cardLog(comp);
    await comp.close();
    return out;
}

async function cellUI() {
    const events = join(dir, 'events.jsonl');
    const fresh = hand.mode === 'fresh';
    await jsonl(events, { at: iso(), ev: 'start', mode: hand.mode, lang: LANG, viewport: VIEWPORT_NAME, chrome: await browser.version() });

    phase = 'sign-in';
    let t = Date.now();
    const signer = await newPage('sign-in');
    await toFront(signer);
    await signIn(signer, events);
    await cardLog(signer);
    await signer.close();
    await phaseDone('sign-in', t);

    phase = 'domains';
    t = Date.now();
    const domains = await domainsPage(events);
    await phaseDone('domains', t, domains);

    phase = 'sidebar';
    t = Date.now();
    const w1280 = await sidebarWidth(events, '1280x800', DESKTOP_1280);
    const w390 = await sidebarWidth(events, '390x844', PHONE);
    await phaseDone('sidebar', t, { w1280, w390 });

    phase = 'dashboard';
    const stale = await waitStale(events, 'dashboard');
    t = Date.now();
    const dash = await dashboardLoad('p2-dashboard', 'dashboard', events, 90000);
    await phaseDone('dashboard', t, { stale: { turn: stale.turn, age_s: stale.age ?? null, absent: stale.absent ?? false,
        probe_scan: scanOf(stale.probe) }, ...dash });

    phase = 'components';
    t = Date.now();
    const compFresh = await componentsLoad(events, 'fresh');
    if (fresh) {
        const stale2 = await waitStale(events, 'components');
        const compStale = await componentsLoad(events, 'stale');
        await phaseDone('components', t, { fresh: compFresh, stale: { turn: stale2.turn, age_s: stale2.age ?? null, absent: stale2.absent ?? false,
            probe_scan: scanOf(stale2.probe) }, stale_load: compStale, scan_posts_fresh: compFresh.scan_posts, scan_posts_stale: compStale.scan_posts });
    } else {
        await phaseDone('components', t, { fresh: compFresh, scan_posts_fresh: compFresh.scan_posts });
    }

    if (fresh) {
        phase = 'clock';
        t = Date.now();
        const clock = await clockPhase(events);
        await phaseDone('clock', t, clock);

        phase = 'agent';
        t = Date.now();
        const agent = await agentPhase(events);
        await phaseDone('agent', t, agent);
    }

    phase = 'reads';
    t = Date.now();
    const reads = await directReads(events);
    await phaseDone('reads', t, { reads });

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
        else if (cell === 'done') await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set16.mjs' }) + '\n');
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
