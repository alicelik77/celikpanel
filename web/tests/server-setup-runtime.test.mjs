import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { setupCatalogFixture } from './fixtures/server-setup-components.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const dataURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), {
    compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText;
const remoteURL = dataURL(compile('../src/lib/remoteDNS.ts'));
const setupURL = dataURL(compile('../src/lib/serverSetup.ts'));
const componentLibURL = dataURL(compile('../src/lib/serverSetupComponents.ts').replace("from './serverSetup'", `from '${setupURL}'`));
const componentNameURL = dataURL(compile('../src/lib/componentName.ts'));
const operationURL = dataURL(compile('../src/lib/serverSetupOperation.ts').replace("from './serverSetup'", `from '${setupURL}'`));
const dnsEnglishURL = dataURL(compile('../src/i18n/setupDNS/en.ts'));
const dnsTurkishURL = dataURL(compile('../src/i18n/setupDNS/tr.ts'));
const dnsCopyURL = dataURL(compile('../src/i18n/setupDNS.ts').replace("from './setupDNS/en'", `from '${dnsEnglishURL}'`).replace("from './setupDNS/tr'", `from '${dnsTurkishURL}'`));
const guidanceURL = dataURL(compile('../src/lib/serverSetupGuidance.ts'));
const handoverURL = dataURL(compile('../src/lib/panelHandover.ts'));
const visibleReadsURL = dataURL(compile('../src/lib/visibleReads.ts'));
const setup = await import(setupURL);
const operations = await import(operationURL);
const stub = dataURL(`
import React from '${reactURL}';
export const useAuth = () => globalThis.setupFixture.auth;
export const useI18n = () => ({ locale:globalThis.setupFixture.locale, t:(key, vars)=>globalThis.setupFixture.translations?.[key]??key, screensReady:true, screensFailed:false });
export const useLocation = () => ({ pathname:globalThis.setupFixture.path });
export const Navigate = props => React.createElement('redirect',props);
export const Link = props => React.createElement('a',{...props,href:props.to});
export const useServerSetup = () => globalThis.setupFixture.context;
export const ServerSetupShell = props => React.createElement('main',null,props.navigation,props.children);
export const Layout = props => React.createElement('div',null,props.children);
export const BrandMark=()=>null, LanguageSwitcher=()=>null, ThemeSwitcher=()=>null, ChangePasswordModal=()=>null;
export const Button=props=>React.createElement('button',props);
export const KeepWords=props=>props.text;
export const Spinner=()=>React.createElement('span',null,'loading');
export const ArrowRight=()=>null, Check=()=>null, Circle=()=>null, Loader2=()=>null;
export const ServerSetupDNSConnection=props=>React.createElement('remote-connection',props);
export const inputClass='';
export const AddressLink=props=>React.createElement('a',{href:props.href},props.address);
export const SetupFirstSite=props=>React.createElement('first-site',props);
`);
const stepsURL = dataURL(`import React from '${reactURL}';\n` + compile('../src/components/ServerSetupSteps.tsx').replace(/from ['"]([^'"]+)['"]/g, (_, path) => `from '${stub}'`));
const choiceURL = dataURL(`import React from '${reactURL}';\n` + compile('../src/components/ServerSetupChoice.tsx').replace(/from ['"]([^'"]+)['"]/g, (_, path) => `from '${path === 'react' ? reactURL : path.endsWith('/serverSetup') ? setupURL : stub}'`));
const componentUIURL = dataURL(`import React from '${reactURL}';\n` + compile('../src/components/ServerSetupComponents.tsx').replace(/from ['"]([^'"]+)['"]/g, (_, path) => `from '${path === 'react' ? reactURL : path.endsWith('/componentName') ? componentNameURL : path.endsWith('/serverSetupComponents') ? componentLibURL : stub}'`));
const { ServerSetupComponents: ComponentPicker } = await import(componentUIURL);
const apiErrorURL = dataURL(compile('../src/lib/apiError.ts'));
const remoteLibURL = dataURL(compile('../src/lib/remote.ts').replace(/from ['"]([^'"]+)['"]/g, (_, path) => `from '${path === 'react' ? reactURL : apiErrorURL}'`));
const wizardStepsURL = dataURL(`import React from '${reactURL}';
` + compile('../src/components/ServerSetupWizardSteps.tsx').replace(/from ['"]([^'"]+)['"]/g, (_, path) => `from '${path === 'react' ? reactURL : path.endsWith('/componentName') ? componentNameURL : path.endsWith('/serverSetupComponents') ? componentLibURL : path.endsWith('/setupDNS') ? dnsCopyURL : path.endsWith('/lib/remote') ? remoteLibURL : path.endsWith('/serverSetup') ? setupURL : stub}'`));
async function loadComponent(name, exported = name) {
    const source = compile(`../src/components/${name}.tsx`).replace(/from ['"]([^'"]+)['"]/g, (_, path) => {
        const url = path === 'react' ? reactURL : path.endsWith('/ServerSetupSteps') ? stepsURL : path.endsWith('/remoteDNS') ? remoteURL : path.endsWith('/ServerSetupComponents') ? componentUIURL : path.endsWith('/ServerSetupWizardSteps') ? wizardStepsURL : path.endsWith('/componentName') ? componentNameURL : path.endsWith('/serverSetupComponents') ? componentLibURL : path.endsWith('/ServerSetupChoice') ? choiceURL : path.endsWith('/setupDNS') ? dnsCopyURL : path.endsWith('/visibleReads') ? visibleReadsURL : path.endsWith('/panelHandover') ? handoverURL : path.endsWith('/serverSetupGuidance') ? guidanceURL : path.endsWith('/serverSetupOperation') ? operationURL : path.endsWith('/serverSetup') ? setupURL : stub;
        return `from '${url}'`;
    });
    return (await import(dataURL(`import React from '${reactURL}';\n${source}`)))[exported];
}
const Gate = await loadComponent('ServerSetupGate');
const Wizard = await loadComponent('ServerSetup');
const original = { fetch:globalThis.fetch, window:globalThis.window, localStorage:globalThis.localStorage, sessionStorage:globalThis.sessionStorage };
const fresh = () => ({ version:1,revision:1,origin:'fresh',status:'new',required:true,
    draft:{purpose:'web',panel_domain:'',mail_hostname:'',dns_mode:'local',remote_dns_connection_id:'',dns_engine:'pdns',dns_role:'primary',ns1:'',ns2:'',local_ip:'',peer_ip:'',peer_ns:'',node_version:'',database:'mariadb'},checks:[] });
let calls, state, tree, store, fixture;
function init(role='admin', overrides={}) {
    calls=[]; state={...fresh(),...overrides}; tree=null; store=new Map();
    const context={snapshot:state,accept(next){state=next;context.snapshot=next;},async reload(){return state;}};
    fixture=globalThis.setupFixture={auth:{role,user:{username:role},logout(){}},path:'/',context};
    globalThis.localStorage={getItem:key=>store.get(key)??null,setItem:(key,value)=>store.set(key,value),removeItem:key=>store.delete(key)};
    const editorStore=new Map();
    globalThis.sessionStorage={getItem:key=>editorStore.get(key)??null,setItem:(key,value)=>editorStore.set(key,value),removeItem:key=>editorStore.delete(key)};
    globalThis.window={setTimeout,clearTimeout,setInterval,clearInterval,location:{hostname:'192.0.2.4',reload(){}}};
    globalThis.fetch=async(url,options)=>{
        calls.push({url,options});
        if(url.includes('/setup/operation'))return Response.json(null);
        if(url==='/api/v1/setup/components')return Response.json(setupCatalogFixture());
        if(url==='/api/v1/setup'&&options?.method==='PUT'){
            const body=JSON.parse(options.body);assert.equal(body.revision,state.revision);
            state={...state,draft:body.draft,revision:state.revision+1,status:'draft'};return Response.json(state);
        }
        if(url==='/api/v1/setup/plan')return Response.json(plan());
        return Response.json(state);
    };
}
function plan(extra={}){return{id:'a'.repeat(32),version:1,revision:state.revision,purpose:state.draft.purpose,steps:[{id:'verify',kind:'verify',target:'web'}],blockers:[],can_start:true,tcp_ports:[22,80,443,2083],udp_ports:[],preserve_ssh:true,persist_firewall:true,contact_email:'admin@example.com',...extra};}
function execution(marker,status='running'){return{id:'c'.repeat(32),plan_id:marker.plan_id,request_id:marker.request_id,status,phase:'queued',steps:[{id:'verify',kind:'verify',target:'web',status:'pending'}]};}
async function mount(Component=Wizard){await act(async()=>{tree=Renderer.create(React.createElement(Component,null,React.createElement('section',{'data-management':true})));});}
async function cleanup(){if(tree)await act(async()=>tree.unmount());for(const[key,value]of Object.entries(original)){if(value===undefined)delete globalThis[key];else globalThis[key]=value;}delete globalThis.setupFixture;}
const findButton=label=>tree.root.findAllByType('button').find(button=>button.props.children===label);
async function submit(){await act(async()=>tree.root.findByType('form').props.onSubmit({preventDefault(){}}));}
const input=id=>tree.root.findByProps({id});
async function type(id,value){await act(async()=>input(id).props.onChange({target:{value}}));}
// The wizard v2 path: main domain, server count, short name, roles, names, DNS home, review.
async function toNames(domain='example.com'){await type('setup-main_domain',domain);await submit();await submit();await submit();await submit();}
async function toDNSHome(domain){await toNames(domain);await submit();}
async function toReview(){await toDNSHome();await act(async()=>tree.root.findByProps({name:'setup-dns',value:'external'}).props.onChange());await submit();}

test('setup state decoder rejects absent provenance and contradictory completion',()=>{
    assert.equal(setup.decodeServerSetup({...fresh(),origin:undefined}),null);
    assert.equal(setup.decodeServerSetup({...fresh(),required:undefined}),null);
    assert.equal(setup.decodeServerSetup({...fresh(),status:'ready',required:true}),null);
    assert.equal(setup.decodeServerSetup({...fresh(),status:'ready',required:false}).status,'ready');
    assert.equal(setup.shouldOpenServerSetup(fresh(),'/domains'),true);
    assert.equal(setup.shouldOpenServerSetup(fresh(),'/settings'),false);
    assert.equal(setup.shouldOpenServerSetup({...fresh(),origin:'legacy',status:'legacy',required:false},'/'),false);
});

test('setup gate never fetches host setup for tenants; fresh administrators go to setup',async()=>{
    for(const role of ['reseller','customer','additional_user','admin']){
        init(role);try{await mount(Gate);assert.equal(calls.length,role==='admin'?1:0);assert.equal(tree.root.findAllByType('redirect').length,role==='admin'?1:0);if(role==='admin')assert.equal(tree.root.findByType('redirect').props.to,'/setup');}finally{await cleanup();}
    }
});
test('legacy and completed hosts keep dashboard access, including empty domain inventories',async()=>{
    for(const status of ['legacy','ready']){init('admin',{status,origin:'legacy',required:false});try{await mount(Gate);assert.equal(tree.root.findAllByProps({'data-management':true}).length,1);assert.equal(tree.root.findAllByType('redirect').length,0);}finally{await cleanup();}}
});
test('failed or malformed assessment does not guess fresh state or expose management',async()=>{
    for(const reply of [()=>Response.json({}, {status:503}),()=>Response.json({status:'new'}),()=>{throw new Error('offline')}]){
        init();globalThis.fetch=async()=>reply();try{await mount(Gate);assert.equal(tree.root.findAllByType('redirect').length,0);assert.equal(tree.root.findAllByProps({'data-management':true}).length,0);assert.ok(findButton('common.retry'));}finally{await cleanup();}
    }
});
test('opening the wizard performs only read requests; purpose and plan are persisted before installation',async()=>{
    init();try{await mount();assert.ok(calls.every(call=>!call.options?.method||call.options.method==='GET'));await toReview();assert.equal(state.draft.panel_domain,'panel.example.com');assert.equal(state.draft.dns_mode,'external');assert.equal(state.draft.main_domain,'example.com');assert.equal(calls.filter(call=>call.options?.method==='PUT').length,6,'each wizard step saves the draft');assert.equal(calls.some(call=>call.url==='/api/v1/setup/start'),false);assert.equal(findButton('setup.start').props.disabled,true);}finally{await cleanup();}
});
test('a blocked plan cannot run, and changing material inputs requires a new review',async()=>{
    init();const read=fetch;globalThis.fetch=async(url,options)=>url==='/api/v1/setup/plan'?Response.json(plan({blockers:['server_setup_dns_mode_unsupported'],can_start:false})):read(url,options);
    try{await mount();await toReview();assert.equal(findButton('setup.start').props.disabled,true);assert.equal(tree.root.findAllByProps({type:'checkbox'}).length,0);await act(async()=>findButton('setup.back').props.onClick());assert.equal(findButton('setup.start'),undefined);assert.equal(tree.root.findByProps({name:'setup-dns',value:'external'}).props.checked,true);}finally{await cleanup();}
});
test('a lost start response reconciles exact request identity and never starts twice',async()=>{
    init();let stored;const read=fetch;
    globalThis.fetch=async(url,options)=>{
        if(url==='/api/v1/setup/start'){calls.push({url,options});stored=JSON.parse(options.body);assert.ok(store.size>0,'recovery marker persisted before mutation');throw new Error('response lost');}
        if(url.includes('/setup/operation?request_id=')){calls.push({url,options});assert.ok(url.endsWith(stored.request_id));return Response.json(execution(stored));}
        return read(url,options);
    };
    try{await mount();await toReview();await act(async()=>tree.root.findByProps({type:'checkbox'}).props.onChange({target:{checked:true}}));const start=findButton('setup.start');await act(async()=>{void start.props.onClick();void start.props.onClick();});assert.equal(calls.filter(call=>call.url==='/api/v1/setup/start').length,1);assert.equal(tree.root.findAllByType('form').length,0);assert.ok(JSON.stringify(tree.toJSON()).includes('setup.installing'));}finally{await cleanup();}
});
test('operation success alone cannot show completion when the server readiness state is not ready',async()=>{
    init('admin',{status:'waiting'});const saved={plan_id:'a'.repeat(32),request_id:'b'.repeat(32),panel_domain:'panel.example.com'};store.set('celikpanel.setup.start.admin',JSON.stringify(saved));const read=fetch;
    globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(execution(saved,'succeeded')):read(url,options);
    try{await mount();assert.ok(JSON.stringify(tree.toJSON()).includes('setup.verificationFailed'));assert.ok(!JSON.stringify(tree.toJSON()).includes('setup.completeTitle'));assert.ok(!JSON.stringify(tree.toJSON()).includes('setup.operation.succeeded'));}finally{await cleanup();}
});
test('execution decoder rejects an unrelated operation and unsafe panel links',()=>{
    const marker={request_id:'b'.repeat(32),plan_id:'a'.repeat(32),panel_domain:'panel.example.com'};
    assert.ok(operations.decodeSetupExecution(execution(marker),marker));
    assert.equal(operations.decodeSetupExecution({...execution(marker),plan_id:'f'.repeat(32)},marker),null);
    assert.equal(operations.safeSetupPanelURL('https://attacker.example/setup','panel.example.com'),null);
    assert.equal(operations.safeSetupPanelURL('javascript:alert(1)','panel.example.com'),null);
    assert.equal(operations.safeSetupPanelURL('https://panel.example.com:2083/','panel.example.com'),'https://panel.example.com:2083/');
});

