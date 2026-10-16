// node cold-load-set18b.mjs <handover-dir> <out-dir> [setup,done]
// set18b (2026-10-16): the twenty-second native record. cold-load-set18.mjs (unchanged on disk) adapted to the
// candidate at the set18 D1/O1/O3 fix (6b363ac46): the owner's guided setup run to READY and the extension flow on
// screen, in a real headless Chrome against a REAL CelikPanel on a disposable lab guest whose lab has its own DNS and
// ACME server (deploy/e2e/release-recovery/guest_set18_lab_dns.py, a lab preparation; set18b_trial.py, set12's browser
// window protocol, window "setup"). Nothing but the loopback address of the driver's forward is opened.
//
//   cell "setup":
//     1  setup to ready (English, 1280x800): sign-in, "Set up with the wizard", purpose Web hosting, panel address
//        hand.lab.panel_domain, DNS external, review, confirm, start; the run is followed (the page sampled every
//        500 ms, every new state pictured) to "Your server is ready". The ready screen again in Turkish at 390x844.
//     3  the Components mail route: Components -> Core Mail -> Install -> the dialog's setup route, the mail host name
//        hand.lab.mail_hostname, "Review in server setup" -> where the wizard lands (D1: it must be the extension's
//        access step with the profile prefilled and "Stop extending"; nothing of the completed run) -> the
//        access step's secondary-DNS box (local DNS, role secondary; UI state only, set back to external, never
//        saved) -> "Stop extending" -> the ready screen; the driver reads the setup and its latest operation.
//     2  the extension from the ready screen: "Extend this server's setup" -> the purpose step (prefilled purpose
//        recorded) -> "Customize selection" -> the components step (each row's installed/kept text recorded) ->
//        Postfix + Dovecot -> the access step (mail host name) -> review ("What changes / To be installed / Already
//        installed" recorded; again in Turkish at 390x844) -> confirm, Start -> followed to ready: the queued
//        sentence (setup.guide.queued, O1), the mail host's public-DNS wait (the owner adds the A record by the
//        tool's own trigger when the run waits at that step: H3 fixed), the mail certificate, the mail enrollment
//        ("Continue recorded operation": each press and the button's state afterwards recorded; O3), the reverse DNS
//        wait (the owner sets it at the provider, then "Check requirements again"), "Finishing setup", ready.
//   cell "done": writes done.json
//
// set18 harness note H3 (fixed here): set18's trigger for the mail record compared the run's phase with the step id,
// but a waiting run reports the step's kind ("access_dns"); the trigger now takes the mail host's access_dns step
// while it is the running step of a waiting run, whatever the phase names.
//
//   BROWSER_INSPECT_MODULES  directory whose node_modules holds puppeteer-core (required; outside the repository)
//   CHROME_PATH              the installed Chrome (default: the usual Windows/Linux places)
//
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

const [handDir, outRoot, cellsArg = 'setup,done'] = process.argv.slice(2);
if (!handDir || !outRoot) { console.error('usage: node cold-load-set18b.mjs <handover-dir> <out-dir> <cells>'); process.exit(2); }
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

