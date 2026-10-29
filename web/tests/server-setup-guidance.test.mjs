import assert from 'node:assert/strict';
import test from 'node:test';
import { setupExecutionGuidance } from '../src/lib/serverSetupGuidance.ts';
import { enScreens as baseEnScreens } from './locale-catalogue.mjs';
import { trScreens as baseTrScreens } from './locale-catalogue.mjs';

import { enSetupDNS } from '../src/i18n/setupDNS/en.ts';
import { trSetupDNS } from '../src/i18n/setupDNS/tr.ts';
const enScreens = {...baseEnScreens,...enSetupDNS};
const trScreens = {...baseTrScreens,...trSetupDNS};

const context = (extra={}) => ({dns_mode:'local',dns_role:'primary',dns_engine:'bind',local_nameserver:'ns1.example.com',local_ip:'192.0.2.10',peer_nameserver:'ns2.example.com',peer_ip:'192.0.2.20',panel_domain:'panel.example.com',mail_hostname:'mail.example.com',dns_hosting_management:'',...extra});
const execution = (extra={}) => ({status:'running',phase:'01-dns',steps:[{id:'01-dns',kind:'dns',target:'local',status:'running'}],context:context(),...extra});
const keys = guidance => [...guidance.messages,...guidance.details].map(item=>item.key);

