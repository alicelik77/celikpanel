import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';

// The shared kit of alpha.132 (components/kit/): the row actions with their
// "More" menu, the guidance block, the state label and the words helpers,
// compiled from source so a mounted screen draws the shipped pieces.
//
// kitModules(i18nURL, uiURL) gives their module URLs. `i18nURL` is a module
// that exports useI18n (a test's stub), `uiURL` one that exports Button and
// StatusDot (the real components/ui.tsx or a stub). Icons draw nothing, and
// the menu's portal renders in place, as react-test-renderer has no document.
//
// alpha.132 ortak parçaları, bağlanan testte gönderilen kodun kendisi olsun
// diye kaynaktan derlenir.
const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const dataModule = (text) => 'data:text/javascript;base64,' + Buffer.from(text).toString('base64');
const compilerOptions = { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 };
const read = (path) => readFileSync(new URL('../../src/' + path, import.meta.url), 'utf8');

export const kitIconNames = ['AlertTriangle', 'CheckCircle2', 'ChevronDown', 'HelpCircle', 'Hourglass', 'Loader2', 'XCircle'];
const icons = dataModule(kitIconNames.map((name) => `export const ${name} = () => null;`).join(' '));
const portal = dataModule('export const createPortal = (node) => node;');

function compile(path, resolve) {
  const out = ts.transpileModule(read(path), { compilerOptions }).outputText
    .replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => `from '${specifier === 'react' ? reactURL : resolve(specifier)}'`);
  return dataModule(`import React from '${reactURL}';\n${out}`);
}

const cache = new Map();
export function kitModules(i18nURL, uiURL) {
  const key = i18nURL + '|' + uiURL;
  if (cache.has(key)) return cache.get(key);
  const words = compile('components/kit/words.ts', () => i18nURL);
  const base = (specifier) => (specifier === 'lucide-react' ? icons
    : specifier === 'react-dom' ? portal
      : specifier.endsWith('/ui') ? uiURL
        : specifier.endsWith('/words') ? words
          : i18nURL);
  const modules = {
    words,
    guidance: compile('components/kit/rows/Guidance.tsx', base),
    rowActions: compile('components/kit/rows/RowActions.tsx', base),
    stateLabel: compile('components/kit/rows/StateLabel.tsx', base),
  };
  cache.set(key, modules);
  return modules;
}

// The resolver entries for a screen's imports of the kit, by import suffix.
export function kitTable(i18nURL, uiURL) {
  const kit = kitModules(i18nURL, uiURL);
  return {
    '/kit/words': kit.words,
    '/kit/rows/Guidance': kit.guidance,
    '/kit/rows/RowActions': kit.rowActions,
    '/kit/rows/StateLabel': kit.stateLabel,
  };
}
