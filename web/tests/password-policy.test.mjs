import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, passwordPolicyURL, reactURL } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

// The owner's password rule (6 Nov 2026) in the browser, and the one password
// field. The browser and the panel read the same cases and field windows
// (internal/passwordpolicy/testdata), so they refuse exactly the same
// passwords; a generated password always meets the rule and reads "Very
// strong"; the eye shows and hides; generating shows the password and copy
// copies it, without a browser dialogue.
// Sahibin parola kuralı tarayıcıda ve tek parola alanı.

const policy = await import(passwordPolicyURL);
const testdata = (name) => JSON.parse(readFileSync(new URL(`../../internal/passwordpolicy/testdata/${name}`, import.meta.url), 'utf8'));

test('the browser refuses exactly what the panel refuses (shared cases)', () => {
  const cases = testdata('cases.json');
  assert.ok(cases.length >= 10);
  for (const c of cases) {
    assert.deepEqual(policy.passwordProblems(c.password, { min: c.min, max: c.max }), c.rules, JSON.stringify(c));
  }
});

test('the field windows are the panel\'s', () => {
  assert.deepEqual(JSON.parse(JSON.stringify(policy.passwordFields)), testdata('fields.json'));
  assert.deepEqual(policy.passwordProblems('Ab-def', 'mailbox'), ['too_short']);
  assert.deepEqual(policy.passwordProblems('Ab-defgh', 'mailbox'), []);
});

test('the meter levels follow the rule', () => {
  const field = { min: 6, max: 0 };
  assert.equal(policy.passwordStrength('abcdefgh', field), 'tooWeak');
  assert.equal(policy.passwordStrength('Ab-def', field), 'acceptable');
  assert.equal(policy.passwordStrength('Ab-defghijkl', field), 'acceptable', 'no digit');
  assert.equal(policy.passwordStrength('Ab-defghijk1', field), 'strong');
  assert.equal(policy.passwordStrength('Ab-defghijklmno1', field), 'veryStrong');
  assert.equal(policy.passwordStrength('Ab-def', 'mailbox'), 'tooWeak', 'the field minimum counts');
});

test('a generated password is long, meets every field, uses the safe alphabet and is not repeated', () => {
  const seen = new Set();
  const counts = new Map();
  for (let i = 0; i < 400; i++) {
    const made = policy.generatePassword();
    assert.equal(made.length, 20);
    assert.match(made, /^[A-Za-z0-9][A-Za-z0-9._-]{18}[A-Za-z0-9]$/);
    assert.doesNotMatch(made, /[0O1lI]/, 'no look-alike characters');
    for (const field of Object.keys(policy.passwordFields)) assert.ok(policy.passwordAcceptable(made, field), `${made} for ${field}`);
    assert.equal(policy.passwordStrength(made, 'account'), 'veryStrong');
    assert.ok(!seen.has(made));
    seen.add(made);
    for (const ch of made) counts.set(ch, (counts.get(ch) ?? 0) + 1);
  }
  // 8000 draws over 60 characters: every character turns up (a broken draw
  // that used part of the alphabet would not).
  assert.equal(counts.size, 60);
  const source = readFileSync(new URL('../src/lib/passwordPolicy.ts', import.meta.url), 'utf8');
  assert.match(source, /crypto\.getRandomValues/);
  assert.doesNotMatch(source, /Math\.random/);
});

test('the field\'s words exist in both languages, sentence for sentence with the server', () => {
  const keys = Object.keys(enScreens).filter((key) => key.startsWith('pw.') || key.startsWith('err.PASSWORD_WEAK'));
  for (const key of ['pw.show', 'pw.hide', 'pw.generate', 'pw.copy', 'pw.copied', 'pw.level.tooWeak', 'pw.level.acceptable', 'pw.level.strong', 'pw.level.veryStrong', 'pw.rule', 'pw.missing']) {
    assert.ok(keys.includes(key), key);
  }
  for (const rule of ['invalid_text', 'too_short', 'too_long', 'needs_upper', 'needs_lower', 'needs_special']) {
    assert.ok(enScreens['pw.need.' + rule] && trScreens['pw.need.' + rule], rule);
    assert.ok(enScreens['err.PASSWORD_WEAK.' + rule] && trScreens['err.PASSWORD_WEAK.' + rule], rule);
  }
  for (const key of keys) assert.ok(trScreens[key], `${key} in Turkish`);
  assert.equal(trScreens['pw.show'], 'Şifreyi göster');
  assert.equal(trScreens['pw.generate'], 'Güçlü şifre üret');
  // The server's own sentence for each rule (internal/passwordpolicy/policy.go).
  const goSource = readFileSync(new URL('../../internal/passwordpolicy/policy.go', import.meta.url), 'utf8');
  assert.ok(goSource.includes('"Password must contain an uppercase letter."'));
  assert.ok(enScreens['err.PASSWORD_WEAK.needs_upper'].startsWith('Password must contain an uppercase letter.'));
});

