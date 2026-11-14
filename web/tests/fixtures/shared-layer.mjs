import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';
import { kitTable } from './kit.mjs';

// The shared remote-state layer, compiled from source for a mounted test:
// lib/remote.ts, lib/hostingCapabilities.ts, lib/apiError.ts and the REAL
// components/ui.tsx. A screen under test imports these exactly as it does in
// the application, so "checking", "could not check" and the gate around an
// empty state are the shipped ones and not a stand-in that could agree with a
// broken screen.
//
// Everything else a screen imports (icons, the router, the catalogue, toasts)
// comes from the stub the test supplies. That stub must export `useI18n`,
// `useNavigate` and `AlertTriangle`, which ui.tsx itself needs.
//
// Paylaşılan uzak-durum katmanı, bağlanan bir test için kaynaktan derlenir.
// Test edilen ekran bunları uygulamadaki gibi içe aktarır; böylece "kontrol
// ediliyor", "kontrol edilemedi" ve boş durumun çevresindeki kapı, bozuk bir
// ekranla anlaşabilecek bir vekil değil, gönderilen kodun kendisidir.
const require = createRequire(import.meta.url);
export const reactURL = pathToFileURL(require.resolve('react')).href;
export const dataModule = (text) => 'data:text/javascript;base64,' + Buffer.from(text).toString('base64');

const compilerOptions = { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 };
const read = (path) => readFileSync(new URL('../../src/' + path, import.meta.url), 'utf8');

// compileSource turns one file under web/src into an importable module.
// `resolve` maps each of its import specifiers to a module URL.
// alpha.128: the lazily loaded parts of the limits screens (the site's PHP
// values, the plan's limits, the server's limits and the password level) are
// their own chunks; a screen that mounts one of them in a test gets a part
// that draws nothing, as the screens' other lazy parts get their stubs.
// alpha.128: sınır ekranlarının tembel parçaları testte hiçbir şey çizmez.
const alpha128LazyParts = {
  './SitePHPLimits': ['SitePHPLimits'],
  './PlanLimitsSection': ['PlanLimitsSection', 'CustomLimitsSection'],
  './ServerSettingLimits': ['ServerSettingLimits', 'PasswordLevelCard'],
  // alpha.130: the Offered tab's applications are their own chunk too.
  './OfferedAppsSection': ['OfferedAppsSection'],
  // alpha.130: the administrator's switch of connections from outside reads
  // its own address (its own chunk; on the Security page since alpha.133,
  // with CelikPanel's own account on the MariaDB and PostgreSQL pages).
  './DatabaseRemoteServerPanel': ['DatabaseRemoteServerPanel', 'EngineAccountBlock'],
  // alpha.134: database rights by exact name on the MariaDB page (its own
  // chunk and its own mounted tests).
  './DatabaseGrantFormsBlock': ['DatabaseGrantFormsBlock'],
};

export function compileSource(path, resolve) {
  let compiled = ts.transpileModule(read(path), { compilerOptions }).outputText
    .replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => `from '${specifier === 'react' ? reactURL : resolve(specifier)}'`);
  for (const [specifier, names] of Object.entries(alpha128LazyParts)) {
    const empty = dataModule(names.map((name) => `export const ${name} = () => null;`).join(' '));
    compiled = compiled.split(`import('${specifier}')`).join(`import('${empty}')`);
  }
  return dataModule(`import React from '${reactURL}';\n${compiled}`);
}

const unexpected = (file) => (specifier) => {
  throw new Error(`${file} imports ${specifier}, which the shared-layer fixture does not provide`);
};

export const apiErrorURL = compileSource('lib/apiError.ts', unexpected('lib/apiError.ts'));
export const remoteURL = compileSource('lib/remote.ts', (specifier) => (
  specifier.endsWith('/apiError') ? apiErrorURL : unexpected('lib/remote.ts')(specifier)
));
export const capabilitiesURL = compileSource('lib/hostingCapabilities.ts', (specifier) => (
  specifier.endsWith('/remote') ? remoteURL : unexpected('lib/hostingCapabilities.ts')(specifier)
));

