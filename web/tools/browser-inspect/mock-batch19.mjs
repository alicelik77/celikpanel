// Batch 19 (2026-10-17): the ready bundles of alpha.94. Answers only while
// `state.b19` is set. It holds a small server: its components, which of them
// are installed, and the component installs the page asks for. An install is
// an operation that runs for a moment and then succeeds, unless its component
// is listed in `hold`, where it keeps running (to photograph a bundle's step).
// The bundles' states are derived here as the Panel derives them
// (cmd/panel/bundles.go), for the few bundles the scenarios draw.
//
//   b19.server   'boston' (only its DNS engine) | 'boston-dns' (PowerDNS
//                secondary with Nginx, its DNS identity not ready) |
//                'frankfurt' | 'running'
//   b19.hold     component ids whose install keeps running
//
// On dokuzuncu grup: alpha.94'ün hazır paketleri; kurulumlar sahte işlemdir.
import { withSamplePackageVersions } from './sample-package-versions.mjs';
const now = () => new Date().toISOString();
const hex = (n) => n.toString(16).padStart(32, '0');

const component = (id, name, category, installed, extra = {}) => ({
    id, name, description: `${name}`, icon: '', category, kind: 'service', versions: [], config_files: [], packages: [id], ports: [],
    is_installed: installed, status: installed ? 'active (running)' : 'not_installed', repair_available: true, ...(installed ? { unit: id } : {}), ...extra,
});

function catalogue(server) {
    const frankfurt = server === 'frankfurt';
    const running = server === 'running';
    const boston = server === 'boston' || server === 'boston-dns';
    const list = [
        component('bind', 'BIND', 'dns', !boston),
        component('pdns', 'PowerDNS', 'dns', boston),
        component('php-fpm', 'PHP-FPM', 'web', frankfurt, { kind: 'runtime', versions: frankfurt ? ['8.4'] : [] }),
        component('node', 'Node.js', 'web', false, { kind: 'runtime', packages: [] }),
        component('nginx', 'Nginx', 'web', frankfurt || server === 'boston-dns'),
        component('apache', 'Apache', 'web', false),
        component('postgresql', 'PostgreSQL', 'database', false),
        component('mariadb', 'MariaDB', 'database', frankfurt || running),
        component('phpmyadmin', 'phpMyAdmin', 'database', false, { kind: 'tool', requires_missing: frankfurt || running ? undefined : ['mariadb', 'web-server', 'php-fpm'] }),
        component('phppgadmin', 'phpPgAdmin', 'database', false, { kind: 'tool', requires_missing: ['postgresql', 'web-server', 'php-fpm'] }),
        component('postfix', 'Postfix', 'email', frankfurt),
        component('exim', 'Exim', 'email', false, frankfurt ? { conflict_with: 'Postfix' } : {}),
        component('rspamd', 'Rspamd', 'email', false),
        component('dovecot', 'Dovecot', 'email', frankfurt),
        component('roundcube', 'Roundcube', 'email', frankfurt, { kind: 'tool', status: frankfurt ? 'installed' : 'not_installed' }),
        component('spamassassin', 'SpamAssassin', 'email', false),
        component('certbot', 'certbot', 'security', true, { kind: 'tool', status: 'installed' }),
        // Mock data: a stopped service, to show the red dot and its word.
        component('fail2ban', 'Fail2ban', 'security', true, frankfurt ? { status: 'inactive (dead)' } : {}),
        component('clamav', 'ClamAV', 'security', false),
        component('redis', 'Redis', 'cache', false),
        component('cron', 'cron', 'system', true),
    ];
    return list;
}

const BUNDLES = [
    { id: 'mail.core', category: 'email', name_code: 'mail_core', components: ['postfix', 'dovecot'], action_route: 'mail_profile', profile_id: 'core-mail', addons: ['mail.protected'], anchors: ['postfix', 'dovecot'] },
    { id: 'mail.webmail', category: 'email', name_code: 'mail_webmail', components: ['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube'], action_route: 'mail_profile', profile_id: 'webmail', addons: ['mail.protected'], anchors: ['postfix', 'dovecot', 'roundcube'] },
    { id: 'mail.protected', category: 'email', name_code: 'mail_protected', components: ['postfix', 'dovecot', 'rspamd'], action_route: 'mail_profile', profile_id: 'protected-mail', addons: [], anchors: ['postfix', 'dovecot', 'rspamd'] },
    { id: 'db.postgresql', category: 'database', name_code: 'db_postgresql', components: ['postgresql'], action_route: 'components', addons: ['db.postgresql.admin'], anchors: ['postgresql'] },
    { id: 'db.mariadb', category: 'database', name_code: 'db_mariadb', components: ['mariadb'], action_route: 'components', addons: ['db.mariadb.admin'], anchors: ['mariadb'] },
    { id: 'db.postgresql.admin', category: 'database', name_code: 'db_postgresql_admin', components: ['nginx', 'php-fpm', 'phppgadmin'], action_route: 'components', addons: [], parent: 'db.postgresql', anchors: ['phppgadmin'] },
    { id: 'db.mariadb.admin', category: 'database', name_code: 'db_mariadb_admin', components: ['nginx', 'php-fpm', 'phpmyadmin'], action_route: 'components', addons: [], parent: 'db.mariadb', anchors: ['phpmyadmin'] },
    { id: 'web.nginx_php', category: 'web', name_code: 'web_nginx_php', components: ['nginx', 'php-fpm'], action_route: 'components', addons: [], anchors: ['nginx', 'php-fpm'] },
];

