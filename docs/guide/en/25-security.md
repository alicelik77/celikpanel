---
id: security
title: Security: firewall, Fail2ban, emergencies and malware
roles: [admin]
topics: [security]
keywords: [firewall, nftables, fail2ban, unban, audit, emergency, withdraw, clamav, malware, quarantine, hold, updates]
---

**Security** shows the firewall, the system's security updates, Fail2ban and a read-only **Security audit**.

## Firewall {#firewall}

The firewall closes every incoming port except the panel's, SSH's and those of running services; installing or removing a service changes them. **Turn on** asks first and applies at once. **Save for reboot** keeps the policy after a restart. **Turn off** returns every port to the system's default.

SSH is never closed. If SSH is installed but not listening, turning on is refused until you start it. Without SSH you must confirm that you would need your provider's console if you lost the panel.

## Fail2ban and the audit {#fail2ban}

Fail2ban blocks addresses that keep failing to sign in. **Manage Fail2ban** shows **Jails** and **Banned IPs** with **Unban**; its configuration can be read but not yet edited here.

The **Security audit** checks the firewall, SSH sign-in, a pending reboot, update signatures and the panel's certificate. It only reads; **Run again** repeats it.

## Security emergency {#emergency}

PHP, Node.js, MariaDB and PostgreSQL are the programs customer sites run on directly, so only they have these steps. When one of them has a security hole and no fix yet, open **Security emergency** at the bottom of its own page (**Components**, then **Manage**), or follow its link in **Security emergency** on this page. While an emergency is open, its row on **Components** says **Security emergency open**. There are four steps, lightest first:

1. Withdraw (step 1): nobody can choose it for anything new; sites using it keep running. It can be undone.
2. **Emergency stop (step 2)**: stopped and locked; affected sites show a maintenance page.
3. **Move the sites (step 3)** to another version; sites that cannot move are listed.
4. **Emergency removal (step 4)**: the package goes. Files, databases and mailboxes stay.

Steps 2 to 4 list who is affected and ask for a reason, the typed name and your password. Customers and resellers are told in the panel and by email. Once a fix is installed, **Re-enable (patch installed)** brings everything back. No step deletes customer data.

For the **AI and automation apps** offered from **Store**, **Security hold** stops every install at once without removing data; after **Lift the hold**, each owner starts theirs again.

## Malware scanning {#malware}

Install ClamAV from **Components**. A domain's **Malware scan** section offers **Scan this site** and **Scan all sites of the subscription**; there the administrator can also tick **Scan every site weekly (all subscriptions, at low priority)**.

Nothing is deleted automatically. **Quarantine** moves a found file aside, **Put back** returns it, and **Delete for good** works only from quarantine. The customer, its reseller and the administrators are told. With signatures older than 7 days, "nothing found" is not a full all-clear.

## System updates {#os-updates}

**Security updates** lists pending updates and only reads. On Debian and Ubuntu, **Automatic or manual updates** shows the one file it will write; confirm with **Write the file**. It never restarts the server. On Arch it gives a step-by-step guide.