// Since the fourth batch (9 Oct 2026) the shared layer also has the answer
// that did not arrive (lib/lostAnswer.ts), the one decoder of a domain's
// databases (lib/domainDatabases.ts) and the file download (lib/download.ts).
// They are the shipped modules for the same reason as the rest: a stand-in for
// "the result is unknown" could agree with a screen that sends twice.
// Dördüncü partiden beri paylaşılan katmanda, gelmeyen yanıt, bir alan adının
// veritabanlarının tek çözücüsü ve dosya indirme de vardır; hepsi gönderilen
// modüllerdir.
// The answer that did not arrive shares its definition of a lost answer, and
// the list of routes that carry an identity, with the request identity
// (D-029, 10 Oct 2026): the shipped module again.
// Gelmeyen yanıt, kaybolan yanıt tanımını ve kimlik taşıyan rotaların listesini
// istek kimliğiyle paylaşır: yine gönderilen modül.
export const requestIdentityURL = compileSource('lib/requestIdentity.ts', unexpected('lib/requestIdentity.ts'));
// alpha.127: a refusal said once, in its banner (lib/useRefusal.ts): the shipped hook.
export const useRefusalURL = compileSource('lib/useRefusal.ts', unexpected('lib/useRefusal.ts'));
export const lostAnswerURL = compileSource('lib/lostAnswer.ts', (specifier) => (
  specifier.endsWith('/requestIdentity') ? requestIdentityURL : unexpected('lib/lostAnswer.ts')(specifier)
));
// alpha.134 fix round: the length rule of a database create's names (the field's half).
export const databaseNameLimitsURL = compileSource('lib/databaseNameLimits.ts', unexpected('lib/databaseNameLimits.ts'));
export const domainDatabasesURL = compileSource('lib/domainDatabases.ts', (specifier) => (
  specifier.endsWith('/remote') ? remoteURL : unexpected('lib/domainDatabases.ts')(specifier)
));
export const downloadURL = compileSource('lib/download.ts', (specifier) => (
  specifier.endsWith('/apiError') ? apiErrorURL : unexpected('lib/download.ts')(specifier)
));

// sharedLayer returns the resolver a screen is compiled with: the real shared
// modules by their import suffix, `extra` for a test's own real modules, and
// the stub for the rest.
// The readers of one explained answer each (12 Oct 2026) are the shipped ones in
// every mounted test: a site the web server refused, and the note of a Stop
// that left the unit marked as failed. They import types only.
export const siteWebServerRefusedURL = compileSource('lib/siteWebServerRefused.ts', unexpected('lib/siteWebServerRefused.ts'));
export const serviceActionNoteURL = compileSource('lib/serviceActionNote.ts', unexpected('lib/serviceActionNote.ts'));

// The component census (lib/componentCensus.ts) is the shipped module: lib/managedServices.ts
// publishes into it after its conditional check (15 Oct 2026, D-032).
const componentCensusURL = compileSource('lib/componentCensus.ts', unexpected('lib/componentCensus.ts'));
// So is lib/visibleReads.ts, which the held check's resume uses (15 Oct 2026).
const visibleReadsURL = compileSource('lib/visibleReads.ts', unexpected('lib/visibleReads.ts'));
// The installed package version reader (alpha.97) has no imports of its own.
export const packageVersionURL = compileSource('lib/packageVersion.ts', unexpected('lib/packageVersion.ts'));
// One running rule for every screen that reads a component's status (alpha.98).
export const serviceStatusURL = compileSource('lib/serviceStatus.ts', unexpected('lib/serviceStatus.ts'));
// Which PHP version each PHP tool runs on, and who uses each version (alpha.104).
// It reads a lost answer the one way (alpha.104 review).
export const phpToolsURL = compileSource('lib/phpTools.ts', (specifier) => (
  specifier.endsWith('/requestIdentity') ? requestIdentityURL : unexpected('lib/phpTools.ts')(specifier)
));

