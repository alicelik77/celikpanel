---
id: components
title: Components: installing and offering software
roles: [admin]
topics: [components]
keywords: [install, uninstall, php, node.js, versions, bundle, repository, nginx, mariadb, postgresql]
---

**Components** lists the server's services, runtimes such as PHP and Node.js, and tools such as phpMyAdmin. Software comes from the operating system's own packages.

## Installed and Catalog {#views}

**Installed** shows what runs, with its state and **Start**, **Stop**, **Restart**, **Repair and start** and **Manage**. **Catalog** shows what can be installed. **Rescan** reads the server again, for example after you installed something yourself.

## Installing and removing {#install}

**Install** shows what will be installed before anything changes. **Uninstall** shows **What is removed**, **Affected on this server** and **What stays**, and is refused while something still depends on the component. Removing a database engine keeps its data.

**Ready bundles** install several components in a row, such as **Core Mail** or **Nginx + PHP-FPM**. If one step fails, **Continue** installs only what is still missing; nothing is removed.

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