test('waiting plans can be revised only after server proof; rejection preserves the running context',async()=>{
    for(const permitted of [false,true]){
        init('admin',{status:'waiting'});
        state.draft.panel_domain='panel.example.com';
        const marker={plan_id:'a'.repeat(32),request_id:'b'.repeat(32),panel_domain:'panel.example.com'};
        store.set('celikpanel.setup.start.admin',JSON.stringify(marker));
        const running={...execution(marker,'waiting'),phase:'verification',steps:[{id:'verify',kind:'verify',target:'web',status:'succeeded'}]};
        const read=fetch;
        globalThis.fetch=async(url,options)=>{
            if(url.includes('/setup/operation'))return Response.json(running);
            if(url==='/api/v1/setup/revise'){
                calls.push({url,options});
                assert.deepEqual(JSON.parse(options.body),{revision:state.revision,execution_id:running.id});
                if(!permitted)return Response.json({code:'setup_not_ready'},{status:409});
                state={...state,revision:state.revision+1,status:'draft'};
                return Response.json(state);
            }
            return read(url,options);
        };
        try{
            await mount();await act(async()=>findButton('setup.editPlan').props.onClick());
            assert.equal(calls.filter(call=>call.url==='/api/v1/setup/revise').length,1);
            assert.equal(tree.root.findAllByType('form').length,permitted?1:0);
            assert.equal(store.size,permitted?0:1);
            if(permitted)assert.ok(tree.root.findByProps({name:'setup-dns',value:'external'}));
            else assert.ok(JSON.stringify(tree.toJSON()).includes('setup.reviseBlocked'));
        }finally{await cleanup();}
    }
});


test('application setup selects an exact official LTS version; unavailable lists offer retry without installation',async()=>{
    for (const available of [true,false]) {
        init();const base=globalThis.fetch;
        globalThis.fetch=async(url,options)=>{
            if(url==='/api/v1/runtimes/node/lts'){calls.push({url,options});return available?Response.json({releases:[{version:'24.18.0',name:'Fixture'}]}):Response.json({}, {status:502});}
            return base(url,options);
        };
        try{
            await mount();assert.equal(calls.some(call=>call.url==='/api/v1/runtimes/node/lts'),false);
            await type('setup-main_domain','example.com');await submit();await submit();await submit();
            await act(async()=>findButton('setup.components.customize').props.onClick());await toggleComponent('node');
            const selector=tree.root.findByProps({id:'setup-node_version'});assert.equal(selector.type,'select');assert.equal(selector.props.disabled,!available);
            if(available){await act(async()=>selector.props.onChange({target:{value:'24.18.0'}}));assert.equal(tree.root.findByProps({id:'setup-node_version'}).props.value,'24.18.0');}
            else assert.ok(findButton('common.retry'));
            assert.equal(calls.some(call=>call.url==='/api/v1/setup/start'),false);
        }finally{await cleanup();}
    }
});


test('license waiting cannot reopen an unfinished setup plan and mail certificate steps decode',async()=>{
    init();const current={...execution({plan_id:'a'.repeat(32),request_id:'c'.repeat(32)},'waiting'),phase:'license',error:{code:'license_required',message:'activate'},steps:[{id:'mail-tls',kind:'mail_certificate',target:'mail.example.com',status:'pending'}]};
    const base=globalThis.fetch;globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(current):base(url,options);
    try{await mount();assert.equal(findButton('setup.editPlan'),undefined);assert.equal(findButton('setup.verify'),undefined);assert.equal(tree.root.findAllByProps({to:'/settings?section=license'}).length,2);assert.ok(operations.decodeSetupExecution(current));}finally{await cleanup();}
});


test('unverified peer IPv6 is shown as a setup limitation without DNS changes',async()=>{
    init();const current={...execution({plan_id:'a'.repeat(32),request_id:'c'.repeat(32)},'waiting'),phase:'verification',checks:[{id:'dns',state:'action_required',code:'dns_peer_ipv6_unverified'}]};
    const base=globalThis.fetch;globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(current):base(url,options);
    try{await mount();const text=JSON.stringify(tree.toJSON());assert.ok(text.includes('setup.blocker.peerIPv6'));assert.equal(text.includes('setup.blocker.unknown'),false);assert.ok(calls.every(call=>!call.options?.method||call.options.method==='GET'));}finally{await cleanup();}
});


test('remote DNS setup requires fresh proof and reviews only the selected saved connection',async()=>{
    init();const id='d'.repeat(32);const base=globalThis.fetch;
    globalThis.fetch=async(url,options)=>url==='/api/v1/setup/plan'?Response.json(plan({remote_dns_connection:{id,endpoint:'https://dns.example.com:2083',nameservers:['ns1.example.com','ns2.example.com']}})):base(url,options);
    try{
        await mount();await toDNSHome();
        await act(async()=>tree.root.findByProps({name:'setup-dns',value:'existing'}).props.onChange());
        await act(async()=>tree.root.findByType('remote-connection').props.onChange(id));
        const before=calls.length;await submit();assert.equal(calls.length,before,'unverified selection cannot persist or review setup');
        assert.ok(JSON.stringify(tree.toJSON()).includes('setup.remote.proof.failed'));
        await act(async()=>tree.root.findByType('remote-connection').props.onValidityChange(true));
        await submit();
        assert.equal(state.draft.remote_dns_connection_id,id);
        assert.ok(JSON.stringify(tree.toJSON()).includes('https://dns.example.com:2083'));
        assert.equal(findButton('setup.start').props.disabled,true,'review still requires explicit installation confirmation');
        assert.equal(calls.some(call=>call.url==='/api/v1/setup/start'),false);
        assert.equal(store.size,0,'review does not persist credentials or an execution marker');
    }finally{await cleanup();}
});

test('remote DNS review rejects a plan bound to a different saved connection',async()=>{
    init();const base=globalThis.fetch;
    globalThis.fetch=async(url,options)=>url==='/api/v1/setup/plan'?Response.json(plan({remote_dns_connection:{id:'e'.repeat(32),endpoint:'https://other.example.com:2083',nameservers:['ns1.example.com']}})):base(url,options);
    try{
        await mount();await toDNSHome();
        await act(async()=>tree.root.findByProps({name:'setup-dns',value:'existing'}).props.onChange());
        await act(async()=>tree.root.findByType('remote-connection').props.onChange('d'.repeat(32)));
        await act(async()=>tree.root.findByType('remote-connection').props.onValidityChange(true));
        await submit();
        assert.equal(findButton('setup.start'),undefined);
        assert.ok(JSON.stringify(tree.toJSON()).includes('setup.planFailed'));
        assert.equal(calls.some(call=>call.url==='/api/v1/setup/start'),false);
    }finally{await cleanup();}
});

test('pending receipt reconciliation is neutral progress and preserves the operation admission lock',async()=>{
    for(const status of ['running','failed']){
        init('admin',{status});
        const marker={plan_id:'a'.repeat(32),request_id:'b'.repeat(32),panel_domain:'panel.example.com'};
        store.set('celikpanel.setup.start.admin',JSON.stringify(marker));
        const current={...execution(marker,status),phase:'dns',error:{code:'server_setup_reconciling',message:'Pending exact receipt'}};
        const read=fetch;
        globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(current):read(url,options);
        try{
            await mount();
            const notice=tree.root.findAllByProps({role:status==='running'?'status':'alert'}).find(node=>node.findAllByType('p').some(p=>p.props.children===(status==='running'?'setup.guide.confirm':'setup.confirmingPrevious')));
            assert.ok(notice,'pending running receipt is status; terminal failure remains an alert');
            assert.equal(notice.findAllByType('p')[0].props.className.includes('text-danger'),status==='failed');
            assert.equal(tree.root.findAllByType('form').length,0);
            assert.equal(findButton('setup.editPlan'),undefined);
            assert.equal(findButton('setup.start'),undefined);
            assert.equal(findButton('setup.reconnect'),undefined);
            assert.equal(!!findButton('setup.revise'),status==='failed');
            assert.equal(store.get('celikpanel.setup.start.admin'),JSON.stringify(marker));
            assert.ok(calls.every(call=>!call.options?.method||call.options.method==='GET'),'reading reconciliation cannot launch or revise any work');
        }finally{await cleanup();}
    }
});


test('an undecided upgraded host gets a choice, while manual and completed hosts keep access',async()=>{
    for(const [status,guidance,redirect] of [['legacy','undecided',true],['new','manual',false],['legacy','manual',false],['ready','undecided',false],['running','manual',true]]){
        init('admin',{status,guidance,origin:'legacy',required:!['legacy','ready'].includes(status)});
        try {await mount(Gate);assert.equal(tree.root.findAllByType('redirect').length,redirect?1:0);assert.ok(calls.every(c=>!c.options?.method));}
        finally {await cleanup();}
    }
    assert.equal(setup.decodeServerSetup({...fresh(),guidance:'skip_checks'}),null);
});

test('manual choice is persisted once before leaving and does not complete or install anything',async()=>{
    init('admin',{status:'legacy',origin:'legacy',required:false,guidance:'undecided'});
    const read=fetch;
    globalThis.fetch=async(url,options)=>{
        if(url==='/api/v1/setup/guidance'){
            calls.push({url,options});const choice=JSON.parse(options.body);
            assert.deepEqual(choice,{revision:1,guidance:'manual'});
            state={...state,guidance:'manual',revision:2};return Response.json(state);
        }
        return read(url,options);
    };
    try {
        await mount();assert.ok(findButton('setup.useWizard'));assert.ok(findButton('setup.manual'));assert.equal(calls.length,0);
        const button=findButton('setup.manual');await act(async()=>{void button.props.onClick();void button.props.onClick();});
        assert.equal(calls.length,1);assert.equal(tree.root.findByType('redirect').props.to,'/');
        assert.equal(state.status,'legacy');assert.equal(state.guidance,'manual');assert.equal(store.size,0);
    } finally {await cleanup();}
});

test('guidance failure stays visible and a guided choice can reopen a manually configured server',async()=>{
    init('admin',{guidance:'manual'});
    const read=fetch;let fail=true;
    globalThis.fetch=async(url,options)=>{
        if(url==='/api/v1/setup/guidance'){
            if(fail)return Response.json({error:'conflict'},{status:409});
            state={...state,guidance:'guided',revision:state.revision+1};return Response.json(state);
        }
        return read(url,options);
    };
    try {
        await mount();await act(async()=>findButton('setup.useWizard').props.onClick());
        assert.equal(tree.root.findAllByType('redirect').length,0);assert.ok(JSON.stringify(tree.toJSON()).includes('setup.choiceFailed'));
        fail=false;await act(async()=>findButton('setup.useWizard').props.onClick());
        assert.equal(tree.root.findAllByProps({id:'setup-main_domain'}).length,1);
        assert.equal(state.guidance,'guided');assert.equal(state.status,'new');
    } finally {await cleanup();}
});

async function mountPicker(catalog, initial = []) {
    let last = initial;
    function PickerHarness() {
        const [selected, setSelected] = React.useState(initial);
        return React.createElement(ComponentPicker, { catalog, selected, disabled: false, onChange(next) { last = next; setSelected(next); } });
    }
    await act(async () => { tree = Renderer.create(React.createElement(PickerHarness)); });
    return () => last;
}
const component = id => tree.root.findByProps({ id: `setup-component-${id}` });
async function toggleComponent(id) { await act(async () => component(id).props.onChange()); }
async function choosePurpose(purpose) { await act(async () => tree.root.findByProps({ name: 'setup-purpose', value: purpose }).props.onChange()); }
const formNext = () => tree.root.findByProps({ type: 'submit' });

test('component picker can remove its paired mail choice and locks only dependencies of other choices', async () => {
    init();
    try {
        const selected = await mountPicker(setupCatalogFixture());
        assert.equal(tree.root.findAllByProps({ id: 'setup-component-dovecot' }).length, 0, 'paired services have one choice');
        await toggleComponent('postfix');
        assert.equal(component('postfix').props.checked, true);
        assert.equal(component('postfix').props.disabled, false, 'a reciprocal dependency must not trap its own choice');
        await toggleComponent('postfix');
        assert.deepEqual(selected(), []);
        await toggleComponent('roundcube');
        assert.equal(component('postfix').props.checked, true);
        assert.equal(component('postfix').props.disabled, true, 'webmail needs the mail pair');
        assert.equal(component('nginx').props.checked, true);
        assert.equal(component('nginx').props.disabled, true);
        await toggleComponent('roundcube');
        assert.equal(component('postfix').props.checked, false);
        assert.equal(component('postfix').props.disabled, false);
        assert.deepEqual(selected(), []);
        assert.equal(calls.length, 0, 'component selection performs no host operation');
    } finally { await cleanup(); }
});