test('both initial DNS roles identify the reviewed peer without claiming it is offline',()=>{
    for(const role of ['primary','secondary']){
        const guide=setupExecutionGuidance(execution({context:context({dns_role:role})}));
        assert.ok(keys(guide).includes(role==='primary'?'setup.guide.startSecondary':'setup.guide.startPrimary'));
        assert.deepEqual(guide.messages[0].values,{local:'ns1.example.com',localIP:'192.0.2.10',peer:'ns2.example.com',peerIP:'192.0.2.20'});
        assert.ok(!keys(guide).includes('setup.guide.pairUnverified'),'initial work is not evidence of a failed pair check');
    }
});
test('DNS-only and manual secondary operation do not ask for panel authorization',()=>{
    for(const management of ['','manual','panel']){
        const guide=setupExecutionGuidance(execution({context:context({dns_role:'secondary',dns_hosting_management:management}),status:'waiting',phase:'dns_readiness'}));
        assert.ok(keys(guide).includes('setup.guide.pairUnverified'));
        assert.equal(keys(guide).includes('setup.guide.automaticRecords'),management==='panel');
        assert.equal(keys(guide).includes('setup.guide.manualRecords'),management==='manual');
        assert.ok(keys(guide).includes('setup.guide.nativeDNS'));
    }
});
test('external and existing DNS do not inherit paired-server installation instructions',()=>{
    for(const mode of ['external','existing']){
        const guide=setupExecutionGuidance(execution({context:context({dns_mode:mode,dns_role:''})}));
        assert.ok(!keys(guide).some(key=>['setup.guide.startPrimary','setup.guide.startSecondary','setup.guide.pairChecks'].includes(key)));
        assert.ok(keys(guide).includes(mode==='external'?'setup.guide.certificateDNS':'setup.guide.existingDNS'));
    }
});
test('a stopped secondary and an uncertain DNS result do not promise an install retry',()=>{
    const failed=setupExecutionGuidance(execution({status:'failed',steps:[{id:'01-dns',kind:'dns',target:'local',status:'failed'}],context:context({dns_role:'secondary'})}));
    assert.equal(failed.title,'setup.guide.failedTitle');assert.ok(keys(failed).includes('setup.guide.failed'));
    assert.ok(!keys(failed).includes('setup.guide.monitoring'));
    const uncertain=setupExecutionGuidance(execution({error:{code:'server_setup_reconciling',message:'pending receipt'}}));
    assert.equal(uncertain.title,'setup.guide.confirmTitle');assert.equal(uncertain.messages[0].key,'setup.guide.confirm');
    assert.ok(keys(uncertain).includes('setup.guide.startSecondary'));
});
test('certificate guidance uses the affected reviewed domain, never the mail domain for panel access',()=>{
    for(const kind of ['panel_certificate','mail_certificate']){
        const guide=setupExecutionGuidance(execution({steps:[{id:'cert',kind,target:'ignored editable value',status:'running'}]}));
        assert.equal(guide.messages[0].values.domain,kind==='mail_certificate'?'mail.example.com':'panel.example.com');
    }
});
test('license and missing observations stay distinct and preserve recovery meaning',()=>{
    const license=setupExecutionGuidance(execution({status:'waiting',phase:'license'}));
    assert.equal(license.title,'setup.licenseWaiting');assert.deepEqual(keys(license),['setup.guide.license']);
    const unknown=setupExecutionGuidance(execution({context:undefined,steps:[],phase:'new_backend_phase'}));
    assert.ok(keys(unknown).includes('setup.guide.unknown'));
    assert.equal(setupExecutionGuidance(execution({status:'succeeded'})),null);
});
test('verification separates DNS, mail delivery, identity, and unknown checks without false peer advice',()=>{
    const guide=setupExecutionGuidance(execution({status:'waiting',phase:'verification',context:context({dns_mode:'external',dns_role:''}),checks:[
        {id:'dns',state:'unknown',code:'dns_unavailable'},
        {id:'mail_delivery',state:'action_required',code:'mail_delivery_required'},
        {id:'mail_identity',state:'action_required',code:'mail_identity_required'},
        {id:'future_check',state:'unknown',code:'future_check_unavailable'},
        {id:'firewall',state:'ready',code:''},
    ]}));
    for(const key of ['externalChecks','mailDelivery','mailIdentity','checkUnknown'])assert.ok(keys(guide).includes('setup.guide.'+key));
    assert.ok(!keys(guide).includes('setup.guide.pairChecks'));
    assert.ok(!keys(guide).includes('setup.guide.firewall'));
});
test('all guidance states have complete matching English and Turkish placeholders',()=>{
    const enKeys=Object.keys(enScreens).filter(key=>key.startsWith('setup.guide.'));
    assert.deepEqual(Object.keys(trScreens).filter(key=>key.startsWith('setup.guide.')).sort(),enKeys.sort());
    for(const key of enKeys){
        assert.equal(typeof trScreens[key],'string');
        assert.deepEqual((enScreens[key].match(/\{[^}]+\}/g)||[]).sort(),(trScreens[key].match(/\{[^}]+\}/g)||[]).sort(),key);
        assert.ok(!trScreens[key].includes('\ufffd'));
    }
});


test('a new build requires plan review without misdiagnosing DNS or component failure',()=>{
    const guide=setupExecutionGuidance(execution({status:'failed',error:{code:'server_setup_build_changed',message:'review'},steps:[{id:'service',kind:'service',target:'nginx',status:'failed'}]}));
    assert.equal(guide.title,'setup.guide.buildChangedTitle');
    assert.deepEqual(keys(guide),['setup.guide.buildChanged']);
});


test('public DNS prerequisite uses the exact certificate step for panel and mail and distinguishes wrong from unknown answers',()=>{
    for(const domain of ['panel.example.com','mail.example.com'])for(const code of ['server_setup_access_dns_required','server_setup_access_dns_mismatch']){
        const guide=setupExecutionGuidance(execution({status:'waiting',phase:'access_dns',error:{code,message:'result'},context:context({access_dns_ip:'192.0.2.10'}),steps:[{id:'access',kind:'access_dns',target:domain,qualifier:'192.0.2.10',status:'running'}]}));
        assert.deepEqual(guide.messages[0].values,{domain,ip:'192.0.2.10'});
        assert.ok(keys(guide).includes(code.endsWith('mismatch')?'setup.guide.accessDNSMismatch':'setup.guide.accessDNSUnknown'));
        assert.ok(keys(guide).includes('setup.guide.accessDNSResume'));
    }
});

