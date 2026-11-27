# Site sandbox

*alpha.119 · In source and component-tested. Smoke-run once on WSL Debian 13
(systemd 257). The escape matrix on the five supported systems is still
pending in the lab.*

*S4 R2b (22 November 2026) · A site's scheduled tasks run as timers in its slice
(§9); the SMTP block lets its members reach this server's own submission service at the
server's own addresses, only while that service is CelikPanel's own (§3).*

*alpha.127 · Node.js sites joined the sandbox (§1, §6, §7). In source and
component-tested; the Node.js items of the escape kit (19, 20, with the
matrix under a Node.js site's unit) were smoke-run on WSL Debian 13 (systemd
257, Node.js 22.23.4). The lab round on the five systems is still open.*

Turkish: [SITE-SANDBOX.tr.md](SITE-SANDBOX.tr.md).

Every customer program runs in one systemd sandbox. That covers the AI and
automation service apps (`celiksvc-<id>`, and their install step
`celiksvcinst-<id>`), Python apps and uploaded programs (`celikrt-<id>`),
and since alpha.127 Node.js sites (`celikapp-<site id>`, and their install
step `celikappinst-<site id>`).
CelikPanel treats every program as arbitrary code. An uploaded Go binary can
start Python, so the security boundary is the sandbox. Which languages the
server has installed does not matter.

The code is in `internal/sitesandbox`. Service apps render their units
through it (`internal/serviceapps`). The runtime workstream uses it for
Python apps and uploaded programs. Node.js sites render theirs through
`internal/nodesite`. Uploaded programs are described in
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
- **Python apps get nothing more** than the base set and `ProcSubset=pid`:
  no `MemoryDenyWriteExecute` and no `NoExecPaths`/`ExecPaths`. An
  interpreter executes its own extension modules from its environment, and
  some packages generate code at run time, so these would break ordinary
  apps. A Python app can therefore start other programs of the server,
  `/bin/sh` included; what they reach is what this sandbox allows. Its
  environment build runs as the transient unit `celikrtbuild-<id>`: the
  same account and sandbox, no listening port, outbound open for PyPI, the
  SMTP block in place ([RUNTIME-APPS.md §18](RUNTIME-APPS.md)).
- **Uploaded programs** also get `MemoryDenyWriteExecute`, `NoExecPaths=/` and
  `ExecPaths=<app folder> /usr/lib /usr/lib64 /lib /lib64`. These are defence
  in depth, not the boundary. They make `/bin/sh`, `python3` and
  `ld.so /usr/bin/python3` fail.
- **Service apps** keep alpha.118's full `/proc` view. Node.js reads
  `/proc/cpuinfo` and `/proc/meminfo`, and this has not been measured with
  n8n yet.
