---
id: mail
title: Mail and webmail
roles: [admin, reseller, customer]
topics: [domain.mail#mailboxes]
keywords: [email, mailbox, quota, full, forward, forwarder, webmail, roundcube, imap, smtp, outlook, thunderbird, out of office, catch-all]
---

Mailboxes belong to a domain. Open the domain, then its **Mail** tab. The tab appears when the server has a mail service and your plan includes mail.

## Mailboxes {#mailboxes}

On **Accounts**, press **Create address**:

1. Type the **Username**, the part before `@`.
2. Choose a **Password**.
3. Set the **Quota (MB)**, the mailbox's size. It starts at 1024.
4. Press **Create**.

Each row shows the mailbox's **Usage** and **Quota**. Beside it you can change the password (8 or more characters), **Edit quota**, set an out-of-office reply, or delete the mailbox.

## When a mailbox is full {#full}

- At **90%** the mailbox receives one warning message, in English and Turkish.
- At **100%** new mail is refused, and senders get a "mailbox full" bounce. Nothing already in the mailbox is lost.

Who acts: the mailbox owner deletes old mail, or you raise its size with **Edit quota**. Mail is accepted again at once. This works through the mail server itself, also when the panel is stopped.

If the tab says the Dovecot quota plugin is not enabled, sizes are recorded but not enforced yet; the administrator turns it on.

## Forwarding {#forwarding}

On **Forwarding**, press **Add forwarder**, type the **Source address** and the **Destination address**, and press **Create**. Mail sent to the source is delivered to the destination.

On **Settings**, the **Catch-all address** delivers mail sent to any unknown name at your domain to one mailbox. It also collects spam.

## Out-of-office reply {#autoreply}

Open the mailbox's **Out-of-office reply**, tick **Send an automatic reply**, write the **Subject** and **Message**, optionally a **First day** and **Last day**, and press **Save reply**. The mail server sends the reply itself, to each sender at most once in the number of days you set, and never to mailing lists.

## Webmail {#webmail}

The **Webmail** box on the tab has **Open Webmail**. Sign in with the full email address and the mailbox password; no panel account is needed. If it says webmail is not available on this server yet, the administrator installs Roundcube and gives it an address.

## Settings for mail programs {#mail-programs}

**Settings** shows **Mail client setup** for Thunderbird, Outlook, Apple Mail or a phone. For `example.com` the values are:

| | Server | Port | Security |
|---|---|---|---|
| Incoming (IMAP) | `mail.example.com` | 993 | SSL/TLS |
| Incoming (POP3) | `mail.example.com` | 995 | SSL/TLS |
| Outgoing (SMTP) | `mail.example.com` | 587 | STARTTLS |

The **Username** is always the full email address, such as `anna@example.com`, with the mailbox password. The outgoing server needs the same sign-in.

## Mail that lands in spam {#authentication}

**Authentication** shows SPF, DKIM and DMARC: records that prove your mail is genuine. When the DNS is served by this server they are published for you. When it is at another provider, copy the records shown and add them there.
