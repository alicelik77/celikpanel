# Site sandbox escape-test kit

This kit checks the site sandbox (`docs/SITE-SANDBOX.md`) on a real system. It
starts small probe programs under systemd units rendered exactly as CelikPanel
renders them. Each probe tries every escape in the matrix below. The kit
reports **PASS** only when every attempt fails.

The release preparer runs it once per supported system (Debian 12/13,
Ubuntu 22.04/24.04, Arch) in the lab. **Run it only on a lab VM.** It creates
a test account, two test sites, three units and a throwaway network
namespace, and it removes all of them at the end. It never runs
`nft flush ruleset` and never touches the firewall table `inet celikpanel_fw`.

## Contents

| Path | What it is |
|---|---|
| `render/` | Prints the egress table, its guard and a sandboxed unit for this host's capabilities (`sitesandbox.Probe`). It changes nothing. |
| `bin/sandbox-helper` | The Agent, built by `run.sh build`; the bind units and the egress lines run it as the sandbox helper. When the host has none, the kit installs it at `/usr/lib/celikpanel-sandbox/sandbox-helper` and the bind template `celikpanel-sandbox@.service` (`render -what bind-template`), and removes both at the end. |
| `probes/goprobe/` | A static Go probe (the "uploaded program" kind: `NoExecPaths`, `MemoryDenyWriteExecute`). |
| `probes/probe.py` | A Python probe (the python kind), using ctypes for the system calls. |
| `probes/wsgi_probe.py` | The Python probe as a WSGI app (`wsgi_probe:application`): `GET /matrix` runs the matrix once, for `panel-runtime.sh KIND=python`. |
| `run.sh` | The runner. |
| `panel-runtime.sh` | Runs the Go probe (or, with `KIND=python`, the Python probe) as a real runtime app through a lab panel's API (below). |

There is no C or Rust probe: the development system used to write the kit
had neither toolchain. The Go probe is built static (`CGO_ENABLED=0`). A Rust
or C probe can be added later in `probes/` without changing `run.sh`'s
reporting format.

## Use

On the development machine (needs Go):

```sh
deploy/e2e/sandbox-escape/run.sh build      # writes bin/render, bin/goprobe and bin/sandbox-helper
```

Copy `deploy/e2e/sandbox-escape/` to the lab VM. The VM needs root, systemd,
nft, ip, ss, findmnt and python3. Then:

```sh
sudo ./run.sh               # the whole matrix
sudo ./run.sh quick         # the two probes only (no namespace, no load tests, no table removal)
sudo ./run.sh reboot-prepare; sudo reboot; sudo ./run.sh reboot-verify   # item 17 across a reboot
sudo NODE_VERSION=22.23.4 ./run.sh node   # Node.js sites only (alpha.127): the matrix under a Node.js site's unit, items 19 and 20
```

Items 19 and 20 need a Node.js under `/opt/celikpanel/runtimes/node/<version>`
(Components → Node.js on a CelikPanel host); `NODE_VERSION` names it, and
`matrix` and `quick` include them when it is set (SKIP otherwise). The kit
puts a decoy version `0.0.1` beside it for item 19 and removes it at the end.

The last line is `PASS` or `FAIL`. Every item shows its own result and the
attempts that escaped. A host at the **reduced** level fails the items its
named gaps cover. For example, item 9 fails without nft cgroup matching. That
result is correct, and the level on the screen already names the gap.

If CelikPanel already wrote `/usr/lib/celikpanel-sandbox/egress.nft` on the
host, the kit uses that file as it is. Otherwise the kit installs the same
files CelikPanel would write and removes them at the end.

## Output format

Each probe prints one line per attempt:

```
SANDBOX-PROBE <item> <name> BLOCKED|ESCAPED|SKIP <detail>
```

`BLOCKED` means the attempt failed, as it must. `run.sh` reads these lines
from the journal. An item with no line at all is a FAIL.

## The matrix

