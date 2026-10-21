// Batch 20 (2026-10-17): the one ready-bundle list of alpha.95. Answers only
// while `state.b20` is set. It holds a small server: its components, which of
// them are installed, and the component installs the page asks for. An
// install is an operation that runs for a moment and then succeeds, unless
// its component is listed in `hold`, where it keeps running (to photograph a
// bundle's step).
//
// Honest by construction (the owner's alpha.94 screens): the scan reads
// `versions` only for the multi-version runtimes (PHP-FPM, Node.js); every other
// component carries `versions: []`. Since alpha.97 an installed component that
// is not a runtime may also carry the package_version* fields (the system
// package database's reading). They are SAMPLE data here, not measured
// (sample-package-versions.mjs: Debian 13 strings for Frankfurt, Ubuntu 24.04
// strings for Boston); a component the sample does not know carries none. The bundles are derived as the Panel
// derives them (cmd/panel/bundles.go, mail_profiles.go): a mail bundle takes
// its state from its mail profile (installed once all parts are ready; partial
// only when a mail component is present; Nginx or PHP-FPM alone leave it
// available, with the partly-present note); a component bundle is available
// when something is missing and blocked ("component_requires") when an add-on's
// database is not there, since componentPlanBlock projects only the bundle's
// own missing parts onto what is installed.
//
//   b20.server   'boston' (PowerDNS secondary: Nginx, Node.js, PostgreSQL,
//                Fail2ban, certbot, nftables, cron) | 'frankfurt' (everything
//                installed and verified)
//   b20.dnsReady false: the DNS identity is not ready (host-wide mail refusal)
//   b20.hold     component ids whose install keeps running
//   b20.versions 'sample' (default) | 'none': whether package versions are known
//   b20.kept     component ids whose reading is an earlier one (batch 22)
//   b20.extra    component ids installed on top of the server's own set (batch 22)
//
// On yirminci grup: alpha.95 tek paket listesi; kurulumlar sahte islemdir.
import { withSamplePackageVersions } from './sample-package-versions.mjs';
const now = () => new Date().toISOString();
const hex = (n) => n.toString(16).padStart(32, '0');

// What the scan reads a version for: the runtimes only.
const RUNTIME_VERSION = { 'php-fpm': ['8.4'], node: ['22.11.0'] };

const component = (id, name, category, installed, extra = {}) => ({
    id, name, description: `${name}`, icon: '', category, kind: 'service', versions: [], config_files: [], packages: [id], ports: [],
    is_installed: installed, status: installed ? 'active (running)' : 'not_installed', repair_available: true, ...(installed ? { unit: id } : {}), ...extra,
});
const tool = (id, name, category, installed) => {
    const { unit, ...rest } = component(id, name, category, installed, { kind: 'tool', status: installed ? 'installed' : 'not_installed' });
    return rest;
};
const runtime = (id, name, category, installed) => component(id, name, category, installed, { kind: 'runtime', versions: installed ? RUNTIME_VERSION[id] : [], packages: id === 'node' ? [] : [id] });

function catalogue(server) {
    const f = server === 'frankfurt';
    return [
        component('bind', 'BIND', 'dns', f),
        component('pdns', 'PowerDNS', 'dns', !f),
        runtime('php-fpm', 'PHP-FPM', 'web', f),
        runtime('node', 'Node.js', 'web', true),
        component('nginx', 'Nginx', 'web', true),
        component('apache', 'Apache', 'web', false),
        component('postgresql', 'PostgreSQL', 'database', true),
        component('mariadb', 'MariaDB', 'database', f),
        tool('phpmyadmin', 'phpMyAdmin', 'database', f),
        tool('phppgadmin', 'phpPgAdmin', 'database', f),
        component('postfix', 'Postfix', 'email', f),
        component('exim', 'Exim', 'email', false),
        component('rspamd', 'Rspamd', 'email', f),
        component('dovecot', 'Dovecot', 'email', f),
        tool('roundcube', 'Roundcube', 'email', f),
        component('spamassassin', 'SpamAssassin', 'email', false),
        tool('certbot', 'certbot', 'security', true),
        component('fail2ban', 'Fail2ban', 'security', true),
        tool('nftables', 'Firewall engine', 'security', true),
        component('clamav', 'ClamAV', 'security', false),
        component('redis', 'Redis', 'cache', false),
        component('cron', 'cron', 'system', true),
    ];
}

