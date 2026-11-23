// AI and automation apps say truthfully why they cannot be installed yet
// (alpha.140; alpha.138 lab: n8n's line mixed languages and named a Node.js
// installation "the server cannot do"). The server sends facts: what the app
// needs (the catalogue's versions) and what this server has; the screen
// builds the whole sentence in the reader's language, with who acts, the next
// step and how it resumes. An app that cannot run on this operating system is
// not offered to customers and resellers; the administrator sees why.
//
// Yapay zekâ uygulamalarının neden henüz kurulamadığı okurun dilinde; sunucu
// olgu gönderir, cümleyi ekran kurar.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { compileSource } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const { runtimeLine, hasRuntimeLine } = await import(compileSource('lib/serviceAppRuntime.ts', (specifier) => {
  throw new Error(`lib/serviceAppRuntime.ts imports ${specifier}`);
}));
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

const catalogues = { en: enScreens, tr: trScreens };
const used = new Set();
const fill = (text, vars) => String(text).replace(/\{(\w+)\}/g, (all, name) => (vars && vars[name] !== undefined ? String(vars[name]) : all));
const say = (locale) => (key, vars) => {
  used.add(key);
  assert.ok(catalogues[locale][key] !== undefined, `${locale} has no ${key}`);
  return fill(catalogues[locale][key], vars);
};

const n8n = { name: 'n8n', runtime: 'node', available: false, reason: 'runtime_missing', needs: ['24'], found: [], found_known: true,
  detail: 'needs Node.js 24; found none' };
const webuiUbuntu = { name: 'PyApp', runtime: 'python', available: false, reason: 'venv_missing', needs: ['3.11', '3.12'],
  found: [{ version: '3.12', source: 'distribution' }], found_known: true, detail: "the distribution's Python venv module is not installed" };
const webuiDebian = { name: 'PyApp', runtime: 'python', available: false, reason: 'os_unsupported', needs: ['3.11', '3.12'],
  found: [{ version: '3.13', source: 'distribution', venv: true }], found_known: true, detail: 'needs Python 3.11 or 3.12; this system has Python 3.13' };

const noUnfilled = (text) => assert.doesNotMatch(text, /\{\w+\}/, text);
const noServerWords = (text, app) => {
  assert.ok(!text.includes(app.detail), `the server's English line is in the sentence: ${text}`);
  assert.doesNotMatch(text, /\bfound none\b|this system has/);
};

test('n8n without Node.js 24: what it needs, what is here, the administrator’s step and its action', () => {
  const en = runtimeLine(n8n, 'admin', say('en'));
  assert.equal(en.text, 'Needs Node.js 24. No Node.js is installed on this server yet. Next step for you: install Node.js 24 under Components → Node.js (choose 24 in its version list). Once it is installed, Install opens here.');
  assert.deepEqual(en.action, { to: '/components?component=node&install=', label: 'Install Node.js 24' });
  const tr = runtimeLine(n8n, 'admin', say('tr'));
  assert.equal(tr.text, 'Node.js 24 gerekir. Bu sunucuda henüz Node.js kurulu değil. Sizin için sonraki adım: Bileşenler → Node.js bölümünden Node.js 24 sürümünü kurun (sürüm listesinde 24 sürümünü seçin). Kurulunca burada Kur düğmesi açılır.');
  assert.equal(tr.action.label, 'Node.js 24 sürümünü kur');
  for (const line of [en, tr]) { noUnfilled(line.text); noServerWords(line.text, n8n); }
  // The customer is told who acts; no action.
  const customer = runtimeLine(n8n, 'account', say('tr'));
  assert.equal(customer.action, null);
  assert.match(customer.text, /Node\.js 24 sürümünü bu sunucuya yönetici kurar/);
  assert.doesNotMatch(customer.text, /Bileşenler/);
});

test('another Node.js major already installed is named, and the action still opens the version list', () => {
  const line = runtimeLine({ ...n8n, found: [{ version: '22.11.0', source: 'panel' }, { version: '20.19.2', source: 'distribution' }] }, 'admin', say('en'));
  assert.match(line.text, /^Needs Node\.js 24\. This server has Node\.js 22\.11\.0, 20\.19\.2\. Next step for you/);
  assert.equal(line.action.to, '/components?component=node&install=');
});

test('an answer from an earlier release still reads as a sentence, without versions it did not send', () => {
  const old = { name: 'n8n', runtime: 'node', available: false, reason: 'runtime_missing', detail: 'needs Node.js 24; found none' };
  for (const locale of ['en', 'tr']) {
    const line = runtimeLine(old, 'admin', say(locale));
    noUnfilled(line.text);
    noServerWords(line.text, old);
    assert.equal(line.action.to, '/components?component=node&install=');
  }
  // Needs from the catalogue, but the Agent did not report what is here: no claim about it.
  const unreported = runtimeLine({ ...n8n, found: undefined, found_known: false }, 'admin', say('en'));
  assert.equal(unreported.text, 'Needs Node.js 24. Next step for you: install Node.js 24 under Components → Node.js (choose 24 in its version list). Once it is installed, Install opens here.');
});

