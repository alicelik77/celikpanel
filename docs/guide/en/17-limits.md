---
id: limits
title: Limits and suspension
roles: [reseller, customer]
topics: []
keywords: [limit, quota, traffic, bandwidth, disk, full, suspended, plan, upgrade, 80%, 100%]
---

Your plan sets how much you may use: domains, mailboxes, databases, disk space and monthly traffic. This chapter explains what happens when a limit is reached, who acts, and how things open again. Nothing is ever deleted because of a limit.

## Where to see your usage {#my-plan}

**My plan** on the **Dashboard** and under **My account** shows each limit as used of allowed, the disk split into site files, mail and databases, and the **Traffic this month**. At 80% and at 100% you, your provider and the administrator get a notice under **Traffic and disk notices**.

## Traffic {#traffic}

Traffic is what your websites send to their visitors in a calendar month. It is counted every 5 minutes, so a site can pass the limit by what it sends in that time.

When the month's traffic is used up:

- only the websites show "traffic limit reached";
- mail, databases and file access keep working;
- the websites open again by themselves on the 1st of next month, or within minutes after your provider raises the limit.

If your provider is a reseller, your sites can also close when the reseller's shared traffic is used up; the notice says so.

## Disk space {#disk}

Disk use counts the site files, mail and databases, measured every few hours. Backups are not counted.

When the disk is full:

- nothing new can be created: no domain, mailbox, database, application, manual backup or restore;
- uploads stop: the file manager refuses them, FTP and SFTP become read-only (downloads still work), and PHP sites take no uploaded file;
- the websites keep running, and mail is delivered while each mailbox has room.

Who acts: you delete files, mail or databases you no longer need, or ask for a larger plan. Everything opens again by itself after the next measurement under the limit, within hours.

If usage stays over the limit, a notice warns after 5 days, and after 7 days the subscription is suspended.

## When your subscription is suspended {#suspended}

A suspended subscription shows one screen after you sign in: **Your subscription is suspended**. Your websites, mail, databases and other services are switched off. Nothing has been deleted.

The screen tells you:

- **Why:** the reason your provider wrote, or that the disk was over the limit for 7 days, or that the subscription's end date has passed;
- **Who can change this:** your reseller or the server administrator;
- **What to do:** contact them and ask them to resume it;
- **How it continues:** after they resume it, sign in again or reload the page.

A suspension for disk space ends by itself when a measurement is under the limit. Your provider raises the limit, or resumes the subscription to give you 7 more days to free space.

## Summary {#summary}

| Reached | What stops | Opens again |
|---|---|---|
| Traffic 100% | Websites only | On the 1st, or minutes after the limit is raised |
| Disk 100% | New items and uploads | By itself, within hours of going under the limit |
| Disk over the limit for 7 days | All services (suspension) | When usage is under the limit, or your provider resumes it |
| Mailbox full | New mail to that mailbox | As soon as there is room ([details](guide:mail#full)) |
