import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { enScreens, trScreens } from './locale-catalogue.mjs';

// alpha.114, the owner's review of 31 October 2026: the main domain becomes a
// website at setup (asked on the first server only), the ready screen leads
// with what is left, a wizard opened for a purpose says why, an extension
// shows the DNS software the server runs as one fixed line, and mail is added
// from Components in place.
// Sahibin 31 Ekim incelemesi: ana alan adi web sitesi, kalan isler, amac
// bandi, sabit DNS yazilimi satiri ve yerinde posta.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const dataURL = (text) => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const compile = (path) => ts.transpileModule(read(path), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const setupURL = dataURL(compile('../src/lib/serverSetup.ts'));
const setup = await import(setupURL);
const operations = await import(dataURL(compile('../src/lib/serverSetupOperation.ts').replace("from './serverSetup'", `from '${setupURL}'`)));
const wizard = read('../src/components/ServerSetup.tsx');
const steps = read('../src/components/ServerSetupWizardSteps.tsx');
const dialog = read('../src/components/MailSetupDialog.tsx');
const components = read('../src/components/ServiceList.tsx');
const overview = read('../src/components/DNSOverview.tsx');

const snapshot = (more = {}) => ({ version: 1, revision: 3, origin: 'fresh', status: 'draft', required: true, checks: [],
  draft: { purpose: 'web', panel_domain: 'panel.example.com', mail_hostname: '', dns_mode: 'local', remote_dns_connection_id: '', dns_engine: 'bind', dns_role: 'primary', ns1: '', ns2: '', local_ip: '', peer_ip: '', peer_ns: '', node_version: '', database: 'mariadb', main_domain: 'example.com', ...more } });

test('the main domain website answer is part of the draft contract, and only its two values', () => {
  assert.equal(setup.decodeServerSetup(snapshot({ main_site: 'here' }))?.draft.main_site, 'here');
  assert.equal(setup.decodeServerSetup(snapshot({ main_site: 'elsewhere' }))?.draft.main_site, 'elsewhere');
  assert.equal(setup.decodeServerSetup(snapshot({ main_site: 'maybe' })), null);
  assert.ok(setup.decodeServerSetup(snapshot()), 'an older draft without the answer stays readable');
});

test('a mail-only extension is said by the setup answer, and only on an extension', () => {
  const base = snapshot();
  const extension = { ...base, status: 'draft', required: false, extension: true, extension_base: base.draft, extension_scope: 'mail' };
  assert.equal(setup.decodeServerSetup(extension)?.extension_scope, 'mail');
  assert.equal(setup.decodeServerSetup({ ...base, extension_scope: 'mail' }), null, 'a first setup is never a mail-only extension');
  assert.equal(setup.decodeServerSetup({ ...extension, extension_scope: 'web' }), null);
});

test('the main-domain website step is a known step and its outcome is read fail-closed', () => {
  const id = 'a'.repeat(32);
  const step = (main_site) => ({ id: '09-main_site', kind: 'main_site', target: 'example.com', status: 'succeeded', main_site });
  const run = (outcome) => ({ id, request_id: id, plan_id: 'b'.repeat(32), status: 'succeeded', phase: 'complete', steps: [step(outcome), { id: '10-verify', kind: 'verify', target: 'web', status: 'succeeded' }] });
  const decoded = operations.decodeSetupExecution(run({ domain: 'example.com', domain_id: 4, site: 'created', certificate: 'pending', code: 'certificate_issue_failed', message: 'The CA could not reach example.com.' }));
  assert.ok(decoded, 'a run with the new step is read');
  assert.deepEqual(operations.setupMainSiteOutcome(decoded), { domain: 'example.com', domain_id: 4, site: 'created', certificate: 'pending', code: 'certificate_issue_failed', message: 'The CA could not reach example.com.' });
  assert.equal(operations.setupMainSiteOutcome(operations.decodeSetupExecution(run({ domain: 'example.com', site: 'invented', certificate: 'issued' }))), null);
  assert.equal(operations.setupMainSiteOutcome(null), null);
});

