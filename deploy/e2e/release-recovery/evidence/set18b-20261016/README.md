# set18b, 2026-10-16 (UTC): the alpha.87 candidate at the set18 D1 fix (HEAD 6b363ac46) installed fresh on Debian 13 in set18's lab DNS and ACME, the owner's setup to READY, the Components mail route, "Stop extending", and the extension adding Core Mail followed on screen to READY in a real headless Chrome

A measurement record, the twenty-second native record. Host `date -u` at the start of the work: `Thu Oct 15 23:49:48 UTC
2026`; the run directory was made at 23:59:16Z, the build ran 23:59:40Z–00:03:04Z and the lab 00:03:56Z–00:17:33Z
(`host/progress.txt`); no `date -u` at the end of the work is recorded in this folder (the commit message gives only the commit date, 2026-10-16 UTC); the start reading is in `tools/run-notes.txt`. Page text and pictures show the host's
local time (UTC+3); times here are UTC.

The harness addressed no installed server, no licence service and no certificate authority, and did not contact
celikpanel.net. The release origin's name and the certificate-authority names were pinned to the guest's loopback as the
first driver step and read back at its end (`driver/steps/01-set6-name-pinning`, `10-set6-name-pinning-at-the-end`:
passed). **No ACME request left the guest:** set18's lab table was applied unchanged (`guest_set18_lab_dns.py`, verbatim
rule text in `driver/steps/04-set18-lab-network/set18-lab-network.json`); both certificates came from the lab's own pebble
(issuer `CN=Pebble Intermediate CA 388603`, root `CN=Pebble Root CA 674d30`, both renewal lineages `server =
https://127.0.0.1:14000/dir`); outbound TCP 443 was refused except to loopback, the lab networks and the package hosts,
and the reject counter at the end reads `packets 1` with one logged line, the lab's own self-test to 203.0.113.1 at
00:05:35Z (`driver/steps/09-set18-readings/set18-lab-state-at-the-end.json`: `nft`, `block_log`; UDP 443 and IPv6 reject
counters 0). The archives are test-licence (`acceptance_license`) builds; none is a release. `result.json` carries
`native_evidence: false`; this record closes no P0 row. No product file was changed. This is a measurement, not
acceptance evidence: a QEMU/KVM guest, test-licence builds, a lab-only DNS and ACME fixture.

**Outcome in one line (observed in this lab; not acceptance evidence).** D1, O1 and O3 are fixed as observed: the
Components route lands on the extension's access step with the mail host prefilled and "Stop extending", which restores
the ready screen with nothing installed; "Extend setup" opens the purpose step (Web hosting checked), the components step
marks installed ones "Installed / Keep existing installation", the review shows "What changes / To be installed / Already
installed, kept as they are (6)"; the started extension names its first step with the queued sentence, waits for the mail
host's A record (added by the tool's own trigger, H3 fixed), issues the mail certificate, offers "Continue recorded
operation" once (one press, one POST, then disabled "Checking the same operation"), waits at the reverse DNS check naming
resolver 1.1.1.1, shows "Finishing setup" and reaches "Your server is ready" in 205 s. The driver's API readings after
the handover answered 200 (H2 fixed); the overall verdict is `complete-for-review`. No new defect.

## The lab fixture

set18's, unchanged (`deploy/e2e/release-recovery/guest_set18_lab_dns.py`; see `../set18-20261015/README.md`, "The lab
fixture"). Preparation 00:05:21Z–00:05:37Z: server address 192.0.2.10, the four public resolvers DNAT-ed to the lab
responder, `nftables` + `pebble 2.6.0+ds1-1+b7` from Debian's repository (install rc 0), the 443 self-test "Connection
refused". Record table at the start: `panel.set18.test A 192.0.2.10`, PTR of 192.0.2.10 = the provider's default name;
two later changes stand for the owner's actions (`record-changes`: `mail-a` 00:14:33Z, `ptr-mail` 00:15:51Z). Outgoing
port 25 is answered by the guest's own Postfix (DNAT counter 14 packets): a property of the lab.

