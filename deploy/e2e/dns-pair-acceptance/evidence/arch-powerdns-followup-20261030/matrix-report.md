# Distro config matrix report

| release | PASS | WARN | FAIL | RENDER-FAILED | N/A |
|---|---|---|---|---|---|
| arch | 73 | 1 | 0 | 0 | 4 |
| debian-13 | 82 | 0 | 0 | 0 | 0 |

## Failures and warnings

- **WARN** `arch` nginx.stock-baseline (the release's own nginx.conf before any CelikPanel file) — nginx
  - source: -
  - stock configuration, not CelikPanel's: could not build optimal types_hash, you should increase either types_hash_max_size: 1024 or types_hash_bucket_size: 64; ignoring types_hash_bucket_size

## Not applicable

- `arch` apt.all: not an APT release
- `arch` dovecot.all: the release's Dovecot has no /etc/dovecot/conf.d; CelikPanel refuses to configure it (dovecotConfDirMissingReason)
- `arch` mariadb.server-settings: UpdateMySQLSettings edits the fixed path /etc/mysql/mariadb.conf.d/50-server.cnf, which this release does not have
- `arch` powerdns.legacy-symmetric: Arch runs PowerDNS only from a fresh first install; no legacy symmetric pair exists there
