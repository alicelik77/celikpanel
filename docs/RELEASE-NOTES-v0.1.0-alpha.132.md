# v0.1.0-alpha.132

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.132.tr.md)

The first of the small design releases the owner approved on 12 November
2026. Three things change for everyone who uses the panel: a setting is saved
only when you press Save, a domain's page has nine sections instead of eleven
tabs, and the domain list says its actions, kinds and states in words.
**Only the owner updates installed panels, from the panel's own update
screen.**

## What changes for you

### One way of saving

- **Clicking a setting changes nothing on the server any more.** A changed
  setting is marked ("Changed: off → on"), and a bar at the bottom says
  "1 change not saved. Nothing changes until you press Save." with
  **Discard** and **Save**. Save appears only when something changed; when
  it cannot work, the reason stands beside it.
- **Leaving with unsaved changes asks first**, in the panel's own dialog:
  a link, Back or Forward, another section of a domain's page, another tab
  of Users. "Stay on this page" keeps your changes; "Leave without saving"
  gives them up. Closing the browser tab still gets the browser's own
  question.
- **Where this applies:** a domain's certificate settings (automatic
  renewal, force HTTPS, HSTS and its duration, the mail certificate),
  WordPress automatic minor updates, the weekly malware scan, the PHP pool,
  web server settings and directives, a Node.js site's variables, a
  program's settings and environment, the site type, the www redirect,
  automatic backups, mailbox sending limits, the out-of-office reply, a
  site's other logins, scheduled tasks and the file editor; for the
  administrator Offered (permissions, software, applications), offered PHP
  and Node.js versions, PHP extensions, the automatic update check, the
  server mail policy, notifications, service and reseller plan forms,
  server limits and the password level.
- **Saving several changes says what happened to each one:** what was
  saved, what was refused and why, and what is not known. A save whose
  answer was lost is never sent a second time; the panel reads the server
  again first and then says whether it was saved.
- **Someone else changed it meanwhile:** the row shows the server's value
  now and offers to take it.
- **The file editor reads the file again before it writes.** If the file
  changed on the server since you opened it, nothing is written and you
  choose: open the server's version or write yours over it.
- Removing a program or an AI and automation app asks in one dialog that
  lists what is lost and what stays, with the domain's name typed.

### A domain's page in nine sections

- **Overview · Site · Files · Mail · Databases · Domain & DNS · Security ·
  Backups · Advanced.** Every screen that existed is still there, unchanged
  inside, in the section that holds it now. A section with several parts
  lists them under "On this page".
- **The place on the page is in the address,** so a reload, Back and a
  shared link keep it. Every older link still opens the same screen: links
  the server builds, notices stored earlier, and the health checks' fixes.
- **A section your plan does not include stays in its place,** faded with a
  lock. It says what the section is for, who can add it and what to ask
  for. Team members still see only what they were granted; a section the
  server has no engine for is not shown.
- The header says the site's kind and the domain's state in words ("PHP
  site", "Active", "Suspended").
- Texts that named the old tabs (Hosting, Access, the certificate tab …)
  now name the sections, in both languages and in the guide.

### Row actions in words

- **Domain list:** every row has "Manage" and "Open site" as words and a
  "More" menu whose last item, in red after a line, is "Delete…". The menu
  works with the keyboard and a screen reader. On a phone each row is a card
  with the same buttons.
- **Kinds and states in words** on the domain list, the domain's header and
  the administrator's home; a code the panel does not know is a neutral
  word (the administrator also sees the code).
- **One guidance block** (Why · Who acts · Next step · Then) replaces the
  copies on the DNS page, the kept records, the setup wizard, the malware
  scan, the WordPress tools and the DNS publication notice.
- **A refusal shows its plain sentence first;** the server's own words are
  under a closed "Details". Customers and resellers see them there only
  when they name no path, service or command.

### Smaller corrections

- "Your plan does not include connections from outside" names your
  provider or the administrator correctly in Turkish (it read "yönetici
  kişisinden").
- Offered, the offered versions and the offered applications: on a changed
  row the state word says what the server has now; the "Changed" mark says
  what Save will do.

## Known limits

- **The insides of the nine sections are today's screens, regrouped.** They
  are redesigned section by section in the next releases: Overview with the
  domain list and the customer's home first, then Mail, Databases, Domain &
  DNS with Security, Site with Files, Backups with Advanced.
- **File editor:** the check read and the write are two steps; a change made
  on the server in the moment between them is not seen. Closing that window
  needs a version check on the server side.
- **PHP extensions are applied one call at a time,** shown as steps. A stop
  in the middle leaves the earlier ones applied, and the page names them.
- **The kind word for an uploaded program** on the domain list cannot yet
  tell a program from a Python app; it reads "Program or Python app".
- **The "More" menu is on the domain list only** so far; the other lists
  follow with their sections' redesign.
- Moving to another part of the same section asks nothing, because every
  part stays on the same page; moving to another section asks.
- Component tests and real-browser screenshots against a stand-in server do
  not replace a real-system update check.
  **[Lab check: to be filled after the lab round — systems, update path,
  what was tried, what was not.]**
- Earlier limits still apply (see alpha.131).

## Evidence

No migration (schema stays 77) and no Go, Agent or stored-format change.
Older domain-page links (`?tab=…`, including the fix links stored in
`account_notices.fix_path` since migration 072 and the health checks'
`{tab, sub}`) are read through a permanent alias table and rewritten in
place; nothing stored is rewritten. Unsaved changes live only in the open
page; after a rollback to 131 everything saved reads as before. Release
sequence 131 → 132. Web: component tests, mounted tests of the save bar, the
leave question (router links, Back/Forward, section switches), every
converted screen and the nine sections; bundle budgets unchanged (critical
boot path 109.94 of 110 KiB gzip).
