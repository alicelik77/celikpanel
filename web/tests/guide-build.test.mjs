// The user guide's build (alpha.119, docs/guide/FORMAT.md): the parser takes
// only the documented subset, EN and TR must match, links must resolve, and
// the committed chunks and portal pages are exactly what the sources build.
//
// Kılavuz derlemesi: yalnız belgelenen alt küme, EN/TR eşliği, kırık bağlantı
// yok; işlenmiş dosyalar kaynağın ürettiğinin aynısı.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { buildGuide, parseChapter, staleFiles } from '../scripts/build-guide.mjs';

const repoRoot = fileURLToPath(new URL('../../', import.meta.url));

const front = (fields = {}) => {
    const all = { id: 'sample', title: 'Sample', roles: '[customer]', topics: '[]', keywords: '[]', ...fields };
    return `---\n${Object.entries(all).filter(([, v]) => v !== undefined).map(([k, v]) => `${k}: ${v}`).join('\n')}\n---\n`;
};
const errorsOf = (body, fields) => parseChapter(front(fields) + body, 'en/01-sample.md').errors;

test('the documented subset parses into blocks', () => {
    const { chapter, errors } = parseChapter(front() + [
        'Intro with **Bold**, `code`, [a link](guide:sample#two) and [out](https://celikpanel.net/systems.html).',
        '',
        '## Two {#two}',
        '',
        '- one',
        '  continued',
        '- two',
        '',
        '1. first',
        '2. second',
        '',
        '| A | B |',
        '|---|---|',
        '| `x \\| y` | b |',
        '',
        '```',
        'sudo ls <dir>',
        '```',
        '',
        ':::warning',
        'Careful: 5 * 3 and snake_case stay text.',
        ':::',
        '',
    ].join('\n'), 'en/01-sample.md');
    assert.deepEqual(errors, []);
    assert.deepEqual(chapter.blocks.map((b) => b[0]), ['p', 'h', 'ul', 'ol', 't', 'pre', 'warn']);
    assert.deepEqual(chapter.blocks[2][1], ['one continued', 'two']);
    assert.deepEqual(chapter.blocks[4][2][0][0], [['c', 'x \\| y']]);
    assert.equal(chapter.blocks[5][1], 'sudo ls <dir>');
    assert.deepEqual(chapter.blocks[6][1], [['p', 'Careful: 5 * 3 and snake_case stay text.']]);
    assert.deepEqual(chapter.links, [{ id: 'sample', anchor: 'two', line: 8 }]);
});

test('everything outside the subset is refused with its line', () => {
    const cases = {
        'raw HTML': 'Text <b>bold</b>.',
        'a comment': '<!-- hidden -->',
        'an entity': 'Fish &amp; chips.',
        'an image': '![logo](https://celikpanel.net/x.png)',
        'a block quote': '> quoted',
        'a top heading': '# Title',
        'a fourth-level heading': '#### Deep {#deep}',
        'a heading without anchor': '## No anchor',
        'a nested list': '- one\n  - two',
        'an unclosed bold': 'Some **bold text.',
        'an unclosed code span': 'Some `code.',
        'a horizontal rule': 'Text\n\n---\n\nMore',
        'an underlined heading': 'Title\n=====',
        'a reference link': '[x]: https://example.com',
        'a plain link target': '[x](http://example.com)',
        'a relative link target': '[x](other.md)',
        'an unclosed note': ':::note\nText',
        'an unknown block': ':::tip\nText\n:::',
        'a nested note': ':::note\n:::warning\nx\n:::\n:::',
        'a table without separator': '| a | b |\n| c | d |',
        'a table row of the wrong width': '| a | b |\n|---|---|\n| c |',
        'indented text': '    indented code',
        'a lone bracket': 'Choose [optional] here.',
    };
    for (const [name, body] of Object.entries(cases)) {
        const errors = errorsOf(`${body}\n`);
        assert.ok(errors.length > 0, `${name} was accepted`);
        assert.match(errors[0], /^en\/01-sample\.md:\d+: /, name);
    }
});

