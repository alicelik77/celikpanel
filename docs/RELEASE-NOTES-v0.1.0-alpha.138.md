# v0.1.0-alpha.138

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.138.tr.md)

A small release of two jobs. A domain's programs and AI apps get their own
place on the domain page, and a program and an AI app no longer take a domain
from each other. Each mailbox shows the settings a mail program needs, and
the mail server name those settings give is now the one this server really
serves.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.137

1. Update from the panel's own update screen, as before. This release does
   not change the panel's database layout (it stays at the alpha.137 form),
   so going back to alpha.137 later needs no database restore.
2. There is nothing to press or set after the update. Domain pages show the
   new Programs section by themselves; a page left open from before the
   update shows it after it is opened again.

## What changes for you

### Programs has its own section on the domain page

The owner found his running program under **Site** and asked for it to have
its own place in the domain page's row (19 November 2026). The row is now:

**Overview · Site · Programs · Files · Mail · Databases · Domain & DNS ·
Security · Backups · Advanced**

- **Programs** holds the two cards that were in Site, unchanged: "Programs and
  Python apps" (the program with Deploy again, Stop, Restart, Log,
  Environment, Settings and Remove the program, how to upload a program, and
  what a program can do and what is blocked) and "AI & automation". On a
  program's site the program's card comes first, on an AI app's site the
  app's card.
- **Site** keeps what the site is and how visitors reach it: Site type,
  Applications (PHP sites), PHP settings and Redirects.
- **Overview** has a Programs tile; on a program's or an app's site it says
  what runs there.
- **Old links keep working.** A link or bookmark to the old place
  (`?tab=programs`, `?tab=ai`, and the address the cards had under Site)
  opens the cards in Programs.
- **Programs is shown only where something in it can be done.** It is there
  when the plan includes programs, Python apps or AI apps and the site's type
  can take one, and whenever the domain already holds one. A Node.js site
  takes neither, so it has no Programs section. A team member does not get
  it, as before. The sentences that said "the Site section" or "the Programs
  tab" for these cards now say "the Programs section".
- **The user guide** lists the new section and has a short part about it.

### A program is never taken over

A domain serves one of them at a time: a program (or Python app) or an AI
app. Before this release the server let an AI app be installed on a domain
whose program was already created, and a program be deployed again on a
domain an AI app served. Either took the domain from the other, and the
domain then could not be deleted while the page no longer showed the card
that removes what was left. The server now refuses all three, before
anything is changed:

- **Installing an AI app on a domain a program uses** (created or deployed):
  "Nothing was changed: a program or Python app uses this domain, and the app
  would take its place. Remove the program in the Programs section first, or
  install the app on another domain or subdomain."
- **Updating an AI app on a domain a program uses** (only on a domain that
  already holds both): the same sentence.
- **Deploying a program on a domain an AI app uses:** "Nothing was changed:
  an AI or automation app uses this domain. Remove it in the Programs section,
  under AI & automation, first."

The page does not offer these presses in the first place.

**Anything that exists on a domain is shown where it can be removed.** The
domain list now says whether a domain holds a program or an AI app, whatever
the plan or the site's type say, and Programs draws its card:

- **A domain that already holds both** (possible on servers updated from an
  earlier version): Programs shows both cards. The card of the one the domain
  does not serve shows what manages and removes it (Stop, Restart, Log,
  Environment, Settings, Remove) but nothing that would put it back on the
  domain (no Deploy again, no Open, no update or install retry). Remove the
  one you do not want; the domain can then be deleted.
- **A program created and never deployed, with the plan's program rights
  closed since:** Programs and the program's card are there, with what the
  plan still allows and **Remove the program**.
- When the domain cannot be deleted because of one of them, the refusal sends
  you to the Programs section, where the card with its removal is shown.
- A domain holding a program (even one never deployed) does not offer an AI
  app's **Install**, and a domain holding an AI app does not offer a new
  program: the server would refuse both.

### Each mailbox's connection settings

- Each mailbox row's **More** menu starts with **Connection settings…**. It
  opens a window with what a mail program (Outlook, Thunderbird, Apple Mail,
  a phone) needs: the user name, the incoming (IMAP) server, the POP3
  alternative and the outgoing (SMTP) server with port and security, and the
  mailbox's webmail link. Each value can be copied, and **Copy all** copies
  everything as plain text for a message, in the page's language.
- **The password is never shown or copied.** The window says it is the one
  set for the mailbox and where a new one is set.
- **Outgoing mail requires sign-in** with the same user name and password;
  the window says so when the server does.
- **When the settings would not work, the window says so.** If mail is not
  installed on this server, it says that nothing answers mail programs here,
  who acts and what to do, and **Copy all** is off. If the server name shown
  is not confirmed, it says the settings may not reach the mailbox, and the
  copied text starts with a note that the server name is not confirmed yet.
- After a mailbox is created, its notice offers **Show connection settings**;
  a notice that offers a next step stays until it is closed.
- A team member who may only see the mailboxes opens the window beside
  **Webmail**.