test('Ubuntu 24.04: Python 3.12 without its venv module; the action is Components → Python', () => {
  const en = runtimeLine(webuiUbuntu, 'admin', say('en'));
  assert.equal(en.text, 'Needs Python 3.11 or 3.12. Python 3.12 is here, but its venv module (the operating system’s python3-venv package) is not installed. Next step for you: under Components → Python, install what is missing; it adds python3-venv from the operating system. Once it is installed, Install opens here.');
  assert.deepEqual(en.action, { to: '/components?component=python&install=', label: 'Install Python’s venv module' });
  const tr = runtimeLine(webuiUbuntu, 'admin', say('tr'));
  assert.match(tr.text, /^Python 3\.11 ya da 3\.12 gerekir\. Python 3\.12 var, ama venv modülü/);
  for (const line of [en, tr]) { noUnfilled(line.text); noServerWords(line.text, webuiUbuntu); }
});

test('Debian 13: a Python app (PyApp, alpha.141: Open WebUI was withdrawn) cannot run on Python 3.13; the administrator reads why and what would change it, with no action', () => {
  const en = runtimeLine(webuiDebian, 'admin', say('en'));
  assert.equal(en.action, null);
  assert.equal(en.text, 'Needs Python 3.11 or 3.12. This server’s operating system has Python 3.13. Nothing can be installed for this: the operating system’s own packages do not provide that Python, and CelikPanel does not add outside package sources. It can be used here once PyApp supports Python 3.13, or on a server whose system ships Python 3.11 or 3.12. Until then customers and resellers do not see it on this server and it cannot be offered.');
  const tr = runtimeLine(webuiDebian, 'admin', say('tr'));
  assert.match(tr.text, /PyApp, Python 3\.13 ile çalışmaya başladığında/);
  for (const line of [en, tr]) { noUnfilled(line.text); noServerWords(line.text, webuiDebian); }
  assert.equal(hasRuntimeLine({ available: false, reason: 'probe_failed' }), false);
  assert.equal(hasRuntimeLine({ available: true, reason: 'runtime_missing' }), false);
});

test('every word the lines use exists in both languages, and the old {detail} sentences are gone', () => {
  for (const key of used) {
    assert.ok(enScreens[key], `EN ${key}`);
    assert.ok(trScreens[key], `TR ${key}`);
  }
  for (const key of ['svcapps.unavailable.runtime_missing', 'svcapps.unavailable.os_unsupported', 'svcapps.unavailable.venv_missing']) {
    assert.equal(enScreens[key], undefined, key);
    assert.equal(trScreens[key], undefined, key);
  }
  for (const key of ['svcapps.reason.runtime_missing', 'err.SERVICE_APP.runtime_missing', 'err.SERVICE_APP.os_unsupported']) {
    for (const catalogue of [enScreens, trScreens]) {
      assert.doesNotMatch(catalogue[key], /Node\.js 24|\{detail\}/, key);
    }
  }
  assert.match(enScreens['err.COMPONENT_INSTALLED_BY_VERSION'], /choose a version under Components → \{name\}/);
  assert.match(trScreens['err.COMPONENT_INSTALLED_BY_VERSION'], /Bileşenler → \{name\} bölümünden bir sürüm seçin/);
  assert.ok(enScreens['err.COMPONENT_INSTALLED_BY_VERSION.action'] && trScreens['err.COMPONENT_INSTALLED_BY_VERSION.action']);
});

test('the screens use the line; the offer form is not shown for an app that cannot run here', () => {
  const domain = source('components/DomainServiceApps.tsx');
  assert.doesNotMatch(domain, /detail: app\.detail/);
  assert.match(domain, /why === 'runtime' \? <RuntimeLineNote app=\{app\} \/>/);
  const admin = source('components/AdminServiceApps.tsx');
  assert.doesNotMatch(admin, /detail: app\.detail/);
  assert.match(admin, /const cannotRunHere = !app\.available && app\.reason === 'os_unsupported';/);
  assert.match(admin, /\{\(!app\.offered \|\| app\.offer_stale\) && !cannotRunHere && \(/);
});

test('the Components page opens what the line asks for also when the runtime is installed', () => {
  const list = source('components/ServiceList.tsx');
  assert.match(list, /else if \(target && pkg === '' && target\.is_installed === true && target\.kind === 'runtime' && !target\.requires_missing\?\.length\) \{/);
  assert.match(list, /else if \(isTarballRuntime\(target\)\) setOpenDrawers\(\(prev\) => new Set\(prev\)\.add\(target\.id\)\);\s*else if \(distributionOwn && target\.repair_available\) requestComponentAction\(\{ kind: 'repair', service: target \}\);/);
  assert.match(list, /const distributionOwn = !isTarballRuntime\(target\) && \(target\.instances \?\? \[\]\)\.every\(\(copy\) => !copy\.managed\);/);
});
