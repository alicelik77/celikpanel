# AI and automation apps

*alpha.118 · [Türkçe](AI-APPS.tr.md) · Rights: [PLANS-AND-RIGHTS.md §4.6](PLANS-AND-RIGHTS.md)*

Owner, November 3, 2026: "hermes, openclaw gibi daha nice AI ile ilgili yazılımlar da olsun",
"paperclip, n8n gibi programlar da seçeneklerde olabilir" and "güvenlik önemli". A customer can
run an AI or automation application on one of their domains, next to the PHP app catalog.
Security comes first: an app is closed until the administrator offers it, it runs as the site's
own account in a systemd sandbox, it listens only inside the server and it opens only over HTTPS.

This document describes what is built in alpha.118. It is not a claim that the apps were tried on
every supported system: one lab round for the release is still to come.

---

## 1. The catalog

Versions and hashes were read from the official registries on November 3, 2026. Every app is
pinned to one exact version; an update is a new CelikPanel release with a new pin and an explicit
"Update to version X" button. Nothing updates itself.

| App | What it is | Source | Licence | Pinned version | Runtime |
|---|---|---|---|---|---|
| **n8n** | Workflow automation (connect services, AI steps, webhooks) | npm `n8n`, github.com/n8n-io/n8n | Sustainable Use License 1.0 | 2.46.4, integrity `sha512-Ek/AlWCx…Xr1Wg==` | Node.js 24 |
| **Open WebUI** | A chat interface for AI models (OpenAI-compatible providers) | PyPI `open-webui`, github.com/open-webui/open-webui | Open WebUI License (BSD-3 with a branding clause) | 0.12.5, wheel sha256 `74fafb4e…19d3fd` | Python 3.11 or 3.12 |
| OpenClaw | AI agent gateway | npm `openclaw`, MIT | — | not offered (see §6) | Node.js 24/26 |
| Hermes Agent | AI agent by Nous Research | PyPI `hermes-agent`, MIT | — | not offered (see §6) | Python |
| Paperclip | AI agent company platform | npm `paperclipai`, MIT | — | not offered (see §6) | Node.js 24 + PostgreSQL |

**Licences the administrator reads before offering:**
- n8n's Sustainable Use License allows use "only for your own internal business purposes or for
  non-commercial or personal use"; offering n8n itself to others as a paid service is restricted.
  The admin entry shows this note. **n8n is closed to resellers** (owner's decision, 3 Nov 2026,
  until the licence question is answered): see §3.
- Open WebUI's licence keeps its branding in place unless the deployment has at most 50 end users
  in 30 days, written permission or an enterprise licence.

## 2. Where it can run

CelikPanel adds no package source (no NodeSource, no deadsnakes, no Docker, no `curl | sh`).

| System | Node.js (distribution) | Python 3 | n8n | Open WebUI |
|---|---|---|---|---|
| Debian 12 | 18 | 3.11 | with the panel's Node.js 24 | yes (needs `python3-venv`) |
| Debian 13 | 20 | 3.13 | with the panel's Node.js 24 | **no** (needs 3.11/3.12) |
| Ubuntu 22.04 | 12 | 3.10 | with the panel's Node.js 24 | **no** |
| Ubuntu 24.04 | 18 | 3.12 | with the panel's Node.js 24 | yes (needs `python3-venv`) |
| Arch | 26 / lts 24 | 3.14 | yes | **no** |

n8n needs Node.js 24 (`engines: >=24.0.0`; its isolated-vm dependency too). No supported
distribution except Arch ships it, so n8n uses the Node.js runtime the administrator installs
under Services → Node.js: the official nodejs.org tarball, checked against the official
SHASUMS256.txt before it is unpacked (the same runtime Node.js sites use). If no Node.js 24 is
installed, the screen says so and who installs it. Where the system's Python is outside 3.11-3.12
Open WebUI is shown as not available on that system, with the found version.

## 3. Rights and offering

- Permission **AI and automation apps** (`service_apps`) and one offering per app, `app:<id>`.
- **Closed until offered.** The administrator opens Add-ons → catalog tab → *AI and automation
  apps*, reads the risk summary, ticks that they read it and offers it, with memory, CPU and
  process limits for every install. The request carries the summary's digest; a release that
  changes it closes the offer again until it is read and offered once more.
- Unlimited plans take an offered app; a reseller or plan can only leave it out. The server
  checks every layer when an app is installed or updated. Stopping, the log and removal are never
  refused, so a narrowed right never strands a running app.
