# alpha.130 ops lane — what changed and how the lab proves it

Owner findings of 8 Nov 2026 (Boston, Ubuntu 24.04; Frankfurt, Debian 13), all
read-only. Component tests pass; nothing below has been tried on a real system
yet. These are the lab steps, in order.

## Resilience note (D-025)

- Invariants touched: owner independence (D-022) — the SSH drop-in is plain
  sshd configuration; without CelikPanel the owner deletes
  `/etc/ssh/sshd_config.d/00-celikpanel.conf` and runs `systemctl reload ssh`
  (Arch: `sshd`). Owner edits are never overwritten: a drop-in whose text is not
  exactly CelikPanel's is left alone and blocks both buttons.
- Schema: none (no migration). Contract: `SecurityAuditTLSResponse` gains
  optional `names` and `hostname` (panel-local; no Agent change). New Agent RPCs
  `SSHPasswordLoginStatusV1` (read) and `SetSSHPasswordLoginV1` (mutation step
  `ssh_password_login`/`openssh`).
- Recovery: every failed step of the SSH change takes CelikPanel's file back
  (remove, `sshd -t`, reload); a failed take-back says `rollback_failed` with the
  path to check. Open SSH sessions are never cut (reload, never restart).
- Open: the Components scan still judges Postfix on Debian/Ubuntu by its
  wrapper `postfix.service` (the catalogue does not list `postfix@-`); the
  Monitoring page and "Problems now" read the real instance.

## 1. "Problems now" and Postfix (Debian 13 and Ubuntu 24.04)

Root cause: the minute-old live service reading replaced the scan's
"active (running)" with systemd's raw words for the unit. On Debian and Ubuntu
`postfix.service` is a oneshot wrapper that always says "active (exited)"; the
problems rule counts only "running", so Postfix was listed as stopped (and an
e-mail alert could follow). The same applied to `wg-quick@wg0` (oneshot) and to
units systemd generates from init scripts. Now the live reading reads the
daemon itself (`postfix@-` on apt systems), applies the scan's own rule (any
active state is up, except PostgreSQL's apt wrapper), and each problem carries
the time of the reading that decided it. A service that only an older-than-15-
minutes component scan describes is reported as "not checked (only an old
reading)", never as a current problem.

Lab:
1. Install Postfix from Components. `systemctl status postfix postfix@-` shows
   the wrapper `active (exited)` and the instance `active (running)`.
2. Open Monitoring. Expect: no "Postfix is not running"; the services list shows
   Postfix running with memory/CPU (from `postfix@-`).
3. `sudo systemctl stop postfix@-` (the wrapper stays active). Within about a
   minute "Problems now" shows "Postfix is not running" with "last checked
   HH:MM". Press Start: expect the instance running again
   (`systemctl is-active postfix@-`). If Start does not bring it back, record
   it: the Start action targets `postfix.service` (its `Wants=` should start the
   instance).
4. WireGuard (if installed): `wg-quick@wg0` is not listed as stopped.
5. Arch: Postfix's own `postfix.service` decides; repeat steps 2–3 with
   `systemctl stop postfix`.

## 2. Panel TLS audit (both)

Root cause: the listener serves the issued certificate for its name and keeps
the start-up self-signed certificate for a bare IP address. The audit dialled
`127.0.0.1` without a name, received the start-up certificate and reported the
issued one as "Failed". The self-dial now asks for the certificate's own name.
New row "Panel address and certificate": pass when the certificate covers the
address in the browser, a warning when it does not, "not known" for a bare IP;
the names it covers are listed.

Lab:
1. Issue the panel certificate from Settings (Pebble in the lab).
2. Security → Server security: "Panel TLS certificate" is Pass; the names are
   listed; opened by name → address row Pass; opened by IP → address row says
   the start-up certificate is used on an address.

## 3. Security updates card (both)

The automatic line now says, e.g., "Automatic updates are on: every day the
system installs security updates from Debian, point releases of Debian." The
raw unattended-upgrades patterns are only behind "Exact settings, as apt
reports them". When an installed update waits for a reboot the card says so
with a link to Server → Maintenance. Lab: open Security; compare with
`apt-config dump | grep -A3 Origins-Pattern`.

## 4. Dates

Every date goes through `web/src/lib/dateFormat.ts` with the interface
language. Lab: switch to Turkish; Components "last scan", audit log and backups
show Turkish dates ("8 Kas 2026 05:20").

## 5. SSH password sign-in (Debian 13 and Ubuntu 24.04 VMs)

Prepare each VM like a cloud image:

```sh
echo 'PasswordAuthentication yes' | sudo tee /etc/ssh/sshd_config.d/50-cloud-init.conf
echo 'PasswordAuthentication no'  | sudo tee /etc/ssh/sshd_config.d/60-cloudimg-settings.conf
sudo systemctl reload ssh
sudo sshd -T | grep -E '^(passwordauthentication|kbdinteractiveauthentication|permitrootlogin)'
```

Expect `passwordauthentication yes` (first match wins).

1. Without a key: Security → "SSH password sign-in" says password sign-in is
   on, "Decided by /etc/ssh/sshd_config.d/50-cloud-init.conf, line 1", no
   administrator has a key, and the button is disabled with the reason next to
   it.
2. Add a key: `ssh-copy-id` to a sudo user (Debian: `debian`, Ubuntu:
   `ubuntu`). Check again: that account is listed with its key count; the button
   is enabled.
3. Keep the current SSH session open. In a new terminal prove the key works:
   `ssh -o PreferredAuthentications=publickey <user>@<vm>`.
4. Press "Turn off password sign-in", confirm. The progress window shows check →
   apply → verify. Expect: `/etc/ssh/sshd_config.d/00-celikpanel.conf` exists
   (0644 root), `sshd -T` says `passwordauthentication no` and
   `kbdinteractiveauthentication no`, the open session is still alive.
5. Proof: `ssh -o PreferredAuthentications=password -o PubkeyAuthentication=no <user>@<vm>`
   is refused (`Permission denied (publickey)`); the key login from step 3
   still works.
6. "Turn password sign-in back on": the drop-in is gone, `sshd -T` says yes
   again (50-cloud-init decides), no temporary files are left in
   `/etc/ssh/sshd_config.d/` (`ls -la`).
7. Refusal before writing: with passwords on, add `Bogus yes` to
   `/etc/ssh/sshd_config.d/50-cloud-init.conf`. "Turn off" is refused ("sshd
   does not accept its current configuration") and nothing is written. Remove
   the line. The rollbacks after writing (sshd -t refuses, sshd -T still says
   yes, the reload fails, and turning back on when sshd refuses the remaining
   configuration) are covered by the Agent tests
   (`cmd/agent/ssh_password_login_test.go`); do not break a lab VM's sshd to
   reproduce them.
8. Owner edit: edit `00-celikpanel.conf` by hand (add a comment). Both buttons
   refuse with "changed by someone other than CelikPanel"; the file is kept.
9. Order guard: move the `Include` line in `/etc/ssh/sshd_config` below a
   `PasswordAuthentication yes` line; "Turn off" is refused naming that line.
10. Ubuntu socket activation: if `ssh.service` is inactive (only `ssh.socket`),
    the result says the setting applies when the next connection starts sshd.
