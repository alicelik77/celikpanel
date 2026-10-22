// Batch 24 (2026-10-22): the person's own email of the alpha.99 candidate
// (D-037 amendment). My account gains "My email"; its dialog asks for the new
// address and the current password, waits visibly, shows each refusal in its
// own words and says when the result is not known. The panel certificate's
// contact refusal stays on its card with a button to /account#email, and an
// additional user finds "Change email" in the personal menu.
//
//   y24account    /account as an administrator: the three cards, then the
//                 dialog empty, filled, and saving (the change is held open)
//   y24refusals   the dialog after a wrong password, an address another
//                 account uses, an address the panel cannot use
//   y24unknown    the dialog when no answer arrives (the connection is cut
//                 twice: the first send and the one asking again)
//   y24success    the card after a change (the toast and the new address)
//   y24guidance   Settings → Panel address and HTTPS: the contact refusal on
//                 the card, then its button followed to /account#email
//   y24member     an additional user's personal menu, and its email dialog
//
// Every answer comes from the mock on 127.0.0.1 (mock.mjs): /api/v1/auth/me and
// the PUT to /api/v1/auth/email are constants set through the mock's
// overrides, so no new mock file is needed. Nothing else is contacted.
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// Yirmi dorduncu grup: alpha.99 kendi e-postasini degistirme.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const ME = '/api/v1/auth/me';
const EMAIL = '/api/v1/auth/email';
const CERT = '/api/v1/panel/certificate';
const BASE_USER = { account_type: 'account', email: 'owner@example.com', impersonating: false, features: { team_members: false } };
const USERS = {
    admin: { ...BASE_USER, username: 'admin', role: 'admin', effective_role: 'admin' },
    member: { ...BASE_USER, username: 'member1', role: 'additional_user', effective_role: 'additional_user', account_type: 'additional_user', email: 'member1@example.com' },
};
const ENGLISH_PHRASES = ['My email', 'Change email', 'Save address', 'Current password', 'New email address', 'nothing was changed', 'Open My account', 'Your email address was changed', 'is not known yet', 'Cancel', 'Retry'];
const RAW = /\b(account|profile|err|common|setup|panelCert|settings)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-zA-Z]+\}/;

const facts = (page, locale) => page.evaluate((english, rawSource, locale) => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const body = document.body.innerText || '';
    const lines = body.split('\n').map((l) => l.trim()).filter(Boolean);
    const re = new RegExp(rawSource);
    const rect = (node) => { if (!node) return null; const r = node.getBoundingClientRect(); return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) }; };
    const dialog = document.querySelector('[role="dialog"]');
    const targets = Array.from(document.querySelectorAll('[role="dialog"] button, [role="dialog"] input, #email button, [data-personal-menu] button, [data-personal-menu] a, [data-panel-certificate-refusal] button'))
        .filter(visible).map((node) => ({ node, r: node.getBoundingClientRect() }))
        .filter(({ r }) => window.innerWidth <= 639 && r.height < 43.5)
        .map(({ node, r }) => `${(node.innerText || node.getAttribute('name') || node.type || '').trim().slice(0, 40)} ${Math.round(r.height)}px`);
    return {
        lang: document.documentElement.lang,
        location: { path: location.pathname, hash: location.hash },
        rawKeys: lines.filter((line) => re.test(line)).slice(0, 8),
        englishLeaks: locale === 'tr' ? english.filter((item) => body.includes(item)) : [],
        emailCard: rect(document.getElementById('email')),
        emailCardInView: (() => { const r = document.getElementById('email')?.getBoundingClientRect(); return r ? r.top >= -1 && r.top < window.innerHeight : null; })(),
        dialog: dialog ? { rect: rect(dialog), text: dialog.innerText.replace(/\n{2,}/g, '\n').slice(0, 900), busy: dialog.getAttribute('aria-busy'), focus: document.activeElement?.getAttribute('name') ?? document.activeElement?.tagName ?? null } : null,
        refusal: document.querySelector('[data-email-refusal]')?.getAttribute('data-email-refusal') ?? null,
        unknown: !!document.querySelector('[data-email-unknown]'),
        saving: !!document.querySelector('[data-email-saving]'),
        certificateRefusal: document.querySelector('[data-panel-certificate-refusal]')?.innerText.replace(/\s+/g, ' ').trim() ?? null,
        menu: Array.from(document.querySelectorAll('[data-personal-menu] a, [data-personal-menu] button')).filter(visible).map((n) => n.innerText.replace(/\s+/g, ' ').trim()),
        toasts: Array.from(document.querySelectorAll('[role="status"], [role="alert"]')).filter(visible).map((n) => n.innerText.replace(/\s+/g, ' ').trim().slice(0, 120)),
        smallTargets: targets.slice(0, 12),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        dialogOverflowX: dialog ? dialog.scrollWidth > dialog.clientWidth + 1 : false,
    };
}, ENGLISH_PHRASES, RAW.source, locale);

