// alpha.127, monitoring stages 2 and 3: the alerts with their one settings
// block, the services and the website checks on the Monitoring page, and the
// Dashboard's line for new alerts. Each list is mounted with the real shared
// reader and the real buttons; what is sent is counted.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';
import { enMonitoringScreens } from '../src/i18n/screens/monitoring/en.ts';
import { trMonitoringScreens } from '../src/i18n/screens/monitoring/tr.ts';
import { enMonitoringAlerts } from '../src/i18n/monitoringAlerts/en.ts';
import { trMonitoringAlerts } from '../src/i18n/monitoringAlerts/tr.ts';

const words = {
  en: { ...enShell, ...enMonitoringScreens, ...enMonitoringAlerts },
  tr: { ...trShell, ...trMonitoringScreens, ...trMonitoringAlerts },
};
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.a127Say = (key, vars) => (words.en[key] !== undefined ? fill(words.en[key], vars) : key);
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, Bell = () => null, CheckCircle2 = () => null, Globe = () => null, Server = () => null, XCircle = () => null;
  export const Loader2 = () => null, RotateCw = () => null, X = () => null, Info = () => null;
  export const useI18n = () => ({ t: globalThis.a127Say, locale: 'en' });
  export const Link = ({ to, children, ...rest }) => React.createElement('a', { href: to, ...rest }, children);
  export const useNavigate = () => () => true;
  export const monitoringAlertsTranslation = () => null;
  export const ALERTS_URL = '/api/v1/metrics/alerts';
  export const EmailVerificationCard = () => React.createElement('aside', null, 'email-verification-card');
  export const componentName = ({ id, name }) => name || id;
  export const isProblemKind = (kind) => ['disk_full', 'site_down', 'service_stopped', 'service_restarting', 'site_certificate'].includes(kind);
  export const problemVars = (p) => ({ name: p.name || p.subject || '', domain: p.subject || '', days: p.value, value: p.value });
  export function postJSON(url, body, method = 'POST') {
    return fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  }
`);
const shared = sharedLayer(stub);
// ProblemsNow (the Dashboard's new-alerts line) with its real words and the
// pieces it draws beside its list stubbed.
const problemsStub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, Bell = () => null, CheckCircle2 = () => null, XCircle = () => null, Loader2 = () => null;
  export const useI18n = () => ({ t: globalThis.a127Say, locale: 'en' });
  export const Link = ({ to, children, ...rest }) => React.createElement('a', { href: to, ...rest }, children);
  export const componentName = ({ id, name }) => name || id;
  export const ServiceActionNotice = () => null, isServiceActionOutcome = () => false, showToast = () => {};
  export const useNavigate = () => () => true, X = () => null, Info = () => null, RotateCw = () => null;
`);
const detail = await import(shared.compile('components/MonitoringDetail.tsx'));
const { AlertsSection, ServicesSection, SitesSection, decodeAlerts, spikeMarks, formatBytes } = detail;
const { AlertsNotice } = await import(sharedLayer(problemsStub).compile('components/ProblemsNow.tsx'));

