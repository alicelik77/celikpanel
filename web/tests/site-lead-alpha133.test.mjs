// alpha.133: a site that IS a program or a service app is easy to find
// (owner, 12 Nov 2026: after alpha.132 moved "Programs" from a tab into the
// Site section he could not find his running Go program). In the Site
// section the part that matches the site's kind comes first and open; the
// Overview's Site tile says what runs there and goes straight to it, from
// the domain's own facts (no new read).
//
// alpha.133: Program ya da hizmet uygulaması olan sitede Site bölümünün ilk
// ve açık parçası odur; Genel Bakış'taki Site kutucuğu orada ne çalıştığını
// söyler ve doğrudan oraya götürür.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { compileSource } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const sections = await import(compileSource('components/domain/sections.ts', (specifier) => { throw new Error(`sections.ts imports ${specifier}`); }));
const server = { mail: true, databases: true };
const site = (projectType, role = 'customer', closedRights = []) => sections.buildSections({ role, projectType, closedRights, server })
  .find((section) => section.id === 'site').parts;

test('the Site section leads with the part that is the site, per kind, and opens it', () => {
  for (const role of ['admin', 'reseller', 'customer']) {
    const program = site('runtime_app', role);
    assert.equal(program[0].id, 'programs', `${role}: a program site leads with Programs`);
    assert.equal(program[0].startsOpen, true);
    assert.ok(program.findIndex((part) => part.id === 'type') > 0, 'Site type comes after');
    // Its right closed later: what runs stays manageable, and still leads.
    assert.equal(site('runtime_app', role, ['program_apps', 'python_apps'])[0].id, 'programs');

    const service = site('service_app', role);
    assert.equal(service[0].id, 'ai', `${role}: a service app site leads with AI and automation`);
    assert.equal(service[0].startsOpen, true);
    assert.ok(service.findIndex((part) => part.id === 'type') > 0);
  }
  // Every other kind keeps the order it had: Site type first.
  for (const kind of ['php', 'static', 'node', 'proxy', 'forwarding']) {
    assert.equal(site(kind)[0].id, 'type', kind);
    const programs = site(kind).find((part) => part.id === 'programs');
    if (programs) assert.equal(programs.startsOpen, false, `${kind}: Programs does not start open`);
  }
  assert.equal(sections.siteLeadPart('runtime_app'), 'programs');
  assert.equal(sections.siteLeadPart('service_app'), 'ai');
  assert.equal(sections.siteLeadPart('php'), null);
  assert.equal(sections.siteLeadPart(undefined), null);
});

test('the Overview\'s Site tile says what runs there with the kit\'s kind word and goes to that part, with no read of its own', () => {
  const overview = readFileSync(new URL('../src/components/domain/sections/OverviewSection.tsx', import.meta.url), 'utf8');
  assert.match(overview, /const lead = siteLeadPart\(domain\.project_type\);/);
  assert.match(overview, /go\(runs && lead \? \{ section: 'site', part: lead \} : \{ section: section\.id \}\)/);
  assert.match(overview, /t\('domain\.overview\.siteRuns', \{ kind: siteKindWord\(t, domain\.project_type\) \}\)/);
  assert.doesNotMatch(overview, /useRemote|fetch\(/, 'the Overview reads nothing new');
  assert.equal(enScreens['domain.overview.siteRuns'], '{kind} runs here');
  assert.equal(trScreens['domain.overview.siteRuns'], '{kind} çalışıyor');
});
