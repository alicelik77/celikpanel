# v0.1.0-alpha.123

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.123.tr.md)

One rule for what a component needs, applied to removal first, and fixes for
installing on Debian 12 and Ubuntu 22.04. **Only the owner updates installed
panels, from the panel's own update screen.**

## Removing a component

- Removal asks one question: what needs this component? The removal window
  names every component, site, database, mailbox, app, certificate, VPN peer
  and quarantined file that uses it, each with a link to where it is moved or
  removed. While that list is not empty, removal is refused. There is no
  "force", and customer data is never removed along with a component.
- Small components that nothing else uses and that cannot work without it
  (for example phpPgAdmin with PostgreSQL, Roundcube with the mail server)
  are listed under **Will be removed together**, ticked. The server removes
  them first, then the component, as one operation. It goes on if the page
  is closed or the panel restarts; a page opened meanwhile shows the same
  progress instead of a lost-connection notice. Unticked, the component is
  not removed.
- When nothing needs it, the window says what stops and what keeps working
  (for example: without Roundcube there is no webmail, mail is still
  delivered).
- A Node.js version that an app such as n8n runs on is not removed; the
  window names the app. The server checks again when Remove is pressed.
- Plans, reseller limits and custom account rights that offered the removed
  component or version lose that entry automatically.
- The security emergency keeps its speed, but shows the same list before its
  removal step.
- An installed component whose required component is missing says so on its
  row: **Fail2ban** names the missing **Firewall engine** and has its
  **Install** button. A component that needs others first offers
  **Install … first** in its install window.

## Debian 12 and Ubuntu 22.04

- Installing no longer stops on Ubuntu 22.04 at the system manager check:
  its older systemd creates a socket with wider file permissions, which
  systemd itself still only lets the root user use.
- BIND is recognised on Debian 12 and Ubuntu 22.04, where its service file
  is still found under `/lib/systemd/system`.
- BIND also starts on Ubuntu 22.04, whose own BIND service file runs the
  name server in the background (the older way); only that exact file is
  accepted there.
- PowerDNS is offered only where it has been tested: as the main DNS server
  on Debian 13 and Arch Linux, and as the backup DNS server also on Ubuntu
  24.04 (unchanged from alpha.122: on Ubuntu 24.04 the main DNS server uses
  BIND).
  Elsewhere setup, joining by code and the DNS page say so and use BIND;
  nothing is changed on the server.

## Server setup

- When the panel's own database is briefly busy, a setup step now waits and
  tries again by itself instead of reporting a failure. Nothing is started
  twice. If the database stays busy for about ten minutes, setup stops that
  step and tells the owner the one command to run and how to continue;
  completed steps are kept.

## Known limits

- The install window still lists the packages that will be installed. A view
  that shows components instead is planned for alpha.124.
- Installing a component does not yet install what it needs in the same
  operation; the window offers **Install … first** for each.
- Debian 12 and Ubuntu 22.04: these fixes come from the lab round on
  alpha.121, but at the time of writing a complete install and setup on
  both systems with this release had not yet been repeated. That lab round
  comes next.

## Evidence

No schema change and no migration. Release sequence 122 → 123.
Go tests (whole module) and web tests pass, the web bundles stay within their
budgets. The lab round on fresh Debian 12, Ubuntu 22.04 and the usual systems
is still pending.