// alpha.130: the database engines' own marks (an elephant and a sea lion),
// shared by the Components page and the domain's database cards. Compiled
// from source; its icon factory is a stand-in that draws nothing, like
// every stub icon.
export const engineIconsURL = compileSource('lib/engineIcons.ts', (specifier) => (
  specifier === 'lucide-react' ? dataModule('export const createLucideIcon = () => () => null;') : unexpected('lib/engineIcons.ts')(specifier)
));
// alpha.130: the components' own marks (componentMarks.ts), the same way.
export const componentMarksURL = compileSource('lib/componentMarks.ts', (specifier) => (
  specifier === 'lucide-react' ? dataModule('export const createLucideIcon = () => () => null;') : unexpected('lib/componentMarks.ts')(specifier)
));

// alpha.105: what an install brings, and the component's version reads.
export const installPreviewLibURL = compileSource('lib/installPreview.ts', unexpected('lib/installPreview.ts'));
export const componentRepoURL = compileSource('lib/componentRepo.ts', unexpected('lib/componentRepo.ts'));
// alpha.107: the webmail update check, read by its card and the Components row.
// alpha.110: webmail's own address, the only URL a screen may link to.
export const webmailAddressLibURL = compileSource('lib/webmailAddress.ts', unexpected('lib/webmailAddress.ts'));
export const roundcubeUpdateURL = compileSource('lib/roundcubeUpdate.ts', (specifier) => (
  specifier.endsWith('/webmailAddress') ? webmailAddressLibURL : unexpected('lib/roundcubeUpdate.ts')(specifier)
));
// alpha.107: the dated support table, read by every screen that marks a version.
export const supportTableURL = compileSource('lib/supportTable.ts', (specifier) => (
  specifier.endsWith('/remote') ? remoteURL : unexpected('lib/supportTable.ts')(specifier)
));
// alpha.108 slice d: the versions offered to customers (D-038 §14).
export const offeredVersionsURL = compileSource('lib/offeredVersions.ts', (specifier) => (
  specifier.endsWith('/remote') ? remoteURL : unexpected('lib/offeredVersions.ts')(specifier)
));

// alpha.108: the vendor lane and the tested marks.
export const vendorLaneURL = compileSource('lib/vendorLane.ts', unexpected('lib/vendorLane.ts'));
export const testedVersionsURL = compileSource('lib/testedVersions.ts', (specifier) => (
  specifier.endsWith('/supportTable') ? supportTableURL : unexpected('lib/testedVersions.ts')(specifier)
));

// The certificate Add domain asks for after the site exists: the shipped module,
// so a test that mounts the dialog sends the real request and reads the real
// answer rather than a stand-in.
export const siteCertificateRequestURL = compileSource('lib/siteCertificateRequest.ts', (specifier) => (
  specifier.endsWith('/apiError') ? apiErrorURL
    : specifier.endsWith('/lostAnswer') ? lostAnswerURL
      : unexpected('lib/siteCertificateRequest.ts')(specifier)
));

// alpha.118: remote backups' decoders and decisions; no imports of its own.
export const remoteBackupURL = compileSource('lib/remoteBackup.ts', unexpected('lib/remoteBackup.ts'));
export const securityEmergencyURL = compileSource('lib/securityEmergency.ts', unexpected('lib/securityEmergency.ts'));
// alpha.123: the server-side remove-together operation, followed by the removal window.
export const removalOperationURL = compileSource('lib/removalOperation.ts', unexpected('lib/removalOperation.ts'));
// alpha.124: what a component needs, read by the install window.
export const installPlanURL = compileSource('lib/installPlan.ts', unexpected('lib/installPlan.ts'));