test('component picker blocks conflicting and unsupported choices and preserves installed software', async () => {
    init();const catalog = setupCatalogFixture();
    catalog.components.find(row => row.id === 'nginx').installed = true;
    try {
        const selected = await mountPicker(catalog, ['nginx']);
        assert.ok(JSON.stringify(tree.toJSON()).includes('setup.components.installed'));
        assert.equal(tree.root.findAllByProps({ id: 'setup-component-nftables' }).length, 0);
        assert.equal(tree.root.findAllByProps({ id: 'setup-component-certbot' }).length, 0);
        assert.equal(component('clamav').props.disabled, true);
        await toggleComponent('redis');
        assert.equal(component('valkey').props.disabled, true);
        assert.equal(component('redis').props.disabled, false);
        await toggleComponent('redis');
        assert.equal(component('valkey').props.disabled, false);
        assert.equal(tree.root.findAllByProps({ id: 'setup-component-nginx' }).length, 0, 'installed software has no box');
        assert.ok(JSON.stringify(tree.toJSON()).includes('setup.components.installedKeep'));
        assert.deepEqual(selected(), ['nginx']);
        assert.equal(catalog.components.find(row => row.id === 'nginx').installed, true);
        assert.ok(JSON.stringify(tree.toJSON()).includes('setup.components.preserve'));
        assert.equal(calls.length, 0, 'deselecting installed software cannot call removal or installation APIs');
    } finally { await cleanup(); }
});

test('unknown inventory disables new choices without inventing installed state', async () => {
    init();const catalog = setupCatalogFixture();catalog.inventory_state = 'unknown';
    try {
        await mountPicker(catalog);
        assert.ok(tree.root.findAllByProps({ type: 'checkbox' }).every(input => input.props.disabled));
        assert.ok(JSON.stringify(tree.toJSON()).includes('setup.components.inventoryUnknown'));
        assert.equal(JSON.stringify(tree.toJSON()).includes('setup.components.installed'), false);
        assert.equal(calls.length, 0);
    } finally { await cleanup(); }
});

test('draft decoding distinguishes an untouched preset from explicit empty customization', () => {
    const preset = fresh();
    assert.equal(setup.decodeServerSetup(preset).draft.customization, undefined);
    const custom = { ...preset, draft: { ...preset.draft, purpose: 'custom', customization: { components: [] } } };
    assert.deepEqual(setup.decodeServerSetup(custom).draft.customization, { components: [] });
    for (const customization of [null, {}, { components: null }, { components: [1] }, { components: ['redis', 'redis'] }]) {
        assert.equal(setup.decodeServerSetup({ ...custom, draft: { ...custom.draft, customization } }), null);
    }
});

test('completion links and labels follow customized services instead of the original profile', async () => {
    const scenarios = [
        ['web', ['fail2ban'], '/services', 'setup.nextComponents'],
        ['custom', [], '/dns-infrastructure', 'setup.nextDNS'],
        ['custom', ['nginx'], '/domains', 'setup.nextWebsite'],
        ['web', ['node'], '/domains', 'setup.nextApplication'],
        ['web', undefined, '/domains', 'setup.nextWebsite'],
        ['dns', undefined, '/dns-infrastructure', 'setup.nextDNS'],
    ];
    for (const [purpose, selected, expectedPath, label] of scenarios) {
        const draft = { ...fresh().draft, purpose, ...(selected === undefined ? {} : { customization: { components: selected } }) };
        init('admin', { status: 'ready', required: false, draft });
        try {
            await mount();
            assert.equal(tree.root.findAllByType('a').filter(link => link.props.href === expectedPath).length, 1, `${purpose}/${selected} next action`);
            assert.ok(JSON.stringify(tree.toJSON()).includes(label), `${purpose}/${selected} label`);
            assert.equal(tree.root.findAllByType('form').length, 0);
            assert.ok(calls.every(call => !call.options?.method || call.options.method === 'GET'));
        } finally { await cleanup(); }
    }
});

// alpha.115: the end of setup offers the first site, prefilled with the main
// domain, only when websites were set up; it is the existing form and sends nothing.
test('a finished setup with websites offers the first site with the main domain; without websites or a main domain it does not', async () => {
    for (const [purpose, main, offered] of [['web', 'Example.COM', true], ['dns', 'example.com', false], ['web', '', false]]) {
        const draft = { ...fresh().draft, purpose, ...(main ? { main_domain: main } : {}) };
        init('admin', { status: 'ready', required: false, draft });
        try {
            await mount();
            const offers = tree.root.findAllByType('first-site');
            assert.equal(offers.length, offered ? 1 : 0, `${purpose}/${main}`);
            if (offered) assert.equal(offers[0].props.domain, 'example.com');
            assert.ok(calls.every(call => !call.options?.method || call.options.method === 'GET'), 'the offer sends nothing by itself');
        } finally { await cleanup(); }
    }
});

test('DNS role changes preserve physical server names, addresses and unrelated draft fields', () => {
    const draft = {...fresh().draft, ns1:'ns2.example.com', ns2:'ns1.example.com', peer_ns:'ns1.example.com', local_ip:'72.62.38.15', peer_ip:'2.25.80.4'};
    const secondary = setup.changeSetupDNSRole(draft, 'secondary');
    assert.equal(secondary.ns2, draft.ns1);
    assert.equal(secondary.ns1, draft.ns2);
    assert.equal(secondary.local_ip, draft.local_ip);
    assert.equal(secondary.peer_ip, draft.peer_ip);
    assert.equal(secondary.peer_ns, draft.peer_ns);
    assert.equal(setup.setupDNSNames(secondary).mismatch, false);
    assert.deepEqual(setup.changeSetupDNSRole(secondary, 'primary'), draft);
    assert.equal(setup.changeSetupDNSRole(draft, 'primary'), draft);
    assert.equal(setup.setupDNSNames({...draft, peer_ns:' NS1.Example.COM. '}).mismatch, false);
});

test('only a usable server-reported IPv4 is offered for prefilling', () => {
    for (const value of [undefined, '', '10.0.0.1', '127.0.0.1', '169.254.2.1', '172.16.0.2', '192.168.1.3', '100.64.1.1', '224.0.0.1', '0.1.2.3', '256.1.1.1', '1.2.3', '01.2.3.4', '2001:db8::1']) assert.equal(setup.setupDetectedIPv4(value), '');
    assert.equal(setup.setupDetectedIPv4('72.62.38.15'), '72.62.38.15');
});

test('waiting secondary keeps its reviewed endpoint and binds only an explicitly verified connection',async()=>{
    for(const lost of [false,true]) {
        init('admin',{status:'running',draft:{...fresh().draft,purpose:'web_mail',dns_role:'secondary'}});
        const marker={plan_id:'a'.repeat(32),request_id:'b'.repeat(32)};
        let op={...execution(marker,'waiting'),phase:'dns_publisher',steps:[{id:'01-dns',kind:'dns',target:'local',status:'succeeded'},{id:'02-publisher',kind:'dns_publisher',target:'https://primary.example.com:2083',status:'running'},{id:'03-mail',kind:'mail_profile',target:'core',status:'pending'}]};
        const read=fetch; let bound=false;
        globalThis.fetch=async(url,options)=>{
            if(url==='/api/v1/setup/operation'){calls.push({url,options});return Response.json(op);}
            if(url==='/api/v1/setup/publisher') {
                calls.push({url,options});
                assert.deepEqual(JSON.parse(options.body),{execution_id:op.id,connection_id:'d'.repeat(32)});
                bound=true;op={...op,status:'running',phase:'03-mail'};
                if(lost)throw new Error('reply lost after binding');
                return Response.json(op);
            }
            return read(url,options);
        };
        try {
            await mount();
            const picker=()=>tree.root.findByType('remote-connection');
            assert.equal(picker().props.requiredEndpoint,'https://primary.example.com:2083');
            assert.equal(findButton('setup.publisher.continue').props.disabled,true);
            assert.equal(calls.some(c=>c.url==='/api/v1/setup/publisher'),false);
            await act(async()=>picker().props.onChange('d'.repeat(32)));
            assert.equal(findButton('setup.publisher.continue').props.disabled,true);
            await act(async()=>picker().props.onValidityChange(true));
            assert.equal(findButton('setup.publisher.continue').props.disabled,false);
            await act(async()=>findButton('setup.publisher.continue').props.onClick());
            assert.equal(bound,true);
            assert.equal(calls.filter(c=>c.url==='/api/v1/setup/publisher').length,1);
            assert.equal(calls.some(c=>c.url==='/api/v1/setup/start'||c.url==='/api/v1/setup/revise'),false);
            assert.equal(tree.root.findAllByType('remote-connection').length,0);
        } finally {await cleanup();}
    }
});

async function remountWizard() {
    await act(async()=>tree.unmount());
    fixture.context.snapshot=state;
    await mount();
}

test('review survives remount with a fresh server plan and requires confirmation again',async()=>{
    init();try{
        await mount();await toReview();
        await act(async()=>tree.root.findByProps({type:'checkbox'}).props.onChange({target:{checked:true}}));
        const before=calls.length;
        await remountWizard();
        assert.ok(findButton('setup.start'));
        assert.equal(findButton('setup.start').props.disabled,true);
        assert.equal(tree.root.findByProps({type:'checkbox'}).props.checked,false);
        assert.equal(calls.slice(before).filter(call=>call.url==='/api/v1/setup/plan').length,1);
        assert.equal(calls.some(call=>call.url==='/api/v1/setup/start'),false);
        assert.equal(calls.slice(before).some(call=>call.options?.method==='PUT'),false);
    }finally{await cleanup();}
});

test('server operation wins over a saved review after reload',async()=>{
    init();try{
        await mount();await toReview();
        const before=calls.length, base=fetch;
        globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(execution({plan_id:'a'.repeat(32),request_id:'b'.repeat(32)})):base(url,options);
        await remountWizard();
        assert.equal(findButton('setup.start'),undefined);
        assert.ok(JSON.stringify(tree.toJSON()).includes('setup.installing'));
        assert.equal(calls.slice(before).some(call=>call.url==='/api/v1/setup/plan'),false);
        assert.equal(sessionStorage.getItem('celikpanel.setup.editor.admin'),null);
    }finally{await cleanup();}
});

test('editor checkpoints cannot override changed server revisions, completed setup, or another user',async()=>{
    init();try{
        await mount();await toReview();
        const raw=sessionStorage.getItem('celikpanel.setup.editor.admin');
        assert.ok(setup.decodeSetupEditorCheckpoint(raw,state));
        assert.equal(setup.decodeSetupEditorCheckpoint(raw,{...state,revision:state.revision+1}),null);
        assert.equal(setup.decodeSetupEditorCheckpoint(raw,{...state,status:'ready',required:false}),null);
        assert.equal(setup.decodeSetupEditorCheckpoint('{broken',state),null);
        const edited=JSON.parse(raw);edited.draft.panel_domain='changed.example.com';
        assert.equal(setup.decodeSetupEditorCheckpoint(JSON.stringify(edited),state),null);
        fixture.auth.user.username='another-admin';
        await remountWizard();
        assert.equal(findButton('setup.start'),undefined);
    }finally{await cleanup();}
});

test('unavailable session storage does not prevent normal wizard use',async()=>{
    init();globalThis.sessionStorage={getItem(){throw new Error('denied')},setItem(){throw new Error('denied')},removeItem(){throw new Error('denied')}};
    try{await mount();await toReview();assert.ok(findButton('setup.start'));}finally{await cleanup();}
});


test('optional progress context tolerates nonlocal roles and drops malformed metadata without hiding execution',()=>{
    const current=execution({plan_id:'a'.repeat(32),request_id:'b'.repeat(32)});
    const context={dns_mode:'external',dns_role:'',dns_engine:'',local_nameserver:'',local_ip:'',peer_nameserver:'',peer_ip:'',panel_domain:'panel.example.com',mail_hostname:'',dns_hosting_management:''};
    assert.deepEqual(operations.decodeSetupExecution({...current,context}).context,context);
    for(const invalid of [null,{}, {...context,dns_role:'wrong'}, {...context,peer_ip:42}]){
        const decoded=operations.decodeSetupExecution({...current,context:invalid});
        assert.equal(decoded.id,current.id);assert.equal(decoded.context,undefined);
    }
});

test('resumed DNS guidance is above the step list and uses the saved plan context without a new start',async()=>{
    init('admin',{status:'running',guidance:'guided'});
    state.draft.peer_ip='203.0.113.99';
    const current={...execution({plan_id:'a'.repeat(32),request_id:'b'.repeat(32)}),phase:'01-dns',
        context:{dns_mode:'local',dns_role:'primary',dns_engine:'bind',local_nameserver:'ns1.example.com',local_ip:'192.0.2.10',peer_nameserver:'ns2.example.com',peer_ip:'192.0.2.20',panel_domain:'panel.example.com',mail_hostname:'',dns_hosting_management:''},
        steps:[{id:'01-dns',kind:'dns',target:'local',status:'running'}]};
    const base=fetch;globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(current):base(url,options);
    try{
        await mount();
        const notice=tree.root.findByProps({'aria-labelledby':'setup-guidance-title'});
        assert.ok(notice.findAllByType('p').some(item=>item.props.children==='setup.guide.startSecondary'));
        const rendered=JSON.stringify(tree.toJSON());
        assert.ok(rendered.indexOf('setup-guidance-title')<rendered.indexOf('mt-6 divide-y divide-border'));
        assert.ok(calls.every(call=>!call.options?.method||call.options.method==='GET'));
    }finally{await cleanup();}
});


const infrastructurePlan = () => ({zone:'example.com',expected_digest:'d'.repeat(64),records:[
    {name:'example.com',type:'NS',content:'ns1.example.com.',ttl:300,action:'keep'},
    {name:'panel.example.com',type:'A',content:'192.0.2.10',ttl:300,action:'add'},
    {name:'boston.example.com',type:'A',content:'192.0.2.20',ttl:300,action:'add'},
]});
const primaryDraft = () => ({...fresh().draft,panel_domain:'panel.example.com',ns1:'ns1.example.com',ns2:'ns2.example.com',peer_ns:'ns2.example.com',local_ip:'192.0.2.10',peer_ip:'192.0.2.20'});

