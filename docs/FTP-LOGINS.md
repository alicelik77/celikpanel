# Additional FTP/SFTP logins of a site

*alpha.128, gap G16. Design note of the data-access workstream (G16 section of
docs/A128-DATA-ACCESS-DESIGN.md), built and proved in the lab. D-022 (owner
independence), D-024 (operation guidance), D-025 (resilience contract).*

A site already has one access account: the site's own system account
(alpha.117, docs/ACCOUNT-ACCESS.md). An additional login lets the site's
owner give a designer or a colleague their own user name and password that
sees **one folder of the site and nothing else**: SFTP and, where vsftpd is
CelikPanel's, FTP over TLS. A login can never open a command line.

## 1. What a login is

| | |
|---|---|
| Account | A local account with the **site account's uid and primary group** (`useradd -o`), the name the owner chose (it must contain a hyphen, so it can never be a site account's name), shell `/usr/sbin/nologin`, home `/run/celikpanel-ftp/<login>/./files`, member of `celikpanel-site`. Locked until the very last step of its creation. |
| Root | `/run/celikpanel-ftp/<login>` (root:<site group> 0750), inside `/run/celikpanel-ftp` (root:root 0711). |
| Folder | The chosen folder of the site, bound onto `/run/celikpanel-ftp/<login>/files` by the sandbox helper. |
| Unit | `/etc/systemd/system/celikpanel-ftp-<login>.service` (oneshot, `RemainAfterExit`, `WantedBy=multi-user.target`): `sandbox-helper ftp-bind` at start, `ftp-unbind` at stop. |
| SFTP | One `Match User <login>` block in `/etc/ssh/sshd_config.d/60-celikpanel-file-access.conf`: `ChrootDirectory /run/celikpanel-ftp/<login>`, `ForceCommand internal-sftp -d /files -u 0027` (`-R` while the subscription's disk is full), `PubkeyAuthentication no`, `PermitTTY no`, `DisableForwarding yes`, `PermitTunnel no`, `PermitUserRC no`. The login is also excluded from the "every other hosting account" deny block. |
| FTP | The login's name in `/etc/vsftpd/celikpanel-users.list`, its file in `/etc/vsftpd/celikpanel-users/<login>` (`write_enable=NO` while the disk is full), and `passwd_chroot_enable=YES` in `vsftpd.conf` once any login exists: vsftpd chroots at the home's `/./` (the root-owned jail) and starts in `/files`. TLS is required for login and data, as for site accounts. |
| Record | The drop-in's login lines (`# celikpanel-login login=… site=… subscription=… domain=… folder=… readonly=…`) are the server's record of who has a login. The Panel keeps a row per login (`ftp_logins`, migration 072) for the plan's count; never a password. |

### Why the site's uid, not an account of its own

The site's files are `site account:web server group`, folders `2750`, files
`0640` (cmd/agent/hosting_layout.go). A login with a uid of its own could not
enter the folder at all, and making it writable for a group would let the web
server write the site's files. Access lists fail on the create mode (an SFTP
client's `0644` masks the site account's named entry to read only, so PHP
could not change what the login uploaded) and leave entries for deleted uids
behind. With the site's uid every upload is the site's, exactly like the site
account's own uploads: PHP edits it, the web server reads it through its
group, the disk count is unchanged, and removing a login leaves no file
without its owner.

The cost is that a login is not a separate identity for programs that look at
uids. That does not widen anything, because a login runs no program: SFTP is
`internal-sftp` inside sshd, FTP is vsftpd, forwarding and tunnels are off,
the shell is `nologin`. `ps` and audit records name the site account; sshd and
vsftpd log the login's own name.

## 2. Why the helper, and what it checks

sshd's `ChrootDirectory` and vsftpd's chroot need root-owned folders on the
whole path. A folder of the site belongs to the site account, which could
also replace it with a link at any time. So the folder is never the chroot:

1. `sandbox-helper ftp-bind --login=<l> --site=<s> --folder=<path>` (the unit's
   values, root's own file) reads `/etc/passwd` directly and proves that the
   site account is a hosting account whose home is the home on the folder's
   path, that the login has the site's uid and primary group, its jail home
   and no shell, and that the site's group is private (no other account has
   it as primary group, no other member).
2. It opens the folder from the hosting root **one folder at a time** with
   `openat2(RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS)`,
   `RESOLVE_NO_XDEV` below the site home, checking every owner: root and not
   group/other-writable above the home, the site account from the home down.
3. It makes the jail folders (root-owned, modes above), removes any earlier
   mount at `files`, and binds the opened folder **by its descriptor**
   (`open_tree(OPEN_TREE_CLONE)`, not recursive, `nosuid,nodev`, then
   `move_mount`), and proves that `files` shows that very folder (device and
   inode).
4. Any failure removes the jail. `/run` is empty after a reboot, so a folder
   that fails its check at boot leaves no jail, and sshd and vsftpd refuse the
   login: it fails closed.

After the mount the folder is held by the mount: renaming it or replacing it
with a link changes nothing for the login until the next start, which checks
again.

