# CelikPanel Roadmap

*Last updated: October 10, 2026 · [Türkçe](ROADMAP.tr.md)*

---

## The Constitution — Every Decision's Filter

Every feature, commit and design decision must satisfy these requirements.
They are obligations, not a claim that the present implementation meets them.
The [resilience contract and source audit](docs/RESILIENCE-CONTRACT.md) records
open P0 work, implementation order and the evidence required to close it (D-025).

### 1. Security and owner authority
- Authenticate management access and authorize each action for its exact resources. Use least privilege, local authenticated IPC, parameterized SQL and `crypto/rand` for secrets.
- The unprivileged Panel and privileged Agent remain separate. Normal privileged automation uses the authorized Agent API; supported owner recovery has a separate narrow contract. Neither an AI planner nor a recovery UI gains unrestricted root execution.
- The server owner retains native service administration. Detect owner changes and reconcile explicitly; never silently overwrite them to match cached intent (D-022).
- When evidence is uncertain, block the affected unsafe mutation. Preserve authenticated diagnostics and supported recovery without granting unverified privileges.

### 2. Continuity and recoverability
- Panel outage, license loss or removal must not stop hosted workloads or their native renewal, scheduling and boot mechanisms. Remaining dependencies must be eliminated and tested before claiming independence.
- Every mutation defines affected resources, read-only preflight, durable checkpoints, bounded retry/recovery and terminal proof. Permission or ownership normalization is also a mutation.
- Recovery must remain usable when the candidate release, ordinary Agent, application migration or license verifier fails. Mixed or unverified application state may block normal management; it must not erase the independent recovery path.
- Preserve the last verified usable state and recovery material until the candidate and its recovery compatibility are verified. Cleanup follows that proof.
- Automatic repair is a deterministic, idempotent continuation or compensation of the accepted operation. It does not invent missing evidence, undo later owner changes, bypass validation or launch a second unknown mutation.

### 3. Truthful state and shared contracts
- Separate owner intent, authority, published configuration, observation, execution, verification and recovery. Unknown is not absent, failed, expired or completed.
- Each durable artifact has one versioned producer/reader/restore contract with explicit supported transitions. Compare evidence according to its role; do not use whole-record equality where legitimate publication advances only part of a record.
- A completed installation step is historical execution evidence, not proof of present health. Report the current reason, responsible actor, next action and how the same operation resumes (D-024).
- The browser observes authoritative operation state. Refresh, reconnect and timeout never authorize duplicate work or imply completion.

### 4. Simplicity
- Give each routine task one clear user path. Share the underlying operation contract across the browser and supported owner recovery; a single screen is not a single point of recovery failure.
- Add features only for a concrete user need. Use safe defaults within the accepted scope and reveal advanced choices when needed.
- Keep unused service-specific navigation quiet. Keep installation discovery, actual conflicts and recovery actions visible when they help the user complete the task.
- Normal operation should be possible through the panel. Native owner administration and recovery remain supported; a manual rescue is evidence of an automation gap, not a reason to prohibit rescue.

### 5. Speed with evidence
- Targets remain API response under 100 ms, responsive interaction and a 60-second minimal installation. Record the measured platform and scope before claiming any target achieved.
- Keep the runtime small. The existing Panel/Agent privilege split, native workload services and an independent recovery mechanism take precedence over a one-binary slogan.
- Speed does not justify skipping validation, recoverable checkpoints or fault testing.

### 6. Flexibility and independence
- Use typed APIs, modular services and standard protocols; optional automation is separate from service operation.
- Backups and exports use standard formats. Management software does not own or hold the owner's data hostage.
- Standard DNS replication does not require a remote panel or its license. Separately authorized remote record management is optional.
- Installed-panel updates are initiated only by the owner in CelikPanel's update UI. Publication, diagnosis and supported rollback do not authorize assistant-side installation.

### The honesty and release rule

Tests, security review and documentation remain necessary. Lifecycle support
also requires complete fault-transition evidence in disposable native environments:
real previous-release state, failed update, actual automatic restoration, recovery
interruption/reboot, preserved owner changes, management recovery and workload
probes. Component tests, mocked service managers and a successful installation
alone do not prove that contract.

Every lifecycle change names its affected invariant, schema/version transition,
recovery behavior and acceptance evidence. Unmeasured or inconclusive results stay
open. A narrowly scoped incident correction may ship with its limits explicit;
it does not close foundational P0 work or justify unrelated feature expansion.
The exit matrix in D-025 must be implemented and passed before claiming resilient
operation. No system is promised to recover autonomously from every possible fault.


---

## Where We Are — September 26, 2026

**Current priority: finish D-025 architectural resilience before expanding the product.**
This review covers the local source through `f253d318` and the retained acceptance
reports. It is not a new inspection of Frankfurt/Boston, a published release, or
proof that these changes are installed. P0.1–P0.5 are all **partial**; none is closed.
The constitutional requirements and their identifiers remain unchanged.

### Evidence and remaining work

| Existing item | Implemented or demonstrated within a stated scope | Still required for closure |
|---|---|---|
| P0.1 — Native update/rollback | Real old-release restoration on Arch/Debian; genuine Alpha64 schema38 data used in later migration and recovery trials. [Evidence](deploy/e2e/release-recovery/ISOLATED-DATABASE.md). | Complete the supported update/fault/workload matrix and production-signed candidate admission; a successful selected checkpoint is not whole-update acceptance. |
| P0.2 — Access and truthful status | Independent authenticated recovery/status entrypoint; selected CLI, HTTP and browser terminal results agree. Native boot-wait retries retain the same operation. [Evidence](deploy/e2e/release-recovery/BOUND-WORKER.md). | Native waiting/error/reconnect cases, preservation of known failures, browser access during waits and the owner-initiated update path with production trust. |
| P0.3 — Independent recovery | Separately retained recovery code/data, isolated DB migration and atomic publication; selected cuts during rollback and reboot recover automatically with cut-time rows preserved. [Two-fault evidence](deploy/e2e/release-recovery/NATIVE-EXCHANGE-RECOVERY.md). | Remaining checkpoints, incomplete capture, metadata transitions, old-version compatibility and safe cleanup. Eventual recovery is not uninterrupted service or power-loss durability. |
| P0.4 — Shared DNS/TLS contracts | DNS acquisition/publication roles separated; shared TLS and DNS readers; independent DNS observation and selected Agent-mediated fault recovery. [Contract](docs/DNS-ENGINE-ARTIFACT.md). | Supported **Agent-independent DNS inverse execution**, native interruption/owner-edit acceptance, complete producer/restore transitions, and peer deletion proof when no authoritative parent is available. Dormant inverse code and read-only observation do not close this gap. |
| P0.5 — Native service independence | Scoped firewall/mail renewal and enrollment recovery; standalone PowerDNS and BIND/BIND pair serving after management-absent or management-disabled reboot, as specified in each report. [Mail evidence](deploy/e2e/release-recovery/MAIL-ENROLLMENT-MANAGEMENT-ABSENT-BE.json); [DNS evidence](deploy/e2e/dns-kill-matrix/NATIVE-BIND-PAIR-TARGET-STAGED-20260926.md). | Cross-engine DNS pair combinations, full setup/enrollment and old-application compatibility, and web/DB/mail/cron/renewal/firewall checks under every claimed management-absence/removal mode. Disabled management is not full removal. |

The detailed [acceptance register](docs/RESILIENCE-CONTRACT.md) retains failed
attempts and exact boundaries. New evidence updates an existing P0 item; it does
not create a replacement architecture plan.

### Release state — October 10, 2026

This subsection records where the releases stand (written on 2026-10-10; the
entry for v0.1.0-alpha.83 was added on 2026-10-11 as a candidate, amended
for its publication the same day, and amended again on 2026-10-12 for the read
of the installed servers and the owner's report; the entry for
v0.1.0-alpha.84 was added on 2026-10-13 as a candidate, amended for its
publication the same day, and amended again that day for the read of the
installed servers and the owner's report; the entry for
v0.1.0-alpha.85 was added on 2026-10-15 as a candidate and amended for its
publication the same day; the entry for v0.1.0-alpha.86 was added on
2026-10-15 as a candidate and amended the same day for the native records
set17 and set17b and for its publication; the entry for v0.1.0-alpha.87 was
added on 2026-10-16 as a candidate and amended the same day for its
publication; the entry for v0.1.0-alpha.88 was added on 2026-10-16 as a
candidate and amended the same day for its publication; the entry for
v0.1.0-alpha.89 was added on 2026-10-16 as a candidate and amended the same day
for its publication and the owner's report of both updates; the entry for
v0.1.0-alpha.90 was added on 2026-10-16 as a candidate and amended the same day
for its publication and the owner's report of the update; the entry for
v0.1.0-alpha.91 was added on 2026-10-16 as a candidate and amended the same day
(tagged, deliberately not on the download portal); the entry for
v0.1.0-alpha.92 was added on 2026-10-16 as a candidate and amended the same day
for its publication and the owner's report of the update; the entry for
v0.1.0-alpha.93 was added on 2026-10-16 as a candidate and amended the same day
for its publication; the entry for
v0.1.0-alpha.94 was added on 2026-10-17 as a candidate and amended the same day
for its publication; the entry for
v0.1.0-alpha.95 was added on 2026-10-17 as a candidate). It changes no
P0 status: P0.1–P0.5 are all still **partial**, and every open acceptance item
above and below stays open.

- **v0.1.0-alpha.81 is published.** The tag `v0.1.0-alpha.81` is at `a0beb726`
  (2026-10-04), which is the head of the published `main`. Reported by the
  owner, and not recorded in a file here: the publication steps (the merge of
  pull request #204, the tag, the portal publication), the verification of the
  six signed assets, and that on 2026-10-08 the owner updated the two installed
  panels (one Ubuntu 24.04, one Debian 13) from the panel's own update screen.
  The repository holds no evidence file for those installed servers, so this
  is the owner's report, not a measurement, and it is not used as acceptance
  evidence for any item here.
- **v0.1.0-alpha.82 is published, 2026-10-10.** Owner
  decisions of 2026-10-10 (clock date), three: the number is v0.1.0-alpha.82;
  the final code is installed fresh and measured before publication (done, the
  sixth run below); and the Panel's `Strict-Transport-Security` header is
  limited to the Panel's own host name and enters this release
  ([D-030](docs/DECISIONS.md), `72b879eea`). The owner's own test was the
  update of the two installed servers, which the owner started from the
  panel's update screen; no disposable server was used for it. The release
  was the branch `fix/setup-handover-guidance`, pull request #205: 37
  commits after `a0beb726` when the candidate entry was written (head
  `85aa05272`). The last
  commit that changes a file under `cmd/`, `internal/` or `web/` is
  `72b879eea`; the commits after it add test harness, evidence, documents and
  the release-sequence preparation
  ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.82.md), whose "Publication
  record" holds the details below). It contains:
  screens that show "checking" or "could not be checked" instead of a negative
  statement they do not know; settings writes that carry the version of what
  they were built from and are refused when it is stale (mail policy, backup
  schedule, scheduled tasks, configuration files, catch-all); reloads and
  service actions answered by what the service shows afterwards; one identity
  per state-changing request on eight routes, so that a repeated request is
  answered and not run again ([D-029](docs/DECISIONS.md)); corrections to the
  cPanel import, certificate requests and PHP sites on Arch; and one rule for
  values read from `postconf`, which corrects a mail certificate defect that
  is also in the published v0.1.0-alpha.81.
  - *Measured on disposable guests, 2026-10-08 and 2026-10-09 (Debian 13,
    Ubuntu 24.04, Arch; one sequence per cell unless the record gives a
    number):* the settings writes on real cron, Postfix, PostgreSQL and MariaDB
    ([first run](deploy/e2e/release-recovery/evidence/set1-20261010/README.md));
    the corrections that run led to, the service actions and the request
    identity
    ([second run](deploy/e2e/release-recovery/evidence/set2-20261011/README.md));
    the second round of corrections and the owner-started update from the
    published v0.1.0-alpha.81, ten cells, all reaching their expected end
    ([third run](deploy/e2e/release-recovery/evidence/set3-20261012/README.md));
    the last corrections on fresh guests at `557b554eb`: a PHP site created,
    run and deleted on all three platforms, Arch included, a site created by
    the published v0.1.0-alpha.81 answering ten requests identically across
    the update on Debian 13 and Ubuntu 24.04, and one failed check, the note
    after a Postfix stop on Ubuntu 24.04
    ([fourth run](deploy/e2e/release-recovery/evidence/set4-20261009/README.md));
    the cause of that failure and its correction, four stops each on fresh
    Ubuntu 24.04 and Debian 13 guests
    ([follow-up](deploy/e2e/release-recovery/evidence/set4b-20261009/README.md)).
    The names of the first three directories carry later dates than the days
    the runs were made. The guests are not network-isolated: they run on QEMU
    user networking with outbound NAT. What is established is name pinning for
    the release origin in every cell and for the Let's Encrypt names in some
    cells (which, and when in the cell, the release notes say), a license step
    answered by the test build, and no installed server's name or address in
    the raw records. The guests' traffic was not recorded.
  - *Measured as a reading only:* what the real `postconf` prints when
    `main.cf` makes it warn, and what the old rollback command does with the
    text the old code kept
    ([reading](deploy/e2e/release-recovery/evidence/set4c-20261009/README.md);
    Postfix 3.10.13, a private configuration directory on a development guest).
    The correction built on it (`67b62cc0f`) has component tests only: a mail
    TLS change, its rollback and a certificate publication with such a file
    were not run.
  - *Measured at `67b62cc0f`, 2026-10-09:* the update matrix again, the same
    ten cells as the third run, each once
    ([fifth run](deploy/e2e/release-recovery/evidence/set5-20261009/README.md)).
    That code differs from the final code only in the header rule of
    `72b879eea` (one line of code, its comment and its test).
    All ten reached the end the third run measured at `cfa329676`, with the
    same verdict for every step, the same outcome and the same compared facts:
    update verified on Debian 13, Ubuntu 24.04 and Arch; automatic return to
    the published v0.1.0-alpha.81 on all three; return after a failed start
    check (Debian 13); owner continuation (Debian 13, Ubuntu 24.04); management
    off across a reboot (Debian 13). A site created by v0.1.0-alpha.81 answered
    ten requests identically across the update (Debian 13, Ubuntu 24.04), and
    one Postfix stop per platform on the updated server answered with the note.
    The guests' disks were in memory in this run, so its durations are not
    comparable with earlier runs; the host logged modern standby for most of
    it while the samplers show no pause.
  - *Measured on the final code (`72b879eea`), 2026-10-09 21:46–23:21 UTC
    (2026-10-10 local time), the sixth native record:* the code installed
    fresh on Arch, Debian 13 and Ubuntu 24.04 with the checks of the fourth
    run and its follow-up (PHP site, import with its three lists, absolute
    archive entry, refused site, reload of stopped services; the Postfix stop
    note on Debian 13 and Ubuntu 24.04, not on Arch); the Panel's header read
    on the guest from the running Panel in 14 readings of eleven HTTPS
    answers each, exactly `max-age=31536000` on every answer read from this
    code and `max-age=31536000; includeSubDomains` on every answer of the
    published v0.1.0-alpha.81 before each update, and `400` without the
    header for plain HTTP to the Panel's port; and one verified update from
    the published v0.1.0-alpha.81 per platform
    ([sixth run](deploy/e2e/release-recovery/evidence/set6-20261009/README.md)).
    One harness rule failed in the first Arch cell and passed when the cell
    was run again; no product check failed. Not measured: a fresh install of
    an archive labelled v0.1.0-alpha.82, anything in a browser, the header on
    a Panel with its managed certificate, the guests' traffic.
  - *Publication, 2026-10-10 (UTC unless noted).* The owner merged pull
    request #205 into `main` as the squash commit `2a0af8866` (merged 07:15,
    read from the repository host), created the annotated tag
    `v0.1.0-alpha.82` on it (08:01) and pushed it; the repository variable
    `CELIKPANEL_RELEASE_SEQUENCE` had been set to 82 by the owner before the
    merge (owner-reported). The tag's CI run completed with 22 of 22 jobs
    successful (08:01 to 08:21, read from the repository host) and published
    exactly six assets: the generic archive and its checksum, the linux/amd64
    archive and its checksum, the signed manifest v2 and its signature.
    Platform archive SHA-256
    `a37671064f2ea0b08fd5e0e25c14218bb8b005067855bf9c13af2fc653ffe4bc`,
    65,894,979 bytes, sequence 82 (size and digest read from the repository
    host); that host lists the release as published at 08:21:32. The portal's
    `published_at` field, 07:15:10, is the commit's time. The six assets were
    verified on a private clone of the tag (reported by the release preparer,
    not recorded in a file here): checksums, generic archive byte-identical
    to the platform archive, `release.commit` and `release.tree` equal to the
    tag, bootstrap members equal to the tag's, both release guards pass, the
    manifest signature verifies with the tracked public key, no
    acceptance-licence build tag in the binaries. The download portal was
    assembled in pre-signed mode from the four platform assets, packaged
    reproducibly (package SHA-256
    `aa19c55826e04bf7de470af44adb2bba864595e4648620bcf45793bf7fad215e`,
    132,163,894 bytes), rehearsed locally through `promote-download-portal.py`
    against a fixture root holding the alpha.81 portal, and published by the
    owner with `deploy/publish-download-portal.ps1` at about 09:36: the
    transaction reported `status: committed`, one public verification pass of
    26 requests (`status: ok`), the backup kept (owner-reported). An
    independent read of the public site afterwards (release preparer, not
    recorded in a file here): `releases/latest.txt`, `latest.json` (sequence
    82 and the commit), the served manifest, signature and checksum, `get.sh`
    and the public key all agree with the tag's assets; alpha.81's archive is
    still served. **Installed panels (owner-reported):** the owner updated
    both installed servers (one Ubuntu 24.04, one Debian 13) from the panel's
    own update screen on 2026-10-10; both report v0.1.0-alpha.82 and
    `2a0af8866`. The Panel's `Strict-Transport-Security` header, read from the
    outside on each before and after: `max-age=31536000; includeSubDomains`
    before, `max-age=31536000` after: D-030 observed on real servers, not in
    a browser. No assistant updated or touched an installed panel.
  - *Measured in a real Chrome against the published code, 2026-10-10, the
    seventh native record*
    ([seventh run](deploy/e2e/release-recovery/evidence/set7-20261010/README.md);
    committed after its intake check, whose narrowings the record carries):
    disposable Debian 13 guests, Chrome 154 headless, the tags' source
    rebuilt with the acceptance-test licence, not the published archives. (1)
    alpha.82 interface, update started from the
    card: **pass** — Settings stayed mounted and inert under the hold layer
    for about 9 to 11 s of a 25 s outage and continued where it was. (2)
    alpha.81 interface: the page was replaced by the full-screen "License
    status could not be checked" page for about 78 s of an 89 s outage,
    the screen the owner described (the two outages are not comparable). (3)
    tab hidden 631 s, Panel running: **pass**, zero requests while hidden,
    one quiet read on return. (4) hidden across a Panel restart: **pass**,
    with the limit that the restart lasted under 0.5 s, shorter than the
    probes' intervals. (5) cold full page load of `/setup`, `/` and
    `/settings?section=updates`, 18 loads: **fail on all three routes**: the
    full-page "Checking panel access" screen of `RecoveryAccess` (a gate not
    yet converted) was painted before any session read answered, about 80 to
    100 ms on a fast link and about 1.7 s throttled to 2 Mbit/s and 300 ms;
    the rule that nothing is drawn until the quiet time (1.5 s) has passed or
    a read answered without confirming access is not met by that gate. Not
    measured: Ubuntu, a headful browser, a phone viewport, Turkish, the HTTP
    cache enabled, any installed server. No P0 item is closed or advanced.
  - *Which check rests on which code:* settings writes at `c4cf7fd9d`;
    corrected settings writes, service actions and request identity at
    `faa5ef085`; second corrections and the first update matrix at
    `cfa329676`; fresh-server checks at `557b554eb`; four Postfix stops per
    platform on files identical to `1f182a483`; the ten-cell update matrix at
    `67b62cc0f`; fresh install, the header and one update per platform at
    `72b879eea`. Not repeated on the final code: the settings-write,
    request-identity and full service-action cells of the first three runs,
    and seven of the ten update cells. Not run on any real system: the
    corrected mail certificate path (snapshot, restore, read-back).
  - *Open for the next release, with its exit:* the Panel's own start writes
    the web server configuration of hosted sites again, in this candidate and
    in earlier releases. What an owner's hand edit of such a file meets at
    that start has not been measured or audited against the rule of constitution
    section 1 (detect owner changes, never silently overwrite). Exit: a
    native reading of an owner-edited site configuration file across a Panel
    start and across an update, and the product either detects and reports
    the edit or the overwrite is a stated, documented limit.
  - *Component tests only:* `e2be8af30` (a `mysqld` that is not MariaDB is not
    accepted as the check of a MariaDB file), the two new notes for a unit that
    did not settle or could not be read after a stop, and the mail certificate
    renewal's check of the served certificate.
  - *Open and named in the release notes:* 31 interface source files still read
    the server the old way (access gates, the setup wizard, the update and
    operation overlays, the Services pages and others); the state-changing
    routes outside the eight have no request identity; a server that already
    has the independent mail renewal helper keeps its installed helper; no
    screen was rendered against a real Panel in the first six native runs
    (the seventh did, for the update and the first page load only); the update from
    the published release was measured from the tag's source built with the
    test license, not from the signed archive; a 58-character site name is
    refused by stock nginx; the certificate-validation directory stays after a
    refused create and after a delete.
  - *Open for the next release (2026-10-10, after publication).* None of
    these is fixed in v0.1.0-alpha.82, and none changes a P0 status:
    1. Owner observation on the Ubuntu server: an update started from the
       alpha.81 interface showed the full-screen "License status could not
       be checked" page during the Panel restart, then recovered by itself;
       on the Debian server, started the same way, it did not appear
       (owner-reported; the lab reproduced the alpha.81 screen, seventh
       record).
    2. Owner observation on the Debian server's updates page: the card
       showed v0.1.0-alpha.82 installed while a corner notice still said the
       update was being applied and updates were locked, at T+02:03. The two
       disagreed (owner-reported).
    3. Owner observation: a cold full load of `/setup` shows a full-page
       "Checking panel access" screen for a moment (owner-reported). The
       seventh record, cell 5, measured the same on `/setup`, `/` and the
       updates page: the `RecoveryAccess` gate paints before any session read
       answers. It is on the list of gates not yet converted to the
       known-state rule.
    4. Owner observation: the setup wizard page says "This operation stopped
       ... A required check needs attention" at the top while its last step
       reads "Verify the prepared server - In progress", and the reason is
       under the collapsed "Checks and how to continue" and "Technical
       details"; D-024 requires the reason and who acts before the list. The
       same in alpha.81 (owner-reported).
    5. The audit of what an owner's edit of a generated site configuration
       file meets at a Panel start (the open item above, unchanged).
    6. The rest of the alpha.82 limits that were deferred: the 31 interface
       source files not yet converted; the state-changing routes without a
       request identity; the migration of installed renewal helpers; the
       mail certificate path behind a `postconf` warning, never measured on a
       server; the vendor publishing tools and two one-time recovery scripts
       that the archive still carries (owner decision of 2026-10-11: they stay
       in v0.1.0-alpha.83; removing them is a later release's packaging
       change); the Postfix
       restriction list that a DNSBL save rewrites to one line; and the
       missing control for a database user's password.
  - No P0 item is closed or advanced by this release. Publishing it did not
    update any installed panel; the owner updated the two installed panels
    from the panel's own update screen.