test('DNS record plans reject malformed mutation details but optional execution metadata cannot hide an active operation',()=>{
    init();
    const reviewed=plan({infrastructure_dns:infrastructurePlan(),server_ip:'192.0.2.10'});
    assert.ok(operations.decodeSetupPlan(reviewed,state.revision));
    for(const bad of [null,{}, {...infrastructurePlan(),records:[{name:'example.com',type:'A',content:'192.0.2.10',ttl:300,action:'delete'}]}, {...infrastructurePlan(),expected_digest:42}]){
        assert.equal(operations.decodeSetupPlan({...reviewed,infrastructure_dns:bad},state.revision),null);
    }
    const current=execution({plan_id:'a'.repeat(32),request_id:'b'.repeat(32)});
    const context={dns_mode:'local',dns_role:'primary',dns_engine:'bind',local_nameserver:'ns1.example.com',local_ip:'192.0.2.10',peer_nameserver:'ns2.example.com',peer_ip:'192.0.2.20',panel_domain:'panel.example.com',mail_hostname:'',dns_hosting_management:'',access_dns_ip:'192.0.2.10',infrastructure_dns:infrastructurePlan()};
    assert.equal(operations.decodeSetupExecution({...current,context}).context.infrastructure_dns.zone,'example.com');
    const preserved=operations.decodeSetupExecution({...current,context:{...context,infrastructure_dns:{invalid:true}}});
    assert.equal(preserved.id,current.id);assert.equal(preserved.context,undefined);
});

test('public DNS waiting retains exact operation and exposes safe in-wizard plan revision',async()=>{
    init('admin',{status:'waiting'});
    state.draft.panel_domain='edited.invalid.example';
    const current={...execution({plan_id:'a'.repeat(32),request_id:'b'.repeat(32)},'waiting'),phase:'access_dns',steps:[{id:'access',kind:'access_dns',target:'panel.example.com',qualifier:'192.0.2.10',status:'running'}],error:{code:'server_setup_access_dns_required',message:'not verified'}};
    const base=fetch;globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(current):base(url,options);
    try{
        await mount();
        assert.ok(findButton('setup.editPlan'));
        assert.ok(JSON.stringify(tree.toJSON()).includes('setup.guide.accessDNSRecord'));
        assert.ok(JSON.stringify(tree.toJSON()).includes('setup.guide.accessDNSResume'));
        assert.equal(tree.root.findAllByType('form').length,0);
        assert.ok(calls.every(c=>!c.options?.method||c.options.method==='GET'));
    }finally{await cleanup();}
});


test('final verification stays waiting and manual checks report unchanged, unknown, passed or unavailable evidence without installation',async()=>{
    for(const outcome of ['action_required','unknown','ready','empty','error']){
        init('admin',{status:'waiting'});
        const marker={plan_id:'a'.repeat(32),request_id:'b'.repeat(32),panel_domain:'panel.example.com'};
        store.set('celikpanel.setup.start.admin',JSON.stringify(marker));
        const oldCheck={id:'panel_https',state:'action_required',code:'panel_https_required'};
        const waiting={...execution(marker,'waiting'),phase:'verification',steps:[{id:'verify',kind:'verify',target:'web',status:'succeeded'}],checks:[oldCheck]};
        const read=fetch;
        globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(waiting):read(url,options);
        let finish, reject, manual=false;
        const reloads=[];
        fixture.context.reload=(checks)=>{reloads.push(checks);return manual?new Promise((resolve,fail)=>{finish=resolve;reject=fail;}):Promise.resolve(state);};
        try{
            await mount();
            const row=tree.root.findAllByType('ol').find(item=>item.props['aria-live']==='polite');
            assert.ok(JSON.stringify(row.findAllByType('span').map(item=>item.props.children.filter(child=>typeof child==='string'))).includes('setup.verifyWaiting'));
            assert.ok(!JSON.stringify(row.findAllByType('span').map(item=>item.props.children.filter(child=>typeof child==='string'))).includes('setup.operation.succeeded'));
            manual=true;
            await act(async()=>{void findButton('setup.verify').props.onClick();});
            assert.equal(findButton('setup.verifyChecking').props.disabled,true);
            await act(async()=>{
                if(outcome==='error')reject(new Error('offline'));
                else finish({...state,checks:outcome==='empty'?[]:[{id:'mail_identity',state:outcome,code:outcome==='ready'?'ready':outcome==='unknown'?'mail_identity_unavailable':'mail_identity_required'}]});
            });
            // The owner's check is the explicit one; page reads are not.
            assert.equal(reloads.at(-1),'manual');
            assert.ok(reloads.slice(0,-1).every(value=>value!=='manual'));
            // Every check passed: the server is finishing, so there is nothing to press.
            if(outcome==='ready')assert.equal(findButton('setup.verify'),undefined);
            else assert.equal(findButton('setup.verify').props.disabled,false);
            const message=outcome==='error'||outcome==='empty'?'setup.verificationFailed':outcome==='unknown'?'setup.verifyUnknown':outcome==='ready'?'setup.finishing':'setup.verifyStillWaiting';
            const result=tree.root.findAllByProps({role:'status'}).find(item=>item.props['aria-atomic']==='true');
            const said=[...result.findAllByType('p'),...result.findAllByType('span'),...result.findAllByType('li')].flatMap(item=>[item.props.children].flat()).filter(child=>typeof child==='string');
            assert.ok(said.includes(message),outcome);
            // The box says each open check as the run's guidance does (2026-10-16):
            // a check that could not be read is said as not read, not as missing.
            if(outcome==='action_required')assert.ok(said.includes('setup.blocker.mailIdentity'));
            if(outcome==='unknown')assert.ok(said.includes('setup.check.notRead')&&!said.includes('setup.blocker.mailIdentity'));
            assert.ok(!JSON.stringify(tree.toJSON()).includes('setup.completeTitle'));
            assert.ok(calls.every(call=>!call.options?.method||call.options.method==='GET'));
            assert.equal(store.size,1);
        }finally{await cleanup();}
    }
});


// Gap 8 of the second-server work (2026-10-16): after "Check requirements
// again" the box under the button says the typed reason of an open check, the
// same sentence the run's guidance uses, instead of the generic one.
test('the manual re-check box says the typed reason sentence the guidance uses',async()=>{
    init('admin',{status:'waiting'});
    const marker={plan_id:'a'.repeat(32),request_id:'b'.repeat(32),panel_domain:'panel.example.com'};
    store.set('celikpanel.setup.start.admin',JSON.stringify(marker));
    const typed={id:'mail_identity',state:'action_required',code:'mail_identity_required',reason:'reverse_dns_mismatch',vars:{hostname:'mail.example.com',ip:'203.0.113.7',ptr:'old.example.net',resolver:'1.1.1.1'}};
    const waiting={...execution(marker,'waiting'),phase:'verification',steps:[{id:'verify',kind:'verify',target:'web',status:'succeeded'}],checks:[typed]};
    const read=fetch;
    globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(waiting):read(url,options);
    let finish, manual=false;
    fixture.context.reload=()=>manual?new Promise(resolve=>{finish=resolve;}):Promise.resolve(state);
    try{
        await mount();
        manual=true;
        await act(async()=>{void findButton('setup.verify').props.onClick();});
        await act(async()=>{finish({...state,checks:[typed]});});
        const result=tree.root.findAllByProps({role:'status'}).find(item=>item.props['aria-atomic']==='true');
        const items=result.findAllByType('li').flatMap(item=>[item.props.children].flat()).filter(child=>typeof child==='string');
        assert.ok(items.includes('setup.check.mailIdentity.reverseDNSFrom'),JSON.stringify(items));
        // Negative control: the generic sentence is not said for a typed reason.
        assert.ok(!items.includes('setup.blocker.mailIdentity'),JSON.stringify(items));
        const guidance=JSON.stringify(tree.root.findAllByProps({'data-setup-checks':''}).map(list=>list.findAllByType('li').flatMap(item=>[item.props.children].flat()).filter(child=>typeof child==='string')));
        assert.ok(guidance.includes('setup.check.mailIdentity.reverseDNSFrom'),guidance);
    }finally{await cleanup();}
});

test('the answer to the manual re-check is brought into view when it arrives, not while it is read',async()=>{
    init('admin',{status:'waiting'});
    const marker={plan_id:'a'.repeat(32),request_id:'b'.repeat(32),panel_domain:'panel.example.com'};
    store.set('celikpanel.setup.start.admin',JSON.stringify(marker));
    const typed={id:'mail_identity',state:'action_required',code:'mail_identity_required',reason:'reverse_dns_mismatch',vars:{hostname:'mail.example.com',ip:'203.0.113.7',ptr:'old.example.net',resolver:'1.1.1.1'}};
    const waiting={...execution(marker,'waiting'),phase:'verification',steps:[{id:'verify',kind:'verify',target:'web',status:'succeeded'}],checks:[typed]};
    const read=fetch;
    globalThis.fetch=async(url,options)=>url.includes('/setup/operation')?Response.json(waiting):read(url,options);
    let finish, manual=false;
    fixture.context.reload=()=>manual?new Promise(resolve=>{finish=resolve;}):Promise.resolve(state);
    // Each host node gets the least a ref is asked for; scrolls are recorded.
    const scrolled=[];
    const createNodeMock=element=>({focus(){},scrollIntoView(options){scrolled.push({type:element.type,className:String(element.props.className||''),options});}});
    try{
        await act(async()=>{tree=Renderer.create(React.createElement(Wizard,null,React.createElement('section',{'data-management':true})),{createNodeMock});});
        manual=true;
        await act(async()=>{void findButton('setup.verify').props.onClick();});
        const box=()=>scrolled.filter(item=>item.type==='div'&&item.className.includes('scroll-mb-6'));
        assert.equal(box().length,0,'the box was scrolled to while the check was still being read');
        await act(async()=>{finish({...state,checks:[typed]});});
        assert.deepEqual(box().map(item=>item.options),[{block:'nearest'}]);
    }finally{await cleanup();}
});

test('recorded mail enrollment survives reload and continues only by explicit click with the same step', async()=>{
    for (const outcome of ['accepted','lost','refused']) {
        init('admin',{status:'running'});
        const saved={plan_id:'a'.repeat(32),request_id:'b'.repeat(32),panel_domain:'panel.example.com'};
        store.set('celikpanel.setup.start.admin',JSON.stringify(saved));
        const running={...execution(saved),phase:'mail-renewal',steps:[{id:'mail-renewal',kind:'mail_enrollment',target:'mail-renewal',qualifier:'c'.repeat(64),status:'running'}],error:{code:'server_setup_mail_enrollment_running',message:'recorded'}};
        const read=fetch;let posted=0;
        globalThis.fetch=async(url,options)=>{
            if(url==='/api/v1/setup/mail-enrollment/continue'){
                posted++; assert.deepEqual(JSON.parse(options.body),{execution_id:running.id,step_id:'mail-renewal'});
                if(outcome==='lost')throw new Error('reply lost');
                return Response.json({handoff:'accepted'},{status:outcome==='refused'?409:202});
            }
            if(url.includes('/setup/operation'))return Response.json(running);
            return read(url,options);
        };
        try{
            await mount();assert.equal(posted,0);
            const button=findButton('setup.mailEnrollment.continue');assert.ok(button);
            assert.ok(!JSON.stringify(tree.toJSON()).includes('setup.blocker.unknown'),'recorded waiting is not a DNS failure');
            await act(async()=>{void button.props.onClick();void button.props.onClick();});
            assert.equal(posted,1,'double click/lost reply must not dispatch twice');
            assert.ok(JSON.stringify(tree.toJSON()).includes(outcome==='accepted'?'setup.mailEnrollment.checking':'setup.mailEnrollment.unknown'));
            assert.ok(!calls.some(call=>call.url==='/api/v1/setup/start'));
            await act(async()=>tree.unmount());tree=null;
            await mount();assert.equal(posted,1,'remount is observational');
        }finally{await cleanup();}
    }
});

test('unverified, terminal and mismatched enrollment states never offer continuation',()=>{
    const saved={plan_id:'a'.repeat(32),request_id:'b'.repeat(32)};
    const pending={...execution(saved),phase:'mail-renewal',steps:[{id:'mail-renewal',kind:'mail_enrollment',target:'mail-renewal',status:'running'}],error:{code:'server_setup_mail_enrollment_running',message:'recorded'}};
    assert.ok(operations.decodeSetupExecution(pending,saved));
    assert.ok(operations.continuableMailEnrollment(pending));
    assert.ok(operations.continuableMailEnrollment({...pending,error:{code:'server_setup_mail_enrollment_rollback',message:'restoring'}}));
    for(const code of ['server_setup_mail_enrollment_unknown','server_setup_mail_enrollment_not_recorded','server_setup_mail_enrollment_handoff','server_setup_reconciling']){
        assert.equal(operations.continuableMailEnrollment({...pending,error:{code,message:'unknown'}}),null);
    }
    for(const status of ['waiting','failed','succeeded'])assert.equal(operations.continuableMailEnrollment({...pending,status}),null);
    assert.equal(operations.continuableMailEnrollment({...pending,phase:'other'}),null);
});

test('unrecorded reviewed handoff retries only by explicit click and never repeats after lost reply or remount', async()=>{
    for (const outcome of ['accepted','lost','refused']) {
        init('admin',{status:'running'});
        const saved={plan_id:'a'.repeat(32),request_id:'b'.repeat(32),panel_domain:'panel.example.com'};
        store.set('celikpanel.setup.start.admin',JSON.stringify(saved));
        const running={...execution(saved),phase:'mail-renewal',steps:[{id:'mail-renewal',kind:'mail_enrollment',target:'mail-renewal',qualifier:'c'.repeat(64),status:'running'}],error:{code:'server_setup_mail_enrollment_not_recorded',message:'recorded'}};
        const read=fetch;let posted=0;
        globalThis.fetch=async(url,options)=>{
            if(url==='/api/v1/setup/mail-enrollment/retry'){
                posted++; assert.deepEqual(JSON.parse(options.body),{execution_id:running.id,step_id:'mail-renewal'});
                if(outcome==='lost')throw new Error('reply lost');
                return Response.json({handoff:'accepted'},{status:outcome==='refused'?409:202});
            }
            if(url.includes('/setup/operation'))return Response.json(running);
            return read(url,options);
        };
        try{
            await mount();assert.equal(posted,0);
            const button=findButton('setup.mailEnrollment.retry');assert.ok(button);
            assert.ok(!JSON.stringify(tree.toJSON()).includes('setup.blocker.unknown'),'recorded waiting is not a DNS failure');
            await act(async()=>{void button.props.onClick();void button.props.onClick();});
            assert.equal(posted,1,'double click/lost reply must not dispatch twice');
            assert.ok(JSON.stringify(tree.toJSON()).includes(outcome==='accepted'?'setup.mailEnrollment.checking':'setup.mailEnrollment.unknown'));
            assert.ok(!calls.some(call=>call.url==='/api/v1/setup/start'));
            await act(async()=>tree.unmount());tree=null;
            await mount();assert.equal(posted,1,'remount is observational');
        }finally{await cleanup();}
    }
});

