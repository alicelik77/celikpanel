# D-039 real-system evidence (Debian 13 chroot, 2026-10-30)

One disposable Debian 13 (trixie) chroot built with `mmdebstrap` from Debian's
own packages (`build-chroot.sh`): certbot 4.0.0-2+deb13u1, python3-acme 4.0.0-1,
nginx-light 1.26.3-3+deb13u7, openssl 3.5.7, and Pebble 2.6.0+ds1-1+b7 as the
certificate authority. It ran inside its own network namespace with only
loopback, so no real CA could be reached (a `--dry-run` without `--server` tried
Let's Encrypt staging and failed name resolution, which is how that was noticed).
Pebble required EAB (external account binding), validated HTTP-01 on port 80 and
issued one-day certificates; `PEBBLE_AUTHZREUSE=0` so every renewal validates
again. Pebble's own API used a throwaway CA trusted only inside the chroot. The
chroot has no systemd: a stand-in `systemctl` in `/usr/local/sbin` answers the
hook's `is-active`/`reload nginx` and the scheduler reader's
`is-enabled`/`is-active certbot.timer` from the chroot's own files and processes
(every call is in `systemctl.calls`). Nothing ran on an installed server; the
chroot was built on the WSL development guest `CelikPanel-S2-Debian` under
`/var/tmp` and its services were not touched.

The Agent's `realsystem` test binary (SHA-256 `1cda26d50af3ea7d…12291ac8`) was
built from branch `fix/site-cert-native-renewal` at `cdfeb6263` and copied into the
chroot only for its phases; `run.sh` deletes it before every
renewal and checks that no CelikPanel binary or process rooted in the chroot
exists.

| Step | Result | What it shows |
|---|---|---|
| Prepare | PASS | The product's own vhost validation and renderer (`validatedVhostData`, `NginxGenerator.Render`) and the Panel-owned challenge file `/etc/nginx/celikpanel-managed.d/site.test/acme-http-01.conf`; `nginx -t` passes. |
| Issue (`Agent.IssueLetsEncryptCertificate`, EAB) | PASS | Lineage `celikpanel-site-42-dfca0be9…` in `/etc/letsencrypt` (files `root:root`, key `0600`); certbot's own renewal configuration says `authenticator = webroot`, `[[webroot_map]]` for both names = the site's challenge root, `server` = the CA; the deploy hook is root `0755`; the EAB HMAC key is in no file under `/etc/letsencrypt` or `/var/log/letsencrypt`; nginx re-rendered with certbot's live paths serves exactly the live certificate. |
| 1. certbot dry run, no CelikPanel program | PASS | `certbot renew --dry-run --server <CA> --cert-name <lineage>` with the written configuration. |
| 2. The OS renews it, no CelikPanel program | PASS | The chroot's `certbot.service` `ExecStart` (`/usr/bin/certbot -q renew --no-random-sleep-on-renew`) exit 0; the hook ran `nginx -t` and reloaded nginx (record `nginx=reloaded`); nginx serves the new certificate (fingerprint changed and equals live). `SiteCertificateRenewalStateV1` reads `scheduled`, the newer expiry, `certbot.timer`, hook `installed`. |
| 3. A failed renewal | PASS | Challenge file removed (an owner's change): certbot exits 1 and logs `Failed to renew certificate <lineage> with error: Some challenges have failed.`; the Agent reads `failed` with that reason. |
| 4. Renewed, nginx broken | PASS | An invalid directive in the owner include directory: certbot renews (exit 0; it logs the hook's non-zero exit only as a warning), the hook does not reload nginx (record `nginx=config_test_failed`), nginx still serves the previous certificate while live/ has the new one; the Agent reads `deploy_incomplete`. After the fix and a reload, the renewed certificate is served. |

**Not covered here.** A booted system where systemd's `certbot.timer` fires by
itself; Ubuntu 24.04 (snap certbot) and Arch; a real CA (Let's Encrypt, ZeroSSL,
Google); the Panel's observer, Dashboard and certificate page against this Agent
(component tests only); mail SNI of a site certificate with the Panel absent (open,
see D-039); a reboot; an upgrade from a release with isolated-store certificates.

Files: `build-chroot.sh`, `run.sh` (the harness; root on a development machine
only), `cp-d039-evidence.tar.gz` — SHA-256
`e80a8041895671b5aa675837d70a48f4fe5b5433fd6a58cc30d78ade349655f0` (run log,
renewal configuration, hook, vhost, challenge file, the three state readings,
certbot outputs, Pebble log, systemctl stand-in calls, package versions, test
binary digest).