- **v0.1.0-alpha.83 is published, 2026-10-11; the owner updated both
  installed servers to it on 2026-10-11 (observed from outside on 2026-10-12);
  the owner reported two screenshots on 2026-10-12, recorded below, with open
  parts.** Owner
  decisions of 2026-10-11 (clock date), two: the number is v0.1.0-alpha.83;
  and the owner-kept site configuration files
  ([D-031](docs/DECISIONS.md)) enter this release. Recorded with them: the
  narrower `Strict-Transport-Security` header of v0.1.0-alpha.82
  ([D-030](docs/DECISIONS.md)) was read from the outside on both installed
  servers before and after their update (owner-reported, in the publication
  entry above); this release does not change it. The release was the branch
  `fix/alpha83-known-state-gates`, pull request #206 into `main`: 44
  commits after `2a0af8866` when the candidate entry was written. The last
  commit that changes a file under `cmd/`, `internal/` or `web/src` other than a test is
  `40f02ecb6`; the commits after it add tests, evidence, documents and the
  release preparation (sequence 83, the installer's bootstrap pins and the
  README version lines, in `ab67d9fc4`, which changes no file under `cmd/`,
  `internal/` or `web/`), and further commits of that kind raise the count
  without changing this
  ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.83.md), whose "Publication
  record" holds the details below). It contains:
  a site's nginx file, PHP-FPM pool and application unit that the owner
  changed are kept and named, with three choices on the domain's page
  (database schema 43 to 44, migration 044); certificate validation published
  through a directory of CelikPanel's own, with a check before a certificate is
  requested for a kept file; a refused change puts the previous file back and
  reloads the service only when the service's own check accepts it; the first
  page load draws nothing until there is something to say, the layer over the
  page names the cause it read, the update notice and the card agree, and a
  stopped setup says why above its steps.
  - *Measured on disposable guests, 2026-10-10 and 2026-10-11 (UTC), the
    seventh to the thirteenth native records* (test-licence builds; updates
    from v0.1.0-alpha.81 and alpha.82 rebuilt from their tags' source, not the
    published archives; QEMU user networking with outbound NAT, traffic not
    recorded; names pinned to loopback; one host):
    [seventh](deploy/e2e/release-recovery/evidence/set7-20261010/README.md)
    (the alpha.81 and alpha.82 interfaces in a real Chrome, Debian 13),
    [eighth](deploy/e2e/release-recovery/evidence/set8-20261010/README.md)
    (what alpha.82 does with an owner's edit, three platforms),
    [ninth](deploy/e2e/release-recovery/evidence/set9-20261010/README.md)
    (first load, access wait and an update in a real Chrome, Debian 13),
    [tenth](deploy/e2e/release-recovery/evidence/set10-20261010/README.md)
    (the first part of D-031, three platforms),
    [eleventh](deploy/e2e/release-recovery/evidence/set11-20261011/README.md)
    (its corrections: Debian 13 and Ubuntu 24.04, Arch for the include
    directories, the locked file and one validation case),
    [twelfth](deploy/e2e/release-recovery/evidence/set12-20261011/README.md)
    (the pool, the unit and the update from alpha.82, three platforms) and
    [thirteenth](deploy/e2e/release-recovery/evidence/set13-20261011/README.md)
    (the corrections after that, Debian 13 and Ubuntu 24.04 only; Arch not
    run). Each record's intake corrections narrow its own text and win over
    this summary. The measured code differs from the final code by the changes
    named in the release notes; the changes after the thirteenth record
    (the resume and "not running" sentences, a verify time-out, the page read
    that records what it finds, the `service` field of a successful choice)
    have component tests only.
  - *Continuous integration on the pull request:* the runs on the heads of
    10:20, 11:40, 11:44 and 11:49 UTC on 2026-10-11 each passed 23 of 23 checks
    (the job that publishes assets runs only on a release tag); the head of
    11:44 UTC carries the last change to product code. Six runs before them had
    failed on a data race in a test's fake Agent, fixed in test code. Runs of
    commits made after 12:09 UTC are not entered here.
  - *Publication, 2026-10-11 (UTC).* The owner squash-merged pull request #206
    into `main` as `0447558e956d4c02c7bf06b368def0924856f8fc` (tree
    `7adde05327ecadb3c7d3451ec9b08fab9494a480`, equal to the tree of the pull
    request's last commit `304da2466`); CI on that commit (run 38145660891)
    passed 23 of 23 jobs that ran, and the job that publishes assets was
    skipped because it runs only for a tag. The branch protection of `main`
    required only "Release — reproducible archive and checksum" (read on
    2026-10-11), so the split race shards needed no rename. The owner set the
    repository variable `CELIKPANEL_RELEASE_SEQUENCE` to 83 before tagging,
    then created and pushed the annotated tag `v0.1.0-alpha.83` (tag object
    `f5772a5b96fe8a341c63fd71a27a750bade4e382`) on that commit. The tag's CI
    run (38153704563) completed successfully with 24 jobs and published the
    release as a pre-release at 16:22:31 with six assets. Platform archive
    `celikpanel-v0.1.0-alpha.83-linux-amd64.tar.gz`, 66,132,418 bytes,
    SHA-256 `6275bebed2b581bad58d4323217ed14defb54d06c23cef5af94218da573aefe5`,
    sequence 83. The assets were checked in a local build guest from a clone at
    the tag (reported by the release preparer, not recorded in a file here):
    both checksum files verify; the two archives are byte-identical; the signed
    manifest says sequence 83, version v0.1.0-alpha.83, the commit above, that
    digest and size, with `published_at` 2026-10-11T14:03:46Z; its Ed25519
    signature verifies against the tag's `deploy/release-signing-ed25519.pem`
    (the same file as the copy in the archive and the pin in its
    `libexec/get.sh`); the archive's `release.commit` and `release.tree` equal
    the tag's; `install.sh`, `libexec/get.sh` and the public key in the archive
    are byte-identical to the tag's; both release guards exit 0; the bootstrap
    pins are sequence 83 and v0.1.0-alpha.83. The download portal package was
    built from the tag in pre-signed mode with the six assets, twice with
    identical results (132,634,305 bytes, SHA-256
    `6787d0b42ff70621fe1a2ee46c75d336ca24092108a2182e6c35a2da0f5c94d2`), its
    contract test and the public-verifier (17 tests) and promotion (14 tests)
    tests passed, and a local rehearsal of the real promoter against a fixture
    copy of the alpha.82 site ended `committed` with 26 requests. The owner
    published the portal with `deploy/publish-download-portal.ps1` (previous
    v0.1.0-alpha.82, target v0.1.0-alpha.83): status `committed`, the backup
    kept, one public verification pass of 26 requests (`status: ok`), the
    success marker printed once (owner-reported). A read-only read of the
    public site from outside afterwards (release preparer, not recorded in a
    file here): `releases/latest.txt` is `v0.1.0-alpha.83`, `latest.json`
    names the commit and the digest, the served manifest, signature and
    checksum are byte-identical to the verified assets, the archive URL
    answers 200 with `Content-Length: 66132418` (not downloaded again), `get.sh`
    pins sequence 83 and v0.1.0-alpha.83, and the alpha.82 manifest is still
    served. **Installed panels: not updated when this publication record was
    written on 2026-10-11.** Each owner starts that from the panel's own update
    screen. **On 2026-10-12 at 18:36 UTC a read from outside** (read-only, over
    HTTPS, without logging in; nobody other than the owner touched the servers,
    and the release preparer did not start, watch or verify the update) found
    both installed servers serving the interface build of v0.1.0-alpha.83: their
    start page references the same built interface files as `web/dist/index.html`
    in the verified archive, and one of those files, read from each server, is
    byte-identical (same SHA-256) to the archive's copy; its `Last-Modified`
    dates are 2026-10-11 19:27:52 UTC (first server) and 19:28:17 UTC (second
    server); both still send `Strict-Transport-Security: max-age=31536000`.
    This shows that the Panel process on each server serves this release's
    interface, so the owner updated both from the panel's own update screen on
    2026-10-11, at about 19:27 to 19:28 UTC. It does not show the update's
    verification result inside the panel, the Agent's version, what the first
    start did with the site configuration files (how many were adopted, how
    many left as they were), or anything the owner saw on screen. This read is
    not acceptance evidence. **Reported by the owner on 2026-10-12** (two
    screenshots, English interface, one per server; the owner's observation,
    not acceptance evidence, nothing measured in the lab): on both servers the
    sidebar reads `CelikPanel · v0.1.0-alpha.83 · build 0447558e956d` (the
    12-character prefix of the release commit), and the build id wraps in the
    middle of the word. The first server (hosting purpose "Custom setup", no
    domains) lands on the Dashboard; its "System services" card says "Unknown"
    ("Component status is outdated. Open Components to rescan.") and its "Needs
    attention" box says the installed components were not checked recently; no
    error and no licence page is reported for it. The second server lands on the
    setup page, step 5, because its setup plan was reopened before the update
    and its final check never finished: above the steps the page says "Setup
    stopped: its plan was reopened", that this is not a failure, that nothing
    continues by itself, which check was still waiting, and what to do next
    (review the plan and start it again); every step reads "Completed" except
    the last, "Verify the prepared server", which reads "Stopped", and none
    reads "In progress". That is the setup-page observation of alpha.82 (stopped
    and in progress together) seen resolved on an installed server. Read from
    outside on 2026-10-12 by the release preparer: that server's mail host name
    resolves to the server's address, but the reverse (PTR) record of the
    address names a different host name, so the pending final check will not
    pass until the owner changes the PTR record at the hosting provider,
    outside CelikPanel. Not reported: whether the full-screen licence page
    appeared during the restart; whether the update notice and the card agreed;
    whether the momentary licence page still appears on a direct load of the
    setup address; the update's verification result inside the panel; the
    Agent's version; the Domains list (both servers have no domains, so the
    "left as they were" row cannot appear on them and the first start's
    adoption of site files had nothing to adopt there); the `postconf -n
    >/dev/null` output. The release notes' "Publication record" items 12 and 13
    hold the details.
  - *Open for the next release, with the release's limits as the release
    notes list them:* the sidebar's build id wrapping in the middle of the word
    (owner's screenshots, 2026-10-12; it can read as a different number); a PHP
    version switch never measured; Arch absent from the
    thirteenth record; "Keep mine", "Recreate" and "Write CelikPanel's text now"
    on a pool or unit, and the not-running case for nginx and an application,
    run in no lab; the removal of CelikPanel's own include directory at a
    site's deletion not looked at; no certificate issued in any lab, and the
    success answer's fields for a check that did not run never seen; the first
    start's writes not measured on an installed server's files; a second
    update after a return not run; 30 interface source files (88 places) not
    yet converted to the known-state rule; twelve routes with a request
    identity, the rest without; installed mail renewal helpers not replaced by
    an update; the mail certificate path behind a `postconf` warning never
    measured on a server; the Postfix restriction list that a DNSBL save
    rewrites to one line; no control for a database user's password. The open
    item "what an owner's hand edit of a generated site configuration file
    meets at a Panel start" is answered by this release and stays open: both
    installed servers serve this release's interface, but they have no
    domains, so the first start had no site files to adopt on them and the
    question is not answered on an installed server; it stays open until the
    release is seen on an installed server with sites.
  - No P0 item is closed or advanced by this release or by its publication.
    Publishing it did not update any installed panel; only the owner starts
    that, from the panel's own update screen.
- **v0.1.0-alpha.84 is published, 2026-10-13; the owner updated both
  installed servers to it on 2026-10-13, seen from outside and in the owner's
  screenshots, with observations not all reported.** Written 2026-10-13 as a
  candidate, amended for its publication the same day, and amended again that
  day for the installed servers. The owner
  decided on 2026-10-13 that the number is v0.1.0-alpha.84. Owner
  decision of 2026-10-12, after the update to v0.1.0-alpha.83: the Dashboard
  checks the components by itself, without wasting resources
  ([D-032](docs/DECISIONS.md)). The candidate was the branch
  `feat/alpha84-dashboard-self-check`, from `main` at `0447558e9` (the merge of
  the v0.1.0-alpha.83 candidate), pull request #208 into `main` (a draft until
  the day of publication). Its
  commits: `a5830dd3e` (the Dashboard's "System services" card checks by itself:
  once per 300 s at most, only while the tab is visible, no interval timer, no
  retry loop, four distinct states, a held state for a change in progress),
  `f6eaa0b57` (the fourteenth native record, evidence only), `e6de64a5b` (the
  page dates a scan from the moment it received it: defect D1 of that record),
  `1a74eeda5` (the server states a scan's age, `scan_age_seconds`; an
  unreachable Agent answers `503 AGENT_UNREACHABLE` in the scan handler and in
  the admission check of eleven mutation handlers), `fb5ec06f8` (the follow-up
  native record set14b, 2026-10-13, evidence only: no product code), `7024534bc`
  (2026-10-13: the licence check states the time left, `valid_for_seconds`,
  after defect D2 of that record; the typed Agent answer moves into the shared
  error writer and reaches the firewall read and the system check, observation
  O3; the Components page says a failed read could not read the state instead of
  saying an action may have completed, observation O2), `68b79b372` (2026-10-13:
  the read banner's text keeps a readable measure; classes only), `4ed7b1278`
  (documents only), `c76861cde` (2026-10-13: the sixteenth native record,
  set14c, evidence only: no product code), `6e264615f` (documents only),
  `1636b73f0` (2026-10-13: the release preparation: release sequence policy 84,
  bootstrap pins, version lines and three contract tests; no file under
  `cmd/`, `internal/` or `web/`), `aa7dac2b2` (the draft release notes;
  documents only) and `e6ed563ad` (2026-10-13: the seventeenth native record,
  set15, evidence only: no product code). No schema change, no new route
  ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.84.md)).
  - *Measured: the fourteenth native record*
    ([set14](deploy/e2e/release-recovery/evidence/set14-20261012/README.md),
    2026-10-12 UTC; Debian 13 with an English desktop window and Ubuntu 24.04
    with a Turkish phone window, a real headless Chrome; a test-licence build of
    `a5830dd3e` only). Exactly one conditional scan on a stale load, none on a
    fresh one, one more not before 300 s on a page kept visible, none while
    hidden and one when shown, `409 service_operation_busy` shown as "a change
    is in progress" with one follow-up, and with the Agent stopped a `500`
    shown as "Could not check" with the button; the Agent's CPU for a check
    about 0.2 to 0.4 s. Defect D1 (the card said "Not checked in the last 5
    minutes" for 20 to 29 s after every check when the guest's clock was ahead)
    was found there and corrected in `e6de64a5b` and `1a74eeda5`. Its limits:
    Arch not run; `server_setup_busy` and the never-checked state not
    measured; the scan's server-side wall time not measured; the held state
    reachable only by a change that does not scan first. Each intake correction
    of the record narrows its own text and wins over this summary.
  - *Measured: the fifteenth native record, set14b*
    ([set14b](deploy/e2e/release-recovery/evidence/set14b-20261013/README.md),
    2026-10-13 UTC; one disposable Debian 13 guest, a real headless Chrome; a
    test-licence build of `1a74eeda5`). With the guest's clock 1.5 s ahead, a
    stale load sent one conditional scan and the card went straight to the
    result, with no "Not checked in the last 5 minutes" frame (D1 is gone);
    `scan_age_seconds` was a number on every answer that carried a scan. With
    the Agent stopped, the Dashboard (English and Turkish), the Components page
    and a service restart request answered `503 AGENT_UNREACHABLE` with the
    owner's next step and no socket path, nginx unchanged; after the Agent's
    start one press of the button sent one scan. The Components page sent one
    conditional scan. **Defect D2, found in the lab; measured after its fix in
    the sixteenth record:** with the guest's clock 418 s behind, no Panel page
    loaded (the licence check read 19 times in 90 s), so the "clock behind"
    measurement of the scan age was not possible. Not measured: Ubuntu, Arch,
    the window rule (unchanged since the fourteenth record).
  - *Measured: the sixteenth native record, set14c*
    ([set14c](deploy/e2e/release-recovery/evidence/set14c-20261013/README.md),
    2026-10-13 UTC; one disposable Debian 13 guest, a real headless Chrome; a
    test-licence build of `68b79b372`). With the guest's clock seven minutes
    behind and then seven minutes ahead of the browser's, the Dashboard mounted
    in 130 to 192 ms, read the licence access four times in 90 s as with a
    correct clock, sent exactly one conditional scan and showed the result;
    every access answer carried `valid_for_seconds` (42.5 to 60.0 s) and the
    page re-read 15 s before the deadline derived from it (D2 is gone). With
    the Agent stopped, the firewall read, the system check and a restart
    request answered `503 AGENT_UNREACHABLE` with only `code` and `error` and
    no socket path, nginx unchanged, and the Dashboard card said "Could not
    check" (O3). The Components page showed "Component state could not be
    read" / "Bileşen durumu okunamadı" with the Agent sentence and Rescan,
    never the "may have completed" wording; after the Agent's start one press
    sent one unconditional scan and the banner was gone (O2). No defect in the
    changed code. Not measured: Ubuntu, Arch, the "may have completed" banner
    after an action, the guest's NTP settings. Observations for the next
    release: O4, O5, O6 and untranslated mail-stack text (below).
  - *Measured: the seventeenth native record, set15, the pre-publication
    measurement*
    ([set15](deploy/e2e/release-recovery/evidence/set15-20261013/README.md),
    2026-10-13 UTC; disposable guests, a real headless Chrome; test-licence
    builds of `1636b73f0` itself and of a rebuild of the alpha.83 tag's source).
    Fresh installs on Debian 13 (English) and Ubuntu 24.04 (Turkish): version
    v0.1.0-alpha.84, schema 44, bootstrap pins 84 in the archive and in the
    installed copy, no domains and no first-state row, `valid_for_seconds` and
    `scan_age_seconds` present, exactly one conditional scan per stale Dashboard
    load (1126 ms and 1590 ms), the Components page's conditional scan once
    stale (Ubuntu; Debian not measured). The update from the alpha.83 rebuild
    to the candidate on Debian 13, three runs, each `update_verified`, schema 44
    before and after (no migration between the tag and the candidate); the
    Dashboard after the update checked by itself (one scan, "6 / 6"), with the
    Mail stacks card's own "Open Components to rescan" seen for about 1.2 s (the
    known open item, not new). The defective candidate returned by itself
    (`recovered`, `rollback_verified`, two attempts, a reboot at
    `payload_restored`) and the Panel answered as alpha.83. **Open, recorded and
    not judged:** after the return the lab's database comparison found
    `managed_site_files` differing (digests only; the likely cause from the
    source is the Panel rewriting `observed_at` and the other observation
    fields at every start, `internal/services/site_file_ledger.go` lines 263 to
    273, 297 to 300, 327, 391, 426; not confirmed from the rows); and the
    recovery foundation file still names the defective candidate while the
    running build is alpha.83 and the floor reads 84 (the pattern is already
    known from upd2 and upd7). The host slept for about two hours during run c,
    so that run's workload verdict failed for the environment; runs a and b
    passed it. Not run: Arch, the Debian stale-Components load, the readings
    after the defective return. This closes the earlier limit that the release
    preparation was in no measured build. Closes no P0 row.
  - *Component tests on `7024534bc` (`68b79b372` after it changed classes only;
    `npm test` 1339 of 1339):* `go test ./cmd/panel/... ./internal/...` 7028
    passed, 44 skipped, 0 failed; `npm test` 1339 of 1339; `npm run build`
    within budget. D2's fix (`valid_for_seconds`), the Agent answer on the
    firewall and system-check reads (O3) and the Components page's read banner
    (O2) had component tests, and the sixteenth record measured them.
  - *Answered here, not listed before:* the owner's question why the card did
    not check by itself (D-032, `a5830dd3e`); observation O1 of the fourteenth
    record, the generic internal error with the Agent stopped (`1a74eeda5`);
    defect D2 of the fifteenth record, a server clock behind the browser's
    keeping every page from loading, and observations O2 and O3 of the same
    record (`7024534bc`, component-tested, then measured in the sixteenth
    record).
  - *Publication, 2026-10-13 (UTC).* The record of the publication of
    v0.1.0-alpha.83 (pull request #207, documents only) was squash-merged by
    the owner first; `main` became `c1623ff71`. The candidate branch then took
    `main` in (merge commit `dc0ed2747`; the only conflict was the alpha.83
    entry above, resolved to `main`'s text), its pull request #208 left draft,
    and CI on that head passed 24 of 24 jobs. The owner squash-merged #208 into
    `main` as `f0157e6c8f1614cff176821172f8a9067493e625` (tree
    `df896a5b7288110b5418225a1439241f84a8ba49`, equal to the tree of the pull
    request's last commit `dc0ed2747`); CI on that commit (run 38418859503)
    passed 23 of 23 jobs that ran, and the job that publishes assets was
    skipped because it runs only for a tag. The owner set the repository
    variable `CELIKPANEL_RELEASE_SEQUENCE` to 84 before tagging, then created
    and pushed the annotated tag `v0.1.0-alpha.84` (tag object
    `e735308d19fab5166a2f2f4a2860b857ad0e9362`) on that commit. **Process
    note:** the owner pushed the tag while the run on the main commit was still
    in progress (8 of its jobs had finished, none had failed); it completed
    successfully afterwards. The order "the run on main finished, then the
    tag" is the intended one and was not followed this time (corrected
    2026-10-14: an earlier version of this note said the publishing job
    depends only on the packaging job and not on the test jobs; in fact the
    publishing job depends on the packaging job, which depends on every test
    and build job of the same run, so a tag's assets are published only after
    that run passed; the note about the order between the two runs stands);
    the same tree had passed 24 of 24 on the pull request. This is a process
    note, not a defect of the release. The tag's CI run (38419289967)
    completed successfully with 24 jobs and published the release as a
    pre-release at 15:30:58 with six assets. Platform archive
    `celikpanel-v0.1.0-alpha.84-linux-amd64.tar.gz`, 66,131,994 bytes,
    SHA-256 `6dc899e5dabaf471978cde47bfde4a0a649b7c3f6941b1431284c84c9743c301`,
    sequence 84. The assets were checked in a local build guest from a clone at
    the tag (reported by the release preparer, not recorded in a file here):
    both checksum files verify, the two archives are byte-identical, the signed
    manifest (`published_at=2026-10-13T15:04:13Z`) and its Ed25519 signature
    verify against the key in the tag (SHA-256
    `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the
    same as in the archive and as the pin in its `libexec/get.sh`), the
    archive's recorded commit and tree equal the tag's, `install.sh`,
    `libexec/get.sh` and the public key in the archive are byte-identical to
    the tag's files, the content guard and the acceptance-licence guard both
    exited 0, and the bootstrap pins are sequence 84 and v0.1.0-alpha.84. The
    download portal package was built from the tag checkout in pre-signed mode
    (132,639,245 bytes, SHA-256
    `dd1b93d26b4a7d3ebc1a7fd618f75be210a941c66a95b83e28965ec3a30f856f`, built
    twice byte-identical, 54 members, layout identical to the alpha.83
    package); its contract test passed, the public-verifier tests (17) and
    promotion tests (14) were OK in the tag checkout, and a local rehearsal of
    the real promoter against a fixture copy of the alpha.83 site ended
    `committed` after 26 requests with the previous version and backup kept.
    The owner published the portal with `deploy/publish-download-portal.ps1`
    (previous v0.1.0-alpha.83, target v0.1.0-alpha.84); owner-reported: status
    `committed`, the backup `httpdocs.before-v0.1.0-alpha.84.20261013T160559Z…`
    kept on the server, the single public verification pass `status: ok`
    (26 requests, one archive download, 66,671,880 bytes), and the marker
    `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED` printed once. A read-only read of
    the public site from outside at 16:16 UTC (release preparer): `latest.txt`
    is `v0.1.0-alpha.84`; `latest.json` names sequence 84, the commit and the
    SHA-256; the published manifest, signature and checksum file are
    byte-identical to the verified GitHub assets; the archive URL answers 200
    with `Content-Length: 66131994` (not downloaded again); `get.sh` pins
    sequence 84 and v0.1.0-alpha.84; the alpha.83 manifest is still served.
    **Installed panels: not updated when this publication record was written
    on 2026-10-13.** At the same read both installed servers still served the
    interface build of v0.1.0-alpha.83 (their start page references the
    alpha.83 archive's built interface file, not the alpha.84 one). Each owner
    starts the update from the panel's own update screen. **Updated by the
    owner on 2026-10-13, later the same day.** A read from outside at 16:59
    UTC (read-only, HTTPS, no sign-in; nobody but the owner touched the
    servers; the release preparer did not start, watch or verify the update on
    them) found both servers serving the interface build of v0.1.0-alpha.84:
    the same built interface file as `web/dist/index.html` in the verified
    archive, byte-identical (same SHA-256) on each server, with `Last-Modified`
    16:57:37 UTC and 16:57:50 UTC; both still send `Strict-Transport-Security:
    max-age=31536000`. That shows each Panel serves this release's interface;
    it does not show the update's verification result inside the panel, the
    Agent's version, or what the first start did. **Reported by the owner on
    2026-10-13** (two screenshots, English interface, one per server; the
    owner's observation, not acceptance evidence, nothing measured in the lab;
    the owner's message labelled the setup-page screenshot as the first
    server's, but its own sidebar names the second server, so they are recorded
    by what they show): on both servers the sidebar reads `CelikPanel ·
    v0.1.0-alpha.84 · build f0157e6c8f16` and the build id still wraps in the
    middle of the word ("build f01", then "57e6c8f16"), the known open item,
    unchanged. The first server (hosting purpose "Custom setup", no domains)
    lands on the Dashboard; its "System services" card says "5 / 5 running /
    installed", with no "Unknown" and no "Open Components to rescan", and the
    owner did not have to press anything (after the alpha.83 update the same
    card had said "Unknown" and the owner had rescanned by hand); its "Needs
    attention" box says nothing CelikPanel read needs action; a "Mail stacks"
    card says "Partially installed"; no error and no licence page is reported
    for it. The second server lands on the setup page, step 5, "Setup stopped:
    its plan was reopened", unchanged from alpha.83: every step "Completed"
    except the last, "Stopped", none "In progress". That is expected: its
    pending final check needs the reverse (PTR) record to name the mail host,
    and a public-resolver read on 2026-10-13 shows the PTR still names a
    different host name of the owner's domain, so the owner's action at the
    hosting provider is still outstanding; not a defect of this release. Not
    reported: the update's verification result inside the panel, the Agent's
    version, whether the full-screen licence page appeared during the update,
    the `postconf -n >/dev/null` output; both servers have no domains, so the
    site-file adoption of the first start had nothing to adopt and the D-031
    question stays open on installed servers, unchanged from alpha.83.
    Publication is not product acceptance. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.84.md), "Publication
    record" (items 14 and 15 for the installed servers), say who did and who
    reported each step.
  - *Open for the next release, with this release's limits as the release
    notes list them:* the mail-stack card on the Dashboard still has its own
    stale copy and its "Open Components" button; the sidebar's build identifier
    wraps in the middle of a word (the owner's observation after the alpha.83
    update, seen again after the alpha.84 update); Ubuntu (after the changes),
    Arch, `server_setup_busy` and the never-checked state not measured; the "may have completed" banner after an
    action and the guest's NTP settings not measured; the update to this
    candidate was measured on Debian 13 only, from a rebuild of the alpha.83
    tag; the open finding on `managed_site_files` after an automatic return and
    the recovery foundation file naming the defective candidate after it (see
    the seventeenth record above); the lab tools `mock.mjs` and
    `cold-load-set9.mjs`/`cold-load-set11.mjs` still send only `valid_until` on
    the access check; the HSTS retirement in `DomainSSLSettings` still compares
    a server timestamp with the browser's clock; the Agent answer's "nothing was
    changed" assumes no earlier change in the same request (the joined-error
    guard covers aggregate errors; a handler that changes something and later
    fails on a separate unjoined dial would still claim it); observation O4 of
    the sixteenth record (the Turkish Components page shows the firewall
    banner's server sentence in English, `ServiceList.tsx:3259-3271`, `:3286`,
    `:3393`), O5 (the firewall banner is read only on mount,
    `ServiceList.tsx:3289-3291`, `:3318`), O6 (the Dashboard's "Needs attention"
    box does not name the Agent, `Dashboard.tsx:895`, `:1163-1165`; all lines at
    `68b79b372`) and untranslated mail-stack text on the Turkish Components
    page; and every limit of the alpha.83 candidate above, which this release
    does not address.
  - No P0 item is closed or advanced by this release or by its publication.
    P0.1–P0.5 stay **partial**. Publishing it did not update any installed
    panel; only the owner starts that, from the panel's own update screen.
- **v0.1.0-alpha.85 is published, 2026-10-15; the owner has updated both
  installed servers (report of the same day).** Written 2026-10-15 as a candidate and amended for its publication the
  same day. The
  number v0.1.0-alpha.85 was chosen by the release preparer under the owner's
  standing instruction of 2026-10-13 (release preparation `5443fe055`). Owner decision
  of 2026-10-14, after the final setup step on the second installed server: a
  screen that makes the owner wait shows that it waits, or says that nothing
  needs to be done, and ends with an honest "not known yet" after a bound
  ([D-033](docs/DECISIONS.md)). The candidate was the branch
  `fix/alpha85-setup-finish-wait`, from `main` at `e3c8f735a` (the record of
  the publication of v0.1.0-alpha.84). Its product commits: `a94f83cb0` (the
  Agent asks the next resolver when the first reverse DNS answer is stale),
  `febbe09e3` (setup finishes on the owner's passing re-check,
  `GET /api/v1/setup?check=1&manual=1`), `c714238d6` and `e358ebe3c` ("Finishing
  setup" shown as active and bounded, "Waiting for reverse DNS", no Edit plan
  while finishing), `2e273cc55` to `1c5454c53` (the waiting-state audit's
  sixteen changes: setup reconnecting, the Components firewall banner, O4 and
  O5 of alpha.84, the Dashboard's attention notice, O6, the Mail stacks card,
  typed mail profile sentences, the server-change pauses, spinners on long
  requests, the HSTS time left `hsts_retire_remaining_seconds`, the sidebar
  build id, the lab mock's `valid_for_seconds`), `8e68b3ce2`, `3abbe3f76` and
  `0ad26074d` (the audit's rows 16, 25 and 35, decided by the owner the same
  day), and `8dfc3ff69`, `733b798db`, `795f31674` and `89cf88397` (the
  corrections from the screenshot round). Besides: `8b09e4e61` (a correction
  of the alpha.84 records about the publishing job), `a87ce0b97` (the
  release-recovery probe records `managed_site_files` rows and a stable
  digest), `0b43ee81e` (the twelfth browser-inspection batch), `a945a4dbf`
  and `e3feee06d` (the set16 harness and the eighteenth native record,
  evidence only), and documents. No schema change, no new route
  ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.85.md)).
  - *Measured: the eighteenth native record, set16, the pre-publication
    measurement*
    ([set16](deploy/e2e/release-recovery/evidence/set16-20261014/README.md),
    2026-10-14 UTC; disposable QEMU/KVM guests, a real headless Chrome;
    test-licence builds of the fixture commit `d5d673eb` (`89cf88397` plus the
    sequence policy) and of a local rebuild of the alpha.84 tag, which is not
    byte-identical to the published archive). A fresh install on Debian 13:
    version v0.1.0-alpha.85, schema 44. The update from alpha.84 on Debian 13
    and Ubuntu 24.04, each `update_verified`, floor 85, ledger 44, Domains one
    row `managed_unchanged`. The defective candidate returned by itself on
    both (`recovered`, `rollback_verified`, two automatic attempts), and the
    ledger after the return was `equal-except-volatile` with only
    `observed_at` changed in `managed_site_files`, the stable digest equal:
    this answers the alpha.84 open finding for this fixture. In the browser:
    the Dashboard sent one conditional scan per load in every lab and, on the
    updated candidates, its Mail stacks card went checking → "Ready" with no
    "Open Components to rescan";
    with the guest clock about ±90 s off the page mounted every time; with the Agent
    stopped (Debian only) the Turkish firewall banner spoke Turkish on the
    attention surface and read "Güvenlik duvarı: Açık" after one Rescan (O4,
    O5), and "Needs attention" named the Agent (O6); the sidebar build id stood
    on one line at 1280×800 and 390×844. Not reachable or not measured:
    "Finishing setup" (no lab guest has a reverse DNS name), the update card's
    busy state, Arch, a fresh Ubuntu install, the Mail stacks card's held
    state. Observations O1–O3 (the page language attribute, an English
    firewall engine name, raw audit action ids), pre-existing and not judged.
    No defect of the candidate observed. Closes no P0 row.
  - *Component tests:* `go test ./cmd/panel/...` 1542 passed, 6 skipped, 0
    failed (at `1c5454c53`; no Go changed after it); `go test ./cmd/agent/...`
    1798 passed, 29 skipped, 0 failed under a clean mount namespace in the WSL
    guest (with `a94f83cb0` to `e358ebe3c`); `npm test` 1391 of 1391 and `npm run build`
    within budget (re-run on 2026-10-15 at `5443fe055`); the three contract
    tests of the release preparation pass. The twelfth browser-inspection batch:
    31 states in 8 configurations against the mock, its defects corrected
    before set16.
  - *Answered here, not listed before:* the owner's report from the second
    installed server (setup finishing, the reverse DNS wait); D-033 and its
    audit.
  - *Answered here from the alpha.84 list:* O4, O5 and O6; the Mail stacks
    card's stale copy; untranslated mail profile text on the Turkish Components
    page; the sidebar build id wrapping mid-word; the HSTS retirement comparing
    two clocks; the lab tools sending only `valid_until`; the
    `managed_site_files` finding after an automatic return (for set16's
    fixture).
  - *Publication, 2026-10-15 (UTC).* The owner squash-merged pull request #210
    into `main` as `36c33c4531ee6c1306d324db7a666403176f3bf7` (tree
    `c451c26de3cb179251e8dfd4b033ec7a9e18005e`); CI on the pull request: 23
    jobs succeeded, 1 skipped (the publish job, tag only). The run on the main
    commit (38685686782) first concluded failure: 2 jobs ("Platform portability
    - Linux arm64 compile only" and "Panel race - Server setup") failed before
    any step ran, both with GitHub's error "An action could not be found at the
    URI https://codeload.github.com/actions/setup-go/tar.gz/924ae3a1…" (the
    download of `actions/setup-go@v6`). The release preparer re-ran the failed
    jobs; the run concluded success, 24 jobs, 0 failed. The tag was pushed only
    after that, so the order "the run on main finished, then the tag" was
    followed this time. The owner set the repository variable
    `CELIKPANEL_RELEASE_SEQUENCE` from 84 to 85 and created and pushed the
    annotated tag `v0.1.0-alpha.85` (tag object
    `c491a103f2f6913f58e5ba418a3361188ef11dde`). The tag's CI run (38688902643)
    completed successfully with 24 jobs and published the release as a
    pre-release at 2026-10-15T06:10:06Z with six assets. Platform archive,
    byte-identical to the generic archive, 66,146,417 bytes, SHA-256
    `cb6424b712052df2768f6c16dd6a66c8ea05a13393e71933c38ae639c3fb0792`,
    sequence 85. The assets were checked by the release preparer from a
    download (`gh release download`; reported by the preparer, not recorded in
    a file here): both `.sha256` files verify; the manifest (332 bytes:
    sequence 85, version v0.1.0-alpha.85, commit `36c33c45…`, `published_at`
    2026-10-15T05:14:06Z, linux/amd64) and its 64-byte Ed25519 signature verify
    with `openssl pkeyutl -verify -rawin -pubin` against the tracked
    `deploy/release-signing-ed25519.pem` (SHA-256
    `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the
    same as alpha.84's); the archive has 234 members, its `libexec/get.sh` pins
    85 / v0.1.0-alpha.85, its `release-sequence-policy` says 85 with previous 84
    `f0157e6c8f1614cff176821172f8a9067493e625`, its `release.commit` is the main
    commit, its interface bundle is `web/dist/assets/app-Br53Jd-B.js` (242,703
    bytes, SHA-256
    `33930d92e988c87ad677de54a3e2ef5c4b53361b64c777686eb4f0121806eda4`;
    alpha.84's was `app-CX7VTgkx.js`), and the content guard and the
    acceptance-licence guard both returned 0. Limit: the tag's files were taken
    with `git archive` on Windows, not from a clone in a build guest. The
    download portal package was built pre-signed with
    `deploy/build-download-portal.sh` (no private key used): 132,665,325 bytes,
    SHA-256 `80358e4b84551778906833a9750bff2ab66a79ddfca0653668a7ae2565c8d69b`,
    54 members, built twice byte-identical; `releases/latest.txt`
    v0.1.0-alpha.85; the packaged `get.sh` (SHA-256
    `9b0cf04f0cd52578d837c37c41cea5adf19d81b0eb8d8b2fdbd2c1aeaf598d2f`) equals
    the tag's. No promoter rehearsal against a copy of the live site was made.
    The portal was published by the release preparer, not the owner, under the
    owner's instruction of 2026-10-15 ("bunları neden sen yapmıyorsun… benden
    istiyorsun"; from this release on the merge, the repository variable, the
    tag and the portal publication are the preparer's, and installed-panel
    updates remain the owner's only), with `deploy/publish-download-portal.ps1`
    (`-PreviousVersion v0.1.0-alpha.84 -TargetVersion v0.1.0-alpha.85
    -TransferTimeoutSeconds 1800`). The first invocation, with a relative script
    path, stopped at parameter binding before any transfer (`Join-Path : Cannot
    bind argument to parameter 'Path' because it is an empty string`, line 19:
    `$PSScriptRoot` is empty in a parameter default). The second, with the
    script's absolute path and explicit `-Promoter` and `-PublicVerifier`,
    succeeded: promoter `status: committed`, previous v0.1.0-alpha.84, backup
    `httpdocs.before-v0.1.0-alpha.85.20261015T062427Z…`, public verification
    `status: ok` (26 requests of a limit of 26, one archive GET of 66,686,303
    bytes), `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`. A read from outside at
    06:24:40Z: `releases/latest.txt` is v0.1.0-alpha.85; `get.sh` is served with
    `Strict-Transport-Security: max-age=15768000; includeSubDomains` and pins
    85; `releases/latest.json` names sequence 85, commit `36c33c45…` and SHA-256
    `cb6424b7…`. **Installed panels: not updated when this record was
    written; updated by the owner afterwards.** Only the owner starts that, from
    the panel's own update screen, and did so for both servers (six
    screenshots and a message of 2026-10-15, received about 07:00Z; recorded
    09:58Z). Both show Current version v0.1.0-alpha.85, commit `36c33c45…`, and
    the sidebar build id on one line. Owner observations, an outside read and not
    acceptance evidence: during Frankfurt's update the page showed a
    warning-coloured "could not be read just now, so it is not known whether it
    has finished" notice and a toast saying the same, inside the update's own
    expected restart; the owner finds it reads like a problem when nothing is
    wrong. **Open for the next release (D-033, D-024):** calm progress with the
    T+ counter during the expected restart window; the warning surface and the
    "not known" wording only after the window passes or on a verified failure;
    one notice instead of a banner plus a toast (code to look at:
    `web/src/components/SystemUpdateOperation.tsx`,
    `web/src/components/PanelUpdateCard.tsx`). Boston's Dashboard after the
    update read 5 / 5 services, the typed partial mail-profile text and "Nothing
    CelikPanel read needs action"; its Needs attention had failed a read before
    the update (alpha.84), cause not known. Frankfurt's Dashboard showed the
    "Checking…" states right after the update; the result was not captured.
    "Recent activity" still shows raw audit ids (set16 O3). Not reported, so not
    recorded: the full-screen licence page during the update, the Agent's
    version, Frankfurt's setup final step after the update, and the output of
    `postconf -n >/dev/null`. Lab-environment note, not a product fact: the lab
    host lost KVM after Windows update KB5127648 (AMD; microsoft/WSL issues
    #41917 and #41918) and the owner removed that update to run set16.
    Publication is not product acceptance. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.85.md), "Publication
    record", say who did and who reported each step.
  - *Open for the next release, with this release's limits as the release
    notes list them:* `deploy/publish-download-portal.ps1` fails with a relative
    script path because its parameter defaults use `$PSScriptRoot` (compute them
    in the script body); no installed server measured; "Finishing setup" and the
    update card's busy state not reached natively (mounted tests and the mock
    browser only), nor the component lock's reconnect, the spreading-domain
    checks and the HSTS time left; the resolver that answered is not named;
    the next-resolver change takes effect only after an Agent update; set16 ran
    a fixture commit, so the release preparation `5443fe055` (pins 85) is in no
    measured build; the alpha.84 baseline was a local build, not byte-identical
    to the published archive; the recovery foundation file names the defective
    candidate after a return (set12, set15, set16); set16's O1–O3; templated
    text (`component_not_installable`, roles, "rescan" outside the mail
    profiles); Arch; and every limit of the alpha.84 release above that this
    release does not address.
  - No P0 item is closed or advanced by this release or by its publication.
    P0.1–P0.5 stay **partial**; set16 is a measurement, not acceptance.
    Publishing it did not update any installed panel; only the owner starts
    that, from the panel's own update screen.

- **v0.1.0-alpha.86 is published, 2026-10-15; no installed server is updated
  yet.** Written 2026-10-15 as a candidate, amended the same day for the native
  records set17 and set17b and for its publication. The
  number v0.1.0-alpha.86 was chosen by the release preparer under the owner's
  standing instruction of 2026-10-13 (release preparation `313f3eea2`, sequence
  86 after 85, with the alpha.85 release commit `36c33c45…` as previous). It
  started with the owner's observation of 2026-10-15 during the real update of
  the second installed server to alpha.85: while the panel restarted as the
  update's plan says it will, the update page showed a warning-coloured "could
  not be read just now, so it is not known whether it has finished" notice and
  a toast saying the same with T+02:59; in substance, the owner's words: this
  looks like a problem although nothing is wrong. D-033 and D-024 apply; no
  new decision is recorded (a dated note under D-033 lists this candidate's
  application of it). The owner's reply "devam. sorun yok." of 2026-10-15
  decided that this release ships the items in hand and the second-server work
  follows. The candidate was the branch
  `fix/alpha86-update-restart-notice`, from `main` at `36c33c453`. Its product
  commits: `8e27dd0eb` and `15d80851e` (the update's own restart shown as calm
  progress inside a bound of 6 minutes from the start and at least 2 minutes
  from the first lost read, and one notice instead of two), `f89551e9b` (the
  publishing script resolves its helper-script defaults in the body),
  `0082c9d08` and `1abe1717d` (reads only while the tab is visible, and a read
  at once when it is shown again), `d2c0add01` (the page's language
  attribute, set16 O1), `151969774` and `ef7690fc0` (the 27 component names
  and descriptions in the interface language, O2), `8a5f299fa`, `8a3729cbe`
  and `7956001d0` (179 audit actions as sentences, with a Go test that derives
  the list from the source, O3), `4372fc86e` and `8fe746582` (the `system`
  actor said "System", and `system` reserved as a username,
  `USERNAME_RESERVED`), `cd2275e47`, `d2b58e7e9`, `297f1f0b1`, `00138c530`,
  `a6bdf410b` and `21f07e7a8` (no sentence asks for a rescan; the Dashboard,
  the Components page and a component's own page check an unverifiable record
  by themselves, and a held check resumes by itself), `80d763949`, `0a81043cf`
  and `370afa2c9` (the platform in `component_not_installable`, role names and
  plural requirements in mail profiles), `e2c630faa` (the Agent names the
  resolver behind the reverse DNS answer, `reverse_dns_resolver` and
  `reverse_dns_resolvers_disagree`), and `1f564fdb0`, `01861bfc9` and
  `e2853c873` (the Store's "State not known yet" and corrections from the
  fourteenth browser batch), and, after the native records, `e8aa9c5c0` (set17
  D1: the licence gate keeps its last positive decision through the update's
  restart), `b7b0f4766` (the session gate lets the update's restart own
  `503 PANEL_STARTING`) and `7265d3c99` (set17b F1: the navigation lock covers
  the restart, at the latest 6 minutes from the start; the release preparer's
  decision of 2026-10-15 under D-033 and the owner's standing instruction of
  2026-10-13, not an owner decision), all three web only. Besides: `a45c1bfcb`
  (browser batch 14;
  batch 13 is in `15d80851e`), `59e793b2b`, `389ce2831` and `155a984ea`
  (browser batch 15), `4a29f4754`, `0bf1ee601` and `1f9495f7b` (the native
  records set17 and set17b with their harness), `d72e4528a` (their intake
  corrections), `498dc49e1` (a lab test race, merged from `main`), and
  documents. No schema change, no new route
  ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.86.md)).
  - *Measured:* two native records on earlier commits of the candidate,
    QEMU/KVM guests with test-licence builds, not acceptance evidence:
    [set17](deploy/e2e/release-recovery/evidence/set17-20261015/README.md) at
    `313f3eea2` (fresh install on Debian 13; the update from alpha.85 good
    and defective on Debian 13 and Ubuntu 24.04, `update_verified` and
    `recovered`/`rollback_verified`, ledger `equal-except-volatile`; the
    candidate's own update page twice) and
    [set17b](deploy/e2e/release-recovery/evidence/set17b-20261015/README.md)
    at `ecd2a5f08` (Debian 13; a restart outage of about 3 minutes made by a
    lab delay, a lab change and not a product property). Outcome: the
    candidate's update page showed the calm restart, with 0 samples of the
    "not known" wording or the warning surface, in all five candidate runs
    (two in set17, three in set17b), where alpha.85's page showed both in all
    three control runs; set17 recorded D1 (the licence gate's dialog
    over the restart in one of two runs), not reproduced in set17b's two runs
    on a candidate carrying its fix (mechanism not established); set17b
    recorded F1 (the lock ended at T+120 s mid-outage), changed afterwards in
    `7265d3c99`, which is not measured natively. `PANEL_STARTING` never
    reached the page; the 6-minute overdue wording was not produced. The
    READMEs are the source of truth; the release notes summarise them.
  - *Component tests:* `go test -v ./cmd/panel/... ./internal/transport/...
    ./internal/auth/...` 1602 top-level tests passed, 6 skipped, 0 failed (at
    `8fe746582`, and again at `21f07e7a8` with the same counts);
    `go test ./cmd/agent/...` 1799 passed, 29 skipped, 0 failed under a clean
    mount namespace in the WSL guest (at `1abe1717d`; nothing the Agent builds
    changed after it); no Go file changed after `21f07e7a8`, so Go was not run
    again after the web-only corrections; `npm test` 1463 of 1463 (at
    `00138c530`, and again at `313f3eea2`), then 1483 of 1483 at `7265d3c99`
    (and again at `155a984ea`), and `npm run build` within budget; the three
    contract tests of the release preparation pass. Browser batch 13: three
    states of the restart notice in 8 configurations against the mock; batch
    14: 23 states in 8 configurations, six defects corrected after it, and a
    seventh (the bundle budget) found by `npm run build`; batch 15: the
    licence gate, the session gate's `PANEL_STARTING` and the lock through a
    restart, in 8 configurations.
  - *Answered here from the alpha.85 list and the owner's report:* the
    problem-looking notice during the expected restart (c); the publishing
    script's `$PSScriptRoot` defect; the install dialog's and setup's reads not
    gated on visibility; set16's O1–O3 and raw audit ids on both installed
    servers (h); templated text (`component_not_installable`, roles, "rescan"
    outside the mail profiles); the resolver that answered (in source; needs an
    Agent update). All in source with component tests; none measured natively.
  - *Publication, 2026-10-15 (UTC).* All of it by the release preparer, under
    the owner's instruction of 2026-10-15 (the merge, the repository variable,
    the tag and the portal publication are the preparer's; installed-panel
    updates remain the owner's only). Pull request #213 (CI: 24 jobs, all
    success or skipped) was squash-merged into `main` as
    `8d260f317e58bed7fcd84c6cbb01823c4fe3f166` (tree
    `5aaee3d01a4c80632ee3e1abec2c11c986da90df`). The run on the main commit
    (38795120470) concluded success, 24 jobs; the tag was pushed only after it
    finished, so the order "the run on main finished, then the tag" was
    followed. The preparer set the repository variable
    `CELIKPANEL_RELEASE_SEQUENCE` from 85 to 86 and created and pushed the
    annotated tag `v0.1.0-alpha.86` (tag object
    `8399668a3fc600ed3951c1c3c22739b3f4ffa35d`). The tag's CI run
    (38797990342) completed successfully with 24 jobs and the release was
    published at 2026-10-15T20:18:29Z as a pre-release with six assets.
    Platform archive, byte-identical to the generic archive, 66,159,282 bytes,
    SHA-256 `5949a0fd36f8a9f9f0308749962ec92c641df0e8cdbce72ed6b6316578a16a7d`,
    sequence 86. The assets were checked by the preparer from a download
    (`gh release download`; checks in the `CelikPanel-S2-Debian` guest; reported
    by the preparer, not recorded in a file here): both `.sha256` files verify;
    the manifest (332 bytes: format v2, sequence 86, version v0.1.0-alpha.86,
    commit `8d260f31…`, `published_at` 2026-10-15T19:35:12Z, linux/amd64, the
    archive's SHA-256 and size matching) and its 64-byte Ed25519 signature
    verify with `openssl pkeyutl -verify -rawin -pubin` against the tracked
    `deploy/release-signing-ed25519.pem` (SHA-256
    `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the
    same as alpha.85's); the archive has 237 members, its `libexec/get.sh` pins
    86 / v0.1.0-alpha.86 (SHA-256
    `0a71748503e8e8b67766577a269379eb48f92da968f1daf00f5f6c47a286dbf7`), its
    `release-sequence-policy` says 86 with previous 85
    `36c33c4531ee6c1306d324db7a666403176f3bf7`, its `release.commit` and
    `release.tree` equal the tag's, the binaries are go1.26.5 trimpath
    CGO_ENABLED=0, the guards returned 0, and its interface bundle is
    `web/dist/assets/app-DAGE4avW.js` (248,116 bytes, SHA-256
    `262f8434444ad325249c8949b34f9d4cd3ee7bc2839aa088986ad8a166dba33c`;
    alpha.85's was `app-Br53Jd-B.js`). Limits: the tag's files were taken with
    `git archive` on Windows, not from a clone in a build guest; the archive was
    not compared file by file with alpha.85's package. The download portal
    package was built pre-signed with `deploy/build-download-portal.sh` (no
    private key used): 132,691,628 bytes, SHA-256
    `a36496cdca0547609b12e2aaa0d103127d5ea3ed9cebd976a857dc865151e72b`, 54
    members, built twice byte-identical; `releases/latest.txt` v0.1.0-alpha.86;
    the promoter's package rules accepted it; at the tag the portal contract
    test returned 0, the public-verifier tests passed 17 of 17 and the promotion
    tests 14 of 14. No promoter rehearsal against a copy of the live site was
    made and `deploy/test-download-portal-publisher.ps1` was not run. The
    portal was published with `deploy/publish-download-portal.ps1` (absolute
    path, explicit `-Promoter` and `-PublicVerifier`, `-PreviousVersion
    v0.1.0-alpha.85 -TargetVersion v0.1.0-alpha.86 -TransferTimeoutSeconds
    1800`): `CELIKPANEL_PORTAL_UPLOAD_READY`, `CELIKPANEL_PORTAL_UPLOAD_PINNED`,
    promoter `status: committed`, previous v0.1.0-alpha.85, backup
    `httpdocs.before-v0.1.0-alpha.86.20261015T202726Z…`, public verification
    `status: ok` (26 of 26 requests), `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`;
    that invocation, with the absolute path, did not exercise the correction of
    the relative-path defect. A read from outside at 20:27:35Z:
    `releases/latest.txt` is v0.1.0-alpha.86 (it read v0.1.0-alpha.85 at
    20:24:59Z, before the publication); `get.sh` is served with
    `Strict-Transport-Security: max-age=15768000; includeSubDomains` and pins
    86; `releases/latest.json` names sequence 86, commit `8d260f31…` and SHA-256
    `5949a0fd…`. **Installed panels: not updated when this record was
    written.** Only the owner starts that, from the panel's own update screen.
    To be reported by the owner and not recorded until then: both servers'
    update result and what the update page showed during the restart (the calm
    notice, no gate dialog, the lock), the full-screen licence page during the
    update, the Agent's version, Frankfurt's setup final step after the update,
    and the output of `postconf -n >/dev/null`. Publication is not product
    acceptance. The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.86.md),
    "Publication record", say who did and who reported each step.
  - *Open for the next release, with this release's limits as the release
    notes list them:* the
    native records measured `313f3eea2` and `ecd2a5f08`, so the F1 lock
    extension of `7265d3c99` (the lock kept past two minutes until the panel
    answers, and its changed line) is not measured natively; the session
    gate's `PANEL_STARTING` path was not exercised natively; the 6-minute
    overdue wording was not produced; Ubuntu 24.04 and Arch were not run in
    set17b, Arch and a fresh Ubuntu install in neither; the 6-minute bound
    rests on lab updates and one owner observation; the new
    update screen is seen only from the update after this one; the resolver
    sentence needs an Agent update; the audit log stores no actor kind; no
    installed server measured; and every limit of the alpha.85 release above
    that this release does not address ("Finishing setup" and the update
    card's busy state not reached natively, the recovery foundation file after
    a return, Arch, what the owner did not report after the alpha.85 update).
  - No P0 item is closed or advanced by this release or by its publication.
    P0.1–P0.5 stay **partial**. Publishing it did not update any installed
    panel; only the owner starts that, from the panel's own update screen.

- **v0.1.0-alpha.87 is published, 2026-10-16; no installed server is reported
  updated yet.** Written 2026-10-16 as a candidate and amended the same day for
  its publication. The number
  v0.1.0-alpha.87 was chosen by the release preparer under the owner's standing
  instruction of 2026-10-13 (release preparation `471377d60`, sequence 87
  after 86, with the alpha.86 release commit `8d260f31…` as previous). It is
  part 1 of the owner's goal that the second installed server (Boston, the DNS
  secondary) host websites and mail as a full peer of the first (Frankfurt,
  the DNS primary); the owner's reply "devam. sorun yok." of 2026-10-15 put
  this work after alpha.86. A read-only investigation at main `e3c8f735a` (the main of 2026-10-13) found
  eight gaps; the five that need no owner decision are corrected, and three
  owner decisions stay open for part 2: whether the primary's own setup
  carries the peer's mail host name, an editor for the infrastructure zone on
  the primary, and whether host records travel over the remote DNS connection
  automatically. Until then the record of Boston's mail host name is made by
  the owner on Frankfurt or at the DNS provider. D-021 and D-033 apply; no new
  decision is recorded (dated notes under both). The choices the release preparer
  made for part 1 (listed in the release notes, among them the "Custom setup"
  label of set18b O5) were made under D-021, D-033 and the owner's standing
  instruction of 2026-10-13; none is an owner decision, and the owner may confirm
  or change them. The candidate was the branch
  `feat/alpha87-second-server` (pull request #215), from `main` at the
  alpha.86 release commit `8d260f317`. Its product commits: `99e90ac74`
  (`POST /api/v1/setup/extend`, the extension marker and base, the state
  fields `extension` and `extension_base`), `eddc11610` (a corrected mail host
  name is saved only once its operation is admitted), `60ea75c1c` (on a DNS
  secondary the mail box names the primary and the exact record,
  `mail_dns_guidance`), `f51d98e4b` ("Extend this server's setup" from the
  ready screen and Settings, the review's three blocks, "Stop extending"; one
  typed sentence per open check), `450af0d88` and `32f0a3766` (Components
  mail through setup on a prepared server, the mail host name "Change", the
  secondary box), `c40f14f87` and `b67987865` (seven corrections from browser
  batch 16), and `6b363ac46` (set18 D1: an extension no longer opens on the
  completed run, additive `superseded`; O1: a started run names its first
  step; O3: a pressed continuation stays single-flight). Besides: `343d3eb9e`
  (the operation guidance entry), `5d54aba1e` (browser batch 16),
  `ddea67344` and `0be5c8e5a` (the native records set18 and set18b with their
  harness), `90b3fe2f2` (their intake corrections), the merge of `main`
  (`bef12820e`) and documents. No schema change (44; the extension base is one
  `panel_settings` row); nothing under `internal/` or `cmd/agent/` changed
  ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.87.md)).
  - *Measured:* two native records, QEMU/KVM guests on Debian 13 with
    test-licence builds and, for the first time in this evidence folder, a lab
    DNS responder and a lab ACME server (pebble) inside the guest, outbound
    443 refused except to the package hosts, port 25 answered by the guest's
    own Postfix; not acceptance evidence:
    [set18](deploy/e2e/release-recovery/evidence/set18-20261015/README.md) at
    `5d54aba1e` (setup to "Your server is ready" through the public DNS check
    and the trusted panel certificate; the Components mail route; the
    extension adding Core Mail to ready through the mail record wait, the mail
    certificate, the reverse DNS sentence naming 1.1.1.1 and "Finishing
    setup"; D1 recorded, the extension opening on the completed run) and
    [set18b](deploy/e2e/release-recovery/evidence/set18b-20261016/README.md)
    at `6b363ac46` (the same flow on screen with every step pressed in the
    page; D1, O1 and O3 observed fixed; no new defect; O5, the purpose label
    "Custom setup" after adding mail, recorded and left as it is). The
    READMEs are the source of truth; the release notes summarise them.
  - *Component tests:* `npm test` 1499 of 1499 (after the set18 corrections,
    and again at `bef12820e`); `go test -count=1 -v ./cmd/panel/...` 1558
    top-level tests passed, 6 skipped, 0 failed, 1675 subtests passed, at
    `bef12820e` (the same Go code as `6b363ac46`; the release preparer's measurement in the WSL
    guest, not recorded in a file), `go vet` and gofmt clean;
    the Agent was not run (unchanged). Browser batch 16: 36 states in 8
    configurations against the mock, seven defects corrected. The release
    preparation's sequence-policy contract passes ("86 -> 87"); the portal and
    signed-manifest contracts pass in the WSL guest (the release preparer's report,
    not recorded in a file).
  - *Publication, 2026-10-16 (UTC).* All of it by the release preparer, under
    the owner's instruction of 2026-10-15 (the merge, the repository variable,
    the tag and the portal publication are the preparer's; installed-panel
    updates remain the owner's only). Pull request #215 (CI: 24 jobs, all
    success or skipped) was squash-merged into `main` as
    `a8cddcb546302daa0f5eb65b18fe6b9566b69e66` (tree
    `c63a9600572cba0dc6bd19ecb0388acb45b395c4`). The run on the main commit
    (38834849023) concluded success, 24 jobs; the tag was pushed only after it
    finished, so the order "the run on main finished, then the tag" was
    followed. The preparer set the repository variable
    `CELIKPANEL_RELEASE_SEQUENCE` from 86 to 87 and created and pushed the
    annotated tag `v0.1.0-alpha.87` (tag object
    `f277e0b469acd3a33b9fb87c3327566ef1c2ba04`). The tag's CI run
    (38836331003) completed successfully with 24 jobs and the release was
    published at 2026-10-16T01:57:38Z as a pre-release with six assets.
    Platform archive, byte-identical to the generic archive, 66,182,207 bytes,
    SHA-256 `d7b33548d431c05f99e2d98d3052692647a3c662c9cb57d60fb03bf5e5a91d60`,
    sequence 87. The assets were checked by the preparer from a download
    (`gh release download`; checks in the `CelikPanel-S2-Debian` guest; reported
    by the preparer, not recorded in a file here): both `.sha256` files verify;
    the manifest (332 bytes: format v2, sequence 87, version v0.1.0-alpha.87,
    commit `a8cddcb5…`, `published_at` 2026-10-16T01:23:21Z, linux/amd64, the
    archive's SHA-256 and size matching) and its 64-byte Ed25519 signature
    verify with `openssl pkeyutl -verify -rawin -pubin` against the tracked
    `deploy/release-signing-ed25519.pem` (SHA-256
    `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the
    same as alpha.85's and alpha.86's); the archive has 237 members, its
    `libexec/get.sh` pins 87 / v0.1.0-alpha.87 (SHA-256
    `7c8823849cb2fb310b4a58d178364c9ed4d89c4d94908f0cf0856df3fb577b5d`), its
    `release-sequence-policy` says 87 with previous 86
    `8d260f317e58bed7fcd84c6cbb01823c4fe3f166`, its `release.commit` and
    `release.tree` equal the tag's, the binaries are go1.26.5 trimpath
    CGO_ENABLED=0 with no acceptance tag, the guards returned 0, and its
    interface bundle is `web/dist/assets/app-D8EjyWzW.js` (248,119 bytes,
    SHA-256 `b86955f5b8d4f1b3f0af15612c983b118037390e1cc01615cafa1ace4a184cb1`;
    alpha.86's was `app-DAGE4avW.js`). Limits: the tag's files were taken with
    `git archive` on Windows, not from a clone in a build guest; the archive was
    not compared file by file with alpha.86's package. The download portal
    package was built pre-signed with `deploy/build-download-portal.sh` (no
    private key used): 132,735,450 bytes, SHA-256
    `d7ec2b4940457d0b9bbe717d50506fd343ebed869b1f441348d670f35dbcf88c`, 54
    members, built twice byte-identical; `releases/latest.txt` v0.1.0-alpha.87;
    the promoter's package rules accepted it; at the tag the portal contract
    test returned 0, the public-verifier tests passed 17 of 17 and the promotion
    tests 14 of 14; the four portal scripts are unchanged since alpha.86. No
    promoter rehearsal against a copy of the live site was made and
    `deploy/test-download-portal-publisher.ps1` was not run. The portal was
    published with `deploy/publish-download-portal.ps1` (absolute path, explicit
    `-Promoter` and `-PublicVerifier`, `-PreviousVersion v0.1.0-alpha.86
    -TargetVersion v0.1.0-alpha.87 -TransferTimeoutSeconds 1800`):
    `CELIKPANEL_PORTAL_UPLOAD_READY`, `CELIKPANEL_PORTAL_UPLOAD_PINNED`,
    promoter `status: committed`, previous v0.1.0-alpha.86, backup
    `httpdocs.before-v0.1.0-alpha.87.20261016T020434Z…`, public verification
    `status: ok` (26 requests), `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`. A read
    from outside at 02:04:42Z: `releases/latest.txt` is v0.1.0-alpha.87 (it read
    v0.1.0-alpha.86 at 02:02:54Z, before the publication); `get.sh` is served
    with `Strict-Transport-Security: max-age=15768000; includeSubDomains` and
    pins 87; `releases/latest.json` names sequence 87, commit `a8cddcb5…` and
    SHA-256 `d7b33548…`. **Installed panels: not reported updated when this
    record was written.** Only the owner starts that, from the panel's own
    update screen. Alpha.87 came about 5.5 hours after alpha.86 and before the
    owner had reported updating any server to alpha.86, so the alpha.86
    observations are also still open; the updater's sequence floor admits an
    update from alpha.85 or alpha.86 straight to alpha.87 (the policy's
    `previous_commit` is a build-contract pin, not an install precondition;
    read from `cmd/agent/system_update_contract.go` and the absence of a
    `previous_commit` check under `cmd/`, not a measured update). To be
    reported by the owner and not recorded until then: both servers' update
    result (from whichever version), what the update page showed during the
    restart (on an update from alpha.86 the calm restart notice should be seen
    for the first time on an installed server; on an update from alpha.85 the
    page shown is alpha.85's), the full-screen licence page, the Agent's
    version, Frankfurt's setup final step, `postconf -n >/dev/null`, and whether
    "Extend this server's setup" appears on Boston's ready screen. Publication
    is not product acceptance. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.87.md), "Publication
    record", say who did and who reported each step.
  - *Open for the next release, with this release's limits as the release
    notes list them:* part 2 (three owner decisions); the secondary-DNS boxes
    not reached natively; the resolvers-disagree sentence and the slow
    "Finishing setup" variant not produced; a lab, not the public ACME and
    DNS path; Debian 13 only; observed in the raw files and not judged: a
    "not a valid FQDN" journal line at each Panel start on a web-only setup,
    `cron DEATH` lines, a "self-signed fallback" toast while a trusted mail
    certificate followed (a candidate D-033 item), and the licence gate's
    "latest access check failed" banner around setup's own planned restart
    (not covered by alpha.86's update-restart grace); O2 and O4 not judged,
    O5 left as it is by the release preparer's decision; no installed server
    measured; and every limit of the alpha.86
    release above that this release does not address.
  - No P0 item is closed or advanced by this release or by its publication.
    P0.1–P0.5 stay **partial**. Publishing it did not update any installed
    panel; only the owner starts that, from the panel's own update screen.

- **v0.1.0-alpha.88 is published, 2026-10-16; no installed server ran it.**
  Written as a candidate and amended the same day for its publication. The number
  was chosen by the release preparer under the owner's standing instruction of
  2026-10-13 (release preparation `7231afdbf`, sequence 88 after 87, with the
  alpha.87 release commit `a8cddcb5…` as previous). One subject: after the
  owner updated Frankfurt from alpha.85 to alpha.87 on 2026-10-16 and saw the
  old "connection interrupted" warning, the owner asked for one progress bar
  per step, the next appearing below, nothing alarming, and the updated state
  at the end. The candidate (branch `feat/alpha88-update-steps`, from `main`
  at `89995199f`): the Agent reads the updater's download and verification
  progress (`b1d2ee381`); the Panel declares five steps on the start and
  status answers and measures the restart into `panel_settings`
  `panel_update_last_restart` (`22cdc2c4a`); the page draws one bar per step,
  the restart as a planned step with one calm sentence and "Updated to" at the
  end (`8b96ca5ce`, `b63da83e0`); the lock sentence beside the steps is
  shortened (`07aaa50d5`). D-024, D-032 and D-033 apply; no new decision. No
  schema change (44) ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.88.md)).
  - *Measured:* web `npm test` 1523 of 1523 at `07aaa50d5`; Go at
    `b63da83e0` (panel and transport 1602 tests, 1695 subtests, 6 skipped;
    Agent 1805 tests, 2838 subtests, 29 skipped, clean namespace); browser
    batch 17 (9 states in 8 configurations, one defect fixed). **No native
    record, by the owner's instruction of 2026-10-16.**
  - *Open:* the Agent's progress read, the floor timestamp and the restart
    measurement not observed during a real update; the steps are first seen on
    the update from alpha.88 (an update to alpha.88 runs on alpha.87's page), so
    alpha.89 follows with the same code; a retry of the same release reports no
    verify boundary; no installed server measured. No P0 item is closed or
    advanced; P0.1–P0.5 stay **partial**; publication would not be acceptance.
  - *Publication, 2026-10-16 (UTC).* By the release preparer, under the owner's
    instruction of 2026-10-15; installed-panel updates remain the owner's only.
    Pull request #217 was squash-merged into `main` as
    `1cb3628ff1969f3fc2e9f0d848b7adf629a815b0` (tree
    `abc2a0a3ee177f064d6b8af8304d66ae1feb1c63`); the run on it (38865909812)
    concluded success, 24 jobs (the preparer's chain script first stopped early
    because `gh run watch` returned before the run completed, safely before any
    variable or tag step, and was resumed); the repository variable went from 87
    to 88; the annotated tag `v0.1.0-alpha.88` (object
    `851766854005a2a223edd9e097afd7853a02ff30`) was pushed after the main run;
    the tag's run (38867798325) concluded success; the release was published at
    2026-10-16T07:49:41Z as a pre-release with six assets. Checked from a
    download in the `CelikPanel-S2-Debian` guest (reported by the preparer, not
    recorded in a file here): archives byte-identical, 66,196,013 bytes, SHA-256
    `feaf77476a100425a2f048b47ece9da85a45585b5b8f3387692b497ad04a45d3`;
    manifest sequence 88 with a verifying signature (key `7eadeb0b…`); 237
    members; `libexec/get.sh` pins 88; policy 88 after 87; `release.commit` and
    `release.tree` equal the tag's; go1.26.5 trimpath CGO_ENABLED=0; interface
    `app-DAi4coqL.js` (248,222 bytes). Portal package 132,766,352 bytes, SHA-256
    `83bfe1327b4802f7c2d9b1ffdbb371227b1418a84c33a36cb8a83b502bfa63b5`, built
    twice identical, accepted by the promoter; published by the preparer
    (committed, backup `httpdocs.before-v0.1.0-alpha.88.20261016T080028Z…`,
    verification ok, 26 requests); read from outside at 08:00:37Z (latest.txt,
    get.sh pin and latest.json all 88). **Installed panels:** the owner
    started both updates from the panel's own update screen and both went
    directly from their previous version to alpha.89, so neither ran alpha.88
    (see the alpha.89 entry). Limits: tag files via `git archive` on Windows, no
    promoter rehearsal against a copy of the live site, no file-by-file
    comparison with alpha.87's package, the publisher's own test not run.
    Publication is not acceptance. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.88.md), "Publication
    record", say who did and who reported each step.

- **v0.1.0-alpha.89 is published, 2026-10-16; both installed servers were
  updated to it by the owner, directly from their previous versions.** Written as
  a candidate and amended the same day. The same product
  code as alpha.88 (`git diff --stat 1cb3628ff HEAD -- cmd internal web` is empty), released right after
  it by the owner's instruction of 2026-10-16 so that the update steps are first seen on the update from alpha.88; the number was chosen by the release preparer. Sequence 89 after 88, schema 44, no native record; P0.1–P0.5 stay **partial** ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.89.md)).
  - *Publication, 2026-10-16 (UTC).* By the release preparer, same instruction.
    Pull request #218 was squash-merged into `main` as
    `ac36fccf4e4238a8bc4624356a478ba8dab20b34` (tree
    `29f08b6bef1ca6e19c4f02890f83cfeb3f678433`); the main run succeeded; the
    variable went from 88 to 89; tag `v0.1.0-alpha.89`; the tag's run
    (38874797963) succeeded; the release was published at 2026-10-16T08:57:23Z
    with six assets. Checked from a download (preparer, not recorded in a file
    here): archives byte-identical, 66,195,105 bytes, SHA-256
    `58c6911be7a1b640b0bafb595a8c2b54451d70d8238d51b1c726a76bb7f59526`;
    manifest sequence 89 with a verifying signature; 237 members; `get.sh`
    pins 89; policy 89 after 88; `release.commit` and `release.tree` equal the
    tag's; the interface bundle is byte-identical to alpha.88's (compared
    directly). Portal package 132,763,175 bytes, SHA-256
    `bb36ebce7b52261988651d9df234ebe9c26b45ec605feacb5bda9f2da1649f1e`;
    published by the preparer (committed, backup
    `httpdocs.before-v0.1.0-alpha.89.20261016T090510Z…`, verification ok, 26
    requests); read from outside at 09:05:19Z (all 89, HSTS present).
    **Installed panels, reported by the owner (2026-10-16, two screenshots,
    about 09:30Z):** the owner started both updates from the panel's own update
    screen. Frankfurt (from alpha.87) and Boston (from the version it ran, not
    reported, alpha.85 or later) went directly to alpha.89; the sequence floor
    admits it. Both Settings, CelikPanel updates pages show current version
    v0.1.0-alpha.89 and commit `ac36fccf4e42…`, the sidebar reads build
    `ac36fccf4e42`, and a card "Updated to v0.1.0-alpha.89" lists five completed
    steps: after the reload the alpha.89 interface renders the server's step
    plan. The live step view during the restart was **not** seen (each update
    ran on the previous version's page); it is first seen on the next update,
    from alpha.89. Frankfurt's page shows "There is no new update on this
    channel." with a warning-triangle icon, a good state drawn with a warning
    icon: a D-033 item for the next release, not changed here. Not reported: the
    licence page, the Agent's version, Frankfurt's setup final step, Boston's
    "Extend this server's setup", `postconf -n`. Publication is not acceptance;
    no P0 item is closed or advanced; P0.1–P0.5 stay **partial**. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.89.md), "Publication
    record", have the detail.

- **v0.1.0-alpha.90 is published, 2026-10-16; the owner updated both installed servers to it, Frankfurt through its own recovery.** Written as a candidate and amended the same day. One correction since alpha.89 (D-033): after "Check for updates" finds nothing, Settings, CelikPanel updates says "You are on the latest version." with a check mark on the normal surface instead of "There is no new update on this channel." with a warning triangle; on the same card a found update and a readiness read in progress are neutral, and the warning mark stays only for a check that could not be read, a server that is not ready, a failed start, an earlier failed attempt and the alpha notice. Released by the owner's instruction of 2026-10-16 ("90'ı çıkar") so that the step-by-step update view is seen live when updating from alpha.89; the number was chosen by the release preparer. Sequence 90 after 89, schema 44, no migration, no native record; P0.1–P0.5 stay **partial** ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.90.md)).
  - *Publication, 2026-10-16 (UTC).* By the release preparer, under the owner's
    instruction of 2026-10-15; installed-panel updates were the owner's. Pull
    request #220 was squash-merged into `main` as
    `1743ac69edb0e5723dc60d992b53094dcc4b1431` (tree
    `bbfa5305cc8c5b6bb4a34a2e640633582dc7534b`); the main run succeeded; the
    variable went from 89 to 90; the annotated tag `v0.1.0-alpha.90` (object
    `9c98483d71b4a747f522f87c476f22125868bca7`); the tag's run succeeded; the
    release was published at 2026-10-16T14:42:18Z with six assets. Checked from
    a download (preparer, not recorded in a file here): archives byte-identical,
    66,194,918 bytes, SHA-256
    `3719aba4f60b3eb3da0762b2d1c57302b628cdb6c958d72c2a5ac0430e2ff3e1`;
    manifest sequence 90 (`published_at` 2026-10-16T13:58:20Z) with a verifying
    signature (key `7eadeb0b…`); 237 members; `get.sh` pins 90; policy 90 after
    89; `release.commit` and `release.tree` equal the tag's; interface
    `app-HNXfBLHb.js` (248,218 bytes). Portal package 132,762,586 bytes, SHA-256
    `b2a31a43d2b1e497b7dc9da53f405d79fa70d7193cb86f518f5d367e74b3acef`, 54
    members, built twice identical, accepted by the promoter, tests OK;
    published by the preparer (committed, backup
    `httpdocs.before-v0.1.0-alpha.90.20261016T145144Z…`, verification ok, 26
    requests); read from outside at 14:51:50Z (latest.txt, get.sh pin and
    latest.json all 90). **Installed panels, reported by the owner (screenshots,
    2026-10-16, the update from alpha.89):** Boston updated cleanly (card
    "Updated to v0.1.0-alpha.90", build `1743ac69edb0`, five steps, then a green
    "You are on the latest version." after "Check for updates"). Frankfurt's
    update showed a red cross at T+01:23, "reviewed updater failed: exit status
    1: Certbot renewal scheduler state could not be restored", and "Update
    verified" at T+01:39 for the same request
    (`13193201ba7a3d832bb64cc6d972f8a5`); its browser kept alpha.89's interface
    until Ctrl+Shift+R (sidebar alpha.89, "Current version" alpha.90), and after
    the reload there was no "Updated to" card because the update finished
    through the recovery record. Copying Boston's card pasted the build line
    with the short and the full id run together. Cause and correction of the
    Frankfurt failure: the alpha.92 entry. The live step view was not reported
    as seen. Publication is not acceptance; no P0 item is closed or advanced;
    P0.1–P0.5 stay **partial**. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.90.md), "Publication
    record", have the detail.
- **v0.1.0-alpha.91 is tagged and released on GitHub, 2026-10-16, and deliberately not on the download portal; no installed server ran it.** Written as a candidate and amended the same day. The second server as a full peer, part 2, after the owner's three decisions of 2026-10-16 ("önerin gibi olacak": the second server can be a full web and mail peer; the infrastructure zone is a normal entry of Domains on the primary, setup-created records kept by setup and the owner's own preserved; the second server's host records travel to the primary automatically over the existing authorized remote DNS connection, manual entry staying possible) and the owner's remark that users configure their own servers (the panel offers; the user decides). Gap 4: the peer's mail host name in the primary's plan; gap 5: the infrastructure zone in Domains with its record editor and `managed_by`; gap 6: `POST /api/v1/dns/remote/receiver/host-record` and the secondary's once-per-step request with its outcome on the setup page. The number was chosen by the release preparer under the owner's standing instruction of 2026-10-13; the eight implementation choices are the preparer's, to be confirmed. Sequence 91 after 90, schema 44, no migration, no native record by the owner's instruction of 2026-10-16; nothing measured on an installed server; P0.1–P0.5 stay **partial** ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.91.md)).
  - *Tag, 2026-10-16 (UTC).* By the release preparer, under the owner's
    instruction of 2026-10-15. Pull request #221 was squash-merged into `main`
    as `356109b532b9d0d0df5dd31fc19e9932918a9648`; the main run succeeded; the
    variable went from 90 to 91; tag `v0.1.0-alpha.91`; the tag's run
    succeeded; the GitHub release was published at 2026-10-16T15:48:29Z with six
    assets. **Not put on the portal, by the preparer's decision:** the Frankfurt
    update of alpha.89 to alpha.90 showed an updater defect that is also in
    alpha.91, so its second-server work ships in alpha.92. No asset
    verification and no portal package were made for it; the live portal went
    from alpha.90 to alpha.92, and the owner's updates went from alpha.90 to
    alpha.92. Publication is not acceptance; no P0 item is closed or advanced;
    P0.1–P0.5 stay **partial**. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.91.md), "Publication
    record", have the detail.
