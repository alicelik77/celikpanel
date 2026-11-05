// alpha.124: CelikPanel's own database held busy (alpha.123 lab). A component
// operation whose progress cannot be saved is shown waiting (who acts: nobody;
// past the bound: the owner, with the command), and the audit log says how
// many rows wait to be saved instead of leaving them out.
// Panelin kendi veritabanı meşgulken işlem bekliyor görünür; denetim günlüğü
// bekleyen kayıtları söyler.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { componentOperationPhaseKey } from '../src/lib/componentOperationGuidance.ts';
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';
import { enLifecycleScreens } from '../src/i18n/screens/lifecycle/en.ts';
import { trLifecycleScreens } from '../src/i18n/screens/lifecycle/tr.ts';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

test('a running operation that cannot save its progress is said as a wait', () => {
    assert.equal(componentOperationPhaseKey('panel_busy'), 'services.operation.phase.panel_busy');
    assert.equal(componentOperationPhaseKey('panel_stayed_busy'), 'services.operation.phase.panel_stayed_busy');
    for (const catalog of [enComponentsScreens, trComponentsScreens]) {
        const wait = catalog['services.operation.phase.panel_busy'];
        const stayed = catalog['services.operation.phase.panel_stayed_busy'];
        assert.ok(wait && stayed);
        assert.match(stayed, /sudo systemctl restart celikpanel-panel/);
    }
    assert.match(enComponentsScreens['services.operation.phase.panel_busy'], /Nobody needs to act/);
    assert.match(trComponentsScreens['services.operation.phase.panel_busy'], /Kimsenin bir şey yapması gerekmiyor/);
});

test('the audit log says the rows that wait to be saved', () => {
    for (const catalog of [enLifecycleScreens, trLifecycleScreens]) {
        assert.match(catalog['life.audit.pending.waiting'], /\{count\}/);
        assert.match(catalog['life.audit.pending.stalled'], /\{count\}.*sudo systemctl restart celikpanel-panel/s);
    }
    const page = source('components/AuditLogPage.tsx');
    assert.match(page, /audit_rows_waiting/);
    assert.match(page, /audit_rows_stalled/);
    assert.match(page, /role="status"/);
});
