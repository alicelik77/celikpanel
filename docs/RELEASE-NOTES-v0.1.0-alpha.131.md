# v0.1.0-alpha.131

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.131.tr.md)

A small release with one fix. On Boston the owner moved PostgreSQL from port
5433 to 5432, and remote access to databases stopped working. It failed
closed: nothing was exposed, but no customer could connect from outside any
more. **Only the owner updates installed panels, from the panel's own
update screen.**

## Why it stopped

CelikPanel's record of what it had set up for remote access stayed keyed to
the old port. On the new port, CelikPanel's own settings (listening on the
network, `auth_delay`) looked like the owner's, so every rule was refused,
and the firewall filter kept a stale entry for the old port.

## What changed

- **The record follows the cluster.** A PostgreSQL entry now names the
  cluster's data folder (record version 2). A version-1 record is converted
  when it is read, and its first apply runs in full.
- **Every apply repairs a record whose port lost its cluster:** by the data
  folder, or, for an old record, by CelikPanel's own settings found in
  exactly one cluster. When it is not clear which cluster is meant, nothing
  is opened.
- **A port change moves the record and the filter** (and the cluster's old
  Fail2ban jail) at once, and the panel then asks for a full apply.
- **No restart for customers' addresses while the server switch is on.**
  When the last address is removed, the engine keeps listening on the
  network with TLS and `auth_delay` (MariaDB keeps its option file), so the
  next customer needs no restart. Only turning the server switch off undoes
  these settings.
- **MariaDB is not affected** (the panel offers no port change for it).
- **The customer's card:** rows laid out properly; a new end date is saved
  only with an explicit Apply (or dropped with Discard); the client settings
  show the server's own host name instead of the mail server's name; an
  owner's setting is described in calm words; normal spacing; the manual
  refresh button on the Databases list is gone (the list refreshes by
  itself).

## Lab results

Lab (QEMU, no real servers): Boston's problem was reproduced on Ubuntu 24.04
with 130. After the update to 131 remote access healed by itself in 404 s,
without a restart. An encrypted connection worked and an unencrypted one was
refused. Removing the last address kept the engine listening; adding an
address again worked within 1 s. A port change from 5432 to 5434 was
followed in 6 s.

Evidence: [`deploy/e2e/dns-pair-acceptance/evidence/a131/`](https://github.com/alicelik77/celikpanel/tree/e52d8c641/deploy/e2e/dns-pair-acceptance/evidence/a131)
on the lab branch `lab/alpha114-acceptance` (commit `e52d8c641`).

## Known limits

- On 130 the `hostssl` line stayed in `pg_hba.conf` after a port change; 131
  repairs it.
- The administrator's card shows the engine as "open" even with no address
  listed.
- The larger redesign (preparing everything when the server switch is
  turned on, with one confirmed restart) and the rule that every screen
  saves only on an explicit action come in alpha.132.
- Earlier limits still apply (see alpha.130).

## Evidence

No migration (schema stays 77). The Agent's record on disk goes from
version 1 to version 2. A 130 Agent refuses a version-2 record and changes
nothing (it fails closed), so after a rollback to 130 remote access stays
closed until 131 is installed again. Release sequence 130 → 131.
