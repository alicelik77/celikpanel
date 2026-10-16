// node cold-load-set18.mjs <handover-dir> <out-dir> [setup,done]
// set18 (2026-10-15): the owner's guided setup run to READY and an extension of it, in a real headless Chrome against
// a REAL CelikPanel on a disposable lab guest whose lab has its own DNS and ACME server (deploy/e2e/release-recovery/
// guest_set18_lab_dns.py, a lab preparation; set18_trial.py, set12's browser window protocol, window "setup").
// Nothing but the loopback address of the driver's forward is opened.
//
//   cell "setup":
//     1  setup to ready (English, 1280x800): sign-in, "Set up with the wizard", purpose Web hosting, panel address
//        hand.lab.panel_domain, DNS external, review (the plan's blocks recorded), confirm, start; the run is followed
//        (the page sampled every second, every new state pictured) to "Your server is ready". The page's own reads
//        are recorded with their bodies (/api/v1/setup*). The Panel's certificate changes in this run (the handover),
//        so Chrome is started with --ignore-certificate-errors (lab loopback only); the leaf is recorded before and
//        after.
//     3  the Components mail route: Components -> Core Mail -> Install -> the dialog's setup route, the mail host name
//        hand.lab.mail_hostname, "Review in server setup" -> the wizard's access step (prefilled) -> the review (what
//        changes / to be installed / kept) -> back to the access step, the secondary-DNS box of the access step
//        (local DNS, role secondary; UI state only, never saved) -> "Stop extending" -> the ready screen; the driver
//        reads the setup and its latest operation (nothing started).
//     2  the extension from the ready screen: "Extend setup" -> "Customize selection" -> Postfix + Dovecot -> the
//        access step (mail host name) -> review (blocks recorded) -> confirm, start; followed to ready. When the run
//        waits at the public DNS check of the mail host name, the owner adds the record (driver preparation
//        "lab-mail-a": the lab's record table); when the final check names the reverse DNS, the owner sets it at the
//        provider ("lab-ptr-mail") and presses "Check requirements again" (the "Finishing setup" state).
//     Browser checks: the ready screen (with the extension section), the Components mail dialog (setup route), the
//     extension's access step and review, at 1280x800 and 390x844, English and Turkish.
//   cell "resume" (set18 run, after D1): the extension wizard shows the COMPLETED first run instead of its steps
//     (GET /api/v1/setup/operation answers the finished run while the setup is an extension draft), so "Stop
//     extending" and the wizard's review and start are not reachable. In a new browser (no stored marker) the owner
//     opens /setup (D1 recorded again); the blocked UI actions are then made by the page's own session with the
//     wizard's own requests (POST /api/v1/setup/extend cancel; from the ready screen "Extend setup", then POST
//     /api/v1/setup/extend begin with Core Mail and the mail host name, POST /api/v1/setup/plan, POST
//     /api/v1/setup/start), each recorded as a STAND-IN for the blocked button; the run is then followed in the page
//     exactly as cell 2 (mail record, reverse DNS, "Check requirements again", "Finishing setup", ready).
//   cell "done": writes done.json
//
//   BROWSER_INSPECT_MODULES  directory whose node_modules holds puppeteer-core (required; outside the repository)
//   CHROME_PATH              the installed Chrome (default: the usual Windows/Linux places)
//
// The owner's password is read from the root-only file the driver names and typed into the sign-in form; every text
// this script writes is passed through the redaction list of that file first. Cookies are never read.
import { createHash, randomBytes } from 'node:crypto';
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

const [handDir, outRoot, cellsArg = 'setup,done'] = process.argv.slice(2);
if (!handDir || !outRoot) { console.error('usage: node cold-load-set18.mjs <handover-dir> <out-dir> <cells>'); process.exit(2); }
const cells = cellsArg.split(',');
const hand = JSON.parse(readFileSync(join(handDir, 'ready.json'), 'utf8'));
const port = Number(hand.local_port);
if (!(port > 1024 && port < 65536)) throw new Error('bad port in the handover file');
const LAB = hand.lab || {};
if (!/^[a-z0-9.-]+\.test$/.test(LAB.panel_domain || '') || !/^[a-z0-9.-]+\.test$/.test(LAB.mail_hostname || '')) throw new Error('the handover names no lab host names');
const base = `https://127.0.0.1:${port}`;
const wslShare = process.env.LIVE_WSL_SHARE || '//wsl.localhost/archlinux';
const secretPath = process.platform === 'win32' ? wslShare + hand.password_file : hand.password_file;
const pause = ms => new Promise(done => setTimeout(done, ms));
const iso = (ms = Date.now()) => new Date(ms).toISOString();
const VIEWPORTS = { '1280': { width: 1280, height: 800, deviceScaleFactor: 1 }, phone: { width: 390, height: 844, deviceScaleFactor: 2, isMobile: true, hasTouch: true } };

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
            done({ leaf_sha256: createHash('sha256').update(cert.raw).digest('hex'), subject: cert.subject, issuer: cert.issuer, valid_to: cert.validTo, san: cert.subjectAltName });
        });
        socket.setTimeout(8000, () => { socket.destroy(); fail(new Error('timeout')); });
        socket.on('error', fail);
    });
}