// Mounted: the real component with the real rule.
const stub = dataModule(`
  export const useI18n = () => ({ t: (key, vars) => vars ? key + ':' + Object.values(vars).join(',') : key, locale: 'en' });
  export const inputClass = 'input';
  export const Check = () => null, Copy = () => null, Eye = () => null, EyeOff = () => null, Wand2 = () => null;
`);
const inputURL = compileSource('components/PasswordInput.tsx', (specifier) => (specifier.endsWith('/passwordPolicy') ? passwordPolicyURL : stub));
const { PasswordInput } = await import(inputURL);

function Harness({ field, generate, onValue }) {
  const [value, setValue] = React.useState('');
  return React.createElement(PasswordInput, { field, generate, value, onChange: (v) => { setValue(v); onValue?.(v); } });
}
const text = (node) => JSON.stringify(node.toJSON());
const buttonByLabel = (tree, label) => tree.root.findAll((n) => n.type === 'button' && n.props['aria-label'] === label)[0];

test('the eye shows and hides; a new password gets generate and copy, an existing secret does not', async () => {
  let tree;
  let typed = '';
  await act(async () => { tree = Renderer.create(React.createElement(Harness, { field: 'mailbox', onValue: (v) => { typed = v; } })); });
  const input = () => tree.root.findByType('input');
  assert.equal(input().props.type, 'password');
  assert.equal(input().props.autoComplete, 'new-password');
  assert.ok(text(tree).includes('pw.rule:8'), 'the rule is shown before anything is typed');
  const eye = buttonByLabel(tree, 'pw.show');
  assert.equal(eye.props['aria-pressed'], false);
  await act(async () => eye.props.onClick());
  assert.equal(input().props.type, 'text');
  assert.equal(buttonByLabel(tree, 'pw.hide').props['aria-pressed'], true);
  await act(async () => buttonByLabel(tree, 'pw.hide').props.onClick());
  assert.equal(input().props.type, 'password');

  // Typed and weak: the meter says too weak and names what is missing.
  await act(async () => input().props.onChange({ target: { value: 'abc' } }));
  assert.ok(text(tree).includes('pw.level.tooWeak'));
  assert.ok(text(tree).includes('pw.missing'));

  // Generated: shown, very strong, and copy writes it to the clipboard.
  await act(async () => buttonByLabel(tree, 'pw.generate').props.onClick());
  assert.equal(input().props.type, 'text', 'a generated password is shown');
  assert.ok(policy.passwordAcceptable(typed, 'mailbox'));
  assert.ok(text(tree).includes('pw.level.veryStrong'));
  const written = [];
  const before = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
  Object.defineProperty(globalThis, 'navigator', { value: { clipboard: { writeText: async (v) => { written.push(v); } } }, configurable: true });
  try {
    await act(async () => { await buttonByLabel(tree, 'pw.copy').props.onClick(); });
    assert.deepEqual(written, [typed]);
    assert.ok(buttonByLabel(tree, 'pw.copied'), 'copy says it copied');
    // A clipboard that refuses: the field says so itself, no browser dialogue.
    Object.defineProperty(globalThis, 'navigator', { value: { clipboard: { writeText: async () => { throw new Error('denied'); } } }, configurable: true });
    await act(async () => { await buttonByLabel(tree, 'pw.copied').props.onClick(); });
    assert.ok(text(tree).includes('pw.copyFailed'));
  } finally {
    if (before) Object.defineProperty(globalThis, 'navigator', before); else delete globalThis.navigator;
  }
  await act(async () => tree.unmount());

  // An existing secret: show/hide only, no rule.
  await act(async () => { tree = Renderer.create(React.createElement(Harness, {})); });
  assert.equal(buttonByLabel(tree, 'pw.generate'), undefined);
  assert.equal(buttonByLabel(tree, 'pw.copy'), undefined);
  assert.ok(buttonByLabel(tree, 'pw.show'));
  assert.ok(!text(tree).includes('pw.rule'));
  assert.equal(tree.root.findByType('input').props.autoComplete, 'off');
  await act(async () => tree.unmount());
});