// 6 Nov 2026: the product's own confirmation (lib/confirmAction.ts) is the
// shipped module. A mounted test has no ConfirmHost, so the fixture registers
// one that answers through `globalThis.confirm(text)` - the test's own answerer,
// a plain function the test sets; nothing here is the browser's dialogue. The
// text it is given is the request's message, or its title when it has none.
// A confirmed request's action runs before the answer, as in the dialogue.
export const confirmActionURL = compileSource('lib/confirmAction.ts', unexpected('lib/confirmAction.ts'));
const confirmActionModule = await import(confirmActionURL);
// The steps an action reports and how it ended are kept on
// globalThis.operationSteps / globalThis.operationEnds for the test to read; an
// operation followed without a question (followOperation) is not asked.
confirmActionModule.registerConfirmHost(async (request) => {
  if (!request.follow) {
    const answer = globalThis.confirm;
    if (typeof answer !== 'function') throw new Error('a confirmation was asked, and the test set no globalThis.confirm answerer');
    if (!answer(request.message ?? request.title ?? '', request)) return false;
  }
  if (request.action) {
    const end = await request.action({ steps: (steps) => { (globalThis.operationSteps ??= []).push(steps); } });
    (globalThis.operationEnds ??= []).push(end);
  }
  return true;
});

// alpha.127: the real steps of a change, read from the server while it runs
// (lib/requestProgress.ts): the shipped module, with the shipped confirmation,
// refusal reader and request identity.
export const requestProgressURL = compileSource('lib/requestProgress.ts', (specifier) => (
  specifier.endsWith('/apiError') ? apiErrorURL
    : specifier.endsWith('/confirmAction') ? confirmActionURL
      : specifier.endsWith('/requestIdentity') ? requestIdentityURL
        : unexpected('lib/requestProgress.ts')(specifier)
));

// A part of a screen loaded lazily (`import('./X')`) is given a stand-in by the
// test that mounts the screen: the compiled module, with that one import
// pointed at `replacementURL`. Any other lazy import is left as it is.
// Bir ekranın tembel yüklenen parçasına, ekranı bağlayan test bir vekil verir.
export function replaceLazyImport(moduleURL, specifier, replacementURL) {
  const source = Buffer.from(moduleURL.slice(moduleURL.indexOf(',') + 1), 'base64').toString();
  const quoted = specifier.replace(/[.*+?^${}()|[\]\\/]/g, '\\$&');
  const pattern = new RegExp(`import\\(['"]${quoted}['"]\\)`, 'g');
  const replaced = source.replace(pattern, `import('${replacementURL}')`);
  if (replaced === source) throw new Error(`${specifier} is not imported lazily`);
  return dataModule(replaced);
}

// 8 Nov 2026: every date and time follows the interface language through the one formatter (no imports of its own).
export const dateFormatURL = compileSource('lib/dateFormat.ts', unexpected('lib/dateFormat.ts'));
// alpha.130: the update sources in plain words (no imports of its own).
export const updateOriginsURL = compileSource('lib/updateOrigins.ts', unexpected('lib/updateOrigins.ts'));
// alpha.127: the owner's password rule, read by every password field and form.
export const passwordPolicyURL = compileSource('lib/passwordPolicy.ts', unexpected('lib/passwordPolicy.ts'));

