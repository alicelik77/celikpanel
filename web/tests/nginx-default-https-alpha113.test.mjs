// alpha.113: CelikPanel's default HTTPS server on the Nginx page. The state is
// read from the server and drawn with its reason, who acts, the next action
// and what happens next (D-024); a state the page does not know is unknown,
// never a guess; reading never changes anything, and "Set up now" / "Try
// again" is one identified POST offered only where something can be set up.
//
// alpha.113: Nginx sayfasında varsayılan HTTPS sunucusu. Okuma hiçbir şeyi
// değiştirmez; düğme yalnız kurulacak bir şey varsa çıkar.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  DEFAULT_HTTPS_STATES, decodeDefaultHTTPS, defaultHTTPSOffersSetup, defaultHTTPSTone, NGINX_DEFAULT_HTTPS_URL,
} from '../src/lib/nginxDefaultHTTPS.ts';
import { isReaskRoute } from '../src/lib/requestIdentity.ts';
import { enNginxDefaultHttpsScreens as en } from '../src/i18n/screens/nginxDefaultHttps/en.ts';
import { trNginxDefaultHttpsScreens as tr } from '../src/i18n/screens/nginxDefaultHttps/tr.ts';

const read = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

test('the decoder reads the Agent\'s answer and refuses anything else', () => {
  const answer = decodeDefaultHTTPS({
    outcome: 'failed',
    status: { state: 'failed', mode: 'placeholder', nginx_version: '1.18.0', path: '/etc/nginx/conf.d/celikpanel-default-https.conf', detail: 'nginx -t refused it', last_at: '2026-10-30T10:00:00Z' },
  });
  assert.equal(answer.outcome, 'failed');
  assert.deepEqual(answer.status, {
    state: 'failed', mode: 'placeholder', nginxVersion: '1.18.0', path: '/etc/nginx/conf.d/celikpanel-default-https.conf',
    ownerFile: '', detail: 'nginx -t refused it', lastAt: '2026-10-30T10:00:00Z',
  });
  assert.equal(decodeDefaultHTTPS({ status: { state: 'not_needed', path: '/x' } }).status.mode, '');
  for (const bad of [
    null, [], 'x', {}, { status: null }, { status: [] }, { status: { state: 'broken' } }, { status: { state: 'active', mode: 'other' } },
    { status: { state: 'active', path: 7 } }, { outcome: 1, status: { state: 'active' } },
  ]) assert.throws(() => decodeDefaultHTTPS(bad), JSON.stringify(bad));
});

test('only a state with something to set up offers the change', () => {
  const offered = DEFAULT_HTTPS_STATES.filter(defaultHTTPSOffersSetup);
  assert.deepEqual(offered, ['pending', 'failed']);
  assert.equal(defaultHTTPSTone('active'), 'ok');
  assert.equal(defaultHTTPSTone('failed'), 'bad');
  assert.equal(defaultHTTPSTone('unknown'), 'unknown');
});

test('the change is an identified route; reading is not', () => {
  assert.equal(NGINX_DEFAULT_HTTPS_URL, '/api/v1/nginx/default-https');
  assert.equal(isReaskRoute('POST', NGINX_DEFAULT_HTTPS_URL), true);
  assert.equal(isReaskRoute('GET', NGINX_DEFAULT_HTTPS_URL), false);
  const card = read('components/NginxDefaultHTTPSCard.tsx');
  // One POST, only from the button's handler, through the lost-answer hook.
  assert.equal([...card.matchAll(/method: 'POST'/g)].length, 1);
  assert.match(card, /answer\.send\(NGINX_DEFAULT_HTTPS_URL, \{ method: 'POST'/);
  assert.doesNotMatch(card, /useEffect/, 'the card sends nothing by itself');
  assert.match(read('components/NginxManagement.tsx'), /<NginxDefaultHTTPSCard \/>/);
});

test('every state has its words in English and Turkish, with the same placeholders', () => {
  const keys = ['nginxdh.title', 'nginxdh.checking', 'nginxdh.unknownRead', 'nginxdh.label.who', 'nginxdh.label.next', 'nginxdh.label.then',
    'nginxdh.nobody', 'nginxdh.active.reject', 'nginxdh.active.placeholder', 'nginxdh.setup', 'nginxdh.retry', 'nginxdh.running',
    'nginxdh.check', 'nginxdh.version', 'nginxdh.versionUnknown', 'nginxdh.lastAttempt'];
  for (const state of DEFAULT_HTTPS_STATES) {
    keys.push(`nginxdh.${state}.head`);
    if (state !== 'active') keys.push(`nginxdh.${state}.reason`);
    if (state !== 'active' && state !== 'not_needed') keys.push(`nginxdh.${state}.who`, `nginxdh.${state}.next`);
    if (['not_needed', 'pending', 'failed', 'unknown'].includes(state)) keys.push(`nginxdh.${state}.then`);
  }
  const vars = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join(',');
  for (const key of keys) {
    assert.equal(typeof en[key], 'string', `en ${key}`);
    assert.equal(typeof tr[key], 'string', `tr ${key}`);
    assert.equal(vars(tr[key]), vars(en[key]), key);
  }
  // The owner's own choices name the file they are about.
  for (const key of ['nginxdh.owner_default.reason', 'nginxdh.owner_file.reason']) assert.match(tr[key], /\{file\}/);
});
