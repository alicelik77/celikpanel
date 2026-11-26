import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import { refusalActor } from '../src/lib/apiError.ts';
import { enProblemsScreens } from '../src/i18n/screens/problems/en.ts';
import { trProblemsScreens } from '../src/i18n/screens/problems/tr.ts';
import { enDataTools as enData } from '../src/i18n/dataTools/en.ts';
import { trDataTools as trData } from '../src/i18n/dataTools/tr.ts';

// alpha.149 lab W1: the progress window said "Waiting for someone to act" over
// a reason that names the server's administrator. Where the reason names the
// actor, the window's line names that actor, in English and in Turkish.
// Neden kimin işlem yapacağını adlandırıyorsa pencerenin satırı da adlandırır.
const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('a database account refusal that names the administrator names them in the heading', () => {
    const naming = ['privileged', 'member', 'unreadable', 'unsupported_engine', 'owns_objects'];
    for (const reason of naming) {
        assert.equal(refusalActor({ message: '', code: 'DATABASE_ACCOUNT_REFUSED', reason }), 'administrator', reason);
        // The reason's own words name the administrator in both languages.
        assert.match(enData[`err.DATABASE_ACCOUNT_REFUSED.${reason}`], /server's administrator/, reason);
        assert.match(trData[`err.DATABASE_ACCOUNT_REFUSED.${reason}`], /[Ss]unucunun yöneticisi/, reason);
    }
    // The general sentence (an unknown reason) names them too.
    assert.equal(refusalActor({ message: '', code: 'DATABASE_ACCOUNT_REFUSED' }), 'administrator');
    assert.match(enData['err.DATABASE_ACCOUNT_REFUSED'], /server's administrator/);
    // Sentences that name no one keep the general heading.
    for (const reason of ['absent', 'system_database']) {
        assert.equal(refusalActor({ message: '', code: 'DATABASE_ACCOUNT_REFUSED', reason }), undefined, reason);
        assert.doesNotMatch(enData[`err.DATABASE_ACCOUNT_REFUSED.${reason}`], /administrator/, reason);
        assert.doesNotMatch(trData[`err.DATABASE_ACCOUNT_REFUSED.${reason}`], /yönetici/, reason);
    }
    assert.equal(refusalActor({ message: 'x', code: 'OTHER' }), undefined);
    assert.equal(refusalActor(null), undefined);
});

test('the heading names the administrator in English and Turkish; the general one stays', () => {
    assert.equal(enProblemsScreens['operation.blocked.administrator'], "Waiting for the server's administrator to act");
    assert.equal(trProblemsScreens['operation.blocked.administrator'], 'Sunucunun yöneticisinin bir şey yapması bekleniyor');
    assert.equal(enProblemsScreens['operation.blocked'], 'Waiting for someone to act');
    const progress = read('../src/components/OperationProgress.tsx');
    assert.match(progress, /end\.state === 'blocked' && end\.actor === 'administrator' \? 'operation\.blocked\.administrator' : END_TITLE\[end\.state\]/);
    // Every database account flow that can receive the refusal passes its actor.
    const users = read('../src/components/DatabaseUsers.tsx');
    assert.equal((users.match(/actor: refusalActor\(problem\)/g) ?? []).length, 2, 'add and remove');
    assert.match(read('../src/components/DatabaseUserPassword.tsx'), /actor: refusalActor\(problem\)/);
});
