// Batch 22 (2026-10-21): the installed package versions of alpha.97
// (D-036), photographed on the Components list and on a component's page.
//
//   y22boston      Components on a Boston-shaped server (Ubuntu 24.04 sample
//                  strings): the version column, the short form in mono
//   y22frankfurt   Components on a Frankfurt-shaped server (Debian 13 sample
//                  strings) with SpamAssassin installed ("several versions")
//                  and Postfix's reading kept from an earlier scan
//   y22none        Components with no package version known on either server:
//                  no column, no dash, nothing
//   y22component   the component page, whose header (ServiceShell) says the
//                  installed version once under the name and status line on
//                  every page, generic and specialised: Nginx (a specialised
//                  page), MariaDB (epoch kept), SpamAssassin (two packages,
//                  two versions), BIND (an earlier reading) and phpMyAdmin (a
//                  tool without a version: no line)
//
// Every version string is SAMPLE data, not measured. Every answer comes from
// the mock on 127.0.0.1 (mock.mjs, mock-batch22.mjs, mock-batch20.mjs);
// nothing else is contacted. Screenshots also go to BROWSER_INSPECT_SHOTS
// (default: the run's out dir) as <state>-<lang>-<theme>-<width>.png.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { batch22State } from './mock-batch22.mjs';

const RAW = /\b(services|component)\.[a-zA-Z]+\b|\{[a-z]+\}|package_version/;
const ENGLISH = ['several versions', 'Installed version', 'system package database', 'Its packages report', 'Overview'];

const facts = (page, locale) => page.evaluate((raw, english, locale) => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const main = document.querySelector('main');
    const text = main ? main.innerText : '';
    const re = new RegExp(raw);
    const colour = (cls, prop) => { const probe = document.createElement('span'); probe.className = cls; document.body.appendChild(probe); const value = getComputedStyle(probe)[prop]; probe.remove(); return value; };
    const warning = colour('text-warning', 'color');
    const danger = colour('text-danger', 'color');
    const success = colour('text-success', 'color');
    const rows = Array.from(document.querySelectorAll('[data-row-version]')).filter(visible).map((node) => {
        const style = getComputedStyle(node);
        const row = node.closest('li');
        const box = node.getBoundingClientRect();
        return {
            component: row?.querySelector('a')?.innerText.trim() ?? '',
            shown: Array.from(node.childNodes).filter((child) => child.nodeType === 3 || !child.classList.contains('sr-only')).map((child) => child.textContent).join('').trim(),
            full: node.getAttribute('data-row-version'),
            title: node.getAttribute('title'),
            mono: /mono/i.test(style.fontFamily),
            colour: style.color,
            size: style.fontSize,
            clipped: node.scrollWidth > node.clientWidth + 1,
            insideColumn: !!node.closest('[data-version-column]'),
            right: Math.round(box.right),
        };
    });
    const detail = document.querySelector('[data-package-version]');
    const leaves = (colourValue) => Array.from(document.querySelectorAll('main *')).filter((node) => visible(node) && node.children.length === 0 && getComputedStyle(node).color === colourValue && (node.innerText || '').trim()).map((node) => node.innerText.trim().slice(0, 50));
    const wide = Array.from(document.querySelectorAll('main *')).filter((node) => visible(node) && node.getBoundingClientRect().right > window.innerWidth + 0.5).map((node) => `${node.tagName.toLowerCase()} ${(node.innerText || '').trim().slice(0, 30)}`);
    return {
        lang: document.documentElement.lang,
        versionColumnCells: document.querySelectorAll('[data-version-column]').length,
        versionColumnVisible: Array.from(document.querySelectorAll('[data-version-column]')).filter(visible).length,
        rowVersions: rows,
        dashesInRows: Array.from(document.querySelectorAll('main li')).filter((li) => visible(li) && /(^|\s)[—–-](\s|$)/.test(li.innerText)).map((li) => li.innerText.replace(/\s+/g, ' ').slice(0, 60)),
        detailVersion: detail ? { full: detail.getAttribute('data-package-version'), text: detail.innerText.replace(/\s+/g, ' ').trim(), source: detail.querySelector('[data-package-version-source]')?.getAttribute('data-package-version-source') } : null,
        rawKeys: text.split('\n').filter((line) => re.test(line)).map((line) => line.slice(0, 160)),
        englishLeaks: locale === 'tr' ? english.filter((item) => text.includes(item)) : [],
        amberText: leaves(warning), redText: leaves(danger), greenText: leaves(success),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        mainOverflowX: main ? main.scrollWidth > main.clientWidth + 1 : false,
        pastRightEdge: wide.slice(0, 8),
    };
}, RAW.source, ENGLISH, locale);

export default function batch22(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `y22-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y22-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const clean = () => ctl({ clearAll: true, b19: null, b20: null, b22: null, b12: null, b16: null, managedScan: null, componentOperation: null, domains: [], infrastructureZones: [] });
    const open = async (path) => {
        const page = await newPage({ 'celikpanel.components.view': 'catalog' });
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(700);
        return page;
    };

    const list = (name, server, options) => async () => {
        await reset(); await clean();
        await ctl({ b22: batch22State(server, options) });
        const page = await open('/services');
        await waitFor(page, () => document.querySelectorAll('main li').length >= 6, 15000).catch(() => {});
        await whole(page, name);
        await closePage(page);
        await clean();
    };
    scenarios.y22boston = list('components-boston', 'boston', {});
    scenarios.y22frankfurt = list('components-frankfurt', 'frankfurt', { extra: ['spamassassin'], kept: ['postfix'] });
    scenarios.y22none = async () => {
        await list('components-boston-none', 'boston', { versions: 'none' })();
        await list('components-frankfurt-none', 'frankfurt', { versions: 'none', extra: ['spamassassin'] })();
    };

    const detail = (name, server, id, options) => async () => {
        await reset(); await clean();
        await ctl({ b22: batch22State(server, options) });
        const page = await open(`/services/${id}`);
        await waitFor(page, () => !!document.querySelector('[data-package-version]') || /Overview|Genel bakış/.test(document.querySelector('main')?.innerText || ''), 15000).catch(() => {});
        await pause(600);
        await whole(page, name);
        await closePage(page);
        await clean();
    };
    scenarios.y22component = async () => {
        await detail('component-nginx', 'frankfurt', 'nginx', {})();
        await detail('component-mariadb', 'frankfurt', 'mariadb', {})();
        await detail('component-spamassassin', 'frankfurt', 'spamassassin', { extra: ['spamassassin'] })();
        await detail('component-bind-kept', 'frankfurt', 'bind', { kept: ['bind'] })();
        await detail('component-phpmyadmin-unknown', 'frankfurt', 'phpmyadmin', {})();
    };
}
