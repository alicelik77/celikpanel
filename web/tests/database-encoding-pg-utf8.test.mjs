// A PostgreSQL database's text encoding on its card (20 Nov 2026). The domain
// list carries each PostgreSQL database's encoding, read by the server in one
// call; the card names it after the creation date, and a database that is not
// UTF-8 says so once, in one guidance block: what it means, who acts and the
// way out through the card's own buttons.
// Bir PostgreSQL veritabanının metin kodlaması kartında; UTF-8 olmayan
// veritabanı bunu bir kez, çıkış yoluyla söyler.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, domainDatabasesURL, reactURL } from './fixtures/shared-layer.mjs';
import { englishScreens, turkishScreens } from './locale-catalogue.mjs';
import { enDbEncodingScreens } from '../src/i18n/screens/dbEncoding/en.ts';
import { trDbEncodingScreens } from '../src/i18n/screens/dbEncoding/tr.ts';

const { encodingLabel, databaseEncodingWarning } = await import(domainDatabasesURL);

test('only a PostgreSQL database known not to be UTF8 gets a warning, of its kind', () => {
    assert.equal(databaseEncodingWarning('postgresql', 'SQL_ASCII'), 'unchecked');
    assert.equal(databaseEncodingWarning('postgresql', 'LATIN1'), 'limited');
    assert.equal(databaseEncodingWarning('postgresql', 'UTF8'), null);
    assert.equal(databaseEncodingWarning('postgresql', 'utf8'), null);
    // Not known: nothing is said rather than guessed.
    assert.equal(databaseEncodingWarning('postgresql', undefined), null);
    assert.equal(databaseEncodingWarning('postgresql', ''), null);
    assert.equal(databaseEncodingWarning('mariadb', 'SQL_ASCII'), null);
    assert.equal(encodingLabel('UTF8'), 'UTF-8');
    assert.equal(encodingLabel('SQL_ASCII'), 'SQL_ASCII');
});

const stub = dataModule(`
  import React from '${reactURL}';
  export const useI18n = () => ({ t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' });
  export const whoWord = (t, who) => t('kit.guidance.who.' + who);
  export function Guidance(props) {
    return React.createElement('div', { 'data-guidance': props.tone, 'data-live': props.live, 'data-kind': props['data-database-encoding-warning'] },
      ['title', 'why', 'who', 'next', 'then'].map((row) => React.createElement('p', { key: row }, row + '=' + props[row])));
  }
`);
const warningURL = compileSource('components/DatabaseEncodingWarning.tsx', (specifier) => (
    specifier.endsWith('/domainDatabases') ? domainDatabasesURL : stub
));
const { DatabaseEncodingWarning } = await import(warningURL);

async function render(props) {
    let tree;
    await act(async () => { tree = Renderer.create(React.createElement(DatabaseEncodingWarning, props)); });
    const text = JSON.stringify(tree.toJSON());
    await act(async () => tree.unmount());
    return text;
}

test('a SQL_ASCII database: unchecked text, you act, through Delete or Copy; not announced on every read', async () => {
    const text = await render({ kind: 'unchecked', encoding: 'SQL_ASCII', canAct: true });
    assert.match(text, /"data-guidance":"blocked","data-live":"off","data-kind":"unchecked"/);
    assert.match(text, /title=dbenc\.title\.unchecked\{\\"encoding\\":\\"SQL_ASCII\\"\}/);
    assert.match(text, /why=dbenc\.why\.unchecked/);
    assert.match(text, /who=kit\.guidance\.who\.you/);
    assert.match(text, /next=dbenc\.next/);
    assert.match(text, /then=dbenc\.then/);
});

test('someone who cannot use the card\'s buttons reads who can; another encoding says it cannot hold every letter', async () => {
    const text = await render({ kind: 'limited', encoding: 'LATIN1', canAct: false });
    assert.match(text, /title=dbenc\.title\.limited\{\\"encoding\\":\\"LATIN1\\"\}/);
    assert.match(text, /who=dbenc\.who\.manager/);
    assert.doesNotMatch(text, /who\.you/);
});

test('both languages carry every text; the steps name the card\'s own buttons', () => {
    assert.deepEqual(Object.keys(trDbEncodingScreens).sort(), Object.keys(enDbEncodingScreens).sort());
    for (const key of Object.keys(enDbEncodingScreens)) {
        assert.ok(englishScreens.includes(`'${key}'`) && turkishScreens.includes(`'${key}'`), key);
        assert.notEqual(trDbEncodingScreens[key], enDbEncodingScreens[key], key);
    }
    // The button names as the card shows them (dataTools catalogue).
    const dataToolsTr = readFileSync(new URL('../src/i18n/dataTools/tr.ts', import.meta.url), 'utf8');
    assert.match(dataToolsTr, /'dbt\.copy': 'Kopyala'/);
    assert.match(trDbEncodingScreens['dbenc.next'], /Kopyala/);
    assert.match(enDbEncodingScreens['dbenc.next'], /Copy/);
    // What happens when the text is not UTF-8, and that new databases are UTF-8.
    assert.match(trDbEncodingScreens['dbenc.then'], /hiçbir şey değişmez/);
    assert.match(enDbEncodingScreens['dbenc.then'], /New databases are always created as UTF-8/);
});

test('the card names the encoding after the creation date and shows the warning once, outside its tools', () => {
    const card = readFileSync(new URL('../src/components/DomainDatabaseManager.tsx', import.meta.url), 'utf8');
    assert.match(card, /t\('dbenc\.fact', \{ encoding: encodingLabel\(db\.encoding\) \}\)/);
    assert.equal(card.match(/<DatabaseEncodingWarning /g)?.length, 1);
    assert.ok(card.indexOf('<DatabaseEncodingWarning ') < card.indexOf('<DatabaseTools '), 'the warning comes before the tools');
    const tools = readFileSync(new URL('../src/components/DatabaseTools.tsx', import.meta.url), 'utf8');
    assert.doesNotMatch(tools, /dbenc\./);
    // The server's field name.
    const handler = readFileSync(new URL('../../cmd/panel/domain_database_handlers.go', import.meta.url), 'utf8');
    assert.match(handler, /json:"encoding,omitempty"/);
});
