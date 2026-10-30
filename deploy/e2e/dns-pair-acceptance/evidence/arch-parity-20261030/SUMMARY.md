# Arch parity — real-system evidence (2026-10-30)

Branch `feat/arch-parity`, merged into `release/alpha113`. Disposable QEMU
guests in the `archlinux` WSL lab, lab DNS and a local Pebble ACME CA.
Acceptance builds: licence behaviour is not evidenced. No installed server,
celikpanel.net, licence service or real CA was contacted.

Archive: `arch-parity-evidence.tar.gz`, SHA-256
`f5f94ef46afc5c0aa5d576799dcc9c511916d7e882f81d3078f4ccc76d28778f`
(secret scan of 416 files: clean). Driver logs and `dists.jsonl` sit beside it.

| Run | Setup | Result |
|---|---|---|
| ap-a | Arch standalone, first try | found: OpenDKIM tables not readable by the `opendkim` user (fixed) |
| ap-b | Arch standalone | found: nginx drop-in directories root:celikpanel made certificate renewal read "required" (fixed) |
| ap-d | Arch standalone: web + mail + BIND + phpMyAdmin + Rspamd + Roundcube | 19/19 steps complete, about 4.5 minutes |
| ap-e | the same, after the fakeroot and MariaDB fixes | 19/19 complete; one owner "Retry reviewed handoff" (see open items) |
| ap-c | Debian 13 BIND main + Arch BIND backup, joined by code | both complete; the Arch backup answers the zone authoritatively with the same SOA serial |

Checked on the Arch guests: `doveconf -n`, `postfix check`, `nginx -t` and
`named-checkconf` clean; IMAPS 993 and submission 587 serve the Pebble
certificate and verify; OpenDKIM running with DKIM/MX/SPF/DMARC in the zone;
PHP `mysqli` and `iconv` loaded; `certbot-renew.timer` enabled; the security
updates card reads `known` (count and security count); the MariaDB editor
writes and reads back `/etc/my.cnf.d/server.cnf`, MariaDB listens on 127.0.0.1
only; Fail2ban installed from Components with the sshd jail active.

Still closed on Arch, each with an owner-facing reason: PowerDNS (Arch ships
5.1.5; the engine switch, journals and recovery are proven only with 4.9),
Redis (Arch ships Valkey), phpPgAdmin and SpamAssassin's Postfix bridge
(AUR only; official repositories only). Security updates: no automatic
upgrade on Arch; the card counts waiting updates (checkupdates) and known
vulnerabilities (arch-audit) and says the administrator runs `pacman -Syu`.

Open: PowerDNS on Arch; an Arch main with a Debian backup; reboot and
owner-independence drills on Arch; ClamAV (lab mirrors blocked); mail
delivery between mailboxes; webmail on its own address on Arch. Product
observation (not Arch-specific): a Components install preview requested
while setup ran held the host lock, the mail enrollment worker was refused and
setup waited for the owner's "Retry reviewed handoff".
