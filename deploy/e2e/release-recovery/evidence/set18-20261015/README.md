# set18, 2026-10-15 (UTC): the alpha.87 candidate (HEAD 5d54aba1e) installed fresh on Debian 13 in a lab with its own DNS and its own ACME server, the owner's guided setup run to READY in a real headless Chrome, the Components mail route, and an extension adding Core Mail followed to READY (through the reverse DNS check and "Finishing setup")

A measurement record, the twenty-first native record. Host `date -u` at the start of the work: `Thu Oct 15 22:24:14 UTC
2026` (reported by the run coordinator; not recorded in this folder; the folder's earliest reading is 22:38:15Z, `host/c-drive-watch.txt`); the run
directory was made at 22:38:28Z and the last lab ended at 23:27:49Z (`host/progress.txt`). Page text and pictures show
the host's local time (UTC+3); times here are UTC.

The harness addressed no installed server, no licence service and no certificate authority, and did not contact
celikpanel.net. The release origin's name and the certificate-authority names were pinned to the guest's loopback as the
first driver step and read back at its end (`*/driver/steps/01-set6-name-pinning`, `*-name-pinning-at-the-end`: passed
in both labs). **No ACME request left the guest**: every certificate came from the lab's own pebble inside the guest;
outbound TCP/UDP 443 from the guest was refused except to loopback, the lab networks and the package hosts, and the
only refused 443 connection in the whole run is the lab's own self-test to 203.0.113.1 (`host/native-after-c2.txt`,
"443 block log" and the reject counter `packets 1`; the same at the lab's last reading, 23:27:45Z,
`browser/handover-setup/prep-done-lab-state-1792106865548.json`). Every archive is a test-licence (`acceptance_license`) build with
`release: false`; none is a release. Every `result.json` carries `native_evidence: false`; this record closes no P0 row.
No product file was changed. This is a measurement, not acceptance evidence: QEMU/KVM guests, test-licence builds, a
lab-only DNS and ACME fixture, nothing judged.

**Outcome in one line (observed in this lab; not acceptance evidence).** With a lab DNS and a lab ACME server, a fresh candidate reached "Your server is ready" through every setup step, for the first time in this evidence folder (no earlier README here records "Your server is ready"; the earlier READMEs that describe a setup say it waited at `access_dns` or was set up by hand), including "Verify public
DNS before certificate issuance" and "Secure panel access" (a trusted certificate from the lab CA and the planned Panel
restart), in 115 s; the extension adding Core Mail ran natively through the mail host's public-DNS wait, the mail
profile, the mail certificate, the mail renewal enrollment, the reverse DNS check with the resolver named, **"Finishing
setup"** and back to ready. **One defect blocks the extension in the browser (D1):** while a completed setup is being
extended, the wizard shows the *completed* first run ("Verifying the result", "Completion could not be verified…",
"Reconnect to this operation") instead of the extension's steps, on both routes (Components → "Review in server setup"
and the ready screen's "Extend setup"), so the access step, the extension review ("What changes / To be installed /
Already installed") and "Stop extending" are not reachable for the owner. The blocked button presses were made by the
page's own session with the wizard's own requests (stand-ins, below); the run itself was then followed in the page.

## The lab fixture (lab preparation, not a product change; `deploy/e2e/release-recovery/guest_set18_lab_dns.py`)

Written by the driver step `set18-lab-network` before CelikPanel was installed (so the Agent and the Panel load the lab
CA with the system trust); its full record, every command and the rule text verbatim are in
`debian13/fresh-setup-b/driver/steps/04-set18-lab-network/set18-lab-network.json`.

