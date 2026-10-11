// node cold-load-set13.mjs <handover-dir> <out-dir> <cell[,cell...]>
// set13 (2026-10-11): commit 816b8dd60's answer to a refused choice and the Domains row of a file that was not read, in a
// real headless Chrome against a REAL CelikPanel on a disposable lab guest, reached through the lab driver's loopback
// SSH forward (deploy/e2e/release-recovery/set13_trial.py, set12's browser window "ui"). Nothing but the loopback
// address of the forward is opened. The driver prepares each state on request (prep-request-<name>-<n> -> prep-done).
//
//   cells: ui    the refused "take" on an owner-edited pool while an owner's pool file makes php-fpm -t fail: after
//                the click, the answer ([data-site-config-refusal], role=status) and whether its box is inside the
//                viewport (EN desktop, TR phone; viewport and full-page pictures); then the Domains row while the pool
//                was not read ("could not be read", EN and TR, desktop and phone), and after the page was read again
//          done  writes done.json
//
//   BROWSER_INSPECT_MODULES  directory whose node_modules holds puppeteer-core (required; outside the repository)
//   CHROME_PATH              the installed Chrome (default: the usual Windows/Linux places)
//   LIVE_PROFILE_ROOT        a short directory for the browser profile (profile-dir.mjs; refused beyond 120 characters)
//
// The owner's password is read from the root-only file the driver names and is typed into the sign-in form; every
// text this script writes is passed through the redaction list of that file first. Cookies are never read. Every
// picture is taken with the form empty or after sign-in.
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
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
if (!handDir || !outRoot) { console.error('usage: node cold-load-set13.mjs <handover-dir> <out-dir> <cells>'); process.exit(2); }
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

async function newPage(dir, label, { lang = 'en', viewport = DESKTOP } = {}) {
    const page = await browser.newPage();
    await page.setViewport(viewport);
    await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }]);
    await page.evaluateOnNewDocument(value => { try { localStorage.setItem('celikpanel.lang', value); localStorage.setItem('celikpanel.theme', 'light'); } catch { /* none */ } }, lang);
    const net = join(dir, `network-${label}.jsonl`);
    page.on('response', res => { if (res.url().includes('/api/')) void jsonl(net, { at: iso(), method: res.request().method(), url: res.url().replace(base, ''), status: res.status() }); });
    page.on('requestfailed', req => { if (req.url().includes('/api/')) void jsonl(net, { at: iso(), ev: 'failed', method: req.method(), url: req.url().replace(base, ''), error: req.failure()?.errorText }); });
    page.on('console', message => { if (['error', 'warn', 'warning'].includes(message.type())) void jsonl(join(dir, `console-${label}.jsonl`), { at: iso(), type: message.type(), text: message.text().slice(0, 400) }); });
    return page;
}

async function signIn(page, dir) {
    await page.goto(base + '/', { waitUntil: 'networkidle2', timeout: 60000 });
    const shown = await page.waitForFunction(() => document.getElementById('username') ? 'form' : document.querySelector('nav, aside') ? 'signed-in' : null, { timeout: 60000 }).then(handle => handle.jsonValue());
    await jsonl(join(dir, 'events.jsonl'), { at: iso(), ev: 'sign-in-page', shown });
    if (shown === 'signed-in') return;
    await page.type('#username', secret.username);
    await page.type('#password', secret.password);
    await Promise.all([page.keyboard.press('Enter'), pause(500)]);
    await page.waitForFunction(() => !document.getElementById('password'), { timeout: 60000 });
    await pause(2500);
}

async function prep(name, events) {
    const key = `${name}-${Date.now()}`;
    await writeFile(join(handDir, `prep-request-${key}`), iso() + '\n');
    await jsonl(events, { at: iso(), ev: 'prep-requested', name });
    for (let i = 0; i < 900 && !existsSync(join(handDir, `prep-done-${key}.json`)); i++) await pause(1000);
    const record = existsSync(join(handDir, `prep-done-${key}.json`)) ? JSON.parse(readFileSync(join(handDir, `prep-done-${key}.json`), 'utf8')) : { missing: true };
    await jsonl(events, { at: iso(), ev: 'prep-done', name, record });
    return record;
}

