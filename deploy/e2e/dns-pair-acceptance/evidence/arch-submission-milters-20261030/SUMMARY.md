# Mail filters on every submission service (D3) — evidence (2026-10-30)

Branch `fix/arch-submission-milters` (6f639c72c, 7a05f2586, 0399ef653,
bf1a5e912), merged into `release/alpha113`. Native check built from
0399ef653 (acceptance-licence dist labelled `v0.1.0-alpha.113`,
`dist.json`), installed through the owner's `get.sh` command as in final
acceptance run 2 (`get.sh.diff`: pinned key digest only). Lab only. Secret
scan clean (186 files, `secret-scan.json`); full logs in
`arch-submission-milters-evidence.tar.gz` (SHA-256
`a31a112c2fae31f5518a52d68382c90f3e84d9fa222925b8585887eb3d99dbcd`).

**What changed.** Milters are numeric (`inet:127.0.0.1:8891` OpenDKIM,
`inet:127.0.0.1:11332` Rspamd); submission and smtps take the chroot field
of the distribution's own `smtp/inet` entry (Debian/Ubuntu `y`, Arch and
upstream `n`); the Panel-start repair (`Agent.WireMailFilters`) moves
entries still exactly as alpha.113 wrote them and rewrites milter lists that
hold only CelikPanel's entries, keeping and naming owner-changed ones.

**Distro config matrix** (`matrix-*-results.tsv`). The postfix validator now
runs a real OpenDKIM milter on every inet address a case names, fails on any
milter warning, and copies chroot files only where the distribution's unit
does (copying them everywhere had hidden D3).

| Run | Result |
|---|---|
| fixed, debian-13 | 83 pass |
| fixed, ubuntu-24.04 | 83 pass |
| fixed, arch | 79 pass, 1 warn (stock nginx), 2 n/a |
| baseline (alpha.113 code), arch | fails exactly `postfix.submission` and `postfix.everything-together` with the acceptance's line |

`postfix.submission-upgrade` replays the Panel-start repair: pass on all
three.

**Native check (Arch and Debian 13 standalone, setup 18/18).**

| Check | Arch | Debian 13 |
|---|---|---|
| smtp / submission / smtps chroot | n / n / n | y / y / y |
| Milter warnings, all boots | 0 | 0 |
| "not a valid FQDN" lines | 0 (an info line waits for the mail identity) | 0 |
| File manager missing path / existing name | 404 / 409 with guidance | same |
| Site DKIM: Generate key, Add to DNS | signing on, TXT published, card ok | same |
| 587 send, IMAPS read | ok | ok |
| DKIM signature on 587 mail and on PHP `mail()`, verified | yes | yes |
| Rspamd scanned the 587 mail (its log) | yes | yes |
| After a power cut: signed and verified, 0 warnings | yes | yes |

**Open here.** A site domain added after setup got no DKIM key until the
owner pressed "Generate DKIM key" (addressed separately); deleting a domain
left its DKIM key behind; downloading a missing file opened a tab with the
JSON answer.
