import assert from 'node:assert/strict';
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

// Owner, 12 Nov 2026, with a screenshot of the form that lets an address reach
// a database: "the fields where an IP address is typed are long enough to take
// epic texts; is that normal aesthetically?" and "there are lots of fields like
// this; selects too". A field is as wide as what it holds, not as wide as its
// card: every text, number and date input and every select under
// src/components declares one of the sizes of src/index.css (field-xs, -sm,
// -md, -lg, -xl, -full). The shared fields take a size prop with a deliberate
// default (PasswordInput: lg; SetupWizardInput: lg). This file fails when a
// field has no declared size.
//
// Sahip, 12 Kas 2026: alan, kartı kadar değil tuttuğu değer kadar geniştir.
// components altındaki her metin, sayı ve tarih alanı ile her açılır liste
// index.css boyutlarından birini taşır; taşımayan alan bu testi düşürür.

const components = fileURLToPath(new URL('../src/components/', import.meta.url));
const css = readFileSync(fileURLToPath(new URL('../src/index.css', import.meta.url)), 'utf8');

// Files other alpha.133 lanes were editing while the sizes were applied. The
// integrator gives their fields the sizes listed in the alpha.133 report and
// removes each line; an entry whose fields are all sized fails below, so the
// list only shrinks. Entries without a field today stay until those lanes land.
// alpha.133'te başka şeritlerin düzenlediği dosyalar; birleştiren boyutları
// verip satırı siler.
const notYetSized = new Set([
    'AddDatabaseModalV2.tsx', // lane dblist-fe
    'AddUserModalV2.tsx', // lane dblist-fe
    'DatabaseManagementV2.tsx', // lane dblist-fe
    'DatabaseAccountStrip.tsx', // lane dbserver
    'DatabaseRemoteAccess.tsx', // lane dbserver
    'DatabaseRemoteServerPanel.tsx', // lane dbserver
    'MariaDBManagement.tsx', // lane dbserver
    'PostgreSQLManagement.tsx', // lane dbserver
    'SecurityPage.tsx', // lane dbserver
    'PlanLimitsSection.tsx', // lane leftovers (plan forms)
    'PlanRights.tsx', // lane leftovers (plan forms)
    'AccountRights.tsx', // lane leftovers (plan choice of an account)
    'RightsSetEditor.tsx', // plan forms
    'PostfixManagement.tsx', // Postfix page
    'MailRelaySettings.tsx', // Postfix page: its relay form
    'Domains.tsx', // lane leftovers
]);
const notYetSizedDirs = ['databases/', 'kit/'];

function sources(dir) {
    return readdirSync(dir).flatMap((name) => {
        const path = join(dir, name);
        if (statSync(path).isDirectory()) return sources(path);
        return /\.tsx$/.test(name) ? [path] : [];
    });
}

const code = (text) => text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:'"`])\/\/.*$/gm, '$1');

// Each opening tag of a field, whole: braces are followed so an arrow function
// or a template in an attribute does not end the tag.
export function fieldTags(text) {
    const out = [];
    for (const m of text.matchAll(/<(input|select|PasswordInput|SetupWizardInput)\b/g)) {
        let i = m.index + m[0].length;
        let depth = 0;
        for (; i < text.length; i++) {
            const c = text[i];
            if (c === '{') depth++;
            else if (c === '}') depth--;
            else if (c === '>' && depth === 0) break;
        }
        out.push({ kind: m[1], text: text.slice(m.index, i + 1), line: text.slice(0, m.index).split('\n').length });
    }
    return out;
}