- **v0.1.0-alpha.92 is published, 2026-10-16; the owner updated both installed servers to it from alpha.90, Frankfurt without a failure or a recovery.** Written as a candidate and amended the same day. The Frankfurt update of 2026-10-16 (alpha.89 to alpha.90, request `13193201ba7a3d832bb64cc6d972f8a5`) died at its last step: turning `certbot.timer` back on started Certbot's catch-up renewal at once (`Persistent=true`), and the scheduler restore required the renewal service to be idle; the server's own recovery finished the update 16 s later. The updater now never stops, starts or resets the owner's renewal run (D-022, D-025): it pauses only the timers, waits read-only (at most 180 s) for a running renewal, accepts a failed last run as the owner's result and checks the services on load and enable only; the forward scheduler step has its own failure code, `update_finishing_scheduler`; while recovery owns the request the update page keeps a calm "Finishing the update" view with an additive `retrying` step, and a recovery that verifies the update is "Updated to" on the page and the update card (D-024, D-033); the build line copies as it reads. alpha.91 was tagged but not put on the portal, so alpha.92 also delivers its second-server work. Sequence 92 after 91, schema 44, no migration; one native lab record, set19 (Debian 13, QEMU, test-licence builds): the update's own timer start began the renewal and the update ended `update_verified` with no failure and no recovery; a renewal service failed before the update stayed the owner's and the update ended the same way; the page stayed calm; P0.1–P0.5 stay **partial** ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.92.md)).
  - *Publication, 2026-10-16 (UTC).* By the release preparer, same instruction.
    Pull request #222 was squash-merged into `main` as
    `961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a` (tree
    `091d5fb23cb16cbd217c8c1191033185a7d50b81`); its first CI run failed in
    "Verify offline release-recovery fixture contracts (no VM)" (the harness's
    mirrored code lists lacked `update_finishing_scheduler`), corrected in
    `5f662635f`, then 24 of 24; the main run succeeded; the variable went from
    91 to 92; the annotated tag `v0.1.0-alpha.92` (object
    `16a8a961aa9f06948902e4c972c4ce6603cc50fc`); the tag's run succeeded; the
    release was published at 2026-10-16T18:51:22Z with six assets. Checked from
    a download (preparer, not recorded in a file here): archives
    byte-identical, 66,240,132 bytes, SHA-256
    `3e243da119a252988df80745d77f3e500b6856c60717fa4b2b8f94e1e54e4795`;
    manifest sequence 92 (`published_at` 2026-10-16T18:09:41Z) with a verifying
    signature (same key); 241 members; `get.sh` pins 92; policy 92 after 91;
    `release.commit` and `release.tree` equal the tag's; the archive's
    `deploy/panel-tls-snapshot.sh` contains `_panel_tls_wait_service_idle` and
    `_panel_tls_note_owner_failed_renewal`, `update.sh` contains
    `update_finishing_scheduler`; interface `app-DomwK8kh.js` (248,892 bytes).
    Portal package 132,855,924 bytes, SHA-256
    `809e4f501a020ee54a847c580406f8698d1eb0809aa287be27316f00dd72ffcd`, 54
    members, built twice identical, accepted by the promoter, tests OK;
    published by the preparer with `-PreviousVersion v0.1.0-alpha.90` (the live
    portal never served alpha.91): committed, backup
    `httpdocs.before-v0.1.0-alpha.92.20261016T185901Z…`, verification ok, 26
    requests; read from outside at 18:59:09Z (all 92). **Installed panels,
    reported by the owner (two screenshots, about 19:10Z, the update from
    alpha.90):** both show current version v0.1.0-alpha.92, commit
    `961f2cb2fcae…`, sidebar build `961f2cb2fcae` and a card "Updated to
    v0.1.0-alpha.92" with five completed steps, Frankfurt too: no failure and no
    recovery this time (one update each; not acceptance of the Certbot
    correction). What the page showed during the update (the live step view
    drawn by alpha.90's page) was not reported. Not reported: Frankfurt's
    Domains "Infrastructure zone" row, Boston's mail extension with the peer
    host record, the licence page, the Agent's version, `postconf -n`.
    Publication is not acceptance; no P0 item is closed or advanced; P0.1–P0.5
    stay **partial**. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.92.md), "Publication
    record", have the detail.
