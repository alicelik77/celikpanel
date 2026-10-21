# v0.1.0-alpha.98 (candidate, 2026-10-22)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.98.tr.md)

This is the release that follows v0.1.0-alpha.97. It is a candidate: when this was
written it had not been tagged or published, and nothing from it was on an installed
server. Written on 2026-10-22, on the branch `feat/alpha98-menu-layout`, from `main`
after the publication record of alpha.97.

## Who should update, and why

Everyone who runs alpha.97 or an earlier release, once this release is published.
On 2026-10-21 the owner asked whether everything in the Settings submenu was really
about settings, and said that every operation's place must be right, that nothing
should sit in a senseless, unrelated place, and that everyone should find what they
look for easily. This release moves each page to the place its name says (D-037).
Nothing else changes: the same pages, the same reads and changes, the same roles.
Only the owner of a server updates it, from the panel's own update screen.

## What changes on screen

**Where each moved thing is now.**

- Server security is now **Server → Security**. It answers "is this server safe?" in
  one place: the firewall with its settings (turn on or off, save for reboot, open
  ports, the SSH line), Fail2ban's state with a link to its component page, and the
  read-only check of what the server exposes. The firewall box used to open from the
  one-line firewall bar on Components, and the check was Settings → Server security.
- DNS infrastructure is now **Server → DNS and peer servers**: the shared
  nameservers, this server's role, its peer server and DNS replication, as they
  were.
- System databases are now **Server → Maintenance**: inspect and safely maintain the
  SQLite files CelikPanel and installed services keep, as they were.
- Account security is now **My account**, in the menu at the top right (the user name),
  first item. It holds the two-factor authentication panel and a Password card that
  opens the same password dialog the menu used to open.
- The Settings section called "Panel HTTPS" is now **Panel address and HTTPS**.

**Settings keeps four things:** Panel address and HTTPS, License, CelikPanel updates
and Server setup. They are choices made once or rarely that decide how the panel
behaves. No administrator-wide security rules section was added, because no such
setting exists.

**Server now holds:** Components · DNS and peer servers · Monitoring · Security ·
Maintenance · Import · Audit log. **Hosting is unchanged:** Domains, Databases,
Users, Store, VPN.

**Where the second thoughts landed.** A first draft moved Import and Server setup;
both stay. Import stays under Server (a tool used rarely, looked for among server
tools), and Server setup stays in Settings (run once, then rarely, to extend; the
server's purpose is a configuration choice). Store and VPN stay under Hosting
because resellers and customers use them.

**Resellers and customers no longer see an empty Settings.** Their Settings had one
section, their own account. They now have **My account** in the menu and no Settings
in the rail. An additional user (team member) never had Settings or the account
section; they keep "Change password" in the menu.

**Old links and bookmarks keep working.** `/settings?section=account` goes to
`/account`, `?section=security` to `/security`, `?section=dns` to
`/dns-infrastructure` and `?section=system-databases` to `/maintenance`, without an
extra step in the Back button's history. `/settings` for a reseller or a customer
goes to My account. The panel's own links (the start guide, Domains' "DNS needed",
the Add domain dialog, Components, the server setup's next step "Open DNS and peer
servers", the setup recovery links) and the guidance texts now name the new place:
"Server → DNS and peer servers" or "Server → Security", not "Settings".

**Calm where it matters.** A healthy firewall is still one calm line on Components;
its link, now labelled "Manage on Security", opens the firewall on the Security page. A firewall that is off,
unreadable, without an engine or needing attention still shows its full box on
Components, because that is where you are when it matters. The Security page shows
Fail2ban as one of: running; installed but not running (start it on its component
page); installed, running state not read yet; not installed (install it from
Components); not checked yet. A failure to read says so and offers Retry. When the
server is busy, the page shows the same pause as Components ("Server changes are
temporarily unavailable.") and lifts it by itself, with nothing to reload.

## What changes underneath

- **The interface only.** The rail, three new pages (`/dns-infrastructure`,
  `/security`, `/maintenance`), My account (`/account`), a Settings with four
  sections, the redirects, and the links and texts that named the old places. The
  icons come from the lucide set already in the boot bundle. The personal menu is a
  disclosure: `aria-expanded`, Escape closes it and returns focus to its button.
- **The password dialog** is fetched when it is opened, not on every page load, and
  its field texts moved from the shell catalogue to the settings screen part
  (`account.password.*`).
- **Guidance action addresses on the server.** Every guidance action that pointed at
  `/settings?section=dns` (the DNS engine workflow, DNSSEC, DNS-only children,
  import, remote DNS authority, DNS setup) now points at `/dns-infrastructure`; the
  panel certificate's missing-contact-email action points at `/account`. Two English
  fallback messages that named Settings or DNS infrastructure settings now name
  Server → DNS and peer servers. These are strings in answers; an older Panel's
  links still work because the browser redirects them.
- **The setup gate** keeps `/account`, `/security`, `/dns-infrastructure` and
  `/maintenance` reachable while setup is required, as `/settings` was, and the
  offline recovery shell's route list includes them.
- **The public portal's links** (`download-portal/index.html`, `technical.html`,
  `security.txt`, `SECURITY.md` and the portal contract test) now point to
  `github.com/alicelik77/...` after the owner renamed the GitHub account from
  `celikbros` to `alicelik77` on 2026-10-21 (the old addresses redirect
  permanently; the feedback repository is `alicelik77/celikpanel-feedback`). They
  take effect when the portal is next published. The recorded evidence files under
  `deploy/e2e/**/evidence/**` are historical and were not touched.

## What does not change

- No API, Agent, updater, schema, role, polling or component behaviour. The same
  components, the same reads, the same tests' meaning. Roles of every page and API
  are unchanged; only the place moved.
- The updater's mutation path (`update.sh`, `get.sh`, the system update worker,
  rollback and recovery) is not touched, so no native record is made, by the owner's
  instruction of 2026-10-16.
