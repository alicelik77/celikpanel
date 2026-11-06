import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { confirmActionURL, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// The product's own confirmation (owner report, 6 Nov 2026), mounted: the
// question, Cancel and the action's button; one question at a time; a
// confirmed action runs once, with the button busy and every other exit closed
// until it ends; Cancel and Escape answer "no".
// Ürünün kendi onay penceresi, bağlı hâlde.

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  export const HelpCircle = () => null;
  export const useI18n = () => ({ t: (key) => key, locale: 'en' });
  export const useNavigate = () => () => {};
`);
const shared = sharedLayer(stub);
const { ConfirmHost } = await import(shared.compile('components/ConfirmHost.tsx'));
const { confirmAction } = await import(confirmActionURL);

let keydown = [];
globalThis.document = {
  activeElement: null,
  getElementById: () => null,
  addEventListener: (type, listener) => { if (type === 'keydown') keydown.push(listener); },
  removeEventListener: (type, listener) => { keydown = keydown.filter((item) => item !== listener); },
};
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));

async function mounted(run) {
  let tree;
  await act(async () => { tree = Renderer.create(React.createElement(ConfirmHost)); });
  try {
    await run(tree);
  } finally {
    await act(async () => tree.unmount());
  }
}
const button = (tree, id) => tree.root.findAll((node) => node.type === 'button' && node.props.id === id)[0];
const press = async (node) => { await act(async () => { node.props.onClick({}); await settled(); }); };

test('a confirmed action runs once, busy, and nothing else can close the question meanwhile', async () => {
  await mounted(async (tree) => {
    let finish;
    let runs = 0;
    let answer;
    await act(async () => {
      answer = confirmAction({ title: 'Delete example.com?', message: 'Its files go.', confirmLabel: 'Delete', tone: 'danger', action: () => { runs++; return new Promise((resolve) => { finish = resolve; }); } });
      await settled();
    });
    const text = JSON.stringify(tree.toJSON());
    assert.ok(text.includes('Delete example.com?') && text.includes('Its files go.') && text.includes('common.cancel'), 'the question is not shown');
    // A second question while one is open is "no" at once.
    assert.equal(await confirmAction({ message: 'another' }), false);
    const confirm = button(tree, 'confirm-action-confirm');
    await press(confirm);
    await press(confirm);
    assert.equal(runs, 1, 'the action ran twice');
    assert.equal(button(tree, 'confirm-action-confirm').props.disabled, true, 'the button is not busy');
    assert.equal(button(tree, 'confirm-action-cancel').props.disabled, true, 'Cancel is open while the action runs');
    for (const listener of keydown) listener({ key: 'Escape', preventDefault() {} });
    await act(async () => { finish(); await settled(); });
    assert.equal(await answer, true);
    assert.equal(tree.toJSON(), null, 'the question stayed after the action ended');
  });
});

test('Cancel and Escape answer no, and nothing runs', async () => {
  await mounted(async (tree) => {
    let runs = 0;
    let answer;
    await act(async () => { answer = confirmAction({ message: 'Remove it?', action: async () => { runs++; } }); await settled(); });
    await press(button(tree, 'confirm-action-cancel'));
    assert.equal(await answer, false);
    await act(async () => { answer = confirmAction({ message: 'Remove it?', action: async () => { runs++; } }); await settled(); });
    await act(async () => { for (const listener of keydown) listener({ key: 'Escape', preventDefault() {} }); await settled(); });
    assert.equal(await answer, false);
    assert.equal(runs, 0);
  });
});
