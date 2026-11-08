import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { compileSource } from './fixtures/shared-layer.mjs';

// lib/configRefusalText.ts (alpha.130, 8 Nov 2026): a refused save of
// pg_hba.conf, postgresql.conf or a MariaDB option file leads with a plain
// sentence that names the fix. The owner read "a rule you changed or added is
// not one PostgreSQL accepts" and could not find why. The pg_hba.conf lines
// are the exact ones the agent's check gives (Go hbaLineError); a line the
// table does not know gives null, so the screen shows the service's own words
// and never a guessed reason.

const { plainConfigReason } = await import(compileSource('lib/configRefusalText.ts', (specifier) => {
  throw new Error(`lib/configRefusalText.ts imports ${specifier}; it is meant to stand alone`);
}));

const hba = [
  ['invalid connection type "hots"', 'dbconf.why.hba.type', 'hots'],
  ['end-of-line before database specification', 'dbconf.why.hba.noDatabase', ''],
  ['end-of-line before role specification', 'dbconf.why.hba.noUser', ''],
  ['end-of-line before IP address specification', 'dbconf.why.hba.noAddress', ''],
  ['end-of-line before netmask specification', 'dbconf.why.hba.noMask', ''],
  ['invalid CIDR mask in address "203.0.113.5/40"', 'dbconf.why.hba.badCidr', '203.0.113.5/40'],
  ['specifying both host name and CIDR mask is invalid: "db.example.com/32"', 'dbconf.why.hba.hostWithMask', 'db.example.com/32'],
  ['invalid IP mask "255.255.0"', 'dbconf.why.hba.badMask', '255.255.0'],
  ['IP address and mask do not match', 'dbconf.why.hba.maskFamily', ''],
  ['end-of-line before authentication method', 'dbconf.why.hba.noMethod', ''],
  ['invalid authentication method "scram"', 'dbconf.why.hba.badMethod', 'scram'],
  ['peer authentication is only supported on local sockets', 'dbconf.why.hba.peerHost', ''],
  ['cert authentication is only supported on hostssl connections', 'dbconf.why.hba.certNoSsl', ''],
  ['gss authentication is not supported on local sockets', 'dbconf.why.hba.localGss', 'gss'],
  ['sspi authentication is not supported on local sockets', 'dbconf.why.hba.localGss', 'sspi'],
  ['multiple values specified for connection type', 'dbconf.why.hba.multiple', ''],
  ['multiple values specified for host address', 'dbconf.why.hba.multiple', ''],
  ['multiple values specified for netmask', 'dbconf.why.hba.multiple', ''],
  ['multiple values specified for authentication type', 'dbconf.why.hba.multiple', ''],
  ['authentication option not in name=value format: map', 'dbconf.why.hba.optionFormat', 'map'],
  ['unrecognized authentication option name: "mapp"', 'dbconf.why.hba.optionName', 'mapp'],
  ['clientcert can only be configured for "hostssl" rows', 'dbconf.why.hba.clientcert', ''],
  ['an include directive takes exactly one file or directory name', 'dbconf.why.hba.include', ''],
  ['hostssl record cannot match because SSL is disabled', 'dbconf.why.hba.sslOff', ''],
];

test('every pg_hba.conf line the agent gives has a plain sentence, with the value it names', () => {
  for (const [detail, key, value] of hba) {
    assert.deepEqual(plainConfigReason(detail), { key, vars: { value } }, detail);
  }
});

