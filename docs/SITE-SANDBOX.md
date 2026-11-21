# Site sandbox

*alpha.119 · In source and component-tested. Smoke-run once on WSL Debian 13
(systemd 257). The escape matrix on the five supported systems is still
pending in the lab.*

*S4 R2b (22 November 2026) · A site's scheduled tasks run as timers in its slice
(§9); the SMTP block lets its members reach this server's own submission service on
loopback (§3).*

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
- The output chain rejects TCP to port 25 on any address, 127.0.0.1 and ::1
  included, and to ports 465 and 587 on any address but loopback, for members
  of the set. It counts the rejections in the named counter `smtp_rejects`.
- **Submission on loopback is allowed (S4 R2b, 22 November 2026; it applies to
  every unit in the set, a site's programs and its scheduled tasks alike).**
  Two rules before the reject accept TCP to 127.0.0.0/8 and ::1 on ports 465
  and 587: this server's own submission services, which take only a client
  that logs in with a mailbox's name and password
  (`smtpd_client_restrictions=permit_sasl_authenticated,reject`,
  `cmd/agent/mail_submission_rpc.go`; a submission entry the server owner made
  in `master.cf` is the owner's). It is the supported way for a site to send
  mail. Port 25 stays refused everywhere: Postfix's relay list has not
  allowed this server's own addresses without a password since alpha.125, and
  the block keeps it closed anyway. A changed rule set is loaded into the
  running table as soon as the Agent writes the file.
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
for the cgroup the unit runs in, read from systemd (with an empty
environment), and `nft delete element …` after the stop. This fails closed. If
nft cannot load the table, or the unit's cgroup is neither
`/system.slice/<unit>` nor its site's `/celiksite_<id>.slice/<unit>` (§8; for
example, an owner's drop-in moved it to another slice), the unit does not
start.

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

*21 November 2026 · In source and component-tested; the facts below were measured on
throwaway Debian 13 and Ubuntu 24.04 machines; the panel's lab round on both is open
(steps in the pull request).*

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
slice (every program written before R2a, until the administrator moves the site, and
such a program again after each restart of the server until it is moved), the site's
PHP (its PHP-FPM pool; R3), the crontab of a site not yet moved (§9), and a user manager
a site account already kept running before R2b ("lingering", §9.5) are not bounded by
the site's limits.

**The name.** A dash in a slice's name is nesting for systemd (`celiksite-7.slice`
would be a child of `celiksite.slice`). Without one the slice sits below the root,
and a unit in it is `/celiksite_7.slice/<unit>`: depth two, like
`/system.slice/<unit>`. The SMTP block's set (`typeof socket cgroupv2 level 2`, §3)
therefore matches a unit in either place with no change to the table. The helper's
`egress-add` adds the element of the cgroup the unit really runs in (its
`ControlGroup=`), accepts only the system slice or a site's slice, and refuses any
other place, so a unit an owner's drop-in moved elsewhere does not start (as
before). The guard timer re-adds units in either place.

**Limits.** The slice gets the site's effective `site.memory_mb`, `site.tasks` and
`site.cpu_percent` (docs/SITE-LIMITS.md §4a). systemd applies the most stringent
limit of a unit and its parent slices ("EffectiveMemoryMax= … is the most
stringent limit of the unit and parent slices", systemd.resource-control(5),
systemd 257 and 255). A unit keeps its own limits in its file (a Node.js
application 1536 MB, n8n 1536 MB); the slice is the outer bound. Without any level,
the slice gets 512 tasks, 200 % and, for memory, **the smaller of 3072 MB and 75 %
of the host's memory** (`MemTotal` in `/proc/meminfo`, rounded down to a whole MB,
never under 64 MB), so one site never takes the whole host by default.

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
slice. The files about to be made are recorded first in the Agent's private folder;
an Agent stop between the set and the removal leaves the same effective values in
`/run` (harmless, gone at the next reboot) and the Agent's next start removes them.
**A lowered limit therefore applies at once on both systems**, which is why it asks
first (§8.5).

**A missing file.** A slice without a file has no limits (F4). When a unit names the
slice or runs in it, the next change of limits writes an absent file again, and so
does the administrator's move; the site's card says until then that the limits do
not apply because the file is missing, and Problems now counts nothing as limited.
A file the owner deleted to give it back to CelikPanel is written again the same way.

**The card for the administrator** names the owner's drop-ins by file name with "the
server owner's additions apply", and an edited file as kept.

### 8.3 Where a unit goes when it is written

A changed `Slice=` reaches a running unit only at its next start (F1). So:

- a unit that does not run (a new one, a stopped one, every install or build step)
  is written into its site's slice;