## Source and build (`build/`)

Candidate: HEAD `6b363ac4680146f856642cffdab0b57662f6159c` of `feat/alpha87-second-server` (the set18 D1/O1/O3 fix),
built from `git clone`, never from the working tree; no uncommitted product change and no migration since the
v0.1.0-alpha.86 tag (`build/source-check.txt`). Its own release policy reads v0.1.0-alpha.86 / 86, so the installed
archive is set18's policy-only fixture over it (policy `v0.1.0-alpha.87 / 87`, previous 86 = the v0.1.0-alpha.86 tag
commit 8d260f317e58bed7fcd84c6cbb01823c4fe3f166). Go 1.26.5, `build-set18b-artifacts.sh` from run copy b.

| Role | Version / seq | Commit | Archive SHA-256 |
| --- | --- | --- | --- |
| baseline (installed) | v0.1.0-alpha.87 / 87 | a97050d41d0e0e01f46887c56b83938ed120edcd | `6d645d8def2316853d7b1c191b4771c48d51fc65c925d3b0aae3e3312ad0464b` |
| good (never installed) | v0.1.0-alpha.88 / 88 | 0e339739f7a2f0baff21860c25ec419f03573848 | `d5f06737cf1b5eef60de169a05a6dc439aec3de5a0e7d8b1967b5eb44321a32e` |
| defective (never installed) | v0.1.0-alpha.88 / 88 | 7d4b66b295a7058f511aeed706efc0cf69564583 | `c07524d08a501c3c386b06cf13267cbe8f18692c06f9ddfad35186af95d9d539` |

Proof (`set18b_fixture.py prove`, rc 0), dry run (rc 0) and the H2 offline check (`build/snitest-c.json`, below) from run
copy c (`harness-run-copy/c/suite.txt`).

## Lab

| Lab | UTC | Outcome | Raw |
| --- | --- | --- | --- |
| **s18b-a** | 00:03:56Z–00:17:33Z | every driver step passed or observed: lab network, install, owner, licence passed; browser window 00:06:15–00:17:29Z `observed`; `set18-readings` `observed` (H2 fixed); pinning at the end passed; overall `complete-for-review` | `debian13/fresh-setup-a` |

One lab; no second was needed. Gates before the guest start: no other qemu process; C: 346 GiB free, reading 47 s old
(`host/c-drive-cells.txt`, `host/neighbour-gate.txt`).

## Cells (lab s18b-a, browser `browser/setup`, EN 1280×800 unless named; the page sampled every 500 ms)

### 1. Fresh install, owner setup with purpose web, to ready

Sign-in, "Set up with the wizard", Web hosting, `panel.set18.test`, External DNS provider, review (`c1-05-review.txt`),
confirm, "Start reviewed setup" 00:06:34.23Z. States (`c1-samples.jsonl`, 11 pictures `c1-state-*`):