- **v0.1.0-alpha.93 is published, 2026-10-16; the installed servers had not been updated to it when this was written.** Written as a candidate and amended the same day. Six small corrections from the owner's screenshots of both installed servers on alpha.92 (2026-10-16), each decided by the owner the same day (D-024, D-032, D-033): Core Mail is verified when a larger verified profile containing all of its components (Webmail, Spam-Protected Mail) proves the same reconciliation on the same scan, so Frankfurt's Core Mail no longer asks for Repair (component state alone still proves nothing; a running or failed own attempt keeps its state); "Repair webmail" is drawn like Manage and is amber only when the server reports a webmail problem; the Dashboard says "Mail is installed and verified on this server."; a mail profile with only web-server parts present is "Not installed", not "Partially installed" (Boston); Recent activity names a mail profile prepared during setup; counts are pluralised ("1 item"; Turkish "1 öğe"). The number was chosen by the release preparer under the owner's standing instruction of 2026-10-13. Sequence 93 after 92, schema 44, no migration, no native record by the owner's instruction of 2026-10-16; nothing measured on an installed server; P0.1–P0.5 stay **partial** ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.93.md)).
  - *Publication, 2026-10-16 (UTC).* By the release preparer, under the owner's
    standing instruction; installed-panel updates are the owner's. Pull request
    #224 was merged into `main` as `0897ee9238bdfee295149adfe8b16968dd6fe0de`.
    The release chain's first run tagged the wrong commit (the merge was refused
    because the pull request had conflicts and the script carried on); CI's
    policy guard rejected it ("tag does not match policy version"), no release
    was created, the tag was deleted and the chain script now stops unless the
    pull request is merged, `main` equals its merge commit and `main`'s policy
    version equals the tag. The second run produced the annotated tag
    `v0.1.0-alpha.93` (object `c64b60021c3fc6b6bb506e99bdaf6b1354f531cf`, tree
    `7b18e5252045eef74264a946d41efcbbc927b545`) and a pre-release published at
    2026-10-16T22:04:09Z with six assets. Checked from a download in the WSL
    guest `CelikPanel-S2-Debian` (preparer, not recorded in a file here):
    archives byte-identical, 66,240,206 bytes, SHA-256
    `887fbe30a29c6fa364bfafc44ec676414c32e41758417738ff4cecc287f9fc39`;
    manifest sequence 93 (`published_at` 2026-10-16T21:21:23Z) with a signature
    verified by `openssl pkeyutl -verify -rawin -pubin` (key `7eadeb0b…`); both
    content guards 0; Go go1.26.5, CGO off, trimpath; `get.sh` pins 93; policy
    93 after 92 (`961f2cb2…`); interface `app-BskYbaLr.js` (248,904 bytes).
    Portal package 132,853,434 bytes, SHA-256
    `b27504acc26d7c291d7e0ec3ebfb051ac04996b27af65e78e99c3a0ea934f83c`, 54
    entries, built twice identical; contract test 0, public verifier 17 tests,
    promotion 14 tests; published by the preparer with previous
    v0.1.0-alpha.92: committed, backup
    `httpdocs.before-v0.1.0-alpha.93.20261016T221039Z…`, verification 26 of 26
    requests, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; read from outside after the
    publication (`latest.txt` v0.1.0-alpha.93, `latest.json` sequence 93).
    **Installed panels:** Boston and Frankfurt were on alpha.92 when this was
    written; the owner had not yet updated either, so nothing from an installed
    server is recorded. Publication is not acceptance; no P0 item is closed or
    advanced; P0.1–P0.5 stay **partial**. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.93.md), "Publication
    record", have the detail.
