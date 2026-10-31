// The deletion check between two servers joined by code is set up by the
// backup's setup over the setup link (cmd/panel/server_setup_peer_inspection.go).
// Every wait the Panel can return and every outcome its peer view can carry
// must reach the owner as a sentence in both languages, with the same values,
// and the Turkish sentences carry no internal identifiers.
//
// Iki sunucu arasindaki silme denetiminin her beklemesi ve sonucu iki dilde
// ayni degerlerle soylenir; Turkce metinde ic kod adi yoktur.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const goSource = read('../../cmd/panel/server_setup_peer_inspection.go');
const steps = read('../src/components/ServerSetupWizardSteps.tsx');
const setup = read('../src/components/ServerSetup.tsx');
const guidance = read('../src/lib/serverSetupGuidance.ts');

const waitCodes = [...new Set([...goSource.matchAll(/"(server_setup_peer_inspection_[a-z_]+)"/g)].map((m) => m[1]))]
    // not_main is the main's machine refusal over the link, never a wait.
    .filter((code) => code !== 'server_setup_peer_inspection_' && code !== 'server_setup_peer_inspection_not_main');
const viewCodes = [...(/setupPeerInspectionCodes = \[([^\]]+)\]/.exec(steps)?.[1] ?? '').matchAll(/'([a-z_]+)'/g)].map((m) => m[1]);
const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

test('every deletion-check wait has an English and a Turkish sentence and is a link wait', () => {
    assert.ok(waitCodes.length >= 8, `waits found: ${waitCodes}`);
    for (const code of waitCodes) {
        const key = `setup.v5.wait.${code}`;
        assert.ok(enScreens[key], `${key} (en)`);
        assert.ok(trScreens[key], `${key} (tr)`);
        assert.ok(setup.includes(`${code}: '${key}'`), `${code} mapped in ServerSetup.tsx`);
        assert.ok(guidance.includes(`'${code}'`), `${code} in the setup link waits`);
    }
});

test('every outcome the peer view can carry is said in both languages with the same values', () => {
    for (const code of ['ready', 'existing', 'owner_conflict', 'revoked', 'inspector_unavailable', 'unsupported', 'link_ended', 'working', 'ssh_port', 'ssh_unreachable']) {
        assert.ok(viewCodes.includes(code), `${code} accepted by the view decoder`);
    }
    for (const code of [...viewCodes, 'title', 'side.main', 'side.backup']) {
        const key = `setup.v5.inspection.${code}`;
        assert.ok(enScreens[key] && trScreens[key], key);
        assert.deepEqual(placeholders(trScreens[key]), placeholders(enScreens[key]), `${key} placeholders`);
    }
    // Outcomes the Panel writes are a subset of what the screen accepts.
    for (const [, outcome] of goSource.matchAll(/peerInspectionOutcome\w+\s+= "([a-z_]+)"/g)) {
        if (outcome !== 'not_applicable') assert.ok(viewCodes.includes(outcome), `${outcome} has a view code`);
    }
});

test('the Turkish deletion-check sentences name no internal identifiers', () => {
    const keys = Object.keys(trScreens).filter((key) => key.startsWith('setup.v5.inspection.') || key.startsWith('setup.v5.wait.server_setup_peer_inspection_'));
    assert.ok(keys.length > 20);
    for (const key of keys) {
        const text = trScreens[key];
        assert.doesNotMatch(text, /server_setup|peer_inspection|\bD-\d|\bP\d\b|\bR-\d|owner_conflict|ssh_port/, key);
    }
});
