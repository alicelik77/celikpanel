# D-040 real-system evidence (Debian 13 chroot, 2026-10-30)

One disposable Debian 13 (trixie) chroot built with `mmdebstrap` from Debian's own
packages (`build-chroot.sh`): certbot 4.0.0-2+deb13u1, python3-acme 4.0.0-1,
nginx-light 1.26.3-3+deb13u7, postfix 3.10.13-0+deb13u1, dovecot 2.4.1 (core and
imapd), openssl 3.5.7, and Pebble 2.6.0+ds1-1+b7 as the certificate authority. It
ran inside its own network namespace with only loopback, so no real CA could be
reached (a `--dry-run` without `--server` tried Let's Encrypt staging and failed name
resolution, which is how that was noticed). Pebble required EAB, validated HTTP-01
on port 80, issued one-day certificates and `PEBBLE_AUTHZREUSE=0` made every
renewal validate again. Pebble's own API used a throwaway CA trusted only inside the
chroot. The chroot has no systemd: a stand-in `systemctl` in `/usr/local/sbin`
answers `is-active`/`reload` for nginx, Postfix (`postfix status`/`postfix reload`)
and Dovecot (`pgrep`/`doveadm reload`) and `is-enabled`/`is-active certbot.timer`
from the chroot's own files and processes (every call is in `systemctl.calls`).
Nothing ran on an installed server; the chroot was built on the WSL development
guest `CelikPanel-S2-Debian` under `/var/tmp` and its services were not touched.

The Agent's `realsystem` test binary (SHA-256 `28caf1630c481f55…5cac0ef83`) was built
from branch `fix/site-cert-native-renewal` at `b00885775` (rebased on
`release/alpha113`) and copied into the chroot only for its phases; `run.sh` deletes
it before every renewal and checks that no CelikPanel binary or process rooted in
the chroot exists.

| Step | Result | What it shows |
|---|---|---|
| Prepare | PASS | The product's own vhost validation and renderer (`validatedVhostData`, `NginxGenerator.Render`, with `mail.site.test` as a validation name as the Panel publishes it) and the challenge file `/etc/nginx/celikpanel-managed.d/site.test/acme-http-01.conf`; `nginx -t` passes. |
| Issue (`Agent.IssueLetsEncryptCertificate`, EAB) | PASS | Lineage `celikpanel-site-42-e463ddcd…` for site.test, www.site.test and mail.site.test in `/etc/letsencrypt`; certbot's renewal configuration: `authenticator = webroot`, `[[webroot_map]]` for all three names = the site's challenge root, `server` = the CA; deploy hook root `0755`; the EAB HMAC key in no file under `/etc/letsencrypt` or `/var/log/letsencrypt`; nginx serves exactly the live certificate. |
| Mail | PASS | The Agent's own mail TLS writers and validators (`validateMailSNIEntries`, `preflightMailTLSCommands` → map type `hash`, Dovecot 2.4 dialect, `configurePostfixTLS`, `configureDovecotTLSForDialect`, `postfix check`, `doveconf -n`) and the plan contract (`CanonicalMailTLSSync`) accept an SNI entry naming the lineage's live files; Postfix (STARTTLS on 25) and Dovecot (IMAPS on 993) offer the lineage's certificate for SNI `mail.site.test` (the default certificate is a different, self-signed `mx.site.test`). |
| 1. certbot dry run, no CelikPanel program | PASS | `certbot renew --dry-run --server <CA> --cert-name <lineage>` with the written configuration. |
| 2. The OS renews it, no CelikPanel program | PASS | The chroot's `certbot.service` `ExecStart` (`/usr/bin/certbot -q renew --no-random-sleep-on-renew`) exit 0. The hook reloaded nginx, rebuilt the SNI map, reloaded Postfix and Dovecot (record `nginx=reloaded`, `mail=reloaded`, `dovecot=reloaded`); nginx, Postfix and Dovecot all offer the new certificate (fingerprint changed and equals live); the SNI map source and Dovecot's drop-in are byte-identical to before. `SiteCertificateRenewalStateV1` reads `scheduled`, the newer expiry, `certbot.timer`, hook `installed`. |
| 3. A failed renewal | PASS | Challenge file removed (an owner's change): certbot exits 1 and logs `Failed to renew certificate <lineage> with error: Some challenges have failed.`; the Agent reads `failed` with that reason. |
| 4. Renewed, nginx broken | PASS | An invalid directive in the owner include directory: certbot renews (exit 0; the hook's non-zero exit is only a warning in certbot's log); nginx is not reloaded and still serves the previous certificate, mail takes the renewed one; the Agent reads `deploy_incomplete`. After the fix and a reload, nginx serves the renewed certificate. |

**Not covered here.** A booted system where systemd's `certbot.timer` fires by
itself; Ubuntu 24.04 (snap certbot), Arch, and Dovecot 2.3's `ssl_cert = <` form; a
real CA; the Panel's observer, Dashboard and certificate page against this Agent
(component tests only); a reboot; an upgrade from a release with isolated-store
certificates; the distro config matrix's five releases (its render now includes the
native vhost, the native SNI entry and the site hook, but it was not run for this
change).

Files: `build-chroot.sh`, `run.sh` (the harness; root on a development machine
only), `cp-d040-evidence.tar.gz` — SHA-256
`4bf63785198254f9aa890a07de4138a0b6de4ad27caaa1c226695efec28d6ab9` (run log,
renewal configuration, hook, vhost, challenge file, Postfix SNI map source and
Dovecot drop-in, the three state readings, certbot outputs, Pebble and Postfix logs,
systemctl stand-in calls, package versions, test binary digest).