- **v0.1.0-alpha.94 is published, 2026-10-17; the installed servers had not been updated to it when this was written.** Written as a candidate and amended the same day. Ready bundles (owner decision of 2026-10-17, D-034): shortcuts over the catalogue's own components, not store products. The three mail stack cards at the top of Components are replaced by one "Ready bundles" line at the head of each category (mail through the existing mail profiles; a PostgreSQL and a MariaDB bundle with phpPgAdmin and phpMyAdmin as add-ons; Nginx + PHP-FPM), derived by the Panel from the same scan and mail profiles; a database or web bundle installs its missing components one at a time through the ordinary component install, stops at the first failure with that component's guidance and resumes only on the owner's Continue. The Dashboard's Mail stacks card is replaced by one calm Ready bundles card. The Components colour corrections of the design review of 2026-10-16 are applied as existing DESIGN.md rules (neutral row icons at rest, four status-dot tones with a red stopped service, outlined row Install, a one-line healthy firewall, line icons from one map, fixed columns, 44 px row actions on a phone). The number was chosen by the release preparer under the owner's standing instruction of 2026-10-13. Sequence 94 after 93, schema 44, no migration, no new Agent job kind, no native record by the owner's instruction of 2026-10-16; nothing measured on an installed server; P0.1–P0.5 stay **partial** ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.94.md)).
  - *Publication, 2026-10-17 (UTC).* By the release preparer, under the owner's
    standing instruction; installed-panel updates are the owner's. Pull request
    #226 was merged into `main` as `d506a05e8c003dffee8de2e1268402865520bd6b`.
    The release sequence variable moved 93 to 94 before the tag was made; the
    main run and the tag run both succeeded and the release chain ran cleanly
    (no incident). The annotated tag `v0.1.0-alpha.94` (object
    `d17aa39f13568a6254eeb5f1ab271eccff5eb5be`, tree
    `7c1fd69a4dd1c43953037a344f7850dc150922b6`) points at the release commit; the
    pre-release was published at 2026-10-17T00:30:13Z with six assets. Checked
    from a download in the WSL guest `CelikPanel-S2-Debian` on 2026-10-17
    (preparer, not recorded in a file here): archives byte-identical, 66,259,323
    bytes, SHA-256
    `bec3c015317fecc226d0a282ecf7e30547e305a621e7fa70943947c6ef0a6788`; manifest
    sequence 94 (`published_at` 2026-10-16T23:46:25Z) with a signature verified by
    `openssl pkeyutl -verify -rawin -pubin` (key `7eadeb0b…`); both content
    guards 0; Go go1.26.5, CGO off, trimpath; `get.sh` pins 94; policy 94 after
    93 (`0897ee92…`); interface `app-C9mMpV6A.js` (249,028 bytes). Portal package
    132,891,376 bytes, SHA-256
    `b493b72b09e49301fdce059a52ad2d0b92f892dd0e82c1ce1f1550c4b6dcf215`, 54
    entries, built twice identical; contract test 0, public verifier 17 tests,
    promotion 14 tests; published by the preparer at about 00:48Z with previous
    v0.1.0-alpha.93: committed, backup
    `httpdocs.before-v0.1.0-alpha.94.20261017T004816Z…`, verification 26 of 26
    requests, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`; read from outside after the
    publication (`latest.txt` v0.1.0-alpha.94, `latest.json` sequence 94).
    **Installed panels:** Boston and Frankfurt were on alpha.92 when this was
    written; the owner had not yet updated either, and had not reported updating
    to alpha.93 either, so nothing from an installed server is recorded for
    alpha.93 or alpha.94. Three small items found in review are left open for a
    later release (see the release notes, "Not done"). Publication is not
    acceptance; no P0 item is closed or advanced; P0.1–P0.5 stay **partial**. The
    [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.94.md), "Publication
    record", have the detail.
- **v0.1.0-alpha.95 is a candidate, 2026-10-17; not published, no installed server updated to it.** Written on the branch `fix/alpha95-bundle-consistency` from `main` at the alpha.94 release commit `d506a05e8c003dffee8de2e1268402865520bd6b`. The owner looked at Components and the Dashboard on Boston and Frankfurt (alpha.94) and said "ok. sorunları tekrar değerlendir ve düzelt" (owner decision of 2026-10-17, D-034 amendment). Every Components category now shows the same ready bundles in the same order on every server, with the state on each chip; this withdraws alpha.94's "installed line plus at most one more offer" rule (the release preparer's mistake), which hid the MariaDB bundle on Boston and Core Mail on Frankfurt and made the Dashboard count differ from the list. One counting rule for the card and the chips (add-ons included, blocked excluded, "included" counts as installed). Installed chips open a dialogue with Repair (mail) or Verify (components). The three review items of alpha.94 are closed (the step gate's unverified flag, Verify held while a component operation runs, "Checking the server before installing X…" until the request is accepted). Row actions share one outlined icon button with a stop-sign Stop icon; the desktop version column of dashes is gone because the Panel reads versions only for PHP-FPM and Node.js (no new Agent RPC; **open item: read installed package versions for every component**); Roundcube leftovers get a muted note and a "Clean up leftovers…" confirmation listing the Agent's paths, offered on the row only when the Panel's own audit record shows an unfinished Roundcube install or removal (the Panel still cannot see the disk) and always on Roundcube's own page; sidebar counts are plain numbers; the Dashboard Domains tile and the sidebar Domains badge count domains plus the infrastructure DNS zones the Domains page lists with one helper, so tile, badge and page agree. An independent review of the candidate found ten problems, fixed in the candidate (D-034 amendment): leftovers only with a Panel record, an installed-bundle dialogue that describes the bundle, honest lines under the chips, "included" only for a bundle that is itself verified with no failed or running attempt, one Domains count, one Dashboard line per unverified bundle with an attempt, blocked chips as quiet buttons that show their reason, Verify usable on a busy host lock or package manager, the wording "2/3 · installing PHP-FPM" and "n of m installed", and the install form for a mail chip that says Install. Evidence on that tree: web `npm test` 1599 passed, `npm run build` succeeded; Go `./cmd/panel/` and `./internal/...` 7124 passed, 0 failed, 44 skipped; browser batch 20 was not re-run after the review fixes. Sequence 95 after 94, schema 44, no migration, no Agent change, no new Agent job kind, updater unchanged, so no native record; nothing measured on an installed server; P0.1–P0.5 stay **partial** ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.95.md)).

### Latest DNS results and their limits

- A [current-source BIND/PowerDNS native trial](deploy/e2e/dns-kill-matrix/NATIVE-BIND-PDNS-CURRENT-SOURCE-SMOKE-20260927.md) verified initial transfer from managed BIND on Debian to panel-free PowerDNS on Arch. SOA, NS and selected A answers matched over UDP/TCP before and after both guests rebooted; the actual Panel and Agent units stayed disabled. The bundle was local and unsigned. Later record changes, the reverse topology, installed servers and update recovery remain untested here; P0.4/P0.5 stay open.
- A [disposable PowerDNS-primary/BIND-secondary native trial](deploy/e2e/dns-kill-matrix/evidence/pdns-master-bind-20260928/contract.json) transferred a `MASTER` member and its catalog from Debian PowerDNS 4.9.17 to panel-free Arch BIND. Both answered the same SOA/A over UDP/TCP. A separate [daemon-transition measurement](deploy/e2e/dns-kill-matrix/evidence/pdns-native-transform-20260927/contract.json) found PowerDNS-generated SOA/serial, `CATALOG-HASH`, WAL/SHM and a different catalog PTR owner. The producer-aware AXFR parser has focused tests, but the product switch, reboot and independent rollback were not exercised; the paired-primary gate remains closed.
- The PowerDNS-primary path in a DNS pair remains blocked in setup and engine preview before an Agent mutation is claimed. The [failed native switch](deploy/e2e/dns-kill-matrix/NATIVE-PDNS-BIND-PEER-STAGE2-20260927.md) found a changed PowerDNS producer catalog; [PowerDNS documents](https://doc.powerdns.com/authoritative/catalog.html) automatic serial advances. V1 cannot prove that live target or safely roll it back. [V3 code](docs/PDNS-PRIMARY-SWITCH-V3-DESIGN.md) now separates the source and native serials, records exact target evidence, and supports a protected prestart owner rollback. An Agent-mediated, same-request poststart forward path now has checkpoint and drift package tests. Owner-approved committed-journal archiving is implemented in source; its durable publication and active-record retirement have focused interruption tests. Product fault/reboot acceptance remains open. V3 evidence also refuses application replacement until an explicit compatible Agent/recovery contract exists; the old evidence-policy marker is insufficient. These new schemas are not shipped or installed. The safety gate stays closed; P0.4/P0.5 remain open.
- A [fresh paired PowerDNS-primary V3 native fault trial](deploy/e2e/dns-kill-matrix/evidence/pdns-v3-native-20260928/README.md) used an uninitialized Debian primary and panel-free Arch BIND secondary. After the `target-enable-intent` process cut, the first separate same-request recovery reached a terminal `succeeded` ledger receipt, archived its V3 journal, and preserved UDP/TCP authority across a management-disabled primary reboot. An earlier run safely retained an unknown result when Debian PowerDNS kept catalog NS RDATA unchanged while normalizing SOA; later host reconciliation preserved that historical failure. The two SSH-launched cuts returned `255`; a third fresh run independently recorded `Result=signal`, `ExecMainStatus=9` (SIGKILL) under systemd and again recovered the exact request on its first attempt. No shell exit `137` is claimed. Existing BIND-to-PowerDNS migration, owner edits, independent inverse, later cuts and full pair lifecycle remain untested; the public gate and P0.4/P0.5 remain open.
- [Fresh PowerDNS V3 prestart inverse](deploy/e2e/dns-kill-matrix/evidence/pdns-v3-prestart-20260928/README.md) now has one independent, manifest-verified owner-CLI recovery after a real SIGKILL before target start on disposable Debian. Two earlier fresh attempts refused without effects and exposed overly strict masked-unit cgroup and loopback-stub listener checks; both were narrowed with focused regressions. The clean attempt restored the pre-switch inactive/masked PowerDNS state, removed the candidate database and active journal, retained a terminal rollback verdict across a management-disabled reboot, and left the original switch job correctly failed. The native exact-request status trial found a missing volatile lock after reboot; the journal-free terminal-ledger path now has focused tests and a separate disposable Debian reboot check using that same canonical historical ledger, without a producer/inverse replay or installed kit. Later cuts, migration and public admission remain open; P0.4/P0.5 stay partial.
- Separate future engine-migration gap: moving an existing paired managed BIND primary to PowerDNS is not established by the fresh V3 path or standalone V4 adapter. It needs its own paired source/native target proof and recovery trials. This is distinct from accepting a fresh PowerDNS-primary/BIND-secondary setup.
- Immediate two-topology acceptance path: the disposable BIND-primary/PowerDNS-secondary pair has initial authority, terminal edit, owner-enrolled terminal delete/re-add, and management-disabled reboot evidence, but ordinary owner enrollment and fault cells remain open. A [fresh PowerDNS-primary/BIND-secondary V3 trial](deploy/e2e/dns-kill-matrix/evidence/pdns-v3-zone-20260928/README.md) now has a real target-enable-intent SIGKILL with separate same-request recovery, terminal add/edit/delete/re-add for an authoritative child zone, and both native services answering after management-disabled reboots. In that trial, a parentless deletion correctly remained pending without authenticated peer-native absence proof. Public setup/RPC admission, ordinary owner enrollment, later fault cells, owner edits and independent inverse remain open; P0.4/P0.5 and the paired-primary gate are not complete.
- An [owner-operated BIND inspector enrollment trial](deploy/e2e/dns-kill-matrix/evidence/owner-bind-enrollment-20260928/README.md) exercised local primary/secondary enrollment, fixed-command SSH authentication, reboot persistence, explicit revocation and native BIND survival on disposable Debian/Arch guests. The final secondary state is configured, not a proven deletion response. A valid transferred-catalog challenge, product host-key pin, interrupted-install cleanup/rotation, native Debian secondary and packaging remain open; P0.4/P0.5 stay partial.
- A later [owner-enrolled PowerDNS-primary/BIND-secondary parentless deletion trial](deploy/e2e/dns-kill-matrix/evidence/pdns-bind-owner-proof-20260928/README.md) used the pinned product SSH transport and native transferred-catalog/BIND-zone observation. The actual V3 Agent deletion reached a terminal durable receipt; after secondary reboot the catalog was empty and the zone unloaded with management absent. This closes that earlier experiment only. Product packaging/admission, other cuts and independent inverse remain open; P0.4/P0.5 stay partial.
- [Explicit owner enrollment resume](deploy/e2e/dns-kill-matrix/evidence/owner-bind-resume-20260928/README.md) now reuses exact staged BIND inspector files and the restricted account. A disposable Arch trial completed materialized SSH interruption states, repeated a completed enrollment without replacing its key, reauthorized after explicit revocation, and preserved an owner SSH edit while refusing continuation. Native BIND stayed active. This is not a process-kill/power-loss trial; Debian, remaining cuts, cleanup/rotation and product packaging remain open.
- [Debian owner resume and archive integration](deploy/e2e/dns-kill-matrix/evidence/owner-bind-debian-20260928/README.md) passed the same materialized resume checkpoints with the current strict account checks on native Debian BIND/OpenSSH. The ordinary CLI preserved an owner SSH edit and kept the key revoked with BIND active. Source packaging now carries optional owner tools without running enrollment; the real dist recipe passed modes, checksum/tamper and umask reproducibility checks with actual tools and inert unrelated payloads. This closes these Debian checkpoints and source archive integration, not full enrollment, public admission, a signed release or independent inverse.
- The independent fresh-PowerDNS V3 executable checker incorrectly expected `unix.Stat_t` from `os.Lstat`/`File.Stat`, which actually return `syscall.Stat_t` on Linux. A real root-owned-file regression reproduced rejection of valid metadata; the corrected checker now passes the complete recovery package tests and rejects changed content, wrong owner/mode, special permission bits and symlink/hardlink inputs. The root-only temporary-file regression is wired into CI. No schema or recovery decision changed; this is a file-proof correction, not native independent recovery acceptance.
- [Managed BIND V3 deletion](deploy/e2e/dns-kill-matrix/NATIVE-BIND-V3-DELETION-TERMINAL-20260926.md) reached a verified terminal result with a parent-authoritative, panel-free secondary. Native removal survived reboot. Without that parent proof, deletion remains pending; `REFUSED` alone never proves absence.
- [Paired target-staged/after-write](deploy/e2e/dns-kill-matrix/NATIVE-BIND-PAIR-TARGET-STAGED-20260926.md) passed a real SIGKILL, same-request Agent recovery, secondary transfer and subsequent management-disabled reboot. Reboot observations are documented separately from its sealed fault archive.
- [PowerDNS owner-edit refusal](deploy/e2e/dns-kill-matrix/NATIVE-PDNS-OWNER-EDIT-20260925.md) preserved a post-kill owner edit, the journal and native serving. It proves bounded refusal, not concurrent-edit safety at every effect or an independent inverse.
- Standalone managed BIND to initially inactive and disabled PowerDNS now has V4 frozen-candidate and protected owner-CLI rollback source code for pre-activation cuts. The owner command can first record an exact rollback decision for an interrupted intent, target-staged or source-stopped operation; it then replays the same request. Source code also handles an enabled but inactive pre-start target only with a durable V4 enable-intent checkpoint. The producer is not wired, and no native trial or post-start or committed recovery is accepted; see [the exact scope](docs/DNS-ENGINE-ARTIFACT.md#standalone-bind-to-powerdns-v4-recovery-code-scope-2026-09-27). P0.4 remains open.
- The [DNS inventory](deploy/e2e/dns-kill-matrix/README.md) has 510 raw combinations: 268 applicable/runnable and 242 explicit N/A. **268 is not a passed-test count or a ready-fixture count.** Missing managed-BIND and legacy-secondary fixture producers, later paired phases and unexecuted cells remain open. Repeating a measured cell does not increase phase coverage.
- The September 12 cross-engine removal claim was corrected: its negative reply was insufficient evidence. New BIND/BIND success does not retroactively close BIND/PowerDNS acceptance.

- 2026-09-29: a read-only audit of `e9d1019d` produced the [DNS recovery acceptance register](docs/DNS-RECOVERY-ACCEPTANCE.md). It found one reachable path with no recovery contract: switching a serving BIND to PowerDNS on a standalone host or paired secondary wrote the legacy V1 journal, had only the Agent's own inverse, and had never been tried natively. [D-026](docs/DECISIONS.md#d-026--dns-engine-recovery-refuse-the-unrecoverable-switch-accept-same-operation-recovery-for-first-installs) declares it unsupported for this release; Panel, setup and Agent now refuse it before any mutation (`bind_source_pdns_switch_unsupported`) with the reason and next action on screen. Startup recovery of an existing journal does not pass through the gate. Same-operation Agent recovery is accepted for first installs. The kill-matrix fixture was extended to prepare fresh standalone PowerDNS and fresh standalone BIND target-verified cells; that is preparation, not evidence, and the 268 runnable count did not change. `cmd/panel` and the new `cmd/agent` tests pass locally; the full `cmd/agent` package cannot run cleanly on the local WSL guest under any user/group layout and fails identically at `e9d1019d`, so its full verification remains a CI matter for an unpushed branch.
- 2026-09-29 to 2026-10-01: kill-matrix batches 4 to 12 and seven product-flow pair runs on two disposable CelikPanel servers closed item 2 with named limits ([register](docs/DNS-RECOVERY-ACCEPTANCE.md#item-2-status-closed-on-2026-10-01-with-named-limits)). [Pair 7](deploy/e2e/dns-pair-acceptance/evidence/pair7-20261001/README.md) passed every step on BIND/BIND, BIND/PowerDNS and PowerDNS/BIND, including zone deletion proven on the secondary after the owner's `dns-peer-enroll`, re-add, a reboot with the panel and Agent disabled while DNS kept answering, and management return; [batch 12](deploy/e2e/dns-kill-matrix/evidence/batch12-zero-zone-complete-20261001/README.md) completed the zero-zone fresh PowerDNS primary cells with a parentless deletion resumed after enrollment. Eleven product defects were found only by these native runs and closed in source on the way; each is in the register with its evidence. The fresh paired PowerDNS primary gate is open on the main line within the measured envelope ([D-028](docs/DECISIONS.md#d-028--the-fresh-paired-powerdns-primary-is-offered-within-the-measured-envelope)). P0.4 and P0.5 stay partial: no SIGKILL inside a zone-sync operation, one run per cell on a laptop host with the test-only license, the re-stamp admission and proof time-out not observed natively, no installed server touched.

### Next work, in dependency order

| Order | Work within the existing plan | Exit evidence |
|---|---|---|
| 1 | P0.4: the [DNS recovery acceptance register](docs/DNS-RECOVERY-ACCEPTANCE.md) (2026-09-29) binds every DNS engine mutation the product can start to passed / named gap / unsupported. Per [D-026](docs/DECISIONS.md#d-026--dns-engine-recovery-refuse-the-unrecoverable-switch-accept-same-operation-recovery-for-first-installs) the serving-BIND→PowerDNS switch is now refused in Panel, setup and Agent for every topology (`bind_source_pdns_switch_unsupported`; component tests pass) and same-operation Agent recovery is the accepted contract for first installs. Four bounded Debian protected owner-CLI paths have native evidence (external PowerDNS adoption, PowerDNS-to-BIND rollback, one running-BIND adoption rollback, one fresh PowerDNS V3 prestart inverse). Four fresh-install cells ran natively on 2026-09-29 ([evidence](deploy/e2e/dns-kill-matrix/evidence/fresh-install-20260929/README.md)): fresh PowerDNS after target start and fresh BIND after target start (Debian and Arch) recovered on the same request; fresh PowerDNS cut before target start **failed** — the rollback's stopped-target proof rejected the never-started, install-masked unit. Fixed in `1c336f6d`; the [re-run](deploy/e2e/dns-kill-matrix/evidence/fresh-install-rerun-20260929/README.md) of the `target-staged` and `intent` cells on the fixed source rolled back at startup and converged on the same-request retry. Owner-command guidance (D-024) is closed in source: the status command and Agent refusals name the exact owner command and request id, or say that none applies. That work found an open code gap: after a restarted Agent releases its lease (`released-undecided`), `recover-dns-bind-switch`, `recover-dns-bind-adoption` and `recover-dns-pdns-adoption` refuse the journal, so an interrupted PowerDNS→BIND switch or adoption has no admitted owner command while the Agent runs; earlier native passes were taken with the Agent inactive. The admission rule (exactly the Agent's deliberate release, no schema change, no ledger rewrite; truthful text computed at read time) and the controller flow `--owner-inverse-after-restart` are in source. Native result: the first run on `7ad24282` failed with safety passed (the owner inverse rejected the never-started, guard-masked BIND target), fixed in `411398d9`, and the [re-run](deploy/e2e/dns-kill-matrix/evidence/owner-inverse-after-restart-rerun-20260929/README.md) of the `target-staged` and `intent` cells passed with the Agent restarted and running — PowerDNS served on the same process throughout and BIND never started. The post-stop cuts (`source-stopped`, `target-started`) then [passed](deploy/e2e/dns-kill-matrix/evidence/owner-inverse-critical-20260929/README.md) on the critical variant of the flow: PowerDNS served again as the only authority with the zone serial unchanged; measured PowerDNS outage upper bounds were 21.6 s and 9.4 s and DNS is not continuous in those cells by construction. **Item 1 is closed as of 2026-09-29 with named limits** (owner decision the same day): fresh BIND, fresh PowerDNS, PowerDNS→BIND and external PowerDNS adoption on a standalone host have native same-operation recovery evidence before and after the target starts; paths without a recovery contract are refused in code. Carried to item 2 by name in the [register](docs/DNS-RECOVERY-ACCEPTANCE.md): all paired topologies, running-BIND and PowerDNS adoption with a running Agent (component tests only), stopped-BIND takeover and BIND reinstall (no native trial), V2 before-write and rolled-back cells, reboot cells, re-run exit status and on-disk residue. Closing item 1 does not close P0.4. Fresh paired-secondary cells need a panel-free native primary peer the fixture does not have; they move to item 2 and stay open. Item 1/P0.4 are not complete. | [Acceptance register](docs/DNS-RECOVERY-ACCEPTANCE.md); [Running-BIND adoption](deploy/e2e/dns-kill-matrix/NATIVE-BIND-ADOPTION-OWNER-CLI-20260927.md), [PowerDNS-to-BIND rollback](deploy/e2e/dns-kill-matrix/NATIVE-BIND-PROTECTED-OWNER-CLI-20260927.md), [PowerDNS adoption](deploy/e2e/dns-kill-matrix/NATIVE-PDNS-PROTECTED-OWNER-CLI-20260926.md), and [fresh PowerDNS V3 prestart inverse](deploy/e2e/dns-kill-matrix/evidence/pdns-v3-prestart-20260928/README.md). Detailed trial limits are in each report. |
| 2 | P0.4/P0.5: complete missing source/peer fixtures and practical deletion verification without requiring a parent zone or a remote panel. Carried from item 1 (2026-09-29, named in the [register](docs/DNS-RECOVERY-ACCEPTANCE.md)): a panel-free native primary peer for fresh paired-secondary cells, adoption inverses with a running Agent, stopped-BIND takeover and BIND reinstall cells, V2 before-write and rolled-back cells, reboot cells. **Item 2 is closed as of 2026-10-01 with named limits** ([register section](docs/DNS-RECOVERY-ACCEPTANCE.md#item-2-status-closed-on-2026-10-01-with-named-limits)). The kill matrix ran batches 4 to 12 (panel-free BIND and PowerDNS primary peers; fresh BIND and PowerDNS secondaries cut before and after start with management-disabled reboots; stopped-BIND takeover; V2 before-write and rolled-back cells; the fresh paired PowerDNS primary through the public RPC with one zone and with zero zones, owner edits held, an Agent-released job finished by the owner command, a parentless deletion resumed after owner enrollment) and the product-flow pair driver ran seven times on two disposable CelikPanel servers; [pair 7](deploy/e2e/dns-pair-acceptance/evidence/pair7-20261001/README.md) passed every step on BIND/BIND, BIND/PowerDNS and PowerDNS/BIND: setup, zone add, record edit, zone delete with the product's proof after the owner enrollment (`dns-peer-enroll`, BIND and PowerDNS secondaries), re-add, a reboot with the panel and Agent disabled while DNS kept answering, and management return. Eleven product defects were found only by the native runs and closed in source on the way (zero-zone catalog check, wizard states and license refresh, evidence in the customer archive, rndc key on Arch, PowerDNS notify port, loopback catalog transfer and inspector reasons on the managed secondary, mail stage on a DNS-only domain, the owner's inspector on a managed PowerDNS secondary, the BIND primary plan's source state, the daemon re-stamp taken for an owner change, a positive proof discarded at the wave bound). On that evidence the fresh paired PowerDNS primary gate is open on the main line within the measured envelope ([D-028](docs/DECISIONS.md#d-028--the-fresh-paired-powerdns-primary-is-offered-within-the-measured-envelope)). Register rows 3, 5, 6, 12 and 17 are PASSED with limits; rows 8 and 14 stay GAP; the named limits (one run per cell, laptop host, test-only license, no SIGKILL inside zone-sync, the re-stamp admission and proof time-out not observed natively, a version-1 BIND secondary with no upgrade trigger, the e2e harness in the customer archive) are in the register. Closing item 2 does not close P0.4 or P0.5 and authorises no installed-panel update. | Real supported primary/secondary combinations prove add/edit/delete, native loaded-zone state and reboot; uncertainty gives an actionable same-operation recovery path. Do not reclassify fixture gaps as N/A. |
| 3 | P0.1–P0.5: close the remaining end-to-end update, access, schema, TLS/enrollment and workload matrix. **Item 3 is closed as of 2026-10-01 with named limits** ([contract section](docs/RESILIENCE-CONTRACT.md#roadmap-item-3-status-closed-on-2026-10-01-with-named-limits-p01p02p03p05)). Owner-started update runs upd2 to upd6 on disposable Debian 13 and Arch guests (upd1 reached no update) measured: a good candidate installed through the Panel's update-start API as the logged-in owner; a candidate failing before completion returned to the previous release automatically, also after a second fault (VM reset on Debian, SIGKILL on Arch); a candidate failing after completion retried three times, paused with its cause kept, and finished after the owner's printed retry; cron never interrupted and the site interrupted only around the injected VM reset (about 22 s at most); with Panel and Agent disabled across a reboot, site, database, cron and firewall kept serving (SMTP on Debian) and the renewal timer kept its state. The product defects those runs found are closed in source. Closing item 3 closes no P0 item: fixture signing and loopback origin, few repetitions and no browser, no Ubuntu, no power loss, no rollback after completion, no live status at the Panel's address while it is stopped, and paths not triggered natively stay named in the contract section. | Owner UI update admission, failed candidate, automatic rollback, a second recovery fault, authenticated guidance and preserved native workloads agree on one operation. Each claimed platform/version combination has retained evidence. |
| 4 | Release review of the exact candidate and a short owner test path. **Candidate review done; the candidate's exact code (`f6cdd5a0`) has one complete run of the update matrix on 2026-10-02/03 and publication waits for the owner** ([release notes](docs/RELEASE-NOTES-v0.1.0-alpha.81.md)). The customer archive no longer ships the harness, test scripts or evidence and the signing step refuses one that does. A read-only review of the update path found seven defects, all seven corrected in source (component tests; residuals are listed in the contract section); the most important was that a worker started by v0.1.0-alpha.80 wrote no status record, so the new guidance never appeared on the first upgrade. That first upgrade was then measured, one run per case, from the alpha.80 source rebuilt with a test license (not the signed archive) on Debian 13 and Ubuntu 24.04 (good candidate, owner continuation, defective candidate returning to alpha.80); Arch from alpha.80 was not measured. Ubuntu exposed that an idle package helper blocked setup and could refuse an update start; the first fix did not hold on a real system, the corrected one was measured. Open for the owner: the version number; the root-only packaging tests (18) on a disposable machine or CI; production signing and its compatibility check; whether the vendor publishing tools stay in the archive; the owner test; every installed-panel update. After the review corrections, the Panel's deferred retry of its mail startup steps was added and measured, and the whole matrix was run once more on the final code (upd13: 20 cells, no candidate defect). Named limits are in the release notes; no P0 item is closed. **State on 2026-10-09:** the release is published as `v0.1.0-alpha.81` (tag at `a0beb726`, 2026-10-04). Reported by the owner, and not recorded in a file here: the publication steps (merge of pull request #204, tag, portal publication), the verification of the six signed assets, and the update of the two installed panels (one Ubuntu 24.04, one Debian 13) from the panel's own update screen on 2026-10-08. | All required acceptance items are closed, or a deliberately scoped release states its still-open limits. Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 5 | Release review of `v0.1.0-alpha.82` (see "Release state — October 10, 2026" above). **Published 2026-10-10 (tag on `2a0af8866`; 22 of 22 CI jobs; six assets; portal published by the owner; both installed servers updated by the owner from the panel's update screen); the number is v0.1.0-alpha.82 (owner decision of 2026-10-10).** A seventh native record, a real Chrome against the published code, passed the update-window and hidden-tab cells and failed the cold page load (the `RecoveryAccess` gate); it is retained with its intake corrections. Four native runs, one follow-up and one reading of `postconf` on 2026-10-08 and 2026-10-09 measured the settings writes, the service actions, the request identity, the update from the published v0.1.0-alpha.81 (at `cfa329676`) and the last corrections on fresh guests (at `557b554eb`, PHP sites on Arch included); the one failed check, the note after a Postfix stop on Ubuntu 24.04, was corrected and measured again. The `postconf` correction (`67b62cc0f`) rests on one reading of the real program and component tests. The update matrix was repeated at `67b62cc0f` on 2026-10-09 in a fifth run: all ten cells reached the end the third run measured, once each, with guest disks in memory. The final code (`72b879eea`, which adds the Panel's narrower `Strict-Transport-Security` header, [D-030](docs/DECISIONS.md)) was then installed fresh on three platforms, its header read on the guests and one update per platform verified, in a sixth run. Not repeated on the final code: the settings-write, request-identity and full service-action cells, and seven update cells. Not run on a real system: the corrected mail certificate path. Decided by the owner on 2026-10-10: the number; the fresh-install measurement before publication; the header change. The owner's own test is the update of the two installed servers, started by the owner from the panel's update screen, with no disposable server. Done by the owner on 2026-10-10: the release sequence variable, merge, tag and portal publication; production signing ran in CI on the tag and the assets were verified; both installed-panel updates (the owner started them). Carried to the next release: the audit of what an owner's edit of a generated site configuration file meets at a Panel start, and the open list under "Release state" ("Open for the next release (2026-10-10, after publication)"), which includes four owner observations from the installed updates and the first-load interstitial. No P0 item is closed or advanced. | The fifth and sixth runs are retained ([update matrix](deploy/e2e/release-recovery/evidence/set5-20261009/README.md), [fresh install and header](deploy/e2e/release-recovery/evidence/set6-20261009/README.md)) and the [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.82.md) say which check rests on which code. The release states its still-open limits. Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 6 | Release review of `v0.1.0-alpha.83` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-11 (tag on `0447558e9`; CI on the tag 24 jobs, successful; six assets; portal published by the owner); both installed servers were updated to it by the owner on 2026-10-11, found serving its interface from outside on 2026-10-12, and shown on the owner's two screenshots of 2026-10-12 (recorded, with open parts); the number is v0.1.0-alpha.83 (owner decision of 2026-10-11); D-031 enters it.** Seven native records (the seventh to the thirteenth, 2026-10-10 and 2026-10-11, disposable guests, test-licence builds, updates from the tags' source rebuilt) measured the interfaces of alpha.81 and alpha.82 in a real Chrome, the first load and the access wait of the candidate, what v0.1.0-alpha.82 does with an owner's edit, the first part of D-031, its corrections, its second step (pool, application unit) with the update from alpha.82, and the corrections after that, on Debian 13, Ubuntu 24.04 and Arch except where a record says otherwise; the thirteenth ran on Debian 13 and Ubuntu 24.04 only. The changes made after the thirteenth record have component tests only. Done by the owner on 2026-10-11: the release sequence variable set to 83, the decision that the vendor publishing tools and two recovery scripts stay in the archive, merge, tag, portal publication; the six signed assets were produced by CI on the tag and checked (a read of the branch protection showed no rename of required checks was needed). Installed panels: not updated when the publication record was written on 2026-10-11; on 2026-10-12 a read from outside found both serving this release's interface, so the owner updated both from the panel's own update screen on 2026-10-11. The owner's two screenshots of 2026-10-12 show v0.1.0-alpha.83 in the sidebar of both (its build id wraps mid-word), a Dashboard on the first server, and on the second the setup page saying "Setup stopped: its plan was reopened" with the last step "Stopped" and none "In progress", its final check waiting for a reverse DNS (PTR) record only the owner can change. Not reported: the full-screen licence page during the restart, the update notice against the card, the momentary licence page on a direct load of the setup address, the update's verification result inside the panel, the Agent's version, the `postconf -n >/dev/null` output; both servers have no domains, so the site-file adoption of the first start had nothing to adopt there and the Domains row cannot be seen. No P0 item is closed or advanced; this is the owner's observation, not acceptance evidence. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.83.md) say which check rests on which code and list the limits; the records are under `deploy/e2e/release-recovery/evidence/`. Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 7 | Release review of `v0.1.0-alpha.84` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-13 (tag on `f0157e6c8`; CI on the tag 24 jobs, successful; six assets; portal published by the owner); both installed servers were updated to it by the owner on 2026-10-13 (still serving the interface build of v0.1.0-alpha.83 at a read from outside at 16:16 UTC; serving alpha.84's at 16:59 UTC) and shown on the owner's two screenshots of 2026-10-13 (recorded, with open parts); the number is v0.1.0-alpha.84 (owner decision of 2026-10-13); D-032 enters it.** The Dashboard's "System services" card checks the components by itself, the server states a scan's age and the time left on the licence, and an unreachable Agent is named. Four native records (the fourteenth to the seventeenth, 2026-10-12 and 2026-10-13, disposable guests, test-licence builds, real headless Chrome) measured it: the seventeenth, on the release commit itself, covered fresh installs on Debian 13 and Ubuntu 24.04 and the update from alpha.83 on Debian 13 only, and left two open items. Done by the owner on 2026-10-13: merge of #207 then #208, the release sequence variable set to 84, tag, portal publication; the six signed assets were produced by CI on the tag and checked. Process note: the tag was pushed while the CI run on the main commit was still in progress (it passed afterwards; the same tree had passed 24 of 24 on the pull request); (corrected 2026-10-14: the publishing job depends on the packaging job, which depends on every test and build job of the same run, so a tag's assets are published only after that run passed; the note about the order between the run on main and the tag stands). Installed panels: not updated when the publication record was written; on 2026-10-13 a read from outside found both serving this release's interface, so the owner updated both from the panel's own update screen. The owner's two screenshots show v0.1.0-alpha.84 in the sidebar of both (its build id still wraps mid-word), on the first server a Dashboard whose "System services" card reads "5 / 5 running / installed" with nothing pressed, and on the second the setup page unchanged from alpha.83, its final check waiting for a reverse DNS (PTR) record only the owner can change. Not reported: the update's verification result inside the panel, the Agent's version, the full-screen licence page during the update, the `postconf -n >/dev/null` output; both servers have no domains, so the D-031 question stays open on installed servers. No P0 item is closed or advanced; publication is not product acceptance, and this is the owner's observation and an outside read, not acceptance evidence. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.84.md) say which check rests on which code and list the limits; the records are under `deploy/e2e/release-recovery/evidence/`. Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 8 | Release review of `v0.1.0-alpha.85` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-15 (tag on `36c33c45`; CI on the tag 24 jobs, successful; six assets; portal published by the release preparer); installed panels not updated when the publication record was written, then updated by the owner from the panel's own update screen (report of 2026-10-15, recorded 09:58Z); the number v0.1.0-alpha.85 was chosen by the release preparer under the owner's standing instruction of 2026-10-13 (release preparation `5443fe055`, sequence 85 after 84); D-033 enters it.** The owner's rule of 2026-10-14 after the last setup step on the second installed server: a screen that makes the owner wait shows that it waits or says that nothing needs to be done, and ends with an honest "not known yet" after a bound. Setup finishes on the owner's passing check and names the reverse DNS wait; an audit of 37 waiting states changed 19 (16, then the 3 the owner decided the same day); O4–O6 of alpha.84, the Mail stacks card, typed mail profile sentences, the HSTS time left on the server's clock and the sidebar build id are corrected; the Agent asks the next resolver on a stale PTR. One native record (the eighteenth, set16, 2026-10-14, disposable guests, test-licence builds of a fixture commit, real headless Chrome) measured a fresh install on Debian 13 and the update from alpha.84 on Debian 13 and Ubuntu 24.04, good and defective (`update_verified`; `recovered` / `rollback_verified`; the ledger after the return `equal-except-volatile`, only `observed_at`). Not reached natively: "Finishing setup" and the update card's busy state. Done on 2026-10-15: pull request #210 squash-merged by the owner; the run on the main commit failed once on two jobs before any step ran (the download of `actions/setup-go@v6`), was re-run by the release preparer and succeeded, and only then was the tag pushed (the intended order was followed); the owner set the release sequence variable to 85 and pushed the tag; CI signed six assets; the assets and the portal package were checked from a download by the preparer; the preparer published the portal (the first invocation stopped at parameter binding before any transfer, because `$PSScriptRoot` is empty in a parameter default: open for the next release). Owner's report after the updates (screenshots, an outside read, not acceptance evidence): both servers show v0.1.0-alpha.85 and the sidebar build id on one line; Frankfurt's update page showed a warning-coloured "could not be read just now / not known whether it has finished" notice plus a toast during the update's own expected restart, which the owner finds reads like a problem when nothing is wrong (open for the next release under D-033 and D-024: calm progress with the T+ counter inside the expected window, the warning and "not known" wording only after it or on a verified failure, one notice; `web/src/components/SystemUpdateOperation.tsx`, `PanelUpdateCard.tsx`); Boston's Dashboard read 5 / 5 services and "Nothing CelikPanel read needs action" (its earlier failed read has no known cause); Frankfurt's Dashboard showed "Checking…" states, result not captured; raw audit ids remain in "Recent activity". Not reported: the full-screen licence page, the Agent's version, Frankfurt's setup final step after the update, `postconf -n >/dev/null`. No installed server measured by the release preparer; no P0 item is closed or advanced; publication is not product acceptance, and set16 is a measurement, not acceptance evidence. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.85.md) say which check rests on which code and list the limits; the records are under `deploy/e2e/release-recovery/evidence/`. Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 9 | Release review of `v0.1.0-alpha.86` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-15 (tag on `8d260f31`; CI on the tag 24 jobs, successful; six assets; portal published by the release preparer); installed panels not updated when the publication record was written; the number v0.1.0-alpha.86 was chosen by the release preparer under the owner's standing instruction of 2026-10-13 (release preparation `313f3eea2`, sequence 86 after 85); D-033 and D-024 apply, no new decision.** The owner's observation of 2026-10-15 during the real update to alpha.85: the update's own restart was shown with a warning and a toast saying the state was not known, although nothing was wrong. The candidate shows that restart as calm progress, keeps the warning and "not known" wording for after a bound (6 minutes from the start, at least 2 minutes from the first lost read) or a verified failure, and says it in one notice. Besides: the publishing script's relative-path defect; reads only while the tab is visible; set16 O1–O3 (the page language, 27 component names and descriptions in Turkish, 179 audit actions as sentences); `system` reserved as a username; no sentence asks for a rescan, and a component's own page checks an unverifiable record by itself; the platform and role names in mail-profile sentences; the resolver behind the reverse DNS sentence (needs an Agent update); the Store's "State not known yet"; after the native records, the licence gate and the session gate stay quiet through the update's restart (set17 D1) and the navigation lock covers the restart, at the latest 6 minutes from the start (set17b F1, the release preparer's decision under D-033, not an owner decision), all web only. Component tests (web 1483 of 1483 at `7265d3c99`) and browser batches 13 to 15 against the mock; native records [set17](deploy/e2e/release-recovery/evidence/set17-20261015/README.md) (at `313f3eea2`) and [set17b](deploy/e2e/release-recovery/evidence/set17b-20261015/README.md) (at `ecd2a5f08`, a restart outage of about 3 minutes made by a lab delay): the candidate's page stayed calm with no "not known" wording in all five candidate runs where alpha.85's page showed it; set17 D1 not reproduced in set17b's two runs (mechanism not established); F1's change after set17b and the `PANEL_STARTING` path not measured natively. Done on 2026-10-15, all by the release preparer under the owner's instruction: pull request #213 squash-merged; the run on the main commit succeeded (24 jobs) and only then was the tag pushed (the intended order was followed); the release sequence variable set from 85 to 86; CI signed six assets; the assets and the portal package were checked from a download by the preparer; the preparer published the portal (the invocation used the absolute path, so the relative-path correction was not exercised by it) and read the public site from outside. Limits: tag files via `git archive` on Windows, no promoter rehearsal against a copy of the live site, no file-by-file comparison with alpha.85's package, the publisher's own test not run. To be reported by the owner after updating: both servers' update result and what the update page showed during the restart (the calm notice, no gate dialog, the lock), the full-screen licence page, the Agent's version, Frankfurt's setup final step, `postconf -n >/dev/null`. No installed server measured; no P0 item is closed or advanced; publication is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.86.md) say which check rests on which code and list the limits; the records are under `deploy/e2e/release-recovery/evidence/`. Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 10 | Release review of `v0.1.0-alpha.87` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-16 (tag on `a8cddcb5`; CI on the tag 24 jobs, successful; six assets; portal published by the release preparer); installed panels not reported updated when the publication record was written; the number v0.1.0-alpha.87 was chosen by the release preparer under the owner's standing instruction of 2026-10-13 (release preparation `471377d60`, sequence 87 after 86); D-021 and D-033 apply, no new decision.** Part 1 of the second installed server as a full peer: a completed setup can be extended (`POST /api/v1/setup/extend`, "Extend this server's setup", a review of what changes, what is installed and what is kept, "Stop extending" removes nothing); Components mail on a prepared server goes through setup's steps and checks; a saved mail host name can be corrected and a refused change leaves it as it was; a DNS secondary names the primary and the exact record instead of "Configure DNS" (`mail_dns_guidance`); one typed sentence per open check; after set18, an extension no longer opens on the completed run (D1, `superseded`), a started run names its first step (O1) and a pressed continuation stays single-flight (O3). Component tests (web 1499 of 1499; `./cmd/panel` 1558 passed, 6 skipped, 1675 subtests at `bef12820e`; the Agent unchanged) and browser batch 16 (36 states in 8 configurations, seven defects corrected); native records [set18](deploy/e2e/release-recovery/evidence/set18-20261015/README.md) (at `5d54aba1e`) and [set18b](deploy/e2e/release-recovery/evidence/set18b-20261016/README.md) (at `6b363ac46`) in a lab with its own DNS and ACME on Debian 13: setup and the Core Mail extension to ready, D1 recorded in set18 and observed fixed in set18b with O1 and O3, no new defect. Open: part 2's three owner decisions (until then Boston's mail record is made by hand on Frankfurt or at the DNS provider); the secondary-DNS boxes not reached natively; the public ACME/DNS path, Ubuntu and Arch not run; the raw-file observations of the intake (the FQDN journal line, `cron DEATH`, the self-signed-fallback toast, the access banner around setup's restart) not judged. Done on 2026-10-16, all by the release preparer under the owner's instruction: pull request #215 squash-merged; the run on the main commit succeeded (24 jobs) and only then was the tag pushed (the intended order was followed); the release sequence variable set from 86 to 87; CI signed six assets; the assets and the portal package were checked from a download by the preparer; the preparer published the portal and read the public site from outside. Limits: tag files via `git archive` on Windows, no promoter rehearsal against a copy of the live site, no file-by-file comparison with alpha.86's package, the publisher's own test not run. Published about 5.5 hours after alpha.86 and before the owner had reported updating any server to alpha.86; the updater's sequence floor admits an update from alpha.85 or alpha.86 straight to alpha.87 (read from the code, not measured). To be reported by the owner after updating: both servers' update result (from whichever version), what the update page showed during the restart, the full-screen licence page, the Agent's version, Frankfurt's setup final step, `postconf -n >/dev/null`, and whether "Extend this server's setup" appears on Boston's ready screen. No installed server measured; no P0 item is closed or advanced; publication is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.87.md) say which check rests on which code and list the limits; the records are under `deploy/e2e/release-recovery/evidence/`. Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 11 | Release review of `v0.1.0-alpha.88` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-16 (tag on `1cb3628f`; the tag's run successful; six assets; portal published by the release preparer); no installed server ran it, because both went directly to alpha.89; the number v0.1.0-alpha.88 was chosen by the release preparer under the owner's standing instruction of 2026-10-13 (release preparation `7231afdbf`, sequence 88 after 87); D-024, D-032 and D-033 apply, no new decision.** The owner's design of 2026-10-16 after updating Frankfurt from alpha.85 to alpha.87: one bar per update step (download, verify, prepare, install and restart as one step, confirm), the restart as a planned step with one calm sentence, "Updated to {version}" at the end, overdue or failure wording only after the 6-minute window or a verified failure, one notice, a short lock sentence beside the steps. Agent progress read, Panel plan and restart measurement (`panel_settings` `panel_update_last_restart`), page steps. Component tests (web 1523 of 1523; Go panel and transport 1602/1695/6, Agent 1805/2838/29) and browser batch 17 (9 states × 8, one defect fixed). No native record by the owner's instruction; the steps were meant to be first seen on the update from alpha.88, so alpha.89 followed with the same code. No installed server measured; no P0 item is closed or advanced; publication is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.88.md) say which check rests on which code and list the limits. Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 12 | Release review of `v0.1.0-alpha.89` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-16 (tag on `ac36fccf`; the tag's run successful; six assets; portal published by the release preparer); the number was chosen by the release preparer under the owner's standing instruction of 2026-10-13; released right after alpha.88 by the owner's instruction of 2026-10-16.** The same product code as alpha.88 (`git diff --stat 1cb3628ff HEAD -- cmd internal web` is empty). The owner updated both installed servers to it from the panel's own update screen, directly from their previous versions (Frankfurt from alpha.87); both show the version, the commit and a card with five completed steps (two screenshots, about 09:30Z). The live step view during the restart was not seen (the update ran on the previous version's page): first seen on the next update, from alpha.89. A good state ("no new update") drawn with a warning icon is a D-033 item for the next release. No native record, by the owner's instruction. Licence page, Agent version, Frankfurt's setup final step, Boston's extend option and `postconf -n` not reported. No P0 item is closed or advanced; publication is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.89.md) point to the alpha.88 notes for everything else. Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 13 | Release review of `v0.1.0-alpha.90` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-16 (tag on `1743ac69`; the tag's run successful; six assets; portal published by the release preparer); the number was chosen by the release preparer under the owner's standing instruction of 2026-10-13; released by the owner's instruction of 2026-10-16 ("90'ı çıkar").** One D-033 correction: a check that finds no new update reads "You are on the latest version." with a check mark instead of a warning triangle; the card's other good or neutral states are no longer drawn as warnings. The owner updated both installed servers to it from the panel's own update screen: Boston cleanly (card "Updated to", then a green "You are on the latest version."); Frankfurt failed at the last step on the renewal timer's catch-up run and was finished by its own recovery 16 s later (cause and correction in alpha.92), with the browser keeping alpha.89's interface until a hard reload and no "Updated to" card afterwards (screenshots). The live step view was not reported as seen. No native record, by the owner's instruction. No installed server measured by the release preparer; no P0 item is closed or advanced; publication is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.90.md). Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 14 | Release review of `v0.1.0-alpha.91` (see "Release state — October 10, 2026" above, the entry for this release). **Tagged 2026-10-16 (tag on `356109b5`; the tag's run successful; six assets on GitHub), deliberately not put on the download portal, so no installed server ran it; the number was chosen by the release preparer under the owner's standing instruction of 2026-10-13.** The second server as a full peer, part 2 (gaps 4, 5 and 6): the infrastructure zone as a Domains entry with its record editor, the peer's mail host name in the primary's plan, and the host record requested from the primary over the authorized remote DNS connection, with the exact record to enter by hand when there is no connection or a refusal. Needs the channel between the two installed panels for the automatic record. It ships to servers in alpha.92, because the Frankfurt update of alpha.89 to alpha.90 showed an updater defect that is also in this release. No native record, by the owner's instruction of 2026-10-16. No installed server measured; no P0 item is closed or advanced; publication is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.91.md). Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 15 | Release review of `v0.1.0-alpha.92` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-16 (tag on `961f2cb2`; the tag's run successful; six assets; portal published by the release preparer with the previous version alpha.90, because the live portal never served alpha.91); the number follows the decision of 2026-10-16 that alpha.91 is not put on the portal.** The updater's Certbot renewal correction (the Frankfurt update of 2026-10-16), the typed `update_finishing_scheduler` code, the calm "Finishing the update" view and the recovered success as "Updated to", the copy-clean build line; it also delivers alpha.91's second-server work. One native lab record, set19. The owner updated both installed servers from alpha.90 to it from the panel's own update screen; both show the version, the commit and a card with five completed steps, Frankfurt too without a failure or a recovery (two screenshots, about 19:10Z). The live step view during the update, Frankfurt's Domains infrastructure-zone row, Boston's mail extension, the licence page, the Agent's version and `postconf -n` were not reported. No installed server measured by the release preparer; no P0 item is closed or advanced; publication is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.92.md). Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 16 | Release review of `v0.1.0-alpha.93` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-16 (tag on `0897ee92`; six assets; portal published by the release preparer); the number was chosen by the release preparer under the owner's standing instruction of 2026-10-13.** Six small corrections from the installed servers on alpha.92: Core Mail verified through a larger verified profile, a calm "Repair webmail" button, plain words for verified mail, "Not installed" for a mail stack with no mail component, profile names in Recent activity, pluralised counts. No native record, by the owner's instruction of 2026-10-16. The first run of the release chain tagged the wrong commit and was rejected by CI before any release existed; the second produced this release (details in the release notes). Boston and Frankfurt were on alpha.92 and not yet updated when this was written; nothing from an installed server is recorded. No installed server measured by the release preparer; no P0 item is closed or advanced; publication is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.93.md). Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 17 | Release review of `v0.1.0-alpha.94` (see "Release state — October 10, 2026" above, the entry for this release). **Published 2026-10-17 (tag on `d506a05e`; six assets; portal published by the release preparer); the number was chosen by the release preparer under the owner's standing instruction of 2026-10-13.** Ready bundles at the head of each Components category and one Ready bundles card on the Dashboard (D-034), with the Components colour corrections. No native record, by the owner's instruction of 2026-10-16. The release chain ran cleanly. Boston and Frankfurt were on alpha.92 and not yet updated when this was written; nothing from an installed server is recorded. Three small items found in review are left open (release notes, "Not done"). No installed server measured by the release preparer; the bundle sequence has not run against a real Agent; no P0 item is closed or advanced; publication is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.94.md). Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |
| 18 | Release review of `v0.1.0-alpha.95` (candidate; see "Release state — October 10, 2026" above, the entry for this release). **Candidate 2026-10-17; not published; the number was chosen by the release preparer under the owner's standing instruction of 2026-10-13.** The same ready bundle list on every server with the state on each chip (the owner's decision of 2026-10-17, D-034 amendment), one counting rule for the Dashboard card and the chips, the three open review items of alpha.94 closed, one outlined icon button and a stop-sign Stop icon for row actions, no version column of dashes, Roundcube leftovers cleanup (offered on the row only with a Panel record of an unfinished install or removal), one Domains count for tile, sidebar badge and page, plain sidebar counts, and ten corrections from an independent review (web 1599 and Go 7124 tests passed; browser batch 20 not re-run after the review fixes). No schema, Agent or updater change, so no native record. Open: installed package versions for every component are not read. No installed server measured; no P0 item is closed or advanced; a candidate is not acceptance. | The [release notes](docs/RELEASE-NOTES-v0.1.0-alpha.95.md). Verify signed artifacts and recovery compatibility; only the user starts installed-panel updates. |

**Open item (alpha.95, 2026-10-17): read installed package versions for every component.** The component scan fills a version only for PHP-FPM and Node.js (from runtime instances). The Agent's per-unit binary probe is not a package version, is unreliable for several units and does not cover tools without a unit, so the Components rows say a version only where it is known. A real answer needs a new Agent RPC or extended `dpkg`/`pacman` queries; it is not part of any release yet ([D-034 amendment](docs/DECISIONS.md)).

Each slice must identify the open acceptance it closes before more tests are added.
Repeat a passing trial only after a relevant change or to resolve a named uncertainty.
After retaining and checking evidence, stop disposable guests and remove their
verified temporary overlays; preserve recovery material and owner data.

**Completion date:** not established by current evidence. Remaining implementation
and fixture gaps prevent an honest percentage or fixed finish date. Re-estimate
after the independent DNS path and missing fixture scope are verified; completion
requires the existing acceptance gates, not a growing count of small commits.

### AI assistant milestone — after the operation/recovery foundation

The user-requested AI assistant remains planned: explain observed state and help
the user carry out authorized panel actions through the same typed, scoped
operation APIs. It must show the plan, honor permissions, retain the operation ID
and report verified results. Acceptance must include denied permissions, unavailable
services, interrupted requests and retry without duplicate effects. It receives
neither unrestricted root execution nor authority to invent missing evidence,
bypass licensing or initiate installed-panel updates. AI integration is not a
substitute for deterministic recovery and is not claimed implemented by this review.

---

## The Version Ladder

The version ladder below is a historical record of earlier milestones. Its past
golden-path results and architecture assessments do not establish compliance
with the September 14 resilience contract; that acceptance remains open.

Destination: **v1.0 — a panel a stranger can install on a clean VPS in minutes,
run a real hosting business on, and trust.** Everything below is a stone on that road.

The July 17 update worked three new requirements into the ladder — not a vague "later", but step by step:
1. **In-panel help/tips:** foundation stones in v0.2.5 (as natural extensions of the existing debt items),
   user-facing content in v0.3, help center + wizard + palette in v0.6–0.9.
2. **Reseller + customer as first-class experiences:** the door-opening slice in v0.2.5/B1, the body in
   v0.3 (including collection powers), `additional_user` as a real feature in v0.35.
3. **Free-tier subscription/plan system:** data model + period/cancel/suspend machine + payments ledger +
   reseller pools in v0.3, "every promise enforced or deleted" honesty in v0.35, the payment-provider
   decision (refunds/chargebacks included) in v0.5, self-signup + abuse brakes in v0.6–0.9.

### ✅ v0.0 — Takeover *(July 3, 2026)*
~23k lines inherited: architecture sound (Panel + root Agent, SQLite) but insecure
(open TCP agent, SQL injection, no authentication) and a UI full of fake data.
Decision made: continue, no rewrite.

### ✅ v0.1 — Secure Core + Proven Golden Path *(July 3–10, 2026 — historical v0.1.0 milestone)*
Eight days, four fronts, all pushed:
- **Security (Phase 0):** agent behind Unix socket + token · session identity (argon2id) + 2FA/TOTP ·
  SQL injection cleanup · CSRF/headers/rate limit · gosec highs closed · leaked passwords neutralized.
- **Hosting core (Phases 1–3):** domain types (php/static/node/proxy/redirect) + subdomains ·
  real auto-renewing SSL · authoritative DNS (PowerDNS/SQLite sync, DNSSEC, DANE) ·
  full mail stack (TLS+SNI, authenticated submission 587/465, DKIM signing, server policy,
  deliverability health screen) · databases v2 · one-click WordPress · cPanel importer v1 ·
  accounts (admin/reseller/customer, plans, quotas, impersonation) · entitlements + WireGuard VPN ·
  scheduled backups with retention · audit log · firewall (default-deny) · service uninstall ·
  unattended security patches · managed vendor repos (PGDG version choice).
- **Operations (Phase 2):** `install.sh` (one command → login screen) · snapshotting `update.sh` · `rollback.sh` ·
  systemd units · **golden path proven end-to-end on Ubuntu**: clean install → domain → HTTPS →
  own DNS answering the world → DKIM-signed mail in Gmail's INBOX.
- **Product & design:** Plesk-density UI, light/dark, TR+EN · design system on claude.ai/design
  (design loop: describe → agent draws with real components → filter → ship) · self-hosted brand fonts ·
  the new scale on every page · live dashboard with a setup journey.
- **Alpha operating model (D-008):** the operator drives the panel like a real customer; every wall they
  hit becomes a product fix. ~20 real bugs found and shipped this way in two days.

**Exit criterion met:** golden path proven end-to-end (Ubuntu) · the panel carries its own updates · the alpha model works.

### 🔶 v0.2 — Alpha Complete: The Debian Re-Proof *(historical milestone; current priority above)*
The same golden path, re-proven on the production VPS (Debian 13) **entirely with panel clicks**:
- ✅ Panel-only install (zero extra packages) · ✅ PowerDNS installed from the panel ·
  ✅ honest management page (config visibility, working repair)
- ⏳ Next clicks: auto-repair → first domain → panel Let's Encrypt certificate →
  DS record at the registrar → web server + live site → mail stack → **Gmail INBOX from Debian**
- Remaining alpha rough edges as they surface + `autodiscover` (mail client auto-config)
- External blocker: the domain suspension at the registrar (operator's task)

**Exit criterion:** a visitor can open `https://celikpanel.cloud` and mail sent from it lands in
Gmail's INBOX — every configuring click in the panel, none in a shell.