- a running unit keeps the slice it runs in on every ordinary write (a settings
  change, an update, a restart by the customer);
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
  change of the program takes): the file must still be the text the move planned
  from (otherwise it is a change made meanwhile: kept, "changed"), CelikPanel's
  unchanged text gets `Slice=celiksite_<id>.slice` (re-sealed), systemd is reloaded,
  and a running unit restarts once and must run in the slice and listen again on
  every port it listened on before **within its start time**: the unit's own
  `TimeoutStartUSec` (these units set none, so systemd's 90 seconds), never under 20
  or over 300 seconds. The whole move stays within 10 minutes: a program whose wait
  no longer fits is not started, and the answer says to press again.
- If a unit fails, it and every unit of the site moved before it are put back to
  their pre-move bytes, but only over the move's own text: a file changed or removed
  meanwhile is left as it is. Other sites are never touched.

**The journal** (`site-slice-move.json` in the Agent's private folder): the
pre-move bytes and mode of every unit file the move will change, whether the slice
file existed, and the step each unit reached. At the Agent's start:

| Step reached | At the Agent's next start |
|---|---|
| planned (nothing written) | not touched ("not tried") |
| written, stopped unit | kept: its file names the slice, it starts there ("rewritten") |
| written, running unit (not restarted) | pre-move bytes back, daemon-reload; it never left ("put back") |
| restarting (outcome unknown) | pre-move bytes back, daemon-reload, restart ("put back") |
| moved (it ran and answered in the slice) | kept ("moved") |
| put back, or changed meanwhile | nothing more |

A file that no longer holds what the move wrote is left as it is ("changed"). A
slice file the move created is removed again when no unit names it. What the Agent
did is shown on the site's card for the administrator ("the last move was
interrupted … each program was finished or put back") until the site's next move.

### 8.5 Lowering a limit below what a site uses

Lowering `site.memory_mb` below what a site's programs use now makes the kernel stop
one of them at once (F4); lowering `site.tasks` below what they hold stops new
processes. The plan form, a reseller plan and an account's own limits therefore
show, in the change's preview before it is confirmed (the panel's own confirmation),
the sites whose programs use more now than the new value and that the system stops
them at once when the change is saved. Nothing else about applying limits changes.
The server's own limits screen does not list them.

### 8.6 What people read when a limit is hit (D-024)

The Agent reads the slice's counters with the site's status read and, for all sites
that run programs, at most every five minutes in the background (in pages of 1000
sites; no reading is refused for the number of sites; the ledger is read once per
reading). The panel records when a counter rose (`panel_settings`
`site_limit_events_v1`; the first reading of a site is its baseline; it is written
only when something was reached, a counter started again or a site is gone).

- "The site's own limit" (memory, processes) is only what §8.1 proves. The card then
  says what happened and when, what the plan allows and who can raise it: "your
  provider" for a customer, "you, in the customer's plan within your own limits" for
  a reseller, the plan or reseller by name for the administrator. A site that
  reached its own memory or process limit at two or more readings in a day is a line
  in Problems now.
- A program stopped for lack of memory without the site's limit (its own smaller
  limit, or a shortage of the whole server), or refused a process at its own limit,
  is said as only that ("a program of this site was stopped for lack of memory"):
  the plan is never blamed and no Problems now line follows.
- Where the limits do not apply (every program outside them, no slice file, a host
  that cannot enforce them) the card says so instead of "stayed within their
  limits".

**Sites still to move.** Problems now has one line, for the administrator only:
the sites whose own programs run, or would start, outside their limits and that the
move can take (their number and the first five names). It opens the first site, is
a warning (a to-do), and goes away by itself when no site is left (`panel_settings`
`site_limit_outside_v1`, written only when the list changes).

### 8.7 A deleted site's slice

When a site is deleted, CelikPanel's unchanged slice file goes and the ledger
forgets it; a file the owner changed, and the owner's drop-in folder, are moved
aside under names systemd does not load (`<file>.celikpanel-aside-<UTC time>`) and
named in the audit log (`site.limits_file_aside`). A slice file found for a new
site's id that the Agent's ledger does not know (a site of an earlier installation
with the same id) is treated the same way before the new one is written.

**Owner independence (D-022).** The slices are native units. With the panel
stopped, removed or unlicensed, they and their limits keep working. To lift or
change a site's limits without the panel, the owner adds a drop-in
(`/etc/systemd/system/celiksite_<id>.slice.d/owner.conf` with, for example,
`[Slice]` and `MemoryMax=infinity`) and runs `systemctl daemon-reload` (on systemd
257 the new value reaches running programs when one of them starts, or with
`systemctl set-property --runtime`); CelikPanel never writes or removes it.

## 9. A site's scheduled tasks inside its limits (S4 R2b)

*22 November 2026 · In source and component-tested; the facts below were measured on
throwaway Debian 13 and Ubuntu 24.04 machines; the panel's lab round on both is open
(steps in the pull request).*

