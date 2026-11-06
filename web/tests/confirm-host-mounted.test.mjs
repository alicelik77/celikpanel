import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { confirmActionURL, dataModule, reactURL, replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';

// The product's own confirmation (owner report, 6 Nov 2026), mounted: the
// question, Cancel and the action's button; one question at a time; a
// confirmed action runs once, with the button busy and every other exit closed
// until it ends; Cancel and Escape answer "no".
// Ürünün kendi onay penceresi, bağlı hâlde.

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
  // The toast capture of components/Toast.tsx: while an operation runs, a
  // toast goes to the window. Outside one it is recorded as shown.
  const captures = [];
  export const captureToasts = (sink) => { captures.push(sink); return () => captures.splice(captures.lastIndexOf(sink), 1); };
  export const showToast = (type, message, extra) => {
    const sink = captures[captures.length - 1];
    if (sink) sink({ type, message, ...extra }); else (globalThis.shownToasts ??= []).push([type, message]);
  };
`);
const shared = sharedLayer(stub);
const progressURL = shared.compile('components/OperationProgress.tsx');
const hostURL = replaceLazyImport(shared.compile('components/ConfirmHost.tsx'), './OperationProgress', progressURL);
const { ConfirmHost } = await import(hostURL);
const { showToast } = await import(stub);
const { confirmAction } = await import(confirmActionURL);

globalThis.window = { setInterval, clearInterval, setTimeout, clearTimeout };
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
    // Two presses before the window has redrawn.
    const click = button(tree, 'confirm-action-confirm').props.onClick;
    await act(async () => { click({}); click({}); await settled(); });
    assert.equal(runs, 1, 'the action ran twice');
    // The question became the locked progress window: no Cancel, no close.
    assert.equal(button(tree, 'confirm-action-cancel'), undefined, 'Cancel is offered while the action runs');
    assert.equal(tree.root.findAll((node) => node.type === 'button' && node.props['data-operation-running'] !== undefined).length, 1, 'the window does not say it is working');
    assert.equal(tree.root.findAll((node) => node.props['data-operation-close'] !== undefined).length, 0, 'the running operation can be closed');
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

// The owner's firm order (6 Nov 2026): after the question the same window
// shows the work, locked, step by step, and ends with "Completed" (OK closes)
// or the reason; what the action would toast is said in the window instead.
test('a confirmed action shows its steps, locked, and ends with Completed or the reason', async () => {
  await mounted(async (tree) => {
    let finish;
    let answer;
    await act(async () => {
      answer = confirmAction({
        title: 'Delete example.com?', progressTitle: 'Deleting example.com', tone: 'danger',
        action: (report) => {
          report.steps([{ id: 'site', label: 'Removing the website', state: 'done' }, { id: 'dns', label: 'Removing DNS', state: 'waiting', since: Date.now(), note: 'The backup confirms.' }]);
          return new Promise((resolve) => { finish = resolve; });
        },
      });
      await settled();
    });
    await press(button(tree, 'confirm-action-confirm'));
    await act(async () => { await settled(); await settled(); });
    const steps = tree.root.findAll((node) => node.props['data-operation-step'] !== undefined);
    assert.deepEqual(steps.map((node) => `${node.props['data-operation-step']}=${node.props['data-step-state']}`), ['site=done', 'dns=waiting']);
    assert.ok(JSON.stringify(tree.toJSON()).includes('Deleting example.com'), 'the progress title is not shown');
    assert.equal(tree.root.findAll((node) => node.props['data-operation-close'] !== undefined).length, 0, 'the running operation can be closed');
    // Escape does nothing while it runs.
    for (const listener of keydown) listener({ key: 'Escape', preventDefault() {} });
    await act(async () => { finish({ state: 'done' }); await settled(); await settled(); });
    assert.ok(JSON.stringify(tree.toJSON()).includes('operation.completed'), 'the end is not said');
    const ok = tree.root.findAll((node) => node.type === 'button' && node.props['data-operation-close'] !== undefined)[0];
    await act(async () => { ok.props.onClick({}); await settled(); });
    assert.equal(await answer, true);
    assert.equal(tree.toJSON(), null);
  });
});

test('an action that only toasts its failure ends in the window with that reason, and no toast', async () => {
  globalThis.shownToasts = [];
  await mounted(async (tree) => {
    let answer;
    await act(async () => {
      answer = confirmAction({ message: 'Remove it?', action: async () => { showToast('error', 'The server refused: in use.', { action: { label: 'Open it', run() {} } }); } });
      await settled();
    });
    await press(button(tree, 'confirm-action-confirm'));
    await act(async () => { await settled(); await settled(); });
    const text = JSON.stringify(tree.toJSON());
    assert.ok(text.includes('operation.failed') && text.includes('The server refused: in use.'), 'the failure is not in the window');
    assert.equal(tree.root.findAll((node) => node.type === 'button' && node.props['data-operation-fix'] !== undefined).length, 1, 'the fix is not offered');
    assert.deepEqual(globalThis.shownToasts, [], 'the failure was also toasted');
    const ok = tree.root.findAll((node) => node.type === 'button' && node.props['data-operation-close'] !== undefined)[0];
    await act(async () => { ok.props.onClick({}); await settled(); });
    assert.equal(await answer, true);
  });
});
