# Leftovers audit (alpha.130)

The owner's rule of 8 Nov 2026: **no leftovers, ever.** A flow that replaces
or removes something cleans up what it made obsolete in the same operation.
User data is never deleted silently: it is migrated first, or the owner
chooses explicitly.

This file lists what a removal still leaves on the server after alpha.130,
why, and the proposed choice for the owner. It is a record of open decisions,
not a claim that they are implemented.

Sahibin kuralı: hiçbir yerde kalıntı yok. Bu dosya alpha.130'dan sonra bir
kaldırmanın sunucuda hâlâ bıraktıklarını, nedenini ve sahibe önerilen seçimi
listeler; uygulanmış olduğu iddiası değildir.

## Cleaned up in alpha.130

| Flow | What now goes with it |
|---|---|
| PostgreSQL major or whole component (Debian/Ubuntu) | Packages are purged (no dpkg `rc`), including the major's `postgresql-client-NN`. The cluster's config, standard data directory and logs are removed after the purge. A cluster that holds anything beyond what initdb made is archived first in `/var/backups/celikpanel/postgresql/`. The removal is still refused while a cluster holds a database. |
| Leftover PostgreSQL clusters from older releases | A cluster holding only initdb's objects is removed once after the panel starts, with an audit row. A 30-day archive (`…-auto.tar.gz`) is kept and deleted by the Agent at a start after it expires. Any other cluster waits for the administrator's **Archive and remove**. |
| Dovecot | Removed when still CelikPanel's: the mailbox 90% warning timer, service and script (the timer is stopped and disabled first); the drop-ins `90-celikpanel-quota`, `96-celikpanel-sieve`, `97-celikpanel-submission`, `98-celikpanel-tls` and `99-celikpanel`; and the marker folder `/var/lib/celikpanel-quota-warning`. Empty `/etc/dovecot/conf.d` and `/etc/dovecot` are removed too. |
| Postfix | Removed when still CelikPanel's: `celikpanel_transport`, `celikpanel_domain_mail_options`, `celikpanel_login_map`, `celikpanel_sni`, `celikpanel_sender_access`, `celikpanel-relay-sasl` (holds the relay password) with their `.db/.lmdb/.cdb` indexes, and `celikpanel-subaddressing`. |
| postfwd | Removed when still CelikPanel's: the systemd drop-in `postfwd.service.d/celikpanel.conf` and the rule file CelikPanel wrote (a package's own file is left to the package manager). Also removed: CelikPanel's list folder and, on Debian/Ubuntu, `/var/lib/celikpanel-postfwd`. |
| Node.js versions | Removing the last version also removes the empty `/opt/celikpanel/runtimes/node`. |

A file CelikPanel wrote is removed only while it is still CelikPanel's, by the
same rule its writer uses: a matching digest line, CelikPanel's marker line,
or a file CelikPanel rewrites whole every time. A file the owner changed is
kept, and the removal's answer names it.

## Remaining leftovers (owner decisions)

| Leftover | Why it remains | Proposed choice |
|---|---|---|
| MariaDB: `/var/lib/mysql`, `/etc/mysql`, packages in dpkg `rc` | Removal uses `apt-get remove` (alpha.107, D-038 §8). The databases are user data, and purging is not proven to keep them. | Same as PostgreSQL. Refuse while a user database exists. Otherwise archive `/var/lib/mysql` and `/etc/mysql` under `/var/backups/celikpanel/mariadb/`, then purge and remove. |
| Roundcube: `/var/lib/celikpanel-webmail` (with `db/roundcube.sqlite3`) | This is the opposite problem: removal **deletes** the tree, including users' contacts, identities and settings, without an archive. Behaviour is unchanged in alpha.130. | Archive the sqlite database under `/var/backups/celikpanel/webmail/` before removal, and name the archive in the answer. |
| WireGuard: `/etc/wireguard/wg0.conf` (keys and peers), `/etc/sysctl.d/99-celikpanel-vpn.conf` | The keys and peers are the owner's access. Removing them locks peers out for good. | Archive `wg0.conf` (root-only) and remove it with the package. The sysctl file is CelikPanel's and can go without asking. |
| Mail: `/var/vmail` (mail), `/etc/dovecot/users` (mailbox password hashes), DKIM keys in `/var/lib/celikpanel-dkim`, mailbox Sieve scripts | User data and credentials, needed to restore the mail service. | Keep. Offer an explicit "Archive and remove mail data" in a removal of the whole mail stack. |
| Postfix: `/etc/postfix/vmailbox`, `virtual`, `vmailbox_domains`, and CelikPanel's lines in `main.cf`/`master.cf` if the package's purge leaves those files | The names are generic and an owner may keep their own. The panel can write them again from its records. | Remove them only when they carry a CelikPanel marker (needs a marker in a later release). |
| `/etc/ssl/celikpanel/_mail` (copies of the mail certificates and keys) | Shared by Postfix and Dovecot. Removing it with one of them would break the other. | Remove when the last mail component goes. |
| Postfix spool `/var/spool/postfix` | The package's purge deletes it, so queued mail is lost. | Before removal, warn when the queue is not empty (`postqueue -j`). |
| Arch: `/etc/php/conf.d/celikpanel-phpmyadmin.ini` (enables `mysqli`, `iconv`) | Arch has one PHP. Sites (for example WordPress) may rely on `mysqli`, which this file turned on, so removing it with phpMyAdmin could silently break sites. | Remove only when no site's PHP needs the extension, or move it into a site-runtime file CelikPanel owns. |
| Arch: `/var/lib/postgres/data` | `pacman -Rns` does not remove the data directory, and its databases are user data. | The same holds check and archive as on Debian/Ubuntu (needs Arch's single-cluster reading). |
| PostgreSQL data directories outside `/var/lib/postgresql/V/N` | CelikPanel did not place them. | Refused with the path. The owner removes them. |
| DNS engines: the old engine's packages and zone trees; `dpkg-statoverride` on `/var/cache/bind` | Kept for rollback of an engine switch. | Remove with the old engine after the switch is final. Remove the statoverride with BIND. |
| fail2ban jail and filter files, `/etc/celikpanel/firewall.nft`, `celikpanel-database-remote-firewall.{service,timer}` | Owned by other workstreams (ops and database-remote). They are config only. | Remove with their component, by the same "still CelikPanel's" rule. |
| `*.celikpanel-removed-<time>` copies of changed units, owners' drop-ins | They are kept on purpose: the owner changed them. | Keep. |
| ClamAV quarantine, `/var/lib/clamav` signatures | The quarantine holds user files. | Keep the quarantine. Signatures go with the purge. |

## Known limit of the fresh-cluster reading

A leftover PostgreSQL cluster counts as "fresh" only when the files show
nothing made after initdb:

- every database directory and relation file is numbered below 16384;
- there are no tablespaces;
- pg_authid holds no role but CelikPanel's own.

Objects without storage can't be seen this way: functions, views, types and
comments made in the `postgres` or `template1` database. The automatic removal
therefore keeps an archive of the removed cluster under
`/var/backups/celikpanel/postgresql/` (`postgresql-V-N-<time>-auto.tar.gz`,
root-only) for 30 days. The Agent deletes it at its first start after that.
The pg_authid reading also assumes 8 KiB little-endian pages (PostgreSQL 12 or
later). Anything else counts as "may hold data", so it is never removed
automatically.
