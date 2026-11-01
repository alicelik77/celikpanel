import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { enScreens, trScreens } from './locale-catalogue.mjs';
import { enSetupDNS } from '../src/i18n/setupDNS/en.ts';
import { trSetupDNS } from '../src/i18n/setupDNS/tr.ts';

// alpha.114, the lab screenshot review (31 October 2026): the screen findings
// S1 to S24 that need no server. Each block names its finding; the lab run
// proves the same screens again on real servers.
// alpha.114 laboratuvar ekran incelemesi: sunucu gerektirmeyen ekran bulgulari.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const dataURL = (text) => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const compile = (path) => ts.transpileModule(read(path), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const src = (name) => read(`../src/components/${name}.tsx`);
const wizard = src('ServerSetup');
const steps = src('ServerSetupWizardSteps');
const en = enScreens;
const tr = trScreens;

const bothHave = (keys) => {
  for (const key of keys) {
    assert.ok(typeof en[key] === 'string' && en[key] !== '', `EN ${key}`);
    assert.ok(typeof tr[key] === 'string' && tr[key] !== '', `TR ${key}`);
  }
};

test('S24: every setup step kind has text in both languages, in the catalogue the Components dialog reads', () => {
  const kinds = [...read('../src/lib/serverSetupOperation.ts').match(/const kinds = \[([^\]]+)\]/)[1].matchAll(/'([a-z_]+)'/g)].map((match) => match[1]);
  assert.ok(kinds.includes('access_dns') && kinds.includes('infrastructure_dns'), 'the list of kinds is read');
  for (const kind of kinds) {
    const key = `setup.kind.${kind}`;
    // The Components dialog uses the plain catalogue (no setup-page overlay).
    assert.ok(en[key], `EN ${key}`);
    assert.ok(tr[key], `TR ${key}`);
    // The setup page's own overlay, when it has the key, says the same.
    if (enSetupDNS[key]) assert.equal(en[key], enSetupDNS[key], `${key} EN agrees with the setup page`);
    if (trSetupDNS[key]) assert.equal(tr[key], trSetupDNS[key], `${key} TR agrees with the setup page`);
  }
  // No raw key can come from a step the dialog draws.
  const used = new Set([...src('MailSetupDialog').matchAll(/setup\.kind\.\$\{/g)]);
  assert.ok(used.size > 0, 'the dialog builds the key from the step kind');
});

test('S1: the roles question reads "this server" in lower case in English; Turkish keeps the capital', () => {
  assert.match(steps, /const titleServer = locale === 'en' \? server\.charAt\(0\)\.toLowerCase\(\) \+ server\.slice\(1\) : server;/);
  assert.match(steps, /t\('setup\.v2\.roles\.title', \{ server: titleServer \}\)/);
  assert.equal(en['setup.v2.roles.title'].replace('{server}', 'this server (x)'), 'What will this server (x) do?');
});

test('S2: the phone step strip wraps inside the page and never sets a width of its own', () => {
  const strip = src('ServerSetupSteps');
  assert.match(strip, /mobile \? 'flex min-w-0 flex-wrap gap-1\.5'/);
  assert.doesNotMatch(strip, /flex-1 justify-center/, 'ten flexible cells no longer force the page wider');
});

test('S3, S12: one way to host the main-domain website; "Add a website" once one exists', () => {
  // The card is the one action; the sentence and the bottom button do not repeat it.
  assert.match(wizard, /readyDraft\.main_site === 'elsewhere' && readyDomain && !firstSiteWanted && <p/);
  assert.match(wizard, /offerFirstSite \? 'setup\.nextDashboard' : readyDraft\.main_site === 'here' \|\| readySiteDone \? 'setup\.nextWebsiteMore' : 'setup\.nextWebsite'/);
  assert.match(wizard, /const readyNextPath = offerFirstSite && nextPath === '\/domains' && !isNode \? '\/' : nextPath;/);
  assert.match(wizard, /<Link to=\{readyNextPath\}/);
  bothHave(['setup.nextWebsiteMore', 'setup.nextDashboard', 'dnsov.site.title', 'dnsov.site.add']);
  assert.equal(en['dnsov.site.add'], 'Host a website');
  assert.equal(en['setup.nextWebsiteMore'], 'Add a website');
  assert.equal(tr['dnsov.site.add'], tr['domains.infrastructure.hostWebsite'], 'one name in Turkish too');
  assert.equal(en['dnsov.site.add'], en['domains.infrastructure.hostWebsite']);
});

test('S13: the ready headline counts the open items shown, the joined server included', () => {
  assert.match(wizard, /const readyCount = readyTasks\.length \+ \(readyPeerPending \? 1 : 0\);/);
  assert.match(wizard, /<SetupJoinCard joinPending=\{false\} finished onPending=\{setReadyPeerPending\} \/>/);
  assert.match(steps, /const peerSetupPending = finished && view\?\.peer_setup === 'pending';/);
  assert.match(steps, /useEffect\(\(\) => \{ onPending\?\.\(peerSetupPending\); \}, \[onPending, peerSetupPending\]\);/);
  assert.doesNotMatch(wizard, /count: readyTasks\.length/);
});

