import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// alpha.133: "send through another server" (the Postfix page's relay form) on
// the one way of saving, mounted with the shipped save kit, password field,
// confirmation and request steps. A change sends nothing; Save is on only when
// something changed and the form can be sent; "Test the connection" tries the
// typed values, saves nothing and says which step failed; the password is
// never rendered back; Discard restores; a save whose answer was lost sends
// nothing again by itself.
//
// Aktarma formu tek kaydetme yolunda: değişiklik hiçbir şey göndermez; deneme
// kaydetmez ve hangi adımın geçmediğini söyler; parola geri gösterilmez.

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, ShieldAlert = () => null;
  const words = { locale: 'en', t: (key, vars) => (vars ? key + JSON.stringify(vars) : key) };
  export const useI18n = () => words;
  export const useMailConfigWords = () => words;
  export const useNavigate = () => () => true;
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const { MailRelaySettings } = await import(shared.compile('components/MailRelaySettings.tsx'));

const PASSWORD = 'S3cret-relay-pass';
const answer = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const settle = async () => { for (let i = 0; i < 10; i++) await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); }); };
const textOf = (node) => (node === null || node === undefined || typeof node === 'boolean' ? '' : typeof node === 'string' || typeof node === 'number' ? String(node) : Array.isArray(node) ? node.map(textOf).join(' ') : textOf(node.children));
const view = (settings) => ({ settings, apply: { state: 'applied' }, server: { postfix: true, state: settings.enabled ? 'on' : 'off', sasl_ready: true } });
const saved = { enabled: true, host: 'smtp.example.net', port: 587, username: 'relay-user', has_password: true };

async function mount(server) {
  const state = { settings: { ...saved } };
  const calls = [];
  const original = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    const call = { url: String(url), method: init.method ?? 'GET', body: init.body ? JSON.parse(init.body) : undefined };
    calls.push(call);
    if (call.method === 'GET' && call.url === '/api/v1/mail/relay') return answer(200, view(state.settings));
    return server(call, state);
  };
  let tree;
  await act(async () => { tree = Renderer.create(React.createElement(MailRelaySettings)); });
  await settle();
  const all = (data) => tree.root.findAll((node) => typeof node.type === 'string' && node.props[data] !== undefined);
  const field = (name) => tree.root.find((node) => node.type === 'input' && node.props['data-mail-relay-field'] === name);
  return {
    tree, calls, field,
    password: () => tree.root.find((node) => node.type === 'input' && node.props['aria-label'] === 'mcfg.relay.password'),
    writes: () => calls.filter((call) => call.method !== 'GET'),
    text: () => textOf(tree.toJSON()),
    json: () => JSON.stringify(tree.toJSON()),
    marks: () => all('data-changed-mark').length,
    save: () => all('data-save').filter((node) => node.type === 'button')[0],
    discard: () => all('data-save-discard').filter((node) => node.type === 'button')[0],
    testButton: () => all('data-mail-relay-test-button').filter((node) => node.type === 'button')[0],
    type: async (node, value) => { await act(async () => node.props.onChange({ target: { value } })); await settle(); },
    press: async (node) => { await act(async () => { await node.props.onClick({}); }); await settle(); },
    done: async () => { await act(async () => tree.unmount()); globalThis.fetch = original; delete globalThis.confirm; delete globalThis.operationSteps; delete globalThis.operationEnds; },
  };
}

