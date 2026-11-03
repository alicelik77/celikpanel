---
id: import
title: Importing a cPanel account
roles: [admin]
topics: [import]
keywords: [import, migrate, migration, cpanel, cpmove, archive, move]
---

**Import** brings one cPanel account into CelikPanel from its cpmove or backup archive. Other panels cannot be imported yet.

## Before you start {#before}

- Create the customer first; the import goes into one of its subscriptions.
- As root, copy the archive onto this server into the import folder that the page names, readable only by its owner. The panel does not download it for you.
- The domain must not exist in CelikPanel yet; importing into an existing domain is refused.

## The steps {#steps}

1. **Source:** enter the **Archive path on the server** and press **Inspect archive**. The page lists what the archive holds.
2. **Preview:** choose the **Target subscription**, the domain to import, and what to bring over: website files, mail accounts and forwarders, DNS records, databases.
3. Press **Start import**.
4. **Result:** the import finished, finished in part, or its result is not known.

"In part" lists what did not come over; bring those parts by hand. "Not known" means the answer was lost: look at the domain before you start again.

## What is not brought over {#limits}

- The messages already in the mailboxes. Mail accounts keep their passwords; a mailbox with no password in the archive is not created. Move old mail with an IMAP tool.
- Database users. Create new ones and point the site's configuration at them.

After the import, check the site and switch the domain's DNS or nameservers to this server.
