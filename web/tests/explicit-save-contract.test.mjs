import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

// One way of saving everywhere (alpha.132; owner, 8 Nov 2026): a setting is
// never saved by clicking it. A checkbox, a switch, a radio button or a
// select changes the screen's draft; the server is written only by Save in
// the shared save bar (components/kit/save). This test reads every component
// and fails when such a control sends something to the server straight from
// its onChange or onClick, directly or through a function of the same file
// (followed three calls deep).
//
// The controls that still did so when the rule was written are listed below
// with where they were. The list may only shrink: a control that no longer
// sends is a stale entry and fails the test too, so whoever converts one
// removes its line. The lanes SAVE-DOMAIN and SAVE-SERVER of alpha.132 empty it.
//
// Hiçbir ayar tıklanarak kaydedilmez: onay kutusu, anahtar, seçim düğmesi ya da
// açılır liste yalnız taslağı değiştirir. Bu test, onChange/onClick'ten
// doğrudan sunucuya yazan denetimi yakalar. Aşağıdaki liste yalnız küçülebilir.

const components = fileURLToPath(new URL('../src/components/', import.meta.url));

// file (under src/components) → the handler expressions that still save on
// change there, each with how many controls use it.
const stillSavingOnChange = {
    // :365 offered item; `change` :324
    'PlanRights.tsx': { 'change(item, e.target.checked)': 1 },
    // :71 offered application
    'OfferedAppsSection.tsx': { 'change(app, e.target.checked)': 1 },
    // :94 offered version line
    'OfferedVersionsCard.tsx': { 'change(line, event.target.checked)': 1 },
    // :134 PHP extension on/off (`toggleExt` :61)
    'PHPManagement.tsx': { 'toggleExt(ext)': 1 },
    // :973 automatic update check (`saveAutoCheck` :555)
    'PanelUpdateCard.tsx': { 'saveAutoCheck(event.currentTarget.checked)': 1 },
};

const files = (dir) => readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? files(path) : name.endsWith('.tsx') ? [path] : [];
});

// The text from `start` (an opening `{`, `(` or `<`) to its partner,
// skipping strings and template literals.
function balanced(source, start) {
    const open = source[start];
    const close = { '{': '}', '(': ')', '<': '>' }[open];
    let depth = 0;
    for (let at = start; at < source.length; at++) {
        const char = source[at];
        if (char === '"' || char === "'" || char === '`') {
            const quote = char;
            for (at++; at < source.length && source[at] !== quote; at++) if (source[at] === '\\') at++;
            continue;
        }
        if (open === '<') {
            if (char === '{') { at = balanced(source, at).end; continue; }
            if (char === '>') return { text: source.slice(start, at + 1), end: at };
            continue;
        }
        if (char === open) depth++;
        else if (char === close && --depth === 0) return { text: source.slice(start, at + 1), end: at };
    }
    return { text: source.slice(start), end: source.length };
}

// A settings control: a checkbox, a radio button, a select, a switch, or one
// of the product's toggle components.
const controlTag = /<(input|select|[A-Z]\w*Toggle)\b/g;
const isSettingControl = (tag, name) => name === 'select' || /Toggle$/.test(name) || /\btype="(checkbox|radio)"/.test(tag) || /\brole="switch"/.test(tag);

// What writes to the server.
const network = /\bfetch\(|\bapi\.\w+\(|\bsend[A-Z]\w*\(|\.send\(|\b(post|put|patch|del)\(|'(POST|PUT|PATCH|DELETE)'/;

function localBody(source, name) {
    const definition = new RegExp(`(?:const|let)\\s+${name}\\s*=\\s*(?:useCallback\\(\\s*)?(?:async\\s*)?(?:\\([^)]*\\)|\\w+)(?:\\s*:\\s*[^=]+?)?\\s*=>\\s*|(?:async\\s+)?function\\s+${name}\\s*\\([^)]*\\)[^{]*`);
    const found = definition.exec(source);
    if (!found) return null;
    const after = found.index + found[0].length;
    return source[after] === '{' || source[after] === '(' ? balanced(source, after).text : source.slice(after, source.indexOf('\n', after));
}

function sends(source, body, depth = 0) {
    if (network.test(body)) return true;
    if (depth >= 3) return false;
    for (const [, name] of body.matchAll(/\b([a-zA-Z_]\w*)\s*\(/g)) {
        const inner = localBody(source, name);
        if (inner && inner !== body && sends(source, inner, depth + 1)) return true;
    }
    return false;
}

// The handler of a control, as written: an inline function's body, or a name.
function handlerOf(tag) {
    const found = /\bon(Change|Click)=\{/.exec(tag);
    if (!found) return null;
    const expression = balanced(tag, found.index + found[0].length - 1).text.slice(1, -1).trim();
    const arrow = /^(?:async\s*)?(?:\([^)]*\)|\w+)\s*=>\s*/.exec(expression);
    if (!arrow) return { key: expression, body: expression, name: expression };
    const body = expression.slice(arrow[0].length).replace(/^void\s+/, '');
    return { key: body.split('\n')[0].trim().replace(/^\{\s*$/, '') || body.split('\n')[1].trim(), body };
}

function offenders() {
    const found = {};
    for (const path of files(components)) {
        const file = relative(components, path).split('\\').join('/');
        if (file.startsWith('kit/save/')) continue;
        const source = readFileSync(path, 'utf8');
        for (const match of source.matchAll(controlTag)) {
            const tag = balanced(source, match.index).text;
            if (!isSettingControl(tag, match[1])) continue;
            const handler = handlerOf(tag);
            if (!handler) continue;
            const body = handler.name ? localBody(source, handler.name) ?? '' : handler.body;
            if (!sends(source, body)) continue;
            const line = source.slice(0, match.index).split('\n').length;
            ((found[file] ??= {})[handler.key] ??= []).push(line);
        }
    }
    return found;
}

test('no settings control saves on change, apart from the listed ones that are still being converted', () => {
    const found = offenders();
    const counted = Object.fromEntries(Object.entries(found).map(([file, handlers]) => [file, Object.fromEntries(Object.entries(handlers).map(([key, lines]) => [key, lines.length]))]));
    const where = Object.entries(found).flatMap(([file, handlers]) => Object.entries(handlers).map(([key, lines]) => `${file}:${lines.join(',')} ${key}`));
    assert.deepEqual(counted, stillSavingOnChange,
        `A settings control sends to the server from onChange/onClick (use useDraft + SaveBar from components/kit/save), or a listed one was converted and its line must go.\nFound:\n${where.join('\n')}`);
});

test('the sweep sees a control that saves on change, and not one that only edits a draft', () => {
    // The two conversions of alpha.132 are what the rule asks for.
    const found = offenders();
    assert.equal(found['ServerSettingLimits.tsx'], undefined, 'the password level saves on choose again');
    assert.equal(found['DomainGeneralSettings.tsx'], undefined, 'the www switch saves on change');
    // And the sweep itself still recognises the pattern it is for.
    const sample = `const choose = async (level) => { await sendSettings('/x', 'PUT', { level }); };
        <input type="radio" checked={a} onChange={() => void choose(level)} />`;
    const tag = balanced(sample, sample.indexOf('<input')).text;
    assert.ok(isSettingControl(tag, 'input'));
    assert.ok(sends(sample, handlerOf(tag).body));
    const draftOnly = `<input type="checkbox" checked={a} onChange={(e) => draft.set('x', e.target.checked)} />`;
    assert.ok(!sends(draftOnly, handlerOf(balanced(draftOnly, 0).text).body));
});
