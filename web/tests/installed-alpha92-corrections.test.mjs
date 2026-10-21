// Six small corrections from the installed servers on alpha.92 (owner
// screenshots of 2026-10-16). Each test has a negative control, so the rule it
// holds is not satisfied by accident.
//
// alpha.92 kurulu sunucularından altı küçük düzeltme (sahibin 2026-10-16
// ekran görüntüleri). Her testin bir karşı denetimi vardır.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer from 'react-test-renderer';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from './locale-catalogue.mjs';
import { trScreens } from './locale-catalogue.mjs';
import { enServerScreens } from './locale-catalogue.mjs';
import { trServerScreens } from './locale-catalogue.mjs';

const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const dataModule = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const compile = (text) => ts.transpileModule(text, { compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const catalogues = { en: { ...en, ...enScreens, ...enServerScreens }, tr: { ...tr, ...trScreens, ...trServerScreens } };

const serviceList = read('../src/components/ServiceList.tsx');
const dashboard = read('../src/components/Dashboard.tsx');
const operation = read('../src/components/ComponentOperation.tsx');

// 1. Core Mail verified through a verified larger profile: the server sends
// verified=true with Core Mail's own latest attempt "none". The page's closed
// decoder must accept that, and still refuse a verified profile whose own
// attempt is running or failed.
const decoderSource = operation.slice(operation.indexOf('const MAIL_PROFILE_IDS'), operation.indexOf('const MAIL_HOSTNAME_SOURCES'))
  + operation.slice(operation.indexOf('export function decodeManagedMailProfiles('), operation.indexOf('// A component operation that has ended'));
const { decodeManagedMailProfiles } = await import(dataModule(compile(decoderSource)));
const serviceIDs = new Set(['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube', 'rspamd']);
const wire = (coreAttempt) => [
  { id: 'core-mail', name: 'Core Mail', description: 'd', services: ['postfix', 'dovecot'], status: 'complete', available: true, verified: true, latest_attempt_status: coreAttempt },
  { id: 'webmail', name: 'Webmail', description: 'd', services: ['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube'], status: 'complete', available: true, verified: true, latest_attempt_status: 'succeeded' },
  { id: 'protected-mail', name: 'Spam-Protected Mail', description: 'd', services: ['postfix', 'dovecot', 'rspamd'], status: 'complete', available: true, verified: true, latest_attempt_status: 'succeeded' },
];

test('a profile verified by a larger verified profile is accepted by the decoder', () => {
  assert.ok(decodeManagedMailProfiles(wire('none'), serviceIDs), 'inherited proof with no own attempt');
  assert.ok(decodeManagedMailProfiles(wire('succeeded'), serviceIDs), 'own receipt');
  // Negative controls: a running or failed own attempt is never verified.
  assert.equal(decodeManagedMailProfiles(wire('in_progress'), serviceIDs), null);
  const failed = wire('failed');
  failed[0].latest_attempt_error = 'submission verification failed';
  assert.equal(decodeManagedMailProfiles(failed, serviceIDs), null);
});

// 2. "Repair webmail" is drawn like Manage unless the server reports a webmail
// problem. The button is mounted; the helper reads the two webmail codes.
const textLib = await import(dataModule(compile(read('../src/lib/mailProfileText.ts'))));
const lucide = dataModule(`import React from '${reactURL}'; export const RotateCw = () => React.createElement('repair-icon');`);
const { WebmailRepairButton } = await import(dataModule(`import React from '${reactURL}';\n`
  + compile(read('../src/components/WebmailRepairButton.tsx')).replace(/from ['"]lucide-react['"]/, `from '${lucide}'`)));
// alpha.94: every row action shares one outlined recipe (rowButtonClass).
const manageClass = /const rowButtonClass = '([^']+)'/.exec(serviceList)?.[1];

test('Repair webmail is an ordinary action unless webmail reports a problem', () => {
  assert.ok(manageClass, 'the Manage button class is found');
  const mount = (problem) => {
    const tree = Renderer.create(React.createElement(WebmailRepairButton, { problem, disabled: false, title: 'Repair webmail', label: 'Repair webmail', onClick: () => {} }));
    return tree.root.findByType('button').props;
  };
  const calm = mount(false);
  assert.equal(calm['data-tone'], 'neutral');
  assert.equal(calm.className, manageClass, 'drawn like Manage');
  assert.match(serviceList, /onClick=\{\(\) => onManageService\?\.\(s\.id, s\.versions\)\}\s+className=\{`ml-1 \$\{rowButtonClass\}`\}/);
  assert.doesNotMatch(calm.className, /warning/);
  // Negative control: a reported webmail problem keeps the amber.
  const amber = mount(true);
  assert.equal(amber['data-tone'], 'warning');
  assert.match(amber.className, /bg-warning-mark\/20/);

  const profiles = (warningCode) => [{ id: 'core-mail' }, { id: 'webmail', warning_code: warningCode }, { id: 'protected-mail' }];
  assert.equal(textLib.webmailReportsProblem(profiles(undefined)), false);
  assert.equal(textLib.webmailReportsProblem(profiles('repair_to_verify')), false);
  assert.equal(textLib.webmailReportsProblem(profiles('partly_present')), false);
  assert.equal(textLib.webmailReportsProblem(profiles('webmail_unverified')), true);
  assert.equal(textLib.webmailReportsProblem(profiles('roundcube_not_ready')), true);
  assert.equal(textLib.webmailReportsProblem([{ id: 'core-mail', warning_code: 'roundcube_not_ready' }]), false, 'only the Webmail profile counts');
  assert.match(serviceList, /<WebmailRepairButton\s+problem=\{webmailProblem\}/);
  assert.match(serviceList, /const webmailProblem = webmailReportsProblem\(profiles\);/);
});

// 3. The Dashboard says verified mail in plain words. Since alpha.94 the
// Dashboard's Ready bundles card lists an installed bundle as "verified" or
// "not verified", and the Components strip says "installed and verified".
test('the Ready bundles card says verified mail in plain words', () => {
  // alpha.95: the strip's chip and the installed dialogue carry the words;
  // the Dashboard card counts them.
  assert.equal(catalogues.en['services.bundles.installed.verified'], '{name} is installed and verified.');
  assert.equal(catalogues.tr['services.bundles.installed.verified'], '{name} kurulu ve doğrulandı.');
  assert.equal(catalogues.tr['services.bundles.chip.notVerified'], 'doğrulanmadı');
  for (const locale of ['en', 'tr']) {
    for (const key of ['services.bundles.installed.verified', 'services.bundles.installed.unverified', 'services.bundles.chip.verifiedLabel', 'dashboard.bundles.summary', 'dashboard.bundles.unverifiedLine']) {
      assert.doesNotMatch(catalogues[locale][key], /scan|reconcil|tarama|uzlaştır/i);
    }
  }
});

// 4. No mail component installed: the mail bundles are offered, neutrally;
// the old "partially installed" words are gone from the page. Whether a
// profile is partial is the Panel's (mailProfilesView, Go tests with Nginx
// alone and one mail component as controls); a real partial mail bundle keeps
// its own line with Continue and its reason.
test('a mail stack with no mail component reads Not installed, neutrally', () => {
  assert.equal(catalogues.en['services.mailProfiles.status.partial'], undefined, 'the "Partially installed" label is gone');
  assert.equal(catalogues.tr['services.mailProfiles.status.partial'], undefined, 'the "Kısmen kurulu" label is gone');
  // The strip uses amber only for a webmail runtime that is installed but
  // not ready, and never while an attempt is the reason said.
  const strip = read('../src/components/ReadyBundleStrip.tsx');
  assert.match(strip, /bundle\.warning_code === 'roundcube_not_ready' && !bundle\.latest_attempt_status\?\.match\(\/failed\|in_progress\/\) \? 'text-warning' : 'text-fg-muted'/);
  assert.equal((strip.match(/text-warning/g) ?? []).length, 1, 'one amber in the strip');
});

// 5 is held in tests/audit-actions.test.mjs.

// 6. Counts are pluralised; Turkish does not pluralise after a number.
test('item counts are pluralised in both languages', () => {
  assert.equal(catalogues.en['common.itemsTotalOne'], '1 item');
  assert.equal(catalogues.en['common.itemsTotal'], '{n} items');
  assert.equal(catalogues.tr['common.itemsTotalOne'], '1 öğe');
  assert.equal(catalogues.tr['common.itemsTotal'], '{n} öğe');
  for (const file of ['Domains', 'DatabaseManagementV2', 'DomainDNSManager', 'DomainMailManager', 'ServiceList', 'TeamMembersPage', 'UsersPage']) {
    const source = read(`../src/components/${file}.tsx`);
    const many = source.match(/t\('common\.itemsTotal', /g)?.length ?? 0;
    const one = source.match(/t\('common\.itemsTotalOne'\)/g)?.length ?? 0;
    assert.ok(many > 0, `${file} shows a count`);
    assert.equal(one, many, `${file}: every count has its singular`);
  }
  assert.ok(!Object.values(catalogues.en).some((text) => /\b1 items\b|items total/.test(text)), 'no "1 items" or "items total" left');
});
