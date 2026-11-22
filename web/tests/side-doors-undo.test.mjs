// S4 R1 fix round (21 Nov 2026): the side doors step's further lines, the
// server owner's undo with the administrator's explicit "close it again",
// the administrator-only Keep on a crontab line not made in CelikPanel, and
// a tool that could not get its own account at its install.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSideDoorsUndoScreens as en } from '../src/i18n/screens/sideDoorsUndo/en.ts';
import { trSideDoorsUndoScreens as tr } from '../src/i18n/screens/sideDoorsUndo/tr.ts';
import { enSiteJobsScreens } from '../src/i18n/screens/siteJobs/en.ts';
import { trSiteJobsScreens } from '../src/i18n/screens/siteJobs/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const kinds = ['tool_down', 'stock_pool_not_put_back', 'stock_pool_failed', 'stock_pool_unknown', 'stock_pool_owner_restored', 'at_owner_removed', 'site_crontab_unreadable'];

test('every new kind is shown, worded in both languages and says who acts', () => {
  const list = read('../src/components/ProblemsNow.tsx').match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/);
  const shown = [...list[1].matchAll(/'([a-z_]+)'/g)].map((m) => m[1]);
  for (const kind of kinds) {
    assert.ok(shown.includes(kind), `${kind} is shown`);
    for (const part of ['title', 'body', 'action']) {
      assert.ok(en[`pnow.${kind}.${part}`] && tr[`pnow.${kind}.${part}`], `${kind}.${part}`);
    }
    assert.match(en[`pnow.${kind}.body`], /administrator|server administrator/, `${kind} says who acts`);
  }
  assert.deepEqual(Object.keys(tr).sort(), Object.keys(en).sort());
});

test('the owner’s undo is closed again only by an explicit action, asked first in the panel’s own window', () => {
  const source = read('../src/components/ProblemsNow.tsx');
  assert.match(source, /p\.action\.kind === 'reclose' \? RECLOSE\.includes/, 'only the known doors');
  assert.match(source, /const RECLOSE_URL = '\/api\/v1\/metrics\/side-doors\/reclose';/);
  assert.match(source, /confirmAction\(\{[\s\S]*?fetch\(RECLOSE_URL, \{\s*method: 'POST'/, 'asked, then sent once');
  // Second fix round (B4): the panel runs it apart from the page; the window
  // follows it with reads only, and leaving the page does not cancel it.
  assert.match(source, /while \(status\?\.running[\s\S]*?await readRemote\(RECLOSE_URL,/, 'followed with reads only');
  assert.equal((source.match(/method: 'POST'/g) ?? []).length, 2, 'one start request (and the service start), never one per read');
  assert.match(source, /report\.steps\(/, 'the work is shown while it runs');
  assert.match(source, /handle\.retry\(\)/, 'the list reads itself again');
  for (const kind of ['stock_pool_owner_restored', 'at_owner_removed']) {
    assert.ok(en[`pnow.${kind}.confirm`] && tr[`pnow.${kind}.confirm`], `${kind} says what closing again does`);
    assert.match(en[`pnow.${kind}.body`], /does not write the file again by itself|leaves it on/, `${kind} says it is not redone by itself`);
  }
});

test('Keep is the administrator’s; a customer keeps only Delete', () => {
  const source = read('../src/components/DomainCronManager.tsx');
  assert.match(source, /const canKeep = role === 'admin' && !readOnly;/);
  assert.match(source, /\{canKeep && \(\s*<button\s+onClick=\{\(\) => void keepLine\(job\.id, job\.command\)\}/, 'on a marked task');
  assert.match(source, /\{canKeep && \(\s*<button\s+onClick=\{\(\) => void keepLine\(other\.id, other\.line\)\}/, 'on an other line');
  assert.match(source, /cron\?keep=\$\{encodeURIComponent\(id\)\}&version=/, 'against the crontab the list was read from');
  assert.match(source, /finally \{\s*loadJobs\(\);/, 'the list reads itself again');
  assert.equal(en['cron.outside.keep'], 'Keep');
  assert.equal(tr['cron.outside.keep'], 'Kalsın');
});

test('the guidance for lines not made in CelikPanel can be followed', () => {
  for (const words of [enSiteJobsScreens, trSiteJobsScreens]) {
    assert.ok(!/add it again|yeniden ekleyin/i.test(words['cron.outside.notice']), 'no order the panel refuses');
    assert.ok(!/add it again|yeniden ekleyin/i.test(words['pnow.site_crontab_outside.body']));
  }
  assert.match(enSiteJobsScreens['cron.outside.notice'], /Keep/);
  assert.match(trSiteJobsScreens['cron.outside.notice'], /Kalsın/);
});

test('a tool without its own account at its install is said with who acts', async () => {
  const { decodePHPToolProblems, installedNotes } = await import(new URL('../src/lib/componentOperationNotes.ts', import.meta.url).href);
  const result = { php_tool_problems: [{ tool: 'webmail', version: '8.4', kind: 'own_account_failed' }] };
  assert.deepEqual(decodePHPToolProblems(result), [{ tool: 'webmail', version: '8.4', kind: 'own_account_failed' }]);
  const say = (key, params) => (en[key] ?? key).replace(/\{(\w+)\}/g, (_, name) => String(params?.[name] ?? ''));
  const notes = installedNotes({ result, name: 'Roundcube', fallbackOnly: false, t: say });
  assert.ok(notes.some((note) => note.kind === 'warning' && /own account on PHP 8\.4/.test(note.text)), JSON.stringify(notes));
});

// Third fix round (finding 5): the panel keeps the last close-again with its
// time over a restart; a run a restart cut is said as not known, and a line
// that offers the action shows a last result that did not close the door.
test('the last close-again is shown again when the page is opened, a cut one as not known', () => {
  const source = read('../src/components/ProblemsNow.tsx');
  assert.match(source, /status\?\.running \|\| status\?\.unknown \|\|/, 'a run cut by a restart is not known');
  assert.match(source, /if \(offersReclose\) void readLastReclose\(\);/, 'read once when a line offers it');
  assert.match(source, /lastReclose\.error \|\| lastReclose\.unknown/, 'only a result that did not close it');
  assert.match(source, /t\('pnow\.reclose\.lastFailed', \{ time: clock\(lastReclose\.at\), reason: lastReclose\.error \}\)/);
  assert.match(source, /t\('pnow\.reclose\.unknown', \{ time: clock\(lastReclose\.at\) \}\)/);
  for (const key of ['pnow.reclose.lastFailed', 'pnow.reclose.unknown']) {
    assert.ok(en[key].includes('{time}') && tr[key].includes('{time}'), key);
  }
  assert.match(en['pnow.reclose.unknown'], /press the button again/, 'says what to do');
});