- Schema 44, no migration. Nothing persisted changes.
- Hosted websites, mail, DNS, databases, scheduled jobs and certificate renewal do
  not depend on where a page is.
- Installed panels are updated only by their owner, from the panel's own update
  screen.

## What the owner reported

On 2026-10-22 the owner sent a screenshot of Boston on alpha.97. Components shows
real versions: PowerDNS 4.8.3, Nginx 1.24.0, PostgreSQL 16.15, certbot 2.9.0,
Fail2ban 1.0.2, nftables 1.0.9, cron 3.0pl1 and Node.js 24.21.0. This is the first
measurement of D-036 on an installed server. Frankfurt's alpha.97 state was not
reported, so the open item "read installed package versions for every component"
stays open until the owner sees them there too; a kept reading and a busy package
manager have not been seen on a real server.

## Known gaps

- **Nothing was measured on an installed server.** The new layout has been exercised
  by component and mounted tests only; the owner has not yet seen it on Boston or
  Frankfurt.
- **The critical boot path has 0.25 KiB of its budget left** (360.75 of 361 KiB raw;
  1.8 KiB were left after alpha.97). The ceiling was not raised. The next addition to
  the boot path must move something out of it first.
- **alpha.98 back to alpha.97.** The new addresses (`/security`, `/maintenance`,
  `/dns-infrastructure`, `/account`) are unknown to alpha.97, so a bookmark to one of
  them lands on alpha.97's Dashboard. Nothing is lost and nothing needs repairing;
  alpha.97 has its own menu and its Settings sections at the old addresses.
- **Frankfurt's state** (its alpha.97 update, and its update after the alpha.95
  stop) was not reported.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; a candidate is not
  acceptance.

## Evidence

Everything below was run on the candidate tree on 2026-10-22 in a development
environment; none of it was run on an installed server.

- Go: `go test ./cmd/panel/ ./internal/...` (-timeout 45m) in the
  `CelikPanel-S2-Debian` WSL guest: 7156 passed, 0 failed, 44 skipped.
- Web: `npm test` 1691 passed (1675 on alpha.97). The new tests are in
  `web/tests/menu-layout-alpha98.test.mjs`: the rail per role, the redirects, every
  web link and every Go guidance action resolving to a route and to an existing
  Settings section, the personal menu mounted, Fail2ban's states, the new labels in
  English and Turkish, and each new page with its header and the content it
  received. `npm run build` is within its budgets: the critical boot path 360.75 of
  361 KiB raw and 107.68 of 110 KiB gzip (359.20 raw on alpha.97).
- Release-recovery harness (it mirrors the update card's texts and outcome logic,
  and reads the catalogue parts this release changed):
  `python3 -m unittest discover -s deploy/e2e/release-recovery -p 'test_*.py'` in a
  fresh clone in the `archlinux` WSL guest: 1109 tests, OK (1 skipped). In the same
  clone `deploy/test-release-sequence-policy.sh` passed (97 -> 98).
- Deploy contracts: `deploy/test-download-portal-contract.sh` (with the portal's new
  GitHub links) and `deploy/test-signed-release-manifest-contract.sh` passed in the
  `CelikPanel-S2-Debian` guest.
- Browser batch 23 (mock server, loopback only): the rail for an administrator, a
  reseller and a customer (and the phone drawer), each new page, Settings with its
  four sections, the personal menu, every old `/settings?section=` address and
  `/settings` as a customer, and the Components firewall line followed to
  `/security#firewall`, in English and Turkish, light and dark, at 1440 and 390 px
  (30 states on the desktop, 27 on the phone). Every redirect landed on its page
  without a new history entry; no raw key, no sideways scrolling. The first look
  found a running Fail2ban drawn as "installed but not running" (the stored record
  says systemd's "active (running)"), the firewall line's link still labelled
  "Settings", and the Turkish audit rows saying "firewall" beside "Güvenlik duvarı";
  all three were corrected. The second look (Security, Components, the redirects and My
  account in both languages at both widths, both themes sampled): Fail2ban reads
  "Running.", a stopped one "Installed but not running", a missing one is calm; the
  link reads "Manage on Security"; no "firewall" on a Turkish page; nothing new.
  Left as they were (moved as is, not new): the System databases card's "Open panel
  settings" link, the sideways tab strip of Settings on a phone, and buttons under
  44 px on a phone in the moved cards.

Component and mounted tests do not establish native behaviour.

## How to update

Install this release only through CelikPanel's update interface (Settings →
CelikPanel updates). Publishing it does not update installed servers; only the owner
of a server starts its update, from the panel's own update screen.