### 🩺 v0.2.5 — Debt Payment (the Autopsy Prescription) + Foundation Stones
The July 11, 2026 forensic audit ([AUTOPSY](docs/AUTOPSY.md)) surfaced live breaks and structural debt;
decision: **refactor, not rewrite.** B0 (stop the bleeding: dead TypeID constants, broken Databases page
for non-admins, dead code) closed the same day. The rest in order: **B1** one API (v2→v1, tenant scope
from auth, OpenAPI + generated client) · **B2** route+authz table · **B3** the catalog as sole owner of
service knowledge · **B4** UI discipline (one Button/fmtBytes/modal) · **B5** golden-path smoke CI.

The foundation stones of the three new requirements are laid on this step **not as separate work but as
natural extensions of B1–B5** (added later, they would all break a second time):
- **B1 addendum — the error contract:** all error bodies move to `{code, message, hint?, action?}`.
  `code` is a machine-readable constant (e.g. `DNS_SERVER_REQUIRED`), `hint` resolves via i18n keys in
  the frontend, `action` can be an in-panel link ("Install PowerDNS" → /services). Every deliberate
  refusal is coded: the D-009 409, quota 409s, conflict groups, the entitlement 402. One `ErrorBanner`
  component in the frontend; the error body is defined in the OpenAPI schema — the generated client is
  born right once.
