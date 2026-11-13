---
id: domains-websites
title: Domains and websites
roles: [admin, reseller, customer]
topics: [domains#domain-list, domain.overview#overview, domain.site#hosting, domain.security#https]
keywords: [domain, website, subdomain, php version, certificate, https, ssl, lets encrypt, nameservers, static, node.js]
---

Everything you host lives under a domain: the website, its mail, its databases and its DNS. This chapter covers adding a domain, its page, its site settings and its certificate.

## The domain list {#domain-list}

**Domains** in the menu lists your domains with their type, disk usage, traffic and status. Choose a domain, or **Manage**, to open its page. **Visit site** opens the website; **Delete** removes the domain with its site and mail for good, once you type its name; the question lists what goes and what stays. Its databases stay on the database server but leave the panel, so delete them first if they should go too. Take a backup first (see [Backups](guide:backups#backups)).

## Adding a domain {#adding}

1. Press **Add domain**.
2. Type the **Domain name** without `www`, for example `example.com`. A subdomain is added the same way: `shop.example.com`.
3. Under **What is this domain for?** choose **Website**, or **DNS only (no website)** if the site is hosted elsewhere.
4. Leave **Enable SSL (Let’s Encrypt)** ticked if the domain already points to this server.
5. Press **Create domain**.

If you asked for a certificate, the window waits for it, up to about a minute. Leave it open; nothing is sent twice. When the certificate could not be issued, the site is still created and works without HTTPS; request it later as described in [Certificates and HTTPS](#https).

## The overview {#overview}

A domain's page has one row of sections: **Overview**, **Site**, **Files**, **Mail**, **Databases**, **Domain & DNS**, **Security**, **Backups** and **Advanced**. You see only the sections that apply to the domain and to you. A section with several parts lists them under **On this page**. A section your plan does not include stays in its place, faded and with a lock; it says who can add it.

The line under the domain's name shows its **Type**, **PHP version**, **SSL**, **Disk usage** and **Traffic** this month.

**Connecting this domain**, on **Overview** and under **Domain & DNS**, says whether the name points to this server. Until it does, nothing hosted here answers for it. It offers two routes; enter the values at the company you bought the domain from:

- **Route A** hands DNS to this server's nameservers. Mail records and certificates are then managed for you.
- **Route B** keeps DNS at your current provider; you add only the address records shown.

A change at the registrar can take from minutes to several hours to spread. The card checks again by itself every half minute for 30 minutes; **Check again** checks at once. Do not change the records again while it says the change is spreading.

## Site settings {#hosting}

The **Site** section holds what runs on the domain and how:

- **Site type**: the **Project type**: **PHP**, **Static**, **Node.js**, **Proxy** or **Forwarding**. Choose one and press **Apply**.
- **Applications**, **AI & automation** and **Programs**, if your plan includes them.
- **PHP settings**, below, and **Redirects**, if your plan includes them.

The **Names** part of **Domain & DNS** holds the **Document root**, **Redirect to www** and **Domain aliases**, extra names that show the same site. **Protected folders** are under **Security**.

### PHP version {#php}

A new website starts as a static site. To run PHP, go to **Site type** in the **Site** section, choose **PHP**, pick a version and press **Apply**. To change the version later, go to **PHP settings** in the same section, choose it under **PHP version** and press **Apply**. Only versions installed on the server are listed; if **PHP-FPM** is missing, the administrator installs it from **Components**.

## Certificates and HTTPS {#https}

Open **Security**; its first part is **Certificate (HTTPS)**:

1. On **Free certificate (ACME)**, check the **Email address** and keep **Enable automatic renewal** on.
2. Press **Get certificate**.

The certificate authority must reach the domain at this server, so finish **Connecting this domain** first. A free certificate renews by itself. **Custom certificate** takes a certificate you bought: paste the **Certificate (PEM)** and the **Private key (PEM)** and press **Upload certificate**.

**Force HTTPS** sends every visitor to `https://`. Turn on **Enable HSTS** only when HTTPS works for good.

If **SSL** says **Configured — needs attention**, the certificate exists but the web server or mail server does not use it yet. Open **Security** and press **Retry activation**.
