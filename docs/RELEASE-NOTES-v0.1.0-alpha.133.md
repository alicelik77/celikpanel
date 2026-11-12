# v0.1.0-alpha.133

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.133.tr.md)

The second small design release. The left menu's **Databases** page now
lists everyone's databases by the one visibility rule, and what belongs to
the server itself has left it for Security and the MariaDB and PostgreSQL
pages. Fields are as wide as what they hold, deleting a domain says what
goes and what stays, and a false blocklist alarm is gone.
**Only the owner updates installed panels, from the panel's own update
screen.**

## What changes for you

### Databases: one list, by who may see what

- **An administrator sees every database on the server,** a reseller their
  own customers', a customer their own. Before, an administrator saw an
  empty list ("No databases yet") although customers had databases.
- Administrators and resellers read one list with **Database, Owner,
  Domain and Type**. Under the owner it says "you", or which reseller a
  customer belongs to. One search looks in the name, the owner and the
  domain; a click on a column heading sorts by it. On a phone each row is a
  card. A customer sees their databases grouped by domain.
- **Each row says its actions in words:** "Open" goes to the domain's
  Databases section; the "More" menu has **New password…** (asks which user
  when there are several) and **Delete…** (lists what is lost and what
  stays and asks for the database's name to be typed). Deleting from this
  list **keeps the database's users**; the question says so, and that
  deleting the database in its domain's Databases section also removes the
  user created with it.
- **"Add database"** asks "For which domain?" and opens that domain's
  Databases section with its form already open.
- Passwords and the contents of a database are never shown on this page.
  The list reads itself again after every change and every half minute.
- If the server could not be asked which database services run, the
  administrator is told exactly that, never "no database service".

### Server matters moved to their own places

- **"Customers may connect to their databases from outside"** is now on
  **Security**, in the card **Open to the outside**. It is saved only with
  Save, and the question before saving says what turning it on or off
  means. Under it each installed database service says "Ready · port ·
  encrypted connections only" or "Not ready", with why, who acts, the next
  step and one button, in plain words.
- **The account CelikPanel connects to MariaDB or PostgreSQL with** is on
  that service's page in Components, with Show password, New password… and
  Remove account… in its "More" menu.
- The administrator's Databases list ends with one quiet link to the
  outside-connection setting on Security. A domain's outside-connection
  card sends the administrator there instead of turning the switch on
  itself.
- The MariaDB page no longer suggests `bind-address = 0.0.0.0` for remote
  connections: that opens the server to every network.

### Fields sized to what they hold

Every text, number and date field and every list across the panel has one
of six sizes chosen by what it holds: a port or a count is short, an
address or a name medium, a host name, e-mail address or password wider,
a web address, path or command widest. On a phone a field never runs past
the screen. A test keeps it that way, with no exceptions.

### Other corrections

- **Deleting a domain** lists what goes (the website and its files, extra
  FTP logins, mailboxes and forwarding, CelikPanel's DNS records,
  certificates, the backup schedule) and what stays (databases stay on the
  database server but leave the panel; backups already taken stay but
  leave the panel; stored mail is moved aside; records at another DNS
  provider are untouched), and asks for the domain's name to be typed.
- **Plan limits:** a value outside its range is named beside Save, which
  stays off until it is corrected (Save used to stay on and the server
  refused). The notice levels (50–95 %) keep what is typed. A value above a
  key's own maximum names that maximum, never "at most no limit".
- **"Send through another server" (Postfix):** the last form with its own
  Save now saves like every other screen, with marked changes, the save bar
  and the leave question. "Test the connection" tries the typed values,
  saves and sends nothing, and says where it stopped: not reachable, not a
  mail server, no encrypted connection, or user name or password refused.
- **Blocklist check:** the panel no longer reports a listing when a list
  refuses the question. Spamhaus does not answer questions that come
  through a public DNS resolver, and that refusal was read as "listed".
  Each list's answer is now read against its documented codes: listed, not
  listed, or **could not check** with the reason; a list that could not be
  asked is a grey line that never colours the mail card or the domain. A
  policy entry (Spamhaus PBL) is said as one. dnsbl.sorbs.net, shut down in
  2024, is no longer asked.
- **A program or a service app is easy to find:** in a domain's Site
  section the part that is the site ("Programs", or "AI and automation")
  comes first and open, and the Overview's Site tile says what runs there
  and goes straight to it.
- Turkish uses "Parola" for a password and "Tekrar dene" for retry
  everywhere; the domain list's title comes before the plan cards on a
  phone; the notice's close button is named in your language.

## Known limits

- **Site backups do not include a program's files and data.** A backup of
  a site holds its web folder (and its databases in a full backup), not
  the folder an uploaded program or Python app runs from or the data it
  keeps there. This is fixed in the next release. Until then: a scheduled
  **full** backup of the domain covers its databases; keep your own copy of
  the program's folder.
- **Deleting a domain removes the panel's records of its databases but
  leaves the databases themselves on the server.** The delete question says
  so; delete the databases first if they should go too. A choice in that
  question comes in the next release.
- **Deleting a database from the Databases list keeps its users** on the
  database server. The panel shows a user only under a database it can
  reach, so delete the database in its domain's Databases section when the
  user created with it should go too.
- **The size column is absent** from the Databases list until sizes can be
  read in one call for the whole list.
- **The certificate screen does not yet say whether the mail name is
  covered.**
- A team member's Databases list is empty; their databases are on the
  pages of the domains they were given, as before.
- TO CONFIRM after the lab run: real-system check (update from alpha.132
  through the panel's own update screen; what was tried by hand, as which
  roles; what was not checked).
- Earlier limits still apply (see alpha.132).

## Evidence

No migration (schema stays 77). New endpoint `GET /api/v1/databases/visible`
(names, owners, domains, engines and user names only; never a password, a
connection secret or content). Deleting a database and setting a database
user's password now authorize by the visibility of the database's owner
instead of the caller's first subscription; a missing and a foreign record
answer alike; both audit rows name who acted and whose database it was
(`database.delete`). The Agent's blocklist answer gains the additive gob
fields Outcome, Reason and Sublists; an older Panel reads Listed, which a
current Agent sets only for a documented listing, and an older Agent's
answer is shown as "could not check". Release sequence 132 → 133.