test('S4, S5: the Dashboard says a domain has no website yet, with one button; "Components" is one word', () => {
  const dashboard = src('Dashboard');
  assert.match(dashboard, /standaloneZoneNames\.map\(\(zone\) => \(/);
  assert.match(dashboard, /navigate\(`\/domains\?host=\$\{encodeURIComponent\(zone\)\}`\)/);
  bothHave(['dashboard.noWebsite.title', 'dashboard.noWebsite.help', 'dashboard.noWebsite.action', 'dashboard.installService', 'dashboard.openComponents']);
  assert.equal(en['dashboard.noWebsite.title'].replace('{domain}', 'example.test'), 'example.test has no website yet');
  assert.equal(en['dashboard.noWebsite.action'], 'Host a website');
  assert.equal(en['dashboard.installService'], en['nav.services'] ?? 'Components');
  assert.equal(tr['dashboard.installService'], 'Bileşenler');
  assert.doesNotMatch(dashboard, /t\('dashboard\.installService'\)/, 'the attention items name the page, not "Install a service"');
  // Domains opens the dialog once for that name and cleans the address.
  const domains = src('Domains');
  assert.match(domains, /const hostParam = searchParams\.get\('host'\);/);
  assert.match(domains, /openAdd\(hostParam\);/);
  assert.match(domains, /next\.delete\('host'\);/);
});

test('S6: a website for a name whose DNS is served here starts with the certificate box ticked', () => {
  const modal = src('AddDomainModal');
  assert.match(modal, /sslByDefault = false/);
  assert.match(modal, /const sslEnabled = sslChoice \?\? sslByDefault;/);
  assert.match(modal, /onChange=\{\(e\) => setSSLChoice\(e\.target\.checked\)\}/);
  assert.match(src('Domains'), /sslByDefault=\{addServedHere\}/);
  assert.match(src('InfrastructureZone'), /initialDomain=\{zone\}\s+sslByDefault/);
  assert.match(src('SetupFirstSite'), /<AddDomainModal initialDomain=\{domain\} sslByDefault/);
});

test('S7, S20: the administrator\'s own sites card has a plain name and no limit; other accounts are named', async () => {
  const domains = src('Domains');
  assert.match(domains, /subscriptionTitle\(s, viewer, t\)/);
  assert.match(domains, /s\.owner && s\.owner !== viewer && <p/);
  assert.match(domains, /t\('quota\.domainsFree', \{ n: u\.domains \}\)/);
  bothHave(['quota.domainsFree', 'quota.databasesFree', 'quota.mailFree', 'quota.ownWorkspace', 'quota.accountOf', 'users.poolDomains', 'users.poolDomainsUnlimited', 'users.poolDisk', 'users.poolDiskUnlimited']);
  assert.doesNotMatch(read('../src/components/UsersPage.tsx'), /'∞'/, 'the pool line says "without a limit" in words');
  // The title function, as the page uses it.
  const title = domains.match(/export function subscriptionTitle[\s\S]*?\n}\n/)[0].replace(/: \{ name: string; owner: string \}/, '').replace(/: string \| undefined/, '').replace(/: \(key: TranslationKey\) => string/, '').replace(/\): string \{/, ') {').replace('export function', 'function');
  const subscriptionTitle = new Function(`${title}; return subscriptionTitle;`)();
  const say = (key) => en[key];
  assert.equal(subscriptionTitle({ name: 'Admin Subscription', owner: 'owner' }, 'owner', say), 'Your own sites');
  assert.equal(subscriptionTitle({ name: 'basic', owner: 'cust1' }, 'owner', say), 'basic');
});

test('S8: dialog titles use sentence case', () => {
  assert.equal(en['domains.add.title'], 'Add domain');
  assert.equal(tr['domains.add.title'], 'Alan adı ekle');
});

test('S10, S15: one example for server names, built from the panel name', () => {
  assert.match(en['setup.v2.names.mailHelp'], /mail\.boston\.\{domain\}/);
  assert.doesNotMatch(en['setup.v2.names.mailHelp'] + tr['setup.v2.names.mailHelp'], /mail2/);
  assert.match(steps, /example: `\$\{shortName \|\| 'panel'\}\.\$\{value \|\| 'example\.com'\}`/);
  assert.match(wizard, /<SetupDomainStep shortName=\{draft\.short_name\}/);
});

test('S11: a refusal the guidance already says is not printed a second time', () => {
  assert.match(wizard, /const failureAlreadySaid = !!execution\?\.error && !!guidance && guidance\.messages\.some\(message => say\(message\) === failureText\(/);
  assert.match(wizard, /!hostBusy && !peerRecordApplied && !failureAlreadySaid &&/);
});

test('S14: the joined path hides "Number of servers"', () => {
  assert.match(wizard, /\['domain', \.\.\.\(joinedLink \|\| kind === 'join' \? \[\] : \['count' as const\]\)/);
});

test('S16, S17: the panel address is shown with "Change"; the port trial opens by the panel name', async () => {
  const settings = src('Settings');
  assert.match(settings, /data-panel-current-address=\{currentName\}/);
  assert.match(settings, /const showForm = currentName === '' \|\| changing \|\| pendingOperation !== null \|\| outcome\.state !== 'none';/);
  bothHave(['panelCert.current', 'panelCert.openCurrent', 'panelCert.change']);
  const certificate = await import(dataURL(compile('../src/lib/panelCertificate.ts')));
  const trusted = { https_enabled: true, self_signed: false, subject: 'arch.example.test', dns_names: ['arch.example.test'] };
  assert.equal(certificate.panelCertificateName(trusted), 'arch.example.test');
  assert.equal(certificate.panelCertificateName({ ...trusted, dns_names: ['*.example.test', '192.0.2.5', 'Panel.Example.Test'] }), 'panel.example.test');
  assert.equal(certificate.panelCertificateName({ ...trusted, self_signed: true }), '', 'a self-signed certificate is no panel name');
  assert.equal(certificate.panelCertificateName({ ...trusted, https_enabled: false }), '');
  assert.equal(certificate.panelCertificateName(undefined), '');
  assert.equal(certificate.decodePanelCertificate({ https_enabled: true, self_signed: false, dns_names: ['a.b', 3] }).dns_names.length, 1);
  const port = await import(dataURL(compile('../src/lib/panelPort.ts')));
  const here = { protocol: 'https:', hostname: '192.0.3.152' };
  assert.equal(port.panelAddressOn(2089, here, 'arch.example.test'), 'https://arch.example.test:2089/settings?section=panel');
  assert.equal(port.panelAddressOn(2089, here), 'https://192.0.3.152:2089/settings?section=panel', 'no trusted name: the address, as before');
  assert.equal(port.panelAddressOn(2089, { protocol: 'https:', hostname: 'other.example.test' }, 'arch.example.test'), 'https://other.example.test:2089/settings?section=panel', 'already opened by a name: that name');
  assert.match(src('PanelPortCard'), /panelAddressOn\(change\.to_port, window\.location, trustedName\)/);
});

test('S18: an additional user reads "Additional user of <owner>"', () => {
  const users = src('UsersPage');
  assert.match(users, /u\.account_type === 'additional_user' && u\.parent_name \? t\('users\.role\.additionalOf', \{ owner: u\.parent_name \}\)/);
  bothHave(['users.role.additionalOf']);
  assert.equal(en['users.role.additionalOf'].replace('{owner}', 'cust1'), 'Additional user of cust1');
});

test('S19: the Components version line is the version; where it was read from stays in the tooltip', () => {
  const list = src('ServiceList');
  assert.match(list, /<span className="sr-only">\{` \(\$\{facts\.full\}\)`\}<\/span>/);
  assert.match(list, /title=\{`\$\{facts\.full\} · \$\{source\}`\}/);
});

test('S21: a reseller reads the pool first, and that a plan is only for their own sites', () => {
  const card = src('MyPlanCard');
  assert.ok(card.indexOf('{plan.pool && <PoolBlock') < card.indexOf("t(plan.pool ? 'myPlan.noneReseller' : 'myPlan.none')"));
  bothHave(['myPlan.noneReseller', 'myPlan.none']);
  assert.match(en['myPlan.noneReseller'], /without a plan of your own/);
});

test('S22: an invitation to a new account does not use the reset wording', () => {
  assert.match(src('AccountLinkPage'), /t\(purpose === 'invitation' \? 'acct\.link\.hintInvitation' : 'acct\.link\.hint'\)/);
  bothHave(['acct.link.hintInvitation']);
  assert.doesNotMatch(en['acct.link.hintInvitation'] + tr['acct.link.hintInvitation'], /signed out|oturumları kapan/);
});

test('S23: the forgot-password page shows the real command to copy', () => {
  const page = src('ForgotPassword');
  const command = page.match(/OWNER_RESET_COMMAND = '([^']+)'/)[1];
  assert.match(page, /<OwnerCommand \/>/);
  bothHave(['acct.forgot.copy', 'acct.forgot.copied', 'acct.forgot.owner']);
  assert.doesNotMatch(en['acct.forgot.owner'], /create-administrator/, 'no internal command name');
  // The command is built from what the installer and the panel really use.
  const install = read('../../install.sh');
  assert.match(install, /^PREFIX=\/opt\/celikpanel$/m);
  assert.match(install, /^DATA_DIR=\/var\/lib\/celikpanel$/m);
  assert.match(install, /^SVC_USER=celikpanel$/m);
  assert.match(read('../../cmd/panel/main.go'), /flag\.Bool\("create-admin"/);
  assert.match(read('../../cmd/panel/config.go'), /CELIKPANEL_DATA_DIR/);
  assert.equal(command, 'sudo -u celikpanel env CELIKPANEL_DATA_DIR=/var/lib/celikpanel /opt/celikpanel/bin/panel --create-admin');
});
