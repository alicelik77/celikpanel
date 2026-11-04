# Lab images: Debian 12 and Ubuntu 22.04 (2026-11-04, 19:39-20:05 UTC)

**Release:** published tag `v0.1.0-alpha.121` at `ba0a53d11`, built the usual way (lab portal with a
lab release key; acceptance-licence dist `acc903eb…`).

**Guests:**
- **d12:** Debian 12, fresh install with the owner's get.sh command, one-server setup with BIND.
- **u22:** Ubuntu 22.04, fresh install with the owner's get.sh command.

Lab only. Driven through the panel API like a user; SSH was used only to look.

**Result: stopped at product defects. Both new images are blocked at alpha.121.** Steps 3 (Ubuntu
22.04 sandbox probe, items 9 and 18) and 4 (Debian 12 probe, PGDG 18 preview) were not run.

## Images (step 1)

| Image | Source | Check |
|---|---|---|
| `debian-12-genericcloud-amd64-20261026-2643.qcow2` | cloud.debian.org bookworm/20261026-2643 | SHA-512 matches that build's `SHA512SUMS`. Debian publishes no signature there (`.sign` and `.gpg` both 404). |
| `ubuntu-22.04-server-cloudimg-amd64-20261020.img` | cloud-images.ubuntu.com jammy/release-20261020 | SHA-256 matches `SHA256SUMS`. That file has GOODSIG/VALIDSIG from `D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81`. |

The files are stored read-only (0444) beside the others in `/var/tmp/cp-v3n28/images`, pinned in
`beta1/lab.py` and documented in the README. The previous (stopped) lab agent had downloaded both;
this run re-verified them from fresh sums. Evidence: `raw/image-verify.txt`.

## Defect 1: the Ubuntu 22.04 install is refused

get.sh verified the archive, then `install.sh` stopped after 8.6 s:

`ERROR / HATA: systemd private socket is group/other writable`

**Cause.** Ubuntu 22.04's systemd 249 creates `/run/systemd/private` as `srwxrwxrwx` (0777). Later
systemd uses 0700: Debian 12's systemd 252 shows `srwx------`.

`validate_systemd_runtime` in `install.sh` refuses any group/other write bit. The same check is in
`rollback.sh` (in since `8ddea98b0`, 2026-08-24).

The socket is root:root in a 0755 directory, and systemd's manager checks the caller's credentials
itself. So mode 0777 is not a gap in itself, and the check is stricter than the platform.

**Effect.** No fresh install is possible on Ubuntu 22.04, which the product lists as supported.
Nothing was installed (`/opt/celikpanel` absent).

Evidence: `raw/install-u22-main.log`, `raw/socket-mode-u22-d12.txt`.

## Defect 2: Debian 12 setup pauses at the BIND activation

The install itself succeeded: get.sh terminal 46.7 s (provision 21.2 s), licence active.

Setup started at 19:53:43 with BIND (plan `can_start true`, 19 steps). Step 01-dns installed bind9
1:9.18.49-1~deb12u3, then failed at 19:54:16:

`activate BIND generation …: verify BIND vendor identity before enabling aliases: named.service does not resolve to the exact APT vendor BIND identity`

The agent rolled back. Setup is `waiting` at 01-dns with `server_setup_dns_agent_failed`, and the
screen gives the recovery command `recovery dns-switch-status --quiesced --request-id …`. The state
was unchanged at 20:02.

**Cause.** `internal/dnsunitidentity/identity.go` requires
`FragmentPath=/usr/lib/systemd/system/named.service`. Debian 12's bind9 ships the unit under
`/lib/systemd/system/` (dpkg owns that path; `/lib` is a symlink to `usr/lib`), so systemd reports
`FragmentPath=/lib/systemd/system/named.service`.

Everything else matches exactly: Id, Names, no drop-ins, `ExecStart=/usr/sbin/named -f $OPTIONS`,
`Alias=bind9.service`.

Ubuntu 22.04's bind9 also ships under `/lib`, so it would probably hit the same check after
defect 1. Not observed.

Evidence: `raw/dnsdiag-d12.log`, `raw/bind-unit-identity-d12.txt`, `raw/setup-state-d12-final.txt`,
`raw/single-d12.log`, `raw/plan-d12-main.json`.

## Times

| Guest | Provision | get.sh install | Setup |
|---|---|---|---|
| d12 | 21.2 s | 46.7 s, rc 0 | paused at 01-dns after 33 s (19:53:43 → 19:54:16) |
| u22 | 35.0 s | refused after 8.6 s, rc 1 | not started |

## Not done

- Sandbox capability levels on both images.
- Items 9 and 18 on Ubuntu 22.04.
- The PGDG PostgreSQL 18 preview on Debian 12.

The scripts are ready in `raw/kit/` (`R12U.sh`, `R12D.sh`, `probe.sh`, `g_pgsim.sh`) for a rerun
once both defects are fixed.

## Cleanup

- Removed: guests d12 and u22 with their state, the lab portal, the dist, the kit build and the work
  directory.
- labnet stopped (kept).
- The two new images are kept.
