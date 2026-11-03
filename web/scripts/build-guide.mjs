// The user guide's build (alpha.119). It reads docs/guide/{en,tr}/NN-slug.md,
// refuses anything outside the format of docs/guide/FORMAT.md, and writes:
//   - web/src/help/guide/gen/<role>.<locale>.ts: six lazy chunks, one per role
//     and language, each with its chapters, topics and search index;
//   - download-portal/guide/: the same chapters as static pages for
//     celikpanel.net, and manifest.sha256, the reviewed list the portal build
//     copies and checks.
// No dependency: the parser below accepts only the documented subset.
//
//   node scripts/build-guide.mjs          write the generated files
//   node scripts/build-guide.mjs --check  fail if they differ from the sources
//
// Kılavuzun derlemesi: docs/guide altındaki dosyaları okur, biçim dışını
// reddeder; paneldeki altı parçayı ve celikpanel.net sayfalarını yazar.
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { fold, wordsOf } from '../src/help/guide/search.ts';

export const ROLES = ['admin', 'reseller', 'customer'];
export const LOCALES = ['en', 'tr'];
const FIELDS = ['id', 'title', 'roles', 'topics', 'keywords'];
const ID = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const ANCHOR = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const TOPIC = /^[a-z0-9]+(?:[.-][a-z0-9]+)*$/;
const FILE = /^(\d\d)-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$/;

class GuideError extends Error {}

// ------------------------------------------------------------------ parsing

/**
 * Parses one chapter. Returns { chapter, errors }: errors are "file:line: why".
 * Bir bölümü ayrıştırır; hatalar "dosya:satır: neden" biçimindedir.
 */