test('reviewed handoff retry is limited to verified absence on the current running step',()=>{
 const pending={...execution({plan_id:'a'.repeat(32),request_id:'b'.repeat(32)}),phase:'mail-renewal',steps:[{id:'mail-renewal',kind:'mail_enrollment',target:'mail-renewal',status:'running'}],error:{code:'server_setup_mail_enrollment_not_recorded',message:'absent'}};
 assert.ok(operations.retryableMailEnrollmentHandoff(pending));
 for(const code of ['server_setup_mail_enrollment_running','server_setup_mail_enrollment_rollback','server_setup_mail_enrollment_unknown','server_setup_mail_enrollment_handoff','server_setup_reconciling'])assert.equal(operations.retryableMailEnrollmentHandoff({...pending,error:{code,message:'other'}}),null);
 for(const status of ['waiting','failed','succeeded'])assert.equal(operations.retryableMailEnrollmentHandoff({...pending,status}),null);
 assert.equal(operations.retryableMailEnrollmentHandoff({...pending,phase:'other'}),null);
 assert.equal(operations.retryableMailEnrollmentHandoff({...pending,steps:[{...pending.steps[0],status:'succeeded'}]}),null);
});


test('planned certificate handover: advance notice, then a calm state when the connection drops',async()=>{
    const host='panel.example.com';
    const marker={request_id:'d'.repeat(32),plan_id:'a'.repeat(32),panel_domain:host,handover:true};
    const steps=certificate=>[{id:'service-certbot',kind:'service',target:'certbot',status:'succeeded'},{id:'panel-certificate',kind:'panel_certificate',target:host,status:certificate},{id:'service-nginx',kind:'service',target:'nginx',status:'pending'},{id:'verify',kind:'verify',target:'web',status:'pending'}];
    const running=certificate=>({...execution(marker),panel_url:`https://${host}:2083/`,steps:steps(certificate)});
    const text=()=>JSON.stringify(tree.toJSON());
    const start=async(current,hostname='192.0.2.4')=>{
        init('admin',{status:'running',draft:{...fresh().draft,panel_domain:host}});
        store.set('celikpanel.setup.start.admin',JSON.stringify(marker));
        let reply=()=>Response.json(current),tick=()=>{};
        globalThis.window={...globalThis.window,setInterval:fn=>{tick=fn;return 1;},location:{hostname,port:'2083',reload(){}}};
        globalThis.fetch=async url=>url.includes('/setup/operation')?reply():Response.json(state);
        await mount();
        return{drop:async()=>{reply=()=>{throw new Error('connection refused')};await act(async()=>{tick();});await act(async()=>{});},back:async next=>{reply=()=>Response.json(next);await act(async()=>{tick();});await act(async()=>{});}};
    };
    // Before and during the step, on the IP address: the notice and a real link.
    for(const certificate of ['pending','running']){
        const page=await start(running(certificate));
        try{
            assert.ok(text().includes('setup.handover.title')&&text().includes('setup.handover.notice'),certificate);
            assert.ok(tree.root.findAllByType('a').some(link=>link.props.href===`https://${host}:2083/setup`&&link.props.children===`https://${host}:2083`),certificate);
            assert.equal(tree.root.findAllByProps({role:'alert'}).length,0);
            // The connection drops: the planned restart is named; nothing is called unknown.
            await page.drop();
            assert.ok(text().includes('setup.handover.dropTitle')&&text().includes('setup.handover.drop'),certificate);
            assert.ok(!text().includes('setup.uncertain')&&!text().includes('setup.reconnecting'),certificate);
            assert.ok(tree.root.findAllByType('a').some(link=>link.props.href===`https://${host}:2083/setup`));
            assert.ok(findButton('setup.reconnect'),'reading the same operation stays available');
            assert.ok(!calls.some(call=>call.options?.method==='POST'));
            // It reconnects by itself; after the step the existing secure-address link remains.
            await page.back(running('succeeded'));
            assert.ok(!text().includes('setup.handover.')&&text().includes('setup.secureAddress'),certificate);
        }finally{await cleanup();}
    }
    // The restart follows the finished step: still the planned one, without the advance notice.
    let page=await start(running('succeeded'));
    try{assert.ok(!text().includes('setup.handover.title'));await page.drop();assert.ok(text().includes('setup.handover.dropTitle'));}finally{await cleanup();}
    // Already on the host being secured: no notice and no link, but the restart is still named.
    page=await start(running('running'),host);
    try{
        assert.ok(!text().includes('setup.handover.title'));
        await page.drop();
        assert.ok(text().includes('setup.handover.drop')&&!text().includes('setup.handover.dropAddress'));
    }finally{await cleanup();}
    // A drop during another step stays an unknown result.
    page=await start({...running('pending'),steps:[{id:'service-certbot',kind:'service',target:'certbot',status:'running'},...steps('pending').slice(1)]});
    try{
        assert.ok(text().includes('setup.handover.title'));
        await page.drop();
        assert.ok(text().includes('setup.reconnecting')&&text().includes('setup.uncertain')&&!text().includes('setup.handover.'));
    }finally{await cleanup();}
    // Once a later step has finished the saved marker loses its flag.
    page=await start({...running('succeeded'),steps:steps('succeeded').map(item=>item.kind==='service'?{...item,status:'succeeded'}:item)});
    try{assert.equal(JSON.parse(store.get('celikpanel.setup.start.admin')).handover,undefined);}finally{await cleanup();}
});
test('a reviewed plan with the certificate step states the restart before setup starts',async()=>{
    init();const reviewed=plan({steps:[{id:'panel-certificate',kind:'panel_certificate',target:'panel.example.com'},{id:'verify',kind:'verify',target:'web'}]});
    const base=globalThis.fetch;globalThis.fetch=async(url,options)=>url==='/api/v1/setup/plan'?(calls.push({url,options}),Response.json({...reviewed,revision:state.revision})):base(url,options);
    try{
        await mount();await toReview();
        const content=JSON.stringify(tree.toJSON());
        assert.ok(content.includes('setup.handover.title')&&content.includes('setup.handover.address'),content.slice(0,400));
        assert.ok(tree.root.findAllByType('a').some(link=>link.props.href==='https://panel.example.com/setup'));
    }finally{await cleanup();}
});


// --- Extending a completed setup (2026-10-16, second server as a full peer) ---
// The ready screen and Settings offer the extension and say what it does and
// does not do; beginning it sends one request and reopens the wizard on the
// purpose step; the review says what changes; stopping restores the completed
// setup; an extension never routes the owner into setup.
const Notice = await loadComponent('ServerSetupGate', 'ServerSetupDashboardNotice');
const { ServerSetupSettings } = await import(choiceURL);
const NoticeInGate = () => React.createElement(Gate, null, React.createElement(Notice));
const readyDraft = () => ({ ...fresh().draft, panel_domain: 'panel.example.com', dns_mode: 'external' });
const writes = () => calls.filter(call => call.options?.method && call.options.method !== 'GET');
const textOf = () => JSON.stringify(tree.toJSON());

test('extension state decoder: an extension is never ready or required, and a base needs an extension', () => {
    const base = readyDraft();
    const extending = { ...fresh(), status: 'draft', required: false, extension: true, extension_base: base };
    assert.equal(setup.decodeServerSetup(extending).extension, true);
    assert.equal(setup.decodeServerSetup({ ...extending, status: 'ready' }), null);
    assert.equal(setup.decodeServerSetup({ ...extending, required: true }), null);
    assert.equal(setup.decodeServerSetup({ ...fresh(), extension_base: base }), null);
    assert.equal(setup.decodeServerSetup({ ...extending, extension_base: { purpose: 'nothing' } }), null);
    // Routing: an extension stays out of setup; the same draft as a first setup does not.
    assert.equal(setup.shouldOpenServerSetup(extending, '/domains'), false);
    assert.equal(setup.shouldOpenServerSetup({ ...extending, extension: undefined, extension_base: undefined, required: true }, '/domains'), true);
    assert.deepEqual(setup.setupExtensionChanges(base, { ...base, mail_hostname: 'mail.example.com', customization: { components: ['postfix'] } }).map(change => [change.key, change.before, change.after]),
        [['purpose', 'web', 'custom'], ['mail_hostname', '', 'mail.example.com']]);
    assert.deepEqual(setup.setupExtensionChanges(undefined, base), []);
});

test('a completed setup offers its extension, says what it does not do, and begins it with one request', async () => {
    init('admin', { status: 'ready', required: false, draft: readyDraft() });
    const base = state.draft; const read = fetch; const posted = [];
    globalThis.fetch = async (url, options) => {
        if (url === '/api/v1/setup/extend') {
            calls.push({ url, options }); posted.push(JSON.parse(options.body));
            state = { ...state, status: 'draft', extension: true, extension_base: base, revision: state.revision + 1 };
            return Response.json(state);
        }
        return read(url, options);
    };
    try {
        await mount();
        const section = tree.root.findByProps({ 'data-setup-extend': '' });
        assert.ok(JSON.stringify(section.findAllByType('p').map(item => item.props.children)).includes('setup.extend.help'));
        assert.equal(writes().length, 0, 'the ready screen changed something by itself');
        await act(async () => { await findButton('setup.extend.action').props.onClick(); });
        assert.deepEqual(posted, [{ revision: 1, action: 'begin' }]);
        // The gate draws the accepted state: the wizard reopens as an extension.
        await act(async () => { tree.update(React.createElement(Wizard, null)); await new Promise(resolve => setTimeout(resolve, 0)); });
        const shown = textOf();
        assert.ok(shown.includes('setup.extend.title') && shown.includes('setup.extend.intro'), shown.slice(0, 400));
        assert.ok(tree.root.findAllByProps({ id: 'setup-main_domain' }).length > 0, 'the extension opens on the first step');
        assert.ok(findButton('setup.extend.stop'));
        assert.ok(!shown.includes('setup.manualHelp'), 'the manual-setup choice is offered to an extension');
        assert.ok(writes().every(call => call.url === '/api/v1/setup/extend'));
    } finally { await cleanup(); }
    // Control: a first setup in progress has no extension section.
    init('admin', { status: 'draft' });
    try { await mount(); assert.equal(tree.root.findAllByProps({ 'data-setup-extend': '' }).length, 0); } finally { await cleanup(); }
});

test('a refused extension says why, leaves the setup as it was and is not sent again', async () => {
    init('admin', { status: 'ready', required: false, draft: readyDraft() });
    const read = fetch;
    globalThis.fetch = async (url, options) => {
        if (url === '/api/v1/setup/extend') { calls.push({ url, options }); return Response.json({ error: 'busy', code: 'server_setup_busy' }, { status: 409 }); }
        return read(url, options);
    };
    try {
        await mount();
        await act(async () => { await findButton('setup.extend.action').props.onClick(); });
        const alert = tree.root.findAllByProps({ role: 'alert' }).map(item => item.props.children);
        assert.ok(alert.includes('setup.extend.busy'), JSON.stringify(alert));
        assert.equal(writes().length, 1);
        assert.equal(state.status, 'ready');
        assert.ok(textOf().includes('setup.completeTitle'));
    } finally { await cleanup(); }
});

test('the review of an extension says what changes, what is installed and what is kept', async () => {
    const base = readyDraft();
    const draft = { ...base, mail_hostname: 'mail.example.com', customization: { components: ['dovecot', 'mariadb', 'nginx', 'php-fpm', 'postfix'] } };
    init('admin', { status: 'draft', required: false, extension: true, extension_base: base, draft });
    globalThis.sessionStorage.setItem('celikpanel.setup.editor.admin', JSON.stringify({ version: 1, revision: state.revision, step: 'review', draft }));
    const read = fetch;
    globalThis.fetch = async (url, options) => url === '/api/v1/setup/plan'
        ? Response.json(plan({ steps: [{ id: '01-mail_profile', kind: 'mail_profile', target: 'core-mail' }, { id: '02-verify', kind: 'verify', target: 'custom' }],
            components: [{ id: 'nginx', selected: true, required: false, installed: true }, { id: 'postfix', selected: true, required: false, installed: false }] }))
        : read(url, options);
    try {
        await mount();
        const review = tree.root.findByProps({ 'data-setup-extension-review': '' });
        const said = JSON.stringify(review.findAll(node => typeof node.props?.children === 'string').map(node => node.props.children));
        const mailRow = review.findByProps({ 'data-change': 'mail_hostname' });
        assert.ok(JSON.stringify(mailRow.findAllByType('span').map(item => item.props.children)).includes('mail.example.com'));
        assert.ok(JSON.stringify(mailRow.findAllByType('span').map(item => item.props.children)).includes('setup.extend.none'));
        // A literal keeps the mono face; "none" is a word (2026-10-16).
        const face = value => String(mailRow.findAllByType('span').find(item => item.props.children === value).props.className || '').includes('font-mono');
        assert.deepEqual([face('setup.extend.none'), face('mail.example.com')], [false, true]);
        assert.ok(review.findAllByProps({ 'data-change': 'purpose' }).length === 1);
        assert.equal(review.findAllByProps({ 'data-change': 'panel_domain' }).length, 0, 'an unchanged setting is listed as a change');
        assert.equal(review.findAllByProps({ 'data-install': 'core-mail' }).length, 1, said);
        assert.ok(said.includes('setup.extend.keptTitle'), said);
        assert.ok(!textOf().includes('setup.components.reviewTitle'), 'the whole component list replaces what changes');
    } finally { await cleanup(); }
    // Control: the same review as a first setup keeps the whole component list.
    init('admin', { status: 'draft', draft });
    globalThis.sessionStorage.setItem('celikpanel.setup.editor.admin', JSON.stringify({ version: 1, revision: state.revision, step: 'review', draft }));
    const again = fetch;
    globalThis.fetch = async (url, options) => url === '/api/v1/setup/plan' ? Response.json(plan({ components: [{ id: 'nginx', selected: true, required: false, installed: true }] })) : again(url, options);
    try {
        await mount();
        assert.equal(tree.root.findAllByProps({ 'data-setup-extension-review': '' }).length, 0);
        assert.ok(textOf().includes('setup.components.reviewTitle'));
    } finally { await cleanup(); }
});