| # | Attempt | How it is judged |
|---|---|---|
| 1 | Another site's files; listing the hosting tree and /home | The probe reads `/var/www/celikpanel/subscriptions/990002/.../secret.txt` (mode 0644) |
| 2 | `/etc/shadow`, `/etc/celikpanel`, `/var/lib/celikpanel*` | read or list |
| 3 | The Agent socket, other sites' PHP-FPM sockets | connect; any visible socket is a FAIL |
| 4 | setuid(0), sudo, su, `chmod u+s` | each must fail; the setuid bit must not stick |
| 5 | ptrace and process_vm_readv of another process | each must fail. Under `PrivatePIDs` the probe is PID 1 and has no parent in its namespace, so it forks a child of its own (only the system-call filter stands in the way) and tries `PROBE_TARGET_PID`, a host PID outside its namespace that the runner passes (a PHP-FPM worker, else journald). In the probe's namespace that number names no process (`ESRCH`) or the attempt is refused (`EPERM`): both are PASS. |
| 6 | mount, unshare (user, mount), pivot_root | each must fail |
| 7 | Processes of other users in /proc | none visible |
| 8 | AF_PACKET, raw ICMP | socket() must fail |
| 9 | Ports 25, 465 and 587 on 127.0.0.1, ::1 and the internet; sendmail | each must fail, **and** the egress counter must rise by at least 6 (so an absent listener cannot pass for the block) |
| 10 | Binding another port, port 0 or UDP; listening on 0.0.0.0 | bind must fail. A 0.0.0.0 listener on the app's own port (which SocketBind allows) must not be reachable from a veth in a throwaway namespace. If a reference listener outside the sandbox is unreachable too, the host's own firewall dropped both, and the result is SKIP: inconclusive, not a failure. The runner says so in the item's line and lists every SKIP item before the verdict |
| 11 | Fork bomb, memory balloon, CPU spin, log flood | refused after TasksMax; `Result=oom-kill`; CPU ≤ 14 s in 20 s at 50 %; fewer than 20 000 of 50 000 lines in the journal |
| 12 | 169.254.169.254, fd00:ec2::254 | connect must fail |
| 13 | Writes outside the app folder; /tmp and /var/tmp persistence | write must fail; markers must not survive a restart |
| 14 | exec of /bin/sh, python3, `ld.so /usr/bin/python3` | must fail for the program kind; SKIP for the python kind, whose interpreter is its runtime |
| 15 | Kernel keyring (keyctl) | must fail |
| 16 | Writes to /sys/fs/cgroup (root and own memory.max) | must fail |
| 17 | The SMTP block after a restart, after a reboot and after the egress table is removed | the unit's cgroup is in `@smtp_blocked` again, and a new attempt is refused (the counter rises). After the table is removed, the guard timer must restore it within 45 seconds. |
| 18 | Swap the app folder for a link to another site's folder while the program runs, then restart | the running program still sees only its own folder (read through `/proc/<pid>/root`, its own mount namespace; `nsenter -m test` cannot run there under `NoExecPaths`); the restart is refused (`app_folder_not_real` in the journal of `celikpanel-sandbox@<unit name>`) and no mount's source folder (`FSROOT`) is the other site's; with the real folder back the unit starts again, and within 15 seconds of a stop no `/run/celikpanel-sandbox/<unit>` is left |
| 19 | A Node.js site (alpha.127): another Node.js version, writing the runtime, the toolchains | The probe runs as the site's code through its start command (`render -what node-unit`, the Agent's renderer). A decoy version beside the site's must not be listed; the site's own `bin/node` must be reachable; a write into the runtime and a listing of `/opt/celikpanel/toolchains` must fail. Items 1–16 are judged under the Node.js unit too (14 is SKIP: npm needs `/bin/sh`) |
| 20 | A Node.js site's install step `celikappinst-<id>` | `systemd-run` must accept the rendered arguments (`render -what node-install`); npm installs a project whose `postinstall` runs the probe once (`PROBE_ONCE`) and whose `build` runs node. No attempt may escape (mail ports on a host without the SMTP block count for item 9), the step must end `success`, and the build must have written its file |