test('public DNS ownership determines who must act without requiring a website or remote panel',()=>{
    for(const mode of ['local','external','existing'])for(const role of ['primary','secondary']){
        const infrastructure_dns=mode==='local'&&role==='primary'?{zone:'example.com'}:undefined;
        const guide=setupExecutionGuidance(execution({status:'waiting',phase:'access_dns',context:context({dns_mode:mode,dns_role:role,infrastructure_dns}),steps:[{id:'access',kind:'access_dns',target:'panel.example.com',qualifier:'192.0.2.10',status:'running'}]}));
        const expected=mode==='local'?(role==='primary'?'setup.guide.accessDNSPrepared':'setup.guide.accessDNSSecondary'):'setup.guide.accessDNSProvider';
        assert.ok(keys(guide).includes(expected));
        assert.ok(!keys(guide).includes('setup.guide.automaticRecords'));
    }
});

test('stopped DNS checks never promise automatic continuation while active secondary waits identify the primary',()=>{
    const failed=setupExecutionGuidance(execution({status:'failed',phase:'access_dns',steps:[{id:'access',kind:'access_dns',target:'panel.example.com',qualifier:'192.0.2.10',status:'failed'}]}));
    assert.ok(keys(failed).includes('setup.guide.failed'));
    assert.ok(!keys(failed).includes('setup.guide.accessDNSResume'));
    const waiting=setupExecutionGuidance(execution({status:'waiting',phase:'primary_dns',context:context({dns_role:'secondary'})}));
    assert.ok(keys(waiting).includes('setup.guide.primaryDNSWaiting'));
    assert.deepEqual(waiting.messages[0].values,{primary:'ns2.example.com',primaryIP:'192.0.2.20'});
    assert.ok(keys(waiting).includes('setup.guide.nativeDNS'));
});

test('unknown infrastructure publication preserves uncertainty instead of directing duplicate creation',()=>{
    const guide=setupExecutionGuidance(execution({phase:'infrastructure_dns',error:{code:'server_setup_infrastructure_dns_unknown',message:'pending outcome'},context:context({infrastructure_dns:{zone:'example.com'}})}));
    assert.ok(keys(guide).includes('setup.infrastructure.unknown'));
    assert.ok(!keys(guide).includes('setup.guide.infrastructureDNSPeer'));
    assert.ok(!keys(guide).includes('setup.guide.accessDNSResume'));
});

test('infrastructure record copy has complete bilingual placeholders and intact Turkish characters',()=>{
    const enKeys=Object.keys(enScreens).filter(key=>key.startsWith('setup.infrastructure.'));
    assert.deepEqual(Object.keys(trScreens).filter(key=>key.startsWith('setup.infrastructure.')).sort(),enKeys.sort());
    for(const key of enKeys){
        assert.deepEqual((enScreens[key].match(/\{[^}]+\}/g)||[]).sort(),(trScreens[key].match(/\{[^}]+\}/g)||[]).sort(),key);
        assert.ok(!trScreens[key].includes('?'),key);
    }
    assert.ok(trScreens['setup.infrastructure.title'].includes('kayıtları'));
});


test('mail enrollment guidance distinguishes recorded forward, inverse, unknown and failed outcomes in both languages',()=>{
    for(const [code,status,key] of [
        ['server_setup_mail_enrollment_running','running','setup.guide.mailEnrollmentRecorded'],
        ['server_setup_mail_enrollment_rollback','running','setup.guide.mailEnrollmentRollback'],
        ['server_setup_mail_enrollment_unknown','running','setup.guide.mailEnrollmentUnknown'],
        ['server_setup_mail_enrollment_not_recorded','running','setup.guide.mailEnrollmentUnrecorded'],
        ['mail_enrollment_restored','failed','setup.guide.mailEnrollmentFailed'],
    ]){
        const guide=setupExecutionGuidance(execution({status,phase:'mail-renewal',steps:[{id:'mail-renewal',kind:'mail_enrollment',target:'mail-renewal',status:status==='failed'?'failed':'running'}],error:{code,message:'recorded'}}));
        assert.ok(keys(guide).includes(key));
        assert.ok(enScreens[key]);assert.ok(trScreens[key]);
        assert.ok(!keys(guide).includes('setup.guide.component'));
    }
});

