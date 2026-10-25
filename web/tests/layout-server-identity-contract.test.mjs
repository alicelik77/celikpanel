import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { englishCatalogue, turkishCatalogue } from './locale-catalogue.mjs';

const layout = readFileSync(new URL('../src/components/Layout.tsx', import.meta.url), 'utf8');
const en = englishCatalogue;
const tr = turkishCatalogue;

test('the admin shell reuses one bounded no-store runtime response', () => {
  assert.equal(
    (layout.match(/fetch\('\/api\/v1\/panel\/version'/g) ?? []).length,
    1,
    'Layout must make exactly one panel-version request',
  );
  assert.match(layout, /if \(role !== 'admin'\) \{\s*setPanelRuntime\(null\)/);
  assert.match(layout, /cache: 'no-store', credentials: 'same-origin'/);
  assert.doesNotMatch(layout, /api\.getSystemStats\(\)/);
  assert.match(layout, /raw\.hostname === 'string' && raw\.hostname\.length <= 253/);
  assert.match(layout, /raw\.ipv4 === 'string' && raw\.ipv4\.length <= 64/);
});

test('hostname and IPv4 remain visible in the desktop footer and narrow header', () => {
  assert.match(layout, /<section\s+data-server-identity="sidebar"/);
  assert.match(layout, /<section\s+data-server-identity="mobile"/);
  assert.match(layout, /placement="mobile"/);
  assert.match(layout, /max-w-\[10rem\] md:hidden/);
  assert.match(layout, /dir="ltr"/);
  assert.match(layout, /\{identity\.hostname\}/);
  assert.match(layout, /\{identity\.ipv4\}/);
  assert.match(layout, /<aside id="celikpanel-sidebar" className="hidden shrink-0 md:block"/);

  const sidebarIdentity = layout.indexOf('placement="sidebar"');
  const buildStamp = layout.indexOf('<BuildStamp runtime={panelRuntime} />', sidebarIdentity);
  assert.ok(
    sidebarIdentity >= 0 && buildStamp > sidebarIdentity,
    'desktop identity must appear above the build stamp',
  );

  const mobileHeader = layout.indexOf('<header');
  const mobileIdentity = layout.indexOf('placement="mobile"');
  const mobileHeaderEnd = layout.indexOf('</header>', mobileHeader);
  assert.ok(
    mobileHeader >= 0 && mobileIdentity > mobileHeader && mobileIdentity < mobileHeaderEnd,
    'compact identity must be rendered inside the narrow-screen header',
  );
});

test('the build stamp consumes the shared runtime without a second request', () => {
  const start = layout.indexOf('function BuildStamp');
  assert.ok(start >= 0, 'BuildStamp must exist');
  const buildStamp = layout.slice(start);
  assert.match(buildStamp, /function BuildStamp\(\{ runtime \}/);
  assert.match(buildStamp, /if \(!runtime\) return/);
  assert.doesNotMatch(buildStamp, /fetch\(/);
  assert.doesNotMatch(buildStamp, /useState/);
});

test('server identity uses existing localized labels', () => {
  assert.match(layout, /t\('dashboard\.serverInfo'\)/);
  for (const key of ['dashboard.serverInfo', 'dashboard.hostname', 'dashboard.ipv4']) {
    assert.ok(en.includes("'" + key + "':"), 'missing EN key ' + key);
    assert.ok(tr.includes("'" + key + "':"), 'missing TR key ' + key);
  }
});

// The build id is one unit that never breaks inside (owner, alpha.83 and
// alpha.84: "build f01" / "57e6c8f16"). Rendered from source with every other
// import of Layout stubbed.
test('the sidebar build id wraps only before "build", never inside the id, and keeps the full id', async () => {
  const { createRequire } = await import('node:module');
  const { pathToFileURL } = await import('node:url');
  const ts = (await import('typescript')).default;
  const React = (await import('react')).default;
  const Renderer = (await import('react-test-renderer')).default;
  const require = createRequire(import.meta.url);
  const reactURL = pathToFileURL(require.resolve('react')).href;
  const data = (text) => 'data:text/javascript;base64,' + Buffer.from(text).toString('base64');
  const names = new Set();
  for (const match of layout.matchAll(/^import (?!type)\{([^}]+)\} from '([^']+)';$/gm)) {
    if (match[2] === 'react') continue;
    for (const item of match[1].split(',')) { const name = item.replace(/\btype\b/, '').trim(); if (name && !/^type\s/.test(item.trim())) names.add(name); }
  }
  names.delete('useI18n');
  names.delete('BuildCommit');
  const stub = data(`import React from '${reactURL}';
    ${[...names].map((name) => `export const ${name} = () => null;`).join('\n')}
    export const useI18n = () => ({ t: (key, vars) => vars ? key + ' ' + Object.values(vars).join(' ') : key, locale: 'en' });`);
  const jsx = { compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } };
  // The stamp's build line is the shared, copy-clean one (2026-10-16).
  const buildCommit = data(`import React from '${reactURL}';\n${ts.transpileModule(readFileSync(new URL('../src/components/BuildCommit.tsx', import.meta.url), 'utf8'), jsx).outputText
    .replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => `from '${specifier === 'react' ? reactURL : stub}'`)}`);
  const compiled = ts.transpileModule(layout, jsx).outputText
    .replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => `from '${specifier === 'react' ? reactURL : specifier === './BuildCommit' ? buildCommit : stub}'`);
  const { BuildStamp } = await import(data(`import React from '${reactURL}';\n${compiled}`));
  const commit = 'f0157e6c8f16a1b2c3d4e5f60718293a4b5c6d7e';
  const tree = Renderer.create(React.createElement(BuildStamp, { runtime: { version: 'v0.1.0-alpha.84', commit, agent_commit: commit, agent_matches: true } }));
  const unit = tree.root.find((node) => node.props['data-build-stamp'] !== undefined);
  assert.match(unit.props.className, /whitespace-nowrap/);
  assert.doesNotMatch(unit.props.className, /break-all|break-words|overflow-wrap/);
  assert.ok(unit.props.title.endsWith(commit), 'the full id is in the title');
  const walk = (node) => (typeof node === 'string' ? node : (node.children || []).map(walk).join(' '));
  const shown = walk(unit);
  assert.ok(shown.includes('f0157e6c8f16') && shown.includes(commit), 'the short id is shown and the full id is kept for screen readers');
  // Owner 2026-10-16: copying the stamp gives what is seen, "build f0157e6c8f16".
  const copied = (node) => (typeof node === 'string' ? node
    : String(node.props?.className ?? '').split(/\s+/).includes('select-none') ? '' : (node.children || []).map(copied).join(''));
  assert.equal(copied(unit), 'build f0157e6c8f16');
});
