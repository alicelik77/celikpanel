# alpha.110 real-system evidence (Debian 13, 2026-10-26)

One disposable Debian 13 virtual machine (QEMU/KVM inside the `archlinux` WSL
guest), an overlay of the kept base image `debian13-base.qcow2`
(SHA-256 `e1c1173b677dcd4b…`, the same image as alpha.108's and alpha.109's runs).
The Agent's and the Panel's `realsystem` test binaries were built from branch
`feat/alpha110-webmail-origin` at `55b2bf22b07e4fe6fcaea1a9132574cd6622a9f4`
(Agent SHA-256 `34b15d5e422f9678…`, Panel `d564a002cfc05736…`) and run with
`CELIKPANEL_REAL_SYSTEM_A110=1`. No real CA was contacted: certbot's issuance was
replaced by a disposable test CA (added to the VM's trust store); everything else
was the production path. The name `mail.a110.test` resolved through `/etc/hosts`.
Nothing ran on an installed server; the overlay was deleted afterwards and the base
image kept.

| Test | Result | What it proves |
|---|---|---|
| `TestRealSystemA110WebmailAddressEnable` | PASS (42 s) | Under the Agent unit's `UMask=0027`, with Debian's PHP-FPM 8.4 and certbot: Roundcube 1.6.20 installed; the pending port-80 server answered certbot's HTTP-01 check through nginx (`HTTP 200`); the address enabled and proved (`proved: true`, `certificate_issued: true`); `certbot.timer` enabled and active; Roundcube's lines added; `http://mail.a110.test/?_task=mail` → `301` to HTTPS; the login page `HTTP 200` with Roundcube's inline start-up script, `Content-Security-Policy: frame-ancestors 'self'; object-src 'none'; base-uri 'self'; form-action 'self'`, HSTS, nosniff, and its session cookie `secure; HttpOnly; SameSite=Strict`; the stylesheet `text/css`; CelikPanel's PHP check asked from the VM's other address `403`. |
| `TestRealSystemA110PanelRedirectsToWebmailAddress` | PASS | The panel's `/webmail/` (behind the panel's own headers) answered `302` to `https://mail.a110.test/`; following it reached Roundcube served by nginx, certificate verified against the VM's trust store, with webmail's own policy (not the panel's). |
| `TestRealSystemA110WebmailAddressRemove` | PASS | Removal deleted the file (the certificate kept); after nginx's previous workers finished (1 s) the name no longer answered; Roundcube's lines taken back; `nginx -t` passes. |

Two earlier runs on the same commit line failed in the test harness, not in the
product path, and were fixed before this run: the stand-in challenge file was
written under the unit's umask (certbot writes it world-readable), and the first run
also showed nginx's previous workers answering port 80 right after the reload, which
led to the product change that waits until this server answers the check before
certbot starts (`CHALLENGE_UNREACHABLE` otherwise).

**Not covered here.** Ubuntu 24.04; a real CA (Let's Encrypt) and a renewal; a name
already used by another server block (component tests); the panel's database and
operation flow (component tests). Nothing measured on an installed server.

Archive:

- `cp-a110-evidence.tar.gz` — SHA-256 `cf48534276ca863de7b3f57f7676af9a6d6fa9fe44d220460930c561888497b6`
  (run log, VM log, build log, serial log, commit, binary and base image digests)
