# alpha.109 real-system evidence (Debian 13, 2026-10-26)

One disposable Debian 13 virtual machine (QEMU/KVM inside the `archlinux` WSL
guest), an overlay of the kept base image `debian13-base.qcow2`
(SHA-256 `e1c1173b677dcd4b…`, the same image as alpha.108's run: nginx,
unattended-upgrades, the owner's own Sury source, no PHP). The Agent's `realsystem`
test binary was built from branch `fix/alpha109-webmail-update` at
`3247835f9debea5548b423f4a5598af166a1c17a` (binary SHA-256
`e5e2f0c96040ce8b80447f1e5dc8f0eaac42f2df1d3f924f0757fe10b0b0ff75`) and run with
`CELIKPANEL_REAL_SYSTEM_A109=1`. Nothing ran on an installed server; the overlay was
deleted afterwards and the base image kept.

| Test | Result | What it proves |
|---|---|---|
| `TestRealSystemA109WebmailUpdateFromAnOlderConfiguration` | PASS (38 s) | Under the Agent unit's `UMask=0027`, with Debian's PHP-FPM 8.4: Roundcube 1.6.15 installed and configured; the webmail configuration an older release wrote put back, and **Frankfurt's failure reproduced word for word** (`PHP-FPM 8.4 could not open CelikPanel's PHP check for Webmail (Roundcube) (HTTP 404: 404 Not Found)`). Through webmail's socket the login page starts Roundcube with three inline scripts (`new rcube_webmail()`, which the panel's old `script-src 'self'` refused) and its stylesheet answers HTTP 200 as `text/css` (the type was never the cause). An update to 1.6.20 whose first proof was made to fail: the check location was added first (the file is today's form with the same sockets), the previous 1.6.15 tree came back, answered (`previous_answers: true`) and its stylesheet stayed readable by the web server user. The real update to 1.6.20 then proved through the check, the login page and its stylesheet (`updated: true, proved: true`); two backups kept, mode 0600 in a 0700 directory. |

**Not covered here.** Ubuntu 24.04; an owner-edited webmail configuration (component
tests); the panel's proxy headers in a browser (component test of the proxy and the
browser look on a mock); a Panel restart during an update (component test of the
read-only reconciliation). Nothing measured on an installed server.

Archive:

- `cp-a109-evidence.tar.gz` — SHA-256 `e77d1ab443d8f28b1af518a2c526865928612e37df718e53babb66547960f7ee`
  (run log, VM log, build log, serial log, commit, binary and base image digests)
