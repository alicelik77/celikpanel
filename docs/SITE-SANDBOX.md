# Site sandbox

*alpha.119 · In source and component-tested. Smoke-run once on WSL Debian 13
(systemd 257). The escape matrix on the five supported systems is still
pending in the lab.*

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
it runs until the administrator moves the site). When a program's unit is
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
the helper's `egress-add`. Between a flush and the guard's next run, the block is absent for
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
| the site's PHP-FPM pool, its scheduled tasks | | not yet: PHP and tasks join the slice in later steps |
| `celikpanel-sandbox@…`, `celikpanel-ftp@…` | root's bind units | no: they run no customer code |

**What is not bounded yet after this release.** Say it plainly: a site's programs
that run outside its slice (every program written before this release, until the
administrator moves the site, and such a program again after each restart of the
server until it is moved), the site's PHP (its PHP-FPM pool), its scheduled tasks,
and user managers a site account may keep running ("lingering", Ubuntu with
polkitd) are not bounded by the site's limits. PHP is R3; scheduled tasks and
lingering user managers are R2b.

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
limits and offers the move. Where a unit names the slice or runs in it and the file is gone
(the owner removed it), the card says the file is missing and the next change of limits
writes it again; the site counts in the "outside" line, and the administrator's card offers
the move, which writes it. A file CelikPanel wrote that the owner deleted to give it back is
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
  whether it runs;
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
as before, and stays outside until the administrator's move.** A program whose file
was written while it did not run, or that was moved, names its site's slice and
starts inside it.

### 8.4 The administrator's move

The site's Overview shows the administrator where each program runs and one action,
`POST /api/v1/domains/{id}/site-limits/move {request_id}` (`Agent.MoveSiteSliceV1`).
One move at a time across the panel; a repeated `request_id` answers the first
answer; it is audited as `site.limits_move`.

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
  continues with them.
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
compared with the recorded pre-move bytes and with the text the move intended:

| The unit file holds | At the Agent's next start |
|---|---|
| the pre-move bytes, step "planned" or "writing" | not touched ("not tried") |
| the pre-move bytes, a later step | already put back; restarted if it ran before and does not run in the system slice now ("put back") |
| the move's text, stopped unit | kept: its file names the slice, it starts there ("rewritten") |
| the move's text, running unit, step "moved" (it ran and answered in the slice) | kept ("moved") |
| the move's text, running unit, any other step | pre-move bytes back, daemon-reload, and a restart unless it still runs in the system slice ("put back") |
| anything else, or no file | left as it is ("changed") |

Unit files are written and put back through the writer every other managed file uses
(the text synced before the rename, the folder after it), so a power loss right after a
write never leaves an empty unit.

The slice file: when every unit is settled, no unit names the slice after this and the file
still holds exactly the bytes the move wrote, the recorded original comes back (its earlier
bytes and ledger digest, or no file where there was none); otherwise it stays.

**A put-back that does not complete keeps the journal** (third fix round). That is: a
pre-move file that cannot be written; a unit whose place systemd cannot tell (a failed
status read is "not known": nothing is restarted on it, never "not running, so restart it");
a `daemon-reload` that fails (systemd would still hold the moved text, so no unit is
restarted); a restart that fails (the program is stopped) or is still starting at its start
time; at the Agent's start, a put-back restart that does not fit the start's own time
(alpha.148 fix round: the whole recovery at the Agent's start keeps within 4 seconds,
because it runs before the Agent's socket opens and an update waits about 12 seconds for
that socket; a restart is sent only while 1 second is left and waits at most what is left,
"still starting" when it runs on, and one that does not fit is not sent at this start, "not
restarted yet"). While the journal is kept, no site is moved: a new move is refused with the
site whose move is unfinished. The site's card (administrator) says the last move did not
finish and, per program left, its state (stopped and did not start again; place not known,
not restarted; still starting; not restarted yet), `journalctl -u <unit>` for why, and the
by-hand step (start it on its page or with `systemctl start <unit>`); its **Try again**
button (`POST …/site-limits/move {request_id, recover: true}`, audited as `site.limits_move`
with `recover`) runs the put-back now and moves nothing new. A program started by hand
meanwhile is found running in the system slice and the journal goes; there is no button that
forgets the journal while a program stays stopped. Problems now has ONE administrator line
while the journal is kept (`site_move_kept`, opening the site). The Agent's next start and
the next move's attempt try the same put-back. Once it completes, the card says the last
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
  run where it ran (active, in the system slice) and listen again on every port it
  listened on before, within its start time, as the move's own step checks the slice. A
  program whose restart succeeded but that does not run there (a `Type=simple` unit's
  restart succeeds as soon as its process starts) is **not** "put back": the journal is
  kept with it as stopped, still starting or not answering. At the Agent's start the same
  check runs within the start's 4 seconds; what cannot be seen in that time is kept as
  "not known yet" (neither a failure nor a success); the journal now also records each
  program's ports (an earlier journal has none and is checked without them). The card and
  Problems now say that the program is not running now, that the administrator acts, the
  next step (its own log, `sudo journalctl -u <unit>`, then **Try again** or start it) and
  that nothing else was changed.
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
  The recorded reason is a code (`kernel_differs`, `unchecked`, `write_failed`, …).
- **A reached process limit on Ubuntu 24.04** (D3). Without `pids.events.local` the slice's
  own `pids.peak` reaching its `pids.max` since the last reading is also "reached its
  process limit" (seen at that reading; the kernel does not count how often), because a
  unit's counters go with its cgroup when the program restarts.
- **The reseller's bound** (CHECK 2). Where neither the server nor the reseller sets a
  ceiling, the server's default bounds a reseller's customer; the card names it as that
  default ("which also bounds the reseller"), not as a limit the administrator set.
- **No update during a move** (D5). While a site's programs are being moved, the panel's own
  update is not started: the panel and the Agent both refuse it, and the update page says
  to wait for the move's window and press update again. The Agent's start writes one
  journal line per startup step with its time (`agent start: <step> took …`).

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