test('front matter is exact', () => {
    assert.ok(errorsOf('Text\n', { roles: '[customer, owner]' }).some((e) => e.includes('unknown role "owner"')));
    assert.ok(errorsOf('Text\n', { roles: '[]' }).some((e) => e.includes('at least one role')));
    assert.ok(errorsOf('Text\n', { keywords: undefined }).some((e) => e.includes('lacks "keywords"')));
    assert.ok(errorsOf('Text\n', { id: 'Bad_Id' }).some((e) => e.includes('lowercase')));
    assert.ok(errorsOf('Text\n', { title: 'A **bold** title' }).some((e) => e.includes('plain text')));
    assert.ok(errorsOf('Text\n', { topics: '[domain.files#Bad]' }).some((e) => e.includes('topic')));
    assert.ok(parseChapter(`${front()}Text\n`.replace('---\n', '---\nextra: x\n'), 'x.md').errors.some((e) => e.includes('unknown front matter field "extra"')));
    assert.ok(parseChapter(`${front()}Text\n`.replace(/\n/g, '\r\n'), 'x.md').errors.some((e) => e.includes('LF')));
});

// A throwaway repository: docs/guide/{en,tr} and an optional screen source.
function fixture(files) {
    const root = mkdtempSync(join(tmpdir(), 'cp-guide-'));
    for (const [path, text] of Object.entries(files)) {
        mkdirSync(join(root, path, '..'), { recursive: true });
        writeFileSync(join(root, path), text);
    }
    return root;
}
const chapter = (id, fields, body) => front({ id, title: id, ...fields }) + body;
const pair = (name, en, tr = en) => ({ [`docs/guide/en/${name}`]: en, [`docs/guide/tr/${name}`]: tr });

test('EN and TR must have the same chapters, roles, topics, anchors and links', () => {
    const base = chapter('start', { roles: '[admin, reseller, customer]', topics: '[dashboard]' }, 'Hi.\n\n## One {#one}\n\nSee [two](guide:files#keys).\n');
    const files = chapter('files', { topics: '[domain.files#keys]' }, 'Files.\n\n## Keys {#keys}\n\nText.\n');
    const cases = [
        [{ ...pair('01-start.md', base), 'docs/guide/en/02-files.md': files }, /has no tr\/02-files\.md/],
        [{ ...pair('01-start.md', base, base.replace('[admin, reseller, customer]', '[customer]')), ...pair('02-files.md', files) }, /roles differ/],
        [{ ...pair('01-start.md', base, base.replace('[dashboard]', '[]')), ...pair('02-files.md', files) }, /topics differ/],
        [{ ...pair('01-start.md', base, base.replace('{#one}', '{#uno}')), ...pair('02-files.md', files) }, /headings differ/],
        [{ ...pair('01-start.md', base, base.replace('guide:files#keys', 'guide:files')), ...pair('02-files.md', files) }, /guide links differ/],
    ];
    for (const [tree, pattern] of cases) {
        const root = fixture(tree);
        try {
            const { errors } = buildGuide({ repoRoot: root });
            assert.ok(errors.some((e) => pattern.test(e)), `${pattern}: ${errors.join(' | ')}`);
        } finally {
            rmSync(root, { recursive: true, force: true });
        }
    }
});

