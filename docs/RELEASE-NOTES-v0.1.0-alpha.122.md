# v0.1.0-alpha.122

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.122.tr.md)

A small release with one Components list, after the owner's question on
Boston (alpha.121): "Why can't phpPgAdmin and phpMyAdmin be installed
separately?" **Only the owner updates installed panels, from the panel's own
update screen.**

## Components

- One list instead of the **Installed** and **Catalog** views. Every component
  the server offers is shown in its category, the installed ones first, and
  every category is open on the first visit. **Expand all**, **Collapse all**
  and each category's header still fold the list; the browser remembers what
  was folded. Search finds every component.
- A component that is not installed reads **Not installed** with **Install**,
  which opens its own install window. phpPgAdmin, phpMyAdmin, Fail2ban, ClamAV,
  vsftpd, Redis, Valkey, Memcached, Netdata, Node.js, Go and Rust are no longer
  hidden.
- One that cannot be installed here says why in one line: **Conflicts with
  Nginx** beside Apache (Exim beside Postfix, PowerDNS beside BIND), or not
  offered on this system. Postfix, Dovecot, Rspamd and Roundcube point to the
  mail setup at the top of the email category; DNS engines to the DNS page.
- A ready bundle whose parts are partly installed names what it adds, for
  example **PostgreSQL + phpPgAdmin · Add phpPgAdmin**. One with nothing
  installed yet still says **Install**.
- Where another page says a component is missing (file access: vsftpd and
  Fail2ban; malware scan: ClamAV; AI apps: Node.js; Python apps: Python; VPN:
  WireGuard; Security: Fail2ban), the administrator gets an **Install** link
  that opens that component's install window. Nothing starts until it is
  confirmed.

## Known limits

- The site creation and hosting type screens, and the WordPress tools, still
  say "install it from Components" without a direct link.
- When Python is installed but its `venv` module is not, the Python link may
  say Python is installed instead of opening an install.

## Evidence

No schema change. Web tests pass within the bundle budgets; release sequence
121 → 122.
Screen-only release: no Agent or Panel server change, so no lab round; checked by the automatic checks. Those checks themselves were made independent of GitHub's `/run` mount flags: the three contract tests that stage executables under `/run` now use a private tmpfs; this had stopped the alpha.121 package build once and the alpha.122 checks once.