| UTC | Page | Run |
| --- | --- | --- |
| 00:06:34.24 | "Preparing your server" (no guidance yet) | – |
| **00:06:34.75** | "What happens at this step": **"Configure DNS: Use an external DNS provider" — "Setup has started with the step shown here. The panel begins it in a moment and this page updates by itself; you do not need to do anything."** (O1 fixed) | accepted, queued |
| 00:06:37–00:08:14 | "Preparing your server", components Nginx, PHP-FPM, MariaDB, cron, certbot | 02–06 |
| 00:08:23 | "Secure panel access: panel.set18.test" | 09 (pebble order; the Panel's restart was not caught as its own state at 500 ms) |
| 00:08:29 | final check running; the picture `c1-state-09-viewport.png` also shows a banner above the header, "The latest access check failed. Retrying automatically; the current page is preserved while the last verified access remains valid.", with "Check again" and "Reload page" (not in the page-text samples) | verification |
| 00:08:32 | "Verifying the result" | `succeeded / complete` |
| 00:08:33.30 | **"Your server is ready"** (Web hosting, "Extend this server's setup") | – |

Run 119 s; "Finishing setup" did not appear (no waiting state was sampled at the final check, as in set18). The ready screen in
Turkish at 390×844: `v-ready-tr-phone` ("Sunucunuz hazır", "Web barındırma", "Bu sunucunun kurulumunu genişletin",
"Kurulumu genişlet"; no horizontal overflow).

### 3. The Components mail route and "Stop extending" (D1)

Components → "One-click mail stacks" → Core Mail → "Install": the dialog's setup route (`c3-02-dialog*`), mail host name
typed `mail.set18.test`, "Review in server setup" pressed 00:10:04.92Z → `POST /api/v1/setup/extend
{"revision":3,"action":"begin","mail_profile":"core-mail","mail_hostname":"mail.set18.test"}` → 200, `draft`,
`extension: true`, revision 4, components dovecot, mariadb, nginx, php-fpm, postfix.

**Landed on the extension's access step (D1 fixed)** (`c3-04-access-step*`, `setup-summary.json c3_d1`): heading "Extend
this server's setup" with "Choose what to add. Components already installed stay as they are, the review shows what
changes, and nothing starts until you confirm it."; "Set the panel address and DNS method"; Panel address
`panel.set18.test` with "Required public A record before the panel certificate: panel.set18.test → 192.0.2.10."; **Mail
server name prefilled `mail.set18.test`** ("This address identifies the mail service and its certificate. The system
hostname and panel address are preserved. Reverse DNS must match this mail address."); DNS "External DNS provider"
selected; buttons "Back", "Review setup plan", **"Stop extending"** with "Stopping returns this server to its setup as it
was completed. Nothing installed is removed." Twelve samples over 6 s: always the access step, never a mark of the
completed run ("Verifying the result", "Completion could not be verified", "Reconnect to this operation"). The profile
itself is not named on the access step (it is carried by the draft and named in the review; cell 2).

"Stop extending" pressed 00:10:31.48Z → `POST /api/v1/setup/extend {"revision":4,"action":"cancel"}` → 200, `ready`,
revision 5; the page showed **"Your server is ready"** with "Extend this server's setup" again (`c3-05-ready-again*`);
the driver's read (H2 fixed: 200) shows setup `ready` and the latest operation still the first run (576f64fc…); the
page's own read of Components: Postfix and Dovecot `is_installed: false`, `not_installed` (nothing installed).

### 2. The extension from the ready screen, adding Core Mail

| UTC | Screen | What it showed |
| --- | --- | --- |
| 00:10:49.96 | "Extend setup" pressed → `POST /api/v1/setup/extend {"revision":5,"action":"begin"}` → revision 6 | – |
| 00:11:07 | **purpose step** (`c2-01-purpose*`) | "Extend this server's setup", intro as above; "What will you use this server for?" with **Web hosting checked** (Recommended) and "Customize selection" under it; Continue; "Stop extending" |
| 00:11:07–23 | **components step** (`c2-02-components*`) | Nginx, PHP-FPM, MariaDB checked and marked **"Installed" / "Keep existing installation"**; Node.js "Includes required components: Nginx."; "Postfix + Dovecot — These services are prepared together as one supported mail setup." unchecked; the owner checks it (`c2-03-components-mail*`) |
| 00:11:24 | **access step** (`c2-04-access*`) | panel address `panel.set18.test`, DNS external, Mail server name empty → typed `mail.set18.test` |
| 00:11:26 | **review** (`c2-05-review*`, `setup-summary.json c2_review`) | **What changes:** "Server purpose: Web hosting → Custom setup", "Mail server name: none → mail.set18.test"; **To be installed:** "Prepare mail profile: Core Mail"; **Already installed, kept as they are (6):** MariaDB, Nginx, PHP-FPM, Firewall engine, Let's Encrypt client (certbot), Scheduled tasks (cron), each "Keep existing installation"; steps DNS external → mail profile Core Mail → firewall → public DNS panel.set18.test 192.0.2.10 → public DNS mail.set18.test 192.0.2.10 → secure mail access → independent mail renewal → verify; TCP 22, 25, 80, 110, 143, 443, 465, 587, 993, 995, 2083; UDP "No additional ports" |
| 00:12:17 | review in Turkish at 390×844 (`v-ext-review-tr-phone*`) | "Neler değişiyor: Sunucunun kullanım amacı Web barındırma → Özel kurulum; Posta sunucusu adı yok → mail.set18.test", "Kurulacaklar: E-posta profilini hazırla: Temel Posta", "Zaten kurulu, olduğu gibi kalanlar (6)", "Mevcut kurulum korunur"; no horizontal overflow; the view read at 00:12:34.5Z, after that picture, had "Start reviewed setup" disabled |
| 00:12:35.07 | confirm, "Start reviewed setup" | – |

The run (`c2-samples.jsonl`, 13 pictures `c2-state-*`, `c2-summary.json`):

| UTC | Page | Run / owner |
| --- | --- | --- |
| 00:12:35.08 | "Preparing your server" (no guidance yet) | – |
| **00:12:36.10** | **"Configure DNS: Use an external DNS provider" — "Setup has started with the step shown here. …you do not need to do anything."** (O1) | queued |
| 00:12:39 | "Prepare mail profile: Core Mail" | 02 |
| 00:13:39 | "Apply and save the firewall policy" | 03 |
| 00:13:42–00:14:39 | **"Setup is waiting for a requirement"**, "Verify public DNS before certificate issuance: mail.set18.test": "Certificate issuance requires this public A record: mail.set18.test → 192.0.2.10. The public record has not been verified yet. This result alone does not establish whether the record is absent or a DNS server is unreachable. Create or correct this record at the DNS provider or primary that manages it. You do not need to create a website in Domains. This prerequisite is checked automatically. After it passes, the same setup continues. To change the reviewed names or addresses, use Edit plan below." | waiting, phase `access_dns`, `server_setup_access_dns_required` |
| 00:14:32.70 | – | **the tool asked for `lab-mail-a` itself after 51 s of waiting (H3 fixed)**; the owner's A record at the provider, done 00:14:33 |
| 00:14:39 | "Secure mail access: mail.set18.test" | 06 (pebble order, serial 3c7ede156200427c, 00:14:40–41Z) |
| 00:14:48 | "Prepare independent mail certificate renewal": "The mail renewal result cannot yet be verified. Checks continue for the same request. …" | 07 |
| 00:14:51.48 | "Mail renewal preparation is recorded; its final result is not yet verified. … continue this recorded operation below." **"Continue recorded operation"** enabled | 07, `server_setup_mail_enrollment_running` |
| 00:14:51.80 | the tool pressed it (its rule: press whenever enabled) → `POST /api/v1/setup/mail-enrollment/continue {"execution_id":"07b147cb…","step_id":"07-mail_enrollment"}` → 202 `handoff unknown` | – |
| 00:14:52.1, 00:14:53.9 | **"Checking the same operation", disabled** (read 300 ms and 2 s after the press; O3 fixed) | 07 |
| 00:14:58 | **"Setup is waiting for a requirement"**: "Every installation step has finished. Setup is waiting at its final check for this: **The public resolver 1.1.1.1 reports host-192-0-2-10.provider.set18.test as the reverse DNS (PTR) name of this server's address 192.0.2.10; mail servers expect mail.set18.test. You set this at your server provider, not in CelikPanel: in the provider's control panel, or by asking its support to set the reverse DNS of 192.0.2.10 to mail.set18.test.**" | waiting `verification`, `mail_identity action_required` (`reverse_dns_mismatch`, resolver 1.1.1.1) |
| 00:15:44.25 | – | owner "Check requirements again" (same sentence in the result box, `c2-reverse-dns-checked*`) |
| 00:15:51.5 | – | the owner sets the reverse DNS at the provider (`lab-ptr-mail`) |
| 00:15:54.51 | – | owner "Check requirements again" |
| **00:15:56.05** | **"Finishing setup"**: "Every check passed. The panel is completing setup now, and you do not need to do anything. This page updates by itself when it is done." | waiting `verification`, all checks passed |
| 00:15:59 | "Verifying the result" | `succeeded / complete` |
| **00:16:00.21** | **"Your server is ready"** — "Custom setup" (O5) | – |

Run 205 s (57 s waiting for the mail record, 58 s at the reverse DNS check). The enrollment button was on screen in 10
samples; **one press, one `mail-enrollment` POST in the whole run** (`requests.jsonl`: POSTs login 1, extend 3, plan 4,
start 2, mail-enrollment/continue 1). The "no more specific requirement" sentence appeared in no sample of either run.
Ready again in Turkish at 390×844 (`v-ready-after-extension-tr-phone`). Components afterwards: Postfix and Dovecot
`is_installed: true`, `active (running)` (`c2-components-after*`).

After the run (driver readings, `driver/steps/09-set18-readings/native-reading.txt`, API 200 at 00:17:30Z): setup
`ready`; Panel 2083 with SNI `DNS:panel.set18.test` from the lab CA; 587 and 993 the lab-CA certificate (mail lineage
`celikpanel-mail-d1f1f42a…`, `server = https://127.0.0.1:14000/dir`); certbot.timer enabled and active; Postfix
`myhostname = mail.set18.test`, `smtpd_tls_security_level = may`; nginx, postfix, dovecot, mariadb, Panel and Agent
active; system hostname kept (`dns-debian13`). Ledger reading: version 44, the released schema (`ledger-set18-fresh.json`).

### 4. The secondary-DNS box: not reached

Tried cheaply on the Components route's access step (UI state only): the tool's click on "This server" did not change the
selection (DNS stayed "External DNS provider", no role field appeared, no error), so the box
(`setup.infrastructure.secondaryHelp`) was not shown (`c4-secondary-box*` shows the access step with External selected);
DNS was external before and after, and "Stop extending" discarded the draft. Not investigated; not a product finding.
The Components box for a secondary in a DNS pair needs a completed secondary setup: not attempted.

### 5. Ledger and update cells: not part of this record (no update was started).

## Verdicts on the set18 findings

| Finding | Observation here | Verdict |
| --- | --- | --- |
| **D1** extension opened on the completed run | Components route: access step, mail host prefilled, "Stop extending", no completed-run marks in 12 samples; ready screen: purpose step (Web hosting checked), components with kept marks, access, review with the three blocks; Start and Stop reachable in the page; no stand-in request was needed | **fixed (observed)** |
| **O1** generic sentence after Start | both runs: within 0.5–1 s of Start the first step is named with "Setup has started with the step shown here…"; the generic sentence in no sample | **fixed (observed)** |
| **O3** continue button stayed enabled | one press; disabled "Checking the same operation" at +0.3 s and +2 s; one POST (202, `handoff unknown`); the step ended 7 s later | **fixed (observed)** |
| O2 no-SNI certificate | `leaf.jsonl`: after the handover SNI `localhost` gets the lab-CA leaf (`bb44ad72…`), no SNI the first self-signed one (`O=CelikPanel, CN=dns-debian13`, `65fe0f66…`) | unchanged; not judged |
| O4 handover address port | the review's notice offers `https://panel.set18.test:19320` (the lab forward's port); during the run, with the run's server URL, it offers `:2083` | unchanged; not judged |

## Defects, observations, harness notes

**No new defect.**

**O5 (observation, not judged):** adding Core Mail through "Customize selection" turns the server's purpose label into
"Custom setup": the review says "Server purpose: Web hosting → Custom setup" (TR "Web barındırma → Özel kurulum") and the
ready screen after the extension shows "Custom setup" where it showed "Web hosting". It is stated before Start; whether a
web server with mail added should keep "Web hosting" (or read "Web and mail hosting") is for the owner.

**H2 (harness, fixed in this run):** `set18b_trial.py` `SNIPinnedHTTPSTransport` sends SNI `localhost` (the name the
guest's pin reading sends), the exact leaf pin unchanged. Offline check (`build/snitest-c.json`): against a server with a
named and a default certificate, the set18b transport sent SNI `localhost` with Host `127.0.0.1:port` and got 200; set18's
transport (negative control) sent none and got `PinMismatch`. In the lab: `setup-read` at 00:09:28, 00:10:49 and 00:17:27
and the readings at 00:17:30 answered 200; `pin_after_the_window` unchanged.

**H3 (harness, fixed in this run):** `cold-load-set18b.mjs` asks for the mail record when the mail host's `access_dns`
step is the running step of a waiting run, whatever the phase names; it asked after 51 s.

**H5 (harness):** `job-build.sh`'s last grep named set18's build log, so `artifacts.path` stayed empty after a successful
build; the coordinator wrote it from `build-set18b.out` (`tools/suite2.sh`, 00:03:45Z in `host/progress.txt`). The text
that ran is `build/job-build-as-run.sh`; `tools/job-build.sh` is corrected.

**H6 (harness):** the coordinator's own native reading (`tools/native.sh`) came after the driver had stopped the lab
(00:17:33Z) and was not taken; the driver's readings cover it. The tool's Components summary field (`installed`) read a
field the answer does not have (`is_installed`); the values above come from the recorded answers in `events.jsonl`.

## Open items

- The secondary-DNS boxes (setup access step; Components in a DNS pair): not reached.
- The resolvers-disagree PTR sentence; "Finishing setup" slow variant; a final check failing after "Finishing setup": not
  produced. The Panel's own restart was not caught as a separate page state in cell 1.
- O2, O4 and O5 not judged.
- Ubuntu 24.04, Arch; a public, non-lab ACME/DNS path: not run.

## Harness changes in this run (committed with this record)

New: `set18b_fixture.py` (set18's profile, source 6b363ac46), `build-set18b-artifacts.sh` (set18's build with
`set18b_fixture.py`), `set18b_trial.py` (H2), `run-set18b.sh`, `web/tools/browser-inspect/cold-load-set18b.mjs` (H3; the
extension's steps on screen; samples every 500 ms; enrollment presses and POSTs counted; Turkish 390×844 of the ready and
review screens; leaf with and without SNI). set18's files and `guest_set18_lab_dns.py` are unchanged. Run copies
(`harness-run-copy/`): b (build), c (lab s18b-a); each with the overlay's hashes and its diff against 6b363ac46. The
browser tool was assembled from set18's by `tools/assemble.py` with `tools/head.txt`, `follow.txt`, `cell.txt`.

## Secrets, host, leftovers

Redacted at collection by the harness (owner password, session, fixture keys); set6's digest sweep over the staged lab
changed 0 files; driver `SHA256SUMS` verified on the staged copy (46 files) before and after the sweep. `tools/scrub.py`
replaced host paths in 23 text files (`host/scrub.txt`). C: (Get-PSDrive) 352.88 GiB free at 23:59:12Z, lowest 344.61 GiB,
344.70 GiB at 00:18:13Z (`host/c-drive-watch.txt`); WSL `/` 941 GB free at 23:59:16Z (`host/hostcheck-before.txt`). The lab, the run directory, the dist
folders, the browser profile and the scratch browser output are removed after this record is committed.