test('the table covers every reason hbaLineError can return', () => {
  // The Go function is the source of these lines: a new reason there without
  // a sentence here would fall back to PostgreSQL's words alone.
  const go = readFileSync(new URL('../../cmd/agent/db_config_hba.go', import.meta.url), 'utf8');
  const body = go.slice(go.indexOf('func hbaLineError('), go.indexOf('func validateHBACandidate('));
  const returned = [...body.matchAll(/return ("[^"]+"|`[^`]+`)/g)].map((match) => match[1].slice(1, -1));
  assert.ok(returned.length >= 15, 'hbaLineError was not found where it used to be');
  const samples = { 'invalid connection type "': 'invalid connection type "x"', 'specifying both host name and CIDR mask is invalid: "': 'specifying both host name and CIDR mask is invalid: "x"', 'invalid CIDR mask in address "': 'invalid CIDR mask in address "x"', 'invalid IP mask "': 'invalid IP mask "x"', 'invalid authentication method "': 'invalid authentication method "x"', 'authentication option not in name=value format: ': 'authentication option not in name=value format: x', 'unrecognized authentication option name: "': 'unrecognized authentication option name: "x"', ' authentication is not supported on local sockets': 'gss authentication is not supported on local sockets' };
  for (const line of returned) {
    const detail = samples[line] ?? line;
    assert.ok(plainConfigReason(detail), `hbaLineError can return "${line}" and the screen has no plain sentence for it`);
  }
});

test('settings: what postgres -C and mariadbd print', () => {
  assert.deepEqual(plainConfigReason('unrecognized configuration parameter "max_conections"'), { key: 'dbconf.why.setting.unknown', vars: { value: 'max_conections' } });
  assert.deepEqual(plainConfigReason('invalid value for parameter "max_connections": "lots"'), { key: 'dbconf.why.setting.invalidValue', vars: { value: 'lots' } });
  assert.deepEqual(plainConfigReason('parameter "fsync" requires a Boolean value'), { key: 'dbconf.why.setting.needsBoolean', vars: { value: '' } });
  assert.deepEqual(plainConfigReason('parameter "work_mem" requires a numeric value'), { key: 'dbconf.why.setting.needsNumber', vars: { value: '' } });
  assert.deepEqual(plainConfigReason('100000000 is outside the valid range for parameter "max_connections" (1 .. 262143)'), { key: 'dbconf.why.setting.range', vars: { value: '1 .. 262143' } });
  assert.equal(plainConfigReason('syntax error in file "/etc/postgresql/17/main/postgresql.conf" line 64, near token "MB"').key, 'dbconf.why.setting.syntax');
  assert.deepEqual(plainConfigReason("unknown variable 'max_connectionz=10'"), { key: 'dbconf.why.setting.unknown', vars: { value: 'max_connectionz' } });
  assert.deepEqual(plainConfigReason("unknown option '--skip-thing'"), { key: 'dbconf.why.setting.unknown', vars: { value: 'skip-thing' } });
  assert.deepEqual(plainConfigReason("option 'max_connections': unsigned value 0 adjusted to 10."), { key: 'dbconf.why.setting.adjusted', vars: { value: '10' } });
  assert.deepEqual(plainConfigReason("option 'skip_name_resolve': boolean value 'maybe' wasn't recognized. Set to OFF."), { key: 'dbconf.why.setting.notBoolean', vars: { value: 'OFF' } });
});

test('a line the table does not know is null: the screen then shows the service’s words only', () => {
  assert.equal(plainConfigReason(''), null);
  assert.equal(plainConfigReason('   '), null);
  assert.equal(plainConfigReason('could not open file "/etc/postgresql/17/main/conf.d/x.conf": Permission denied'), null);
  assert.equal(plainConfigReason('Job for postgresql@17-main.service failed'), null);
});

test('every sentence exists in English and Turkish, names the value it is given, and stays out of the boot catalogue', () => {
  const read = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
  const lib = read('lib/configRefusalText.ts');
  const keys = [...new Set([...lib.matchAll(/'(dbconf\.why\.[a-zA-Z.]+)'/g)].map((match) => match[1]))];
  assert.ok(keys.length >= 28);
  const en = read('i18n/screens/dbConfig/en.ts');
  const tr = read('i18n/screens/dbConfig/tr.ts');
  const valueOf = (source, key) => source.match(new RegExp(`'${key.replace(/\./g, '\\.')}': "([^"]*)"`))?.[1];
  const valued = new Set(hba.filter(([, , value]) => value).map(([, key]) => key));
  for (const key of keys) {
    const english = valueOf(en, key);
    const turkish = valueOf(tr, key);
    assert.ok(english && turkish, `${key} is missing in a locale`);
    assert.notEqual(english, turkish, `${key} is not translated`);
    if (valued.has(key)) {
      assert.ok(english.includes('{value}') && turkish.includes('{value}'), `${key} does not name the value`);
    }
  }
  for (const boot of ['i18n/en.ts', 'i18n/tr.ts']) {
    assert.ok(!/'dbconf\.(why|target)\./.test(read(boot)), `${boot} holds refusal copy; it belongs in the lazy dbConfig part`);
  }
});
