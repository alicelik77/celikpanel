# v0.1.0-alpha.111 (candidate)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.111.tr.md)

This is the release that follows v0.1.0-alpha.110. **It is a candidate, written on
2026-10-29; it is not published and nothing from it is on an installed server.
Only the owner updates installed panels, from the panel's own update screen.**
Written on the branch `feat/setup-wizard-v2` from `main` at the publication record
of alpha.110 (`79059a644`; alpha.110's release commit is
`3abeb40d137e2664988a2a3735f2a44361f5774b`).

This one release ships everything planned as alpha.111 to alpha.115: the new
setup wizard, joining the second server by code, the redesigned DNS page, the
setup link between the two servers and the fixes from four virtual machine runs.
It is the Beta 1 setup.

## Who should install it, and how

**New servers.** The test servers are reinstalled from it. No upgrade path from
earlier alpha setups is promised: a server set up with an earlier alpha's setup
should be reinstalled, not updated, to use the new wizard. The path for a fresh
install:

1. Install a fresh operating system on each server (Debian 13 and Ubuntu 24.04
   were the ones tested).
2. On each server run the installer from celikpanel.net as root (the command on
   the download page).
3. On the first server open the panel and go through the wizard: the main
   domain, one or two servers, the roles, the names and where DNS lives.
4. With two servers on your own DNS, the first server waits for the second. Make
   a join code on the first server, then on the second server choose "Join the
   first server with a code" and enter the first server's address and the code.
   The second server takes the shared answers from the first.
5. When the registrar card says so, set the domain's nameservers at your
   registrar and, for nameserver names inside the domain, their IP records
   (glue). The card checks the registry by itself and says when it is right.

## What changed

- **A new setup wizard.** The main domain comes first, then one or two servers,
  the server's short name, the roles (websites, email, DNS), optional components,
  the names proposed from the main domain (your edits are kept), where DNS lives,
  a summary and the install. Every step says what it installs.
- **Where DNS lives.** *Your own pair*: the two servers keep the domain's DNS,
  one as the main DNS server and one as the backup, with BIND or PowerDNS; either
  server can be the main. *An external provider*: no DNS software is installed;
  the screen lists the records to add and shows what the internet answers for
  each. *Standalone*: one server keeps its own DNS, with both nameserver names
  on it.
- **Join by code.** The second server's address is never typed on the first.
  The code is shown once, is valid for 24 hours, allows at most 5 wrong tries
  from one address (50 in total), and is checked against the first server's key
  before anything is sent, so a wrong address never receives it. A lost answer
  is retried as the same request and gives exactly one join.
- **Registrar card.** It names the nameservers and glue to set, asks the
  registry's own servers and our servers, and says correct, not yet, waiting
  for the backup DNS server, or could not be checked. An unread answer is never
  "not yet".
- **The DNS page.** Server → DNS starts with one sentence about the whole
  picture (who acts and what to do next), then the servers, the names with what
  the internet answers, the mail records and the records you added. The previous
  settings are kept under Advanced. "Move mail" changes the one MX record after
  you confirm, when you kept the old mail server.
- **Existing records and mail records.** Setup reads what the internet answers
  for the domain's usual names; you import or skip each record. With email it
  writes MX, SPF (merged), DMARC and DKIM. Reverse DNS is listed as a task for
  your server provider; setup does not wait for it.
- **Mail after DNS.** The mail steps run after the DNS steps on both servers, so
  mail always has its DNS identity.
- **One mail receiver.** Exactly one server receives the main domain's mail.
  Choosing mail on both servers is refused at review with what to change.
- **Waits and recovery texts.** Every wait and failure says why, who acts and
  what to do next. A server still starting after a reboot is a wait, not a
  failure. Another change running on the server holds the start and the run
  waits for it. A clock behind the certificate authority is said with its
  seconds and `timedatectl set-ntp true`. The setup link stays until the backup
  says it is ready, and its texts name servers by role ("the other server",
  "the main DNS server"). Your own native DNS edits are noticed, kept and
  reported, never overwritten.

## Known limits

- The join's source check is IPv4 only.
- A second server that loses its link must join again with a new code.
- After your own native DNS edit there is no adopt or hand-back yet: the change
  is kept and reported, and CelikPanel publishes no DNS change there meanwhile.
