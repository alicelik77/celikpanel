# Distro config matrix report

| release | PASS | WARN | FAIL | RENDER-FAILED | N/A |
|---|---|---|---|---|---|
| arch | 74 | 1 | 0 | 0 | 2 |
| debian-12 | 79 | 2 | 0 | 0 | 0 |
| debian-13 | 81 | 0 | 0 | 0 | 0 |
| ubuntu-22.04 | 78 | 2 | 0 | 0 | 1 |
| ubuntu-24.04 | 80 | 1 | 0 | 0 | 0 |

## Failures and warnings

- **WARN** `debian-12` systemd.celikpanel-agent (static unit celikpanel-agent.service) — systemd
  - source: deploy/systemd/celikpanel-agent.service
  - /etc/systemd/system/celikpanel-agent.service:74: Unknown key 'RestartSteps' in section [Service], ignoring. ⏎ /etc/systemd/system/celikpanel-agent.service:75: Unknown key 'RestartMaxDelaySec' in section [Service], ignoring.
- **WARN** `debian-12` systemd.celikpanel-panel (static unit celikpanel-panel.service) — systemd
  - source: deploy/systemd/celikpanel-panel.service
  - /etc/systemd/system/celikpanel-panel.service:79: Unknown key 'RestartSteps' in section [Service], ignoring. ⏎ /etc/systemd/system/celikpanel-panel.service:80: Unknown key 'RestartMaxDelaySec' in section [Service], ignoring.
- **WARN** `ubuntu-22.04` systemd.celikpanel-agent (static unit celikpanel-agent.service) — systemd
  - source: deploy/systemd/celikpanel-agent.service
  - /etc/systemd/system/celikpanel-agent.service:74: Unknown key name 'RestartSteps' in section 'Service', ignoring. ⏎ /etc/systemd/system/celikpanel-agent.service:75: Unknown key name 'RestartMaxDelaySec' in section 'Service', ignoring.
- **WARN** `ubuntu-22.04` systemd.celikpanel-panel (static unit celikpanel-panel.service) — systemd
  - source: deploy/systemd/celikpanel-panel.service
  - /etc/systemd/system/celikpanel-panel.service:79: Unknown key name 'RestartSteps' in section 'Service', ignoring. ⏎ /etc/systemd/system/celikpanel-panel.service:80: Unknown key name 'RestartMaxDelaySec' in section 'Service', ignoring.
- **WARN** `ubuntu-24.04` nginx.everything-together (every site shape + panel ACME + webmail socket + webmail origin + dbtools + default HTTPS server (measured nginx 1.24.0: HTTP/2 form "listen")) — nginx
  - source: internal/services/templates/nginx/vhost.conf.tmpl (rendered by internal/services/nginx_generator.go:110 via cmd/agent/vhost_rpc.go:498)
  - protocol options redefined for 0.0.0.0:443 in /etc/nginx/sites-enabled/site12.example.test.conf:36 ⏎ protocol options redefined for [::]:443 in /etc/nginx/sites-enabled/site12.example.test.conf:37
- **WARN** `arch` nginx.stock-baseline (the release's own nginx.conf before any CelikPanel file) — nginx
  - source: -
  - stock configuration, not CelikPanel's: could not build optimal types_hash, you should increase either types_hash_max_size: 1024 or types_hash_bucket_size: 64; ignoring types_hash_bucket_size

## Not applicable

- `ubuntu-22.04` nginx.default-https-server: not written on this release: measured nginx 1.18.0: HTTP/2 form "listen", and ssl_reject_handshake needs nginx 1.19.4
- `arch` apt.all: not an APT release
- `arch` powerdns.all: the PowerDNS engine switch is certified only on APT hosts; pacman hosts are refused
