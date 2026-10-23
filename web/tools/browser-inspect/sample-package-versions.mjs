// Sample installed-package versions for the mocks of batches 19, 20 and 22
// (v0.1.0-alpha.97). SAMPLE DATA, NOT MEASURED: the strings below are shaped
// like what dpkg prints on Debian 13 (trixie, the Frankfurt-shaped mock) and on
// Ubuntu 24.04 (noble, the Boston-shaped mock), so that the screens are drawn
// with realistic lengths, epochs and revisions. They were not read from any
// server, and no installed server is contacted.
//
// Honest by construction: the fields are added only the way the Panel adds
// them (an installed component that is not a multi-version runtime and whose
// packages the table below knows); a component the table does not know
// (Roundcube; cron; Rspamd) carries NO field at all, which the interface reads
// as unknown and draws as nothing. alpha.101: the Debian 13 database tools have
// samples too, phpMyAdmin's shaped as a Debian "really" version (the owner saw
// "5.2.2-really…" on Frankfurt); still sample strings, not Frankfurt's own.
//
// Fields: package_version, package_version_short, package_versions_differ,
// package_versions [{package, version}], package_version_kept,
// package_version_read_at (RFC 3339, second precision, Z). When a component's
// packages report different versions, package_version and
// package_version_short are absent.

const DEB = '+deb13u1';
const dovecot = (v) => ['dovecot-core', 'dovecot-imapd', 'dovecot-pop3d', 'dovecot-lmtpd'].map((p) => [p, v]);

// component id -> [[package, version], ...]
const SAMPLES = {
    // Debian 13 (trixie).
    debian13: {
        nginx: [['nginx', `1.26.3-3${DEB}`]],
        mariadb: [['mariadb-server', `1:11.8.3-0${DEB}`]],
        postgresql: [['postgresql-17', `17.6-0${DEB}`]],
        postfix: [['postfix', '3.10.4-1~deb13u1']],
        dovecot: dovecot(`1:2.4.1+dfsg1-6${DEB}`),
        certbot: [['certbot', '4.0.0-2']],
        bind: [['bind9', `1:9.20.11-4${DEB}`]],
        redis: [['redis-server', '5:8.0.2-3']],
        fail2ban: [['fail2ban', '1.1.0-8']],
        nftables: [['nftables', '1.1.3-1']],
        phpmyadmin: [['phpmyadmin', '4:5.2.2-really+dfsg-1+deb13u1']],
        phppgadmin: [['phppgadmin', '7.14.7+dfsg-1']],
        // Two packages, two versions: the row says "several versions".
        spamassassin: [['spamassassin', '4.0.1-3'], ['spamass-milter', '0.4.0-2.1']],
    },
    // Ubuntu 24.04 (noble).
    ubuntu2404: {
        nginx: [['nginx', '1.24.0-2ubuntu7.5']],
        mariadb: [['mariadb-server', '1:10.11.13-0ubuntu0.24.04.1']],
        postgresql: [['postgresql-16', '16.10-0ubuntu0.24.04.1']],
        postfix: [['postfix', '3.8.6-1build2']],
        dovecot: dovecot('1:2.3.21+dfsg1-2ubuntu6'),
        certbot: [['certbot', '2.9.0-1']],
        pdns: [['pdns-server', '4.8.3-4build3']],
        redis: [['redis-server', '5:7.0.15-1ubuntu0.24.04.2']],
        fail2ban: [['fail2ban', '1.0.2-3ubuntu0.1']],
        nftables: [['nftables', '1.0.9-1build1']],
    },
};

// The upstream part, as the Panel derives it (upstreamPackageVersion): drop
// the epoch ("N:"), drop the last "-revision", take the real version after a
// Debian "really" (alpha.101; alpha.104 when no version follows it), cut at
// the first "+" or "~".
export function shortVersion(full) {
    let v = full.replace(/^\d+:/, '');
    const dash = v.lastIndexOf('-');
    if (dash > 0) v = v.slice(0, dash);
    const really = /[+~.-]really(\d.*)$/.exec(v);
    if (really) v = really[1];
    // alpha.104: "really" followed by a suffix, not a version (Debian 13's
    // phpMyAdmin 4:5.2.2-really+dfsg-1+deb13u1): the version before it.
    else {
        const marker = /^(\d.*?)[+~.-]really[+~]/.exec(v);
        if (marker) v = marker[1];
    }
    return v.split(/[+~]/)[0];
}

const instant = (ms) => new Date(ms).toISOString().replace(/\.\d{3}Z$/, 'Z');

// `distro` is 'debian13' | 'ubuntu2404'. `kept` lists component ids whose
// reading is shown as an earlier one (the package manager was busy at the last
// scan). Returns the service unchanged when it carries no known version.
export function withSamplePackageVersions(service, distro, { kept = [], readAt = Date.now() - 90_000 } = {}) {
    if (!service.is_installed || service.kind === 'runtime') return service;
    const packages = SAMPLES[distro]?.[service.id];
    if (!packages) return service;
    const list = packages.map(([name, version]) => ({ package: name, version }));
    const differ = new Set(list.map((item) => item.version)).size > 1;
    const isKept = kept.includes(service.id);
    return {
        ...service,
        ...(differ ? { package_versions_differ: true } : { package_version: list[0].version, package_version_short: shortVersion(list[0].version) }),
        package_versions: list,
        ...(isKept ? { package_version_kept: true } : {}),
        package_version_read_at: instant(isKept ? readAt - 3_600_000 : readAt),
    };
}