- **Closed to resellers by licence (n8n).** An app whose catalogue entry carries
  `reseller_closed` is never available through a reseller. The administrator may still offer it
  and give it to the administrator's own customers. The server refuses it, with the licence as
  the reason (`SERVICE_APP` / `licence_reseller`, HTTP 409), when it would be written into a
  reseller's rights, a reseller plan, a plan a reseller owns or the rights of a subscription in a
  reseller's tree, and when it would be installed or updated in such a subscription, whoever
  asks (an administrator acting for that account too). Taking an item out of "everything
  offered" never adds it for a reseller. Stopping, the log and removal are never refused. The
  reseller and their customers read why on the app's row (the licence; the administrator
  decides). Open WebUI is not affected.
- **Security hold** (admin): stops every install now, keeps them from starting at boot, refuses
  installs, updates and starts. Lifting the hold starts nothing. Data is never removed.
- Additional users (team members) do not see the tab.

## 4. How one install works

The customer picks a domain that has a website and an active certificate (a separate subdomain is
recommended: the domain then shows the app instead of its website; the website's files stay and
come back when the app is removed). They enter the app's first administrator (e-mail and a
password of 12-128 characters, so no visitor can claim the app first) and, for Open WebUI,
optionally an OpenAI-compatible key and address.

| Step | Who | What |
|---|---|---|
| prepare | Agent (root) | Downloads the pinned file to its private state folder and checks its sha512/sha256 against the pin; a mismatch is discarded. |
| | site user | Creates `<site home>/apps/<app>/{dist,data,config}`, writes the verified file and `config/app.env` (0600). Root never writes inside the site's folder. |
| installing | site user, transient unit `celiksvcinst-<id>` | `npm install` of the verified tarball (then `npm audit signatures`) or `python -m venv` + `pip install` of the verified wheel, inside the same sandbox as the app. |
| activate | Agent | Writes `/etc/systemd/system/celiksvc-<id>.service`, enables and starts it. |
| verify | Agent → Panel | Proves that a process **of this unit** listens on `127.0.0.1:<port>` and on no other address. A unit that listens elsewhere, or a port another process holds, is stopped and not published. |
| publish | Panel | The domain's site becomes type `service_app`: nginx proxies to `127.0.0.1:<port>`, HTTPS forced. Without an active certificate the domain never serves the app over HTTP. |
| finalize | site user | Earlier versions' folders are removed; Open WebUI's first password leaves the environment file. |

Every step is a durable operation (D-025): a Panel restart continues it from its step; the same
operation identity never starts the install twice; the screen's polling only reads. The screen
names each failure as a verified failure, an unmet prerequisite or an unknown result, with who
acts and the next step (D-024).

## 5. The sandbox

Each app's unit (and its install unit) runs with:

- `User=` the site's own account (proved: its home is exactly the site's home); never root, no
  supplementary groups, no Docker.
- `NoNewPrivileges`, empty `CapabilityBoundingSet`, `RestrictSUIDSGID`, `LockPersonality`,
  `RestrictNamespaces`, `RestrictRealtime`, `SystemCallFilter=@system-service`,
  `SystemCallArchitectures=native`.
