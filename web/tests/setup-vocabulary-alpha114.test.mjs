import assert from 'node:assert/strict';
import test from 'node:test';
import { enScreens, trScreens } from './locale-catalogue.mjs';
import { enSetupDNS } from '../src/i18n/setupDNS/en.ts';
import { trSetupDNS } from '../src/i18n/setupDNS/tr.ts';

// alpha.114 (owner decision): the two servers of a setup are named by role,
// never by an order word that can be read two ways. "The first server" is the
// one where setup started and the main domain was chosen; "this server" and
// "the joined server" is the one that joined with a code. "Main DNS" and
// "backup DNS" are used only where the DNS role matters. "Second server",
// "peer server", "other server" (and their Turkish forms) are banned in the
// setup and DNS overview copy.
// Iki sunucu rolle adlandirilir: ilk sunucu, bu sunucu, katilan sunucu.
// Iki turlu okunabilen sira sozcukleri (ikinci/es/diger/obur sunucu) yasak.

const scopes = ['setup.', 'dnsov.', 'services.mailProfiles.secondary.'];
// Whole words only: "another server" and "other servers" are not the pair's other member.
const bannedEn = [/\bsecond server\b/, /\bpeer server\b/, /\bother server\b/];
const bannedTr = [/(?<!\p{L})ikinci sunucu/u, /(?<!\p{L})eş sunucu/u, /(?<!\p{L})diğer sunucu/u, /(?<!\p{L})öbür sunucu/u, /(?<!\p{L})öteki sunucu/u];

// The name of the navigation page ("DNS and peer servers", nav.dnsPeers) is
// a fixed label that the menu layout contract pins; it is a page name, not a
// description of a server, so it is removed before the check.
// Menu sayfasinin adi sabit bir etikettir; sunucu adi degil.
const pageName = /DNS and peer servers|DNS ve eş sunucu\p{L}*/giu;

const entries = (screens, extra) => Object.entries({ ...screens, ...extra })
    .filter(([key]) => scopes.some(scope => key.startsWith(scope)));

function offenders(items, banned) {
    const found = [];
    for (const [key, value] of items) {
        const text = String(value).replace(pageName, '').toLowerCase();
        for (const pattern of banned) if (pattern.test(text)) found.push(`${key}: ${pattern}`);
    }
    return found;
}

test('the setup copy has the keys it is checked on', () => {
    assert.ok(entries(enScreens, enSetupDNS).length > 300);
    assert.ok(entries(trScreens, trSetupDNS).length > 300);
});

test('English setup copy never names a server by an order word', () => {
    assert.deepEqual(offenders(entries(enScreens, enSetupDNS), bannedEn), []);
});

test('Turkish setup copy never names a server by an order word', () => {
    assert.deepEqual(offenders(entries(trScreens, trSetupDNS), bannedTr), []);
});

test('the DNS choice in the first server wizard no longer calls the main DNS server "second"', () => {
    for (const key of ['setup.v5.dns.main.thisHelp', 'setup.v5.dns.main.second', 'setup.v5.dns.main.secondHelp']) {
        assert.ok(!/second/i.test(enScreens[key]), `${key}: ${enScreens[key]}`);
        assert.ok(!/ikinci/i.test(trScreens[key]), `${key}: ${trScreens[key]}`);
    }
    assert.match(enScreens['setup.v5.dns.main.second'], /will join/i);
    assert.match(trScreens['setup.v5.dns.main.second'], /katılacak/i);
});

test('the stop-waiting button and the sentences that name it use the same words', () => {
    assert.equal(enScreens['setup.v5.peerSetup.stop'], 'Stop waiting for the joined server');
    for (const key of ['setup.v5.wait.server_setup_peer_ready_refused', 'setup.v5.wait.server_setup_peer_inspection_ssh_port']) {
        assert.ok(enScreens[key].includes(`"${enScreens['setup.v5.peerSetup.stop']}"`), key);
        assert.ok(trScreens[key].includes(`"${trScreens['setup.v5.peerSetup.stop']}"`), key);
    }
});

test('every placeholder of the Turkish copy is the English one', () => {
    const names = text => [...String(text).matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort().join(',');
    for (const [key, value] of entries(enScreens, enSetupDNS)) {
        const other = { ...trScreens, ...trSetupDNS }[key];
        if (other !== undefined) assert.equal(names(other), names(value), key);
    }
});
