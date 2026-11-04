---
id: plans-rights
title: Plans, rights and what is offered
roles: [admin, reseller]
topics: [store]
keywords: [plan, rights, permissions, quota, limit, unlimited, pool, disk, traffic, offer, store]
---

What an account may use sits inside four layers: what is installed, what the administrator offers, what the reseller has, and what the account has. The server enforces this; screens hide what an account cannot use.

## Rights {#rights}

- **Quantities:** customers (for a reseller), domains, databases, mailboxes, disk and monthly traffic. `0` means **Unlimited**.
- **Permissions:** for example **Edit DNS records**, **Command line (SSH)**, **VPN**.
- **Software:** PHP and Node.js versions, database engines, webmail, applications.

Taking a right away keeps what already runs; nothing new of that kind can be created.

## Offered {#offered}

On **Users**, the administrator's tab **Offered** marks every permission and installed program **Offered** or **Hidden**. Applications such as WordPress are under **Applications (app installer)**. The built-in **Unlimited** plans follow it at once.

## Plans {#plans}

**Service plans** and, for the administrator, **Reseller plans**: **Add plan**, give a **Plan name**, set its parts, **Save**. An edit that takes something away first shows the accounts that follow the plan. **Unlimited** and **Unlimited reseller** are built in and cannot be edited.

**Rights** on an account row offers **Unlimited**, **A plan** or **Custom**. Changing one item of a plan's account makes it **Custom**, after asking. **End date** suspends the subscription on that day.

## As a reseller {#reseller}

- You can only narrow. Items outside your rights are greyed out; the administrator can add them.
- **Unlimited** for your customer means everything in your own rights.
- Your **Pool** caps what all your customers really use together, whatever your plans hand out. **My plan** shows **Your reseller pool**, with a notice at 80% and 100%.

## Real usage {#usage}

**My plan** shows each meter. Disk counts site files, mail and databases; traffic is counted per month, a few minutes behind.

| At the limit | What happens |
|---|---|
| A quantity | New items of that kind are refused. |
| Traffic | Only the websites stop, until the 1st or until the limit is raised. Mail and files keep working. |
| Disk | Nothing new is created and uploads stop. After 7 days the subscription is suspended; it reopens by itself under the limit. |

Notices at 80% and 100% reach the customer, its reseller and the administrator.

## Store {#store}

**Store** grants product rights to one subscription with **Grant access** and **Remove access**; it installs nothing. In **Catalog management** the administrator offers the **AI and automation apps** after reading their risks.