test('stopping an extension restores the completed setup with one request', async () => {
    const base = readyDraft();
    init('admin', { status: 'draft', required: false, extension: true, extension_base: base, draft: { ...base, mail_hostname: 'mail.example.com' } });
    const read = fetch; const posted = [];
    globalThis.fetch = async (url, options) => {
        if (url === '/api/v1/setup/extend') {
            calls.push({ url, options }); posted.push(JSON.parse(options.body));
            state = { ...fresh(), status: 'ready', required: false, draft: base, revision: state.revision + 1 };
            return Response.json(state);
        }
        return read(url, options);
    };
    try {
        await mount();
        assert.ok(textOf().includes('setup.extend.stopHelp'));
        await act(async () => { await findButton('setup.extend.stop').props.onClick(); });
        assert.deepEqual(posted, [{ revision: 1, action: 'cancel' }]);
        await act(async () => { tree.update(React.createElement(Wizard, null)); await new Promise(resolve => setTimeout(resolve, 0)); });
        assert.ok(textOf().includes('setup.completeTitle'));
    } finally { await cleanup(); }
});

test('the Dashboard and Settings say an extension is not a setup to resume, and the gate keeps management open', async () => {
    const base = readyDraft();
    for (const status of ['draft', 'running']) {
        init('admin', { status, required: false, extension: true, extension_base: base, draft: base });
        // The notice reads the gate's own context: it is mounted inside the
        // gate, on a page the gate always draws.
        fixture.path = '/settings';
        try {
            await mount(NoticeInGate);
            const shown = textOf();
            assert.ok(shown.includes('setup.extend.dashboardTitle') && shown.includes(status === 'running' ? 'setup.extend.dashboardRunning' : 'setup.extend.dashboardDraft'), shown);
            // A running extension needs nothing from the owner: its link shows the progress (2026-10-16).
            assert.ok(shown.includes(status === 'running' ? 'setup.extend.showProgress' : 'setup.extend.continue') && !shown.includes('setup.resume'), shown);
            assert.ok(!shown.includes(status === 'running' ? 'setup.extend.continue' : 'setup.extend.showProgress'), shown);
        } finally { await cleanup(); }
        init('admin', { status, required: false, extension: true, extension_base: base, draft: base });
        try {
            await mount(ServerSetupSettings);
            const shown = textOf();
            assert.ok(shown.includes(status === 'running' ? 'setup.extend.showProgress' : 'setup.extend.continue'), shown);
        } finally { await cleanup(); }
        init('admin', { status, required: false, extension: true, extension_base: base, draft: base });
        fixture.path = '/domains';
        try {
            await mount(Gate);
            assert.equal(tree.root.findAllByType('redirect').length, 0, 'an extension routed the owner into setup');
            assert.equal(tree.root.findAllByProps({ 'data-management': true }).length, 1);
        } finally { await cleanup(); }
    }
    init('admin', { status: 'ready', required: false, draft: base });
    try {
        await mount(ServerSetupSettings);
        const shown = textOf();
        assert.ok(shown.includes('setup.extend.help') && shown.includes('setup.extend.title'), shown);
    } finally { await cleanup(); }
    // Control: a first setup in progress is still resumed from the Dashboard.
    init('admin', { status: 'draft' });
    fixture.path = '/settings';
    try { await mount(NoticeInGate); assert.ok(textOf().includes('setup.resume')); } finally { await cleanup(); }
});

// set18 D1 (2026-10-16): while a completed setup is being extended, the latest
// run is the completed setup's own succeeded run. The operation read marks it
// superseded; the page shows the extension's wizard and "Stop extending", not
// "Verifying the result" and "Completion could not be verified".
const completedRun = (extra = {}) => ({ id: 'd'.repeat(32), request_id: 'd'.repeat(32), plan_id: 'e'.repeat(32), status: 'succeeded', phase: 'complete',
    steps: [{ id: '01-verify', kind: 'verify', target: 'web', status: 'succeeded' }], ...extra });
function extensionFixture({ flag = true, step } = {}) {
    const base = readyDraft();
    init('admin', { status: 'draft', required: false, extension: true, extension_base: base, completed_at: '2026-10-15T22:57:21Z', draft: { ...base, mail_hostname: 'mail.example.com' } });
    if (step) globalThis.sessionStorage.setItem('celikpanel.setup.editor.admin', JSON.stringify({ version: 1, revision: state.revision, step, draft: state.draft }));
    const read = fetch; const run = { started: null, posted: [] };
    globalThis.fetch = async (url, options) => {
        if (url.includes('/setup/operation')) {
            calls.push({ url, options });
            if (run.started && url.endsWith('request_id=' + run.started.request_id)) return Response.json(execution(run.started));
            // The server flags the completed run only while the extension is a draft.
            return Response.json(completedRun(flag && state.extension && state.status === 'draft' ? { superseded: true } : {}));
        }
        if (url === '/api/v1/setup/start') {
            calls.push({ url, options }); run.started = JSON.parse(options.body);
            state = { ...state, status: 'running' };
            return Response.json(execution(run.started), { status: 202 });
        }
        if (url === '/api/v1/setup/extend') {
            calls.push({ url, options }); run.posted.push(JSON.parse(options.body));
            state = { ...fresh(), status: 'ready', required: false, completed_at: '2026-10-15T22:57:21Z', draft: base, revision: state.revision + 1 };
            return Response.json(state);
        }
        return read(url, options);
    };
    return run;
}
const settle = () => act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });

test('an extension of a completed setup opens its wizard, not the completed run (set18 D1)', async () => {
    extensionFixture();
    try {
        await mount(); await settle();
        const shown = textOf();
        assert.ok(calls.some(call => call.url.includes('/setup/operation')), 'the operation was not read');
        assert.ok(!shown.includes('setup.verificationFailed') && !shown.includes('setup.verifying') && !shown.includes('setup.reconnect'), shown.slice(0, 600));
        assert.ok(tree.root.findAllByProps({ id: 'setup-main_domain' }).length > 0, 'the extension does not open on the first step');
        assert.ok(findButton('setup.extend.stop'), 'Stop extending is not reachable');
        assert.equal(tree.root.findAllByProps({ 'data-setup-extend-stop': '' }).length, 1);
        assert.equal(writes().length, 0);
    } finally { await cleanup(); }
    // The Components route lands on the names step with the mail host name.
    extensionFixture({ step: 'names' });
    try {
        await mount(); await settle();
        assert.equal(tree.root.findByProps({ id: 'setup-panel_domain' }).props.value, 'panel.example.com');
        assert.ok(findButton('setup.extend.stop'));
        assert.ok(!textOf().includes('setup.verificationFailed'));
    } finally { await cleanup(); }
    // A start marker left from the completed run is dropped, not reconciled.
    extensionFixture();
    store.set('celikpanel.setup.start.admin', JSON.stringify({ request_id: 'd'.repeat(32), plan_id: 'e'.repeat(32), panel_domain: 'panel.example.com' }));
    try {
        await mount(); await settle();
        assert.ok(calls.some(call => call.url.endsWith('request_id=' + 'd'.repeat(32))));
        assert.equal(store.has('celikpanel.setup.start.admin'), false, 'the finished run marker was kept');
        assert.ok(findButton('setup.extend.stop') && !textOf().includes('setup.verificationFailed'));
    } finally { await cleanup(); }
    // Negative control: the same state without the server's flag is the D1 screen.
    extensionFixture({ flag: false });
    try {
        await mount(); await settle();
        assert.ok(textOf().includes('setup.verificationFailed'), 'the control no longer reproduces D1');
        assert.equal(findButton('setup.extend.stop'), undefined);
    } finally { await cleanup(); }
});

test('a started extension shows its own run and first step; stopping returns to the ready screen (set18 D1, O1)', async () => {
    const run = extensionFixture();
    try {
        await mount(); await settle();
        await toReview();
        await act(async () => tree.root.findByProps({ type: 'checkbox' }).props.onChange({ target: { checked: true } }));
        await act(async () => { await findButton('setup.start').props.onClick(); });
        await settle();
        assert.equal(calls.filter(call => call.url === '/api/v1/setup/start').length, 1);
        const shown = textOf();
        assert.ok(shown.includes('setup.installing') && shown.includes('setup.kind.verify'), shown.slice(0, 600));
        // O1: the accepted run names its first step, not the generic sentence.
        assert.ok(shown.includes('setup.guide.queued') && !shown.includes('setup.guide.unknown'), shown.slice(0, 800));
        assert.ok(!shown.includes('setup.verificationFailed'));
        assert.equal(findButton('setup.extend.stop'), undefined, 'Stop extending offered while the extension runs');
        assert.ok(run.started && JSON.parse(store.get('celikpanel.setup.start.admin')).request_id === run.started.request_id);
    } finally { await cleanup(); }
    const stopped = extensionFixture();
    try {
        await mount(); await settle();
        await act(async () => { await findButton('setup.extend.stop').props.onClick(); });
        assert.deepEqual(stopped.posted, [{ revision: 1, action: 'cancel' }]);
        await act(async () => { tree.update(React.createElement(Wizard, null)); await new Promise(resolve => setTimeout(resolve, 0)); });
        await settle();
        const shown = textOf();
        assert.ok(shown.includes('setup.completeTitle') && !shown.includes('setup.verificationFailed'), shown.slice(0, 600));
    } finally { await cleanup(); }
});

test('operation decoder keeps the superseded flag only on a succeeded run', () => {
    assert.equal(operations.decodeSetupExecution(completedRun({ superseded: true })).superseded, true);
    const running = operations.decodeSetupExecution({ ...completedRun({ superseded: true }), status: 'running', steps: [{ id: '01-verify', kind: 'verify', target: 'web', status: 'running' }] });
    assert.ok(running && running.superseded === undefined, 'a flag hid a run in progress');
    const odd = operations.decodeSetupExecution(completedRun({ superseded: 'yes' }));
    assert.ok(odd && odd.superseded === undefined);
});

// set18 O3: "Continue recorded operation" stays disabled and says it is
// checking from the first press until a read shows the operation moved on.
test('a pressed mail enrollment continuation stays single-flight until the operation moves (set18 O3)', async () => {
    init('admin', { status: 'running' });
    const saved = { plan_id: 'a'.repeat(32), request_id: 'b'.repeat(32), panel_domain: 'panel.example.com' };
    store.set('celikpanel.setup.start.admin', JSON.stringify(saved));
    let current = { ...execution(saved), phase: 'mail-renewal', steps: [{ id: 'mail-renewal', kind: 'mail_enrollment', target: 'mail-renewal', qualifier: 'c'.repeat(64), status: 'running' }], error: { code: 'server_setup_mail_enrollment_running', message: 'recorded' } };
    const ticks = []; const settleTimers = [];
    globalThis.window.setInterval = callback => { ticks.push(callback); return ticks.length; };
    globalThis.window.clearInterval = () => {};
    globalThis.window.setTimeout = (callback, ms) => { if (ms === 15000) { settleTimers.push(callback); return 0; } return setTimeout(callback, ms); };
    globalThis.window.clearTimeout = id => { if (id) clearTimeout(id); };
    const read = fetch; let posted = 0;
    globalThis.fetch = async (url, options) => {
        if (url === '/api/v1/setup/mail-enrollment/continue') { posted++; return Response.json({ handoff: 'unknown' }); }
        if (url.includes('/setup/operation')) return Response.json(current);
        return read(url, options);
    };
    const tick = () => act(async () => { await ticks[ticks.length - 1](); await new Promise(resolve => setTimeout(resolve, 0)); });
    try {
        await mount();
        const button = findButton('setup.mailEnrollment.continue');
        await act(async () => { void button.props.onClick(); void button.props.onClick(); });
        let sent = findButton('setup.mailEnrollment.sent');
        assert.ok(sent && sent.props.disabled === true && sent.props.loading === true, 'the pressed button is offered again');
        assert.equal(findButton('setup.mailEnrollment.continue'), undefined);
        await act(async () => { await sent.props.onClick(); });
        // The same state read again does not confirm anything.
        await tick();
        sent = findButton('setup.mailEnrollment.sent');
        assert.ok(sent && sent.props.disabled === true);
        await act(async () => { await sent.props.onClick(); });
        assert.equal(posted, 1, 'the continuation was sent again while in flight');
        // A read showing the operation moved settles the press.
        current = { ...current, error: { code: 'server_setup_mail_enrollment_rollback', message: 'recorded' } };
        await tick();
        const again = findButton('setup.mailEnrollment.continue');
        assert.ok(again && !again.props.disabled, 'a moved operation keeps the button disabled');
        // Unconfirmed: after the bound the button is offered again.
        await act(async () => { await again.props.onClick(); });
        assert.equal(posted, 2);
        assert.ok(findButton('setup.mailEnrollment.sent'));
        await act(async () => { settleTimers[settleTimers.length - 1](); });
        assert.ok(findButton('setup.mailEnrollment.continue') && !findButton('setup.mailEnrollment.continue').props.disabled);
    } finally { await cleanup(); }
});

// Second server as a full peer, part 2 (2026-10-16).
test('the draft decoder accepts the peer’s mail host name and refuses one that is not the contract', () => {
    const draft = (infrastructure) => ({...fresh(), draft:{...fresh().draft, infrastructure_dns:infrastructure}});
    assert.ok(setup.decodeServerSetup(draft({zone:'example.com', peer_mail_hostname:'mail.boston.example.com'})));
    assert.equal(setup.decodeServerSetup(draft({zone:'example.com', peer_mail_hostname:7})), null);
    assert.equal(setup.decodeServerSetup(draft({zone:'example.com', peer_mail_hostname:'a'.repeat(254)})), null);
});