const notText = /^(checkbox|radio|hidden|file|range|color|submit|button|reset|image)$/;
// Whether a tag is a field that holds typed text, a number or a date.
export function needsSize(tag) {
    if (tag.kind === 'select') return true;
    if (tag.kind !== 'input') return false;
    if (/\shidden(\s|\/|>|=\{true\})/.test(tag.text)) return false;
    const fixed = tag.text.match(/\btype=(?:"([^"]*)"|'([^']*)'|\{\s*["'`]([^"'`]*)["'`]\s*\})/);
    if (fixed) return !notText.test(fixed[1] ?? fixed[2] ?? fixed[3]);
    const chosen = tag.text.match(/\btype=\{([^}]*)\}/);
    if (chosen) {
        const options = [...chosen[1].matchAll(/["'`]([a-z-]+)["'`]/g)].map((x) => x[1]);
        // type={type} of a choice component: radio or checkbox, never typed text.
        return options.some((type) => !notText.test(type));
    }
    return true;
}

const declared = /(^|[\s"'`{])field-(xs|sm|md|lg|xl|full)(?=[\s"'`}]|$)|field-\$\{size\}/;

test('the six field sizes are defined once, as tokens, in index.css', () => {
    for (const size of ['xs', 'sm', 'md', 'lg', 'xl']) {
        assert.match(css, new RegExp(`--field-${size}:\\s*[\\d.]+rem;`), `--field-${size}`);
        assert.ok(css.includes(`.field-${size} { --field-width: var(--field-${size}); }`), `.field-${size}`);
    }
    assert.ok(css.includes('.field-full { --field-width: 100%; }'));
    // A field has its size and never grows past its container, so a phone
    // narrows it; a frame around a field fills its container up to the size.
    assert.match(css, /:is\(input, select, textarea\):is\([^)]*\) \{\s*display: block;\s*width: var\(--field-width\);\s*max-width: 100%;\s*\}/);
    assert.match(css, /\.field-full \{\s*width: 100%;\s*max-width: var\(--field-width\);\s*\}/);
});

test('every text, number and date field and every select declares its size', () => {
    const missing = [];
    for (const file of sources(components)) {
        const name = relative(components, file).split('\\').join('/');
        if (notYetSized.has(name) || notYetSizedDirs.some((dir) => name.startsWith(dir))) continue;
        for (const tag of fieldTags(code(readFileSync(file, 'utf8')))) {
            if (tag.kind === 'PasswordInput' || tag.kind === 'SetupWizardInput') {
                // Shared fields: lg unless the form names another size.
                const size = tag.text.match(/\ssize=(?:"([^"]*)"|'([^']*)'|\{([^}]*)\})/);
                if (size) assert.match(size[1] ?? size[2] ?? size[3], /^(xs|sm|md|lg|xl|full)$/, `${name}:${tag.line}`);
                continue;
            }
            if (needsSize(tag) && !declared.test(tag.text)) missing.push(`${name}:${tag.line} <${tag.kind}>`);
        }
    }
    assert.deepEqual(missing, [], 'give each field a width class: field-xs | field-sm | field-md | field-lg | field-xl | field-full (src/index.css)');
});

test('the list of files not yet sized only shrinks', () => {
    for (const name of notYetSized) {
        const file = join(components, name);
        if (!existsSync(file)) continue;
        const tags = fieldTags(code(readFileSync(file, 'utf8'))).filter((tag) => tag.kind !== 'PasswordInput' && tag.kind !== 'SetupWizardInput' && needsSize(tag));
        // A file with no field yet (another lane may add one) stays listed.
        if (tags.length > 0) assert.ok(tags.some((tag) => !declared.test(tag.text)), `${name} has every field sized: remove it from notYetSized`);
    }
});

test('the shared fields default to a deliberate size', () => {
    const password = readFileSync(join(components, 'PasswordInput.tsx'), 'utf8');
    assert.match(password, /size = 'lg',/);
    assert.match(password, /className=\{`min-w-0 field-\$\{size\}`\}/);
    const wizard = readFileSync(join(components, 'ServerSetupWizardSteps.tsx'), 'utf8');
    assert.match(wizard, /export function SetupWizardInput\(\{[^}]*size = 'lg' \}/);
});

test('the guard reads each way a field is written', () => {
    const sample = [
        '<input value={a} className={inputClass} />',
        '<input type="number" className={`${inputClass} field-xs`} />',
        "<input type='checkbox' className='h-4 w-4' />",
        '<input type={show ? \'text\' : \'password\'} className="w-full" />',
        '<input type={type} name={name} />',
        '<input type="text" readOnly hidden />',
        '<select onChange={(e) => set(e.target.value)} className="w-full rounded-lg field-md">',
        '<select className={inputClass}>',
    ].join('\n');
    const tags = fieldTags(sample);
    assert.equal(tags.length, 8);
    assert.deepEqual(tags.map((tag) => needsSize(tag)), [true, true, false, true, false, false, true, true]);
    assert.deepEqual(tags.filter((tag) => needsSize(tag)).map((tag) => declared.test(tag.text)), [false, true, false, true, false]);
});
