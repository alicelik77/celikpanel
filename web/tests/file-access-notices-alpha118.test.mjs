import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const source = read('../src/components/DomainFileAccess.tsx');
const en = read('../src/i18n/screens/fileAccess/en.ts');
const tr = read('../src/i18n/screens/fileAccess/tr.ts');

test('saving access shows its warnings on the page, never as pop-ups over it', () => {
    assert.doesNotMatch(source, /showToast\(\s*'warning'/);
    assert.match(source, /setLastWarnings\(changeWarnings\(answered, t\)\)/);
    const shownLine = source.split('\n').find(l => l.startsWith('const SHOWN_ON_PAGE')) ?? '';
    for (const shown of ['ftp_not_installed', 'fail2ban_missing', 'ssh_password_auth_off', 'ftp_owner_config', 'ftp_self_signed']) {
        assert.ok(shownLine.includes(`'${shown}'`), shown);
    }
});

test('missing command-line tools are named with who acts and what next, in EN and TR', () => {
    for (const lang of [en, tr]) {
        const line = lang.split('\n').find(l => l.includes("'fileAccess.warn.jail_programs_missing_named'"));
        assert.ok(line, 'named key present');
        assert.match(line, /\{names\}/);
    }
    assert.match(en, /jail_programs_missing_named': '[^']*server administrator[^']*save access here again/);
    assert.match(tr, /jail_programs_missing_named': '[^']*Sunucu yöneticisi[^']*yeniden kaydedebilir/);
});