export function sharedLayer(stubURL, extra = {}) {
  const uiURL = compileSource('components/ui.tsx', (specifier) => (
    specifier.endsWith('/apiError') ? apiErrorURL : stubURL
  ));
  // alpha.127: the command copy box is the shipped one; it needs only useI18n.
  const copyCommandURL = compileSource('components/CopyCommand.tsx', () => stubURL);
  const table = {
    '/lib/apiError': apiErrorURL,
    '/apiError': apiErrorURL,
    '/lib/remote': remoteURL,
    '/lib/hostingCapabilities': capabilitiesURL,
    '/lib/lostAnswer': lostAnswerURL,
    '/lib/siteCertificateRequest': siteCertificateRequestURL,
    '/lib/domainDatabases': domainDatabasesURL,
    '/lib/databaseNameLimits': databaseNameLimitsURL,
    '/lib/download': downloadURL,
    '/lib/siteWebServerRefused': siteWebServerRefusedURL,
    '/lib/serviceActionNote': serviceActionNoteURL,
    '/componentCensus': componentCensusURL,
    '/visibleReads': visibleReadsURL,
    '/lib/packageVersion': packageVersionURL,
    '/lib/serviceStatus': serviceStatusURL,
    '/lib/phpTools': phpToolsURL,
    '/lib/installPreview': installPreviewLibURL,
    '/lib/componentRepo': componentRepoURL,
    '/lib/roundcubeUpdate': roundcubeUpdateURL,
    '/lib/webmailAddress': webmailAddressLibURL,
    '/lib/supportTable': supportTableURL,
    '/lib/offeredVersions': offeredVersionsURL,
    '/lib/vendorLane': vendorLaneURL,
    '/lib/testedVersions': testedVersionsURL,
    '/lib/remoteBackup': remoteBackupURL,
    '/lib/securityEmergency': securityEmergencyURL,
    '/lib/removalOperation': removalOperationURL,
    '/lib/installPlan': installPlanURL,
    '/lib/confirmAction': confirmActionURL,
    '/lib/useRefusal': useRefusalURL,
    '/lib/requestProgress': requestProgressURL,
    // alpha.123: ServiceList sends the removal's own request identity header.
    '/lib/requestIdentity': requestIdentityURL,
    '/lib/passwordPolicy': passwordPolicyURL,
    '/engineIcons': engineIconsURL,
    '/componentMarks': componentMarksURL,
    '/lib/dateFormat': dateFormatURL,
    '/lib/updateOrigins': updateOriginsURL,
    '/ui': uiURL,
    '/CopyCommand': copyCommandURL,
    // alpha.132: the shared kit (row actions, guidance block, words) is the shipped code.
    ...kitTable(stubURL, uiURL),
    ...extra,
  };
  const resolve = (specifier) => {
    for (const [suffix, url] of Object.entries(table)) if (specifier.endsWith(suffix)) return url;
    return stubURL;
  };
  // alpha.132: the shared saving parts (components/kit/save: the draft, the
  // save bar, the leave guard and the structured confirmation) are the
  // shipped modules in every mounted test, so a screen's Save, Discard and
  // leave question are the real ones. Their icons come from their own stub.
  const kitIcons = dataModule(['CheckCircle2', 'XCircle'].map((name) => `export const ${name} = () => null;`).join(' '));
  for (const [suffix, file] of [['/leaveAsker', 'leaveAsker.ts'], ['/words', 'words.ts'], ['/saveRun', 'saveRun.ts'], ['/leaveGuard', 'leaveGuard.ts'],
    ['/draft', 'draft.ts'], ['/SaveBar', 'SaveBar.tsx'], ['/ConfirmDetails', 'ConfirmDetails.tsx']]) {
    if (!extra[suffix]) table[suffix] = compileSource(`components/kit/save/${file}`, (specifier) => (specifier === 'lucide-react' ? kitIcons : resolve(specifier)));
  }
  // alpha.127: every password field is the shipped PasswordInput with the
  // shipped rule. Its icons come from its own small stub, so a test's stub
  // need not list them.
  if (!extra['/PasswordInput']) {
    const icons = dataModule(['Check', 'Copy', 'Eye', 'EyeOff', 'Wand2'].map((name) => `export const ${name} = () => null;`).join(' '));
    table['/PasswordInput'] = compileSource('components/PasswordInput.tsx', (specifier) => (
      specifier === 'lucide-react' ? icons : resolve(specifier)
    ));
  }
  // The firewall box left ServiceList.tsx for its own module in alpha.98; a
  // screen that mounted it inside the Components page still gets the real one.
  if (!extra['/FirewallPanel']) table['/FirewallPanel'] = compileSource('components/FirewallPanel.tsx', resolve);
  if (!extra['/InstallPreviewNotes']) table['/InstallPreviewNotes'] = compileSource('components/InstallPreviewNotes.tsx', resolve);
  if (!extra['/SupportMark']) table['/SupportMark'] = compileSource('components/SupportMark.tsx', resolve);
  if (!extra['/LifeLine']) table['/LifeLine'] = compileSource('components/LifeLine.tsx', resolve);
  if (!extra['/RemovalTraces']) table['/RemovalTraces'] = compileSource('components/RemovalTraces.tsx', resolve);
  // alpha.123 (D-045): the removal plan's three groups and the install requirements.
  if (!extra['/RemovalPlan']) table['/RemovalPlan'] = compileSource('components/RemovalPlan.tsx', resolve);
  // alpha.124: the install window's requirements.
  if (!extra['/InstallRequirements']) table['/InstallRequirements'] = compileSource('components/InstallRequirements.tsx', resolve);
  if (!extra['/OfferedVersionsCard']) table['/OfferedVersionsCard'] = compileSource('components/OfferedVersionsCard.tsx', resolve);
  // alpha.108: the one confirmed webmail update flow, started by the webmail card and the Components row.
  if (!extra['/RoundcubeUpdateFlow']) table['/RoundcubeUpdateFlow'] = compileSource('components/RoundcubeUpdateFlow.tsx', resolve);
  if (!extra['/VendorLaneDialog']) table['/VendorLaneDialog'] = compileSource('components/VendorLaneDialog.tsx', resolve);
  if (!extra['/NodePatches']) table['/NodePatches'] = compileSource('components/NodePatches.tsx', resolve);
  // alpha.127: Node.js sites in the site sandbox (Components card, Hosting tab line).
  if (!extra['/NodeSandboxCard']) table['/NodeSandboxCard'] = compileSource('components/NodeSandboxCard.tsx', resolve);
  if (!extra['/NodeSiteSandbox']) table['/NodeSiteSandbox'] = compileSource('components/NodeSiteSandbox.tsx', resolve);
  // alpha.117 (D-043): the security emergency section of a component.
  if (!extra['/SecurityEmergency']) table['/SecurityEmergency'] = compileSource('components/SecurityEmergency.tsx', resolve);
  // alpha.114: the owner's automatic/manual choice and Arch's step-by-step guide.
  if (!extra['/OSUpdatesChoice']) table['/OSUpdatesChoice'] = compileSource('components/OSUpdatesChoice.tsx', resolve);
  if (!extra['/ArchUpdateGuide']) table['/ArchUpdateGuide'] = compileSource('components/ArchUpdateGuide.tsx', resolve);
  // The release after alpha.133: what a site backup holds, and what a restore did with the program.
  if (!extra['/lib/backupContents']) table['/lib/backupContents'] = compileSource('lib/backupContents.ts', resolve);
  if (!extra['/BackupContentsNote']) table['/BackupContentsNote'] = compileSource('components/BackupContentsNote.tsx', resolve);
  return { uiURL, resolve, compile: (path, more = {}) => quietLazyParts(compileSource(path, (specifier) => {
    for (const [suffix, url] of Object.entries(more)) if (specifier.endsWith(suffix)) return url;
    return resolve(specifier);
  })) };
}

// alpha.128 (G15): the API tokens card (My account) and the remote API switch
// (Settings) are their own lazy chunk. A screen mounted for another reason
// gets a quiet stand-in for it, so its tests stay about that screen; the
// card itself is mounted in tests/api-tokens-alpha128.test.mjs.
// API jetonları kartı ve anahtarı kendi tembel paketidir; başka bir ekran
// bağlanırken sessiz bir vekil alır.
const apiTokensStandInURL = dataModule('export const ApiTokensCard = () => null; export const RemoteApiSwitch = () => null;');
function quietLazyParts(moduleURL) {
  const source = Buffer.from(moduleURL.slice(moduleURL.indexOf(',') + 1), 'base64').toString();
  if (!/import\(['"]\.\/ApiTokens['"]\)/.test(source)) return moduleURL;
  return dataModule(source.replace(/import\(['"]\.\/ApiTokens['"]\)/g, `import('${apiTokensStandInURL}')`));
}