// servername null: no SNI (an IP address in the URL), set18 O2.
function leafOf(servername = 'localhost') {
    return new Promise((done, fail) => {
        const socket = tlsConnect({ host: '127.0.0.1', port, rejectUnauthorized: false, ...(servername ? { servername } : {}) }, () => {
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
const leafRecord = async (label, servername = 'localhost') => { try { const leaf = await leafOf(servername); await jsonl(join(dir, 'leaf.jsonl'), { at: iso(), label, sni: servername, ...leaf, handover_leaf_sha256: hand.tls_leaf_sha256 }); return leaf; } catch (error) { await jsonl(join(dir, 'leaf.jsonl'), { at: iso(), label, error: String(error.message).slice(0, 120) }); return null; } };
await leafRecord('start'); await leafRecord('start-no-sni', null);
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
// Every POST of the mail enrollment continuation the page sends (set18 O3): counted from the page's own requests.
const enrollPosts = [];
const BODY_PREFIXES = ['/api/v1/setup', '/api/v1/panel/version'];

async function newPage(label, lang = 'en', vp = '1280') {
    const page = await browser.newPage();
    page.__label = label;
    await page.setViewport(VIEWPORTS[vp]);
    await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }]);
    // The language follows a per-tab value (sessionStorage) so that a reload can switch it.
    await page.evaluateOnNewDocument(value => { try { const chosen = sessionStorage.getItem('set18b.lang') || value; localStorage.setItem('celikpanel.lang', chosen); localStorage.setItem('celikpanel.theme', 'light'); } catch { /* none */ } }, lang);
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
        if (entry.method === 'POST' && path0.startsWith('/api/v1/setup/mail-enrollment/')) enrollPosts.push({ at: entry.start, url: entry.url, status: entry.status, body: entry.body ? entry.body.slice(0, 600) : null, request_body: entry.request_body || null });
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
const ENROLL_BUTTON = /Continue recorded operation|Retry reviewed handoff|Checking the same operation|Kayıtlı işleme devam et|Aynı işlem denetleniyor/;
const enrollButtons = s => (s.buttons || []).filter(b => ENROLL_BUTTON.test(b.text));
const signature = s => JSON.stringify([s.h1, s.progress_title, s.guidance_title, !!s.finishing, s.steps.map(x => x.state), s.checks.map(x => `${x.id}:${x.state}`), !!s.reconnecting, !!s.reconnect_slow, s.alerts.length, (s.guidance || '').slice(0, 160), enrollButtons(s).map(b => `${b.text}:${b.disabled}`)]);
const op = () => latest.operation?.value || null;
const opStep = (execution, kind, target) => (execution?.steps || []).find(x => x.kind === kind && (!target || x.target === target));
const QUEUED = /Setup has started with the step shown here|Kurulum burada gösterilen adımla başladı/;
const UNKNOWN = /No more specific requirement has been confirmed for this step|daha belirli bir gereksinim henüz doğrulanmadı/;

// Follow a started run until the server is ready (or failed, or the bound). The page is sampled every 500 ms.
async function follow(page, label, limitMs, mail) {
    const started = Date.now();
    const result = { label, started_at: iso(started), sample_ms: 500, states: 0, pictures: [], owner_actions: [], lab: [], enrollment_presses: [], enrollment_button_samples: 0, unknown_sentence_samples: [], enroll_posts_before: enrollPosts.length };
    let lastSig = null; let shots = 0; let lastHeartbeat = 0;
    let mailWaitSince = null; let ptrSeenAt = null; let lastVerifyPress = 0; let waitingVerifySince = null; let slowSince = null;
    let done = false;
    while (!done && Date.now() - started < limitMs) {
        const t0 = Date.now();
        let s;
        try { s = await page.evaluate(SETUP_SAMPLE); } catch (error) { s = { t: t0, error: String(error.message).slice(0, 160) }; }
        const execution = op();
        if (!s.error) {
            s.operation = execution ? { status: execution.status, phase: execution.phase, error: execution.error?.code || null, superseded: execution.superseded ?? null, steps: (execution.steps || []).map(x => `${x.id}:${x.status}`) } : null;
            s.enroll_buttons = enrollButtons(s);
            if (s.enroll_buttons.length) result.enrollment_button_samples++;
            const sig = signature(s);
            if (sig !== lastSig || Date.now() - lastHeartbeat > 30000) {
                lastHeartbeat = Date.now();
                if (sig !== lastSig) { result.states++; s.new_state = true; }
                if (sig !== lastSig && shots < 90 && s.vis === 'visible') { shots++; s.picture = await shot(page, `${label}-state-${String(shots).padStart(2, '0')}`, { t_plus_s: Math.round((Date.now() - started) / 1000) }); if (s.picture) result.pictures.push(s.picture); }
                lastSig = sig;
                await jsonl(join(dir, `${label}-samples.jsonl`), s);
            }
            if (QUEUED.test(s.guidance || '') && !result.queued_seen) result.queued_seen = { at: iso(), guidance_title: s.guidance_title, guidance: s.guidance, operation: s.operation };
            if (UNKNOWN.test(s.guidance || '') && result.unknown_sentence_samples.length < 20) result.unknown_sentence_samples.push({ at: iso(), operation: s.operation });
            if (s.finishing && !result.finishing_seen) { result.finishing_seen = { at: iso(), text: s.finishing }; }
            if (/ready|hazır/i.test(s.h1 || '') && s.extend_section) { result.ready_at = iso(); done = true; break; }
            if (execution?.status === 'failed') { result.failed = { at: iso(), error: execution.error }; if (!result.failed_picture) result.failed_picture = await shot(page, `${label}-failed`, {}, true); done = true; break; }
            // the reconnect has been slow for a minute: the owner reloads the page, as it offers
            if (s.reconnect_slow) { slowSince = slowSince || Date.now(); if (Date.now() - slowSince > 60000) { result.owner_actions.push({ at: iso(), action: 'reload (Reload this page)' }); await clickText(page, /Reload this page|Sayfayı yeniden yükle/); slowSince = null; await settle(page); } } else slowSince = null;
            // a recorded mail enrollment wait offers the owner's explicit action: pressed whenever it is enabled (O3:
            // after one press it must be disabled and say "Checking the same operation"); the state after each press
            // is read 300 ms and 2 s later.
            if (s.buttons.some(b => /Continue recorded operation|Retry reviewed handoff/.test(b.text) && !b.disabled)) {
                const before = s.enroll_buttons;
                const c = await clickText(page, /^(Continue recorded operation|Retry reviewed handoff)$/);
                const press = { at: iso(), clicked: c, before, operation: s.operation, posts_before: enrollPosts.length };
                await pause(300);
                press.after_300ms = enrollButtons(await page.evaluate(SETUP_SAMPLE).catch(() => ({ buttons: [] })));
                await pause(1700);
                press.after_2s = enrollButtons(await page.evaluate(SETUP_SAMPLE).catch(() => ({ buttons: [] })));
                press.posts_after_2s = enrollPosts.length;
                result.enrollment_presses.push(press);
                result.owner_actions.push({ at: press.at, action: c });
                await shot(page, `${label}-enrollment-after-press`, { press: result.enrollment_presses.length });
            }
            if (mail && execution) {
                // H3 fixed: the mail host's access_dns step while it is the running step of a waiting run
                const mailDNS = opStep(execution, 'access_dns', LAB.mail_hostname);
                const atMailDNS = execution.status === 'waiting' && mailDNS && mailDNS.status === 'running' && [mailDNS.id, mailDNS.kind].includes(execution.phase);
                if (atMailDNS && !result.lab.some(x => x.what === 'lab-mail-a')) {
                    mailWaitSince = mailWaitSince || Date.now();
                    result.mail_dns_wait_seen = result.mail_dns_wait_seen || { at: iso(), phase: execution.phase, step: mailDNS, guidance_title: s.guidance_title, guidance: s.guidance };
                    if (Date.now() - mailWaitSince > 50000) {
                        await shot(page, `${label}-mail-dns-waiting`, {}, true);
                        const r = await prep('lab-mail-a', 120000);
                        result.lab.push({ at: iso(), what: 'lab-mail-a', ok: !r.result?.error && !r.missing, trigger: 'tool (H3 fixed)', waited_s: Math.round((Date.now() - mailWaitSince) / 1000) });
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
                        result.lab.push({ at: iso(), what: 'lab-ptr-mail', ok: !r.result?.error && !r.missing });
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
        await pause(Math.max(0, 500 - (Date.now() - t0)));
    }
    result.ended_at = iso();
    result.duration_s = Math.round((Date.now() - started) / 1000);
    result.final_operation = op();
    result.enroll_posts = enrollPosts.slice(result.enroll_posts_before);
    result.final_picture = await shot(page, `${label}-final`, {}, true);
    if (!result.ready_at && !result.failed) result.not_finished = `not ready within ${Math.round(limitMs / 60000)} min`;
    await writeFile(join(dir, `${label}-summary.json`), redact(JSON.stringify(result, null, 2)));
    return result;
}

// One picture per language and viewport (default: Turkish at 390x844); reopen() brings the page back after a reload.
async function variants(page, name, reopen, combos = [['tr', 'phone']]) {
    const out = [];
    for (const [lang, vp] of combos) {
        try {
            await page.setViewport(VIEWPORTS[vp]);
            await page.evaluate(value => sessionStorage.setItem('set18b.lang', value), lang);
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
    await page.evaluate(() => sessionStorage.setItem('set18b.lang', 'en')).catch(() => {});
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

// What the wizard shows now: which step, the extension marks, and the set18 D1 screen's marks.
const WIZARD_VIEW = () => {
    const txt = el => el ? (el.innerText || '').replace(/\s+/g, ' ').trim() : null;
    const main = document.querySelector('main') || document.body;
    return {
        at: Date.now(), path: location.pathname, h1: txt(main.querySelector('h1')), intro: txt(main.querySelector('h1')?.nextElementSibling),
        step: document.getElementById('setup-plan-title') ? 'review' : document.getElementById('setup-panel_domain') ? 'access'
            : document.querySelector('input[id^="setup-component-"]') ? 'components' : document.querySelector('input[name="setup-purpose"]') ? 'purpose'
                : document.getElementById('setup-progress-title') ? 'progress' : document.querySelector('[data-setup-extend]') ? 'ready' : 'other',
        purpose_checked: document.querySelector('input[name="setup-purpose"]:checked')?.value ?? null,
        values: { panel_domain: document.getElementById('setup-panel_domain')?.value ?? null, mail_hostname: document.getElementById('setup-mail_hostname')?.value ?? null, dns: document.querySelector('input[name="setup-dns"]:checked')?.value ?? null },
        extend_stop: txt(document.querySelector('[data-setup-extend-stop]')), extend_section: txt(document.querySelector('[data-setup-extend]')),
        progress_title: txt(document.getElementById('setup-progress-title')),
        completed_run_marks: /Completion could not be verified|Reconnect to this operation|Verifying the result/.test(main.innerText || ''),
        alerts: Array.from(document.querySelectorAll('[role="alert"]')).filter(a => a.getClientRects().length).map(a => txt(a).slice(0, 600)),
        buttons: Array.from(main.querySelectorAll('button, a')).filter(b => b.getClientRects().length).map(b => ({ text: txt(b), disabled: !!b.disabled })).filter(b => b.text).slice(0, 30),
        overflow_x: document.documentElement.scrollWidth > document.documentElement.clientWidth,
    };
};
const COMPONENT_ROWS = () => Array.from(document.querySelectorAll('input[id^="setup-component-"]')).map(x => ({ id: x.id, checked: x.checked, disabled: x.disabled, text: (x.closest('label')?.innerText || '').replace(/\s+/g, ' ').trim() }));
const view = async (page, label) => { const value = await page.evaluate(WIZARD_VIEW).catch(error => ({ error: String(error.message).slice(0, 160) })); await note('view', { label, view: value }); return value; };
// The page's own read (GET only), recorded in full.
async function pageRead(page, label, path) {
    const value = await page.evaluate(async p => {
        const response = await fetch(p, { cache: 'no-store' });
        const text = await response.text();
        let json = null; try { json = JSON.parse(text); } catch { /* not JSON */ }
        return { status: response.status, json, text: json ? null : text.slice(0, 600) };
    }, path).catch(error => ({ error: String(error.message).slice(0, 300) }));
    await note('page-read', { label, path, status: value.status, answer: value.json ?? value.text ?? value.error });
    return value;
}
const installedOf = services => services.json ? { status: services.status, installed: (services.json.services || []).filter(s => s.installed).map(s => s.id), mail_profiles: (services.json.mail_profiles || []).map(x => ({ id: x.id, status: x.status })) } : services;

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
        await leafRecord('after-c1', 'localhost'); await leafRecord('after-c1-no-sni', null);
        summary.c1_pin = await prep('refresh-pin', 120000);
        await write();
        if (!summary.c1.ready_at) return summary;
        await settle(page, 2000);
        await shot(page, 'c1-ready', {}, true);
        summary.c1_ready_variants = await variants(page, 'v-ready', null);
        summary.c1_setup_read = await prep('setup-read', 120000);
        await write();

        // ---- 3. the Components mail route: where the wizard lands (D1), then "Stop extending"
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
        if (await page.$('#mail-profile-hostname')) await fill(page, '#mail-profile-hostname', LAB.mail_hostname);
        else if (await clickText(page, /^Change$/, '[role="dialog"]')) { await pause(500); await fill(page, '#mail-profile-hostname', LAB.mail_hostname); }
        await pause(800);
        await shot(page, 'c3-03-dialog-filled', {}, true);
        summary.c3_review_in_setup_pressed_at = iso();
        await clickText(page, /^Review in server setup$/, '[role="dialog"]');
        await waitFor(page, () => location.pathname === '/setup' && !!(document.getElementById('setup-panel_domain') || document.getElementById('setup-progress-title') || document.querySelector('input[name="setup-purpose"]')), 60000);
        await settle(page, 2500);
        // Sampled for 6 s at 500 ms: the landing must stay on the access step (D1 put the completed run here).
        summary.c3_landing_samples = [];
        for (let i = 0; i < 12; i++) { summary.c3_landing_samples.push(await page.evaluate(WIZARD_VIEW).catch(() => null)); await pause(500); }
        summary.c3_landed = await view(page, 'c3 landed after Review in server setup');
        summary.c3_d1 = { access_step: summary.c3_landed.step === 'access', mail_hostname_prefilled: summary.c3_landed.values?.mail_hostname === LAB.mail_hostname,
            stop_extending_shown: !!summary.c3_landed.extend_stop, completed_run_marks: summary.c3_landing_samples.some(x => x?.completed_run_marks),
            steps_seen: [...new Set(summary.c3_landing_samples.map(x => x?.step))] };
        await shot(page, 'c3-04-access-step', {}, true);
        // the access step's secondary-DNS box: local DNS, role secondary (UI state only), then back to external
        phase = 'c4-secondary-box';
        if (summary.c3_landed.step === 'access') {
            summary.c4_dns_before = summary.c3_landed.values?.dns;
            await page.click('input[name="setup-dns"][value="local"]').catch(error => note('c4-local-refused', { error: String(error.message).slice(0, 160) }));
            await pause(600);
            await page.select('#setup-dns_role', 'secondary').catch(() => {});
            await pause(800);
            summary.c4_secondary_box = await page.evaluate(() => {
                const txt = el => el ? (el.innerText || '').replace(/\s+/g, ' ').trim() : null;
                const box = Array.from(document.querySelectorAll('main div')).find(d => /This secondary receives its DNS zones from/.test(d.innerText || '') && d.children.length <= 3);
                if (box) box.scrollIntoView({ block: 'center' });
                return { text: txt(box), dns: document.querySelector('input[name="setup-dns"]:checked')?.value ?? null, role: document.getElementById('setup-dns_role')?.value ?? null };
            });
            await shot(page, 'c4-secondary-box', {}, true);
            if (summary.c4_dns_before) await page.click(`input[name="setup-dns"][value="${summary.c4_dns_before}"]`).catch(() => {});
            await pause(600);
            summary.c4_dns_after = await page.evaluate(() => document.querySelector('input[name="setup-dns"]:checked')?.value ?? null);
        } else summary.c4_secondary_box = { not_reached: `the wizard landed on ${summary.c3_landed.step}` };
        // stop extending: the ready screen comes back and nothing was installed
        phase = 'c3-stop';
        summary.c3_stop_pressed_at = iso();
        summary.c3_stop_clicked = await clickText(page, /^Stop extending$/);
        const restored = await waitFor(page, () => !!document.querySelector('[data-setup-extend]'), 60000);
        await settle(page, 1500);
        summary.c3_stopped = { ready_screen: restored, at: iso(), view: await view(page, 'c3 after Stop extending') };
        await shot(page, 'c3-05-ready-again', {}, true);
        summary.c3_setup_read = await prep('setup-read', 120000);
        summary.c3_components_after_stop = installedOf(await pageRead(page, 'components after Stop extending', '/api/v1/managed-services'));
        await write();

        // ---- 2. the extension from the ready screen: Core Mail
        phase = 'c2-wizard';
        summary.c2_extend_clicked = await clickText(page, /^Extend setup$/);
        await waitFor(page, () => !!document.querySelector('input[name="setup-purpose"]') || !!document.getElementById('setup-panel_domain') || !!document.getElementById('setup-progress-title'), 60000);
        await settle(page, 1500);
        summary.c2_purpose = await view(page, 'c2 after Extend setup');
        await shot(page, 'c2-01-purpose', {}, true);
        if (summary.c2_purpose.step !== 'purpose') { summary.c2_stopped = `Extend setup opened ${summary.c2_purpose.step}, not the purpose step`; await write(); return summary; }
        summary.c2_customize_clicked = await clickText(page, /^Customize selection$/);
        await waitFor(page, () => !!document.querySelector('input[id^="setup-component-"]'), 60000);
        await settle(page, 1000);
        summary.c2_components = { view: await view(page, 'c2 components step'), rows: await page.evaluate(COMPONENT_ROWS) };
        await shot(page, 'c2-02-components', {}, true);
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
        summary.c2_components_after = await page.evaluate(COMPONENT_ROWS);
        await shot(page, 'c2-03-components-mail', {}, true);
        await clickText(page, /^Continue/, 'main', 'button[type="submit"]');
        await page.waitForSelector('#setup-mail_hostname', { timeout: 60000 });
        summary.c2_access_before_fill = await view(page, 'c2 access step');
        await fill(page, '#setup-mail_hostname', LAB.mail_hostname);
        await pause(600);
        summary.c2_access = await view(page, 'c2 access step filled');
        await shot(page, 'c2-04-access', {}, true);
        await clickText(page, /^Review setup plan/, 'main', 'button[type="submit"]');
        const reviewed2 = await waitFor(page, () => !!document.getElementById('setup-plan-title'), 90000);
        await settle(page);
        summary.c2_review = reviewed2 ? await reviewBlocks(page) : { not_shown: true, view: await view(page, 'c2 review not shown') };
        await shot(page, 'c2-05-review', {}, true);
        if (!reviewed2 || summary.c2_review.blockers) { summary.c2_stopped = 'the review did not show a plan that can start'; await write(); return summary; }
        const backToReview = async () => {
            if (!(await waitFor(page, () => !!document.getElementById('setup-plan-title'), 15000))) {
                await note('review-not-restored-after-reload');
                if (await page.$('#setup-panel_domain')) { await clickText(page, /^(Review setup plan|Kurulum planını incele)/, 'main', 'button[type="submit"]'); await waitFor(page, () => !!document.getElementById('setup-plan-title'), 90000); }
            }
            await page.evaluate(() => { const d = document.querySelector('[data-setup-extension-review] details'); if (d) d.open = true; });
        };
        summary.c2_review_variants = await variants(page, 'v-ext-review', backToReview);
        summary.c2_review_after_variants = await view(page, 'c2 review after the Turkish picture');
        if (summary.c2_review_after_variants.step !== 'review') { summary.c2_stopped = 'the review did not come back after the Turkish picture'; await write(); return summary; }
        await page.click('section[aria-labelledby="setup-plan-title"] input[type="checkbox"]');
        await pause(500);
        phase = 'c2-run';
        summary.c2_start_pressed_at = iso();
        summary.c2_start_clicked = await clickText(page, /^Start reviewed setup$/);
        summary.c2 = await follow(page, 'c2', 90 * 60000, true);
        await leafRecord('after-c2', 'localhost'); await leafRecord('after-c2-no-sni', null);
        await write();
        if (summary.c2.ready_at) {
            await settle(page, 2000);
            await shot(page, 'c2-ready', {}, true);
            summary.c2_ready_variants = await variants(page, 'v-ready-after-extension', null);
            await page.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
            await settle(page, 3000);
            await page.evaluate(() => document.getElementById('mail-stacks')?.scrollIntoView({ block: 'start' }));
            await shot(page, 'c2-components-after', {}, true);
            summary.c2_components_after_run = installedOf(await pageRead(page, 'components after the extension', '/api/v1/managed-services'));
        }
        summary.c2_setup_read = await prep('setup-read', 120000);
        summary.lab_state = await prep('lab-state', 120000);
        summary.enroll_posts_total = enrollPosts.length;
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

for (const cell of cells) {
    if (cell === 'setup') await cellSetup();
    else if (cell === 'done') { if (!existsSync(join(handDir, 'done.json'))) await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set18b.mjs' }) + '\n'); }
}
await browser.close();
