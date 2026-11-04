# Uploaded programs and Python apps (runtime apps)

*alpha.119 · In source and component-tested. The lab round on the five
supported systems is pending.*

Turkish: [RUNTIME-APPS.tr.md](RUNTIME-APPS.tr.md) · Sandbox:
[SITE-SANDBOX.md](SITE-SANDBOX.md) · Rights:
[PLANS-AND-RIGHTS.md §4.8–4.9](PLANS-AND-RIGHTS.md)

A customer can run their own compiled program (Go, Rust, or any other program
built for this server's processor) as their site. The program runs as the
site's own account, inside the site sandbox, on one port that only the server
itself can reach. Visitors reach it only over HTTPS, through the site's nginx.

A customer can also run a Python web app (Django, Flask, FastAPI) on the same
engine, built into its own environment from PyPI with the distribution's
Python (§18).

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
| `python` | Implemented: a WSGI or ASGI web app in a virtual environment of the distribution's Python, with its own permission (`python_apps`) and its offering per distribution Python (`component:python:<minor>`). See §18. |

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
| The `app` folder, and every folder from the hosting tree down to it, is a real folder (not a link, not a mount point) | `app_folder_not_real` | Replace the link with the real folder, with the file manager or SFTP. |
| The `app` folder and the site home belong to the site's account; the folders above the site home belong to root and others cannot write to them | `app_folder_not_owned` | The administrator corrects the owner. |
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
  `WorkingDirectory=<app folder>` (resolved inside the sandbox, where that
  path is the bind of the folder); `EnvironmentFile=-<env file>`;
  `HOME=<app>/data`, `PATH=<app>/bin`, `PORT`, `HOST`;
  `ExecStart=<app>/bin/<entry> <arguments>`; `Restart=on-failure`,
  `RestartSec=5`, `TimeoutStopSec=30`; and the sandbox directives (§7).
- `Requires=` and `After=` of the bind unit `celikpanel-sandbox@celikrt-<id>.service`
  ([SITE-SANDBOX.md §1a](SITE-SANDBOX.md)): it checks the app folder before
  every start and binds it onto `/run/celikpanel-sandbox/celikrt-<id>.service/app`.
  `BindPaths=` binds only that root-owned path into the sandbox, at the
  folder's usual path.
- On a host that enforces the egress block, `ExecStartPre=+… sandbox-helper
  egress-add` adds the unit's cgroup to the `smtp_blocked` set and
  `ExecStopPost=-+… egress-remove` removes it ([SITE-SANDBOX.md §3](SITE-SANDBOX.md)).
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
| `render` | The Agent makes `data/` and `config/` as the site's account, renders the unit, checks the egress table, writes the unit under the D-031 rule and reloads systemd. It starts nothing. | `unit_edited` (prerequisite: the owner changed the unit). `site_folder` and `unit_render` (verified failures). `unit_write`, `unit_dir`, `sandbox_helper` and `sandbox_egress` (unknown). `build_mismatch` (prerequisite: the Panel and the Agent are from different builds; it clears when the update finishes). |
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
| Stop | `stop` and `disable`, so the program stays stopped after a reboot. A unit systemd does not know (never deployed, or its unit is gone) has nothing running: the stop succeeds without `systemctl`. For an app that was never deployed the answer is `reason: not_deployed` with “nothing to stop; deploy it to start it”, and its state stays. | Never by rights. Only while a deploy or removal of the app runs (`busy`). |
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
| `app_folder_not_real` | prerequisite | `app`, or a folder on its way, is a link or a mount point; the sandbox helper does not start the program. | Customer | Replace the link with the real folder. | Deploy. |
| `app_folder_not_owned` | prerequisite | `app` is not the site's own folder, or a folder above the site home is not root's alone. | Administrator | Correct the owner. | Deploy. |
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
- Closed in source (alpha.119): `BindPaths=` no longer names the site home.
  The bind unit opens the folder without following links at every start and
  binds it by descriptor; a folder swapped for a link later changes nothing
  for the running program, and the next start is refused
  (`app_folder_not_real`). Matrix item 18 of the escape kit proves it on a
  real system; the lab round is still open.
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

## 18. Python apps

A customer can run a Python web app (Django, Flask, FastAPI or any WSGI or
ASGI app) on the same engine. Everything in §§5–16 applies: the site's own
account, the site sandbox, one loopback port, HTTPS only, the durable deploy,
the hold, the suspension, the D-024 states and the native files. This section
lists what is different.

The uploaded code is arbitrary code, and so is every package pip installs.
The boundary is the sandbox, not the language.

### 18.1 The distribution's Python only

| System | Python | Packages |
|---|---|---|
| Debian 12 | 3.11 | `python3`, `python3-venv` |
| Debian 13 | 3.13 | `python3`, `python3-venv` (pulls `python3.13-venv`) |
| Ubuntu 22.04 | 3.10 | `python3`, `python3-venv` (pulls `python3.10-venv`) |
| Ubuntu 24.04 | 3.12 | `python3`, `python3-venv` (pulls `python3.12-venv`) |
| Arch | 3.14 (rolling) | `python` (carries `ensurepip`) |

Checked against the distributions' package indexes on 2026-11-04. No
deadsnakes, pyenv or python.org builds are used.

The administrator installs **Python** under Components (category Web, kind
Runtime). The Agent reads the minor from `/usr/bin/python3`'s link and checks
`/usr/bin/python3.<minor>` and `/usr/lib/python3.<minor>/ensurepip/__init__.py`.
On Debian and Ubuntu a `python3` that is not the system's own minor is
refused. On Arch, whatever minor `python3` names is the distribution's, as
long as this release pins an app server for it (3.10–3.14). A newer Arch
Python (3.15) waits for a CelikPanel update; the screen says so.

Removing the Python component is refused while other packages need `python3`
(the removal simulation names them, as for every component).

### 18.2 Rights and the offering

- Permission **`python_apps`** (Python apps).
- Offering **`component:python:<minor>`** for the server's Python, for
  example `component:python:3.13`. Unlike other component versions it is
  **closed until the administrator offers it** on the Add-ons page, after
  reading its risk summary (§18.9). Installing Python offers nothing.
- The administrator may also allow **source builds** for the offering. That is
  off by default and changes the risk summary the administrator agrees to (it
  adds “source builds”). The stored risk digest records the choice, so no
  schema change was needed (migration 063 holds the Python kind; 064 is not
  used).
- The route itself names no permission: `program_apps` or `python_apps` is
  checked inside, by the kind of the app.
- After a system upgrade (for example Debian 12 to 13) an app still names the
  old offering. Deploy refuses with `python_moved`; saving the app's settings
  once moves it to the server's current Python, under that offering's gates.

### 18.3 Files

```
<site home>/app/
  src/              the uploaded code and its requirements.txt
  venv-<n>/         the environment of build n (the running one and the one before)
  venv-<n>.partial  a build in progress (removed when it fails)
  .pip-cache/       pip's download cache (counts toward the site's disk)
  .build-tmp/       pip's temporary folder during a build
  data/ config/     as in §2
```

The unit runs in `app/src` (`WorkingDirectory`), so `module:callable` is
imported from there.

### 18.4 Settings

| Setting | Rule |
|---|---|
| App server | `wsgi` (gunicorn: Django, Flask) or `asgi` (uvicorn: FastAPI, Starlette). |
| App | `module:callable`, for example `mysite.wsgi:application` or `main:app`. A dotted module of identifiers and one identifier; no factory call, no expression. |
| Workers | 1 to `min(8, memory limit / 128 MB)`. |
| Health path | As in §8. |

The record keeps the callable in `runtime_apps.entry` and the server and
workers as two argument tokens (`server=wsgi`, `workers=2`) in
`runtime_apps.args`.

### 18.5 requirements.txt

Before a build the Agent reads `app/src/requirements.txt` and every file it
includes, all beneath `app/src`, without following a link (openat2
`RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS`). It refuses, naming the file and the
line:

| Refused | Code |
|---|---|
| `-e`, `--editable` | `editable` |
| `git+…`, `hg+…`, `svn+…`, `bzr+…`, `name @ git+…` | `vcs` |
| any other URL, `name @ https://…` | `url` |
| `./x`, `../x`, `/x`, `x/y`, `*.whl`, `*.tar.gz` and other archives | `path` |
| `-i`, `--index-url`, `--no-index` | `index_url` |
| `--extra-index-url` | `extra_index_url` |
| `--trusted-host` | `trusted_host` |
| `-f`, `--find-links` | `find_links` |
| `-c` naming a file outside `app/src` | `constraint_outside` |
| `-r` naming a file outside `app/src` | `nested_outside` |
| `-r`/`-c` naming a missing file | `nested_missing` |
| any other option (`--no-binary`, `--global-option`, …) | `option` |
| `$`, a backquote, or a line that is not `name[extras] spec ; marker` | `syntax` |
| more than 16 files, 256 KiB per file or 4000 lines | `too_large` |
| `-r` deeper than four levels | `nested_too_deep` |

Allowed: requirement lines, `--hash=sha256:…` (and sha384/sha512) after a
requirement, `--require-hashes`, `--pre`, `--prefer-binary`, and `-r`/`-c`
inside `app/src`. Credentials in a refused line are hidden before it is
shown.

pip reads a **flattened copy** that the Agent writes as root into
`/run/celikpanel-rtbuild/<app id>/` (directory 0750 and files 0640,
`root:<site group>`), never the customer's file. When any line carries a hash
(or the file says `--require-hashes`), pip runs with `--require-hashes`.

This is the offering's supply-chain policy, not the security boundary: the
customer can upload any code into `app/src` anyway.

### 18.6 The build (deploy step `build`)

Deploy: `validate → build → render → restart → proof → health → publish`.

The build is a transient unit `celikrtbuild-<id>.service`: the site's own
account, the app's sandbox without a listening port (outbound network open
for PyPI, the SMTP block in place, `Type=oneshot`, at most 30 minutes). Like
the app's own unit it requires its bind unit `celikpanel-sandbox@celikrtbuild-<id>`,
which binds the app folder from a root-owned mount point, and its only root
commands are the static helper's `egress-add`/`egress-remove`, which come
before the steps ([SITE-SANDBOX.md](SITE-SANDBOX.md) §1a). Its
steps are separate command lines; there is no shell:

```
ExecStart=/usr/bin/python3.<m> -m venv <app>/venv-<n>.partial
ExecStartPost=<venv>/bin/python -m pip install --isolated --no-input … --index-url=https://pypi.org/simple
              --cache-dir=<app>/.pip-cache --only-binary=:all: --require-hashes --no-deps -r /run/celikpanel-rtbuild/<id>/server.txt
ExecStartPost=<venv>/bin/python -m pip install --isolated … --only-binary=:all: [--require-hashes] -r …/requirements.txt [-c …/constraints.txt]
ExecStartPost=<venv>/bin/python -m pip check
```

The unit's command (`ExecStart`) is the system's own interpreter, which
creates the environment. `systemd-run` looks up the program of its command
on the host before the unit exists, so the command can never be a path the
build creates. (alpha.119 lab round 1: with the environment's python as the
command, every build failed with "Failed to find executable
…/venv-1.partial/bin/python".) systemd runs the `ExecStartPost` lines in
order after `ExecStart` succeeded. The first one that fails ends the build
(`Result=exit-code`), and the rest do not run. The root `egress-add`
(`ExecStartPre=+`) runs before all of them.

`--isolated` and `PIP_CONFIG_FILE=/dev/null` keep a `pip.conf` or a `PIP_*`
variable from changing the index. `--only-binary=:all:` is left out of the
customer's step only when the offering allows source builds; the pins are
always wheels.

**Reading the build (alpha.119 lab round 3).** Right after `systemd-run`, the
build unit waits for its bind unit. During that wait `systemctl show` reports
it as `inactive` with a queued start job (`Job=<id>`, no `InvocationID`). The
Agent counts that as a running build. Before this fix, the Panel's first read
took the waiting build for a stopped one and stopped it, and the bind unit's
helper was stopped with it (SIGTERM). So no build ever ran. A start job that
ends without running the unit (the bind unit failed) is reported with the
helper's reason, for example `app_folder_not_real`, never as "stopped
without a result".

**A new attempt always finds the name free.** Before every build, and when an
operation ends, the Agent stops the build unit and its bind unit, resets
both, and waits until systemd has unloaded the unit. An inactive transient
unit that a failed bind unit still refers to stays loaded, and `systemd-run`
then refuses the same name ("already loaded"). The service-app install
(`celiksvcinst-<id>`) works the same way. A repeat of the same operation
answers its record and never starts a second build.

**End-to-end check on a real systemd.** `cmd/agent/sandbox_e2e_linux_test.go`
and `sandbox_e2e_svc_linux_test.go` drive the Agent's own code with the real
bind template, the real helper and a real site account. They build a Python
app twice from PyPI, start it, serve it and stop it. They also stop a build
from outside and build again, start and stop an uploaded program, and run the
service-app install unit twice and the long-running service-app unit once.
They run only on a throwaway system:
`CELIKPANEL_E2E_SYSTEMD=1 CELIKPANEL_E2E_AGENT=<agent binary> go test -run TestE2E ./cmd/agent/`
(as root). They passed on Debian 13 (systemd 257) and Arch (systemd 262).

Generations (`internal/siteruntime/venv.go`):

1. Leftover `venv-*.partial` folders are removed; `n` is one above every
   generation the folder or the unit names.
2. The build writes `venv-<n>.partial`. **The running app is not touched.**
3. Success: one rename (`mv -T`, as the site's account) to `venv-<n>`. Every
   other generation except `venv-<n>` and the one the unit runs is removed.
4. Failure: `venv-<n>.partial` is removed; the operation names why.
5. Render writes the unit with `venv-<n>` and the app is restarted.
6. If an app that was deployed before does not come up (no listener, a
   failed start, a restart loop, an OOM, a public listener or a failing
   health path), the unit is written again with the generation it ran with
   and restarted. The operation still fails and says so. The code in
   `app/src` is what the customer uploaded; only the environment goes back.

The Agent keeps a record of each build (`runtime-apps/<id>/build.json`,
root's): the operation, the generation built and the one before. A Panel
restart continues the same build; a repeat of the same operation starts
nothing new. A build that systemd no longer knows (a reboot) is an unknown
result, never a success.

Why `python -m gunicorn` and not `venv/bin/gunicorn`: a console script keeps
the folder name it was installed under in its `#!` line, and the
environment is built as `venv-<n>.partial` and renamed. The interpreter link
and `pyvenv.cfg` do not depend on the folder name.

### 18.7 App servers pinned by hash

CelikPanel installs these wheels first, with `--require-hashes --no-deps`.
They are `py3-none-any`, so the same pins serve every supported minor;
`typing_extensions` is installed only on 3.10.

| Package | Version | Uploaded to PyPI | sha256 of the wheel |
|---|---|---|---|
| gunicorn | 26.2.0 | 2026-08-24 | `bd249d0b3f7972f7432f0a6b6ff3b3ee2d129f70cd1ff6c09a9dd9e29a2b88e3` |
| uvicorn | 0.54.0 | 2026-09-25 | `505bdb0f318731d45f1f712071fc781a8981f6847a31c902c9f5e652d4f67faf` |
| click | 8.5.0 | 2026-08-26 | `255bc9599cf7748b4b1a446ccc735421bd08a2ae529a8b88597d3de5664ee360` |
| h11 | 0.16.0 | 2025-04-24 | `63cf8bbe7522de3bf65932fda1d9c2772064ffb3dae62d55932da54b31cb6c86` |
| typing_extensions | 4.16.0 | 2026-07-02 | `481caa481374e813c1b176ada14e97f1f67a4539ce9cfeb3f350d78d6370c2e8` |

Source: PyPI's JSON API (`https://pypi.org/pypi/<name>/<version>/json`, the
`digests.sha256` of the `bdist_wheel` file), read on 2026-11-04. Every
version was at least two weeks old and not yanked. uvicorn is installed
without its `[standard]` extra, so nothing compiled comes with it. A new pin
is a reviewed change of `internal/siteruntime/pins.go`.

A requirements line that names gunicorn or uvicorn may change the installed
copy; the app server must stay importable.

### 18.8 The unit

```
WorkingDirectory=<app>/src
Environment=PATH=<app>/venv-<n>/bin:/usr/bin:/bin
Environment=PYTHONUNBUFFERED=1
Environment=PYTHONNOUSERSITE=1
ExecStart=<app>/venv-<n>/bin/python -m gunicorn --bind 127.0.0.1:<port> --workers <N> <module:callable>
ExecStart=<app>/venv-<n>/bin/python -m uvicorn --host 127.0.0.1 --port <port> --workers <N> <module:callable>
```

The ExecStart is fixed: every word is checked, there is no shell and no `$`
or `%` reaches the text.

**Sandbox:** the base set of [SITE-SANDBOX.md §1](SITE-SANDBOX.md) and
`ProcSubset=pid`. A Python app does **not** get `MemoryDenyWriteExecute` or
`NoExecPaths`/`ExecPaths`: an interpreter executes its own extension modules
from the environment, and some packages generate code at run time. So a
Python app can start other programs of the server (`/bin/sh` too); what they
can reach is what the sandbox allows, nothing more.

### 18.9 Risk summary

What the administrator reads before offering, and the customer before
deploying:

- **It can:** run any code, read and write its own folder, listen on one
  loopback port, open outbound connections, install packages from PyPI into
  its own environment.
- **The sandbox blocks:** other sites, CelikPanel's files, privileges, the
  mail ports, the metadata address, other ports.
- **Remains a risk:** outbound abuse; mail APIs; sharing CPU and memory; the
  reduced level where accepted; the customer's own code; **PyPI packages are
  third-party code — CelikPanel checks only that a file matches PyPI's
  published hash, not what it does;** an interpreter can start other
  programs and generate code; and, when allowed, **source builds** run each
  package's own build code during the deploy, in the same sandbox.

### 18.10 States (D-024)

| Code | Kind | Who acts | Meaning |
|---|---|---|---|
| `runtime_missing` | prerequisite | administrator | The server's Python or its venv module is not ready (or another minor, or no pins). |
| `src_missing` | prerequisite | customer | `app/src` is not there, or is a link or a file. |
| `venv_missing` | prerequisite | customer | The environment the unit names is gone; deploy builds a new one. |
| `requirements_refused` | prerequisite | customer | A line is refused; the screen names the file, the line and why. |
| `wheel_unavailable` | verified failure | customer | A package has no wheel for this Python on PyPI (or does not exist); the package is named. |
| `build_failed` | verified failure | customer | pip or the venv step failed; pip's last lines, redacted. |
| `pypi_unreachable` | unknown | customer, then administrator | The build could not reach pypi.org. |

Every build failure says that the running app was not changed. Polling reads
the operation; it never starts a build.

### 18.11 Without CelikPanel

The unit, its environment file and `app/venv-<n>` are ordinary files. The
owner can run `systemctl restart celikrt-<id>`, or rebuild an environment by
hand as the site's account (`python3 -m venv`, `pip install -r
app/src/requirements.txt`). A unit the owner changed is kept (§13).

### 18.12 Known gaps

- No lab run yet: Django (gunicorn), FastAPI (uvicorn) and the escape kit's
  Python probe on the five systems, with `ProcSubset=pid` and the build's
  outbound access, are for the lab round.
- The rollback restores the environment, not the code in `app/src`.
- pip itself (from `ensurepip`, the distribution's bundled wheel) is not
  pinned by CelikPanel.
- The build's outbound traffic is not limited to pypi.org.