const originalFetch = globalThis.fetch;
let tree, calls;
function serve(table, changes = {}) {
  calls = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url, body: options.body });
    if (method !== 'GET') {
      const change = changes[url];
      return Promise.resolve().then(() => (change ? change(options) : Response.json({ ok: true })));
    }
    const answer = table[url.split('?')[0]];
    if (!answer) return Promise.resolve(Response.json({ error: 'no such route' }, { status: 404 }));
    return Promise.resolve().then(() => answer());
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(Component) {
  await act(async () => {
    tree = Renderer.create(React.createElement(Component));
    await settled();
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const flat = (node) => (node === null ? '' : typeof node === 'string' ? node : Array.isArray(node) ? node.map(flat).join('') : (node.children || []).map(flat).join(''));
const text = () => flat(tree.toJSON());
const instanceText = (node) => (typeof node === 'string' ? node : (node.children || []).map(instanceText).join(''));
const buttonsWith = (label) => tree.root.findAll((node) => node.type === 'button' && instanceText(node).includes(label));
const press = async (node) => {
  await act(async () => {
    await node.props.onClick({});
    await settled();
    await settled();
  });
};
const reads = (path) => calls.filter((call) => call.method === 'GET' && call.url === path).length;

const settings = { delay_minutes: 10, email: true, site_checks: true };
const alertsAnswer = (over = {}) => ({
  settings, delay_choices: [5, 10, 15, 30, 60],
  email: { confirmed: 0, own_set: true, own_verified: false, mail: { available: true } },
  alerts: [
    { id: 2, key: 'site:a.example', kind: 'site_down', severity: 'critical', subject: 'a.example', value: 0, state: 'open', opened_at: '2026-11-06T10:00:00Z', mail_state: 'not_confirmed', seen: false },
    { id: 1, key: 'disk:/', kind: 'disk_full', severity: 'warning', subject: '/', value: 88, state: 'resolved', opened_at: '2026-11-05T10:00:00Z', resolved_at: '2026-11-05T11:00:00Z', mail_state: 'failed', mail_detail: 'queue file write error', seen: true },
    { id: 9, key: 'x', kind: 'not_a_kind', severity: 'warning', value: 0, state: 'open', opened_at: '', mail_state: 'none', seen: false },
  ],
  ...over,
});

test('Alerts: each says what lasted, when, and what happened to its email; the panel-down limit is said', async () => {
  serve({ '/api/v1/metrics/alerts': () => Response.json(alertsAnswer()) });
  try {
    await mount(AlertsSection);
    const body = text();
    assert.ok(body.includes('a.example does not answer'), body);
    assert.ok(body.includes('The disk is 88 full'), body);
    assert.ok(body.includes('Not emailed: no administrator has confirmed an email address yet'), body);
    assert.ok(body.includes('the mail system refused it (queue file write error)'), body);
    assert.ok(body.includes('1 new'), 'the unseen count is not shown');
    assert.ok(body.includes('outside uptime checker'), 'the panel-down limit is not said');
    assert.ok(!body.includes('not_a_kind'), 'an alert of an unknown kind was drawn without words');
    // An unconfirmed own address offers its confirmation in place.
    assert.ok(body.includes('Your address is not confirmed yet') && tree.root.findAll((node) => node.type === 'a' && node.props.href === '/account#email').length === 1, body);
  } finally {
    await cleanup();
  }
});

test('Alerts: "Mark all as seen" sends one request and the list reads again; nothing else is sent', async () => {
  let seen = false;
  serve({ '/api/v1/metrics/alerts': () => Response.json(alertsAnswer(seen ? { alerts: alertsAnswer().alerts.map((a) => ({ ...a, seen: true })) } : {})) },
    { '/api/v1/metrics/alerts/seen': () => { seen = true; return Response.json({ seen: true }); } });
  try {
    await mount(AlertsSection);
    const [mark] = buttonsWith('Mark all as seen');
    assert.ok(mark, 'no Mark all as seen');
    await press(mark);
    const posts = calls.filter((call) => call.method !== 'GET');
    assert.equal(posts.length, 1);
    assert.equal(posts[0].url, '/api/v1/metrics/alerts/seen');
    assert.equal(reads('/api/v1/metrics/alerts'), 2, 'the list did not read again');
    assert.ok(!text().includes('1 new'));
  } finally {
    await cleanup();
  }
});

test('Alert settings: Save is off until something changes, then sends the three settings once', async () => {
  serve({ '/api/v1/metrics/alerts': () => Response.json(alertsAnswer()) },
    { '/api/v1/metrics/alerts/settings': () => Response.json({ delay_minutes: 30, email: true, site_checks: true }) });
  try {
    await mount(AlertsSection);
    const save = () => tree.root.find((node) => node.type === 'button' && node.props['data-alert-save'] !== undefined);
    assert.equal(save().props.disabled, true, 'Save is on with nothing changed');
    const select = tree.root.find((node) => node.type === 'select' && node.props['data-alert-delay'] !== undefined);
    await act(async () => { select.props.onChange({ target: { value: '30' } }); });
    assert.equal(save().props.disabled, false);
    await press(save());
    const puts = calls.filter((call) => call.method === 'PUT');
    assert.equal(puts.length, 1);
    assert.equal(puts[0].url, '/api/v1/metrics/alerts/settings');
    assert.deepEqual(JSON.parse(puts[0].body), { delay_minutes: 30, email: true, site_checks: true });
    assert.ok(text().includes('Saved.'));
  } finally {
    await cleanup();
  }
});

test('Alert settings: no mail system is said plainly; a missing own address links to My account', async () => {
  serve({ '/api/v1/metrics/alerts': () => Response.json(alertsAnswer({ alerts: [], email: { confirmed: 0, own_set: false, own_verified: false, mail: { available: false, reason: 'no_mail_server' } } })) });
  try {
    await mount(AlertsSection);
    const body = text();
    assert.ok(body.includes('No alerts in the last 30 days'), body);
    assert.ok(body.includes('no mail system the panel can hand mail to'), body);
    assert.ok(tree.root.findAll((node) => node.type === 'a' && node.props.href === '/account#email').length === 1);
  } finally {
    await cleanup();
  }
  for (const answer of [() => Response.json({ error: 'x' }, { status: 500 }), () => Response.json('not the contract')]) {
    serve({ '/api/v1/metrics/alerts': answer });
    try {
      await mount(AlertsSection);
      assert.ok(!text().includes('No alerts in the last 30 days'), 'an unreadable list said "no alerts"');
    } finally {
      await cleanup();
    }
  }
});

test('Services: running since, restarts, CPU and memory; an unreadable reading never says "not running"', async () => {
  const services = [
    { id: 'nginx', name: 'Nginx', unit: 'nginx', active_state: 'active', sub_state: 'running', active_since: '2026-11-06T08:00:00Z', restarts_last_hour: 0, memory_bytes: 64 * 1024 * 1024, cpu: 1.5 },
    { id: 'bind', name: 'BIND', unit: 'named', active_state: 'failed', sub_state: 'failed', restarts_last_hour: 4 },
  ];
  serve({ '/api/v1/metrics/services': () => Response.json({ state: 'known', services }) });
  try {
    await mount(ServicesSection);
    const body = text();
    assert.ok(body.includes('running since'), body);
    assert.ok(body.includes('memory 64 MB') && body.includes('CPU 1.5%'), body);
    assert.ok(body.includes('not running (failed, failed)') && body.includes('4 restarts in the last hour'), body);
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/metrics/services': () => Response.json({ state: 'unreadable', services: [{ id: 'nginx', name: 'Nginx', unit: 'nginx', restarts_last_hour: 0 }] }) });
  try {
    await mount(ServicesSection);
    const body = text();
    assert.ok(body.includes('could not be read') && !body.includes('not running'), body);
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/metrics/services': () => Response.json({ state: 'not_scanned', services: [] }) });
  try {
    await mount(ServicesSection);
    assert.ok(text().includes('Open Components'));
  } finally {
    await cleanup();
  }
});

test('Websites: down since when and why; certificate days; checks off is said', async () => {
  serve({ '/api/v1/metrics/sites': () => Response.json({ enabled: true, sites: [
    { domain: 'down.example', checked_at: '2026-11-06T10:05:00Z', state: 'down', scheme: 'https', reason: 'refused', down_since: '2026-11-06T10:00:00Z' },
    { domain: 'up.example', checked_at: '2026-11-06T10:05:00Z', state: 'up', scheme: 'https', http_status: 200, response_ms: 41, cert_days: 60, cert_valid: true },
    { domain: 'plain.example', checked_at: '2026-11-06T10:05:00Z', state: 'up', scheme: 'http', http_status: 301, response_ms: 3 },
  ] }) });
  try {
    await mount(SitesSection);
    const body = text();
    assert.ok(body.includes('the web server refused the connection'), body);
    assert.ok(body.includes('answers (200, 41 ms)') && body.includes('certificate: 60 days left'), body);
    assert.ok(body.includes('no certificate (HTTP)'), body);
    assert.ok(body.includes('outside uptime checker'), 'the limit of a check from the server itself is not said');
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/metrics/sites': () => Response.json({ enabled: false, sites: [] }) });
  try {
    await mount(SitesSection);
    assert.ok(text().includes('Website checks are off'));
  } finally {
    await cleanup();
  }
});

test('Dashboard: one line only while open alerts are unseen', async () => {
  serve({ '/api/v1/metrics/alerts': () => Response.json(alertsAnswer()) });
  try {
    await mount(AlertsNotice);
    assert.ok(text().includes('1 new alerts on Monitoring') && tree.root.findAll((node) => node.type === 'a' && node.props.href === '/monitoring').length === 1, text());
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/metrics/alerts': () => Response.json(alertsAnswer({ alerts: [] })) });
  try {
    await mount(AlertsNotice);
    assert.equal(tree.toJSON(), null);
  } finally {
    await cleanup();
  }
});

test('Helpers: decoding keeps only known kinds; spike marks fall on the chart; sizes read plainly', () => {
  assert.equal(decodeAlerts(alertsAnswer()).alerts.length, 2);
  assert.throws(() => decodeAlerts({ alerts: [] }));
  const snaps = [{ ts: '2026-11-06T10:30:00Z', reason: 'cpu', value: 95, processes: [] }, { ts: '2026-11-06T09:00:00Z', reason: 'cpu', value: 91, processes: [] }];
  assert.deepEqual(spikeMarks(snaps, 'cpu', '2026-11-06T10:00:00Z', '2026-11-06T11:00:00Z'), [0.5]);
  assert.deepEqual(spikeMarks(snaps, 'memory', '2026-11-06T10:00:00Z', '2026-11-06T11:00:00Z'), []);
  assert.equal(formatBytes(1536), '1.5 KB');
  assert.equal(formatBytes(64 * 1024 * 1024), '64 MB');
});

test('Every new sentence exists in English and Turkish', () => {
  const source = readFileSync(new URL('../src/components/MonitoringDetail.tsx', import.meta.url), 'utf8');
  const keys = new Set([...source.matchAll(/'((?:malert|msvc|msite|mmach)\.[A-Za-z_.]+)'/g)].map((m) => m[1]));
  for (const state of ['none', 'pending', 'sent', 'suppressed', 'not_confirmed', 'not_available', 'failed']) keys.add(`malert.mail.${state}`);
  for (const reason of ['timeout', 'refused', 'tls', 'closed', 'http_5xx', 'error']) keys.add(`msite.reason.${reason}`);
  for (const kind of ['site_down', 'site_certificate', 'service_restarting']) {
    for (const part of ['title', 'body', 'action']) keys.add(`pnow.${kind}.${part}`);
  }
  keys.add('pnow.source.sites');
  keys.add('pnow.alertsNew');
  assert.ok(keys.size > 40, String(keys.size));
  for (const key of keys) {
    assert.ok(words.en[key], `English ${key}`);
    assert.ok(words.tr[key], `Turkish ${key}`);
  }
});

test('The page copy is route-scoped: it follows the locale and leaves every other key to the catalogue', async () => {
  const none = (specifier) => { throw new Error('unexpected import ' + specifier); };
  const enURL = compileSource('i18n/monitoringAlerts/en.ts', none);
  const trURL = compileSource('i18n/monitoringAlerts/tr.ts', none);
  const { monitoringAlertsTranslation } = await import(compileSource('i18n/monitoringAlerts.ts', (specifier) => (specifier.endsWith('/en') ? enURL : trURL)));
  assert.equal(monitoringAlertsTranslation('tr', 'malert.newCount', { n: 2 }), '2 yeni');
  assert.equal(monitoringAlertsTranslation('en', 'malert.minutes', { n: 10 }), '10 minutes');
  assert.equal(monitoringAlertsTranslation('en', 'pnow.title'), null);
  assert.equal(monitoringAlertsTranslation('de', 'malert.title'), null);
  // Not a screen part: the critical boot path does not load it.
  const index = readFileSync(new URL('../src/i18n/index.tsx', import.meta.url), 'utf8');
  assert.ok(!index.includes('monitoringAlerts'));
});
