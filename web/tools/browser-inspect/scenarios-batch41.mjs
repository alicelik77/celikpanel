// Batch 41 (2026-11-19, alpha.139): the setup review's zone-wide rule said in
// its own words. When another DNS zone hosted on this server lies inside the
// main domain's zone or around it (a customer's domain under the main domain),
// the review and the peer join no longer say "a record clashes" or "change the
// second server's names".
//
//   y41overlapplan  the plan review with that blocker
//   y41overlapwait  the main server's run waiting at the peer join for it
//
// Every answer comes from the mock on 127.0.0.1 through overrides; nothing is
// contacted or started. Example names only.
// Kırk birinci grup: kurulumun bolge geneli kurali kendi sozleriyle.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const HOST = 'panel.example.com';
const BLOCKER = 'server_setup_infrastructure_dns_zone_overlap:the infrastructure zone overlaps another managed DNS zone; review its delegation first';
const STEPS = [
    { id: 'service-nginx', kind: 'service', target: 'nginx' },
    { id: 'service-certbot', kind: 'service', target: 'certbot' },
    { id: 'firewall', kind: 'firewall', target: 'nftables' },
    { id: 'panel-certificate', kind: 'panel_certificate', target: HOST },
    { id: 'verify', kind: 'verify', target: '' },
];
const PLAN = {
    id: 'a'.repeat(32), version: 1, revision: 4, purpose: 'web', steps: STEPS, blockers: [BLOCKER], can_start: false,
    tcp_ports: [22, 80, 443, 2083], udp_ports: [], preserve_ssh: true, persist_firewall: true, contact_email: 'owner@example.com', server_ip: '203.0.113.10',
};
const PRIMARY = { dns_mode: 'local', dns_role: 'primary', dns_engine: 'pdns', local_nameserver: 'ns1.example.com', local_ip: '203.0.113.10', peer_nameserver: 'ns2.example.com', peer_ip: '198.51.100.20', panel_domain: HOST, mail_hostname: '', dns_hosting_management: 'panel' };
const WAIT = {
    request_id: 'adopt', statuses: [], extra: {
        status: 'waiting', phase: 'peer_join',
        steps: [
            { id: '01-dns', kind: 'dns', target: 'local', status: 'succeeded' },
            { id: '02-peer_join', kind: 'peer_join', target: 'second-server', status: 'running' },
            { id: '03-infrastructure_dns', kind: 'infrastructure_dns', target: 'example.com', status: 'pending' },
        ],
        error: { code: 'server_setup_peer_zone_overlap', message: "The main domain's zone example.com cannot take the names of the server that joined with the code now." },
        context: PRIMARY,
    },
};

const facts = (page) => page.evaluate(() => {
    const text = (document.querySelector('main')?.innerText || '');
    return {
        lang: document.documentElement.lang,
        rawTokens: text.split('\n').filter((line) => /setup\.(problem|v5)\.|server_setup_|\{[a-z]+\}/.test(line)).map((line) => line.slice(0, 160)),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch41(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, into, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state) => {
        await shot(page, `y41-${state}`, await facts(page));
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y41-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const clean = () => ctl({ clearAll: true, b16: null, execution: null, setupStatus: 'ready' });

    scenarios.y41overlapplan = async () => {
        await reset(); await clean();
        await ctl({ setupStatus: 'draft' });
        const page = await newPage();
        await page.goto(`${base}/setup`, { waitUntil: 'networkidle0' });
        // The wizard reopens at its summary from its own session checkpoint
        // (the server-saved draft, as after a reload on that step).
        const revision = await page.evaluate(async () => {
            const snapshot = await (await fetch('/api/v1/setup')).json();
            const user = await (await fetch('/api/v1/auth/me')).json();
            sessionStorage.setItem(`celikpanel.setup.editor.${user.username || 'admin'}`, JSON.stringify({ version: 1, revision: snapshot.revision, step: 'review', draft: snapshot.draft }));
            return snapshot.revision;
        });
        await ctl({ override: { '/api/v1/setup/plan': { method: 'POST', status: 200, body: { ...PLAN, revision } } } });
        await page.reload({ waitUntil: 'networkidle0' });
        await quiet(page); await pause(1500);
        await waitFor(page, () => /overlap|içinde ya da çevresinde|inside the main domain/.test(document.querySelector('main')?.innerText || ''), 5000).catch(() => {});
        const target = await page.evaluate(() => {
            const node = Array.from(document.querySelectorAll('main *')).find((item) => item.children.length === 0 && /inside the main domain|içinde ya da çevresinde/.test(item.textContent || ''));
            node?.scrollIntoView({ block: 'center' });
            return !!node;
        });
        await pause(300);
        await snap(page, target ? 'overlap-plan' : 'overlap-plan-missing');
        await closePage(page);
        await clean();
    };

    scenarios.y41overlapwait = async () => {
        await reset(); await clean();
        await ctl({ setupStatus: 'waiting', execution: WAIT });
        const marker = { 'celikpanel.setup.start.admin': JSON.stringify({ request_id: 'd'.repeat(32), plan_id: 'a'.repeat(32), panel_domain: HOST }) };
        const page = await newPage(marker);
        await page.goto(`${base}/setup`, { waitUntil: 'networkidle0' });
        await page.waitForSelector('#setup-progress-title', { timeout: 20000 }).catch(() => {});
        await quiet(page); await pause(900);
        await into(page, '#setup-progress-title').catch(() => {});
        await page.evaluate(() => {
            const node = Array.from(document.querySelectorAll('main *')).find((item) => item.children.length === 0 && /lies inside it or around it|onun içinde ya da çevresinde/.test(item.textContent || ''));
            node?.scrollIntoView({ block: 'center' });
        });
        await pause(300);
        await snap(page, 'overlap-wait');
        await closePage(page);
        await clean();
    };
}
