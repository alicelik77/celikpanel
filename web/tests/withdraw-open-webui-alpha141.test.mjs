// Open WebUI was withdrawn from the catalogue (owner, 20 November 2026). The
// server lists it for nobody; a domain that already holds it keeps its card,
// which says plainly that the app was withdrawn, keeps running, gets no
// updates from CelikPanel and can be stopped, started or removed there.
//
// Open WebUI katalogdan çıkarıldı; var olan kurulum kartında bunu söyler,
// çalışmaya devam eder ve kaldırılabilir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const repo = (path) => readFileSync(new URL('../../' + path, import.meta.url), 'utf8');

test('the withdrawn line and refusals have words in both languages', () => {
  for (const key of ['svcapps.withdrawnInstalled', 'svcapps.reason.withdrawn', 'err.SERVICE_APP.withdrawn']) {
    assert.ok(enScreens[key], `EN ${key}`);
    assert.ok(trScreens[key], `TR ${key}`);
  }
  assert.match(enScreens['svcapps.withdrawnInstalled'], /withdrawn from CelikPanel’s catalogue\. It keeps running here, but CelikPanel no longer updates it\. You can stop, start or remove it below\./);
  assert.match(trScreens['svcapps.withdrawnInstalled'], /kataloğundan çıkarıldı\. Burada çalışmaya devam eder, ama CelikPanel onu artık güncellemez\./);
  for (const catalogue of [enScreens, trScreens]) {
    assert.match(catalogue['err.SERVICE_APP.withdrawn'], /\{app\}/);
    assert.doesNotMatch(catalogue['svcapps.desc'], /Open WebUI/);
  }
});

test('an installed withdrawn app keeps its card with the line; no update is offered for it', () => {
  const domain = source('components/DomainServiceApps.tsx');
  assert.match(domain, /\{app\?\.withdrawn && <Notice tone="info">\{t\('svcapps\.withdrawnInstalled', \{ name: install\.name \}\)\}<\/Notice>\}/);
  assert.match(domain, /\{retryKind === 'update' && !republishRefused && !app\?\.withdrawn && \(/);
  assert.match(source('lib/serviceApps.ts'), /withdrawn\?: string;/);
});

test('the user guide no longer presents Open WebUI as available', () => {
  for (const locale of ['en', 'tr']) {
    const chapter = repo(`docs/guide/${locale}/15-ai-apps.md`);
    assert.match(chapter, /\{#withdrawn\}/);
    assert.doesNotMatch(chapter, /For Open WebUI|Open WebUI için isterseniz|Open WebUI is several GB|Open WebUI birkaç GB|or Open WebUI \(|ya da Open WebUI \(/);
  }
});