export default function batch24(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `y24-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y24-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const clean = () => ctl({ clearAll: true, b3: null, b12: null, b16: null, b19: null, b20: null, b21: null, b22: null, managedScan: null, componentOperation: null, domains: [], infrastructureZones: [], twoFactor: false, services: [] });
    const as = (who, email) => ctl({ override: { [ME]: { status: 200, body: email ? { ...USERS[who], email } : USERS[who] } } });
    const open = async (path, ready) => {
        const page = await newPage();
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        if (ready) await waitFor(page, ready, 15000).catch(() => {});
        await pause(900);
        return page;
    };
    const cardReady = () => !!document.querySelector('[data-own-email]');
    const openDialog = async (page) => {
        await page.click('#email button');
        await waitFor(page, () => !!document.querySelector('[role="dialog"] input[name="email"]'), 8000).catch(() => {});
        await pause(400);
    };
    const fill = async (page, address, password) => {
        await page.type('[role="dialog"] input[name="email"]', address);
        await page.type('[role="dialog"] input[name="current-password"]', password);
        await pause(200);
    };
    const save = async (page) => {
        await page.click('[role="dialog"] button[type="submit"]');
    };

    scenarios.y24account = async () => {
        await reset(); await clean();
        await as('admin');
        await ctl({ override: { [EMAIL]: { method: 'PUT', hang: true } } });
        const page = await open('/account', cardReady);
        await whole(page, 'account');
        await openDialog(page);
        await snap(page, 'dialog-empty');
        await fill(page, 'new.owner@example.com', 'correct horse');
        await snap(page, 'dialog-filled');
        await save(page);
        await pause(700);
        await snap(page, 'dialog-saving');
        await closePage(page);
        await clean();
    };

    scenarios.y24refusals = async () => {
        for (const [code, status] of [['account_email_wrong_password', 403], ['account_email_taken', 409], ['account_email_invalid', 400]]) {
            await reset(); await clean();
            await as('admin');
            await ctl({ override: { [EMAIL]: { method: 'PUT', status, body: { error: 'server sentence', code } } } });
            const page = await open('/account', cardReady);
            await openDialog(page);
            await fill(page, code === 'account_email_invalid' ? 'owner@localhost' : 'someone@example.com', 'correct horse');
            await save(page);
            await waitFor(page, () => !!document.querySelector('[data-email-refusal]'), 8000).catch(() => {});
            await pause(400);
            await snap(page, `refusal-${code.replace('account_email_', '')}`);
            await closePage(page);
            await clean();
        }
    };

    scenarios.y24unknown = async () => {
        await reset(); await clean();
        await as('admin');
        await ctl({ override: { [EMAIL]: { method: 'PUT', drop: true } } });
        const page = await open('/account', cardReady);
        await openDialog(page);
        await fill(page, 'new.owner@example.com', 'correct horse');
        await save(page);
        await waitFor(page, () => !!document.querySelector('[data-email-unknown]'), 12000).catch(() => {});
        await pause(500);
        await snap(page, 'unknown');
        await closePage(page);
        await clean();
    };

    scenarios.y24success = async () => {
        await reset(); await clean();
        await as('admin');
        await ctl({ override: { [EMAIL]: { method: 'PUT', status: 200, body: { success: true } } } });
        const page = await open('/account', cardReady);
        await openDialog(page);
        await fill(page, 'new.owner@example.com', 'correct horse');
        // The server now holds the new address: the card's next read says so.
        await as('admin', 'new.owner@example.com');
        await save(page);
        await waitFor(page, () => !document.querySelector('[role="dialog"]'), 8000).catch(() => {});
        await pause(600);
        await snap(page, 'success');
        await closePage(page);
        await clean();
    };

    scenarios.y24guidance = async () => {
        await reset(); await clean();
        await as('admin', '');
        await ctl({ override: { [CERT]: { method: 'POST', status: 409, body: { error: 'server sentence', code: 'panel_certificate_contact_email_invalid', action: '/account#email' } } } });
        const page = await open('/settings?section=panel', () => !!document.querySelector('#panel-certificate-domain'));
        await page.click('#panel-certificate-domain', { clickCount: 3 }).catch(() => {});
        await page.type('#panel-certificate-domain', 'panel.example.com').catch(() => {});
        await pause(300);
        await page.evaluate(() => {
            const field = document.querySelector('#panel-certificate-domain');
            const button = field?.parentElement?.querySelector('button');
            button?.click();
        });
        await waitFor(page, () => !!document.querySelector('[data-panel-certificate-refusal]'), 8000).catch(() => {});
        await page.evaluate(() => document.querySelector('[data-panel-certificate-refusal]')?.scrollIntoView({ block: 'center' }));
        await pause(500);
        await snap(page, 'certificate-refusal');
        await page.click('[data-panel-certificate-refusal] button').catch(() => {});
        await waitFor(page, () => location.pathname === '/account' && !!document.querySelector('#email'), 8000).catch(() => {});
        await pause(1200);
        await snap(page, 'certificate-refusal-followed');
        await closePage(page);
        await clean();
    };

    scenarios.y24member = async () => {
        await reset(); await clean();
        await as('member');
        await ctl({ override: { [EMAIL]: { method: 'PUT', hang: true } } });
        const page = await open('/', null);
        await (await page.$$('header button[aria-expanded]')).pop()?.click();
        await pause(600);
        await snap(page, 'member-menu');
        await page.click('[data-personal-menu-item="email"]').catch(() => {});
        await waitFor(page, () => !!document.querySelector('[role="dialog"] input[name="email"]'), 8000).catch(() => {});
        await pause(500);
        await snap(page, 'member-dialog');
        await closePage(page);
        await clean();
    };
}
