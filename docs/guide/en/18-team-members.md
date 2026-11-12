---
id: team-members
title: Team members
roles: [customer]
topics: [team]
keywords: [team, user, additional user, colleague, developer, access, permission, view, manage, invitation]
---

A team member is a colleague, a developer or an agency who works on your sites with their own sign-in. You choose what each member may see and change; your own password stays yours.

## Where to find it {#where}

Open **Users** in the menu. It shows **Team members** when your plan allows them. If **Users** is missing, ask your provider.

## Adding a team member {#adding}

1. Press **Add team member**.
2. Under **Account details**, enter the **Username** and **Email**.
3. Choose a **Password** of at least 8 characters, or tick **Send an invitation instead of setting a password**. With an invitation, the person chooses their own password with a link that works for 7 days.
4. Set the **Access permissions** (below).
5. Press **Create member**.

## Access permissions {#access}

Access is given per area: **Files**, **Databases**, **Mail**, **DNS**, **SSL**, **Scheduled tasks**, **Backups**, **PHP** and **Statistics**. For each area choose **No access**, **View** or **Manage**.

- **Subscription access** applies to every domain of a subscription, also domains added later.
- **Domain access** adds access for one domain only.
- **All view**, **All manage** and **Clear** set every area at once.

Permissions add up: the strongest one given counts. A member who can only view sees the pages with a "View-only access" note and cannot change anything.

Some parts are never open to team members: the domain's **Overview**, **FTP, SFTP and SSH**, **Applications**, **AI & automation**, **Programs**, the malware scan, WordPress tools and remote backup destinations. A team member sees only the sections of a domain that were opened for them.

## Changing or removing a member {#changing}

In the list, **Edit** changes the details, the password or the access. **Suspend** signs the member out at once and blocks their sign-in; **Activate** lets them in again. **Delete** removes the member's account and permissions; your subscriptions, domains and sites stay.

Any change to the password, status or access signs the member out of their current sessions.

## What a member sees {#member-view}

The member signs in with their own username and password and sees only the domains and areas opened for them. They change their password from their own menu with **Change password**.