- `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `PrivateDevices`, `ProtectKernel*`,
  `ProtectControlGroups`, `ProtectClock`, `ProtectHostname`, `ProtectProc=invisible` (no other
  user's processes).
- `TemporaryFileSystem=/var/www/celikpanel:ro` with only the app's own folder bound back in:
  every other site and subscription disappears. `ReadWritePaths` is only the app's folder.
  CelikPanel's own configuration and state folders are inaccessible.
- `MemoryMax`, `MemorySwapMax=0`, `CPUQuota`, `TasksMax` from the administrator's limits;
  `UMask=0077`.
- `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK`: it can call the internet (the
  apps need AI providers), and it can reach services that listen on 127.0.0.1 of the server, as
  any site's PHP code can (§7).

What CelikPanel turns off with each app's own supported settings:

- **n8n:** `NODES_EXCLUDE` = Execute Command and Local File Trigger (the nodes that act on the
  host itself); community packages off; Python in the Code node off; `N8N_BLOCK_ENV_ACCESS_IN_NODE`;
  file nodes limited to `data/files`; Git hooks off; telemetry and version checks off; the task
  broker on 127.0.0.1 and its own port; the owner set by environment
  (`N8N_INSTANCE_OWNER_MANAGED_BY_ENV`, a bcrypt hash computed by the Panel - the password itself
  never reaches the server). The JavaScript Code node stays, in n8n's own task-runner sandbox with
  no built-in or external modules.
- **Open WebUI:** listens on `--host 127.0.0.1`; code execution and the code interpreter off;
  `SAFE_MODE` (functions off); sign-up off and new users pending; community sharing, Ollama,
  update checks, telemetry off. These are "persistent" settings: the app's own administrator (the
  customer) can turn them back on inside the app; they still run only inside the sandbox.
  What stays: chat with the providers whose keys are entered, documents, web reading on request,
  Python "tools" that an Open WebUI admin adds (run in the app's process, inside the sandbox).

Secrets (provider keys, Open WebUI's first password, n8n's encryption key) live only in
`config/app.env`, 0600, owned by the site's user. They are not stored by the Panel, not written to
its audit log (only which settings changed), never shown again, and every log line the panel
shows is redacted on the server first (the file's own values, and anything that looks like a key,
token, password or bcrypt hash).

## 6. Not offered in this release

On a shared hosting server an agent that runs shell commands and reads untrusted content
(prompt injection) is a risk to the other customers even inside the sandbox. The release preparer
decided (with the owner informed) that these are not offerable to resellers or customers in
alpha.118:

- **OpenClaw** (MIT, npm `openclaw` 2026.10.10): a gateway whose tools "run on the host"; it needs
  install scripts (native `node-pty`), describes itself as beta. Would need an
  administrator-only, single-owner mode.
- **Hermes Agent** (MIT): the only documented installer is `curl … | bash`, which CelikPanel does
  not run; the PyPI release (0.19.0) lags the GitHub release (0.22.6) and its owner on PyPI could
  not be verified; its API server gives full terminal access.
- **Paperclip** (MIT, npm `paperclipai` 2026.1023.0): drives Claude Code, Codex and bash on the
  host; its default mode has no sign-in; it bundles an embedded PostgreSQL (the account's
  PostgreSQL right would be the way).

They are listed in the catalog with this reason; the server refuses to offer or install them.

## 7. Known limits

- **Dependencies are not hash-pinned.** The top-level file is pinned and checked twice (by the
  Agent and again by the install step). n8n publishes no lockfile (`_hasShrinkwrap: false`): its
  ~165 dependencies are resolved by npm within n8n's ranges at install time; npm checks each
  tarball against the registry's integrity and `npm audit signatures` checks the registry
  signatures. Open WebUI pins most direct dependencies exactly (`==`), but pip resolves them from
  PyPI without `--require-hashes` (a full hash lock for each Python version is a later step).
- **n8n is installed with npm's `--force` (alpha.118 lab round 2).** n8n-nodes-base 2.46.x lists
  the Alpine (musl) builds of `@napi-rs/canvas` and `@napi-rs/image` as ordinary dependencies, not
  optional ones, so a plain `npm install` stops with `EBADPLATFORM` on every glibc server (Debian,
  Ubuntu, Arch). `--force` is npm's only switch for that; it also stores those unused binaries
  (a few MB) and lets npm continue past peer-dependency conflicts it would otherwise refuse.
  Integrity checks and `npm audit signatures` still apply. Only n8n gets it
  (`NPMPlatformDepsRequired` in `internal/serviceapps/catalog.go`); drop it when n8n makes those
  dependencies optional again.
- **Install scripts run.** `--ignore-scripts` would break both apps (n8n: sqlite3 and isolated-vm
  native modules; sqlite3's script downloads a prebuilt binary from its GitHub release without a
  pinned hash). They run as the site's user inside the sandbox. Without a prebuilt binary they
  need build tools (python3, make, g++), which the administrator installs.
- **Size.** Open WebUI pulls PyTorch and other large packages: several GB per install, counted in
  the subscription's disk. Its first start downloads an embedding model from Hugging Face.
- **Loopback.** An app can reach services on 127.0.0.1 (as PHP can). A port collision is detected
  and stops the app; it does not prevent another tenant's process from listening on a free port.
- **Plan limits.** Memory, CPU and process limits come from the administrator's per-app defaults,
  not yet from a plan.
- **D-043.** The administrator's security hold covers apps; the full security-emergency workflow
  (affected-customer notices, password re-entry, version withdrawal) is not wired to apps yet.
- **Owner independence.** The unit and environment file are plain systemd configuration and keep
  running without the Panel. n8n uses the Panel's Node.js runtime under `/opt/celikpanel/runtimes`
  (as Node.js sites do); removing that folder stops n8n.
- **WebSockets** pass through nginx; `proxy_read_timeout` is 60 s and the default upload limit is
  nginx's 1 MB; larger uploads need an owner include in the site's nginx include folder.
- Subscription deletion with an installed app: the domain's deletion is refused until the app is
  removed; subscription-level deletion paths were not changed.

## 8. Running and stopping it without the panel

On the server, as root (`<id>` is shown on the app's tab):

```
systemctl status celiksvc-<id>       # is it running
systemctl stop celiksvc-<id>         # stop now
systemctl disable celiksvc-<id>      # do not start at boot
systemctl start celiksvc-<id>        # start again
journalctl -u celiksvc-<id> -n 100   # its log
```

Own settings go in `/etc/systemd/system/celiksvc-<id>.service.d/*.conf` (CelikPanel never
writes there). An edit to the unit file itself is kept: CelikPanel refuses to overwrite it and
says so. The app's files are in `/var/www/celikpanel/subscriptions/<sub>/sites/<domain id>/apps/<app>/`.
