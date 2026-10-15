// Loopback-only mock routes for the sixteenth batch (2026-10-16): the second
// server as a full peer, part 1 (branch feat/alpha87-second-server). Loaded by
// mock.mjs before its own routes, because the setup record of an extension
// (`extension`, `extension_base`) and POST /api/v1/setup/extend are answered
// here. Like mock.mjs, this answers constants and contacts nothing.
//
// Everything lives in `state.b16`; the routes answer only while a scenario of
// this batch has set it, and each scenario clears it at its end.
//   status        the setup record's status ('ready', 'draft', 'running', ...)
//   guidance      'guided' (default) or 'manual'
//   draft         the setup draft as saved
//   base          the completed draft while an extension is open
//   extension     true while a completed setup is being extended
//   revision      the record's revision
//   extendRefuse  a typed refusal of POST /setup/extend (409 with that code)
//   extendDelay   ms before POST /setup/extend answers (the busy state)
//   catalog       what GET /api/v1/setup/components answers
// GET /api/v1/setup?check=1&manual=1 stays with batch 12 (`state.b12`).
//
// On altinci grup: ikinci sunucu tam es, birinci bolum.
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const MAIL = ['postfix', 'dovecot', 'rspamd', 'roundcube'];

function selected(b16) {
    const draft = b16.draft;
    if (draft.customization) return new Set(draft.customization.components);
    return new Set(b16.catalog.presets[draft.purpose] || []);
}

// The plan as cmd/panel builds it for these drafts (server_setup_operations.go):
// a service step for each component not installed, the DNS step, the mail
// profile, the firewall, the public DNS of the panel and of the mail host
// name, the mail certificate and its renewal enrolment, the final check.
function plan(b16) {
    const chosen = selected(b16);
    const installed = new Set(b16.catalog.components.filter(item => item.installed).map(item => item.id));
    const steps = [];
    const add = (kind, target, qualifier) => steps.push({ id: `${kind}-${target || steps.length}`, kind, target, ...(qualifier ? { qualifier } : {}) });
    for (const id of chosen) if (!installed.has(id) && !MAIL.includes(id)) add('service', id);
    add('dns', b16.draft.dns_mode);
    const mail = MAIL.some(id => chosen.has(id));
    if (mail) add('mail_profile', 'core-mail');
    add('firewall', 'enable_and_persist');
    add('access_dns', b16.draft.panel_domain, '203.0.113.10');
    if (mail) {
        add('access_dns', b16.draft.mail_hostname, '203.0.113.10');
        add('mail_certificate', b16.draft.mail_hostname);
        add('mail_enrollment', 'mail-renewal', 'c'.repeat(64));
    }
    add('verify', b16.draft.purpose);
    const components = b16.draft.customization
        ? b16.catalog.components.filter(item => chosen.has(item.id) || item.id === 'certbot').map(item => ({ id: item.id, selected: chosen.has(item.id), required: !chosen.has(item.id), installed: item.installed }))
        : undefined;
    return {
        id: 'f'.repeat(32), version: 1, revision: b16.revision, purpose: b16.draft.purpose, steps, blockers: [], can_start: true,
        tcp_ports: mail ? [22, 25, 80, 143, 443, 465, 587, 993, 2083] : [22, 80, 443, 2083], udp_ports: [],
        preserve_ssh: true, persist_firewall: true, contact_email: 'owner@example.com', server_ip: '203.0.113.10',
        ...(components ? { components } : {}),
    };
}

export async function batch16(req, res, path, query, { state, send, coded, readBody }) {
    const b16 = state.b16;
    if (!b16) return false;
    const key = `${req.method} ${path}`;
    const snapshot = () => ({
        version: 1, revision: b16.revision, origin: 'fresh', status: b16.status,
        required: !b16.extension && !['ready'].includes(b16.status) && b16.guidance !== 'manual',
        guidance: b16.guidance || 'guided', draft: b16.draft, checks: [], server_ip: '203.0.113.10',
        ...(b16.extension ? { extension: true, extension_base: b16.base } : {}),
    });
    if (key === 'GET /api/v1/setup' && !(query.get('manual') === '1' && state.b12?.manualChecks)) { send(res, 200, snapshot()); return true; }
    if (key === 'PUT /api/v1/setup') {
        const body = await readBody(req);
        if (body.revision !== b16.revision) { coded(res, 409, 'setup_conflict', 'the setup changed'); return true; }
        // A draft saved over a ready setup keeps the completed draft as the base.
        if (b16.status === 'ready') { b16.base = structuredClone(b16.draft); b16.extension = true; }
        b16.draft = { ...b16.draft, ...(body.draft || {}) };
        if (body.draft && !('customization' in body.draft)) delete b16.draft.customization;
        b16.status = 'draft'; b16.revision++;
        send(res, 200, snapshot()); return true;
    }
    if (key === 'POST /api/v1/setup/extend') {
        const body = await readBody(req);
        if (b16.extendDelay) await pause(b16.extendDelay);
        if (b16.extendRefuse) { coded(res, 409, b16.extendRefuse, 'server setup is running'); return true; }
        if (body.action === 'begin') {
            if (!b16.extension) { b16.base = structuredClone(b16.draft); b16.extension = true; }
            if (body.mail_profile) {
                const components = [...new Set([...(b16.draft.customization?.components || b16.catalog.presets[b16.draft.purpose] || []), 'postfix', 'dovecot'])];
                b16.draft = { ...b16.draft, purpose: 'web_mail', customization: { components } };
            }
            if (body.mail_hostname) b16.draft = { ...b16.draft, mail_hostname: body.mail_hostname };
            b16.status = 'draft'; b16.revision++;
            send(res, 200, snapshot()); return true;
        }
        if (body.action === 'cancel') {
            if (!b16.extension) { coded(res, 409, 'server_setup_extend_not_active', 'no extension is open'); return true; }
            b16.draft = b16.base; b16.base = undefined; b16.extension = false; b16.status = 'ready'; b16.revision++;
            send(res, 200, snapshot()); return true;
        }
        coded(res, 400, 'bad_request', 'unknown action'); return true;
    }
    if (key === 'GET /api/v1/setup/components') { send(res, 200, b16.catalog); return true; }
    if (key === 'POST /api/v1/setup/plan') { send(res, 200, plan(b16)); return true; }
    return false;
}
