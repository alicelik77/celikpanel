---
id: apps-wordpress
title: Applications, WordPress tools and malware scan
roles: [admin, reseller, customer]
topics: [domain.apps#catalogue]
keywords: [wordpress, prestashop, nextcloud, drupal, joomla, mediawiki, install, plugin, update, staging, malware, virus, clamav, quarantine]
---

The **Applications** tab of a PHP site installs a ready-made application, and has tools for WordPress. The malware scan is under **Advanced**.

## Installing an application {#catalogue}

The catalogue can hold WordPress, PrestaShop, Nextcloud, Drupal, Joomla and MediaWiki; the administrator decides which are offered. Each is the official release, checked before it is unpacked.

A site holds one application, and only an empty site gets one, so nothing of yours is overwritten. Under each application the tab says whether this site is ready, or what is missing:

- **Needs PHP …**: change the site's PHP version under **Hosting**, then **PHP**.
- Missing PHP extensions: the administrator installs the packages named.
- The site's public folder already holds files: move or delete them, or use a new subdomain such as `shop.example.com`.
- Not available to this site: ask your provider to offer it.

Press **Install** and confirm. The panel creates the database and runs the application's own setup.

## The administrator password, shown once {#password-once}

When setup ends, the tab shows **Administrator** and **Password** for the application. Save the password now, then press **I have saved it**: CelikPanel does not keep it afterwards. **Open administration** opens the application's sign-in page. If the page was closed before you saw the password, **Show the password (once)** shows it one more time.

WordPress is different: it opens its own setup page, where you choose the site's name and your administrator.

## When setup does not finish {#setup-failed}

- **Retry setup**: the files are in place but the application's setup stopped. It continues where it stopped.
- The result is not known (an answer was lost): press **Remove** to clean up, then install again.
- The installation failed: nothing was left on the site; read the reason and try again.

## Removing an application {#removing}

**Remove** asks you to type the domain. The application's files go, and the site shows its placeholder page. Its database stays unless you tick **Also delete its database**; Nextcloud's data folder likewise.

## WordPress tools {#wordpress}

**WordPress tools** shows the WordPress version, the plugins and the themes, with updates from wordpress.org.

- **Update to …** updates WordPress; **Update selected plugins** updates the ticked plugins.
- Before every update a backup is made. If the update fails, the site is put back as it was.
- If the result is not known, open the site. If it does not work, restore the backup the message names, under **Advanced**, then **Backups**.
- Themes: update them from WordPress's own Dashboard, after a backup. CelikPanel does not install them, because wordpress.org publishes no checksum for themes.
- **Install security and maintenance releases of WordPress automatically** checks once a day. Major releases stay your choice.

**Copy to another site (staging)** copies files and database to an empty PHP site of the same subscription and changes the address inside them. Choose the **Target site** and press **Copy**. The copy counts against your limits; copying back to the live site is not offered.

## Malware scan {#malware}

Under **Advanced**, **Malware scan** checks the site's files with ClamAV. Press **Scan this site** or **Scan all sites of the subscription**. One scan runs at a time on the server, at low priority; the page shows its progress.

Nothing is deleted automatically. Each infected file can be moved to **Quarantine**, brought back with **Put back**, or removed with **Delete for good**. After a finding, change the site's passwords and update its software.

If ClamAV is not installed or has no signatures yet, the scan cannot run; with old signatures it runs, but the result warns that newer threats may be missed. In both cases the administrator acts in **Components**, and you start the scan again.
