# Site sandbox

*alpha.119 · In source and component-tested. Smoke-run once on WSL Debian 13
(systemd 257). The escape matrix on the five supported systems is still
pending in the lab.*

Turkish: [SITE-SANDBOX.tr.md](SITE-SANDBOX.tr.md).

Every customer program runs in one systemd sandbox. That covers the AI and
automation service apps (`celiksvc-<id>`, and their install step
`celiksvcinst-<id>`), Python apps and uploaded programs (`celikrt-<id>`).
CelikPanel treats every program as arbitrary code. An uploaded Go binary can
start Python, so the security boundary is the sandbox. Which languages the
server has installed does not matter.

The code is in `internal/sitesandbox`. Service apps render their units
through it (`internal/serviceapps`). The runtime workstream uses it for
Python apps and uploaded programs. Uploaded programs are described in
[RUNTIME-APPS.md](RUNTIME-APPS.md).

## 1. What every sandboxed program gets

| Area | Settings |
|---|---|
| Account | `User=`/`Group=` is the site's own account, never root. `NoNewPrivileges`, an empty `CapabilityBoundingSet` and `AmbientCapabilities`, `RestrictSUIDSGID`. |
| Files | `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp` (/tmp and /var/tmp are the unit's own and gone after a stop), `PrivateDevices`, `UMask=0077`. The hosting tree is an empty read-only `TemporaryFileSystem` with only the program's folder bound back in, from the helper's root-owned mount point (§1a). That folder is the only writable path. |
| Hidden paths | `/run/celikpanel` (the Agent socket), `/run/php` and `/run/php-fpm` (every site's PHP-FPM socket), `/etc/celikpanel`, `/var/lib/celikpanel*`, `/var/log/celikpanel`, `/root`, `/opt/celikpanel/toolchains`, `/var/spool/postfix` (with it the `sendmail` drop). |
| Kernel | `ProtectKernelTunables/Modules/Logs`, `ProtectClock`, `ProtectHostname`, `ProtectControlGroups`, `RestrictNamespaces`, `RestrictRealtime`, `LockPersonality`, `RemoveIPC`, `PrivateIPC`. |
| System calls | `@system-service`, minus `@debug @mount @raw-io @reboot @swap @module @cpu-emulation @obsolete @keyring` and `process_vm_readv`/`process_vm_writev`. Native architecture only. |
| Processes | `ProtectProc=invisible`, so other users' processes are not visible. |
| Network | `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK`, so there are no raw or packet sockets. `IPAddressDeny` blocks link-local (169.254/16, fe80::/10) and the metadata addresses fd00:ec2::254 and 100.100.100.200. `SocketBindAllow=tcp:<own ports>` plus `SocketBindDeny=any` means the program can bind only its own loopback ports: no other port, no port 0, no UDP. |
| Mail | The SMTP egress block (§3). |
| Limits | `MemoryMax`, `MemorySwapMax=0`, `CPUQuota`, `TasksMax` and `LimitNOFILE` from the administrator's limits, plus `LimitCORE=0`. `LogRateLimitIntervalSec=30s`/`LogRateLimitBurst=1000`. `StartLimitIntervalSec=300`/`StartLimitBurst=5` in `[Unit]`. |

**Additions by kind:**

- **Python apps and uploaded programs** also get `ProcSubset=pid`.
- **Uploaded programs** also get `MemoryDenyWriteExecute`, `NoExecPaths=/` and
  `ExecPaths=<app folder> /usr/lib /usr/lib64 /lib /lib64`. These are defence
  in depth, not the boundary. They make `/bin/sh`, `python3` and
  `ld.so /usr/bin/python3` fail.
- **Service apps** keep alpha.118's full `/proc` view. Node.js reads
  `/proc/cpuinfo` and `/proc/meminfo`, and this has not been measured with
  n8n and Open WebUI yet.

**Additions on systemd 257 and later:** `PrivatePIDs=yes` (the program is
PID 1 of its own namespace) and `ProtectControlGroups=strict`.

## 1a. How the app folder gets into the sandbox

systemd resolves the paths of a unit as root, on the host, and follows
links: the source of `BindPaths=` and `EnvironmentFile=`. The site's account
owns its site home. It can rename its app folder and put a link in its
place, between CelikPanel's checks and a start, or before any later restart.
If the unit named the folder, systemd would bind whatever the link points
to: another site's folder or `/etc/celikpanel`. Unix permissions would still
apply, but the sandbox must not depend on them.

So no unit names a path inside the site home on the host. Each sandboxed
unit requires a **bind unit**, `celikpanel-sandbox@<unit name>.service`
(`Requires=` and `After=`), made from the template
`/etc/systemd/system/celikpanel-sandbox@.service`. The bind unit has no
sandbox at all and runs the **sandbox helper** as root in the host's mount
namespace (a separate unit is needed: systemd sets up `BindPaths=` and
`TemporaryFileSystem=` for every command of a unit, `ExecStartPre=+`
included). Before the sandboxed unit starts, the helper:

1. Reads the sandboxed unit's own `User=`, `BindPaths=` and
   `EnvironmentFile=` from systemd (`systemctl show`). Nothing else tells it
   which folder to bind.
2. Opens the app folder from `/var/www/celikpanel/subscriptions`, one folder
   at a time, with `openat2(RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS |
   RESOLVE_NO_MAGICLINKS)`. A link anywhere on the way is refused. Below the
   site home, a mount point is refused too (`RESOLVE_NO_XDEV`).
3. Checks the owners: root owns the subscription folder and its `sites`
   folder, and nobody else may write to them. The site's account owns the
   site home and every folder below it. The account's home must be that site
   home, and it must be a regular account (uid 1000 or above).
4. Binds the opened folder, by its descriptor and never by its name, onto
   `/run/celikpanel-sandbox/<unit>/app`, `nosuid` and `nodev`, not
   recursively (`open_tree`, `mount_setattr`, `move_mount`; on a kernel
   without them, `mount` of `/proc/self/fd/<n>`). It then checks that the
   mount point shows that very folder. `/run/celikpanel-sandbox` and each
   unit's folder are root's, mode 0700.
5. Reads a service app's environment file (`config/app.env`) the same way (a
   regular file of the site's account, no link, at most 256 KiB) and copies
   it to `/run/celikpanel-sandbox/<unit>/app.env` (root, 0600). The unit
   reads only that copy.

The sandboxed unit has `BindPaths=/run/celikpanel-sandbox/<unit>/app:<app folder>`:
the program sees its folder at the usual path, inside the otherwise empty
hosting tree. `WorkingDirectory=`, `ExecPaths=` and `ExecStart=` name that
path too, but systemd resolves them inside the sandbox, where it is the
bind. The mount holds the folder itself, so renaming the folder or swapping
it for a link later changes nothing for the running program.

The bind unit is `PartOf=` the sandboxed unit: every start, stop and restart
of the sandboxed unit (by the panel, the owner or the boot) starts, stops or
restarts it too, and every start checks again. It also stops by itself when
the sandboxed unit no longer runs (`StopWhenUnneeded=`), so a program that
has exited is checked again at its next start. An automatic restart after a
crash (`Restart=on-failure`) keeps the mount it already checked.

The helper fails closed. If a check or the mount fails, the bind unit fails
and the sandboxed unit does not start ("a dependency job failed"). The bind
unit's journal names the folder and the reason in English and Turkish
(`journalctl -u celikpanel-sandbox@<unit name>`). The panel shows
`app_folder_not_real` (a link, not a folder, a mount point: the customer puts
the real folder back) or `app_folder_not_owned` (the administrator corrects
the owner). The Agent runs the same walk, without the mount, before it
writes or starts a unit, and after a start fails. Stopping the bind unit
removes the mount and the copy.

**No root command receives the customer's environment through a dynamic
loader.** systemd gives every command of a unit the unit's environment, and a
service app's environment file is the customer's. The sandboxed unit's only
root commands are the helper's `egress-add`/`egress-remove` (§3): the helper
is a static Go program, ignores its environment, and starts nft with an
empty one. Before generation 3, the units ran nft directly as root with that
environment, and systemd read `config/app.env` from the site home as root.

**The helper** is the Agent binary started under the name `sandbox-helper`.
The Agent copies itself to `/usr/lib/celikpanel-sandbox/sandbox-helper`
(§4), so the units keep starting when CelikPanel is removed.

| System | Kernel | `openat2` (5.6) | `open_tree`/`move_mount` (5.2) | `mount_setattr` (5.12) | systemd |
|---|---|---|---|---|---|
| Ubuntu 22.04 | 5.15 (GA), 6.x (HWE) | yes | yes | yes | 249 |
| Debian 12 | 6.1 | yes | yes | yes | 252 |
| Ubuntu 24.04 | 6.8 | yes | yes | yes | 255 |
| Debian 13 | 6.12 | yes | yes | yes | 257 |
| Arch | current | yes | yes | yes | 258+ |

The systemd side (a template unit, `Requires=`/`After=`/`PartOf=`,
`BindPaths=source:destination`, `Type=oneshot` with `RemainAfterExit=`) is
older than every supported release. On a kernel without `openat2` the
helper refuses to start anything (`app_folder_unreadable`); it never falls
back to a walk by name. It was run once on WSL Debian 13 (systemd 257,
kernel 6.18): the bind, writes from the site's account, a swap to a link
while running (the program kept its own folder), the refused restart and the
clean stop. The five systems are still to be measured in the lab.

## 2. Per system, and the reduced level

Each directive is written only where the host's systemd has it. Where
systemd would accept a BPF directive but silently not enforce it, that
directive is left out.

| Directive | Needs | Ubuntu 22.04 (249) | Debian 12 (252) | Ubuntu 24.04 (255) | Debian 13 (257) | Arch (258+) |
|---|---|---|---|---|---|---|
| ProtectProc, ProcSubset | 247 | yes | yes | yes | yes | yes |
| PrivateIPC, ExecPaths/NoExecPaths | 248 | yes | yes | yes | yes | yes |
| SocketBindAllow/Deny | 249 + BPF | if BPF | if BPF | if BPF | if BPF | if BPF |
| IPAddressDeny | 235 + BPF | if BPF | if BPF | if BPF | if BPF | if BPF |
| PrivatePIDs, ProtectControlGroups=strict | 257 | — | — | — | yes | yes |
| SMTP egress block | nft `socket cgroupv2` + cgroup v2 | to be measured (nft 1.0.2) | yes (expected) | yes (expected) | yes (expected) | yes (expected) |

The Agent's read-only probe (`SandboxCapsV1`) collects four facts. It asks
`systemctl --version` for the systemd version and `+BPF_FRAMEWORK`. It checks
for cgroup v2. It runs `nft --check` on the egress table, which applies
nothing. It changes nothing on the server.

The level is **full** when all of these hold: systemd 249 or later, BPF,
cgroup v2, and nft can load the egress table. Otherwise the level is
**reduced**, and the screen names each gap:

- `systemd_old` or `systemd_unknown`: systemd is older than 249, or its
  version could not be read.
- `bpf_missing`: the port-binding rule and the metadata block are not
  enforced. The listen check still stops a program that opens a public
  address.
- `cgroup_v2_missing`: the cgroup v2 hierarchy is not mounted.
- `nft_cgroup_missing`: there is no mail block. On this host, programs can
  send mail through the local mail server.

A runtime is offered at the reduced level only when the administrator
accepts the named gap (`sitesandbox.Permit`). The service apps were offered
before levels existed. They run at the level the host enforces, and the
administrator's page shows it.

## 3. The SMTP egress block

Postfix relays mail from 127.0.0.1 without a password (`permit_mynetworks`).
Without a block, any program on the server could send mail as the server.

The block lives in its own nftables table, `inet celikpanel_egress`, in the
native file `/usr/lib/celikpanel-sandbox/egress.nft`. It is never in the
firewall's `inet celikpanel_fw`, because firewall policy V2 is input-only.

- The set `smtp_blocked` (`typeof socket cgroupv2 level 2`) holds the cgroups
  of sandboxed units.
- The output chain rejects TCP to ports 25, 465 and 587 on any address,
  127.0.0.1 and ::1 included, for members of the set. It counts the
  rejections in the named counter `smtp_rejects`.
- The input chain drops new connections from outside (not `lo`) to a socket
  of a set member. A program that opens 0.0.0.0 on its own port cannot be
  reached from outside.
- Loading the file again is atomic and harmless. The table, set, counter and
  chains are declared. Only the two chains are flushed and refilled, in the
  same transaction. The set's elements and the counter survive a reload.
  This was measured on nft 1.1.3 in a network namespace.

On a host that holds the block, every sandboxed unit gets these lines
(the helper, §1a):

```
ExecStartPre=+/usr/lib/celikpanel-sandbox/sandbox-helper egress-add --unit=<unit>
ExecStopPost=-+/usr/lib/celikpanel-sandbox/sandbox-helper egress-remove --unit=<unit>
```

The helper runs `nft -f /usr/lib/celikpanel-sandbox/egress.nft` and
`nft add element inet celikpanel_egress smtp_blocked '{ "system.slice/<unit>" }'`
(with an empty environment), and `nft delete element …` after the stop. This
fails closed. If nft cannot load the table, or the unit's cgroup
is not `/system.slice/<unit>` (for example, an owner's drop-in moved it to
another slice), the unit does not start.

`celikpanel-egress-guard.timer` runs every 30 seconds. If someone ran
`nft flush ruleset` or restarted `nftables.service`, the guard loads the table
again. It then adds every running sandboxed unit whose `ExecStartPre` runs
the helper's `egress-add`. Between a flush and the guard's next run, the block is absent for
up to 30 seconds. A unit started by an earlier release (with no
`ExecStartPre`) is never added silently.

**Not covered:** port 2525, mail sent through HTTP mail APIs, and a program's
own outbound traffic in general. A program's outbound traffic is also not
counted against the subscription's traffic.

## 4. Native files and owner independence (D-022, D-031)

The Agent writes four files through the D-031 render rule
(`services.ApplyManagedSiteFile`). Each file carries the `# celikpanel-render
v2 sha256=` header. The Agent records each write, and the helper's, in
`/var/lib/celikpanel-agent-private/sandbox/native-files.json`.

- `/usr/lib/celikpanel-sandbox/egress.nft`
- `/usr/lib/celikpanel-sandbox/egress-guard.sh`
- `/etc/systemd/system/celikpanel-egress-guard.service`
- `/etc/systemd/system/celikpanel-egress-guard.timer`

With the helper, the Agent also writes the bind units' template
`/etc/systemd/system/celikpanel-sandbox@.service` under the same rule, on
every host (it does not depend on the egress block).

The sixth file is the helper, `/usr/lib/celikpanel-sandbox/sandbox-helper`: a
copy of the Agent binary (0755, root's). The Agent writes it when it is
absent and replaces its own earlier copy after an update. A file the owner
put there instead is kept and used as it is (the digest differs from the
one recorded). The helper works without CelikPanel.

The Agent writes an absent file, replaces CelikPanel's own unchanged text,
and keeps a file the owner changed. It places its own text beside the kept
file as `<file>.celikpanel-pending`, and the administrator's page names the
kept file. If the owner's version no longer loads, sandboxed units refuse to
start. That is the fail-closed answer.

All of this keeps working without CelikPanel: the units, the table, the timer
and the guard. To remove the block, run:

```sh
systemctl disable --now celikpanel-egress-guard.timer
nft delete table inet celikpanel_egress
```

The Agent writes these files only during a mutation (installing or starting
an app), never during a status read.

## 5. Proof before publish

When an operation reaches `verify`, the Agent proves the running unit against
its profile (`sitesandbox.Prove`):

- `systemctl show` matches the profile: account, cgroup, every protection,
  memory, tasks, `Requires=`/`After=` of the bind unit, `BindPaths=` from
  its mount point, and the helper's egress lines.
- No bind source (`BindPaths=`, `BindReadOnlyPaths=`) and no environment
  file lies in the hosting tree on the host, for example from an owner's
  drop-in (`site_path_on_host`).
- The unit's cgroup is in `@smtp_blocked`, and the reject rule is loaded.
- The unit listens on loopback only.

Any mismatch, and anything that cannot be read, is a coded refusal:
`property_mismatch`, `site_path_on_host`, `cgroup_unexpected`, `not_in_smtp_set`,
`egress_rule_missing`, `listens_elsewhere`, `port_taken`, `not_listening` or
`unreadable`. The Panel then stops the app and does not publish it
(`sandbox_proof`).

## 6. Moving running apps to the new sandbox

A unit written by an earlier release (with no `# celikpanel-sandbox-generation`
marker, or a lower generation than 3) is never rewritten silently. Generation
3 is the bind unit with its mount and environment copy (§1a). The domain's page shows "runs in an
earlier release's protection". The administrator's page shows how many apps
need the new sandbox, and has one action:

`POST /api/v1/admin/service-apps/sandbox-refresh {request_id, expected}`

- The administrator sends `expected`, the number shown on the page. If the
  number changed since the page was read, the request is refused.
- The action runs one at a time across the Panel. A second press while it
  runs is refused.
- A repeated `request_id` (a lost answer) returns the first answer and
  starts nothing.
- Each running app gets a durable `update` operation to its own version,
  starting at `activate`. The unit is rewritten and restarted once, then
  proved and published.
- A stopped app has its unit rewritten and stays stopped.
- A unit the owner changed is kept and listed. An app at another release's
  version gets the new sandbox with its update.
- The action is audited as `service_app.sandbox_refresh`.

## 7. Escape tests

The kit is in `deploy/e2e/sandbox-escape/` (see its README). It runs a static
Go probe and a Python probe under rendered units through matrix items 1–18,
and reports PASS only when every attempt fails. The lab round on the five
supported systems is still open.