test('relay: a change sends nothing; Save needs a valid form; the test saves nothing and names the failed step; the password is never shown back', async () => {
  const relay = await mount(({ url, method, body }, state) => {
    if (url === '/api/v1/mail/relay/test') return answer(200, { ok: false, stage: 'auth', verified: false, detail: '535 authentication failed' });
    if (url === '/api/v1/mail/relay' && method === 'POST') {
      state.settings = { enabled: body.enabled, host: body.host, port: body.port, username: body.username, has_password: true };
      return answer(200, { result: { applied: true } });
    }
    return answer(404, { error: 'none' });
  });
  try {
    assert.equal(relay.save(), undefined, 'Save is shown with nothing changed');
    assert.equal(relay.password().props.value, '', 'a saved password was put in the field');
    await relay.type(relay.field('host'), 'smtp.other.net');
    assert.equal(relay.writes().length, 0, 'changing a field sent something');
    assert.equal(relay.marks(), 1, 'the changed field is not marked');
    // A new relay needs its password: Save is off and says why.
    assert.equal(relay.save().props.disabled, true, 'Save is on although the new relay has no password');
    assert.ok(relay.text().includes('mcfg.relay.passwordRequired'), relay.text());
    assert.equal(relay.testButton().props.disabled, true, 'the test is on without a password');
    await relay.type(relay.password(), PASSWORD);
    assert.equal(relay.save().props.disabled, false);
    assert.equal(relay.marks(), 2);
    // The test tries the typed values and saves nothing.
    await relay.press(relay.testButton());
    assert.deepEqual(relay.writes().map((call) => call.url), ['/api/v1/mail/relay/test'], 'the test saved the settings');
    assert.deepEqual(relay.writes()[0].body, { enabled: true, host: 'smtp.other.net', port: 587, username: 'relay-user', password: PASSWORD });
    assert.ok(relay.text().includes('mcfg.relay.fail.auth{"relay":"smtp.other.net:587"}'), `the failed step is not said: ${relay.text()}`);
    assert.ok(relay.text().includes('mcfg.relay.testDetail'), 'the server\'s detail is not kept under the sentence');
    assert.ok(relay.text().includes('mcfg.relay.testUnsaved'), 'the test does not say its values are not saved');
    assert.equal(relay.marks(), 2, 'the test changed the draft');
    // Save asks, sends the form once, and the password is not shown again.
    const asked = [];
    globalThis.confirm = (text, request) => { asked.push(request); return true; };
    await relay.press(relay.save());
    assert.equal(asked.length, 1, 'Save did not ask before changing the server');
    const saves = relay.writes().filter((call) => call.url === '/api/v1/mail/relay');
    assert.equal(saves.length, 1);
    assert.equal(saves[0].body.host, 'smtp.other.net');
    assert.equal(saves[0].body.password, PASSWORD);
    assert.equal(relay.save(), undefined, 'the bar stayed after the save');
    assert.equal(relay.password().props.value, '', 'the password was shown back after the save');
    assert.ok(!relay.json().includes(PASSWORD), 'the password is on screen');
    assert.equal(relay.field('host').props.value, 'smtp.other.net');
  } finally {
    await relay.done();
  }
});

test('relay: Discard restores; a save whose answer was lost keeps the change and sends nothing again', async () => {
  const relay = await mount(({ url, method }) => {
    if (url === '/api/v1/mail/relay' && method === 'POST') return answer(200, { unknown: 'the agent did not answer' });
    return answer(404, { error: 'none' });
  });
  try {
    await relay.type(relay.field('username'), 'other-user');
    await relay.type(relay.password(), PASSWORD);
    assert.equal(relay.marks(), 2);
    await relay.press(relay.discard());
    assert.equal(relay.marks(), 0);
    assert.equal(relay.field('username').props.value, 'relay-user');
    assert.equal(relay.password().props.value, '');
    assert.equal(relay.save(), undefined);
    // Turning the relay off needs nothing else.
    const enabled = relay.tree.root.find((node) => node.type === 'input' && node.props.type === 'checkbox');
    await act(async () => enabled.props.onChange({ target: { checked: false } }));
    await settle();
    assert.equal(relay.writes().length, 0, 'unticking sent something');
    assert.equal(relay.save().props.disabled, false);
    globalThis.confirm = () => true;
    await relay.press(relay.save());
    assert.deepEqual(globalThis.operationEnds.map((end) => end.state), ['unknown']);
    assert.equal(relay.writes().length, 1, 'a lost answer was followed by a second save');
    assert.equal(relay.marks(), 1, 'the change whose answer was lost is no longer shown as a change');
  } finally {
    await relay.done();
  }
});