async function clickText(page, texts) {
    return page.evaluate(wanted => {
        const nodes = Array.from(document.querySelectorAll('button, a, [role="tab"], [role="button"]'));
        for (const text of wanted) {
            const hit = nodes.find(n => (n.innerText || '').trim() === text && n.getClientRects().length > 0);
            if (hit) { hit.click(); return text; }
        }
        return null;
    }, texts);
}
async function pageText(page) {
    return page.evaluate(() => (document.querySelector('main') || document.body).innerText.replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n').trim().slice(0, 8000));
}
async function shot(page, dir, name, events, extra = {}) {
    await page.screenshot({ path: join(dir, `${name}.png`), fullPage: true });
    await writeFile(join(dir, `${name}.txt`), redact(await pageText(page)));
    const facts = await page.evaluate(() => ({
        path: location.pathname + location.search,
        files: Array.from(document.querySelectorAll('[data-site-config-files-row]')).map(n => ({ kind: n.getAttribute('data-site-config-files-row'), text: n.innerText.trim().slice(0, 200) })),
        states: Array.from(document.querySelectorAll('[data-site-config-files-state]')).map(n => n.getAttribute('data-site-config-files-state')),
        notice: Array.from(document.querySelectorAll('[data-site-config-notice]')).map(n => ({ state: n.getAttribute('data-site-config-notice'), file: n.getAttribute('data-site-config-notice-file'), text: n.innerText.trim().slice(0, 300) })),
        alerts: Array.from(document.querySelectorAll('[role="alert"]')).map(n => n.innerText.trim().slice(0, 600)).filter(Boolean),
        done: Array.from(document.querySelectorAll('[data-site-config-done]')).map(n => n.innerText.trim().slice(0, 300)),
        apply: !!document.querySelector('[data-site-config-apply]'),
        first_state: (document.querySelector('[data-site-config-first-state]')?.innerText || '').trim() || null,
        password_fields_with_text: Array.from(document.querySelectorAll('input[type="password"]')).filter(i => i.value).length,
    })).catch(error => ({ error: String(error.message).slice(0, 200) }));
    await jsonl(events, { at: iso(), ev: 'shot', name, ...extra, facts });
    return facts;
}
async function openConfig(page, lang) {
    const direct = await clickText(page, lang === 'tr' ? ['Yapılandırma dosyasını aç'] : ['Open configuration file']);
    if (direct) return direct;
    await clickText(page, lang === 'tr' ? ['Barındırma'] : ['Hosting']);
    await pause(1500);
    return clickText(page, lang === 'tr' ? ['Yapılandırma dosyası'] : ['Configuration file']);
}
async function domainConfig(page, domain, lang) {
    await page.goto(base + '/domains/' + encodeURIComponent(domain), { waitUntil: 'networkidle2', timeout: 60000 });
    await pause(3000);
    const by = await openConfig(page, lang);
    await pause(6000);
    return by;
}
async function takePool(page, events, label) {
    const first = await page.$('[data-site-config-file="php_pool"] [data-site-config-take]');
    if (!first) { await jsonl(events, { at: iso(), ev: 'take-button', label, found: false }); return false; }
    await first.click();
    await pause(800);
    const confirm = await page.$('[data-site-config-file="php_pool"] [data-site-config-take]');
    const before = await page.evaluate(() => ({ scrollY: Math.round(window.scrollY), innerHeight: window.innerHeight }));
    await confirm.click();
    await jsonl(events, { at: iso(), ev: 'take-confirmed', label, before });
    await pause(12000);
    return true;
}
async function noticeFacts(page) {
    return page.evaluate(() => {
        const box = el => { if (!el) return null; const r = el.getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), height: Math.round(r.height) }; };
        const refusal = document.querySelector('[data-site-config-refusal]');
        const notice = document.querySelector('[data-site-config-notice]');
        const vh = window.innerHeight, vw = window.innerWidth;
        const b = box(refusal);
        return {
            viewport: { width: vw, height: vh, scrollY: Math.round(window.scrollY) },
            refusal: refusal ? { role: refusal.getAttribute('role'), restore: refusal.getAttribute('data-site-config-refusal'), text: refusal.innerText.trim().slice(0, 700), box: b,
                top_in_view: b ? b.top >= 0 && b.top < vh : false, fully_in_view: b ? b.top >= 0 && b.bottom <= vh && b.left >= 0 && b.right <= vw : false } : null,
            notice_box: box(notice),
            take_button_box: box(document.querySelector('[data-site-config-file="php_pool"] [data-site-config-take]')),
            password_fields_with_text: Array.from(document.querySelectorAll('input[type="password"]')).filter(i => i.value).length,
        };
    }).catch(error => ({ error: String(error.message).slice(0, 200) }));
}
async function badgeFacts(page, domain) {
    return page.evaluate(name => {
        const badges = Array.from(document.querySelectorAll('[data-site-config-badge]')).map(n => {
            const row = n.closest('tr, li, [role="row"]') || n.parentElement;
            const r = n.getBoundingClientRect();
            return { state: n.getAttribute('data-site-config-badge'), file: n.getAttribute('data-site-config-badge-file'), text: n.innerText.trim(),
                row_text: (row?.innerText || '').trim().slice(0, 200), visible: r.width > 0 && r.height > 0 };
        });
        return { badges, for_domain: badges.filter(b => b.row_text.includes(name)),
            password_fields_with_text: Array.from(document.querySelectorAll('input[type="password"]')).filter(i => i.value).length };
    }, domain).catch(error => ({ error: String(error.message).slice(0, 200) }));
}
async function viewShot(page, dir, name, events, facts) {
    await page.screenshot({ path: join(dir, `${name}-viewport.png`), fullPage: false });
    await page.screenshot({ path: join(dir, `${name}-full.png`), fullPage: true });
    await writeFile(join(dir, `${name}.txt`), redact(await pageText(page)));
    await jsonl(events, { at: iso(), ev: 'shot', name, facts });
}

