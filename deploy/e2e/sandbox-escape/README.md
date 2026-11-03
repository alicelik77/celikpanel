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
| `probes/goprobe/` | A static Go probe (the "uploaded program" kind: `NoExecPaths`, `MemoryDenyWriteExecute`). |
| `probes/probe.py` | A Python probe (the python kind), using ctypes for the system calls. |
| `run.sh` | The runner. |

There is no C or Rust probe: the development system used to write the kit
had neither toolchain. The Go probe is built static (`CGO_ENABLED=0`). A Rust
or C probe can be added later in `probes/` without changing `run.sh`'s
reporting format.

## Use

On the development machine (needs Go):

```sh
deploy/e2e/sandbox-escape/run.sh build      # writes bin/render and bin/goprobe
```

Copy `deploy/e2e/sandbox-escape/` to the lab VM. The VM needs root, systemd,
nft, ip, ss and python3. Then:

```sh
sudo ./run.sh               # the whole matrix
sudo ./run.sh quick         # the two probes only (no namespace, no load tests, no table removal)
sudo ./run.sh reboot-prepare; sudo reboot; sudo ./run.sh reboot-verify   # item 17 across a reboot
```

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
| 5 | ptrace and process_vm_readv of another process | each must fail |
| 6 | mount, unshare (user, mount), pivot_root | each must fail |
| 7 | Processes of other users in /proc | none visible |
| 8 | AF_PACKET, raw ICMP | socket() must fail |
| 9 | Ports 25, 465 and 587 on 127.0.0.1, ::1 and the internet; sendmail | each must fail, **and** the egress counter must rise by at least 6 (so an absent listener cannot pass for the block) |
| 10 | Binding another port, port 0 or UDP; listening on 0.0.0.0 | bind must fail. A 0.0.0.0 listener on the app's own port (which SocketBind allows) must not be reachable from a veth in a throwaway namespace. If a reference listener outside the sandbox is unreachable too, the result is SKIP (inconclusive) |
| 11 | Fork bomb, memory balloon, CPU spin, log flood | refused after TasksMax; `Result=oom-kill`; CPU ≤ 14 s in 20 s at 50 %; fewer than 20 000 of 50 000 lines in the journal |
| 12 | 169.254.169.254, fd00:ec2::254 | connect must fail |
| 13 | Writes outside the app folder; /tmp and /var/tmp persistence | write must fail; markers must not survive a restart |
| 14 | exec of /bin/sh, python3, `ld.so /usr/bin/python3` | must fail for the program kind; SKIP for the python kind, whose interpreter is its runtime |
| 15 | Kernel keyring (keyctl) | must fail |
| 16 | Writes to /sys/fs/cgroup (root and own memory.max) | must fail |
| 17 | The SMTP block after a restart, after a reboot and after the egress table is removed | the unit's cgroup is in `@smtp_blocked` again, and a new attempt is refused (the counter rises). After the table is removed, the guard timer must restore it within 45 seconds. |

## Known limits of the kit

- Item 17's "nft flush" is simulated with `nft delete table inet celikpanel_egress`.
  For this table, that has the same effect as `nft flush ruleset`, and it
  leaves the lab VM's firewall in place.
- Item 10's reachability test relies on the host firewall not dropping
  the veth traffic first. When the firewall drops both listeners, the
  result is SKIP rather than PASS.
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
