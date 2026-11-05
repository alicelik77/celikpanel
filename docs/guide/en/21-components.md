---
id: components
title: Components: installing and offering software
roles: [admin]
topics: [components]
keywords: [install, uninstall, php, node.js, versions, bundle, repository, nginx, mariadb, postgresql]
---

**Components** lists the server's services, runtimes such as PHP and Node.js, and tools such as phpMyAdmin. Software comes from the operating system's own packages.

## One list {#views}

Every component the server offers is listed in its category, the installed ones first. An installed row shows its state with **Start**, **Stop**, **Restart**, **Repair and start** and **Manage**. A row that reads **Not installed** has **Install**, or says in one line why it cannot be installed here, for example **Conflicts with Nginx** beside Apache. Mail engines such as Postfix and Dovecot are installed with a mail setup at the top of the email category: their row says **Install with a mail setup**. DNS engines are chosen on the DNS page.

Every category is open on the first visit. **Collapse all**, **Expand all** or a category's own header folds and unfolds the categories; this browser remembers what you folded. Search finds any component. **Rescan** reads the server again, for example after you installed something yourself.

When another page says a component is missing, such as Fail2ban, ClamAV, vsftpd, Python, Node.js or WireGuard, its **Install** link opens that component's install window here. Nothing starts until you confirm.

## Installing and removing {#install}

**Install** shows what the component needs before anything changes, under **Requires**: each requirement is **Installed**, installed together with it first, or missing with the reason and where it is done (mail engines come with a mail setup, for example). Missing parts that can be installed here, such as MariaDB and Nginx for phpMyAdmin, are installed first in the same operation; if one step fails, what was installed before it stays. phpMyAdmin, phpPgAdmin and Roundcube choose here the installed PHP version they run on. Package names are not listed; the window asks you to confirm only a change of repository, which it names (for example PostgreSQL (PGDG)).

**Uninstall** asks one question: what needs this component? The window answers in three groups:

- **Must be moved or removed first**: sites, databases, mailboxes, apps, certificates, VPN peers, quarantined files, the panel firewall, or another component that is itself in use. Each one has a link to where you move or remove it. The removal is refused until the list is empty.
- **Will be removed together**: small components that nothing else uses and that cannot work without it, such as phpPgAdmin with PostgreSQL. They are ticked; the server removes them first, then the component, as one operation that goes on even if you close the page. Unticking them means the component cannot be removed.
- **What stops**: for example webmail when Roundcube goes. Mail keeps working.

Plans and accounts that offered the removed component or version lose that entry. Removing a database engine keeps its data.

**Ready bundles** install several components in a row, such as **Core Mail** or **Nginx + PHP-FPM**. A bundle with nothing installed yet says **Install**; one whose parts are partly here names what it adds, for example **Add phpPgAdmin**. If one step fails, **Continue** installs only what is still missing; nothing is removed.

## Versions {#versions}

PHP and Node.js keep several versions side by side; each site picks its own. Use **Versions** on the row to **Install version** or **Remove version**. If tools such as phpMyAdmin run on a version, the panel offers to move them first.

For PHP or PostgreSQL versions the system does not ship, the vendor's repository can be added for that component alone with **Enable with its confinement and updates**. Turning it off later removes nothing.

## Offering software {#offering}

Installing gives nothing to anyone. A customer can use a version or program only when you offer it and their plan includes it:

- **Offered to customers** on the PHP and Node.js pages: tick each version. Hiding one changes no running site.
- **Users**, tab **Offered**: every permission, program and application. See [Plans and rights](guide:plans-rights#offered).

There is no separate developer toolbox yet; developers use the PHP and Node.js versions and SSH where their plan includes it.

## When something is wrong {#problems}

A stopped service shows on its row and under **Needs attention** on the dashboard. **Repair and start** tries to repair and start it. A failed install or change says what was tried and what is in place now.