// The registry order of cmd/panel/bundles.go.
const BUNDLES = [
    { id: 'mail.core', category: 'email', name_code: 'mail_core', components: ['postfix', 'dovecot'], action_route: 'mail_profile', profile_id: 'core-mail', addons: ['mail.protected'] },
    { id: 'mail.webmail', category: 'email', name_code: 'mail_webmail', components: ['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube'], action_route: 'mail_profile', profile_id: 'webmail', addons: ['mail.protected'] },
    { id: 'mail.protected', category: 'email', name_code: 'mail_protected', components: ['postfix', 'dovecot', 'rspamd'], action_route: 'mail_profile', profile_id: 'protected-mail', addons: [] },
    { id: 'db.postgresql', category: 'database', name_code: 'db_postgresql', components: ['postgresql'], action_route: 'components', addons: ['db.postgresql.admin'] },
    { id: 'db.mariadb', category: 'database', name_code: 'db_mariadb', components: ['mariadb'], action_route: 'components', addons: ['db.mariadb.admin'] },
    { id: 'db.postgresql.admin', category: 'database', name_code: 'db_postgresql_admin', components: ['nginx', 'php-fpm', 'phppgadmin'], action_route: 'components', addons: [], parent: 'db.postgresql' },
    { id: 'db.mariadb.admin', category: 'database', name_code: 'db_mariadb_admin', components: ['nginx', 'php-fpm', 'phpmyadmin'], action_route: 'components', addons: [], parent: 'db.mariadb' },
    { id: 'web.nginx_php', category: 'web', name_code: 'web_nginx_php', components: ['nginx', 'php-fpm'], action_route: 'components', addons: [] },
];
const MAIL_COMPONENTS = new Set(['postfix', 'dovecot', 'rspamd', 'roundcube']);
const NAMES = { mariadb: 'MariaDB', postgresql: 'PostgreSQL', phpmyadmin: 'phpMyAdmin', phppgadmin: 'phpPgAdmin' };
// What a tool needs beyond the web server and PHP-FPM (core.RequirementsMissing).
const NEEDS = { phpmyadmin: 'mariadb', phppgadmin: 'postgresql' };

const ready = (id, services) => {
    const item = services.find((entry) => entry.id === id);
    return !!item && item.is_installed && (item.kind === 'tool' || /^active/.test(item.status));
};

// The mail profile of a set of services (mail_profiles.go): complete when all
// are ready, partial when a mail component is present, else available; Nginx
// or PHP-FPM alone leave it available with the partly-present note.
function profileState(def, services) {
    const present = def.components.filter((id) => services.find((item) => item.id === id)?.is_installed);
    const complete = def.components.every((id) => ready(id, services));
    if (complete) return { status: 'complete', verified: true };
    if (present.some((id) => MAIL_COMPONENTS.has(id))) return { status: 'partial', warning_code: 'partly_present' };
    return present.length > 0 ? { status: 'available', warning_code: 'partly_present' } : { status: 'available' };
}

function bundles(services, dnsReady) {
    const installed = new Set(services.filter((item) => item.is_installed).map((item) => item.id));
    return BUNDLES.map((def) => {
        const mail = def.action_route === 'mail_profile';
        const present = (id) => installed.has(id) || (!!def.parent && id === 'nginx' && installed.has('apache'));
        const missing = def.components.filter((id) => !present(id));
        const out = { ...def, missing_components: missing };
        if (mail) {
            const p = profileState(def, services);
            // Behind the host-wide refusal the profile is blocked before it can warn (mail_profiles.go).
            const base = p.warning_code && dnsReady ? { ...out, warning_code: p.warning_code } : out;
            if (!dnsReady) {
                // Installed before the host-wide refusal: it stays installed.
                return p.status === 'complete'
                    ? { ...base, status: 'installed_verified', action: 'repair', blocked_code: 'dns_identity_not_ready' }
                    : { ...base, status: 'blocked', action: 'none', blocked_code: 'dns_identity_not_ready' };
            }
            if (p.status === 'complete') return { ...base, status: 'installed_verified', action: 'repair' };
            if (p.status === 'partial') return missing.length === 0 ? { ...base, status: 'installed_unverified', action: 'repair' } : { ...base, status: 'partial', action: 'continue' };
            return { ...base, status: 'available', action: 'install' };
        }
        if (missing.length === 0) {
            return def.components.every((id) => ready(id, services))
                ? { ...out, status: 'installed_verified', action: 'none' }
                : { ...out, status: 'installed_unverified', action: 'verify' };
        }
        // componentPlanBlock: the bundle's own missing parts are projected onto
        // what is installed; a tool whose database is not there is blocked.
        const projected = new Set([...installed, ...missing]);
        for (const id of missing) {
            const need = NEEDS[id];
            if (need && !projected.has(need)) {
                return { ...out, status: 'blocked', action: 'none', blocked_code: 'component_requires', blocked_vars: { component: NAMES[id], required: NAMES[need] } };
            }
        }
        return { ...out, status: 'available', action: 'install' };
    });
}

