# v0.1.0-alpha.112

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.112.tr.md)

This release follows v0.1.0-alpha.111 and fixes only the first installation.
**It is a candidate, written on 2026-10-29; it is not published and nothing from
it is on an installed server. Only the owner updates installed panels, from the
panel's own update screen.** Branch `fix/fresh-install-kit-modes` from `main`
at `cb225a922`; previous release commit `34786df487ee5d4bc3c946ed947952086ead26c4`.

## Who should use it, and why

Anyone installing a new server. With alpha.111 the public install command
(`curl … https://celikpanel.net/get.sh && sh celikpanel-install.sh`) stopped at
"systemd services" with:

```
recovery runtime unavailable: runtime_unsafe_metadata
ERROR / HATA: fresh install independent recovery runtime could not be enrolled
```

Installed panels are not affected: updates from the panel's own screen already
restore the file modes, and nothing changes for them. A server where the
command failed is reset and installed again with this release.

## What changed

- `get.sh` downloads and extracts with `umask 077`, so the release files arrived
  as 0700/0600. Since alpha.81 the first install enrolls the independent
  recovery, firewall and mail-renewal kits, which require their published
  0755/0644 modes and refuse anything else. The first-install branch now
  restores the archive's modes (`chmod -R go+rX`) inside the root-only,
  link-free staging tree after all checksum and signature checks, and keeps the
  release root at 0700. The update branch already did the same in
  `bootstrap-prebuilt-update.sh`; extraction itself is unchanged, because the
  update branch requires the 0700 release root it produces.
- `deploy/test-fresh-install-entry-boundary-contract.sh` runs the real `get.sh`
  against a signed test portal; it now also checks the modes the installer
  receives (release root 0700, directories 0755, executables 0755, other
  files 0644).

## Why earlier testing missed it

The four alpha.111 lab runs started `install.sh` directly from an extracted
release, not through `get.sh`. The servers installed in September were
installed before alpha.81 and later updated from the panel. The test above now
covers the command the owner types.

## Evidence

- The real alpha.111 archive, extracted exactly as `get.sh` does: the recovery
  kit is refused with `runtime_unsafe_metadata` (the owner's error); after the
  change it is accepted by the same verifier the installer uses.
- CI runs the entry-boundary contract on a clean runner.

Sequence 112, schema 46, no other code change. Open: no full lab install
through the public command; the first install on the owner's servers is the
measurement. No P0 item is closed or advanced.