export function parseChapter(text, file = 'chapter.md') {
    const errors = [];
    const fail = (line, message) => errors.push(`${file}:${line}: ${message}`);
    if (text.charCodeAt(0) === 0xfeff) fail(1, 'remove the byte-order mark');
    if (text.includes('\r')) fail(1, 'use LF line endings');
    const lines = text.replace(/^﻿/, '').replace(/\r\n?/g, '\n').split('\n');

    const meta = {};
    let at = 0;
    if (lines[0] !== '---') {
        fail(1, 'the file must start with front matter between two --- lines');
        return { chapter: null, errors };
    }
    at = 1;
    while (at < lines.length && lines[at] !== '---') {
        const match = /^([a-z]+):(?: (.*))?$/.exec(lines[at]);
        if (!match) fail(at + 1, `front matter line is not "field: value": ${lines[at]}`);
        else if (!FIELDS.includes(match[1])) fail(at + 1, `unknown front matter field "${match[1]}"`);
        else if (match[1] in meta) fail(at + 1, `front matter field "${match[1]}" appears twice`);
        else meta[match[1]] = { value: (match[2] ?? '').trim(), line: at + 1 };
        at += 1;
    }
    if (at >= lines.length) {
        fail(1, 'front matter is not closed with ---');
        return { chapter: null, errors };
    }
    const bodyStart = at + 1;
    for (const field of FIELDS) {
        if (!(field in meta)) fail(1, `front matter lacks "${field}"`);
    }
    const list = (field) => {
        const entry = meta[field];
        if (!entry) return [];
        const match = /^\[(.*)\]$/.exec(entry.value);
        if (!match) {
            fail(entry.line, `"${field}" must be a list: [a, b]`);
            return [];
        }
        const items = match[1].split(',').map((item) => item.trim()).filter((item) => item !== '');
        if (new Set(items).size !== items.length) fail(entry.line, `"${field}" lists an item twice`);
        return items;
    };
    const id = meta.id?.value ?? '';
    if (meta.id && !ID.test(id)) fail(meta.id.line, `id "${id}" must be lowercase a-z, 0-9 and -`);
    const title = meta.title?.value ?? '';
    if (meta.title) {
        if (title === '' || title.length > 80) fail(meta.title.line, 'title must be 1 to 80 characters');
        if (/[<>*`[\]&{}]/.test(title)) fail(meta.title.line, 'title is plain text: no markup characters');
    }
    const roles = list('roles');
    if (meta.roles && roles.length === 0) fail(meta.roles.line, 'roles must name at least one role');
    for (const role of roles) {
        if (!ROLES.includes(role)) fail(meta.roles.line, `unknown role "${role}" (use ${ROLES.join(', ')})`);
    }
    const topics = list('topics').map((entry) => {
        const [topic, anchor = '', extra] = entry.split('#');
        if (extra !== undefined || !TOPIC.test(topic) || (anchor !== '' && !ANCHOR.test(anchor)) || entry.endsWith('#')) {
            fail(meta.topics.line, `topic "${entry}" must be name or name#anchor (a-z, 0-9, . and -)`);
        }
        return { topic, anchor };
    });
    const keywords = list('keywords');
    for (const keyword of keywords) {
        if (/[<>*`[\]&{}]/.test(keyword) || keyword.length > 40) fail(meta.keywords.line, `keyword "${keyword}" must be plain text of at most 40 characters`);
    }

    const state = { file, fail, anchors: [], links: [], chapterId: id };
    const blocks = parseBlocks(lines, bodyStart, lines.length, state, false);
    return {
        chapter: { id, title, roles, topics, keywords, blocks, anchors: state.anchors, links: state.links },
        errors,
    };
}

const STARTS_BLOCK = /^(```|:::|#|\||- |\d+\. |>|\s+\S)/;

function parseBlocks(lines, from, to, state, inCallout) {
    const { fail } = state;
    const blocks = [];
    let i = from;
    while (i < to) {
        const line = lines[i];
        const n = i + 1;
        if (line.trim() === '') {
            i += 1;
            continue;
        }
        if (line.startsWith('```')) {
            const lang = line.slice(3).trim();
            if (!/^[a-z0-9+-]*$/.test(lang)) fail(n, 'a code fence may name one language word only');
            let end = i + 1;
            while (end < to && lines[end].trimEnd() !== '```') end += 1;
            if (end >= to) {
                fail(n, 'code block is not closed with ```');
                return blocks;
            }
            blocks.push(['pre', lines.slice(i + 1, end).join('\n')]);
            i = end + 1;
            continue;
        }
        if (line.startsWith(':::')) {
            const kind = /^:::(note|warning)\s*$/.exec(line)?.[1];
            if (!kind) {
                fail(n, line.trim() === ':::' ? 'a ::: closes nothing here' : 'only :::note and :::warning blocks exist');
                i += 1;
                continue;
            }
            if (inCallout) fail(n, 'a note or warning cannot hold another one');
            let end = i + 1;
            while (end < to && !/^:::\s*$/.test(lines[end])) {
                if (lines[end].startsWith('```')) {
                    // Skip a fenced block inside so its lines are not read as :::.
                    end += 1;
                    while (end < to && lines[end].trimEnd() !== '```') end += 1;
                }
                end += 1;
            }
            if (end >= to) {
                fail(n, `:::${kind} is not closed with :::`);
                return blocks;
            }
            const inner = parseBlocks(lines, i + 1, end, state, true);
            if (inner.length === 0) fail(n, `:::${kind} is empty`);
            blocks.push([kind === 'note' ? 'note' : 'warn', inner]);
            i = end + 1;
            continue;
        }
        if (/^#+(\s|$)/.test(line) || /^#+[^#\s]/.test(line)) {
            const level = /^#+/.exec(line)[0].length;
            if (level !== 2 && level !== 3) fail(n, `only ## and ### headings are allowed (the title is the chapter's top heading)`);
            else if (inCallout) fail(n, 'a heading cannot stand inside a note or warning');
            else {
                const match = /^#{2,3} (.+?) \{#([^}]*)\}\s*$/.exec(line);
                if (!match) fail(n, 'a heading needs an anchor: ## Heading {#anchor}');
                else if (!ANCHOR.test(match[2])) fail(n, `anchor "${match[2]}" must be lowercase a-z, 0-9 and -`);
                else {
                    if (state.anchors.some((anchor) => anchor.anchor === match[2])) fail(n, `anchor "${match[2]}" is used twice`);
                    state.anchors.push({ level, anchor: match[2], line: n });
                    blocks.push(['h', level, match[2], parseInline(match[1], state, n)]);
                }
            }
            i += 1;
            continue;
        }
        if (/^\s*>/.test(line)) {
            fail(n, 'block quotes are not part of the format; use :::note');
            i += 1;
            continue;
        }
        if (/^(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
            fail(n, 'horizontal rules are not part of the format');
            i += 1;
            continue;
        }
        if (/^\[[^\]]*\]:/.test(line)) {
            fail(n, 'reference links are not part of the format; write [text](target)');
            i += 1;
            continue;
        }
        if (line.startsWith('|')) {
            if (inCallout) fail(n, 'a table cannot stand inside a note or warning');
            const separator = lines[i + 1] ?? '';
            if (!/^\|(\s*:?-{3,}:?\s*\|)+\s*$/.test(separator)) {
                fail(n + 1, 'a table needs a |---|---| line under its header');
                i += 1;
                continue;
            }
            const head = splitRow(line, state, n);
            const rows = [];
            let end = i + 2;
            while (end < to && lines[end].startsWith('|')) {
                const row = splitRow(lines[end], state, end + 1);
                if (row.length !== head.length) fail(end + 1, `this row has ${row.length} cells, the header ${head.length}`);
                rows.push(row);
                end += 1;
            }
            if (separator.split('|').length - 2 !== head.length) fail(n + 1, 'the |---| line must have one cell per header cell');
            if (rows.length === 0) fail(n, 'a table needs at least one row');
            blocks.push(['t', head, rows]);
            i = end;
            continue;
        }
        const marker = /^(- |\d+\. )/.exec(line);
        if (marker) {
            const ordered = marker[1] !== '- ';
            const items = [];
            let text = [line.slice(marker[1].length)];
            let first = n;
            let end = i + 1;
            const flush = () => items.push(parseInline(text.join(' ').trim(), state, first));
            while (end < to) {
                const next = lines[end];
                if (next.trim() === '') break;
                const nextMarker = /^(- |\d+\. )/.exec(next);
                if (nextMarker && (nextMarker[1] !== '- ') === ordered) {
                    flush();
                    text = [next.slice(nextMarker[1].length)];
                    first = end + 1;
                } else if (/^\s+(- |\d+\. )/.test(next)) {
                    fail(end + 1, 'nested lists are not part of the format');
                } else if (/^ {2,}\S/.test(next)) {
                    text.push(next.trim());
                } else {
                    fail(end + 1, nextMarker
                        ? 'a list switches between - and 1. here; separate the two lists with a blank line'
                        : 'separate a list from the next paragraph with a blank line, or indent a continuation by two spaces');
                    break;
                }
                end += 1;
            }
            flush();
            blocks.push([ordered ? 'ol' : 'ul', items]);
            i = end;
            continue;
        }
        if (/^\s+\S/.test(line)) {
            fail(n, 'indented text is not part of the format; use ``` for code');
            i += 1;
            continue;
        }
        // A paragraph: lines up to a blank line or the start of another block.
        let end = i + 1;
        while (end < to && lines[end].trim() !== '' && !STARTS_BLOCK.test(lines[end])) {
            if (/^(=+|-+)\s*$/.test(lines[end])) fail(end + 1, 'underlined headings are not part of the format; use ## Heading {#anchor}');
            end += 1;
        }
        blocks.push(['p', parseInline(lines.slice(i, end).map((part) => part.trim()).join(' '), state, n)]);
        i = end;
    }
    return blocks;
}

// Splits a table row on the pipes that are not escaped and not in code.
function splitRow(line, state, n) {
    const body = line.trim();
    if (!body.endsWith('|') || body.endsWith('\\|')) state.fail(n, 'a table row must end with |');
    const cells = [];
    let current = '';
    let code = false;
    for (let k = 1; k < body.length; k += 1) {
        const c = body[k];
        if (c === '\\' && !code && k + 1 < body.length) {
            current += c + body[k + 1];
            k += 1;
        } else if (c === '`') {
            code = !code;
            current += c;
        } else if (c === '|' && !code) {
            cells.push(current);
            current = '';
        } else {
            current += c;
        }
    }
    if (current.trim() !== '') cells.push(current);
    return cells.map((cell) => parseInline(cell.trim(), state, n));
}

const ESCAPABLE = new Set(['*', '`', '[', ']', '|', '\\', '#', '{', '}']);

/** Parses inline text into a string or a run of strings and marks. */
export function parseInline(text, state, n, inLink = false) {
    const { fail } = state;
    const out = [];
    let plain = '';
    const push = (mark) => {
        if (plain !== '') out.push(plain);
        plain = '';
        out.push(mark);
    };
    let k = 0;
    while (k < text.length) {
        const c = text[k];
        if (c === '\\' && ESCAPABLE.has(text[k + 1])) {
            plain += text[k + 1];
            k += 2;
            continue;
        }
        if (c === '`') {
            const close = text.indexOf('`', k + 1);
            if (close < 0) {
                fail(n, 'a ` is not closed');
                plain += text.slice(k);
                break;
            }
            const code = text.slice(k + 1, close);
            if (code === '') fail(n, 'empty `` code');
            push(['c', code]);
            k = close + 1;
            continue;
        }
        if (c === '*' && text[k + 1] === '*') {
            const close = findClose(text, '**', k + 2);
            if (close < 0) {
                fail(n, 'a ** is not closed');
                plain += text.slice(k);
                break;
            }
            const inner = text.slice(k + 2, close);
            if (inner.trim() === '' || inner !== inner.trim()) fail(n, '**bold** must hold words with no space inside the stars');
            if (inner.includes('**')) fail(n, 'bold cannot be nested');
            push(['b', parseInline(inner, state, n, inLink)]);
            k = close + 2;
            continue;
        }
        if (c === '!' && text[k + 1] === '[') {
            fail(n, 'images are not part of the format');
            k += 2;
            continue;
        }
        if (c === '[') {
            const close = findClose(text, ']', k + 1);
            if (close < 0 || text[close + 1] !== '(') {
                fail(n, 'a [ starts a link: [text](target); write \\[ for the character');
                plain += c;
                k += 1;
                continue;
            }
            const end = text.indexOf(')', close + 2);
            if (end < 0) {
                fail(n, 'a link target is not closed with )');
                plain += text.slice(k);
                break;
            }
            if (inLink) fail(n, 'a link cannot hold another link');
            const label = parseInline(text.slice(k + 1, close), state, n, true);
            if (label === '' ) fail(n, 'a link needs text');
            const target = text.slice(close + 2, end).trim();
            push(linkMark(target, label, state, n));
            k = end + 1;
            continue;
        }
        if (c === '<' && /[A-Za-z/!?]/.test(text[k + 1] ?? '')) {
            fail(n, 'raw HTML is not part of the format');
            k += 1;
            continue;
        }
        if (c === '&' && /^&(#?[A-Za-z0-9]+);/.test(text.slice(k))) {
            fail(n, 'HTML entities are not part of the format; write the character itself');
            k += 1;
            continue;
        }
        plain += c;
        k += 1;
    }
    if (plain !== '') out.push(plain);
    if (out.length === 0) return '';
    if (out.length === 1 && typeof out[0] === 'string') return out[0];
    return out;
}

function findClose(text, token, from) {
    for (let k = from; k < text.length; k += 1) {
        if (text[k] === '\\') {
            k += 1;
            continue;
        }
        if (text[k] === '`') {
            const close = text.indexOf('`', k + 1);
            if (close < 0) return -1;
            k = close;
            continue;
        }
        if (text.startsWith(token, k)) return k;
    }
    return -1;
}

function linkMark(target, label, state, n) {
    if (target.startsWith('https://')) {
        if (!/^https:\/\/[A-Za-z0-9.-]+(?::\d+)?(?:[/?#][^\s<>"']*)?$/.test(target)) state.fail(n, `outside link "${target}" is not a plain https address`);
        return ['x', target, label];
    }
    let id = '';
    let anchor = '';
    if (target.startsWith('guide:')) {
        [id, anchor = ''] = target.slice(6).split('#');
        if (!ID.test(id)) state.fail(n, `guide link "${target}" must be guide:id or guide:id#anchor`);
    } else if (target.startsWith('#')) {
        id = state.chapterId;
        anchor = target.slice(1);
    } else {
        state.fail(n, `link target "${target}" must start with guide:, # or https://`);
        return ['x', 'https://celikpanel.net/', label];
    }
    if (anchor !== '' && !ANCHOR.test(anchor)) state.fail(n, `link anchor "#${anchor}" must be lowercase a-z, 0-9 and -`);
    state.links.push({ id, anchor, line: n });
    return ['g', id, anchor, label];
}

// ------------------------------------------------------------- the whole guide

/** Reads and checks every chapter. Returns { locales: {en: [...], tr: [...]}, errors }. */
export function readGuide(guideDir) {
    const errors = [];
    const locales = {};
    for (const locale of LOCALES) {
        const dir = join(guideDir, locale);
        locales[locale] = [];
        if (!existsSync(dir)) {
            errors.push(`${relativeName(guideDir, dir)}: the ${locale} chapters are missing`);
            continue;
        }
        for (const name of readdirSync(dir).sort()) {
            const file = `${locale}/${name}`;
            const match = FILE.exec(name);
            if (!match) {
                errors.push(`${file}: a chapter file is named NN-slug.md (two digits, lowercase slug)`);
                continue;
            }
            const { chapter, errors: found } = parseChapter(readFileSync(join(dir, name), 'utf8'), file);
            errors.push(...found);
            if (!chapter) continue;
            if (chapter.id !== match[2]) errors.push(`${file}:1: id "${chapter.id}" must equal the file's slug "${match[2]}"`);
            locales[locale].push({ ...chapter, file, order: match[1], name });
        }
    }
    for (const locale of LOCALES) checkLocale(locales[locale], errors);
    checkParity(locales, errors);
    return { locales, errors };
}

function checkLocale(chapters, errors) {
    const ids = new Map();
    const orders = new Map();
    const topics = new Map();
    for (const chapter of chapters) {
        if (ids.has(chapter.id)) errors.push(`${chapter.file}: id "${chapter.id}" is also used by ${ids.get(chapter.id)}`);
        ids.set(chapter.id, chapter.file);
        if (orders.has(chapter.order)) errors.push(`${chapter.file}: number ${chapter.order} is also used by ${orders.get(chapter.order)}`);
        orders.set(chapter.order, chapter.file);
        for (const { topic, anchor } of chapter.topics) {
            if (topics.has(topic)) errors.push(`${chapter.file}: topic "${topic}" is also claimed by ${topics.get(topic)}`);
            topics.set(topic, chapter.file);
            if (anchor !== '' && !chapter.anchors.some((a) => a.anchor === anchor)) {
                errors.push(`${chapter.file}: topic "${topic}#${anchor}" names an anchor this chapter does not have`);
            }
        }
    }
    const byId = new Map(chapters.map((chapter) => [chapter.id, chapter]));
    for (const chapter of chapters) {
        for (const link of chapter.links) {
            const target = byId.get(link.id);
            if (!target) errors.push(`${chapter.file}:${link.line}: broken link: no chapter "${link.id}"`);
            else if (link.anchor !== '' && !target.anchors.some((a) => a.anchor === link.anchor)) {
                errors.push(`${chapter.file}:${link.line}: broken link: chapter "${link.id}" has no anchor "#${link.anchor}"`);
            }
        }
    }
}

function checkParity(locales, errors) {
    const [first, second] = LOCALES;
    const a = new Map(locales[first].map((chapter) => [chapter.name, chapter]));
    const b = new Map(locales[second].map((chapter) => [chapter.name, chapter]));
    for (const name of a.keys()) if (!b.has(name)) errors.push(`${first}/${name}: has no ${second}/${name}`);
    for (const name of b.keys()) if (!a.has(name)) errors.push(`${second}/${name}: has no ${first}/${name}`);
    const same = (x, y) => JSON.stringify(x) === JSON.stringify(y);
    for (const [name, one] of a) {
        const two = b.get(name);
        if (!two) continue;
        const where = `${first}/${name} and ${second}/${name}`;
        if (one.id !== two.id) errors.push(`${where}: ids differ`);
        if (!same([...one.roles].sort(), [...two.roles].sort())) errors.push(`${where}: roles differ`);
        const topicList = (chapter) => chapter.topics.map((t) => `${t.topic}#${t.anchor}`).sort();
        if (!same(topicList(one), topicList(two))) errors.push(`${where}: topics differ`);
        const anchorList = (chapter) => chapter.anchors.map((x) => `${x.level}:${x.anchor}`);
        if (!same(anchorList(one), anchorList(two))) {
            errors.push(`${where}: headings differ (anchors and levels, in order): ${anchorList(one).join(' ')} / ${anchorList(two).join(' ')}`);
        }
        const linkSet = (chapter) => [...new Set(chapter.links.filter((l) => l.id !== chapter.id).map((l) => `${l.id}#${l.anchor}`))].sort();
        if (!same(linkSet(one), linkSet(two))) errors.push(`${where}: guide links differ: ${linkSet(one).join(' ')} / ${linkSet(two).join(' ')}`);
    }
}

// The topics the panel's screens name: <GuideLink topic="…"> and
// openGuideTopic('…') in web/src. A named topic that no chapter claims fails.
export function sourceTopics(srcDir) {
    const found = [];
    const walk = (dir) => {
        for (const name of readdirSync(dir)) {
            const path = join(dir, name);
            if (statSync(path).isDirectory()) {
                if (name !== 'gen' && name !== 'node_modules') walk(path);
            } else if (/\.(tsx?|mjs)$/.test(name)) {
                const text = readFileSync(path, 'utf8');
                for (const match of text.matchAll(/<GuideLink\b[^>]*?\btopic="([^"]+)"|openGuideTopic\('([^']+)'\)/g)) {
                    found.push({ topic: match[1] ?? match[2], file: relative(srcDir, path).split(sep).join('/') });
                }
            }
        }
    };
    if (existsSync(srcDir)) walk(srcDir);
    return found;
}

/** The route topics of web/src/help/guide/routes.ts (topic: and customerTopic: entries). */
export function routeTopics(routesFile) {
    if (!existsSync(routesFile)) return [];
    const text = readFileSync(routesFile, 'utf8');
    return [...new Set([...text.matchAll(/\b(?:topic|customerTopic): '([^']+)'/g)].map((m) => m[1]))];
}

// ----------------------------------------------------------------- the chunks
// One index per role and language (chapter list, topics, search) and one
// chunk per chapter and language, shared by every role that reads it, so no
// chapter is carried twice. A link to a chapter outside the reader's role is
// kept in the chapter and drawn as its words by the panel (ChapterView).
// Rol ve dil başına bir dizin, bölüm ve dil başına bir parça.

function indexFor(chapters, role, locale) {
    const included = chapters.filter((chapter) => chapter.roles.includes(role));
    const topics = {};
    for (const chapter of included) {
        for (const { topic, anchor } of chapter.topics) topics[topic] = [chapter.id, anchor];
    }
    // Titles, headings and keywords only: the text's words come from the
    // chapters themselves when someone searches (search.ts chapterBodies).
    const search = [];
    included.forEach((chapter, index) => {
        search.push([index, '', chapter.title, wordsOf(chapter.keywords.join(' ')).join(' ')]);
        for (const section of sectionsOf(chapter).slice(1)) search.push([index, section.anchor, section.heading, '']);
    });
    return {
        role, locale,
        chapters: included.map((chapter) => ({ id: chapter.id, title: chapter.title })),
        topics: Object.fromEntries(Object.entries(topics).sort()),
        search,
    };
}

export function inlineText(value) {
    if (typeof value === 'string') return value;
    return value.map((part) => {
        if (typeof part === 'string') return part;
        if (part[0] === 'c') return part[1];
        if (part[0] === 'b') return inlineText(part[1]);
        if (part[0] === 'g') return inlineText(part[3]);
        return inlineText(part[2]);
    }).join('');
}

function blockText(b) {
    switch (b[0]) {
        case 'h': return inlineText(b[3]);
        case 'p': return inlineText(b[1]);
        case 'ul': case 'ol': return b[1].map(inlineText).join(' ');
        case 't': return [...b[1], ...b[2].flat()].map(inlineText).join(' ');
        case 'pre': return b[1];
        default: return b[1].map(blockText).join(' ');
    }
}

// The chapter's opening text, then one section per heading.
function sectionsOf(chapter) {
    const sections = [{ anchor: '', heading: chapter.title, text: '' }];
    for (const b of chapter.blocks) {
        if (b[0] === 'h') sections.push({ anchor: b[2], heading: inlineText(b[3]), text: inlineText(b[3]) });
        else sections[sections.length - 1].text += ` ${blockText(b)}`;
    }
    return sections;
}

const GENERATED = (locale) => `// Generated by web/scripts/build-guide.mjs from docs/guide/${locale}. Do not edit:
// change the chapters and run \`npm run guide\` in web/.
// build-guide.mjs tarafından üretildi; elle değiştirmeyin.
`;

function indexSource(index) {
    const chapters = index.chapters.map((chapter) => `    { id: ${JSON.stringify(chapter.id)}, title: ${JSON.stringify(chapter.title)}, load: () => import('./chapters/${chapter.id}.${index.locale}') },`).join('\n');
    const search = index.search.map((entry) => `    ${JSON.stringify(entry)},`).join('\n');
    return `${GENERATED(index.locale)}import type { GuideIndex } from '../types';

const guide: GuideIndex = {
  v: 2,
  role: ${JSON.stringify(index.role)},
  locale: ${JSON.stringify(index.locale)},
  chapters: [
${chapters}
  ],
  topics: ${JSON.stringify(index.topics)},
  search: [
${search}
  ],
};

export default guide;
`;
}

function chapterSource(chapter, locale) {
    const blocks = chapter.blocks.map((block) => `    ${JSON.stringify(block)},`).join('\n');
    return `${GENERATED(locale)}import type { GuideChapter } from '../../types';

const chapter = {
  id: ${JSON.stringify(chapter.id)},
  title: ${JSON.stringify(chapter.title)},
  blocks: [
${blocks}
  ],
} as unknown as GuideChapter;

export default chapter;
`;
}

// ------------------------------------------------------------- portal pages

const PORTAL = {
    en: {
        lang: 'en', other: 'tr', guide: 'User guide', skip: 'Skip to main content', home: 'CelikPanel home page',
        menu: 'Main menu', languages: 'Language selection', otherName: 'Türkçe', chapters: 'Chapters',
        note: 'Note', warning: 'Warning', table: 'Table', previous: 'Previous', next: 'Next', footer: 'Footer navigation',
        description: 'How to use CelikPanel: sites, mail, domains, files and the server, for administrators, resellers and customers.',
        lede: 'How to use CelikPanel, chapter by chapter. The same guide is in the panel: press ? at the top right of any page.',
        groups: { all: 'For everyone', admin: 'For administrators', reseller: 'For resellers', customer: 'For customers' },
        nav: [['Explore the panel', '/#panel'], ['Features', '/#ozellikler'], ['Technical evidence', '/technical.html'], ['Supported systems', '/systems.html'], ['User guide', '/guide/en/'], ['My account', '/account/']],
        tagline: 'Your server. Your control.', bottom: 'Free during alpha · Linux amd64',
        choose: 'Choose a language',
    },
    tr: {
        lang: 'tr', other: 'en', guide: 'Kullanım kılavuzu', skip: 'Ana içeriğe geç', home: 'CelikPanel ana sayfa',
        menu: 'Ana menü', languages: 'Dil seçimi', otherName: 'English', chapters: 'Bölümler',
        note: 'Not', warning: 'Uyarı', table: 'Tablo', previous: 'Önceki', next: 'Sonraki', footer: 'Alt menü',
        description: 'CelikPanel nasıl kullanılır: siteler, e-posta, alan adları, dosyalar ve sunucu; yöneticiler, bayiler ve müşteriler için.',
        lede: 'CelikPanel’in kullanımı, bölüm bölüm. Aynı kılavuz panelde de var: herhangi bir sayfanın sağ üstündeki ? düğmesine basın.',
        groups: { all: 'Herkes için', admin: 'Yöneticiler için', reseller: 'Bayiler için', customer: 'Müşteriler için' },
        nav: [['Paneli keşfet', '/#panel'], ['Özellikler', '/#ozellikler'], ['Teknik kanıtlar', '/technical.html'], ['Desteklenen sistemler', '/systems.html'], ['Kullanım kılavuzu', '/guide/tr/'], ['Hesabım', '/account/']],
        tagline: 'Sunucunuz sizin. Yönetimi de öyle.', bottom: 'Alfa boyunca ücretsiz · Linux amd64',
        choose: 'Bir dil seçin',
    },
};

const escapeHTML = (value) => value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

const BRAND = '<svg class="brand-mark" viewBox="0 0 32 32" width="36" height="36" aria-hidden="true" focusable="false"><path fill="currentColor" d="M11 3H29V10H14L10 14V18L14 22H29V29H11L3 21V11Z M20 13H29V19H20Z"/></svg><span class="brand-name">Celik<span>Panel</span></span>';

function inlineHTML(value) {
    if (typeof value === 'string') return escapeHTML(value);
    return value.map((part) => {
        if (typeof part === 'string') return escapeHTML(part);
        if (part[0] === 'b') return `<strong>${inlineHTML(part[1])}</strong>`;
        if (part[0] === 'c') return `<code>${escapeHTML(part[1])}</code>`;
        if (part[0] === 'g') return `<a href="${escapeHTML(`${part[1]}.html${part[2] ? `#${part[2]}` : ''}`)}">${inlineHTML(part[3])}</a>`;
        return `<a href="${escapeHTML(part[1])}" rel="noopener noreferrer">${inlineHTML(part[2])}</a>`;
    }).join('');
}

function blocksHTML(blocks, words, label) {
    let heading = label;
    return blocks.map((b) => {
        switch (b[0]) {
            case 'h':
                heading = inlineText(b[3]);
                return `<h${b[1]} id="${b[2]}">${inlineHTML(b[3])}</h${b[1]}>`;
            case 'p': return `<p>${inlineHTML(b[1])}</p>`;
            case 'ul': case 'ol': return `<${b[0]}>${b[1].map((item) => `<li>${inlineHTML(item)}</li>`).join('')}</${b[0]}>`;
            case 't': return `<div class="table-scroll guide-table-scroll" tabindex="0" role="region" aria-label="${escapeHTML(`${words.table}: ${heading}`)}"><table class="guide-table"><thead><tr>${b[1].map((cell) => `<th scope="col">${inlineHTML(cell)}</th>`).join('')}</tr></thead><tbody>${b[2].map((row) => `<tr>${row.map((cell) => `<td>${inlineHTML(cell)}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
            case 'pre': return `<pre class="guide-code"><code>${escapeHTML(b[1])}</code></pre>`;
            case 'note': case 'warn': return `<aside class="guide-callout guide-${b[0] === 'note' ? 'note' : 'warning'}"><p class="guide-callout-label">${b[0] === 'note' ? words.note : words.warning}</p>${blocksHTML(b[1], words, heading)}</aside>`;
            default: throw new GuideError(`unknown block ${b[0]}`);
        }
    }).join('\n');
}

function audience(chapter) {
    return ROLES.every((role) => chapter.roles.includes(role)) ? ['all'] : ROLES.filter((role) => chapter.roles.includes(role));
}

function tocHTML(chapters, words, currentId) {
    const groups = ['all', ...ROLES].map((group) => [group, chapters.filter((chapter) => audience(chapter).includes(group))]).filter(([, list]) => list.length > 0);
    // On the index the groups are the page's sections; beside a chapter they
    // are labels, so the chapter's own title stays its first heading.
    const label = (text) => (currentId === '' ? `<h2>${text}</h2>` : `<p class="guide-toc-group">${text}</p>`);
    return groups.map(([group, list]) => `${label(escapeHTML(words.groups[group]))}<ol>${list.map((chapter) => {
        const current = chapter.id === currentId ? ' aria-current="page"' : '';
        return `<li><a href="${chapter.id}.html"${current}>${escapeHTML(chapter.title)}</a></li>`;
    }).join('')}</ol>`).join('');
}

function page(words, { title, description, path, otherPath, body }) {
    const other = PORTAL[words.other];
    return `<!doctype html><html lang="${words.lang}"><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>CelikPanel | ${escapeHTML(title)}</title><meta name="description" content="${escapeHTML(description)}"/>
<meta name="theme-color" content="#0d2247"/><link rel="icon" type="image/svg+xml" href="/assets/favicon-v2.svg"/>
<link rel="stylesheet" href="/assets/site.css"/><link rel="stylesheet" href="/assets/guide.css"/>
<link rel="alternate" hreflang="${other.lang}" href="${otherPath}"/></head><body data-page="guide">
<!-- Generated by web/scripts/build-guide.mjs from docs/guide. Do not edit. -->
<a class="skip-link" href="#content">${escapeHTML(words.skip)}</a>
<header class="site-header"><div class="container nav-shell">
<a class="brand" href="/" aria-label="${escapeHTML(words.home)}">${BRAND}</a>
<nav class="site-nav" aria-label="${escapeHTML(words.menu)}">
${words.nav.map(([label, href]) => `<a href="${href}"${href === `/guide/${words.lang}/` ? ' aria-current="true"' : ''}>${escapeHTML(label)}</a>`).join('\n')}</nav>
<div class="nav-tools"><nav class="language-switch guide-language" aria-label="${escapeHTML(words.languages)}">
<a href="${path}" lang="${words.lang}" aria-current="true">${words.lang.toUpperCase()}</a>
<a href="${otherPath}" lang="${other.lang}" hreflang="${other.lang}" aria-label="${escapeHTML(words.otherName)}">${other.lang.toUpperCase()}</a></nav></div></div></header>
<main id="content" class="technical-page guide-page">
${body}
</main><footer class="site-footer"><div class="container footer-main">
<div><a class="brand" href="/" aria-label="CelikPanel">${BRAND}</a><p class="footer-description">${escapeHTML(words.tagline)}</p></div>
<div class="footer-links"><nav aria-label="${escapeHTML(words.footer)}">${words.nav.slice(2, 5).map(([label, href]) => `<a href="${href}">${escapeHTML(label)}</a>`).join('')}</nav></div></div>
<div class="container footer-bottom"><span>© 2026 CelikPanel</span><span>${escapeHTML(words.bottom)}</span></div></footer></body></html>
`;
}

function firstParagraph(chapter) {
    const first = chapter.blocks.find((b) => b[0] === 'p');
    const text = first ? inlineText(first[1]) : chapter.title;
    if (text.length <= 160) return text;
    const cut = text.slice(0, 157).replace(/\s+\S*$/, '');
    return /[.!?]$/.test(cut) ? cut : `${cut}…`;
}

function portalPages(locales) {
    const files = {};
    for (const locale of LOCALES) {
        const words = PORTAL[locale];
        const chapters = locales[locale];
        files[`${locale}/index.html`] = page(words, {
            title: words.guide,
            description: words.description,
            path: `/guide/${locale}/`,
            otherPath: `/guide/${words.other}/`,
            body: `<section class="technical-intro"><div class="container"><h1>${escapeHTML(words.guide)}</h1><p class="section-lede">${escapeHTML(words.lede)}</p></div></section>
<section class="system-block"><div class="container guide-index">${tocHTML(chapters, words, '')}</div></section>`,
        });
        chapters.forEach((chapter, index) => {
            const previous = chapters[index - 1];
            const next = chapters[index + 1];
            const pager = [
                previous ? `<a rel="prev" href="${previous.id}.html"><span>${escapeHTML(words.previous)}</span>${escapeHTML(previous.title)}</a>` : '',
                next ? `<a rel="next" href="${next.id}.html"><span>${escapeHTML(words.next)}</span>${escapeHTML(next.title)}</a>` : '',
            ].join('');
            files[`${locale}/${chapter.id}.html`] = page(words, {
                title: `${chapter.title} · ${words.guide}`,
                description: firstParagraph(chapter),
                path: `/guide/${locale}/${chapter.id}.html`,
                otherPath: `/guide/${words.other}/${chapter.id}.html`,
                body: `<div class="container guide-layout">
<nav class="guide-toc" aria-label="${escapeHTML(words.chapters)}"><p class="guide-toc-home"><a href="./">${escapeHTML(words.guide)}</a></p>${tocHTML(chapters, words, chapter.id)}</nav>
<article class="guide-article"><h1>${escapeHTML(chapter.title)}</h1>
<p class="guide-audience">${audience(chapter).map((group) => escapeHTML(words.groups[group])).join(' · ')}</p>
${blocksHTML(chapter.blocks, words, chapter.title)}
${pager ? `<nav class="guide-pager" aria-label="${escapeHTML(words.chapters)}">${pager}</nav>` : ''}
</article></div>`,
            });
        });
    }
    files['index.html'] = `<!doctype html><html lang="tr"><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>CelikPanel | Kullanım kılavuzu · User guide</title><meta name="description" content="${escapeHTML(PORTAL.tr.description)}"/>
<meta name="theme-color" content="#0d2247"/><link rel="icon" type="image/svg+xml" href="/assets/favicon-v2.svg"/>
<link rel="stylesheet" href="/assets/site.css"/><link rel="stylesheet" href="/assets/guide.css"/></head><body data-page="guide">
<!-- Generated by web/scripts/build-guide.mjs from docs/guide. Do not edit. -->
<main id="content" class="technical-page guide-page"><section class="technical-intro"><div class="container">
<a class="brand" href="/" aria-label="${escapeHTML(PORTAL.tr.home)}">${BRAND}</a>
<h1>Kullanım kılavuzu <span lang="en">· User guide</span></h1>
<p class="section-lede">${escapeHTML(PORTAL.tr.choose)} <span lang="en">· ${escapeHTML(PORTAL.en.choose)}</span></p>
<nav class="technical-nav" aria-label="${escapeHTML(PORTAL.tr.languages)}"><a href="/guide/tr/" lang="tr" hreflang="tr">Türkçe</a><a href="/guide/en/" lang="en" hreflang="en">English</a></nav>
</div></section></main></body></html>
`;
    const manifest = Object.keys(files).sort().map((name) => `${sha256(files[name])}  ${name}`).join('\n');
    files['manifest.sha256'] = `${manifest}\n`;
    return files;
}

const sha256 = (text) => createHash('sha256').update(text, 'utf8').digest('hex');

// ------------------------------------------------------------------- driver

/**
 * Builds everything in memory. Returns { files: Map(path relative to the
 * repository -> text), errors, notes }. Nothing is written.
 */
export function buildGuide({ repoRoot }) {
    const guideDir = join(repoRoot, 'docs', 'guide');
    const { locales, errors } = readGuide(guideDir);
    const notes = [];
    for (const locale of LOCALES) {
        const claimed = new Set(locales[locale].flatMap((chapter) => chapter.topics.map((t) => t.topic)));
        for (const { topic, file } of sourceTopics(join(repoRoot, 'web', 'src'))) {
            if (!claimed.has(topic)) errors.push(`web/src/${file}: topic "${topic}" is claimed by no ${locale} chapter`);
        }
        if (locale === 'en') {
            const open = routeTopics(join(repoRoot, 'web', 'src', 'help', 'guide', 'routes.ts')).filter((topic) => !claimed.has(topic));
            if (open.length > 0) notes.push(`pages without a chapter yet (they open the first chapter): ${open.join(', ')}`);
        }
    }
    const files = new Map();
    if (errors.length > 0) return { files, errors: [...new Set(errors)], notes };
    for (const locale of LOCALES) {
        for (const role of ROLES) {
            files.set(`web/src/help/guide/gen/${role}.${locale}.ts`, indexSource(indexFor(locales[locale], role, locale)));
        }
        for (const chapter of locales[locale]) {
            files.set(`web/src/help/guide/gen/chapters/${chapter.id}.${locale}.ts`, chapterSource(chapter, locale));
        }
    }
    for (const [name, text] of Object.entries(portalPages(locales))) files.set(`download-portal/guide/${name}`, text);
    return { files, errors, notes };
}

// Every file the guide owns on disk, so a removed chapter's page goes too.
function ownedOnDisk(repoRoot) {
    const owned = [];
    const walk = (dir) => {
        if (!existsSync(dir)) return;
        for (const name of readdirSync(dir)) {
            const path = join(dir, name);
            if (statSync(path).isDirectory()) walk(path);
            else owned.push(relative(repoRoot, path).split(sep).join('/'));
        }
    };
    walk(join(repoRoot, 'web', 'src', 'help', 'guide', 'gen'));
    walk(join(repoRoot, 'download-portal', 'guide'));
    return owned;
}

/** Compares the built files with the disk. Returns the paths that differ. */
export function staleFiles(repoRoot, files) {
    const stale = [];
    for (const [path, text] of files) {
        const full = join(repoRoot, path);
        if (!existsSync(full) || readFileSync(full, 'utf8') !== text) stale.push(path);
    }
    for (const path of ownedOnDisk(repoRoot)) if (!files.has(path)) stale.push(path);
    return stale.sort();
}

function main() {
    const repoRoot = fileURLToPath(new URL('../../', import.meta.url));
    const check = process.argv.includes('--check');
    const { files, errors, notes } = buildGuide({ repoRoot });
    if (errors.length > 0) {
        console.error(`Guide: ${errors.length} problem(s) in docs/guide (see docs/guide/FORMAT.md):`);
        for (const error of errors) console.error(`- ${error}`);
        process.exit(1);
    }
    const stale = staleFiles(repoRoot, files);
    if (check) {
        if (stale.length > 0) {
            console.error('Guide: the generated files are not up to date. Run `npm run guide` in web/ and commit them:');
            for (const path of stale) console.error(`- ${path}`);
            process.exit(1);
        }
    } else {
        for (const path of ownedOnDisk(repoRoot)) if (!files.has(path)) rmSync(join(repoRoot, path));
        for (const [path, text] of files) {
            const full = join(repoRoot, path);
            mkdirSync(dirname(full), { recursive: true });
            if (!existsSync(full) || readFileSync(full, 'utf8') !== text) writeFileSync(full, text);
        }
    }
    const chapters = files.size;
    console.log(`Guide: ${check ? 'checked' : 'wrote'} ${chapters} generated files${!check && stale.length ? ` (${stale.length} changed)` : ''}.`);
    for (const note of notes) console.log(`Guide: ${note}.`);
}

function relativeName(root, path) {
    return relative(root, path).split(sep).join('/');
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
    try {
        main();
    } catch (error) {
        console.error(error instanceof GuideError ? `Guide: ${error.message}` : error);
        process.exit(1);
    }
}

export { fold };
