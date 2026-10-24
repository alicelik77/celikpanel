// alpha.107 (D-038 §16): an audit row says what it was about: the component,
// from and to, what was confirmed, a version's security support.
// Denetim satırı neyle ilgili olduğunu söyler.
import assert from 'node:assert/strict';
import test from 'node:test';
import { compileSource } from './fixtures/shared-layer.mjs';
import { enLifecycleScreens } from '../src/i18n/screens/lifecycle/en.ts';
import { trLifecycleScreens } from '../src/i18n/screens/lifecycle/tr.ts';

const componentNameURL = compileSource('lib/componentName.ts', () => { throw new Error('componentName imports nothing at run time'); });
const { auditDetailParts } = await import(compileSource('lib/auditAction.ts', (specifier) => {
  if (specifier.endsWith('/componentName')) return componentNameURL;
  throw new Error('unexpected import ' + specifier);
}));
const labelIn = (words) => (key) => words['life.audit.key.' + key] ?? key;

test('a site PHP change says from and to, its support and the confirmation', () => {
  const parts = auditDetailParts({
    site: 'shop.example', component: 'php-fpm', from: '8.4', to: '8.1',
    support: 'unsupported', support_until: '2025-12-31', confirmed_unsupported: true,
  }, labelIn(enLifecycleScreens));
  assert.deepEqual(parts, ['php-fpm', '8.4 → 8.1', 'site: shop.example', 'security support: unsupported', 'fixes until: 2025-12-31', 'confirmed a version without security fixes']);
});

test('an install says the packages that were confirmed, in Turkish too; nothing empty is said', () => {
  const parts = auditDetailParts({ component: 'phpmyadmin', confirmed_installs: ['php8.4-common', 'php8.4-xml'], php_version: '', rolled_back: false, operation: 'service_install' }, labelIn(trLifecycleScreens));
  assert.deepEqual(parts, ['phpmyadmin', 'onaylanan paketler: php8.4-common, php8.4-xml']);
  assert.deepEqual(auditDetailParts(null, labelIn(enLifecycleScreens)), []);
  assert.deepEqual(auditDetailParts(['x'], labelIn(enLifecycleScreens)), []);
});
