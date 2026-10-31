import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';

const require = createRequire(import.meta.url);
const dataModule = text => 'data:text/javascript;base64,' + Buffer.from(text).toString('base64');
const reactURL = pathToFileURL(require.resolve('react')).href;
const accessURL = dataModule(ts.transpileModule(readFileSync(new URL('../src/lib/accessObservation.ts',import.meta.url),'utf8'), {compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2020}}).outputText);
const offlineURL = dataModule(ts.transpileModule(readFileSync(new URL('../src/lib/licenseOffline.ts',import.meta.url),'utf8'), {compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2020}}).outputText);
const stub = dataModule(`
  import React from '${reactURL}';
  export const useAuth = () => globalThis.licenseTest.auth;
  export const useNavigate = () => globalThis.licenseTest.navigate;
  export const useLocation = () => ({ pathname: globalThis.licenseTest.pathname || '/domains' });
  export const useSearchParams = () => [new URLSearchParams(globalThis.licenseTest.search)];
  export const useI18n = () => ({ t: key => key, locale: 'en', screensReady: true, screensFailed: false });
 export const PanelAddressHint = () => null;
 export const RecoveryStatus = () => null;
 export const RecoveryAccess = props => React.createElement('aside', {failed:true,onCheck:props.onRetry,cause:props.cause,licenseReason:props.licenseReason}, 'recovery');
 export const BrandMark = () => null, LanguageSwitcher = () => null, ThemeSwitcher = () => null, ChangePasswordModal = () => null, ToastContainer = () => null;
 export const LicensePanel = () => React.createElement('section', null, 'activation');
 export const PanelUpdateCard = props => React.createElement('article', props, 'signed-update');
 export const Spinner = () => React.createElement('span', null, 'loading');
 export const LicenseLockScreen = props => React.createElement('aside', props, 'locked');
  export const Button = props => React.createElement('button', props);
  export const inputClass = '';
  export const BadgeCheck = () => null, Eye = () => null, EyeOff = () => null;
  export const readApiError = async response => { const data = await response.json(); return { message: data.error, code: data.code }; };
  export const apiErrorText = error => error.message;
  export const createPortal = node => node;
  export const Dialog = props => React.createElement('dialog', { id: props.id }, props.title, props.description, props.children, props.actions);
  export const usePanelHandover = () => null, handoverAddress = () => '', useAccessGuidance = () => null;
  export const AddressLink = props => React.createElement('a', { href: props.href }, props.address);
  // The first-read quiet time and the update hint have their own tests (access-hold-runtime).
  export const useQuietRead = () => false, savedUpdateUnfinished = () => false, UPDATE_MARKER_KEY = 'celikpanel.system-update-operation.v1';
`);
// The followed update's restart signal and its window are the real ones (set17 D1).
const compileTs = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), {compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2020}}).outputText;
const trackingURL = dataModule(compileTs('../src/lib/systemUpdateTracking.ts'));
const restartURL = dataModule(compileTs('../src/lib/systemUpdateRestartSignal.ts')
  .replace(/from ['"]react['"]/, `from '${reactURL}'`).replace(/from ['"]\.\/systemUpdateTracking['"]/, `from '${trackingURL}'`));
const watchdogURL = dataModule(compileTs('../src/lib/systemUpdateWatchdog.ts'));
const leaseURL = dataModule(compileTs('../src/lib/useSystemUpdateNavigationLease.ts')
  .replace(/from ['"]react['"]/, `from '${reactURL}'`).replace(/from ['"]\.\/systemUpdateWatchdog['"]/, `from '${watchdogURL}'`));
const holdURL = dataModule(`import React from '${reactURL}';\n` + ts.transpileModule(readFileSync(new URL('../src/components/AccessHold.tsx', import.meta.url), 'utf8'), { compilerOptions: {
  jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020,
}}).outputText.replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => `from '${specifier === 'react' ? reactURL : stub}'`));
async function component(name) {
  const source = readFileSync(new URL(`../src/components/${name}.tsx`, import.meta.url), 'utf8');
  const compiled = ts.transpileModule(source, { compilerOptions: {
    jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020,
  }}).outputText.replace(/import\('\.\/LicenseLockScreen'\)/g, `import('${stub}')`).replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => `from '${specifier === 'react' ? reactURL : specifier.endsWith('/accessObservation') ? accessURL : specifier.endsWith('/licenseOffline') ? offlineURL : specifier.endsWith('/AccessHold') ? holdURL : specifier.endsWith('/systemUpdateRestartSignal') ? restartURL : stub}'`);
  return (await import(dataModule(`import React from '${reactURL}';\n${compiled}`)))[name];
}
const LicenseOnboarding = await component('LicenseOnboarding');
const LicensePanel = await component('LicensePanel');
const LicenseLockScreen = await component('LicenseLockScreen');
const originalFetch = globalThis.fetch;
const events = new EventTarget();
globalThis.window = Object.assign(events, { setTimeout, clearTimeout, setInterval, clearInterval });
globalThis.document = Object.assign(new EventTarget(), { visibilityState: 'visible' });
let calls, navigations, tree;
// Management stays mounted behind a hold: the pages are inert and the layer explains the state.
const heldPages = () => tree.root.findAllByProps({ 'data-access-hold': 'blocked' }).filter(node => typeof node.type === 'string');
const holdLayer = () => tree.root.findAllByType('dialog');
async function showTab(state) { document.visibilityState = state; await act(async () => document.dispatchEvent(new Event('visibilitychange'))); }
function fixture(role = 'admin', state = 'missing') {
  calls = []; navigations = []; tree = undefined;
  globalThis.licenseTest = {
    auth: { role, user: { username: role } }, search: 'setup=1',
    navigate: (...args) => { navigations.push(args); return true; },
  };
  globalThis.fetch = async (url, options) => {
    calls.push({ url, options });
    return Response.json(url.includes('/license/access') ? { state, observation: state.endsWith('unavailable') ? 'unavailable' : 'known', can_use_panel: state === 'active', valid_until: state === 'active' ? Math.floor(Date.now()/1000)+3600 : 0 } : { state, can_provision: state === 'active' });
  };
}
async function mount(Component) { await act(async () => { tree = Renderer.create(React.createElement(Component)); }); }
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  globalThis.fetch = originalFetch;
  delete globalThis.licenseTest;
}
const button = label => tree.root.findAllByType('button').find(node => node.props.children === label);
const input = () => tree.root.findByType('input');
async function enter(value) { await act(async () => input().props.onChange({ target: { value } })); }
async function submit() { await act(async () => tree.root.findByType('form').props.onSubmit({ preventDefault() {} })); }

test('all roles are locked for missing, expired, invalid and unverifiable licenses; active access mounts children', async () => {
 for (const role of ['admin','reseller','customer','additional_user']) {
  for (const state of ['missing','expired','invalid','verification_unavailable','active']) {
   fixture(role,state);
   try {
    await act(async()=>{tree=Renderer.create(React.createElement(LicenseOnboarding,null,React.createElement('main',null,'management')))});
    assert.equal(calls.length,1);
    assert.equal(tree.root.findAllByType('main').length,state==='active'?1:0);
    assert.equal(tree.root.findAllByType('aside').length,state==='active'?0:1);
    if(['missing','expired','invalid'].includes(state)) assert.equal(navigations.at(-1)[0],'/activate','known license rejection returns to activation');
    else assert.equal(navigations.length,0,'uncertainty does not imply activation');
   } finally {await cleanup()}
  }
 }
});
test('network errors and malformed status keep management closed; a refused request holds mounted pages and retry releases them',async()=>{
 for(const reply of [()=>{throw new Error('offline')},()=>Response.json({can_use_panel:true}),()=>Response.json({}, {status:503})]) {
  fixture();
  globalThis.fetch=async()=>reply();
  try {await mount(LicenseOnboarding);assert.equal(tree.root.findByType('aside').props.failed,true);assert.equal(navigations.length,0,'unavailable access is not activation')} finally {await cleanup()}
 }
 fixture('admin','active');
 try {
  let mounts=0;
  function Management(){React.useEffect(()=>{mounts++},[]);return React.createElement('main',null,'management')}
  await act(async()=>{tree=Renderer.create(React.createElement(LicenseOnboarding,null,React.createElement(Management)))});
  assert.equal(heldPages().length,0);
  globalThis.fetch=async()=>Response.json({}, {status:503});
  await act(async()=>window.dispatchEvent(new Event('celikpanel:license-locked')));
  assert.equal(tree.root.findAllByType('main').length,1,'unknown access keeps the page mounted');
  assert.equal(heldPages().length,1,'and unreachable');
  assert.equal(holdLayer().length,1,'a failed read is explained');
  assert.equal(tree.root.findAllByType('aside').length,0,'the full-screen gate is for a known negative or a first load');
  assert.equal(navigations.length,0);
  globalThis.fetch=async()=>Response.json({can_use_panel:true,valid_until:Math.floor(Date.now()/1000)+60});
  await act(async()=>button('recovery.retry').props.onClick());
  assert.equal(tree.root.findAllByType('main').length,1);
  assert.equal(heldPages().length,0);assert.equal(holdLayer().length,0);
  assert.equal(mounts,1,'the page was never rebuilt');
 } finally {await cleanup()}
});
test('late access after logout cannot mount management',async()=>{
 fixture();let resolve;
 globalThis.fetch=()=>new Promise(done=>{resolve=done});
 try {await mount(LicenseOnboarding);await act(async()=>tree.unmount());await act(async()=>resolve(Response.json({can_use_panel:true,valid_until:2000000000})));assert.equal(navigations.length,0)} finally {await cleanup()}
});
test('format errors are actionable; pasted whitespace is trimmed and successful activation clears the secret', async () => {
  fixture();
  try {
    await mount(LicensePanel);
    assert.equal(input().props.type, 'password');
    await enter('wrong'); await submit();
    assert.equal(calls.length, 1, 'invalid format must not contact activation');
    assert.equal(input().props['aria-invalid'], true);
    assert.equal(tree.root.findByProps({ id: 'license-key-error' }).props.children, 'license.keyInvalid');
    await act(async () => tree.root.findByProps({ 'aria-label': 'license.showKey' }).props.onClick());
    assert.equal(input().props.type, 'text');
    const key = 'CPK-' + 'a'.repeat(64);
    await enter('  ' + key + '\n');
    globalThis.fetch = async (url, options) => {
      calls.push({ url, options });
      assert.deepEqual(JSON.parse(options.body), { action: 'activate', key });
      return Response.json({ state: 'active', can_provision: true, expires_at: 2000000000 });
    };
    await submit();
    assert.equal(tree.root.findAllByType('input').length, 0);
    assert.ok(!JSON.stringify(tree.toJSON()).includes(key));
    assert.equal(button('license.continueSetup'), undefined);
    assert.equal(button('license.refresh'), undefined);
    assert.deepEqual(navigations, [['/', { replace: true }]]);
  } finally { await cleanup(); }
});
test('activation rejection preserves the key and offers no exploration bypass', async () => {
  fixture();
  try {
    await mount(LicensePanel);
    const key = 'CPK-' + 'b'.repeat(64);
    await enter(key);
    globalThis.fetch = async () => Response.json({ code: 'license_action_failed', error: 'license: license_in_use' }, { status: 409 });
    await submit();
    assert.equal(input().props.value, key);
    // The page's own sentence; the server's code under Technical details (2026-10-31).
    const alert = tree.root.findByProps({ role: 'alert' });
    assert.equal(alert.findByType('p').props.children, 'license.inUse');
    assert.equal(alert.findByType('code').props.children, 'license_action_failed');
    assert.equal(button('license.activate').props.disabled, false);
    assert.equal(button('license.explore'), undefined);
    assert.equal(navigations.length, 0);
  } finally { await cleanup(); }
});

test('an open session is held at the signed deadline and browser history cannot escape',async()=>{
 fixture('admin','active');
 const timers=[];const previous=window.setTimeout;
 window.setTimeout=(fn,ms)=>{if(ms>15000){timers.push(fn);return 0}return previous(fn,ms)};
 try {
  await act(async()=>{tree=Renderer.create(React.createElement(LicenseOnboarding,null,React.createElement('main',null,'management')))});
  globalThis.fetch=async()=>Response.json({can_use_panel:false,valid_until:0});
  await act(async()=>timers.at(-1)());
  assert.equal(heldPages().length,1,'no management control is reachable past the deadline');
  assert.equal(holdLayer().length,1);
  assert.equal(navigations.length,0,'legacy false has no activation diagnosis');
  globalThis.licenseTest.pathname='/services';
  await act(async()=>tree.update(React.createElement(LicenseOnboarding,null,React.createElement('main',null,'management'))));
  assert.equal(navigations.length,0);
  assert.equal(heldPages().length,1,'another address is held the same way');
 }finally{window.setTimeout=previous;await cleanup()}
});

test('only administrators can open signed updates while activation remains mounted', async () => {
 for (const role of ['admin','reseller','customer','additional_user']) {
  fixture(role);
  try {
   await act(async()=>{tree=Renderer.create(React.createElement(LicenseLockScreen,{checking:false,failed:false,onCheck(){}}))});
   const disclosure=tree.root.findAllByType('details');
   assert.equal(disclosure.length,role==='admin'?1:0);
   assert.equal(tree.root.findAllByType('article').length,0);
   if(role==='admin') {
    await act(async()=>disclosure[0].props.onToggle({currentTarget:{open:true}}));
    assert.equal(tree.root.findByType('article').props.activation,true);
    assert.equal(tree.root.findAllByType('section').length,1,'activation stays available');
    await act(async()=>disclosure[0].props.onToggle({currentTarget:{open:false}}));
    assert.equal(tree.root.findAllByType('article').length,0);
   }
  } finally {await cleanup()}
 }
});

test('short verification renews before expiry without remounting management; a hidden page does not poll, is held unseen and reads once on return',async()=>{
 fixture('admin','active');
 const previousTimer=window.setTimeout,previousNow=Date.now;
 const timers=[];let now=Date.now(),mounts=0;
 Date.now=()=>now;
 window.setTimeout=(fn,ms)=>{if(ms>15000){timers.push({fn,ms});return 0}return previousTimer(fn,ms)};
 globalThis.fetch=async()=>{calls.push('access');return Response.json({can_use_panel:true,valid_until:Math.floor(now/1000)+60})};
 function Management(){React.useEffect(()=>{mounts++},[]);return React.createElement('main',null,'management')}
 try{
  await act(async()=>{tree=Renderer.create(React.createElement(LicenseOnboarding,null,React.createElement(Management)))});
  const early=timers.find(timer=>timer.ms>40000&&timer.ms<=45000);
  assert.ok(early,'renew before the hard deadline');
  now+=45000;
  await act(async()=>early.fn());
  assert.equal(calls.length,2);
  assert.equal(mounts,1,'successful renewal preserves page state');
  assert.equal(navigations.length,0);
  await showTab('hidden');
  const lastEarly=timers.filter(timer=>timer.ms>40000&&timer.ms<=45000).at(-1);
  await act(async()=>lastEarly.fn());
  assert.equal(calls.length,2,'hidden page does not renew');
  const deadline=timers.filter(timer=>timer.ms>45000&&timer.ms<=60000).at(-1);
  now+=15000;
  await act(async()=>deadline.fn());
  assert.equal(calls.length,2,'expiry itself does not poll while hidden');
  assert.equal(heldPages().length,1,'hidden expiry still closes management');
  assert.equal(tree.root.findAllByType('main').length,1,'without removing the page');
  assert.equal(holdLayer().length,0,'a decision that only ran out in a hidden tab is not reported as a failure');
  assert.equal(tree.root.findAllByType('aside').length,0);
  await showTab('visible');
  assert.equal(calls.length,3,'returning reads once');
  assert.equal(heldPages().length,0);assert.equal(holdLayer().length,0);
  assert.equal(mounts,1,'the page the owner left is the page they return to');
  assert.equal(navigations.length,0);
 }finally{Date.now=previousNow;window.setTimeout=previousTimer;document.visibilityState='visible';await cleanup()}
});


test('locked activation automatically rechecks access; active settings stay on their page', async () => {
  for (const locked of [true, false]) {
    fixture();
    globalThis.licenseTest.search = '';
    let accessChecks = 0;
    try {
      await act(async () => { tree = Renderer.create(React.createElement(LicensePanel, {
        locked, onContinue: () => { accessChecks++; },
      })); });
      await enter('CPK-' + 'c'.repeat(64));
      globalThis.fetch = async () => Response.json({ state: 'active', can_provision: true });
      await submit();
      assert.equal(accessChecks, locked ? 1 : 0);
      assert.equal(navigations.length, 0, 'locked activation delegates to the verified access gate');
      assert.equal(button('license.continueSetup'), undefined);
      assert.equal(!!button('license.refresh'), !locked);
    } finally { await cleanup(); }
  }
});

test('an already active activation page automatically rechecks access', async () => {
  fixture('admin', 'active');
  let accessChecks = 0;
  try {
    await act(async () => { tree = Renderer.create(React.createElement(LicensePanel, {
      locked: true, onContinue: () => { accessChecks++; },
    })); });
    assert.equal(accessChecks, 1);
    assert.equal(button('license.continueSetup'), undefined);
    assert.equal(button('license.refresh'), undefined);
  } finally { await cleanup(); }
});


test('transient access failures preserve the current route and mounted form, also past the signed deadline', async () => {
 fixture('admin','active');
 const originalTimer=window.setTimeout;
 const timers=[];let mounts=0;
 window.setTimeout=(fn,ms)=>{if(ms>15000){timers.push(fn);return 0}return originalTimer(fn,ms)};
 function Management(){React.useEffect(()=>{mounts++},[]);return React.createElement('main',null,'setup')}
 try {
  await act(async()=>{tree=Renderer.create(React.createElement(LicenseOnboarding,null,React.createElement(Management)))});
  for(const reply of [()=>{throw new TypeError('Failed to fetch')},()=>Response.json({}, {status:503}),()=>Response.json({can_use_panel:true})]) {
   globalThis.fetch=async()=>reply();
   await act(async()=>window.dispatchEvent(new Event('focus')));
   assert.equal(tree.root.findAllByType('main').length,1);
   assert.equal(mounts,1,'connection banner must not remount the form');
   assert.equal(navigations.length,0);
   assert.ok(button('common.reloadPage'),'a full page reload is available for TLS recovery');
  }
  await act(async()=>timers.at(-1)());
  assert.equal(heldPages().length,1,'cached access cannot outlive the server deadline');
  assert.equal(holdLayer().length,1,'the failed read is explained over the page');
  assert.equal(tree.root.findAllByType('main').length,1);
  assert.equal(mounts,1,'the form is not rebuilt');
  assert.equal(button('common.reloadPage'),undefined,'the banner of a still valid decision is gone');
  assert.equal(navigations.length,0,'unknown state retains the setup URL');
 } finally {window.setTimeout=originalTimer;await cleanup()}
});

test('focus checks share an in-flight access request; a refusal holds at once and a known rejection still locks',async()=>{
 fixture('admin','active');let resolve;let requests=0;
 try {
  await act(async()=>{tree=Renderer.create(React.createElement(LicenseOnboarding,null,React.createElement('main')))});
  globalThis.fetch=()=>{requests++;return new Promise(done=>{resolve=done})};
  await act(async()=>{window.dispatchEvent(new Event('focus'));window.dispatchEvent(new Event('focus'))});
  assert.equal(requests,1);
  const staleResolve=resolve;
  await act(async()=>window.dispatchEvent(new Event('celikpanel:license-locked')));
  assert.equal(heldPages().length,1);
  assert.equal(requests,2,'the refusal starts its own read');
  await act(async()=>{window.dispatchEvent(new Event('celikpanel:license-locked'));window.dispatchEvent(new Event('celikpanel:license-locked'))});
  assert.equal(requests,2,'pages that keep reporting the refusal cannot restart the read');
  await act(async()=>staleResolve(Response.json({can_use_panel:true,valid_until:Math.floor(Date.now()/1000)+3600})));
  assert.equal(heldPages().length,1,'superseded response cannot undo the hold');
  assert.equal(navigations.length,0,'an unknown state does not diagnose a missing license');
  await act(async()=>resolve(Response.json({can_use_panel:false,valid_until:0,state:'expired',observation:'known'})));
  assert.equal(navigations.at(-1)[0],'/activate');
  assert.equal(tree.root.findAllByType('main').length,0,'a known rejection removes management as before');
  assert.equal(heldPages().length,0);
  assert.equal(tree.root.findAllByType('aside').length,1);
 }finally{await cleanup()}
});

test('connection recovery has retry and reload without an activation form or update requests',async()=>{
 fixture();let reloads=0;window.location={reload(){reloads++}};
 try {
  await act(async()=>{tree=Renderer.create(React.createElement(LicenseLockScreen,{checking:false,failed:true,onCheck(){}}))});
  assert.equal(tree.root.findAllByType('section').length,1);
  assert.ok(!JSON.stringify(tree.toJSON()).includes('activation'));
  assert.equal(tree.root.findAllByType('details').length,0);
  assert.ok(button('license.refresh'));
  await act(async()=>button('common.reloadPage').props.onClick());assert.equal(reloads,1);
  assert.equal(calls.length,0);
 }finally{await cleanup()}
});

test('license status fetch failure does not ask for a replacement key',async()=>{
 fixture();globalThis.fetch=async()=>{throw new TypeError('Failed to fetch')};
 try {
  await mount(LicensePanel);
  assert.equal(tree.root.findAllByType('form').length,0);
  assert.equal(tree.root.findByProps({role:'alert'}).findByType('p').props.children,'license.lockError');
  assert.ok(button('license.refresh'));
 }finally{await cleanup()}
});


test('panel address hint only links to safe server metadata and preserves the current port',async()=>{
 const Hint=await component('PanelAddressHint');
 for(const hostname of ['panel.example.test','evil.example/path','user@evil.example','*.example.test','',null,'127.0.0.1']) {
  fixture(); window.location={origin:'https://127.0.0.1:2083',hostname:'127.0.0.1'};
  globalThis.fetch=async()=>Response.json({hostname});
  try {
   await mount(Hint);
   const links=tree.root.findAllByType('a');
   assert.equal(links.length,hostname==='panel.example.test'?1:0);
   if(links.length)assert.equal(links[0].props.href,'https://panel.example.test:2083/');
  }finally{await cleanup()}
 }
});


test('unavailable license observations never display an activation key form',async()=>{
 for(const state of ['verification_unavailable','status_unavailable']) {
  fixture('admin',state);
  try {
   await mount(LicensePanel);
   assert.equal(tree.root.findAllByType('form').length,0);
   assert.equal(tree.root.findAllByType('input').length,0);
   assert.ok(JSON.stringify(tree.toJSON()).includes('recovery.licenseTitle'));
   assert.ok(button('license.refresh'));
  }finally{await cleanup()}
 }
});

// set14b D2: a server clock 418 s behind the browser's kept every page from
// loading, because the browser compared the absolute deadline with its own
// clock. The server now states the time left on its own clock; the browser
// adds it to the moment it asked. Without the field the old rule stays.
// Sunucu saati tarayıcınınkinden 418 s gerideyken hiçbir sayfa açılmıyordu;
// sunucu artık kalan süreyi kendi saatiyle söyler, tarayıcı sorduğu ana ekler.
test('the server-stated time left decides access whatever the two clocks say; without it the absolute deadline stays',async()=>{
 const previousTimer=window.setTimeout;
 const cases=[
  {name:'server clock 7 minutes behind, 55 s left',body:now=>({can_use_panel:true,valid_until:Math.floor(now/1000)-420,valid_for_seconds:55,state:'active',observation:'known'}),accepted:true},
  {name:'no time left',body:now=>({can_use_panel:true,valid_until:Math.floor(now/1000)+60,valid_for_seconds:0,state:'active',observation:'known'}),accepted:false},
  {name:'older server, deadline 7 minutes behind',body:now=>({can_use_panel:true,valid_until:Math.floor(now/1000)-420,state:'active',observation:'known'}),accepted:false},
  {name:'older server, deadline ahead',body:now=>({can_use_panel:true,valid_until:Math.floor(now/1000)+60,state:'active',observation:'known'}),accepted:true},
 ];
 for(const item of cases){
  fixture('admin','active');
  const timers=[];
  window.setTimeout=(fn,ms)=>{if(ms>15000){timers.push(ms);return 0}return previousTimer(fn,ms)};
  globalThis.fetch=async()=>{calls.push('access');return Response.json(item.body(Date.now()))};
  try{
   await act(async()=>{tree=Renderer.create(React.createElement(LicenseOnboarding,null,React.createElement('main',null,'management')))});
   assert.equal(calls.length,1,item.name);
   assert.equal(tree.root.findAllByType('main').length,item.accepted?1:0,item.name);
   assert.equal(heldPages().length,0,item.name);
   assert.equal(tree.root.findAllByType('aside').length,item.accepted?0:1,`${item.name}: the licence gate`);
   assert.equal(navigations.length,0,`${item.name}: never activation`);
   if(item.name.startsWith('server clock')){
    // Renewal 15 s before the server's deadline: 55 - 15 = 40 s from asking.
    assert.ok(timers.some(ms=>ms>38000&&ms<=40000),`renewal follows the stated time left: ${timers}`);
    assert.ok(timers.some(ms=>ms>53000&&ms<=55000),`deadline follows the stated time left: ${timers}`);
   }
  } finally {window.setTimeout=previousTimer;await cleanup()}
 }
});

// Native record set17, defect D1 (2026-10-15): during a real update's restart,
// once the outage outlasted the access validity of the last good access read,
// the gate drew "The Panel is not answering; an update may be restarting it"
// over the update's calm restart, and the observation recovery it raised paused
// the update tracking, which released the update's navigation lock for good.
// While the followed update restarts the Panel (its own window, the same bound as
// the calm notice), a read that got no answer keeps the last positive decision,
// nothing is drawn, and the access is read again quietly; after the window the
// honest dialog appears. The browser's clock only: valid_for_seconds from the
// moment of asking, plus the update's window.
//
// set17 D1: guncellemenin yeniden baslamasinda lisans kapisi kendi penceresini
// aciyor ve guncellemenin gezinme kilidini birakiyordu. Pencere icinde son olumlu
// karar korunur, bir sey cizilmez, erisim sessizce yeniden okunur.
const restartSignal = await import(restartURL);
const { useSystemUpdateNavigationLease } = await import(leaseURL);
const settle = async () => { for (let i = 0; i < 4; i++) await new Promise(done => setImmediate(done)); };
function fakeTime(start) {
 const saved = { setTimeout: window.setTimeout, clearTimeout: window.clearTimeout, setInterval: window.setInterval, clearInterval: window.clearInterval, now: Date.now };
 let now = start, next = 1; const tasks = new Map();
 window.setTimeout = (fn, delay = 0) => { const id = next++; tasks.set(id, { at: now + Math.max(0, Number(delay) || 0), fn, every: 0 }); return id; };
 window.setInterval = (fn, delay = 0) => { const id = next++; tasks.set(id, { at: now + delay, fn, every: Math.max(1, delay) }); return id; };
 window.clearTimeout = window.clearInterval = id => { tasks.delete(id); };
 Date.now = () => now;
 return {
  async advance(ms) {
   const target = now + ms;
   for (;;) {
    let due = null;
    for (const [id, task] of tasks) if (task.at <= target && (!due || task.at < due[1].at)) due = [id, task];
    if (!due) break;
    const [id, task] = due;
    now = task.at;
    if (task.every) task.at += task.every; else tasks.delete(id);
    await act(async () => { task.fn(); await settle(); });
   }
   now = target;
   await act(settle);
  },
  restore() { Object.assign(window, { setTimeout: saved.setTimeout, clearTimeout: saved.clearTimeout, setInterval: saved.setInterval, clearInterval: saved.clearInterval }); Date.now = saved.now; },
 };
}
// The lab's figures: 45 s of access validity, an update started 20 s before the
// page opened, the Panel down from T+20 s to T+90 s.
const VALID_FOR = 45;
function restartFixture(start, { startedAt = start - 20000 } = {}) {
 fixture('admin', 'active');
 const reads = [];
 let down = false;
 globalThis.fetch = async url => {
  reads.push(Date.now() - start);
  if (down) throw new TypeError('Failed to fetch');
  return Response.json({ can_use_panel: true, state: 'active', observation: 'known', valid_until: Math.floor(Date.now() / 1000) + VALID_FOR, valid_for_seconds: VALID_FOR });
 };
 const recoveries = [];
 let mounts = 0;
 function Management() { React.useEffect(() => { mounts++; }, []); return React.createElement('main', null, 'management'); }
 return {
  reads, recoveries, startedAt,
  get mounts() { return mounts; },
  setDown(value) { down = value; },
  async publish(lostSince) { await act(async () => restartSignal.publishSystemUpdateRestart({ requestID: 'a'.repeat(32), startedAt, lostSince })); },
  element: (Gate = LicenseOnboarding, extra = {}) => React.createElement(Gate, { onRecoveryChange: value => recoveries.push(value), ...extra }, React.createElement(Management)),
 };
}
const banner = () => tree.root.findAll(node => node.type === 'div' && node.props.role === 'status');
async function endRestart() { await act(async () => restartSignal.publishSystemUpdateRestart(null)); }

test('set17 D1: inside the update restart an expired access validity draws nothing, keeps the decision and reads on quietly', async () => {
 const start = Date.now();
 const clock = fakeTime(start);
 const lab = restartFixture(start);
 try {
  await lab.publish(null);
  await act(async () => { tree = Renderer.create(lab.element()); await settle(); });
  assert.equal(tree.root.findAllByType('main').length, 1);
  await clock.advance(20000);
  lab.setDown(true);
  // The update tracking's first lost read (T+25.6 s in set17 run a).
  await clock.advance(5600);
  await lab.publish(start + 25600);
  // Past the access validity (45 s) and well into the outage.
  await clock.advance(64400);
  assert.equal(tree.root.findAllByType('main').length, 1, 'the page stays mounted');
  assert.equal(heldPages().length, 0, 'the update overlay owns the screen; the gate adds no hold');
  assert.equal(holdLayer().length, 0, 'no "The Panel is not answering" dialog');
  assert.equal(tree.root.findAllByType('aside').length, 0);
  assert.equal(banner().length, 0, 'no connection banner either');
  assert.ok(!lab.recoveries.includes(true), 'no observation recovery, so the update tracking is never paused');
  const during = lab.reads.filter(at => at > 20000);
  assert.ok(during.length >= 10, `the access is read again by itself during the outage: ${during}`);
  // The 5-second re-check, the read at the access deadline and the regular
  // 60-second check may meet in one moment; never more often than that.
  const moments = [...new Set(during)];
  assert.ok(moments.every((at, index) => index === 0 || at - moments[index - 1] >= 5000), `not more often than the re-check: ${during}`);
  assert.ok(during.length <= moments.length + 2, `at most the two coinciding timers: ${during}`);
  // Reconnection: the update tracking reads the Panel again; the gate reads at once.
  lab.setDown(false);
  const before = lab.reads.length;
  await lab.publish(null);
  assert.equal(lab.reads.length, before + 1, 'one read at once when the update reached the Panel again');
  await clock.advance(2000);
  assert.equal(tree.root.findAllByType('main').length, 1);
  assert.equal(holdLayer().length, 0);
  assert.equal(banner().length, 0);
  assert.equal(lab.mounts, 1, 'the page was never rebuilt');
  assert.ok(!lab.recoveries.includes(true));
 } finally { await endRestart(); clock.restore(); await cleanup(); }
});

test('set17 D1: without a followed update a lost read past the access validity is explained as before', async () => {
 const start = Date.now();
 const clock = fakeTime(start);
 const lab = restartFixture(start);
 try {
  await act(async () => { tree = Renderer.create(lab.element()); await settle(); });
  await clock.advance(20000);
  lab.setDown(true);
  await clock.advance(24000);
  assert.equal(holdLayer().length, 0, 'inside the access validity nothing is drawn');
  await clock.advance(3000);
  assert.equal(heldPages().length, 1, 'past it the pages are held');
  assert.equal(holdLayer().length, 1, 'and the gate explains it');
  assert.ok(lab.recoveries.includes(true), 'the observation recovery is raised as before');
 } finally { await endRestart(); clock.restore(); await cleanup(); }
});

test('set17 D1: the decision is kept to the end of the update restart window, then the honest dialog appears', async () => {
 // The window is the calm notice's: 6 minutes from the start, at least 2 minutes
 // from the first lost read. Access validity ends at T+45 s in both cases.
 const cases = [
  { name: 'started at T-20 s: 6 minutes from the start', startedAt: -20000, end: 340000 },
  { name: 'started at T-5 min 30 s: 2 minutes from the lost read at T+20 s', startedAt: -330000, end: 140000 },
 ];
 for (const item of cases) {
  const start = Date.now();
  const clock = fakeTime(start);
  const lab = restartFixture(start, { startedAt: start + item.startedAt });
  try {
   await lab.publish(null);
   await act(async () => { tree = Renderer.create(lab.element()); await settle(); });
   await clock.advance(20000);
   lab.setDown(true);
   await lab.publish(start + 20000);
   await clock.advance(item.end - 22000);
   assert.equal(holdLayer().length, 0, `${item.name}: 2 s before the end, still the update restart`);
   assert.ok(!lab.recoveries.includes(true), item.name);
   await clock.advance(4000);
   assert.equal(heldPages().length, 1, `${item.name}: 2 s after the end, the window is over`);
   assert.equal(holdLayer().length, 1, item.name);
   assert.ok(lab.recoveries.includes(true), item.name);
  } finally { await endRestart(); clock.restore(); await cleanup(); }
 }
});

test('set17 D1: an answer from the Panel is applied during the restart; a known negative is not kept', async () => {
 const start = Date.now();
 const clock = fakeTime(start);
 const lab = restartFixture(start);
 try {
  await lab.publish(null);
  await act(async () => { tree = Renderer.create(lab.element()); await settle(); });
  await clock.advance(20000);
  lab.setDown(true);
  await lab.publish(start + 20000);
  await clock.advance(30000);
  assert.equal(holdLayer().length, 0);
  globalThis.fetch = async () => Response.json({ can_use_panel: false, state: 'expired', observation: 'known', valid_until: 0 });
  await clock.advance(5000);
  assert.equal(tree.root.findAllByType('main').length, 0, 'a known negative replaces the page as before');
  assert.equal(navigations.at(-1)?.[0], '/activate');
 } finally { await endRestart(); clock.restore(); await cleanup(); }
});

// The update's navigation lock (2 minutes from its start) is released for good
// once the update tracking is paused; App pauses it while the gate raises its
// observation recovery (publishSystemUpdateAuthentication). The harness wires
// the gate to the real lease the way App and the provider do.
test('set17 D1: the update navigation lock survives the restart and is still held after reconnection', async () => {
 const start = Date.now();
 const clock = fakeTime(start);
 const lab = restartFixture(start);
 const values = new Map();
 const runtime = {
  now: () => Date.now(), monotonicNow: () => Date.now(),
  schedule: (fn, delay) => window.setTimeout(fn, delay), cancel: id => window.clearTimeout(id),
  storage: { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value), removeItem: key => values.delete(key), key: () => null, length: 0 },
  subscribe: () => () => {}, subscribeWake: () => () => {},
 };
 const lease = [];
 function Harness() {
  const [recovering, setRecovering] = React.useState(false);
  // The restart keeps the lock as the provider wires it (set17b F1).
  const restart = restartSignal.systemUpdateNavigationRestart(restartSignal.useSystemUpdateRestart(), 'a'.repeat(32), lab.startedAt);
  const state = useSystemUpdateNavigationLease('a'.repeat(32), lab.startedAt, !recovering, restart, runtime);
  lease.push(state.blocked);
  return lab.element(LicenseOnboarding, { onRecoveryChange: value => { lab.recoveries.push(value); setRecovering(value); } });
 }
 try {
  await lab.publish(null);
  await act(async () => { tree = Renderer.create(React.createElement(Harness)); await settle(); });
  assert.equal(lease.at(-1), true, 'the lock is held while the update runs');
  await clock.advance(20000);
  lab.setDown(true);
  await lab.publish(start + 25600);
  await clock.advance(70000);
  assert.equal(lease.at(-1), true, 'T+90 s: the lock is still held through the outage');
  lab.setDown(false);
  await lab.publish(null);
  await clock.advance(5000);
  assert.equal(lease.at(-1), true, 'after reconnection the lock is the one held before the outage');
  assert.ok(!lease.includes(false), 'it was never released on the way');
  // Its own 2-minute bound still ends it (T-20 s + 120 s).
  await clock.advance(10000);
  assert.equal(lease.at(-1), false, 'the bounded lease ends by itself as before');
 } finally { await endRestart(); clock.restore(); await cleanup(); }
});

// D-039: the license page explains the license check itself.
const drawn = () => JSON.stringify(tree.toJSON());
const flush = () => act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });
const answering = answers => {
  globalThis.fetch = async (url, options) => {
    calls.push({ url, options });
    const next = answers.length > 1 ? answers.shift() : answers[0];
    return typeof next === 'function' ? next() : Response.json(next);
  };
};
test('license page: an unreachable service inside the offline window says nothing is needed and until when', async () => {
  fixture('admin', 'active'); globalThis.licenseTest.search = '';
  answering([{ state: 'active', can_provision: true, expires_at: 2000000000, check: 'unreachable', offline_for_seconds: 5 * 86400, verified_seconds_ago: 2 * 86400 }]);
  try {
    await mount(LicensePanel); await flush();
    const box = tree.root.findByProps({ 'data-license-check': 'unreachable' });
    assert.equal(box.props.role, 'status');
    for (const key of ['license.offline.title', 'license.offline.help', 'license.lastCheck', 'license.state.active']) assert.ok(drawn().includes(key), key);
    assert.ok(!drawn().includes('recovery.licenseHelp'));
  } finally { await cleanup(); }
});
test('license page: the last 48 hours warn and say who acts; a lapse names its cause instead of the generic text', async () => {
  for (const [answer, keys, role] of [
    [{ state: 'active', can_provision: true, check: 'unreachable', offline_for_seconds: 40 * 3600, verified_seconds_ago: 128 * 3600 }, ['license.offline.soonTitle', 'license.offline.soonHelp'], 'alert'],
    [{ state: 'active', can_provision: true, check: 'clock_behind', offline_for_seconds: 5 * 86400 }, ['license.clock.title', 'license.clock.help'], 'alert'],
    [{ state: 'verification_unavailable', can_provision: false, reason: 'offline_ended' }, ['license.offlineEnded.title', 'license.offlineEnded.help']],
    [{ state: 'verification_unavailable', can_provision: false, reason: 'clock_behind' }, ['license.clockBehind.title', 'license.clockBehind.help']],
    [{ state: 'verification_unavailable', can_provision: false }, ['recovery.licenseTitle', 'recovery.licenseHelp']],
    [{ state: 'active', can_provision: true }, ['license.state.active']],
  ]) {
    fixture('admin', 'active'); globalThis.licenseTest.search = '';
    answering([answer]);
    try {
      await mount(LicensePanel); await flush();
      for (const key of keys) assert.ok(drawn().includes(key), `${JSON.stringify(answer)} lacks ${key}`);
      const boxes = tree.root.findAll(node => node.props && node.props['data-license-check'] !== undefined && typeof node.type === 'string');
      if (role) assert.equal(boxes[0].props.role, role);
      else assert.equal(boxes.length, 0, 'only management that is open carries a check box');
      // A lapse is not a key problem: no activation form for it.
      if (answer.state === 'verification_unavailable') assert.equal(tree.root.findAllByType('form').length, 0);
      assert.ok(!!button('license.refresh'), 'Check again stays available');
    } finally { await cleanup(); }
  }
});
test('license page: a failed Check again keeps its error and reads the status again to show the offline window', async () => {
  fixture('admin', 'active'); globalThis.licenseTest.search = '';
  answering([
    { state: 'active', can_provision: true },
    () => Response.json({ code: 'license_action_failed', error: 'license service unavailable; existing license retained' }, { status: 400 }),
    { state: 'active', can_provision: true, check: 'unreachable', offline_for_seconds: 5 * 86400, verified_seconds_ago: 3600 },
  ]);
  try {
    await mount(LicensePanel); await flush();
    await act(async () => button('license.refresh').props.onClick()); await flush(); await flush();
    assert.deepEqual(calls.map(call => call.options?.method ?? 'GET'), ['GET', 'POST', 'GET']);
    assert.equal(tree.root.findAll(node => node.props?.role === 'alert' && typeof node.type === 'string' && node.findAllByType('p').some(p => p.props.children === 'license.unavailable')).length, 1, 'the failed check is still said');
    assert.ok(drawn().includes('license.offline.title'), 'the re-read shows how long management stays open');
    assert.equal(button('license.refresh').props.disabled, false);
  } finally { await cleanup(); }
});
test('the lock screen is told why verification lapsed (D-039), and only for a license cause', async () => {
  for (const reason of ['offline_ended', 'clock_behind', undefined, 'unknown-reason']) {
    fixture('admin', 'verification_unavailable');
    globalThis.fetch = async (url, options) => {
      calls.push({ url, options });
      return Response.json({ state: 'verification_unavailable', observation: 'unavailable', can_use_panel: false, valid_until: 0, valid_for_seconds: 0, ...(reason ? { reason } : {}) });
    };
    try {
      await act(async () => { tree = Renderer.create(React.createElement(LicenseOnboarding, null, React.createElement('main', null, 'management'))); });
      const aside = tree.root.findByType('aside');
      assert.equal(aside.props.cause, 'license');
      assert.equal(aside.props.licenseReason, reason === 'offline_ended' || reason === 'clock_behind' ? reason : undefined);
      assert.equal(tree.root.findAllByType('main').length, 0);
    } finally { await cleanup(); }
  }
});