### Mail programs are told the mail name this server really serves

For a subdomain, a domain without a website, or a domain whose mail name was
already in use elsewhere, the mail settings (the **Settings** tab of the
domain's mail and now each mailbox's window) named `mail.<domain>`. This
server neither chose nor serves that name with a matching certificate, so a
mail program set up with it could warn about the certificate or not connect.
They now name the mail name this server really serves: usually the server's
own mail name.

**If you gave someone `mail.<domain>` as the server for such a domain, use
the name the panel shows now.** Domains whose own `mail.<domain>` this server
serves with a certificate keep that name.

### The four styles have new names

The panel's styles (the palette button beside the light and dark switch) are
now **Celik**, **Sky**, **Forest** and **Amber**; none of them is named after
another company's product. A style chosen before is kept and shown under its
new name.

## Known limits

- **Node.js stays under Site.** A Node.js application's start, stop, restart
  and log stay under Site → Site type, beside the type, version and start
  command they belong to.
- **One English sentence still says "Programs tab".** The server's own
  English refusal behind a site type change or a domain deletion still names
  "the Programs tab"; the page shows its own words, which name the Programs
  section.
- **"May not reach" for a subdomain on a server without mail.** On a server
  where mail is not installed, a subdomain's mail settings say they may not
  reach the mailbox, not that mail is missing here.
- **No automatic setup hint.** The mailbox window does not mention that some
  mail programs can find these settings by themselves.

## Checked in the lab

Lab check finished: one run, on a Debian 13 and an Ubuntu 24.04 test server.

- Both servers updated through the panel's own update page, by themselves,
  with nothing left failed.
- **A running program was not taken over.** On a domain whose program was
  deployed and running, a direct request to install an AI app was refused
  with "Nothing was changed: a program uses this domain. Remove it in the
  Programs section first, or use another domain or subdomain for n8n."
  Nothing was created, and the program answered 45 of 45 checks during and
  after the attempt.
- **What a domain holds is shown.** A program created but never deployed,
  on a domain whose plan no longer included programs, was still shown in
  Programs with **Remove the program**; it was removed and the domain was
  deleted.
- The ten-entry section row fits one line at desktop width and scrolls
  inside itself on a phone, without the page scrolling sideways. Old
  addresses (`?section=site&part=programs`, `?tab=ai`) land on Programs.
- The mailbox window in Turkish and English, at desktop and phone width: the
  settings, the outgoing sign-in line, and **Copy all**, whose text has no
  password. A new mailbox's notice offered **Show connection settings**,
  which opened the window.
- The server's mail name was shown for a subdomain and for an ordinary
  domain; it matched the server's certificate on ports 993, 995 and 587, and
  signing in over IMAP with it worked.
- A domain was created and deleted on a primary and backup DNS server pair.

**Not checked:** a domain that holds both a program and an AI app, and what
installing an AI app over a program did on the earlier version: no AI or
automation app can be installed on the lab's Debian 13 and Ubuntu 24.04
servers at all (Node.js 24 and Python 3.11 or 3.12 are missing there), so
that part rests on component tests. A Node.js site without the section. The
warning for a mail name that is not confirmed.

**Fixed after the lab run (screen only):** on a PHP domain with a program
that was created but never deployed, Programs also showed the AI card with
its **Install** entries, which the server refuses there. It no longer does,
and a domain holding an AI app no longer offers a new program.

### Found in the lab, left for the next release

- The line saying n8n "cannot be installed yet" mixes languages and points
  to a Node.js installation the server cannot do.
- Right after a domain deletion that waits for the backup DNS server, the
  status says the deletion stopped and to delete it again, although it
  finishes by itself (about 51 seconds in the lab).
- No AI or automation app can be installed on Debian 13 or Ubuntu 24.04
  (the runtime versions they need are missing there).

## Evidence

No schema change: schema 079, as in alpha.137. No persisted record changes;
the domain list carries two additive fields (`holds_program`, `holds_app`),
absent when false and for a team member, read in one query each for the
whole list. The changes are in the domain page (`web/src/components/domain`,
the Programs section and its rule), the two cards (`DomainRuntimeApps`,
`DomainServiceApps`), the server's install, update and deploy handlers of AI
apps and programs and the domain list (`cmd/panel`), the mailbox window
(`MailboxConnectionDialog`, `DomainMailManager`) and the mail setup answer
(`cmd/panel/mail_autoconfig.go`, `domain_mail_extra_handlers.go`).
Component tests: the section per site type, plan, role and what the domain
holds; the screen's refusal table read against the server's conditions in
its source; the three refusals on the server's own handlers (nothing
recorded, the Agent not asked, the other site types unchanged); the domain
list's fields per role; every older address of the cards; the mailbox window
in each mail state, in both languages; the mail setup answer for every kind
of domain; the four styles and a style stored under its earlier name. The
`cmd/panel` tests were compared name by name with alpha.137: no new failure.
One lab run (above). **No P0 item is closed.**