- **Node.js sites** (alpha.127) get the base set with the full `/proc` view
  (as the service apps: `os.cpus()` would see no processor, and code that
  sizes a worker pool from it would start none; `ProtectProc=invisible`
  still hides every other account's processes). No `MemoryDenyWriteExecute`
  (V8 compiles JavaScript at run time) and no `NoExecPaths` (npm runs the
  project's scripts with `/bin/sh`). In addition:
  - **One Node.js version.** `/opt/celikpanel/runtimes` is an empty read-only
    `TemporaryFileSystem` with only the site's chosen version bound back in
    read-only (`BindReadOnlyPaths=/opt/celikpanel/runtimes/node/<version>`).
    A version the site's plan does not allow cannot be started by its full
    path; the source is root's, outside the hosting tree.
  - **Its own folder.** The app folder is the site's document root
    (`<site home>/public_html`), bound by the helper like any app folder
    (§1a). `HOME` is `<document root>/.home` (npm's cache is `HOME/.npm`).
    `PATH` is the version's `bin`, then `/usr/bin:/bin`.
  - **Its own port.** The port the site was given (1024–65535, as before:
    the Panel checks it against every other site's); `SocketBindAllow` lets
    the program bind only that one. `PORT`, `HOST=127.0.0.1` and
    `NODE_ENV=production` are set.
  - **The start command** stays the customer's shell command line, run as
    `ExecStart=/bin/sh -c "<command>"` as the site's account inside the
    sandbox. It is written so that the shell receives exactly what was typed
    (`%` as `%%`, `$` as `$$`, `\` and `"` escaped; a line break refused).
  - **Limits** (fixed in this release; a Node.js site had none before):
    `MemoryMax=1536M`, `CPUQuota=200%`, `TasksMax=512`; the install step gets
    `MemoryMax=2048M`.
  - **The install step** `celikappinst-<site id>` (the Hosting tab's "Install
    packages and build"): a transient unit, the same sandbox without a port,
    outbound open for the npm registry and the SMTP block in place. It runs
    `<version>/bin/node <version>/lib/node_modules/npm/bin/npm-cli.js ci`
    (`install` without `package-lock.json`) `--no-audit --no-fund
    --foreground-scripts`, then `npm run build --if-present`, as separate
    command lines. It runs only for a site whose unit is already in the
    sandbox; the running application is not touched.

**Additions on systemd 257 and later:** `PrivatePIDs=yes` (the program is
PID 1 of its own namespace) and `ProtectControlGroups=strict`.

**Transient units** (the Python build `celikrtbuild-<id>` and a service
app's install `celiksvcinst-<id>`, started with `systemd-run --property`)
get the same settings, but in the form that systemd-run parses
(`sitesandbox.TransientDirectives`). systemd 257's systemd-run reads
`ProtectControlGroups=` only as a boolean and refuses `strict`, while a unit
file accepts it. So a transient unit gets `ProtectControlGroupsEx=strict`,
which has the same effect. In alpha.119 lab round 2, every Python build
failed with "Failed to parse ProtectControlGroups: strict". Every property
name passed to systemd-run is listed in
`sitesandbox.TransientPropertyMinimums`, with the oldest systemd whose
systemd-run parses it, and tests check every list against systemd 249, 252,
255, 256, 257, 258 and 262. The rendered lists were run with systemd-run
on systemd 257 (Debian 13) and 262 (Arch).

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

The helper reads no environment file. A unit's values come only from root's
own file outside the hosting tree, which the Agent writes without following
links: `/etc/celikpanel-svcapps/celiksvc-<id>.env` for a service app (the
alpha.118 security fix, [AI-APPS.md](AI-APPS.md) §5) and
`/etc/celikpanel-runtime/` for a runtime app ([RUNTIME-APPS.md](RUNTIME-APPS.md)).
Nothing in the site's folder, and no copy of it, becomes a unit's environment.

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
removes the mount.

**No root command receives the customer's environment through a dynamic
loader.** systemd gives every command of a unit the unit's environment, and
that environment carries values the customer sets. The sandboxed unit's only
root commands are the helper's `egress-add`/`egress-remove` (§3): the helper
is a static Go program, ignores its environment, and starts nft with an
empty one. Before generation 3, the units ran nft directly as root with that
environment.

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

The helper accepts three kinds of app folder: a runtime app's
`<site home>/app`, a service app's `<site home>/apps/<app>` and, since
alpha.127, a Node.js site's document root `<site home>/public_html`.

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
| SMTP egress block | nft `socket cgroupv2` + cgroup v2 | probe accepted the table (nft 1.0.2, alpha.123 lab) | yes (expected) | yes (expected) | yes (expected) | yes (expected) |

Ubuntu 22.04 and Ubuntu 24.04 are treated the same (owner decision, 2026-11-05). Both measured
**reduced** in the alpha.123 lab with the single gap `bpf_missing` (systemd 249 and 255 are built
without BPF), so programs, Python apps and service apps get the same directives, the same proof and
the same offering on both; only the distribution Python differs (3.10 and 3.12). Debian 12 is also
reduced; Debian 13 is full. Nothing in the code is keyed to the distribution: the level comes from the
measured capabilities (`TestUbuntu2204SandboxIsTheSameAsUbuntu2404`).

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
administrator's page shows it. Node.js sites existed before levels too and
run the same way: at the level the host enforces, shown with its named gaps
under Components → Node.js. Ubuntu 22.04 and 24.04 give them the same unit
(`TestNodeUbuntu2204IsUbuntu2404`).

## 3. The SMTP egress block

Postfix relays mail from 127.0.0.1 without a password (`permit_mynetworks`).
Without a block, any program on the server could send mail as the server.

The block lives in its own nftables table, `inet celikpanel_egress`, in the
native file `/usr/lib/celikpanel-sandbox/egress.nft`. It is never in the
firewall's `inet celikpanel_fw`, because firewall policy V2 is input-only.

- The set `smtp_blocked` (`typeof socket cgroupv2 level 2`) holds the cgroups
  of sandboxed units.
- The output chain rejects TCP to ports 25, 465 and 587 on any address,
  127.0.0.1 and ::1 included, for members of the set, with the one exception
  below. It counts the rejections in the named counter `smtp_rejects`.
- **Submission to this server, only where it is safe (S4 R2b, 22 November
  2026, narrowed in its fix round; it applies to every unit in the set, a
  site's programs and its scheduled tasks alike).** One rule before the
  reject accepts TCP to ports 465 and 587 that leaves through the loopback
  interface (`oifname "lo"`): traffic to any of this server's own addresses
  does, 127.0.0.1, ::1 and its public addresses alike, so a task that
  connects to the server's host name reaches the name its certificate
  carries. The Agent writes this rule **only while** both submission
  entries in `master.cf` are CelikPanel's own, unchanged ones
  (`submission/inet` and `smtps/inet` in the form
  `cmd/agent/mail_submission_rpc.go` writes them) that take nothing but a
  mailbox's login (`smtpd_sasl_auth_enable=yes`,
  `smtpd_client_restrictions=permit_sasl_authenticated,reject`) and let a
  logged-in mailbox send only as its own addresses
  (`reject_sender_login_mismatch` before any `permit`, and a
  `smtpd_sender_login_maps`; second fix round: a setting that is missing or
  cannot be read means no exception), no other
  `master.cf` entry listens on 465 or 587, and every socket listening there
  belongs to Postfix's master process (`cmd/agent/site_tasks_mail.go`). With
  an owner's own entry or another program on those ports (an msmtpd, an
  stunnel to a smarthost), or no submission service at all, the file has no
  exception, and the site's Overview card says that tasks and programs of
  sites cannot send mail through this server and why. The decision is made
  again at each Panel start and after the mail submission step, and a
  changed rule set is loaded into the running table at once. **It stays as
  last decided until the Panel's next start, and for good without the
  panel**: an owner who later puts another program on 465 or 587 removes the
  rule by hand (the file's own header says how). When the server owner has
  changed the block's file, it is kept, and the card says that the kept file
  decides (not what CelikPanel would have written). Port 25 stays
  refused everywhere: Postfix's relay list has not allowed this server's own
  addresses without a password since alpha.125, and the block keeps it
  closed anyway.
- **Known limit: password guessing over this path (a precondition of the
  reach slice, R4).** CelikPanel adds no limiter of its own yet. Today a
  site's PHP, which still runs outside the block, can already guess the same
  way, so the step is small; once R3 puts PHP inside the block this path is
  the only gate, and R4 must limit it before it opens anything further. What exists today:
  Postfix's `smtpd_client_auth_rate_limit` is 0 (no limit) by default and
  its event limits exempt `$mynetworks` (loopback) by default
  (`smtpd_client_event_limit_exceptions`); a session is dropped after 20
  errors (`smtpd_hard_error_limit`) with a one-second pause after each error
  past the tenth, and Dovecot answers each failed login only after
  `auth_failure_delay` (2 seconds by default; its documentation names no
  exemption for local clients). A site's code can open many sessions at
  once, so this slows guessing but does not stop it.
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
`nft add element inet celikpanel_egress smtp_blocked '{ "<slice>/<unit>" }'`
for the cgroup the unit runs in (with an empty environment), and
`nft delete element …` after the stop. It finds that cgroup in this order:
its own cgroup in `/proc/self/cgroup` (it runs inside the unit), when that is
exactly `/system.slice/<unit>` or `/celiksite_<id>.slice/<unit>`; otherwise
systemd's `ControlGroup` of the unit; and when systemd cannot be asked or
names no place, the system slice, as before. When its own cgroup line is not
used, one journal line of the unit says why, which step decided and the place
taken. The SMTP helper so adds no dependency on systemd answering; the bind
unit's own read of the unit's settings, unchanged from earlier releases, still
stops a start while systemd does not answer. This fails closed. The unit does not start when nft
cannot load the table, when systemd names another place (for example, an
owner's drop-in moved the unit to another slice), or when the unit's place
cannot be established: nft refuses a cgroup path that does not exist, so the
system slice's element is not taken for a unit that runs elsewhere. The
journal then says what was being established, that the server's
administrator acts, and the command that shows the place:
`systemctl show -p ControlGroup,Slice <unit>`.

The site's slice is where this release places a site's programs (§8: a
program written while it does not run goes into it; a running one stays where
it runs until the site is moved, by the panel itself since alpha.150, §8.4). When a program's unit is
written while the site's limits cannot be applied (this host cannot hold
them, §8, or the request's limits are out of range), a unit file that
already names its site's slice keeps its `Slice=celiksite_<id>.slice` line,
as the release before this one does, only when all of these hold: it is the
last `Slice=` line of the main unit file (drop-ins are not read), it names the
slice of the unit's own site, and `/etc/systemd/system/celiksite_<id>.slice`
is a regular file (not a link or a folder). Otherwise such a write gives the
unit `Slice=system.slice`, as before.

`celikpanel-egress-guard.timer` runs every 30 seconds. If someone ran
`nft flush ruleset` or restarted `nftables.service`, the guard loads the table
again. It then adds every running sandboxed unit whose `ExecStartPre` runs
the helper's `egress-add` (the programs' copy or the tasks' own copy,
§9.4), and every task cgroup in a site's slice that still holds a process a
finished task left running, by its path alone (`celikcron-<id>-<n>.service`
under `celiksite_<id>.slice`; also after the task's unit file was removed,
when systemd no longer shows its `ExecStartPre`, §9.2). Between a flush and the guard's next run, the block is absent for
up to 30 seconds. A unit started by an earlier release (with no
`ExecStartPre`) is never added silently. Since alpha.127 the guard also
covers `celikapp-*` and `celikappinst-*`; a Node.js site's unit an earlier
release wrote has no helper line and stays out.

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

**Node.js sites** are proved after every start the Agent makes (a new site,
a settings change, the move of §6), once the unit runs and listens on its
port (up to 20 seconds). Node.js listens on every address when a program
calls `listen(port)` without a host. Where the host holds the egress block,
its input chain drops every new connection from outside to a sandboxed
socket, so a wildcard listener on the site's own port counts as loopback
(`sitesandbox.ProveNodeListening`); a wildcard on another port, or a named
public address, is `listens_elsewhere`. Without the block a wildcard is
exposed, as for every other kind. A new site without code yet (not running,
not listening) is reported, not refused. A unit already in the sandbox that
fails its proof is stopped and not published.

Any mismatch, and anything that cannot be read, is a coded refusal:
`property_mismatch`, `site_path_on_host`, `cgroup_unexpected`, `not_in_smtp_set`,
`egress_rule_missing`, `listens_elsewhere`, `port_taken`, `not_listening` or
`unreadable`. The Panel then stops the app and does not publish it
(`sandbox_proof`).

## 6. Moving running apps to the new sandbox

A unit written by an earlier release (with no `# celikpanel-sandbox-generation`
marker, or a lower generation than 3) is never rewritten silently. Generation
3 is the bind unit with its root-owned mount of the app folder (§1a); the environment file stays root's `/etc/celikpanel-svcapps/celiksvc-<id>.env`. The domain's page shows "runs in an
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

### 6a. Node.js sites (alpha.127)

A Node.js site's unit keeps its name, `celikapp-<site id>`. Which text the
Agent writes is decided from the request and the file on the server:

- a new site (no unit file yet) and a unit already in the sandbox get the
  sandbox;
- a unit an earlier release wrote (no `# celikpanel-sandbox-generation`
  marker) keeps that release's text on every ordinary write (a settings
  change, a Node.js version move, the site configuration page). It is never
  rewritten silently. The site's Hosting tab says "runs in an earlier
  release's protection" and who moves it;
- an application that still runs as the shared web account stays outside:
  the sandbox needs the site's own account, so "Run under the site's own
  user" (D-038 §6) comes first.

Components → Node.js shows the host's level and gaps, how many sites run in
the sandbox, and one action:

`POST /api/v1/runtimes/node/sandbox-refresh {request_id, expected}`

- `expected` is the number the page showed; a changed number is refused.
- One move at a time across the Panel; a second press while it runs is
  refused, a repeated `request_id` returns the first answer.
- The sites move one after the other (`ApplyAppUnit` with `sandbox_move`).
  A running application restarts once in the sandbox and is proved (§5). If
  the proof refutes the move, or an application that answered on its port
  before no longer does, the Agent writes the earlier unit again and starts
  the application on it (put back; resilience contract invariant 4). A
  stopped application gets the new unit and stays stopped (`no_start`).
- Owner-changed units are kept and listed, never moved. Sites on the shared
  web account are listed with their next action.
- Each written file is recorded in the site-file ledger; the action is
  audited as `site.node_sandbox_refresh`.

The install step (§1) is offered only once the site is in the sandbox:
`POST /api/v1/domains/{id}/app/packages {request_id, build}` starts it (a
repeated `request_id` answers the first start; a second step while one runs
is refused), `GET` reads it and never starts anything. It is audited as
`site.node_install`.

## 7. Escape tests

The kit is in `deploy/e2e/sandbox-escape/` (see its README). It runs a static
Go probe and a Python probe under rendered units through matrix items 1–18,
and reports PASS only when every attempt fails. The lab round on the five
supported systems is still open.

**Node.js sites (alpha.127).** With `NODE_VERSION=<version>` (a Node.js
under `/opt/celikpanel/runtimes/node`) the kit also renders a Node.js site's
unit with the Agent's own renderer (`render -what node-unit`) and runs the Go
probe as the site's code through its start command, so items 1–16 are
judged under the Node.js profile (item 14 is SKIP: npm needs `/bin/sh`).
Two items are added:

| # | Attempt | How it is judged |
|---|---|---|
| 19 | Another Node.js version; writing the runtime; the toolchains | A decoy version beside the site's must not be listed; the site's own `bin/node` must be reachable; a write into the runtime and a listing of `/opt/celikpanel/toolchains` must fail |
| 20 | The install step | `systemd-run` with the rendered install arguments (`render -what node-install`) must be accepted; npm installs a project whose `postinstall` runs the probe once and whose `build` runs node; no attempt may escape, the step must end `success` and the build must have written its file |

`sudo NODE_VERSION=<v> ./run.sh node` runs only these; `matrix` and `quick`
include them. On WSL Debian 13 (systemd 257, kernel without nft
`socket cgroupv2`, so level reduced with `nft_cgroup_missing`) items 1–8,
10, 12–16, 19 and 20 passed; item 9 failed as the named gap says (no SMTP
block on that host). Still open for the lab: the five systems, item 10's
outside reach of a wildcard Node.js listener with the egress table loaded,
and a real Node.js application (Express, Next.js) moved by the
administrator's action on each system.

## 8. The site's slice: one set of limits for all of a site's programs (S4 R2a)

*21 November 2026, second fix round 22 November 2026, third 23 November 2026 · In source and component-tested; the
facts below were measured on throwaway Debian 13 and Ubuntu 24.04 machines; the panel's lab
round on both is open (steps in the pull request).*

Every program CelikPanel starts for one site as a systemd unit runs in that site's
slice, `celiksite_<site id>.slice`, whose `MemoryMax=`, `MemorySwapMax=0`,
`TasksMax=` and `CPUQuota=` bound them **together**:

| Unit | What | In the slice |
|---|---|---|
| `celikapp-<site id>` | the Node.js application | yes |
| `celikrt-<app id>` | an uploaded program or a Python app | yes |
| `celiksvc-<install id>` | a service app (n8n) | yes |
| `celikappinst-`, `celikgit-<site id>` | the Node.js install step, the Git deploy | yes (a new unit each time) |
| `celikrtbuild-`, `celiksvcinst-<id>` | the Python build, a service app's install | yes (a new unit each time) |
| `celikcron-<site id>-<n>` | a scheduled task of a moved site (§9) | yes (S4 R2b) |
| the site's PHP-FPM pool | | not yet: PHP joins the slice in R3 |
| `celikpanel-sandbox@…`, `celikpanel-ftp@…` | root's bind units | no: they run no customer code |

**What is not bounded yet.** Say it plainly: a site's programs that run outside its
slice (every program written before R2a, until the panel has moved the site by itself
after the update, §8.4, and such a program again after each restart of the server until
it is moved), the site's
PHP (its PHP-FPM pool; R3), the crontab of a site not yet moved (§9), and a user manager
a site account already kept running before R2b ("lingering", §9.5) are not bounded by
the site's limits.

**The name.** A dash in a slice's name is nesting for systemd (`celiksite-7.slice`
would be a child of `celiksite.slice`). Without one the slice sits below the root,
and a unit in it is `/celiksite_7.slice/<unit>`: depth two, like
`/system.slice/<unit>`. The SMTP block's set (`typeof socket cgroupv2 level 2`, §3)
therefore matches a unit in either place with no change to the table. The helper's
`egress-add` adds the element of the cgroup the unit really runs in, found in the
order of §3 (its own cgroup line, then systemd's `ControlGroup=`, then the system
slice, which nft refuses for a unit that runs elsewhere), accepts only the system
slice or a site's slice, and refuses any other place, so a unit an owner's drop-in
moved elsewhere does not start (as before). The Agent reads where a program runs
by the same rule (`sitesandbox.UnitControlGroupAllowed`). The guard timer re-adds
units in either place.

**Limits.** The slice gets the site's effective `site.memory_mb`, `site.tasks` and
`site.cpu_percent` (docs/SITE-LIMITS.md §4a). systemd applies the most stringent
limit of a unit and its parent slices ("EffectiveMemoryMax= … is the most
stringent limit of the unit and parent slices", systemd.resource-control(5),
systemd 257 and 255). A unit keeps its own limits in its file (a Node.js
application 1536 MB, n8n 1536 MB); the slice is the outer bound. Without any level,
the slice gets 512 tasks, 200 % and, for memory, **the smaller of 3072 MB and 75 %
of the host's memory** (`MemTotal` in `/proc/meminfo`, rounded down to a whole MB,
never under 64 MB), so one site never takes the whole host by default. That memory default
is **fixed when it is first computed** (the panel's first start with this release; stored in
`panel_settings` `site_memory_default_v1` with the host memory it was computed for). A later
change of the host's memory moves nothing by itself: the administrator reads one Problems now
line ("the default was set for the earlier memory", with both sizes) whose one action previews
the sites a lower default would stop, as for any lowered limit, and applies it only with that
list confirmed.

**A reseller cannot starve the server by default.** Where the administrator set no ceiling for
a `site.*` key (neither the server's limits nor the reseller's own plan or Custom set), the
reseller's plans, its customers' Custom sets and every site below it are bounded by the
server's default for that key (the fixed, host-bound one for memory). The reseller's form shows
the bound as "set by the server administrator"; the administrator raises it by giving the
reseller (its reseller plan or Custom set) a ceiling. The administrator's own plans are not
bounded by the default.

**A host that cannot enforce limits.** Without the unified cgroup hierarchy
(`/sys/fs/cgroup/cgroup.controllers`) or without its `memory`, `pids` and `cpu`
controllers, nothing is written that would pretend to limit: no slice file, no
unit names a slice, the move is refused, every site's card says the limits do not
apply, and Problems now has one administrator line naming what is missing.

### 8.1 Facts measured on real systems (21 November 2026)

Plain `systemctl` and cgroup files on two throwaway machines started from the base
images: Debian 13 (kernel 6.12.105, systemd 257.13) and Ubuntu 24.04.4 (kernel
6.8.0-138, systemd 255.4); both use the unified hierarchy with `cpu memory pids`.

| | Question | Debian 13 / systemd 257 | Ubuntu 24.04 / systemd 255 |
|---|---|---|---|
| F1 | A running unit's file changes from `Slice=system.slice` to `Slice=celiksite_1.slice`; `daemon-reload`, then `systemctl restart` | after the reload `Slice=` shows the new slice but the process stays in `/system.slice/u.service`; after the restart it runs in `/celiksite_1.slice/u.service` | same |
| F1 | A drop-in sets `Slice=` | the drop-in outranks the file in both directions (`systemctl show -p Slice` is the effective place) | same |
| F2 | Slice `TasksMax=30` reached (one unit, or two units sharing it) | the slice's `pids.events.local` `max 1`; the unit's `max 0` | no `pids.events.local`; the slice's `pids.events` stays `max 0`; the forking unit's `pids.events` `max 1`, its `pids.peak` (31, 11) below its own `pids.max` |
| F2 | The unit's own `TasksMax=10` reached inside the slice | the slice's `pids.events` `max 1` but its `pids.events.local` `max 0`; the unit's local `max 1` | the unit's `pids.events` `max 1` with `pids.peak` 10 = its `pids.max` 10; the slice's `pids.peak` 10 of 30 |
| F3 | Slice `MemoryMax=128M` exceeded | the slice's `memory.events.local` `max 25 oom 1 oom_kill 0`; its `memory.events` `oom_kill 1` | the same (`max 35 oom 1`) |
| F3 | The unit's own `MemoryMax=96M` inside a 512M slice | the slice's `memory.events` `max 36 oom 1 oom_kill 1`; its `memory.events.local` all 0 | the same (`max 42`) |
| F3 | Host-wide shortage (slice limit 8G on a 2G machine, `global_oom` in the kernel log) | the slice's `memory.events` `oom 0 oom_kill 1`; `memory.events.local` all 0 | the same |
| F4 | The slice file changes (512M → 300M) while a unit runs in it; `daemon-reload` | **the kernel keeps 512M** (`memory.max` 536870912 six seconds later, `daemon-reexec` too); the new value reaches the cgroup when a unit of the slice starts, or with `systemctl set-property --runtime` | the reload applies it at once |
| F4 | The owner deletes the slice file; `daemon-reload` | the old limits stay until a unit of the slice restarts, then none | the limits go at once (`memory.max` `max`) |
| F4 | Lowering 512M → 100M under 200M of use | nothing until the next start | the program is stopped at once (`oom` 10576) |

So: the site's own memory limit is proved by the slice's `memory.events.local`
`oom`; its own process limit by the slice's `pids.events.local` `max` where it
exists (Debian's 6.12, and 6.13 and later), else (6.8) by a unit's `pids.events`
`max` while that unit's `pids.peak` stayed below its own `pids.max` (the slice is
the only other level with a limit). `memory.peak` does not tell the cases apart.

### 8.2 The file and how a change reaches running programs

`/etc/systemd/system/celiksite_<id>.slice`, written under the D-031 rule with
CelikPanel's header: an absent file is written, CelikPanel's unchanged text
replaced, a file the owner changed is kept with CelikPanel's text beside it as
`<file>.celikpanel-pending` (its own values apply). The owner's drop-ins go in
`celiksite_<id>.slice.d/*.conf`; CelikPanel never writes or removes them.

A changed limit is written into the slices (`Agent.ApplySiteSlicesV1`, at most 100
sites per call; a large change goes in batches, so a customer's change of a program
waits for one batch at most, and the administrator's site card shows "New limits
are being written into the sites: n of N" while it runs). Because systemd 257 does
not apply a changed slice file to a running slice (F4), the Agent then compares
what the kernel enforces (`memory.max`, `memory.swap.max`, `pids.max`, `cpu.max`)
with the **effective** value systemd reports for the slice (`systemctl show`: the
file and every drop-in, so an owner's drop-in keeps its number) and, only for a key
that differs, runs `systemctl set-property --runtime <slice> <key>=<that value>`.
systemd writes it as a control drop-in (`/run/systemd/system.control/<slice>.d/
50-<key>.conf`), which outranks `/etc`; the Agent removes exactly the files that
call made and reloads, so the slice file and the owner's drop-ins stay the only
settings. A key whose control drop-in already exists (the owner's own runtime
setting) is never touched. On systemd 255 the reload already applied the values and
nothing is set. Nothing is restarted and no process is started in a customer's
slice. The files about to be made are recorded first in the Agent's private folder and
removed in the same step on every way out of it. Only an Agent stop between the set and
the removal leaves them: they carry the effective values the slice had at that moment, so
nothing changes for the owner then, but **until they are removed they outrank the owner's
own drop-ins** (`/etc/systemd/system/<slice>.d/*.conf`) for those keys: a drop-in the owner
adds or changes for such a key meanwhile applies only after the Agent's next start (it
removes them first, right after taking its instance lock, before anything reads limits) or
the next reboot (`/run` is emptied). An owner's own `systemctl set-property --runtime` of
the same key made in that window is removed with them.
**A lowered limit therefore applies at once on both systems**, which is why it asks
first (§8.5).

**A change that does not reach every site.** One change of limits is written at a time (a
second change waits, then reads the limits as they are then, so two changes never
interleave); each batch of 100 sites has its own deadline. A batch the Agent does not
answer (and the batches after it), a batch it refuses or cannot make systemd take (a failed
`daemon-reload`, a ledger it cannot read), and a site whose file could not be written keep
their earlier limits, and that is recorded (`panel_settings` `site_limit_apply_v1`): the
administrator reads ONE Problems now line naming the change, how many sites still have the
earlier limits and why; every hour the panel writes those sites again with the limits as
they are then, and the line goes when every site has them. A site's card shows the limit in
force (what systemd applies) beside the plan's value whenever they differ.

**"In force" is what the kernel says** (third fix round). The status read also reads the
running slice's `memory.max`, `pids.max` and `cpu.max` and compares them with what systemd
applies (memory allowing the kernel's rounding to whole pages). The card says the limits
apply only when they are equal and systemd has read the file that is on disk
(`FragmentPath` set, `NeedDaemonReload=no`). Otherwise the administrator reads "written, not
in force yet", with the kernel's values as the ones in force: on systemd 257 a changed file
reaches a running slice only by CelikPanel's own push or a program's start, or, for an
owner's edit, after a `daemon-reload` and some time. After `sudo systemctl daemon-reload`,
an owner's change reaches running programs at once on Ubuntu 24.04 and about a minute and a
half later on Debian 13, without any program starting; a program's start applies it at once
(measured, facts A7/N4). A cgroup
file that cannot be read is "not known", never "in force"; a slice that does not run takes
the file when a program starts. The push itself reports per site why a running slice does
not hold its limits (systemd could not be read, the push's record could not be written,
`set-property` failed, or the kernel's files still differ after it): such a site is a failed
outcome of the change, never "written", so it is in the record above, its Problems now line
and the hourly round, which pushes again even where the file did not change. A push after a
program's own write or inside the move is only logged: each program started in the slice
makes systemd apply the file, and the card reads the kernel.

**A missing file.** A slice without a file has no limits (F4). The card tells three states
apart. Every existing site right after the update has no slice file because its programs
were never moved: no unit names the slice, so the card says the programs run outside the
limits, and the panel moves them by itself (§8.4). Where a unit names the slice or runs in it
and the file is gone (the owner removed it), the card says the file is missing and the next
change of limits writes it again; the site counts as outside, and the panel's worker moves it
(the move writes the file) once per installed version. A file CelikPanel wrote that the owner deleted to give it back is
written again by the next change even where no unit names it.

**The card for the administrator** names the owner's drop-ins by file name with "the
server owner's additions apply", and an edited file as kept.

### 8.3 Where a unit goes when it is written

A changed `Slice=` reaches a running unit only at its next start (F1). So:

- a unit that does not run (a new one, a stopped one, every install or build step)
  is written into its site's slice. Since alpha.149 (the alpha.148 lab's O2) this includes
  a **Start from the panel** of a program that does not run (an uploaded program, a Node.js
  site, an AI or automation app, and a Restart of one that does not run): its unit is
  written into the slice first (the site's slice file is put in place, CelikPanel's own
  unchanged text gets `Slice=celiksite_<id>.slice`, systemd reloads), then it starts
  there. Not a unit an owner changed or placed with a drop-in, not one of an earlier
  protection, not one a move's kept record names, and nothing when systemd cannot say
  whether it runs. Before that write the Start puts this release's sandbox helper and SMTP
  block in place, as the move does (fix round F2: before alpha.147 the helper added only
  system-slice cgroups to the SMTP block, so a unit in its site's slice was refused at its
  `ExecStartPre`); when that cannot be done nothing is written and the program starts where
  it started before, outside the limits;
- a running unit keeps the slice it runs in on every ordinary write (a settings
  change, an update, a restart by the customer);
- a unit whose file already names its site's slice, while that slice's file exists, keeps
  that line on every write, also when systemd cannot say where it runs (it does not answer,
  or shows an empty or unknown place; alpha.148 fix round): a reading that cannot be made
  never moves a program out of its limits, nor into them;
- the administrator's sandbox move of a Node.js site (§6a) restarts it anyway and
  puts it into the slice, with its proof and put-back.

**After a server restart.** systemd starts every unit at boot from its own file, in
the slice its `Slice=` line names. The file of every program written before this
release, and of every program written while it ran, says `Slice=system.slice`. **So
a site's existing program starts outside its site's limits again after a restart,
as before, and stays outside until it is moved** (by the panel itself after the update,
§8.4). A program whose file was written while it did not run, or that was moved, names
its site's slice and starts inside it.

**A new site, and a restored one (alpha.150, verified for D-048).** Every program of a site
created on this release is written before it first runs (the Node.js site, an uploaded
program, a Python app, a service app such as n8n, every install, build and Git deploy step),
so it is written into the slice by the rule above; its scheduled tasks are timers from the
start (§9). A backup's restore writes no unit and no crontab (the Agent's restore puts back
files and databases), and the program it stopped for the restore starts again through the
panel's Start, which writes a program that does not run into the slice first. What can still
leave a new or restored site outside without an owner's own change: a unit written while
systemd could not say whether it runs, a Start whose helper and SMTP block could not be put
in place (both above), and a new site whose account cron's files would still let in because
CelikPanel's `cron.allow` could not be written (§9.3). The panel's worker takes the third
case one minute after the site is created; the first two wait for its next run (the next
start of the panel, or the next site created or restored).

### 8.4 The move into the limits, made by the panel itself (alpha.150, D-048)

Until alpha.149 an existing site went inside its limits only when the administrator pressed
"Move into the limits" (or "Move all"); on a server whose administrator never pressed it, no
existing site was inside its limits. The owner's words of 27 November 2026: "zaten otomatik
olarak sınırlarda olmalıydı. buton falan saçmalığına gerek yok. bu bir hataydı. bu hatayı hiç
olmamış gibi düzelt ve bir daha olmasın." Since alpha.150 **the panel moves every site still
outside its limits by itself**; the move below (its record, put-back, proofs, time rule, locks
and the update's gate) is unchanged, only who starts it and what the screens offer changed.

**The worker** (`cmd/panel/site_auto_move.go`). ONE worker of the panel:

- **When.** Five minutes after the panel's start (after the side doors step), and one minute
  after a site is created or restored. Never because a page was read (D-024): reading the card
  or Problems now starts no move. A site created or restored while it runs adds one more pass,
  never a second worker.
- **Which sites, in which order.** It first reads every site's slice and scheduled tasks (the
  same reading as the background one behind Problems now) and takes the sites whose programs
  run, or would start, outside their limits (the ones the move takes: not a unit the server
  owner changed or placed with a drop-in, not one of an earlier protection) or whose tasks
  still run in cron, in the order of the sites' ids, **one site at a time**, with 20 seconds
  between two sites. A host that cannot hold a site's limits has nothing to move into.
- **How.** Each site through the same press the administrator's action used: the scheduled
  tasks first (§9.3), then the programs, with one fresh request id per site
  (`auto-<random>`), audited as `site.limits_move` with `automatic: true`.
- **The gate.** Before each site it waits while the panel's own update is admitted or running
  (the panel's own word), while the server is busy with another change (the Agent's own readiness:
  another change, package manager work), and while a move the
  administrator pressed runs. The Agent refuses a move while an update record is queued or
  running and while another move holds its claim, also across a restart of the panel (the
  claim is the Agent's, never a variable of the panel). A refusal for such a reason is not a
  failure: the site goes to the end of the run and is tried again after a minute, doubling up
  to ten; after six hours of such refusals it is marked failed with that reason.
- **Once per site per installed version, never a loop.** Before the move is sent the site is
  marked "started" in `panel_settings` `site_auto_move_v1` (with the panel's version and build;
  marks of another version count as none, so each version tries a site still outside once). No
  schema step: the panel already keeps its other site-limit records in `panel_settings`. A mark
  that cannot be written means nothing is sent; a record that cannot be read means no site is
  moved by itself. After the answer the mark says what came of it: moved; left (the programs
  went in, the tasks stayed in cron for the server owner's reason: a line the timers do not
  reproduce or one not made in CelikPanel that the administrator has not kept, the owner's own
  `cron.allow`, no cron); failed (refused or put back), stopped for its time, kept, or not known.
  **None of these is tried again by the worker.** A site whose move failed, stopped for its time,
  ended unknown or left a record that was settled since, and that is still outside its limits,
  is ONE Problems now line per site (`site_auto_move_failed`, administrator): why, and **Try
  again**, which is the same move (`POST /api/v1/domains/{id}/site-limits/move`). A site left for
  the server owner's reason is no to-do: its card names the reason with who acts and offers Try
  again for once it is changed, and the owner's own lines (`cron.allow`, a crontab line not made
  here) stay as they were. The administrator's own Try again is recorded the same way, so the
  line and the card follow the latest attempt.
- **A panel stopped in the middle.** A mark "started" that a new panel process finds (the panel
  stopped while a move ran) is settled by reading only: a site inside its limits is "moved",
  one still outside is "not known" (its line offers Try again). The move is never sent twice;
  if the Agent still runs the first one, its claim refuses the worker's next move meanwhile.
- **A kept record stops the run**, as it stops a press: the worker does not move the next site
  while a move record is kept; the existing line and card say what the administrator does
  (Try again on that site's card), and the worker reads every five minutes (a reading only)
  whether it was settled, then goes on by itself.
- **A site put back on purpose.** When the administrator puts a site's scheduled tasks back
  into cron ("Put the tasks back into cron", §9.3), the site is marked
  (`site_tasks_put_back_v1`) and the worker leaves it alone, also in later versions, until the
  administrator moves it again from that site's card: the one place the move stays offered
  without a failure.
- **What people read.** While the worker moves sites, the administrator reads one calm line on
  Home, in Problems now and on each site's card: "Sites are being moved into their limits (3 /
  12) — you do not need to do anything" ("Siteler sınırlarının içine alınıyor (3 / 12) — bir şey
  yapmanız gerekmiyor"); it reads again with the list and goes when the run ends (not while it
  waits for a kept record). A site inside its limits says so on its card and that is all. A
  customer and a reseller read nothing of the worker. There is no "Move into the limits" or
  "Move all" on a healthy site and no line asking to press one.

**What the site's customer notices.** Each running program of the site restarts once when it is
moved (the restart and the check that it runs in the slice and answers on its ports again,
bounded by its start time: these units set none, so systemd's 90 seconds, never under 20 or over
300; a program that does not come up in that time is put back where it ran and restarted there
once more). Its scheduled tasks keep their times; one run can be missed at the minute of the move
(§9.3). `crontab -e` for the site's account is closed (§9.5). A server with N sites outside their
limits is done after about N × (20 s + the site's own move) once the worker starts, five minutes
after the panel's start: a site without programs takes seconds, one with programs about as long
as its slowest program takes to restart and answer.

**Until alpha.149, and as Try again since:** `POST /api/v1/domains/{id}/site-limits/move
{request_id}` (`Agent.MoveSiteSliceV1`, after the tasks' `Agent.MoveSiteTasksV1`). One move at a
time across the panel (the worker takes the same flag); a repeated `request_id` answers the first
answer; it is audited as `site.limits_move`. A move (and Try again) is refused while the
panel's own update is admitted or running, by the panel and by the Agent ("the panel's
update is in progress; move the site after the update has finished"; fix round F6), and the
update is refused while a move runs (§8.6a). Only a move in flight does that: the Agent
keeps a flag of its own for the move's whole run, and the lock a status read takes for a
moment never makes the update or a move refuse (fix round F5). The Agent reads the update's
own records for this, read only: a queued or running record counts only while it can still
be a live update, created less than 31 minutes ago (the update's worker and its recovery are
each bounded to 15 minutes; the update's own reconciliation fails an older record), so a
record a dead worker left "running" never refuses moves for good (second fix round, R2). A
record that cannot be read refuses the move, on the safe side, with its own words ("it is not
known whether the panel's update is in progress"), and stops refusing by itself 31 minutes
after it last changed. The update's gate is held only for that check and the setting of the
move's flag, never while the move waits for its record's lock (R7): the update's admission
never waits behind a status read.

- The move holds the site's lock without waiting for it: a site being deleted or
  changed right now is not moved ("being changed; read its page again in a
  minute"), and a move never re-creates anything of a deleted site.
- It reads each unit first: a unit whose file an owner changed, one in an earlier
  protection, and one an owner's drop-in places (`Slice=` in a drop-in, read before
  anything restarts) are not moved and not restarted; the card names them with who
  acts, and they do not count in the "outside" line.
- Before its first write it records the journal (below). Then the slice, then each
  unit in turn **under that program's own mutation lock** (the lock every other
  change of the program takes; one lock per program, shared with no other program or
  site): the file must still be the text the move planned from (otherwise it is a
  change made meanwhile: kept, "changed"), CelikPanel's unchanged text gets
  `Slice=celiksite_<id>.slice` (re-sealed), systemd is reloaded, and a running unit
  restarts once and must run in the slice and listen again on every port it listened
  on before **within its start time**: the unit's own `TimeoutStartUSec` (these units
  set none, so systemd's 90 seconds), never under 20 or over 300 seconds. The restart
  itself is bounded by that same time whatever the unit's own `TimeoutStartSec` says
  (even `infinity`): a start still running then is "still starting" (systemd keeps the
  start job; only the move stops waiting), and the unit is put back. What waits
  behind what: a change of the program being moved waits for its step (at most twice
  its start time: the restart and a put-back); a change of the same site is refused
  while the move holds the site ("being changed"), as the move is refused while the
  site is changed; other programs and other sites never wait.
- **The move's time rule.** The move has 11 minutes. A running program is restarted only
  while what is left holds its own start time, its own put-back (a restart that may take
  that time again) and the put-back of every program moved before it, plus 5 seconds; the
  first program therefore always fits, even with the longest start time of 300 seconds,
  and the whole move, put-backs included, stays below the panel's 12-minute bound of the
  call. A move that stops for time says so: the programs it moved stay inside the limits
  and are marked done, the ones it did not start are "not tried", and the next move
  continues with them. **One press, one deadline** (second fix round, R4): when the move
  first settles its own site's kept record, that and the move share the same 11 minutes,
  counted from the press; what no longer fits is not started and is told as such.
- If a unit fails, it and every unit of the site moved before it are put back to
  their pre-move bytes, but only over the move's own text: a file changed or removed
  meanwhile is left as it is. Other sites are never touched. The answer says "put back"
  only for the programs confirmed back and names the ones whose put-back did not
  complete.

**The journal** (`site-slice-move.json` in the Agent's private folder) is written
**ahead of every write** and synced: before the first write, the pre-move bytes and mode
of every unit file the move will change, the slice file's bytes and mode (or that there
was none) with its ledger digest, and each unit's step; before a unit file is written its
step becomes "writing"; right after the slice file is written, its new bytes. A journal
write that fails is a failed step: nothing further is written, and what was written is put
back. At the Agent's start the step name alone is never trusted: each unit's file is
compared with the recorded pre-move bytes and with the text the move intended. **The
Agent's start never starts, restarts or waits for a site's program** (fix round F1): an
update waits only a limited time for the fresh Agent (about a minute from alpha.149, about
twelve seconds before), so nothing on the Agent's start path may wait for a customer's
program. At the start the record is only read and reconciled with the files:

| The unit file holds | At the Agent's next start |
|---|---|
| the pre-move bytes, step "planned" or "writing" | not touched ("not tried") |
| the pre-move bytes, a later step | already put back: left exactly as it is, running or not (a program its owner stopped stays stopped); its outcome is what this move's own record said, or, where the put-back was cut short, one reading: running in the system slice without a break for its start time (and without ports: the start reads none) is "put back", running there otherwise "not known yet", not running "not restarted yet" |
| the move's text, stopped unit | kept: its file names the slice, it starts there ("rewritten") |
| the move's text, running unit, step "moved" (it ran and answered in the slice) | kept ("moved") |
| the move's text, running unit, any other step | pre-move bytes back, ONE daemon-reload for all, and, unless it still runs in the system slice, ONE restart request for all of them without waiting for the jobs (`systemctl restart --no-block u1 … uN`): "not known yet" |
| anything else, or no file | left as it is ("changed") |

The start's calls to systemd are therefore at most four whatever the number of programs: one
reading of the programs whose place decides what happens to them, one daemon-reload, one
reset-failed and one restart request naming every program that ran when the move began. No
polling, no reading of ports, no wait. They share **one total bound of 15 seconds** (second fix
round, R5); when it runs out nothing further is requested, a reading or reload that did not
answer leaves "not known", a request not sent "not restarted yet", and the Agent goes on
starting. Whatever the one request's exit code, each program it named is "not known yet" and
the record stays. **"Not known yet" is resolved outside the start, by reading only** (fix
round F3, second fix round R1/R3): the site's status read (the card, and the five-minute
background reading behind Problems now) checks each program left "not known yet", "not
running", "not restarted yet" or "still starting". Its entry is closed only when ONE reading
shows the program active and running in the system slice, up without a break for at least its
start time (never under 20 seconds; systemd's `ActiveEnterTimestampMonotonic`), its file holds
its earlier bytes and, if it listened on ports before the move, it listens on a port now. A
program in systemd's restart cycle is never closed by one lucky reading. A program left "not
known yet" that is stopped, failed or inside the restart cycle becomes "not running", named on
the card and in Problems now with the administrator's action; anything else stays as it is. So
a program the administrator starts on its page, or one systemd brings up again after a passing
moment, is closed by the next read once it has run for its start time, and the line in Problems
now goes away by itself. When nothing is left, the record is closed and the slice file comes
back as below, only while the slice lock can be had at once (R6; otherwise the next read closes
it). A read never starts or restarts anything. Try again and a move's attempt on the same site
use the same table but restart a program that ran before and does not run where it ran, and
check it within its start time; they are bounded by the move's own 11 minutes (fix round F4:
a restart is sent only while what is left holds its start time and 5 seconds; the rest are "not
restarted yet"). A move of **another** site finds the record and is refused at once, before
anything of the first site is restarted.

**"Not answering" has an exit, and it is the administrator's press** (second fix round, R8). The
move's own put-back keeps saying "not answering, look at it". Try again restarts such a program
once more (the administrator asked), waits its start time, and if it then runs in the system
slice without a break for that time, its entry is closed as put back even though it listens on
no port; the answer names it ("runs, listens on no port"), so the administrator has been told
twice. One that went down and came up again within that time stays "not answering". A listener
table that cannot be read (`ss` fails) is "not known yet", never "listens on no port".

Unit files are written and put back through the writer every other managed file uses
(the text synced before the rename, the folder after it), so a power loss right after a
write never leaves an empty unit.

The slice file: when every unit is settled, no unit names the slice after this and the file
still holds exactly the bytes the move wrote, the recorded original comes back (its earlier
bytes and ledger digest, or no file where there was none); otherwise it stays. This is done
under the slice lock that changes of limits take, after the move record's lock (second fix
round, R6).

**A put-back that does not complete keeps the journal** (third fix round). That is: a
pre-move file that cannot be written; a unit whose place systemd cannot tell (a failed
status read is "not known": nothing is restarted on it, never "not running, so restart it");
a `daemon-reload` that fails (systemd would still hold the moved text, so no unit is
restarted); a restart that fails (the program is stopped) or is still starting at its start
time; a program that was restarted but whose place could not be told ("not known yet", never
"not restarted"); at the Agent's start, every restart it requested ("not known yet",
above) or did not send before its bound ran out ("not restarted yet"). While the journal is kept, no site is moved: a new move is refused with the
site whose move is unfinished. The site's card (administrator) says the last move did not
finish and, per program left, its state (put back but not running; place not known, not
restarted; still starting; not restarted yet; started again, not known yet; runs but no longer
answers where it listened before), `journalctl -u <unit>` for why, and the next step: for a
program that is not running or was not restarted, start it on its page or with `systemctl start
<unit>`; for one not known yet or not answering, look first (`systemctl status <unit>` and its
log), never "start it" (fix round W3); its **Try again**
button (`POST …/site-limits/move {request_id, recover: true}`, audited as `site.limits_move`
with `recover`) runs the put-back now and moves nothing new. A program started by hand
meanwhile is found running in the system slice and the journal goes; there is no button that
forgets the journal while a program stays stopped. Problems now has ONE administrator line
while the journal is kept (`site_move_kept`, opening the site). The status read settles what
it can see by reading, and Try again and the next move's attempt of that site try the same
put-back; the Agent's start only reconciles the record (above). Once it completes, the card says the last
move was interrupted and each program was finished, put back, or left as changed meanwhile,
until the site's next move.

### 8.5 Lowering a limit below what a site uses

Lowering `site.memory_mb` below what a site's programs use now makes the kernel stop
one of them at once (F4); lowering `site.tasks` below what they hold refuses new
processes and stops none of the running ones. The plan form, a reseller plan and an
account's own limits therefore show, in the change's preview before it is confirmed
(the panel's own confirmation), the sites whose programs use more now than the new
value, and say exactly that: a program over the memory limit is stopped at once; at the
process limit no new process starts and running ones go on. **The preview is enforced by
the server**, not only by the screen: a request that lowers a site limit below what sites
use now is saved only with the token of the list the server answers at that moment
(`confirm_stopped`, the preview's `stopped_token`); a request through the API without it,
or with the token of an earlier list (the sites' use changed meanwhile), is refused with
the current list. The same holds for the action of a default fixed for the host's earlier
memory (§8 "Limits"). Nothing else about applying limits changes. The server's own limits
screen does not list them (a declared gap: a lowered server ceiling or default of a
`site.*` key applies without a preview).

### 8.6 What people read when a limit is hit (D-024)

The Agent reads the slice's counters with the site's status read and, for all sites
that run programs, at most every five minutes in the background (in pages of at most
1000 sites and 4000 programs; a site may have any number of programs; no reading is
refused for the number of sites or programs; the ledger is read once per reading). The
panel records when a counter rose (`panel_settings` `site_limit_events_v1`; the first
reading of a site is its baseline; it is written only when something was reached, a
counter started again or a site is gone). The record is bounded: it keeps at most one
time a limit was reached per kind every five minutes and never more than a day's (288),
so however often a card is read, the record does not grow with it; the counts stay
whole.

- "The site's own limit" (memory, processes) is only what §8.1 proves. The card then
  says what happened and when, the limit that held then (alpha.149, the alpha.148 lab's
  CHECK 2: recorded with the event from the kernel's value at that reading; an event
  recorded earlier is shown without a number, never with today's value) and, where the
  limit in force is the plan's, who can raise it:
  "your provider" for a customer, "you, in the customer's plan within your own limits"
  for a reseller, the plan or reseller by name for the administrator. A site that
  reached its own memory or process limit at two or more readings in a day is a line
  in Problems now.
- **What is counted.** One time the site's own memory limit was reached is one program
  the kernel stopped while that limit was reached: the rise of the slice's `oom_kill`
  over a reading in which its own `memory.events.local` `oom` rose (at least one), never
  the raw `oom` count, which counts every failed reclaim (F4: `oom 10576` for one stopped
  program). A process refused at the site's limit counts each refused start.
- A program stopped for lack of memory without the site's limit (its own smaller
  limit, or a shortage of the whole server), or a process refusal the kernel does not
  attribute to the site's limit (the program's own limit, or on 6.8 where the unit has
  no `pids.peak` to tell), is said as only that ("a program of this site could not
  start new processes; the kernel does not show that it was this site's limit"): the
  plan is never blamed and no Problems now line follows.
- Where the limits do not apply (every program outside them, no slice file, a host
  that cannot enforce them, written but not in force yet, not known) the card says so
  instead of "stayed within their limits". **A customer or reseller cannot act on any
  of these**, so they read one neutral sentence ("these limits do not apply yet; the
  server's administrator sees to it; nothing is needed from you"); the administrator
  reads the reason and the step.

**Sites still to move.** Problems now has one line, for the administrator only:
the sites whose own programs run, or would start, outside their limits and that the
move can take (their number and the first five names). It opens the first site, is
a warning (a to-do), and goes away by itself when no site is left (`panel_settings`
`site_limit_outside_v1`, written only when the list changes).

### 8.6a What alpha.149 changed (the alpha.148 lab)

- **A put-back is checked like the move** (D6). After its restart a put-back program must
  run where it ran (active, in the system slice) within its start time, as the move's own
  step checks the slice; one that had ports must listen on a port again (fix round F10: a
  program put back where it ran that answers on a changed port runs and keeps no record;
  "not answering" is only a program that had ports and listens on none after its start
  time; a program without ports runs as soon as it is active in its place; Try again uses
  the same check). A program whose restart succeeded but that does not run there (a
  `Type=simple` unit's restart succeeds as soon as its process starts) is **not** "put
  back": the journal is kept with it as stopped, still starting or not answering. The
  Agent's start does not run this check (fix round F1): it only requests the restart and
  keeps the program as "not known yet" (neither a failure nor a success), which the status
  read resolves by reading (§8.4: only for a program that runs without a break for its start
  time, second fix round R1). A listener table that cannot be read is "not known yet", never
  "listens on no port" (R8). The journal also records each program's ports (an earlier
  journal has none and is checked without them). The card and Problems now say that the
  program is not running now, that the administrator acts, the next step (its own log,
  `sudo journalctl -u <unit>`, then **Try again** or start it) and that nothing else was
  changed. The move's window says "put back where they ran, running again" only of programs
  that ran before the move, and "put back as they were" of the others (fix round W1).
- **A program inside systemd's automatic restart cycle** (`activating`/`auto-restart`, no
  cgroup) is shown as not running, never as placed by the server owner; it is not counted
  for the move.
- **Reasons in the page's language** (D7, O3). The Agent answers a move's failure, its
  put-back and a whole move it refused as a code with its values (`not_running`,
  `still_starting`, `not_in_slice`, `not_listening` with the ports, `restart_failed`,
  `another_move`, `site_busy`…); the page words them in English or Turkish, and a code it
  does not know is a general sentence pointing to the Agent's journal, where the English
  detail is written. The plan preview's site lines ("memory 215 MB in use, new limit
  128 MB") are sent as values and worded by the page; one site and one account are said
  in the singular.
- **The unapplied-limits line is true** (O3). On every way out of a push the Agent reads
  the running slice's kernel values again before it answers: a site whose kernel holds its
  effective values has its limits whatever step failed; one whose values could not be
  compared is "not known" (its own Problems now wording), never "does not have the limits".
  The recorded reason is a code (`kernel_differs`, `unchecked`, `write_failed`, …), kept per
  subscription with the number of sites for each (fix round F9): the one line words each site
  by its own reason. Only sites whose limits file was not written or whose kernel still holds
  the earlier values are said to keep the earlier limits; for the others (not read, the Agent
  did not answer or did not take the change) it says that it is not known.
- **A reached process limit on Ubuntu 24.04** (D3). Without `pids.events.local` the slice's
  own `pids.peak` is read with the `pids.max` it was read under. `pids.peak` is a high-water
  mark that a changed limit does not reset, so the card says "reached their process limit"
  only when the limit is unchanged since the previous reading, that reading had the peak and
  was below the limit, and the peak now reached it (fix round F7): a limit lowered below an
  earlier peak, or the first reading after the update, is not a reached limit. The kernel
  there does not count how often, nor whether a new process was refused, and the card says
  so (it never says that new processes could not start).
- **The limit then** (fix round F8). Before a change of limits is pushed, the panel reads the
  affected sites' counters once, so that what happened before the change is recorded with the
  limit that held. When that reading cannot be made, the events the next reading finds carry
  no limit value (they may be from before the change), never the new one.
- **The reseller's bound** (CHECK 2). Where neither the server nor the reseller sets a
  ceiling, the server's default bounds a reseller's customer; the card names it as that
  default ("which also bounds the reseller"), not as a limit the administrator set.
- **No update during a move** (D5). While a site's programs are being moved, the panel's own
  update is not started: the panel and the Agent both refuse it, and the update page says
  to wait for the move's window and press update again. Only a move really in flight does
  that (fix round F5: the Agent's own in-flight flag, never the lock a status read takes).
  The other way round, a move or Try again is refused while the update is admitted or
  running (fix round F6); a stale update record stops refusing by itself (second fix round,
  R2, §8.4). The Agent's start writes one journal line per startup step with its time
  (`agent start: <step> took …`).

### 8.7 A deleted site's slice

When a site is deleted, CelikPanel's unchanged slice file goes and the ledger
forgets it; a file the owner changed, and the owner's drop-in folder, are moved
aside under names systemd does not load (`<file>.celikpanel-aside-<UTC time>`) and
named in the audit log (`site.limits_file_aside`). **When a site is created** (nothing
of it exists yet: its home and account are new), a slice file of its id that the Agent's
ledger does not know (a site of an earlier installation with the same id) is moved aside
the same way **together with its drop-in folder**, so the new customer's site inherits
neither; it is named in the Agent's log. Where such a file or folder is there and cannot be
classified (the ledger cannot be read) or moved aside, **the site is refused** before
anything of it is made, with the file and the reason ("a configuration file for this name
already exists on the server"; the server owner moves it away, then the site is created
again); with no such file the creation goes on whatever the ledger. A later write for an
existing site never moves an owner's file or drop-in aside. A slice file the ledger knows (an earlier site of this same
installation whose deletion did not run) is left as it is. **A ledger that cannot be
read** is an error, never an empty ledger: nothing is written, removed or moved aside on
that reading, and the caller says so (a limit change's Problems now line, the move's
answer, the deletion's failed step, the Agent's log for a program's own write).

**Owner independence (D-022).** The slices are native units. With the panel
stopped, removed or unlicensed, they and their limits keep working. To lift or
change a site's limits without the panel, the owner adds a drop-in
(`/etc/systemd/system/celiksite_<id>.slice.d/owner.conf` with, for example,
`[Slice]` and `MemoryMax=infinity`) and runs `systemctl daemon-reload`. After `sudo
systemctl daemon-reload`, an owner's change reaches running programs at once on Ubuntu 24.04
and within seconds to about a minute and a half on Debian 13 (alpha.148 lab: within 15 s),
without any program starting; a program's start applies it at once. CelikPanel never writes or removes it.

**Taking one program out of its site's limits, the panel staying.** The panel has no action
for it. The owner adds a drop-in beside the program's unit, the same on Debian 13 and Ubuntu
24.04: `/etc/systemd/system/<unit>.service.d/slice.conf` with `[Service]` and
`Slice=system.slice`, then `sudo systemctl daemon-reload` and `sudo systemctl restart <unit>`.
CelikPanel never writes or removes a unit's drop-ins, and nothing of the panel refuses the
program afterwards: the status read takes the drop-in's `Slice=` from `DropInPaths`
(`siteUnitDropInSlice`), so the administrator's card shows the program outside the limits with
"the server owner placed it with an addition of their own; it stays there" and the "outside"
line does not count it; the move leaves it, not restarted (`owner_slice`); a save writes
CelikPanel's own unit file as usual (its `Slice=` line, kept by the rule of §8.3, is outranked
by the drop-in), so no `unit_edited` or kept file follows; the mail block's helper and proof
accept the system slice. Removing the drop-in, then the same two commands, brings the program
back inside. Editing CelikPanel's unit file instead makes it the owner's (D-031): a service
app's next activation is refused with `unit_edited`, and a program's or Node.js site's writes
are kept beside it as `.celikpanel-pending`, so the panel no longer manages that unit.

**What stays on the server when the panel is removed.** The limits keep working, also
across reboots: at each start systemd applies the slice file to the programs whose unit
names it (measured with the file present across a reboot, facts N4). What stays behind:

- `/etc/systemd/system/celiksite_<id>.slice`, the owner's `celiksite_<id>.slice.d/`
  folders, and any `celiksite_<id>.slice.celikpanel-pending` and
  `….celikpanel-aside-<time>` copies;
- the `Slice=celiksite_<id>.slice` line in each moved program's unit file;
- the Agent's own records in its private folder (the ledger, `site-slice-move.json`,
  `site-slice-move-outcomes.json`, `site-slice-runtime.json`): they do nothing without the
  Agent;
- only if the Agent stopped in the middle of a push: control drop-ins in
  `/run/systemd/system.control/celiksite_<id>.slice.d/`, gone at the next reboot.

To remove the limits by hand once the panel is removed: give each program the drop-in
above (`<unit>.service.d/slice.conf` with `[Service]` and `Slice=system.slice`), or change
`Slice=celiksite_<id>.slice` to `Slice=system.slice` in its unit file, remove the
`celiksite_*.slice` files and their `.d` folders, run `systemctl daemon-reload` and restart
each program. To keep the programs where they are and only lift a limit, use the drop-in
above.

### 8.8 Going back to an earlier release

**An update that fails returns by itself, before anything of this section exists.** The
update's automatic return covers a failure up to the new panel's read-only start check; the
update's completion mark is set after that check and before the new Agent and panel are
started. This release does nothing inside the update itself: it has no database migration
(the layout stays schema 81), the updater rewrites no program's unit, and the release
archive installs no helper (the Agent puts its own copy of the sandbox helper and the guard
in place at the first program write after its start). An automatic return therefore finds
no slice file, no moved program, no `site.*` or `db.*` value written by this release and
none of the Agent's records of this section.

**A completed update is not taken back.** From the moment the new Agent and panel are
started the update is carried to its end, also when that start fails. The panel offers no
way back to the release before; the release's `rollback.sh` refuses a completed update
before it stops anything, with
`!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`;
and `sudo /usr/libexec/celikpanel/recovery recover` only finishes an update or rollback
that was interrupted. So no product path brings the release before (or an earlier one)
back onto a server that has run this one. The release before, alpha.147, already accepts a
program that runs in its site's slice (its helper, guard and proof), keeps limit keys it
does not know when it saves a plan or account, and keeps a unit's
`Slice=celiksite_<id>.slice` line while that slice's file exists; that is preparation, not
a way back.

**What this release did is undone on this release:**

- A site's limits are lifted or changed without the panel by the owner's drop-in (§8.7,
  "Owner independence"); clearing a `site.*` key in the plan, the account or the server's
  limits brings the site back to the default.
- A program is taken out of its site's slice by the owner's drop-in (§8.7, "Taking one
  program out of its site's limits"): the panel has no action for it, and keeps managing the
  program.
- A database user's time and temporary-file limits: clearing the `db.*` key where it is
  set makes the panel remove the value it wrote itself (and only that); a value the server
  owner set by hand is the owner's to remove (`ALTER ROLE … RESET …`, or for MariaDB
  `ALTER USER … WITH MAX_STATEMENT_TIME 0`).

**Going back to alpha.149 from alpha.150, and the move made by the panel itself (D-048).**

- **Nothing of the worker runs before the update is complete.** The update's automatic return
  happens before the new panel starts (above), and the worker is part of the new panel: it
  starts five minutes after that panel's start, and before each site it waits while the
  panel's update is admitted or running, while the server is busy with another change (the Agent's own readiness) and
  while the Agent refuses with `update_running` (an update record queued or running,
  created less than 31 minutes ago) or `update_unreadable`. The component test
  `TestTheWorkerStartsOnlyAfterTheGate` (`cmd/panel/site_auto_move_test.go`) proves that nothing
  is sent and no site is marked while any of these holds, and that the same test fails when the
  panel's own gate is taken out. An automatic return therefore finds no site moved by the
  worker and no mark of it.
- **A completed update undone outside the product** (not a product path, above): the programs
  the worker moved keep `Slice=celiksite_<id>.slice` in their unit files; alpha.149 knows the
  slice (it is alpha.148's), keeps that line while the slice file exists and starts them there,
  and its own "Move into the limits" finds them "already". Their scheduled tasks are as §9.4
  says for a site moved by the administrator (they keep running as timers; alpha.149's Scheduled
  tasks screen cannot read them). The worker's marks (`panel_settings` `site_auto_move_v1`, and
  `site_tasks_put_back_v1`) are settings alpha.149 does not read: they do nothing there, and a
  later alpha.150 or newer finds them (marks of another version count as none). alpha.149 again
  offers its button on a site still outside; nothing moves by itself there.

## 9. A site's scheduled tasks inside its limits (S4 R2b)

*22 November 2026; brought onto alpha.149 on 26 November 2026 (one move record, the update
gate and one deadline per press, as for the programs' move, §8.4) · In source and
component-tested; the facts below were measured on throwaway Debian 13 and Ubuntu 24.04
machines; the panel's lab round on both is open (steps in the pull request).*

A moved site's scheduled tasks no longer run through cron. Each enabled task is a
systemd timer and service pair, `celikcron-<site id>-<n>.timer` and `.service`, whose
service runs in the site's slice (§8) under the network rules of the site's other
programs. **New sites are moved from the start. An existing site is moved by the panel
itself** (alpha.150, D-048), with its programs, by the worker of §8.4; nobody presses
anything.

**What still runs outside the limits after this step.** Say it plainly: the site's PHP
(its PHP-FPM pool; R3), the programs and the crontab of a site not yet moved (until the
panel's worker reaches it after the update, or one it could not move: Problems now names
it with Try again), a program
written before R2a again after each restart of the server until it is moved (§8.3), a
crontab made again for a moved site's account (reported, §9.4), and programs of a
site account that already lingered before this step (§9.5).

### 9.1 Facts measured on real systems (22 November 2026)

Plain `crontab`, `systemctl` and `loginctl` on two throwaway machines started from the base
images: Debian 13 (kernel 6.12.105, systemd 257.13, cron 3.0pl1-197, polkitd 126 installed
and started on demand) and Ubuntu 24.04.4 (kernel 6.8.0-138, systemd 255.4, cron
3.0pl1-184ubuntu2, polkitd 124 running). Both gave the same answers unless a cell says so.

| | Question | Answer |
|---|---|---|
| F1 | `/etc/cron.allow` leaves an account out | its **existing crontab keeps running** every minute; `crontab -u <account> -l`, `-r` and installing a file are refused even for root ("The user … cannot use this program (crontab)"); root can still read, remove and rewrite the spool file `/var/spool/cron/crontabs/<account>` in place (not create it: `fs.protected_regular=2`), and cron notices a removed or changed file at the next minute without a signal |
| F2 | The same line run by cron and by a timer + service (`User=`, `Slice=`) | the same account, groups, home, working folder, `LANG`, capabilities; different: `PATH` (cron `/usr/bin:/bin`, on Ubuntu the one in `/etc/environment`), `SHELL` (cron sets `/bin/sh`), umask (cron 0002), the session's login id, extra systemd variables, stdin (an empty pipe, `/dev/null`) and stderr (the journal) |
| F2 | `%` in a command; two `OnCalendar=` lines; `AccuracySec=` | byte-identical standard input; both lines accepted and both next times listed; the default accuracy is one minute, so `AccuracySec=1s` keeps cron's minute |
| F2 | A job longer than its interval | cron starts a new copy every time (they overlap); a timer never starts a second copy, and a time that passed while it ran starts it **once more right after it ends** (`Type=oneshot` and `Type=simple` alike); the journal says nothing about the skipped time |
| F3 | A site account with no session (as a site's PHP) runs `loginctl enable-linger <itself>` | **allowed** with polkitd as shipped (`set-self-linger` is "yes" for anyone); it then runs its own user services in `user@<uid>.service`, outside its slice; Debian without polkitd: "Access denied" for every account but root |
| F3 | A rules file that asks for an administrator unless the caller is in the administrator group | works on polkit 124 and 126: the site account gets "Interactive authentication required", an administrator and root still succeed; an existing `/var/lib/systemd/linger/<account>` stays (still lingering, also after `systemd-logind` restarts) |
| F4 | The outgoing-mail block | a cron job runs in `/system.slice/cron.service` and is **not** blocked; a service in the site's slice is blocked only when its own cgroup is added (the helper's `egress-add`), not by being in the slice |

### 9.2 The files and how a task runs

| File | What |
|---|---|
| `/etc/celikpanel-cron/<site id>.crontab` | the site's tasks in crontab form, the one source (0640 root:root in a 0711 folder; CelikPanel's render header and a six-line `#|` intro) |
| `/etc/celikpanel-cron/<site id>.d/<n>.sh`, `<n>.in` | task n's command exactly as cron hands it to the shell, and its standard input (`%`); 0640 root with the site's group, in a 0750 folder of that group |
| `/etc/celikpanel-cron/<site id>.d/env` | the crontab's `NAME=value` settings (an environment file systemd reads as root) |
| `/etc/systemd/system/celikcron-<site id>-<n>.service`, `.timer` | the units, under the D-031 rule |

The command and its input never stand in a unit's own settings: systemd tells every local
account a unit's settings (`systemctl show`), while a crontab is readable by its account
only. A task keeps its number while its text stays the same (its service names it with a
`# celikpanel-task:` line), so its last run stays its own.

**The service** (`Type=oneshot`, `KillMode=process`, `NoNewPrivileges=yes`):
`ConditionPathExists=` the site's slice file (below), `User=` and `Group=` the site's
account, `Slice=` the site's slice, `WorkingDirectory=` its home, `UMask=0002`,
`Environment=PATH=/usr/bin:/bin SHELL=/bin/sh`, then `EnvironmentFile=-/etc/environment`,
`-/etc/default/locale` and the site's own settings file (a later file wins), `StandardInput=file:<n>.in` where the task has input,
`ExecStart=<the crontab's SHELL> <n>.sh`, the SMTP block's helper lines (§3, through the
tasks' own copy of the helper, `/usr/lib/celikpanel-sandbox/sandbox-task-helper`, §9.4),
`RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK`, the programs' log rate limit
(`LogRateLimitIntervalSec=30s`, `LogRateLimitBurst=1000`) and `LimitCORE=0`, and, where
systemd enforces it, `IPAddressDeny=` of link-local and metadata addresses. It does **not**
get the programs' file system sandbox (§1): a cron job never had one, and it would stop
tasks that write the site's files. PHP and the file system view join in R3.

**No extra rights inside a task (second fix round).** `NoNewPrivileges=yes` is set, as for
the site's programs. Measured on systemd 257 and 255: `RestrictAddressFamilies=` with `User=`
does **not** imply it (a set-group-id `crontab` gained its group inside a task without it). So
a program that needs extra rights does not work in a task: `sudo`, `su`, `crontab`, `at`,
`ping` with file capabilities, `dotlockfile`, and the server's own `sendmail` command - and
therefore PHP's `mail()` called from a command-line script, which hands the message to
`sendmail`. A task sends mail by SMTP with a mailbox's name and password (§3). The move's
preview names every task that calls such a program in command position (the start of the
command, or after `;`, `&&`, `||`, `|`), by the program's name.

**What a task leaves running keeps running, inside the limits (fix round).** With
`KillMode=process`, systemd stops only the task's own shell when it ends; a process the task
started in the background (`nohup worker &`, `setsid ./queue &`) stays in the unit's cgroup,
so inside the site's slice (its memory, process and processor limits) and under the SMTP
block: the helper's `egress-remove` keeps the cgroup in the set while any process other than
itself is in it, and the guard re-adds such cgroups after a flush (§3). Both fail towards the
block: where the helper cannot tell where the task ran or read its cgroup's processes, the
cgroup stays in the set; where the guard cannot read a task cgroup's processes, it adds the
cgroup as if one ran there, and a running unit whose helper line cannot be read is added as if
its helper loaded the block (alpha.150). The next run still
starts. systemd logs "Unit process <pid> (<name>) remains running after unit stopped." when
the run ends, and "Found left-over process <pid> (<name>) in control group while starting
unit. Ignoring." when the next run starts; both are expected. `systemctl stop` of the task's
service does not end them; `systemctl kill <service>` does. Removing a task (or the site's
move back into cron) does not end them either, as cron did not: a process a task leaves
behind **keeps running inside the site's limits until the administrator ends it**
(`systemctl kill celikcron-<id>-<n>.service`, also after the task was removed). Measured:
after its unit file is removed, it stays in the unit's cgroup under the slice's limits and the
address rule, and the guard keeps it under the SMTP block by its cgroup's path (§3). A program
that should run all the time belongs in the site's Programs, which the customer can see,
restart and stop; a task is for work that ends.

**The site's limits file first (fix round).** No timer of a site is switched on unless the
site's slice file is in place and loaded by systemd: a new site's file is written with the
site, every Scheduled tasks request carries the site's limits and writes a missing file, and
the side doors step after each Panel start puts a missing file back from the limits the Panel
sends. A file that cannot be written is an answer with who acts, never a log line: a save
answers that the task waits for the site's limits (the task file is saved, its timers are
not written yet), the move is refused before anything changes, and a site whose running
timers find their file gone at the start has them held (stopped and remembered; they start
again at the first start that finds the file back) with a Problems now line.

**Held without the panel too (second fix round).** Every task service carries
`ConditionPathExists=/etc/systemd/system/celiksite_<id>.slice`. Measured on both systems,
with two reboots: while the file is missing, systemd **skips** each run (an information line
"skipped, unmet condition check", the unit not failed, `systemctl --failed` empty), also
after a reboot and with no panel at all; the Scheduled tasks screen shows such a run as
skipped, not failed. **They do not run while the limits file is missing; they run again at
their next time once it is back.** One measured gap: a slice file put back **without**
`systemctl daemon-reload` leaves the slice that systemd made without a file in place, so the
next runs have **no** limits until a reload or a reboot. CelikPanel's own way of putting the
file back (a save of the subscription's limits, the start-up step) writes it, reloads
systemd and pushes the limits to the running slice before the held timers start again. The
server owner who puts the file back by hand runs `systemctl daemon-reload` right after.

**The timer**: `OnCalendar=` translated from the line (`internal/sitecron`): every value
list as systemd's list, cron's OR of the two day fields (neither starts with `*`) as two
`OnCalendar=` lines, `7` and names for the weekday, month names, steps on ranges, and the
special strings; `@reboot` becomes `OnBootSec=1min`, **enabled but not started** (it runs
once after each start of the server, as cron runs `@reboot` lines when it starts);
`AccuracySec=1s`; `Persistent=false` (cron catches nothing up). A line the translation does
not know (`L`, `W`, `#`, `?`, a value out of range, a backwards range, a step of 0) is not
translated.

**A line is read exactly as cron 3.0pl1 reads it, or refused (fix round; Debian's `entry.c`,
`env.c` and `cron.h`).** Only an ASCII space or tab separates fields, in the task file and in
the screen's own check of a schedule: a no-break space or another Unicode blank is part of a
field, which is then refused as cron refuses it. The command keeps its trailing blanks. A
command of 999 bytes or more is refused (cron refuses it as too long); the screen refuses to
save one. A setting line follows `load_env`: `NAME=` is the empty value, `A="x` and `A="x"y`
are not settings (cron then fails them as jobs), quotes and trailing blanks come off as cron
takes them off, and a setting line of 1000 bytes or more is refused (cron would cut it
silently). A value systemd cannot read in an environment file (not valid UTF-8, U+FEFF, a
noncharacter, a quote mark, a backslash, a control character) is a line that cannot be
translated: it keeps a crontab in cron at the move, is never written into the settings file,
and a save of a task file holding one is refused. cron accepts a few forms the translation
refuses (numbers with more than two digits such as `007`, steps over 99, a backwards weekday
range such as `fri-sun`); those are shown with their reason. **A setting that is part of the
schedule for some crons is refused (alpha.150 fix round):** `CRON_TZ=`, `RANDOM_DELAY=` and
`START_HOURS_RANGE=` change when jobs run under cronie (and systemd-cron), while Debian's and
Ubuntu's cron 3.0pl1 keep them as plain settings (its program has no such name). A timer would
keep only the setting, so the schedule could change silently; which cron a server runs cannot be
told apart reliably, so such a line keeps the whole crontab in cron at the move (named, with
why) and a save of a task file holding one is refused, on every server. The server owner
deletes the line and gives the time in each task's own schedule. systemd-cron's own names that
change a job's timing (`PERSISTENT=`, `BATCH=`, `DELAY=`) are **not** refused (fix round 4, as
it is): the supported systems run their distribution's cron, for which they are plain settings,
so a host that runs systemd-cron instead keeps such a line as a setting only. **So are the names the Go
runtime reads (alpha.150 fix round):** `GODEBUG`, `GOGC`, `GOMAXPROCS`, `GOMEMLIMIT`,
`GOTRACEBACK` and `GORACE`. The site's settings file is also the environment of the task's
helper lines, which run as root (`ExecStartPre=+`); the helper is a static Go program that clears
its environment first thing, but its runtime reads these before that. Such a line is refused (it
keeps the crontab in cron at the move and is shown as not running), never written into the
settings file; a task that needs one sets it in its command (`GOGC=off ./job`).

**A cron job and a timer task: what differs.**

| | cron job | timer task |
|---|---|---|
| account, groups, home, working folder, `LANG`, shell | the same | the same |
| `SHELL`, umask | as cron sets them | made the same (above) |
| `PATH` when `/etc/environment` sets one | Debian: still `/usr/bin:/bin`; Ubuntu: the one in `/etc/environment` | the one in `/etc/environment` on both (it is read after `Environment=`; measured with `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`); the crontab's own `PATH=` wins over both |
| a variable set both in `/etc/environment` and in the crontab | the one in `/etc/environment` (PAM sets it last) | the crontab's (the site's own settings file comes last) |
| `export NAME=value` lines and names with a dot or a dash (`MY.NAME`, `MY-NAME`) in `/etc/environment` | `export` lines are taken; dot and dash names are ignored | all three are ignored (systemd logs "Ignoring invalid environment assignment"); the task still starts |
| `NAME = value` (blanks around `=`) and indented lines in `/etc/environment` | ignored; the indent is dropped | taken as `NAME=value`; an indented line keeps its leading blanks in the name |
| `$0` | `/bin/sh` | the task's file (`<n>.sh`) |
| extra environment | — | `USER`, `INVOCATION_ID`, `JOURNAL_STREAM`, `MEMORY_PRESSURE_*`, `SYSTEMD_EXEC_PID`, `TRIGGER_*` |
| login id (audit) | set by PAM | not set |
| standard input without `%` | an empty pipe | `/dev/null` (both end at once) |
| output | not mailed (`MAILTO=""` since alpha.125) | kept in the journal with the run |
| open files hard limit (Ubuntu) | 1048576 | 524288 |
| still running when its time comes | a second copy starts | not started again; starts once more right after it ends |
| a process left running in the background | keeps running | keeps running, in the site's slice and under §3's rule; systemd logs it (above) |
| a set-user-id or set-group-id program (`sudo`, `crontab`, `ping` with file capabilities, `dotlockfile`, `at`, the server's `sendmail` and so PHP's `mail()` from a script) | gains its rights | runs without them (`NoNewPrivileges=yes`, set explicitly) and may fail; mail goes by SMTP with a mailbox's login |
| the site's limits file is missing | — | the run is skipped (not failed), also after a reboot and without the panel |
| the server owner's per-login limits (`/etc/security/limits.conf`, `pam_limits`) | apply (cron opens a PAM session) | do not apply (no PAM session); the site's slice limits do |
| many tasks due in the same second | all start in that second (cron has no random delay) | the same: no random delay is added |
| limits, outgoing mail | none; mail to every port | the site's slice; §3's rule |

**What people read.** The Scheduled tasks screen says, for a moved site, the two halves of
a long run in one sentence and the differences from cron a site may notice (what it starts in
the background keeps running, inside the limits; output kept, not mailed; no login session,
so the per-login limits do not apply; a set-user-id program runs without its rights; `$0`);
the move's preview names each task that calls a common set-user-id program and says how a
task can send mail (or that it cannot, §3); and for each task its last run (when, how long, exit status
and systemd's word for it, the last ten lines it wrote, from its own journal entries), when
it runs next, and a task whose timer the server owner turned off, whose timer CelikPanel
wrote but could not switch on yet (second fix round: its own state, never "turned off"; a
save is recorded as applied only once every new timer is on, and the next start or save
tries again), whose unit was changed on the server, that is not scheduled yet ("save it
again"), whose last run was skipped because the site's limits file was missing, or whose
line does not run (with why). After a restart of the server, a task that has not run since says "No run since the
server started" (systemd forgets the last run then; the journal keeps it). The customer sees
only their own site's tasks. A finished run's last lines are read from the journal once and
kept (a bounded cache); one reading of the list starts at most eight journal readers, and the
runs it has not read yet show their lines at the next reading.

### 9.3 An existing site's crontab moved by the panel itself, and the undo

The panel's worker (§8.4, D-048) moves each existing site's crontab into timers by itself,
before the site's programs, in the same press. A site that has no crontab is moved too: its
account then leaves cron (`crontab -e` is closed for it, §9.5) and its future tasks are timers.
Try again (next to a move that did not bring the site inside) and the move of a site put back on
purpose read a **preview** first
(what becomes what, the settings and paused tasks that come along, a task that calls a
set-user-id program, and a task that sends mail: after the move it can send only through
this server's own submission service, or not through this server at all, §3). A line that
cannot be translated, and a line not made in CelikPanel that the administrator has not kept
(S4 R1), keep the **whole** crontab in cron and are named; nothing is half-moved (the
worker then marks the site "left": its programs go in, its tasks stay in cron, and the card
names the lines with who acts, never a Problems now to-do). The move
holds the same locks as the programs' move (one move at a time; the site's lock without
waiting, so a site being deleted is not moved) and the lock of every change of the site's
tasks; it restarts nothing.

**A moved site whose account may still install jobs is not a state CelikPanel creates (fix
round).** Before anything changes, the move is refused, with who acts, when cron's access
files would still let the account (or a login sharing its uid) use `crontab` afterwards: the
server owner's own `/etc/cron.allow` names it (the owner removes the name, or deletes the
file so CelikPanel writes its own), CelikPanel's own file was deleted by the owner (the
administrator presses "Close it again" in Problems now), or accounts come from a directory
service (the owner keeps the account out with their own file). `/etc/cron.deny` naming the
account is enough. The preview says so too, and a new site whose account would stay allowed
keeps cron - checked again once CelikPanel's `cron.allow` was brought up to date, so a write
that failed keeps the new site in cron too (alpha.150 fix round).

**The record** (the move's one record, `site-slice-move.json` in the Agent's private folder:
version 3 with a `tasks` part; a programs' move writes version 2): the crontab's exact bytes
(as `crontab -l` printed them) and each step, written before the next step starts. From
`source_written` on, the task file holds the same crontab too.

| Step reached | What it means | At the Agent's start |
|---|---|---|
| planned | the crontab's bytes are recorded | nothing ("not tried"); a task file written before its record is removed |
| source_written | the task file holds the crontab | put back (below) |
| units_written | every timer and service written and checked by systemd, none started | put back |
| cron_closed | `/etc/cron.allow` leaves the account out (checked), and every `crontab` process of its uid started before that is ended (it passed crontab's check before and could still install a file); when those processes cannot be listed, the move fails here and is put back, nothing removed (second fix round); so does a process of the uid whose program cannot be read, or one that cannot be ended (alpha.150 fix round: never skipped), and the list is read once before anything changes, so a list that cannot be read refuses the move then. A process's program is known by its file (the file `/proc/<pid>/exe` leads to, compared with the `crontab` program's), not by the link's text, which the kernel cannot print for a program started from a folder deeper than a path can be long (fix round 4): such a program cannot keep the site's move refused. A process whose file cannot be read still refuses, worded by its own code: the administrator looks at the account's processes and ends the one the Agent's journal names. A `crontab` process is ended with SIGKILL and not waited for (as it is): the spool is read after it, at the next step, and a crontab it installed before it ended changes the move into a put-back | put back |
| crontab_emptied | the spool file, checked to hold the recorded crontab, is removed directly (root may; `crontab -u` is refused by now) and checked gone | the account's spool is read first (below); with no crontab: finished, the timers switched on ("moved"). If they cannot be, the record stays for Try again and is not put back there (that would take more calls than the start makes); the timers are switched off again in one call - always, also when the site's limits file looks missing (an enable sent before the Agent stopped can have switched some on) - so the site's tasks are known not to run until Try again |
| enabled | the timers enabled and started | the account's spool is read first (below); with no crontab: finished ("moved"). With a crontab (or a spool that cannot be read) the record is saved as `crontab_emptied` before the timers are switched off (alpha.150 fix round 4), so once the crontab is gone the next pass switches the timers on again as in the row above, never "moved" with them off |

**One run can be missed at the moment of the move, none runs twice** (alpha.150 fix round): the
timers start only after the crontab is gone, and a minute can begin in between (cron has no line
any more, the timer is not on yet), so a task due in that minute runs at its next time instead.

There is no moment in which the site's own code can put a crontab back: the account leaves
cron before its crontab goes. The crontab must still be the one recorded when it goes: a
change made meanwhile is never lost (the move is put back and says so). **Putting back**
switches the move's timers off, lets the account use cron again and writes the recorded
crontab back when it is gone, checks it with `crontab -l`, and only then removes the timers,
the files and the record. The crontab is written back only after the call that switches the
timers off has answered success: a batched `systemctl enable --now` that failed half-way can
have left some of them on, and a crontab beside them would run every task twice. When that is
not confirmed, or the crontab cannot be written back, the record, the task file and the units
stay (the timers the record names switched on again where the site's limits allow), so
nothing is lost and nothing runs twice. The put-back never switches the timers on again
beside a crontab it wrote but could not read back the same (alpha.150 fix round): the recorded
crontab in the spool counts as back; anything else there, or a spool that cannot be read,
keeps the timers off and the record (the tasks run from cron). **This holds for one pass** (a
known limit, fix round 4): a later pass of a record kept at `source_written`, `units_written` or
`cron_closed` (Try again, the Agent's start) takes any crontab it then finds as back and finishes
the put-back - the timers, their files, the task file and the record go, and the crontab stays as
it is, so the recorded crontab is then only in what that crontab holds. At `crontab_emptied` the
spool check below keeps the record instead. After the move, the move never
answers "moved" while a crontab exists for the account.

**One record, one line, Try again (alpha.150, on alpha.149's move record).** The tasks' move
and undo keep the same one record as the programs' move (§8.4). While it is kept:

- **Every new move and undo is refused** before anything changes (`kept_earlier`): a record
  of another site at once; a record of the same site is first settled within the press's own
  deadline, and the press goes on only if it then is. The Panel does not start the programs
  part of a press whose tasks part answered "unknown" or was refused before it began, and
  the panel's worker stops its run there until the record is settled (§8.4).
- **One line in Problems now** ("A move into a site's limits did not finish") opens the site.
  For a tasks record it says how the site's scheduled tasks run now, as the Agent recorded
  it: **not running** (the crontab emptied, the timers off) until the administrator presses
  Try again; running as timers inside the limits; or running from cron with switched-off
  timer files left. Only when the Agent could not tell does it say that they may not run.
- **The site's card** says the same, the step the record reached, **Try again**, and the
  server owner's way back into cron by hand should Try again not settle it (below).
- **Try again** settles the record: it finishes the step or puts the move back, within the
  press's deadline. The Agent's start tries too, within its bound (below).

**A crontab while the record is kept (alpha.150 fix round).** At `crontab_emptied` and
`enabled` the account's spool is read first, at the Agent's start and at Try again (a file
read, not a systemd call). A crontab there - the server owner put the tasks back by hand -
or a spool that cannot be read **never gets the timers switched on beside it**: they are
switched off in one call and the record stays; the card and Problems now say the tasks run
from cron (or, when the spool cannot be read, that it is not known). At Try again a crontab
that holds the recorded tasks - as `crontab -l` printed them before the move, or the task file
installed whole - finishes the put-back: the timers and their files and the task file go, and
cron lets the account in again. A crontab that holds anything else is left as it is and the
record stays (the comparison is exact, byte for byte, so it can only fail towards "kept"); the
server owner compares it with the task file and keeps one, and the card says how (fix round 4),
then the administrator presses Try again: **to keep the timers**, the owner removes that
crontab (`sudo rm /var/spool/cron/crontabs/<account>`, `/var/spool/cron/<account>` with cronie;
`crontab -r` is refused for the account) and Try again switches the timers on again (the
`crontab_emptied` row); **to keep cron with the recorded tasks**, the owner removes it the same
way and runs the way back by hand below, and Try again finishes the put-back. At the other steps
a record whose tasks run from cron is finished by Try again whatever the crontab holds (the
put-back's and the undo's own rows), and the card says only that.

**The way back into cron by hand** (the card gives it with the site's id and account, should
Try again not settle the record; run for real on systemd 257 with throw-away units, including
a crontab already there and a second run after a stop at every step, fix round 4; each step
only if the one before worked):

```sh
{ [ ! -e /etc/cron.allow.celikpanel-before ] || sudo mv /etc/cron.allow.celikpanel-before /etc/cron.allow; } && { sudo test ! -e /var/spool/cron/crontabs/<account> && sudo test ! -e /var/spool/cron/<account> || { echo 'Stopped, nothing changed: <account> has a crontab. Press Try again.' >&2; false; }; } && sudo systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-<id>-*.timer' | awk '{print $1}') && sudo cp -p /etc/cron.allow /etc/cron.allow.celikpanel-before && echo <account> | sudo tee -a /etc/cron.allow >/dev/null && { sudo crontab -u <account> /etc/celikpanel-cron/<id>.crontab; ok=$?; sudo mv /etc/cron.allow.celikpanel-before /etc/cron.allow && [ $ok = 0 ]; }
```

In order: (1) a backup of `/etc/cron.allow` that an earlier, stopped run left is put back;
(2) **it stops, changing nothing, when the account has a crontab** - the spool is looked at
directly (`/var/spool/cron/crontabs/<account>` on Debian and Ubuntu, `/var/spool/cron/<account>`
with cronie), as `crontab -l` is refused for an account `/etc/cron.allow` leaves out, also for
root - so it never overwrites one; (3) the site's timers are switched off, listed by name
(`systemctl disable` takes no pattern: `'celikcron-<id>-*.timer'` is refused as a unit name;
with none listed this step fails and nothing else runs); (4) `/etc/cron.allow` is copied aside;
(5) the account is let in for the one `crontab` command (`crontab -u` is refused for an account
`/etc/cron.allow` leaves out, also for root); (6) the task file **as it is now** is installed as
the account's crontab, and the copy is put back byte for byte - also when the install failed -
so CelikPanel's file stays CelikPanel's and no other account is let in. A task switched off on
the server comes back as the task file has it (for the record of an undo too: the steps do not
read which timers were off). Then the administrator presses **Try again**: it finds the
crontab back, removes the timer files left and the record, and lets the account use cron
again, so sites can be moved again.

**Run again after a stop** (measured): stopped after (1) or (2) - nothing was changed, the
second run starts afresh (or stops again at (2) while a crontab is there); after (3) - the
timers are already off, the second run switches them off again and goes on to the end; after
(4) - (1) puts the unchanged copy back and the run goes on to the end; after (5) - (1) takes the
account out again and the run goes on to the end (the account is listed once, the file ends as
CelikPanel's); after the install but before the copy went back - (1) puts `/etc/cron.allow` back
and (2) stops because the crontab is there: the administrator presses Try again, which finds it
back. Every run that reaches its end leaves `/etc/cron.allow` byte for byte as it was and no
copy beside it. **Without `/etc/cron.allow`** (a host where `/etc/cron.deny` names the account)
the command stops at (4) after the timers are off, and again at each later run; the server
owner then removes the account's name from `/etc/cron.deny` and runs `sudo crontab -u <account>
/etc/celikpanel-cron/<id>.crontab`, and the administrator presses Try again. Pressed without a
crontab installed, Try again settles the record by its step instead (from `crontab_emptied` on:
the timers switched on again, the table above).

**The Agent's start is bounded.** A tasks record is settled at the Agent's start with at most
**four systemd calls** under one 15-second bound (the slice's reading, the timers on or off,
one reload), counted by a test on every step and failure; it never starts, stops or waits for
a task or a timer. A call that does not answer in time is left to systemd and counts as not
known; nothing further is sent. A record the start cannot settle stays for Try again. **A call
given up at the bound is not cancelled** (alpha.150 fix round, as it is): systemd keeps it and
may still carry it out after the record was written - an enable given up there can switch timers
on later, and could overtake the switch-off of a later Try again. This needs systemd to hang past
the bound; the record then says that it is not known how the tasks run, and a Try again once
systemd answers again reads the state anew. The same bound covers only systemd's calls: the
`crontab` command and the writes of `cron.allow` are not cut by it.

**The panel's update.** The tasks' move, its undo and "Rebuild the task file" (§9.4) take the
same claim as the programs' move: each is refused while the panel's update is queued or
running (`update_running`), while an update record cannot be read (`update_unreadable`) or
while another move runs (`another_move`), with the same words, and the update is refused
while one of them runs (§8.4).

**One press, one deadline.** A press moves the site's scheduled tasks first, then its
programs, within the move's 11 minutes: the programs part gets what the tasks part left of
them, never more. With less than a second left it is not started, and the press says that the
time ran out and names each program for the next press. **The tasks part itself is not cut by
that deadline** (alpha.150 fix round): its steps, its systemd calls and its checks of every
written unit (`systemd-analyze verify`, two per task) run to their end. One deadline holds for
what follows the tasks part. When the tasks part alone takes longer than the panel waits for
the press (12 minutes, which only a very large crontab on a slow systemd reaches), the press
answers that the result is not known while the Agent still finishes the tasks part and holds the
move's claim: the page says to read it again before pressing again, and a press meanwhile is
refused as another move running.

**The undo** ("Put the tasks back into cron", administrator): refused before anything
changes when the server owner's own `/etc/cron.allow` leaves the account out (or
`/etc/cron.deny` names it), when the task file was changed by hand (or is missing), or when
the account has a crontab again, and when the state of the site's timers cannot be read (it
would not be known which tasks the server owner switched off). **The undo does not start again
what the server owner switched off (alpha.150 fix round, D-022):** a task whose timer is not
enabled when the undo runs - switched off on the server (`systemctl disable --now`), not one
CelikPanel wrote and could not switch on yet - comes back into the crontab as CelikPanel's
paused line (`# DISABLED: <the line>`) under one fixed comment line, `# CelikPanel: the task
below stays paused, as its timer was switched off on the server.` (fix round 4: never written
twice above a line); the answer names those tasks and their timers, and the Scheduled tasks
screen resumes one. What counts is whether the timer is **enabled**: one that is enabled but
only stopped (`systemctl stop`) comes back as a live cron line (fix round 4, as it is); one
enabled only until the next start (`enable --runtime`) or masked counts as switched off and comes
back paused. The screen never shows that comment as a task's description (a description
above it stays the task's) and shows the task as paused; every save on the screen leaves the
comment out, so it goes once the task is resumed there. A task paused on the
screen is already such a line in the task file and comes back as it is. Otherwise the crontab is
the task file's, byte for byte. The undo is journaled step by step, and the crontab and the
timers are never both on and never both gone:

| Step reached | What it means | At the Agent's start |
|---|---|---|
| undo_planned | the crontab to write (the task file's bytes, the tasks switched off on the server paused) and the timers that are on are recorded | the recorded timers on again, cron closed again ("not tried") |
| timers_disabled | those timers are switched off (a running task is not stopped) | the same |
| cron_opened | CelikPanel's `cron.allow` lets the account in again | finished when the spool holds the recorded crontab ("undone"); otherwise rolled back as above |
| crontab_written | the crontab is written and checked: `crontab -l` gives back exactly the bytes `crontab -l` gave before the move | finished: the timers and files removed ("undone") |

A crontab the site's own code made in the moment cron was open is never removed: the
rollback leaves it, and it is reported as running outside the limits (§9.4). When the
crontab the undo wrote cannot be confirmed, only a spool file that holds exactly what the
undo wrote is taken away again; any other file is left as it is and named in the answer
(second fix round). When the timers cannot be switched on again in a rollback, or the timer
files cannot be removed after the crontab is back, the record stays as above, and the card
and Problems now say how the tasks run (not running, or from cron).

### 9.4 The server owner and the task files (D-022)

- **On the screen.** The administrator changes a moved site's tasks on its Scheduled tasks
  screen; CelikPanel writes the task file and renders only the timers that changed.
- **By hand, in the task file.** CelikPanel keeps the owner's file and applies it: its
  timers follow it at the next save on the screen and after each start of the panel
  (the side doors step re-renders a site whose task file changed since its timers were
  made). CelikPanel's own later changes wait beside it as `<file>.celikpanel-pending`, and
  the screen says so. To give the file back, the owner moves the pending file over it.
- **A task file that goes missing (fix round).** While timers of the site remain, a missing
  task file is a state to report, never one to rebuild from a single line: a change on the
  screen is refused with why (it would remove every other task). The administrator gets one
  action on the screen, "Rebuild the task file", which shows first what the file will hold -
  the copy of what CelikPanel last made the timers from, kept in the Agent's private folder -
  and then writes it. **While the account has a crontab** (an undo by hand half done, or a
  put-back that left a unit file the owner changed), the tasks run from that crontab: the
  rebuild and its preview are refused (it would run every task twice), and the screen says
  that the tasks run from the crontab again and names the timer files left behind, which the
  server owner removes (`sudo systemctl disable --now $(systemctl list-unit-files --no-legend
  'celikcron-<id>-*.timer' | awk '{print $1}') && sudo rm -f <the files named> && sudo
  systemctl daemon-reload`; the screen's answer names the files). The rebuild writes units
  and reloads systemd, so it is refused while the panel's update runs or a move runs, and the
  update while it runs (§9.3); it and its preview are also refused while a move record of the
  site is kept, whose Try again settles the site's tasks first (alpha.150 fix round). A move
  record that cannot be read refuses the rebuild of every site, and the words say a move on
  this server did not finish, not that it is this site's (fix round 4).
- **Natively, in the units.** A changed `celikcron-*` unit is kept (D-031) and the screen
  names it. A timer the owner stopped or disabled (`systemctl disable --now`) stays so: a
  changed task restarts its timer only when it runs, and the undo puts a disabled one's task back
  into cron paused (§9.3).
- **A crontab again.** A crontab that exists for a moved account is read straight from the
  spool (`/var/spool/cron/crontabs/<account>` on Debian and Ubuntu, `/var/spool/cron/<account>`
  with cronie on Arch) at each start and on each read of the screen and the card, and its
  lines are named as running outside the limits (administrator only). It is called the
  server owner's only when the account could not have installed it itself: CelikPanel's own
  `cron.allow`, unchanged, has kept the account out since the move checked it had no crontab,
  and the file was written after that. Otherwise the words say the site's own code can have
  made it, and why (cron let the account in). The undo is refused while it exists.
- **Without CelikPanel.** The timers, services and files are native: they keep running with
  the panel stopped, removed or unlicensed. Once the panel is gone, editing
  `/etc/celikpanel-cron/<id>.crontab` changes **no** timer (only the panel turns that file
  into units); the owner changes a task in its files instead: the time in
  `/etc/systemd/system/celikcron-<id>-<n>.timer`, the command in
  `/etc/celikpanel-cron/<id>.d/<n>.sh`, then `systemctl daemon-reload` (a new task is a new
  timer and service pair, enabled with `systemctl enable --now <timer>`). Every task's run
  calls `/usr/lib/celikpanel-sandbox/sandbox-task-helper` first (the SMTP block, §3), so
  removing CelikPanel's files stops the tasks too: the owner who removes them puts the site
  back into cron first. To put a site back into cron by hand, **timers off first, then the
  crontab** (never both on), each step only if the one before worked, and nothing at all when
  the account already has a crontab (fix round 4: it would be overwritten):
  `sudo test ! -e /var/spool/cron/crontabs/<account> && sudo test ! -e /var/spool/cron/<account>
  && sudo systemctl disable --now $(systemctl list-unit-files --no-legend
  'celikcron-<id>-*.timer' | awk '{print $1}') && echo <account> | sudo tee -a /etc/cron.allow
  >/dev/null && sudo crontab -u <account> /etc/celikpanel-cron/<id>.crontab` (`/etc/cron.allow`
  must let the account in for `crontab -u` to work, also for root, and keeps it in afterwards,
  as the panel no longer closes cron; the first six `#|` lines and the header line are
  CelikPanel's comments; without `/etc/cron.allow` the owner removes the account from
  `/etc/cron.deny` instead), then remove the units, the folder and the task file, and
  `systemctl daemon-reload`.
- **What stays after the panel is removed.** The timers of moved sites, held ones included
  (they stay held while the site's limits file is missing, §9.2); the SMTP block with its
  submission exception as last decided (§3); `/etc/cron.allow`, which keeps moved accounts
  out and any account created later as well until the owner adds it or deletes the file;
  the lingering rule; and the masks of the site accounts' service managers (§9.5).
- **Going back to alpha.149 (version transition).** The schema does not change (layout 81).
  - **An update that fails returns by itself before this release's Agent has started** (§8.8):
    the update installs no task unit, task file, `/etc/cron.allow`, lingering rule, mask,
    task helper or record of this section (the Agent writes each of them later), so the return
    finds none of them.
  - **A completed update is not taken back** (§8.8): the panel offers no way back, the
    release's `rollback.sh` refuses a completed update, and the recovery tool only finishes an
    interrupted one. What follows is what alpha.149 would do if it were nevertheless brought
    back onto a server where this release moved sites' tasks.
  - **The tasks keep running as timers, inside the sites' limits.** alpha.149 knows no
    `celikcron-*` unit, `/etc/celikpanel-cron` file or tasks record, and neither removes nor
    changes them. A task's service in its own site's slice counts for alpha.149 as naming that
    slice, so the slice file and its limits stay. The task units call their own helper,
    `/usr/lib/celikpanel-sandbox/sandbox-task-helper`, which alpha.149 neither manages nor
    removes, so they start under the SMTP block.
  - **The SMTP block and its guard.** At its first program write alpha.149 writes its own SMTP
    block file and guard back: the submission exception goes (tasks cannot send mail through
    this server), and its guard, which still re-adds tasks and what they left running after an
    `nft flush ruleset`, skips a task cgroup whose processes it cannot read.
  - **Scheduled tasks screen.** `/etc/cron.allow` (CelikPanel's file, which alpha.149 neither
    writes nor removes) keeps moved accounts out, and any account created later: alpha.149's
    screen says that the crontab could not be read because `/etc/cron.allow` does not list the
    account, does not show the timers, and cannot add a task for those accounts. Editing
    `/etc/celikpanel-cron/<id>.crontab` changes no timer any more.
  - **The lingering rule** (`/etc/polkit-1/rules.d/49-celikpanel-linger.rules`) and **the masks
    of the site accounts' service managers** stay; alpha.149 does not know them.
  - **The undo's comment above a task it put back paused** (§9.3) is an ordinary comment for
    alpha.149: its Scheduled tasks screen shows it, in English, as that paused task's description
    (a description written above it is no longer shown), and keeps it when the task is resumed
    there. The task itself stays paused and resumes as before; the server owner deletes the
    comment line by hand.
  - **A tasks record kept at that moment** is read by alpha.149's start as a move without
    programs: it counts it as settled and deletes it. The crontab is not lost - from
    `source_written` on the task file holds it, and before that the spool still did - but
    nothing says any more that the site's move was not finished; a move stopped after the
    crontab was emptied leaves that site's tasks not running.
  - **By hand, per moved site** (as root; `<id>` is the site's id, `<account>` its account):
    timers off first, then the crontab, never both on, each step only if the one before
    worked, and nothing at all when the account already has a crontab: `sudo test ! -e
    /var/spool/cron/crontabs/<account> && sudo test ! -e /var/spool/cron/<account> && sudo
    systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-<id>-*.timer' |
    awk '{print $1}') && echo <account> | sudo tee -a /etc/cron.allow >/dev/null && sudo crontab
    -u <account> /etc/celikpanel-cron/<id>.crontab` (its first
    header line and six `#|` lines are CelikPanel's comments; without `/etc/cron.allow`, the
    owner removes the account from `/etc/cron.deny` instead). The recommended order is still to put every site's
    tasks back into cron on this release first ("Put the tasks back into cron" on each site's
    Overview).

### 9.5 The last doors for a site's own jobs

**`/etc/cron.allow`** keeps moved sites' accounts from installing their own crontab. It is
written by R1's `at.allow` rules (`cmd/agent/side_doors_cron.go`): only when absent and only
once a site is moved; root first, then every account that may use crontab today (not named
in `/etc/cron.deny`), minus the moved sites' accounts, CelikPanel's tool accounts and every
login that shares a site's id (crontab decides by the first passwd name of the caller's
uid). A site not yet moved stays allowed, and R1's marking of lines not made in CelikPanel
keeps working for it. CelikPanel's file is brought up to date at each start and after each
move or undo while it is as CelikPanel wrote it; the owner's file is kept byte for byte, and
a moved account it names is a Problems now line; the owner's deletion of CelikPanel's file
stands (Problems now, with "Close it again"); accounts from a directory service: nothing is
written, and it is said. A crontab an account already has keeps running after the file
leaves it out (F1): the move empties it itself. An account the owner creates later may use
`crontab` from the next Panel start (CelikPanel's file is brought up to date then, as
`at.allow` is); until then, or without the panel, the owner adds its name to the file or
deletes the file - its header says so.

**Lingering** is closed where polkitd is installed: `/etc/polkit-1/rules.d/49-celikpanel-linger.rules`
answers "an administrator's password" to `org.freedesktop.login1.set-self-linger` unless
the caller is in an administrator group the system has (of `sudo`, `admin` and `wheel`:
`sudo` on Debian 13 and Ubuntu 24.04), under CelikPanel's render header with its own words
on what it does and how to undo it (delete the file; polkit reads the change at once). Only
those three group names count, as `/etc/group` names them: an owner whose administrators
come from another group (an LDAP or SSSD group, for example) runs `sudo loginctl
enable-linger <name>` for them, or adds that group to the rule (the file is then the
owner's, and CelikPanel keeps it); the rule's header says so. The
owner's change is kept; the owner's deletion stands (Problems now, "Close it again").
Without polkitd, systemd-logind already lets only root switch lingering on (F3). On Ubuntu
22.04 (polkit 0.105) the rules file is not read: the step reports lingering as not closed.
**Accounts that already linger keep lingering**: Problems now names them with the command
that ends it (`loginctl disable-linger <account>`); nothing is removed by itself.

**A site account's own service manager at an SFTP login is closed (second fix round).**
Measured on Debian 13 and Ubuntu 24.04: an SFTP login of a site account goes through sshd
with `UsePAM yes` and `pam_systemd`, which starts `user@<uid>.service` in `user.slice`,
outside the site's slice and the SMTP block; the site's own code could then run units of its
own there (planted under `~/.config/systemd/user`, or `systemd-run --user` while a session is
open) for as long as a session stays open. With `user@<uid>.service` masked, the SFTP login
still works, `/run/user/<uid>` is still made, and no manager starts. So every site account's
uid gets a mask, `/etc/systemd/system/user@<uid>.service` linked to `/dev/null`: CelikPanel's
own link, recorded like the other files of this section, written when a site is created and by
the step after each Panel start for existing sites (followed by `systemctl daemon-reload`),
and removed when the uid no longer belongs to a site account (its site was deleted). A
session that is open when the mask is written keeps its manager until it ends. Never for root or for a uid an
administrator account shares (a member of `sudo`, `admin` or `wheel`). A file or link of that
name that is not CelikPanel's is the server owner's: kept, and named in Problems now with the
command that closes it; the owner's removal of CelikPanel's link stands and is named the same
way; a mask that could not be written is named too. FTP as the panel configures it starts no
manager (measured). **After the panel is removed the masks stay**; the owner removes one with
`systemctl unmask user@<uid>.service`.

**What a site account can still keep running after this step**: nothing new of its own
once its site is moved and polkitd (or none) is as above — no crontab, no `at` job (R1), no
lingering user manager, no user manager at an SFTP login. Its PHP (until R3) and its
programs run where CelikPanel starts them; a user manager that already lingered keeps running
until the owner ends it, and a process a finished task left behind keeps running inside the
site's limits until the administrator ends it (§9.2).
