// alpha.150 lab D6 (28 Nov 2026): a site's Try again, refused by the Agent
// because a marker of the panel's update was there, said "The result is not
// known — The server did not answer" although the Agent answered at once with
// its reason. On the real path the Agent's empty list of programs does not
// survive its transport (gob), so the page received "units": null and read no
// answer at all.
//
// The site card's real move (moveSiteIntoLimits) runs here with the product's
// question and window replaced by a recorder, and is fed exactly the JSON the
// panel's real handler sent for each refusal the Agent's real handler gives
// before anything changes (web/tests/fixtures/real-path/panel-move-refusals.json,
// kept by cmd/panel/real_path_refusals_test.go from the Agent's answers in
// agent-move-refusals.json). Each says its reason's own words and "nothing
// was moved" as a stop, never "not known"; a call that really got no answer
// keeps "the result is not known".
//
// Taşımanın reddi gerçek yolda: nedenin kendi sözleri, "hiçbir şey taşınmadı";
// yanıt gelmeyen çağrı "sonuç bilinmiyor" olarak kalır.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';
import { decodeMoveAnswer, moveRefusedBeforeAnything } from '../src/lib/siteLimits.ts';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const require = createRequire(import.meta.url);
const moduleURL = source => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const fileURL = relativePath => new URL(relativePath, import.meta.url).href;
const from = (path, url) => [new RegExp(`from ['"]${path.replace(/[.*+?^${}()|[\]\\/]/g, '\\$&')}['"]`, 'g'), `from '${url}'`];
const reactURL = pathToFileURL(require.resolve('react')).href;
const jsxRuntimeURL = pathToFileURL(require.resolve('react/jsx-runtime')).href;
let card = ts.transpileModule(readFileSync(new URL('../src/components/SiteLimitsCard.tsx', import.meta.url), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
}).outputText;
for (const [pattern, replacement] of [
    from('react/jsx-runtime', jsxRuntimeURL),
    from('react', reactURL),
    from('lucide-react', moduleURL('const icon = () => null; export const AlertTriangle = icon, ArrowRightLeft = icon, CheckCircle2 = icon, Gauge = icon, Undo2 = icon;')),
    from('../i18n', moduleURL('export function useI18n() { throw new Error("not rendered here"); }')),
    from('../i18n/siteLimitsLane', fileURL('../src/i18n/siteLimitsLane.ts')),
    from('../lib/dateFormat', fileURL('../src/lib/dateFormat.ts')),
    from('../lib/apiError', fileURL('../src/lib/apiError.ts')),
    // The product's question and window: the action runs at once and its end is kept.
    from('../lib/confirmAction', moduleURL(`export async function confirmAction(request) { const steps = []; globalThis.__end = await request.action({ steps: (s) => steps.push(s) }); globalThis.__steps = steps; return true; }
export async function followOperation(request) { return confirmAction(request); }`)),
    from('../lib/remote', moduleURL('export async function readRemote() { return { state: "unread" }; } export function useRemote() { throw new Error("not rendered here"); }')),
    from('../lib/visibleReads', moduleURL('export function startVisibleReads() { return () => {}; }')),
    from('../lib/settingLimits', fileURL('../src/lib/settingLimits.ts')),
    from('../lib/siteLimits', fileURL('../src/lib/siteLimits.ts')),
    from('../lib/siteTasks', fileURL('../src/lib/siteTasks.ts')),
    from('./ui', moduleURL('export const Button = () => null, Checking = () => null, CouldNotCheck = () => null;')),
]) card = card.replace(pattern, replacement);
const { moveSiteIntoLimits } = await import(moduleURL(card));

const fill = (text, vars) => String(text).replace(/\{(\w+)\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
const words = { en: { ...en, ...enScreens, ...enSiteLimitsLane }, tr: { ...tr, ...trScreens, ...trSiteLimitsLane } };
const sayIn = (locale) => (key, vars) => fill(words[locale][key] ?? key, vars);

// Exactly what the panel's real handler sent (cmd/panel/real_path_refusals_test.go).
const pages = JSON.parse(readFileSync(new URL('./fixtures/real-path/panel-move-refusals.json', import.meta.url), 'utf8'));
// What the Agent's real handler answered on the real transport (cmd/agent).
const agent = JSON.parse(readFileSync(new URL('./fixtures/real-path/agent-move-refusals.json', import.meta.url), 'utf8'));

// The lab's site: its tasks already inside, one program outside.
const view = {
    applies: 'not_yet', needs_move: 1,
    units: [{ unit: 'celikrt-1.service', label: 'program', placement: 'outside' }],
    tasks: { moved: true },
};

async function press(locale, answer) {
    globalThis.__end = undefined;
    globalThis.fetch = answer;
    await moveSiteIntoLimits(7, view, sayIn(locale), { id: 'move-0123456789abcdef01234567' });
    return globalThis.__end;
}

test('the Agent answers every refusal at once with its code, and its list of programs does not survive the transport', () => {
    assert.deepEqual(Object.keys(agent).sort(), ['another_move', 'build', 'kept_earlier', 'site_busy', 'update_finishing', 'update_running', 'update_unreadable']);
    for (const [code, answer] of Object.entries(agent)) {
        assert.equal(answer.reason?.code, code);
        assert.equal(answer.units, null, `${code}: the Agent's empty list arrives as null`);
        // The page's own reading of that raw answer is an answer, not "no answer".
        assert.doesNotThrow(() => decodeMoveAnswer(answer), code);
        assert.equal(moveRefusedBeforeAnything(decodeMoveAnswer(answer)), true, code);
    }
});

for (const [code, body] of Object.entries(pages)) {
    for (const locale of ['en', 'tr']) {
        test(`a refused Try again says why (${code}, ${locale})`, async () => {
            assert.deepEqual(body.units, [], 'the panel sends a list');
            const end = await press(locale, async () => new Response(JSON.stringify(body), { status: 200 }));
            const say = sayIn(locale);
            const why = say(`slim.why.${code}`);
            assert.notEqual(why, `slim.why.${code}`, 'the reason has words');
            assert.equal(end.state, 'blocked', 'a stop, not a failure or an unknown result');
            assert.equal(end.reason, why, 'the reason’s own words, once');
            assert.doesNotMatch(end.reason, new RegExp(say('slim.move.unknown').slice(0, 20)));
            assert.match(end.reason, locale === 'en' ? /nothing (new )?was (moved|changed)|Nothing (new )?was moved/ : /hiçbir şey (taşınmadı|değiştirilmedi)|taşınmadı/);
            // The JSON the lab's pair sent (the panel passed the Agent's null on): the same words.
            const lab = await press(locale, async () => new Response(JSON.stringify({ ...body, units: null }), { status: 200 }));
            assert.deepEqual(lab, end);
        });
    }
}

test('a call that got no answer keeps "the result is not known"', async () => {
    for (const locale of ['en', 'tr']) {
        const end = await press(locale, async () => { throw new TypeError('network'); });
        assert.equal(end.state, 'unknown');
        assert.equal(end.reason, sayIn(locale)('slim.move.unknown'));
        const unreadable = await press(locale, async () => new Response('{"neither":"done"}', { status: 200 }));
        assert.equal(unreadable.state, 'unknown', 'an answer that is not an answer stays unknown');
    }
});