## Running the probe as a real runtime app

`run.sh` renders its own units. `panel-runtime.sh` instead lets a **lab
panel** do everything a customer's uploaded program goes through
([docs/RUNTIME-APPS.md](../../../docs/RUNTIME-APPS.md)): create, the entry
check, the real `celikrt-<id>.service` unit with its environment file,
deploy (validate, render, restart, sandbox proof, health, publish), and the
redacted log view. The probe runs with `-serve -port={port}`: it runs the
matrix once, prints its `SANDBOX-PROBE` lines to the journal, then listens on
`127.0.0.1:$PORT` and answers 200 on `/` so that the health step passes.

**Lab VM only.** The script refuses to start without `LAB_ONLY=yes`. Point
it only at a lab panel, never at a real server. It never updates a panel,
never touches the firewall, and starts nothing on the server except, with
`UPLOAD=local`, copying the probe into the site's `app/bin` as the site's
own account.

Prerequisites on the lab panel: the administrator offered `runtime:program`
on the Add-ons page; the account's plan grants `program_apps`; the lab domain
has a website and a certificate, and no other program.

```sh
./run.sh build          # on a machine with Go: bin/goprobe (static)

# On the lab server as root (uploads as the site's account; also reads the SMTP counter):
sudo LAB_ONLY=yes PANEL_URL=https://lab-vm:1923 COOKIE=<celikpanel_session> \
  DOMAIN_ID=<id> OTHER_SITE_FILE=<a 0644 file of another lab site> \
  CURL_INSECURE=1 REMOVE=1 ./panel-runtime.sh

# From elsewhere, uploading over the domain's SFTP account (key authentication):
LAB_ONLY=yes PANEL_URL=https://lab-vm:1923 COOKIE=<celikpanel_session> DOMAIN_ID=<id> \
  UPLOAD=sftp SFTP_TARGET=<sftp user>@lab-vm SFTP_IDENTITY=~/.ssh/lab_key ./panel-runtime.sh
```

- **Authentication.** The panel has no API tokens. Sign in to the lab panel
  in a browser as the domain's owner (or the administrator) and copy the
  value of the `celikpanel_session` cookie into `COOKIE`. The script passes
  it through a private curl config file, not on the command line, and sends
  `Origin: $PANEL_URL` (the panel refuses a write without it).
- **Upload.** `UPLOAD=local` (the default as root) copies the probe with
  `runuser` as the account whose home is the site home. `UPLOAD=sftp` puts it
  into `app/bin/goprobe` with `sftp -b` (batch mode needs key
  authentication). `UPLOAD=none` means you uploaded it yourself with the file
  manager or SFTP, executable by its owner.
- **Environment.** Before the deploy the script sets `PROBE_RUN` (a marker,
  so only this start's lines are judged), `PROBE_SECRET` as a secret (the
  probe prints it on purpose, and the log view must hide it), and
  `PROBE_OTHER_SITE` from `OTHER_SITE_FILE`. Without `OTHER_SITE_FILE`,
  item 1 reads a path that may not exist, which proves nothing.
- **The Python probe (`KIND=python`).** The script uploads `probes/probe.py`
  and `probes/wsgi_probe.py` into `app/src` and creates a Python app
  `wsgi_probe:application` with gunicorn and one worker. There is no
  `requirements.txt`, so the build installs only CelikPanel's pinned
  gunicorn from PyPI (the lab VM needs outbound HTTPS to pypi.org). After the
  deploy it asks `GET /matrix` once (`APP_URL`, default
  `http://127.0.0.1:<port>` with `UPLOAD=local`, else `https://<domain>`);
  the matrix runs in a thread and the lines are judged from the log view as
  for the Go probe. Item 14 is `SKIP` for the python kind. Prerequisites: the
  administrator installed Python in Components and offered
  `component:python:<minor>`; the plan grants `python_apps`.

  ```sh
  sudo LAB_ONLY=yes KIND=python PANEL_URL=https://lab-vm:1923 COOKIE=<celikpanel_session>     DOMAIN_ID=<id> OTHER_SITE_FILE=<a 0644 file of another lab site> CURL_INSECURE=1 ./panel-runtime.sh
  ```