* **Server address.** The Panel reads this server's address from the route to 192.0.2.1: `192.0.2.10` (the guest's
  second NIC, TEST-NET-1; set17's raw browser files contain the same address). The Agent reads it from the route to 1.1.1.1. The lab adds
  `ip route replace <resolver>/32 via 10.0.2.2 dev mgmt0 src 192.0.2.10` for 1.1.1.1, 8.8.8.8, 9.9.9.9 and
  208.67.222.222, so both read the same address (`route_get_1_1_1_1`: `1.1.1.1 via 10.0.2.2 dev mgmt0 src 192.0.2.10`),
  as on a server with one public address. 192.0.2.10 is "public" to the product (`IsPrivate` false).
* **Lab DNS.** A small responder (the helper's `serve` mode, transient unit `set18-lab-resolver.service`) on 127.0.0.1,
  TCP and UDP, one port per resolver it stands in for (5300 pebble, 5301 1.1.1.1, 5302 8.8.8.8, 5303 9.9.9.9, 5304
  208.67.222.222). It answers the zone `set18.test` and `2.0.192.in-addr.arpa` from a record table, NXDOMAIN for unknown
  names inside them and REFUSED for every other name, and logs every question. The public resolvers hardcoded in the
  product were found by a search of `cmd/` and `internal/`: `cmd/panel/setup_dns_nameservers.go` (1.1.1.1, 8.8.8.8: the
  setup's public-DNS checks), `cmd/panel/nameservers.go` (1.1.1.1, 8.8.8.8), `cmd/panel/domain_connection.go` (1.1.1.1,
  8.8.8.8, 9.9.9.9, 208.67.222.222), `cmd/agent/mail_health_dns.go` (1.1.1.1, 8.8.8.8: reverse DNS, TCP only). No DoH
  endpoint is used. Initial table: `panel.set18.test A 192.0.2.10` (prepared before setup, as the wizard asks),
  `host-192-0-2-10.provider.set18.test A 192.0.2.10`, `PTR 192.0.2.10 → host-192-0-2-10.provider.set18.test` (the
  provider's default name); `mail.set18.test` absent. Two later changes stand for the owner's actions at the DNS provider
  and the server provider (`record-changes` in the lab state; preparations `lab-mail-a`, `lab-ptr-mail`).
* **nftables** (table `ip set18lab`, and `ip6 set18lab6` for the IPv6 443 filter), verbatim from the record:

```
table ip set18lab {
    set allow443 { type ipv4_addr; }
    chain out_nat {
        type nat hook output priority -100; policy accept;
        ip daddr 1.1.1.1 udp dport 53 counter dnat to 127.0.0.1:5301
        ip daddr 1.1.1.1 tcp dport 53 counter dnat to 127.0.0.1:5301
        ip daddr 8.8.8.8 udp dport 53 counter dnat to 127.0.0.1:5302
        ip daddr 8.8.8.8 tcp dport 53 counter dnat to 127.0.0.1:5302
        ip daddr 9.9.9.9 udp dport 53 counter dnat to 127.0.0.1:5303
        ip daddr 9.9.9.9 tcp dport 53 counter dnat to 127.0.0.1:5303
        ip daddr 208.67.222.222 udp dport 53 counter dnat to 127.0.0.1:5304
        ip daddr 208.67.222.222 tcp dport 53 counter dnat to 127.0.0.1:5304
        tcp dport 25 ip daddr != { 127.0.0.0/8, 192.0.2.10, 10.0.2.0/24 } counter dnat to 127.0.0.1:25
    }
    chain out_filter {
        type filter hook output priority 0; policy accept;
        tcp dport 443 ip daddr { 127.0.0.0/8, 192.0.2.0/24, 10.0.2.0/24 } counter accept
        tcp dport 443 ip daddr @allow443 counter accept
        tcp dport 443 counter log prefix "set18-lab-443-block " reject with tcp reset
        udp dport 443 counter reject
    }
}
table ip6 set18lab6 {
    set allow443 { type ipv6_addr; }
    chain out_filter {
        type filter hook output priority 0; policy accept;
        tcp dport 443 ip6 daddr ::1 counter accept
        tcp dport 443 ip6 daddr @allow443 counter accept
        tcp dport 443 counter log prefix "set18-lab-443-block6 " reject with tcp reset
        udp dport 443 counter reject
    }
}
```

  `allow443` holds the resolved addresses of the apt package hosts (`deb.debian.org`, `security.debian.org`,
  `packages.sury.org`, plus any `https://` host in the apt sources; refreshed every 10 s by the guard unit
  `set18-lab-guard.service`, which also re-applies a missing table; it never had to). Port 25: the product's
  outbound-25 check (`cmd/agent/mail_health_rpc.go`, a TCP connect to gmail-smtp-in.l.google.com:25) is answered by the
  guest's own Postfix (counter 148 packets at the 23:25:16Z reading, 152 at 23:27:45Z), so **"Outgoing mail (port 25)" passing is a property of the lab**, not a
  measurement of any network; no SMTP left the guest. Self-checks at preparation: the panel name answered 8 of 8 times
  through the four redirected resolvers (UDP and TCP), the PTR through 1.1.1.1/TCP, `mail.set18.test` NXDOMAIN,
  `example.com` REFUSED; `bash /dev/tcp/203.0.113.1/443` → "Connection refused", logged by the reject rule.
* **Packages installed by the lab** from Debian's repository before CelikPanel: `nftables` (absent from the Debian 13
  cloud image; needed for the lab table; `nftables.service` stayed disabled and inactive) and `pebble 2.6.0+ds1-1+b7`.
  Consequence: setup's own nftables component read as already installed and no "Prepare component: nftables" step
  appears in the plan (the plan's component list: `nftables required, installed`).
* **ACME (pebble; possible by configuration, no product change).** The product runs `certbot certonly … --webroot`
  without `--server` (`cmd/agent/panel_cert_rpc.go`, `mail_host_certificate_rpc.go`; `internal/core/acme_providers.go`:
  Let's Encrypt's directory is certbot's default), so certbot's own user configuration decides the directory:
  `/root/.config/letsencrypt/cli.ini` = `server = https://127.0.0.1:14000/dir`, `no-verify-ssl = true` (written by the
  lab). pebble ran as `set18-lab-pebble.service` (`/usr/bin/pebble -config …/pebble-config.json -dnsserver
  127.0.0.1:5300`, `PEBBLE_VA_NOSLEEP=1`, `PEBBLE_WFE_NONCEREJECT=0`; listen 127.0.0.1:14000, management 127.0.0.1:15000,
  HTTP-01 on port 80 of the guest, its own listener certificate). Its root (`CN=Pebble Root CA 3a4c96`, SHA-256
  `55:22:C5:…:F6:F9`) was installed as `/usr/local/share/ca-certificates/set18-lab-pebble-root.crt` +
  `update-ca-certificates`, because the Agent verifies the panel and mail certificates against the system roots
  (`cmd/agent/panel_cert_source_linux.go`, `internal/mailhostartifact/certificate.go`) and the Panel's readiness dials
  its own listener with system verification. Both renewal lineages record `server = https://127.0.0.1:14000/dir`.
  pebble's log shows two orders, both validated over HTTP-01 against this guest: `panel.set18.test` (22:57:14–16Z,
  serial 141aeaedc88ec857) and `mail.set18.test` (23:23:18–19Z, serial 777b8a714b5256be).

## Source and build (`build/`)

Candidate: HEAD `5d54aba1e9c93357fa0c19d3b0487e27384acd01` of `feat/alpha87-second-server`, built from `git clone`,
never from the working tree; its own release policy still reads v0.1.0-alpha.86 / 86, so the installed archive is a
policy-only fixture over it: `c84ebee3…` (policy `v0.1.0-alpha.87 / 87`, previous 86 = the v0.1.0-alpha.86 tag commit
8d260f317e58bed7fcd84c6cbb01823c4fe3f166). No migration between that tag and HEAD (`build/source-check.txt`). The origin's
candidate (`1a1f50a9…`, v0.1.0-alpha.88, policy only) and the defective archive (`149378b2…`) were built only because the
artifacts document needs them, and never installed (set18 starts no update). Go 1.26.5, job 22:39:00Z–22:42:37Z,
`build-set18-artifacts.sh` from run copy b.

| Role | Version / seq | Commit | Archive SHA-256 |
| --- | --- | --- | --- |
| baseline (installed) | v0.1.0-alpha.87 / 87 | c84ebee347202ae3d647726c9bf096662dcf621e | `708e2030d5c46b9074c94a1ced72cc122d96beb5ba77419e8f9df64e6e87b0a6` |
| good (never installed) | v0.1.0-alpha.88 / 88 | 1a1f50a956498e30a5810492331245a2e3e5c496 | `6bafc5b1ea7c02d74721c267c944385c6b3d56af2401497e080760398c7efd57` |
| defective (never installed) | v0.1.0-alpha.88 / 88 | 149378b2a5dedf5dfc9a6300a6a3723e4cb42b2f | `55409d108be96458df122165cf5c9f7af50c310b0175698b7afbc0397d3382b2` |

Proof (`set18_fixture.py prove`, rc 0), dry runs from copies c and d (rc 0), offline checks (`harness-run-copy/*/suite.txt`).

## Labs

| Lab | UTC | Outcome | Raw |
| --- | --- | --- | --- |
| s18-a | 22:50:14Z–22:51:55Z | `set18-lab-network` inconclusive: `nft` absent in the cloud image (H1); the lab was stopped before any install | `debian13/fresh-a-lab-network-failed` |
| **s18-b** | 22:52:52Z–23:27:49Z | lab network passed 22:54:15–28Z; install 22:54:28–55:02Z; owner, licence passed; browser window 22:55:03–23:27:47Z `observed`; `set18-readings` inconclusive (H2); overall `inconclusive` | `debian13/fresh-setup-b` |

## Cells (lab s18-b; browser windows: `browser/setup` = run 1 of the tool, `browser/resume` = run 2)

### 1. Fresh install, owner setup with purpose web, to ready (EN, 1280×800)

Owner: sign-in, "Set up with the wizard", purpose Web hosting, panel address `panel.set18.test`, DNS "External DNS
provider", "Review setup plan" (`setup/c1-05-review.txt`: the handover notice "The Panel restarts once during this
setup", 10 steps, TCP 22, 80, 443, 2083, contact `labadmin@example.invalid`), confirm, "Start reviewed setup"
(22:55:26.56Z). States (`setup/c1-samples.jsonl`, 11 pictures `c1-state-*`):

| UTC | Page | Run |
| --- | --- | --- |
| 22:55:26–27 | "Preparing your server" | queued; first guidance block (O1) |
| 22:55:30–57:03 | "Preparing your server" | 02–06 components (Nginx, PHP-FPM, MariaDB, cron, certbot) |
| 22:57:12 | same | 07 firewall |
| (≤ 3 s) | – | **08 "Verify public DNS before certificate issuance: panel.set18.test / 192.0.2.10" succeeded between the 22:57:12 and 22:57:15 samples (never sampled as its own state; "at its first check" is not recorded in this folder)** |
| 22:57:15 | same | **09 "Secure panel access: panel.set18.test"** running (pebble order 22:57:14–16Z) |
| 22:57:18 | **"The Panel restarts at this step"**, "This page lost its connection while setup was securing panel access…" (the planned handover); the picture `setup/c1-state-10-viewport.png` also shows a banner above the header, "The latest access check failed. Retrying automatically; the current page is preserved while the last verified access remains valid.", with "Check again" and "Reload page" (not in the page-text samples) | 09 running |
| 22:57:21 | **"Your server is ready"** (Web hosting; "Add your first website"; the "Extend this server's setup" section) | `succeeded / complete` |

No waiting state was sampled at the final check, so **"Finishing setup" did not appear in cell 1**; in cell 2 it appeared 1.3 s after the owner's second "Check requirements again" while the run waited at the final check (whether it can appear without that press is not shown). Run 115 s.
The Panel's certificate after the run: `DNS:panel.set18.test`, issuer `CN=Pebble Intermediate CA 7c74ac`, valid to
2031-10-15 (`setup/leaf.jsonl`, `host/native-after-c1.txt`); certbot.timer enabled and active; renewal lineage
`celikpanel-panel-8fd6660f…` with `authenticator = webroot`.

### 3. The Components mail route (same server, before the extension)

Components → "One-click mail stacks" → Core Mail → "Install": the dialog shows the setup route note ("On this server
mail is added through server setup, so it gets the same steps and checks as setup: the public DNS record of the mail
host name, a trusted certificate and its renewal, reverse DNS (PTR) and outgoing port 25. You review the changes there
before anything starts.") and the button "Review in server setup" (TR "Sunucu kurulumunda incele";
`setup/c3-02-dialog*`, `v-mail-dialog-*`). Mail host name typed `mail.set18.test`; "Review in server setup" pressed
23:04:00.155Z → `POST /api/v1/setup/extend {"revision":3,"action":"begin","mail_profile":"core-mail","mail_hostname":"mail.set18.test"}`
→ 200, `status: draft, extension: true, revision 4`, the draft carrying postfix + dovecot and the mail host name; the
page went to `/setup`.

**It did not land on the access step (D1).** `/setup` showed the first run's progress (`setup/c3-04-access-step.txt`):
"Extend this server's setup" / "Verifying the result", the completed run's first nine steps "Completed", "Verify the prepared server — In progress", and the alert "Completion could not be verified. Reconnect to obtain fresh evidence;
the server has not been declared ready.", buttons "Continue at the secure panel address", "Reconnect to this
operation"; it read `GET /api/v1/setup/operation` (→ the completed run 38545bfb…) and `GET /api/v1/setup?check=1` every
3 s. No access step, no review, no "Stop extending" (`[data-setup-extend-stop]` absent). The same screen in EN/TR at
1280 and 390 (`v-ext-access-*`). A new browser (run 2, no stored marker, 23:12:38Z) saw the same
(`resume/r1-extension-shows-completed-run`), so it does not come from a marker in the first browser.

"Stop extending" made by the page's session as a stand-in (`POST /api/v1/setup/extend {"revision":4,"action":"cancel"}`
→ 200 `status: ready`, revision 5, the completed draft restored: no mail host name, no customization); `/setup` then
showed "Your server is ready" again (`resume/r2-ready-again`); the latest operation was still the first run
(38545bfb…), nothing was started or installed (Postfix and Dovecot not installed, `resume/events.jsonl`
"read components after stopping").

### 2. The extension from the ready screen, adding Core Mail

"Extend setup" pressed on the ready screen (23:12:44Z, `POST /api/v1/setup/extend begin` by the page itself → revision
6): **the same D1 screen** (`resume/r3-after-extend-setup`; no purpose step, no "Customize selection"). The wizard steps
the owner could not reach were made by the page's session with the wizard's own requests, each recorded as a stand-in
(`resume/events.jsonl` `stand-in-request`): Core Mail and the mail host name (`POST /api/v1/setup/extend begin
core-mail mail.set18.test` → revision 7, components dovecot, mariadb, nginx, php-fpm, postfix), the review
(`POST /api/v1/setup/plan` → `resume/r3-plan.json`) and the start (`POST /api/v1/setup/start`, 23:12:53.85Z → 202, run
49fc52bb…, with the wizard's own start marker). **The review blocks could not be seen on screen**; the plan they would
render: what changes — the component selection (purpose `web`, now customised) and mail host name `mail.set18.test`;
to be installed — "Prepare mail profile: Core Mail" (postfix, dovecot: `installed false`); kept — mariadb, nginx,
php-fpm (selected, installed), nftables, certbot, cron (required, installed); steps `01-dns external`,
`02-mail_profile core-mail`, `03-firewall`, `04-access_dns panel.set18.test 192.0.2.10`, `05-access_dns
mail.set18.test 192.0.2.10`, `06-mail_certificate mail.set18.test`, `07-mail_enrollment mail-renewal`, `08-verify`;
TCP 22, 25, 80, 110, 143, 443, 465, 587, 993, 995, 2083; `hostname_change` none.

The run, followed in the page (`resume/c2-samples.jsonl`, pictures `c2-state-*`):

| UTC | Page | Run / owner |
| --- | --- | --- |
| 23:13:10 | "Preparing your server", "Prepare mail profile: Core Mail" | 02 running |
| 23:13:58–23:23:09 | **"Setup is waiting for a requirement"**, "Verify public DNS before certificate issuance: mail.set18.test": "Certificate issuance requires this public A record: mail.set18.test → 192.0.2.10. The public record has not been verified yet. This result alone does not establish whether the record is absent or a DNS server is unreachable. Create or correct this record at the DNS provider or primary that manages it. … This prerequisite is checked automatically. After it passes, the same setup continues. …" with "Edit plan" | waiting `access_dns`, `server_setup_access_dns_required`, re-checked every ~20 s (NXDOMAIN at both resolvers, resolver log); the picture `resume/c2-state-02-viewport.png` also shows a warning toast, "core-mail installed with a self-signed fallback certificate. Mail clients will show a trust warning until a matching trusted certificate is configured." (not in the page-text samples) |
| 23:23:09 | – | the owner adds the A record at the provider (`lab-mail-a`; asked by the coordinator, H3) |
| 23:23:19 | "Preparing your server" | 05 passed at the next check; 06 mail certificate (pebble 23:23:18–19Z) |
| 23:23:28–35 | "The mail renewal result cannot yet be verified. Checks continue for the same request…" with "Continue recorded operation" | 07 mail enrollment (`server_setup_mail_enrollment_handoff`, then `_running`); the tool pressed the button (O3) |
| 23:23:38–24:30 | **"Setup is waiting for a requirement"**, the open check: **"The public resolver 1.1.1.1 reports host-192-0-2-10.provider.set18.test as the reverse DNS (PTR) name of this server's address 192.0.2.10; mail servers expect mail.set18.test. You set this at your server provider, not in CelikPanel: in the provider's control panel, or by asking its support to set the reverse DNS of 192.0.2.10 to mail.set18.test."** | waiting `verification`, `mail_identity action_required`; the owner pressed "Check requirements again" at 23:24:23.95Z (same sentence in the result box) |
| 23:24:30 | – | the owner sets the reverse DNS at the provider (`lab-ptr-mail`), then "Check requirements again" 23:24:33.61Z |
| **23:24:34.87** | **"Finishing setup"**: "Every check passed. The panel is completing setup now, and you do not need to do anything. This page updates by itself when it is done." (spinner; the verify row "finishing") | waiting `verification`, all checks passed |
| 23:24:37 | "Verifying the result" | `succeeded / complete` |
| 23:24:39 | **"Your server is ready"** | – |

Followed for 689 s (the tool's samples, 23:13:10Z to ready at 23:24:39Z; the start request was answered at 23:12:53.85Z, 705 s before ready), of it 9 min 11 s waiting for the mail record (first seen 23:13:58Z) and 56 s at the reverse DNS check. The resolver named in the
PTR sentence is the address the Agent dialled, 1.1.1.1 (DNAT-ed to the lab responder's port 5301); the responders'
"disagree" variant was not produced (both answered the same). After the run (`host/native-after-c2.txt`): Postfix
`myhostname = mail.set18.test`, `smtpd_tls_security_level = may`, the certificate on 587 `DNS:mail.set18.test` (993: the same issuer and validity; its SAN line is not recorded)
from the lab CA, lineage `celikpanel-mail-d1f1f42a…` (`server = https://127.0.0.1:14000/dir`), postfix and dovecot
active, the system hostname kept (`dns-debian13`).

### 4. The secondary-DNS guidance box: not reached

The Components box ("This server is the secondary DNS server…", `mail_dns_guidance`) needs an active DNS engine in the
secondary role of a pair (`cmd/panel/mail_dns_identity.go`, `mailProfileDNSGuidanceFor`), i.e. a completed local-DNS
secondary setup with a primary; not cheap in this lab and not attempted. The setup access step's own secondary box
(`setup.infrastructure.secondaryHelp`) was planned as a UI-only capture inside the extension's access step, which D1
made unreachable (`setup/c4-secondary-box*` shows the D1 screen, not the box).

### 5. Ledger and update cells: not part of this record (no update was started).

## Browser checks (EN and TR at 1280×800 and 390×844; `variants.jsonl`, `v-*` pictures, all `overflow_x: false`, `lang` matching)

| Screen | Result |
| --- | --- |
| Ready screen with "Extend this server's setup" (after cell 1, `setup/v-ready-*`; after cell 2, `resume/v-ready-after-extension-*`) | four variants each, no horizontal overflow, `lang` en / tr |
| Components → Core Mail dialog, setup route (`setup/v-mail-dialog-*`) | four variants; TR note and "Sunucu kurulumunda incele" present |
| Extension access step (`setup/v-ext-access-*`) and extension review | **not reachable (D1)**; the four `v-ext-access-*` pictures show the D1 screen |
| Components after the extension (`resume/c2-components-after`) | EN 1280 only |

Console and requests: `*/console.jsonl`, `*/requests.jsonl` (bodies of the `/api/v1/setup*` answers that completed; 21 requests recorded without a status, 12 in `setup` and 9 in `resume`, have none).

## Defects, observations, harness notes

**D1 (defect, verified in this lab, both routes, two browsers): an extension of a completed setup opens on the completed
first run, so the owner cannot review, start or stop the extension.** Reproduce: complete setup; press "Extend setup"
on the ready screen, or Components → a mail profile → "Review in server setup". `/setup` shows "Verifying the result",
the old run's steps, "Completion could not be verified. Reconnect to obtain fresh evidence; the server has not been
declared ready." and polls every 3 s; no access step, no review, no "Stop extending". Mechanism as read in the source
(not changed): `GET /api/v1/setup/operation` without `request_id` answers the latest execution
(`cmd/panel/server_setup_operations.go`, `latestServerSetupExecution`), which is the succeeded first run;
`web/src/components/ServerSetup.tsx` `reconcile` then sets the progress step, and for a succeeded run reloads the setup
state, sees `status: draft` (the extension) and sets `completionFailed`; "Stop extending" is drawn only when
`!hasOperation`. Classification: verified failure of the alpha.87 candidate's extension flow in the browser (the server
side of extend/plan/start/cancel answered correctly to the same requests). Also a false alarm: "has not been declared
ready" is shown on a server that is ready and whose status the owner just saw as ready.

**O1 (observation):** for about three seconds after "Start reviewed setup" (22:55:27.6Z until the next recorded change at 22:55:30.6Z, before the first operation read), the guidance reads "No more specific requirement has been confirmed for this step. … waiting alone may not resolve a missing
requirement." (`setup/c1-state-01/02`); the next recorded state replaced it.

**O2 (observation):** after the certificate handover the Panel serves the lab CA certificate for SNI `panel.set18.test` (`host/native-after-c*.txt`; SNI `localhost` gets it too per set18b's `leaf.jsonl` and H2 below, set18's own `leaf.jsonl` does not record the SNI it used) but its original self-signed certificate (`O=CelikPanel, CN=dns-debian13`) to a client that sends
no SNI (`host/native-after-c*.txt`; an IP address in the URL is the reading's premise, not recorded). Not judged.

**O3 (observation):** during the mail enrollment step the page offered "Continue recorded operation" for about 4 s while
the enrollment was already running; the tool pressed it on each of four samples (its rule for that button); three POSTs
reached the Panel and each answered the same request id (`1ef445f7…`: `handoff unknown` twice, then `accepted` with reason `mail_enrollment_already_terminal`); no second enrollment started. The button stayed enabled between presses.

**O4 (observation, lab artifact):** the handover notice offers "https://panel.set18.test:19319" — the port of the lab's
loopback forward the page was opened on, not the Panel's 2083 (`lib/panelHandover.ts handoverAddress` uses the current
page's port before the run has a server URL).

**H1 (harness):** lab s18-a: the lab helper called `nft`, absent from the cloud image; fixed by installing `nftables`
with pebble first (copy d), lab s18-b.

**H2 (harness):** the driver's pinned API client connects to `127.0.0.1` without SNI and reads the guest's leaf with SNI
`localhost` (O2), so after the handover every driver API read failed (`PinMismatch`): the `setup-read` preparations
and `set18-readings` are inconclusive. The coordinator took two read-only native readings over the lab's SSH key while
the lab ran (`host/native-after-c1.txt` 23:00:35Z, `host/native-after-c2.txt` 23:25:16Z; script `tools/native.sh`); the
setup state at the end comes from the page (`resume/resume-summary.json c2_state`: `ready`, no open check).

**H3 (harness):** the tool's trigger for the mail record compared the run's phase with the step id, but a waiting run
reports the kind (`access_dns`), so the tool never asked; after 9 minutes of recorded waiting the coordinator wrote the
same preparation request into the handover directory (23:23:08Z, `host/progress.txt`, `tools/askmail.sh`). The
committed tool is the text that ran (not corrected).

**H4 (harness):** the browser tool's first run (`browser/setup`) was stopped by the coordinator after D1 (the tool's run ended `rc=127` at 23:10:45Z, `browser/node-run-setup.txt`; the coordinator's note is stamped 23:10:56Z, `tools/run-notes.txt`) (before it wrote `done.json`, so the driver's window and the lab stayed open); the cell `resume` was added to the tool
and run 2 (`browser/resume`, tool SHA-256 `07283c83…`; run 1 `9faaa131…`) continued on the same window
(`tools/run-notes.txt`). The lab helper's `at` stamps put the fraction after the `Z` (cosmetic).

## Open items

- D1 blocks the owner's extension in the browser (both routes); the extension review blocks were never seen rendered.
- The Components secondary-DNS box and the setup access step's secondary box: not reached.
- The resolvers-disagree PTR sentence; "Finishing setup" slow variant (`setup.finishingSlow`); a run whose final check
  fails after "Finishing setup": not produced.
- O2 (no-SNI certificate) and O3 (enabled continue button) not judged.
- Ubuntu 24.04, Arch; a public, non-lab ACME/DNS path: not run.

## Harness changes in this run (committed with this record)

New: `set18_fixture.py`, `build-set18-artifacts.sh`, `set18_trial.py`, `run-set18.sh`, `guest_set18_lab_dns.py` (lab
preparation), `web/tools/browser-inspect/cold-load-set18.mjs` (cells `setup`, `resume`, `done`). Run copies
(`harness-run-copy/`): b (build), c (lab s18-a), d (lab s18-b); each with the overlay's hashes and diff against 5d54aba1e.

## Secrets, host, leftovers

Redacted at collection by the harness (owner password, session, fixture keys); set6's digest sweep over each staged lab
changed 0 files; driver `SHA256SUMS` verified on the staged copies. `tools/scrub.py` replaced host paths with
`<redacted>` (`host/scrub.txt`). C: (Get-PSDrive) 353.50 GiB free at 22:38:15Z, lowest 352.24 GiB at 22:48:16Z, 352.90
GiB at 23:28:18Z (`host/c-drive-watch.txt`); WSL `/` 941 GB free at 22:38Z (`host/hostcheck-before.txt`). The labs, the
run directory, the dist folders and the browser profiles are removed after this record is committed.
