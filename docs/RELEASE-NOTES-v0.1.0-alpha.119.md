# v0.1.0-alpha.119

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.119.tr.md)

A user guide inside the panel, uploaded programs and Python web apps that run
in a new site sandbox, developer tools for the administrator, and a stronger
sandbox for the AI apps of alpha.118. Every uploaded program is treated as
arbitrary code: the boundary is the sandbox, not the list of languages.
**Only the owner updates installed panels, from the panel's own update screen.**

## User guide

- A guide for every role (administrator, reseller, customer) is part of the
  panel. The "?" in the top bar opens the chapter for the screen you are on;
  /help shows the whole guide, with search.
- On first login a short tour shows the main parts of the screen. It can be
  skipped and replayed at any time ("Take the tour"); it is never forced.
- The administrator can view the guide as a reseller or a customer sees it.
  The role only chooses what is relevant; it is not a security boundary.
- The same guide, in English and Turkish, is published with the download
  portal (`/guide/`).

## Uploaded programs

- A customer can run their own compiled program (Go, Rust or any other program
  built for the server's processor) as their site. It is uploaded into the
  site's `app/bin` folder with the file manager or SFTP; CelikPanel builds
  nothing from source.
- The program runs as the site's own account, in the site sandbox, on one port
  that only the server reaches. Visitors reach it only over HTTPS, through the
  site.
- A deploy checks the program, starts it, proves the sandbox systemd actually
  loaded, waits for it to listen and answer, and only then switches the site
  to it. Every failure says why, who acts and what to do next.
- Nothing opens until the administrator offers uploaded programs after
  reading the risk summary, and the administrator can put them on a security
  hold.

## Python web apps

- Django, Flask, FastAPI and other WSGI or ASGI apps run on the same engine.
  The code goes into `app/src` with its `requirements.txt`.
- Only the distribution's own Python is used (Debian 12: 3.11, Debian 13:
  3.13, Ubuntu 22.04: 3.10, Ubuntu 24.04: 3.12, Arch: 3.14). The
  administrator installs it from Components and offers it per Python version.
- Each deploy builds a fresh environment from PyPI in a sandboxed build, as
  the site's account, while the running app keeps running. gunicorn and
  uvicorn are pinned by fingerprint. If the new version does not start, the
  previous environment is put back.
- `requirements.txt` lines that could fetch code from elsewhere (`-e`,
  links, local paths, other package indexes) are refused, and the screen names
  the line.

## Developer tools (administrator only)

- Go and Rust can be installed from Components, under "Developer tools
  (administrator only)", from the operating system's official packages.
- They are the administrator's own tools: they can never be offered to
  resellers or customers, and they are not available in the SFTP/SSH jail.

## A stronger sandbox for AI apps

- n8n and Open WebUI move to the new site sandbox: the panel's own socket and
  other sites' PHP sockets are hidden, the cloud metadata address is blocked,
  the app can listen only on its own port, and it cannot send mail through
  ports 25, 465 or 587, the local mail server included.
- The app's folder is attached to the sandbox by the server itself before every
  start, never by following a link the site could have planted. The app's
  settings stay in the protected server folder introduced by alpha.118's
  security fix.
- Apps installed with alpha.118 keep running as they are. The administrator
  moves them once with **Apply the new sandbox** on the AI apps page; each
  running app restarts once, is checked and comes back. A unit the owner
  changed by hand is kept and listed.

## New rights

Uploaded programs (`program_apps`) and Python apps (`python_apps`) are new
permissions, and `runtime:program` and `component:python:<version>` are new
offerings. All start closed until the administrator offers them. Developer
tools are never a right.

## Known limits

- A program's own outbound traffic is not measured against the subscription's
  traffic.
- Only mail ports 25, 465 and 587 are blocked; port 2525 and mail sent through
  HTTP mail APIs are not.
- Ubuntu 22.04 may run at the reduced sandbox level. The panel then names each
  missing protection, and the administrator must accept it before offering
  programs there.
- Python apps use ready-made packages (wheels) only by default; building
  packages from source is an administrator opt-in per offering.
- Go and Rust are the distribution's versions, which can be older than the
  newest release (Debian 12: Go 1.19, Ubuntu 22.04: Go 1.18).
- When a Python deploy fails, the rollback restores the previous environment,
  not the code in `app/src`.

## Evidence

Integrated tree (`release/alpha119`): Go build, vet and gofmt clean; the
`internal` and `cmd/panel` suites pass; the Agent suite shows only the 19
known guest-only DNS and mail failures that also fail on alpha.118, none new;
2412 web tests pass, the guide's generated files match their sources, and the
build stays within the bundle budgets; release-recovery fixtures extended to
schema 63; download portal and release sequence contracts pass (118 → 119).
Lab round on the five supported systems: pending.

<!-- LAB -->
