// alpha.145 lab, D1 (23 November 2026): a longer change must never stay "in
// progress" for ever. While the panel records the end of a change after it
// lost contact with its agent, every screen that follows the change says what
// is true and that it ends by itself; when it cannot be recorded, the screen,
// the refusal of another change and Problems now name the server owner's
// step. Words in English and Turkish.
// Kaydedilen ya da kaydedilemeyen değişikliğin ekranları ne olduğunu ve kimin
// ne yapacağını söyler.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { componentOperationPhaseKey, removalOperationPhase, settlePhaseKey } from '../src/lib/componentOperationGuidance.ts';
import { apiErrorText } from '../src/lib/apiError.ts';
import { enOpstepsScreens } from '../src/i18n/screens/opsteps/en.ts';
import { trOpstepsScreens } from '../src/i18n/screens/opsteps/tr.ts';
import { enRefusalsScreens } from '../src/i18n/screens/refusals/en.ts';
import { trRefusalsScreens } from '../src/i18n/screens/refusals/tr.ts';
import { enMonitoringScreens } from '../src/i18n/screens/monitoring/en.ts';
import { trMonitoringScreens } from '../src/i18n/screens/monitoring/tr.ts';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const RESTART = /sudo systemctl restart celikpanel-panel/;

test('a row being settled or unsettled is said, on every screen that follows it', () => {
    for (const phase of ['settling_contact', 'settling_recording', 'unsettled']) {
        const key = 'services.operation.phase.' + phase;
        assert.equal(settlePhaseKey(phase), key);
        assert.equal(componentOperationPhaseKey(phase), key);
        assert.equal(removalOperationPhase(phase).key, key);
        for (const catalog of [enOpstepsScreens, trOpstepsScreens]) assert.ok(catalog[key], key);
    }
    assert.equal(settlePhaseKey('updating'), null);
    assert.equal(settlePhaseKey(undefined), null);
    // While it settles nothing is needed from the reader and it ends by
    // itself; when it cannot, the server owner checks the agent first, then
    // restarts the panel's web process.
    assert.match(enOpstepsScreens['services.operation.phase.settling_contact'], /Nothing is needed from you: this ends by itself/);
    assert.match(trOpstepsScreens['services.operation.phase.settling_contact'], /Sizin bir şey yapmanız gerekmiyor: bu kendiliğinden biter/);
    for (const catalog of [enOpstepsScreens, trOpstepsScreens]) {
        const unsettled = catalog['services.operation.phase.unsettled'];
        assert.ok(unsettled.indexOf('systemctl status celikpanel-agent') >= 0 && unsettled.indexOf('systemctl status celikpanel-agent') < unsettled.search(RESTART));
    }
    // D1 review, finding 1: true per kind. A webmail update or address is only
    // read; an install kind is run once more from its start and relies on its
    // steps being safe to repeat. Never "starts nothing again", "continues
    // where it stopped" or "what is already done is not changed again".
    const settleTexts = [
        ...['settling_contact', 'settling_recording', 'unsettled'].flatMap((phase) => [enOpstepsScreens, trOpstepsScreens].map((c) => c['services.operation.phase.' + phase])),
        enMonitoringScreens['pnow.service_operation_unsettled.body'], trMonitoringScreens['pnow.service_operation_unsettled.body'],
    ];
    for (const text of settleTexts) {
        assert.match(text, /only read|yalnız okunur/);
        assert.match(text, /run once more from its start|baştan bir kez daha çalıştırılır/);
        assert.match(text, /safe to repeat|yinelenmesinin zararsız/);
        assert.doesNotMatch(text, /starts nothing again|nothing is started again|where it stopped|not changed again|yeniden başlatmıyor|yeniden başlatılmıyor|kaldığı yerden|yeniden değiştirilmez/);
    }
    // Finding 9: the end of the change could not be recorded (not always a
    // lost contact), and what is refused is named.
    assert.doesNotMatch(enOpstepsScreens['services.operation.phase.settling_contact'], /lost contact/);
    assert.match(enMonitoringScreens['pnow.service_operation_unsettled.body'], /other component and setup changes/);
    assert.match(source('lib/webmailAddressFlow.ts'), /'err\.service_operation_busy\.unsettled' \? 'warn'/);
    // The webmail update row and the webmail address card say the same words.
    assert.match(source('components/RoundcubeUpdateFlow.tsx'), /settlePhaseKey\(shown\.phase\)/);
    assert.match(source('components/WebmailAddressCard.tsx'), /settlePhaseKey\(shown\.phase\)/);
});

test('another change refused meanwhile says who acts and when', () => {
    const t = (key, vars) => {
        const words = { ...enRefusalsScreens, 'err.service_operation_busy': 'Another package operation is already in progress. Wait for it to finish.' };
        return words[key] ?? key;
    };
    const settling = apiErrorText({ message: 'x', code: 'service_operation_busy', reason: 'settling' }, t);
    assert.match(settling, /Nobody needs to act: this ends by itself/);
    const unsettled = apiErrorText({ message: 'x', code: 'service_operation_busy', reason: 'unsettled' }, t);
    assert.match(unsettled, RESTART);
    assert.doesNotMatch(unsettled, /Wait for it to finish/);
    for (const catalog of [enRefusalsScreens, trRefusalsScreens]) {
        assert.ok(catalog['err.service_operation_busy.settling']);
        assert.match(catalog['err.service_operation_busy.unsettled'], RESTART);
    }
    // The two webmail flows read the reason instead of their own "press again
    // when it ends".
    assert.match(source('components/RoundcubeUpdateFlow.tsx'), /'err\.service_operation_busy\.unsettled'/);
    assert.match(source('lib/webmailAddressFlow.ts'), /'err\.service_operation_busy\.unsettled'/);
});

test('Problems now has one line for a change that could not be recorded', () => {
    // A kind the list knows (others are left out), worded by the general
    // pnow.<kind>.title / .body keys.
    const problems = source('components/ProblemsNow.tsx');
    const kinds = /const KINDS[^=]*=\s*\[([\s\S]*?)\];/.exec(problems)?.[1] ?? '';
    assert.match(kinds, /'service_operation_unsettled'/);
    assert.doesNotMatch(problems, /service_operation_unsettled' &&/);
    for (const catalog of [enMonitoringScreens, trMonitoringScreens]) {
        assert.ok(catalog['pnow.service_operation_unsettled.title']);
        assert.ok(catalog['pnow.service_operation_unsettled.action']);
        assert.match(catalog['pnow.service_operation_unsettled.body'], RESTART);
        assert.match(catalog['pnow.service_operation_unsettled.body'], /\{name\}/);
    }
});
