// A check that finds nothing is a good state (owner observation 2026-10-16,
// D-033): Settings -> CelikPanel updates showed "There is no new update on this
// channel." with a warning triangle after "Check for updates". It now reads
// "You are on the latest version." with a check mark on a normal surface. A
// found update is neutral information, a readiness read in progress is neutral,
// and only a check that could not be read, a refused readiness, or a start that
// failed keeps the warning mark.
//
// Yeni guncelleme yoksa iyi durumdur: onay isareti, normal yuzey. Uyari isareti
// yalniz okunamayan kontrol, sunucunun hazir olmadigi ve baslatilamayan
// guncelleme icin kalir.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enServerScreens } from '../src/i18n/screens/server/en.ts';
import { trServerScreens } from '../src/i18n/screens/server/tr.ts';

globalThis.document ??= Object.assign(new EventTarget(), { visibilityState: 'visible' });
globalThis.window ??= { setTimeout: () => 0, clearTimeout: () => {} };

const require = createRequire(import.meta.url);
const moduleURL = source => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
function compileURL(relativePath, replacements = []) {
    let compiled = ts.transpileModule(readFileSync(new URL(relativePath, import.meta.url), 'utf8'), {
        compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
    }).outputText;
    for (const [pattern, replacement] of replacements) compiled = compiled.replace(pattern, replacement);
    return moduleURL(compiled);
}
const reactURL = pathToFileURL(require.resolve('react')).href;
const jsxRuntimeURL = pathToFileURL(require.resolve('react/jsx-runtime')).href;
const catalogues = { en: { ...en, ...enServerScreens }, tr: { ...tr, ...trServerScreens } };
const iconsURL = moduleURL(`import React from '${reactURL}';
const icon = name => props => React.createElement('i', { ...props, 'data-icon': name });
export const AlertTriangle = icon('AlertTriangle'), CheckCircle2 = icon('CheckCircle2'), DownloadCloud = icon('DownloadCloud'), Loader2 = icon('Loader2'), RefreshCw = icon('RefreshCw');`);
const uiURL = moduleURL(`import React from '${reactURL}'; export function Button({ children, ...props }) { return React.createElement('button', props, children); }`);
const i18nURL = moduleURL(`const fill = (text, vars) => String(text).replace(/\\{(\\w+)\\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
export function useI18n() { const c = globalThis.__checkCatalogue; return { t: (key, vars) => fill(c.strings[key] ?? key, vars), locale: c.locale }; }`);
const apiErrorURL = moduleURL(`export async function readApiError() { return {}; } export function apiErrorText() { return 'api-error'; }`);
const operationURL = moduleURL(`
export function createSystemUpdateRequestID() { return 'mounted-request-id'; }
export function systemUpdateResponseHint() { return 'response-error'; }
export function useSystemUpdateOperation() { return { active: false, start: async () => ({ kind: 'accepted' }), tracking: null }; }
export function validUpdateTarget() { return true; }
export function validUpdateVersion() { return true; }`);
const admissionURL = moduleURL(`
export function unverifiedHostMutationReadiness() { return { ready: false, code: 'HOST_MUTATION_UNAVAILABLE', reason: 'state_unverified' }; }
export function fetchHostMutationReadiness() { return globalThis.__checkReadiness(); }
export async function runHostMutationAdmission(read, onReady) { const r = await read(); if (r.ready) await onReady(); return r; }`);
const outcomeURL = compileURL('../src/lib/systemUpdateOutcome.ts', [
    [/from ['"]\.\/recoveryObservation['"]/g, `from '${compileURL('../src/lib/recoveryObservation.ts')}'`],
    [/from ['"]\.\/systemUpdateFailure['"]/g, `from '${compileURL('../src/lib/systemUpdateFailure.ts')}'`],
]);
const { PanelUpdateCard } = await import(compileURL('../src/components/PanelUpdateCard.tsx', [
    [/from ['"]\.\.\/lib\/systemUpdateOutcome['"]/g, `from '${outcomeURL}'`],
    [/from ['"]react['"]/g, `from '${reactURL}'`],
    [/from ['"]react\/jsx-runtime['"]/g, `from '${jsxRuntimeURL}'`],
    [/from ['"]lucide-react['"]/g, `from '${iconsURL}'`],
    [/from ['"]\.\/ui['"]/g, `from '${uiURL}'`],
    [/from ['"]\.\.\/i18n['"]/g, `from '${i18nURL}'`],
    [/from ['"]\.\.\/lib\/apiError['"]/g, `from '${apiErrorURL}'`],
    [/from ['"]\.\.\/lib\/panelUpdateAdmission['"]/g, `from '${admissionURL}'`],
    [/from ['"]\.\/SystemUpdateOperation['"]/g, `from '${operationURL}'`],
    [/from ['"]\.\/SystemUpdateSteps['"]/g, `from 'data:text/javascript,export function SystemUpdateSteps() { return null; } export function systemUpdateStepsAnnouncement() { return ""; }'`],
    [/from ['"]\.\/BuildCommit['"]/g, `from '${compileURL('../src/components/BuildCommit.tsx', [[/from ['"]react\/jsx-runtime['"]/g, `from '${jsxRuntimeURL}'`], [/from ['"]\.\.\/i18n['"]/g, `from '${i18nURL}'`]])}'`],
]));

const flush = async () => { for (let i = 0; i < 6; i += 1) await Promise.resolve(); };
function textOf(node) {
    if (node === null || node === undefined || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join(' ');
    return textOf(node.children);
}
const icons = node => node.findAll(child => child.props && child.props['data-icon'] !== undefined).map(child => child.props['data-icon']);
const CURRENT = { version: 'v0.1.0-alpha.89', commit: 'a'.repeat(40) };
const TARGET = { version: 'v0.1.0-alpha.90', commit: 'b'.repeat(40), sequence: '90', os: 'linux', arch: 'amd64', archive_sha256: 'c'.repeat(64), archive_size: '22500000' };

async function checkWith(locale, checkResponse, readiness = () => Promise.resolve({ ready: true })) {
    globalThis.__checkCatalogue = { locale, strings: catalogues[locale] };
    globalThis.__checkReadiness = readiness;
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async (path) => {
        if (String(path) === '/api/v1/panel/version') return { ok: true, json: async () => CURRENT };
        if (String(path) === '/api/v1/panel/update/check') return checkResponse();
        throw new Error('unexpected fetch ' + path);
    };
    let renderer;
    try {
        await act(async () => { renderer = TestRenderer.create(React.createElement(PanelUpdateCard)); await flush(); });
        await act(async () => { renderer.root.findAllByType('button')[0].props.onClick(); await flush(); });
    } finally {
        globalThis.fetch = originalFetch;
    }
    return renderer;
}
const messageOf = renderer => renderer.root.find(node => node.type === 'div' && node.props.role === 'status' && node.props['data-message-tone'] !== undefined);
const noUpdate = () => ({ ok: true, json: async () => ({ supported: true, available: false, current_version: CURRENT.version, current_commit: CURRENT.commit }) });
const anUpdate = () => ({ ok: true, json: async () => ({ supported: true, available: true, current_version: CURRENT.version, current_commit: CURRENT.commit, target: TARGET }) });

for (const [locale, sentence] of [['en', 'You are on the latest version.'], ['tr', 'En son sürümü kullanıyorsunuz.']]) {
    test(`${locale}: a check that found nothing is a calm confirmation, not a warning`, async () => {
        const renderer = await checkWith(locale, noUpdate);
        const message = messageOf(renderer);
        assert.equal(textOf(message.children).trim(), sentence);
        assert.equal(message.props['data-message-tone'], 'latest');
        assert.deepEqual(icons(message), ['CheckCircle2']);
        assert.ok(!/warning/.test(message.props.className), message.props.className);
        assert.ok(!/kanal|channel/i.test(textOf(message.children).trim()), 'the card does not speak of a channel it does not distinguish');
        renderer.unmount();
    });
}

test('a check that could not be read keeps the warning mark', async () => {
    const renderer = await checkWith('en', () => ({ ok: false, status: 502, json: async () => ({}) }));
    const message = messageOf(renderer);
    assert.equal(message.props['data-message-tone'], 'warning');
    assert.deepEqual(icons(message), ['AlertTriangle']);
    assert.ok(textOf(message.children).trim().length > 0);
    assert.notEqual(textOf(message.children).trim(), 'You are on the latest version.');
    renderer.unmount();
});

test('a found update is neutral information; a readiness read in progress is neutral; a refused readiness is a warning', async () => {
    let release;
    const renderer = await checkWith('en', anUpdate, () => new Promise((resolve) => { release = resolve; }));
    const message = messageOf(renderer);
    assert.equal(textOf(message.children).trim(), 'A signed update is available.');
    assert.equal(message.props['data-message-tone'], 'available');
    assert.deepEqual(icons(message), ['DownloadCloud']);
    const box = () => renderer.root.find(node => node.type === 'div' && node.props['data-readiness-tone'] !== undefined);
    assert.equal(box().props['data-readiness-tone'], 'neutral');
    assert.ok(!/warning/.test(box().props.className), box().props.className);
    assert.deepEqual(icons(box()), ['Loader2']);
    await act(async () => { release({ ready: false, code: 'HOST_MUTATION_BUSY', reason: 'operation_active' }); await flush(); });
    assert.equal(box().props['data-readiness-tone'], 'warning');
    assert.match(box().props.className, /warning/);
    assert.ok(icons(box()).includes('AlertTriangle'));
    renderer.unmount();
});