- **B1 addendum — opening the way for Databases self-service:** server registration stays admin;
  DB/user CRUD endpoints open to customer+reseller under tenant scope; the temporary admin lock in
  `nav.ts` lifts; the phpMyAdmin proxy verifies ownership. (The real precondition of v0.3 — this is why
  the hard constraint exists.)
- **B2 addendum — fail-closed roles:** a request whose user record can't be read never proceeds with an
  empty role (today `middleware.go` continues with Role=''). On top of the route+authz table, a
  **role×endpoint matrix test**: with the `--demo` seed accounts, every (endpoint × admin/reseller/
  customer/anonymous) cell is verified against the expected 200/403/404; an endpoint not in the table
  fails the test.
- **B3 addendum — Setup Journey honesty:** journey steps read real service state from the catalog
  (installed + enabled + running), not package presence. Field proof already exists: the dormant bind in
  Hostinger's Arch image counted as "DNS installed: Done" (July 16). That scenario enters B5 smoke as a
  regression.
- **B3 addendum — catalog kinds + an "installed-first" default (Jul 20, D-010):** `ManagedService` gains
  `Kind` (service/runtime/tool) and `Role`; php-fpm and a new **node** entry become `Kind=runtime`,
  phpMyAdmin/phpPgAdmin become `Kind=tool`. Row rendering branches on `Kind` and the
  `Daemonless = len(SystemNames)==0` heuristic is deleted (it marks three different things today).
  **Versions are not rows; they live inside the row** (a version drawer) — list explosion is cut at its
  source. The Services page becomes installed-first: "hide not installed" defaults ON, categories
  collapsed, search always spans the whole catalog and overrides both. No separate `/runtimes` or
  `/apps` page.
- **B3 addendum — versions first-class + Node declared in the catalog:** one agent contract,
  `Agent.ListServiceInstances(id)`, returns Version/Unit/Path/Managed/SizeBytes per instance
  (`DetectInstalledPHPVersions` and `ListNodeVersions` are its first two implementations); the
  `extractVersion` switch and the `"default"` sentinel go. **The Node.js capability already exists in the
  code** (`runtime_rpc.go`, `app_rpc.go`) but is not declared in the catalog — this is visibility work,
  not new code. The node entry declares its web-server need via `Requires` (the reverse-proxy requirement,
  today expressible nowhere, becomes declarative).
- **B3 addendum — real multi-PHP (Sury):** D-002 claimed "✅ built" but only half was — detection and
  per-site selection work, multi-version INSTALLATION does not (`php-fpm` has no `Repo`, and no line in
  the codebase mentions `sury`). The existing `ManagedRepo` mechanism (PGDG) is applied to php-fpm:
  side-by-side `php8.x-fpm` becomes installable from the panel on Debian/Ubuntu; on Arch the panel
  honestly says "the distro's single version". The "a picker with nothing to pick" state ends.
- **B3 addendum — one address for runtime installs + an ownership ledger:** `AdminNodeInstall`
  (HostingTypePanel) is deleted; install/remove lives only in the Services version drawer (version
  endpoints parameterized — no Hestia 5050). The free-text semver box goes; the agent fetches the LTS
  list (3-5 named choices). The "system interpreter" escape hatch is removed — the panel runs only what
  it installed. A `site_runtimes` ledger + `RuntimeUsage`/`Dependents`: a version/service in use cannot
  be removed, returning a coded refusal (`RUNTIME_IN_USE`, `SERVICE_HAS_DEPENDENTS`) with the blocking
  site list (today php-fpm can be removed in one click while 40 sites use it).
- **B3 addendum — one source for project types + Node selectable at creation:** `CreationProjectTypes`
  (3 types) and `validProjectTypes` (5 types) derive from one `ProjectTypes` table. Add Domain gains a
  "Node.js application" card (domain + version + start command; the port is automatic). Preflight checks
  read from that table; the web-server requirement for node also returns a coded refusal **before
  persisting** — today's asymmetry ("disable the button for PHP, but for Node save and blow up in the
  agent") becomes impossible at code level. Running apps are counted under the Node runtime row
  ("3 apps · 1 failing") and link to their domain.
- **B4 addendum — the help layer's atom:** one Tooltip/InfoTip component in `ui.tsx` (HelpCircle +
  i18n'd explanation, keyboard accessible); the existing 6 Info callouts and ≥10 critical "what is
  this?" fields (DNSSEC DS, DKIM, catch-all, SNI…) migrate to it. The one obvious way to add a new tip
  is this component.
