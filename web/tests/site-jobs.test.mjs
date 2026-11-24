// S4 R1 (21 Nov 2026): jobs a site's own code can make outside CelikPanel.
// The Problems now lines (crontab lines not made in CelikPanel, at) are
// kinds the list shows, with words in both languages in their own part that
// say who acts; the Scheduled tasks list marks such lines, removes them the
// way it removes tasks and offers no edit or pause for them.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enMonitoringScreens } from '../src/i18n/screens/monitoring/en.ts';
import { enSiteJobsScreens } from '../src/i18n/screens/siteJobs/en.ts';
import { trSiteJobsScreens } from '../src/i18n/screens/siteJobs/tr.ts';

const kinds = ['site_crontab_outside', 'site_at_jobs', 'at_allow_lists_sites', 'at_directory_accounts', 'at_not_closed'];

test('every site jobs kind is shown and has a title, body and action in English and Turkish', () => {
  const source = readFileSync(new URL('../src/components/ProblemsNow.tsx', import.meta.url), 'utf8');
  const list = source.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/);
  assert.ok(list, 'the KINDS list');
  const shown = [...list[1].matchAll(/'([a-z_]+)'/g)].map((m) => m[1]);
  for (const kind of kinds) {
    assert.ok(shown.includes(kind), `${kind} is shown`);
    for (const part of ['title', 'body', 'action']) {
      assert.ok(enSiteJobsScreens[`pnow.${kind}.${part}`], `English pnow.${kind}.${part}`);
      assert.ok(trSiteJobsScreens[`pnow.${kind}.${part}`], `Turkish pnow.${kind}.${part}`);
    }
    assert.equal(enMonitoringScreens[`pnow.${kind}.title`], undefined, `${kind} is not added to the full monitoring part`);
    assert.match(enSiteJobsScreens[`pnow.${kind}.body`], /administrator|site’s owner/, `${kind} says who acts`);
  }
  assert.deepEqual(Object.keys(trSiteJobsScreens).sort(), Object.keys(enSiteJobsScreens).sort());
});

test('a crontab changed by hand is worded as changed outside CelikPanel, not as an attack', () => {
  for (const words of [enSiteJobsScreens, trSiteJobsScreens]) {
    for (const [key, text] of Object.entries(words)) {
      assert.ok(!/attack|malicious|saldır|kötü amaçlı/i.test(text), `${key} accuses nobody`);
    }
  }
  assert.match(enSiteJobsScreens['cron.outside.notice'], /someone working on the server/);
});

test('the Scheduled tasks list marks lines not made in CelikPanel and only removes them', () => {
  const source = readFileSync(new URL('../src/components/DomainCronManager.tsx', import.meta.url), 'utf8');
  assert.match(source, /job\.outside_panel && \(/, 'the mark');
  assert.match(source, /!readOnly && !job\.outside_panel && \(/, 'pause and edit only for tasks made in CelikPanel');
  assert.match(source, /removeLine\(other\.id, other\.line, 'cron\.outside\.deleteConfirm', 'cron\.outside\.deleted'\)/, 'other lines removed by their ID');
  assert.ok(!/window\.confirm|[^.]confirm\(|alert\(/.test(source.replace(/confirmAction/g, '')), 'no browser dialog');
});
