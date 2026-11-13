// Batch 34 (2026-11-13): database rights by exact name (alpha.134).
//
//   y34grants   The MariaDB page's block "Database rights by exact name" in
//               its states - nothing to do, being rewritten, waiting for a
//               panel update, blocked on a registered server's account, an
//               engine that is not MariaDB - with narrowed rights that also
//               reached other databases and rights left as they are (Details
//               open), and the one home notification about them.
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is changed or contacted.
// Otuz dördüncü grup: veritabanı hakları tam adla; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

// The cached component scan with MariaDB running (the shape batch 33 serves).
const SCAN = () => ({
    scanned_at: new Date().toISOString().replace(/\.\d{3}Z$/, 'Z'), scan_age_seconds: 0, dns_identity_ready: true,
    mail_hostname: { current: 'server1.example.com', hostname: 'server1.example.com', source: 'saved', current_usable: true, will_set_hostname: false },
    services: [{ id: 'mariadb', name: 'MariaDB', description: '', icon: '', category: 'database', kind: 'service', unit: 'mariadb', version: '11.8.6',
        status: 'active (running)', is_installed: true, versions: [], packages: ['mariadb-server'], repair_available: true,
        config_files: [{ path: '/etc/mysql/mariadb.cnf' }, { path: '/etc/mysql/mariadb.conf.d/50-server.cnf' }] },
        { id: 'postfix', name: 'Postfix', description: '', icon: '', category: 'email', kind: 'service', status: 'active (running)', is_installed: true, versions: [], packages: ['postfix'], repair_available: true }],
    bundles: [],
    profiles: ['core-mail', 'webmail', 'protected-mail'].map((id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] })),
});
const LOCAL = (state, reason, counts) => ({
    key: 'mariadb@this-machine:3306', name: 'local', local: true, state, reason, version: '11.8.6-MariaDB-0+deb13u1',
    checked_at: '2026-11-13 20:42:15', ...counts,
});
const REMOTE = (state, reason, version = '10.11.14-MariaDB') => ({
    key: 'mariadb@db.example.net:3306', name: 'db.example.net', local: false, state, reason, version,
    checked_at: '2026-11-13 20:42:15', narrowed: 0, left: state === 'list_only' ? 1 : 0, waiting: state === 'blocked' ? 4 : 0,
});
const NARROWED = [
    { engine: 'mariadb@this-machine:3306', account: 's5__x_user@localhost', pattern: 's5__x', database: 's5__x', kind: 'ours', reached: ['s51_x'], at: '2026-11-13 20:42:16' },
];
const LEFT = [
    { engine: 'mariadb@this-machine:3306', account: 'raporcu@%', pattern: 'pastane_siparis', database: '', kind: 'not_ours', reason: 'not_provable',
        reached: ['pastane1siparis'], statement: "GRANT SELECT ON `pastane\\_siparis`.* TO 'raporcu'@'%';\nREVOKE ALL PRIVILEGES ON `pastane_siparis`.* FROM 'raporcu'@'%';" },
    { engine: 'mariadb@this-machine:3306', account: 'yedek@localhost', pattern: 'atolye\\_%', database: '', kind: 'not_ours', reason: 'not_provable',
        reached: ['atolye_muhasebe', 'atolye_stok'], statement: "REVOKE ALL PRIVILEGES ON `atolye\\_%`.* FROM 'yedek'@'localhost';" },
    { engine: 'mariadb@this-machine:3306', account: 'kahve_user@localhost', pattern: 'kahve_magaza', database: 'kahve_magaza', kind: 'owner_row', reason: 'owner_row',
        reached: [], statement: "GRANT ALL PRIVILEGES ON `kahve\\_magaza`.* TO 'kahve_user'@'localhost' WITH GRANT OPTION;\nREVOKE ALL PRIVILEGES ON `kahve_magaza`.* FROM 'kahve_user'@'localhost';\nREVOKE GRANT OPTION ON `kahve_magaza`.* FROM 'kahve_user'@'localhost';" },
];
const VIEWS = {
    done: { engines: [LOCAL('done', '', { narrowed: 7, left: 0, waiting: 0 })], narrowed: [], left: [] },
    running: { engines: [LOCAL('running', '', { narrowed: 3, left: 0, waiting: 4 })], narrowed: NARROWED, left: [] },
    waiting: { engines: [LOCAL('waiting', 'update_verifying', { narrowed: 0, left: 0, waiting: 7 })], narrowed: [], left: [] },
    blocked: { engines: [LOCAL('done', '', { narrowed: 7, left: 3, waiting: 0 }), REMOTE('blocked', 'credential_refused')], narrowed: NARROWED, left: LEFT },
    mysql: { engines: [LOCAL('done', '', { narrowed: 7, left: 0, waiting: 0 }), REMOTE('list_only', 'not_mariadb', '8.0.36')], narrowed: [], left: [
        { engine: 'mariadb@db.example.net:3306', account: 'shop_a_user@localhost', pattern: 'shop_a', database: 'shop_a', kind: 'ours', reason: 'not_mariadb', reached: [],
            statement: "GRANT ALL PRIVILEGES ON `shop\\_a`.* TO 'shop_a_user'@'localhost';\nREVOKE ALL PRIVILEGES ON `shop_a`.* FROM 'shop_a_user'@'localhost';" },
    ] },
};
const NOTICE = {
    items: [{
        id: 41, kind: 'database_pattern_rights', audience: 'administrator', needs_action: true, fix_path: '/components/mariadb', created_at: '2026-11-13 20:42:16', source: 'account',
        words: {
            title_en: 'Database rights written as a pattern',
            text_en: 'Some MariaDB rights on this server are written as a name pattern, so an account can also use databases whose names only match it. CelikPanel did not make them, so it leaves them as they are. On Components → MariaDB they are listed with the databases they reach and what you can do.',
            title_tr: 'Kalıp olarak yazılmış veritabanı hakları',
            text_tr: 'Bu sunucudaki bazı MariaDB hakları bir ad kalıbı olarak yazılmış; bu yüzden hesap, adı yalnız kalıba uyan veritabanlarını da kullanabiliyor. Bunları CelikPanel vermediği için olduğu gibi bırakıyor. Bileşenler → MariaDB sayfasında, eriştikleri veritabanları ve yapabilecekleriniz ile birlikte listelenir.',
        },
    }],
};