const DNS_REFUSAL = 'DNS identity is not ready. Install a supported DNS server and save the shared nameserver names and operating mode in Settings before installing a mail profile.';
const PARTLY = 'Some components are already present. Installing this profile will reconcile the full plan without removing completed services.';
const PROFILES = [
    { id: 'core-mail', name: 'Core Mail', components: ['postfix', 'dovecot'] },
    { id: 'webmail', name: 'Webmail', components: ['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube'] },
    { id: 'protected-mail', name: 'Spam-Protected Mail', components: ['postfix', 'dovecot', 'rspamd'] },
];
const profile = (def, services, dnsReady) => {
    const base = { id: def.id, name: def.name, description: `${def.name} for this server`, services: def.components };
    if (!dnsReady) return { ...base, status: 'blocked', available: false, verified: false, latest_attempt_status: 'none', blocked_reason: DNS_REFUSAL, blocked_code: 'dns_identity_not_ready' };
    const p = profileState(def, services);
    return {
        ...base, status: p.status, available: true, verified: !!p.verified, latest_attempt_status: p.verified ? 'succeeded' : 'none',
        ...(p.warning_code ? { warning: PARTLY, warning_code: p.warning_code } : {}),
    };
};

export function batch20State(server, { hold = [], dnsReady = true, versions = 'sample', kept = [], extra = [] } = {}) {
    const services = catalogue(server).map((item) => (extra.includes(item.id) && !item.is_installed
        ? { ...item, is_installed: true, status: item.kind === 'tool' ? 'installed' : 'active (running)', ...(item.kind === 'tool' ? {} : { unit: item.id }) } : item));
    return { server, hold, dnsReady, versions, kept, services, operations: [] };
}

function payload(b20) {
    const ok = b20.dnsReady !== false;
    // The tools' unmet requirements, as the scan lists them (the tokens the page names).
    const have = new Set(b20.services.filter((item) => item.is_installed).map((item) => item.id));
    // Sample package versions (not measured) for what is installed and is not a runtime.
    const distro = b20.server === 'frankfurt' ? 'debian13' : 'ubuntu2404';
    const services = b20.services.map((item) => {
        if (item.is_installed) return b20.versions === 'none' ? item : withSamplePackageVersions(item, distro, { kept: b20.kept ?? [] });
        if (!NEEDS[item.id]) return item;
        const missing = [NEEDS[item.id], ...(have.has('nginx') || have.has('apache') ? [] : ['web-server']), ...(have.has('php-fpm') ? [] : ['php-fpm'])].filter((id) => !have.has(id));
        return missing.length > 0 ? { ...item, requires_missing: missing } : item;
    });
    return {
        scanned_at: now(), scan_age_seconds: 0, dns_identity_ready: ok,
        mail_hostname: { current: 'mail.example.net', hostname: 'mail.example.net', source: 'saved', current_usable: true, will_set_hostname: false },
        services,
        profiles: PROFILES.map((def) => profile(def, b20.services, ok)),
        bundles: bundles(b20.services, ok),
    };
}

function view(b20, op) {
    if (op.status === 'running' && !b20.hold.includes(op.service_id) && Date.now() - op.at > 1500) {
        op.status = 'succeeded';
        op.phase = 'succeeded';
        op.finished_at = now();
        const service = b20.services.find((item) => item.id === op.service_id);
        if (service) Object.assign(service, { is_installed: true, status: service.kind === 'tool' ? 'installed' : 'active (running)', versions: RUNTIME_VERSION[service.id] ?? [], ...(service.kind === 'tool' ? {} : { unit: service.id }) });
    }
    const { at, ...rest } = op;
    return rest;
}

export async function batch20(req, res, path, query, { state, send, readBody }) {
    const b20 = state.b20;
    if (!b20) return false;
    const key = `${req.method} ${path}`;
    if (key === 'GET /api/v1/managed-services' || key === 'POST /api/v1/managed-services/scan') {
        b20.operations.forEach((op) => view(b20, op));
        send(res, 200, payload(b20));
        return true;
    }
    if (key === 'POST /api/v1/service/install') {
        const body = await readBody(req);
        const op = { id: hex(0xb20000 + b20.operations.length + 1), request_id: body.request_id, kind: 'service_install', service_id: body.service_id, status: 'running', phase: 'installing', started_at: now(), at: Date.now() };
        b20.operations.push(op);
        send(res, 202, { operation: view(b20, op) });
        return true;
    }
    if (key === 'GET /api/v1/service/operation') {
        const id = query.get('id');
        const requestID = query.get('request_id');
        const op = b20.operations.find((item) => (id && item.id === id) || (requestID && item.request_id === requestID));
        if (op) { send(res, 200, { operation: view(b20, op) }); return true; }
        if (query.get('active') === '1') {
            const active = b20.operations.map((item) => view(b20, item)).find((item) => item.status === 'running');
            send(res, 200, { operation: active ?? null });
            return true;
        }
        return false;
    }
    return false;
}
