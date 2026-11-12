import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { confirmActionURL, dataModule, reactURL, replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';

// The structured confirmation of alpha.132 (drawing X16), mounted in the real
// ConfirmHost: what is lost and what stays, the heavier choice, the typed
// name that turns the destructive button on, and the labels of both buttons.
// Yapılı onay penceresi, gerçek ConfirmHost içinde.

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  export const HelpCircle = () => null;
  export const CheckCircle2 = () => null;
  export const Clock = () => null;
  export const Loader2 = () => null;
  export const XCircle = () => null;
  export const useI18n = () => ({ t: (key) => key, locale: 'en' });
  export const useNavigate = () => () => {};
  export const captureToasts = () => () => {};
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const progressURL = shared.compile('components/OperationProgress.tsx');
let hostURL = replaceLazyImport(shared.compile('components/ConfirmHost.tsx'), './OperationProgress', progressURL);
hostURL = replaceLazyImport(hostURL, './kit/save/ConfirmDetails', shared.resolve('/ConfirmDetails'));
const { ConfirmHost } = await import(hostURL);
const { confirmAction } = await import(confirmActionURL);
const { typedNameMatches } = await import(shared.resolve('/ConfirmDetails'));

globalThis.window = { setInterval, clearInterval, setTimeout, clearTimeout };
globalThis.document = { activeElement: null, getElementById: () => null, addEventListener() {}, removeEventListener() {} };
const settled = async () => { for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 0)); };

async function mounted(run) {
  let tree;
  await act(async () => { tree = Renderer.create(React.createElement(ConfirmHost)); });
  try { await run(tree); } finally { await act(async () => tree.unmount()); }
}
const button = (tree, id) => tree.root.findAll((node) => node.type === 'button' && node.props.id === id)[0];
const input = (tree, data) => tree.root.findAll((node) => node.type === 'input' && node.props[data] !== undefined)[0];
const text = (tree) => JSON.stringify(tree.toJSON());

test('the typed name matches exactly, or regardless of letter case for a domain; spaces around it do not count', () => {
  assert.equal(typedNameMatches(' PHP 8.5 ', { value: 'PHP 8.5', label: '' }), true);
  assert.equal(typedNameMatches('php 8.5', { value: 'PHP 8.5', label: '' }), false);
  assert.equal(typedNameMatches('Example.COM', { value: 'example.com', label: '', ignoreCase: true }), true);
  assert.equal(typedNameMatches('example.co', { value: 'example.com', label: '', ignoreCase: true }), false);
});

test('what is lost and what stays are listed; the heavier choice asks for the name before the button turns on', async () => {
  await mounted(async (tree) => {
    let answered;
    let result;
    await act(async () => {
      result = confirmAction({
        title: 'Remove the app from example.com',
        tone: 'danger',
        confirmLabel: 'Remove the app',
        cancelLabel: 'Keep it',
        lost: ['The service'],
        kept: ['The domain'],
        option: { label: 'Also delete the data', lost: ['The app’s data'], typedName: { value: 'example.com', label: 'Type example.com', ignoreCase: true } },
        action: async (_report, answer) => { answered = answer; },
      });
      await settled();
    });
    const lost = () => tree.root.findAll((node) => node.props['data-confirm-lost'] !== undefined).length;
    assert.equal(lost(), 1);
    assert.equal(tree.root.findAll((node) => node.props['data-confirm-kept'] !== undefined).length, 1);
    assert.ok(text(tree).includes('Keep it') && text(tree).includes('Remove the app'));
    // Without the choice no name is asked.
    assert.equal(input(tree, 'data-confirm-typed'), undefined);
    assert.equal(button(tree, 'confirm-action-confirm').props.disabled, false);
    // Ticked: its loss joins the list and the button waits for the name.
    await act(async () => { input(tree, 'data-confirm-option').props.onChange({ target: { checked: true } }); });
    assert.equal(lost(), 2);
    assert.equal(button(tree, 'confirm-action-confirm').props.disabled, true, 'the destructive button is on before the name is typed');
    await act(async () => { input(tree, 'data-confirm-typed').props.onChange({ target: { value: 'example.org' } }); });
    assert.equal(button(tree, 'confirm-action-confirm').props.disabled, true);
    // A press that slips through while it is off does nothing.
    await act(async () => { button(tree, 'confirm-action-confirm').props.onClick({}); await settled(); });
    assert.equal(answered, undefined, 'the action ran without the name');
    await act(async () => { input(tree, 'data-confirm-typed').props.onChange({ target: { value: 'Example.com ' } }); });
    assert.equal(button(tree, 'confirm-action-confirm').props.disabled, false);
    await act(async () => { button(tree, 'confirm-action-confirm').props.onClick({}); await settled(); });
    assert.deepEqual(answered, { option: true, typed: 'Example.com ' });
    assert.equal(await result, true);
  });
});

test('a question with a typed name of its own keeps the button off until it is typed; Cancel answers no', async () => {
  await mounted(async (tree) => {
    let result;
    await act(async () => {
      result = confirmAction({ title: 'Delete it?', tone: 'danger', typedName: { value: 'shop', label: 'Type shop' } });
      await settled();
    });
    assert.equal(button(tree, 'confirm-action-confirm').props.disabled, true);
    assert.ok(text(tree).includes('common.cancel'), 'the default Cancel label is gone');
    await act(async () => { button(tree, 'confirm-action-cancel').props.onClick({}); await settled(); });
    assert.equal(await result, false);
  });
});