// The Panel's derivation, reduced to what these servers need: the mail
// bundles are verified once all their parts are present (the mock's
// Frankfurt has its proofs); an add-on waits for its parent.
// A component bundle is never partial (alpha.94 review): the parts present
// are kept and Install installs the rest. Without a DNS identity every mail
// bundle that is not installed is blocked; an installed one stays installed.
function bundles(services, dnsReady) {
    const installed = new Set(services.filter((item) => item.is_installed).map((item) => item.id));
    return BUNDLES.map(({ anchors, ...bundle }) => {
        const missing = bundle.components.filter((id) => !installed.has(id));
        const out = { ...bundle, missing_components: missing };
        const mail = bundle.action_route === 'mail_profile';
        if (mail && !dnsReady) {
            return missing.length === 0
                ? { ...out, status: 'installed_verified', action: 'none', blocked_code: 'dns_identity_not_ready' }
                : { ...out, status: 'blocked', action: 'none', blocked_code: 'dns_identity_not_ready' };
        }
        const parent = bundle.parent && BUNDLES.find((item) => item.id === bundle.parent);
        if (parent && !parent.components.every((id) => installed.has(id))) {
            return { ...out, status: 'blocked', action: 'none', blocked_code: 'component_requires', blocked_vars: { component: bundle.components[bundle.components.length - 1], required: parent.components[0] } };
        }
        if (missing.length === 0) return { ...out, status: 'installed_verified', action: bundle.action_route === 'mail_profile' ? 'repair' : 'none' };
        if (mail && anchors.some((id) => installed.has(id))) return { ...out, status: 'partial', action: 'continue' };
        return { ...out, status: 'available', action: 'install' };
    });
}

const DNS_REFUSAL = 'DNS identity is not ready. Install a supported DNS server and save the shared nameserver names and operating mode in Server → DNS and peer servers before installing a mail profile.';
const profile = (id, name, services, installedSet, dnsReady) => {
    if (!dnsReady) return { id, name, description: `${name} for this server`, status: 'blocked', available: false, verified: false, latest_attempt_status: 'none', services, blocked_reason: DNS_REFUSAL, blocked_code: 'dns_identity_not_ready' };
    const complete = services.every((item) => installedSet.has(item));
    const any = services.some((item) => installedSet.has(item) && ['postfix', 'dovecot', 'rspamd', 'roundcube'].includes(item));
    const status = complete ? 'complete' : any ? 'partial' : 'available';
    return { id, name, description: `${name} for this server`, status, available: true, verified: complete, latest_attempt_status: complete ? 'succeeded' : 'none', services };
};

export function batch19State(server, hold = []) {
    return { server, hold, dnsReady: server !== 'boston-dns', services: catalogue(server), operations: [] };
}

function payload(b19) {
    const installedSet = new Set(b19.services.filter((item) => item.is_installed).map((item) => item.id));
    return {
        scanned_at: now(), scan_age_seconds: 0, dns_identity_ready: b19.dnsReady !== false,
        mail_hostname: { current: 'mail.example.net', hostname: 'mail.example.net', source: 'saved', current_usable: true, will_set_hostname: false },
        // Package versions (alpha.97) are SAMPLE data, not measured: Ubuntu 24.04 strings for the Boston-like
        // servers, Debian 13 for the Frankfurt-like ones; a component the sample does not know carries none.
        services: b19.services.map((item) => withSamplePackageVersions(item, b19.server === 'boston' || b19.server === 'boston-dns' ? 'ubuntu2404' : 'debian13')),
        profiles: [
            profile('core-mail', 'Core Mail', ['postfix', 'dovecot'], installedSet, b19.dnsReady !== false),
            profile('webmail', 'Webmail', ['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube'], installedSet, b19.dnsReady !== false),
            profile('protected-mail', 'Spam-Protected Mail', ['postfix', 'dovecot', 'rspamd'], installedSet, b19.dnsReady !== false),
        ],
        bundles: bundles(b19.services, b19.dnsReady !== false),
    };
}

function view(b19, op) {
    if (op.status === 'running' && !b19.hold.includes(op.service_id) && Date.now() - op.at > 1500) {
        op.status = 'succeeded';
        op.phase = 'succeeded';
        op.finished_at = now();
        const service = b19.services.find((item) => item.id === op.service_id);
        if (service) Object.assign(service, { is_installed: true, status: service.kind === 'tool' ? 'installed' : 'active (running)', versions: service.kind === 'runtime' ? ['8.4'] : [], unit: service.id });
    }
    const { at, ...rest } = op;
    return rest;
}

export async function batch19(req, res, path, query, { state, send, readBody }) {
    const b19 = state.b19;
    if (!b19) return false;
    const key = `${req.method} ${path}`;
    if (key === 'GET /api/v1/managed-services' || key === 'POST /api/v1/managed-services/scan') {
        b19.operations.forEach((op) => view(b19, op));
        send(res, 200, payload(b19));
        return true;
    }
    if (key === 'POST /api/v1/service/install') {
        const body = await readBody(req);
        const op = { id: hex(0xb19000 + b19.operations.length + 1), request_id: body.request_id, kind: 'service_install', service_id: body.service_id, status: 'running', phase: 'installing', started_at: now(), at: Date.now() };
        b19.operations.push(op);
        send(res, 202, { operation: view(b19, op) });
        return true;
    }
    if (key === 'GET /api/v1/service/operation') {
        const id = query.get('id');
        const requestID = query.get('request_id');
        const op = b19.operations.find((item) => (id && item.id === id) || (requestID && item.request_id === requestID));
        if (op) { send(res, 200, { operation: view(b19, op) }); return true; }
        if (query.get('active') === '1') {
            const active = b19.operations.map((item) => view(b19, item)).find((item) => item.status === 'running');
            send(res, 200, { operation: active ?? null });
            return true;
        }
        return false;
    }
    return false;
}