- **B4 addendum — i18n discipline:** a lint that catches bare strings in JSX (the "Coming soon..." in
  App.tsx goes; the vsftpd placeholder either becomes an honest i18n'd EmptyState or drops from nav) ·
  an en.ts/tr.ts key-parity check (`tools/check-i18n`) in CI — a missing key can't silently fall back
  to English.
- **B5 addendum — the framework-name CI gate (D-011):** grepping Go sources for
  `laravel|symfony|django|nextjs|ghost` must return zero matches (i18n strings excluded). A rule living only
  in Markdown will not survive two years; once an enum constant / DB column / API value / systemd unit name
  carries a framework name, the catalog is implicitly born and becomes irreversible because it persists in the DB.
- **Version singularity + CHANGELOG:** annotated git tag (first candidate v0.2.0) · the version is baked
  into both binaries via `-ldflags`, served from `/api/v1/panel/version`, and the hard-coded "v0.1.0" in
  Layout.tsx is deleted · CHANGELOG.md + CHANGELOG.tr.md start in Keep-a-Changelog format; `update.sh`
  prints "changes: CHANGELOG.md" on exit.

**Exit criterion:** AUTOPSY B1–B5 closed · the role×endpoint matrix runs in CI and no endpoint returns
200 for anonymous/empty-role · all deliberate 4xx checks are coded and ErrorBanner translates them ·
`panel --version`, the UI footer and the git tag say the same string · user-visible non-i18n English
strings in web: 0 (lint in CI) · as customer: create DB → open phpMyAdmin → accessing someone else's DB
is 404 (all three in B5 smoke) · **on a clean server the Services page shows at most 4 rows** (nothing
uninstalled is drawn) · the only way to install a Node version in the panel is Services
(`AdminNodeInstall` is absent from the codebase) · Add Domain creates a working Node site in one form (no
"create static first, then switch type" step) · on Debian a second PHP version installs from the panel and
is assigned to a site (Sury), while on Arch the same screen honestly says "the distro's single version" ·
attempting to remove an in-use PHP version/service returns a coded refusal listing the blocking sites ·
the project-type list lives in one file; every criterion verified on both test servers.
**Hard constraint: v0.3 cannot start before B1 is done.**

### 🚨 v0.2.6 — The Trust Floor *(new step, 25 Jul 2026)*
The code audit run while preparing this roadmap found **three live data-loss / dishonesty defects**.
They come before every other feature: a panel cannot be sold until someone can entrust a paying
customer's website to it. All three were verified in the code, not guessed.

**Three holes in the floor:**
1. **A "Full" backup contains no databases.** `createFullBackup` falls through to the files backup,
   with its own comment admitting it (*"For now, just backup files"*). An operator who takes a "Full"
   backup before a risky change cannot get the database back — silent data loss dressed as a safety
   net. For the same reason, restoring a `full_` archive returns only files.
2. **Restore can overwrite the wrong database.** The target name is derived by splitting the file
   name on underscores and taking the first part: a backup of `wp_site1` is restored into a database
   called `wp`. Underscores are common in database names, so this means **overwriting another
   customer's data**.
3. **Install never checks whether the service started.** The result of `systemctl enable --now` is
   discarded and success is reported unconditionally. The constitution's first rule — "installed
   means working" — is not measured at the one moment that decides it.

**Four structural gaps in the floor:**
4. **Template fixes never reach existing sites** (config drift, AUTOPSY section C). Two closed
   security findings — `.env` served as plain text and PHP source offered as a download — are still
   live on every site created before the fix. There is no "regenerate every vhost" path.
5. **Four site settings never reach the server.** Document root, www/https redirect, HSTS/force-HTTPS
   and domain aliases are written to the database and never to nginx. The operator changes a setting,
   the screen confirms it, and the server keeps serving the old one.
6. **Removing SSL does not regenerate the vhost** — it arms an outage for the next reload.
7. **Apache takes the web-server seat with no Apache writer.** The only vhost generator is nginx's
   (`internal/services/nginx_generator.go`; the template directory contains only `nginx/`). Installing
   Apache blackholes port 80. Until the writer exists, the row must refuse honestly.

**Exit criteria:** a "Full" archive opened by hand CONTAINS a database dump and every archive carries
its own manifest · the restore target is read from the manifest instead of guessed from the file name
(an underscored name lands in the right database on both servers) · a snapshot is taken before every
restore · install does not say "installed" when the start failed, and shows the journal tail as its
reason · the four site settings changed in the panel are measurable on the server · `nginx -t` passes
after SSL removal · one vhost writer remains and a "regenerate every site" action fixes existing sites
too (both old security findings close on pre-existing sites) · the Apache row is refused with a coded
reason while no writer exists · CI boots a real panel and runs the smoke scripts for these criteria.
**Hard constraint: v0.3 cannot start before this step is done.**

### v0.3 — Multi-Tenant Reality
Selling to more than one tenant without embarrassment. Four legs: customer and reseller can live on
their own; the plan/subscription machine can express "free tier + paid plan" **from entry to exit**
(cancellation as first-class as purchase); the essentials a cPanel emigrant looks for in week one are in
place; production trust is done before the first real tenant.

**Customer and reseller first-class:**
- The customer sees their own subscription: `GET /api/v1/my/subscription` (on B1 tenant scope) — plan
  name, quotas, live usage (domain/DB/mail counts + measured disk). A "My plan" card on the Dashboard:
  usage bars, warning color above 80%, an "Upgrade" button. A 409 quota error is consistent with the
  numbers on screen.
- Password recovery: single-use, 15-minute, argon2id-hashed token; mail from the panel's own MTA.
  E-mail verification arrives — no reset is sent to an unverified address.
- Invitation flow: password optional at user creation; without one the account opens "pending" and a
  first-password link goes by mail. A reseller never sees or relays a customer's password on any channel.
- Password change and reset drop all of the target's other sessions (completing the "leaked password
  neutralized" promise — today an open session survives a password change).
- Impersonation is accountable: `impersonate.start/stop` written to audit_logs; actions under
  impersonation carry an `acting_as` field marking the real operator; a persistent "Viewing as X — exit"
  bar at the top of the panel.
- **Reseller collection powers:** a reseller can call suspend/resume, "mark paid" and plan change for
  subscriptions in their own tree (B1 tenant scope filters); all of it lands in audit with `acting_as`.
  The reseller also sees their customer's subscription + payment state. A reseller who can't cut off a
  non-paying customer forwards collections to the operator — "reseller first-class" is half a promise
  without collections.
- Role-aware onboarding (the existing journey card pattern, no library): for the customer "first domain →
  SSL → first mailbox → connect your client"; for the reseller "plan → first customer → subscription".
  Tracks live completion, disappears when done.
- Per-page descriptions: one-two sentences TR+EN for 12 routes + 8 domain tabs (`pages.<id>.desc`).
  A "Why?" explanation at the 5 constraint-producing spots (D-002, D-003, D-009, conflict groups, pkg
  support) — texts consistent with the DECISIONS records; a constraint never hits as an unexplained wall.

**The plan and subscription machine (the free tier's foundation):**
- Prices on plans: `service_plans` gains `price_cents, currency, billing_period, is_free, is_public,
  sort_order, vat_included`. The admin defines "Free — 1 domain, 0₺" and "Pro — 10 domains, X₺/mo" from
  the panel; product prices move from code constants to the DB. The customer sees their plan's name and price.
- **Offering ledger (D-017 / the missing link in D-014):** `plan_offerings` uses one canonical namespace:
  `component:<id>[:<version>]`, `integration:acme:<id>`, and `product:<id>`. Selector and action endpoints
  must enforce the caller's effective offering set, with coded `NOT_OFFERED` refusals and audit records.
  Existing plain Store IDs need an explicit backward-compatible data/API migration; do not rename them ad hoc.
- **The period model** (the answer to "I upgraded — what am I paying?"): subscriptions gain
  `current_period_start/end`; the rule is the simplest honest one and goes to DECISIONS — an upgrade
  starts a new period immediately (no proration, declared openly); downgrades and cancellations apply at
  period end; `expires_at` derives from the period edge. The v0.5 webhook extends these fields — no
  provider integration is built on top of an undefined term.
- Subscription suspension produces real effect (today `status`/`expires_at` are dead fields): on a
  suspended/expired subscription new resources are 403, vhosts flip to a reversible "account suspended"
  page, mail delivery stops (mailboxes are not deleted). Subscriptions past `expires_at` are flipped to
  expired by a daily loop + a grace-period field. A manual "mark paid" button drives the same machine —
  the payment-provider decision is v0.5; the machine works from day one.
- **Cancellation is first-class** (the subscription form of "data is never held hostage"):
  customer-triggered "cancel at period end" (`cancel_at_period_end`); automatic downgrade to Free at
  period end. If usage exceeds the Free quota the machine doesn't deadlock: **forced-downgrade mode** —
  nothing is deleted, the excess is frozen (new resources 403 + excess vhosts get the suspension page) +
  a list of the overflowing resources + an X-day wind-down period + a notification mail.
  `subscription.cancel` lands in audit. (The same mode drives the v0.6 trial expiry — an unattended
  downgrade can't hit a 409 and stay Pro forever.)
- Plan change (upgrades are monetization's main flow): `PUT /api/v1/subscriptions/{id}/plan` — quotas
  are re-copied from the plan; on a **human** downgrade, if current usage > new quota then 409 + the list
  of overflowing resources; in **unattended/forced** mode, the freeze rule above. `subscription.plan_change`
  in audit. A customer's "Upgrade" request initially produces a notification to the operator.
- **The payments ledger** (even manual mode leaves a trace): a `payments` table
  (subscription_id, amount_cents, currency, period, method=manual|provider, marked_by, created_at).
  "Mark paid" writes to it; the v0.5 webhook feeds the same table. The customer sees a payment history
  under the "My plan" card + a printable simple receipt. "What did I pay, what did I get" is answered
  from the panel.
- **Time-based notifications** (the cheapest collections tool): mail from the panel's own MTA to the
  customer (and the reseller, in reseller scenarios) at `expires_at`−7/−3/−1 days and at grace start; a
  "why + how to reopen" mail at suspension. The customer learns of the suspension from their inbox, not
  from their visitors.
- **The reseller pool is a plan type** (a quota with a commercial life): `reseller_pools`
  (max_customers, total disk/domain/DB) binds to the reseller's plan — pool sizes + price are written in
  the reseller plan; the reseller version of the "Upgrade" flow grows the pool. When a subscription
  opens, the reseller tree's total commitment is compared to the pool; on overflow 409 + remaining-pool
  message; a usage bar in Users. **The chain rule** (to DECISIONS): if a reseller is suspended, new
  resources in their tree are 403, but existing customer sites/mail live until the end of grace — an
  innocent end-customer is not blacked out instantly for their reseller's debt.
- Reseller-owned plans: the dead `service_plans.owner_id` comes alive — a reseller builds their own
  plans (quotas can't exceed their pool), sees global + own plans in the list, assigns only to their own
  customers; "apply to subscribers" copying respects owner scope.
- **Licensed products and the resale chain (D-012, Jul 20 — third parties in scope from the start):** an
  entitlement becomes a **pool** like disk; the `reseller_pools` pattern extends to products (admin quota →
  reseller → customer; the admin may also sell directly to a customer). The product definition gains
  `license_model {server|seat}` + `seat_unit {mailbox|site|subscription|server}` — under *seat*,
  over-allocation is real money and a licence breach, so the pool is enforced hard (a coded refusal, not a
  warning). The licence key is sealed with A4's `enc:v1`. A price stops being one number (vendor→admin,
  admin→reseller, reseller→customer; the same shape as reseller-owned plans). Visibility follows
  entitlement: if the reseller did not buy it, their customers never see it (a reseller may switch on a
  "buy" prompt). An entitlement cannot be sold for a product that is not installed → coded refusal.
  Revocation follows the SAME rule as subscription suspension (new allocations 403, existing usage lives to
  the end of grace, no data deleted). **The honesty limit is stated in the UI and the docs:** the panel
  enforces only its own allocation records; the vendor may count differently (a reconciliation view is
  shown, but the invoice is the vendor's truth) and **the right to sublicense is between the operator and
  the vendor** — the panel does not verify it and does not claim to.
- The billing ledger: `plan.create/update/delete`, `subscription.plan_change`, `subscription.cancel`,
  `subscription.suspend/resume`, `quota.exceeded` audit events — "when and by whom did this quota
  change" is a dispute question; it is never unrecorded.

**Hosting essentials (a cPanel emigrant's first week):**
- FTP (vsftpd) end-to-end — with criteria: per-domain accounts, chroot to the site user's docroot,
  **FTPS required** (plain FTP refused — security is the default). Proof: connect with FileZilla →
  upload → the live site changes; an escape attempt from chroot fails.
- Webmail (Roundcube) — with criteria: one-click install from the catalog; `webmail.<domain>` vhost +
  Let's Encrypt + Dovecot wiring automatic. Proof: a mailbox created in the panel sends mail to Gmail
  from webmail and reads the reply, without ever touching a shell. (Roundcube is not "for the panel" but
  "a service the panel installs" — the no-external-dependency rule is untouched.)
- File manager polish — three measurable items: (1) upload-and-extract zip/tar.gz + compress-and-download
  selection, (2) in-place text editing (ownership/permissions preserved), (3) view/change permissions
  (warning at 777). Proof: a WordPress theme zip installed via the file manager alone shows on the site.
- Noisy-neighbor brake: a systemd slice per site/subscription (CPUWeight + MemoryMax as plan fields;
  a field only arrives together with its enforcement — no dead fields are born). PHP-FPM pools and
  `celikapp-*` units attach to their slice; no CloudLinux license required. Proof: with one tenant
  running an infinite PHP loop, the neighbor site opens in <1 s.
- OS-level disk enforcement (the ROLES deferrals) · the cPanel importer proven with a **real** customer
  archive (DB users included) · WordPress Toolkit depth (updates, hardening, clone/staging).
- **Framework hosting primitives (D-011, Jul 20):** the four real blockers to hosting
  Laravel/Symfony/Django without a catalog entry — none of them framework-specific, all of them missing
  *generic* capabilities: (1) **docroot subdirectory selection** — pinned to `public_html` today; the
  dropdown lists PATH values, not framework names (`(root) | public | public_html`). (2) **A run-as-site-user
  command endpoint** — to run `composer install`, `artisan migrate`, `npm ci` (today `composer` appears zero
  times in the codebase; the user is pushed to SSH). Streamed output, timeout, audited. (3) **Long-running
  processes (queue workers)** — three independent blocks today: the `RunAsUser: "www-data"` constant, the
  `req.Port <= 0` rejection, the `project_type == "node"` lock; the `celikapp-*` unit abstraction must also
  carry a portless, site-user worker. (4) **Site cron** — the scheduler is not a separate concept, it is an
  ordinary crontab line. On top of these, a **preset**: a button that PRE-FILLS the form with a framework's
  defaults (not a type, not an installer); it is subject to D-011's structural purity test — a preset needing
  one new field or one `if framework ==` branch is rejected, and passing 3 presets opens a strategy debate.
- **DNS provider abstraction (D-009 re-weighing + Jul 18 operator decision):** DNS must be a CHOICE, not
  an imposition. The panel already computes the full record set and writes it to one place (its own
  PowerDNS); that "writer" becomes pluggable — three backends, operator's choice (possibly per domain):
  (1) **Self-hosted PowerDNS** (default, zero-dependency: for those who want everything in one box — the
  panel is authoritative, ns1/ns2 are this server);
  (2) **Cloudflare-class managed DNS** (the operator provides a DNS-edit-scoped API token; the panel writes
  the SAME record set to the provider's API instead of PowerDNS SQL; creates the zone if missing). **The
  recommended path for security** and the operator's Jul 18 observation: :53 isn't exposed on the box,
  Cloudflare absorbs DDoS, the single point of failure is gone. The honest core counterpart of Plesk's
  "Cloudflare DNS Integration" extension;
  (3) **External/manual** (the panel writes nothing; a "enter these records" list + the live verification
  from mail-auth). Everything downstream (mail-auth records, HTTP-01 certs, the panel hostname) works
  unchanged — only the writer differs, the computed record set is identical. **The ONE un-automatable step
  is stated honestly:** the registrar's nameserver delegation (pointing celikhost.com's NS at Cloudflare
  or at this server) exists in no provider API; the panel shows + verifies it, the human makes that one
  click at the registrar. The decision (which backends, which default, the recommendation text) goes to
  DECISIONS; the abstraction seam + paths (1) and (3) in v0.3; the (2) Cloudflare backend in v0.4 (see the
  managed DNS backend).
- **Panel identity — guided hostname + certificate (Jul 18 field gap):** making the panel's own name
  (e.g. `boston.celikhost.com`) resolve was NOT designed — on the test servers a non-operator hand (a
  record added from ANOTHER server's panel) closed it; that is the hidden manual step D-008 forbids. The
  panel must handle its own hostname with the same three honest paths as adding a domain: (a) **if this
  panel serves the parent zone itself** → one-click write the A record into its own zone (single-server,
  the generalization of the zone template seeding its own FQDN); (b) **DNS is external / on another
  server** → show "add this A record: `<host>` → `<IP>`" and wait, via the live DNS check from mail-auth,
  until it resolves, then offer the certificate; (c) **already resolves** → straight to the certificate.
  The certificate flow (v0.2) sits on top of this pre-step — the "install.sh → login → real certificate"
  chain no longer has a manual DNS gap. Cross-server auto-registration (registering a sibling's name in
  the zone-authority server) belongs deliberately to the multi-server feature (post-1.0) — it needs an
  inter-panel trust model; until then path (b) serves N servers honestly.

**Production trust (required before the first tenant):**
- Secret encryption pulled forward: A4's proven `enc:v1` mechanism extends to TOTP secrets and the
  private keys the panel stores (DKIM, WireGuard); legacy rows sealed idempotently at boot. (Only the
  external-audit verification remains in v0.5 — the first tenant's 2FA secret doesn't wait months in
  plain text.)
- Per-tenant rate limits: expensive endpoints (certificate issuance, backup trigger, import, bulk DNS
  writes) protected by subscription-keyed limits; a Let's Encrypt failed-attempt counter + an honest
  "approaching the LE limit" warning — one tenant's loop can't block everyone's certificates.
- Migration discipline (expand/contract): a destructive schema change splits across two releases (add +
  double-write in N, remove in N+1) — rollback loses no data. Two CI tests: the full chain onto a clean
  DB + the current chain onto a populated v(N−1) fixture; `rollback.sh` reports the number of rows
  written after the snapshot and asks for explicit confirmation; the sentence "rollback loses changes
  made after the snapshot" is documented.
- CI security gates: `gosec` and `govulncheck` on every PR; networked dependency audits require explicit
  operator authorisation because they disclose package names and versions to the configured registry; exceptions are
  `#nosec` + reason. (The v0.5 external audit is met with these gates' monthly green history.)
- A written promotion ritual (OPERATIONS.md): (1) CI green → (2) `update.sh` + golden-path smoke on both
  test servers (boston/Debian, frankfurt/Arch) → (3) production. Channels become explicit: main=edge
  (test servers), tag=stable (production runs only tagged commits).
- `release.yml`: on a `v*` tag push, `make dist` in CI → SHA256SUMS → tarball+checksum+CHANGELOG section
  automatically to the GitHub Release.

**Exit criterion:** one reseller + two customers run **one week self-service** — password reset,
invitations, quota views, DB, FTP, webmail included; operator touches: zero · the admin defines Free +
Pro plans from the panel, one subscription moves to Pro in a single call, the audit record lands · a
cancelling Pro customer automatically drops to Free at period end; resources over quota frozen and
listed, no data deleted · a subscription near expiry receives the −7/−3/−1 mails; a suspended one
receives the "why + how to reopen" mail · every "mark paid" lands in the payments ledger and the
customer sees their receipt in the panel · a suspended subscription serves the suspension page within
60 s; resuming loses zero data · a reseller with a 10 GB pool cannot open two 6+6 GB subscriptions (the
second is 409); a reseller suspends their non-paying customer and reopens them with "mark paid" on their
own · a real cPanel account migrates in one click · the external-DNS decision is recorded in DECISIONS ·
the v0.3.0 tag produced a downloadable release without a human hand.

### v0.35 — Plan Honesty: No Dead Fields
A short step. Every promise that exists in schema/catalog but is not enforced is either enforced or
deleted — the honesty rule's plan form. The paid tier isn't "done" until every line of a sold plan is real:
- `bandwidth_quota_mb`: enforce or remove. Enforcement: a subscription-level monthly counter from usage
  measurement (reset at period start); nginx logs count only web traffic — that mail/FTP are excluded is
  honestly written in the plan text.
- The overflow policy — the single answer to "when does the limit bite": per plan
  `enforcement ∈ {block_new, notify, suspend_writes}`; mail to customer + operator at the 80% and 100%
  thresholds; a quota row in the Dashboard's "Needs attention".
- Mailbox quota: `mailbox_quota_mb` + the Dovecot quota plugin (the existing file-based pattern,
  idempotent full-state push). `business_email` raises this limit — the product's first real gate.
- Product gates: every product listed in Addons is either wired to at least one `requireEntitlement`
  gate or cannot be purchased. `extra_ip` plumbing stays "coming soon" until v0.5; `firewall` leaves the
  catalog until a customer-visible feature exists. No "for sale" product that works without purchase remains.
  **`app_installer` is brought back to reality (D-011):** today's "WordPress *and other apps*" sells a
  one-entry list as a plural plan feature — it shows sales an empty bucket to fill, and once the list is bound
  to a plan feature, deleting an entry becomes a contract breach. The product becomes "One-click WordPress
  install"; the plural wording goes.
- `additional_user` becomes a real feature: bound to a customer account (`parent_id`), resource-scoped
  permissions via `user_permissions` (domain list + file/mail sub-permissions), its own login. (The CHECK
  widening and the honesty of the dead role branches in the frontend happen in v0.2.5/B2.)
- The user-detail view: one screen with a customer's subscriptions + quota usage, domains, entitlements,
  last login (`users.last_login_at`) and last 10 audit rows — "why is this customer getting 409" doesn't
  tour four screens. Admin sees all; a reseller sees their own tree (the screen of v0.3's collection powers).
- Cron reliability: per job, last run time + exit code + a tail of the last output; on a failing job,
  mail to the domain owner (the existing mail stack, no new dependency).

**Exit criterion:** not one dead quota/state field between schema and enforcement (proven with a
field-by-field checklist) · a test subscription reaching 80% gets mail within 5 minutes; at 100% the
policy applies · the 101st MB into a 100 MB mailbox is refused with "Quota exceeded" · every Addons
product gated or unpurchasable · an additional user sees only the permitted domain's tabs · a cron
deliberately exiting 1 shows red in the list and lands in its owner's INBOX.

### v0.4 — Operational Trust
What the operator needs at 3 a.m.:
- Monitoring + alerts (service down, disk full, certificate error → mail/webhook) · log viewer in the panel
- **Metric history (from the Plesk comparison, Jul 17):** today's cards show an instant value, not a
  story. A lightweight sampler in the agent — CPU/RAM/disk/traffic every N seconds into a SQLite ring
  table, old data auto-downsampled (NO external dependency: no Prometheus/Grafana, the constitution
  holds). Sparklines on the dashboard cards (cards stay quiet); clicking a card opens a 24-hour / 7-day
  detail chart. Alert thresholds read the same data — charts and alarms are two faces of one substrate.
  Multi-location external uptime monitoring is deliberately out of scope: the honest answer is the
  heartbeat + "use UptimeRobot/360".
- **The alert channel's own health:** alerts go via two independent channels, mail AND webhook (if mail
  can't enter the queue it falls to webhook) + an outward heartbeat — the panel pings an external
  endpoint the operator chose every N minutes; when it stops, the alarm rings FROM OUTSIDE. The most
  likely failure is the death of the channel that carries the alert.
- Remote backup targets (S3/FTP) + restore drills as a product feature — **with two hardenings:**
  (1) **Panel-state disaster backup:** SQLite (consistent copy via the online backup API) + `secret.key` +
  DKIM/WireGuard keys + panel certificates in one archive, to the remote target with the same retention
  as domain backups. Losing `secret.key` = every sealed secret irrecoverable; "domains in backup but the
  panel's brain missing" is not accepted. (2) **Client-side encryption:** every archive leaving for a
  remote target is encrypted in the panel; the backup key is kept separate from `secret.key` and shown
  once at setup — the "backups unreadable without the key" honesty is written in the UI.
- **Customer self-service restore:** from the backup list, full-site / single dir-file / DB dump
  restores — with a confirmation modal + audit record. The most expensive answer to "will YOU solve
  every user's problem?" is restore; at 3 a.m. the customer reverts their own mistake themselves.
- **The secondary-DNS truth:** today ns1/ns2 point at the same machine (a single point of failure); a
  secondary PowerDNS (AXFR) on a cheap second VPS, or honest documentation (added July 11)
- **Managed DNS backend (Cloudflare-class) — the concrete provider for v0.3's abstraction:** a "DNS
  provider" choice in Settings; the Cloudflare backend = a scoped API token + zone auto-creation + a
  second writer on the `syncZoneToDNS` seam (same record set, different target). This is also the
  cleanest answer to the secondary-DNS single-point problem: handing DNS wholly to Cloudflare is simpler
  and safer than standing up AXFR on a second VPS. The panel never opens :53; by "security is the
  default" it RECOMMENDS this path but does not impose it (self-hosted PowerDNS stays the zero-dependency
  default for those who want it). The registrar NS delegation is shown honestly as a manual step +
  verified. This is NOT an external dependency of the panel itself — it is an operator-chosen optional
  backend (like the MariaDB↔PostgreSQL choice); the token is the operator's, and the panel runs fully
  offline.
- One-click panel self-update in the UI (update.sh's front end) · WebSocket live notifications
- **The update chain hardens:** the release-binary channel becomes primary — `update.sh` downloads the
  release tarball, **verifies a signature** (minisign/cosign; public key baked into install.sh, rotation
  plan written), verifies SHA256, skips the build step. No Go/Node toolchain needed on production
  (attack surface + OOM on small VPSes + bit-level drift close all at once); `--from-source` remains for
  dev/test servers. On verification failure: stay on the current version + "Needs attention" + audit record.
- **Post-update self-check:** an automatic smoke at the end — panel HTTP 200 + login render + agent
  socket ping + `PRAGMA quick_check`; if anything remains the output says "rollback.sh recommended", and
  the one-click flow offers a rollback button.
- **The failure matrix — proof that "panel dead, hosting alive":** panel process killed → web/DNS/mail
  keep serving (measured by test), the panel comes back via `Restart=on-failure`; a "break glass"
  runbook (diagnosis steps over SSH when the panel won't start) is written into D-008 as the emergency
  exception.
- **CelikPanel→CelikPanel migration:** an account-level export archive (domains + docroot + DB dump +
  maildir + DNS zone + DKIM key + subscription/quota metadata in one signed tar); on the target server
  the cPanel importer's inspect→confirm→apply flow recognizes this format too. The full form of "data is
  never held hostage"; changing servers is hosting routine.
- **Visitor statistics (minimal, deliberately bounded):** traffic measurement already reads the nginx
  access log; the same pass extracts daily hits / unique IPs / top 10 pages / top 10 referrers into a
  card on the domain Overview. Full analytics (sessions, geo, real-time) is out of scope — the honest
  answer: "put Plausible/GA on your site".
- **Audit integrity:** write failures are counted and surface in "Needs attention" (without blocking the
  action, but never silent); configurable audit retention + pruning.
- **Active sessions:** a session list in Settings (created, last used, IP) + single/bulk termination;
  an admin can drop a target's sessions from Users.
- **Clean start:** the panel marks catalog services it did not install itself as "foreign" (any catalog
  service with no `service.install` audit-log entry) and offers removal with the operator's consent —
  adopt or evict, the mirror of the Import philosophy. Field proof (July 16): Hostinger's Arch image
  shipped a dormant bind; the setup journey counted "DNS installed: Done". Builds on B3.
- **Dashboard SSL/TLS summary** (from the Plesk comparison, July 17): expiring-soon / valid /
  no-certificate counts in one card — the "certificate silently dies in 90 days" class becomes visible
  on the panel's face
- **Dashboard Mail Queue card** (from the Plesk comparison, July 17): Total/Deferred/Held plus one-click
  queue clear — the first place an operator looks at 3 a.m.
- **Self-diagnosis:** the operator's July 17 question is the design bar — "will YOU solve every user's
  problem?" The panel must check by itself the classes we diagnosed by hand: does the DNS delegation
  actually point at this server, **does the panel's own hostname resolve to THIS server** (Jul 18: the
  boston.celikhost.com record lived in frankfurt's zone, boston had no idea — that coupling was
  invisible), does the certificate renewal timer actually run, can the service config actually start the
  engine. A finding = a "Needs attention" row + one-click repair (the honest counterpart of Plesk's
  Repair Kit)

- **Catalogue widening — Python, databases on Arch, the migration lever (25 Jul 2026):**
  - **Python as a runtime.** There is **no Python in the panel today** — not a catalogue entry, not a
    line of code; the comment in `hosting_handlers.go` says "so go/python can be added without a table
    rebuild", but the entry was never opened. The three primitives Node uses already exist and are
    largely generic: a per-site systemd unit (`ApplyAppUnit`/`ControlAppUnit`/`AppUnitStatus`/
    `AppUnitLogs`), the reverse proxy in front of it, and the version drawer. Python adds three things:
    a **per-site virtual environment (venv)**, an **application-server choice** (gunicorn/uvicorn — the
    WSGI vs ASGI split is a product decision and cannot be hidden), and a "Python Application" card in
    Add Domain. **No use of the system python3:** the "system interpreter" leak deliberately removed
    for Node is not reopened here — the panel only runs what it installed itself. Django/Flask/FastAPI
    hosting is the equivalent of cPanel's "Setup Python App" and the place most free panels do badly;
    it is the largest market widening that requires no new concept.
  - **Databases on Arch.** MariaDB and PostgreSQL are not offered on Arch at all, because the catalogue
    has no "post-install initialisation" field (there is no way to express `initdb` /
    `mariadb-install-db`). **One catalogue field** opens both components, closes two cells in the
    distro matrix, and makes one-click WordPress possible on Arch. This is the measurable test of the
    "no OS lock-in" claim.
  - **DNS zone import/export (+ CAA).** Nobody switches panels without a path in; zone transfer is the
    cheapest migration lever, and owning our own authoritative DNS makes it structurally easier here.

  **Exit criteria:** on both distros a Python version installs from the panel and a single Add Domain
  form brings up a working Django/FastAPI site (venv per site, unit running, reverse proxy correct) ·
  MariaDB AND PostgreSQL install from the panel on Arch and the first database is created · those
  three cells in the distro matrix are no longer empty · a cPanel zone file imports and the domain
  answers queries correctly.

**Exit criterion:** a killed service alerts within a minute; a server with its plug pulled produces an
**external** alarm within 5 minutes · the restore drill's definition: on a clean VPS, `install.sh` +
panel-state restore + domain backups bring the full server up and DKIM-signed mail lands in the INBOX
**with the same keys** · no object in the remote store is unencrypted · a version update succeeds on a
production VPS with no Go/Node installed; a tarball with a broken signature is refused and lands in
audit · a customer restores their deleted `wp-config.php` from the panel on their own · every cell of
the failure matrix drilled at least once.

### v0.5 — Security Depth
- The WAF decision (ModSecurity or an honest alternative) · deep fail2ban integration · scheduled ClamAV site scans
- External-audit verification of secret encryption (implementation done in v0.3: TOTP + DKIM + WG keys under `enc:v1`)
- External security audit · dedicated IP plumbing (the sellable `extra_ip` — the gate marked "coming soon" since v0.35 opens)
- **API token management:** named tokens per user (crypto/rand, only the hash in the DB, shown once,
  scope: read-only/full, revocation); token'd requests pass the tenant-scope filter and land in audit.
  The "API-first" promise cannot be kept with an API unusable from curl.
- **SFTP/SSH key management:** the customer adds/removes public keys for the site user (ledger in the
  panel, the agent writes `authorized_keys` with full-state push); password login and shell **off by
  default** (internal-sftp + chroot docroot); `access.ssh_key.add` in audit. This is the path for
  agencies beyond plain FTP and for CI/rsync/git-hook flows; an in-panel terminal stays deliberately
  out of scope.
- **2FA depth:** 8 single-use recovery codes at enablement (argon2id-hashed, shown once); a "use a
  recovery code" branch at login; a "2FA required for admin/reseller" setting — when required, a user
  without 2FA is locked to the setup screen at first login.
- **The payment-integration decision record (D-0xx):** provider class = hosted-checkout /
  Merchant-of-Record (the Stripe Checkout / Paddle / iyzico class — card data never enters the panel,
  PCI scope zero); the panel is only a **webhook consumer**: a subscription↔provider-customer mapping
  table + one signature-verified, idempotent webhook endpoint (a ledger of processed event_ids) +
  `payment.event` audit. The event contract has three classes: "paid" → the period extends (a row in the
  payments ledger), "unpaid" → grace → suspension (the v0.3 machine), **"refund/chargeback"** — refund:
  the period's extension is reverted (`expires_at` shortens) + `payment.refund` audit + operator
  notification; chargeback: automatic suspension + a "Needs attention" row (a chargeback is also an
  abuse signal). Invoice PDFs are links from the provider. The manual "mark paid" mode remains for the
  provider-less operator; **in the first release the provider integration is operator-plane only —
  reseller collections continue via the manual flow (deliberate and written).** Constitution constraint:
  one binary + SQLite — payments are never embedded in the core.

**Exit criterion:** the external audit yields no high-severity findings · a customer buys and uses a
dedicated IP from the panel · the single curl example in the docs opens a domain with a token; a revoked
token is 401 immediately · no plain secret is grep-able in the DB dump or dataDir · a forged webhook
event extends `expires_at` on the test server; the same event a second time is a no-op; a refund event
reverts the extension · login with a recovery code succeeds once, 403 the second time; with enforcement
on, a reseller without 2FA cannot log in · a keyed customer sees only their own docroot over sftp; a
shell attempt is refused.

### v0.6 – v0.9 — Beta Program
- OpenAPI documentation (proof of the API-first promise) · admin and user guides TR+EN
- **Help center + deep links:** a static TR+EN site generated from markdown under docs/ (simple
  generator, no external dependency enters the panel); a help icon in every panel page's header going to
  the doc page via a stable slug (`pages.<id>` → `/help/<lang>/<slug>`); a broken-slug test in B5 CI.
  Documentation not linked from the panel is born dead.
- **Browser first-boot:** when no admin exists in the DB the panel enters a one-time first-boot mode —
  the admin is created from the browser with a single-use token produced by `install.sh`; the mode closes
  permanently once an admin exists. The "return to the terminal, run a CLI" step in the middle of the
  "install.sh → login screen" promise disappears; placed in this release, not rushed, so that no
  unprotected first-setup endpoint ever exists.
- **Self-signup + abuse brakes** (the free tier's scaling path): an e-mail-verified signup form —
  **default OFF** (security is the default), the operator who enables it binds it to a free plan;
  a disposable-domain list + per-IP signup rate limit. Brakes: an hourly outbound-mail cap on the free
  plan (on the mail_policy pattern), an optional 24-hour send-hold on new accounts, resource-creation
  rate limits. Trials: `trial_days` per plan → `expires_at` fills automatically; on expiry the account
  drops to free **in forced-downgrade mode** (the v0.3 rule — an unattended transition can't hit a 409).
  One bad customer can burn the IP reputation earned with the deliverability screen — signup does not
  open without brakes.
- **Command palette:** Ctrl+K — nav + domain list + service pages in one fuzzy search; navigation only
  in the first release (no action execution — the security surface doesn't grow), a "?" shortcut list
  beside it; dependency-free, respects the role filter. It would be a luxury on today's 12 routes; the
  right moment for a beta operator's daily efficiency is here.
- **SUPPORT.md (TR+EN):** before 1.0 only the latest minor is supported, security fixes are patched onto
  the latest minor; the upgrade path is "from any release to the newest, the migration chain proven with
  fixture tests"; at v1.0 it widens to N−1. Published together with the beta invitation — the answer is
  not improvised in the moment.
- **KVKK/GDPR minimums:** data export = the customer-triggered form of the migration archive; account
  deletion covers, beyond the DB cascade, the maildir + docroot + the account's backup archives and
  produces a "what was deleted" report; log/audit retention configurable and documented.
- Real external beta users; their walls become fixes (the alpha model, scaled)
- Performance targets measured and enforced (<100 ms API)
- **The license/business-model decision** (suggestion: open core) → repo visibility accordingly. With
  the decision, two commitments go to DECISIONS: (1) **the two-plane separation** — the operator
  charging their own customers (a panel feature) and CelikPanel charging the operator (a license) are
  separate planes; no table/endpoint of the first ever carries telemetry/license checks for the second,
  and the panel works fully offline. (2) **Price positioning:** the price is per server and **never**
  scales with hosted accounts/domains — cPanel's post-2019 per-account model triggered the largest
  migration in panel history; announced late, beta arrives with the "this too will cPanel-ify" suspicion.
- **The "Why CelikPanel" document** (docs/WHY.tr.md + WHY.md): three columns (CelikPanel/cPanel/Plesk) —
  install footprint, default security, one binary vs a forest of services, TR-first, the price
  principle; plus a reasoned confession of deliberate gaps (every refusal links to a DECISIONS record).
  We do the comparison ourselves, honestly, before competitor marketing does.
- **Light brand customization:** a global "panel name + logo + accent color" setting (one table row;
  login + sidebar + mail templates read one source). Per-reseller branding is post-1.0.

**Exit criterion:** ≥3 external operators run real sites for ≥1 month; documentation answers their
questions before we do · on a clean VPS an admin is created and logged in from the browser without
returning to SSH; a token-less/second attempt is 403 · no account can be opened with an unverified
e-mail; a free account is limited at the N+1st mail in an hour · every panel page has a working doc
link, the broken-slug test is in CI · the beta announcement shipped with the price principle and the
WHY page · SUPPORT.md is live and the v0.3.0→current jump test is green in CI.

### 🎯 v1.0 — General Availability
- Clean VPS → a working panel in minutes, documented, self-updating
- "Domain → live site" 100 times in a row without error (the Phase 1 promise, now in CI)
- Migration from cPanel proven repeatedly · pricing/license published

**Exit criterion:** a stranger with no help beyond the documentation reaches, on a clean VPS, from
install to a live HTTPS site and mail landing in the INBOX, on their own · "domain → live site" 100/100
in CI · at least one real cPanel migration and one CelikPanel→CelikPanel move proven in production ·
the pricing/license page and the SUPPORT policy are published.

### Post-1.0 — horizon *(demand persists, fantasies don't)*
Each only with real demand, in keeping with the deliberate non-goals:
- Multi-server (an inter-panel trust model + **sibling-server DNS auto-registration**: a new CelikPanel
  server registers its own hostname A record with the other CelikPanel that is the zone authority for the
  brand domain, using an operator-provided API token — the productized form of today's manual
  boston.celikhost.com step; v0.3's path (b) external-DNS covers N servers until this lands) · a BSD
  agent backend · billing integrations (WHMCS etc.)
- **Plesk/DirectAdmin importers** — only once real demand exists (≥5 concrete migration requests); the
  cPanel importer's inspect→confirm→apply pattern is reused. Until then the honest answer is documented:
  "Coming from Plesk? A manual migration guide, for now."
- **Per-reseller white-label** (custom login domain, reseller logo) — light global branding is in
  v0.6–0.9; per-reseller only if reseller sales demand it.
- **A hosted service** (CelikPanel itself renting VPS+panel) — not rejected outright like the
  marketplace, but not designed now; if it opens, it carries not one line of telemetry/phone-home into
  the self-hosted product's architecture.
- A marketplace **never** (the AltaVista mistake).

---

## Deliberate Non-Goals

Simplicity is being able to say no. These are absent **on purpose** — and most of the refusals are the
product itself:

- ❌ **A Docker/container layer** — the target market is classic hosting; native is correct. It is also
  the competitive sentence: a Plesk install brings hundreds of packages and dozens of services;
  CelikPanel brings two binaries. That difference is not a gap — it is the product.
- ❌ **A management screen for every conceivable service** — what isn't installed is invisible.
- ❌ **Theme/skin markets, portal showcases, a plugin marketplace** — the AltaVista mistake; moreover the
  no-third-party-code guarantee is the natural consequence of "only the Panel reaches the root Agent."
  A plugin market sells that guarantee away.
- ❌ **The general app-catalog race (Softaculous's 400+ entries)** — a shallow-wide catalog is a
  maintenance black hole and demand is overwhelmingly WordPress. The catalog grows one entry at a time
  only when (a) proof of real demand and (b) WordPress-quality end-to-end install (official tarball,
  verification, full configuration) hold **together**. Position: deeper than anyone on WordPress,
  deliberately narrow on the catalog.
- ❌ **Full visitor analytics** (sessions, geo, real-time) — the minimal log card in v0.4 suffices; for
  more, the honest answer: put Plausible/GA on the site. An AWStats clone fails the simplicity filter.
- ❌ **An in-panel terminal** — attack surface and simplicity; SFTP/SSH key management (v0.5) serves the
  legitimate need.
- ❌ **Multi-server / cluster (for now)** — no distributed-system dreams before one server is flawless.
- ❌ **External dependencies for the panel itself** (Redis, external DB, message queue) — one binary +
  SQLite stays. Even payments are not embedded in the core; the panel is only a webhook consumer (the
  v0.5 decision).
- ❌ **Telemetry / phone-home** — the panel makes no outbound request of its own volition (the
  operator-configured heartbeat and inbound webhooks excepted). Whatever the license model, this does
  not change.
- ❌ **BSD support (for now)** — **but the option is deliberately preserved, and never as a fork.** The
  panel↔agent RPC contract is OS-neutral by design: the panel (HTTP/SQLite/UI/business logic) is
  portable Go that already cross-compiles to FreeBSD; only the agent's "hands" (systemd/apt/nftables)
  are Linux-specific. If real demand arises (e.g. a Linux trust crisis pushing hosts to BSD), the move
  is a BSD agent backend behind the same RPC surface — work measured in weeks, one product. There will
  never be two CelikPanels. The discipline that keeps it cheap: new agent features keep the "what" (RPC
  surface) separate from the "how" (exec calls) — the code is already written that way.
  *(Decision: July 8, 2026.)*

---

## Historical Snapshot — August 29, 2026

**Version:** now single-sourced — version and commit are linked into BOTH binaries, served from
`/api/v1/panel/version`, read back by the panel footer, and a panel/agent build mismatch raises a
warning. The hand-typed "v0.1.0" literal is gone. Both test servers verified on the same commit.
**Current live identity:** the earlier two-server matching-commit observation is
historical evidence, not proof of either server's current state. Reverify live
values in [the dated live-state record](docs/LIVE-STATE-2026-08-29.md).
**Position on the ladder:** v0.2 in progress; many v0.2.5 items closed along the way; **a new v0.2.6
step now sits in front of v0.3** because of the three data-loss defects above.

**Live breakages closed in the second half of July (all proven on both servers):** the spam filter
actually filters now (single-owner milter chain; a GTUBE test was rejected on both distros) · Postfix
on Arch was rejecting EVERY incoming message (the lookup-table type is now discovered:
lmdb/hash/btree/texthash) · WireGuard actually works after install · the agent no longer trusts
request-supplied paths or URLs (path allow-list + symlink refusal; the repo key comes from its own
catalogue) · the config editor writes → validates → **rolls back** (nginx/postfix/dovecot/apache
validators) · seat conflicts are enforced in the agent (the UI blocked them, the API did not) · an
alias unit no longer counts as installed · the update script no longer installs a stale build.

**New capabilities:** a **Help** button on every management page (bespoke content for 21 components
plus three generic fallbacks by kind, all bilingual; a page without help is structurally impossible) ·
the ban on empty management pages (D-019) · the distro support matrix is **generated from the
catalogue** and a guard test fails when it goes stale · a "not offered on this distro" badge · the
monitoring page · Valkey added to the catalogue (the real cost of a new component: 2 source files,
zero Go code).

**Debt status:** the generated OpenAPI client, one declarative route/authz table, UI consolidation and
measured end-to-end latency remain open. D-017 defines canonical offering identities, but persisted Store
rows still use plain legacy IDs; adoption requires an explicit data/API compatibility migration. CI is no
longer compile-only: it covers Go formatting/build/vet/test/race, shell and repository contracts, the
locked web build and dependency gate, and reproducible release artifacts.

**Repository snapshot:** run `bash tools/repo-metrics.sh` against the exact
commit whenever a measurement is needed. The output is generated source-tree
evidence and is intentionally not copied into this roadmap as hand-maintained
line, file, route, migration or command-site counts. Record the command output
with its full commit in review or release evidence. It never proves either
server's deployed build.


---

## How This Document Is Updated

- **Every significant decision lands here via a commit.** The reasoning goes to
  [DECISIONS](docs/DECISIONS.md), the debt to [AUTOPSY](docs/AUTOPSY.md); this file records only the
  place on the ladder and the criterion.
- A new request **cannot enter as "later"**: it is either written onto a version step with a measurable
  exit criterion, or added to Deliberate Non-Goals with a reason. There is no third way.
- The next step does not start before the exit criterion is met; if a criterion must change, the change
  is committed together with its reason (no silent dilution).
- Every edit refreshes the "Last updated" date and the "Where We Are" section; the Turkish original
  (ROADMAP.tr.md) and this mirror update in the same commit.