- After the install and in recovery, PowerDNS's origin is checked by its
  version rule only (Debian's own archive build of the tested 4.9 series).
- There is no "add a second server later" and no "Move main DNS" yet.
- Mail on both servers for one domain is refused.
- A resumed install refused as busy is a verified failure; it is not resumed
  again.
- A backward jump of the clock during a mail step can fail that step; Edit plan
  and start again recovers it.
- Not measured on real systems: licence behaviour (the runs used an acceptance
  build), a busy wait in the middle of a run, the mail certificate at a large
  clock skew, a link refusal that is a bare HTTP 401, IPv6 and reverse DNS.

## Upgrade and recovery

No schema change (schema 46). The wizard's answers, join codes (only as keyed
hashes), the setup link's sealed values and the list of records setup wrote are
kept in `panel_settings` and the existing setup and DNS tables. Every secret a
server must use again is sealed under its own key file; a copy of the database
alone gives no code, credential or enrollment. The link credential and the join
code are removed on both servers when setup completes.

DNS replication between the two servers is the DNS software's own zone transfer
and does not need the panel (D-022). In the first run, with the panel and the
agent stopped, an owner's native `rndc` edit reached the backup by transfer;
afterwards the panel noticed it and kept it.

An interrupted step resumes as the same operation after a reboot or a panel
restart; polling never starts a change. The panel itself recovers by the ordinary
release rules (no schema change). Going back to alpha.110 in the middle of a
setup was not measured; for these fresh test servers the way back is a reinstall.
Installed panels are updated only by their owners, from the panel's own update
screen.

## Evidence

- Four runs on disposable virtual machines in a lab network, each from a fresh
  operating system through the customer installer and the wizard: lab DNS root
  and parent, a lab resolver and a lab certificate authority (pebble). No real
  server, registrar or certificate authority was contacted.
  - First run (product commit `c2d60e574`): the Debian 13 + Ubuntu 24.04 BIND
    pair and the external-provider layout passed; join refusals, replay, reboots,
    name clashes and the registrar card behaved as written. It found five
    defects (MariaDB refused on fresh images, the standalone zone kind, mail
    before DNS, the PowerDNS version, an engine change with no recovery), all
    fixed.
  - Second run (`b86ab6fa4`): standalone BIND and PowerDNS as the main passed,
    and so did the engine-change recovery; five further defects were found and
    fixed.
  - Third run (`cfcadd3f1`): those fixes passed; six further defects (a booting
    host, the clock reason, DKIM over the link, the mail certificate wait, the
    backup naming itself as the mail receiver, one text) were found and fixed.
  - Fourth run (`68a9915a0`, this candidate's product code): both two-server
    layouts reached ready end to end from fresh installs. A reboot during a real
    DNS wait, during the main's MariaDB step and later during access DNS
    resumed with nothing done twice; a start during a Components install was
    refused with `server_setup_host_busy`; a clock 200 s behind was named and
    finished after the fix; mail on both servers was refused at review; mail on
    the backup in Option A got its DKIM over the link. The earlier checks (join
    code, registrar card, owner independence, standalone, PowerDNS main after a
    reboot, Move mail) passed again.
  - Summaries and archives:
    [first](../deploy/e2e/dns-pair-acceptance/evidence/beta1-20261029/SUMMARY.md),
    [second](../deploy/e2e/dns-pair-acceptance/evidence/beta1b-20261029/SUMMARY.md),
    [third](../deploy/e2e/dns-pair-acceptance/evidence/beta1c-20261029/SUMMARY.md),
    [fourth](../deploy/e2e/dns-pair-acceptance/evidence/beta1d-20261029/SUMMARY.md).
    The archives were scanned for keys, codes, credentials, passwords and
    cookies before they were committed: clean.
- Tests on the candidate tree: Agent 4870 passed, 29 skipped (as root in a clean mount namespace), with one known failure of this test guest's environment (`TestTheReal1620ArchiveVerifiesWithThePinnedKey`); Panel 3601 passed, 6 skipped; every internal package passes; the race shards were not run locally (CI runs them); web 1980 passed, the type check and the
  production build within its budgets; the release sequence, download portal and
  signed manifest contracts pass.

Constitution: invariants 1 (the owner's services and native edits), 2 (unknown
is never "not yet" or failure), 4 (every step resumes as the same operation) and
6 (waits and progress apart from access; polling changes nothing), with D-021,
D-022, D-024 and D-025. Nothing was measured on an installed server. A candidate
is not acceptance; P0.1–P0.5 stay **partial**; no P0 item is closed or advanced.