async function cellUI(dir) {
    const events = join(dir, 'events.jsonl');
    const domain = (hand.domains || [])[0]?.name;
    const signer = await newPage(dir, 'sign-in');
    await signIn(signer, dir);
    await signer.close();
    // 1. the refused take: owner-edited pool + an owner's pool file php-fpm -t refuses
    await prep('pool-edited', events);
    await prep('broken-add', events);
    for (const [lang, vpName, viewport] of [['en', 'desktop', DESKTOP], ['tr', 'phone', PHONE]]) {
        const page = await newPage(dir, `refused-${lang}-${vpName}`, { lang, viewport });
        await domainConfig(page, domain, lang);
        await shot(page, dir, `refused-${lang}-${vpName}-1-before`, events, { lang, viewport: vpName });
        const clicked = await takePool(page, events, `${lang}-${vpName}`);
        const facts = await noticeFacts(page);
        await viewShot(page, dir, `refused-${lang}-${vpName}-2-after-click`, events, { clicked, lang, viewport: vpName, ...facts });
        await page.close();
        await prep('php-state', events);
    }
    await prep('broken-remove', events);
    await prep('end', events);
    // 2. the Domains row while the pool was not read, then after the page read it again
    await prep('u-unread', events);
    for (const [lang, vpName, viewport] of [['en', 'desktop', DESKTOP], ['tr', 'desktop', DESKTOP], ['en', 'phone', PHONE], ['tr', 'phone', PHONE]]) {
        const page = await newPage(dir, `unread-${lang}-${vpName}`, { lang, viewport });
        await page.goto(base + '/domains', { waitUntil: 'networkidle2', timeout: 60000 });
        await pause(4000);
        const facts = await badgeFacts(page, domain);
        await viewShot(page, dir, `unread-${lang}-${vpName}-domains`, events, { lang, viewport: vpName, ...facts });
        await page.close();
    }
    await prep('rows', events);
    const page = await newPage(dir, 'reread-en', { lang: 'en' });
    await domainConfig(page, domain, 'en');
    await shot(page, dir, 'reread-en-desktop-1-config', events);
    await page.goto(base + '/domains', { waitUntil: 'networkidle2', timeout: 60000 });
    await pause(4000);
    await viewShot(page, dir, 'reread-en-desktop-2-domains', events, await badgeFacts(page, domain));
    await page.close();
    await prep('rows', events);
}

const results = {};
for (const cell of cells) {
    const dir = join(outRoot, cell);
    await mkdir(dir, { recursive: true });
    const started = iso();
    try {
        if (cell === 'ui') await cellUI(dir);
        else if (cell === 'done') await writeFile(join(handDir, 'done.json'), JSON.stringify({ at: iso(), by: 'cold-load-set13.mjs' }) + '\n');
        else throw new Error(`unknown cell ${cell}`);
        results[cell] = { started, ended: iso(), ok: true };
    } catch (error) {
        results[cell] = { started, ended: iso(), ok: false, error: redact(String(error.stack || error)).slice(0, 1500) };
    }
    await writeFile(join(dir, 'run.json'), redact(JSON.stringify({ cell, chrome: await browser.version(), handover: { ...hand, password_file: '(root-only file, not copied)' }, ...results[cell] }, null, 2)));
    console.log(JSON.stringify({ cell, ...results[cell] }));
}
await browser.close();
