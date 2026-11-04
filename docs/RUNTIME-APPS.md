# Uploaded programs (runtime apps)

*alpha.119 · In source and component-tested. The lab round on the five
supported systems is pending.*

Turkish: [RUNTIME-APPS.tr.md](RUNTIME-APPS.tr.md) · Sandbox:
[SITE-SANDBOX.md](SITE-SANDBOX.md) · Rights:
[PLANS-AND-RIGHTS.md §4.8](PLANS-AND-RIGHTS.md)

A customer can run their own compiled program (Go, Rust, or any other program
built for this server's processor) as their site. The program runs as the
site's own account, inside the site sandbox, on one port that only the server
itself can reach. Visitors reach it only over HTTPS, through the site's nginx.

CelikPanel treats every uploaded program as arbitrary code. An uploaded Go
binary could start Python. The security boundary is therefore the sandbox,
not a list of allowed languages.

This document describes what the code does in alpha.119. It does not claim
that the feature was tried on every supported system. The lab round is still
to come (§17).

---

## 1. Kinds

| Kind | State in alpha.119 |
|---|---|
| `program` | Implemented: an uploaded executable. |
| `python` | Known to the database, but not implemented in this release. The Python apps workstream builds it on the same engine, with its own permission (`python_apps`) and its offering per distribution Python (`component:python:<minor>`). A request to create one is refused with `kind_unavailable`. |

A domain can have at most one runtime app.

## 2. Where the files go

```
<site home>/app/
  bin/     the uploaded program (the entry and anything it needs)
  data/    the program's own data; its HOME
  config/  the program's own configuration files
```

`<site home>` is `/var/www/celikpanel/subscriptions/<subscription>/sites/<domain id>`.
The `app` folder sits beside `public_html`, not inside it. Its files belong to
the site's account, count toward the subscription's disk, and nginx never
serves them.

The customer uploads the program into `app/bin` with the panel's file manager
or over SFTP. An SFTP session starts in the site home, so the target is
`app/bin/`. CelikPanel creates `data/` and `config/` itself during a deploy,
as the site's account (`mkdir`, run directly, with no shell). Root never
writes inside the site's folder.

The environment file is **not** in this folder (§5).

## 3. The entry

The entry is the file systemd starts. The customer names it relative to
`app/bin`, for example `server` or `release/api`. A leading `bin/` is
dropped. A name holds letters, digits, `.`, `_` and `-`, in at most four
parts separated by `/`. It may not name a parent folder.

The Agent checks the file before every deploy and on every status read. It
**reads** the file, never runs it. It opens the app folder beneath the site
home, and the entry beneath the app folder, with `openat2`
(`RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS |
RESOLVE_NO_XDEV`, `O_NOFOLLOW`, `O_NONBLOCK`). A link anywhere on the path is
refused. A FIFO planted as the entry cannot stall the Agent.

| Rule | Refusal code | What to do |
|---|---|---|
| The `app` folder is a real folder of the site's account (not a link) | `app_folder_not_owned` | Remove the link, or make the folder again with the file manager. |
| The file exists | `entry_missing` | Upload it to `app/bin/<entry>`. |
| It is not a link | `entry_link` | Upload the program itself. |
| It is a regular file | `entry_not_regular` | Upload the program file, not a folder or a special file. |
| It belongs to the site's account | `entry_not_owned` | Upload it again with the file manager or SFTP. |
| No setuid or setgid bit | `entry_setid` | `chmod u-s,g-s` |
| Not writable by others | `entry_writable` | `chmod o-w` |
| Executable by its owner | `entry_not_executable` | `chmod u+x` (the file manager or an SFTP client can do this) |
| An ELF program for this server's processor (x86-64 or AArch64) | `entry_wrong_arch` | Build it again for this server, for example with `GOARCH=amd64` or `GOARCH=arm64`. |
| Or a `#!` script whose interpreter is inside the app folder | `entry_interpreter` | Upload a compiled program, or ship the interpreter inside `app/`. |
| Anything else | `entry_unknown_format` | Upload the compiled program. |

**Why system interpreters are refused.** A script that starts `/bin/sh` or
`python3` is refused before it starts. The program kind's sandbox lets the
program execute only from its own folder and the shared libraries
(`NoExecPaths=/`, `ExecPaths=<app folder> /usr/lib /usr/lib64 /lib /lib64`).
Such a script could never run, so the panel says so up front instead of
showing a start failure.

## 4. Arguments

Arguments are a list of tokens, never a command line. Each token becomes one
word of `ExecStart`. There is no shell.

- At most 32 tokens, each 1–256 characters.
- Allowed characters: letters, digits and `. _ @ : / + = , -`.
- `{port}` is the only placeholder. It becomes the app's port, for example
  `-addr=127.0.0.1:{port}`.

**Why `$`, `%`, spaces, quotes and backslashes are refused.** systemd expands
`$` and `%` in a unit's command line, and splits or unquotes on spaces,
quotes and backslashes. No single escape is right in every case, so a token
that contains any of them is refused, never escaped. As a final guard, the
rendered unit is checked again: no `$` on any line, the only `%` is
`CPUQuota`'s, and `ExecStart` never starts a shell or carries a privilege
prefix (`+`, `!`, `@`, `-`).

The health path (default `/`) starts with `/` and holds only letters, digits
and `. _ ~ / -`, with no `..` and no query.

## 5. Environment

The customer can set up to 64 values. Each value is one line of printable
text, 1–4096 bytes. A key is upper-case letters, digits and `_`, starts with
a letter, and has at most 64 characters.

**Refused keys:**

| Keys | Why |
|---|---|
| `PATH`, `HOME`, `PORT`, `HOST` | CelikPanel sets them in the unit. The program reads `PORT` and `HOST` to know where to listen. |
| `LD_*` | They would load other code into the program. |
| `PYTHONPATH`, `PYTHONHOME`, `PYTHONSTARTUP` | The same, for Python. |
| `USER`, `LOGNAME`, `SHELL` | Set by systemd for the account. |
| `NOTIFY_SOCKET`, `LISTEN_FDS`, `LISTEN_PID`, `LISTEN_FDNAMES` | systemd's own protocol. |
| `CELIKPANEL_*`, `SYSTEMD_*` | They belong to CelikPanel or systemd. |

**Public and secret values.** A public value is stored by the Panel and shown
again. A secret is write-only: the Panel stores only its key. The value goes
to the server once, into the environment file, and is never shown again. The
audit log names the keys that changed, never a value. To keep a secret
unchanged, send its key without a value. A new secret needs a value of at
least 8 characters, so the log can hide it without hiding ordinary words.

Every save sends the **full** list. A key that is not listed is removed. If
the program is deployed and running, it is restarted so it reads the new
values (`systemctl try-restart`).

**The environment file** is
`/etc/celikpanel-runtime/celikrt-<id>.service.env`. It is root's file, mode
0600, in a root folder with mode 0700. It is deliberately **not** in the
site's folder. systemd reads `EnvironmentFile=` as root, before the sandbox
starts. A file that the site's account controls could be replaced by a link
to a root-only file, such as a key of another service, and systemd would read
it. The unit names the file with a leading `-`, so the program also starts
without it.

The file is rewritten whole on each save from the panel, but never over an
owner's change: when the file holds a key that CelikPanel did not write and
the save does not name, nothing is changed and the save is refused with
`env_edited`, naming the keys. Add them on the Programs tab to keep them, or
remove them from the file (D-022).

**Logs are redacted on the server.** Before the panel shows a log line, the
Agent hides every secret value of the environment file (secret values are at
least 8 characters) and anything that looks like a key, a token, a password,
a bearer token, a bcrypt hash or `user:password@` in a URL.

## 6. Port, network and the unit

**Port.** Each app gets one port from 30000–34999, the lowest free one. Service
apps use 21000–29999, so the two ranges never meet. The unit sets
`PORT=<port>` and `HOST=127.0.0.1`. The program must listen on
`127.0.0.1:$PORT`, or on the `{port}` argument. The sandbox lets it bind only
that port. During a deploy, a listener on any other address is found by the
listen check, and the app is stopped and not published. Later reads show it
on the Programs tab (`listens_elsewhere`).

**Publishing.** After a successful deploy the site's type becomes
`runtime_app`. nginx proxies to `127.0.0.1:<port>` and forces HTTPS. Without
an active certificate the domain never serves the program over plain HTTP.
The site then serves its own folder instead.

**The unit** is `/etc/systemd/system/celikrt-<id>.service`, where `<id>` is
the app's number, shown on the Programs tab. It contains:

- CelikPanel's render header (`# celikpanel-render v2 sha256=…`, D-031),
  comment lines naming the commands that work without CelikPanel, the sandbox
  generation marker, and `# celikpanel-runtime-kind: program`.
- `[Unit]`: `After=`/`Wants=network-online.target`,
  `StartLimitIntervalSec=300`, `StartLimitBurst=5`.
- `[Service]`: `Type=simple`; `User=`/`Group=` the site's account;
  `WorkingDirectory=<app folder>`; `EnvironmentFile=-<env file>`;
  `HOME=<app>/data`, `PATH=<app>/bin`, `PORT`, `HOST`;
  `ExecStart=<app>/bin/<entry> <arguments>`; `Restart=on-failure`,
  `RestartSec=5`, `TimeoutStopSec=30`; and the sandbox directives (§7).
- The SMTP egress lines, on a host that enforces the egress block:
  `ExecStartPre=+` loads the egress table and adds the unit's cgroup to the
  `smtp_blocked` set; `ExecStopPost=-+` removes it again
  ([SITE-SANDBOX.md §3](SITE-SANDBOX.md)).
- `[Install]`: `WantedBy=multi-user.target`.

A program that stops 5 times within 5 minutes is not restarted again
(`restart_loop`). Start, or a new deploy, clears the counter
(`systemctl reset-failed`) and makes a real new attempt.

**Owner changes (D-031).** The owner's own settings go in
`/etc/systemd/system/celikrt-<id>.service.d/*.conf`. CelikPanel never writes or
removes that folder, and the Programs tab lists its files. If the owner edits
the unit file itself, CelikPanel keeps the edit. The deploy stops with
`unit_edited`, and CelikPanel's own text is placed beside it as
`celikrt-<id>.service.celikpanel-pending`. Move your settings into a drop-in,
or put CelikPanel's text back, then deploy again.

The sandbox proof (§8) reads the loaded unit, drop-ins included, and refuses
to publish when one of the compared properties differs from the profile. See
§17 for which properties the proof compares.

## 7. The sandbox

Every program runs in the shared site sandbox. It is described in full in
[SITE-SANDBOX.md](SITE-SANDBOX.md): the site's own account and no
privileges; an empty read-only hosting tree with only the app folder bound
in; hidden Agent, PHP-FPM and CelikPanel paths; a system-call filter; no raw
sockets; binding only its own loopback port; the metadata block; the SMTP
egress block; and the memory, CPU, process and file limits of the offering.

The program kind adds three settings on top:

- `MemoryDenyWriteExecute=yes`: writable memory cannot be made executable.
- `NoExecPaths=/` and `ExecPaths=<app folder> /usr/lib /usr/lib64 /lib /lib64`:
  the program can execute only from its own folder and the shared libraries.
  `/bin/sh`, `python3` and `ld.so /usr/bin/python3` fail.
- `ProcSubset=pid`: `/proc` shows only processes, as for Python apps.

These are defence in depth, not the boundary.

## 8. Deploy

A deploy is one durable operation (D-025). Its steps run in this order:

| Step | What happens | Failure and what it means |
|---|---|---|
| `validate` | The Panel checks the security hold, the offering and the suspension again. The Agent checks the site's account, the host's sandbox level against the accepted level, and the entry (§3). Nothing changes. | `held`, `not_offered`, `suspended`, `sandbox_reduced`, an entry code, `site_account`: unmet prerequisites. `entry_unreadable` and `agent_unreachable`: unknown result. |
| `render` | The Agent makes `data/` and `config/` as the site's account, renders the unit, checks the egress table, writes the unit under the D-031 rule and reloads systemd. It starts nothing. | `unit_edited` (prerequisite: the owner changed the unit). `site_folder` and `unit_render` (verified failures). `unit_write`, `unit_dir` and `sandbox_egress` (unknown). `build_mismatch` (prerequisite: the Panel and the Agent are from different builds; it clears when the update finishes). |
| `restart` | `systemctl reset-failed`, `enable` and `restart`. | `systemctl` (verified failure), `unit_missing` (prerequisite). |
| `proof` | For up to 2 minutes, the Panel waits until a process **of this unit** listens on `127.0.0.1:<port>`, then proves the loaded sandbox against its profile ([SITE-SANDBOX.md §5](SITE-SANDBOX.md)). | `listens_elsewhere`, `port_taken`, `oom`, `restart_loop`, `sandbox_proof`: the program is stopped and disabled, and nothing is published. `start_failed`: the unit failed while starting. `not_listening`: it ran but did not listen in time. |
| `health` | For up to 90 seconds, one GET to `http://127.0.0.1:<port><health path>` every 3 seconds, with the site's name as `Host`, `X-Forwarded-Proto: https`, redirects not followed. Any answer below 500 counts as healthy. | `health_failing`: the program keeps running so its log and answers can be read, but the site is not switched to it. |
| `publish` | The site becomes type `runtime_app` with the app's port and forced HTTPS, and its nginx file is written again. The app is `running` and `deployed`. | `publish` (unknown): the program runs, but the domain could not be switched yet. Deploy again. |

**A redeploy has no rollback to an earlier upload.** The files are the
customer's own, and CelikPanel keeps no copy. On a redeploy of an app that is
already published, the site already points at the port. If the proof fails,
the program is stopped, and the site answers with an error until the next
successful deploy. If the health check fails, the new version keeps
answering.

Every failure keeps its class (verified failure, unmet prerequisite or
unknown result), its reason code and the server's own redacted words. The
screen shows them with who acts and the next step (§12).

## 9. Daily actions

| Action | What it does | Refused when |
|---|---|---|
| Settings (`PUT …/settings`) | Changes entry, arguments and health path. They apply at the next deploy. | Any gate of new use: rights, hold, suspension, host level, full disk (§10, §11). |
| Start / Restart | `reset-failed`, `enable`, `restart`. | Not deployed yet; held; suspended; host level not accepted; another operation is running (`busy`). |
| Stop | `stop` and `disable`, so the program stays stopped after a reboot. | Never by rights. Only while a deploy or removal of the app runs (`busy`). |
| Environment | §5. | Only while an operation runs (`busy`). |
| Log | The last lines (default 200, at most 500), redacted on the server. | Never by rights. `logs_unavailable` when the journal cannot be read. |
| Remove | Durable operation: `unpublish` → `remove` → `done`. | Never by rights. Only while an operation runs (`busy`). |

**Removal.** `unpublish` puts the site back to the type and port it had
before the program, and writes its nginx file again. `remove` stops,
disables and resets the unit, then removes the unit file and the environment
file and reloads systemd. If the owner had changed the unit, a copy is kept
as `celikrt-<id>.service.celikpanel-removed-<UTC time>`. The owner's drop-in
folder is not touched. The Agent then checks that the unit is no longer
active (`verify`).

**The app folder is kept** unless the customer ticks "remove files" and types
the domain name (`confirm_files` otherwise). The folder is then removed by
the site's own account.

While a program exists on a domain, the domain's site type cannot be changed
and the domain cannot be deleted (`app_created`). Remove the program first.
Team members (additional users) cannot use this feature.

## 10. Rights (D-042)

- Permission **Uploaded programs** (`program_apps`) and the offering
  **`runtime:program`**.
- **Closed until offered.** The administrator opens the offering on the Add-ons
  page after reading its risk summary: what a program can do, what the sandbox
  blocks, network, and what remains a risk. The administrator also sets the
  memory (64–65536 MB, default 512), CPU (10–1600 %, default 100) and process
  (16–8192, default 256) limits.
- **The offer records the summary's digest.** The digest covers the kind, the
  sandbox generation, the default limits and the risk lists. A release that
  changes any of them closes the offer again (`offer_stale`) until the
  administrator reads it and offers it once more. A stale request is refused
  with `risk_changed`.
- **Reduced level.** On a host that enforces less than the full sandbox, the
  offer is accepted only when the administrator accepts each named gap
  (`systemd_old`, `systemd_unknown`, `bpf_missing`, `cgroup_v2_missing`,
  `nft_cgroup_missing`). A reduced level never passes silently. If the host
  later shows a gap that was not accepted, deploy and start are refused
  (`sandbox_reduced`) until the administrator accepts it or fixes the host.
  The Agent checks this again itself.
- **The chain only narrows.** Unlimited plans take an offered `runtime:program`.
  A reseller or a plan can only leave it out. The server checks every layer on
  create, settings and deploy.
- **Stopping, the log, the environment and removal are never refused for
  rights** (P5), so a narrowed right never strands a running program.
  Withdrawing the offer stops new use only. What runs keeps running, and a
  deployed program can still be started.
- Limits are written into each unit when it is deployed. New limits reach an
  app at its next deploy.

## 11. Security hold, suspension and a full disk

- **Security hold** (administrator, with a reason of 1–500 characters): every
  deployed program is stopped and disabled at once. The answer lists the
  programs that were stopped and those that could not be stopped, with the
  reason. Create, settings, deploy, start and restart are refused until the
  hold is lifted. Lifting the hold starts nothing: each customer starts their
  program again. Files are never removed. A deploy that is running reads the
  hold, the offering and the suspension again at `validate`, before the
  program is started and before the site is switched to it, and stops there
  with `held` (a program it already started is stopped). The hold and a
  suspension also cover a first deploy that is still running.
- **Suspension** of a subscription stops and disables its deployed programs,
  so they stay stopped after a reboot and without the panel. Resuming
  restores only what ran before (compare-and-set, D-022). A program whose
  offering is held stays stopped at resume.
- **Full disk** (D-044): at 100% of the subscription's disk, create, settings
  and deploy are refused. A running program is not stopped for it.

## 12. What the screen says (D-024)

Each state names the reason, who acts, the next action and how work resumes.
The states are listed most blocking first. **Unknown** is never shown as a
failure.

| State | Class | Reason | Who acts | Next action | How it resumes |
|---|---|---|---|---|---|
| `agent_unreachable` | unknown | The server's Agent did not answer the status read. The program's real state is not known. | Nobody at first; the administrator if it lasts | Refresh in a minute. The administrator checks `systemctl status celikpanel-agent`. | The next read shows the real state. The program itself keeps running under systemd. |
| `suspended` | prerequisite | The subscription is suspended, and the program is stopped. | Administrator or reseller | Resume the subscription. | The resume starts it again if it ran before. |
| `held` | prerequisite | The administrator put uploaded programs on a security hold. | Administrator | Lift the hold when the advisory is handled. | The customer presses Start or deploys. Lifting starts nothing. |
| `not_offered` | prerequisite | The offering or the permission is not open for this account (not offered, withdrawn, its summary changed, or left out by a plan). | The grantor the screen names | Ask them to offer it. | Create, settings and deploy work again. A running program keeps running meanwhile. |
| `sandbox_reduced` | prerequisite | This server enforces less than the full sandbox, and the named gaps were not accepted. | Administrator | Fix the host (systemd, BPF, cgroup v2, nft), or accept the gaps on the Add-ons page. | Deploy and start work again. |
| `no_certificate` | prerequisite | The domain has no certificate, and a program is served only over HTTPS. | Customer | Get a free certificate on the SSL page. | Deploy. |
| `disk_full` | prerequisite | The subscription's disk is full. | Customer, or the provider for a larger limit | Free space or raise the limit. | Create and deploy are accepted again. |
| `entry_missing` | prerequisite | `app/bin/<entry>` is not on the server. | Customer | Upload it with the file manager or SFTP. | Deploy. |
| `entry_not_executable` | prerequisite | The entry cannot be executed. | Customer | `chmod u+x`. | Deploy. |
| `entry_link` | prerequisite | The entry, or a folder on its path, is a link. | Customer | Upload the program itself. | Deploy. |
| `entry_not_regular` | prerequisite | The entry is a folder or a special file. | Customer | Upload the program file. | Deploy. |
| `entry_not_owned` | prerequisite | Another account owns the entry. | Customer; the administrator if root owns it | Upload it again with the file manager or SFTP. | Deploy. |
| `entry_setid` | prerequisite | The setuid or setgid bit is set. | Customer | `chmod u-s,g-s`. | Deploy. |
| `entry_writable` | prerequisite | Anyone on the server could change the entry. | Customer | `chmod o-w`. | Deploy. |
| `entry_unknown_format` | prerequisite | Neither ELF nor `#!`. | Customer | Upload the compiled program. | Deploy. |
| `entry_wrong_arch` | prerequisite | It was built for another processor. | Customer | Build it for this server's processor. | Deploy. |
| `entry_interpreter` | prerequisite | A `#!` script names an interpreter outside the app folder. | Customer | Upload a compiled program. | Deploy. |
| `app_folder_not_owned` | prerequisite | `app` is a link, or not the site's own folder. | Customer; the administrator if root owns it | Make `app` a real folder of the site. | Deploy. |
| `restart_loop` | verified failure | The program stopped 5 times within 5 minutes, and systemd gave up. | Customer | Read the log, fix the program, upload it again. | Start or deploy (the counter is cleared). |
| `oom` | verified failure | It used more memory than its limit, and the kernel stopped it. | Customer, or the administrator for a higher limit | Use less memory, or raise the offering's limit. | Start (a new limit needs a deploy). |
| `listens_elsewhere` | verified failure | It listened beyond 127.0.0.1. It was stopped. | Customer | Listen on `HOST:PORT` from the environment. | Deploy. |
| `port_taken` | verified failure | Another process holds the app's port. | Administrator | Find it (`ss -tlnp 'sport = :<port>'`) and stop it. | Deploy. |
| `not_listening` | verified failure | It runs but does not listen on `127.0.0.1:<port>`. | Customer | Read `PORT` and `HOST`, or use `{port}`. | Deploy. |
| `health_failing` | verified failure | The health path answered 500 or more, or not at all. | Customer | Fix the program, or choose another health path. | Deploy. |

The health read runs only during a deploy and when the person asks for a
fresh read (`?health=1`), never from the screen's polling.

## 13. Without CelikPanel (D-022)

The unit, its environment file and the app folder are plain systemd and file
system state. They keep working if the panel stops, loses its licence or is
removed. On the server, as root:

```sh
systemctl status celikrt-<id>          # is it running
systemctl stop celikrt-<id>            # stop now
systemctl disable celikrt-<id>         # do not start at boot
systemctl start celikrt-<id>           # start again
journalctl -u celikrt-<id> -n 100      # its log
```

To remove a program by hand:

```sh
systemctl disable --now celikrt-<id>.service
rm /etc/systemd/system/celikrt-<id>.service
rm /etc/celikpanel-runtime/celikrt-<id>.service.env
rm -r /etc/systemd/system/celikrt-<id>.service.d   # only if you made drop-ins
systemctl daemon-reload
```

nginx keeps proxying the domain to the port until the site's type changes.
With the panel still installed, removing the program on the Programs tab puts
the site type back (it also works when the unit is already gone). Without the
panel, the owner edits the site's nginx file. The app folder stays until
someone deletes it.

The SMTP egress block keeps working without the panel
([SITE-SANDBOX.md §4](SITE-SANDBOX.md)).

## 14. Not covered

- Port 2525 and mail sent through HTTP mail APIs (only 25, 465 and 587 are
  blocked).
- The program's own outbound traffic is not counted against the
  subscription's traffic.
- CelikPanel does not build programs from source. The customer uploads a
  finished program.

## 15. Audit

Actions are written to the audit log as `runtime.create`, `runtime.settings`,
`runtime.deploy`, `runtime.start`, `runtime.stop`, `runtime.restart`,
`runtime.env` (keys only), `runtime.remove`, and for the administrator
`runtime.offer`, `runtime.withdraw`, `runtime.hold` and `runtime.release`.

## 16. Resilience (D-025)

This section covers invariants 1 (the owner keeps authority and working
services) and 4 (every mutation carries a recovery contract) of
[RESILIENCE-CONTRACT.md](RESILIENCE-CONTRACT.md).

- **Schema.** Migration 063 is additive. It creates three new tables
  (`runtime_offerings`, `runtime_apps`, `runtime_app_operations`) and their
  indexes, and changes no existing table, trigger or value. A release before
  alpha.119 refuses a database whose ledger has entry 63. Going back is the
  pre-update snapshot restore, as for every migration.
- **Durable operations.** At most one operation runs per app (a unique
  index). A Panel restart continues a running operation from its recorded
  step. Every Agent step can be repeated safely. A repeated operation id
  returns the first answer and starts nothing (lost-answer guard). Nothing
  secret is held only in memory: the environment file is already on the
  server.
- **Polling never mutates.** The screen's status read calls only read-only
  Agent methods. It does not start a step, and it does not run the health
  GET.
- **Native state belongs to the server.** The Panel's rows are its record of
  the unit, the environment file and the folder. They are not the source of
  the program's lifecycle.

Component tests cover the rules above. They are not lab evidence: no real
update and automatic rollback across these tables has been run yet.

## 17. Known gaps (for the lab round)

- The escape matrix ([SITE-SANDBOX.md §7](SITE-SANDBOX.md)) has not been run
  on the five supported systems yet. `deploy/e2e/sandbox-escape/panel-runtime.sh`
  can run its Go probe as a real runtime app through the panel (lab VM only).
- `ProcSubset=pid`, `MemoryDenyWriteExecute` and `NoExecPaths`/`ExecPaths`
  have not been measured with real Go and Rust programs. Some programs may
  need a `/proc` file or a JIT, and would fail to start.
- `BindPaths=` follows a link. If the customer replaces `<site home>/app` with
  a link between the folder check and systemd's start, the link is bound in.
  The folder check is not atomic with the start. This is a gap of the sandbox
  core, shared with service apps.
- The sandbox proof compares a fixed list of properties (account, cgroup,
  file and kernel protections, memory, processes, IP and bind denial, the
  egress lines). A drop-in that changes something outside that list, for
  example `NoExecPaths=`, `SystemCallFilter=` or `CPUQuota=`, is listed on
  the Programs tab but does not stop publication.
- The health GET runs on demand and during a deploy, not yet every 5 minutes.
- Abuse notices from cgroup events (OOM kills, the process limit) are not
  sent yet. The counts are shown on the Programs tab.
- On Ubuntu 22.04, nft's `socket cgroupv2` matching (the SMTP block) is
  unproven.