## 3. Changes, in steps

Add: check (names, folder, record, owner-edited files, the folder walk) →
account (locked; a suspended site's expiry is copied) → jail (helper copy,
unit, `systemctl enable`, `restart`, mount proved) → sshd (drop-in, `sshd -t`,
`sshd -T -C user=<login>` must show the chroot and the forced command, reload)
→ FTP (vsftpd files, restart, TLS handshake; optional: SFTP works without it)
→ password (`chpasswd`, last) → verify (sshd's password setting, the mount, an
FTP over TLS sign-in as the login that must start in `/files`). A failed step
puts back what the earlier steps did.

Change: a new password and/or a new folder (open sessions of the login end
first; the jail is made again with the new folder; the record follows).

Remove: lock → end the login's sessions (its processes are found by their
root, the jail; the site's own processes are never touched) → stop and remove
the unit and jail → drop-in (sshd must then deny the name) → FTP files →
`userdel -f` of an account that is provably the login (its jail home and the
site's uid; no `-r`). Every step leaves the login closed, so a removal that
stops half way is safe and can be repeated.

Site deletion removes the site's logins (recorded or left unfinished) before
the site account. If that fails, the site account is kept and the deletion is
reported incomplete, so a login's uid is never freed while the login exists.

Suspension: a login always carries the site account's expiry. Suspending the
site gives its logins the same past expiry (PAM refuses them in sshd and
vsftpd) and ends their sessions; restoring gives them the site's expiry back.

Transfer: when the transferred site's account loses its password and keys,
its logins lose their passwords too (`*`) and their sessions end. The new
owner sets new passwords or removes them.

A full disk (D-044) makes the subscription's logins read-only with its
accounts.

## 4. Owner edits and independence

The drop-in, a login's unit and the vsftpd files carry CelikPanel's sealed
header. A file that no longer matches was edited by the owner: CelikPanel
changes no login while the drop-in is the owner's, and does not change or
remove a login whose unit is the owner's; the screen says who acts.

Everything is native. Without CelikPanel the logins keep working and are
managed with the system's own tools:

```sh
systemctl status celikpanel-ftp-<login>        # the jail
journalctl -u celikpanel-ftp-<login>           # why a jail is missing
passwd <login>                                 # a new password
# remove a login by hand:
systemctl disable --now celikpanel-ftp-<login>
rm /etc/systemd/system/celikpanel-ftp-<login>.service
# delete its Match block and its name in the drop-in's deny line, check with sshd -t, reload sshd
userdel -f <login>                             # never -r
```

## 5. Plan quantity

A subscription may have at most the plan's number of logins. The count is
checked on the server when a login is added, in the same transaction that
writes its row, so two requests cannot both take the last place. The
administrator's own workspace has no limit. The number is the key
`ftp.max_logins` of the limits chain (docs/SITE-LIMITS.md: server ⊇ reseller ⊇
plan or Custom): 10 per subscription on the server level (migration 072), and
every plan, reseller plan and Custom set can lower it in its limits (0: no
additional login). One server keeps at most 500 logins.

## 6. Escape matrix

Run as the login on Debian 13 and Ubuntu 24.04 lab guests with the real Agent
code (kit: the workstream's lab scripts; results in the release notes):
start folder and `..` (SFTP and FTP), `/etc/passwd`, another site, the own
site outside the folder, writes into the jail root, links made by the login,
hard links to files outside, `/proc`, `ssh id`, a tty, `ssh -W` forwarding,
legacy `scp`, plain FTP; the folder swapped for a link after the mount (the
login keeps the original folder), before a restart and before a reboot (no
jail, sign-in refused), a link in a parent folder, a mount inside the folder
(not visible), another site's account and the web server reaching the jail
(refused); uploads owned by the site and editable by PHP; read-only,
suspension, transfer, folder change, owner-edited files, removal (no account,
unit, jail, entry or ownerless file left) and site deletion.

## 7. Limits

- SFTP with a password needs the server to accept passwords; where sshd
  accepts keys only, logins use FTP over TLS (the screen says so). Logins have
  no keys.
- FTP needs CelikPanel's vsftpd configuration; an owner's vsftpd is kept and
  logins then use SFTP only.
- Arch Linux: sshd's PAM stack (system-login) runs pam_shells, and
  /etc/shells does not list the no-login shell, so the system itself refuses
  SFTP with a password for logins (and site accounts): closed, not open. The
  Agent detects it (read only) and the screen says logins connect with FTP
  over TLS. CelikPanel does not change the system's rule.
- Arch Linux: vsftpd 3.0.5 as Arch packages it kills every TLS data
  connection in its seccomp sandbox (listings and uploads failed for every
  account, site accounts included). CelikPanel's vsftpd.conf sets
  `seccomp_sandbox=NO` on Arch only; vsftpd's chroot and privilege
  separation stay. Debian and Ubuntu keep the sandbox.
- Folder names: letters, digits, dot, hyphen and underscore, at most 12
  levels below the site home. A folder must belong to the site account; a
  folder another account owns is refused.
- A login is not a separate uid (section 1).
