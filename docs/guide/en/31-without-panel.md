---
id: without-panel
title: Running the server without CelikPanel
roles: [admin]
topics: []
keywords: [independence, native, systemd, nginx, postfix, bind, powerdns, cron, certbot, license, outside]
---

CelikPanel operates the server for you; it does not own your services. They are the operating system's ordinary services with their usual configuration files, and you can look after them with the standard tools.

## What keeps running {#keeps-running}

When the panel is stopped, unreachable or waiting for its license, these keep working:

- websites (nginx, PHP-FPM) and Node.js applications;
- mail (Postfix, Dovecot) and webmail;
- DNS (BIND or PowerDNS), including the copy to a backup DNS server;
- databases (MariaDB, PostgreSQL);
- scheduled tasks, which are ordinary crontabs of each site's system user;
- renewal of site certificates, by the system's own certbot timer.

## What still needs the panel {#needs-panel}

- renewing the panel's and the mail server's own certificates, and site certificates from older releases until they are issued again;
- restoring the firewall policy at boot;
- sending scheduled remote backups.

Removing CelikPanel from a server is not supported yet. Do not delete its files by hand.

## Changes you make yourself {#own-changes}

CelikPanel notices your own edits and does not silently overwrite them:

- A site's configuration file shows **Edited outside CelikPanel** on its **Configuration file** page: choose **Take CelikPanel’s**, **Keep mine** or **Merge by hand**.
- A component's file changed after the page read it is not saved over; reload it and decide.
- DNS changed on the server is kept (see [DNS and peer servers](guide:dns-servers#outside)).

The panel itself runs as two services; restarting them does not stop the hosted services:

```
sudo systemctl status celikpanel-panel celikpanel-agent
```
