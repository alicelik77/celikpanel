import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const source = read('../src/components/DomainFileAccess.tsx');
const en = read('../src/i18n/screens/fileAccess/en.ts');
const tr = read('../src/i18n/screens/fileAccess/tr.ts');

test('saving access shows its warnings on the page, never as pop-ups over it', () => {
    assert.doesNotMatch(source, /showToast\(\s*'warning'/);
    assert.match(source, /setLastWarnings\(changeWarnings\(answered, t, isAdmin\)\)/);
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
    // alpha.127: the provider acts for everyone else; the administrator is told what to do.
    assert.match(en, /jail_programs_missing_named': '[^']*Your provider \(the server’s administrator\)[^']*ask them/);
    assert.match(tr, /jail_programs_missing_named': '[^']*sağlayıcınız \(sunucu yöneticisi\)[^']*sorun/);
    assert.match(en, /jail_programs_missing_namedAdmin': '[^']*Install them as root[^']*save access here again/);
    assert.match(tr, /jail_programs_missing_namedAdmin': '[^']*root olarak[^']*yeniden kaydedin/);
    assert.match(source, /isAdmin \? 'fileAccess\.warn\.jail_programs_missing_namedAdmin' : 'fileAccess\.warn\.jail_programs_missing_named'/);
});
