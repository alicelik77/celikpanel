// A database's tools on the domain's Databases tab (alpha.129): the
// connection guide says how the site connects and how a person connects from
// their own computer. Databases listen on this server only, so the
// administrator is given the SSH tunnel's fields and anyone else the web tool
// in the panel (or, when it is not installed, that the provider installs it);
// remote access itself is not offered.
// Bağlantı rehberi: site için bilgiler; yöneticiye SSH tüneli, diğerlerine
// paneldeki web aracı.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, domainDatabasesURL, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  export const useNavigate = () => () => true;
  export const CopyPlus = () => null;
  export const Download = () => null;
  export const ShieldCheck = () => null;
  export const Upload = () => null;
  export const Wrench = () => null;
  export const HardDrive = () => null;
  export const Table2 = () => null;
  export const Plug = () => null;
  const words = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => words;
  export const useDataToolsWords = () => words;
  export const formatSize = (n) => n + ' B';
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const { DatabaseTools } = await import(shared.compile('components/DatabaseTools.tsx'));

const info = { known: true, size_bytes: 2048, tables: 12, connections: 1, host: 'localhost', port: 3306, user: 'shop_db_user', can_repair: true };
const originalFetch = globalThis.fetch;
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
let tree;
async function mount(props) {
    globalThis.fetch = () => Promise.resolve(Response.json(info));
    globalThis.window = { location: { hostname: 'panel.example.net' } };
    await act(async () => {
        tree = Renderer.create(React.createElement(DatabaseTools, {
            domainId: 4, db: { id: 9, name: 'shop_db', type: 'mysql' }, readOnly: false, onRefusal: () => {}, onClear: () => {}, ...props,
        }));
        await settled();
        await settled();
    });
}
async function cleanup() {
    if (tree) await act(async () => tree.unmount());
    tree = undefined;
    globalThis.fetch = originalFetch;
    delete globalThis.window;
}
const text = () => JSON.stringify(tree.toJSON());
const words = (node) => (typeof node === 'string' ? node : (node?.children ?? []).map(words).join(' '));

test('the administrator gets the SSH tunnel: SSH host, port 22, then 127.0.0.1 and the database port', async () => {
    await mount({ isAdmin: true, webTool: 'installed' });
    try {
        const shown = text();
        for (const part of ['dbt.fromSite', 'dbt.tunnelIntro', 'panel.example.net', '"22"', '127.0.0.1', '3306', 'dbt.tunnelToolsMaria']) {
            assert.ok(shown.includes(part), `${part} in ${shown}`);
        }
        assert.ok(!shown.includes('dbt.noRemote'), 'no "not available" sentence for the administrator');
    } finally {
        await cleanup();
    }
});

test('a customer is told plainly that connecting from their computer is not available, and where to go instead', async () => {
    await mount({ isAdmin: false, webTool: 'installed' });
    try {
        const shown = text();
        assert.ok(shown.includes('dbt.noRemoteUseTool') && shown.includes('phpMyAdmin'), shown);
        assert.ok(!shown.includes('dbt.tunnelIntro') && !shown.includes('panel.example.net'), 'no SSH fields for a customer');
    } finally {
        await cleanup();
    }
    await mount({ isAdmin: false, webTool: 'missing', db: { id: 9, name: 'shop_pg', type: 'postgresql' } });
    try {
        const shown = text();
        assert.ok(shown.includes('dbt.noRemoteAskTool') && shown.includes('phpPgAdmin'), shown);
    } finally {
        await cleanup();
    }
});

test('size, tables and connections are three small boxes, and Import without a file is a plain unavailable button with its reason', async () => {
    await mount({ isAdmin: true, webTool: 'installed' });
    try {
        const stats = tree.root.findAll((node) => node.type === 'dl' && node.props['data-database-stats'] === 9)[0];
        assert.ok(stats, 'the stat boxes are drawn');
        assert.deepEqual(stats.findAll((node) => node.type === 'dd').map((node) => node.children.join('')), ['2048 B', '12', '1']);
        const openImport = tree.root.findAll((node) => node.type === 'button' && words(node).trim() === 'dbt.import')[0];
        await act(async () => openImport.props.onClick());
        const start = tree.root.findAll((node) => node.type === 'button' && words(node).includes('dbt.importStart'))[0];
        assert.equal(start.props.disabled, true);
        assert.ok(!String(start.props.className).includes('dashed'), start.props.className);
        assert.ok(JSON.stringify(tree.toJSON()).includes('dbt.importChooseFirst'));
    } finally {
        await cleanup();
    }
});

test('a card names the engine the server runs: MariaDB is never "MYSQL", the major version when known', async () => {
    const { databaseEngine, engineLabel } = await import(domainDatabasesURL);
    assert.equal(engineLabel(databaseEngine('mariadb', 'mysql'), '11.8'), 'MariaDB 11.8');
    assert.equal(engineLabel(databaseEngine(undefined, 'mysql')), 'MariaDB', 'an answer without the engine is still MariaDB');
    assert.equal(engineLabel(databaseEngine('mysql', 'mysql'), '8.0'), 'MySQL 8.0');
    assert.equal(engineLabel(databaseEngine('postgresql', 'postgresql'), '17'), 'PostgreSQL 17');
});