await mkdir(outRoot, { recursive: true });
const dir = outRoot;
const events = join(dir, 'events.jsonl');
const leafRecord = async label => { try { const leaf = await leafOf(); await jsonl(join(dir, 'leaf.jsonl'), { at: iso(), label, ...leaf, handover_leaf_sha256: hand.tls_leaf_sha256 }); return leaf; } catch (error) { await jsonl(join(dir, 'leaf.jsonl'), { at: iso(), label, error: String(error.message).slice(0, 120) }); return null; } };
await leafRecord('start');
secret = JSON.parse(readFileSync(secretPath, 'utf8'));
const profile = shortProfileDir(outRoot);
const browser = await puppeteer.launch({
    executablePath: chrome, headless: true, userDataDir: profile,
    // The Panel's certificate changes during setup (lab pebble CA, not trusted by Chrome); only the loopback forward is opened.
    args: ['--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--disable-component-update',
        '--disable-sync', '--disable-domain-reliability', '--force-color-profile=srgb', '--ignore-certificate-errors'],
});

// -- recording -----------------------------------------------------------------------------------------------------

let phase = 'start';
const front = { page: null };
const latest = { operation: null, setup: null };
const BODY_PREFIXES = ['/api/v1/setup', '/api/v1/panel/version'];

async function newPage(label, lang = 'en', vp = '1280') {
    const page = await browser.newPage();
    page.__label = label;
    await page.setViewport(VIEWPORTS[vp]);
    await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }]);
    // The language follows a per-tab value (sessionStorage) so that a reload can switch it.
    await page.evaluateOnNewDocument(value => { try { const chosen = sessionStorage.getItem('set18.lang') || value; localStorage.setItem('celikpanel.lang', chosen); localStorage.setItem('celikpanel.theme', 'light'); } catch { /* none */ } }, lang);
    const started = new Map();
    page.on('request', req => { if (req.url().includes('/api/')) started.set(req, Date.now()); });
    const finish = async (req, failed) => {
        if (!req.url().includes('/api/')) return;
        const t0 = started.get(req) ?? Date.now();
        started.delete(req);
        const res = failed ? null : req.response();
        const entry = { page: label, phase, method: req.method(), url: req.url().replace(base, ''), status: res ? res.status() : null, start: iso(t0), ms: Date.now() - t0 };
        if (failed) entry.error = req.failure()?.errorText;
        const path0 = entry.url.split('?')[0];
        if (res && BODY_PREFIXES.some(prefix => path0.startsWith(prefix))) {
            try {
                const text = await res.text();
                entry.body = text.slice(0, 20000);
                const value = JSON.parse(text);
                if (path0 === '/api/v1/setup/operation' && value && typeof value === 'object') latest.operation = { at: Date.now(), value };
                if (path0 === '/api/v1/setup' && value && typeof value === 'object' && req.method() === 'GET') latest.setup = { at: Date.now(), value };
            } catch (error) { entry.body_unreadable = String(error.message).slice(0, 120); }
            if (['POST', 'PUT'].includes(req.method())) { try { entry.request_body = (req.postData() || '').slice(0, 4000); } catch { /* none */ } }
        } else if (res && res.status() >= 400) {
            try { const code = (JSON.parse(await res.text()) || {}).code; if (typeof code === 'string') entry.code = code.slice(0, 80); } catch { /* not JSON */ }
        }
        await jsonl(join(dir, 'requests.jsonl'), entry);
    };
    page.on('requestfinished', req => void finish(req, false));
    page.on('requestfailed', req => void finish(req, true));
    page.on('console', message => { if (['error', 'warn', 'warning'].includes(message.type())) void jsonl(join(dir, 'console.jsonl'), { at: iso(), page: label, phase, type: message.type(), text: message.text().slice(0, 400) }); });
    return page;
}

