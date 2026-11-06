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
    assert.match(install, /const packages = view\.server\?\.jailPackages \?\? \[\];/);
    assert.match(install, /body: JSON\.stringify\(\{ packages \}\)/);
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
    assert.equal(keys.length, 16);
    for (const key of keys) {
        assert.ok(turkish.has(key), key);
        const holes = s => [...s.matchAll(/\{(\w+)\}/g)].map(m => m[1]).sort().join();
        assert.equal(holes(turkish.get(key)), holes(english.get(key)), key);
    }
    assert.match(english.get('fileAccess.sshTools.customer'), /server’s administrator/);
    assert.match(turkish.get('fileAccess.sshTools.customer'), /sunucu yöneticisi/);
    assert.match(english.get('fileAccess.sshTools.missing'), /FTP and SFTP work/);
});

// alpha.127 (owner, 6 Nov): installing runs on the server, so it is asked in
// the panel's own window and shown there, locked, step by step; the option
// appears without a reload because the access is read again before "done".
test('installing the tools is confirmed in the panel window and shown step by step', () => {
    const install = source.slice(source.indexOf('const installTools'), source.indexOf('return (', source.indexOf('const installTools')));
    assert.match(source, /import \{ confirmAction, type OperationEnd, type OperationStep \} from '\.\.\/lib\/confirmAction';/);
    assert.match(install, /await confirmAction\(\{/);
    assert.match(install, /progressTitle: t\('fileAccess\.sshTools\.progressTitle'/);
    assert.match(install, /report\.steps\(\[installing, checking\]\)/);
    // A lost answer is an unknown result, never a second install.
    assert.match(install, /if \(!res\) return \{ state: 'unknown', reason: t\('common\.resultUnknown'\) \};/);
    // Done only after the access was read again.
    assert.ok(install.lastIndexOf('await access.retry()') < install.indexOf("return { state: 'done'"));
    assert.doesNotMatch(install, /window\.(confirm|alert)/);
});

test('the administrator and everyone else each get their own sentence', () => {
    const notice = source.slice(source.indexOf('function SSHToolsNotice'));
    assert.match(notice, /isAdmin \? 'fileAccess\.sshTools\.unknownAdmin' : 'fileAccess\.sshTools\.unknown'/);
    const blockers = source.slice(source.indexOf('function AccessBlockers'), source.indexOf('function SSHToolsNotice'));
    assert.match(blockers, /isAdmin \? 'fileAccess\.block\.noSSHDAdmin' : 'fileAccess\.block\.noSSHD'/);
    assert.match(blockers, /isAdmin \? \{ tone: 'info', key: 'fileAccess\.block\.ftpMissingAdmin', install: 'vsftpd' \} : \{ tone: 'info', key: 'fileAccess\.block\.ftpMissing' \}/);
    const english = entries(en);
    const turkish = entries(tr);
    // Everyone else is told who acts and what is missing; the administrator is never told to ask.
    for (const key of ['fileAccess.sshTools.customer', 'fileAccess.block.ftpMissing', 'fileAccess.block.noSSHD']) {
        assert.match(english.get(key), /Your provider \(the server’s administrator\)/, key);
        assert.match(turkish.get(key), /sağlayıcınız \(sunucu yöneticisi\)/i, key);
    }
    for (const key of ['fileAccess.sshTools.admin', 'fileAccess.sshTools.unknownAdmin', 'fileAccess.block.ftpMissingAdmin', 'fileAccess.block.noSSHDAdmin']) {
        assert.doesNotMatch(english.get(key), /ask|provider/i, key);
        assert.doesNotMatch(turkish.get(key), /sorun|sağlayıcı/i, key);
    }
});