test('the execution context keeps a valid record request outcome and drops only an invalid one', () => {
    const run = (peer) => ({id:'c'.repeat(32), plan_id:'a'.repeat(32), request_id:'b'.repeat(32), status:'waiting', phase:'access_dns',
        steps:[{id:'05-access_dns', kind:'access_dns', target:'mail.boston.example.com', qualifier:'192.0.2.10', status:'running'}],
        context:{dns_mode:'local', dns_role:'secondary', dns_engine:'bind', local_nameserver:'ns2.example.com', local_ip:'192.0.2.10', peer_nameserver:'ns1.example.com', peer_ip:'192.0.2.20', panel_domain:'boston.example.com', mail_hostname:'mail.boston.example.com', dns_hosting_management:'panel', peer_host_record:peer}});
    const good = operations.decodeSetupExecution(run({channel:true, outcome:{status:'applied', result:'created', zone:'example.com', name:'mail.boston.example.com', type:'A', content:'192.0.2.10', at:'2026-10-16T00:00:00Z', attempts:1, request_id:'b'.repeat(32)}}));
    assert.equal(good.context.peer_host_record.outcome.status, 'applied');
    assert.equal(good.context.peer_host_record.outcome.zone, 'example.com');
    const bad = operations.decodeSetupExecution(run({channel:'yes'}));
    assert.ok(bad.context, 'an invalid additive field dropped the whole context');
    assert.equal(bad.context.peer_host_record, undefined);
    assert.equal(bad.context.peer_nameserver, 'ns1.example.com');
    const strange = operations.decodeSetupExecution(run({channel:true, outcome:{status:'exploded', name:'x', content:'y'}}));
    assert.deepEqual(strange.context.peer_host_record, {channel:true});
});

// Setup wizard v2 (alpha.111): main domain first, server count, short name,
// roles, names proposed from the main domain, DNS home, then the review.
const putDraft = () => state.draft;
const radio = (name, value) => tree.root.findByProps({ name, value });
async function pick(name, value) { await act(async () => radio(name, value).props.onChange()); }
const putCount = () => calls.filter(call => call.options?.method === 'PUT').length;

test('wizard v2: the main domain comes first and every name is proposed from it, an owner edit is kept', async () => {
    init('admin', { short_name_suggestion: 'frankfurt' });
    try {
        await mount();
        await submit();
        assert.equal(putCount(), 0, 'an empty main domain is refused before any request');
        assert.ok(textOf().includes('setup.v2.domain.invalid'));
        await type('setup-main_domain', 'https://www.Example.com/');
        await submit();
        assert.equal(putDraft().main_domain, 'example.com');
        assert.equal(radio('setup-server_count', '2').props.checked, true, 'two servers are recommended');
        assert.ok(textOf().includes('setup.v5.dns.second.help'), 'the second server joins by code; its address is never typed here');
        await submit();
        assert.equal(input('setup-short_name').props.value, 'frankfurt');
        await submit();
        assert.equal(putDraft().short_name, 'frankfurt');
        await submit();
        assert.equal(input('setup-panel_domain').props.value, 'frankfurt.example.com');
        await type('setup-panel_domain', 'admin.example.com');
        await act(async () => findButton('setup.back').props.onClick());
        await act(async () => findButton('setup.back').props.onClick());
        await type('setup-short_name', 'boston');
        await submit(); await submit();
        assert.equal(input('setup-panel_domain').props.value, 'admin.example.com', 'a name the owner typed is never replaced');
        await submit();
        assert.equal(putDraft().ns1, 'ns1.example.com');
        assert.equal(putDraft().ns2, 'ns2.example.com');
        assert.equal(putDraft().peer_ns, 'ns2.example.com');
        assert.equal(putDraft().main_dns_server, 'this');
        assert.equal(calls.some(call => call.url === '/api/v1/setup/start'), false);
    } finally { await cleanup(); }
});

test('wizard v2: roles combine, preselect their components, and customising opens the grid', async () => {
    init();
    try {
        await mount();
        await type('setup-main_domain', 'example.com'); await submit(); await submit(); await submit();
        assert.equal(radio('setup-role', 'web').props.checked, true);
        assert.equal(radio('setup-role', 'dns').props.checked, true);
        await pick('setup-role', 'mail');
        assert.ok(textOf().includes('Postfix'), 'the mail role says what it installs');
        await act(async () => findButton('setup.components.customize').props.onClick());
        assert.ok(component('redis'), 'the grid lists every choice');
        await toggleComponent('redis');
        await submit();
        assert.equal(putDraft().purpose, 'custom');
        assert.ok(putDraft().customization.components.includes('redis'));
        assert.ok(putDraft().customization.components.includes('postfix'));
        await act(async () => findButton('setup.back').props.onClick());
        await act(async () => findButton('setup.back').props.onClick());
        assert.ok(textOf().includes('setup.v2.roles.customized'));
        await act(async () => tree.root.findAllByType('button').find(button => button.props.children === 'setup.v2.roles.reset').props.onClick());
        await submit();
        assert.equal(putDraft().customization.components.includes('redis'), false, 'the recommended set drops the explicit extra');
    } finally { await cleanup(); }
});

test('wizard v2: one server keeps its own DNS standalone, both names on this server', async () => {
    init();
    try {
        await mount();
        await type('setup-main_domain', 'example.com'); await submit();
        await pick('setup-server_count', '1'); await submit(); await submit(); await submit(); await submit();
        assert.equal(tree.root.findAllByProps({ 'data-setup-dns-standalone': '' }).length, 1, 'one server keeps the DNS itself');
        assert.ok(textOf().includes('setup.v5.dns.standalone.help'));
        assert.equal(textOf().includes('setup.v2.dns.localOneServer'), false, 'no refusal for one server');
        assert.equal(tree.root.findAllByProps({ id: 'setup-peer_ip' }).length, 0);
        assert.ok(input('setup-ns1') && input('setup-ns2'), 'both nameserver names are this server\'s');
        await type('setup-local_ip', '203.0.113.10');
        await submit();
        assert.equal(putDraft().dns_mode, 'local');
        assert.equal(putDraft().server_count, 1);
        assert.equal(putDraft().peer_ip, '');
        assert.equal(putDraft().peer_ns, '');
        assert.equal(putDraft().main_dns_server, 'this');
    } finally { await cleanup(); }
});

test('wizard v2: the DNS software offer follows the catalogue, and the backup role keeps ns1 on the main server', async () => {
    init();const base = globalThis.fetch;
    globalThis.fetch = async (url, options) => url === '/api/v1/setup/components'
        ? Response.json({ ...setupCatalogFixture(), dns_engines: [
            { id: 'bind', role: 'main', offered: true, candidate_version: '9.20' }, { id: 'pdns', role: 'main', offered: false, reason: 'server_setup_pdns_main_untested_system' },
            { id: 'bind', role: 'backup', offered: true }, { id: 'pdns', role: 'backup', offered: true }] })
        : base(url, options);
    try {
        await mount();
        await toDNSHome();
        await type('setup-local_ip', '203.0.113.10');
        assert.equal(tree.root.findAllByProps({ id: 'setup-peer_ip' }).length, 0, 'the second server\'s address is never typed');
        assert.ok(textOf().includes('setup.v5.dns.second.title'), 'the second server is not connected yet');
        assert.ok(textOf().includes('setup.v2.engine.untested'), 'the reason is shown where PowerDNS is offered');
        await submit();
        assert.equal(calls.some(call => call.url === '/api/v1/setup/plan'), false);
        assert.ok(textOf().includes('setup.v2.engine.notOffered'));
        await pick('setup-dns_engine', 'bind');
        await pick('setup-main_dns_server', 'peer');
        assert.ok(input('setup-ns2'), 'this backup server carries the second nameserver');
        await submit();
        assert.equal(putDraft().dns_role, 'secondary');
        assert.equal(putDraft().ns1, 'ns1.example.com');
        assert.equal(putDraft().peer_ns, 'ns1.example.com');
        assert.equal(putDraft().peer_ip, '');
        assert.equal(putDraft().main_dns_server, 'peer', 'the second server is the main DNS server (Option A)');
        assert.equal(putDraft().infrastructure_dns, undefined, 'a backup DNS server keeps no zone of its own');
        assert.equal(tree.root.findAllByProps({ id: 'setup-records-title' }).length, 1, 'this server still prepares the list it hands over with the join');
    } finally { await cleanup(); }
});

function waitingFixture(draft, phase, extra = () => null) {
    init('admin', { status: 'waiting', draft: { ...fresh().draft, ...draft }, server_ip: '203.0.113.10' });
    const marker = { plan_id: 'a'.repeat(32), request_id: 'b'.repeat(32), panel_domain: 'panel.example.com' };
    store.set('celikpanel.setup.start.admin', JSON.stringify(marker));
    const running = { ...execution(marker, 'waiting'), phase, steps: [{ id: '01-access_dns', kind: 'access_dns', target: 'panel.example.com', status: 'running' }, { id: 'verify', kind: 'verify', target: 'web', status: 'pending' }] };
    const base = globalThis.fetch;
    globalThis.fetch = async (url, options) => url.includes('/setup/operation') ? Response.json(running) : (extra(url, options) ?? base(url, options));
}

test('wizard v2: with an external provider the progress screen lists the records with their internet state, reading only', async () => {
    waitingFixture({ dns_mode: 'external', panel_domain: 'panel.example.com', main_domain: 'example.com' }, 'access_dns', url => url === '/api/v1/setup/provider-records'
        ? (calls.push({ url }), Response.json({ mode: 'external', servers: [{ label: '', ip4: '203.0.113.10', rows: [{ name: 'panel.example.com', type: 'A', value: '203.0.113.10', state: 'different', seen: ['192.0.2.80'] }] }], checked_at: new Date().toISOString(), next_check_after: 60 }))
        : null);
    try {
        await mount(); await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });
        const text = textOf();
        assert.ok(calls.some(call => call.url === '/api/v1/setup/provider-records'));
        assert.ok(text.includes('setup.v2.provider.different') && text.includes('panel.example.com'), text.slice(0, 400));
        assert.ok(text.includes('setup.v2.access.title') && text.includes('https://203.0.113.10:'), 'the temporary address is said plainly');
        assert.equal(tree.root.findAllByProps({ 'data-setup-registrar': 'now' }).length, 0);
        assert.equal(writes().length, 0, 'polling never mutates');
    } finally { await cleanup(); }
});

test('wizard v2: the registrar card names the nameservers and the IP records inside the main domain', async () => {
    waitingFixture({ dns_mode: 'local', dns_role: 'primary', main_domain: 'example.com', panel_domain: 'panel.example.com', ns1: 'ns1.example.com', ns2: 'ns2.example.com', peer_ns: 'ns2.example.com', local_ip: '203.0.113.10', peer_ip: '198.51.100.20' }, 'access_dns');
    try {
        await mount();
        const text = textOf();
        assert.equal(tree.root.findAllByProps({ 'data-setup-registrar': 'now' }).length, 1, 'the panel waits for its name: the registrar step is now');
        for (const value of ['setup.v2.registrar.glueTitle', 'ns1.example.com', 'ns2.example.com', '198.51.100.20', 'setup.v2.registrar.nowWhy']) assert.ok(text.includes(value), value);
        assert.equal(writes().length, 0);
    } finally { await cleanup(); }
});

// Setup wizard v2 phase 2 (alpha.112): existing records and mail records.
const publicRead = (records) => ({ domain: 'example.com', extras: [], records, checked_at: new Date().toISOString(), next_check_after: 60 });
function withPublicRecords(records) {
    const base = globalThis.fetch;
    globalThis.fetch = async (url, options) => url.startsWith('/api/v1/setup/public-records') ? (calls.push({ url }), Response.json(publicRead(records))) : base(url, options);
}
async function settleReads() { await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }
async function toRecords(extra = async () => {}) {
    await type('setup-main_domain', 'example.com'); await submit(); await submit(); await submit();
    await extra();
    await submit(); await submit();
    await type('setup-local_ip', '203.0.113.10');
    await submit(); await settleReads();
}

test('wizard v2: existing records are read only, each is imported or skipped, and a server name is never taken over', async () => {
    init(); withPublicRecords([
        { name: 'example.com', type: 'A', values: ['192.0.2.80'], state: 'found' },
        { name: 'example.com', type: 'TXT', values: ['google-site-verification=abc'], state: 'found' },
        { name: 'ns1.example.com', type: 'A', values: ['192.0.2.53'], state: 'found' },
        { name: 'www.example.com', type: 'CNAME', values: [], state: 'unknown' },
    ]);
    try {
        await mount(); await toRecords();
        assert.ok(tree.root.findAllByProps({ 'data-setup-records': '' }).length === 1, 'the records step follows the DNS step for the main DNS server');
        assert.ok(textOf().includes('setup.v3.records.clash'), 'a name the setup gives to a server is not imported');
        assert.ok(textOf().includes('setup.v3.records.unread'), 'an unread name is said as unread, not as empty');
        await act(async () => tree.root.findByProps({ name: 'setup-import-1', value: 'skip' }).props.onChange());
        await submit();
        const decisions = Object.fromEntries(state.draft.imported_records.map(item => [`${item.name} ${item.type}`, item.decision]));
        assert.deepEqual(decisions, { 'example.com A': 'import', 'example.com TXT': 'skip', 'ns1.example.com A': 'skip' });
        assert.ok(calls.filter(call => call.url.startsWith('/api/v1/setup/public-records')).length >= 1);
        assert.equal(calls.some(call => call.url === '/api/v1/setup/start'), false);
    } finally { await cleanup(); }
});

test('wizard v2: mail keeps the old MX until the owner moves it, SPF is merged, and the PTR rows are shown', async () => {
    init(); withPublicRecords([
        { name: 'example.com', type: 'MX', values: ['10 mx.old.example'], state: 'found' },
        { name: 'example.com', type: 'TXT', values: ['v=spf1 include:_spf.old.example ~all'], state: 'found' },
    ]);
    try {
        await mount(); await toRecords(async () => { await pick('setup-role', 'mail'); });
        assert.ok(state.draft.mail_hostname, 'a mail host name was proposed');
        await submit();
        assert.ok(tree.root.findAllByProps({ 'data-setup-mail': '' }).length === 1);
        assert.equal(radio('setup-mail_mx', 'later').props.checked, true, 'the old MX is kept by default');
        assert.ok(textOf().includes(`v=spf1 include:_spf.old.example a:${state.draft.mail_hostname} ~all`), 'SPF is merged, not replaced');
        assert.ok(tree.root.findAllByProps({ 'data-setup-ptr': '' }).length === 1 && textOf().includes('203.0.113.10'));
        await pick('setup-mail_mx', 'now');
        await submit();
        assert.equal(state.draft.mail_dns.mx, 'now');
        assert.ok(findButton('setup.start'), 'the review follows the mail step');
    } finally { await cleanup(); }
});