test('the wizard asks about the main domain website on the first server only, Yes by default', () => {
  // Asked when this server hosts websites and keeps (or does not own) the main domain's DNS; never on a joined server.
  assert.match(wizard, /const mainSiteAsked = !joinedLink && roles\.web && validSetupDomain\(mainDomain\) && \(draft\.dns_mode !== 'local' \|\| draft\.dns_role === 'primary'\);/);
  assert.match(wizard, /const mainSiteAnswer: 'here' \| 'elsewhere' = draft\.main_site \?\? 'here';/);
  assert.match(wizard, /main_site: mainSiteAsked \? mainSiteAnswer : undefined/);
  assert.match(wizard, /\{mainSiteAsked && <SetupMainSiteQuestion domain=\{mainDomain\} value=\{mainSiteAnswer\}/);
  assert.match(steps, /value="here" checked=\{value === 'here'\}[^\n]*tag=\{t\('setup\.v7\.site\.here\.tag'\)\}/);
  // The summary says plainly what happens to the main domain for both answers.
  assert.match(wizard, /mainSiteAsked && mainSiteAnswer === 'here' && <li data-setup-review-site="here">\{t\('setup\.v7\.review\.siteHere'/);
  assert.match(wizard, /t\('setup\.v7\.review\.siteElsewhere'/);
  assert.doesNotMatch(wizard, /setup\.v2\.review\.site'/, 'the old "no record imported" sentence is gone');
  for (const catalogue of [enScreens, trScreens]) {
    for (const key of ['setup.v7.site.title', 'setup.v7.site.here', 'setup.v7.site.here.help', 'setup.v7.site.elsewhere', 'setup.v7.site.elsewhere.help', 'setup.v7.review.siteHere', 'setup.v7.review.siteElsewhere', 'setup.kind.main_site']) {
      assert.ok(catalogue[key], key);
    }
  }
  assert.match(enScreens['setup.v7.site.elsewhere.help'], /will only keep its DNS records here; its website is elsewhere\. You can add the website later with “Host a website”/);
});

test('the ready screen leads with what is left for the owner, each with its action', () => {
  assert.match(wizard, /readyCount === 0 \? t\('setup\.v2\.ready'/);
  assert.match(wizard, /t\(readyCount === 1 \? 'setup\.v7\.ready\.tasksOne' : 'setup\.v7\.ready\.tasksMany'/);
  // The list is drawn right under the title, before the join card and the first-site offer.
  const ready = wizard.slice(wizard.indexOf('{completed ? <section'));
  assert.ok(ready.indexOf('data-setup-ready-list') < ready.indexOf('<SetupJoinCard') && ready.indexOf('data-setup-ready-list') < ready.indexOf('<SetupFirstSite'));
  assert.match(wizard, /id: 'registrar', text: t\('setup\.v7\.task\.registrar'/);
  assert.match(wizard, /to: `\$\{domainPage\}\?tab=ssl`, label: 'setup\.v7\.action\.ssl'/);
  assert.match(enScreens['setup.v7.ready.tasksMany'], /\{server\} is ready, with \{count\} things left for you/);
  assert.match(trScreens['setup.v7.ready.tasksMany'], /\{server\} hazır; size \{count\} iş kaldı/);
});

test('a wizard opened for a purpose says why at the top, and only those steps change', () => {
  assert.match(wizard, /const purpose: 'mail' \| 'dkim' \| null = urlPurpose \|\| \(snapshot\.extension_scope === 'mail' \? 'mail' : null\);/);
  assert.match(wizard, /data-setup-purpose=\{purpose\}/);
  for (const key of ['setup.v7.purpose.mail', 'setup.v7.purpose.dkim']) {
    assert.match(enScreens[key], /^You are here to .+; only the mail steps change\.$/);
    assert.ok(trScreens[key]);
  }
  // The pages that sent the owner to setup with no reason now name the purpose, or open the in-place dialog.
  assert.doesNotMatch(components, /navigate\('\/setup'\)/);
  assert.match(components, /navigate\('\/setup\?purpose=mail'\)/);
  assert.match(overview, /<Link to="\/setup\?purpose=dkim"/);
  assert.match(overview, /<Suspense fallback={null}><MailSetupDialog onClose=/);
});

test('an extension shows the DNS software the server runs as one fixed line', () => {
  assert.match(steps, /const running = setupActiveEngine\(catalog, draft\.dns_role\);/);
  assert.match(steps, /const software = running \? <p [^>]*data-setup-dns-engine-fixed=\{running\}>\{t\('setup\.v7\.engine\.fixed'/);
  assert.equal(enScreens['setup.v7.engine.fixed'], 'DNS software: {engine}, already running; it is not changed here.');
  // The Existing records note is said only where that step exists.
  assert.match(steps, /\{web && mainDomain && \(joined \|\| recordsStep\) && /);
  assert.match(wizard, /recordsStep=\{steps\.includes\('records'\)\}/);
});

test('the Components mail dialog asks the mail name only and runs a mail-only extension in place', () => {
  assert.match(dialog, /post\(\{ revision: snapshot\.revision, action: 'begin', scope: 'mail'/);
  assert.match(dialog, /fetch\('\/api\/v1\/setup\/start'/);
  assert.doesNotMatch(dialog, /navigate\('\/setup'\)/);
  assert.match(dialog, /setupJoinedMailProposal\(general, draft\?\.panel_domain \|\| ''\)\.mail/);
  assert.match(dialog, /setupJoinedMailTaken\(name, joinedNames\)/);
  assert.match(components, /\{profileTarget && mailThroughSetup && \(/);
  assert.doesNotMatch(components, /addProfileThroughSetup/);
  for (const catalogue of [enScreens, trScreens]) {
    for (const key of ['svc.mailSetup.title', 'svc.mailSetup.hostname', 'svc.mailSetup.hostnameTaken', 'svc.mailSetup.stepsTitle', 'svc.mailSetup.confirm', 'svc.mailSetup.running', 'svc.mailSetup.done', 'svc.mailSetup.failed', 'svc.mailSetup.failedNext']) assert.ok(catalogue[key], key);
  }
});

// Integration of the owner-setup and owner-UI work (alpha.114): the in-place
// mail run tells every screen that shows components, mail or DNS records, once,
// when it ends (reads only); the setup's first site and Domains share one view
// of the names that already point elsewhere; the ready screen names the button
// Domains really shows.
// Yerinde posta bitince bilesen/posta/DNS okumalari bir kez yenilenir; ilk site
// ile Alan adlari ayni "baska yere yonelen adlar" gorunumunu kullanir.
test('the mail run re-reads every screen it changed, and the first site shares the Domains view', () => {
  assert.match(dialog, /import \{ invalidate, readRemote, useRemote \} from '\.\.\/lib\/remote'/);
  assert.match(dialog, /if \(\(next\.status === 'succeeded' \|\| next\.status === 'failed'\) && !announced\.current\) \{\s*announced\.current = true;\s*invalidate\('components', 'mail', 'dns'\);/);
  const firstSite = read('../src/components/SetupFirstSite.tsx');
  assert.match(firstSite, /<KeptSiteRecords domain=\{domain\} outcome=\{kept\} \/>/);
  assert.doesNotMatch(firstSite, /dnsov\.site\.keptWhy/, 'one copy of the kept-names view');
  assert.match(read('../src/components/Domains.tsx'), /t\('domains\.infrastructure\.hostWebsite'\)/);
  for (const [catalogue, label] of [[enScreens, 'domains.infrastructure.hostWebsite'], [trScreens, 'domains.infrastructure.hostWebsite']]) {
    for (const key of ['setup.v7.site.elsewhere.help', 'setup.v7.site.failed']) assert.ok(catalogue[key].includes(catalogue[label]), `${key} names “${catalogue[label]}”`);
  }
});
