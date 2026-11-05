// alpha.124 (Frankfurt, 2026-11-05): on a PowerDNS main + backup pair the
// "Sign the zone" button was offered, the server refused and the screen said
// "internal error". The button is now closed with its reason beside it, the
// refusal has its own words, and the card says the safe order plainly.
// İmzalama bu ikilide kapalı, nedeni düğmenin yanında; kart sade dille.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enDnsCertificatesScreens } from '../src/i18n/screens/dnsCertificates/en.ts';
import { trDnsCertificatesScreens } from '../src/i18n/screens/dnsCertificates/tr.ts';
import { enErrorsScreens } from '../src/i18n/screens/errors/en.ts';
import { trErrorsScreens } from '../src/i18n/screens/errors/tr.ts';

const source = readFileSync(new URL('../src/components/DomainDNSManager.tsx', import.meta.url), 'utf8');

test('the sign button is closed for the pair, with its reason beside it', () => {
    assert.match(source, /reason === 'DNSSEC_PAIR_UNSUPPORTED'/);
    assert.match(source, /disabled=\{[^}]*shown\.value\.pairUnsupported\}/);
    assert.match(source, /data-dnssec-closed=""\>\{t\('dnssec\.pairUnsupported'\)\}/);
    assert.match(trDnsCertificatesScreens['dnssec.pairUnsupported'], /henüz denenmedi; bu yüzden kapalı/);
    assert.match(enDnsCertificatesScreens['dnssec.pairUnsupported'], /not been tried yet/);
    for (const catalog of [enErrorsScreens, trErrorsScreens]) assert.ok(catalog['err.DNSSEC_PAIR_UNSUPPORTED']);
});

test('the card says the safe order without registry jargon', () => {
    for (const catalog of [enDnsCertificatesScreens, trDnsCertificatesScreens]) {
        for (const key of ['dnssec.offHint', 'dnssec.signed', 'dnssec.dsHint']) {
            assert.doesNotMatch(catalog[key], /delegasyon|delegation|\bDS\b|DNSKEY|RRSIG|registrar|kayıtçı/i, key);
        }
        assert.match(catalog['dnssec.offHint'], /1\..*2\..*3\./s);
    }
});
