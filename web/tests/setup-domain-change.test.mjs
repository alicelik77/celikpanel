import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { enScreens, trScreens } from './locale-catalogue.mjs';

// Owner, 2 November 2026: the main domain was changed from celikpanel.net to
// celikhost.com after the records step; the draft kept celikpanel.net's
// imported records, the server refused it, and the wizard said "check the
// connection".
// Sahip, 2 Kasim: alan adi degisince eski kayitlar taslakta kaldi.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const dataURL = (text) => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const compile = (path) => ts.transpileModule(read(path), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const setup = await import(dataURL(compile('../src/lib/serverSetup.ts')));
const wizard = read('../src/components/ServerSetup.tsx');

const imported = [
  { name: 'celikpanel.net', type: 'A', content: '185.95.0.123', decision: 'import' },
  { name: 'www.celikpanel.net', type: 'CNAME', content: 'celikpanel.net', decision: 'skip' },
];
const draft = { main_domain: 'celikpanel.net', mail_hostname: 'mail.celikpanel.net', imported_records: imported };

test('a changed main domain drops the old domain\'s record answers', () => {
  const next = setup.setupWithMainDomain(draft, 'celikhost.com');
  assert.equal(next.main_domain, 'celikhost.com');
  assert.equal(next.imported_records, undefined);
  assert.equal(next.mail_hostname, 'mail.celikpanel.net', 'the names step re-proposes a proposed name itself');
  assert.equal(draft.imported_records, imported, 'the previous draft is not changed');
});

test('the same domain written differently keeps the answers', () => {
  for (const value of ['celikpanel.net', ' CelikPanel.net ', 'https://www.celikpanel.net/', 'celikpanel.net.']) {
    assert.equal(setup.setupWithMainDomain(draft, value).imported_records, imported, value);
  }
  const first = setup.setupWithMainDomain({ main_domain: '' }, 'example.com');
  assert.equal(first.main_domain, 'example.com');
});

test('the mail2 name offered for the old domain goes with it', () => {
  const next = setup.setupWithMainDomain({ ...draft, mail_hostname: 'mail2.celikpanel.net' }, 'celikhost.com');
  assert.equal(next.mail_hostname, '');
  const typed = setup.setupWithMainDomain({ ...draft, mail_hostname: 'mx.example.org' }, 'celikhost.com');
  assert.equal(typed.mail_hostname, 'mx.example.org', 'a name the owner typed is kept');
});

test('the domain step uses it, and a refused draft says the server\'s reason, not the connection', () => {
  assert.match(wizard, /onChange=\{value => updateDraft\(previous => setupWithMainDomain\(previous, value\)\)\}/);
  assert.match(wizard, /response\.status === 400[\s\S]{0,200}readApiError\(response\)[\s\S]{0,200}setup\.saveRefused/);
  for (const screens of [enScreens, trScreens]) {
    const text = JSON.stringify(screens);
    assert.match(text, /"setup\.saveRefused":"[^"]*\{reason\}/);
  }
});