test('broken links, doubled topics and unclaimed screen topics fail the build', () => {
    const cases = [
        [pair('01-a.md', chapter('a', {}, 'See [b](guide:missing).\n')), /broken link: no chapter "missing"/],
        [pair('01-a.md', chapter('a', {}, 'See [b](#nowhere).\n')), /broken link: chapter "a" has no anchor "#nowhere"/],
        [pair('01-a.md', chapter('a', { topics: '[x#nowhere]' }, 'Text.\n')), /names an anchor this chapter does not have/],
        [{ ...pair('01-a.md', chapter('a', { topics: '[x]' }, 'A.\n')), ...pair('02-b.md', chapter('b', { topics: '[x]' }, 'B.\n')) }, /topic "x" is also claimed/],
        [{ ...pair('01-a.md', chapter('a', {}, 'A.\n')), 'docs/guide/en/1-b.md': 'x' }, /named NN-slug\.md/],
        [{ ...pair('01-a.md', chapter('b', {}, 'A.\n')) }, /must equal the file's slug/],
        [{ ...pair('01-a.md', chapter('a', { topics: '[domain.files]' }, 'A.\n')), 'web/src/components/X.tsx': '<GuideLink topic="domain.nope" />' }, /topic "domain\.nope" is claimed by no en chapter/],
    ];
    for (const [tree, pattern] of cases) {
        const root = fixture(tree);
        try {
            const { errors, files } = buildGuide({ repoRoot: root });
            assert.ok(errors.some((e) => pattern.test(e)), `${pattern}: ${errors.join(' | ')}`);
            assert.equal(files.size, 0, 'nothing is generated from a guide with problems');
        } finally {
            rmSync(root, { recursive: true, force: true });
        }
    }
});

test('a chunk holds its role\'s chapters; a link to another role\'s chapter keeps only its words', () => {
    const root = fixture({
        ...pair('01-start.md', chapter('start', { roles: '[admin, reseller, customer]' }, 'Read [the admin part](guide:admin-only) now.\n')),
        ...pair('02-admin-only.md', chapter('admin-only', { roles: '[admin]' }, 'Admin.\n')),
    });
    try {
        const { files, errors } = buildGuide({ repoRoot: root });
        assert.deepEqual(errors, []);
        const customer = files.get('web/src/help/guide/gen/customer.en.ts');
        const admin = files.get('web/src/help/guide/gen/admin.en.ts');
        assert.doesNotMatch(customer, /admin-only/);
        assert.match(customer, /"Read the admin part now\."/);
        assert.match(admin, /\["g","admin-only","",/);
        // The portal has every chapter, so the link stays a link there.
        assert.match(files.get('download-portal/guide/en/start.html'), /<a href="admin-only\.html">the admin part<\/a>/);
    } finally {
        rmSync(root, { recursive: true, force: true });
    }
});

test('the committed chunks and portal pages are exactly what docs/guide builds', () => {
    const { files, errors } = buildGuide({ repoRoot });
    assert.deepEqual(errors, []);
    assert.deepEqual(staleFiles(repoRoot, files), [], 'run `npm run guide` in web/ and commit the result');
    for (const role of ['admin', 'reseller', 'customer']) {
        for (const locale of ['en', 'tr']) assert.ok(files.has(`web/src/help/guide/gen/${role}.${locale}.ts`), `${role}.${locale}`);
    }
});

test('the portal pages are static, bilingual and listed in their manifest', () => {
    const dir = join(repoRoot, 'download-portal', 'guide');
    const manifest = readFileSync(join(dir, 'manifest.sha256'), 'utf8').trim().split('\n').map((line) => line.split('  '));
    const names = manifest.map(([, name]) => name);
    for (const required of ['index.html', 'en/index.html', 'tr/index.html', 'en/getting-started.html', 'tr/getting-started.html', 'en/files-access.html', 'tr/reading-states.html']) {
        assert.ok(names.includes(required), required);
    }
    for (const [sum, name] of manifest) {
        const text = readFileSync(join(dir, name));
        assert.equal(createHash('sha256').update(text).digest('hex'), sum, name);
        const html = text.toString('utf8');
        // The portal's CSP allows no inline script or style.
        assert.doesNotMatch(html, /<script|style=|onclick=/i, name);
        assert.match(html, /<link rel="stylesheet" href="\/assets\/guide\.css"\/>/, name);
    }
    const en = readFileSync(join(dir, 'en', 'files-access.html'), 'utf8');
    assert.match(en, /<html lang="en">/);
    assert.match(en, /href="\/guide\/tr\/files-access\.html" lang="tr"/);
    assert.match(en, /<h2 id="ssh-tools">/);
    assert.match(readFileSync(join(dir, 'tr', 'files-access.html'), 'utf8'), /<html lang="tr">[\s\S]*Araçları kur/);
});