A moved site's scheduled tasks no longer run through cron. Each enabled task is a
systemd timer and service pair, `celikcron-<site id>-<n>.timer` and `.service`, whose
service runs in the site's slice (§8) under the network rules of the site's other
programs. **New sites are moved from the start. An existing site keeps cron until the
administrator moves it** with the same "Move into the limits" action as its programs
(§8.4, §9.3), or with "Move all" for every site still outside.

**What still runs outside the limits after this step.** Say it plainly: the site's PHP
(its PHP-FPM pool; R3), the programs and the crontab of a site not yet moved, a program
written before R2a again after each restart of the server until it is moved (§8.3), a
crontab the server owner makes again for a moved site's account (§9.4), and programs of a
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
| `/etc/celikpanel-cron/<site id>.crontab` | the site's tasks in crontab form, the one source (0640 root:celikpanel; CelikPanel's render header and a short `#|` intro) |
| `/etc/celikpanel-cron/<site id>.d/<n>.sh`, `<n>.in` | task n's command exactly as cron hands it to the shell, and its standard input (`%`); 0640 root with the site's group, in a 0750 folder of that group |
| `/etc/celikpanel-cron/<site id>.d/env` | the crontab's `NAME=value` settings (an environment file systemd reads as root) |
| `/etc/systemd/system/celikcron-<site id>-<n>.service`, `.timer` | the units, under the D-031 rule |

The command and its input never stand in a unit's own settings: systemd tells every local
account a unit's settings (`systemctl show`), while a crontab is readable by its account
only. A task keeps its number while its text stays the same (its service names it with a
`# celikpanel-task:` line), so its last run stays its own.

**The service** (`Type=oneshot`): `User=` and `Group=` the site's account, `Slice=` the
site's slice, `WorkingDirectory=` its home, `UMask=0002`, `Environment=PATH=/usr/bin:/bin
SHELL=/bin/sh`, then `EnvironmentFile=-/etc/environment`, `-/etc/default/locale` and the
site's own settings file (a later file wins, as the crontab's settings outrank PAM's in
cron), `StandardInput=file:<n>.in` where the task has input, `ExecStart=<the crontab's
SHELL> <n>.sh`, the SMTP block's helper lines (§3), `RestrictAddressFamilies=AF_INET
AF_INET6 AF_UNIX AF_NETLINK` and, where systemd enforces it, `IPAddressDeny=` of link-local
and metadata addresses. It does **not** get the programs' file system sandbox (§1): a cron
job never had one, and it would stop tasks that write the site's files. PHP and the file
system view join in R3.

**The timer**: `OnCalendar=` translated from the line (`internal/sitecron`): every value
list as systemd's list, cron's OR of the two day fields (neither starts with `*`) as two
`OnCalendar=` lines, `7` and names for the weekday, month names, steps on ranges, and the
special strings; `@reboot` becomes `OnBootSec=1min`, **enabled but not started** (it runs
once after each start of the server, as cron runs `@reboot` lines when it starts);
`AccuracySec=1s`; `Persistent=false` (cron catches nothing up). A line the translation does
not know (`L`, `W`, `#`, `?`, a value out of range, a backwards range, a step of 0) is not
translated.

**A cron job and a timer task: what differs.**

| | cron job | timer task |
|---|---|---|
| account, groups, home, working folder, `LANG`, shell | the same | the same |
| `PATH`, `SHELL`, umask | as cron sets them | made the same (above) |
| `$0` | `/bin/sh` | the task's file (`<n>.sh`) |
| extra environment | — | `USER`, `INVOCATION_ID`, `JOURNAL_STREAM`, `MEMORY_PRESSURE_*`, `SYSTEMD_EXEC_PID`, `TRIGGER_*` |
| login id (audit) | set by PAM | not set |
| standard input without `%` | an empty pipe | `/dev/null` (both end at once) |
| output | not mailed (`MAILTO=""` since alpha.125) | kept in the journal with the run |
| open files hard limit (Ubuntu) | 1048576 | 524288 |
| still running when its time comes | a second copy starts | not started again; starts once more right after it ends |
| limits, outgoing mail | none; mail to every port | the site's slice; §3's rule |