test('wizard v2: reverse DNS is an open task with who acts, never what the final check waits for', async () => {
    const ptr = { id: 'mail_identity', state: 'action_required', code: 'mail_identity_required', reason: 'reverse_dns_mismatch', advisory: true, vars: { hostname: 'mail.example.com', ip: '203.0.113.10', ptr: 'static.provider.example' } };
    waitingFixture({ dns_mode: 'external', panel_domain: 'panel.example.com' }, 'access_dns');
    const base = globalThis.fetch;
    globalThis.fetch = async (url, options) => url.includes('/setup/operation')
        ? Response.json({ ...execution({ plan_id: 'a'.repeat(32), request_id: 'b'.repeat(32) }, 'waiting'), phase: 'verification', checks: [{ id: 'services', state: 'action_required', code: 'services_required' }, ptr], steps: [{ id: 'verify', kind: 'verify', target: 'web', status: 'pending' }] })
        : base(url, options);
    try {
        await mount();
        const box = tree.root.findAllByProps({ 'data-setup-open-tasks': '' });
        assert.equal(box.length, 1, 'the PTR is listed as an open task');
        assert.ok(textOf().includes('setup.v3.tasks.who'));
        const waitedFor = tree.root.findAllByProps({ 'data-check': 'mail_identity' }).filter(node => node.props['data-state'] !== undefined);
        assert.equal(waitedFor.length, 0, 'the open task is not among the checks setup waits for');
        assert.ok(tree.root.findAllByProps({ 'data-check': 'services' }).length > 0, 'the blocking check is still listed');
    } finally { await cleanup(); }
});

test('wizard v2: the registrar card reads the live check, waits for the backup and refuses "now" until the main answers', async () => {
    const local = { dns_mode: 'local', dns_role: 'primary', main_domain: 'example.com', panel_domain: 'panel.example.com', ns1: 'ns1.example.com', ns2: 'ns2.example.com', peer_ns: 'ns2.example.com', local_ip: '203.0.113.10', peer_ip: '198.51.100.20' };
    const delegation = (mainAnswers) => ({ zone: 'example.com', checked_at: new Date().toISOString(), now_allowed: mainAnswers,
        expected_ns: [{ name: 'ns1.example.com', server: 'main', ipv4: '203.0.113.10', ipv6: '2001:db8::10', glue_needed: true }, { name: 'ns2.example.com', server: 'backup', ipv4: '198.51.100.20', glue_needed: true }],
        items: [{ id: 'answers_main', state: mainAnswers ? 'ok' : 'unknown', vars: { ip: '203.0.113.10' } }, { id: 'answers_backup', state: 'waiting', vars: { ip: '198.51.100.20' } },
            { id: 'glue_v4:ns1.example.com', state: 'not_yet', vars: { name: 'ns1.example.com', ip: '203.0.113.10' } }, { id: 'nameservers', state: 'not_yet', vars: { expected: 'ns1.example.com, ns2.example.com', seen: 'ns1.old.example' } }] });
    for (const mainAnswers of [false, true]) {
        waitingFixture(local, 'infrastructure_dns', url => url.startsWith('/api/v1/setup/delegation') ? (calls.push({ url }), Response.json(delegation(mainAnswers))) : null);
        try {
            await mount(); await settleReads();
            assert.equal(tree.root.findAllByProps({ 'data-setup-registrar': 'later' }).length, 1, 'with two servers the card waits for the backup DNS server');
            assert.ok(calls.some(call => call.url === '/api/v1/setup/delegation?zone=example.com'));
            assert.equal(tree.root.findAllByProps({ 'data-delegation-item': 'nameservers' })[0].props['data-state'], 'not_yet');
            assert.ok(textOf().includes('2001:db8::10'), 'the IPv6 IP record is offered, as optional');
            await act(async () => findButton('setup.v4.early').props.onClick());
            assert.equal(tree.root.findAllByProps({ 'data-setup-registrar-refused': '' }).length, mainAnswers ? 0 : 1, 'refused with the reason until the main answers');
            assert.equal(tree.root.findAllByProps({ 'data-setup-registrar': 'now' }).length, mainAnswers ? 1 : 0);
            assert.equal(writes().length, 0, 'the card only reads');
        } finally { await cleanup(); }
    }
});


test('wizard v2: the registrar card stops asking once everything answers correctly', async () => {
    const local = { dns_mode: 'local', dns_role: 'primary', main_domain: 'example.com', panel_domain: 'panel.example.com', ns1: 'ns1.example.com', ns2: 'ns2.example.com', peer_ns: 'ns2.example.com', local_ip: '203.0.113.10', peer_ip: '198.51.100.20' };
    const done = { zone: 'example.com', checked_at: new Date().toISOString(), now_allowed: true,
        expected_ns: [{ name: 'ns1.example.com', server: 'main', ipv4: '203.0.113.10', glue_needed: true }, { name: 'ns2.example.com', server: 'backup', ipv4: '198.51.100.20', glue_needed: true }],
        items: [{ id: 'answers_main', state: 'ok', vars: { ip: '203.0.113.10' } }, { id: 'answers_backup', state: 'ok', vars: { ip: '198.51.100.20' } }, { id: 'nameservers', state: 'ok', vars: { expected: 'ns1.example.com, ns2.example.com' } }, { id: 'imported', state: 'ok', vars: { matched: '3', total: '3' } }] };
    const realSet = globalThis.setInterval, realClear = globalThis.clearInterval;
    const active = new Map();
    globalThis.setInterval = (callback, ms) => { const id = realSet(callback, ms); active.set(id, ms); return id; };
    globalThis.clearInterval = id => { active.delete(id); realClear(id); };
    waitingFixture(local, 'infrastructure_dns', url => url.startsWith('/api/v1/setup/delegation') ? Response.json(done) : null);
    try {
        await mount(); await settleReads();
        assert.ok(textOf().includes('setup.v4.doneTitle'), 'the card says everything answers correctly');
        assert.equal([...active.values()].filter(ms => ms === 60_000).length, 0, 'a done card keeps no minute timer');
        const stamp = tree.root.findAll(node => node.type === 'p' && [].concat(node.props.children).some(child => typeof child === 'string' && child.includes('setup.v4.checkedAt')));
        assert.equal(stamp.length, 1);
        for (let node = stamp[0]; node; node = node.parent) assert.notEqual(node.props?.['aria-live'], 'polite', 'the check time is outside the live region');
    } finally { globalThis.setInterval = realSet; globalThis.clearInterval = realClear; await cleanup(); }
});

test('wizard v2: a value the review would refuse is shown as not importable and never sent', async () => {
    init(); withPublicRecords([
        { name: 'example.com', type: 'MX', values: ['0 ', '10 mx.old.example'], state: 'found', not_importable: { '0 ': 'content_invalid' } },
        { name: 'example.com', type: 'TXT', values: ['v=spf1 -all', 'v=spf1 mx -all'], state: 'found', not_importable: { 'v=spf1 mx -all': 'one_per_name' } },
    ]);
    try {
        await mount(); await toRecords();
        assert.equal(tree.root.findAllByProps({ 'data-not-importable': 'content_invalid' }).length, 1);
        assert.equal(tree.root.findAllByProps({ 'data-not-importable': 'one_per_name' }).length, 1);
        await submit();
        const sent = state.draft.imported_records.map(item => `${item.type} ${item.content}`).sort();
        assert.deepEqual(sent, ['MX mx.old.example', 'TXT v=spf1 -all'], 'refused values are never part of the draft');
    } finally { await cleanup(); }
});

test('wizard v2: with an external provider the shown MX default is saved and DKIM says who adds it', async () => {
    init();
    try {
        await mount();
        await type('setup-main_domain', 'example.com'); await submit(); await submit(); await submit();
        await pick('setup-role', 'mail'); await submit(); await submit();
        await pick('setup-dns', 'external'); await submit();
        assert.ok(tree.root.findAllByProps({ 'data-setup-mail': '' }).length === 1);
        assert.equal(radio('setup-mail_mx', 'later').props.checked, true);
        assert.ok(textOf().includes('setup.v3.mail.dkimExternal'));
        await submit();
        assert.equal(state.draft.mail_dns.mx, 'later', 'the server applies exactly what the page showed');
    } finally { await cleanup(); }
});


// alpha.114: the second server joins the first by code at the start.
const joinedLink = { address: 'https://203.0.113.10:2083', main_domain: 'example.com', main_dns_server: 'first', ns1: 'ns1.example.com', ns2: 'ns2.example.com',
    first: { ipv4: '203.0.113.10', short_name: 'alpha', panel_domain: 'alpha.example.com', dns_engine: 'bind' }, imported: 0, joined_at: '2026-10-28T10:00:00Z' };
test('wizard v2: the second server joins with the first server\'s address and code; each answer says what happened', async () => {
    init();
    let joined = false; const answers = [
        { state: 'refused', code: 'server_setup_join_code_wrong', attempts_left: 3 },
        { state: 'unknown', code: 'server_setup_join_unknown' },
        { state: 'ok', code: 'server_setup_peer_draft_retry' },
    ];
    const base = globalThis.fetch;
    globalThis.fetch = async (url, options) => {
        if (url === '/api/v1/setup/peer') return Response.json(joined ? { role: 'second', link: joinedLink } : { role: 'none' });
        if (url === '/api/v1/setup/peer/connect') {
            calls.push({ url, options });
            const next = answers.shift();
            if (next) return Response.json(next);
            joined = true;
            state = { ...state, revision: state.revision + 1, status: 'draft', draft: { ...state.draft, main_domain: 'example.com', server_count: 2, dns_mode: 'local', dns_role: 'secondary', main_dns_server: 'peer', ns1: 'ns1.example.com', ns2: 'ns2.example.com', peer_ip: '203.0.113.10', peer_ns: 'ns1.example.com', dns_publisher_endpoint: 'https://alpha.example.com:2083' } };
            return Response.json({ state: 'ok', setup: state });
        }
        return base(url, options);
    };
    try {
        await mount(); await settleReads();
        await act(async () => findButton('setup.v5.join.open').props.onClick());
        await type('setup-join_address', '203.0.113.10:2083');
        await type('setup-join_code', 'ABCD-EFGH-IJKL-MNOP-QRST-UVWX');
        assert.equal(tree.root.findAllByProps({ 'data-setup-join-ip': '' }).length, 0, 'no address known, no line');
        assert.ok(input('setup-join_public_ipv4'), 'without a known public address the owner can type it (1:1 NAT)');
        await act(async () => findButton('setup.v5.join.submit').props.onClick());
        assert.equal(tree.root.findAllByProps({ 'data-setup-join-state': 'refused' }).length, 1, 'a refusal names its reason');
        assert.ok(textOf().includes('setup.v5.join.refused.server_setup_join_code_wrong'));
        await act(async () => findButton('setup.v5.join.retry').props.onClick());
        assert.ok(textOf().includes('setup.v5.join.state.unknown'), 'a lost answer says the same join is asked again');
        await act(async () => findButton('setup.v5.join.retry').props.onClick());
        assert.ok(textOf().includes('setup.v5.join.draftRetry'), 'a join whose answers could not be saved says what to do');
        assert.ok(input('setup-main_domain'), 'the wizard stays until the answers are applied');
        await act(async () => findButton('setup.v5.join.retry').props.onClick());
        await settleReads();
        const sent = calls.filter(call => call.url === '/api/v1/setup/peer/connect').map(call => JSON.parse(call.options.body));
        assert.equal(sent.length, 4);
        assert.deepEqual(sent[3], { address: '203.0.113.10:2083', code: 'ABCD-EFGH-IJKL-MNOP-QRST-UVWX' });
        assert.ok(input('setup-short_name'), 'after the join the wizard goes on with this server\'s own name');
        await act(async () => findButton('setup.back').props.onClick());
        assert.equal(tree.root.findAllByProps({ 'data-setup-joined': '' }).length, 1, 'the shared answers are shown, fixed');
        assert.equal(tree.root.findAllByProps({ id: 'setup-main_domain' }).length, 0);
    } finally { await cleanup(); }
});

test('wizard v2: the first server waiting for the second shows the code once and what the second has done', async () => {
    let view = { role: 'none' };
    const marker = { plan_id: 'a'.repeat(32), request_id: 'b'.repeat(32), panel_domain: 'panel.example.com' };
    init('admin', { status: 'waiting', draft: { ...fresh().draft, main_domain: 'example.com', server_count: 2, ns1: 'ns1.example.com', ns2: 'ns2.example.com', local_ip: '203.0.113.10' }, server_ip: '203.0.113.10' });
    store.set('celikpanel.setup.start.admin', JSON.stringify(marker));
    const waiting = { ...execution(marker, 'waiting'), phase: 'peer_join', error: { code: 'server_setup_peer_join_waiting', message: 'Waiting for the second server to join.' },
        steps: [{ id: '01-service', kind: 'service', target: 'nginx', status: 'succeeded' }, { id: '02-peer_join', kind: 'peer_join', target: 'second', status: 'running' }, { id: '03-dns', kind: 'dns', target: 'local', status: 'pending' }, { id: 'verify', kind: 'verify', target: 'web', status: 'pending' }] };
    const base = globalThis.fetch; let codes = 0;
    globalThis.fetch = async (url, options) => {
        if (url.includes('/setup/operation')) return Response.json(waiting);
        if (url === '/api/v1/setup/peer') return Response.json(view);
        if (url === '/api/v1/setup/join-codes') {
            codes++;
            view = { role: 'first', code: { address: 'https://203.0.113.10:2083', expires_at: '2026-10-29T10:00:00Z', attempts_left: 5, used: false } };
            return Response.json({ code: 'ABCD-EFGH-IJKL-MNOP-QRST-UVWX', address: 'https://203.0.113.10:2083', expires_at: '2026-10-29T10:00:00Z', attempts_left: 5 });
        }
        return base(url, options);
    };
    try {
        await mount(); await settleReads();
        assert.equal(tree.root.findAllByProps({ 'data-setup-join-card': '' }).length, 1, 'the join card leads while waiting');
        assert.ok(textOf().includes('setup.v5.wait.server_setup_peer_join_waiting'), 'the wait names its reason');
        assert.equal(codes, 0, 'reading never makes a code');
        await act(async () => findButton('setup.v5.card.create').props.onClick());
        assert.equal(codes, 1);
        assert.ok(textOf().includes('ABCD-EFGH-IJKL-MNOP-QRST-UVWX'), 'the code is shown once');
        assert.ok(textOf().includes('setup.v5.card.shownOnce'));
    } finally { await cleanup(); }
});