const facts = (page) => page.evaluate(() => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const root = document.querySelector('[data-grant-forms]') || document.querySelector('main') || document.body;
    const raw = /\bdbgf\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-zA-Z]+\}/;
    return {
        text: root.innerText.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 80),
        rawKeys: root.innerText.split('\n').filter((l) => raw.test(l)).slice(0, 6),
        buttons: Array.from(root.querySelectorAll('button')).filter(visible).map((b) => b.innerText.trim()).slice(0, 8),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch34(scenarios, { base, locale, theme, out, viewport, ctl, reset, drainLog, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state) => {
        await shot(page, `y34-${state}`, { ...(await facts(page)), requests: await drainLog() });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y34-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const clean = () => ctl({ clearAll: true });
    const open = async (view, openDetails = false) => {
        await reset(); await clean();
        await ctl({ override: { '/api/v1/managed-services': { status: 200, body: SCAN() }, '/api/v1/admin/database-grant-forms': { status: 200, body: view } } });
        const page = await newPage({});
        await page.goto(`${base}/services/mariadb`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        await waitFor(page, () => !!document.querySelector('[data-grant-forms]'), 15000).catch(() => {});
        await pause(600);
        if (openDetails) await page.evaluate(() => document.querySelectorAll('[data-grant-forms] details').forEach((d) => { d.open = true; }));
        await page.evaluate(() => document.querySelector('[data-grant-forms]')?.scrollIntoView({ block: 'start' }));
        await pause(300);
        return page;
    };

    scenarios.y34grants = async () => {
        for (const [name, view, details] of [['done', VIEWS.done], ['running', VIEWS.running], ['waiting', VIEWS.waiting], ['blocked-left', VIEWS.blocked, true], ['mysql', VIEWS.mysql, true]]) {
            const page = await open(view, details);
            await snap(page, `grants-${name}`);
            await closePage(page);
        }
        await reset(); await clean();
        await ctl({ override: { '/api/v1/notifications': { status: 200, body: NOTICE } } });
        const page = await newPage({});
        await page.goto(`${base}/`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        await waitFor(page, () => document.body.innerText.includes('Kalıp olarak') || document.body.innerText.includes('written as a pattern'), 15000).catch(() => {});
        await pause(500);
        await page.evaluate(() => {
            const node = Array.from(document.querySelectorAll('main *')).find((el) => /Kalıp olarak yazılmış|written as a pattern/.test(el.textContent || '') && el.children.length === 0);
            node?.scrollIntoView({ block: 'center' });
        });
        await pause(300);
        await shot(page, 'y34-notice', { text: (await page.evaluate(() => document.querySelector('main')?.innerText || '')).split('\n').filter(Boolean).slice(0, 40) });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, 'y34-notice.png'), join(shots, `notice-${locale}-${theme}-${viewport.width}.png`));
        await closePage(page);
        await clean();
    };
}