- **One operation id.** The deploy is posted with one operation id. If no
  answer arrives, the same id is posted again (the panel's lost-answer
  guard). The script then only reads (`GET` without `?health=1`) until the
  operation ends. It never posts a new id while one is running.
- **Item 5's outside process.** `TARGET_PID` is a host PID outside the app.
  With `UPLOAD=local` the default is the site's own PHP-FPM worker, then any
  PHP-FPM worker, then journald. Without it (`UPLOAD=sftp` or `none`) the
  probe tries only a child of its own, and the script notes that.
- **Judging.** The script reads `GET …/runtime-apps/logs?lines=500` and
  judges the lines after this run's marker as `run.sh` does: an item with no
  line is a FAIL, and any `ESCAPED` is a FAIL. It also checks that `HOST` is
  `127.0.0.1` and `PORT` is the app's port (`env`), and that the secret never
  appears in the log view (`log`). A failed deploy is a FAIL, with its step,
  class, reason and message. With `UPLOAD=local` the SMTP counter must rise
  by at least 6 (item 9). Items 11 and 17 stay with `run.sh`.
- **Removal.** `REMOVE=1` removes the program at the end with its own
  operation id. `REMOVE_FILES=1` also removes the app folder (the script
  types the domain as the confirmation).

The last line is `PASS` or `FAIL`, and the exit status is 0 or 1.

## Known limits of the kit

- Item 17's "nft flush" is simulated with `nft delete table inet celikpanel_egress`.
  For this table, that has the same effect as `nft flush ruleset`, and it
  leaves the lab VM's firewall in place.
- Item 10's reachability test relies on the host firewall not dropping
  the veth traffic first. When the firewall drops both listeners, the
  result is SKIP rather than PASS, and the runner says why. This happened on
  the alpha.119 lab VMs.
- alpha.119 lab round 1 found these kit errors, all now corrected:
  - `panel-runtime.sh` stopped silently (`set -euo pipefail`) when it read
    the SMTP counter before the egress table existed (the first deploy).
  - `run.sh`'s `mark` returned 1 after a PASS without a detail, so every
    `cond && mark N PASS || mark N FAIL` also marked FAIL (false FAILs for
    items 9, 11, 17 and 18).
  - Item 18 ran `nsenter -m test` inside the program's namespace
    (impossible under `NoExecPaths`). Its mount check matched the python
    test unit's name (`celikrt-990002`) instead of the other site's folder.
    Its check that the stop left nothing raced the bind unit's stop.
  - Item 5 had no result: under `PrivatePIDs` the probe is PID 1 with no
    parent in its namespace. It now forks a child of its own and tries a
    host PID that the runner passes.

  The corrected kit has not been run again yet.
- The kit was smoke-run once (`quick`) on WSL Debian 13 (systemd 257, a
  kernel without `CONFIG_NFT_SOCKET`). Both probes started under the
  rendered units. Items 1–4, 6–8, 10, 12 and 14–16 passed. Item 9 failed,
  as it must at the reduced level. That run found two probe errors and one
  sandbox gap, all now corrected:
  - Item 5: with `PrivatePIDs` the probe read its own memory as PID 1.
    The probe now skips its own PID, and `process_vm_readv`/`writev` are
    now denied outright.
  - Item 13: `/var/tmp` is private under `PrivateTmp` and is writable by
    design. The probe now checks that `/var/tmp` does not persist, instead
    of treating a write there as an escape.

  The corrected kit has not been run again yet.
