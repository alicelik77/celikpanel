---
id: databases
title: Databases
roles: [admin, reseller, customer]
topics: [domain.databases#domain-databases, databases#databases-page]
keywords: [database, mysql, mariadb, postgresql, user, password, phpmyadmin, phppgadmin, privileges]
---

A website such as WordPress keeps its content in a database. CelikPanel creates MariaDB (MySQL) and PostgreSQL databases on the server, with a user for each.

## In the domain's Databases section {#domain-databases}

The quickest way is from the domain itself. Open the domain, then its **Databases** section, and press **Create database**:

1. Type a **Database name**. The panel puts the domain in front of it: for `example.com` and `shop`, the full name is `example_com_shop`.
2. Choose the **Database engine**.
3. Choose a **Password**.
4. Press **Create database**.

A user with the same name and `_user` at the end, here `example_com_shop_user`, is created with full access. Your application connects to `localhost` with that name, user and password.

## The Databases page {#databases-page}

**Databases** in the menu lists every database you may see: an administrator sees every database on the server with its **Owner**, a reseller their customers' databases, a customer their own, grouped by domain. Passwords and the contents of a database are never shown here.

- **Open** takes you to the database's domain, at its **Databases** section.
- **More › New password…** gives one of the database's users a new password. Programs that use the old password stop connecting until they get the new one; an application CelikPanel installed gets it automatically.
- **More › Delete…** deletes the database and all its data once you type its name. Its users are not deleted; deleting the database in its domain's **Databases** section also removes the user created with it.
- **Add database** asks **For which domain?** and opens that domain's **Databases** section with its form.

What belongs to the server is not on this page. An administrator turns connections from outside on or off on **Security**, under **Open to the outside**, and finds the account CelikPanel connects to MariaDB or PostgreSQL with on that service's page in **Components**.

## phpMyAdmin and phpPgAdmin {#web-tools}

**Database web tools** in the domain's **Databases** section has **Open phpMyAdmin** (MariaDB) or **Open phpPgAdmin** (PostgreSQL). The tool opens through the panel and is never open to the internet. Sign in with the database user and its password; you see only your own databases.

If the button says to install the tool from **Components**, the administrator installs it.

## Passwords {#passwords}

CelikPanel does not keep database passwords and cannot show them again. If you lost one, give the user a new password and put it into your application's configuration file. When your plan's database count or the disk is full, new databases are refused; see [Limits and suspension](guide:limits#disk).

:::warning
Deleting a database deletes all its data. Take a backup first in the domain's **Backups** section.
:::