**What people read.** The Scheduled tasks screen says, for a moved site, the two halves of
a long run in one sentence and the differences from cron a site may notice (output kept,
not mailed; no login session; `$0`), and for each task its last run (when, how long, exit status
and systemd's word for it, the last ten lines it wrote, from its own journal entries), when
it runs next, and a task whose timer the server owner turned off, whose unit was changed on
the server, that is not scheduled yet ("save it again"), or whose line does not run (with
why). After a restart of the server, a task that has not run since says "No run since the
server started" (systemd forgets the last run then; the journal keeps it). The customer sees
only their own site's tasks.

### 9.3 Moving an existing site's crontab, and the undo

The administrator's "Move into the limits" on the site's Overview reads a **preview** first
(what becomes what, the settings and paused tasks that come along, and a task that sends
mail: after the move it can send only through this server's own submission service on
loopback, §3). A line that cannot be translated, and a line not made in CelikPanel that the
administrator has not kept (S4 R1), keep the **whole** crontab in cron and are named;
nothing is half-moved. The move holds the same locks as the programs' move (one move at a
time; the site's lock without waiting, so a site being deleted is not moved) and the lock
of every change of the site's tasks; it restarts nothing.

**The journal** (the programs' move journal, version 2 with a `tasks` part): the
crontab's exact bytes and each step, written before the next step starts.

| Step reached | What it means | At the Agent's next start |
|---|---|---|
| planned | the crontab's bytes are recorded; nothing written | nothing ("not tried") |
| source_written | the task file holds the crontab | put back: every timer and file of the move removed; the crontab written again only if it is gone ("put back") |
| units_written | every timer and service written and checked by systemd, none started | the same |
| crontab_emptied | `crontab -u <account> -r` | finished: the timers enabled ("moved"); if that fails, put back |
| enabled | the timers enabled and started | finished ("moved") |

The crontab must still be the one recorded when it is emptied: a change made meanwhile is
never lost (the move is put back and says so). Only then is `/etc/cron.allow` brought up to
date (§9.5). The administrator's card shows an interrupted move's outcome.

**The undo** ("Put the tasks back into cron", administrator): refused before anything
changes when the server owner's own `/etc/cron.allow` leaves the account out (or
`/etc/cron.deny` names it), when the task file was changed by hand, or when the account has
a crontab again. Otherwise CelikPanel's `cron.allow` lets the account use cron again, the
crontab is written (byte for byte the crontab that was moved, when the tasks did not change
since; otherwise the tasks as they are now, with `MAILTO=""` where it is missing), and then
the timers and files go. Its journal steps: `undo_planned` (nothing changed; at the start
nothing) and `crontab_written` (at the start the timers and files are removed).

### 9.4 The server owner and the task files (D-022)

- **On the screen.** The administrator changes a moved site's tasks on its Scheduled tasks
  screen; CelikPanel writes the task file and renders only the timers that changed.
- **By hand, in the task file.** CelikPanel keeps the owner's file and applies it: its
  timers follow it at the next save on the screen and after each start of the panel
  (the side doors step re-renders a site whose task file changed since its timers were
  made). CelikPanel's own later changes wait beside it as `<file>.celikpanel-pending`, and
  the screen says so. To give the file back, the owner moves the pending file over it.
- **Natively, in the units.** A changed `celikcron-*` unit is kept (D-031) and the screen
  names it. A timer the owner stopped or disabled (`systemctl disable --now`) stays so: a
  changed task restarts its timer only when it runs.
- **A crontab again.** A crontab the owner makes for a moved account (after changing
  `cron.allow`) is read straight from the spool at each start and on the screen's read,
  reported as the owner's, and its lines are named as running outside the limits
  (administrator only).
- **Without CelikPanel.** The timers, services and files are native: they keep running with
  the panel stopped, removed or unlicensed. To put a site back into cron by hand:
  `crontab -u <account> /etc/celikpanel-cron/<id>.crontab` (after `/etc/cron.allow` allows
  it), then `systemctl disable --now 'celikcron-<id>-*.timer'` and remove the units and the
  folder.

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
leaves it out (F1): the move empties it itself.

**Lingering** is closed where polkitd is installed: `/etc/polkit-1/rules.d/49-celikpanel-linger.rules`
answers "an administrator's password" to `org.freedesktop.login1.set-self-linger` unless
the caller is in an administrator group the system has (of `sudo`, `admin` and `wheel`:
`sudo` on Debian 13 and Ubuntu 24.04), under CelikPanel's render header with its own words
on what it does and how to undo it (delete the file; polkit reads the change at once). The
owner's change is kept; the owner's deletion stands (Problems now, "Close it again").
Without polkitd, systemd-logind already lets only root switch lingering on (F3). On Ubuntu
22.04 (polkit 0.105) the rules file is not read: the step reports lingering as not closed.
**Accounts that already linger keep lingering**: Problems now names them with the command
that ends it (`loginctl disable-linger <account>`); nothing is removed by itself.

**What a site account can still keep running after this step**: nothing new of its own
once its site is moved and polkitd (or none) is as above — no crontab, no `at` job (R1), no
lingering user manager. Its PHP (until R3) and its programs run where CelikPanel starts
them; a user manager that already lingered keeps running until the owner ends it.
