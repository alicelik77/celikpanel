import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

// alpha.118, owner: what SSH needs is installed before SSH is turned on.
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const source = read('../src/components/DomainFileAccess.tsx');
const en = read('../src/i18n/screens/fileAccess/en.ts');
const tr = read('../src/i18n/screens/fileAccess/tr.ts');

function entries(text) {
    const out = new Map();
    for (const match of text.matchAll(/^\s*'([^']+)':\s*'((?:[^'\\]|\\.)*)',\s*$/gm)) out.set(match[1], match[2]);
    return out;
}

test('the Access view reads the missing tools and their packages from the server', () => {
    for (const field of ['jail_missing', 'jail_packages', 'jail_unknown']) {
        assert.match(source, new RegExp(`server\\.${field} \\?\\? \\[\\]`), field);
    }
});

test('the command-line option gives way to the notice while tools are missing; an account that has it keeps it', () => {
    assert.match(source, /const sshBlocked = view\.rights\.ssh && tools\.length > 0 && !current\?\.ssh;/);
    assert.match(source, /\{!sshBlocked && <label/);
    assert.match(source, /<SSHToolsNotice[\s\S]*?kept=\{current\.ssh\}/);
    // FTP and SFTP stay: the file-access switch does not depend on the tools.
    const enable = source.slice(source.indexOf("t('fileAccess.enable')") - 600, source.indexOf("t('fileAccess.enable')"));
    assert.doesNotMatch(enable, /sshBlocked|jailMissing/);
});

test('only the administrator gets the install button; others are told who acts', () => {
    const notice = source.slice(source.indexOf('function SSHToolsNotice'));
    assert.match(notice, /isAdmin && server\.jailPackages\.length > 0 \?/);
    assert.match(notice, /fileAccess\.sshTools\.install/);
    assert.match(notice, /!isAdmin && <p>\{t\('fileAccess\.sshTools\.customer'\)\}/);
    assert.match(notice, /fileAccess\.sshTools\.kept/);
});

test('installing sends exactly the packages shown, once, through the lost-answer guard', () => {
    const install = source.slice(source.indexOf('const installTools'), source.indexOf('return (', source.indexOf('const installTools')));
    assert.match(install, /answer\.send\('\/api\/v1\/service\/file-access-tools'/);
    assert.match(install, /packages: view\.server\?\.jailPackages \?\? \[\]/);
    assert.match(install, /await access\.retry\(\)/);
    assert.doesNotMatch(install, /fetch\(/);
    for (const code of ['FILE_ACCESS_TOOLS_CHANGED', 'FILE_ACCESS_TOOLS_FAILED', 'FILE_ACCESS_TOOLS_UNSUPPORTED']) assert.ok(install.includes(code), code);
});

test('a server refusal to turn SSH on names the tools', () => {
    assert.match(source, /error\.code === 'FILE_ACCESS_SSH_TOOLS_MISSING'[\s\S]{0,300}fileAccess\.sshTools\.missing', \{ names: \(error\.details \?\? \[\]\)\.join/);
});

test('every new sentence is in English and Turkish with the same placeholders', () => {
    const english = entries(en);
    const turkish = entries(tr);
    const keys = [...english.keys()].filter(k => k.startsWith('fileAccess.sshTools.') || k === 'err.FILE_ACCESS_REFUSED.jail_tools_missing');
    assert.equal(keys.length, 10);
    for (const key of keys) {
        assert.ok(turkish.has(key), key);
        const holes = s => [...s.matchAll(/\{(\w+)\}/g)].map(m => m[1]).sort().join();
        assert.equal(holes(turkish.get(key)), holes(english.get(key)), key);
    }
    assert.match(english.get('fileAccess.sshTools.customer'), /server administrator/);
    assert.match(turkish.get('fileAccess.sshTools.customer'), /sunucu yöneticisi/);
    assert.match(english.get('fileAccess.sshTools.missing'), /FTP and SFTP work/);
});