// Second server as a full peer, part 2 (2026-10-16): on a DNS secondary whose
// public DNS check waits, the guidance says first what the request to the
// primary got. Applied: nothing to do, and no instruction to add the record
// by hand. Every other outcome keeps the manual record instruction.
const peerWait = (peer) => execution({status:'waiting',phase:'access_dns',
    steps:[{id:'05-access_dns',kind:'access_dns',target:'mail.boston.example.com',qualifier:'192.0.2.10',status:'running'}],
    context:context({dns_role:'secondary',peer_host_record:peer})});
test('a secondary waiting for its host record says what the primary answered, and the manual record only when the owner must act',()=>{
    const applied=setupExecutionGuidance(peerWait({channel:true,outcome:{status:'applied',result:'created',zone:'example.com',name:'mail.boston.example.com',content:'192.0.2.10'}}));
    assert.ok(keys(applied).includes('setup.guide.peerRecordApplied'));
    assert.ok(!keys(applied).includes('setup.guide.accessDNSSecondary'),'an applied record still asks the owner to add it');
    assert.equal(applied.messages.find(item=>item.key==='setup.guide.peerRecordApplied').values.zone,'example.com');
    const present=setupExecutionGuidance(peerWait({channel:true,outcome:{status:'applied',result:'present',zone:'example.com',name:'x',content:'y'}}));
    assert.ok(keys(present).includes('setup.guide.peerRecordPresent'));
    for(const [outcome,key] of [
        [{status:'refused',code:'REMOTE_DNS_HOST_RECORD_CONFLICT'},'setup.guide.peerRecordRefused.REMOTE_DNS_HOST_RECORD_CONFLICT'],
        [{status:'refused',code:'REMOTE_DNS_HOST_RECORD_OUTSIDE_ZONE'},'setup.guide.peerRecordRefused.REMOTE_DNS_HOST_RECORD_OUTSIDE_ZONE'],
        [{status:'refused',code:'SOMETHING_NEW'},'setup.guide.peerRecordRefused.other'],
        [{status:'unsupported'},'setup.guide.peerRecordUnsupported'],
        [{status:'unknown'},'setup.guide.peerRecordUnknown'],
    ]){
        const guide=setupExecutionGuidance(peerWait({channel:true,outcome:{name:'n',content:'c',...outcome}}));
        assert.ok(keys(guide).includes(key),key);
        assert.ok(keys(guide).includes('setup.guide.accessDNSSecondary'),`${key} drops the manual record`);
    }
    const none=setupExecutionGuidance(peerWait({channel:false}));
    assert.ok(keys(none).includes('setup.guide.peerRecordNoChannel'));
    assert.ok(keys(none).includes('setup.guide.accessDNSSecondary'));
    const asking=setupExecutionGuidance(peerWait({channel:true}));
    assert.ok(keys(asking).includes('setup.guide.peerRecordAsking'));
    // Control: a primary and an older server without the field say what they said before.
    const older=setupExecutionGuidance(peerWait(undefined));
    assert.ok(!keys(older).some(key=>key.startsWith('setup.guide.peerRecord')));
    assert.ok(keys(older).includes('setup.guide.accessDNSSecondary'));
});
test('every sentence about the record requested from the primary exists in both languages and names the primary',()=>{
    const peerKeys=Object.keys(enSetupDNS).filter(key=>key.startsWith('setup.guide.peerRecord'));
    assert.equal(peerKeys.length,10);
    for(const key of peerKeys){
        assert.ok(trScreens[key]&&trScreens[key]!==enScreens[key],`${key} has no Turkish text`);
        for(const table of [enScreens,trScreens]) assert.ok(table[key].includes('{primary}'),`${key} does not name the primary`);
    }
});
// Beta-1 re-run P7: the backup waits to tell the main DNS server it is ready;
// the reason is said in both languages, not as an unknown wait.
test('the backup waiting to report ready names that reason in both languages',()=>{
    for(const code of ['server_setup_peer_ready_unreachable','server_setup_peer_ready_refused','server_setup_peer_ready_identity_changed']){
        const guide=setupExecutionGuidance(execution({status:'waiting',phase:'verification',error:{code,message:'x'},steps:[{id:'02-verify',kind:'verify',target:'web',status:'pending'}]}));
        assert.ok(keys(guide).includes(`setup.v5.wait.${code}`),code);
        assert.ok(enScreens[`setup.v5.wait.${code}`]&&trScreens[`setup.v5.wait.${code}`],code);
    }
});
// Beta-1 re-run P8: a run stopped because its second server joined again
// says so, with the owner's next action, never the generic failure.
test('a run stopped by a rejoin names that state in both languages',()=>{
    const guide=setupExecutionGuidance(execution({status:'failed',phase:'10-dns_publisher',error:{code:'server_setup_plan_changed_by_rejoin',message:'x'},steps:[{id:'10-dns_publisher',kind:'dns_publisher',target:'',status:'failed'}]}));
    assert.ok(keys(guide).includes('setup.v5.wait.server_setup_plan_changed_by_rejoin'));
    assert.match(enScreens['setup.v5.wait.server_setup_plan_changed_by_rejoin'],/Review a revised plan/);
    assert.match(trScreens['setup.v5.wait.server_setup_plan_changed_by_rejoin'],/Düzeltilmiş planı incele/);
});
// Beta-1 third re-run N1: a booting host is a wait with its own sentence;
// a host that stays starting is a verified failure naming systemd.
test('a booting host is said as a wait, and its bound as a failure',()=>{
    for(const [code,status] of [['server_setup_host_starting','waiting'],['server_setup_host_not_started','failed'],['server_setup_agent_not_answering','failed']]){
        const guide=setupExecutionGuidance(execution({status,error:{code,message:'x'}}));
        assert.ok(keys(guide).includes(`setup.v5.wait.${code}`),code);
        assert.ok(enScreens[`setup.v5.wait.${code}`]&&trScreens[`setup.v5.wait.${code}`],code);
    }
});
// Beta-1 third re-run N6: the setup link's texts name the servers by role
// (the server that created the join code, the main DNS server), never by
// first/second, which in Option A pointed at the wrong server. Review fix
// (item 10): the waiting card for the other server and its button too, and
// the button's name quoted in other texts is the card's own.
const roleNamed=key=>key.startsWith('setup.v5.wait.')||key.startsWith('setup.v5.peerSetup.');
test('the setup link texts name the servers by role',()=>{
    for(const [key,text] of Object.entries(enScreens).filter(([key])=>roleNamed(key))){
        assert.ok(!/first server|second server/i.test(text),`${key}: ${text}`);
    }
    for(const [key,text] of Object.entries(trScreens).filter(([key])=>roleNamed(key))){
        assert.ok(!/ilk sunucu|ikinci sunucu/i.test(text),`${key}: ${text}`);
    }
    assert.ok(enScreens['setup.v5.wait.server_setup_peer_ready_refused'].includes(`"${enScreens['setup.v5.peerSetup.stop']}"`));
    assert.ok(trScreens['setup.v5.wait.server_setup_peer_ready_refused'].includes(`"${trScreens['setup.v5.peerSetup.stop']}"`));
});