async function toFront(page) { await page.bringToFront(); front.page = page; await pause(300); }
async function pageText(page) {
    return page.evaluate(() => (document.querySelector('main') || document.body).innerText.replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n').trim().slice(0, 16000)).catch(() => '');
}
const shotNames = new Set();
async function shot(page, name, extra = {}, full = false) {
    if (front.page !== page) { await jsonl(events, { at: iso(), ev: 'shot-skipped', name }); return null; }
    let n = name; let i = 2; while (shotNames.has(n)) n = `${name}-${i++}`; shotNames.add(n);
    try {
        await page.screenshot({ path: join(dir, `${n}-viewport.png`), fullPage: false });
        if (full) await page.screenshot({ path: join(dir, `${n}-full.png`), fullPage: true });
    } catch (error) { await jsonl(events, { at: iso(), ev: 'shot-failed', name: n, error: String(error.message).slice(0, 160) }); return null; }
    await writeFile(join(dir, `${n}.txt`), redact(await pageText(page)));
    await jsonl(events, { at: iso(), ev: 'shot', name: n, phase, ...extra });
    return n;
}
async function prep(name, limitMs = 900000) {
    const key = `${name}-${Date.now()}`;
    const asked = Date.now();
    await writeFile(join(handDir, `prep-request-${key}`), iso() + '\n');
    await jsonl(events, { at: iso(), ev: 'prep-requested', name, phase });
    const file = join(handDir, `prep-done-${key}.json`);
    while (Date.now() - asked < limitMs && !existsSync(file)) await pause(200);
    const record = existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : { missing: true, waited_ms: Date.now() - asked };
    const out = { ...record, host_asked_ms: asked, host_got_ms: Date.now() };
    await jsonl(events, { at: iso(), ev: 'prep-done', name, phase, record: out });
    return out;
}
const note = async (ev, extra = {}) => jsonl(events, { at: iso(), ev, phase, ...extra });

async function signIn(page) {
    await page.goto(base + '/', { waitUntil: 'networkidle2', timeout: 60000 });
    const shown = await page.waitForFunction(() => document.getElementById('username') ? 'form' : document.querySelector('main, nav, aside') ? 'signed-in' : null, { timeout: 60000 }).then(handle => handle.jsonValue());
    await note('sign-in-page', { shown });
    if (shown === 'signed-in') return;
    await page.type('#username', secret.username);
    await page.type('#password', secret.password);
    await Promise.all([page.keyboard.press('Enter'), pause(500)]);
    await page.waitForFunction(() => !document.getElementById('password'), { timeout: 60000 });
    await pause(2500);
}

// -- page helpers --------------------------------------------------------------------------------------------------

async function clickText(page, pattern, scope = 'main', tags = 'button, a') {
    const source = pattern.source; const flags = pattern.flags;
    const clicked = await page.evaluate((scopeSel, tagSel, src, fl) => {
        const re = new RegExp(src, fl);
        const root = document.querySelector(scopeSel) || document.body;
        const el = Array.from(root.querySelectorAll(tagSel)).find(x => re.test((x.innerText || x.textContent || '').trim()) && !x.disabled && x.getClientRects().length);
        if (!el) return null;
        el.scrollIntoView({ block: 'center' }); el.click();
        return (el.innerText || '').trim().slice(0, 120);
    }, scope, tags, source, flags).catch(() => null);
    await note('click', { pattern: String(pattern), clicked });
    return clicked;
}
async function waitFor(page, fn, timeout = 60000, arg = null) {
    try { await page.waitForFunction(fn, { timeout, polling: 500 }, arg); return true; } catch { return false; }
}
async function fill(page, selector, value) {
    await page.waitForSelector(selector, { timeout: 30000 });
    await page.click(selector, { clickCount: 3 });
    await page.keyboard.down('Control'); await page.keyboard.press('KeyA'); await page.keyboard.up('Control');
    await page.keyboard.press('Backspace');
    await page.type(selector, value, { delay: 20 });
    const now = await page.$eval(selector, el => el.value);
    await note('fill', { selector, value, now });
    return now === value;
}
async function settle(page, ms = 1500) { await page.waitForNetworkIdle({ idleTime: 600, timeout: 15000 }).catch(() => {}); await pause(ms); }

const SETUP_SAMPLE = () => {
    const txt = el => el ? (el.innerText || '').replace(/\s+/g, ' ').trim() : null;
    const main = document.querySelector('main') || document.body;
    const guidance = document.querySelector('aside[aria-labelledby="setup-guidance-title"], aside[aria-labelledby="setup-progress-title"]');
    return {
        t: Date.now(), path: location.pathname, vis: document.visibilityState, lang: document.documentElement.lang || null,
        h1: txt(main.querySelector('h1')), progress_title: txt(document.getElementById('setup-progress-title')),
        guidance_title: txt(document.getElementById('setup-guidance-title')), guidance: guidance ? txt(guidance).slice(0, 3000) : null,
        checks: Array.from(document.querySelectorAll('[data-check]')).map(li => ({ id: li.getAttribute('data-check'), state: li.getAttribute('data-state'), text: txt(li) })),
        steps: Array.from(document.querySelectorAll('[data-step-state]')).map(li => ({ state: li.getAttribute('data-step-state'), text: txt(li) })),
        finishing: txt(document.querySelector('[data-setup-finishing]')),
        reconnecting: txt(document.querySelector('[data-setup-reconnecting]')), reconnect_slow: txt(document.querySelector('[data-setup-reconnect-slow]')),
        handover: txt(document.querySelector('aside[aria-labelledby="setup-handover-title"]')),
        alerts: Array.from(document.querySelectorAll('[role="alert"]')).filter(a => a.getClientRects().length).map(a => txt(a).slice(0, 800)),
        statuses: Array.from(main.querySelectorAll('[role="status"]')).filter(a => a.getClientRects().length).map(a => txt(a).slice(0, 800)).slice(0, 8),
        buttons: Array.from(main.querySelectorAll('button, a')).filter(b => b.getClientRects().length).map(b => ({ text: txt(b), disabled: !!b.disabled })).filter(b => b.text).slice(0, 30),
        extend_section: txt(document.querySelector('[data-setup-extend]')), extend_stop: txt(document.querySelector('[data-setup-extend-stop]')),
        overflow_x: document.documentElement.scrollWidth > document.documentElement.clientWidth,
    };
};
const signature = s => JSON.stringify([s.h1, s.progress_title, s.guidance_title, !!s.finishing, s.steps.map(x => x.state), s.checks.map(x => `${x.id}:${x.state}`), !!s.reconnecting, !!s.reconnect_slow, s.alerts.length, (s.guidance || '').slice(0, 160)]);
const op = () => latest.operation?.value || null;
const opStep = (execution, kind, target) => (execution?.steps || []).find(x => x.kind === kind && (!target || x.target === target));

// Follow a started run until the server is ready (or failed, or the bound).
async function follow(page, label, limitMs, mail) {
    const started = Date.now();
    const result = { label, started_at: iso(started), states: 0, pictures: [], owner_actions: [], lab: [] };
    let lastSig = null; let shots = 0; let lastHeartbeat = 0;
    let mailWaitSince = null; let ptrSeenAt = null; let lastVerifyPress = 0; let waitingVerifySince = null; let slowSince = null;
    let done = false;
    while (!done && Date.now() - started < limitMs) {
        const t0 = Date.now();
        let s;
        try { s = await page.evaluate(SETUP_SAMPLE); } catch (error) { s = { t: t0, error: String(error.message).slice(0, 160) }; }
        const execution = op();
        if (!s.error) {
            s.operation = execution ? { status: execution.status, phase: execution.phase, error: execution.error?.code || null, steps: (execution.steps || []).map(x => `${x.id}:${x.status}`) } : null;
            const sig = signature(s);
            if (sig !== lastSig || Date.now() - lastHeartbeat > 30000) {
                lastHeartbeat = Date.now();
                if (sig !== lastSig) { result.states++; s.new_state = true; }
                if (sig !== lastSig && shots < 70 && s.vis === 'visible') { shots++; s.picture = await shot(page, `${label}-state-${String(shots).padStart(2, '0')}`, { t_plus_s: Math.round((Date.now() - started) / 1000) }); if (s.picture) result.pictures.push(s.picture); }
                lastSig = sig;
                await jsonl(join(dir, `${label}-samples.jsonl`), s);
            }
            if (s.finishing && !result.finishing_seen) { result.finishing_seen = { at: iso(), text: s.finishing }; }
            if (/ready|hazır/i.test(s.h1 || '') && s.extend_section) { result.ready_at = iso(); done = true; break; }
            if (execution?.status === 'failed') { result.failed = { at: iso(), error: execution.error }; if (!result.failed_picture) result.failed_picture = await shot(page, `${label}-failed`, {}, true); if (Date.now() - Date.parse(result.failed.at) > 0) { done = true; break; } }
            // the reconnect has been slow for a minute: the owner reloads the page, as it offers
            if (s.reconnect_slow) { slowSince = slowSince || Date.now(); if (Date.now() - slowSince > 60000) { result.owner_actions.push({ at: iso(), action: 'reload (Reload this page)' }); await clickText(page, /Reload this page|Sayfayı yeniden yükle/); slowSince = null; await settle(page); } } else slowSince = null;
            // a recorded mail enrollment wait offers the owner's explicit action
            if (s.buttons.some(b => /Continue recorded operation|Retry reviewed handoff/.test(b.text) && !b.disabled)) { const c = await clickText(page, /Continue recorded operation|Retry reviewed handoff/); result.owner_actions.push({ at: iso(), action: c }); }
            if (mail && execution) {
                const mailDNS = opStep(execution, 'access_dns', LAB.mail_hostname);
                if (execution.status === 'waiting' && execution.phase === mailDNS?.id && !result.lab.some(x => x.what === 'lab-mail-a')) {
                    mailWaitSince = mailWaitSince || Date.now();
                    if (Date.now() - mailWaitSince > 50000) {
                        await shot(page, `${label}-mail-dns-waiting`, {}, true);
                        const r = await prep('lab-mail-a', 120000);
                        result.lab.push({ at: iso(), what: 'lab-mail-a', ok: !r.result?.error, waited_s: Math.round((Date.now() - mailWaitSince) / 1000) });
                    }
                }
                const identity = (execution.checks || []).find(c => c.id === 'mail_identity');
                if (execution.status === 'waiting' && execution.phase === 'verification' && identity && identity.state !== 'ready' && !result.lab.some(x => x.what === 'lab-ptr-mail')) {
                    ptrSeenAt = ptrSeenAt || Date.now();
                    result.mail_identity_seen = result.mail_identity_seen || { at: iso(), check: identity, page_lines: s.checks };
                    if (Date.now() - ptrSeenAt > 45000) {
                        await shot(page, `${label}-reverse-dns`, {}, true);
                        // the owner also asks for the check once, so the result box names it too
                        await clickText(page, /^Check requirements again$/); result.owner_actions.push({ at: iso(), action: 'Check requirements again (before the PTR change)' });
                        await pause(6000); await shot(page, `${label}-reverse-dns-checked`, {}, true);
                        await jsonl(join(dir, `${label}-samples.jsonl`), { ...(await page.evaluate(SETUP_SAMPLE).catch(() => ({}))), marker: 'after the owner check, before the PTR change' });
                        const r = await prep('lab-ptr-mail', 120000);
                        result.lab.push({ at: iso(), what: 'lab-ptr-mail', ok: !r.result?.error });
                        await pause(3000);
                        await clickText(page, /^Check requirements again$/); lastVerifyPress = Date.now();
                        result.owner_actions.push({ at: iso(), action: 'Check requirements again (after the PTR change)' });
                    }
                }
            }
            // waiting at the final check with no lab change pending: the owner checks again, at most once a minute
            if (execution?.status === 'waiting' && execution.phase === 'verification') {
                waitingVerifySince = waitingVerifySince || Date.now();
                const pending = mail && !result.lab.some(x => x.what === 'lab-ptr-mail');
                if (!pending && Date.now() - waitingVerifySince > 25000 && Date.now() - lastVerifyPress > 60000) {
                    if (await clickText(page, /^Check requirements again$/)) { lastVerifyPress = Date.now(); result.owner_actions.push({ at: iso(), action: 'Check requirements again' }); }
                }
            } else waitingVerifySince = null;
        } else {
            await jsonl(join(dir, `${label}-samples.jsonl`), s);
        }
        await pause(Math.max(0, (s.finishing || execution?.phase === 'verification' ? 500 : 1000) - (Date.now() - t0)));
    }
    result.ended_at = iso();
    result.duration_s = Math.round((Date.now() - started) / 1000);
    result.final_operation = op();
    result.final_picture = await shot(page, `${label}-final`, {}, true);
    if (!result.ready_at && !result.failed) result.not_finished = `not ready within ${Math.round(limitMs / 60000)} min`;
    await writeFile(join(dir, `${label}-summary.json`), redact(JSON.stringify(result, null, 2)));
    return result;
}

// One picture per language and viewport; reopen() brings the page back to the view after a reload.
async function variants(page, name, reopen) {
    const out = [];
    for (const [lang, vp] of [['en', '1280'], ['en', 'phone'], ['tr', '1280'], ['tr', 'phone']]) {
        try {
            await page.setViewport(VIEWPORTS[vp]);
            await page.evaluate(value => sessionStorage.setItem('set18.lang', value), lang);
            await page.reload({ waitUntil: 'domcontentloaded', timeout: 60000 });
            await settle(page, 2000);
            if (reopen) await reopen(lang);
            await settle(page, 1200);
            const facts = await page.evaluate(() => ({ lang: document.documentElement.lang || null, overflow_x: document.documentElement.scrollWidth > document.documentElement.clientWidth, width: document.documentElement.clientWidth })).catch(() => ({}));
            const picture = await shot(page, `${name}-${lang}-${vp}`, { lang, vp, facts }, true);
            out.push({ lang, vp, picture, ...facts });
        } catch (error) { out.push({ lang, vp, error: String(error.message).slice(0, 200) }); }
    }
    await page.setViewport(VIEWPORTS['1280']);
    await page.evaluate(() => sessionStorage.setItem('set18.lang', 'en')).catch(() => {});
    await page.reload({ waitUntil: 'domcontentloaded', timeout: 60000 }).catch(() => {});
    await settle(page, 2000);
    if (reopen) await reopen('en');
    await jsonl(join(dir, 'variants.jsonl'), { at: iso(), name, out });
    return out;
}

async function reviewBlocks(page) {
    return page.evaluate(() => {
        const txt = el => el ? (el.innerText || '').replace(/\s+/g, ' ').trim() : null;
        const review = document.querySelector('[data-setup-extension-review]');
        const changes = Array.from(document.querySelectorAll('[data-change]')).map(x => ({ key: x.getAttribute('data-change'), text: txt(x) }));
        const installs = Array.from(document.querySelectorAll('[data-install]')).map(x => ({ target: x.getAttribute('data-install'), text: txt(x) }));
        const kept = review ? review.querySelector('details') : null;
        if (kept) kept.open = true;
        const steps = Array.from(document.querySelectorAll('section[aria-labelledby="setup-plan-title"] ol > li')).map(txt);
        const dl = Array.from(document.querySelectorAll('section[aria-labelledby="setup-plan-title"] dl > div')).map(txt);
        const blockers = txt(document.querySelector('section[aria-labelledby="setup-plan-title"] [role="alert"]'));
        return { extension_review: txt(review), changes, installs, kept: kept ? txt(kept) : null, steps, facts: dl, blockers, handover: txt(document.querySelector('aside[aria-labelledby="setup-handover-title"]')) };
    });
}

// -- the cell ------------------------------------------------------------------------------------------------------

async function cellSetup() {
    const summary = { cell: hand.cell, lab: LAB, chrome: await browser.version(), started_at: iso() };
    const write = async () => writeFile(join(dir, 'setup-summary.json'), redact(JSON.stringify(summary, null, 2)));
    await note('start', { chrome: summary.chrome });
    const page = await newPage('owner', 'en', '1280');
    await toFront(page);
    try {
        // ---- 1. the owner's guided setup, purpose web, to ready
        phase = 'c1-wizard';
        await signIn(page);
        await page.goto(base + '/setup', { waitUntil: 'domcontentloaded', timeout: 60000 });
        await settle(page, 2500);
        await shot(page, 'c1-01-opened', {}, true);
        if (await clickText(page, /^Set up with the wizard$/)) { await settle(page); await shot(page, 'c1-02-wizard'); }
        await page.waitForSelector('input[name="setup-purpose"][value="web"]', { timeout: 60000 });
        await page.click('input[name="setup-purpose"][value="web"]');
        await pause(500);
        await shot(page, 'c1-03-purpose');
        await clickText(page, /^Continue/, 'main', 'button[type="submit"]');
        await page.waitForSelector('#setup-panel_domain', { timeout: 60000 });
        await fill(page, '#setup-panel_domain', LAB.panel_domain);
        await page.click('input[name="setup-dns"][value="external"]');
        await pause(800);
        await shot(page, 'c1-04-access', {}, true);
        await clickText(page, /^Review setup plan/, 'main', 'button[type="submit"]');
        const reviewed = await waitFor(page, () => !!document.getElementById('setup-plan-title'), 90000);
        await settle(page);
        summary.c1_review = reviewed ? await reviewBlocks(page) : { not_shown: true };
        await shot(page, 'c1-05-review', {}, true);
        if (!reviewed || summary.c1_review.blockers) { summary.c1_stopped = 'the review did not show a plan that can start'; await write(); return summary; }
        await page.click('section[aria-labelledby="setup-plan-title"] input[type="checkbox"]');
        await pause(500);
        phase = 'c1-run';
        summary.c1_start_pressed_at = iso();
        await clickText(page, /^Start reviewed setup$/);
        summary.c1 = await follow(page, 'c1', 75 * 60000, false);
        await leafRecord('after-c1');
        summary.c1_pin = await prep('refresh-pin', 120000);
        await write();
        if (!summary.c1.ready_at) return summary;
        await settle(page, 2000);
        await shot(page, 'c1-ready', {}, true);
        summary.c1_ready_variants = await variants(page, 'v-ready', null);
        summary.c1_setup_read = await prep('setup-read', 120000);
        await write();

        // ---- 3. the Components mail route, then "Stop extending"
        phase = 'c3-components';
        await page.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
        await settle(page, 3000);
        // Core Mail's card: the one whose component chips are Postfix and Dovecot only (the same names in EN and TR).
        const openDialog = async () => {
            const opened = await page.evaluate(() => {
                const card = Array.from(document.querySelectorAll('#mail-stacks article')).find(a => {
                    const chips = Array.from(a.querySelectorAll('span.rounded-md')).map(x => (x.innerText || '').trim()).join(' ');
                    return /Postfix/.test(chips) && /Dovecot/.test(chips) && !/Roundcube|Rspamd/i.test(chips);
                });
                const button = card ? Array.from(card.querySelectorAll('button')).find(b => !b.disabled) : null;
                if (!button) return null;
                button.scrollIntoView({ block: 'center' }); button.click();
                return { card: (card.querySelector('h3')?.innerText || '').trim(), button: (button.innerText || '').trim() };
            }).catch(() => null);
            await waitFor(page, () => !!document.querySelector('[data-mail-profile-route]'), 20000);
            return opened;
        };
        await page.waitForSelector('#mail-stacks', { timeout: 60000 }).catch(() => {});
        await shot(page, 'c3-01-components', {}, true);
        summary.c3_card_button = await openDialog();
        await settle(page, 1000);
        summary.c3_dialog = await page.evaluate(() => {
            const txt = el => el ? (el.innerText || '').replace(/\s+/g, ' ').trim() : null;
            const dialog = document.querySelector('[data-mail-profile-route]')?.closest('[role="dialog"]') || document.querySelector('[role="dialog"]');
            return { route: document.querySelector('[data-mail-profile-route]')?.getAttribute('data-mail-profile-route') || null, text: txt(dialog)?.slice(0, 4000),
                hostname_value: document.getElementById('mail-profile-hostname')?.value ?? null, buttons: Array.from(dialog?.querySelectorAll('button') || []).map(b => ({ text: txt(b), disabled: b.disabled })) };
        });
        await shot(page, 'c3-02-dialog', {}, true);
        summary.c3_dialog_variants = await variants(page, 'v-mail-dialog', async () => { await page.waitForSelector('#mail-stacks', { timeout: 60000 }).catch(() => {}); await settle(page, 1500); await openDialog(); });
        if (await page.$('#mail-profile-hostname')) await fill(page, '#mail-profile-hostname', LAB.mail_hostname);
        else if (await clickText(page, /^Change$/, '[role="dialog"]')) { await pause(500); await fill(page, '#mail-profile-hostname', LAB.mail_hostname); }
        await pause(800);
        await shot(page, 'c3-03-dialog-filled', {}, true);
        summary.c3_review_in_setup_pressed_at = iso();
        await clickText(page, /^Review in server setup$/, '[role="dialog"]');
        const landed = await waitFor(page, () => location.pathname === '/setup' && !!document.getElementById('setup-panel_domain'), 60000);
        await settle(page, 2000);
        summary.c3_landed = { access_step: landed, at: iso(), sample: await page.evaluate(SETUP_SAMPLE).catch(() => null),
            values: await page.evaluate(() => ({ panel_domain: document.getElementById('setup-panel_domain')?.value ?? null, mail_hostname: document.getElementById('setup-mail_hostname')?.value ?? null, dns: document.querySelector('input[name="setup-dns"]:checked')?.value ?? null })).catch(() => null) };
        await shot(page, 'c3-04-access-step', {}, true);
        summary.c3_access_variants = await variants(page, 'v-ext-access', null);
        await clickText(page, /^Review setup plan/, 'main', 'button[type="submit"]');
        const reviewed3 = await waitFor(page, () => !!document.getElementById('setup-plan-title'), 90000);
        await settle(page);
        summary.c3_review = reviewed3 ? await reviewBlocks(page) : { not_shown: true };
        await shot(page, 'c3-05-review', {}, true);
        if (reviewed3) summary.c3_review_variants = await variants(page, 'v-ext-review', async () => { await waitFor(page, () => !!document.getElementById('setup-plan-title'), 60000); await page.evaluate(() => { const d = document.querySelector('[data-setup-extension-review] details'); if (d) d.open = true; }); });
        // back to the access step: the secondary-DNS box (UI state only; "Stop extending" discards it)
        await clickText(page, /^Back$/);
        await page.waitForSelector('#setup-panel_domain', { timeout: 30000 }).catch(() => {});
        phase = 'c4-secondary-box';
        await page.click('input[name="setup-dns"][value="local"]').catch(() => {});
        await pause(600);
        await page.select('#setup-dns_role', 'secondary').catch(() => {});
        await pause(600);
        const nsInputs = await page.evaluate(() => Array.from(document.querySelectorAll('input[id^="setup-ns"], #setup-local_ip, #setup-peer_ip')).map(x => x.id));
        for (const id of nsInputs) {
            const value = id === 'setup-ns1' ? 'ns1.set18.test' : id === 'setup-ns2' ? 'ns2.set18.test' : id === 'setup-peer_ip' ? '192.0.2.20' : null;
            if (value) await fill(page, `#${id}`, value);
        }
        await pause(800);
        summary.c4_secondary_box = await page.evaluate(() => {
            const txt = el => el ? (el.innerText || '').replace(/\s+/g, ' ').trim() : null;
            const box = Array.from(document.querySelectorAll('main div')).find(d => /This secondary receives its DNS zones from/.test(d.innerText || '') && d.children.length <= 3);
            return { text: txt(box), inputs: Array.from(document.querySelectorAll('input[id^="setup-"], select[id^="setup-"]')).map(x => ({ id: x.id, value: x.value })) };
        });
        await page.evaluate(() => { const b = Array.from(document.querySelectorAll('main div')).find(d => /This secondary receives its DNS zones from/.test(d.innerText || '') && d.children.length <= 3); if (b) b.scrollIntoView({ block: 'center' }); });
        await shot(page, 'c4-secondary-box', {}, true);
        // stop extending: the ready screen comes back and nothing was installed
        phase = 'c3-stop';
        summary.c3_stop_pressed_at = iso();
        await clickText(page, /^Stop extending$/);
        const restored = await waitFor(page, () => !!document.querySelector('[data-setup-extend]'), 60000);
        await settle(page, 1500);
        summary.c3_stopped = { ready_screen: restored, at: iso(), sample: await page.evaluate(SETUP_SAMPLE).catch(() => null) };
        await shot(page, 'c3-06-ready-again', {}, true);
        summary.c3_setup_read = await prep('setup-read', 120000);
        await write();

        // ---- 2. the extension from the ready screen: Core Mail
        phase = 'c2-wizard';
        await clickText(page, /^Extend setup$/);
        await waitFor(page, () => !!document.querySelector('input[name="setup-purpose"]') || !!document.getElementById('setup-panel_domain'), 60000);
        await settle(page, 1500);
        await shot(page, 'c2-01-extending', {}, true);
        if (!(await page.$('input[name="setup-purpose"]'))) await clickText(page, /^Back$/);
        await clickText(page, /^Customize selection$/);
        await waitFor(page, () => !!document.querySelector('input[id^="setup-component-"]'), 60000);
        await settle(page, 1000);
        const mailGroup = await page.evaluate(() => {
            const rows = Array.from(document.querySelectorAll('input[id^="setup-component-"]'));
            const row = rows.find(x => /Postfix/i.test(x.closest('label')?.innerText || '') && /Dovecot/i.test(x.closest('label')?.innerText || '') && !/Roundcube|Rspamd/i.test(x.closest('label')?.innerText || ''))
                || rows.find(x => /Postfix/i.test(x.closest('label')?.innerText || ''));
            if (!row) return null;
            if (!row.checked) row.click();
            return { id: row.id, label: (row.closest('label')?.innerText || '').replace(/\s+/g, ' ').trim(), checked_now: row.checked };
        });
        summary.c2_mail_component = mailGroup;
        await pause(800);
        await shot(page, 'c2-02-components', {}, true);
        await clickText(page, /^Continue/, 'main', 'button[type="submit"]');
        await page.waitForSelector('#setup-mail_hostname', { timeout: 60000 });
        await fill(page, '#setup-mail_hostname', LAB.mail_hostname);
        await pause(600);
        await shot(page, 'c2-03-access', {}, true);
        await clickText(page, /^Review setup plan/, 'main', 'button[type="submit"]');
        const reviewed2 = await waitFor(page, () => !!document.getElementById('setup-plan-title'), 90000);
        await settle(page);
        summary.c2_review = reviewed2 ? await reviewBlocks(page) : { not_shown: true };
        await shot(page, 'c2-04-review', {}, true);
        if (!reviewed2 || summary.c2_review.blockers) { summary.c2_stopped = 'the review did not show a plan that can start'; await write(); return summary; }
        await page.click('section[aria-labelledby="setup-plan-title"] input[type="checkbox"]');
        await pause(500);
        phase = 'c2-run';
        summary.c2_start_pressed_at = iso();
        await clickText(page, /^Start reviewed setup$/);
        summary.c2 = await follow(page, 'c2', 90 * 60000, true);
        await leafRecord('after-c2');
        if (summary.c2.ready_at) { await settle(page, 2000); summary.c2_ready_variants = await variants(page, 'v-ready-after-extension', null); }
        summary.c2_setup_read = await prep('setup-read', 120000);
        summary.lab_state = await prep('lab-state', 120000);
        summary.ended_at = iso();
        await write();
        return summary;
    } catch (error) {
        summary.error = String(error.stack || error.message).slice(0, 2000);
        await shot(page, 'error', {}, true).catch(() => {});
        await write();
        return summary;
    }
}

// The page's own session makes one wizard request (a stand-in for a button D1 makes unreachable); recorded in full.
async function pageCall(page, label, method, path, body) {
    const value = await page.evaluate(async (m, p, b) => {
        const response = await fetch(p, { method: m, headers: b ? { 'Content-Type': 'application/json' } : {}, body: b ? JSON.stringify(b) : undefined, cache: 'no-store' });
        const text = await response.text();
        let json = null; try { json = JSON.parse(text); } catch { /* not JSON */ }
        return { status: response.status, json, text: json ? null : text.slice(0, 600) };
    }, method, path, body ?? null).catch(error => ({ error: String(error.message).slice(0, 300) }));
    await note('stand-in-request', { label, method, path, body, status: value.status, answer: value.json ?? value.text ?? value.error });
    return value;
}

async function cellResume() {
    const summary = { cell: hand.cell, lab: LAB, mode: 'resume', chrome: await browser.version(), started_at: iso() };
    const write = async () => writeFile(join(dir, 'resume-summary.json'), redact(JSON.stringify(summary, null, 2)));
    await note('start', { mode: 'resume' });
    const page = await newPage('owner-resume', 'en', '1280');
    await toFront(page);
    try {
        phase = 'r1-extension-opened';
        await signIn(page);
        await page.goto(base + '/setup', { waitUntil: 'domcontentloaded', timeout: 60000 });
        await settle(page, 6000);
        summary.r1_sample = await page.evaluate(SETUP_SAMPLE).catch(() => null);
        summary.r1_stop_button_present = await page.evaluate(() => !!document.querySelector('[data-setup-extend-stop]'));
        await shot(page, 'r1-extension-shows-completed-run', {}, true);
        summary.r1_state = await pageCall(page, 'read setup', 'GET', '/api/v1/setup');
        summary.r1_operation = await pageCall(page, 'read latest operation', 'GET', '/api/v1/setup/operation');
        // stand-in for "Stop extending" (not shown: the wizard believes an operation is open)
        phase = 'r2-stop-extending';
        const cancel = await pageCall(page, 'stand-in for Stop extending', 'POST', '/api/v1/setup/extend', { revision: summary.r1_state.json?.revision, action: 'cancel' });
        summary.r2_cancel = { status: cancel.status, state: cancel.json && { status: cancel.json.status, extension: cancel.json.extension, revision: cancel.json.revision, draft_components: cancel.json.draft?.customization || null, mail_hostname: cancel.json.draft?.mail_hostname ?? null } };
        await page.goto(base + '/setup', { waitUntil: 'domcontentloaded', timeout: 60000 });
        await settle(page, 4000);
        summary.r2_sample = await page.evaluate(SETUP_SAMPLE).catch(() => null);
        await shot(page, 'r2-ready-again', {}, true);
        summary.r2_operation = await pageCall(page, 'read latest operation after stopping', 'GET', '/api/v1/setup/operation');
        const services = await pageCall(page, 'read components after stopping', 'GET', '/api/v1/managed-services');
        summary.r2_services = services.json ? { status: services.status, installed: (services.json.services || []).filter(s => s.installed).map(s => s.id), mail_profiles: (services.json.mail_profiles || []).map(x => ({ id: x.id, status: x.status })) } : services;
        await write();
        // cell 2 from the ready screen: the owner presses "Extend setup"
        phase = 'r3-extend-from-ready';
        summary.r3_extend_clicked = await clickText(page, /^Extend setup$/);
        await settle(page, 6000);
        summary.r3_sample = await page.evaluate(SETUP_SAMPLE).catch(() => null);
        summary.r3_purpose_step_shown = await page.evaluate(() => !!document.querySelector('input[name="setup-purpose"]'));
        await shot(page, 'r3-after-extend-setup', {}, true);
        const state3 = await pageCall(page, 'read setup after Extend setup', 'GET', '/api/v1/setup');
        // stand-in for "Customize selection" -> Postfix + Dovecot -> mail host name -> Review (not shown under D1)
        const begin = await pageCall(page, 'stand-in for the wizard adding Core Mail and the mail host name', 'POST', '/api/v1/setup/extend', { revision: state3.json?.revision, action: 'begin', mail_profile: 'core-mail', mail_hostname: LAB.mail_hostname });
        summary.r3_begin = { status: begin.status, extension: begin.json?.extension, revision: begin.json?.revision, components: begin.json?.draft?.customization || null, purpose: begin.json?.draft?.purpose, mail_hostname: begin.json?.draft?.mail_hostname, base: begin.json?.extension_base ? { purpose: begin.json.extension_base.purpose, customization: begin.json.extension_base.customization || null, mail_hostname: begin.json.extension_base.mail_hostname || null } : null };
        const plan = await pageCall(page, 'stand-in for Review setup plan', 'POST', '/api/v1/setup/plan', { revision: begin.json?.revision });
        summary.r3_plan = plan.json ? { status: plan.status, id: plan.json.id, can_start: plan.json.can_start, blockers: plan.json.blockers, steps: (plan.json.steps || []).map(s => `${s.id} ${s.kind} ${s.target}${s.qualifier ? ' ' + s.qualifier : ''}`), components: plan.json.components, tcp: plan.json.tcp_ports, udp: plan.json.udp_ports, hostname_change: plan.json.hostname_change || null, contact: plan.json.contact_email } : plan;
        await writeFile(join(dir, 'r3-plan.json'), redact(JSON.stringify(plan.json, null, 2)));
        await write();
        if (!plan.json?.can_start) { summary.r3_stopped = 'the plan cannot start'; await write(); return summary; }
        phase = 'c2-run';
        const requestID = randomBytes(16).toString('hex');
        // the wizard's own start marker, so the page follows this request exactly as after its own start
        await page.evaluate((key, marker) => { try { localStorage.setItem(key, JSON.stringify(marker)); } catch { /* none */ } }, `celikpanel.setup.start.${secret.username}`, { request_id: requestID, plan_id: plan.json.id, revision: begin.json?.revision, panel_domain: LAB.panel_domain });
        summary.c2_start_pressed_at = iso();
        const start = await pageCall(page, 'stand-in for Start reviewed setup', 'POST', '/api/v1/setup/start', { plan_id: plan.json.id, request_id: requestID, confirmed: true });
        summary.c2_start = { status: start.status, execution: start.json && { id: start.json.id, status: start.json.status, phase: start.json.phase } };
        await page.goto(base + '/setup', { waitUntil: 'domcontentloaded', timeout: 60000 });
        await settle(page, 1500);
        summary.c2 = await follow(page, 'c2', 90 * 60000, true);
        await leafRecord('after-c2');
        await write();
        if (summary.c2.ready_at) {
            await settle(page, 2000);
            summary.c2_ready_variants = await variants(page, 'v-ready-after-extension', null);
            await page.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
            await settle(page, 3000);
            await page.evaluate(() => document.getElementById('mail-stacks')?.scrollIntoView({ block: 'start' }));
            await shot(page, 'c2-components-after', {}, true);
        }
        const end = await pageCall(page, 'read setup at the end', 'GET', '/api/v1/setup');
        summary.c2_state = { status: end.status, setup_status: end.json?.status, extension: end.json?.extension, checks: end.json?.checks };
        summary.lab_state = await prep('lab-state', 120000);
        summary.ended_at = iso();
        await write();
        return summary;
    } catch (error) {
        summary.error = String(error.stack || error.message).slice(0, 2000);
        await shot(page, 'resume-error', {}, true).catch(() => {});
        await write();
        return summary;
    }
}

for (const cell of cells) {
    if (cell === 'setup') await cellSetup();
    else if (cell === 'resume') await cellResume();
    else if (cell === 'done') { if (!existsSync(join(handDir, 'done.json'))) await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set18.mjs' }) + '\n'); }
}
await browser.close();
