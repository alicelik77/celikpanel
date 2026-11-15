// Batch 37 (2026-11-15): D-047 stage 1, a licence that does not allow full use
// and the warning before the term ends.
//
//   y37expiry    The administrator's Dashboard 30, 7 and 1 day before the
//                licence term ends, and the license page 7 days before
//   y37limited   The notice on top of every page while the licence does not
//                allow full use: the administrator (ended, none yet, not
//                accepted, could not be checked), the license page, a
//                reseller and a customer (ended)
//
// Every answer comes from the mock on 127.0.0.1; no license service is
// contacted. Otuz yedinci grup: lisans uyarıları ve sınırlı düzey; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const ME = '/api/v1/auth/me';
const BASE_USER = { account_type: 'account', email: 'owner@example.com', impersonating: false, features: { team_members: false } };
const USERS = {
    admin: { ...BASE_USER, username: 'admin', role: 'admin', effective_role: 'admin' },
    reseller: { ...BASE_USER, username: 'bayi1', role: 'reseller', effective_role: 'reseller' },
    customer: { ...BASE_USER, username: 'musteri1', role: 'customer', effective_role: 'customer' },
};
const DAY = 86400;

const facts = (page) => page.evaluate(() => {
    const notice = document.querySelector('[data-license-limited]') || document.querySelector('[data-license-expiry]');
    const text = notice ? notice.innerText : '';
    return {
        notice: text.split('\n').map((l) => l.trim()).filter(Boolean),
        rawKeys: /\blicense\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-z_]+\}/.test(document.body.innerText),
        namesLicense: /licen|lisans/i.test(text),
        noticeTop: notice ? Math.round(notice.getBoundingClientRect().top) : null,
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch37(scenarios, { base, locale, theme, out, viewport, ctl, reset, drainLog, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, extra = {}) => {
        await shot(page, `y37-${state}`, { ...(await facts(page)), ...extra, requests: await drainLog() });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y37-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const open = async (path, role, licence, term, wanted) => {
        await reset();
        await ctl({ clearAll: true, domains: [], license: licence, licenseTerm: term, validity: 3600, override: { [ME]: { status: 200, body: USERS[role] } } });
        const page = await newPage({});
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        if (wanted) await waitFor(page, (selector) => !!document.querySelector(selector), 20000, wanted).catch(() => {});
        await pause(700);
        return page;
    };
    const done = async (page) => {
        await closePage(page);
        await reset();
        await ctl({ clearAll: true, licenseTerm: null });
    };

    scenarios.y37expiry = async () => {
        for (const [name, left] of [['30', 29 * DAY + 3600], ['7', 6 * DAY + 3600], ['1', 20 * 3600]]) {
            const page = await open('/', 'admin', 'active', left, '[data-license-expiry]');
            await snap(page, `expiry-${name}-dashboard`, { left });
            await done(page);
        }
        const page = await open('/settings?section=license', 'admin', 'active', 6 * DAY + 3600, '[data-license-expiry]');
        await snap(page, 'expiry-7-license-page');
        await done(page);
    };

    scenarios.y37limited = async () => {
        for (const [role, licence] of [['admin', 'expired'], ['admin', 'missing'], ['admin', 'invalid'], ['admin', 'unavailable'], ['reseller', 'expired'], ['customer', 'expired']]) {
            const page = await open('/', role, licence, licence === 'expired' ? 0 : null, '[data-license-limited]');
            await snap(page, `limited-${licence}-${role}`, { role });
            await done(page);
        }
        const page = await open('/settings?section=license', 'admin', 'expired', 0, '[data-license-limited]');
        await snap(page, 'limited-expired-license-page');
        await done(page);
    };
}
