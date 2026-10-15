import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ts from 'typescript';

// The document's language attribute follows the language the page is drawn in
// (set16 observation O1: every Turkish page was `lang="en"`, because only
// index.html set it). This mounts the real provider over the real catalogue
// files and reads `document.documentElement.lang` at startup, while a switch's
// catalogue is still loading, and after it has arrived. Against the previous
// provider the attribute never changes from "en", so every Turkish case fails.
//
// Belgenin dil özniteliği, sayfanın çizildiği dili izler (set16 gözlemi O1:
// Türkçe sayfalar `lang="en"` idi). Gerçek sağlayıcı gerçek kataloglarla
// bağlanır; açılışta, dil değişirken ve yeni katalog gelince öznitelik okunur.
const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const jsxRuntimeURL = pathToFileURL(require.resolve('react/jsx-runtime')).href;
const moduleURL = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const compile = (path) => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
}).outputText;

const catalogueURL = {
    './en': moduleURL(compile('../src/i18n/en.ts')),
    './tr': moduleURL(compile('../src/i18n/tr.ts')),
    './screens/en': moduleURL(compile('../src/i18n/screens/en.ts')),
    './screens/tr': moduleURL(compile('../src/i18n/screens/tr.ts')),
    './screens/server/en': moduleURL(compile('../src/i18n/screens/server/en.ts')),
    './screens/server/tr': moduleURL(compile('../src/i18n/screens/server/tr.ts')),
};

let scenario = 0;
async function loadProvider(heldPart) {
    const id = ++scenario;
    const imports = { ...catalogueURL };
    if (heldPart) imports[heldPart] = moduleURL(`// scenario ${id}\nawait globalThis.langPartGate;\nexport * from '${catalogueURL[heldPart]}';`);
    const source = compile('../src/i18n/index.tsx')
        .replace(/from ['"]([^'"]+)['"]/g, (_m, path) => {
            const target = path === 'react' ? reactURL : path === 'react/jsx-runtime' ? jsxRuntimeURL : null;
            assert.ok(target, `unmapped static import ${path}`);
            return `from '${target}'`;
        })
        .replace(/import\(['"]([^'"]+)['"]\)/g, (_m, path) => {
            assert.ok(imports[path], `unmapped dynamic import ${path}`);
            return `import('${imports[path]}')`;
        });
    return import(moduleURL(`// lang provider ${id}\n${source}`));
}

function gate() {
    let open;
    globalThis.langPartGate = new Promise((resolve) => { open = resolve; });
    return open;
}

async function mount(i18n, stored) {
    globalThis.window = { localStorage: { getItem: () => stored, setItem: () => {} } };
    globalThis.document = { documentElement: { lang: 'en' } };
    let api = null;
    function Probe() {
        api = i18n.useI18n();
        return React.createElement('p', null, api.t('app.name'));
    }
    let renderer;
    await act(async () => {
        renderer = TestRenderer.create(React.createElement(i18n.I18nProvider, null, React.createElement(Probe)));
    });
    const settle = async () => {
        for (let i = 0; i < 20; i++) await act(async () => { await new Promise((resolve) => setTimeout(resolve, 5)); });
    };
    return {
        settle,
        lang: () => globalThis.document.documentElement.lang,
        locale: () => api?.locale,
        switchTo: (locale) => act(async () => api.setLocale(locale)),
        unmount: () => act(async () => renderer.unmount()),
    };
}

test('a Turkish page is lang="tr" from the boot screen on', async () => {
    const open = gate();
    const i18n = await loadProvider('./tr');
    const view = await mount(i18n, 'tr');
    await view.settle();
    assert.equal(view.locale(), undefined, 'the shell catalogue was held back');
    assert.equal(view.lang(), 'tr', 'the boot screen is drawn in Turkish, so the document is Turkish');
    open();
    await view.settle();
    assert.equal(view.locale(), 'tr');
    assert.equal(view.lang(), 'tr');
    await view.unmount();
});

test('an English page stays lang="en"', async () => {
    const i18n = await loadProvider(null);
    const view = await mount(i18n, 'en');
    await view.settle();
    assert.equal(view.locale(), 'en');
    assert.equal(view.lang(), 'en');
    await view.unmount();
});

for (const [from, to] of [['en', 'tr'], ['tr', 'en']]) {
    test(`switching ${from} → ${to} changes lang when the ${to} text is drawn, not before`, async () => {
        const i18n = await loadProvider(`./${to}`);
        const open = gate();
        const view = await mount(i18n, from);
        await view.settle();
        assert.equal(view.lang(), from);
        await view.switchTo(to);
        await view.settle();
        assert.equal(view.locale(), from, 'the new shell catalogue was held back');
        assert.equal(view.lang(), from, 'the page is still drawn in the old language');
        open();
        await view.settle();
        assert.equal(view.locale(), to);
        assert.equal(view.lang(), to);
        await view.unmount();
    });
}

test.after(() => {
    delete globalThis.document;
});
